// Package generation reconciles PM5340's interval energy
// (model.MeterReading.IntervalGenerationKwh) into meter_readings'
// active_export register — a running, anchored cumulative total — so that
// active_export reads the same for PM5340 as it would for a provider that
// reports the cumulative export register directly (06 §5, plan ruling R15).
package generation

import (
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Accumulate is the pure core: a running sum starting at base, over rows in
// Ts-ascending order, one output value per input row.
//
// A row whose IntervalGenerationKwh is nil adds nothing to the running sum —
// it is not "no data" for the interval, it is "this reading contributes zero
// change", per R15 and 06 §5's "unavailable registers are nil, never zero"
// rule applied to the running total rather than the register itself: the
// row's own active_export still carries the running value FORWARD (it is
// never itself nil), because active_export is a cumulative register and a
// gap in interval reporting does not un-happen the energy already
// generated before it.
//
// rows must already be sorted by Ts ascending; Accumulate does not sort or
// validate — that is the caller's job (the store's Range/BulkInsert
// contracts already guarantee ascending order and unique keys).
func Accumulate(base decimal.Decimal, rows []model.MeterReading) []decimal.Decimal {
	out := make([]decimal.Decimal, len(rows))
	cur := base
	for i, row := range rows {
		if row.IntervalGenerationKwh != nil {
			cur = cur.Add(*row.IntervalGenerationKwh)
		}
		out[i] = cur
	}
	return out
}
