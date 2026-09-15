package normalize_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

func TestMultiplyPreservesNil(t *testing.T) {
	m := decimal.RequireFromString("1.5")

	got := normalize.Multiply(nil, m)
	require.Nil(t, got, "unavailable registers must stay nil, never become zero")

	v := decimal.RequireFromString("10")
	got = normalize.Multiply(&v, m)
	require.NotNil(t, got)
	require.Equal(t, "15", got.String())
}

// TestUnitConversionsAreExact proves WhToKWh and KWFor15MinToKWh produce
// exact decimal results with no rounding error, and that nil (unavailable)
// values are preserved as nil.
func TestUnitConversionsAreExact(t *testing.T) {
	wh := decimal.RequireFromString("1500")
	kwh := normalize.WhToKWh(&wh)
	require.NotNil(t, kwh)
	require.Equal(t, "1.5", kwh.String())
	require.Nil(t, normalize.WhToKWh(nil))

	w := decimal.RequireFromString("2400")
	kw := normalize.WToKW(&w)
	require.NotNil(t, kw)
	require.Equal(t, "2.4", kw.String())
	require.Nil(t, normalize.WToKW(nil))

	kwFor15 := decimal.RequireFromString("2.4")
	kwhFrom15 := normalize.KWFor15MinToKWh(&kwFor15)
	require.NotNil(t, kwhFrom15)
	require.Equal(t, "0.6", kwhFrom15.String())
	require.Nil(t, normalize.KWFor15MinToKWh(nil))
}
