package marketdata

import (
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// MissingHours returns, per Istanbul-local day in [from, to), the expected
// hour starts absent from prices, keyed "2006-01-02".
//
// "Expected hour starts" is computed against Go's own tzdata (via
// normalize.Istanbul), never a hardcoded 24: a day is enumerated by
// stepping one absolute hour at a time from that day's Istanbul-local
// midnight up to (but not including) the next day's Istanbul-local
// midnight, so a spring-forward day naturally yields 23 starts and a
// fall-back day 25 (R6) — the elapsed real duration between two
// consecutive local midnights, as Go's time.Date computes it, IS that
// day's hour count; nothing here special-cases a DST transition.
//
// A day with zero missing hours is not present in the result map at all
// (an empty slice would be indistinguishable from "one day, no gaps" only
// by inspecting len(), which every caller would have to remember to do;
// omitting the key removes the ambiguity).
func MissingHours(prices []model.MarketPrice, from, to time.Time) map[string][]time.Time {
	present := make(map[time.Time]bool, len(prices))
	for _, p := range prices {
		present[p.Ts.UTC()] = true
	}

	result := make(map[string][]time.Time)
	if !from.Before(to) {
		return result
	}

	local := from.In(normalize.Istanbul)
	y, m, d := local.Date()
	dayStart := time.Date(y, m, d, 0, 0, 0, 0, normalize.Istanbul)

	for dayStart.Before(to) {
		nextDay := time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day()+1, 0, 0, 0, 0, normalize.Istanbul)

		var missing []time.Time
		for h := dayStart; h.Before(nextDay); h = h.Add(time.Hour) {
			hUTC := h.UTC()
			if hUTC.Before(from) || !hUTC.Before(to) {
				continue // clip to the requested half-open window
			}
			if !present[hUTC] {
				missing = append(missing, hUTC)
			}
		}
		if len(missing) > 0 {
			result[dayStart.Format("2006-01-02")] = missing
		}
		dayStart = nextDay
	}
	return result
}
