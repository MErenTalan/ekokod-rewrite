package carbon

import (
	"time"

	"github.com/shopspring/decimal"
)

// Emission is 02 §9.1: quantity × unit multiplier × base factor, kg CO₂e at
// the column's 6 dp.
func Emission(quantity, multiplier, factor decimal.Decimal) decimal.Decimal {
	return quantity.Mul(multiplier).Mul(factor).Round(6)
}

// Dated is a value spread evenly over the inclusive civil dates Start..End.
type Dated struct {
	Start, End time.Time
	Value      decimal.Decimal
}

func civil(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func days(from, to time.Time) int64 {
	return int64(civil(to).Sub(civil(from)).Hours()/24) + 1
}

// Share is R308: the part of d that falls in the inclusive window [from, to],
// by days. It is not rounded, so aggregates add exact shares.
func Share(d Dated, from, to time.Time) decimal.Decimal {
	start, end := civil(d.Start), civil(d.End)
	if f := civil(from); f.After(start) {
		start = f
	}
	if t := civil(to); t.Before(end) {
		end = t
	}
	if end.Before(start) {
		return decimal.Zero
	}
	span := days(d.Start, d.End)
	if span <= 0 {
		return decimal.Zero
	}
	return d.Value.Mul(decimal.NewFromInt(days(start, end))).Div(decimal.NewFromInt(span))
}
