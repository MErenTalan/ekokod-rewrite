package normalize_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// TestProviderNumberTable is the table half of R10 (06 §2 "numbers …
// sometimes with thousands separators"). Every row comes verbatim from the
// task brief / plan's R10 ruling.
func TestProviderNumberTable(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string // decimal string form, "" means expect an error
	}{
		{"1.234,56", "1234.56"},
		{"1,234.56", "1234.56"},
		{"1234,5", "1234.5"},
		{"1.234.567", "1234567"},
		{"12.5", "12.5"},
		{"1 234,5", "1234.5"},
		{"-3,25", "-3.25"},
		{"123456789012.3456", "123456789012.3456"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := normalize.ProviderNumber(tc.raw)
			require.NoError(t, err)
			want, werr := decimal.NewFromString(tc.want)
			require.NoError(t, werr)
			require.True(t, want.Equal(got), "%s: want %s got %s", tc.raw, want, got)
		})
	}
}

func TestProviderNumberInvalid(t *testing.T) {
	_, err := normalize.ProviderNumber("abc")
	require.Error(t, err)
}

func TestProviderNumberEmptyIsErrEmpty(t *testing.T) {
	_, err := normalize.ProviderNumber("")
	require.ErrorIs(t, err, normalize.ErrEmpty)
}

// TestProviderNumberNeverGoesThroughFloat proves that a value with more
// significant digits than float64 can represent exactly survives
// ProviderNumber unchanged. If ProviderNumber were implemented by parsing
// through strconv.ParseFloat (or json.Unmarshal into float64/any), this
// value would be corrupted by float64's ~15-17 significant digit limit.
//
// Mutation proof (see task-3-report.md): replacing ProviderNumber's final
// `return decimal.NewFromString(normalised)` with
// `f, _ := strconv.ParseFloat(normalised, 64); return
// decimal.NewFromFloat(f), nil` makes this test FAIL — observed output:
// "9007199254740993.0001" round-trips through float64 to "9007199254740994"
// (strconv.ParseFloat(...,64) then decimal.NewFromFloat), not the original
// digits. (123456789012.3456, used in the table test above, happens to
// survive a float64 round trip, which is exactly why this test uses a
// larger value that does not.)
func TestProviderNumberNeverGoesThroughFloat(t *testing.T) {
	got, err := normalize.ProviderNumber("9007199254740993.0001")
	require.NoError(t, err)
	require.Equal(t, "9007199254740993.0001", got.String())
}

func TestOptionalNumber(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		wantNil bool
		want    string
	}{
		{"", true, ""},
		{"null", true, ""},
		{"-", true, ""},
		{"12.5", false, "12.5"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := normalize.OptionalNumber(tc.raw)
			require.NoError(t, err)
			if tc.wantNil {
				require.Nil(t, got, "unavailable registers must be nil, never zero")
				return
			}
			require.NotNil(t, got)
			require.Equal(t, tc.want, got.String())
		})
	}
}

// TestJSONNumberIsExact proves JSONNumber converts a json.Number decoded
// with UseNumber() without going through float64: a value with more
// significant digits than float64 can hold survives with its string form
// preserved.
func TestJSONNumberIsExact(t *testing.T) {
	var doc struct {
		V json.Number `json:"v"`
	}

	raw := []byte(`{"v": 9007199254740993.0001}`)
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&doc))
	require.Equal(t, json.Number("9007199254740993.0001"), doc.V)

	got, err := normalize.JSONNumber(doc.V)
	require.NoError(t, err)
	require.Equal(t, "9007199254740993.0001", got.String())
}

func TestOptionalJSONNumber(t *testing.T) {
	got, err := normalize.OptionalJSONNumber(nil)
	require.NoError(t, err)
	require.Nil(t, got, "unavailable registers must be nil, never zero")

	n := json.Number("42.5")
	got, err = normalize.OptionalJSONNumber(&n)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "42.5", got.String())
}
