//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/stretchr/testify/require"
)

// TestMigrationsLeaveNoTablesBehind proves `down` is genuinely reversible
// rather than merely non-erroring: after down, no table this phase created may
// remain. TestMigrateUpDownUp already covers that up -> down -> up succeeds,
// but a down section that forgets a `drop table` still succeeds — the forgotten
// table simply survives, and the next `up` then fails on an object that already
// exists, or worse, silently reuses stale rows. This test catches that at the
// point the omission is made. Extensions are exempt: 00001's down is a
// deliberate no-op, and goose's own bookkeeping table is not ours to drop.
//
// The schema is populated before the rollback: a `down` exercised only against
// empty tables is a weaker claim than the one production makes of it, and the
// seeded rows put the foreign keys between companies, buildings and everything
// hanging off them under real load while the drops run in reverse order.
func TestMigrationsLeaveNoTablesBehind(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))

	pool := newPool(t, dsn)
	seedCompanyAndBuilding(t, ctx, pool)

	require.NoError(t, postgres.MigrateDownAll(ctx, dsn, discardLogger()))
	var remaining []string
	rows, err := pool.Query(ctx, `select tablename from pg_tables
		where schemaname = 'public' and tablename <> 'goose_db_version'`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		remaining = append(remaining, name)
	}
	require.NoError(t, rows.Err())
	require.Empty(t, remaining, "down migrations must drop every table they created")
}
