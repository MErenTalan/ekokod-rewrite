// Package ops is the read side of 01 §7.13 and 09 §F7's job history: the
// Messages screen, the job-run list and manual job triggering.
//
// Nothing here writes a message or a run — those are written by the jobs
// themselves. Triggering enqueues an allow-listed task and returns its id, so
// GET /jobs/{id} (R192) can watch it with the convention F6b already set.
package ops

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/hibiken/asynq"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// maxQueryLength caps the Messages search box (R226).
const maxQueryLength = 200

// Triggerable is R220's allow-list. Nothing outside it can ever be enqueued,
// so no caller can name an arbitrary asynq task type — the reason this is a
// list rather than a pass-through of whatever the path segment said.
//
// consumption.refresh is deliberately NOT here, though the phase plan listed
// it: its payload needs an explicit From/To, and a no-argument button would
// have to invent a window the operator never chose. A trigger that takes a
// period belongs with the screens that already ask for one (F8).
var Triggerable = []string{
	job.TypeAlarmEvaluate,
	job.TypeBillingDispatch,
	job.TypeIntegrationSyncDispatch,
	job.TypeEPIASSyncPrices,
	// R268: the report ticks, so an operator can re-run a missed month or year.
	job.TypeReportDispatchMonthly,
	job.TypeReportDispatchYearly,
	// R288: the iSolar ticks.
	job.TypeSolarDispatchSync,
	job.TypeSolarFetchAlarms,
	// R311: the carbon accrual, for yesterday.
	job.TypeCarbonAccrual,
}

// TaskEnqueuer is the job client seam.
type TaskEnqueuer interface {
	Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Deps is what New needs.
type Deps struct {
	Ops      store.OpsRepository
	Enqueuer TaskEnqueuer
	Clock    clock.Clock
	MaxRetry int
}

// Service reads operational messages and job runs, and triggers jobs.
type Service struct{ d Deps }

// New validates deps.
func New(d Deps) (*Service, error) {
	if d.Ops == nil || d.Enqueuer == nil || d.Clock == nil {
		return nil, errors.New("ops: Ops, Enqueuer and Clock are required")
	}
	return &Service{d: d}, nil
}

func validation(field string, codes ...string) error {
	return perr.Validation.WithParams(map[string]any{field: codes})
}

// Messages lists the tenant's operational messages.
func (s *Service) Messages(ctx context.Context, sc store.Scope, f store.MessageFilter) ([]model.OperationalMessage, error) {
	if len(f.Q) > maxQueryLength {
		return nil, validation("q", "max")
	}
	return s.d.Ops.ListMessages(ctx, sc, f)
}

// JobRuns lists the tenant's job history.
func (s *Service) JobRuns(ctx context.Context, sc store.Scope, f store.JobRunFilter) ([]model.JobRun, error) {
	return s.d.Ops.ListRuns(ctx, sc, f)
}

// Trigger enqueues one allow-listed job for the caller's own company and
// returns the asynq task id.
//
// The company always comes from the Scope, never from the request: a trigger
// that took a company id would let an admin start another tenant's work from a
// path that says nothing about scope.
func (s *Service) Trigger(ctx context.Context, sc store.Scope, jobType string) (string, error) {
	if !slices.Contains(Triggerable, jobType) {
		return "", validation("type", "oneof")
	}
	task, err := s.taskFor(sc, jobType)
	if err != nil {
		return "", err
	}
	info, err := s.d.Enqueuer.Enqueue(ctx, task)
	switch {
	case errors.Is(err, asynq.ErrTaskIDConflict), errors.Is(err, asynq.ErrDuplicateTask):
		// R225 working: this job is already queued for this scope and hour.
		// That is a conflict the operator can understand, not a crash — an
		// e2e run surfaced it as "Beklenmeyen bir hata oluştu".
		return "", perr.New("job_already_queued", perr.Conflict.HTTPStatus, "errors.jobs.alreadyQueued")
	case err != nil:
		return "", err
	}
	return info.ID, nil
}

// taskFor builds the one task this type means. alarm.evaluate is the only
// company-scoped member of the list; the others are platform ticks that
// fan out to every tenant themselves.
func (s *Service) taskFor(sc store.Scope, jobType string) (*asynq.Task, error) {
	opts := job.TaskOptions{MaxRetry: s.d.MaxRetry}
	switch jobType {
	case job.TypeAlarmEvaluate:
		return job.NewAlarmEvaluateTask(job.AlarmEvaluatePayload{
			CompanyID: sc.CompanyID, Hour: s.d.Clock.Now().UTC().Truncate(time.Hour),
		}, opts)
	case job.TypeBillingDispatch:
		return job.NewBillingDispatchTask(opts)
	case job.TypeIntegrationSyncDispatch:
		return job.NewSyncDispatchTask(opts)
	case job.TypeEPIASSyncPrices:
		return job.NewSyncPricesTask(job.SyncPricesPayload{}, opts)
	case job.TypeReportDispatchMonthly:
		return job.NewReportDispatchTask("monthly", opts)
	case job.TypeReportDispatchYearly:
		return job.NewReportDispatchTask("yearly", opts)
	case job.TypeSolarDispatchSync:
		return job.NewSolarDispatchTask(opts)
	case job.TypeSolarFetchAlarms:
		return job.NewSolarAlarmsTask(opts)
	case job.TypeCarbonAccrual:
		return job.NewCarbonAccrualTask(job.CarbonAccrualPayload{}, opts)
	}
	// Unreachable: Trigger checked the allow-list first. Refusing rather than
	// falling through keeps a new list entry from silently enqueuing nothing.
	return nil, validation("type", "oneof")
}
