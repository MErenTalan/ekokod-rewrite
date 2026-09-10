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
// deliberate no-op, and goose's own bookkeeping table and its sequence are not
// ours to drop.
//
// The guard queries pg_class rather than pg_tables because a continuous
// aggregate is a view over a materialisation hypertable that lives in
// _timescaledb_internal, so it never appears in pg_tables in schema public at
// all: a forgotten `drop materialized view` in 00005 would have passed the
// original query silently, which is exactly the omission this test exists to
// catch. Enum types are covered too, belt-and-braces — TestMigrateUpDownUp
// already catches a forgotten `drop type` indirectly, because the second `up`
// then fails on a `create type` that has no `if not exists`.
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
	rows, err := pool.Query(ctx, `
		select c.relname
		  from pg_class c
		  join pg_namespace n on n.oid = c.relnamespace
		 where n.nspname = 'public'
		   -- ordinary and partitioned tables, views, materialized views, sequences
		   and c.relkind in ('r', 'p', 'v', 'm', 'S')
		   -- goose's bookkeeping table and the sequence behind its id column
		   and c.relname not in ('goose_db_version', 'goose_db_version_id_seq')
		union all
		select t.typname
		  from pg_type t
		  join pg_namespace n on n.oid = t.typnamespace
		 where n.nspname = 'public' and t.typtype = 'e'`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		remaining = append(remaining, name)
	}
	require.NoError(t, rows.Err())
	require.Empty(t, remaining, "down migrations must drop every object they created")
}
