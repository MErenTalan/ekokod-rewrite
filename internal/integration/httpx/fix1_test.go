package httpx_test

// fix1_test.go covers the opus review findings from F2 Task 2 fix round 1
// (task-2-fix1-findings.md): I1-I5 plus the M6-M14 minors folded into this
// round. Each test's doc comment names the finding it proves and, where a
// mutation was used to confirm the test actually discriminates (rather
// than passing vacuously), says so; the mutation-and-restore cycles
// themselves are not committed — task-2-report.md records the observed
// FAIL output for each.

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	lock "github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
)

// ---------------------------------------------------------------------
// I1: redirects must never be followed.
// ---------------------------------------------------------------------

// TestRedirectsAreNeverFollowedAndLeakNoSecrets is I1's exact scenario: a
// pinned HTTPS origin answers with a 307 redirecting to a plain-http
// server on a different host:port, carrying a secret header and a secret
// body the way OSOS's legacy flow does. Before the fix, Go's default
// http.Client redirect policy forwards a custom header like X-Access-Key
// (Go only strips the small "sensitive" set — Authorization, Cookie,
// Www-Authenticate — on a cross-origin redirect, never an
// application-defined header) and the request body verbatim to the
// redirect target. Do must refuse the redirect outright: the target never
// receives the request at all, and Do returns a non-retryable error that
// echoes neither the secret header, the secret body content, nor either
// server's URL.
func TestRedirectsAreNeverFollowedAndLeakNoSecrets(t *testing.T) {
	const secretHeaderValue = "FIXTURE-ACCESS-KEY-7e1c"
	const secretBodyMarker = "FIXTURE-APPKEY-9b40"

	var attackerHits int32
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attackerHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", attacker.URL+"/steal")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	pool, _ := httpxTestPool(t, origin)
	client := pool.Client(httpx.ClientConfig{Provider: "osos", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 3})

	_, err := client.Do(context.Background(), httpx.Request{
		Op:       "authentication",
		Method:   http.MethodPost,
		Template: origin.URL + "/",
		Header:   http.Header{"X-Access-Key": {secretHeaderValue}},
		Body:     []byte(`{"appkey":"` + secretBodyMarker + `"}`),
	})

	require.Error(t, err)
	require.Zero(t, atomic.LoadInt32(&attackerHits), "the redirect target must receive NOTHING — no request at all")
	require.NotContains(t, err.Error(), secretHeaderValue)
	require.NotContains(t, err.Error(), secretBodyMarker)
	require.NotContains(t, err.Error(), attacker.URL)
	require.NotContains(t, err.Error(), origin.URL)
	require.False(t, integration.Retryable(err), "a 3xx is a non-retryable ErrMalformedPayload classification")

	var ierr *integration.Error
	require.ErrorAs(t, err, &ierr)
	require.Equal(t, http.StatusTemporaryRedirect, ierr.HTTPStatus, "the 3xx status is recorded, per the ruling")
	require.ErrorIs(t, err, integration.ErrMalformedPayload)
}

// ---------------------------------------------------------------------
// I2: Expand's path/query escaping split.
// ---------------------------------------------------------------------

// TestExpandPathEscapesWhenTemplateHasNoQueryString is I2's exact bug: a
// template with NO '?' at all is entirely a path, so every placeholder in
// it must be url.PathEscape'd. The pre-fix code's "qIdx >= 0 && start <
// qIdx" test is false for every placeholder whenever qIdx is -1 (no '?'
// present at all), silently falling through to QueryEscape instead — a
// space becomes "+" (query form) rather than "%20" (path form), and other
// path-illegal characters get query-only escaping in a path segment.
func TestExpandPathEscapesWhenTemplateHasNoQueryString(t *testing.T) {
	u, err := httpx.Expand("https://h/{a}/x", map[string]httpx.Param{
		"a": {Value: "A B"},
	})
	require.NoError(t, err)
	require.Contains(t, u.EscapedPath(), "A%20B", "a query-only template must still PATH-escape when there is no '?' at all")
	require.NotContains(t, u.EscapedPath(), "A+B")
}

// ---------------------------------------------------------------------
// I3: the SerializeKey lease is per-attempt, not per-call.
// ---------------------------------------------------------------------

// eventLocker is a Locker whose Acquire/Release both append to a single,
// shared, mutex-guarded event log alongside a *recordingSleep's own sleep
// calls (passed in as sleepLog), so the RELATIVE ORDER of acquire/release
// pairs and backoff sleeps can be asserted directly — the only way to
// prove I3's "release BEFORE any backoff sleep" rather than merely "one
// acquire/release pair exists somewhere".
type eventLocker struct {
	mu     sync.Mutex
	events *[]string
}

type eventLease struct {
	l   *eventLocker
	key string
}

func (l *eventLocker) Acquire(_ context.Context, key string, _ time.Duration) (lock.Lease, error) {
	l.mu.Lock()
	*l.events = append(*l.events, "acquire")
	l.mu.Unlock()
	return &eventLease{l: l, key: key}, nil
}

func (l *eventLease) Release(ctx context.Context) error {
	l.l.mu.Lock()
	entry := "release"
	if ctx.Err() != nil {
		// Would only happen if Release were (wrongly) called with the
		// caller's own, already-cancelled ctx instead of a detached one.
		entry = "release-on-cancelled-ctx"
	}
	*l.l.events = append(*l.l.events, entry)
	l.l.mu.Unlock()
	return nil
}

// TestSerializeLockIsPerAttemptNotPerCall (I3): with MaxAttempts=2 and a
// server that fails the first attempt (retryable) and succeeds the
// second, the lock must be acquired and released ONCE PER ATTEMPT — never
// held across the backoff sleep between them. The combined event log
// (shared between the lock and the injected Sleep) proves the exact
// order: acquire, release, sleep(backoff), acquire, release — not
// acquire, sleep, release.
func TestSerializeLockIsPerAttemptNotPerCall(t *testing.T) {
	var events []string
	var calls int32

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	locker := &eventLocker{events: &events}
	// sleepFn shares locker's own mutex (not a second, independent one) —
	// both write into the same *events slice, and using two different
	// mutexes to guard one slice would itself be a data race under -race.
	sleepFn := func(ctx context.Context, d time.Duration) error {
		locker.mu.Lock()
		events = append(events, "sleep")
		locker.mu.Unlock()
		return ctx.Err()
	}
	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{hostOf(t, srv.URL): base64.StdEncoding.EncodeToString(srv.Certificate().Raw)},
		Sleep:       sleepFn,
		Locker:      locker,
	})
	require.NoError(t, err)

	client := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 2, SerializeKey: "serialize-per-attempt"})
	_, err = client.Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.NoError(t, err)

	require.Equal(t, []string{"acquire", "release", "sleep", "acquire", "release"}, events,
		"the lease must be released BEFORE the backoff sleep, and re-acquired for the second attempt — never held across the sleep")
}

// TestSerializeLockReleaseSucceedsAfterCallerContextCancelled (I3): the
// caller's ctx is ALREADY cancelled before Do is even called. The single
// attempt this test allows (MaxAttempts=1) therefore fails immediately
// (ctx is done before net/http ever dials), but Release must still be
// called — and, crucially, with a ctx that is NOT itself already Done
// (eventLease.Release records "release-on-cancelled-ctx" instead of
// "release" if it ever observes ctx.Err() != nil). A Release
// implementation that reused the caller's own already-cancelled ctx
// instead of a detached one would fail this — exactly I3's "release
// succeeds after the caller ctx is cancelled" requirement.
func TestSerializeLockReleaseSucceedsAfterCallerContextCancelled(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	locker := &eventLocker{events: &[]string{}}
	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{hostOf(t, srv.URL): base64.StdEncoding.EncodeToString(srv.Certificate().Raw)},
		Sleep:       noSleep,
		Locker:      locker,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 1, SerializeKey: "release-after-cancel"})
	_, err = client.Do(ctx, httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.Error(t, err, "the attempt itself fails: ctx was already cancelled before Do")

	require.Equal(t, []string{"acquire", "release"}, *locker.events,
		"Release must be called (and observe a LIVE ctx) even though the caller's own ctx was already cancelled")
}

// ---------------------------------------------------------------------
// I4: the rate limiter is consulted before EVERY attempt.
// ---------------------------------------------------------------------

// tickingClock is a fake clock whose Sleep ADVANCES its own Now() by the
// requested duration before returning, instead of either blocking in real
// time or leaving Now() static. This is what makes "3 attempts are >=3s
// apart" a deterministic, non-flaky assertion: the rate limiter's own
// notion of elapsed time between attempts is driven entirely by how much
// this fake clock has been told to "sleep" for, never by real wall-clock
// time (which a loopback HTTP round trip finishes in microseconds).
type tickingClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTickingClock() *tickingClock { return &tickingClock{now: time.Unix(1_700_000_000, 0)} }

func (c *tickingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *tickingClock) Sleep(ctx context.Context, d time.Duration) error {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
	return ctx.Err()
}

// TestLimiterAppliesToEveryAttempt (I4): Every=3s, Burst=1, and a server
// that fails the first two attempts (retryable) before succeeding on the
// third. The server records, via the shared tickingClock, the virtual
// time at which each of the three attempts actually reached it. Before
// the fix (limiter consulted once, before the whole Do call, never again
// per retry), attempts 2 and 3 would be spaced only by the jittered
// BACKOFF delay (at most ~2s under this package's defaults) — well under
// 3s — because no additional limiter wait would ever be inserted between
// retries. After the fix, the limiter tops up whatever the backoff sleep
// didn't already cover, so consecutive attempts are always >= Every apart.
func TestLimiterAppliesToEveryAttempt(t *testing.T) {
	clock := newTickingClock()

	var mu sync.Mutex
	var seenAt []time.Time
	var calls int32

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenAt = append(seenAt, clock.Now())
		mu.Unlock()

		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{hostOf(t, srv.URL): base64.StdEncoding.EncodeToString(srv.Certificate().Raw)},
		Sleep:       clock.Sleep,
		Now:         clock.Now,
	})
	require.NoError(t, err)

	client := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 3 * time.Second, Burst: 1, MaxAttempts: 3})
	_, err = client.Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.NoError(t, err)

	require.Len(t, seenAt, 3)
	// golang.org/x/time/rate computes reservations internally via
	// float64 seconds, so a spacing that is mathematically exactly Every
	// can observably land a handful of nanoseconds under it (seen in
	// practice: 2.999999999s). A 1ms tolerance absorbs that float
	// imprecision without weakening the property under test — the
	// pre-fix behaviour this guards against was off by whole seconds,
	// not nanoseconds.
	const floatTolerance = time.Millisecond
	require.GreaterOrEqual(t, seenAt[1].Sub(seenAt[0])+floatTolerance, 3*time.Second, "attempt 2 must be >=Every after attempt 1")
	require.GreaterOrEqual(t, seenAt[2].Sub(seenAt[1])+floatTolerance, 3*time.Second, "attempt 3 must be >=Every after attempt 2")
}

// ---------------------------------------------------------------------
// I5(i): no connection leak across oversized-body and error responses.
// ---------------------------------------------------------------------

// TestNoConnectionLeakAcrossOversizedAndErrorResponses (I5-i): five
// sequential calls on ONE Client — cycling an oversized 200 body, a small
// 5xx body, and a moderately large (but still within the drain cap) 5xx
// body — must all reuse a single underlying TCP connection. Every
// response body this package classifies as a failure or as oversized is
// still fully drained and closed (client.go's readCappedBody), which is
// what lets Go's Transport return the connection to its idle pool instead
// of being forced to open a new one for the next call.
func TestNoConnectionLeakAcrossOversizedAndErrorResponses(t *testing.T) {
	var newConns int32

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("case") {
		case "oversized":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(make([]byte, 5000))
		case "small5xx":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("small error body"))
		case "large5xx":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write(make([]byte, 900))
		}
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			atomic.AddInt32(&newConns, 1)
		}
	}
	srv.StartTLS()
	defer srv.Close()

	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{hostOf(t, srv.URL): base64.StdEncoding.EncodeToString(srv.Certificate().Raw)},
		Sleep:       noSleep,
	})
	require.NoError(t, err)

	client := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 1, MaxBodyBytes: 1000})

	for _, c := range []string{"oversized", "small5xx", "large5xx", "oversized", "small5xx"} {
		_, err := client.Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL + "/?case=" + c})
		require.Error(t, err)
	}

	require.EqualValues(t, 1, atomic.LoadInt32(&newConns), "all 5 calls must reuse a single connection — a leaked (undrained/unclosed) body forces a fresh dial per call")
}

// ---------------------------------------------------------------------
// M7: lock-acquire and limiter failures are *integration.Error, no
// SerializeKey/UUID/lock-backend text.
// ---------------------------------------------------------------------

type failingLocker struct{}

func (failingLocker) Acquire(_ context.Context, key string, _ time.Duration) (lock.Lease, error) {
	return nil, fmt.Errorf("redis: dial tcp 10.0.0.7:6379: connection refused (lease key %q, corr-id 8f14e45f-ceea-4f5c-b0f2-1a2b3c4d5e6f)", key)
}

// TestLockAcquireFailureIsIntegrationErrorWithNoInternalText (M7): a
// Locker whose Acquire fails with an error that itself embeds the
// SerializeKey and a UUID-shaped correlation id (modelling a real
// Redis-backend error) must never let that text reach Do's returned
// error — only a bare *integration.Error survives.
func TestLockAcquireFailureIsIntegrationErrorWithNoInternalText(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{hostOf(t, srv.URL): base64.StdEncoding.EncodeToString(srv.Certificate().Raw)},
		Sleep:       noSleep,
		Locker:      failingLocker{},
	})
	require.NoError(t, err)

	const serializeKey = "osos:44c7b6d2-6c66-4f1b-9e2a-2b6a6a2b6a6a"
	client := pool.Client(httpx.ClientConfig{Provider: "osos", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 1, SerializeKey: serializeKey})
	_, err = client.Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})

	require.Error(t, err)
	var ierr *integration.Error
	require.ErrorAs(t, err, &ierr, "a lock-acquire failure must surface as *integration.Error")
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
	require.NotContains(t, err.Error(), serializeKey)
	require.NotContains(t, err.Error(), "8f14e45f-ceea-4f5c-b0f2-1a2b3c4d5e6f")
	require.NotContains(t, err.Error(), "redis")
	require.NotContains(t, err.Error(), "10.0.0.7")
}

// ---------------------------------------------------------------------
// M8: Expand requires an absolute http/https URL with a host.
// ---------------------------------------------------------------------

func TestExpandRequiresAbsoluteHTTPURLWithHost(t *testing.T) {
	for name, template := range map[string]string{
		"no scheme at all":    "//h/x",
		"relative path only":  "/x",
		"wrong scheme":        "ftp://h/x",
		"scheme with no host": "https:///x",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := httpx.Expand(template, nil)
			require.Error(t, err)
		})
	}
}

// ---------------------------------------------------------------------
// M9: Retry-After parsing never overflows or goes negative.
// ---------------------------------------------------------------------

// TestRetryAfterAbsurdlyLargeValueClampsInsteadOfOverflowing (M9): a
// Retry-After header naming a seconds count near (or beyond) int64's
// range must clamp to a bounded duration rather than overflowing the
// multiplication into an undefined (possibly negative) time.Duration.
// Exercised behaviourally through Do + a MaxRetryAfter cap: a clamped,
// huge-but-sane RetryAfter must exceed any real MaxRetryAfter cap
// (non-retried, RetryAfter() reports a large positive value) rather than
// wrapping negative (which, before a clamp, could slip under the cap and
// be treated as a short, immediate retry instead of the "never retry
// this" outcome a properly huge value demands).
func TestRetryAfterAbsurdlyLargeValueClampsInsteadOfOverflowing(t *testing.T) {
	for name, headerValue := range map[string]string{
		"far beyond int64 range":                             "999999999999999999999999999999",
		"exactly at int64 overflow boundary for seconds*1e9": "9223372036854775807",
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", headerValue)
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			defer srv.Close()

			pool, rs := httpxTestPool(t, srv)
			_, err := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 3, MaxRetryAfter: 120 * time.Second}).
				Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})

			require.ErrorIs(t, err, integration.ErrRateLimited)
			require.Empty(t, rs.recorded(), "a clamped-huge RetryAfter must still exceed MaxRetryAfter and never be slept out")

			wait, ok := integration.RetryAfter(err)
			require.True(t, ok)
			require.Positive(t, wait, "an overflowed multiplication could wrap negative; a properly clamped value never does")
		})
	}
}

// ---------------------------------------------------------------------
// M13: pinned-host key normalisation and plain-http refusal.
// ---------------------------------------------------------------------

// TestPinnedHostKeyIsCaseAndTrailingDotNormalised (M13): a PinnedCerts
// entry configured as "Host.Example." (mixed case, trailing dot) must
// still match a request to the lowercase, dot-free form the server
// actually presents — httptest's own srv.Certificate() is pinned under a
// deliberately differently-cased, dotted key here.
func TestPinnedHostKeyIsCaseAndTrailingDotNormalised(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host := hostOf(t, srv.URL)
	weirdKey := strings.ToUpper(host) + "."

	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: map[string]string{weirdKey: base64.StdEncoding.EncodeToString(srv.Certificate().Raw)},
		Sleep:       noSleep,
	})
	require.NoError(t, err)

	resp, err := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 1}).
		Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: srv.URL})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)
}

// TestPlainHTTPToPinnedHostIsRefused (M13): a pinned host's protection
// lives entirely in its dedicated TLS transport. A request whose template
// names that same host over plain http:// must never be silently carried
// in the clear over the pinned transport's non-TLS path (which would
// bypass the pin completely) — it must be refused before any bytes go on
// the wire.
func TestPlainHTTPToPinnedHostIsRefused(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the pinned host's handler must never be reached over plain http")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Same host:port as the pinned pool below, but scheme downgraded to
	// http — srv itself only speaks TLS, so if this were ever dialled as
	// plain HTTP the peer would just fail the handshake at the TCP
	// framing level; the point here is that httpx refuses it before even
	// attempting to dial.
	pool, rs := httpxTestPool(t, srv)
	plainURL := "http://" + strings.TrimPrefix(srv.URL, "https://")

	_, err := pool.Client(httpx.ClientConfig{Provider: "gridbox", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 3}).
		Do(context.Background(), httpx.Request{Op: "op", Method: "GET", Template: plainURL})
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
	// classify.go treats this the same as a certificate verification
	// failure: retrying a deterministic misconfiguration inside THIS
	// Do call wastes the retry budget on an outcome that can never
	// change. Proven the same way pin_test.go's TLS-failure cases are
	// documented as non-retried: with MaxAttempts=3 and a real
	// recordingSleep, a genuine internal retry would have to call Sleep
	// between attempts (BaseBackoff defaults to 1s > 0) — zero recorded
	// calls means zero retries happened.
	require.Empty(t, rs.recorded(), "a plain-http-to-pinned-host refusal must not be retried internally")
}

// ---------------------------------------------------------------------
// M14: Request.NoRetry forces exactly one attempt.
// ---------------------------------------------------------------------

// TestNoRetryForcesExactlyOneAttempt (R32/M14): even with MaxAttempts=5,
// a Request with NoRetry=true must be tried exactly once — modelling a
// non-idempotent auth/token-refresh POST where a second attempt after a
// transient failure would be actively harmful (invalidating a token the
// first, possibly-successful-but-unacknowledged attempt already
// obtained), not merely wasteful.
func TestNoRetryForcesExactlyOneAttempt(t *testing.T) {
	var calls int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError) // retryable, if retries were allowed
	}))
	defer srv.Close()

	pool, _ := httpxTestPool(t, srv)
	_, err := pool.Client(httpx.ClientConfig{Provider: "isolar", LimiterKey: t.Name(), Every: 0, Burst: 1, MaxAttempts: 5}).
		Do(context.Background(), httpx.Request{Op: "refresh_token", Method: "POST", Template: srv.URL, NoRetry: true})
	require.Error(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(&calls))
}

// ---------------------------------------------------------------------
// M12: pinning survives an HTTP CONNECT proxy (ruling R33) — NOT
// AUTOMATABLE in this test suite; documented rather than faked.
// ---------------------------------------------------------------------
//
// pin.go's baseTransport sets Proxy: http.ProxyFromEnvironment on every
// transport this package builds (both the per-host pinned transports and
// the shared one), which is the code-level fix for R33. An automated test
// exercising the CONNECT-tunnel path itself was attempted (an in-process
// CONNECT proxy plus a subprocess re-exec, the same technique
// pinonly_internal_test.go uses, to dodge net/http's own envProxyFunc
// sync.Once caching) and DOES NOT WORK, for a reason specific to this
// environment rather than to the fix: golang.org/x/net/http/httpproxy's
// ProxyFunc has a hard-coded, unconditional rule (proxy.go's useProxy:
// "if host == localhost { return false }", and separately "if
// ip.IsLoopback() { return false }") that NEVER proxies a request to
// 127.0.0.1/localhost, regardless of HTTP_PROXY/HTTPS_PROXY/NO_PROXY —
// confirmed empirically: http.ProxyFromEnvironment(req) for a
// 127.0.0.1-addressed request returns (nil, nil) even with HTTPS_PROXY
// freshly set in an otherwise-untouched subprocess. Every TLS test
// fixture available to this package (httptest.NewTLSServer,
// fake.NewTLSServer) listens on 127.0.0.1, so no test built from them can
// ever observe a proxy being consulted at all — the request always goes
// direct, independent of whether Proxy is wired correctly. Reaching a
// non-loopback listener would require either real DNS/hosts-file control
// or a non-loopback network interface, neither available in this sandbox
// and neither appropriate to depend on in a unit test run under `go test
// ./...`. The fix is verified by source inspection instead (baseTransport
// in pin.go, and golangci-lint/go vet finding no dead code around it) —
// a reviewer with a network sandbox that permits a non-loopback listener
// could reproduce the CONNECT-tunnel test straightforwardly by swapping
// the origin fixture's bind address.
