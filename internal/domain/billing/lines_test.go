package billing_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func TestSummariseUnitPricesMatchesTheMEPASAllInPrice(t *testing.T) {
	kwh := "kWh"
	line := func(code, qty string, price *decimal.Decimal) model.BillLine {
		return model.BillLine{Code: code, Quantity: dp(qty), Unit: &kwh, UnitPrice: price}
	}
	// Fuga, 18.08–16.09.2026: energy 3.774401 + distribution 2.4794 = 6.253801 TL/kWh.
	got := billing.SummariseUnitPrices([]model.BillLine{
		line(model.BillLineEnergy, "17240.84", dp("3.774401")),
		line(model.BillLineDistribution, "17240.84", dp("2.4794")),
		{Code: model.BillLineVat, RatePct: dp("20")},
	})
	require.Equal(t, "3.774401", got.Energy.String())
	require.Equal(t, "2.4794", got.Distribution.String())
	require.Equal(t, "6.253801", got.Total.String())

	tiers := billing.SummariseUnitPrices([]model.BillLine{
		line(model.BillLineEnergyLowTier, "100", dp("2")), line(model.BillLineEnergyHighTier, "300", dp("3")),
	})
	require.Equal(t, "2.75", tiers.Energy.String(), "weighted by kWh")
	require.Nil(t, tiers.Total, "no distribution line, no all-in price")

	missing := billing.SummariseUnitPrices([]model.BillLine{
		line(model.BillLineEnergy, "100", nil), line(model.BillLineDistribution, "100", dp("2.4794")),
	})
	require.Nil(t, missing.Energy, "an unpriced PTF line has no unit price (R560)")
	require.Nil(t, missing.Total)
	require.Equal(t, "2.4794", missing.Distribution.String())
}
