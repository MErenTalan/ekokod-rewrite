package consumption

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// HourlyOverlap sums min(import, export) over sound hourly rows: the energy
// hourly netting removes from both sides (R531). Nil when no hour carries both.
func HourlyOverlap(rows []Row) *decimal.Decimal {
	var sum *decimal.Decimal
	for _, r := range rows {
		imp, exp := r.Values[energy.ActiveImport], r.Values[energy.ActiveExport]
		absorbed := !r.SpanFrom.IsZero() && r.SpanTo.Sub(r.SpanFrom) > time.Hour // §3.1 merged missing hours
		if imp == nil || exp == nil || absorbed || r.Partial || len(r.Suspect) > 0 {
			continue
		}
		v := decimal.Max(decimal.Zero, decimal.Min(*imp, *exp))
		if sum != nil {
			v = v.Add(*sum)
		}
		sum = &v
	}
	return sum
}
