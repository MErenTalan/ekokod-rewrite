//go:build integration

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
