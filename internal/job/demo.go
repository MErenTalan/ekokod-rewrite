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

// DemoExtender extends the demo readings; a missing demo company is a no-op.
type DemoExtender interface {
	ExtendDemo(ctx context.Context) error
}

func (h *Handlers) handleDemoExtend(ctx context.Context, _ *asynq.Task) error {
	return ClassifyForRetry(h.Demo.ExtendDemo(ctx))
}
