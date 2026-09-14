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
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestAnalyzerRepositoryNeverReturnsAnotherCompanysRows(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewAnalyzerRepository(pool)

	_, err := repo.Get(ctx, mine.AdminScope, theirs.Analyzers[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.List(ctx, mine.AdminScope, store.AnalyzerFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, got, "this assertion is vacuous if the list came back empty")
	for _, a := range got {
		require.Equal(t, mine.Company.ID, a.CompanyID)
	}
}

func TestAnalyzerRepositoryHonoursANarrowScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewAnalyzerRepository(pool)

	got, err := repo.List(ctx, tenant.Scope, store.AnalyzerFilter{})
	require.NoError(t, err)
	require.Len(t, got, 2, "a scope naming one building must not return the other building's analyzers")
	for _, a := range got {
		require.NotNil(t, a.BuildingID)
		require.Equal(t, tenant.Buildings[0].ID, *a.BuildingID)
	}

	_, err = repo.Get(ctx, tenant.Scope, tenant.Analyzers[2].ID) // under Buildings[1]
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestAnalyzerRepositoryRejectsAnInvalidScope(t *testing.T) {
	repo := postgres.NewAnalyzerRepository(testfixtures.NewIsolatedDB(t))
	_, err := repo.List(context.Background(), store.Scope{}, store.AnalyzerFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestAnalyzerRepositoryCreateGetUpdateSoftDelete(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewAnalyzerRepository(pool)

	buildingID := tenant.Buildings[0].ID
	now := time.Now().UTC()
	created, err := repo.Create(ctx, tenant.AdminScope, model.Analyzer{
		CompanyID: tenant.Company.ID, BuildingID: &buildingID,
		Provider: model.IntegrationProviderARIL, ProviderSubtype: "Baskent",
		InstallationNumber: "CRUD-TEST-1", MeterMultiplier: decimal.RequireFromString("1"),
		IsActive: true, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenant.AdminScope, created.ID)
	require.NoError(t, err)
	require.Equal(t, "CRUD-TEST-1", got.InstallationNumber)

	customer := "Renamed Customer"
	got.CustomerName = &customer
	got.UpdatedAt = time.Now().UTC()
	updated, err := repo.Update(ctx, tenant.AdminScope, got)
	require.NoError(t, err)
	require.Equal(t, customer, *updated.CustomerName)

	require.NoError(t, repo.SoftDelete(ctx, tenant.AdminScope, created.ID, time.Now().UTC()))

	_, err = repo.Get(ctx, tenant.AdminScope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.List(ctx, tenant.AdminScope, store.AnalyzerFilter{})
	require.NoError(t, err)
	for _, a := range list {
		require.NotEqual(t, created.ID, a.ID, "a soft-deleted analyzer must disappear from List too")
	}
}

func TestAnalyzerRepositoryCreateRefusesAForeignBuilding(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewAnalyzerRepository(pool)

	foreignBuilding := theirs.Buildings[0].ID
	now := time.Now().UTC()
	_, err := repo.Create(ctx, mine.AdminScope, model.Analyzer{
		CompanyID: mine.Company.ID, BuildingID: &foreignBuilding,
		Provider: model.IntegrationProviderPM5340, ProviderSubtype: "Baskent",
		InstallationNumber: "BAD-BUILDING", MeterMultiplier: decimal.RequireFromString("1"),
		IsActive: true, CreatedAt: now, UpdatedAt: now,
	})
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestAnalyzerRepositoryUnassignedIsVisibleOnlyUnderAllBuildings pins the
// NULL building_id ruling: an analyzer with no building is reachable ONLY
// by a Scope with AllBuildings.
func TestAnalyzerRepositoryUnassignedIsVisibleOnlyUnderAllBuildings(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewAnalyzerRepository(pool)

	now := time.Now().UTC()
	unassigned, err := repo.Create(ctx, tenant.AdminScope, model.Analyzer{
		CompanyID: tenant.Company.ID, Provider: model.IntegrationProviderOSOS,
		ProviderSubtype: "Baskent", InstallationNumber: "UNASSIGNED-1",
		MeterMultiplier: decimal.RequireFromString("1"), IsActive: true,
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	require.Nil(t, unassigned.BuildingID)

	_, err = repo.Get(ctx, tenant.Scope, unassigned.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	narrow, err := repo.List(ctx, tenant.Scope, store.AnalyzerFilter{Unassigned: true})
	require.NoError(t, err)
	require.Empty(t, narrow, "a narrow scope must never see an unassigned analyzer")

	wide, err := repo.List(ctx, tenant.AdminScope, store.AnalyzerFilter{Unassigned: true})
	require.NoError(t, err)
	require.Len(t, wide, 1)
	require.Equal(t, unassigned.ID, wide[0].ID)

	got, err := repo.Get(ctx, tenant.AdminScope, unassigned.ID)
	require.NoError(t, err)
	require.Equal(t, unassigned.ID, got.ID)
}

func TestAnalyzerRepositoryTouchLastReadingNeverRetreats(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewAnalyzerRepository(pool)

	id := tenant.Analyzers[0].ID
	later := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	earlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	require.NoError(t, repo.TouchLastReading(ctx, tenant.AdminScope, id, later))
	got, err := repo.Get(ctx, tenant.AdminScope, id)
	require.NoError(t, err)
	require.NotNil(t, got.LastReadingAt)
	require.True(t, got.LastReadingAt.Equal(later))

	require.NoError(t, repo.TouchLastReading(ctx, tenant.AdminScope, id, earlier))
	got, err = repo.Get(ctx, tenant.AdminScope, id)
	require.NoError(t, err)
	require.True(t, got.LastReadingAt.Equal(later), "an out-of-order backfill must not retreat last_reading_at")
}

func TestAnalyzerRepositoryGetByInstallationIsolatesByCompany(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewAnalyzerRepository(pool)

	theirAnalyzer := theirs.Analyzers[0]
	_, err := repo.GetByInstallation(ctx, mine.AdminScope, theirAnalyzer.Provider,
		theirAnalyzer.ProviderSubtype, theirAnalyzer.InstallationNumber)
	require.ErrorIs(t, err, store.ErrNotFound)

	mineAnalyzer := mine.Analyzers[0]
	got, err := repo.GetByInstallation(ctx, mine.AdminScope, mineAnalyzer.Provider,
		mineAnalyzer.ProviderSubtype, mineAnalyzer.InstallationNumber)
	require.NoError(t, err)
	require.Equal(t, mineAnalyzer.ID, got.ID)
}
