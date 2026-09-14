//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func reportFixtureRow(companyID, buildingID uuid.UUID, period string) model.Report {
	now := time.Now().UTC()
	return model.Report{
		CompanyID: companyID, BuildingID: buildingID, Type: model.ReportTypeMonthly, Period: period,
		PlantSelection: model.PlantSelectionAll, Payload: json.RawMessage(`{}`),
		Status: model.ReportStatusPending, CreatedAt: now, UpdatedAt: now,
	}
}

func TestReportGetAndListAreScopedToVisibleBuildings(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 300)
	repo := postgres.NewReportRepository(pool)

	visible, err := repo.Upsert(ctx, tenant.AdminScope, reportFixtureRow(tenant.Company.ID, tenant.Buildings[0].ID, "2026-01"))
	require.NoError(t, err)
	invisible, err := repo.Upsert(ctx, tenant.AdminScope, reportFixtureRow(tenant.Company.ID, tenant.Buildings[1].ID, "2026-01"))
	require.NoError(t, err)

	// The narrow Scope (Buildings[0] only) reads its own building's report…
	_, err = repo.Get(ctx, tenant.Scope, visible.ID)
	require.NoError(t, err)
	// …but not Buildings[1]'s, even within its own company.
	_, err = repo.Get(ctx, tenant.Scope, invisible.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.List(ctx, tenant.Scope, store.ReportFilter{})
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, visible.ID, list[0].ID)

	// Cross-tenant is refused entirely.
	other := testfixtures.NewTenant(t, ctx, pool, 301)
	_, err = repo.Get(ctx, other.AdminScope, visible.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestReportUpsertReplacesRatherThanAccumulates pins the (building_id, type,
// period) key: regenerating a period replaces its report.
func TestReportUpsertReplacesRatherThanAccumulates(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 310)
	repo := postgres.NewReportRepository(pool)
	buildingID := tenant.Buildings[0].ID

	first, err := repo.Upsert(ctx, tenant.Scope, reportFixtureRow(tenant.Company.ID, buildingID, "2026-01"))
	require.NoError(t, err)

	row := reportFixtureRow(tenant.Company.ID, buildingID, "2026-01")
	row.Status = model.ReportStatusCompleted
	second, err := repo.Upsert(ctx, tenant.Scope, row)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "the same (building, type, period) key must replace, not duplicate")
	require.Equal(t, model.ReportStatusCompleted, second.Status)

	list, err := repo.List(ctx, tenant.Scope, store.ReportFilter{})
	require.NoError(t, err)
	require.Len(t, list, 1)
}

// TestReportUpsertRejectsBuildingOutsideScope is the write-side isolation
// check: reports.building_id is NOT NULL, so a narrow Scope may only upsert
// into a building it can see.
func TestReportUpsertRejectsBuildingOutsideScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 320)
	repo := postgres.NewReportRepository(pool)

	_, err := repo.Upsert(ctx, tenant.Scope, reportFixtureRow(tenant.Company.ID, tenant.Buildings[1].ID, "2026-01"))
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestReportUpdateStatusIsScoped(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 330)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 331)
	repo := postgres.NewReportRepository(pool)

	rp, err := repo.Upsert(ctx, tenantB.AdminScope, reportFixtureRow(tenantB.Company.ID, tenantB.Buildings[0].ID, "2026-01"))
	require.NoError(t, err)

	errMsg := "boom"
	_, err = repo.UpdateStatus(ctx, tenantA.AdminScope, rp.ID, model.ReportStatusError, &errMsg, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)

	updated, err := repo.UpdateStatus(ctx, tenantB.AdminScope, rp.ID, model.ReportStatusError, &errMsg, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, model.ReportStatusError, updated.Status)
	require.Equal(t, &errMsg, updated.ErrorMessage)
}
