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
	// locker is the SAME lock.Locker handed to credentials.Deps.Locker —
	// kept here so a test can acquire the isolar token lock itself
	// (TestUpdateExtraBlocksWhileISolarTokenLockIsHeld, I3) to prove Update
	// actually serialises through it.
	locker lock.Locker
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
		locker:       lock.NewMemory(now2clock(now)),
	}
	svc, err := credentials.New(credentials.Deps{
		Integrations: h.integrations,
		Analyzers:    h.analyzers,
		Verifiers:    h.verifiers,
		ISolar:       h.isolarFake,
		Enqueuer:     h.enqueuer,
		Locker:       h.locker,
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

// now2clock returns a func() time.Time frozen at now — lock.Memory takes a
// clock func, and the harness's own h.clock.Now is equally frozen (the
// fake clock never advances during these tests), so either works; this
// helper avoids a forward reference to h.clock in newCredHarness's own
// struct literal.
func now2clock(now time.Time) func() time.Time {
	return func() time.Time { return now }
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

	// M5 (final-review-A): a URL carrying userinfo must be refused outright
	// — pm5340_url is stored in plaintext, never sealed, and the write-only
	// View returns it verbatim, so a credential embedded in the URL itself
	// would leak straight back out through View.PM5340URL.
	t.Run("rejects a URL with userinfo", func(t *testing.T) {
		bad := "http://user:pass@10.0.0.5:502"
		view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
			Provider: model.IntegrationProviderPM5340, Subtype: "creds-in-url",
			PM5340URL: &bad, InstallationNumber: ptr("PM-INSTALL-USERINFO"),
		})
		require.ErrorIs(t, err, credentials.ErrInvalidSettings)
		require.Empty(t, view.ID, "a rejected Configure must not return a usable credential")

		// Never stored: no credential for this subtype exists to open or view.
		list, lerr := h.svc.List(ctx, tenant.AdminScope)
		require.NoError(t, lerr)
		for _, c := range list {
			require.NotEqual(t, "creds-in-url", c.Subtype, "the rejected credential must never have been stored")
		}
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
	require.False(t, p.Force, "Force defaults to false when the caller does not ask for it")
}

// TestBackfillForceInputReachesThePayload is R53's write-model plumbing
// proof: BackfillInput.Force must reach job.BackfillPayload.Force verbatim
// — without this, Backfiller.Backfill's Force re-mint logic would be
// unreachable from the one real production path that enqueues a backfill
// (credentials.Service.Backfill), since nothing else in F2 builds a
// job.BackfillPayload by hand.
func TestBackfillForceInputReachesThePayload(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140009)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	secret := integration.NewSecret([]byte("FIXTURE-PW"))
	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default", Secret: &secret,
	})
	require.NoError(t, err)

	from, to := credNow.Add(-48*time.Hour), credNow
	_, err = h.svc.Backfill(ctx, tenant.AdminScope, view.ID, credentials.BackfillInput{
		Kinds: []model.ReadingKind{model.ReadingKindLoadProfile}, From: from, To: to, Force: true,
	})
	require.NoError(t, err)

	tasks := h.enqueuer.Tasks()
	require.Len(t, tasks, 1)
	p, err := job.DecodeBackfill(tasks[0])
	require.NoError(t, err)
	require.True(t, p.Force, "BackfillInput.Force must reach job.BackfillPayload.Force")
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

// TestVerifyPreservesErrorChain is fix round 2's M proof (dropped last
// round): Verify's redacted error must still let a caller errors.Is
// against the verifier's own sentinel (integration.ErrAuth) — fix round
// 1's Verify returned errors.New(redactedText(...)), a brand-new error
// with no Unwrap, which threw the chain away entirely even though the
// redacted TEXT it produced was already correct (that text-only guarantee
// is TestVerifyRedactsAuthFailureAndRecordsSuccess, above, which never
// checked errors.Is). This test asserts BOTH: the secret is still absent
// from Error(), AND errors.Is(err, integration.ErrAuth) is true.
func TestVerifyPreservesErrorChain(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140026)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	pw := "FIXTURE-PW-SECRET-VALUE-CHAIN"
	secret := integration.NewSecret([]byte(pw))
	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default", Secret: &secret,
	})
	require.NoError(t, err)

	fv := h.verifiers.forProvider(integration.ProviderGridBox)
	// The message text WOULD carry the credential if redaction were
	// broken; %w wraps integration.ErrAuth so the chain has something
	// real to preserve.
	fv.err = fmt.Errorf("gridbox verify: login rejected for password %s (HTTP 401): %w", pw, integration.ErrAuth)

	_, err = h.svc.Verify(ctx, tenant.AdminScope, view.ID)
	require.Error(t, err)
	require.NotContains(t, err.Error(), pw, "the redacted text must never carry the credential")
	require.ErrorIs(t, err, integration.ErrAuth,
		"Verify's redacted error must preserve the chain so callers can errors.Is against the verifier's sentinel")
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

	// Folded minor (fix round 1): tenant B gets its OWN isolar credential
	// FIRST, with its own refresh token, so "tenant B's list is unchanged"
	// below is a meaningful cross-tenant proof — not just "tenant B has
	// nothing at all so of course nothing changed".
	viewB, err := h.svc.Configure(ctx, tenantB.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderISolar, Subtype: "EU",
		Extra: isolarExtraFixture("tenant-b-refresh", "tenant-b-access"),
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
	// caller-supplied or default scope: tenant B's OWN credential (and its
	// OWN refresh token, configured above) is completely untouched. Read
	// via the repository directly, NOT svc.Open: tenant B's credential has
	// never been verified (no TokenExpiresAt), so Open's own isolar
	// freshness check would treat it as "not fresh" and trigger an
	// UNRELATED refresh attempt of its own — exactly the side effect this
	// assertion must not introduce while checking for one.
	_, extraPlainB, err := h.integrations.OpenSecret(ctx, tenantB.AdminScope, viewB.ID)
	require.NoError(t, err)
	var extraB map[string]string
	require.NoError(t, json.Unmarshal(extraPlainB, &extraB))
	require.Equal(t, "tenant-b-refresh", extraB["refresh_token"])
	require.Equal(t, "tenant-b-access", extraB["access_token"])

	listB, err := h.svc.List(ctx, tenantB.AdminScope)
	require.NoError(t, err)
	require.Len(t, listB, 1, "tenant B's own credential — and only that one")
	require.Nil(t, listB[0].LastVerifiedAt, "tenant A's callback must never touch tenant B's row")
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

// ============================================================================
// Fix round 1 (opus review of 0ec2398..9ce1bbc) — I1-I5 and the folded
// "invalid key accepted" minor. See task-14-fix1-findings.md.
// ============================================================================

// TestConfigureRefusesWhenCredentialAlreadyExists is I2/R45: a second
// Configure for the same (company, definition) must be refused with an
// ErrConflict-matching error, and must NOT touch the existing row at all —
// the original bug silently replaced it via UpsertCredential's upsert,
// wiping its sealed Extra (any OAuth tokens) and resetting
// settings/is_active to whatever the second call's Input happened to carry.
func TestConfigureRefusesWhenCredentialAlreadyExists(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140019)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	secret := integration.NewSecret([]byte("FIXTURE-PW-ORIGINAL"))
	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default",
		Username: ptr("original-user"), Secret: &secret,
		Settings: json.RawMessage(`{"use_billing_indexes":true}`),
		Extra: map[string]integration.Secret{
			"app_id": integration.NewSecret([]byte("original-app-id")),
		},
	})
	require.NoError(t, err)

	otherSecret := integration.NewSecret([]byte("FIXTURE-PW-OVERWRITE-ATTEMPT"))
	_, err = h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default",
		Username: ptr("overwrite-attempt-user"), Secret: &otherSecret,
	})
	require.ErrorIs(t, err, store.ErrConflict)

	// Nothing about the original credential changed: same secret, same
	// settings, same Extra — Configure's failed second call left no trace.
	creds, err := h.svc.Open(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	require.Equal(t, "FIXTURE-PW-ORIGINAL", creds.Secret.Reveal())
	require.Equal(t, "original-app-id", creds.Extra["app_id"].Reveal())

	list, err := h.svc.List(ctx, tenant.AdminScope)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, ptr("original-user"), list[0].Username)
	require.JSONEq(t, `{"use_billing_indexes":true}`, string(list[0].Settings))
}

// TestVerifyPreservesRefreshedTokenExpiry is I1's proof. Verify calls Open,
// which — for an isolar credential whose token is inside the 5-minute
// refresh window — refreshes it and ALREADY persists the new
// token_expires_at (persistIsolarToken's own RecordVerification call).
// Verify's own, later RecordVerification call must record that SAME
// refreshed expiry, never the stale, pre-refresh one it would get from
// creds.TokenExpiresAt (built by buildCredentials before Open's refresh
// ran).
func TestVerifyPreservesRefreshedTokenExpiry(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140020)
	credInsertDefinition(t, ctx, pool, "isolar", "EU", nil)
	h := newCredHarness(t, pool, credNow)

	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderISolar, Subtype: "EU",
		Extra: isolarExtraFixture("refresh-1", "access-old"),
	})
	require.NoError(t, err)
	// Inside the 5-minute refresh window: Open (inside Verify) will refresh.
	require.NoError(t, h.integrations.RecordVerification(ctx, tenant.AdminScope, view.ID, credNow, ptr(credNow.Add(1*time.Minute))))

	refreshedExpiry := credNow.Add(2 * time.Hour)
	h.isolarFake.setRefreshToken(isolar.Token{
		AccessToken:  integration.NewSecret([]byte("access-new")),
		RefreshToken: integration.NewSecret([]byte("refresh-new")),
		ExpiresAt:    refreshedExpiry,
	})

	updated, err := h.svc.Verify(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.TokenExpiresAt)
	require.True(t, updated.TokenExpiresAt.Equal(refreshedExpiry),
		"Verify must record the REFRESHED expiry, not the stale pre-refresh one")

	// The stored expiry is now far from the refresh window: a further
	// ISolarAccessToken call must make ZERO extra Refresh calls — a stale
	// stored expiry (still ~1 minute out) would trigger a second one.
	tok, err := h.svc.ISolarAccessToken(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	require.Equal(t, "access-new", tok.Reveal())
	require.EqualValues(t, 1, h.isolarFake.RefreshCalls(), "Verify's own refresh must be the ONLY Refresh call")
}

// TestUpdateExtraBlocksWhileISolarTokenLockIsHeld is I3's deterministic
// mutation proof. Update's read-merge-write of an isolar credential's
// Extra must serialise under the SAME lock key
// ("isolar:token:<company>") as ISolarAccessToken's refresh and
// ISolarCallback's persistIsolarToken — otherwise a concurrent Update can
// read the pre-refresh Extra, and its later write silently discards
// whichever new token a concurrent refresh just wrote (or vice versa).
//
// Rather than relying on network-delay timing to occasionally reproduce a
// lost update, this test acquires the lock itself, directly, before
// calling Update: with the fix, Update blocks until the test releases the
// lock; with I3's regression (Update's Extra merge unlocked), Update
// returns almost immediately regardless of who else holds the lock — that
// is exactly the observable difference this test pins.
func TestUpdateExtraBlocksWhileISolarTokenLockIsHeld(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140021)
	credInsertDefinition(t, ctx, pool, "isolar", "EU", nil)
	h := newCredHarness(t, pool, credNow)

	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderISolar, Subtype: "EU",
		Extra: isolarExtraFixture("refresh-1", "access-1"),
	})
	require.NoError(t, err)

	// Same key format as isolarTokenLockKey(sc.CompanyID) in service.go.
	lockKey := "isolar:token:" + tenant.Company.ID.String()
	lease, err := h.locker.Acquire(ctx, lockKey, 60*time.Second)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, uerr := h.svc.Update(ctx, tenant.AdminScope, view.ID, credentials.Input{
			Extra: map[string]integration.Secret{"note": integration.NewSecret([]byte("update-mark"))},
		})
		done <- uerr
	}()

	select {
	case uerr := <-done:
		t.Fatalf("Update returned (err=%v) while the isolar token lock was still held externally — "+
			"its Extra merge is not serialised under isolarTokenLockKey (I3 regression)", uerr)
	case <-time.After(200 * time.Millisecond):
		// Still blocked on the lock, as required.
	}

	require.NoError(t, lease.Release(ctx))

	select {
	case uerr := <-done:
		require.NoError(t, uerr)
	case <-time.After(2 * time.Second):
		t.Fatal("Update never returned after the lock was released")
	}

	creds, err := h.svc.Open(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	require.Equal(t, "update-mark", creds.Extra["note"].Reveal())
	require.Equal(t, "refresh-1", creds.Extra["refresh_token"].Reveal(), "Update's Extra merge must preserve unrelated keys")
}

// blockingUpsertRepo wraps a store.IntegrationRepository, blocking the
// FIRST call to UpsertCredential made AFTER the test arms it (via
// armed.Store(true)) until the test closes proceed, and signalling
// entered once that call has started. Every other UpsertCredential call —
// including Configure's own, made while setting up the test fixture,
// before arming — passes straight through to the embedded, real
// repository. It exists for TestUpdateHoldsIsolarLockAcrossUpsertWrite
// (I3, fix round 2): a fake Locker cannot tell the difference between
// "released before the write" and "released after the write" the way a
// real timing race would, but a repository that can pause UpsertCredential
// mid-flight lets the test force exactly that window open and prove
// nothing else can run in it. Arming is required because Configure (the
// test's own setup) also calls UpsertCredential once, before Update ever
// runs — without it, the sync.Once would fire (and hang, since nothing is
// listening on entered/proceed yet) during setup instead of during the
// Update this test targets.
type blockingUpsertRepo struct {
	store.IntegrationRepository
	armed   atomic.Bool
	once    sync.Once
	entered chan struct{}
	proceed chan struct{}
}

func (r *blockingUpsertRepo) UpsertCredential(ctx context.Context, s store.Scope, c model.IntegrationCredential, secretPlain, extra []byte) (model.IntegrationCredential, error) {
	if r.armed.Load() {
		r.once.Do(func() {
			close(r.entered)
			<-r.proceed
		})
	}
	return r.IntegrationRepository.UpsertCredential(ctx, s, c, secretPlain, extra)
}

// TestUpdateHoldsIsolarLockAcrossUpsertWrite is I3's fix-round-2 proof.
// TestUpdateExtraBlocksWhileISolarTokenLockIsHeld (above) only proves
// Update's Extra READ-MERGE runs under isolarTokenLockKey — fix round 1's
// bug was releasing the lease right after that merge, LEAVING THE ACTUAL
// UpsertCredential WRITE UNLOCKED, so that older test stayed green under
// the very regression this one targets. Here, UpsertCredential itself is
// made to block (via blockingUpsertRepo) so a concurrent
// ISolarAccessToken refresh can be started WHILE Update is provably
// inside its write: with the fix (lease held across the write, via
// defer), the refresh must not complete until Update's blocked
// UpsertCredential is released; with the regression (lease released
// before the write), the refresh's Acquire succeeds immediately and it
// races Update's write, and the two writes interleave in whichever order
// the goroutine scheduler picks.
func TestUpdateHoldsIsolarLockAcrossUpsertWrite(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140027)
	credInsertDefinition(t, ctx, pool, "isolar", "EU", nil)

	realIntegrations := postgres.NewIntegrationRepository(pool, credCipher(t))
	blocker := &blockingUpsertRepo{
		IntegrationRepository: realIntegrations,
		entered:               make(chan struct{}),
		proceed:               make(chan struct{}),
	}
	isolarFake := &credFakeISolar{}
	svc, err := credentials.New(credentials.Deps{
		Integrations: blocker,
		Analyzers:    postgres.NewAnalyzerRepository(pool),
		Verifiers:    newCredFakeVerifierResolver(),
		ISolar:       isolarFake,
		Enqueuer:     &recordingEnqueuer{},
		Locker:       lock.NewMemory(now2clock(credNow)),
		Nonces:       lock.NewMemory(now2clock(credNow)),
		Clock:        clock.NewFake(credNow),
		StateKey:     []byte("integration-test-oauth-state-32b"),
		RedirectURI:  "https://app.example.invalid/integrations/isolar/callback",
		MaxRetry:     3,
	})
	require.NoError(t, err)

	view, err := svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderISolar, Subtype: "EU",
		Extra: isolarExtraFixture("refresh-1", "access-old"),
	})
	require.NoError(t, err)
	// Inside the 5-minute refresh window, so ISolarAccessToken below will
	// attempt a real refresh rather than taking the fast, unlocked "fresh"
	// path.
	require.NoError(t, realIntegrations.RecordVerification(ctx, tenant.AdminScope, view.ID, credNow, ptr(credNow.Add(1*time.Minute))))

	isolarFake.setRefreshToken(isolar.Token{
		AccessToken:  integration.NewSecret([]byte("access-new")),
		RefreshToken: integration.NewSecret([]byte("refresh-new")),
		ExpiresAt:    credNow.Add(2 * time.Hour),
	})

	// Arm the blocker only now — Configure's own UpsertCredential call
	// above (test setup) must pass straight through.
	blocker.armed.Store(true)

	updateDone := make(chan error, 1)
	go func() {
		_, uerr := svc.Update(ctx, tenant.AdminScope, view.ID, credentials.Input{
			Extra: map[string]integration.Secret{"note": integration.NewSecret([]byte("update-mark"))},
		})
		updateDone <- uerr
	}()

	select {
	case <-blocker.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("Update never reached UpsertCredential")
	}

	refreshDone := make(chan struct {
		tok integration.Secret
		err error
	}, 1)
	go func() {
		tok, rerr := svc.ISolarAccessToken(ctx, tenant.AdminScope, view.ID)
		refreshDone <- struct {
			tok integration.Secret
			err error
		}{tok, rerr}
	}()

	select {
	case res := <-refreshDone:
		t.Fatalf("ISolarAccessToken's refresh completed (err=%v) while Update's UpsertCredential write was still "+
			"blocked — Update released the isolar token lock before its write, not after (I3 regression)", res.err)
	case <-time.After(200 * time.Millisecond):
		// Still blocked behind Update's held lease, as required.
	}

	close(blocker.proceed)

	require.NoError(t, <-updateDone)
	res := <-refreshDone
	require.NoError(t, res.err)
	require.Equal(t, "access-new", res.tok.Reveal())

	creds, err := svc.Open(ctx, tenant.AdminScope, view.ID)
	require.NoError(t, err)
	require.Equal(t, "update-mark", creds.Extra["note"].Reveal(),
		"Update's Extra write must have landed")
	require.Equal(t, "refresh-new", creds.Extra["refresh_token"].Reveal(),
		"the refresh's NEW refresh_token must be the one finally stored — Update ran (and released) first, so "+
			"the refresh's later read-merge-write, serialised after it by the SAME lock, sees Update's merge and "+
			"is never clobbered by (or itself clobbers) Update's write")
}

// TestISolarAccessTokenRefreshesOnceAcrossTwoServiceInstances is I4's
// central proof: Deps.Locker's double-checked refresh must serialise
// ACROSS process/instance boundaries, not just within one *Service's own
// goroutines — the real deployment shares one Redis-backed lock.Locker
// across every worker process. Two *credentials.Service instances (each
// with its own Clock, but sharing the SAME lock.Memory as their Locker and
// the SAME isolar fake/repositories) refresh concurrently: exactly ONE
// Refresh call must reach the provider.
func TestISolarAccessTokenRefreshesOnceAcrossTwoServiceInstances(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140022)
	credInsertDefinition(t, ctx, pool, "isolar", "EU", nil)

	sharedLocker := lock.NewMemory(now2clock(credNow))
	integrations := postgres.NewIntegrationRepository(pool, credCipher(t))
	analyzers := postgres.NewAnalyzerRepository(pool)
	isolarFake := &credFakeISolar{}
	verifiers := newCredFakeVerifierResolver()
	enqueuer := &recordingEnqueuer{}

	newInstance := func() *credentials.Service {
		svc, err := credentials.New(credentials.Deps{
			Integrations: integrations,
			Analyzers:    analyzers,
			Verifiers:    verifiers,
			ISolar:       isolarFake,
			Enqueuer:     enqueuer,
			Locker:       sharedLocker,
			Nonces:       lock.NewMemory(now2clock(credNow)),
			Clock:        clock.NewFake(credNow),
			StateKey:     []byte("integration-test-oauth-state-32b"),
			RedirectURI:  "https://app.example.invalid/integrations/isolar/callback",
			MaxRetry:     3,
		})
		require.NoError(t, err)
		return svc
	}
	svcA := newInstance()
	svcB := newInstance()

	view, err := svcA.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderISolar, Subtype: "EU",
		Extra: isolarExtraFixture("refresh-1", "access-old"),
	})
	require.NoError(t, err)
	require.NoError(t, integrations.RecordVerification(ctx, tenant.AdminScope, view.ID, credNow, ptr(credNow.Add(1*time.Minute))))

	isolarFake.setRefreshToken(isolar.Token{
		AccessToken:  integration.NewSecret([]byte("access-new")),
		RefreshToken: integration.NewSecret([]byte("refresh-new")),
		ExpiresAt:    credNow.Add(2 * time.Hour),
	})
	isolarFake.mu.Lock()
	isolarFake.refreshDelay = 50 * time.Millisecond
	isolarFake.mu.Unlock()

	const n = 10
	results := make([]integration.Secret, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		svc := svcA
		if i%2 == 0 {
			svc = svcB
		}
		go func(i int, svc *credentials.Service) {
			defer wg.Done()
			results[i], errs[i] = svc.ISolarAccessToken(ctx, tenant.AdminScope, view.ID)
		}(i, svc)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		require.NoError(t, errs[i])
		require.Equal(t, "access-new", results[i].Reveal())
	}
	require.EqualValues(t, 1, isolarFake.RefreshCalls(),
		"exactly one Refresh call must reach the provider, across BOTH service instances sharing one lock.Memory")
}

// TestPM5340UpdateChangesInstallationNumberOnSingleAnalyzer is I5's proof.
// Update with a DIFFERENT InstallationNumber must leave the credential with
// exactly ONE ACTIVE pm5340 analyzer, never two — the original bug looked
// the analyzer up by the (now stale) OLD installation number via
// GetByInstallation, found nothing, and created a SECOND active one
// alongside the first. AnalyzerRepository.Update cannot change
// installation_number in place (it is that repository's own immutable
// natural key), so the fix retires the old row (SoftDelete) and creates a
// fresh one — this test asserts on the OBSERVABLE guarantee ("exactly one
// active analyzer, with the new number"), not on the row keeping its id.
func TestPM5340UpdateChangesInstallationNumberOnSingleAnalyzer(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140023)
	credInsertDefinition(t, ctx, pool, "pm5340", "default", nil)
	h := newCredHarness(t, pool, credNow)

	pm5340URL := "http://10.0.0.6:502"
	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderPM5340, Subtype: "default",
		PM5340URL: &pm5340URL, InstallationNumber: ptr("PM-OLD"),
	})
	require.NoError(t, err)

	original, err := h.analyzers.GetByInstallation(ctx, tenant.AdminScope, model.IntegrationProviderPM5340, "default", "PM-OLD")
	require.NoError(t, err)

	_, err = h.svc.Update(ctx, tenant.AdminScope, view.ID, credentials.Input{
		InstallationNumber: ptr("PM-NEW"),
	})
	require.NoError(t, err)

	// Exactly one ACTIVE (default List excludes soft-deleted) pm5340
	// analyzer for this company — not two.
	list, err := h.analyzers.List(ctx, tenant.AdminScope, store.AnalyzerFilter{Providers: []model.IntegrationProvider{model.IntegrationProviderPM5340}})
	require.NoError(t, err)
	require.Len(t, list, 1, "InstallationNumber change must never leave a second active analyzer around")
	require.Equal(t, "PM-NEW", list[0].InstallationNumber)
	require.True(t, list[0].IsActive)
	require.NotEqual(t, original.ID, list[0].ID, "InstallationNumber is AnalyzerRepository.Update's immutable natural key — it retires the old row and creates a fresh one")

	// The OLD installation number no longer resolves as an active analyzer
	// (its row still exists, soft-deleted).
	_, err = h.analyzers.GetByInstallation(ctx, tenant.AdminScope, model.IntegrationProviderPM5340, "default", "PM-OLD")
	require.ErrorIs(t, err, store.ErrNotFound)
	includingDeleted, err := h.analyzers.List(ctx, tenant.AdminScope, store.AnalyzerFilter{
		Providers: []model.IntegrationProvider{model.IntegrationProviderPM5340}, IncludeDeleted: true,
	})
	require.NoError(t, err)
	require.Len(t, includingDeleted, 2, "the old row is retired, not erased")

	// Idempotent: Update with the SAME (now current) installation number a
	// second time does not create a third row.
	_, err = h.svc.Update(ctx, tenant.AdminScope, view.ID, credentials.Input{InstallationNumber: ptr("PM-NEW")})
	require.NoError(t, err)
	list2, err := h.analyzers.List(ctx, tenant.AdminScope, store.AnalyzerFilter{Providers: []model.IntegrationProvider{model.IntegrationProviderPM5340}})
	require.NoError(t, err)
	require.Len(t, list2, 1)
	require.Equal(t, list[0].ID, list2[0].ID)
}

// TestConfigureAndUpdateRejectInvalidExtraKeyNames is the folded minor's
// mutation proof ("invalid key accepted"): an Input.Extra key not matching
// ^[a-z][a-z0-9_]{0,63}$ must be refused with ErrInvalidExtraKey, on BOTH
// Configure and Update, and must never reach OpenSecret's stored blob.
func TestConfigureAndUpdateRejectInvalidExtraKeyNames(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 140024)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	badKeys := []string{"APP_ID", "app id", "-app_id", "", "app.id", "app-id"}
	for _, k := range badKeys {
		t.Run("Configure rejects "+k, func(t *testing.T) {
			_, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
				Provider: model.IntegrationProviderGridbox, Subtype: "default",
				Extra: map[string]integration.Secret{k: integration.NewSecret([]byte("v"))},
			})
			require.ErrorIs(t, err, credentials.ErrInvalidExtraKey)
		})
	}

	view, err := h.svc.Configure(ctx, tenant.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default",
		Extra: map[string]integration.Secret{"app_id": integration.NewSecret([]byte("ok"))},
	})
	require.NoError(t, err)

	for _, k := range badKeys {
		t.Run("Update rejects "+k, func(t *testing.T) {
			_, err := h.svc.Update(ctx, tenant.AdminScope, view.ID, credentials.Input{
				Extra: map[string]integration.Secret{k: integration.NewSecret([]byte("v"))},
			})
			require.ErrorIs(t, err, credentials.ErrInvalidExtraKey)
		})
	}

	// A good key still works.
	updated, err := h.svc.Update(ctx, tenant.AdminScope, view.ID, credentials.Input{
		Extra: map[string]integration.Secret{"app_key": integration.NewSecret([]byte("ok2"))},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"app_id", "app_key"}, updated.ExtraKeys)
}

// TestUpdateDeleteVerifyDiscoverAreScopedToTenant is the folded minor's
// cross-tenant coverage for Update/Delete/Verify/Discover: tenant B's
// AdminScope against tenant A's credential id gets store.ErrNotFound for
// every one of them, with tenant A's own scope (a positive control) still
// succeeding on the SAME credential afterward.
func TestUpdateDeleteVerifyDiscoverAreScopedToTenant(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 140025)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 140026)
	credInsertDefinition(t, ctx, pool, "gridbox", "default", nil)
	h := newCredHarness(t, pool, credNow)

	secret := integration.NewSecret([]byte("FIXTURE-PW"))
	view, err := h.svc.Configure(ctx, tenantA.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "default", Secret: &secret,
	})
	require.NoError(t, err)

	_, err = h.svc.Update(ctx, tenantB.AdminScope, view.ID, credentials.Input{Username: ptr("hijacked")})
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = h.svc.Verify(ctx, tenantB.AdminScope, view.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = h.svc.Discover(ctx, tenantB.AdminScope, view.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	require.ErrorIs(t, h.svc.Delete(ctx, tenantB.AdminScope, view.ID), store.ErrNotFound)
	// DeleteCredential is scoped SQL (`where id = $1 and company_id = $2`),
	// so a wrong-tenant Delete either affects 0 rows (ErrNotFound) or, if
	// the repository is more permissive, must still not remove the row —
	// the positive control below is what actually proves it survived.

	// Positive control: tenant A itself can still act on the credential —
	// the row was never touched by tenant B's attempts above.
	updated, err := h.svc.Update(ctx, tenantA.AdminScope, view.ID, credentials.Input{Username: ptr("owner-update")})
	require.NoError(t, err)
	require.Equal(t, ptr("owner-update"), updated.Username)

	_, err = h.svc.Verify(ctx, tenantA.AdminScope, view.ID)
	require.NoError(t, err)

	taskID, err := h.svc.Discover(ctx, tenantA.AdminScope, view.ID)
	require.NoError(t, err)
	require.NotEmpty(t, taskID)

	require.NoError(t, h.svc.Delete(ctx, tenantA.AdminScope, view.ID))
}

// TestISolarMethodsAreScopedToTenant is the folded minor's cross-tenant
// coverage for ISolarAuthorizeURL and ISolarAccessToken: tenant B's
// AdminScope against tenant A's isolar credential id gets
// store.ErrNotFound, with tenant A's own scope succeeding as a positive
// control.
func TestISolarMethodsAreScopedToTenant(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 140027)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 140028)
	credInsertDefinition(t, ctx, pool, "isolar", "EU", nil)
	h := newCredHarness(t, pool, credNow)

	view, err := h.svc.Configure(ctx, tenantA.AdminScope, credentials.Input{
		Provider: model.IntegrationProviderISolar, Subtype: "EU",
		Extra: isolarExtraFixture("refresh-1", "access-1"),
	})
	require.NoError(t, err)
	require.NoError(t, h.integrations.RecordVerification(ctx, tenantA.AdminScope, view.ID, credNow, ptr(credNow.Add(2*time.Hour))))

	_, err = h.svc.ISolarAuthorizeURL(ctx, tenantB.AdminScope, view.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = h.svc.ISolarAccessToken(ctx, tenantB.AdminScope, view.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Positive control.
	authorizeURL, err := h.svc.ISolarAuthorizeURL(ctx, tenantA.AdminScope, view.ID)
	require.NoError(t, err)
	require.NotEmpty(t, authorizeURL)

	tok, err := h.svc.ISolarAccessToken(ctx, tenantA.AdminScope, view.ID)
	require.NoError(t, err)
	require.Equal(t, "access-1", tok.Reveal())
}
