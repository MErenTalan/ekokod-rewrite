package lock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
)

func TestMemoryLockIsExclusiveUntilReleased(t *testing.T) {
	m := lock.NewMemory(nil)

	lease, err := m.Acquire(context.Background(), "k", time.Minute)
	require.NoError(t, err)

	// A second Acquire on the same key must not succeed while the first is
	// held: race it against a short deadline and expect ErrNotAcquired.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = m.Acquire(ctx, "k", time.Minute)
	require.Error(t, err)
	require.ErrorIs(t, err, lock.ErrNotAcquired)

	require.NoError(t, lease.Release(context.Background()))

	// Released: a fresh Acquire on the same key must now succeed promptly.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	lease2, err := m.Acquire(ctx2, "k", time.Minute)
	require.NoError(t, err)
	require.NoError(t, lease2.Release(context.Background()))
}

func TestMemoryLockExpiresAfterTTL(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	m := lock.NewMemory(clock.Now)

	_, err := m.Acquire(context.Background(), "k", time.Second)
	require.NoError(t, err)

	// Still within the TTL: a second Acquire must not succeed.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = m.Acquire(ctx, "k", time.Second)
	require.ErrorIs(t, err, lock.ErrNotAcquired)

	// Advance the injected clock past the TTL: the key is now free.
	clock.advance(2 * time.Second)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	lease2, err := m.Acquire(ctx2, "k", time.Second)
	require.NoError(t, err)
	require.NoError(t, lease2.Release(context.Background()))
}

func TestAcquireRespectsContext(t *testing.T) {
	m := lock.NewMemory(nil)

	_, err := m.Acquire(context.Background(), "k", time.Minute)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = m.Acquire(ctx, "k", time.Minute)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.True(t, errors.Is(err, lock.ErrNotAcquired) || errors.Is(err, context.DeadlineExceeded))
	require.Less(t, elapsed, 500*time.Millisecond, "Acquire must return promptly once ctx is done, not hang")
}

func TestMemoryConsumeIsSingleUse(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	m := lock.NewMemory(clock.Now)

	found, err := m.Consume(context.Background(), "nonce", time.Second)
	require.NoError(t, err)
	require.True(t, found, "first consume must be the one that marks the key")

	found, err = m.Consume(context.Background(), "nonce", time.Second)
	require.NoError(t, err)
	require.False(t, found, "a replay before ttl expires must report found=false")

	clock.advance(2 * time.Second)

	found, err = m.Consume(context.Background(), "nonce", time.Second)
	require.NoError(t, err)
	require.True(t, found, "after ttl elapses the key is consumable again")
}

// TestMemoryConsumeNeverBlocks proves Consume is a single atomic
// lookup-and-set, not a poll loop shaped like Acquire's: a second Consume on
// an unexpired key must return within a few milliseconds, never waiting for
// ctx or ttl the way a blocked Acquire would.
func TestMemoryConsumeNeverBlocks(t *testing.T) {
	m := lock.NewMemory(nil)

	_, err := m.Consume(context.Background(), "nonce", time.Hour)
	require.NoError(t, err)

	start := time.Now()
	found, err := m.Consume(context.Background(), "nonce", time.Hour)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.False(t, found)
	require.Less(t, elapsed, 50*time.Millisecond, "Consume must never wait/poll")
}

// TestMemoryReleaseIsOwnerOnly proves Release only clears a key it still
// owns: a lease whose TTL already elapsed (and was reacquired by someone
// else) must not delete the new holder's entry when it releases late.
func TestMemoryReleaseIsOwnerOnly(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	m := lock.NewMemory(clock.Now)

	staleLease, err := m.Acquire(context.Background(), "k", time.Second)
	require.NoError(t, err)

	clock.advance(2 * time.Second) // staleLease's TTL has now elapsed

	newLease, err := m.Acquire(context.Background(), "k", time.Minute)
	require.NoError(t, err, "the key is free once the stale lease's ttl elapses")

	// The stale holder releasing late must not delete the new holder's key.
	require.NoError(t, staleLease.Release(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = m.Acquire(ctx, "k", time.Minute)
	require.ErrorIs(t, err, lock.ErrNotAcquired, "the new holder's lease must still be held after the stale release")

	require.NoError(t, newLease.Release(context.Background()))
}

// TestMemoryAcquireRejectsNonPositiveTTL proves M1: a zero or negative ttl
// is rejected up front rather than silently held forever (a zero ttl would
// have no expiry to fall back on the way Redis's SET...PX 0 does not
// expire).
func TestMemoryAcquireRejectsNonPositiveTTL(t *testing.T) {
	m := lock.NewMemory(nil)

	_, err := m.Acquire(context.Background(), "k", 0)
	require.ErrorIs(t, err, lock.ErrInvalidTTL)

	_, err = m.Acquire(context.Background(), "k", -time.Second)
	require.ErrorIs(t, err, lock.ErrInvalidTTL)
}

// TestMemoryConsumeRejectsNonPositiveTTL is TestMemoryAcquireRejectsNonPositiveTTL's
// Consume counterpart (M1).
func TestMemoryConsumeRejectsNonPositiveTTL(t *testing.T) {
	m := lock.NewMemory(nil)

	_, err := m.Consume(context.Background(), "nonce", 0)
	require.ErrorIs(t, err, lock.ErrInvalidTTL)

	_, err = m.Consume(context.Background(), "nonce", -time.Second)
	require.ErrorIs(t, err, lock.ErrInvalidTTL)
}

// TestMemoryConsumeDoesNotBlockAcquire proves M2's namespace-separation
// requirement holds for Memory (it already does — held and consumed are
// separate maps — this is the regression test; Redis needed the actual fix,
// see TestRedisConsumeDoesNotBlockAcquire).
func TestMemoryConsumeDoesNotBlockAcquire(t *testing.T) {
	m := lock.NewMemory(nil)

	found, err := m.Consume(context.Background(), "x", time.Minute)
	require.NoError(t, err)
	require.True(t, found)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	lease, err := m.Acquire(ctx, "x", time.Minute)
	require.NoError(t, err, "Consume(\"x\") must not block Acquire(\"x\")")
	require.NoError(t, lease.Release(context.Background()))
}

// fakeClock is a minimal, single-goroutine-use clock: every test above uses
// it sequentially (Acquire/Consume calls happen in the test goroutine, with
// no concurrent advance), so it needs no locking.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
}
