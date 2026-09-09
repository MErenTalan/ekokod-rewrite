package clock_test

import (
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/stretchr/testify/require"
)

func TestFakeClockIsDeterministic(t *testing.T) {
	start := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	c := clock.NewFake(start)

	require.Equal(t, start, c.Now())
	c.Advance(90 * time.Minute)
	require.Equal(t, start.Add(90*time.Minute), c.Now())
	require.Equal(t, 90*time.Minute, c.Since(start))
}

func TestSystemClockMoves(t *testing.T) {
	c := clock.System()
	first := c.Now()
	time.Sleep(time.Millisecond)
	require.True(t, c.Now().After(first))
}
