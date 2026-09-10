//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestEmissionFactorKeyUniqueness proves the coalesce-based unique index from
// 04-data-model.md §9: a company may override a master-catalogue key, but may
// not define the same key twice, and two companies may each hold their own
// copy of one key.
func TestEmissionFactorKeyUniqueness(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

	insert := func(company *uuid.UUID, key string) error {
		_, err := pool.Exec(ctx, `insert into emission_factors
			(company_id, key, label, main_category, base_factor, base_unit)
			values ($1,$2,'l','c',1,'kg')`, company, key)
		return err
	}
	var a, b uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `insert into companies (name) values ('a') returning id`).Scan(&a))
	require.NoError(t, pool.QueryRow(ctx, `insert into companies (name) values ('b') returning id`).Scan(&b))

	require.NoError(t, insert(nil, "diesel"), "the master catalogue may hold a key")
	require.Error(t, insert(nil, "diesel"), "the master catalogue may not hold it twice")
	require.NoError(t, insert(&a, "diesel"), "a company may override a master key")
	require.Error(t, insert(&a, "diesel"), "a company may not hold the same key twice")
	require.NoError(t, insert(&b, "diesel"), "a second company keeps its own copy")
}

// TestAutomatedCarbonActivitiesAreUniquePerDay guards the partial unique index
// "one automated record per building, activity type and day". Manual records
// are deliberately exempt.
func TestAutomatedCarbonActivitiesAreUniquePerDay(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))
	pool := testfixtures.NewPool(t, dsn)

	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)
	insert := func(automated bool) error {
		_, err := pool.Exec(ctx, `insert into carbon_activities
			(company_id, building_id, main_category, sub_category, activity_type,
			 period_start, period_end, quantity, unit, emission_kgco2e, scope,
			 iso_category, is_automated)
			values ($1,$2,'energy','grid','electricity','2026-01-01','2026-01-01',
			        1,'kWh',1,'scope_2','cat1',$3)`, companyID, buildingID, automated)
		return err
	}
	require.NoError(t, insert(true))
	require.Error(t, insert(true), "a second automated row for the same day must be rejected")
	require.NoError(t, insert(false), "manual rows are exempt from the partial index")
	require.NoError(t, insert(false), "and may repeat")
}
