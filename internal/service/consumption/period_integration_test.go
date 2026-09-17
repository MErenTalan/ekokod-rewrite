//go:build integration

package consumption_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestPeriodConsumptionCutoffOneEqualsMonthly: R107 — cut-off day 1 is
// identical to Consumption at Monthly over the same real readings: billing
// snapshots, a reset, a negative delta and a gap.
func TestPeriodConsumptionCutoffOneEqualsMonthly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	loc := istanbulLoc(t)
	m := func(month time.Month) time.Time { return time.Date(2026, month, 1, 0, 0, 0, 0, loc) }
	readingRepo := pathsNewReadingRepo(pool)
	seed := func(ts time.Time, kind model.ReadingKind, v map[string]string) {
		pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, ts, kind, v))
	}

	// Jan 1 and Feb 1: billing snapshots (with the month peaks) beat the
	// load_profile readings at the same instants.
	seed(m(1).Add(-2*time.Hour), model.ReadingKindBilling, map[string]string{"active_import": "1000", "t1_import": "500"})
	seed(m(1), model.ReadingKindLoadProfile, map[string]string{"active_import": "1010", "t1_import": "505"})
	seed(m(1).Add(10*24*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1200", "max_demand_kw": "40"})
	seed(m(1).Add(20*24*time.Hour), model.ReadingKindBilling, map[string]string{"max_demand_kw": "95"})
	seed(m(2), model.ReadingKindBilling, map[string]string{"active_import": "2000", "t1_import": "900"})
	seed(m(2).Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "2005", "t1_import": "902"})
	// February: a registered reset, with a load_profile prior before it.
	seed(m(2).Add(9*24*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "2400", "t1_import": "1100"})
	seed(m(2).Add(10*24*time.Hour), model.ReadingKindReset, map[string]string{"active_import": "0", "t1_import": "0"})
	// Mar 1: no billing snapshot; load_profile 30 h early resolves it.
	seed(m(3).Add(-30*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "700", "t1_import": "300", "max_demand_kw": "70"})
	// March: negative delta on t1 only.
	seed(m(4), model.ReadingKindDaily, map[string]string{"active_import": "1500", "t1_import": "250"})
	// April: no reading anywhere near May 1 — a gap.

	now := m(5).Add(consumption.SettleDelayMonthly)
	b := anomaliesNewBilling(t, pool, readingRepo, postgres.NewAnomalyRepository(pool), postgres.NewOpsRepository(pool), postgres.NewAnalyzerRepository(pool), lock.NewMemory(nil), now)

	monthly, err := b.Consumption(ctx, tenant.Scope, consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Monthly, Range: store.TimeRange{From: m(1), To: m(5)}})
	require.NoError(t, err)
	period, err := b.PeriodConsumption(ctx, tenant.Scope, consumption.PeriodRequest{
		AnalyzerIDs: []uuid.UUID{id},
		Windows:     []energy.Window{win(m(1), m(2)), win(m(2), m(3)), win(m(3), m(4)), win(m(4), m(5))},
	})
	require.NoError(t, err)

	require.Len(t, monthly, 3, "April is a gap")
	require.Equal(t, monthly, period)

	// Pin the fixture's shape so the equality is not vacuous.
	require.Equal(t, energy.KindBilling, monthly[0].Source)
	require.Equal(t, "1000", monthly[0].Values[energy.ActiveImport].String())
	require.Equal(t, "95", monthly[0].MaxDemandKw.String())
	require.Equal(t, "1100", monthly[1].Values[energy.ActiveImport].String(), "(2400-2000) + (700-0)")
	require.Nil(t, monthly[2].Values[energy.T1Import])
	require.Equal(t, energy.ReasonNegativeDelta, monthly[2].Suspect[energy.T1Import].Reason)
	require.Equal(t, "800", monthly[2].Values[energy.ActiveImport].String())
}

// TestPeriodConsumptionAndRecordWritesMissingReadingsForTheWindow: R97 at an
// explicit window — the gap is recorded for exactly that window, once.
func TestPeriodConsumptionAndRecordWritesMissingReadingsForTheWindow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	loc := istanbulLoc(t)
	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, loc)
	feb15 := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)
	readingRepo := pathsNewReadingRepo(pool)
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, jan15, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))

	b := anomaliesNewBilling(t, pool, readingRepo, postgres.NewAnomalyRepository(pool), postgres.NewOpsRepository(pool), postgres.NewAnalyzerRepository(pool), lock.NewMemory(nil), feb15.Add(consumption.SettleDelayMonthly))
	req := consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: []energy.Window{win(jan15, feb15)}}
	for range 2 {
		rows, err := b.PeriodConsumptionAndRecord(ctx, tenant.Scope, req)
		require.NoError(t, err)
		require.Empty(t, rows)
	}

	an, err := b.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}})
	require.NoError(t, err)
	require.Len(t, an, 1)
	require.Equal(t, string(energy.ReasonMissingReadings), an[0].Reason)
	require.True(t, an[0].PeriodStart.Equal(jan15))
	require.True(t, an[0].PeriodEnd.Equal(feb15))
	var detail map[string]any
	require.NoError(t, json.Unmarshal(an[0].Detail, &detail))
	require.Equal(t, []any{"end"}, detail["boundaries"])
}

// TestPeriodConsumptionIsolatesTenants: another tenant's AdminScope cannot
// read an analyzer's cut-off consumption (F1 ruling 4), with a positive control.
func TestPeriodConsumptionIsolatesTenants(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	id := tenantB.Analyzers[0].ID
	loc := istanbulLoc(t)
	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, loc)
	feb15 := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)
	readingRepo := pathsNewReadingRepo(pool)
	pathsSeedReading(t, ctx, readingRepo, tenantB.Scope, readingRow(id, jan15, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, readingRepo, tenantB.Scope, readingRow(id, feb15, model.ReadingKindLoadProfile, map[string]string{"active_import": "1250"}))

	b := anomaliesNewBilling(t, pool, readingRepo, postgres.NewAnomalyRepository(pool), postgres.NewOpsRepository(pool), postgres.NewAnalyzerRepository(pool), lock.NewMemory(nil), feb15.Add(consumption.SettleDelayMonthly))
	req := consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: []energy.Window{win(jan15, feb15)}}

	_, err := b.PeriodConsumption(ctx, tenantA.AdminScope, req)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = b.PeriodConsumptionAndRecord(ctx, tenantA.AdminScope, req)
	require.ErrorIs(t, err, store.ErrNotFound)

	rows, err := b.PeriodConsumption(ctx, tenantB.AdminScope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "250", rows[0].Values[energy.ActiveImport].String())
}

// TestPeriodSuspectAnomalyResolvesByRegisteringReset: A2 — a negative delta
// recorded for a 15th-to-15th window re-derives with explicit-window
// semantics when the operator registers the reset.
func TestPeriodSuspectAnomalyResolvesByRegisteringReset(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	loc := istanbulLoc(t)
	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	feb15 := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)
	readingRepo := pathsNewReadingRepo(pool)
	seed := func(ts time.Time, v string) {
		pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, ts, model.ReadingKindLoadProfile, map[string]string{"active_import": v}))
	}
	seed(jan15, "1000")
	seed(feb1, "1400")
	seed(feb15, "190")

	b := anomaliesNewBilling(t, pool, readingRepo, postgres.NewAnomalyRepository(pool), postgres.NewOpsRepository(pool), postgres.NewAnalyzerRepository(pool), lock.NewMemory(nil), feb15.Add(consumption.SettleDelayMonthly))
	req := consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: []energy.Window{win(jan15, feb15)}}

	before, err := b.PeriodConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.Nil(t, before[0].Values[energy.ActiveImport])

	an, err := b.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, an, 1)
	require.Equal(t, string(energy.ReasonNegativeDelta), an[0].Reason)

	resetTS := feb1.Add(24 * time.Hour)
	_, err = b.ResolveAnomaly(ctx, tenant.Scope, an[0].ID, tenant.Users[model.UserRoleCompanyAdmin].ID, consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.NoError(t, err)

	after, err := b.PeriodConsumption(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.Empty(t, after[0].Suspect)
	require.Equal(t, "490", after[0].Values[energy.ActiveImport].String(), "(1400-1000) + (190-100)")
}
