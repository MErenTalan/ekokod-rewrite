package model

import (
	"encoding/json"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Company is a tenant. Every other aggregate in the schema is reachable from
// exactly one company, and store.Scope's CompanyID names it.
//
// Mirrors table `companies` (migration 00002). `companies (lower(name))` is
// unique among live rows, so two companies may not differ only by case.
type Company struct {
	ID   uuid.UUID
	Name string

	Address *string
	// TotalAreaM2 is numeric(14,2) — an area, not money, but decimal for the
	// same reason: it divides into per-m² intensity figures that end up on a
	// report.
	TotalAreaM2    *decimal.Decimal
	PersonnelCount *int32
	ContactName    *string
	ContactPhone   *string
	Sector         *string

	CreatedAt time.Time
	UpdatedAt time.Time
	// DeletedAt is nil for a live row. Every query in the repository layer
	// filters it; a non-nil value means the row is invisible to the product.
	DeletedAt *time.Time
}

// User is a principal. Mirrors table `users` (migration 00002).
//
// PasswordHash is the stored hash and never the password itself. It is a field
// here because the repository must write it; no layer above internal/store has
// any reason to read it, and nothing may log it.
type User struct {
	ID        uuid.UUID
	CompanyID uuid.UUID

	Name string
	// Email is citext in SQL, so comparison is case-insensitive and the
	// partial unique index `users (email)` treats A@b.com and a@B.com as the
	// same live user.
	Email string
	Phone *string

	PasswordHash      string
	PasswordChangedAt *time.Time

	Role        UserRole
	IsActive    bool
	LastLoginAt *time.Time

	// UIPreferences is the opaque ekokod_ui cookie value the web app syncs
	// (R169); Locale is 'tr' or 'en'.
	UIPreferences *string
	Locale        string

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// PasswordHistoryEntry is one previously used password hash, kept so that
// reuse can be refused. Mirrors table `user_password_history` (migration 00002).
type PasswordHistoryEntry struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	PasswordHash string
	CreatedAt    time.Time
}

// Session is a refresh-token grant. Mirrors table `sessions` (migration 00002).
//
// RefreshTokenHash is the hash of the token, never the token; the plaintext
// exists only in the response that issued it.
type Session struct {
	ID     uuid.UUID
	UserID uuid.UUID

	RefreshTokenHash  string
	DeviceFingerprint *string
	UserAgent         *string
	// IP mirrors the `inet` column. netip.Addr is a pure value type — it
	// parses and formats addresses and performs no I/O, which is why the
	// domain layer may hold it.
	IP *netip.Addr

	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time

	// Client is 'web' or 'mobile'; Remember selects the long lifetime (R141).
	Client   string
	Remember bool
	// LastUsedAt is stamped at most every five minutes by Touch.
	LastUsedAt *time.Time
	// RevokedReason says why RevokedAt was set; 'rotated' is what lets a
	// refresh tell a concurrent tab from a replayed token (R141).
	RevokedReason *string
	RotatedFrom   *uuid.UUID
}

// PasswordReset is one single-use password-reset token (R148). Mirrors table
// `password_reset_tokens` (migration 00015); TokenHash is SHA-256 hex.
type PasswordReset struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// AuditEntry is one row of the append-only audit trail. Mirrors table
// `audit_log` (migration 00002).
//
// Before and After are jsonb snapshots of the entity. They are raw JSON here
// rather than a decoded shape because the table stores a different shape per
// EntityType and nothing queries into them by field.
type AuditEntry struct {
	// ID is bigserial, hence int64 rather than a uuid.
	ID int64

	// CompanyID and UserID are nullable: a platform-level action has no
	// company, and a system-initiated one has no user.
	CompanyID *uuid.UUID
	UserID    *uuid.UUID

	Action     string
	EntityType string
	EntityID   *uuid.UUID

	Before json.RawMessage
	After  json.RawMessage
	IP     *netip.Addr

	CreatedAt time.Time
}
