package ml

import "time"

// Wire types of the 03 §6.3 contract as the F13a service implements it.

// Point is one history value; a nil Value is a missing step (a gap).
type Point struct {
	Ts    time.Time `json:"ts"`
	Value *float64  `json:"value"`
}

// DayType is one Europe/Istanbul date's type: workday, weekend, holiday or half_day.
type DayType struct {
	Date string `json:"date"`
	Type string `json:"type"`
}

// Vacation flags one date.
type Vacation struct {
	Date     string `json:"date"`
	Vacation bool   `json:"vacation"`
}

// Covariates travel with the history; the service stores nothing.
type Covariates struct {
	DayType  []DayType  `json:"day_type,omitempty"`
	Vacation []Vacation `json:"vacation,omitempty"`
}

// ForecastRequest is POST /v1/forecast.
type ForecastRequest struct {
	SeriesID    string     `json:"series_id"`
	Granularity string     `json:"granularity"`
	Horizon     int        `json:"horizon"`
	History     []Point    `json:"history"`
	Covariates  Covariates `json:"covariates"`
	Model       string     `json:"model,omitempty"`
}

// Gap is one run of missing steps in the supplied history.
type Gap struct {
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	MissingHours int       `json:"missing_hours"`
}

// ForecastResponse carries an explicit status and the model that produced it.
type ForecastResponse struct {
	Status         string      `json:"status"`
	ModelID        string      `json:"model_id"`
	ModelVersion   string      `json:"model_version"`
	Timestamps     []time.Time `json:"timestamps"`
	Median         []float64   `json:"median"`
	P10            []float64   `json:"p10"`
	P90            []float64   `json:"p90"`
	UsedCovariates []string    `json:"used_covariates"`
	Gaps           []Gap       `json:"gaps"`
	FallbackFrom   *string     `json:"fallback_from"`
	Message        *string     `json:"message"`
}

// AnomalyRequest is POST /v1/anomaly.
type AnomalyRequest struct {
	SeriesID string    `json:"series_id"`
	Ts       time.Time `json:"ts"`
	Actual   float64   `json:"actual"`
	History  []Point   `json:"history"`
}

// AnomalyResponse names its method; score and bands are nil with insufficient history.
type AnomalyResponse struct {
	IsAnomaly    bool     `json:"is_anomaly"`
	Score        *float64 `json:"score"`
	Expected     *float64 `json:"expected"`
	Lower        *float64 `json:"lower"`
	Upper        *float64 `json:"upper"`
	Method       string   `json:"method"`
	ModelID      string   `json:"model_id"`
	ModelVersion string   `json:"model_version"`
}

// Model is one GET /v1/models entry.
type Model struct {
	ID        string  `json:"id"`
	Version   string  `json:"version"`
	Available bool    `json:"available"`
	TrainedAt *string `json:"trained_at"`
	Default   bool    `json:"default"`
}
