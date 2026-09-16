package consumption

import (
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// isSuspectRegister is the ONE place that answers "is reg one of r's suspect
// registers" (Task 9 fix round 1). soundValue and soundIndex below both call
// it rather than each re-deriving the same map lookup, so the rule — a
// register listed in Row.Suspect must never surface as a number or a zero in
// any output — cannot drift between the callers that need it.
func isSuspectRegister(r Row, reg energy.Register) bool {
	_, suspect := r.Suspect[reg]
	return suspect
}

// soundValue returns r.Values[reg], or nil when reg is suspect for r — even
// when r.Values[reg] is itself non-nil, which the real pipeline
// (Difference/Derive) never produces but a hand-built Row can (that is
// exactly the gap task-9-review.md flagged against Balance and ExportRows).
// Summarise, Balance and exportRow all call this instead of reading
// r.Values[reg] directly, so a suspect register can never render as a
// number through any of the three.
func soundValue(r Row, reg energy.Register) *decimal.Decimal {
	if isSuspectRegister(r, reg) {
		return nil
	}
	return r.Values[reg]
}

// soundIndex is soundValue's sibling for r.Indexes: a suspect register's
// closing index must not render as sound either (task-9-review.md, minor
// finding). exportRow calls this for every <register>_index cell.
func soundIndex(r Row, reg energy.Register) *decimal.Decimal {
	if isSuspectRegister(r, reg) {
		return nil
	}
	return r.Indexes[reg]
}
