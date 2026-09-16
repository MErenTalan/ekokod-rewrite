package billing_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func dp(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

func istanbul(t testing.TB) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	return loc
}

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

// baseInput: fixed single-time commercial LV incumbent monomial, 600 kWh over 30 days (untiered).
func baseInput(t testing.TB) billing.Input {
	loc := istanbul(t)
	w := energy.Window{From: time.Date(2026, 1, 1, 0, 0, 0, 0, loc), To: time.Date(2026, 1, 31, 0, 0, 0, 0, loc)}
	return billing.Input{
		PeriodKey: "2026-01", Period: w, Days: 30,
		Tariff: model.Tariff{
			ID: uuid.New(), EffectiveFrom: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Currency: model.CurrencyTRY, EnergyType: model.EnergyTypeGrid, VoltageLevel: model.VoltageLevelLV,
			UserGroup: model.UserGroupCommercial, PriceType: model.PriceTypeSingleTime, Term: model.TariffTermMonomial,
			SupplyCompany: model.SupplyCompanyIncumbent, SingleTimePrice: dp("2.5"), OverusePrice: dp("3.5"),
			DistributionCost: d("0.5"), ReactivePowerPrice: d("1"), VatRate: d("20"), GenerationUsage: model.GenerationUsageNone,
		},
		Params:           seedParams(),
		Quantities:       billing.Quantities{ActiveImport: dp("600"), ReactiveInductive: dp("0"), ReactiveCapacitive: dp("0"), ActiveExport: dp("0")},
		InstalledPowerKw: dp("50"),
	}
}

func compute(t testing.TB, in billing.Input) billing.Invoice {
	t.Helper()
	inv, err := billing.Compute(in)
	require.NoError(t, err)
	return inv
}

// amounts maps line code → amount string.
func amounts(inv billing.Invoice) map[string]string {
	out := map[string]string{}
	for _, l := range inv.Lines {
		out[l.Code] = l.Amount.StringFixed(2)
	}
	return out
}

func line(t testing.TB, inv billing.Invoice, code string) billing.Line {
	t.Helper()
	for _, l := range inv.Lines {
		if l.Code == code {
			return l
		}
	}
	require.Failf(t, "line missing", "no %s line in %v", code, amounts(inv))
	return billing.Line{}
}
