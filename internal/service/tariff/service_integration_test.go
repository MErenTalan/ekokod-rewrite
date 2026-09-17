//go:build integration

package tariff_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff/icmal"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	tariffsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

type env struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	tenant testfixtures.Tenant
	svc    *tariffsvc.Service
}

func newEnv(t *testing.T, seed int64) env {
	t.Helper()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	svc, err := tariffsvc.New(tariffsvc.Deps{
		Tariffs: postgres.NewTariffRepository(pool), Templates: postgres.NewTariffTemplateRepository(pool),
		Buildings: postgres.NewBuildingRepository(pool), Analyzers: postgres.NewAnalyzerRepository(pool),
		Icmal: postgres.NewIcmalRepository(pool), Prices: postgres.NewPriceRepository(pool), Params: postgres.NewBillingParameterRepository(pool),
		Clock: clock.NewFake(time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)), Log: testfixtures.DiscardLogger(),
	})
	require.NoError(t, err)
	return env{ctx: ctx, pool: pool, tenant: testfixtures.NewTenant(t, ctx, pool, seed), svc: svc}
}

func dp(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

func fixedInput(building *uuid.UUID) tariffsvc.Input {
	return tariffsvc.Input{
		Tariff: model.Tariff{BuildingID: building, EffectiveFrom: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC), Currency: model.CurrencyTRY,
			EnergyType: model.EnergyTypeGrid, VoltageLevel: model.VoltageLevelMV, UserGroup: model.UserGroupCommercial,
			PriceType: model.PriceTypeSingleTime, Term: model.TariffTermBinomial, SupplyCompany: model.SupplyCompanyIncumbent,
			SingleTimePrice: dp("2.5"), DistributionCost: decimal.RequireFromString("0.5"), ReactivePowerPrice: decimal.RequireFromString("1"),
			ContractedPowerKw: dp("400"), PowerUnitPrice: dp("43.37"), GenerationUsage: model.GenerationUsageNone},
		VatRate: dp("20"),
		Taxes:   []model.TariffTax{{Name: "BTV", Rate: decimal.NewFromInt(5)}},
	}
}

func (e env) tariffCount(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, e.pool.QueryRow(e.ctx, `select count(*) from tariffs`).Scan(&n))
	return n
}

func TestCreateMultiTimePTFTariff(t *testing.T) {
	t.Parallel()
	e := newEnv(t, 8001)
	b := e.tenant.Buildings[0].ID
	in := fixedInput(&b)
	in.Tariff.PriceType, in.Tariff.Term, in.Tariff.SupplyCompany = model.PriceTypeMultiTime, model.TariffTermMonomial, model.SupplyCompanyPrivate
	in.Tariff.SingleTimePrice, in.Tariff.ContractedPowerKw, in.Tariff.PowerUnitPrice = nil, nil, nil
	in.Tariff.UsePtfYekdem, in.Tariff.KbkEnergy = true, dp("1.1")
	in.Tariff.KbkT1, in.Tariff.KbkT2, in.Tariff.KbkT3 = dp("1.2"), dp("1"), dp("0.8")
	in.Tariff.ReactivePriceSource, in.Tariff.DistributionPriceSource = model.PriceSourceFixed, model.PriceSourceFixed
	def, err := e.svc.Create(e.ctx, e.tenant.Scope, in)
	require.NoError(t, err)
	require.Nil(t, def.Tariff.T1Price)
	require.Equal(t, "1.2", def.Tariff.KbkT1.String())
}

func TestCreatePersistsTaxesExtraChargesManualYekdem(t *testing.T) {
	t.Parallel()
	e := newEnv(t, 8002)
	b := e.tenant.Buildings[0].ID
	in := fixedInput(&b)
	in.ExtraCharges = []model.TariffExtraCharge{{Name: "cap", Basis: model.ExtraChargeBasisPerContractedKw, Amount: decimal.NewFromInt(2)}}
	in.ManualYekdem = []model.TariffManualYekdem{{Year: 2026, Month: 1, Value: decimal.NewFromInt(700)}}
	def, err := e.svc.Create(e.ctx, e.tenant.Scope, in)
	require.NoError(t, err)
	got, err := e.svc.Get(e.ctx, e.tenant.Scope, def.Tariff.ID)
	require.NoError(t, err)
	require.Len(t, got.Taxes, 1)
	require.Len(t, got.ExtraCharges, 1)
	require.Len(t, got.ManualYekdem, 1)
	require.Equal(t, "20", got.Tariff.VatRate.String())

	in.Tariff.SingleTimePrice = dp("3")
	in.Taxes = nil
	updated, err := e.svc.Update(e.ctx, e.tenant.Scope, def.Tariff.ID, in)
	require.NoError(t, err)
	require.Equal(t, "3", updated.Tariff.SingleTimePrice.String())
	require.Empty(t, updated.Taxes)
	require.NoError(t, e.svc.Delete(e.ctx, e.tenant.Scope, def.Tariff.ID))
	_, err = e.svc.Get(e.ctx, e.tenant.Scope, def.Tariff.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestApplicableFallsBackToCompanyWideTariff(t *testing.T) {
	t.Parallel()
	e := newEnv(t, 8003)
	b1 := e.tenant.Buildings[1].ID
	require.NoError(t, postgres.NewTariffRepository(e.pool).SoftDelete(e.ctx, e.tenant.AdminScope, e.tenant.Tariffs[1].ID, time.Now()))
	wide := fixedInput(nil)
	wide.Tariff.EffectiveFrom = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	created, err := e.svc.Create(e.ctx, e.tenant.AdminScope, wide)
	require.NoError(t, err)
	def, err := e.svc.Applicable(e.ctx, e.tenant.AdminScope, b1, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, created.Tariff.ID, def.Tariff.ID)
	require.Len(t, def.Taxes, 1)
	current, err := e.svc.CurrentForBuildings(e.ctx, e.tenant.AdminScope, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, created.Tariff.ID, current[b1].Tariff.ID)
	require.Equal(t, e.tenant.Tariffs[0].ID, current[e.tenant.Buildings[0].ID].Tariff.ID)
}

func TestTemplatePayloadValidatedOnWrite(t *testing.T) {
	t.Parallel()
	e := newEnv(t, 8004)
	bad := fixedInput(nil)
	bad.VatRate = nil
	_, err := e.svc.CreateTemplate(e.ctx, e.tenant.AdminScope, "no vat", nil, bad, false)
	var ve *tariff.ValidationError
	require.ErrorAs(t, err, &ve)
	list, err := e.svc.ListTemplates(e.ctx, e.tenant.AdminScope, store.TariffTemplateFilter{})
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestApplyTemplateCarriesPowerAndTaxes(t *testing.T) {
	t.Parallel()
	e := newEnv(t, 8005)
	tpl, err := e.svc.CreateTemplate(e.ctx, e.tenant.AdminScope, "OG binomial", nil, fixedInput(nil), true)
	require.NoError(t, err)
	ids := []uuid.UUID{e.tenant.Buildings[0].ID, e.tenant.Buildings[1].ID}
	defs, err := e.svc.ApplyTemplate(e.ctx, e.tenant.AdminScope, tpl.ID, ids, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, defs, 2)
	for i, d := range defs {
		require.Equal(t, ids[i], *d.Tariff.BuildingID)
		require.Equal(t, "400", d.Tariff.ContractedPowerKw.String())
		require.Equal(t, "43.37", d.Tariff.PowerUnitPrice.String())
		require.Len(t, d.Taxes, 1)
		require.True(t, d.Tariff.EffectiveFrom.Equal(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)))
	}
}

func TestBulkAssignIsAllOrNothing(t *testing.T) {
	t.Parallel()
	e := newEnv(t, 8006)
	other := testfixtures.NewTenant(t, e.ctx, e.pool, 8106)
	before := e.tariffCount(t)
	_, err := e.svc.BulkAssign(e.ctx, e.tenant.AdminScope, fixedInput(nil), []uuid.UUID{e.tenant.Buildings[0].ID, other.Buildings[0].ID})
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Equal(t, before, e.tariffCount(t), "nothing written")
	defs, err := e.svc.BulkAssign(e.ctx, e.tenant.AdminScope, fixedInput(nil), []uuid.UUID{e.tenant.Buildings[0].ID, e.tenant.Buildings[1].ID})
	require.NoError(t, err, "positive control")
	require.Len(t, defs, 2)
	require.Equal(t, before+2, e.tariffCount(t))
}

// icmalEnv links two analyzers to fixture ETSO codes and stores Nov/Dec 2025 market data (base 2.5 / 2.6).
func icmalEnv(t *testing.T, seed int64) (env, []byte) {
	t.Helper()
	e := newEnv(t, seed)
	for etso, analyzer := range map[string]uuid.UUID{"40ZTEST000000030": e.tenant.Analyzers[0].ID, "40ZTEST000000009": e.tenant.Analyzers[2].ID} {
		_, err := e.pool.Exec(e.ctx, `update analyzers set etso_code = $1 where id = $2`, etso, analyzer)
		require.NoError(t, err)
	}
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	market := admin.NewMarketDataRepository(e.pool)
	var prices []model.MarketPrice
	for ts := time.Date(2025, 11, 1, 0, 0, 0, 0, loc); ts.Before(time.Date(2026, 1, 1, 0, 0, 0, 0, loc)); ts = ts.Add(time.Hour) {
		ptf := int64(2000)
		if ts.Month() == time.December {
			ptf = 2100
		}
		prices = append(prices, model.MarketPrice{Ts: ts.UTC(), PTF: decimal.NewFromInt(ptf), FetchedAt: time.Now().UTC()})
	}
	_, err = market.UpsertHourlyPrices(e.ctx, prices)
	require.NoError(t, err)
	_, err = market.UpsertYekdem(e.ctx, []model.YekdemMonthly{{Year: 2025, Month: 11, Value: decimal.NewFromInt(500), FetchedAt: time.Now()}, {Year: 2025, Month: 12, Value: decimal.NewFromInt(500), FetchedAt: time.Now()}})
	require.NoError(t, err)
	content, err := os.ReadFile("../../domain/tariff/icmal/testdata/ck_icmal_2025_11_12_anonymised.csv")
	require.NoError(t, err)
	return e, content
}

func analysis(as []icmal.Analysis, etso string) icmal.Analysis {
	for _, a := range as {
		if a.EtsoCode == etso {
			return a
		}
	}
	return icmal.Analysis{}
}

func TestImportRealFixtureAnalysesWithoutWritingTariffs(t *testing.T) {
	t.Parallel()
	e, content := icmalEnv(t, 8007)
	b1 := e.tenant.Buildings[1].ID
	contract := fixedInput(&b1) // 400 kW in force during the icmal periods
	contract.Tariff.EffectiveFrom = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := e.svc.Create(e.ctx, e.tenant.AdminScope, contract)
	require.NoError(t, err)
	before := e.tariffCount(t)
	res, err := e.svc.Import(e.ctx, e.tenant.AdminScope, e.tenant.Users[model.UserRoleCompanyAdmin].ID, "icmal.csv", content)
	require.NoError(t, err)
	require.Equal(t, before, e.tariffCount(t), "import writes no tariff")
	require.Equal(t, tariffsvc.ImportStatusAnalysed, res.Import.Status)
	require.Equal(t, int32(45), res.Import.RowCount)
	require.Len(t, res.Analyses, 37)
	require.Len(t, res.Unmatched, 35)

	overrun := analysis(res.Analyses, "40ZTEST000000030")
	require.Equal(t, "43.3700", overrun.PowerUnitPrice.Value.StringFixed(4), "no contract in Nov 2025: (13011.01 + 12282.39/2) / 441.6 (C-2)")
	contracted := analysis(res.Analyses, "40ZTEST000000009")
	require.Equal(t, "43.3700", contracted.PowerUnitPrice.Value.StringFixed(4), "17348.01 / the building tariff's 400 kW (C-2)")
	require.NotNil(t, overrun.EnergyKbk.Value, "base prices came from stored PTF + YEKDEM")

	rows, err := postgres.NewIcmalRepository(e.pool).ListRows(e.ctx, e.tenant.AdminScope, res.Import.ID, store.Page{Limit: 100})
	require.NoError(t, err)
	require.Len(t, rows, 45)
	matched := 0
	for _, r := range rows {
		if r.BuildingID != nil {
			matched++
		}
	}
	require.Equal(t, 2, matched)

	again, err := e.svc.GetImport(e.ctx, e.tenant.AdminScope, res.Import.ID)
	require.NoError(t, err)
	require.Len(t, again.Analyses, 37)
	require.Equal(t, res.Unmatched, again.Unmatched)
}

func TestApplyImportRequiresConfirmationAndEffectiveFrom(t *testing.T) {
	t.Parallel()
	e, content := icmalEnv(t, 8008)
	res, err := e.svc.Import(e.ctx, e.tenant.AdminScope, e.tenant.Users[model.UserRoleCompanyAdmin].ID, "icmal.csv", content)
	require.NoError(t, err)
	b0 := e.tenant.Buildings[0].ID
	base := fixedInput(&b0)
	for name, confs := range map[string][]tariffsvc.ApplyConfirmation{
		"none":              nil,
		"no effective_from": {{BuildingID: b0, EtsoCode: "40ZTEST000000030", Base: &base}},
		"unknown ETSO":      {{BuildingID: b0, EtsoCode: "NOPE", EffectiveFrom: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)}},
	} {
		_, err := e.svc.ApplyImport(e.ctx, e.tenant.AdminScope, res.Import.ID, confs)
		require.ErrorIs(t, err, tariffsvc.ErrInvalidRequest, name)
	}
}

func TestApplyImportCreatesNewVersionWithSources(t *testing.T) {
	t.Parallel()
	e, content := icmalEnv(t, 8009)
	// The base carries a KBK power coefficient, so only an explicit fixed source keeps the derived flat price.
	tariffs := postgres.NewTariffRepository(e.pool)
	baseTariff, err := tariffs.Get(e.ctx, e.tenant.AdminScope, e.tenant.Tariffs[0].ID)
	require.NoError(t, err)
	baseTariff.KbkPowerPrice = dp("2")
	_, err = tariffs.Update(e.ctx, e.tenant.AdminScope, baseTariff)
	require.NoError(t, err)
	res, err := e.svc.Import(e.ctx, e.tenant.AdminScope, e.tenant.Users[model.UserRoleCompanyAdmin].ID, "icmal.csv", content)
	require.NoError(t, err)
	b0 := e.tenant.Buildings[0].ID
	effective := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	defs, err := e.svc.ApplyImport(e.ctx, e.tenant.AdminScope, res.Import.ID, []tariffsvc.ApplyConfirmation{{BuildingID: b0, EtsoCode: "40ZTEST000000030", EffectiveFrom: effective}})
	require.NoError(t, err)
	require.Len(t, defs, 1)
	nt := defs[0].Tariff
	require.NotEqual(t, e.tenant.Tariffs[0].ID, nt.ID, "a new version, not an edit")
	require.True(t, nt.UsePtfYekdem)
	require.True(t, nt.EffectiveFrom.Equal(effective))
	require.Equal(t, model.PriceSourceFixed, nt.PowerPriceSource)
	require.Equal(t, "43.37", nt.PowerUnitPrice.String())
	require.Equal(t, model.PriceSourceFixed, nt.ReactivePriceSource)
	require.Equal(t, model.PriceSourceKbk, nt.DistributionPriceSource)
	require.Equal(t, "1.2633", nt.KbkDistributionCostTlPerKwh.String())
	require.Equal(t, "20", nt.VatRate.String())
	require.Len(t, defs[0].Taxes, 1)
	require.Equal(t, "BTV", defs[0].Taxes[0].Name)
	require.Equal(t, "5", defs[0].Taxes[0].Rate.String())
	imp, err := e.svc.GetImport(e.ctx, e.tenant.AdminScope, res.Import.ID)
	require.NoError(t, err)
	require.Equal(t, tariffsvc.ImportStatusApplied, imp.Import.Status)
}

func TestApplyImportOnCompanyWideBaseCreatesBuildingTariff(t *testing.T) {
	t.Parallel()
	e, content := icmalEnv(t, 8010)
	require.NoError(t, postgres.NewTariffRepository(e.pool).SoftDelete(e.ctx, e.tenant.AdminScope, e.tenant.Tariffs[0].ID, time.Now()))
	wide := fixedInput(nil)
	wide.Tariff.EffectiveFrom = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := e.svc.Create(e.ctx, e.tenant.AdminScope, wide)
	require.NoError(t, err)
	res, err := e.svc.Import(e.ctx, e.tenant.AdminScope, e.tenant.Users[model.UserRoleCompanyAdmin].ID, "icmal.csv", content)
	require.NoError(t, err)
	b0 := e.tenant.Buildings[0].ID
	defs, err := e.svc.ApplyImport(e.ctx, e.tenant.AdminScope, res.Import.ID, []tariffsvc.ApplyConfirmation{{BuildingID: b0, EtsoCode: "40ZTEST000000030", EffectiveFrom: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)}})
	require.NoError(t, err)
	require.NotNil(t, defs[0].Tariff.BuildingID)
	require.Equal(t, b0, *defs[0].Tariff.BuildingID, "I-15: the confirmed building, never company-wide")
	still, err := e.svc.Applicable(e.ctx, e.tenant.AdminScope, e.tenant.Buildings[1].ID, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.False(t, still.Tariff.UsePtfYekdem, "other buildings keep the company-wide tariff")
}

func TestImportIsolatesTenants(t *testing.T) {
	t.Parallel()
	e, content := icmalEnv(t, 8011)
	other := testfixtures.NewTenant(t, e.ctx, e.pool, 8111)
	res, err := e.svc.Import(e.ctx, e.tenant.AdminScope, e.tenant.Users[model.UserRoleCompanyAdmin].ID, "icmal.csv", content)
	require.NoError(t, err)
	_, err = e.svc.GetImport(e.ctx, other.AdminScope, res.Import.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = e.svc.ApplyImport(e.ctx, other.AdminScope, res.Import.ID, []tariffsvc.ApplyConfirmation{{BuildingID: other.Buildings[0].ID, EtsoCode: "40ZTEST000000030", EffectiveFrom: time.Now()}})
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = e.svc.GetImport(e.ctx, e.tenant.AdminScope, res.Import.ID)
	require.NoError(t, err, "positive control")
	theirs, err := e.svc.Import(e.ctx, other.AdminScope, other.Users[model.UserRoleCompanyAdmin].ID, "icmal.csv", content)
	require.NoError(t, err)
	require.Len(t, theirs.Unmatched, 37, "tenant A's ETSO links are invisible to tenant B")
}
