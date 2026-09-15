//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestBuildingRepositoryNeverReturnsAnotherCompanysRows(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewBuildingRepository(pool)

	_, err := repo.Get(ctx, mine.AdminScope, theirs.Buildings[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound,
		"another company's building must be indistinguishable from a missing one")

	got, err := repo.List(ctx, mine.AdminScope, store.BuildingFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, got, "this assertion is vacuous if the list came back empty")
	for _, b := range got {
		require.Equal(t, mine.Company.ID, b.CompanyID)
	}
}

func TestBuildingRepositoryHonoursANarrowScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewBuildingRepository(pool)

	got, err := repo.List(ctx, tenant.Scope, store.BuildingFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1, "a scope naming one building must not return the others")
	require.Equal(t, tenant.Buildings[0].ID, got[0].ID)

	// Get must honour the same narrow scope, not just company_id.
	_, err = repo.Get(ctx, tenant.Scope, tenant.Buildings[1].ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestBuildingRepositoryRejectsAnInvalidScope(t *testing.T) {
	repo := postgres.NewBuildingRepository(testfixtures.NewIsolatedDB(t))
	_, err := repo.List(context.Background(), store.Scope{}, store.BuildingFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestBuildingRepositoryCreateGetUpdateSoftDelete(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewBuildingRepository(pool)

	now := time.Now().UTC()
	created, err := repo.Create(ctx, tenant.AdminScope, model.Building{
		CompanyID: tenant.Company.ID, Name: "New Site", BillCutoffDay: 1,
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenant.AdminScope, created.ID)
	require.NoError(t, err)
	require.Equal(t, "New Site", got.Name)

	got.Name = "Renamed Site"
	got.UpdatedAt = time.Now().UTC()
	updated, err := repo.Update(ctx, tenant.AdminScope, got)
	require.NoError(t, err)
	require.Equal(t, "Renamed Site", updated.Name)

	require.NoError(t, repo.SoftDelete(ctx, tenant.AdminScope, created.ID, time.Now().UTC()))

	_, err = repo.Get(ctx, tenant.AdminScope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.List(ctx, tenant.AdminScope, store.BuildingFilter{})
	require.NoError(t, err)
	for _, b := range list {
		require.NotEqual(t, created.ID, b.ID, "a soft-deleted building must disappear from List too")
	}
}

// TestBuildingRepositoryUpdateRefusesAForeignCompany is the fix-round-1 proof
// for Important 3: Update must refuse a model value whose CompanyID names
// another company, exactly like Create does, and write nothing.
//
// The row named by b.ID belongs to mine's OWN company throughout —
// BuildingUpdate's WHERE clause already scopes by s.CompanyID regardless of
// this guard, so a victim row from ANOTHER tenant would make this test pass
// whether or not the guard exists. Only a model value that lies about its
// OWN row's CompanyID exercises the guard.
func TestBuildingRepositoryUpdateRefusesAForeignCompany(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewBuildingRepository(pool)

	myBuilding := mine.Buildings[0]
	tampered := myBuilding
	tampered.CompanyID = theirs.Company.ID
	tampered.Name = "Renamed By A Lying CompanyID"
	tampered.UpdatedAt = time.Now().UTC()

	_, err := repo.Update(ctx, mine.AdminScope, tampered)
	require.ErrorIs(t, err, store.ErrNotFound)

	still, err := repo.Get(ctx, mine.AdminScope, myBuilding.ID)
	require.NoError(t, err)
	require.Equal(t, myBuilding.Name, still.Name, "nothing may be written by a refused Update")
}

// TestBuildingRepositoryCreateRequiresAnAllBuildingsScope pins the
// fix-round-1 controller ruling on the folded minor: a narrow Scope cannot
// create a building outside its own grant, because it has no way to grant
// itself a new building id.
func TestBuildingRepositoryCreateRequiresAnAllBuildingsScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewBuildingRepository(pool)

	now := time.Now().UTC()
	_, err := repo.Create(ctx, tenant.Scope, model.Building{
		CompanyID: tenant.Company.ID, Name: "Narrow Cannot Create", BillCutoffDay: 1,
		CreatedAt: now, UpdatedAt: now,
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.Create(ctx, tenant.AdminScope, model.Building{
		CompanyID: tenant.Company.ID, Name: "AllBuildings Can Create", BillCutoffDay: 1,
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err, "an AllBuildings scope must still be able to create")
}

// TestBuildingRepositoryContactsExcludeASoftDeletedParent pins
// wave-f-context rule 4 for BuildingContactsList's LEFT JOIN shape: a child
// of a soft-deleted parent must not be readable through the parent-scoped
// query either.
func TestBuildingRepositoryContactsExcludeASoftDeletedParent(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewBuildingRepository(pool)

	name := "Site Manager"
	_, err := repo.ReplaceContacts(ctx, tenant.AdminScope, tenant.Buildings[0].ID,
		[]model.BuildingContact{{Name: &name}})
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, tenant.AdminScope, tenant.Buildings[0].ID, time.Now().UTC()))

	_, err = repo.Contacts(ctx, tenant.AdminScope, tenant.Buildings[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound,
		"a soft-deleted building's contacts must not stay reachable through the join")
}

// TestBuildingRepositoryReplaceContactsRefusesANarrowScopeOutsideItsGrant
// pins the folded minor: a narrow Scope (Buildings[0] only) may not replace
// the contacts of Buildings[1], even inside its own company.
func TestBuildingRepositoryReplaceContactsRefusesANarrowScopeOutsideItsGrant(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewBuildingRepository(pool)

	name := "Outside The Grant"
	_, err := repo.ReplaceContacts(ctx, tenant.Scope, tenant.Buildings[1].ID,
		[]model.BuildingContact{{Name: &name}})
	require.ErrorIs(t, err, store.ErrNotFound)

	// Nothing was written: the building's own (admin-scoped) contact list is
	// still empty.
	got, err := repo.Contacts(ctx, tenant.AdminScope, tenant.Buildings[1].ID)
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestBuildingRepositoryCreateRefusesAForeignResponsibleUser is also the
// controller's second-dispatch ruling proof: an AdminScope (AllBuildings)
// write storing tenant B's user id as a foreign key must be refused with
// ErrNotFound, and B's own data must be untouched. This is the exact shape
// of bug a sibling task hit: a Go-level check using Scope.AllowsBuilding()
// (or an equivalent building-only check) says nothing about company_id, so
// under AllBuildings it can wrongly authorise ANY id, including another
// tenant's. BuildingCreate's WHERE clause embeds the company_id check
// itself now (queries/buildings.sql), so this is provably not that bug.
func TestBuildingRepositoryCreateRefusesAForeignResponsibleUser(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewBuildingRepository(pool)

	theirsBuildingsBefore, err := repo.List(ctx, theirs.AdminScope, store.BuildingFilter{})
	require.NoError(t, err)

	foreignUser := theirs.Users[model.UserRoleCompanyAdmin].ID
	now := time.Now().UTC()
	_, err = repo.Create(ctx, mine.AdminScope, model.Building{
		CompanyID: mine.Company.ID, Name: "Bad", BillCutoffDay: 1,
		ResponsibleUserID: &foreignUser, CreatedAt: now, UpdatedAt: now,
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	theirsBuildingsAfter, err := repo.List(ctx, theirs.AdminScope, store.BuildingFilter{})
	require.NoError(t, err)
	require.Equal(t, theirsBuildingsBefore, theirsBuildingsAfter, "theirs's own buildings must be completely unaffected")
}

// TestBuildingRepositoryUpdateRefusesAForeignResponsibleUser is Update's
// counterpart to the Create proof above: the same tenant-B-id-as-FK refusal,
// on the path that used to rely SOLELY on a Go-level pre-check with no SQL
// enforcement of its own.
func TestBuildingRepositoryUpdateRefusesAForeignResponsibleUser(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewBuildingRepository(pool)

	foreignUser := theirs.Users[model.UserRoleCompanyAdmin].ID
	myBuilding := mine.Buildings[0]
	tampered := myBuilding
	tampered.ResponsibleUserID = &foreignUser
	tampered.UpdatedAt = time.Now().UTC()

	_, err := repo.Update(ctx, mine.AdminScope, tampered)
	require.ErrorIs(t, err, store.ErrNotFound)

	still, err := repo.Get(ctx, mine.AdminScope, myBuilding.ID)
	require.NoError(t, err)
	require.Equal(t, myBuilding.ResponsibleUserID, still.ResponsibleUserID, "nothing may be written by a refused Update")
}

// TestBuildingRepositoryContactsIsolateThroughTheParentBuilding is the
// mandatory isolation test for "BuildingRepository.Contacts,
// ReplaceContacts — building_contacts -> buildings".
func TestBuildingRepositoryContactsIsolateThroughTheParentBuilding(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewBuildingRepository(pool)

	_, err := repo.Contacts(ctx, mine.AdminScope, theirs.Buildings[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	foreignName := "x"
	_, err = repo.ReplaceContacts(ctx, mine.AdminScope, theirs.Buildings[0].ID,
		[]model.BuildingContact{{Name: &foreignName}})
	require.ErrorIs(t, err, store.ErrNotFound)

	// Nothing was written to theirs's building either.
	theirContacts, err := repo.Contacts(ctx, theirs.AdminScope, theirs.Buildings[0].ID)
	require.NoError(t, err)
	require.Empty(t, theirContacts)

	// A visible building with no contacts yet returns empty, not ErrNotFound.
	empty, err := repo.Contacts(ctx, mine.AdminScope, mine.Buildings[0].ID)
	require.NoError(t, err)
	require.Empty(t, empty)

	name := "Site Manager"
	phone := "+90 555 000 0000"
	replaced, err := repo.ReplaceContacts(ctx, mine.AdminScope, mine.Buildings[0].ID, []model.BuildingContact{
		{Name: &name, Phone: &phone, SortOrder: 0},
	})
	require.NoError(t, err)
	require.Len(t, replaced, 1)

	got, err := repo.Contacts(ctx, mine.AdminScope, mine.Buildings[0].ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, name, *got[0].Name)

	// The narrow Scope (Buildings[0] only) sees the same contacts, but
	// Buildings[1] stays out of its reach.
	sameContacts, err := repo.Contacts(ctx, mine.Scope, mine.Buildings[0].ID)
	require.NoError(t, err)
	require.Len(t, sameContacts, 1)

	_, err = repo.Contacts(ctx, mine.Scope, mine.Buildings[1].ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}
