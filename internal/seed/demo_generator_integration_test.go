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
	"github.com/MErenTalan/ekokod-rewrite/internal/storage"
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
	billing, err := NewDemoBilling(pool, discardLog)
	require.NoError(t, err)
	files := storage.Store{Root: t.TempDir(), Max: 10 << 20, Allowed: []string{"application/pdf"}}
	return DemoDeps{Pool: pool, Hasher: auth.Hasher{Cost: 4}, Cipher: cipher, Billing: billing, Storage: files, Log: discardLog}
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
	// The UI labels an analyzer by its customer name, so each needs its own.
	require.Equal(t, len(b.analyzers), countRows(t, d.Pool,
		`select count(distinct customer_name) from analyzers where company_id = $1`, DemoCompanyID))

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

func TestDemoMoneyBills(t *testing.T) {
	t.Parallel()
	d, b, ctx := demoDeps(t), DemoBaseline(), t.Context()
	require.NoError(t, ensureDemoSites(ctx, d, b, demoNow))
	require.NoError(t, ensureDemoPeople(ctx, d, b, "Guvenli!Sifre-42", demoNow))
	_, err := ensureDemoSeries(ctx, d, b, demoNow)
	require.NoError(t, err)
	require.NoError(t, ensureDemoMoney(ctx, d, b, demoNow))

	live := `select count(*) from bills where company_id = $1 and scope = $2 and status <> 'superseded' and total_cost > 0
		and period_key between '2025-10' and '2026-09'`
	require.Equal(t, 3*12, countRows(t, d.Pool, live, DemoCompanyID, "building"))
	require.Equal(t, 6*12, countRows(t, d.Pool, live, DemoCompanyID, "analyzer"))
	require.Equal(t, 12, countRows(t, d.Pool, live, DemoCompanyID, "company"), "the EKORM dashboard reads the company bill")
	require.Zero(t, countRows(t, d.Pool, `select count(*) from bills where company_id = $1 and total_cost <= 0`, DemoCompanyID))
	var flagged string
	require.NoError(t, d.Pool.QueryRow(ctx, `select coalesce(string_agg(distinct scope || ':' || period_key || ':' || flag_reason, ' '), '')
		from bills where company_id = $1 and status = 'flagged'`, DemoCompanyID).Scan(&flagged))
	require.Empty(t, flagged, "a flagged bill shows the visitor an İncelenmeli badge")
	reactive := `select count(*) from bill_lines l join bills b on b.id = l.bill_id
		where b.analyzer_id = $1 and b.scope = 'analyzer' and l.code = 'reactive_inductive' and l.amount > 0`
	require.Positive(t, countRows(t, d.Pool, reactive, b.analyzers[2].id), "the compressor pays a reactive penalty in summer")
	sale := `select count(*) from bill_lines l join bills b on b.id = l.bill_id
		where b.building_id = $1 and l.code = 'generation_sale'`
	require.Positive(t, countRows(t, d.Pool, sale, b.buildings[2].id), "Gebze nets its rooftop export")
	require.Equal(t, 3, countRows(t, d.Pool, `select count(*) from tariffs where company_id = $1 and deleted_at is null`, DemoCompanyID))
	require.Equal(t, 1, countRows(t, d.Pool, `select count(*) from tariff_templates where company_id = $1`, DemoCompanyID))
	require.Equal(t, 1, countRows(t, d.Pool, `select count(*) from tariff_bulk_assignments where company_id = $1`, DemoCompanyID))
	require.Equal(t, 12, countRows(t, d.Pool, `select count(*) from icmal_rows r join icmal_imports i on i.id = r.import_id
		where i.company_id = $1`, DemoCompanyID))
	require.Equal(t, 2, countRows(t, d.Pool, `select count(*) from solar_tariffs where company_id = $1`, DemoCompanyID))

	before := countRows(t, d.Pool, `select count(*) from bills where company_id = $1`, DemoCompanyID)
	require.NoError(t, ensureDemoMoney(ctx, d, b, demoNow))
	require.Equal(t, before, countRows(t, d.Pool, `select count(*) from bills where company_id = $1`, DemoCompanyID), "a rerun creates nothing")
	require.Equal(t, 3, countRows(t, d.Pool, `select count(*) from tariffs where company_id = $1`, DemoCompanyID))
}

func TestDemoContentFillsEveryScreen(t *testing.T) {
	t.Parallel()
	d, b, ctx := demoDeps(t), DemoBaseline(), t.Context()
	_, _, err := loadEmissionFactors(ctx, admin.NewCatalogueRepository(d.Pool), discardLog)
	require.NoError(t, err)
	require.NoError(t, ensureDemoSites(ctx, d, b, demoNow))
	require.NoError(t, ensureDemoPeople(ctx, d, b, "Guvenli!Sifre-42", demoNow))
	_, err = ensureDemoSeries(ctx, d, b, demoNow)
	require.NoError(t, err)
	require.NoError(t, ensureDemoContent(ctx, d, b, demoNow))

	id := DemoCompanyID
	counts := map[string]string{
		"alarms":     `select count(*) from alarms where company_id = $1 and deleted_at is null`,
		"events":     `select count(*) from alarm_events e join alarms a on a.id = e.alarm_id where a.company_id = $1`,
		"messages":   `select count(*) from operational_messages where company_id = $1`,
		"job_runs":   `select count(*) from job_runs where company_id = $1`,
		"anomalies":  `select count(*) from consumption_anomalies x join analyzers a on a.id = x.analyzer_id where a.company_id = $1`,
		"resolved":   `select count(*) from consumption_anomalies x join analyzers a on a.id = x.analyzer_id where a.company_id = $1 and x.resolved_at is not null`,
		"reports":    `select count(*) from reports where company_id = $1 and status = 'completed'`,
		"iso":        `select count(*) from iso50001_projects where company_id = $1`,
		"notes":      `select count(*) from iso50001_notes n join iso50001_projects p on p.id = n.project_id where p.company_id = $1`,
		"files":      `select count(*) from stored_files where company_id = $1 and deleted_at is null`,
		"activities": `select count(*) from carbon_activities where company_id = $1`,
		"carbon":     `select count(*) from carbon_reports where company_id = $1`,
		"forecasts":  `select count(*) from forecasts f join analyzers a on a.id = f.analyzer_id where a.company_id = $1`,
	}
	got := map[string]int{}
	for k, q := range counts {
		got[k] = countRows(t, d.Pool, q, id)
	}
	require.Equal(t, 5, got["alarms"])
	require.GreaterOrEqual(t, got["events"], 35)
	require.GreaterOrEqual(t, got["messages"], 30)
	require.GreaterOrEqual(t, got["job_runs"], 24)
	require.Equal(t, 6, got["anomalies"])
	require.Equal(t, 2, got["resolved"])
	require.Equal(t, 3*3+1, got["reports"])
	require.Equal(t, 1, got["iso"])
	require.Equal(t, 4, got["notes"])
	require.Equal(t, 3, got["files"])
	require.GreaterOrEqual(t, got["activities"], 60)
	require.Equal(t, 3, got["carbon"])
	for _, x := range b.buildings {
		require.Equal(t, 1, countRows(t, d.Pool, `select count(*) from carbon_reports where building_id = $1`, x.id),
			"EKOCM lists reports per building; %s has one", x.name)
	}
	ahead := `select count(*) from forecasts where analyzer_id = $1 and ts >= $2`
	for _, a := range b.analyzers {
		require.Positive(t, countRows(t, d.Pool, ahead, a.id, demoNow.Add(7*24*time.Hour-2*time.Hour)), "%s forecast reaches a week ahead", a.number)
	}
	rows, err := d.Pool.Query(ctx, `select owner_type, stored_path from stored_files where company_id = $1`, id)
	require.NoError(t, err)
	for rows.Next() {
		var area, path string
		require.NoError(t, rows.Scan(&area, &path))
		f, err := d.Storage.Open(id, area, path)
		require.NoError(t, err, "the file is on disk")
		require.NoError(t, f.Close())
	}
	require.NoError(t, rows.Err())

	require.NoError(t, ensureDemoContent(ctx, d, b, demoNow))
	for k, q := range counts {
		require.Equal(t, got[k], countRows(t, d.Pool, q, id), "a rerun adds no %s", k)
	}
}
