//go:build integration

package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestAdminStartAndFinishPlatformRun(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	repo := admin.NewJournalRepository(pool)

	started, err := repo.StartPlatformRun(ctx, model.JobRun{JobType: "market-import"})
	require.NoError(t, err)
	require.Nil(t, started.CompanyID)
	require.Equal(t, "running", started.Status)

	finished, err := repo.FinishPlatformRun(ctx, started.ID, "success", 10, 2, 0, nil, []byte(`{"note":"ok"}`), time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, "success", finished.Status)
	require.EqualValues(t, 10, finished.Processed)

	// A run whose CompanyID is non-nil is refused before any database call.
	companyID := uuid.New()
	_, err = repo.StartPlatformRun(ctx, model.JobRun{CompanyID: &companyID, JobType: "x"})
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestAdminFinishPlatformRunNeverTouchesATenantRun proves the isolation the
// interface promises: a tenant's job run id returns ErrNotFound from the
// admin surface, and nothing is written.
func TestAdminFinishPlatformRunNeverTouchesATenantRun(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 9010)
	opsRepo := postgres.NewOpsRepository(pool)
	journalRepo := admin.NewJournalRepository(pool)

	companyID := tenant.Company.ID
	tenantRun, err := opsRepo.StartRun(ctx, tenant.Scope, model.JobRun{CompanyID: &companyID, JobType: "bill-generation"})
	require.NoError(t, err)

	_, err = journalRepo.FinishPlatformRun(ctx, tenantRun.ID, "success", 1, 0, 0, nil, nil, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)

	stillRunning, err := opsRepo.GetRun(ctx, tenant.Scope, tenantRun.ID)
	require.NoError(t, err)
	require.Equal(t, "running", stillRunning.Status, "the refused admin finish must not have touched the tenant's run")
}

func TestAdminAppendPlatformMessage(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	repo := admin.NewJournalRepository(pool)

	msg, err := repo.AppendPlatformMessage(ctx, model.OperationalMessage{
		Kind: "system", Category: "market-import", Status: "info", Message: "started",
	})
	require.NoError(t, err)
	require.Nil(t, msg.CompanyID)
	require.Positive(t, msg.ID)

	companyID := uuid.New()
	_, err = repo.AppendPlatformMessage(ctx, model.OperationalMessage{
		CompanyID: &companyID, Kind: "system", Category: "x", Status: "info", Message: "x",
	})
	require.ErrorIs(t, err, store.ErrNotFound)
}
