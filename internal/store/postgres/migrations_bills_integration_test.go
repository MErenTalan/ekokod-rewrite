//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// TestOneLiveBillPerScopeAndPeriod covers 04-data-model.md §14: the unique
// index enforces one live bill per scope and period, and recomputation
// supersedes rather than deletes, preserving what was issued.
func TestOneLiveBillPerScopeAndPeriod(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)
	insert := func(status string) (uuid.UUID, error) {
		var id uuid.UUID
		err := pool.QueryRow(ctx, `insert into bills
			(company_id, building_id, scope, period_key, period_start, period_end,
			 days_in_period, index_start, index_end, status)
			values ($1,$2,'building','2026-01','2026-01-01','2026-02-01',31,'{}','{}',$3)
			returning id`, companyID, buildingID, status).Scan(&id)
		return id, err
	}

	first, err := insert("issued")
	require.NoError(t, err)

	_, err = insert("draft")
	require.Error(t, err, "a second live bill for the same scope and period must be rejected")
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "the rejection must come from postgres, not a driver-side error")
	require.Equal(t, "23505", pgErr.Code,
		"the rejection must be the unique_violation from the partial index, not an unrelated not-null violation")

	_, err = pool.Exec(ctx, `update bills set status = 'superseded' where id = $1`, first)
	require.NoError(t, err)
	_, err = insert("draft")
	require.NoError(t, err, "superseding the previous bill must free the period")

	var kept int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from bills`).Scan(&kept))
	require.Equal(t, 2, kept, "the superseded bill must be preserved, not deleted")
}
