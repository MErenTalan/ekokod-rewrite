package admin

// This file implements store.AdminAuditRepository: the platform journal
// append that has no tenant to attribute to. See internal/store/repository.go's
// "UNSCOPED ADMIN INTERFACES" section for why it cannot take a store.Scope.

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// AuditRepository is the postgres store.AdminAuditRepository.
type AuditRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.AdminAuditRepository = (*AuditRepository)(nil)

// NewAuditRepository builds an AuditRepository over pool.
func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{q: sqlcgen.New(pool), pool: pool}
}

// AppendPlatform appends an audit row with company_id NULL. An entry whose
// CompanyID is non-nil names a tenant row, which this surface never writes:
// it is refused with ErrNotFound before any database round trip.
func (r *AuditRepository) AppendPlatform(ctx context.Context, e model.AuditEntry) (model.AuditEntry, error) {
	if e.CompanyID != nil {
		return model.AuditEntry{}, store.ErrNotFound
	}
	row, err := r.q.AdminAppendPlatformAudit(ctx, sqlcgen.AdminAppendPlatformAuditParams{
		UserID:     e.UserID,
		Action:     e.Action,
		EntityType: e.EntityType,
		EntityID:   e.EntityID,
		Before:     e.Before,
		After:      e.After,
		Ip:         e.IP,
		At:         adminTSOrNow(e.CreatedAt),
	})
	if err != nil {
		return model.AuditEntry{}, pgerr.Translate(r.pool, "append platform audit entry", err)
	}
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
	}, nil
}
