package normalize

import "time"

// Window is a half-open time range [From, To). It is deliberately a plain
// struct, not job.Window (Task 1): normalize has no F2-task dependency and
// must stay that way regardless of wave order. Callers that need
// job.Window (backfill, Task 15) convert with one field-for-field copy.
type Window struct {
	From, To time.Time
}

// Chunk splits [from, to) into consecutive half-open windows of at most
// max, each aligned to Europe/Istanbul local midnight so a window never
// splits a local day (06 §2 "requests to one provider are serialised per
// company"; windows are also the pagination unit for OSOS, GridBox and
// EPİAŞ, R20).
//
// Every boundary after from is computed from scratch as the next
// Europe/Istanbul local midnight (time.Date(y, m, d+1, 0, 0, 0, 0,
// Istanbul)) — never by adding a fixed 24h duration — so a window never
// drifts off local midnight across a DST change.
//
// An empty or inverted range ([from, to) with from >= to) yields no
// windows. A non-positive max is invalid and also yields no windows.
func Chunk(from, to time.Time, max time.Duration) []Window {
	if !from.Before(to) || max <= 0 {
		return nil
	}

	var windows []Window
	cur := from
	for cur.Before(to) {
		local := cur.In(Istanbul)
		y, m, d := local.Date()
		midnight := time.Date(y, m, d+1, 0, 0, 0, 0, Istanbul)

		end := midnight
		if capped := cur.Add(max); capped.Before(end) {
			end = capped
		}
		if end.After(to) {
			end = to
		}

		windows = append(windows, Window{From: cur, To: end})
		cur = end
	}
	return windows
}
