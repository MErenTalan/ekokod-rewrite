package consumption_test

// This file is the package's ONE shared-test-helper file (I-9): Tasks 8, 9
// and 11a reuse what is here and never redefine a helper with the same
// name, matching internal/domain/energy's and internal/service/loadprofile's
// own helpers_test.go convention.
//
// Names that could plausibly collide with a helper Task 11a's
// refresh_test.go / refresh_integration_test.go independently needs
// (insertReading, newTenant, ctx, and anything shaped like them) are either
// avoided entirely (every test below builds its own local ctx rather than
// sharing a package-level one) or given a "paths"-prefixed, clearly
// path-specific name (pathsSeedReading, pathsInsertReadings). See the task-7
// report for the full list.

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// dec parses a decimal literal, panicking on a typo (every literal in this
// package's tests is a fixed constant, so a parse failure is a test bug).
func dec(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

// testLog is the *slog.Logger every AnalyticsDeps/BillingDeps in this
// package's tests uses. slog.DiscardHandler is used directly rather than
// testfixtures.DiscardLogger to keep this package's unit tests free of a
// dependency on internal/testfixtures where a real one is not otherwise
// needed; the integration tests in paths_integration_test.go, which already
// depend on testfixtures for NewIsolatedDB/NewTenant, use
// testfixtures.DiscardLogger directly instead of this helper.
func testLog(t testing.TB) *slog.Logger {
	t.Helper()
	return slog.New(slog.DiscardHandler)
}

// --- fakeAnalytics: store.AnalyticsRepository -------------------------------

// fakeAnalytics is a configurable store.AnalyticsRepository fake. rows is
// the default every Consumption* method returns when its own
// level-specific field is nil; a level-specific field (hourlyRows,
// dailyRows, monthlyRows, yearlyRows) overrides it. ProductionDaily/Monthly
// are never used by this package and always return nil, nil.
// calls, when non-nil, records every (method, Scope, analyzerIDs, TimeRange)
// this fakeAnalytics call receives, in call order (I-1, I-7): a test can
// inspect it afterwards to assert the exact range/scope a caller queried
// with, without needing a bespoke recording type per assertion.
type analyticsCall struct {
	method      string
	scope       store.Scope
	analyzerIDs []uuid.UUID
	r           store.TimeRange
}

type fakeAnalytics struct {
	rows        []model.ConsumptionBucket
	hourlyRows  []model.ConsumptionBucket
	dailyRows   []model.ConsumptionBucket
	monthlyRows []model.ConsumptionBucket
	yearlyRows  []model.ConsumptionBucket
	err         error

	calls *[]analyticsCall

	// t and wantScope, when both set, fail the test immediately (I-7) if
	// any method is called with a Scope other than wantScope — a mutation
	// that substitutes a different Scope on one call turns this red.
	t         *testing.T
	wantScope *store.Scope
}

func (f fakeAnalytics) record(method string, s store.Scope, ids []uuid.UUID, r store.TimeRange) {
	if f.t != nil && f.wantScope != nil {
		f.t.Helper()
		require.Equal(f.t, *f.wantScope, s, "%s called with a different Scope than the caller's own", method)
	}
	if f.calls != nil {
		*f.calls = append(*f.calls, analyticsCall{method: method, scope: s, analyzerIDs: ids, r: r})
	}
}

func (f fakeAnalytics) ConsumptionHourly(_ context.Context, s store.Scope, ids []uuid.UUID, r store.TimeRange) ([]model.ConsumptionBucket, error) {
	f.record("ConsumptionHourly", s, ids, r)
	if f.hourlyRows != nil {
		return f.hourlyRows, f.err
	}
	return f.rows, f.err
}

func (f fakeAnalytics) ConsumptionDaily(_ context.Context, s store.Scope, ids []uuid.UUID, r store.TimeRange) ([]model.ConsumptionBucket, error) {
	f.record("ConsumptionDaily", s, ids, r)
	if f.dailyRows != nil {
		return f.dailyRows, f.err
	}
	return f.rows, f.err
}

func (f fakeAnalytics) ConsumptionMonthly(_ context.Context, s store.Scope, ids []uuid.UUID, r store.TimeRange) ([]model.ConsumptionBucket, error) {
	f.record("ConsumptionMonthly", s, ids, r)
	if f.monthlyRows != nil {
		return f.monthlyRows, f.err
	}
	return f.rows, f.err
}

func (f fakeAnalytics) ConsumptionYearly(_ context.Context, s store.Scope, ids []uuid.UUID, r store.TimeRange) ([]model.ConsumptionBucket, error) {
	f.record("ConsumptionYearly", s, ids, r)
	if f.yearlyRows != nil {
		return f.yearlyRows, f.err
	}
	return f.rows, f.err
}

func (f fakeAnalytics) ProductionDaily(context.Context, store.Scope, []uuid.UUID, store.TimeRange) ([]model.PlantProductionBucket, error) {
	return nil, nil
}

func (f fakeAnalytics) ProductionMonthly(context.Context, store.Scope, []uuid.UUID, store.TimeRange) ([]model.PlantProductionBucket, error) {
	return nil, nil
}

// noAnalytics panics on every method (I-6): used for validation tests on the
// Analytics path, so a validation check that runs after the first I/O call
// (rather than before, as MaxBuckets and the other checks must) fails the
// test immediately instead of quietly returning a fake's zero value.
type noAnalytics struct{}

func (noAnalytics) ConsumptionHourly(context.Context, store.Scope, []uuid.UUID, store.TimeRange) ([]model.ConsumptionBucket, error) {
	panic("consumption: validation must run before any AnalyticsRepository.ConsumptionHourly call")
}

func (noAnalytics) ConsumptionDaily(context.Context, store.Scope, []uuid.UUID, store.TimeRange) ([]model.ConsumptionBucket, error) {
	panic("consumption: validation must run before any AnalyticsRepository.ConsumptionDaily call")
}

func (noAnalytics) ConsumptionMonthly(context.Context, store.Scope, []uuid.UUID, store.TimeRange) ([]model.ConsumptionBucket, error) {
	panic("consumption: validation must run before any AnalyticsRepository.ConsumptionMonthly call")
}

func (noAnalytics) ConsumptionYearly(context.Context, store.Scope, []uuid.UUID, store.TimeRange) ([]model.ConsumptionBucket, error) {
	panic("consumption: validation must run before any AnalyticsRepository.ConsumptionYearly call")
}

func (noAnalytics) ProductionDaily(context.Context, store.Scope, []uuid.UUID, store.TimeRange) ([]model.PlantProductionBucket, error) {
	panic("consumption: validation must run before any AnalyticsRepository.ProductionDaily call")
}

func (noAnalytics) ProductionMonthly(context.Context, store.Scope, []uuid.UUID, store.TimeRange) ([]model.PlantProductionBucket, error) {
	panic("consumption: validation must run before any AnalyticsRepository.ProductionMonthly call")
}

// --- fakeReadings: store.ReadingRepository ----------------------------------

// fakeReadings is a configurable store.ReadingRepository fake, keyed by
// kind. Range filters byKind[kind] to the half-open TimeRange; BoundaryReadings
// scans byKind[kind] for the last reading at or before start/end — the same
// rule energy.SelectBoundary implements, reproduced independently here (not
// via a domain-package call) so this fake exercises the SAME CONTRACT the
// real repository promises without depending on the code under test.
// Readings in byKind must already be in ascending ts order, exactly as the
// real ReadingRange query's ORDER BY guarantees, and this fake does not sort
// them — a test that wants to prove ordering matters supplies an unsorted
// slice on purpose.
type fakeReadings struct {
	byKind map[model.ReadingKind][]model.MeterReading
	err    error

	// bulkInsertErr, when set, makes BulkInsert fail without writing
	// anything — this fix round's C4/I2 tests use it to prove a failed
	// insert never marks the anomaly resolved.
	bulkInsertErr error
	// inserted, when non-nil, records every row BulkInsert is called with,
	// in call order — a test can inspect it without needing byKind
	// round-tripping.
	inserted *[]model.MeterReading

	// t and wantScope, when both set, fail the test immediately (I-7) if
	// any method is called with a Scope other than wantScope — a mutation
	// that substitutes a different (even if still valid) Scope on one
	// repository call turns this red, rather than relying on an accidental
	// ErrNotFound from an unrelated sibling call.
	t         *testing.T
	wantScope *store.Scope
}

func (f fakeReadings) checkScope(s store.Scope) {
	if f.t == nil || f.wantScope == nil {
		return
	}
	f.t.Helper()
	require.Equal(f.t, *f.wantScope, s, "called with a different Scope than the caller's own")
}

// BulkInsert writes rows into byKind (re-sorted ascending by ts afterward,
// matching the real repository's own ORDER BY precondition), so a test can
// register a reset through ResolveAnomaly and then observe it via Range —
// byKind must already be a non-nil map (even if empty for a kind) for this
// round-trip to be visible: assigning into a nil map from a value-receiver
// copy would only mutate that copy's own reference, never the caller's.
func (f fakeReadings) BulkInsert(_ context.Context, s store.Scope, rows []model.MeterReading) (int, int, error) {
	f.checkScope(s)
	if f.bulkInsertErr != nil {
		return 0, 0, f.bulkInsertErr
	}
	if f.inserted != nil {
		*f.inserted = append(*f.inserted, rows...)
	}
	if f.byKind != nil {
		for _, r := range rows {
			f.byKind[r.Kind] = append(f.byKind[r.Kind], r)
		}
		for kind := range f.byKind {
			kind := kind
			sort.Slice(f.byKind[kind], func(i, j int) bool { return f.byKind[kind][i].Ts.Before(f.byKind[kind][j].Ts) })
		}
	}
	return len(rows), 0, nil
}

func (f fakeReadings) Range(_ context.Context, s store.Scope, _ uuid.UUID, r store.TimeRange, kind model.ReadingKind) ([]model.MeterReading, error) {
	f.checkScope(s)
	if f.err != nil {
		return nil, f.err
	}
	var out []model.MeterReading
	for _, row := range f.byKind[kind] {
		if !row.Ts.Before(r.From) && row.Ts.Before(r.To) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f fakeReadings) BoundaryReadings(_ context.Context, s store.Scope, _ uuid.UUID, kind model.ReadingKind, start, end time.Time) (*model.MeterReading, *model.MeterReading, error) {
	f.checkScope(s)
	if f.err != nil {
		return nil, nil, f.err
	}
	rows := f.byKind[kind]
	var startReading, endReading *model.MeterReading
	for i := range rows {
		if !rows[i].Ts.After(start) {
			r := rows[i]
			startReading = &r
		}
		if !rows[i].Ts.After(end) {
			r := rows[i]
			endReading = &r
		}
	}
	return startReading, endReading, nil
}

func (f fakeReadings) Latest(context.Context, store.Scope, uuid.UUID, store.TimeRange, model.ReadingKind) (*model.MeterReading, error) {
	return nil, nil
}

// noReadings panics on every method (I-6): used for validation tests on the
// Billing path, so a validation check that runs after the first I/O call
// fails the test immediately instead of quietly returning a fake's zero
// value.
type noReadings struct{}

func (noReadings) BulkInsert(context.Context, store.Scope, []model.MeterReading) (int, int, error) {
	panic("consumption: validation must run before any ReadingRepository.BulkInsert call")
}

func (noReadings) Range(context.Context, store.Scope, uuid.UUID, store.TimeRange, model.ReadingKind) ([]model.MeterReading, error) {
	panic("consumption: validation must run before any ReadingRepository.Range call")
}

func (noReadings) BoundaryReadings(context.Context, store.Scope, uuid.UUID, model.ReadingKind, time.Time, time.Time) (*model.MeterReading, *model.MeterReading, error) {
	panic("consumption: validation must run before any ReadingRepository.BoundaryReadings call")
}

func (noReadings) Latest(context.Context, store.Scope, uuid.UUID, store.TimeRange, model.ReadingKind) (*model.MeterReading, error) {
	panic("consumption: validation must run before any ReadingRepository.Latest call")
}

// --- no-op AnomalyRepository / OpsRepository --------------------------------

// noAnomalies panics on every WRITE method: Billing.Consumption must never
// create, fetch-by-id or resolve an anomaly (that is Task 8's
// ConsumptionAndRecord/ResolveAnomaly, on the same type) — a panic surfaces
// that violation immediately rather than letting a nil-ish fake silently
// return zero values.
//
// List is the one exception (Task 8, C-6): Consumption itself now reads
// resolved manual_override/accepted anomalies to substitute their values
// (applyResolvedAnomalies, anomalies.go) — a read Task 7 never needed but
// R61 never forbade (R61 only ever forbids Billing reaching an
// AnalyticsRepository). Returning (nil, nil) here matches "no anomalies
// exist" exactly like a real repository would for a period with none, so
// every Task 7 test that never seeds a resolved anomaly is unaffected.
type noAnomalies struct{}

func (noAnomalies) Get(context.Context, store.Scope, uuid.UUID) (model.ConsumptionAnomaly, error) {
	panic("consumption: Task 7's Billing.Consumption must never call AnomalyRepository.Get")
}

func (noAnomalies) List(context.Context, store.Scope, store.AnomalyFilter) ([]model.ConsumptionAnomaly, error) {
	return nil, nil
}

func (noAnomalies) Create(context.Context, store.Scope, model.ConsumptionAnomaly) (model.ConsumptionAnomaly, error) {
	panic("consumption: Task 7's Billing.Consumption must never call AnomalyRepository.Create")
}

func (noAnomalies) Resolve(context.Context, store.Scope, uuid.UUID, uuid.UUID, string, []byte, time.Time) (model.ConsumptionAnomaly, error) {
	panic("consumption: Task 7's Billing.Consumption must never call AnomalyRepository.Resolve")
}

// noOps panics on every method, for the same reason noAnomalies does.
type noOps struct{}

func (noOps) StartRun(context.Context, store.Scope, model.JobRun) (model.JobRun, error) {
	panic("consumption: Task 7's Billing.Consumption must never call OpsRepository.StartRun")
}

func (noOps) FinishRun(context.Context, store.Scope, uuid.UUID, string, int32, int32, int32, *string, []byte, time.Time) (model.JobRun, error) {
	panic("consumption: Task 7's Billing.Consumption must never call OpsRepository.FinishRun")
}

func (noOps) GetRun(context.Context, store.Scope, uuid.UUID) (model.JobRun, error) {
	panic("consumption: Task 7's Billing.Consumption must never call OpsRepository.GetRun")
}

func (noOps) ListRuns(context.Context, store.Scope, store.JobRunFilter) ([]model.JobRun, error) {
	panic("consumption: Task 7's Billing.Consumption must never call OpsRepository.ListRuns")
}

func (noOps) AppendMessage(context.Context, store.Scope, model.OperationalMessage) (model.OperationalMessage, error) {
	panic("consumption: Task 7's Billing.Consumption must never call OpsRepository.AppendMessage")
}

func (noOps) ListMessages(context.Context, store.Scope, store.MessageFilter) ([]model.OperationalMessage, error) {
	panic("consumption: Task 7's Billing.Consumption must never call OpsRepository.ListMessages")
}

// --- reading fixture builder -------------------------------------------------

// readingRow builds a model.MeterReading of kind at ts, with the given
// register values (active_import, t1_import, ... — a subset of
// energy.Register names as strings, for terseness in test tables) and
// MultiplierApplied fixed at 1. Deliberately NOT named insertReading: no row
// is written anywhere by this helper, and Task 11a's own integration tests
// may want a same-named "insert" helper of their own without colliding with
// this one's shape or its name.
func readingRow(analyzerID uuid.UUID, ts time.Time, kind model.ReadingKind, values map[string]string) model.MeterReading {
	r := model.MeterReading{
		AnalyzerID:        analyzerID,
		Ts:                ts,
		Kind:              kind,
		MultiplierApplied: decimal.RequireFromString("1"),
		SourceProvider:    model.IntegrationProviderOSOS,
		IngestedAt:        ts,
	}
	for reg, v := range values {
		d := decimal.RequireFromString(v)
		switch reg {
		case "active_import":
			r.ActiveImport = &d
		case "reactive_inductive_import":
			r.ReactiveInductiveImport = &d
		case "reactive_capacitive_import":
			r.ReactiveCapacitiveImport = &d
		case "t1_import":
			r.T1Import = &d
		case "t2_import":
			r.T2Import = &d
		case "t3_import":
			r.T3Import = &d
		case "active_export":
			r.ActiveExport = &d
		case "max_demand_kw":
			r.MaxDemandKw = &d
		default:
			panic("readingRow: unknown register " + reg)
		}
	}
	return r
}

// pathsSeedReading inserts one meter reading through a real ReadingRepository,
// for paths_integration_test.go's real-database tests. Named with the
// "paths" prefix per the task-7 report's collision-avoidance convention.
func pathsSeedReading(t *testing.T, ctx context.Context, repo store.ReadingRepository, scope store.Scope, r model.MeterReading) {
	t.Helper()
	_, _, err := repo.BulkInsert(ctx, scope, []model.MeterReading{r})
	require.NoError(t, err)
}

// pathsNewReadingRepo and pathsNewAnalyticsRepo are thin, path-specific
// aliases over the postgres constructors, so paths_integration_test.go reads
// without an extra postgres.New… qualifier at every call site.
func pathsNewReadingRepo(pool *pgxpool.Pool) store.ReadingRepository {
	return postgres.NewReadingRepository(pool)
}
func pathsNewAnalyticsRepo(pool *pgxpool.Pool) store.AnalyticsRepository {
	return postgres.NewAnalyticsRepository(pool)
}

// --- fakeAnomalies: store.AnomalyRepository ---------------------------------

// containsUUID reports whether id is one of ids.
var (
	_ store.AnomalyRepository  = (*fakeAnomalies)(nil)
	_ store.OpsRepository      = (*fakeOps)(nil)
	_ store.AnalyzerRepository = fakeAnalyzers{}
	_ store.UserRepository     = fakeUsers{}
)

func containsUUID(ids []uuid.UUID, id uuid.UUID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// fakeAnomalies is an in-memory, configurable store.AnomalyRepository used
// by this fix round's unit tests (C1-C4, I1-I3, R97): it reproduces the real
// repository's own pagination contract — a default page limit (100,
// matching postgres's own anomalyDefaultPageLimit unless a test overrides
// it), rows ordered by period_start desc — so a test can prove C3's full
// paging fix and C4's dedup-key fix without a real database. Safe for
// concurrent use (I1's deterministic race test needs that).
type fakeAnomalies struct {
	mu sync.Mutex

	rows []model.ConsumptionAnomaly
	err  error

	// defaultLimit mimics AnomalyRepository's own "a zero Limit means the
	// repository's default" contract; 0 here defaults to 100, matching
	// production, so a caller that (bug) never sets an explicit Page.Limit
	// reproduces the review's P4/P5 cutoff exactly.
	defaultLimit int32

	// listBarrier, when non-nil, is invoked at the START of every List call,
	// before the read — I1's deterministic race test uses it to hold two
	// concurrent ConsumptionAndRecord calls at the check step until both
	// have arrived, so removing the dedup lock is guaranteed (not merely
	// likely) to let both callers see "no existing row" and both create one.
	listBarrier func()

	// resolveErr, when set, makes Resolve fail for exactly the ids it
	// names — I2's ordering test uses this to fail the F2 cascade's own
	// Resolve call and assert the F3 row's own Resolve is never reached.
	resolveErr map[uuid.UUID]error
}

func (f *fakeAnomalies) seed(a model.ConsumptionAnomaly) model.ConsumptionAnomaly {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	f.rows = append(f.rows, a)
	return a
}

func (f *fakeAnomalies) get(id uuid.UUID) (model.ConsumptionAnomaly, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.rows {
		if a.ID == id {
			return a, true
		}
	}
	return model.ConsumptionAnomaly{}, false
}

func (f *fakeAnomalies) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.ConsumptionAnomaly, error) {
	a, ok := f.get(id)
	if !ok {
		return model.ConsumptionAnomaly{}, store.ErrNotFound
	}
	return a, nil
}

func (f *fakeAnomalies) List(_ context.Context, _ store.Scope, filt store.AnomalyFilter) ([]model.ConsumptionAnomaly, error) {
	if f.listBarrier != nil {
		f.listBarrier()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}

	var matched []model.ConsumptionAnomaly
	for _, a := range f.rows {
		if len(filt.AnalyzerIDs) > 0 && !containsUUID(filt.AnalyzerIDs, a.AnalyzerID) {
			continue
		}
		if filt.Reason != nil && a.Reason != *filt.Reason {
			continue
		}
		if filt.Unresolved && a.ResolvedAt != nil {
			continue
		}
		if filt.Range != nil {
			if a.PeriodStart.Before(filt.Range.From) || !a.PeriodStart.Before(filt.Range.To) {
				continue
			}
		}
		matched = append(matched, a)
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].PeriodStart.After(matched[j].PeriodStart) })

	limit := filt.Page.Limit
	if limit <= 0 {
		limit = f.defaultLimit
		if limit <= 0 {
			limit = 100
		}
	}
	offset := int(filt.Page.Offset)
	if offset >= len(matched) {
		return nil, nil
	}
	end := offset + int(limit)
	if end > len(matched) {
		end = len(matched)
	}
	return append([]model.ConsumptionAnomaly(nil), matched[offset:end]...), nil
}

func (f *fakeAnomalies) Create(_ context.Context, _ store.Scope, a model.ConsumptionAnomaly) (model.ConsumptionAnomaly, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return model.ConsumptionAnomaly{}, f.err
	}
	a.ID = uuid.New()
	a.CreatedAt = time.Now()
	f.rows = append(f.rows, a)
	return a, nil
}

// resolveErrIDs, when an id is present, makes Resolve fail for exactly that
// id — I2's ordering test uses this to make the F2 cascade's own Resolve
// call fail and asserts the F3 row's own Resolve is never reached.
func (f *fakeAnomalies) Resolve(_ context.Context, _ store.Scope, id, resolvedBy uuid.UUID, resolution string, overrides []byte, at time.Time) (model.ConsumptionAnomaly, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if f.rows[i].ID != id {
			continue
		}
		if f.resolveErr != nil {
			if err, ok := f.resolveErr[id]; ok {
				return model.ConsumptionAnomaly{}, err
			}
		}
		t := at
		rb := resolvedBy
		res := resolution
		f.rows[i].ResolvedAt = &t
		f.rows[i].ResolvedBy = &rb
		f.rows[i].Resolution = &res
		f.rows[i].OverrideValues = overrides
		return f.rows[i], nil
	}
	return model.ConsumptionAnomaly{}, store.ErrNotFound
}

// --- fakeOps: store.OpsRepository --------------------------------------------

// fakeOps is an in-memory store.OpsRepository fake for this fix round's
// message-related tests (M1: a dedup hit against an unresolved row backfills
// a missing message).
type fakeOps struct {
	mu       sync.Mutex
	messages []model.OperationalMessage
	nextID   int64
	err      error
}

func (f *fakeOps) StartRun(context.Context, store.Scope, model.JobRun) (model.JobRun, error) {
	panic("fakeOps: StartRun unused by this fix round's tests")
}
func (f *fakeOps) FinishRun(context.Context, store.Scope, uuid.UUID, string, int32, int32, int32, *string, []byte, time.Time) (model.JobRun, error) {
	panic("fakeOps: FinishRun unused by this fix round's tests")
}
func (f *fakeOps) GetRun(context.Context, store.Scope, uuid.UUID) (model.JobRun, error) {
	panic("fakeOps: GetRun unused by this fix round's tests")
}
func (f *fakeOps) ListRuns(context.Context, store.Scope, store.JobRunFilter) ([]model.JobRun, error) {
	panic("fakeOps: ListRuns unused by this fix round's tests")
}

func (f *fakeOps) AppendMessage(_ context.Context, _ store.Scope, m model.OperationalMessage) (model.OperationalMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return model.OperationalMessage{}, f.err
	}
	f.nextID++
	m.ID = f.nextID
	m.CreatedAt = time.Now()
	f.messages = append(f.messages, m)
	return m, nil
}

func (f *fakeOps) ListMessages(_ context.Context, _ store.Scope, filt store.MessageFilter) ([]model.OperationalMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.OperationalMessage
	for _, m := range f.messages {
		if filt.RelatedID != nil && (m.RelatedID == nil || *m.RelatedID != *filt.RelatedID) {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// --- fakeAnalyzers: store.AnalyzerRepository --------------------------------

// fakeAnalyzers is a fixed-provider store.AnalyzerRepository fake:
// buildResetReading (resolve.go) only ever calls Get, for the analyzer's
// own Provider.
type fakeAnalyzers struct {
	analyzer model.Analyzer
	err      error
}

func (f fakeAnalyzers) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.Analyzer, error) {
	if f.err != nil {
		return model.Analyzer{}, f.err
	}
	a := f.analyzer
	a.ID = id
	return a, nil
}
func (f fakeAnalyzers) GetByInstallation(context.Context, store.Scope, model.IntegrationProvider, string, string) (model.Analyzer, error) {
	panic("fakeAnalyzers: GetByInstallation unused by this fix round's tests")
}
func (f fakeAnalyzers) List(context.Context, store.Scope, store.AnalyzerFilter) ([]model.Analyzer, error) {
	panic("fakeAnalyzers: List unused by this fix round's tests")
}
func (f fakeAnalyzers) Create(context.Context, store.Scope, model.Analyzer) (model.Analyzer, error) {
	panic("fakeAnalyzers: Create unused by this fix round's tests")
}
func (f fakeAnalyzers) Update(context.Context, store.Scope, model.Analyzer) (model.Analyzer, error) {
	panic("fakeAnalyzers: Update unused by this fix round's tests")
}
func (f fakeAnalyzers) SoftDelete(context.Context, store.Scope, uuid.UUID, time.Time) error {
	panic("fakeAnalyzers: SoftDelete unused by this fix round's tests")
}
func (f fakeAnalyzers) TouchLastReading(context.Context, store.Scope, uuid.UUID, time.Time) error {
	panic("fakeAnalyzers: TouchLastReading unused by this fix round's tests")
}

// --- fakeUsers: store.UserRepository -----------------------------------------

// fakeUsers is a configurable store.UserRepository fake for I2/P6's
// resolvedBy-validated-before-any-write test: Get returns ErrNotFound for
// any id not in validIDs, exactly like a real, scoped repository would for a
// user outside the caller's own company.
type fakeUsers struct {
	validIDs map[uuid.UUID]bool
}

func (f fakeUsers) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.User, error) {
	if f.validIDs[id] {
		return model.User{ID: id}, nil
	}
	return model.User{}, store.ErrNotFound
}
func (f fakeUsers) List(context.Context, store.Scope, store.UserFilter) ([]model.User, error) {
	panic("fakeUsers: List unused by this fix round's tests")
}
func (f fakeUsers) Create(context.Context, store.Scope, model.User) (model.User, error) {
	panic("fakeUsers: Create unused by this fix round's tests")
}
func (f fakeUsers) Update(context.Context, store.Scope, model.User) (model.User, error) {
	panic("fakeUsers: Update unused by this fix round's tests")
}
func (f fakeUsers) SoftDelete(context.Context, store.Scope, uuid.UUID, time.Time) error {
	panic("fakeUsers: SoftDelete unused by this fix round's tests")
}
func (f fakeUsers) SetPassword(context.Context, store.Scope, uuid.UUID, string, time.Time) error {
	panic("fakeUsers: SetPassword unused by this fix round's tests")
}
func (f fakeUsers) PasswordHistory(context.Context, store.Scope, uuid.UUID, int32) ([]model.PasswordHistoryEntry, error) {
	panic("fakeUsers: PasswordHistory unused by this fix round's tests")
}
func (f fakeUsers) RecordLogin(context.Context, store.Scope, uuid.UUID, time.Time) error {
	panic("fakeUsers: RecordLogin unused by this fix round's tests")
}
