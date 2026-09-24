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
	return model.Bill{ID: uuid.New(), Scope: model.BillScopeBuilding, PeriodKey: period, NetConsumption: dec(net), TotalCost: dec(cost), Currency: c}
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
	requireDec(t, "1500", jan.ConsumptionKwh, "Σ net consumption of January's bills")
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
