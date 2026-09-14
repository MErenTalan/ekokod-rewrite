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

// ---------------------------------------------------------------------------
// CompanyRepository
// ---------------------------------------------------------------------------

// TestCompanyRepositoryOnlyEverSeesItsOwnCompany is CompanyRepository's
// isolation test: a company IS the tenant, so the cross-tenant case is
// "another company's id", never "another company's row of mine".
func TestCompanyRepositoryOnlyEverSeesItsOwnCompany(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewCompanyRepository(pool)

	_, err := repo.Get(ctx, mine.AdminScope, theirs.Company.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.List(ctx, mine.AdminScope, store.CompanyFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, mine.Company.ID, got[0].ID)
}

func TestCompanyRepositoryRejectsAnInvalidScope(t *testing.T) {
	repo := postgres.NewCompanyRepository(testfixtures.NewIsolatedDB(t))
	_, err := repo.List(context.Background(), store.Scope{}, store.CompanyFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestCompanyRepositoryCreateGetUpdateSoftDelete(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := postgres.NewCompanyRepository(pool)

	id := uuid.New()
	scope := store.Scope{CompanyID: id, AllBuildings: true}
	now := time.Now().UTC()

	created, err := repo.Create(ctx, scope, model.Company{
		ID: id, Name: "Acme", CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, id, created.ID)

	got, err := repo.Get(ctx, scope, id)
	require.NoError(t, err)
	require.Equal(t, "Acme", got.Name)

	got.Name = "Acme Corp"
	got.UpdatedAt = time.Now().UTC()
	updated, err := repo.Update(ctx, scope, got)
	require.NoError(t, err)
	require.Equal(t, "Acme Corp", updated.Name)

	require.NoError(t, repo.SoftDelete(ctx, scope, id, time.Now().UTC()))

	_, err = repo.Get(ctx, scope, id)
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.List(ctx, scope, store.CompanyFilter{})
	require.NoError(t, err)
	require.Empty(t, list, "a soft-deleted company must disappear from List too")
}

// TestCompanyRepositoryCreateRefusesAnotherCompanysID pins the header rule
// ("WRITES STORE THE SCOPE'S COMPANY") for the one repository where the
// tenant IS the row: a Scope may only ever create or update its own id.
func TestCompanyRepositoryCreateRefusesAnotherCompanysID(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := postgres.NewCompanyRepository(pool)

	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	now := time.Now().UTC()
	_, err := repo.Create(ctx, scope, model.Company{
		ID: uuid.New(), Name: "Not Mine", CreatedAt: now, UpdatedAt: now,
	})
	require.ErrorIs(t, err, store.ErrNotFound)
}

// ---------------------------------------------------------------------------
// UserRepository
// ---------------------------------------------------------------------------

func TestUserRepositoryNeverReturnsAnotherCompanysRows(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewUserRepository(pool)

	theirUser := theirs.Users[model.UserRoleCompanyAdmin]
	_, err := repo.Get(ctx, mine.AdminScope, theirUser.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.List(ctx, mine.AdminScope, store.UserFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, got, "this assertion is vacuous if the list came back empty")
	for _, u := range got {
		require.Equal(t, mine.Company.ID, u.CompanyID)
	}
}

func TestUserRepositoryRejectsAnInvalidScope(t *testing.T) {
	repo := postgres.NewUserRepository(testfixtures.NewIsolatedDB(t))
	_, err := repo.List(context.Background(), store.Scope{}, store.UserFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestUserRepositoryCreateGetUpdateSoftDelete(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewUserRepository(pool)

	now := time.Now().UTC()
	created, err := repo.Create(ctx, tenant.AdminScope, model.User{
		CompanyID: tenant.Company.ID, Name: "New Hire", Email: "new-hire@tenant-1.example.invalid",
		PasswordHash: "hash", Role: model.UserRoleBuildingAdmin, IsActive: true,
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenant.AdminScope, created.ID)
	require.NoError(t, err)
	require.Equal(t, "New Hire", got.Name)

	got.Name = "Renamed Hire"
	got.UpdatedAt = time.Now().UTC()
	updated, err := repo.Update(ctx, tenant.AdminScope, got)
	require.NoError(t, err)
	require.Equal(t, "Renamed Hire", updated.Name)

	require.NoError(t, repo.SoftDelete(ctx, tenant.AdminScope, created.ID, time.Now().UTC()))

	_, err = repo.Get(ctx, tenant.AdminScope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.List(ctx, tenant.AdminScope, store.UserFilter{})
	require.NoError(t, err)
	for _, u := range list {
		require.NotEqual(t, created.ID, u.ID, "a soft-deleted user must disappear from List too")
	}
}

func TestUserRepositoryCreateRefusesAnotherCompany(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewUserRepository(pool)

	now := time.Now().UTC()
	_, err := repo.Create(ctx, mine.AdminScope, model.User{
		CompanyID: theirs.Company.ID, Name: "Sneaky", Email: "sneaky@tenant-1.example.invalid",
		PasswordHash: "hash", Role: model.UserRoleDemo, IsActive: true, CreatedAt: now, UpdatedAt: now,
	})
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestUserRepositorySetPasswordAndPasswordHistoryIsolateByCompany is the
// mandatory isolation test for the task-8b dispatch row "UserRepository.
// SetPassword, PasswordHistory — user_password_history -> users".
func TestUserRepositorySetPasswordAndPasswordHistoryIsolateByCompany(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewUserRepository(pool)

	theirUser := theirs.Users[model.UserRoleCompanyAdmin]

	err := repo.SetPassword(ctx, mine.AdminScope, theirUser.ID, "stolen-hash", time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)

	// Neither the hash nor a history row was written.
	stillTheirs, err := repo.Get(ctx, theirs.AdminScope, theirUser.ID)
	require.NoError(t, err)
	require.Equal(t, theirUser.PasswordHash, stillTheirs.PasswordHash)

	hist, err := repo.PasswordHistory(ctx, mine.AdminScope, theirUser.ID, 10)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Nil(t, hist)

	myUser := mine.Users[model.UserRoleCompanyAdmin]
	oldHash := myUser.PasswordHash
	require.NoError(t, repo.SetPassword(ctx, mine.AdminScope, myUser.ID, "new-hash-123", time.Now().UTC()))

	got, err := repo.Get(ctx, mine.AdminScope, myUser.ID)
	require.NoError(t, err)
	require.Equal(t, "new-hash-123", got.PasswordHash)

	mine2, err := repo.PasswordHistory(ctx, mine.AdminScope, myUser.ID, 10)
	require.NoError(t, err)
	require.Len(t, mine2, 1)
	require.Equal(t, oldHash, mine2[0].PasswordHash)

	// The other tenant's history is untouched and still invisible to mine.
	theirHist, err := repo.PasswordHistory(ctx, theirs.AdminScope, theirUser.ID, 10)
	require.NoError(t, err)
	require.Empty(t, theirHist)
}

// ---------------------------------------------------------------------------
// SessionRepository
// ---------------------------------------------------------------------------

// TestSessionRepositoryIsolatesReadsAndCreateByCompany covers the reading
// and creating half of the mandatory isolation row "SessionRepository.Get,
// List (f.UserID narrows), Create (sess.UserID must be the company's) ...".
func TestSessionRepositoryIsolatesReadsAndCreateByCompany(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewSessionRepository(pool)

	theirUser := theirs.Users[model.UserRoleCompanyAdmin]
	myUser := mine.Users[model.UserRoleCompanyAdmin]

	_, err := repo.Create(ctx, mine.AdminScope, model.Session{
		UserID: theirUser.ID, RefreshTokenHash: "hash-foreign",
		ExpiresAt: time.Now().Add(time.Hour).UTC(), CreatedAt: time.Now().UTC(),
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	mySess, err := repo.Create(ctx, mine.AdminScope, model.Session{
		UserID: myUser.ID, RefreshTokenHash: "hash-mine",
		ExpiresAt: time.Now().Add(time.Hour).UTC(), CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	theirSess, err := repo.Create(ctx, theirs.AdminScope, model.Session{
		UserID: theirUser.ID, RefreshTokenHash: "hash-theirs",
		ExpiresAt: time.Now().Add(time.Hour).UTC(), CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	_, err = repo.Get(ctx, mine.AdminScope, theirSess.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.List(ctx, mine.AdminScope, store.SessionFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, mySess.ID, got[0].ID)

	// f.UserID narrows but never widens: naming another tenant's user from
	// this scope yields nothing at all, never their session.
	narrowed, err := repo.List(ctx, mine.AdminScope, store.SessionFilter{UserID: &theirUser.ID})
	require.NoError(t, err)
	require.Empty(t, narrowed)
}

// TestSessionRepositoryRevokeIsolatesByCompany covers Revoke and
// RevokeAllForUser from the same mandatory isolation row.
func TestSessionRepositoryRevokeIsolatesByCompany(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewSessionRepository(pool)

	theirUser := theirs.Users[model.UserRoleCompanyAdmin]
	theirSess1, err := repo.Create(ctx, theirs.AdminScope, model.Session{
		UserID: theirUser.ID, RefreshTokenHash: "hash-revoke-1",
		ExpiresAt: time.Now().Add(time.Hour).UTC(), CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	theirSess2, err := repo.Create(ctx, theirs.AdminScope, model.Session{
		UserID: theirUser.ID, RefreshTokenHash: "hash-revoke-2",
		ExpiresAt: time.Now().Add(time.Hour).UTC(), CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	err = repo.Revoke(ctx, mine.AdminScope, theirSess1.ID, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.RevokeAllForUser(ctx, mine.AdminScope, theirUser.ID, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)

	// Neither call touched anything: the rightful owner still revokes both.
	require.NoError(t, repo.Revoke(ctx, theirs.AdminScope, theirSess1.ID, time.Now().UTC()))
	n, err := repo.RevokeAllForUser(ctx, theirs.AdminScope, theirUser.ID, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "only theirSess2 was still active; theirSess1 was already revoked above")

	got, err := repo.Get(ctx, theirs.AdminScope, theirSess2.ID)
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)
}

// TestSessionRepositoryDeleteExpiredOnlyTouchesTheCompanysUsers is the last
// method in the same mandatory isolation row.
func TestSessionRepositoryDeleteExpiredOnlyTouchesTheCompanysUsers(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewSessionRepository(pool)

	theirUser := theirs.Users[model.UserRoleCompanyAdmin]
	theirExpired, err := repo.Create(ctx, theirs.AdminScope, model.Session{
		UserID: theirUser.ID, RefreshTokenHash: "hash-expired",
		ExpiresAt: time.Now().Add(-time.Minute).UTC(), CreatedAt: time.Now().Add(-time.Hour).UTC(),
	})
	require.NoError(t, err)

	n, err := repo.DeleteExpired(ctx, mine.AdminScope, time.Now().UTC())
	require.NoError(t, err)
	require.Zero(t, n, "another tenant's expired session must survive this sweep")

	stillThere, err := repo.Get(ctx, theirs.AdminScope, theirExpired.ID)
	require.NoError(t, err)
	require.Nil(t, stillThere.RevokedAt)

	n, err = repo.DeleteExpired(ctx, theirs.AdminScope, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
}

func TestSessionRepositoryRejectsAnInvalidScope(t *testing.T) {
	repo := postgres.NewSessionRepository(testfixtures.NewIsolatedDB(t))
	_, err := repo.List(context.Background(), store.Scope{}, store.SessionFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

// ---------------------------------------------------------------------------
// AuditRepository
// ---------------------------------------------------------------------------

// TestAuditRepositoryAppendRefusesPlatformAndForeignEntries is the mandatory
// nullable-company_id test: Append stores and sees only company_id =
// s.CompanyID, never a NULL (platform) row.
func TestAuditRepositoryAppendRefusesPlatformAndForeignEntries(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewAuditRepository(pool)

	_, err := repo.Append(ctx, mine.AdminScope, model.AuditEntry{
		Action: "platform.action", EntityType: "system", CreatedAt: time.Now().UTC(),
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	foreign := theirs.Company.ID
	_, err = repo.Append(ctx, mine.AdminScope, model.AuditEntry{
		CompanyID: &foreign, Action: "x", EntityType: "y", CreatedAt: time.Now().UTC(),
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	mineID := mine.Company.ID
	entry, err := repo.Append(ctx, mine.AdminScope, model.AuditEntry{
		CompanyID: &mineID, Action: "building.update", EntityType: "building", CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NotZero(t, entry.ID)

	got, err := repo.List(ctx, mine.AdminScope, store.AuditFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, mineID, *got[0].CompanyID)

	// theirs's own scope must never see mine's row either.
	theirList, err := repo.List(ctx, theirs.AdminScope, store.AuditFilter{})
	require.NoError(t, err)
	require.Empty(t, theirList)
}

func TestAuditRepositoryRejectsAnInvalidScope(t *testing.T) {
	repo := postgres.NewAuditRepository(testfixtures.NewIsolatedDB(t))
	_, err := repo.List(context.Background(), store.Scope{}, store.AuditFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestAuditRepositoryListRejectsAnInvalidRange(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewAuditRepository(pool)

	bad := store.TimeRange{}
	_, err := repo.List(ctx, tenant.AdminScope, store.AuditFilter{Range: &bad})
	require.ErrorIs(t, err, store.ErrInvalidRange)
}
