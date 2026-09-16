package admin

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// BillingRepository implements store.AdminBillingRepository: the billing
// dispatcher's cross-tenant discovery of buildings to invoice.
type BillingRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewBillingRepository builds a BillingRepository on pool.
func NewBillingRepository(pool *pgxpool.Pool) *BillingRepository {
	return &BillingRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.AdminBillingRepository = (*BillingRepository)(nil)

// BillableBuildings implements store.AdminBillingRepository.BillableBuildings.
func (r *BillingRepository) BillableBuildings(ctx context.Context) ([]store.BillableBuilding, error) {
	rows, err := r.q.AdminBillableBuildings(ctx)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "billable buildings", err)
	}
	out := make([]store.BillableBuilding, len(rows))
	for i, row := range rows {
		out[i] = store.BillableBuilding{CompanyID: row.CompanyID, BuildingID: row.BuildingID, CutoffDay: int(row.BillCutoffDay)}
	}
	return out, nil
}
