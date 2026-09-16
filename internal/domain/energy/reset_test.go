package energy_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// --- §3.1 not-emitted rules, carried over onto Derive ---------------------

func TestDeriveEmitsNothingWhenABoundaryIsMissing(t *testing.T) {
	w := win(t0, time.Hour)

	dEnd := energy.Derive(w, nil, readingAt(t0.Add(time.Hour), "100"), nil, nil)
	require.False(t, dEnd.Emitted)
	require.Nil(t, dEnd.Values)
	require.Nil(t, dEnd.Suspect)

	dStart := energy.Derive(w, readingAt(t0, "90"), nil, nil, nil)
	require.False(t, dStart.Emitted)
	require.Nil(t, dStart.Values)
	require.Nil(t, dStart.Suspect)
}

func TestDeriveEmitsNothingWhenBothBoundariesAreTheSameReading(t *testing.T) {
	r := readingAt(t0, "100")
	d := energy.Derive(win(t0, time.Hour), r, r, nil, nil)
	require.False(t, d.Emitted)
	require.Nil(t, d.Values)
	require.Nil(t, d.Suspect)
}

func TestDeriveOnReversedBoundariesEmitsNothing(t *testing.T) {
	start := readingAt(t0.Add(time.Hour), "100")
	end := readingAt(t0, "50")
	d := energy.Derive(win(t0, time.Hour), start, end, nil, nil)
	require.False(t, d.Emitted)
	require.Nil(t, d.Values)
	require.Nil(t, d.Suspect)
}

// --- §3.2 reset formula -----------------------------------------------------

func TestDeriveAppliesTheResetFormula(t *testing.T) {
	// start 900 -> before reset 1000 (+100), meter resets to 0 -> end 40 (+40)
	start := readingAt(t0, "900")
	before := readingAt(t0.Add(30*time.Minute), "1000")
	reset := resetAt(t0.Add(40*time.Minute), "0")
	end := readingAt(t0.Add(time.Hour), "40")

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*before})

	require.True(t, d.Emitted)
	require.Equal(t, "140", d.Values[energy.ActiveImport].String())
	require.Empty(t, d.Suspect)
}

func TestDeriveWithAResetThatDoesNotRestartAtZero(t *testing.T) {
	// A replacement meter starts at 5, not 0: the formula uses the row's own value.
	d := energy.Derive(win(t0, time.Hour), readingAt(t0, "900"), readingAt(t0.Add(time.Hour), "45"),
		[]energy.Reading{*resetAt(t0.Add(40*time.Minute), "5")},
		[]energy.Reading{*readingAt(t0.Add(30*time.Minute), "1000")})

	require.Equal(t, "140", d.Values[energy.ActiveImport].String())
	require.Empty(t, d.Suspect)
}

func TestDeriveWithoutAResetEventIsSuspect(t *testing.T) {
	d := energy.Derive(win(t0, time.Hour), readingAt(t0, "999000"), readingAt(t0.Add(time.Hour), "12.5"), nil, nil)

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonNegativeDelta, d.Suspect[energy.ActiveImport].Reason)
	require.Zero(t, d.Suspect[energy.ActiveImport].ResetRows)
}

// I-1: inverted from the plan's original "contributes zero" test. No prior
// reading precedes the reset, so the register must be meter_reset-suspect,
// never a zero-contribution first segment.
func TestDeriveIsSuspectWhenNoReadingPrecedesTheReset(t *testing.T) {
	d := energy.Derive(win(t0, time.Hour), readingAt(t0, "900"), readingAt(t0.Add(time.Hour), "40"),
		[]energy.Reading{*resetAt(t0.Add(10*time.Minute), "0")}, nil)

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason)
	require.Equal(t, 1, d.Suspect[energy.ActiveImport].ResetRows)
}

// I-16: a reset row that reports no value for a register carries no
// evidence for it. active_import (which the reset does cover) still
// derives through the reset formula; t1_import (which it does not) falls
// back to a plain difference across the whole period and is suspect only
// because that plain difference happens to be negative here — not because
// of the reset at all.
func TestDeriveWithAnUnusableResetRowIsSuspectForThatRegisterOnly(t *testing.T) {
	reset := resetAt(t0.Add(40*time.Minute), "0") // resetAt only ever carries active_import
	start := &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: vals("900", "400")}
	end := &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindLoadProfile, Values: vals("40", "10")}

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*readingAt(t0.Add(30*time.Minute), "1000")})

	require.Equal(t, "140", d.Values[energy.ActiveImport].String(), "a sound register still derives through the reset")
	require.Empty(t, d.Suspect[energy.ActiveImport].Reason)

	require.Nil(t, d.Values[energy.T1Import])
	require.Equal(t, energy.ReasonNegativeDelta, d.Suspect[energy.T1Import].Reason, "I-16: no evidence from the reset means plain difference, not meter_reset")
	require.Equal(t, "-390", d.Suspect[energy.T1Import].Delta.String())
	require.Equal(t, 1, d.Suspect[energy.T1Import].ResetRows, "ResetRows counts in-window resets regardless of whether they applied to this register")
}

// I-16, positive case: a reset row missing a register's value still lets
// that register derive normally by plain difference when the difference
// is not negative.
func TestDeriveWithAnUnusableResetRowStillDerivesWhenThePlainDifferenceIsPositive(t *testing.T) {
	reset := resetAt(t0.Add(40*time.Minute), "0")
	start := &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: vals("900", "400")}
	end := &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindLoadProfile, Values: vals("40", "440")}

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*readingAt(t0.Add(30*time.Minute), "1000")})

	require.Equal(t, "140", d.Values[energy.ActiveImport].String())
	require.Equal(t, "40", d.Values[energy.T1Import].String())
	require.NotContains(t, d.Suspect, energy.T1Import)
}

func TestDeriveHandlesTwoResetsInOnePeriod(t *testing.T) {
	// 900 -> prior 1000 -> reset 0 -> prior 60 -> reset 0 -> end 25
	// == (1000-900) + (60-0) + (25-0) == 185
	start := readingAt(t0, "900")
	prior1 := readingAt(t0.Add(10*time.Minute), "1000")
	reset1 := resetAt(t0.Add(20*time.Minute), "0")
	prior2 := readingAt(t0.Add(30*time.Minute), "60")
	reset2 := resetAt(t0.Add(40*time.Minute), "0")
	end := readingAt(t0.Add(time.Hour), "25")

	d := energy.Derive(win(t0, time.Hour), start, end,
		// resets passed out of TS order on purpose: Derive must sort them.
		[]energy.Reading{*reset2, *reset1},
		[]energy.Reading{*prior2, *prior1})

	require.Equal(t, "185", d.Values[energy.ActiveImport].String())
	require.Empty(t, d.Suspect)
}

// --- I-18 evidence window: (start.TS, end.TS], not w -----------------------

func TestDeriveUsesAResetExactlyAtTheEndBoundary(t *testing.T) {
	start := readingAt(t0, "500")
	prior := readingAt(t0.Add(30*time.Minute), "600")
	reset := resetAt(t0.Add(time.Hour), "0") // reset.TS == end.TS
	end := readingAt(t0.Add(time.Hour), "8")

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*prior})

	require.Equal(t, "108", d.Values[energy.ActiveImport].String(), "(before_reset - start) + (end - after_reset) = (600-500) + (8-0)")
	require.Empty(t, d.Suspect)
}

func TestDeriveUsesAResetBetweenStartTSAndWFrom(t *testing.T) {
	// start's own TS is earlier than w.From (the usual shape once the
	// caller selects boundaries with a look-back). The reset sits between
	// start.TS and w.From, outside w but inside (start.TS, end.TS].
	startTS := t0.Add(-30 * time.Minute)
	start := readingAt(startTS, "900")
	prior := readingAt(startTS.Add(5*time.Minute), "1000") // t0 - 25m
	reset := resetAt(startTS.Add(15*time.Minute), "0")     // t0 - 15m, before w.From == t0
	end := readingAt(t0.Add(time.Hour), "40")

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*prior})

	require.Equal(t, "140", d.Values[energy.ActiveImport].String(), "a reset before w.From but after start.TS must still be used")
	require.Empty(t, d.Suspect)
}

func TestDeriveIgnoresAResetAfterTheEndBoundary(t *testing.T) {
	start := readingAt(t0, "900")
	end := readingAt(t0.Add(time.Hour), "40") // negative plain delta without the reset
	reset := resetAt(t0.Add(90*time.Minute), "0")

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, nil)

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonNegativeDelta, d.Suspect[energy.ActiveImport].Reason, "a reset after end.TS carries no evidence")
	require.Zero(t, d.Suspect[energy.ActiveImport].ResetRows, "the out-of-window reset must not even be counted")
}

// --- purity -----------------------------------------------------------------

func TestDeriveDoesNotMutateInputs(t *testing.T) {
	resets := []energy.Reading{*resetAt(t0.Add(40*time.Minute), "0"), *resetAt(t0.Add(10*time.Minute), "0")}
	priors := []energy.Reading{*readingAt(t0.Add(30*time.Minute), "1000"), *readingAt(t0.Add(5*time.Minute), "50")}
	resetsBefore := append([]energy.Reading(nil), resets...)
	priorsBefore := append([]energy.Reading(nil), priors...)

	energy.Derive(win(t0, time.Hour), readingAt(t0, "900"), readingAt(t0.Add(time.Hour), "40"), resets, priors)

	require.Equal(t, resetsBefore, resets, "Derive must not reorder or mutate the caller's resets slice")
	require.Equal(t, priorsBefore, priors, "Derive must not reorder or mutate the caller's priors slice")
}
