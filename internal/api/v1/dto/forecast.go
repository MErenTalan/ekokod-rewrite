package dto

import (
	"time"

	"github.com/google/uuid"
)

// 05 §16, F13b R370–R379.

// ForecastQuery is GET /forecast (R373).
type ForecastQuery struct {
	AnalyzerID uuid.UUID  `query:"analyzer_id" json:"-" validate:"required" required:"true"`
	From       *time.Time `query:"from" json:"-" validate:"required" required:"true"`
	To         *time.Time `query:"to" json:"-" validate:"required" required:"true"`
}

// ForecastRunRequest is POST /forecast/run (R372).
type ForecastRunRequest struct {
	AnalyzerID   uuid.UUID `json:"analyzer_id" validate:"required" required:"true"`
	HorizonHours int       `json:"horizon_hours" validate:"required,min=1,max=744" required:"true" minimum:"1" maximum:"744"`
}

// ForecastWeeklyRequest is POST /forecast/weekly (R374).
type ForecastWeeklyRequest struct {
	AnalyzerID uuid.UUID `json:"analyzer_id" validate:"required" required:"true"`
	WeekStart  *Date     `json:"week_start" validate:"required" required:"true"`
}

// ForecastMonthlyRequest is POST /forecast/monthly (R374).
type ForecastMonthlyRequest struct {
	AnalyzerID uuid.UUID `json:"analyzer_id" validate:"required" required:"true"`
	Month      string    `json:"month" validate:"required,len=7" required:"true" pattern:"^[0-9]{4}-[0-9]{2}$"`
}

// ForecastPoint is one step: the median with its p10–p90 band.
type ForecastPoint struct {
	Ts     time.Time `json:"ts" required:"true"`
	Median Decimal   `json:"median" required:"true"`
	P10    *Decimal  `json:"p10,omitempty"`
	P90    *Decimal  `json:"p90,omitempty"`
}

// ForecastGap is one run of missing history hours (01 §7.5).
type ForecastGap struct {
	Start        time.Time `json:"start" required:"true"`
	End          time.Time `json:"end" required:"true"`
	MissingHours int       `json:"missing_hours" required:"true"`
}

// Forecast is every forecast answer. Status is the ML status, or none when no run is stored.
type Forecast struct {
	Status         string          `json:"status" required:"true" enum:"ok,insufficient_data,no_data,model_error,none"`
	ModelID        *string         `json:"model_id,omitempty"`
	ModelVersion   *string         `json:"model_version,omitempty"`
	GeneratedAt    *time.Time      `json:"generated_at,omitempty"`
	FallbackFrom   *string         `json:"fallback_from,omitempty"`
	UsedCovariates []string        `json:"used_covariates" required:"true"`
	Points         []ForecastPoint `json:"points" required:"true"`
	Gaps           []ForecastGap   `json:"gaps" required:"true"`
}
