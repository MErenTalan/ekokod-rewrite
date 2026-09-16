//go:build integration

// This file is Task 13's acceptance suite for 09-implementation-plan.md
// §F3: one test per acceptance criterion, named after it, with a comment
// quoting the criterion verbatim (M-5: it lives in-package, next to the code
// it exercises, not in a separate internal/acceptance package).
//
// Wiring note (clarification binding this dispatch): consumption.Analytics
// and consumption.Billing are NOT constructed by worker.Build — their only
// consumer through F5 is the HTTP layer, which does not exist until F6 — so
// the plan's "drive worker.Build" rule cannot apply here. Every test below
// instead constructs *Analytics/*Billing directly with REAL repositories
// (postgres.NewReadingRepository, postgres.NewAnalyticsRepository) over
// testfixtures.NewIsolatedDB, a real clock.Clock fake, and a real tenant
// from testfixtures.NewTenant — exactly the pattern paths_integration_test.go
// and service_integration_test.go already use for this package's other
// real-database tests.
//
// The golden-file criterion ("Golden-file tests for consumption derivation
// against hand-computed fixtures...") is NOT re-asserted here (M-5): Task
// 4's TestGolden (internal/domain/energy/golden_test.go, run over
// internal/domain/energy/testdata/golden/*.json) already fails loudly on
// any of the six cases regressing, every time `go test ./internal/domain/energy
// -run TestGolden` runs — see the phase verification block. A second,
// acceptance-layer restatement here would only re-assert "the six files
// exist and run", which TestGolden already guarantees.
//
// TestF3NegativeDeltaProducesNullSuspectAndMessage (the negative-delta ->
// NULL + suspect + operator message criterion) is deliberately ABSENT from
// this file: it needs Task 8's Billing.ConsumptionAndRecord (the write-and-
// record wrapper that owns consumption_anomalies and OperationalMessage),
// which is not on this branch's base (Task 8 is still in its fix round). It
// will be added once Task 8 merges.
package consumption_test

import (
	"context"
	"fmt"
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
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// f3NewBilling builds a *consumption.Billing wired to REAL repositories
// (readingRepo, analyticsRepo unused here) plus the panicking no-op
// AnomalyRepository/OpsRepository fakes: Task 7's plain Consumption (what
// this file exercises) must never reach either — that is Task 8's
// ConsumptionAndRecord, on the same type, not merged onto this branch yet.
func f3NewBilling(t *testing.T, readingRepo store.ReadingRepository, now time.Time) *consumption.Billing {
	t.Helper()
	b, err := consumption.NewBilling(consumption.BillingDeps{
		Readings:  readingRepo,
		Anomalies: noAnomalies{},
		Ops:       noOps{},
		Clock:     clock.NewFake(now),
		Log:       testfixtures.DiscardLogger(),
	})
	require.NoError(t, err)
	return b
}

func f3NewAnalytics(t *testing.T, analyticsRepo store.AnalyticsRepository) *consumption.Analytics {
	t.Helper()
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analyticsRepo, Log: testfixtures.DiscardLogger()})
	require.NoError(t, err)
	return a
}

// TestF3LevelsDeriveFromTheirOwnBoundaries is 09 §F3's criterion: "Hourly,
// daily, monthly and yearly figures each derive from their own boundaries;
// summing the level below is asserted not to be how they are produced."
//
// Proven twice, against a real database, through Billing.Consumption:
//
//  1. Hourly vs. Daily: dailyFixtureReadings (billing_test.go) is a 25-point,
//     hourly series across one Istanbul day whose hour 10 is a negative
//     delta (1100 -> 1050) with no covering reset, so hour 10 is suspect
//     (nil). The daily row derives directly from its own two boundaries:
//     1240 - 1000 = 240, untouched by hour 10's problem. Summing the 23
//     non-suspect hourly rows instead gives 240 - (-50) = 290 (the
//     telescoping identity: the true sum of all 24 deltas is always 240,
//     so omitting hour 10's own -50 from that total leaves 290). 290 != 240
//     is the assertion that summing the level below is not how the daily
//     figure is produced.
//
//  2. Monthly vs. Daily: January has one reading at every day's own 00:00
//     boundary, value(d) = 1000 + 100*(d-1) for d = 1..31, EXCEPT day 15's
//     reading is never written. R96 caps every kind's boundary tolerance at
//     half the bucket's own width (12h at Daily level), so a 24h-old
//     neighbour can never stand in for the missing Jan-15 00:00 instant:
//     both the bucket ending there ([Jan 14, Jan 15)) and the bucket
//     starting there ([Jan 15, Jan 16)) resolve to no row. Twenty-nine of
//     January's 31 daily buckets survive. Their sum is
//     (2300-1000) + (4100-2500) = 1300 + 1600 = 2900 (value(Jan 14) = 2300,
//     value(Jan 16) = 2500, value(Feb 1) = 4100). The monthly row derives
//     directly from ITS OWN two boundaries (Jan 1 = 1000, Feb 1 = 4100):
//     4100 - 1000 = 3100. 3100 != 2900 is the same assertion at the
//     monthly/daily pair: the two missing daily buckets (worth exactly
//     value(Jan 16) - value(Jan 14) = 200) are invisible to a sum of the
//     level below, but not to the monthly row's own boundary derivation.
func TestF3LevelsDeriveFromTheirOwnBoundaries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	readingRepo := pathsNewReadingRepo(pool)

	loc := istanbulLoc(t)

	// --- Part 1: Hourly vs. Daily -------------------------------------
	hourlyAnalyzerID := tenant.Analyzers[0].ID
	dayStart := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	hourlyFixture := dailyFixtureReadings(hourlyAnalyzerID, dayStart)
	_, _, err := readingRepo.BulkInsert(ctx, tenant.Scope, hourlyFixture)
	require.NoError(t, err)

	billing := f3NewBilling(t, readingRepo, dayStart.Add(24*time.Hour))

	hourlyRows, err := billing.Consumption(ctx, tenant.Scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{hourlyAnalyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: dayStart, To: dayStart.Add(24 * time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, hourlyRows, 24, "every hour has both boundaries, even the suspect one")

	dailyRows, err := billing.Consumption(ctx, tenant.Scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{hourlyAnalyzerID},
		Level:       energy.Daily,
		Range:       store.TimeRange{From: dayStart, To: dayStart.Add(24 * time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, dailyRows, 1)

	require.Nil(t, hourlyRows[10].Values[energy.ActiveImport], "hour 10's negative delta with no reset is suspect")
	require.NotNil(t, dailyRows[0].Values[energy.ActiveImport])
	require.Equal(t, "240", dailyRows[0].Values[energy.ActiveImport].String(), "the daily row derives from its own two boundaries")

	sumOfHours := sumOfNonNilHourly(hourlyRows, energy.ActiveImport)
	require.Equal(t, "290", sumOfHours.String())
	require.NotEqual(t, dailyRows[0].Values[energy.ActiveImport].String(), sumOfHours.String(),
		"summing the non-suspect hours is not how the daily figure is produced")

	// --- Part 2: Monthly vs. Daily -------------------------------------
	monthlyAnalyzerID := tenant.Analyzers[1].ID
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := jan1.AddDate(0, 1, 0)

	var monthRows []model.MeterReading
	for d := 1; d <= 31; d++ {
		if d == 15 {
			continue // deliberately missing: the day-15 boundary instant.
		}
		ts := time.Date(2026, 1, d, 0, 0, 0, 0, loc)
		val := fmt.Sprintf("%d", 1000+100*(d-1))
		monthRows = append(monthRows, readingRow(monthlyAnalyzerID, ts, model.ReadingKindLoadProfile, map[string]string{"active_import": val}))
	}
	monthRows = append(monthRows, readingRow(monthlyAnalyzerID, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "4100"}))
	_, _, err = readingRepo.BulkInsert(ctx, tenant.Scope, monthRows)
	require.NoError(t, err)

	monthBilling := f3NewBilling(t, readingRepo, feb1)

	monthlyDailyRows, err := monthBilling.Consumption(ctx, tenant.Scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{monthlyAnalyzerID},
		Level:       energy.Daily,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, monthlyDailyRows, 29, "31 January days minus the 2 buckets touching the missing Jan-15 boundary")

	monthlyRows, err := monthBilling.Consumption(ctx, tenant.Scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{monthlyAnalyzerID},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: feb1},
	})
	require.NoError(t, err)
	require.Len(t, monthlyRows, 1)
	require.NotNil(t, monthlyRows[0].Values[energy.ActiveImport])
	require.Equal(t, "3100", monthlyRows[0].Values[energy.ActiveImport].String(), "the monthly row derives from its own two boundaries")

	dailySum := decimal.Zero
	for _, r := range monthlyDailyRows {
		if v := r.Values[energy.ActiveImport]; v != nil {
			dailySum = dailySum.Add(*v)
		}
	}
	require.Equal(t, "2900", dailySum.String())
	require.NotEqual(t, monthlyRows[0].Values[energy.ActiveImport].String(), dailySum.String(),
		"summing the daily rows is not how the monthly figure is produced")
}

// TestF3BillingAndAnalyticsDifferByTheBoundaryStep is 09 §F3's criterion:
// "billing.Consumption and analytics.Consumption are shown to differ at a
// bucket boundary by the expected step, documenting the trade-off in a
// test."
//
// Hand arithmetic. Readings at 09:00=1000, 09:45=1030, 10:00=1040:
//
//	analytics (consumption_hourly's last(active_import)-first(active_import)
//	           INSIDE the [09:00,10:00) bucket) = 1030 - 1000 = 30
//	billing   (true boundary 09:00 -> true boundary 10:00) = 1040 - 1000 = 40
//	difference = 40 - 30 = 10, exactly the 09:45->10:00 step the aggregate
//	misses (04 §4.3).
func TestF3BillingAndAnalyticsDifferByTheBoundaryStep(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	analyzerID := tenant.Analyzers[0].ID

	readingRepo := pathsNewReadingRepo(pool)
	analyticsRepo := pathsNewAnalyticsRepo(pool)

	hourStart := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC) // on the hour, so consumption_hourly's UTC bucket lines up exactly.
	rows := []model.MeterReading{
		readingRow(analyzerID, hourStart, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		readingRow(analyzerID, hourStart.Add(45*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1030"}),
		readingRow(analyzerID, hourStart.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
	}
	_, _, err := readingRepo.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	analytics := f3NewAnalytics(t, analyticsRepo)
	billing := f3NewBilling(t, readingRepo, hourStart.Add(time.Hour))

	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: hourStart, To: hourStart.Add(time.Hour)},
	}

	analyticsRows, err := analytics.Consumption(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Len(t, analyticsRows, 1)
	require.NotNil(t, analyticsRows[0].Values[energy.ActiveImport])
	require.Equal(t, "30", analyticsRows[0].Values[energy.ActiveImport].String())

	billingRows, err := billing.Consumption(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Len(t, billingRows, 1)
	require.NotNil(t, billingRows[0].Values[energy.ActiveImport])
	require.Equal(t, "40", billingRows[0].Values[energy.ActiveImport].String())

	diff := billingRows[0].Values[energy.ActiveImport].Sub(*analyticsRows[0].Values[energy.ActiveImport])
	require.Equal(t, "10", diff.String(), "exactly the 09:45->10:00 step the aggregate misses")
}

// TestF3MonthlyPrefersBillingReadings is 09 §F3's criterion: "Monthly
// consumption prefers billing-kind readings when present."
//
// Two analyzers, same calendar month (January 2026), same tenant:
//
//   - Analyzer A has billing-kind readings at both January boundaries
//     (1000 -> 6000, exactly at the bounds, well within
//     BillingSnapshotTolerance) AND load_profile readings at the SAME two
//     instants with DIFFERENT values (1000 -> 1200), so the fixture is
//     unambiguous about which kind produced the result: billing gives
//     6000-1000 = 5000; load_profile would have given 1200-1000 = 200.
//   - Analyzer B has ONLY load_profile readings at the same two instants
//     (1000 -> 1200) — no billing-kind reading anywhere near January.
//
// Hand arithmetic: A's monthly row must be exactly 5000 (Source = billing);
// B's monthly row must be exactly 200 = 1200 - 1000 (Source = load_profile).
func TestF3MonthlyPrefersBillingReadings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	readingRepo := pathsNewReadingRepo(pool)

	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := jan1.AddDate(0, 1, 0)

	analyzerA := tenant.Analyzers[0].ID
	analyzerB := tenant.Analyzers[1].ID

	rows := []model.MeterReading{
		readingRow(analyzerA, jan1, model.ReadingKindBilling, map[string]string{"active_import": "1000"}),
		readingRow(analyzerA, feb1, model.ReadingKindBilling, map[string]string{"active_import": "6000"}),
		readingRow(analyzerA, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		readingRow(analyzerA, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1200"}),
		readingRow(analyzerB, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		readingRow(analyzerB, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1200"}),
	}
	_, _, err := readingRepo.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	billing := f3NewBilling(t, readingRepo, feb1)

	reqFor := func(id uuid.UUID) consumption.SeriesRequest {
		return consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Monthly, Range: store.TimeRange{From: jan1, To: feb1}}
	}

	aRows, err := billing.Consumption(ctx, tenant.Scope, reqFor(analyzerA))
	require.NoError(t, err)
	require.Len(t, aRows, 1)
	require.Equal(t, energy.KindBilling, aRows[0].Source, "billing-kind readings cover the month, so they are preferred")
	require.NotNil(t, aRows[0].Values[energy.ActiveImport])
	require.Equal(t, "5000", aRows[0].Values[energy.ActiveImport].String(), "the utility snapshot, not the load profile")

	bRows, err := billing.Consumption(ctx, tenant.Scope, reqFor(analyzerB))
	require.NoError(t, err)
	require.Len(t, bRows, 1)
	require.Equal(t, energy.KindLoadProfile, bRows[0].Source, "no billing-kind reading covers this analyzer's month")
	require.NotNil(t, bRows[0].Values[energy.ActiveImport])
	require.Equal(t, "200", bRows[0].Values[energy.ActiveImport].String())
}

// TestF3ZeroConsumptionProducesANullRatio is 09 §F3's criterion: "A
// zero-consumption period produces a null ratio, not a ratio computed
// against a substituted 1."
//
// One analyzer, one hour, active_import UNCHANGED across the boundary
// (1000 -> 1000, zero consumption) while both reactive registers DO move
// (inductive 500 -> 530, delta 30; capacitive 200 -> 220, delta 20). Hand
// arithmetic: InductiveRatio would be 30/0 and CapacitiveRatio 20/0 if the
// legacy defect (reactivePenalty.ts:47's `totalActive === 0 ? 1 :
// totalActive`) were reproduced — 30 and 20 respectively. R54/energy.Ratio
// instead requires both to be nil: a zero (non-positive) denominator is
// never substituted. Proven through BOTH consumption paths against the
// same real data: Billing.Consumption reads meter_readings directly;
// Analytics.Consumption reads consumption_hourly, which is
// materialized_only = false (migration 00005), so the real-time aggregate
// sees the just-inserted readings with no refresh_continuous_aggregate
// call needed.
func TestF3ZeroConsumptionProducesANullRatio(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	analyzerID := tenant.Analyzers[0].ID

	readingRepo := pathsNewReadingRepo(pool)
	analyticsRepo := pathsNewAnalyticsRepo(pool)

	hourStart := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	rows := []model.MeterReading{
		readingRow(analyzerID, hourStart, model.ReadingKindLoadProfile, map[string]string{
			"active_import": "1000", "reactive_inductive_import": "500", "reactive_capacitive_import": "200",
		}),
		readingRow(analyzerID, hourStart.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{
			"active_import": "1000", "reactive_inductive_import": "530", "reactive_capacitive_import": "220",
		}),
	}
	_, _, err := readingRepo.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: hourStart, To: hourStart.Add(time.Hour)},
	}

	billing := f3NewBilling(t, readingRepo, hourStart.Add(time.Hour))
	billingRows, err := billing.Consumption(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Len(t, billingRows, 1)
	require.NotNil(t, billingRows[0].Values[energy.ActiveImport])
	require.Equal(t, "0", billingRows[0].Values[energy.ActiveImport].String())
	require.NotNil(t, billingRows[0].Values[energy.ReactiveInductiveImport])
	require.Equal(t, "30", billingRows[0].Values[energy.ReactiveInductiveImport].String())
	require.Nil(t, billingRows[0].InductiveRatio, "30/0 must be null, never a substituted 30/1")
	require.Nil(t, billingRows[0].CapacitiveRatio, "20/0 must be null, never a substituted 20/1")

	analytics := f3NewAnalytics(t, analyticsRepo)
	analyticsRows, err := analytics.Consumption(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Len(t, analyticsRows, 1)
	require.NotNil(t, analyticsRows[0].Values[energy.ActiveImport])
	require.Equal(t, "0", analyticsRows[0].Values[energy.ActiveImport].String())
	require.Nil(t, analyticsRows[0].InductiveRatio, "the analytics path must apply the same null-on-zero-denominator rule")
	require.Nil(t, analyticsRows[0].CapacitiveRatio)
}
