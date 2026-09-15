package admin

// This file implements store.AdminAuthRepository: the pre-auth lookups a
// login and a refresh need, before either has produced a Scope. See
// internal/store/repository.go's "UNSCOPED ADMIN INTERFACES" section for why
// these two methods cannot take a store.Scope.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// AuthRepository is the postgres store.AdminAuthRepository.
type AuthRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.AdminAuthRepository = (*AuthRepository)(nil)

// NewAuthRepository builds an AuthRepository over pool.
func NewAuthRepository(pool *pgxpool.Pool) *AuthRepository {
	return &AuthRepository{q: sqlcgen.New(pool), pool: pool}
}

func adminUserFromRow(row sqlcgen.User) model.User {
	return model.User{
		ID:                row.ID,
		CompanyID:         row.CompanyID,
		Name:              row.Name,
		Email:             row.Email,
		Phone:             row.Phone,
		PasswordHash:      row.PasswordHash,
		PasswordChangedAt: adminTSPtr(row.PasswordChangedAt),
		Role:              model.UserRole(row.Role),
		IsActive:          row.IsActive,
		LastLoginAt:       adminTSPtr(row.LastLoginAt),
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
		DeletedAt:         adminTSPtr(row.DeletedAt),
	}
}

// UserByEmail resolves a login. See the doc on store.AdminAuthRepository for
// why it cannot take a Scope.
func (r *AuthRepository) UserByEmail(ctx context.Context, email string) (model.User, error) {
	row, err := r.q.AdminUserByEmail(ctx, email)
	if err != nil {
		return model.User{}, pgerr.Translate(r.pool, "admin get user by email", err)
	}
	return adminUserFromRow(row), nil
}

// SessionByRefreshTokenHash resolves a refresh. See the doc on
// store.AdminAuthRepository for why it returns the owning user alongside the
// session and why it cannot take a Scope.
func (r *AuthRepository) SessionByRefreshTokenHash(ctx context.Context, hash string) (model.Session, model.User, error) {
	row, err := r.q.AdminSessionByRefreshTokenHash(ctx, hash)
	if err != nil {
		return model.Session{}, model.User{}, pgerr.Translate(r.pool, "admin get session by refresh token hash", err)
	}
	sess := model.Session{
		ID:                row.ID,
		UserID:            row.UserID,
		RefreshTokenHash:  row.RefreshTokenHash,
		DeviceFingerprint: row.DeviceFingerprint,
		UserAgent:         row.UserAgent,
		IP:                row.Ip,
		ExpiresAt:         row.ExpiresAt.Time,
		RevokedAt:         adminTSPtr(row.RevokedAt),
		CreatedAt:         row.SessionCreatedAt.Time,
	}
	user := model.User{
		ID:                row.UserID,
		CompanyID:         row.CompanyID,
		Name:              row.Name,
		Email:             row.Email,
		Phone:             row.Phone,
		PasswordHash:      row.PasswordHash,
		PasswordChangedAt: adminTSPtr(row.PasswordChangedAt),
		Role:              model.UserRole(row.Role),
		IsActive:          row.IsActive,
		LastLoginAt:       adminTSPtr(row.LastLoginAt),
		CreatedAt:         row.UserCreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
		DeletedAt:         adminTSPtr(row.DeletedAt),
	}
	return sess, user, nil
}

// adminTSPtr converts a NULLABLE pgtype.Timestamptz into a *time.Time, nil
// for SQL NULL. This package builds on sqlcgen directly rather than package
// postgres (see repository.go's "UNSCOPED ADMIN INTERFACES" note: admin is
// its own implementation, not a wrapper over the scoped one), so it keeps
// this tiny conversion rather than importing postgres's unexported tsPtr.
func adminTSPtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

// adminTSOrNow is adminTSPtr's write-side counterpart for a NOT NULL
// created_at column written from a plain time.Time: a caller that leaves
// CreatedAt at its zero value gets SQL NULL, which AdminAppendPlatformAudit
// pairs with `coalesce(sqlc.narg(at)::timestamptz, now())` so the row gets
// the database's own now() instead of literally storing 0001-01-01. See
// package postgres's tsOrNow (internal/store/postgres/convert.go) for the
// scoped-surface twin of this same conversion.
func adminTSOrNow(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}
