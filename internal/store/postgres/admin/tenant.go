package admin

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgnum"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

const (
	tenantPageDefaultLimit = 50
	tenantPageMaxLimit     = 1000
)

// TenantRepository is the postgres store.AdminTenantRepository.
type TenantRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.AdminTenantRepository = (*TenantRepository)(nil)

// NewTenantRepository builds a TenantRepository over pool.
func NewTenantRepository(pool *pgxpool.Pool) *TenantRepository {
	return &TenantRepository{q: sqlcgen.New(pool), pool: pool}
}

// ListCompanies lists every tenant for the platform operator. See
// store.AdminTenantRepository for why it cannot take a Scope.
func (r *TenantRepository) ListCompanies(ctx context.Context, f store.CompanyFilter) ([]model.Company, error) {
	limit := f.Page.Limit
	if limit <= 0 {
		limit = tenantPageDefaultLimit
	}
	limit = min(limit, tenantPageMaxLimit)
	rows, err := r.q.AdminCompanyList(ctx, sqlcgen.AdminCompanyListParams{
		IncludeDeleted: f.IncludeDeleted,
		NameContains:   f.NameContains,
		Sector:         f.Sector,
		PageOffset:     f.Page.Offset,
		PageLimit:      limit,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "admin list companies", err)
	}
	out := make([]model.Company, 0, len(rows))
	for _, row := range rows {
		area, err := pgnum.NumericToDecimalPtr(row.TotalAreaM2)
		if err != nil {
			return nil, err
		}
		out = append(out, model.Company{
			ID: row.ID, Name: row.Name, Address: row.Address, TotalAreaM2: area,
			PersonnelCount: row.PersonnelCount, ContactName: row.ContactName, ContactPhone: row.ContactPhone,
			Sector: row.Sector, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
			DeletedAt: adminTSPtr(row.DeletedAt),
		})
	}
	return out, nil
}
