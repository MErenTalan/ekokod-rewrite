package energy

import "github.com/shopspring/decimal"

// Difference implements 02 §3.1 for one window with no reset evidence.
// start and end are the readings selected by "the last reading with
// ts <= bound". Either may be nil. A negative difference is left suspect
// with ReasonNegativeDelta; applying reset evidence is Derive's job (Task 2).
//
// R92(1), amending the original identical-boundary rule: a same-instant
// pair (start.TS.Equal(end.TS)) never emits, whatever the kinds. The
// original rule only treated this as "no row" when the kinds also matched,
// so a same-instant pair of different kinds (e.g. a load_profile start and
// a reset row landing on the same instant) fell through and derived a
// plain difference across zero elapsed time — I-6's zero-width defect. A
// zero-width window measures nothing, so it is never emitted regardless of
// what produced each reading.
//
// Precondition: !end.TS.Before(start.TS). A caller must select start and end
// in chronological order; Difference does not infer intent from a reversed
// pair, and treats one as "no row" (Emitted: false) rather than emitting a
// negative-delta suspicion that a caller bug, not a meter event, produced.
//
// Difference never rounds (R79) and never mutates its inputs: it only reads
// start and end's Values maps and produces new decimal.Decimal values.
//
// !Emitted always leaves Values and Suspect nil, never an empty non-nil
// map: a caller must be able to tell "no row" apart from "a row with
// nothing suspect" without inspecting Emitted at all.
func Difference(w Window, start, end *Reading) Derivation {
	d := Derivation{Window: w}

	if start == nil || end == nil {
		// 02 §3.1: "if either reading is missing ... the period yields no
		// row". Emitted stays false and Values/Suspect stay nil — a caller
		// must not turn this into a zero row.
		return d
	}
	if start.TS.Equal(end.TS) {
		// R92(1): a zero-width pair never emits, whatever the kinds — see
		// the doc above for why the original kind-sensitive check let a
		// same-instant reset/reading pair through.
		return d
	}
	if end.TS.Before(start.TS) {
		// Boundaries out of order is a caller bug (see precondition above),
		// not a meter event: emit nothing rather than a negative-delta
		// suspicion that would write a spurious anomaly downstream.
		return d
	}

	d.Emitted = true
	d.Source = end.Kind
	values := make(map[Register]*decimal.Decimal, len(allRegisters))
	suspect := make(map[Register]Suspicion)

	for _, reg := range allRegisters {
		startValue := start.Value(reg)
		endValue := end.Value(reg)
		if startValue == nil || endValue == nil {
			// The meter did not report this register on one of the
			// boundaries: absent, never zero, and never suspect
			// (removed-behaviour 21) — the difference simply cannot be
			// computed, which is a different situation from a computed
			// difference that came out wrong.
			values[reg] = nil
			continue
		}

		delta := endValue.Sub(*startValue)
		if delta.IsNegative() {
			// removed-behaviour 1: never return the end register value
			// itself. The register decreased; without reset evidence this
			// register is suspect, and its consumption is nil, never a
			// number. delta is redeclared fresh on every loop iteration
			// (the `:=` above), so taking its address here is safe and
			// unique per register.
			suspect[reg] = Suspicion{Reason: ReasonNegativeDelta, Delta: &delta}
			values[reg] = nil
			continue
		}

		values[reg] = &delta
	}

	d.Values = values
	d.Suspect = suspect
	return d
}
