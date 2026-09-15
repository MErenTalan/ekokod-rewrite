//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var pricesEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func TestPriceHourlyRangeReadsWhatAdminWrote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	priceRepo := postgres.NewPriceRepository(pool)
	adminRepo := admin.NewMarketDataRepository(pool)

	prices := []model.MarketPrice{
		{Ts: pricesEpoch, PTF: decimal.RequireFromString("1500.0000")},
		{Ts: pricesEpoch.Add(time.Hour), PTF: decimal.RequireFromString("1600.0000")},
	}
	n, err := adminRepo.UpsertHourlyPrices(ctx, prices)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)

	got, err := priceRepo.HourlyRange(ctx, tenant.Scope, store.TimeRange{From: pricesEpoch, To: pricesEpoch.Add(2 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.True(t, decimal.RequireFromString("1500.0000").Equal(got[0].PTF))
}

func TestPriceYekdemReadsWhatAdminWrote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	priceRepo := postgres.NewPriceRepository(pool)
	adminRepo := admin.NewMarketDataRepository(pool)

	n, err := adminRepo.UpsertYekdem(ctx, []model.YekdemMonthly{{Year: 2026, Month: 1, Value: decimal.RequireFromString("42.5000")}})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	got, err := priceRepo.Yekdem(ctx, tenant.Scope, 2026, 1)
	require.NoError(t, err)
	require.True(t, decimal.RequireFromString("42.5000").Equal(got.Value))
}

func TestPriceYekdemNotFoundForUnknownPeriod(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	priceRepo := postgres.NewPriceRepository(pool)

	_, err := priceRepo.Yekdem(ctx, tenant.Scope, 1999, 1)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// --- scope validation. market_prices_hourly and yekdem_monthly are
// PLATFORM-WIDE: the Scope narrows nothing, but it is still required and
// still validated (repository.go's PriceRepository doc). ------------------

func TestPriceRepositoryRejectsInvalidScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := postgres.NewPriceRepository(pool)
	invalid := store.Scope{}

	_, err := repo.HourlyRange(ctx, invalid, store.TimeRange{From: pricesEpoch, To: pricesEpoch.Add(time.Hour)})
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.Yekdem(ctx, invalid, 2026, 1)
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestPriceHourlyRangeRejectsInvalidRange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewPriceRepository(pool)

	_, err := repo.HourlyRange(ctx, tenant.Scope, store.TimeRange{})
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

// A narrow Scope and AllBuildings both see the SAME platform rows: the Scope
// genuinely narrows nothing here.
func TestPriceHourlyRangeIsTheSameUnderAnyValidScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	priceRepo := postgres.NewPriceRepository(pool)
	adminRepo := admin.NewMarketDataRepository(pool)

	_, err := adminRepo.UpsertHourlyPrices(ctx, []model.MarketPrice{{Ts: pricesEpoch, PTF: decimal.RequireFromString("1.0000")}})
	require.NoError(t, err)

	fromA, err := priceRepo.HourlyRange(ctx, tenantA.Scope, store.TimeRange{From: pricesEpoch, To: pricesEpoch.Add(time.Hour)})
	require.NoError(t, err)
	fromB, err := priceRepo.HourlyRange(ctx, tenantB.Scope, store.TimeRange{From: pricesEpoch, To: pricesEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Equal(t, len(fromA), len(fromB))
	require.Len(t, fromA, 1)
}
