package renewable_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/renewable"
)

func requireDec(t *testing.T, want string, got *decimal.Decimal, msg string) {
	t.Helper()
	require.NotNil(t, got, msg)
	require.True(t, decimal.RequireFromString(want).Equal(*got), "%s: want %s got %s", msg, want, got)
}

func TestRealtimeFromLastCompleteHour(t *testing.T) {
	rt := renewable.BuildRealtime(fixture("1", "0", 36))
	requireDec(t, "5", rt.CurrentPowerKw, "11:00–12:00 generated 5 kWh")
	requireDec(t, "14", rt.TodayKwh, "2+3+4+5 since midnight")
	require.Equal(t, "producing", *rt.Status)
	require.Len(t, rt.Series24h, 24)
	requireDec(t, "7", rt.MaxPowerKw, "the day's peak hour (13:00, 13 % 7 + 1)")
	require.Nil(t, rt.SystemEfficiencyPct)
	require.NotEmpty(t, rt.Unavailable["system_efficiency_pct"])
}

func TestPowerFactorFromTodaysBuckets(t *testing.T) {
	in := renewable.Inputs{Now: now, From: now.AddDate(0, 0, -1), To: now, Hourly: []model.ConsumptionBucket{
		{AnalyzerID: analyzer, Bucket: now.Truncate(time.Hour).Add(-time.Hour), ActiveConsumption: dp("30"), InductiveConsumption: dp("40"), ActiveGeneration: dp("0")},
	}}
	g := renewable.BuildGridInteraction(in)
	requireDec(t, "0.600", g.PowerFactor, "30 / sqrt(30² + 40²)")
	require.Equal(t, "import", *g.Direction)
	require.Nil(t, g.VoltageV)
	require.NotEmpty(t, g.Unavailable["voltage_v"])
	require.NotEmpty(t, g.Unavailable["import_price"], "no bill, no price")
}

func TestAccuracyIsHundredMinusMAPE(t *testing.T) {
	f := renewable.BuildForecast(fixture("1", "0", 36))
	requireDec(t, "96.67", f.AccuracyDailyPct, "|2.9 − 3| / 3 = 3.33 %")
	requireDec(t, "69.6", f.ConsumptionNext24hKwh, "24 × 2.9")
	require.Nil(t, f.EstimatedGenerationKwh)
	require.NotEmpty(t, f.Unavailable["estimated_generation_kwh"])
}

func TestForecastWithoutRunsIsUnavailable(t *testing.T) {
	in := fixture("1", "0", 36)
	in.Forecasts = nil
	f := renewable.BuildForecast(in)
	require.Nil(t, f.ConsumptionNext24hKwh)
	require.Equal(t, "no_forecasts", f.Unavailable["consumption_next_24h_kwh"])
}

func TestEnvironmentalUsesTheGridFactor(t *testing.T) {
	e := renewable.BuildEnvironmental(fixture("1", "0", 36))
	requireDec(t, "252", e.GenerationKwh, "Σ 20..36 step 2 over nine days")
	requireDec(t, "110.88", e.Co2AvoidedKg, "252 × 0.44, never legacy's 0.997")
	requireDec(t, "5.09", e.Trees, "110.88 / 21.77")
	in := fixture("1", "0", 36)
	delete(in.Equivalences, renewable.EquivCar)
	e = renewable.BuildEnvironmental(in)
	require.Nil(t, e.CarKm)
	require.Equal(t, "no_factor", e.Unavailable["car_km"])
}

func TestSystemStatusWorstOfAvailable(t *testing.T) {
	in := fixture("1", "0", 36)
	old := now.Add(-30 * time.Hour)
	in.LastReadingAt = &old
	s := renewable.BuildSystemStatus(in)
	require.Equal(t, "attention", *s.Monitoring)
	require.Equal(t, "healthy", *s.GridConnection)
	require.Equal(t, "attention", *s.Overall, "the worst of the components that report")
	require.Nil(t, s.Inverter)
}

func TestEarningsFromTheBillsGenerationCredit(t *testing.T) {
	a := renewable.BuildAnalytics(fixture("1", "0", 36))
	requireDec(t, "120", a.Financial.MonthEarnings, "March's bill")
	requireDec(t, "430", a.Financial.YearEarnings, "February and March")
	requireDec(t, "2.5", a.Financial.ImportPrice, "the latest bill's energy price")
	require.Equal(t, "TRY", *a.Financial.Currency)
	require.Nil(t, a.Financial.RoiPct)
	require.Equal(t, "no_investment_cost", a.Financial.Unavailable["roi_pct"])
	require.Equal(t, 13, *a.PeakHour, "13:00 has the highest mean generation")
}
