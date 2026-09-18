//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
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

// policyDB is a database the migrations were run against DIRECTLY, with its
// continuous-aggregate refresh policies intact.
//
// NewIsolatedDB cannot serve the two tests below: F6b's b34faba strips those
// policies from the shared template and from every clone, because a policy
// firing inside a short-lived clone materialised a monthly bucket for one
// analyzer while another stayed composed. That is right for repository tests
// and wrong for these two, which assert what migration 00005 declares for
// PRODUCTION — the only place the policies actually run.
func policyDB(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := testfixtures.NewEmptyDB(t)
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	return testfixtures.NewPool(t, dsn)
}

// TestHypertablesAreConfigured pins the partitioning decisions from
// 04-data-model.md §4. These are not cosmetic: chunk_time_interval and the
// space partition determine whether a one-month single-analyzer query reads
// four chunks or the whole table.
func TestHypertablesAreConfigured(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

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
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

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
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

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
	t.Parallel()
	ctx := context.Background()
	pool := policyDB(t, ctx)

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
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

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

// istanbul is the wall clock every business rule in this product is evaluated
// against (02-domain-rules.md §1). It is a fixed +03:00 rather than a
// time.LoadLocation of "Europe/Istanbul" so the test does not depend on the host
// carrying a tzdata database: Turkey has had no DST and no offset change since
// 2016, so for the 2025 dates used here the two are identical. The migrations
// themselves use the IANA name, which is what makes them correct if that ever
// changes.
var istanbul = time.FixedZone("+03", 3*60*60)

func at(year int, month time.Month, day, hour, min int) time.Time {
	return time.Date(year, month, day, hour, min, 0, 0, istanbul)
}

// seedAnalyzerReadings inserts nine load profile readings spanning three
// Istanbul calendar days at the turn of a year, plus one reading of another
// kind that must never reach a consumption aggregate. Two of them sit in the
// 00:00–03:00 Istanbul window that UTC bucketing misfiles: the 01:00 reading on
// 1 January belongs to January and to 2025, and the 01:00 reading on 2 January
// belongs to 2 January, but in UTC both fall on the previous day — and the first
// falls in the previous month and year.
func seedAnalyzerReadings(t *testing.T, ctx context.Context, pool *pgxpool.Pool, analyzerID uuid.UUID) {
	t.Helper()

	readings := []struct {
		ts        time.Time
		active    string
		maxDemand string
	}{
		{at(2025, time.January, 1, 1, 0), "990.0000", "12.5"},
		{at(2025, time.January, 2, 1, 0), "1000.0000", "12.5"},
		{at(2025, time.January, 2, 9, 0), "1010.0000", "12.5"},
		{at(2025, time.January, 2, 9, 15), "1020.5000", "12.5"},
		{at(2025, time.January, 2, 9, 30), "1030.2500", "20.0"},
		{at(2025, time.January, 2, 9, 45), "1035.0000", "12.5"},
		{at(2025, time.January, 2, 23, 45), "1040.0000", "12.5"},
		{at(2025, time.January, 3, 0, 15), "1050.0000", "12.5"},
		{at(2025, time.January, 3, 12, 0), "1080.0000", "12.5"},
	}
	for _, r := range readings {
		_, err := pool.Exec(ctx,
			`insert into meter_readings (analyzer_id, ts, kind, active_import, max_demand_kw, source_provider)
			 values ($1, $2, 'load_profile', $3::numeric, $4::numeric, 'osos')`,
			analyzerID, r.ts, r.active, r.maxDemand)
		require.NoError(t, err)
	}

	// Sits inside the 09:00 hour that the hourly leg asserts on, so every one of
	// the four consumption views covers its own `where kind = 'load_profile'`
	// clause rather than inheriting the coverage from a coarser bucket.
	_, err := pool.Exec(ctx,
		`insert into meter_readings (analyzer_id, ts, kind, active_import, source_provider)
		 values ($1, $2, 'daily', 99999::numeric, 'osos')`,
		analyzerID, at(2025, time.January, 2, 9, 30))
	require.NoError(t, err)
}

func seedAnalyzer(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)
	var analyzerID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into analyzers (company_id, building_id, provider, provider_subtype, installation_number)
		 values ($1, $2, 'osos', 'Baskent', $3) returning id`,
		companyID, buildingID, uuid.NewString()).Scan(&analyzerID))
	return analyzerID
}

// refreshAggregate materialises a whole window explicitly, ignoring the refresh
// policy's offsets. Note that TimescaleDB does not materialise a bucket that the
// window only partly covers, which is why every assertion below reads a bucket
// that closed before the test ran.
func refreshAggregate(t *testing.T, ctx context.Context, pool *pgxpool.Pool, view string) {
	t.Helper()
	refreshWindow(t, ctx, pool, view, at(2020, time.January, 1, 0, 0), at(2026, time.January, 1, 0, 0))
}

// pauseRefreshPolicies unschedules every continuous-aggregate refresh job before
// a test materialises a window by hand. TimescaleDB's job scheduler is live in
// the container and runs each policy shortly after it is registered, so a manual
// refresh over the same range loses to it with "could not refresh continuous
// aggregate due to a concurrent refresh" (SQLSTATE 55P03). That is a race in the
// test, not a defect in the migration, and unscheduling removes it rather than
// papering over it with a sleep.
func pauseRefreshPolicies(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`select alter_job(job_id, scheduled => false)
		   from timescaledb_information.jobs
		  where proc_name = 'policy_refresh_continuous_aggregate'`)
	require.NoError(t, err)
}

// refreshWindow materialises [from, to], retrying briefly if a policy run that
// was already in flight when the test unscheduled it still holds the lock.
func refreshWindow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, view string, from, to time.Time) {
	t.Helper()
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		_, err = pool.Exec(ctx,
			`call refresh_continuous_aggregate($1::regclass, $2::timestamptz, $3::timestamptz)`,
			view, from, to)
		if err == nil {
			return
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.NoError(t, err, "refresh %s", view)
}

// TestConsumptionAggregatesComputeRegisterDeltas is the end-to-end proof that
// all four consumption aggregates mean what §4.3 says they mean — consumption is
// last minus first of the cumulative register inside the bucket, not a sum of
// register values — and that each one buckets at the width it claims to.
// consumption_daily, _monthly and _yearly are hand-copied thirty-line blocks, so
// a wrong time_bucket literal in one of them would otherwise pass every other
// assertion in this package.
//
// It doubles as the timezone guard: every expected bucket below is an Istanbul
// calendar boundary, and readings deliberately sit in the 00:00–03:00 local
// window where UTC bucketing puts them on the previous day, month and year.
func TestConsumptionAggregatesComputeRegisterDeltas(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	pauseRefreshPolicies(t, ctx, pool)
	analyzerID := seedAnalyzer(t, ctx, pool)
	seedAnalyzerReadings(t, ctx, pool, analyzerID)

	for _, view := range []string{
		"consumption_hourly", "consumption_daily", "consumption_monthly", "consumption_yearly",
	} {
		refreshAggregate(t, ctx, pool, view)
	}

	for _, want := range []struct {
		view        string
		bucket      time.Time
		consumption string
		maxDemand   string
		index       string
		readings    int64
		why         string
	}{
		{"consumption_hourly", at(2025, time.January, 2, 9, 0), "25.0000", "20.0000", "1035.0000", 4,
			"four quarter-hourly readings inside one hour"},
		{"consumption_daily", at(2025, time.January, 1, 0, 0), "0.0000", "12.5000", "990.0000", 1,
			"the 01:00 Istanbul reading opens 1 January, not 31 December"},
		{"consumption_daily", at(2025, time.January, 2, 0, 0), "40.0000", "20.0000", "1040.0000", 6,
			"01:00 through 23:45 Istanbul is one local day"},
		{"consumption_daily", at(2025, time.January, 3, 0, 0), "30.0000", "12.5000", "1080.0000", 2,
			"00:15 Istanbul belongs to 3 January"},
		{"consumption_monthly", at(2025, time.January, 1, 0, 0), "90.0000", "20.0000", "1080.0000", 9,
			"January starts at 00:00 Istanbul on 1 January"},
		{"consumption_yearly", at(2025, time.January, 1, 0, 0), "90.0000", "20.0000", "1080.0000", 9,
			"2025 starts at 00:00 Istanbul on 1 January"},
	} {
		var consumption, maxDemand, index pgtype.Numeric
		var readings int64
		err := pool.QueryRow(ctx,
			`select active_consumption, max_demand_kw, active_index, reading_count
			   from `+pgIdent(t, want.view)+`
			  where analyzer_id = $1 and bucket = $2`,
			analyzerID, want.bucket).Scan(&consumption, &maxDemand, &index, &readings)
		require.NoError(t, err, "%s must hold a bucket at %s (%s)", want.view, want.bucket, want.why)

		require.Equal(t, want.readings, readings, "%s %s reading_count (%s)", want.view, want.bucket, want.why)
		requireNumericEquals(t, want.consumption, consumption)
		requireNumericEquals(t, want.maxDemand, maxDemand)
		requireNumericEquals(t, want.index, index)
	}

	// Nothing may land outside the Istanbul calendar boundaries above: a UTC
	// bucket would show up here as an extra December 2024 / 2024 row.
	for _, view := range []string{"consumption_daily", "consumption_monthly", "consumption_yearly"} {
		var earliest time.Time
		require.NoError(t, pool.QueryRow(ctx,
			`select min(bucket) from `+pgIdent(t, view)+` where analyzer_id = $1`, analyzerID).Scan(&earliest))
		require.True(t, earliest.Equal(at(2025, time.January, 1, 0, 0)),
			"%s must not open a bucket before 00:00 Istanbul on 1 January 2025, got %s", view, earliest)
	}
}

// TestPlantProductionAggregatesBucketInIstanbul does for the solar roll-ups what
// the test above does for consumption: a plant's "daily production" is a local
// calendar day, so a reading at 01:00 Istanbul belongs to that day and not to
// the one before.
func TestPlantProductionAggregatesBucketInIstanbul(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	pauseRefreshPolicies(t, ctx, pool)
	companyID, _ := seedCompanyAndBuilding(t, ctx, pool)
	var plantID, deviceID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into power_plants (company_id, name) values ($1, 'seed plant') returning id`,
		companyID).Scan(&plantID))
	require.NoError(t, pool.QueryRow(ctx,
		`insert into power_plant_devices (plant_id, device_sn) values ($1, $2) returning id`,
		plantID, uuid.NewString()).Scan(&deviceID))

	for _, r := range []struct {
		ts              time.Time
		kwh, power, eff string
	}{
		{at(2025, time.January, 1, 1, 0), "5.0000", "3.0000", "18.000"},
		{at(2025, time.January, 1, 13, 0), "7.0000", "9.0000", "20.000"},
		{at(2025, time.January, 2, 13, 0), "11.0000", "4.0000", "16.000"},
	} {
		_, err := pool.Exec(ctx,
			`insert into plant_production (plant_id, ts, device_id, production_kwh, active_power_kw, efficiency_pct)
			 values ($1, $2, $3, $4::numeric, $5::numeric, $6::numeric)`,
			plantID, r.ts, deviceID, r.kwh, r.power, r.eff)
		require.NoError(t, err)
	}

	for _, view := range []string{"plant_production_daily", "plant_production_monthly"} {
		refreshAggregate(t, ctx, pool, view)
	}

	var kwh, power, eff pgtype.Numeric
	require.NoError(t, pool.QueryRow(ctx,
		`select production_kwh, max_active_power_kw, avg_efficiency_pct
		   from plant_production_daily where plant_id = $1 and bucket = $2`,
		plantID, at(2025, time.January, 1, 0, 0)).Scan(&kwh, &power, &eff),
		"the 01:00 Istanbul row must belong to 1 January, not 31 December")
	requireNumericEquals(t, "12.0000", kwh)
	requireNumericEquals(t, "9.0000", power)
	requireNumericEquals(t, "19.000", eff)

	require.NoError(t, pool.QueryRow(ctx,
		`select production_kwh, max_active_power_kw, avg_efficiency_pct
		   from plant_production_monthly where plant_id = $1 and bucket = $2`,
		plantID, at(2025, time.January, 1, 0, 0)).Scan(&kwh, &power, &eff))
	requireNumericEquals(t, "23.0000", kwh)
	requireNumericEquals(t, "9.0000", power)
	requireNumericEquals(t, "18.000", eff)
}

// TestAggregateBucketsAreConfiguredForIstanbul pins the bucket width and the
// timezone of every aggregate directly, rather than only through the values they
// produce. Timescale keeps this in its own catalogue because the information
// view does not expose it in 2.30. consumption_hourly is deliberately
// timezone-free: a whole-hour offset makes the timezone argument a no-op there,
// and leaving it off keeps the aggregate a fixed-width bucket.
func TestAggregateBucketsAreConfiguredForIstanbul(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	for _, want := range []struct{ view, width, timezone string }{
		{"consumption_hourly", "1 hour", ""},
		{"consumption_daily", "1 day", "Europe/Istanbul"},
		{"consumption_monthly", "1 month", "Europe/Istanbul"},
		{"consumption_yearly", "1 year", "Europe/Istanbul"},
		{"plant_production_daily", "1 day", "Europe/Istanbul"},
		{"plant_production_monthly", "1 month", "Europe/Istanbul"},
	} {
		var widthMatches bool
		var width, timezone string
		require.NoError(t, pool.QueryRow(ctx,
			`select b.bucket_width::interval = $2::interval, b.bucket_width, coalesce(b.bucket_timezone, '')
			   from timescaledb_information.continuous_aggregates ca
			   join _timescaledb_catalog.hypertable h
			     on h.schema_name = ca.materialization_hypertable_schema
			    and h.table_name  = ca.materialization_hypertable_name
			   join _timescaledb_catalog.continuous_aggs_bucket_function b
			     on b.mat_hypertable_id = h.id
			  where ca.view_name = $1`, want.view, want.width).Scan(&widthMatches, &width, &timezone))
		require.True(t, widthMatches, "%s bucket width: want %s, got %s", want.view, want.width, width)
		require.Equal(t, want.timezone, timezone, "%s bucket timezone", want.view)
	}
}

// TestRefreshPolicyOffsetsAreConfigured pins the offsets themselves, not just
// that a job exists. Only consumption_hourly's values come from the spec; the
// other five were chosen in this task, which makes them exactly the values most
// likely to be changed by accident later.
func TestRefreshPolicyOffsetsAreConfigured(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := policyDB(t, ctx)

	for _, want := range []struct{ view, start, end, schedule string }{
		{"consumption_hourly", "30 days", "1 hour", "30 minutes"},
		{"consumption_daily", "90 days", "1 hour", "1 hour"},
		{"consumption_monthly", "1 year", "1 hour", "6 hours"},
		{"consumption_yearly", "5 years", "1 hour", "1 day"},
		{"plant_production_daily", "90 days", "1 hour", "1 hour"},
		{"plant_production_monthly", "1 year", "1 hour", "6 hours"},
	} {
		var startOK, endOK, scheduleOK bool
		var start, end, schedule string
		require.NoError(t, pool.QueryRow(ctx,
			`select (config->>'start_offset')::interval = $2::interval,
			        (config->>'end_offset')::interval   = $3::interval,
			        schedule_interval                   = $4::interval,
			        config->>'start_offset', config->>'end_offset', schedule_interval::text
			   from timescaledb_information.jobs
			  where proc_name = 'policy_refresh_continuous_aggregate'
			    and hypertable_name = $1`,
			want.view, want.start, want.end, want.schedule).Scan(&startOK, &endOK, &scheduleOK, &start, &end, &schedule))
		require.True(t, startOK, "%s start_offset: want %s, got %s", want.view, want.start, start)
		require.True(t, endOK, "%s end_offset: want %s, got %s", want.view, want.end, end)
		require.True(t, scheduleOK, "%s schedule_interval: want %s, got %s", want.view, want.schedule, schedule)
	}
}

// TestCompressionSettingsAreConfigured covers what a policy count cannot: a
// compression policy with the wrong segmentby is worse than none, because it
// compresses the table into an arrangement every query then has to decompress.
// segmentby must be the columns queries filter on, orderby the time column
// descending.
func TestCompressionSettingsAreConfigured(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	for _, want := range []struct{ table, segmentby, orderby string }{
		{"meter_readings", "analyzer_id,kind", "ts DESC"},
		{"plant_production", "plant_id,device_id", "ts DESC"},
	} {
		var segmentby, orderby string
		require.NoError(t, pool.QueryRow(ctx,
			`select segmentby, orderby from timescaledb_information.hypertable_compression_settings
			  where hypertable = $1::regclass`, want.table).Scan(&segmentby, &orderby))
		require.Equal(t, want.segmentby, segmentby, "%s compress_segmentby", want.table)
		require.Equal(t, want.orderby, orderby, "%s compress_orderby", want.table)
	}

	// The compression threshold is a business decision (§4.1), not a default.
	for _, table := range []string{"meter_readings", "plant_production"} {
		var matches bool
		var actual string
		require.NoError(t, pool.QueryRow(ctx,
			`select (config->>'compress_after')::interval = interval '90 days', config->>'compress_after'
			   from timescaledb_information.jobs
			  where proc_name = 'policy_compression' and hypertable_name = $1`, table).Scan(&matches, &actual))
		require.True(t, matches, "%s must compress after 90 days, got %s", table, actual)
	}
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

// pgIdent guards the handful of places where a view name has to be interpolated
// into SQL because it names a relation rather than a value. Only the literal
// names this file declares may pass.
func pgIdent(t *testing.T, name string) string {
	t.Helper()
	switch name {
	case "consumption_hourly", "consumption_daily", "consumption_monthly", "consumption_yearly",
		"plant_production_daily", "plant_production_monthly":
		return name
	}
	t.Fatalf("refusing to interpolate unknown relation %q", name)
	return ""
}

// seedPlantWithDevice inserts the plant and device that plant_production's
// foreign keys need, and returns both ids. device_id is part of that
// hypertable's primary key, so Postgres promotes it to NOT NULL and a production
// row without a device cannot be stored — see the task report.
func seedPlantWithDevice(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (plantID, deviceID uuid.UUID) {
	t.Helper()
	companyID, _ := seedCompanyAndBuilding(t, ctx, pool)
	require.NoError(t, pool.QueryRow(ctx,
		`insert into power_plants (company_id, name) values ($1, 'seed plant') returning id`,
		companyID).Scan(&plantID))
	require.NoError(t, pool.QueryRow(ctx,
		`insert into power_plant_devices (plant_id, device_sn) values ($1, $2) returning id`,
		plantID, uuid.NewString()).Scan(&deviceID))
	return plantID, deviceID
}

// istanbulBucketStart truncates now to the start of the Istanbul hour, day,
// month or year that is still open at now. Test data is anchored to the
// bucket start rather than to a fresh time.Now() per call so a run that
// begins at HH:59 or at 23:59 local cannot straddle two buckets — and, now
// that this package runs under t.Parallel, so that a caller computing
// several bucket starts and later a refresh window (refreshPolicyRange) all
// agree on the same instant instead of each reading its own now() and
// risking an hour/day boundary crossing between calls under scheduling
// delay (F2 Task 0P2, review B minor M8).
func istanbulBucketStart(now time.Time, unit string) time.Time {
	now = now.In(istanbul)
	switch unit {
	case "hour":
		return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, istanbul)
	case "day":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, istanbul)
	case "month":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, istanbul)
	default:
		return time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, istanbul)
	}
}

// refreshPolicyRange materialises exactly what the view's refresh policy would:
// [now - startOffset, now - end_offset], with end_offset one hour everywhere.
// The bucket currently open therefore stays unmaterialised, which is the whole
// point of both tests below — whether it is nonetheless visible depends only on
// materialized_only.
//
// now is the caller's single wall-clock read, the same one passed to
// istanbulBucketStart when seeding — not a fresh time.Now() here — so the
// seeded bucket and the materialised window can never straddle an hour/day
// boundary that opened between the two calls (F2 Task 0P2, review B minor
// M8).
func refreshPolicyRange(t *testing.T, ctx context.Context, pool *pgxpool.Pool, view string, startOffset time.Duration, now time.Time) {
	t.Helper()
	refreshWindow(t, ctx, pool, view, now.Add(-startOffset), now.Add(-time.Hour))
}

// TestOpenBucketIsVisibleInRealTimeAggregates proves the half of the
// materialized_only split that costs something to get wrong. TimescaleDB does
// not materialise a bucket the refresh window only partly covers, at any
// end_offset, so the hour, day or month currently in progress is never in the
// materialised data. On a materialized_only view that means a dashboard asking
// for "today" gets no row at all — not a stale figure, an absent one, which is
// the same silent-undercount failure as an unrefreshed backfill.
//
// consumption_hourly, consumption_daily and plant_production_daily therefore set
// timescaledb.materialized_only = false, so the query unions the materialised
// buckets with a live scan above the materialisation watermark. That tail is at
// most one open bucket — a day of raw readings for one analyzer — which is cheap.
func TestOpenBucketIsVisibleInRealTimeAggregates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	pauseRefreshPolicies(t, ctx, pool)
	now := time.Now()
	hourStart := istanbulBucketStart(now, "hour")
	dayStart := istanbulBucketStart(now, "day")

	// Two analyzers, deliberately. Between 00:00 and 01:00 Europe/Istanbul the
	// open hour and the open day begin at the same instant, so seeding both legs
	// against one analyzer would collide on meter_readings' primary key
	// (analyzer_id, ts, kind) — and the test would then fail for one hour every
	// day with a value nobody could account for. Keeping the legs on separate
	// analyzers makes them independent of the wall clock. The inserts carry no
	// `on conflict` clause for the same reason: a seeder that silently discards
	// rows it meant to write can only ever fail confusingly.
	dayAnalyzer := seedAnalyzer(t, ctx, pool)
	hourAnalyzer := seedAnalyzer(t, ctx, pool)
	for _, r := range []struct {
		analyzer uuid.UUID
		ts       time.Time
		active   string
	}{
		{dayAnalyzer, dayStart, "2000.0000"},
		{dayAnalyzer, dayStart.Add(time.Second), "2005.0000"},
		{hourAnalyzer, hourStart, "2100.0000"},
		{hourAnalyzer, hourStart.Add(time.Second), "2130.0000"},
	} {
		_, err := pool.Exec(ctx,
			`insert into meter_readings (analyzer_id, ts, kind, active_import, max_demand_kw, source_provider)
			 values ($1, $2, 'load_profile', $3::numeric, 7::numeric, 'osos')`,
			r.analyzer, r.ts, r.active)
		require.NoError(t, err)
	}

	plantID, deviceID := seedPlantWithDevice(t, ctx, pool)
	_, err := pool.Exec(ctx,
		`insert into plant_production (plant_id, ts, device_id, production_kwh, active_power_kw, efficiency_pct)
		 values ($1, $2, $3, 4::numeric, 2::numeric, 15::numeric)`,
		plantID, dayStart, deviceID)
	require.NoError(t, err)

	refreshPolicyRange(t, ctx, pool, "consumption_hourly", 30*24*time.Hour, now)
	refreshPolicyRange(t, ctx, pool, "consumption_daily", 90*24*time.Hour, now)
	refreshPolicyRange(t, ctx, pool, "plant_production_daily", 90*24*time.Hour, now)

	var consumption pgtype.Numeric
	require.NoError(t, pool.QueryRow(ctx,
		`select active_consumption from consumption_hourly where analyzer_id = $1 and bucket = $2`,
		hourAnalyzer, hourStart).Scan(&consumption),
		"the hour in progress must be visible in consumption_hourly")
	requireNumericEquals(t, "30.0000", consumption)

	require.NoError(t, pool.QueryRow(ctx,
		`select active_consumption from consumption_daily where analyzer_id = $1 and bucket = $2`,
		dayAnalyzer, dayStart).Scan(&consumption),
		"the Istanbul day in progress must be visible in consumption_daily")
	requireNumericEquals(t, "5.0000", consumption)

	var kwh pgtype.Numeric
	require.NoError(t, pool.QueryRow(ctx,
		`select production_kwh from plant_production_daily where plant_id = $1 and bucket = $2`,
		plantID, dayStart).Scan(&kwh),
		"the Istanbul day in progress must be visible in plant_production_daily")
	requireNumericEquals(t, "4.0000", kwh)
}

// TestOpenBucketIsDeliberatelyAbsentFromCoarseAggregates pins an absence, so it
// has to say why: this is a decision, not an oversight, and the next person to
// see a missing month-to-date figure must not "fix" it by flipping
// materialized_only.
//
// consumption_monthly, consumption_yearly and plant_production_monthly stay
// materialized_only. Their materialisation watermark sits at the end of the last
// CLOSED month or year, so real-time aggregation would put a live scan of every
// raw reading since then behind an innocuous dashboard query — in September,
// consumption_yearly would rescan nine months per analyzer every time it is
// read. That is a worse outcome than composing the figure explicitly.
//
// CONSTRAINT ON TASK 10: month-to-date and year-to-date must be composed —
// closed buckets from the monthly or yearly aggregate, plus the open period
// taken from consumption_daily or from meter_readings. The open bucket is not in
// these views and must never be assumed to be.
func TestOpenBucketIsDeliberatelyAbsentFromCoarseAggregates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	pauseRefreshPolicies(t, ctx, pool)
	analyzerID := seedAnalyzer(t, ctx, pool)
	now := time.Now()
	monthStart := istanbulBucketStart(now, "month")
	yearStart := istanbulBucketStart(now, "year")
	for i, active := range []string{"3000.0000", "3050.0000"} {
		_, err := pool.Exec(ctx,
			`insert into meter_readings (analyzer_id, ts, kind, active_import, source_provider)
			 values ($1, $2, 'load_profile', $3::numeric, 'osos')`,
			analyzerID, monthStart.Add(time.Duration(i)*time.Second), active)
		require.NoError(t, err)
	}

	plantID, deviceID := seedPlantWithDevice(t, ctx, pool)
	_, err := pool.Exec(ctx,
		`insert into plant_production (plant_id, ts, device_id, production_kwh)
		 values ($1, $2, $3, 9::numeric)`, plantID, monthStart, deviceID)
	require.NoError(t, err)

	refreshPolicyRange(t, ctx, pool, "consumption_monthly", 365*24*time.Hour, now)
	refreshPolicyRange(t, ctx, pool, "consumption_yearly", 5*365*24*time.Hour, now)
	refreshPolicyRange(t, ctx, pool, "plant_production_monthly", 365*24*time.Hour, now)

	// The rows exist and are reachable through a real-time view, so an absence
	// below is the materialisation boundary and not a failed seed.
	refreshPolicyRange(t, ctx, pool, "consumption_daily", 90*24*time.Hour, now)
	var visible int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from consumption_daily where analyzer_id = $1 and bucket >= $2`,
		analyzerID, monthStart).Scan(&visible))
	require.Positive(t, visible, "the seeded readings must be visible somewhere")

	for _, absent := range []struct {
		view, idColumn string
		id             uuid.UUID
		bucket         time.Time
	}{
		{"consumption_monthly", "analyzer_id", analyzerID, monthStart},
		{"consumption_yearly", "analyzer_id", analyzerID, yearStart},
		{"plant_production_monthly", "plant_id", plantID, monthStart},
	} {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`select count(*) from `+pgIdent(t, absent.view)+
				` where `+pgColumn(t, absent.idColumn)+` = $1 and bucket = $2`,
			absent.id, absent.bucket).Scan(&n))
		require.Zero(t, n,
			"%s must not expose the open bucket: it is materialized_only by design, and a "+
				"to-date figure has to be composed from the daily aggregate or the hypertable",
			absent.view)
	}
}

// TestMaterializedOnlyIsConfiguredPerView pins the split itself. Flipping any of
// these six changes what a dashboard shows, or what a dashboard query costs,
// without changing a line of Go — so it must trip a test rather than ship.
func TestMaterializedOnlyIsConfiguredPerView(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	for _, want := range []struct {
		view             string
		materializedOnly bool
		why              string
	}{
		{"consumption_hourly", false, "the open hour must be visible; the live tail is one hour"},
		{"consumption_daily", false, "the open Istanbul day must be visible; the live tail is one day"},
		{"plant_production_daily", false, "the open Istanbul day must be visible; the live tail is one day"},
		{"consumption_monthly", true, "a live tail would rescan every reading since the last closed month"},
		{"consumption_yearly", true, "a live tail would rescan every reading since the last closed year"},
		{"plant_production_monthly", true, "a live tail would rescan every row since the last closed month"},
	} {
		var materializedOnly bool
		require.NoError(t, pool.QueryRow(ctx,
			`select materialized_only from timescaledb_information.continuous_aggregates
			  where view_name = $1`, want.view).Scan(&materializedOnly))
		require.Equal(t, want.materializedOnly, materializedOnly, "%s: %s", want.view, want.why)
	}
}

// pgColumn is pgIdent's counterpart for the two grouping columns these views are
// keyed by.
func pgColumn(t *testing.T, name string) string {
	t.Helper()
	if name == "analyzer_id" || name == "plant_id" {
		return name
	}
	t.Fatalf("refusing to interpolate unknown column %q", name)
	return ""
}

// TestBtreeGinIsInstalled covers the extension §1 mandates and 00001 originally
// missed. It is not decorative: without btree_gin a GIN index cannot mix a
// scalar column with a jsonb one, so the first migration that tries fails at
// deploy time rather than here.
func TestBtreeGinIsInstalled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	var installed bool
	require.NoError(t, pool.QueryRow(ctx,
		`select exists(select 1 from pg_extension where extname = 'btree_gin')`).Scan(&installed))
	require.True(t, installed)
}
