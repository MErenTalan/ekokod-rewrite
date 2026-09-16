package loadprofile

import (
	"time"

	"github.com/shopspring/decimal"
)

// precisionScale is the decimal.DivRound scale used throughout this
// package: at least 20 fractional digits of PRECISION, not presentation
// (R79) — nothing computed here is ever rounded for display; that happens
// once, elsewhere, when a figure is written to an invoice line or shown to a
// user.
const precisionScale = 20

// HourValue is one hourly consumption figure. Day is the Istanbul-local
// calendar date the hour belongs to; Hour is 0-23 (R83).
type HourValue struct {
	Day   time.Time
	Hour  int
	Value decimal.Decimal
}

// Profile holds the mean consumption for each hour of the day, or nil for an
// hour no matching day reported (R68: values are active-import consumption;
// the package never names a register).
type Profile struct {
	Key   Key
	Hours [24]*decimal.Decimal
	Days  int // distinct days that contributed
}

// dayAccumulator sums and counts the values reported for each hour of one
// profile, and tracks the distinct calendar days that contributed to it.
type dayAccumulator struct {
	sums   [24]decimal.Decimal
	counts [24]int
	days   map[time.Time]struct{}
}

// Build groups values by the profile(s) each value's day belongs to
// (KeysFor, from Classify and SeasonOf) and, for every profile and every
// hour, averages the values reported for that hour across all the days that
// reported one. A day with no matching values contributes nothing to a
// profile; an hour no day reported stays nil — never decimal.Zero, which
// means an idle building that reported a measured zero (R68).
func Build(values []HourValue, cfg Config) map[Key]Profile {
	accumulators := map[Key]*dayAccumulator{}

	for _, v := range values {
		dt := Classify(v.Day, cfg)
		season := SeasonOf(v.Day, cfg.Location)
		day := calendarDate(v.Day, cfg.Location)

		for _, key := range KeysFor(dt, season) {
			acc, ok := accumulators[key]
			if !ok {
				acc = &dayAccumulator{days: map[time.Time]struct{}{}}
				accumulators[key] = acc
			}
			acc.sums[v.Hour] = acc.sums[v.Hour].Add(v.Value)
			acc.counts[v.Hour]++
			acc.days[day] = struct{}{}
		}
	}

	profiles := make(map[Key]Profile, len(accumulators))
	for key, acc := range accumulators {
		p := Profile{Key: key, Days: len(acc.days)}
		for h := 0; h < 24; h++ {
			if acc.counts[h] == 0 {
				continue // no day reported this hour: stays nil, not zero
			}
			// Precision, not presentation (R79): DivRound's scale bounds
			// how many fractional digits are kept, it does not round the
			// figure for display.
			mean := acc.sums[h].DivRound(decimal.NewFromInt(int64(acc.counts[h])), precisionScale)
			p.Hours[h] = &mean
		}
		profiles[key] = p
	}
	return profiles
}
