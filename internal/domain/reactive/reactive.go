// Package reactive evaluates the reactive-energy penalty (02 §6.6) under the
// dated billing parameters (R106, R117, R118, I-18). It is pure.
package reactive

import (
	"slices"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

const divisionScale int32 = 20

// Exemption reasons.
const (
	ExemptTerm       = "term"
	ExemptUserGroup  = "user_group"
	ExemptGeneration = "generation"
	ExemptBelowKw    = "below_kw"
)

// Missing data markers.
const (
	MissingInstalledPower = "installed_power"
	MissingInductive      = "reactive_inductive"
	MissingCapacitive     = "reactive_capacitive"
	MissingActiveExport   = "active_export"
)

// Input is one invoice's reactive evaluation input.
type Input struct {
	NetConsumption        decimal.Decimal
	Inductive, Capacitive *decimal.Decimal
	ActiveExport          *decimal.Decimal // nil-propagating aggregate (R116)
	ActiveExportKnownSum  decimal.Decimal  // I-18: sum over members that reported export
	// ActiveExportPartial: some members reported export and some did not, so the
	// generation exemption cannot be decided (I-18).
	ActiveExportPartial bool
	InstalledPowerKw    *decimal.Decimal
	Term                model.TariffTerm
	UserGroup           model.DistributionUserGroup
	UnitPrice           *decimal.Decimal // nil → no penalty even when exceeded (R119)
	Params              model.BillingParameters
}

// Result is the evaluated penalty, unrounded.
type Result struct {
	Exempt                                        bool
	ExemptReason                                  string
	InductiveRatio, CapacitiveRatio               *decimal.Decimal // nil when net is 0 (R117)
	InductiveLimit, CapacitiveLimit               *decimal.Decimal
	InductiveKvarhCharged, CapacitiveKvarhCharged *decimal.Decimal // nil = side not penalised
	InductivePenalty, CapacitivePenalty           decimal.Decimal
	Applied                                       bool
	Missing                                       []string
}

// Evaluate applies exemptions, then the band limits, per side.
func Evaluate(in Input) Result {
	var r Result
	r.InductiveRatio = ratio(in.Inductive, in.NetConsumption)
	r.CapacitiveRatio = ratio(in.Capacitive, in.NetConsumption)

	p := in.Params
	switch {
	case slices.Contains(p.ReactiveExemptTerms, in.Term):
		r.Exempt, r.ExemptReason = true, ExemptTerm
	case slices.Contains(p.ReactiveExemptUserGroups, in.UserGroup):
		r.Exempt, r.ExemptReason = true, ExemptUserGroup
	case in.ActiveExportKnownSum.GreaterThan(p.ReactiveGenerationExemptKwh): // I-18: known lower bound
		r.Exempt, r.ExemptReason = true, ExemptGeneration
	case in.InstalledPowerKw != nil && p.ReactiveExemptBelowKw != nil && in.InstalledPowerKw.LessThan(*p.ReactiveExemptBelowKw):
		r.Exempt, r.ExemptReason = true, ExemptBelowKw
	}
	if r.Exempt {
		return r
	}
	if in.ActiveExport == nil && in.ActiveExportPartial {
		r.Missing = append(r.Missing, MissingActiveExport)
	}
	if in.InstalledPowerKw == nil {
		r.Missing = append(r.Missing, MissingInstalledPower)
		return r
	}
	band, ok := findBand(p.ReactiveBands, *in.InstalledPowerKw)
	if !ok {
		r.Missing = append(r.Missing, MissingInstalledPower)
		return r
	}
	r.InductiveLimit, r.CapacitiveLimit = &band.Inductive, &band.Capacitive

	side := func(reg *decimal.Decimal, limit decimal.Decimal, missing string) (*decimal.Decimal, decimal.Decimal) {
		if reg == nil {
			r.Missing = append(r.Missing, missing)
			return nil, decimal.Zero
		}
		net := in.NetConsumption
		exceeded := (net.IsPositive() && reg.GreaterThan(limit.Mul(net))) || (net.IsZero() && reg.IsPositive()) // R117 ε form
		if !exceeded {
			return nil, decimal.Zero
		}
		charged := *reg
		if p.ReactivePenaltyBasis == model.ReactivePenaltyBasisExcessOverLimit {
			charged = reg.Sub(limit.Mul(net))
		}
		r.Applied = true
		if in.UnitPrice == nil {
			return &charged, decimal.Zero
		}
		return &charged, charged.Mul(*in.UnitPrice)
	}
	r.InductiveKvarhCharged, r.InductivePenalty = side(in.Inductive, band.Inductive, MissingInductive)
	r.CapacitiveKvarhCharged, r.CapacitivePenalty = side(in.Capacitive, band.Capacitive, MissingCapacitive)
	return r
}

func ratio(reg *decimal.Decimal, net decimal.Decimal) *decimal.Decimal {
	if reg == nil || !net.IsPositive() {
		return nil
	}
	v := reg.DivRound(net, divisionScale)
	return &v
}

func findBand(bands []model.ReactiveBand, kw decimal.Decimal) (model.ReactiveBand, bool) {
	for _, b := range bands {
		if kw.GreaterThanOrEqual(b.MinKw) && (b.MaxKw == nil || kw.LessThan(*b.MaxKw)) {
			return b, true
		}
	}
	return model.ReactiveBand{}, false
}
