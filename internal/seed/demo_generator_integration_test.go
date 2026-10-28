//go:build integration

package seed

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// demoNow is a Wednesday afternoon in Istanbul.
var demoNow = time.Date(2026, 10, 28, 14, 20, 0, 0, istanbul)

func demoDeps(t *testing.T) DemoDeps {
	t.Helper()
	pool := testfixtures.NewIsolatedDB(t)
	_, err := loadIntegrationDefinitions(t.Context(), admin.NewCatalogueRepository(pool), discardLog)
	require.NoError(t, err)
	cipher, err := crypto.NewCipher([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	return DemoDeps{Pool: pool, Hasher: auth.Hasher{Cost: 4}, Cipher: cipher, Log: discardLog}
}

func countRows(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(t.Context(), q, args...).Scan(&n))
	return n
}

func TestDemoBaselineIsDeterministic(t *testing.T) {
	a, b := DemoBaseline(), DemoBaseline()
	require.Equal(t, a, b)
	require.Len(t, a.buildings, 3)
	require.Len(t, a.analyzers, 6)
	require.Len(t, a.plants, 2)
	require.Len(t, a.colleagues, 4)
	require.Equal(t, DemoCompanyID, a.company)
	require.ElementsMatch(t, a.Keep().Analyzers, DemoAnalyzerIDs)
}

func TestEnsureDemoSitesConverges(t *testing.T) {
	t.Parallel()
	d, b, ctx := demoDeps(t), DemoBaseline(), t.Context()
	require.NoError(t, ensureDemoSites(ctx, d, b, demoNow))
	sc := store.SystemScope(DemoCompanyID)
	company, err := postgres.NewCompanyRepository(d.Pool).Get(ctx, sc, DemoCompanyID)
	require.NoError(t, err)
	require.True(t, company.IsDemo)
	require.Equal(t, "Demo Enerji A.Ş.", company.Name)

	buildings := postgres.NewBuildingRepository(d.Pool)
	office := b.buildings[1]
	got, err := buildings.Get(ctx, sc, office.id)
	require.NoError(t, err)
	got.Name = "Ziyaretçi değiştirdi"
	_, err = buildings.Update(ctx, sc, got)
	require.NoError(t, err)
	require.NoError(t, postgres.NewAnalyzerRepository(d.Pool).SoftDelete(ctx, sc, b.analyzers[2].id, demoNow))
	require.NoError(t, postgres.NewPlantRepository(d.Pool).SoftDelete(ctx, sc, b.plants[1].id, demoNow))

	require.NoError(t, ensureDemoSites(ctx, d, b, demoNow.Add(time.Hour)))
	got, err = buildings.Get(ctx, sc, office.id)
	require.NoError(t, err)
	require.Equal(t, office.name, got.Name)
	_, err = postgres.NewAnalyzerRepository(d.Pool).Get(ctx, sc, b.analyzers[2].id)
	require.NoError(t, err, "a soft-deleted baseline analyzer is restored")
	_, err = postgres.NewPlantRepository(d.Pool).Get(ctx, sc, b.plants[1].id)
	require.NoError(t, err)
	require.Equal(t, 3, countRows(t, d.Pool, `select count(*) from buildings where company_id = $1`, DemoCompanyID))
	require.Equal(t, 6, countRows(t, d.Pool, `select count(*) from analyzers where company_id = $1`, DemoCompanyID))
	require.Equal(t, 6, countRows(t, d.Pool, `select count(*) from power_plant_devices where plant_id = any($1)`, b.Keep().Plants))
}

func TestDemoSeriesFillsWindow(t *testing.T) {
	t.Parallel()
	d, b, ctx := demoDeps(t), DemoBaseline(), t.Context()
	require.NoError(t, ensureDemoSites(ctx, d, b, demoNow))
	n, err := ensureDemoSeries(ctx, d, b, demoNow)
	require.NoError(t, err)
	hours := int(demoHistory/time.Hour) + 1
	require.Equal(t, 6*hours, n)
	for _, a := range b.analyzers {
		require.Equal(t, hours, countRows(t, d.Pool, `select count(*) from meter_readings where analyzer_id = $1`, a.id), a.number)
	}
	again, err := ensureDemoSeries(ctx, d, b, demoNow)
	require.NoError(t, err)
	require.Zero(t, again)

	warehouse := b.analyzers[4]
	require.Equal(t, "DEMO-3001", warehouse.number)
	weekendNoonExport := `select count(*) from (
		select ts, active_export - lag(active_export) over (order by ts) as step from meter_readings where analyzer_id = $1) s
		where step > 0 and extract(isodow from ts at time zone 'Europe/Istanbul') in (6, 7)
		and extract(hour from ts at time zone 'Europe/Istanbul') = 13`
	require.Positive(t, countRows(t, d.Pool, weekendNoonExport, warehouse.id), "the rooftop out-produces the warehouse at a weekend noon")
	require.Zero(t, countRows(t, d.Pool, `select count(*) from meter_readings where analyzer_id = $1 and active_export is not null`, b.analyzers[0].id))
	night := `select count(*) from plant_production_totals where plant_id = $1 and extract(hour from ts at time zone 'Europe/Istanbul') = 2 and production_kwh > 0`
	require.Zero(t, countRows(t, d.Pool, night, b.plants[1].id))
	require.Positive(t, countRows(t, d.Pool, `select count(*) from plant_production_totals where plant_id = $1 and production_kwh > 0`, b.plants[1].id))
	require.Positive(t, countRows(t, d.Pool, `select count(*) from consumption_daily where analyzer_id = $1`, b.analyzers[0].id),
		"the bounded refresh materialises history")
}

func TestEnsureDemoPeopleConverges(t *testing.T) {
	t.Parallel()
	d, b, ctx := demoDeps(t), DemoBaseline(), t.Context()
	require.NoError(t, ensureDemoSites(ctx, d, b, demoNow))
	require.Error(t, ensureDemoPeople(ctx, d, b, "", demoNow), "the first run needs a password")
	require.NoError(t, ensureDemoPeople(ctx, d, b, "Guvenli!Sifre-42", demoNow))
	users := postgres.NewUserRepository(d.Pool)
	sc := store.SystemScope(DemoCompanyID)
	demo, err := users.Get(ctx, sc, DemoUserID)
	require.NoError(t, err)
	require.Equal(t, model.UserRoleDemo, demo.Role)
	hash := demo.PasswordHash

	require.NoError(t, ensureDemoPeople(ctx, d, b, "", demoNow.Add(time.Hour)))
	demo, err = users.Get(ctx, sc, DemoUserID)
	require.NoError(t, err)
	require.Equal(t, hash, demo.PasswordHash, "a reset keeps the password")
	require.Equal(t, 5, countRows(t, d.Pool, `select count(*) from users where company_id = $1 and deleted_at is null`, DemoCompanyID))
	require.Equal(t, 2, countRows(t, d.Pool, `select count(*) from company_weekend_days where company_id = $1`, DemoCompanyID))
	require.GreaterOrEqual(t, countRows(t, d.Pool, `select count(*) from company_vacations where company_id = $1`, DemoCompanyID), 16)
	require.GreaterOrEqual(t, countRows(t, d.Pool, `select count(*) from calendar_events where company_id = $1`, DemoCompanyID), 3)
	require.Equal(t, 2, countRows(t, d.Pool, `select count(*) from integration_credentials where company_id = $1 and last_verified_at is not null`, DemoCompanyID))
	require.Equal(t, 1, countRows(t, d.Pool, `select count(*) from smtp_settings where company_id = $1`, DemoCompanyID))
	konya, err := postgres.NewPlantRepository(d.Pool).Get(ctx, sc, b.plants[1].id)
	require.NoError(t, err)
	require.NotNil(t, konya.IsolarCredentialID, "the Konya plant is linked to the seeded iSolar credential")
}
