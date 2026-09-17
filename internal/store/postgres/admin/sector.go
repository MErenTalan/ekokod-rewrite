package admin

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgnum"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// SectorRepository is the postgres store.AdminSectorRepository.
type SectorRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.AdminSectorRepository = (*SectorRepository)(nil)

// NewSectorRepository builds a SectorRepository over pool.
func NewSectorRepository(pool *pgxpool.Pool) *SectorRepository {
	return &SectorRepository{q: sqlcgen.New(pool), pool: pool}
}

// SectorFigures reads every peer's consumption over the two windows. See
// store.AdminSectorRepository for why it cannot take a Scope.
func (r *SectorRepository) SectorFigures(ctx context.Context, sector string, daily, month store.TimeRange) ([]store.SectorFigures, error) {
	if !daily.Valid() || !month.Valid() {
		return nil, store.ErrInvalidRange
	}
	window := func(tr store.TimeRange) ([]sqlcgen.AdminSectorConsumptionRow, error) {
		return r.q.AdminSectorConsumption(ctx, sqlcgen.AdminSectorConsumptionParams{
			Sector: sector, FromTs: pgtype.Timestamptz{Time: tr.From, Valid: true}, ToTs: pgtype.Timestamptz{Time: tr.To, Valid: true},
		})
	}
	dailyRows, err := window(daily)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "admin sector daily consumption", err)
	}
	monthRows, err := window(month)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "admin sector monthly consumption", err)
	}
	byBuilding := make(map[[16]byte]sqlcgen.AdminSectorConsumptionRow, len(monthRows))
	for _, row := range monthRows {
		byBuilding[row.BuildingID] = row
	}
	out := make([]store.SectorFigures, 0, len(dailyRows))
	for _, row := range dailyRows {
		area, err := pgnum.NumericToDecimalPtr(row.TotalAreaM2)
		if err != nil {
			return nil, err
		}
		dailySum, err := pgnum.NumericToDecimalPtr(row.Consumption)
		if err != nil {
			return nil, err
		}
		monthly, err := pgnum.NumericToDecimalPtr(byBuilding[row.BuildingID].Consumption)
		if err != nil {
			return nil, err
		}
		out = append(out, store.SectorFigures{
			BuildingID: row.BuildingID, PersonnelCount: row.PersonnelCount, TotalAreaM2: area,
			DailySum: dailySum, DaysWithData: row.Days, MonthlySum: monthly,
		})
	}
	return out, nil
}
