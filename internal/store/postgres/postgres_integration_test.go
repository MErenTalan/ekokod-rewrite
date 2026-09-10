//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// startPostgres boots TimescaleDB and returns its DSN.
func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "timescale/timescaledb:2.30.0-pg16",
		tcpostgres.WithDatabase("ekokod"),
		tcpostgres.WithUsername("ekokod"),
		tcpostgres.WithPassword("ekokod"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return dsn
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestMigrateUpDownUp(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)
	log := discardLogger()

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
	dsn := startPostgres(t)
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 4, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second}, discardLogger())
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	var version string
	require.NoError(t, pool.QueryRow(ctx,
		`select extversion from pg_extension where extname = 'timescaledb'`).Scan(&version))
	require.NotEmpty(t, version)
}

func TestPoolCheckReportsHealth(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 4, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second}, discardLogger())
	require.NoError(t, err)

	check := postgres.PoolCheck(pool)
	require.Equal(t, "database", check.Name)

	require.NoError(t, check.Fn(ctx))
	pool.Close()
	require.Error(t, check.Fn(ctx), "a closed pool must report unhealthy")
}

func TestMigrationsCheckReportsPendingThenCurrent(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)
	log := discardLogger()

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
	dsn := startPostgres(t)

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 2, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 250 * time.Millisecond}, discardLogger())
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
	dsn := startPostgres(t)
	log := discardLogger()

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
// renders. scrubPoolErr flattens the driver error to redacted text rather
// than wrapping it with %w, so an unwrappable error is the observable proof
// that the value went through the scrubber.
func TestMigrationsCheckErrorIsScrubbed(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 2, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second}, discardLogger())
	require.NoError(t, err)

	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool.Close() // every subsequent query fails outside the undefined-table path

	err = postgres.MigrationsCheck(pool).Fn(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read schema version")
	require.Nil(t, errors.Unwrap(err), "the error must be flattened by scrubPoolErr, not wrapped with %%w")
	require.NotContains(t, err.Error(), "ekokod:ekokod", "no credential may reach the readiness body")
}
