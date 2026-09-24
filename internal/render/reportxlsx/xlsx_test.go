package reportxlsx_test

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/reportview"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/reportxlsx"
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

func monthly() report.Payload {
	var bills [12]*report.Bill
	bills[2] = &report.Bill{Currency: "TRY", Total: *d("5000"), EnergyCost: *d("3000"), NetConsumption: *d("1000")}
	m := report.BuildMonthly(report.MonthlyInput{Year: 2026, Month: 3, Selection: report.SelectionAll,
		Buildings: []report.BuildingInput{{ID: uuid.New(), Name: "Merkez",
			Consumption: map[int]report.MonthSeries{2026: every("1000"), 2025: every("900")},
			Bills:       map[int][12]*report.Bill{2026: bills}}},
		Plants: []report.PlantInput{{ID: uuid.New(), Name: "Arazi", Kind: report.KindGrid}}})
	return report.Payload{Version: 1, Type: report.TypeMonthly, Period: "2026-03", Monthly: &m}
}

func yearly() report.Payload {
	year := 2022
	y := report.BuildYearly(report.YearlyInput{Year: 2025, Selection: report.SelectionAll,
		Buildings: []report.BuildingInput{{ID: uuid.New(), Name: "Merkez", Consumption: map[int]report.MonthSeries{2025: every("1000")}}},
		Plants: []report.PlantInput{{ID: uuid.New(), Name: "Arazi", Kind: report.KindGrid, YearlyTarget: d("9000"),
			Production: map[int]report.MonthSeries{2025: every("600")}}},
		Factor: &report.GridFactor{Value: *d("0.45"), Unit: "kg CO2e/kWh", SourceYear: &year}})
	return report.Payload{Version: 1, Type: report.TypeYearly, Period: "2025", Yearly: &y}
}

// cells is every non-empty cell of every sheet, and the chart parts in the zip.
func open(t *testing.T, raw []byte) (map[string][][]string, int) {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	sheets := map[string][][]string{}
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		require.NoError(t, err)
		sheets[name] = rows
	}
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	charts := 0
	for _, file := range z.File {
		if strings.HasPrefix(file.Name, "xl/charts/chart") && strings.HasSuffix(file.Name, ".xml") {
			charts++
		}
	}
	return sheets, charts
}

func firstColumn(rows [][]string) string {
	var b strings.Builder
	for _, r := range rows {
		if len(r) > 0 {
			b.WriteString(r[0] + "\n")
		}
	}
	return b.String()
}

func requireSections(t *testing.T, p report.Payload, locale string) {
	t.Helper()
	raw, err := reportxlsx.Render(p, "Ekokod A.Ş.", []string{"Merkez"}, locale)
	require.NoError(t, err)
	sheets, charts := open(t, raw)
	doc := reportview.Build(p, "Ekokod A.Ş.", []string{"Merkez"}, locale)
	main := sheets[reportxlsx.SheetName(locale)]
	require.NotNil(t, main, "the report sheet exists")
	text := firstColumn(main)
	withCharts := 0
	for _, s := range doc.Sections {
		require.Contains(t, text, s.Title+"\n", "section %q is in the workbook (%s)", s.Key, locale)
		for _, r := range s.Rows {
			require.Contains(t, text, r[0]+"\n", "row %q of %q", r[0], s.Key)
		}
		if s.Chart != nil {
			withCharts++
		}
	}
	require.Equal(t, withCharts, charts, "one native chart per chart section")
	require.NotEmpty(t, sheets[reportxlsx.DataSheetName(locale)], "the chart data sheet feeds the charts")
}

func TestMonthlyWorkbookHasEverySection(t *testing.T) {
	for _, locale := range []string{"tr", "en"} {
		requireSections(t, monthly(), locale)
	}
}

func TestYearlyWorkbookHasEverySection(t *testing.T) {
	for _, locale := range []string{"tr", "en"} {
		requireSections(t, yearly(), locale)
	}
	// Independent of reportview: §7.14's last yearly section, by its own words.
	raw, err := reportxlsx.Render(yearly(), "Ekokod A.Ş.", []string{"Merkez"}, "tr")
	require.NoError(t, err)
	sheets, _ := open(t, raw)
	text := firstColumn(sheets[reportxlsx.SheetName("tr")])
	for _, title := range []string{"Nihai karşılaştırma", "Karbon emisyonu", "Güneş enerjisi üretim raporu"} {
		require.Contains(t, text, title)
	}
}

func TestWorkbookMissingIsNoData(t *testing.T) {
	raw, err := reportxlsx.Render(monthly(), "Ekokod A.Ş.", []string{"Merkez"}, "tr")
	require.NoError(t, err)
	sheets, _ := open(t, raw)
	found := false
	for _, r := range sheets[reportxlsx.SheetName("tr")] {
		if len(r) >= 2 && r[0] == "Arazi GES aylık üretimi" {
			require.Equal(t, "veri yok", r[1])
			found = true
		}
	}
	require.True(t, found)
}

func TestEmptyPayloadRenders(t *testing.T) {
	_, err := reportxlsx.Render(report.Payload{}, "", nil, "tr")
	require.NoError(t, err, "a pending report's empty payload must not break the download")
}
