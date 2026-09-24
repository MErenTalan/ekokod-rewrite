package metrics_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/metrics"
)

func TestJobMiddlewareCountsEveryOutcome(t *testing.T) {
	for _, tc := range []struct {
		err     error
		outcome string
	}{{nil, "success"}, {errors.New("boom"), "failure"}, {fmt.Errorf("bad payload: %w", asynq.SkipRetry), "skipped"}} {
		h := metrics.JobMiddleware(asynq.HandlerFunc(func(context.Context, *asynq.Task) error { return tc.err }))
		require.ErrorIs(t, h.ProcessTask(context.Background(), asynq.NewTask("test.metrics", nil)), tc.err, "the error passes through")
	}
	expected := `
# HELP ekokod_job_runs_total Job executions by type and outcome (success, failure, skipped).
# TYPE ekokod_job_runs_total counter
ekokod_job_runs_total{outcome="failure",type="test.metrics"} 1
ekokod_job_runs_total{outcome="skipped",type="test.metrics"} 1
ekokod_job_runs_total{outcome="success",type="test.metrics"} 1
`
	require.NoError(t, testutil.GatherAndCompare(prometheusGatherer(), strings.NewReader(expected), "ekokod_job_runs_total"))
}

type fakeInspector map[string]*asynq.QueueInfo

func (f fakeInspector) GetQueueInfo(q string) (*asynq.QueueInfo, error) {
	if info, ok := f[q]; ok {
		return info, nil
	}
	return nil, errors.New("no such queue")
}

func TestQueueCollectorReportsEveryState(t *testing.T) {
	c := metrics.QueueCollector{Inspector: fakeInspector{"default": {Pending: 7, Active: 2, Scheduled: 1, Retry: 3, Archived: 4}},
		Queues: []string{"default", "missing"}, Log: slog.New(slog.DiscardHandler)}
	expected := `
# HELP ekokod_queue_tasks Tasks in a queue by state.
# TYPE ekokod_queue_tasks gauge
ekokod_queue_tasks{queue="default",state="active"} 2
ekokod_queue_tasks{queue="default",state="archived"} 4
ekokod_queue_tasks{queue="default",state="pending"} 7
ekokod_queue_tasks{queue="default",state="retry"} 3
ekokod_queue_tasks{queue="default",state="scheduled"} 1
`
	require.NoError(t, testutil.CollectAndCompare(c, strings.NewReader(expected)), "an unreadable queue is skipped, not fatal")
}

type fakeRuns []metrics.IntegrationRun

func (f fakeRuns) IntegrationRuns(context.Context) ([]metrics.IntegrationRun, error) { return f, nil }

func TestIntegrationCollectorReportsTheNewestPulls(t *testing.T) {
	ok, bad := time.Unix(1790000000, 0), time.Unix(1790003600, 0)
	c := metrics.IntegrationCollector{Runs: fakeRuns{{Provider: "osos", LastSuccess: &ok, LastError: &bad}, {Provider: "aril", LastSuccess: &ok}},
		Log: slog.New(slog.DiscardHandler)}
	expected := `
# HELP ekokod_integration_last_failure_timestamp_seconds The newest failed reading pull per provider (Unix seconds).
# TYPE ekokod_integration_last_failure_timestamp_seconds gauge
ekokod_integration_last_failure_timestamp_seconds{provider="osos"} 1.7900036e+09
# HELP ekokod_integration_last_success_timestamp_seconds The newest successful reading pull per provider (Unix seconds).
# TYPE ekokod_integration_last_success_timestamp_seconds gauge
ekokod_integration_last_success_timestamp_seconds{provider="aril"} 1.79e+09
ekokod_integration_last_success_timestamp_seconds{provider="osos"} 1.79e+09
`
	require.NoError(t, testutil.CollectAndCompare(c, strings.NewReader(expected)))
}
