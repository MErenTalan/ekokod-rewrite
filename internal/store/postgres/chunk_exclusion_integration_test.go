//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
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

// captured is the SQL and arguments a repository really sent.
type captured struct {
	mu    sync.Mutex
	sql   string
	args  []any
	match string
}

func (c *captured) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, c.match) {
		c.mu.Lock()
		c.sql, c.args = d.SQL, d.Args
		c.mu.Unlock()
	}
	return ctx
}

func (c *captured) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// chunksScanned counts the distinct chunk relations in a JSON plan.
func chunksScanned(t *testing.T, plan []byte) map[string]bool {
	t.Helper()
	var doc []map[string]any
	require.NoError(t, json.Unmarshal(plan, &doc))
	chunks := map[string]bool{}
	var walk func(n map[string]any)
	walk = func(n map[string]any) {
		if rel, ok := n["Relation Name"].(string); ok && strings.Contains(rel, "_chunk") {
			chunks[rel] = true
		}
		if kids, ok := n["Plans"].([]any); ok {
			for _, k := range kids {
				walk(k.(map[string]any))
			}
		}
	}
	walk(doc[0]["Plan"].(map[string]any))
	return chunks
}

// explainAs EXPLAINs the captured statement with its own arguments.
func explainAs(t *testing.T, pool *pgxpool.Pool, c *captured) map[string]bool {
	t.Helper()
	c.mu.Lock()
	sql, args := c.sql, c.args
	c.mu.Unlock()
	require.NotEmpty(t, sql, "the repository ran the query")
	var plan []byte
	require.NoError(t, pool.QueryRow(context.Background(), "explain (format json) "+sql, args...).Scan(&plan))
	return chunksScanned(t, plan)
}

// TestRangeQueriesExcludeChunks is F15b R465: with ten weeks of readings (ten
// weekly chunks), the repositories' one-day queries plan against only the
// chunks that overlap the day.
func TestRangeQueriesExcludeChunks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, base, 1961)
	analyzer := tenant.Analyzers[0]
	start := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	_, err := base.Exec(ctx, `insert into meter_readings (analyzer_id, ts, kind, active_import, source_provider, multiplier_applied)
		select $1, g, 'load_profile', extract(epoch from g - $2::timestamptz) / 3600, $3, 1
		from generate_series($2::timestamptz, $2::timestamptz + interval '70 days', interval '1 hour') g`,
		analyzer.ID, start, analyzer.Provider)
	require.NoError(t, err)
	var chunks int
	require.NoError(t, base.QueryRow(ctx, `select count(*) from timescaledb_information.chunks where hypertable_name = 'meter_readings'`).Scan(&chunks))
	require.GreaterOrEqual(t, chunks, 10, "the fixture spans many chunks")
	_, err = base.Exec(ctx, `call refresh_continuous_aggregate('consumption_hourly', null, null)`)
	require.NoError(t, err)

	cfg, err := pgxpool.ParseConfig(base.Config().ConnString())
	require.NoError(t, err)
	tracer := &captured{}
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	sc := store.SystemScope(tenant.Company.ID)
	day := store.TimeRange{From: start.AddDate(0, 0, 30), To: start.AddDate(0, 0, 31)}

	tracer.match = "from meter_readings mr"
	readings, err := postgres.NewReadingRepository(pool).Range(ctx, sc, analyzer.ID, day, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.Len(t, readings, 24)
	scanned := explainAs(t, pool, tracer)
	require.NotEmpty(t, scanned)
	require.LessOrEqual(t, len(scanned), 2, "meter_readings: only the chunks overlapping the day, got %v", scanned)

	tracer.match = "from consumption_hourly"
	_, err = postgres.NewAnalyticsRepository(pool).ConsumptionHourly(ctx, sc, []uuid.UUID{analyzer.ID}, day)
	require.NoError(t, err)
	scanned = explainAs(t, pool, tracer)
	require.LessOrEqual(t, len(scanned), 3, "consumption_hourly: the materialized chunk(s) for the day plus the real-time tail, got %v", scanned)
}
