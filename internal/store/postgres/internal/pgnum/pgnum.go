// Package pgnum holds the single audited conversion pair between the
// generated pgtype.Numeric and the domain model's decimal.Decimal.
//
// 251 columns in this schema are `numeric`, and every one of them is money, a
// price, an energy quantity or a coefficient that multiplies one. sqlc reads
// them as pgtype.Numeric; model structs hold decimal.Decimal. Something has to
// convert, and where that conversion lives decides whether this schema's
// exactness survives contact with Go.
//
// It lives HERE, once, so that every caller under internal/store/postgres —
// package postgres itself (through numeric.go's unexported delegating
// wrappers, kept so no repository call site changes) and package admin, the
// spec's only unscoped query surface, which cannot reach postgres's
// unexported functions because it is a separate package — converts through
// the same code. A per-repository or per-package conversion is not a style
// preference to be argued about: the obvious one-liner,
//
//	f, _ := n.Float64Value()          // DO NOT
//	d := decimal.NewFromFloat(f.Float64)
//
// compiles, reads naturally, passes any test written with small round numbers,
// and silently destroys precision on exactly the values this phase exists to
// protect. float64 carries about 15-17 significant decimal digits;
// numeric(18,6) has 18 and numeric(18,8) has 18 with eight after the point, so
// the loss is not hypothetical. pgnum_test.go asserts this pair is exact on a
// 29-digit value, and that test has been RUN against a float64-based
// implementation to confirm it fails — see the Wave F integration report.
//
// Both directions go through the coefficient and exponent, which is not a
// conversion at all but a relabelling: pgtype.Numeric is {Int *big.Int,
// Exp int32} and decimal.Decimal is {value *big.Int, exp int32}, both meaning
// Int x 10^Exp. No base change, no rounding, no intermediate representation
// that could round. A string round-trip would also be exact, but it would
// parse and format on every one of those 251 columns per row, for no gain.
//
// Before Wave F this pair lived unexported in internal/store/postgres's own
// numeric.go, and package admin carried a verbatim copy (two, briefly, from
// Tasks 10 and 11b converging on the same file) because it could not reach an
// unexported function in a sibling package. The Wave F integration commit
// moved the implementation here — one level below both postgres and admin,
// alongside pgerr, which solved the identical problem for error translation —
// and deleted every copy.
package pgnum

import (
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

var (
	// ErrNull is returned when a NOT NULL column arrives as SQL NULL.
	// It is a distinct error rather than a zero Decimal because zero is a
	// legitimate meter reading, price and charge: silently substituting it
	// would turn a missing value into a real one, and 02-domain-rules.md
	// §3.1 is explicit that a missing reading yields no row rather than a
	// zero. Callers reading a nullable column use NumericToDecimalPtr, which
	// reports NULL as a nil pointer instead.
	ErrNull = errors.New("numeric is null")

	// ErrNotFinite is returned for NaN and +/-Infinity. Postgres permits NaN
	// in a `numeric` column, and neither NaN nor an infinity has any
	// representation in decimal.Decimal — decimal is a coefficient and an
	// exponent, with nowhere to put one. Rejecting is the only honest
	// outcome; mapping either to zero would put a fabricated number on an
	// invoice.
	ErrNotFinite = errors.New("numeric is not finite")
)

// NumericToDecimal converts a NOT NULL numeric column to a decimal.Decimal,
// exactly.
//
// It returns ErrNull for a SQL NULL and ErrNotFinite for NaN or an infinity.
// A Valid numeric with a nil Int is zero: pgtype.Numeric{Valid: true} is the
// natural literal for zero and pgx itself produces a nil Int for no other
// value, so treating it as zero is reading the value as written, not papering
// over a malformed one.
func NumericToDecimal(n pgtype.Numeric) (decimal.Decimal, error) {
	if !n.Valid {
		return decimal.Decimal{}, ErrNull
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite {
		return decimal.Decimal{}, ErrNotFinite
	}
	if n.Int == nil {
		return decimal.New(0, n.Exp), nil
	}
	// NewFromBigInt copies the coefficient, so the returned Decimal does not
	// alias n.Int and a later mutation of either cannot reach the other.
	return decimal.NewFromBigInt(n.Int, n.Exp), nil
}

// NumericToDecimalPtr is NumericToDecimal for a NULLABLE column: a SQL NULL
// becomes a nil pointer rather than an error.
//
// The nil is the whole point. A nullable numeric column in this schema means
// the provider did not report the register, or the tariff does not define
// that price — states that must stay distinguishable from zero all the way up
// to the model struct, which is why every nullable numeric maps to
// *decimal.Decimal there.
func NumericToDecimalPtr(n pgtype.Numeric) (*decimal.Decimal, error) {
	if !n.Valid {
		return nil, nil
	}
	d, err := NumericToDecimal(n)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// DecimalToNumeric converts a decimal.Decimal to the pgtype.Numeric the
// generated code writes, exactly and without error: every decimal.Decimal is a
// finite coefficient and exponent, which is precisely what a Valid
// pgtype.Numeric holds.
//
// The exponent is preserved rather than normalised, so a value read from a
// numeric(18,6) column and written straight back keeps its scale. Postgres
// rescales to the column's declared scale on write in any case; preserving it
// here means a value that never touches the database still compares equal to
// the one that did.
//
// EXCESS SCALE IS PASSED THROUGH, NOT ROUNDED HERE. A decimal with more
// fractional digits than the column declares (2.4512345 into numeric(18,6))
// reaches Postgres intact and the column's typmod rounds it half away from
// zero on write, so the stored value differs from the Go value without any
// error. Callers therefore round in DOMAIN code, under the rounding rule the
// business specifies for that quantity, before the value reaches a
// repository; this function does not guess a rounding mode on their behalf.
func DecimalToNumeric(d decimal.Decimal) pgtype.Numeric {
	// Coefficient returns a fresh big.Int, so the result does not alias d's
	// internals.
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

// DecimalPtrToNumeric is DecimalToNumeric for a NULLABLE column: a nil
// pointer becomes SQL NULL.
//
// The zero value pgtype.Numeric{} has Valid false, which is how pgx encodes
// NULL, so the nil branch is deliberately the zero value and not a Numeric
// holding zero.
func DecimalPtrToNumeric(d *decimal.Decimal) pgtype.Numeric {
	if d == nil {
		return pgtype.Numeric{}
	}
	return DecimalToNumeric(*d)
}
