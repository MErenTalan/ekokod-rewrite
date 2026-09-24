package dto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AlarmSettings is the union of every alarm type's settings. Which fields are
// meaningful follows from Alarm.Type — the schema keeps one wide row, so the
// type is the only thing that says what means anything (04 §7).
type AlarmSettings struct {
	InductiveRatioThreshold  *Decimal `json:"inductive_ratio_threshold,omitempty"`
	InductivePeriodValue     *int32   `json:"inductive_period_value,omitempty" minimum:"1" maximum:"8760"`
	InductivePeriodUnit      *string  `json:"inductive_period_unit,omitempty" validate:"omitempty,oneof=hours days" enum:"hours,days"`
	CapacitiveRatioThreshold *Decimal `json:"capacitive_ratio_threshold,omitempty"`
	CapacitivePeriodValue    *int32   `json:"capacitive_period_value,omitempty" minimum:"1" maximum:"8760"`
	CapacitivePeriodUnit     *string  `json:"capacitive_period_unit,omitempty" validate:"omitempty,oneof=hours days" enum:"hours,days"`

	ActiveConsumptionMax            *Decimal `json:"active_consumption_max,omitempty"`
	ActiveConsumptionMaxPeriodValue *int32   `json:"active_consumption_max_period_value,omitempty" minimum:"1" maximum:"8760"`
	ActiveConsumptionMaxPeriodUnit  *string  `json:"active_consumption_max_period_unit,omitempty" validate:"omitempty,oneof=hours days" enum:"hours,days"`
	ActiveConsumptionMin            *Decimal `json:"active_consumption_min,omitempty"`
	ActiveConsumptionMinPeriodValue *int32   `json:"active_consumption_min_period_value,omitempty" minimum:"1" maximum:"8760"`
	ActiveConsumptionMinPeriodUnit  *string  `json:"active_consumption_min_period_unit,omitempty" validate:"omitempty,oneof=hours days" enum:"hours,days"`

	CommunicationThresholdHours *int32 `json:"communication_threshold_hours,omitempty" minimum:"1" maximum:"8760"`

	PowerMax *Decimal `json:"power_max,omitempty"`
	PowerMin *Decimal `json:"power_min,omitempty"`
	// VoltageMax and VoltageMin exist ONLY so that a client sending one gets a
	// 422 naming the field instead of a silent drop (R212). No provider in
	// 06-integrations reports voltage, and legacy's alarm read a field that
	// existed in no payload, so it never fired.
	VoltageMax *Decimal `json:"voltage_max,omitempty"`
	VoltageMin *Decimal `json:"voltage_min,omitempty"`

	InvoiceThresholdPct *Decimal `json:"invoice_threshold_pct,omitempty"`
}

// AlarmChannel is one delivery target.
type AlarmChannel struct {
	Channel string `json:"channel" validate:"required,oneof=email sms" required:"true" enum:"email,sms"`
	Target  string `json:"target" validate:"required,max=254" required:"true"`
}

// AlarmFields is an alarm create or full replace.
type AlarmFields struct {
	Name      string `json:"name" validate:"required,max=200" required:"true"`
	Type      string `json:"type" validate:"required,oneof=reactive_limit data_communication current_voltage_power invoice_increase" required:"true" enum:"reactive_limit,data_communication,current_voltage_power,invoice_increase"`
	IsEnabled bool   `json:"is_enabled"`
	// AnalyzerIDs must all be inside the caller's scope; one that is not
	// refuses the whole call (R213).
	AnalyzerIDs []uuid.UUID    `json:"analyzer_ids" validate:"required,min=1,max=200" required:"true"`
	Channels    []AlarmChannel `json:"channels" validate:"max=70,dive"`

	NotificationFrequencyValue *int32  `json:"notification_frequency_value,omitempty" minimum:"1" maximum:"8760"`
	NotificationFrequencyUnit  *string `json:"notification_frequency_unit,omitempty" validate:"omitempty,oneof=hours days" enum:"hours,days"`

	Settings AlarmSettings `json:"settings"`
}

// AlarmUpdateRequest is PATCH /alarms/{id}: a full replace, as
// PATCH /calendar/events/{id} is. A partial merge over a wide nullable
// settings row would make "clear this threshold" unexpressible.
type AlarmUpdateRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	AlarmFields
}

// AlarmAnalyzer is one attached meter, labelled for the list column.
type AlarmAnalyzer struct {
	ID                 uuid.UUID  `json:"id" required:"true"`
	InstallationNumber string     `json:"installation_number" required:"true"`
	BuildingID         *uuid.UUID `json:"building_id"`
}

// Alarm is a stored rule with its attachments.
type Alarm struct {
	ID        uuid.UUID       `json:"id" required:"true"`
	Name      string          `json:"name" required:"true"`
	Type      string          `json:"type" required:"true" enum:"reactive_limit,data_communication,current_voltage_power,invoice_increase"`
	IsEnabled bool            `json:"is_enabled" required:"true"`
	Analyzers []AlarmAnalyzer `json:"analyzers" required:"true"`
	Channels  []AlarmChannel  `json:"channels" required:"true"`

	NotificationFrequencyValue *int32  `json:"notification_frequency_value"`
	NotificationFrequencyUnit  *string `json:"notification_frequency_unit"`

	Settings  AlarmSettings `json:"settings" required:"true"`
	CreatedAt time.Time     `json:"created_at" required:"true"`
	UpdatedAt time.Time     `json:"updated_at" required:"true"`
}

// AlarmListRequest is GET /alarms.
type AlarmListRequest struct {
	PageRequest
	Type       *string    `query:"type" json:"-" validate:"omitempty,oneof=reactive_limit data_communication current_voltage_power invoice_increase" enum:"reactive_limit,data_communication,current_voltage_power,invoice_increase"`
	IsEnabled  *bool      `query:"is_enabled" json:"-"`
	AnalyzerID *uuid.UUID `query:"analyzer_id" json:"-"`
}

// AlarmEventsRequest is GET /alarms/{id}/events.
type AlarmEventsRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	PageRequest
	From *time.Time `query:"from" json:"-"`
	To   *time.Time `query:"to" json:"-"`
}

// AlarmEvent is one firing.
type AlarmEvent struct {
	ID          uuid.UUID  `json:"id" required:"true"`
	AnalyzerID  *uuid.UUID `json:"analyzer_id"`
	TriggeredAt time.Time  `json:"triggered_at" required:"true"`
	Message     string     `json:"message" required:"true"`
	// Detail is the curated object of R224 — measured values, thresholds and
	// the window — never a provider or SMTP string.
	Detail json.RawMessage `json:"detail,omitempty"`
	// NotifiedAt nil with a non-nil NotificationError is a real state: the
	// alarm fired and nobody was told.
	NotifiedAt        *time.Time `json:"notified_at"`
	NotificationError *string    `json:"notification_error"`
}

// AlarmEvaluateRequest is POST /alarms/{id}/evaluate.
type AlarmEvaluateRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	// Notify runs the real firing path instead of a dry run (R223).
	Notify bool `query:"notify" json:"-"`
}

// AlarmEvaluationBreach is one crossed threshold, already rendered.
type AlarmEvaluationBreach struct {
	Field     string  `json:"field" required:"true"`
	Measured  Decimal `json:"measured" required:"true"`
	Threshold Decimal `json:"threshold" required:"true"`
	Message   string  `json:"message" required:"true"`
}

// AlarmEvaluationAnalyzer is one meter's outcome.
type AlarmEvaluationAnalyzer struct {
	AnalyzerID         uuid.UUID               `json:"analyzer_id" required:"true"`
	InstallationNumber string                  `json:"installation_number" required:"true"`
	Fired              bool                    `json:"fired" required:"true"`
	Breaches           []AlarmEvaluationBreach `json:"breaches" required:"true"`
	// NoVerdict names the settings that could not be decided for lack of data.
	// It is NOT "compliant", and the screen must not render it as such.
	NoVerdict []string `json:"no_verdict" required:"true"`
}

// AlarmEvaluation is the result of an evaluate call.
type AlarmEvaluation struct {
	EvaluatedAt time.Time                 `json:"evaluated_at" required:"true"`
	DryRun      bool                      `json:"dry_run" required:"true"`
	Analyzers   []AlarmEvaluationAnalyzer `json:"analyzers" required:"true"`
	// NotificationsSent is 0 on a dry run and when the frequency suppressed
	// every firing (R218).
	NotificationsSent int `json:"notifications_sent" required:"true"`
}
