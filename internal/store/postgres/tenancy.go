package postgres

// This file implements store.CompanyRepository, store.UserRepository,
// store.SessionRepository and store.AuditRepository: everything grouped
// under "Tenancy, identity and access (migration 00002)" in
// internal/store/repository.go.

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// ---------------------------------------------------------------------------
// CompanyRepository
// ---------------------------------------------------------------------------

const (
	companyPageDefaultLimit = 50
	companyPageMaxLimit     = 500
)

// CompanyRepository is the postgres store.CompanyRepository.
type CompanyRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.CompanyRepository = (*CompanyRepository)(nil)

// NewCompanyRepository builds a CompanyRepository over pool.
func NewCompanyRepository(pool *pgxpool.Pool) *CompanyRepository {
	return &CompanyRepository{q: sqlcgen.New(pool), pool: pool}
}

func companyFromRow(row sqlcgen.Company) (model.Company, error) {
	totalArea, err := numericToDecimalPtr(row.TotalAreaM2)
	if err != nil {
		return model.Company{}, err
	}
	return model.Company{
		ID:             row.ID,
		Name:           row.Name,
		Address:        row.Address,
		TotalAreaM2:    totalArea,
		PersonnelCount: row.PersonnelCount,
		ContactName:    row.ContactName,
		ContactPhone:   row.ContactPhone,
		Sector:         row.Sector,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
		DeletedAt:      tsPtr(row.DeletedAt),
	}, nil
}

// Get implements store.CompanyRepository.Get.
func (r *CompanyRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.Company, error) {
	if !s.Valid() {
		return model.Company{}, store.ErrInvalidScope
	}
	row, err := r.q.CompanyGet(ctx, sqlcgen.CompanyGetParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return model.Company{}, pgerr.Translate(r.pool, "get company", err)
	}
	return companyFromRow(row)
}

// List implements store.CompanyRepository.List.
func (r *CompanyRepository) List(ctx context.Context, s store.Scope, f store.CompanyFilter) ([]model.Company, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	rows, err := r.q.CompanyList(ctx, sqlcgen.CompanyListParams{
		CompanyID:      s.CompanyID,
		IncludeDeleted: f.IncludeDeleted,
		NameContains:   f.NameContains,
		Sector:         f.Sector,
		PageOffset:     f.Page.Offset,
		PageLimit:      pageLimit(f.Page, companyPageDefaultLimit, companyPageMaxLimit),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list companies", err)
	}
	out := make([]model.Company, 0, len(rows))
	for _, row := range rows {
		c, err := companyFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// Create implements store.CompanyRepository.Create.
func (r *CompanyRepository) Create(ctx context.Context, s store.Scope, c model.Company) (model.Company, error) {
	if !s.Valid() {
		return model.Company{}, store.ErrInvalidScope
	}
	// A company IS the tenant: the only id this Scope may ever create is its
	// own. A caller that names another company is refused, never silently
	// redirected.
	if c.ID != uuid.Nil && c.ID != s.CompanyID {
		return model.Company{}, store.ErrNotFound
	}
	row, err := r.q.CompanyCreate(ctx, sqlcgen.CompanyCreateParams{
		ID:             s.CompanyID,
		Name:           c.Name,
		Address:        c.Address,
		TotalAreaM2:    decimalPtrToNumeric(c.TotalAreaM2),
		PersonnelCount: c.PersonnelCount,
		ContactName:    c.ContactName,
		ContactPhone:   c.ContactPhone,
		Sector:         c.Sector,
		At:             tsOrNow(c.CreatedAt),
	})
	if err != nil {
		return model.Company{}, pgerr.Translate(r.pool, "create company", err)
	}
	return companyFromRow(row)
}

// Update implements store.CompanyRepository.Update.
func (r *CompanyRepository) Update(ctx context.Context, s store.Scope, c model.Company) (model.Company, error) {
	if !s.Valid() {
		return model.Company{}, store.ErrInvalidScope
	}
	if c.ID != s.CompanyID {
		return model.Company{}, store.ErrNotFound
	}
	row, err := r.q.CompanyUpdate(ctx, sqlcgen.CompanyUpdateParams{
		ID:             c.ID,
		CompanyID:      s.CompanyID,
		Name:           c.Name,
		Address:        c.Address,
		TotalAreaM2:    decimalPtrToNumeric(c.TotalAreaM2),
		PersonnelCount: c.PersonnelCount,
		ContactName:    c.ContactName,
		ContactPhone:   c.ContactPhone,
		Sector:         c.Sector,
		UpdatedAt:      ts(c.UpdatedAt),
	})
	if err != nil {
		return model.Company{}, pgerr.Translate(r.pool, "update company", err)
	}
	return companyFromRow(row)
}

// SoftDelete implements store.CompanyRepository.SoftDelete.
func (r *CompanyRepository) SoftDelete(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.CompanySoftDelete(ctx, sqlcgen.CompanySoftDeleteParams{
		DeletedAt: ts(at), ID: id, CompanyID: s.CompanyID,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "soft delete company", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// UserRepository
// ---------------------------------------------------------------------------

const (
	userPageDefaultLimit = 50
	userPageMaxLimit     = 500
)

// UserRepository is the postgres store.UserRepository.
type UserRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.UserRepository = (*UserRepository)(nil)

// NewUserRepository builds a UserRepository over pool.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{q: sqlcgen.New(pool), pool: pool}
}

func userFromRow(row sqlcgen.User) model.User {
	return model.User{
		ID:                row.ID,
		CompanyID:         row.CompanyID,
		Name:              row.Name,
		Email:             row.Email,
		Phone:             row.Phone,
		PasswordHash:      row.PasswordHash,
		PasswordChangedAt: tsPtr(row.PasswordChangedAt),
		Role:              model.UserRole(row.Role),
		IsActive:          row.IsActive,
		LastLoginAt:       tsPtr(row.LastLoginAt),
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
		DeletedAt:         tsPtr(row.DeletedAt),
	}
}

// Get implements store.UserRepository.Get.
func (r *UserRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.User, error) {
	if !s.Valid() {
		return model.User{}, store.ErrInvalidScope
	}
	row, err := r.q.UserGet(ctx, sqlcgen.UserGetParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return model.User{}, pgerr.Translate(r.pool, "get user", err)
	}
	return userFromRow(row), nil
}

// List implements store.UserRepository.List.
func (r *UserRepository) List(ctx context.Context, s store.Scope, f store.UserFilter) ([]model.User, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	roles := make([]string, len(f.Roles))
	for i, role := range f.Roles {
		roles[i] = string(role)
	}
	rows, err := r.q.UserList(ctx, sqlcgen.UserListParams{
		CompanyID:      s.CompanyID,
		IncludeDeleted: f.IncludeDeleted,
		Roles:          roles,
		IsActive:       f.IsActive,
		EmailContains:  f.EmailContains,
		PageOffset:     f.Page.Offset,
		PageLimit:      pageLimit(f.Page, userPageDefaultLimit, userPageMaxLimit),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list users", err)
	}
	out := make([]model.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, userFromRow(row))
	}
	return out, nil
}

// Create implements store.UserRepository.Create.
func (r *UserRepository) Create(ctx context.Context, s store.Scope, u model.User) (model.User, error) {
	if !s.Valid() {
		return model.User{}, store.ErrInvalidScope
	}
	if u.CompanyID != s.CompanyID {
		return model.User{}, store.ErrNotFound
	}
	id := u.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	row, err := r.q.UserCreate(ctx, sqlcgen.UserCreateParams{
		ID:           id,
		CompanyID:    s.CompanyID,
		Name:         u.Name,
		Email:        u.Email,
		Phone:        u.Phone,
		PasswordHash: u.PasswordHash,
		Role:         sqlcgen.UserRole(u.Role),
		IsActive:     u.IsActive,
		At:           tsOrNow(u.CreatedAt),
	})
	if err != nil {
		return model.User{}, pgerr.Translate(r.pool, "create user", err)
	}
	return userFromRow(row), nil
}

// Update implements store.UserRepository.Update.
func (r *UserRepository) Update(ctx context.Context, s store.Scope, u model.User) (model.User, error) {
	if !s.Valid() {
		return model.User{}, store.ErrInvalidScope
	}
	if u.CompanyID != s.CompanyID {
		return model.User{}, store.ErrNotFound
	}
	row, err := r.q.UserUpdate(ctx, sqlcgen.UserUpdateParams{
		ID:        u.ID,
		CompanyID: s.CompanyID,
		Name:      u.Name,
		Email:     u.Email,
		Phone:     u.Phone,
		Role:      sqlcgen.UserRole(u.Role),
		IsActive:  u.IsActive,
		UpdatedAt: ts(u.UpdatedAt),
	})
	if err != nil {
		return model.User{}, pgerr.Translate(r.pool, "update user", err)
	}
	return userFromRow(row), nil
}

// SoftDelete implements store.UserRepository.SoftDelete.
func (r *UserRepository) SoftDelete(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.UserSoftDelete(ctx, sqlcgen.UserSoftDeleteParams{
		DeletedAt: ts(at), ID: id, CompanyID: s.CompanyID,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "soft delete user", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// SetPassword writes the new hash and appends the old one to the history, in
// one transaction: a crash between the two must not lose the record that
// blocks reuse.
//
// Isolation: user_password_history has no company_id — join through users
// (company_id = s.CompanyID). Another tenant's userID returns ErrNotFound and
// writes neither the hash nor the history row: UserGetPasswordHashForUpdate is
// itself scoped, so an invisible user never reaches the two writes below.
func (r *UserRepository) SetPassword(ctx context.Context, s store.Scope, userID uuid.UUID, hash string, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return pgerr.Translate(r.pool, "begin set password", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	oldHash, err := qtx.UserGetPasswordHashForUpdate(ctx, sqlcgen.UserGetPasswordHashForUpdateParams{
		ID: userID, CompanyID: s.CompanyID,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "lock user for set password", err)
	}
	if err := qtx.UserSetPasswordHash(ctx, sqlcgen.UserSetPasswordHashParams{
		PasswordHash: hash, At: ts(at), ID: userID, CompanyID: s.CompanyID,
	}); err != nil {
		return pgerr.Translate(r.pool, "set password hash", err)
	}
	if err := qtx.UserPasswordHistoryInsert(ctx, sqlcgen.UserPasswordHistoryInsertParams{
		ID: uuid.New(), UserID: userID, PasswordHash: oldHash, At: ts(at),
	}); err != nil {
		return pgerr.Translate(r.pool, "insert password history", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return pgerr.Translate(r.pool, "commit set password", err)
	}
	return nil
}

// PasswordHistory — Isolation: join user_password_history through users
// (company_id = s.CompanyID). Another tenant's userID returns ErrNotFound;
// the join alone cannot express that (an invisible user with no history looks
// identical to a visible one with none), so visibility is checked first.
func (r *UserRepository) PasswordHistory(ctx context.Context, s store.Scope, userID uuid.UUID, limit int32) ([]model.PasswordHistoryEntry, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	visible, err := r.q.SessionUserVisible(ctx, sqlcgen.SessionUserVisibleParams{UserID: userID, CompanyID: s.CompanyID})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "check user visibility", err)
	}
	if !visible {
		return nil, store.ErrNotFound
	}
	rows, err := r.q.UserPasswordHistoryList(ctx, sqlcgen.UserPasswordHistoryListParams{
		UserID: userID, CompanyID: s.CompanyID, RowLimit: limit,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list password history", err)
	}
	out := make([]model.PasswordHistoryEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.PasswordHistoryEntry{
			ID:           row.ID,
			UserID:       row.UserID,
			PasswordHash: row.PasswordHash,
			CreatedAt:    row.CreatedAt.Time,
		})
	}
	return out, nil
}

// RecordLogin implements store.UserRepository.RecordLogin.
func (r *UserRepository) RecordLogin(ctx context.Context, s store.Scope, userID uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.UserRecordLogin(ctx, sqlcgen.UserRecordLoginParams{At: ts(at), ID: userID, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "record login", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// SessionRepository
// ---------------------------------------------------------------------------

const (
	sessionPageDefaultLimit = 50
	sessionPageMaxLimit     = 500
)

// SessionRepository is the postgres store.SessionRepository.
type SessionRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.SessionRepository = (*SessionRepository)(nil)

// NewSessionRepository builds a SessionRepository over pool.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{q: sqlcgen.New(pool), pool: pool}
}

func sessionFromRow(row sqlcgen.Session) model.Session {
	return model.Session{
		ID:                row.ID,
		UserID:            row.UserID,
		RefreshTokenHash:  row.RefreshTokenHash,
		DeviceFingerprint: row.DeviceFingerprint,
		UserAgent:         row.UserAgent,
		IP:                row.Ip,
		ExpiresAt:         row.ExpiresAt.Time,
		RevokedAt:         tsPtr(row.RevokedAt),
		CreatedAt:         row.CreatedAt.Time,
	}
}

// Get implements store.SessionRepository.Get.
func (r *SessionRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.Session, error) {
	if !s.Valid() {
		return model.Session{}, store.ErrInvalidScope
	}
	row, err := r.q.SessionGet(ctx, sqlcgen.SessionGetParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return model.Session{}, pgerr.Translate(r.pool, "get session", err)
	}
	return sessionFromRow(row), nil
}

// List implements store.SessionRepository.List.
func (r *SessionRepository) List(ctx context.Context, s store.Scope, f store.SessionFilter) ([]model.Session, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	var activeAt pgtype.Timestamptz
	if f.ActiveAt != nil {
		activeAt = ts(*f.ActiveAt)
	}
	rows, err := r.q.SessionList(ctx, sqlcgen.SessionListParams{
		CompanyID:  s.CompanyID,
		UserID:     f.UserID,
		ActiveAt:   activeAt,
		PageOffset: f.Page.Offset,
		PageLimit:  pageLimit(f.Page, sessionPageDefaultLimit, sessionPageMaxLimit),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list sessions", err)
	}
	out := make([]model.Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionFromRow(row))
	}
	return out, nil
}

// Create — Isolation: sess.UserID must be a user of s.CompanyID, or the
// insert is refused with ErrNotFound: the insert only runs when that user is
// visible, so an invisible user leaves the table untouched.
func (r *SessionRepository) Create(ctx context.Context, s store.Scope, sess model.Session) (model.Session, error) {
	if !s.Valid() {
		return model.Session{}, store.ErrInvalidScope
	}
	id := sess.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	var revokedAt pgtype.Timestamptz
	if sess.RevokedAt != nil {
		revokedAt = ts(*sess.RevokedAt)
	}
	row, err := r.q.SessionCreate(ctx, sqlcgen.SessionCreateParams{
		ID:                id,
		UserID:            sess.UserID,
		RefreshTokenHash:  sess.RefreshTokenHash,
		DeviceFingerprint: sess.DeviceFingerprint,
		UserAgent:         sess.UserAgent,
		Ip:                sess.IP,
		ExpiresAt:         ts(sess.ExpiresAt),
		RevokedAt:         revokedAt,
		At:                tsOrNow(sess.CreatedAt),
		CompanyID:         s.CompanyID,
	})
	if err != nil {
		return model.Session{}, pgerr.Translate(r.pool, "create session", err)
	}
	return sessionFromRow(row), nil
}

// Revoke implements store.SessionRepository.Revoke.
func (r *SessionRepository) Revoke(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.SessionRevoke(ctx, sqlcgen.SessionRevokeParams{At: ts(at), ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "revoke session", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// RevokeAllForUser — Isolation: join sessions through users; another tenant's
// userID returns ErrNotFound. Visibility is checked separately from the bulk
// update: a visible user with no active sessions must return (0, nil), not
// ErrNotFound, and the update's own row count cannot tell the two apart.
func (r *SessionRepository) RevokeAllForUser(ctx context.Context, s store.Scope, userID uuid.UUID, at time.Time) (int64, error) {
	if !s.Valid() {
		return 0, store.ErrInvalidScope
	}
	visible, err := r.q.SessionUserVisible(ctx, sqlcgen.SessionUserVisibleParams{UserID: userID, CompanyID: s.CompanyID})
	if err != nil {
		return 0, pgerr.Translate(r.pool, "check user visibility", err)
	}
	if !visible {
		return 0, store.ErrNotFound
	}
	n, err := r.q.SessionRevokeAllForUser(ctx, sqlcgen.SessionRevokeAllForUserParams{
		At: ts(at), UserID: userID, CompanyID: s.CompanyID,
	})
	if err != nil {
		return 0, pgerr.Translate(r.pool, "revoke all sessions for user", err)
	}
	return n, nil
}

// DeleteExpired implements store.SessionRepository.DeleteExpired.
func (r *SessionRepository) DeleteExpired(ctx context.Context, s store.Scope, before time.Time) (int64, error) {
	if !s.Valid() {
		return 0, store.ErrInvalidScope
	}
	n, err := r.q.SessionDeleteExpired(ctx, sqlcgen.SessionDeleteExpiredParams{Before: ts(before), CompanyID: s.CompanyID})
	if err != nil {
		return 0, pgerr.Translate(r.pool, "delete expired sessions", err)
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// AuditRepository
// ---------------------------------------------------------------------------

const (
	auditPageDefaultLimit = 100
	auditPageMaxLimit     = 1000
)

// AuditRepository is the postgres store.AuditRepository.
type AuditRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.AuditRepository = (*AuditRepository)(nil)

// NewAuditRepository builds an AuditRepository over pool.
func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{q: sqlcgen.New(pool), pool: pool}
}

func auditFromRow(row sqlcgen.AuditLog) model.AuditEntry {
	return model.AuditEntry{
		ID:         row.ID,
		CompanyID:  row.CompanyID,
		UserID:     row.UserID,
		Action:     row.Action,
		EntityType: row.EntityType,
		EntityID:   row.EntityID,
		Before:     json.RawMessage(row.Before),
		After:      json.RawMessage(row.After),
		IP:         row.Ip,
		CreatedAt:  row.CreatedAt.Time,
	}
}

// Append stores s.CompanyID as company_id. An entry whose CompanyID is nil (a
// platform row) or names another company is refused with ErrNotFound: a
// platform action belongs on AdminAuditRepository.AppendPlatform, never here.
func (r *AuditRepository) Append(ctx context.Context, s store.Scope, e model.AuditEntry) (model.AuditEntry, error) {
	if !s.Valid() {
		return model.AuditEntry{}, store.ErrInvalidScope
	}
	if e.CompanyID == nil || *e.CompanyID != s.CompanyID {
		return model.AuditEntry{}, store.ErrNotFound
	}
	row, err := r.q.AuditAppend(ctx, sqlcgen.AuditAppendParams{
		CompanyID:  &s.CompanyID,
		UserID:     e.UserID,
		Action:     e.Action,
		EntityType: e.EntityType,
		EntityID:   e.EntityID,
		Before:     e.Before,
		After:      e.After,
		Ip:         e.IP,
		At:         tsOrNow(e.CreatedAt),
	})
	if err != nil {
		return model.AuditEntry{}, pgerr.Translate(r.pool, "append audit entry", err)
	}
	return auditFromRow(row), nil
}

// List implements store.AuditRepository.List.
func (r *AuditRepository) List(ctx context.Context, s store.Scope, f store.AuditFilter) ([]model.AuditEntry, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	var rangeFrom, rangeTo pgtype.Timestamptz
	if f.Range != nil {
		if !f.Range.Valid() {
			return nil, store.ErrInvalidRange
		}
		rangeFrom, rangeTo = ts(f.Range.From), ts(f.Range.To)
	}
	rows, err := r.q.AuditList(ctx, sqlcgen.AuditListParams{
		CompanyID:  &s.CompanyID,
		UserID:     f.UserID,
		EntityType: f.EntityType,
		EntityID:   f.EntityID,
		Action:     f.Action,
		RangeFrom:  rangeFrom,
		RangeTo:    rangeTo,
		PageOffset: f.Page.Offset,
		PageLimit:  pageLimit(f.Page, auditPageDefaultLimit, auditPageMaxLimit),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list audit entries", err)
	}
	out := make([]model.AuditEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, auditFromRow(row))
	}
	return out, nil
}
