package tariff_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func dp(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

func istanbul(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	return loc
}

// fixedTariff is a valid single-time, monomial, non-PTF tariff.
func fixedTariff() model.Tariff {
	return model.Tariff{
		Currency: model.CurrencyTRY, EnergyType: model.EnergyTypeGrid, VoltageLevel: model.VoltageLevelLV,
		UserGroup: model.UserGroupCommercial, PriceType: model.PriceTypeSingleTime, Term: model.TariffTermMonomial,
		SupplyCompany: model.SupplyCompanyIncumbent, SingleTimePrice: dp("2.5"),
		DistributionCost: d("0.5"), ReactivePowerPrice: d("1"), GenerationUsage: model.GenerationUsageNone,
	}
}

// ptfTariff is a valid single-time PTF tariff with every kbk-sourced coefficient.
func ptfTariff() model.Tariff {
	t := fixedTariff()
	t.SingleTimePrice = nil
	t.SupplyCompany = model.SupplyCompanyPrivate
	t.UsePtfYekdem = true
	t.KbkEnergy = dp("1.1")
	t.KbkReactivePower = dp("0.5")
	t.KbkDistributionCostTlPerKwh = dp("0.8")
	return t
}

// seedParams is a hand-built copy of migration 00014's seed row.
func seedParams() model.BillingParameters {
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
		TieringGroups:           map[model.DistributionUserGroup]decimal.Decimal{model.UserGroupResidential: d("8"), model.UserGroupCommercial: d("30")},
		TieringMode:             model.TieringModeSplitAtThreshold,
		TieringVoltageLevels:    []model.VoltageLevel{model.VoltageLevelLV},
		TieringSupplyCompanies:  []model.SupplyCompany{model.SupplyCompanyIncumbent},
		PTFMissingHourTolerance: d("0.02"),
		DemandOverrunMultiplier: d("2"),
		MoneyRoundingMode:       model.MoneyRoundingHalfUp,
	}
}
