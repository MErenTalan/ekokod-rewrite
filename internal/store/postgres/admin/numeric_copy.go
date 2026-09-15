// DUPLICATE(internal/store/postgres/numeric.go): verbatim copy, unified into
// one internal package at the Wave F merge — do not edit.
package admin

import (
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// decimalToNumeric is a SECOND copy of the coefficient/exponent relabelling
// internal/store/postgres/numeric.go documents as "the ONE audited pair" —
// deliberately, not out of oversight. That pair is unexported, private to
// package postgres, and package admin (this package) is a SEPARATE package
// from postgres, one level below both alongside pgerr — it cannot call an
// unexported function in another package. Unlike pgerr.Translate, which was
// carved into its own importable package precisely so admin and postgres
// could share it, numeric.go's functions were not, and this task must not
// unilaterally relocate shared infrastructure two other parallel tasks
// (9 and 11) are already calling by its current unqualified name inside
// package postgres — that move belongs to a controller decision, flagged in
// this task's report, not to a silent refactor here.
//
// This copy is exact and minimal: Coefficient()/Exponent() is the same
// no-op relabelling numeric.go's own does, never a float64 round-trip.
// Every value this file converts (MarketPrice.PTF, YekdemMonthly.Value) is a
// NOT NULL decimal.Decimal, so there is no NULL/pointer branch to duplicate.
func decimalToNumeric(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}
