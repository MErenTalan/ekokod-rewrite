package httpx_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
)

// TestRetriesUpstreamFailuresWithJitteredBackoff: 500, 500, 200 -> success;
// exactly 3 requests; the two recorded sleeps fall within the equal-jitter
// windows [0.5s,1s] then [1s,2s] the plan's defaults (BaseBackoff=1s) imply.
func TestRetriesUpstreamFailuresWithJitteredBackoff(t *testing.T) {
	var calls int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Every: 0 (unlimited) is deliberate here: I4 made httpx wait for a
	// limiter token before EVERY attempt, not just the first, so with any
	// finite Every a retry's own limiter wait would add a THIRD, tiny
	// recorded delay alongside the two backoff sleeps this test asserts —
	// TestLimiterSpacesRequestsPerKey and the dedicated
	// TestLimiterAppliesToEveryAttempt below cover that behaviour on their
	// own terms, with a fixed injected clock so it is deterministic rather
	// than racing real wall-clock time against a 1ms cadence.
	pool, rs := httpxTestPool(t, srv)
	_, err := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 3}).
		Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.NoError(t, err)
	require.EqualValues(t, 3, atomic.LoadInt32(&calls))

	delays := rs.recorded()
	require.Len(t, delays, 2)
	require.GreaterOrEqual(t, delays[0], 500*time.Millisecond)
	require.LessOrEqual(t, delays[0], time.Second)
	require.GreaterOrEqual(t, delays[1], time.Second)
	require.LessOrEqual(t, delays[1], 2*time.Second)
}

// TestDoesNotRetryAuthOrMalformed: a non-retryable classification (401,
// 404, 400) makes exactly 1 request even with MaxAttempts > 1.
func TestDoesNotRetryAuthOrMalformed(t *testing.T) {
	for _, status := range []int{401, 404, 400} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls int32
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(status)
			}))
			defer srv.Close()

			pool, _ := httpxTestPool(t, srv)
			_, err := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: time.Millisecond, Burst: 1, MaxAttempts: 3}).
				Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
			require.Error(t, err)
			require.EqualValues(t, 1, atomic.LoadInt32(&calls))
		})
	}
}

// TestRateLimitedHonoursRetryAfterUpToCap: Retry-After: 7 -> one sleep of
// exactly 7s then success; Retry-After: 600 (over the default 120s
// MaxRetryAfter cap) -> no sleep, ErrRateLimited with RetryAfter()==600s.
func TestRateLimitedHonoursRetryAfterUpToCap(t *testing.T) {
	t.Run("under cap: retried after exactly the header's delay", func(t *testing.T) {
		var calls int32
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&calls, 1) == 1 {
				w.Header().Set("Retry-After", "7")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		pool, rs := httpxTestPool(t, srv)
		_, err := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 2}).
			Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
		require.NoError(t, err)
		require.Equal(t, []time.Duration{7 * time.Second}, rs.recorded())
	})

	t.Run("over cap: not retried, RetryAfter reports the uncapped value", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()

		pool, rs := httpxTestPool(t, srv)
		_, err := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 3}).
			Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
		require.ErrorIs(t, err, integration.ErrRateLimited)
		require.Empty(t, rs.recorded(), "the provider's wait exceeds MaxRetryAfter: httpx must not sleep for it")

		wait, ok := integration.RetryAfter(err)
		require.True(t, ok)
		require.Equal(t, 600*time.Second, wait)
	})
}

// TestDefaultRateLimitWait: a 429 with no Retry-After header uses
// ClientConfig.DefaultRateLimitWait verbatim.
func TestDefaultRateLimitWait(t *testing.T) {
	var calls int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool, rs := httpxTestPool(t, srv)
	_, err := pool.Client(httpx.ClientConfig{
		Provider: "epias", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 2,
		DefaultRateLimitWait: 65 * time.Second,
	}).Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.NoError(t, err)
	require.Equal(t, []time.Duration{65 * time.Second}, rs.recorded())
}

// TestContextCancellationStopsRetriesPromptly (I5-iii): a context that is
// cancelled right after the FIRST attempt's response comes back — but
// before doWithRetry would otherwise sleep out a backoff and issue a
// second one — must stop Do immediately: exactly one request reaches the
// server (asserted via the handler's own call count, not merely inferred),
// and Sleep is never invoked for a backoff delay. The handler cancels ctx
// itself right after being invoked once, which is what makes "no second
// attempt" and "no sleep after cancel" an observation about THIS run
// rather than a race against real wall-clock time. The resulting error
// keeps ctx's own cancellation identity (M6): errors.Is(err,
// context.Canceled) holds, and it is not classified Retryable.
func TestContextCancellationStopsRetriesPromptly(t *testing.T) {
	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		cancel()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	pool, rs := httpxTestPool(t, srv)
	_, err := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 3}).
		Do(ctx, httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled, "cancellation identity must survive: errors.Is(err, ctx.Err())")
	require.False(t, integration.Retryable(err), "a cancellation-identity error must never be classified Retryable")
	require.Empty(t, rs.recorded(), "no backoff sleep may be attempted once ctx is already done")
	require.EqualValues(t, 1, atomic.LoadInt32(&calls), "exactly one attempt reaches the server — no retry once ctx is cancelled")
}

// TestContextAlreadyCancelledBeforeDoNeverDialsTheServer is a companion
// edge case to the test above: when ctx is cancelled BEFORE Do is ever
// called (rather than mid-flight), the very first attempt fails before a
// connection is even dialled — net/http's own Transport checks ctx.Err()
// ahead of dialling — so the server sees zero requests, not one. Do still
// returns promptly with the same cancellation-identity error and no sleep.
func TestContextAlreadyCancelledBeforeDoNeverDialsTheServer(t *testing.T) {
	var calls int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pool, rs := httpxTestPool(t, srv)
	_, err := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 3}).
		Do(ctx, httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, integration.Retryable(err))
	require.Empty(t, rs.recorded())
	require.EqualValues(t, 0, atomic.LoadInt32(&calls), "an already-cancelled context never even reaches the server")
}
