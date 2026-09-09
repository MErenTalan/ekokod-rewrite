//go:build integration

package job_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func startRedis(t *testing.T) config.Redis {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7.4.11-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	uri, err := container.ConnectionString(ctx)
	require.NoError(t, err)
	return config.Redis{URL: uri, CacheDB: 0, QueueDB: 1}
}

func TestNoopTaskRoundTrip(t *testing.T) {
	cfg := startRedis(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

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
	cfg := startRedis(t)
	opt, err := job.RedisOpt(cfg)
	require.NoError(t, err)
	require.Equal(t, cfg.QueueDB, opt.DB, "the queue must not share the cache database")
}
