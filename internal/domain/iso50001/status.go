package iso50001

import (
	"math"
	"time"
)

// DateRange is a main clause's planned period (inclusive civil dates).
type DateRange struct{ Start, End time.Time }

// ClauseDates is one row of the calendar (R332).
type ClauseDates struct {
	ClauseID   string
	Start, End time.Time
}

// Progress is R333: sub-clauses with a note or a file, over all of them.
func Progress(done map[string]bool) int {
	ids := SubIDs()
	n := 0
	for _, id := range ids {
		if done[id] {
			n++
		}
	}
	return int(math.Round(float64(n) * 100 / float64(len(ids))))
}

// Gantt is R334. A clause without dates has no status; the chart is
// available only when all five main clauses have both dates.
func Gantt(dates map[string]DateRange, done map[string]bool, today time.Time) (statuses map[string]string, available bool, start, end *time.Time) {
	statuses = map[string]string{}
	available = true
	day := civil(today)
	for _, m := range catalogue {
		r, ok := dates[m.ID]
		if !ok || r.Start.IsZero() || r.End.IsZero() {
			available = false
			continue
		}
		s, e := civil(r.Start), civil(r.End)
		if start == nil || s.Before(*start) {
			start = &s
		}
		if end == nil || e.After(*end) {
			end = &e
		}
		all, some := true, false
		for _, sub := range m.Subs {
			if done[sub.ID] {
				some = true
			} else {
				all = false
			}
		}
		switch {
		case all:
			statuses[m.ID] = "completed"
		case day.After(e):
			statuses[m.ID] = "expired"
		case day.Before(s):
			statuses[m.ID] = "not_started"
		case some:
			statuses[m.ID] = "in_progress"
		default:
			statuses[m.ID] = "not_started"
		}
	}
	if !available {
		start, end = nil, nil
	}
	return statuses, available, start, end
}

// ValidateDates is R332: every main clause once, each with both dates, start ≤ end.
func ValidateDates(rows []ClauseDates) map[string]string {
	seen := map[string]bool{}
	for _, r := range rows {
		if !IsMain(r.ClauseID) || seen[r.ClauseID] {
			return map[string]string{"clauses": "invalid"}
		}
		seen[r.ClauseID] = true
	}
	if len(seen) != len(MainIDs) {
		return map[string]string{"clauses": "incomplete"}
	}
	for _, r := range rows {
		if r.Start.IsZero() || r.End.IsZero() {
			return map[string]string{"clauses": "incomplete"}
		}
	}
	for _, r := range rows {
		if civil(r.Start).After(civil(r.End)) {
			return map[string]string{"clauses." + r.ClauseID: "start_after_end"}
		}
	}
	return nil
}

func civil(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
