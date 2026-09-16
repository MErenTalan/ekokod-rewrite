package energy

import "time"

// SelectBoundary returns the last reading with TS <= bound (02 §3.1's
// boundary-selection rule: "the last reading with ts <= start/end"), or nil
// when no reading qualifies.
//
// Precondition: readings is sorted ascending by TS. SelectBoundary does not
// sort — sorting on every call would make repeated boundary selection over
// the same slice quadratic — and it has no opinion on Kind: the caller has
// already filtered readings to the kind(s) it wants considered (R64; I-17).
func SelectBoundary(readings []Reading, bound time.Time) *Reading {
	var out *Reading
	for i := range readings {
		if readings[i].TS.After(bound) {
			break
		}
		out = &readings[i]
	}
	return out
}
