//go:build integration

package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestSectorFiguresCrossTenantAggregates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	ist, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	a := testfixtures.NewTenant(t, ctx, pool, 1)
	b := testfixtures.NewTenant(t, ctx, pool, 2)
	c := testfixtures.NewTenant(t, ctx, pool, 3)
	exec := func(sql string, args ...any) {
		_, err := pool.Exec(ctx, sql, args...)
		require.NoError(t, err)
	}
	exec(`update buildings set sector = 'Üretim' where id = $1`, a.Buildings[0].ID)
	exec(`update buildings set sector = '  üretim ' where id = $1`, b.Buildings[0].ID)
	exec(`update buildings set sector = 'Perakende' where id = $1`, a.Buildings[1].ID)
	exec(`update buildings set sector = 'Üretim' where id = $1`, c.Buildings[0].ID)
	exec(`update buildings set sector = null where id = $1`, b.Buildings[1].ID)
	require.NoError(t, postgres.NewCompanyRepository(pool).SoftDelete(ctx, c.AdminScope, c.Company.ID, time.Now()))

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, ist)
	to := time.Date(2026, 9, 17, 0, 0, 0, 0, ist)
	testfixtures.InsertReadings(t, ctx, pool, a.Company.ID, testfixtures.HourlyReadings(a.Analyzers[0].ID, from, to, testfixtures.Constant("10")))
	testfixtures.InsertReadings(t, ctx, pool, b.Company.ID, testfixtures.HourlyReadings(b.Analyzers[0].ID, from, to, testfixtures.Constant("5")))
	testfixtures.InsertReadings(t, ctx, pool, c.Company.ID, testfixtures.HourlyReadings(c.Analyzers[0].ID, from, to, testfixtures.Constant("99")))

	daily := store.TimeRange{From: time.Date(2026, 8, 18, 0, 0, 0, 0, ist), To: to}
	month := store.TimeRange{From: from, To: time.Date(2026, 9, 1, 0, 0, 0, 0, ist)}
	figures, err := admin.NewSectorRepository(pool).SectorFigures(ctx, "ÜRETİM", daily, month)
	require.NoError(t, err)
	byID := map[string]store.SectorFigures{}
	for _, f := range figures {
		byID[f.BuildingID.String()] = f
	}
	require.Len(t, byID, 2, "deleted companies and other sectors are excluded: %v", byID)

	fa := byID[a.Buildings[0].ID.String()]
	require.Equal(t, "7190", fa.DailySum.String(), "30 days of hourly readings: 719 steps × 10")
	require.EqualValues(t, 30, fa.DaysWithData)
	require.Equal(t, "7430", fa.MonthlySum.String(), "August: 743 steps × 10")
	fb := byID[b.Buildings[0].ID.String()]
	require.Equal(t, "3595", fb.DailySum.String())
	require.Equal(t, "3715", fb.MonthlySum.String())

	_, err = admin.NewSectorRepository(pool).SectorFigures(ctx, "Üretim", store.TimeRange{}, month)
	require.ErrorIs(t, err, store.ErrInvalidRange)
}
