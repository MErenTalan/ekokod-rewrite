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

// OpsRepository implements store.OpsRepository.
//
// job_runs.company_id and operational_messages.company_id are nullable, for
// platform-wide work (AdminJournalRepository). Every method here stores and
// sees only company_id = s.CompanyID; the queries in ops.sql filter
// company_id explicitly rather than "is null or …", so a platform row is
// never reachable from this surface.
type OpsRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewOpsRepository builds an OpsRepository on pool.
func NewOpsRepository(pool *pgxpool.Pool) *OpsRepository {
	return &OpsRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.OpsRepository = (*OpsRepository)(nil)

const (
	opsListDefaultLimit = 100
	opsListMaxLimit     = 1000
)

func opsPageBounds(p store.Page) (limit, offset int32) {
	limit = p.Limit
	if limit <= 0 {
		limit = opsListDefaultLimit
	}
	if limit > opsListMaxLimit {
		limit = opsListMaxLimit
	}
	offset = p.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func opsJobRunFromRow(row sqlcgen.JobRun) model.JobRun {
	var finishedAt *time.Time
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		finishedAt = &t
	}
	return model.JobRun{
		ID: row.ID, CompanyID: row.CompanyID, JobType: row.JobType, Scope: row.Scope,
		StartedAt: row.StartedAt.Time, FinishedAt: finishedAt, Status: row.Status,
		Processed: row.Processed, Skipped: row.Skipped, Failed: row.Failed,
		Error: row.Error, Detail: row.Detail,
	}
}

func opsOperationalMessageFromRow(row sqlcgen.OperationalMessage) model.OperationalMessage {
	return model.OperationalMessage{
		ID: row.ID, CompanyID: row.CompanyID, Kind: row.Kind, Category: row.Category, Status: row.Status,
		Message: row.Message, Detail: row.Detail, RelatedType: row.RelatedType, RelatedID: row.RelatedID,
		Metadata: row.Metadata, CreatedAt: row.CreatedAt.Time,
	}
}

// StartRun inserts a job_runs row in the 'running' state. run.CompanyID must
// equal s.CompanyID, checked before any database call. A caller-supplied
// run.Status is IGNORED, not merely defaulted: per the contract, a job run
// always starts 'running' — only FinishRun may set a terminal status (fix
// round 1, folded minor).
func (r *OpsRepository) StartRun(ctx context.Context, s store.Scope, run model.JobRun) (model.JobRun, error) {
	if !s.Valid() {
		return model.JobRun{}, store.ErrInvalidScope
	}
	if run.CompanyID == nil || *run.CompanyID != s.CompanyID {
		return model.JobRun{}, store.ErrNotFound
	}
	scope := run.Scope
	if scope == nil {
		scope = []byte("{}")
	}
	row, err := r.q.OpsStartRun(ctx, sqlcgen.OpsStartRunParams{
		ID: run.ID, CompanyID: s.CompanyID, JobType: run.JobType, Scope: scope, Status: "running",
	})
	if err != nil {
		return model.JobRun{}, pgerr.Translate(r.pool, "start job run", err)
	}
	return opsJobRunFromRow(row), nil
}

// FinishRun stamps finished_at, the final status and the three counts. Only a
// run with company_id = s.CompanyID can be finished here.
func (r *OpsRepository) FinishRun(ctx context.Context, s store.Scope, id uuid.UUID, status string, processed, skipped, failed int32, errText *string, detail []byte, at time.Time) (model.JobRun, error) {
	if !s.Valid() {
		return model.JobRun{}, store.ErrInvalidScope
	}
	row, err := r.q.OpsFinishRun(ctx, sqlcgen.OpsFinishRunParams{
		At: pgtype.Timestamptz{Time: at, Valid: true}, Status: status,
		Processed: processed, Skipped: skipped, Failed: failed, ErrorText: errText, Detail: detail,
		ID: id, CompanyID: s.CompanyID,
	})
	if err != nil {
		return model.JobRun{}, pgerr.Translate(r.pool, "finish job run", err)
	}
	return opsJobRunFromRow(row), nil
}

// GetRun returns one job run by id, scoped to s.
func (r *OpsRepository) GetRun(ctx context.Context, s store.Scope, id uuid.UUID) (model.JobRun, error) {
	if !s.Valid() {
		return model.JobRun{}, store.ErrInvalidScope
	}
	row, err := r.q.OpsGetRun(ctx, sqlcgen.OpsGetRunParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return model.JobRun{}, pgerr.Translate(r.pool, "get job run", err)
	}
	return opsJobRunFromRow(row), nil
}

// ListRuns returns a company's job run history matching f.
func (r *OpsRepository) ListRuns(ctx context.Context, s store.Scope, f store.JobRunFilter) ([]model.JobRun, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if f.Range != nil && !f.Range.Valid() {
		return nil, store.ErrInvalidRange
	}
	limit, offset := opsPageBounds(f.Page)
	var from, to pgtype.Timestamptz
	if f.Range != nil {
		from, to = pgtype.Timestamptz{Time: f.Range.From, Valid: true}, pgtype.Timestamptz{Time: f.Range.To, Valid: true}
	}
	rows, err := r.q.OpsListRuns(ctx, sqlcgen.OpsListRunsParams{
		CompanyID: s.CompanyID, JobType: f.JobType, Statuses: f.Statuses, Running: f.Running,
		RangeFrom: from, RangeTo: to, OffsetVal: offset, LimitVal: limit,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list job runs", err)
	}
	out := make([]model.JobRun, 0, len(rows))
	for _, row := range rows {
		out = append(out, opsJobRunFromRow(row))
	}
	return out, nil
}

// AppendMessage stores s.CompanyID as company_id; a message whose CompanyID
// is nil (a platform message) or names another company is refused with
// ErrNotFound.
func (r *OpsRepository) AppendMessage(ctx context.Context, s store.Scope, m model.OperationalMessage) (model.OperationalMessage, error) {
	if !s.Valid() {
		return model.OperationalMessage{}, store.ErrInvalidScope
	}
	if m.CompanyID == nil || *m.CompanyID != s.CompanyID {
		return model.OperationalMessage{}, store.ErrNotFound
	}
	row, err := r.q.OpsAppendMessage(ctx, sqlcgen.OpsAppendMessageParams{
		CompanyID: s.CompanyID, Kind: m.Kind, Category: m.Category, Status: m.Status, Message: m.Message,
		Detail: m.Detail, RelatedType: m.RelatedType, RelatedID: m.RelatedID, Metadata: m.Metadata,
	})
	if err != nil {
		return model.OperationalMessage{}, pgerr.Translate(r.pool, "append operational message", err)
	}
	return opsOperationalMessageFromRow(row), nil
}

// ListMessages returns a company's Messages screen entries matching f.
func (r *OpsRepository) ListMessages(ctx context.Context, s store.Scope, f store.MessageFilter) ([]model.OperationalMessage, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if f.Range != nil && !f.Range.Valid() {
		return nil, store.ErrInvalidRange
	}
	limit, offset := opsPageBounds(f.Page)
	var from, to pgtype.Timestamptz
	if f.Range != nil {
		from, to = pgtype.Timestamptz{Time: f.Range.From, Valid: true}, pgtype.Timestamptz{Time: f.Range.To, Valid: true}
	}
	rows, err := r.q.OpsListMessages(ctx, sqlcgen.OpsListMessagesParams{
		CompanyID: s.CompanyID, Kinds: f.Kinds, Categories: f.Categories, Statuses: f.Statuses,
		RelatedType: f.RelatedType, RelatedID: f.RelatedID, RangeFrom: from, RangeTo: to,
		Q:         f.Q,
		OffsetVal: offset, LimitVal: limit,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list operational messages", err)
	}
	out := make([]model.OperationalMessage, 0, len(rows))
	for _, row := range rows {
		out = append(out, opsOperationalMessageFromRow(row))
	}
	return out, nil
}
