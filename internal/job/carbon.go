package job

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TypeCarbonAccrual is R311's daily carbon accrual.
const TypeCarbonAccrual = "carbon.daily_accrual"

// CarbonAccrualPayload names the day to accrue; empty means yesterday (Istanbul).
type CarbonAccrualPayload struct {
	Day string `json:"day,omitempty"`
}

// NewCarbonAccrualTask builds carbon.daily_accrual. No TaskID: a re-run of
// the same day converges on the same rows (the partial unique index).
func NewCarbonAccrualTask(p CarbonAccrualPayload, o TaskOptions) (*asynq.Task, error) {
	payload, err := integEncode(TypeCarbonAccrual, p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeCarbonAccrual, payload, append([]asynq.Option{asynq.Timeout(30 * time.Minute)}, integMaxRetryOptions(o)...)...), nil
}

// CarbonJobs serves carbon.daily_accrual.
type CarbonJobs interface {
	AccrueTask(ctx context.Context, day string) error
}

func (h *Handlers) handleCarbonAccrual(ctx context.Context, task *asynq.Task) error {
	var p CarbonAccrualPayload
	if err := integDecode(TypeCarbonAccrual, task.Payload(), &p); err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.Carbon.AccrueTask(ctx, p.Day))
}
