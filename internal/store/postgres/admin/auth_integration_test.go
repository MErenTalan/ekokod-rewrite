//go:build integration

package admin_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestAdminAuthUserByEmailIsCaseInsensitiveAndExcludesASoftDeletedUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := admin.NewAuthRepository(pool)

	user := tenant.Users[model.UserRoleCompanyAdmin]

	got, err := repo.UserByEmail(ctx, strings.ToUpper(user.Email))
	require.NoError(t, err)
	require.Equal(t, user.ID, got.ID)

	_, err = pool.Exec(ctx, `update users set deleted_at = now() where id = $1`, user.ID)
	require.NoError(t, err)

	_, err = repo.UserByEmail(ctx, user.Email)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestAdminAuthUserByEmailExcludesUsersOfASoftDeletedCompany(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := admin.NewAuthRepository(pool)

	user := tenant.Users[model.UserRoleCompanyAdmin]
	_, err := pool.Exec(ctx, `update companies set deleted_at = now() where id = $1`, tenant.Company.ID)
	require.NoError(t, err)

	_, err = repo.UserByEmail(ctx, user.Email)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestAdminAuthSessionByRefreshTokenHashReturnsRevokedAndExpiredSessions
// pins the two states AdminAuthRepository must NOT filter out, because
// rejecting them is the caller's job, and the deleted-user exclusion it
// must apply on top.
func TestAdminAuthSessionByRefreshTokenHashReturnsRevokedAndExpiredSessions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	authRepo := admin.NewAuthRepository(pool)
	sessRepo := postgres.NewSessionRepository(pool)

	user := tenant.Users[model.UserRoleCompanyAdmin]
	sess, err := sessRepo.Create(ctx, tenant.AdminScope, model.Session{
		UserID: user.ID, RefreshTokenHash: "admin-hash-1",
		ExpiresAt: time.Now().Add(-time.Hour).UTC(), CreatedAt: time.Now().Add(-2 * time.Hour).UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, sessRepo.Revoke(ctx, tenant.AdminScope, sess.ID, time.Now().UTC()))

	gotSess, gotUser, err := authRepo.SessionByRefreshTokenHash(ctx, "admin-hash-1")
	require.NoError(t, err)
	require.Equal(t, sess.ID, gotSess.ID)
	require.NotNil(t, gotSess.RevokedAt, "a revoked session must still be returned")
	require.True(t, gotSess.ExpiresAt.Before(time.Now()), "an expired session must still be returned")
	require.Equal(t, user.ID, gotUser.ID)
	require.Equal(t, tenant.Company.ID, gotUser.CompanyID)

	_, err = pool.Exec(ctx, `update users set deleted_at = now() where id = $1`, user.ID)
	require.NoError(t, err)

	_, _, err = authRepo.SessionByRefreshTokenHash(ctx, "admin-hash-1")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestAdminAuthSessionByRefreshTokenHashExcludesASoftDeletedCompany(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	authRepo := admin.NewAuthRepository(pool)
	sessRepo := postgres.NewSessionRepository(pool)

	user := tenant.Users[model.UserRoleCompanyAdmin]
	_, err := sessRepo.Create(ctx, tenant.AdminScope, model.Session{
		UserID: user.ID, RefreshTokenHash: "admin-hash-2",
		ExpiresAt: time.Now().Add(time.Hour).UTC(), CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `update companies set deleted_at = now() where id = $1`, tenant.Company.ID)
	require.NoError(t, err)

	_, _, err = authRepo.SessionByRefreshTokenHash(ctx, "admin-hash-2")
	require.ErrorIs(t, err, store.ErrNotFound)
}
