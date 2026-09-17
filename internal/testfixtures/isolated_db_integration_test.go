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
	t.Parallel()
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
	t.Parallel()
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
//
// PROOF 2's DERIVATION (Task 8c fix round 1, Important 3). consumption_hourly
// is declared `timescaledb.materialized_only = false` (00005:113), so a
// plain `select … from consumption_hourly` unions in real-time aggregation
// over the raw hypertable REGARDLESS of whether anything was ever
// materialised — a review proved this by deleting the
// refresh_continuous_aggregate call below and watching the old version of
// this test still pass. Reading only what the refresh actually materialised
// requires flipping the clone's view to `materialized_only = true` first, so
// the numbers below are exactly what got written into the materialisation
// hypertable, nothing else.
//
// The five readings inserted below are exactly one hour apart
// (base, base+1h, …, base+4h), and consumption_hourly's own bucket is
// `time_bucket('1 hour', ts)` — so each reading lands in its OWN,
// single-reading bucket: five buckets, one row per bucket. For a
// single-reading bucket, first(active_import, ts) and last(active_import,
// ts) are the same one value, so active_index (= last(active_import, ts))
// for the base+4h bucket is exactly that reading's active_import, 40, and
// reading_count for every bucket is exactly 1. Both are asserted as EXACT
// values, not merely "> 0", which is what makes this proof non-vacuous:
// with the refresh removed, materialized_only means the row for base+4h
// does not exist at all, and the query below returns sql.ErrNoRows.
func TestIsolatedDBCloneSupportsTimescaleOperations(t *testing.T) {
	t.Parallel()
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

	// Proof 2: refresh_continuous_aggregate still materialises the EXACT
	// expected rows — see the derivation above.
	_, err := pool.Exec(ctx,
		`call refresh_continuous_aggregate('consumption_hourly', $1::timestamptz, $2::timestamptz)`,
		base.Add(-24*time.Hour), base.Add(24*time.Hour))
	require.NoError(t, err, "refresh_continuous_aggregate errored on the clone")

	// Flip to materialized_only so the query below can ONLY see what the
	// refresh actually wrote, never the real-time union.
	_, err = pool.Exec(ctx, `alter materialized view consumption_hourly set (timescaledb.materialized_only = true)`)
	require.NoError(t, err, "could not switch the clone's view to materialized_only")

	var totalBuckets int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from consumption_hourly where analyzer_id = $1`, analyzerID).Scan(&totalBuckets))
	require.Equal(t, 5, totalBuckets, "one reading per hour must materialise into five distinct hourly buckets")

	var activeIndex string
	var readingCount int
	require.NoError(t, pool.QueryRow(ctx,
		`select active_index::text, reading_count from consumption_hourly where analyzer_id = $1 and bucket = $2`,
		analyzerID, base.Add(4*time.Hour)).Scan(&activeIndex, &readingCount))
	require.Equal(t, "40.0000", activeIndex, "the base+4h bucket's only reading has active_import=40")
	require.Equal(t, 1, readingCount, "a single-reading bucket must report reading_count=1")

	// Proof 3: compress_chunk still succeeds, and the chunk is ACTUALLY
	// compressed afterwards — not merely that the call returned no error.
	var chunkSchema, chunkName string
	require.NoError(t, pool.QueryRow(ctx,
		`select chunk_schema, chunk_name from timescaledb_information.chunks
		 where hypertable_name = 'meter_readings' limit 1`).Scan(&chunkSchema, &chunkName))
	qualified := chunkSchema + "." + chunkName
	_, err = pool.Exec(ctx, `select compress_chunk($1::regclass)`, qualified)
	require.NoError(t, err, "compress_chunk errored on the clone")

	var isCompressed bool
	require.NoError(t, pool.QueryRow(ctx,
		`select is_compressed from timescaledb_information.chunks
		 where chunk_schema = $1 and chunk_name = $2`, chunkSchema, chunkName).Scan(&isCompressed))
	require.True(t, isCompressed, "compress_chunk reported success but the chunk is not marked compressed")
}

// TestNewIsolatedDBIsSafeUnderConcurrentCalls exercises the concurrency
// requirement from the Part C brief directly, rather than trusting it from
// the design alone: several subtests call NewIsolatedDB in parallel, and
// under -race that catches a shared name or a shared connection actually
// racing, not merely a design that was SUPPOSED to avoid one.
func TestNewIsolatedDBIsSafeUnderConcurrentCalls(t *testing.T) {
	for i := range 40 {
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

// Scheduled refresh policies make aggregate reads race the container's job
// scheduler (a materialised bucket and an R94-composed one differ by the
// boundary step), so no clone may carry one.
func TestIsolatedDBHasNoRefreshPolicies(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	var jobs int
	require.NoError(t, pool.QueryRow(t.Context(),
		`select count(*) from timescaledb_information.jobs where proc_name = 'policy_refresh_continuous_aggregate'`).Scan(&jobs))
	require.Zero(t, jobs, "the isolated template must ship without continuous-aggregate refresh policies")
}
