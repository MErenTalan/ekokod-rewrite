package jobs_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/jobs"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var (
	companyA  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	companyB  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	analyzerA = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	analyzerB = uuid.MustParse("44444444-4444-4444-4444-444444444444")
	buildingA = uuid.MustParse("55555555-5555-5555-5555-555555555555")
	buildingB = uuid.MustParse("66666666-6666-6666-6666-666666666666")
)

type fakeInspector struct {
	tasks   map[string]map[string]*asynq.TaskInfo // queue → id
	queried []string
}

func (f *fakeInspector) GetTaskInfo(queue, id string) (*asynq.TaskInfo, error) {
	f.queried = append(f.queried, queue)
	if info, ok := f.tasks[queue][id]; ok {
		return info, nil
	}
	return nil, asynq.ErrTaskNotFound
}

// fakeAnalyzers answers Get only for the analyzers the scope may see.
type fakeAnalyzers struct {
	store.AnalyzerRepository
	visible map[uuid.UUID]bool
}

func (f fakeAnalyzers) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.Analyzer, error) {
	if f.visible[id] {
		return model.Analyzer{ID: id}, nil
	}
	return model.Analyzer{}, store.ErrNotFound
}

func taskInfo(id, queue, taskType string, state asynq.TaskState, payload any) *asynq.TaskInfo {
	raw, _ := json.Marshal(payload)
	return &asynq.TaskInfo{ID: id, Queue: queue, Type: taskType, State: state, Payload: raw}
}

func newService(t *testing.T, insp *fakeInspector, visible ...uuid.UUID) *jobs.Service {
	t.Helper()
	seen := map[uuid.UUID]bool{}
	for _, id := range visible {
		seen[id] = true
	}
	s, err := jobs.New(jobs.Deps{Inspector: insp, Analyzers: fakeAnalyzers{visible: seen}})
	require.NoError(t, err)
	return s
}

func scopeA() store.Scope { return store.SystemScope(companyA) }

func TestJobStatusMapping(t *testing.T) {
	t.Parallel()
	completed := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		state asynq.TaskState
		want  jobs.Status
	}{
		{asynq.TaskStatePending, jobs.Queued},
		{asynq.TaskStateScheduled, jobs.Queued},
		{asynq.TaskStateAggregating, jobs.Queued},
		{asynq.TaskStateActive, jobs.Running},
		{asynq.TaskStateRetry, jobs.Running},
		{asynq.TaskStateCompleted, jobs.Succeeded},
		{asynq.TaskStateArchived, jobs.Failed},
	}
	for _, c := range cases {
		info := taskInfo("t1", job.QueueDefault, job.TypeIntegrationRefreshAnalyzer, c.state,
			job.RefreshAnalyzerPayload{CompanyID: companyA, AnalyzerID: analyzerA, Mode: job.RefreshModeHourly})
		if c.state == asynq.TaskStateCompleted {
			info.CompletedAt = completed
		}
		insp := &fakeInspector{tasks: map[string]map[string]*asynq.TaskInfo{job.QueueDefault: {"t1": info}}}
		v, err := newService(t, insp, analyzerA).Get(t.Context(), scopeA(), "t1")
		require.NoError(t, err, c.state)
		require.Equal(t, c.want, v.Status, c.state)
		require.Equal(t, job.TypeIntegrationRefreshAnalyzer, v.Type)
		if c.state == asynq.TaskStateCompleted {
			require.NotNil(t, v.CompletedAt)
			require.True(t, completed.Equal(*v.CompletedAt))
		} else {
			require.Nil(t, v.CompletedAt, c.state)
		}
	}
}

func TestJobHiddenOutsideScope(t *testing.T) {
	t.Parallel()
	refresh := func(company, analyzer uuid.UUID) *asynq.TaskInfo {
		return taskInfo("t1", job.QueueDefault, job.TypeIntegrationRefreshAnalyzer, asynq.TaskStatePending,
			job.RefreshAnalyzerPayload{CompanyID: company, AnalyzerID: analyzer, Mode: job.RefreshModeHourly})
	}
	cases := []struct {
		name    string
		info    *asynq.TaskInfo
		visible []uuid.UUID
	}{
		{"another company", refresh(companyB, analyzerB), []uuid.UUID{analyzerA, analyzerB}},
		{"an analyzer outside the scope", refresh(companyA, analyzerB), []uuid.UUID{analyzerA}},
		{"a job type no screen starts", taskInfo("t1", job.QueueDefault, "billing.generate", asynq.TaskStatePending,
			map[string]any{"CompanyID": companyA}), []uuid.UUID{analyzerA}},
		{"an unreadable payload", &asynq.TaskInfo{ID: "t1", Queue: job.QueueDefault,
			Type: job.TypeIntegrationRefreshAnalyzer, State: asynq.TaskStatePending, Payload: []byte("not json")}, nil},
	}
	for _, c := range cases {
		insp := &fakeInspector{tasks: map[string]map[string]*asynq.TaskInfo{job.QueueDefault: {"t1": c.info}}}
		_, err := newService(t, insp, c.visible...).Get(t.Context(), scopeA(), "t1")
		require.ErrorIs(t, err, store.ErrNotFound, c.name)
	}

	empty := &fakeInspector{tasks: map[string]map[string]*asynq.TaskInfo{}}
	_, err := newService(t, empty).Get(t.Context(), scopeA(), "missing")
	require.ErrorIs(t, err, store.ErrNotFound, "an unknown id answers exactly like a hidden one")
	require.Equal(t, []string{job.QueueCritical, job.QueueDefault, job.QueueLow}, empty.queried)
}

func TestJobFoundInTheLastQueue(t *testing.T) {
	t.Parallel()
	insp := &fakeInspector{tasks: map[string]map[string]*asynq.TaskInfo{job.QueueLow: {"b1": taskInfo(
		"b1", job.QueueLow, job.TypeIntegrationBackfill, asynq.TaskStateActive,
		job.BackfillPayload{CompanyID: companyA, CredentialID: uuid.New()})}}}
	v, err := newService(t, insp).Get(t.Context(), scopeA(), "b1")
	require.NoError(t, err)
	require.Equal(t, jobs.Running, v.Status)
	require.Equal(t, job.TypeIntegrationBackfill, v.Type)
}

func buildingScope() store.Scope {
	return store.Scope{CompanyID: companyA, BuildingIDs: []uuid.UUID{uuid.MustParse("55555555-5555-5555-5555-555555555555")}}
}

func TestJobBackfillChecksEveryAnalyzer(t *testing.T) {
	t.Parallel()
	backfill := func(ids ...uuid.UUID) *fakeInspector {
		return &fakeInspector{tasks: map[string]map[string]*asynq.TaskInfo{job.QueueLow: {"b1": taskInfo(
			"b1", job.QueueLow, job.TypeIntegrationBackfill, asynq.TaskStateActive,
			job.BackfillPayload{CompanyID: companyA, CredentialID: uuid.New(), AnalyzerIDs: ids})}}}
	}
	_, err := newService(t, backfill(analyzerA, analyzerB), analyzerA).Get(t.Context(), buildingScope(), "b1")
	require.ErrorIs(t, err, store.ErrNotFound, "one analyzer outside the scope hides the whole job")

	v, err := newService(t, backfill(analyzerA), analyzerA).Get(t.Context(), buildingScope(), "b1")
	require.NoError(t, err)
	require.Equal(t, jobs.Running, v.Status)
}

func TestJobWithoutAnalyzersNeedsTheWholeCompany(t *testing.T) {
	t.Parallel()
	insp := func() *fakeInspector {
		return &fakeInspector{tasks: map[string]map[string]*asynq.TaskInfo{job.QueueDefault: {"s1": taskInfo(
			"s1", job.QueueDefault, job.TypeIntegrationSyncAnalyzers, asynq.TaskStatePending,
			job.SyncAnalyzersPayload{CompanyID: companyA, CredentialID: uuid.New()})}}}
	}
	_, err := newService(t, insp(), analyzerA).Get(t.Context(), buildingScope(), "s1")
	require.ErrorIs(t, err, store.ErrNotFound, "a company-wide job is not a building role's to watch")

	v, err := newService(t, insp()).Get(t.Context(), scopeA(), "s1")
	require.NoError(t, err)
	require.Equal(t, jobs.Queued, v.Status)
}

// fakeBuildings answers Get only for the buildings the scope may see.
type fakeBuildings struct {
	store.BuildingRepository
	visible map[uuid.UUID]bool
}

func (f fakeBuildings) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.Building, error) {
	if f.visible[id] {
		return model.Building{ID: id}, nil
	}
	return model.Building{}, store.ErrNotFound
}

// fakeOps answers RunByTaskID from a fixed table, like the job_runs row the
// billing generator writes.
type fakeOps struct {
	store.OpsRepository
	runs map[string]model.JobRun
	asks []string
}

func (f *fakeOps) RunByTaskID(_ context.Context, _ store.Scope, taskID string) (model.JobRun, error) {
	f.asks = append(f.asks, taskID)
	if run, ok := f.runs[taskID]; ok {
		return run, nil
	}
	return model.JobRun{}, store.ErrNotFound
}

func billingService(t *testing.T, insp *fakeInspector, ops *fakeOps, buildings ...uuid.UUID) *jobs.Service {
	t.Helper()
	seen := map[uuid.UUID]bool{}
	for _, id := range buildings {
		seen[id] = true
	}
	s, err := jobs.New(jobs.Deps{Inspector: insp, Analyzers: fakeAnalyzers{}, Buildings: fakeBuildings{visible: seen}, Ops: ops})
	require.NoError(t, err)
	return s
}

func billingTask(id string, state asynq.TaskState, scope model.BillScope, subject uuid.UUID) *asynq.TaskInfo {
	return taskInfo(id, job.QueueDefault, job.TypeBillingGenerate, state,
		job.BillingGeneratePayload{CompanyID: companyA, Scope: scope, SubjectID: subject, PeriodKey: "2026-08"})
}

func TestBillingGenerateIsWatchable(t *testing.T) {
	t.Parallel()
	// Before F8a the allow-list held only the three integration jobs, so the
	// bills screen could not watch the job it had just started (R237).
	info := billingTask("bg-1", asynq.TaskStatePending, model.BillScopeBuilding, buildingA)
	insp := &fakeInspector{tasks: map[string]map[string]*asynq.TaskInfo{job.QueueDefault: {"bg-1": info}}}
	v, err := billingService(t, insp, &fakeOps{}, buildingA).Get(t.Context(), scopeA(), "bg-1")
	require.NoError(t, err)
	require.Equal(t, job.TypeBillingGenerate, v.Type)
	require.Equal(t, jobs.Queued, v.Status)
	require.Empty(t, v.ErrorCode, "a queued job has no failure to report")
}

func TestBillingGenerateFailureCarriesItsR113Code(t *testing.T) {
	t.Parallel()
	info := billingTask("bg-2", asynq.TaskStateArchived, model.BillScopeBuilding, buildingA)
	insp := &fakeInspector{tasks: map[string]map[string]*asynq.TaskInfo{job.QueueDefault: {"bg-2": info}}}
	taskID := job.BillingGenerateTaskID(job.BillingGeneratePayload{CompanyID: companyA, Scope: model.BillScopeBuilding,
		SubjectID: buildingA, PeriodKey: "2026-08"})
	ops := &fakeOps{runs: map[string]model.JobRun{
		taskID: {Status: "failed", Detail: json.RawMessage(`{"code":"tariff_not_found","detail":{"building":"x"}}`)},
	}}
	v, err := billingService(t, insp, ops, buildingA).Get(t.Context(), scopeA(), "bg-2")
	require.NoError(t, err)
	require.Equal(t, jobs.Failed, v.Status)
	require.Equal(t, "tariff_not_found", v.ErrorCode, "§7.10's message needs the machine code")
	require.Equal(t, []string{taskID}, ops.asks)
}

func TestErrorCodeIsNeverFreeText(t *testing.T) {
	t.Parallel()
	// R192 holds: the worker's own error text never leaves the process, and a
	// code outside R113's closed set is not repeated either.
	for _, detail := range []string{`{"error":"dial tcp 10.0.0.1:5432: connect: refused"}`, `{"code":"dial tcp failed"}`} {
		info := billingTask("bg-3", asynq.TaskStateArchived, model.BillScopeBuilding, buildingA)
		insp := &fakeInspector{tasks: map[string]map[string]*asynq.TaskInfo{job.QueueDefault: {"bg-3": info}}}
		taskID := job.BillingGenerateTaskID(job.BillingGeneratePayload{CompanyID: companyA, Scope: model.BillScopeBuilding,
			SubjectID: buildingA, PeriodKey: "2026-08"})
		ops := &fakeOps{runs: map[string]model.JobRun{taskID: {Status: "failed", Detail: json.RawMessage(detail)}}}
		v, err := billingService(t, insp, ops, buildingA).Get(t.Context(), scopeA(), "bg-3")
		require.NoError(t, err)
		require.Empty(t, v.ErrorCode, detail)
	}
}

func TestBillingGenerateIsScopedToItsSubject(t *testing.T) {
	t.Parallel()
	info := billingTask("bg-4", asynq.TaskStatePending, model.BillScopeBuilding, buildingB)
	insp := &fakeInspector{tasks: map[string]map[string]*asynq.TaskInfo{job.QueueDefault: {"bg-4": info}}}
	// buildingB is outside the scope: the answer is the same as for an
	// unknown id, so a job id is never an oracle.
	_, err := billingService(t, insp, &fakeOps{}, buildingA).Get(t.Context(), scopeA(), "bg-4")
	require.ErrorIs(t, err, store.ErrNotFound)

	company := billingTask("bg-5", asynq.TaskStatePending, model.BillScopeCompany, companyA)
	insp = &fakeInspector{tasks: map[string]map[string]*asynq.TaskInfo{job.QueueDefault: {"bg-5": company}}}
	scoped := store.Scope{CompanyID: companyA, BuildingIDs: []uuid.UUID{buildingA}}
	_, err = billingService(t, insp, &fakeOps{}, buildingA).Get(t.Context(), scoped, "bg-5")
	require.ErrorIs(t, err, store.ErrNotFound, "a company-wide job needs a company-wide scope")
}
