package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

// Plan-stated defaults (Task 2 brief's ClientConfig field docs).
const (
	defaultRequestTimeout       = 30 * time.Second
	defaultMaxAttempts          = 3
	defaultBaseBackoff          = time.Second
	defaultMaxBackoff           = 30 * time.Second
	defaultMaxRetryAfter        = 120 * time.Second
	defaultMaxBodyBytes   int64 = 50 * 1024 * 1024 // 50 MiB

	// serializeLeaseTTL is the fixed TTL ClientConfig.SerializeKey's lease
	// is held for, per the brief ("TTL 60s").
	serializeLeaseTTL = 60 * time.Second
)

// ClientConfig configures one Client drawn from a Pool: which provider and
// rate-limit key it speaks for, and its retry/timeout policy. Every
// zero-valued duration/count field falls back to the default named above.
type ClientConfig struct {
	Provider integration.Provider
	// LimiterKey names the shared rate limiter this Client's requests draw
	// from, e.g. "osos:<company uuid>". Two Clients (from this Pool or
	// another built the same way) naming the same key are spaced against
	// each other.
	LimiterKey string
	Every      time.Duration // one request per Every
	Burst      int

	RequestTimeout time.Duration // default 30s; applied per attempt
	MaxAttempts    int           // default 3
	BaseBackoff    time.Duration // default 1s
	MaxBackoff     time.Duration // default 30s
	MaxRetryAfter  time.Duration // default 120s

	// DefaultRateLimitWait is used when a 429 carries no Retry-After
	// header. Zero: fall back to the same jittered backoff schedule as
	// any other retryable failure. EPİAŞ sets 65s.
	DefaultRateLimitWait time.Duration

	// SerializeKey, if non-empty, has Do hold Pool's Locker for each
	// individual ATTEMPT (I3/R6 — see attemptWithLock's doc: acquired
	// after the rate limiter wait, released immediately after that one
	// round trip, never held across a retry's backoff sleep), TTL
	// serializeLeaseTTL. M11: this comment previously (round 1) said "for
	// the whole call (all retries)" — that was never the implementation;
	// corrected to match attemptWithLock. Empty, or a Pool built with no
	// Locker: no cross-process serialisation.
	SerializeKey string

	MaxBodyBytes int64 // default 50 MiB
}

// Param is one Expand/Request placeholder value.
type Param struct {
	Value string
	// Secret marks a value that must never appear in any error text.
	// Expand escapes it into the request URL like any other value — no
	// httpx error ever echoes a request URL, query string, body or header
	// value back, secret or not (classify.go), so Secret is documentation
	// for callers/reviewers rather than something this package branches
	// on.
	Secret bool
}

// Request is one call through a Client.
type Request struct {
	Op          string // endpoint key for errors, e.g. "authentication" — never a URL
	Method      string
	Template    string // may contain {placeholders}
	Params      map[string]Param
	Header      http.Header // values may be secret: never rendered in errors
	Body        []byte
	ContentType string

	// NoRetry forces exactly one attempt regardless of ClientConfig's
	// MaxAttempts (R32): for a non-idempotent call whose retry would be
	// actively harmful rather than merely wasteful — e.g. an
	// auth/token-refresh POST that invalidates the previous token on the
	// provider's side, so a second attempt after a network blip logs the
	// caller out of the token it just obtained instead of retrying the
	// same login.
	NoRetry bool
}

// Response is a successful (2xx) call's result.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Client sends Requests through its Pool's pinned transport, applying (in
// order) a per-LimiterKey rate-limit wait, an optional cross-process
// serialise lock held for the whole call, then — per attempt — a timeout
// and jittered retry.
type Client struct {
	pool    *Pool
	cfg     ClientConfig
	limiter *rate.Limiter
}

// Client builds a Client from p, applying c's defaults and looking up (or
// creating) the shared rate limiter for c.LimiterKey.
func (p *Pool) Client(c ClientConfig) *Client {
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = defaultRequestTimeout
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = defaultMaxAttempts
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = defaultBaseBackoff
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = defaultMaxBackoff
	}
	if c.MaxRetryAfter <= 0 {
		c.MaxRetryAfter = defaultMaxRetryAfter
	}
	if c.MaxBodyBytes <= 0 {
		c.MaxBodyBytes = defaultMaxBodyBytes
	}

	lim := p.limiters.get(c.LimiterKey, c.Every, c.Burst)
	return &Client{pool: p, cfg: c, limiter: lim}
}

// Do sends req through c. For each attempt, in order: a per-LimiterKey
// rate-limit wait (I4: every attempt draws its own token, so a retry never
// bypasses the provider rate by skipping the wait the first attempt
// already paid), then — if ClientConfig.SerializeKey is set — acquiring
// Pool's Locker for that ONE attempt only (I3/R6: "per request, not per
// task" — the lease is held across exactly one round trip, never across a
// whole call's retries and backoff sleeps, and is released before any
// backoff delay), then a per-attempt timeout and classify. Returns a
// *integration.Error for every non-2xx outcome after retries are
// exhausted; a 2xx response is returned as-is.
func (c *Client) Do(ctx context.Context, req Request) (Response, error) {
	u, err := Expand(req.Template, req.Params)
	if err != nil {
		return Response{}, err
	}
	return c.doWithRetry(ctx, req, u)
}

// doWithRetry runs the per-attempt limiter-wait + optional-lock + timeout +
// classify + retry loop. lastErr is always an *integration.Error, a
// cancellation-identity error (see ctxCancelledError), or an Expand-shaped
// config error (which never reaches here) — see attempt.
func (c *Client) doWithRetry(ctx context.Context, req Request, u *url.URL) (Response, error) {
	maxAttempts := c.cfg.MaxAttempts
	if req.NoRetry {
		maxAttempts = 1 // R32: exactly one attempt, no matter what MaxAttempts says.
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if waitErr := waitForToken(ctx, c.limiter, c.pool.now, c.pool.sleep); waitErr != nil {
			// M6: a limiter wait abandoned to context cancellation keeps
			// that identity; a genuine limiter failure (not ctx-caused)
			// is M7's *integration.Error, never the raw internal text.
			if ctx.Err() != nil {
				return Response{}, ctxCancelledError(ctx)
			}
			return Response{}, &integration.Error{Kind: integration.ErrRateLimited, Provider: c.cfg.Provider, Op: req.Op}
		}

		result := c.attemptWithLock(ctx, req, u)
		if result.err == nil {
			return result.resp, nil
		}
		lastErr = result.err

		if !result.retry || attempt == maxAttempts {
			return Response{}, lastErr
		}

		if ctx.Err() != nil {
			// M6 / I5-iii: do not even start a backoff sleep once the
			// caller has already given up — stop now, with an error that
			// keeps errors.Is(err, ctx.Err()) true.
			return Response{}, ctxCancelledError(ctx)
		}

		wait := result.wait
		if !result.explicitWait {
			wait = backoffForAttempt(c.cfg.BaseBackoff, c.cfg.MaxBackoff, attempt)
		}
		if sleepErr := c.pool.sleep(ctx, wait); sleepErr != nil {
			// ctx was done before the delay elapsed — stop retrying now
			// rather than issue one more attempt the caller no longer
			// wants.
			return Response{}, ctxCancelledError(ctx)
		}
	}
	return Response{}, lastErr
}

// attemptWithLock wraps attempt with the optional SerializeKey lease,
// scoped to exactly this one attempt (I3/R6). The lease is acquired after
// the limiter wait above and released immediately after the round trip
// returns — before doWithRetry's caller ever reaches a backoff sleep — so
// nothing is ever held across a sleep, and a slow or hung attempt cannot
// starve the lease past its own single round trip.
func (c *Client) attemptWithLock(ctx context.Context, req Request, u *url.URL) attemptResult {
	if c.pool.locker == nil || c.cfg.SerializeKey == "" {
		return c.attempt(ctx, req, u)
	}

	lease, lockErr := c.pool.locker.Acquire(ctx, c.cfg.SerializeKey, serializeLeaseTTL)
	if lockErr != nil {
		// M6/M7: same split as the limiter above — a lock-acquire failure
		// caused by ctx cancellation keeps that identity; any other
		// failure (lock backend unreachable, etc.) becomes a bare
		// *integration.Error with no SerializeKey name, lock-backend
		// text, or wrapped error text in it.
		if ctx.Err() != nil {
			return attemptResult{err: ctxCancelledError(ctx)}
		}
		return attemptResult{err: &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: c.cfg.Provider, Op: req.Op}}
	}

	result := c.attempt(ctx, req, u)

	// Release with a context that has already been detached from ctx's
	// own cancellation (I3): the caller's ctx may itself be the reason
	// this attempt failed (e.g. RequestTimeout, or the caller giving up
	// entirely) — releasing must still succeed so the lease is not held
	// for its full TTL just because the request that held it briefly
	// timed out or was cancelled. Bounded to 5s so a truly wedged lock
	// backend cannot hang Do forever on the way out.
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	_ = lease.Release(releaseCtx)
	cancel()

	return result
}

// ctxCancelledError reports ctx's own cancellation as the reason Do
// stopped, preserving errors.Is(err, ctx.Err()) (M6). It never wraps any
// other error text (so no attempt-classified failure's status, and no
// limiter/lock internal text, ever piggybacks on this path), and — since
// it does not wrap either integration sentinel — integration.Retryable
// reports false for it, satisfying M6's "and is NOT Retryable" without any
// extra bookkeeping.
func ctxCancelledError(ctx context.Context) error {
	return fmt.Errorf("httpx: request stopped: %w", ctx.Err())
}

// attemptResult is one HTTP attempt's outcome.
type attemptResult struct {
	resp Response
	err  error // nil on success (2xx, within MaxBodyBytes)
	// retry reports whether this failure is worth retrying if attempts
	// remain.
	retry bool
	// wait is the delay to use before the next attempt when
	// explicitWait is true (a provider-told Retry-After/DefaultRateLimitWait,
	// honoured exactly, no jitter); when explicitWait is false the caller
	// computes a jittered backoff instead.
	wait         time.Duration
	explicitWait bool
}

// attempt performs exactly one HTTP round trip: build the request with a
// fresh per-attempt timeout, send it, drain and classify the response (or
// the transport failure). It never retries by itself — doWithRetry decides
// that from the returned attemptResult.
func (c *Client) attempt(ctx context.Context, req Request, u *url.URL) attemptResult {
	attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()

	httpReq := c.buildRequest(attemptCtx, req, u)

	resp, err := c.pool.httpClient.Do(httpReq)
	if err != nil {
		ierr, retry := classifyTransportError(c.cfg.Provider, req.Op, err)
		return attemptResult{err: ierr, retry: retry}
	}
	// readCappedBody always drains and closes resp.Body itself (see its
	// doc comment) — this explicit, otherwise-redundant close is here only
	// to satisfy bodyclose's static analysis, which cannot see through
	// that call. A second Close on an already-closed body is a no-op.
	defer func() { _ = resp.Body.Close() }()

	body, oversized, readErr := readCappedBody(resp.Body, c.cfg.MaxBodyBytes)
	if readErr != nil {
		return attemptResult{
			err:   &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: c.cfg.Provider, Op: req.Op, HTTPStatus: resp.StatusCode},
			retry: true,
		}
	}
	if oversized {
		return attemptResult{
			err: &integration.Error{Kind: integration.ErrMalformedPayload, Provider: c.cfg.Provider, Op: req.Op, HTTPStatus: resp.StatusCode},
		}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return attemptResult{resp: Response{Status: resp.StatusCode, Header: resp.Header.Clone(), Body: body}}
	}

	kind := classifyStatus(resp.StatusCode)
	ierr := &integration.Error{Kind: kind, Provider: c.cfg.Provider, Op: req.Op, HTTPStatus: resp.StatusCode}

	if errors.Is(kind, integration.ErrRateLimited) {
		if wait, explicit := c.rateLimitWait(resp.Header); explicit {
			ierr.RetryAfter = wait
			if wait > c.cfg.MaxRetryAfter {
				return attemptResult{err: ierr} // provider's wait exceeds the cap: do not retry in-client
			}
			return attemptResult{err: ierr, retry: true, wait: wait, explicitWait: true}
		}
		return attemptResult{err: ierr, retry: true} // no provider signal: generic jittered backoff
	}

	if integration.Retryable(ierr) {
		return attemptResult{err: ierr, retry: true}
	}
	return attemptResult{err: ierr}
}

// rateLimitWait resolves how long to wait after a 429: the response's own
// Retry-After header if present (seconds or HTTP-date), else
// ClientConfig.DefaultRateLimitWait if set. ok is false when neither is
// available, meaning the caller falls back to the generic jittered backoff
// schedule.
func (c *Client) rateLimitWait(header http.Header) (wait time.Duration, ok bool) {
	if d, parsed := parseRetryAfter(header.Get("Retry-After"), c.pool.now()); parsed {
		return d, true
	}
	if c.cfg.DefaultRateLimitWait > 0 {
		return c.cfg.DefaultRateLimitWait, true
	}
	return 0, false
}

// buildRequest constructs the *http.Request for one attempt directly from
// the already-expanded URL u, never re-parsing a URL string (which could
// fail with an error that echoes the string itself, including any secret
// query param — the same hazard classify.go's transport-error rule guards
// against).
func (c *Client) buildRequest(ctx context.Context, req Request, u *url.URL) *http.Request {
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	var body io.ReadCloser
	var contentLength int64
	if len(req.Body) > 0 {
		body = io.NopCloser(bytes.NewReader(req.Body))
		contentLength = int64(len(req.Body))
	}

	httpReq := &http.Request{
		Method:        method,
		URL:           u,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header, len(req.Header)+1),
		Body:          body,
		ContentLength: contentLength,
		Host:          u.Host,
	}
	for k, vv := range req.Header {
		httpReq.Header[k] = append([]string(nil), vv...)
	}
	if req.ContentType != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", req.ContentType)
	}
	if len(req.Body) > 0 {
		bodyBytes := req.Body
		httpReq.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
	}
	return httpReq.WithContext(ctx)
}

// drainCap bounds the deferred best-effort drain below (M10): a body that
// is still not fully consumed after the primary capped read is drained by
// AT MOST this many further bytes before Close, never unbounded — an
// unbounded io.Copy(io.Discard, body) here would let a provider (or an
// attacker on a compromised provider connection) force this client to read
// an arbitrarily large response indefinitely just to be allowed to close
// the connection. A body larger than max+1+drainCap simply does not get
// its underlying TCP connection returned to the pool for reuse (Close on a
// not-fully-drained body forces the transport to discard rather than
// recycle it) — safe, just not reused; never a leak.
const drainCap = 64 * 1024

// readCappedBody reads at most max+1 bytes of body, ALWAYS draining
// (bounded, see drainCap) and closing it before returning (no connection
// leak, even on an oversized or erroring read) via the deferred
// drain-then-close below, which runs whether ReadAll succeeds, errors, or
// stopped early at the cap.
func readCappedBody(body io.ReadCloser, max int64) (data []byte, oversized bool, err error) {
	defer func() {
		_, _ = io.CopyN(io.Discard, body, drainCap)
		_ = body.Close()
	}()

	data, err = io.ReadAll(io.LimitReader(body, max+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > max {
		return data[:max], true, nil
	}
	return data, false, nil
}
