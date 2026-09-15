package admin

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

// JournalRepository implements store.AdminJournalRepository: platform job
// runs and operational messages, filed with company_id NULL. See doc.go for
// why none of this can take a Scope.
type JournalRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewJournalRepository builds a JournalRepository on pool.
func NewJournalRepository(pool *pgxpool.Pool) *JournalRepository {
	return &JournalRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.AdminJournalRepository = (*JournalRepository)(nil)

func adminJobRunFromRow(row sqlcgen.JobRun) model.JobRun {
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

func adminOperationalMessageFromRow(row sqlcgen.OperationalMessage) model.OperationalMessage {
	return model.OperationalMessage{
		ID: row.ID, CompanyID: row.CompanyID, Kind: row.Kind, Category: row.Category, Status: row.Status,
		Message: row.Message, Detail: row.Detail, RelatedType: row.RelatedType, RelatedID: row.RelatedID,
		Metadata: row.Metadata, CreatedAt: row.CreatedAt.Time,
	}
}

// StartPlatformRun inserts a job_runs row with company_id NULL in the
// 'running' state. A run whose CompanyID is non-nil is refused with
// ErrNotFound, before any database call. A caller-supplied run.Status is
// IGNORED, not merely defaulted: per the contract, a job run always starts
// 'running' — only FinishPlatformRun may set a terminal status (fix round 1,
// folded minor; mirrors OpsRepository.StartRun).
func (r *JournalRepository) StartPlatformRun(ctx context.Context, run model.JobRun) (model.JobRun, error) {
	if run.CompanyID != nil {
		return model.JobRun{}, store.ErrNotFound
	}
	scope := run.Scope
	if scope == nil {
		scope = []byte("{}")
	}
	row, err := r.q.AdminStartPlatformRun(ctx, sqlcgen.AdminStartPlatformRunParams{
		ID: run.ID, JobType: run.JobType, Scope: scope, Status: "running",
	})
	if err != nil {
		return model.JobRun{}, pgerr.Translate(r.pool, "start platform job run", err)
	}
	return adminJobRunFromRow(row), nil
}

// FinishPlatformRun stamps a PLATFORM run exactly as OpsRepository.FinishRun
// stamps a tenant's. A tenant run's id returns ErrNotFound: the query only
// ever matches a company_id-null row (admin_journal.sql).
func (r *JournalRepository) FinishPlatformRun(ctx context.Context, id uuid.UUID, status string, processed, skipped, failed int32, errText *string, detail []byte, at time.Time) (model.JobRun, error) {
	row, err := r.q.AdminFinishPlatformRun(ctx, sqlcgen.AdminFinishPlatformRunParams{
		At: pgtype.Timestamptz{Time: at, Valid: true}, Status: status,
		Processed: processed, Skipped: skipped, Failed: failed, ErrorText: errText, Detail: detail,
		ID: id,
	})
	if err != nil {
		return model.JobRun{}, pgerr.Translate(r.pool, "finish platform job run", err)
	}
	return adminJobRunFromRow(row), nil
}

// AppendPlatformMessage appends an operational message with company_id
// NULL. A message whose CompanyID is non-nil is refused with ErrNotFound,
// before any database call.
func (r *JournalRepository) AppendPlatformMessage(ctx context.Context, m model.OperationalMessage) (model.OperationalMessage, error) {
	if m.CompanyID != nil {
		return model.OperationalMessage{}, store.ErrNotFound
	}
	row, err := r.q.AdminAppendPlatformMessage(ctx, sqlcgen.AdminAppendPlatformMessageParams{
		Kind: m.Kind, Category: m.Category, Status: m.Status, Message: m.Message,
		Detail: m.Detail, RelatedType: m.RelatedType, RelatedID: m.RelatedID, Metadata: m.Metadata,
	})
	if err != nil {
		return model.OperationalMessage{}, pgerr.Translate(r.pool, "append platform operational message", err)
	}
	return adminOperationalMessageFromRow(row), nil
}
