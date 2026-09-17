package tariff_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

func TestValidateParamsDefaultsFromMigrationAreValid(t *testing.T) {
	require.NoError(t, tariff.ValidateParams(seedParams()))
}

func TestValidateParamsRejectsGapInBands(t *testing.T) {
	cases := map[string]func(p *model.BillingParameters){
		"gap between bands":        func(p *model.BillingParameters) { p.ReactiveBands[1].MinKw = d("31") },
		"first band not at exempt": func(p *model.BillingParameters) { p.ReactiveExemptBelowKw = nil },
		"bounded last band":        func(p *model.BillingParameters) { p.ReactiveBands[1].MaxKw = dp("100") },
		"zero limit":               func(p *model.BillingParameters) { p.ReactiveBands[0].Capacitive = d("0") },
		"tolerance above 1":        func(p *model.BillingParameters) { p.PTFMissingHourTolerance = d("1.01") },
		"negative multiplier":      func(p *model.BillingParameters) { p.DemandOverrunMultiplier = d("-1") },
		"zero tiering threshold":   func(p *model.BillingParameters) { p.TieringGroups[model.UserGroupResidential] = d("0") },
		"bad mode":                 func(p *model.BillingParameters) { p.MoneyRoundingMode = "bankers" },
		"bad supply company":       func(p *model.BillingParameters) { p.TieringSupplyCompanies = []model.SupplyCompany{"x"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := seedParams()
			mutate(&p)
			require.ErrorIs(t, tariff.ValidateParams(p), tariff.ErrInvalidParams)
		})
	}
	p := seedParams()
	p.ReactiveExemptBelowKw = nil
	p.ReactiveBands[0].MinKw = d("0")
	require.NoError(t, tariff.ValidateParams(p), "exemption disabled: bands start at 0")
}
