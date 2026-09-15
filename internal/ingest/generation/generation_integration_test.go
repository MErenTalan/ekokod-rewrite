//go:build integration

package generation_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest/generation"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// genTestNow is comfortably after every fixed reading timestamp these tests
// use, so the forward recomputation window (now+FutureTolerance) always
// covers them.
var genTestNow = time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)

func genTS(s string) time.Time {
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("generation_test: bad timestamp literal " + s + ": " + err.Error())
	}
	return tm
}

// genTestNewPM5340Analyzer creates a fresh analyzer with provider pm5340.
//
// Analyzers.Update cannot switch an existing analyzer's provider — the
// generated AnalyzerUpdate SQL (internal/store/postgres/queries/analyzers.sql)
// deliberately has no `provider` column in its SET list, since
// (provider, provider_subtype, installation_number) is the natural key
// GetByInstallation resolves and Update never touches identity columns.
// The brief's Step 2 text says "switched … via Analyzers.Update"; this is a
// resolvable test-fixture detail, not a change to the Accumulator's
// algorithm or interfaces, so rather than stopping with NEEDS_CONTEXT this
// creates a NEW pm5340 analyzer through Analyzers.Create instead — the same
// observable state (a pm5340 analyzer visible to the tenant's scope) the
// brief's setup wants.
func genTestNewPM5340Analyzer(t *testing.T, ctx context.Context, analyzers *postgres.AnalyzerRepository, sc store.Scope, companyID, buildingID uuid.UUID, installation string) model.Analyzer {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a, err := analyzers.Create(ctx, sc, model.Analyzer{
		CompanyID: companyID, BuildingID: &buildingID,
		Provider: model.IntegrationProviderPM5340, ProviderSubtype: "Baskent",
		InstallationNumber: installation, MeterMultiplier: decimal.RequireFromString("1"),
		IsActive: true, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	return a
}

// genTestReading builds a load_profile PM5340 row: IntervalGenerationKwh set
// (or nil for ""), ActiveExport always nil — exactly how Task 10's pipeline
// persists a freshly-fetched row, before any hook has touched it.
func genTestReading(analyzerID uuid.UUID, tsStr, intervalKwh string) model.MeterReading {
	r := model.MeterReading{
		AnalyzerID: analyzerID, Ts: genTS(tsStr), Kind: model.ReadingKindLoadProfile,
		MultiplierApplied: decimal.NewFromInt(1), SourceProvider: model.IntegrationProviderPM5340,
	}
	if intervalKwh != "" {
		v := decimal.RequireFromString(intervalKwh)
		r.IntervalGenerationKwh = &v
	}
	return r
}

// genTestFindByTs returns the row at tsStr from rows, failing the test if it
// is absent — every test below asserts specific rows by timestamp.
func genTestFindByTs(t *testing.T, rows []model.MeterReading, tsStr string) model.MeterReading {
	t.Helper()
	want := genTS(tsStr)
	for _, r := range rows {
		if r.Ts.Equal(want) {
			return r
		}
	}
	t.Fatalf("no row at %s among %d rows", tsStr, len(rows))
	return model.MeterReading{}
}

// genTestAllReadings reads every load_profile row for analyzerID over a
// window wide enough to cover every fixed timestamp these tests use.
func genTestAllReadings(t *testing.T, ctx context.Context, readings *postgres.ReadingRepository, sc store.Scope, analyzerID uuid.UUID) []model.MeterReading {
	t.Helper()
	rows, err := readings.Range(ctx, sc, analyzerID,
		store.TimeRange{From: genTS("2020-01-01T00:00:00Z"), To: genTS("2030-01-01T00:00:00Z")}, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	return rows
}

func genTestRequireActiveExport(t *testing.T, rows []model.MeterReading, tsStr, want string) {
	t.Helper()
	r := genTestFindByTs(t, rows, tsStr)
	require.NotNil(t, r.ActiveExport, "row at %s: active_export is nil", tsStr)
	require.True(t, decimal.RequireFromString(want).Equal(*r.ActiveExport),
		"row at %s: want active_export %s, got %s", tsStr, want, r.ActiveExport.String())
}

// genTestCountingAnchors wraps a real store.GenerationRepository and counts
// SetAnchor calls, so TestGenerationCreatesInitialAnchorOnce can assert the
// initial anchor is written exactly once rather than infer it indirectly.
type genTestCountingAnchors struct {
	store.GenerationRepository
	setAnchorCalls int
}

func (g *genTestCountingAnchors) SetAnchor(ctx context.Context, s store.Scope, a model.GenerationAnchor) error {
	g.setAnchorCalls++
	return g.GenerationRepository.SetAnchor(ctx, s, a)
}

type genTestRepos struct {
	pool      *pgxpool.Pool
	analyzers *postgres.AnalyzerRepository
	readings  *postgres.ReadingRepository
	anchors   *postgres.GenerationRepository
}

func genTestNewRepos(pool *pgxpool.Pool) genTestRepos {
	return genTestRepos{
		pool:      pool,
		analyzers: postgres.NewAnalyzerRepository(pool),
		readings:  postgres.NewReadingRepository(pool),
		anchors:   postgres.NewGenerationRepository(pool),
	}
}

// ---------------------------------------------------------------------------

// TestGenerationAccumulatesAcrossAGap is the F2 acceptance criterion "PM5340
// interval generation accumulates into the cumulative register correctly
// across a gap" (removed-behaviour 22).
func TestGenerationAccumulatesAcrossAGap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 11001)
	repos := genTestNewRepos(pool)
	analyzer := genTestNewPM5340Analyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, "GAP-1")

	require.NoError(t, repos.anchors.SetAnchor(ctx, tn.Scope, model.GenerationAnchor{
		AnalyzerID: analyzer.ID, AnchorTs: genTS("2026-09-01T09:45:00Z"),
		ActiveExport: decimal.RequireFromString("1000"), Source: "operator",
	}))

	acc := generation.New(repos.readings, repos.anchors, clock.NewFake(genTestNow))

	// Batch 1: 10:00=1.0kWh, 10:15=1.0kWh.
	batch1 := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T10:00:00Z", "1.0"),
		genTestReading(analyzer.ID, "2026-09-01T10:15:00Z", "1.0"),
	}
	_, _, err := repos.readings.BulkInsert(ctx, tn.Scope, batch1)
	require.NoError(t, err)
	require.NoError(t, acc.AfterPersist(ctx, tn.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T10:00:00Z"), genTS("2026-09-01T10:15:00Z")))

	// Gap: 10:30 and 10:45 never arrive. Batch 2: 11:00=0.5, 11:15=0.5.
	batch2 := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T11:00:00Z", "0.5"),
		genTestReading(analyzer.ID, "2026-09-01T11:15:00Z", "0.5"),
	}
	_, _, err = repos.readings.BulkInsert(ctx, tn.Scope, batch2)
	require.NoError(t, err)
	require.NoError(t, acc.AfterPersist(ctx, tn.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T11:00:00Z"), genTS("2026-09-01T11:15:00Z")))

	rows := genTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID)
	require.Len(t, rows, 4)
	genTestRequireActiveExport(t, rows, "2026-09-01T10:00:00Z", "1001")
	genTestRequireActiveExport(t, rows, "2026-09-01T10:15:00Z", "1002")
	genTestRequireActiveExport(t, rows, "2026-09-01T11:00:00Z", "1002.5")
	genTestRequireActiveExport(t, rows, "2026-09-01T11:15:00Z", "1003")

	// Recompute must yield identical values.
	require.NoError(t, acc.Recompute(ctx, tn.Scope, analyzer.ID))
	rows = genTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID)
	require.Len(t, rows, 4)
	genTestRequireActiveExport(t, rows, "2026-09-01T10:00:00Z", "1001")
	genTestRequireActiveExport(t, rows, "2026-09-01T10:15:00Z", "1002")
	genTestRequireActiveExport(t, rows, "2026-09-01T11:00:00Z", "1002.5")
	genTestRequireActiveExport(t, rows, "2026-09-01T11:15:00Z", "1003")
}

// TestGenerationOutOfOrderBatchRecomputesLaterRows is the F2 acceptance
// criterion "… across … an out-of-order batch" (removed-behaviour 22).
func TestGenerationOutOfOrderBatchRecomputesLaterRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 11002)
	repos := genTestNewRepos(pool)
	analyzer := genTestNewPM5340Analyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, "OOO-1")

	require.NoError(t, repos.anchors.SetAnchor(ctx, tn.Scope, model.GenerationAnchor{
		AnalyzerID: analyzer.ID, AnchorTs: genTS("2026-09-01T09:45:00Z"),
		ActiveExport: decimal.Zero, Source: "operator",
	}))

	acc := generation.New(repos.readings, repos.anchors, clock.NewFake(genTestNow))

	// Batch 1 (later): 11:00=2, 11:15=2.
	later := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T11:00:00Z", "2"),
		genTestReading(analyzer.ID, "2026-09-01T11:15:00Z", "2"),
	}
	_, _, err := repos.readings.BulkInsert(ctx, tn.Scope, later)
	require.NoError(t, err)
	require.NoError(t, acc.AfterPersist(ctx, tn.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T11:00:00Z"), genTS("2026-09-01T11:15:00Z")))

	rows := genTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID)
	genTestRequireActiveExport(t, rows, "2026-09-01T11:00:00Z", "2")
	genTestRequireActiveExport(t, rows, "2026-09-01T11:15:00Z", "4")

	// Batch 2 (earlier, arrives second): 10:00=1, 10:15=1.
	earlier := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T10:00:00Z", "1"),
		genTestReading(analyzer.ID, "2026-09-01T10:15:00Z", "1"),
	}
	_, _, err = repos.readings.BulkInsert(ctx, tn.Scope, earlier)
	require.NoError(t, err)
	require.NoError(t, acc.AfterPersist(ctx, tn.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T10:00:00Z"), genTS("2026-09-01T10:15:00Z")))

	rows = genTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID)
	require.Len(t, rows, 4)
	genTestRequireActiveExport(t, rows, "2026-09-01T10:00:00Z", "1")
	genTestRequireActiveExport(t, rows, "2026-09-01T10:15:00Z", "2")
	genTestRequireActiveExport(t, rows, "2026-09-01T11:00:00Z", "4")
	genTestRequireActiveExport(t, rows, "2026-09-01T11:15:00Z", "6")
}

// TestGenerationDuplicateFetchIsStable is the F2 acceptance criterion "… a
// duplicate fetch" (removed-behaviour 22).
func TestGenerationDuplicateFetchIsStable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 11003)
	repos := genTestNewRepos(pool)
	analyzer := genTestNewPM5340Analyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, "DUP-1")

	require.NoError(t, repos.anchors.SetAnchor(ctx, tn.Scope, model.GenerationAnchor{
		AnalyzerID: analyzer.ID, AnchorTs: genTS("2026-09-01T09:45:00Z"),
		ActiveExport: decimal.Zero, Source: "operator",
	}))

	acc := generation.New(repos.readings, repos.anchors, clock.NewFake(genTestNow))

	batch := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T10:00:00Z", "1"),
		genTestReading(analyzer.ID, "2026-09-01T10:15:00Z", "1"),
	}
	_, _, err := repos.readings.BulkInsert(ctx, tn.Scope, batch)
	require.NoError(t, err)
	require.NoError(t, acc.AfterPersist(ctx, tn.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T10:00:00Z"), genTS("2026-09-01T10:15:00Z")))

	// A LATER batch, already fetched and accumulated, and untouched by the
	// duplicate below — its own stability is part of what this test proves:
	// a duplicate re-fetch of an EARLIER window must not perturb data the
	// duplicate never re-sent, however wide a `to` the caller happens to
	// pass (see the doc on AfterPersist: `to` may legitimately be wider than
	// the exact persisted range of the duplicated rows alone, e.g. a
	// chunk-window-shaped caller).
	later := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T11:00:00Z", "1"),
		genTestReading(analyzer.ID, "2026-09-01T11:15:00Z", "1"),
	}
	_, _, err = repos.readings.BulkInsert(ctx, tn.Scope, later)
	require.NoError(t, err)
	require.NoError(t, acc.AfterPersist(ctx, tn.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T11:00:00Z"), genTS("2026-09-01T11:15:00Z")))

	first := genTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID)
	require.Len(t, first, 4)
	firstStrs := map[string]string{
		"2026-09-01T10:00:00Z": genTestFindByTs(t, first, "2026-09-01T10:00:00Z").ActiveExport.String(),
		"2026-09-01T10:15:00Z": genTestFindByTs(t, first, "2026-09-01T10:15:00Z").ActiveExport.String(),
		"2026-09-01T11:00:00Z": genTestFindByTs(t, first, "2026-09-01T11:00:00Z").ActiveExport.String(),
		"2026-09-01T11:15:00Z": genTestFindByTs(t, first, "2026-09-01T11:15:00Z").ActiveExport.String(),
	}

	// The FIRST batch, fetched again: BulkInsert's upsert sets active_export
	// back to NULL on both its rows (the freshly-fetched provider row never
	// carries ActiveExport), then AfterPersist runs again with the same
	// `from`. `to` is widened to cover the already-computed later batch too
	// — a caller is free to pass a `to` this wide (see AfterPersist's doc),
	// and this is exactly the shape a base-selection bug that searches "at
	// or before to" instead of "strictly before from" needs to be exposed:
	// with a narrow `to` (matching only the duplicated rows themselves,
	// which the real ingestion pipeline always passes) that bug is inert,
	// because every row up to a narrow `to` is one this very call just
	// nulled. Task 11's mutation-proof step applies exactly this mutation
	// and records the FAIL this wider `to` produces.
	dup := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T10:00:00Z", "1"),
		genTestReading(analyzer.ID, "2026-09-01T10:15:00Z", "1"),
	}
	inserted, updated, err := repos.readings.BulkInsert(ctx, tn.Scope, dup)
	require.NoError(t, err)
	require.Equal(t, 0, inserted)
	require.Equal(t, 2, updated)

	// Confirm the upsert really did null active_export out, so the
	// idempotence proof below is not vacuous.
	midway := genTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID)
	require.Nil(t, genTestFindByTs(t, midway, "2026-09-01T10:00:00Z").ActiveExport)
	require.Nil(t, genTestFindByTs(t, midway, "2026-09-01T10:15:00Z").ActiveExport)

	require.NoError(t, acc.AfterPersist(ctx, tn.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T10:00:00Z"), genTS("2026-09-01T11:15:00Z")))

	second := genTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID)
	require.Len(t, second, 4, "row count must be unchanged")
	for _, tsStr := range []string{
		"2026-09-01T10:00:00Z", "2026-09-01T10:15:00Z",
		"2026-09-01T11:00:00Z", "2026-09-01T11:15:00Z",
	} {
		got := genTestFindByTs(t, second, tsStr)
		require.NotNil(t, got.ActiveExport, "row at %s: active_export must not be left null by the duplicate fetch", tsStr)
		require.Equal(t, firstStrs[tsStr], got.ActiveExport.String(),
			"row at %s: byte-for-byte identical after the duplicate fetch", tsStr)
	}
}

// TestGenerationRecomputeAfterHistoryCorrection changes one interval value
// and confirms every later cumulative row shifts by exactly the correction.
func TestGenerationRecomputeAfterHistoryCorrection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 11004)
	repos := genTestNewRepos(pool)
	analyzer := genTestNewPM5340Analyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, "CORR-1")

	require.NoError(t, repos.anchors.SetAnchor(ctx, tn.Scope, model.GenerationAnchor{
		AnalyzerID: analyzer.ID, AnchorTs: genTS("2026-09-01T09:45:00Z"),
		ActiveExport: decimal.Zero, Source: "operator",
	}))

	acc := generation.New(repos.readings, repos.anchors, clock.NewFake(genTestNow))

	batch := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T10:00:00Z", "1"),
		genTestReading(analyzer.ID, "2026-09-01T10:15:00Z", "1"),
		genTestReading(analyzer.ID, "2026-09-01T10:30:00Z", "1"),
	}
	_, _, err := repos.readings.BulkInsert(ctx, tn.Scope, batch)
	require.NoError(t, err)
	require.NoError(t, acc.AfterPersist(ctx, tn.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T10:00:00Z"), genTS("2026-09-01T10:30:00Z")))

	before := genTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID)
	genTestRequireActiveExport(t, before, "2026-09-01T10:00:00Z", "1")
	genTestRequireActiveExport(t, before, "2026-09-01T10:15:00Z", "2")
	genTestRequireActiveExport(t, before, "2026-09-01T10:30:00Z", "3")

	// Correct the middle row's interval value from 1 to 1.5 — a +0.5 shift.
	corrected := genTestReading(analyzer.ID, "2026-09-01T10:15:00Z", "1.5")
	_, _, err = repos.readings.BulkInsert(ctx, tn.Scope, []model.MeterReading{corrected})
	require.NoError(t, err)

	require.NoError(t, acc.Recompute(ctx, tn.Scope, analyzer.ID))

	after := genTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID)
	genTestRequireActiveExport(t, after, "2026-09-01T10:00:00Z", "1")   // unaffected: before the correction
	genTestRequireActiveExport(t, after, "2026-09-01T10:15:00Z", "2.5") // 2 + 0.5
	genTestRequireActiveExport(t, after, "2026-09-01T10:30:00Z", "3.5") // 3 + 0.5
}

// TestGenerationNullIntervalStaysNull: a row with no interval value keeps
// interval_generation_kwh NULL, and its active_export carries the running
// value forward unchanged.
func TestGenerationNullIntervalStaysNull(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 11005)
	repos := genTestNewRepos(pool)
	analyzer := genTestNewPM5340Analyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, "NULL-1")

	require.NoError(t, repos.anchors.SetAnchor(ctx, tn.Scope, model.GenerationAnchor{
		AnalyzerID: analyzer.ID, AnchorTs: genTS("2026-09-01T09:45:00Z"),
		ActiveExport: decimal.RequireFromString("50"), Source: "operator",
	}))

	acc := generation.New(repos.readings, repos.anchors, clock.NewFake(genTestNow))

	batch := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T10:00:00Z", "1"),
		genTestReading(analyzer.ID, "2026-09-01T10:15:00Z", ""), // unavailable interval register
		genTestReading(analyzer.ID, "2026-09-01T10:30:00Z", "1"),
	}
	_, _, err := repos.readings.BulkInsert(ctx, tn.Scope, batch)
	require.NoError(t, err)
	require.NoError(t, acc.AfterPersist(ctx, tn.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T10:00:00Z"), genTS("2026-09-01T10:30:00Z")))

	rows := genTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID)
	genTestRequireActiveExport(t, rows, "2026-09-01T10:00:00Z", "51")
	mid := genTestFindByTs(t, rows, "2026-09-01T10:15:00Z")
	require.Nil(t, mid.IntervalGenerationKwh, "interval_generation_kwh must stay NULL")
	require.NotNil(t, mid.ActiveExport)
	require.True(t, decimal.RequireFromString("51").Equal(*mid.ActiveExport),
		"active_export must carry the running value forward: got %s", mid.ActiveExport.String())
	genTestRequireActiveExport(t, rows, "2026-09-01T10:30:00Z", "52")
}

// TestGenerationCreatesInitialAnchorOnce: the first AfterPersist call with
// no prior anchor creates exactly one; a second call reuses it.
func TestGenerationCreatesInitialAnchorOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 11006)
	repos := genTestNewRepos(pool)
	analyzer := genTestNewPM5340Analyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, "INIT-1")

	countingAnchors := &genTestCountingAnchors{GenerationRepository: repos.anchors}
	acc := generation.New(repos.readings, countingAnchors, clock.NewFake(genTestNow))

	batch1 := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T10:00:00Z", "1"),
		genTestReading(analyzer.ID, "2026-09-01T10:15:00Z", "1"),
	}
	_, _, err := repos.readings.BulkInsert(ctx, tn.Scope, batch1)
	require.NoError(t, err)
	require.NoError(t, acc.AfterPersist(ctx, tn.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T10:00:00Z"), genTS("2026-09-01T10:15:00Z")))
	require.Equal(t, 1, countingAnchors.setAnchorCalls)

	anchorAfterFirst, err := repos.anchors.Anchor(ctx, tn.Scope, analyzer.ID)
	require.NoError(t, err)
	require.True(t, anchorAfterFirst.AnchorTs.Equal(genTS("2026-09-01T09:45:00Z")),
		"anchor must be 15m before the first-ever row: got %s", anchorAfterFirst.AnchorTs)
	require.True(t, decimal.Zero.Equal(anchorAfterFirst.ActiveExport))
	require.Equal(t, "initial", anchorAfterFirst.Source)

	batch2 := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T11:00:00Z", "1"),
	}
	_, _, err = repos.readings.BulkInsert(ctx, tn.Scope, batch2)
	require.NoError(t, err)
	require.NoError(t, acc.AfterPersist(ctx, tn.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T11:00:00Z"), genTS("2026-09-01T11:00:00Z").Add(time.Nanosecond)))
	require.Equal(t, 1, countingAnchors.setAnchorCalls, "the second call must not recreate the anchor")

	anchorAfterSecond, err := repos.anchors.Anchor(ctx, tn.Scope, analyzer.ID)
	require.NoError(t, err)
	require.True(t, anchorAfterSecond.AnchorTs.Equal(anchorAfterFirst.AnchorTs), "anchor must be unchanged")
	require.True(t, anchorAfterSecond.ActiveExport.Equal(anchorAfterFirst.ActiveExport), "anchor must be unchanged")
}

// TestGenerationIsScoped: another tenant's scope resolves the analyzer to
// ErrNotFound and rewrites nothing. Per Global Constraints / F1 binding
// ruling 4, the cross-tenant proof uses the OTHER tenant's AdminScope (the
// widest legitimate scope that tenant has), with a same-setup positive
// control under the owning tenant's own scope to prove the proof is not
// vacuous.
func TestGenerationIsScoped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 11007)
	theirs := testfixtures.NewTenant(t, ctx, pool, 11008)
	repos := genTestNewRepos(pool)
	analyzer := genTestNewPM5340Analyzer(t, ctx, repos.analyzers, mine.Scope, mine.Company.ID, mine.Buildings[0].ID, "SCOPE-1")

	batch := []model.MeterReading{
		genTestReading(analyzer.ID, "2026-09-01T10:00:00Z", "1"),
		genTestReading(analyzer.ID, "2026-09-01T10:15:00Z", "1"),
	}
	_, _, err := repos.readings.BulkInsert(ctx, mine.Scope, batch)
	require.NoError(t, err)

	acc := generation.New(repos.readings, repos.anchors, clock.NewFake(genTestNow))

	// Negative: the OTHER tenant's own AdminScope cannot reach it.
	err = acc.AfterPersist(ctx, theirs.AdminScope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T10:00:00Z"), genTS("2026-09-01T10:15:00Z"))
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repos.anchors.Anchor(ctx, mine.Scope, analyzer.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "no anchor may have been created by the cross-tenant attempt")

	rows := genTestAllReadings(t, ctx, repos.readings, mine.Scope, analyzer.ID)
	for _, r := range rows {
		require.Nil(t, r.ActiveExport, "nothing must have been rewritten by the cross-tenant attempt")
	}

	// Positive control: the owning tenant's own scope succeeds.
	require.NoError(t, acc.AfterPersist(ctx, mine.Scope, analyzer, model.ReadingKindLoadProfile,
		genTS("2026-09-01T10:00:00Z"), genTS("2026-09-01T10:15:00Z")))
	rows = genTestAllReadings(t, ctx, repos.readings, mine.Scope, analyzer.ID)
	genTestRequireActiveExport(t, rows, "2026-09-01T10:00:00Z", "1")
	genTestRequireActiveExport(t, rows, "2026-09-01T10:15:00Z", "2")
}
