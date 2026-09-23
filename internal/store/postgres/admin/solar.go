package admin

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// SolarRepository implements store.AdminSolarRepository.
type SolarRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewSolarRepository builds a SolarRepository over pool.
func NewSolarRepository(pool *pgxpool.Pool) *SolarRepository {
	return &SolarRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.AdminSolarRepository = (*SolarRepository)(nil)

// LinkedPlants lists every live iSolar-linked plant of a live company.
func (r *SolarRepository) LinkedPlants(ctx context.Context) ([]store.CompanyPlant, error) {
	rows, err := r.q.AdminLinkedPlants(ctx)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list linked plants", err)
	}
	out := make([]store.CompanyPlant, len(rows))
	for i, row := range rows {
		out[i] = store.CompanyPlant{CompanyID: row.CompanyID, PlantID: row.PlantID}
	}
	return out, nil
}
