package postgres

import (
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgnum"
)

// The single audited conversion pair between the generated types and the
// domain model, moved to internal/store/postgres/internal/pgnum at the Wave F
// integration commit so that package admin — a separate package that cannot
// reach an unexported function here — can call it directly instead of
// carrying its own verbatim copy. These four functions are one-line
// delegating wrappers kept under their original unexported names so that no
// repository call site in this package changes; pgnum.go documents the pair
// itself, including why it exists and the test that proves it is exact.

// numericToDecimal converts a NOT NULL numeric column to a decimal.Decimal,
// exactly. See pgnum.NumericToDecimal.
func numericToDecimal(n pgtype.Numeric) (decimal.Decimal, error) {
	return pgnum.NumericToDecimal(n)
}

// numericToDecimalPtr is numericToDecimal for a NULLABLE column. See
// pgnum.NumericToDecimalPtr.
func numericToDecimalPtr(n pgtype.Numeric) (*decimal.Decimal, error) {
	return pgnum.NumericToDecimalPtr(n)
}

// decimalToNumeric converts a decimal.Decimal to the pgtype.Numeric the
// generated code writes, exactly and without error. See
// pgnum.DecimalToNumeric.
func decimalToNumeric(d decimal.Decimal) pgtype.Numeric {
	return pgnum.DecimalToNumeric(d)
}

// decimalPtrToNumeric is decimalToNumeric for a NULLABLE column. See
// pgnum.DecimalPtrToNumeric.
func decimalPtrToNumeric(d *decimal.Decimal) pgtype.Numeric {
	return pgnum.DecimalPtrToNumeric(d)
}
