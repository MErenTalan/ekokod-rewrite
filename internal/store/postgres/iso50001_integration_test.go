//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func iso50001DatePtr(t time.Time) *time.Time { return &t }

func TestISO50001EnsureProjectIsIdempotentAndScoped(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 5001)
	repo := postgres.NewISO50001Repository(pool)

	buildingID := tenant.Buildings[0].ID
	first, err := repo.EnsureProject(ctx, tenant.Scope, buildingID)
	require.NoError(t, err)

	second, err := repo.EnsureProject(ctx, tenant.Scope, buildingID)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "building_id is unique: this must not create a second project")

	got, err := repo.Project(ctx, tenant.Scope, buildingID)
	require.NoError(t, err)
	require.Equal(t, first.ID, got.ID)

	// A narrow scope cannot ensure or read Buildings[1]'s project.
	_, err = repo.EnsureProject(ctx, tenant.Scope, tenant.Buildings[1].ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = repo.Project(ctx, tenant.Scope, tenant.Buildings[1].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	// The admin scope can.
	admin, err := repo.EnsureProject(ctx, tenant.AdminScope, tenant.Buildings[1].ID)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, admin.ID)
}

// TestISO50001ClauseDatesIsolation is the mandatory isolation test:
// iso50001_clause_dates has no company_id, so a project not visible to the
// Scope must return ErrNotFound for both the read and ReplaceClauseDates.
func TestISO50001ClauseDatesIsolation(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 5010)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 5011)
	repo := postgres.NewISO50001Repository(pool)

	project, err := repo.EnsureProject(ctx, tenantA.Scope, tenantA.Buildings[0].ID)
	require.NoError(t, err)

	dates := []model.ISO50001ClauseDate{
		{ClauseID: "5", StartDate: iso50001DatePtr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), EndDate: iso50001DatePtr(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))},
		{ClauseID: "6", StartDate: nil, EndDate: nil},
	}
	require.NoError(t, repo.ReplaceClauseDates(ctx, tenantA.Scope, project.ID, dates))

	got, err := repo.ClauseDates(ctx, tenantA.Scope, project.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Tenant B cannot read or replace tenant A's project's clause dates,
	// including under tenant B's OWN AdminScope: AllBuildings bypasses the
	// building-narrowing branch entirely, so this is what proves the
	// company_id predicate itself — not just the building-id-list check —
	// is load-bearing.
	_, err = repo.ClauseDates(ctx, tenantB.Scope, project.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.ReplaceClauseDates(ctx, tenantB.Scope, project.ID, nil)
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.ReplaceClauseDates(ctx, tenantB.AdminScope, project.ID, nil)
	require.ErrorIs(t, err, store.ErrNotFound)

	stillA, err := repo.ClauseDates(ctx, tenantA.Scope, project.ID)
	require.NoError(t, err)
	require.Len(t, stillA, 2, "tenant B's refused replace must not have touched tenant A's rows")

	// Replacing converges: fewer rows the second time.
	require.NoError(t, repo.ReplaceClauseDates(ctx, tenantA.Scope, project.ID, dates[:1]))
	got, err = repo.ClauseDates(ctx, tenantA.Scope, project.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

// TestISO50001NotesIsolation is the mandatory isolation test for
// iso50001_notes.
func TestISO50001NotesIsolation(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 5020)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 5021)
	repo := postgres.NewISO50001Repository(pool)

	project, err := repo.EnsureProject(ctx, tenantA.Scope, tenantA.Buildings[0].ID)
	require.NoError(t, err)
	createdBy := tenantA.Users[model.UserRoleCompanyAdmin].ID

	note, err := repo.CreateNote(ctx, tenantA.Scope, model.ISO50001Note{
		ProjectID: project.ID, ClauseID: "5.1", Body: "Initial note", CreatedBy: &createdBy,
	})
	require.NoError(t, err)

	list, err := repo.Notes(ctx, tenantA.Scope, project.ID, nil)
	require.NoError(t, err)
	require.Len(t, list, 1)

	filtered, err := repo.Notes(ctx, tenantA.Scope, project.ID, iso50001PtrString("5.1"))
	require.NoError(t, err)
	require.Len(t, filtered, 1)

	note.Body = "Updated note"
	updated, err := repo.UpdateNote(ctx, tenantA.Scope, note)
	require.NoError(t, err)
	require.Equal(t, "Updated note", updated.Body)

	// Tenant B cannot read, create, update or delete tenant A's project's
	// notes.
	_, err = repo.Notes(ctx, tenantB.Scope, project.ID, nil)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.CreateNote(ctx, tenantB.Scope, model.ISO50001Note{ProjectID: project.ID, ClauseID: "5.1", Body: "hack"})
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.UpdateNote(ctx, tenantB.Scope, model.ISO50001Note{ID: note.ID, ProjectID: project.ID, ClauseID: "5.1", Body: "hack"})
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.DeleteNote(ctx, tenantB.Scope, note.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	stillThere, err := repo.Notes(ctx, tenantA.Scope, project.ID, nil)
	require.NoError(t, err)
	require.Len(t, stillThere, 1, "tenant B's refused writes must not have touched tenant A's note")

	// CreateNote also refuses a created_by that is not a user of the
	// caller's company.
	otherUser := tenantB.Users[model.UserRoleCompanyAdmin].ID
	_, err = repo.CreateNote(ctx, tenantA.Scope, model.ISO50001Note{
		ProjectID: project.ID, ClauseID: "6.1", Body: "x", CreatedBy: &otherUser,
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	require.NoError(t, repo.DeleteNote(ctx, tenantA.Scope, note.ID))
	empty, err := repo.Notes(ctx, tenantA.Scope, project.ID, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func iso50001PtrString(s string) *string { return &s }

func TestISO50001RepositoryRejectsInvalidScope(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	repo := postgres.NewISO50001Repository(pool)
	var invalid store.Scope

	_, err := repo.Project(ctx, invalid, uuid.New())
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.ClauseDates(ctx, invalid, uuid.New())
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.Notes(ctx, invalid, uuid.New(), nil)
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

// TestISO50001AdminScopeCannotStoreAnotherTenantsForeignKeys proves that
// AdminScope (AllBuildings: true) cannot be used to store another tenant's
// building or user id: Scope.AllowsBuilding returns true for any id at all
// under AllBuildings, so the real check lives in each write's own SQL.
func TestISO50001AdminScopeCannotStoreAnotherTenantsForeignKeys(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 5030)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 5031)
	repo := postgres.NewISO50001Repository(pool)

	_, err := repo.EnsureProject(ctx, tenantA.AdminScope, tenantB.Buildings[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	project, err := repo.EnsureProject(ctx, tenantA.AdminScope, tenantA.Buildings[0].ID)
	require.NoError(t, err)

	foreignUser := tenantB.Users[model.UserRoleCompanyAdmin].ID
	_, err = repo.CreateNote(ctx, tenantA.AdminScope, model.ISO50001Note{
		ProjectID: project.ID, ClauseID: "5.1", Body: "x", CreatedBy: &foreignUser,
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	notes, err := repo.Notes(ctx, tenantA.AdminScope, project.ID, nil)
	require.NoError(t, err)
	require.Empty(t, notes, "the refused CreateNote must not have written anything")
}
