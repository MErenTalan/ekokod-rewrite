package dto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ConsumptionQuery selects exactly one of analyzer_id / building_id (R160).
type ConsumptionQuery struct {
	AnalyzerID  *uuid.UUID `query:"analyzer_id" json:"-"`
	BuildingID  *uuid.UUID `query:"building_id" json:"-"`
	Granularity string     `query:"granularity" json:"-" validate:"required,oneof=hourly daily monthly yearly" required:"true" enum:"hourly,daily,monthly,yearly"`
	From        Date       `query:"from" json:"-" validate:"required" required:"true"`
	To          Date       `query:"to" json:"-" validate:"required" required:"true"`
}

// ConsumptionExportQuery adds the file format.
type ConsumptionExportQuery struct {
	ConsumptionQuery
	Format string `query:"format" json:"-" validate:"required,oneof=csv xlsx" required:"true" enum:"csv,xlsx"`
}

// Registers holds one value per energy register.
type Registers struct {
	ActiveImport             *Decimal `json:"active_import"`
	ReactiveInductiveImport  *Decimal `json:"reactive_inductive_import"`
	ReactiveCapacitiveImport *Decimal `json:"reactive_capacitive_import"`
	T1Import                 *Decimal `json:"t1_import"`
	T2Import                 *Decimal `json:"t2_import"`
	T3Import                 *Decimal `json:"t3_import"`
	ActiveExport             *Decimal `json:"active_export"`
	ReactiveInductiveExport  *Decimal `json:"reactive_inductive_export"`
	ReactiveCapacitiveExport *Decimal `json:"reactive_capacitive_export"`
	T1Export                 *Decimal `json:"t1_export"`
	T2Export                 *Decimal `json:"t2_export"`
	T3Export                 *Decimal `json:"t3_export"`
}

// RegisterIndexes holds each register's closing index.
type RegisterIndexes struct {
	ActiveImportIndex             *Decimal `json:"active_import_index"`
	ReactiveInductiveImportIndex  *Decimal `json:"reactive_inductive_import_index"`
	ReactiveCapacitiveImportIndex *Decimal `json:"reactive_capacitive_import_index"`
	T1ImportIndex                 *Decimal `json:"t1_import_index"`
	T2ImportIndex                 *Decimal `json:"t2_import_index"`
	T3ImportIndex                 *Decimal `json:"t3_import_index"`
	ActiveExportIndex             *Decimal `json:"active_export_index"`
	ReactiveInductiveExportIndex  *Decimal `json:"reactive_inductive_export_index"`
	ReactiveCapacitiveExportIndex *Decimal `json:"reactive_capacitive_export_index"`
	T1ExportIndex                 *Decimal `json:"t1_export_index"`
	T2ExportIndex                 *Decimal `json:"t2_export_index"`
	T3ExportIndex                 *Decimal `json:"t3_export_index"`
}

// ConsumptionRow is one period: every 01 §7.3 column. Export registers are the
// generation columns (U1–U3 = t1–t3_export).
type ConsumptionRow struct {
	PeriodStart      time.Time  `json:"period_start" required:"true"`
	PeriodEnd        time.Time  `json:"period_end" required:"true"`
	AnalyzerID       *uuid.UUID `json:"analyzer_id"`
	Source           string     `json:"source" required:"true"`
	Partial          bool       `json:"partial" required:"true"`
	SuspectRegisters []string   `json:"suspect_registers" required:"true"`
	RegisterIndexes
	Registers
	InductiveRatio  *Decimal `json:"inductive_ratio"`
	CapacitiveRatio *Decimal `json:"capacitive_ratio"`
	MaxDemandKw     *Decimal `json:"max_demand_kw"`
}

// ConsumptionSeries is GET /consumption and GET /generation.
type ConsumptionSeries struct {
	Items []ConsumptionRow `json:"items" required:"true"`
}

// PeriodValue names a period and its active consumption.
type PeriodValue struct {
	PeriodStart  time.Time `json:"period_start" required:"true"`
	ActiveImport Decimal   `json:"active_import" required:"true"`
}

// ConsumptionSummary is GET /consumption/summary.
type ConsumptionSummary struct {
	Rows        int          `json:"rows" required:"true"`
	SuspectRows int          `json:"suspect_rows" required:"true"`
	Totals      Registers    `json:"totals" required:"true"`
	Averages    Registers    `json:"averages" required:"true"`
	Peak        *PeriodValue `json:"peak"`
	Valley      *PeriodValue `json:"valley"`
	MaxDemandKw *Decimal     `json:"max_demand_kw"`
}

// BalanceRow is one period of the energy balance.
type BalanceRow struct {
	PeriodStart time.Time `json:"period_start" required:"true"`
	PeriodEnd   time.Time `json:"period_end" required:"true"`
	Consumption *Decimal  `json:"consumption"`
	Generation  *Decimal  `json:"generation"`
	GridImport  *Decimal  `json:"grid_import"`
	GridExport  *Decimal  `json:"grid_export"`
}

// EnergyBalance is GET /energy-balance.
type EnergyBalance struct {
	Items []BalanceRow `json:"items" required:"true"`
}

// AnomalyQuery is GET /consumption/anomalies.
type AnomalyQuery struct {
	AnalyzerID *uuid.UUID `query:"analyzer_id" json:"-"`
	BuildingID *uuid.UUID `query:"building_id" json:"-"`
	Unresolved bool       `query:"unresolved" json:"-"`
	PageRequest
}

// Anomaly is a suspect period.
type Anomaly struct {
	ID          uuid.UUID       `json:"id" required:"true"`
	AnalyzerID  uuid.UUID       `json:"analyzer_id" required:"true"`
	PeriodStart time.Time       `json:"period_start" required:"true"`
	PeriodEnd   time.Time       `json:"period_end" required:"true"`
	Reason      string          `json:"reason" required:"true"`
	Detail      json.RawMessage `json:"detail"`
	Resolution  *string         `json:"resolution"`
	ResolvedAt  *time.Time      `json:"resolved_at"`
	ResolvedBy  *uuid.UUID      `json:"resolved_by"`
	CreatedAt   time.Time       `json:"created_at" required:"true"`
}

// AnomalyResolveRequest is POST /consumption/anomalies/{id}/resolve.
type AnomalyResolveRequest struct {
	ID         uuid.UUID          `path:"id" json:"-"`
	Mode       string             `json:"mode" validate:"required,oneof=reset_registered manual_override accepted" required:"true" enum:"reset_registered,manual_override,accepted"`
	ResetTs    *time.Time         `json:"reset_ts,omitempty"`
	ResetAfter map[string]Decimal `json:"reset_after,omitempty"`
	Overrides  map[string]Decimal `json:"overrides,omitempty"`
}

// ReactiveStatusQuery is GET /consumption/reactive-status.
type ReactiveStatusQuery struct {
	Month      string     `query:"month" json:"-" validate:"omitempty,datetime=2006-01" pattern:"^[0-9]{4}-[0-9]{2}$"`
	BuildingID *uuid.UUID `query:"building_id" json:"-"`
}

// ReactiveAnalyzer is one analyzer's reactive position (R166).
type ReactiveAnalyzer struct {
	AnalyzerID       uuid.UUID  `json:"analyzer_id" required:"true"`
	BuildingID       *uuid.UUID `json:"building_id"`
	HasData          bool       `json:"has_data" required:"true"`
	InductiveRatio   *Decimal   `json:"inductive_ratio"`
	CapacitiveRatio  *Decimal   `json:"capacitive_ratio"`
	InductiveLimit   *Decimal   `json:"inductive_limit"`
	CapacitiveLimit  *Decimal   `json:"capacitive_limit"`
	InstalledPowerKw *Decimal   `json:"installed_power_kw"`
	ExemptReason     *string    `json:"exempt_reason" enum:"term,user_group,generation,below_kw"`
	PenaltyApplies   bool       `json:"penalty_applies" required:"true"`
}

// ReactiveExtreme names the analyzer with the highest ratio.
type ReactiveExtreme struct {
	AnalyzerID uuid.UUID  `json:"analyzer_id" required:"true"`
	BuildingID *uuid.UUID `json:"building_id"`
	Ratio      Decimal    `json:"ratio" required:"true"`
}

// ReactiveStatus is GET /consumption/reactive-status.
type ReactiveStatus struct {
	Month             string             `json:"month" required:"true"`
	Analyzers         []ReactiveAnalyzer `json:"analyzers" required:"true"`
	HighestInductive  *ReactiveExtreme   `json:"highest_inductive"`
	HighestCapacitive *ReactiveExtreme   `json:"highest_capacitive"`
}

// LoadProfileQuery is GET /load-profile, /statistics and /export.
type LoadProfileQuery struct {
	AnalyzerID uuid.UUID `query:"analyzer_id" json:"-" validate:"required" required:"true"`
	From       Date      `query:"from" json:"-" validate:"required" required:"true"`
	To         Date      `query:"to" json:"-" validate:"required" required:"true"`
	Profiles   []string  `query:"profiles" json:"-"`
}

// LoadProfileConfig is the calendar the split used.
type LoadProfileConfig struct {
	WeekendDays   []int  `json:"weekend_days" required:"true"`
	WeekendSource string `json:"weekend_source" required:"true" enum:"company,default"`
	Vacations     int    `json:"vacations" required:"true"`
}

// LoadProfiles is GET /load-profile: 24 hourly means per profile.
type LoadProfiles struct {
	Profiles map[string][]*Decimal `json:"profiles" required:"true"`
	Days     map[string]int        `json:"days" required:"true"`
	Config   LoadProfileConfig     `json:"config" required:"true"`
}

// ProfileStatistics is one profile's §10.3 statistics.
type ProfileStatistics struct {
	Max        *Decimal `json:"max"`
	Min        *Decimal `json:"min"`
	HourOfMax  *int     `json:"hour_of_max"`
	Mean       *Decimal `json:"mean"`
	StdDev     *Decimal `json:"stddev"`
	Range      *Decimal `json:"range"`
	LoadFactor *Decimal `json:"load_factor"`
}

// LoadProfileStatistics is GET /load-profile/statistics.
type LoadProfileStatistics struct {
	Statistics map[string]ProfileStatistics `json:"statistics" required:"true"`
	Config     LoadProfileConfig            `json:"config" required:"true"`
}

// AnomalyCheckRequest is POST /anomaly/check.
type AnomalyCheckRequest struct {
	AnalyzerID uuid.UUID `json:"analyzer_id" validate:"required" required:"true"`
	Ts         time.Time `json:"ts" validate:"required" required:"true"`
	Actual     *Decimal  `json:"actual" validate:"required" required:"true"`
}

// AnomalyCheck is the verdict, or availability=false before F13 (R165).
type AnomalyCheck struct {
	Available bool   `json:"available" required:"true"`
	Reason    string `json:"reason,omitempty"`
}
