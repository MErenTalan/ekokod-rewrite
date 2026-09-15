//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestOpsJobRunNullableCompanyIDIsolation is the mandatory isolation test:
// job_runs.company_id is nullable for platform work, and every OpsRepository
// method must store and see only company_id = s.CompanyID, never a platform
// (NULL) row.
func TestOpsJobRunNullableCompanyIDIsolation(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 8001)
	repo := postgres.NewOpsRepository(pool)

	companyID := tenant.Company.ID
	started, err := repo.StartRun(ctx, tenant.Scope, model.JobRun{CompanyID: &companyID, JobType: "bill-generation"})
	require.NoError(t, err)
	require.Equal(t, "running", started.Status)

	// A run whose CompanyID is nil (a platform run) is refused before any
	// database call.
	_, err = repo.StartRun(ctx, tenant.Scope, model.JobRun{JobType: "market-import"})
	require.ErrorIs(t, err, store.ErrNotFound)

	// A platform run, inserted directly (company_id null), is invisible to
	// every tenant.
	var platformRunID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into job_runs (company_id, job_type) values (null, 'market-import') returning id`,
	).Scan(&platformRunID))

	_, err = repo.GetRun(ctx, tenant.Scope, platformRunID)
	require.ErrorIs(t, err, store.ErrNotFound)

	finished, err := repo.FinishRun(ctx, tenant.Scope, platformRunID, "success", 1, 0, 0, nil, nil, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Zero(t, finished.ID, "a platform run's id must not be finishable through the tenant surface")

	list, err := repo.ListRuns(ctx, tenant.Scope, store.JobRunFilter{})
	require.NoError(t, err)
	require.Len(t, list, 1)
	for _, r := range list {
		require.NotEqual(t, platformRunID, r.ID, "a platform run must never appear in a tenant's ListRuns")
	}

	realFinished, err := repo.FinishRun(ctx, tenant.Scope, started.ID, "success", 5, 1, 0, nil, []byte(`{"ok":true}`), time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, "success", realFinished.Status)
	require.EqualValues(t, 5, realFinished.Processed)

	running, err := repo.ListRuns(ctx, tenant.Scope, store.JobRunFilter{Running: true})
	require.NoError(t, err)
	require.Empty(t, running, "the finished run must not appear in a Running-only filter")
}

// TestOpsOperationalMessageNullableCompanyIDIsolation is the mandatory
// isolation test for operational_messages.
func TestOpsOperationalMessageNullableCompanyIDIsolation(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 8010)
	repo := postgres.NewOpsRepository(pool)

	companyID := tenant.Company.ID
	msg, err := repo.AppendMessage(ctx, tenant.Scope, model.OperationalMessage{
		CompanyID: &companyID, Kind: "job", Category: "bill-generation", Status: "success", Message: "done",
	})
	require.NoError(t, err)
	require.Positive(t, msg.ID)

	_, err = repo.AppendMessage(ctx, tenant.Scope, model.OperationalMessage{
		Kind: "system", Category: "market-import", Status: "info", Message: "platform message",
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = pool.Exec(ctx,
		`insert into operational_messages (company_id, kind, category, status, message)
		 values (null, 'system', 'market-import', 'info', 'platform message')`)
	require.NoError(t, err)

	list, err := repo.ListMessages(ctx, tenant.Scope, store.MessageFilter{})
	require.NoError(t, err)
	require.Len(t, list, 1, "a platform message must never appear in a tenant's ListMessages")
	require.Equal(t, "bill-generation", list[0].Category)

	filtered, err := repo.ListMessages(ctx, tenant.Scope, store.MessageFilter{Kinds: []string{"job"}})
	require.NoError(t, err)
	require.Len(t, filtered, 1)

	none, err := repo.ListMessages(ctx, tenant.Scope, store.MessageFilter{Kinds: []string{"system"}})
	require.NoError(t, err)
	require.Empty(t, none)
}

func TestOpsListRunsRejectsInvalidRange(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 8020)
	repo := postgres.NewOpsRepository(pool)

	invalidRange := &store.TimeRange{}
	_, err := repo.ListRuns(ctx, tenant.Scope, store.JobRunFilter{Range: invalidRange})
	require.ErrorIs(t, err, store.ErrInvalidRange)

	_, err = repo.ListMessages(ctx, tenant.Scope, store.MessageFilter{Range: invalidRange})
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

func TestOpsRepositoryRejectsInvalidScope(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	repo := postgres.NewOpsRepository(pool)
	var invalid store.Scope

	_, err := repo.StartRun(ctx, invalid, model.JobRun{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.GetRun(ctx, invalid, uuid.New())
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.ListRuns(ctx, invalid, store.JobRunFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.AppendMessage(ctx, invalid, model.OperationalMessage{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.ListMessages(ctx, invalid, store.MessageFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}
