package energy

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// Derive implements 02 §3.1 together with §3.2's meter-reset and
// replacement rule.
//
// start and end are the boundary readings already selected by SelectBoundary
// (§3.1: "the last reading with ts <= bound"). resets are kind=reset
// readings and priors are kind=load_profile readings that carry the
// register values seen strictly between a segment's start and a reset
// (I-1: priors are load_profile readings regardless of the boundary kind).
//
// Precondition (I-2, the same one SelectBoundary documents): both resets
// and priors must be supplied sorted ascending by TS. Derive does not sort
// or copy them — doing that unconditionally made repeated derivation over a
// shared year-long priors slice cost ~1.49ms and ~2.1MB of garbage per call
// (~15s and ~20GB for 10,000 buckets) — so an unsorted slice gives an
// unspecified result. See the fast path below for the one case where
// priors's order (and its contents) never matters at all.
//
// Derive applies the same not-emitted rules as Difference: a missing
// boundary, identical start/end readings, or reversed boundaries all yield
// Emitted: false with Values and Suspect left nil (M-9).
//
// Source is end.Kind, except when end.Kind is KindReset (M-4): a reset row
// is evidence of a physical meter event, not a boundary a caller should
// attribute the derived row to, so Source falls back to start.Kind in that
// case.
//
// The reset evidence window is (start.TS, end.TS] (I-18) — NOT w. A reset
// exactly at end.TS is used; one before w.From but after start.TS is used;
// one after end.TS is ignored. A reset at start.TS is never applied, even
// when start is itself that reset row: it stays outside the open lower
// bound of the window.
//
// I-2 fast path: when no reset falls anywhere in this window, Derive
// returns exactly Difference's Values and Suspect and never inspects
// priors at all — the common case once a reset has rolled off the trailing
// window.
//
// Per register, the period is split at every in-window reset that carries a
// non-nil value for that register (R56). For a segment ending at a reset,
//
//	segment_delta = before_reset - segment_start_value
//
// where before_reset is the value of the SINGLE LAST prior reading with
// start.TS < prior.TS < reset.TS (or, for a later segment, with the
// previous reset's TS < prior.TS < this reset's TS). R91/I-1: if that one
// reading exists but does not report the register, before_reset is
// missing — Derive never reaches further back to an older reading that
// does report it, because that older value would silently omit whatever
// usage happened between it and the missing reading. after_reset — the
// reset row's own value — becomes the next segment's start value. If no
// prior reading satisfies the range at all, or the single one that does
// lacks the register, the register is left suspect with ReasonMeterReset:
// a missing before-reset value NEVER contributes zero (I-1, amending R55's
// original "contributes zero" text). Consumption is the sum of the segment
// deltas; a negative segment delta makes the register suspect with
// ReasonNegativeDelta and that segment's own delta (M-2: Suspicion.Delta is
// the FIRST failing segment's own difference in time order — never the
// whole period's — and is nil for meter_reset).
//
// R90 (review C-1): when the last in-window reset carrying a register's
// value sits exactly at end.TS and end is not itself a reset row
// (end.Kind != KindReset), end is ambiguous evidence — it could be the
// post-reset meter's first reading, or the OLD meter's last cumulative
// reading landing on the same instant, since operator- and provider-
// entered resets tend to fall on round local times that are also where
// billing buckets end and load-profile readings are stamped. Derive
// resolves this the only safe way: end's value for that register must
// equal the reset row's own value exactly (decimal.Equal). If it does, the
// trailing segment contributes zero by construction — no time passed
// between the reset and a reading at the same instant. If it does not, the
// register is ReasonMeterReset-suspect: meter_reset wins over
// negative_delta here, because unusable evidence, not a negative number,
// is the root cause, and the mismatched value is never billed as a
// difference. This check happens before finalDelta is ever computed, so a
// mismatch that would also read as negative still comes out meter_reset.
//
// A reset row with no value for a register carries no evidence for that
// register: that register derives across the reset by plain difference
// between start and end, and is suspect only if that plain difference is
// negative (I-16). This is also exactly what happens when there is no
// reset evidence for a register at all.
//
// Suspicion.ResetRows on every Suspicion this function returns is the
// count of resets inside the evidence window, regardless of whether they
// carried evidence for the suspect register — it lets the operator message
// distinguish "a reset exists but could not be applied" from "no reset at
// all" (line up with the same rule in Task 1's Suspicion doc).
//
// Registers untouched by any problem keep their derived values (R58). No
// rounding, no modulus, no rollover arithmetic (R57).
func Derive(w Window, start, end *Reading, resets []Reading, priors []Reading) Derivation {
	d := Derivation{Window: w}

	if start == nil || end == nil {
		// 02 §3.1: a missing boundary yields no row.
		return d
	}
	if start.TS.Equal(end.TS) && start.Kind == end.Kind {
		// The same reading selected for both bounds: still no row.
		return d
	}
	if end.TS.Before(start.TS) {
		// Reversed boundaries are a caller bug, not a meter event.
		return d
	}

	d.Emitted = true
	d.Source = end.Kind
	if end.Kind == KindReset {
		// M-4: a reset row is evidence, not a boundary; do not attribute
		// the derived row to it.
		d.Source = start.Kind
	}

	windowResets := inEvidenceWindow(resets, start.TS, end.TS)
	if len(windowResets) == 0 {
		// I-2 fast path: no reset evidence in the window for any register,
		// so priors is never touched. Delegate to Difference's exact
		// per-register semantics rather than duplicating them.
		plain := Difference(w, start, end)
		d.Values = plain.Values
		d.Suspect = plain.Suspect
		return d
	}

	values := make(map[Register]*decimal.Decimal, len(allRegisters))
	suspect := make(map[Register]Suspicion)

	for _, reg := range allRegisters {
		val, susp := deriveRegister(reg, start, end, windowResets, priors)
		values[reg] = val
		if susp != nil {
			suspect[reg] = *susp
		}
	}

	d.Values = values
	d.Suspect = suspect
	return d
}

// inEvidenceWindow returns a fresh slice of the resets whose TS falls in
// (startTS, endTS] (I-18), in the same relative order as resets. The input
// slice is never mutated or aliased into the result.
//
// Precondition: resets is sorted ascending by TS (the same precondition
// Derive documents); inEvidenceWindow does not sort — filtering a sorted
// slice preserves ascending order, so a second sort would be redundant
// work on every call.
func inEvidenceWindow(resets []Reading, startTS, endTS time.Time) []Reading {
	out := make([]Reading, 0, len(resets))
	for _, r := range resets {
		if r.TS.After(startTS) && !r.TS.After(endTS) {
			out = append(out, r)
		}
	}
	return out
}

// deriveRegister derives one register's consumption over [start, end],
// applying reset segmentation (§3.2, R90/R91) when the register has usable
// reset evidence, and a plain difference (§3.1) otherwise. It returns a nil
// value with a nil Suspicion when the register is simply unreported (never
// treated as zero, removed-behaviour 21).
func deriveRegister(reg Register, start, end *Reading, resets, priors []Reading) (*decimal.Decimal, *Suspicion) {
	startValue := start.Value(reg)
	endValue := end.Value(reg)
	if startValue == nil || endValue == nil {
		return nil, nil
	}

	var regResets []Reading
	for _, r := range resets {
		if r.Value(reg) != nil {
			regResets = append(regResets, r)
		}
	}

	if len(regResets) == 0 {
		// No usable reset evidence for this register at all: plain
		// difference (I-16 covers both "no resets in window" and "resets
		// exist but none carry this register").
		delta := endValue.Sub(*startValue)
		if delta.IsNegative() {
			return nil, &Suspicion{Reason: ReasonNegativeDelta, Delta: &delta, ResetRows: len(resets)}
		}
		return &delta, nil
	}

	total := decimal.Zero
	segStart := startValue
	prevTS := start.TS
	for _, r := range regResets {
		before := priorValueBefore(priors, reg, prevTS, r.TS)
		if before == nil {
			// R91/I-1: a missing before-reset value is never a zero
			// contribution, and never falls back to an older reading. The
			// register is suspect and its consumption is nil.
			return nil, &Suspicion{Reason: ReasonMeterReset, ResetRows: len(resets)}
		}
		delta := before.Sub(*segStart)
		if delta.IsNegative() {
			return nil, &Suspicion{Reason: ReasonNegativeDelta, Delta: &delta, ResetRows: len(resets)}
		}
		total = total.Add(delta)

		segStart = r.Value(reg) // after_reset: the reset row's own value.
		prevTS = r.TS
	}

	// R90: the last reset carrying this register's value sits exactly at
	// end.TS, and end is not that reset row itself. end's value must equal
	// after_reset exactly (segStart, just set above), or it is ambiguous
	// old-meter/new-meter evidence and the register is suspect rather than
	// billed as a number. This check happens BEFORE finalDelta is computed
	// on purpose (M-2): meter_reset must win even when the mismatch would
	// also read as a negative difference.
	if last := regResets[len(regResets)-1]; last.TS.Equal(end.TS) && end.Kind != KindReset {
		if !endValue.Equal(*segStart) {
			return nil, &Suspicion{Reason: ReasonMeterReset, ResetRows: len(resets)}
		}
		// The trailing segment is zero by construction: no time passes
		// between the reset row and a reading at the same instant.
		return &total, nil
	}

	finalDelta := endValue.Sub(*segStart)
	if finalDelta.IsNegative() {
		return nil, &Suspicion{Reason: ReasonNegativeDelta, Delta: &finalDelta, ResetRows: len(resets)}
	}
	total = total.Add(finalDelta)
	return &total, nil
}

// priorValueBefore returns reg's value from the SINGLE LAST reading in
// priors (sorted ascending by TS — the same precondition Derive documents)
// with after < TS < before, or nil when no reading qualifies.
//
// R91/I-1: if that one reading exists but does not report reg, its nil IS
// the answer. priorValueBefore never reaches further back to an earlier
// reading that does report reg — an older value would silently omit
// whatever usage happened between it and the missing reading.
//
// The lookup is a binary search (I-2): the first index with TS >= before,
// stepped back one, then checked against after. This replaces an O(P) scan
// per reset per register with O(log P).
func priorValueBefore(priors []Reading, reg Register, after, before time.Time) *decimal.Decimal {
	i := sort.Search(len(priors), func(i int) bool { return !priors[i].TS.Before(before) })
	if i == 0 {
		return nil
	}
	last := priors[i-1]
	if !last.TS.After(after) {
		return nil
	}
	return last.Value(reg)
}
