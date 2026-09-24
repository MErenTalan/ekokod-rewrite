package dto

import (
	"time"

	"github.com/google/uuid"
)

// ReportListRequest is GET /reports (R275).
type ReportListRequest struct {
	Type       *string    `query:"type" json:"-" validate:"omitempty,oneof=monthly yearly" enum:"monthly,yearly"`
	BuildingID *uuid.UUID `query:"building_id" json:"-"`
	Year       *int       `query:"year" json:"-" validate:"omitempty,min=2000,max=2100"`
	PageRequest
}

// ReportSummary is one archive row.
type ReportSummary struct {
	ID             uuid.UUID  `json:"id" required:"true"`
	BuildingID     uuid.UUID  `json:"building_id" required:"true"`
	BuildingName   string     `json:"building_name" required:"true"`
	Type           string     `json:"type" required:"true" enum:"monthly,yearly"`
	Period         string     `json:"period" required:"true"`
	PlantSelection string     `json:"plant_selection" required:"true" enum:"all,grid,rooftop"`
	Status         string     `json:"status" required:"true" enum:"pending,completed,error"`
	ProcessedAt    *time.Time `json:"processed_at"`
	CreatedAt      time.Time  `json:"created_at" required:"true"`
}

// ReportPage is the archive: a page plus the whole match's count.
type ReportPage struct {
	Items      []ReportSummary `json:"items" required:"true"`
	NextCursor *string         `json:"next_cursor"`
	Total      int64           `json:"total" required:"true"`
}

// ReportPreviewRequest is GET /reports/preview (R273).
type ReportPreviewRequest struct {
	Type           string      `query:"type" json:"-" validate:"required,oneof=monthly yearly" required:"true" enum:"monthly,yearly"`
	Period         string      `query:"period" json:"-" validate:"required" required:"true"`
	PlantSelection string      `query:"plant_selection" json:"-" validate:"required,oneof=all grid rooftop" required:"true" enum:"all,grid,rooftop"`
	BuildingIDs    []uuid.UUID `query:"building_ids" json:"-" validate:"required,min=1,max=50" required:"true"`
	PlantIDs       []uuid.UUID `query:"plant_ids" json:"-" validate:"max=100"`
}

// ReportGenerateRequest is POST /reports/generate (R263).
type ReportGenerateRequest struct {
	Type           string      `json:"type" validate:"required,oneof=monthly yearly" required:"true" enum:"monthly,yearly"`
	Period         string      `json:"period" validate:"required" required:"true"`
	PlantSelection string      `json:"plant_selection" validate:"required,oneof=all grid rooftop" required:"true" enum:"all,grid,rooftop"`
	BuildingIDs    []uuid.UUID `json:"building_ids" validate:"required,min=1,max=50" required:"true"`
	PlantIDs       []uuid.UUID `json:"plant_ids,omitempty" validate:"max=100"`
}

// ReportGenerateItem is one building's generation.
type ReportGenerateItem struct {
	BuildingID uuid.UUID `json:"building_id" required:"true"`
	ReportID   uuid.UUID `json:"report_id" required:"true"`
	JobID      string    `json:"job_id" required:"true"`
}

// ReportGenerateAccepted lists every building's job.
type ReportGenerateAccepted struct {
	Items []ReportGenerateItem `json:"items" required:"true"`
}

// ReportEmailRequest is POST /reports/{id}/email (R266).
type ReportEmailRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	To []string  `json:"to" validate:"required,min=1,max=10" required:"true"`
}

// ReportEmailAccepted names the delivery job.
type ReportEmailAccepted struct {
	JobID string `json:"job_id" required:"true"`
}

// Report is one stored report with its payload.
type Report struct {
	ReportSummary
	Payload *ReportPayload `json:"payload"`
}

// ReportFigure is a nullable quantity with its provenance (R258, R274).
type ReportFigure struct {
	Value    *Decimal `json:"value"`
	Partial  bool     `json:"partial"`
	Excluded bool     `json:"excluded"`
	WithData int      `json:"with_data" required:"true"`
	Of       int      `json:"of" required:"true"`
}

// ReportMoney is one currency's sum; currencies never add (R270).
type ReportMoney struct {
	Currency string  `json:"currency" required:"true"`
	Value    Decimal `json:"value" required:"true"`
	WithData int     `json:"with_data" required:"true"`
	Of       int     `json:"of" required:"true"`
}

// ReportPriceRange is one price when min equals max (R257).
type ReportPriceRange struct {
	Min *Decimal `json:"min"`
	Max *Decimal `json:"max"`
}

// ReportDelta is a year-over-year change in percent (R259).
type ReportDelta struct {
	Pct *Decimal `json:"pct"`
}

// ReportCurrencyDelta is a ReportDelta for one currency.
type ReportCurrencyDelta struct {
	Currency string   `json:"currency" required:"true"`
	Pct      *Decimal `json:"pct"`
}

// ReportBuildingLine is a building's row.
type ReportBuildingLine struct {
	BuildingID    uuid.UUID `json:"building_id" required:"true"`
	Name          string    `json:"name" required:"true"`
	TariffName    *string   `json:"tariff_name"`
	PurchasePrice *Decimal  `json:"purchase_price"`
	Currency      *string   `json:"currency"`
}

// ReportPlantLine is a plant's row.
type ReportPlantLine struct {
	PlantID        uuid.UUID    `json:"plant_id" required:"true"`
	Name           string       `json:"name" required:"true"`
	Kind           string       `json:"kind" required:"true"`
	Production     ReportFigure `json:"production" required:"true"`
	FeedIn         *Decimal     `json:"feed_in"`
	Target         *Decimal     `json:"target"`
	AchievementPct *Decimal     `json:"achievement_pct"`
}

// ReportMonthPoint is one month of a this-year-vs-last-year chart.
type ReportMonthPoint struct {
	Month    int      `json:"month" required:"true"`
	Current  *Decimal `json:"current"`
	Previous *Decimal `json:"previous"`
}

// ReportMonthly is the monthly report (01 §7.14).
type ReportMonthly struct {
	Year                 int                   `json:"year" required:"true"`
	Month                int                   `json:"month" required:"true"`
	DaysInMonth          int                   `json:"days_in_month" required:"true"`
	PlantSelection       string                `json:"plant_selection" required:"true"`
	Partial              bool                  `json:"partial" required:"true"`
	Buildings            []ReportBuildingLine  `json:"buildings" required:"true"`
	Plants               []ReportPlantLine     `json:"plants" required:"true"`
	Consumption          ReportFigure          `json:"consumption" required:"true"`
	ConsumptionPrev      ReportFigure          `json:"consumption_prev" required:"true"`
	DailyConsumption     ReportFigure          `json:"daily_consumption" required:"true"`
	T1                   ReportFigure          `json:"t1" required:"true"`
	T2                   ReportFigure          `json:"t2" required:"true"`
	T3                   ReportFigure          `json:"t3" required:"true"`
	Inductive            ReportFigure          `json:"inductive" required:"true"`
	Capacitive           ReportFigure          `json:"capacitive" required:"true"`
	Rooftop              ReportFigure          `json:"rooftop" required:"true"`
	RooftopPrev          ReportFigure          `json:"rooftop_prev" required:"true"`
	Utility              ReportFigure          `json:"utility" required:"true"`
	UtilityPrev          ReportFigure          `json:"utility_prev" required:"true"`
	Production           ReportFigure          `json:"production" required:"true"`
	ProductionPrev       ReportFigure          `json:"production_prev" required:"true"`
	DailyProduction      ReportFigure          `json:"daily_production" required:"true"`
	Bill                 []ReportMoney         `json:"bill" required:"true"`
	BillPrev             []ReportMoney         `json:"bill_prev" required:"true"`
	EnergyCost           []ReportMoney         `json:"energy_cost" required:"true"`
	DistributionCost     []ReportMoney         `json:"distribution_cost" required:"true"`
	Taxes                []ReportMoney         `json:"taxes" required:"true"`
	ReactivePenalty      []ReportMoney         `json:"reactive_penalty" required:"true"`
	ReactivePenaltyPrev  []ReportMoney         `json:"reactive_penalty_prev" required:"true"`
	InductiveRatio       *Decimal              `json:"inductive_ratio"`
	CapacitiveRatio      *Decimal              `json:"capacitive_ratio"`
	AveragePurchasePrice []ReportMoney         `json:"average_purchase_price" required:"true"`
	RooftopFeedIn        ReportPriceRange      `json:"rooftop_feed_in" required:"true"`
	UtilityFeedIn        ReportPriceRange      `json:"utility_feed_in" required:"true"`
	ConsumptionDelta     ReportDelta           `json:"consumption_delta" required:"true"`
	ProductionDelta      ReportDelta           `json:"production_delta" required:"true"`
	BillDelta            []ReportCurrencyDelta `json:"bill_delta" required:"true"`
	ReactivePenaltyDelta []ReportCurrencyDelta `json:"reactive_penalty_delta" required:"true"`
	ConsumptionChart     []ReportMonthPoint    `json:"consumption_chart" required:"true"`
	BillChart            []ReportMonthPoint    `json:"bill_chart" required:"true"`
	ChartCurrency        *string               `json:"chart_currency"`
	OmittedCurrencies    []string              `json:"omitted_currencies" required:"true"`
}

// ReportYearMonthRow is one month of the yearly table.
type ReportYearMonthRow struct {
	Month           int           `json:"month" required:"true"`
	Consumption     ReportFigure  `json:"consumption" required:"true"`
	Rooftop         ReportFigure  `json:"rooftop" required:"true"`
	Bill            []ReportMoney `json:"bill" required:"true"`
	ReactivePenalty []ReportMoney `json:"reactive_penalty" required:"true"`
}

// ReportYearPoint is one year of the history charts.
type ReportYearPoint struct {
	Year        int           `json:"year" required:"true"`
	Consumption *Decimal      `json:"consumption"`
	Production  *Decimal      `json:"production"`
	Bill        []ReportMoney `json:"bill" required:"true"`
}

// ReportCarbon is 02 §9.4 in tonnes per year.
type ReportCarbon struct {
	Factor       Decimal  `json:"factor" required:"true"`
	FactorUnit   string   `json:"factor_unit" required:"true"`
	SourceYear   *int     `json:"source_year"`
	ConsumptionT *Decimal `json:"consumption_t"`
	ReductionT   *Decimal `json:"reduction_t"`
	NetT         *Decimal `json:"net_t"`
}

// ReportYearly is the yearly report (01 §7.14).
type ReportYearly struct {
	Year              int                   `json:"year" required:"true"`
	PlantSelection    string                `json:"plant_selection" required:"true"`
	Partial           bool                  `json:"partial" required:"true"`
	Buildings         []ReportBuildingLine  `json:"buildings" required:"true"`
	Plants            []ReportPlantLine     `json:"plants" required:"true"`
	Months            []ReportYearMonthRow  `json:"months" required:"true"`
	Consumption       ReportFigure          `json:"consumption" required:"true"`
	Rooftop           ReportFigure          `json:"rooftop" required:"true"`
	Utility           ReportFigure          `json:"utility" required:"true"`
	Production        ReportFigure          `json:"production" required:"true"`
	Bill              []ReportMoney         `json:"bill" required:"true"`
	ReactivePenalty   []ReportMoney         `json:"reactive_penalty" required:"true"`
	DailyConsumption  ReportFigure          `json:"daily_consumption" required:"true"`
	DailyRooftop      ReportFigure          `json:"daily_rooftop" required:"true"`
	DailyProduction   ReportFigure          `json:"daily_production" required:"true"`
	DailyUtility      ReportFigure          `json:"daily_utility" required:"true"`
	Target            *Decimal              `json:"target"`
	AchievementPct    *Decimal              `json:"achievement_pct"`
	ConsumptionDelta  ReportDelta           `json:"consumption_delta" required:"true"`
	BillDelta         []ReportCurrencyDelta `json:"bill_delta" required:"true"`
	SolarSharePct     *Decimal              `json:"solar_share_pct"`
	GridSharePct      *Decimal              `json:"grid_share_pct"`
	Carbon            *ReportCarbon         `json:"carbon"`
	CarbonReason      string                `json:"carbon_reason,omitempty"`
	History           []ReportYearPoint     `json:"history" required:"true"`
	ChartCurrency     *string               `json:"chart_currency"`
	OmittedCurrencies []string              `json:"omitted_currencies" required:"true"`
}

// ReportPayload is a report's figures: GET /reports/preview and
// GET /reports/{id}'s payload.
type ReportPayload struct {
	Version int            `json:"version" required:"true"`
	Type    string         `json:"type" required:"true" enum:"monthly,yearly"`
	Period  string         `json:"period" required:"true"`
	Monthly *ReportMonthly `json:"monthly,omitempty"`
	Yearly  *ReportYearly  `json:"yearly,omitempty"`
}
