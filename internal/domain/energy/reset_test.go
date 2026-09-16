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

// R92(1)/I-6: a zero-width pair — start and end share the same instant,
// even when they are different kinds — measures nothing and must never
// emit. This used to fall through to the fast path and derive 3990
// (5000 - 1010) via Difference, since the old check only excluded a
// same-TS pair when the kinds also matched.
func TestDeriveEmitsNothingWhenBoundariesShareAnInstantOfDifferentKinds(t *testing.T) {
	start := readingAt(t0, "1010")
	end := resetAt(t0, "5000") // same TS as start, different kind
	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*end}, nil)
	require.False(t, d.Emitted, "R92(1): never 3990")
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

// R93 (reverses I-16, review I-9): a reset row that reports no value for a
// register that BOTH boundary readings report is unusable evidence for that
// register — never a plain difference across the reset. active_import
// (which the reset does cover) still derives through the reset formula;
// t1_import (which it does not) is meter_reset-suspect, regardless of the
// sign the plain difference would have had. This is the reviewer's minimal
// fixture: a physical replacement where the operator's ResetAfter omitted
// t1_import must never let t1_import bill a number at all.
func TestDeriveWithAnUnusableResetRowIsSuspectForThatRegisterOnly(t *testing.T) {
	reset := resetAt(t0.Add(40*time.Minute), "0") // resetAt only ever carries active_import
	start := &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: vals("900", "400")}
	end := &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindLoadProfile, Values: vals("40", "10")}

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*readingAt(t0.Add(30*time.Minute), "1000")})

	requireValue(t, d, energy.ActiveImport, "140")
	require.Empty(t, d.Suspect[energy.ActiveImport].Reason, "a sound register still derives through the reset")

	require.Nil(t, d.Values[energy.T1Import], "never -390 or any other plain difference")
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.T1Import].Reason, "R93: an unusable reset row is meter_reset, not negative_delta")
	require.Nil(t, d.Suspect[energy.T1Import].Delta, "meter_reset never carries a Delta (M-2)")
	require.Equal(t, 1, d.Suspect[energy.T1Import].ResetRows, "ResetRows counts in-window resets regardless of whether they applied to this register")
}

// R93 (reverses I-16, review I-9): the reviewer's minimal money fixture. A
// plain difference across the omitted register would have been POSITIVE
// (4610) and so, under the old I-16 rule, would have billed silently and
// unflagged — this is the over-bill the property test found in review
// round 2. It must be meter_reset-suspect regardless of the sign the plain
// difference would have had.
func TestDeriveWithAnUnusableResetRowIsSuspectEvenWhenThePlainDifferenceWouldBePositive(t *testing.T) {
	start := &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: vals("900", "400")}
	prior := &energy.Reading{TS: t0.Add(30 * time.Minute), Kind: energy.KindLoadProfile, Values: vals("1000", "450")}
	reset := &energy.Reading{ // ResetAfter omitted t1_import
		TS: t0.Add(40 * time.Minute), Kind: energy.KindReset,
		Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("5000")},
	}
	end := &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindLoadProfile, Values: vals("5040", "5010")}

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*prior})

	requireValue(t, d, energy.ActiveImport, "140")
	require.Empty(t, d.Suspect[energy.ActiveImport].Reason)

	require.Nil(t, d.Values[energy.T1Import], "never 4610")
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.T1Import].Reason, "never 4610, unflagged")
}

// R93: a register that NEITHER boundary reports is simply unreported — a
// reset row omitting a register nobody asked about must not manufacture
// suspicion for it. This is unaffected by R93 because the up-front
// startValue/endValue nil check in deriveRegister returns before the new
// reset-participation rule ever runs.
func TestDeriveLeavesARegisterNeitherBoundaryReportsNilWithoutSuspicion(t *testing.T) {
	reset := resetAt(t0.Add(40*time.Minute), "0") // carries only active_import
	start := readingAt(t0, "900")                 // carries only active_import
	end := readingAt(t0.Add(time.Hour), "40")     // carries only active_import

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*readingAt(t0.Add(30*time.Minute), "1000")})

	requireValue(t, d, energy.ActiveImport, "140")
	require.Nil(t, d.Values[energy.T1Import])
	require.NotContains(t, d.Suspect, energy.T1Import, "neither boundary reports t1_import, so it is unreported, not suspect")
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

// I-7/R2: R90's end check must apply for any non-reset end kind, not only
// KindLoadProfile — Task 7's billing-derived windows pass a KindBilling
// end, and the review found that narrowing the gate to KindLoadProfile
// alone let the legacy over-bill (1110) back in for a billing end.
func TestDeriveIsSuspectWhenABillingEndAtTheResetInstantIsNotPostReset(t *testing.T) {
	start := readingAt(t0, "900")
	prior := readingAt(t0.Add(45*time.Minute), "1000")
	reset := resetAt(t0.Add(time.Hour), "0") // reset.TS == end.TS
	end := &energy.Reading{
		TS: t0.Add(time.Hour), Kind: energy.KindBilling,
		Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("1010")}, // old meter's closing value
	}

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*prior})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason, "never 1110")
}

// I-7/R3 (also closes M-8's sibling case on the end side): R90's end check
// must use the LAST of several in-window resets, not the first. Two
// resets both carry active_import; only the second sits at end.TS.
func TestDeriveAppliesR90ToTheLastOfSeveralResets(t *testing.T) {
	start := readingAt(t0, "900")
	prior1 := readingAt(t0.Add(10*time.Minute), "1000")
	reset1 := resetAt(t0.Add(20*time.Minute), "0")
	prior2 := readingAt(t0.Add(30*time.Minute), "60")
	reset2 := resetAt(t0.Add(time.Hour), "0") // reset2.TS == end.TS

	mismatched := readingAt(t0.Add(time.Hour), "70") // does not match reset2's own value
	dMismatch := energy.Derive(win(t0, time.Hour), start, mismatched,
		[]energy.Reading{*reset1, *reset2}, []energy.Reading{*prior1, *prior2})
	require.Nil(t, dMismatch.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, dMismatch.Suspect[energy.ActiveImport].Reason, "never 230 (100+60+70)")

	matched := readingAt(t0.Add(time.Hour), "0") // matches reset2's own value exactly
	dMatch := energy.Derive(win(t0, time.Hour), start, matched,
		[]energy.Reading{*reset1, *reset2}, []energy.Reading{*prior1, *prior2})
	requireValue(t, dMatch, energy.ActiveImport, "160")
	require.Empty(t, dMatch.Suspect)
}

// I-7/R5: Task 7's priors slice will contain the start boundary reading
// itself. R91 excludes a prior by strict TS > start.TS, never TS == start.TS.
func TestDeriveIgnoresAPriorAtTheStartInstant(t *testing.T) {
	start := readingAt(t0, "900")
	reset := resetAt(t0.Add(40*time.Minute), "0")
	end := readingAt(t0.Add(time.Hour), "40")

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*start})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason, "never 40")
}

// I-7/R5, a later segment: a prior exactly at the previous reset's own TS
// must be excluded the same way as one at start.TS (strict lower bound).
func TestDeriveIgnoresAPriorAtThePreviousResetInstant(t *testing.T) {
	start := readingAt(t0, "900")
	priorBeforeReset1 := readingAt(t0.Add(10*time.Minute), "1000")
	reset1 := resetAt(t0.Add(20*time.Minute), "0")
	priorAtReset1 := readingAt(t0.Add(20*time.Minute), "7")
	reset2 := resetAt(t0.Add(40*time.Minute), "0")
	end := readingAt(t0.Add(time.Hour), "25")

	d := energy.Derive(win(t0, time.Hour), start, end,
		[]energy.Reading{*reset1, *reset2},
		[]energy.Reading{*priorBeforeReset1, *priorAtReset1})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason, "never 132")
}

// I-5: R91's meter_reset precedence must hold anywhere in the window, not
// only when the end-of-window ambiguity check fires. An earlier segment
// going negative must not short-circuit past a later segment's missing
// before-reset value.
func TestDeriveMeterResetWinsOverAnEarlierNegativeSegment(t *testing.T) {
	start := readingAt(t0, "900")
	prior := readingAt(t0.Add(10*time.Minute), "800") // 800-900 = -100, negative
	reset1 := resetAt(t0.Add(20*time.Minute), "0")
	reset2 := resetAt(t0.Add(40*time.Minute), "0") // no prior in (20,40)
	end := readingAt(t0.Add(time.Hour), "25")

	d := energy.Derive(win(t0, time.Hour), start, end,
		[]energy.Reading{*reset1, *reset2}, []energy.Reading{*prior})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason, "never negative_delta")
	require.Nil(t, d.Suspect[energy.ActiveImport].Delta)
	require.Equal(t, 2, d.Suspect[energy.ActiveImport].ResetRows)
}

// I-6/R92(2): the start-side mirror of R90/C-1. start is a load_profile
// reading landing on the exact instant of a reset row, but its value
// (1010) is the OLD meter's closing cumulative index, not evidence that
// start already reflects the replacement meter. Billing it as-is would
// derive 4030 (5040-1010) — C-1's defect from the other boundary.
func TestDeriveIsSuspectWhenTheStartReadingAtTheResetInstantIsNotPostReset(t *testing.T) {
	start := readingAt(t0, "1010") // old meter's closing value, NOT the reset's own value
	reset := resetAt(t0, "5000")   // reset.TS == start.TS
	end := readingAt(t0.Add(time.Hour), "5040")

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, nil)

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason, "never 4030")
	require.Nil(t, d.Suspect[energy.ActiveImport].Delta)
	require.Equal(t, 1, d.Suspect[energy.ActiveImport].ResetRows)
}

// I-8 (review round 2): the start-side R92(2) check must apply for any
// non-reset start kind, not only KindLoadProfile — Task 7's billing-derived
// windows pass a KindBilling start, and a gate narrowed to load_profile let
// the legacy over-bill (4030) back in for a billing start. This mirrors
// TestDeriveIsSuspectWhenABillingEndAtTheResetInstantIsNotPostReset on the
// other boundary.
func TestDeriveIsSuspectWhenABillingStartAtTheResetInstantIsNotPostReset(t *testing.T) {
	start := &energy.Reading{
		TS: t0, Kind: energy.KindBilling,
		Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("1010")}, // old meter's closing value
	}
	reset := resetAt(t0, "5000") // reset.TS == start.TS
	end := &energy.Reading{
		TS: t0.Add(time.Hour), Kind: energy.KindBilling,
		Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("5040")},
	}

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, nil)

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason, "never 4030")
	require.Nil(t, d.Suspect[energy.ActiveImport].Delta)
	require.Equal(t, 1, d.Suspect[energy.ActiveImport].ResetRows)
}

// I-8 (review round 2): the start-side equality check must be exact
// (decimal.Equal), never loosened to "at or below" the reset value — a
// start ABOVE the reset value at the reset instant is exactly as ambiguous
// as one below it (it could still be the old meter's last cumulative
// index), and billing it would derive 540 (6040-5500) against a true usage
// of 1040.
func TestDeriveIsSuspectWhenTheStartReadingAtTheResetInstantIsAboveTheResetValue(t *testing.T) {
	start := readingAt(t0, "5500") // ABOVE the reset's own value, old meter's closing value
	reset := resetAt(t0, "5000")   // reset.TS == start.TS
	end := readingAt(t0.Add(time.Hour), "6040")

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, nil)

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason, "never 540")
	require.Nil(t, d.Suspect[energy.ActiveImport].Delta)
	require.Equal(t, 1, d.Suspect[energy.ActiveImport].ResetRows)
}

// M-11 (review round 2): R92(4)'s precedence must also hold at the R90
// end-of-window check specifically — an earlier negative segment must not
// win when the end-of-window mismatch is meter_reset. reset1@20 follows a
// negative segment (800-900); reset2 sits at end.TS and end (70) mismatches
// its own value (0). meter_reset must win, never negative_delta (-100).
func TestDeriveMeterResetWinsWhenAnEarlierSegmentIsNegativeAndTheEndMismatches(t *testing.T) {
	start := readingAt(t0, "900")
	prior1 := readingAt(t0.Add(10*time.Minute), "800") // 800-900 = -100, negative
	reset1 := resetAt(t0.Add(20*time.Minute), "0")
	prior2 := readingAt(t0.Add(40*time.Minute), "30")
	reset2 := resetAt(t0.Add(time.Hour), "0") // reset2.TS == end.TS
	end := readingAt(t0.Add(time.Hour), "70") // mismatches reset2's own value

	d := energy.Derive(win(t0, time.Hour), start, end,
		[]energy.Reading{*reset1, *reset2}, []energy.Reading{*prior1, *prior2})

	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason, "never negative_delta (-100)")
	require.Nil(t, d.Suspect[energy.ActiveImport].Delta)
	require.Equal(t, 2, d.Suspect[energy.ActiveImport].ResetRows)
}

// M-13 (review round 2): a boundary reading of Kind == KindReset is always
// treated as reset evidence at its own instant, even when the caller did
// not also include it in resets. Without this, end being the reset row
// itself but absent from resets would take the I-2 fast path and derive a
// plain difference (5000-100 = 4900), which is wrong: 150 (the prior) minus
// 100 (start) is the pre-reset segment (50), and the trailing segment from
// the reset to itself is zero. The expected result is 50, NOT a suspicion:
// end being the reset row itself removes R90's old/new-meter ambiguity
// entirely (mirroring M-8's treatment of a reset at start when start IS
// that reset row) — there is nothing to disambiguate about a reset row's
// own value against itself.
func TestDeriveTreatsAResetKindBoundaryAsEvidenceEvenWhenAbsentFromResets(t *testing.T) {
	start := readingAt(t0, "100")
	prior := readingAt(t0.Add(30*time.Minute), "150")
	end := resetAt(t0.Add(time.Hour), "5000") // end IS a reset row, but resets is nil

	d := energy.Derive(win(t0, time.Hour), start, end, nil, []energy.Reading{*prior})

	requireValue(t, d, energy.ActiveImport, "50")
	require.Empty(t, d.Suspect, "never 4900")
}

// I-6, matching case: start's value equals the reset row's own value
// exactly, so start counts as trustworthy post-reset evidence and the
// window derives normally (no other reset falls inside (start.TS, end.TS]).
func TestDeriveDerivesNormallyWhenTheStartReadingMatchesTheResetAtStartTS(t *testing.T) {
	start := readingAt(t0, "5000") // matches the reset row's own value
	reset := resetAt(t0, "5000")
	end := readingAt(t0.Add(time.Hour), "5040")

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, nil)

	requireValue(t, d, energy.ActiveImport, "40")
	require.Empty(t, d.Suspect)
}

// M-8/R92(2): a reset row at start.TS whose value matches start ITSELF
// (start is the reset row) is never applied as a segmentation point — it
// lies outside the open lower bound of (start.TS, end.TS] — so the result
// is exactly the plain difference from start to end.
func TestDeriveIgnoresAResetAtStartWhenStartIsThatResetRow(t *testing.T) {
	start := resetAt(t0, "5000") // start IS the reset row itself
	end := readingAt(t0.Add(time.Hour), "5040")

	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*start}, nil)
	plain := energy.Difference(win(t0, time.Hour), start, end)

	requireValue(t, d, energy.ActiveImport, "40")
	require.Empty(t, d.Suspect)
	require.Equal(t, plain.Values, d.Values, "M-8: identical to the plain difference, no reset formula applied")
}

// M-3/R92(5): resets must be kind=reset and priors must be kind=load_profile
// (a documented Derive precondition). An element of the wrong kind is
// skipped, never trusted as evidence. A wrong-kind "reset" that would
// otherwise add a spurious segmentation point, and a wrong-kind (billing)
// reading positioned closer to the real reset than the true prior, must
// both be invisible to Derive: the result is identical to
// TestDeriveAppliesTheResetFormula, which uses the same true fixture
// without either wrong-kind element.
// M-12: two wrong-kind elements in resets, one before and one after the
// real reset. A filterKind that dropped only the FIRST wrong-kind element
// and blindly copied every later one (however correctly filtered the tail
// looked with a single wrong-kind fixture) would leak notAResetAfter
// through as a bogus segmentation point at 45min, between the real reset
// (40min) and end (60min) — with no prior in that gap, the missing
// before-reset value would make active_import wrongly suspect instead of
// 140.
func TestDeriveIgnoresElementsOfTheWrongKind(t *testing.T) {
	start := readingAt(t0, "900")
	truePrior := readingAt(t0.Add(30*time.Minute), "1000")
	reset := resetAt(t0.Add(40*time.Minute), "0")
	end := readingAt(t0.Add(time.Hour), "40")

	notAReset := energy.Reading{ // load_profile, not reset — must not segment
		TS: t0.Add(5 * time.Minute), Kind: energy.KindLoadProfile,
		Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("99999")},
	}
	notAResetAfter := energy.Reading{ // billing, not reset — placed AFTER the real reset (M-12)
		TS: t0.Add(45 * time.Minute), Kind: energy.KindBilling,
		Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("12345")},
	}
	notAPrior := energy.Reading{ // billing, not load_profile — must not be used as before_reset
		TS: t0.Add(35 * time.Minute), Kind: energy.KindBilling,
		Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("777")},
	}

	d := energy.Derive(win(t0, time.Hour), start, end,
		[]energy.Reading{notAReset, *reset, notAResetAfter},
		[]energy.Reading{*truePrior, notAPrior})

	requireValue(t, d, energy.ActiveImport, "140")
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
