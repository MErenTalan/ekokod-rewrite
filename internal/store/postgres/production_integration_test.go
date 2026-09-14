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

var productionEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// productionSeedDevice inserts one power_plant_devices row and returns its
// id. testfixtures.NewTenant seeds plants but not their devices, and
// ProductionRepository.BulkInsert needs at least one real device per plant
// to exercise the device-belongs-to-plant isolation rule.
func productionSeedDevice(t *testing.T, ctx context.Context, pool *pgxpool.Pool, plantID uuid.UUID, deviceSN string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(ctx,
		`insert into power_plant_devices (id, plant_id, device_sn) values ($1, $2, $3)`,
		id, plantID, deviceSN)
	require.NoError(t, err)
	return id
}

func productionRow(plantID, deviceID uuid.UUID, ts time.Time, kwh string) model.PlantProduction {
	d := decimal.RequireFromString(kwh)
	return model.PlantProduction{
		PlantID:       plantID,
		Ts:            ts,
		DeviceID:      deviceID,
		ProductionKwh: &d,
		Source:        "isolar",
	}
}

func TestProductionBulkInsertIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewProductionRepository(pool)

	plantID := tenant.Plants[0].ID
	deviceID := productionSeedDevice(t, ctx, pool, plantID, "DEV-1")

	rows := []model.PlantProduction{productionRow(plantID, deviceID, productionEpoch, "10.5000")}
	inserted, updated, err := repo.BulkInsert(ctx, tenant.AdminScope, rows)
	require.NoError(t, err)
	require.Equal(t, 1, inserted)
	require.Equal(t, 0, updated)

	inserted, updated, err = repo.BulkInsert(ctx, tenant.AdminScope, rows)
	require.NoError(t, err)
	require.Equal(t, 0, inserted)
	require.Equal(t, 1, updated)
}

func TestProductionRangeAndLatest(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewProductionRepository(pool)

	plantID := tenant.Plants[0].ID
	deviceID := productionSeedDevice(t, ctx, pool, plantID, "DEV-1")

	rows := []model.PlantProduction{
		productionRow(plantID, deviceID, productionEpoch, "1.0000"),
		productionRow(plantID, deviceID, productionEpoch.Add(time.Hour), "2.0000"),
	}
	_, _, err := repo.BulkInsert(ctx, tenant.AdminScope, rows)
	require.NoError(t, err)

	got, err := repo.Range(ctx, tenant.AdminScope, plantID,
		store.TimeRange{From: productionEpoch, To: productionEpoch.Add(2 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 2)

	latest, err := repo.Latest(ctx, tenant.AdminScope, plantID,
		store.TimeRange{From: productionEpoch, To: productionEpoch.Add(2 * time.Hour)})
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.True(t, decimal.RequireFromString("2.0000").Equal(*latest.ProductionKwh))
}

func TestProductionLatestReturnsNilWhenNoneInRange(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewProductionRepository(pool)

	got, err := repo.Latest(ctx, tenant.AdminScope, tenant.Plants[0].ID,
		store.TimeRange{From: productionEpoch, To: productionEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Nil(t, got)
}

// --- scope / range validation ----------------------------------------------

func TestProductionRepositoryRejectsInvalidScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := postgres.NewProductionRepository(pool)
	invalid := store.Scope{}
	plantID := uuid.New()
	validRange := store.TimeRange{From: productionEpoch, To: productionEpoch.Add(time.Hour)}

	_, _, err := repo.BulkInsert(ctx, invalid, []model.PlantProduction{productionRow(plantID, uuid.New(), productionEpoch, "1")})
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.Range(ctx, invalid, plantID, validRange)
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.Latest(ctx, invalid, plantID, validRange)
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestProductionRepositoryRejectsInvalidRange(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewProductionRepository(pool)
	invalidRange := store.TimeRange{}

	_, err := repo.Range(ctx, tenant.Scope, tenant.Plants[0].ID, invalidRange)
	require.ErrorIs(t, err, store.ErrInvalidRange)

	_, err = repo.Latest(ctx, tenant.Scope, tenant.Plants[0].ID, invalidRange)
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

// --- isolation: plant_production has no company_id, joins through
// power_plants; device_id must belong to that row's plant ------------------

func TestProductionBulkInsertRefusesWholeBatchWhenPlantNotVisible(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewProductionRepository(pool)

	ownPlant := tenantA.Plants[0].ID
	ownDevice := productionSeedDevice(t, ctx, pool, ownPlant, "DEV-A")
	foreignPlant := tenantB.Plants[0].ID
	foreignDevice := productionSeedDevice(t, ctx, pool, foreignPlant, "DEV-B")

	rows := []model.PlantProduction{
		productionRow(ownPlant, ownDevice, productionEpoch, "1.0000"),
		productionRow(foreignPlant, foreignDevice, productionEpoch, "2.0000"),
	}
	_, _, err := repo.BulkInsert(ctx, tenantA.Scope, rows)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.Range(ctx, tenantA.Scope, ownPlant,
		store.TimeRange{From: productionEpoch, To: productionEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Empty(t, got, "the whole batch must be refused: even the row naming the caller's own plant")
}

func TestProductionBulkInsertRefusesWholeBatchWhenDeviceBelongsToAnotherPlant(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewProductionRepository(pool)

	plantA := tenant.Plants[0].ID
	plantB := tenant.Plants[1].ID
	deviceOfB := productionSeedDevice(t, ctx, pool, plantB, "DEV-ON-B")

	// deviceOfB names plantA, but the device itself belongs to plantB.
	rows := []model.PlantProduction{productionRow(plantA, deviceOfB, productionEpoch, "1.0000")}
	_, _, err := repo.BulkInsert(ctx, tenant.AdminScope, rows)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.Range(ctx, tenant.AdminScope, plantA,
		store.TimeRange{From: productionEpoch, To: productionEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestProductionRangeAndLatestPlantNotVisibleReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewProductionRepository(pool)
	validRange := store.TimeRange{From: productionEpoch, To: productionEpoch.Add(time.Hour)}

	_, err := repo.Range(ctx, tenantA.Scope, tenantB.Plants[0].ID, validRange)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.Latest(ctx, tenantA.Scope, tenantB.Plants[0].ID, validRange)
	require.ErrorIs(t, err, store.ErrNotFound)
}
