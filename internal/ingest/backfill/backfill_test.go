package backfill_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/ingest/backfill"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// backfillTestTs parses an RFC3339 timestamp, panicking on a typo — every
// literal here is a constant in a test file, so a parse failure is a bug in
// the test, not a runtime condition worth threading an error for.
func backfillTestTs(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("backfill_test: bad timestamp literal " + s + ": " + err.Error())
	}
	return t
}

// TestWindowsWrapsNormalizeChunk proves ONLY that Windows' job.Window
// conversion is faithful to normalize.Chunk's own boundaries and count — the
// gap/overlap/DST-alignment property itself is Task 3's own
// TestChunkCoversRangeWithoutGapOrOverlap / TestChunkAlignsToIstanbulMidnight
// (S7), not re-proven here.
func TestWindowsWrapsNormalizeChunk(t *testing.T) {
	cases := []struct {
		name     string
		from, to time.Time
		max      time.Duration
	}{
		{
			name: "single sub-day window, mid-day start",
			from: backfillTestTs("2026-08-31T21:00:00Z"), // 2026-09-01T00:00 Istanbul
			to:   backfillTestTs("2026-09-01T09:00:00Z"), // 2026-09-01T12:00 Istanbul
			max:  24 * time.Hour,
		},
		{
			name: "45 days at a 30-day max — two windows",
			from: backfillTestTs("2026-08-01T00:00:00Z"),
			to:   backfillTestTs("2026-09-15T00:00:00Z"),
			max:  30 * 24 * time.Hour,
		},
		{
			name: "exact day boundary, several whole days",
			from: backfillTestTs("2026-08-31T21:00:00Z"),
			to:   backfillTestTs("2026-09-03T21:00:00Z"),
			max:  24 * time.Hour,
		},
		{
			name: "empty range yields no windows",
			from: backfillTestTs("2026-09-01T00:00:00Z"),
			to:   backfillTestTs("2026-09-01T00:00:00Z"),
			max:  24 * time.Hour,
		},
		{
			name: "wide multi-month range at a 90-day max",
			from: backfillTestTs("2026-01-01T00:00:00Z"),
			to:   backfillTestTs("2026-06-01T00:00:00Z"),
			max:  90 * 24 * time.Hour,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := normalize.Chunk(tc.from, tc.to, tc.max)
			got := backfill.Windows(tc.from, tc.to, tc.max)

			require.Len(t, got, len(want), "window count must match normalize.Chunk")
			for i := range want {
				require.True(t, want[i].From.Equal(got[i].From), "window %d From must match normalize.Chunk", i)
				require.True(t, want[i].To.Equal(got[i].To), "window %d To must match normalize.Chunk", i)
			}
		})
	}
}
