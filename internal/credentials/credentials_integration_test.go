//go:build integration

package credentials_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/credentials"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// credNow is the fixed instant every test's clock starts at.
var credNow = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

// credCipher builds a fresh random 32-byte AES-256-GCM cipher, matching
// internal/store/postgres's own integration test helper
// (integrations_integration_test.go's integrationCipher).
func credCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	c, err := crypto.NewCipher(key)
	require.NoError(t, err)
	return c
}

// credInsertDefinition inserts an integration_definitions row directly —
// the same pattern internal/ingest's own integration test and
// internal/store/postgres's use for a provider the seed catalogue does not
// carry (only isolar/{EU,CN,AU} come from
// internal/seed/data/integration_definitions.json — session-2 ruling 13).
func credInsertDefinition(t *testing.T, ctx context.Context, pool *pgxpool.Pool, provider, subtype string, endpoints map[string]string) uuid.UUID {
	t.Helper()
	var raw []byte
	if endpoints != nil {
		var err error
		raw, err = json.Marshal(endpoints)
		require.NoError(t, err)
	} else {
		raw = []byte(`{}`)
	}
	var id uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_definitions (provider, subtype, endpoints) values ($1, $2, $3::jsonb) returning id`,
		provider, subtype, raw).Scan(&id))
	return id
}

// --- fakes ---------------------------------------------------------------

// credFakeVerifier is credentials.Verifier: it records every call and
// returns whatever error it is configured with (nil by default).
type credFakeVerifier struct {
	mu    sync.Mutex
	err   error
	calls int
}

func (v *credFakeVerifier) Verify(context.Context, integration.Credentials) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.calls++
	return v.err
}

// credFakeVerifierResolver is credentials.VerifierResolver, handing out one
// credFakeVerifier per provider, created lazily so a test can configure it
// (e.g. its err) before or after the first Verifier() call.
type credFakeVerifierResolver struct {
	mu        sync.Mutex
	verifiers map[integration.Provider]*credFakeVerifier
}

func newCredFakeVerifierResolver() *credFakeVerifierResolver {
	return &credFakeVerifierResolver{verifiers: make(map[integration.Provider]*credFakeVerifier)}
}

func (r *credFakeVerifierResolver) Verifier(p integration.Provider) (credentials.Verifier, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.verifiers[p]
	if !ok {
		v = &credFakeVerifier{}
		r.verifiers[p] = v
	}
	return v, nil
}

func (r *credFakeVerifierResolver) forProvider(p integration.Provider) *credFakeVerifier {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.verifiers[p]
	if !ok {
		v = &credFakeVerifier{}
		r.verifiers[p] = v
	}
	return v
}

// credFakeISolar is credentials.ISolarTokens. AuthorizeURL returns
// redirectURI UNCHANGED, so a test can recover the signed state straight
// out of the URL credentials.Service.ISolarAuthorizeURL returns (it always
// appends "?state=..." to Deps.RedirectURI before calling AuthorizeURL).
type credFakeISolar struct {
	mu            sync.Mutex
	exchangeToken isolar.Token
	refreshToken  isolar.Token
	refreshDelay  time.Duration
	refreshCalls  int32
	exchangeCalls int32
}

func (f *credFakeISolar) AuthorizeURL(_ integration.Credentials, redirectURI string) (string, error) {
	return redirectURI, nil
}

func (f *credFakeISolar) ExchangeCode(context.Context, integration.Credentials, string, string) (isolar.Token, error) {
	atomic.AddInt32(&f.exchangeCalls, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exchangeToken, nil
}

func (f *credFakeISolar) Refresh(_ context.Context, _ integration.Credentials) (isolar.Token, error) {
	atomic.AddInt32(&f.refreshCalls, 1)
	f.mu.Lock()
	delay := f.refreshDelay
	tok := f.refreshToken
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	return tok, nil
}

func (f *credFakeISolar) setExchangeToken(tok isolar.Token) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exchangeToken = tok
}

func (f *credFakeISolar) setRefreshToken(tok isolar.Token) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshToken = tok
}

func (f *credFakeISolar) RefreshCalls() int32 { return atomic.LoadInt32(&f.refreshCalls) }

// recordingEnqueuer is ingest.Enqueuer, recording every task it is asked to
// enqueue.
type recordingEnqueuer struct {
	mu    sync.Mutex
	tasks []*asynq.Task
}

func (e *recordingEnqueuer) Enqueue(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tasks = append(e.tasks, task)
	return &asynq.TaskInfo{ID: fmt.Sprintf("task-%d", len(e.tasks))}, nil
}

func (e *recordingEnqueuer) Tasks() []*asynq.Task {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*asynq.Task, len(e.tasks))
	copy(out, e.tasks)
	return out
}

// --- harness ---------------------------------------------------------------

// credHarness bundles one *credentials.Service, real postgres repositories
// and every fake it depends on, so each test can reach whichever it needs
// to assert on.
type credHarness struct {
	svc          *credentials.Service
	integrations *postgres.IntegrationRepository
	analyzers    *postgres.AnalyzerRepository
	enqueuer     *recordingEnqueuer
	isolarFake   *credFakeISolar
	verifiers    *credFakeVerifierResolver
	clock        *clock.Fake
	stateKey     []byte
	redirectURI  string
}

func newCredHarness(t *testing.T, pool *pgxpool.Pool, now time.Time) *credHarness {
	t.Helper()
	h := &credHarness{
		integrations: postgres.NewIntegrationRepository(pool, credCipher(t)),
		analyzers:    postgres.NewAnalyzerRepository(pool),
		enqueuer:     &recordingEnqueuer{},
		isolarFake:   &credFakeISolar{},
		verifiers:    newCredFakeVerifierResolver(),
		clock:        clock.NewFake(now),
		stateKey:     []byte("integration-test-oauth-state-32b"),
		redirectURI:  "https://app.example.invalid/integrations/isolar/callback",
	}
	svc, err := credentials.New(credentials.Deps{
		Integrations: h.integrations,
		Analyzers:    h.analyzers,
		Verifiers:    h.verifiers,
		ISolar:       h.isolarFake,
		Enqueuer:     h.enqueuer,
		Locker:       lock.NewMemory(h.clock.Now),
		Nonces:       lock.NewMemory(h.clock.Now),
		Clock:        h.clock,
		StateKey:     h.stateKey,
		RedirectURI:  h.redirectURI,
		MaxRetry:     3,
	})
	require.NoError(t, err)
	h.svc = svc
	return h
}

// isolarExtraFixture is a plausible set of isolar Extra keys: app_id (for
// AuthorizeURL), app_key/secret_key (for every authenticated call).
func isolarExtraFixture(refreshToken, accessToken string) map[string]integration.Secret {
	m := map[string]integration.Secret{
		"app_id":     integration.NewSecret([]byte("fixture-app-id")),
		"app_key":    integration.NewSecret([]byte("fixture-app-key")),
		"secret_key": integration.NewSecret([]byte("fixture-secret-key")),
	}
	if refreshToken != "" {
		m["refresh_token"] = integration.NewSecret([]byte(refreshToken))
	}
	if accessToken != "" {
		m["access_token"] = integration.NewSecret([]byte(accessToken))
	}
	return m
}

func extractState(t *testing.T, authorizeURL string) string {
	t.Helper()
	u, err := url.Parse(authorizeURL)
	require.NoError(t, err)
	state := u.Query().Get("state")
	require.NotEmpty(t, state)
	return state
}

// --- tests -----------------------------------------------------------------

func TestConfiguredSecretNeverAppearsInListOrJSON(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140001)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	pw := "FIXTURE-PW-" + uuid.NewString()
	secret := integration.NewSecret([]byte(pw))
	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default",
		Username: ptr("user1"), Secret: &secret,
	})
	require.NoError(t, err)
	require.True(t, view.HasSecret)

	list, err := h.svc.List(ctx, tenant.AdminScope)
	require.NoError(t, err)
	require.Len(t, list, 1)

	rawView, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(rawView), pw)

	rawList, err := json.Marshal(list)
	require.NoError(t, err)
	require.NotContains(t, string(rawList), pw)

	require.NotContains(t, fmt.Sprintf("%+v", view), pw)
	require.NotContains(t, fmt.Sprintf("%+v", list), pw)

	var secretEnc []byte
	require.NoError(t, pool.QueryRow(ctx,
		`select secret_enc from integration_credentials where id = $1`, view.ID).Scan(&secretEnc))
	require.NotContains(t, string(secretEnc), pw)
	require.NotEmpty(t, secretEnc)
}

func TestUpdateWithoutSecretKeepsIt(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140002)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	pw := "FIXTURE-PW-ORIGINAL"
	secret := integration.NewSecret([]byte(pw))
	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default", Secret: &secret,
	})
	require.NoError(t, err)

	_, err = h.svc.Update(ctx, tenant.AdminScope, view.ID, credentials.Input{Username: ptr("changed-user")})
	require.NoError(t, err)

	creds, err := h.svc.Open(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	require.Equal(t, pw, creds.Secret.Reveal())
	require.Equal(t, "changed-user", creds.Username)
}

func TestUpdateMergesExtraKeys(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140003)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default",
		Extra: map[string]integration.Secret{
			"app_id":  integration.NewSecret([]byte("app-1")),
			"app_key": integration.NewSecret([]byte("key-1")),
		},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"app_id", "app_key"}, view.ExtraKeys)

	updated, err := h.svc.Update(ctx, tenant.AdminScope, view.ID, credentials.Input{
		Extra: map[string]integration.Secret{
			"secret_key": integration.NewSecret([]byte("sk-1")), // added
			"app_key":    {},                                    // zero: deleted
		},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"app_id", "secret_key"}, updated.ExtraKeys)

	creds, err := h.svc.Open(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	require.Equal(t, "app-1", creds.Extra["app_id"].Reveal())
	require.Equal(t, "sk-1", creds.Extra["secret_key"].Reveal())
	require.True(t, creds.Extra["app_key"].IsZero())

	// A nil Extra (not touched at all) leaves everything as-is.
	untouched, err := h.svc.Update(ctx, tenant.AdminScope, view.ID, credentials.Input{Username: ptr("still-untouched")})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"app_id", "secret_key"}, untouched.ExtraKeys)
}

func TestOpenIsScoped(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 140004)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 140005)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	secret := integration.NewSecret([]byte("FIXTURE-PW"))
	view, err := h.svc.Configure(ctx, tenantA.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default", Secret: &secret,
	})
	require.NoError(t, err)

	_, err = h.svc.Open(ctx, tenantB.AdminScope, view.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	// A positive control: tenant A itself can still open it.
	_, err = h.svc.Open(ctx, tenantA.AdminScope, view.ID)
	require.NoError(t, err)
}

func TestPM5340ConfigureCreatesItsAnalyzer(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140006)
	credInsertDefinition(t, ctx, pool, "pm5340", "default", nil)
	h := newCredHarness(t, pool, credNow)

	pm5340URL := "http://10.0.0.5:502"
	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderPM5340, Subtype: "default",
		PM5340URL: &pm5340URL, InstallationNumber: ptr("PM-INSTALL-1"),
	})
	require.NoError(t, err)
	require.Equal(t, &pm5340URL, view.PM5340URL)

	analyzer, err := h.analyzers.GetByInstallation(ctx, tenant.AdminScope, model.IntegrationProviderPM5340, "default", "PM-INSTALL-1")
	require.NoError(t, err)
	require.Equal(t, tenant.Company.ID, analyzer.CompanyID)
	require.True(t, analyzer.IsActive)
	require.True(t, analyzer.MeterMultiplier.Equal(decimal.NewFromInt(1)))

	// Idempotent: Update with the same installation number does not error
	// or create a second analyzer.
	_, err = h.svc.Update(ctx, tenant.AdminScope, view.ID, credentials.Input{InstallationNumber: ptr("PM-INSTALL-1")})
	require.NoError(t, err)
	list, err := h.analyzers.List(ctx, tenant.AdminScope, store.AnalyzerFilter{Providers: []model.IntegrationProvider{model.IntegrationProviderPM5340}})
	require.NoError(t, err)
	require.Len(t, list, 1)

	t.Run("rejects a URL with no scheme", func(t *testing.T) {
		bad := "10.0.0.5:502"
		_, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
			Provider: model.IntegrationProviderPM5340, Subtype: "default", PM5340URL: &bad,
		})
		require.ErrorIs(t, err, credentials.ErrInvalidSettings)
	})

	t.Run("rejects a URL with no host", func(t *testing.T) {
		bad := "http://"
		_, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
			Provider: model.IntegrationProviderPM5340, Subtype: "default", PM5340URL: &bad,
		})
		require.ErrorIs(t, err, credentials.ErrInvalidSettings)
	})
}

func TestDiscoverEnqueuesSyncAnalyzers(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140007)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	secret := integration.NewSecret([]byte("FIXTURE-PW"))
	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default", Secret: &secret,
	})
	require.NoError(t, err)

	taskID, err := h.svc.Discover(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	require.NotEmpty(t, taskID)

	tasks := h.enqueuer.Tasks()
	require.Len(t, tasks, 1)
	require.Equal(t, job.TypeIntegrationSyncAnalyzers, tasks[0].Type())

	p, err := job.DecodeSyncAnalyzers(tasks[0])
	require.NoError(t, err)
	require.Equal(t, tenant.Company.ID, p.CompanyID)
	require.Equal(t, view.ID, p.CredentialID)
}

func TestBackfillEnqueuesBackfillTask(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140008)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	secret := integration.NewSecret([]byte("FIXTURE-PW"))
	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default", Secret: &secret,
	})
	require.NoError(t, err)

	from, to := credNow.Add(-48*time.Hour), credNow
	taskID, err := h.svc.Backfill(ctx, tenant.AdminScope, view.ID, credentials.BackfillInput{
		Kinds: []model.ReadingKind{model.ReadingKindLoadProfile}, From: from, To: to,
	})
	require.NoError(t, err)
	require.NotEmpty(t, taskID)

	tasks := h.enqueuer.Tasks()
	require.Len(t, tasks, 1)
	require.Equal(t, job.TypeIntegrationBackfill, tasks[0].Type())
	p, err := job.DecodeBackfill(tasks[0])
	require.NoError(t, err)
	require.Equal(t, view.ID, p.CredentialID)
	require.True(t, p.From.Equal(from))
	require.True(t, p.To.Equal(to))
}

func TestVerifyRedactsAuthFailureAndRecordsSuccess(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140009)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	pw := "FIXTURE-PW-SECRET-VALUE"
	secret := integration.NewSecret([]byte(pw))
	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default", Secret: &secret,
	})
	require.NoError(t, err)

	fv := h.verifiers.forProvider(integration.ProviderGridBox)
	fv.err = &integration.Error{Kind: integration.ErrAuth, Provider: integration.ProviderGridBox, Op: "verify", HTTPStatus: 401}

	// The isolar error text never carries the credential; use a message
	// that WOULD carry it if redaction were broken.
	fv.err = fmt.Errorf("gridbox verify: login rejected for password %s (HTTP 401)", pw)

	_, err = h.svc.Verify(ctx, tenant.AdminScope, view.ID)
	require.Error(t, err)
	require.NotContains(t, err.Error(), pw)

	fv.err = nil
	updated, err := h.svc.Verify(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.LastVerifiedAt)
}

func TestISolarCallbackStoresTokensForTheStatesCompany(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 140010)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 140011)
	credInsertDefinition(t, ctx, pool, "isolar", "EU", nil)
	h := newCredHarness(t, pool, credNow)

	view, err := h.svc.Configure(ctx, tenantA.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderISolar, Subtype: "EU",
		Extra: isolarExtraFixture("", ""),
	})
	require.NoError(t, err)

	h.isolarFake.setExchangeToken(isolar.Token{
		AccessToken:  integration.NewSecret([]byte("access-1")),
		RefreshToken: integration.NewSecret([]byte("refresh-1")),
		ExpiresAt:    credNow.Add(2 * time.Hour),
	})

	authorizeURL, err := h.svc.ISolarAuthorizeURL(ctx, tenantA.AdminScope, view.ID)
	require.NoError(t, err)
	state := extractState(t, authorizeURL)

	require.NoError(t, h.svc.ISolarCallback(ctx, "auth-code", state))

	list, err := h.svc.List(ctx, tenantA.AdminScope)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NotNil(t, list[0].TokenExpiresAt)
	require.True(t, list[0].TokenExpiresAt.Equal(credNow.Add(2*time.Hour)))
	require.NotNil(t, list[0].LastVerifiedAt)

	// tenant B's own scope proves the callback wrote under the STATE's
	// company (derived internally, via store.SystemScope), never a
	// caller-supplied or default scope: tenant B has no credential at all.
	listB, err := h.svc.List(ctx, tenantB.AdminScope)
	require.NoError(t, err)
	require.Empty(t, listB)
}

// TestISolarCallbackKeepsRefreshTokenWhenProviderOmitsANewOne is the
// dedicated "zero refresh token overwrites" guard the Rules section names
// alongside the four Step-5 mutations. A provider response with no
// refresh_token decodes to a ZERO isolar.Token.RefreshToken
// (Token.IsZero() == true); persistIsolarToken must leave the stored
// refresh_token exactly as it was.
func TestISolarCallbackKeepsRefreshTokenWhenProviderOmitsANewOne(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140012)
	credInsertDefinition(t, ctx, pool, "isolar", "EU", nil)
	h := newCredHarness(t, pool, credNow)

	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderISolar, Subtype: "EU",
		Extra: isolarExtraFixture("refresh-original", "access-old"),
	})
	require.NoError(t, err)
	require.NoError(t, h.integrations.RecordVerification(ctx, tenant.AdminScope, view.ID, credNow, ptr(credNow.Add(1*time.Minute))))

	// The provider's callback response carries a NEW access token but NO
	// refresh token (Token.RefreshToken stays zero).
	h.isolarFake.setExchangeToken(isolar.Token{
		AccessToken:  integration.NewSecret([]byte("access-new")),
		RefreshToken: integration.Secret{},
		ExpiresAt:    credNow.Add(2 * time.Hour),
	})

	authorizeURL, err := h.svc.ISolarAuthorizeURL(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	state := extractState(t, authorizeURL)
	require.NoError(t, h.svc.ISolarCallback(ctx, "auth-code", state))

	creds, err := h.svc.Open(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	require.Equal(t, "refresh-original", creds.Extra["refresh_token"].Reveal(),
		"a zero RefreshToken from the provider must never overwrite the stored one")
}

func TestISolarAccessTokenDoesNotRefreshFreshToken(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140013)
	credInsertDefinition(t, ctx, pool, "isolar", "EU", nil)
	h := newCredHarness(t, pool, credNow)

	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderISolar, Subtype: "EU",
		Extra: isolarExtraFixture("refresh-1", "access-fresh"),
	})
	require.NoError(t, err)
	// Far from expiry: well outside the 5-minute refresh window.
	require.NoError(t, h.integrations.RecordVerification(ctx, tenant.AdminScope, view.ID, credNow, ptr(credNow.Add(2*time.Hour))))

	tok, err := h.svc.ISolarAccessToken(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	require.Equal(t, "access-fresh", tok.Reveal())
	require.EqualValues(t, 0, h.isolarFake.RefreshCalls())
}

// TestISolarAccessTokenRefreshesOnceUnderConcurrency is the central proof
// for Deps.Locker's double-checked refresh: 10 concurrent callers, a token
// expiring in 1 minute (inside the 5-minute refresh window), exactly ONE
// Refresh call, and every caller gets the SAME new token.
func TestISolarAccessTokenRefreshesOnceUnderConcurrency(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140014)
	credInsertDefinition(t, ctx, pool, "isolar", "EU", nil)
	h := newCredHarness(t, pool, credNow)

	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderISolar, Subtype: "EU",
		Extra: isolarExtraFixture("refresh-1", "access-old"),
	})
	require.NoError(t, err)
	require.NoError(t, h.integrations.RecordVerification(ctx, tenant.AdminScope, view.ID, credNow, ptr(credNow.Add(1*time.Minute))))

	h.isolarFake.setRefreshToken(isolar.Token{
		AccessToken:  integration.NewSecret([]byte("access-new")),
		RefreshToken: integration.NewSecret([]byte("refresh-new")),
		ExpiresAt:    credNow.Add(2 * time.Hour),
	})
	h.isolarFake.mu.Lock()
	h.isolarFake.refreshDelay = 50 * time.Millisecond
	h.isolarFake.mu.Unlock()

	const n = 10
	results := make([]integration.Secret, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = h.svc.ISolarAccessToken(ctx, tenant.AdminScope, view.ID)
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		require.NoError(t, errs[i])
		require.Equal(t, "access-new", results[i].Reveal())
	}
	require.EqualValues(t, 1, h.isolarFake.RefreshCalls(), "exactly one Refresh call must reach the provider")
}
