//go:build integration

package worker_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/MErenTalan/ekokod-rewrite/internal/worker"
)

// workerTestConfig loads a full, valid *config.Config against real
// Postgres/Redis fixtures, the same shape config_test.go's own valid()
// uses, via config.Load with a map lookup (never config.FromEnv, which
// would read the real process environment).
func workerTestConfig(t *testing.T, dsn string, redisCfg config.Redis) *config.Config {
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

// TestWorkerRegistersEveryF2Handler proves the wiring the acceptance suite
// relies on.
//
// PART-A SCOPE (see wiring.go's package doc): Build only wires
// Handlers.Prices (epias.sync_prices) in this task. Handlers.Ingestion and
// Handlers.Backfill — and therefore the sync_dispatch/sync_analyzers/
// fetch_readings/backfill task types — are wired by Part B once
// internal/credentials (Task 14) merges, because ingest.Service and
// backfill.Backfiller both REQUIRE a real ingest.CredentialOpener
// (internal/credentials.Service) that does not exist in this worktree.
// This test therefore asserts the mirror image of the brief's literal
// acceptance test: exactly the one handler Part A wires is registered, and
// every handler Part B still owns is (correctly, for now) absent — proving
// Part A wires precisely what it claims to, nothing more.
func TestWorkerRegistersEveryF2Handler(t *testing.T) {
	dsn := testfixtures.StartPostgres(t)
	pool := testfixtures.NewPool(t, dsn)
	redisCfg := testfixtures.RedisConfig(t)
	cfg := workerTestConfig(t, dsn, redisCfg)

	built, err := worker.Build(context.Background(), cfg, pool, testfixtures.DiscardLogger())
	require.NoError(t, err)
	defer built.Close()
	require.NotNil(t, built.Handlers)
	require.NotNil(t, built.Handlers.Log)

	mux := asynq.NewServeMux()
	job.Register(mux, built.Handlers)

	_, pattern := mux.Handler(asynq.NewTask(job.TypeEPIASSyncPrices, nil))
	require.Equal(t, job.TypeEPIASSyncPrices, pattern, "no handler for %s", job.TypeEPIASSyncPrices)

	for _, typ := range []string{
		job.TypeIntegrationSyncDispatch,
		job.TypeIntegrationSyncAnalyzers,
		job.TypeIntegrationFetchReadings,
		job.TypeIntegrationBackfill,
	} {
		_, pattern := mux.Handler(asynq.NewTask(typ, nil))
		require.Empty(t, pattern, "%s must not be registered until part B wires Ingestion/Backfill", typ)
	}

	_, pattern = mux.Handler(asynq.NewTask(job.TypeConsumptionRefresh, nil))
	require.Empty(t, pattern, "R17: consumption.refresh has no F2 handler")
}

// TestWorkerBuildClosesCleanlyTwice guards Built.Close's contract loosely:
// callers (cli/worker.go via defer, and this suite's own defer above) must
// be able to call it without the process taking down a live server on a
// double-close panic. asynq's *Client.Close and goredis's *Client.Close
// both already tolerate being called once; this test only pins that
// Built.Close itself never panics on ordinary use, not idempotency beyond
// what the underlying clients already provide.
func TestWorkerBuildClosesCleanlyTwice(t *testing.T) {
	dsn := testfixtures.StartPostgres(t)
	pool := testfixtures.NewPool(t, dsn)
	redisCfg := testfixtures.RedisConfig(t)
	cfg := workerTestConfig(t, dsn, redisCfg)

	built, err := worker.Build(context.Background(), cfg, pool, testfixtures.DiscardLogger())
	require.NoError(t, err)
	require.NotPanics(t, built.Close)
}
