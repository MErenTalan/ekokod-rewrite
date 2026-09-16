package energy_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

func TestDifferenceEmitsNothingWhenABoundaryIsMissing(t *testing.T) {
	w := win(t0, time.Hour)

	dEnd := energy.Difference(w, nil, readingAt(t0.Add(time.Hour), "100"))
	require.False(t, dEnd.Emitted)
	require.Nil(t, dEnd.Values, "M-1: !Emitted must leave Values nil, never an empty non-nil map")
	require.Nil(t, dEnd.Suspect, "M-1: !Emitted must leave Suspect nil, never an empty non-nil map")

	dStart := energy.Difference(w, readingAt(t0, "90"), nil)
	require.False(t, dStart.Emitted)
	require.Nil(t, dStart.Values)
	require.Nil(t, dStart.Suspect)
}

func TestDifferenceEmitsNothingWhenBothBoundariesAreTheSameReading(t *testing.T) {
	w := win(t0, time.Hour)

	r := readingAt(t0, "100")
	same := energy.Difference(w, r, r)
	require.False(t, same.Emitted, "identical boundary readings must not become a zero row")
	require.Nil(t, same.Values)
	require.Nil(t, same.Suspect)

	// I-7: two DISTINCT *Reading values carrying the same TS and Kind (the
	// normal shape once a converter selects "the reading at ts" separately
	// for each boundary, as Task 7's converter will) must be treated
	// identically to the one-pointer case above. A pointer-identity check
	// (`start == end`) would let these two separately allocated Readings
	// slip through as an emitted zero row, exactly what §3.1 forbids.
	start := &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: vals("100.0000", "10.0000")}
	end := &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: vals("150.0000", "20.0000")}
	distinct := energy.Difference(w, start, end)
	require.False(t, distinct.Emitted, "distinct pointers with the same TS and Kind must still be treated as the same reading")
	require.Nil(t, distinct.Values)
	require.Nil(t, distinct.Suspect)
}

func TestDifferenceOnReversedBoundariesEmitsNothing(t *testing.T) {
	// M-6: end.TS before start.TS is a caller bug (Difference's documented
	// precondition is !end.TS.Before(start.TS)), not a meter event. A
	// caller passing boundaries out of order must get "no row", never a
	// negative-delta suspicion that would write a spurious anomaly
	// downstream.
	start := readingAt(t0.Add(time.Hour), "100.0000")
	end := readingAt(t0, "50.0000")
	d := energy.Difference(win(t0, time.Hour), start, end)
	require.False(t, d.Emitted)
	require.Nil(t, d.Values)
	require.Nil(t, d.Suspect)
}

func TestDifferenceSubtractsPerRegisterAndKeepsNilNil(t *testing.T) {
	w := win(t0, time.Hour)
	start := &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{
		// M-3: a 4-decimal-place fixture, so a delta.Round(3) mutation
		// changes the asserted string ("12.2525" would become "12.253" or
		// similar) instead of coincidentally matching.
		energy.ActiveImport: dec("1000.1234"), energy.T1Import: dec("400.0000"),
	}}
	end := &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindBilling, Values: map[energy.Register]*decimal.Decimal{
		energy.ActiveImport: dec("1012.3759"), // T1Import absent at the end boundary
		// I-6: absent at the START boundary, with a large end value. A
		// mutation that treats a nil start register as zero would return
		// the full end register value here (removed-behaviour 1 in a new
		// form) instead of leaving it nil.
		energy.T2Import: dec("5000.0000"),
	}}
	d := energy.Difference(w, start, end)
	require.True(t, d.Emitted)

	// M-4: Window and Source are asserted, and Source is distinguished from
	// start.Kind by using a KindBilling end reading.
	require.True(t, w.From.Equal(d.Window.From), "got %s", d.Window.From)
	require.True(t, w.To.Equal(d.Window.To), "got %s", d.Window.To)
	require.Equal(t, energy.KindBilling, d.Source, "Source is the end reading's kind")

	require.Equal(t, "12.2525", d.Values[energy.ActiveImport].String())
	require.Nil(t, d.Values[energy.T1Import], "an unreported register at the end is nil, never zero")
	require.Nil(t, d.Values[energy.T2Import], "an unreported register at the start is nil, never the full end value")
	require.Nil(t, d.Values[energy.ActiveExport], "a register absent on both sides is nil")
	require.Len(t, d.Values, 12)
	require.Empty(t, d.Suspect, "an unreported register is not suspect")
}

func TestDifferenceLeavesANegativeDeltaSuspectAndNeverReturnsTheEndValue(t *testing.T) {
	start := readingAt(t0, "999000.0000")
	end := readingAt(t0.Add(time.Hour), "12.5000")
	d := energy.Difference(win(t0, time.Hour), start, end)
	require.True(t, d.Emitted)
	require.Nil(t, d.Values[energy.ActiveImport], "removed-behaviour 1: never the end register value")
	require.Equal(t, energy.ReasonNegativeDelta, d.Suspect[energy.ActiveImport].Reason)
	require.Equal(t, "-998987.5", d.Suspect[energy.ActiveImport].Delta.String())
}
