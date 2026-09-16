package energy

import (
	"sort"
	"time"
)

// SelectBoundary returns the last reading with TS <= bound (02 §3.1's
// boundary-selection rule: "the last reading with ts <= start/end"), or nil
// when no reading qualifies.
//
// Precondition: readings is sorted ascending by TS. SelectBoundary does not
// sort — sorting on every call would make repeated boundary selection over
// the same slice quadratic — and it has no opinion on Kind: the caller has
// already filtered readings to the kind(s) it wants considered (R64; I-17).
// An unsorted slice gives an unspecified result.
//
// I-2: the lookup is a binary search (sort.Search for the first reading
// after bound, then step back one) rather than a linear scan — over a
// billing year of 15-minute readings and hourly bounds, the linear version
// cost about 1.75s of CPU for one request; BenchmarkBillingYearHourly
// records the current cost. Ties: when several readings share the same
// TS <= bound, this returns the LAST of them in the slice — the same
// reading the linear scan it replaces would have returned.
//
// The returned pointer aliases readings: it points into the caller's
// slice, not a copy. A caller that mutates *readings after calling this
// also mutates the returned Reading.
func SelectBoundary(readings []Reading, bound time.Time) *Reading {
	i := sort.Search(len(readings), func(i int) bool { return readings[i].TS.After(bound) })
	if i == 0 {
		return nil
	}
	return &readings[i-1]
}
