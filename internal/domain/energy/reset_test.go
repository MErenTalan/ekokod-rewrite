package energy_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
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
	requireValue(t, d, energy.ActiveImport, "140")
	require.Empty(t, d.Suspect)
}

func TestDeriveWithAResetThatDoesNotRestartAtZero(t *testing.T) {
	// A replacement meter starts at 5, not 0: the formula uses the row's own value.
	d := energy.Derive(win(t0, time.Hour), readingAt(t0, "900"), readingAt(t0.Add(time.Hour), "45"),
		[]energy.Reading{*resetAt(t0.Add(40*time.Minute), "5")},
		[]energy.Reading{*readingAt(t0.Add(30*time.Minute), "1000")})

	requireValue(t, d, energy.ActiveImport, "140")
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

// I-3(a)/T5b: a prior before start.TS must never be used. start is itself a
// later reading than that prior; using the older value as before_reset
// would include usage from before the period. Only prior in the fixture is
// before start, so the register must be meter_reset-suspect, not derive
// off that older reading.
func TestDeriveIgnoresAPriorBeforeTheStartBoundary(t *testing.T) {
	start := readingAt(t0, "900")
	end := readingAt(t0.Add(time.Hour), "40")
	reset := resetAt(t0.Add(40*time.Minute), "0")
	priorBeforeStart := readingAt(t0.Add(-10*time.Minute), "1000")

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*priorBeforeStart})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason)
	require.Equal(t, 1, d.Suspect[energy.ActiveImport].ResetRows)
}

// I-3(b)/T5a: a prior reading exactly AT the reset instant is not a
// before-value. The code treats a reading at the reset instant as
// post-reset everywhere else (a reset at end.TS is included, one at
// start.TS is not), so a prior with TS == reset.TS must be excluded by
// strict "<", the same as start.TS < TS.
func TestDeriveIgnoresAPriorAtTheResetInstant(t *testing.T) {
	start := readingAt(t0, "900")
	end := readingAt(t0.Add(time.Hour), "40")
	reset := resetAt(t0.Add(40*time.Minute), "0")
	priorAtReset := readingAt(t0.Add(40*time.Minute), "1000") // TS == reset.TS

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*priorAtReset})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason)
	require.Equal(t, 1, d.Suspect[energy.ActiveImport].ResetRows)
}

// I-3(c)/T2: two resets in the window, but only the first has a prior in
// its own (start.TS, reset.TS) range. The second segment's missing
// before-reset value must NOT fall back to the first reset's after-value
// (which would silently under-bill); the whole register is meter_reset-
// suspect, with no Delta and ResetRows counting both in-window resets.
func TestDeriveIsSuspectWhenOnlyTheFirstResetHasAPrior(t *testing.T) {
	start := readingAt(t0, "900")
	priorBeforeReset1 := readingAt(t0.Add(10*time.Minute), "1000")
	reset1 := resetAt(t0.Add(20*time.Minute), "0")
	reset2 := resetAt(t0.Add(40*time.Minute), "0")
	end := readingAt(t0.Add(time.Hour), "25")
	// A prior before start must also be ignored (I-3a) alongside this case.
	priorBeforeStart := readingAt(t0.Add(-5*time.Minute), "890")

	d := energy.Derive(win(t0, time.Hour), start, end,
		[]energy.Reading{*reset1, *reset2},
		[]energy.Reading{*priorBeforeStart, *priorBeforeReset1})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason)
	require.Nil(t, d.Suspect[energy.ActiveImport].Delta)
	require.Equal(t, 2, d.Suspect[energy.ActiveImport].ResetRows)
}

// I-3(d)/T3: a negative post-reset segment must be suspect and reported as
// that segment's own (negative) delta — never billed as a positive number
// via .Abs(), and never as the whole period's delta.
func TestDeriveIsSuspectWhenAPostResetSegmentIsNegative(t *testing.T) {
	start := readingAt(t0, "900")
	prior := readingAt(t0.Add(30*time.Minute), "1000")
	reset := resetAt(t0.Add(40*time.Minute), "50") // after_reset = 50
	end := readingAt(t0.Add(time.Hour), "40")      // 40 - 50 = -10

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*prior})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonNegativeDelta, d.Suspect[energy.ActiveImport].Reason)
	requireDelta(t, d.Suspect[energy.ActiveImport], "-10")
	require.Equal(t, 1, d.Suspect[energy.ActiveImport].ResetRows)
}

// I-3(d): the same invariant on the FIRST (pre-reset) segment.
func TestDeriveIsSuspectWhenAPreResetSegmentIsNegative(t *testing.T) {
	start := readingAt(t0, "900")
	prior := readingAt(t0.Add(30*time.Minute), "800") // 800 - 900 = -100
	reset := resetAt(t0.Add(40*time.Minute), "0")
	end := readingAt(t0.Add(time.Hour), "40")

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*prior})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonNegativeDelta, d.Suspect[energy.ActiveImport].Reason)
	requireDelta(t, d.Suspect[energy.ActiveImport], "-100")
	require.Equal(t, 1, d.Suspect[energy.ActiveImport].ResetRows)
}

// R91/I-1, review fixture T6b: reset1 carries only active_import; reset2
// carries active_import and t1_import. The single last prior between
// reset1 and reset2 (p@30) does not report t1_import. before_reset for
// t1_import is therefore missing, and must NOT fall back to the earlier
// prior p@10's t1_import value (450) — that would silently under-bill:
// (470-400)+(15-0) = 65 looks plausible but skips unknown usage between
// p@10 and reset2. active_import is unaffected and still derives 185.
func TestDeriveIsSuspectWhenTheLastPriorDoesNotReportTheRegister(t *testing.T) {
	start := &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: vals("900", "400")}
	reset1 := &energy.Reading{
		TS: t0.Add(20 * time.Minute), Kind: energy.KindReset,
		Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("0")}, // no t1_import
	}
	prior10 := &energy.Reading{TS: t0.Add(10 * time.Minute), Kind: energy.KindLoadProfile, Values: vals("1000", "450")}
	prior30 := &energy.Reading{
		TS: t0.Add(30 * time.Minute), Kind: energy.KindLoadProfile,
		Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("60")}, // no t1_import
	}
	reset2 := &energy.Reading{TS: t0.Add(40 * time.Minute), Kind: energy.KindReset, Values: vals("0", "0")}
	end := &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindLoadProfile, Values: vals("25", "15")}

	d := energy.Derive(win(t0, time.Hour), start, end,
		[]energy.Reading{*reset1, *reset2},
		[]energy.Reading{*prior10, *prior30})

	requireValue(t, d, energy.ActiveImport, "185")
	require.Empty(t, d.Suspect[energy.ActiveImport].Reason)

	require.Nil(t, d.Values[energy.T1Import])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.T1Import].Reason)
	require.Nil(t, d.Suspect[energy.T1Import].Delta)
	require.Equal(t, 2, d.Suspect[energy.T1Import].ResetRows)
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

	requireValue(t, d, energy.ActiveImport, "140")
	require.Empty(t, d.Suspect[energy.ActiveImport].Reason, "a sound register still derives through the reset")

	require.Nil(t, d.Values[energy.T1Import])
	require.Equal(t, energy.ReasonNegativeDelta, d.Suspect[energy.T1Import].Reason, "I-16: no evidence from the reset means plain difference, not meter_reset")
	requireDelta(t, d.Suspect[energy.T1Import], "-390")
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

	requireValue(t, d, energy.ActiveImport, "140")
	requireValue(t, d, energy.T1Import, "40")
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

	// I-2: resets and priors must be supplied sorted ascending by TS —
	// Derive no longer sorts a defensive copy, so this fixture keeps both
	// slices in ascending order rather than testing that it corrects them.
	d := energy.Derive(win(t0, time.Hour), start, end,
		[]energy.Reading{*reset1, *reset2},
		[]energy.Reading{*prior1, *prior2})

	requireValue(t, d, energy.ActiveImport, "185")
	require.Empty(t, d.Suspect)
}

// --- I-2: no reset in the window never touches priors ----------------------

func TestDeriveDoesNotNeedPriorsWhenNoResetIsInTheWindow(t *testing.T) {
	start := readingAt(t0, "900")
	end := readingAt(t0.Add(time.Hour), "1000")
	// Deliberately unsorted: the fast path must not even look at priors
	// when no reset falls in the evidence window, so their order (and, in
	// a stricter sense, their content) cannot matter here.
	unsortedPriors := []energy.Reading{
		*readingAt(t0.Add(50*time.Minute), "1"),
		*readingAt(t0.Add(5*time.Minute), "2"),
	}

	d := energy.Derive(win(t0, time.Hour), start, end, nil, unsortedPriors)

	requireValue(t, d, energy.ActiveImport, "100")
	require.Empty(t, d.Suspect)
}

// --- I-18 evidence window: (start.TS, end.TS], not w -----------------------

// R90: a reset exactly at end.TS whose value matches end's own value is
// trustworthy post-reset evidence, and the trailing segment contributes
// zero by construction.
func TestDeriveUsesAResetExactlyAtTheEndBoundary(t *testing.T) {
	start := readingAt(t0, "500")
	prior := readingAt(t0.Add(30*time.Minute), "600")
	reset := resetAt(t0.Add(time.Hour), "0") // reset.TS == end.TS
	end := readingAt(t0.Add(time.Hour), "0") // matches the reset's own value

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*prior})

	requireValue(t, d, energy.ActiveImport, "100")
	require.Empty(t, d.Suspect, "(before_reset - start) + (end - after_reset) = (600-500) + (0-0), end matches the reset")
}

// R90/C-1, review fixture T4c: end is a load_profile reading landing on the
// exact reset instant, but its value (1010) is the OLD meter's closing
// cumulative index, not the new meter's first reading. Billing it as
// post-reset evidence would derive 1110 — the legacy defect by another
// route. It must be meter_reset-suspect instead.
func TestDeriveIsSuspectWhenTheEndReadingAtTheResetInstantIsNotPostReset(t *testing.T) {
	start := readingAt(t0, "900")
	prior := readingAt(t0.Add(45*time.Minute), "1000")
	reset := resetAt(t0.Add(time.Hour), "0")    // reset.TS == end.TS
	end := readingAt(t0.Add(time.Hour), "1010") // old meter's closing value, NOT 0

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*prior})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason)
	require.Nil(t, d.Suspect[energy.ActiveImport].Delta)
	require.Equal(t, 1, d.Suspect[energy.ActiveImport].ResetRows)
}

// M-2/R90: when the end reading at the reset instant mismatches AND that
// mismatch would also read as a negative segment if billed anyway,
// meter_reset must win — unusable evidence, not a negative number, is the
// root cause, and the equality check happens before finalDelta is ever
// computed.
func TestDeriveMeterResetWinsOverNegativeDeltaAtTheEndBoundary(t *testing.T) {
	start := readingAt(t0, "900")
	prior := readingAt(t0.Add(30*time.Minute), "1000")
	reset := resetAt(t0.Add(time.Hour), "50") // reset.TS == end.TS, after_reset = 50
	end := readingAt(t0.Add(time.Hour), "10") // mismatched AND 10-50 would be negative

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*prior})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason, "meter_reset must win over negative_delta here")
}

// M-4: a reset row passed as end is evidence, not a boundary. Source falls
// back to start.Kind rather than reporting the row's Kind as "reset".
func TestDeriveSourceFallsBackToStartWhenEndIsAResetRow(t *testing.T) {
	start := readingAt(t0, "900")
	prior := readingAt(t0.Add(30*time.Minute), "1000")
	end := resetAt(t0.Add(time.Hour), "0") // end IS the reset row itself

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*end}, []energy.Reading{*prior})

	require.True(t, d.Emitted)
	require.Equal(t, energy.KindLoadProfile, d.Source, "a reset row must not be attributed as Source")
	requireValue(t, d, energy.ActiveImport, "100")
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

	requireValue(t, d, energy.ActiveImport, "140")
	require.Empty(t, d.Suspect, "a reset before w.From but after start.TS must still be used")
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

// M-1: resetsBefore/priorsBefore are rebuilt from literals rather than
// copied from resets/priors, so they share no Values map or *decimal.Decimal
// pointer with the inputs — a mutation that edited a map entry or a decimal
// in place would otherwise change both sides identically and stay green.
// The fixture also derives successfully through BOTH resets (positive
// result, no early return), so a mutation anywhere in the loop gets a
// chance to touch the inputs before the function returns.
func TestDeriveDoesNotMutateInputs(t *testing.T) {
	resets := []energy.Reading{*resetAt(t0.Add(20*time.Minute), "0"), *resetAt(t0.Add(40*time.Minute), "0")}
	priors := []energy.Reading{*readingAt(t0.Add(10*time.Minute), "1000"), *readingAt(t0.Add(30*time.Minute), "60")}

	wantResets := []energy.Reading{*resetAt(t0.Add(20*time.Minute), "0"), *resetAt(t0.Add(40*time.Minute), "0")}
	wantPriors := []energy.Reading{*readingAt(t0.Add(10*time.Minute), "1000"), *readingAt(t0.Add(30*time.Minute), "60")}

	d := energy.Derive(win(t0, time.Hour), readingAt(t0, "900"), readingAt(t0.Add(time.Hour), "25"), resets, priors)

	requireValue(t, d, energy.ActiveImport, "185")
	require.Empty(t, d.Suspect)

	require.Equal(t, wantResets, resets, "Derive must not reorder or mutate the caller's resets slice")
	require.Equal(t, wantPriors, priors, "Derive must not reorder or mutate the caller's priors slice")
}
