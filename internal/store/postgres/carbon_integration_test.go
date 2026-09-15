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
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// carbonDec is this file's fixture-literal decimal parser, prefixed per the
// package-wide test-helper naming rule (wave-f-context.md rule 6).
func carbonDec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// --- emission_factors: nullable company_id -------------------------------

func TestCarbonFactorPlatformReadableCompanyWritable(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 4001)
	repo := postgres.NewCarbonRepository(pool)

	// A platform factor (company_id null) is inserted directly: only the
	// admin surface may write one.
	var platformID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `insert into emission_factors
		(company_id, key, label, main_category, base_factor, base_unit)
		values (null, 'diesel', 'Diesel', 'fuel', 2.68, 'kg')
		returning id`).Scan(&platformID))

	// Any valid tenant scope can read the platform factor.
	got, err := repo.Factor(ctx, tenant.Scope, platformID)
	require.NoError(t, err)
	require.Nil(t, got.CompanyID)
	require.Equal(t, "diesel", got.Key)

	// An invalid scope is rejected before any database call.
	_, err = repo.Factor(ctx, store.Scope{}, platformID)
	require.ErrorIs(t, err, store.ErrInvalidScope)

	// UpsertFactor with a nil CompanyID (as if trying to write the platform
	// factor through the scoped surface) is refused with ErrNotFound.
	_, err = repo.UpsertFactor(ctx, tenant.Scope, model.EmissionFactor{
		Key: "diesel", Label: "x", MainCategory: "fuel", BaseFactor: carbonDec("1"), BaseUnit: "kg",
	})
	require.ErrorIs(t, err, store.ErrNotFound, "a nil CompanyID must be refused before any write")

	// A company-owned upsert with the tenant's own CompanyID succeeds and
	// shadows the platform key without touching it.
	companyID := tenant.Company.ID
	own, err := repo.UpsertFactor(ctx, tenant.Scope, model.EmissionFactor{
		CompanyID: &companyID, Key: "diesel", Label: "Our Diesel", MainCategory: "fuel",
		BaseFactor: carbonDec("2.70"), BaseUnit: "kg",
	})
	require.NoError(t, err)
	require.NotEqual(t, platformID, own.ID, "the company's row must be a DIFFERENT row from the platform one")

	platformStill, err := repo.Factor(ctx, tenant.Scope, platformID)
	require.NoError(t, err)
	require.True(t, platformStill.BaseFactor.Equal(carbonDec("2.68000000")), "the platform row must be untouched")

	// Re-upserting the same key converges on the SAME row (idempotent on the
	// natural key), not a new one.
	again, err := repo.UpsertFactor(ctx, tenant.Scope, model.EmissionFactor{
		CompanyID: &companyID, Key: "diesel", Label: "Our Diesel Updated", MainCategory: "fuel",
		BaseFactor: carbonDec("2.71"), BaseUnit: "kg",
	})
	require.NoError(t, err)
	require.Equal(t, own.ID, again.ID)
	require.Equal(t, "Our Diesel Updated", again.Label)
}

// TestCarbonUpsertFactorRefusesAnotherCompanysID proves the ID-ownership
// guard: an id already naming another company's factor is refused with
// ErrNotFound and nothing is written, even though the key differs.
func TestCarbonUpsertFactorRefusesAnotherCompanysID(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 4002)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 4003)
	repo := postgres.NewCarbonRepository(pool)

	companyA := tenantA.Company.ID
	ownA, err := repo.UpsertFactor(ctx, tenantA.Scope, model.EmissionFactor{
		CompanyID: &companyA, Key: "petrol", Label: "Petrol", MainCategory: "fuel",
		BaseFactor: carbonDec("2.30"), BaseUnit: "kg",
	})
	require.NoError(t, err)

	companyB := tenantB.Company.ID
	_, err = repo.UpsertFactor(ctx, tenantB.AdminScope, model.EmissionFactor{
		ID: ownA.ID, CompanyID: &companyB, Key: "brand-new-key-for-b", Label: "x", MainCategory: "fuel",
		BaseFactor: carbonDec("1"), BaseUnit: "kg",
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.ListFactors(ctx, tenantB.AdminScope, store.EmissionFactorFilter{})
	require.NoError(t, err)
	for _, f := range list {
		require.NotEqual(t, "brand-new-key-for-b", f.Key, "the refused write must not have gone through under any key")
		require.NotEqual(t, ownA.ID, f.ID, "tenant A's own factor must never appear in tenant B's ListFactors (fix round 1, Important 4)")
	}

	// Factor(): tenant B (even under AdminScope) cannot read tenant A's own
	// factor by id — it is indistinguishable from a missing one.
	_, err = repo.Factor(ctx, tenantB.AdminScope, ownA.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	// UpsertFactor also refuses a foreign, NON-NIL CompanyID (not just a nil
	// one): the Go-level equality check catches a real other-tenant id too.
	_, err = repo.UpsertFactor(ctx, tenantA.AdminScope, model.EmissionFactor{
		CompanyID: &companyB, Key: "petrol", Label: "hijacked", MainCategory: "fuel",
		BaseFactor: carbonDec("1"), BaseUnit: "kg",
	})
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestCarbonUpsertFactorRefusesPlatformFactorID proves the ID-ownership
// guard also refuses a PLATFORM factor's id through the scoped surface: the
// `target.ok` clause treats a platform row (company_id null) exactly like
// another company's — "distinct from" the caller's own company_id — so it
// can never be silently annexed into a company's catalogue via UpsertFactor.
func TestCarbonUpsertFactorRefusesPlatformFactorID(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 4004)
	repo := postgres.NewCarbonRepository(pool)

	var platformID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `insert into emission_factors
		(company_id, key, label, main_category, base_factor, base_unit)
		values (null, 'coal', 'Coal', 'fuel', 3.15, 'kg')
		returning id`).Scan(&platformID))

	companyID := tenant.Company.ID
	_, err := repo.UpsertFactor(ctx, tenant.Scope, model.EmissionFactor{
		ID: platformID, CompanyID: &companyID, Key: "hijacked-coal", Label: "x", MainCategory: "fuel",
		BaseFactor: carbonDec("1"), BaseUnit: "kg",
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	still, err := repo.Factor(ctx, tenant.Scope, platformID)
	require.NoError(t, err)
	require.Nil(t, still.CompanyID, "the platform factor must still be a platform factor")
	require.Equal(t, "coal", still.Key, "the refused write must not have overwritten it")
}

// --- emission_factor_conversions: no company_id at all --------------------

// TestCarbonConversionsIsolation is the mandatory isolation test for
// emission_factor_conversions: readable for a platform factor or one of the
// caller's own; another company's factorID returns ErrNotFound both for the
// read and for ReplaceConversions.
func TestCarbonConversionsIsolation(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 4010)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 4011)
	repo := postgres.NewCarbonRepository(pool)

	companyA := tenantA.Company.ID
	factor, err := repo.UpsertFactor(ctx, tenantA.Scope, model.EmissionFactor{
		CompanyID: &companyA, Key: "lpg", Label: "LPG", MainCategory: "fuel",
		BaseFactor: carbonDec("1.51"), BaseUnit: "kg",
	})
	require.NoError(t, err)

	err = repo.ReplaceConversions(ctx, tenantA.Scope, factor.ID, []model.EmissionFactorConversion{
		{Unit: "litre", Multiplier: carbonDec("0.55"), Label: "per litre"},
	})
	require.NoError(t, err)

	conversions, err := repo.Conversions(ctx, tenantA.Scope, factor.ID)
	require.NoError(t, err)
	require.Len(t, conversions, 1)
	require.True(t, conversions[0].Multiplier.Equal(carbonDec("0.55000000")))

	// Tenant B cannot read tenant A's factor's conversions — including under
	// tenant B's OWN AdminScope (fix round 1, Important 4: the
	// company_id-only tautology (f.company_id is null or f.company_id = $2
	// or true) must fail this call specifically, since AdminScope has no
	// building-list branch to (wrongly) mask it).
	_, err = repo.Conversions(ctx, tenantB.Scope, factor.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = repo.Conversions(ctx, tenantB.AdminScope, factor.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Tenant B cannot replace them either, and nothing changes — again under
	// AdminScope, so the isolation is proven at the company_id predicate
	// itself, not by a narrow scope's building-id-list.
	err = repo.ReplaceConversions(ctx, tenantB.Scope, factor.ID, []model.EmissionFactorConversion{
		{Unit: "hacked", Multiplier: carbonDec("999"), Label: "x"},
	})
	require.ErrorIs(t, err, store.ErrNotFound)
	err = repo.ReplaceConversions(ctx, tenantB.AdminScope, factor.ID, []model.EmissionFactorConversion{
		{Unit: "hacked-admin", Multiplier: carbonDec("999"), Label: "x"},
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	stillA, err := repo.Conversions(ctx, tenantA.Scope, factor.ID)
	require.NoError(t, err)
	require.Len(t, stillA, 1)
	require.Equal(t, "litre", stillA[0].Unit, "tenant B's refused replace must not have touched tenant A's row")

	// A platform factor's conversions are readable by any tenant.
	var platformFactorID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `insert into emission_factors
		(company_id, key, label, main_category, base_factor, base_unit)
		values (null, 'natural_gas', 'Natural Gas', 'fuel', 1.89, 'm3')
		returning id`).Scan(&platformFactorID))
	_, err = pool.Exec(ctx, `insert into emission_factor_conversions (factor_id, unit, multiplier, label)
		values ($1, 'kwh', 0.1832, 'per kwh')`, platformFactorID)
	require.NoError(t, err)

	platformConversions, err := repo.Conversions(ctx, tenantB.Scope, platformFactorID)
	require.NoError(t, err)
	require.Len(t, platformConversions, 1)

	// And ReplaceConversions on a platform factor is refused too: the scoped
	// surface never writes a company_id-null row.
	err = repo.ReplaceConversions(ctx, tenantB.Scope, platformFactorID, nil)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestCarbonReplaceConversionsSQLScopingSurvivesGoCheckBypass proves
// ReplaceConversions' isolation does not depend on any Go-level check (fix
// round 1, Important 2): CarbonDeleteConversions and CarbonInsertConversions
// — the exact two statements ReplaceConversions calls — are invoked HERE
// DIRECTLY through sqlcgen, entirely bypassing ReplaceConversions' own
// Scope.Valid() check, its FOR SHARE lock, and the repository method itself.
// With no Go-level guard anywhere in the call path, a mismatched company_id
// still deletes and inserts ZERO rows against another company's factor,
// because both statements carry the company_id predicate themselves, joined
// through emission_factors.
func TestCarbonReplaceConversionsSQLScopingSurvivesGoCheckBypass(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 4012)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 4013)
	repo := postgres.NewCarbonRepository(pool)

	companyA := tenantA.Company.ID
	factor, err := repo.UpsertFactor(ctx, tenantA.Scope, model.EmissionFactor{
		CompanyID: &companyA, Key: "propane-bypass", Label: "Propane", MainCategory: "fuel",
		BaseFactor: carbonDec("1.51"), BaseUnit: "kg",
	})
	require.NoError(t, err)
	require.NoError(t, repo.ReplaceConversions(ctx, tenantA.Scope, factor.ID, []model.EmissionFactorConversion{
		{Unit: "litre", Multiplier: carbonDec("0.55"), Label: "per litre"},
	}))

	q := sqlcgen.New(pool)

	// The lock query itself, called with tenant B's company id against
	// tenant A's factor, matches zero rows — this is what makes the OLD
	// Go-level `if owner != s.CompanyID` comparison unnecessary: there is
	// nothing left to compare against once the lock query is scoped.
	_, err = q.CarbonFactorOwnerForShare(ctx, sqlcgen.CarbonFactorOwnerForShareParams{
		ID: factor.ID, CompanyID: tenantB.Company.ID,
	})
	require.Error(t, err, "the lock must not match another company's factor")

	// The delete, called DIRECTLY with tenant B's company id — no Scope
	// check, no lock, no repository method anywhere in the call path —
	// still affects nothing: the join to emission_factors filters it out.
	require.NoError(t, q.CarbonDeleteConversions(ctx, sqlcgen.CarbonDeleteConversionsParams{
		FactorID: factor.ID, CompanyID: tenantB.Company.ID,
	}))

	// The insert, same bypass: it SELECTS zero rows to insert (the join
	// predicate f.id=... and f.company_id=... matches nothing for tenant
	// B's id), so nothing lands under the wrong company either.
	require.NoError(t, q.CarbonInsertConversions(ctx, sqlcgen.CarbonInsertConversionsParams{
		FactorID: factor.ID, CompanyID: tenantB.Company.ID,
		Conversions: []byte(`[{"unit":"hacked","multiplier":"999","label":"x"}]`),
	}))

	// Tenant A's real conversion is untouched by either bypassed call.
	stillA, err := repo.Conversions(ctx, tenantA.Scope, factor.ID)
	require.NoError(t, err)
	require.Len(t, stillA, 1)
	require.Equal(t, "litre", stillA[0].Unit, "a cross-tenant call must be refused even with every Go-level check bypassed")
}

// --- carbon_selected_activities --------------------------------------------

func TestCarbonSelectedActivitiesReplace(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 4020)
	repo := postgres.NewCarbonRepository(pool)

	buildingID := tenant.Buildings[0].ID
	err := repo.ReplaceSelectedActivities(ctx, tenant.Scope, buildingID, []string{"electricity", "natural_gas"})
	require.NoError(t, err)

	list, err := repo.SelectedActivities(ctx, tenant.Scope, buildingID)
	require.NoError(t, err)
	require.Len(t, list, 2)

	// Replacing again converges rather than accumulating.
	err = repo.ReplaceSelectedActivities(ctx, tenant.Scope, buildingID, []string{"electricity"})
	require.NoError(t, err)
	list, err = repo.SelectedActivities(ctx, tenant.Scope, buildingID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "electricity", list[0].ActivityKey)

	// A narrow scope (covers Buildings[0] only) cannot replace Buildings[1]'s
	// selection: the building is locked FOR SHARE and found invisible.
	err = repo.ReplaceSelectedActivities(ctx, tenant.Scope, tenant.Buildings[1].ID, []string{"x"})
	require.ErrorIs(t, err, store.ErrNotFound)

	// The admin scope (AllBuildings) can.
	err = repo.ReplaceSelectedActivities(ctx, tenant.AdminScope, tenant.Buildings[1].ID, []string{"water"})
	require.NoError(t, err)
}

// --- carbon_activities ------------------------------------------------------

func carbonActivityFixture(companyID, buildingID uuid.UUID) model.CarbonActivity {
	return model.CarbonActivity{
		CompanyID: companyID, BuildingID: buildingID,
		MainCategory: "energy", SubCategory: "grid", ActivityType: "electricity",
		PeriodStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Quantity:    carbonDec("1000.500000"), Unit: "kWh",
		ConversionMultiplier: carbonDec("1.00000000"), EmissionKgco2e: carbonDec("485.230000"),
		Scope: model.CarbonScope2, IsoCategory: "cat1", Status: model.CarbonStatusPending,
	}
}

func TestCarbonActivityCRUDAndScopeNarrowing(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 4030)
	repo := postgres.NewCarbonRepository(pool)

	a := carbonActivityFixture(tenant.Company.ID, tenant.Buildings[0].ID)
	created, err := repo.CreateActivity(ctx, tenant.Scope, a)
	require.NoError(t, err)
	require.True(t, created.Quantity.Equal(carbonDec("1000.500000")))

	got, err := repo.Activity(ctx, tenant.Scope, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)

	created.Quantity = carbonDec("1200.750000")
	created.CompanyID = tenant.Company.ID
	updated, err := repo.UpdateActivity(ctx, tenant.Scope, created)
	require.NoError(t, err)
	require.True(t, updated.Quantity.Equal(carbonDec("1200.750000")))

	statusUpdated, err := repo.SetActivityStatus(ctx, tenant.Scope, created.ID, model.CarbonStatusApproved, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, model.CarbonStatusApproved, statusUpdated.Status)

	list, err := repo.ListActivities(ctx, tenant.Scope, store.CarbonActivityFilter{})
	require.NoError(t, err)
	require.Len(t, list, 1)

	// A row created (via the admin scope) under Buildings[1] is invisible to
	// the narrow Scope.
	other := carbonActivityFixture(tenant.Company.ID, tenant.Buildings[1].ID)
	otherCreated, err := repo.CreateActivity(ctx, tenant.AdminScope, other)
	require.NoError(t, err)

	_, err = repo.Activity(ctx, tenant.Scope, otherCreated.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.DeleteActivity(ctx, tenant.Scope, otherCreated.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "a narrow scope must not be able to delete another building's activity")

	require.NoError(t, repo.DeleteActivity(ctx, tenant.AdminScope, otherCreated.ID))
}

func TestCarbonCreateActivityRefusesInvisibleBuildingAndWrongCompany(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 4040)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 4041)
	repo := postgres.NewCarbonRepository(pool)

	// Narrow scope cannot create against Buildings[1].
	_, err := repo.CreateActivity(ctx, tenantA.Scope, carbonActivityFixture(tenantA.Company.ID, tenantA.Buildings[1].ID))
	require.ErrorIs(t, err, store.ErrNotFound)

	// A CompanyID mismatch is refused before any database call, even for a
	// visible building id borrowed from another tenant.
	_, err = repo.CreateActivity(ctx, tenantA.Scope, carbonActivityFixture(tenantB.Company.ID, tenantA.Buildings[0].ID))
	require.ErrorIs(t, err, store.ErrNotFound)

	// UpdateActivity has the identical Go-level check (fix round 1,
	// Important 4: previously untested): a foreign CompanyID is refused
	// before any database call, even naming a real, own-company activity id.
	created, err := repo.CreateActivity(ctx, tenantA.Scope, carbonActivityFixture(tenantA.Company.ID, tenantA.Buildings[0].ID))
	require.NoError(t, err)
	created.CompanyID = tenantB.Company.ID
	_, err = repo.UpdateActivity(ctx, tenantA.Scope, created)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestCarbonUpsertAutomatedActivityConvergesOnOneRow(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 4050)
	repo := postgres.NewCarbonRepository(pool)

	a := carbonActivityFixture(tenant.Company.ID, tenant.Buildings[0].ID)
	first, err := repo.UpsertAutomatedActivity(ctx, tenant.Scope, a)
	require.NoError(t, err)
	require.True(t, first.IsAutomated)

	a.EmissionKgco2e = carbonDec("999.990000")
	second, err := repo.UpsertAutomatedActivity(ctx, tenant.Scope, a)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "the partial unique index must converge on one row")
	require.True(t, second.EmissionKgco2e.Equal(carbonDec("999.990000")))

	list, err := repo.ListActivities(ctx, tenant.Scope, store.CarbonActivityFilter{IsAutomated: boolPtrCarbon(true)})
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func boolPtrCarbon(b bool) *bool { return &b }

// --- carbon_reports ----------------------------------------------------------

func TestCarbonReportsCreateAndList(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 4060)
	repo := postgres.NewCarbonRepository(pool)

	rep := model.CarbonReport{
		CompanyID: tenant.Company.ID, BuildingID: tenant.Buildings[0].ID,
		Name: "Q1 GHG", ReportType: "ghg", Period: "2026-Q1", Payload: []byte(`{"total":1}`),
	}
	created, err := repo.CreateReport(ctx, tenant.Scope, rep)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, created.ID)

	list, err := repo.ListReports(ctx, tenant.Scope, nil, store.Page{})
	require.NoError(t, err)
	require.Len(t, list, 1)

	// A narrow scope cannot create a report against Buildings[1].
	_, err = repo.CreateReport(ctx, tenant.Scope, model.CarbonReport{
		CompanyID: tenant.Company.ID, BuildingID: tenant.Buildings[1].ID,
		Name: "x", ReportType: "ghg", Period: "2026-Q1", Payload: []byte(`{}`),
	})
	require.ErrorIs(t, err, store.ErrNotFound)
}

// --- invalid scope, before any database call --------------------------------

func TestCarbonRepositoryRejectsInvalidScope(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	repo := postgres.NewCarbonRepository(pool)
	var invalid store.Scope

	_, err := repo.ListFactors(ctx, invalid, store.EmissionFactorFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.Conversions(ctx, invalid, uuid.New())
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.ListActivities(ctx, invalid, store.CarbonActivityFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.ListReports(ctx, invalid, nil, store.Page{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

// TestCarbonAdminScopeCannotStoreAnotherTenantsForeignKeys proves that
// AdminScope (AllBuildings: true) — the widest legitimate scope a single
// tenant can hold — still cannot be used to store ANOTHER tenant's building
// or user id. Scope.AllowsBuilding returns true for any id at all under
// AllBuildings, so every write here embeds the real ownership check
// (company_id = the caller's own) directly in its SQL rather than trusting
// that Go-level helper.
func TestCarbonAdminScopeCannotStoreAnotherTenantsForeignKeys(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 4070)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 4071)
	repo := postgres.NewCarbonRepository(pool)

	foreignBuilding := tenantB.Buildings[0].ID
	foreignUser := tenantB.Users[model.UserRoleCompanyAdmin].ID

	// CreateActivity: tenant A's AdminScope cannot store tenant B's building.
	_, err := repo.CreateActivity(ctx, tenantA.AdminScope, carbonActivityFixture(tenantA.Company.ID, foreignBuilding))
	require.ErrorIs(t, err, store.ErrNotFound)

	// CreateActivity: a valid, own building but a created_by from another
	// tenant is refused too.
	withForeignCreator := carbonActivityFixture(tenantA.Company.ID, tenantA.Buildings[0].ID)
	withForeignCreator.CreatedBy = &foreignUser
	_, err = repo.CreateActivity(ctx, tenantA.AdminScope, withForeignCreator)
	require.ErrorIs(t, err, store.ErrNotFound)

	// UpsertAutomatedActivity: same two checks.
	_, err = repo.UpsertAutomatedActivity(ctx, tenantA.AdminScope, carbonActivityFixture(tenantA.Company.ID, foreignBuilding))
	require.ErrorIs(t, err, store.ErrNotFound)

	withForeignCreator2 := carbonActivityFixture(tenantA.Company.ID, tenantA.Buildings[0].ID)
	withForeignCreator2.CreatedBy = &foreignUser
	_, err = repo.UpsertAutomatedActivity(ctx, tenantA.AdminScope, withForeignCreator2)
	require.ErrorIs(t, err, store.ErrNotFound)

	// CreateReport: tenant A's AdminScope cannot store tenant B's building.
	_, err = repo.CreateReport(ctx, tenantA.AdminScope, model.CarbonReport{
		CompanyID: tenantA.Company.ID, BuildingID: foreignBuilding,
		Name: "x", ReportType: "ghg", Period: "2026-Q1", Payload: []byte(`{}`),
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	// ReplaceSelectedActivities: tenant A's AdminScope cannot replace tenant
	// B's building's selection.
	err = repo.ReplaceSelectedActivities(ctx, tenantA.AdminScope, foreignBuilding, []string{"electricity"})
	require.ErrorIs(t, err, store.ErrNotFound)

	// Nothing was written to tenant B's data by any of the refused attempts.
	activities, err := repo.ListActivities(ctx, tenantB.AdminScope, store.CarbonActivityFilter{})
	require.NoError(t, err)
	require.Empty(t, activities, "no refused write must have landed under tenant B")
}

// TestCarbonActivityFactorIDMustBeOwnedOrPlatform is the mandatory isolation
// test for R1 (fix round 1, Important 1): carbon_activities.factor_id is a
// stored FK that CreateActivity, UpdateActivity and UpsertAutomatedActivity
// must all validate atomically with the write — a platform factor or the
// caller's own is allowed, another company's factor id is refused.
func TestCarbonActivityFactorIDMustBeOwnedOrPlatform(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 4080)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 4081)
	repo := postgres.NewCarbonRepository(pool)

	companyB := tenantB.Company.ID
	foreignFactor, err := repo.UpsertFactor(ctx, tenantB.Scope, model.EmissionFactor{
		CompanyID: &companyB, Key: "foreign", Label: "Foreign", MainCategory: "fuel",
		BaseFactor: carbonDec("1"), BaseUnit: "kg",
	})
	require.NoError(t, err)

	withForeignFactor := carbonActivityFixture(tenantA.Company.ID, tenantA.Buildings[0].ID)
	withForeignFactor.FactorID = &foreignFactor.ID

	_, err = repo.CreateActivity(ctx, tenantA.Scope, withForeignFactor)
	require.ErrorIs(t, err, store.ErrNotFound, "CreateActivity must refuse another company's factor_id")

	_, err = repo.UpsertAutomatedActivity(ctx, tenantA.Scope, withForeignFactor)
	require.ErrorIs(t, err, store.ErrNotFound, "UpsertAutomatedActivity must refuse another company's factor_id")

	// A legitimate own-company activity, then an UPDATE trying to move its
	// factor_id onto tenant B's factor.
	created, err := repo.CreateActivity(ctx, tenantA.Scope, carbonActivityFixture(tenantA.Company.ID, tenantA.Buildings[0].ID))
	require.NoError(t, err)
	created.FactorID = &foreignFactor.ID
	_, err = repo.UpdateActivity(ctx, tenantA.Scope, created)
	require.ErrorIs(t, err, store.ErrNotFound, "UpdateActivity must refuse another company's factor_id")

	stillNoFactor, err := repo.Activity(ctx, tenantA.Scope, created.ID)
	require.NoError(t, err)
	require.Nil(t, stillNoFactor.FactorID, "the refused update must not have moved the factor_id")

	// A platform factor is fine for both create and update.
	var platformFactorID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `insert into emission_factors
		(company_id, key, label, main_category, base_factor, base_unit)
		values (null, 'platform-diesel-4080', 'Diesel', 'fuel', 2.68, 'kg')
		returning id`).Scan(&platformFactorID))

	withPlatformFactor := carbonActivityFixture(tenantA.Company.ID, tenantA.Buildings[0].ID)
	withPlatformFactor.FactorID = &platformFactorID
	createdWithPlatform, err := repo.CreateActivity(ctx, tenantA.Scope, withPlatformFactor)
	require.NoError(t, err)
	require.Equal(t, platformFactorID, *createdWithPlatform.FactorID)

	created.FactorID = &platformFactorID
	updated, err := repo.UpdateActivity(ctx, tenantA.Scope, created)
	require.NoError(t, err)
	require.Equal(t, platformFactorID, *updated.FactorID)

	// And the caller's OWN factor is fine too.
	companyA := tenantA.Company.ID
	ownFactor, err := repo.UpsertFactor(ctx, tenantA.Scope, model.EmissionFactor{
		CompanyID: &companyA, Key: "own-4080", Label: "Own", MainCategory: "fuel",
		BaseFactor: carbonDec("1"), BaseUnit: "kg",
	})
	require.NoError(t, err)
	updated.FactorID = &ownFactor.ID
	updated2, err := repo.UpdateActivity(ctx, tenantA.Scope, updated)
	require.NoError(t, err)
	require.Equal(t, ownFactor.ID, *updated2.FactorID)
}

// TestCarbonCrossTenantAdminScopeReadAndMutationIsolation is the fix round 1,
// Important 4 test: every carbon_activities/carbon_reports/
// carbon_selected_activities method that had NO second-tenant test at all
// (tautology on the company_id predicate would have passed every existing
// TestCarbon* test) now has one, called with the OTHER tenant's AdminScope —
// the widest legitimate scope a single tenant can hold, with no
// building-id-list branch to (wrongly) mask a broken company_id check.
func TestCarbonCrossTenantAdminScopeReadAndMutationIsolation(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 4090)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 4091)
	repo := postgres.NewCarbonRepository(pool)

	activityA, err := repo.CreateActivity(ctx, tenantA.Scope, carbonActivityFixture(tenantA.Company.ID, tenantA.Buildings[0].ID))
	require.NoError(t, err)
	reportA, err := repo.CreateReport(ctx, tenantA.Scope, model.CarbonReport{
		CompanyID: tenantA.Company.ID, BuildingID: tenantA.Buildings[0].ID,
		Name: "A's report", ReportType: "ghg", Period: "2026-Q1", Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	require.NoError(t, repo.ReplaceSelectedActivities(ctx, tenantA.Scope, tenantA.Buildings[0].ID, []string{"electricity"}))

	// Activity: tenant B's AdminScope cannot read tenant A's activity.
	_, err = repo.Activity(ctx, tenantB.AdminScope, activityA.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	// ListActivities: tenant A's activity never appears under tenant B's
	// AdminScope.
	listB, err := repo.ListActivities(ctx, tenantB.AdminScope, store.CarbonActivityFilter{})
	require.NoError(t, err)
	require.Empty(t, listB)

	// UpdateActivity: tenant B's AdminScope, with its own CompanyID (passing
	// the Go-level check), still cannot update tenant A's row by id — the
	// SQL's own company_id predicate is what refuses it.
	borrowed := activityA
	borrowed.CompanyID = tenantB.Company.ID
	borrowed.Quantity = carbonDec("1.000000")
	_, err = repo.UpdateActivity(ctx, tenantB.AdminScope, borrowed)
	require.ErrorIs(t, err, store.ErrNotFound)

	// SetActivityStatus: same, by id alone.
	_, err = repo.SetActivityStatus(ctx, tenantB.AdminScope, activityA.ID, model.CarbonStatusApproved, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)

	// DeleteActivity: same.
	err = repo.DeleteActivity(ctx, tenantB.AdminScope, activityA.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Tenant A's activity is untouched by every refused tenant B attempt.
	stillA, err := repo.Activity(ctx, tenantA.Scope, activityA.ID)
	require.NoError(t, err)
	require.True(t, stillA.Quantity.Equal(carbonDec("1000.500000")), "no refused tenant B write must have touched tenant A's activity")

	// ListReports: tenant A's report never appears under tenant B's
	// AdminScope.
	reportsB, err := repo.ListReports(ctx, tenantB.AdminScope, nil, store.Page{})
	require.NoError(t, err)
	for _, r := range reportsB {
		require.NotEqual(t, reportA.ID, r.ID)
	}

	// SelectedActivities: tenant B's AdminScope reading tenant A's building id
	// sees nothing (the query's own company_id predicate, not a building
	// list, is what excludes it).
	selectedB, err := repo.SelectedActivities(ctx, tenantB.AdminScope, tenantA.Buildings[0].ID)
	require.NoError(t, err)
	require.Empty(t, selectedB)
}
