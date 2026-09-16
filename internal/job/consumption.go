package job

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// hourFloor truncates t down to the start of its UTC hour.
func hourFloor(t time.Time) time.Time {
	return t.UTC().Truncate(time.Hour)
}

// hourCeil rounds t up to the start of the next UTC hour, unless t already
// sits exactly on an hour boundary, in which case it is returned unchanged.
func hourCeil(t time.Time) time.Time {
	floor := hourFloor(t)
	if floor.Equal(t.UTC()) {
		return floor
	}
	return floor.Add(time.Hour)
}

// ConsumptionRefreshTaskID computes consumption.refresh's deterministic task
// ID (R71, amended by R100(2)): the hour-floored From and hour-ceiled To,
// both UTC, PLUS now truncated down to the minute. Neither a company nor an
// analyzer feeds the id — a refresh is platform work that covers every
// tenant's buckets in the window (R72) — but unlike the original R71
// formula, `now` DOES feed it: final review A I-2 found that asynq's
// EXISTS check for a TaskID conflict matches a task that is pending,
// retrying, OR ACTIVE, so a window-only id could silently drop rows
// committed after an active task for the same window had already passed the
// view that would have covered them. Folding in a one-minute debounce slot
// means the id only ever collapses a BURST of enqueues arriving within the
// same wall-clock minute (still pending, never active — see
// NewConsumptionRefreshTask's ProcessIn(1m) below, which guarantees that): a
// later burst, arriving after the first task has actually started, computes
// a fresh id in the next minute and always gets its own task.
func ConsumptionRefreshTaskID(from, to, now time.Time) string {
	return "consumption.refresh:" + hourFloor(from).UTC().Format(time.RFC3339) + "/" + hourCeil(to).UTC().Format(time.RFC3339) +
		":" + now.UTC().Truncate(time.Minute).Format(time.RFC3339)
}

// NewConsumptionRefreshTask builds a consumption.refresh task. now is the
// enqueuing side's own clock reading, threaded through as an explicit
// parameter (rather than added to ConsumptionRefreshPayload) precisely so
// the wire format asynq persists and DecodeConsumptionRefresh reads back is
// unchanged — now feeds only the TaskID and the ProcessIn delay computed
// here, never anything the handler itself needs to see.
//
// Unlike the windowed shape of NewFetchReadingsTask, this NEVER sets
// asynq.Retention: the F2 predecessor's R53 trap came from exactly that
// combination — a deterministic id plus a long retention meant a completed
// task's id stayed "taken" for the retention period, silently swallowing a
// later, legitimate re-run of the same window.
//
// R100(2): the task is enqueued with asynq.ProcessIn(time.Minute), not
// immediately. Combined with the id's one-minute debounce slot, this
// guarantees every task built from this function sits PENDING (never
// active) for at least a minute before a worker can pick it up, which is
// exactly the window during which a same-minute burst must collapse onto
// it via asynq.ErrTaskIDConflict (mapped to success by the worker adapter,
// never this constructor).
func NewConsumptionRefreshTask(p ConsumptionRefreshPayload, now time.Time, o TaskOptions) (*asynq.Task, error) {
	if p.CompanyID == uuid.Nil {
		return nil, fmt.Errorf("consumption.refresh: CompanyID is required")
	}
	if p.From.IsZero() || p.To.IsZero() {
		return nil, fmt.Errorf("consumption.refresh: From and To are required")
	}
	if !p.From.Before(p.To) {
		return nil, fmt.Errorf("consumption.refresh: From must be before To")
	}
	if now.IsZero() {
		return nil, fmt.Errorf("consumption.refresh: now is required")
	}

	payload, err := integEncode(TypeConsumptionRefresh, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{
		asynq.Timeout(30 * time.Minute),
		asynq.ProcessIn(time.Minute),
	}, integMaxRetryOptions(o)...)
	opts = append(opts, asynq.TaskID(ConsumptionRefreshTaskID(p.From, p.To, now)))
	return asynq.NewTask(TypeConsumptionRefresh, payload, opts...), nil
}

// DecodeConsumptionRefresh reads a consumption.refresh payload.
func DecodeConsumptionRefresh(t *asynq.Task) (ConsumptionRefreshPayload, error) {
	var p ConsumptionRefreshPayload
	if err := integDecode(TypeConsumptionRefresh, t.Payload(), &p); err != nil {
		return ConsumptionRefreshPayload{}, err
	}
	return p, nil
}

// Refresher handles consumption.refresh. internal/service/consumption
// implements it; internal/job still imports nothing from internal/store —
// this interface is the seam that keeps that true (Task 11a).
type Refresher interface {
	RefreshConsumption(ctx context.Context, p ConsumptionRefreshPayload) error
}

// integHandleConsumptionRefresh decodes a consumption.refresh payload and
// adapts Handlers.ConsumptionRefresh.RefreshConsumption to asynq's handler
// signature, mirroring integHandleSyncPrices exactly.
func (h *Handlers) integHandleConsumptionRefresh(ctx context.Context, task *asynq.Task) error {
	p, err := DecodeConsumptionRefresh(task)
	if err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.ConsumptionRefresh.RefreshConsumption(ctx, p))
}
