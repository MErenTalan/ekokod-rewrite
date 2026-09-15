//go:build integration

package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestAdminAuditAppendPlatformRefusesATenantEntry pins the admin-wide rule:
// a model value with a non-nil CompanyID is refused with ErrNotFound.
func TestAdminAuditAppendPlatformRefusesATenantEntry(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := admin.NewAuditRepository(pool)

	companyID := tenant.Company.ID
	_, err := repo.AppendPlatform(ctx, model.AuditEntry{
		CompanyID: &companyID, Action: "x", EntityType: "y", CreatedAt: time.Now().UTC(),
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	entry, err := repo.AppendPlatform(ctx, model.AuditEntry{
		Action: "catalogue.upsert", EntityType: "national_tariff_schedule", CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.Nil(t, entry.CompanyID)
	require.NotZero(t, entry.ID)
}
