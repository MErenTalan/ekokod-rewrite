//go:build integration

package seed_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestE2EFixturesIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	hasher := auth.Hasher{Pepper: []byte("pepper-0123456789abcdef-0123456789"), Cost: bcrypt.MinCost}
	now := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)

	first, err := seed.E2EFixtures(ctx, pool, hasher, "Guvenli!Sifre-42", now)
	require.NoError(t, err)
	second, err := seed.E2EFixtures(ctx, pool, hasher, "Guvenli!Sifre-42", now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, first, second)

	var users, buildings, analyzers int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from users`).Scan(&users))
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from buildings`).Scan(&buildings))
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from analyzers`).Scan(&analyzers))
	require.Equal(t, []int{6, 3, 3}, []int{users, buildings, analyzers})

	a1, err := postgres.NewBuildingRepository(pool).Get(ctx, store.SystemScope(first.CompanyA), first.BuildingA1)
	require.NoError(t, err)
	require.Equal(t, first.Users[seed.E2EBuildingAdminEmail], *a1.ResponsibleUserID)
	ba, err := postgres.NewUserRepository(pool).Get(ctx, store.SystemScope(first.CompanyA), first.Users[seed.E2EBuildingAdminEmail])
	require.NoError(t, err)
	require.Equal(t, model.UserRoleBuildingAdmin, ba.Role)
}
