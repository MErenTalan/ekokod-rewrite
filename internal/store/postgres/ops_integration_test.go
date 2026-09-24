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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestOpsCrossTenantIsolation is the fix round 1, Important 4 test: every
// prior Ops isolation test exercised only a PLATFORM (company_id NULL) row
// leaking into a tenant's surface; none used a SECOND REAL company's rows.
// This proves GetRun/FinishRun/ListRuns/AppendMessage/ListMessages all
// refuse tenant A's rows under tenant B's OWN AdminScope — the company_id
// predicate itself, not a coincidence of "no other tenant existed yet".
func TestOpsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8030)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 8031)
	repo := postgres.NewOpsRepository(pool)

	companyA := tenantA.Company.ID
	runA, err := repo.StartRun(ctx, tenantA.Scope, model.JobRun{CompanyID: &companyA, JobType: "bill-generation"})
	require.NoError(t, err)

	companyB := tenantB.Company.ID
	msgA, err := repo.AppendMessage(ctx, tenantA.Scope, model.OperationalMessage{
		CompanyID: &companyA, Kind: "job", Category: "bill-generation", Status: "success", Message: "A's message",
	})
	require.NoError(t, err)

	// GetRun: tenant B's AdminScope cannot read tenant A's run.
	_, err = repo.GetRun(ctx, tenantB.AdminScope, runA.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	// FinishRun: tenant B's AdminScope cannot finish tenant A's run.
	_, err = repo.FinishRun(ctx, tenantB.AdminScope, runA.ID, "success", 1, 0, 0, nil, nil, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)

	stillRunning, err := repo.GetRun(ctx, tenantA.Scope, runA.ID)
	require.NoError(t, err)
	require.Equal(t, "running", stillRunning.Status, "tenant B's refused FinishRun must not have touched tenant A's run")

	// ListRuns: tenant A's run never appears under tenant B's AdminScope,
	// even once tenant B has runs of its own.
	_, err = repo.StartRun(ctx, tenantB.Scope, model.JobRun{CompanyID: &companyB, JobType: "bill-generation"})
	require.NoError(t, err)
	listB, err := repo.ListRuns(ctx, tenantB.AdminScope, store.JobRunFilter{})
	require.NoError(t, err)
	for _, r := range listB {
		require.NotEqual(t, runA.ID, r.ID)
	}

	// ListMessages: tenant A's message never appears under tenant B's
	// AdminScope.
	messagesB, err := repo.ListMessages(ctx, tenantB.AdminScope, store.MessageFilter{})
	require.NoError(t, err)
	for _, m := range messagesB {
		require.NotEqual(t, msgA.ID, m.ID)
	}
}

// TestOpsStartRunIgnoresCallerSuppliedStatus is the folded-minor test (fix
// round 1): a job run is INSERTED in state 'running' per the contract,
// regardless of what the caller puts in run.Status.
func TestOpsStartRunIgnoresCallerSuppliedStatus(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 8040)
	repo := postgres.NewOpsRepository(pool)

	companyID := tenant.Company.ID
	started, err := repo.StartRun(ctx, tenant.Scope, model.JobRun{
		CompanyID: &companyID, JobType: "bill-generation", Status: "success",
	})
	require.NoError(t, err)
	require.Equal(t, "running", started.Status, "a caller-supplied Status must be ignored, not honoured, on StartRun")
}

// TestOpsListMessagesSearch is R226: the Messages screen's search box, a
// case-insensitive substring over message and detail.
func TestOpsListMessagesSearch(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 8101)
	repo := postgres.NewOpsRepository(pool)
	companyID := tenant.Company.ID

	detail := "Endüktif oran %25"
	for _, m := range []model.OperationalMessage{
		{CompanyID: &companyID, Kind: "job", Category: "analyzer-refresh", Status: "success", Message: "Analizör yenilendi"},
		{CompanyID: &companyID, Kind: "alarm", Category: "alarm-trigger", Status: "warning", Message: "Alarm tetiklendi", Detail: &detail},
		{CompanyID: &companyID, Kind: "system", Category: "auth", Status: "info", Message: "Oturum açıldı"},
	} {
		_, err := repo.AppendMessage(ctx, tenant.Scope, m)
		require.NoError(t, err)
	}

	got, err := repo.ListMessages(ctx, tenant.Scope, store.MessageFilter{Q: "alarm"})
	require.NoError(t, err)
	require.Len(t, got, 1, "matches the message column, case-insensitively")

	// "ENDÜKTIF" with an ASCII I, deliberately: Postgres' lower() maps the
	// Turkish dotted İ to "i" plus a combining dot, which would not match the
	// plain "i" stored in the text. The Ü still proves the fold is not ASCII-only.
	got, err = repo.ListMessages(ctx, tenant.Scope, store.MessageFilter{Q: "ENDÜKTIF"})
	require.NoError(t, err)
	require.Len(t, got, 1, "matches the detail column too")

	// A LIKE metacharacter is a literal, not a wildcard: '%' must match only
	// the row whose detail actually contains one.
	got, err = repo.ListMessages(ctx, tenant.Scope, store.MessageFilter{Q: "%"})
	require.NoError(t, err)
	require.Len(t, got, 1)

	got, err = repo.ListMessages(ctx, tenant.Scope, store.MessageFilter{Q: "_"})
	require.NoError(t, err)
	require.Empty(t, got, "an underscore is a literal too")

	got, err = repo.ListMessages(ctx, tenant.Scope, store.MessageFilter{Q: "yok"})
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = repo.ListMessages(ctx, tenant.Scope, store.MessageFilter{})
	require.NoError(t, err)
	require.Len(t, got, 3, "an empty Q filters nothing")
}

// TestOpsListMessagesSearchCombinesWithFilters pins that the search narrows
// the other filters rather than replacing them.
func TestOpsListMessagesSearchCombinesWithFilters(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 8102)
	repo := postgres.NewOpsRepository(pool)
	companyID := tenant.Company.ID

	for _, m := range []model.OperationalMessage{
		{CompanyID: &companyID, Kind: "alarm", Category: "alarm-trigger", Status: "warning", Message: "Alarm tetiklendi"},
		{CompanyID: &companyID, Kind: "job", Category: "alarm-evaluate", Status: "success", Message: "Alarm değerlendirmesi"},
	} {
		_, err := repo.AppendMessage(ctx, tenant.Scope, m)
		require.NoError(t, err)
	}

	got, err := repo.ListMessages(ctx, tenant.Scope, store.MessageFilter{Q: "alarm", Kinds: []string{"job"}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "job", got[0].Kind)
}

// R237: the screen holds a task id, so the run records one — and the NEWEST
// run for that id is the one that explains the job being watched.
func TestOpsRunByTaskIDReturnsTheNewestRunAndStaysInTenant(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 8009)
	other := testfixtures.NewTenant(t, ctx, pool, 8010)
	repo := postgres.NewOpsRepository(pool)

	companyID := tenant.Company.ID
	taskID := "billing.generate:building:11111111-1111-1111-1111-111111111111:2026-08"
	first, err := repo.StartRun(ctx, tenant.Scope, model.JobRun{CompanyID: &companyID, JobType: "billing.generate", TaskID: &taskID})
	require.NoError(t, err)
	require.NotNil(t, first.TaskID)
	require.Equal(t, taskID, *first.TaskID)

	_, err = repo.FinishRun(ctx, tenant.Scope, first.ID, "failed", 0, 0, 1, nil,
		[]byte(`{"code":"tariff_not_found"}`), time.Now().UTC())
	require.NoError(t, err)

	// A recomputation of the same period reuses the deterministic task id.
	second, err := repo.StartRun(ctx, tenant.Scope, model.JobRun{CompanyID: &companyID, JobType: "billing.generate", TaskID: &taskID})
	require.NoError(t, err)

	got, err := repo.RunByTaskID(ctx, tenant.Scope, taskID)
	require.NoError(t, err)
	require.Equal(t, second.ID, got.ID, "the newest run wins")

	// Another company's run with the same task id is invisible.
	otherCompany := other.Company.ID
	_, err = repo.StartRun(ctx, other.Scope, model.JobRun{CompanyID: &otherCompany, JobType: "billing.generate", TaskID: &taskID})
	require.NoError(t, err)
	still, err := repo.RunByTaskID(ctx, tenant.Scope, taskID)
	require.NoError(t, err)
	require.Equal(t, second.ID, still.ID)

	_, err = repo.RunByTaskID(ctx, tenant.Scope, "billing.generate:building:unknown:2026-08")
	require.ErrorIs(t, err, store.ErrNotFound)
}
