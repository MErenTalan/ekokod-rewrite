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

func (f fakeReadings) BulkInsert(context.Context, store.Scope, []model.MeterReading) (int, int, error) {
	return 0, 0, nil
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
