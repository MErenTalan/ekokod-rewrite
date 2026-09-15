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
		{&integration.Error{Kind: integration.ErrRateLimited}, false},
		{&integration.Error{Kind: integration.ErrUpstreamUnavailable}, false},
		{errors.New("db down"), false},
	} {
		require.Equal(t, tc.skip, errors.Is(job.ClassifyForRetry(tc.err), asynq.SkipRetry), "%v", tc.err)
	}
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
