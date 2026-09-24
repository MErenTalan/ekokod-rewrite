package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// AlarmRepository implements store.AdminAlarmRepository: the one unscoped
// alarm read, used by the hourly dispatcher to find tenants with work.
type AlarmRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewAlarmRepository builds an AlarmRepository over pool.
func NewAlarmRepository(pool *pgxpool.Pool) *AlarmRepository {
	return &AlarmRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.AdminAlarmRepository = (*AlarmRepository)(nil)

// CompaniesWithEnabledAlarms returns every live company holding an enabled rule.
func (r *AlarmRepository) CompaniesWithEnabledAlarms(ctx context.Context) ([]uuid.UUID, error) {
	ids, err := r.q.AdminCompaniesWithEnabledAlarms(ctx)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list companies with enabled alarms", err)
	}
	return ids, nil
}
