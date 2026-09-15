package httpx

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

// classifyStatus maps a non-2xx HTTP status to one of the integration
// sentinels (06 §1 rule 5 / this package's Classification rule): 401/403 ->
// ErrAuth, 404 -> ErrNotFound, 429 -> ErrRateLimited, 408 and 5xx ->
// ErrUpstreamUnavailable (retryable), every other 4xx -> ErrMalformedPayload.
// Never called for a 2xx status — Do returns those as-is.
func classifyStatus(status int) error {
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return integration.ErrAuth
	case status == http.StatusNotFound:
		return integration.ErrNotFound
	case status == http.StatusTooManyRequests:
		return integration.ErrRateLimited
	case status == http.StatusRequestTimeout, status >= 500:
		return integration.ErrUpstreamUnavailable
	default:
		return integration.ErrMalformedPayload
	}
}

// classifyTransportError turns a RoundTrip failure into the
// *integration.Error Do ultimately returns, and reports whether it is worth
// retrying.
//
// err is typically *url.Error, which embeds the request's URL VERBATIM —
// including its query string, which for a provider like OSOS carries a
// password in the auth URL itself (removed-behaviour 17). err is
// deliberately never wrapped (no %w, no err.Error() appended) into the
// returned error: only the classification below is allowed to survive.
// Task brief Step 5(a)'s mutation — replacing the returned
// *integration.Error with something that wraps err via %w — is exactly the
// change TestHTTPErrorsNeverContainSubstitutedSecrets' network-failure case
// exists to catch.
//
// TLS/x509 certificate verification failures are classified the same as
// any other transport failure (ErrUpstreamUnavailable) but are NOT
// retried in-client: retrying a handshake against a certificate that will
// never verify only burns the retry budget on a deterministic outcome.
func classifyTransportError(provider integration.Provider, op string, err error) (result error, retry bool) {
	if isTLSVerificationFailure(err) {
		return &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: provider, Op: op}, false
	}
	return &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: provider, Op: op}, true
}

// isTLSVerificationFailure reports whether err is (or wraps) a certificate
// verification failure, as opposed to a connection-level failure (refused,
// reset, DNS, timeout) that is retryable like any other transient network
// error.
func isTLSVerificationFailure(err error) bool {
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return true
	}
	var hostErr x509.HostnameError
	if errors.As(err, &hostErr) {
		return true
	}
	var authErr x509.UnknownAuthorityError
	if errors.As(err, &authErr) {
		return true
	}
	var invalidErr x509.CertificateInvalidError
	if errors.As(err, &invalidErr) {
		return true
	}
	var constraintErr x509.ConstraintViolationError
	return errors.As(err, &constraintErr)
}

// parseRetryAfter parses a Retry-After header value per RFC 7231 §7.1.3:
// either an integer number of seconds, or an HTTP-date. now anchors an
// HTTP-date's conversion to a duration. ok is false for an empty or
// unparseable value.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			secs = 0
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}
