package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// CursorRepository implements store.CursorRepository.
//
// ingestion_cursors has no company_id — every method joins through
// analyzers, per repository.go's "ROWS WITHOUT company_id" header.
type CursorRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewCursorRepository wraps pool in the generated query set.
func NewCursorRepository(pool *pgxpool.Pool) *CursorRepository {
	return &CursorRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.CursorRepository = (*CursorRepository)(nil)

// Get implements store.CursorRepository.Get.
func (r *CursorRepository) Get(ctx context.Context, s store.Scope, analyzerID uuid.UUID, kind model.ReadingKind) (model.IngestionCursor, error) {
	if !s.Valid() {
		return model.IngestionCursor{}, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	row, err := r.q.CursorGet(ctx, sqlcgen.CursorGetParams{
		AnalyzerID:   analyzerID,
		Kind:         sqlcgen.ReadingKind(kind),
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return model.IngestionCursor{}, pgerr.Translate(r.pool, "cursor get", err)
	}
	return cursorFromRow(row), nil
}

// List implements store.CursorRepository.List. An empty analyzerIDs means
// "every analyzer visible to the Scope" — it narrows the scope's own join
// predicate, it never widens past it (the BuildingFilter.IDs convention).
func (r *CursorRepository) List(ctx context.Context, s store.Scope, analyzerIDs []uuid.UUID) ([]model.IngestionCursor, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	rows, err := r.q.CursorList(ctx, sqlcgen.CursorListParams{
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
		AnalyzerIds:  analyzerIDs,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "cursor list", err)
	}
	out := make([]model.IngestionCursor, len(rows))
	for i, row := range rows {
		out[i] = cursorFromRow(row)
	}
	return out, nil
}

// RecordSuccess implements store.CursorRepository.RecordSuccess.
func (r *CursorRepository) RecordSuccess(ctx context.Context, s store.Scope, analyzerID uuid.UUID, kind model.ReadingKind, lastTs, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	affected, err := r.q.CursorRecordSuccess(ctx, sqlcgen.CursorRecordSuccessParams{
		AnalyzerID:   analyzerID,
		Kind:         sqlcgen.ReadingKind(kind),
		LastTs:       toTimestamptz(lastTs),
		At:           toTimestamptz(at),
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "cursor record success", err)
	}
	if affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

// RecordFailure implements store.CursorRepository.RecordFailure.
func (r *CursorRepository) RecordFailure(ctx context.Context, s store.Scope, analyzerID uuid.UUID, kind model.ReadingKind, message string, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	affected, err := r.q.CursorRecordFailure(ctx, sqlcgen.CursorRecordFailureParams{
		AnalyzerID:   analyzerID,
		Kind:         sqlcgen.ReadingKind(kind),
		Message:      message,
		At:           toTimestamptz(at),
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "cursor record failure", err)
	}
	if affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func cursorFromRow(row sqlcgen.IngestionCursor) model.IngestionCursor {
	var lastTs, lastSuccessAt, lastErrorAt *time.Time
	if row.LastTs.Valid {
		t := row.LastTs.Time
		lastTs = &t
	}
	if row.LastSuccessAt.Valid {
		t := row.LastSuccessAt.Time
		lastSuccessAt = &t
	}
	if row.LastErrorAt.Valid {
		t := row.LastErrorAt.Time
		lastErrorAt = &t
	}
	return model.IngestionCursor{
		AnalyzerID:          row.AnalyzerID,
		Kind:                model.ReadingKind(row.Kind),
		LastTs:              lastTs,
		LastSuccessAt:       lastSuccessAt,
		LastError:           row.LastError,
		LastErrorAt:         lastErrorAt,
		ConsecutiveFailures: row.ConsecutiveFailures,
	}
}
