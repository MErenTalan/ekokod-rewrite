package invoicepdf

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func TestLabelsCoverEveryLineCode(t *testing.T) {
	codes := append(append([]string{}, model.BillLineCodes...), model.BillLineExtraPrefix, model.BillLineTaxPrefix)
	for _, locale := range []string{"tr", "en"} {
		for _, code := range codes {
			_, ok := labels[locale][code]
			require.True(t, ok, "%s label missing for %q", locale, code)
		}
		for key := range labels["tr"] {
			_, ok := labels[locale][key]
			require.True(t, ok, "%s lacks key %q that tr has", locale, key)
		}
	}
	require.Equal(t, "Vergi/fon: BTV", lineLabel("tr", "tax:BTV"))
}

func TestDayRangesCollapsesRuns(t *testing.T) {
	got := DayRanges([]string{"2026-09-01", "2026-09-02", "2026-09-03", "2026-09-05", "2026-09-30", "2026-10-01"})
	require.Equal(t, []string{"01.09.2026 – 03.09.2026", "05.09.2026", "30.09.2026 – 01.10.2026"}, got)
	require.Empty(t, DayRanges(nil))
}

// The index table names registers in the invoice's language, in a fixed
// order, and leaves out the ones the meter never reported.
func TestIndexRowsNameAndSkipRegisters(t *testing.T) {
	start := []byte(`{"active_import":"16783.576","reactive_inductive_import":"5958.197","t1_import":null,"active_export":"2375.891","t2_export":null}`)
	end := []byte(`{"active_import":"18630.836","reactive_inductive_import":"6784.622","t1_import":null,"active_export":"3066.135","custom_reg":"7"}`)

	rows := IndexRows("tr", start, end)
	require.Equal(t, [][4]string{
		{"Aktif çekiş (kWh)", "16.783,576", "18.630,836", "1.847,260"},
		{"Endüktif reaktif (kVArh)", "5.958,197", "6.784,622", "826,425"},
		{"Aktif veriş (kWh)", "2.375,891", "3.066,135", "690,244"},
		{"custom_reg", "—", "7,000", "—"},
	}, rows)
	require.Empty(t, IndexRows("tr", nil, nil))
	require.Equal(t, "Active import (kWh)", IndexRows("en", start, end)[0][0])
}

func TestPercent(t *testing.T) {
	ratio := decimal.RequireFromString("0.4474")
	require.Equal(t, "%44,74", Percent("tr", &ratio))
	limit := decimal.RequireFromString("0.2")
	require.Equal(t, "%20,00", Percent("tr", &limit))
	require.Equal(t, "—", Percent("tr", nil))
	require.Equal(t, "44.74%", Percent("en", &ratio))
}

func TestMonthLabel(t *testing.T) {
	require.Equal(t, "Ocak 2026", monthLabel("tr", "2026-01"))
	require.Equal(t, "Aralık 2025", monthLabel("tr", "2025-12"))
	require.Equal(t, "January 2026", monthLabel("en", "2026-01"))
	require.Equal(t, "2026-13", monthLabel("tr", "2026-13"), "an unparsable key prints as is")
}

func TestFlagText(t *testing.T) {
	require.Equal(t, "PTF verisi eksik, Kurulu güç tanımsız", flagText("tr", "ptf_data_missing,installed_power_missing"))
	require.Equal(t, "Reactive data missing, other_flag", flagText("en", "reactive_data_missing,other_flag"))
}

func TestReactiveStatus(t *testing.T) {
	limit := decimal.RequireFromString("0.2")
	flag := func(s string) *string { return &s }
	waiver := []model.BillLine{{Code: model.BillLineReactiveWaiver, Amount: decimal.RequireFromString("-10")}}
	cases := []struct {
		name  string
		bill  model.Bill
		lines []model.BillLine
		want  string
	}{
		{"waived", model.Bill{Scope: model.BillScopeAnalyzer, ReactivePenaltyApplied: true, InductiveThreshold: &limit}, waiver, "st:waived"},
		{"charged", model.Bill{Scope: model.BillScopeAnalyzer, ReactivePenaltyApplied: true, InductiveThreshold: &limit}, nil, "st:charged"},
		{"within", model.Bill{Scope: model.BillScopeBuilding, InductiveThreshold: &limit}, nil, "st:within"},
		{"no data", model.Bill{Scope: model.BillScopeBuilding, InductiveThreshold: &limit, FlagReason: flag("reactive_data_missing")}, nil, "st:no_data"},
		{"no power", model.Bill{Scope: model.BillScopeBuilding, FlagReason: flag("ptf_data_missing,installed_power_missing")}, nil, "st:no_power"},
		{"exempt", model.Bill{Scope: model.BillScopeBuilding}, nil, "st:exempt"},
		{"company charged", model.Bill{Scope: model.BillScopeCompany, ReactivePenaltyApplied: true, ReactivePenalty: decimal.RequireFromString("5")}, waiver, "st:company_charged"},
		{"company all waived", model.Bill{Scope: model.BillScopeCompany, ReactivePenaltyApplied: true}, waiver, "st:waived"},
		{"company none", model.Bill{Scope: model.BillScopeCompany}, nil, "st:company_none"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, reactiveStatus(c.bill, c.lines), c.name)
	}
}

func TestSplitLinesMovesVatAndCreditBelowTheBase(t *testing.T) {
	amount := decimal.RequireFromString
	lines := []model.BillLine{
		{Code: model.BillLineVat, Amount: amount("20"), SortOrder: 3},
		{Code: model.BillLineGenerationCredit, Amount: amount("-5"), SortOrder: 4},
		{Code: model.BillLineGenerationSale, Amount: amount("7"), SortOrder: 5},
		{Code: model.BillLineDistribution, Amount: amount("40"), SortOrder: 1},
		{Code: model.BillLineEnergy, Amount: amount("60"), SortOrder: 0},
	}
	body, vat, credit, sale := splitLines(lines)
	require.Len(t, body, 2)
	require.Equal(t, model.BillLineEnergy, body[0].Code)
	require.Equal(t, model.BillLineDistribution, body[1].Code)
	require.Equal(t, "20", vat.Amount.String())
	require.Equal(t, "-5", credit.Amount.String())
	require.Equal(t, "7", sale.Amount.String(), "R531: the sale prints below the amount due")
	_, vat, credit, sale = splitLines(body)
	require.Nil(t, vat)
	require.Nil(t, credit)
	require.Nil(t, sale)
}
