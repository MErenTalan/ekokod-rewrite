//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// newPool opens a pool against dsn with the settings every migration test in
// this package wants: a small connection count and a statement timeout short
// enough that a runaway query fails the test instead of hanging it. Go forbids
// redeclaring a function across files in one package, so this and
// seedCompanyAndBuilding live here and are shared by every F1 migration test.
func newPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()

	pool, err := postgres.NewPool(context.Background(), config.DB{
		URL:              dsn,
		MaxConns:         4,
		MinConns:         1,
		MaxConnLifetime:  time.Hour,
		StatementTimeout: 10 * time.Second,
	}, discardLogger())
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// seedCompanyAndBuilding inserts the two rows that every tenant-scoped table
// added after migration 00003 needs a foreign key target for, and returns their
// ids. It writes only the not-null columns, so it keeps working as later
// migrations add optional ones.
func seedCompanyAndBuilding(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (companyID, buildingID uuid.UUID) {
	t.Helper()

	require.NoError(t, pool.QueryRow(ctx,
		`insert into companies (name) values ($1) returning id`,
		"seed company "+uuid.NewString(),
	).Scan(&companyID))

	require.NoError(t, pool.QueryRow(ctx,
		`insert into buildings (company_id, name) values ($1, $2) returning id`,
		companyID, "seed building",
	).Scan(&buildingID))

	return companyID, buildingID
}
