package render_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/render"
)

// TestMoneyFormatNeverUsesFloat: an 18-significant-digit amount formats
// exactly, which a float64 path cannot (I-16b).
func TestMoneyFormatNeverUsesFloat(t *testing.T) {
	v := decimal.RequireFromString("1234567890123456.78")
	require.Equal(t, "1.234.567.890.123.456,78", render.FormatMoney(v, "tr"))
	require.Equal(t, "1,234,567,890,123,456.78", render.FormatMoney(v, "en"))
	require.Equal(t, "-1.234,50", render.FormatMoney(decimal.RequireFromString("-1234.5"), "tr"))
	require.Equal(t, "0,00", render.FormatMoney(decimal.Zero, "tr"))
	require.Equal(t, "999,999", render.FormatDecimal(decimal.RequireFromString("999.9994"), 3, "tr"))
	require.Equal(t, "123", render.FormatDecimal(decimal.RequireFromString("123.4"), 0, "tr"))
}
