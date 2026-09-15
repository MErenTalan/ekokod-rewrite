package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// ProductionRepository implements store.ProductionRepository.
//
// plant_production has no company_id — every method joins through
// power_plants, which itself has no building_id: a Scope narrows a plant to
// the company and no further (repository.go's PlantRepository doc).
//
// plant_production.device_id is NOT NULL even though the spec's prose treats
// it as nullable: the column is part of the primary key, and PostgreSQL does
// not allow a nullable column inside a primary key. A plant-level sample
// with no device breakdown is therefore unstorable today — a known schema
// constraint (04-data-model.md §4.5, recorded in Task 2's and Task 10's
// reports) rather than something this repository papers over. Every row
// BulkInsert writes must therefore name a real device of its plant, and the
// isolation check below enforces exactly that.
type ProductionRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewProductionRepository wraps pool in the generated query set.
func NewProductionRepository(pool *pgxpool.Pool) *ProductionRepository {
	return &ProductionRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.ProductionRepository = (*ProductionRepository)(nil)

var plantProductionStagingColumns = []string{
	"plant_id", "ts", "device_id",
	"production_kwh", "active_power_kw", "efficiency_pct",
	"irradiance_wm2", "module_temp_c", "ambient_temp_c", "source",
}

const createPlantProductionStaging = `create temporary table plant_production_staging (
	plant_id        uuid          not null,
	ts              timestamptz   not null,
	device_id       uuid          not null,
	production_kwh  numeric(16,4),
	active_power_kw numeric(14,4),
	efficiency_pct  numeric(6,3),
	irradiance_wm2  numeric(10,3),
	module_temp_c   numeric(6,2),
	ambient_temp_c  numeric(6,2),
	source          text not null default 'isolar'
) on commit drop`

const upsertPlantProductionFromStaging = `insert into plant_production (
	plant_id, ts, device_id,
	production_kwh, active_power_kw, efficiency_pct,
	irradiance_wm2, module_temp_c, ambient_temp_c, source
)
select distinct on (plant_id, ts, device_id)
	plant_id, ts, device_id,
	production_kwh, active_power_kw, efficiency_pct,
	irradiance_wm2, module_temp_c, ambient_temp_c, source
from plant_production_staging
order by plant_id, ts, device_id
on conflict (plant_id, ts, device_id) do update set
	production_kwh  = excluded.production_kwh,
	active_power_kw = excluded.active_power_kw,
	efficiency_pct  = excluded.efficiency_pct,
	irradiance_wm2  = excluded.irradiance_wm2,
	module_temp_c   = excluded.module_temp_c,
	ambient_temp_c  = excluded.ambient_temp_c,
	source          = excluded.source
returning (xmax = 0) as inserted`

// BulkInsert implements store.ProductionRepository.BulkInsert. Like
// ReadingRepository.BulkInsert, the staging table cannot appear in sqlc's
// schema catalogue, so the COPY-then-upsert is plain SQL here; the two
// visibility checks that guard it (plant, then device-belongs-to-plant) are
// generated queries.
func (r *ProductionRepository) BulkInsert(ctx context.Context, s store.Scope, rows []model.PlantProduction) (inserted, updated int, err error) {
	if !s.Valid() {
		return 0, 0, store.ErrInvalidScope
	}
	if len(rows) == 0 {
		return 0, 0, nil
	}
	const op = "production bulk insert"

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	plantIDs := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		plantIDs[i] = row.PlantID
	}
	distinctPlantIDs := distinctUUIDs(plantIDs)

	// `for share` locks every visible plant row for the rest of this
	// transaction — see ReadingRepository.BulkInsert's identical comment.
	visiblePlantIDs, err := r.q.WithTx(tx).ProductionVisiblePlantIDs(ctx, sqlcgen.ProductionVisiblePlantIDsParams{
		PlantIds:  distinctPlantIDs,
		CompanyID: s.CompanyID,
	})
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	if len(visiblePlantIDs) != len(distinctPlantIDs) {
		return 0, 0, store.ErrNotFound
	}

	pairPlantIDs, pairDeviceIDs := distinctPlantDevicePairs(rows)
	validPairs, err := r.q.WithTx(tx).ProductionValidDevicePairCount(ctx, sqlcgen.ProductionValidDevicePairCountParams{
		PlantIds:  pairPlantIDs,
		DeviceIds: pairDeviceIDs,
	})
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	if validPairs != int64(len(pairPlantIDs)) {
		// At least one row's device_id is not a device of that row's
		// plant_id: the whole batch is refused.
		return 0, 0, store.ErrNotFound
	}

	if _, err := tx.Exec(ctx, createPlantProductionStaging); err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}

	_, err = tx.CopyFrom(ctx, pgx.Identifier{"plant_production_staging"}, plantProductionStagingColumns,
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			row := rows[i]
			return []any{
				row.PlantID,
				row.Ts,
				row.DeviceID,
				decimalPtrToNumeric(row.ProductionKwh),
				decimalPtrToNumeric(row.ActivePowerKw),
				decimalPtrToNumeric(row.EfficiencyPct),
				decimalPtrToNumeric(row.IrradianceWm2),
				decimalPtrToNumeric(row.ModuleTempC),
				decimalPtrToNumeric(row.AmbientTempC),
				row.Source,
			}, nil
		}))
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}

	upsertRows, err := tx.Query(ctx, upsertPlantProductionFromStaging)
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	inserted, updated, err = scanUpsertCounts(upsertRows)
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	return inserted, updated, nil
}

// Range implements store.ProductionRepository.Range. Like
// ReadingRepository.Range, the scope is carried by ProductionRange's OWN
// join through power_plants; requirePlantVisible is consulted only to choose
// an error once that query has already come back empty.
func (r *ProductionRepository) Range(ctx context.Context, s store.Scope, plantID uuid.UUID, tr store.TimeRange) ([]model.PlantProduction, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	const op = "production range"

	rows, err := r.q.ProductionRange(ctx, sqlcgen.ProductionRangeParams{
		PlantID:   plantID,
		FromTs:    toTimestamptz(tr.From),
		ToTs:      toTimestamptz(tr.To),
		CompanyID: s.CompanyID,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, op, err)
	}
	if len(rows) == 0 {
		if err := r.requirePlantVisible(ctx, s, plantID, op); err != nil {
			return nil, err
		}
		return []model.PlantProduction{}, nil
	}
	out := make([]model.PlantProduction, len(rows))
	for i, row := range rows {
		pp, err := productionFromRow(row)
		if err != nil {
			return nil, pgerr.Translate(r.pool, op, err)
		}
		out[i] = pp
	}
	return out, nil
}

// Latest implements store.ProductionRepository.Latest.
func (r *ProductionRepository) Latest(ctx context.Context, s store.Scope, plantID uuid.UUID, tr store.TimeRange) (*model.PlantProduction, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	const op = "production latest"

	row, err := r.q.ProductionLatest(ctx, sqlcgen.ProductionLatestParams{
		PlantID:   plantID,
		FromTs:    toTimestamptz(tr.From),
		ToTs:      toTimestamptz(tr.To),
		CompanyID: s.CompanyID,
	})
	if err != nil {
		translated := pgerr.Translate(r.pool, op, err)
		if !isNotFound(translated) {
			return nil, translated
		}
		if err := r.requirePlantVisible(ctx, s, plantID, op); err != nil {
			return nil, err
		}
		return nil, nil
	}
	pp, err := productionFromRow(row)
	if err != nil {
		return nil, pgerr.Translate(r.pool, op, err)
	}
	return &pp, nil
}

// requirePlantVisible exists only to choose an error after Range/Latest's own
// scoped query has already come back empty — see
// ReadingRepository.requireAnalyzerVisible's identical comment.
func (r *ProductionRepository) requirePlantVisible(ctx context.Context, s store.Scope, plantID uuid.UUID, op string) error {
	visible, err := r.q.ProductionPlantVisible(ctx, sqlcgen.ProductionPlantVisibleParams{
		PlantID:   plantID,
		CompanyID: s.CompanyID,
	})
	if err != nil {
		return pgerr.Translate(r.pool, op, err)
	}
	if !visible {
		return store.ErrNotFound
	}
	return nil
}

// distinctPlantDevicePairs returns the batch's distinct (plant_id, device_id)
// pairs as two parallel arrays, which is the shape
// ProductionValidDevicePairCount's unnest(...) zip expects.
func distinctPlantDevicePairs(rows []model.PlantProduction) (plantIDs, deviceIDs []uuid.UUID) {
	type pair struct{ plant, device uuid.UUID }
	seen := make(map[pair]struct{}, len(rows))
	plantIDs = make([]uuid.UUID, 0, len(rows))
	deviceIDs = make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		p := pair{row.PlantID, row.DeviceID}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		plantIDs = append(plantIDs, row.PlantID)
		deviceIDs = append(deviceIDs, row.DeviceID)
	}
	return plantIDs, deviceIDs
}

func productionFromRow(row sqlcgen.PlantProduction) (model.PlantProduction, error) {
	productionKwh, err := numericToDecimalPtr(row.ProductionKwh)
	if err != nil {
		return model.PlantProduction{}, err
	}
	activePowerKw, err := numericToDecimalPtr(row.ActivePowerKw)
	if err != nil {
		return model.PlantProduction{}, err
	}
	efficiencyPct, err := numericToDecimalPtr(row.EfficiencyPct)
	if err != nil {
		return model.PlantProduction{}, err
	}
	irradianceWm2, err := numericToDecimalPtr(row.IrradianceWm2)
	if err != nil {
		return model.PlantProduction{}, err
	}
	moduleTempC, err := numericToDecimalPtr(row.ModuleTempC)
	if err != nil {
		return model.PlantProduction{}, err
	}
	ambientTempC, err := numericToDecimalPtr(row.AmbientTempC)
	if err != nil {
		return model.PlantProduction{}, err
	}
	return model.PlantProduction{
		PlantID:       row.PlantID,
		Ts:            row.Ts.Time,
		DeviceID:      row.DeviceID,
		ProductionKwh: productionKwh,
		ActivePowerKw: activePowerKw,
		EfficiencyPct: efficiencyPct,
		IrradianceWm2: irradianceWm2,
		ModuleTempC:   moduleTempC,
		AmbientTempC:  ambientTempC,
		Source:        row.Source,
	}, nil
}
