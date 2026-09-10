//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/stretchr/testify/require"
)

func TestMigrateUpDownUp(t *testing.T) {
	ctx := context.Background()
	dsn := testfixtures.StartPostgresUnmigrated(t)
	log := testfixtures.DiscardLogger()

	require.NoError(t, postgres.MigrateUp(ctx, dsn, log))

	pending, err := postgres.PendingMigrations(ctx, dsn)
	require.NoError(t, err)
	require.Zero(t, pending)

	require.NoError(t, postgres.MigrateDownAll(ctx, dsn, log))

	pending, err = postgres.PendingMigrations(ctx, dsn)
	require.NoError(t, err)
	require.Positive(t, pending, "after down --all every migration is pending again")

	require.NoError(t, postgres.MigrateUp(ctx, dsn, log))
	pending, err = postgres.PendingMigrations(ctx, dsn)
	require.NoError(t, err)
	require.Zero(t, pending)
}

func TestTimescaleExtensionIsInstalled(t *testing.T) {
	ctx := context.Background()
	dsn := testfixtures.StartPostgresUnmigrated(t)
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 4, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second}, testfixtures.DiscardLogger())
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	var version string
	require.NoError(t, pool.QueryRow(ctx,
		`select extversion from pg_extension where extname = 'timescaledb'`).Scan(&version))
	require.NotEmpty(t, version)
}

func TestPoolCheckReportsHealth(t *testing.T) {
	ctx := context.Background()
	dsn := testfixtures.StartPostgresUnmigrated(t)

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 4, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second}, testfixtures.DiscardLogger())
	require.NoError(t, err)

	check := postgres.PoolCheck(pool)
	require.Equal(t, "database", check.Name)

	require.NoError(t, check.Fn(ctx))
	pool.Close()
	require.Error(t, check.Fn(ctx), "a closed pool must report unhealthy")
}

func TestMigrationsCheckReportsPendingThenCurrent(t *testing.T) {
	ctx := context.Background()
	dsn := testfixtures.StartPostgresUnmigrated(t)
	log := testfixtures.DiscardLogger()

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 4, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second}, log)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	check := postgres.MigrationsCheck(pool)
	require.Equal(t, "migrations", check.Name)

	require.Error(t, check.Fn(ctx), "no migration has run yet, so the schema must report pending")

	require.NoError(t, postgres.MigrateUp(ctx, dsn, log))
	require.NoError(t, check.Fn(ctx), "the schema is current once every migration has been applied")

	require.NoError(t, postgres.MigrateDownAll(ctx, dsn, log))
	require.Error(t, check.Fn(ctx), "after down --all the schema must report pending again")
}

func TestStatementTimeoutIsApplied(t *testing.T) {
	ctx := context.Background()
	dsn := testfixtures.StartPostgresUnmigrated(t)

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 2, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 250 * time.Millisecond}, testfixtures.DiscardLogger())
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	_, err = pool.Exec(ctx, "select pg_sleep(2)")
	require.Error(t, err, "the configured statement timeout must cancel a long query")
}

// TestDownMigrationLeavesExtensionsInstalled pins the intent of 00001's Down
// section. The Up uses CREATE EXTENSION IF NOT EXISTS, which is a no-op when
// an extension is already present, so the migration cannot claim to have
// installed it — and an unconditional DROP on the way down would destroy
// extensions a DBA pre-provisioned, or that another schema in a shared
// database depends on, cascading through every hypertable built on
// timescaledb.
func TestDownMigrationLeavesExtensionsInstalled(t *testing.T) {
	ctx := context.Background()
	dsn := testfixtures.StartPostgresUnmigrated(t)
	log := testfixtures.DiscardLogger()

	require.NoError(t, postgres.MigrateUp(ctx, dsn, log))
	require.NoError(t, postgres.MigrateDownAll(ctx, dsn, log))

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 2, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second}, log)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	for _, ext := range []string{"timescaledb", "pgcrypto", "citext"} {
		var installed bool
		require.NoError(t, pool.QueryRow(ctx,
			`select exists(select 1 from pg_extension where extname = $1)`, ext).Scan(&installed))
		require.True(t, installed, "%s must survive migrate down --all", ext)
	}
}

// TestMigrationsCheckErrorIsScrubbed covers the one error path out of this
// package that used to escape scrubbing. It matters more than the others:
// MigrationsCheck is wired into readyHandler, which serialises the error text
// into the unauthenticated /health/ready body that the web health page then
// renders.
//
// CHANGED in task 8a. This test used to assert errors.Unwrap(err) == nil,
// treating an unwrappable error as "the observable proof that the value went
// through the scrubber". That proxy was crude, but it was doing real work:
// nothing else in the package produced an unwrappable error, so it was a
// unique fingerprint of scrubPoolErr. Making scrubbed errors traversable
// (see secret.Wrap) was correct, but it destroyed that fingerprint, and the
// first replacement written for it did not restore one — every assertion
// below is also satisfied by a naive fmt.Errorf("read schema version: %w").
//
// That gap mattered specifically here. The failure is induced with
// pool.Close(), so the driver text is "closed pool", which never contained a
// credential to begin with: the NotContains assertions below are VACUOUS on
// this path and pass whether or not any redaction ran. They are kept because
// they cost nothing and would catch a future change to how the error is
// induced, but they are not what protects the readiness body.
//
// secret.IsScrubbed is the restored fingerprint, and it is a positive one: a
// plain %w wrap cannot satisfy it. If health.go's scrubPoolErr call were
// reverted to a bare wrap tomorrow, this test fails. The redaction itself —
// mask present, credential absent, on a cause that really does carry the
// credential — is proved without a container in
// scrub_internal_test.go:TestScrubPoolErrRedactsTheCredentialOnTheReadinessPath,
// because only a synthetic cause can make that assertion non-vacuous.
//
// What this test uniquely proves is the wiring: that the REAL MigrationsCheck
// path, against a REAL database, returns a scrubbed, traversable error.
// Unwrapping deliberately yields unredacted text; only the logging and HTTP
// paths, which print err.Error(), are bound by the redaction.
func TestMigrationsCheckErrorIsScrubbed(t *testing.T) {
	ctx := context.Background()
	dsn := testfixtures.StartPostgresUnmigrated(t)

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 2, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second}, testfixtures.DiscardLogger())
	require.NoError(t, err)

	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool.Close() // every subsequent query fails outside the undefined-table path

	err = postgres.MigrationsCheck(pool).Fn(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read schema version")

	// The load-bearing assertion: this error came out of the scrubber, not
	// out of a bare fmt.Errorf("%w"). Nothing else in the package can
	// satisfy it.
	require.True(t, secret.IsScrubbed(err),
		"the readiness path must return a scrubbed error: a plain %w wrap would put the driver text, credential and all, into the unauthenticated /health/ready body")

	// Vacuous on this particular induced failure ("closed pool" carries no
	// credential), kept only as a cheap tripwire if the inducement changes.
	// The real redaction proof is the unit test named in the doc comment.
	require.NotContains(t, err.Error(), "ekokod:ekokod", "no credential may reach the readiness body")
	require.NotContains(t, err.Error(), "ekokod", "no credential fragment may reach the readiness body")

	cause := errors.Unwrap(err)
	require.Error(t, cause, "the driver error must stay reachable: errors.Is must work through a scrubbed error")
	require.Contains(t, cause.Error(), "closed pool",
		"the reachable cause must be the driver's own error, not a copy of the redacted wrapper")
	require.ErrorIs(t, err, cause, "errors.Is must traverse the scrubbed wrapper")
}
