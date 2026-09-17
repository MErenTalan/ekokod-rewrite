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

// PasswordResetRepository is the postgres store.PasswordResetRepository.
type PasswordResetRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.PasswordResetRepository = (*PasswordResetRepository)(nil)

// NewPasswordResetRepository builds a PasswordResetRepository over pool.
func NewPasswordResetRepository(pool *pgxpool.Pool) *PasswordResetRepository {
	return &PasswordResetRepository{q: sqlcgen.New(pool), pool: pool}
}

// Create — Isolation: join through users. The user's unused tokens are
// invalidated in the same transaction, so only the newest link works (R148).
func (r *PasswordResetRepository) Create(ctx context.Context, s store.Scope, p model.PasswordReset) (model.PasswordReset, error) {
	if !s.Valid() {
		return model.PasswordReset{}, store.ErrInvalidScope
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.PasswordReset{}, pgerr.Translate(r.pool, "begin create password reset", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlcgen.New(tx)
	at := p.CreatedAt
	if at.IsZero() {
		at = time.Now()
	}
	if err := q.PasswordResetInvalidateUnused(ctx, sqlcgen.PasswordResetInvalidateUnusedParams{
		At: ts(at), UserID: p.UserID, CompanyID: s.CompanyID,
	}); err != nil {
		return model.PasswordReset{}, pgerr.Translate(r.pool, "invalidate password resets", err)
	}
	id := p.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	row, err := q.PasswordResetCreate(ctx, sqlcgen.PasswordResetCreateParams{
		ID: id, UserID: p.UserID, TokenHash: p.TokenHash, ExpiresAt: ts(p.ExpiresAt),
		At: tsOrNow(p.CreatedAt), CompanyID: s.CompanyID,
	})
	if err != nil {
		return model.PasswordReset{}, pgerr.Translate(r.pool, "create password reset", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.PasswordReset{}, pgerr.Translate(r.pool, "commit password reset", err)
	}
	return model.PasswordReset{
		ID: row.ID, UserID: row.UserID, TokenHash: row.TokenHash, ExpiresAt: row.ExpiresAt.Time,
		UsedAt: tsPtr(row.UsedAt), CreatedAt: row.CreatedAt.Time,
	}, nil
}

// MarkUsed — Isolation: join through users. A token already used returns
// ErrConflict, so two concurrent resets cannot both succeed.
func (r *PasswordResetRepository) MarkUsed(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	visible, err := r.q.PasswordResetVisible(ctx, sqlcgen.PasswordResetVisibleParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "check password reset visibility", err)
	}
	if !visible {
		return store.ErrNotFound
	}
	n, err := r.q.PasswordResetMarkUsed(ctx, sqlcgen.PasswordResetMarkUsedParams{At: ts(at), ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "mark password reset used", err)
	}
	if n == 0 {
		return store.ErrConflict
	}
	return nil
}
