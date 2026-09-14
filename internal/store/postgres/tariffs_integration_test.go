//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// tariffFixtureRow is a minimally valid single_time tariff for buildingID
// (nil for company-wide), effective from effectiveFrom.
func tariffFixtureRow(companyID uuid.UUID, buildingID *uuid.UUID, effectiveFrom time.Time) model.Tariff {
	return model.Tariff{
		CompanyID:          companyID,
		BuildingID:         buildingID,
		EffectiveFrom:      effectiveFrom,
		Currency:           model.CurrencyTRY,
		EnergyType:         model.EnergyTypeGrid,
		VoltageLevel:       model.VoltageLevelLV,
		UserGroup:          model.UserGroupCommercial,
		PriceType:          model.PriceTypeSingleTime,
		Term:               model.TariffTermMonomial,
		SupplyCompany:      model.SupplyCompanyIncumbent,
		SingleTimePrice:    tariffDecPtr("2.500000"),
		DistributionCost:   decimal.RequireFromString("0.500000"),
		ReactivePowerPrice: decimal.RequireFromString("1.000000"),
		VatRate:            decimal.RequireFromString("20.000"),
		GenerationUsage:    model.GenerationUsageNone,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
}

func tariffDecPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func TestTariffGetIsScopedToCompany(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewTariffRepository(pool)

	// Tenant A cannot read tenant B's tariff by id: another tenant's row is
	// indistinguishable from a missing one.
	_, err := repo.Get(ctx, tenantA.AdminScope, tenantB.Tariffs[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Tenant B can read its own.
	got, err := repo.Get(ctx, tenantB.AdminScope, tenantB.Tariffs[0].ID)
	require.NoError(t, err)
	require.Equal(t, tenantB.Tariffs[0].ID, got.ID)

	// An invalid scope is rejected before any query.
	_, err = repo.Get(ctx, store.Scope{}, tenantA.Tariffs[0].ID)
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

// TestTariffListExcludesCompanyWideForNarrowScope pins the NULL building_id
// ruling: a company-wide tariff (building_id is null) is visible only to a
// Scope with AllBuildings, never to a narrow one, even within List's own
// company.
func TestTariffListExcludesCompanyWideForNarrowScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 10)
	repo := postgres.NewTariffRepository(pool)

	companyWide, err := repo.Create(ctx, tenant.AdminScope, tariffFixtureRow(tenant.Company.ID, nil, date(2026, 2, 1)))
	require.NoError(t, err)

	// AdminScope (AllBuildings) sees the company-wide tariff.
	all, err := repo.List(ctx, tenant.AdminScope, store.TariffFilter{CompanyWide: true})
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, companyWide.ID, all[0].ID)

	// The narrow Scope (Buildings[0] only) sees none: List does not narrow
	// into "all" just because CompanyWide was not asked for.
	narrow, err := repo.List(ctx, tenant.Scope, store.TariffFilter{})
	require.NoError(t, err)
	for _, tr := range narrow {
		require.NotNil(t, tr.BuildingID, "a narrow scope must never see a company-wide tariff via List")
	}

	// Get on the company-wide tariff by id is refused for the narrow Scope too.
	_, err = repo.Get(ctx, tenant.Scope, companyWide.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestEffectiveTariffPicksTheLatestNotAfterTheDate pins tariff resolution by
// effective_from — the wrong row here misprices every invoice.
func TestEffectiveTariffPicksTheLatestNotAfterTheDate(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 20)
	repo := postgres.NewTariffRepository(pool)
	buildingID := tenant.Buildings[0].ID

	// NewTenant already seeds a tariff for this building dated 2026-01-01;
	// remove it so the dates below are the only candidates and ties cannot
	// make this test pass by accident.
	require.NoError(t, repo.SoftDelete(ctx, tenant.AdminScope, tenant.Tariffs[0].ID, time.Now().UTC()))

	jan, err := repo.Create(ctx, tenant.AdminScope, tariffFixtureRow(tenant.Company.ID, &buildingID, date(2026, 1, 1)))
	require.NoError(t, err)
	mar, err := repo.Create(ctx, tenant.AdminScope, tariffFixtureRow(tenant.Company.ID, &buildingID, date(2026, 3, 1)))
	require.NoError(t, err)
	jun, err := repo.Create(ctx, tenant.AdminScope, tariffFixtureRow(tenant.Company.ID, &buildingID, date(2026, 6, 1)))
	require.NoError(t, err)

	cases := []struct {
		name string
		on   time.Time
		want uuid.UUID
	}{
		{"exactly on jan", date(2026, 1, 1), jan.ID},
		{"between jan and mar", date(2026, 2, 15), jan.ID},
		{"exactly on mar", date(2026, 3, 1), mar.ID},
		{"between mar and jun", date(2026, 5, 1), mar.ID},
		{"on or after jun", date(2026, 12, 31), jun.ID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.Effective(ctx, tenant.Scope, buildingID, tc.on)
			require.NoError(t, err)
			require.Equal(t, tc.want, got.ID)
		})
	}

	// Before the earliest tariff, there is nothing in force yet.
	_, err = repo.Effective(ctx, tenant.Scope, buildingID, date(2025, 12, 31))
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestTariffEffectiveFallsBackToCompanyWideTariff is the second half of the
// NULL building_id ruling: a narrow Scope's Effective on its OWN visible
// building may resolve to the company-wide tariff when the building has
// none of its own, even though the narrow Scope could never List or Get
// that company-wide row directly.
func TestTariffEffectiveFallsBackToCompanyWideTariff(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 30)
	repo := postgres.NewTariffRepository(pool)

	// NewTenant already seeds a building-specific tariff for Buildings[0]
	// dated 2026-01-01; soft-delete it so only the company-wide one applies.
	require.NoError(t, repo.SoftDelete(ctx, tenant.AdminScope, tenant.Tariffs[0].ID, time.Now().UTC()))

	companyWide, err := repo.Create(ctx, tenant.AdminScope, tariffFixtureRow(tenant.Company.ID, nil, date(2026, 1, 1)))
	require.NoError(t, err)

	got, err := repo.Effective(ctx, tenant.Scope, tenant.Buildings[0].ID, date(2026, 6, 1))
	require.NoError(t, err)
	require.Equal(t, companyWide.ID, got.ID)
}

// TestTariffEffectiveRequiresVisibleBuilding proves Effective checks the
// BUILDING'S visibility before it ever considers a company-wide fallback: a
// narrow Scope cannot resolve a tariff for a building outside it, even
// though a company-wide tariff exists.
//
// Deliberate-break proof (reverted after running): removing the
// `if !visible { return ... ErrNotFound }` branch in TariffRepository.
// Effective made this test fail with "expected: not found, actual: <nil>"
// — see the task report.
func TestTariffEffectiveRequiresVisibleBuilding(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 40)
	repo := postgres.NewTariffRepository(pool)

	_, err := repo.Create(ctx, tenant.AdminScope, tariffFixtureRow(tenant.Company.ID, nil, date(2026, 1, 1)))
	require.NoError(t, err)

	_, err = repo.Effective(ctx, tenant.Scope, tenant.Buildings[1].ID, date(2026, 6, 1))
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestTariffTaxesJoinThroughTariffIsolation covers both mandatory methods on
// tariff_taxes: Taxes and ReplaceTaxes must join through tariffs, so another
// tenant's tariff id yields ErrNotFound and a write touches nothing.
//
// Deliberate-break proof (reverted): replacing requireVisible's body with
// `return nil` in tariffs.go made this test's cross-tenant assertions fail
// with "expected: not found, actual: <nil>" for both Taxes and ReplaceTaxes.
func TestTariffTaxesJoinThroughTariffIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 50)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 51)
	repo := postgres.NewTariffRepository(pool)

	_, err := repo.Taxes(ctx, tenantA.AdminScope, tenantB.Tariffs[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.ReplaceTaxes(ctx, tenantA.AdminScope, tenantB.Tariffs[0].ID, []model.TariffTax{
		{Name: "BTV", Rate: decimal.RequireFromString("2.000"), SortOrder: 1},
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	// Own tariff: round-trips.
	taxes, err := repo.ReplaceTaxes(ctx, tenantB.AdminScope, tenantB.Tariffs[0].ID, []model.TariffTax{
		{Name: "BTV", Rate: decimal.RequireFromString("2.000"), SortOrder: 1},
		{Name: "TRT", Rate: decimal.RequireFromString("1.000"), SortOrder: 2},
	})
	require.NoError(t, err)
	require.Len(t, taxes, 2)

	got, err := repo.Taxes(ctx, tenantB.AdminScope, tenantB.Tariffs[0].ID)
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Replacing again REPLACES, not accumulates.
	replaced, err := repo.ReplaceTaxes(ctx, tenantB.AdminScope, tenantB.Tariffs[0].ID, []model.TariffTax{
		{Name: "Enerji Fonu", Rate: decimal.RequireFromString("1.000"), SortOrder: 1},
	})
	require.NoError(t, err)
	require.Len(t, replaced, 1)
}

// TestTariffManualYekdemJoinThroughTariffIsolation covers both mandatory
// methods on tariff_manual_yekdem.
func TestTariffManualYekdemJoinThroughTariffIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 60)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 61)
	repo := postgres.NewTariffRepository(pool)

	_, err := repo.ManualYekdem(ctx, tenantA.AdminScope, tenantB.Tariffs[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.ReplaceManualYekdem(ctx, tenantA.AdminScope, tenantB.Tariffs[0].ID, []model.TariffManualYekdem{
		{Year: 2026, Month: 1, Value: decimal.RequireFromString("450.5000")},
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	values, err := repo.ReplaceManualYekdem(ctx, tenantB.AdminScope, tenantB.Tariffs[0].ID, []model.TariffManualYekdem{
		{Year: 2026, Month: 1, Value: decimal.RequireFromString("450.5000")},
		{Year: 2026, Month: 2, Value: decimal.RequireFromString("460.5000")},
	})
	require.NoError(t, err)
	require.Len(t, values, 2)

	got, err := repo.ManualYekdem(ctx, tenantB.AdminScope, tenantB.Tariffs[0].ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
}

// TestTariffCreateRejectsBuildingOutsideScope proves the write side of
// building visibility: a narrow Scope cannot create a tariff for a building
// it does not cover, and cannot create a company-wide tariff at all.
func TestTariffCreateRejectsBuildingOutsideScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 70)
	repo := postgres.NewTariffRepository(pool)

	otherBuilding := tenant.Buildings[1].ID
	_, err := repo.Create(ctx, tenant.Scope, tariffFixtureRow(tenant.Company.ID, &otherBuilding, date(2026, 1, 1)))
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.Create(ctx, tenant.Scope, tariffFixtureRow(tenant.Company.ID, nil, date(2026, 1, 1)))
	require.ErrorIs(t, err, store.ErrNotFound, "a narrow scope may not create a company-wide tariff")

	ownBuilding := tenant.Buildings[0].ID
	created, err := repo.Create(ctx, tenant.Scope, tariffFixtureRow(tenant.Company.ID, &ownBuilding, date(2026, 7, 1)))
	require.NoError(t, err)
	require.Equal(t, ownBuilding, *created.BuildingID)
}

// TestSolarTariffScopedToCompanyNotBuilding pins the ruling power_plants
// carries (no building_id): solar_tariffs, which prices a plant, is visible
// to the whole company Scope, and cross-tenant access is refused.
func TestSolarTariffScopedToCompanyNotBuilding(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 80)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 81)
	repo := postgres.NewSolarTariffRepository(pool)

	created, err := repo.Create(ctx, tenantA.AdminScope, model.SolarTariff{
		CompanyID: tenantA.Company.ID, PlantID: tenantA.Plants[0].ID, EffectiveFrom: date(2026, 1, 1),
		FeedInTariff: decimal.RequireFromString("1.500000"), Currency: model.CurrencyTRY, CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	// A narrow (building-scoped) Scope can still see it: solar tariffs are
	// not narrowed by building, only by company.
	_, err = repo.Get(ctx, tenantA.Scope, created.ID)
	require.NoError(t, err)

	// Another tenant, and a plant belonging to another tenant, are refused.
	_, err = repo.Get(ctx, tenantB.AdminScope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.Create(ctx, tenantB.AdminScope, model.SolarTariff{
		CompanyID: tenantB.Company.ID, PlantID: tenantA.Plants[0].ID, EffectiveFrom: date(2026, 1, 1),
		FeedInTariff: decimal.RequireFromString("1.500000"), Currency: model.CurrencyTRY, CreatedAt: time.Now().UTC(),
	})
	require.ErrorIs(t, err, store.ErrNotFound, "tenant B cannot price tenant A's plant")

	_, err = repo.Effective(ctx, tenantB.AdminScope, tenantA.Plants[0].ID, date(2026, 6, 1))
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestNationalTariffScopeValidatedButNotNarrowed pins the platform-table
// rule: an invalid Scope is refused, but a valid Scope from either tenant
// sees the SAME published rows, because the table has no company_id at all.
func TestNationalTariffScopeValidatedButNotNarrowed(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 90)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 91)
	repo := postgres.NewNationalTariffRepository(pool)

	_, err := pool.Exec(ctx, `insert into national_tariff_schedule
		(effective_from, user_group, voltage_level, term, energy_price, distribution_price, vat_rate)
		values ('2026-01-01', 'commercial', 'lv', 'monomial', 2.5, 0.5, 20)`)
	require.NoError(t, err)

	_, err = repo.List(ctx, store.Scope{}, store.NationalTariffFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)

	fromA, err := repo.List(ctx, tenantA.Scope, store.NationalTariffFilter{})
	require.NoError(t, err)
	fromB, err := repo.List(ctx, tenantB.AdminScope, store.NationalTariffFilter{})
	require.NoError(t, err)
	require.Len(t, fromA, 1)
	require.Len(t, fromB, 1)
	require.Equal(t, fromA[0].ID, fromB[0].ID, "the schedule is platform-wide: both tenants see the identical row")

	entry, err := repo.Effective(ctx, tenantA.Scope, model.UserGroupCommercial, model.VoltageLevelLV, model.TariffTermMonomial, date(2026, 6, 1))
	require.NoError(t, err)
	require.Equal(t, "2.500000", entry.EnergyPrice.StringFixed(6))
}

// TestIcmalRowsJoinThroughImportIsolation covers both mandatory methods on
// icmal_rows: InsertRows and ListRows must join through icmal_imports, so
// another tenant's import id yields ErrNotFound.
//
// Deliberate-break proof (reverted): removing the IcmalImportVisible check
// from InsertRows made a call with tenant B's own import id succeed against
// tenant A's Scope, instead of failing with ErrNotFound.
func TestIcmalRowsJoinThroughImportIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 100)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 101)
	repo := postgres.NewIcmalRepository(pool)

	impB, err := repo.CreateImport(ctx, tenantB.AdminScope, model.IcmalImport{FileName: "b.xlsx"})
	require.NoError(t, err)

	_, err = repo.InsertRows(ctx, tenantA.AdminScope, impB.ID, []model.IcmalRow{{Period: "202601"}})
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.ListRows(ctx, tenantA.AdminScope, impB.ID, store.Page{})
	require.ErrorIs(t, err, store.ErrNotFound)

	// Tenant B, its own import: works.
	n, err := repo.InsertRows(ctx, tenantB.AdminScope, impB.ID, []model.IcmalRow{{Period: "202601"}, {Period: "202602"}})
	require.NoError(t, err)
	require.Equal(t, int64(2), n)

	rows, err := repo.ListRows(ctx, tenantB.AdminScope, impB.ID, store.Page{})
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

// TestIcmalInsertRowsRefusesWholeBatchForInvisibleBuilding proves the
// batch-refusal rule: if ANY row names a building not visible to the Scope,
// nothing in the batch is written.
func TestIcmalInsertRowsRefusesWholeBatchForInvisibleBuilding(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 110)
	repo := postgres.NewIcmalRepository(pool)

	imp, err := repo.CreateImport(ctx, tenant.Scope, model.IcmalImport{FileName: "icmal.xlsx"})
	require.NoError(t, err)

	visible := tenant.Buildings[0].ID
	invisible := tenant.Buildings[1].ID
	_, err = repo.InsertRows(ctx, tenant.Scope, imp.ID, []model.IcmalRow{
		{Period: "202601", BuildingID: &visible},
		{Period: "202601", BuildingID: &invisible},
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	rows, err := repo.ListRows(ctx, tenant.Scope, imp.ID, store.Page{})
	require.NoError(t, err)
	require.Empty(t, rows, "the whole batch must be refused, including the row with a visible building")
}

// date builds a UTC midnight time.Time for the `date`-column tests above.
func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
