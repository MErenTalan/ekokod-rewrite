package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Alarm is one configured alarm rule for a company. Mirrors table `alarms`
// (migration 00011).
//
// The schema keeps one wide table with a nullable threshold group per alarm
// type rather than a table per type, so Type is the ONLY thing that says which
// fields are meaningful. Reading a reactive threshold off a
// data_communication alarm is not a type error and will not fail — it will
// quietly be nil — so every consumer must switch on Type first.
//
// An alarm is attached to analyzers through AlarmAnalyzer rows and delivers
// through AlarmChannel rows; neither is embedded here because both are
// separate join tables the repository loads on demand.
type Alarm struct {
	ID        uuid.UUID
	CompanyID uuid.UUID

	Name      string
	Type      AlarmType
	IsEnabled bool

	// AlarmTypeReactiveLimit. The ratio thresholds are numeric(6,3) and the
	// period pairs are a count plus a unit.
	InductiveRatioThreshold  *decimal.Decimal
	InductivePeriodValue     *int32
	InductivePeriodUnit      *PeriodUnit
	CapacitiveRatioThreshold *decimal.Decimal
	CapacitivePeriodValue    *int32
	CapacitivePeriodUnit     *PeriodUnit

	ActiveConsumptionMax            *decimal.Decimal
	ActiveConsumptionMaxPeriodValue *int32
	ActiveConsumptionMaxPeriodUnit  *PeriodUnit
	ActiveConsumptionMin            *decimal.Decimal
	ActiveConsumptionMinPeriodValue *int32
	ActiveConsumptionMinPeriodUnit  *PeriodUnit

	// AlarmTypeDataCommunication.
	CommunicationThresholdHours *int32

	// AlarmTypeCurrentVoltagePower.
	VoltageMax *decimal.Decimal
	VoltageMin *decimal.Decimal
	PowerMax   *decimal.Decimal
	PowerMin   *decimal.Decimal

	// AlarmTypeInvoiceIncrease. A percent, not a ratio.
	InvoiceThresholdPct *decimal.Decimal

	// How often a firing alarm may notify, regardless of type.
	NotificationFrequencyValue *int32
	NotificationFrequencyUnit  *PeriodUnit

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// AlarmAnalyzer attaches an alarm to an analyzer. Mirrors table
// `alarm_analyzers` (migration 00011), primary key (alarm_id, analyzer_id).
type AlarmAnalyzer struct {
	AlarmID    uuid.UUID
	AnalyzerID uuid.UUID
}

// AlarmChannel is one delivery target for an alarm. Mirrors table
// `alarm_channels` (migration 00011), primary key (alarm_id, channel, target)
// — so the same address may be registered once per channel.
type AlarmChannel struct {
	AlarmID uuid.UUID
	Channel NotifyChannel
	// Target is an email address or a phone number, depending on Channel.
	Target string
}

// AlarmEvent is one firing of an alarm. Mirrors table `alarm_events`
// (migration 00011).
//
// NotifiedAt distinguishes "fired" from "delivered": an event with a nil
// NotifiedAt and a non-nil NotificationError fired and failed to reach anyone,
// which is a different operational state from one that never fired.
type AlarmEvent struct {
	ID      uuid.UUID
	AlarmID uuid.UUID
	// AnalyzerID is nullable: an alarm may fire on something other than one
	// analyzer.
	AnalyzerID *uuid.UUID

	TriggeredAt time.Time
	Message     string
	Detail      json.RawMessage

	NotifiedAt        *time.Time
	NotificationError *string
}

// AlarmFiredBill records that a bill has already fired an alarm, so that a
// recomputation cannot fire it a second time. Mirrors table
// `alarm_fired_bills` (migration 00011), primary key (alarm_id, bill_id).
type AlarmFiredBill struct {
	AlarmID uuid.UUID
	BillID  uuid.UUID
}

// IsolarForwardedAlarm deduplicates iSolar alarm forwarding. Mirrors table
// `isolar_forwarded_alarms` (migration 00011), primary key
// (plant_id, alarm_ref).
type IsolarForwardedAlarm struct {
	PlantID uuid.UUID
	// AlarmRef is the provider's own identifier for the alarm.
	AlarmRef string
	SentAt   time.Time
}
