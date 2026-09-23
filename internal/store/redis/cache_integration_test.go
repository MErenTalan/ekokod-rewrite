//go:build integration

package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	ekoredis "github.com/MErenTalan/ekokod-rewrite/internal/store/redis"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestCacheRoundTripAndTTL(t *testing.T) {
	ctx := context.Background()
	client, err := ekoredis.New(ctx, testfixtures.SharedRedisConfig(t), testfixtures.DiscardLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	cache := ekoredis.NewCache(client, "test:"+uuid.NewString()+":")

	_, ok, err := cache.Get(ctx, "missing")
	require.NoError(t, err)
	require.False(t, ok, "a miss is not an error")

	require.NoError(t, cache.Set(ctx, "k", []byte("v"), time.Minute))
	got, ok, err := cache.Get(ctx, "k")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "v", string(got))
}
