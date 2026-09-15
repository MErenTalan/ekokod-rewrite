//go:build integration

package production_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest/production"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var productionStoreEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func ptrStr(s string) *string { return &s }

func dec(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

// sample builds a device-attributed isolar.ProductionSample.
func sample(psKey string, ts time.Time, kwh string) isolar.ProductionSample {
	return isolar.ProductionSample{
		PSKey:         ptrStr(psKey),
		Ts:            ts,
		ProductionKwh: dec(kwh),
	}
}

// plantSample builds a plant-level (BLOCKER) isolar.ProductionSample: PSKey
// and DeviceSN are both nil.
func plantSample(ts time.Time, kwh string) isolar.ProductionSample {
	return isolar.ProductionSample{Ts: ts, ProductionKwh: dec(kwh)}
}

// TestPlantLevelProductionIsQuarantinedNotFabricated pins the BLOCKER
// ruling: until the product owner answers, a plant-level sample is never
// written and never attributed to a device.
func TestPlantLevelProductionIsQuarantinedNotFabricated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	plants := postgres.NewPlantRepository(pool)
	prod := postgres.NewProductionRepository(pool)
	ops := postgres.NewOpsRepository(pool)

	plantID := tenant.Plants[0].ID
	dev1, err := plants.UpsertDevice(ctx, tenant.AdminScope, model.PlantDevice{PlantID: plantID, DeviceSN: "DEV-1", ProviderKey: ptrStr("FXKEY001")})
	require.NoError(t, err)
	dev2, err := plants.UpsertDevice(ctx, tenant.AdminScope, model.PlantDevice{PlantID: plantID, DeviceSN: "DEV-2", ProviderKey: ptrStr("FXKEY002")})
	require.NoError(t, err)

	samples := []isolar.ProductionSample{
		sample("FXKEY001", productionStoreEpoch, "1.0"),
		sample("FXKEY001", productionStoreEpoch.Add(time.Hour), "1.1"),
		sample("FXKEY002", productionStoreEpoch, "2.0"),
		sample("FXKEY002", productionStoreEpoch.Add(time.Hour), "2.1"),
		plantSample(productionStoreEpoch, "99.0"),
		plantSample(productionStoreEpoch.Add(time.Hour), "99.1"),
		plantSample(productionStoreEpoch.Add(2*time.Hour), "99.2"),
	}

	scope := store.SystemScope(tenant.Company.ID)
	deps := production.Deps{Plants: plants, Production: prod, Ops: ops}
	res, err := production.Store(ctx, scope, deps, plantID, samples)
	require.NoError(t, err)
	require.Equal(t, production.Result{Inserted: 4, Updated: 0, Quarantined: 3, UnknownDevice: 0}, res)

	got, err := prod.Range(ctx, tenant.AdminScope, plantID, store.TimeRange{From: productionStoreEpoch, To: productionStoreEpoch.Add(3 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 4, "plant_production must carry exactly the 4 device-attributed rows, never the 3 plant-level ones")

	perDevice := map[uuid.UUID]int{}
	for _, row := range got {
		require.Contains(t, []uuid.UUID{dev1.ID, dev2.ID}, row.DeviceID, "every stored device_id must be one of the plant's two devices")
		perDevice[row.DeviceID]++
	}
	require.Equal(t, 2, perDevice[dev1.ID])
	require.Equal(t, 2, perDevice[dev2.ID])

	msgs, err := ops.ListMessages(ctx, tenant.Scope, store.MessageFilter{Categories: []string{"plant-production"}})
	require.NoError(t, err)
	require.Len(t, msgs, 1, "exactly one operational message for the quarantine")
	require.Equal(t, "job", msgs[0].Kind)
	require.Equal(t, "warning", msgs[0].Status)

	var meta map[string]any
	require.NoError(t, json.Unmarshal(msgs[0].Metadata, &meta))
	require.Equal(t, "plant_level_production_unstorable", meta["code"])
	require.EqualValues(t, 3, meta["samples"])
}

// TestProductionStoreIsIdempotent: re-running Store with the same samples
// converges (BulkInsert's own upsert), never duplicates rows nor
// re-quarantines/re-reports beyond a second identical warning message.
func TestProductionStoreIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 2)

	plants := postgres.NewPlantRepository(pool)
	prod := postgres.NewProductionRepository(pool)
	ops := postgres.NewOpsRepository(pool)

	plantID := tenant.Plants[0].ID
	_, err := plants.UpsertDevice(ctx, tenant.AdminScope, model.PlantDevice{PlantID: plantID, DeviceSN: "DEV-1", ProviderKey: ptrStr("FXKEY001")})
	require.NoError(t, err)

	samples := []isolar.ProductionSample{
		sample("FXKEY001", productionStoreEpoch, "1.0"),
		sample("FXKEY001", productionStoreEpoch.Add(time.Hour), "1.1"),
	}
	scope := store.SystemScope(tenant.Company.ID)
	deps := production.Deps{Plants: plants, Production: prod, Ops: ops}

	res1, err := production.Store(ctx, scope, deps, plantID, samples)
	require.NoError(t, err)
	require.Equal(t, production.Result{Inserted: 2, Updated: 0}, res1)

	res2, err := production.Store(ctx, scope, deps, plantID, samples)
	require.NoError(t, err)
	require.Equal(t, production.Result{Inserted: 0, Updated: 2}, res2)

	got, err := prod.Range(ctx, tenant.AdminScope, plantID, store.TimeRange{From: productionStoreEpoch, To: productionStoreEpoch.Add(2 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 2, "re-ingesting the same window must converge, never duplicate")
}

// TestProductionUnknownDeviceIsSkippedAndReported: a sample whose PSKey
// matches no known device is skipped (never auto-created — device sync is
// F9's job) and reported via one warning message, code unknown_device.
func TestProductionUnknownDeviceIsSkippedAndReported(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 3)

	plants := postgres.NewPlantRepository(pool)
	prod := postgres.NewProductionRepository(pool)
	ops := postgres.NewOpsRepository(pool)

	plantID := tenant.Plants[0].ID
	_, err := plants.UpsertDevice(ctx, tenant.AdminScope, model.PlantDevice{PlantID: plantID, DeviceSN: "DEV-1", ProviderKey: ptrStr("FXKEY001")})
	require.NoError(t, err)

	samples := []isolar.ProductionSample{
		sample("FXKEY001", productionStoreEpoch, "1.0"),
		sample("FX-UNKNOWN", productionStoreEpoch, "5.0"),
	}
	scope := store.SystemScope(tenant.Company.ID)
	deps := production.Deps{Plants: plants, Production: prod, Ops: ops}

	res, err := production.Store(ctx, scope, deps, plantID, samples)
	require.NoError(t, err)
	require.Equal(t, production.Result{Inserted: 1, Updated: 0, Quarantined: 0, UnknownDevice: 1}, res)

	got, err := prod.Range(ctx, tenant.AdminScope, plantID, store.TimeRange{From: productionStoreEpoch, To: productionStoreEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 1)

	msgs, err := ops.ListMessages(ctx, tenant.Scope, store.MessageFilter{Categories: []string{"plant-production"}})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(msgs[0].Metadata, &meta))
	require.Equal(t, "unknown_device", meta["code"])
	require.EqualValues(t, 1, meta["samples"])
}

// TestProductionStoreRefusesAnotherTenantsPlant: a plantID belonging to
// another company is refused via Devices' own isolation check (join through
// power_plants) — nothing is written, no message is appended.
func TestProductionStoreRefusesAnotherTenantsPlant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 4)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 5)

	plants := postgres.NewPlantRepository(pool)
	prod := postgres.NewProductionRepository(pool)
	ops := postgres.NewOpsRepository(pool)

	foreignPlantID := tenantB.Plants[0].ID
	_, err := plants.UpsertDevice(ctx, tenantB.AdminScope, model.PlantDevice{PlantID: foreignPlantID, DeviceSN: "DEV-B", ProviderKey: ptrStr("FXKEYB01")})
	require.NoError(t, err)

	samples := []isolar.ProductionSample{sample("FXKEYB01", productionStoreEpoch, "1.0")}
	scope := store.SystemScope(tenantA.Company.ID) // tenant A's scope, tenant B's plant
	deps := production.Deps{Plants: plants, Production: prod, Ops: ops}

	res, err := production.Store(ctx, scope, deps, foreignPlantID, samples)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Equal(t, production.Result{}, res)

	got, err := prod.Range(ctx, tenantB.AdminScope, foreignPlantID, store.TimeRange{From: productionStoreEpoch, To: productionStoreEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Empty(t, got, "nothing may be written through another tenant's scope")

	msgs, err := ops.ListMessages(ctx, tenantA.Scope, store.MessageFilter{Categories: []string{"plant-production"}})
	require.NoError(t, err)
	require.Empty(t, msgs)
}
