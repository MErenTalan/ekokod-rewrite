package normalize

import "time"

// Window is a half-open time range [From, To). It is deliberately a plain
// struct, not job.Window (Task 1): normalize has no F2-task dependency and
// must stay that way regardless of wave order. Callers that need
// job.Window (backfill, Task 15) convert with one field-for-field copy.
type Window struct {
	From, To time.Time
}

// Chunk splits [from, to) into consecutive half-open windows aligned to
// Europe/Istanbul local midnight so a window never splits a local day (06
// §2 "requests to one provider are serialised per company"; windows are
// also the pagination unit for OSOS, GridBox and EPİAŞ, R20). Boundaries
// always land on an Istanbul local midnight, except that the very first
// window may start mid-day at from (R36).
//
// The size of a window depends on max:
//
//   - max < 24h: sub-day behaviour, unchanged — each window is capped at
//     max and never crosses a local midnight, so it may be shorter than
//     max when a midnight falls first.
//   - max >= 24h: each window covers as many whole Istanbul calendar days
//     as fit in max, counting max in CALENDAR days (floor(max / 24h); a
//     23h or 25h DST-transition day still counts as one day), truncated
//     by to. The day containing cur — even when cur starts mid-day —
//     counts as the first of those days, so e.g. max = 24h still produces
//     one boundary per local midnight (unchanged from the prior
//     behaviour), and max = 30*24h produces a boundary every 30 calendar
//     days.
//
// Every boundary is computed from scratch via time.Date(y, m, d+n, 0, 0,
// 0, 0, Istanbul) on cur's Istanbul-local calendar date — never by adding
// a fixed duration — so a window never drifts off local midnight, and its
// day count is never off by one, across a DST change.
//
// An empty or inverted range ([from, to) with from >= to) yields no
// windows. A non-positive max is invalid and also yields no windows.
func Chunk(from, to time.Time, max time.Duration) []Window {
	if !from.Before(to) || max <= 0 {
		return nil
	}

	const day = 24 * time.Hour

	var windows []Window
	cur := from
	for cur.Before(to) {
		local := cur.In(Istanbul)
		y, m, d := local.Date()

		var end time.Time
		if max < day {
			end = time.Date(y, m, d+1, 0, 0, 0, 0, Istanbul)
			if capped := cur.Add(max); capped.Before(end) {
				end = capped
			}
		} else {
			days := int(max / day)
			end = time.Date(y, m, d+days, 0, 0, 0, 0, Istanbul)
		}
		if end.After(to) {
			end = to
		}

		windows = append(windows, Window{From: cur, To: end})
		cur = end
	}
	return windows
}
