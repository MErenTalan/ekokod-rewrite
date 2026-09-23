package report

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Report types and plant selections, as stored in reports.type and
// reports.plant_selection.
const (
	TypeMonthly = "monthly"
	TypeYearly  = "yearly"

	SelectionAll     = "all"
	SelectionGrid    = "grid"
	SelectionRooftop = "rooftop"

	KindGrid    = "grid"
	KindRooftop = "rooftop"

	// PayloadVersion is written into every stored payload so a later
	// renderer can tell which shape it is reading.
	PayloadVersion = 1
)

// MonthSeries is January…December of one year; nil means no data.
type MonthSeries [12]*decimal.Decimal

// Bill is the part of one building-scope invoice a report reads (R255).
type Bill struct {
	Currency         string
	Total            decimal.Decimal
	EnergyCost       decimal.Decimal
	DistributionCost decimal.Decimal
	// Taxes is VAT plus the other taxes.
	Taxes           decimal.Decimal
	ReactivePenalty decimal.Decimal
	NetConsumption  decimal.Decimal
	EffectivePrice  *decimal.Decimal
	GenerationPrice *decimal.Decimal
}

// BuildingInput is one selected building. Maps are keyed by calendar year.
type BuildingInput struct {
	ID         uuid.UUID
	Name       string
	TariffName *string
	// Consumption is Σ active_import and Export Σ active_export of the
	// building's analyzers per Istanbul calendar month (R254, R256).
	Consumption map[int]MonthSeries
	Export      map[int]MonthSeries
	// Partial marks months composed from an unrefreshed or open period (R274).
	Partial map[int][12]bool
	// The report month's registers (monthly reports only).
	T1, T2, T3, Inductive, Capacitive *decimal.Decimal
	// Bills holds the building-scope bill whose period key is that month.
	Bills map[int][12]*Bill
}

// PlantInput is one selected power plant.
type PlantInput struct {
	ID         uuid.UUID
	Name       string
	Kind       string
	Production map[int]MonthSeries
	// FeedIn is the solar tariff in force on the period's first day.
	FeedIn       *decimal.Decimal
	YearlyTarget *decimal.Decimal
	// MonthlyTargets is January…December when all twelve rows exist, else empty.
	MonthlyTargets []decimal.Decimal
}

// MonthlyInput is everything BuildMonthly reads.
type MonthlyInput struct {
	Year, Month int
	Selection   string
	Buildings   []BuildingInput
	Plants      []PlantInput
}

// Figure is a nullable quantity with its provenance: how many of the
// selected subjects had data (R258), whether any was incomplete (R274), and
// whether the plant selection left it out altogether.
type Figure struct {
	Value    *decimal.Decimal `json:"value"`
	Partial  bool             `json:"partial,omitempty"`
	Excluded bool             `json:"excluded,omitempty"`
	WithData int              `json:"with_data"`
	Of       int              `json:"of"`
}

// MoneyFigure is one currency's sum; currencies never add (R270).
type MoneyFigure struct {
	Currency string          `json:"currency"`
	Value    decimal.Decimal `json:"value"`
	WithData int             `json:"with_data"`
	Of       int             `json:"of"`
}

// PriceRange is one price when Min equals Max, a range otherwise, and
// unresolved when both are nil (R257).
type PriceRange struct {
	Min *decimal.Decimal `json:"min"`
	Max *decimal.Decimal `json:"max"`
}

// Delta is a year-over-year change in percent (R259); nil when undefined.
type Delta struct {
	Pct *decimal.Decimal `json:"pct"`
}

// CurrencyDelta is a Delta for one currency's money figure.
type CurrencyDelta struct {
	Currency string           `json:"currency"`
	Pct      *decimal.Decimal `json:"pct"`
}

// BuildingLine is a building's row in the information table.
type BuildingLine struct {
	BuildingID    uuid.UUID        `json:"building_id"`
	Name          string           `json:"name"`
	TariffName    *string          `json:"tariff_name"`
	PurchasePrice *decimal.Decimal `json:"purchase_price"`
	Currency      *string          `json:"currency"`
}

// PlantLine is a plant's row.
type PlantLine struct {
	PlantID        uuid.UUID        `json:"plant_id"`
	Name           string           `json:"name"`
	Kind           string           `json:"kind"`
	Production     Figure           `json:"production"`
	FeedIn         *decimal.Decimal `json:"feed_in"`
	Target         *decimal.Decimal `json:"target"`
	AchievementPct *decimal.Decimal `json:"achievement_pct"`
}

// MonthPoint is one month of a this-year-vs-last-year chart.
type MonthPoint struct {
	Month    int              `json:"month"`
	Current  *decimal.Decimal `json:"current"`
	Previous *decimal.Decimal `json:"previous"`
}

// Monthly is the monthly report (01 §7.14, 02 §10.1).
type Monthly struct {
	Year        int            `json:"year"`
	Month       int            `json:"month"`
	DaysInMonth int            `json:"days_in_month"`
	Selection   string         `json:"plant_selection"`
	Partial     bool           `json:"partial"`
	Buildings   []BuildingLine `json:"buildings"`
	Plants      []PlantLine    `json:"plants"`

	Consumption      Figure `json:"consumption"`
	ConsumptionPrev  Figure `json:"consumption_prev"`
	DailyConsumption Figure `json:"daily_consumption"`
	T1               Figure `json:"t1"`
	T2               Figure `json:"t2"`
	T3               Figure `json:"t3"`
	Inductive        Figure `json:"inductive"`
	Capacitive       Figure `json:"capacitive"`

	Rooftop         Figure `json:"rooftop"`
	RooftopPrev     Figure `json:"rooftop_prev"`
	Utility         Figure `json:"utility"`
	UtilityPrev     Figure `json:"utility_prev"`
	Production      Figure `json:"production"`
	ProductionPrev  Figure `json:"production_prev"`
	DailyProduction Figure `json:"daily_production"`

	Bill                []MoneyFigure `json:"bill"`
	BillPrev            []MoneyFigure `json:"bill_prev"`
	EnergyCost          []MoneyFigure `json:"energy_cost"`
	DistributionCost    []MoneyFigure `json:"distribution_cost"`
	Taxes               []MoneyFigure `json:"taxes"`
	ReactivePenalty     []MoneyFigure `json:"reactive_penalty"`
	ReactivePenaltyPrev []MoneyFigure `json:"reactive_penalty_prev"`

	InductiveRatio       *decimal.Decimal `json:"inductive_ratio"`
	CapacitiveRatio      *decimal.Decimal `json:"capacitive_ratio"`
	AveragePurchasePrice []MoneyFigure    `json:"average_purchase_price"`
	RooftopFeedIn        PriceRange       `json:"rooftop_feed_in"`
	UtilityFeedIn        PriceRange       `json:"utility_feed_in"`

	ConsumptionDelta     Delta           `json:"consumption_delta"`
	ProductionDelta      Delta           `json:"production_delta"`
	BillDelta            []CurrencyDelta `json:"bill_delta"`
	ReactivePenaltyDelta []CurrencyDelta `json:"reactive_penalty_delta"`

	ConsumptionChart  []MonthPoint `json:"consumption_chart"`
	BillChart         []MonthPoint `json:"bill_chart"`
	ChartCurrency     *string      `json:"chart_currency"`
	OmittedCurrencies []string     `json:"omitted_currencies"`
}

// GridFactor is the grid emission factor a yearly report states (R262).
type GridFactor struct {
	Value      decimal.Decimal
	Unit       string
	SourceYear *int
}

// YearlyInput is everything BuildYearly reads.
type YearlyInput struct {
	Year      int
	Selection string
	Buildings []BuildingInput
	Plants    []PlantInput
	// Factor nil makes the carbon section unavailable.
	Factor *GridFactor
}

// YearMonthRow is one month of the yearly consumption table.
type YearMonthRow struct {
	Month           int           `json:"month"`
	Consumption     Figure        `json:"consumption"`
	Rooftop         Figure        `json:"rooftop"`
	Bill            []MoneyFigure `json:"bill"`
	ReactivePenalty []MoneyFigure `json:"reactive_penalty"`
}

// YearPoint is one year of the yearly history charts.
type YearPoint struct {
	Year        int              `json:"year"`
	Consumption *decimal.Decimal `json:"consumption"`
	Production  *decimal.Decimal `json:"production"`
	Bill        []MoneyFigure    `json:"bill"`
}

// Carbon is 02 §9.4's figures in tonnes per year.
type Carbon struct {
	Factor       decimal.Decimal  `json:"factor"`
	FactorUnit   string           `json:"factor_unit"`
	SourceYear   *int             `json:"source_year"`
	ConsumptionT *decimal.Decimal `json:"consumption_t"`
	ReductionT   *decimal.Decimal `json:"reduction_t"`
	NetT         *decimal.Decimal `json:"net_t"`
}

// Yearly is the yearly report (01 §7.14, 02 §10.2).
type Yearly struct {
	Year      int            `json:"year"`
	Selection string         `json:"plant_selection"`
	Partial   bool           `json:"partial"`
	Buildings []BuildingLine `json:"buildings"`
	Plants    []PlantLine    `json:"plants"`
	Months    []YearMonthRow `json:"months"`

	Consumption      Figure        `json:"consumption"`
	Rooftop          Figure        `json:"rooftop"`
	Utility          Figure        `json:"utility"`
	Production       Figure        `json:"production"`
	Bill             []MoneyFigure `json:"bill"`
	ReactivePenalty  []MoneyFigure `json:"reactive_penalty"`
	DailyConsumption Figure        `json:"daily_consumption"`
	DailyRooftop     Figure        `json:"daily_rooftop"`
	DailyProduction  Figure        `json:"daily_production"`
	// DailyUtility is the solar section's daily average: over the same
	// plants as its target, so the two are comparable.
	DailyUtility Figure `json:"daily_utility"`

	Target         *decimal.Decimal `json:"target"`
	AchievementPct *decimal.Decimal `json:"achievement_pct"`

	ConsumptionDelta Delta            `json:"consumption_delta"`
	BillDelta        []CurrencyDelta  `json:"bill_delta"`
	SolarSharePct    *decimal.Decimal `json:"solar_share_pct"`
	GridSharePct     *decimal.Decimal `json:"grid_share_pct"`

	Carbon       *Carbon `json:"carbon"`
	CarbonReason string  `json:"carbon_reason,omitempty"`

	History           []YearPoint `json:"history"`
	ChartCurrency     *string     `json:"chart_currency"`
	OmittedCurrencies []string    `json:"omitted_currencies"`
}

// Payload is what reports.payload stores and GET /reports/preview answers.
type Payload struct {
	Version int      `json:"version"`
	Type    string   `json:"type"`
	Period  string   `json:"period"`
	Monthly *Monthly `json:"monthly,omitempty"`
	Yearly  *Yearly  `json:"yearly,omitempty"`
}
