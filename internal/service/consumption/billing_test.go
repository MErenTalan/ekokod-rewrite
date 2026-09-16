package consumption_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// billingT0 is an arbitrary, fixed instant this file's fake-clock tests
// anchor to.
var billingT0 = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

// newBilling builds a *consumption.Billing wired to readings, with
// AnomalyRepository and OpsRepository set to the panicking no-op fakes
// (Task 7's Consumption must never reach either — that is Task 8's
// ConsumptionAndRecord, on the same type). The clock is fixed at billingT0.
func newBilling(t *testing.T, readings fakeReadings) *consumption.Billing {
	t.Helper()
	return newBillingAt(t, readings, billingT0)
}

// newBillingAt is newBilling with an explicit clock instant, for a test
// whose own fixture window must be CLOSED under R98 (Billing drops every
// bucket whose To is after the deps clock's now) even though that window
// does not fall entirely before the file's shared billingT0 anchor.
func newBillingAt(t *testing.T, readings fakeReadings, now time.Time) *consumption.Billing {
	t.Helper()
	b, err := consumption.NewBilling(consumption.BillingDeps{
		Readings:  readings,
		Anomalies: noAnomalies{},
		Ops:       noOps{},
		Clock:     clock.NewFake(now),
		Log:       testLog(t),
	})
	require.NoError(t, err)
	return b
}

// TestBillingPathCannotReachAnAggregate is R61's symmetric guard, named in
// the plan's own guard list: Billing has no AnalyticsRepository field at
// all. fakeReadings{} has no data, so every boundary lookup comes back nil
// and no row is emitted — proving only fakeReadings was ever consulted
// (noAnomalies/noOps would panic if Consumption touched either, and there
// is no aggregate dependency present to have reached in the first place).
//
// I-4 (final review B): this behavioural probe alone is vacuous against a
// FIELD-level bypass (a poisoned field nobody's code path ever calls, or a
// narrower consumer-defined interface a real aggregate repository could
// still satisfy — guard_test.go's own I-4 fix). It now delegates to
// guard_test.go's reflection walk (the same one
// TestBillingDepsHasNoAnalyticsRepositoryField calls) FIRST, so the plan's
// own named guard also catches what the structural guard catches.
func TestBillingPathCannotReachAnAggregate(t *testing.T) {
	analyticsRepo := reflect.TypeOf((*store.AnalyticsRepository)(nil)).Elem()
	assertNoForbiddenField(t, reflect.TypeOf(consumption.BillingDeps{}), analyticsRepo)

	b := newBilling(t, fakeReadings{})

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{uuid.New()},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: billingT0, To: billingT0.Add(time.Hour)},
	}

	rows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows)
}

// TestBillingRefusesAnEmptyAnalyzerList is fail-closed, no I/O: an empty
// AnalyzerIDs means no rows, never "all" (R89).
func TestBillingRefusesAnEmptyAnalyzerList(t *testing.T) {
	b := newBilling(t, fakeReadings{})

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: nil,
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: billingT0, To: billingT0.Add(time.Hour)},
	}

	_, err := b.Consumption(ctx, scope, req)
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)
}

// --- R104(2): MaxCells replaces MaxBuckets -----------------------------

// cellsWindow returns a UTC range (Hourly, no DST in play so the bucket
// count equals the number of whole hours exactly) whose Hourly bucket count
// times analyzerCount is exactly cells — the caller picks analyzerCount and
// hours so the product lands exactly on the boundary under test, never
// merely "over".
func cellsWindow(hours int) (time.Time, time.Time) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return from, from.Add(time.Duration(hours) * time.Hour)
}

// TestBillingAcceptsExactlyMaxCells is R104(2)'s positive control on the
// Billing path: 50 analyzers (MaxAnalyzersPerRequest) times 1000 Hourly
// buckets = exactly MaxCells (50000) cells must not be refused. noReadings
// panics on any repository call, so a non-nil error here could only be
// ErrInvalidRequest, never a panic escaping as a different failure.
func TestBillingAcceptsExactlyMaxCells(t *testing.T) {
	from, to := cellsWindow(1000)
	b, err := consumption.NewBilling(consumption.BillingDeps{Readings: fakeReadings{}, Anomalies: noAnomalies{}, Ops: noOps{}, Clock: clock.NewFake(to), Log: testLog(t)})
	require.NoError(t, err)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: newUUIDs(consumption.MaxAnalyzersPerRequest),
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: from, To: to},
	}
	require.Equal(t, consumption.MaxAnalyzersPerRequest*1000, consumption.MaxCells, "the fixture must land exactly on MaxCells")
	rows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows, "the (empty) fake has no data — this only proves the request itself is accepted")
}

// TestBillingRefusesOverMaxCells is R104(2)'s own probe on the Billing
// path, replacing the deleted MaxBuckets test: 21 analyzers times 2381
// Hourly buckets is exactly MaxCells+1 (50001) cells — one analyzer, or one
// bucket, short of the cap either way would pass — refused before any I/O.
// noReadings panics on any repository call, proving the refusal happens
// before a single Range call.
func TestBillingRefusesOverMaxCells(t *testing.T) {
	from, to := cellsWindow(2381)
	b, err := consumption.NewBilling(consumption.BillingDeps{Readings: noReadings{}, Anomalies: noAnomalies{}, Ops: noOps{}, Clock: clock.NewFake(to), Log: testLog(t)})
	require.NoError(t, err)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: newUUIDs(21),
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: from, To: to},
	}
	require.Equal(t, consumption.MaxCells+1, 21*2381, "the fixture must land exactly one over MaxCells")
	_, err = b.Consumption(ctx, scope, req)
	require.ErrorIs(t, err, consumption.ErrInvalidRequest, "must be refused before any ReadingRepository call: noReadings panics on any call")
}

// --- R98: an open or future bucket is never emitted, complete or partial --

// TestBillingNeverEmitsAnOpenOrFutureBucket is R98's own acceptance probe
// (final review A I-1's exact scenario): 15-minute load_profile readings
// exist up to Jan 30 22:00 Istanbul (3872, so 3872-1000=2872 accrued so
// far). Requesting the whole January Monthly bucket ([Jan1, Feb1)) while
// the clock still reads Jan 30 22:00 — well before the bucket's own To —
// must yield NO row at all: the pre-R98 defect let the Feb-01 boundary
// resolve to the Jan 30 22:00 reading (inside R96's 36h load_profile
// tolerance) and silently returned one row, 2872, as if the month were
// closed. Advancing the clock to Feb 1 00:00 + SettleDelayMonthly (R104(1):
// a bucket is closed only once its own settle window has elapsed too, not
// merely once its own To has passed) closes it and the row appears,
// unchanged.
func TestBillingNeverEmitsAnOpenOrFutureBucket(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	jan30At22 := time.Date(2026, 1, 30, 22, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, jan30At22, model.ReadingKindLoadProfile, map[string]string{"active_import": "3872"}),
		},
	}}
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	}

	stillOpen := newBillingAt(t, readings, jan30At22)
	rows, err := stillOpen.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows, "January has not closed yet (now is Jan 30 22:00, well inside the bucket): no row, never a partial 2872")

	closed := newBillingAt(t, readings, feb1.Add(consumption.SettleDelayMonthly))
	rows, err = closed.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1, "at Feb 1 00:00 + SettleDelayMonthly the bucket has settled")
	require.NotNil(t, rows[0].Values[energy.ActiveImport])
	require.Equal(t, "2872", rows[0].Values[energy.ActiveImport].String())
}

// TestBillingSettleDelayAvoidsBillingAStaleEndBoundaryJustAfterClose is
// R104(1)'s own acceptance probe (final review X's I-A exact scenario): a
// daily-only meter's daily-kind snapshots stamp Jan 1 (1000) and Jan 31
// (1300) exactly at 00:00; February's own snapshot (1310) exists in the
// fixture but the request is run BEFORE R104(1)'s settle delay has
// elapsed. Requesting the whole January Monthly bucket at Feb 1 06:00 — six
// hours past the bucket's own To, and well inside R96's DailySnapshotTolerance
// (36h) — must yield NO row: under the plain R98 clock-only rule, the Jan
// 31 00:00 snapshot was still within tolerance of the Feb 1 bound, so the
// bucket would have silently billed 300 (1300-1000) instead of the true 310
// — a boundary the provider had not actually stamped for February yet.
// Once SettleDelayMonthly elapses (Feb 4 00:00), the bucket closes and
// bills the REAL Feb 1 00:00 snapshot: 310.
func TestBillingSettleDelayAvoidsBillingAStaleEndBoundaryJustAfterClose(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	jan31 := jan1.AddDate(0, 0, 30)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindDaily: {
			readingRow(analyzerID, jan1, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, jan31, model.ReadingKindDaily, map[string]string{"active_import": "1300"}),
			readingRow(analyzerID, feb1, model.ReadingKindDaily, map[string]string{"active_import": "1310"}),
		},
	}}
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	}

	notYetSettled := newBillingAt(t, readings, feb1.Add(6*time.Hour))
	rows, err := notYetSettled.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows, "Feb 1 06:00 is only 6h past the bucket's own To — well inside SettleDelayMonthly (72h) — so the bucket must not be billed yet, even though the stale Jan 31 snapshot is itself still within DailySnapshotTolerance")

	settled := newBillingAt(t, readings, feb1.Add(consumption.SettleDelayMonthly))
	rows, err = settled.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Values[energy.ActiveImport])
	require.Equal(t, "310", rows[0].Values[energy.ActiveImport].String(), "the true Feb 1 00:00 snapshot, never the stale Jan 31 one")
}

// TestClosedBucketExactSettleDelayEdge is R104(1)'s own exact-edge pin: a
// bucket is closed exactly when now equals its own To plus SettleDelay for
// its level — never one microsecond earlier. Hourly is used because its
// settle delay (2h) keeps the fixture simple; closedBucket itself has no
// per-level branching (SettleDelay does), so this pins the shared
// predicate directly.
func TestClosedBucketExactSettleDelayEdge(t *testing.T) {
	h := billingT0
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
		},
	}}
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: h, To: h.Add(time.Hour)},
	}
	settleAt := h.Add(time.Hour).Add(consumption.SettleDelayHourly)

	exactlyAtEdge := newBillingAt(t, readings, settleAt)
	rows, err := exactlyAtEdge.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1, "now == To + SettleDelay must be closed")

	oneMicrosecondEarly := newBillingAt(t, readings, settleAt.Add(-time.Microsecond))
	rows, err = oneMicrosecondEarly.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows, "one microsecond before To + SettleDelay must still be open")
}

// TestMonthlyPrefersBillingKindReadingsWhenPresent is R62/R63's acceptance
// test. January's billing-kind boundary readings (1000 -> 6000) cover the
// month exactly at its own boundaries, so they are preferred over the
// load_profile readings present for the SAME instants (1000 -> 1200): if
// the load_profile reading were used instead the result would be "200", not
// "5000" — the fixture deliberately makes the two paths disagree so the
// preference is unambiguous.
func TestMonthlyPrefersBillingKindReadingsWhenPresent(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindBilling: {
			readingRow(analyzerID, jan1, model.ReadingKindBilling, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, feb1, model.ReadingKindBilling, map[string]string{"active_import": "6000"}),
		},
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1200"}),
		},
	}}
	b := newBilling(t, readings)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	}

	rows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, energy.KindBilling, row.Source)
	require.NotNil(t, row.Values[energy.ActiveImport])
	require.Equal(t, "5000", row.Values[energy.ActiveImport].String(), "the utility snapshot, not the load profile")
}

// TestMonthlyFallsBackToLoadProfileWhenBillingReadingsDoNotCoverTheMonth is
// R63/R96: only ONE billing-kind reading exists, exactly at January's own
// start (distance 0, well within BillingSnapshotTolerance) — so under R96's
// per-INSTANT resolution the START boundary DOES resolve to billing (990),
// while the END boundary has no billing candidate anywhere near it (the
// same January reading is a full month away, far outside tolerance) and
// falls back to load_profile (1200). Source is load_profile — R96 always
// names the END boundary's own kind — but the VALUE, 210 (1200-990), proves
// the START actually used billing's own 990, not load_profile's 1000: M-3
// (final review B) found the original fixture used the SAME value (1000)
// on both kinds at January's start, so this test passed even under a
// mutation that fell back to load_profile on BOTH sides (the pre-R96
// whole-period rule this test's old name and doc described), because 1000
// and 1000 are indistinguishable in the result. Mutation "resolveBoundary
// never tries billing" (or the pre-R96 whole-period fallback) now gives
// 200, not 210.
func TestMonthlyFallsBackToLoadProfileWhenBillingReadingsDoNotCoverTheMonth(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindBilling: {
			readingRow(analyzerID, jan1, model.ReadingKindBilling, map[string]string{"active_import": "990"}),
		},
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1200"}),
		},
	}}
	b := newBilling(t, readings)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	}

	rows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, energy.KindLoadProfile, row.Source, "R96 names the END boundary's own kind, even though the START resolved to billing")
	require.NotNil(t, row.Values[energy.ActiveImport])
	require.Equal(t, "210", row.Values[energy.ActiveImport].String(), "1200 (load_profile end) - 990 (billing start, NOT load_profile's own 1000)")
}

// TestMonthlyMixesBillingStartWithLoadProfileEndWhenOnlyTheEndSnapshotIsStale
// is R96's amendment of the old "one stale side fails coverage for the WHOLE
// month" rule (formerly I-3/C3, back when a period's boundaries were chosen
// together, per PERIOD, never mixing kinds). A billing-kind reading exists
// near the START boundary (exactly at BillingSnapshotTolerance, still
// covering), but the reading nearest the END boundary is 4 days (96h, over
// the 72h tolerance) before February's start. Under R96 each boundary
// instant resolves independently: the START still resolves to billing
// (900), but the END — having no usable billing candidate — falls back on
// its own to load_profile (1200), giving a MIXED-kind row: 1200 - 900 = 300,
// Source = load_profile (the END boundary's own kind). This is deliberately
// no longer "200" (the pre-R96 answer of falling back to load_profile at
// BOTH ends merely because one side was stale): 300 uses the freshest
// available reading at each side, and is the more correct figure.
func TestMonthlyMixesBillingStartWithLoadProfileEndWhenOnlyTheEndSnapshotIsStale(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	dec29 := jan1.Add(-72 * time.Hour) // exactly at tolerance: still covers the start.
	jan28 := feb1.Add(-96 * time.Hour) // 4 days before February's start: stale.
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindBilling: {
			readingRow(analyzerID, dec29, model.ReadingKindBilling, map[string]string{"active_import": "900"}),
			readingRow(analyzerID, jan28, model.ReadingKindBilling, map[string]string{"active_import": "5900"}),
		},
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1200"}),
		},
	}}
	b := newBilling(t, readings)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	}

	rows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, energy.KindLoadProfile, row.Source, "the END boundary's own kind (R96), even though the START resolved to billing")
	require.NotNil(t, row.Values[energy.ActiveImport])
	require.Equal(t, "300", row.Values[energy.ActiveImport].String(), "900 (billing start) to 1200 (load_profile end)")
}

// TestBillingNeverUsesCurrentIndexReadingsAsBoundaries is R64: current_index
// never participates in §3.1 differencing. A current_index reading sits AT
// the exact end boundary (10:00), with a wildly different value (9999) that
// would corrupt the result if it were ever merged into the boundary
// candidate pool and won the end-boundary tie against the real 10:00
// load_profile reading. Since it must be ignored, the derived value is
// exactly 1040 - 1000 = 40, unaffected by its presence.
func TestBillingNeverUsesCurrentIndexReadingsAsBoundaries(t *testing.T) {
	hourStart := billingT0
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, hourStart, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, hourStart.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
		},
		model.ReadingKindCurrentIndex: {
			readingRow(analyzerID, hourStart.Add(time.Hour), model.ReadingKindCurrentIndex, map[string]string{"active_import": "9999"}),
		},
	}}
	// R98: the clock must be at or after the bucket's own To (hourStart+1h)
	// for this still-open-at-billingT0 hour to be emitted as a row at all.
	b := newBillingAt(t, readings, hourStart.Add(time.Hour).Add(consumption.SettleDelayHourly))

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: hourStart, To: hourStart.Add(time.Hour)},
	}

	rows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Values[energy.ActiveImport])
	require.Equal(t, "40", rows[0].Values[energy.ActiveImport].String(),
		"the current_index reading at the end boundary must never be considered a boundary candidate")
}

// dailyFixtureReadings builds the 25 hourly load_profile readings (00:00
// through 24:00) TestDailyDoesNotEqualTheSumOfItsHoursWhenOneHourIsSuspect
// hand-computes against, anchored at dayStart.
func dailyFixtureReadings(analyzerID uuid.UUID, dayStart time.Time) []model.MeterReading {
	values := []string{
		"1000", "1010", "1020", "1030", "1040", "1050",
		"1060", "1070", "1080", "1090", "1100", "1050",
		"1160", "1170", "1180", "1190", "1200", "1205",
		"1210", "1215", "1220", "1225", "1230", "1235",
		"1240",
	}
	rows := make([]model.MeterReading, 0, len(values))
	for i, v := range values {
		ts := dayStart.Add(time.Duration(i) * time.Hour)
		rows = append(rows, readingRow(analyzerID, ts, model.ReadingKindLoadProfile, map[string]string{"active_import": v}))
	}
	return rows
}

// billingHourlyAndDailyFor runs Billing.Consumption at Hourly and at Daily
// level over the same 24-hour window built from dailyFixtureReadings, and
// returns both result sets.
func billingHourlyAndDailyFor(t *testing.T, analyzerID uuid.UUID, dayStart time.Time, readings []model.MeterReading) (hourly []consumption.Row, daily consumption.Row) {
	t.Helper()
	fake := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: readings,
	}}
	// R98: the clock must be at or after dayStart+24h — the widest bucket
	// (Daily) this helper requests — for both the hourly and the daily
	// buckets to be closed, never dropped as still-open/future.
	b := newBillingAt(t, fake, dayStart.Add(24*time.Hour).Add(consumption.SettleDelayDaily))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	hourlyRows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: dayStart, To: dayStart.Add(24 * time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, hourlyRows, 24, "every hour has both boundaries, even the suspect one")

	dailyRows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Daily,
		Range:       store.TimeRange{From: dayStart, To: dayStart.Add(24 * time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, dailyRows, 1)

	return hourlyRows, dailyRows[0]
}

// sumOfNonNilHourly sums reg's value over every hourly row whose value is
// non-nil (excluding suspect hours), in order.
func sumOfNonNilHourly(hourly []consumption.Row, reg energy.Register) decimal.Decimal {
	total := decimal.Zero
	for _, row := range hourly {
		if v := row.Values[reg]; v != nil {
			total = total.Add(*v)
		}
	}
	return total
}

// TestDailyDoesNotEqualTheSumOfItsHoursWhenOneHourIsSuspect is C-8 / 09
// §F3's acceptance criterion: levels derive from their own boundaries,
// never by summing the level below.
//
// Hour 10 ([10:00,11:00)) is 1050-1100 = -50: a negative delta with no
// covering reset, so that hourly row is suspect (nil). Every other hourly
// window is non-negative. The daily window ([00:00,24:00)) derives
// directly from its own two boundaries: 1240-1000 = 240 — untouched by
// hour 10's problem, because the daily row never sums the hours below it.
//
// Sum of the 23 non-suspect hourly rows: the telescoping sum of all 24
// windows' deltas always equals the daily total (240) regardless of
// intermediate values, so excluding hour 10's own delta (-50) from that
// total gives 240 - (-50) = 290. 290 != 240 is the assertion that summing
// the level below is NOT how the daily figure is produced.
func TestDailyDoesNotEqualTheSumOfItsHoursWhenOneHourIsSuspect(t *testing.T) {
	loc := istanbulLoc(t)
	dayStart := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	hourly, daily := billingHourlyAndDailyFor(t, analyzerID, dayStart, dailyFixtureReadings(analyzerID, dayStart))

	require.Nil(t, hourly[10].Values[energy.ActiveImport], "hour 10's negative delta with no reset is suspect")
	require.NotNil(t, daily.Values[energy.ActiveImport])
	require.Equal(t, "240", daily.Values[energy.ActiveImport].String())

	sum := sumOfNonNilHourly(hourly, energy.ActiveImport)
	require.Equal(t, "290", sum.String())
	require.NotEqual(t, "240", sum.String(), "summing the hours is not how the daily figure is produced")
}

// --- C1: reset look-back must reach the EARLIEST of any boundary kind ------

// TestBillingMonthlyResetBeforeLoadProfileLookback is C1's acceptance test
// (review probe P1). Billing start snapshot Dec 30 00:00 = 900 (old meter;
// 48h before Jan 1, within BillingSnapshotTolerance). Last old-meter
// load_profile reading Dec 30 12:00 (36h before Jan 1) = 1000. Reset row Dec
// 31 00:00 (24h before Jan 1) = 0. New-meter load_profile at Jan 1 00:00 =
// 50. Billing end Feb 1 = 5050. True value = (1000-900) + (5050-0) = 5150.
//
// The reset sits inside the evidence window (billingStart.TS, end.TS] but
// BEFORE load_profile's OWN look-back (there is a load_profile reading
// exactly at Jan 1, so the old code's lpLookback was Jan 1 itself — 24h
// after the reset). Loading resets and priors only from lpLookback silently
// drops the reset and returns 4150 (a plain difference across it) unflagged.
// The fix loads resets and load_profile from min(lpLookback,
// billingLookback), which reaches back far enough to find both the reset and
// its load_profile "before" evidence (the Dec 30 12:00 reading).
//
// The answer must not depend on the request's own range: starting the
// request a month earlier so January is no longer the first bucket (and the
// reset already sits inside the naturally-loaded window) must give the
// exact same January row.
func TestBillingMonthlyResetBeforeLoadProfileLookback(t *testing.T) {
	loc := istanbulLoc(t)
	dec1 := time.Date(2025, 12, 1, 0, 0, 0, 0, loc)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	billingRows := []model.MeterReading{
		readingRow(analyzerID, jan1.Add(-48*time.Hour), model.ReadingKindBilling, map[string]string{"active_import": "900"}),
		readingRow(analyzerID, feb1, model.ReadingKindBilling, map[string]string{"active_import": "5050"}),
	}
	resetRows := []model.MeterReading{
		readingRow(analyzerID, jan1.Add(-24*time.Hour), model.ReadingKindReset, map[string]string{"active_import": "0"}),
	}
	lpNarrow := []model.MeterReading{
		readingRow(analyzerID, jan1.Add(-36*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "50"}),
		readingRow(analyzerID, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "5050"}),
	}
	lpWide := append([]model.MeterReading{
		readingRow(analyzerID, dec1, model.ReadingKindLoadProfile, map[string]string{"active_import": "100"}),
	}, lpNarrow...)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	run := func(lp []model.MeterReading, from, to time.Time) []consumption.Row {
		t.Helper()
		b := newBilling(t, fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
			model.ReadingKindBilling:     billingRows,
			model.ReadingKindLoadProfile: lp,
			model.ReadingKindReset:       resetRows,
		}})
		rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
			AnalyzerIDs: []uuid.UUID{analyzerID},
			Level:       energy.Monthly,
			Range:       store.TimeRange{From: from, To: to},
		})
		require.NoError(t, err)
		return rows
	}

	narrow := run(lpNarrow, jan1, feb1)
	require.Len(t, narrow, 1)
	require.Equal(t, energy.KindBilling, narrow[0].Source)
	require.NotNil(t, narrow[0].Values[energy.ActiveImport])
	require.Equal(t, "5150", narrow[0].Values[energy.ActiveImport].String(),
		"C1: the reset before load_profile's own look-back must still be found")

	wide := run(lpWide, dec1, feb1)
	require.Len(t, wide, 2, "December and January")
	require.NotNil(t, wide[1].Values[energy.ActiveImport])
	require.Equal(t, narrow[0].Values[energy.ActiveImport].String(), wide[1].Values[energy.ActiveImport].String(),
		"[Dec,Feb)'s January row must equal [Jan,Feb)'s own")
}

// --- C2(a): reset priors must be load_profile, never current_index --------

// TestBillingResetPriorsAreLoadProfileOnly is C2's unit half (review probe
// P2). Daily level: lp 00:00=1000, lp 10:00=1100 (old meter), current_index
// 11:00=1500 (a decoy that must never be treated as a prior), reset
// 12:00=0, lp 24:00=200. True = (1100-1000) + (200-0) = 300.
//
// m-6 (final fix X review): mutation (a) (priors := a current_index Range)
// does NOT produce "700 unflagged" under the current code — that claim was
// stale. It makes the reset segment's own before/after values disagree
// (the decoy 1500 in place of the real 1100 prior), which Derive reports as
// a suspect meter_reset (nil values + Suspicion), not a wrong plain number.
// The assertions below still catch the mutation either way: Suspect is no
// longer empty and the value is no longer "300".
func TestBillingResetPriorsAreLoadProfileOnly(t *testing.T) {
	loc := istanbulLoc(t)
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, day, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, day.Add(10*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1100"}),
			readingRow(analyzerID, day.Add(24*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "200"}),
		},
		model.ReadingKindCurrentIndex: {
			readingRow(analyzerID, day.Add(11*time.Hour), model.ReadingKindCurrentIndex, map[string]string{"active_import": "1500"}),
		},
		model.ReadingKindReset: {
			readingRow(analyzerID, day.Add(12*time.Hour), model.ReadingKindReset, map[string]string{"active_import": "0"}),
		},
	}}
	// R98: this fixture's day (Mar 10) is after billingT0 (Mar 1); the clock
	// must be at or after day+24h for the bucket to be closed.
	b := newBillingAt(t, readings, day.Add(24*time.Hour).Add(consumption.SettleDelayDaily))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Daily,
		Range:       store.TimeRange{From: day, To: day.Add(24 * time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].Suspect, "priors must come from load_profile, never current_index")
	require.NotNil(t, rows[0].Values[energy.ActiveImport])
	require.Equal(t, "300", rows[0].Values[energy.ActiveImport].String())
}

// --- I4: no look-back must not drop a first bucket ------------------------

// TestBillingIncludesAFirstBucketWhoseStartReadingPrecedesFrom is I-4
// (review probe P3a). Mutation (b) (lookback := first.From, dropping the
// look-back entirely) makes the reading 15 minutes before From invisible,
// so the first bucket's own start boundary resolves to nil and the row goes
// missing (not wrong — silently absent).
func TestBillingIncludesAFirstBucketWhoseStartReadingPrecedesFrom(t *testing.T) {
	loc := istanbulLoc(t)
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, day.Add(-15*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "990"}),
			readingRow(analyzerID, day.Add(45*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1030"}),
		},
	}}
	// R98: the fixture window closes at day+1h, after billingT0.
	b := newBillingAt(t, readings, day.Add(time.Hour).Add(consumption.SettleDelayHourly))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: day, To: day.Add(time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1, "a first bucket whose start reading precedes From must not be dropped")
	require.NotNil(t, rows[0].Values[energy.ActiveImport])
	require.Equal(t, "40", rows[0].Values[energy.ActiveImport].String())
}

// --- C3: START-side staleness of the billing snapshot ----------------------

// TestMonthlyFallsBackWhenTheStartSnapshotIsStale is C3 (review probe P3b),
// updated for R96: the START billing snapshot is 73h before January (just
// over BillingSnapshotTolerance), so the START boundary falls back to
// load_profile (1000) on its own; the END billing snapshot is fresh (exactly
// at February's own bound), so the END boundary still resolves to billing
// (6000). Under the pre-R96 whole-period rule this fell back to load_profile
// at BOTH ends; under R96 only the stale side does, giving a MIXED row
// (Source = billing, the END boundary's own kind) with the SAME correct
// value, 5000 — a stale START must still never let the wrong December
// figure (6000 - 0 = 6000, ten December days billed into January) through.
func TestMonthlyFallsBackWhenTheStartSnapshotIsStale(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	staleStart := jan1.Add(-73 * time.Hour)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindBilling: {
			readingRow(analyzerID, staleStart, model.ReadingKindBilling, map[string]string{"active_import": "0"}),
			readingRow(analyzerID, feb1, model.ReadingKindBilling, map[string]string{"active_import": "6000"}),
		},
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "6000"}),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, energy.KindBilling, rows[0].Source, "R96: the END boundary resolves independently and is fresh billing")
	require.Equal(t, "5000", rows[0].Values[energy.ActiveImport].String(), "a stale START snapshot must not count as covering AT THAT BOUNDARY")
}

// TestMonthlyFallsBackWhenTheStartSnapshotIsMissing is C3's second fixture,
// updated for R96: no billing reading anywhere near the START boundary at
// all (only at February's own start), so the START boundary falls back to
// load_profile (1000) on its own, while the END boundary — billing exists
// exactly at February's own bound — still resolves to billing (6000): a
// MIXED row, Source = billing, value unchanged at 5000.
func TestMonthlyFallsBackWhenTheStartSnapshotIsMissing(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindBilling: {
			readingRow(analyzerID, feb1, model.ReadingKindBilling, map[string]string{"active_import": "6000"}),
		},
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "6000"}),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, energy.KindBilling, rows[0].Source, "R96: the END boundary resolves independently and is fresh billing")
	require.Equal(t, "5000", rows[0].Values[energy.ActiveImport].String())
}

// TestMonthlyBillingCoversWhenTheEndSnapshotIsExactlyAtTolerance pins the
// boundary itself: an END reading exactly BillingSnapshotTolerance (72h)
// before February's own start still counts as covering ("<=", not "<").
func TestMonthlyBillingCoversWhenTheEndSnapshotIsExactlyAtTolerance(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	exactEnd := feb1.Add(-consumption.BillingSnapshotTolerance)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindBilling: {
			readingRow(analyzerID, jan1, model.ReadingKindBilling, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, exactEnd, model.ReadingKindBilling, map[string]string{"active_import": "6000"}),
		},
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1200"}),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, energy.KindBilling, rows[0].Source, "exactly at tolerance must still count as covering")
	require.Equal(t, "5000", rows[0].Values[energy.ActiveImport].String())
}

// TestMonthlyLoadProfileCoversExactlyAtToleranceButNotOneMicrosecondOver is
// R96's LoadProfileBoundaryTolerance (36h) exact-edge pin at Monthly level
// (final review B I-2: this parameter was unpinned by any test — mutating
// 36h to 13h, 60h or even 168h left the whole package green, because every
// existing user of it either had a fresher reading nearby or a
// billing/daily fallback that happened to give the same numeric answer
// either way). No billing or daily data exists anywhere near the END
// boundary here, so the END boundary resolves ONLY through load_profile's
// own tolerance check, with no fallback kind able to mask a wrong answer.
//
// The offsets below are the literal 36h, NOT consumption.LoadProfileBoundaryTolerance
// itself: using the constant as the offset would make this test
// self-referential (a mutation to the constant moves the fixture and the
// production check together, so nothing could ever fail). The first
// assertion below separately pins the constant's own value.
func TestMonthlyLoadProfileCoversExactlyAtToleranceButNotOneMicrosecondOver(t *testing.T) {
	require.Equal(t, 36*time.Hour, consumption.LoadProfileBoundaryTolerance, "R96's own ruled value")

	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	run := func(t *testing.T, offset time.Duration, wantRow bool) {
		readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
			model.ReadingKindLoadProfile: {
				readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
				readingRow(analyzerID, feb1.Add(-offset), model.ReadingKindLoadProfile, map[string]string{"active_import": "1200"}),
			},
		}}
		b := newBilling(t, readings)
		ctx := context.Background()
		scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

		rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
			AnalyzerIDs: []uuid.UUID{analyzerID},
			Level:       energy.Monthly,
			Range:       store.TimeRange{From: jan1, To: feb1},
		})
		require.NoError(t, err)
		if wantRow {
			require.Len(t, rows, 1, "exactly at LoadProfileBoundaryTolerance must still cover")
			require.Equal(t, energy.KindLoadProfile, rows[0].Source)
			require.Equal(t, "200", rows[0].Values[energy.ActiveImport].String())
		} else {
			require.Empty(t, rows, "one microsecond over LoadProfileBoundaryTolerance must not cover, with no billing/daily fallback present")
		}
	}

	t.Run("exactly at tolerance yields a row", func(t *testing.T) {
		run(t, 36*time.Hour, true)
	})
	t.Run("one microsecond over tolerance yields no row", func(t *testing.T) {
		run(t, 36*time.Hour+time.Microsecond, false)
	})
}

// --- R95: daily is a fallback boundary kind, never at Hourly ---------------

// TestBillingFallsBackToDailyWhenLoadProfileHasNoUsablePair covers R95's
// last fallback step: no load_profile data at all, but daily-kind readings
// cover the day within DailySnapshotTolerance.
func TestBillingFallsBackToDailyWhenLoadProfileHasNoUsablePair(t *testing.T) {
	loc := istanbulLoc(t)
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindDaily: {
			readingRow(analyzerID, day, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, day.Add(24*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "1300"}),
		},
	}}
	// R98: the fixture window closes at day+24h, after billingT0.
	b := newBillingAt(t, readings, day.Add(24*time.Hour).Add(consumption.SettleDelayDaily))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Daily,
		Range:       store.TimeRange{From: day, To: day.Add(24 * time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, energy.KindDaily, rows[0].Source)
	require.Equal(t, "300", rows[0].Values[energy.ActiveImport].String())
}

// TestBillingPrefersLoadProfileOverDailyWhenBothPresent is R95's precedence
// test for a mixed analyzer: load_profile must win even though daily is
// also present and would resolve to a different value.
func TestBillingPrefersLoadProfileOverDailyWhenBothPresent(t *testing.T) {
	loc := istanbulLoc(t)
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, day, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, day.Add(24*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1250"}),
		},
		model.ReadingKindDaily: {
			readingRow(analyzerID, day, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, day.Add(24*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "1300"}),
		},
	}}
	// R98: the fixture window closes at day+24h, after billingT0.
	b := newBillingAt(t, readings, day.Add(24*time.Hour).Add(consumption.SettleDelayDaily))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Daily,
		Range:       store.TimeRange{From: day, To: day.Add(24 * time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, energy.KindLoadProfile, rows[0].Source)
	require.Equal(t, "250", rows[0].Values[energy.ActiveImport].String(), "load_profile must win over daily when both are present")
}

// TestBillingDailyBoundaryToleranceCapsAtTwelveHoursOnANormalDay is R96's
// K1 fix, pinned on an ORDINARY (non-DST) 24h day: DailySnapshotTolerance
// is 36h, but boundaryTolerance caps every kind's tolerance at HALF the
// width of the bucket(s) meeting at the boundary — 12h for a normal 24h
// Daily bucket — so a daily-kind reading up to 12h before its own bound
// still counts as covering, but one any older does not.
//
// M-3 (final review B): the test this replaces used a fixed 37h-old
// reading — "one hour over the raw 36h tolerance" — but at Daily level the
// 12h half-bucket cap ALREADY binds well before 36h, so that fixture passed
// for ANY tolerance >= 12h (36h, 20h, 13h, ...) and never actually pinned
// Daily's own binding edge. This exact-edge pair does: 12h must still
// cover; 13h must not.
func TestBillingDailyBoundaryToleranceCapsAtTwelveHoursOnANormalDay(t *testing.T) {
	loc := istanbulLoc(t)
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	run := func(t *testing.T, offset time.Duration, wantRow bool) {
		readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
			model.ReadingKindDaily: {
				readingRow(analyzerID, day.Add(-offset), model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
				readingRow(analyzerID, day.Add(24*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "1300"}),
			},
		}}
		// R98: the fixture window closes at day+24h, after billingT0.
		b := newBillingAt(t, readings, day.Add(24*time.Hour).Add(consumption.SettleDelayDaily))
		ctx := context.Background()
		scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

		rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
			AnalyzerIDs: []uuid.UUID{analyzerID},
			Level:       energy.Daily,
			Range:       store.TimeRange{From: day, To: day.Add(24 * time.Hour)},
		})
		require.NoError(t, err)
		if wantRow {
			require.Len(t, rows, 1, "12h before the bound is exactly half of the 24h bucket — within the capped tolerance")
			require.Equal(t, "300", rows[0].Values[energy.ActiveImport].String())
		} else {
			require.Empty(t, rows, "13h before the bound exceeds half of the 24h bucket — outside the capped tolerance")
		}
	}

	t.Run("12h before the bound yields a row", func(t *testing.T) { run(t, 12*time.Hour, true) })
	t.Run("13h before the bound yields no row", func(t *testing.T) { run(t, 13*time.Hour, false) })
}

// TestHourlyNeverUsesDailyAsABoundary: daily-kind readings cover the hour
// perfectly, but R95 says daily is never a boundary kind at Hourly — with no
// load_profile data present, the bucket must be skipped, not billed from
// daily.
func TestHourlyNeverUsesDailyAsABoundary(t *testing.T) {
	loc := istanbulLoc(t)
	hourStart := time.Date(2026, 3, 10, 9, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		// Two DISTINCT readings, both inside the plain [hourStart, hourStart+1h)
		// window (the range Hourly fetches daily over, per R95). This is
		// deliberate: if start and end resolved to the SAME reading, Derive's
		// own same-instant rule would already produce "no row" for a reason
		// that has nothing to do with the Hourly guard, silently weakening
		// this test to prove nothing about R95's own precedence.
		model.ReadingKindDaily: {
			readingRow(analyzerID, hourStart, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, hourStart.Add(30*time.Minute), model.ReadingKindDaily, map[string]string{"active_import": "1005"}),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: hourStart, To: hourStart.Add(time.Hour)},
	})
	require.NoError(t, err)
	require.Empty(t, rows, "daily must never be used as a boundary kind at Hourly")
}

// TestMonthlyFallsBackToDailyWhenNeitherBillingNorLoadProfileCover completes
// R95's Monthly precedence chain: billing -> load_profile -> daily, with
// only daily-kind data present at all.
func TestMonthlyFallsBackToDailyWhenNeitherBillingNorLoadProfileCover(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindDaily: {
			readingRow(analyzerID, jan1, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, feb1, model.ReadingKindDaily, map[string]string{"active_import": "6000"}),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, energy.KindDaily, rows[0].Source)
	require.Equal(t, "5000", rows[0].Values[energy.ActiveImport].String())
}

// --- I5: ratios and max demand on the Billing path -------------------------

// TestBillingRatiosAndMaxDemandFromDerivedValues is I-5's Billing half.
// Deltas: active 1000->1100 = 100, reactive_inductive 100->125 = 25 (ratio
// 0.25), reactive_capacitive 40->52 = 12 (ratio 0.12). R101 (amending R65):
// at Hourly, MaxDemandKindsFor allows only load_profile — daily (20) and
// billing (15) rows, however INSIDE the window, must no longer win, proving
// a coarser kind's peak cannot land inside a single hour. Max demand is
// therefore the maximum of load_profile ALONE: 10, at the start boundary
// (the 500 load_profile peak sits exactly at w.To, excluded by the
// half-open window — it belongs to the NEXT bucket) — never the
// current_index decoy (999, wrong kind, always excluded regardless of
// level) and never daily's 20 or billing's 15.
//
// m-4 (final fix X review): the daily and billing decoys here no longer
// prove the kind exclusion by themselves — loadAnalyzerBoundaryData never
// fetches daily- or billing-kind readings for MaxDemand at Hourly at all
// now (they were dead reads once R101 excluded both kinds there), so a
// regression in maxDemandInWindow's own kind filter is no longer provable
// through this fixture. TestMaxDemandInWindowExcludesDisallowedKindsPerLevel
// (billing_internal_test.go) is the white-box test that still pins it
// directly.
func TestBillingRatiosAndMaxDemandFromDerivedValues(t *testing.T) {
	h := billingT0
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			mustSetMaxDemand(readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{
				"active_import": "1000", "reactive_inductive_import": "100", "reactive_capacitive_import": "40",
			}), "10"),
			mustSetMaxDemand(readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{
				"active_import": "1100", "reactive_inductive_import": "125", "reactive_capacitive_import": "52",
			}), "500"),
		},
		model.ReadingKindDaily: {
			mustSetMaxDemand(readingRow(analyzerID, h.Add(20*time.Minute), model.ReadingKindDaily, map[string]string{"active_import": "1020"}), "20"),
		},
		model.ReadingKindBilling: {
			mustSetMaxDemand(readingRow(analyzerID, h.Add(40*time.Minute), model.ReadingKindBilling, map[string]string{"active_import": "1040"}), "15"),
		},
		model.ReadingKindCurrentIndex: {
			mustSetMaxDemand(readingRow(analyzerID, h.Add(10*time.Minute), model.ReadingKindCurrentIndex, map[string]string{"active_import": "1010"}), "999"),
		},
	}}
	// R98: the fixture window closes at h+1h, at or after billingT0.
	b := newBillingAt(t, readings, h.Add(time.Hour).Add(consumption.SettleDelayHourly))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: h, To: h.Add(time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]

	require.NotNil(t, row.InductiveRatio)
	require.Equal(t, "0.25", row.InductiveRatio.String())
	require.NotNil(t, row.CapacitiveRatio)
	require.Equal(t, "0.12", row.CapacitiveRatio.String())

	require.NotNil(t, row.MaxDemandKw)
	require.Equal(t, "10", row.MaxDemandKw.String(),
		"R101: only load_profile counts at Hourly — daily's 20 and billing's 15 must not land in this hour, current_index is always excluded, and the 500 peak sits at w.To")
}

// mustSetMaxDemand sets r.MaxDemandKw to v, for readingRow fixtures that
// need both register values and a max-demand figure (readingRow's own
// "max_demand_kw" key works too, but this helper keeps the call sites above
// readable when several readings need distinct max-demand values).
func mustSetMaxDemand(r model.MeterReading, v string) model.MeterReading {
	d := decimal.RequireFromString(v)
	r.MaxDemandKw = &d
	return r
}

// --- I7: every repository call must use the caller's own Scope ------------

// TestBillingReadingCallsUseTheCallersScope is I-7's unit half: a
// scope-checking fakeReadings fails the test immediately if ANY call
// (load_profile, reset, billing, daily, or any of the three kinds'
// firstBoundaryLookback BoundaryReadings calls) uses a Scope other than the
// caller's own. sc deliberately has AllBuildings=false with an explicit
// BuildingIDs, so it is NOT equal to store.SystemScope(sc.CompanyID) —
// mutation (f) (substituting SystemScope(sc.CompanyID) on any one Range or
// BoundaryReadings call) is only provable if the two scopes actually differ.
//
// R96's re-review (I-7 remainder) found this test ran at Hourly ONLY, so the
// billing- and daily-kind Range calls and their look-backs — which exist
// only at Daily/Monthly — were never exercised: a mutation on
// billing.go:186's billing Range call stayed green in unit and integration.
// Table-driven across Hourly, Daily and Monthly (each level's own fixture
// supplies every kind that level's Consumption call touches) closes that
// gap: every call site in consumptionForAnalyzer now runs under the
// scope-checking fake at least once.
func TestBillingReadingCallsUseTheCallersScope(t *testing.T) {
	analyzerID := uuid.New()
	sc := store.Scope{CompanyID: uuid.New(), BuildingIDs: []uuid.UUID{uuid.New()}}
	loc := istanbulLoc(t)
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)

	cases := []struct {
		name  string
		level energy.Level
		from  time.Time
		to    time.Time
		kinds map[model.ReadingKind][]model.MeterReading
	}{
		{
			name: "hourly", level: energy.Hourly, from: billingT0, to: billingT0.Add(time.Hour),
			kinds: map[model.ReadingKind][]model.MeterReading{
				model.ReadingKindLoadProfile: {
					readingRow(analyzerID, billingT0, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
					readingRow(analyzerID, billingT0.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1110"}),
				},
				model.ReadingKindReset: {
					readingRow(analyzerID, billingT0.Add(time.Hour), model.ReadingKindReset, map[string]string{"active_import": "100"}),
				},
			},
		},
		{
			// Exercises the daily-kind Range and its own look-back (useDaily,
			// billing.go's non-Hourly branch), absent at Hourly.
			name: "daily", level: energy.Daily, from: day, to: day.Add(24 * time.Hour),
			kinds: map[model.ReadingKind][]model.MeterReading{
				model.ReadingKindLoadProfile: {
					readingRow(analyzerID, day, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
					readingRow(analyzerID, day.Add(24*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1100"}),
				},
				model.ReadingKindDaily: {
					readingRow(analyzerID, day, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
					readingRow(analyzerID, day.Add(24*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "1100"}),
				},
				model.ReadingKindReset: {
					readingRow(analyzerID, day.Add(12*time.Hour), model.ReadingKindReset, map[string]string{"active_import": "0"}),
				},
			},
		},
		{
			// Exercises the billing-kind Range and look-back (useBilling),
			// Monthly-only, the specific call site (billing.go:186) the
			// re-review's mutation targeted.
			name: "monthly", level: energy.Monthly, from: jan1, to: feb1,
			kinds: map[model.ReadingKind][]model.MeterReading{
				model.ReadingKindBilling: {
					readingRow(analyzerID, jan1, model.ReadingKindBilling, map[string]string{"active_import": "1000"}),
					readingRow(analyzerID, feb1, model.ReadingKindBilling, map[string]string{"active_import": "6000"}),
				},
				model.ReadingKindLoadProfile: {
					readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
					readingRow(analyzerID, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1200"}),
				},
				model.ReadingKindDaily: {
					readingRow(analyzerID, jan1, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
					readingRow(analyzerID, feb1, model.ReadingKindDaily, map[string]string{"active_import": "1300"}),
				},
				model.ReadingKindReset: {
					readingRow(analyzerID, jan1.Add(15*24*time.Hour), model.ReadingKindReset, map[string]string{"active_import": "0"}),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			readings := fakeReadings{t: t, wantScope: &sc, byKind: tc.kinds}
			b := newBilling(t, readings)
			ctx := context.Background()
			req := consumption.SeriesRequest{
				AnalyzerIDs: []uuid.UUID{analyzerID},
				Level:       tc.level,
				Range:       store.TimeRange{From: tc.from, To: tc.to},
			}
			_, err := b.Consumption(ctx, sc, req)
			require.NoError(t, err, "every repository call must use the caller's own Scope, never a substituted one")
		})
	}
}

// --- I6: validation runs before ANY I/O, on the Billing path --------------

// TestBillingValidatesBeforeAnyIO uses noReadings — which panics on every
// method — so a validation check that runs after even the FIRST repository
// call fails the test immediately, rather than quietly succeeding against a
// quiet fake that returns zero values.
func TestBillingValidatesBeforeAnyIO(t *testing.T) {
	ctx := context.Background()
	validScope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	validRange := store.TimeRange{From: billingT0, To: billingT0.Add(time.Hour)}

	cases := []struct {
		name  string
		scope store.Scope
		req   consumption.SeriesRequest
	}{
		{"invalid scope", store.Scope{}, consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{uuid.New()}, Level: energy.Hourly, Range: validRange}},
		{"empty analyzer ids", validScope, consumption.SeriesRequest{AnalyzerIDs: nil, Level: energy.Hourly, Range: validRange}},
		{"invalid range", validScope, consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{uuid.New()}, Level: energy.Hourly, Range: store.TimeRange{From: billingT0, To: billingT0}}},
		{"over MaxCells", validScope, consumption.SeriesRequest{AnalyzerIDs: newUUIDs(21), Level: energy.Hourly, Range: store.TimeRange{From: billingT0, To: billingT0.Add(2381 * time.Hour)}}},
		// m-5: these three (duplicate id, 51 ids, 401-day span) were
		// previously exercised only on the Analytics side
		// (analytics_test.go's TestAnalyticsValidatesBeforeAnyIO); Billing
		// shares the same validateRequest, but nothing pinned that fact
		// directly against the Billing path until now.
		{"duplicate analyzer id (I-5's probe)", validScope, consumption.SeriesRequest{AnalyzerIDs: duplicateAnalyzerID(), Level: energy.Hourly, Range: validRange}},
		{"51 analyzer ids", validScope, consumption.SeriesRequest{AnalyzerIDs: newUUIDs(consumption.MaxAnalyzersPerRequest + 1), Level: energy.Hourly, Range: validRange}},
		{"401-day span", validScope, consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{uuid.New()}, Level: energy.Yearly, Range: store.TimeRange{From: billingT0, To: billingT0.Add(401 * 24 * time.Hour)}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := consumption.NewBilling(consumption.BillingDeps{Readings: noReadings{}, Anomalies: noAnomalies{}, Ops: noOps{}, Clock: clock.NewFake(billingT0), Log: testLog(t)})
			require.NoError(t, err)
			_, err = b.Consumption(ctx, tc.scope, tc.req)
			require.ErrorIs(t, err, consumption.ErrInvalidRequest, "must be refused before any ReadingRepository call: noReadings panics on any call")
		})
	}
}

// TestBillingAcceptsExactlyMaxRequestSpan is R99's positive control for the
// span cap on the Billing path: exactly 400 days, well before billingT0 (so
// R98 never drops a bucket here), must not be refused.
func TestBillingAcceptsExactlyMaxRequestSpan(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	b := newBillingAt(t, fakeReadings{}, from.Add(consumption.MaxRequestSpan))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{uuid.New()},
		Level:       energy.Yearly,
		Range:       store.TimeRange{From: from, To: from.Add(consumption.MaxRequestSpan)},
	}
	rows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows, "the (empty) fake has no data — this only proves the request itself is accepted")
}

// --- R96/K1: a kind's tolerance is capped at half the bucket's width ------

// TestDailyMissingSnapshotYieldsNoRowForEitherAdjacentDayNotOneMergedRow is
// R96's K1 fix: a daily-only analyzer's Mar 10 00:00 snapshot is missing.
// Without R96's bucketWidth/2 cap, DailySnapshotTolerance (36h) exceeds a
// normal Daily bucket's own width (24h): the Mar 10 boundary would resolve
// to the STALE Mar 9 00:00 reading (24h away, inside an uncapped 36h),
// silently moving Mar 9's and Mar 10's true consumption into a single "Mar
// 10" row. R96 caps every kind's tolerance at half the bucket's width — 12h
// at Daily level for a normal 24h day — so a reading 24h away no longer
// qualifies: the Mar 10 boundary resolves to nil, and BOTH Mar 9 (whose own
// END boundary is now nil) and Mar 10 (whose own START boundary is nil) are
// silently absent, never merged into one wrong row. Mar 11, whose own
// boundaries both have exact daily readings, is unaffected: 1400 - 1300 =
// 100.
func TestDailyMissingSnapshotYieldsNoRowForEitherAdjacentDayNotOneMergedRow(t *testing.T) {
	loc := istanbulLoc(t)
	mar9 := time.Date(2026, 3, 9, 0, 0, 0, 0, loc)
	mar10 := mar9.Add(24 * time.Hour)
	mar11 := mar10.Add(24 * time.Hour)
	mar12 := mar11.Add(24 * time.Hour)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindDaily: {
			readingRow(analyzerID, mar9, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			// Mar 10's own snapshot is missing entirely.
			readingRow(analyzerID, mar11, model.ReadingKindDaily, map[string]string{"active_import": "1300"}),
			readingRow(analyzerID, mar12, model.ReadingKindDaily, map[string]string{"active_import": "1400"}),
		},
	}}
	// R98: the fixture window closes at mar12, after billingT0.
	b := newBillingAt(t, readings, mar12.Add(consumption.SettleDelayDaily))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Daily,
		Range:       store.TimeRange{From: mar9, To: mar12},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1, "Mar 9 and Mar 10 must both be silently absent — never merged into one row")
	require.True(t, rows[0].Window.From.Equal(mar11), "the only surviving row is Mar 11, whose own boundaries are both exact")
	require.Equal(t, "100", rows[0].Values[energy.ActiveImport].String())
}

// TestBillingDailyBucketToleranceCapsAtTheDSTNarrowedBucketWidth is Minor
// M-f (re-review round 2): boundaryTolerance's cap (billing.go:355-382)
// consults BOTH the bucket ending at bound and the bucket beginning at
// bound, taking the SMALLER width — but the committed suite only ever
// exercised normal 24h days, where both neighbours are the same width, so a
// mutant that looks at only the NEXT bucket's width (billing.go:373's `next`
// alone) stays green. 2015-03-29 in Europe/Istanbul is the spring-forward
// day: the bucket [Mar 29 00:00, Mar 30 00:00) is 23h wide, one hour short
// of a normal Daily bucket, so half of it is 11h30m rather than 12h. A
// daily-kind reading exactly 11h30m before Mar 30 00:00 (the boundary where
// the 23h bucket and the following normal 24h bucket meet) is within the
// capped tolerance and must yield the Mar 29 row; one 11h45m before it must
// not.
func TestBillingDailyBucketToleranceCapsAtTheDSTNarrowedBucketWidth(t *testing.T) {
	loc := istanbulLoc(t)
	mar29 := time.Date(2015, 3, 29, 0, 0, 0, 0, loc)
	mar30 := time.Date(2015, 3, 30, 0, 0, 0, 0, loc)
	require.Equal(t, 23*time.Hour, mar30.Sub(mar29), "2015-03-29 must be the 23h Istanbul spring-forward day")
	analyzerID := uuid.New()

	run := func(t *testing.T, offset time.Duration, wantRow bool) {
		readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
			model.ReadingKindDaily: {
				readingRow(analyzerID, mar29, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
				readingRow(analyzerID, mar30.Add(-offset), model.ReadingKindDaily, map[string]string{"active_import": "1100"}),
			},
		}}
		b := newBilling(t, readings)
		ctx := context.Background()
		scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

		rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
			AnalyzerIDs: []uuid.UUID{analyzerID},
			Level:       energy.Daily,
			Range:       store.TimeRange{From: mar29, To: mar30},
		})
		require.NoError(t, err)
		if wantRow {
			require.Len(t, rows, 1, "11h30m is exactly half of the 23h bucket — within the capped tolerance")
			require.Equal(t, "100", rows[0].Values[energy.ActiveImport].String())
		} else {
			require.Empty(t, rows, "11h45m exceeds half of the 23h bucket — outside the capped tolerance")
		}
	}

	t.Run("11h30m before the bound yields a row", func(t *testing.T) {
		run(t, 11*time.Hour+30*time.Minute, true)
	})
	t.Run("11h45m before the bound yields no row", func(t *testing.T) {
		run(t, 11*time.Hour+45*time.Minute, false)
	})
}

// --- R96/K2: adjacent periods telescope exactly, never gap nor overlap ----

// TestAdjacentMonthlyPeriodsTelescopeToTheTrueTotal is R96's K2 fix (the
// review's adjacent-months probe): December has no billing-kind reading of
// its own near its START, but January's billing snapshot was taken 48h
// early (Dec 30, well within BillingSnapshotTolerance) — the SAME resolved
// reading is used for BOTH December's own END boundary and January's own
// START boundary, because R96 resolves each boundary INSTANT exactly once
// and reuses it on both sides. December's start falls back to daily (Dec
// 1); January's end falls back to daily too (Feb 1 — no billing snapshot
// exists anywhere near it, only the stale Dec 30 one). The two monthly rows
// must sum to EXACTLY the true total between the first and last resolved
// boundary reading — 7200 - 1000 = 6200 — with no gap and no double-count
// at the Dec 30/Jan 1 seam.
func TestAdjacentMonthlyPeriodsTelescopeToTheTrueTotal(t *testing.T) {
	loc := istanbulLoc(t)
	dec1 := time.Date(2025, 12, 1, 0, 0, 0, 0, loc)
	dec30 := time.Date(2025, 12, 30, 0, 0, 0, 0, loc)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindBilling: {
			readingRow(analyzerID, dec30, model.ReadingKindBilling, map[string]string{"active_import": "6200"}),
		},
		model.ReadingKindDaily: {
			readingRow(analyzerID, dec1, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, jan1, model.ReadingKindDaily, map[string]string{"active_import": "6400"}),
			readingRow(analyzerID, feb1, model.ReadingKindDaily, map[string]string{"active_import": "7200"}),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: dec1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, rows, 2, "December and January")

	december, january := rows[0], rows[1]
	require.Equal(t, energy.KindBilling, december.Source, "December's END boundary resolves to the Dec 30 billing snapshot")
	require.Equal(t, energy.KindDaily, january.Source, "January's END boundary falls back to the Feb 1 daily snapshot")

	// The Jan 1 daily reading (6400) sits exactly at the Dec/Jan seam instant,
	// but must NOT be used for January's own START: the resolver picks the
	// billing snapshot (Dec 30) for that instant, exactly the same reading
	// December's own END used. If a regression let the two sides of the seam
	// resolve to different readings, December would still read 5200 but
	// January would read 800 (7200-6400) instead of 1000 (7200-6200).
	require.Equal(t, "5200", december.Values[energy.ActiveImport].String(), "December: 6200 (Dec 30 billing) - 1000 (Dec 1 daily)")
	require.Equal(t, "1000", january.Values[energy.ActiveImport].String(), "January: 7200 (Feb 1 daily) - 6200 (the SAME Dec 30 billing reading), never 800")

	sum := december.Values[energy.ActiveImport].Add(*january.Values[energy.ActiveImport])
	require.Equal(t, "6200", sum.String(), "the two rows must telescope to EXACTLY the true total, no gap and no overlap")
}

// --- R96/K3: a stale load_profile boundary must not beat a fresh daily pair

// TestBillingPrefersFreshDailyOverStaleLoadProfileAtMonthly is R96's K3 fix
// (Q7's "known risk", now closed): load_profile stops reporting after Jan 10
// (2500), while daily-kind readings cover the whole month fresh at both
// ends (1000 -> 6000). Before R96, load_profile's look-back at Daily/Monthly/
// Yearly was deliberately UNBOUNDED (02 §3.1), so the stale Jan 10 reading
// (22 days from February's own bound) would still win the END boundary over
// the fresh daily pair, giving 2500-1000=1500 instead of the true 5000. R96
// gives load_profile its own LoadProfileBoundaryTolerance (36h) at these
// levels: the Jan 10 reading is far outside it, so the END boundary falls
// through to daily, and the row comes out 6000-1000=5000, Source=daily.
func TestBillingPrefersFreshDailyOverStaleLoadProfileAtMonthly(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	jan10 := time.Date(2026, 1, 10, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, jan10, model.ReadingKindLoadProfile, map[string]string{"active_import": "2500"}),
		},
		model.ReadingKindDaily: {
			readingRow(analyzerID, jan1, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, feb1, model.ReadingKindDaily, map[string]string{"active_import": "6000"}),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, energy.KindDaily, rows[0].Source)
	require.Equal(t, "5000", rows[0].Values[energy.ActiveImport].String(), "never 1500 (the stale load_profile reading)")
}

// --- I-A: the C1 shared-minimum look-back must include the DAILY leg ------

// TestBillingDailyLookbackFeedsTheSharedMinimum is I-A (re-review round 1):
// C1's shared-minimum look-back must reach back through DAILY's own
// look-back too, never merely load_profile's. Daily level, no load_profile
// data at all: the daily-kind reading nearest the START (D-6h=1000) is what
// this request's own daily look-back finds, and a reset sits between it and
// D (D-2h=0) with NO load_profile prior anywhere. Unmutated: the shared
// minimum reaches back to D-6h, the reset IS found in the evidence window,
// and — since no load_profile prior exists at all to supply a before-reset
// value — the register comes out nil + meter_reset, the SAFE outcome (R91/
// I-1: a missing before-reset value is never billed as a number). Mutation
// (dropping dailyLookback's contribution to the shared minimum) makes the
// reset invisible instead: resets are then fetched only from first.From
// onward, missing D-2h entirely, and Derive's fast path returns a WRONG,
// UNFLAGGED plain difference (1300-1000=300) rather than the safe
// suspicion.
func TestBillingDailyLookbackFeedsTheSharedMinimum(t *testing.T) {
	loc := istanbulLoc(t)
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindDaily: {
			readingRow(analyzerID, day.Add(-6*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, day.Add(24*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "1300"}),
		},
		model.ReadingKindReset: {
			readingRow(analyzerID, day.Add(-2*time.Hour), model.ReadingKindReset, map[string]string{"active_import": "0"}),
		},
	}}
	// R98: the fixture window closes at day+24h, after billingT0.
	b := newBillingAt(t, readings, day.Add(24*time.Hour).Add(consumption.SettleDelayDaily))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Daily,
		Range:       store.TimeRange{From: day, To: day.Add(24 * time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1, "a suspect register is still a row (nil values + Suspicion), never omitted")
	require.Equal(t, energy.KindDaily, rows[0].Source)
	require.Nil(t, rows[0].Values[energy.ActiveImport], "no load_profile prior exists anywhere: never billed as a number")
	require.Contains(t, rows[0].Suspect, energy.ActiveImport)
	require.Equal(t, energy.ReasonMeterReset, rows[0].Suspect[energy.ActiveImport].Reason)
}

// --- I-B: R95/R96 daily fallback staleness on the END side ----------------

// TestMonthlyEndSideDailyStalenessYieldsNoRow is I-B: the START daily
// boundary is fresh (exact), but the only daily reading near the END is 37h
// before February's own bound — one hour over DailySnapshotTolerance (36h;
// the Monthly bucket is far wider than 72h, so R96's bucketWidth/2 cap never
// binds here). With no load_profile or billing data at all, the END
// boundary resolves to nil and the whole row is skipped, never billed from
// a stale end snapshot.
func TestMonthlyEndSideDailyStalenessYieldsNoRow(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindDaily: {
			readingRow(analyzerID, jan1, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, feb1.Add(-37*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "6000"}),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Empty(t, rows, "a daily boundary older than DailySnapshotTolerance on the END side must not count as covering")
}

// TestMonthlyDailyCoversWhenBothBoundsAreExactlyAtTolerance is I-B's
// positive control, pinning the boundary itself on BOTH sides: a daily
// reading exactly DailySnapshotTolerance (36h) before its own bound still
// counts as covering ("<=", not "<"), on the START side as well as the END.
//
// m-3 (final review B I-2 / final fix X review): the offset here is a
// LITERAL 36*time.Hour, never `-consumption.DailySnapshotTolerance` — the
// self-referential shape a mutation to the constant's own value would move
// in lockstep with, so the test could never actually pin what the constant
// IS, only that the code uses it consistently. TestDailySnapshotTolerance
// IsThirtySixHours below pins the literal value directly; a mutation to
// DailySnapshotTolerance (35h or 24h) makes THIS test's fixed 36h-old
// reading fall outside the (now narrower) tolerance on both sides, so no
// row is emitted at all and `require.Len(rows, 1)` below goes red.
func TestMonthlyDailyCoversWhenBothBoundsAreExactlyAtTolerance(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindDaily: {
			readingRow(analyzerID, jan1.Add(-36*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, feb1.Add(-36*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "6000"}),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, energy.KindDaily, rows[0].Source)
	require.Equal(t, "5000", rows[0].Values[energy.ActiveImport].String())
}

// TestDailySnapshotToleranceIsThirtySixHours is m-3's own constant pin: the
// literal 36h TestMonthlyDailyCoversWhenBothBoundsAreExactlyAtTolerance uses
// as its own offset must actually equal the constant it is meant to be
// pinning.
func TestDailySnapshotToleranceIsThirtySixHours(t *testing.T) {
	require.Equal(t, 36*time.Hour, consumption.DailySnapshotTolerance)
}

// --- I-C: R96 explicitly ALLOWS a period to start and end on different -----
// --- kinds; the pre-R96 "load_profile fails to emit as a pair" concept is --
// --- obsolete (kept here only as a positive demonstration). ---------------

// TestBillingMixesLoadProfileStartWithDailyEndUnderR96 replaces the old I-C
// concern. Pre-R96, load_profile was used for a period only when it
// resolved to two DISTINCT boundary readings for that whole period
// (boundaryPairEmits); a single load_profile reading (here, only one exists,
// at Jan 1) made load_profile resolve the SAME reading for both January's
// start AND its end, so the whole period fell back to daily instead. R96
// deleted that period-level concept entirely: each boundary instant is
// resolved on its own. January's START still finds that one load_profile
// reading (fresh, distance 0) and uses it; January's END has no usable
// load_profile candidate (the same one reading is 31 days stale) and falls
// through to daily on its own. The result is a MIXED row — load_profile
// start, daily end — Source = daily (the END's own kind), value 5010. This
// is deliberately no longer "no row" or "daily for the whole period": R96
// allows, and this test pins, exactly this mix.
func TestBillingMixesLoadProfileStartWithDailyEndUnderR96(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
		model.ReadingKindDaily: {
			readingRow(analyzerID, feb1, model.ReadingKindDaily, map[string]string{"active_import": "6010"}),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, energy.KindDaily, rows[0].Source, "the END boundary's own kind, even though the START resolved via load_profile")
	require.Equal(t, "5010", rows[0].Values[energy.ActiveImport].String())
}

// --- R101: a billing-kind row's peak must not land in an Hourly bucket ----

// TestBillingHourlyMaxDemandExcludesBillingKindEvenAsTheUniqueMaximum is
// R101's Hourly proof (amending I-D/R65, which used to require the
// opposite): the billing-kind reading is the ONLY one with a large
// MaxDemandKw (500) — load_profile (5 at h, 8 at h+1h, excluded by the
// half-open window) and daily (10) are both small decoys — yet at Hourly,
// energy.MaxDemandKindsFor allows load_profile only, so 500 must NOT win
// even though it is the unique maximum in the window: the answer is 5 (the
// one load_profile reading actually inside [h, h+1h)).
//
// m-4 (final fix X review): the daily and billing decoys no longer prove
// the kind exclusion by themselves — loadAnalyzerBoundaryData never
// fetches daily- or billing-kind readings for MaxDemand at Hourly at all
// now (dead reads once R101 excluded both kinds there), so the "use the
// unqualified MaxDemandKinds instead of MaxDemandKindsFor(level)" mutation
// this comment used to name no longer makes this 500: neither decoy is
// ever loaded to win in the first place.
// TestMaxDemandInWindowExcludesDisallowedKindsPerLevel
// (billing_internal_test.go) is the white-box test that still pins that
// mutation directly.
func TestBillingHourlyMaxDemandExcludesBillingKindEvenAsTheUniqueMaximum(t *testing.T) {
	h := billingT0
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			mustSetMaxDemand(readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}), "5"),
			mustSetMaxDemand(readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1100"}), "8"),
		},
		model.ReadingKindDaily: {
			mustSetMaxDemand(readingRow(analyzerID, h.Add(20*time.Minute), model.ReadingKindDaily, map[string]string{}), "10"),
		},
		model.ReadingKindBilling: {
			mustSetMaxDemand(readingRow(analyzerID, h.Add(40*time.Minute), model.ReadingKindBilling, map[string]string{}), "500"),
		},
	}}
	// R98: the fixture window closes at h+1h, at or after billingT0.
	b := newBillingAt(t, readings, h.Add(time.Hour).Add(consumption.SettleDelayHourly))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: h, To: h.Add(time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].MaxDemandKw)
	require.Equal(t, "5", rows[0].MaxDemandKw.String(), "R101: billing's 500 (and daily's 10) must not land in an Hourly bucket, even as the unique maximum")
}

// TestBillingMonthlyMaxDemandFromBillingKindIsTheUniqueMaximum is I-G
// (re-review round 2): the Hourly test above (now
// TestBillingHourlyMaxDemandExcludesBillingKindEvenAsTheUniqueMaximum,
// R101) proves the OPPOSITE at that level; this is R101's Monthly/Yearly
// positive control, where billing-kind rows still count. Originally this
// only exercised the HOURLY leg of I-D's fix (billing.go:216/248), so
// dropping billing-kind rows from the MONTHLY MaxDemand read
// (`maxDemandBilling := billing[:0]` at billing.go:229) stayed green — at
// Monthly, `billing` is already loaded for boundary resolution and reused
// as-is for MaxDemand, and that is exactly where R65/R85's ARIL demand
// charge lives. Here the billing-kind reading (Jan 16, far from either
// boundary, so it is never itself a boundary candidate under
// BillingSnapshotTolerance) carries the only large MaxDemandKw (500);
// load_profile at both January boundaries is a small decoy.
func TestBillingMonthlyMaxDemandFromBillingKindIsTheUniqueMaximum(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	jan16 := time.Date(2026, 1, 16, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			mustSetMaxDemand(readingRow(analyzerID, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}), "5"),
			mustSetMaxDemand(readingRow(analyzerID, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1100"}), "8"),
		},
		model.ReadingKindBilling: {
			mustSetMaxDemand(readingRow(analyzerID, jan16, model.ReadingKindBilling, map[string]string{}), "500"),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].MaxDemandKw)
	require.Equal(t, "500", rows[0].MaxDemandKw.String(), "the Monthly leg must also count billing-kind rows as a MaxDemand source (R65)")
}

// TestBillingDailyMaxDemandExcludesBillingKindRow is R101's Daily proof:
// MaxDemandKindsFor(Daily) is load_profile+daily, NOT billing — a whole
// month's own stamped peak (billing-kind, 500) must not land inside a
// single day even when it sits inside that day's own window, while a
// daily-kind reading's peak (20, that DAY's own stamped maximum) still
// counts.
//
// m-4 (final fix X review): the billing decoy no longer proves the kind
// exclusion by itself — loadAnalyzerBoundaryData never fetches billing-kind
// readings for MaxDemand at Daily at all now (a dead read once R101
// excluded that kind there), so the "use MaxDemandKinds instead of
// MaxDemandKindsFor(level)" mutation this comment used to name no longer
// makes this 500: the decoy is never loaded to win in the first place.
// TestMaxDemandInWindowExcludesDisallowedKindsPerLevel
// (billing_internal_test.go) is the white-box test that still pins that
// mutation directly.
func TestBillingDailyMaxDemandExcludesBillingKindRow(t *testing.T) {
	loc := istanbulLoc(t)
	day := time.Date(2026, 1, 10, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindDaily: {
			readingRow(analyzerID, day, model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			mustSetMaxDemand(readingRow(analyzerID, day.Add(6*time.Hour), model.ReadingKindDaily, map[string]string{}), "20"),
			readingRow(analyzerID, day.Add(24*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "1300"}),
		},
		model.ReadingKindBilling: {
			mustSetMaxDemand(readingRow(analyzerID, day.Add(12*time.Hour), model.ReadingKindBilling, map[string]string{}), "500"),
		},
	}}
	b := newBilling(t, readings)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Daily,
		Range:       store.TimeRange{From: day, To: day.Add(24 * time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].MaxDemandKw)
	require.Equal(t, "20", rows[0].MaxDemandKw.String(), "R101: billing's month-wide 500 must not land inside a single Daily bucket")
}

// --- I-E: the I-9 sort.Search lower bound must include w.From -------------

// TestMaxDemandIncludesAReadingExactlyAtTheBucketStartButExcludesWTo is I-E:
// two adjacent hourly buckets. A peak sits exactly at the first bucket's own
// w.From (50) and must be INCLUDED; a peak sits exactly at the boundary
// between the two buckets (70) and must count for the SECOND bucket (whose
// own w.From it is) but be EXCLUDED from the first (whose own w.To it is);
// a peak at the very last w.To (99) must be excluded from both, since there
// is no third bucket for it to belong to.
func TestMaxDemandIncludesAReadingExactlyAtTheBucketStartButExcludesWTo(t *testing.T) {
	h := billingT0
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			mustSetMaxDemand(readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}), "50"),
			mustSetMaxDemand(readingRow(analyzerID, h.Add(30*time.Minute), model.ReadingKindLoadProfile, map[string]string{}), "10"),
			mustSetMaxDemand(readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1010"}), "70"),
			mustSetMaxDemand(readingRow(analyzerID, h.Add(90*time.Minute), model.ReadingKindLoadProfile, map[string]string{}), "5"),
			mustSetMaxDemand(readingRow(analyzerID, h.Add(2*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1020"}), "99"),
		},
	}}
	// R98: the fixture window closes at h+2h, after billingT0.
	b := newBillingAt(t, readings, h.Add(2*time.Hour).Add(consumption.SettleDelayHourly))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: h, To: h.Add(2 * time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NotNil(t, rows[0].MaxDemandKw)
	require.Equal(t, "50", rows[0].MaxDemandKw.String(), "the reading exactly at the first bucket's w.From must be included")
	require.NotNil(t, rows[1].MaxDemandKw)
	require.Equal(t, "70", rows[1].MaxDemandKw.String(), "the reading exactly at the shared boundary belongs to the SECOND bucket, and 99 at the final w.To is excluded from both")
}

// --- 09 §F3 criterion 3: Yearly derives from its own boundaries too -------

// TestYearlyDoesNotEqualTheSumOfItsMonthsWhenOneMonthBucketIsMissing is 09
// §F3's criterion 3, Yearly's own leg (final review B I-1: no test in this
// package ever requested energy.Yearly at all, so a mutation replacing the
// yearly figure with the sum of 12 monthly Derive calls left the entire
// package green, unit and integration).
//
// One load_profile reading at every month's own 00:00 boundary,
// value(m) = 1000 + 100*m for m = 0..12 (January 1 of year 1 through
// January 1 of year 2), EXCEPT month 6's reading (July 1) is never
// written. Under R96 neither the June bucket (whose own END is July 1) nor
// the July bucket (whose own START is July 1) can resolve: both are
// silently absent. The other 10 monthly buckets survive and sum to 1200
// (the true Jan1->Jan1 total) - 100 (June's own missing delta) - 100
// (July's own missing delta) = 1000. The yearly row derives directly from
// ITS OWN two boundaries (Jan 1 year 1 = 1000, Jan 1 year 2 = 2200):
// 2200 - 1000 = 1200. 1200 != 1000 is the assertion that the yearly figure
// is never produced by summing its 12 months.
func TestYearlyDoesNotEqualTheSumOfItsMonthsWhenOneMonthBucketIsMissing(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	nextJan1 := jan1.AddDate(1, 0, 0)
	analyzerID := uuid.New()

	var lp []model.MeterReading
	for m := 0; m <= 12; m++ {
		if m == 6 {
			continue // July 1's own reading is deliberately missing.
		}
		ts := jan1.AddDate(0, m, 0)
		val := fmt.Sprintf("%d", 1000+100*m)
		lp = append(lp, readingRow(analyzerID, ts, model.ReadingKindLoadProfile, map[string]string{"active_import": val}))
	}
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: lp,
	}}
	// R98: the fixture window closes at nextJan1, after billingT0.
	b := newBillingAt(t, readings, nextJan1.Add(consumption.SettleDelayMonthly))
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	monthlyRows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: nextJan1},
	})
	require.NoError(t, err)
	require.Len(t, monthlyRows, 10, "12 months minus June and July, both touching the missing July-1 boundary")

	monthlySum := decimal.Zero
	for _, r := range monthlyRows {
		require.NotNil(t, r.Values[energy.ActiveImport])
		monthlySum = monthlySum.Add(*r.Values[energy.ActiveImport])
	}
	require.Equal(t, "1000", monthlySum.String())

	yearlyRows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Yearly,
		Range:       store.TimeRange{From: jan1, To: nextJan1},
	})
	require.NoError(t, err)
	require.Len(t, yearlyRows, 1)
	require.NotNil(t, yearlyRows[0].Values[energy.ActiveImport])
	require.Equal(t, "1200", yearlyRows[0].Values[energy.ActiveImport].String(), "the yearly row derives from its own two boundaries")
	require.NotEqual(t, "1200", monthlySum.String(), "summing the monthly rows is not how the yearly figure is produced")
}
