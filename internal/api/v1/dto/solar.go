package dto

import (
	"time"

	"github.com/google/uuid"
)

// PlantRealtime is GET /plants/{id}/realtime (R282).
type PlantRealtime struct {
	AsOf                   *time.Time `json:"as_of"`
	Stale                  bool       `json:"stale" required:"true"`
	InverterCount          int        `json:"inverter_count" required:"true"`
	ActivePowerKw          *Decimal   `json:"active_power_kw"`
	YieldTodayKwh          *Decimal   `json:"yield_today_kwh"`
	YieldMonthKwh          *Decimal   `json:"yield_month_kwh"`
	YieldYearKwh           *Decimal   `json:"yield_year_kwh"`
	YieldTotalKwh          *Decimal   `json:"yield_total_kwh"`
	CapacityKw             *Decimal   `json:"capacity_kw"`
	CapacityUtilisationPct *Decimal   `json:"capacity_utilisation_pct"`
	Connection             string     `json:"connection" required:"true" enum:"connected,error,never_synced"`
	ConnectionError        *string    `json:"connection_error"`
	LastSyncAt             *time.Time `json:"last_sync_at"`
}

// PlantProductionRequest is GET /plants/{id}/production (R284).
type PlantProductionRequest struct {
	ID          uuid.UUID `path:"id" json:"-"`
	Granularity string    `query:"granularity" json:"-" validate:"required,oneof=hour day month" required:"true" enum:"hour,day,month"`
	From        Date      `query:"from" json:"-" validate:"required" required:"true"`
	To          Date      `query:"to" json:"-" validate:"required" required:"true"`
}

// PlantProductionPoint is one series value; basis is null for monthly points.
type PlantProductionPoint struct {
	Ts            time.Time `json:"ts" required:"true"`
	ProductionKwh *Decimal  `json:"production_kwh"`
	Basis         *string   `json:"basis" enum:"plant_meter,inverter_sum,daily_total"`
}

// PlantProductionSeries is the production series.
type PlantProductionSeries struct {
	Granularity string                 `json:"granularity" required:"true" enum:"hour,day,month"`
	Points      []PlantProductionPoint `json:"points" required:"true"`
	MixedBasis  bool                   `json:"mixed_basis" required:"true"`
}

// PlantDevicesRequest is GET /plants/{id}/devices.
type PlantDevicesRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	Q  string    `query:"q" json:"-" validate:"omitempty,max=100"`
}

// PlantDeviceView is one row of the devices tab (R285).
type PlantDeviceView struct {
	ID            uuid.UUID  `json:"id" required:"true"`
	DeviceSN      string     `json:"device_sn" required:"true"`
	DeviceName    *string    `json:"device_name"`
	DeviceType    *int32     `json:"device_type"`
	Status        *string    `json:"status" enum:"normal,alarm,fault,offline"`
	ActivePowerKw *Decimal   `json:"active_power_kw"`
	YieldTodayKwh *Decimal   `json:"yield_today_kwh"`
	YieldTotalKwh *Decimal   `json:"yield_total_kwh"`
	LastUpdate    *time.Time `json:"last_update"`
}

// PlantDevices is the devices list.
type PlantDevices struct {
	Items []PlantDeviceView `json:"items" required:"true"`
}

// PlantAlarmsRequest pages a plant's faults.
type PlantAlarmsRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	PageRequest
}

// PlantFault is one iSolar fault, translated (R286).
type PlantFault struct {
	Ref        string     `json:"ref" required:"true"`
	Code       string     `json:"code" required:"true"`
	Name       string     `json:"name" required:"true"`
	MessageTr  string     `json:"message_tr" required:"true"`
	Translated bool       `json:"translated" required:"true"`
	Level      *int32     `json:"level"`
	Type       *int32     `json:"type"`
	DeviceName *string    `json:"device_name"`
	OccurredAt time.Time  `json:"occurred_at" required:"true"`
	ClosedAt   *time.Time `json:"closed_at"`
}

// PlantFaultPage is the alarms tab's page.
type PlantFaultPage struct {
	Items      []PlantFault `json:"items" required:"true"`
	NextCursor *string      `json:"next_cursor"`
	Total      int          `json:"total" required:"true"`
}

// MoneyAmount is one currency's amount (R253: currencies never add).
type MoneyAmount struct {
	Currency string  `json:"currency" required:"true" enum:"TRY,USD,EUR"`
	Amount   Decimal `json:"amount" required:"true"`
}

// RevenuePeriod is one revenue card (R283).
type RevenuePeriod struct {
	Amounts      []MoneyAmount `json:"amounts" required:"true"`
	UnpricedDays int           `json:"unpriced_days" required:"true"`
	Partial      bool          `json:"partial" required:"true"`
	Since        *Date         `json:"since"`
}

// PlantRevenue is GET /plants/{id}/revenue.
type PlantRevenue struct {
	Available bool           `json:"available" required:"true"`
	Reason    string         `json:"reason,omitempty" enum:"no_solar_tariff"`
	Daily     *RevenuePeriod `json:"daily,omitempty"`
	Monthly   *RevenuePeriod `json:"monthly,omitempty"`
	Yearly    *RevenuePeriod `json:"yearly,omitempty"`
	Total     *RevenuePeriod `json:"total,omitempty"`
}

// ISolarPlantsRequest is GET /integrations/isolar/plants.
type ISolarPlantsRequest struct {
	CredentialID uuid.UUID `query:"credential_id" json:"-" validate:"required" required:"true"`
}

// ISolarAccountPlant is one plant on the connected account.
type ISolarAccountPlant struct {
	PSID          string     `json:"ps_id" required:"true"`
	Name          string     `json:"name" required:"true"`
	InstalledKw   *Decimal   `json:"installed_kw"`
	LinkedPlantID *uuid.UUID `json:"linked_plant_id"`
}

// ISolarAccountPlants is the link modal's list.
type ISolarAccountPlants struct {
	Items []ISolarAccountPlant `json:"items" required:"true"`
}

// ISolarLinkRequest is POST /plants/{id}/isolar-link (R280, R281).
type ISolarLinkRequest struct {
	ID           uuid.UUID `path:"id" json:"-"`
	CredentialID uuid.UUID `json:"credential_id" validate:"required" required:"true"`
	PSID         string    `json:"ps_id" validate:"required,max=64" required:"true"`
}

// ISolarLinked is the linked plant and its backfill job.
type ISolarLinked struct {
	Plant PlantDetail `json:"plant" required:"true"`
	JobID string      `json:"job_id" required:"true"`
}

// AlarmRecipientsRequest is PUT /plants/{id}/alarm-recipients (R298).
type AlarmRecipientsRequest struct {
	ID     uuid.UUID `path:"id" json:"-"`
	Emails []string  `json:"emails" validate:"required,max=50" required:"true"`
}

// AlarmRecipients is the stored list.
type AlarmRecipients struct {
	Emails []string `json:"emails" required:"true"`
}

// WeatherRequest is GET /weather: exactly one of plant_id and building_id (R291).
type WeatherRequest struct {
	PlantID    *uuid.UUID `query:"plant_id" json:"-"`
	BuildingID *uuid.UUID `query:"building_id" json:"-"`
}

// WeatherCurrent is now; wind km/h, pressure hPa, visibility km.
type WeatherCurrent struct {
	TemperatureC     *Decimal `json:"temperature_c"`
	HumidityPct      *Decimal `json:"humidity_pct"`
	WindKmh          *Decimal `json:"wind_kmh"`
	PressureHpa      *Decimal `json:"pressure_hpa"`
	VisibilityKm     *Decimal `json:"visibility_km"`
	UVIndex          *Decimal `json:"uv_index"`
	PrecipitationPct *Decimal `json:"precipitation_pct"`
	WeatherCode      *int32   `json:"weather_code"`
}

// WeatherDay is one forecast day with its generation potential.
type WeatherDay struct {
	Date             Date     `json:"date" required:"true"`
	MinC             *Decimal `json:"min_c"`
	MaxC             *Decimal `json:"max_c"`
	WeatherCode      *int32   `json:"weather_code"`
	PrecipitationPct *Decimal `json:"precipitation_pct"`
	UVIndexMax       *Decimal `json:"uv_index_max"`
	ShortwaveMJ      *Decimal `json:"shortwave_mj_m2"`
	Potential        *string  `json:"potential" enum:"high,medium,low"`
}

// Weather is GET /weather; it never carries a location name.
type Weather struct {
	Available      bool            `json:"available" required:"true"`
	Reason         string          `json:"reason,omitempty" enum:"weather_not_configured,location_not_configured,weather_unavailable"`
	Current        *WeatherCurrent `json:"current,omitempty"`
	Days           []WeatherDay    `json:"days" required:"true"`
	PotentialBasis string          `json:"potential_basis,omitempty"`
}
