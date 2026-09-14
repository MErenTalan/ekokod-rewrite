package admin

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// decimalToNumeric is a SECOND copy of the coefficient/exponent relabelling
// internal/store/postgres/numeric.go documents as "the ONE audited pair" —
// deliberately, not out of oversight. That pair is unexported, private to
// package postgres, and package admin (this package) is a SEPARATE package
// from postgres, one level below both alongside pgerr — it cannot call an
// unexported function in another package. Unlike pgerr.Translate, which was
// carved into its own importable package precisely so admin and postgres
// could share it, numeric.go's functions were not, and this task must not
// unilaterally relocate shared infrastructure two other parallel tasks
// (9 and 11) are already calling by its current unqualified name inside
// package postgres — that move belongs to a controller decision, flagged in
// this task's report, not to a silent refactor here.
//
// This copy is exact and minimal: Coefficient()/Exponent() is the same
// no-op relabelling numeric.go's own does, never a float64 round-trip.
// Every value this file converts (MarketPrice.PTF, YekdemMonthly.Value) is a
// NOT NULL decimal.Decimal, so there is no NULL/pointer branch to duplicate.
func decimalToNumeric(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

// fetchedAtOrNow defaults a zero FetchedAt to the current instant, so a
// caller that only fills in Ts/PTF (or Year/Month/Value) does not have to
// also think about a bookkeeping timestamp.
func fetchedAtOrNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}

// MarketDataRepository implements store.AdminMarketDataRepository: the
// platform-wide market_prices_hourly and yekdem_monthly writes. Neither
// table has a company_id, so neither method here takes a store.Scope — see
// repository.go's AdminMarketDataRepository doc for why no Scope could ever
// authorise these writes safely. Tenants read the same tables through
// store.PriceRepository.
type MarketDataRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewMarketDataRepository wraps pool in the generated query set.
func NewMarketDataRepository(pool *pgxpool.Pool) *MarketDataRepository {
	return &MarketDataRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.AdminMarketDataRepository = (*MarketDataRepository)(nil)

// UpsertHourlyPrices implements
// store.AdminMarketDataRepository.UpsertHourlyPrices.
func (r *MarketDataRepository) UpsertHourlyPrices(ctx context.Context, prices []model.MarketPrice) (int64, error) {
	if len(prices) == 0 {
		return 0, nil
	}

	ts := make([]pgtype.Timestamptz, len(prices))
	ptf := make([]pgtype.Numeric, len(prices))
	fetchedAt := make([]pgtype.Timestamptz, len(prices))
	for i, p := range prices {
		ts[i] = pgtype.Timestamptz{Time: p.Ts, Valid: true}
		ptf[i] = decimalToNumeric(p.PTF)
		fetchedAt[i] = pgtype.Timestamptz{Time: fetchedAtOrNow(p.FetchedAt), Valid: true}
	}

	n, err := r.q.AdminMarketDataUpsertHourlyPrices(ctx, sqlcgen.AdminMarketDataUpsertHourlyPricesParams{
		Ts:        ts,
		Ptf:       ptf,
		FetchedAt: fetchedAt,
	})
	if err != nil {
		return 0, pgerr.Translate(r.pool, "admin market data upsert hourly prices", err)
	}
	return n, nil
}

// UpsertYekdem implements store.AdminMarketDataRepository.UpsertYekdem.
func (r *MarketDataRepository) UpsertYekdem(ctx context.Context, values []model.YekdemMonthly) (int64, error) {
	if len(values) == 0 {
		return 0, nil
	}

	years := make([]int16, len(values))
	months := make([]int16, len(values))
	amounts := make([]pgtype.Numeric, len(values))
	fetchedAt := make([]pgtype.Timestamptz, len(values))
	for i, v := range values {
		years[i] = v.Year
		months[i] = v.Month
		amounts[i] = decimalToNumeric(v.Value)
		fetchedAt[i] = pgtype.Timestamptz{Time: fetchedAtOrNow(v.FetchedAt), Valid: true}
	}

	n, err := r.q.AdminMarketDataUpsertYekdem(ctx, sqlcgen.AdminMarketDataUpsertYekdemParams{
		Year:      years,
		Month:     months,
		Value:     amounts,
		FetchedAt: fetchedAt,
	})
	if err != nil {
		return 0, pgerr.Translate(r.pool, "admin market data upsert yekdem", err)
	}
	return n, nil
}
