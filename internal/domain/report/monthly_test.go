package report_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
)

func d(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

// eq compares a nullable figure to a decimal string ("" = nil).
func eq(t *testing.T, want string, got *decimal.Decimal, msg string) {
	t.Helper()
	if want == "" {
		require.Nil(t, got, msg)
		return
	}
	require.NotNil(t, got, msg)
	require.True(t, decimal.RequireFromString(want).Equal(*got), "%s: want %s, got %s", msg, want, got)
}

func series(month int, v *decimal.Decimal) report.MonthSeries {
	var s report.MonthSeries
	s[month-1] = v
	return s
}

func bills(month int, b *report.Bill) [12]*report.Bill {
	var s [12]*report.Bill
	s[month-1] = b
	return s
}

func tryBill(total, energy, dist, taxes, reactive, net, eff, gen string) *report.Bill {
	return &report.Bill{Currency: "TRY", Total: *d(total), EnergyCost: *d(energy), DistributionCost: *d(dist),
		Taxes: *d(taxes), ReactivePenalty: *d(reactive), NetConsumption: *d(net), EffectivePrice: d(eff), GenerationPrice: d(gen)}
}

var (
	b1 = uuid.MustParse("00000000-0000-0000-0000-0000000000b1")
	b2 = uuid.MustParse("00000000-0000-0000-0000-0000000000b2")
	p1 = uuid.MustParse("00000000-0000-0000-0000-0000000000a1")
	p2 = uuid.MustParse("00000000-0000-0000-0000-0000000000a2")
)

// fixture is March 2026 (31 days) for two buildings, one utility-scale plant
// and one rooftop plant. Every expected value in the hand-computed test below
// is worked out in its comment.
func fixture() report.MonthlyInput {
	tariff := "Sanayi OG"
	return report.MonthlyInput{
		Year: 2026, Month: 3, Selection: report.SelectionAll,
		Buildings: []report.BuildingInput{
			{
				ID: b1, Name: "B1", TariffName: &tariff,
				Consumption: map[int]report.MonthSeries{2026: series(3, d("1000")), 2025: series(3, d("800"))},
				Export:      map[int]report.MonthSeries{2026: series(3, d("100")), 2025: series(3, d("50"))},
				T1:          d("500"), T2: d("300"), T3: d("200"), Inductive: d("150"), Capacitive: d("20"),
				Bills: map[int][12]*report.Bill{
					2026: bills(3, tryBill("5000", "3000", "1000", "1000", "100", "900", "3.3333", "1.2")),
					2025: bills(3, tryBill("4000", "2400", "800", "800", "50", "800", "3", "1.2")),
				},
			},
			{
				ID: b2, Name: "B2",
				Consumption: map[int]report.MonthSeries{2026: series(3, d("500"))},
				T1:          d("250"), T2: d("150"), T3: d("100"), Inductive: d("50"), Capacitive: d("10"),
				Bills: map[int][12]*report.Bill{
					2026: bills(3, tryBill("2500", "1500", "500", "500", "0", "500", "3", "1.2")),
				},
			},
		},
		Plants: []report.PlantInput{
			{ID: p1, Name: "Arazi GES", Kind: report.KindGrid, FeedIn: d("2.0"),
				Production: map[int]report.MonthSeries{2026: series(3, d("2000")), 2025: series(3, d("1500"))}},
			{ID: p2, Name: "Çatı GES", Kind: report.KindRooftop,
				Production: map[int]report.MonthSeries{2026: series(3, d("700"))}},
		},
	}
}

func TestMonthlyHandComputedFixture(t *testing.T) {
	t.Parallel()
	m := report.BuildMonthly(fixture())

	require.Equal(t, 31, m.DaysInMonth)
	// consumption 1000 + 500 = 1500 (2 of 2); previous year only B1: 800 (1 of 2)
	eq(t, "1500", m.Consumption.Value, "consumption")
	require.Equal(t, [2]int{2, 2}, [2]int{m.Consumption.WithData, m.Consumption.Of})
	eq(t, "800", m.ConsumptionPrev.Value, "consumption prev")
	require.Equal(t, [2]int{1, 2}, [2]int{m.ConsumptionPrev.WithData, m.ConsumptionPrev.Of})
	// 1500 / 31 = 48.38709… → 48.3871
	eq(t, "48.3871", m.DailyConsumption.Value, "daily consumption")
	eq(t, "750", m.T1.Value, "t1")
	eq(t, "450", m.T2.Value, "t2")
	eq(t, "300", m.T3.Value, "t3")
	eq(t, "200", m.Inductive.Value, "inductive")
	eq(t, "30", m.Capacitive.Value, "capacitive")
	// 200 / 1500 × 100 = 13.33; 30 / 1500 × 100 = 2
	eq(t, "13.33", m.InductiveRatio, "inductive ratio")
	eq(t, "2", m.CapacitiveRatio, "capacitive ratio")

	// rooftop = B1's metered export only; utility = the grid plant only (the
	// rooftop-kind plant's own production is not building-attributable, R256)
	eq(t, "100", m.Rooftop.Value, "rooftop")
	eq(t, "50", m.RooftopPrev.Value, "rooftop prev")
	eq(t, "2000", m.Utility.Value, "utility")
	eq(t, "1500", m.UtilityPrev.Value, "utility prev")
	eq(t, "2100", m.Production.Value, "production")
	eq(t, "1550", m.ProductionPrev.Value, "production prev")
	// 2100 / 31 = 67.74193… → 67.7419
	eq(t, "67.7419", m.DailyProduction.Value, "daily production")

	// deltas: (1500−800)/800 = 87.5 %; (2100−1550)/1550 = 35.48… → 35.5 %
	eq(t, "87.5", m.ConsumptionDelta.Pct, "consumption delta")
	eq(t, "35.5", m.ProductionDelta.Pct, "production delta")

	require.Len(t, m.Bill, 1)
	require.Equal(t, "TRY", m.Bill[0].Currency)
	require.True(t, decimal.RequireFromString("7500").Equal(m.Bill[0].Value))
	require.Equal(t, [2]int{2, 2}, [2]int{m.Bill[0].WithData, m.Bill[0].Of})
	require.True(t, decimal.RequireFromString("4000").Equal(m.BillPrev[0].Value))
	require.True(t, decimal.RequireFromString("4500").Equal(m.EnergyCost[0].Value))
	require.True(t, decimal.RequireFromString("1500").Equal(m.DistributionCost[0].Value))
	require.True(t, decimal.RequireFromString("1500").Equal(m.Taxes[0].Value))
	require.True(t, decimal.RequireFromString("100").Equal(m.ReactivePenalty[0].Value))
	// (7500−4000)/4000 = 87.5 %; reactive (100−50)/50 = 100 %
	eq(t, "87.5", m.BillDelta[0].Pct, "bill delta")
	eq(t, "100", m.ReactivePenaltyDelta[0].Pct, "reactive delta")
	// Σ energy / Σ net = 4500 / 1400 = 3.2142857… → 3.214286
	require.Len(t, m.AveragePurchasePrice, 1)
	require.True(t, decimal.RequireFromString("3.214286").Equal(m.AveragePurchasePrice[0].Value))

	eq(t, "1.2", m.RooftopFeedIn.Min, "rooftop feed-in")
	eq(t, "1.2", m.RooftopFeedIn.Max, "rooftop feed-in")
	eq(t, "2", m.UtilityFeedIn.Min, "utility feed-in")

	require.Len(t, m.Buildings, 2)
	require.Equal(t, "Sanayi OG", *m.Buildings[0].TariffName)
	eq(t, "3.3333", m.Buildings[0].PurchasePrice, "B1 purchase price")
	require.Nil(t, m.Buildings[1].TariffName)

	eq(t, "1500", m.ConsumptionChart[2].Current, "chart march")
	eq(t, "800", m.ConsumptionChart[2].Previous, "chart march prev")
	eq(t, "7500", m.BillChart[2].Current, "bill chart march")
	eq(t, "4000", m.BillChart[2].Previous, "bill chart march prev")
	require.Equal(t, "TRY", *m.ChartCurrency)
	require.Empty(t, m.OmittedCurrencies)
}

func TestMonthlyCoverageCountsBuildingsWithData(t *testing.T) {
	t.Parallel()
	in := fixture()
	in.Buildings[1].Consumption = nil // B2 has analyzers but no readings this month
	m := report.BuildMonthly(in)
	eq(t, "1000", m.Consumption.Value, "one building's value, not half and not null")
	require.Equal(t, [2]int{1, 2}, [2]int{m.Consumption.WithData, m.Consumption.Of})

	in.Buildings[0].Consumption = nil
	m = report.BuildMonthly(in)
	require.Nil(t, m.Consumption.Value, "no building with data is null, never zero")
	require.Nil(t, m.DailyConsumption.Value)
	require.Nil(t, m.ConsumptionDelta.Pct)
}

func TestMonthlyKeepsCurrenciesApart(t *testing.T) {
	t.Parallel()
	in := fixture()
	usd := tryBill("100", "60", "20", "20", "5", "50", "1.2", "0.1")
	usd.Currency = "USD"
	in.Buildings[1].Bills = map[int][12]*report.Bill{2026: bills(3, usd)}
	m := report.BuildMonthly(in)
	require.Len(t, m.Bill, 2)
	require.Equal(t, "TRY", m.Bill[0].Currency, "TRY first")
	require.True(t, decimal.RequireFromString("5000").Equal(m.Bill[0].Value))
	require.Equal(t, [2]int{1, 2}, [2]int{m.Bill[0].WithData, m.Bill[0].Of})
	require.Equal(t, "USD", m.Bill[1].Currency)
	require.True(t, decimal.RequireFromString("100").Equal(m.Bill[1].Value))
	require.Len(t, m.AveragePurchasePrice, 2)
	require.True(t, decimal.RequireFromString("3.333333").Equal(m.AveragePurchasePrice[0].Value)) // 3000 / 900
	require.True(t, decimal.RequireFromString("1.2").Equal(m.AveragePurchasePrice[1].Value))      // 60 / 50
	require.Len(t, m.BillDelta, 2)
	eq(t, "25", m.BillDelta[0].Pct, "TRY (5000−4000)/4000")
	require.Nil(t, m.BillDelta[1].Pct, "USD has no previous year")
	require.Equal(t, "TRY", *m.ChartCurrency)
	require.Equal(t, []string{"USD"}, m.OmittedCurrencies)
}

func TestMonthlyPlantSelection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		selection                 string
		rooftop, utility, product string
		rooftopOut, utilityOut    bool
	}{
		{report.SelectionAll, "100", "2000", "2100", false, false},
		{report.SelectionRooftop, "100", "", "100", false, true},
		{report.SelectionGrid, "", "2000", "2000", true, false},
	} {
		in := fixture()
		in.Selection = tc.selection
		m := report.BuildMonthly(in)
		eq(t, tc.rooftop, m.Rooftop.Value, tc.selection+" rooftop")
		eq(t, tc.utility, m.Utility.Value, tc.selection+" utility")
		eq(t, tc.product, m.Production.Value, tc.selection+" production")
		require.Equal(t, tc.rooftopOut, m.Rooftop.Excluded, tc.selection)
		require.Equal(t, tc.utilityOut, m.Utility.Excluded, tc.selection)
	}
}

func TestMonthlyUtilityIsNullWithoutPlantProduction(t *testing.T) {
	t.Parallel()
	in := fixture()
	in.Plants[0].Production = nil // F9 has not filled plant_production yet
	m := report.BuildMonthly(in)
	require.Nil(t, m.Utility.Value, "no plant production is null, never 0")
	require.Equal(t, [2]int{0, 1}, [2]int{m.Utility.WithData, m.Utility.Of})
	eq(t, "100", m.Production.Value, "production is what was measured")
}

func TestMonthlyNoDefaultPrices(t *testing.T) {
	t.Parallel()
	in := fixture()
	for i := range in.Buildings {
		in.Buildings[i].Bills = nil
	}
	in.Plants[0].FeedIn = nil
	m := report.BuildMonthly(in)
	require.Empty(t, m.Bill)
	require.Empty(t, m.AveragePurchasePrice, "legacy's ₺2.5 default is not rebuilt (R257)")
	require.Nil(t, m.Buildings[0].PurchasePrice)
	require.Nil(t, m.RooftopFeedIn.Min)
	require.Nil(t, m.UtilityFeedIn.Min, "legacy's ₺1.5 default is not rebuilt")
	require.NotNil(t, m.InductiveRatio, "ratios come from readings and still exist")
	require.Nil(t, m.ChartCurrency)
}

func TestMonthlyFeedInRange(t *testing.T) {
	t.Parallel()
	in := fixture()
	in.Plants = append(in.Plants, report.PlantInput{ID: uuid.New(), Name: "Arazi 2", Kind: report.KindGrid, FeedIn: d("2.5")})
	in.Buildings[1].Bills[2026][2].GenerationPrice = nil
	m := report.BuildMonthly(in)
	eq(t, "2", m.UtilityFeedIn.Min, "min")
	eq(t, "2.5", m.UtilityFeedIn.Max, "max")
	eq(t, "1.2", m.RooftopFeedIn.Min, "a missing price does not widen the range")
	eq(t, "1.2", m.RooftopFeedIn.Max, "equal ends mean one value")
}

func TestDeltaRules(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ cur, prev, want string }{
		{"150", "100", "50"},
		{"50", "100", "-50"},
		{"100", "0", ""},          // zero previous: omitted
		{"", "100", ""},           // missing current
		{"100", "", ""},           // missing previous
		{"100.5", "300", "-66.5"}, // −66.5
		{"1", "3", "-66.7"},       // −66.666… rounds away from the truncation
		{"2", "3", "-33.3"},
	} {
		var cur, prev *decimal.Decimal
		if tc.cur != "" {
			cur = d(tc.cur)
		}
		if tc.prev != "" {
			prev = d(tc.prev)
		}
		eq(t, tc.want, report.DeltaOf(cur, prev).Pct, tc.cur+" vs "+tc.prev)
	}
}

func TestMonthlyPartialPropagates(t *testing.T) {
	t.Parallel()
	in := fixture()
	var p [12]bool
	p[2] = true
	in.Buildings[1].Partial = map[int][12]bool{2026: p}
	m := report.BuildMonthly(in)
	require.True(t, m.Consumption.Partial)
	require.True(t, m.DailyConsumption.Partial)
	require.False(t, m.ConsumptionPrev.Partial)
	require.True(t, m.Partial, "the report says it holds an incomplete month (R274)")
}

func TestMonthlyChartsHaveTwelvePoints(t *testing.T) {
	t.Parallel()
	m := report.BuildMonthly(fixture())
	require.Len(t, m.ConsumptionChart, 12)
	require.Len(t, m.BillChart, 12)
	for i, p := range m.ConsumptionChart {
		require.Equal(t, i+1, p.Month)
	}
	require.Nil(t, m.ConsumptionChart[0].Current, "January has no data: null, not 0")
}

func TestDaysIn(t *testing.T) {
	t.Parallel()
	require.Equal(t, 29, report.DaysIn(2024, 2))
	require.Equal(t, 28, report.DaysIn(2026, 2))
	require.Equal(t, 31, report.DaysIn(2026, 12))
}
