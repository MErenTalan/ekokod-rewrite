//go:build integration

package lock_test

import (
	"context"
	"errors"
	"strings"
	"sync"
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

// TestRedisAcquireRejectsNonPositiveTTL and TestRedisConsumeRejectsNonPositiveTTL
// prove M1: without the ttl<=0 guard, SetNX(...,0) sets NO expiry (Redis
// treats PX 0 as "no TTL"), holding the lock/consume mark forever instead
// of returning an error.
func TestRedisAcquireRejectsNonPositiveTTL(t *testing.T) {
	client, _ := newRedisTestClients(t)
	locker := lock.NewRedis(client)

	_, err := locker.Acquire(context.Background(), "TestRedisAcquireRejectsNonPositiveTTL", 0)
	require.ErrorIs(t, err, lock.ErrInvalidTTL)

	_, err = locker.Acquire(context.Background(), "TestRedisAcquireRejectsNonPositiveTTL", -time.Second)
	require.ErrorIs(t, err, lock.ErrInvalidTTL)
}

func TestRedisConsumeRejectsNonPositiveTTL(t *testing.T) {
	client, _ := newRedisTestClients(t)
	locker := lock.NewRedis(client)

	_, err := locker.Consume(context.Background(), "TestRedisConsumeRejectsNonPositiveTTL", 0)
	require.ErrorIs(t, err, lock.ErrInvalidTTL)

	_, err = locker.Consume(context.Background(), "TestRedisConsumeRejectsNonPositiveTTL", -time.Second)
	require.ErrorIs(t, err, lock.ErrInvalidTTL)
}

// TestRedisConsumeDoesNotBlockAcquire proves M2: Consume and Acquire use
// separate Redis key namespaces (ekokod:consume: vs ekokod:lock:), so
// marking a key consumed must never make Acquire on the SAME key string
// block.
func TestRedisConsumeDoesNotBlockAcquire(t *testing.T) {
	client, _ := newRedisTestClients(t)
	locker := lock.NewRedis(client)

	key := "TestRedisConsumeDoesNotBlockAcquire"

	found, err := locker.Consume(context.Background(), key, time.Minute)
	require.NoError(t, err)
	require.True(t, found)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	lease, err := locker.Acquire(ctx, key, time.Minute)
	require.NoError(t, err, "Consume(key) must not block Acquire(key)")
	require.NoError(t, lease.Release(context.Background()))
}

// commandRecordingHook records the top-level Redis commands OUR go-redis
// client issues over the wire (ProcessHook wraps only calls the client
// itself makes; it never sees a Lua script's internal redis.call()
// invocations, which the server executes without going back through the
// client's Process path). This is why it is the right instrument for I5:
// `INFO commandstats` was tried first and rejected — Redis's own
// commandstats counts a script's internal redis.call("GET",...) and
// redis.call("DEL",...) under cmdstat_get/cmdstat_del too, so EVEN THE
// CORRECT compare-and-delete script (which necessarily calls GET and DEL
// *inside* Lua) moves those counters, making that signal indistinguishable
// from a client-side GET-then-DEL. The hook sees only what the Go client
// actually sent, so EVAL/EVALSHA vs. separate top-level GET/DEL commands is
// unambiguous.
type commandRecordingHook struct {
	mu       sync.Mutex
	commands []string
}

func (h *commandRecordingHook) reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.commands = nil
}

func (h *commandRecordingHook) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.commands...)
}

func (h *commandRecordingHook) record(cmd goredis.Cmder) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.commands = append(h.commands, strings.ToUpper(cmd.Name()))
}

func (h *commandRecordingHook) DialHook(next goredis.DialHook) goredis.DialHook {
	return next
}

func (h *commandRecordingHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		h.record(cmd)
		return next(ctx, cmd)
	}
}

func (h *commandRecordingHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		for _, cmd := range cmds {
			h.record(cmd)
		}
		return next(ctx, cmds)
	}
}

// TestRedisReleaseIsAtomic proves I5: Release must execute as a single
// server-side script (redisReleaseScript, run via EVAL or EVALSHA), never
// as a client-side GET, compare, then DEL — the latter has a window
// between the client's GET and DEL where another process's Acquire on the
// just-expired key and this stale Release's DEL can race, deleting the new
// holder's key. A client-side GET+compare+DEL implementation of Release
// would still pass TestRedisReleaseDoesNotDeleteAnotherHoldersLock (that
// test only checks the outcome after both calls settle, not how Release
// got there), so this test inspects the actual commands the client sends
// via commandRecordingHook: Release must send EVAL/EVALSHA, and must NOT
// send a top-level GET or DEL.
func TestRedisReleaseIsAtomic(t *testing.T) {
	client, _ := newRedisTestClients(t)
	hook := &commandRecordingHook{}
	client.AddHook(hook)
	locker := lock.NewRedis(client)

	key := "TestRedisReleaseIsAtomic"
	lease, err := locker.Acquire(context.Background(), key, time.Minute)
	require.NoError(t, err)

	hook.reset() // drop the SETNX Acquire issued; only Release's commands matter
	require.NoError(t, lease.Release(context.Background()))

	cmds := hook.snapshot()
	require.True(t,
		containsAny(cmds, "EVAL", "EVALSHA"),
		"Release must execute via a server-side script (EVAL/EVALSHA), got %v", cmds)
	require.NotContains(t, cmds, "GET", "Release must not send a client-side GET, got %v", cmds)
	require.NotContains(t, cmds, "DEL", "Release must not send a client-side DEL, got %v", cmds)
}

func containsAny(haystack []string, needles ...string) bool {
	for _, h := range haystack {
		for _, n := range needles {
			if h == n {
				return true
			}
		}
	}
	return false
}
