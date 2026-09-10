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
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

// redisWaitStrategy replaces the wait strategy that testcontainers' redis
// module installs by default. That module hardcodes a 10-second budget on the
// port check (modules/redis@v0.44.0/redis.go:72,
// `wait.ForListeningPort(redisPort).WithStartupTimeout(time.Second*10)`),
// whereas the postgres module leaves `wait.ForListeningPort` on the library
// default of 60s (modules/postgres@v0.44.0/wait_strategies.go:24). That
// asymmetry is the whole flake: each poll of the port check issues a
// `GET /containers/<id>/json` Docker API call, and under `go test ./... -race
// -count=3` the WSL2 Docker Desktop daemon serves those slowly enough that 93
// retries exhausted the 10s budget while redis had *already* logged
// "Ready to accept connections". The container was healthy; only the readiness
// probe's budget was too small. Restoring the library default 60s here aligns
// redis with postgres, which has never flaked under the same load.
//
// This must REPLACE rather than append: WithAdditionalWaitStrategy would leave
// the module's own 10-second strategy in place and still fail.
func redisWaitStrategy() testcontainers.CustomizeRequestOption {
	return testcontainers.WithWaitStrategy(
		wait.ForListeningPort("6379/tcp").WithStartupTimeout(60*time.Second),
		wait.ForLog("* Ready to accept connections"),
	)
}

func startRedis(t *testing.T) config.Redis {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7.4.11-alpine", redisWaitStrategy())
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
