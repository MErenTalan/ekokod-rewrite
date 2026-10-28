//go:build integration

package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestDemoTablesCoverSchema is R549: every table that reaches a company is
// classified, so a future table cannot keep demo writes past a reset.
func TestDemoTablesCoverSchema(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	require.Empty(t, unclassified(t, pool))

	_, err := pool.Exec(t.Context(), `create table f29_probe (id uuid primary key, building_id uuid references buildings(id))`)
	require.NoError(t, err)
	require.Equal(t, []string{"f29_probe"}, unclassified(t, pool), "the guard must see a new company table")
}

// unclassified walks company_id columns and FK chains to them.
func unclassified(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
with recursive reach(name) as (
    select 'companies'::text
    union
    select c.table_name::text from information_schema.columns c
    join information_schema.tables tb on tb.table_schema = c.table_schema and tb.table_name = c.table_name
    where c.table_schema = 'public' and c.column_name = 'company_id' and tb.table_type = 'BASE TABLE'
    union
    select k.conrelid::regclass::text from pg_constraint k join reach r on k.confrelid::regclass::text = r.name
    where k.contype = 'f'
)
select name from reach order by name`)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		if _, ok := admin.DemoTables[name]; !ok {
			out = append(out, name)
		}
	}
	require.NoError(t, rows.Err())
	return out
}

type purgeFixture struct {
	demo, other testfixtures.Tenant
	keep        admin.DemoKeep
	visitor     model.User
	extra       uuid.UUID // a non-kept analyzer with readings
	from, to    time.Time
}

func newPurgeFixture(t *testing.T, pool *pgxpool.Pool) purgeFixture {
	t.Helper()
	ctx := t.Context()
	f := purgeFixture{demo: testfixtures.NewTenant(t, ctx, pool, 1), other: testfixtures.NewTenant(t, ctx, pool, 2)}
	require.NoError(t, admin.NewTenantRepository(pool).SetDemo(ctx, f.demo.Company.ID, true))
	f.keep = admin.DemoKeep{
		Buildings: []uuid.UUID{f.demo.Buildings[0].ID},
		Analyzers: []uuid.UUID{f.demo.Analyzers[0].ID, f.demo.Analyzers[1].ID},
		User:      f.demo.Users[model.UserRoleDemo].ID,
	}
	for _, p := range f.demo.Plants {
		f.keep.Plants = append(f.keep.Plants, p.ID)
	}
	f.visitor = f.demo.Users[model.UserRoleCompanyAdmin]
	f.extra = f.demo.Analyzers[len(f.demo.Analyzers)-1].ID
	f.to = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	f.from = f.to.Add(-48 * time.Hour)
	readings := postgres.NewReadingRepository(pool)
	for _, tc := range []struct {
		sc store.Scope
		id uuid.UUID
	}{{f.demo.AdminScope, f.keep.Analyzers[0]}, {f.demo.AdminScope, f.extra}, {f.other.AdminScope, f.other.Analyzers[0].ID}} {
		_, _, err := readings.BulkInsert(ctx, tc.sc, testfixtures.HourlyReadings(tc.id, f.from, f.to, testfixtures.Constant("10")))
		require.NoError(t, err)
	}
	for _, tn := range []testfixtures.Tenant{f.demo, f.other} {
		_, err := pool.Exec(ctx, `insert into alarms (company_id, name, type) values ($1, 'a', 'data_communication')`, tn.Company.ID)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `insert into stored_files (company_id, owner_type, original_name, stored_path, content_type, size_bytes, checksum)
			values ($1, 'iso50001', 'x.pdf', $2, 'application/pdf', 1, 'c')`, tn.Company.ID, tn.Company.ID.String()+"/iso50001/x.pdf")
		require.NoError(t, err)
	}
	for _, u := range []uuid.UUID{f.keep.User, f.visitor.ID} {
		_, err := pool.Exec(ctx, `insert into sessions (user_id, refresh_token_hash, expires_at) values ($1, $2, now() + interval '1 day')`,
			u, uuid.NewString())
		require.NoError(t, err)
	}
	return f
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(t.Context(), q, args...).Scan(&n))
	return n
}

func TestPurgeRefusesNonDemoCompany(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	f := newPurgeFixture(t, pool)
	_, err := admin.NewDemoRepository(pool).Purge(t.Context(), f.other.Company.ID, admin.DemoKeep{})
	require.Error(t, err)
	require.Equal(t, 1, count(t, pool, `select count(*) from alarms where company_id = $1`, f.other.Company.ID))
}

func TestPurgeKeepsKeptRowsAndSeries(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	f := newPurgeFixture(t, pool)
	id := f.demo.Company.ID
	kept := count(t, pool, `select count(*) from meter_readings where analyzer_id = $1`, f.keep.Analyzers[0])

	res, err := admin.NewDemoRepository(pool).Purge(t.Context(), id, f.keep)
	require.NoError(t, err)

	require.Equal(t, kept, count(t, pool, `select count(*) from meter_readings where analyzer_id = $1`, f.keep.Analyzers[0]))
	require.Zero(t, count(t, pool, `select count(*) from meter_readings where analyzer_id = $1`, f.extra))
	require.Zero(t, count(t, pool, `select count(*) from analyzers where id = $1`, f.extra))
	require.Equal(t, 2, count(t, pool, `select count(*) from analyzers where company_id = $1`, id))
	require.Equal(t, 1, count(t, pool, `select count(*) from buildings where company_id = $1`, id))
	require.Zero(t, count(t, pool, `select count(*) from alarms where company_id = $1`, id))
	require.Zero(t, count(t, pool, `select count(*) from tariffs where company_id = $1`, id))
	require.Equal(t, 1, count(t, pool, `select count(*) from users where company_id = $1`, id))
	require.Equal(t, 1, count(t, pool, `select count(*) from sessions where user_id = $1`, f.keep.User), "R546: the shared session survives")
	require.Zero(t, count(t, pool, `select count(*) from sessions where user_id = $1`, f.visitor.ID))
	require.Equal(t, len(f.keep.Plants), count(t, pool, `select count(*) from power_plants where company_id = $1`, id))
	require.Equal(t, []string{id.String() + "/iso50001/x.pdf"}, res.Files)
	require.NotNil(t, res.ReadingRange)
	require.Equal(t, f.from, res.ReadingRange.From.UTC())
	require.Equal(t, f.to.Add(-time.Hour), res.ReadingRange.To.UTC(), "HourlyReadings is half-open")
	require.Nil(t, res.PlantRange)
}

func TestPurgeLeavesOtherCompanies(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	f := newPurgeFixture(t, pool)
	tables := []string{"alarms", "analyzers", "buildings", "users", "tariffs", "power_plants", "stored_files"}
	before := map[string]int{}
	for _, tb := range tables {
		before[tb] = count(t, pool, `select count(*) from `+tb+` where company_id = $1`, f.other.Company.ID)
	}
	readings := count(t, pool, `select count(*) from meter_readings where analyzer_id = $1`, f.other.Analyzers[0].ID)

	_, err := admin.NewDemoRepository(pool).Purge(context.Background(), f.demo.Company.ID, f.keep)
	require.NoError(t, err)

	for _, tb := range tables {
		require.Equal(t, before[tb], count(t, pool, `select count(*) from `+tb+` where company_id = $1`, f.other.Company.ID), tb)
	}
	require.Equal(t, readings, count(t, pool, `select count(*) from meter_readings where analyzer_id = $1`, f.other.Analyzers[0].ID))
}
