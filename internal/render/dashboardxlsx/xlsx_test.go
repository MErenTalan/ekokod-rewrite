package dashboardxlsx_test

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/dashboardxlsx"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func result() billing.DashboardResult {
	b1, b2 := uuid.New(), uuid.New()
	row := func(b uuid.UUID, building, analyzer, kwh, cost string) billing.DashboardRow {
		id := uuid.New()
		price := d("3.12")
		return billing.DashboardRow{BillID: uuid.New(), BuildingID: &b, AnalyzerID: &id, BuildingName: building,
			AnalyzerName: analyzer, InstallationNumber: "40001", EtsoCode: "40Z0000000001A", PeriodKey: "2026-08",
			Consumption: d(kwh), Production: d("0"), ConsumptionPrice: &price, Invoice: d(cost), Currency: model.CurrencyTRY}
	}
	return billing.BuildDashboard([]billing.DashboardRow{
		row(b1, "Bina A", "Sayaç 1", "100", "250.50"),
		row(b1, "Bina A", "Sayaç 2", "50", "125.25"),
		row(b2, "ĞÜŞİÖÇ Binası", "Sayaç 3", "200", "500"),
	}, nil, nil)
}

func open(t *testing.T, body []byte) *excelize.File {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(body))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestWorkbookHasABillsSheetAndASummarySheet(t *testing.T) {
	body, err := dashboardxlsx.Render(result(), "2026-08", "tr")
	require.NoError(t, err)
	require.Equal(t, []string{"Faturalar", "Özet"}, open(t, body).GetSheetList())

	en, err := dashboardxlsx.Render(result(), "2026-08", "en")
	require.NoError(t, err)
	require.Equal(t, []string{"Invoices", "Summary"}, open(t, en).GetSheetList())
}

func TestWorkbookTotalEqualsTheSumOfItsRows(t *testing.T) {
	// R239: the file may not disagree with the screen, and the screen's total
	// is the sum of the rows it shows (09 §F8).
	body, err := dashboardxlsx.Render(result(), "2026-08", "tr")
	require.NoError(t, err)
	rows, err := open(t, body).GetRows("Faturalar")
	require.NoError(t, err)

	last := rows[len(rows)-1]
	require.True(t, strings.HasPrefix(last[0], "Toplam"), "the grand total is the last row, got %q", last[0])

	invoices := decimal.Zero
	for _, r := range rows[1 : len(rows)-1] {
		if strings.HasPrefix(r[0], "Toplam") || strings.HasPrefix(r[0], "Ara toplam") {
			continue
		}
		v, err := decimal.NewFromString(r[len(r)-1])
		require.NoError(t, err, "row %v", r)
		invoices = invoices.Add(v)
	}
	require.Equal(t, "875.75", invoices.String())
	require.Equal(t, "875.75", last[len(last)-1])
}

func TestWorkbookCarriesASubtotalPerBuilding(t *testing.T) {
	body, err := dashboardxlsx.Render(result(), "2026-08", "tr")
	require.NoError(t, err)
	rows, err := open(t, body).GetRows("Faturalar")
	require.NoError(t, err)

	var subtotals []string
	for _, r := range rows {
		if strings.HasPrefix(r[0], "Ara toplam") {
			subtotals = append(subtotals, r[len(r)-1])
		}
	}
	require.Equal(t, []string{"375.75", "500"}, subtotals)
}

func TestSummarySheetSaysThePlantSectionHasNoSourceYet(t *testing.T) {
	// The file must not be quietly missing a section the screen shows (R235).
	body, err := dashboardxlsx.Render(result(), "2026-08", "tr")
	require.NoError(t, err)
	rows, err := open(t, body).GetRows("Özet")
	require.NoError(t, err)

	var flat []string
	for _, r := range rows {
		flat = append(flat, strings.Join(r, " "))
	}
	joined := strings.Join(flat, "\n")
	require.Contains(t, joined, "875.75", "the netting total belongs in the summary")
	require.Contains(t, joined, "GES", "the plant section is named even though it has no data")
}

func TestEveryDataRowCarriesTheSameColumnCount(t *testing.T) {
	body, err := dashboardxlsx.Render(result(), "2026-08", "tr")
	require.NoError(t, err)
	rows, err := open(t, body).GetRows("Faturalar")
	require.NoError(t, err)
	for i, r := range rows {
		require.Len(t, r, len(rows[0]), "row "+strconv.Itoa(i)+" is ragged: "+strings.Join(r, "|"))
	}
}
