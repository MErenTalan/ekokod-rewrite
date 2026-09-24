package job_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
)

func TestAlarmEvaluateTaskIDIsOnePerCompanyPerHour(t *testing.T) {
	t.Parallel()
	// R225: a manual trigger inside the cron tick's own hour is de-duplicated
	// by asynq rather than evaluating and notifying twice.
	company := uuid.New()
	hour := time.Date(2026, 9, 18, 14, 0, 0, 0, time.UTC)

	a, err := job.NewAlarmEvaluateTask(job.AlarmEvaluatePayload{CompanyID: company, Hour: hour}, job.TaskOptions{})
	require.NoError(t, err)
	b, err := job.NewAlarmEvaluateTask(job.AlarmEvaluatePayload{CompanyID: company, Hour: hour.Add(37 * time.Minute)}, job.TaskOptions{})
	require.NoError(t, err)
	require.Equal(t,
		job.AlarmEvaluateTaskID(job.AlarmEvaluatePayload{CompanyID: company, Hour: hour}),
		job.AlarmEvaluateTaskID(job.AlarmEvaluatePayload{CompanyID: company, Hour: hour.Add(37 * time.Minute)}),
		"the same hour is the same task whatever minute names it")
	require.Equal(t, a.Type(), b.Type())

	next := job.AlarmEvaluateTaskID(job.AlarmEvaluatePayload{CompanyID: company, Hour: hour.Add(time.Hour)})
	require.NotEqual(t, job.AlarmEvaluateTaskID(job.AlarmEvaluatePayload{CompanyID: company, Hour: hour}), next)

	other := job.AlarmEvaluateTaskID(job.AlarmEvaluatePayload{CompanyID: uuid.New(), Hour: hour})
	require.NotEqual(t, job.AlarmEvaluateTaskID(job.AlarmEvaluatePayload{CompanyID: company, Hour: hour}), other,
		"one company's tick must never suppress another's")
}

func TestAlarmNotifyTaskIDIsOncePerEvent(t *testing.T) {
	t.Parallel()
	event := uuid.New()
	p := job.AlarmNotifyPayload{CompanyID: uuid.New(), AlarmID: uuid.New(), EventID: event}
	q := job.AlarmNotifyPayload{CompanyID: uuid.New(), AlarmID: uuid.New(), EventID: event}
	require.Equal(t, job.AlarmNotifyTaskID(p), job.AlarmNotifyTaskID(q))
	require.NotEqual(t, job.AlarmNotifyTaskID(p),
		job.AlarmNotifyTaskID(job.AlarmNotifyPayload{EventID: uuid.New()}))
}

func TestNewAlarmTasksRejectEmptyPayloads(t *testing.T) {
	t.Parallel()
	_, err := job.NewAlarmEvaluateTask(job.AlarmEvaluatePayload{}, job.TaskOptions{})
	require.Error(t, err, "a company is required")
	_, err = job.NewAlarmEvaluateTask(job.AlarmEvaluatePayload{CompanyID: uuid.New()}, job.TaskOptions{})
	require.Error(t, err, "an hour is required")
	_, err = job.NewAlarmNotifyTask(job.AlarmNotifyPayload{CompanyID: uuid.New()}, job.TaskOptions{})
	require.Error(t, err, "an alarm and an event are required")
	_, err = job.NewAlarmNotifyTask(job.AlarmNotifyPayload{
		CompanyID: uuid.New(), AlarmID: uuid.New(), EventID: uuid.New()}, job.TaskOptions{})
	require.NoError(t, err)
}
