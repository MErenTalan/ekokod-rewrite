//go:build integration

package testfixtures_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestNewIsolatedDBGivesEachCallItsOwnDatabase is the property every
// repository test relies on instead of a fresh container: two calls in the
// SAME container must not see each other's rows, exactly as two calls to
// NewMigratedPool (each its own container) never could.
func TestNewIsolatedDBGivesEachCallItsOwnDatabase(t *testing.T) {
	ctx := context.Background()

	first := testfixtures.NewIsolatedDB(t)
	second := testfixtures.NewIsolatedDB(t)

	tenant := testfixtures.NewTenant(t, ctx, first, 1)

	var onSecond int
	require.NoError(t, second.QueryRow(ctx,
		`select count(*) from companies where id = $1`, tenant.Company.ID).Scan(&onSecond))
	require.Zero(t, onSecond, "a row inserted through one call's pool must not be visible through another's")

	var onFirst int
	require.NoError(t, first.QueryRow(ctx, `select count(*) from companies where id = $1`, tenant.Company.ID).Scan(&onFirst))
	require.Equal(t, 1, onFirst, "the row must still be visible through the pool that inserted it")
}

// TestNewIsolatedDBSupportsTheSameSeedTwice is exactly what item 2 of the
// Part C brief calls out: NewTenant with a fixed seed collides with itself
// on the schema's unique indexes within ONE database, so a design that ever
// shared a database across calls would make this fail. Calling it against
// TWO calls' pools, with the SAME seed, must succeed on both — proof that
// each is a genuinely fresh, independent, fully migrated database rather
// than the same one reused.
func TestNewIsolatedDBSupportsTheSameSeedTwice(t *testing.T) {
	ctx := context.Background()

	a := testfixtures.NewTenant(t, ctx, testfixtures.NewIsolatedDB(t), 1)
	b := testfixtures.NewTenant(t, ctx, testfixtures.NewIsolatedDB(t), 1)

	require.Equal(t, a.Company.ID, b.Company.ID, "same seed, so same deterministic id — in DIFFERENT databases")
}

// TestIsolatedDBCloneSupportsTimescaleOperations is the fast-path proof
// NewIsolatedDB's doc comment promises: TimescaleDB keeps catalogue state
// and background workers per database, so cloning a migrated template with
// `CREATE DATABASE … TEMPLATE …` is only safe for this schema if the clone
// behaves like a directly migrated database for the three things that are
// specific to TimescaleDB rather than to plain Postgres. If any of these
// three ever regresses, NewIsolatedDB must fall back to
// `CREATE DATABASE` + postgres.MigrateUp per call, and this test is what
// would tell you to.
func TestIsolatedDBCloneSupportsTimescaleOperations(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	analyzerID := tenant.Analyzers[0].ID

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 5 {
		_, err := pool.Exec(ctx, `
			insert into meter_readings (analyzer_id, ts, kind, active_import, source_provider)
			values ($1, $2, 'load_profile', $3, 'osos')`,
			analyzerID, base.Add(time.Duration(i)*time.Hour), fmt.Sprintf("%d.0000", i*10))
		require.NoError(t, err)
	}

	// Proof 1: inserting into a hypertable still creates chunks on a clone.
	var chunks int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from timescaledb_information.chunks where hypertable_name = 'meter_readings'`,
	).Scan(&chunks))
	require.Greater(t, chunks, 0, "inserting into meter_readings created no chunk on the clone")

	// Proof 2: refresh_continuous_aggregate still materialises a row.
	_, err := pool.Exec(ctx,
		`call refresh_continuous_aggregate('consumption_hourly', $1::timestamptz, $2::timestamptz)`,
		base.Add(-24*time.Hour), base.Add(24*time.Hour))
	require.NoError(t, err, "refresh_continuous_aggregate errored on the clone")
	var materialised int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from consumption_hourly where analyzer_id = $1`, analyzerID).Scan(&materialised))
	require.Greater(t, materialised, 0, "consumption_hourly materialised no row on the clone")

	// Proof 3: compress_chunk still succeeds.
	var chunkName string
	require.NoError(t, pool.QueryRow(ctx,
		`select show_chunks::text from show_chunks('meter_readings') show_chunks limit 1`).Scan(&chunkName))
	_, err = pool.Exec(ctx, `select compress_chunk($1::regclass)`, chunkName)
	require.NoError(t, err, "compress_chunk errored on the clone")
}

// TestNewIsolatedDBIsSafeUnderConcurrentCalls exercises the concurrency
// requirement from the Part C brief directly, rather than trusting it from
// the design alone: several subtests call NewIsolatedDB in parallel, and
// under -race that catches a shared name or a shared connection actually
// racing, not merely a design that was SUPPOSED to avoid one.
func TestNewIsolatedDBIsSafeUnderConcurrentCalls(t *testing.T) {
	for i := range 6 {
		t.Run(fmt.Sprintf("parallel-%d", i), func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			pool := testfixtures.NewIsolatedDB(t)
			tenant := testfixtures.NewTenant(t, ctx, pool, int64(i))

			var companies int
			require.NoError(t, pool.QueryRow(ctx, `select count(*) from companies`).Scan(&companies))
			require.Equal(t, 1, companies, "this call's database must hold only what this call inserted")
			require.NotEmpty(t, tenant.Buildings)
		})
	}
}
