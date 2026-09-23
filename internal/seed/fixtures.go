package seed

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// e2eNamespace derives every fixture id, so ids are stable across runs (R183).
var e2eNamespace = uuid.MustParse("3d0b6a1e-5b8f-4b7a-9d57-2e0c6f1a7c11")

func e2eID(name string) uuid.UUID { return uuid.NewSHA1(e2eNamespace, []byte(name)) }

// Fixture users' e-mails.
const (
	E2EAdminEmail            = "admin@e2e.ekokod.test"
	E2ECompanyAdminEmail     = "ca@a.e2e.ekokod.test"
	E2ECompanyReadonlyEmail  = "cr@a.e2e.ekokod.test"
	E2EBuildingAdminEmail    = "ba@a.e2e.ekokod.test"
	E2EBuildingReadonlyEmail = "br@a.e2e.ekokod.test"
	E2ECompanyBAdminEmail    = "ca@b.e2e.ekokod.test"
)

// Fixtures names what E2EFixtures created.
type Fixtures struct {
	PlatformCompany, CompanyA, CompanyB uuid.UUID
	// Buildings: A1 (responsible BA), A2 (responsible BR), B1.
	BuildingA1, BuildingA2, BuildingB1 uuid.UUID
	AnalyzerA1, AnalyzerA2, AnalyzerB1 uuid.UUID
	Users                              map[string]uuid.UUID // by e-mail
}

// E2EFixtures idempotently creates the platform company and admin plus two
// tenants with one user per non-demo role (R183). Refuse it in production.
func E2EFixtures(ctx context.Context, pool *pgxpool.Pool, hasher auth.Hasher, password string, now time.Time) (Fixtures, error) {
	f := Fixtures{
		PlatformCompany: e2eID("company/platform"), CompanyA: e2eID("company/a"), CompanyB: e2eID("company/b"),
		BuildingA1: e2eID("building/a1"), BuildingA2: e2eID("building/a2"), BuildingB1: e2eID("building/b1"),
		AnalyzerA1: e2eID("analyzer/a1"), AnalyzerA2: e2eID("analyzer/a2"), AnalyzerB1: e2eID("analyzer/b1"),
		Users: map[string]uuid.UUID{},
	}
	companies := []struct {
		id     uuid.UUID
		name   string
		sector string
	}{
		{f.PlatformCompany, "Ekokod Platform", "Hizmet"},
		{f.CompanyA, "E2E Şirket A", "Üretim"},
		{f.CompanyB, "E2E Şirket B", "Üretim"},
	}
	for _, c := range companies {
		if err := ensureCompany(ctx, pool, c.id, c.name, c.sector, now); err != nil {
			return f, err
		}
	}
	users := []struct {
		email, name string
		role        model.UserRole
		company     uuid.UUID
	}{
		{E2EAdminEmail, "Platform Yöneticisi", model.UserRoleAdmin, f.PlatformCompany},
		{E2ECompanyAdminEmail, "Ayşe Kaya", model.UserRoleCompanyAdmin, f.CompanyA},
		{E2ECompanyReadonlyEmail, "Can Demir", model.UserRoleCompanyReadonlyAdmin, f.CompanyA},
		{E2EBuildingAdminEmail, "Burak Şahin", model.UserRoleBuildingAdmin, f.CompanyA},
		{E2EBuildingReadonlyEmail, "Elif Arslan", model.UserRoleBuildingReadonlyAdmin, f.CompanyA},
		{E2ECompanyBAdminEmail, "Deniz Yıldız", model.UserRoleCompanyAdmin, f.CompanyB},
	}
	userRepo := postgres.NewUserRepository(pool)
	for _, u := range users {
		id := e2eID("user/" + u.email)
		f.Users[u.email] = id
		_, err := userRepo.Get(ctx, store.SystemScope(u.company), id)
		if err == nil {
			continue
		}
		if !errors.Is(err, store.ErrNotFound) {
			return f, err
		}
		if _, err := CreateUser(ctx, pool, hasher, UserInput{
			ID: id, Email: u.email, Name: u.name, Role: u.role, CompanyID: u.company, Password: password,
		}, now); err != nil {
			return f, err
		}
	}
	buildings := []struct {
		id, company, responsible uuid.UUID
		name                     string
		lat, lon                 string
	}{
		{f.BuildingA1, f.CompanyA, f.Users[E2EBuildingAdminEmail], "A1 Fabrika", "39.925533", "32.866287"},
		{f.BuildingA2, f.CompanyA, f.Users[E2EBuildingReadonlyEmail], "A2 Depo", "41.008238", "28.978359"},
		{f.BuildingB1, f.CompanyB, f.Users[E2ECompanyBAdminEmail], "B1 Ofis", "38.423734", "27.142826"},
	}
	buildingRepo := postgres.NewBuildingRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)
	for i, b := range buildings {
		sc := store.SystemScope(b.company)
		if _, err := buildingRepo.Get(ctx, sc, b.id); errors.Is(err, store.ErrNotFound) {
			lat, lon := decimal.RequireFromString(b.lat), decimal.RequireFromString(b.lon)
			area, personnel, sector := decimal.RequireFromString("5000.00"), int32(40+10*i), "Üretim"
			responsible := b.responsible
			if _, err := buildingRepo.Create(ctx, sc, model.Building{
				ID: b.id, CompanyID: b.company, Name: b.name, Latitude: &lat, Longitude: &lon, Sector: &sector,
				TotalAreaM2: &area, PersonnelCount: &personnel, ResponsibleUserID: &responsible, BillCutoffDay: 1,
				CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return f, err
			}
		} else if err != nil {
			return f, err
		}
		analyzerID := []uuid.UUID{f.AnalyzerA1, f.AnalyzerA2, f.AnalyzerB1}[i]
		if _, err := analyzerRepo.Get(ctx, sc, analyzerID); errors.Is(err, store.ErrNotFound) {
			buildingID := b.id
			power := decimal.RequireFromString("120.00")
			if _, err := analyzerRepo.Create(ctx, sc, model.Analyzer{
				ID: analyzerID, CompanyID: b.company, BuildingID: &buildingID, Provider: model.IntegrationProviderOSOS,
				ProviderSubtype: "Baskent", InstallationNumber: "E2E-" + b.name[:2], InstalledPowerKw: &power,
				MeterMultiplier: decimal.NewFromInt(1), IsActive: true, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return f, err
			}
		} else if err != nil {
			return f, err
		}
	}

	return f, nil
}

// e2eReadingHistory is how far back E2EData's readings go: enough for a
// "last 6 months" filter, the 7-day activity rule (R163) and a full previous
// month for the sectoral comparison (R162).
const e2eReadingHistory = 75 * 24 * time.Hour

// E2EData fills the fixture analyzers with hourly readings and gives A1 a bill
// for the previous month (R194), so the screens have something real to show.
// It is separate from E2EFixtures because the Go HTTP tests seed the readings
// their own assertions need; only `ekokod seed e2e` (the Playwright stack)
// calls this.
func E2EData(ctx context.Context, pool *pgxpool.Pool, f Fixtures, now time.Time) (int, error) {
	total := 0
	for _, tenant := range []struct {
		company   uuid.UUID
		analyzers []uuid.UUID
		profiles  []int
	}{
		{f.CompanyA, []uuid.UUID{f.AnalyzerA1, f.AnalyzerA2}, []int{0, 1}},
		{f.CompanyB, []uuid.UUID{f.AnalyzerB1}, []int{2}},
	} {
		n, err := fillHourly(ctx, pool, store.SystemScope(tenant.company), tenant.analyzers, tenant.profiles,
			e2eReadingHistory, now)
		total += n
		if err != nil {
			return total, err
		}
	}
	if err := ensureBill(ctx, pool, store.SystemScope(f.CompanyA), f.BuildingA1, now); err != nil {
		return total, err
	}
	if err := ensureBillsAndTariffs(ctx, pool, f, now); err != nil {
		return total, err
	}
	return total, ensureAlarms(ctx, pool, f, now)
}

func ensureCompany(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, name, sector string, now time.Time) error {
	repo := postgres.NewCompanyRepository(pool)
	sc := store.SystemScope(id)
	if _, err := repo.Get(ctx, sc, id); !errors.Is(err, store.ErrNotFound) {
		return err
	}
	_, err := repo.Create(ctx, sc, model.Company{ID: id, Name: name, Sector: &sector, CreatedAt: now, UpdatedAt: now})
	return err
}
