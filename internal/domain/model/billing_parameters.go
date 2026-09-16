package model

import (
	"time"

	"github.com/shopspring/decimal"
)

// ReactiveBand is one installed-power band of the reactive limits (02 §6.6).
type ReactiveBand struct {
	MinKw      decimal.Decimal
	MaxKw      *decimal.Decimal // nil = unbounded
	Inductive  decimal.Decimal
	Capacitive decimal.Decimal
}

// BillingParameters is one dated, platform-wide set of the regulatory rules the
// product owner has not confirmed (R106). Mirrors table `billing_parameters`
// (migration 00014); it resolves by EffectiveFrom like a tariff.
type BillingParameters struct {
	EffectiveFrom               time.Time
	ReactivePenaltyBasis        ReactivePenaltyBasis
	ReactiveExemptBelowKw       *decimal.Decimal // nil = no size exemption
	ReactiveExemptTerms         []TariffTerm
	ReactiveExemptUserGroups    []DistributionUserGroup
	ReactiveGenerationExemptKwh decimal.Decimal
	ReactiveBands               []ReactiveBand
	TieringGroups               map[DistributionUserGroup]decimal.Decimal // group → kWh/day threshold
	TieringMode                 TieringMode
	TieringVoltageLevels        []VoltageLevel
	TieringSupplyCompanies      []SupplyCompany // C-3: gates tiering off free-market contracts
	PTFMissingHourTolerance     decimal.Decimal // fraction, 0..1
	DemandOverrunMultiplier     decimal.Decimal
	MoneyRoundingMode           MoneyRoundingMode
	CreatedAt                   time.Time
}
