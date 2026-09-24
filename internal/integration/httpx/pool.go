// Package httpx is the one HTTP seam every provider adapter uses: timeouts,
// jittered retries, per-key rate limits, certificate pinning, strict URL
// templates and redaction-safe errors. Adapters call nothing else — they
// never construct their own *http.Client, and never see a raw transport
// error (classify.go turns every failure into an *integration.Error,
// stripped of any URL, query string, request/response body or header
// value).
//
// A Pool owns TLS pinning, the shared rate-limiter registry and (optional)
// cross-process lock; a Client (built from a Pool via Pool.Client) carries
// one provider/endpoint's retry and rate-limit policy.
package httpx

import (
	"context"
	"net/http"
	"time"

	lock "github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
)

// Locker and Lease are aliases of internal/platform/lock's own types (F2
// plan ruling R2) — not a structurally-identical redeclaration. Go does not
// treat two separately named interfaces with an identical method set as
// satisfying each other when a method's return type is itself one of those
// named interfaces, so a redeclared httpx.Locker/httpx.Lease would NOT be
// satisfied by *lock.Memory/*lock.Redis without an adapter. A `type X =
// lock.X` alias sidesteps that entirely: httpx.Locker IS lock.Locker.
type Locker = lock.Locker

// Lease is a held lock returned by Locker.Acquire. See Locker's doc above.
type Lease = lock.Lease

// PoolOptions configures a Pool.
type PoolOptions struct {
	// PinnedCerts maps host -> base64(DER) — the same shape as
	// config.External.PinnedCerts (EKOKOD_PINNED_CERTS). A host listed
	// here trusts ONLY that host's own certificate; every other host is
	// verified against the system root CAs.
	PinnedCerts map[string]string
	// Locker serialises requests that share a ClientConfig.SerializeKey,
	// across processes. nil: no cross-process serialisation (unit tests,
	// or a provider that never sets SerializeKey — PM5340, per the plan).
	Locker Locker
	// Sleep waits out a retry or rate-limit delay. nil: a real timer.
	// Tests inject a recording, non-blocking implementation so retry and
	// rate-limit tests never actually wait in real time.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now is the clock the rate limiter reserves tokens against. nil:
	// time.Now.
	Now func() time.Time

	// dialContext overrides every transport's dialer this Pool builds.
	// Unexported: only this package's own _test.go files (I5-ii's
	// pin-only-trust subprocess test, and any test that needs a minted
	// hostname to resolve to a loopback listener without real DNS) ever
	// set it, by constructing a PoolOptions literal from within package
	// httpx. Production code, and every external caller (httpx_test and
	// every adapter package), can never reach this field — it is simply
	// absent from the package's public API surface (R2-style: an
	// unexported field, not a new exported knob).
	dialContext dialContextFunc
}

// Pool owns one pinned-aware *http.Client plus the shared rate-limiter
// registry and optional Locker every Client built from it draws on.
type Pool struct {
	httpClient *http.Client
	locker     Locker
	now        func() time.Time
	sleep      sleepFunc
	limiters   *limiterRegistry
}

// NewPool builds a Pool. An invalid base64 payload or DER certificate in
// PinnedCerts is an error naming the offending host only — never the
// certificate bytes or the underlying parse error's own text, which in a
// misconfiguration could echo operator-supplied input back into a log line.
func NewPool(o PoolOptions) (*Pool, error) {
	transport, err := newPinnedRoundTripper(o.PinnedCerts, o.dialContext)
	if err != nil {
		return nil, err
	}

	now := o.Now
	if now == nil {
		now = time.Now
	}
	sleep := o.Sleep
	if sleep == nil {
		sleep = defaultSleep
	}

	return &Pool{
		httpClient: &http.Client{
			Transport: transport,
			// I1: never follow a redirect automatically. The default
			// policy forwards every header that Go does not specifically
			// treat as sensitive (Authorization, Cookie, Www-Authenticate
			// — NOT a provider's own custom auth header, e.g.
			// X-Access-Key) to whatever host and scheme the 3xx response
			// names, including a plain http:// target on a different
			// host entirely. http.ErrUseLastResponse makes Do return the
			// 3xx response itself instead — classify.go's classifyStatus
			// then turns it into a non-retryable ErrMalformedPayload
			// (HTTPStatus recorded), and the redirect target never
			// receives the request at all.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		locker:   o.Locker,
		now:      now,
		sleep:    sleep,
		limiters: newLimiterRegistry(),
	}, nil
}
