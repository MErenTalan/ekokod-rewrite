package job

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/hibiken/asynq"
)

// ClassifyForRetry wraps err with asynq.SkipRetry for the integration
// failure kinds that will not resolve themselves on a retry — auth,
// malformed payload, not found — so asynq stops after the first attempt
// instead of burning the retry budget on a task that can never succeed. A
// nil err, a retryable integration error (rate limited, upstream
// unavailable) and any error that is not an *integration.Error at all are
// returned unchanged, so asynq's ordinary retry/backoff policy applies to
// them.
func ClassifyForRetry(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, integration.ErrAuth) ||
		errors.Is(err, integration.ErrMalformedPayload) ||
		errors.Is(err, integration.ErrNotFound) {
		return fmt.Errorf("%w: %w", err, asynq.SkipRetry)
	}
	return err
}

// integRetryBase and integRetryCap bound the exponential backoff RetryDelay
// falls back to when err carries no provider-supplied RetryAfter.
const (
	integRetryBase = 30 * time.Second
	integRetryCap  = 30 * time.Minute
)

// integBackoff computes 30s·2^n, capped at 30 minutes, using only integer
// Duration arithmetic — never a float, per the money/energy rule extended
// to backoff timing. It stops doubling the moment it would reach or pass
// the cap, so a large n can never overflow the underlying int64
// nanosecond count.
func integBackoff(n int) time.Duration {
	d := integRetryBase
	for range n {
		if d >= integRetryCap {
			return integRetryCap
		}
		d *= 2
	}
	return min(d, integRetryCap)
}

// integJitter returns a value drawn uniformly from [d/2, d], using
// rand.Int64N rather than any float-based random source.
func integJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	half := d / 2
	span := d - half + 1 // +1 nanosecond: makes the upper bound inclusive
	return half + time.Duration(rand.Int64N(int64(span)))
}

// RetryDelay is asynq.Config.RetryDelayFunc. It honours a provider's own
// Retry-After when integration.RetryAfter finds one on err; otherwise it
// backs off exponentially from 30s, capped at 30 minutes, jittered into
// [d/2, d] so that a burst of tasks failing at once does not retry in
// lockstep.
func RetryDelay(n int, err error, _ *asynq.Task) time.Duration {
	if d, ok := integration.RetryAfter(err); ok {
		return d
	}
	return integJitter(integBackoff(n))
}
