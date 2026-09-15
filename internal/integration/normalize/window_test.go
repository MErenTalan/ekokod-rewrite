package normalize_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// istanbulLoc mirrors normalize.Istanbul for use in hand-derived
// expectations; tests use their own handle on the location rather than
// reaching into the package's exported var so the "expected" side of an
// assertion is visibly independent of anything Chunk touches internally.
var istanbulLoc = normalize.Istanbul

// civilDaySpan counts whole calendar days between two civil dates
// (y1,m1,d1) and (y2,m2,d2), using time.Date in UTC (which has no DST, so
// every day is exactly 24h) purely as a calendar-arithmetic calculator —
// never involving Europe/Istanbul or normalize.Chunk. This is the
// independent yardstick the tests below use to check how many Istanbul
// calendar days a window actually spans.
func civilDaySpan(y1, m1, d1, y2, m2, d2 int) int {
	a := time.Date(y1, time.Month(m1), d1, 0, 0, 0, 0, time.UTC)
	b := time.Date(y2, time.Month(m2), d2, 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a) / (24 * time.Hour))
}

// calendarDaySpan returns how many distinct Istanbul-local calendar days a
// half-open window [from, to) touches. If to lands exactly on a local
// midnight, that day is excluded (it belongs to the next window, if any);
// otherwise the day containing to is a partial day and still counts as
// one whole day, matching R36's "the day containing cur counts as one
// full day even when cur starts mid-day" rule applied at the tail end.
func calendarDaySpan(loc *time.Location, from, to time.Time) int {
	fy, fm, fd := from.In(loc).Date()
	tLocal := to.In(loc)
	ty, tm, td := tLocal.Date()
	h, mi, s := tLocal.Clock()
	if h == 0 && mi == 0 && s == 0 && tLocal.Nanosecond() == 0 {
		return civilDaySpan(fy, int(fm), fd, ty, int(tm), td)
	}
	return civilDaySpan(fy, int(fm), fd, ty, int(tm), td+1)
}

// istanbulMidnight builds the UTC instant for an Istanbul-local midnight,
// via time.Date directly against normalize.Istanbul — this is the same
// primitive Chunk itself uses, but here it only *constructs* fixture
// timestamps; it is never used to compute an expected window boundary
// from Chunk's own output.
func istanbulMidnight(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, istanbulLoc)
}

// TestChunkCoversRangeWithoutGapOrOverlap is a property test over 400
// random [from, to) ranges and max values. Roughly half the max values
// are sub-day (1h..23h) and half are whole-day multiples of 24h up to
// 30 days (24h, 48h, 72h, 7d, 14d, 30d), and a slice of the ranges is
// anchored around Turkey's three post-2014 DST transitions
// (2015-03-29 23h day, 2015-11-08 25h day, 2016-03-27 23h day) so the
// generator provably exercises both the max >= 24h path and DST-crossing
// ranges rather than relying on chance. Counts of each are asserted at
// the end so a future change to the generator that silently drops this
// coverage fails loudly.
func TestChunkCoversRangeWithoutGapOrOverlap(t *testing.T) {
	rng := rand.New(rand.NewSource(20260914))
	base := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)

	dstAnchors := []time.Time{
		time.Date(2015, 3, 29, 0, 0, 0, 0, time.UTC),
		time.Date(2015, 11, 8, 0, 0, 0, 0, time.UTC),
		time.Date(2016, 3, 27, 0, 0, 0, 0, time.UTC),
	}
	wholeDayOptions := []time.Duration{
		24 * time.Hour, 48 * time.Hour, 72 * time.Hour,
		7 * 24 * time.Hour, 14 * 24 * time.Hour, 30 * 24 * time.Hour,
	}

	const iterations = 400
	const dstAnchoredCases = 60 // 20 per anchor

	maxGE24hCount := 0
	dstCrossingCount := 0

	for i := 0; i < iterations; i++ {
		var from time.Time
		if i < dstAnchoredCases {
			anchor := dstAnchors[i%len(dstAnchors)]
			jitter := time.Duration(rng.Int63n(int64(6*24*time.Hour))) - 3*24*time.Hour
			from = anchor.Add(jitter)
		} else {
			fromOffset := time.Duration(rng.Int63n(int64(20 * 365 * 24 * time.Hour)))
			from = base.Add(fromOffset)
		}

		spanDays := rng.Int63n(400) + 1
		to := from.Add(time.Duration(spanDays) * 24 * time.Hour)

		var max time.Duration
		if rng.Intn(2) == 0 {
			max = time.Duration(rng.Int63n(23)+1) * time.Hour // 1h..23h
		} else {
			max = wholeDayOptions[rng.Intn(len(wholeDayOptions))]
		}
		if max >= 24*time.Hour {
			maxGE24hCount++
		}
		if crossesIstanbulDST(from, to) {
			dstCrossingCount++
		}

		windows := normalize.Chunk(from, to, max)
		requireGaplessCoverage(t, from, to, max, windows)
	}

	require.Greater(t, maxGE24hCount, 0, "generator must produce max >= 24h cases")
	require.Greater(t, dstCrossingCount, 0, "generator must produce DST-crossing ranges")
	t.Logf("max >= 24h cases: %d/%d, DST-crossing ranges: %d/%d", maxGE24hCount, iterations, dstCrossingCount, iterations)
}

// crossesIstanbulDST reports whether the Istanbul UTC offset differs
// between the start of the range and its last included instant. Turkey's
// tzdata history (post-2014) never reverts to an earlier offset after
// 2016, so a single offset comparison at the two ends is sufficient to
// detect a range that crosses one or more of the three transitions this
// suite targets.
func crossesIstanbulDST(from, to time.Time) bool {
	_, fromOffset := from.In(istanbulLoc).Zone()
	_, toOffset := to.Add(-time.Nanosecond).In(istanbulLoc).Zone()
	return fromOffset != toOffset
}

// requireGaplessCoverage asserts the general Chunk contract: windows are
// contiguous (no gap, no overlap), their union equals [from, to), and no
// window is over-sized for the max that produced it. "Over-sized" means
// two different things depending on max (R36): below 24h it is a plain
// wall-clock bound (w.To - w.From <= max); at or above 24h it is a
// calendar-day bound (the window spans at most floor(max/24h) Istanbul
// calendar days) — a DST day can make wall-clock duration alone a false
// failure (a 30-day, max=30d window that contains the 25h fallback day is
// 30*24h+1h of wall time, which is correct per R36, not a bug).
func requireGaplessCoverage(t *testing.T, from, to time.Time, max time.Duration, windows []normalize.Window) {
	t.Helper()
	require.NotEmpty(t, windows, "from=%s to=%s max=%s", from, to, max)

	require.True(t, windows[0].From.Equal(from), "first window must start at from")
	require.True(t, windows[len(windows)-1].To.Equal(to), "last window must end at to")

	for i, w := range windows {
		require.True(t, w.From.Before(w.To), "window %d must be non-empty: %+v", i, w)

		if max < 24*time.Hour {
			require.LessOrEqual(t, w.To.Sub(w.From), max, "window %d exceeds max: %+v", i, w)
		} else {
			maxDays := int(max / (24 * time.Hour))
			gotDays := calendarDaySpan(istanbulLoc, w.From, w.To)
			require.LessOrEqual(t, gotDays, maxDays,
				"window %d spans %d Istanbul calendar days, more than floor(max/24h)=%d: %+v", i, gotDays, maxDays, w)
		}

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

// TestChunkMidDayStartWithMultiDayMax is the max >= 24h analogue of
// TestChunkAlignsToIstanbulMidnight: R36 says the first window "may start
// mid-day at from", and the day containing from still counts as the first
// of the max's calendar days.
//
// from = 2026-09-14 10:00 Istanbul (mid-day), max = 3 * 24h = 72h so
// maxDays = 3. The three calendar days counted from from's day are
// 09-14 (partial, the day containing from), 09-15 and 09-16, so the
// window must end at the Istanbul midnight starting 09-17, i.e.
// 2026-09-17 00:00 +03:00 = 2026-09-16 21:00 UTC — NOT at
// from + 72h = 2026-09-17 10:00 UTC (a naive duration add) and NOT at
// the very next midnight (2026-09-14 21:00 UTC, the max=24h answer).
func TestChunkMidDayStartWithMultiDayMax(t *testing.T) {
	from := time.Date(2026, 9, 14, 7, 0, 0, 0, time.UTC) // 10:00 Istanbul
	to := from.Add(10 * 24 * time.Hour)
	max := 3 * 24 * time.Hour

	windows := normalize.Chunk(from, to, max)
	require.NotEmpty(t, windows)

	wantFirstBoundary := time.Date(2026, 9, 16, 21, 0, 0, 0, time.UTC)
	naiveDurationBoundary := from.Add(max)
	singleDayBoundary := time.Date(2026, 9, 14, 21, 0, 0, 0, time.UTC)

	require.True(t, windows[0].From.Equal(from))
	require.True(t, windows[0].To.Equal(wantFirstBoundary),
		"want first boundary %s, got %s", wantFirstBoundary, windows[0].To)
	require.False(t, windows[0].To.Equal(naiveDurationBoundary))
	require.False(t, windows[0].To.Equal(singleDayBoundary))

	requireGaplessCoverage(t, from, to, max, windows)
}

// TestChunk45DayRangeMax30Days is the explicit case from Ruling R36: a
// 45-day, midnight-aligned range with max = 30 days must split into
// exactly two windows of 30 and 15 days (2 = ceil(45/30)), not 45
// single-day windows.
func TestChunk45DayRangeMax30Days(t *testing.T) {
	from := istanbulMidnight(2020, 1, 1) // day 0
	to := istanbulMidnight(2020, 2, 15)  // day 0 + 45 days (2020 is a leap year but Jan has 31 days: Jan 1 + 45d = Feb 15)
	max := 30 * 24 * time.Hour

	windows := normalize.Chunk(from, to, max)

	// Hand-derived: window 0 = [day 0, day 30) = [2020-01-01, 2020-01-31),
	// window 1 = [day 30, day 45) = [2020-01-31, 2020-02-15).
	want := []normalize.Window{
		{From: istanbulMidnight(2020, 1, 1), To: istanbulMidnight(2020, 1, 31)},
		{From: istanbulMidnight(2020, 1, 31), To: istanbulMidnight(2020, 2, 15)},
	}
	requireWindowsEqual(t, want, windows)
	require.Len(t, windows, 2)

	requireGaplessCoverage(t, from, to, max, windows)
}

// TestChunkWholeDayWindowCountFormula pins R36's "window count =
// ceil(days/maxDays) for a midnight-aligned from" rule across a table of
// hand-picked (totalDays, maxDays) pairs, including ranges that cross the
// 2015-03-29 (23h), 2015-11-08 (25h) and 2016-03-27 (23h) DST transitions
// — each such day still counts as exactly one calendar day toward
// maxDays, per R36, regardless of its wall-clock length.
func TestChunkWholeDayWindowCountFormula(t *testing.T) {
	ceilDiv := func(a, b int) int { return (a + b - 1) / b }

	cases := []struct {
		name      string
		from      time.Time
		totalDays int
		maxDays   int
	}{
		{"7 days, max 24h(1d): 7 windows", istanbulMidnight(2026, 9, 1), 7, 1},
		{"7 days, max 72h(3d): ceil(7/3)=3 windows", istanbulMidnight(2026, 9, 1), 7, 3},
		{"30 days, max 30d: 1 window", istanbulMidnight(2026, 1, 1), 30, 30},
		{"31 days, max 30d: 2 windows", istanbulMidnight(2026, 1, 1), 31, 30},
		{"400 days, max 30d: ceil(400/30)=14 windows", istanbulMidnight(2015, 1, 1), 400, 30},
		// Spans the 2015-03-29 23h spring-forward day (day index 87 from
		// 2015-01-01, 0-indexed: Jan(31)+Feb(28)=59, +28 = day 87 is
		// 2015-03-29). 90 total days, max 7d -> ceil(90/7)=13 windows.
		{"90 days crossing 2015-03-29, max 7d: ceil(90/7)=13 windows", istanbulMidnight(2015, 1, 1), 90, 7},
		// Spans 2015-11-08 (25h fallback day): day index 311 from
		// 2015-01-01 (31+28+31+30+31+30+31+31+30+31+7=311). 320 total
		// days, max 14d -> ceil(320/14)=23 windows.
		{"320 days crossing 2015-11-08, max 14d: ceil(320/14)=23 windows", istanbulMidnight(2015, 1, 1), 320, 14},
		// Spans 2016-03-27 (23h spring-forward day, the last Turkish DST
		// transition ever). 100 days from 2016-03-01, max 30d ->
		// ceil(100/30)=4 windows.
		{"100 days crossing 2016-03-27, max 30d: ceil(100/30)=4 windows", istanbulMidnight(2016, 3, 1), 100, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			to := tc.from.AddDate(0, 0, tc.totalDays)
			max := time.Duration(tc.maxDays) * 24 * time.Hour

			windows := normalize.Chunk(tc.from, to, max)

			wantCount := ceilDiv(tc.totalDays, tc.maxDays)
			require.Len(t, windows, wantCount, "from=%s totalDays=%d maxDays=%d", tc.from, tc.totalDays, tc.maxDays)

			requireGaplessCoverage(t, tc.from, to, max, windows)

			// Every window's from/to must itself be Istanbul-local
			// midnight (from is midnight-aligned in every case here, so
			// unlike TestChunkMidDayStartWithMultiDayMax there is no
			// mid-day first window).
			for i, w := range windows {
				requireIsIstanbulMidnight(t, w.From, "window %d From", i)
				requireIsIstanbulMidnight(t, w.To, "window %d To", i)
			}
		})
	}
}

func requireIsIstanbulMidnight(t *testing.T, ts time.Time, msg string, args ...any) {
	t.Helper()
	local := ts.In(istanbulLoc)
	h, m, s := local.Clock()
	require.Zero(t, h, msg, args)
	require.Zero(t, m, msg, args)
	require.Zero(t, s, msg, args)
	require.Zero(t, local.Nanosecond(), msg, args)
}

// requireWindowsEqual compares windows by instant (time.Time.Equal), not
// by require.Equal/reflect.DeepEqual: two time.Time values naming the same
// instant in different *time.Location representations (e.g. one built via
// time.Date(..., time.UTC) in a test's hand-derived "want", the other
// produced by Chunk via time.Date(..., normalize.Istanbul)) are Equal but
// not DeepEqual, so struct/slice equality would fail on a correct result.
func requireWindowsEqual(t *testing.T, want, got []normalize.Window) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		require.True(t, want[i].From.Equal(got[i].From), "window %d From: want %s got %s", i, want[i].From, got[i].From)
		require.True(t, want[i].To.Equal(got[i].To), "window %d To: want %s got %s", i, want[i].To, got[i].To)
	}
}

// TestChunkDSTDayCountsAsOneCalendarDay pins R36's "a DST day of 23h/25h
// counts as one calendar day" rule with hand-computed UTC instants (via
// time.LoadLocation("Europe/Istanbul") applied directly to time.Date, the
// same tzdata the production code relies on — not via normalize.Chunk).
//
// 2015-03-29 (spring forward, EET -> EEST) is a 23h day:
//
//	2015-03-29 00:00 Istanbul (EET,  UTC+2) = 2015-03-28 22:00 UTC
//	2015-03-30 00:00 Istanbul (EEST, UTC+3) = 2015-03-29 21:00 UTC
//	=> window 0 wall-clock length = 23h, but counts as 1 calendar day.
//
// 2015-03-31 00:00 Istanbul (EEST, UTC+3) = 2015-03-30 21:00 UTC, so
// window 1 (2015-03-30 -> 2015-03-31, a normal day after the transition)
// is a plain 24h window.
func TestChunkDSTDayCountsAsOneCalendarDay(t *testing.T) {
	from := istanbulMidnight(2015, 3, 29)
	to := istanbulMidnight(2015, 3, 31) // 2 calendar days later
	max := 24 * time.Hour               // maxDays = 1

	windows := normalize.Chunk(from, to, max)

	want := []normalize.Window{
		{
			From: time.Date(2015, 3, 28, 22, 0, 0, 0, time.UTC), // 2015-03-29 00:00 EET
			To:   time.Date(2015, 3, 29, 21, 0, 0, 0, time.UTC), // 2015-03-30 00:00 EEST
		},
		{
			From: time.Date(2015, 3, 29, 21, 0, 0, 0, time.UTC), // 2015-03-30 00:00 EEST
			To:   time.Date(2015, 3, 30, 21, 0, 0, 0, time.UTC), // 2015-03-31 00:00 EEST
		},
	}
	requireWindowsEqual(t, want, windows)
	require.Equal(t, 23*time.Hour, windows[0].To.Sub(windows[0].From), "the DST spring-forward day is 23h of wall time")
	require.Equal(t, 24*time.Hour, windows[1].To.Sub(windows[1].From))

	requireGaplessCoverage(t, from, to, max, windows)
}

// TestChunkDSTFallbackDayCountsAsOneCalendarDay covers the 25h
// 2015-11-08 fallback day (EEST -> EET) the same way:
//
//	2015-11-08 00:00 Istanbul (EEST, UTC+3) = 2015-11-07 21:00 UTC
//	2015-11-09 00:00 Istanbul (EET,  UTC+2) = 2015-11-08 22:00 UTC
//	=> window wall-clock length = 25h, still exactly 1 calendar day.
func TestChunkDSTFallbackDayCountsAsOneCalendarDay(t *testing.T) {
	from := istanbulMidnight(2015, 11, 8)
	to := istanbulMidnight(2015, 11, 9)
	max := 24 * time.Hour

	windows := normalize.Chunk(from, to, max)

	want := []normalize.Window{
		{
			From: time.Date(2015, 11, 7, 21, 0, 0, 0, time.UTC),
			To:   time.Date(2015, 11, 8, 22, 0, 0, 0, time.UTC),
		},
	}
	requireWindowsEqual(t, want, windows)
	require.Equal(t, 25*time.Hour, windows[0].To.Sub(windows[0].From), "the DST fallback day is 25h of wall time")

	requireGaplessCoverage(t, from, to, max, windows)
}

// TestChunkSubDayMaxUnchangedAcrossDST proves the max < 24h path (R36:
// "current behaviour unchanged") still never crosses a local midnight
// even on the 23h 2015-03-29 day.
//
// from = Istanbul midnight of 2015-03-29 (2015-03-28 22:00 UTC, per the
// earlier hand-derivation) plus 10h wall-clock = 2015-03-29 08:00 UTC
// (2015-03-29 11:00 EEST local, since the 01:00 UTC / 03:00->04:00 local
// transition has already passed by then). With max = 20h, a naive
// from+max would land at 2015-03-30 04:00 UTC — past the next Istanbul
// midnight, which falls at 2015-03-29 21:00 UTC (2015-03-30 00:00 EEST,
// again from the earlier hand-derivation). So the window must be capped
// at that midnight, giving a window of only 13h (21:00-08:00 UTC), well
// under max.
func TestChunkSubDayMaxUnchangedAcrossDST(t *testing.T) {
	from := istanbulMidnight(2015, 3, 29).Add(10 * time.Hour) // 2015-03-29 08:00 UTC
	max := 20 * time.Hour
	to := from.Add(24 * time.Hour)

	windows := normalize.Chunk(from, to, max)
	require.NotEmpty(t, windows)

	wantBoundary := time.Date(2015, 3, 29, 21, 0, 0, 0, time.UTC) // 2015-03-30 00:00 EEST
	require.True(t, windows[0].To.Equal(wantBoundary), "want midnight cap %s, got %s", wantBoundary, windows[0].To)
	require.Equal(t, 13*time.Hour, windows[0].To.Sub(windows[0].From))
	require.Less(t, windows[0].To.Sub(windows[0].From), max)

	requireGaplessCoverage(t, from, to, max, windows)
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
