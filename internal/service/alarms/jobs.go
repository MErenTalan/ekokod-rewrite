package alarms

import (
	"context"

	"github.com/hibiken/asynq"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

// TaskEnqueuer is the job client seam, as internal/service/billing declares
// its own: the service depends on the shape it uses, not on asynq.
type TaskEnqueuer interface {
	Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// JobAdapter connects the Service to the three asynq task types. One type
// implements all three interfaces because they share a Service; Register only
// routes the ones it is given, so a worker built without alarm deps simply has
// no route for them.
type JobAdapter struct {
	Service *Service
	Clock   clock.Clock
}

var (
	_ job.AlarmDispatcher = JobAdapter{}
	_ job.AlarmEvaluator  = JobAdapter{}
	_ job.AlarmNotifier   = JobAdapter{}
)

// Dispatch runs the hourly platform tick.
func (a JobAdapter) Dispatch(ctx context.Context) error { return a.Service.Dispatch(ctx) }

// Evaluate runs one company's rules. The payload's Hour is what the task was
// keyed on, but the evaluation itself uses the CURRENT time: a task retried
// twenty minutes later must measure the window that ends now, not the one that
// ended when it was enqueued.
func (a JobAdapter) Evaluate(ctx context.Context, p job.AlarmEvaluatePayload) error {
	return a.Service.Run(ctx, p.CompanyID, a.Clock.Now())
}

// Notify delivers one event.
func (a JobAdapter) Notify(ctx context.Context, p job.AlarmNotifyPayload) error {
	return a.Service.Notify(ctx, Notification{
		CompanyID: p.CompanyID, AlarmID: p.AlarmID, EventID: p.EventID, AnalyzerID: p.AnalyzerID,
	})
}

// EnqueueEvaluate builds the enqueue function DispatchDeps needs.
func EnqueueEvaluate(enqueuer TaskEnqueuer, maxRetry int) func(context.Context, job.AlarmEvaluatePayload) error {
	return func(ctx context.Context, p job.AlarmEvaluatePayload) error {
		task, err := job.NewAlarmEvaluateTask(p, job.TaskOptions{MaxRetry: maxRetry})
		if err != nil {
			return err
		}
		_, err = enqueuer.Enqueue(ctx, task)
		return err
	}
}

// EnqueueNotify builds the enqueue function RunDeps needs.
func EnqueueNotify(enqueuer TaskEnqueuer, maxRetry int) func(context.Context, job.AlarmNotifyPayload) error {
	return func(ctx context.Context, p job.AlarmNotifyPayload) error {
		task, err := job.NewAlarmNotifyTask(p, job.TaskOptions{MaxRetry: maxRetry})
		if err != nil {
			return err
		}
		_, err = enqueuer.Enqueue(ctx, task)
		return err
	}
}
