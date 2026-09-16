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
// Precondition (R92/M-3): every element of resets must be KindReset and
// every element of priors must be KindLoadProfile. Derive does not simply
// trust the caller: an element of the wrong kind is skipped rather than
// used as evidence — filterKind drops a wrong-kind reset before it is ever
// searched, and priorValueBefore skips a wrong-kind prior in place, walking
// back to the next candidate as if the wrong-kind one were never in the
// slice. A caller should still pre-filter (Task 7's readers do), both for
// clarity and because the skip path costs a little more than trusting
// homogeneous input.
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
// boundary, boundaries sharing the same instant (R92(1) — regardless of
// kind, see below), or reversed boundaries all yield Emitted: false with
// Values and Suspect left nil (M-9).
//
// R92(1), amending the original identical-boundary rule: start.TS.Equal
// (end.TS) alone is enough to emit nothing, even when start and end are
// different kinds. The original rule only matched when the kinds were also
// equal, so a load_profile start and a reset row landing on the same
// instant fell through to the fast path and derived a plain difference
// between two readings zero time apart — I-6's zero-width defect (a start
// of 1010 and an end that was actually the reset row at 5000 derived 3990,
// when nothing happened in between at all: no time passed, so nothing can
// have been measured).
//
// Source is end.Kind, except when end.Kind is KindReset (M-4): a reset row
// is evidence of a physical meter event, not a boundary a caller should
// attribute the derived row to, so Source falls back to start.Kind in that
// case.
//
// The reset evidence window used for SEGMENTATION is (start.TS, end.TS]
// (I-18) — NOT w. A reset exactly at end.TS is used as a segmentation
// point; one before w.From but after start.TS is used; one after end.TS is
// ignored. A reset at start.TS is NEVER a segmentation point, even when
// start is itself that reset row: it stays outside the open lower bound of
// the window. This does NOT mean a reset at start.TS is invisible to
// Derive — see R92(2) immediately below, which inspects it for a different
// purpose than segmentation.
//
// R92(2)/I-6 (amends R90, needs no separate implementation of R90 itself):
// the start-side mirror of C-1/R90. When a reset row's TS equals start.TS
// and start is not itself that reset row, start's value at the shared
// instant is exactly as ambiguous as end's value is at a reset landing on
// end.TS (R90) — it could be the replacement meter's first reading, or the
// OLD meter's last cumulative reading landing on the same instant. Derive
// resolves this the same way R90 does at the other boundary: for each
// register the start-side reset carries, start's value for that register
// must equal the reset row's own value exactly (decimal.Equal), or the
// register is ReasonMeterReset-suspect. This check runs BEFORE any
// segmentation is attempted for the register (mirroring R90's ordering
// relative to finalDelta) and before the no-reset fast path below, because
// inEvidenceWindow's open lower bound would otherwise hide this reset from
// Derive entirely. R92(3): both this check and R90's use the LAST reset row
// at a shared instant (lastResetAt), the same tie-break SelectBoundary
// documents for readings — the primary key (analyzer_id, ts, kind) makes it
// impossible for more than one reset row to actually share an instant, but
// the tie-break exists so the rule is well defined regardless (M-17).
//
// I-2 fast path: when no reset falls anywhere in the segmentation window
// AND no reset sits exactly at start.TS (so R92(2) has nothing to check
// either), Derive returns exactly Difference's Values and Suspect and
// never inspects priors at all — the common case once a reset has rolled
// off the trailing window.
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
// original "contributes zero" text).
//
// R92(4)/I-5 (amends R91's precedence, which the original implementation
// only honored at the end boundary): ReasonMeterReset wins over
// ReasonNegativeDelta for a register whenever BOTH problems occur anywhere
// in the window, regardless of which one a naive left-to-right scan would
// meet first. Concretely: a negative segment earlier in time does not
// short-circuit the scan — Derive keeps examining every remaining segment
// (and, once the loop finishes without a reset-evidence problem, the R90
// end-of-window check) before deciding. If any segment lacks its
// before-reset value, the register's reason is ReasonMeterReset even though
// an earlier segment was negative first; only when no reset-evidence
// problem exists anywhere does the FIRST negative segment's own delta win,
// as before (M-2). Consumption is the sum of the segment deltas when
// nothing is suspect.
//
// R90 (review C-1): when the last in-window reset carrying a register's
// value sits exactly at end.TS and end is not itself a reset row
// (end.Kind != KindReset — this is checked for ANY non-reset end kind,
// including KindBilling, not only KindLoadProfile), end is ambiguous
// evidence — it could be the post-reset meter's first reading, or the OLD
// meter's last cumulative reading landing on the same instant, since
// operator- and provider-entered resets tend to fall on round local times
// that are also where billing buckets end and load-profile readings are
// stamped. Derive resolves this the only safe way: end's value for that
// register must equal the reset row's own value exactly (decimal.Equal).
// If it does, the trailing segment contributes zero by construction — no
// time passed between the reset and a reading at the same instant. If it
// does not, the register is ReasonMeterReset-suspect: meter_reset wins over
// negative_delta here, because unusable evidence, not a negative number,
// is the root cause, and the mismatched value is never billed as a
// difference. This check happens before finalDelta is ever computed, so a
// mismatch that would also read as negative still comes out meter_reset
// (M-2), and R92(4) extends that same precedence to a negative segment
// found anywhere earlier in the window, not only one that would coincide
// with this check.
//
// R93 (reverses I-16): a reset row that participates in the window — inside
// (start.TS, end.TS], or at the start.TS/end.TS instant per R92(2)/R90 —
// but has NO value for a register that BOTH boundary readings report
// carries UNUSABLE evidence for that register: the register is
// ReasonMeterReset-suspect, never a plain difference across the reset. This
// holds even when another reset row in the same window does carry the
// register — a single partial row is enough, because we cannot tell what
// happened to that register at the meter event the partial row records. A
// register that NEITHER boundary reports is untouched by this rule and
// stays nil with no suspicion, as always (it is simply unreported). Plain
// difference across a reset only happens when NO reset row participates in
// the window at all for either boundary (the true I-2 "no reset evidence"
// case) — see the fast path above and the len(windowResets)==0 branch
// below.
//
// Suspicion.ResetRows on every Suspicion this function returns is the
// count of resets inside the segmentation window (start.TS, end.TS],
// regardless of whether they carried evidence for the suspect register,
// PLUS one more when a start-side reset (R92(2)) was inspected for that
// register (whether or not it carried a value — R93 makes an unusable
// start-side row suspect too) — it lets the operator message distinguish
// "a reset exists but could not be applied" from "no reset at all" (line up
// with the same rule in Task 1's Suspicion doc).
//
// Registers untouched by any problem keep their derived values (R58). No
// rounding, no modulus, no rollover arithmetic (R57).
func Derive(w Window, start, end *Reading, resets []Reading, priors []Reading) Derivation {
	d := Derivation{Window: w}

	if start == nil || end == nil {
		// 02 §3.1: a missing boundary yields no row.
		return d
	}
	if start.TS.Equal(end.TS) {
		// R92(1): a zero-width pair never emits, whatever the kinds — see
		// the doc above for why the original kind-sensitive check let a
		// same-instant reset/reading pair through.
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

	// M-3/R92(5): never trust a reset row of the wrong kind. priors's kind
	// precondition is enforced lazily, inside priorValueBefore, so the
	// no-reset fast path below still never touches priors at all.
	resets = filterKind(resets, KindReset)

	windowResets := inEvidenceWindow(resets, start.TS, end.TS)
	startReset := lastResetAt(resets, start.TS) // R92(2)/(3)

	// M-13: a boundary reading of KindReset is always treated as reset
	// evidence at its own instant, whether or not the caller also included
	// it in resets. Task 7 never selects a reset row as a SelectBoundary
	// result, but a caller must not have to duplicate a reset-kind boundary
	// into resets for Derive to see it as evidence. end is appended (not
	// prepended) because inEvidenceWindow already returns entries in
	// ascending TS order and end.TS is the window's maximum instant — this
	// only fires when no element of resets already sits at end.TS, so the
	// slice stays sorted. On the start side, no analogous append is needed:
	// deriveRegister's R92(2) check is gated on start.Kind != KindReset, so
	// a start.Kind == KindReset boundary is never inspected as ambiguous
	// evidence regardless of startReset's value (see M-8) — startReset is
	// still set here for documentation symmetry with the ruling's wording,
	// but it changes no behaviour.
	if end.Kind == KindReset && lastResetAt(resets, end.TS) == nil {
		windowResets = append(windowResets, *end)
	}
	if start.Kind == KindReset && startReset == nil {
		startReset = start
	}

	if len(windowResets) == 0 && startReset == nil {
		// I-2 fast path: no reset evidence in the window for any register,
		// and no start-side reset to check either, so priors is never
		// touched. Delegate to Difference's exact per-register semantics
		// rather than duplicating them.
		plain := Difference(w, start, end)
		d.Values = plain.Values
		d.Suspect = plain.Suspect
		return d
	}

	values := make(map[Register]*decimal.Decimal, len(allRegisters))
	suspect := make(map[Register]Suspicion)

	for _, reg := range allRegisters {
		val, susp := deriveRegister(reg, start, end, windowResets, startReset, priors)
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
// slice is never mutated or aliased into the result. A reset with TS ==
// startTS is deliberately excluded here — R92(2) inspects that case
// separately, outside this window, via lastResetAt.
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

// lastResetAt returns a pointer aliasing the LAST element of resets (sorted
// ascending by TS) whose TS exactly equals ts, or nil when none does.
// R92(3): both the start-side (R92(2)) and end-side (R90) checks use the
// last reset row at a shared instant — the same tie-break rule
// SelectBoundary uses for readings, implemented the same way (binary
// search for the first TS after ts, then step back one and confirm the
// exact match). The primary key (analyzer_id, ts, kind) makes it
// impossible for more than one reset row to actually share an instant, but
// the tie-break exists so the rule is well defined regardless (M-17).
func lastResetAt(resets []Reading, ts time.Time) *Reading {
	i := sort.Search(len(resets), func(i int) bool { return resets[i].TS.After(ts) })
	if i == 0 {
		return nil
	}
	last := &resets[i-1]
	if !last.TS.Equal(ts) {
		return nil
	}
	return last
}

// filterKind returns readings unchanged when every element already has
// kind — the expected shape once a caller filters by kind before calling
// Derive, as Task 7's readers do — costing one O(n) scan and no
// allocation. Only a genuinely mixed slice pays for a fresh copy
// (M-3/R92(5)): Derive must not trust an element of the wrong kind as
// evidence. Relative order is preserved, so sorted-ascending input stays
// sorted, and the input slice is never mutated.
func filterKind(readings []Reading, kind Kind) []Reading {
	for i, r := range readings {
		if r.Kind != kind {
			out := make([]Reading, 0, len(readings)-1)
			out = append(out, readings[:i]...)
			for _, r2 := range readings[i+1:] {
				if r2.Kind == kind {
					out = append(out, r2)
				}
			}
			return out
		}
	}
	return readings
}

// deriveRegister derives one register's consumption over [start, end],
// applying the R92(2) start-side check, reset segmentation (§3.2, R90/R91)
// when the register has usable reset evidence, and a plain difference
// (§3.1) otherwise. It returns a nil value with a nil Suspicion when the
// register is simply unreported (never treated as zero, removed-behaviour
// 21).
func deriveRegister(reg Register, start, end *Reading, windowResets []Reading, startReset *Reading, priors []Reading) (*decimal.Decimal, *Suspicion) {
	startValue := start.Value(reg)
	endValue := end.Value(reg)
	if startValue == nil || endValue == nil {
		return nil, nil
	}

	resetRows := len(windowResets)

	// R92(2)/I-6: the start-side mirror of R90. A reset row at start.TS is
	// never a segmentation point (M-8) — see inEvidenceWindow's open lower
	// bound — but when start is not itself that reset row, its value at the
	// shared instant is ambiguous old/new-meter evidence exactly like R90's
	// end-side case. start counts as trustworthy post-reset evidence for
	// reg only when its value equals the start-side reset row's own value
	// for reg; a mismatch is meter_reset-suspect, decided before any
	// segmentation is attempted (mirrors R90's ordering relative to
	// finalDelta, and R92(4)'s precedence over any later negative segment).
	//
	// R93 (reverses I-16): resetRows counts this row whenever it is
	// inspected here, whether or not it carries reg — a reset row that
	// PARTICIPATES at start.TS but has no value for reg is unusable
	// evidence for reg, not silent non-evidence, since both boundaries
	// report reg (checked above).
	if startReset != nil && start.Kind != KindReset {
		resetRows++
		srVal := startReset.Value(reg)
		if srVal == nil {
			return nil, &Suspicion{Reason: ReasonMeterReset, ResetRows: resetRows}
		}
		if !startValue.Equal(*srVal) {
			return nil, &Suspicion{Reason: ReasonMeterReset, ResetRows: resetRows}
		}
	}

	// R93 (reverses I-16): a reset row inside the segmentation window that
	// has no value for reg is unusable evidence for reg — never a plain
	// difference across it — because both boundaries report reg (checked
	// above) and we cannot tell what happened at that meter event for reg.
	// This fires even when OTHER resets in the same window do carry reg:
	// one partial row is enough to make reg suspect. After this loop,
	// windowResets is safe to use directly as every element (if any) is
	// guaranteed to carry reg.
	for _, r := range windowResets {
		if r.Value(reg) == nil {
			return nil, &Suspicion{Reason: ReasonMeterReset, ResetRows: resetRows}
		}
	}

	if len(windowResets) == 0 {
		// I-2: no reset evidence anywhere in the window for ANY register
		// (R93 above already handles "resets exist but omit reg"), so
		// nothing marks a segmentation point: plain difference (§3.1),
		// suspect only when it happens to be negative.
		delta := endValue.Sub(*startValue)
		if delta.IsNegative() {
			return nil, &Suspicion{Reason: ReasonNegativeDelta, Delta: &delta, ResetRows: resetRows}
		}
		return &delta, nil
	}
	regResets := windowResets

	total := decimal.Zero
	segStart := startValue
	prevTS := start.TS
	var negative *Suspicion
	resetProblem := false

	for _, r := range regResets {
		before := priorValueBefore(priors, reg, prevTS, r.TS)
		if before == nil {
			// R91/I-1: a missing before-reset value is never a zero
			// contribution, and never falls back to an older reading.
			// R92(4)/I-5: do not return yet. meter_reset must win over a
			// negative segment found anywhere else in the window, so keep
			// scanning the remaining resets — and, after the loop, the R90
			// end check — before deciding; the answer is only final once
			// every segment has been examined.
			resetProblem = true
			segStart = r.Value(reg) // after_reset: keep segmenting so later
			prevTS = r.TS           // problems are still discoverable.
			continue
		}

		delta := before.Sub(*segStart)
		if delta.IsNegative() {
			if negative == nil {
				// M-2: the FIRST failing segment's own difference, in time
				// order — never the whole period's — wins if nothing worse
				// (a reset-evidence problem, R92(4)) turns up later.
				d2 := delta
				negative = &Suspicion{Reason: ReasonNegativeDelta, Delta: &d2, ResetRows: resetRows}
			}
		} else if !resetProblem {
			total = total.Add(delta)
		}

		segStart = r.Value(reg) // after_reset: the reset row's own value.
		prevTS = r.TS
	}

	if resetProblem {
		// R92(4)/I-5: some segment's before-reset value was missing, so
		// meter_reset wins regardless of any negative segment found above.
		return nil, &Suspicion{Reason: ReasonMeterReset, ResetRows: resetRows}
	}

	// R90: the last reset carrying this register's value sits exactly at
	// end.TS, and end is not that reset row itself. end's value must equal
	// after_reset exactly (segStart, just set above), or it is ambiguous
	// old-meter/new-meter evidence and the register is suspect rather than
	// billed as a number. This check happens BEFORE finalDelta is computed
	// on purpose (M-2): meter_reset must win even when the mismatch would
	// also read as a negative difference, and R92(4) extends the same
	// precedence over any negative segment already found earlier in the
	// loop above.
	if last := regResets[len(regResets)-1]; last.TS.Equal(end.TS) && end.Kind != KindReset {
		if !endValue.Equal(*segStart) {
			return nil, &Suspicion{Reason: ReasonMeterReset, ResetRows: resetRows}
		}
		if negative != nil {
			// R92(4): the end-of-window ambiguity resolved cleanly (no
			// reset-evidence problem there), but an earlier segment was
			// still genuinely negative — that stands on its own.
			return nil, negative
		}
		// The trailing segment is zero by construction: no time passes
		// between the reset row and a reading at the same instant.
		return &total, nil
	}

	finalDelta := endValue.Sub(*segStart)
	if finalDelta.IsNegative() && negative == nil {
		negative = &Suspicion{Reason: ReasonNegativeDelta, Delta: &finalDelta, ResetRows: resetRows}
	}
	if negative != nil {
		return nil, negative
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
// M-3/R92(5): a candidate reading of the wrong kind (not KindLoadProfile)
// is skipped as if it were never in priors at all, and the search steps
// back to the next candidate — it is never trusted as before-reset
// evidence, and it never counts as "the single last reading" that I-1's
// no-fallback rule pins to.
//
// The lookup starts with a binary search (I-2): the first index with
// TS >= before, stepped back one. Stepping further back only happens to
// skip a wrong-kind reading; a same-kind reading is decided immediately,
// so the well-typed input this precondition expects still costs O(log P).
func priorValueBefore(priors []Reading, reg Register, after, before time.Time) *decimal.Decimal {
	i := sort.Search(len(priors), func(i int) bool { return !priors[i].TS.Before(before) })
	for i > 0 {
		i--
		candidate := priors[i]
		if candidate.Kind != KindLoadProfile {
			continue
		}
		if !candidate.TS.After(after) {
			return nil
		}
		return candidate.Value(reg)
	}
	return nil
}
