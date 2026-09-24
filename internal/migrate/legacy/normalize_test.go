package legacy_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
)

func TestIDsAreDeterministicPerCollection(t *testing.T) {
	a := legacy.ID("analyzers", "64f0c0ffee0000000000abcd")
	require.Equal(t, a, legacy.ID("analyzers", "64f0c0ffee0000000000abcd"))
	require.NotEqual(t, a, legacy.ID("buildings", "64f0c0ffee0000000000abcd"))
	require.EqualValues(t, 5, a.Version())
}

func TestDatesAreParsedOrRejectedNeverGuessed(t *testing.T) {
	ist := time.FixedZone("TRT", 3*3600)
	ok := map[string]time.Time{
		"2026-09-24T10:00:00Z":      time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC),
		"2026-09-24T13:00:00+03:00": time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC),
		"2026-09-24":                time.Date(2026, 9, 24, 0, 0, 0, 0, ist).UTC(),
		"24-09-2026":                time.Date(2026, 9, 24, 0, 0, 0, 0, ist).UTC(),
		"24.09.2026":                time.Date(2026, 9, 24, 0, 0, 0, 0, ist).UTC(),
		"24/09/2026":                time.Date(2026, 9, 24, 0, 0, 0, 0, ist).UTC(),
		"24-Eyl-2026":               time.Date(2026, 9, 24, 0, 0, 0, 0, ist).UTC(),
		"24-Sep-2026":               time.Date(2026, 9, 24, 0, 0, 0, 0, ist).UTC(),
		"24-Ağu-2026":               time.Date(2026, 8, 24, 0, 0, 0, 0, ist).UTC(),
		"2026-09-24 13:15":          time.Date(2026, 9, 24, 10, 15, 0, 0, time.UTC),
		"2026-09-24 13:15:30":       time.Date(2026, 9, 24, 10, 15, 30, 0, time.UTC),
		"2026-09":                   time.Date(2026, 9, 1, 0, 0, 0, 0, ist).UTC(),
	}
	for in, want := range ok {
		got, err := legacy.ParseDate(in)
		require.NoError(t, err, in)
		require.True(t, want.Equal(got), "%s: got %s want %s", in, got, want)
	}
	for _, in := range []string{"03/04/24", "24-09-26", "2026-13-01", "31-02-2026", "yesterday", "", "09/24/2026", "24-Foo-2026"} {
		_, err := legacy.ParseDate(in)
		require.Error(t, err, in)
		require.True(t, errors.Is(err, legacy.ErrBadDate), in)
	}
	_, err := legacy.ParseDate("03-04-24")
	require.ErrorIs(t, err, legacy.ErrAmbiguousDate, "a two-digit year is ambiguous")
}

func TestNumbersParseOnlyWhenUnambiguous(t *testing.T) {
	ok := map[string]string{"1234.56": "1234.56", "1.234,56": "1234.56", "1,234.56": "1234.56", "12,5": "12.5", "-3": "-3", "1.234.567,8": "1234567.8"}
	for in, want := range ok {
		got, err := legacy.ParseNumber(in)
		require.NoError(t, err, in)
		require.True(t, decimal.RequireFromString(want).Equal(*got), "%s → %s", in, got)
	}
	padded, err := legacy.ParseNumber(" 42 ")
	require.NoError(t, err)
	require.True(t, decimal.NewFromInt(42).Equal(*padded), "surrounding spaces are trimmed")
	got, err := legacy.ParseNumber("")
	require.NoError(t, err)
	require.Nil(t, got, "empty is null")
	for _, in := range []string{"1.234", "1,234", "abc", "1,2,3.4.5"} {
		_, err := legacy.ParseNumber(in)
		require.Error(t, err, in)
	}
}

func TestEnumsMapTurkishValues(t *testing.T) {
	for in, want := range map[string]string{"Mesken": "residential", "TİCARETHANE": "commercial", "sanayi": "industrial", "Tarımsal": "agricultural", "tarim": "agricultural", "aydınlatma": "lighting"} {
		got, err := legacy.SubscriberGroup(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	_, err := legacy.SubscriberGroup("uzay")
	require.Error(t, err)
	v, err := legacy.Voltage("OG")
	require.NoError(t, err)
	require.Equal(t, "mv", v)
	term, err := legacy.Term("Çift Terim")
	require.NoError(t, err)
	require.Equal(t, "binomial", term)
	multi, err := legacy.MultiTime("Çok Zamanlı")
	require.NoError(t, err)
	require.True(t, multi)
}

func TestRejectionsAreCountedAndTheInvariantHolds(t *testing.T) {
	var buf bytes.Buffer
	r := legacy.NewRejections(&buf)
	r.Accept("users")
	r.Accept("users")
	r.Reject("users", "64f0", "role", "unknown_role", "superuser")
	require.NoError(t, r.Check("users", 3))
	require.ErrorContains(t, r.Check("users", 4), "4 read, 2 accepted + 1 rejected")
	require.Contains(t, buf.String(), `"reason":"unknown_role"`)
	require.Equal(t, 1, strings.Count(buf.String(), "\n"))
	require.Equal(t, map[string]legacy.Tally{"users": {Accepted: 2, Rejected: 1}}, r.Summary())
}
