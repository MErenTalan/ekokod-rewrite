package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// AnalyticsRepository implements store.AnalyticsRepository: the MATERIALISED
// reads over the six continuous aggregates (migration 00005), for dashboards
// and reports. This is deliberately NOT the billing-grade surface — that is
// ReadingRepository.BoundaryReadings, which reads meter_readings directly —
// and the two live in different files with different names so the
// distinction 04-data-model.md §4.3 requires is impossible to miss in code.
//
// consumption_hourly/daily/monthly/yearly have no company_id and join
// through analyzers; plant_production_daily/monthly have no company_id and
// join through power_plants, which itself has no building_id.
//
// consumption_monthly, consumption_yearly and plant_production_monthly are
// materialized_only = true (migration 00005): the bucket currently in
// progress is ABSENT from them, not stale. A caller wanting month-to-date or
// year-to-date must compose the closed buckets these methods return with the
// open period read from consumption_daily/meter_readings — this repository
// does not do that composition, and callers must not "fix" a missing
// current-period row by asking migration 00005 to flip materialized_only.
//
// Bucket boundaries for every view coarser than hourly (Daily, Monthly,
// Yearly, on both the consumption and production sides) are
// Europe/Istanbul-LOCAL instants, not UTC — see each method's own doc
// comment and TestAnalyticsConsumptionDailyBucketsInIstanbulNotUTC.
// consumption_hourly/plant_production_daily's hourly granularity has no
// timezone dependence (an hour is an hour everywhere).
//
// analyzerIDs/plantIDs on every method here are REQUIRED POSITIONAL
// parameters: an empty or nil slice means NO ROWS, fail-closed, identical to
// Scope.BuildingIDs — there is no "every id visible to scope" form (this
// differs from AnomalyFilter.AnalyzerIDs, which is an optional FILTER-STRUCT
// field where empty means "no narrowing"; the two conventions are
// deliberately different and repository.go documents which is which).
type AnalyticsRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewAnalyticsRepository wraps pool in the generated query set.
func NewAnalyticsRepository(pool *pgxpool.Pool) *AnalyticsRepository {
	return &AnalyticsRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.AnalyticsRepository = (*AnalyticsRepository)(nil)

// ConsumptionHourly implements store.AnalyticsRepository.ConsumptionHourly.
func (r *AnalyticsRepository) ConsumptionHourly(ctx context.Context, s store.Scope, analyzerIDs []uuid.UUID, tr store.TimeRange) ([]model.ConsumptionBucket, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	rows, err := r.q.AnalyticsConsumptionHourly(ctx, sqlcgen.AnalyticsConsumptionHourlyParams{
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
		AnalyzerIds:  analyzerIDs,
		FromTs:       timeseriesToTimestamptz(tr.From),
		ToTs:         timeseriesToTimestamptz(tr.To),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "analytics consumption hourly", err)
	}
	out := make([]model.ConsumptionBucket, len(rows))
	for i, row := range rows {
		b, err := consumptionBucketFromHourly(row)
		if err != nil {
			return nil, pgerr.Translate(r.pool, "analytics consumption hourly", err)
		}
		out[i] = b
	}
	return out, nil
}

// ConsumptionDaily implements store.AnalyticsRepository.ConsumptionDaily.
//
// Buckets are Europe/Istanbul-LOCAL instants (migration 00005), not UTC: a
// day here runs midnight-to-midnight Istanbul time. tr.From/tr.To are
// compared directly against that bucket instant, so a caller that builds
// them from UTC calendar-day boundaries sees this window's own edges land
// mid-bucket rather than aligned to it — see
// TestAnalyticsConsumptionDailyBucketsInIstanbulNotUTC.
func (r *AnalyticsRepository) ConsumptionDaily(ctx context.Context, s store.Scope, analyzerIDs []uuid.UUID, tr store.TimeRange) ([]model.ConsumptionBucket, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	rows, err := r.q.AnalyticsConsumptionDaily(ctx, sqlcgen.AnalyticsConsumptionDailyParams{
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
		AnalyzerIds:  analyzerIDs,
		FromTs:       timeseriesToTimestamptz(tr.From),
		ToTs:         timeseriesToTimestamptz(tr.To),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "analytics consumption daily", err)
	}
	out := make([]model.ConsumptionBucket, len(rows))
	for i, row := range rows {
		b, err := consumptionBucketFromDaily(row)
		if err != nil {
			return nil, pgerr.Translate(r.pool, "analytics consumption daily", err)
		}
		out[i] = b
	}
	return out, nil
}

// ConsumptionMonthly implements store.AnalyticsRepository.ConsumptionMonthly.
// The open month is ABSENT from consumption_monthly (materialized_only =
// true): see this file's header.
func (r *AnalyticsRepository) ConsumptionMonthly(ctx context.Context, s store.Scope, analyzerIDs []uuid.UUID, tr store.TimeRange) ([]model.ConsumptionBucket, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	rows, err := r.q.AnalyticsConsumptionMonthly(ctx, sqlcgen.AnalyticsConsumptionMonthlyParams{
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
		AnalyzerIds:  analyzerIDs,
		FromTs:       timeseriesToTimestamptz(tr.From),
		ToTs:         timeseriesToTimestamptz(tr.To),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "analytics consumption monthly", err)
	}
	out := make([]model.ConsumptionBucket, len(rows))
	for i, row := range rows {
		b, err := consumptionBucketFromMonthly(row)
		if err != nil {
			return nil, pgerr.Translate(r.pool, "analytics consumption monthly", err)
		}
		out[i] = b
	}
	return out, nil
}

// ConsumptionYearly implements store.AnalyticsRepository.ConsumptionYearly.
// The open year is ABSENT from consumption_yearly (materialized_only =
// true): see this file's header.
func (r *AnalyticsRepository) ConsumptionYearly(ctx context.Context, s store.Scope, analyzerIDs []uuid.UUID, tr store.TimeRange) ([]model.ConsumptionBucket, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	rows, err := r.q.AnalyticsConsumptionYearly(ctx, sqlcgen.AnalyticsConsumptionYearlyParams{
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
		AnalyzerIds:  analyzerIDs,
		FromTs:       timeseriesToTimestamptz(tr.From),
		ToTs:         timeseriesToTimestamptz(tr.To),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "analytics consumption yearly", err)
	}
	out := make([]model.ConsumptionBucket, len(rows))
	for i, row := range rows {
		b, err := consumptionBucketFromYearly(row)
		if err != nil {
			return nil, pgerr.Translate(r.pool, "analytics consumption yearly", err)
		}
		out[i] = b
	}
	return out, nil
}

// ProductionDaily implements store.AnalyticsRepository.ProductionDaily.
func (r *AnalyticsRepository) ProductionDaily(ctx context.Context, s store.Scope, plantIDs []uuid.UUID, tr store.TimeRange) ([]model.PlantProductionBucket, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}

	rows, err := r.q.AnalyticsProductionDaily(ctx, sqlcgen.AnalyticsProductionDailyParams{
		CompanyID: s.CompanyID,
		PlantIds:  plantIDs,
		FromTs:    timeseriesToTimestamptz(tr.From),
		ToTs:      timeseriesToTimestamptz(tr.To),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "analytics production daily", err)
	}
	out := make([]model.PlantProductionBucket, len(rows))
	for i, row := range rows {
		b, err := productionBucketFromDaily(row)
		if err != nil {
			return nil, pgerr.Translate(r.pool, "analytics production daily", err)
		}
		out[i] = b
	}
	return out, nil
}

// ProductionMonthly implements store.AnalyticsRepository.ProductionMonthly.
// The open month is ABSENT from plant_production_monthly (materialized_only =
// true): see this file's header.
func (r *AnalyticsRepository) ProductionMonthly(ctx context.Context, s store.Scope, plantIDs []uuid.UUID, tr store.TimeRange) ([]model.PlantProductionBucket, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}

	rows, err := r.q.AnalyticsProductionMonthly(ctx, sqlcgen.AnalyticsProductionMonthlyParams{
		CompanyID: s.CompanyID,
		PlantIds:  plantIDs,
		FromTs:    timeseriesToTimestamptz(tr.From),
		ToTs:      timeseriesToTimestamptz(tr.To),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "analytics production monthly", err)
	}
	out := make([]model.PlantProductionBucket, len(rows))
	for i, row := range rows {
		b, err := productionBucketFromMonthly(row)
		if err != nil {
			return nil, pgerr.Translate(r.pool, "analytics production monthly", err)
		}
		out[i] = b
	}
	return out, nil
}

// The four consumption_* views share one column set, so one converter body
// would do — but each takes a distinct generated struct type (ConsumptionHourly,
// ConsumptionDaily, ConsumptionMonthly, ConsumptionYearly), and Go has no way
// to write that once without reflection this codebase does not otherwise use.
// Four short, identical-shaped functions are the honest cost of that.

func consumptionBucketFromHourly(row sqlcgen.ConsumptionHourly) (model.ConsumptionBucket, error) {
	return newConsumptionBucket(
		row.AnalyzerID, row.Bucket, row.ActiveImportStart, row.ActiveImportEnd,
		row.ActiveConsumption, row.InductiveConsumption, row.CapacitiveConsumption,
		row.T1Consumption, row.T2Consumption, row.T3Consumption,
		row.ActiveGeneration, row.InductiveGeneration, row.CapacitiveGeneration,
		row.T1Generation, row.T2Generation, row.T3Generation,
		row.MaxDemandKw, row.ActiveIndex, row.InductiveIndex, row.CapacitiveIndex,
		row.T1Index, row.T2Index, row.T3Index, row.ActiveGenerationIndex, row.ReadingCount,
	)
}

func consumptionBucketFromDaily(row sqlcgen.ConsumptionDaily) (model.ConsumptionBucket, error) {
	return newConsumptionBucket(
		row.AnalyzerID, row.Bucket, row.ActiveImportStart, row.ActiveImportEnd,
		row.ActiveConsumption, row.InductiveConsumption, row.CapacitiveConsumption,
		row.T1Consumption, row.T2Consumption, row.T3Consumption,
		row.ActiveGeneration, row.InductiveGeneration, row.CapacitiveGeneration,
		row.T1Generation, row.T2Generation, row.T3Generation,
		row.MaxDemandKw, row.ActiveIndex, row.InductiveIndex, row.CapacitiveIndex,
		row.T1Index, row.T2Index, row.T3Index, row.ActiveGenerationIndex, row.ReadingCount,
	)
}

func consumptionBucketFromMonthly(row sqlcgen.ConsumptionMonthly) (model.ConsumptionBucket, error) {
	return newConsumptionBucket(
		row.AnalyzerID, row.Bucket, row.ActiveImportStart, row.ActiveImportEnd,
		row.ActiveConsumption, row.InductiveConsumption, row.CapacitiveConsumption,
		row.T1Consumption, row.T2Consumption, row.T3Consumption,
		row.ActiveGeneration, row.InductiveGeneration, row.CapacitiveGeneration,
		row.T1Generation, row.T2Generation, row.T3Generation,
		row.MaxDemandKw, row.ActiveIndex, row.InductiveIndex, row.CapacitiveIndex,
		row.T1Index, row.T2Index, row.T3Index, row.ActiveGenerationIndex, row.ReadingCount,
	)
}

func consumptionBucketFromYearly(row sqlcgen.ConsumptionYearly) (model.ConsumptionBucket, error) {
	return newConsumptionBucket(
		row.AnalyzerID, row.Bucket, row.ActiveImportStart, row.ActiveImportEnd,
		row.ActiveConsumption, row.InductiveConsumption, row.CapacitiveConsumption,
		row.T1Consumption, row.T2Consumption, row.T3Consumption,
		row.ActiveGeneration, row.InductiveGeneration, row.CapacitiveGeneration,
		row.T1Generation, row.T2Generation, row.T3Generation,
		row.MaxDemandKw, row.ActiveIndex, row.InductiveIndex, row.CapacitiveIndex,
		row.T1Index, row.T2Index, row.T3Index, row.ActiveGenerationIndex, row.ReadingCount,
	)
}

func newConsumptionBucket(
	analyzerID uuid.UUID, bucket pgtype.Timestamptz,
	activeImportStart, activeImportEnd,
	activeConsumption, inductiveConsumption, capacitiveConsumption,
	t1Consumption, t2Consumption, t3Consumption,
	activeGeneration, inductiveGeneration, capacitiveGeneration,
	t1Generation, t2Generation, t3Generation,
	maxDemandKw, activeIndex, inductiveIndex, capacitiveIndex,
	t1Index, t2Index, t3Index, activeGenerationIndex pgtype.Numeric,
	readingCount int64,
) (model.ConsumptionBucket, error) {
	fields := []*pgtype.Numeric{
		&activeImportStart, &activeImportEnd,
		&activeConsumption, &inductiveConsumption, &capacitiveConsumption,
		&t1Consumption, &t2Consumption, &t3Consumption,
		&activeGeneration, &inductiveGeneration, &capacitiveGeneration,
		&t1Generation, &t2Generation, &t3Generation,
		&maxDemandKw, &activeIndex, &inductiveIndex, &capacitiveIndex,
		&t1Index, &t2Index, &t3Index, &activeGenerationIndex,
	}
	converted := make([]*decimal.Decimal, len(fields))
	for i, f := range fields {
		d, err := numericToDecimalPtr(*f)
		if err != nil {
			return model.ConsumptionBucket{}, err
		}
		converted[i] = d
	}
	return model.ConsumptionBucket{
		AnalyzerID:            analyzerID,
		Bucket:                bucket.Time,
		ActiveImportStart:     converted[0],
		ActiveImportEnd:       converted[1],
		ActiveConsumption:     converted[2],
		InductiveConsumption:  converted[3],
		CapacitiveConsumption: converted[4],
		T1Consumption:         converted[5],
		T2Consumption:         converted[6],
		T3Consumption:         converted[7],
		ActiveGeneration:      converted[8],
		InductiveGeneration:   converted[9],
		CapacitiveGeneration:  converted[10],
		T1Generation:          converted[11],
		T2Generation:          converted[12],
		T3Generation:          converted[13],
		MaxDemandKw:           converted[14],
		ActiveIndex:           converted[15],
		InductiveIndex:        converted[16],
		CapacitiveIndex:       converted[17],
		T1Index:               converted[18],
		T2Index:               converted[19],
		T3Index:               converted[20],
		ActiveGenerationIndex: converted[21],
		ReadingCount:          readingCount,
	}, nil
}

func productionBucketFromDaily(row sqlcgen.PlantProductionDaily) (model.PlantProductionBucket, error) {
	return newPlantProductionBucket(row.PlantID, row.Bucket, row.ProductionKwh, row.MaxActivePowerKw, row.AvgEfficiencyPct)
}

func productionBucketFromMonthly(row sqlcgen.PlantProductionMonthly) (model.PlantProductionBucket, error) {
	return newPlantProductionBucket(row.PlantID, row.Bucket, row.ProductionKwh, row.MaxActivePowerKw, row.AvgEfficiencyPct)
}

func newPlantProductionBucket(plantID uuid.UUID, bucket pgtype.Timestamptz, productionKwh, maxActivePowerKw, avgEfficiencyPct pgtype.Numeric) (model.PlantProductionBucket, error) {
	pKwh, err := numericToDecimalPtr(productionKwh)
	if err != nil {
		return model.PlantProductionBucket{}, err
	}
	maxKw, err := numericToDecimalPtr(maxActivePowerKw)
	if err != nil {
		return model.PlantProductionBucket{}, err
	}
	avgEff, err := numericToDecimalPtr(avgEfficiencyPct)
	if err != nil {
		return model.PlantProductionBucket{}, err
	}
	return model.PlantProductionBucket{
		PlantID:          plantID,
		Bucket:           bucket.Time,
		ProductionKwh:    pKwh,
		MaxActivePowerKw: maxKw,
		AvgEfficiencyPct: avgEff,
	}, nil
}
