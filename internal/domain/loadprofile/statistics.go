package loadprofile

import "github.com/shopspring/decimal"

// oneHalf is exactly 1/2, expressed as a Decimal literal (mantissa 5,
// exponent -1) rather than decimal.NewFromFloat(0.5): no float64 appears
// anywhere in internal/domain, not even as a literal argument.
var oneHalf = decimal.New(5, -1)

// Statistics are computed over the 24 hourly means held in a Profile — not
// over each hour's spread across the days that fed it (R84). Every field is
// nil when the profile has no hours (no day contributed any value to it).
type Statistics struct {
	Max        *decimal.Decimal
	Min        *decimal.Decimal
	HourOfMax  *int
	Mean       *decimal.Decimal
	StdDev     *decimal.Decimal // population standard deviation, divisor n (R67)
	Range      *decimal.Decimal // Max - Min
	LoadFactor *decimal.Decimal // Mean / Max; decimal.Zero when Max is zero (R81)
}

// Stats computes §10.3's per-profile statistics from p's 24 hourly means.
func Stats(p Profile) Statistics {
	var values []decimal.Decimal
	var hours []int
	for h, v := range p.Hours {
		if v == nil {
			continue
		}
		values = append(values, *v)
		hours = append(hours, h)
	}
	if len(values) == 0 {
		return Statistics{}
	}

	max := values[0]
	min := values[0]
	hourOfMax := hours[0]
	sum := decimal.Zero
	for i, v := range values {
		sum = sum.Add(v)
		if v.GreaterThan(max) {
			// Strictly greater, so the FIRST (lowest-hour) occurrence of the
			// maximum wins a tie: HourOfMax is deterministic under ties.
			max = v
			hourOfMax = hours[i]
		}
		if v.LessThan(min) {
			min = v
		}
	}

	n := decimal.NewFromInt(int64(len(values)))
	// Precision, not presentation (R79): DivRound's scale bounds how many
	// fractional digits this mean keeps internally; it is not display
	// rounding.
	mean := sum.DivRound(n, DivisionScale)

	// R67: population variance — divisor n over the 24 hourly means
	// themselves (or however many are non-nil), not n-1. These means are
	// the whole curve §10.3 describes, not a sample drawn from a larger
	// population.
	varianceSum := decimal.Zero
	for _, v := range values {
		diff := v.Sub(mean)
		varianceSum = varianceSum.Add(diff.Mul(diff))
	}
	variance := varianceSum.DivRound(n, DivisionScale)

	stdDev, err := variance.PowWithPrecision(oneHalf, DivisionScale)
	var stdDevPtr *decimal.Decimal
	if err != nil {
		// Unreachable today (M-8, final review A): variance is a sum of
		// squares divided by a positive count, so it is never negative, and
		// PowWithPrecision's documented failures (0**0, a negative base with
		// a non-integer exponent) cannot occur here. The branch is kept —
		// for a future decimal upgrade that DID make it reachable — but it
		// must not turn into a silently wrong 0: StdDev is a pointer
		// specifically so "unavailable" (nil) is representable, so an
		// unreachable-today error returns nil, never a fabricated zero.
		stdDevPtr = nil
	} else {
		stdDevPtr = &stdDev
	}

	rng := max.Sub(min)

	var loadFactor decimal.Decimal
	if max.IsZero() {
		// R81: §10.3 says load factor is 0 when max = 0 — deliberately
		// UNLIKE the R54 consumption ratios elsewhere in F3, which are nil
		// on an undefined denominator instead of zero.
		loadFactor = decimal.Zero
	} else {
		loadFactor = mean.DivRound(max, DivisionScale)
	}

	return Statistics{
		Max:        &max,
		Min:        &min,
		HourOfMax:  &hourOfMax,
		Mean:       &mean,
		StdDev:     stdDevPtr,
		Range:      &rng,
		LoadFactor: &loadFactor,
	}
}
