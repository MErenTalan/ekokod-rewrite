package normalize

import "github.com/shopspring/decimal"

// quarterHour is the fraction of an hour a 15-minute interval represents,
// used by KWFor15MinToKWh (R15). Built from a string literal, never a
// float, so the multiplication stays exact.
var quarterHour = decimal.RequireFromString("0.25")

// Multiply returns v × m, preserving nil (removed-behaviour 21: an
// unavailable register stays nil, it is never multiplied into a zero).
// This is the one multiplication point per adapter (02 §2.2): the meter
// multiplier is applied exactly once, at ingestion.
func Multiply(v *decimal.Decimal, m decimal.Decimal) *decimal.Decimal {
	if v == nil {
		return nil
	}
	r := v.Mul(m)
	return &r
}

// WhToKWh converts watt-hours to kilowatt-hours (÷ 1000), exactly: Shift
// moves the decimal point rather than performing a rounded division.
func WhToKWh(v *decimal.Decimal) *decimal.Decimal {
	return shift(v, -3)
}

// WToKW converts watts to kilowatts (÷ 1000), exactly.
func WToKW(v *decimal.Decimal) *decimal.Decimal {
	return shift(v, -3)
}

// KWFor15MinToKWh converts a 15-minute average/instantaneous kW reading to
// the kWh energy for that interval: × 0.25 (R15).
func KWFor15MinToKWh(v *decimal.Decimal) *decimal.Decimal {
	if v == nil {
		return nil
	}
	r := v.Mul(quarterHour)
	return &r
}

func shift(v *decimal.Decimal, exp int32) *decimal.Decimal {
	if v == nil {
		return nil
	}
	r := v.Shift(exp)
	return &r
}
