package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PowerPlant is a solar installation. Mirrors table `power_plants`
// (migration 00003).
//
// A plant belongs to a company, not to a building: the schema has no
// building_id here, so a scope that names buildings cannot narrow plants any
// further than the company. Repositories must not invent a join to make one.
type PowerPlant struct {
	ID        uuid.UUID
	CompanyID uuid.UUID

	Name               string
	InstallationNumber *string
	// PlantKind is 'rooftop' or 'grid'. It is a plain text column with a
	// default rather than a SQL enum, so it is a string here: inventing an
	// enum type the database does not have would let a value round-trip
	// through the model and be rejected on write.
	PlantKind string

	PvBrandModel *string
	// PanelPowerW is numeric(10,2) watts per panel.
	PanelPowerW        *decimal.Decimal
	PanelEfficiencyPct *decimal.Decimal
	PanelCount         *int32
	StringCount        *int32
	Orientation        *PanelOrientation
	TiltAngleDeg       *decimal.Decimal

	// TotalCapacityKw and YearlyTargetKwh are the plant's headline figures;
	// both feed performance-ratio arithmetic and are decimal for it.
	TotalCapacityKw *decimal.Decimal
	YearlyTargetKwh *decimal.Decimal

	// InstallationDate is a `date` column: the time part is meaningless and
	// callers must not read it.
	InstallationDate *time.Time

	Address   *string
	Latitude  *decimal.Decimal
	Longitude *decimal.Decimal

	// The iSolarCloud link. IsolarPsID is unique across the table wherever it
	// is set, so two plants may not point at the same iSolar station.
	IsolarPsID        *string
	IsolarPsKey       *string
	IsolarPsName      *string
	IsolarInstalledKw *decimal.Decimal
	IsolarLinkedAt    *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// PlantMonthlyTarget is one month's production target for a plant. Mirrors
// table `power_plant_monthly_targets` (migration 00003), whose primary key is
// (plant_id, month).
type PlantMonthlyTarget struct {
	PlantID uuid.UUID
	// Month is 1..12, checked by the schema.
	Month     int16
	TargetKwh decimal.Decimal
}

// PlantDevice is one inverter or logger under a plant. Mirrors table
// `power_plant_devices` (migration 00003); (plant_id, device_sn) is unique.
type PlantDevice struct {
	ID      uuid.UUID
	PlantID uuid.UUID

	DeviceSN       string
	DeviceName     *string
	DeviceType     *int32
	DeviceTypeName *string
	// ProviderKey is the iSolar ps_key for this device.
	ProviderKey *string

	Brand        *string
	Model        *string
	RatedPowerKw *decimal.Decimal
	Status       *string
	// EfficiencyPct is numeric(6,3) percent, not a ratio.
	EfficiencyPct *decimal.Decimal

	LastSeenAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// PlantAlarmRecipient is one email address notified about a plant's alarms.
// Mirrors table `power_plant_alarm_recipients` (migration 00003), whose
// primary key is (plant_id, email); email is citext.
type PlantAlarmRecipient struct {
	PlantID uuid.UUID
	Email   string
}
