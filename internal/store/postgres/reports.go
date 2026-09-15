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

// reportDefaultPageLimit and reportMaxPageLimit are this file's Page
// defaults.
const (
	reportDefaultPageLimit = 50
	reportMaxPageLimit     = 500
)

func reportPageLimits(p store.Page) (limit, offset int32) {
	limit = p.Limit
	if limit <= 0 {
		limit = reportDefaultPageLimit
	}
	if limit > reportMaxPageLimit {
		limit = reportMaxPageLimit
	}
	offset = p.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// ReportRepository implements store.ReportRepository.
//
// reports.building_id is NOT NULL, so every row is scoped exactly as
// BuildingRepository scopes buildings: company_id = s.CompanyID and the
// Scope's building branch.
type ReportRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewReportRepository builds a ReportRepository over pool.
func NewReportRepository(pool *pgxpool.Pool) *ReportRepository {
	return &ReportRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.ReportRepository = (*ReportRepository)(nil)

// Get returns one report by id, scoped to the company and its visible buildings.
func (r *ReportRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.Report, error) {
	if !s.Valid() {
		return model.Report{}, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.ReportGet(ctx, sqlcgen.ReportGetParams{ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids})
	if err != nil {
		return model.Report{}, pgerr.Translate(r.pool, "get report", err)
	}
	return reportFromRow(row), nil
}

// List returns the Scope's visible reports matching f.
func (r *ReportRepository) List(ctx context.Context, s store.Scope, f store.ReportFilter) ([]model.Report, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	limit, offset := reportPageLimits(f.Page)
	var reportType sqlcgen.NullReportType
	if f.Type != nil {
		reportType = sqlcgen.NullReportType{ReportType: sqlcgen.ReportType(*f.Type), Valid: true}
	}
	statuses := make([]string, 0, len(f.Statuses))
	for _, st := range f.Statuses {
		statuses = append(statuses, string(st))
	}
	rows, err := r.q.ReportList(ctx, sqlcgen.ReportListParams{
		CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids, BuildingID: f.BuildingID,
		ReportType: reportType, Period: f.Period, Statuses: statuses, LimitVal: limit, OffsetVal: offset,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list reports", err)
	}
	out := make([]model.Report, 0, len(rows))
	for _, row := range rows {
		out = append(out, reportFromRow(row))
	}
	return out, nil
}

// Upsert is keyed on (building_id, type, period): regenerating a period
// REPLACES its report rather than accumulating a second one.
func (r *ReportRepository) Upsert(ctx context.Context, s store.Scope, rp model.Report) (model.Report, error) {
	if !s.Valid() {
		return model.Report{}, store.ErrInvalidScope
	}
	if rp.CompanyID != uuid.Nil && rp.CompanyID != s.CompanyID {
		return model.Report{}, store.ErrNotFound
	}
	// Fast pre-check only — see Scope.AllowsBuilding's doc comment and
	// Critical Finding 1 (task-11a fix round 1): an AllBuildings Scope
	// answers true here for ANY building id, including another tenant's.
	// ReportUpsert validates the stored building_id itself, in SQL, against
	// this Scope's company, deleted_at and building branch, so the write is
	// refused even if this check were skipped entirely.
	if !s.AllowsBuilding(rp.BuildingID) {
		return model.Report{}, store.ErrNotFound
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.ReportUpsert(ctx, sqlcgen.ReportUpsertParams{
		CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids, BuildingID: rp.BuildingID, ReportType: sqlcgen.ReportType(rp.Type),
		Period: rp.Period, PlantSelection: sqlcgen.PlantSelection(rp.PlantSelection), Payload: rp.Payload,
		PdfPath: rp.PdfPath, ExcelPath: rp.ExcelPath, EmailSubject: rp.EmailSubject, EmailBody: rp.EmailBody,
		Status: sqlcgen.ReportStatus(rp.Status), ErrorMessage: rp.ErrorMessage,
		ProcessedAt: tariffNullableTimestamptz(rp.ProcessedAt), CreatedAt: tariffTimestamptz(rp.CreatedAt),
	})
	if err != nil {
		return model.Report{}, pgerr.Translate(r.pool, "upsert report", err)
	}
	return reportFromRow(row), nil
}

// UpdateStatus records completion or failure. errorMessage must already be
// scrubbed by the caller: it becomes operator-facing text stored verbatim.
func (r *ReportRepository) UpdateStatus(ctx context.Context, s store.Scope, id uuid.UUID, status model.ReportStatus, errorMessage *string, at time.Time) (model.Report, error) {
	if !s.Valid() {
		return model.Report{}, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.ReportUpdateStatus(ctx, sqlcgen.ReportUpdateStatusParams{
		Status: sqlcgen.ReportStatus(status), ErrorMessage: errorMessage, ProcessedAt: tariffTimestamptz(at),
		ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return model.Report{}, pgerr.Translate(r.pool, "update report status", err)
	}
	return reportFromRow(row), nil
}

func reportFromRow(row sqlcgen.Report) model.Report {
	return model.Report{
		ID: row.ID, CompanyID: row.CompanyID, BuildingID: row.BuildingID, Type: model.ReportType(row.Type),
		Period: row.Period, PlantSelection: model.PlantSelection(row.PlantSelection), Payload: row.Payload,
		PdfPath: row.PdfPath, ExcelPath: row.ExcelPath, EmailSubject: row.EmailSubject, EmailBody: row.EmailBody,
		Status: model.ReportStatus(row.Status), ErrorMessage: row.ErrorMessage,
		ProcessedAt: tariffNullTimestamptz(row.ProcessedAt), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}
