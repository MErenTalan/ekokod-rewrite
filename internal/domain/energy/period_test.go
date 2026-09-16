package energy_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

func TestBucketMonthlyUsesIstanbulCalendar(t *testing.T) {
	loc := istanbul(t)
	// 2025-03-01T00:00+03:00 is 2025-02-28T21:00Z: a UTC-based implementation
	// would put this instant in February.
	ts := time.Date(2025, 2, 28, 22, 30, 0, 0, time.UTC)
	got := energy.Bucket(energy.Monthly, ts, loc)
	require.True(t, time.Date(2025, 3, 1, 0, 0, 0, 0, loc).Equal(got.From), "got %s", got.From)
	require.True(t, time.Date(2025, 4, 1, 0, 0, 0, 0, loc).Equal(got.To), "got %s", got.To)
}

func TestBucketMonthlyInAPre2016WinterUsesTheTzOffset(t *testing.T) {
	loc := istanbul(t)
	// 2015-01-01T00:30+02:00 (pre-2016 Istanbul winter was a real +02:00,
	// not the fixed +03:00 Türkiye adopted permanently in Sept 2016, and
	// not UTC). Both a fixed-+03 construction and a UTC construction stay
	// green against the post-2016 fixture above; this one catches both.
	ts := time.Date(2014, 12, 31, 22, 30, 0, 0, time.UTC)
	got := energy.Bucket(energy.Monthly, ts, loc)
	want := energy.Window{
		From: time.Date(2014, 12, 31, 22, 0, 0, 0, time.UTC),
		To:   time.Date(2015, 1, 31, 22, 0, 0, 0, time.UTC),
	}
	require.True(t, want.From.Equal(got.From), "got %s", got.From)
	require.True(t, want.To.Equal(got.To), "got %s", got.To)
}

func TestBucketYearlyUsesIstanbulCalendar(t *testing.T) {
	loc := istanbul(t)
	// 2025-01-01T00:30+03:00 (post-2016, permanent +03). A yearly bucket
	// computed entirely in UTC would put this instant in 2024.
	ts := time.Date(2024, 12, 31, 21, 30, 0, 0, time.UTC)
	got := energy.Bucket(energy.Yearly, ts, loc)
	want := energy.Window{
		From: time.Date(2024, 12, 31, 21, 0, 0, 0, time.UTC),
		To:   time.Date(2025, 12, 31, 21, 0, 0, 0, time.UTC),
	}
	require.True(t, want.From.Equal(got.From), "got %s", got.From)
	require.True(t, want.To.Equal(got.To), "got %s", got.To)
}

func TestBucketYearlyInAPre2016WinterUsesTheTzOffset(t *testing.T) {
	loc := istanbul(t)
	// Same pre-2016 +02:00 fixture as the monthly case above: a yearly
	// bucket computed in UTC would put this instant's year boundary at
	// 2015-01-01T00:00Z instead of the real Istanbul local midnight.
	ts := time.Date(2014, 12, 31, 22, 30, 0, 0, time.UTC)
	got := energy.Bucket(energy.Yearly, ts, loc)
	want := time.Date(2014, 12, 31, 22, 0, 0, 0, time.UTC)
	require.True(t, want.Equal(got.From), "got %s", got.From)
}

func TestBucketDailyCrossesAHistoricalDSTTransition(t *testing.T) {
	loc := istanbul(t)
	// 2015-03-29: Türkiye still observed DST; the local day is 23 hours long.
	w := energy.Bucket(energy.Daily, time.Date(2015, 3, 29, 12, 0, 0, 0, loc), loc)
	require.Equal(t, 23*time.Hour, w.To.Sub(w.From))
}

// TestBucketHourlyAcrossTheHistoricalFallBack pins I-1: every instant across
// the 2015-11-08 fall-back (when the local hour 00:00-01:00+02 repeats)
// must lie inside its own bucket, and every bucket must be exactly 1 hour
// wide. Rebuilding the hour from loc's wall-clock fields, as every other
// level does, fails this: time.Date picks the LATER of the two occurrences
// of a repeated local hour, so the bucket for the FIRST occurrence would not
// even contain ts.
func TestBucketHourlyAcrossTheHistoricalFallBack(t *testing.T) {
	loc := istanbul(t)
	start := time.Date(2015, 11, 7, 22, 0, 0, 0, time.UTC)
	end := time.Date(2015, 11, 8, 3, 0, 0, 0, time.UTC)
	for ts := start; !ts.After(end); ts = ts.Add(15 * time.Minute) {
		w := energy.Bucket(energy.Hourly, ts, loc)
		require.True(t, w.Contains(ts), "%s not contained in its own bucket [%s, %s)", ts, w.From, w.To)
		require.Equal(t, time.Hour, w.To.Sub(w.From), "bucket for %s is %s wide, not 1h", ts, w.To.Sub(w.From))
	}
}

// TestBucketsRejectsAnUnknownLevel pins I-2: Bucket returns the zero Window
// for an unrecognised level, and Buckets must not turn that into an
// infinite loop (the zero Window's From is always before any real w.To, and
// Bucket of the same unknown level is the zero Window again on every
// iteration).
func TestBucketsRejectsAnUnknownLevel(t *testing.T) {
	loc := istanbul(t)
	require.Nil(t, energy.Buckets(energy.Level("weekly"), win(t0, time.Hour), loc))
}

func TestBucketsRejectsAnInvalidWindow(t *testing.T) {
	loc := istanbul(t)
	require.Nil(t, energy.Buckets(energy.Daily, win(t0, 0), loc), "empty window (From == To)")
	require.Nil(t, energy.Buckets(energy.Daily, energy.Window{From: t0.Add(time.Hour), To: t0}, loc), "inverted window")
}

// TestBucketsFromMidBucketToAnExactBoundary pins I-9: a range starting
// mid-bucket and ending exactly on a bucket boundary must include the
// bucket the range starts inside of (even though its own From precedes
// w.From) and must exclude the bucket that would start exactly at w.To.
func TestBucketsFromMidBucketToAnExactBoundary(t *testing.T) {
	loc := istanbul(t)
	w := energy.Window{
		From: time.Date(2025, 3, 15, 12, 0, 0, 0, loc),
		To:   time.Date(2025, 5, 1, 0, 0, 0, 0, loc),
	}
	buckets := energy.Buckets(energy.Monthly, w, loc)
	require.Len(t, buckets, 2, "expected exactly March and April, no May")

	march := energy.Window{From: time.Date(2025, 3, 1, 0, 0, 0, 0, loc), To: time.Date(2025, 4, 1, 0, 0, 0, 0, loc)}
	april := energy.Window{From: time.Date(2025, 4, 1, 0, 0, 0, 0, loc), To: time.Date(2025, 5, 1, 0, 0, 0, 0, loc)}

	require.True(t, buckets[0].From.Equal(march.From))
	require.True(t, buckets[0].To.Equal(march.To))
	require.True(t, buckets[0].From.Before(w.From), "the first bucket's From must precede w.From")
	require.True(t, buckets[1].From.Equal(april.From))
	require.True(t, buckets[1].To.Equal(april.To))
	require.True(t, buckets[1].To.Equal(w.To), "the range ends exactly on the April/May boundary")
}

// TestBucketsAcrossAMonthBoundaryAtDailyLevel pins the third I-9 scenario:
// a Daily range spanning a calendar month boundary.
func TestBucketsAcrossAMonthBoundaryAtDailyLevel(t *testing.T) {
	loc := istanbul(t)
	w := energy.Window{
		From: time.Date(2025, 2, 27, 0, 0, 0, 0, loc),
		To:   time.Date(2025, 3, 2, 0, 0, 0, 0, loc),
	}
	buckets := energy.Buckets(energy.Daily, w, loc)
	require.Len(t, buckets, 3, "Feb 27, Feb 28, Mar 1")
	require.True(t, buckets[0].From.Equal(time.Date(2025, 2, 27, 0, 0, 0, 0, loc)))
	require.True(t, buckets[1].From.Equal(time.Date(2025, 2, 28, 0, 0, 0, 0, loc)))
	require.True(t, buckets[2].From.Equal(time.Date(2025, 3, 1, 0, 0, 0, 0, loc)))
	require.True(t, buckets[2].To.Equal(w.To))
	for i := 0; i < len(buckets)-1; i++ {
		require.True(t, buckets[i].To.Equal(buckets[i+1].From), "buckets must be contiguous")
	}
}

// TestBucketsDailyThreeDayRangeAcrossTheHistoricalSpringForward covers a
// 3-day Daily range containing the 2015-03-29 23-hour day: widths 24h, 23h,
// 24h, and the buckets stay contiguous despite the irregular width.
func TestBucketsDailyThreeDayRangeAcrossTheHistoricalSpringForward(t *testing.T) {
	loc := istanbul(t)
	w := energy.Window{
		From: time.Date(2015, 3, 28, 0, 0, 0, 0, loc),
		To:   time.Date(2015, 3, 31, 0, 0, 0, 0, loc),
	}
	buckets := energy.Buckets(energy.Daily, w, loc)
	require.Len(t, buckets, 3)
	require.Equal(t, 24*time.Hour, buckets[0].To.Sub(buckets[0].From))
	require.Equal(t, 23*time.Hour, buckets[1].To.Sub(buckets[1].From))
	require.Equal(t, 24*time.Hour, buckets[2].To.Sub(buckets[2].From))
	require.True(t, buckets[0].To.Equal(buckets[1].From))
	require.True(t, buckets[1].To.Equal(buckets[2].From))
}

func TestWindowContainsBoundaries(t *testing.T) {
	w := win(t0, time.Hour)
	require.True(t, w.Contains(w.From), "From is inside the half-open window")
	require.False(t, w.Contains(w.To), "To is exclusive")
	require.True(t, w.Contains(w.To.Add(-time.Nanosecond)), "To minus 1ns is the last instant inside")
}

func TestWindowValid(t *testing.T) {
	require.True(t, win(t0, time.Hour).Valid())
	require.False(t, win(t0, 0).Valid(), "From == To is not valid")
	require.False(t, energy.Window{From: t0.Add(time.Hour), To: t0}.Valid(), "inverted window is not valid")
}
