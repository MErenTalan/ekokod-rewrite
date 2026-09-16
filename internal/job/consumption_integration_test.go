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
// end-to-end proof that ConsumptionRefreshTaskID's window+debounce-minute
// formula (R100(2)) actually collapses a burst of different analyzers'/
// companies' refreshes for the SAME window, enqueued within the SAME
// wall-clock minute, into ONE pending task: no server ever runs here, so
// the first enqueued task stays pending (also helped by
// NewConsumptionRefreshTask's own ProcessIn(1m)), and asynq itself must
// reject the second enqueue of the SAME id with asynq.ErrTaskIDConflict
// (R71/R72/R100).
func TestConsumptionRefreshTaskIDConflictsAcrossAnalyzersWhilePending(t *testing.T) {
	cfg := testfixtures.RedisConfig(t)
	client, err := job.NewClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	from, to := consumptionIntegrationWindow()
	now := time.Now()

	task1, err := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{
		CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to,
	}, now, job.TaskOptions{})
	require.NoError(t, err)
	_, err = client.Enqueue(context.Background(), task1)
	require.NoError(t, err)

	// A DIFFERENT company and analyzer, but the SAME window and the SAME
	// debounce minute: the id must still collide, because
	// AnalyzerID/CompanyID never feed it.
	task2, err := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{
		CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to,
	}, now, job.TaskOptions{})
	require.NoError(t, err)
	_, err = client.Enqueue(context.Background(), task2)
	require.Error(t, err)
	require.ErrorIs(t, err, asynq.ErrTaskIDConflict,
		"a second refresh for the same window and debounce minute, from a different analyzer/company, must collide with the first while it is pending")
}

// TestConsumptionRefreshTaskIDDiffersACrossDebounceMinutes proves the other
// half of R100(2): a later enqueue for the SAME window, but computed from a
// `now` a minute after the first, gets its OWN id and does not collide —
// the debounce only ever collapses a burst inside one wall-clock minute, it
// must never suppress a legitimately later refresh of the same window.
func TestConsumptionRefreshTaskIDDiffersAcrossDebounceMinutes(t *testing.T) {
	cfg := testfixtures.RedisConfig(t)
	client, err := job.NewClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	from, to := consumptionIntegrationWindow()
	now := time.Now()

	task1, err := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{
		CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to,
	}, now, job.TaskOptions{})
	require.NoError(t, err)
	_, err = client.Enqueue(context.Background(), task1)
	require.NoError(t, err)

	task2, err := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{
		CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to,
	}, now.Add(time.Minute), job.TaskOptions{})
	require.NoError(t, err)
	_, err = client.Enqueue(context.Background(), task2)
	require.NoError(t, err, "a refresh for the same window a debounce minute later must get its own task id")
}

// TestNewConsumptionRefreshTaskDelaysProcessingByOneMinute is R100(2)'s
// direct proof that NewConsumptionRefreshTask actually applies
// asynq.ProcessIn(time.Minute): a task read back immediately after
// enqueueing (no server ever runs here) must be in asynq's "scheduled"
// state — never "pending" — with NextProcessAt roughly a minute out. A
// mutant that drops the ProcessIn option would leave the task "pending"
// with NextProcessAt at (or before) enqueue time, which this asserts
// against.
func TestNewConsumptionRefreshTaskDelaysProcessingByOneMinute(t *testing.T) {
	cfg := testfixtures.RedisConfig(t)
	client, err := job.NewClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	from, to := consumptionIntegrationWindow()
	enqueuedAt := time.Now()

	task, err := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{
		CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to,
	}, enqueuedAt, job.TaskOptions{})
	require.NoError(t, err)

	info, err := client.Enqueue(context.Background(), task)
	require.NoError(t, err)

	redisOpt, err := job.RedisOpt(cfg)
	require.NoError(t, err)
	insp := asynq.NewInspector(redisOpt)
	t.Cleanup(func() { _ = insp.Close() })

	taskInfo, err := insp.GetTaskInfo(info.Queue, info.ID)
	require.NoError(t, err)
	require.Equal(t, asynq.TaskStateScheduled, taskInfo.State,
		"a task built by NewConsumptionRefreshTask must be scheduled, never immediately pending")
	require.WithinDuration(t, enqueuedAt.Add(time.Minute), taskInfo.NextProcessAt, 10*time.Second,
		"NextProcessAt must sit roughly one minute after enqueue")
}

// TestNewConsumptionRefreshTaskAppliesConfiguredMaxRetry proves
// NewConsumptionRefreshTask actually applies integMaxRetryOptions(o) to the
// built task: asynq.Task exposes no accessor for its options outside a real
// broker round trip (see TestConsumptionRefreshTaskIDIgnoresAnalyzerAndCompany
// above), so this enqueues for real and reads the persisted MaxRetry back
// via asynq.Inspector. The mutation that
// drops integMaxRetryOptions(o)... from the option slice would silently fall
// back to asynq's default MaxRetry (25), which this asserts against.
func TestNewConsumptionRefreshTaskAppliesConfiguredMaxRetry(t *testing.T) {
	cfg := testfixtures.RedisConfig(t)
	client, err := job.NewClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	from, to := consumptionIntegrationWindow()
	const wantMaxRetry = 7 // deliberately far from asynq's default of 25

	task, err := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{
		CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to,
	}, time.Now(), job.TaskOptions{MaxRetry: wantMaxRetry})
	require.NoError(t, err)

	info, err := client.Enqueue(context.Background(), task)
	require.NoError(t, err)

	redisOpt, err := job.RedisOpt(cfg)
	require.NoError(t, err)
	insp := asynq.NewInspector(redisOpt)
	t.Cleanup(func() { _ = insp.Close() })

	taskInfo, err := insp.GetTaskInfo(info.Queue, info.ID)
	require.NoError(t, err)
	require.Equal(t, wantMaxRetry, taskInfo.MaxRetry,
		"NewConsumptionRefreshTask must apply the configured TaskOptions.MaxRetry to the built task")
}

// TestConsumptionRefreshTaskIDIsFreeAgainAfterCompletion proves
// NewConsumptionRefreshTask sets NO asynq.Retention: once the first task for
// a window+debounce-minute COMPLETES, a later refresh built from that EXACT
// SAME (payload, now) pair — reproducing the identical id — must be able to
// enqueue again, cleanly. F2's R53 trap (a deterministic id plus a long
// retention keeping the id "taken" long after completion) must not recur
// here.
//
// NextProcessAt itself is already proven by
// TestNewConsumptionRefreshTaskDelaysProcessingByOneMinute (no-server,
// GetTaskInfo, zero wait), so this test's own job is only the
// completion-then-re-enqueue half: it calls asynq.Inspector.RunTask right
// after enqueueing, which moves task1 from scheduled straight to pending so
// the running server executes it immediately — proving the same "no
// Retention" fact without spending the debounce minute idle.
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
	now := time.Now()

	task1, err := job.NewConsumptionRefreshTask(payload, now, job.TaskOptions{})
	require.NoError(t, err)
	info, err := client.Enqueue(context.Background(), task1)
	require.NoError(t, err)

	redisOpt, err := job.RedisOpt(cfg)
	require.NoError(t, err)
	insp := asynq.NewInspector(redisOpt)
	t.Cleanup(func() { _ = insp.Close() })

	// Skip the debounce minute: move task1 straight from scheduled to
	// pending so the running server picks it up at once, instead of
	// waiting out its ProcessIn(1m).
	require.NoError(t, insp.RunTask(info.Queue, info.ID))

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not execute the first enqueued consumption.refresh task after Inspector.RunTask")
	}

	// asynq marks a task's completion asynchronously after the handler
	// returns; give it a moment to actually record completion before
	// re-enqueuing the same id, rather than racing the retry/removal step.
	// task2 is built from the EXACT SAME (payload, now) pair as task1, so it
	// reproduces the identical id — a fresh time.Now() here would already
	// differ by more than a minute and trivially get its own id, which
	// would prove nothing about Retention.
	require.Eventually(t, func() bool {
		task2, err := job.NewConsumptionRefreshTask(payload, now, job.TaskOptions{})
		if err != nil {
			return false
		}
		_, enqueueErr := client.Enqueue(context.Background(), task2)
		return enqueueErr == nil
	}, 10*time.Second, 200*time.Millisecond,
		"the SAME window+debounce-minute task id must become enqueueable again once the first run completed — no Retention must be set")
}
