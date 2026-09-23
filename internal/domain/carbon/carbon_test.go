package carbon_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func requireEq(t *testing.T, want string, got decimal.Decimal, msg string) {
	t.Helper()
	require.True(t, dec(want).Equal(got), "%s: want %s got %s", msg, want, got)
}

func TestMappingIsLegacyVerbatim(t *testing.T) {
	require.Equal(t, []string{"cat_stationary", "cat_mobility", "cat_logistics", "cat_gas_emissions", "cat_electricity",
		"cat_external_energy", "cat_products", "cat_waste"}, carbon.Mains())
	want := map[string][3]string{
		"sub_space_heating":       {"cat_stationary", "scope_1", "category_1"},
		"sub_process_combustion":  {"cat_stationary", "scope_1", "category_1"},
		"sub_other_combustion":    {"cat_stationary", "scope_1", "category_1"},
		"sub_passenger_transport": {"cat_mobility", "scope_1", "category_1"},
		"sub_business_travel":     {"cat_mobility", "scope_3", "category_3"},
		"sub_employee_commuting":  {"cat_mobility", "scope_3", "category_3"},
		"sub_inbound_freight":     {"cat_logistics", "scope_3", "category_3"},
		"sub_outbound_freight":    {"cat_logistics", "scope_3", "category_3"},
		"sub_process_emissions":   {"cat_gas_emissions", "scope_1", "category_1"},
		"sub_fugitive_emissions":  {"cat_gas_emissions", "scope_1", "category_1"},
		"sub_grid_electricity":    {"cat_electricity", "scope_2", "category_2"},
		"sub_elec_generation":     {"cat_electricity", "scope_1", "category_1"},
		"sub_purchased_hc":        {"cat_external_energy", "scope_2", "category_2"},
		"sub_purchased_steam":     {"cat_external_energy", "scope_2", "category_2"},
		"sub_purchased_goods":     {"cat_products", "scope_3", "category_4"},
		"sub_eol_sold_products":   {"cat_products", "scope_3", "category_5"},
		"sub_capital_goods":       {"cat_products", "scope_3", "category_4"},
		"sub_waste_disposal":      {"cat_waste", "scope_3", "category_6"},
	}
	n := 0
	for _, m := range carbon.Mains() {
		for _, s := range carbon.Subs(m) {
			n++
			w, ok := want[s.Key]
			require.True(t, ok, s.Key)
			require.Equal(t, w, [3]string{s.Main, string(s.Scope), s.ISO}, s.Key)
			got, ok := carbon.SubByKey(s.Key)
			require.True(t, ok)
			require.Equal(t, s, got)
		}
	}
	require.Equal(t, len(want), n)
	_, ok := carbon.SubByKey("sub_unknown")
	require.False(t, ok)
	require.Equal(t, []string{"sub_purchased_goods", "sub_eol_sold_products", "sub_capital_goods"},
		keys(carbon.Subs("cat_products")), "legacy order")
}

func keys(subs []carbon.Sub) []string {
	out := make([]string, len(subs))
	for i, s := range subs {
		out[i] = s.Key
	}
	return out
}

// One hand-checked fixture per main category (quantity × multiplier × factor, 6 dp).
func TestEveryMainCategoryEmission(t *testing.T) {
	for name, c := range map[string][4]string{
		"stationary natural gas m3":    {"1000", "1", "2.06672", "2066.72"},
		"mobility diesel litres":       {"250.5", "1", "2.51279", "629.453895"},
		"logistics tonne-km":           {"12000", "1", "0.10787", "1294.44"},
		"gas emissions R-410A kg":      {"3.2", "1", "2088", "6681.6"},
		"electricity grid kWh":         {"1234.5", "1", "0.469", "578.9805"},
		"external energy MWh→kWh":      {"2", "1000", "0.03341", "66.82"},
		"products tonnes":              {"1.75", "1", "1071.3", "1874.775"},
		"waste kg→tonne multiplier":    {"500", "0.001", "446.2", "223.1"},
		"rounds to 6 dp half away":     {"1", "1", "0.00000025", "0"},
		"rounds up at the 7th decimal": {"1", "1", "0.0000005", "0.000001"},
	} {
		requireEq(t, c[3], carbon.Emission(dec(c[0]), dec(c[1]), dec(c[2])), name)
	}
}

func TestShareInclusiveDays(t *testing.T) {
	jan := carbon.Dated{Start: day(2026, 1, 1), End: day(2026, 1, 31), Value: dec("310")}
	requireEq(t, "310", carbon.Share(jan, day(2025, 12, 1), day(2026, 12, 31)), "inside")
	requireEq(t, "160", carbon.Share(jan, day(2026, 1, 16), day(2026, 1, 31)), "16 of 31 days")
	requireEq(t, "10", carbon.Share(jan, day(2026, 1, 31), day(2026, 2, 10)), "one day")
	requireEq(t, "0", carbon.Share(jan, day(2026, 2, 1), day(2026, 2, 28)), "no overlap")
	single := carbon.Dated{Start: day(2026, 3, 5), End: day(2026, 3, 5), Value: dec("7")}
	requireEq(t, "7", carbon.Share(single, day(2026, 3, 5), day(2026, 3, 5)), "one-day record")
}

func rec(sub string, start, end time.Time, kg string, st model.CarbonStatus) carbon.Record {
	return carbon.Record{Sub: sub, Start: start, End: end, KgCO2e: dec(kg), Status: st, Quantity: dec("1")}
}

func TestBuildYearReconciles(t *testing.T) {
	records := []carbon.Record{
		rec("sub_space_heating", day(2026, 1, 1), day(2026, 1, 31), "100", model.CarbonStatusApproved),
		rec("sub_grid_electricity", day(2026, 3, 1), day(2026, 3, 1), "50", model.CarbonStatusPending),
		rec("sub_business_travel", day(2026, 6, 10), day(2026, 6, 10), "999", model.CarbonStatusRejected),
		// 31 Dec 2025 – 31 Jan 2026: 32 days, 1 in 2025 and 31 in 2026.
		rec("sub_waste_disposal", day(2025, 12, 31), day(2026, 1, 31), "320", model.CarbonStatusApproved),
		rec("sub_purchased_goods", day(2025, 5, 1), day(2025, 5, 1), "40", model.CarbonStatusApproved),
	}
	y := carbon.BuildYear(records, 2026)
	requireEq(t, "460", y.Total, "100 + 50 + 310")
	require.Equal(t, 3, y.Count)
	require.Equal(t, 1, y.Pending)
	sumMain, sumMonth, sumScope := decimal.Zero, decimal.Zero, decimal.Zero
	for _, v := range y.ByMain {
		sumMain = sumMain.Add(v)
	}
	for _, v := range y.Monthly {
		sumMonth = sumMonth.Add(v)
	}
	for _, v := range y.ByScope {
		sumScope = sumScope.Add(v)
	}
	requireEq(t, "460", sumMain, "Σ by main")
	requireEq(t, "460", sumMonth, "Σ monthly")
	requireEq(t, "460", sumScope, "Σ by scope")
	require.Len(t, y.ByMain, 8, "every main category, zeros included")
	require.Len(t, y.ByScope, 3)
	requireEq(t, "410", y.Monthly[0], "January: heating 100 + waste 310")
	requireEq(t, "50", y.Monthly[2], "March")
	require.NotNil(t, y.Highest)
	require.Equal(t, "sub_waste_disposal", y.Highest.Sub)
	requireEq(t, "310", y.Highest.KgCO2e, "highest source")

	prev := carbon.BuildYear(records, 2025)
	requireEq(t, "50", prev.Total, "10 of the waste record + 40")
	require.Nil(t, carbon.BuildYear(nil, 2026).Highest)
}

func TestGroupsIsoAndGhg(t *testing.T) {
	records := []carbon.Record{
		rec("sub_grid_electricity", day(2026, 1, 1), day(2026, 1, 1), "10", model.CarbonStatusApproved),
		rec("sub_space_heating", day(2026, 1, 1), day(2026, 1, 1), "5", model.CarbonStatusPending),
		rec("sub_process_emissions", day(2026, 1, 2), day(2026, 1, 2), "2", model.CarbonStatusApproved),
		rec("sub_waste_disposal", day(2026, 1, 1), day(2026, 1, 1), "1", model.CarbonStatusApproved),
		rec("sub_waste_disposal", day(2026, 1, 1), day(2026, 1, 1), "8", model.CarbonStatusRejected),
	}
	iso, total, pending := carbon.Groups(records, "iso", day(2026, 1, 1), day(2026, 1, 31))
	requireEq(t, "18", total, "rejected excluded")
	require.Equal(t, 1, pending)
	require.Equal(t, []string{"category_1", "category_2", "category_3", "category_4", "category_5", "category_6"}, groupKeys(iso))
	requireEq(t, "7", iso[0].Total, "category 1")
	require.Equal(t, []string{"sub_process_emissions", "sub_space_heating"}, lineKeys(iso[0]), "lines sorted by key")
	requireEq(t, "0", iso[3].Total, "empty category present")
	requireEq(t, "1", iso[5].Total, "waste is category 6")

	ghg, _, _ := carbon.Groups(records, "ghg", day(2026, 1, 1), day(2026, 1, 31))
	require.Equal(t, []string{"scope_1", "scope_2", "scope_3"}, groupKeys(ghg))
	requireEq(t, "10", ghg[1].Total, "scope 2")
}

func groupKeys(gs []carbon.Group) []string {
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.Key
	}
	return out
}

func lineKeys(g carbon.Group) []string {
	out := make([]string, len(g.Lines))
	for i, l := range g.Lines {
		out[i] = l.Sub
	}
	return out
}

func TestReport94(t *testing.T) {
	grid := carbon.Record{Sub: "sub_grid_electricity", Start: day(2026, 1, 1), End: day(2026, 1, 1), Quantity: dec("10000"),
		KgCO2e: dec("4690"), Status: model.CarbonStatusApproved, Automated: true}
	gen := carbon.Record{Sub: "sub_elec_generation", Start: day(2026, 1, 2), End: day(2026, 1, 2), Quantity: dec("2000"),
		Status: model.CarbonStatusApproved, Automated: true}
	manual := carbon.Record{Sub: "sub_grid_electricity", Start: day(2026, 1, 3), End: day(2026, 1, 3), Quantity: dec("500"),
		KgCO2e: dec("234.5"), Status: model.CarbonStatusApproved}
	f := carbon.Report94([]carbon.Record{grid, gen, manual}, day(2026, 1, 1), day(2026, 1, 31), dec("0.469"))
	require.NotNil(t, f)
	requireEq(t, "10000", f.ConsumptionKwh, "automated grid only")
	requireEq(t, "2000", f.GenerationKwh, "generation")
	requireEq(t, "4.69", f.ConsumptionT, "consumption t")
	requireEq(t, "0.938", f.ReductionT, "reduction t")
	requireEq(t, "3.752", f.NetT, "net t")
	require.Nil(t, carbon.Report94([]carbon.Record{manual}, day(2026, 1, 1), day(2026, 1, 31), dec("0.469")))
}
