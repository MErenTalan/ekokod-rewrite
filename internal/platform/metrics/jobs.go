package metrics

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	jobRuns = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ekokod_job_runs_total",
		Help: "Job executions by type and outcome (success, failure, skipped).",
	}, []string{"type", "outcome"})
	jobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ekokod_job_duration_seconds",
		Help:    "Job execution time by type.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300, 900},
	}, []string{"type"})
)

// JobMiddleware records every task's outcome and duration (R461). A SkipRetry
// error is "skipped": the task failed for a reason a retry cannot fix.
func JobMiddleware(next asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
		started := time.Now()
		err := next.ProcessTask(ctx, t)
		outcome := "success"
		switch {
		case errors.Is(err, asynq.SkipRetry):
			outcome = "skipped"
		case err != nil:
			outcome = "failure"
		}
		jobRuns.WithLabelValues(t.Type(), outcome).Inc()
		jobDuration.WithLabelValues(t.Type()).Observe(time.Since(started).Seconds())
		return err
	})
}

// QueueInspector is the part of asynq.Inspector the queue collector reads.
type QueueInspector interface {
	GetQueueInfo(queue string) (*asynq.QueueInfo, error)
}

var queueDesc = prometheus.NewDesc("ekokod_queue_tasks", "Tasks in a queue by state.", []string{"queue", "state"}, nil)

// QueueCollector reports queue depth on every scrape (R462).
type QueueCollector struct {
	Inspector QueueInspector
	Queues    []string
	Log       *slog.Logger
}

// Describe implements prometheus.Collector.
func (c QueueCollector) Describe(ch chan<- *prometheus.Desc) { ch <- queueDesc }

// Collect implements prometheus.Collector.
func (c QueueCollector) Collect(ch chan<- prometheus.Metric) {
	for _, q := range c.Queues {
		info, err := c.Inspector.GetQueueInfo(q)
		if err != nil {
			c.Log.Warn("queue metrics unavailable", slog.String("queue", q), slog.Any("error", err))
			continue
		}
		for state, n := range map[string]int{"pending": info.Pending, "active": info.Active, "scheduled": info.Scheduled,
			"retry": info.Retry, "archived": info.Archived} {
			ch <- prometheus.MustNewConstMetric(queueDesc, prometheus.GaugeValue, float64(n), q, state)
		}
	}
}

// IntegrationRun is a provider's newest successful and failed pull.
type IntegrationRun struct {
	Provider               string
	LastSuccess, LastError *time.Time
}

// IntegrationRuns reads them (admin.OpsHealthRepository).
type IntegrationRuns interface {
	IntegrationRuns(ctx context.Context) ([]IntegrationRun, error)
}

var (
	integrationSuccessDesc = prometheus.NewDesc("ekokod_integration_last_success_timestamp_seconds",
		"The newest successful reading pull per provider (Unix seconds).", []string{"provider"}, nil)
	integrationFailureDesc = prometheus.NewDesc("ekokod_integration_last_failure_timestamp_seconds",
		"The newest failed reading pull per provider (Unix seconds).", []string{"provider"}, nil)
)

// IntegrationCollector reports integration health on every scrape (R463).
type IntegrationCollector struct {
	Runs IntegrationRuns
	Log  *slog.Logger
}

// Describe implements prometheus.Collector.
func (c IntegrationCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- integrationSuccessDesc
	ch <- integrationFailureDesc
}

// Collect implements prometheus.Collector.
func (c IntegrationCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runs, err := c.Runs.IntegrationRuns(ctx)
	if err != nil {
		c.Log.Warn("integration metrics unavailable", slog.Any("error", err))
		return
	}
	for _, r := range runs {
		if r.LastSuccess != nil {
			ch <- prometheus.MustNewConstMetric(integrationSuccessDesc, prometheus.GaugeValue, float64(r.LastSuccess.Unix()), r.Provider)
		}
		if r.LastError != nil {
			ch <- prometheus.MustNewConstMetric(integrationFailureDesc, prometheus.GaugeValue, float64(r.LastError.Unix()), r.Provider)
		}
	}
}
