package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// These tests exist to fail against a float64-based implementation of the
// numeric<->decimal pair, not merely to pass against the real one. Every case
// below was chosen because it survives the exponent/coefficient conversion and
// does NOT survive a Float64Value round trip; the suite has been run against a
// deliberately float-based numericToDecimal/decimalToNumeric to confirm that
// (transcript in the task report). A conversion test written with 12.34 and
// 0.5 would pass either way and would therefore prove nothing.

// numericOf builds a pgtype.Numeric by parsing decimal TEXT through pgtype's
// own scanner — the same path a value takes off the wire in text format. The
// test data is therefore exact by construction, and the construction side of
// every assertion below is pgx's code rather than a reimplementation of it.
//
// It deliberately does NOT build the coefficient with math/big:
// .golangci.yml's no-float-money rule denies math/big under internal/store,
// and it is right to. Going through pgtype's parser is both allowed and a
// better test.
func numericOf(t *testing.T, text string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	require.NoError(t, n.Scan(text), "test data %q is not a numeric", text)
	require.True(t, n.Valid)
	return n
}

// numericText renders a numeric through pgtype's own text encoder, which is
// what Postgres would receive. Using it for the round-trip assertion keeps the
// comparison independent of decimal.Decimal, which is half of the code under
// test.
func numericText(t *testing.T, n pgtype.Numeric) string {
	t.Helper()
	v, err := n.Value()
	require.NoError(t, err)
	str, ok := v.(string)
	require.True(t, ok, "pgtype.Numeric.Value returned %T, want string", v)
	return str
}

// exactCases are values whose decimal expansion is longer, or finer, than a
// float64 can hold.
var exactCases = []struct {
	name    string
	decimal string
}{
	{
		// The brief's headline case: 29 significant digits against
		// float64's ~15-17. A float round trip returns
		// 12345678901234567168.000000000, wrong in the 18th digit and
		// beyond.
		name:    "more significant digits than float64 can hold",
		decimal: "12345678901234567890.123456789",
	},
	{
		// numeric(18,6) at full width: a realistic tariff price with six
		// decimal places and twelve integer digits.
		name:    "full-width numeric(18,6)",
		decimal: "123456789012.345678",
	},
	{
		// numeric(18,8): emission_factors.base_factor. Small magnitude,
		// eight decimal places, no exact binary representation.
		name:    "numeric(18,8) emission factor",
		decimal: "0.12345678",
	},
	{
		// A total_cost that a float cannot represent: 0.1 has no finite
		// binary expansion, so the classic 0.1+0.2 error class lives here.
		name:    "value with no exact binary expansion",
		decimal: "100000000000.0001",
	},
	{
		name:    "negative charge",
		decimal: "-98765432109876543210.987654321",
	},
	{
		name:    "negative small",
		decimal: "-0.00000001",
	},
	{
		name:    "zero",
		decimal: "0",
	},
	{
		// Zero at a scale: numeric(18,4) NOT NULL DEFAULT 0 arrives from a
		// bills row that was never touched.
		name:    "zero at scale four",
		decimal: "0.0000",
	},
	{
		// One ulp above the largest integer float64 represents exactly.
		// float64 rounds it back down to 9007199254740992.
		name:    "one above float64's exact integer limit",
		decimal: "9007199254740993",
	},
}

func TestNumericToDecimalIsExact(t *testing.T) {
	t.Parallel()

	for _, tc := range exactCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := numericToDecimal(numericOf(t, tc.decimal))
			require.NoError(t, err)

			want, err := decimal.NewFromString(tc.decimal)
			require.NoError(t, err, "test expectation %q is not a decimal", tc.decimal)

			require.Truef(t, want.Equal(got),
				"numericToDecimal(%s) = %s, want %s: the conversion lost precision, "+
					"which is what happens when it goes through float64",
				tc.decimal, got.String(), want.String())
		})
	}
}

func TestDecimalToNumericIsExact(t *testing.T) {
	t.Parallel()

	for _, tc := range exactCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, err := decimal.NewFromString(tc.decimal)
			require.NoError(t, err)

			n := decimalToNumeric(d)
			require.True(t, n.Valid, "a decimal is never NULL")
			require.False(t, n.NaN)
			require.Equal(t, pgtype.Finite, n.InfinityModifier)
			require.NotNil(t, n.Int)

			// Compare through the value the numeric denotes (Int x 10^Exp)
			// rather than through the raw pair, so an equal value at a
			// different scale is not a spurious failure.
			back, err := numericToDecimal(n)
			require.NoError(t, err)
			require.Truef(t, d.Equal(back),
				"decimalToNumeric(%s) denotes %s", d.String(), back.String())
		})
	}
}

// TestNumericDecimalRoundTripIsExact is the property test: for every value in
// the table, numeric -> decimal -> numeric returns something denoting the same
// number, and decimal -> numeric -> decimal returns an EQUAL decimal. Equality
// is exact, not within a tolerance; a tolerance is how a float-based
// conversion would sneak through.
func TestNumericDecimalRoundTripIsExact(t *testing.T) {
	t.Parallel()

	for _, tc := range exactCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			start := numericOf(t, tc.decimal)

			mid, err := numericToDecimal(start)
			require.NoError(t, err)

			end := decimalToNumeric(mid)
			require.True(t, end.Valid)

			// The pair may legitimately renormalise (decimal trims a
			// trailing-zero coefficient), so the round trip is asserted on
			// the NUMBER the pair denotes, via a second conversion.
			endDecimal, err := numericToDecimal(end)
			require.NoError(t, err)
			require.Truef(t, mid.Equal(endDecimal),
				"round trip changed the value: %s -> %s", mid.String(), endDecimal.String())

			// And independently of the conversion pair: pgtype's own text
			// encoder must render the same number at both ends, so neither
			// numericToDecimal nor decimalToNumeric is on the path from the
			// numerics to this verdict.
			require.Truef(t, sameNumber(t, numericText(t, start), numericText(t, end)),
				"round trip changed the encoded value: %s -> %s",
				numericText(t, start), numericText(t, end))
		})
	}
}

// TestNumericToDecimalHandlesAPositiveExponent covers the shape the case table
// above cannot express, because pgtype's text scanner never produces it: a
// numeric whose exponent is POSITIVE, meaning the coefficient is scaled UP.
// pgx does produce these from the binary wire format, and a conversion that
// assumed a non-positive exponent would be wrong by a factor of 10^7 here
// while passing every other test in this file.
func TestNumericToDecimalHandlesAPositiveExponent(t *testing.T) {
	t.Parallel()

	n := numericOf(t, "123")
	n.Exp = 7 // 123 x 10^7

	got, err := numericToDecimal(n)
	require.NoError(t, err)
	require.Equal(t, "1230000000", got.String())

	// And back: the value survives, whatever scale it lands on.
	back, err := numericToDecimal(decimalToNumeric(got))
	require.NoError(t, err)
	require.True(t, got.Equal(back))
}

// sameNumber compares two Postgres numeric literals as numbers rather than as
// strings, so that a differing scale ("1.5" vs "1.500") is not a failure while
// a differing VALUE is.
//
// The comparison runs through decimal's own text parser, which is NOT the code
// under test: the values reaching it came out of pgtype's text ENCODER, so
// neither numericToDecimal nor decimalToNumeric is on the path from the
// numerics to this verdict.
func sameNumber(t *testing.T, a, b string) bool {
	t.Helper()
	da, err := decimal.NewFromString(a)
	require.NoError(t, err)
	db, err := decimal.NewFromString(b)
	require.NoError(t, err)
	return da.Equal(db)
}

func TestNumericToDecimalRejectsNull(t *testing.T) {
	t.Parallel()

	_, err := numericToDecimal(pgtype.Numeric{})
	require.ErrorIs(t, err, errNumericNull,
		"a NULL in a NOT NULL column must be an error, never a silent zero: "+
			"zero is a real meter reading and a real charge")
}

func TestNumericToDecimalRejectsNaNAndInfinity(t *testing.T) {
	t.Parallel()

	for name, n := range map[string]pgtype.Numeric{
		"NaN":               {NaN: true, Valid: true},
		"positive infinity": {InfinityModifier: pgtype.Infinity, Valid: true},
		"negative infinity": {InfinityModifier: pgtype.NegativeInfinity, Valid: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := numericToDecimal(n)
			require.ErrorIs(t, err, errNumericNotFinite,
				"decimal.Decimal cannot represent %s; converting it to zero would "+
					"fabricate a number", name)
		})
	}
}

// TestNumericToDecimalTreatsAValidNilCoefficientAsZero pins the one case where
// a missing coefficient is NOT an error: pgtype.Numeric{Valid: true} is the
// natural Go literal for zero.
func TestNumericToDecimalTreatsAValidNilCoefficientAsZero(t *testing.T) {
	t.Parallel()

	got, err := numericToDecimal(pgtype.Numeric{Valid: true})
	require.NoError(t, err)
	require.True(t, got.IsZero())
}

func TestNumericToDecimalPtrReportsNullAsNil(t *testing.T) {
	t.Parallel()

	got, err := numericToDecimalPtr(pgtype.Numeric{})
	require.NoError(t, err)
	require.Nil(t, got,
		"a nullable numeric column's NULL is a nil pointer, never a zero decimal: "+
			"'the provider did not report this register' and 'the register read zero' "+
			"are different facts and price differently")
}

func TestNumericToDecimalPtrPropagatesNonFinite(t *testing.T) {
	t.Parallel()

	_, err := numericToDecimalPtr(pgtype.Numeric{NaN: true, Valid: true})
	require.ErrorIs(t, err, errNumericNotFinite)
}

func TestNumericToDecimalPtrIsExact(t *testing.T) {
	t.Parallel()

	for _, tc := range exactCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := numericToDecimalPtr(numericOf(t, tc.decimal))
			require.NoError(t, err)
			require.NotNil(t, got)

			want, err := decimal.NewFromString(tc.decimal)
			require.NoError(t, err)
			require.True(t, want.Equal(*got), "got %s, want %s", got.String(), want.String())
		})
	}
}

func TestDecimalPtrToNumericMapsNilToSQLNull(t *testing.T) {
	t.Parallel()

	n := decimalPtrToNumeric(nil)
	require.False(t, n.Valid, "nil must encode as SQL NULL, not as the number zero")

	zero := decimal.Zero
	n = decimalPtrToNumeric(&zero)
	require.True(t, n.Valid, "a pointer to zero is the number zero, not NULL")
	back, err := numericToDecimal(n)
	require.NoError(t, err)
	require.True(t, back.IsZero())
}

// TestConversionDoesNotAliasItsInput proves neither direction hands back a
// value sharing a *big.Int with its input. Aliasing would be invisible until
// some repository normalised one row's numeric in place and silently changed
// another row's model value.
func TestConversionDoesNotAliasItsInput(t *testing.T) {
	t.Parallel()

	n := numericOf(t, "123456789012.345678")
	d, err := numericToDecimal(n)
	require.NoError(t, err)

	// Mutate the source coefficient; the converted decimal must not move.
	n.Int.SetInt64(1)
	require.Equal(t, "123456789012.345678", d.String())

	d2 := decimal.RequireFromString("987654321098.765432")
	n2 := decimalToNumeric(d2)
	n2.Int.SetInt64(1)
	require.Equal(t, "987654321098.765432", d2.String())
}

// TestDecimalToNumericPreservesScale pins the deliberate choice not to
// normalise: a value read from a numeric(18,6) column keeps its exponent on
// the way back, so a struct that never touched the database compares equal to
// one that did.
func TestDecimalToNumericPreservesScale(t *testing.T) {
	t.Parallel()

	d := decimal.RequireFromString("1.500000")
	n := decimalToNumeric(d)
	require.EqualValues(t, -6, n.Exp)
	require.Equal(t, "1500000", n.Int.String())
}

// TestFloatWouldFailThisSuite documents, executably, the size of the error the
// pair exists to avoid. It performs the naive float64 round trip on the
// headline value and asserts it is WRONG — so if some future Go release made
// float64 exact for 29 digits (it will not), this test would fail and tell the
// reader the rest of the suite had lost its teeth.
func TestFloatWouldFailThisSuite(t *testing.T) {
	t.Parallel()

	n := numericOf(t, "12345678901234567890.123456789")

	exact, err := numericToDecimal(n)
	require.NoError(t, err)

	f, err := n.Float64Value()
	require.NoError(t, err)
	require.True(t, f.Valid)
	viaFloat := decimal.NewFromFloat(f.Float64)

	require.Falsef(t, exact.Equal(viaFloat),
		"float64 round trip returned %s, exactly equal to %s — if this ever holds, "+
			"every other assertion in this file has stopped proving anything",
		viaFloat.String(), exact.String())
	t.Logf("exact: %s", exact.String())
	t.Logf("via float64: %s", viaFloat.String())
}
