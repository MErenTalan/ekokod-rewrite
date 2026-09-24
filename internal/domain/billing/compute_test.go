package billing_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

func TestSingleRateUntiered(t *testing.T) {
	inv := compute(t, baseInput(t))
	require.Equal(t, map[string]string{"energy": "1500.00", "distribution": "300.00", "vat": "360.00"}, amounts(inv))
	require.False(t, inv.TieredApplied)
	require.Equal(t, "1800", inv.VatBase.String())
	require.Equal(t, "2160", inv.TotalCost.String())
	require.Equal(t, "2.5", inv.EffectiveEnergyPrice.String())
	require.Empty(t, inv.Flags)
}

func residential(t *testing.T, net string) billing.Input {
	in := baseInput(t)
	in.Tariff.UserGroup = model.UserGroupResidential
	in.Tariff.SingleTimePrice, in.Tariff.OverusePrice = dp("2"), dp("3")
	in.Quantities.ActiveImport = dp(net)
	return in
}

func TestTieredResidentialSplit(t *testing.T) {
	inv := compute(t, residential(t, "400")) // 13.33/day > 8
	require.True(t, inv.TieredApplied)
	require.Equal(t, "240", inv.LowTierKwh.String())
	require.Equal(t, "160", inv.HighTierKwh.String())
	require.Equal(t, map[string]string{"energy_low_tier": "480.00", "energy_high_tier": "480.00", "distribution": "200.00", "vat": "232.00"}, amounts(inv))
	require.Equal(t, "1392", inv.TotalCost.String())
	require.Equal(t, "2.4", inv.EffectiveEnergyPrice.String())
}

func TestTieredCommercialSplit(t *testing.T) {
	in := baseInput(t)
	in.Quantities.ActiveImport = dp("1200")
	inv := compute(t, in)
	require.True(t, inv.TieredApplied)
	require.Equal(t, map[string]string{"energy_low_tier": "2250.00", "energy_high_tier": "1050.00", "distribution": "600.00", "vat": "780.00"}, amounts(inv))
}

func TestTieringThresholdOverride(t *testing.T) {
	in := baseInput(t)
	in.Quantities.ActiveImport = dp("400")
	require.False(t, compute(t, in).TieredApplied, "13.3/day is under the commercial 30")
	in.Tariff.OveruseThresholdKwhPerDay = dp("10")
	inv := compute(t, in)
	require.True(t, inv.TieredApplied)
	require.Equal(t, "300", inv.LowTierKwh.String())
	require.Equal(t, "100", inv.HighTierKwh.String())
}

func TestTieringWholeConsumptionSwitchMode(t *testing.T) {
	in := baseInput(t)
	in.Quantities.ActiveImport = dp("1200")
	in.Params.TieringMode = model.TieringModeWholeConsumptionSwitch
	inv := compute(t, in)
	require.True(t, inv.TieredApplied)
	require.Equal(t, map[string]string{"energy_high_tier": "4200.00", "distribution": "600.00", "vat": "960.00"}, amounts(inv))
	require.True(t, inv.LowTierKwh.IsZero())
	require.Equal(t, "1200", inv.HighTierKwh.String())
}

func TestTieringSkippedOutsideVoltageLevels(t *testing.T) {
	in := baseInput(t)
	in.Quantities.ActiveImport = dp("1200")
	in.Tariff.VoltageLevel = model.VoltageLevelMV
	inv := compute(t, in)
	require.False(t, inv.TieredApplied)
	require.Equal(t, "3000.00", amounts(inv)["energy"])
}

func TestTieringSkippedForPrivateSupply(t *testing.T) {
	in := baseInput(t)
	in.Quantities.ActiveImport = dp("1200")
	in.Tariff.SupplyCompany = model.SupplyCompanyPrivate
	inv := compute(t, in)
	require.False(t, inv.TieredApplied)
	require.Equal(t, "3000.00", amounts(inv)["energy"])
}

func TestTieringNilOverusePriceFlagsNotZero(t *testing.T) {
	in := baseInput(t)
	in.Quantities.ActiveImport = dp("1200")
	in.Tariff.OverusePrice = nil
	inv := compute(t, in)
	require.False(t, inv.TieredApplied)
	require.Equal(t, "3000.00", amounts(inv)["energy"], "untiered, never a zero-price high tier")
	require.Equal(t, []billing.Flag{billing.FlagTieringPriceMissing}, inv.Flags)
}

func multiTime(t *testing.T) billing.Input {
	in := residential(t, "600") // 20/day, far above 8
	in.Tariff.PriceType = model.PriceTypeMultiTime
	in.Tariff.SingleTimePrice = nil
	in.Tariff.T1Price, in.Tariff.T2Price, in.Tariff.T3Price = dp("3"), dp("2"), dp("1")
	in.Tariff.OverusePrice = dp("10")
	in.Quantities.T1, in.Quantities.T2, in.Quantities.T3 = dp("300"), dp("200"), dp("100")
	return in
}

func TestMultiTimeNeverTieredNeverAveraged(t *testing.T) {
	inv := compute(t, multiTime(t))
	require.False(t, inv.TieredApplied)
	require.Equal(t, map[string]string{"energy_t1": "900.00", "energy_t2": "400.00", "energy_t3": "100.00", "distribution": "300.00", "vat": "340.00"}, amounts(inv))
}

func TestDistributionChargedOnTotalNotLowTier(t *testing.T) {
	inv := compute(t, residential(t, "400"))
	l := line(t, inv, model.BillLineDistribution)
	require.Equal(t, "400", l.Quantity.String())
	require.Equal(t, "200.00", l.Amount.StringFixed(2))
}

func TestGenerationNone(t *testing.T) {
	in := baseInput(t)
	in.Quantities.ActiveExport = dp("100")
	in.Quantities.ActiveExportKnownSum = d("100")
	inv := compute(t, in)
	require.Equal(t, "600", inv.NetConsumption.String())
	require.NotContains(t, amounts(inv), model.BillLineGenerationCredit)
	require.Empty(t, inv.Flags)
}

func TestGenerationSubtractFromConsumption(t *testing.T) {
	in := residential(t, "500")
	in.Tariff.GenerationUsage = model.GenerationUsageSubtractFromConsumption
	in.Quantities.ActiveExport = dp("100")
	inv := compute(t, in)
	require.Equal(t, "400", inv.NetConsumption.String())
	require.Equal(t, "140", inv.LowTierKwh.String(), "R122: 30×8 − 100")
	require.Equal(t, "260", inv.HighTierKwh.String())
	require.Equal(t, "1060.00", inv.EnergyCost.StringFixed(2))
}

func TestGenerationSubtractFromTotal(t *testing.T) {
	in := baseInput(t)
	in.Tariff.GenerationUsage = model.GenerationUsageSubtractFromTotal
	in.Tariff.GenerationPricePerKwh = dp("1.5")
	in.Quantities.ActiveExport = dp("100")
	inv := compute(t, in)
	require.Equal(t, "600", inv.NetConsumption.String())
	require.Equal(t, "1800", inv.VatBase.String(), "the credit is after VAT")
	require.Equal(t, "150", inv.GenerationCredit.String())
	require.Equal(t, "-150.00", amounts(inv)[model.BillLineGenerationCredit])
	require.Equal(t, "2010", inv.TotalCost.String())

	in.Quantities.ActiveExport = dp("10000")
	require.True(t, compute(t, in).TotalCost.IsZero(), "total clamps at 0")
}

func TestGenerationMultiTimeProRata(t *testing.T) {
	in := multiTime(t)
	in.Tariff.GenerationUsage = model.GenerationUsageSubtractFromConsumption
	in.Quantities.ActiveImport, in.Quantities.ActiveExport = dp("600"), dp("200")
	in.Quantities.T1, in.Quantities.T2, in.Quantities.T3 = dp("250"), dp("150"), dp("100")
	inv := compute(t, in)
	require.Equal(t, "200", line(t, inv, model.BillLineEnergyT1).Quantity.String())
	require.Equal(t, "120", line(t, inv, model.BillLineEnergyT2).Quantity.String())
	require.Equal(t, "80", line(t, inv, model.BillLineEnergyT3).Quantity.String())
	require.Equal(t, "920.00", inv.EnergyCost.StringFixed(2))
}

func TestGenerationExportNilFlags(t *testing.T) {
	in := baseInput(t)
	in.Tariff.GenerationUsage = model.GenerationUsageSubtractFromTotal
	in.Tariff.GenerationPricePerKwh = dp("1.5")
	in.Quantities.ActiveExport = nil
	inv := compute(t, in)
	require.Contains(t, inv.Flags, billing.FlagGenerationDataMissing)
	require.NotContains(t, amounts(inv), model.BillLineGenerationCredit)
}

func binomial(t *testing.T) billing.Input {
	in := baseInput(t)
	in.Tariff.Term = model.TariffTermBinomial
	in.Tariff.ContractedPowerKw, in.Tariff.PowerUnitPrice = dp("100"), dp("40")
	in.Quantities.MaxDemandKw = dp("130")
	return in
}

func TestDemandOverrunUsesMultiplier(t *testing.T) {
	inv := compute(t, binomial(t))
	require.Equal(t, "4000.00", amounts(inv)[model.BillLinePower])
	require.Equal(t, "2400.00", amounts(inv)[model.BillLineDemandOverrun])
	require.True(t, inv.DemandDataAvailable)
	in := binomial(t)
	in.Params.DemandOverrunMultiplier = d("3")
	require.Equal(t, "3600.00", amounts(compute(t, in))[model.BillLineDemandOverrun])
}

func TestDemandNilMeansNoOverrunAndUnavailable(t *testing.T) {
	in := binomial(t)
	in.Quantities.MaxDemandKw = nil
	inv := compute(t, in)
	require.False(t, inv.DemandDataAvailable)
	require.NotContains(t, amounts(inv), model.BillLineDemandOverrun)
	require.Equal(t, "4000.00", amounts(inv)[model.BillLinePower])
}

func TestMonomialHasNoPowerLines(t *testing.T) {
	in := baseInput(t)
	in.Quantities.MaxDemandKw = dp("500")
	in.Tariff.ContractedPowerKw, in.Tariff.PowerUnitPrice = dp("100"), dp("40") // ignored on monomial
	inv := compute(t, in)
	require.NotContains(t, amounts(inv), model.BillLinePower)
	require.NotContains(t, amounts(inv), model.BillLineDemandOverrun)
}

func reactiveInput(t *testing.T) billing.Input {
	in := binomial(t)
	in.Quantities.ActiveImport = dp("1000")
	in.Quantities.MaxDemandKw = dp("90")
	in.Quantities.ReactiveInductive, in.Quantities.ReactiveCapacitive = dp("500"), dp("100")
	in.Tariff.ReactivePowerPrice = d("2")
	return in
}

func TestReactiveAppliedAddsLinesAndVatBase(t *testing.T) {
	inv := compute(t, reactiveInput(t))
	require.True(t, inv.Reactive.Applied)
	a := amounts(inv)
	require.Equal(t, "1000.00", a[model.BillLineReactiveInductive])
	require.NotContains(t, a, model.BillLineReactiveCapacitive)
	require.Equal(t, "1000", inv.ReactivePenalty.String())
	// energy 2600 (tiered: 900×2.5 + 100×3.5) + distribution 500 + power 4000 + reactive 1000
	require.Equal(t, "8100", inv.VatBase.String())
}

func TestReactiveExemptionsProduceNoLines(t *testing.T) {
	cases := map[string]func(in *billing.Input){
		"monomial":    func(in *billing.Input) { in.Tariff.Term = model.TariffTermMonomial },
		"residential": func(in *billing.Input) { in.Tariff.UserGroup = model.UserGroupResidential },
		"lighting":    func(in *billing.Input) { in.Tariff.UserGroup = model.UserGroupLighting },
		"generation": func(in *billing.Input) {
			in.Quantities.ActiveExport, in.Quantities.ActiveExportKnownSum = dp("5"), d("5")
		},
		"sub-9 kW": func(in *billing.Input) { in.InstalledPowerKw = dp("8") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := reactiveInput(t)
			mutate(&in)
			inv := compute(t, in)
			require.True(t, inv.Reactive.Exempt)
			require.NotContains(t, amounts(inv), model.BillLineReactiveInductive)
			require.True(t, inv.ReactivePenalty.IsZero())
			require.Empty(t, inv.Flags)
		})
	}
}

func TestReactiveMissingDataFlags(t *testing.T) {
	in := reactiveInput(t)
	in.InstalledPowerKw = nil
	require.Equal(t, []billing.Flag{billing.FlagInstalledPowerMissing}, compute(t, in).Flags)
	in = reactiveInput(t)
	in.Quantities.ReactiveCapacitive = nil
	require.Equal(t, []billing.Flag{billing.FlagReactiveDataMissing}, compute(t, in).Flags)
}

func TestNamedTaxesAreRoundedPercentOfRoundedEnergy(t *testing.T) {
	in := baseInput(t)
	in.Quantities.ActiveImport = dp("101.7")
	in.Tariff.SingleTimePrice = dp("0.115")
	in.Taxes = []model.TariffTax{{Name: "BTV", Rate: d("5"), SortOrder: 2}, {Name: "TRT", Rate: d("0"), SortOrder: 1}}
	inv := compute(t, in)
	a := amounts(inv)
	require.Equal(t, "11.70", a["energy"])
	require.Equal(t, "0.59", a["tax:BTV"], "5% of the rounded 11.70, not of 11.6955")
	require.Equal(t, "0.59", inv.OtherTaxesCost.String())
	require.Equal(t, "tax:TRT", inv.Lines[2].Code, "taxes in SortOrder, after distribution")
	require.Equal(t, "5", line(t, inv, "tax:BTV").RatePct.String())
}

func TestVatOnRoundedBase(t *testing.T) {
	in := baseInput(t)
	in.Quantities.ActiveImport = dp("33.33")
	in.Tariff.SingleTimePrice = dp("2.345")
	inv := compute(t, in)
	require.Equal(t, "94.83", inv.VatBase.StringFixed(2), "78.16 + 16.67")
	require.Equal(t, "18.97", inv.VatCost.StringFixed(2), "the unrounded base 94.82385 would give 18.96")
}

func TestTotalEqualsVatBasePlusVatMinusCredit(t *testing.T) {
	in := reactiveInput(t)
	in.Tariff.GenerationUsage = model.GenerationUsageSubtractFromTotal
	in.Tariff.GenerationPricePerKwh = dp("0.333")
	in.Quantities.ActiveExport = dp("0.5")
	in.Taxes = []model.TariffTax{{Name: "BTV", Rate: d("5")}}
	inv := compute(t, in)
	sum := decimal.Zero
	for _, l := range inv.Lines {
		if l.Code != model.BillLineVat && l.Code != model.BillLineGenerationCredit {
			sum = sum.Add(l.Amount)
		}
	}
	require.True(t, sum.Equal(inv.VatBase))
	require.True(t, inv.TotalCost.Equal(inv.VatBase.Add(inv.VatCost).Sub(inv.GenerationCredit)))
	require.Equal(t, "0.17", inv.GenerationCredit.String())
}

func TestRoundingModeHalfEvenMatchesIcmalTieRow(t *testing.T) {
	in := baseInput(t)
	in.Quantities.ActiveImport = dp("1")
	in.Tariff.SingleTimePrice = dp("221.30")
	in.Tariff.DistributionCost = d("0")
	in.Taxes = []model.TariffTax{{Name: "BTV", Rate: d("5")}}
	require.Equal(t, "11.07", amounts(compute(t, in))["tax:BTV"])
	in.Params.MoneyRoundingMode = model.MoneyRoundingHalfEven
	inv := compute(t, in)
	require.Equal(t, "11.06", amounts(inv)["tax:BTV"], "CSV line 37")
	require.Equal(t, "232.36", inv.VatBase.StringFixed(2))
	require.Equal(t, "46.47", inv.VatCost.StringFixed(2))
}

func TestNegativeCreditRoundsHalfUpOnAbsoluteValue(t *testing.T) {
	require.Equal(t, "-1.01", billing.Round2(d("-1.005"), model.MoneyRoundingHalfUp).String())
	require.Equal(t, "1.01", billing.Round2(d("1.005"), model.MoneyRoundingHalfUp).String())
	require.Equal(t, "-1", billing.Round2(d("-1.005"), model.MoneyRoundingHalfEven).String())
	require.Equal(t, "-1.02", billing.Round2(d("-1.015"), model.MoneyRoundingHalfEven).String())

	in := baseInput(t)
	in.Tariff.GenerationUsage = model.GenerationUsageSubtractFromTotal
	in.Tariff.GenerationPricePerKwh = dp("1.005")
	in.Quantities.ActiveExport = dp("1")
	require.Equal(t, "-1.01", amounts(compute(t, in))[model.BillLineGenerationCredit])
}

// icmalRow builds a fixed-price invoice whose unit prices are the supplier's
// amount / quantity, so line amounts reproduce the supplier's (I-2).
func icmalRow(t *testing.T, kwh, energy, distribution string) billing.Input {
	in := baseInput(t)
	unit := func(amount, qty string) *decimal.Decimal {
		v := d(amount).DivRound(d(qty), tariff.DivisionScale)
		return &v
	}
	in.Quantities.ActiveImport = dp(kwh)
	in.Tariff.SupplyCompany = model.SupplyCompanyPrivate
	in.Tariff.SingleTimePrice = unit(energy, kwh)
	in.Tariff.DistributionCost = *unit(distribution, kwh)
	in.Taxes = []model.TariffTax{{Name: "BTV", Rate: d("5")}}
	return in
}

func TestIcmalRowAssemblyMatchesSupplierToTheKurus(t *testing.T) {
	withReactive := func(in billing.Input, inductive, charge string) billing.Input {
		in.Params.ReactiveExemptTerms = nil // R118: the icmal penalises monomial rows
		in.Quantities.ReactiveInductive = dp(inductive)
		in.Tariff.ReactivePowerPrice = d(charge).DivRound(d(inductive), tariff.DivisionScale)
		return in
	}
	t.Run("line 18 reactive", func(t *testing.T) {
		inv := compute(t, withReactive(icmalRow(t, "3996.76", "12466.17", "7503.56"), "1542.07", "4079.52"))
		require.Equal(t, "623.31", amounts(inv)["tax:BTV"])
		require.Equal(t, "24672.56", inv.VatBase.StringFixed(2))
		require.Equal(t, "4934.51", inv.VatCost.StringFixed(2))
	})
	t.Run("line 25 reactive", func(t *testing.T) {
		inv := compute(t, withReactive(icmalRow(t, "33324", "106910.63", "62562.81"), "7410.40", "19604.02"))
		require.Equal(t, "194422.99", inv.VatBase.StringFixed(2))
		require.Equal(t, "38884.60", inv.VatCost.StringFixed(2))
	})
	t.Run("line 31 power and overrun", func(t *testing.T) {
		in := icmalRow(t, "169833.84", "552557.29", "214549.90")
		in.Tariff.Term = model.TariffTermBinomial
		in.Tariff.ContractedPowerKw = dp("300")
		unit := d("13011.01").DivRound(d("300"), tariff.DivisionScale)
		in.Tariff.PowerUnitPrice = &unit
		over := d("12282.39").DivRound(unit.Mul(d("2")), tariff.DivisionScale)
		in.Quantities.MaxDemandKw = func() *decimal.Decimal { v := over.Add(d("300")); return &v }()
		in.Quantities.ReactiveInductive, in.Quantities.ReactiveCapacitive = dp("2870.40"), dp("4504.32") // under the limits
		inv := compute(t, in)
		a := amounts(inv)
		require.Equal(t, "13011.01", a[model.BillLinePower])
		require.Equal(t, "12282.39", a[model.BillLineDemandOverrun])
		require.Equal(t, "27627.86", a["tax:BTV"])
		require.Equal(t, "820028.45", inv.VatBase.StringFixed(2))
		require.Equal(t, "164005.69", inv.VatCost.StringFixed(2))
	})
}

func ptfInput(t *testing.T) billing.Input {
	in := baseInput(t)
	in.Quantities.ActiveImport = dp("1000")
	in.Tariff.UsePtfYekdem, in.Tariff.SingleTimePrice = true, nil
	in.Tariff.SupplyCompany = model.SupplyCompanyPrivate
	in.Tariff.KbkEnergy = dp("1.1")
	in.Pricing = &tariff.Pricing{Method: tariff.MethodHourly, EnergyUnitPrice: dp("3.1"), DistributionUnitPrice: dp("0.8"),
		ReactiveUnitPrice: dp("2.6455"), HoursExpected: 744, HoursMatched: 744, Complete: true}
	return in
}

func TestPTFSingleRateUsesPricingEnergyUnitPrice(t *testing.T) {
	inv := compute(t, ptfInput(t))
	require.Equal(t, map[string]string{"energy": "3100.00", "distribution": "800.00", "vat": "780.00"}, amounts(inv))
	require.True(t, inv.PtfYekdemUsed)
	require.Empty(t, inv.Flags)
}

func TestPTFIsNeverTiered(t *testing.T) {
	in := ptfInput(t)
	in.Tariff.SupplyCompany = model.SupplyCompanyIncumbent
	in.Quantities.ActiveImport = dp("100000")
	inv := compute(t, in)
	require.False(t, inv.TieredApplied)
	require.Contains(t, amounts(inv), model.BillLineEnergy)
}

func TestPTFMultiTimeUsesBandPrices(t *testing.T) {
	in := ptfInput(t)
	in.Tariff.PriceType = model.PriceTypeMultiTime
	in.Quantities.T1, in.Quantities.T2, in.Quantities.T3 = dp("500"), dp("300"), dp("200")
	in.Pricing.T1, in.Pricing.T2, in.Pricing.T3 = dp("4"), dp("3"), nil
	inv := compute(t, in)
	a := amounts(inv)
	require.Equal(t, "2000.00", a[model.BillLineEnergyT1])
	require.Equal(t, "900.00", a[model.BillLineEnergyT2])
	require.NotContains(t, a, model.BillLineEnergyT3, "nil band price → no line")
}

func TestPTFNilCoefficientProducesNoLine(t *testing.T) {
	in := ptfInput(t)
	in.Tariff.Term = model.TariffTermBinomial
	in.Tariff.ContractedPowerKw, in.Tariff.PowerUnitPrice = dp("100"), dp("40")
	in.Quantities.ReactiveInductive = dp("900")
	in.Pricing.PowerUnitPrice, in.Pricing.ReactiveUnitPrice = nil, nil
	inv := compute(t, in)
	a := amounts(inv)
	require.True(t, inv.Reactive.Applied, "exceeded")
	require.NotContains(t, a, model.BillLineReactiveInductive)
	require.NotContains(t, a, model.BillLinePower)
}

func TestPTFIncompleteFlags(t *testing.T) {
	in := ptfInput(t)
	in.Pricing.Complete, in.Pricing.Reason = false, tariff.ReasonOverTolerance
	require.Equal(t, []billing.Flag{billing.FlagPTFDataMissing}, compute(t, in).Flags)
}

func TestComputeRejectsNilActiveImport(t *testing.T) {
	in := baseInput(t)
	in.Quantities.ActiveImport = nil
	_, err := billing.Compute(in)
	require.ErrorIs(t, err, billing.ErrInvalidInput)
	in = ptfInput(t)
	in.Pricing = nil
	_, err = billing.Compute(in)
	require.ErrorIs(t, err, billing.ErrInvalidInput)
	in = baseInput(t)
	in.Days = 0
	_, err = billing.Compute(in)
	require.ErrorIs(t, err, billing.ErrInvalidInput)
}

func TestComputeRejectsMultiTimeNilBands(t *testing.T) {
	in := multiTime(t)
	in.Quantities.T2 = nil
	_, err := billing.Compute(in)
	require.ErrorIs(t, err, billing.ErrInvalidInput)
}

func TestExtraChargesEachBasis(t *testing.T) {
	in := binomial(t)
	in.ExtraCharges = []model.TariffExtraCharge{
		{Name: "fixed", Basis: model.ExtraChargeBasisFixedPerPeriod, Amount: d("25"), SortOrder: 5},
		{Name: "kwh", Basis: model.ExtraChargeBasisPerKwh, Amount: d("0.01"), SortOrder: 1},
		{Name: "cap", Basis: model.ExtraChargeBasisPerContractedKw, Amount: d("2"), SortOrder: 2},
		{Name: "peak", Basis: model.ExtraChargeBasisPerMaxDemandKw, Amount: d("1.5"), SortOrder: 3},
		{Name: "discount", Basis: model.ExtraChargeBasisPctOfEnergy, Amount: d("-10"), SortOrder: 4},
	}
	in.Taxes = []model.TariffTax{{Name: "BTV", Rate: d("5")}}
	inv := compute(t, in)
	a := amounts(inv)
	require.Equal(t, "6.00", a["extra:kwh"])
	require.Equal(t, "200.00", a["extra:cap"])
	require.Equal(t, "195.00", a["extra:peak"])
	require.Equal(t, "-150.00", a["extra:discount"])
	require.Equal(t, "25.00", a["extra:fixed"])
	require.Equal(t, "276", inv.ExtraChargesCost.String())
	require.Equal(t, "75.00", a["tax:BTV"], "taxes stay % of energy only")
	// energy 1500 + distribution 300 + power 4000 + overrun 2400 + extras 276 + BTV 75
	require.Equal(t, "8551", inv.VatBase.String())
	codes := []string{}
	for _, l := range inv.Lines {
		codes = append(codes, l.Code)
	}
	require.Equal(t, []string{"energy", "distribution", "power", "demand_overrun", "extra:kwh", "extra:cap", "extra:peak", "extra:discount", "extra:fixed", "tax:BTV", "vat"}, codes)

	in.Quantities.MaxDemandKw = nil
	inv = compute(t, in)
	require.NotContains(t, amounts(inv), "extra:peak")
	require.False(t, inv.DemandDataAvailable)
}

func TestToModelMapsFlagsLinesAndHourly(t *testing.T) {
	in := ptfInput(t)
	in.Pricing.Complete = false
	in.Pricing.HoursMatched, in.Pricing.HoursMissingConsumption = 700, 44
	in.Pricing.Hourly = []tariff.HourDetail{{Kwh: d("1"), PTF: d("1000"), Yekdem: d("500"), Kbk: d("1.1"), UnitPrice: d("1.65"), Cost: d("1.65")}}
	in.Quantities.IndexEnd = map[string]*decimal.Decimal{"active_import": dp("123.45"), "t1_import": nil}
	inv := compute(t, in)
	b, lines, hourly := billing.ToModel(inv, model.BillScopeAnalyzer)
	require.Equal(t, model.BillStatusFlagged, b.Status)
	require.Equal(t, "ptf_data_missing", *b.FlagReason)
	require.Equal(t, int32(744), *b.PtfHoursExpected)
	require.Equal(t, int32(44), *b.PtfHoursMissing)
	require.Equal(t, int32(44), *b.ConsumptionHoursMissing)
	require.JSONEq(t, `{"active_import":"123.45","t1_import":null}`, string(b.IndexEnd))
	require.JSONEq(t, `{}`, string(b.IndexStart))
	require.Len(t, lines, 3)
	require.Equal(t, int16(2), lines[2].SortOrder)
	require.Equal(t, "vat", lines[2].Label)
	require.Len(t, hourly, 1)
	require.Equal(t, *inv.TariffID, *b.TariffID)

	b, _, _ = billing.ToModel(compute(t, baseInput(t)), model.BillScopeBuilding)
	require.Equal(t, model.BillStatusIssued, b.Status)
	require.Nil(t, b.FlagReason)
	require.Nil(t, b.PtfHoursExpected)
}
