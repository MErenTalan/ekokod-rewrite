package energy

import "github.com/shopspring/decimal"

// MaxDemandKinds is the allowlist of reading kinds MaxDemand considers
// (C-3, R65 re-ruled). current_index is deliberately excluded: ARIL stamps
// those rows at ProfileDate, so a current_index row can carry the previous
// month's peak — the misattribution F2's R49 fixed. ARIL's authoritative
// monthly maximum instead arrives on billing rows keyed to the Istanbul
// month of the row's own timestamp (verified in
// internal/integration/aril/source.go). reset rows are excluded too: a
// reset row is evidence of a physical register reset, not a demand
// reading.
var MaxDemandKinds = []Kind{KindLoadProfile, KindDaily, KindBilling}

func maxDemandKindAllowed(kinds []Kind, k Kind) bool {
	for _, allowed := range kinds {
		if k == allowed {
			return true
		}
	}
	return false
}

// MaxDemandKindsFor is R101's amendment to R65: the max-demand kinds allowed
// depend on the requesting Level, so a day's or month's stamped peak can no
// longer leak into a single finer hour or day the way an unqualified
// MaxDemandKinds allowlist let it (final review A M-2). Hourly allows
// load_profile only — nothing coarser has a meaningful per-hour peak. Daily
// adds daily-kind rows (a day's own stamped peak). Monthly and Yearly add
// billing-kind rows too (MaxDemandKinds' original set) — a billing row's
// peak is a whole month's, only ever attributable at Monthly/Yearly. An
// unrecognised Level returns nil; callers are expected to have validated
// Level already.
func MaxDemandKindsFor(level Level) []Kind {
	switch level {
	case Hourly:
		return []Kind{KindLoadProfile}
	case Daily:
		return []Kind{KindLoadProfile, KindDaily}
	case Monthly, Yearly:
		return []Kind{KindLoadProfile, KindDaily, KindBilling}
	default:
		return nil
	}
}

// MaxDemandOf is MaxDemand generalised to an explicit kind allowlist (R101):
// the maximum MaxDemandKw over readings whose TS falls inside w (half-open:
// [From, To)) and whose Kind is in kinds — never a difference and never a
// sum (§3.5). nil when no matching reading carries a value.
func MaxDemandOf(w Window, readings []Reading, kinds []Kind) *decimal.Decimal {
	var max *decimal.Decimal
	for _, r := range readings {
		if !w.Contains(r.TS) {
			continue
		}
		if !maxDemandKindAllowed(kinds, r.Kind) {
			continue
		}
		if r.MaxDemandKw == nil {
			continue
		}
		if max == nil || r.MaxDemandKw.GreaterThan(*max) {
			v := *r.MaxDemandKw
			max = &v
		}
	}
	return max
}

// MaxDemand is the maximum MaxDemandKw over readings whose TS falls inside w
// (half-open: [From, To)) and whose Kind is in MaxDemandKinds — never a
// difference and never a sum (§3.5). nil when no matching reading carries a
// value. It is exactly MaxDemandOf(w, readings, MaxDemandKinds), kept as its
// own function because most existing callers and tests have no Level to
// hand and want the original, unqualified allowlist. A caller that DOES have
// a Level should prefer MaxDemandOf(w, readings, MaxDemandKindsFor(level))
// (R101) so a coarser kind's peak cannot land inside a finer window.
func MaxDemand(w Window, readings []Reading) *decimal.Decimal {
	return MaxDemandOf(w, readings, MaxDemandKinds)
}
