package normalize_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// TestChunkCoversRangeWithoutGapOrOverlap is a property test: 200 random
// [from, to) ranges and max values, asserting the resulting windows are
// contiguous (no gap, no overlap), their union equals the input range, and
// each window is no longer than max.
func TestChunkCoversRangeWithoutGapOrOverlap(t *testing.T) {
	rng := rand.New(rand.NewSource(20260914))
	base := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 200; i++ {
		fromOffset := time.Duration(rng.Int63n(int64(20 * 365 * 24 * time.Hour)))
		spanHours := rng.Int63n(24*30) + 1 // up to ~30 days
		from := base.Add(fromOffset)
		to := from.Add(time.Duration(spanHours) * time.Hour)
		maxHours := rng.Int63n(24*7) + 1 // 1h..168h
		max := time.Duration(maxHours) * time.Hour

		windows := normalize.Chunk(from, to, max)
		requireGaplessCoverage(t, from, to, max, windows)
	}
}

func requireGaplessCoverage(t *testing.T, from, to time.Time, max time.Duration, windows []normalize.Window) {
	t.Helper()
	require.NotEmpty(t, windows, "from=%s to=%s max=%s", from, to, max)

	require.True(t, windows[0].From.Equal(from), "first window must start at from")
	require.True(t, windows[len(windows)-1].To.Equal(to), "last window must end at to")

	for i, w := range windows {
		require.True(t, w.From.Before(w.To), "window %d must be non-empty: %+v", i, w)
		require.LessOrEqual(t, w.To.Sub(w.From), max, "window %d exceeds max: %+v", i, w)
		if i > 0 {
			require.True(t, w.From.Equal(windows[i-1].To),
				"window %d does not start where window %d ended (gap or overlap): %+v vs %+v",
				i, i-1, windows[i-1], w)
		}
	}
}

// TestChunkAlignsToIstanbulMidnight proves a range starting mid-day with a
// max of 24h still produces a boundary at Istanbul-local midnight, not at
// from + 24h.
func TestChunkAlignsToIstanbulMidnight(t *testing.T) {
	// 2026-09-14 10:00 Istanbul local (UTC+3) = 07:00 UTC.
	from := time.Date(2026, 9, 14, 7, 0, 0, 0, time.UTC)
	to := from.Add(48 * time.Hour)

	windows := normalize.Chunk(from, to, 24*time.Hour)
	require.NotEmpty(t, windows)

	// The naive from+24h boundary would be 2026-09-15 07:00 UTC. The
	// correct Istanbul-local-midnight boundary is 2026-09-14 21:00 UTC
	// (2026-09-15 00:00 Istanbul, UTC+3).
	wantFirstBoundary := time.Date(2026, 9, 14, 21, 0, 0, 0, time.UTC)
	naiveBoundary := from.Add(24 * time.Hour)

	require.True(t, windows[0].To.Equal(wantFirstBoundary),
		"want first boundary at Istanbul local midnight %s, got %s", wantFirstBoundary, windows[0].To)
	require.False(t, windows[0].To.Equal(naiveBoundary),
		"boundary must not be from+24h (%s)", naiveBoundary)

	requireGaplessCoverage(t, from, to, 24*time.Hour, windows)
}

func TestChunkEmptyRangeIsEmpty(t *testing.T) {
	from := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	require.Empty(t, normalize.Chunk(from, from, time.Hour))
}

func TestChunkInvertedRangeIsEmpty(t *testing.T) {
	from := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	to := from.Add(-time.Hour)
	require.Empty(t, normalize.Chunk(from, to, time.Hour))
}

func TestChunkNonPositiveMaxIsEmpty(t *testing.T) {
	from := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	require.Empty(t, normalize.Chunk(from, to, 0))
	require.Empty(t, normalize.Chunk(from, to, -time.Hour))
}
