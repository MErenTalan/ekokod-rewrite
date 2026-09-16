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
//
// Hourly is the one level that does NOT reconstruct the local wall clock:
// it truncates ts to the UTC hour instead. Two facts make this both correct
// and necessary. First, this is what migration 00005's
// `time_bucket('1 hour', ts)` does to consumption_hourly, so Bucket(Hourly,
// …) and the analytics path key the exact same instant into the exact same
// hour — including at a pre-2016 DST fall-back, where the local hour
// repeats and time.Date(y, m, d, hour, …, loc) picks the LATER of the two
// occurrences, silently producing a bucket that does not even contain ts
// (see TestBucketHourlyAcrossTheHistoricalFallBack). Second, this is safe
// for Istanbul specifically because every one of its historical UTC offsets
// has been a whole number of hours (+01, +02, +03; no half-hour offset was
// ever in effect), so a UTC top-of-hour is always also an Istanbul
// top-of-hour: truncating in UTC never produces a bucket boundary that
// falls mid-hour on the local wall clock.
func Bucket(level Level, ts time.Time, loc *time.Location) Window {
	if level == Hourly {
		from := ts.UTC().Truncate(time.Hour)
		return Window{From: from, To: from.Add(time.Hour)}
	}

	local := ts.In(loc)
	year, month, day := local.Date()

	switch level {
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

// Buckets returns every level-sized bucket intersecting w, ascending. It
// returns nil for an invalid window (w.Valid() false) or an unrecognised
// level: Bucket returns the zero Window for any level it does not
// recognise, and the zero Window is itself !Valid() (From == To), so
// checking the first bucket catches a bad level before the loop below ever
// runs. Without this, an unknown level would make the loop append the zero
// Window forever — its From (the zero time.Time) is always before w.To,
// and Bucket of a zero level is the zero Window again — growing without
// bound until the process runs out of memory.
func Buckets(level Level, w Window, loc *time.Location) []Window {
	if !w.Valid() {
		return nil
	}
	cur := Bucket(level, w.From, loc)
	if !cur.Valid() {
		return nil
	}
	var out []Window
	for cur.From.Before(w.To) {
		out = append(out, cur)
		next := Bucket(level, cur.To, loc)
		if !next.From.After(cur.From) {
			// Belt-and-braces: a step that fails to advance would loop
			// forever otherwise. Every real level advances strictly, so
			// this only ever fires in the already-excluded unknown-level
			// case, but the guard above should never be the sole thing
			// standing between a bad Level value and a hung worker.
			break
		}
		cur = next
	}
	return out
}
