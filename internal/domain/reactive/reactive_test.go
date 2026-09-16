package reactive_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/reactive"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func dp(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

func params() model.BillingParameters {
	return model.BillingParameters{
		EffectiveFrom:               time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		ReactivePenaltyBasis:        model.ReactivePenaltyBasisWholeQuantity,
		ReactiveExemptBelowKw:       dp("9"),
		ReactiveExemptTerms:         []model.TariffTerm{model.TariffTermMonomial},
		ReactiveExemptUserGroups:    []model.DistributionUserGroup{model.UserGroupResidential, model.UserGroupLighting},
		ReactiveGenerationExemptKwh: d("1"),
		ReactiveBands: []model.ReactiveBand{
			{MinKw: d("9"), MaxKw: dp("30"), Inductive: d("0.33"), Capacitive: d("0.20")},
			{MinKw: d("30"), Inductive: d("0.20"), Capacitive: d("0.15")},
		},
	}
}

// base: binomial commercial 50 kW, net 1000, inductive 500 (0.5 > 0.20), capacitive 100 (0.1 ≤ 0.15).
func base() reactive.Input {
	return reactive.Input{
		NetConsumption: d("1000"), Inductive: dp("500"), Capacitive: dp("100"), ActiveExport: dp("0"),
		InstalledPowerKw: dp("50"), Term: model.TariffTermBinomial, UserGroup: model.UserGroupCommercial,
		UnitPrice: dp("2.6455"), Params: params(),
	}
}

func TestReactiveWholeQuantityBasis(t *testing.T) {
	r := reactive.Evaluate(base())
	require.False(t, r.Exempt)
	require.True(t, r.Applied)
	require.Equal(t, "500", r.InductiveKvarhCharged.String())
	require.Equal(t, "1322.75", r.InductivePenalty.String())
	require.Nil(t, r.CapacitiveKvarhCharged)
	require.True(t, r.CapacitivePenalty.IsZero())
	require.Equal(t, "0.5", r.InductiveRatio.String())
	require.Equal(t, "0.2", r.InductiveLimit.String())
	require.Empty(t, r.Missing)
}

func TestReactiveExcessOverLimitBasis(t *testing.T) {
	in := base()
	in.Params.ReactivePenaltyBasis = model.ReactivePenaltyBasisExcessOverLimit
	r := reactive.Evaluate(in)
	require.Equal(t, "300", r.InductiveKvarhCharged.String(), "500 − 0.20 × 1000")
	require.Equal(t, "793.65", r.InductivePenalty.String())
}

func exempt(t *testing.T, in reactive.Input, reason string) {
	t.Helper()
	r := reactive.Evaluate(in)
	require.True(t, r.Exempt)
	require.Equal(t, reason, r.ExemptReason)
	require.False(t, r.Applied)
	require.Nil(t, r.InductiveKvarhCharged)
	require.True(t, r.InductivePenalty.IsZero())
}

func TestReactiveExemptMonomial(t *testing.T) {
	in := base()
	in.Term = model.TariffTermMonomial
	exempt(t, in, reactive.ExemptTerm)
}

func TestReactiveExemptResidential(t *testing.T) {
	in := base()
	in.UserGroup = model.UserGroupResidential
	exempt(t, in, reactive.ExemptUserGroup)
}

func TestReactiveExemptLighting(t *testing.T) {
	in := base()
	in.UserGroup = model.UserGroupLighting
	exempt(t, in, reactive.ExemptUserGroup)
}

func TestReactiveExemptGeneration(t *testing.T) {
	in := base()
	in.ActiveExport, in.ActiveExportKnownSum = dp("1.0001"), d("1.0001")
	exempt(t, in, reactive.ExemptGeneration)
	in.ActiveExport, in.ActiveExportKnownSum = dp("1"), d("1")
	require.False(t, reactive.Evaluate(in).Exempt, "exactly 1 kWh is not generation")
}

func TestReactiveExemptBelow9kW(t *testing.T) {
	in := base()
	in.InstalledPowerKw = dp("8.999")
	exempt(t, in, reactive.ExemptBelowKw)
	in.InstalledPowerKw = dp("9")
	r := reactive.Evaluate(in)
	require.False(t, r.Exempt)
	require.Equal(t, "0.33", r.InductiveLimit.String())
}

func TestReactiveBandsAt30kW(t *testing.T) {
	in := base()
	in.InstalledPowerKw = dp("29.999")
	require.Equal(t, "0.33", reactive.Evaluate(in).InductiveLimit.String())
	in.InstalledPowerKw = dp("30")
	r := reactive.Evaluate(in)
	require.Equal(t, "0.2", r.InductiveLimit.String())
	require.Equal(t, "0.15", r.CapacitiveLimit.String())
}

func TestReactiveZeroNetWithReactiveIsExceeded(t *testing.T) {
	in := base()
	in.NetConsumption = d("0")
	in.Capacitive = dp("0")
	r := reactive.Evaluate(in)
	require.Nil(t, r.InductiveRatio, "R117: ratio stays nil")
	require.Equal(t, "500", r.InductiveKvarhCharged.String())
	require.Nil(t, r.CapacitiveKvarhCharged, "net 0 and reactive 0 is not exceeded")
}

func TestReactiveStrictGreaterThanLimit(t *testing.T) {
	in := base()
	in.Inductive = dp("200") // exactly 0.20
	in.Capacitive = dp("150.0001")
	r := reactive.Evaluate(in)
	require.Nil(t, r.InductiveKvarhCharged)
	require.Equal(t, "150.0001", r.CapacitiveKvarhCharged.String())
}

func TestReactiveMissingInstalledPower(t *testing.T) {
	in := base()
	in.InstalledPowerKw = nil
	r := reactive.Evaluate(in)
	require.False(t, r.Exempt)
	require.False(t, r.Applied)
	require.Equal(t, []string{reactive.MissingInstalledPower}, r.Missing)
}

func TestReactiveNilRegisterSkipsThatSideOnly(t *testing.T) {
	in := base()
	in.Capacitive = nil
	in.Inductive = dp("500")
	r := reactive.Evaluate(in)
	require.Equal(t, []string{reactive.MissingCapacitive}, r.Missing)
	require.Equal(t, "500", r.InductiveKvarhCharged.String())
}

func TestReactiveExemptionDisabledUsesParamsBands(t *testing.T) {
	in := base()
	in.Params.ReactiveExemptBelowKw = nil
	in.Params.ReactiveBands[0].MinKw = d("0")
	in.InstalledPowerKw = dp("5")
	r := reactive.Evaluate(in)
	require.False(t, r.Exempt)
	require.Equal(t, "0.33", r.InductiveLimit.String())
	require.True(t, r.Applied)

	in.Params.ReactiveBands[0].MinKw = d("9")
	r = reactive.Evaluate(in)
	require.Equal(t, []string{reactive.MissingInstalledPower}, r.Missing, "below the first band with no exemption")
}

func TestReactiveGenerationExemptionUsesKnownExportLowerBound(t *testing.T) {
	in := base()
	in.ActiveExport, in.ActiveExportKnownSum, in.ActiveExportPartial = nil, d("5000"), true
	exempt(t, in, reactive.ExemptGeneration)

	in.ActiveExportKnownSum = d("0.5")
	r := reactive.Evaluate(in)
	require.False(t, r.Exempt)
	require.Contains(t, r.Missing, reactive.MissingActiveExport, "I-18: partial export cannot decide the exemption")

	in.ActiveExportPartial = false
	require.NotContains(t, reactive.Evaluate(in).Missing, reactive.MissingActiveExport, "no member reports export at all")
}

func TestReactiveNilUnitPriceReportsQuantityWithZeroPenalty(t *testing.T) {
	in := base()
	in.UnitPrice = nil
	r := reactive.Evaluate(in)
	require.True(t, r.Applied)
	require.Equal(t, "500", r.InductiveKvarhCharged.String())
	require.True(t, r.InductivePenalty.IsZero())
}
