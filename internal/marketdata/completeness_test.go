package marketdata_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/marketdata"
)

// TestMissingHoursUsesIstanbulDays pins that MissingHours buckets by the
// Istanbul-local calendar day, not by the UTC day the instant happens to
// fall on: 2026-09-14 local midnight IS 2026-09-13T21:00:00Z, so a
// UTC-bucketed implementation would file every one of that day's hours
// under the key "2026-09-13" instead of "2026-09-14".
func TestMissingHoursUsesIstanbulDays(t *testing.T) {
	from := time.Date(2026, 9, 14, 0, 0, 0, 0, normalize.Istanbul).UTC()
	to := time.Date(2026, 9, 15, 0, 0, 0, 0, normalize.Istanbul).UTC()
	require.Equal(t, "2026-09-13T21:00:00Z", from.Format(time.RFC3339))

	// 01:00 Istanbul local on the 14th is 22:00 UTC on the 13th — a
	// UTC-bucketed implementation would file it under "2026-09-13", not
	// "2026-09-14", so this specific hour (unlike a mid-day one) actually
	// discriminates the two bucketing strategies.
	missingHour := time.Date(2026, 9, 14, 1, 0, 0, 0, normalize.Istanbul).UTC()

	var prices []model.MarketPrice
	for h := from; h.Before(to); h = h.Add(time.Hour) {
		if h.Equal(missingHour) {
			continue
		}
		prices = append(prices, model.MarketPrice{Ts: h, PTF: decimal.RequireFromString("100.0000")})
	}

	got := marketdata.MissingHours(prices, from, to)
	require.Len(t, got, 1)
	missing, ok := got["2026-09-14"]
	require.Truef(t, ok, "the missing hour must be keyed under its Istanbul-local day (2026-09-14), not the UTC day its instant falls on: got keys %v", keysOf(got))
	require.Equal(t, []time.Time{missingHour}, missing)
}

// TestMissingHoursPre2016DSTDays pins R6 against Go's own tzdata (verified
// live via `go run`, see task-12-report.md): 2015-03-29 is a 23-hour
// spring-forward day, 2015-10-25 is an ORDINARY 24-hour day (Turkey's
// government postponed that year's scheduled fall-back by decree), and
// 2015-11-08 is the actual, delayed 25-hour fall-back day. All three are
// asserted together so this test documents the anomaly rather than landing
// on a lucky pair.
func TestMissingHoursPre2016DSTDays(t *testing.T) {
	for _, tc := range []struct {
		date string
		want int
	}{
		{"2015-03-29", 23},
		{"2015-10-25", 24},
		{"2015-11-08", 25},
	} {
		t.Run(tc.date, func(t *testing.T) {
			day, err := time.ParseInLocation("2006-01-02", tc.date, normalize.Istanbul)
			require.NoError(t, err)
			from := day.UTC()
			to := time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, normalize.Istanbul).UTC()

			got := marketdata.MissingHours(nil, from, to)
			require.Len(t, got[tc.date], tc.want)
		})
	}
}

// TestMissingHoursOmitsFullyCoveredDays: a day with no gaps is absent from
// the result map entirely, not present with an empty slice.
func TestMissingHoursOmitsFullyCoveredDays(t *testing.T) {
	from := time.Date(2026, 1, 5, 0, 0, 0, 0, normalize.Istanbul).UTC()
	to := time.Date(2026, 1, 6, 0, 0, 0, 0, normalize.Istanbul).UTC()

	var prices []model.MarketPrice
	for h := from; h.Before(to); h = h.Add(time.Hour) {
		prices = append(prices, model.MarketPrice{Ts: h, PTF: decimal.RequireFromString("100.0000")})
	}

	got := marketdata.MissingHours(prices, from, to)
	require.Empty(t, got)
}

// TestMissingHoursClipsToWindow: an hour outside [from, to) is never
// reported as missing even though it falls on a day the window touches.
func TestMissingHoursClipsToWindow(t *testing.T) {
	dayStart := time.Date(2026, 1, 5, 0, 0, 0, 0, normalize.Istanbul).UTC()
	from := dayStart.Add(6 * time.Hour) // 06:00 local, mid-day
	to := dayStart.Add(12 * time.Hour)  // 12:00 local

	got := marketdata.MissingHours(nil, from, to)
	require.Len(t, got["2026-01-05"], 6) // hours 06..11, not 00..23
}

func keysOf(m map[string][]time.Time) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
