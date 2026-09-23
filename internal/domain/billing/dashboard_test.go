package billing_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func TestDashboardTotalsFollowTheRowsShown(t *testing.T) {
	t.Parallel()
	b := uuid.New()
	got := billing.BuildDashboard([]billing.DashboardRow{
		analyzerRow(b, "A", "120.5", "310.25"),
		analyzerRow(b, "B", "80.5", "199.75"),
	}, nil, nil)

	require.Len(t, got.Buildings, 1)
	// 09 §F8's acceptance criterion: the total IS the sum of the rows.
	require.Equal(t, "201", got.Buildings[0].TotalConsumption.String())
	require.Equal(t, "510", got.Buildings[0].TotalInvoice.String())
	require.Len(t, got.Netting, 1)
	require.Equal(t, "510", got.Netting[0].TotalInvoice.String())
	require.Equal(t, "201", got.Netting[0].TotalConsumption.String())
}

func TestDashboardShowsBuildingBillDivergenceInsteadOfHidingIt(t *testing.T) {
	t.Parallel()
	b := uuid.New()
	rows := []billing.DashboardRow{analyzerRow(b, "A", "100", "250"), analyzerRow(b, "B", "100", "250")}
	// 02 §6.11: a building invoice prices the aggregate once, so it may
	// legitimately differ from the sum of the analyzer invoices.
	got := billing.BuildDashboard(rows, []billing.DashboardRow{buildingRow(b, "200", "480")}, nil)

	require.Equal(t, "500", got.Buildings[0].TotalInvoice.String())
	require.NotNil(t, got.Buildings[0].BuildingBill)
	require.Equal(t, "480", got.Buildings[0].BuildingBill.Invoice.String())
	require.True(t, got.Buildings[0].DivergesFromRows)
}

func TestDashboardDoesNotCallARoundingDifferenceADivergence(t *testing.T) {
	t.Parallel()
	b := uuid.New()
	rows := []billing.DashboardRow{analyzerRow(b, "A", "100", "250"), analyzerRow(b, "B", "100", "250")}
	got := billing.BuildDashboard(rows, []billing.DashboardRow{buildingRow(b, "200", "500.01")}, nil)

	require.False(t, got.Buildings[0].DivergesFromRows, "a kurus is rounding, not a different invoice")
}

func TestDashboardKeepsCurrenciesApart(t *testing.T) {
	t.Parallel()
	try, usd := analyzerRow(uuid.New(), "A", "100", "250"), analyzerRow(uuid.New(), "B", "100", "250")
	usd.Currency = model.CurrencyUSD
	got := billing.BuildDashboard([]billing.DashboardRow{try, usd}, nil, nil)

	require.Len(t, got.Netting, 2, "one netting summary per currency; TRY and USD are never added")
	byCurrency := map[model.CurrencyCode]string{}
	for _, n := range got.Netting {
		byCurrency[n.Currency] = n.TotalInvoice.String()
	}
	require.Equal(t, "250", byCurrency[model.CurrencyTRY])
	require.Equal(t, "250", byCurrency[model.CurrencyUSD])
}

func TestDashboardKeepsRowsWithoutLiveParents(t *testing.T) {
	t.Parallel()
	// A building soft-deleted after the invoice was issued: the service
	// resolves no name, and the row still counts — the invoice was issued.
	row := analyzerRow(uuid.New(), "A", "50", "120")
	row.BuildingName = ""
	got := billing.BuildDashboard([]billing.DashboardRow{row}, nil, nil)

	require.Len(t, got.Buildings, 1)
	require.Equal(t, "120", got.Buildings[0].TotalInvoice.String())
}

func TestDashboardNettingAndEfficiency(t *testing.T) {
	t.Parallel()
	b := uuid.New()
	produce := analyzerRow(b, "B", "0", "0")
	produce.Production = d("250")
	got := billing.BuildDashboard([]billing.DashboardRow{analyzerRow(b, "A", "1000", "2500"), produce}, nil, nil)

	n := got.Netting[0]
	require.Equal(t, "750", n.Net.String())
	require.Equal(t, "net_consumption", n.NetStatus)
	require.Equal(t, "25", n.EfficiencyPct.String())
	require.Equal(t, "2026-08", n.PeriodKey)
}

func TestDashboardEfficiencyIsAbsentWithoutConsumption(t *testing.T) {
	t.Parallel()
	row := analyzerRow(uuid.New(), "A", "0", "0")
	row.Production = d("40")
	got := billing.BuildDashboard([]billing.DashboardRow{row}, nil, nil)

	require.Nil(t, got.Netting[0].EfficiencyPct, "no ratio exists against zero consumption")
	require.Equal(t, "net_production", got.Netting[0].NetStatus)
	require.Equal(t, "-40", got.Netting[0].Net.String())
}

func TestDashboardCarriesTheCompanyBillOfItsCurrency(t *testing.T) {
	t.Parallel()
	b := uuid.New()
	company := buildingRow(b, "200", "495")
	company.BuildingID = nil
	got := billing.BuildDashboard([]billing.DashboardRow{analyzerRow(b, "A", "200", "500")}, nil, []billing.DashboardRow{company})

	require.NotNil(t, got.Netting[0].CompanyBillID)
	require.Equal(t, company.BillID, *got.Netting[0].CompanyBillID)
}

func TestDashboardWithoutRowsHasNoTotals(t *testing.T) {
	t.Parallel()
	got := billing.BuildDashboard(nil, nil, nil)

	require.Empty(t, got.Buildings)
	require.Empty(t, got.Netting, "an empty month shows an empty state, never a zero total over an empty table")
}

func analyzerRow(buildingID uuid.UUID, name, kwh, cost string) billing.DashboardRow {
	id := uuid.New()
	return billing.DashboardRow{BillID: uuid.New(), BuildingID: &buildingID, AnalyzerID: &id, AnalyzerName: name,
		BuildingName: "Bina", InstallationNumber: "40001", EtsoCode: "40Z0000000001A", PeriodKey: "2026-08",
		Consumption: d(kwh), Production: d("0"), Invoice: d(cost), Currency: model.CurrencyTRY}
}

func buildingRow(buildingID uuid.UUID, kwh, cost string) billing.DashboardRow {
	return billing.DashboardRow{BillID: uuid.New(), BuildingID: &buildingID, BuildingName: "Bina", PeriodKey: "2026-08",
		Consumption: d(kwh), Production: d("0"), Invoice: d(cost), Currency: model.CurrencyTRY}
}
