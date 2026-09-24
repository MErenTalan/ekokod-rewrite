package job

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

type recordingForecast struct{ calls int }

func (r *recordingForecast) RunTask(context.Context) error {
	r.calls++
	return nil
}

func TestForecastRunTaskAndRoute(t *testing.T) {
	task, err := NewForecastRunTask(TaskOptions{})
	require.NoError(t, err)
	require.Equal(t, "forecast.run", task.Type())

	mux := asynq.NewServeMux()
	Register(mux, &Handlers{})
	_, pattern := mux.Handler(task)
	require.Empty(t, pattern, "no handler without the forecast service")

	rec := &recordingForecast{}
	mux = asynq.NewServeMux()
	Register(mux, &Handlers{Forecast: rec})
	require.NoError(t, mux.ProcessTask(context.Background(), task))
	require.Equal(t, 1, rec.calls)
}
