//go:build integration

package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
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

	// The second call must have actually CHANGED the stored value, not just
	// reported a row written: an upsert that silently no-ops on conflict
	// would pass the assertions above without ever updating anything.
	var value string
	require.NoError(t, pool.QueryRow(ctx, `select value::text from yekdem_monthly where year = 2026 and month = 1`).Scan(&value))
	require.Equal(t, "20.0000", value)
}

// --- input validation: refused loudly, before any database round trip -----

func TestUpsertHourlyPricesRefusesZeroTs(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := admin.NewMarketDataRepository(pool)

	_, err := repo.UpsertHourlyPrices(ctx, []model.MarketPrice{
		{PTF: decimal.RequireFromString("1.0000")}, // Ts left zero
	})
	require.ErrorIs(t, err, store.ErrConflict)

	var count int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from market_prices_hourly`).Scan(&count))
	require.Zero(t, count, "nothing may be written when the call is refused")
}

func TestUpsertHourlyPricesRefusesDuplicateTsWithinOneCall(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := admin.NewMarketDataRepository(pool)

	prices := []model.MarketPrice{
		{Ts: marketDataEpoch, PTF: decimal.RequireFromString("1.0000")},
		{Ts: marketDataEpoch, PTF: decimal.RequireFromString("2.0000")}, // same Ts, different value
	}
	_, err := repo.UpsertHourlyPrices(ctx, prices)
	require.ErrorIs(t, err, store.ErrConflict)

	var count int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from market_prices_hourly`).Scan(&count))
	require.Zero(t, count, "nothing may be written when the batch itself contains a duplicate ts")
}

func TestUpsertYekdemRefusesMonthOutsideValidRange(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := admin.NewMarketDataRepository(pool)

	for _, month := range []int16{0, 13, -1} {
		_, err := repo.UpsertYekdem(ctx, []model.YekdemMonthly{{Year: 2026, Month: month, Value: decimal.RequireFromString("1.0000")}})
		require.ErrorIsf(t, err, store.ErrConflict, "month %d must be refused", month)
	}

	var count int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from yekdem_monthly`).Scan(&count))
	require.Zero(t, count, "nothing may be written when the call is refused")
}

func TestUpsertYekdemRefusesDuplicateYearMonthWithinOneCall(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := admin.NewMarketDataRepository(pool)

	values := []model.YekdemMonthly{
		{Year: 2026, Month: 1, Value: decimal.RequireFromString("1.0000")},
		{Year: 2026, Month: 1, Value: decimal.RequireFromString("2.0000")}, // same (year, month), different value
	}
	_, err := repo.UpsertYekdem(ctx, values)
	require.ErrorIs(t, err, store.ErrConflict)

	var count int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from yekdem_monthly`).Scan(&count))
	require.Zero(t, count, "nothing may be written when the batch itself contains a duplicate (year, month)")
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
