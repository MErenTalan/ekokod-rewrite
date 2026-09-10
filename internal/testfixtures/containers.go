// Package testfixtures holds the container helpers and the tenant factory that
// every integration test in this repository shares.
//
// It exists because the same twenty lines of testcontainers setup had been
// copy-pasted into four packages — internal/store/postgres,
// internal/store/redis, internal/job and internal/scheduler — and the copies
// had already drifted: three of them replaced the redis module's wait strategy
// and one did not, which is exactly the flake described on StartRedis below.
// One copy of a fix is a fix; four copies of a fix is three chances to miss one.
//
// LAYERING NOTE. This package imports internal/store and
// internal/store/postgres, so a test in internal/job that calls StartRedis now
// reaches the store layer transitively. internal/arch's
// TestTheJobPackageDoesNotImportTheStore is unaffected and still meaningful:
// it loads production packages (packages.Config with Tests unset), so it
// constrains what internal/job's own code may import, which is the layering
// rule that matters. Test binaries linking a test helper are not that.
package testfixtures

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

const (
	// postgresImage and redisImage are pinned, not floating tags. An
	// integration suite that silently follows :latest turns an upstream
	// release into a local test failure with no local change to explain it.
	postgresImage = "timescale/timescaledb:2.30.0-pg16"
	redisImage    = "redis:7.4.11-alpine"

	fixtureDB       = "ekokod"
	fixtureUser     = "ekokod"
	fixturePassword = "ekokod"

	// postgresReadyBudget is how long a TimescaleDB container gets to become
	// reachable. It is MEASURED, not chosen for comfort.
	//
	// tcpostgres.BasicWaitStrategies leaves wait.ForListeningPort on the
	// library default of 60s, and testcontainers.WithWaitStrategy imposes a
	// 60s deadline over the whole set (options.go:395-397 delegates to
	// WithWaitStrategyAndDeadline(60*time.Second, …)). Running this package's
	// integration tests at 2x concurrency exhausted exactly that: a container
	// failed at 61.07s after 520 polls of the port check with
	// `context deadline exceeded` on
	// `GET /containers/<id>/json`, and a test that takes 5.34s in isolation
	// failed at 62.32s. Each poll is a Docker API call, and under contention
	// the WSL2 daemon serves them at roughly 115ms each rather than the
	// 100ms poll interval, so the budget is spent waiting on the daemon while
	// the database is already up.
	//
	// This is the SAME failure class as the redis flake documented on
	// redisWaitStrategy, arrived at from the opposite direction: redis had a
	// 10s budget from the start, and postgres grew into its 60s one when F1
	// took this package from about nine integration tests to twenty-three,
	// each booting its own container.
	//
	// 180s is three times the measured exhaustion point. It costs nothing on
	// a healthy machine — the wait returns as soon as the port answers — and
	// it is a bound, not a sleep.
	postgresReadyBudget = 180 * time.Second

	// redisReadyBudget restores the library default that the redis module
	// overrides down to 10s. See redisWaitStrategy. It is deliberately left
	// at 60s rather than raised alongside postgres: 60s is the value this
	// phase measured and shipped for redis, far fewer redis containers are
	// started per run, and raising an unmeasured bound on a guess is how a
	// budget stops meaning anything.
	redisReadyBudget = 60 * time.Second
)

// DiscardLogger is the *slog.Logger every helper here passes to production
// code that demands one.
//
// It uses the standard library's slog.DiscardHandler rather than a
// hand-constructed text handler over io.Discard, which is what the four copies
// of this helper did. internal/arch's
// TestOnlyLoggingPackageConstructsBareSlogHandlers forbids constructing a bare
// slog handler in any non-test file under internal/, and this file — test-only
// in purpose but a plain .go file — is covered by that guard. Using the
// standard library's discard handler satisfies it by not constructing a
// handler at all, which is the honest resolution rather than an exemption.
//
// Note that the guard is a TEXTUAL scan of file contents, so even naming the
// forbidden constructors in a comment here would fail it. That is a real
// weakness of the guard rather than of this file, and it is why this comment
// describes them instead of spelling them.
func DiscardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// postgresWaitStrategy REPLACES tcpostgres.BasicWaitStrategies, reproducing
// both of its checks and giving them postgresReadyBudget instead of the
// library default 60s.
//
// It must REPLACE, and it must replace with the DEADLINE form. Three separate
// traps, all of which look correct in a diff and none of which works:
//
//   - testcontainers.WithAdditionalWaitStrategy APPENDS. BasicWaitStrategies
//     is itself an Additional (modules/postgres@v0.44.0/wait_strategies.go:14),
//     so appending a generous check leaves the 60s one in place, both must
//     pass, and the 60s one still fails.
//   - testcontainers.WithWaitStrategy replaces the strategies but hardcodes a
//     60-SECOND OUTER DEADLINE over all of them
//     (testcontainers-go@v0.44.0/options.go:395-397 delegates to
//     WithWaitStrategyAndDeadline(60*time.Second, …), which builds
//     wait.ForAll(strategies...).WithDeadline(deadline)). A 180s
//     WithStartupTimeout on the port check underneath a 60s ForAll deadline
//     is capped at 60s and changes nothing. Only
//     WithWaitStrategyAndDeadline actually raises the bound.
//   - Dropping the double-occurrence log check would be worse than the flake.
//     Postgres restarts itself once during first-time initialisation, so
//     ForLog(...).WithOccurrence(2) is what distinguishes "ready" from "ready
//     and about to bounce". A DSN handed out after the FIRST readiness line
//     points at a database that is about to go away, and the resulting
//     failure is a connection reset in an unrelated test.
//
// Both checks below are therefore transcribed from BasicWaitStrategies
// verbatim, in its order, with only the budget changed.
func postgresWaitStrategy() testcontainers.CustomizeRequestOption {
	return testcontainers.WithWaitStrategyAndDeadline(postgresReadyBudget,
		// Twice: the server restarts itself after first-time init.
		wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		// Then wait for Docker to actually serve the port on localhost.
		wait.ForListeningPort("5432/tcp").WithStartupTimeout(postgresReadyBudget),
	)
}

// StartPostgresUnmigrated boots a TimescaleDB container and returns its DSN
// with NO migrations applied.
//
// It is what the migration tests themselves need — a virgin database is the
// only thing `up`, `down --all`, `up` can be asserted against — and what the
// scheduler's advisory-lock tests need, since those touch no table at all.
// Everything else wants StartPostgres.
func StartPostgresUnmigrated(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase(fixtureDB),
		tcpostgres.WithUsername(fixtureUser),
		tcpostgres.WithPassword(fixturePassword),
		tcpostgres.WithSQLDriver("pgx"),
		postgresWaitStrategy(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return dsn
}

// StartPostgres boots a TimescaleDB container, applies every migration, and
// returns its DSN. It is the entry point for repository and acceptance tests.
func StartPostgres(t *testing.T) string {
	t.Helper()

	dsn := StartPostgresUnmigrated(t)
	require.NoError(t, postgres.MigrateUp(context.Background(), dsn, DiscardLogger()),
		"migrating the fixture database")
	return dsn
}

// redisWaitStrategy REPLACES the wait strategy that testcontainers' redis
// module installs by default.
//
// That module hardcodes a 10-second budget on the port check
// (modules/redis@v0.44.0/redis.go:72,
// `wait.ForListeningPort(redisPort).WithStartupTimeout(time.Second*10)`),
// whereas the postgres module leaves wait.ForListeningPort on the library
// default of 60s (modules/postgres@v0.44.0/wait_strategies.go:24). That
// asymmetry is the whole flake: each poll of the port check issues a
// `GET /containers/<id>/json` Docker API call, and under
// `go test ./... -tags=integration -race -count=3` the WSL2 Docker Desktop
// daemon serves those slowly enough that 93 retries exhausted the 10s budget
// while redis had ALREADY logged "Ready to accept connections". The container
// was healthy; only the readiness probe's budget was too small. Restoring the
// library default 60s here aligns redis with postgres.
//
// It must REPLACE rather than append. testcontainers.WithAdditionalWaitStrategy
// would leave the module's own 10-second strategy in place, both would have to
// pass, and the 10s one would still fail — the change would look right in a
// diff and fix nothing.
//
// The DEADLINE form is used, for the reason spelled out on
// postgresWaitStrategy: plain WithWaitStrategy pins a 60s deadline over the
// whole set regardless of what the individual strategies are given, so the
// per-strategy timeout alone is not the bound. Here the two happen to agree at
// 60s, which is precisely why writing it out matters — the next person to
// raise this number needs to see which knob actually binds.
//
// Consolidating this into one function is also a fix in its own right:
// internal/scheduler/scheduler_integration_test.go called tcredis.Run with no
// override at all and was carrying the original 10-second budget.
func redisWaitStrategy() testcontainers.CustomizeRequestOption {
	return testcontainers.WithWaitStrategyAndDeadline(redisReadyBudget,
		wait.ForListeningPort("6379/tcp").WithStartupTimeout(redisReadyBudget),
		wait.ForLog("* Ready to accept connections"),
	)
}

// StartRedis boots a redis container and returns its URI.
//
// See redisWaitStrategy for the replaced-not-appended wait strategy this
// depends on, and why.
func StartRedis(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, redisImage, redisWaitStrategy())
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	uri, err := container.ConnectionString(ctx)
	require.NoError(t, err)
	return uri
}

// RedisConfig is StartRedis for callers that want a ready-made config.Redis:
// the queue and cache databases are deliberately DIFFERENT, so a test that
// mixes them up fails rather than silently sharing one keyspace.
func RedisConfig(t *testing.T) config.Redis {
	t.Helper()
	return config.Redis{URL: StartRedis(t), CacheDB: 0, QueueDB: 1}
}

// NewPool opens a connection pool against dsn with the settings every
// integration test wants: a small connection count, and a statement timeout
// short enough that a runaway query fails the test instead of hanging the
// suite until the CI job's own timeout fires.
func NewPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()

	pool, err := postgres.NewPool(context.Background(), config.DB{
		URL:              dsn,
		MaxConns:         4,
		MinConns:         1,
		MaxConnLifetime:  time.Hour,
		StatementTimeout: 10 * time.Second,
	}, DiscardLogger())
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// NewMigratedPool is StartPostgres plus NewPool: the one call a repository
// test needs before it can do anything.
func NewMigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return NewPool(t, StartPostgres(t))
}
