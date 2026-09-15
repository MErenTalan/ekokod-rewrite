package httpx

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// limiterRegistry holds one *rate.Limiter per LimiterKey, created lazily and
// shared across every Client built from the same Pool: two ClientConfigs
// (or two Do calls) naming the same key are spaced against each other, not
// just within one Client — ClientConfig.LimiterKey's doc says "shared
// limiter per key".
type limiterRegistry struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func newLimiterRegistry() *limiterRegistry {
	return &limiterRegistry{limiters: make(map[string]*rate.Limiter)}
}

// get returns the limiter for key, creating it on first use from every/
// burst. every<=0 means unlimited (rate.Inf); a subsequent call for the
// same key with different every/burst reuses the already-created limiter —
// the first caller to touch a given key decides its rate, which every
// caller within this codebase (one ClientConfig per provider/key) always
// agrees on.
func (r *limiterRegistry) get(key string, every time.Duration, burst int) *rate.Limiter {
	r.mu.Lock()
	defer r.mu.Unlock()

	if lim, ok := r.limiters[key]; ok {
		return lim
	}

	limit := rate.Inf
	if every > 0 {
		limit = rate.Every(every)
	}
	if burst < 1 {
		burst = 1
	}

	lim := rate.NewLimiter(limit, burst)
	r.limiters[key] = lim
	return lim
}

// waitForToken reserves one token from lim as of now() and sleeps (via
// sleep) for the resulting delay, honouring ctx cancellation. The
// reservation is computed against the INJECTED now, not time.Now — that is
// what lets TestLimiterSpacesRequestsPerKey observe deterministic spacing
// without a rate-limit test ever waiting in real time.
//
// M11: if the wait is abandoned (sleep returns an error — ctx was
// cancelled before the delay elapsed), the reservation is cancelled rather
// than left consumed. lim.ReserveN already committed the token as of now();
// Cancel gives it back (best-effort, per rate.Reservation's own docs) so a
// request that never actually happened does not permanently steal a slot
// from whichever request retries next.
func waitForToken(ctx context.Context, lim *rate.Limiter, now func() time.Time, sleep sleepFunc) error {
	t := now()
	res := lim.ReserveN(t, 1)
	if !res.OK() {
		return errRateLimiterBurstExceeded
	}

	delay := res.DelayFrom(t)
	if delay <= 0 {
		return nil
	}
	if err := sleep(ctx, delay); err != nil {
		res.CancelAt(now())
		return err
	}
	return nil
}

// errRateLimiterBurstExceeded is waitForToken's internal signal that
// ReserveN refused the reservation outright (Burst < 1, or a negative
// delay reservation was requested — not a real-world path with this
// package's own ClientConfig defaults, which always coerce Burst >= 1, but
// kept as a distinct sentinel rather than the removed fmt.Errorf so the
// caller in client.go can build a *integration.Error without ever
// formatting or echoing raw limiter-internal text, per M7.
var errRateLimiterBurstExceeded = fmt.Errorf("httpx: rate limiter refused the reservation")
