package job

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TypeForecastRun is F13b R376's scheduled forecast run.
const TypeForecastRun = "forecast.run"

// ForecastRunPayload carries nothing today: the run covers every company.
type ForecastRunPayload struct{}

// NewForecastRunTask builds forecast.run. No TaskID: a re-run adds a new run
// (generated_at is part of the key), which is what makes forecasts scorable.
func NewForecastRunTask(o TaskOptions) (*asynq.Task, error) {
	payload, err := integEncode(TypeForecastRun, ForecastRunPayload{})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeForecastRun, payload, append([]asynq.Option{asynq.Timeout(2 * time.Hour)}, integMaxRetryOptions(o)...)...), nil
}

// ForecastJobs serves forecast.run.
type ForecastJobs interface {
	RunTask(ctx context.Context) error
}

func (h *Handlers) handleForecastRun(ctx context.Context, task *asynq.Task) error {
	var p ForecastRunPayload
	if err := integDecode(TypeForecastRun, task.Payload(), &p); err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.Forecast.RunTask(ctx))
}
