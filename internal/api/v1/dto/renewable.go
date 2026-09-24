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
	Kwh *Decimal  `json:"kwh"`
}

// RenewableOverview is the overview panel.
type RenewableOverview struct {
	ActiveGenerationKwh       *Decimal    `json:"active_generation_kwh"`
	InductiveGenerationKvarh  *Decimal    `json:"inductive_generation_kvarh"`
	CapacitiveGenerationKvarh *Decimal    `json:"capacitive_generation_kvarh"`
	AverageGenerationKwh      *Decimal    `json:"average_generation_kwh"`
	Unavailable               Unavailable `json:"unavailable" required:"true"`
}

// RenewableRealtime is the real-time panel from the last complete hour.
type RenewableRealtime struct {
	CurrentPowerKw      *Decimal         `json:"current_power_kw"`
	TodayKwh            *Decimal         `json:"today_kwh"`
	MaxPowerKw          *Decimal         `json:"max_power_kw"`
	AvgPowerKw          *Decimal         `json:"avg_power_kw"`
	Status              *string          `json:"status" enum:"producing,idle,no_data"`
	Series24h           []RenewablePoint `json:"series_24h" required:"true"`
	SystemEfficiencyPct *Decimal         `json:"system_efficiency_pct"`
	Unavailable         Unavailable      `json:"unavailable" required:"true"`
}

// RenewableGridInteraction is the grid interaction panel.
type RenewableGridInteraction struct {
	Direction      *string     `json:"direction"`
	TodayImportKwh *Decimal    `json:"today_import_kwh"`
	TodayExportKwh *Decimal    `json:"today_export_kwh"`
	PowerFactor    *Decimal    `json:"power_factor"`
	VoltageV       *Decimal    `json:"voltage_v"`
	FrequencyHz    *Decimal    `json:"frequency_hz"`
	ImportPrice    *Decimal    `json:"import_price"`
	ExportPrice    *Decimal    `json:"export_price"`
	Currency       *string     `json:"currency"`
	NetToday       *Decimal    `json:"net_today"`
	Unavailable    Unavailable `json:"unavailable" required:"true"`
}

// EquivalenceFactor is one seeded equivalence's provenance (R293).
type EquivalenceFactor struct {
	Key    string  `json:"key" required:"true" enum:"equiv_tree_co2_kg_per_year,equiv_coal_kg_per_kwh,equiv_car_co2_kg_per_km,equiv_home_heating_kwh_per_year"`
	Factor Decimal `json:"factor" required:"true"`
	Unit   string  `json:"unit" required:"true"`
	Source string  `json:"source" required:"true"`
	Year   *int16  `json:"year"`
}

// RenewableEnvironmental is R293's panel.
type RenewableEnvironmental struct {
	GenerationKwh    *Decimal            `json:"generation_kwh"`
	Co2AvoidedKg     *Decimal            `json:"co2_avoided_kg"`
	Trees            *Decimal            `json:"trees"`
	CoalKg           *Decimal            `json:"coal_kg"`
	CarKm            *Decimal            `json:"car_km"`
	Homes            *Decimal            `json:"homes"`
	GridFactor       *Decimal            `json:"grid_factor"`
	GridFactorUnit   *string             `json:"grid_factor_unit"`
	GridFactorSource *string             `json:"grid_factor_source"`
	Factors          []EquivalenceFactor `json:"factors" required:"true"`
	Unavailable      Unavailable         `json:"unavailable" required:"true"`
}

// RenewableEfficiency is the efficiency panel; nothing in it is measurable yet.
type RenewableEfficiency struct {
	OverallPct      *Decimal         `json:"overall_pct"`
	PanelPct        *Decimal         `json:"panel_pct"`
	InverterPct     *Decimal         `json:"inverter_pct"`
	BatteryPct      *Decimal         `json:"battery_pct"`
	GridPct         *Decimal         `json:"grid_pct"`
	Trend           []RenewablePoint `json:"trend" required:"true"`
	Recommendations []string         `json:"recommendations" required:"true"`
	Unavailable     Unavailable      `json:"unavailable" required:"true"`
}

// RenewableForecast is the forecast panel from stored forecast runs.
type RenewableForecast struct {
	ConsumptionNext24hKwh  *Decimal    `json:"consumption_next_24h_kwh"`
	ConsumptionNext7dKwh   *Decimal    `json:"consumption_next_7d_kwh"`
	ConsumptionNext28dKwh  *Decimal    `json:"consumption_next_28d_kwh"`
	AccuracyDailyPct       *Decimal    `json:"accuracy_daily_pct"`
	AccuracyWeeklyPct      *Decimal    `json:"accuracy_weekly_pct"`
	AccuracyOverallPct     *Decimal    `json:"accuracy_overall_pct"`
	EstimatedGenerationKwh *Decimal    `json:"estimated_generation_kwh"`
	NetExcessKwh           *Decimal    `json:"net_excess_kwh"`
	WeatherImpact          *string     `json:"weather_impact"`
	Unavailable            Unavailable `json:"unavailable" required:"true"`
}

// RenewableFinancial is §7.8's financial gains inside the analytics panel.
type RenewableFinancial struct {
	ImportPrice        *Decimal    `json:"import_price"`
	ExportPrice        *Decimal    `json:"export_price"`
	Currency           *string     `json:"currency"`
	TodayImportCost    *Decimal    `json:"today_import_cost"`
	TodayExportRevenue *Decimal    `json:"today_export_revenue"`
	NetToday           *Decimal    `json:"net_today"`
	MonthEarnings      *Decimal    `json:"month_earnings"`
	YearEarnings       *Decimal    `json:"year_earnings"`
	TotalSavings       *Decimal    `json:"total_savings"`
	RoiPct             *Decimal    `json:"roi_pct"`
	PaybackYears       *Decimal    `json:"payback_years"`
	BillSavings        *Decimal    `json:"bill_savings"`
	Unavailable        Unavailable `json:"unavailable" required:"true"`
}

// RenewableAnalytics is the analytics panel.
type RenewableAnalytics struct {
	PeakGenerationKwh       *Decimal           `json:"peak_generation_kwh"`
	PeakGenerationAt        *time.Time         `json:"peak_generation_at"`
	AverageGenerationKwh    *Decimal           `json:"average_generation_kwh"`
	Trend                   []RenewablePoint   `json:"trend" required:"true"`
	PeakHour                *int               `json:"peak_hour"`
	DataAvailabilityPct     *Decimal           `json:"data_availability_pct"`
	SystemEfficiencyPct     *Decimal           `json:"system_efficiency_pct"`
	EfficiencyChange30dPct  *Decimal           `json:"efficiency_change_30d_pct"`
	ConsumptionOptimisation *string            `json:"consumption_optimisation"`
	MaintenanceRequired     *string            `json:"maintenance_required"`
	Financial               RenewableFinancial `json:"financial" required:"true"`
	Unavailable             Unavailable        `json:"unavailable" required:"true"`
}

// RenewableSystemStatus is the system status panel.
type RenewableSystemStatus struct {
	Overall              *string     `json:"overall"`
	Monitoring           *string     `json:"monitoring"`
	GridConnection       *string     `json:"grid_connection"`
	LastReadingAt        *time.Time  `json:"last_reading_at"`
	SolarPanels          *string     `json:"solar_panels"`
	Inverter             *string     `json:"inverter"`
	Battery              *string     `json:"battery"`
	Security             *string     `json:"security"`
	TotalGenerationKwh   *Decimal    `json:"total_generation_kwh"`
	AverageEfficiencyPct *Decimal    `json:"average_efficiency_pct"`
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
	ConsumptionKwh  *Decimal      `json:"consumption_kwh"`
	Cost            []MoneyAmount `json:"cost" required:"true"`
	ProductionKwh   *Decimal      `json:"production_kwh"`
	Revenue         []MoneyAmount `json:"revenue" required:"true"`
	RevenuePartial  bool          `json:"revenue_partial" required:"true"`
	OffsetKwh       *Decimal      `json:"offset_kwh"`
	GridPurchaseKwh *Decimal      `json:"grid_purchase_kwh"`
	GridSaleKwh     *Decimal      `json:"grid_sale_kwh"`
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
	Single       *Decimal  `json:"single"`
	T1           *Decimal  `json:"t1"`
	T2           *Decimal  `json:"t2"`
	T3           *Decimal  `json:"t3"`
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
	Month         *int             `json:"month"`
	AnalyzerCount int              `json:"analyzer_count" required:"true"`
	PlantCount    int              `json:"plant_count" required:"true"`
	Figures       FinancialMonth   `json:"figures" required:"true"`
	WithData      int              `json:"with_data" required:"true"`
	Of            int              `json:"of" required:"true"`
	Tariffs       FinancialTariffs `json:"tariffs" required:"true"`
}
