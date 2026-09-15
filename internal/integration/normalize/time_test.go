package normalize_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// TestOffsetlessTimestampIsIstanbulLocal is the unit half of the F2
// acceptance criterion "an offset-less provider timestamp is stored as the
// correct UTC instant for Europe/Istanbul". 2015-03-29 crosses the last
// Turkish DST change (Turkey has used permanent UTC+3 since 2016), proving
// the tz database, not a fixed +03:00, is used: 2015-01-10 is winter
// (UTC+2) and 2015-07-10 is summer (UTC+3).
func TestOffsetlessTimestampIsIstanbulLocal(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want time.Time
	}{
		{"2026-09-14T10:15:00", time.Date(2026, 9, 14, 7, 15, 0, 0, time.UTC)},
		{"2026-09-14T10:15:00Z", time.Date(2026, 9, 14, 10, 15, 0, 0, time.UTC)},
		{"2026-09-14T10:15:00+02:00", time.Date(2026, 9, 14, 8, 15, 0, 0, time.UTC)},
		{"2015-01-10T10:00:00", time.Date(2015, 1, 10, 8, 0, 0, 0, time.UTC)}, // winter, UTC+2
		{"2015-07-10T10:00:00", time.Date(2015, 7, 10, 7, 0, 0, 0, time.UTC)}, // summer, UTC+3
	} {
		got, err := normalize.ISO8601(tc.raw)
		require.NoError(t, err, tc.raw)
		require.True(t, tc.want.Equal(got), "%s: want %s got %s", tc.raw, tc.want, got)
		require.Equal(t, time.UTC, got.Location())
	}
}

// TestOSOSDateAcrossFallback covers the 2015-11-08 Turkish fall-back (the
// last one; Turkey has had no DST since 2016 — NOT the usual late-October
// date). At 04:00 local on 2015-11-08 clocks were already back at UTC+2
// (the fall-back happened at 04:00, moving to 03:00), so both 02:30 local
// instances of that day are ambiguous; we pick a time clearly on the
// UTC+2 side and one clearly on the summer UTC+3 side the day before, to
// prove the tz database (not a fixed offset) drives the conversion.
func TestOSOSDateAcrossFallback(t *testing.T) {
	// 2015-11-07 12:00 Istanbul local was still summer time, UTC+3.
	gotBefore, err := normalize.OSOSDate("07/11/2015 12:00:00")
	require.NoError(t, err)
	require.True(t, time.Date(2015, 11, 7, 9, 0, 0, 0, time.UTC).Equal(gotBefore))

	// 2015-11-09 12:00 Istanbul local is after the fall-back, UTC+2.
	gotAfter, err := normalize.OSOSDate("09/11/2015 12:00:00")
	require.NoError(t, err)
	require.True(t, time.Date(2015, 11, 9, 10, 0, 0, 0, time.UTC).Equal(gotAfter))
}

func TestOSOSDateRoundTrip(t *testing.T) {
	for _, raw := range []string{
		"14/09/2026 10:15:00",
		"01/01/2016 00:00:00",
		"10/07/2015 23:59:59", // summer, historical DST
	} {
		t.Run(raw, func(t *testing.T) {
			parsed, err := normalize.OSOSDate(raw)
			require.NoError(t, err)
			require.Equal(t, time.UTC, parsed.Location())

			back := normalize.FormatOSOSDate(parsed)
			require.Equal(t, raw, back)
		})
	}
}

func TestARILProfileDate(t *testing.T) {
	// 2026-09-14 10:15:00 Istanbul local, UTC+3 (no DST since 2016) → 07:15 UTC.
	got, err := normalize.ARILProfileDate(20260914101500)
	require.NoError(t, err)
	require.True(t, time.Date(2026, 9, 14, 7, 15, 0, 0, time.UTC).Equal(got))
	require.Equal(t, time.UTC, got.Location())

	_, err = normalize.ARILProfileDate(2026091410150) // 13 digits
	require.Error(t, err)
}

func TestPM5340DateFormats(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want time.Time
	}{
		{"2026-09-14T10:15:00Z", time.Date(2026, 9, 14, 10, 15, 0, 0, time.UTC)},
		{"2026-09-14T10:15:00", time.Date(2026, 9, 14, 7, 15, 0, 0, time.UTC)},
		{"14/09/2026 10:15", time.Date(2026, 9, 14, 7, 15, 0, 0, time.UTC)},
		{"14/09/2026 10:15:30", time.Date(2026, 9, 14, 7, 15, 30, 0, time.UTC)},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := normalize.PM5340Date(tc.raw)
			require.NoError(t, err)
			require.True(t, tc.want.Equal(got), "%s: want %s got %s", tc.raw, tc.want, got)
			require.Equal(t, time.UTC, got.Location())
		})
	}
}

func TestPM5340DateInvalid(t *testing.T) {
	_, err := normalize.PM5340Date("not-a-date")
	require.Error(t, err)
}
