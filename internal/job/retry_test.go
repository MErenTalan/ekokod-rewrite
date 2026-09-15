package job_test

import (
	"errors"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func TestClassifyForRetrySkipsNonRetryableKinds(t *testing.T) {
	for _, tc := range []struct {
		err  error
		skip bool
	}{
		{&integration.Error{Kind: integration.ErrAuth}, true},
		{&integration.Error{Kind: integration.ErrMalformedPayload}, true},
		{&integration.Error{Kind: integration.ErrNotFound}, true},
		{&integration.Error{Kind: integration.ErrConfig}, true},
		{&integration.Error{Kind: integration.ErrRateLimited}, false},
		{&integration.Error{Kind: integration.ErrUpstreamUnavailable}, false},
		{errors.New("db down"), false},
	} {
		require.Equal(t, tc.skip, errors.Is(job.ClassifyForRetry(tc.err), asynq.SkipRetry), "%v", tc.err)
	}
}

// TestClassifyForRetryConfigIsNotAuth is R48/I5's classification-contract
// regression: ErrConfig must skip retry (config errors are non-retryable,
// same as ErrAuth) WITHOUT ever satisfying errors.Is(_, integration.ErrAuth)
// — a caller building F3's credential-health/"re-authenticate" logic on
// errors.Is(err, integration.ErrAuth) must never see a config error (a
// missing endpoint template, a zero multiplier, …) misclassified as a
// failed credential.
func TestClassifyForRetryConfigIsNotAuth(t *testing.T) {
	wrapped := job.ClassifyForRetry(&integration.Error{Kind: integration.ErrConfig, Op: "config:token"})
	require.ErrorIs(t, wrapped, asynq.SkipRetry)
	require.ErrorIs(t, wrapped, integration.ErrConfig)
	require.NotErrorIs(t, wrapped, integration.ErrAuth)
}

func TestRetryDelayIsBoundedAndHonoursRetryAfter(t *testing.T) {
	for n := 0; n < 20; n++ {
		d := job.RetryDelay(n, errors.New("x"), nil)
		require.GreaterOrEqual(t, d, 15*time.Second)
		require.LessOrEqual(t, d, 30*time.Minute)
	}
	ra := &integration.Error{Kind: integration.ErrRateLimited, RetryAfter: 90 * time.Second}
	require.Equal(t, 90*time.Second, job.RetryDelay(3, ra, nil))
}

// TestRetryDelayCapsFarFutureRetryAfter proves a provider-supplied
// Retry-After is capped at the same 30-minute ceiling the exponential
// fallback obeys (M4, R5): a misconfigured or hostile far-future value must
// not park a task for hours.
func TestRetryDelayCapsFarFutureRetryAfter(t *testing.T) {
	ra := &integration.Error{Kind: integration.ErrRateLimited, RetryAfter: 6 * time.Hour}
	require.Equal(t, 30*time.Minute, job.RetryDelay(0, ra, nil))
}
