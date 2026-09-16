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
