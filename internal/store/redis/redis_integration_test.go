//go:build integration

package redis_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	ekoredis "github.com/MErenTalan/ekokod-rewrite/internal/store/redis"
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

func startRedis(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7.4.11-alpine", redisWaitStrategy())
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	uri, err := container.ConnectionString(ctx)
	require.NoError(t, err)
	return uri
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestNewConnectsAndSelectsTheCacheDB(t *testing.T) {
	ctx := context.Background()
	uri := startRedis(t)

	client, err := ekoredis.New(ctx, config.Redis{URL: uri, CacheDB: 2, QueueDB: 1}, discardLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Ping(ctx).Err())
	require.Equal(t, 2, client.Options().DB)
}

func TestCheckReportsHealth(t *testing.T) {
	ctx := context.Background()
	uri := startRedis(t)

	client, err := ekoredis.New(ctx, config.Redis{URL: uri, CacheDB: 0, QueueDB: 1}, discardLogger())
	require.NoError(t, err)

	check := ekoredis.Check(client)
	require.Equal(t, "redis", check.Name)
	require.NoError(t, check.Fn(ctx))

	require.NoError(t, client.Close())
	require.Error(t, check.Fn(ctx), "a closed client must report unhealthy")
}

func TestCheckOnNilClientReportsUnhealthy(t *testing.T) {
	check := ekoredis.Check(nil)
	require.Error(t, check.Fn(context.Background()))
}

// TestNewNeverLeaksThePasswordOnParseError guards against the same defect
// class already fixed twice in the postgres DSN path: net/url's parse error
// embeds its whole input, including the password, verbatim in its Error()
// text, and goredis.ParseURL forwards that error unchanged.
func TestNewNeverLeaksThePasswordOnParseError(t *testing.T) {
	_, err := ekoredis.New(context.Background(),
		config.Redis{URL: "redis://user:s3cr3t pass@localhost:6379/0", CacheDB: 0, QueueDB: 1},
		discardLogger())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "s3cr3t", "the redis password must never appear in an error message")
}

// TestNewNeverLeaksThePasswordOnPingFailure covers the case where the URL
// parses but the server is unreachable: any dial or auth error must still
// never contain the configured password.
func TestNewNeverLeaksThePasswordOnPingFailure(t *testing.T) {
	_, err := ekoredis.New(context.Background(),
		config.Redis{URL: "redis://user:s3cr3tpassword@127.0.0.1:1/0", CacheDB: 0, QueueDB: 1},
		discardLogger())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "s3cr3tpassword", "the redis password must never appear in an error message")
}
