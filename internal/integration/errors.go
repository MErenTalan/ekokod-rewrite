package integration

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel error kinds (06 §1 rule 5). Every adapter failure is wrapped in
// an *Error carrying one of these as Kind, so callers classify failures
// with errors.Is against a small, closed set instead of parsing text.
//
// ErrConfig (R48/I5) is a missing/invalid configuration or precondition —
// a missing endpoint template or placeholder, a zero multiplier, an empty
// installation/device id, an iSolar window wider than MaxWindow, a missing
// PM5340 base URL, a missing EPİAŞ username/password. It is deliberately
// NOT ErrAuth: every adapter used to report these as ErrAuth (round-1's
// documented stop-gap, "no dedicated configuration sentinel exists yet"),
// which meant one misconfigured analyzer, or a missing endpoint
// definition, rendered as "authentication failed" and would flag a whole
// credential as bad-password to F3's credential-health/"re-authenticate"
// logic. ErrConfig is non-retryable (job.ClassifyForRetry → SkipRetry,
// same as ErrAuth) but callers must never mark a credential as
// failed-auth, or show an "authentication failed"/"re-authenticate"
// message, for it. A genuine 401/403 response, or a provider's own
// "login succeeded but returned an empty token" response, remains ErrAuth.
var (
	ErrAuth                = errors.New("integration: authentication failed")
	ErrRateLimited         = errors.New("integration: rate limited")
	ErrUpstreamUnavailable = errors.New("integration: upstream unavailable")
	ErrMalformedPayload    = errors.New("integration: malformed payload")
	ErrNotFound            = errors.New("integration: not found")
	ErrConfig              = errors.New("integration: configuration incomplete or invalid")
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

// kindText renders Kind as one of the six sentinels' own text — never
// Kind.Error() directly. Kind is documented as "one of the sentinels
// above", but nothing in the type system enforces that: a caller can build
// an *Error with any error as Kind, including one built from live request
// data (e.g. fmt.Errorf("login %s rejected", pw)), and Error() must never
// echo that text — only the fixed, known-safe sentinel strings, or
// "unknown" for anything else (including a nil Kind, which would otherwise
// render as fmt's "%!s(<nil>)").
func kindText(kind error) string {
	switch {
	case errors.Is(kind, ErrAuth):
		return ErrAuth.Error()
	case errors.Is(kind, ErrRateLimited):
		return ErrRateLimited.Error()
	case errors.Is(kind, ErrUpstreamUnavailable):
		return ErrUpstreamUnavailable.Error()
	case errors.Is(kind, ErrMalformedPayload):
		return ErrMalformedPayload.Error()
	case errors.Is(kind, ErrNotFound):
		return ErrNotFound.Error()
	case errors.Is(kind, ErrConfig):
		return ErrConfig.Error()
	default:
		return "unknown"
	}
}

// Error renders using only Provider, Op, Kind and HTTPStatus — never a URL,
// query string or body — e.g. "gridbox load_profiles: integration: rate
// limited (HTTP 429)". Kind renders as one of the six sentinel strings
// (via kindText) or "unknown"; it never echoes an arbitrary Kind error's
// own text, which could otherwise carry request data into an operational
// message.
func (e *Error) Error() string {
	return fmt.Sprintf("%s %s: %s (HTTP %d)", e.Provider, e.Op, kindText(e.Kind), e.HTTPStatus)
}

// Unwrap exposes Kind to errors.Is/errors.As, so callers match against the
// sentinels above without caring which provider or endpoint produced them.
func (e *Error) Unwrap() error { return e.Kind }

// Retryable reports whether err is an integration failure that is worth
// retrying: rate limiting and upstream unavailability are transient by
// nature, while auth, malformed-payload, not-found and config failures
// will not resolve themselves on a retry.
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
