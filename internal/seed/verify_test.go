package seed

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestCompareFactors(t *testing.T) {
	shipped := []EmissionFactor{
		{Key: "a", BaseFactor: decimal.RequireFromString("1.5")},
		{Key: "b", BaseFactor: decimal.RequireFromString("2")},
		{Key: "c", BaseFactor: decimal.RequireFromString("3")},
	}
	ok := CompareFactors(shipped, map[string]decimal.Decimal{
		"a": decimal.RequireFromString("1.50000000"), "b": decimal.RequireFromString("2"), "c": decimal.RequireFromString("3"),
	})
	require.True(t, ok.OK())
	require.Equal(t, "emission factors: 3/3 match", ok.String())

	bad := CompareFactors(shipped, map[string]decimal.Decimal{
		"a": decimal.RequireFromString("1.6"), "c": decimal.RequireFromString("3"), "z": decimal.RequireFromString("9"),
	})
	require.False(t, bad.OK())
	require.Equal(t, []string{"b"}, bad.Missing)
	require.Equal(t, []string{"z"}, bad.Extra)
	require.Equal(t, []string{"a"}, bad.Changed)
	require.Equal(t, "emission factors: 1/3 match; missing [b]; extra [z]; changed [a]", bad.String())
}

// The shipped catalogue is the legacy master list (182) plus R293's four equivalences.
func TestShippedCatalogueCount(t *testing.T) {
	rows, err := EmissionFactors()
	require.NoError(t, err)
	require.Len(t, rows, 186)
}
