//go:build integration

package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var marketDataEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func TestUpsertHourlyPricesIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := admin.NewMarketDataRepository(pool)

	prices := []model.MarketPrice{
		{Ts: marketDataEpoch, PTF: decimal.RequireFromString("1000.0000")},
		{Ts: marketDataEpoch.Add(time.Hour), PTF: decimal.RequireFromString("1100.0000")},
	}
	n, err := repo.UpsertHourlyPrices(ctx, prices)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)

	// Re-importing the same hours converges rather than duplicating: the
	// unique primary key is `ts`, so a repeat write still reports 2 rows
	// written (an upsert), never 4 stored.
	prices[0].PTF = decimal.RequireFromString("2000.0000")
	n, err = repo.UpsertHourlyPrices(ctx, prices)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)

	var count int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from market_prices_hourly`).Scan(&count))
	require.Equal(t, 2, count)

	var ptf string
	require.NoError(t, pool.QueryRow(ctx, `select ptf::text from market_prices_hourly where ts = $1`, marketDataEpoch).Scan(&ptf))
	require.Equal(t, "2000.0000", ptf)
}

func TestUpsertYekdemIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := admin.NewMarketDataRepository(pool)

	values := []model.YekdemMonthly{{Year: 2026, Month: 1, Value: decimal.RequireFromString("10.0000")}}
	n, err := repo.UpsertYekdem(ctx, values)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	values[0].Value = decimal.RequireFromString("20.0000")
	n, err = repo.UpsertYekdem(ctx, values)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	var count int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from yekdem_monthly`).Scan(&count))
	require.Equal(t, 1, count)
}

func TestUpsertHourlyPricesAndYekdemHandleEmptyInput(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := admin.NewMarketDataRepository(pool)

	n, err := repo.UpsertHourlyPrices(ctx, nil)
	require.NoError(t, err)
	require.Zero(t, n)

	n, err = repo.UpsertYekdem(ctx, nil)
	require.NoError(t, err)
	require.Zero(t, n)
}
