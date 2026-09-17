//go:build integration

package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestUpsertBillingParametersIsIdempotentOnEffectiveFrom(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1420)
	repo := admin.NewCatalogueRepository(pool)
	seed, err := postgres.NewBillingParameterRepository(pool).Effective(ctx, tenant.Scope, time.Now())
	require.NoError(t, err)

	p := seed
	p.EffectiveFrom = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = repo.UpsertBillingParameters(ctx, p)
	require.NoError(t, err)
	p.DemandOverrunMultiplier = decimal.RequireFromString("2.5")
	p.TieringSupplyCompanies = []model.SupplyCompany{model.SupplyCompanyIncumbent, model.SupplyCompanyPrivate}
	saved, err := repo.UpsertBillingParameters(ctx, p)
	require.NoError(t, err)
	require.Equal(t, "2.5", saved.DemandOverrunMultiplier.String())
	require.Equal(t, p.TieringSupplyCompanies, saved.TieringSupplyCompanies)

	var n int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from billing_parameters`).Scan(&n))
	require.Equal(t, 2, n, "seed + one converged row")
}

func TestBillableBuildingsExcludesDeletedAndAnalyzerless(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	live := testfixtures.NewTenant(t, ctx, pool, 1421)
	gone := testfixtures.NewTenant(t, ctx, pool, 1422)

	_, err := pool.Exec(ctx, `update buildings set bill_cutoff_day = 15 where id = $1`, live.Buildings[0].ID)
	require.NoError(t, err)
	// Buildings[1] keeps its analyzers but is deleted; Buildings[2+] lose theirs.
	_, err = pool.Exec(ctx, `update buildings set deleted_at = now() where id = $1`, live.Buildings[1].ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `update analyzers set deleted_at = now() where company_id = $1 and building_id <> $2`, live.Company.ID, live.Buildings[0].ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `update companies set deleted_at = now() where id = $1`, gone.Company.ID)
	require.NoError(t, err)

	got, err := admin.NewBillingRepository(pool).BillableBuildings(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, live.Company.ID, got[0].CompanyID)
	require.Equal(t, live.Buildings[0].ID, got[0].BuildingID)
	require.Equal(t, 15, got[0].CutoffDay)
	require.NotEqual(t, uuid.Nil, got[0].BuildingID)
}
