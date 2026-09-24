package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Tariff is one priced contract, effective from a date. Mirrors table
// `tariffs` (migration 00006).
//
// Resolution is by EffectiveFrom: the applicable tariff for a period is the
// live row with the greatest EffectiveFrom not after the period, which is why
// there is no end date and no "is current" flag to fall out of step.
//
// The schema's check constraints are business rules, not formalities, and the
// nullable price fields exist because of them: a single_time tariff must carry
// SingleTimePrice, a multi_time tariff must carry all three of T1/T2/T3,
// generation_usage 'subtract_from_total' must carry GenerationPricePerKwh, and
// UsePtfYekdem must carry KbkEnergy. A struct cannot express "exactly one of
// these groups"; the database refuses the write, and callers should treat a
// nil price in an applicable group as a data error rather than as zero.
type Tariff struct {
	ID        uuid.UUID
	CompanyID uuid.UUID
	// BuildingID is nullable: a tariff may be defined for the whole company.
	BuildingID *uuid.UUID

	Name *string
	// EffectiveFrom is a `date` column; only its calendar date is meaningful.
	EffectiveFrom time.Time

	Currency      CurrencyCode
	EnergyType    EnergyType
	VoltageLevel  VoltageLevel
	UserGroup     DistributionUserGroup
	PriceType     PriceType
	Term          TariffTerm
	SupplyCompany SupplyCompany

	// Fixed prices, TL/kWh unless noted, numeric(18,6).
	SingleTimePrice           *decimal.Decimal
	T1Price                   *decimal.Decimal
	T2Price                   *decimal.Decimal
	T3Price                   *decimal.Decimal
	OverusePrice              *decimal.Decimal
	OveruseThresholdKwhPerDay *decimal.Decimal
	// DistributionCost and ReactivePowerPrice are NOT NULL in the schema:
	// every tariff has them, so they are values rather than pointers.
	DistributionCost            decimal.Decimal
	ReactivePowerPrice          decimal.Decimal
	GreenEnergyPrice            *decimal.Decimal
	GreenEnergyDistributionCost *decimal.Decimal
	ContractedPowerKw           *decimal.Decimal
	// PowerUnitPrice is TL/kW/month and is what makes a binomial tariff
	// binomial.
	PowerUnitPrice *decimal.Decimal

	GenerationUsage       GenerationUsage
	GenerationPricePerKwh *decimal.Decimal

	// VatRate is NOT NULL and has no default in the schema — deliberately, so
	// that a tariff cannot be created with an unstated VAT rate that quietly
	// prices at zero.
	VatRate decimal.Decimal

	// Dynamic pricing: when UsePtfYekdem is set the energy charge comes from
	// the hourly market price plus YEKDEM plus the KBK coefficients below,
	// rather than from the fixed prices above.
	UsePtfYekdem                bool
	KbkEnergy                   *decimal.Decimal
	KbkT1                       *decimal.Decimal
	KbkT2                       *decimal.Decimal
	KbkT3                       *decimal.Decimal
	KbkPowerPrice               *decimal.Decimal
	KbkOverusePrice             *decimal.Decimal
	KbkReactivePower            *decimal.Decimal
	KbkDistributionCostTlPerKwh *decimal.Decimal
	// UseManualYekdem takes the YEKDEM value from this tariff's own
	// ManualYekdem rows instead of the platform-wide yekdem_monthly table.
	UseManualYekdem bool

	// R119: where a PTF tariff's power, reactive and distribution unit prices
	// come from — base × KBK coefficient, or the tariff's own fixed column.
	PowerPriceSource        PriceSource
	ReactivePriceSource     PriceSource
	DistributionPriceSource PriceSource

	CreatedBy *uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// TariffTax is one additional tax applied to a tariff's energy charge, such as
// BTV, Enerji Fonu or TRT Payı. Mirrors table `tariff_taxes`
// (migration 00006).
type TariffTax struct {
	ID       uuid.UUID
	TariffID uuid.UUID

	Name string
	// Rate is a PERCENT, numeric(6,3) — not a ratio. 2.000 means 2%.
	Rate      decimal.Decimal
	SortOrder int16
}

// TariffExtraCharge is a custom contract charge billed as its own line inside
// the VAT base (R126). Mirrors table `tariff_extra_charges` (migration 00014).
type TariffExtraCharge struct {
	ID       uuid.UUID
	TariffID uuid.UUID
	Name     string
	Basis    ExtraChargeBasis
	// Amount is TL per basis unit, or a percent for pct_of_energy; negative is a discount.
	Amount    decimal.Decimal
	SortOrder int16
}

// TariffManualYekdem is one month's manually entered YEKDEM value for a
// tariff, used when Tariff.UseManualYekdem is set. Mirrors table
// `tariff_manual_yekdem` (migration 00006), primary key (tariff_id, year, month).
type TariffManualYekdem struct {
	TariffID uuid.UUID
	Year     int16
	Month    int16
	// Value is TL/MWh.
	Value decimal.Decimal
}

// TariffTemplate is a saved tariff definition a company can apply to create a
// new Tariff. Mirrors table `tariff_templates` (migration 00006);
// (company_id, lower(name)) is unique.
//
// Payload is the full tariff definition as jsonb. It is raw JSON here because
// applying a template copies it into `tariffs`, where the columns are typed —
// decoding it into a Tariff is the applying code's job, not this struct's.
type TariffTemplate struct {
	ID        uuid.UUID
	CompanyID uuid.UUID

	Name        string
	Description *string
	IsDefault   bool
	Payload     json.RawMessage

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TariffBulkAssignment records one bulk assignment. Mirrors table
// `tariff_bulk_assignments` (migration 00017). BuildingIDs lists the buildings
// that actually received a version: a call that failed halfway records what
// happened, never what was asked for (R241).
type TariffBulkAssignment struct {
	ID        uuid.UUID
	CompanyID uuid.UUID
	// TemplateID is set when the assignment came from a template.
	TemplateID    *uuid.UUID
	TariffName    *string
	EffectiveFrom time.Time
	BuildingIDs   []uuid.UUID
	CreatedBy     *uuid.UUID
	CreatedAt     time.Time
}

// SolarTariff prices a plant's generation, effective from a date. Mirrors
// table `solar_tariffs` (migration 00006). It resolves by EffectiveFrom the
// same way Tariff does.
type SolarTariff struct {
	ID        uuid.UUID
	CompanyID uuid.UUID
	PlantID   uuid.UUID

	EffectiveFrom time.Time
	// FeedInTariff is TL/kWh and NOT NULL: a solar tariff with no feed-in
	// price prices nothing.
	FeedInTariff  decimal.Decimal
	PurchasePrice *decimal.Decimal
	Currency      CurrencyCode
	Notes         *string

	CreatedAt time.Time
	DeletedAt *time.Time
}

// NationalTariffScheduleEntry is one published national tariff row, backing the
// public bill calculator. Mirrors table `national_tariff_schedule`
// (migration 00006); (effective_from, user_group, voltage_level, term) is
// unique.
//
// This table is platform-wide: it has no company_id, so it is the one tariff
// surface a scoped repository reads without a tenant predicate.
type NationalTariffScheduleEntry struct {
	ID uuid.UUID

	EffectiveFrom time.Time
	UserGroup     DistributionUserGroup
	VoltageLevel  VoltageLevel
	Term          TariffTerm

	EnergyPrice       decimal.Decimal
	T1Price           *decimal.Decimal
	T2Price           *decimal.Decimal
	T3Price           *decimal.Decimal
	DistributionPrice decimal.Decimal
	PowerPrice        *decimal.Decimal
	OverusePrice      *decimal.Decimal
	DailyThresholdKwh *decimal.Decimal
	VatRate           decimal.Decimal

	Source    *string
	CreatedAt time.Time
}

// IcmalImport is one uploaded distributor summary (icmal) file. Mirrors table
// `icmal_imports` (migration 00006).
type IcmalImport struct {
	ID         uuid.UUID
	CompanyID  uuid.UUID
	UploadedBy *uuid.UUID

	FileName string
	RowCount int32
	// Status is 'pending', 'analysed', 'applied' or 'rejected'. It is a plain
	// text column with a default, not a SQL enum.
	Status string
	// Result holds derived coefficients, statistics and warnings.
	Result json.RawMessage

	CreatedAt time.Time
}

// IcmalRow is one line of an imported icmal file: what the distributor said a
// building consumed and was charged in a period, which is what a computed Bill
// is reconciled against.
//
// Mirrors table `icmal_rows` (migration 00006). Almost every column is
// nullable because the file formats differ per distributor and a missing
// figure must stay missing rather than become zero.
type IcmalRow struct {
	ID         uuid.UUID
	ImportID   uuid.UUID
	BuildingID *uuid.UUID

	// Period is char(6), 'YYYYMM'.
	Period   string
	EtsoCode *string

	TotalKwh *decimal.Decimal
	T0Kwh    *decimal.Decimal
	T1Kwh    *decimal.Decimal
	T2Kwh    *decimal.Decimal
	T3Kwh    *decimal.Decimal

	EnergyCharge       *decimal.Decimal
	DistributionCharge *decimal.Decimal
	ReactiveCharge     *decimal.Decimal
	PowerCharge        *decimal.Decimal
	OveruseCharge      *decimal.Decimal

	InductiveKvarh  *decimal.Decimal
	CapacitiveKvarh *decimal.Decimal
	DemandKw        *decimal.Decimal

	VatBase          *decimal.Decimal
	Vat              *decimal.Decimal
	Btv              *decimal.Decimal
	EnergyFund       *decimal.Decimal
	Trt              *decimal.Decimal
	PriceDifference  *decimal.Decimal
	CorrectionAmount *decimal.Decimal

	IsCancelled bool
	// Term, VoltageLevel and IsMultiTime are the file's own free-text
	// statements of what the tariff was. They are NOT the typed enums used
	// elsewhere: the file may say anything, and coercing it here would throw
	// away the evidence needed to explain a mismatch.
	Term         *string
	VoltageLevel *string
	IsMultiTime  *bool

	Raw json.RawMessage
}
