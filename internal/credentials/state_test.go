package credentials

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var stateTestKey = []byte("test-hmac-key-0123456789abcdef!!") // 33 bytes >= minStateKeyLen

// TestStateRoundTrip proves signState/verifyState agree with each other:
// a freshly signed state verifies to the exact claims it was signed with.
func TestStateRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	companyID, credentialID := uuid.New(), uuid.New()

	token, err := signState(stateTestKey, companyID, credentialID, now)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := verifyState(stateTestKey, token, now)
	require.NoError(t, err)
	require.Equal(t, companyID, claims.CompanyID)
	require.Equal(t, credentialID, claims.CredentialID)
	require.True(t, claims.Expiry.Equal(now.Add(stateTTL)))

	// Verifying a few seconds later, still inside the 10-minute window, must
	// still succeed.
	claims2, err := verifyState(stateTestKey, token, now.Add(9*time.Minute))
	require.NoError(t, err)
	require.Equal(t, claims.Nonce, claims2.Nonce)
}

// TestStateRejectsTamperExpiryAndWrongKey covers every named mutation
// pattern for state verification: a tampered payload (including one that
// swaps in ANOTHER company's id — "wrong-company state accepted"), the
// wrong signing key, and a correctly-signed-but-now-expired token. Each is
// asserted independently so that a broken verifyState which checks the
// HMAC with == and skips the expiry entirely (Step 5(c)'s mutation) is
// caught specifically by the expiry sub-test, not accidentally saved by
// one of the others.
func TestStateRejectsTamperExpiryAndWrongKey(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	companyID, credentialID := uuid.New(), uuid.New()
	otherCompanyID := uuid.New()

	token, err := signState(stateTestKey, companyID, credentialID, now)
	require.NoError(t, err)

	t.Run("wrong key", func(t *testing.T) {
		_, err := verifyState([]byte("a-completely-different-key-here"), token, now)
		require.ErrorIs(t, err, ErrInvalidState)
	})

	t.Run("expired, correct signature", func(t *testing.T) {
		// The token is unmodified and its signature is valid; only the
		// verification time has moved past the 10-minute window. A
		// verifyState that skips the expiry check accepts this.
		_, err := verifyState(stateTestKey, token, now.Add(stateTTL+time.Second))
		require.ErrorIs(t, err, ErrInvalidState)
	})

	t.Run("tampered payload: wrong company id, same signature", func(t *testing.T) {
		tampered := tamperStatePayload(t, token, func(payload []byte) {
			copy(payload[0:16], otherCompanyID[:])
		})
		_, err := verifyState(stateTestKey, tampered, now)
		require.ErrorIs(t, err, ErrInvalidState, "a state naming a different company must not verify — the HMAC no longer matches the tampered payload")
	})

	t.Run("tampered payload: wrong credential id", func(t *testing.T) {
		otherCred := uuid.New()
		tampered := tamperStatePayload(t, token, func(payload []byte) {
			copy(payload[16:32], otherCred[:])
		})
		_, err := verifyState(stateTestKey, tampered, now)
		require.ErrorIs(t, err, ErrInvalidState)
	})

	t.Run("malformed token", func(t *testing.T) {
		_, err := verifyState(stateTestKey, "not-a-valid-state-token", now)
		require.ErrorIs(t, err, ErrInvalidState)
	})

	t.Run("empty token", func(t *testing.T) {
		_, err := verifyState(stateTestKey, "", now)
		require.ErrorIs(t, err, ErrInvalidState)
	})
}

// tamperStatePayload decodes token's payload half, applies mutate, and
// re-encodes it WITHOUT recomputing the signature — simulating an attacker
// who can see and edit the payload but does not know the HMAC key.
func tamperStatePayload(t *testing.T, token string, mutate func(payload []byte)) string {
	t.Helper()
	encPayload, encSig, ok := strings.Cut(token, ".")
	require.True(t, ok)
	payload, err := base64.RawURLEncoding.DecodeString(encPayload)
	require.NoError(t, err)
	mutate(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + encSig
}

// TestStateIsSingleUse is R3's central proof: the first ISolarCallback for
// a state succeeds, and a replay of the SAME state — under its own short
// deadline — fails FAST with ErrInvalidState rather than hanging. See
// isolar_oauth.go's ISolarCallback doc comment for why this must be a
// non-blocking Consume, never a held Locker lease.
func TestStateIsSingleUse(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.NewFake(now)

	companyID, credentialID, definitionID := uuid.New(), uuid.New(), uuid.New()

	integrations := newStateFakeIntegrations()
	integrations.defs = append(integrations.defs, model.IntegrationDefinition{
		ID: definitionID, Provider: model.IntegrationProviderISolar, Subtype: "EU",
		Endpoints: json.RawMessage(`{"gateway":"https://gw.example","authorize_origin":"https://auth.example","cloud_id":"3","token":"/token","refresh_token":"/refresh"}`),
	})
	extraPlain, err := json.Marshal(map[string]string{"app_id": "app-1", "app_key": "key-1", "secret_key": "secret-1"})
	require.NoError(t, err)
	integrations.put(model.IntegrationCredential{
		ID: credentialID, CompanyID: companyID, DefinitionID: definitionID, IsActive: true,
	}, nil, extraPlain)

	isolarFake := &stateFakeISolar{
		exchangeToken: isolar.Token{
			AccessToken:  integration.NewSecret([]byte("access-token-1")),
			RefreshToken: integration.NewSecret([]byte("refresh-token-1")),
			ExpiresAt:    now.Add(2 * time.Hour),
		},
	}

	svc, err := New(Deps{
		Integrations: integrations,
		Analyzers:    stateFakeAnalyzers{},
		Buildings:    stateFakeBuildings{},
		Verifiers:    stateFakeVerifiers{},
		ISolar:       isolarFake,
		Enqueuer:     stateFakeEnqueuer{},
		Locker:       lock.NewMemory(clk.Now),
		Nonces:       lock.NewMemory(clk.Now),
		Clock:        clk,
		StateKey:     stateTestKey,
		RedirectURI:  "https://app.example.invalid/integrations/isolar/callback",
		MaxRetry:     3,
	})
	require.NoError(t, err)

	state, err := signState(stateTestKey, companyID, credentialID, now)
	require.NoError(t, err)

	// First use succeeds.
	require.NoError(t, svc.ISolarCallback(context.Background(), "auth-code-1", state))
	require.Equal(t, 1, isolarFake.exchangeCalls())

	// Replay, under a short deadline: must fail fast with ErrInvalidState,
	// not hang until the deadline and fail with context.DeadlineExceeded.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	err = svc.ISolarCallback(ctx, "auth-code-1", state)
	elapsed := time.Since(start)

	require.ErrorIs(t, err, ErrInvalidState)
	require.Less(t, elapsed, 500*time.Millisecond, "a replay must fail immediately, never block")
	require.Equal(t, 1, isolarFake.exchangeCalls(), "a replay must never reach the provider a second time")
}

// --- fakes used only by this file's tests -----------------------------

type stateFakeIntegrations struct {
	mu          sync.Mutex
	defs        []model.IntegrationDefinition
	credentials map[uuid.UUID]model.IntegrationCredential
	secretPlain map[uuid.UUID][]byte
	extraPlain  map[uuid.UUID][]byte
}

func newStateFakeIntegrations() *stateFakeIntegrations {
	return &stateFakeIntegrations{
		credentials: make(map[uuid.UUID]model.IntegrationCredential),
		secretPlain: make(map[uuid.UUID][]byte),
		extraPlain:  make(map[uuid.UUID][]byte),
	}
}

func (f *stateFakeIntegrations) put(c model.IntegrationCredential, secret, extra []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(extra) > 0 {
		c.ExtraEnc = []byte("sealed")
	}
	if len(secret) > 0 {
		c.SecretEnc = []byte("sealed")
	}
	f.credentials[c.ID] = c
	f.secretPlain[c.ID] = secret
	f.extraPlain[c.ID] = extra
}

func (f *stateFakeIntegrations) Definitions(_ context.Context, s store.Scope) ([]model.IntegrationDefinition, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]model.IntegrationDefinition, len(f.defs))
	copy(out, f.defs)
	return out, nil
}

func (f *stateFakeIntegrations) Definition(_ context.Context, s store.Scope, provider model.IntegrationProvider, subtype string) (model.IntegrationDefinition, error) {
	if !s.Valid() {
		return model.IntegrationDefinition{}, store.ErrInvalidScope
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.defs {
		if d.Provider == provider && d.Subtype == subtype {
			return d, nil
		}
	}
	return model.IntegrationDefinition{}, store.ErrNotFound
}

func (f *stateFakeIntegrations) Credential(_ context.Context, s store.Scope, definitionID uuid.UUID) (model.IntegrationCredential, error) {
	if !s.Valid() {
		return model.IntegrationCredential{}, store.ErrInvalidScope
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.credentials {
		if c.DefinitionID == definitionID && c.CompanyID == s.CompanyID {
			return c, nil
		}
	}
	return model.IntegrationCredential{}, store.ErrNotFound
}

func (f *stateFakeIntegrations) ListCredentials(_ context.Context, s store.Scope) ([]model.IntegrationCredential, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.IntegrationCredential
	for _, c := range f.credentials {
		if c.CompanyID == s.CompanyID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *stateFakeIntegrations) UpsertCredential(_ context.Context, s store.Scope, c model.IntegrationCredential, secret, extra []byte) (model.IntegrationCredential, error) {
	if !s.Valid() {
		return model.IntegrationCredential{}, store.ErrInvalidScope
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.credentials[c.ID]
	if len(secret) > 0 {
		f.secretPlain[c.ID] = append([]byte(nil), secret...)
		c.SecretEnc = []byte("sealed")
	} else if ok {
		c.SecretEnc = existing.SecretEnc
	}
	if len(extra) > 0 {
		f.extraPlain[c.ID] = append([]byte(nil), extra...)
		c.ExtraEnc = []byte("sealed")
	} else if ok {
		c.ExtraEnc = existing.ExtraEnc
	}
	if ok {
		c.TokenExpiresAt = existing.TokenExpiresAt
		c.LastVerifiedAt = existing.LastVerifiedAt
	}
	c.CompanyID = s.CompanyID
	f.credentials[c.ID] = c
	return c, nil
}

func (f *stateFakeIntegrations) OpenSecret(_ context.Context, s store.Scope, credentialID uuid.UUID) ([]byte, []byte, error) {
	if !s.Valid() {
		return nil, nil, store.ErrInvalidScope
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.credentials[credentialID]
	if !ok || c.CompanyID != s.CompanyID {
		return nil, nil, store.ErrNotFound
	}
	return append([]byte(nil), f.secretPlain[credentialID]...), append([]byte(nil), f.extraPlain[credentialID]...), nil
}

func (f *stateFakeIntegrations) DeleteCredential(_ context.Context, s store.Scope, credentialID uuid.UUID) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.credentials[credentialID]
	if !ok || c.CompanyID != s.CompanyID {
		return store.ErrNotFound
	}
	delete(f.credentials, credentialID)
	delete(f.secretPlain, credentialID)
	delete(f.extraPlain, credentialID)
	return nil
}

func (f *stateFakeIntegrations) RecordVerification(_ context.Context, s store.Scope, credentialID uuid.UUID, verifiedAt time.Time, tokenExpiresAt *time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.credentials[credentialID]
	if !ok || c.CompanyID != s.CompanyID {
		return store.ErrNotFound
	}
	c.LastVerifiedAt = &verifiedAt
	c.TokenExpiresAt = tokenExpiresAt
	f.credentials[credentialID] = c
	return nil
}

var _ store.IntegrationRepository = (*stateFakeIntegrations)(nil)

// stateFakeBuildings is a not-implemented stub: no state test names a building.
type stateFakeBuildings struct{ store.BuildingRepository }

// stateFakeAnalyzers is a not-implemented stub: TestStateIsSingleUse never
// exercises the pm5340 analyzer path.
type stateFakeAnalyzers struct{}

func (stateFakeAnalyzers) Get(context.Context, store.Scope, uuid.UUID) (model.Analyzer, error) {
	return model.Analyzer{}, errors.New("not implemented")
}
func (stateFakeAnalyzers) GetByInstallation(context.Context, store.Scope, model.IntegrationProvider, string, string) (model.Analyzer, error) {
	return model.Analyzer{}, errors.New("not implemented")
}
func (stateFakeAnalyzers) List(context.Context, store.Scope, store.AnalyzerFilter) ([]model.Analyzer, error) {
	return nil, errors.New("not implemented")
}
func (stateFakeAnalyzers) Create(context.Context, store.Scope, model.Analyzer) (model.Analyzer, error) {
	return model.Analyzer{}, errors.New("not implemented")
}
func (stateFakeAnalyzers) Update(context.Context, store.Scope, model.Analyzer) (model.Analyzer, error) {
	return model.Analyzer{}, errors.New("not implemented")
}
func (stateFakeAnalyzers) SoftDelete(context.Context, store.Scope, uuid.UUID, time.Time) error {
	return errors.New("not implemented")
}
func (stateFakeAnalyzers) TouchLastReading(context.Context, store.Scope, uuid.UUID, time.Time) error {
	return errors.New("not implemented")
}

var _ store.AnalyzerRepository = (*stateFakeAnalyzers)(nil)

// stateFakeVerifiers is a not-implemented stub: TestStateIsSingleUse never
// calls Verify.
type stateFakeVerifiers struct{}

func (stateFakeVerifiers) Verifier(integration.Provider) (Verifier, error) {
	return nil, errors.New("not implemented")
}

// stateFakeEnqueuer is a not-implemented stub: TestStateIsSingleUse never
// calls Discover/Backfill.
type stateFakeEnqueuer struct{}

func (stateFakeEnqueuer) Enqueue(context.Context, *asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error) {
	return nil, errors.New("not implemented")
}

// stateFakeISolar implements ISolarTokens, recording ExchangeCode calls so
// TestStateIsSingleUse can assert the provider is reached exactly once.
type stateFakeISolar struct {
	mu            sync.Mutex
	exchangeToken isolar.Token
	calls         int
}

func (f *stateFakeISolar) AuthorizeURL(integration.Credentials, string) (string, error) {
	return "", errors.New("not implemented")
}

func (f *stateFakeISolar) ExchangeCode(_ context.Context, _ integration.Credentials, _, _ string) (isolar.Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.exchangeToken, nil
}

func (f *stateFakeISolar) Refresh(context.Context, integration.Credentials) (isolar.Token, error) {
	return isolar.Token{}, errors.New("not implemented")
}

func (f *stateFakeISolar) exchangeCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
