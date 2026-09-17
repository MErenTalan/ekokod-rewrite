package testfixtures

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Tenant is a complete, coherent company: users of every role, two buildings,
// analyzers under each, plants, tariffs, and the two scopes a test needs to
// tell a narrow read from a company-wide one.
//
// TWO BUILDINGS AND A SCOPE COVERING ONLY THE FIRST IS THE POINT. With one
// building, a repository that ignored its Scope entirely would return the same
// rows as one that honoured it, and Task 13's scope-isolation test would pass
// against a completely unscoped implementation. Buildings[1] exists so that
// "the scope leaked" has an observable consequence. Nothing here may be
// reduced to a single building for tidiness.
type Tenant struct {
	Company model.Company

	// Users holds exactly one user per model.UserRole, keyed by role, so a
	// permission test can name the role it wants rather than index into a
	// slice and hope.
	Users map[model.UserRole]model.User

	// Buildings has at least two. Scope covers Buildings[0] ONLY.
	Buildings []model.Building

	// Analyzers has at least two per building, in building order, so that
	// Analyzers[0] and Analyzers[1] are under Buildings[0].
	Analyzers []model.Analyzer

	Plants  []model.PowerPlant
	Tariffs []model.Tariff

	// Scope grants Buildings[0] and nothing else.
	Scope store.Scope

	// AdminScope grants the whole company via AllBuildings. It is the scope a
	// cross-tenant test uses to prove that even the widest legitimate scope
	// cannot reach another company.
	AdminScope store.Scope
}

// fixtureNamespace is the UUID namespace every fixture id is derived from. A
// fixed namespace plus a (seed, label) name makes every id a pure function of
// the seed: a failing test prints an id, and re-running with the same seed
// produces that same id, which is what makes a failure reproducible instead of
// merely repeatable.
var fixtureNamespace = uuid.MustParse("6f9619ff-8b86-d011-b42d-00c04fc964ff")

// fixtureEpoch is the created_at/updated_at every fixture row gets. It is
// fixed rather than time.Now() so that a struct built by NewTenant can be
// compared with the row a repository reads back.
//
// What "compares equal" means, precisely:
//
//   - Timestamps are equal as INSTANTS, under time.Time.Equal — not under ==
//     or require.Equal. pgx decodes timestamptz into time.Local, so the value
//     read back carries a different *time.Location than this UTC epoch
//     whenever the test process's local zone is not UTC, and a reflect-based
//     comparison of the two structs fails although nothing is wrong.
//   - Decimals are equal in value AND in scale: every decimal literal below is
//     written at exactly its column's declared scale, so the column stores it
//     unrounded (TestTenantRowsAreActuallyInTheDatabase reads each one back as
//     text). Whether decimal.Decimal values also compare equal under
//     reflection depends on the exponent the read path produces; use
//     Decimal.Equal.
var fixtureEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// NewTenant inserts a complete tenant and returns it.
//
// It is DETERMINISTIC in seed: identical seeds produce identical ids, names,
// emails and values. Two tenants with DIFFERENT seeds coexist in one database
// without colliding on any of the schema's unique indexes —
// companies(lower(name)), users(email), analyzers(provider, provider_subtype,
// installation_number) and power_plants(isolar_ps_id) — because every one of
// those values carries the seed.
//
// Calling it TWICE WITH THE SAME SEED against the same database fails, on a
// primary key conflict. That is deliberate: the same seed names the same
// tenant, and silently producing a second, different one would break the
// reproducibility the seed exists for.
//
// The pool must already be migrated; use NewMigratedPool.
func NewTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, seed int64) Tenant {
	t.Helper()

	id := func(label string) uuid.UUID {
		return uuid.NewSHA1(fixtureNamespace, fmt.Appendf(nil, "%d/%s", seed, label))
	}

	tenant := Tenant{Users: make(map[model.UserRole]model.User, len(model.UserRoles()))}

	// --- company -----------------------------------------------------------
	tenant.Company = model.Company{
		ID:             id("company"),
		Name:           fmt.Sprintf("Fixture Tenant %d", seed),
		Address:        ptr(fmt.Sprintf("%d Fixture Street, Istanbul", seed)),
		TotalAreaM2:    decPtr("12345.67"),
		PersonnelCount: ptr(int32(42)),
		ContactName:    ptr("Fixture Contact"),
		ContactPhone:   ptr("+90 212 000 0000"),
		Sector:         ptr("office"),
		CreatedAt:      fixtureEpoch,
		UpdatedAt:      fixtureEpoch,
	}
	exec(t, ctx, pool, `
		insert into companies
			(id, name, address, total_area_m2, personnel_count, contact_name,
			 contact_phone, sector, created_at, updated_at)
		values ($1, $2, $3, $4::text::numeric, $5, $6, $7, $8, $9, $9)`,
		tenant.Company.ID, tenant.Company.Name, tenant.Company.Address,
		tenant.Company.TotalAreaM2.String(), tenant.Company.PersonnelCount,
		tenant.Company.ContactName, tenant.Company.ContactPhone,
		tenant.Company.Sector, fixtureEpoch)

	// --- users, one per role ----------------------------------------------
	for i, role := range model.UserRoles() {
		u := model.User{
			ID:        id("user/" + string(role)),
			CompanyID: tenant.Company.ID,
			Name:      fmt.Sprintf("Fixture %s %d", role, seed),
			// The email carries the seed, so tenants never collide on the
			// users(email) unique index. example.invalid is reserved by
			// RFC 2606 and can never resolve, so a test that accidentally
			// sends mail fails loudly instead of reaching a real inbox.
			Email:        fmt.Sprintf("%s@tenant-%d.example.invalid", role, seed),
			Phone:        ptr(fmt.Sprintf("+90 555 %04d %03d", seed, i)),
			PasswordHash: "$2a$10$fixturehashfixturehashfixturehashfixturehashfixtureha",
			Role:         role,
			IsActive:     true,
			Locale:       "tr",
			CreatedAt:    fixtureEpoch,
			UpdatedAt:    fixtureEpoch,
		}
		exec(t, ctx, pool, `
			insert into users
				(id, company_id, name, email, phone, password_hash, role,
				 is_active, created_at, updated_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
			u.ID, u.CompanyID, u.Name, u.Email, u.Phone, u.PasswordHash,
			string(u.Role), u.IsActive, fixtureEpoch)
		tenant.Users[role] = u
	}

	responsible := tenant.Users[model.UserRoleCompanyAdmin].ID

	// --- two buildings -----------------------------------------------------
	for i := range 2 {
		b := model.Building{
			ID:                id(fmt.Sprintf("building/%d", i)),
			CompanyID:         tenant.Company.ID,
			Name:              fmt.Sprintf("Fixture Building %c (tenant %d)", 'A'+rune(i), seed),
			Address:           ptr(fmt.Sprintf("Building %d, Fixture Street", i)),
			Latitude:          decPtr(fmt.Sprintf("41.%06d", i)),
			Longitude:         decPtr(fmt.Sprintf("29.%06d", i)),
			Floors:            ptr(int32(3 + i)),
			PersonnelCount:    ptr(int32(10 + i)),
			TotalAreaM2:       decPtr(fmt.Sprintf("%d.50", 1000+i)),
			Sector:            ptr("office"),
			ResponsibleUserID: &responsible,
			// A cutoff day other than 1 so that a bill period is provably
			// not just "the calendar month".
			BillCutoffDay: int16(15),
			CreatedAt:     fixtureEpoch,
			UpdatedAt:     fixtureEpoch,
		}
		exec(t, ctx, pool, `
			insert into buildings
				(id, company_id, name, address, latitude, longitude, floors,
				 personnel_count, total_area_m2, sector, responsible_user_id,
				 bill_cutoff_day, created_at, updated_at)
			values ($1, $2, $3, $4, $5::text::numeric, $6::text::numeric, $7, $8,
			        $9::text::numeric, $10, $11, $12, $13, $13)`,
			b.ID, b.CompanyID, b.Name, b.Address, b.Latitude.String(),
			b.Longitude.String(), b.Floors, b.PersonnelCount,
			b.TotalAreaM2.String(), b.Sector, b.ResponsibleUserID,
			b.BillCutoffDay, fixtureEpoch)
		tenant.Buildings = append(tenant.Buildings, b)
	}

	// --- two analyzers per building ---------------------------------------
	providers := []model.IntegrationProvider{
		model.IntegrationProviderOSOS,
		model.IntegrationProviderGridbox,
	}
	for bi, b := range tenant.Buildings {
		for ai := range 2 {
			buildingID := b.ID
			a := model.Analyzer{
				ID:              id(fmt.Sprintf("analyzer/%d/%d", bi, ai)),
				CompanyID:       tenant.Company.ID,
				BuildingID:      &buildingID,
				Provider:        providers[ai],
				ProviderSubtype: "Baskent",
				// The seed is in the installation number, so two tenants
				// never collide on
				// analyzers(provider, provider_subtype, installation_number).
				InstallationNumber: fmt.Sprintf("T%d-B%d-A%d", seed, bi, ai),
				CustomerName:       ptr(tenant.Company.Name),
				Province:           ptr("Istanbul"),
				District:           ptr("Kadikoy"),
				InstalledPowerKw:   decPtr("250.500"),
				MeterNumber:        ptr(fmt.Sprintf("METER-%d-%d-%d", seed, bi, ai)),
				// A multiplier other than 1, so that a repository which
				// forgets to apply or record it is observably wrong.
				MeterMultiplier: dec("40.000000"),
				Latitude:        decPtr(fmt.Sprintf("41.%06d", bi*10+ai)),
				Longitude:       decPtr(fmt.Sprintf("29.%06d", bi*10+ai)),
				EtsoCode:        ptr(fmt.Sprintf("40X%013d", seed*100+int64(bi*10+ai))),
				IsActive:        true,
				CreatedAt:       fixtureEpoch,
				UpdatedAt:       fixtureEpoch,
			}
			exec(t, ctx, pool, `
				insert into analyzers
					(id, company_id, building_id, provider, provider_subtype,
					 installation_number, customer_name, province, district,
					 installed_power_kw, meter_number, meter_multiplier,
					 latitude, longitude, etso_code, is_active,
					 created_at, updated_at)
				values ($1, $2, $3, $4, $5, $6, $7, $8, $9,
				        $10::text::numeric, $11, $12::text::numeric,
				        $13::text::numeric, $14::text::numeric, $15, $16, $17, $17)`,
				a.ID, a.CompanyID, a.BuildingID, string(a.Provider), a.ProviderSubtype,
				a.InstallationNumber, a.CustomerName, a.Province, a.District,
				a.InstalledPowerKw.String(), a.MeterNumber, a.MeterMultiplier.String(),
				a.Latitude.String(), a.Longitude.String(), a.EtsoCode, a.IsActive,
				fixtureEpoch)
			tenant.Analyzers = append(tenant.Analyzers, a)
		}
	}

	// --- plants ------------------------------------------------------------
	// power_plants has no building_id, so these belong to the company and are
	// deliberately NOT narrowable by Scope.BuildingIDs. One rooftop and one
	// grid plant, so a PlantFilter on kind has something to distinguish.
	for pi, kind := range []string{"rooftop", "grid"} {
		p := model.PowerPlant{
			ID:                 id(fmt.Sprintf("plant/%d", pi)),
			CompanyID:          tenant.Company.ID,
			Name:               fmt.Sprintf("Fixture %s Plant %d", kind, seed),
			InstallationNumber: ptr(fmt.Sprintf("PP-%d-%d", seed, pi)),
			PlantKind:          kind,
			PanelPowerW:        decPtr("550.00"),
			PanelEfficiencyPct: decPtr("21.500"),
			PanelCount:         ptr(int32(400)),
			StringCount:        ptr(int32(8)),
			Orientation:        orientation(model.PanelOrientationS),
			TiltAngleDeg:       decPtr("30.00"),
			TotalCapacityKw:    decPtr("220.000"),
			YearlyTargetKwh:    decPtr("330000.000"),
			// Unique where not null, so the seed goes in.
			IsolarPsID:        ptr(fmt.Sprintf("isolar-%d-%d", seed, pi)),
			IsolarInstalledKw: decPtr("220.000"),
			CreatedAt:         fixtureEpoch,
			UpdatedAt:         fixtureEpoch,
		}
		exec(t, ctx, pool, `
			insert into power_plants
				(id, company_id, name, installation_number, plant_kind,
				 panel_power_w, panel_efficiency_pct, panel_count, string_count,
				 orientation, tilt_angle_deg, total_capacity_kw, yearly_target_kwh,
				 isolar_ps_id, isolar_installed_kw, created_at, updated_at)
			values ($1, $2, $3, $4, $5, $6::text::numeric, $7::text::numeric, $8, $9,
			        $10, $11::text::numeric, $12::text::numeric, $13::text::numeric,
			        $14, $15::text::numeric, $16, $16)`,
			p.ID, p.CompanyID, p.Name, p.InstallationNumber, p.PlantKind,
			p.PanelPowerW.String(), p.PanelEfficiencyPct.String(), p.PanelCount,
			p.StringCount, string(*p.Orientation), p.TiltAngleDeg.String(),
			p.TotalCapacityKw.String(), p.YearlyTargetKwh.String(),
			p.IsolarPsID, p.IsolarInstalledKw.String(), fixtureEpoch)
		tenant.Plants = append(tenant.Plants, p)
	}

	// --- one tariff per building ------------------------------------------
	createdBy := tenant.Users[model.UserRoleCompanyAdmin].ID
	for bi, b := range tenant.Buildings {
		buildingID := b.ID
		tar := model.Tariff{
			ID:            id(fmt.Sprintf("tariff/%d", bi)),
			CompanyID:     tenant.Company.ID,
			BuildingID:    &buildingID,
			Name:          ptr(fmt.Sprintf("Fixture Tariff %d/%d", seed, bi)),
			EffectiveFrom: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			Currency:      model.CurrencyTRY,
			EnergyType:    model.EnergyTypeGrid,
			VoltageLevel:  model.VoltageLevelMV,
			UserGroup:     model.UserGroupCommercial,
			// single_time, so the schema's single_time_needs_price check is
			// satisfied by SingleTimePrice alone. The T1/T2/T3 prices are set
			// anyway so a multi-time test can flip PriceType without
			// rebuilding the row.
			PriceType:                   model.PriceTypeSingleTime,
			Term:                        model.TariffTermBinomial,
			SupplyCompany:               model.SupplyCompanyIncumbent,
			SingleTimePrice:             decPtr("2.451234"),
			T1Price:                     decPtr("2.100001"),
			T2Price:                     decPtr("1.050002"),
			T3Price:                     decPtr("3.900003"),
			DistributionCost:            dec("0.987654"),
			ReactivePowerPrice:          dec("1.234567"),
			ContractedPowerKw:           decPtr("500.000"),
			PowerUnitPrice:              decPtr("12.345678"),
			GenerationUsage:             model.GenerationUsageNone,
			VatRate:                     dec("20.000"),
			UsePtfYekdem:                false,
			UseManualYekdem:             false,
			KbkDistributionCostTlPerKwh: nil,
			CreatedBy:                   &createdBy,
			CreatedAt:                   fixtureEpoch,
			UpdatedAt:                   fixtureEpoch,
		}
		exec(t, ctx, pool, `
			insert into tariffs
				(id, company_id, building_id, name, effective_from, currency,
				 energy_type, voltage_level, user_group, price_type, term,
				 supply_company, single_time_price, t1_price, t2_price, t3_price,
				 distribution_cost, reactive_power_price, contracted_power_kw,
				 power_unit_price, generation_usage, vat_rate, created_by,
				 created_at, updated_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			        $13::text::numeric, $14::text::numeric, $15::text::numeric,
			        $16::text::numeric, $17::text::numeric, $18::text::numeric,
			        $19::text::numeric, $20::text::numeric, $21,
			        $22::text::numeric, $23, $24, $24)`,
			tar.ID, tar.CompanyID, tar.BuildingID, tar.Name, tar.EffectiveFrom,
			string(tar.Currency), string(tar.EnergyType), string(tar.VoltageLevel),
			string(tar.UserGroup), string(tar.PriceType), string(tar.Term),
			string(tar.SupplyCompany), tar.SingleTimePrice.String(),
			tar.T1Price.String(), tar.T2Price.String(), tar.T3Price.String(),
			tar.DistributionCost.String(), tar.ReactivePowerPrice.String(),
			tar.ContractedPowerKw.String(), tar.PowerUnitPrice.String(),
			string(tar.GenerationUsage), tar.VatRate.String(), tar.CreatedBy,
			fixtureEpoch)
		tenant.Tariffs = append(tenant.Tariffs, tar)
	}

	// --- the two scopes ----------------------------------------------------
	tenant.Scope = store.Scope{
		CompanyID:   tenant.Company.ID,
		BuildingIDs: []uuid.UUID{tenant.Buildings[0].ID},
	}
	tenant.AdminScope = store.Scope{
		CompanyID:    tenant.Company.ID,
		AllBuildings: true,
	}

	// Guard the invariants the fixture promises, so that a future edit that
	// quietly drops the second building fails HERE with an explanation rather
	// than in Task 13 as an unexplained pass.
	require.GreaterOrEqual(t, len(tenant.Buildings), 2,
		"a tenant needs at least two buildings or scope isolation is untestable")
	require.GreaterOrEqual(t, len(tenant.Analyzers), 2*len(tenant.Buildings),
		"a tenant needs at least two analyzers per building")
	require.Len(t, tenant.Users, len(model.UserRoles()),
		"a tenant needs one user of every role")
	require.True(t, tenant.Scope.Valid())
	require.True(t, tenant.AdminScope.Valid())
	require.True(t, tenant.Scope.AllowsBuilding(tenant.Buildings[0].ID))
	require.False(t, tenant.Scope.AllowsBuilding(tenant.Buildings[1].ID),
		"Scope must cover Buildings[0] ONLY: it is what lets a leak be observed")

	return tenant
}

// exec runs one fixture insert and fails the test with the SQL on error.
func exec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	_, err := pool.Exec(ctx, sql, args...)
	require.NoErrorf(t, err, "fixture insert failed:%s", sql)
}

func ptr[T any](v T) *T { return &v }

func orientation(o model.PanelOrientation) *model.PanelOrientation { return &o }

// dec parses a fixture literal into a decimal.Decimal, panicking on a typo.
// Fixture values are constants in this file, so a parse failure is a bug in
// the fixture and not a runtime condition worth threading an error for.
func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func decPtr(s string) *decimal.Decimal {
	d := dec(s)
	return &d
}
