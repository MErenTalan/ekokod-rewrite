package energy_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// SelectBoundary is "the last reading with ts <= bound", over a slice
// already sorted ascending and already filtered to the kind(s) the caller
// wants. These tests probe the three offsets around a shared bound plus the
// two empty-result shapes.
func TestSelectBoundaryOneNanosecondBeforeTheBound(t *testing.T) {
	readings := []energy.Reading{
		*readingAt(t0, "10"),
		*readingAt(t0.Add(time.Hour), "20"),
	}
	got := energy.SelectBoundary(readings, t0.Add(time.Hour).Add(-time.Nanosecond))
	requireReadingValue(t, got, energy.ActiveImport, "10")
}

func TestSelectBoundaryExactlyAtTheBound(t *testing.T) {
	readings := []energy.Reading{
		*readingAt(t0, "10"),
		*readingAt(t0.Add(time.Hour), "20"),
	}
	got := energy.SelectBoundary(readings, t0.Add(time.Hour))
	requireReadingValue(t, got, energy.ActiveImport, "20", "the reading AT the bound must be selected, not skipped")
}

func TestSelectBoundaryOneNanosecondAfterTheBound(t *testing.T) {
	readings := []energy.Reading{
		*readingAt(t0, "10"),
		*readingAt(t0.Add(time.Hour), "20"),
		*readingAt(t0.Add(2*time.Hour), "30"),
	}
	got := energy.SelectBoundary(readings, t0.Add(time.Hour).Add(time.Nanosecond))
	requireReadingValue(t, got, energy.ActiveImport, "20", "the next reading must not be pulled in one ns early")
}

func TestSelectBoundaryOnEmptyInput(t *testing.T) {
	require.Nil(t, energy.SelectBoundary(nil, t0))
}

func TestSelectBoundaryWhenEveryReadingIsAfterTheBound(t *testing.T) {
	readings := []energy.Reading{*readingAt(t0.Add(time.Hour), "10"), *readingAt(t0.Add(2*time.Hour), "20")}
	require.Nil(t, energy.SelectBoundary(readings, t0))
}

func TestSelectBoundaryDoesNotMutateOrReorderInput(t *testing.T) {
	readings := []energy.Reading{*readingAt(t0, "10"), *readingAt(t0.Add(time.Hour), "20"), *readingAt(t0.Add(2*time.Hour), "30")}
	cp := append([]energy.Reading(nil), readings...)

	energy.SelectBoundary(readings, t0.Add(time.Hour))

	require.Equal(t, cp, readings)
}

// Ties: two readings share the same TS <= bound; SelectBoundary must
// return the LAST of them in the slice, same as the linear scan it
// replaces.
func TestSelectBoundaryReturnsTheLastOfDuplicateTimestamps(t *testing.T) {
	readings := []energy.Reading{
		*readingAt(t0, "10"),
		*readingAt(t0, "20"), // same TS as above, later in the slice
		*readingAt(t0.Add(time.Hour), "30"),
	}
	got := energy.SelectBoundary(readings, t0)
	requireReadingValue(t, got, energy.ActiveImport, "20", "ties: the last of equal timestamps <= bound wins")
}

// linearSelectBoundary is a reference behavior: scan ascending, keep the
// last reading with TS <= bound. TestSelectBoundaryMatches...
// below checks the binary-search implementation agrees with it exactly,
// including on duplicate timestamps.
func linearSelectBoundary(readings []energy.Reading, bound time.Time) *energy.Reading {
	var out *energy.Reading
	for i := range readings {
		if readings[i].TS.After(bound) {
			break
		}
		out = &readings[i]
	}
	return out
}

// SelectBoundary's binary search must agree with a linear reference
// scan over randomized sorted input, including duplicate timestamps, for
// bounds before, between, on and after every reading. Fixed seed so a
// failure is reproducible.
//
// Every reading carries the value "1", so a TS-only comparison cannot
// tell which of several equal-timestamp readings was picked — a mutation
// that walked back to the FIRST of a run of duplicates instead of the LAST
// stayed green under that comparison. require.Same asserts pointer
// identity into the shared readings slice instead, which does distinguish
// them.
func TestSelectBoundaryMatchesALinearScanOverRandomizedSortedInput(t *testing.T) {
	rng := rand.New(rand.NewSource(20260916))

	for trial := 0; trial < 200; trial++ {
		n := rng.Intn(50) + 1
		ts := make([]time.Time, n)
		cur := t0
		for i := range ts {
			if i > 0 && rng.Intn(4) == 0 {
				// Repeat the previous timestamp so duplicates get exercised.
				ts[i] = ts[i-1]
			} else {
				cur = cur.Add(time.Duration(rng.Intn(120)+1) * time.Minute)
				ts[i] = cur
			}
		}
		readings := make([]energy.Reading, n)
		for i, at := range ts {
			readings[i] = *readingAt(at, "1")
		}

		maxMin := int(ts[n-1].Sub(t0).Minutes())
		for b := 0; b < 30; b++ {
			offset := rng.Intn(maxMin+11) - 5 // -5 .. maxMin+5 minutes, straddling every reading
			bound := t0.Add(time.Duration(offset) * time.Minute)

			want := linearSelectBoundary(readings, bound)
			got := energy.SelectBoundary(readings, bound)
			if want == nil {
				require.Nil(t, got, "trial %d bound %v", trial, bound)
				continue
			}
			require.NotNil(t, got, "trial %d bound %v", trial, bound)
			require.Same(t, want, got, "trial %d bound %v: want reading at %v got %v (M-9: pointer identity, not just TS)", trial, bound, want.TS, got.TS)
		}
	}
}

// BenchmarkBillingYearHourly measures SelectBoundary's cost over a billing
// year: 35,040 fifteen-minute readings (365 days) against 8,760 hourly
// bounds (365 days) — the shape that makes a linear scan per bound cost
// about 1.75s of CPU for one request, which is why SelectBoundary uses a
// binary search instead.
func BenchmarkBillingYearHourly(b *testing.B) {
	const readingsPerYear = 35040 // 15-minute intervals, 365 days
	const boundsPerYear = 8760    // hourly bounds, 365 days

	readings := make([]energy.Reading, readingsPerYear)
	for i := range readings {
		readings[i] = *readingAt(t0.Add(time.Duration(i)*15*time.Minute), "100")
	}
	bounds := make([]time.Time, boundsPerYear)
	for i := range bounds {
		bounds[i] = t0.Add(time.Duration(i) * time.Hour)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, bound := range bounds {
			_ = energy.SelectBoundary(readings, bound)
		}
	}
}
