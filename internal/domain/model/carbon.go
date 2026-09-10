package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// EmissionFactor is one entry of the emission factor catalogue. Mirrors table
// `emission_factors` (migration 00007).
//
// CompanyID is nullable and its null means "platform master catalogue": the
// unique index coalesces it to the zero uuid, so a company may shadow a
// platform key with its own factor of the same Key but cannot have two of its
// own.
type EmissionFactor struct {
	ID        uuid.UUID
	CompanyID *uuid.UUID

	Key   string
	Label string

	MainCategory  string
	SubCategories []string
	CategoryPath  []string

	// BaseFactor is numeric(18,8) — eight decimal places, which is why it
	// cannot be a float: emission factors are small numbers multiplied by
	// large quantities, and the rounding shows up in the reported total.
	BaseFactor decimal.Decimal
	BaseUnit   string

	FuelType    *string
	VehicleType *string
	Scope       *CarbonScope
	IsoCategory *string
	Status      *string

	Source     *string
	SourceYear *int16
	SourceURL  *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// EmissionFactorConversion converts a factor's base unit into another unit.
// Mirrors table `emission_factor_conversions` (migration 00007), primary key
// (factor_id, unit).
type EmissionFactorConversion struct {
	FactorID   uuid.UUID
	Unit       string
	Multiplier decimal.Decimal
	Label      string
}

// CarbonSelectedActivity records that a building tracks a given activity.
// Mirrors table `carbon_selected_activities` (migration 00007), primary key
// (building_id, activity_key).
type CarbonSelectedActivity struct {
	CompanyID   uuid.UUID
	BuildingID  uuid.UUID
	ActivityKey string
}

// CarbonActivity is one recorded emitting activity over a period. Mirrors
// table `carbon_activities` (migration 00007).
//
// FactorKey, FactorValue and ConversionMultiplier duplicate what the referenced
// EmissionFactor said AT THE TIME. That is deliberate: the catalogue is
// updated as new national figures are published, and a restated factor must
// not silently change an emission figure already reported.
//
// IsAutomated marks a row derived from meter data rather than entered by hand;
// the schema permits only one automated row per (building, activity type, day).
type CarbonActivity struct {
	ID         uuid.UUID
	CompanyID  uuid.UUID
	BuildingID uuid.UUID

	MainCategory string
	SubCategory  string
	ActivityType string

	// PeriodStart and PeriodEnd are `date` columns.
	PeriodStart time.Time
	PeriodEnd   time.Time

	Quantity decimal.Decimal
	Unit     string

	FactorID             *uuid.UUID
	FactorKey            *string
	FactorValue          *decimal.Decimal
	ConversionMultiplier decimal.Decimal

	// EmissionKgco2e is the computed result, stored rather than recomputed on
	// read for the same reason the factor is copied.
	EmissionKgco2e decimal.Decimal

	Scope       CarbonScope
	IsoCategory string
	Description *string
	Details     json.RawMessage

	Status      CarbonStatus
	IsAutomated bool

	CreatedBy *uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CarbonReport is a generated GHG or ISO carbon report. Mirrors table
// `carbon_reports` (migration 00007).
type CarbonReport struct {
	ID         uuid.UUID
	CompanyID  uuid.UUID
	BuildingID uuid.UUID

	Name string
	// ReportType is 'ghg' or 'iso', enforced by a check constraint rather
	// than a SQL enum, so it is a string here.
	ReportType string
	Period     string

	Payload json.RawMessage
	PdfPath *string

	CreatedAt time.Time
}

// ISO50001Project is a building's ISO 50001 programme. Mirrors table
// `iso50001_projects` (migration 00007); building_id is unique, so a building
// has at most one.
type ISO50001Project struct {
	ID         uuid.UUID
	CompanyID  uuid.UUID
	BuildingID uuid.UUID

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ISO50001ClauseDate is the planned window for one top-level clause of a
// project. Mirrors table `iso50001_clause_dates` (migration 00007), primary key
// (project_id, clause_id).
type ISO50001ClauseDate struct {
	ProjectID uuid.UUID
	// ClauseID is a top-level clause number: '5', '6', '7', '8', '9'.
	ClauseID string

	// StartDate and EndDate are `date` columns, both nullable; the schema
	// requires StartDate <= EndDate whenever both are set.
	StartDate *time.Time
	EndDate   *time.Time
}

// ISO50001Note is one note filed against a clause. Mirrors table
// `iso50001_notes` (migration 00007).
type ISO50001Note struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	// ClauseID here is a sub-clause: '5.1', '6.3', …
	ClauseID string

	Title *string
	Body  string

	CreatedBy *uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}
