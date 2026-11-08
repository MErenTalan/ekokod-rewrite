package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// BillPeriods lists a building's saved windows (R555).
func (r *BillRepository) BillPeriods(ctx context.Context, s store.Scope, buildingID uuid.UUID, keys []string) ([]model.BillPeriod, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	rows, err := r.q.BillPeriodList(ctx, sqlcgen.BillPeriodListParams{
		CompanyID: s.CompanyID, BuildingID: buildingID, AllBuildings: all, BuildingIds: ids, Keys: append([]string{}, keys...),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list bill periods", err)
	}
	out := make([]model.BillPeriod, len(rows))
	for i, row := range rows {
		out[i] = billPeriodFromRow(row)
	}
	return out, nil
}

// UpsertBillPeriod saves one window; the building must be visible.
func (r *BillRepository) UpsertBillPeriod(ctx context.Context, s store.Scope, p model.BillPeriod) (model.BillPeriod, error) {
	if !s.Valid() {
		return model.BillPeriod{}, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.BillPeriodUpsert(ctx, sqlcgen.BillPeriodUpsertParams{
		CompanyID: s.CompanyID, BuildingID: p.BuildingID, PeriodKey: p.PeriodKey,
		PeriodStart: ts(p.PeriodStart), PeriodEnd: ts(p.PeriodEnd), CreatedBy: p.CreatedBy,
		AllBuildings: all, BuildingIds: ids,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.BillPeriod{}, store.ErrNotFound
	}
	if err != nil {
		return model.BillPeriod{}, pgerr.Translate(r.pool, "upsert bill period", err)
	}
	return billPeriodFromRow(row), nil
}

// DeleteBillPeriod removes one saved window.
func (r *BillRepository) DeleteBillPeriod(ctx context.Context, s store.Scope, buildingID uuid.UUID, key string) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	n, err := r.q.BillPeriodDelete(ctx, sqlcgen.BillPeriodDeleteParams{
		CompanyID: s.CompanyID, BuildingID: buildingID, PeriodKey: key, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "delete bill period", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func billPeriodFromRow(row sqlcgen.BillPeriod) model.BillPeriod {
	return model.BillPeriod{
		CompanyID: row.CompanyID, BuildingID: row.BuildingID, PeriodKey: row.PeriodKey,
		PeriodStart: row.PeriodStart.Time, PeriodEnd: row.PeriodEnd.Time, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}
