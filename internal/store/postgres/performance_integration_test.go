//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// F1 acceptance criterion under test in this file:
// "Inserting 1,000,000 synthetic readings across 100 analyzers completes and
// produces the expected chunk count; a 1-month range query for one analyzer
// touches only the expected chunks (verified with EXPLAIN)."
//
// GATING. This test is guarded TWO ways, deliberately:
//   - testing.Short() — the literal mechanism task-13-brief.md's Step 1
//     pseudocode names.
//   - EKOKOD_PERF=1 — because `go test ./... -tags=integration -race
//     -count=1` (make test-integration) does NOT pass -short, testing.Short()
//     alone would leave this test running on every ordinary integration
//     invocation, which is exactly the "ordinary integration run does not
//     execute the 1M-row test by default" requirement this is meant to
//     satisfy. A build tag was rejected: the F1 acceptance gate command runs
//     `go test ./internal/store/postgres/ -tags=integration ... -run
//     'TestOneMillion|...'`, and a test hidden behind an extra build tag
//     would not even compile under plain -tags=integration, silently
//     matching zero tests. An env-var opt-in keeps the test reachable by
//     name under the one tag every other integration test already uses, and
//     `make test-perf` sets the var explicitly. `make ci` never runs
//     integration tests at all, so its runtime is unaffected either way.
const (
	// perfChunkTimeInterval must match migration 00004's
	// meter_readings chunk_time_interval.
	perfChunkTimeInterval = 7 * 24 * time.Hour
	// perfNumPartitions must match migration 00004's meter_readings
	// number_partitions.
	perfNumPartitions       = 16
	perfAnalyzerCount       = 100
	perfReadingsPerAnalyzer = 10_000
)

// perfWeekIndex returns the index of the chunk_time_interval-wide time slice
// t falls in, under TimescaleDB's default time-dimension origin.
//
// This was verified empirically against timescale/timescaledb:2.30.0-pg16
// before writing this test: a hypertable created with no explicit
// `create_hypertable(..., chunk_time_interval => ...)` origin gets chunk
// boundaries that are exact multiples of chunk_time_interval measured from
// the Unix epoch (1970-01-01T00:00:00Z) — e.g. a chunk covering
// 2026-09-10T00:00:00Z .. 2026-09-17T00:00:00Z for a 7-day interval, and
// extract(epoch from '2026-09-10T00:00:00Z') / 86400 / 7 is exactly 2958,
// an integer. perfWeekIndex is therefore a pure function of t and the
// interval: it needs no database round trip and does not depend on which
// chunks the test run actually created, which is what makes it a legitimate
// independent prediction rather than an echo of the code under test.
func perfWeekIndex(t time.Time) int64 {
	return t.Unix() / int64(perfChunkTimeInterval/time.Second)
}

func TestOneMillionReadingsChunkLayout(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: inserts 1,000,000 rows")
	}
	if os.Getenv("EKOKOD_PERF") != "1" {
		t.Skip("perf suite: set EKOKOD_PERF=1 (see `make test-perf`) to run this test")
	}

	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)
	analyzerIDs := seedPerfAnalyzers(t, ctx, pool, companyID, buildingID, perfAnalyzerCount)

	// AdminScope (AllBuildings): BulkInsert's own visibility check must see
	// every one of the 100 analyzers, all under one building.
	scope := store.Scope{CompanyID: companyID, AllBuildings: true}
	require.True(t, scope.Valid())

	repo := postgres.NewReadingRepository(pool)

	// 100 analyzers x 10,000 hourly readings = 1,000,000 rows. Every
	// analyzer shares the identical timestamp series, so every one of the
	// perfWeekIndex(end)-perfWeekIndex(start)+1 weekly slices holds data
	// from every analyzer — the condition the chunk-count derivation below
	// depends on (see its comment).
	start := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Duration(perfReadingsPerAnalyzer-1) * time.Hour)

	insertStart := time.Now()
	for _, analyzerID := range analyzerIDs {
		rows := make([]model.MeterReading, perfReadingsPerAnalyzer)
		for i := range rows {
			rows[i] = readingsRow(analyzerID, start.Add(time.Duration(i)*time.Hour),
				model.ReadingKindLoadProfile, fmt.Sprintf("%d.0000", 1000+i), "1")
		}
		inserted, updated, err := repo.BulkInsert(ctx, scope, rows)
		require.NoError(t, err)
		require.Equal(t, perfReadingsPerAnalyzer, inserted)
		require.Zero(t, updated)
	}
	insertElapsed := time.Since(insertStart)
	t.Logf("inserted %d readings across %d analyzers in %s",
		perfAnalyzerCount*perfReadingsPerAnalyzer, perfAnalyzerCount, insertElapsed)

	var totalRows int64
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from meter_readings`).Scan(&totalRows))
	require.EqualValues(t, perfAnalyzerCount*perfReadingsPerAnalyzer, totalRows)

	// --- chunk count: computed from the span and the migration's own -------
	// --- configuration, never pinned to an observed number -----------------
	//
	// meter_readings is space-partitioned into 16 hash buckets of
	// analyzer_id (00004:51-54). 100 analyzers hashed into 16 partitions do
	// NOT guarantee all 16 are occupied — pigeonhole gives no such
	// guarantee — so the expected chunk count is (number of 7-day slices
	// the data spans) x (number of DISTINCT partitions the 100 analyzer ids
	// ACTUALLY hash into), not weeks x 16.
	//
	// distinctPartitions is computed with TimescaleDB's own partitioning
	// hash function, _timescaledb_functions.get_partition_hash, against the
	// dimension_slice ranges Postgres already has for meter_readings'
	// analyzer_id dimension. Those ranges are a pure function of
	// number_partitions (16 fixed-width buckets over the hash's int4
	// range) — verified empirically to be data-INdependent boundaries, even
	// though the *rows* in dimension_slice are only materialised lazily as
	// chunks touching them are created — so joining the known 100 ids
	// against them is an independent computation, not a re-read of "how
	// many chunks did this run create".
	var distinctPartitions int
	require.NoError(t, pool.QueryRow(ctx, `
		with slices as (
			select ds.range_start, ds.range_end
			from _timescaledb_catalog.dimension d
			join _timescaledb_catalog.dimension_slice ds on ds.dimension_id = d.id
			join _timescaledb_catalog.hypertable h on h.id = d.hypertable_id
			where h.table_name = 'meter_readings' and d.column_name = 'analyzer_id'
		)
		select count(distinct s.range_start)
		from unnest($1::uuid[]) as a(id)
		join slices s
		  on _timescaledb_functions.get_partition_hash(a.id) >= s.range_start
		 and _timescaledb_functions.get_partition_hash(a.id) <  s.range_end
	`, analyzerIDs).Scan(&distinctPartitions))
	require.Positive(t, distinctPartitions, "at least one partition must be occupied")
	require.LessOrEqual(t, distinctPartitions, perfNumPartitions)

	numWeeks := perfWeekIndex(end) - perfWeekIndex(start) + 1
	expectedChunks := int(numWeeks) * distinctPartitions

	var actualChunks int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from timescaledb_information.chunks where hypertable_name = 'meter_readings'`,
	).Scan(&actualChunks))

	require.Equal(t, expectedChunks, actualChunks,
		"chunk count must be (weeks spanned = %d) x (occupied hash partitions = %d); "+
			"a mismatch means either chunk_time_interval/number_partitions drifted from "+
			"migration 00004, or the uniform per-analyzer coverage this derivation assumes broke",
		numWeeks, distinctPartitions)

	// --- EXPLAIN: a one-month, single-analyzer query must exclude ----------
	// --- chunks — assert on the plan, never on wall-clock time -------------
	queryAnalyzer := analyzerIDs[0]
	queryFrom := start.Add(60 * 24 * time.Hour)   // well inside the generated span
	queryTo := queryFrom.Add(30 * 24 * time.Hour) // one month

	touchedChunks := explainTouchedChunks(t, ctx, pool, scope, queryAnalyzer, model.ReadingKindLoadProfile, queryFrom, queryTo)
	require.NotEmpty(t, touchedChunks, "the query must touch at least one chunk")
	require.Less(t, len(touchedChunks), actualChunks,
		"a one-month single-analyzer query must name strictly fewer chunks than the "+
			"table's total (%d touched of %d total) — otherwise chunk exclusion is not "+
			"working and the query is reading the whole table", len(touchedChunks), actualChunks)

	starts, ends := chunkRanges(t, ctx, pool, touchedChunks)
	for _, name := range touchedChunks {
		rs, re := starts[name], ends[name]
		require.True(t, rs.Before(queryTo) && re.After(queryFrom),
			"chunk %s (%s .. %s) does not overlap the query window %s .. %s: a chunk "+
				"outside the queried range was scanned", name, rs, re, queryFrom, queryTo)
	}
}

// seedPerfAnalyzers inserts n analyzers directly (BulkInsert's own tests
// already cover NewTenant's shape; this test needs a company-scale analyzer
// count NewTenant does not build), all visible to companyID/buildingID.
func seedPerfAnalyzers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, companyID, buildingID uuid.UUID, n int) []uuid.UUID {
	t.Helper()
	ids := make([]uuid.UUID, n)
	for i := range ids {
		require.NoError(t, pool.QueryRow(ctx, `
			insert into analyzers (company_id, building_id, provider, provider_subtype, installation_number)
			values ($1, $2, 'osos', 'Baskent', $3)
			returning id`,
			companyID, buildingID, fmt.Sprintf("PERF-%d", i)).Scan(&ids[i]))
	}
	return ids
}

// rangeQueryTracer is a pgx.QueryTracer that records the SQL text and bound
// arguments of every query pgx sends over the pool it is attached to.
// explainTouchedChunks uses it to capture the EXACT statement
// ReadingRepository.Range hands to Postgres, so the EXPLAIN this test runs
// is over the real repository query path, not a hand-copied stand-in for
// it.
type rangeQueryTracer struct {
	mu      sync.Mutex
	queries []struct {
		sql  string
		args []any
	}
}

func (rt *rangeQueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.queries = append(rt.queries, struct {
		sql  string
		args []any
	}{sql: data.SQL, args: data.Args})
	return ctx
}

func (rt *rangeQueryTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// explainTouchedChunks runs EXPLAIN (FORMAT JSON) over the EXACT statement
// ReadingRepository.Range sends to Postgres for this call — including
// ReadingRange's company_id and all_buildings/building_ids scope predicates
// (queries/readings.sql) — and returns every "_hyper_*_chunk" relation the
// plan names, at any nesting depth (chunk exclusion at planning time removes
// non-matching chunks from the Append node's Plans list rather than adding a
// filter node, so the remaining "Plans" entries ARE the touched-chunk list).
//
// The SQL text and bound args are captured off the wire with a
// pgx.QueryTracer attached to a pool built for this call only (copied from
// the caller's pool config, so it targets the same isolated database): a
// hand-written copy of ReadingRange's SQL previously used here dropped the
// company_id and all_buildings/building_ids predicates readings.sql actually
// has, so it silently could not have caught a regression in either one. A
// tracer, rather than referencing sqlcgen's unexported `readingRange`
// constant directly, also captures the ARGS sqlc binds (company_id,
// all_buildings, building_ids, and the Kind/Timestamptz conversions
// ReadingRange's Params type applies) exactly as the repository sends them,
// which this test package cannot reconstruct by hand without risking the
// same kind of drift the SQL text itself just had.
func explainTouchedChunks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, scope store.Scope, analyzerID uuid.UUID, kind model.ReadingKind, from, to time.Time) []string {
	t.Helper()

	tracer := &rangeQueryTracer{}
	tracedCfg := pool.Config()
	tracedCfg.ConnConfig.Tracer = tracer
	tracedPool, err := pgxpool.NewWithConfig(ctx, tracedCfg)
	require.NoError(t, err, "building a traced pool for EXPLAIN capture")
	defer tracedPool.Close()

	repo := postgres.NewReadingRepository(tracedPool)
	rangeRows, err := repo.Range(ctx, scope, analyzerID, store.TimeRange{From: from, To: to}, kind)
	require.NoError(t, err)
	require.NotEmpty(t, rangeRows, "the captured query must return rows for its plan to be meaningful")

	tracer.mu.Lock()
	captured := tracer.queries
	tracer.mu.Unlock()

	var capturedSQL string
	var capturedArgs []any
	found := false
	for _, q := range captured {
		if !strings.Contains(q.sql, "from meter_readings mr") {
			continue
		}
		require.False(t, found, "expected exactly one ReadingRange query traced on the dedicated pool, found a second")
		capturedSQL, capturedArgs = q.sql, q.args
		found = true
	}
	require.True(t, found, "ReadingRange's query was not captured off the traced pool")

	var planJSON []byte
	require.NoError(t, pool.QueryRow(ctx, "explain (format json) "+capturedSQL, capturedArgs...).Scan(&planJSON))

	var plan []struct {
		Plan map[string]any `json:"Plan"`
	}
	require.NoError(t, json.Unmarshal(planJSON, &plan))
	require.Len(t, plan, 1, "EXPLAIN (FORMAT JSON) always returns exactly one top-level plan")

	var names []string
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		if rel, ok := node["Relation Name"].(string); ok && strings.HasPrefix(rel, "_hyper_") {
			names = append(names, rel)
		}
		if subs, ok := node["Plans"].([]any); ok {
			for _, s := range subs {
				if m, ok := s.(map[string]any); ok {
					walk(m)
				}
			}
		}
	}
	walk(plan[0].Plan)
	return names
}

// chunkRanges looks up each named chunk's own [range_start, range_end) —
// independent, catalog-level ground truth for where a chunk the plan named
// actually sits in time, used to check that EXPLAIN named no chunk outside
// the queried window.
func chunkRanges(t *testing.T, ctx context.Context, pool *pgxpool.Pool, names []string) (starts, ends map[string]time.Time) {
	t.Helper()
	starts = make(map[string]time.Time, len(names))
	ends = make(map[string]time.Time, len(names))
	rows, err := pool.Query(ctx, `
		select chunk_name, range_start, range_end
		from timescaledb_information.chunks
		where hypertable_name = 'meter_readings' and chunk_name = any($1::text[])`, names)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var name string
		var rs, re time.Time
		require.NoError(t, rows.Scan(&name, &rs, &re))
		starts[name] = rs
		ends[name] = re
	}
	require.NoError(t, rows.Err())
	return starts, ends
}
