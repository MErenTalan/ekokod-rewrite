//go:build integration

package report_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/assets"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var ist = func() *time.Location { l, _ := time.LoadLocation("Europe/Istanbul"); return l }()

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func realService(t *testing.T, pool *pgxpool.Pool) *report.Service {
	t.Helper()
	analytics, err := consumption.NewAnalytics(consumption.AnalyticsDeps{
		Analytics: postgres.NewAnalyticsRepository(pool), Log: testfixtures.DiscardLogger()})
	require.NoError(t, err)
	s, err := report.New(report.Deps{
		Buildings: postgres.NewBuildingRepository(pool), Analyzers: postgres.NewAnalyzerRepository(pool),
		Bills: postgres.NewBillRepository(pool), Plants: postgres.NewPlantRepository(pool),
		Solar: postgres.NewSolarTariffRepository(pool), Tariffs: postgres.NewTariffRepository(pool),
		Carbon: postgres.NewCarbonRepository(pool), Analytics: postgres.NewAnalyticsRepository(pool),
		Consumption: analytics, Clock: clock.NewFake(time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
	return s
}

func buildingBill(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sc store.Scope, building uuid.UUID, period, total, energy, net string) {
	t.Helper()
	start, err := time.ParseInLocation("2006-01", period, ist)
	require.NoError(t, err)
	end := start.AddDate(0, 1, 0)
	price := dec(energy).DivRound(dec(net), 6)
	b := model.Bill{
		CompanyID: sc.CompanyID, BuildingID: &building, Scope: model.BillScopeBuilding, PeriodKey: period,
		PeriodStart: start, PeriodEnd: end, DaysInPeriod: int32(end.Sub(start).Hours() / 24),
		Currency: model.CurrencyTRY, Status: model.BillStatusIssued, GenerationUsage: model.GenerationUsageNone,
		ActiveImport: dec(net), NetConsumption: dec(net), EnergyCost: dec(energy), TotalCost: dec(total),
		EffectiveEnergyPrice: &price, ComputedAt: time.Now(), IndexStart: json.RawMessage(`{}`), IndexEnd: json.RawMessage(`{}`),
	}
	_, err = postgres.NewBillRepository(pool).Create(ctx, sc, b, []model.BillLine{{Code: model.BillLineEnergy, Label: "Enerji", Amount: dec(energy)}}, nil)
	require.NoError(t, err)
}

// reportFixture seeds March 2026 for a tenant:
//   - Buildings[0]/Analyzers[0]: 2 kWh/h active, 0.5 kWh/h export; Buildings[1]/Analyzers[2]: 1 kWh/h.
//     consumption_monthly is last − first reading of the month: 743 steps in
//     March's 744 hourly readings (04 §4.3's understatement), so 1486 and 743 kWh;
//     inductive 20 %, capacitive 5 % of each step; export 371.5 kWh.
//   - building bills 2026-03: ₺5000 (energy 3000, net 1400) and ₺2500 (energy 1500, net 700);
//     2025-03 for Buildings[0]: ₺4000.
//   - one grid plant, yearly target 1000 kWh, three 100 kWh samples in March, feed-in ₺2.0.
//   - the company's grid factor 0.5 kg/kWh, source year 2022.
func reportFixture(t *testing.T) (*report.Service, testfixtures.Tenant, uuid.UUID, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 500)
	sc := tenant.AdminScope
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, ist)
	to := from.AddDate(0, 1, 0)

	rows := testfixtures.HourlyReadings(tenant.Analyzers[0].ID, from, to, testfixtures.Constant("2"))
	for i := range rows {
		e := decimal.NewFromInt(int64(i)).Mul(dec("0.5"))
		rows[i].ActiveExport = &e
	}
	rows = append(rows, testfixtures.HourlyReadings(tenant.Analyzers[2].ID, from, to, testfixtures.Constant("1"))...)
	testfixtures.InsertReadings(t, ctx, pool, tenant.Company.ID, rows)

	buildingBill(t, ctx, pool, sc, tenant.Buildings[0].ID, "2026-03", "5000", "3000", "1400")
	buildingBill(t, ctx, pool, sc, tenant.Buildings[1].ID, "2026-03", "2500", "1500", "700")
	buildingBill(t, ctx, pool, sc, tenant.Buildings[0].ID, "2025-03", "4000", "2400", "1200")

	target := dec("1000")
	plants := postgres.NewPlantRepository(pool)
	plant, err := plants.Create(ctx, sc, model.PowerPlant{CompanyID: tenant.Company.ID, Name: "Arazi GES", PlantKind: "grid", YearlyTargetKwh: &target})
	require.NoError(t, err)
	device, err := plants.UpsertDevice(ctx, sc, model.PlantDevice{PlantID: plant.ID, DeviceSN: "INV-1"})
	require.NoError(t, err)
	var prod []model.PlantProduction
	for h := range 3 {
		v := dec("100")
		prod = append(prod, model.PlantProduction{PlantID: plant.ID, DeviceID: device.ID, Ts: from.Add(time.Duration(h+12) * time.Hour).UTC(), ProductionKwh: &v, Source: "isolar"})
	}
	_, _, err = postgres.NewProductionRepository(pool).BulkInsert(ctx, sc, prod)
	require.NoError(t, err)
	_, err = postgres.NewSolarTariffRepository(pool).Create(ctx, sc, model.SolarTariff{CompanyID: tenant.Company.ID, PlantID: plant.ID,
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), FeedInTariff: dec("2.0"), Currency: model.CurrencyTRY})
	require.NoError(t, err)
	year := int16(2022)
	_, err = postgres.NewCarbonRepository(pool).UpsertFactor(ctx, sc, model.EmissionFactor{CompanyID: &tenant.Company.ID,
		Key: assets.GridFactorKey, Label: "Şebeke elektriği", MainCategory: "electricity", BaseFactor: dec("0.5"),
		BaseUnit: "kg CO2e/kWh", SourceYear: &year})
	require.NoError(t, err)

	for _, view := range []string{"consumption_monthly", "plant_production_monthly"} {
		_, err := pool.Exec(ctx, "call refresh_continuous_aggregate('"+view+"', NULL, NULL)")
		require.NoError(t, err)
	}
	return realService(t, pool), tenant, plant.ID, pool
}

func requireDec(t *testing.T, want string, got *decimal.Decimal, msg string) {
	t.Helper()
	require.NotNil(t, got, msg)
	require.True(t, dec(want).Equal(*got), "%s: want %s, got %s", msg, want, got)
}

func TestPreviewMonthlyMatchesHandComputedFixture(t *testing.T) {
	t.Parallel()
	s, tenant, plantID, pool := reportFixture(t)
	p, err := s.Preview(t.Context(), tenant.AdminScope, report.Request{Type: domain.TypeMonthly, Period: "2026-03",
		Selection: domain.SelectionAll, BuildingIDs: []uuid.UUID{tenant.Buildings[0].ID, tenant.Buildings[1].ID}, PlantIDs: []uuid.UUID{plantID}})
	require.NoError(t, err)
	m := p.Monthly
	requireDec(t, "2229", m.Consumption.Value, "1486 + 743")
	requireDec(t, "71.9032", m.DailyConsumption.Value, "2229 / 31")
	requireDec(t, "445.8", m.Inductive.Value, "20 % of 2229")
	requireDec(t, "111.45", m.Capacitive.Value, "5 % of 2229")
	requireDec(t, "20", m.InductiveRatio, "445.8 / 2229")
	requireDec(t, "5", m.CapacitiveRatio, "111.45 / 2229")
	requireDec(t, "371.5", m.Rooftop.Value, "0.5 × 743")
	require.Equal(t, 1, m.Rooftop.WithData, "only Buildings[0] exports")
	requireDec(t, "300", m.Utility.Value, "3 × 100")
	requireDec(t, "671.5", m.Production.Value, "371.5 + 300")
	require.Nil(t, m.ConsumptionPrev.Value, "no readings in March 2025")
	require.Len(t, m.Bill, 1)
	require.True(t, dec("7500").Equal(m.Bill[0].Value))
	require.True(t, dec("4000").Equal(m.BillPrev[0].Value))
	requireDec(t, "87.5", m.BillDelta[0].Pct, "(7500 − 4000) / 4000")
	require.True(t, dec("2.142857").Equal(m.AveragePurchasePrice[0].Value), "4500 / 2100")
	requireDec(t, "2", m.UtilityFeedIn.Min, "the plant's solar tariff")
	// The tenant fixture gives each building a tariff; the name shown is the
	// one in force at the period's last instant.
	tariff, err := postgres.NewTariffRepository(pool).Effective(t.Context(), tenant.AdminScope, tenant.Buildings[0].ID, time.Date(2026, 4, 1, 0, 0, 0, 0, ist).Add(-time.Nanosecond))
	require.NoError(t, err)
	require.Equal(t, tariff.Name, m.Buildings[0].TariffName)
	requireDec(t, "2229", m.ConsumptionChart[2].Current, "chart march")
}

func TestPreviewYearlyMatchesHandComputedFixture(t *testing.T) {
	t.Parallel()
	s, tenant, plantID, _ := reportFixture(t)
	// The tenant fixture owns plants of its own; naming ours keeps the target hand-computable.
	p, err := s.Preview(t.Context(), tenant.AdminScope, report.Request{Type: domain.TypeYearly, Period: "2026",
		Selection: domain.SelectionAll, BuildingIDs: []uuid.UUID{tenant.Buildings[0].ID, tenant.Buildings[1].ID}, PlantIDs: []uuid.UUID{plantID}})
	require.NoError(t, err)
	y := p.Yearly
	requireDec(t, "2229", y.Consumption.Value, "only March has readings")
	requireDec(t, "671.5", y.Production.Value, "rooftop + utility")
	requireDec(t, "1000", y.Target, "the plant's yearly target")
	requireDec(t, "30", y.AchievementPct, "300 / 1000")
	requireDec(t, "30.1", y.SolarSharePct, "671.5 / 2229 = 30.12…")
	require.True(t, dec("7500").Equal(y.Bill[0].Value))
	require.NotNil(t, y.Carbon)
	requireDec(t, "1.115", y.Carbon.ConsumptionT, "2229 × 0.5 / 1000 = 1.1145")
	requireDec(t, "0.336", y.Carbon.ReductionT, "671.5 × 0.5 / 1000 = 0.33575")
	requireDec(t, "0.779", y.Carbon.NetT, "1.115 − 0.336")
	require.Equal(t, 2022, *y.Carbon.SourceYear)
	require.Len(t, y.History, 3)
	require.True(t, dec("4000").Equal(y.History[1].Bill[0].Value), "2025's bill")
}

func TestPreviewScopeHidesOtherBuilding(t *testing.T) {
	t.Parallel()
	s, tenant, _, _ := reportFixture(t)
	_, err := s.Preview(t.Context(), tenant.Scope, report.Request{Type: domain.TypeMonthly, Period: "2026-03",
		Selection: domain.SelectionAll, BuildingIDs: []uuid.UUID{tenant.Buildings[1].ID}})
	require.ErrorIs(t, err, store.ErrNotFound, "Scope covers Buildings[0] only")
}
