//go:build integration

package worker_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	goredis "github.com/redis/go-redis/v9"
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
// relies on: worker.Build wires a real Handlers.Ingestion, Handlers.Backfill
// and Handlers.Prices, so every F2 integration task type routes to a
// handler, and R17's consumption.refresh (declared, never enqueued in F2)
// has none.
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
	require.NotNil(t, built.Handlers.Ingestion)
	require.NotNil(t, built.Handlers.Backfill)
	require.NotNil(t, built.Handlers.Prices)

	mux := asynq.NewServeMux()
	job.Register(mux, built.Handlers)

	for _, typ := range []string{
		job.TypeIntegrationSyncDispatch,
		job.TypeIntegrationSyncAnalyzers,
		job.TypeIntegrationFetchReadings,
		job.TypeIntegrationBackfill,
		job.TypeEPIASSyncPrices,
	} {
		_, pattern := mux.Handler(asynq.NewTask(typ, nil))
		require.Equal(t, typ, pattern, "no handler for %s", typ)
	}

	_, pattern := mux.Handler(asynq.NewTask(job.TypeConsumptionRefresh, nil))
	require.Empty(t, pattern, "R17: consumption.refresh has no F2 handler")
}

// TestWorkerBuildClosesCleanlyTwice is the fix-round-1 M1 test: Built.Close
// is guarded by sync.Once (worker.Build wraps graph.closers in one), so it
// is safe to call twice, not merely "safe once, and probably fine again
// because the underlying clients happen to tolerate it" — this actually
// calls Close() twice and requires neither call to panic.
func TestWorkerBuildClosesCleanlyTwice(t *testing.T) {
	dsn := testfixtures.StartPostgres(t)
	pool := testfixtures.NewPool(t, dsn)
	redisCfg := testfixtures.RedisConfig(t)
	cfg := workerTestConfig(t, dsn, redisCfg)

	built, err := worker.Build(context.Background(), cfg, pool, testfixtures.DiscardLogger())
	require.NoError(t, err)
	require.NotPanics(t, built.Close)
	require.NotPanics(t, built.Close, "Built.Close must be idempotent: a second call must not panic")
}

// TestWorkerBuildCloseReleasesRedisOnSuccessPath is the fix-round-1 I3
// test: TestWorkerBuildClosesCleanlyTwice only proved Close() doesn't
// panic, not that it actually releases anything — a no-op Close would pass
// it too. This asserts the platform redis client's connection is gone
// (connected_clients back at baseline) after built.Close() on the ordinary
// success path, the same connected_clients technique
// TestWorkerBuildClosesEarlierResourcesOnLateFailure below uses for the
// error path.
func TestWorkerBuildCloseReleasesRedisOnSuccessPath(t *testing.T) {
	dsn := testfixtures.StartPostgres(t)
	pool := testfixtures.NewPool(t, dsn)
	redisCfg := testfixtures.RedisConfig(t)
	cfg := workerTestConfig(t, dsn, redisCfg)

	monitorOpts, err := goredis.ParseURL(redisCfg.URL)
	require.NoError(t, err)
	monitor := goredis.NewClient(monitorOpts)
	t.Cleanup(func() { _ = monitor.Close() })

	baseline := connectedClients(t, monitor)

	built, err := worker.Build(context.Background(), cfg, pool, testfixtures.DiscardLogger())
	require.NoError(t, err)
	require.Greater(t, connectedClients(t, monitor), baseline, "Build must have opened its own redis client")

	built.Close()

	require.Eventually(t, func() bool {
		return connectedClients(t, monitor) <= baseline
	}, 5*time.Second, 50*time.Millisecond,
		"Built.Close must release the redis client it opened on the success path, not no-op")
}

// connectedClients reads Redis's own INFO clients: connected_clients, the
// server-side ground truth for how many client connections are open right
// now — independent of, and unable to be fooled by, whichever
// *goredis.Client object this process happens to hold a reference to.
// Mirrors internal/scheduler's identical helper (duplicated here rather
// than shared: each package's integration suite is independently buildable
// under -tags=integration, and this is the only place internal/worker
// needs it).
func connectedClients(t *testing.T, c *goredis.Client) int {
	t.Helper()
	info, err := c.Info(context.Background(), "clients").Result()
	require.NoError(t, err)
	for _, line := range strings.Split(info, "\r\n") {
		if n, ok := strings.CutPrefix(line, "connected_clients:"); ok {
			v, err := strconv.Atoi(strings.TrimSpace(n))
			require.NoError(t, err)
			return v
		}
	}
	t.Fatal("connected_clients not found in INFO clients output")
	return 0
}

// TestWorkerBuildClosesEarlierResourcesOnLateFailure is the Task 16 part-A
// review carryover: when a LATE Build step fails, every long-lived resource
// already opened FOR THIS CALL must be closed on that step's error path,
// not leaked until process exit.
//
// The failing step is crypto.NewCipher: this test truncates the already
// config.Load-validated cfg.Security.EncryptionKey to 16 bytes (32
// required) so every earlier step (redis, httpx pool, EPİAŞ client, job
// client) still opens its resource and only the cipher step fails.
//
// The assertion targets the platform redis client specifically because
// internal/store/redis.New dials eagerly (pings before returning), so its
// connection is externally observable via connected_clients; the job
// client and httpx pool's clients dial lazily on first use (same fact
// internal/scheduler's TestSchedulerReleasesRedisClientOnRegisterFailure
// documents), so this metric can't independently discriminate their Close
// calls — it does discriminate closeAll()'s presence as a whole, which is
// what "every resource closed on every error path" requires.
func TestWorkerBuildClosesEarlierResourcesOnLateFailure(t *testing.T) {
	dsn := testfixtures.StartPostgres(t)
	pool := testfixtures.NewPool(t, dsn)
	redisCfg := testfixtures.RedisConfig(t)
	cfg := workerTestConfig(t, dsn, redisCfg)

	monitorOpts, err := goredis.ParseURL(redisCfg.URL)
	require.NoError(t, err)
	monitor := goredis.NewClient(monitorOpts)
	t.Cleanup(func() { _ = monitor.Close() })

	baseline := connectedClients(t, monitor)

	cfg.Security.EncryptionKey = cfg.Security.EncryptionKey[:16]

	_, err = worker.Build(context.Background(), cfg, pool, testfixtures.DiscardLogger())
	require.Error(t, err)
	require.Contains(t, err.Error(), "cipher")

	require.Eventually(t, func() bool {
		return connectedClients(t, monitor) <= baseline
	}, 5*time.Second, 50*time.Millisecond,
		"Build must close every resource it already opened (the redis client behind the lock) "+
			"when a later construction step fails, not leak it")
}
