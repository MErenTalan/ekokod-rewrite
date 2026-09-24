package admin

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/metrics"
)

// OpsHealthRepository reads operational health across tenants for the metrics collectors (F15b R463).
type OpsHealthRepository struct{ pool *pgxpool.Pool }

// NewOpsHealthRepository builds an OpsHealthRepository over pool.
func NewOpsHealthRepository(pool *pgxpool.Pool) *OpsHealthRepository {
	return &OpsHealthRepository{pool: pool}
}

// IntegrationRuns is each provider's newest successful and failed reading pull of the last week.
func (r *OpsHealthRepository) IntegrationRuns(ctx context.Context) ([]metrics.IntegrationRun, error) {
	rows, err := r.pool.Query(ctx, `select a.provider::text,
			max(j.finished_at) filter (where j.status = 'success'),
			max(j.finished_at) filter (where j.status = 'failed')
		from job_runs j join analyzers a on a.id = (j.scope->>'analyzer_id')::uuid
		where j.job_type = 'integration.fetch_readings' and j.started_at > now() - interval '7 days'
		group by 1 order by 1`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (metrics.IntegrationRun, error) {
		var run metrics.IntegrationRun
		return run, row.Scan(&run.Provider, &run.LastSuccess, &run.LastError)
	})
}
