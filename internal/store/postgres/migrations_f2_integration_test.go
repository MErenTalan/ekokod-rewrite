//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestF2MeterReadingsHasIntervalGenerationColumn pins migration 00012's
// `alter table meter_readings add column interval_generation_kwh` (PM5340's
// interval energy, 06 §5, R1): numeric(18,4), nullable — never a float, and
// never NOT NULL, since it is nil for every provider but PM5340.
func TestF2MeterReadingsHasIntervalGenerationColumn(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

	var dataType, isNullable string
	var precision, scale int
	require.NoError(t, pool.QueryRow(ctx,
		`select data_type, is_nullable, numeric_precision, numeric_scale
		   from information_schema.columns
		  where table_schema = 'public' and table_name = 'meter_readings'
		    and column_name = 'interval_generation_kwh'`,
	).Scan(&dataType, &isNullable, &precision, &scale))

	require.Equal(t, "numeric", dataType)
	require.Equal(t, "YES", isNullable, "interval_generation_kwh must be nullable: nil for every provider but PM5340")
	require.Equal(t, 18, precision)
	require.Equal(t, 4, scale)
}

// TestF2GenerationAnchorsTable pins migration 00012's generation_anchors
// table: its columns, its primary key (analyzer_id alone — one anchor per
// analyzer), and the active_export >= 0 check constraint.
func TestF2GenerationAnchorsTable(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

	for _, want := range []struct {
		column   string
		dataType string
	}{
		{"analyzer_id", "uuid"},
		{"anchor_ts", "timestamp with time zone"},
		{"active_export", "numeric"},
		{"source", "text"},
		{"updated_at", "timestamp with time zone"},
	} {
		var dataType string
		require.NoError(t, pool.QueryRow(ctx,
			`select data_type from information_schema.columns
			  where table_schema = 'public' and table_name = 'generation_anchors'
			    and column_name = $1`, want.column).Scan(&dataType),
			"generation_anchors.%s must exist", want.column)
		require.Equal(t, want.dataType, dataType, "generation_anchors.%s", want.column)
	}

	// Primary key is analyzer_id alone.
	var pkColumns []string
	rows, err := pool.Query(ctx,
		`select kcu.column_name
		   from information_schema.table_constraints tc
		   join information_schema.key_column_usage kcu
		     on kcu.constraint_name = tc.constraint_name and kcu.table_schema = tc.table_schema
		  where tc.table_schema = 'public' and tc.table_name = 'generation_anchors'
		    and tc.constraint_type = 'PRIMARY KEY'
		  order by kcu.ordinal_position`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var col string
		require.NoError(t, rows.Scan(&col))
		pkColumns = append(pkColumns, col)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []string{"analyzer_id"}, pkColumns)

	// The active_export >= 0 check constraint, proven black-box: seed a
	// company/building/analyzer and try to write a negative anchor.
	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)
	analyzerID := f2seedAnalyzer(t, ctx, pool, companyID, buildingID, "GEN-CHECK-1")

	_, err = pool.Exec(ctx,
		`insert into generation_anchors (analyzer_id, anchor_ts, active_export, source)
		 values ($1, now(), -1, 'initial')`, analyzerID)
	require.Error(t, err, "a negative active_export must be rejected by the check constraint")
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23514", pgErr.Code, "must be a check_violation, not some other error")

	// A non-negative value is accepted, so the assertion above is not
	// vacuous — it is this specific constraint, not a broken insert.
	_, err = pool.Exec(ctx,
		`insert into generation_anchors (analyzer_id, anchor_ts, active_export, source)
		 values ($1, now(), 0, 'initial')`, analyzerID)
	require.NoError(t, err)
}

// TestF2ProviderHourlyValuesIsAHypertable pins migration 00013:
// provider_hourly_values is a hypertable with a 30-day chunk interval.
func TestF2ProviderHourlyValuesIsAHypertable(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

	var dims int
	require.NoError(t, pool.QueryRow(ctx,
		`select num_dimensions from timescaledb_information.hypertables
		  where hypertable_name = 'provider_hourly_values'`).Scan(&dims),
		"provider_hourly_values must be a hypertable")
	require.Equal(t, 1, dims)

	var interval pgtype.Interval
	require.NoError(t, pool.QueryRow(ctx,
		`select time_interval from timescaledb_information.dimensions
		  where hypertable_name = 'provider_hourly_values'`).Scan(&interval))
	require.Equal(t, 30*24*time.Hour, chunkInterval(t, interval))
}

// TestF2IntervalColumnSurvivesCompressedChunk is the F2 Task 5 acceptance
// test from task-5-brief.md step 1: interval_generation_kwh survives
// compression exactly, and 00012/00013's add-column/drop-column pair
// genuinely round-trips — `migrate down 2` (00013 then 00012) followed by
// `migrate up` (replaying both) leaves the base row intact and the column
// back in place.
func TestF2IntervalColumnSurvivesCompressedChunk(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)
	analyzerID := f2seedAnalyzer(t, ctx, pool, companyID, buildingID, "GEN-COMPRESS-1")

	// Older than meter_readings' 90-day compression threshold (migration
	// 00004), so compress_chunk below has something to act on.
	old := time.Now().UTC().Add(-120 * 24 * time.Hour).Truncate(time.Second)
	_, err := pool.Exec(ctx,
		`insert into meter_readings (analyzer_id, ts, kind, active_import, source_provider, interval_generation_kwh)
		 values ($1, $2, 'load_profile', '100.0000', 'pm5340', '0.6250')`,
		analyzerID, old)
	require.NoError(t, err)

	var got string
	require.NoError(t, pool.QueryRow(ctx,
		`select interval_generation_kwh::text from meter_readings where analyzer_id = $1 and ts = $2`,
		analyzerID, old).Scan(&got))
	require.Equal(t, "0.6250", got, "must read back exactly, before compression")

	// Compress the chunk directly, per aggregates_integration_test.go's
	// TestCompressedChunksStillReturnCorrectRows reasoning: waiting on the
	// background compression policy is a timing dependency this phase has
	// already paid for once.
	var compressedChunks []string
	rows, err := pool.Query(ctx,
		`select compress_chunk(c)::text from show_chunks('meter_readings', older_than => interval '90 days') c`)
	require.NoError(t, err)
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		compressedChunks = append(compressedChunks, name)
	}
	require.NoError(t, rows.Err())
	rows.Close()
	require.NotEmpty(t, compressedChunks, "the fixture reading is 120 days old and must land in a chunk older than 90 days")

	got = ""
	require.NoError(t, pool.QueryRow(ctx,
		`select interval_generation_kwh::text from meter_readings where analyzer_id = $1 and ts = $2`,
		analyzerID, old).Scan(&got))
	require.Equal(t, "0.6250", got, "must read back exactly, from a COMPRESSED chunk")

	// The migration round-trip: down 2 (00013 then 00012 — a single down 1
	// would only revert 00013) then up (replays both). This proves the
	// add-column/drop-column pair itself is reversible against a real,
	// already-compressed chunk, which a fresh `up`-only database never
	// exercises. seedCompanyAndBuilding's rows, and the reading inserted
	// above, live in tables 00012/00013 do not touch (companies, buildings,
	// analyzers, meter_readings itself), so they survive the round-trip
	// undisturbed — only the interval_generation_kwh COLUMN is dropped and
	// re-added, which necessarily resets it to NULL for every existing row,
	// since dropping a column discards its data.
	//
	// A FRESH pool is used for the post-round-trip queries below, rather
	// than reusing the one above, so that nothing about this proof can
	// depend on how any one pgx connection caches a query plan across the
	// schema change — the round trip is real, not an artefact of one
	// connection's cache.
	require.NoError(t, postgres.MigrateDownN(ctx, dsn, testfixtures.DiscardLogger(), 2))
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))

	pool2 := testfixtures.NewPool(t, dsn)
	var analyzerStillThere uuid.UUID
	require.NoError(t, pool2.QueryRow(ctx,
		`select id from analyzers where id = $1`, analyzerID).Scan(&analyzerStillThere),
		"the analyzer row itself must survive the down-2/up round trip untouched")

	var activeImport string
	var newColumn *string
	require.NoError(t, pool2.QueryRow(ctx,
		`select active_import::text, interval_generation_kwh::text from meter_readings
		  where analyzer_id = $1 and ts = $2`, analyzerID, old).Scan(&activeImport, &newColumn),
		"re-reading the same row after the round trip must still find it")
	require.Equal(t, "100.0000", activeImport, "every OTHER column on the row must be untouched by the round trip")
	require.Nil(t, newColumn, "dropping and re-adding the column necessarily loses its own data — "+
		"the round trip proves the SCHEMA change reverses cleanly, not that a dropped column's data survives its own drop")
}

// f2seedAnalyzer inserts one analyzer under buildingID and returns its id.
// installationNumber must be unique per test/company to avoid colliding with
// analyzers(provider, provider_subtype, installation_number).
func f2seedAnalyzer(t *testing.T, ctx context.Context, pool *pgxpool.Pool, companyID, buildingID uuid.UUID, installationNumber string) uuid.UUID {
	t.Helper()
	var analyzerID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into analyzers (company_id, building_id, provider, provider_subtype, installation_number)
		 values ($1, $2, 'pm5340', 'Baskent', $3) returning id`,
		companyID, buildingID, installationNumber).Scan(&analyzerID))
	return analyzerID
}
