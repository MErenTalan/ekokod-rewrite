//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

// chunkInterval scans a timescaledb_information.dimensions time_interval, which
// is a Postgres interval and so does not scan straight into a time.Duration.
// Every chunk interval this schema uses is a whole number of days, so a month
// component would mean the migration configured something other than what it
// meant to and is rejected rather than silently approximated at 30 days.
func chunkInterval(t *testing.T, iv pgtype.Interval) time.Duration {
	t.Helper()
	require.True(t, iv.Valid, "the time dimension must have an interval")
	require.Zero(t, iv.Months, "a chunk interval in months has no exact duration")
	return time.Duration(iv.Days)*24*time.Hour + time.Duration(iv.Microseconds)*time.Microsecond
}

// TestHypertablesAreConfigured pins the partitioning decisions from
// 04-data-model.md §4. These are not cosmetic: chunk_time_interval and the
// space partition determine whether a one-month single-analyzer query reads
// four chunks or the whole table.
func TestHypertablesAreConfigured(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	for _, want := range []struct {
		table       string
		numDims     int
		chunkInterv time.Duration
	}{
		{"meter_readings", 2, 7 * 24 * time.Hour},
		{"plant_production", 1, 30 * 24 * time.Hour},
		{"market_prices_hourly", 1, 90 * 24 * time.Hour},
		{"forecasts", 1, 30 * 24 * time.Hour},
	} {
		var dims int
		require.NoError(t, pool.QueryRow(ctx,
			`select num_dimensions from timescaledb_information.hypertables
			 where hypertable_name = $1`, want.table).Scan(&dims),
			"%s must be a hypertable", want.table)
		require.Equal(t, want.numDims, dims, "%s dimension count", want.table)

		var interval pgtype.Interval
		require.NoError(t, pool.QueryRow(ctx,
			`select time_interval from timescaledb_information.dimensions
			 where hypertable_name = $1 and dimension_type = 'Time'`, want.table).Scan(&interval))
		require.Equal(t, want.chunkInterv, chunkInterval(t, interval), "%s chunk interval", want.table)
	}
}

// TestMeterReadingsIsSpacePartitionedByAnalyzer covers the half of §4.1 that a
// dimension count alone does not: meter_readings has two dimensions, but the
// second one is only useful if it hashes analyzer_id into the 16 partitions the
// spec asks for. A migration that passed a different column, or a different
// partition count, would still report num_dimensions = 2.
func TestMeterReadingsIsSpacePartitionedByAnalyzer(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	var column string
	var partitions int
	require.NoError(t, pool.QueryRow(ctx,
		`select column_name, num_partitions from timescaledb_information.dimensions
		 where hypertable_name = 'meter_readings' and dimension_type = 'Space'`).
		Scan(&column, &partitions))
	require.Equal(t, "analyzer_id", column)
	require.Equal(t, 16, partitions)
}

// TestCompressionPoliciesExist covers the F1 acceptance criterion that a
// chunk older than the threshold compresses. Task 13 proves compression
// actually runs and that queries still return correct rows from a compressed
// chunk; this test only proves the policy was installed by the migration.
func TestCompressionPoliciesExist(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	for _, table := range []string{"meter_readings", "plant_production"} {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`select count(*) from timescaledb_information.jobs
			 where proc_name = 'policy_compression' and hypertable_name = $1`, table).Scan(&n))
		require.Equal(t, 1, n, "%s must have exactly one compression policy", table)
	}
}

// TestContinuousAggregatesExist pins the aggregate names Task 10's analytics
// repositories and Task 13's correctness test both depend on.
func TestContinuousAggregatesExist(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	for _, name := range []string{
		"consumption_hourly", "consumption_daily", "consumption_monthly",
		"consumption_yearly", "plant_production_daily", "plant_production_monthly",
	} {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`select count(*) from timescaledb_information.continuous_aggregates
			 where view_name = $1`, name).Scan(&n))
		require.Equal(t, 1, n, "continuous aggregate %s must exist", name)

		var policies int
		require.NoError(t, pool.QueryRow(ctx,
			`select count(*) from timescaledb_information.jobs
			 where proc_name = 'policy_refresh_continuous_aggregate'
			   and hypertable_name = $1`, name).Scan(&policies))
		require.Equal(t, 1, policies, "%s must have a refresh policy", name)
	}
}

// TestConsumptionColumnsAreNumeric guards the explicit ::numeric casts in
// 00005. Postgres would produce numeric for these expressions with or without
// the casts, but sqlc does not know first()/last() and guesses: without the
// cast it generated `ActiveConsumption int32` for an energy quantity, which no
// linter or migration test would catch and which reaches a customer invoice as
// wrong kWh. Asserting the catalogue type here is the cheap standing check that
// the casts were not dropped in a later edit.
func TestConsumptionColumnsAreNumeric(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	for _, view := range []string{
		"consumption_hourly", "consumption_daily", "consumption_monthly", "consumption_yearly",
	} {
		for _, column := range []string{
			"active_import_start", "active_import_end", "active_consumption",
			"inductive_consumption", "capacitive_consumption",
			"t1_consumption", "t2_consumption", "t3_consumption",
			"active_generation", "inductive_generation", "capacitive_generation",
			"t1_generation", "t2_generation", "t3_generation",
			"max_demand_kw", "active_index", "inductive_index", "capacitive_index",
			"t1_index", "t2_index", "t3_index", "active_generation_index",
		} {
			var dataType string
			require.NoError(t, pool.QueryRow(ctx,
				`select data_type from information_schema.columns
				 where table_schema = 'public' and table_name = $1 and column_name = $2`,
				view, column).Scan(&dataType), "%s.%s must exist", view, column)
			require.Equal(t, "numeric", dataType, "%s.%s must be numeric, never a float or an integer", view, column)
		}
	}

	for _, view := range []string{"plant_production_daily", "plant_production_monthly"} {
		for _, column := range []string{"production_kwh", "max_active_power_kw", "avg_efficiency_pct"} {
			var dataType string
			require.NoError(t, pool.QueryRow(ctx,
				`select data_type from information_schema.columns
				 where table_schema = 'public' and table_name = $1 and column_name = $2`,
				view, column).Scan(&dataType), "%s.%s must exist", view, column)
			require.Equal(t, "numeric", dataType, "%s.%s must be numeric", view, column)
		}
	}
}

// TestConsumptionHourlyComputesRegisterDeltas is the one end-to-end check that
// the aggregate means what §4.3 says it means: consumption is last minus first
// of the cumulative register inside the bucket, not a sum of the register
// values. It also proves the hypertable accepts a real reading through its
// foreign keys and enum, which nothing else in this package covers.
func TestConsumptionHourlyComputesRegisterDeltas(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)
	var analyzerID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into analyzers (company_id, building_id, provider, provider_subtype, installation_number)
		 values ($1, $2, 'osos', 'Baskent', $3) returning id`,
		companyID, buildingID, uuid.NewString()).Scan(&analyzerID))

	// One hour of quarter-hourly load profile readings on a cumulative register.
	base := time.Date(2025, 3, 4, 9, 0, 0, 0, time.UTC)
	for i, active := range []string{"1000.0000", "1010.5000", "1025.2500", "1040.0000"} {
		_, err := pool.Exec(ctx,
			`insert into meter_readings (analyzer_id, ts, kind, active_import, max_demand_kw, source_provider)
			 values ($1, $2, 'load_profile', $3::numeric, $4::numeric, 'osos')`,
			analyzerID, base.Add(time.Duration(i)*15*time.Minute), active, "12.5")
		require.NoError(t, err)
	}
	// A reading of another kind in the same hour must not reach the aggregate.
	_, err := pool.Exec(ctx,
		`insert into meter_readings (analyzer_id, ts, kind, active_import, source_provider)
		 values ($1, $2, 'daily', 99999::numeric, 'osos')`, analyzerID, base.Add(30*time.Minute))
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`call refresh_continuous_aggregate('consumption_hourly', $1::timestamptz, $2::timestamptz)`,
		base.Add(-time.Hour), base.Add(2*time.Hour))
	require.NoError(t, err)

	var consumption, maxDemand, index pgtype.Numeric
	var readings int64
	require.NoError(t, pool.QueryRow(ctx,
		`select active_consumption, max_demand_kw, active_index, reading_count
		 from consumption_hourly where analyzer_id = $1 and bucket = $2`,
		analyzerID, base).Scan(&consumption, &maxDemand, &index, &readings))

	require.Equal(t, int64(4), readings, "only the load_profile readings belong to the bucket")
	requireNumericEquals(t, "40.0000", consumption)
	requireNumericEquals(t, "12.5000", maxDemand)
	requireNumericEquals(t, "1040.0000", index)
}

func requireNumericEquals(t *testing.T, want string, got pgtype.Numeric) {
	t.Helper()
	require.True(t, got.Valid, "value must not be null")
	var wanted pgtype.Numeric
	require.NoError(t, wanted.Scan(want))
	cmp, err := got.Float64Value()
	require.NoError(t, err)
	expected, err := wanted.Float64Value()
	require.NoError(t, err)
	require.InDelta(t, expected.Float64, cmp.Float64, 1e-9)
}

// TestBtreeGinIsInstalled covers the extension §1 mandates and 00001 originally
// missed. It is not decorative: without btree_gin a GIN index cannot mix a
// scalar column with a jsonb one, so the first migration that tries fails at
// deploy time rather than here.
func TestBtreeGinIsInstalled(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	var installed bool
	require.NoError(t, pool.QueryRow(ctx,
		`select exists(select 1 from pg_extension where extname = 'btree_gin')`).Scan(&installed))
	require.True(t, installed)
}
