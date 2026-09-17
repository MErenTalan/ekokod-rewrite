package tariff_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

func fields(t *testing.T, err error) map[string]string {
	t.Helper()
	var ve *tariff.ValidationError
	require.ErrorAs(t, err, &ve)
	return ve.Fields
}

func TestValidateAcceptsValidTariffAndSetsVat(t *testing.T) {
	got, err := tariff.Validate(tariff.Draft{Tariff: fixedTariff(), VatRate: dp("20")})
	require.NoError(t, err)
	require.Equal(t, "20", got.VatRate.String())
	require.Equal(t, model.PriceSourceKbk, got.PowerPriceSource)

	_, err = tariff.Validate(tariff.Draft{Tariff: ptfTariff(), VatRate: dp("20")})
	require.NoError(t, err)
}

func TestValidateRejectsMissingVatRate(t *testing.T) {
	_, err := tariff.Validate(tariff.Draft{Tariff: fixedTariff()})
	require.Equal(t, map[string]string{"vat_rate": tariff.CodeRequired}, fields(t, err))
}

func TestValidateCollectsEveryFieldError(t *testing.T) {
	tr := fixedTariff()
	tr.SingleTimePrice = nil
	tr.GenerationUsage = model.GenerationUsageSubtractFromTotal
	tr.Currency = "XXX"
	_, err := tariff.Validate(tariff.Draft{Tariff: tr, Taxes: []model.TariffTax{{Name: "", Rate: d("-1")}},
		ManualYekdem: []model.TariffManualYekdem{{Year: 2026, Month: 13}, {Year: 2026, Month: 1}, {Year: 2026, Month: 1}}})
	require.Equal(t, map[string]string{
		"vat_rate": tariff.CodeRequired, "single_time_price": tariff.CodeRequired, "generation_price_per_kwh": tariff.CodeRequired,
		"currency": tariff.CodeInvalid, "taxes[0].name": tariff.CodeRequired, "taxes[0].rate": tariff.CodeNegative,
		"manual_yekdem[0].month": tariff.CodeOutOfRange, "manual_yekdem[2]": tariff.CodeDuplicate,
	}, fields(t, err))
}

// TestValidateRejectsNilTaxRate: model.TariffTax.Rate cannot be nil, so the
// legacy `?? 1` default is unrepresentable; a negative rate is what remains.
func TestValidateRejectsNilTaxRate(t *testing.T) {
	_, err := tariff.Validate(tariff.Draft{Tariff: fixedTariff(), VatRate: dp("20"), Taxes: []model.TariffTax{{Name: "BTV", Rate: d("-0.01")}}})
	require.Equal(t, map[string]string{"taxes[0].rate": tariff.CodeNegative}, fields(t, err))
}

func TestValidateBinomialNeedsContractAndPrice(t *testing.T) {
	tr := fixedTariff()
	tr.Term = model.TariffTermBinomial
	_, err := tariff.Validate(tariff.Draft{Tariff: tr, VatRate: dp("20")})
	require.Equal(t, map[string]string{"contracted_power_kw": tariff.CodeRequired, "power_unit_price": tariff.CodeRequired}, fields(t, err))
	tr.ContractedPowerKw, tr.PowerUnitPrice = dp("400"), dp("43.37")
	_, err = tariff.Validate(tariff.Draft{Tariff: tr, VatRate: dp("20")})
	require.NoError(t, err)
}

func TestValidateMonomialRejectsPowerFields(t *testing.T) {
	tr := fixedTariff()
	tr.ContractedPowerKw, tr.PowerUnitPrice = dp("400"), dp("43.37")
	_, err := tariff.Validate(tariff.Draft{Tariff: tr, VatRate: dp("20")})
	require.Equal(t, map[string]string{"contracted_power_kw": tariff.CodeMustBeEmpty, "power_unit_price": tariff.CodeMustBeEmpty}, fields(t, err))
}

func TestValidatePtfNeedsTRYAndEnergyKbk(t *testing.T) {
	tr := ptfTariff()
	tr.KbkEnergy = nil
	tr.Currency = model.CurrencyUSD
	_, err := tariff.Validate(tariff.Draft{Tariff: tr, VatRate: dp("20")})
	require.Equal(t, map[string]string{"kbk_energy": tariff.CodeRequired, "currency": tariff.CodeMustBeTRY}, fields(t, err))
}

func TestValidateMultiTimePtfNeedsAllBandKbks(t *testing.T) {
	tr := ptfTariff()
	tr.PriceType = model.PriceTypeMultiTime
	tr.KbkT1, tr.KbkT2 = dp("1"), dp("1")
	_, err := tariff.Validate(tariff.Draft{Tariff: tr, VatRate: dp("20")})
	require.Equal(t, map[string]string{"kbk_t3": tariff.CodeRequired}, fields(t, err))
	tr.KbkT3 = dp("1")
	_, err = tariff.Validate(tariff.Draft{Tariff: tr, VatRate: dp("20")})
	require.NoError(t, err, "a multi-time PTF tariff needs no fixed band prices")

	fixed := fixedTariff()
	fixed.PriceType = model.PriceTypeMultiTime
	fixed.T1Price = dp("1")
	_, err = tariff.Validate(tariff.Draft{Tariff: fixed, VatRate: dp("20")})
	require.Equal(t, map[string]string{"t2_price": tariff.CodeRequired, "t3_price": tariff.CodeRequired}, fields(t, err))
}

func TestValidateBinomialPtfKbkNeedsPowerPrice(t *testing.T) {
	tr := ptfTariff()
	tr.Term = model.TariffTermBinomial
	tr.ContractedPowerKw = dp("400")
	_, err := tariff.Validate(tariff.Draft{Tariff: tr, VatRate: dp("20")})
	require.Equal(t, map[string]string{"kbk_power_price": tariff.CodeRequired}, fields(t, err))
	tr.PowerPriceSource = model.PriceSourceFixed
	_, err = tariff.Validate(tariff.Draft{Tariff: tr, VatRate: dp("20")})
	require.Equal(t, map[string]string{"power_unit_price": tariff.CodeRequired}, fields(t, err))
}

func TestValidateRejectsPtfKbkDistributionOrReactiveWithoutCoefficient(t *testing.T) {
	tr := ptfTariff()
	tr.KbkDistributionCostTlPerKwh, tr.KbkReactivePower = nil, nil
	_, err := tariff.Validate(tariff.Draft{Tariff: tr, VatRate: dp("20")})
	require.Equal(t, map[string]string{"kbk_distribution_cost_tl_per_kwh": tariff.CodeRequired, "kbk_reactive_power": tariff.CodeRequired}, fields(t, err))
	tr.DistributionPriceSource, tr.ReactivePriceSource = model.PriceSourceFixed, model.PriceSourceFixed
	_, err = tariff.Validate(tariff.Draft{Tariff: tr, VatRate: dp("20")})
	require.NoError(t, err)
}

func TestValidateRejectsTieringThresholdWithNilOverusePrice(t *testing.T) {
	tr := fixedTariff()
	tr.OveruseThresholdKwhPerDay = dp("30")
	_, err := tariff.Validate(tariff.Draft{Tariff: tr, VatRate: dp("20")})
	require.Equal(t, map[string]string{"overuse_price": tariff.CodeRequired}, fields(t, err))
}

func TestValidateExtraChargePerContractedKwNeedsContractedPower(t *testing.T) {
	charges := []model.TariffExtraCharge{
		{Name: "cap", Basis: model.ExtraChargeBasisPerContractedKw, Amount: d("1")},
		{Name: "", Basis: "bogus", Amount: d("-1")},
	}
	_, err := tariff.Validate(tariff.Draft{Tariff: fixedTariff(), VatRate: dp("20"), ExtraCharges: charges})
	require.Equal(t, map[string]string{"extra_charges[0].basis": tariff.CodeRequired, "extra_charges[1].name": tariff.CodeRequired, "extra_charges[1].basis": tariff.CodeInvalid}, fields(t, err))
}
