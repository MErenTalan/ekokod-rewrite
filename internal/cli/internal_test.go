package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/stretchr/testify/require"
)

// TestNewAPIServerBoundsHeaderBytes guards the X-Request-Id defect noted in
// task 9's review: middleware.RequestID (internal/api/middleware/requestid.go)
// echoes a client-supplied header verbatim, so the HTTP server must cap
// header size explicitly rather than rely on being reminded to do so later.
func TestNewAPIServerBoundsHeaderBytes(t *testing.T) {
	srv := newAPIServer(":8080", http.NotFoundHandler())
	require.Greater(t, srv.MaxHeaderBytes, 0, "MaxHeaderBytes must be set explicitly")
	// require.Equal, not an upper bound like <= 1<<20: that would admit
	// exactly Go's own DefaultMaxHeaderBytes (1 MiB), the value this whole
	// requirement exists to be tighter than (task 9 review, Minor-5).
	require.Equal(t, apiMaxHeaderBytes, srv.MaxHeaderBytes,
		"must be exactly the deliberately-chosen bound, not Go's own 1MiB default")
	require.Positive(t, srv.ReadHeaderTimeout)
}

// TestIsCleanShutdownDistinguishesCancellationFromRealErrors pins the
// scheduler command's classification of scheduler.Scheduler.Run's return
// value: Run returns the elector's ctx.Err() on a normal SIGINT/SIGTERM
// shutdown (context.Canceled), which must exit 0, while any other error
// (e.g. a Redis configuration failure at startup) must still be reported.
//
// context.DeadlineExceeded is deliberately NOT accepted (task 9 review,
// Important-3): $GOROOT/src/net/net.go defines
// (*timeoutError).Is(err) { return err == context.DeadlineExceeded }, so any
// error chain wrapping an ordinary net dial/i-o timeout would satisfy an
// errors.Is(err, context.DeadlineExceeded) clause. That would let a real
// scheduler failure (a Postgres or Redis dial timeout) be misclassified as
// a clean shutdown, exit 0, and never be restarted by a supervisor — with
// cron then silently stopping cluster-wide. Only context.Canceled, which is
// the only value scheduler.Scheduler.Run can actually produce today
// (Elector.Run's sole return statement is `return ctx.Err()` on a context
// with no deadline), is treated as clean.
func TestIsCleanShutdownDistinguishesCancellationFromRealErrors(t *testing.T) {
	require.True(t, isCleanShutdown(context.Canceled))
	require.False(t, isCleanShutdown(context.DeadlineExceeded),
		"a bare DeadlineExceeded must not be treated as a clean shutdown")
	require.False(t, isCleanShutdown(fmt.Errorf("dial tcp: %w", context.DeadlineExceeded)),
		"a %w-wrapped DeadlineExceeded (what a real dial timeout produces) must not be treated as clean")
	require.False(t, isCleanShutdown(fmt.Errorf("dial: %w", os.ErrDeadlineExceeded)),
		"an os-level deadline error must not be treated as clean either")
	require.False(t, isCleanShutdown(errors.New("boom")))
	require.False(t, isCleanShutdown(nil))
}

// TestNewHealthTrackerTripsAfterConsecutiveFailures pins Important-1 from
// the task 9 review: asynq starts no healthcheck goroutine at all unless a
// non-nil HealthCheckFunc is supplied, so without wiring one up, a worker
// whose Redis broker is unreachable runs forever with no health signal and
// no way to exit non-zero. newHealthTracker is the pure counting policy
// worker.go wires into job.NewServer's onHealthCheck parameter.
func TestNewHealthTrackerTripsAfterConsecutiveFailures(t *testing.T) {
	var tripped int
	track := newHealthTracker(4, func() { tripped++ })

	track(errors.New("dial refused"))
	track(errors.New("dial refused"))
	track(errors.New("dial refused"))
	require.Equal(t, 0, tripped, "must not trip before the limit is reached")

	track(errors.New("dial refused"))
	require.Equal(t, 1, tripped, "must trip exactly at the limit")

	track(errors.New("dial refused"))
	require.Equal(t, 1, tripped, "must not trip again for the same unbroken run of failures")
}

func TestNewHealthTrackerResetsOnSuccess(t *testing.T) {
	var tripped int
	track := newHealthTracker(4, func() { tripped++ })

	track(errors.New("dial refused"))
	track(errors.New("dial refused"))
	track(errors.New("dial refused"))
	track(nil) // a single successful ping resets the streak
	track(errors.New("dial refused"))
	track(errors.New("dial refused"))
	track(errors.New("dial refused"))
	require.Equal(t, 0, tripped, "a reset streak must need the full limit again before tripping")

	track(errors.New("dial refused"))
	require.Equal(t, 1, tripped)

	// A later, fresh run of failures (after a success) can trip again.
	track(nil)
	track(errors.New("dial refused"))
	track(errors.New("dial refused"))
	track(errors.New("dial refused"))
	track(errors.New("dial refused"))
	require.Equal(t, 2, tripped, "a fresh run of failures after a success must be able to trip again")
}

// TestNewCommandLoggerRedactsSecretLookingAttributes pins carried-forward
// requirement 1 from task 9's brief (also task 9 review, Minor-4): every
// long-running command's logger must go through logging.New, whose
// redacting handler is the sole thing keeping secret-looking attribute
// values out of the logs. Before this test existed, reverting any one of
// api.go/worker.go/scheduler.go's logger construction to a bare
// slog.NewJSONHandler compiled and passed the entire suite silently.
func TestNewCommandLoggerRedactsSecretLookingAttributes(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{LogLevel: "info", LogFormat: config.LogFormatJSON}

	log := newCommandLogger(cfg, &buf)
	log.Info("connecting", slog.String("password", "s3cr3t-value"))

	require.NotContains(t, buf.String(), "s3cr3t-value", "a secret-looking attribute value must never reach the log output")
	require.Contains(t, buf.String(), "REDACTED", "the redacted placeholder must be present instead")
}
