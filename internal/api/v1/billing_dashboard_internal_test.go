package v1

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func dashboardFixture() domain.DashboardResult {
	b := uuid.New()
	id := uuid.New()
	price := decimal.RequireFromString("3.12")
	row := domain.DashboardRow{BillID: uuid.New(), BuildingID: &b, AnalyzerID: &id, BuildingName: "Bina A",
		AnalyzerName: "Sayaç 1", InstallationNumber: "40001", EtsoCode: "40Z0000000001A", PeriodKey: "2026-08",
		Consumption: decimal.RequireFromString("150"), Production: decimal.Zero, ConsumptionPrice: &price,
		Invoice: decimal.RequireFromString("468"), Currency: model.CurrencyTRY}
	return domain.BuildDashboard([]domain.DashboardRow{row}, nil, nil)
}

func TestDashboardDTOAlwaysSaysThePlantSectionHasNoSource(t *testing.T) {
	// R235: the section 01 §7.10 describes has no data source until F9, and
	// the API says so rather than returning an empty table that reads as zero.
	got := dashboardDTO(dashboardFixture(), "2026-08")
	require.False(t, got.Plants.Available)
	require.Equal(t, "no_plant_production_source", got.Plants.Reason)
}

func TestDashboardDTOCarriesEveryColumnOf710(t *testing.T) {
	got := dashboardDTO(dashboardFixture(), "2026-08")
	require.Equal(t, "2026-08", got.Period)
	require.Len(t, got.Buildings, 1)
	row := got.Buildings[0].Rows[0]
	require.Equal(t, "Bina A", row.BuildingName)
	require.Equal(t, "Sayaç 1", row.AnalyzerName)
	require.Equal(t, "40001", row.InstallationNumber)
	require.Equal(t, "40Z0000000001A", row.EtsoCode)
	require.Equal(t, "150", row.Consumption.String())
	require.NotNil(t, row.ConsumptionPrice)
	require.Equal(t, "3.12", row.ConsumptionPrice.String())
	require.Equal(t, "468", row.Invoice.String())
	require.Equal(t, "TRY", row.Currency)
}

func TestDashboardDTOTotalsAreTheRowsTotals(t *testing.T) {
	got := dashboardDTO(dashboardFixture(), "2026-08")
	require.Equal(t, "468", got.Buildings[0].TotalInvoice.String())
	require.Len(t, got.Netting, 1)
	require.Equal(t, "468", got.Netting[0].TotalInvoice.String())
	require.Equal(t, "net_consumption", got.Netting[0].NetStatus)
	// No production at all: the efficiency is zero, not absent — the ratio exists.
	require.NotNil(t, got.Netting[0].EfficiencyPct)
	require.Equal(t, "0", got.Netting[0].EfficiencyPct.String())
}

func TestDashboardDTOOmitsAnEfficiencyThatDoesNotExist(t *testing.T) {
	res := dashboardFixture()
	res.Netting[0].EfficiencyPct = nil
	require.Nil(t, dashboardDTO(res, "2026-08").Netting[0].EfficiencyPct)
}
