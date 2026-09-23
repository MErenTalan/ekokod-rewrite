//go:build integration

package admin_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestLinkedPlantsSkipsUnlinkedDeletedAndDeadCompanies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	live := testfixtures.NewTenant(t, ctx, pool, 1901)
	gone := testfixtures.NewTenant(t, ctx, pool, 1902)
	_, err := pool.Exec(ctx, `update companies set deleted_at = now() where id = $1`, gone.Company.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `update power_plants set isolar_ps_id = null where id = $1`, live.Plants[1].ID)
	require.NoError(t, err)

	got, err := admin.NewSolarRepository(pool).LinkedPlants(ctx)
	require.NoError(t, err)
	require.Equal(t, []store.CompanyPlant{{CompanyID: live.Company.ID, PlantID: live.Plants[0].ID}}, got)
}
