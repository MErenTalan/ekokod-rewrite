package dto

import (
	"time"

	"github.com/google/uuid"
)

// Building is a building, with the list includes when requested (R163).
type Building struct {
	ID                  uuid.UUID  `json:"id" required:"true"`
	Name                string     `json:"name" required:"true"`
	Address             *string    `json:"address"`
	Latitude            *Decimal   `json:"latitude"`
	Longitude           *Decimal   `json:"longitude"`
	Floors              *int32     `json:"floors"`
	PersonnelCount      *int32     `json:"personnel_count"`
	TotalAreaM2         *Decimal   `json:"total_area_m2"`
	Sector              *string    `json:"sector"`
	ResponsibleUserID   *uuid.UUID `json:"responsible_user_id"`
	BillCutoffDay       int16      `json:"bill_cutoff_day" required:"true"`
	CreatedAt           time.Time  `json:"created_at" required:"true"`
	UpdatedAt           time.Time  `json:"updated_at" required:"true"`
	AnalyzerCount       *int       `json:"analyzer_count,omitempty"`
	ActiveAnalyzerCount *int       `json:"active_analyzer_count,omitempty"`
	ActivityStatus      *string    `json:"activity_status,omitempty" enum:"active,passive"`
}

// Contact is a building contact.
type Contact struct {
	Name  *string `json:"name" validate:"omitempty,max=200"`
	Phone *string `json:"phone" validate:"omitempty,max=40"`
}

// TariffSummary is one tariff version in a building's history.
type TariffSummary struct {
	ID            uuid.UUID `json:"id" required:"true"`
	Name          *string   `json:"name"`
	EffectiveFrom Date      `json:"effective_from" required:"true"`
}

// BuildingDetail is GET /buildings/{id}.
type BuildingDetail struct {
	Building
	Contacts      []Contact       `json:"contacts" required:"true"`
	TariffHistory []TariffSummary `json:"tariff_history" required:"true"`
}

// BuildingListRequest is GET /buildings.
type BuildingListRequest struct {
	Include []string `query:"include" json:"-" enum:"analyzer_count,active_status"`
	Q       string   `query:"q" json:"-"`
	Sector  string   `query:"sector" json:"-"`
	PageRequest
}

// BuildingFields are the writable building fields.
type BuildingFields struct {
	Address           *string    `json:"address,omitempty" validate:"omitempty,max=500"`
	Latitude          *Decimal   `json:"latitude,omitempty"`
	Longitude         *Decimal   `json:"longitude,omitempty"`
	Floors            *int32     `json:"floors,omitempty" validate:"omitempty,min=0,max=300"`
	PersonnelCount    *int32     `json:"personnel_count,omitempty" validate:"omitempty,min=0"`
	TotalAreaM2       *Decimal   `json:"total_area_m2,omitempty"`
	Sector            *string    `json:"sector,omitempty" validate:"omitempty,max=100"`
	ResponsibleUserID *uuid.UUID `json:"responsible_user_id,omitempty"`
	ClearResponsible  bool       `json:"clear_responsible_user,omitempty"`
	BillCutoffDay     *int16     `json:"bill_cutoff_day,omitempty" validate:"omitempty,min=1,max=31"`
	Contacts          *[]Contact `json:"contacts,omitempty" validate:"omitempty,max=20,dive"`
}

// BuildingCreateRequest is POST /buildings.
type BuildingCreateRequest struct {
	Name string `json:"name" validate:"required,max=200" required:"true"`
	BuildingFields
}

// BuildingUpdateRequest is PATCH /buildings/{id}.
type BuildingUpdateRequest struct {
	ID   uuid.UUID `path:"id" json:"-"`
	Name *string   `json:"name,omitempty" validate:"omitempty,min=1,max=200"`
	BuildingFields
}

// Analyzer is a metering point.
type Analyzer struct {
	ID                 uuid.UUID  `json:"id" required:"true"`
	BuildingID         *uuid.UUID `json:"building_id"`
	Provider           string     `json:"provider" required:"true" enum:"osos,gridbox,aril,pm5340,isolar"`
	ProviderSubtype    string     `json:"provider_subtype" required:"true"`
	InstallationNumber string     `json:"installation_number" required:"true"`
	CustomerName       *string    `json:"customer_name"`
	Address            *string    `json:"address"`
	Province           *string    `json:"province"`
	District           *string    `json:"district"`
	TariffType         *string    `json:"tariff_type"`
	InstalledPowerKw   *Decimal   `json:"installed_power_kw"`
	MeterNumber        *string    `json:"meter_number"`
	MeterModel         *string    `json:"meter_model"`
	MeterMultiplier    Decimal    `json:"meter_multiplier" required:"true"`
	Latitude           *Decimal   `json:"latitude"`
	Longitude          *Decimal   `json:"longitude"`
	LastReadingAt      *time.Time `json:"last_reading_at"`
	IsActive           bool       `json:"is_active" required:"true"`
	ActivityStatus     string     `json:"activity_status" required:"true" enum:"active,passive"`
}

// AnalyzerListRequest is GET /analyzers.
type AnalyzerListRequest struct {
	BuildingID *uuid.UUID `query:"building_id" json:"-"`
	Unassigned bool       `query:"unassigned" json:"-"`
	Provider   []string   `query:"provider" json:"-"`
	IsActive   *bool      `query:"is_active" json:"-"`
	Q          string     `query:"q" json:"-"`
	PageRequest
}

// AnalyzerUpdateRequest is PATCH /analyzers/{id}: only the assignable fields.
type AnalyzerUpdateRequest struct {
	ID               uuid.UUID  `path:"id" json:"-"`
	BuildingID       *uuid.UUID `json:"building_id,omitempty"`
	UnassignBuilding bool       `json:"unassign_building,omitempty"`
	MeterMultiplier  *Decimal   `json:"meter_multiplier,omitempty"`
	InstalledPowerKw *Decimal   `json:"installed_power_kw,omitempty"`
	Latitude         *Decimal   `json:"latitude,omitempty"`
	Longitude        *Decimal   `json:"longitude,omitempty"`
}

// AnalyzerRefreshRequest is POST /analyzers/{id}/refresh.
type AnalyzerRefreshRequest struct {
	ID   uuid.UUID `path:"id" json:"-"`
	Mode string    `json:"mode" validate:"required,oneof=hourly energy" required:"true" enum:"hourly,energy"`
}

// Plant is a power plant.
type Plant struct {
	ID                 uuid.UUID  `json:"id" required:"true"`
	Name               string     `json:"name" required:"true"`
	InstallationNumber *string    `json:"installation_number"`
	PlantKind          string     `json:"plant_kind" required:"true" enum:"rooftop,grid"`
	PvBrandModel       *string    `json:"pv_brand_model"`
	PanelPowerW        *Decimal   `json:"panel_power_w"`
	PanelEfficiencyPct *Decimal   `json:"panel_efficiency_pct"`
	PanelCount         *int32     `json:"panel_count"`
	StringCount        *int32     `json:"string_count"`
	Orientation        *string    `json:"orientation" enum:"n,s,e,w,ne,se,nw,sw"`
	TiltAngleDeg       *Decimal   `json:"tilt_angle_deg"`
	TotalCapacityKw    *Decimal   `json:"total_capacity_kw"`
	YearlyTargetKwh    *Decimal   `json:"yearly_target_kwh"`
	InstallationDate   *Date      `json:"installation_date"`
	Address            *string    `json:"address"`
	Latitude           *Decimal   `json:"latitude"`
	Longitude          *Decimal   `json:"longitude"`
	IsolarPsName       *string    `json:"isolar_ps_name"`
	IsolarLinkedAt     *time.Time `json:"isolar_linked_at"`
	CreatedAt          time.Time  `json:"created_at" required:"true"`
}

// PlantDevice is an inverter, meter or weather station.
type PlantDevice struct {
	ID            uuid.UUID  `json:"id" required:"true"`
	DeviceSN      string     `json:"device_sn" required:"true"`
	DeviceName    *string    `json:"device_name"`
	Brand         *string    `json:"brand"`
	Model         *string    `json:"model"`
	RatedPowerKw  *Decimal   `json:"rated_power_kw"`
	Status        *string    `json:"status"`
	EfficiencyPct *Decimal   `json:"efficiency_pct"`
	LastSeenAt    *time.Time `json:"last_seen_at"`
}

// PlantDetail is GET /power-plants/{id}.
type PlantDetail struct {
	Plant
	MonthlyTargets  []Decimal     `json:"monthly_targets" required:"true"`
	Devices         []PlantDevice `json:"devices" required:"true"`
	AlarmRecipients []string      `json:"alarm_recipients" required:"true"`
}

// PlantFields are the writable plant fields.
type PlantFields struct {
	InstallationNumber *string    `json:"installation_number,omitempty" validate:"omitempty,max=100"`
	PlantKind          *string    `json:"plant_kind,omitempty" validate:"omitempty,oneof=rooftop grid" enum:"rooftop,grid"`
	PvBrandModel       *string    `json:"pv_brand_model,omitempty" validate:"omitempty,max=200"`
	PanelPowerW        *Decimal   `json:"panel_power_w,omitempty"`
	PanelEfficiencyPct *Decimal   `json:"panel_efficiency_pct,omitempty"`
	PanelCount         *int32     `json:"panel_count,omitempty" validate:"omitempty,min=0"`
	StringCount        *int32     `json:"string_count,omitempty" validate:"omitempty,min=0"`
	Orientation        *string    `json:"orientation,omitempty" validate:"omitempty,oneof=n s e w ne se nw sw" enum:"n,s,e,w,ne,se,nw,sw"`
	TiltAngleDeg       *Decimal   `json:"tilt_angle_deg,omitempty"`
	TotalCapacityKw    *Decimal   `json:"total_capacity_kw,omitempty"`
	YearlyTargetKwh    *Decimal   `json:"yearly_target_kwh,omitempty"`
	InstallationDate   *Date      `json:"installation_date,omitempty"`
	Address            *string    `json:"address,omitempty" validate:"omitempty,max=500"`
	Latitude           *Decimal   `json:"latitude,omitempty"`
	Longitude          *Decimal   `json:"longitude,omitempty"`
	MonthlyTargets     *[]Decimal `json:"monthly_targets,omitempty" validate:"omitempty,len=12"`
	AlarmRecipients    *[]string  `json:"alarm_recipients,omitempty" validate:"omitempty,max=50,dive,email"`
}

// PlantCreateRequest is POST /power-plants.
type PlantCreateRequest struct {
	Name string `json:"name" validate:"required,max=200" required:"true"`
	PlantFields
}

// PlantUpdateRequest is PATCH /power-plants/{id}.
type PlantUpdateRequest struct {
	ID   uuid.UUID `path:"id" json:"-"`
	Name *string   `json:"name,omitempty" validate:"omitempty,min=1,max=200"`
	PlantFields
}

// ComparisonMetric is one figure of the sectoral comparison: the building's
// value, the sector average and the ascending rank (0 when unranked).
type ComparisonMetric struct {
	Value   *Decimal `json:"value"`
	Average *Decimal `json:"average"`
	Rank    int      `json:"rank" required:"true"`
	Ranked  int      `json:"ranked" required:"true"`
}

// BuildingComparison is GET /buildings/{id}/comparison (R162): no peer identity.
type BuildingComparison struct {
	Available            bool             `json:"available" required:"true"`
	Reason               *string          `json:"reason" enum:"sector_too_small"`
	Sector               string           `json:"sector" required:"true"`
	Peers                int              `json:"peers" required:"true"`
	DailyConsumption     ComparisonMetric `json:"daily_consumption" required:"true"`
	MonthlyConsumption   ComparisonMetric `json:"monthly_consumption" required:"true"`
	CO2EmissionKg        ComparisonMetric `json:"co2_emission_kg" required:"true"`
	ConsumptionPerCapita ComparisonMetric `json:"consumption_per_capita" required:"true"`
	ConsumptionPerArea   ComparisonMetric `json:"consumption_per_area" required:"true"`
}
