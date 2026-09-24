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
	"hash/fnv"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"os"
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

	// isolatedTestDSNEnv opts the isolated-database path (isolatedRoot,
	// isolatedTemplate, NewIsolatedDB, NewEmptyDB) into a long-lived,
	// externally managed Postgres/TimescaleDB server — e.g. the one
	// `make test-db-up` starts — instead of every test binary booting and
	// owning its own container (F4 Task 0).
	//
	// Set to a superuser DSN reachable from every test process that wants to
	// share it. Unset — the default, and what CI runs today — every call
	// behaves exactly as before this task: isolatedRoot boots its own
	// container, scoped to this one process. Only the isolated-database
	// path reads this variable; see StartPostgresUnmigrated's doc comment
	// for why StartPostgres/StartPostgresUnmigrated/NewMigratedPool do not.
	isolatedTestDSNEnv = "EKOKOD_TEST_PG_DSN"
)

// isolatedConnBoundArgs raises the shared isolatedRoot container's
// max_connections and timescaledb.max_background_workers past the image's
// defaults (final-review-B-report.md §"t.Parallel readiness", item 3).
//
// Each database NewIsolatedDB clones costs up to config.DB.MaxConns=4 pool
// connections (see NewPool), one TimescaleDB per-database scheduler backend
// and one root connection used for the clone/drop dance itself. The image's
// default max_connections=100 and Timescale's default
// max_background_workers cap safe fan-out at roughly 15 concurrent
// databases — far below `-parallel 8` run against every scoped repository
// package at once, let alone the 24-way stress in
// TestNewIsolatedDBIsSafeUnderConcurrentCalls. These are passed as extra
// `-c` arguments appended (not replacing — see postgresWaitStrategy's own
// note on REPLACE-vs-APPEND traps) to the module's own
// `postgres -c fsync=off`, so fsync stays disabled for test speed and only
// these two knobs are added.
var isolatedConnBoundArgs = []string{
	"-c", "max_connections=300",
	"-c", "timescaledb.max_background_workers=64",
}

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
//
// F4 TASK 0 DECISION (does not read isolatedTestDSNEnv, unlike
// isolatedRoot): this function, StartPostgres and NewMigratedPool keep
// booting their OWN dedicated container regardless of the shared-server env
// var. internal/scheduler's elector_integration_test.go and
// scheduler_integration_test.go — both callers of this function — run
// leader-election over a session-level Postgres advisory lock
// (scheduler.LockKeyScheduler) and, to prove a dropped connection is
// noticed, query `pg_locks where locktype = 'advisory'` / `pg_stat_activity`
// with NO scoping and pg_terminate_backend whatever pid they find. Advisory
// locks and pg_locks/pg_stat_activity are SERVER-wide, not per-database:
// on a server shared with unrelated concurrently-running test binaries,
// that query could match — and that terminate could kill — a completely
// unrelated test's backend. A dedicated container removes the shared
// namespace those tests rely on being alone in, which no per-database
// isolation (a clone, a fresh empty DB) can substitute for. Since all three
// functions share this one entry point, and migrations_f2_integration_test.go
// (the other StartPostgresUnmigrated caller) would have been safe to move,
// splitting the safe caller onto NewEmptyDB is left to a future task rather
// than done opportunistically here — no test in this task's scope
// regressed by leaving it alone, and this task's job is the shared-server
// path, not a rewrite of every caller.
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
// startup cost for what could instead be a fresh DATABASE inside one shared
// container. NewIsolatedDB keeps this call's shape but reuses one container
// for the whole test binary and gives every call its own database inside
// it, which is also what lets `go test -count=3` repeat a test safely: the
// same test body run three times gets three fresh, independent databases,
// not the same one three times over.
func NewMigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return NewPool(t, StartPostgres(t))
}

// isolatedTemplateNamePrefix names the database NewIsolatedDB clones on
// every call. The full name (isolatedTemplateName) appends
// postgres.MigrationsFingerprint — see that function's call site in
// isolatedTemplateName for why: a name-only constant was fine when every
// test binary owned its own container, since a stale template could only
// ever be this SAME process's own prior migration set, but a template built
// under isolatedTestDSNEnv lives on a server other processes (a different
// branch's build, a stale `make test-db-up` left running) may also have
// touched, and a name that does not change when the migrations do would
// let one of those reuse a template that no longer matches this build's
// schema.
const isolatedTemplateNamePrefix = "ekokod_isolated_template_"

// isolatedTemplateName returns the full name of the database NewIsolatedDB
// migrates once (see ensureIsolatedTemplate) and clones on every call.
func isolatedTemplateName() (string, error) {
	fingerprint, err := postgres.MigrationsFingerprint()
	if err != nil {
		return "", fmt.Errorf("compute migrations fingerprint for the isolated template name: %w", err)
	}
	return templateNameForFingerprint(fingerprint), nil
}

// templateNameForFingerprint is isolatedTemplateName's pure part, split out
// so a unit test can prove "a changed migration hash yields a different
// template name" directly, without needing a database connection or the
// real embedded migrations.
func templateNameForFingerprint(fingerprint string) string {
	return isolatedTemplateNamePrefix + fingerprint
}

// isolatedProcessSalt tags every clone/empty database this PROCESS creates.
//
// isolatedDBSeq alone is only unique WITHIN one process's counter, which
// was enough when every test binary owned its own container — no other
// process ever saw its names. Under isolatedTestDSNEnv, several test
// binaries (different packages, `go test -p=N`) share one server, each
// starting isolatedDBSeq back at 1, so two of them would otherwise both try
// to create "isolated_db_1" and one loses with a duplicate-name error. The
// salt is computed once per process (pid, cheap and always available, plus
// a random value in case two processes share a pid across time on a
// short-lived CI runner) and folded into every name below it.
var isolatedProcessSalt = fmt.Sprintf("%d_%d", os.Getpid(), rand.Int64N(1_000_000_000))

// isolatedProcessTag is isolatedProcessSalt, folded down to 8 hex chars via
// FNV-1a, for the ONE name that cannot afford the full salt's length:
// ensureIsolatedTemplate's temporary build name is built on top of the
// FULL template name (prefix + 16-hex fingerprint, 41 bytes already), and
// Postgres silently TRUNCATES identifiers past 63 bytes (NAMEDATALEN=64) —
// appending "_build_" plus the full 17-ish byte isolatedProcessSalt would
// overflow that limit and risk two different processes' temp names
// truncating to the same bytes, defeating the whole point of salting them.
// 8 hex chars keeps every name this package constructs comfortably under
// the limit while still making two processes' temp names collide only by
// a 1-in-2^32 coincidence, same as any other hash collision this package
// already accepts (e.g. templateLockKey).
var isolatedProcessTag = func() string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(isolatedProcessSalt))
	return fmt.Sprintf("%08x", h.Sum32())
}()

// isolatedRetryAttempts and isolatedRetryBaseDelay bound the retry loop
// terminateSessionsAndRetry runs. See that function for why any retry is
// needed at all, and for why the bound below is measured in SECONDS, not
// milliseconds, despite the terminate-then-act pair costing under 150ms in
// the overwhelmingly common case where nothing races it.
//
// A RETRY, not the common path, is the one case that can be slow: it is
// only reached when the action still failed with 55006 immediately after a
// terminate, meaning a replacement backend reconnected in that gap — and
// Postgres's own createdb/dropdb do not fail fast against a database that
// is still in use. They poll internally and only report 55006 after
// roughly five seconds (measured directly; see terminateSessionsAndRetry).
// isolatedRetryAttempts=10 therefore bounds a retry-exhausted call at
// roughly 10 * 5s = 50s, not the sub-second reading a naive
// 10 * isolatedRetryBaseDelay would suggest — the jittered delay only ever
// elapses BETWEEN a failed attempt and the next terminate, never instead of
// the ~5s the failed attempt itself already cost.
//
// Raised from 3 (final-review-B-report.md §"t.Parallel readiness", item 1):
// the template is now protected (see isolatedTemplate) so the template side
// of this race is closed structurally, but a CLONE — which stays
// connectable for the pool's own lifetime — can still pick up its own
// TimescaleDB scheduler backend before dropIsolatedDB runs, and `-parallel`
// fan-out widens that window. Ten attempts with jitter is the belt on top
// of DROP DATABASE ... WITH (FORCE) being the primary fix for that path.
const (
	isolatedRetryAttempts  = 10
	isolatedRetryBaseDelay = 20 * time.Millisecond
)

// isolatedRetryJitteredDelay returns isolatedRetryBaseDelay scaled up by
// attempt (capped) with up to 50% random jitter added, so that many
// concurrent callers retrying at once do not all retry in lock-step and
// collide with each other again.
func isolatedRetryJitteredDelay(attempt int) time.Duration {
	shift := attempt
	if shift > 6 {
		shift = 6 // cap growth at 64x base, ~1.28s
	}
	base := isolatedRetryBaseDelay << shift
	return base + time.Duration(rand.Int64N(int64(base)/2+1))
}

// cloneMu serialises every call to cloneIsolatedDB. CREATE DATABASE …
// TEMPLATE takes a lock tied to the template it reads, so concurrent clones
// of the SAME template under high `-parallel` fan-out contend on that lock
// rather than genuinely running in parallel — final-review-B-report.md
// measured serialising at ~120ms per clone, which is cheaper than the lock
// contention it removes. Cheap enough, and rare enough (NewEmptyDB's plain
// `CREATE DATABASE` with no TEMPLATE clause never touches this lock, but
// shares the mutex anyway for one simple, load-bearing invariant: at most
// one CREATE DATABASE runs against isolatedRoot's container at a time).
var cloneMu sync.Mutex

var (
	// isolatedContainerOnce guards booting the ONE TimescaleDB container a
	// test binary's NewIsolatedDB calls share. It is package-level state,
	// deliberately: "one container per test binary" means one per process,
	// and a process links this package once no matter how many test files
	// in the package call NewIsolatedDB.
	isolatedContainerOnce sync.Once
	isolatedRootDSN       string
	isolatedContainerErr  error

	// isolatedTemplateOnce guards this PROCESS building/finding the template
	// exactly once. It is not what makes the template safe across
	// PROCESSES sharing isolatedTestDSNEnv's server — ensureIsolatedTemplate's
	// advisory lock does that; this only avoids every one of this process's
	// own NewIsolatedDB calls redoing the check.
	isolatedTemplateOnce  sync.Once
	isolatedTemplateErr   error
	isolatedTemplateDBVal string

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
// The per-call database is not migrated from scratch: the shared template
// (see isolatedTemplateName and ensureIsolatedTemplate) is migrated once —
// once per binary normally, or once total for however many binaries share a
// server under isolatedTestDSNEnv — and every call clones it with
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
	templateName := isolatedTemplate(t, rootDSN)

	name := fmt.Sprintf("isolated_db_%s_%d", isolatedProcessSalt, isolatedDBSeq.Add(1))
	require.NoError(t, cloneIsolatedDB(ctx, rootDSN, name, templateName), "cloning the isolated-database template")

	// Registered BEFORE calling NewPool, and that order is load-bearing.
	// t.Cleanup runs LAST REGISTERED FIRST, so this runs AFTER NewPool's own
	// t.Cleanup(pool.Close) below — DROP DATABASE fails while this call's
	// own pool still holds a connection open against it.
	t.Cleanup(func() { dropIsolatedDB(t, rootDSN, name) })

	// Also here, not only in the template: an existing template (same migrations
	// fingerprint) is reused as it was built, and a clone that kept a policy would
	// race the scheduler again.
	require.NoError(t, dropRefreshPolicies(ctx, withDatabase(rootDSN, name)), "removing refresh policies from the clone")

	return NewPool(t, withDatabase(rootDSN, name))
}

// NewEmptyDB returns the DSN of a brand-new, COMPLETELY UNMIGRATED database
// inside the ONE shared TimescaleDB container NewIsolatedDB itself boots
// (see isolatedRoot) — the same container reuse NewIsolatedDB gives every
// scoped-repository test, but without cloning the shared template, for a
// caller that needs to run its OWN migrate up/down/round-trip against a
// virgin schema rather than observe one that is already migrated.
//
// This is I7b (final-review-B-report.md §"t.Parallel readiness", item 2):
// StartPostgresUnmigrated boots a fresh CONTAINER per call, which is right
// for a genuine container-level test (there are none among the migration
// tests today) but wasteful for the common case — "run `up`, assert
// something, maybe `down --all`, assert again" — which only ever needed a
// fresh DATABASE. `CREATE DATABASE name` with no TEMPLATE clause clones
// template1, the same base state a container's own initial database
// (fixtureDB) starts from, so the schema a caller sees is identical to what
// StartPostgresUnmigrated gave it; only the container is shared now, not
// booted again.
//
// It returns a DSN, not a pool — like StartPostgresUnmigrated, and unlike
// NewIsolatedDB — because its callers pass the DSN straight to
// postgres.MigrateUp/MigrateDownAll themselves; a database with no schema
// has nothing a pool's own queries could target yet.
func NewEmptyDB(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	rootDSN := isolatedRoot(t)
	name := fmt.Sprintf("isolated_empty_%s_%d", isolatedProcessSalt, isolatedDBSeq.Add(1))

	func() {
		cloneMu.Lock()
		defer cloneMu.Unlock()

		root, err := pgx.Connect(ctx, rootDSN)
		require.NoError(t, err, "connecting to create an empty isolated database")
		defer func() { _ = root.Close(ctx) }()

		ident := pgx.Identifier{name}.Sanitize()
		_, err = root.Exec(ctx, "create database "+ident)
		require.NoError(t, err, "creating an empty isolated database")
	}()

	t.Cleanup(func() { dropIsolatedDB(t, rootDSN, name) })

	return withDatabase(rootDSN, name)
}

// isolatedRoot returns the DSN of the one TimescaleDB server every
// NewIsolatedDB call in this test binary shares.
//
// If isolatedTestDSNEnv is set, that DSN is returned directly and NO
// container is booted here: the server is externally managed (e.g.
// `make test-db-up`) and may be shared with OTHER test binaries running
// concurrently. Everything downstream of this call (isolatedTemplate's
// advisory lock, isolatedProcessSalt in clone/empty names) exists because
// of that sharing.
//
// Otherwise this boots its own container, exactly as before F4 Task 0. It
// deliberately does NOT call t.Cleanup(container.Terminate), unlike
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
		if dsn := os.Getenv(isolatedTestDSNEnv); dsn != "" {
			isolatedRootDSN = dsn
			return
		}

		ctx := context.Background()
		container, err := tcpostgres.Run(ctx, postgresImage,
			tcpostgres.WithDatabase(fixtureDB),
			tcpostgres.WithUsername(fixtureUser),
			tcpostgres.WithPassword(fixturePassword),
			tcpostgres.WithSQLDriver("pgx"),
			postgresWaitStrategy(),
			// APPENDS to the module's own `postgres -c fsync=off` (WithCmd
			// would replace it) — see isolatedConnBoundArgs.
			testcontainers.WithCmdArgs(isolatedConnBoundArgs...),
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

// isolatedTemplate ensures the shared template exists inside the container
// at rootDSN, once per test binary, and returns its name.
//
// The process-local sync.Once here is a fast path only — every one of THIS
// process's own calls after the first return instantly. It is NOT what
// makes the template safe when isolatedTestDSNEnv points several different
// test binaries at the same server: a sync.Once in one process cannot stop
// another process's sync.Once from also deciding the template does not
// exist yet and racing to build it. ensureIsolatedTemplate's advisory lock
// is what makes that safe; see its doc comment.
func isolatedTemplate(t *testing.T, rootDSN string) string {
	t.Helper()
	isolatedTemplateOnce.Do(func() {
		name, err := isolatedTemplateName()
		if err != nil {
			isolatedTemplateErr = err
			return
		}
		isolatedTemplateDBVal = name
		isolatedTemplateErr = ensureIsolatedTemplate(context.Background(), rootDSN, name)
	})
	require.NoError(t, isolatedTemplateErr, "migrating and protecting the shared isolated-database template")
	return isolatedTemplateDBVal
}

// templateLockKey derives a stable pg_advisory_lock key from a template
// name, via FNV-1a folded into int64 (pg_advisory_lock's argument type).
// Different migration fingerprints get different keys, so building two
// different templates (e.g. two branches' migration sets, on the same
// shared server) never serialises against each other for no reason — only
// concurrent attempts to build the SAME template do.
func templateLockKey(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return int64(h.Sum64()) //nolint:gosec // deliberate: any bit pattern is a valid pg_advisory_lock key, signedness is irrelevant
}

// templateExists reports whether a database named name exists on the
// server conn is connected to.
func templateExists(ctx context.Context, conn *pgx.Conn, name string) (bool, error) {
	var exists bool
	err := conn.QueryRow(ctx, `select exists(select 1 from pg_database where datname = $1)`, name).Scan(&exists)
	return exists, err
}

// ensureIsolatedTemplate makes database name exist on the server at
// rootDSN, fully migrated and PROTECTED (ALLOW_CONNECTIONS false,
// IS_TEMPLATE true — see below), building it if it does not already exist.
//
// Safe to call concurrently, from multiple connections and multiple
// PROCESSES sharing one server (isolatedTestDSNEnv is exactly this): it
// takes a session-level pg_advisory_lock keyed on name (templateLockKey)
// before doing anything else, so at most one caller anywhere ever builds a
// given template, and every other caller blocks until the lock is free —
// then RE-CHECKS existence before touching anything, so a caller that lost
// the race to build finds the template already there and returns
// immediately instead of building a second one or erroring.
//
// A HALF-BUILT TEMPLATE MUST NEVER BE TRUSTED: if this process is killed
// (or the connection drops) partway through migrating, the advisory lock
// releases with the connection and a later caller must not mistake the
// wreckage for a finished template. This is why the build happens under a
// TEMPORARY name (isolatedTemplateNamePrefix collision-free per attempt —
// see the pid+random suffix below) and only the temp database is renamed to
// the final name — a single, atomic catalog operation — once it is fully
// migrated AND protected. Nothing but a fully-built, fully-protected
// database is ever visible under name, so templateExists (and every
// re-check above) can trust a positive result unconditionally: an
// in-progress build is invisible under that name by construction, not by
// convention.
func ensureIsolatedTemplate(ctx context.Context, rootDSN, name string) error {
	root, err := pgx.Connect(ctx, rootDSN)
	if err != nil {
		return fmt.Errorf("connect to ensure the isolated template: %w", err)
	}
	defer func() { _ = root.Close(ctx) }()

	key := templateLockKey(name)
	if _, err := root.Exec(ctx, `select pg_advisory_lock($1)`, key); err != nil {
		return fmt.Errorf("acquire the isolated-template advisory lock: %w", err)
	}
	// Session-level: releasing explicitly (rather than relying only on
	// root.Close above) means a caller that reuses this same *pgx.Conn for
	// something else later — none does today, but the alternative is a lock
	// held for the rest of the connection's life by accident — is never
	// surprised by it.
	defer func() {
		_, _ = root.Exec(context.Background(), `select pg_advisory_unlock($1)`, key)
	}()

	// Re-check AFTER acquiring the lock: another caller (this process's
	// first NewIsolatedDB call, or a different process entirely) may have
	// built and protected the template while this call was blocked waiting
	// for the lock.
	exists, err := templateExists(ctx, root, name)
	if err != nil {
		return fmt.Errorf("check whether the isolated template already exists: %w", err)
	}
	if exists {
		return nil
	}

	tempName := fmt.Sprintf("%s_build_%s", name, isolatedProcessTag)
	tempIdent := pgx.Identifier{tempName}.Sanitize()

	if _, err := root.Exec(ctx, "create database "+tempIdent); err != nil {
		return fmt.Errorf("create the temporary isolated-template build database: %w", err)
	}
	built := false
	defer func() {
		if !built {
			// Best-effort: a leftover half-built temp database under a
			// process-salted name never collides with a later attempt (see
			// the name above), so leaving it for the reaper/next test-db-up
			// is litter, not a correctness problem.
			_, _ = root.Exec(context.Background(), "drop database if exists "+tempIdent+" with (force)")
		}
	}()

	if err := postgres.MigrateUp(ctx, withDatabase(rootDSN, tempName), DiscardLogger()); err != nil {
		return fmt.Errorf("migrate the isolated-template build database: %w", err)
	}

	if err := dropRefreshPolicies(ctx, withDatabase(rootDSN, tempName)); err != nil {
		return err
	}

	// Terminate whatever migrating may already have caused to spawn (e.g. a
	// scheduler backend reacting to a policy the LAST migration added),
	// THEN make the template non-connectable, THEN terminate once more to
	// catch anything that raced into the gap between those two statements —
	// belt AND suspenders, because this is the one-time setup every later
	// clone's safety depends on. See isolatedTemplateNamePrefix's original
	// note (I7a) for why ALLOW_CONNECTIONS false removes the 55006 race
	// entirely rather than merely narrowing it.
	terminateSQL := `select pg_terminate_backend(pid) from pg_stat_activity where datname = $1 and pid <> pg_backend_pid()`
	if _, err := root.Exec(ctx, terminateSQL, tempName); err != nil {
		return fmt.Errorf("terminate sessions on the isolated-template build database: %w", err)
	}
	if _, err := root.Exec(ctx,
		fmt.Sprintf("alter database %s with allow_connections false is_template true", tempIdent)); err != nil {
		return fmt.Errorf("protect the isolated-template build database: %w", err)
	}
	if _, err := root.Exec(ctx, terminateSQL, tempName); err != nil {
		return fmt.Errorf("terminate sessions on the isolated-template build database after protecting it: %w", err)
	}

	// The rename is the single moment the template becomes visible under
	// its final name — fully migrated and fully protected, never before.
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := root.Exec(ctx, fmt.Sprintf("alter database %s rename to %s", tempIdent, ident)); err != nil {
		return fmt.Errorf("rename the isolated-template build database into place: %w", err)
	}
	built = true
	return nil
}

// dropRefreshPolicies deletes 00005's continuous-aggregate refresh policies in
// the template every clone inherits. A policy job fires on the container's own
// schedule, seconds after a clone appears, and materialises whatever window it
// happens to catch: the same read then answers from a materialised bucket in one
// run and from an R94-composed one in the next — and the two legitimately differ
// by the boundary step. Tests that want a materialised bucket refresh it
// themselves (`call refresh_continuous_aggregate(...)`), so removing the
// scheduled jobs only removes the race, never a behaviour under test.
func dropRefreshPolicies(ctx context.Context, dsn string) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect to the isolated-template build database: %w", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	const sql = `do $$
declare job record;
begin
  for job in select job_id from timescaledb_information.jobs where proc_name = 'policy_refresh_continuous_aggregate' loop
    perform delete_job(job.job_id);
  end loop;
end $$`
	if _, err := conn.Exec(ctx, sql); err != nil {
		return fmt.Errorf("remove continuous-aggregate refresh policies from the isolated template: %w", err)
	}
	return nil
}

// cloneIsolatedDB creates database name inside the container at rootDSN as a
// `CREATE DATABASE … TEMPLATE templateName`.
//
// Serialised behind cloneMu — see that variable's doc comment — and still
// wrapped in terminateSessionsAndRetry as a belt: the template itself is
// protected by isolatedTemplate before this is ever reachable, so the
// terminate below is normally a no-op against an empty result set, cheap
// insurance rather than the load-bearing defence it used to be.
func cloneIsolatedDB(ctx context.Context, rootDSN, name, templateName string) error {
	cloneMu.Lock()
	defer cloneMu.Unlock()

	root, err := pgx.Connect(ctx, rootDSN)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close(ctx) }()

	ident := pgx.Identifier{name}.Sanitize()
	templateIdent := pgx.Identifier{templateName}.Sanitize()
	return terminateSessionsAndRetry(ctx, root, templateName, func() error {
		_, err := root.Exec(ctx, fmt.Sprintf("create database %s template %s", ident, templateIdent))
		return err
	})
}

// dropIsolatedDB drops database name inside the container at rootDSN. Per
// the pool this call's own t.Cleanup ordering already closed, this runs
// with no session of its own open against name — but TimescaleDB's
// per-database background worker scheduler may still hold one, exactly as
// it used to against the template (see isolatedTemplate), because a CLONE
// (unlike the protected template) stays connectable for its pool's whole
// lifetime and so can pick up a scheduler backend of its own.
//
// Uses DROP DATABASE … WITH (FORCE) (PG13+; this image is pg16), which
// disconnects other sessions as part of the drop itself, atomically,
// instead of this package's own terminate-then-act race against whatever
// reconnects in between — the same class of fix as isolatedTemplate's
// ALLOW_CONNECTIONS false, applied where a permanent "never connectable"
// state does not fit because the clone GENUINELY needs to be connectable
// while its test runs.
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
		_, err := root.Exec(ctx, "drop database if exists "+ident+" with (force)")
		return err
	})
	if err != nil {
		t.Logf("NewIsolatedDB: could not drop %s, leaving it for the container's reaper: %v", name, err)
	}
}

// terminateSessionsAndRetry terminates every session against dbName other
// than root's own, then runs action; if action still fails with Postgres
// SQLSTATE 55006 ("source database … is being accessed by other users"), it
// retries the terminate-then-action pair up to isolatedRetryAttempts times
// with jittered backoff (isolatedRetryJitteredDelay).
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
// issuing CREATE DATABASE … TEMPLATE against a database that scheduler is
// still connected to does not fail fast. Postgres's own createdb keeps
// polling for the other backend to go away and only reports 55006 after
// roughly five seconds — call it once per NewIsolatedDB call and every
// call pays that five seconds. Terminating unconditionally first, then
// calling action, measured under 150ms consistently (see the Task 8c
// report); the retry loop below exists only for the rare case where a
// replacement backend reconnects in the gap between the terminate and the
// action, not as the primary path — and for cloneIsolatedDB specifically,
// isolatedTemplate having already made the template non-connectable means
// that gap can no longer be reached at all (see that function).
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
		time.Sleep(isolatedRetryJitteredDelay(attempt))
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

// SharedRedisConfig points at the long-lived test Redis when
// EKOKOD_TEST_REDIS_URL is set (make test-redis-up), else boots a container.
// Callers must namespace their keys: the shared server is not flushed.
func SharedRedisConfig(t *testing.T) config.Redis {
	t.Helper()
	if url := os.Getenv("EKOKOD_TEST_REDIS_URL"); url != "" {
		return config.Redis{URL: url, CacheDB: 0, QueueDB: 1}
	}
	return RedisConfig(t)
}
