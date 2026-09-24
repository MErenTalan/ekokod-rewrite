package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Bill is one computed invoice for one scope and period. Mirrors table `bills`
// (migration 00009).
//
// Recomputation SUPERSEDES rather than deletes (04-data-model.md §14): the
// partial unique index permits exactly one non-superseded bill per
// (Scope, subject, PeriodKey), so a recomputed bill sets the previous one to
// BillStatusSuperseded and inserts a new row. The old figures stay readable,
// which is what makes a disputed invoice explainable months later.
//
// The quantity and charge columns are NOT NULL with a default of 0 in the
// schema, so they are values rather than pointers: a bill that has been
// computed has a number for every charge, even when that number is zero.
// The unit prices are nullable because a bill computed under dynamic pricing
// has no single fixed price to record.
type Bill struct {
	ID        uuid.UUID
	CompanyID uuid.UUID
	// BuildingID and AnalyzerID are nullable and which one is set follows
	// from Scope: an analyzer bill sets both, a building bill sets
	// BuildingID, a company bill sets neither.
	BuildingID *uuid.UUID
	AnalyzerID *uuid.UUID
	Scope      BillScope

	// PeriodKey is char(7), 'YYYY-MM'. PeriodStart and PeriodEnd are the
	// exact UTC instants the period spans, which are NOT the month's UTC
	// boundaries: they follow the building's cutoff day in Europe/Istanbul.
	PeriodStart  time.Time
	PeriodEnd    time.Time
	PeriodKey    string
	DaysInPeriod int32

	// TariffID and TariffEffectiveFrom record which tariff version priced
	// this bill, so that editing a tariff afterwards cannot change what an
	// issued invoice claimed.
	TariffID            *uuid.UUID
	TariffEffectiveFrom *time.Time

	// Quantities, in kWh except where the name says otherwise.
	ActiveImport    decimal.Decimal
	T1Kwh           decimal.Decimal
	T2Kwh           decimal.Decimal
	T3Kwh           decimal.Decimal
	InductiveKvarh  decimal.Decimal
	CapacitiveKvarh decimal.Decimal
	ActiveExport    decimal.Decimal
	NetConsumption  decimal.Decimal
	LowTierKwh      decimal.Decimal
	HighTierKwh     decimal.Decimal
	MaxDemandKw     *decimal.Decimal
	TieredApplied   bool
	// DemandDataAvailable is false when no max demand was known, so no
	// overrun could be billed (R111).
	DemandDataAvailable bool

	// IndexStart and IndexEnd are the register readings at the period
	// boundaries, as jsonb. They are NOT NULL: a bill that cannot name the
	// two readings it differenced cannot be defended.
	IndexStart json.RawMessage
	IndexEnd   json.RawMessage

	// Unit prices actually used, numeric(18,6).
	EffectiveEnergyPrice *decimal.Decimal
	LowTierPrice         *decimal.Decimal
	HighTierPrice        *decimal.Decimal

	// Charges, TL.
	EnergyCost        decimal.Decimal
	DistributionCost  decimal.Decimal
	GreenEnergyCost   decimal.Decimal
	PowerCost         decimal.Decimal
	DemandOverrunCost decimal.Decimal
	ReactivePenalty   decimal.Decimal
	OtherTaxesCost    decimal.Decimal
	VatBase           decimal.Decimal
	VatCost           decimal.Decimal
	GenerationCredit  decimal.Decimal
	ExtraChargesCost  decimal.Decimal
	TotalCost         decimal.Decimal
	// Currency is the tariff's own; there is no FX (R127).
	Currency CurrencyCode

	// Reactive detail: the ratios computed, the thresholds they were compared
	// against, and whether a penalty resulted.
	InductiveRatio         *decimal.Decimal
	CapacitiveRatio        *decimal.Decimal
	InductiveThreshold     *decimal.Decimal
	CapacitiveThreshold    *decimal.Decimal
	ReactivePenaltyApplied bool
	ReactivePowerPrice     *decimal.Decimal

	GenerationUsage       GenerationUsage
	GenerationPricePerKwh *decimal.Decimal

	// Dynamic pricing outcome. PtfHoursMatched and PtfHoursMissing are how a
	// reader knows whether a PTF-priced bill had complete price data; a bill
	// with missing hours is not the same as one that priced them at zero.
	PtfYekdemUsed   bool
	PtfHoursMatched *int32
	PtfHoursMissing *int32
	// PtfHoursExpected and ConsumptionHoursMissing split the missing hours
	// into price-missing and consumption-missing (R108).
	PtfHoursExpected        *int32
	ConsumptionHoursMissing *int32
	PtfAverage              *decimal.Decimal
	YekdemUsed              *decimal.Decimal

	Status BillStatus
	// FlagReason is set when Status is BillStatusFlagged.
	FlagReason *string
	PdfPath    *string

	ComputedAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Bill line codes, shared by the billing engine and the renderer.
const (
	BillLineEnergy             = "energy"
	BillLineEnergyLowTier      = "energy_low_tier"
	BillLineEnergyHighTier     = "energy_high_tier"
	BillLineEnergyT1           = "energy_t1"
	BillLineEnergyT2           = "energy_t2"
	BillLineEnergyT3           = "energy_t3"
	BillLineDistribution       = "distribution"
	BillLineGreenEnergy        = "green_energy"
	BillLinePower              = "power"
	BillLineDemandOverrun      = "demand_overrun"
	BillLineReactiveInductive  = "reactive_inductive"
	BillLineReactiveCapacitive = "reactive_capacitive"
	BillLineVat                = "vat"
	BillLineGenerationCredit   = "generation_credit"
	BillLineExtraPrefix        = "extra:"
	BillLineTaxPrefix          = "tax:"
)

// BillLineCodes is every fixed line code; BillLineExtraPrefix and
// BillLineTaxPrefix cover the dynamic extra:<name> and tax:<name> codes.
var BillLineCodes = []string{
	BillLineEnergy, BillLineEnergyLowTier, BillLineEnergyHighTier, BillLineEnergyT1, BillLineEnergyT2, BillLineEnergyT3,
	BillLineDistribution, BillLineGreenEnergy, BillLinePower, BillLineDemandOverrun, BillLineReactiveInductive,
	BillLineReactiveCapacitive, BillLineVat, BillLineGenerationCredit,
}

// BillLine is one printed line of a bill. Mirrors table `bill_lines`
// (migration 00009).
//
// The lines are a rendering of the Bill's charge columns, not a second source
// of truth: the totals live on Bill and the sum of the lines must agree with
// them.
type BillLine struct {
	ID     uuid.UUID
	BillID uuid.UUID

	// Code is stable and machine-readable ('energy', 'distribution', 'vat',
	// 'tax:BTV', …); Label is the human text as printed, localised at render
	// time.
	Code  string
	Label string

	Quantity  *decimal.Decimal
	Unit      *string
	UnitPrice *decimal.Decimal
	// RatePct is a percent, for lines that are a rate rather than a price.
	RatePct *decimal.Decimal
	// Amount is NOT NULL: every line has a money amount.
	Amount    decimal.Decimal
	SortOrder int16
}

// BillMember records that an analyzer was rolled into a building or company
// bill. Mirrors table `bill_members` (migration 00009), primary key
// (bill_id, analyzer_id).
type BillMember struct {
	BillID     uuid.UUID
	AnalyzerID uuid.UUID
}

// BillHourlyDetail is one hour of a PTF-priced bill, feeding the hourly Excel
// export. Mirrors table `bill_hourly_detail` (migration 00009), primary key
// (bill_id, ts). Every column is NOT NULL: a stored hour is a fully priced
// hour.
type BillHourlyDetail struct {
	BillID uuid.UUID
	Ts     time.Time

	Consumption decimal.Decimal
	// PTF and Yekdem are TL/MWh; Kbk is the tariff's coefficient; UnitPrice
	// is the resulting TL/kWh and Cost the TL for the hour.
	PTF       decimal.Decimal
	Yekdem    decimal.Decimal
	Kbk       decimal.Decimal
	UnitPrice decimal.Decimal
	Cost      decimal.Decimal
}
