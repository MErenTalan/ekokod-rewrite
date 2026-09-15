package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgnum"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

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
//
// Refuses the whole call, before any database round trip, on a zero Ts (an
// unset/forgotten field, never a legitimate hour — market_prices_hourly's
// primary key is ts, and a zero Time is never a real market hour) or on two
// entries sharing the same Ts (an ambiguous import must not silently let
// `on conflict` pick a winner between them).
func (r *MarketDataRepository) UpsertHourlyPrices(ctx context.Context, prices []model.MarketPrice) (int64, error) {
	if len(prices) == 0 {
		return 0, nil
	}
	if err := validateHourlyPrices(prices); err != nil {
		return 0, err
	}

	ts := make([]pgtype.Timestamptz, len(prices))
	ptf := make([]pgtype.Numeric, len(prices))
	fetchedAt := make([]pgtype.Timestamptz, len(prices))
	for i, p := range prices {
		ts[i] = pgtype.Timestamptz{Time: p.Ts, Valid: true}
		ptf[i] = pgnum.DecimalToNumeric(p.PTF)
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
//
// Refuses the whole call, before any database round trip, on a Month outside
// 1..12 or on two entries sharing the same (Year, Month) — yekdem_monthly's
// primary key — for the same reason UpsertHourlyPrices refuses a duplicate
// Ts.
func (r *MarketDataRepository) UpsertYekdem(ctx context.Context, values []model.YekdemMonthly) (int64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	if err := validateYekdemValues(values); err != nil {
		return 0, err
	}

	years := make([]int16, len(values))
	months := make([]int16, len(values))
	amounts := make([]pgtype.Numeric, len(values))
	fetchedAt := make([]pgtype.Timestamptz, len(values))
	for i, v := range values {
		years[i] = v.Year
		months[i] = v.Month
		amounts[i] = pgnum.DecimalToNumeric(v.Value)
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

// validateHourlyPrices refuses the whole UpsertHourlyPrices call, before any
// database round trip, for a zero Ts or a Ts repeated within prices itself.
func validateHourlyPrices(prices []model.MarketPrice) error {
	seen := make(map[time.Time]struct{}, len(prices))
	for _, p := range prices {
		if p.Ts.IsZero() {
			return fmt.Errorf("%w: market price has a zero Ts", store.ErrConflict)
		}
		key := p.Ts.UTC()
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: duplicate market price ts=%s", store.ErrConflict, p.Ts.Format(time.RFC3339))
		}
		seen[key] = struct{}{}
	}
	return nil
}

// validateYekdemValues refuses the whole UpsertYekdem call, before any
// database round trip, for a Month outside 1..12 or a (Year, Month) repeated
// within values itself.
func validateYekdemValues(values []model.YekdemMonthly) error {
	type key struct {
		year, month int16
	}
	seen := make(map[key]struct{}, len(values))
	for _, v := range values {
		if v.Month < 1 || v.Month > 12 {
			return fmt.Errorf("%w: yekdem month %d out of range 1..12 (year=%d)", store.ErrConflict, v.Month, v.Year)
		}
		k := key{v.Year, v.Month}
		if _, ok := seen[k]; ok {
			return fmt.Errorf("%w: duplicate yekdem year=%d month=%d", store.ErrConflict, v.Year, v.Month)
		}
		seen[k] = struct{}{}
	}
	return nil
}
