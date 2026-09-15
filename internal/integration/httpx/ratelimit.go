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
func waitForToken(ctx context.Context, lim *rate.Limiter, now func() time.Time, sleep sleepFunc) error {
	t := now()
	res := lim.ReserveN(t, 1)
	if !res.OK() {
		return fmt.Errorf("httpx: rate limiter burst exceeded")
	}

	delay := res.DelayFrom(t)
	if delay <= 0 {
		return nil
	}
	return sleep(ctx, delay)
}
