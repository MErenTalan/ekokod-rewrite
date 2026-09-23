//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// seedTotal inserts one plant_production_totals row; since 00018 the plant
// aggregates read that table only (R276).
func seedTotal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, plantID uuid.UUID, at time.Time, kwh string) {
	t.Helper()
	_, err := pool.Exec(ctx, `insert into plant_production_totals (plant_id, ts, production_kwh, basis)
		values ($1, $2, $3::numeric, 'plant_meter')`, plantID, at, kwh)
	require.NoError(t, err)
}

var solarIstanbul = func() *time.Location { l, _ := time.LoadLocation("Europe/Istanbul"); return l }()

func solarDay(d int) time.Time { return time.Date(2026, time.March, d, 0, 0, 0, 0, solarIstanbul) }

func totalRow(plant model.PowerPlant, ts time.Time, kwh string) model.PlantProductionTotal {
	v := decimal.RequireFromString(kwh)
	return model.PlantProductionTotal{PlantID: plant.ID, Ts: ts, ProductionKwh: &v, Basis: model.BasisPlantMeter}
}

func sumTotals(rows []model.PlantProductionTotal) decimal.Decimal {
	s := decimal.Zero
	for _, r := range rows {
		s = s.Add(*r.ProductionKwh)
	}
	return s
}

func TestReplaceDaysReplacesOnlyThoseDays(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewProductionTotalsRepository(pool)
	plant := tenant.Plants[0]
	d, d1 := solarDay(10), solarDay(11)

	_, err := repo.ReplaceDays(ctx, tenant.AdminScope, plant.ID, []time.Time{d, d1}, []model.PlantProductionTotal{
		totalRow(plant, d.Add(9*time.Hour), "1"), totalRow(plant, d.Add(10*time.Hour), "2"), totalRow(plant, d.Add(11*time.Hour), "3"),
		totalRow(plant, d1.Add(9*time.Hour), "4"), totalRow(plant, d1.Add(10*time.Hour), "5"),
	})
	require.NoError(t, err)

	n, err := repo.ReplaceDays(ctx, tenant.AdminScope, plant.ID, []time.Time{d}, []model.PlantProductionTotal{
		totalRow(plant, d.Add(12*time.Hour), "7"),
	})
	require.NoError(t, err)
	require.Equal(t, 1, n)

	day, err := repo.Range(ctx, tenant.AdminScope, plant.ID, store.TimeRange{From: d, To: d1})
	require.NoError(t, err)
	require.Len(t, day, 1)
	require.True(t, decimal.RequireFromString("7").Equal(*day[0].ProductionKwh))
	next, err := repo.Range(ctx, tenant.AdminScope, plant.ID, store.TimeRange{From: d1, To: solarDay(12)})
	require.NoError(t, err)
	require.True(t, decimal.RequireFromString("9").Equal(sumTotals(next)))

	first, err := repo.FirstDay(ctx, tenant.AdminScope, plant.ID)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.True(t, first.Equal(d), "first day %s", first)
}

func TestReplaceDaysRefusesARowOutsideTheDays(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewProductionTotalsRepository(pool)
	plant := tenant.Plants[0]

	_, err := repo.ReplaceDays(ctx, tenant.AdminScope, plant.ID, []time.Time{solarDay(10)},
		[]model.PlantProductionTotal{totalRow(plant, solarDay(11).Add(time.Hour), "1")})
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

func TestReplaceDeviceDaysReplacesOnlyThoseDays(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewProductionRepository(pool)
	plantID := tenant.Plants[0].ID
	dev := productionSeedDevice(t, ctx, pool, plantID, "DEV-R")
	d, d1 := solarDay(10), solarDay(11)

	_, err := repo.ReplaceDeviceDays(ctx, tenant.AdminScope, plantID, []time.Time{d, d1}, []model.PlantProduction{
		productionRow(plantID, dev, d.Add(9*time.Hour), "1"), productionRow(plantID, dev, d1.Add(9*time.Hour), "4"),
	})
	require.NoError(t, err)
	_, err = repo.ReplaceDeviceDays(ctx, tenant.AdminScope, plantID, []time.Time{d}, []model.PlantProduction{
		productionRow(plantID, dev, d.Add(10*time.Hour), "2"),
	})
	require.NoError(t, err)

	rows, err := repo.Range(ctx, tenant.AdminScope, plantID, store.TimeRange{From: d, To: solarDay(12)})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.True(t, rows[0].Ts.Equal(d.Add(10*time.Hour)))
	require.True(t, rows[1].Ts.Equal(d1.Add(9*time.Hour)))
}

// R276: the plant aggregates read the totals table only, so device rows never double count.
func TestProductionDailyAggregatesTotalsOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	plant := tenant.Plants[0]
	dev := productionSeedDevice(t, ctx, pool, plant.ID, "DEV-A")
	d := solarDay(10)

	_, err := postgres.NewProductionRepository(pool).ReplaceDeviceDays(ctx, tenant.AdminScope, plant.ID, []time.Time{d},
		[]model.PlantProduction{productionRow(plant.ID, dev, d.Add(9*time.Hour), "5")})
	require.NoError(t, err)
	_, err = postgres.NewProductionTotalsRepository(pool).ReplaceDays(ctx, tenant.AdminScope, plant.ID, []time.Time{d},
		[]model.PlantProductionTotal{totalRow(plant, d.Add(9*time.Hour), "7")})
	require.NoError(t, err)
	analyticsRefresh(t, ctx, pool, "plant_production_daily")

	buckets, err := postgres.NewAnalyticsRepository(pool).ProductionDaily(ctx, tenant.AdminScope, []uuid.UUID{plant.ID},
		store.TimeRange{From: d, To: solarDay(11)})
	require.NoError(t, err)
	require.Len(t, buckets, 1)
	require.True(t, decimal.RequireFromString("7").Equal(*buckets[0].ProductionKwh), "got %s", buckets[0].ProductionKwh)
}

func TestClaimReturnsOnlyNewRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewFaultRepository(pool)
	plantID := tenant.Plants[0].ID

	got, err := repo.Claim(ctx, tenant.AdminScope, plantID, []string{"a", "b"})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"a", "b"}, got)
	got, err = repo.Claim(ctx, tenant.AdminScope, plantID, []string{"b", "c"})
	require.NoError(t, err)
	require.Equal(t, []string{"c"}, got)

	require.NoError(t, repo.Release(ctx, tenant.AdminScope, plantID, []string{"c"}))
	got, err = repo.Claim(ctx, tenant.AdminScope, plantID, []string{"c"})
	require.NoError(t, err)
	require.Equal(t, []string{"c"}, got)
}

func TestFaultUpsertListUnforwardedAndPrune(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewFaultRepository(pool)
	plantID := tenant.Plants[0].ID
	now := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.UTC)
	fault := func(ref string, at time.Time) model.PlantFault {
		return model.PlantFault{PlantID: plantID, Ref: ref, Code: "10", Name: "电网掉电", OccurredAt: at, FirstSeenAt: at}
	}

	n, err := repo.Upsert(ctx, tenant.AdminScope, []model.PlantFault{fault("old", now.AddDate(0, 0, -200)), fault("new", now.Add(-time.Hour))})
	require.NoError(t, err)
	require.Equal(t, 2, n)
	_, err = repo.Upsert(ctx, tenant.AdminScope, []model.PlantFault{fault("new", now.Add(-time.Hour))})
	require.NoError(t, err)

	list, total, err := repo.List(ctx, tenant.AdminScope, plantID, store.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Equal(t, "new", list[0].Ref, "newest first")

	_, err = repo.Claim(ctx, tenant.AdminScope, plantID, []string{"new"})
	require.NoError(t, err)
	open, err := repo.Unforwarded(ctx, tenant.AdminScope, plantID, now.AddDate(0, 0, -365))
	require.NoError(t, err)
	require.Len(t, open, 1)
	require.Equal(t, "old", open[0].Ref)

	pruned, err := repo.Prune(ctx, tenant.AdminScope, now.AddDate(0, 0, -180))
	require.NoError(t, err)
	require.Equal(t, 1, pruned)
	_, total, err = repo.List(ctx, tenant.AdminScope, plantID, store.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
}

func TestPlantSnapshotLinkAndSyncState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewPlantRepository(pool)
	plant := tenant.Plants[0]
	at := time.Date(2026, time.March, 20, 9, 0, 0, 0, time.UTC)

	dev, err := repo.UpsertDevice(ctx, tenant.AdminScope, model.PlantDevice{ID: uuid.New(), PlantID: plant.ID, DeviceSN: "SN-1"})
	require.NoError(t, err)
	p := decimal.RequireFromString("3.2")
	status := int32(4)
	dev.SnapshotAt, dev.ActivePowerKw, dev.FaultStatus = &at, &p, &status
	require.NoError(t, repo.UpdateDeviceSnapshot(ctx, tenant.AdminScope, dev))
	devices, err := repo.Devices(ctx, tenant.AdminScope, plant.ID)
	require.NoError(t, err)
	require.True(t, p.Equal(*devices[0].ActivePowerKw))
	require.Equal(t, int32(4), *devices[0].FaultStatus)

	before, err := repo.ListLinked(ctx, tenant.AdminScope)
	require.NoError(t, err)
	code := "isolar_auth"
	require.NoError(t, repo.SetSyncState(ctx, tenant.AdminScope, plant.ID, at, &code))
	psID, name := "ps-9", "Isolar Plant"
	plant.IsolarPsID, plant.IsolarPsName, plant.IsolarLinkedAt = &psID, &name, &at
	linked, err := repo.SetIsolarLink(ctx, tenant.AdminScope, plant)
	require.NoError(t, err)
	require.Equal(t, "isolar_auth", *linked.IsolarLastSyncError)
	require.Equal(t, psID, *linked.IsolarPsID)

	list, err := repo.ListLinked(ctx, tenant.AdminScope)
	require.NoError(t, err)
	require.Len(t, list, len(before), "fixture plants are all linked already")

	plant.IsolarPsID, plant.IsolarPsName, plant.IsolarLinkedAt = nil, nil, nil
	_, err = repo.SetIsolarLink(ctx, tenant.AdminScope, plant)
	require.NoError(t, err)
	list, err = repo.ListLinked(ctx, tenant.AdminScope)
	require.NoError(t, err)
	require.Len(t, list, len(before)-1)
}
