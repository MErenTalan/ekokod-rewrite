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
// — the one condition under which a "fall back to Readings when the
// aggregate is empty" mutation would actually have somewhere to fall back
// TO, which is what makes this fixture prove the mutation's absence rather
// than merely fail to exercise it.
//
// This behavioural probe alone is vacuous against a
// FIELD-level bypass — adding a poisoned field nobody's code path ever
// calls (or a narrower consumer-defined interface a real repository could
// still satisfy) leaves it green. It delegates to guard_test.go's
// reflection walk (the same one
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

// TestAnalyticsQueriesTheWidenedBucketRange proves a monthly request
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

// --- Ratios must come from the bucket's consumption, not its index -------

// TestAnalyticsRatiosComeFromConsumptionValuesNotClosingIndexes proves the
// bucket's consumption values (100/30/6) and its
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

// --- Validation runs before ANY I/O, on the Analytics path ---------------

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

// duplicateAnalyzerID returns two uuid.UUIDs that are the
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

// TestAnalyticsRefusesAnUnrecognisedLevel proves an unknown Level must be
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

// --- Analytics repository calls must use the caller's own Scope ----------

// TestAnalyticsCallsUseTheCallersScope is the Analytics-side unit test,
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

// --- A composed row with nothing before it is omitted, not all-nil -------

// --- R102: a negative consumption is nil, never a suspicious number --------

// TestMaterialisedNegativeConsumptionIsNilNotNegative proves a materialised
// bucket whose own delta
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

// R499: a month or year the next period's first reading does not close is
// Partial — still open, or its data stops short. Hours and days never are.
func TestCoarseRowsArePartialUnlessClosedByABoundary(t *testing.T) {
	loc := istanbulLoc(t)
	id := uuid.New()
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)

	rows := []model.ConsumptionBucket{
		{AnalyzerID: id, Bucket: jan, ActiveConsumption: dec("300"), ClosedByBoundary: true},
		{AnalyzerID: id, Bucket: feb, ActiveConsumption: dec("120")},
	}
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{
		Analytics: fakeAnalytics{monthlyRows: rows, yearlyRows: rows[1:], dailyRows: rows[1:]}, Log: testLog(t)})
	require.NoError(t, err)
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	get := func(level energy.Level, from, to time.Time) []consumption.Row {
		got, err := a.Consumption(context.Background(), scope, consumption.SeriesRequest{
			AnalyzerIDs: []uuid.UUID{id}, Level: level, Range: store.TimeRange{From: from, To: to}})
		require.NoError(t, err)
		return got
	}

	monthly := get(energy.Monthly, jan, feb.AddDate(0, 1, 0))
	require.Len(t, monthly, 2)
	require.False(t, monthly[0].Partial, "January ends on February's first reading")
	require.True(t, monthly[1].Partial, "nothing closes February")
	require.Equal(t, "120", monthly[1].Values[energy.ActiveImport].String())

	daily := get(energy.Daily, feb, feb.AddDate(0, 0, 1))
	require.Len(t, daily, 1)
	require.False(t, daily[0].Partial, "only months and years are marked")
}
