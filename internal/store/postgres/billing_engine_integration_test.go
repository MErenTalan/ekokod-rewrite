//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var seedParamsDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func TestMigration00014RoundTrips(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dsn := testfixtures.NewEmptyDB(t)
	log := testfixtures.DiscardLogger()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, log))
	require.NoError(t, postgres.MigrateDownN(ctx, dsn, log, 1))
	require.NoError(t, postgres.MigrateUp(ctx, dsn, log))

	pool := testfixtures.NewPool(t, dsn)
	var n int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from billing_parameters where effective_from = '2000-01-01'`).Scan(&n))
	require.Equal(t, 1, n)
}

// TestTariffConstraintsAllowMultiTimePTFWithBandKbks: I-10's relaxed CHECKs
// admit validated PTF tariffs and still reject an unpriced fixed tariff.
func TestTariffConstraintsAllowMultiTimePTFWithBandKbks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1401)
	insert := func(cols, vals string) error {
		_, err := pool.Exec(ctx, `insert into tariffs (company_id, effective_from, voltage_level, user_group, price_type,
			term, supply_company, distribution_cost, reactive_power_price, vat_rate`+cols+`)
			values ($1,'2026-01-01','lv','commercial',`+vals+`)`, tenant.Company.ID)
		return err
	}
	require.NoError(t, insert(`, use_ptf_yekdem, kbk_energy, kbk_t1, kbk_t2, kbk_t3`,
		`'multi_time','monomial','private',0,0,20,true,1.1,1.2,1.0,0.9`))
	require.NoError(t, insert(`, use_ptf_yekdem, kbk_energy`, `'single_time','monomial','private',0,0,20,true,1.1`))

	require.ErrorContains(t, insert(`, use_ptf_yekdem, kbk_energy, kbk_t1, kbk_t2`,
		`'multi_time','monomial','private',0,0,20,true,1.1,1.2,1.0`), "multi_time_needs_prices")
	require.ErrorContains(t, insert(``, `'single_time','monomial','incumbent',0,0,20`), "single_time_needs_price")
	require.ErrorContains(t, insert(`, t1_price, t2_price`, `'multi_time','monomial','incumbent',0,0,20,1,1`), "multi_time_needs_prices")
}

func TestBillingParametersSeedDefaults(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1402)

	p, err := postgres.NewBillingParameterRepository(pool).Effective(ctx, tenant.Scope, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.True(t, p.EffectiveFrom.Equal(seedParamsDate))
	require.Equal(t, model.ReactivePenaltyBasisWholeQuantity, p.ReactivePenaltyBasis)
	require.True(t, p.ReactiveExemptBelowKw.Equal(decimal.NewFromInt(9)))
	require.Equal(t, []model.TariffTerm{model.TariffTermMonomial}, p.ReactiveExemptTerms)
	require.Equal(t, []model.DistributionUserGroup{model.UserGroupResidential, model.UserGroupLighting}, p.ReactiveExemptUserGroups)
	require.Equal(t, []model.VoltageLevel{model.VoltageLevelLV}, p.TieringVoltageLevels)
	require.Equal(t, []model.SupplyCompany{model.SupplyCompanyIncumbent}, p.TieringSupplyCompanies)
	require.Equal(t, model.MoneyRoundingHalfUp, p.MoneyRoundingMode)
	require.Equal(t, model.TieringModeSplitAtThreshold, p.TieringMode)
	require.True(t, p.PTFMissingHourTolerance.Equal(decimal.RequireFromString("0.02")))
	require.True(t, p.DemandOverrunMultiplier.Equal(decimal.NewFromInt(2)))
	require.True(t, p.ReactiveGenerationExemptKwh.Equal(decimal.NewFromInt(1)))
}

// TestBillingParametersDecodeBandsAndGroupsExactly: jsonb decimals decode from
// their strings exactly (mutation: via float64 → the float guard or this test).
func TestBillingParametersDecodeBandsAndGroupsExactly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1403)

	p, err := postgres.NewBillingParameterRepository(pool).Effective(ctx, tenant.Scope, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, p.ReactiveBands, 2)
	b0, b1 := p.ReactiveBands[0], p.ReactiveBands[1]
	require.Equal(t, "9", b0.MinKw.String())
	require.Equal(t, "30", b0.MaxKw.String())
	require.Equal(t, "0.33", b0.Inductive.String())
	require.Equal(t, "0.2", b0.Capacitive.String())
	require.Nil(t, b1.MaxKw)
	require.Equal(t, "0.15", b1.Capacitive.String())
	require.Equal(t, "8", p.TieringGroups[model.UserGroupResidential].String())
	require.Equal(t, "30", p.TieringGroups[model.UserGroupCommercial].String())

	precise := p
	precise.EffectiveFrom = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	precise.ReactiveBands = []model.ReactiveBand{{MinKw: decimal.Zero, Inductive: decimal.RequireFromString("0.3300000000000000001"), Capacitive: decimal.RequireFromString("0.123456789012345678")}}
	precise.TieringGroups = map[model.DistributionUserGroup]decimal.Decimal{model.UserGroupResidential: decimal.RequireFromString("8.000000000000000001")}
	saved, err := admin.NewCatalogueRepository(pool).UpsertBillingParameters(ctx, precise)
	require.NoError(t, err)
	require.Equal(t, "0.3300000000000000001", saved.ReactiveBands[0].Inductive.String())
	require.Equal(t, "0.123456789012345678", saved.ReactiveBands[0].Capacitive.String())
	require.Equal(t, "8.000000000000000001", saved.TieringGroups[model.UserGroupResidential].String())
}

func TestBillingParametersEffectivePicksGreatestNotAfter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1404)
	repo := postgres.NewBillingParameterRepository(pool)
	seed, err := repo.Effective(ctx, tenant.Scope, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	next := seed
	next.EffectiveFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, istanbulForTest(t))
	next.DemandOverrunMultiplier = decimal.NewFromInt(3)
	_, err = admin.NewCatalogueRepository(pool).UpsertBillingParameters(ctx, next)
	require.NoError(t, err)

	istanbul := istanbulForTest(t)
	got, err := repo.Effective(ctx, tenant.Scope, time.Date(2025, 12, 31, 23, 59, 0, 0, istanbul))
	require.NoError(t, err)
	require.True(t, got.EffectiveFrom.Equal(seedParamsDate))
	got, err = repo.Effective(ctx, tenant.Scope, time.Date(2026, 1, 1, 0, 0, 0, 0, istanbul))
	require.NoError(t, err)
	require.Equal(t, "3", got.DemandOverrunMultiplier.String())

	list, err := repo.List(ctx, tenant.Scope)
	require.NoError(t, err)
	require.Len(t, list, 2)

	_, err = repo.Effective(ctx, store.Scope{}, time.Now())
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

// TestBillingParametersEffectiveUsesIstanbulDate: 2025-12-31T21:00Z is
// 2026-01-01 00:00 in Istanbul (I-14).
func TestBillingParametersEffectiveUsesIstanbulDate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1405)
	repo := postgres.NewBillingParameterRepository(pool)
	seed, err := repo.Effective(ctx, tenant.Scope, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	next := seed
	next.EffectiveFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, istanbulForTest(t))
	next.MoneyRoundingMode = model.MoneyRoundingHalfEven
	_, err = admin.NewCatalogueRepository(pool).UpsertBillingParameters(ctx, next)
	require.NoError(t, err)

	got, err := repo.Effective(ctx, tenant.Scope, time.Date(2025, 12, 31, 21, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, model.MoneyRoundingHalfEven, got.MoneyRoundingMode)
}

func TestTariffPriceSourcesRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1406)
	repo := postgres.NewTariffRepository(pool)
	buildingID := tenant.Buildings[0].ID

	row := tariffFixtureRow(tenant.Company.ID, &buildingID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	created, err := repo.Create(ctx, tenant.Scope, row)
	require.NoError(t, err)
	require.Equal(t, model.PriceSourceKbk, created.PowerPriceSource, "unset source defaults to kbk (R119)")

	created.PowerPriceSource = model.PriceSourceFixed
	created.ReactivePriceSource = model.PriceSourceFixed
	created.DistributionPriceSource = model.PriceSourceKbk
	updated, err := repo.Update(ctx, tenant.Scope, created)
	require.NoError(t, err)
	got, err := repo.Get(ctx, tenant.Scope, updated.ID)
	require.NoError(t, err)
	require.Equal(t, model.PriceSourceFixed, got.PowerPriceSource)
	require.Equal(t, model.PriceSourceFixed, got.ReactivePriceSource)
	require.Equal(t, model.PriceSourceKbk, got.DistributionPriceSource)
}

func TestTariffExtraChargesReplaceAndRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1407)
	repo := postgres.NewTariffRepository(pool)
	tariffID := tenant.Tariffs[0].ID

	_, err := repo.ReplaceExtraCharges(ctx, tenant.AdminScope, tariffID, []model.TariffExtraCharge{
		{Name: "a", Basis: model.ExtraChargeBasisPerKwh, Amount: decimal.RequireFromString("0.1")},
		{Name: "b", Basis: model.ExtraChargeBasisFixedPerPeriod, Amount: decimal.RequireFromString("5")},
	})
	require.NoError(t, err)
	_, err = repo.ReplaceExtraCharges(ctx, tenant.AdminScope, tariffID, []model.TariffExtraCharge{
		{Name: "discount", Basis: model.ExtraChargeBasisPctOfEnergy, Amount: decimal.RequireFromString("-2.5"), SortOrder: 2},
		{Name: "power", Basis: model.ExtraChargeBasisPerContractedKw, Amount: decimal.RequireFromString("43.37"), SortOrder: 1},
	})
	require.NoError(t, err)

	got, err := repo.ExtraCharges(ctx, tenant.AdminScope, tariffID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "power", got[0].Name)
	require.Equal(t, model.ExtraChargeBasisPerContractedKw, got[0].Basis)
	require.True(t, got[1].Amount.Equal(decimal.RequireFromString("-2.5")))
}

func TestReplaceExtraChargesRefusesAnotherTenantsTariff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1408)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 1409)
	repo := postgres.NewTariffRepository(pool)
	charges := []model.TariffExtraCharge{{Name: "x", Basis: model.ExtraChargeBasisPerKwh, Amount: decimal.NewFromInt(1)}}

	_, err := repo.ReplaceExtraCharges(ctx, tenantB.AdminScope, tenantA.Tariffs[0].ID, charges)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = repo.ExtraCharges(ctx, tenantB.AdminScope, tenantA.Tariffs[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.ReplaceExtraCharges(ctx, tenantA.AdminScope, tenantA.Tariffs[0].ID, charges)
	require.NoError(t, err, "positive control")
	got, err := repo.ExtraCharges(ctx, tenantA.AdminScope, tenantA.Tariffs[0].ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestBillNewColumnsRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1410)
	repo := postgres.NewBillRepository(pool)
	buildingID := tenant.Buildings[0].ID
	expected, missing := int32(744), int32(3)

	b := billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-01")
	b.Currency = model.CurrencyUSD
	b.ExtraChargesCost = billDec("12.3400")
	b.DemandDataAvailable = false
	b.PtfHoursExpected, b.ConsumptionHoursMissing = &expected, &missing
	created, err := repo.Create(ctx, tenant.Scope, b, nil, nil)
	require.NoError(t, err)
	got, err := repo.Get(ctx, tenant.Scope, created.ID)
	require.NoError(t, err)
	require.Equal(t, model.CurrencyUSD, got.Currency)
	require.Equal(t, "12.34", got.ExtraChargesCost.String())
	require.False(t, got.DemandDataAvailable)
	require.Equal(t, int32(744), *got.PtfHoursExpected)
	require.Equal(t, int32(3), *got.ConsumptionHoursMissing)

	b.Currency = model.CurrencyEUR
	b.DemandDataAvailable = true
	b.PtfHoursExpected, b.ConsumptionHoursMissing = nil, nil
	replacement, err := repo.Supersede(ctx, tenant.Scope, created.ID, b, nil, nil, time.Now().UTC())
	require.NoError(t, err)
	got, err = repo.Get(ctx, tenant.Scope, replacement.ID)
	require.NoError(t, err)
	require.Equal(t, model.CurrencyEUR, got.Currency)
	require.True(t, got.DemandDataAvailable)
	require.Nil(t, got.PtfHoursExpected)
}

func istanbulForTest(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	return loc
}
