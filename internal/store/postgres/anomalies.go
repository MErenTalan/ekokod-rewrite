package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// anomalyDefaultPageLimit and anomalyMaxPageLimit are AnomalyRepository's own
// Page defaults, per repository.go's Page doc: "Each repository documents
// its own default and its own cap."
const (
	anomalyDefaultPageLimit = 100
	anomalyMaxPageLimit     = 1000
)

// AnomalyRepository implements store.AnomalyRepository.
//
// consumption_anomalies has no company_id — every method joins through
// analyzers.
type AnomalyRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewAnomalyRepository wraps pool in the generated query set.
func NewAnomalyRepository(pool *pgxpool.Pool) *AnomalyRepository {
	return &AnomalyRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.AnomalyRepository = (*AnomalyRepository)(nil)

// Get implements store.AnomalyRepository.Get.
func (r *AnomalyRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.ConsumptionAnomaly, error) {
	if !s.Valid() {
		return model.ConsumptionAnomaly{}, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	row, err := r.q.AnomalyGet(ctx, sqlcgen.AnomalyGetParams{
		ID:           id,
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return model.ConsumptionAnomaly{}, pgerr.Translate(r.pool, "anomaly get", err)
	}
	return anomalyFromRow(row), nil
}

// List implements store.AnomalyRepository.List. An empty f.AnalyzerIDs means
// "every analyzer visible to the Scope" — the same never-widens-past-scope
// convention as CursorRepository.List.
func (r *AnomalyRepository) List(ctx context.Context, s store.Scope, f store.AnomalyFilter) ([]model.ConsumptionAnomaly, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if f.Range != nil && !f.Range.Valid() {
		return nil, store.ErrInvalidRange
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	var fromTs, toTs pgtype.Timestamptz
	if f.Range != nil {
		fromTs, toTs = toTimestamptz(f.Range.From), toTimestamptz(f.Range.To)
	}

	limit := f.Page.Limit
	if limit <= 0 {
		limit = anomalyDefaultPageLimit
	}
	if limit > anomalyMaxPageLimit {
		limit = anomalyMaxPageLimit
	}

	rows, err := r.q.AnomalyList(ctx, sqlcgen.AnomalyListParams{
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
		AnalyzerIds:  f.AnalyzerIDs,
		Unresolved:   f.Unresolved,
		Reason:       f.Reason,
		FromTs:       fromTs,
		ToTs:         toTs,
		PageLimit:    limit,
		PageOffset:   f.Page.Offset,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "anomaly list", err)
	}
	out := make([]model.ConsumptionAnomaly, len(rows))
	for i, row := range rows {
		out[i] = anomalyFromRow(row)
	}
	return out, nil
}

// Create implements store.AnomalyRepository.Create.
func (r *AnomalyRepository) Create(ctx context.Context, s store.Scope, a model.ConsumptionAnomaly) (model.ConsumptionAnomaly, error) {
	if !s.Valid() {
		return model.ConsumptionAnomaly{}, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	row, err := r.q.AnomalyCreate(ctx, sqlcgen.AnomalyCreateParams{
		AnalyzerID:   a.AnalyzerID,
		PeriodStart:  toTimestamptz(a.PeriodStart),
		PeriodEnd:    toTimestamptz(a.PeriodEnd),
		Reason:       a.Reason,
		Detail:       a.Detail,
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		translated := pgerr.Translate(r.pool, "anomaly create", err)
		return model.ConsumptionAnomaly{}, translated
	}
	// AnomalyCreate's source is `select a.id from analyzers a where …`: an
	// invisible analyzer makes the source empty and pgx.ErrNoRows is what a
	// :one query with an empty result reports — Translate already turned
	// that into store.ErrNotFound above.
	return anomalyFromRow(row), nil
}

// Resolve implements store.AnomalyRepository.Resolve.
//
// The check that resolvedBy is a user of s.CompanyID is folded into
// AnomalyResolve's own WHERE (an `exists(...)` against users), in the same
// single UPDATE statement as the write — not a separate pre-check that could
// race a concurrent change to resolvedBy's own company_id.
func (r *AnomalyRepository) Resolve(ctx context.Context, s store.Scope, id uuid.UUID, resolvedBy uuid.UUID, resolution string, overrides []byte, at time.Time) (model.ConsumptionAnomaly, error) {
	if !s.Valid() {
		return model.ConsumptionAnomaly{}, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()

	row, err := r.q.AnomalyResolve(ctx, sqlcgen.AnomalyResolveParams{
		ID:             id,
		At:             toTimestamptz(at),
		ResolvedBy:     &resolvedBy,
		Resolution:     resolution,
		OverrideValues: overrides,
		CompanyID:      s.CompanyID,
		AllBuildings:   allBuildings,
		BuildingIds:    buildingIDs,
	})
	if err != nil {
		return model.ConsumptionAnomaly{}, pgerr.Translate(r.pool, "anomaly resolve", err)
	}
	return anomalyFromRow(row), nil
}

func anomalyFromRow(row sqlcgen.ConsumptionAnomaly) model.ConsumptionAnomaly {
	var resolvedAt *time.Time
	if row.ResolvedAt.Valid {
		t := row.ResolvedAt.Time
		resolvedAt = &t
	}
	return model.ConsumptionAnomaly{
		ID:             row.ID,
		AnalyzerID:     row.AnalyzerID,
		PeriodStart:    row.PeriodStart.Time,
		PeriodEnd:      row.PeriodEnd.Time,
		Reason:         row.Reason,
		Detail:         row.Detail,
		ResolvedAt:     resolvedAt,
		ResolvedBy:     row.ResolvedBy,
		Resolution:     row.Resolution,
		OverrideValues: row.OverrideValues,
		CreatedAt:      row.CreatedAt.Time,
	}
}
