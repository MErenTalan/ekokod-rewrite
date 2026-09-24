package ingest

import (
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// NegativeDelta is one register's decrease between two consecutive readings
// of the same kind, not explained by an intervening meter reset (R14).
type NegativeDelta struct {
	Register      string
	PrevTs, CurTs time.Time
}

// DetectNegativeDeltas implements R14. It walks the sequence
// [prev]+rows — prev may be nil, meaning "no reading before rows at all" —
// comparing each consecutive PAIR of readings that share a Kind. For each of
// cumulativeRegisters, a pair whose later value is less than its earlier
// value is a negative delta UNLESS a reading in resets (assumed to already
// be kind=reset; the caller is what filters that, see Service) has a Ts in
// (pair.PrevTs, pair.CurTs] — strictly after the earlier reading and at or
// before the later one. The suppression is evaluated per PAIR, not per
// register: a reset inside one pair's window suppresses every register's
// delta for THAT pair only, and has no effect on any other pair.
//
// A nil register value on either side of a pair never produces a delta
// (removed-behaviour 21: nil means "not reported", not zero).
//
// The readings themselves are never rejected here — R14 says "the reading is
// stored" regardless; DetectNegativeDeltas only reports where an anomaly
// record belongs. Turning that into a stored consumption_anomalies row,
// including the "does an equivalent one already exist" check, is Service's
// job (06 §9 / R14's second half).
func DetectNegativeDeltas(prev *model.MeterReading, rows []model.MeterReading, resets []model.MeterReading) []NegativeDelta {
	seq := make([]model.MeterReading, 0, len(rows)+1)
	if prev != nil {
		seq = append(seq, *prev)
	}
	seq = append(seq, rows...)

	var out []NegativeDelta
	for i := 1; i < len(seq); i++ {
		p, c := seq[i-1], seq[i]
		if p.Kind != c.Kind {
			continue
		}
		suppressed := resetBetween(resets, p.Ts, c.Ts)
		for _, reg := range cumulativeRegisters {
			pv, cv := reg.get(p), reg.get(c)
			if pv == nil || cv == nil {
				continue
			}
			if cv.LessThan(*pv) && !suppressed {
				out = append(out, NegativeDelta{Register: reg.name, PrevTs: p.Ts, CurTs: c.Ts})
			}
		}
	}
	return out
}

// resetBetween reports whether any reading in resets has a Ts strictly
// after from and at or before to.
func resetBetween(resets []model.MeterReading, from, to time.Time) bool {
	for _, r := range resets {
		if r.Ts.After(from) && !r.Ts.After(to) {
			return true
		}
	}
	return false
}
