package dto

import (
	"time"

	"github.com/google/uuid"
)

// RenewableQuery is R298: exactly one subject and inclusive Istanbul days.
type RenewableQuery struct {
	AnalyzerID *uuid.UUID `query:"analyzer_id" json:"-"`
	BuildingID *uuid.UUID `query:"building_id" json:"-"`
	From       Date       `query:"from" json:"-" validate:"required" required:"true"`
	To         Date       `query:"to" json:"-" validate:"required" required:"true"`
}

// Unavailable maps a field to the closed reason it has no value (R292).
type Unavailable map[string]string

// RenewablePoint is one series value.
type RenewablePoint struct {
	Ts  time.Time `json:"ts" required:"true"`
	Kwh *Decimal  `json:"kwh" required:"true"`
}

// RenewableOverview is the overview panel.
type RenewableOverview struct {
	ActiveGenerationKwh       *Decimal    `json:"active_generation_kwh" required:"true"`
	InductiveGenerationKvarh  *Decimal    `json:"inductive_generation_kvarh" required:"true"`
	CapacitiveGenerationKvarh *Decimal    `json:"capacitive_generation_kvarh" required:"true"`
	AverageGenerationKwh      *Decimal    `json:"average_generation_kwh" required:"true"`
	Unavailable               Unavailable `json:"unavailable" required:"true"`
}

// RenewableRealtime is the real-time panel from the last complete hour.
type RenewableRealtime struct {
	CurrentPowerKw      *Decimal         `json:"current_power_kw" required:"true"`
	TodayKwh            *Decimal         `json:"today_kwh" required:"true"`
	MaxPowerKw          *Decimal         `json:"max_power_kw" required:"true"`
	AvgPowerKw          *Decimal         `json:"avg_power_kw" required:"true"`
	Status              *string          `json:"status" required:"true" enum:"producing,idle,no_data"`
	Series24h           []RenewablePoint `json:"series_24h" required:"true"`
	SystemEfficiencyPct *Decimal         `json:"system_efficiency_pct" required:"true"`
	Unavailable         Unavailable      `json:"unavailable" required:"true"`
}

// RenewableGridInteraction is the grid interaction panel.
type RenewableGridInteraction struct {
	Direction      *string     `json:"direction" required:"true"`
	TodayImportKwh *Decimal    `json:"today_import_kwh" required:"true"`
	TodayExportKwh *Decimal    `json:"today_export_kwh" required:"true"`
	PowerFactor    *Decimal    `json:"power_factor" required:"true"`
	VoltageV       *Decimal    `json:"voltage_v" required:"true"`
	FrequencyHz    *Decimal    `json:"frequency_hz" required:"true"`
	ImportPrice    *Decimal    `json:"import_price" required:"true"`
	ExportPrice    *Decimal    `json:"export_price" required:"true"`
	Currency       *string     `json:"currency" required:"true"`
	NetToday       *Decimal    `json:"net_today" required:"true"`
	Unavailable    Unavailable `json:"unavailable" required:"true"`
}

// EquivalenceFactor is one seeded equivalence's provenance (R293).
type EquivalenceFactor struct {
	Key    string  `json:"key" required:"true" enum:"equiv_tree_co2_kg_per_year,equiv_coal_kg_per_kwh,equiv_car_co2_kg_per_km,equiv_home_heating_kwh_per_year"`
	Factor Decimal `json:"factor" required:"true"`
	Unit   string  `json:"unit" required:"true"`
	Source string  `json:"source" required:"true"`
	Year   *int16  `json:"year" required:"true"`
}

// RenewableEnvironmental is R293's panel.
type RenewableEnvironmental struct {
	GenerationKwh    *Decimal            `json:"generation_kwh" required:"true"`
	Co2AvoidedKg     *Decimal            `json:"co2_avoided_kg" required:"true"`
	Trees            *Decimal            `json:"trees" required:"true"`
	CoalKg           *Decimal            `json:"coal_kg" required:"true"`
	CarKm            *Decimal            `json:"car_km" required:"true"`
	Homes            *Decimal            `json:"homes" required:"true"`
	GridFactor       *Decimal            `json:"grid_factor" required:"true"`
	GridFactorUnit   *string             `json:"grid_factor_unit" required:"true"`
	GridFactorSource *string             `json:"grid_factor_source" required:"true"`
	Factors          []EquivalenceFactor `json:"factors" required:"true"`
	Unavailable      Unavailable         `json:"unavailable" required:"true"`
}

// RenewableEfficiency is the efficiency panel; nothing in it is measurable yet.
type RenewableEfficiency struct {
	OverallPct      *Decimal         `json:"overall_pct" required:"true"`
	PanelPct        *Decimal         `json:"panel_pct" required:"true"`
	InverterPct     *Decimal         `json:"inverter_pct" required:"true"`
	BatteryPct      *Decimal         `json:"battery_pct" required:"true"`
	GridPct         *Decimal         `json:"grid_pct" required:"true"`
	Trend           []RenewablePoint `json:"trend" required:"true"`
	Recommendations []string         `json:"recommendations" required:"true"`
	Unavailable     Unavailable      `json:"unavailable" required:"true"`
}

// RenewableForecast is the forecast panel from stored forecast runs.
type RenewableForecast struct {
	ConsumptionNext24hKwh  *Decimal    `json:"consumption_next_24h_kwh" required:"true"`
	ConsumptionNext7dKwh   *Decimal    `json:"consumption_next_7d_kwh" required:"true"`
	ConsumptionNext28dKwh  *Decimal    `json:"consumption_next_28d_kwh" required:"true"`
	AccuracyDailyPct       *Decimal    `json:"accuracy_daily_pct" required:"true"`
	AccuracyWeeklyPct      *Decimal    `json:"accuracy_weekly_pct" required:"true"`
	AccuracyOverallPct     *Decimal    `json:"accuracy_overall_pct" required:"true"`
	EstimatedGenerationKwh *Decimal    `json:"estimated_generation_kwh" required:"true"`
	NetExcessKwh           *Decimal    `json:"net_excess_kwh" required:"true"`
	WeatherImpact          *string     `json:"weather_impact" required:"true"`
	Unavailable            Unavailable `json:"unavailable" required:"true"`
}

// RenewableFinancial is §7.8's financial gains inside the analytics panel.
type RenewableFinancial struct {
	ImportPrice        *Decimal    `json:"import_price" required:"true"`
	ExportPrice        *Decimal    `json:"export_price" required:"true"`
	Currency           *string     `json:"currency" required:"true"`
	TodayImportCost    *Decimal    `json:"today_import_cost" required:"true"`
	TodayExportRevenue *Decimal    `json:"today_export_revenue" required:"true"`
	NetToday           *Decimal    `json:"net_today" required:"true"`
	MonthEarnings      *Decimal    `json:"month_earnings" required:"true"`
	YearEarnings       *Decimal    `json:"year_earnings" required:"true"`
	TotalSavings       *Decimal    `json:"total_savings" required:"true"`
	RoiPct             *Decimal    `json:"roi_pct" required:"true"`
	PaybackYears       *Decimal    `json:"payback_years" required:"true"`
	BillSavings        *Decimal    `json:"bill_savings" required:"true"`
	Unavailable        Unavailable `json:"unavailable" required:"true"`
}

// RenewableAnalytics is the analytics panel.
type RenewableAnalytics struct {
	PeakGenerationKwh       *Decimal           `json:"peak_generation_kwh" required:"true"`
	PeakGenerationAt        *time.Time         `json:"peak_generation_at" required:"true"`
	AverageGenerationKwh    *Decimal           `json:"average_generation_kwh" required:"true"`
	Trend                   []RenewablePoint   `json:"trend" required:"true"`
	PeakHour                *int               `json:"peak_hour" required:"true"`
	DataAvailabilityPct     *Decimal           `json:"data_availability_pct" required:"true"`
	SystemEfficiencyPct     *Decimal           `json:"system_efficiency_pct" required:"true"`
	EfficiencyChange30dPct  *Decimal           `json:"efficiency_change_30d_pct" required:"true"`
	ConsumptionOptimisation *string            `json:"consumption_optimisation" required:"true"`
	MaintenanceRequired     *string            `json:"maintenance_required" required:"true"`
	Financial               RenewableFinancial `json:"financial" required:"true"`
	Unavailable             Unavailable        `json:"unavailable" required:"true"`
}

// RenewableSystemStatus is the system status panel.
type RenewableSystemStatus struct {
	Overall              *string     `json:"overall" required:"true"`
	Monitoring           *string     `json:"monitoring" required:"true"`
	GridConnection       *string     `json:"grid_connection" required:"true"`
	LastReadingAt        *time.Time  `json:"last_reading_at" required:"true"`
	SolarPanels          *string     `json:"solar_panels" required:"true"`
	Inverter             *string     `json:"inverter" required:"true"`
	Battery              *string     `json:"battery" required:"true"`
	Security             *string     `json:"security" required:"true"`
	TotalGenerationKwh   *Decimal    `json:"total_generation_kwh" required:"true"`
	AverageEfficiencyPct *Decimal    `json:"average_efficiency_pct" required:"true"`
	Unavailable          Unavailable `json:"unavailable" required:"true"`
}

// FinancialSummaryRequest is R294's summary: year required, month optional.
type FinancialSummaryRequest struct {
	Year  int  `query:"year" json:"-" validate:"required,gte=2000,lte=2100" required:"true" minimum:"2000" maximum:"2100"`
	Month *int `query:"month" json:"-" validate:"omitempty,gte=1,lte=12" minimum:"1" maximum:"12"`
}

// FinancialMonthlyRequest is the monthly table's year.
type FinancialMonthlyRequest struct {
	Year int `query:"year" json:"-" validate:"required,gte=2000,lte=2100" required:"true" minimum:"2000" maximum:"2100"`
}

// FinancialMonth is one month (0 = the year row); null means no data.
type FinancialMonth struct {
	Month           int           `json:"month" required:"true"`
	ConsumptionKwh  *Decimal      `json:"consumption_kwh" required:"true"`
	Cost            []MoneyAmount `json:"cost" required:"true"`
	ProductionKwh   *Decimal      `json:"production_kwh" required:"true"`
	Revenue         []MoneyAmount `json:"revenue" required:"true"`
	RevenuePartial  bool          `json:"revenue_partial" required:"true"`
	OffsetKwh       *Decimal      `json:"offset_kwh" required:"true"`
	GridPurchaseKwh *Decimal      `json:"grid_purchase_kwh" required:"true"`
	GridSaleKwh     *Decimal      `json:"grid_sale_kwh" required:"true"`
	Net             []MoneyAmount `json:"net" required:"true"`
}

// FinancialCoverage is R258's with_data/of for the year row.
type FinancialCoverage struct {
	ConsumptionMonths int `json:"consumption_months" required:"true"`
	ProductionMonths  int `json:"production_months" required:"true"`
	Of                int `json:"of" required:"true"`
}

// FinancialMonthly is the twelve months and the year row.
type FinancialMonthly struct {
	Items    []FinancialMonth  `json:"items" required:"true"`
	Total    FinancialMonth    `json:"total" required:"true"`
	Coverage FinancialCoverage `json:"coverage" required:"true"`
}

// FinancialBuildingTariff is a building's purchase tariff in force today.
type FinancialBuildingTariff struct {
	BuildingID   uuid.UUID `json:"building_id" required:"true"`
	BuildingName string    `json:"building_name" required:"true"`
	PriceType    string    `json:"price_type" required:"true" enum:"single_time,multi_time"`
	Single       *Decimal  `json:"single" required:"true"`
	T1           *Decimal  `json:"t1" required:"true"`
	T2           *Decimal  `json:"t2" required:"true"`
	T3           *Decimal  `json:"t3" required:"true"`
	Currency     string    `json:"currency" required:"true"`
}

// FinancialPlantFeedIn is a plant's feed-in price in force today.
type FinancialPlantFeedIn struct {
	PlantID   uuid.UUID `json:"plant_id" required:"true"`
	PlantName string    `json:"plant_name" required:"true"`
	Price     Decimal   `json:"price" required:"true"`
	Currency  string    `json:"currency" required:"true"`
}

// FinancialTariffs is the tariff panel.
type FinancialTariffs struct {
	Purchase        []FinancialBuildingTariff `json:"purchase" required:"true"`
	Sale            []FinancialPlantFeedIn    `json:"sale" required:"true"`
	PurchaseMissing bool                      `json:"purchase_missing" required:"true"`
	SaleMissing     bool                      `json:"sale_missing" required:"true"`
}

// FinancialSummary is R294's headline.
type FinancialSummary struct {
	Year          int              `json:"year" required:"true"`
	Month         *int             `json:"month" required:"true"`
	AnalyzerCount int              `json:"analyzer_count" required:"true"`
	PlantCount    int              `json:"plant_count" required:"true"`
	Figures       FinancialMonth   `json:"figures" required:"true"`
	WithData      int              `json:"with_data" required:"true"`
	Of            int              `json:"of" required:"true"`
	Tariffs       FinancialTariffs `json:"tariffs" required:"true"`
}
