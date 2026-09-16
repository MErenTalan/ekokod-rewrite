//go:build integration

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

// pathsEpoch is the fixed instant this file's real-database tests anchor
// their windows and readings to.
var pathsEpoch = time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)

// sub returns a.Values[reg] - b.Values[reg], for asserting the exact step
// two Rows differ by.
func sub(a, b consumption.Row, reg energy.Register) decimal.Decimal {
	return a.Values[reg].Sub(*b.Values[reg])
}

// TestBillingAndAnalyticsDifferByTheBucketBoundaryStep is 09 §F3's headline
// acceptance criterion, proven against REAL repositories (not fakes): the
// analytics path reads consumption_hourly (built by migration 00005 as
// last(active_import) - first(active_import) inside the bucket) and the
// billing path reads meter_readings at the bucket's true [09:00, 10:00)
// boundaries.
//
// Hand arithmetic. Readings at 09:00=1000, 09:45=1030, 10:00=1040:
//
//	analytics (last-first INSIDE the 09:00 bucket) = 1030 - 1000 = 30
//	billing   (true boundary 09:00 -> true boundary 10:00) = 1040 - 1000 = 40
//	difference = 40 - 30 = 10, exactly the 09:45->10:00 step the aggregate misses.
func TestBillingAndAnalyticsDifferByTheBucketBoundaryStep(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	analyzerID := tenant.Analyzers[0].ID

	readingRepo := pathsNewReadingRepo(pool)
	analyticsRepo := pathsNewAnalyticsRepo(pool)

	hourStart := pathsEpoch // on the hour, so consumption_hourly's UTC bucket lines up exactly.
	rows := []model.MeterReading{
		readingRow(analyzerID, hourStart, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		readingRow(analyzerID, hourStart.Add(45*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1030"}),
		readingRow(analyzerID, hourStart.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
	}
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, rows[0])
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, rows[1])
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, rows[2])

	analytics, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: analyticsRepo, Log: testfixtures.DiscardLogger()})
	require.NoError(t, err)
	billing, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: readingRepo, Anomalies: noAnomalies{}, Ops: noOps{},
		Clock: clock.NewFake(hourStart.Add(time.Hour)), Log: testfixtures.DiscardLogger(),
	})
	require.NoError(t, err)

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

	diff := sub(billingRows[0], analyticsRows[0], energy.ActiveImport)
	require.Equal(t, "10", diff.String(), "exactly the step the aggregate misses")
}

// TestBillingIsolatesTenants (F1 ruling 4): the cross-tenant assertion uses
// the OTHER tenant's AdminScope, never a narrow Scope, with a positive
// control that proves the seeded row is real before checking that it never
// crosses tenants.
//
// meter_readings carries no company_id (repository.go's header): isolation
// is enforced entirely by the join through analyzers inside
// ReadingRepository's own SQL. Billing.Consumption never re-implements that
// predicate — it only ever forwards the caller's Scope — so this test also
// pins that Billing passes the Scope through unmodified: Step 5's mutation
// (substituting a different, valid Scope for one repository call) turns it
// red by making tenant B's analyzer suddenly visible to tenant A's request.
func TestBillingIsolatesTenants(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	readingRepo := pathsNewReadingRepo(pool)

	row := readingRow(tenantB.Analyzers[0].ID, pathsEpoch, model.ReadingKindLoadProfile, map[string]string{"active_import": "999"})
	rowEnd := readingRow(tenantB.Analyzers[0].ID, pathsEpoch.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})
	pathsSeedReading(t, ctx, readingRepo, tenantB.Scope, row)
	pathsSeedReading(t, ctx, readingRepo, tenantB.Scope, rowEnd)

	billing, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: readingRepo, Anomalies: noAnomalies{}, Ops: noOps{},
		Clock: clock.NewFake(pathsEpoch.Add(time.Hour)), Log: testfixtures.DiscardLogger(),
	})
	require.NoError(t, err)

	req := func(scope store.Scope, analyzerID uuid.UUID) ([]consumption.Row, error) {
		return billing.Consumption(ctx, scope, consumption.SeriesRequest{
			AnalyzerIDs: []uuid.UUID{analyzerID},
			Level:       energy.Hourly,
			Range:       store.TimeRange{From: pathsEpoch, To: pathsEpoch.Add(time.Hour)},
		})
	}

	// Positive control: tenant B reads its own seeded row through its own
	// Scope, and gets a real value back.
	selfB, err := req(tenantB.Scope, tenantB.Analyzers[0].ID)
	require.NoError(t, err)
	require.Len(t, selfB, 1)
	require.NotNil(t, selfB[0].Values[energy.ActiveImport])
	require.Equal(t, "1", selfB[0].Values[energy.ActiveImport].String())

	// Cross-tenant: tenant A's AdminScope naming tenant B's real analyzer id
	// must never surface tenant B's rows. ReadingRepository's isolation
	// returns ErrNotFound for an analyzer not visible to the scope, so
	// Billing.Consumption must propagate that error rather than silently
	// returning tenant B's data.
	_, err = req(tenantA.AdminScope, tenantB.Analyzers[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestBillingFlagsAResetAtTheUpperBoundAgainstARealDatabase is C2's
// integration half (review probe C/(c)): a reset row sharing the request's
// own upper bound with a non-reset end reading of a DIFFERENT value must
// come out nil + meter_reset (R90) — never a wrong number. This is provable
// only against a REAL timestamptz column: Go's own time.Time arithmetic has
// nanosecond resolution and never rounds through a wire encoding the way
// Postgres's timestamptz does, so mutation (c) (widening the reset query's
// upper bound by time.Nanosecond instead of time.Microsecond) is invisible
// to a fake repository and stays green against one.
func TestBillingFlagsAResetAtTheUpperBoundAgainstARealDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	repo := pathsNewReadingRepo(pool)
	h := pathsEpoch

	pathsSeedReading(t, ctx, repo, tenant.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, repo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1110"}))
	pathsSeedReading(t, ctx, repo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindReset, map[string]string{"active_import": "100"}))

	billing, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: repo, Anomalies: noAnomalies{}, Ops: noOps{},
		Clock: clock.NewFake(h.Add(time.Hour)), Log: testfixtures.DiscardLogger(),
	})
	require.NoError(t, err)

	rows, err := billing.Consumption(ctx, tenant.Scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{id},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: h, To: h.Add(time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Nil(t, rows[0].Values[energy.ActiveImport], "the reset at the upper bound must never be billed as a number")
	require.Contains(t, rows[0].Suspect, energy.ActiveImport)
	require.Equal(t, energy.ReasonMeterReset, rows[0].Suspect[energy.ActiveImport].Reason)
}

// TestNarrowScopeSeesNothingOnBothPaths is I-7: a same-company Scope whose
// BuildingIDs exclude the analyzer's own building must see nothing on
// EITHER path — Billing refuses with store.ErrNotFound (its normal per-call
// isolation contract), Analytics returns an empty slice — with a positive
// control (the analyzer that IS inside the narrow scope) proving real data
// exists on both paths.
func TestNarrowScopeSeesNothingOnBothPaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	require.GreaterOrEqual(t, len(tenant.Analyzers), 3, "need an analyzer under the SECOND building, outside tenant.Scope")

	inScope := tenant.Analyzers[0].ID    // under Buildings[0]: covered by tenant.Scope
	outOfScope := tenant.Analyzers[2].ID // under Buildings[1]: NOT covered

	repo := pathsNewReadingRepo(pool)
	h := pathsEpoch
	for _, id := range []uuid.UUID{inScope, outOfScope} {
		pathsSeedReading(t, ctx, repo, tenant.AdminScope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
		pathsSeedReading(t, ctx, repo, tenant.AdminScope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1100"}))
	}

	billing, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: repo, Anomalies: noAnomalies{}, Ops: noOps{},
		Clock: clock.NewFake(h.Add(time.Hour)), Log: testfixtures.DiscardLogger(),
	})
	require.NoError(t, err)
	analytics, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: pathsNewAnalyticsRepo(pool), Log: testfixtures.DiscardLogger()})
	require.NoError(t, err)

	req := func(id uuid.UUID) consumption.SeriesRequest {
		return consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	}

	// Positive control: the in-scope analyzer is visible on both paths.
	brows, err := billing.Consumption(ctx, tenant.Scope, req(inScope))
	require.NoError(t, err)
	require.Len(t, brows, 1)
	arows, err := analytics.Consumption(ctx, tenant.Scope, req(inScope))
	require.NoError(t, err)
	require.Len(t, arows, 1)

	// The narrow scope must see nothing of the out-of-scope analyzer.
	_, err = billing.Consumption(ctx, tenant.Scope, req(outOfScope))
	require.ErrorIs(t, err, store.ErrNotFound)
	aEmpty, err := analytics.Consumption(ctx, tenant.Scope, req(outOfScope))
	require.NoError(t, err)
	require.Empty(t, aEmpty)
}

// TestAnalyticsIsolatesTenants is I-7's Analytics half: the other tenant's
// AdminScope — the widest legitimate scope a company can hold — must never
// see a row belonging to a different company, with a positive control that
// proves the seeded row is real before checking that it never crosses
// tenants.
func TestAnalyticsIsolatesTenants(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := pathsNewReadingRepo(pool)

	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := jan1.AddDate(0, 1, 0)

	id := tenantB.Analyzers[0].ID
	pathsSeedReading(t, ctx, repo, tenantB.Scope, readingRow(id, jan1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, repo, tenantB.Scope, readingRow(id, feb1, model.ReadingKindLoadProfile, map[string]string{"active_import": "2000"}))

	_, err = pool.Exec(ctx, "call refresh_continuous_aggregate('consumption_monthly', NULL, NULL)")
	require.NoError(t, err)

	analytics, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: pathsNewAnalyticsRepo(pool), Log: testfixtures.DiscardLogger()})
	require.NoError(t, err)

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Monthly, Range: store.TimeRange{From: jan1, To: feb1}}

	// Positive control.
	self, err := analytics.Consumption(ctx, tenantB.Scope, req)
	require.NoError(t, err)
	require.Len(t, self, 1)

	// Cross-tenant: tenant A's AdminScope must see nothing of tenant B's row.
	cross, err := analytics.Consumption(ctx, tenantA.AdminScope, req)
	require.NoError(t, err)
	require.Empty(t, cross)
}

// TestAnalyticsComposesClosedUnrefreshedMonthsAgainstARealDatabase is R94's
// integration acceptance test (the review's own probe): consumption_monthly
// is NEVER refreshed. load_profile readings run continuously from Dec 31
// through Mar 10. A request for [Jan 1, Apr 1) must return a composed,
// Partial row for EVERY one of January, February and March — not only
// March, which happens to be the LAST window in the request (the review's
// I-3 finding: January and February silently vanished because only the
// trailing window was ever a composition candidate).
func TestAnalyticsComposesClosedUnrefreshedMonthsAgainstARealDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	repo := pathsNewReadingRepo(pool)

	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	start := time.Date(2025, 12, 31, 0, 0, 0, 0, loc)
	end := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)

	var rows []model.MeterReading
	v := 1000
	for ts := start; ts.Before(end); ts = ts.Add(12 * time.Hour) {
		rows = append(rows, readingRow(id, ts, model.ReadingKindLoadProfile, map[string]string{"active_import": fmt.Sprintf("%d", v)}))
		v += 10
	}
	_, _, err = repo.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: pathsNewAnalyticsRepo(pool), Log: testfixtures.DiscardLogger()})
	require.NoError(t, err)

	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	apr1 := time.Date(2026, 4, 1, 0, 0, 0, 0, loc)
	got, err := a.Consumption(ctx, tenant.Scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{id},
		Level:       energy.Monthly,
		Range:       store.TimeRange{From: jan1, To: apr1},
	})
	require.NoError(t, err)

	byMonth := make(map[string]consumption.Row, len(got))
	for _, r := range got {
		byMonth[r.Window.From.In(loc).Format("2006-01")] = r
	}
	require.Contains(t, byMonth, "2026-01", "January must not silently disappear")
	require.Contains(t, byMonth, "2026-02", "February must not silently disappear, even though it is not the last window")
	require.Contains(t, byMonth, "2026-03")
	for _, m := range []string{"2026-01", "2026-02", "2026-03"} {
		require.True(t, byMonth[m].Partial, "%s must be composed, never materialized: consumption_monthly is never refreshed here", m)
	}
}
