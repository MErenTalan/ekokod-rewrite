package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// TestTimeRangeValid pins the one gate every range-taking repository method
// passes through before it touches the database. An invalid range that got
// through would be either an unbounded hypertable scan (a zero end) or a
// silently empty result for what is really a caller bug (an empty or inverted
// window).
func TestTimeRangeValid(t *testing.T) {
	from := time.Date(2026, time.March, 1, 10, 0, 0, 0, time.UTC)
	// A fixed +03:00 zone rather than LoadLocation("Europe/Istanbul"), so the
	// test does not depend on the host's tzdata. Only the offset matters.
	istanbul := time.FixedZone("TRT", 3*60*60)

	cases := []struct {
		name  string
		r     store.TimeRange
		valid bool
	}{
		{"zero From", store.TimeRange{To: from.Add(time.Hour)}, false},
		{"zero To", store.TimeRange{From: from}, false},
		{"both zero", store.TimeRange{}, false},
		{"zero From expressed in a non-UTC location", store.TimeRange{From: time.Time{}.In(istanbul), To: from}, false},
		{"From equal to To", store.TimeRange{From: from, To: from}, false},
		{"From after To", store.TimeRange{From: from.Add(time.Hour), To: from}, false},
		{"valid one-hour window", store.TimeRange{From: from, To: from.Add(time.Hour)}, true},
		{
			// 10:00 UTC to 13:30+03:00 (10:30 UTC): ordered as instants.
			"ordered window whose ends carry different locations",
			store.TimeRange{From: from, To: from.Add(30 * time.Minute).In(istanbul)},
			true,
		},
		{
			// 10:00 UTC and 13:00+03:00 are the SAME instant: empty window.
			"same instant in different locations",
			store.TimeRange{From: from, To: from.In(istanbul)},
			false,
		},
		{
			// 12:30+03:00 is 09:30 UTC: the wall clock reads later, the
			// instant is earlier. Comparing wall clocks would accept this.
			"wall clock later but instant earlier",
			store.TimeRange{From: from, To: from.Add(-30 * time.Minute).In(istanbul)},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.valid, tc.r.Valid(), "TimeRange{From: %v, To: %v}", tc.r.From, tc.r.To)
		})
	}
}
