package billing_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

func TestBuildingInvoiceIsNotSumOfAnalyzerInvoicesWhenTiered(t *testing.T) {
	a := residential(t, "200") // 6.67/day each, under 8
	b := residential(t, "150")
	invA, invB := compute(t, a), compute(t, b)
	require.False(t, invA.TieredApplied)
	require.False(t, invB.TieredApplied)

	building := residential(t, "0")
	building.Quantities = billing.AggregateQuantities([]billing.Quantities{a.Quantities, b.Quantities})
	inv := compute(t, building)
	require.True(t, inv.TieredApplied, "350 kWh / 30 days = 11.67 > 8")
	require.Equal(t, "350", inv.NetConsumption.String())
	require.False(t, inv.TotalCost.Equal(invA.TotalCost.Add(invB.TotalCost)))
}

func TestAggregateNilRegisterPropagates(t *testing.T) {
	q := billing.AggregateQuantities([]billing.Quantities{
		{ActiveImport: dp("10"), ReactiveInductive: dp("1"), IndexEnd: map[string]*decimal.Decimal{"active_import": dp("100"), "t1_import": nil}},
		{ActiveImport: dp("5"), ReactiveInductive: nil, IndexEnd: map[string]*decimal.Decimal{"active_import": dp("50"), "t1_import": dp("3")}},
	})
	require.Equal(t, "15", q.ActiveImport.String())
	require.Nil(t, q.ReactiveInductive)
	require.Equal(t, "150", q.IndexEnd["active_import"].String())
	require.Contains(t, q.IndexEnd, "t1_import")
	require.Nil(t, q.IndexEnd["t1_import"])
	require.Nil(t, billing.AggregateQuantities(nil).ActiveImport)
}

func TestAggregateActiveExportKnownSumIgnoresNilMembers(t *testing.T) {
	q := billing.AggregateQuantities([]billing.Quantities{
		{ActiveImport: dp("1"), ActiveExport: nil},
		{ActiveImport: dp("1"), ActiveExport: dp("5000"), ActiveExportKnownSum: d("5000")},
	})
	require.Nil(t, q.ActiveExport)
	require.Equal(t, "5000", q.ActiveExportKnownSum.String())
	require.True(t, q.ActiveExportPartial)

	none := billing.AggregateQuantities([]billing.Quantities{{ActiveImport: dp("1")}, {ActiveImport: dp("1")}})
	require.False(t, none.ActiveExportPartial, "no member reports export")
}

func TestAggregateMaxDemandOnlyForSingleMember(t *testing.T) {
	one := billing.AggregateQuantities([]billing.Quantities{{ActiveImport: dp("1"), MaxDemandKw: dp("40")}})
	require.Equal(t, "40", one.MaxDemandKw.String())
	two := billing.AggregateQuantities([]billing.Quantities{{ActiveImport: dp("1"), MaxDemandKw: dp("40")}, {ActiveImport: dp("1"), MaxDemandKw: dp("30")}})
	require.Nil(t, two.MaxDemandKw)
	require.Nil(t, billing.SumInstalledPower([]*decimal.Decimal{dp("10"), nil}))
	require.Equal(t, "25", billing.SumInstalledPower([]*decimal.Decimal{dp("10"), dp("15")}).String())
	require.Nil(t, billing.SumInstalledPower(nil))
}

func TestAggregateHoursRequiresEveryMember(t *testing.T) {
	h0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h1, h2 := h0.Add(time.Hour), h0.Add(2*time.Hour)
	sum, missing := billing.AggregateHours([][]tariff.HourConsumption{
		{{Hour: h1, Kwh: d("2")}, {Hour: h0, Kwh: d("1")}},
		{{Hour: h0, Kwh: d("3")}, {Hour: h2, Kwh: d("4")}},
	})
	require.Equal(t, []tariff.HourConsumption{{Hour: h0, Kwh: d("4")}}, sum)
	require.Equal(t, []time.Time{h1, h2}, missing)
}

func TestCombineCompanyMergesLinesByCodeAndRate(t *testing.T) {
	a := baseInput(t)
	a.Taxes = []model.TariffTax{{Name: "BTV", Rate: d("5")}}
	b := baseInput(t)
	b.Quantities.ActiveImport = dp("300")
	b.Tariff.SingleTimePrice = dp("3")
	b.Taxes = []model.TariffTax{{Name: "BTV", Rate: d("1")}}
	b.Period.To = b.Period.To.AddDate(0, 0, 14)
	invA, invB := compute(t, a), compute(t, b)
	c, err := billing.CombineCompany("2026-01", []billing.Invoice{invA, invB})
	require.NoError(t, err)

	require.Equal(t, "900", c.NetConsumption.String())
	require.True(t, c.TotalCost.Equal(invA.TotalCost.Add(invB.TotalCost)))
	require.True(t, c.VatCost.Equal(invA.VatCost.Add(invB.VatCost)), "R115: VAT summed, not recomputed")
	require.Nil(t, c.TariffID)
	require.True(t, c.Period.To.Equal(b.Period.To), "hull of the building periods")

	energy := line(t, c, model.BillLineEnergy)
	require.Equal(t, "2400.00", energy.Amount.StringFixed(2))
	require.Equal(t, "900", energy.Quantity.String())
	require.Nil(t, energy.UnitPrice, "M-7: differing unit prices merge to nil")
	require.Equal(t, "0.5", line(t, c, model.BillLineDistribution).UnitPrice.String(), "equal unit prices survive")
	btv := 0
	for _, l := range c.Lines {
		if l.Code == "tax:BTV" {
			btv++
		}
	}
	require.Equal(t, 2, btv, "BTV at 5% and 1% stay separate lines")
}

func TestCombineCompanyRejectsMixedCurrency(t *testing.T) {
	a := compute(t, baseInput(t))
	usd := baseInput(t)
	usd.Tariff.Currency = model.CurrencyUSD
	_, err := billing.CombineCompany("2026-01", []billing.Invoice{a, compute(t, usd)})
	require.ErrorIs(t, err, billing.ErrInvalidInput)
	_, err = billing.CombineCompany("2026-01", nil)
	require.ErrorIs(t, err, billing.ErrInvalidInput)
}
