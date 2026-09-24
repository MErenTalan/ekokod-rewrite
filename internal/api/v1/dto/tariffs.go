package dto

import (
	"time"

	"github.com/google/uuid"
)

// TariffTax is one additional tax or fund; rate is required (F4 deviation).
type TariffTax struct {
	Name string   `json:"name" validate:"required,max=100" required:"true"`
	Rate *Decimal `json:"rate" validate:"required" required:"true"`
}

// TariffExtraCharge is one custom charge (R126).
type TariffExtraCharge struct {
	Name   string   `json:"name" validate:"required,max=100" required:"true"`
	Basis  string   `json:"basis" validate:"required,oneof=per_kwh per_contracted_kw per_max_demand_kw fixed_per_period pct_of_energy" required:"true" enum:"per_kwh,per_contracted_kw,per_max_demand_kw,fixed_per_period,pct_of_energy"`
	Amount *Decimal `json:"amount" validate:"required" required:"true"`
}

// TariffManualYekdem is one manual monthly YEKDEM value.
type TariffManualYekdem struct {
	Year  int16    `json:"year" validate:"required,min=2000,max=2100" required:"true"`
	Month int16    `json:"month" validate:"required,min=1,max=12" required:"true"`
	Value *Decimal `json:"value" validate:"required" required:"true"`
}

// TariffFields is a full tariff version (R178); PATCH replaces the version.
type TariffFields struct {
	BuildingID                  *uuid.UUID           `json:"building_id,omitempty"`
	Name                        *string              `json:"name,omitempty" validate:"omitempty,max=200"`
	EffectiveFrom               Date                 `json:"effective_from" validate:"required" required:"true"`
	Currency                    string               `json:"currency" validate:"required,oneof=TRY USD EUR" required:"true" enum:"TRY,USD,EUR"`
	EnergyType                  string               `json:"energy_type" validate:"required,oneof=grid_energy green_energy" required:"true" enum:"grid_energy,green_energy"`
	VoltageLevel                string               `json:"voltage_level" validate:"required,oneof=lv mv" required:"true" enum:"lv,mv"`
	UserGroup                   string               `json:"user_group" validate:"required,oneof=residential residential_plus commercial commercial_plus industrial agricultural lighting martyrs_families public_lighting" required:"true" enum:"residential,residential_plus,commercial,commercial_plus,industrial,agricultural,lighting,martyrs_families,public_lighting"`
	PriceType                   string               `json:"price_type" validate:"required,oneof=single_time multi_time" required:"true" enum:"single_time,multi_time"`
	Term                        string               `json:"term" validate:"required,oneof=monomial binomial" required:"true" enum:"monomial,binomial"`
	SupplyCompany               string               `json:"supply_company" validate:"required,oneof=incumbent private" required:"true" enum:"incumbent,private"`
	SingleTimePrice             *Decimal             `json:"single_time_price,omitempty"`
	T1Price                     *Decimal             `json:"t1_price,omitempty"`
	T2Price                     *Decimal             `json:"t2_price,omitempty"`
	T3Price                     *Decimal             `json:"t3_price,omitempty"`
	OverusePrice                *Decimal             `json:"overuse_price,omitempty"`
	OveruseThresholdKwhPerDay   *Decimal             `json:"overuse_threshold_kwh_per_day,omitempty"`
	DistributionCost            *Decimal             `json:"distribution_cost" validate:"required" required:"true"`
	ReactivePowerPrice          *Decimal             `json:"reactive_power_price" validate:"required" required:"true"`
	GreenEnergyPrice            *Decimal             `json:"green_energy_price,omitempty"`
	GreenEnergyDistributionCost *Decimal             `json:"green_energy_distribution_cost,omitempty"`
	ContractedPowerKw           *Decimal             `json:"contracted_power_kw,omitempty"`
	PowerUnitPrice              *Decimal             `json:"power_unit_price,omitempty"`
	GenerationUsage             string               `json:"generation_usage" validate:"required,oneof=none subtract_from_consumption subtract_from_total" required:"true" enum:"none,subtract_from_consumption,subtract_from_total"`
	GenerationPricePerKwh       *Decimal             `json:"generation_price_per_kwh,omitempty"`
	VatRate                     *Decimal             `json:"vat_rate" validate:"required" required:"true"`
	UsePtfYekdem                bool                 `json:"use_ptf_yekdem"`
	KbkEnergy                   *Decimal             `json:"kbk_energy,omitempty"`
	KbkT1                       *Decimal             `json:"kbk_t1,omitempty"`
	KbkT2                       *Decimal             `json:"kbk_t2,omitempty"`
	KbkT3                       *Decimal             `json:"kbk_t3,omitempty"`
	KbkPowerPrice               *Decimal             `json:"kbk_power_price,omitempty"`
	KbkOverusePrice             *Decimal             `json:"kbk_overuse_price,omitempty"`
	KbkReactivePower            *Decimal             `json:"kbk_reactive_power,omitempty"`
	KbkDistributionCostTlPerKwh *Decimal             `json:"kbk_distribution_cost_tl_per_kwh,omitempty"`
	UseManualYekdem             bool                 `json:"use_manual_yekdem"`
	PowerPriceSource            string               `json:"power_price_source,omitempty" validate:"omitempty,oneof=kbk fixed" enum:"kbk,fixed"`
	ReactivePriceSource         string               `json:"reactive_price_source,omitempty" validate:"omitempty,oneof=kbk fixed" enum:"kbk,fixed"`
	DistributionPriceSource     string               `json:"distribution_price_source,omitempty" validate:"omitempty,oneof=kbk fixed" enum:"kbk,fixed"`
	Taxes                       []TariffTax          `json:"taxes" validate:"max=20,dive"`
	ExtraCharges                []TariffExtraCharge  `json:"extra_charges" validate:"max=20,dive"`
	ManualYekdem                []TariffManualYekdem `json:"manual_yekdem" validate:"max=240,dive"`
}

// TariffUpdateRequest is PATCH /tariffs/{id}.
type TariffUpdateRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	TariffFields
}

// Tariff is a stored tariff version with its collections.
type Tariff struct {
	ID uuid.UUID `json:"id" required:"true"`
	TariffFields
	CreatedAt time.Time `json:"created_at" required:"true"`
	UpdatedAt time.Time `json:"updated_at" required:"true"`
}

// TariffSummaryItem is a list row.
type TariffSummaryItem struct {
	ID            uuid.UUID  `json:"id" required:"true"`
	BuildingID    *uuid.UUID `json:"building_id"`
	Name          *string    `json:"name"`
	EffectiveFrom Date       `json:"effective_from" required:"true"`
	PriceType     string     `json:"price_type" required:"true"`
	Term          string     `json:"term" required:"true"`
	UsePtfYekdem  bool       `json:"use_ptf_yekdem" required:"true"`
}

// TariffListRequest is GET /tariffs.
type TariffListRequest struct {
	BuildingID *uuid.UUID `query:"building_id" json:"-"`
	PageRequest
}

// TariffApplicableRequest is GET /tariffs/applicable.
type TariffApplicableRequest struct {
	BuildingID uuid.UUID `query:"building_id" json:"-" validate:"required" required:"true"`
	Date       Date      `query:"date" json:"-" validate:"required" required:"true"`
}
