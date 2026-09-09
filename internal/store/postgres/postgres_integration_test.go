//go:build integration

package postgres_test

import (
	"context"
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
