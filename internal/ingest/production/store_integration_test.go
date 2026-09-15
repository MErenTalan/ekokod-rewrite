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
	fetchFrom := productionStoreEpoch
	fetchTo := productionStoreEpoch.Add(3 * time.Hour)
	res, err := production.Store(ctx, scope, deps, plantID, fetchFrom, fetchTo, samples)
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
	// R43: metadata from/to is the FETCH WINDOW passed to Store, not
	// min/max sample.Ts (the widest plant-level sample is epoch+2h, but the
	// fetch window's own "to" is epoch+3h — asserting exactly that value
	// proves the window came from the parameter, not from the samples).
	require.Equal(t, fetchFrom.UTC().Format(time.RFC3339), meta["from"])
	require.Equal(t, fetchTo.UTC().Format(time.RFC3339), meta["to"])
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
	fetchFrom := productionStoreEpoch
	fetchTo := productionStoreEpoch.Add(2 * time.Hour)

	res1, err := production.Store(ctx, scope, deps, plantID, fetchFrom, fetchTo, samples)
	require.NoError(t, err)
	require.Equal(t, production.Result{Inserted: 2, Updated: 0}, res1)

	res2, err := production.Store(ctx, scope, deps, plantID, fetchFrom, fetchTo, samples)
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

	res, err := production.Store(ctx, scope, deps, plantID, productionStoreEpoch, productionStoreEpoch.Add(time.Hour), samples)
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
// power_plants) — nothing is written, no message is appended. Fix-round-1
// minor: this test now carries its OWN positive control (the identical
// call, made through tenant B's own scope, succeeds and writes both a row
// and a message) so the negative case's "nothing written, no message"
// assertions are not vacuously true of a Store that never writes or
// appends anything for anyone.
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

	deps := production.Deps{Plants: plants, Production: prod, Ops: ops}
	fetchFrom := productionStoreEpoch
	fetchTo := productionStoreEpoch.Add(time.Hour)

	// --- negative case: tenant A's scope, tenant B's plant ---
	negativeSamples := []isolar.ProductionSample{sample("FXKEYB01", productionStoreEpoch, "1.0")}
	scopeA := store.SystemScope(tenantA.Company.ID)

	res, err := production.Store(ctx, scopeA, deps, foreignPlantID, fetchFrom, fetchTo, negativeSamples)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Equal(t, production.Result{}, res)

	got, err := prod.Range(ctx, tenantB.AdminScope, foreignPlantID, store.TimeRange{From: fetchFrom, To: fetchTo})
	require.NoError(t, err)
	require.Empty(t, got, "nothing may be written through another tenant's scope")

	msgsA, err := ops.ListMessages(ctx, tenantA.Scope, store.MessageFilter{Categories: []string{"plant-production"}})
	require.NoError(t, err)
	require.Empty(t, msgsA)

	// --- positive control: the SAME call shape, tenant B's own scope,
	// tenant B's own plant — proves Store genuinely writes rows and
	// appends messages when the scope is correct, so the negative case's
	// "empty"/"nothing written" assertions above are not vacuously true.
	positiveSamples := []isolar.ProductionSample{
		sample("FXKEYB01", productionStoreEpoch, "1.0"),
		plantSample(productionStoreEpoch, "9.0"), // triggers the quarantine message
	}
	scopeB := store.SystemScope(tenantB.Company.ID)
	resB, err := production.Store(ctx, scopeB, deps, foreignPlantID, fetchFrom, fetchTo, positiveSamples)
	require.NoError(t, err)
	require.Equal(t, production.Result{Inserted: 1, Quarantined: 1}, resB)

	gotB, err := prod.Range(ctx, tenantB.AdminScope, foreignPlantID, store.TimeRange{From: fetchFrom, To: fetchTo})
	require.NoError(t, err)
	require.Len(t, gotB, 1, "the positive control must actually write a row")

	msgsB, err := ops.ListMessages(ctx, tenantB.Scope, store.MessageFilter{Categories: []string{"plant-production"}})
	require.NoError(t, err)
	require.Len(t, msgsB, 1, "the positive control must actually append a message")
}

// TestProductionStoreCollapsesIdenticalDuplicates: fix-round-1 finding I3 —
// two samples sharing (ts, device) whose VALUES are identical collapse to
// one stored row, no ConflictingDuplicate and no extra message.
func TestProductionStoreCollapsesIdenticalDuplicates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 6)

	plants := postgres.NewPlantRepository(pool)
	prod := postgres.NewProductionRepository(pool)
	ops := postgres.NewOpsRepository(pool)

	plantID := tenant.Plants[0].ID
	_, err := plants.UpsertDevice(ctx, tenant.AdminScope, model.PlantDevice{PlantID: plantID, DeviceSN: "DEV-1", ProviderKey: ptrStr("FXKEY001")})
	require.NoError(t, err)

	samples := []isolar.ProductionSample{
		sample("FXKEY001", productionStoreEpoch, "1.0"),
		sample("FXKEY001", productionStoreEpoch, "1.0"), // identical duplicate
	}
	scope := store.SystemScope(tenant.Company.ID)
	deps := production.Deps{Plants: plants, Production: prod, Ops: ops}

	res, err := production.Store(ctx, scope, deps, plantID, productionStoreEpoch, productionStoreEpoch.Add(time.Hour), samples)
	require.NoError(t, err)
	require.Equal(t, production.Result{Inserted: 1, ConflictingDuplicate: 0}, res, "identical duplicates collapse to one row, not a conflict")

	got, err := prod.Range(ctx, tenant.AdminScope, plantID, store.TimeRange{From: productionStoreEpoch, To: productionStoreEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 1)

	msgs, err := ops.ListMessages(ctx, tenant.Scope, store.MessageFilter{Categories: []string{"plant-production"}})
	require.NoError(t, err)
	require.Empty(t, msgs, "collapsing identical duplicates must not append a conflicting_duplicate message")
}

// TestProductionStoreRejectsConflictingDuplicates: fix-round-1 finding I3 —
// two samples sharing (ts, device) whose values DIFFER are ALL rejected
// (neither is stored, not even the first) and counted as
// ConflictingDuplicate, with one warning message code
// conflicting_duplicate. MUTATION PROOF (task's fix-round-1 requirement):
// temporarily reverting store.go's dedupe to round-1's "last value wins"
// (rows[i] = row on a repeat key, no conflict tracking) makes this test
// fail — see task-13-report.md's Fix round 1 section for the observed
// failure output.
func TestProductionStoreRejectsConflictingDuplicates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 7)

	plants := postgres.NewPlantRepository(pool)
	prod := postgres.NewProductionRepository(pool)
	ops := postgres.NewOpsRepository(pool)

	plantID := tenant.Plants[0].ID
	_, err := plants.UpsertDevice(ctx, tenant.AdminScope, model.PlantDevice{PlantID: plantID, DeviceSN: "DEV-1", ProviderKey: ptrStr("FXKEY001")})
	require.NoError(t, err)

	samples := []isolar.ProductionSample{
		sample("FXKEY001", productionStoreEpoch, "1.0"),
		sample("FXKEY001", productionStoreEpoch, "2.0"), // same (ts, device), different value: a genuine conflict
		sample("FXKEY001", productionStoreEpoch.Add(time.Hour), "3.0"),
	}
	scope := store.SystemScope(tenant.Company.ID)
	deps := production.Deps{Plants: plants, Production: prod, Ops: ops}

	res, err := production.Store(ctx, scope, deps, plantID, productionStoreEpoch, productionStoreEpoch.Add(2*time.Hour), samples)
	require.NoError(t, err)
	require.Equal(t, production.Result{Inserted: 1, ConflictingDuplicate: 2}, res,
		"both conflicting rows are rejected (counted 2), only the unrelated third sample is stored")

	got, err := prod.Range(ctx, tenant.AdminScope, plantID, store.TimeRange{From: productionStoreEpoch, To: productionStoreEpoch.Add(2 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 1, "neither conflicting row is stored, not even the first")
	require.True(t, got[0].Ts.Equal(productionStoreEpoch.Add(time.Hour)), "only the non-conflicting sample survives")

	msgs, err := ops.ListMessages(ctx, tenant.Scope, store.MessageFilter{Categories: []string{"plant-production"}})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(msgs[0].Metadata, &meta))
	require.Equal(t, "conflicting_duplicate", meta["code"])
	require.EqualValues(t, 2, meta["samples"])
}
