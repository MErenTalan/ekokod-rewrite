package consumption_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// istanbulLoc loads Europe/Istanbul, failing the test immediately if it
// cannot be resolved — it always can, since internal/domain/energy and this
// package both blank-import time/tzdata.
func istanbulLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	return loc
}

// TestAnalyticsPathCannotReachTheHypertable is R61's structural guard, named
// in the plan's own guard list: AnalyticsDeps has no ReadingRepository field
// at all, so there is nothing to poison and no fallback path to
// accidentally take. fakeAnalytics{rows: nil} deliberately returns NO rows
// (I-8) — the one condition under which a "fall back to Readings when the
// aggregate is empty" mutation would actually have somewhere to fall back
// TO, which is what makes this fixture prove the mutation's absence rather
// than merely fail to exercise it.
//
// I-4 (final review B): this behavioural probe alone is vacuous against a
// FIELD-level bypass — adding a poisoned field nobody's code path ever
// calls (or a narrower consumer-defined interface a real repository could
// still satisfy, guard_test.go's own I-4 fix) leaves it green. It now
// delegates to guard_test.go's reflection walk (the same one
// TestAnalyticsDepsHasNoReadingOrAnomalyRepositoryField calls) FIRST, so the
// plan's own named guard also catches what the structural guard catches,
// not only what this call happens to exercise at runtime.
func TestAnalyticsPathCannotReachTheHypertable(t *testing.T) {
	readingRepo := reflect.TypeOf((*store.ReadingRepository)(nil)).Elem()
	assertNoForbiddenField(t, reflect.TypeOf(consumption.AnalyticsDeps{}), readingRepo)

	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: fakeAnalytics{rows: nil}, Log: testLog(t)})
	require.NoError(t, err)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{uuid.New()},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 3, 1, 1, 0, 0, 0, time.UTC)},
	}

	rows, err := a.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows, "exactly the fake's zero rows; nothing else could have contributed")
}

// TestOpenMonthIsComposedFromDailyClosingIndexes is R88/R94's acceptance
// test.
//
// Hand arithmetic: consumption_monthly has a closed January row (the
// analyzer's monthly consumption for January itself is not what this test
// checks). February is still open: the request's range ends mid-February,
// so consumption_monthly has no row for it at all — exactly
// materialized_only's documented behaviour for an in-progress bucket.
//
// Daily closing indexes: the last consumption_daily bucket BEFORE February
// began is 31 Jan, active_index = 5000 (the closing register value at the
// end of January). The last consumption_daily bucket INSIDE February (up to
// the request's own truncation at 15 Feb) is 10 Feb, active_index = 5300.
//
//	composed February active_import = 5300 - 5000 = 300
//
// I-2: MaxDemandKw is the MAXIMUM over EVERY daily bucket inside February,
// not merely the last one — 5 Feb's peak (96) is both earlier than and
// larger than 10 Feb's own (42), so a bug that took only the last bucket's
// figure (the review's I-2 finding) would report 42, not the true 96.
func TestOpenMonthIsComposedFromDailyClosingIndexes(t *testing.T) {
	loc := istanbulLoc(t)
	analyzerID := uuid.New()

	janStart := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	jan31 := time.Date(2026, 1, 31, 0, 0, 0, 0, loc)
	feb5 := time.Date(2026, 2, 5, 0, 0, 0, 0, loc)
	feb10 := time.Date(2026, 2, 10, 0, 0, 0, 0, loc)
	midFeb := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)

	analytics := fakeAnalytics{
		monthlyRows: []model.ConsumptionBucket{
			{AnalyzerID: analyzerID, Bucket: janStart, ActiveConsumption: dec("3000.0000"), ActiveIndex: dec("5000.0000")},
		},
		dailyRows: []model.ConsumptionBucket{
			{AnalyzerID: analyzerID, Bucket: jan31, ActiveIndex: dec("5000.0000")},
			{AnalyzerID: analyzerID, Bucket: feb5, ActiveIndex: dec("5150.0000"), MaxDemandKw: dec("96.0000")},
			{AnalyzerID: analyzerID, Bucket: feb10, ActiveIndex: dec("5300.0000"), MaxDemandKw: dec("42.0000")},
		},
	}
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analytics, Log: testLog(t)})
	require.NoError(t, err)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: janStart, To: midFeb},
	}

	rows, err := a.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 2, "January (closed, materialized) and February (open, composed)")

	open := rows[len(rows)-1]
	require.True(t, open.Partial, "the composed row must be marked Partial")
	require.NotNil(t, open.Values[energy.ActiveImport])
	require.Equal(t, "300", open.Values[energy.ActiveImport].String())
	require.Equal(t, energy.KindLoadProfile, open.Source)
	require.NotNil(t, open.MaxDemandKw)
	require.Equal(t, "96", open.MaxDemandKw.String(), "the peak day, not the last day")

	closed := rows[0]
	require.False(t, closed.Partial, "January is a closed, materialized row")
}

// TestAnalyticsComposesEveryAbsentBucketNotOnlyTheLast is R94's acceptance
// test (amending R88): consumption_monthly holds January only. Request
// [Dec, Apr) has daily buckets in every month. December, February and March
// must ALL be composed (Partial=true) — including February, which is
// closed and NOT the last window in the request — while January stays
// materialized (Partial=false), and a month with no daily buckets at all
// (there are none here past March) is simply absent.
func TestAnalyticsComposesEveryAbsentBucketNotOnlyTheLast(t *testing.T) {
	loc := istanbulLoc(t)
	analyzerID := uuid.New()

	nov30 := time.Date(2025, 11, 30, 0, 0, 0, 0, loc)
	dec1 := time.Date(2025, 12, 1, 0, 0, 0, 0, loc)
	dec31 := time.Date(2025, 12, 31, 0, 0, 0, 0, loc)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	jan31 := time.Date(2026, 1, 31, 0, 0, 0, 0, loc)
	feb15 := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)
	mar15 := time.Date(2026, 3, 15, 0, 0, 0, 0, loc)
	apr1 := time.Date(2026, 4, 1, 0, 0, 0, 0, loc)

	analytics := fakeAnalytics{
		monthlyRows: []model.ConsumptionBucket{
			{AnalyzerID: analyzerID, Bucket: jan1, ActiveConsumption: dec("999"), ActiveIndex: dec("2000")},
		},
		dailyRows: []model.ConsumptionBucket{
			{AnalyzerID: analyzerID, Bucket: nov30, ActiveIndex: dec("400")},
			{AnalyzerID: analyzerID, Bucket: dec1, ActiveIndex: dec("500")},
			{AnalyzerID: analyzerID, Bucket: dec31, ActiveIndex: dec("1000")},
			{AnalyzerID: analyzerID, Bucket: jan31, ActiveIndex: dec("2000")},
			{AnalyzerID: analyzerID, Bucket: feb15, ActiveIndex: dec("2400")},
			{AnalyzerID: analyzerID, Bucket: mar15, ActiveIndex: dec("3000")},
		},
	}
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analytics, Log: testLog(t)})
	require.NoError(t, err)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	rows, err := a.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: dec1, To: apr1},
	})
	require.NoError(t, err)

	byMonth := make(map[string]consumption.Row, len(rows))
	for _, r := range rows {
		byMonth[r.Window.From.In(loc).Format("2006-01")] = r
	}
	require.Len(t, byMonth, 4, "Dec, Jan, Feb, Mar — no row at all past March, where there is no daily data")

	dec := byMonth["2025-12"]
	require.True(t, dec.Partial, "December is composed: no materialized row, but a daily bucket both before (Nov 30) and inside it")
	require.Equal(t, "600", dec.Values[energy.ActiveImport].String(), "1000 - 400")

	jan := byMonth["2026-01"]
	require.False(t, jan.Partial, "January is the materialized row")

	feb := byMonth["2026-02"]
	require.True(t, feb.Partial, "February is closed and NOT the last window in the request, but must still be composed")
	require.Equal(t, "400", feb.Values[energy.ActiveImport].String(), "2400 - 2000")

	mar := byMonth["2026-03"]
	require.True(t, mar.Partial)
	require.Equal(t, "600", mar.Values[energy.ActiveImport].String(), "3000 - 2400")

	_, hasApr := byMonth["2026-04"]
	require.False(t, hasApr, "no daily bucket at all inside or before April: absent, not a zero row")
}

// TestAnalyticsQueriesTheWidenedBucketRange is I-1: a monthly request
// starting mid-month (Feb 15) must still query the aggregate from the WHOLE
// bucket's own start (Feb 1), never from req.Range.From directly — the
// materialised view's predicate is `bucket >= from_ts`, so querying with
// Feb 15 itself would silently drop the February row.
func TestAnalyticsQueriesTheWidenedBucketRange(t *testing.T) {
	loc := istanbulLoc(t)
	analyzerID := uuid.New()
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	feb15 := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)
	mar1 := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)

	var calls []analyticsCall
	analytics := fakeAnalytics{calls: &calls}
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analytics, Log: testLog(t)})
	require.NoError(t, err)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	_, err = a.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: feb15, To: mar1},
	})
	require.NoError(t, err)

	require.NotEmpty(t, calls)
	require.Equal(t, "ConsumptionMonthly", calls[0].method)
	require.True(t, calls[0].r.From.Equal(feb1), "the query must widen From to the whole bucket's own start, got %s", calls[0].r.From)
	require.True(t, calls[0].r.To.Equal(mar1))
}

// --- I5: ratios must come from the bucket's consumption, not its index ----

// TestAnalyticsRatiosComeFromConsumptionValuesNotClosingIndexes is I-5's
// Analytics half: the bucket's consumption values (100/30/6) and its
// closing indexes (99999/...) are deliberately set to wildly different
// numbers, so a mutation that computed the ratio from Indexes instead of
// Values would produce an obviously different, wrong ratio.
func TestAnalyticsRatiosComeFromConsumptionValuesNotClosingIndexes(t *testing.T) {
	analyzerID := uuid.New()
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)

	analytics := fakeAnalytics{
		hourlyRows: []model.ConsumptionBucket{{
			AnalyzerID:            analyzerID,
			Bucket:                from,
			ActiveConsumption:     dec("100"),
			InductiveConsumption:  dec("30"),
			CapacitiveConsumption: dec("6"),
			ActiveIndex:           dec("99999"),
			InductiveIndex:        dec("88888"),
			CapacitiveIndex:       dec("77777"),
		}},
	}
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analytics, Log: testLog(t)})
	require.NoError(t, err)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	rows, err := a.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: from, To: to},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	require.NotNil(t, rows[0].InductiveRatio)
	require.Equal(t, "0.3", rows[0].InductiveRatio.String())
	require.NotNil(t, rows[0].CapacitiveRatio)
	require.Equal(t, "0.06", rows[0].CapacitiveRatio.String())

	// The closing indexes are still reported, untouched, alongside the
	// ratio computed from the (very different) consumption values.
	require.Equal(t, "99999", rows[0].Indexes[energy.ActiveImport].String())
}

// --- I6: validation runs before ANY I/O, on the Analytics path -----------

// TestAnalyticsValidatesBeforeAnyIO uses noAnalytics — which panics on
// every method — so a validation check that runs after even the FIRST
// repository call fails the test immediately.
func TestAnalyticsValidatesBeforeAnyIO(t *testing.T) {
	ctx := context.Background()
	validScope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	validRange := store.TimeRange{From: from, To: from.Add(time.Hour)}

	cases := []struct {
		name  string
		scope store.Scope
		req   consumption.SeriesRequest
	}{
		{"invalid scope", store.Scope{}, consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{uuid.New()}, Level: energy.Hourly, Range: validRange}},
		{"empty analyzer ids", validScope, consumption.SeriesRequest{AnalyzerIDs: nil, Level: energy.Hourly, Range: validRange}},
		{"invalid range", validScope, consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{uuid.New()}, Level: energy.Hourly, Range: store.TimeRange{From: from, To: from}}},
		{"over MaxCells", validScope, consumption.SeriesRequest{AnalyzerIDs: newUUIDs(21), Level: energy.Hourly, Range: store.TimeRange{From: from, To: from.Add(2381 * time.Hour)}}},
		{"duplicate analyzer id (I-5's probe)", validScope, consumption.SeriesRequest{AnalyzerIDs: duplicateAnalyzerID(), Level: energy.Hourly, Range: validRange}},
		{"51 analyzer ids", validScope, consumption.SeriesRequest{AnalyzerIDs: newUUIDs(consumption.MaxAnalyzersPerRequest + 1), Level: energy.Hourly, Range: validRange}},
		{"401-day span", validScope, consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{uuid.New()}, Level: energy.Yearly, Range: store.TimeRange{From: from, To: from.Add(401 * 24 * time.Hour)}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: noAnalytics{}, Log: testLog(t)})
			require.NoError(t, err)
			_, err = a.Consumption(ctx, tc.scope, tc.req)
			require.ErrorIs(t, err, consumption.ErrInvalidRequest, "must be refused before any AnalyticsRepository call: noAnalytics panics on any call")
		})
	}
}

// --- R104(2): MaxCells replaces MaxBuckets -----------------------------

// TestAnalyticsAcceptsExactlyMaxCells is R104(2)'s positive control on the
// Analytics path (the Billing-side twin is
// billing_test.go's TestBillingAcceptsExactlyMaxCells): 50 analyzers
// (MaxAnalyzersPerRequest) times 1000 Hourly buckets = exactly MaxCells
// (50000) cells must not be refused.
func TestAnalyticsAcceptsExactlyMaxCells(t *testing.T) {
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: fakeAnalytics{}, Log: testLog(t)})
	require.NoError(t, err)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	from, to := cellsWindow(1000)
	req := consumption.SeriesRequest{
		AnalyzerIDs: newUUIDs(consumption.MaxAnalyzersPerRequest),
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: from, To: to},
	}
	require.Equal(t, consumption.MaxAnalyzersPerRequest*1000, consumption.MaxCells, "the fixture must land exactly on MaxCells")
	rows, err := a.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows)
}

// TestAnalyticsRefusesOverMaxCells is R104(2)'s own probe on the Analytics
// path, replacing the deleted MaxBuckets test: 21 analyzers times 2381
// Hourly buckets is exactly MaxCells+1 (50001) cells — refused before any
// I/O against noAnalytics, which panics on any repository call.
func TestAnalyticsRefusesOverMaxCells(t *testing.T) {
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: noAnalytics{}, Log: testLog(t)})
	require.NoError(t, err)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	from, to := cellsWindow(2381)
	req := consumption.SeriesRequest{
		AnalyzerIDs: newUUIDs(21),
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: from, To: to},
	}
	require.Equal(t, consumption.MaxCells+1, 21*2381, "the fixture must land exactly one over MaxCells")
	_, err = a.Consumption(ctx, scope, req)
	require.ErrorIs(t, err, consumption.ErrInvalidRequest, "must be refused before any AnalyticsRepository call: noAnalytics panics on any call")
}

// duplicateAnalyzerID reproduces final review A's I-5 probe verbatim: the
// same id listed twice.
func duplicateAnalyzerID() []uuid.UUID {
	id := uuid.New()
	return []uuid.UUID{id, id}
}

// newUUIDs returns n distinct uuid.UUIDs, for R99's MaxAnalyzersPerRequest
// boundary tests.
func newUUIDs(n int) []uuid.UUID {
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
	}
	return ids
}

// TestAnalyticsAcceptsExactlyMaxAnalyzersPerRequest is R99's positive
// control for the analyzer-count cap: exactly MaxAnalyzersPerRequest,
// all distinct, must not be refused.
func TestAnalyticsAcceptsExactlyMaxAnalyzersPerRequest(t *testing.T) {
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: fakeAnalytics{}, Log: testLog(t)})
	require.NoError(t, err)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	req := consumption.SeriesRequest{
		AnalyzerIDs: newUUIDs(consumption.MaxAnalyzersPerRequest),
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: from, To: from.Add(time.Hour)},
	}
	_, err = a.Consumption(ctx, scope, req)
	require.NoError(t, err)
}

// TestAnalyticsAcceptsExactlyMaxRequestSpan is R99's positive control for the
// span cap: exactly 400 days must not be refused.
func TestAnalyticsAcceptsExactlyMaxRequestSpan(t *testing.T) {
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: fakeAnalytics{}, Log: testLog(t)})
	require.NoError(t, err)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{uuid.New()},
		Level:       energy.Yearly,
		Range:       store.TimeRange{From: from, To: from.Add(consumption.MaxRequestSpan)},
	}
	_, err = a.Consumption(ctx, scope, req)
	require.NoError(t, err)
}

// TestAnalyticsRefusesAnUnrecognisedLevel is M-1: an unknown Level must be
// refused by validateRequest, never silently reach fetchBuckets' otherwise
// unreachable default branch and return (nil, nil).
func TestAnalyticsRefusesAnUnrecognisedLevel(t *testing.T) {
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: noAnalytics{}, Log: testLog(t)})
	require.NoError(t, err)
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	_, err = a.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{uuid.New()},
		Level:       energy.Level("weekly"),
		Range:       store.TimeRange{From: from, To: from.Add(time.Hour)},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)
}

// --- I7: Analytics repository calls must use the caller's own Scope -------

// TestAnalyticsCallsUseTheCallersScope is I-7's Analytics-side unit test,
// symmetric to TestBillingReadingCallsUseTheCallersScope.
func TestAnalyticsCallsUseTheCallersScope(t *testing.T) {
	analyzerID := uuid.New()
	sc := store.Scope{CompanyID: uuid.New(), BuildingIDs: []uuid.UUID{uuid.New()}}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	analytics := fakeAnalytics{t: t, wantScope: &sc, hourlyRows: []model.ConsumptionBucket{
		{AnalyzerID: analyzerID, Bucket: from, ActiveConsumption: dec("1")},
	}}
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analytics, Log: testLog(t)})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = a.Consumption(ctx, sc, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: from, To: from.Add(time.Hour)},
	})
	require.NoError(t, err, "every AnalyticsRepository call must use the caller's own Scope")
}

// TestComposedRowRatioComesFromConsumptionValuesNotClosingIndexes is Minor
// M-b: composeOpenPeriodRow computes InductiveRatio/CapacitiveRatio from the
// composed VALUES (the inside-minus-before deltas), never from the raw
// closing indexes — a mutation that used insideIdx directly (analytics.go:
// 268-269) stayed green because no test asserted a composed row's ratio at
// all. The indexes and the values are deliberately different numbers here.
func TestComposedRowRatioComesFromConsumptionValuesNotClosingIndexes(t *testing.T) {
	loc := istanbulLoc(t)
	analyzerID := uuid.New()
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, loc)
	dec31 := time.Date(2025, 12, 31, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)

	analytics := fakeAnalytics{
		dailyRows: []model.ConsumptionBucket{
			{AnalyzerID: analyzerID, Bucket: dec31, ActiveIndex: dec("1000"), InductiveIndex: dec("50"), CapacitiveIndex: dec("20")},
			{AnalyzerID: analyzerID, Bucket: jan15, ActiveIndex: dec("1100"), InductiveIndex: dec("80"), CapacitiveIndex: dec("32")},
		},
	}
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analytics, Log: testLog(t)})
	require.NoError(t, err)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	rows, err := a.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].Partial)

	// Values: active 1100-1000=100, inductive 80-50=30 (ratio 0.3), capacitive
	// 32-20=12 (ratio 0.12) — NOT the ratio of the raw closing indexes
	// (80/1100 = 0.0727..., 32/1100 = 0.029...).
	require.NotNil(t, rows[0].InductiveRatio)
	require.Equal(t, "0.3", rows[0].InductiveRatio.String())
	require.NotNil(t, rows[0].CapacitiveRatio)
	require.Equal(t, "0.12", rows[0].CapacitiveRatio.String())
}

// TestAnalyticsCallsUseTheCallersScopeAtMonthlyComposition is I-7's
// remainder: the re-review found TestAnalyticsCallsUseTheCallersScope ran at
// Hourly only, so composeMissingPeriods' own ConsumptionDaily call
// (analytics.go:185) was never exercised by any scope-checking test — a
// mutation substituting store.SystemScope(sc.CompanyID) there stayed green.
// monthlyRows is empty (no materialised row at all), forcing composition to
// run; dailyRows supplies enough for composeOpenPeriodRow to emit a row.
func TestAnalyticsCallsUseTheCallersScopeAtMonthlyComposition(t *testing.T) {
	loc := istanbulLoc(t)
	analyzerID := uuid.New()
	sc := store.Scope{CompanyID: uuid.New(), BuildingIDs: []uuid.UUID{uuid.New()}}
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, loc)
	dec31 := time.Date(2025, 12, 31, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)

	analytics := fakeAnalytics{
		t: t, wantScope: &sc,
		monthlyRows: []model.ConsumptionBucket{}, // empty, never nil: forces composition, never a fallback to `rows`.
		dailyRows: []model.ConsumptionBucket{
			{AnalyzerID: analyzerID, Bucket: dec31, ActiveIndex: dec("1000")},
			{AnalyzerID: analyzerID, Bucket: jan15, ActiveIndex: dec("1100")},
		},
	}
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analytics, Log: testLog(t)})
	require.NoError(t, err)

	ctx := context.Background()
	rows, err := a.Consumption(ctx, sc, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err, "every AnalyticsRepository call, including composeMissingPeriods' ConsumptionDaily, must use the caller's own Scope")
	require.Len(t, rows, 1, "sanity: composition actually ran and produced a row")
}

// --- M-2: a composed row with nothing before it is omitted, not all-nil ---

// TestComposedRowIsOmittedWhenNoDailyBucketPrecedesThePeriod is M-2: when
// there is no consumption_daily bucket at all before the missing period,
// there is nothing to subtract from, so no row is emitted — never a row
// whose every value is nil.
func TestComposedRowIsOmittedWhenNoDailyBucketPrecedesThePeriod(t *testing.T) {
	loc := istanbulLoc(t)
	analyzerID := uuid.New()
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)

	analytics := fakeAnalytics{
		dailyRows: []model.ConsumptionBucket{
			// Only a bucket INSIDE January — nothing before it at all.
			{AnalyzerID: analyzerID, Bucket: jan15, ActiveIndex: dec("100")},
		},
	}
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analytics, Log: testLog(t)})
	require.NoError(t, err)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	rows, err := a.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Empty(t, rows, "nothing to subtract from: no row, never an all-nil-values row")
}

// --- R102: a negative consumption is nil, never a suspicious number --------

// TestMaterialisedNegativeConsumptionIsNilNotNegative reproduces final
// review A's meter-swap probe (M-1): a materialised bucket whose own delta
// column has gone negative (index 50000, then 120 after a physical meter
// swap) must surface as nil, never as -49880 — Analytics has no suspicion
// channel, so a negative number here would silently double-count into
// Summarise/Balance with no signal at all.
func TestMaterialisedNegativeConsumptionIsNilNotNegative(t *testing.T) {
	loc := istanbulLoc(t)
	analyzerID := uuid.New()
	dayStart := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)

	analytics := fakeAnalytics{
		dailyRows: []model.ConsumptionBucket{
			{
				AnalyzerID:        analyzerID,
				Bucket:            dayStart,
				ActiveConsumption: dec("-49880"),
				ActiveIndex:       dec("120"),
			},
		},
	}
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analytics, Log: testLog(t)})
	require.NoError(t, err)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	rows, err := a.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Daily,
		Range:       store.TimeRange{From: dayStart, To: dayStart.Add(24 * time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Nil(t, rows[0].Values[energy.ActiveImport], "a negative materialised consumption must come back nil, never -49880")
}

// TestComposedNegativeConsumptionIsNilNotNegative is
// TestMaterialisedNegativeConsumptionIsNilNotNegative's counterpart for a
// COMPOSED row (R94): the last consumption_daily closing index before the
// missing month (50000) is higher than the last one inside it (120) — the
// same meter-swap shape as the review's probe — so the composed
// inside-minus-before subtraction goes negative and must come back nil, not
// -49880.
func TestComposedNegativeConsumptionIsNilNotNegative(t *testing.T) {
	loc := istanbulLoc(t)
	analyzerID := uuid.New()
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	jan31 := time.Date(2026, 1, 31, 0, 0, 0, 0, loc)
	feb10 := time.Date(2026, 2, 10, 0, 0, 0, 0, loc)
	midFeb := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)

	analytics := fakeAnalytics{
		dailyRows: []model.ConsumptionBucket{
			{AnalyzerID: analyzerID, Bucket: jan31, ActiveIndex: dec("50000")},
			{AnalyzerID: analyzerID, Bucket: feb10, ActiveIndex: dec("120")},
		},
	}
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analytics, Log: testLog(t)})
	require.NoError(t, err)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	rows, err := a.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: midFeb},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1, "January has no monthly materialised row either, but only February is under test here")

	feb := rows[0]
	require.True(t, feb.Partial)
	require.Nil(t, feb.Values[energy.ActiveImport], "a negative composed consumption (120 - 50000) must come back nil, never -49880")
}
