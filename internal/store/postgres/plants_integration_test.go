//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestPlantRepositoryNeverReturnsAnotherCompanysRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewPlantRepository(pool)

	_, err := repo.Get(ctx, mine.AdminScope, theirs.Plants[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.List(ctx, mine.AdminScope, store.PlantFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, got, "this assertion is vacuous if the list came back empty")
	for _, p := range got {
		require.Equal(t, mine.Company.ID, p.CompanyID)
	}
}

func TestPlantRepositoryRejectsAnInvalidScope(t *testing.T) {
	t.Parallel()
	repo := postgres.NewPlantRepository(testfixtures.NewIsolatedDB(t))
	_, err := repo.List(context.Background(), store.Scope{}, store.PlantFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestPlantRepositoryCreateGetUpdateSoftDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewPlantRepository(pool)

	now := time.Now().UTC()
	created, err := repo.Create(ctx, tenant.AdminScope, model.PowerPlant{
		CompanyID: tenant.Company.ID, Name: "New Plant", PlantKind: "rooftop",
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenant.AdminScope, created.ID)
	require.NoError(t, err)
	require.Equal(t, "New Plant", got.Name)

	got.Name = "Renamed Plant"
	got.UpdatedAt = time.Now().UTC()
	updated, err := repo.Update(ctx, tenant.AdminScope, got)
	require.NoError(t, err)
	require.Equal(t, "Renamed Plant", updated.Name)

	require.NoError(t, repo.SoftDelete(ctx, tenant.AdminScope, created.ID, time.Now().UTC()))

	_, err = repo.Get(ctx, tenant.AdminScope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.List(ctx, tenant.AdminScope, store.PlantFilter{})
	require.NoError(t, err)
	for _, p := range list {
		require.NotEqual(t, created.ID, p.ID, "a soft-deleted plant must disappear from List too")
	}
}

// TestPlantRepositoryUpdateRefusesAForeignCompany is the fix-round-1 proof
// for Important 3: Update must refuse a model value whose CompanyID names
// another company, exactly like Create does, and write nothing.
//
// The row named by p.ID belongs to mine's OWN company throughout —
// PlantUpdate's WHERE clause already scopes by s.CompanyID regardless of
// this guard, so a victim row from ANOTHER tenant would make this test pass
// whether or not the guard exists. Only a model value that lies about its
// OWN row's CompanyID exercises the guard.
func TestPlantRepositoryUpdateRefusesAForeignCompany(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewPlantRepository(pool)

	myPlant := mine.Plants[0]
	tampered := myPlant
	tampered.CompanyID = theirs.Company.ID
	tampered.Name = "Renamed By A Lying CompanyID"
	tampered.UpdatedAt = time.Now().UTC()

	_, err := repo.Update(ctx, mine.AdminScope, tampered)
	require.ErrorIs(t, err, store.ErrNotFound)

	still, err := repo.Get(ctx, mine.AdminScope, myPlant.ID)
	require.NoError(t, err)
	require.Equal(t, myPlant.Name, still.Name, "nothing may be written by a refused Update")
}

// TestPlantRepositoryDevicesExcludeASoftDeletedParent pins wave-f-context
// rule 4 for PlantDevicesList's LEFT JOIN shape: a child of a soft-deleted
// parent must not be readable through the parent-scoped query either.
func TestPlantRepositoryDevicesExcludeASoftDeletedParent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewPlantRepository(pool)

	_, err := repo.UpsertDevice(ctx, tenant.AdminScope, model.PlantDevice{
		PlantID: tenant.Plants[0].ID, DeviceSN: "SOFT-DELETE-PARENT",
	})
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, tenant.AdminScope, tenant.Plants[0].ID, time.Now().UTC()))

	_, err = repo.Devices(ctx, tenant.AdminScope, tenant.Plants[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound,
		"a soft-deleted plant's devices must not stay reachable through the join")
}

// TestPlantRepositoryUpsertDeviceConvergesUnderConcurrency is the Important-1
// proof: N goroutines racing to create the SAME new (plant_id, device_sn)
// must all succeed and converge on exactly one row, now that UpsertDevice is
// one atomic `insert … on conflict … do update` instead of the old
// UPDATE-then-INSERT-if-absent shape. See the task report for the failing
// run against that old shape, captured before this fix.
func TestPlantRepositoryUpsertDeviceConvergesUnderConcurrency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewPlantRepository(pool)

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := repo.UpsertDevice(ctx, tenant.AdminScope, model.PlantDevice{
				PlantID: tenant.Plants[0].ID, DeviceSN: "CONCURRENT-NEW-SN",
			})
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "upsert goroutine %d must converge, not conflict", i)
	}

	got, err := repo.Devices(ctx, tenant.AdminScope, tenant.Plants[0].ID)
	require.NoError(t, err)
	require.Len(t, got, 1, "N concurrent upserts of the same new (plant_id, device_sn) must converge to exactly one row")
}

// TestPlantRepositoryMonthlyTargetsIsolateThroughTheParentPlant is the
// mandatory isolation test for "PlantRepository.MonthlyTargets,
// ReplaceMonthlyTargets — power_plant_monthly_targets -> power_plants".
func TestPlantRepositoryMonthlyTargetsIsolateThroughTheParentPlant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewPlantRepository(pool)

	_, err := repo.MonthlyTargets(ctx, mine.AdminScope, theirs.Plants[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.ReplaceMonthlyTargets(ctx, mine.AdminScope, theirs.Plants[0].ID,
		[]model.PlantMonthlyTarget{{Month: 1, TargetKwh: decimal.RequireFromString("100.000")}})
	require.ErrorIs(t, err, store.ErrNotFound)

	// Nothing was written to theirs's plant.
	theirTargets, err := repo.MonthlyTargets(ctx, theirs.AdminScope, theirs.Plants[0].ID)
	require.NoError(t, err)
	require.Empty(t, theirTargets)

	empty, err := repo.MonthlyTargets(ctx, mine.AdminScope, mine.Plants[0].ID)
	require.NoError(t, err)
	require.Empty(t, empty)

	targets, err := repo.ReplaceMonthlyTargets(ctx, mine.AdminScope, mine.Plants[0].ID, []model.PlantMonthlyTarget{
		{Month: 1, TargetKwh: decimal.RequireFromString("1000.000")},
		{Month: 2, TargetKwh: decimal.RequireFromString("1100.000")},
	})
	require.NoError(t, err)
	require.Len(t, targets, 2)

	got, err := repo.MonthlyTargets(ctx, mine.AdminScope, mine.Plants[0].ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
}

// TestPlantRepositoryDevicesIsolateThroughTheParentPlantAndUpsertConverges
// is the mandatory isolation test for "PlantRepository.Devices,
// UpsertDevice (d.PlantID; plant_id never updated) — power_plant_devices ->
// power_plants".
func TestPlantRepositoryDevicesIsolateThroughTheParentPlantAndUpsertConverges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewPlantRepository(pool)

	_, err := repo.Devices(ctx, mine.AdminScope, theirs.Plants[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.UpsertDevice(ctx, mine.AdminScope, model.PlantDevice{PlantID: theirs.Plants[0].ID, DeviceSN: "SN-1"})
	require.ErrorIs(t, err, store.ErrNotFound)

	theirDevices, err := repo.Devices(ctx, theirs.AdminScope, theirs.Plants[0].ID)
	require.NoError(t, err)
	require.Empty(t, theirDevices, "the refused upsert must not have written a device on theirs's plant")

	dev, err := repo.UpsertDevice(ctx, mine.AdminScope, model.PlantDevice{PlantID: mine.Plants[0].ID, DeviceSN: "SN-1"})
	require.NoError(t, err)
	require.Equal(t, mine.Plants[0].ID, dev.PlantID)

	// Re-upsert on the same (plant_id, device_sn) converges rather than
	// duplicating, and plant_id is not something the update branch can move.
	brand := "Huawei"
	updated, err := repo.UpsertDevice(ctx, mine.AdminScope, model.PlantDevice{
		ID: dev.ID, PlantID: mine.Plants[0].ID, DeviceSN: "SN-1", Brand: &brand,
	})
	require.NoError(t, err)
	require.Equal(t, dev.ID, updated.ID)
	require.Equal(t, mine.Plants[0].ID, updated.PlantID)
	require.Equal(t, brand, *updated.Brand)

	got, err := repo.Devices(ctx, mine.AdminScope, mine.Plants[0].ID)
	require.NoError(t, err)
	require.Len(t, got, 1, "the second upsert must converge, not duplicate")
}

// TestPlantRepositoryAlarmRecipientsIsolateThroughTheParentPlant is the
// mandatory isolation test for "PlantRepository.AlarmRecipients,
// ReplaceAlarmRecipients — power_plant_alarm_recipients -> power_plants".
func TestPlantRepositoryAlarmRecipientsIsolateThroughTheParentPlant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewPlantRepository(pool)

	_, err := repo.AlarmRecipients(ctx, mine.AdminScope, theirs.Plants[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.ReplaceAlarmRecipients(ctx, mine.AdminScope, theirs.Plants[0].ID, []string{"ops@example.invalid"})
	require.ErrorIs(t, err, store.ErrNotFound)

	theirRecipients, err := repo.AlarmRecipients(ctx, theirs.AdminScope, theirs.Plants[0].ID)
	require.NoError(t, err)
	require.Empty(t, theirRecipients)

	replaced, err := repo.ReplaceAlarmRecipients(ctx, mine.AdminScope, mine.Plants[0].ID, []string{"ops@example.invalid"})
	require.NoError(t, err)
	require.Len(t, replaced, 1)

	got, err := repo.AlarmRecipients(ctx, mine.AdminScope, mine.Plants[0].ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "ops@example.invalid", got[0].Email)
}
