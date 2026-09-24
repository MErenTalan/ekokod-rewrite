package admin

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
)

// aggregateRefreshSQL calls TimescaleDB's refresh_continuous_aggregate as a
// top-level statement on the pool — never inside a pgx.Tx, which would fail
// with SQLSTATE 25001 because the procedure commits its own work. The window
// parameters are polymorphic in TimescaleDB and need explicit casts; the view
// name is bound as $1::regclass FROM a Go string that Refresh has already
// checked against the closed AggregateView set — never built with
// fmt.Sprintf — so an unknown or malicious name can never reach this
// statement.
const aggregateRefreshSQL = `call refresh_continuous_aggregate($1::regclass, $2::timestamptz, $3::timestamptz)`

// knownAggregateViews is the closed set Refresh validates view against before
// any I/O. Keeping it a map keyed by store.AggregateView (rather than, say,
// switching on view) makes the closed set the same shape as
// store.ConsumptionViews, so the two cannot drift silently.
var knownAggregateViews = func() map[store.AggregateView]bool {
	known := make(map[store.AggregateView]bool)
	for _, v := range store.ConsumptionViews() {
		known[v] = true
	}
	return known
}()

// AggregateRepository implements store.AdminAggregateRepository: refreshing
// TimescaleDB continuous aggregates. See doc.go for why this cannot take a
// Scope — a refresh window necessarily covers every tenant's buckets in it.
//
// Unlike every other repository in this package, AggregateRepository does not
// hold a *sqlcgen.Queries: refresh_continuous_aggregate is a stored
// PROCEDURE invoked with CALL, not a query, and its first argument is cast to
// ::regclass rather than a plain column type sqlc can infer — the same shape
// migrations_timeseries_integration_test.go and
// analytics_integration_test.go already use pool.Exec for directly, rather
// than through a generated query.
type AggregateRepository struct {
	pool *pgxpool.Pool
}

// NewAggregateRepository builds an AggregateRepository on pool.
func NewAggregateRepository(pool *pgxpool.Pool) *AggregateRepository {
	return &AggregateRepository{pool: pool}
}

var _ store.AdminAggregateRepository = (*AggregateRepository)(nil)

// Refresh implements store.AdminAggregateRepository.Refresh. view and r are
// both validated before any database round trip: an unknown view returns
// store.ErrUnknownView, an invalid r returns store.ErrInvalidRange.
func (r *AggregateRepository) Refresh(ctx context.Context, view store.AggregateView, rng store.TimeRange) error {
	if !knownAggregateViews[view] {
		return store.ErrUnknownView
	}
	if !rng.Valid() {
		return store.ErrInvalidRange
	}

	_, err := r.pool.Exec(ctx, aggregateRefreshSQL, string(view), rng.From, rng.To)
	if err != nil {
		return pgerr.Translate(r.pool, "refresh continuous aggregate "+string(view), err)
	}
	return nil
}

// plantViews are migration 00018's plant aggregates, finest first.
var plantViews = []string{"plant_production_daily", "plant_production_monthly"}

// RefreshPlantProduction refreshes the plant aggregates over rng (F14c R430);
// the view names are this closed list, never caller input.
func (r *AggregateRepository) RefreshPlantProduction(ctx context.Context, rng store.TimeRange) error {
	if !rng.Valid() {
		return store.ErrInvalidRange
	}
	for _, v := range plantViews {
		if _, err := r.pool.Exec(ctx, aggregateRefreshSQL, v, rng.From, rng.To); err != nil {
			return pgerr.Translate(r.pool, "refresh continuous aggregate "+v, err)
		}
	}
	return nil
}
