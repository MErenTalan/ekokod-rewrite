//go:build integration

package seed_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestE2EFixturesIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	hasher := auth.Hasher{Pepper: []byte("pepper-0123456789abcdef-0123456789"), Cost: bcrypt.MinCost}
	now := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)

	first, err := seed.E2EFixtures(ctx, pool, hasher, "Guvenli!Sifre-42", now)
	require.NoError(t, err)
	second, err := seed.E2EFixtures(ctx, pool, hasher, "Guvenli!Sifre-42", now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, first, second)

	var users, buildings, analyzers int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from users`).Scan(&users))
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from buildings`).Scan(&buildings))
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from analyzers`).Scan(&analyzers))
	require.Equal(t, []int{6, 3, 3}, []int{users, buildings, analyzers})

	a1, err := postgres.NewBuildingRepository(pool).Get(ctx, store.SystemScope(first.CompanyA), first.BuildingA1)
	require.NoError(t, err)
	require.Equal(t, first.Users[seed.E2EBuildingAdminEmail], *a1.ResponsibleUserID)
	ba, err := postgres.NewUserRepository(pool).Get(ctx, store.SystemScope(first.CompanyA), first.Users[seed.E2EBuildingAdminEmail])
	require.NoError(t, err)
	require.Equal(t, model.UserRoleBuildingAdmin, ba.Role)
}

// R194: the screens need real data in the e2e database, and a second seed run
// must not double it.
func TestE2EFixturesHaveReadingsAndBill(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	hasher := auth.Hasher{Pepper: []byte("pepper-0123456789abcdef-0123456789"), Cost: bcrypt.MinCost}
	now := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)

	fx, err := seed.E2EFixtures(ctx, pool, hasher, "Guvenli!Sifre-42", now)
	require.NoError(t, err)
	_, err = seed.E2EData(ctx, pool, fx, now)
	require.NoError(t, err)
	again, err := seed.E2EFixtures(ctx, pool, hasher, "Guvenli!Sifre-42", now)
	require.NoError(t, err)
	require.Equal(t, fx.CompanyA, again.CompanyA)
	_, err = seed.E2EData(ctx, pool, again, now)
	require.NoError(t, err)

	// F10a: A1's declaration and four manual carbon records, once.
	var selected, manual int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from carbon_selected_activities where building_id = $1`, fx.BuildingA1).Scan(&selected))
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from carbon_activities where building_id = $1 and not is_automated`, fx.BuildingA1).Scan(&manual))
	require.Equal(t, []int{len(seed.E2ECarbonSelection), 4}, []int{selected, manual})

	for _, id := range []uuid.UUID{fx.AnalyzerA1, fx.AnalyzerA2, fx.AnalyzerB1} {
		var readings int
		require.NoError(t, pool.QueryRow(ctx, `select count(*) from meter_readings where analyzer_id = $1`, id).Scan(&readings))
		require.Equal(t, 75*24+1, readings, "75 days of hourly readings, no duplicates after a second run")
	}
	var daily int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from consumption_daily where analyzer_id = $1`, fx.AnalyzerA1).Scan(&daily))
	require.Greater(t, daily, 70, "the daily aggregate is materialised for the seeded range")

	// One building invoice and one analyzer invoice for A1, after two runs:
	// the count per scope is what proves the seed is idempotent, and F8a's
	// dashboard needs the analyzer row next to the building one.
	var buildingBills int
	var total decimal.Decimal
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*), coalesce(max(total_cost), 0) from bills
		 where building_id = $1 and scope = 'building' and status <> 'superseded'`,
		fx.BuildingA1).Scan(&buildingBills, &total))
	require.Equal(t, 1, buildingBills, "exactly one building bill after two seed runs")
	require.Equal(t, "48250.75", total.String())

	var analyzerBills int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from bills where analyzer_id = $1 and scope = 'analyzer' and status <> 'superseded'`,
		fx.AnalyzerA1).Scan(&analyzerBills))
	require.Equal(t, 1, analyzerBills, "exactly one analyzer bill after two seed runs")

	// 02 §6.11 prices the building invoice over the aggregate, so the seeded
	// figures differ on purpose: the dashboard's divergence note (R234) is
	// exercised by real data rather than by a hand-made fixture.
	var analyzerSum decimal.Decimal
	require.NoError(t, pool.QueryRow(ctx,
		`select coalesce(sum(total_cost), 0) from bills
		 where company_id = $1 and scope = 'analyzer' and status <> 'superseded'`,
		fx.CompanyA).Scan(&analyzerSum))
	require.NotEqual(t, total.String(), analyzerSum.String())

	var templates, assignments, imports, solar, schedule int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from tariff_templates where company_id = $1`, fx.CompanyA).Scan(&templates))
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from tariff_bulk_assignments where company_id = $1`, fx.CompanyA).Scan(&assignments))
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from icmal_imports where company_id = $1`, fx.CompanyA).Scan(&imports))
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from solar_tariffs where company_id = $1`, fx.CompanyA).Scan(&solar))
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from national_tariff_schedule`).Scan(&schedule))
	require.Equal(t, 1, templates, "one template after two seed runs")
	require.Equal(t, 1, assignments)
	require.Equal(t, 1, imports)
	require.Equal(t, 2, solar, "the rooftop plant's and, since F9, the linked grid plant's")
	require.GreaterOrEqual(t, schedule, 2)

	// F8b: a utility-scale plant with a yearly target and monthly production
	// (the yearly report's solar section), and one completed monthly report
	// for A1's previous month (the archive), each exactly once after two runs.
	var gridPlants int
	var target decimal.Decimal
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*), coalesce(max(yearly_target_kwh), 0) from power_plants where company_id = $1 and plant_kind = 'grid'`,
		fx.CompanyA).Scan(&gridPlants, &target))
	require.Equal(t, 1, gridPlants)
	require.True(t, target.IsPositive(), "the solar section has a target to compare with")
	var productionMonths int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from plant_production_monthly m join power_plants p on p.id = m.plant_id
		 where p.company_id = $1 and p.plant_kind = 'grid'`, fx.CompanyA).Scan(&productionMonths))
	require.GreaterOrEqual(t, productionMonths, 12, "a year of monthly production, materialised")
	var reports int
	var status string
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*), coalesce(max(status::text), '') from reports where building_id = $1 and type = 'monthly' and period = '2026-08'`,
		fx.BuildingA1).Scan(&reports, &status))
	require.Equal(t, 1, reports)
	require.Equal(t, "completed", status)
}

// F9: the grid plant is linked, has live inverters, sixty days of totals,
// faults, a netting analyzer and a feed-in tariff; a second run changes nothing.
func TestE2EFixturesLinkASolarPlant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	hasher := auth.Hasher{Pepper: []byte("pepper-0123456789abcdef-0123456789"), Cost: bcrypt.MinCost}
	now := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)
	fx, err := seed.E2EFixtures(ctx, pool, hasher, "Guvenli!Sifre-42", now)
	require.NoError(t, err)
	for range 2 {
		_, err = seed.E2EData(ctx, pool, fx, now)
		require.NoError(t, err)
	}

	sc := store.SystemScope(fx.CompanyA)
	plants := postgres.NewPlantRepository(pool)
	linked, err := plants.ListLinked(ctx, sc)
	require.NoError(t, err)
	require.Len(t, linked, 1)
	plant := linked[0]
	require.Equal(t, seed.E2EISolarPsID, *plant.IsolarPsID)
	require.Equal(t, fx.AnalyzerA1, *plant.NettingAnalyzerID)

	devices, err := plants.Devices(ctx, sc, plant.ID)
	require.NoError(t, err)
	snapshots := 0
	for _, d := range devices {
		if d.SnapshotAt != nil && d.ActivePowerKw != nil {
			snapshots++
		}
	}
	require.Equal(t, 2, snapshots)

	var days, faults, tariffs int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from plant_production_totals where plant_id = $1 and ts >= $2`,
		plant.ID, now.AddDate(0, 0, -61)).Scan(&days))
	require.Equal(t, 60, days)
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from plant_faults where plant_id = $1`, plant.ID).Scan(&faults))
	require.Equal(t, 2, faults)
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from solar_tariffs where plant_id = $1`, plant.ID).Scan(&tariffs))
	require.Equal(t, 1, tariffs)
}
