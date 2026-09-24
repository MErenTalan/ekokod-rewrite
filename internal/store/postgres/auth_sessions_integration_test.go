//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func adminAuthRepo(pool *pgxpool.Pool) *admin.AuthRepository { return admin.NewAuthRepository(pool) }

func newSession(t *testing.T, ctx context.Context, repo *postgres.SessionRepository, sc store.Scope, userID uuid.UUID, hash string, at time.Time) model.Session {
	t.Helper()
	fp := "fp-" + hash
	sess, err := repo.Create(ctx, sc, model.Session{
		UserID: userID, RefreshTokenHash: hash, DeviceFingerprint: &fp,
		Client: "mobile", Remember: true, ExpiresAt: at.Add(720 * time.Hour), CreatedAt: at,
	})
	require.NoError(t, err)
	return sess
}

func TestSessionCreateStoresClientAndRemember(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewSessionRepository(pool)
	at := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	user := tenant.Users[model.UserRoleCompanyAdmin]

	sess := newSession(t, ctx, repo, tenant.AdminScope, user.ID, "h-create", at)
	got, err := repo.Get(ctx, tenant.AdminScope, sess.ID)
	require.NoError(t, err)
	require.Equal(t, "mobile", got.Client)
	require.True(t, got.Remember)
	require.Nil(t, got.LastUsedAt)
	require.Nil(t, got.RevokedReason)

	_, err = repo.Create(ctx, tenant.AdminScope, model.Session{UserID: user.ID, RefreshTokenHash: "h-bad", Client: "desktop", ExpiresAt: at.Add(time.Hour)})
	require.Error(t, err, "client is constrained to web|mobile")
}

func TestSessionRotateInsertsAndRevokesAtomically(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	other := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewSessionRepository(pool)
	at := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	user := tenant.Users[model.UserRoleCompanyAdmin]
	old := newSession(t, ctx, repo, tenant.AdminScope, user.ID, "h-old", at)

	next := old
	next.ID = uuid.Nil
	next.RefreshTokenHash = "h-next"
	rotated, err := repo.Rotate(ctx, tenant.AdminScope, old.ID, next, at.Add(time.Hour))
	require.NoError(t, err)
	require.NotEqual(t, old.ID, rotated.ID)
	require.NotNil(t, rotated.RotatedFrom)
	require.Equal(t, old.ID, *rotated.RotatedFrom)
	require.True(t, rotated.ExpiresAt.Equal(old.ExpiresAt))
	require.Equal(t, "mobile", rotated.Client)
	require.True(t, rotated.Remember)

	gotOld, err := repo.Get(ctx, tenant.AdminScope, old.ID)
	require.NoError(t, err)
	require.NotNil(t, gotOld.RevokedAt)
	require.Equal(t, "rotated", *gotOld.RevokedReason)

	again := old
	again.ID = uuid.Nil
	again.RefreshTokenHash = "h-again"
	_, err = repo.Rotate(ctx, tenant.AdminScope, old.ID, again, at.Add(2*time.Hour))
	require.ErrorIs(t, err, store.ErrConflict)
	list, err := repo.List(ctx, tenant.AdminScope, store.SessionFilter{UserID: &user.ID})
	require.NoError(t, err)
	require.Len(t, list, 2, "a refused rotation inserts nothing")

	foreign := rotated
	foreign.ID = uuid.Nil
	foreign.RefreshTokenHash = "h-foreign"
	_, err = repo.Rotate(ctx, other.AdminScope, rotated.ID, foreign, at.Add(3*time.Hour))
	require.ErrorIs(t, err, store.ErrNotFound)
	stillLive, err := repo.Get(ctx, tenant.AdminScope, rotated.ID)
	require.NoError(t, err)
	require.Nil(t, stillLive.RevokedAt)
}

func TestSessionRevokeWithReasonAndAllExcept(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	other := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewSessionRepository(pool)
	at := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	user := tenant.Users[model.UserRoleCompanyAdmin]
	s1 := newSession(t, ctx, repo, tenant.AdminScope, user.ID, "h1", at)
	s2 := newSession(t, ctx, repo, tenant.AdminScope, user.ID, "h2", at)
	s3 := newSession(t, ctx, repo, tenant.AdminScope, user.ID, "h3", at)

	require.ErrorIs(t, repo.RevokeWithReason(ctx, other.AdminScope, s1.ID, "logout", at), store.ErrNotFound)
	require.NoError(t, repo.RevokeWithReason(ctx, tenant.AdminScope, s1.ID, "device_mismatch", at))
	got, err := repo.Get(ctx, tenant.AdminScope, s1.ID)
	require.NoError(t, err)
	require.Equal(t, "device_mismatch", *got.RevokedReason)
	require.Error(t, repo.RevokeWithReason(ctx, tenant.AdminScope, s2.ID, "bogus", at), "reason is constrained")

	_, err = repo.RevokeAllForUserWithReason(ctx, other.AdminScope, user.ID, "logout_all", nil, at)
	require.ErrorIs(t, err, store.ErrNotFound)

	n, err := repo.RevokeAllForUserWithReason(ctx, tenant.AdminScope, user.ID, "password_change", &s2.ID, at)
	require.NoError(t, err)
	require.EqualValues(t, 1, n, "s1 already revoked, s2 excepted, s3 revoked")
	keep, err := repo.Get(ctx, tenant.AdminScope, s2.ID)
	require.NoError(t, err)
	require.Nil(t, keep.RevokedAt)
	gone, err := repo.Get(ctx, tenant.AdminScope, s3.ID)
	require.NoError(t, err)
	require.Equal(t, "password_change", *gone.RevokedReason)
	first, err := repo.Get(ctx, tenant.AdminScope, s1.ID)
	require.NoError(t, err)
	require.Equal(t, "device_mismatch", *first.RevokedReason, "an earlier revocation reason is kept")
}

func TestSessionTouchThrottles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	other := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewSessionRepository(pool)
	at := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	sess := newSession(t, ctx, repo, tenant.AdminScope, tenant.Users[model.UserRoleCompanyAdmin].ID, "ht", at)

	require.ErrorIs(t, repo.Touch(ctx, other.AdminScope, sess.ID, at), store.ErrNotFound)
	require.NoError(t, repo.Touch(ctx, tenant.AdminScope, sess.ID, at))
	lastUsed := func() time.Time {
		got, err := repo.Get(ctx, tenant.AdminScope, sess.ID)
		require.NoError(t, err)
		require.NotNil(t, got.LastUsedAt)
		return *got.LastUsedAt
	}
	require.True(t, lastUsed().Equal(at))
	require.NoError(t, repo.Touch(ctx, tenant.AdminScope, sess.ID, at.Add(time.Minute)))
	require.True(t, lastUsed().Equal(at), "touches within 5 minutes are skipped")
	require.NoError(t, repo.Touch(ctx, tenant.AdminScope, sess.ID, at.Add(6*time.Minute)))
	require.True(t, lastUsed().Equal(at.Add(6*time.Minute)))
}

func TestPasswordResetCreateInvalidatesPreviousAndMarkUsed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	other := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewPasswordResetRepository(pool)
	adminAuth := adminAuthRepo(pool)
	at := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	user := tenant.Users[model.UserRoleCompanyAdmin]

	_, err := repo.Create(ctx, other.AdminScope, model.PasswordReset{UserID: user.ID, TokenHash: "r0", ExpiresAt: at.Add(time.Hour)})
	require.ErrorIs(t, err, store.ErrNotFound)

	first, err := repo.Create(ctx, tenant.AdminScope, model.PasswordReset{UserID: user.ID, TokenHash: "r1", ExpiresAt: at.Add(time.Hour), CreatedAt: at})
	require.NoError(t, err)
	second, err := repo.Create(ctx, tenant.AdminScope, model.PasswordReset{UserID: user.ID, TokenHash: "r2", ExpiresAt: at.Add(time.Hour), CreatedAt: at.Add(time.Minute)})
	require.NoError(t, err)

	gotFirst, gotUser, err := adminAuth.PasswordResetByTokenHash(ctx, "r1")
	require.NoError(t, err)
	require.Equal(t, user.ID, gotUser.ID)
	require.NotNil(t, gotFirst.UsedAt, "creating a new token invalidates the previous unused one")
	gotSecond, _, err := adminAuth.PasswordResetByTokenHash(ctx, "r2")
	require.NoError(t, err)
	require.Nil(t, gotSecond.UsedAt)

	require.ErrorIs(t, repo.MarkUsed(ctx, other.AdminScope, second.ID, at), store.ErrNotFound)
	require.NoError(t, repo.MarkUsed(ctx, tenant.AdminScope, second.ID, at.Add(2*time.Minute)))
	require.ErrorIs(t, repo.MarkUsed(ctx, tenant.AdminScope, second.ID, at.Add(3*time.Minute)), store.ErrConflict)
	_ = first

	_, _, err = adminAuth.PasswordResetByTokenHash(ctx, "missing")
	require.ErrorIs(t, err, store.ErrNotFound)

	require.NoError(t, postgres.NewUserRepository(pool).SoftDelete(ctx, tenant.AdminScope, user.ID, at))
	_, _, err = adminAuth.PasswordResetByTokenHash(ctx, "r2")
	require.ErrorIs(t, err, store.ErrNotFound, "a deleted user's reset token resolves to nothing")
}

func TestUserPreferencesRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewUserRepository(pool)
	user := tenant.Users[model.UserRoleCompanyAdmin]
	require.Equal(t, "tr", user.Locale, "fixture users default to tr")

	prefs := strings.Repeat("x", 512)
	user.Locale = "en"
	user.UIPreferences = &prefs
	user.UpdatedAt = time.Now().UTC()
	updated, err := repo.Update(ctx, tenant.AdminScope, user)
	require.NoError(t, err)
	require.Equal(t, "en", updated.Locale)
	require.Equal(t, prefs, *updated.UIPreferences)

	tooLong := prefs + "y"
	user.UIPreferences = &tooLong
	_, err = repo.Update(ctx, tenant.AdminScope, user)
	require.Error(t, err)
	user.UIPreferences = &prefs
	user.Locale = "de"
	_, err = repo.Update(ctx, tenant.AdminScope, user)
	require.Error(t, err)
}
