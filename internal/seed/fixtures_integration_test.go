//go:build integration

package seed_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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

// R194: the screens need real data in the e2e database, and a second seed run
// must not double it.
func TestE2EFixturesHaveReadingsAndBill(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	hasher := auth.Hasher{Pepper: []byte("pepper-0123456789abcdef-0123456789"), Cost: bcrypt.MinCost}
	now := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)

	fx, err := seed.E2EFixtures(ctx, pool, hasher, "Guvenli!Sifre-42", now)
	require.NoError(t, err)
	_, err = seed.E2EData(ctx, pool, fx, now)
	require.NoError(t, err)
	again, err := seed.E2EFixtures(ctx, pool, hasher, "Guvenli!Sifre-42", now)
	require.NoError(t, err)
	require.Equal(t, fx.CompanyA, again.CompanyA)
	_, err = seed.E2EData(ctx, pool, again, now)
	require.NoError(t, err)

	for _, id := range []uuid.UUID{fx.AnalyzerA1, fx.AnalyzerA2, fx.AnalyzerB1} {
		var readings int
		require.NoError(t, pool.QueryRow(ctx, `select count(*) from meter_readings where analyzer_id = $1`, id).Scan(&readings))
		require.Equal(t, 75*24+1, readings, "75 days of hourly readings, no duplicates after a second run")
	}
	var daily int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from consumption_daily where analyzer_id = $1`, fx.AnalyzerA1).Scan(&daily))
	require.Greater(t, daily, 70, "the daily aggregate is materialised for the seeded range")

	var bills int
	var total decimal.Decimal
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*), coalesce(max(total_cost), 0) from bills where building_id = $1 and status <> 'superseded'`,
		fx.BuildingA1).Scan(&bills, &total))
	require.Equal(t, 1, bills, "exactly one bill after two seed runs")
	require.Equal(t, "48250.75", total.String())
}
