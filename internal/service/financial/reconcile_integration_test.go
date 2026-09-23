//go:build integration

package financial_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/financial"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var ist = func() *time.Location { l, _ := time.LoadLocation("Europe/Istanbul"); return l }()

// TestFinancialReconcilesFixture is 09 §F9's acceptance: every figure equals
// the fixture's invoices and production, month by month and for the year.
func TestFinancialReconcilesFixture(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	sc := tenant.AdminScope
	bills := postgres.NewBillRepository(pool)
	type fx struct{ net, cost string }
	fixtureBills := map[int][]fx{1: {{"1000", "3000"}, {"500", "1600"}}, 2: {{"800", "2500"}, {"400", "1250"}}, 3: {{"900", "2800"}}}
	for m, list := range fixtureBills {
		for i, b := range list {
			building := tenant.Buildings[i]
			_, err := bills.Create(ctx, sc, model.Bill{CompanyID: tenant.Company.ID, BuildingID: &building.ID, Scope: model.BillScopeBuilding,
				PeriodKey: fmt.Sprintf("2026-%02d", m), PeriodStart: time.Date(2026, time.Month(m), 1, 0, 0, 0, 0, ist),
				PeriodEnd: time.Date(2026, time.Month(m)+1, 1, 0, 0, 0, 0, ist), DaysInPeriod: 30, Currency: model.CurrencyTRY,
				Status: model.BillStatusIssued, GenerationUsage: model.GenerationUsageNone, NetConsumption: dec(b.net), TotalCost: dec(b.cost),
				ComputedAt: time.Now(), IndexStart: []byte(`{}`), IndexEnd: []byte(`{}`)}, nil, nil)
			require.NoError(t, err)
		}
	}
	// A superseded bill must not count.
	building := tenant.Buildings[0]
	_, err := bills.Create(ctx, sc, model.Bill{CompanyID: tenant.Company.ID, BuildingID: &building.ID, Scope: model.BillScopeBuilding,
		PeriodKey: "2026-01", PeriodStart: time.Date(2026, 1, 1, 0, 0, 0, 0, ist), PeriodEnd: time.Date(2026, 2, 1, 0, 0, 0, 0, ist),
		DaysInPeriod: 31, Currency: model.CurrencyTRY, Status: model.BillStatusSuperseded, GenerationUsage: model.GenerationUsageNone,
		NetConsumption: dec("99999"), TotalCost: dec("99999"), ComputedAt: time.Now(), IndexStart: []byte(`{}`), IndexEnd: []byte(`{}`)}, nil, nil)
	require.NoError(t, err)

	totals := postgres.NewProductionTotalsRepository(pool)
	day := func(p model.PowerPlant, m, d int, kwh string) {
		v := dec(kwh)
		at := time.Date(2026, time.Month(m), d, 0, 0, 0, 0, ist)
		_, err := totals.ReplaceDays(ctx, sc, p.ID, []time.Time{at}, []model.PlantProductionTotal{{PlantID: p.ID, Ts: at, ProductionKwh: &v, Basis: model.BasisDailyTotal}})
		require.NoError(t, err)
	}
	p1, p2 := tenant.Plants[0], tenant.Plants[1]
	day(p1, 1, 10, "700")
	day(p1, 1, 20, "500")
	day(p1, 2, 5, "300")
	day(p2, 3, 3, "100")
	solar := postgres.NewSolarTariffRepository(pool)
	for _, tr := range []struct {
		plant model.PowerPlant
		from  time.Time
		price string
	}{{p1, time.Date(2025, 1, 1, 0, 0, 0, 0, ist), "2.0"}, {p1, time.Date(2026, 2, 1, 0, 0, 0, 0, ist), "2.5"}} {
		_, err := solar.Create(ctx, sc, model.SolarTariff{CompanyID: tenant.Company.ID, PlantID: tr.plant.ID, EffectiveFrom: tr.from,
			FeedInTariff: dec(tr.price), Currency: model.CurrencyTRY})
		require.NoError(t, err)
	}

	svc := financial.New(financial.Deps{Bills: bills, Plants: postgres.NewPlantRepository(pool), Analytics: postgres.NewAnalyticsRepository(pool),
		Solar: solar, Analyzers: postgres.NewAnalyzerRepository(pool), Buildings: postgres.NewBuildingRepository(pool),
		Tariffs: postgres.NewTariffRepository(pool), Clock: clock.NewFake(time.Date(2026, 4, 2, 12, 0, 0, 0, ist))})
	y, err := svc.Year(ctx, sc, 2026)
	require.NoError(t, err)

	for m, want := range map[int]struct{ cons, cost, prod, revenue, net string }{
		1: {"1500", "4600.00", "1200", "2400.00", "2200.00"},
		2: {"1200", "3750.00", "300", "750.00", "3000.00"},
		3: {"900", "2800.00", "100", "", "2800.00"},
	} {
		row := y.Months[m-1]
		requireDec(t, want.cons, row.ConsumptionKwh, fmt.Sprintf("month %d consumption", m))
		require.Equal(t, want.cost, amount(t, row.Cost, model.CurrencyTRY), "month %d cost", m)
		requireDec(t, want.prod, row.ProductionKwh, fmt.Sprintf("month %d production", m))
		require.Equal(t, want.revenue, amount(t, row.Revenue, model.CurrencyTRY), "month %d revenue", m)
		require.Equal(t, want.net, amount(t, row.Net, model.CurrencyTRY), "month %d net", m)
	}
	require.True(t, y.Months[2].RevenuePartial, "p2 has no tariff")
	requireDec(t, "3600", y.Total.ConsumptionKwh, "the year")
	require.Equal(t, "11150.00", amount(t, y.Total.Cost, model.CurrencyTRY))
	requireDec(t, "-300", y.Months[0].OffsetKwh, "January offset")
	requireDec(t, "300", y.Months[0].GridPurchaseKwh, "January purchase")
	requireDec(t, "0", y.Months[0].GridSaleKwh, "January sale")
	require.Nil(t, y.Months[3].ConsumptionKwh, "April has no bills")
	require.Nil(t, y.Months[3].OffsetKwh)
	requireDec(t, "1600", y.Total.ProductionKwh, "the year's production")
	require.Equal(t, "3150.00", amount(t, y.Total.Revenue, model.CurrencyTRY))
	require.Equal(t, "8000.00", amount(t, y.Total.Net, model.CurrencyTRY), "year net = Σ months")
	require.Equal(t, 3, y.ConsumptionMonths)

	_, err = svc.Year(ctx, tenant.Scope, 2026)
	require.ErrorIs(t, err, store.ErrNotFound, "financial analysis is company-level")
}

func TestFinancialSummaryCountsAndTariffs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	svc := financial.New(financial.Deps{Bills: postgres.NewBillRepository(pool), Plants: postgres.NewPlantRepository(pool),
		Analytics: postgres.NewAnalyticsRepository(pool), Solar: postgres.NewSolarTariffRepository(pool),
		Analyzers: postgres.NewAnalyzerRepository(pool), Buildings: postgres.NewBuildingRepository(pool),
		Tariffs: postgres.NewTariffRepository(pool), Clock: clock.NewFake(time.Date(2026, 4, 2, 12, 0, 0, 0, ist))})
	month := 3
	s, err := svc.Summary(ctx, tenant.AdminScope, 2026, &month)
	require.NoError(t, err)
	require.Equal(t, len(tenant.Analyzers), s.AnalyzerCount)
	require.Equal(t, len(tenant.Plants), s.PlantCount)
	require.Equal(t, 3, s.Period.Month)
	require.True(t, s.Tariffs.SaleMissing, "no plant has a feed-in tariff")
	require.NotEmpty(t, s.Tariffs.Purchase, "the fixture's buildings carry tariffs")
}
