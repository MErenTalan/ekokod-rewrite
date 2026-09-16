package render

import (
	"strings"

	"github.com/shopspring/decimal"
)

// FormatDecimal formats d with places decimals and locale grouping, from the
// decimal's string and never through float: tr "1.234,56", en "1,234.56".
func FormatDecimal(d decimal.Decimal, places int32, locale string) string {
	group, point := ".", ","
	if locale == "en" {
		group, point = ",", "."
	}
	s := d.StringFixed(places)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteString(group)
		}
		b.WriteRune(r)
	}
	out := b.String()
	if places > 0 {
		out += point + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

// FormatMoney formats an amount with 2 decimals.
func FormatMoney(d decimal.Decimal, locale string) string { return FormatDecimal(d, 2, locale) }
