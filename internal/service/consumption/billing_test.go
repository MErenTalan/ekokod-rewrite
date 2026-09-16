package consumption_test

import (
	"context"
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
// ConsumptionAndRecord, on the same type).
func newBilling(t *testing.T, readings fakeReadings) *consumption.Billing {
	t.Helper()
	b, err := consumption.NewBilling(consumption.BillingDeps{
		Readings:  readings,
		Anomalies: noAnomalies{},
		Ops:       noOps{},
		Clock:     clock.NewFake(billingT0),
		Log:       testLog(t),
	})
	require.NoError(t, err)
	return b
}

// TestBillingPathCannotReachAnAggregate is R61's symmetric guard: Billing
// has no AnalyticsRepository field at all. fakeReadings{} has no data, so
// every boundary lookup comes back nil and no row is emitted — proving only
// fakeReadings was ever consulted (noAnomalies/noOps would panic if
// Consumption touched either, and there is no aggregate dependency present
// to have reached in the first place).
func TestBillingPathCannotReachAnAggregate(t *testing.T) {
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

// TestBillingRefusesARequestOverTheMaxBucketsCap proves the MaxBuckets cap
// (10000) is enforced before any I/O: fakeReadings{} would panic-free but
// return nothing useful anyway, so a non-empty result here could only come
// from the cap failing to apply.
func TestBillingRefusesARequestOverTheMaxBucketsCap(t *testing.T) {
	b := newBilling(t, fakeReadings{})

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{uuid.New()},
		Level:       energy.Hourly,
		// MaxBuckets+1 hourly buckets: one over the cap.
		Range: store.TimeRange{From: billingT0, To: billingT0.Add(time.Duration(consumption.MaxBuckets+1) * time.Hour)},
	}

	_, err := b.Consumption(ctx, scope, req)
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)
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
// R63: only ONE billing-kind reading exists (at January's start), so
// SelectBoundary resolves BOTH the start and end boundary to that same
// reading — the end boundary (dated a full month before February's own
// bound) is far outside BillingSnapshotTolerance, so the month is not
// covered and falls back to load_profile: 1200 - 1000 = 200.
func TestMonthlyFallsBackToLoadProfileWhenBillingReadingsDoNotCoverTheMonth(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindBilling: {
			readingRow(analyzerID, jan1, model.ReadingKindBilling, map[string]string{"active_import": "1000"}),
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
	require.Equal(t, energy.KindLoadProfile, row.Source)
	require.NotNil(t, row.Values[energy.ActiveImport])
	require.Equal(t, "200", row.Values[energy.ActiveImport].String())
}

// TestMonthlyFallsBackWhenTheOnlyBillingSnapshotIsOlderThanTheTolerance is
// I-3: a billing-kind reading exists near the START boundary (3 days before
// January's own start — exactly at BillingSnapshotTolerance, still
// covering) but the reading nearest the END boundary is 4 days (96h, over
// the 72h tolerance) before February's start. One stale side is enough to
// fail coverage for the whole month (R63: "each of the month's two
// boundary readings"), so this falls back to load_profile exactly as if no
// billing reading existed at all: 1200 - 1000 = 200.
func TestMonthlyFallsBackWhenTheOnlyBillingSnapshotIsOlderThanTheTolerance(t *testing.T) {
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
	require.Equal(t, energy.KindLoadProfile, row.Source)
	require.NotNil(t, row.Values[energy.ActiveImport])
	require.Equal(t, "200", row.Values[energy.ActiveImport].String())
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
	b := newBilling(t, readings)

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
	b := newBilling(t, fake)
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
// 12:00=0, lp 24:00=200. True = (1100-1000) + (200-0) = 300. Mutation (a)
// (priors := a current_index Range) makes the decoy the "before_reset"
// value instead of the real 1100, producing 700 unflagged.
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
	b := newBilling(t, readings)
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

// TestMonthlyFallsBackWhenTheStartSnapshotIsStale is C3 (review probe P3b).
// The START billing snapshot is 73h before January (just over
// BillingSnapshotTolerance); the END snapshot is fresh. Mutation (d')
// (disabling only the START-side tolerance check) leaves this GREEN with
// the wrong number (6000, ten December days billed into January); the
// correct fallback answer is load_profile's 5000.
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
	require.Equal(t, energy.KindLoadProfile, rows[0].Source)
	require.Equal(t, "5000", rows[0].Values[energy.ActiveImport].String(), "a stale START snapshot must not count as covering")
}

// TestMonthlyFallsBackWhenTheStartSnapshotIsMissing is C3's second fixture:
// no billing reading anywhere near the START boundary at all (only at
// February's own start), so the month is not covered and falls back to
// load_profile exactly as a stale one would.
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
	require.Equal(t, energy.KindLoadProfile, rows[0].Source)
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
	require.Equal(t, energy.KindLoadProfile, rows[0].Source)
	require.Equal(t, "250", rows[0].Values[energy.ActiveImport].String(), "load_profile must win over daily when both are present")
}

// TestBillingStaleDailySnapshotYieldsNoRow: the only daily boundary reading
// available for the start of the window is 37h before it — one hour over
// DailySnapshotTolerance (36h) — so daily does not cover it either, and (with
// no load_profile at all) the bucket is skipped rather than billed from a
// stale snapshot.
func TestBillingStaleDailySnapshotYieldsNoRow(t *testing.T) {
	loc := istanbulLoc(t)
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	analyzerID := uuid.New()

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindDaily: {
			readingRow(analyzerID, day.Add(-37*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, day.Add(24*time.Hour), model.ReadingKindDaily, map[string]string{"active_import": "1300"}),
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
	require.Empty(t, rows, "a daily boundary older than DailySnapshotTolerance must not count as covering")
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
// 0.25), reactive_capacitive 40->52 = 12 (ratio 0.12). Max demand must be
// the maximum of load_profile (10 at the start boundary), daily (20, inside
// the window) and billing (15, inside the window) — never the
// current_index decoy (999, wrong kind, always excluded) and never the 500
// peak sitting exactly at w.To (excluded by the half-open window: it
// belongs to the NEXT bucket, not this one).
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
	b := newBilling(t, readings)
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
	require.Equal(t, "20", row.MaxDemandKw.String(),
		"the maximum of load_profile/daily/billing INSIDE the window, never current_index and never the peak at w.To")
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
// (load_profile, reset, billing, daily) uses a Scope other than the
// caller's own. sc deliberately has AllBuildings=false with an explicit
// BuildingIDs, so it is NOT equal to store.SystemScope(sc.CompanyID) —
// mutation (f) (substituting SystemScope(sc.CompanyID) on the reset Range
// call) is only provable if the two scopes actually differ.
func TestBillingReadingCallsUseTheCallersScope(t *testing.T) {
	analyzerID := uuid.New()
	sc := store.Scope{CompanyID: uuid.New(), BuildingIDs: []uuid.UUID{uuid.New()}}
	readings := fakeReadings{
		t: t, wantScope: &sc,
		byKind: map[model.ReadingKind][]model.MeterReading{
			model.ReadingKindLoadProfile: {
				readingRow(analyzerID, billingT0, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
				readingRow(analyzerID, billingT0.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1110"}),
			},
			model.ReadingKindReset: {
				readingRow(analyzerID, billingT0.Add(time.Hour), model.ReadingKindReset, map[string]string{"active_import": "100"}),
			},
		},
	}
	b := newBilling(t, readings)
	ctx := context.Background()
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: billingT0, To: billingT0.Add(time.Hour)},
	}
	_, err := b.Consumption(ctx, sc, req)
	require.NoError(t, err, "every repository call must use the caller's own Scope, never a substituted one")
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
		{"over MaxBuckets", validScope, consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{uuid.New()}, Level: energy.Hourly, Range: store.TimeRange{From: billingT0, To: billingT0.Add(time.Duration(consumption.MaxBuckets+1) * time.Hour)}}},
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

// TestBillingAcceptsExactlyMaxBuckets is I-6's positive control: a request
// AT the cap (not over it) must be accepted and reach the (empty) fake.
func TestBillingAcceptsExactlyMaxBuckets(t *testing.T) {
	b := newBilling(t, fakeReadings{})
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{uuid.New()},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: billingT0, To: billingT0.Add(time.Duration(consumption.MaxBuckets) * time.Hour)},
	}
	rows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows)
}
