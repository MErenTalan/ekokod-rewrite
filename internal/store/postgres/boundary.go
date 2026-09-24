package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// registerCount is the twelve delta registers, in consumptionDeltas' order.
const registerCount = 12

// boundaryBucket is one consumption aggregate row plus the first/last value
// of every register and the first reading's instant (migration 00021).
type boundaryBucket struct {
	b       model.ConsumptionBucket
	firstTS time.Time
	start   [registerCount]*decimal.Decimal
	end     [registerCount]*decimal.Decimal
}

// consumptionDeltas addresses b's twelve delta fields in register order:
// active, inductive, capacitive, t1–t3 import, then the same for export.
func consumptionDeltas(b *model.ConsumptionBucket) [registerCount]**decimal.Decimal {
	return [registerCount]**decimal.Decimal{
		&b.ActiveConsumption, &b.InductiveConsumption, &b.CapacitiveConsumption,
		&b.T1Consumption, &b.T2Consumption, &b.T3Consumption,
		&b.ActiveGeneration, &b.InductiveGeneration, &b.CapacitiveGeneration,
		&b.T1Generation, &b.T2Generation, &b.T3Generation,
	}
}

// widenForBoundaries extends tr by the bucket before its first bucket and the
// bucket after its last, so every requested bucket has both neighbours.
func widenForBoundaries(level energy.Level, tr store.TimeRange) store.TimeRange {
	from := energy.Bucket(level, tr.From.Add(-time.Nanosecond), istanbulDays).From
	last := energy.Bucket(level, tr.To.Add(-time.Nanosecond), istanbulDays)
	return store.TimeRange{From: from, To: energy.Bucket(level, last.To, istanbulDays).To}
}

// boundaryDifference rewrites every delta as boundary(b1) − boundary(b0)
// (02 §3.1, F15q R471) and keeps the buckets inside tr. rows are ordered by
// analyzer, bucket. A bucket's start is its own first reading when that sits
// on the edge, else the adjacent previous bucket's last; its end is the
// adjacent next bucket's first reading when that sits on the edge, else its
// own last. Across a gap it falls back to its own values, never absorbing it.
func boundaryDifference(level energy.Level, rows []boundaryBucket, tr store.TimeRange) []boundaryBucket {
	out := make([]boundaryBucket, 0, len(rows))
	for i := range rows {
		cur := rows[i]
		if cur.b.Bucket.Before(tr.From) || !cur.b.Bucket.Before(tr.To) {
			continue
		}
		w := energy.Bucket(level, cur.b.Bucket, istanbulDays)
		var prev, next *boundaryBucket
		if i > 0 && rows[i-1].b.AnalyzerID == cur.b.AnalyzerID &&
			energy.Bucket(level, rows[i-1].b.Bucket, istanbulDays).To.Equal(w.From) {
			prev = &rows[i-1]
		}
		if i+1 < len(rows) && rows[i+1].b.AnalyzerID == cur.b.AnalyzerID &&
			rows[i+1].b.Bucket.Equal(w.To) && rows[i+1].firstTS.Equal(w.To) {
			next = &rows[i+1]
		}
		onEdge := cur.firstTS.Equal(w.From)
		deltas := consumptionDeltas(&cur.b)
		for r := 0; r < registerCount; r++ {
			start := cur.start[r]
			if !onEdge && prev != nil && prev.end[r] != nil {
				start = prev.end[r]
			}
			end := cur.end[r]
			if next != nil && next.start[r] != nil {
				end = next.start[r]
			}
			if start == nil || end == nil {
				*deltas[r] = nil
				continue
			}
			d := end.Sub(*start)
			*deltas[r] = &d
		}
		out = append(out, cur)
	}
	return out
}

// boundaryBucketFrom converts one aggregate row; the four views share one
// column list, so their generated structs convert to ConsumptionHourly.
func boundaryBucketFrom(row sqlcgen.ConsumptionHourly) (boundaryBucket, error) {
	b, err := consumptionBucketFromHourly(row)
	if err != nil {
		return boundaryBucket{}, err
	}
	out := boundaryBucket{b: b, firstTS: row.FirstTs.Time}
	starts := []*pgtype.Numeric{
		&row.ActiveImportStart, &row.InductiveConsumptionStart, &row.CapacitiveConsumptionStart,
		&row.T1ConsumptionStart, &row.T2ConsumptionStart, &row.T3ConsumptionStart,
		&row.ActiveGenerationStart, &row.InductiveGenerationStart, &row.CapacitiveGenerationStart,
		&row.T1GenerationStart, &row.T2GenerationStart, &row.T3GenerationStart,
	}
	ends := []*pgtype.Numeric{
		&row.ActiveIndex, &row.InductiveIndex, &row.CapacitiveIndex,
		&row.T1Index, &row.T2Index, &row.T3Index,
		&row.ActiveGenerationIndex, &row.InductiveGenerationIndex, &row.CapacitiveGenerationIndex,
		&row.T1GenerationIndex, &row.T2GenerationIndex, &row.T3GenerationIndex,
	}
	for r := 0; r < registerCount; r++ {
		if out.start[r], err = numericToDecimalPtr(*starts[r]); err != nil {
			return boundaryBucket{}, err
		}
		if out.end[r], err = numericToDecimalPtr(*ends[r]); err != nil {
			return boundaryBucket{}, err
		}
	}
	return out, nil
}
