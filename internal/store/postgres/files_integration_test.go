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

func fileFixture(companyID uuid.UUID, ownerType string, ownerID *uuid.UUID) model.StoredFile {
	return model.StoredFile{
		CompanyID: companyID, OwnerType: ownerType, OwnerID: ownerID,
		OriginalName: "report.pdf", StoredPath: "/objects/report.pdf", ContentType: "application/pdf",
		SizeBytes: 1024, Checksum: "deadbeef",
	}
}

func TestFileCreateGetListSoftDelete(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 6001)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 6002)
	repo := postgres.NewFileRepository(pool)

	ownerID := tenantA.Buildings[0].ID
	created, err := repo.Create(ctx, tenantA.Scope, fileFixture(tenantA.Company.ID, "carbon", &ownerID))
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenantA.Scope, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)

	// Another company cannot read, list or soft-delete it: it is
	// indistinguishable from a missing file.
	_, err = repo.Get(ctx, tenantB.Scope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	listB, err := repo.List(ctx, tenantB.Scope, store.FileFilter{})
	require.NoError(t, err)
	require.Empty(t, listB)

	err = repo.SoftDelete(ctx, tenantB.Scope, created.ID, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)

	listA, err := repo.List(ctx, tenantA.Scope, store.FileFilter{})
	require.NoError(t, err)
	require.Len(t, listA, 1)

	require.NoError(t, repo.SoftDelete(ctx, tenantA.Scope, created.ID, time.Now().UTC()))

	afterDelete, err := repo.List(ctx, tenantA.Scope, store.FileFilter{})
	require.NoError(t, err)
	require.Empty(t, afterDelete, "a soft-deleted file must not appear in an ordinary list")

	withDeleted, err := repo.List(ctx, tenantA.Scope, store.FileFilter{IncludeDeleted: true})
	require.NoError(t, err)
	require.Len(t, withDeleted, 1)

	_, err = repo.Get(ctx, tenantA.Scope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "Get never returns a soft-deleted row")
}

func TestFileCreateRefusesMismatchedCompanyID(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 6010)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 6011)
	repo := postgres.NewFileRepository(pool)

	_, err := repo.Create(ctx, tenantA.Scope, fileFixture(tenantB.Company.ID, "carbon", nil))
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestFileCreateRefusesAnotherTenantsUploadedBy proves uploaded_by, when
// set, must name a user of the caller's own company — checked even under
// AdminScope, since Scope.AllowsBuilding-style reasoning (AllBuildings makes
// everything look permitted) must never be what authorises a stored foreign
// key.
func TestFileCreateRefusesAnotherTenantsUploadedBy(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 6020)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 6021)
	repo := postgres.NewFileRepository(pool)

	f := fileFixture(tenantA.Company.ID, "carbon", nil)
	foreignUser := tenantB.Users[model.UserRoleCompanyAdmin].ID
	f.UploadedBy = &foreignUser

	_, err := repo.Create(ctx, tenantA.AdminScope, f)
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.List(ctx, tenantA.Scope, store.FileFilter{})
	require.NoError(t, err)
	require.Empty(t, list, "the refused create must not have written anything")

	ownUser := tenantA.Users[model.UserRoleCompanyAdmin].ID
	f.UploadedBy = &ownUser
	created, err := repo.Create(ctx, tenantA.Scope, f)
	require.NoError(t, err)
	require.Equal(t, ownUser, *created.UploadedBy)
}

func TestFileRepositoryRejectsInvalidScope(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	repo := postgres.NewFileRepository(pool)
	var invalid store.Scope

	_, err := repo.Get(ctx, invalid, uuid.New())
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.List(ctx, invalid, store.FileFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
	err = repo.SoftDelete(ctx, invalid, uuid.New(), time.Now())
	require.ErrorIs(t, err, store.ErrInvalidScope)
}
