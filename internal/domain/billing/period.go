package billing

import (
	"fmt"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// Period implements 02 §5 for key "YYYY-MM" (R135: spec label).
func Period(periodKey string, cutoffDay int, loc *time.Location) (energy.Window, error) {
	year, month, ok := parseKey(periodKey)
	if !ok {
		return energy.Window{}, fmt.Errorf("%w: period key %q", ErrInvalidInput, periodKey)
	}
	if cutoffDay < 1 || cutoffDay > 31 {
		return energy.Window{}, fmt.Errorf("%w: cut-off day %d", ErrInvalidInput, cutoffDay)
	}
	at := func(y int, m time.Month) time.Time {
		last := time.Date(y, m+1, 0, 0, 0, 0, 0, loc).Day()
		return time.Date(y, m, min(cutoffDay, last), 0, 0, 0, 0, loc)
	}
	from := at(year, time.Month(month))
	next := time.Date(year, time.Month(month)+1, 1, 0, 0, 0, 0, loc)
	return energy.Window{From: from, To: at(next.Year(), next.Month())}, nil
}

// DaysInPeriod is the whole Istanbul calendar days from From to To, never +1.
func DaysInPeriod(w energy.Window, loc *time.Location) int {
	date := func(ts time.Time) time.Time {
		y, m, d := ts.In(loc).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	return int(date(w.To).Sub(date(w.From)) / (24 * time.Hour))
}

// LatestClosedPeriodKey is the latest key whose period end + settle ≤ now.
func LatestClosedPeriodKey(cutoffDay int, now time.Time, settle time.Duration, loc *time.Location) string {
	cur := now.In(loc)
	m := time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, loc)
	for {
		key := fmt.Sprintf("%04d-%02d", m.Year(), int(m.Month()))
		w, err := Period(key, cutoffDay, loc)
		if err == nil && !w.To.Add(settle).After(now) {
			return key
		}
		m = m.AddDate(0, -1, 0)
	}
}

func parseKey(key string) (year, month int, ok bool) {
	if len(key) != 7 || key[4] != '-' {
		return 0, 0, false
	}
	for i, c := range key {
		if i != 4 && (c < '0' || c > '9') {
			return 0, 0, false
		}
	}
	year = int(key[0]-'0')*1000 + int(key[1]-'0')*100 + int(key[2]-'0')*10 + int(key[3]-'0')
	month = int(key[5]-'0')*10 + int(key[6]-'0')
	return year, month, year >= 1900 && month >= 1 && month <= 12
}
