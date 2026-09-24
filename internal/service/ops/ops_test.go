package ops_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/ops"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var companyScope = store.Scope{CompanyID: uuid.MustParse("00000000-0000-0000-0000-0000000000c0"), AllBuildings: true}

type fakeEnqueuer struct {
	tasks []*asynq.Task
	err   error
}

func (f *fakeEnqueuer) Enqueue(_ context.Context, t *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.tasks = append(f.tasks, t)
	return &asynq.TaskInfo{ID: "task-" + t.Type()}, nil
}

type fakeOps struct {
	store.OpsRepository
	messageFilter store.MessageFilter
	runFilter     store.JobRunFilter
}

func (f *fakeOps) ListMessages(_ context.Context, _ store.Scope, fl store.MessageFilter) ([]model.OperationalMessage, error) {
	f.messageFilter = fl
	return nil, nil
}

func (f *fakeOps) ListRuns(_ context.Context, _ store.Scope, fl store.JobRunFilter) ([]model.JobRun, error) {
	f.runFilter = fl
	return nil, nil
}

func newService(t *testing.T) (*ops.Service, *fakeEnqueuer, *fakeOps) {
	t.Helper()
	enq, repo := &fakeEnqueuer{}, &fakeOps{}
	svc, err := ops.New(ops.Deps{Ops: repo, Enqueuer: enq,
		Clock: clock.NewFake(time.Date(2026, 9, 18, 14, 37, 0, 0, time.UTC)), MaxRetry: 3})
	require.NoError(t, err)
	return svc, enq, repo
}

func TestTriggerRefusesAnythingOffTheAllowList(t *testing.T) {
	t.Parallel()
	svc, enq, _ := newService(t)
	for _, bad := range []string{"system.noop", "billing.generate", "alarm.notify", "alarm.dispatch", "", "../etc"} {
		_, err := svc.Trigger(context.Background(), companyScope, bad)
		require.Equal(t, http.StatusUnprocessableEntity, perr.StatusOf(err), bad)
	}
	// R220: nothing off the list ever reaches asynq, not even to be rejected there.
	require.Empty(t, enq.tasks)
}

func TestTriggerEnqueuesEveryAllowListedType(t *testing.T) {
	t.Parallel()
	svc, enq, _ := newService(t)
	for _, good := range ops.Triggerable {
		id, err := svc.Trigger(context.Background(), companyScope, good)
		require.NoError(t, err, good)
		require.NotEmpty(t, id, good)
	}
	require.Len(t, enq.tasks, len(ops.Triggerable))
}

func TestTriggerScopesAlarmEvaluateToTheCallersCompany(t *testing.T) {
	t.Parallel()
	svc, enq, _ := newService(t)
	_, err := svc.Trigger(context.Background(), companyScope, job.TypeAlarmEvaluate)
	require.NoError(t, err)

	var p job.AlarmEvaluatePayload
	require.NoError(t, json.Unmarshal(enq.tasks[0].Payload(), &p))
	require.Equal(t, companyScope.CompanyID, p.CompanyID, "never another tenant's company id")
	// R225: the hour is truncated, so a trigger inside the tick's hour is the
	// same task the tick would enqueue.
	require.Equal(t, time.Date(2026, 9, 18, 14, 0, 0, 0, time.UTC), p.Hour.UTC())
}

func TestMessagesRejectsAnOverlongQuery(t *testing.T) {
	t.Parallel()
	svc, _, _ := newService(t)
	_, err := svc.Messages(context.Background(), companyScope, store.MessageFilter{Q: strings.Repeat("a", 201)})
	require.Equal(t, http.StatusUnprocessableEntity, perr.StatusOf(err))

	_, err = svc.Messages(context.Background(), companyScope, store.MessageFilter{Q: strings.Repeat("a", 200)})
	require.NoError(t, err, "exactly at the cap is allowed")
}

func TestMessagesAndRunsPassTheirFiltersThrough(t *testing.T) {
	t.Parallel()
	svc, _, repo := newService(t)
	_, err := svc.Messages(context.Background(), companyScope,
		store.MessageFilter{Q: "alarm", Kinds: []string{"job"}, Statuses: []string{"error"}})
	require.NoError(t, err)
	require.Equal(t, "alarm", repo.messageFilter.Q)
	require.Equal(t, []string{"job"}, repo.messageFilter.Kinds)
	require.Equal(t, []string{"error"}, repo.messageFilter.Statuses)

	jobType := job.TypeAlarmEvaluate
	_, err = svc.JobRuns(context.Background(), companyScope, store.JobRunFilter{JobType: &jobType})
	require.NoError(t, err)
	require.Equal(t, &jobType, repo.runFilter.JobType)
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	_, err := ops.New(ops.Deps{})
	require.Error(t, err)
}

func TestTriggerReportsAnAlreadyQueuedJobAsAConflict(t *testing.T) {
	t.Parallel()
	// R225 de-duplicates a trigger against the cron tick's own hour. That is a
	// state the operator can act on, not a crash: an e2e run surfaced it as
	// "Beklenmeyen bir hata oluştu" before this mapping existed.
	for _, dup := range []error{asynq.ErrTaskIDConflict, asynq.ErrDuplicateTask} {
		svc, enq, _ := newService(t)
		enq.err = dup

		_, err := svc.Trigger(context.Background(), companyScope, job.TypeAlarmEvaluate)
		require.Equal(t, http.StatusConflict, perr.StatusOf(err))
		require.Equal(t, "job_already_queued", perr.CodeOf(err))
	}
}

func TestTriggerStillReportsRealEnqueueFailures(t *testing.T) {
	t.Parallel()
	svc, enq, _ := newService(t)
	enq.err = errors.New("redis unreachable")
	_, err := svc.Trigger(context.Background(), companyScope, job.TypeAlarmEvaluate)
	require.Error(t, err)
	require.NotEqual(t, "job_already_queued", perr.CodeOf(err))
}

// R268: both report ticks are operator-triggerable, each as its own type.
func TestTriggerableIncludesReportTicks(t *testing.T) {
	require.Contains(t, ops.Triggerable, job.TypeReportDispatchMonthly)
	require.Contains(t, ops.Triggerable, job.TypeReportDispatchYearly)
	svc, enq, _ := newService(t)
	for _, kind := range []string{job.TypeReportDispatchMonthly, job.TypeReportDispatchYearly} {
		_, err := svc.Trigger(context.Background(), companyScope, kind)
		require.NoError(t, err)
	}
	require.Equal(t, job.TypeReportDispatchMonthly, enq.tasks[0].Type())
	require.Equal(t, job.TypeReportDispatchYearly, enq.tasks[1].Type())
}

// R311: the carbon accrual is operator-triggerable (it accrues yesterday).
func TestTriggerableIncludesCarbonAccrual(t *testing.T) {
	require.Contains(t, ops.Triggerable, job.TypeCarbonAccrual)
	require.Contains(t, ops.Triggerable, job.TypeForecastRun)
	svc, enq, _ := newService(t)
	_, err := svc.Trigger(context.Background(), companyScope, job.TypeCarbonAccrual)
	require.NoError(t, err)
	require.Equal(t, job.TypeCarbonAccrual, enq.tasks[0].Type())
}

// R288: both iSolar ticks are operator-triggerable.
func TestTriggerableIncludesIsolarTicks(t *testing.T) {
	require.Contains(t, ops.Triggerable, job.TypeSolarDispatchSync)
	require.Contains(t, ops.Triggerable, job.TypeSolarFetchAlarms)
	svc, enq, _ := newService(t)
	for _, kind := range []string{job.TypeSolarDispatchSync, job.TypeSolarFetchAlarms} {
		_, err := svc.Trigger(context.Background(), companyScope, kind)
		require.NoError(t, err)
	}
	require.Equal(t, job.TypeSolarDispatchSync, enq.tasks[0].Type())
	require.Equal(t, job.TypeSolarFetchAlarms, enq.tasks[1].Type())
}
