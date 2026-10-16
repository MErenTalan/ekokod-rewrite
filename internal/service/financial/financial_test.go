package financial_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/financial"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func requireDec(t *testing.T, want string, got *decimal.Decimal, msg string) {
	t.Helper()
	require.NotNil(t, got, msg)
	require.True(t, dec(want).Equal(*got), "%s: want %s got %s", msg, want, got)
}

func amount(t *testing.T, list []financial.Money, c model.CurrencyCode) string {
	t.Helper()
	for _, m := range list {
		if m.Currency == c {
			return m.Amount.StringFixed(2)
		}
	}
	return ""
}

var p1, p2 = uuid.New(), uuid.New()

func bill(period, net, cost string, c model.CurrencyCode) model.Bill {
	return model.Bill{ID: uuid.New(), Scope: model.BillScopeBuilding, PeriodKey: period, ActiveImport: dec(net), NetConsumption: dec(net), TotalCost: dec(cost), Currency: c}
}

func inputs() financial.YearInputs {
	return financial.YearInputs{
		Year:  2026,
		Bills: []model.Bill{bill("2026-01", "1000", "3000", model.CurrencyTRY), bill("2026-01", "500", "1600", model.CurrencyTRY), bill("2026-02", "800", "2500", model.CurrencyTRY)},
		Production: map[uuid.UUID]map[int]decimal.Decimal{
			p1: {1: dec("1200"), 2: dec("300"), 3: dec("100")},
			p2: {1: dec("100")},
		},
		FeedIn: map[uuid.UUID]map[int]financial.Price{
			p1: {1: {Value: dec("2.0"), Currency: model.CurrencyTRY}, 2: {Value: dec("2.5"), Currency: model.CurrencyTRY}, 3: {Value: dec("2.5"), Currency: model.CurrencyTRY}},
		},
	}
}

// R294: consumption and cost are the building bills'; production the plants'.
func TestMonthFromBillsAndProduction(t *testing.T) {
	y := financial.BuildYear(inputs())
	jan := y.Months[0]
	requireDec(t, "1500", jan.ConsumptionKwh, "Σ import of January's bills")
	require.Equal(t, "4600.00", amount(t, jan.Cost, model.CurrencyTRY))
	requireDec(t, "1300", jan.ProductionKwh, "both plants")
	require.Equal(t, "2400.00", amount(t, jan.Revenue, model.CurrencyTRY), "p1 1200 × 2.0; p2 has no tariff")
	require.True(t, jan.RevenuePartial, "p2 produced without a tariff")
	requireDec(t, "-200", jan.OffsetKwh, "1300 − 1500")
	requireDec(t, "200", jan.GridPurchaseKwh, "")
	requireDec(t, "0", jan.GridSaleKwh, "")
	require.Equal(t, "2200.00", amount(t, jan.Net, model.CurrencyTRY), "cost − revenue, always (legacy dropped revenue here)")
}

func TestAMonthWithoutBillsHasNoConsumption(t *testing.T) {
	y := financial.BuildYear(inputs())
	mar := y.Months[2]
	require.Nil(t, mar.ConsumptionKwh, "no bill is missing, never zero")
	require.Empty(t, mar.Cost)
	requireDec(t, "100", mar.ProductionKwh, "")
	require.Nil(t, mar.OffsetKwh, "an offset needs both sides")
	require.Empty(t, mar.Net)
	require.Nil(t, y.Months[5].ProductionKwh, "June: nothing at all")
}

func TestYearTotalsAndCoverage(t *testing.T) {
	y := financial.BuildYear(inputs())
	requireDec(t, "2300", y.Total.ConsumptionKwh, "January + February")
	requireDec(t, "1700", y.Total.ProductionKwh, "1300 + 300 + 100")
	require.Equal(t, 2, y.ConsumptionMonths)
	require.Equal(t, 3, y.ProductionMonths)
	require.Equal(t, "3400.00", amount(t, y.Total.Revenue, model.CurrencyTRY), "2400 + 750 + 250")
}

func TestFinancialNeverAddsCurrencies(t *testing.T) {
	in := inputs()
	in.Bills = append(in.Bills, bill("2026-01", "10", "99", model.CurrencyUSD))
	jan := financial.BuildYear(in).Months[0]
	require.Len(t, jan.Cost, 2)
	require.Equal(t, "99.00", amount(t, jan.Cost, model.CurrencyUSD))
	require.Equal(t, "4600.00", amount(t, jan.Cost, model.CurrencyTRY))
	requireDec(t, "1510", jan.ConsumptionKwh, "kWh do add")
}

// A prosumer's monthly-netted bill has net 0; the page shows its gross import
// and counts its export as production.
func TestProsumerBillShowsImportAndExport(t *testing.T) {
	b := bill("2026-08", "0", "298.90", model.CurrencyTRY)
	b.ActiveImport, b.ActiveExport = dec("180"), dec("420")
	y := financial.BuildYear(financial.YearInputs{Year: 2026, Bills: []model.Bill{b}})
	aug := y.Months[7]
	requireDec(t, "180", aug.ConsumptionKwh, "gross import, not the netted 0")
	requireDec(t, "420", aug.ProductionKwh, "meter export")
	requireDec(t, "240", aug.GridSaleKwh, "export − import, the bill's sale line")
	require.True(t, aug.RevenuePartial, "the export has no feed-in price")
	require.False(t, aug.Measured)
	require.Equal(t, 1, y.BillMonths)
}

// A month without bills falls back to the meters' readings.
func TestMonthWithoutBillsUsesMeterReadings(t *testing.T) {
	in := inputs()
	in.Metered = map[int]financial.Metered{
		1: {ImportKwh: decP("9999")},
		6: {ImportKwh: decP("350"), ExportKwh: decP("500")},
		7: {ImportKwh: decP("200"), ExportKwh: decP("0")},
	}
	y := financial.BuildYear(in)
	requireDec(t, "1500", y.Months[0].ConsumptionKwh, "a billed month ignores the meters")
	require.False(t, y.Months[0].Measured)
	jun := y.Months[5]
	require.True(t, jun.Measured)
	requireDec(t, "350", jun.ConsumptionKwh, "")
	requireDec(t, "500", jun.ProductionKwh, "")
	require.Empty(t, jun.Cost, "no bill, no cost")
	requireDec(t, "150", jun.OffsetKwh, "")
	jul := y.Months[6]
	requireDec(t, "200", jul.ConsumptionKwh, "")
	require.Nil(t, jul.ProductionKwh, "a zero export is no production")
	require.False(t, jul.RevenuePartial)
	require.Equal(t, 2, y.BillMonths)
	require.Equal(t, 4, y.ConsumptionMonths)
	require.True(t, y.Total.Measured)
}

func decP(s string) *decimal.Decimal { v := dec(s); return &v }

// A building without a building bill that month counts its analyzer bills;
// one with a building bill does not count them again.
func TestAnalyzerBillsFillABuildingWithoutABuildingBill(t *testing.T) {
	b1, b2 := uuid.New(), uuid.New()
	mk := func(period string, scope model.BillScope, building uuid.UUID, kwh, cost string) model.Bill {
		b := bill(period, kwh, cost, model.CurrencyTRY)
		b.Scope, b.BuildingID = scope, &building
		return b
	}
	y := financial.BuildYear(financial.YearInputs{Year: 2026, Bills: []model.Bill{
		mk("2026-02", model.BillScopeAnalyzer, b1, "406.849", "1588.27"),
		mk("2026-08", model.BillScopeBuilding, b1, "224", "298.90"),
		mk("2026-08", model.BillScopeAnalyzer, b1, "224", "298.90"),
		mk("2026-08", model.BillScopeAnalyzer, b2, "100", "50"),
		{ID: uuid.New(), Scope: model.BillScopeCompany, PeriodKey: "2026-08", ActiveImport: dec("999"), TotalCost: dec("999"), Currency: model.CurrencyTRY},
	}})
	requireDec(t, "406.849", y.Months[1].ConsumptionKwh, "February: only the analyzer bill")
	require.Equal(t, "1588.27", amount(t, y.Months[1].Cost, model.CurrencyTRY))
	requireDec(t, "324", y.Months[7].ConsumptionKwh, "b1's building bill + b2's analyzer bill; company bill ignored")
	require.Equal(t, "348.90", amount(t, y.Months[7].Cost, model.CurrencyTRY))
	require.Equal(t, 2, y.BillMonths)
}
