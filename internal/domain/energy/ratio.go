package energy

import "github.com/shopspring/decimal"

// DivisionScale is how many decimal places every division in this package is
// carried to (C-5). It exists for precision, not presentation (R79 as
// amended): storage rounding happens once, when Postgres writes a value into
// a numeric column, and presentation rounding is F6's — nothing in this
// package rounds for either purpose.
const DivisionScale int32 = 20

// Ratio divides numerator by denominator and is nil whenever the result
// would be meaningless: a nil numerator, or a nil, zero or negative
// denominator. Otherwise it is numerator/denominator carried to
// DivisionScale places.
//
// 02-domain-rules.md §3.3 says the ratio is "0 when active_import = 0";
// 09-implementation-plan.md §F3's acceptance criterion requires a null
// ratio for a zero-consumption period and forbids computing it against a
// substituted 1, and the acceptance criterion wins (R54, the F0/F1 standing
// ruling on spec-vs-acceptance conflicts). The forbidden substitution is a
// real legacy defect, not a hypothetical one: reactivePenalty.ts:47 did
// `const safeActive = totalActive === 0 ? 1 : totalActive`, so a
// zero-consumption period reported its raw reactive quantity as if it were
// a ratio — see 10-removed-behaviours.md item 15 ("division-by-zero was
// masked rather than handled"). Ratio never reproduces that: a
// non-positive or missing denominator, or a missing numerator, yields nil,
// never a substituted value.
func Ratio(numerator, denominator *decimal.Decimal) *decimal.Decimal {
	if numerator == nil || denominator == nil {
		return nil
	}
	if !denominator.IsPositive() {
		return nil
	}
	result := numerator.DivRound(*denominator, DivisionScale)
	return &result
}

// Ratios returns the inductive and capacitive ratios of a derivation
// (§3.3): reactive_inductive_import/active_import and
// reactive_capacitive_import/active_import. Both are nil when d was not
// emitted or when active_import is nil or suspect — Ratio's own nil rules
// then also apply (a zero or negative active_import yields nil regardless).
func Ratios(d Derivation) (inductive, capacitive *decimal.Decimal) {
	if !d.Emitted {
		return nil, nil
	}
	if _, suspect := d.Suspect[ActiveImport]; suspect {
		return nil, nil
	}
	active := d.Values[ActiveImport]
	inductive = Ratio(d.Values[ReactiveInductiveImport], active)
	capacitive = Ratio(d.Values[ReactiveCapacitiveImport], active)
	return inductive, capacitive
}
