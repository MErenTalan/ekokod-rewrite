package energy_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

func TestDifferenceEmitsNothingWhenABoundaryIsMissing(t *testing.T) {
	w := energy.Window{From: t0, To: t0.Add(time.Hour)}
	require.False(t, energy.Difference(w, nil, readingAt(t0.Add(time.Hour), "100")).Emitted)
	require.False(t, energy.Difference(w, readingAt(t0, "90"), nil).Emitted)
}

func TestDifferenceEmitsNothingWhenBothBoundariesAreTheSameReading(t *testing.T) {
	r := readingAt(t0, "100")
	d := energy.Difference(energy.Window{From: t0, To: t0.Add(time.Hour)}, r, r)
	require.False(t, d.Emitted, "identical boundary readings must not become a zero row")
	require.Empty(t, d.Values)
}

func TestDifferenceSubtractsPerRegisterAndKeepsNilNil(t *testing.T) {
	start := &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{
		energy.ActiveImport: dec("1000.5000"), energy.T1Import: dec("400.0000"),
	}}
	end := &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{
		energy.ActiveImport: dec("1012.7500"), // T1Import absent at the end boundary
	}}
	d := energy.Difference(energy.Window{From: t0, To: t0.Add(time.Hour)}, start, end)
	require.True(t, d.Emitted)
	require.Equal(t, "12.25", d.Values[energy.ActiveImport].String())
	require.Nil(t, d.Values[energy.T1Import], "an unreported register is nil, never zero")
	require.Empty(t, d.Suspect, "an unreported register is not suspect")
}

func TestDifferenceLeavesANegativeDeltaSuspectAndNeverReturnsTheEndValue(t *testing.T) {
	start := readingAt(t0, "999000.0000")
	end := readingAt(t0.Add(time.Hour), "12.5000")
	d := energy.Difference(energy.Window{From: t0, To: t0.Add(time.Hour)}, start, end)
	require.True(t, d.Emitted)
	require.Nil(t, d.Values[energy.ActiveImport], "removed-behaviour 1: never the end register value")
	require.Equal(t, energy.ReasonNegativeDelta, d.Suspect[energy.ActiveImport].Reason)
	require.Equal(t, "-998987.5", d.Suspect[energy.ActiveImport].Delta.String())
}
