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

// AccumulateBackward is Accumulate run in reverse: it derives active_export
// for rows STRICTLY BEFORE a known reference point (R52 — a generation
// anchor whose Source is 'operator' or 'migration', a real meter-set
// cumulative value the anchor must not be moved off of), by summing forward
// register deltas backward from that point instead of forward from a base.
//
// The formula (R52): active_export(t) = refVal − Σ intervals of every row
// with Ts in (t, refTs] — every row STRICTLY AFTER t, up to and including
// the reference timestamp itself. rows must already be Ts-ascending and
// every row's Ts must be <= refTs (the caller's job, exactly like
// Accumulate's own ordering contract) — see hook.go's deriveOlderRows,
// which only ever calls this with rows read from a Range query bounded at
// the anchor timestamp.
//
// sumAfter seeds the running sum of every already-processed row's interval
// (0 for the slice closest to refTs); a caller chunking a long backward
// span across several bounded Range reads (deriveOlderRows, the same
// rangeSliceWindow-bounded pattern recomputeForward uses going forward)
// carries the returned sumAfter into the next, farther-back chunk — nearest
// chunk first, exactly mirroring Accumulate/recomputeForward's cur.
//
// Proof this inverts Accumulate exactly: seed refVal with Accumulate's own
// last output value for a set of rows, and sumAfter with decimal.Zero; the
// two functions' per-row outputs are then byte-identical (see
// TestAccumulateBackwardInvertsAccumulate) — telescoping cumulative(i) =
// total − Σ(rows after i) is the same algebra either direction is walked.
func AccumulateBackward(refVal, sumAfter decimal.Decimal, rows []model.MeterReading) (out []decimal.Decimal, nextSumAfter decimal.Decimal) {
	out = make([]decimal.Decimal, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		out[i] = refVal.Sub(sumAfter)
		if rows[i].IntervalGenerationKwh != nil {
			sumAfter = sumAfter.Add(*rows[i].IntervalGenerationKwh)
		}
	}
	return out, sumAfter
}
