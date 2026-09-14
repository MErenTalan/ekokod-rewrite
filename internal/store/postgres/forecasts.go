package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// ForecastRepository implements store.ForecastRepository.
//
// forecasts and forecast_gaps have no company_id — every method joins
// through analyzers.
type ForecastRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewForecastRepository wraps pool in the generated query set.
func NewForecastRepository(pool *pgxpool.Pool) *ForecastRepository {
	return &ForecastRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.ForecastRepository = (*ForecastRepository)(nil)

var forecastsStagingColumns = []string{
	"analyzer_id", "ts", "generated_at", "horizon_hours", "median", "p10", "p90", "model_id", "model_version",
}

const createForecastsStaging = `create temporary table forecasts_staging (
	analyzer_id   uuid        not null,
	ts            timestamptz not null,
	generated_at  timestamptz not null,
	horizon_hours integer     not null,
	median        numeric(18,4) not null,
	p10           numeric(18,4),
	p90           numeric(18,4),
	model_id      text        not null,
	model_version text
) on commit drop`

const upsertForecastsFromStaging = `insert into forecasts (
	analyzer_id, ts, generated_at, horizon_hours, median, p10, p90, model_id, model_version
)
select distinct on (analyzer_id, ts, generated_at)
	analyzer_id, ts, generated_at, horizon_hours, median, p10, p90, model_id, model_version
from forecasts_staging
order by analyzer_id, ts, generated_at
on conflict (analyzer_id, ts, generated_at) do update set
	horizon_hours = excluded.horizon_hours,
	median        = excluded.median,
	p10           = excluded.p10,
	p90           = excluded.p90,
	model_id      = excluded.model_id,
	model_version = excluded.model_version
returning (xmax = 0) as inserted`

// BulkInsert implements store.ForecastRepository.BulkInsert. Like
// ReadingRepository.BulkInsert, the staging table cannot appear in sqlc's
// schema catalogue, so the COPY-then-upsert is plain SQL here.
func (r *ForecastRepository) BulkInsert(ctx context.Context, s store.Scope, rows []model.Forecast) (inserted, updated int, err error) {
	if !s.Valid() {
		return 0, 0, store.ErrInvalidScope
	}
	if len(rows) == 0 {
		return 0, 0, nil
	}
	const op = "forecast bulk insert"
	buildingIDs, allBuildings := s.BuildingFilter()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	analyzerIDs := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		analyzerIDs[i] = row.AnalyzerID
	}
	distinctAnalyzerIDs := distinctUUIDs(analyzerIDs)

	visible, err := r.q.WithTx(tx).ForecastVisibleAnalyzerCount(ctx, sqlcgen.ForecastVisibleAnalyzerCountParams{
		AnalyzerIds:  distinctAnalyzerIDs,
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	if visible != int64(len(distinctAnalyzerIDs)) {
		return 0, 0, store.ErrNotFound
	}

	if _, err := tx.Exec(ctx, createForecastsStaging); err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}

	_, err = tx.CopyFrom(ctx, pgx.Identifier{"forecasts_staging"}, forecastsStagingColumns,
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			row := rows[i]
			return []any{
				row.AnalyzerID,
				row.Ts,
				row.GeneratedAt,
				row.HorizonHours,
				decimalToNumeric(row.Median),
				decimalPtrToNumeric(row.P10),
				decimalPtrToNumeric(row.P90),
				row.ModelID,
				row.ModelVersion,
			}, nil
		}))
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}

	upsertRows, err := tx.Query(ctx, upsertForecastsFromStaging)
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

// Range implements store.ForecastRepository.Range.
func (r *ForecastRepository) Range(ctx context.Context, s store.Scope, analyzerID uuid.UUID, tr store.TimeRange) ([]model.Forecast, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	const op = "forecast range"

	if err := r.requireAnalyzerVisible(ctx, s, analyzerID, op); err != nil {
		return nil, err
	}

	rows, err := r.q.ForecastRange(ctx, sqlcgen.ForecastRangeParams{
		AnalyzerID: analyzerID,
		FromTs:     toTimestamptz(tr.From),
		ToTs:       toTimestamptz(tr.To),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, op, err)
	}
	return forecastsFromRows(rows)
}

// LatestRun implements store.ForecastRepository.LatestRun.
func (r *ForecastRepository) LatestRun(ctx context.Context, s store.Scope, analyzerID uuid.UUID, tr store.TimeRange) ([]model.Forecast, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	const op = "forecast latest run"

	if err := r.requireAnalyzerVisible(ctx, s, analyzerID, op); err != nil {
		return nil, err
	}

	rows, err := r.q.ForecastLatestRun(ctx, sqlcgen.ForecastLatestRunParams{
		AnalyzerID: analyzerID,
		FromTs:     toTimestamptz(tr.From),
		ToTs:       toTimestamptz(tr.To),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, op, err)
	}
	return forecastsFromRows(rows)
}

// RecordGaps implements store.ForecastRepository.RecordGaps.
func (r *ForecastRepository) RecordGaps(ctx context.Context, s store.Scope, gaps []model.ForecastGap) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	if len(gaps) == 0 {
		return nil
	}
	const op = "forecast record gaps"
	buildingIDs, allBuildings := s.BuildingFilter()

	analyzerIDs := make([]uuid.UUID, len(gaps))
	for i, g := range gaps {
		analyzerIDs[i] = g.AnalyzerID
	}
	distinctAnalyzerIDs := distinctUUIDs(analyzerIDs)

	visible, err := r.q.ForecastVisibleAnalyzerCount(ctx, sqlcgen.ForecastVisibleAnalyzerCountParams{
		AnalyzerIds:  distinctAnalyzerIDs,
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return pgerr.Translate(r.pool, op, err)
	}
	if visible != int64(len(distinctAnalyzerIDs)) {
		return store.ErrNotFound
	}

	ids := make([]uuid.UUID, len(gaps))
	generatedAts := make([]pgtype.Timestamptz, len(gaps))
	gapStarts := make([]pgtype.Timestamptz, len(gaps))
	gapEnds := make([]pgtype.Timestamptz, len(gaps))
	missingHours := make([]int32, len(gaps))
	for i, g := range gaps {
		id := g.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		ids[i] = id
		generatedAts[i] = toTimestamptz(g.GeneratedAt)
		gapStarts[i] = toTimestamptz(g.GapStart)
		gapEnds[i] = toTimestamptz(g.GapEnd)
		missingHours[i] = g.MissingHours
	}

	err = r.q.ForecastInsertGaps(ctx, sqlcgen.ForecastInsertGapsParams{
		Ids:          ids,
		AnalyzerIds:  analyzerIDs,
		GeneratedAts: generatedAts,
		GapStarts:    gapStarts,
		GapEnds:      gapEnds,
		MissingHours: missingHours,
	})
	if err != nil {
		return pgerr.Translate(r.pool, op, err)
	}
	return nil
}

// Gaps implements store.ForecastRepository.Gaps.
func (r *ForecastRepository) Gaps(ctx context.Context, s store.Scope, analyzerID uuid.UUID, generatedAt time.Time) ([]model.ForecastGap, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	const op = "forecast gaps"

	if err := r.requireAnalyzerVisible(ctx, s, analyzerID, op); err != nil {
		return nil, err
	}

	rows, err := r.q.ForecastGapsByRun(ctx, sqlcgen.ForecastGapsByRunParams{
		AnalyzerID:  analyzerID,
		GeneratedAt: toTimestamptz(generatedAt),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, op, err)
	}
	out := make([]model.ForecastGap, len(rows))
	for i, row := range rows {
		out[i] = model.ForecastGap{
			ID:           row.ID,
			AnalyzerID:   row.AnalyzerID,
			GeneratedAt:  row.GeneratedAt.Time,
			GapStart:     row.GapStart.Time,
			GapEnd:       row.GapEnd.Time,
			MissingHours: row.MissingHours,
		}
	}
	return out, nil
}

func (r *ForecastRepository) requireAnalyzerVisible(ctx context.Context, s store.Scope, analyzerID uuid.UUID, op string) error {
	buildingIDs, allBuildings := s.BuildingFilter()
	visible, err := r.q.ForecastAnalyzerVisible(ctx, sqlcgen.ForecastAnalyzerVisibleParams{
		AnalyzerID:   analyzerID,
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return pgerr.Translate(r.pool, op, err)
	}
	if !visible {
		return store.ErrNotFound
	}
	return nil
}

func forecastsFromRows(rows []sqlcgen.Forecast) ([]model.Forecast, error) {
	out := make([]model.Forecast, len(rows))
	for i, row := range rows {
		median, err := numericToDecimal(row.Median)
		if err != nil {
			return nil, err
		}
		p10, err := numericToDecimalPtr(row.P10)
		if err != nil {
			return nil, err
		}
		p90, err := numericToDecimalPtr(row.P90)
		if err != nil {
			return nil, err
		}
		out[i] = model.Forecast{
			AnalyzerID:   row.AnalyzerID,
			Ts:           row.Ts.Time,
			GeneratedAt:  row.GeneratedAt.Time,
			HorizonHours: row.HorizonHours,
			Median:       median,
			P10:          p10,
			P90:          p90,
			ModelID:      row.ModelID,
			ModelVersion: row.ModelVersion,
		}
	}
	return out, nil
}
