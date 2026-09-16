//go:build integration

package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// consumptionIntegrationWindow is a one-hour window every test in this file
// builds its ConsumptionRefreshPayload fixtures from.
func consumptionIntegrationWindow() (time.Time, time.Time) {
	from := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	return from, from.Add(time.Hour)
}

// TestConsumptionRefreshTaskIDConflictsAcrossAnalyzersWhilePending is the
// end-to-end proof that ConsumptionRefreshTaskID's window-only formula
// actually collapses a burst of different analyzers'/companies' refreshes
// for the SAME window into ONE pending task: no server ever runs here, so
// the first enqueued task stays pending, and asynq itself must reject the
// second enqueue of the SAME id with asynq.ErrTaskIDConflict (R71/R72).
func TestConsumptionRefreshTaskIDConflictsAcrossAnalyzersWhilePending(t *testing.T) {
	cfg := testfixtures.RedisConfig(t)
	client, err := job.NewClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	from, to := consumptionIntegrationWindow()

	task1, err := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{
		CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to,
	}, job.TaskOptions{})
	require.NoError(t, err)
	_, err = client.Enqueue(context.Background(), task1)
	require.NoError(t, err)

	// A DIFFERENT company and analyzer, but the SAME window: the id must
	// still collide, because AnalyzerID/CompanyID never feed it.
	task2, err := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{
		CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to,
	}, job.TaskOptions{})
	require.NoError(t, err)
	_, err = client.Enqueue(context.Background(), task2)
	require.Error(t, err)
	require.ErrorIs(t, err, asynq.ErrTaskIDConflict,
		"a second refresh for the same window, from a different analyzer/company, must collide with the first while it is pending")
}

// TestConsumptionRefreshTaskIDIsFreeAgainAfterCompletion proves
// NewConsumptionRefreshTask sets NO asynq.Retention: once the first task for
// a window COMPLETES, a later refresh of the exact same window must be able
// to enqueue again, cleanly. F2's R53 trap (a deterministic id plus a long
// retention keeping the id "taken" long after completion) must not recur
// here.
func TestConsumptionRefreshTaskIDIsFreeAgainAfterCompletion(t *testing.T) {
	cfg := testfixtures.RedisConfig(t)
	log := testfixtures.DiscardLogger()

	server, mux, err := job.NewServer(cfg, config.Worker{Concurrency: 2, MaxRetries: 1, Timeout: time.Minute}, log, nil)
	require.NoError(t, err)

	done := make(chan struct{}, 1)
	mux.HandleFunc(job.TypeConsumptionRefresh, func(context.Context, *asynq.Task) error {
		done <- struct{}{}
		return nil
	})

	go func() { _ = server.Run(mux) }()
	t.Cleanup(server.Shutdown)

	client, err := job.NewClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	from, to := consumptionIntegrationWindow()
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to}

	task1, err := job.NewConsumptionRefreshTask(payload, job.TaskOptions{})
	require.NoError(t, err)
	_, err = client.Enqueue(context.Background(), task1)
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("worker did not execute the first enqueued consumption.refresh task")
	}

	// asynq marks a task's completion asynchronously after the handler
	// returns; give it a moment to actually record completion before
	// re-enqueuing the same id, rather than racing the retry/removal step.
	require.Eventually(t, func() bool {
		task2, err := job.NewConsumptionRefreshTask(payload, job.TaskOptions{})
		if err != nil {
			return false
		}
		_, enqueueErr := client.Enqueue(context.Background(), task2)
		return enqueueErr == nil
	}, 10*time.Second, 100*time.Millisecond,
		"the SAME window's task id must become enqueueable again once the first run completed — no Retention must be set")
}
