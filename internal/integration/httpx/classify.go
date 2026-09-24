package httpx

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
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
	// Refusing to dial a pinned host over plain HTTP (M13) is a
	// deterministic configuration error, exactly like a certificate that
	// will never verify: retrying burns the retry budget on an outcome
	// that cannot change.
	if isTLSVerificationFailure(err) || errors.Is(err, errPlainHTTPToPinnedHost) {
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

// maxRetryAfterSeconds bounds a parsed Retry-After value BEFORE it is
// multiplied into a time.Duration (M9): a duration is int64 nanoseconds, so
// a provider-supplied seconds count anywhere near math.MaxInt64/1e9 would
// overflow that multiplication and silently wrap into an unrelated
// (possibly negative) duration. math.MaxInt32 seconds is about 68 years —
// already far beyond any value ClientConfig.MaxRetryAfter's cap would ever
// let through unretried — so clamping here loses no legitimate value while
// making the multiplication below always safe.
const maxRetryAfterSeconds = math.MaxInt32

// parseRetryAfter parses a Retry-After header value per RFC 7231 §7.1.3:
// either an integer number of seconds, or an HTTP-date. now anchors an
// HTTP-date's conversion to a duration. ok is false for an empty or
// unparseable value. A negative value clamps to zero; a value too large to
// be a realistic wait (whether merely huge or literally unparseable as an
// int64) clamps to maxRetryAfterSeconds rather than overflowing.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		if secs < 0 {
			secs = 0
		}
		if secs > maxRetryAfterSeconds {
			secs = maxRetryAfterSeconds
		}
		return time.Duration(secs) * time.Second, true
	} else if isRange, matched := asRangeError(err); matched && isRange {
		// The header names an integer too large in magnitude for int64
		// itself (e.g. a provider bug or a hostile response) — still a
		// seconds count, not garbage, so it clamps rather than falling
		// through to the "unparseable" case below. A value this large and
		// negative clamps to zero, same as any other negative value;
		// anything this large and positive clamps to the same cap an
		// in-range huge value would.
		if strings.HasPrefix(strings.TrimSpace(v), "-") {
			return 0, true
		}
		return maxRetryAfterSeconds * time.Second, true
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

// asRangeError reports whether err is a *strconv.NumError signalling
// out-of-range (as opposed to a genuine syntax error, which should fall
// through to the HTTP-date parse attempt below it).
func asRangeError(err error) (isRange bool, matched bool) {
	var numErr *strconv.NumError
	if !errors.As(err, &numErr) {
		return false, false
	}
	return errors.Is(numErr.Err, strconv.ErrRange), true
}
