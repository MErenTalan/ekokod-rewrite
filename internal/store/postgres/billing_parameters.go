package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgbilling"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// BillingParameterRepository implements store.BillingParameterRepository over
// the platform-wide billing_parameters table: the Scope is validated and
// narrows nothing (PriceRepository's rule). Writes are admin-only.
type BillingParameterRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewBillingParameterRepository builds a BillingParameterRepository over pool.
func NewBillingParameterRepository(pool *pgxpool.Pool) *BillingParameterRepository {
	return &BillingParameterRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.BillingParameterRepository = (*BillingParameterRepository)(nil)

// Effective implements store.BillingParameterRepository.Effective.
func (r *BillingParameterRepository) Effective(ctx context.Context, s store.Scope, on time.Time) (model.BillingParameters, error) {
	if !s.Valid() {
		return model.BillingParameters{}, store.ErrInvalidScope
	}
	row, err := r.q.BillingParametersEffective(ctx, tariffTimeToDate(on))
	if err != nil {
		return model.BillingParameters{}, pgerr.Translate(r.pool, "effective billing parameters", err)
	}
	return pgbilling.Decode(row)
}

// List implements store.BillingParameterRepository.List.
func (r *BillingParameterRepository) List(ctx context.Context, s store.Scope) ([]model.BillingParameters, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	rows, err := r.q.BillingParametersList(ctx)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list billing parameters", err)
	}
	out := make([]model.BillingParameters, 0, len(rows))
	for _, row := range rows {
		p, err := pgbilling.Decode(pgbilling.Row(row))
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
