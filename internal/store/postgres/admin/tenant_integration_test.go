//go:build integration

package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestAdminListCompaniesSeesEveryTenant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	a := testfixtures.NewTenant(t, ctx, pool, 1)
	b := testfixtures.NewTenant(t, ctx, pool, 2)
	c := testfixtures.NewTenant(t, ctx, pool, 3)
	require.NoError(t, postgres.NewCompanyRepository(pool).SoftDelete(ctx, c.AdminScope, c.Company.ID, time.Now()))
	repo := admin.NewTenantRepository(pool)

	got, err := repo.ListCompanies(ctx, store.CompanyFilter{})
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, co := range got {
		ids[co.ID.String()] = true
	}
	require.True(t, ids[a.Company.ID.String()])
	require.True(t, ids[b.Company.ID.String()])
	require.False(t, ids[c.Company.ID.String()], "soft-deleted companies are excluded")

	named, err := repo.ListCompanies(ctx, store.CompanyFilter{NameContains: b.Company.Name})
	require.NoError(t, err)
	require.Len(t, named, 1)
	require.Equal(t, b.Company.ID, named[0].ID)
}
