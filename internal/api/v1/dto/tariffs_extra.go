package dto

import (
	"time"

	"github.com/google/uuid"
)

// TariffTemplateFields is a named, reusable tariff definition (01 §7.11).
type TariffTemplateFields struct {
	Name        string  `json:"name" validate:"required,max=200" required:"true"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000"`
	IsDefault   bool    `json:"is_default"`
	// Tariff is the definition the template applies, validated exactly as a
	// tariff version is (R240: no second validation path).
	Tariff TariffFields `json:"tariff" validate:"required" required:"true"`
}

// TariffTemplate is a stored template.
type TariffTemplate struct {
	ID uuid.UUID `json:"id" required:"true"`
	TariffTemplateFields
	CreatedAt time.Time `json:"created_at" required:"true"`
	UpdatedAt time.Time `json:"updated_at" required:"true"`
}

// TariffTemplateUpdateRequest is PATCH /tariff-templates/{id}; it replaces.
type TariffTemplateUpdateRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	TariffTemplateFields
}

// TariffTemplateListRequest is GET /tariff-templates.
type TariffTemplateListRequest struct {
	PageRequest
}

// TariffTemplateApplyRequest is POST /tariff-templates/{id}/apply.
type TariffTemplateApplyRequest struct {
	ID            uuid.UUID   `path:"id" json:"-"`
	BuildingIDs   []uuid.UUID `json:"building_ids" validate:"required,min=1,max=500" required:"true"`
	EffectiveFrom Date        `json:"effective_from" validate:"required" required:"true"`
}

// BulkTariffRequest is POST /buildings/bulk-tariff.
type BulkTariffRequest struct {
	BuildingIDs []uuid.UUID  `json:"building_ids" validate:"required,min=1,max=500" required:"true"`
	Tariff      TariffFields `json:"tariff" validate:"required" required:"true"`
}

// BulkTariffResult names every building that received a version. A call that
// writes nothing returns an empty list rather than a count that hides which
// buildings were touched (R241).
type BulkTariffResult struct {
	BuildingIDs []uuid.UUID `json:"building_ids" required:"true"`
	TariffIDs   []uuid.UUID `json:"tariff_ids" required:"true"`
}

// BuildingTariffState is one row of the current-state table: a building with
// no tariff appears with a null id, because "which buildings have no tariff"
// is the question the screen asks.
type BuildingTariffState struct {
	BuildingID    uuid.UUID  `json:"building_id" required:"true"`
	BuildingName  string     `json:"building_name" required:"true"`
	TariffID      *uuid.UUID `json:"tariff_id"`
	TariffName    *string    `json:"tariff_name"`
	EffectiveFrom *Date      `json:"effective_from"`
	UsePtfYekdem  bool       `json:"use_ptf_yekdem" required:"true"`
}

// BulkTariffAssignment is one history row.
type BulkTariffAssignment struct {
	ID            uuid.UUID   `json:"id" required:"true"`
	TemplateID    *uuid.UUID  `json:"template_id"`
	TariffName    *string     `json:"tariff_name"`
	EffectiveFrom Date        `json:"effective_from" required:"true"`
	BuildingIDs   []uuid.UUID `json:"building_ids" required:"true"`
	CreatedBy     *uuid.UUID  `json:"created_by"`
	CreatedAt     time.Time   `json:"created_at" required:"true"`
}

// IcmalWarning is one row-level note from the parse or the derivation.
type IcmalWarning struct {
	Row  int    `json:"row" required:"true"`
	Code string `json:"code" required:"true"`
	Text string `json:"text" required:"true"`
}

// IcmalCoefficient is one derived value with the evidence behind it (02 §8.3).
type IcmalCoefficient struct {
	Value   *Decimal `json:"value"`
	Samples int      `json:"samples" required:"true"`
	StdDev  *Decimal `json:"std_dev"`
	// Stable is null below two samples: stability is not a verdict that can
	// be reached from one period.
	Stable           *bool    `json:"stable"`
	BackCalcErrorPct *Decimal `json:"back_calc_error_pct"`
}

// IcmalAnalysis is the derivation for one ETSO code.
type IcmalAnalysis struct {
	EtsoCode                 string                      `json:"etso_code" required:"true"`
	BuildingID               *uuid.UUID                  `json:"building_id"`
	Periods                  []string                    `json:"periods" required:"true"`
	EnergyKbk                IcmalCoefficient            `json:"energy_kbk" required:"true"`
	DistributionTlPerKwh     IcmalCoefficient            `json:"distribution_tl_per_kwh" required:"true"`
	PowerUnitPrice           IcmalCoefficient            `json:"power_unit_price" required:"true"`
	ImpliedContractedPowerKw *Decimal                    `json:"implied_contracted_power_kw"`
	ReactiveUnitPrice        IcmalCoefficient            `json:"reactive_unit_price" required:"true"`
	ReactiveKbk              IcmalCoefficient            `json:"reactive_kbk" required:"true"`
	VatRate                  IcmalCoefficient            `json:"vat_rate" required:"true"`
	Taxes                    map[string]IcmalCoefficient `json:"taxes" required:"true"`
	IsMultiTime              *bool                       `json:"is_multi_time"`
	Term                     *string                     `json:"term"`
	VoltageLevel             *string                     `json:"voltage_level"`
	// WithinTolerance is 02 §8.4's 2 % back-calculation target.
	WithinTolerance            bool           `json:"within_tolerance" required:"true"`
	OveruseRequiresManualEntry bool           `json:"overuse_requires_manual_entry" required:"true"`
	Warnings                   []IcmalWarning `json:"warnings" required:"true"`
}

// IcmalImport is an analysed upload. Nothing is written to a tariff until the
// operator confirms (02 §8.4, R245).
type IcmalImport struct {
	ID        uuid.UUID       `json:"id" required:"true"`
	FileName  string          `json:"file_name" required:"true"`
	Status    string          `json:"status" required:"true" enum:"pending,analysed,applied"`
	RowCount  int             `json:"row_count" required:"true"`
	Analyses  []IcmalAnalysis `json:"analyses" required:"true"`
	Unmatched []string        `json:"unmatched" required:"true"`
	Warnings  []IcmalWarning  `json:"warnings" required:"true"`
	CreatedAt time.Time       `json:"created_at" required:"true"`
}

// IcmalConfirmation is one operator-confirmed building.
type IcmalConfirmation struct {
	BuildingID    uuid.UUID `json:"building_id" validate:"required" required:"true"`
	EtsoCode      string    `json:"etso_code" validate:"required,max=40" required:"true"`
	EffectiveFrom Date      `json:"effective_from" validate:"required" required:"true"`
	// Base is required when the building has no applicable tariff at
	// EffectiveFrom; the service says so with its own error.
	Base *TariffFields `json:"base,omitempty"`
}

// IcmalApplyRequest is POST /icmal-imports/{id}/apply.
type IcmalApplyRequest struct {
	ID            uuid.UUID           `path:"id" json:"-"`
	Confirmations []IcmalConfirmation `json:"confirmations" validate:"required,min=1,max=500,dive" required:"true"`
}

// SolarTariffFields is one plant feed-in price (01 §7.11, tab 2).
type SolarTariffFields struct {
	PlantID       uuid.UUID `json:"plant_id" validate:"required" required:"true"`
	EffectiveFrom Date      `json:"effective_from" validate:"required" required:"true"`
	FeedInTariff  *Decimal  `json:"feed_in_tariff" validate:"required" required:"true"`
	PurchasePrice *Decimal  `json:"purchase_price,omitempty"`
	Currency      string    `json:"currency,omitempty" validate:"omitempty,oneof=TRY USD EUR" enum:"TRY,USD,EUR"`
	Notes         *string   `json:"notes,omitempty" validate:"omitempty,max=2000"`
}

// SolarTariff is a stored plant price.
type SolarTariff struct {
	ID uuid.UUID `json:"id" required:"true"`
	SolarTariffFields
	CreatedAt time.Time `json:"created_at" required:"true"`
}

// SolarTariffListRequest is GET /solar-tariffs.
type SolarTariffListRequest struct {
	PlantID uuid.UUID `query:"plant_id" json:"-" validate:"required" required:"true"`
	PageRequest
}

// NationalTariffFields is one row of the platform default catalogue (R243).
type NationalTariffFields struct {
	EffectiveFrom     Date     `json:"effective_from" validate:"required" required:"true"`
	UserGroup         string   `json:"user_group" validate:"required,oneof=residential residential_plus commercial commercial_plus industrial agricultural lighting martyrs_families public_lighting" required:"true" enum:"residential,residential_plus,commercial,commercial_plus,industrial,agricultural,lighting,martyrs_families,public_lighting"`
	VoltageLevel      string   `json:"voltage_level" validate:"required,oneof=lv mv" required:"true" enum:"lv,mv"`
	Term              string   `json:"term" validate:"required,oneof=monomial binomial" required:"true" enum:"monomial,binomial"`
	EnergyPrice       *Decimal `json:"energy_price" validate:"required" required:"true"`
	T1Price           *Decimal `json:"t1_price,omitempty"`
	T2Price           *Decimal `json:"t2_price,omitempty"`
	T3Price           *Decimal `json:"t3_price,omitempty"`
	DistributionPrice *Decimal `json:"distribution_price" validate:"required" required:"true"`
	PowerPrice        *Decimal `json:"power_price,omitempty"`
	OverusePrice      *Decimal `json:"overuse_price,omitempty"`
	DailyThresholdKwh *Decimal `json:"daily_threshold_kwh,omitempty"`
	VatRate           *Decimal `json:"vat_rate" validate:"required" required:"true"`
	Source            *string  `json:"source,omitempty" validate:"omitempty,max=200"`
}

// NationalTariff is a stored catalogue row.
type NationalTariff struct {
	ID uuid.UUID `json:"id" required:"true"`
	NationalTariffFields
	CreatedAt time.Time `json:"created_at" required:"true"`
}

// NationalTariffListRequest is GET /national-tariff-schedule; it is public,
// so it carries no scope at all.
type NationalTariffListRequest struct {
	UserGroup    string `query:"user_group" json:"-" validate:"omitempty,oneof=residential residential_plus commercial commercial_plus industrial agricultural lighting martyrs_families public_lighting"`
	VoltageLevel string `query:"voltage_level" json:"-" validate:"omitempty,oneof=lv mv"`
	Term         string `query:"term" json:"-" validate:"omitempty,oneof=monomial binomial"`
	PageRequest
}
