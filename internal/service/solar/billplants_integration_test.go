//go:build integration

package solar_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/solar"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

func billPlantsHarness(t *testing.T) *harness {
	t.Helper()
	h := readHarness(t)
	h.svc = solar.New(solar.Deps{
		Plants: postgres.NewPlantRepository(h.pool), Totals: postgres.NewProductionTotalsRepository(h.pool),
		Solar: postgres.NewSolarTariffRepository(h.pool), Analytics: postgres.NewAnalyticsRepository(h.pool),
		Analyzers: postgres.NewAnalyzerRepository(h.pool), Bills: postgres.NewBillRepository(h.pool), Clock: fixedClock(syncNow),
	})
	return h
}

// R290: one row per plant; the netting analyzer and its bill only when set.
func TestBillDashboardPlantRows(t *testing.T) {
	t.Parallel()
	h := billPlantsHarness(t)
	ctx := context.Background()
	analyzer := h.tenant.Analyzers[0]
	plant := h.plant
	plant.NettingAnalyzerID = &analyzer.ID
	_, err := postgres.NewPlantRepository(h.pool).Update(ctx, h.sc, plant)
	require.NoError(t, err)
	h.seedDays(t, at(1, 0), 10, "5")
	_, err = postgres.NewSolarTariffRepository(h.pool).Create(ctx, h.sc, model.SolarTariff{CompanyID: h.tenant.Company.ID, PlantID: h.plant.ID,
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, ist), FeedInTariff: decimal.RequireFromString("2.0"), Currency: model.CurrencyTRY})
	require.NoError(t, err)
	price := decimal.RequireFromString("3.25")
	bill, err := postgres.NewBillRepository(h.pool).Create(ctx, h.sc, model.Bill{
		CompanyID: h.tenant.Company.ID, BuildingID: analyzer.BuildingID, AnalyzerID: &analyzer.ID, Scope: model.BillScopeAnalyzer,
		PeriodKey: "2026-03", PeriodStart: at(1, 0), PeriodEnd: time.Date(2026, 4, 1, 0, 0, 0, 0, ist), DaysInPeriod: 31,
		Currency: model.CurrencyTRY, Status: model.BillStatusIssued, GenerationUsage: model.GenerationUsageNone,
		EffectiveEnergyPrice: &price, ComputedAt: syncNow, IndexStart: []byte(`{}`), IndexEnd: []byte(`{}`),
	}, nil, nil)
	require.NoError(t, err)

	got, err := h.svc.BillPlants(ctx, h.sc, 2026, 3)
	require.NoError(t, err)
	require.True(t, got.Available)
	found := 0
	for _, r := range got.Rows {
		switch r.PlantID {
		case h.plant.ID:
			found++
			require.Equal(t, analyzer.InstallationNumber, *r.InstallationNumber)
			requireDec(t, "50", r.ProductionKwh, "March production")
			requireDec(t, "2.0", r.ProductionPrice, "feed-in on the 1st")
			requireDec(t, "3.25", r.ConsumptionPrice, "the netting analyzer's bill")
			requireDec(t, "100.00", r.InvoiceAmount, "50 × 2.0")
			require.Equal(t, bill.ID, *r.BillID)
		case h.tenant.Plants[0].ID:
			found++
			require.Nil(t, r.AnalyzerName)
			require.Nil(t, r.ProductionKwh, "no production is missing, never zero")
			require.Nil(t, r.InvoiceAmount)
		}
	}
	require.Equal(t, 2, found)
	requireDec(t, "50", got.TotalProductionKwh, "sum of known production")
	require.Len(t, got.TotalInvoice, 1)
	require.Equal(t, "100.00", got.TotalInvoice[0].Amount.StringFixed(2))
}

func TestBillDashboardPlantsHiddenForBA(t *testing.T) {
	t.Parallel()
	h := billPlantsHarness(t)
	got, err := h.svc.BillPlants(context.Background(), h.tenant.Scope, 2026, 3)
	require.NoError(t, err)
	require.False(t, got.Available)
	require.Equal(t, "plants_not_in_scope", got.Reason)
	require.Empty(t, got.Rows)
}
