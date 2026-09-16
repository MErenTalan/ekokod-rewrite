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
// ID (R71): the hour-floored From and hour-ceiled To, both UTC. Deliberately
// NEITHER a company nor an analyzer feeds the id — a refresh is platform
// work that covers every tenant's buckets in the window (R72), so a burst of
// refreshes requested by many analyzers, across many companies, all landing
// in the same hour-aligned window collapses to the one task asynq's own
// TaskID de-duplication already gives it for free, rather than piling up one
// task per analyzer or company for work that is identical once expanded.
func ConsumptionRefreshTaskID(from, to time.Time) string {
	return "consumption.refresh:" + hourFloor(from).UTC().Format(time.RFC3339) + "/" + hourCeil(to).UTC().Format(time.RFC3339)
}

// NewConsumptionRefreshTask builds a consumption.refresh task. Unlike the
// windowed shape of NewFetchReadingsTask, this NEVER sets asynq.Retention:
// the F2 predecessor's R53 trap came from exactly that combination — a
// deterministic id plus a long retention meant a completed task's id stayed
// "taken" for the retention period, silently swallowing a later, legitimate
// re-run of the same window. Here, TaskID only needs to deduplicate a BURST
// of concurrent requests for the same window while one is pending, active or
// retrying; the moment a refresh completes, the id must be free again so a
// later backfill of the same window enqueues cleanly.
func NewConsumptionRefreshTask(p ConsumptionRefreshPayload, o TaskOptions) (*asynq.Task, error) {
	if p.CompanyID == uuid.Nil {
		return nil, fmt.Errorf("consumption.refresh: CompanyID is required")
	}
	if p.From.IsZero() || p.To.IsZero() {
		return nil, fmt.Errorf("consumption.refresh: From and To are required")
	}
	if !p.From.Before(p.To) {
		return nil, fmt.Errorf("consumption.refresh: From must be before To")
	}

	payload, err := integEncode(TypeConsumptionRefresh, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{
		asynq.Timeout(30 * time.Minute),
	}, integMaxRetryOptions(o)...)
	opts = append(opts, asynq.TaskID(ConsumptionRefreshTaskID(p.From, p.To)))
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
