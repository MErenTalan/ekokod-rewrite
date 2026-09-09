package cli

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewAPIServerBoundsHeaderBytes guards the X-Request-Id defect noted in
// task 9's review: middleware.RequestID (internal/api/middleware/requestid.go)
// echoes a client-supplied header verbatim, so the HTTP server must cap
// header size explicitly rather than rely on being reminded to do so later.
func TestNewAPIServerBoundsHeaderBytes(t *testing.T) {
	srv := newAPIServer(":8080", http.NotFoundHandler())
	require.Greater(t, srv.MaxHeaderBytes, 0, "MaxHeaderBytes must be set explicitly")
	require.LessOrEqual(t, srv.MaxHeaderBytes, 1<<20, "must not be looser than Go's own 1MiB default")
	require.Positive(t, srv.ReadHeaderTimeout)
}

// TestIsCleanShutdownDistinguishesCancellationFromRealErrors pins the
// scheduler command's classification of scheduler.Scheduler.Run's return
// value: Run returns the elector's ctx.Err() on a normal SIGINT/SIGTERM
// shutdown (context.Canceled), which must exit 0, while any other error
// (e.g. a Redis configuration failure at startup) must still be reported.
func TestIsCleanShutdownDistinguishesCancellationFromRealErrors(t *testing.T) {
	require.True(t, isCleanShutdown(context.Canceled))
	require.True(t, isCleanShutdown(context.DeadlineExceeded))
	require.False(t, isCleanShutdown(errors.New("boom")))
	require.False(t, isCleanShutdown(nil))
}
