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
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// NewMigratedPool is StartPostgres plus NewPool: a fresh container, fully
// migrated, wrapped in a pool.
//
// A repository test wants NewIsolatedDB instead. This boots one container
// PER TEST, which is what the migration round-trip and reversibility tests
// need — they require a virgin container, not a virgin database inside a
// shared one — but which made every repository test pay a fresh container's
// startup cost, and made `go test -count=3` share the same globally seeded
// data three times over. NewIsolatedDB keeps this call's shape but reuses
// one container for the whole test binary and gives every call its own
// database inside it.
func NewMigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return NewPool(t, StartPostgres(t))
}

// isolatedTemplateDB is the name of the database NewIsolatedDB migrates
// exactly once per test binary and clones on every call.
const isolatedTemplateDB = "ekokod_isolated_template"

// isolatedRetryAttempts and isolatedRetryDelay bound the retry loop
// terminateSessionsAndRetry runs. See that function for why any retry is
// needed at all.
const (
	isolatedRetryAttempts = 20
	isolatedRetryDelay    = 20 * time.Millisecond
)

var (
	// isolatedContainerOnce guards booting the ONE TimescaleDB container a
	// test binary's NewIsolatedDB calls share. It is package-level state,
	// deliberately: "one container per test binary" means one per process,
	// and a process links this package once no matter how many test files
	// in the package call NewIsolatedDB.
	isolatedContainerOnce sync.Once
	isolatedRootDSN       string
	isolatedContainerErr  error

	// isolatedTemplateOnce guards migrating isolatedTemplateDB exactly once,
	// the first time any call needs it.
	isolatedTemplateOnce sync.Once
	isolatedTemplateErr  error

	// isolatedDBSeq names every cloned database uniquely, so that concurrent
	// t.Parallel() callers in the same package never race on a name.
	isolatedDBSeq atomic.Uint64
)

// NewIsolatedDB returns a pool over a brand-new, fully migrated database,
// isolated from every other call's — in this test, in a parallel test, and
// in a repeat of this same test under `go test -count=3` — the same way a
// call to NewMigratedPool against its own fresh container always was.
//
// It differs from NewMigratedPool in what it shares: ONE TimescaleDB
// container per test BINARY, started lazily on the first call and left for
// testcontainers' own reaper rather than terminated by any one test's
// t.Cleanup (see isolatedRoot) — but a DIFFERENT, FRESH DATABASE inside that
// container on every call, dropped on that call's own t.Cleanup. Twenty
// repository tests that used to mean twenty containers on one Docker daemon,
// which is the load that exhausted postgresReadyBudget earlier in this
// phase, now mean one.
//
// The per-call database is not migrated from scratch: isolatedTemplateDB is
// migrated once per binary and every call clones it with
// `CREATE DATABASE … TEMPLATE …`, which this package measured at roughly
// 7x faster than a full migration run — about 120ms against about 850ms on
// the machine this was measured on (see the report for Task 8c). That is
// safe for THIS schema specifically because a test against a real clone
// proved three TimescaleDB-specific behaviours survive it: a hypertable
// insert still creates chunks, refresh_continuous_aggregate still
// materialises a continuous aggregate, and compress_chunk still succeeds —
// see TestIsolatedDBCloneSupportsTimescaleOperations. If a future migration
// changes what TimescaleDB keeps per-database in a way that test would
// catch, that test fails and this comment's safety claim no longer holds.
func NewIsolatedDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	rootDSN := isolatedRoot(t)
	isolatedTemplate(t, rootDSN)

	name := fmt.Sprintf("isolated_db_%d", isolatedDBSeq.Add(1))
	require.NoError(t, cloneIsolatedDB(ctx, rootDSN, name), "cloning the isolated-database template")

	// Registered BEFORE calling NewPool, and that order is load-bearing.
	// t.Cleanup runs LAST REGISTERED FIRST, so this runs AFTER NewPool's own
	// t.Cleanup(pool.Close) below — DROP DATABASE fails while this call's
	// own pool still holds a connection open against it.
	t.Cleanup(func() { dropIsolatedDB(t, rootDSN, name) })

	return NewPool(t, withDatabase(rootDSN, name))
}

// isolatedRoot returns the DSN of the one TimescaleDB container every
// NewIsolatedDB call in this test binary shares, booting it on the first
// call.
//
// It deliberately does NOT call t.Cleanup(container.Terminate), unlike
// StartPostgresUnmigrated: that Cleanup runs at the end of whichever TEST
// happened to be first to call NewIsolatedDB, not at the end of the test
// BINARY, and terminating the container there would pull it out from under
// every later test's call. Nothing in this package terminates it at all —
// testcontainers registers every container it starts with the ryuk reaper
// sidecar (see the "Creating container for image testcontainers/ryuk" line
// any of these tests already print), which removes every container in its
// session when the session's connection to it drops, i.e. when this test
// binary's process exits. `go test` exiting is what cleans this one up.
func isolatedRoot(t *testing.T) string {
	t.Helper()
	isolatedContainerOnce.Do(func() {
		ctx := context.Background()
		container, err := tcpostgres.Run(ctx, postgresImage,
			tcpostgres.WithDatabase(fixtureDB),
			tcpostgres.WithUsername(fixtureUser),
			tcpostgres.WithPassword(fixturePassword),
			tcpostgres.WithSQLDriver("pgx"),
			postgresWaitStrategy(),
		)
		if err != nil {
			isolatedContainerErr = err
			return
		}
		isolatedRootDSN, isolatedContainerErr = container.ConnectionString(ctx, "sslmode=disable")
	})
	require.NoError(t, isolatedContainerErr, "starting the shared isolated-database container")
	return isolatedRootDSN
}

// isolatedTemplate migrates isolatedTemplateDB inside the container at
// rootDSN exactly once per test binary.
func isolatedTemplate(t *testing.T, rootDSN string) {
	t.Helper()
	isolatedTemplateOnce.Do(func() {
		isolatedTemplateErr = func() error {
			ctx := context.Background()
			root, err := pgx.Connect(ctx, rootDSN)
			if err != nil {
				return err
			}
			defer func() { _ = root.Close(ctx) }()

			ident := pgx.Identifier{isolatedTemplateDB}.Sanitize()
			if _, err := root.Exec(ctx, "create database "+ident); err != nil {
				return err
			}
			return postgres.MigrateUp(ctx, withDatabase(rootDSN, isolatedTemplateDB), DiscardLogger())
		}()
	})
	require.NoError(t, isolatedTemplateErr, "migrating the shared isolated-database template")
}

// cloneIsolatedDB creates database name inside the container at rootDSN as a
// `CREATE DATABASE … TEMPLATE isolatedTemplateDB`.
func cloneIsolatedDB(ctx context.Context, rootDSN, name string) error {
	root, err := pgx.Connect(ctx, rootDSN)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close(ctx) }()

	ident := pgx.Identifier{name}.Sanitize()
	return terminateSessionsAndRetry(ctx, root, isolatedTemplateDB, func() error {
		_, err := root.Exec(ctx, fmt.Sprintf("create database %s template %s", ident, isolatedTemplateDB))
		return err
	})
}

// dropIsolatedDB drops database name inside the container at rootDSN. Per
// the pool this call's own t.Cleanup ordering already closed, this runs
// with no session of its own open against name — but TimescaleDB's
// per-database background worker scheduler (see terminateSessionsAndRetry)
// may still hold one, exactly as it does against the template, so the same
// retry applies.
//
// A failure here is logged, not failed: dropping is hygiene, not
// correctness — every database this creates lives inside a container that
// testcontainers' reaper removes in full when the process exits (see
// isolatedRoot), so a database this could not drop is not a leak past the
// test run, only litter within it.
func dropIsolatedDB(t *testing.T, rootDSN, name string) {
	t.Helper()
	ctx := context.Background()

	root, err := pgx.Connect(ctx, rootDSN)
	if err != nil {
		t.Logf("NewIsolatedDB: could not connect to drop %s, leaving it for the container's reaper: %v", name, err)
		return
	}
	defer func() { _ = root.Close(ctx) }()

	ident := pgx.Identifier{name}.Sanitize()
	err = terminateSessionsAndRetry(ctx, root, name, func() error {
		_, err := root.Exec(ctx, "drop database if exists "+ident)
		return err
	})
	if err != nil {
		t.Logf("NewIsolatedDB: could not drop %s, leaving it for the container's reaper: %v", name, err)
	}
}

// terminateSessionsAndRetry terminates every session against dbName other
// than root's own, then runs action; if action still fails with Postgres
// SQLSTATE 55006 ("source database … is being accessed by other users"), it
// retries the terminate-then-action pair up to isolatedRetryAttempts times.
//
// WHY THIS EXISTS AT ALL. Both CREATE DATABASE … TEMPLATE and DROP DATABASE
// require that nothing be connected to the database in question, and this
// package's own connections never are — but TimescaleDB adds one that is
// not this package's to close. Every database with an active job (this
// schema's compression and continuous-aggregate policies, added by
// 00005_continuous_aggregates.sql) gets its own "TimescaleDB Background
// Worker Scheduler" backend, spawned by the extension itself, independent of
// any client connection.
//
// WHY IT TERMINATES BEFORE THE FIRST ATTEMPT, not only after one fails.
// Measured directly against this image (timescale/timescaledb:2.30.0-pg16):
// issuing CREATE DATABASE … TEMPLATE against a template that scheduler is
// still connected to does not fail fast. Postgres's own createdb keeps
// polling for the other backend to go away and only reports 55006 after
// roughly five seconds — call it once per NewIsolatedDB call and every
// call pays that five seconds. Terminating unconditionally first, then
// calling action, measured under 150ms consistently (see the Task 8c
// report); the retry loop below exists only for the rare case where a
// replacement backend reconnects in the gap between the terminate and the
// action, not as the primary path.
func terminateSessionsAndRetry(ctx context.Context, root *pgx.Conn, dbName string, action func() error) error {
	for attempt := 1; ; attempt++ {
		if _, err := root.Exec(ctx,
			`select pg_terminate_backend(pid) from pg_stat_activity where datname = $1 and pid <> pg_backend_pid()`,
			dbName); err != nil {
			return err
		}
		err := action()
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55006" || attempt >= isolatedRetryAttempts {
			return err
		}
		time.Sleep(isolatedRetryDelay)
	}
}

// withDatabase returns dsn with its database path replaced by name, keeping
// every other part (host, port, credentials, query parameters) unchanged.
func withDatabase(dsn, name string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		// dsn was already parsed once by whatever produced it (a
		// container's own ConnectionString, or a prior call through this
		// same function), so a parse failure here would be this package's
		// own bug, not a runtime condition a caller could act on.
		panic(fmt.Sprintf("testfixtures: withDatabase: %v", err))
	}
	u.Path = "/" + name
	return u.String()
}
