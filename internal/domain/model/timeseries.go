package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// MeterReading is one register snapshot for one analyzer. Mirrors the
// `meter_readings` hypertable (migration 00004), whose primary key is
// (analyzer_id, ts, kind).
//
// Every register is CUMULATIVE and already multiplied by the analyzer's
// multiplier — 02-domain-rules.md §2.2 applies it exactly once, at ingestion,
// and records what was applied in MultiplierApplied. Consumption over a period
// is the difference between two of these rows, never a sum, which is why every
// register is nullable: a provider that reports only the active register
// leaves the rest genuinely absent rather than zero. A zero in a cumulative
// register would read as "the meter was at zero", which is a different and
// wrong statement.
//
// MaxDemandKw is the one exception: it is an interval maximum, not a running
// total.
type MeterReading struct {
	AnalyzerID uuid.UUID
	// Ts is the reading's timestamp in UTC. Bucketing into local days is a
	// business-logic concern that converts to Europe/Istanbul at the point of
	// use.
	Ts   time.Time
	Kind ReadingKind

	// Import registers — consumption.
	ActiveImport             *decimal.Decimal
	ReactiveInductiveImport  *decimal.Decimal
	ReactiveCapacitiveImport *decimal.Decimal
	T1Import                 *decimal.Decimal
	T2Import                 *decimal.Decimal
	T3Import                 *decimal.Decimal

	// Export registers — generation fed back to the grid.
	ActiveExport             *decimal.Decimal
	ReactiveInductiveExport  *decimal.Decimal
	ReactiveCapacitiveExport *decimal.Decimal
	T1Export                 *decimal.Decimal
	T2Export                 *decimal.Decimal
	T3Export                 *decimal.Decimal

	// MaxDemandKw is an interval value, not cumulative.
	MaxDemandKw *decimal.Decimal

	// IntervalGenerationKwh is PM5340's interval energy (06 §5, R1): the
	// energy generated DURING this interval, not a cumulative register like
	// every other field above. Migration 00012. It is nil for every other
	// provider — PM5340 is the only source that reports interval-shaped
	// generation directly instead of a cumulative export register.
	IntervalGenerationKwh *decimal.Decimal

	MeterSerial *string
	// MultiplierApplied records the multiplier used on this row, so a later
	// change to Analyzer.MeterMultiplier cannot retroactively change what a
	// stored reading meant.
	MultiplierApplied decimal.Decimal
	SourceProvider    IntegrationProvider
	IngestedAt        time.Time

	// Raw is the provider's payload, kept for forensics only. Migration 00004
	// says it plainly: no business logic reads this.
	Raw json.RawMessage
}

// IngestionCursor is the high-water mark for one analyzer and reading kind, so
// that ingestion resumes rather than refetching. Mirrors table
// `ingestion_cursors` (migration 00004), primary key (analyzer_id, kind).
type IngestionCursor struct {
	AnalyzerID uuid.UUID
	Kind       ReadingKind

	LastTs        *time.Time
	LastSuccessAt *time.Time
	LastError     *string
	LastErrorAt   *time.Time
	// ConsecutiveFailures is reset to zero on success; it is what backs off
	// an analyzer that keeps failing.
	ConsecutiveFailures int32
}

// GenerationAnchor is the last known cumulative export register value for one
// analyzer, at one instant — the reference point PM5340's interval energy
// (MeterReading.IntervalGenerationKwh) is reconciled against, so that a gap
// in interval reporting can still be checked against the meter's own
// cumulative register. Mirrors table `generation_anchors` (migration 00012),
// whose primary key is analyzer_id: one anchor per analyzer.
type GenerationAnchor struct {
	AnalyzerID uuid.UUID
	AnchorTs   time.Time
	// ActiveExport is the cumulative export register value AT AnchorTs, never
	// negative (migration 00012's check constraint).
	ActiveExport decimal.Decimal
	// Source is 'initial', 'operator' or 'migration' — a plain text column
	// with a check constraint, not a SQL enum.
	Source    string
	UpdatedAt time.Time
}

// ProviderHourlyValue is one hour of OSOS's own labelled cross-check series
// for one analyzer. Mirrors the `provider_hourly_values` hypertable
// (migration 00013), whose primary key is (analyzer_id, ts).
//
// This series is NEVER read by consumption or billing (06 §2,
// removed-behaviour 23): it exists only so an operator can compare it
// against meter_readings' own hourly figures.
type ProviderHourlyValue struct {
	AnalyzerID        uuid.UUID
	Ts                time.Time
	ActiveConsumption *decimal.Decimal
	ActiveGeneration  *decimal.Decimal
	SourceProvider    IntegrationProvider
	IngestedAt        time.Time
}

// ConsumptionAnomaly marks a period whose readings cannot be trusted — a meter
// reset, a negative delta, or a gap. Mirrors table `consumption_anomalies`
// (migration 00004).
//
// An unresolved anomaly is what stops a bill being issued silently over bad
// data; Resolution records which of the three outcomes was chosen.
type ConsumptionAnomaly struct {
	ID         uuid.UUID
	AnalyzerID uuid.UUID

	PeriodStart time.Time
	PeriodEnd   time.Time
	// Reason is 'meter_reset', 'negative_delta' or 'missing_readings'. It is
	// a plain text column in the schema, so it is a string here.
	Reason string
	Detail json.RawMessage

	ResolvedAt *time.Time
	ResolvedBy *uuid.UUID
	// Resolution is 'reset_registered', 'manual_override' or 'accepted'.
	Resolution *string
	// OverrideValues carries the manually supplied figures when Resolution is
	// 'manual_override'.
	OverrideValues json.RawMessage

	CreatedAt time.Time
}

// PlantProduction is one production sample for a plant, optionally broken down
// by device. Mirrors the `plant_production` hypertable (migration 00004),
// primary key (plant_id, ts, device_id).
//
// DeviceID is part of the primary key and therefore never null, even though
// the column carries a foreign key that would otherwise permit it: a
// plant-level sample with no device breakdown has no representable form here.
// That is a known schema constraint, recorded in Task 2's report.
type PlantProduction struct {
	PlantID  uuid.UUID
	Ts       time.Time
	DeviceID uuid.UUID

	ProductionKwh *decimal.Decimal
	ActivePowerKw *decimal.Decimal
	EfficiencyPct *decimal.Decimal
	IrradianceWm2 *decimal.Decimal
	ModuleTempC   *decimal.Decimal
	AmbientTempC  *decimal.Decimal

	// Source defaults to 'isolar' and is a plain text column.
	Source string
}

// ConsumptionBucket is one row of any of the four consumption continuous
// aggregates — hourly, daily, monthly, yearly (migration 00005). All four have
// identical columns, so they share one struct and the repository says which
// aggregate a value came from; duplicating the struct four times would invite
// them to drift.
//
// The Index fields are the closing register values within the bucket; the
// Consumption and Generation fields are the deltas across it. Both are
// materialised, so a caller must never re-derive one from the other.
type ConsumptionBucket struct {
	AnalyzerID uuid.UUID
	// Bucket is the bucket's start instant in UTC.
	Bucket time.Time

	ActiveImportStart *decimal.Decimal
	ActiveImportEnd   *decimal.Decimal

	ActiveConsumption     *decimal.Decimal
	InductiveConsumption  *decimal.Decimal
	CapacitiveConsumption *decimal.Decimal
	T1Consumption         *decimal.Decimal
	T2Consumption         *decimal.Decimal
	T3Consumption         *decimal.Decimal

	ActiveGeneration     *decimal.Decimal
	InductiveGeneration  *decimal.Decimal
	CapacitiveGeneration *decimal.Decimal
	T1Generation         *decimal.Decimal
	T2Generation         *decimal.Decimal
	T3Generation         *decimal.Decimal

	MaxDemandKw *decimal.Decimal

	ActiveIndex           *decimal.Decimal
	InductiveIndex        *decimal.Decimal
	CapacitiveIndex       *decimal.Decimal
	T1Index               *decimal.Decimal
	T2Index               *decimal.Decimal
	T3Index               *decimal.Decimal
	ActiveGenerationIndex *decimal.Decimal

	// ReadingCount is how many raw readings the bucket was built from. A
	// bucket with too few readings is suspect even when its arithmetic is
	// sound.
	ReadingCount int64
}

// PlantProductionBucket is one row of either plant production continuous
// aggregate — daily or monthly (migration 00005). Both have identical columns,
// for the same reason ConsumptionBucket is shared.
type PlantProductionBucket struct {
	PlantID uuid.UUID
	Bucket  time.Time

	ProductionKwh    *decimal.Decimal
	MaxActivePowerKw *decimal.Decimal
}

// Production totals bases (R279): which source filled a plant's day.
const (
	BasisPlantMeter  = "plant_meter"
	BasisInverterSum = "inverter_sum"
	BasisDailyTotal  = "daily_total"
)

// PlantProductionTotal is one plant-level interval (R276), table
// plant_production_totals; the plant aggregates read only this table.
type PlantProductionTotal struct {
	PlantID       uuid.UUID
	Ts            time.Time
	ProductionKwh *decimal.Decimal
	ActivePowerKw *decimal.Decimal
	Basis         string
}

// MarketPrice is the hourly piyasa takas fiyatı. Mirrors the
// `market_prices_hourly` hypertable (migration 00004), primary key (ts).
type MarketPrice struct {
	// Ts is the hour start. The column's semantics are Europe/Istanbul hours
	// stored as UTC instants, so a caller pricing a local day must convert
	// rather than truncate.
	Ts time.Time
	// PTF is TL/MWh, numeric(14,4) — note the unit: consumption is in kWh, so
	// pricing divides by 1000 somewhere, and doing that in float would be the
	// whole defect this type exists to avoid.
	PTF       decimal.Decimal
	FetchedAt time.Time
}

// YekdemMonthly is the monthly YEKDEM component. Mirrors table
// `yekdem_monthly` (migration 00004), primary key (year, month).
type YekdemMonthly struct {
	Year  int16
	Month int16
	// Value is TL/MWh, like MarketPrice.PTF.
	Value     decimal.Decimal
	FetchedAt time.Time
}

// Forecast is one predicted value for one analyzer and instant, as produced by
// one forecasting run. Mirrors the `forecasts` hypertable (migration 00004),
// primary key (analyzer_id, ts, generated_at) — so successive runs accumulate
// rather than overwrite, and a forecast can be scored against what actually
// happened.
type Forecast struct {
	AnalyzerID uuid.UUID
	Ts         time.Time
	// GeneratedAt identifies the run. It is part of the key.
	GeneratedAt  time.Time
	HorizonHours int32

	Median decimal.Decimal
	P10    *decimal.Decimal
	P90    *decimal.Decimal

	ModelID      string
	ModelVersion *string
}

// ForecastGap records a stretch a forecasting run could not cover. Mirrors
// table `forecast_gaps` (migration 00004).
type ForecastGap struct {
	ID          uuid.UUID
	AnalyzerID  uuid.UUID
	GeneratedAt time.Time

	GapStart     time.Time
	GapEnd       time.Time
	MissingHours int32
}
