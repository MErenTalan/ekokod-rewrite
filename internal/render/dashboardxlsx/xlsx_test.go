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

func withPlants(r billing.DashboardResult) billing.DashboardResult {
	kwh, price, amount, cprice := d("1200"), d("1.8"), d("2160.00"), d("3.12")
	name, number := "Sayaç 1", "40001"
	try := model.CurrencyTRY
	total := d("1200")
	r.Plants = billing.DashboardPlants{Available: true, Rows: []billing.DashboardPlantRow{
		{PlantID: uuid.New(), PlantName: "Konya GES", AnalyzerName: &name, InstallationNumber: &number, ProductionKwh: &kwh,
			ProductionPrice: &price, ProductionCurrency: &try, ConsumptionPrice: &cprice, InvoiceAmount: &amount},
		{PlantID: uuid.New(), PlantName: "Veri Yok GES"},
	}, TotalProductionKwh: &total, TotalInvoice: []billing.DashboardMoney{{Currency: model.CurrencyTRY, Amount: amount}}}
	return r
}

func summary(t *testing.T, r billing.DashboardResult) string {
	t.Helper()
	body, err := dashboardxlsx.Render(r, "2026-08", "tr")
	require.NoError(t, err)
	rows, err := open(t, body).GetRows("Özet")
	require.NoError(t, err)
	var flat []string
	for _, row := range rows {
		flat = append(flat, strings.Join(row, " | "))
	}
	return strings.Join(flat, "\n")
}

// R290: the file lists the plant rows the screen shows, with their totals.
func TestSummarySheetListsPlantRows(t *testing.T) {
	joined := summary(t, withPlants(result()))
	require.Contains(t, joined, "875.75", "the netting total belongs in the summary")
	require.Contains(t, joined, "Konya GES | Sayaç 1 | 40001 | 1200 | 3.12 | 1.8 | 2160 TRY")
	require.Contains(t, joined, "Veri Yok GES | — | — | veri yok", "missing is not zero")
	require.Contains(t, joined, "Toplam üretim (kWh) | 1200")
}

func TestSummaryOmitsPlantsOutOfScope(t *testing.T) {
	r := result()
	r.Plants = billing.DashboardPlants{Reason: "plants_not_in_scope"}
	require.NotContains(t, summary(t, r), "GES")
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
