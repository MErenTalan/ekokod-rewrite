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
// readings, in any order; Derive sorts a copy and never mutates the slice
// the caller passed in. priors are kind=load_profile readings that carry
// the register values seen strictly between a segment's start and a reset
// (I-1: priors are load_profile readings regardless of the boundary kind);
// they too are copied and sorted, never mutated.
//
// Derive applies the same not-emitted rules as Difference: a missing
// boundary, identical start/end readings, or reversed boundaries all yield
// Emitted: false with Values and Suspect left nil (M-9).
//
// The reset evidence window is (start.TS, end.TS] (I-18) — NOT w. A reset
// exactly at end.TS is used; one before w.From but after start.TS is used;
// one after end.TS is ignored.
//
// Per register, the period is split at every in-window reset that carries a
// non-nil value for that register (R56). For a segment ending at a reset,
//
//	segment_delta = before_reset - segment_start_value
//
// where before_reset is the value of the last prior reading with
// start.TS < prior.TS < reset.TS (or, for a later segment, with the
// previous reset's TS < prior.TS < this reset's TS). after_reset — the
// reset row's own value — becomes the next segment's start value. If no
// prior reading satisfies that range, the register is left suspect with
// ReasonMeterReset: a missing before-reset value NEVER contributes zero
// (I-1, amending R55's original "contributes zero" text). Consumption is
// the sum of the segment deltas; a negative segment delta makes the
// register suspect with ReasonNegativeDelta and that segment's delta.
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

	windowResets := inEvidenceWindow(resets, start.TS, end.TS)
	sortedPriors := make([]Reading, len(priors))
	copy(sortedPriors, priors)
	sort.Slice(sortedPriors, func(i, j int) bool { return sortedPriors[i].TS.Before(sortedPriors[j].TS) })

	values := make(map[Register]*decimal.Decimal, len(allRegisters))
	suspect := make(map[Register]Suspicion)

	for _, reg := range allRegisters {
		val, susp := deriveRegister(reg, start, end, windowResets, sortedPriors)
		values[reg] = val
		if susp != nil {
			suspect[reg] = *susp
		}
	}

	d.Values = values
	d.Suspect = suspect
	return d
}

// inEvidenceWindow returns a fresh, ascending-sorted copy of the resets
// whose TS falls in (startTS, endTS] (I-18). The input slice is never
// mutated or aliased into the result.
func inEvidenceWindow(resets []Reading, startTS, endTS time.Time) []Reading {
	out := make([]Reading, 0, len(resets))
	for _, r := range resets {
		if r.TS.After(startTS) && !r.TS.After(endTS) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS.Before(out[j].TS) })
	return out
}

// deriveRegister derives one register's consumption over [start, end],
// applying reset segmentation (§3.2) when the register has usable reset
// evidence, and a plain difference (§3.1) otherwise. It returns a nil value
// with a nil Suspicion when the register is simply unreported (never
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
			// I-1 / amended R55: a missing before-reset value is never a
			// zero contribution. The register is suspect and its
			// consumption is nil.
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

	finalDelta := endValue.Sub(*segStart)
	if finalDelta.IsNegative() {
		return nil, &Suspicion{Reason: ReasonNegativeDelta, Delta: &finalDelta, ResetRows: len(resets)}
	}
	total = total.Add(finalDelta)
	return &total, nil
}

// priorValueBefore returns reg's value from the last reading in priors
// (sorted ascending) with after < TS < before, or nil when none qualifies.
func priorValueBefore(priors []Reading, reg Register, after, before time.Time) *decimal.Decimal {
	var found *decimal.Decimal
	for i := range priors {
		ts := priors[i].TS
		if !ts.After(after) {
			continue
		}
		if !ts.Before(before) {
			break
		}
		if v := priors[i].Value(reg); v != nil {
			found = v
		}
	}
	return found
}
