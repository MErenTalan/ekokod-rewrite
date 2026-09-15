package integration

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel error kinds (06 §1 rule 5). Every adapter failure is wrapped in
// an *Error carrying one of these as Kind, so callers classify failures
// with errors.Is against a small, closed set instead of parsing text.
var (
	ErrAuth                = errors.New("integration: authentication failed")
	ErrRateLimited         = errors.New("integration: rate limited")
	ErrUpstreamUnavailable = errors.New("integration: upstream unavailable")
	ErrMalformedPayload    = errors.New("integration: malformed payload")
	ErrNotFound            = errors.New("integration: not found")
)

// Error is what every adapter call returns on failure. It carries enough
// for retry policy and an operator message, and deliberately nothing more:
// no URL, no query string, no request or response body ever belongs in Op
// or in the error text (03 §7 / 06 §1 rule 5).
type Error struct {
	Kind       error // one of the sentinels above
	Provider   Provider
	Op         string // endpoint key, e.g. "load_profiles" — never a URL
	HTTPStatus int
	RetryAfter time.Duration
}

// Error renders using only Provider, Op, Kind and HTTPStatus — never a URL,
// query string or body — e.g. "gridbox load_profiles: integration: rate
// limited (HTTP 429)".
func (e *Error) Error() string {
	return fmt.Sprintf("%s %s: %s (HTTP %d)", e.Provider, e.Op, e.Kind, e.HTTPStatus)
}

// Unwrap exposes Kind to errors.Is/errors.As, so callers match against the
// sentinels above without caring which provider or endpoint produced them.
func (e *Error) Unwrap() error { return e.Kind }

// Retryable reports whether err is an integration failure that is worth
// retrying: rate limiting and upstream unavailability are transient by
// nature, while auth, malformed-payload and not-found failures will not
// resolve themselves on a retry.
func Retryable(err error) bool {
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrUpstreamUnavailable)
}

// RetryAfter reports the delay a provider told us to wait before retrying.
// It only ever comes from a rate-limited *Error with a positive RetryAfter
// — any other kind, a rate-limited error with no duration attached, or an
// error that is not an *Error at all, reports ok=false so the caller falls
// back to its own backoff policy instead of retrying immediately on a
// zero value it cannot distinguish from "the provider said now".
func RetryAfter(err error) (time.Duration, bool) {
	var ie *Error
	if !errors.As(err, &ie) {
		return 0, false
	}
	if !errors.Is(ie.Kind, ErrRateLimited) || ie.RetryAfter <= 0 {
		return 0, false
	}
	return ie.RetryAfter, true
}
