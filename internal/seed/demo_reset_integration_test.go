//go:build integration

package seed

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func seededDemo(t *testing.T) DemoDeps {
	t.Helper()
	d := demoDeps(t)
	_, err := Demo(t.Context(), d, "Guvenli!Sifre-42", demoNow)
	require.NoError(t, err)
	return d
}

func addSession(t *testing.T, pool *pgxpool.Pool, user uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, pool.QueryRow(t.Context(), `insert into sessions (user_id, refresh_token_hash, expires_at)
		values ($1, $2, now() + interval '1 day') returning id`, user, uuid.NewString()).Scan(&id))
	return id
}

// R547: a visitor's deletions and edits of the baseline come back.
func TestResetRestoresDeletedBaseline(t *testing.T) {
	t.Parallel()
	d, ctx := seededDemo(t), t.Context()
	sc, b := store.SystemScope(DemoCompanyID), DemoBaseline()
	perAnalyzer := `select count(*) from meter_readings where analyzer_id = $1`
	readings := map[uuid.UUID]int{}
	for _, id := range DemoAnalyzerIDs {
		readings[id] = countRows(t, d.Pool, perAnalyzer, id)
	}
	billsQ := `select count(*) from bills where company_id = $1`
	bills := countRows(t, d.Pool, billsQ, DemoCompanyID)
	require.Equal(t, 120, bills)

	buildings, analyzers := postgres.NewBuildingRepository(d.Pool), postgres.NewAnalyzerRepository(d.Pool)
	visitorBuilding, err := buildings.Create(ctx, sc, model.Building{CompanyID: DemoCompanyID, Name: "Ziyaretçi binası", BillCutoffDay: 1})
	require.NoError(t, err)
	require.NoError(t, buildings.SoftDelete(ctx, sc, DemoBuildingID, demoNow))
	a, err := analyzers.Get(ctx, sc, b.analyzers[1].id)
	require.NoError(t, err)
	a.MeteringPointName, a.BuildingID = ptr("Ziyaretçi adı"), &visitorBuilding.ID
	_, err = analyzers.Update(ctx, sc, a)
	require.NoError(t, err)
	tariffs := postgres.NewTariffRepository(d.Pool)
	list, err := tariffs.List(ctx, sc, store.TariffFilter{Page: store.Page{Limit: 1}})
	require.NoError(t, err)
	require.NotEmpty(t, list)
	require.NoError(t, tariffs.SoftDelete(ctx, sc, list[0].ID, demoNow))

	started := time.Now()
	res, err := ResetDemo(ctx, d, lock.NewMemory(time.Now), demoNow)
	require.NoError(t, err)
	t.Logf("ResetDemo took %s", time.Since(started).Round(time.Millisecond))
	require.False(t, res.Skipped)

	building, err := buildings.Get(ctx, sc, DemoBuildingID)
	require.NoError(t, err)
	require.Equal(t, "Ostim Fabrika", building.Name)
	a, err = analyzers.Get(ctx, sc, b.analyzers[1].id)
	require.NoError(t, err)
	require.Equal(t, b.analyzers[1].name, *a.MeteringPointName)
	require.Equal(t, b.buildings[b.analyzers[1].building].id, *a.BuildingID)
	require.Equal(t, 3, countRows(t, d.Pool, `select count(*) from tariffs where company_id = $1 and deleted_at is null`, DemoCompanyID))
	require.Zero(t, countRows(t, d.Pool, `select count(*) from buildings where id = $1`, visitorBuilding.ID))
	for id, n := range readings {
		require.Equal(t, n, countRows(t, d.Pool, perAnalyzer, id), "the kept series stays as it was")
	}
	require.Equal(t, bills, countRows(t, d.Pool, billsQ, DemoCompanyID))
}

// R546, R547: what a visitor added is gone after a reset, on disk too, while
// the shared demo session survives (TestResetKeepsDemoSessions folded in).
func TestResetRemovesVisitorRows(t *testing.T) {
	t.Parallel()
	d, ctx := seededDemo(t), t.Context()
	sc := store.SystemScope(DemoCompanyID)

	building, err := postgres.NewBuildingRepository(d.Pool).Create(ctx, sc, model.Building{CompanyID: DemoCompanyID, Name: "Ziyaretçi binası", BillCutoffDay: 1})
	require.NoError(t, err)
	analyzers := postgres.NewAnalyzerRepository(d.Pool)
	a, err := analyzers.Get(ctx, sc, DemoAnalyzerIDs[0])
	require.NoError(t, err)
	a.ID, a.InstallationNumber, a.MeteringPointName, a.BuildingID = uuid.Nil, "ZIYARETCI-1", ptr("Ziyaretçi sayacı"), &building.ID
	visitor, err := analyzers.Create(ctx, sc, a)
	require.NoError(t, err)
	from := time.Date(2026, 10, 20, 0, 0, 0, 0, istanbul)
	_, _, err = postgres.NewReadingRepository(d.Pool).BulkInsert(ctx, sc,
		testfixtures.HourlyReadings(visitor.ID, from, from.Add(10*time.Hour), testfixtures.Constant("10")))
	require.NoError(t, err)
	require.NoError(t, admin.NewAggregateRepository(d.Pool).Refresh(ctx, store.ViewConsumptionDaily,
		store.TimeRange{From: from, To: from.AddDate(0, 0, 1)}))
	dailyQ := `select count(*) from consumption_daily where analyzer_id = $1`
	require.Positive(t, countRows(t, d.Pool, dailyQ, visitor.ID))

	integrations := postgres.NewIntegrationRepository(d.Pool, d.Cipher)
	creds, err := integrations.ListCredentials(ctx, sc)
	require.NoError(t, err)
	var isolar model.IntegrationCredential
	for _, c := range creds {
		if c.ID == demoID("credential/"+string(model.IntegrationProviderISolar)) {
			isolar = c
		}
	}
	require.NotEqual(t, uuid.Nil, isolar.ID)
	demoSecret, _, err := integrations.OpenSecret(ctx, sc, isolar.ID)
	require.NoError(t, err)
	_, err = integrations.UpsertCredential(ctx, sc, isolar, []byte("visitor-real-token"), nil)
	require.NoError(t, err)

	doc, err := demoPDF("Ziyaretçi", []string{"x"}, demoNow)
	require.NoError(t, err)
	saved, err := d.Storage.Save(ctx, DemoCompanyID, "iso50001", "ziyaretci.pdf", bytes.NewReader(doc))
	require.NoError(t, err)
	_, err = postgres.NewFileRepository(d.Pool).Create(ctx, sc, model.StoredFile{CompanyID: DemoCompanyID, OwnerType: "iso50001",
		OriginalName: "ziyaretci.pdf", StoredPath: saved.Path, ContentType: saved.ContentType, SizeBytes: saved.Size, Checksum: saved.Checksum})
	require.NoError(t, err)
	uploaded := filepath.Join(d.Storage.Root, "iso50001", DemoCompanyID.String(), saved.Path)
	billPDF := filepath.Join(d.Storage.Root, "bills", DemoCompanyID.String(), uuid.NewString()+".pdf")
	require.NoError(t, os.MkdirAll(filepath.Dir(billPDF), 0o700))
	require.NoError(t, os.WriteFile(billPDF, doc, 0o600))

	user, err := postgres.NewUserRepository(d.Pool).Create(ctx, sc, model.User{CompanyID: DemoCompanyID, Name: "Ziyaretçi",
		Email: "ziyaretci@example.com", PasswordHash: "x", Role: model.UserRoleCompanyAdmin, IsActive: true})
	require.NoError(t, err)
	visitorSession := addSession(t, d.Pool, user.ID)
	demoSession := addSession(t, d.Pool, DemoUserID)

	res, err := ResetDemo(ctx, d, lock.NewMemory(time.Now), demoNow)
	require.NoError(t, err)
	require.False(t, res.Skipped)

	for q, arg := range map[string]any{
		`select count(*) from buildings where id = $1`: building.ID,
		`select count(*) from analyzers where id = $1`: visitor.ID,
		`select count(*) from users where id = $1`:     user.ID,
		`select count(*) from sessions where id = $1`:  visitorSession,
		dailyQ: visitor.ID,
		`select count(*) from meter_readings where analyzer_id = $1`: visitor.ID,
	} {
		require.Zero(t, countRows(t, d.Pool, q, arg), q)
	}
	require.Equal(t, 1, countRows(t, d.Pool, `select count(*) from sessions where id = $1`, demoSession), "R546: the shared session survives")
	secret, _, err := integrations.OpenSecret(ctx, sc, isolar.ID)
	require.NoError(t, err)
	require.Equal(t, demoSecret, secret, "a visitor's real token does not outlive the hour")
	require.Equal(t, 3, countRows(t, d.Pool, `select count(*) from stored_files where company_id = $1`, DemoCompanyID))
	for _, p := range []string{uploaded, billPDF} {
		_, err := os.Stat(p)
		require.ErrorIs(t, err, os.ErrNotExist, p)
	}
}

// tableDigest is one md5 over a company's rows of each table.
func tableDigest(t *testing.T, pool *pgxpool.Pool, company uuid.UUID) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, q := range map[string]string{
		"companies":      `select md5(coalesce(string_agg(t::text, ',' order by t.id), '')) from companies t where id = $1`,
		"buildings":      `select md5(coalesce(string_agg(t::text, ',' order by t.id), '')) from buildings t where company_id = $1`,
		"analyzers":      `select md5(coalesce(string_agg(t::text, ',' order by t.id), '')) from analyzers t where company_id = $1`,
		"users":          `select md5(coalesce(string_agg(t::text, ',' order by t.id), '')) from users t where company_id = $1`,
		"alarms":         `select md5(coalesce(string_agg(t::text, ',' order by t.id), '')) from alarms t where company_id = $1`,
		"stored_files":   `select md5(coalesce(string_agg(t::text, ',' order by t.id), '')) from stored_files t where company_id = $1`,
		"power_plants":   `select md5(coalesce(string_agg(t::text, ',' order by t.id), '')) from power_plants t where company_id = $1`,
		"meter_readings": `select md5(coalesce(string_agg(t::text, ',' order by t.analyzer_id, t.ts), '')) from meter_readings t join analyzers a on a.id = t.analyzer_id where a.company_id = $1`,
		"sessions":       `select md5(coalesce(string_agg(t::text, ',' order by t.id), '')) from sessions t join users u on u.id = t.user_id where u.company_id = $1`,
	} {
		var sum string
		require.NoError(t, pool.QueryRow(t.Context(), q, company).Scan(&sum), name)
		out[name] = sum
	}
	return out
}

// R547: a reset touches nothing outside the demo company, on disk included.
func TestResetLeavesOtherCompaniesAlone(t *testing.T) {
	t.Parallel()
	d, ctx := seededDemo(t), t.Context()
	other := testfixtures.NewTenant(t, ctx, d.Pool, 7)
	osc := other.AdminScope
	from := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	_, _, err := postgres.NewReadingRepository(d.Pool).BulkInsert(ctx, osc,
		testfixtures.HourlyReadings(other.Analyzers[0].ID, from, from.Add(24*time.Hour), testfixtures.Constant("10")))
	require.NoError(t, err)
	_, err = d.Pool.Exec(ctx, `insert into alarms (company_id, name, type) values ($1, 'a', 'data_communication')`, other.Company.ID)
	require.NoError(t, err)
	addSession(t, d.Pool, other.Users[model.UserRoleCompanyAdmin].ID)
	saved, err := d.Storage.Save(ctx, other.Company.ID, "iso50001", "baska.pdf", bytes.NewReader([]byte("%PDF-1.4 x")))
	require.NoError(t, err)
	_, err = postgres.NewFileRepository(d.Pool).Create(ctx, osc, model.StoredFile{CompanyID: other.Company.ID, OwnerType: "iso50001",
		OriginalName: "baska.pdf", StoredPath: saved.Path, ContentType: saved.ContentType, SizeBytes: saved.Size, Checksum: saved.Checksum})
	require.NoError(t, err)
	before := tableDigest(t, d.Pool, other.Company.ID)

	_, err = ResetDemo(ctx, d, lock.NewMemory(time.Now), demoNow)
	require.NoError(t, err)

	require.Equal(t, before, tableDigest(t, d.Pool, other.Company.ID))
	f, err := d.Storage.Open(other.Company.ID, "iso50001", saved.Path)
	require.NoError(t, err, "another company's file stays on disk")
	require.NoError(t, f.Close())
}

// R553: a reset already running makes the next one a no-op, and so does a
// database without a demo company.
func TestResetSkipsWhenLocked(t *testing.T) {
	t.Parallel()
	d, ctx := demoDeps(t), t.Context()
	locker := lock.NewMemory(time.Now)
	res, err := ResetDemo(ctx, d, locker, demoNow)
	require.NoError(t, err)
	require.Equal(t, ResetResult{}, res, "no demo company, nothing to do")

	require.NoError(t, ensureDemoSites(ctx, d, DemoBaseline(), demoNow))
	_, err = d.Pool.Exec(ctx, `insert into alarms (company_id, name, type) values ($1, 'ziyaretçi', 'data_communication')`, DemoCompanyID)
	require.NoError(t, err)
	lease, err := locker.Acquire(ctx, DemoResetLock, time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lease.Release(context.Background()) })

	res, err = ResetDemo(ctx, d, locker, demoNow)
	require.NoError(t, err)
	require.Equal(t, ResetResult{Found: true, Skipped: true}, res)
	require.Equal(t, 1, countRows(t, d.Pool, `select count(*) from alarms where company_id = $1`, DemoCompanyID))
}
