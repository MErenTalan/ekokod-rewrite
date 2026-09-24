//go:build integration

package redis_test

import (
	"context"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	ekoredis "github.com/MErenTalan/ekokod-rewrite/internal/store/redis"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/stretchr/testify/require"
)

func TestNewConnectsAndSelectsTheCacheDB(t *testing.T) {
	ctx := context.Background()
	uri := testfixtures.StartRedis(t)

	client, err := ekoredis.New(ctx, config.Redis{URL: uri, CacheDB: 2, QueueDB: 1}, testfixtures.DiscardLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Ping(ctx).Err())
	require.Equal(t, 2, client.Options().DB)
}

func TestCheckReportsHealth(t *testing.T) {
	ctx := context.Background()
	uri := testfixtures.StartRedis(t)

	client, err := ekoredis.New(ctx, config.Redis{URL: uri, CacheDB: 0, QueueDB: 1}, testfixtures.DiscardLogger())
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
		testfixtures.DiscardLogger())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "s3cr3t", "the redis password must never appear in an error message")
}

// TestNewNeverLeaksThePasswordOnPingFailure covers the case where the URL
// parses but the server is unreachable: any dial or auth error must still
// never contain the configured password.
func TestNewNeverLeaksThePasswordOnPingFailure(t *testing.T) {
	_, err := ekoredis.New(context.Background(),
		config.Redis{URL: "redis://user:s3cr3tpassword@127.0.0.1:1/0", CacheDB: 0, QueueDB: 1},
		testfixtures.DiscardLogger())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "s3cr3tpassword", "the redis password must never appear in an error message")
}
