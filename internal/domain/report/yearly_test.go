package report_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
)

func every(v string) report.MonthSeries {
	var s report.MonthSeries
	for i := range s {
		s[i] = d(v)
	}
	return s
}

func everyBill(total, reactive string) [12]*report.Bill {
	var s [12]*report.Bill
	for i := range s {
		s[i] = tryBill(total, total, "0", "0", reactive, "1", "1", "1")
	}
	return s
}

func twelve(v string) []decimal.Decimal {
	out := make([]decimal.Decimal, 12)
	for i := range out {
		out[i] = decimal.RequireFromString(v)
	}
	return out
}

// yearlyFixture is 2025 (365 days): one building at 1 000 kWh and ₺3 000 a
// month, two utility-scale plants. The hand-computed values are in the test.
func yearlyFixture() report.YearlyInput {
	year := 2021
	return report.YearlyInput{
		Year: 2025, Selection: report.SelectionAll,
		Buildings: []report.BuildingInput{{
			ID: b1, Name: "B1",
			Consumption: map[int]report.MonthSeries{2025: every("1000"), 2024: every("800")},
			Export:      map[int]report.MonthSeries{2025: every("100")},
			Bills:       map[int][12]*report.Bill{2025: everyBill("3000", "10"), 2024: everyBill("2500", "0")},
		}},
		Plants: []report.PlantInput{
			{ID: p1, Name: "Arazi 1", Kind: report.KindGrid, YearlyTarget: d("8000"),
				Production: map[int]report.MonthSeries{2025: every("500")}},
			{ID: p2, Name: "Arazi 2", Kind: report.KindGrid, MonthlyTargets: twelve("150"),
				Production: map[int]report.MonthSeries{2025: every("100")}},
		},
		Factor: &report.GridFactor{Value: decimal.RequireFromString("0.45"), Unit: "kg CO2e/kWh", SourceYear: &year},
	}
}

func TestYearlyHandComputedFixture(t *testing.T) {
	t.Parallel()
	y := report.BuildYearly(yearlyFixture())

	require.Len(t, y.Months, 12)
	eq(t, "1000", y.Months[0].Consumption.Value, "january")
	eq(t, "100", y.Months[11].Rooftop.Value, "december rooftop")
	require.True(t, decimal.RequireFromString("3000").Equal(y.Months[5].Bill[0].Value))
	require.True(t, decimal.RequireFromString("10").Equal(y.Months[5].ReactivePenalty[0].Value))

	eq(t, "12000", y.Consumption.Value, "12 × 1000")
	eq(t, "1200", y.Rooftop.Value, "12 × 100")
	eq(t, "7200", y.Utility.Value, "12 × (500 + 100)")
	eq(t, "8400", y.Production.Value, "1200 + 7200")
	require.True(t, decimal.RequireFromString("36000").Equal(y.Bill[0].Value))
	require.True(t, decimal.RequireFromString("120").Equal(y.ReactivePenalty[0].Value))
	// ÷ 365: 32.876712… → 32.8767; 3.287671… → 3.2877; 23.013698… → 23.0137
	eq(t, "32.8767", y.DailyConsumption.Value, "daily consumption")
	eq(t, "3.2877", y.DailyRooftop.Value, "daily rooftop")
	eq(t, "23.0137", y.DailyProduction.Value, "daily production")
	// 7200 / 365 = 19.726027… → 19.7260
	eq(t, "19.726", y.DailyUtility.Value, "daily utility-scale production")

	// targets: 8000 (yearly) + 12 × 150 (monthly) = 9800; actual 6000 + 1200
	// = 7200 → 73.469… → 73.5 %. Per plant: 75 % and 66.7 %.
	eq(t, "9800", y.Target, "target")
	eq(t, "73.5", y.AchievementPct, "achievement")
	eq(t, "75", y.Plants[0].AchievementPct, "plant 1")
	eq(t, "66.7", y.Plants[1].AchievementPct, "plant 2")
	eq(t, "1800", y.Plants[1].Target, "plant 2 target from its months")

	// (12000 − 9600) / 9600 = 25 %; (36000 − 30000) / 30000 = 20 %
	eq(t, "25", y.ConsumptionDelta.Pct, "consumption delta")
	eq(t, "20", y.BillDelta[0].Pct, "bill delta")
	// min(8400, 12000) / 12000 = 70 %
	eq(t, "70", y.SolarSharePct, "solar share")
	eq(t, "30", y.GridSharePct, "grid share")

	// 12000 × 0.45 / 1000 = 5.4 t; 8400 × 0.45 / 1000 = 3.78 t; net 1.62 t
	require.NotNil(t, y.Carbon)
	eq(t, "5.4", y.Carbon.ConsumptionT, "consumption emission")
	eq(t, "3.78", y.Carbon.ReductionT, "reduction")
	eq(t, "1.62", y.Carbon.NetT, "net")
	require.Equal(t, 2021, *y.Carbon.SourceYear)
	require.Equal(t, "kg CO2e/kWh", y.Carbon.FactorUnit)
}

func TestPlantTargetPrefersYearly(t *testing.T) {
	t.Parallel()
	p := report.PlantInput{YearlyTarget: d("5000"), MonthlyTargets: twelve("100")}
	eq(t, "5000", report.PlantTarget(p), "yearly wins")
}

func TestPlantTargetSumsTwelveMonths(t *testing.T) {
	t.Parallel()
	eq(t, "1200", report.PlantTarget(report.PlantInput{MonthlyTargets: twelve("100")}), "12 × 100")
	eq(t, "1200", report.PlantTarget(report.PlantInput{YearlyTarget: d("0"), MonthlyTargets: twelve("100")}), "a zero yearly target is not set")
}

func TestPlantTargetNeedsAllTwelve(t *testing.T) {
	t.Parallel()
	require.Nil(t, report.PlantTarget(report.PlantInput{MonthlyTargets: twelve("100")[:11]}), "eleven months are not a year (E-1)")
	require.Nil(t, report.PlantTarget(report.PlantInput{}), "never capacity × 1500")
}

func TestPlantTargetZeroIsNoTarget(t *testing.T) {
	t.Parallel()
	require.Nil(t, report.PlantTarget(report.PlantInput{MonthlyTargets: twelve("0")}))
}

func TestAchievementIgnoresPlantsWithoutTarget(t *testing.T) {
	t.Parallel()
	in := yearlyFixture()
	in.Plants[1].MonthlyTargets = nil // Arazi 2 has production but no target
	y := report.BuildYearly(in)
	eq(t, "8000", y.Target, "only plants with a target")
	eq(t, "75", y.AchievementPct, "6000 / 8000: Arazi 2's 1200 kWh does not dilute the rate")
	require.Nil(t, y.Plants[1].AchievementPct)

	in.Plants[0].YearlyTarget = nil
	y = report.BuildYearly(in)
	require.Nil(t, y.Target)
	require.Nil(t, y.AchievementPct)
}

func TestSolarShareCapsAtHundred(t *testing.T) {
	t.Parallel()
	in := yearlyFixture()
	in.Buildings[0].Consumption = map[int]report.MonthSeries{2025: every("100")} // 1200 < 8400
	y := report.BuildYearly(in)
	eq(t, "100", y.SolarSharePct, "min(production, consumption)")
	eq(t, "0", y.GridSharePct, "")

	in.Buildings[0].Consumption = map[int]report.MonthSeries{2025: every("0")}
	y = report.BuildYearly(in)
	require.Nil(t, y.SolarSharePct, "zero consumption has no share")
	require.Nil(t, y.GridSharePct)
}

func TestCarbonNetIsNotClamped(t *testing.T) {
	t.Parallel()
	in := yearlyFixture()
	in.Buildings[0].Consumption = map[int]report.MonthSeries{2025: every("100")}
	y := report.BuildYearly(in)
	// 1200 × 0.45 / 1000 = 0.54; 8400 × 0.45 / 1000 = 3.78; net −3.24
	eq(t, "-3.24", y.Carbon.NetT, "a net exporter shows a negative net (R262)")
}

func TestCarbonUnavailableWithoutFactor(t *testing.T) {
	t.Parallel()
	in := yearlyFixture()
	in.Factor = nil
	y := report.BuildYearly(in)
	require.Nil(t, y.Carbon)
	require.Equal(t, "grid_factor_missing", y.CarbonReason)
}

func TestYearlyHistoryCoversThreeYears(t *testing.T) {
	t.Parallel()
	y := report.BuildYearly(yearlyFixture())
	require.Len(t, y.History, 3)
	require.Equal(t, []int{2023, 2024, 2025}, []int{y.History[0].Year, y.History[1].Year, y.History[2].Year})
	require.Nil(t, y.History[0].Consumption, "2023 has no data: null")
	eq(t, "9600", y.History[1].Consumption, "2024")
	eq(t, "12000", y.History[2].Consumption, "2025")
	eq(t, "8400", y.History[2].Production, "2025 production")
	require.True(t, decimal.RequireFromString("30000").Equal(y.History[1].Bill[0].Value))
	require.Equal(t, "TRY", *y.ChartCurrency)
}

func TestYearlyPlantSelectionRooftopDropsUtility(t *testing.T) {
	t.Parallel()
	in := yearlyFixture()
	in.Selection = report.SelectionRooftop
	y := report.BuildYearly(in)
	require.True(t, y.Utility.Excluded)
	eq(t, "1200", y.Production.Value, "rooftop only")
	require.Nil(t, y.Target, "no utility-scale plant counts, so no target")
	_ = uuid.Nil
}
