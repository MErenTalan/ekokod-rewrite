package tariff

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// ErrInvalidParams wraps every ValidateParams failure.
var ErrInvalidParams = errors.New("billing parameters invalid")

// ValidateParams checks a resolved parameter row (M-6): bands contiguous from
// the exemption (or 0) upward with an unbounded last band, positive limits,
// tolerance in [0,1], multiplier ≥ 0, positive tiering thresholds, valid enums.
func ValidateParams(p model.BillingParameters) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidParams, fmt.Sprintf(format, args...))
	}
	if !p.ReactivePenaltyBasis.Valid() || !p.TieringMode.Valid() || !p.MoneyRoundingMode.Valid() {
		return bad("enum value")
	}
	for _, v := range p.ReactiveExemptTerms {
		if !v.Valid() {
			return bad("reactive_exempt_terms %q", v)
		}
	}
	for _, v := range p.ReactiveExemptUserGroups {
		if !v.Valid() {
			return bad("reactive_exempt_user_groups %q", v)
		}
	}
	for _, v := range p.TieringVoltageLevels {
		if !v.Valid() {
			return bad("tiering_voltage_levels %q", v)
		}
	}
	for _, v := range p.TieringSupplyCompanies {
		if !v.Valid() {
			return bad("tiering_supply_companies %q", v)
		}
	}
	if len(p.ReactiveBands) == 0 {
		return bad("no reactive bands")
	}
	next := decimal.Zero
	if p.ReactiveExemptBelowKw != nil {
		next = *p.ReactiveExemptBelowKw
	}
	for i, b := range p.ReactiveBands {
		if !b.MinKw.Equal(next) {
			return bad("band %d starts at %s, want %s", i, b.MinKw, next)
		}
		if !b.Inductive.IsPositive() || !b.Capacitive.IsPositive() {
			return bad("band %d limits must be > 0", i)
		}
		last := i == len(p.ReactiveBands)-1
		if last != (b.MaxKw == nil) {
			return bad("only the last band is unbounded")
		}
		if !last {
			if b.MaxKw.LessThanOrEqual(b.MinKw) {
				return bad("band %d is empty", i)
			}
			next = *b.MaxKw
		}
	}
	if p.PTFMissingHourTolerance.IsNegative() || p.PTFMissingHourTolerance.GreaterThan(decimal.NewFromInt(1)) {
		return bad("ptf_missing_hour_tolerance out of [0,1]")
	}
	if p.DemandOverrunMultiplier.IsNegative() {
		return bad("demand_overrun_multiplier < 0")
	}
	if p.ReactiveGenerationExemptKwh.IsNegative() {
		return bad("reactive_generation_exempt_kwh < 0")
	}
	for g, v := range p.TieringGroups {
		if !g.Valid() || !v.IsPositive() {
			return bad("tiering group %q threshold %s", g, v)
		}
	}
	return nil
}
