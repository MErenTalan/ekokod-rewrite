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
// review carryover: prove that when a LATE Build step fails, every
// long-lived resource Build already opened FOR THIS CALL is closed on that
// step's error path, not leaked until process exit.
//
// The failing step is crypto.NewCipher, a pure function of
// cfg.Security.EncryptionKey — this test truncates that one field to 16
// bytes (crypto.NewCipher requires exactly 32) AFTER config.Load has
// already validated and returned a fully valid cfg, so every step BEFORE
// the cipher (redis connect, the httpx pool, the EPİAŞ client, the job
// client) still succeeds and opens its resource; only the cipher step
// fails. This is "an invalid config value that fails only that step" per
// the review note, not a contrived constructor seam.
//
// Why the assertion is the platform redis client specifically:
// internal/store/redis.New pings before returning (it connects eagerly),
// so its connection is independently, externally observable via
// connected_clients. The job client (asynq.Client) and the httpx pool's
// clients dial lazily, on first real use — internal/scheduler's own
// TestSchedulerReleasesRedisClientOnRegisterFailure documents the same
// fact for asynq/go-redis — so removing only their Close call would not
// move connected_clients by itself; this test cannot independently prove
// those two in isolation, which is why the mutation proof below targets
// the one call this metric DOES discriminate (deleting worker.Build's
// entire closeAll() call on the cipher error path, which also drops the
// two lazy-dial closes, but is caught here by the redis client's leak
// alone).
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
