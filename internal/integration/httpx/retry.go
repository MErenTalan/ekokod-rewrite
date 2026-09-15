package httpx

import (
	"context"
	"math/rand/v2"
	"time"
)

// sleepFunc is the shape of PoolOptions.Sleep and of defaultSleep below: it
// waits for d, honouring ctx cancellation, and returns ctx.Err() if ctx is
// done first. Do's retry loop treats a non-nil return as "stop retrying
// now" — the mechanism that makes context cancellation stop retries
// promptly instead of sleeping out a full backoff after the caller has
// already given up.
type sleepFunc func(ctx context.Context, d time.Duration) error

// defaultSleep is the real-timer implementation PoolOptions.Sleep falls
// back to when nil.
func defaultSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// backoffForAttempt returns a jittered delay for the retry that follows the
// attempt-th failed try (attempt=1: the delay before the 2nd request).
// backoff doubles from base each attempt, capped at maxBackoff, then equal
// jitter draws uniformly from [backoff/2, backoff] via rand.Int64N. With
// the plan's defaults (base=1s), attempt=1 gives a delay in [0.5s,1s] and
// attempt=2 gives a delay in [1s,2s] — the exact windows
// TestRetriesUpstreamFailuresWithJitteredBackoff asserts.
func backoffForAttempt(base, maxBackoff time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if maxBackoff <= 0 {
		maxBackoff = 30 * time.Second
	}

	backoff := base
	for i := 1; i < attempt; i++ {
		if backoff >= maxBackoff {
			backoff = maxBackoff
			break
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	half := backoff / 2
	if half <= 0 {
		return backoff
	}
	return half + time.Duration(rand.Int64N(int64(half)+1))
}
