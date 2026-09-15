package integration_test

import (
	"errors"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/stretchr/testify/require"
)

// TestErrorUnwrapsToItsKind proves *integration.Error.Unwrap reaches
// exactly its own Kind: errors.Is is true for the sentinel it was built
// with and false for every other sentinel.
func TestErrorUnwrapsToItsKind(t *testing.T) {
	kinds := []error{
		integration.ErrAuth,
		integration.ErrRateLimited,
		integration.ErrUpstreamUnavailable,
		integration.ErrMalformedPayload,
		integration.ErrNotFound,
	}
	for _, kind := range kinds {
		err := &integration.Error{Kind: kind, Provider: integration.ProviderGridBox, Op: "load_profiles"}
		for _, other := range kinds {
			if errors.Is(kind, other) {
				require.Truef(t, errors.Is(err, other), "errors.Is(%v, %v) must be true for its own kind", kind, other)
			} else {
				require.Falsef(t, errors.Is(err, other), "errors.Is(%v, %v) must be false for a different kind", kind, other)
			}
		}
	}
}

// TestRetryableClassification pins which sentinels drive a retry: rate
// limiting and upstream unavailability are transient, everything else
// (including a plain, non-integration error) is not.
func TestRetryableClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"rate limited", &integration.Error{Kind: integration.ErrRateLimited}, true},
		{"upstream unavailable", &integration.Error{Kind: integration.ErrUpstreamUnavailable}, true},
		{"auth", &integration.Error{Kind: integration.ErrAuth}, false},
		{"malformed payload", &integration.Error{Kind: integration.ErrMalformedPayload}, false},
		{"not found", &integration.Error{Kind: integration.ErrNotFound}, false},
		{"plain error", errors.New("boom"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, integration.Retryable(tc.err))
		})
	}
}

// TestRetryAfterOnlyFromRateLimited proves RetryAfter reports ok=true only
// for a rate-limited *Error that actually carries a positive duration —
// never for any other kind, even one that happens to carry a RetryAfter
// value, and never as a bare zero duration a caller could mistake for "the
// provider said retry now".
func TestRetryAfterOnlyFromRateLimited(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		wantD  time.Duration
		wantOK bool
	}{
		{
			name:   "rate limited with a duration",
			err:    &integration.Error{Kind: integration.ErrRateLimited, RetryAfter: 5 * time.Second},
			wantD:  5 * time.Second,
			wantOK: true,
		},
		{
			name: "rate limited with no duration",
			err:  &integration.Error{Kind: integration.ErrRateLimited},
		},
		{
			name: "wrong kind carrying a duration",
			err:  &integration.Error{Kind: integration.ErrAuth, RetryAfter: 5 * time.Second},
		},
		{
			name: "plain error",
			err:  errors.New("boom"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, ok := integration.RetryAfter(tc.err)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantD, d)
		})
	}
}
