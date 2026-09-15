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

	// SerializeKey, if non-empty, has Do hold Pool's Locker for the whole
	// call (all retries), TTL serializeLeaseTTL. Empty, or a Pool built
	// with no Locker: no cross-process serialisation.
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

// Do sends req through c: rate limiter, then (if configured) the serialise
// lock, then — per attempt — a timeout and jittered retry. Returns a
// *integration.Error for every non-2xx outcome after retries are
// exhausted; a 2xx response is returned as-is.
func (c *Client) Do(ctx context.Context, req Request) (Response, error) {
	u, err := Expand(req.Template, req.Params)
	if err != nil {
		return Response{}, err
	}

	if err := waitForToken(ctx, c.limiter, c.pool.now, c.pool.sleep); err != nil {
		return Response{}, err
	}

	if c.pool.locker != nil && c.cfg.SerializeKey != "" {
		lease, err := c.pool.locker.Acquire(ctx, c.cfg.SerializeKey, serializeLeaseTTL)
		if err != nil {
			return Response{}, fmt.Errorf("httpx: acquire serialize lock for %q: %w", c.cfg.SerializeKey, err)
		}
		defer func() { _ = lease.Release(ctx) }()
	}

	return c.doWithRetry(ctx, req, u)
}

// doWithRetry runs the per-attempt timeout+classify+retry loop. lastErr is
// always an *integration.Error (or an Expand-shaped config error, which
// never reaches here) — see attempt.
func (c *Client) doWithRetry(ctx context.Context, req Request, u *url.URL) (Response, error) {
	var lastErr error
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		if attempt > 1 && ctx.Err() != nil {
			// Context cancellation stops retries promptly: do not even
			// start another attempt once the caller has given up.
			return Response{}, lastErr
		}

		result := c.attempt(ctx, req, u)
		if result.err == nil {
			return result.resp, nil
		}
		lastErr = result.err

		if !result.retry || attempt == c.cfg.MaxAttempts {
			return Response{}, lastErr
		}

		wait := result.wait
		if !result.explicitWait {
			wait = backoffForAttempt(c.cfg.BaseBackoff, c.cfg.MaxBackoff, attempt)
		}
		if sleepErr := c.pool.sleep(ctx, wait); sleepErr != nil {
			// ctx was done before the delay elapsed — stop retrying now
			// rather than issue one more attempt the caller no longer
			// wants.
			return Response{}, lastErr
		}
	}
	return Response{}, lastErr
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

// readCappedBody reads at most max+1 bytes of body, ALWAYS draining and
// closing it before returning (no connection leak, even on an oversized or
// erroring read) via the deferred drain-then-close below, which runs
// whether ReadAll succeeds, errors, or stopped early at the cap.
func readCappedBody(body io.ReadCloser, max int64) (data []byte, oversized bool, err error) {
	defer func() {
		_, _ = io.Copy(io.Discard, body)
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
