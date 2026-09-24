package billing

import (
	"errors"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// ErrInvalidInput is returned for input Compute cannot price at all (R113).
var ErrInvalidInput = errors.New("billing: invalid input")

// Flag is a persisted data problem that keeps a bill from being issued (R113).
type Flag string

// The flags.
const (
	FlagPTFDataMissing        Flag = "ptf_data_missing"
	FlagReactiveDataMissing   Flag = "reactive_data_missing"
	FlagInstalledPowerMissing Flag = "installed_power_missing"
	FlagGenerationDataMissing Flag = "generation_data_missing"
	FlagTieringPriceMissing   Flag = "tiering_price_missing"
)

func sortedFlags(set map[Flag]bool) []Flag {
	out := make([]Flag, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}

var hundred = decimal.NewFromInt(100)

// Round2 rounds a money amount per R112: a negative amount is rounded on its
// absolute value and negated, under either mode.
func Round2(x decimal.Decimal, mode model.MoneyRoundingMode) decimal.Decimal {
	if x.IsNegative() {
		return Round2(x.Neg(), mode).Neg()
	}
	if mode == model.MoneyRoundingHalfEven {
		return x.RoundBank(2)
	}
	return x.Round(2)
}
