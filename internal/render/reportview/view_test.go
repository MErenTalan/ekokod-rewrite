package reportview_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/reportview"
)

func d(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

func every(v string) report.MonthSeries {
	var s report.MonthSeries
	for i := range s {
		s[i] = d(v)
	}
	return s
}

// Monthly is a small real payload: built by the domain, not by hand.
func Monthly() report.Payload {
	var bills [12]*report.Bill
	bills[2] = &report.Bill{Currency: "TRY", Total: *d("5000"), EnergyCost: *d("3000"), NetConsumption: *d("1000"), ReactivePenalty: *d("10")}
	m := report.BuildMonthly(report.MonthlyInput{Year: 2026, Month: 3, Selection: report.SelectionAll,
		Buildings: []report.BuildingInput{{ID: uuid.New(), Name: "Merkez",
			Consumption: map[int]report.MonthSeries{2026: every("1000"), 2025: every("900")},
			Bills:       map[int][12]*report.Bill{2026: bills}}},
		Plants: []report.PlantInput{{ID: uuid.New(), Name: "Arazi", Kind: report.KindGrid}}})
	return report.Payload{Version: 1, Type: report.TypeMonthly, Period: "2026-03", Monthly: &m}
}

func Yearly(withFactor bool) report.Payload {
	in := report.YearlyInput{Year: 2025, Selection: report.SelectionAll,
		Buildings: []report.BuildingInput{{ID: uuid.New(), Name: "Merkez",
			Consumption: map[int]report.MonthSeries{2025: every("1000")}, Export: map[int]report.MonthSeries{2025: every("50")}}},
		Plants: []report.PlantInput{{ID: uuid.New(), Name: "Arazi", Kind: report.KindGrid, YearlyTarget: d("9000"),
			Production: map[int]report.MonthSeries{2025: every("600")}}}}
	if withFactor {
		y := 2022
		in.Factor = &report.GridFactor{Value: *d("0.45"), Unit: "kg CO2e/kWh", SourceYear: &y}
	}
	y := report.BuildYearly(in)
	return report.Payload{Version: 1, Type: report.TypeYearly, Period: "2025", Yearly: &y}
}

func keys(doc reportview.Document) []string {
	var out []string
	for _, s := range doc.Sections {
		out = append(out, s.Key)
	}
	return out
}

func section(t *testing.T, doc reportview.Document, key string) reportview.Section {
	t.Helper()
	for _, s := range doc.Sections {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("no section %q", key)
	return reportview.Section{}
}

// TestMonthlySections is §7.14's monthly list: information table (fifteen
// rows), four summary cards, two charts with their tables.
func TestMonthlySections(t *testing.T) {
	t.Parallel()
	for _, locale := range []string{"tr", "en"} {
		doc := reportview.Build(Monthly(), "Ekokod A.Ş.", []string{"Merkez"}, locale)
		require.Equal(t, []string{"info", "summary", "consumption_chart", "bill_chart"}, keys(doc), locale)
		require.Len(t, section(t, doc, "info").Rows, 15, locale)
		require.Len(t, section(t, doc, "summary").Rows, 4, locale)
		for _, k := range []string{"consumption_chart", "bill_chart"} {
			s := section(t, doc, k)
			require.NotNil(t, s.Chart, k)
			require.Len(t, s.Chart.Categories, 12, k)
			require.Len(t, s.Chart.Series, 2, k)
			require.Len(t, s.Rows, 12, "the chart's data table (07 §5)")
			require.NotEmpty(t, s.Chart.Unit, "axis units are always labelled")
		}
		for _, s := range doc.Sections {
			require.NotEmpty(t, s.Title, s.Key)
		}
	}
	tr := reportview.Build(Monthly(), "Ekokod A.Ş.", []string{"Merkez"}, "tr")
	en := reportview.Build(Monthly(), "Ekokod A.Ş.", []string{"Merkez"}, "en")
	require.NotEqual(t, tr.Sections[0].Title, en.Sections[0].Title)
}

// TestYearlySections is §7.14's yearly list.
func TestYearlySections(t *testing.T) {
	t.Parallel()
	doc := reportview.Build(Yearly(true), "Ekokod A.Ş.", []string{"Merkez"}, "tr")
	require.Equal(t, []string{"consumption_table", "solar", "summary", "history_chart", "bill_chart", "target_chart", "comparison", "carbon"}, keys(doc))
	require.Len(t, section(t, doc, "consumption_table").Rows, 15, "twelve months, the total and two daily averages")
	require.Len(t, section(t, doc, "summary").Rows, 4)
	require.Len(t, section(t, doc, "comparison").Rows, 4)
	carbon := section(t, doc, "carbon")
	require.Len(t, carbon.Rows, 4, "consumption, reduction, net and the factor")
	require.True(t, strings.Contains(strings.Join(carbon.Notes, " "), "2022"), "the factor's source year is stated")
	require.Len(t, section(t, doc, "target_chart").Chart.Categories, 1, "one bar group per plant")
	require.Len(t, section(t, doc, "history_chart").Chart.Categories, 3, "Y−2…Y")
}

func TestMissingIsNoData(t *testing.T) {
	t.Parallel()
	doc := reportview.Build(Monthly(), "Ekokod A.Ş.", []string{"Merkez"}, "tr")
	info := section(t, doc, "info")
	var utility []string
	for _, r := range info.Rows {
		if strings.HasPrefix(r[0], "Arazi GES aylık üretimi") {
			utility = r
		}
	}
	require.NotNil(t, utility, "the utility-scale row exists")
	require.Equal(t, "veri yok", utility[1], "no plant production is 'veri yok', never 0")
	en := reportview.Build(Monthly(), "Ekokod A.Ş.", []string{"Merkez"}, "en")
	require.Contains(t, strings.Join(section(t, en, "info").Rows[6], " "), "no data")
}

func TestCarbonUnavailableSentence(t *testing.T) {
	t.Parallel()
	doc := reportview.Build(Yearly(false), "Ekokod A.Ş.", []string{"Merkez"}, "tr")
	carbon := section(t, doc, "carbon")
	require.Empty(t, carbon.Rows)
	require.NotEmpty(t, carbon.Notes, "the section says why it is empty")
}

func TestPartialAndOmittedCurrencyAreNoted(t *testing.T) {
	t.Parallel()
	p := Monthly()
	p.Monthly.Partial = true
	p.Monthly.OmittedCurrencies = []string{"USD"}
	doc := reportview.Build(p, "Ekokod A.Ş.", []string{"Merkez"}, "tr")
	require.NotEmpty(t, doc.Notes, "R274: an incomplete month is labelled")
	require.Contains(t, strings.Join(section(t, doc, "bill_chart").Notes, " "), "USD", "R270")
}

func TestEmptyChartBecomesASentence(t *testing.T) {
	t.Parallel()
	for _, locale := range []string{"tr", "en"} {
		doc := reportview.Build(Yearly(true), "Ekokod A.Ş.", []string{"Merkez"}, locale)
		bill := section(t, doc, "bill_chart") // the fixture has no bills
		require.Nil(t, bill.Chart, "no empty axis frame (07 §5)")
		require.NotEmpty(t, bill.Notes, locale)
		require.NotEmpty(t, bill.Notes[0], "the sentence says there is no data (%s)", locale)
	}
}
