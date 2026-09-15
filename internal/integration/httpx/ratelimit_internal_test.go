package httpx

// ratelimit_internal_test.go exercises waitForToken directly (package
// httpx, not httpx_test) because M11's property — an abandoned wait gives
// its reservation back rather than permanently consuming it — is about
// the unexported waitForToken/rate.Reservation interaction itself, not
// something easily isolated through the public Client.Do surface without
// a lot of incidental noise from the retry loop around it.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// TestWaitForTokenCancelsReservationWhenAbandoned (M11): after a wait is
// abandoned (the injected Sleep returns an error, modelling ctx
// cancellation mid-wait), the reservation it was waiting on must be given
// back rather than left permanently consumed. Proven by comparing the
// delay a THIRD reservation needs against what the SECOND (abandoned) one
// needed: with the reservation properly cancelled, the third call needs
// about the same wait the second one did (~1 Every); if the abandoned
// reservation's token were never returned, the deficit would compound and
// the third call would need roughly DOUBLE that.
func TestWaitForTokenCancelsReservationWhenAbandoned(t *testing.T) {
	lim := rate.NewLimiter(rate.Every(time.Second), 1)
	now := time.Unix(1_700_000_000, 0)
	nowFn := func() time.Time { return now }

	// First call: fresh limiter, full burst available, no wait.
	err := waitForToken(context.Background(), lim, nowFn, func(context.Context, time.Duration) error { return nil })
	require.NoError(t, err)

	// Second call, same instant: needs to wait ~1 Every. Abandon it
	// immediately (Sleep reports the ctx is already done) before it
	// "completes".
	var secondDelay time.Duration
	abandonedErr := waitForToken(context.Background(), lim, nowFn, func(_ context.Context, d time.Duration) error {
		secondDelay = d
		return context.Canceled
	})
	require.Error(t, abandonedErr)
	require.Greater(t, secondDelay, time.Duration(0))

	// Third call, same instant again: if the second (abandoned)
	// reservation was properly cancelled, this needs roughly the SAME
	// delay as the second one did (the token it never actually used is
	// available again). If not, the deficit compounds and this needs
	// roughly double.
	var thirdDelay time.Duration
	err = waitForToken(context.Background(), lim, nowFn, func(_ context.Context, d time.Duration) error {
		thirdDelay = d
		return nil
	})
	require.NoError(t, err)

	require.InDelta(t, float64(secondDelay), float64(thirdDelay), float64(100*time.Millisecond),
		"an abandoned wait's reservation must be cancelled (given back), not left permanently consumed")
}
