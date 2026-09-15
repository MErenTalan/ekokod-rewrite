//go:build integration

package lock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func newRedisTestClients(t *testing.T) (*goredis.Client, *goredis.Client) {
	t.Helper()
	uri := testfixtures.StartRedis(t)

	opts, err := goredis.ParseURL(uri)
	require.NoError(t, err)

	a := goredis.NewClient(opts)
	t.Cleanup(func() { _ = a.Close() })
	b := goredis.NewClient(opts)
	t.Cleanup(func() { _ = b.Close() })

	require.NoError(t, a.Ping(context.Background()).Err())
	return a, b
}

func TestRedisLockExclusiveAcrossClients(t *testing.T) {
	clientA, clientB := newRedisTestClients(t)
	lockerA := lock.NewRedis(clientA)
	lockerB := lock.NewRedis(clientB)

	key := "TestRedisLockExclusiveAcrossClients"

	leaseA, err := lockerA.Acquire(context.Background(), key, time.Minute)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = lockerB.Acquire(ctx, key, time.Minute)
	require.Error(t, err)
	require.True(t, errors.Is(err, lock.ErrNotAcquired) || errors.Is(err, context.DeadlineExceeded),
		"expected ErrNotAcquired or a context error, got %v", err)

	require.NoError(t, leaseA.Release(context.Background()))

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	leaseB, err := lockerB.Acquire(ctx2, key, time.Minute)
	require.NoError(t, err, "B should acquire promptly once A releases")
	require.NoError(t, leaseB.Release(context.Background()))
}

func TestRedisReleaseDoesNotDeleteAnotherHoldersLock(t *testing.T) {
	clientA, clientB := newRedisTestClients(t)
	lockerA := lock.NewRedis(clientA)
	lockerB := lock.NewRedis(clientB)

	key := "TestRedisReleaseDoesNotDeleteAnotherHoldersLock"

	leaseA, err := lockerA.Acquire(context.Background(), key, 300*time.Millisecond)
	require.NoError(t, err)

	// Wait out A's TTL so B can acquire the now-expired key.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	leaseB, err := lockerB.Acquire(ctx, key, time.Minute)
	require.NoError(t, err, "B should acquire once A's TTL elapses")

	// A's stale Release must not delete B's key: the compare-and-delete
	// script only deletes when the stored token still matches A's.
	require.NoError(t, leaseA.Release(context.Background()))

	val, err := clientA.Get(context.Background(), "ekokod:lock:"+key).Result()
	require.NoError(t, err, "B's key must still be present after A's stale release")
	require.NotEmpty(t, val)

	require.NoError(t, leaseB.Release(context.Background()))
}

// TestRedisConsumeIsSingleUseAndNonBlocking proves SET NX is used for
// Consume, not a poll loop: a second Consume on an already-consumed key,
// called with context.Background() (no deadline at all), still returns
// found=false in well under a second.
func TestRedisConsumeIsSingleUseAndNonBlocking(t *testing.T) {
	client, _ := newRedisTestClients(t)
	locker := lock.NewRedis(client)

	key := "TestRedisConsumeIsSingleUseAndNonBlocking"

	found, err := locker.Consume(context.Background(), key, time.Minute)
	require.NoError(t, err)
	require.True(t, found)

	start := time.Now()
	found, err = locker.Consume(context.Background(), key, time.Minute)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.False(t, found)
	require.Less(t, elapsed, time.Second, "Consume must never poll/wait")
}
