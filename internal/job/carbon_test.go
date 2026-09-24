package job

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

type recordingCarbon struct{ days []string }

func (r *recordingCarbon) AccrueTask(_ context.Context, day string) error {
	r.days = append(r.days, day)
	return nil
}

func TestCarbonAccrualTaskAndRoute(t *testing.T) {
	task, err := NewCarbonAccrualTask(CarbonAccrualPayload{}, TaskOptions{})
	require.NoError(t, err)
	require.Equal(t, TypeCarbonAccrual, task.Type())
	require.Equal(t, "carbon.daily_accrual", TypeCarbonAccrual)

	mux := asynq.NewServeMux()
	Register(mux, &Handlers{})
	_, pattern := mux.Handler(task)
	require.Empty(t, pattern, "no handler without the carbon service")

	rec := &recordingCarbon{}
	mux = asynq.NewServeMux()
	Register(mux, &Handlers{Carbon: rec})
	require.NoError(t, mux.ProcessTask(context.Background(), task))
	day, err := NewCarbonAccrualTask(CarbonAccrualPayload{Day: "2026-09-01"}, TaskOptions{})
	require.NoError(t, err)
	require.NoError(t, mux.ProcessTask(context.Background(), day))
	require.Equal(t, []string{"", "2026-09-01"}, rec.days)
}
