package job

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TypeDemoExtend keeps the synthetic demo company's readings current (R186).
const TypeDemoExtend = "demo.extend"

// NewDemoExtendTask builds the scheduled tick.
func NewDemoExtendTask(o TaskOptions) (*asynq.Task, error) {
	opts := append([]asynq.Option{asynq.Timeout(10 * time.Minute), asynq.Unique(30 * time.Minute)}, integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeDemoExtend, nil, opts...), nil
}

// TypeDemoReset returns the demo company to its baseline (R553).
const TypeDemoReset = "demo.reset"

// NewDemoResetTask builds the scheduled tick. No retries: a failed reset is
// replaced by the next hour's, and a retry could land on a visitor mid-tour.
func NewDemoResetTask(_ TaskOptions) (*asynq.Task, error) {
	opts := []asynq.Option{asynq.Timeout(15 * time.Minute), asynq.Unique(50 * time.Minute), asynq.MaxRetry(0)}
	return asynq.NewTask(TypeDemoReset, nil, opts...), nil
}

// DemoExtender extends the demo readings; a missing demo company is a no-op.
type DemoExtender interface {
	ExtendDemo(ctx context.Context) error
}

// DemoResetter returns the demo company to its baseline; a missing demo
// company or a reset already running is a no-op.
type DemoResetter interface {
	ResetDemo(ctx context.Context) error
}

func (h *Handlers) handleDemoReset(ctx context.Context, _ *asynq.Task) error {
	return h.DemoReset.ResetDemo(ctx)
}

func (h *Handlers) handleDemoExtend(ctx context.Context, _ *asynq.Task) error {
	return ClassifyForRetry(h.Demo.ExtendDemo(ctx))
}
