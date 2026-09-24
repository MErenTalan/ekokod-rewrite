package dto

import (
	"time"

	"github.com/google/uuid"
)

// 05 §12, F10a R300–R319.

// CarbonBuildingQuery names the building a carbon read is about.
type CarbonBuildingQuery struct {
	BuildingID uuid.UUID `query:"building_id" json:"-" validate:"required" required:"true"`
}

// CarbonOverviewRequest is GET /carbon/overview; year defaults to the current Istanbul year.
type CarbonOverviewRequest struct {
	BuildingID uuid.UUID `query:"building_id" json:"-" validate:"required" required:"true"`
	Year       *int      `query:"year" json:"-" validate:"omitempty,min=2000,max=2100" minimum:"2000" maximum:"2100"`
}

// CarbonAmount is one keyed emission figure in kg CO₂e.
type CarbonAmount struct {
	Key         string  `json:"key" required:"true"`
	TotalKgCO2e Decimal `json:"kgco2e" required:"true"`
}

// CarbonMonth is one month of the year and of the year before.
type CarbonMonth struct {
	Month    int     `json:"month" required:"true"`
	Current  Decimal `json:"current_kgco2e" required:"true"`
	Previous Decimal `json:"previous_kgco2e" required:"true"`
}

// CarbonOverview is R310.
type CarbonOverview struct {
	Year            int              `json:"year" required:"true"`
	TotalKgCO2e     Decimal          `json:"total_kgco2e" required:"true"`
	ActivityCount   int              `json:"activity_count" required:"true"`
	RegisteredCount int              `json:"registered_count" required:"true"`
	PendingCount    int              `json:"pending_count" required:"true"`
	HighestSource   *CarbonAmount    `json:"highest_source,omitempty"`
	ByCategory      []CarbonAmount   `json:"by_category" required:"true"`
	ByScope         []CarbonAmount   `json:"by_scope" required:"true"`
	Monthly         []CarbonMonth    `json:"monthly" required:"true"`
	Recent          []CarbonActivity `json:"recent" required:"true"`
}

// CarbonCatalogueSub is one sub-category with its derived scope and ISO category (R301).
type CarbonCatalogueSub struct {
	Key         string `json:"key" required:"true"`
	Scope       string `json:"scope" required:"true" enum:"scope_1,scope_2,scope_3"`
	IsoCategory string `json:"iso_category" required:"true"`
}

// CarbonCatalogueMain is one main category (R300).
type CarbonCatalogueMain struct {
	Key  string               `json:"key" required:"true"`
	Subs []CarbonCatalogueSub `json:"subs" required:"true"`
}

// CarbonCatalogue is R316.
type CarbonCatalogue struct {
	Items []CarbonCatalogueMain `json:"items" required:"true"`
}

// CarbonSelection is a building's declared sub-categories.
type CarbonSelection struct {
	ActivityKeys []string `json:"activity_keys" required:"true"`
}

// CarbonSelectionRequest is PUT /carbon/selected-activities (R315).
type CarbonSelectionRequest struct {
	BuildingID   uuid.UUID `query:"building_id" json:"-" validate:"required" required:"true"`
	ActivityKeys []string  `json:"activity_keys" validate:"required,max=18" required:"true"`
}

// CarbonActivitiesRequest is R319's list filter.
type CarbonActivitiesRequest struct {
	BuildingID *uuid.UUID `query:"building_id" json:"-"`
	From       *Date      `query:"from" json:"-"`
	To         *Date      `query:"to" json:"-"`
	Scope      *string    `query:"scope" json:"-" validate:"omitempty,oneof=scope_1 scope_2 scope_3" enum:"scope_1,scope_2,scope_3"`
	Status     *string    `query:"status" json:"-" validate:"omitempty,oneof=pending approved rejected" enum:"pending,approved,rejected"`
	Type       *string    `query:"type" json:"-" validate:"omitempty,max=64"`
	Automated  *bool      `query:"automated" json:"-"`
	PageRequest
}

// CarbonActivityBody is the shared create/update body (R305); the emission,
// scope and ISO category are computed, never accepted.
type CarbonActivityBody struct {
	SubCategory string            `json:"sub_category" validate:"required,max=64" required:"true"`
	FactorKey   string            `json:"factor_key" validate:"required,max=200" required:"true"`
	Quantity    *Decimal          `json:"quantity" validate:"required" required:"true"`
	Unit        string            `json:"unit" validate:"required,max=32" required:"true"`
	PeriodStart *Date             `json:"period_start" validate:"required" required:"true"`
	PeriodEnd   *Date             `json:"period_end" validate:"required" required:"true"`
	Description *string           `json:"description,omitempty" validate:"omitempty,max=500"`
	Details     map[string]string `json:"details,omitempty"`
}

// CarbonActivityCreateRequest is POST /carbon/activities.
type CarbonActivityCreateRequest struct {
	BuildingID uuid.UUID `json:"building_id" validate:"required" required:"true"`
	CarbonActivityBody
}

// CarbonActivityUpdateRequest is PATCH /carbon/activities/{id} (R306).
type CarbonActivityUpdateRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	CarbonActivityBody
}

// CarbonStatusRequest is R307.
type CarbonStatusRequest struct {
	ID     uuid.UUID `path:"id" json:"-"`
	Status string    `json:"status" validate:"required,oneof=approved rejected" required:"true" enum:"approved,rejected"`
}

// CarbonActivity is one stored activity with the factor snapshot it used.
type CarbonActivity struct {
	ID                   uuid.UUID         `json:"id" required:"true"`
	BuildingID           uuid.UUID         `json:"building_id" required:"true"`
	MainCategory         string            `json:"main_category" required:"true"`
	SubCategory          string            `json:"sub_category" required:"true"`
	ActivityType         string            `json:"activity_type" required:"true"`
	PeriodStart          Date              `json:"period_start" required:"true"`
	PeriodEnd            Date              `json:"period_end" required:"true"`
	Quantity             Decimal           `json:"quantity" required:"true"`
	Unit                 string            `json:"unit" required:"true"`
	FactorID             *uuid.UUID        `json:"factor_id"`
	FactorKey            *string           `json:"factor_key"`
	FactorValue          *Decimal          `json:"factor_value"`
	ConversionMultiplier Decimal           `json:"conversion_multiplier" required:"true"`
	EmissionKgCO2e       Decimal           `json:"emission_kgco2e" required:"true"`
	Scope                string            `json:"scope" required:"true" enum:"scope_1,scope_2,scope_3"`
	IsoCategory          string            `json:"iso_category" required:"true"`
	Description          *string           `json:"description"`
	Details              map[string]string `json:"details" required:"true"`
	Status               string            `json:"status" required:"true" enum:"pending,approved,rejected"`
	IsAutomated          bool              `json:"is_automated" required:"true"`
	CreatedBy            *uuid.UUID        `json:"created_by"`
	CreatedAt            time.Time         `json:"created_at" required:"true"`
	UpdatedAt            time.Time         `json:"updated_at" required:"true"`
}

// EmissionFactorsRequest narrows the catalogue (R302).
type EmissionFactorsRequest struct {
	SubCategory  *string `query:"sub_category" json:"-" validate:"omitempty,max=64"`
	MainCategory *string `query:"main_category" json:"-" validate:"omitempty,max=64"`
	Q            *string `query:"q" json:"-" validate:"omitempty,max=100"`
}

// EmissionFactorConversion is one supported unit.
type EmissionFactorConversion struct {
	Unit       string  `json:"unit" required:"true"`
	Multiplier Decimal `json:"multiplier" required:"true"`
	Label      string  `json:"label" required:"true"`
}

// EmissionFactorView is one effective catalogue entry (R302).
type EmissionFactorView struct {
	ID                 uuid.UUID                  `json:"id" required:"true"`
	Key                string                     `json:"key" required:"true"`
	Label              string                     `json:"label" required:"true"`
	MainCategory       string                     `json:"main_category" required:"true"`
	SubCategories      []string                   `json:"sub_categories" required:"true"`
	CategoryPath       []string                   `json:"category_path" required:"true"`
	BaseFactor         Decimal                    `json:"base_factor" required:"true"`
	BaseUnit           string                     `json:"base_unit" required:"true"`
	FuelType           *string                    `json:"fuel_type"`
	VehicleType        *string                    `json:"vehicle_type"`
	Scope              *string                    `json:"scope"`
	IsoCategory        *string                    `json:"iso_category"`
	Status             *string                    `json:"status"`
	Source             *string                    `json:"source"`
	SourceYear         *int16                     `json:"source_year"`
	SourceURL          *string                    `json:"source_url"`
	Conversions        []EmissionFactorConversion `json:"conversions" required:"true"`
	Overridden         bool                       `json:"overridden" required:"true"`
	PlatformBaseFactor *Decimal                   `json:"platform_base_factor"`
	UpdatedAt          time.Time                  `json:"updated_at" required:"true"`
}

// EmissionFactorList is GET /carbon/emission-factors.
type EmissionFactorList struct {
	Items []EmissionFactorView `json:"items" required:"true"`
}

// EmissionFactorOverrideRequest is R303.
type EmissionFactorOverrideRequest struct {
	ID         uuid.UUID `path:"id" json:"-"`
	BaseFactor *Decimal  `json:"base_factor" validate:"required" required:"true"`
	Source     *string   `json:"source,omitempty" validate:"omitempty,max=200"`
	SourceYear *int16    `json:"source_year,omitempty"`
	SourceURL  *string   `json:"source_url,omitempty" validate:"omitempty,max=500"`
}

// CarbonReportsRequest is the history filter.
type CarbonReportsRequest struct {
	BuildingID *uuid.UUID `query:"building_id" json:"-"`
	PageRequest
}

// CarbonReportRequest is R312.
type CarbonReportRequest struct {
	BuildingID uuid.UUID `json:"building_id" validate:"required" required:"true"`
	ReportType string    `json:"report_type" validate:"required,oneof=ghg iso" required:"true" enum:"ghg,iso"`
	From       *Date     `json:"from" validate:"required" required:"true"`
	To         *Date     `json:"to" validate:"required" required:"true"`
	Name       *string   `json:"name,omitempty" validate:"omitempty,max=120"`
}

// CarbonReportSummary is one history row.
type CarbonReportSummary struct {
	ID         uuid.UUID `json:"id" required:"true"`
	BuildingID uuid.UUID `json:"building_id" required:"true"`
	Name       string    `json:"name" required:"true"`
	ReportType string    `json:"report_type" required:"true" enum:"ghg,iso"`
	Period     string    `json:"period" required:"true"`
	CreatedAt  time.Time `json:"created_at" required:"true"`
}
