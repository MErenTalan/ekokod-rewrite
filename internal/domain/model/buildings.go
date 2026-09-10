package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Building is the unit almost everything in the product is grouped by, and the
// unit store.Scope's BuildingIDs restricts to. Mirrors table `buildings`
// (migration 00003).
type Building struct {
	ID        uuid.UUID
	CompanyID uuid.UUID

	Name    string
	Address *string
	// Latitude and Longitude are numeric(9,6). They are decimal rather than
	// float because they are stored and compared exactly; six decimal places
	// is roughly 0.1 m, and a float round-trip would not preserve it.
	Latitude  *decimal.Decimal
	Longitude *decimal.Decimal

	Floors         *int32
	PersonnelCount *int32
	TotalAreaM2    *decimal.Decimal
	Sector         *string

	ResponsibleUserID *uuid.UUID

	// BillCutoffDay is the day of month the billing period closes on, 1..31,
	// checked by the schema. It is smallint, hence int16.
	BillCutoffDay int16

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// BuildingContact is one named contact for a building. Mirrors table
// `building_contacts` (migration 00003).
type BuildingContact struct {
	ID         uuid.UUID
	BuildingID uuid.UUID

	Name      *string
	Phone     *string
	SortOrder int16
}

// Analyzer is a metering point: one installation number at one provider, from
// which readings are ingested. Mirrors table `analyzers` (migration 00003).
//
// BuildingID is nullable. An analyzer that has been created but not yet
// attached to a building is a real state in this product, and it is why a
// scope-filtered analyzer query cannot simply join through buildings.
//
// `analyzers (provider, provider_subtype, installation_number)` is unique
// among live rows, which is the constraint two fixtures in the same database
// have to avoid colliding on.
type Analyzer struct {
	ID         uuid.UUID
	CompanyID  uuid.UUID
	BuildingID *uuid.UUID

	Provider        IntegrationProvider
	ProviderSubtype string
	// InstallationNumber is the tesisat/wiring/subscription number the
	// provider knows this metering point by.
	InstallationNumber string

	CustomerName  *string
	Address       *string
	Province      *string
	District      *string
	Neighbourhood *string
	Street        *string

	TariffType       *string
	TariffKind       *string
	InstallationKind *string

	// InstalledPowerKw is kurulu güç, numeric(12,3).
	InstalledPowerKw *decimal.Decimal

	MeterNumber *string
	MeterModel  *string
	// MeterMultiplier is numeric(12,6), not null, default 1. It multiplies
	// raw register values exactly once, at ingestion; the multiplier actually
	// applied is recorded on each MeterReading.
	MeterMultiplier decimal.Decimal

	CounterpartyNo    *string
	MeteringPointName *string

	Latitude  *decimal.Decimal
	Longitude *decimal.Decimal
	EtsoCode  *string
	// DefinitionType is the ARIL owner type, smallint.
	DefinitionType *int16

	LastReadingAt *time.Time
	IsActive      bool

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}
