//go:build integration

package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func TestNoopTaskRoundTrip(t *testing.T) {
	cfg := testfixtures.RedisConfig(t)
	log := testfixtures.DiscardLogger()

	server, mux, err := job.NewServer(cfg, config.Worker{Concurrency: 2, MaxRetries: 1, Timeout: time.Minute}, log, nil)
	require.NoError(t, err)

	handled := make(chan string, 1)
	mux.HandleFunc(job.TypeNoop, func(ctx context.Context, task *asynq.Task) error {
		payload, err := job.DecodeNoop(task)
		if err != nil {
			return err
		}
		handled <- payload.Message
		return nil
	})

	go func() { _ = server.Run(mux) }()
	t.Cleanup(server.Shutdown)

	client, err := job.NewClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	task, err := job.NewNoopTask("hello from the api")
	require.NoError(t, err)
	info, err := client.Enqueue(context.Background(), task)
	require.NoError(t, err)
	require.Equal(t, job.TypeNoop, info.Type)

	select {
	case got := <-handled:
		require.Equal(t, "hello from the api", got)
	case <-time.After(20 * time.Second):
		t.Fatal("worker did not execute the enqueued task")
	}
}

func TestQueueUsesTheConfiguredDatabase(t *testing.T) {
	cfg := testfixtures.RedisConfig(t)
	opt, err := job.RedisOpt(cfg)
	require.NoError(t, err)
	require.Equal(t, cfg.QueueDB, opt.DB, "the queue must not share the cache database")
}

// R192: /jobs/{id} can only answer for a finished task while asynq still keeps
// it, so the three jobs a screen starts must carry a retention window.
func TestWatchableTasksAreRetained(t *testing.T) {
	cfg := testfixtures.RedisConfig(t)
	client, err := job.NewClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	companyID, credentialID, analyzerID := uuid.New(), uuid.New(), uuid.New()
	refresh, err := job.NewRefreshAnalyzerTask(job.RefreshAnalyzerPayload{
		CompanyID: companyID, CredentialID: credentialID, AnalyzerID: analyzerID, Mode: job.RefreshModeHourly,
	}, job.TaskOptions{MaxRetry: 1})
	require.NoError(t, err)
	sync, err := job.NewSyncAnalyzersTask(job.SyncAnalyzersPayload{CompanyID: companyID, CredentialID: credentialID}, job.TaskOptions{MaxRetry: 1})
	require.NoError(t, err)
	backfill, err := job.NewBackfillTask(job.BackfillPayload{
		CompanyID: companyID, CredentialID: credentialID,
		From: time.Now().Add(-48 * time.Hour), To: time.Now(),
	}, job.TaskOptions{MaxRetry: 1})
	require.NoError(t, err)

	for _, task := range []*asynq.Task{refresh, sync, backfill} {
		info, eerr := client.Enqueue(context.Background(), task)
		require.NoError(t, eerr, task.Type())
		require.Equal(t, time.Hour, info.Retention, "%s must be retained for /jobs/{id}", task.Type())
	}
}
