//go:build integration

package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest/generation"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// graphTestConfig is workerTestConfig's same-package twin (see
// wiring_integration_test.go, package worker_test): it loads a full, valid
// *config.Config via config.Load with a map lookup, never config.FromEnv.
// Duplicated rather than shared — the two test files live in different
// packages (worker vs worker_test) and each is independently buildable
// under -tags=integration, the same tradeoff connectedClients already
// documents for internal/scheduler.
func graphTestConfig(t *testing.T, dsn string, redisCfg config.Redis) *config.Config {
	t.Helper()
	env := map[string]string{
		"EKOKOD_PUBLIC_URL":                "https://ekokod.example.com",
		"EKOKOD_DB_URL":                    dsn,
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
	}
	cfg, err := config.Load(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	require.NoError(t, err)
	return cfg
}

// unusedButValidDSN is a syntactically valid, never-dialled DSN: build()
// takes pool directly (already opened by testfixtures.NewIsolatedDB), so
// cfg.DB.URL's only consumer is config.Load's url.Parse check.
const unusedButValidDSN = "postgres://user:pass@localhost:5432/unused"

// buildGraph is build() plus t.Cleanup of the returned graph's own
// closers — every test in this file calls it once and inspects the graph
// directly (the whole point of the build()/Build() split: graph exposes
// credentialsDeps, ingestDeps, cipher and integrationRepo, none of which
// Built's public two-field shape carries).
func buildGraph(t *testing.T, pool *pgxpool.Pool, redisCfg config.Redis) graph {
	t.Helper()
	cfg := graphTestConfig(t, unusedButValidDSN, redisCfg)
	g, err := build(context.Background(), cfg, pool, testfixtures.DiscardLogger())
	require.NoError(t, err)
	t.Cleanup(func() {
		for i := len(g.closers) - 1; i >= 0; i-- {
			g.closers[i].close()
		}
	})
	return g
}

// TestBuildGraphKeySourcesArePinned proves credentials.Deps.StateKey is
// derived from cfg.Security.JWTSigningKey, not cfg.Security.EncryptionKey —
// a mistake that would have stayed green under the acceptance test alone,
// because both keys are 32+ bytes and StateKey never surfaces in any
// handler-registration assertion.
func TestBuildGraphKeySourcesArePinned(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	redisCfg := testfixtures.RedisConfig(t)
	cfg := graphTestConfig(t, unusedButValidDSN, redisCfg)
	g := buildGraph(t, pool, redisCfg)

	require.Equal(t, StateKey(cfg.Security.JWTSigningKey), g.credentialsDeps.StateKey,
		"credentials.Deps.StateKey must be derived from the JWT signing key")
	require.NotEqual(t, StateKey(cfg.Security.EncryptionKey), g.credentialsDeps.StateKey,
		"credentials.Deps.StateKey must never be derived from the encryption key")
}

// TestBuildGraphIntegrationRepoCipherComesFromEncryptionKey proves build()'s
// integration repository seals with a cipher built from
// cfg.Security.EncryptionKey, not cfg.Security.JWTSigningKey[:32] — a
// second, independently correct-looking 32-byte key that would also
// satisfy crypto.NewCipher without erroring, so nothing before this test
// would ever notice the swap.
//
// It seals a secret through g.integrationRepo (the graph's real, single
// cipher instance) and opens it with a SECOND repository built from a
// cipher independently constructed from cfg.Security.EncryptionKey: they
// must agree, because OpenSecret only succeeds against the exact key (and
// AAD) Seal used.
func TestBuildGraphIntegrationRepoCipherComesFromEncryptionKey(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	redisCfg := testfixtures.RedisConfig(t)
	g := buildGraph(t, pool, redisCfg)

	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 9101)
	var definitionID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_definitions (provider, subtype) values ($1, $2) returning id`,
		"osos", "default").Scan(&definitionID))

	const plainSecret = "graph-cipher-round-trip-secret"
	created, err := g.integrationRepo.UpsertCredential(ctx, tenant.Scope, model.IntegrationCredential{
		CompanyID: tenant.Company.ID, DefinitionID: definitionID, IsActive: true,
	}, []byte(plainSecret), nil)
	require.NoError(t, err)

	cfg := graphTestConfig(t, unusedButValidDSN, redisCfg)
	independentCipher, err := crypto.NewCipher(cfg.Security.EncryptionKey)
	require.NoError(t, err)

	secondRepo := postgres.NewIntegrationRepository(pool, independentCipher)
	secret, _, err := secondRepo.OpenSecret(ctx, tenant.Scope, created.ID)
	require.NoError(t, err, "a cipher built from cfg.Security.EncryptionKey must open what the graph's repo sealed")
	require.Equal(t, plainSecret, string(secret))
}

// TestBuildGraphRegistersOnePM5340GenerationHook proves
// ingest.Deps.Hooks[model.IntegrationProviderPM5340] holds exactly one
// *generation.Accumulator, not an empty hook list that would silently stop
// PM5340 readings from ever updating cumulative generation.
func TestBuildGraphRegistersOnePM5340GenerationHook(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	redisCfg := testfixtures.RedisConfig(t)
	g := buildGraph(t, pool, redisCfg)

	hooks := g.ingestDeps.Hooks[model.IntegrationProviderPM5340]
	require.Len(t, hooks, 1, "PM5340 must have exactly one post-persist hook")
	_, ok := hooks[0].(*generation.Accumulator)
	require.True(t, ok, "the PM5340 hook must be a *generation.Accumulator")
}

// TestBuildGraphGenerationHookUsesRedisLockAndConfiguredFutureTolerance
// proves the PM5340 generation hook build() wires is NOT built with a nil
// lock or a hard-coded tolerance: per R52 the hook must share the graph's
// one redis lock, and its recompute upper bound must come from
// cfg.Ingest.FutureTolerance rather than hook.go's own package default.
//
// g.credentialsDeps.Locker is the SAME redisLock variable build() passes to
// every other Locker consumer in the graph (httpx's pool, the EPİAŞ
// client, credentials.Deps) — comparing the hook's own Locker() against it
// by equality proves it is that one shared instance, not some other
// (possibly no-op) Locker.
func TestBuildGraphGenerationHookUsesRedisLockAndConfiguredFutureTolerance(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	redisCfg := testfixtures.RedisConfig(t)
	g := buildGraph(t, pool, redisCfg)
	cfg := graphTestConfig(t, unusedButValidDSN, redisCfg)

	hooks := g.ingestDeps.Hooks[model.IntegrationProviderPM5340]
	require.Len(t, hooks, 1)
	hook, ok := hooks[0].(*generation.Accumulator)
	require.True(t, ok)

	require.NotNil(t, hook.Locker(), "the generation hook must have a Locker, not nil (I3)")
	require.Equal(t, g.credentialsDeps.Locker, hook.Locker(),
		"the generation hook must share the SAME redis lock every other Locker consumer in the graph uses (I3)")

	require.Equal(t, cfg.Ingest.FutureTolerance, hook.FutureTolerance(),
		"the generation hook's recompute upper bound must come from cfg.Ingest.FutureTolerance, not its own package default (M12)")
}

// TestBuildGraphOpensNamedClosersForRedisAndJobClient proves build()
// actually attaches a closer for the job client (not only the redis client
// the connected-clients metric can observe), by asserting the closers
// slice's names directly, in open order.
func TestBuildGraphOpensNamedClosersForRedisAndJobClient(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	redisCfg := testfixtures.RedisConfig(t)
	g := buildGraph(t, pool, redisCfg)

	names := make([]string, len(g.closers))
	for i, c := range g.closers {
		names[i] = c.name
	}
	require.Equal(t, []string{"redis", "job-client"}, names)
}

// TestConsumptionRefreshEnqueuerAppliesConfiguredMaxRetry proves
// consumptionRefreshEnqueuer actually threads its own maxRetry field into
// the asynq task it builds via
// job.NewConsumptionRefreshTask's TaskOptions, rather than the adapter
// silently dropping it with job.TaskOptions{} (which would leave every
// consumption.refresh task at asynq's own zero-value default retry count).
// Mirrors internal/job/consumption_integration_test.go's enqueue-then-
// asynq.Inspector.GetTaskInfo pattern (also used by
// internal/job/ingestion_integration_test.go's TestAuthFailureIsNotRetried)
// at the worker adapter's own level, against a real Redis. MaxRetry is a
// non-default 7 so
// a mutant hard-coding job.TaskOptions{} (retry 0, asynq's default 25, or
// any other incidental value) cannot coincidentally match.
func TestConsumptionRefreshEnqueuerAppliesConfiguredMaxRetry(t *testing.T) {
	redisCfg := testfixtures.RedisConfig(t)
	jobClient, err := job.NewClient(redisCfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = jobClient.Close() })

	fixedNow := time.Date(2026, 5, 1, 9, 30, 0, 0, time.UTC)
	enq := consumptionRefreshEnqueuer{client: jobClient, maxRetry: 7, clock: clock.NewFake(fixedNow)}

	from := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to}
	require.NoError(t, enq.EnqueueConsumptionRefresh(context.Background(), payload))

	redisOpt, err := job.RedisOpt(redisCfg)
	require.NoError(t, err)
	insp := asynq.NewInspector(redisOpt)
	defer func() { _ = insp.Close() }()

	ti, err := insp.GetTaskInfo(job.QueueDefault, job.ConsumptionRefreshTaskID(from, to, fixedNow))
	require.NoError(t, err)
	require.Equal(t, 7, ti.MaxRetry, "the worker adapter must pass its configured MaxRetry through to the enqueued task, not job.TaskOptions{}")
}

// TestConsumptionRefreshEnqueuerMapsTaskIDConflictToSuccess is R100(2)'s
// proof: a second EnqueueConsumptionRefresh call that collides with the
// first (same window, same debounce minute — the designed burst-collapse
// case) must return nil, not asynq.ErrTaskIDConflict. A mutant that drops
// the errors.Is(err, asynq.ErrTaskIDConflict) check in
// consumptionRefreshEnqueuer.EnqueueConsumptionRefresh turns this red.
func TestConsumptionRefreshEnqueuerMapsTaskIDConflictToSuccess(t *testing.T) {
	redisCfg := testfixtures.RedisConfig(t)
	jobClient, err := job.NewClient(redisCfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = jobClient.Close() })

	fixedNow := time.Date(2026, 5, 1, 9, 30, 0, 0, time.UTC)
	enq := consumptionRefreshEnqueuer{client: jobClient, maxRetry: 3, clock: clock.NewFake(fixedNow)}

	from := time.Date(2026, 5, 2, 8, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)

	require.NoError(t, enq.EnqueueConsumptionRefresh(context.Background(), job.ConsumptionRefreshPayload{
		CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to,
	}))
	require.NoError(t, enq.EnqueueConsumptionRefresh(context.Background(), job.ConsumptionRefreshPayload{
		CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to,
	}), "a colliding enqueue (asynq.ErrTaskIDConflict) must be mapped to success, never surfaced as an error")
}
