package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// PriceRepository implements store.PriceRepository: a tenant's READ access to
// the two PLATFORM-WIDE tables market_prices_hourly and yekdem_monthly.
// Neither has a company_id, so the Scope narrows nothing — it is still
// required and still validated, per repository.go's PriceRepository doc. The
// WRITE side is admin.MarketDataRepository (store.AdminMarketDataRepository):
// a scoped write here would let one tenant reprice every tenant's invoices.
type PriceRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewPriceRepository wraps pool in the generated query set.
func NewPriceRepository(pool *pgxpool.Pool) *PriceRepository {
	return &PriceRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.PriceRepository = (*PriceRepository)(nil)

// HourlyRange implements store.PriceRepository.HourlyRange.
func (r *PriceRepository) HourlyRange(ctx context.Context, s store.Scope, tr store.TimeRange) ([]model.MarketPrice, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}

	rows, err := r.q.PriceHourlyRange(ctx, sqlcgen.PriceHourlyRangeParams{
		FromTs: timeseriesToTimestamptz(tr.From),
		ToTs:   timeseriesToTimestamptz(tr.To),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "price hourly range", err)
	}
	out := make([]model.MarketPrice, len(rows))
	for i, row := range rows {
		mp, err := marketPriceFromRow(row)
		if err != nil {
			return nil, pgerr.Translate(r.pool, "price hourly range", err)
		}
		out[i] = mp
	}
	return out, nil
}

// Yekdem implements store.PriceRepository.Yekdem.
func (r *PriceRepository) Yekdem(ctx context.Context, s store.Scope, year, month int16) (model.YekdemMonthly, error) {
	if !s.Valid() {
		return model.YekdemMonthly{}, store.ErrInvalidScope
	}

	row, err := r.q.PriceYekdem(ctx, sqlcgen.PriceYekdemParams{Year: year, Month: month})
	if err != nil {
		return model.YekdemMonthly{}, pgerr.Translate(r.pool, "price yekdem", err)
	}
	value, err := numericToDecimal(row.Value)
	if err != nil {
		return model.YekdemMonthly{}, pgerr.Translate(r.pool, "price yekdem", err)
	}
	return model.YekdemMonthly{
		Year:      row.Year,
		Month:     row.Month,
		Value:     value,
		FetchedAt: row.FetchedAt.Time,
	}, nil
}

func marketPriceFromRow(row sqlcgen.MarketPricesHourly) (model.MarketPrice, error) {
	ptf, err := numericToDecimal(row.Ptf)
	if err != nil {
		return model.MarketPrice{}, err
	}
	return model.MarketPrice{
		Ts:        row.Ts.Time,
		PTF:       ptf,
		FetchedAt: row.FetchedAt.Time,
	}, nil
}
