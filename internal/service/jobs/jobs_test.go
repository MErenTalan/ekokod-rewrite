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
