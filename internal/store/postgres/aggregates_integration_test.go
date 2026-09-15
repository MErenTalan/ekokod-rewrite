//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestContinuousAggregatesMatchAHandComputedFixture is the F1 acceptance
// criterion for continuous-aggregate correctness. It uses a SMALL fixture
// (four readings, two hourly buckets) whose expected values are written out
// BY HAND below, derived from 02-domain-rules.md §3.1 and migration 00005's
// view definition — never computed by the same SQL under test.
//
// It reuses seedAnalyzer/at/pauseRefreshPolicies/refreshWindow/
// requireNumericEquals from migrations_timeseries_integration_test.go
// (same package, same build tag).
func TestContinuousAggregatesMatchAHandComputedFixture(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	pauseRefreshPolicies(t, ctx, pool)
	analyzerID := seedAnalyzer(t, ctx, pool)

	// active_import: 100 @ 00:00, 150 @ 00:30, 175 @ 01:00, 210 @ 01:30.
	for _, r := range []struct {
		ts     time.Time
		active string
	}{
		{at(2025, time.March, 1, 0, 0), "100.0000"},
		{at(2025, time.March, 1, 0, 30), "150.0000"},
		{at(2025, time.March, 1, 1, 0), "175.0000"},
		{at(2025, time.March, 1, 1, 30), "210.0000"},
	} {
		_, err := pool.Exec(ctx,
			`insert into meter_readings (analyzer_id, ts, kind, active_import, source_provider)
			 values ($1, $2, 'load_profile', $3::numeric, 'osos')`,
			analyzerID, r.ts, r.active)
		require.NoError(t, err)
	}

	// consumption_hourly is materialized_only = false IN PRODUCTION (migration
	// 00005): it unions materialised buckets with a live scan above the
	// refresh watermark, so a bare query against it would return the correct
	// rows EVEN IF the refresh_continuous_aggregate call below were deleted —
	// exactly the trap wave-g-task-notes.md records Task 8c's review having
	// found. Setting materialized_only = true HERE, on this disposable
	// isolated-test database only, forces every read below through the
	// materialised data alone, so this test can actually distinguish "the
	// refresh ran" from "real-time aggregation papered over a missing one".
	_, err := pool.Exec(ctx,
		`alter materialized view consumption_hourly set (timescaledb.materialized_only = true)`)
	require.NoError(t, err)

	refreshWindow(t, ctx, pool, "consumption_hourly",
		at(2025, time.March, 1, 0, 0), at(2025, time.March, 1, 2, 0))

	// --- hand-computed expectations --------------------------------------
	//
	// Migration 00005: active_consumption = last(active_import,ts) -
	// first(active_import,ts), taken WITHIN each time_bucket('1 hour', ts).
	//
	// Hour 0 bucket [00:00,01:00) holds 100@00:00 and 150@00:30 — the 01:00
	// row belongs to the NEXT bucket, because time_bucket produces half-open
	// buckets. first=100, last=150: active_consumption = 150-100 = 50.
	//
	// CONTROLLER RULING (task-13-brief.md's pre-flight note F13, restated in
	// wave-g-task-notes.md's Task 13 section): the answer is 50, NOT 75.
	// 75 would be 175-100, which reaches past the bucket boundary to use
	// the 01:00 row — that row is not inside hour 0.
	var hour0Consumption pgtype.Numeric
	var hour0Count int64
	require.NoError(t, pool.QueryRow(ctx,
		`select active_consumption, reading_count from consumption_hourly
		 where analyzer_id = $1 and bucket = $2`,
		analyzerID, at(2025, time.March, 1, 0, 0)).Scan(&hour0Consumption, &hour0Count))
	requireNumericEquals(t, "50.0000", hour0Consumption)
	require.EqualValues(t, 2, hour0Count, "hour 0 must hold exactly the 00:00 and 00:30 readings")

	// Hour 1 bucket [01:00,02:00) holds 175@01:00 and 210@01:30.
	// first=175, last=210: active_consumption = 210-175 = 35.
	var hour1Consumption pgtype.Numeric
	var hour1Count int64
	require.NoError(t, pool.QueryRow(ctx,
		`select active_consumption, reading_count from consumption_hourly
		 where analyzer_id = $1 and bucket = $2`,
		analyzerID, at(2025, time.March, 1, 1, 0)).Scan(&hour1Consumption, &hour1Count))
	requireNumericEquals(t, "35.0000", hour1Consumption)
	require.EqualValues(t, 2, hour1Count, "hour 1 must hold exactly the 01:00 and 01:30 readings")

	// --- the boundary-semantics caveat (04-data-model.md §4.3) -----------
	//
	// The domain function (02-domain-rules.md §3.1) over the SAME two-hour
	// span [00:00, 02:00): reading_start = the last reading at or before
	// 00:00 (100 @ 00:00 itself); reading_end = the last reading at or
	// before 02:00 (210 @ 01:30, the latest row there is). Its answer is
	// 210 - 100 = 110.
	//
	// The two hourly buckets computed above sum to 50 + 35 = 85 —
	// STRICTLY LESS than the domain function's 110. The missing 25 is
	// exactly the step from the last reading INSIDE hour 0 (150 @ 00:30) to
	// the first reading INSIDE hour 1 (175 @ 01:00): that inter-bucket step
	// is invisible to first()/last() computed separately within each
	// bucket, so no bucket ever counts it. This is a documented, deliberate
	// property (04-data-model.md §4.3's escape clause; migration 00005's
	// header comment on the four consumption_* views), not a bug: a future
	// change that made the aggregate sum equal the domain function's answer
	// would silently redefine what a bucket means and break the
	// billing/analytics split the spec draws between them.
	aggregateSum := decimal.RequireFromString("50.0000").Add(decimal.RequireFromString("35.0000"))
	domainAnswer := decimal.RequireFromString("210.0000").Sub(decimal.RequireFromString("100.0000"))
	require.True(t, aggregateSum.LessThan(domainAnswer),
		"the aggregate sum across buckets (%s) must be LESS than the domain function's answer "+
			"over the same span (%s) — this is the boundary-semantics caveat, not a bug",
		aggregateSum, domainAnswer)
	gap := domainAnswer.Sub(aggregateSum)
	require.True(t, gap.Equal(decimal.RequireFromString("25.0000")),
		"the gap between the domain answer and the bucket sum must be exactly the dropped "+
			"inter-bucket step (175-150=25), got %s", gap)
}

// TestCompressedChunksStillReturnCorrectRows is the F1 acceptance criterion
// "compression policy compresses a chunk older than the threshold, and
// queries still return correct results from it". compress_chunk is called
// DIRECTLY rather than waiting for the background compression policy — the
// same reasoning migrations_timeseries_integration_test.go's
// pauseRefreshPolicies comment gives for continuous aggregates: waiting on a
// Timescale background worker is a timing dependency, and this phase has
// already paid for one of those.
func TestCompressedChunksStillReturnCorrectRows(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)
	var analyzerID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into analyzers (company_id, building_id, provider, provider_subtype, installation_number)
		 values ($1, $2, 'osos', 'Baskent', 'COMPRESS-1') returning id`,
		companyID, buildingID).Scan(&analyzerID))

	scope := store.Scope{CompanyID: companyID, AllBuildings: true}
	require.True(t, scope.Valid())
	repo := postgres.NewReadingRepository(pool)

	// Older than meter_readings' 90-day compression threshold (migration
	// 00004: add_compression_policy('meter_readings', interval '90 days')).
	old := time.Now().UTC().Add(-120 * 24 * time.Hour).Truncate(time.Second)
	rows := []model.MeterReading{
		readingsRow(analyzerID, old, model.ReadingKindLoadProfile, "1234.5678", "1"),
		readingsRow(analyzerID, old.Add(15*time.Minute), model.ReadingKindLoadProfile, "1240.1234", "1"),
		readingsRow(analyzerID, old.Add(30*time.Minute), model.ReadingKindLoadProfile, "1250.0001", "1"),
	}
	inserted, updated, err := repo.BulkInsert(ctx, scope, rows)
	require.NoError(t, err)
	require.Equal(t, len(rows), inserted)
	require.Zero(t, updated)

	compressedChunks := compressOldChunks(t, ctx, pool)
	require.NotEmpty(t, compressedChunks,
		"at least one chunk older than 90 days must exist — the fixture's readings are 120 days old")

	for _, chunk := range compressedChunks {
		var isCompressed bool
		require.NoError(t, pool.QueryRow(ctx,
			`select is_compressed from timescaledb_information.chunks
			 where format('%I.%I', chunk_schema, chunk_name) = $1`, chunk).Scan(&isCompressed))
		require.True(t, isCompressed, "chunk %s must report is_compressed after compress_chunk", chunk)
	}

	// Re-read through ReadingRepository.Range (the billing-grade surface —
	// never a continuous aggregate, per analytics.go's header) and compare
	// byte-identical values to what was written.
	got, err := repo.Range(ctx, scope, analyzerID,
		store.TimeRange{From: old.Add(-time.Minute), To: old.Add(time.Hour)},
		model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.Len(t, got, len(rows))
	for i, want := range rows {
		require.True(t, got[i].Ts.Equal(want.Ts), "row %d ts: want %s got %s", i, want.Ts, got[i].Ts)
		require.NotNil(t, got[i].ActiveImport)
		require.True(t, got[i].ActiveImport.Equal(*want.ActiveImport),
			"row %d active_import: want %s got %s", i, want.ActiveImport, got[i].ActiveImport)
	}
}

// compressOldChunks calls compress_chunk directly on every chunk of
// meter_readings older than the 90-day threshold and returns their
// fully-qualified names.
func compressOldChunks(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx,
		`select compress_chunk(c)::text
		   from show_chunks('meter_readings', older_than => interval '90 days') c`)
	require.NoError(t, err)
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	return names
}
