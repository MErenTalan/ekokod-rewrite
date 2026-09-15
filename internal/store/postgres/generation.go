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

// GenerationRepository implements store.GenerationRepository.
//
// generation_anchors has no company_id — every method joins through
// analyzers, per repository.go's "ROWS WITHOUT company_id" header.
type GenerationRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewGenerationRepository wraps pool in the generated query set.
func NewGenerationRepository(pool *pgxpool.Pool) *GenerationRepository {
	return &GenerationRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.GenerationRepository = (*GenerationRepository)(nil)

// Anchor implements store.GenerationRepository.Anchor.
func (r *GenerationRepository) Anchor(ctx context.Context, s store.Scope, analyzerID uuid.UUID) (model.GenerationAnchor, error) {
	if !s.Valid() {
		return model.GenerationAnchor{}, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	row, err := r.q.GenerationAnchorGet(ctx, sqlcgen.GenerationAnchorGetParams{
		AnalyzerID:   analyzerID,
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return model.GenerationAnchor{}, pgerr.Translate(r.pool, "generation anchor get", err)
	}
	return f2genAnchorFromRow(row)
}

// SetAnchor implements store.GenerationRepository.SetAnchor.
//
// The analyzer's visibility is validated IN THE WRITE STATEMENT ITSELF
// (GenerationAnchorUpsert's `insert … select … from analyzers where …`), not
// by a Go-side Scope.AllowsBuilding pre-check against a.AnalyzerID — that
// would be using AllowsBuilding to validate a stored foreign key, which its
// own doc says it cannot do. RowsAffected == 0 means the select-from-
// analyzers source came back empty — the analyzer is not visible to s — and
// nothing was written.
func (r *GenerationRepository) SetAnchor(ctx context.Context, s store.Scope, a model.GenerationAnchor) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	affected, err := r.q.GenerationAnchorUpsert(ctx, sqlcgen.GenerationAnchorUpsertParams{
		AnchorTs:     timeseriesToTimestamptz(a.AnchorTs),
		ActiveExport: decimalToNumeric(a.ActiveExport),
		Source:       a.Source,
		UpdatedAt:    timeseriesToTimestamptz(f2genUpdatedAtOrNow(a.UpdatedAt)),
		AnalyzerID:   a.AnalyzerID,
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "generation anchor set", err)
	}
	if affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

// f2genUpdatedAtOrNow defaults a zero UpdatedAt to the current instant, so a
// caller that only fills in the anchor's own fields does not also have to
// think about a bookkeeping timestamp.
func f2genUpdatedAtOrNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}

// f2genAnchorFromRow converts a generated GenerationAnchor row to the domain
// model, through the one audited numeric pair
// (internal/store/postgres/numeric.go).
func f2genAnchorFromRow(row sqlcgen.GenerationAnchor) (model.GenerationAnchor, error) {
	activeExport, err := numericToDecimal(row.ActiveExport)
	if err != nil {
		return model.GenerationAnchor{}, err
	}
	return model.GenerationAnchor{
		AnalyzerID:   row.AnalyzerID,
		AnchorTs:     row.AnchorTs.Time,
		ActiveExport: activeExport,
		Source:       row.Source,
		UpdatedAt:    row.UpdatedAt.Time,
	}, nil
}
