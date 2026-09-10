//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

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
