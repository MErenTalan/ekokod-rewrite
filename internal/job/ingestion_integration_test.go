//go:build integration

// Package job_test holds Task 17's end-to-end ingestion acceptance suite:
// every F2 acceptance criterion, exercised through a REAL asynq worker
// (job.NewServer/job.Register/job.Client), a real Redis broker and a real
// (isolated-clone) TimescaleDB, with every provider call going through
// fake.NewTLSServer — never a real endpoint.
//
// This file imports internal/store/... (via internal/store/postgres,
// internal/store/postgres/admin and internal/testfixtures), which
// TestTheJobPackageDoesNotImportTheStore (internal/arch/arch_test.go) does
// NOT see: that guard loads internal/job with packages.NeedImports and
// Tests unset — production-package mode only — so a _test.go file's own
// imports never reach it. That is exactly why this file must stay
// "_test.go" and package "job_test" (an external test package): a
// same-package (white-box) test file is still excluded by Tests-unset
// loading, but staying in the conventional "_test.go, external test
// package" shape is what keeps this fact simple to verify by inspection
// rather than by relying on a subtlety of go/packages' loading mode.
package job_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
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
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/MErenTalan/ekokod-rewrite/internal/worker"
)

// ---------------------------------------------------------------------------
// acceptEnv: the shared harness every test in this file builds through.
// ---------------------------------------------------------------------------

const acceptPublicURL = "https://ekokod.example.com"

// acceptConfig loads a full, valid *config.Config via config.Load with a
// map lookup (never config.FromEnv — this never reads the real process
// environment), the same pattern internal/worker/wiring_integration_test.go
// uses. pins becomes cfg.External.PinnedCerts directly (config.Load has no
// env var shaped for a single fake.Server's Pins map, so this assigns the
// struct field after Load, exactly like
// TestWorkerBuildClosesEarlierResourcesOnLateFailure mutates
// cfg.Security.EncryptionKey after Load).
func acceptConfig(t *testing.T, redisCfg config.Redis, pins map[string]string) *config.Config {
	t.Helper()
	env := map[string]string{
		"EKOKOD_PUBLIC_URL":                acceptPublicURL,
		"EKOKOD_DB_URL":                    "postgres://ekokod:ekokod@127.0.0.1:5432/unused", // worker.Build never reads cfg.DB; it takes an already-open pool
		"EKOKOD_REDIS_URL":                 redisCfg.URL,
		"EKOKOD_REDIS_CACHE_DB":            fmt.Sprint(redisCfg.CacheDB),
		"EKOKOD_REDIS_QUEUE_DB":            fmt.Sprint(redisCfg.QueueDB),
		"EKOKOD_ENCRYPTION_KEY":            "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", // 32 bytes
		"EKOKOD_JWT_SIGNING_KEY":           "jwt-signing-key-at-least-32-chars-long!!",
		"EKOKOD_PASSWORD_PEPPER":           "password-pepper-at-least-32-chars-long!!",
		"EKOKOD_DEVICE_FINGERPRINT_SECRET": "device-fingerprint-secret-32-chars-min!!",
		"EKOKOD_STORAGE_ROOT":              t.TempDir(),
		"EKOKOD_EPIAS_USERNAME":            "epias-user",
		"EKOKOD_EPIAS_PASSWORD":            "epias-pass",
		"EKOKOD_ML_API_KEY":                "ml-api-key",
		// 0: every task this suite enqueues carries its own explicit
		// TaskOptions.MaxRetry (never relies on this default) EXCEPT the
		// FetchReadings tasks SyncAnalyzers auto-enqueues internally
		// (ingest/sync.go, job.TaskOptions{MaxRetry: s.opts.MaxRetry}) —
		// zero here keeps TestIngestionEndToEndOneAnalyzerFailureDoesNotAbortTheRun's
		// failing installation's job_runs count deterministic (one failed
		// run per kind, not one plus however many backoff retries land
		// inside this test's own wait window).
		"EKOKOD_JOB_MAX_RETRIES": "0",
	}
	cfg, err := config.Load(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	require.NoError(t, err)
	cfg.External.PinnedCerts = pins
	return cfg
}

// acceptNoVerifier and acceptNoVerifierResolver/acceptNoISolar are
// structurally-required credentials.Deps fields (credentials.New refuses a
// nil Verifiers/ISolar) that this suite never actually calls: every test
// here only calls credentials.Service.Configure (never Verify/the OAuth
// flow), and Configure's own code path (internal/credentials/service.go)
// never touches Deps.Verifiers or Deps.ISolar. Panicking bodies make that
// assumption self-checking: if a future edit to this file ever did call
// Verify/ISolarAuthorizeURL through this harness's *credentials.Service,
// it would fail loudly here instead of silently doing nothing.
type acceptNoVerifier struct{}

func (acceptNoVerifier) Verify(context.Context, integration.Credentials) error {
	panic("accept: Verify called on the acceptEnv credential service — this suite only calls Configure")
}

type acceptNoVerifierResolver struct{}

func (acceptNoVerifierResolver) Verifier(integration.Provider) (credentials.Verifier, error) {
	return acceptNoVerifier{}, nil
}

type acceptNoISolar struct{}

func (acceptNoISolar) AuthorizeURL(integration.Credentials, string) (string, error) {
	panic("accept: AuthorizeURL called — this suite never drives the isolar OAuth flow")
}
func (acceptNoISolar) ExchangeCode(context.Context, integration.Credentials, string, string) (isolar.Token, error) {
	panic("accept: ExchangeCode called — this suite never drives the isolar OAuth flow")
}
func (acceptNoISolar) Refresh(context.Context, integration.Credentials) (isolar.Token, error) {
	panic("accept: Refresh called — this suite never drives the isolar OAuth flow")
}

// acceptEnv is one test's full stack: an isolated database clone, a
// tenant, a real worker.Built (Handlers.Ingestion/Backfill/Prices), a real
// asynq server processing built.Handlers on a real Redis broker, a real
// job.Client to enqueue with, and a *credentials.Service this test drives
// Configure through (never raw SQL — the brief's own rule). credSvc is
// built directly in this file rather than reached through worker.Build
// (which does not expose the *credentials.Service it constructs — only
// Built.Handlers and Built.Close), using its own IntegrationRepository over
// the SAME pool and the SAME cfg.Security.EncryptionKey, so a credential
// this test writes decrypts correctly when the REAL ingestSvc
// (built.Handlers.Ingestion, via worker.Build's own IntegrationRepository)
// reads it back — two *crypto.Cipher instances built from the same key
// bytes are interchangeable for AES-256-GCM.
type acceptEnv struct {
	ctx  context.Context
	pool *pgxpool.Pool
	tn   testfixtures.Tenant
	cfg  *config.Config
	log  *slog.Logger
	logs *acceptLogBuffer // nil unless acceptEnvWithLog built this harness

	built  worker.Built
	server *asynq.Server
	client *job.Client

	credSvc   *credentials.Service
	catalogue *admin.CatalogueRepository

	analyzers *postgres.AnalyzerRepository
	readings  *postgres.ReadingRepository
	cursors   *postgres.CursorRepository
	anomalies *postgres.AnomalyRepository
}

// acceptLogBuffer is a concurrency-safe io.Writer every log line is
// appended to, for TestIngestionSecretsNeverReachRunsCursorsMessagesOrLogs
// to grep. asynq's own server logs on multiple goroutines, so this must be
// safe for concurrent Write calls — bytes.Buffer alone is not.
type acceptLogBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *acceptLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *acceptLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// acceptEnvSetup builds the full harness. seed is testfixtures.NewTenant's
// seed (must be unique across this file's tests so fixture ids never
// collide within one test binary run). pins is the fake.Server.Pins map
// for every provider this test drives through the worker — for a test
// using exactly one fake.NewTLSServer, srv.Pins is enough (every
// NewTLSServer listens on 127.0.0.1 with a distinct port but the SAME pin
// key "127.0.0.1", so a test must never run two fake servers concurrently
// against the same acceptEnv).
func acceptEnvSetup(t *testing.T, seed int64, pins map[string]string) *acceptEnv {
	return acceptEnvSetupFull(t, seed, pins, testfixtures.DiscardLogger(), nil, nil)
}

// acceptEnvSetupTight is acceptEnvSetup with a shortened
// Ingest.InitialLookback (used only by the PM5340 cursor-resume test, so a
// first-ever cursor-driven fetch splits into a small, predictable number of
// weekly chunks instead of the default 30-day lookback's ~5).
func acceptEnvSetupTight(t *testing.T, seed int64, pins map[string]string, lookback time.Duration) *acceptEnv {
	return acceptEnvSetupFull(t, seed, pins, testfixtures.DiscardLogger(), nil, func(cfg *config.Config) {
		cfg.Ingest.InitialLookback = lookback
	})
}

// acceptEnvSetupWithLog is acceptEnvSetup plus an injectable logger — used
// by TestIngestionSecretsNeverReachRunsCursorsMessagesOrLogs, the one test
// that must inspect what the worker actually logged.
func acceptEnvSetupWithLog(t *testing.T, seed int64, pins map[string]string, log *slog.Logger, logs *acceptLogBuffer, mutateCfg func(*config.Config)) *acceptEnv {
	return acceptEnvSetupFull(t, seed, pins, log, logs, mutateCfg)
}

// acceptEnvSetupWithEPIAS is acceptEnvSetup with cfg.External.EPIASCASURL/
// EPIASBaseURL pointed at a fake server BEFORE worker.Build runs (M3, final
// review B: config.External now carries these fields — see
// internal/platform/config/load.go's httpsURL — so worker.Build's own
// epias.New wiring can be driven end to end against fake.NewTLSServer, with
// no Handlers.Prices substitution at the test layer).
func acceptEnvSetupWithEPIAS(t *testing.T, seed int64, pins map[string]string, casURL, baseURL string) *acceptEnv {
	return acceptEnvSetupFull(t, seed, pins, testfixtures.DiscardLogger(), nil, func(cfg *config.Config) {
		cfg.External.EPIASCASURL = casURL
		cfg.External.EPIASBaseURL = baseURL
	})
}

// acceptEnvSetupFull is every other acceptEnvSetup* variant's shared body.
func acceptEnvSetupFull(t *testing.T, seed int64, pins map[string]string, log *slog.Logger, logs *acceptLogBuffer, mutateCfg func(*config.Config)) *acceptEnv {
	t.Helper()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, seed)
	redisCfg := testfixtures.RedisConfig(t)
	cfg := acceptConfig(t, redisCfg, pins)
	if mutateCfg != nil {
		mutateCfg(cfg)
	}

	built, err := worker.Build(ctx, cfg, pool, log)
	require.NoError(t, err)
	t.Cleanup(built.Close)

	client, err := job.NewClient(cfg.Redis)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	server, mux, err := job.NewServer(cfg.Redis, cfg.Worker, log, nil)
	require.NoError(t, err)
	job.Register(mux, built.Handlers)
	require.NoError(t, server.Start(mux))
	t.Cleanup(func() {
		server.Stop()
		server.Shutdown()
	})

	cipher, err := crypto.NewCipher(cfg.Security.EncryptionKey)
	require.NoError(t, err)
	analyzers := postgres.NewAnalyzerRepository(pool)
	credSvc, err := credentials.New(credentials.Deps{
		Integrations: postgres.NewIntegrationRepository(pool, cipher),
		Analyzers:    analyzers,
		Buildings:    postgres.NewBuildingRepository(pool),
		Verifiers:    acceptNoVerifierResolver{},
		ISolar:       acceptNoISolar{},
		Enqueuer:     client,
		Locker:       lock.NewMemory(nil),
		Nonces:       lock.NewMemory(nil),
		Clock:        clock.System(),
		StateKey:     worker.StateKey(cfg.Security.JWTSigningKey),
		RedirectURI:  worker.RedirectURI(cfg),
		MaxRetry:     cfg.Worker.MaxRetries,
	})
	require.NoError(t, err)

	return &acceptEnv{
		ctx: ctx, pool: pool, tn: tn, cfg: cfg, log: log, logs: logs,
		built: built, server: server, client: client,
		credSvc: credSvc, catalogue: admin.NewCatalogueRepository(pool),
		analyzers: analyzers, readings: postgres.NewReadingRepository(pool),
		cursors: postgres.NewCursorRepository(pool), anomalies: postgres.NewAnomalyRepository(pool),
	}
}

// ---------------------------------------------------------------------------
// Generic helpers: definitions, credentials, analyzers, enqueue, polling.
// ---------------------------------------------------------------------------

// acceptDefinition upserts one integration_definitions row through the
// real admin.CatalogueRepository (never raw SQL) and returns its id.
func acceptDefinition(t *testing.T, e *acceptEnv, provider model.IntegrationProvider, subtype string, endpoints map[string]string) uuid.UUID {
	t.Helper()
	body, err := json.Marshal(endpoints)
	require.NoError(t, err)
	def := model.IntegrationDefinition{ID: uuid.New(), Provider: provider, Subtype: subtype, Endpoints: body}
	n, err := e.catalogue.UpsertIntegrationDefinitions(e.ctx, []model.IntegrationDefinition{def})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	return def.ID
}

// acceptConfigure drives credentials.Service.Configure (never raw SQL) and
// returns the new credential's id.
func acceptConfigure(t *testing.T, e *acceptEnv, in credentials.Input) uuid.UUID {
	t.Helper()
	view, err := e.credSvc.Configure(e.ctx, e.tn.Scope, in)
	require.NoError(t, err)
	return view.ID
}

// acceptAnalyzer creates one active analyzer directly through the real
// AnalyzerRepository (a repository call, like Task 10's own
// ingestTestNewActiveAnalyzer helper — not raw SQL; the brief's "never raw
// SQL" rule is stated specifically for the CREDENTIAL, which this suite
// always configures through credentials.Service.Configure).
func acceptAnalyzer(t *testing.T, e *acceptEnv, provider model.IntegrationProvider, subtype, installation string, multiplier decimal.Decimal) model.Analyzer {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a, err := e.analyzers.Create(e.ctx, e.tn.Scope, model.Analyzer{
		CompanyID: e.tn.Company.ID, BuildingID: &e.tn.Buildings[0].ID,
		Provider: provider, ProviderSubtype: subtype, InstallationNumber: installation,
		MeterMultiplier: multiplier, IsActive: true, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	return a
}

// acceptEnqueueFetch enqueues one integration.fetch_readings task and
// returns its *asynq.TaskInfo.
func acceptEnqueueFetch(t *testing.T, e *acceptEnv, credentialID, analyzerID uuid.UUID, kind model.ReadingKind, window *job.Window, maxRetry int) *asynq.TaskInfo {
	t.Helper()
	task, err := job.NewFetchReadingsTask(job.FetchReadingsPayload{
		CompanyID: e.tn.Company.ID, CredentialID: credentialID, AnalyzerID: analyzerID, Kind: kind, Window: window,
	}, job.TaskOptions{MaxRetry: maxRetry})
	require.NoError(t, err)
	info, err := e.client.Enqueue(e.ctx, task)
	require.NoError(t, err)
	return info
}

// acceptJobRunRow is one job_runs row, read directly (the brief's Step 2
// setup explicitly says "wait for completion with require.Eventually
// polling job_runs").
type acceptJobRunRow struct {
	Status                     string
	Processed, Skipped, Failed int32
	Error                      *string
}

// acceptWaitRuns polls job_runs for jobType rows whose jsonb scope carries
// scopeKey=scopeVal (analyzer_id for fetch_readings, credential_id for
// sync_analyzers/backfill), bound 60s, tick 200ms — never time.Sleep — and
// returns once at least wantAtLeast FINISHED (status != "running") rows
// exist, newest first. Per the brief: before believing a failure, check
// for the container-readiness trap; that check is done by the caller
// (every failing require.Eventually in this file's tests names the
// symptom directly, so a `wait until ready`/55006 substring is visible in
// the failure output without extra plumbing here).
func acceptWaitRuns(t *testing.T, e *acceptEnv, jobType, scopeKey string, scopeVal uuid.UUID, wantAtLeast int) []acceptJobRunRow {
	t.Helper()
	var rows []acceptJobRunRow
	require.Eventually(t, func() bool {
		q := `select status, processed, skipped, failed, error from job_runs
			where job_type = $1 and scope->>$2 = $3 and status <> 'running'
			order by started_at desc`
		rs, err := e.pool.Query(e.ctx, q, jobType, scopeKey, scopeVal.String())
		if err != nil {
			return false
		}
		defer rs.Close()
		var out []acceptJobRunRow
		for rs.Next() {
			var r acceptJobRunRow
			if err := rs.Scan(&r.Status, &r.Processed, &r.Skipped, &r.Failed, &r.Error); err != nil {
				return false
			}
			out = append(out, r)
		}
		if rs.Err() != nil {
			return false
		}
		if len(out) < wantAtLeast {
			return false
		}
		rows = out
		return true
	}, 60*time.Second, 200*time.Millisecond, "timed out waiting for %d %s job_runs row(s) scoped to %s=%s", wantAtLeast, jobType, scopeKey, scopeVal)
	return rows
}

// acceptWaitRunsLong is acceptWaitRuns with a longer bound, for the one
// test (PM5340 interrupted-fetch) whose second attempt only runs after
// asynq's own RetryDelayFunc backoff (job/retry.go: 30s base, jittered,
// doubling) — a real wall-clock wait this suite cannot shorten without
// changing job.RetryDelay, which is Task 1's owned behaviour, not a T17
// wiring gap.
func acceptWaitRunsLong(t *testing.T, e *acceptEnv, jobType, scopeKey string, scopeVal uuid.UUID, wantAtLeast int) []acceptJobRunRow {
	t.Helper()
	var rows []acceptJobRunRow
	require.Eventually(t, func() bool {
		q := `select status, processed, skipped, failed, error from job_runs
			where job_type = $1 and scope->>$2 = $3 and status <> 'running'
			order by started_at desc`
		rs, err := e.pool.Query(e.ctx, q, jobType, scopeKey, scopeVal.String())
		if err != nil {
			return false
		}
		defer rs.Close()
		var out []acceptJobRunRow
		for rs.Next() {
			var r acceptJobRunRow
			if err := rs.Scan(&r.Status, &r.Processed, &r.Skipped, &r.Failed, &r.Error); err != nil {
				return false
			}
			out = append(out, r)
		}
		if rs.Err() != nil {
			return false
		}
		if len(out) < wantAtLeast {
			return false
		}
		rows = out
		return true
	}, 150*time.Second, 200*time.Millisecond, "timed out waiting for %d %s job_runs row(s) scoped to %s=%s (allowing for asynq's retry backoff)", wantAtLeast, jobType, scopeKey, scopeVal)
	return rows
}

// acceptRows reads every stored reading for analyzerID/kind over a window
// wide enough to cover every fixed timestamp this file's tests use.
func acceptRows(t *testing.T, e *acceptEnv, analyzerID uuid.UUID, kind model.ReadingKind) []model.MeterReading {
	t.Helper()
	rows, err := e.readings.Range(e.ctx, e.tn.Scope, analyzerID,
		store.TimeRange{From: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)}, kind)
	require.NoError(t, err)
	return rows
}

func acceptSecret(s string) *integration.Secret {
	sec := integration.NewSecret([]byte(s))
	return &sec
}

func acceptPtr[T any](v T) *T { return &v }

// ---------------------------------------------------------------------------
// GridBox fixtures/routes — endpoint keys and paths exactly as
// internal/integration/gridbox/source_test.go's own gridboxEndpoints/
// gridboxBaseRoutes.
// ---------------------------------------------------------------------------

func acceptGridboxEndpoints(srv *fake.Server) map[string]string {
	base := srv.URL + "/gridbox"
	return map[string]string{
		"token":             base + "/token",
		"last_success_date": base + "/last-success-date?wiringNo={wiringNo}",
		"last_endex":        base + "/last-endex?wiringNo={wiringNo}",
		"load_profiles":     base + "/load-profiles?wiringNo={wiringNo}&startDate={startDate}&endDate={endDate}",
		"endexes":           base + "/endexes?wiringNo={wiringNo}&startDate={startDate}&endDate={endDate}&isBilling={isBilling}",
		"energy_values":     base + "/energy-values?wiringNo={wiringNo}&startDate={startDate}&endDate={endDate}",
	}
}

func acceptGridboxBaseRoutes(t *testing.T) []fake.Route {
	t.Helper()
	return []fake.Route{
		{Method: http.MethodPost, Path: "/gridbox/token", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_token.json"))},
		{Method: http.MethodGet, Path: "/gridbox/last-success-date", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_success_date.json"))},
	}
}

// acceptGridboxAuthFailureResponder answers 400 invalid_grant and echoes
// the submitted form body back in the response — the ONLY channel through
// which a submitted plaintext password could leak back into a recorded
// HTTP response body, matching
// internal/integration/gridbox/source_test.go's own
// gridboxAuthFailureResponder used by TestGridBoxErrorsCarryNoCredential.
func acceptGridboxAuthFailureResponder(t *testing.T) fake.Responder {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "FIXTURE-invalid-credentials",
			"echo":              string(raw),
		})
	}
}

func acceptIstanbulDay(year int, month time.Month, day int) (from, to time.Time) {
	loc := time.FixedZone("Europe/Istanbul", 3*60*60) // Turkey: fixed UTC+3, no DST, since 2016
	from = time.Date(year, month, day, 0, 0, 0, 0, loc).UTC()
	to = time.Date(year, month, day+1, 0, 0, 0, 0, loc).UTC()
	return from, to
}

// ---------------------------------------------------------------------------
// OSOS fixtures/routes — endpoint keys exactly as
// internal/integration/osos/source_test.go's own ososTestCreds.
// ---------------------------------------------------------------------------

func acceptOSOSEndpoints(srv *fake.Server) map[string]string {
	return map[string]string{
		"authentication": srv.URL + "/osos/auth?u={username_or_email}&p={secret_password}",
		"analyzers_list": srv.URL + "/osos/analyzers?t={secret_token}",
		"energy_values":  srv.URL + "/osos/energy?t={secret_token}&start={start_date}&end={end_date}&inst={installationNumber}&type={dataType}",
		"hourly_values":  srv.URL + "/osos/hourly?t={secret_token}&month={meter_month}&from={from_date}&inst={installationNumber}",
	}
}

func acceptOSOSAuthRoute(token string) fake.Route {
	return fake.Route{Method: http.MethodPost, Path: "/osos/auth", Respond: fake.JSON(http.StatusOK, []byte(`{"access_token":"`+token+`"}`))}
}

// acceptOSOSDiscoverRoute returns an analyzers_list response listing every
// given installation number.
//
//nolint:misspell // OSOS's own field spelling ("instalation_list"/"instalationNumber"), not an English typo — matches internal/integration/osos/wire.go.
func acceptOSOSDiscoverRoute(installations ...string) fake.Route {
	var rows []string
	for _, inst := range installations {
		rows = append(rows, fmt.Sprintf(`{"instalationNumber":%q,"customerName":"Fixture","meterMultiplier":"1"}`, inst))
	}
	body := []byte(`{"instalation_list":[` + strings.Join(rows, ",") + `]}`)
	return fake.Route{Method: http.MethodGet, Path: "/osos/analyzers", Respond: fake.JSON(http.StatusOK, body)}
}

// ---------------------------------------------------------------------------
// ARIL fixtures/routes — endpoint keys exactly as
// internal/integration/aril/source_test.go's own arilEndpoints.
// ---------------------------------------------------------------------------

func acceptARILEndpoints(srv *fake.Server) map[string]string {
	base := srv.URL + "/aril"
	return map[string]string{
		"authentication":       base + "/authentication",
		"analyzers_list":       base + "/analyzers-list",
		"owner_consumptions":   base + "/owner-consumptions",
		"current_endexes":      base + "/current-endexes",
		"end_of_month_endexes": base + "/end-of-month-endexes",
	}
}

func acceptARILAuthRoute() fake.Route {
	return fake.Route{Method: http.MethodPost, Path: "/aril/authentication", Respond: fake.JSON(http.StatusOK, []byte(`{"access_token":"FIXTURE-aril-token"}`))}
}

// ---------------------------------------------------------------------------
// PM5340 — no Endpoints map; BaseURL only (matches
// internal/integration/pm5340/source_test.go's own pm5340TestCreds).
// ---------------------------------------------------------------------------

// acceptPM5340Envelope builds one readings-endpoint response page.
func acceptPM5340Envelope(items []string) []byte {
	return []byte(fmt.Sprintf(`{"items":[%s],"limit":500,"cursorNext":null,"hasMore":false,"total":%d,"sort":"asc","filters":{}}`,
		strings.Join(items, ","), len(items)))
}

// acceptPM5340Row builds one item row. ts is formatted with the fixed
// +03:00 offset PM5340's own fixtures use (Turkey: no DST). currentGenKw
// is the wire field's own unit — INSTANTANEOUS kW, not kWh — because
// internal/integration/pm5340/mapping.go's mapRow converts it via
// normalize.KWFor15MinToKWh (x0.25, a 15-minute interval): a caller that
// wants a specific model.MeterReading.IntervalGenerationKwh must pass 4x
// that value here.
func acceptPM5340Row(ts time.Time, activeImportKwh, currentGenKw string) string {
	local := ts.In(time.FixedZone("Europe/Istanbul", 3*60*60))
	return fmt.Sprintf(`{"meterDate":%q,"activeImport_kWh":%s,"inductive_kvarh":0,"capacitive_kvarh":0,"dmdKwPeak_kW":0,"currentGeneration":%s,"deviceId":"FIXTURE","deviceIp":"127.0.0.1"}`,
		local.Format("2006-01-02T15:04:05-07:00"), activeImportKwh, currentGenKw)
}

// ---------------------------------------------------------------------------
// EPİAŞ — CASURL/BaseURL are epias.Options fields, not config-driven
// (config.External has no EPIAS CAS/base URL field — see the "Wiring gap"
// note on TestEPIASSyncThroughTheWorkerIsIdempotent below). Path constants
// copied from internal/integration/epias/client_test.go.
// ---------------------------------------------------------------------------

const (
	acceptEPIASCasPath = "/cas/v1/tickets"
	acceptEPIASMcpPath = "/electricity-service/v1/markets/dam/data/mcp"
	acceptEPIASYekPath = "/electricity-service/v1/renewables/data/unit-cost"
)

func acceptEPIASCasRoute(ticket string) fake.Route {
	return fake.Route{Method: http.MethodPost, Path: acceptEPIASCasPath, Respond: func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://giris.epias.com.tr/cas/v1/tickets/"+ticket)
		w.WriteHeader(http.StatusCreated)
	}}
}

// ---------------------------------------------------------------------------
// Test 1 (F2 acceptance criterion 2): fetching the same window twice
// produces no duplicate rows — through the real worker.
// ---------------------------------------------------------------------------

func TestIngestionEndToEndSameWindowTwiceProducesNoDuplicates(t *testing.T) {
	srv := fake.NewTLSServer(t, append(acceptGridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_success.json"))},
	)...)
	e := acceptEnvSetup(t, 17001, srv.Pins)

	acceptDefinition(t, e, model.IntegrationProviderGridbox, "T17SameWindow", acceptGridboxEndpoints(srv))
	credID := acceptConfigure(t, e, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "T17SameWindow",
		Username: acceptPtr("fixture-user"), Secret: acceptSecret("fixture-pass"),
	})
	analyzer := acceptAnalyzer(t, e, model.IntegrationProviderGridbox, "T17SameWindow", "WIRING-SW-1", decimal.NewFromInt(1))

	from, to := acceptIstanbulDay(2026, 9, 1)
	window := &job.Window{From: from, To: to}

	acceptEnqueueFetch(t, e, credID, analyzer.ID, model.ReadingKindLoadProfile, window, 0)
	run1 := acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", analyzer.ID, 1)
	require.Equal(t, "success", run1[0].Status)

	rowsAfterFirst := acceptRows(t, e, analyzer.ID, model.ReadingKindLoadProfile)
	require.Len(t, rowsAfterFirst, 2, "gridbox_success.json fixture has 2 rows")

	// The SECOND fetch of the identical window is driven by calling
	// built.Handlers.Ingestion.FetchReadings directly — the exact same
	// *ingest.Service worker.Build wired and the real asynq handler above
	// called — rather than a second job.Client.Enqueue. An explicit,
	// non-nil job.Window gets a DETERMINISTIC asynq task ID with a 30-day
	// Retention (internal/job/integration.go's NewFetchReadingsTask,
	// R16/backfill semantics: re-enqueuing the identical window must be
	// REFUSED at the queue layer, precisely so an accidental duplicate
	// enqueue can never double-run a backfill chunk) — proven directly:
	// re-enqueuing here returns "task ID conflicts with another task",
	// which is asynq's OWN correct, already-relied-upon guarantee, not
	// this criterion's concern. This acceptance criterion is about the
	// PIPELINE's own upsert behaviour when the same window's data really
	// is persisted twice (the shape a cursor-driven re-run after a
	// clock/cursor anomaly could produce), which calling FetchReadings
	// again — through the same real service, same real credentials, same
	// real adapter, same real fake TLS server — proves without needing a
	// second distinct queue entry.
	dupErr := e.built.Handlers.Ingestion.FetchReadings(e.ctx, job.FetchReadingsPayload{
		CompanyID: e.tn.Company.ID, CredentialID: credID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window,
	})
	require.NoError(t, dupErr)

	rowsAfterSecond := acceptRows(t, e, analyzer.ID, model.ReadingKindLoadProfile)
	require.Len(t, rowsAfterSecond, 2, "re-fetching the same window must upsert, never duplicate")

	// The queue-layer guarantee itself, confirmed directly (documented
	// above): a second Enqueue of the byte-identical explicit window is
	// refused, never silently creating a second job_runs row.
	_, enqErr := e.client.Enqueue(e.ctx, func() *asynq.Task {
		task, terr := job.NewFetchReadingsTask(job.FetchReadingsPayload{
			CompanyID: e.tn.Company.ID, CredentialID: credID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window,
		}, job.TaskOptions{MaxRetry: 0})
		require.NoError(t, terr)
		return task
	}())
	require.Error(t, enqErr, "re-enqueuing the identical explicit-window task must be refused by asynq's own deterministic-ID uniqueness")
}

// ---------------------------------------------------------------------------
// Test 2 (F2 acceptance criterion 3): interrupting a fetch and re-running
// resumes and loses nothing — PM5340, driven by asynq's own task-level
// retry (not a second manual enqueue).
//
// DESIGN NOTE — why this is a single-chunk scenario, not a
// two-chunk/cursor-resume-mid-run one. An earlier version of this test
// tried to force PM5340's real adapter through TWO pipeline chunks (one
// succeeding, the next failing), keyed by matching each request's exact
// "start" query value. That does not work: ingestion_cursors.last_ts is
// set to the PERSISTED ROW's OWN Ts (fetch.go: Cursors.RecordSuccess(...,
// kindMaxTs, ...), not the chunk's nominal boundary — so a retried attempt
// resuming "from the cursor" starts a FRESH chunk anchored at that ROW'S
// timestamp, whose Europe/Istanbul-local-midnight-aligned boundaries
// (normalize.Chunk) do not equal the ORIGINAL second chunk's boundaries
// (which were anchored at chunk-one's END, not at any row's Ts) unless the
// row itself happens to land exactly on that boundary — a coincidence real
// wall-clock "now" (worker.Build hardcodes clock.System(), no fake-clock
// seam) cannot be made to reproduce deterministically. Rather than
// fabricate that alignment, this test uses Ingest.InitialLookback SHORTER
// than PM5340's 7-day MaxWindow, so a first-ever cursor-driven fetch is
// GUARANTEED exactly one chunk/one HTTP request per attempt — removing the
// chunk-boundary question entirely. The interruption this test proves is
// at the TASK level: attempt 1's one request exhausts httpx's in-client
// retry budget (3 attempts, all 500) and persists nothing (R19: a run that
// fetches nothing is not itself a partial success — status is "failed"
// here, 0 rows); asynq retries the whole task once more; attempt 2's
// request succeeds and persists exactly one row, with none lost or
// duplicated by the interruption. Task 10's own already-merged
// TestIngestionInterruptedFetchResumesFromCursor
// (internal/ingest/ingest_integration_test.go) is what proves the
// multi-chunk cursor-resume mechanics in detail, against a fake adapter it
// has full control over — this test's job is proving the SAME task-level
// contract holds through the real PM5340 adapter and a real asynq retry.
//
// I1 (final review B): a single-phase version of this test — enqueue once,
// fail 3 times, retry, succeed — CANNOT catch a broken resume, because
// attempt 1 never has a stored cursor to ignore in the first place: with no
// prior cursor, resolveFetchWindow's ONLY path is the "no cursor yet"
// InitialLookback fallback, identically whether or not the cursor branch
// is broken. Ignoring the live cursor (mutating resolveFetchWindow's
// `cur.LastTs != nil` case to fall back to `now.Add(-InitialLookback)`
// instead of `*cur.LastTs`) left this test PASSING. The test is therefore
// now two phases: phase 1 is a plain, uninterrupted fetch that establishes
// a real cursor; phase 2 is the interrupted-then-retried fetch from the
// original design above, and its own resumed request is asserted to carry
// start == phase 1's cursor — the one assertion that actually exercises the
// cursor-resume branch, not just "a retried task can still succeed".
// ---------------------------------------------------------------------------

// acceptPM5340FailNThenSucceed answers the first n requests with 500, then
// every request after that with one row at the midpoint of THAT request's
// own [start,end) — computed from the request itself (never a literal
// timestamp), so it is always inside whatever window real wall-clock "now"
// produces.
type acceptPM5340FailNThenSucceed struct {
	remaining int32
}

func (r *acceptPM5340FailNThenSucceed) route() fake.Route {
	return fake.Route{
		Method: http.MethodGet,
		Path:   "/api/v1/readings",
		Respond: func(w http.ResponseWriter, req *http.Request) {
			if atomic.AddInt32(&r.remaining, -1) >= 0 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"FIXTURE-boom"}`))
				return
			}
			start := t2Time(req.URL.Query().Get("start"))
			end := t2Time(req.URL.Query().Get("end"))
			ts := acceptPM5340Midpoint(start, end)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(acceptPM5340Envelope([]string{acceptPM5340Row(ts, "1000.000", "4.000")}))
		},
	}
}

func t2Time(s string) time.Time {
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("accept: bad time literal " + s + ": " + err.Error())
	}
	return tm
}

func acceptPM5340Midpoint(from, to time.Time) time.Time {
	return from.Add(to.Sub(from) / 2)
}

func TestIngestionEndToEndInterruptedFetchResumesFromCursor(t *testing.T) {
	router := &acceptPM5340FailNThenSucceed{} // remaining 0: phase 1's one request succeeds outright
	srv := fake.NewTLSServer(t, router.route())
	// 20h: strictly under PM5340's 7-day MaxWindow, so a first-ever
	// cursor-driven fetch ([now-lookback, now)) is always exactly ONE
	// chunk/one HTTP request per asynq attempt — see the design note above.
	e := acceptEnvSetupTight(t, 17002, srv.Pins, 20*time.Hour)

	acceptDefinition(t, e, model.IntegrationProviderPM5340, "T17Resume", map[string]string{})
	credID := acceptConfigure(t, e, credentials.Input{
		Provider: model.IntegrationProviderPM5340, Subtype: "T17Resume", PM5340URL: acceptPtr(srv.URL),
	})
	analyzer := acceptAnalyzer(t, e, model.IntegrationProviderPM5340, "T17Resume", "PM-RESUME-1", decimal.NewFromInt(1))

	// Phase 1: a plain, uninterrupted, nil-Window (cursor-driven) fetch
	// establishes a REAL cursor from a persisted row — see the design note
	// above for why this phase must exist at all.
	acceptEnqueueFetch(t, e, credID, analyzer.ID, model.ReadingKindLoadProfile, nil, 1)
	phase1 := acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", analyzer.ID, 1)
	require.Equal(t, "success", phase1[0].Status)
	require.EqualValues(t, 1, phase1[0].Processed)

	cur, err := e.cursors.Get(e.ctx, e.tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.NotNil(t, cur.LastTs, "phase 1 must advance the cursor to its persisted row's own Ts")
	phase1Cursor := *cur.LastTs

	// Phase 2: fail 3 times (httpx's own default MaxAttempts, all 500) then
	// succeed. Nil Window: cursor-driven, and a nil Window has no
	// deterministic task id (job.NewFetchReadingsTask), so re-enqueuing the
	// same analyzer/kind here is allowed — unlike Test 1 above's identical
	// EXPLICIT window, which asynq's own deterministic id refuses to
	// re-enqueue. Attempt 1's one request exhausts httpx's in-client retry
	// budget and persists nothing (R19: a run that fetches nothing is
	// "failed", not a partial success); asynq retries the whole task once
	// more (job/retry.go's own backoff, hence acceptWaitRunsLong's longer
	// bound); attempt 2's request must resume from phase1Cursor, not
	// restart from InitialLookback, and succeeds.
	atomic.StoreInt32(&router.remaining, 3)
	acceptEnqueueFetch(t, e, credID, analyzer.ID, model.ReadingKindLoadProfile, nil, 1)

	// 3 finished runs total across both phases, newest first: phase 2's
	// retried success, phase 2's exhausted-retries failure, phase 1's
	// success.
	runs := acceptWaitRunsLong(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", analyzer.ID, 3)
	require.Equal(t, "success", runs[0].Status, "phase 2's retried attempt must complete")
	require.EqualValues(t, 1, runs[0].Processed)
	require.Equal(t, "failed", runs[1].Status, "phase 2's first attempt exhausted its retries before persisting anything")
	require.EqualValues(t, 0, runs[1].Processed)

	// I1: the actual resume assertion. The retried attempt's own (only)
	// request must carry start == phase1Cursor — proving the SECOND fetch
	// resumed exactly where the FIRST left off, rather than merely
	// succeeding eventually. Mutating resolveFetchWindow to ignore
	// cur.LastTs (falling back to now.Add(-InitialLookback) instead) makes
	// this assertion FAIL while every other assertion in this test still
	// passes.
	reqs := srv.Requests()
	require.NotEmpty(t, reqs)
	last := reqs[len(reqs)-1]
	q, perr := url.ParseQuery(last.RawQuery)
	require.NoError(t, perr)
	require.Equal(t, phase1Cursor.UTC().Format(time.RFC3339), q.Get("start"),
		"phase 2's resumed request must start exactly at phase 1's cursor, not restart from InitialLookback")

	rows := acceptRows(t, e, analyzer.ID, model.ReadingKindLoadProfile)
	require.Len(t, rows, 2, "phase 1's row plus phase 2's row: the interruption lost nothing and the retry did not duplicate anything")
	require.False(t, rows[0].Ts.Equal(rows[1].Ts), "phase 1 and phase 2 must persist two DISTINCT rows")
}

// ---------------------------------------------------------------------------
// Test 3 (F2 acceptance criterion 4): one analyzer failing does not abort
// the run; job_runs records processed/skipped/failed and the reason.
// OSOS: three discovered installations, one whose energy_values call
// always 500s.
// ---------------------------------------------------------------------------

// TestIngestionEndToEndOneAnalyzerFailureDoesNotAbortTheRun pre-creates the
// three analyzers ACTIVE, not left for SyncAnalyzers to discover fresh:
// ingest/sync.go's upsertMeteringPoint creates a genuinely NEW analyzer
// INACTIVE (R27 — an operator must activate a freshly discovered point
// before it is scheduled), so a discovery-only setup would make
// SyncAnalyzers' own "processed" (which counts ENQUEUED fetch_readings
// tasks for already-ACTIVE analyzers of this subtype, not "analyzers
// synced" — ingest/sync.go's second loop) stay zero. Pre-creating all
// three ACTIVE makes SyncAnalyzers's discovery phase UPDATE (never
// re-activate) each one, matching Task 10's own
// TestIngestionOneAnalyzerFailureDoesNotAbortTheRun "part A" setup.
//
// OSOS.Kinds() advertises three kinds (load_profile, daily, reset), and
// SyncAnalyzers enqueues one integration.fetch_readings task per (active
// analyzer x kind) it lists — so three analyzers means Processed == 9, not
// 3, and FAIL-2's single always-500 energy_values route fails all three of
// its kinds' fetches (the same endpoint URL, differentiated only by the
// dataType query parameter, never by kind-specific path).
func TestIngestionEndToEndOneAnalyzerFailureDoesNotAbortTheRun(t *testing.T) {
	const (
		okInstall1  = "OK-1"
		failInstall = "FAIL-2"
		okInstall3  = "OK-3"
	)
	fail500 := fake.Route{
		Method: http.MethodGet, Path: "/osos/energy",
		Match: func(r *http.Request) bool { return r.URL.Query().Get("inst") == failInstall },
		Respond: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"FIXTURE-upstream-error"}`))
		},
	}
	// SyncAnalyzers enqueues its fetch tasks with a NIL job.Window
	// (cursor-driven) — resolveFetchWindow then uses [now-InitialLookback,
	// now) with real wall-clock "now" (worker.Build hardcodes
	// clock.System()), so a statically-dated fixture like osos_success.json
	// (August 2026) would land outside that window on some runs and inside
	// it on others depending on the real calendar date. This body's one
	// row is dated "yesterday" relative to whenever this test actually
	// runs, so it is always inside any InitialLookback-sized window.
	recentMeterDate := time.Now().UTC().Add(-24 * time.Hour).Format("02/01/2006 15:04:05")
	okBody := []byte(`{"energy":[{"meter_date":"` + recentMeterDate + `","meter_serial_no":"SN0001","t_top_kWh":"1000,500","t_ri_kVarh":"50,250","t_rc_kVarh":"10,000","t_t1_kWh":"400,000","t_t2_kWh":"300,000","t_t3_kWh":"300,000","t_p_kW":"25,000","u_top_kWh":"5,000","u_ri_kVarh":"1,000","u_rc_kVarh":"0,500","u_u1_kWh":"2,000","u_u2_kWh":"2,000","u_u3_kWh":"1,000","u_p_kW":"3,000"}]}`)
	ok200 := fake.Route{Method: http.MethodGet, Path: "/osos/energy", Respond: fake.JSON(http.StatusOK, okBody)}
	hourly200 := fake.Route{Method: http.MethodGet, Path: "/osos/hourly", Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "osos", "osos_hourly_values.json"))}

	srv := fake.NewTLSServer(t,
		acceptOSOSAuthRoute("FIXTURE-token-0003"),
		acceptOSOSDiscoverRoute(okInstall1, failInstall, okInstall3),
		fail500, ok200, hourly200,
	)
	e := acceptEnvSetup(t, 17003, srv.Pins)

	acceptDefinition(t, e, model.IntegrationProviderOSOS, "T17OneFail", acceptOSOSEndpoints(srv))
	credID := acceptConfigure(t, e, credentials.Input{
		Provider: model.IntegrationProviderOSOS, Subtype: "T17OneFail",
		Username: acceptPtr("fixture-user"), Secret: acceptSecret("fixture-pass"),
	})

	ok1 := acceptAnalyzer(t, e, model.IntegrationProviderOSOS, "T17OneFail", okInstall1, decimal.NewFromInt(1))
	fail2 := acceptAnalyzer(t, e, model.IntegrationProviderOSOS, "T17OneFail", failInstall, decimal.NewFromInt(1))
	ok3 := acceptAnalyzer(t, e, model.IntegrationProviderOSOS, "T17OneFail", okInstall3, decimal.NewFromInt(1))

	syncTask, err := job.NewSyncAnalyzersTask(job.SyncAnalyzersPayload{CompanyID: e.tn.Company.ID, CredentialID: credID}, job.TaskOptions{MaxRetry: 0})
	require.NoError(t, err)
	_, err = e.client.Enqueue(e.ctx, syncTask)
	require.NoError(t, err)

	syncRuns := acceptWaitRuns(t, e, job.TypeIntegrationSyncAnalyzers, "credential_id", credID, 1)
	require.Equal(t, "success", syncRuns[0].Status)
	require.EqualValues(t, 9, syncRuns[0].Processed, "3 active analyzers x 3 OSOS kinds (load_profile, daily, reset) fetch tasks enqueued")

	// SyncAnalyzers itself enqueued every fetch_readings task above (nil
	// Window, cursor-driven) — nothing more to enqueue here, only to wait
	// for the worker to finish processing what it already queued.
	runsOK1 := acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", ok1.ID, 3)
	runsFail2 := acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", fail2.ID, 3)
	runsOK3 := acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", ok3.ID, 3)

	for _, r := range runsOK1 {
		require.Equal(t, "success", r.Status)
	}
	for _, r := range runsOK3 {
		require.Equal(t, "success", r.Status)
	}
	for _, r := range runsFail2 {
		require.Equal(t, "failed", r.Status)
		require.NotNil(t, r.Error, "the failed run's job_runs row must record the reason")
	}

	require.Len(t, acceptRows(t, e, ok1.ID, model.ReadingKindLoadProfile), 1)
	require.Len(t, acceptRows(t, e, ok3.ID, model.ReadingKindLoadProfile), 1)
	require.Empty(t, acceptRows(t, e, fail2.ID, model.ReadingKindLoadProfile))
}

// ---------------------------------------------------------------------------
// Test 4 (F2 acceptance criterion 7): an offset-less provider timestamp is
// stored as the correct UTC instant for Europe/Istanbul. GridBox
// gridbox_offsetless.json: "2026-09-14T10:15:00" (no offset) must become
// 2026-09-14T07:15:00Z.
// ---------------------------------------------------------------------------

func TestIngestionGridBoxOffsetlessTimestampStoredAsUTCInstant(t *testing.T) {
	srv := fake.NewTLSServer(t, append(acceptGridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_offsetless.json"))},
	)...)
	e := acceptEnvSetup(t, 17004, srv.Pins)

	acceptDefinition(t, e, model.IntegrationProviderGridbox, "T17Offsetless", acceptGridboxEndpoints(srv))
	credID := acceptConfigure(t, e, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "T17Offsetless",
		Username: acceptPtr("fixture-user"), Secret: acceptSecret("fixture-pass"),
	})
	analyzer := acceptAnalyzer(t, e, model.IntegrationProviderGridbox, "T17Offsetless", "WIRING-OFF-1", decimal.NewFromInt(1))

	from, to := acceptIstanbulDay(2026, 9, 14)
	acceptEnqueueFetch(t, e, credID, analyzer.ID, model.ReadingKindLoadProfile, &job.Window{From: from, To: to}, 0)
	runs := acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", analyzer.ID, 1)
	require.Equal(t, "success", runs[0].Status)

	var tsUTCText string
	require.NoError(t, e.pool.QueryRow(e.ctx,
		`select to_char(ts at time zone 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS') from meter_readings where analyzer_id = $1`,
		analyzer.ID).Scan(&tsUTCText))
	require.Equal(t, "2026-09-14T07:15:00", tsUTCText)
}

// ---------------------------------------------------------------------------
// Test 5 (F2 acceptance criterion 8): ARIL rows produce NULL, not 0, in the
// T1/T2/T3 registers — ARIL's owner_consumptions mapping never populates
// them at all (06 §4 / task-8-brief.md; ARIL has no time-of-use registers),
// so any successful ARIL fetch proves this by construction.
// ---------------------------------------------------------------------------

func TestIngestionARILTimeOfUseRegistersStoredAsNull(t *testing.T) {
	srv := fake.NewTLSServer(t,
		acceptARILAuthRoute(),
		fake.Route{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "aril", "aril_success.json"))},
	)
	e := acceptEnvSetup(t, 17005, srv.Pins)

	acceptDefinition(t, e, model.IntegrationProviderARIL, "T17ARILNull", acceptARILEndpoints(srv))
	credID := acceptConfigure(t, e, credentials.Input{
		Provider: model.IntegrationProviderARIL, Subtype: "T17ARILNull",
		Username: acceptPtr("fixture-user"), Secret: acceptSecret("fixture-pass"),
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	definitionType := int16(15)
	analyzer, err := e.analyzers.Create(e.ctx, e.tn.Scope, model.Analyzer{
		CompanyID: e.tn.Company.ID, BuildingID: &e.tn.Buildings[0].ID,
		Provider: model.IntegrationProviderARIL, ProviderSubtype: "T17ARILNull",
		InstallationNumber: "1000999888", DefinitionType: &definitionType,
		MeterMultiplier: decimal.NewFromInt(1), IsActive: true, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	from, to := acceptIstanbulDay(2026, 9, 1)
	acceptEnqueueFetch(t, e, credID, analyzer.ID, model.ReadingKindLoadProfile, &job.Window{From: from, To: to}, 0)
	runs := acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", analyzer.ID, 1)
	require.Equal(t, "success", runs[0].Status)

	var withTimeOfUse int
	require.NoError(t, e.pool.QueryRow(e.ctx,
		`select count(*) from meter_readings where analyzer_id = $1 and (t1_import is not null or t2_import is not null or t3_import is not null)`,
		analyzer.ID).Scan(&withTimeOfUse))
	require.Zero(t, withTimeOfUse, "ARIL never populates T1/T2/T3 — must be NULL, never 0")

	var withActiveImport int
	require.NoError(t, e.pool.QueryRow(e.ctx,
		`select count(*) from meter_readings where analyzer_id = $1 and active_import is not null`,
		analyzer.ID).Scan(&withActiveImport))
	require.Positive(t, withActiveImport, "active_import must actually be populated (this is a real fetch, not a vacuous one)")
}

// ---------------------------------------------------------------------------
// Test 6 (F2 acceptance criterion 9): PM5340 interval generation accumulates
// into active_export correctly across a gap, an out-of-order batch and a
// duplicate fetch — driven by three explicit-Window FetchReadings tasks
// through the real worker (worker.Build wires generation.New as PM5340's
// PostPersistHook — internal/worker/wiring.go).
//
// Fetch A [00:00,01:00): 00:00 (+2.0), 00:15 (+3.0)         -> running 2.0, 5.0
// Fetch B [01:00,02:00): 01:45 (+1.5) — a GAP (nothing at 00:30/01:00/01:30) -> running 5.0+1.5=6.5
// Fetch C [00:00,02:00): 00:15 (+3.0) DUPLICATE, 00:45 (+1.0) OUT-OF-ORDER
//   -> 00:45 recomputes to 5.0+1.0=6.0, and everything after it (01:45) must
//      be recomputed FORWARD too: 6.0+1.5=7.5 (not left at the stale 6.5).
//
// Expected final active_export, hand-written: 00:00=2.0, 00:15=5.0,
// 00:45=6.0, 01:45=7.5.
// ---------------------------------------------------------------------------

func acceptPM5340WindowRoute(from, to time.Time, items []string) fake.Route {
	wantStart := from.UTC().Format(time.RFC3339)
	wantEnd := to.UTC().Format(time.RFC3339)
	return fake.Route{
		Method: http.MethodGet,
		Path:   "/api/v1/readings",
		Match: func(r *http.Request) bool {
			return r.URL.Query().Get("start") == wantStart && r.URL.Query().Get("end") == wantEnd
		},
		Respond: fake.JSON(http.StatusOK, acceptPM5340Envelope(items)),
	}
}

func TestIngestionPM5340CumulativeGenerationThroughTheWorker(t *testing.T) {
	day := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t000, t0015, t0045, t0145 := day, day.Add(15*time.Minute), day.Add(45*time.Minute), day.Add(105*time.Minute)
	wA := job.Window{From: day, To: day.Add(time.Hour)}
	wB := job.Window{From: day.Add(time.Hour), To: day.Add(2 * time.Hour)}
	wC := job.Window{From: day, To: day.Add(2 * time.Hour)}

	srv := fake.NewTLSServer(t,
		acceptPM5340WindowRoute(wA.From, wA.To, []string{
			acceptPM5340Row(t000, "1000.000", "8.000"),
			acceptPM5340Row(t0015, "1001.000", "12.000"),
		}),
		acceptPM5340WindowRoute(wB.From, wB.To, []string{
			acceptPM5340Row(t0145, "1004.000", "6.000"),
		}),
		acceptPM5340WindowRoute(wC.From, wC.To, []string{
			acceptPM5340Row(t0015, "1001.000", "12.000"), // duplicate of fetch A's second row
			acceptPM5340Row(t0045, "1002.000", "4.000"),  // out-of-order: between 00:15 and 01:45
		}),
	)
	e := acceptEnvSetup(t, 17006, srv.Pins)

	acceptDefinition(t, e, model.IntegrationProviderPM5340, "T17Gen", map[string]string{})
	credID := acceptConfigure(t, e, credentials.Input{
		Provider: model.IntegrationProviderPM5340, Subtype: "T17Gen", PM5340URL: acceptPtr(srv.URL),
	})
	analyzer := acceptAnalyzer(t, e, model.IntegrationProviderPM5340, "T17Gen", "PM-GEN-1", decimal.NewFromInt(1))

	acceptEnqueueFetch(t, e, credID, analyzer.ID, model.ReadingKindLoadProfile, &wA, 0)
	acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", analyzer.ID, 1)

	acceptEnqueueFetch(t, e, credID, analyzer.ID, model.ReadingKindLoadProfile, &wB, 0)
	acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", analyzer.ID, 2)

	acceptEnqueueFetch(t, e, credID, analyzer.ID, model.ReadingKindLoadProfile, &wC, 0)
	runs := acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", analyzer.ID, 3)
	require.Equal(t, "success", runs[0].Status)

	type row struct {
		ts     time.Time
		export string
	}
	rs, err := e.pool.Query(e.ctx, `select ts, active_export::text from meter_readings where analyzer_id = $1 and kind = 'load_profile' order by ts`, analyzer.ID)
	require.NoError(t, err)
	defer rs.Close()
	var got []row
	for rs.Next() {
		var r row
		require.NoError(t, rs.Scan(&r.ts, &r.export))
		got = append(got, r)
	}
	require.NoError(t, rs.Err())

	require.Len(t, got, 4, "00:00, 00:15, 00:45, 01:45 — no duplicate of 00:15")
	wantTs := []time.Time{t000, t0015, t0045, t0145}
	wantExport := []string{"2.0000", "5.0000", "6.0000", "7.5000"}
	for i, w := range got {
		require.True(t, w.ts.Equal(wantTs[i]), "row %d ts", i)
		require.Equal(t, wantExport[i], w.export, "row %d (ts=%s) active_export", i, w.ts)
	}
}

// ---------------------------------------------------------------------------
// Test 7 (F2 acceptance criterion 10): a negative index delta creates an
// unresolved consumption_anomalies row — GridBox, driven through the real
// worker.
// ---------------------------------------------------------------------------

func TestIngestionNegativeDeltaThroughTheWorkerCreatesAnomaly(t *testing.T) {
	newRowBody := []byte(`{"ResultStatus":1,"ResultObject":[{"ProfileDateTime":"2026-09-01T04:00:00+03:00","ActiveEndex":1500}]}`)
	srv := fake.NewTLSServer(t, append(acceptGridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, newRowBody)},
	)...)
	e := acceptEnvSetup(t, 17007, srv.Pins)

	acceptDefinition(t, e, model.IntegrationProviderGridbox, "T17NegDelta", acceptGridboxEndpoints(srv))
	credID := acceptConfigure(t, e, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "T17NegDelta",
		Username: acceptPtr("fixture-user"), Secret: acceptSecret("fixture-pass"),
	})
	analyzer := acceptAnalyzer(t, e, model.IntegrationProviderGridbox, "T17NegDelta", "WIRING-NEG-1", decimal.NewFromInt(1))

	// acceptIstanbulDay(2026,9,1)'s From is Istanbul-local midnight, i.e.
	// 2026-08-31T21:00:00Z (Istanbul is a fixed UTC+3) — the prior reading
	// must be strictly before THAT UTC instant, not before the
	// Istanbul-local calendar date's own midnight in UTC terms.
	priorTs := time.Date(2026, 8, 31, 20, 0, 0, 0, time.UTC) // strictly before the fetch window's chunk.From (2026-08-31T21:00:00Z)
	priorImport := decimal.RequireFromString("2000")
	_, _, err := e.readings.BulkInsert(e.ctx, e.tn.Scope, []model.MeterReading{{
		AnalyzerID: analyzer.ID, Ts: priorTs, Kind: model.ReadingKindLoadProfile,
		ActiveImport: &priorImport, MultiplierApplied: decimal.NewFromInt(1),
		SourceProvider: model.IntegrationProviderGridbox, IngestedAt: priorTs,
	}})
	require.NoError(t, err)

	from, to := acceptIstanbulDay(2026, 9, 1)
	acceptEnqueueFetch(t, e, credID, analyzer.ID, model.ReadingKindLoadProfile, &job.Window{From: from, To: to}, 0)
	runs := acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", analyzer.ID, 1)
	require.Equal(t, "success", runs[0].Status)

	anomalies, err := e.anomalies.List(e.ctx, e.tn.Scope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{analyzer.ID}})
	require.NoError(t, err)
	require.Len(t, anomalies, 1)
	require.Equal(t, "negative_delta", anomalies[0].Reason)
	require.Nil(t, anomalies[0].ResolvedAt, "a freshly created anomaly must be unresolved")
}

// ---------------------------------------------------------------------------
// Test 8 (F2 global constraint: secrets are never logged and never appear
// in error text, job_runs, cursors or operational messages). GridBox's own
// auth-failure fixture route ECHOES the submitted password in its response
// body — the one channel through which the plaintext could leak into
// httpx's error, and from there into every one of the four sinks below —
// so this test drives a real auth failure through the real worker with a
// real slog JSON logger writing into a buffer, and greps all four.
// ---------------------------------------------------------------------------

func TestIngestionSecretsNeverReachRunsCursorsMessagesOrLogs(t *testing.T) {
	const secretPassword = "FIXTURE-SUPER-SECRET-9182"

	srv := fake.NewTLSServer(t,
		fake.Route{Method: http.MethodPost, Path: "/gridbox/token", Respond: acceptGridboxAuthFailureResponder(t)},
	)
	logs := &acceptLogBuffer{}
	log := logging.New("debug", "json", logs)
	e := acceptEnvSetupWithLog(t, 17008, srv.Pins, log, logs, nil)

	acceptDefinition(t, e, model.IntegrationProviderGridbox, "T17Secret", acceptGridboxEndpoints(srv))
	credID := acceptConfigure(t, e, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "T17Secret",
		Username: acceptPtr("fixture-user"), Secret: acceptSecret(secretPassword),
	})
	analyzer := acceptAnalyzer(t, e, model.IntegrationProviderGridbox, "T17Secret", "WIRING-SECRET-1", decimal.NewFromInt(1))

	from, to := acceptIstanbulDay(2026, 9, 1)
	acceptEnqueueFetch(t, e, credID, analyzer.ID, model.ReadingKindLoadProfile, &job.Window{From: from, To: to}, 0)
	runs := acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", analyzer.ID, 1)
	require.Equal(t, "failed", runs[0].Status)

	// Sink 1: job_runs.error.
	require.NotNil(t, runs[0].Error)
	require.NotContains(t, *runs[0].Error, secretPassword, "job_runs.error must never contain the plaintext password")

	// Sink 2: ingestion_cursors.last_error.
	cur, err := e.cursors.Get(e.ctx, e.tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.NotNil(t, cur.LastError)
	require.NotContains(t, *cur.LastError, secretPassword, "ingestion_cursors.last_error must never contain the plaintext password")

	// Sink 3: operational_messages.
	rows, err := e.pool.Query(e.ctx, `select message, coalesce(detail, ''), coalesce(metadata::text, '') from operational_messages where company_id = $1`, e.tn.Company.ID)
	require.NoError(t, err)
	defer rows.Close()
	var messageCount int
	for rows.Next() {
		var message, detail, metadata string
		require.NoError(t, rows.Scan(&message, &detail, &metadata))
		messageCount++
		require.NotContains(t, message, secretPassword, "operational_messages.message must never contain the plaintext password")
		require.NotContains(t, detail, secretPassword, "operational_messages.detail must never contain the plaintext password")
		require.NotContains(t, metadata, secretPassword, "operational_messages.metadata must never contain the plaintext password")
	}
	require.NoError(t, rows.Err())
	require.Positive(t, messageCount, "the failed run must actually have produced an operational message (a guard over an empty set proves nothing)")

	// Sink 4: the real slog JSON log stream.
	require.NotContains(t, logs.String(), secretPassword, "the worker's own log output must never contain the plaintext password")
}

// ---------------------------------------------------------------------------
// Test 9: EPİAŞ sync through the worker is idempotent — running the same
// epias.sync_prices task twice converges rather than duplicating
// market_prices_hourly/yekdem_monthly.
//
// M3 (final review B): config.External now carries EPIASCASURL/
// EPIASBaseURL (internal/platform/config), and worker.Build's own
// epias.New wiring (internal/worker/wiring.go) passes them through — so
// this test drives built.Handlers.Prices UNMODIFIED, through the exact
// production wiring (including the worker's own Redis lock, not a
// lock.NewMemory substitute), pointed at fake.NewTLSServer only via
// acceptEnvSetupWithEPIAS's config override. An earlier version of this
// test had no such config seam and substituted a hand-built
// marketdata.Syncer for Handlers.Prices at the test layer instead; that
// substitution is no longer needed.
// ---------------------------------------------------------------------------

func TestEPIASSyncThroughTheWorkerIsIdempotent(t *testing.T) {
	srv := fake.NewTLSServer(t,
		acceptEPIASCasRoute("TGT-FIXTURE-ticket-9-cas01"), // epias.Client's ticketPattern requires a "TGT-" prefix
		fake.Route{Method: http.MethodPost, Path: acceptEPIASMcpPath, Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_success.json"))},
		fake.Route{Method: http.MethodPost, Path: acceptEPIASYekPath, Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_yekdem.json"))},
	)

	e := acceptEnvSetupWithEPIAS(t, 17009, srv.Pins, srv.URL+acceptEPIASCasPath, srv.URL+"/electricity-service")

	from := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)
	window := job.Window{From: from, To: to}

	enqueueSync := func() {
		task, err := job.NewSyncPricesTask(job.SyncPricesPayload{Window: &window}, job.TaskOptions{MaxRetry: 0})
		require.NoError(t, err)
		_, err = e.client.Enqueue(e.ctx, task)
		require.NoError(t, err)
	}

	waitSyncRun := func(wantCount int) {
		require.Eventually(t, func() bool {
			var n int
			err := e.pool.QueryRow(e.ctx, `select count(*) from job_runs where job_type = $1 and status = 'success'`, job.TypeEPIASSyncPrices).Scan(&n)
			return err == nil && n >= wantCount
		}, 60*time.Second, 200*time.Millisecond, "timed out waiting for %d successful epias.sync_prices run(s)", wantCount)
	}

	enqueueSync()
	waitSyncRun(1)
	var n1 int
	require.NoError(t, e.pool.QueryRow(e.ctx, `select count(*) from market_prices_hourly`).Scan(&n1))
	require.Equal(t, 24, n1)

	enqueueSync()
	waitSyncRun(2)
	var n2 int
	require.NoError(t, e.pool.QueryRow(e.ctx, `select count(*) from market_prices_hourly`).Scan(&n2))
	require.Equal(t, n1, n2, "re-syncing the same window must converge, not duplicate")
}

// ---------------------------------------------------------------------------
// Test 10: an auth failure is not retried — asynq archives the task after
// exactly one attempt (job.ClassifyForRetry wraps integration.ErrAuth with
// asynq.SkipRetry).
// ---------------------------------------------------------------------------

func TestAuthFailureIsNotRetried(t *testing.T) {
	srv := fake.NewTLSServer(t,
		fake.Route{Method: http.MethodPost, Path: "/gridbox/token", Respond: acceptGridboxAuthFailureResponder(t)},
	)
	e := acceptEnvSetup(t, 17010, srv.Pins)

	acceptDefinition(t, e, model.IntegrationProviderGridbox, "T17NoRetry", acceptGridboxEndpoints(srv))
	credID := acceptConfigure(t, e, credentials.Input{
		Provider: model.IntegrationProviderGridbox, Subtype: "T17NoRetry",
		Username: acceptPtr("fixture-user"), Secret: acceptSecret("fixture-pass"),
	})
	analyzer := acceptAnalyzer(t, e, model.IntegrationProviderGridbox, "T17NoRetry", "WIRING-NORETRY-1", decimal.NewFromInt(1))

	from, to := acceptIstanbulDay(2026, 9, 1)
	// MaxRetry: 5 — high enough that if ClassifyForRetry's asynq.SkipRetry
	// wrap were somehow lost, asynq WOULD retry (proving the guard would
	// actually catch a regression, not just happen to match a MaxRetry of
	// 0).
	info := acceptEnqueueFetch(t, e, credID, analyzer.ID, model.ReadingKindLoadProfile, &job.Window{From: from, To: to}, 5)

	runs := acceptWaitRuns(t, e, job.TypeIntegrationFetchReadings, "analyzer_id", analyzer.ID, 1)
	require.Equal(t, "failed", runs[0].Status)

	redisOpt, err := job.RedisOpt(e.cfg.Redis)
	require.NoError(t, err)
	insp := asynq.NewInspector(redisOpt)
	defer func() { _ = insp.Close() }()

	var taskInfo *asynq.TaskInfo
	require.Eventually(t, func() bool {
		ti, ierr := insp.GetTaskInfo(info.Queue, info.ID)
		if ierr != nil {
			return false
		}
		taskInfo = ti
		return ti.State == asynq.TaskStateArchived
	}, 30*time.Second, 200*time.Millisecond, "the task must be archived (not retried) after an auth failure")

	require.Zero(t, taskInfo.Retried, "an auth failure must never be retried")
}
