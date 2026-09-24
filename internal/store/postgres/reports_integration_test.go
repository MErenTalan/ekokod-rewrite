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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 320)
	repo := postgres.NewReportRepository(pool)

	_, err := repo.Upsert(ctx, tenant.Scope, reportFixtureRow(tenant.Company.ID, tenant.Buildings[1].ID, "2026-01"))
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestReportUpsertRejectsCrossTenantBuildingEvenWithAdminScope is Critical
// Finding 1's probe applied to reports: Scope.AllowsBuilding is an in-memory
// grant check that returns true for ANY id under AllBuildings — it cannot
// know which company owns a building — so tenant A's AdminScope must not be
// able to store tenant B's building id as a report's building_id.
func TestReportUpsertRejectsCrossTenantBuildingEvenWithAdminScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 340)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 341)
	repo := postgres.NewReportRepository(pool)

	_, err := repo.Upsert(ctx, tenantA.AdminScope, reportFixtureRow(tenantA.Company.ID, tenantB.Buildings[0].ID, "2026-01"))
	require.ErrorIs(t, err, store.ErrNotFound, "tenant A's AdminScope must not be able to store tenant B's building id")

	// Tenant B's own Upsert for that building and period afterwards succeeds
	// — the CONTROLLER RULING's probe against a silent cross-tenant reservation.
	created, err := repo.Upsert(ctx, tenantB.AdminScope, reportFixtureRow(tenantB.Company.ID, tenantB.Buildings[0].ID, "2026-01"))
	require.NoError(t, err)
	require.Equal(t, tenantB.Buildings[0].ID, created.BuildingID)
}

// TestReportPageLimitsClampNegativeOffset is the folded-minor probe: OFFSET
// must not be negative.
func TestReportPageLimitsClampNegativeOffset(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 342)
	repo := postgres.NewReportRepository(pool)

	_, err := repo.List(ctx, tenant.AdminScope, store.ReportFilter{Page: store.Page{Limit: 10, Offset: -3}})
	require.NoError(t, err, "a negative Offset must be clamped to 0")
}

func TestReportUpdateStatusIsScoped(t *testing.T) {
	t.Parallel()
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

func TestReportListFiltersByYear(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 320)
	repo := postgres.NewReportRepository(pool)
	b := tenant.Buildings[0].ID
	for _, period := range []string{"2025-11", "2026-01", "2026-02"} {
		_, err := repo.Upsert(ctx, tenant.AdminScope, reportFixtureRow(tenant.Company.ID, b, period))
		require.NoError(t, err)
	}
	yearly := reportFixtureRow(tenant.Company.ID, b, "2026")
	yearly.Type = model.ReportTypeYearly
	_, err := repo.Upsert(ctx, tenant.AdminScope, yearly)
	require.NoError(t, err)
	// A '20260' prefix must not match: the year is the whole period or its prefix before '-'.
	odd := reportFixtureRow(tenant.Company.ID, b, "2025")
	odd.Type = model.ReportTypeYearly
	_, err = repo.Upsert(ctx, tenant.AdminScope, odd)
	require.NoError(t, err)

	year := 2026
	got, err := repo.List(ctx, tenant.AdminScope, store.ReportFilter{Year: &year})
	require.NoError(t, err)
	var periods []string
	for _, r := range got {
		periods = append(periods, r.Period)
	}
	require.ElementsMatch(t, []string{"2026-01", "2026-02", "2026"}, periods)
}

func TestReportCountMatchesList(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 330)
	repo := postgres.NewReportRepository(pool)
	for i, period := range []string{"2026-01", "2026-02", "2026-03"} {
		_, err := repo.Upsert(ctx, tenant.AdminScope, reportFixtureRow(tenant.Company.ID, tenant.Buildings[i%2].ID, period))
		require.NoError(t, err)
	}
	for _, f := range []store.ReportFilter{{}, {BuildingID: &tenant.Buildings[0].ID}, {Page: store.Page{Limit: 1}}} {
		n, err := repo.Count(ctx, tenant.AdminScope, f)
		require.NoError(t, err)
		f.Page = store.Page{}
		list, err := repo.List(ctx, tenant.AdminScope, f)
		require.NoError(t, err)
		require.Equal(t, int64(len(list)), n, "count ignores paging and matches the unpaged list")
	}
}

func TestReportCountIsScoped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 340)
	other := testfixtures.NewTenant(t, ctx, pool, 341)
	repo := postgres.NewReportRepository(pool)
	_, err := repo.Upsert(ctx, tenant.AdminScope, reportFixtureRow(tenant.Company.ID, tenant.Buildings[0].ID, "2026-01"))
	require.NoError(t, err)
	_, err = repo.Upsert(ctx, tenant.AdminScope, reportFixtureRow(tenant.Company.ID, tenant.Buildings[1].ID, "2026-01"))
	require.NoError(t, err)
	_, err = repo.Upsert(ctx, other.AdminScope, reportFixtureRow(other.Company.ID, other.Buildings[0].ID, "2026-01"))
	require.NoError(t, err)

	n, err := repo.Count(ctx, tenant.AdminScope, store.ReportFilter{})
	require.NoError(t, err)
	require.Equal(t, int64(2), n, "another company's report is not counted")
	n, err = repo.Count(ctx, tenant.Scope, store.ReportFilter{})
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "an out-of-scope building's report is not counted")
}
