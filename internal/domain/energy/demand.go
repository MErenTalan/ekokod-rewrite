package energy

import "github.com/shopspring/decimal"

// MaxDemandKinds is the allowlist of reading kinds MaxDemand considers by
// default — the widest, Monthly/Yearly set (R101, amending R65); a caller
// wanting the narrower Hourly or Daily set restricts the readings it passes
// to MaxDemand before calling it. current_index is deliberately excluded:
// ARIL stamps those rows at ProfileDate, so a current_index row can carry
// the previous month's peak — the misattribution F2's R49 fixed. ARIL's
// authoritative monthly maximum instead arrives on billing rows keyed to
// the Istanbul month of the row's own timestamp (verified in
// internal/integration/aril/source.go). reset rows are excluded too: a
// reset row is evidence of a physical register reset, not a demand
// reading.
var MaxDemandKinds = []Kind{KindLoadProfile, KindDaily, KindBilling}

func maxDemandKindAllowed(k Kind) bool {
	for _, allowed := range MaxDemandKinds {
		if k == allowed {
			return true
		}
	}
	return false
}

// MaxDemand is the maximum MaxDemandKw over readings whose TS falls inside
// w (half-open: [From, To)) and whose Kind is in MaxDemandKinds — never a
// difference and never a sum (§3.5). nil when no matching reading carries a
// value.
func MaxDemand(w Window, readings []Reading) *decimal.Decimal {
	var max *decimal.Decimal
	for _, r := range readings {
		if !w.Contains(r.TS) {
			continue
		}
		if !maxDemandKindAllowed(r.Kind) {
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
