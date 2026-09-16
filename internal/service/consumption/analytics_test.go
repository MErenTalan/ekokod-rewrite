package consumption_test

import (
	"context"
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

// TestAnalyticsPathCannotReachTheHypertable is R61's structural guard:
// AnalyticsDeps has no ReadingRepository field at all, so there is nothing
// to poison and no fallback path to accidentally take. fakeAnalytics{rows:
// nil} deliberately returns NO rows (I-8) — the one condition under which a
// "fall back to Readings when the aggregate is empty" mutation would
// actually have somewhere to fall back TO, which is what makes this fixture
// prove the mutation's absence rather than merely fail to exercise it.
func TestAnalyticsPathCannotReachTheHypertable(t *testing.T) {
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

// TestOpenMonthIsComposedFromDailyClosingIndexes is R88's acceptance test.
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
// MaxDemandKw is composed from the SAME "last daily bucket inside the open
// period" row (10 Feb), independent of the subtraction: 12.5000.
func TestOpenMonthIsComposedFromDailyClosingIndexes(t *testing.T) {
	loc := istanbulLoc(t)
	analyzerID := uuid.New()

	janStart := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	jan31 := time.Date(2026, 1, 31, 0, 0, 0, 0, loc)
	feb10 := time.Date(2026, 2, 10, 0, 0, 0, 0, loc)
	midFeb := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)

	analytics := fakeAnalytics{
		monthlyRows: []model.ConsumptionBucket{
			{AnalyzerID: analyzerID, Bucket: janStart, ActiveConsumption: dec("3000.0000"), ActiveIndex: dec("5000.0000")},
		},
		dailyRows: []model.ConsumptionBucket{
			{AnalyzerID: analyzerID, Bucket: jan31, ActiveIndex: dec("5000.0000")},
			{AnalyzerID: analyzerID, Bucket: feb10, ActiveIndex: dec("5300.0000"), MaxDemandKw: dec("12.5000")},
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
	require.Equal(t, "12.5", open.MaxDemandKw.String())

	closed := rows[0]
	require.False(t, closed.Partial, "January is a closed, materialized row")
}
