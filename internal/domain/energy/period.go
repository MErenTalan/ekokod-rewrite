package energy

import (
	"time"

	// The zone database must be linked into every binary that resolves
	// Europe/Istanbul, including a scratch container with no system tzdata
	// installed. time.LoadLocation falls back to this embedded copy when the
	// OS has none.
	_ "time/tzdata"
)

// Bucket returns the level-sized bucket containing ts, evaluated in loc
// (02 §3.4). Boundaries come from the tz database, never a fixed offset:
// ts is converted to loc's wall-clock calendar first, and the bucket's From
// and To are built with time.Date from THAT calendar's fields. time.Date
// resolves the correct absolute instant for whatever wall-clock fields it is
// given, in loc's own historical offset at that wall time — never a fixed
// +03:00 — so a bucket spanning a historical DST transition (pre-2016) comes
// out 23 or 25 hours long, not always 24 (see
// TestBucketDailyCrossesAHistoricalDSTTransition).
func Bucket(level Level, ts time.Time, loc *time.Location) Window {
	local := ts.In(loc)
	year, month, day := local.Date()
	hour := local.Hour()

	switch level {
	case Hourly:
		from := time.Date(year, month, day, hour, 0, 0, 0, loc)
		to := time.Date(year, month, day, hour+1, 0, 0, 0, loc)
		return Window{From: from, To: to}
	case Daily:
		from := time.Date(year, month, day, 0, 0, 0, 0, loc)
		to := time.Date(year, month, day+1, 0, 0, 0, 0, loc)
		return Window{From: from, To: to}
	case Monthly:
		// time.Date(y, m, 1, ...) — never time.Hour*24*30 — because a month
		// is a calendar concept, not a fixed duration.
		from := time.Date(year, month, 1, 0, 0, 0, 0, loc)
		to := time.Date(year, month+1, 1, 0, 0, 0, 0, loc)
		return Window{From: from, To: to}
	case Yearly:
		from := time.Date(year, time.January, 1, 0, 0, 0, 0, loc)
		to := time.Date(year+1, time.January, 1, 0, 0, 0, 0, loc)
		return Window{From: from, To: to}
	default:
		return Window{}
	}
}

// Buckets returns every level-sized bucket intersecting w, ascending.
func Buckets(level Level, w Window, loc *time.Location) []Window {
	if !w.Valid() {
		return nil
	}
	var out []Window
	cur := Bucket(level, w.From, loc)
	for cur.From.Before(w.To) {
		out = append(out, cur)
		cur = Bucket(level, cur.To, loc)
	}
	return out
}
