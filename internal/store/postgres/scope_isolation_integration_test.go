//go:build integration

// TestScopeIsolation is Task 13b of the F1 data-model phase: the single most
// important test in the phase (wave-g-task-notes.md §"Task 13 — performance
// and acceptance suite"). It is named literally in the F1 acceptance block:
//
//	go test ./internal/store/postgres -tags=integration -run TestScopeIsolation -v
//
// It walks EVERY scoped repository constructor in package postgres and
// proves that no READ method ever returns another company's rows.
//
// Two proofs, per the controller's binding ruling ("Scope choice is
// load-bearing", wave-g-task-notes.md and HANDOFF_NEXT_SESSION.md rule 4):
//
//   - CROSS-TENANT: every read is called with tenant A's ADMIN scope
//     (AllBuildings: true) against tenant B's ids. Under AllBuildings there is
//     no building-id-list branch to (wrongly) mask a broken company_id
//     predicate, so company_id is the ONLY thing that can exclude tenant B's
//     rows. Task 10's and Carbon's re-reviews each found a
//     `company_id = $n or true` mutation that stayed green under a narrow
//     Scope precisely because the building branch shadowed it.
//   - NARROW-SCOPE: for every repository whose table (or whose query's
//     BuildingFilter branch) actually narrows by building — established by
//     reading each repository's own isolation doc comment in
//     internal/store/repository.go and confirmed against the query source —
//     tenant A's own narrow Scope (which grants Buildings[0] only) is called
//     against tenant A's OWN data on Buildings[1]. This is what tells "the
//     scope leaked" from "the whole company Scope legitimately sees this
//     company-only row" (repositories such as Plant, Production, Alarm,
//     File, Integration, SMTP, Calendar, Ops, TariffTemplate, SolarTariff and
//     Icmal carry no building_id anywhere in their query — a Scope narrows
//     them to the company and no further, exactly as their own doc comments
//     in repository.go say, so a narrow-scope subtest for them would test
//     nothing real and is not written).
//   - PLATFORM-WIDE tables (market_prices_hourly, yekdem_monthly,
//     national_tariff_schedule, and IntegrationRepository.Definitions/
//     Definition) have no company_id at all: repository.go's own ruling 9
//     (global-constraints.md) says their "isolation" is invalid-scope
//     rejection, since the Scope narrows nothing. Those subtests assert
//     ErrInvalidScope for a zero Scope and, for completeness, that the SAME
//     platform row is visible under either tenant's Scope.
//
// TENANT B MUST HAVE REAL DATA in every table a read touches, including
// child tables reached only through a parent — otherwise a broken
// `… or true` predicate stays green because the query has nothing to leak
// (wave-f-context.md's "guards must be proven to fail"). seedScopeIsoFixtures
// below seeds one representative row per table for both directions.
//
// OMISSION DETECTION: TestScopeIsolationCoversEveryRepository below performs
// an AST scan of every exported `New...Repository` constructor declared in
// package postgres (this directory's non-test .go files) and fails if one is
// not registered in scopeIsoRegisteredRepositories — so a repository added by
// a later phase that forgets to register here is a RED test, not a silent
// gap. The scan does not descend into subdirectories, so
// internal/store/postgres/admin is excluded from the scan by construction.
//
// ADMIN PACKAGE EXCLUDED BY DESIGN. internal/store/postgres/admin is the
// single unscoped surface repository.go's header names (03-target-
// architecture.md §2.5): none of its methods take a Scope, so "does it leak
// another tenant's rows under a Scope" does not apply to it. This file uses
// two admin repositories (admin.NewMarketDataRepository,
// and raw SQL inserts matching other integration tests' own pattern) purely
// as SETUP for the platform-wide-table subtests, never as a test subject.
package postgres_test

import (
	"context"
	"crypto/rand"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// ---------------------------------------------------------------------------
// Omission detection
// ---------------------------------------------------------------------------

// scopeIsoRegisteredRepositories is every exported New...Repository
// constructor TestScopeIsolation below exercises. Kept as its own map (not
// inferred from the AST scan) so that TestScopeIsolationCoversEveryRepository
// can compare the two independently: a constructor the scan finds but this
// map lacks is an OMISSION (new repository, forgotten registration); a name
// this map has but the scan does not find is STALE (renamed or removed
// repository, forgotten cleanup). Both directions are asserted.
var scopeIsoRegisteredRepositories = map[string]bool{
	"NewCompanyRepository":        true,
	"NewUserRepository":           true,
	"NewSessionRepository":        true,
	"NewAuditRepository":          true,
	"NewBuildingRepository":       true,
	"NewAnalyzerRepository":       true,
	"NewPlantRepository":          true,
	"NewReadingRepository":        true,
	"NewCursorRepository":         true,
	"NewAnomalyRepository":        true,
	"NewProductionRepository":     true,
	"NewAnalyticsRepository":      true,
	"NewPriceRepository":          true,
	"NewForecastRepository":       true,
	"NewTariffRepository":         true,
	"NewTariffTemplateRepository": true,
	"NewSolarTariffRepository":    true,
	"NewNationalTariffRepository": true,
	"NewIcmalRepository":          true,
	"NewBillRepository":           true,
	"NewReportRepository":         true,
	"NewAlarmRepository":          true,
	"NewCarbonRepository":         true,
	"NewISO50001Repository":       true,
	"NewFileRepository":           true,
	"NewIntegrationRepository":    true,
	"NewSMTPRepository":           true,
	"NewCalendarRepository":       true,
	"NewOpsRepository":            true,
}

// scopeIsoCtorPattern matches an exported repository constructor: New,
// then anything, then Repository. It intentionally does NOT match NewPool or
// New (db.go's DB constructor), which is the point — this file's table is
// about REPOSITORIES, not every exported New… in the package.
var scopeIsoCtorPattern = regexp.MustCompile(`^New[A-Za-z0-9]*Repository$`)

// TestScopeIsolationCoversEveryRepository is the omission guard: it parses
// every non-test .go file in this directory (package postgres) and fails if
// an exported New...Repository constructor is not registered in
// scopeIsoRegisteredRepositories above, or if a registered name no longer
// exists (a stale entry after a rename).
//
// Deliberate-break proof (see task-13b-report.md for the observed output):
// commenting out one entry from scopeIsoRegisteredRepositories makes this
// test FAIL, naming the omitted constructor.
func TestScopeIsolationCoversEveryRepository(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller must resolve this file's own path")
	dir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	// parser.ParseFile per non-test .go file, rather than the deprecated
	// parser.ParseDir (it ignores build tags when grouping files into
	// packages, which parsing one file at a time sidesteps entirely — this
	// scan never needs cross-file package grouping, only each file's own
	// top-level declarations).
	fset := token.NewFileSet()
	sawPackagePostgres := false
	found := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || isTestGoFile(name) {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		require.NoError(t, err, "parsing %s", name)
		require.Equal(t, "postgres", file.Name.Name, "%s is not in package postgres — the scan directory is wrong", name)
		sawPackagePostgres = true
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue // methods are not constructors
			}
			if fn.Name.IsExported() && scopeIsoCtorPattern.MatchString(fn.Name.Name) {
				found[fn.Name.Name] = true
			}
		}
	}
	require.True(t, sawPackagePostgres, "no non-test .go file was found in %s — the scan directory is wrong", dir)
	// Guard the guard: if the scan finds nothing at all, every assertion
	// below would pass vacuously (both "missing" and "stale" would be
	// computed from an empty set on one side).
	require.NotEmpty(t, found, "the AST scan found zero New...Repository constructors — the scan itself is broken")

	var missing []string
	for name := range found {
		if !scopeIsoRegisteredRepositories[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	require.Empty(t, missing,
		"constructor(s) exist in package postgres but are not registered in scopeIsoRegisteredRepositories — "+
			"a repository added later must be added to TestScopeIsolation's table: %v", missing)

	var stale []string
	for name := range scopeIsoRegisteredRepositories {
		if !found[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	require.Empty(t, stale,
		"scopeIsoRegisteredRepositories names a constructor that no longer exists in package postgres: %v", stale)
}

func isTestGoFile(name string) bool {
	return len(name) > len("_test.go") && name[len(name)-len("_test.go"):] == "_test.go"
}

// ---------------------------------------------------------------------------
// Shared fixtures
// ---------------------------------------------------------------------------

var scopeIsoEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func scopeIsoDec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func scopeIsoContainsID[T any](items []T, id uuid.UUID, get func(T) uuid.UUID) bool {
	for _, it := range items {
		if get(it) == id {
			return true
		}
	}
	return false
}

// scopeIsoFixtures is one tenant's worth of data beyond what
// testfixtures.NewTenant already seeds, covering every table a scoped
// repository's read methods touch — including child tables reachable only
// through a parent. It is built at ONE building index (0 or 1) of the given
// tenant:
//
//   - seeded at building index 0 for the tenant used as the CROSS-TENANT
//     victim (tenant B): any building works, because the cross-tenant
//     subtests use tenant A's AdminScope, which narrows nothing by building.
//   - seeded at building index 1 for the tenant used as the NARROW-SCOPE
//     victim (tenant A itself): Buildings[1] is, by testfixtures.NewTenant's
//     own contract, OUTSIDE tenant.Scope's grant (which covers Buildings[0]
//     only), so this is real "own company, wrong building" data.
type scopeIsoFixtures struct {
	tenant      testfixtures.Tenant
	buildingIdx int
	buildingID  uuid.UUID
	analyzerID  uuid.UUID // tenant.Analyzers[buildingIdx*2]
	plantID     uuid.UUID // tenant.Plants[0] — plants carry no building_id
	tariffID    uuid.UUID // tenant.Tariffs[buildingIdx]

	sessionID uuid.UUID

	cursorKind model.ReadingKind

	anomalyID uuid.UUID

	readingsFrom, readingsTo time.Time

	forecastGeneratedAt time.Time

	billID uuid.UUID

	reportID uuid.UUID

	isoProjectID uuid.UUID
	isoNoteID    uuid.UUID

	carbonFactorID   uuid.UUID
	carbonActivityID uuid.UUID
	carbonReportID   uuid.UUID

	alarmID      uuid.UUID
	alarmEventID uuid.UUID

	fileID uuid.UUID

	integrationDefinitionID uuid.UUID
	credentialID            uuid.UUID

	calendarEventID    uuid.UUID
	calendarVacationID uuid.UUID

	jobRunID     uuid.UUID
	opsMessageID int64

	tariffTemplateID uuid.UUID
	solarTariffID    uuid.UUID

	icmalImportID uuid.UUID

	deviceID uuid.UUID
}

// scopeIsoInsertIntegrationDefinition inserts one platform-wide
// integration_definitions row (no company_id at all) with a subtype unique
// to this call, and returns its id. seedScopeIsoFixtures gives each tenant
// its OWN definition (never a shared one) specifically so that
// IntegrationRepository.Credential — keyed by definitionID, not by a
// per-credential surrogate id — has something real to prove isolation
// against: if tenant A's Scope were handed tenant B's OWN definitionID and
// still found a row, that row could only be tenant B's, because tenant A
// never created a credential against it.
func scopeIsoInsertIntegrationDefinition(t *testing.T, ctx context.Context, pool *pgxpool.Pool, subtype string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_definitions (provider, subtype) values ('osos', $1) returning id`, subtype,
	).Scan(&id))
	return id
}

// scopeIsoCipher builds a fresh AES-256-GCM cipher for the integration and
// SMTP credential fixtures, exactly as integrationCipher (integrations_
// integration_test.go) does — duplicated under this file's own prefix per
// wave-f-context.md rule 6 (package-wide helper names must carry the owning
// file's aggregate prefix, and "scopeIso" is this file's).
func scopeIsoCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	c, err := crypto.NewCipher(key)
	require.NoError(t, err)
	return c
}

// seedScopeIsoFixtures inserts one representative row per table into every
// scoped repository's territory, for tenant at buildingIdx (0 or 1). It uses
// each domain's OWN repository Create/Upsert methods wherever one exists
// (the same repositories under test — Task 9-11's own review already proved
// these write paths correct), so this function documents, by construction,
// exactly which write produces which read's data.
func seedScopeIsoFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenant testfixtures.Tenant, buildingIdx int, cipher *crypto.Cipher) scopeIsoFixtures {
	t.Helper()
	require.Contains(t, []int{0, 1}, buildingIdx)

	f := scopeIsoFixtures{
		tenant:      tenant,
		buildingIdx: buildingIdx,
		buildingID:  tenant.Buildings[buildingIdx].ID,
		analyzerID:  tenant.Analyzers[buildingIdx*2].ID,
		plantID:     tenant.Plants[0].ID,
		tariffID:    tenant.Tariffs[buildingIdx].ID,
	}
	// Every write below is issued under the tenant's OWN AdminScope: this
	// function's job is to seed real rows, not to re-prove write isolation
	// (Task 9-11's own tests already do that). AdminScope reaches both
	// buildings of the tenant unconditionally.
	adminScope := tenant.AdminScope

	// --- sessions, users ----------------------------------------------------
	sessRepo := postgres.NewSessionRepository(pool)
	sess, err := sessRepo.Create(ctx, adminScope, model.Session{
		UserID: tenant.Users[model.UserRoleCompanyAdmin].ID, RefreshTokenHash: "scope-iso-hash-" + tenant.Company.Name,
		ExpiresAt: scopeIsoEpoch.Add(24 * time.Hour), CreatedAt: scopeIsoEpoch,
	})
	require.NoError(t, err)
	f.sessionID = sess.ID

	userRepo := postgres.NewUserRepository(pool)
	require.NoError(t, userRepo.SetPassword(ctx, adminScope, tenant.Users[model.UserRoleCompanyAdmin].ID, "scope-iso-new-hash", scopeIsoEpoch))

	// --- audit ---------------------------------------------------------------
	auditRepo := postgres.NewAuditRepository(pool)
	companyID := tenant.Company.ID
	_, err = auditRepo.Append(ctx, adminScope, model.AuditEntry{
		CompanyID: &companyID, Action: "scope-iso.seed", EntityType: "fixture", CreatedAt: scopeIsoEpoch,
	})
	require.NoError(t, err)

	// --- readings, cursor, anomaly, forecast (analyzer-parented) -----------
	f.readingsFrom = scopeIsoEpoch
	f.readingsTo = scopeIsoEpoch.Add(2 * time.Hour)
	readingRepo := postgres.NewReadingRepository(pool)
	_, _, err = readingRepo.BulkInsert(ctx, adminScope, []model.MeterReading{
		readingsRow(f.analyzerID, scopeIsoEpoch, model.ReadingKindLoadProfile, "1000.0000", "1.000000"),
		readingsRow(f.analyzerID, scopeIsoEpoch.Add(30*time.Minute), model.ReadingKindLoadProfile, "1010.0000", "1.000000"),
		readingsRow(f.analyzerID, scopeIsoEpoch.Add(time.Hour), model.ReadingKindLoadProfile, "1025.0000", "1.000000"),
	})
	require.NoError(t, err)

	f.cursorKind = model.ReadingKindLoadProfile
	cursorRepo := postgres.NewCursorRepository(pool)
	require.NoError(t, cursorRepo.RecordSuccess(ctx, adminScope, f.analyzerID, f.cursorKind, scopeIsoEpoch, scopeIsoEpoch.Add(time.Minute)))

	anomalyRepo := postgres.NewAnomalyRepository(pool)
	anomaly, err := anomalyRepo.Create(ctx, adminScope, anomaliesFixture(f.analyzerID))
	require.NoError(t, err)
	f.anomalyID = anomaly.ID

	f.forecastGeneratedAt = scopeIsoEpoch
	forecastRepo := postgres.NewForecastRepository(pool)
	_, _, err = forecastRepo.BulkInsert(ctx, adminScope, []model.Forecast{
		forecastsRow(f.analyzerID, scopeIsoEpoch, f.forecastGeneratedAt, "12.0000"),
	})
	require.NoError(t, err)
	require.NoError(t, forecastRepo.RecordGaps(ctx, adminScope, []model.ForecastGap{
		{AnalyzerID: f.analyzerID, GeneratedAt: f.forecastGeneratedAt, GapStart: scopeIsoEpoch, GapEnd: scopeIsoEpoch.Add(time.Hour), MissingHours: 1},
	}))

	// --- production (plant-parented, company-only) --------------------------
	f.deviceID = productionSeedDevice(t, ctx, pool, f.plantID, "SCOPE-ISO-"+tenant.Company.Name)
	productionRepo := postgres.NewProductionRepository(pool)
	_, _, err = productionRepo.BulkInsert(ctx, adminScope, []model.PlantProduction{
		productionRow(f.plantID, f.deviceID, scopeIsoEpoch, "5.0000"),
		productionRow(f.plantID, f.deviceID, scopeIsoEpoch.Add(time.Hour), "6.0000"),
	})
	require.NoError(t, err)

	plantRepo := postgres.NewPlantRepository(pool)
	_, err = plantRepo.ReplaceMonthlyTargets(ctx, adminScope, f.plantID, []model.PlantMonthlyTarget{
		{Month: 1, TargetKwh: scopeIsoDec("1000.000")},
	})
	require.NoError(t, err)
	_, err = plantRepo.ReplaceAlarmRecipients(ctx, adminScope, f.plantID, []string{"scope-iso-ops@example.invalid"})
	require.NoError(t, err)

	// --- tariff taxes / manual yekdem (tariff-parented) ----------------------
	tariffRepo := postgres.NewTariffRepository(pool)
	_, err = tariffRepo.ReplaceTaxes(ctx, adminScope, f.tariffID, []model.TariffTax{
		{Name: "scope-iso-tax", Rate: scopeIsoDec("1.000"), SortOrder: 1},
	})
	require.NoError(t, err)
	_, err = tariffRepo.ReplaceManualYekdem(ctx, adminScope, f.tariffID, []model.TariffManualYekdem{
		{Year: 2026, Month: 1, Value: scopeIsoDec("100.0000")},
	})
	require.NoError(t, err)

	tariffTemplateRepo := postgres.NewTariffTemplateRepository(pool)
	template, err := tariffTemplateRepo.Create(ctx, adminScope, model.TariffTemplate{
		CompanyID: companyID, Name: "scope-iso template", Payload: []byte(`{}`), CreatedAt: scopeIsoEpoch, UpdatedAt: scopeIsoEpoch,
	})
	require.NoError(t, err)
	f.tariffTemplateID = template.ID

	solarTariffRepo := postgres.NewSolarTariffRepository(pool)
	solarTariff, err := solarTariffRepo.Create(ctx, adminScope, model.SolarTariff{
		CompanyID: companyID, PlantID: f.plantID, EffectiveFrom: scopeIsoEpoch,
		FeedInTariff: scopeIsoDec("1.500000"), Currency: model.CurrencyTRY, CreatedAt: scopeIsoEpoch,
	})
	require.NoError(t, err)
	f.solarTariffID = solarTariff.ID

	icmalRepo := postgres.NewIcmalRepository(pool)
	imp, err := icmalRepo.CreateImport(ctx, adminScope, model.IcmalImport{FileName: "scope-iso.xlsx"})
	require.NoError(t, err)
	f.icmalImportID = imp.ID
	buildingID := f.buildingID
	_, err = icmalRepo.InsertRows(ctx, adminScope, imp.ID, []model.IcmalRow{{Period: "202601", BuildingID: &buildingID}})
	require.NoError(t, err)

	// --- bills (building-parented, with children) ----------------------------
	billRepo := postgres.NewBillRepository(pool)
	bill, err := billRepo.Create(ctx, adminScope, billFixtureRow(companyID, &buildingID, nil, model.BillScopeBuilding, "2026-01"),
		[]model.BillLine{{Code: "energy", Label: "Energy", Amount: billDec("10.0000")}}, nil)
	require.NoError(t, err)
	f.billID = bill.ID
	_, err = billRepo.ReplaceHourlyDetail(ctx, adminScope, bill.ID, []model.BillHourlyDetail{
		{Ts: scopeIsoEpoch, Consumption: billDec("1.0000"), PTF: billDec("2.0000"), Yekdem: billDec("0.5000"), Kbk: billDec("1.0000"), UnitPrice: billDec("3.0000"), Cost: billDec("3.0000")},
	})
	require.NoError(t, err)

	// --- reports ---------------------------------------------------------------
	reportRepo := postgres.NewReportRepository(pool)
	rep, err := reportRepo.Upsert(ctx, adminScope, reportFixtureRow(companyID, f.buildingID, "2026-01"))
	require.NoError(t, err)
	f.reportID = rep.ID

	// --- carbon: company-owned factor (+conversion), activity, report --------
	carbonRepo := postgres.NewCarbonRepository(pool)
	factor, err := carbonRepo.UpsertFactor(ctx, adminScope, model.EmissionFactor{
		CompanyID: &companyID, Key: "scope-iso-fuel-" + tenant.Company.Name, Label: "Scope Iso Fuel", MainCategory: "fuel",
		BaseFactor: carbonDec("1.10"), BaseUnit: "kg",
	})
	require.NoError(t, err)
	f.carbonFactorID = factor.ID
	require.NoError(t, carbonRepo.ReplaceConversions(ctx, adminScope, factor.ID, []model.EmissionFactorConversion{
		{Unit: "litre", Multiplier: carbonDec("0.5"), Label: "per litre"},
	}))
	require.NoError(t, carbonRepo.ReplaceSelectedActivities(ctx, adminScope, f.buildingID, []string{"electricity"}))

	activity, err := carbonRepo.CreateActivity(ctx, adminScope, carbonActivityFixture(companyID, f.buildingID))
	require.NoError(t, err)
	f.carbonActivityID = activity.ID

	carbonReport, err := carbonRepo.CreateReport(ctx, adminScope, model.CarbonReport{
		CompanyID: companyID, BuildingID: f.buildingID, Name: "Scope Iso Report", ReportType: "ghg", Period: "2026-Q1", Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	f.carbonReportID = carbonReport.ID

	// --- ISO 50001 (building-parented, with children) -------------------------
	isoRepo := postgres.NewISO50001Repository(pool)
	project, err := isoRepo.EnsureProject(ctx, adminScope, f.buildingID)
	require.NoError(t, err)
	f.isoProjectID = project.ID
	require.NoError(t, isoRepo.ReplaceClauseDates(ctx, adminScope, project.ID, []model.ISO50001ClauseDate{
		{ClauseID: "5", StartDate: iso50001DatePtr(scopeIsoEpoch), EndDate: iso50001DatePtr(scopeIsoEpoch.AddDate(0, 3, 0))},
	}))
	createdBy := tenant.Users[model.UserRoleCompanyAdmin].ID
	note, err := isoRepo.CreateNote(ctx, adminScope, model.ISO50001Note{ProjectID: project.ID, ClauseID: "5.1", Body: "scope-iso note", CreatedBy: &createdBy})
	require.NoError(t, err)
	f.isoNoteID = note.ID

	// --- alarms (company-only, with analyzer/channel/event children) ---------
	alarmRepo := postgres.NewAlarmRepository(pool)
	alarm, err := alarmRepo.Create(ctx, adminScope, alarmFixtureRow(companyID))
	require.NoError(t, err)
	f.alarmID = alarm.ID
	require.NoError(t, alarmRepo.ReplaceAnalyzers(ctx, adminScope, alarm.ID, []uuid.UUID{f.analyzerID}))
	require.NoError(t, alarmRepo.ReplaceChannels(ctx, adminScope, alarm.ID, []model.AlarmChannel{
		{Channel: model.NotifyChannelEmail, Target: "scope-iso@example.invalid"},
	}))
	ev, err := alarmRepo.CreateEvent(ctx, adminScope, model.AlarmEvent{AlarmID: alarm.ID, AnalyzerID: &f.analyzerID, TriggeredAt: scopeIsoEpoch, Message: "scope-iso fired"})
	require.NoError(t, err)
	f.alarmEventID = ev.ID

	// --- files, integrations, smtp (company-only) -----------------------------
	fileRepo := postgres.NewFileRepository(pool)
	file, err := fileRepo.Create(ctx, adminScope, fileFixture(companyID, "carbon", &buildingID))
	require.NoError(t, err)
	f.fileID = file.ID

	f.integrationDefinitionID = scopeIsoInsertIntegrationDefinition(t, ctx, pool, "scope-iso-"+tenant.Company.Name)
	integrationRepo := postgres.NewIntegrationRepository(pool, cipher)
	cred, err := integrationRepo.UpsertCredential(ctx, adminScope, model.IntegrationCredential{
		CompanyID: companyID, DefinitionID: f.integrationDefinitionID, IsActive: true,
	}, []byte("scope-iso-secret"), nil)
	require.NoError(t, err)
	f.credentialID = cred.ID

	smtpRepo := postgres.NewSMTPRepository(pool, cipher)
	_, err = smtpRepo.Upsert(ctx, adminScope, smtpSettingsFixture(companyID), []byte("scope-iso-pw"))
	require.NoError(t, err)

	// --- calendar (company-only) ----------------------------------------------
	calendarRepo := postgres.NewCalendarRepository(pool)
	calEvent, err := calendarRepo.CreateEvent(ctx, adminScope, calendarEventFixture(companyID, "Scope Iso Event", scopeIsoEpoch))
	require.NoError(t, err)
	f.calendarEventID = calEvent.ID
	require.NoError(t, calendarRepo.ReplaceWeekendDays(ctx, adminScope, []int16{0, 6}))
	vac, err := calendarRepo.CreateVacation(ctx, adminScope, calendarVacationFixture(companyID, scopeIsoEpoch, scopeIsoEpoch.AddDate(0, 0, 5)))
	require.NoError(t, err)
	f.calendarVacationID = vac.ID

	// --- ops (company-only) -----------------------------------------------------
	opsRepo := postgres.NewOpsRepository(pool)
	run, err := opsRepo.StartRun(ctx, adminScope, model.JobRun{CompanyID: &companyID, JobType: "scope-iso-job"})
	require.NoError(t, err)
	f.jobRunID = run.ID
	msg, err := opsRepo.AppendMessage(ctx, adminScope, model.OperationalMessage{
		CompanyID: &companyID, Kind: "job", Category: "scope-iso", Status: "success", Message: "scope-iso message",
	})
	require.NoError(t, err)
	f.opsMessageID = msg.ID

	return f
}

// scopeIsoInsertPlatformData seeds the platform-wide tables ONCE: market
// prices, YEKDEM and the national tariff schedule have no company_id at all,
// so there is nothing to seed "per tenant" — every tenant sees the identical
// rows, which is exactly what their subtests assert.
func scopeIsoInsertPlatformData(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	marketRepo := admin.NewMarketDataRepository(pool)
	_, err := marketRepo.UpsertHourlyPrices(ctx, []model.MarketPrice{
		{Ts: scopeIsoEpoch, PTF: decimal.RequireFromString("1500.0000")},
	})
	require.NoError(t, err)
	_, err = marketRepo.UpsertYekdem(ctx, []model.YekdemMonthly{
		{Year: 2026, Month: 1, Value: decimal.RequireFromString("42.5000")},
	})
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `insert into national_tariff_schedule
		(effective_from, user_group, voltage_level, term, energy_price, distribution_price, vat_rate)
		values ('2026-01-01', 'commercial', 'lv', 'monomial', 2.5, 0.5, 20)`)
	require.NoError(t, err)
}

// scopeIsoRequireExcludes fails t if items contains an element whose id (via
// get) equals forbidden. Used for List-shaped reads where the correct result
// is "the caller's own data, MINUS the foreign row" rather than empty —
// tenant A legitimately has its own rows in most of these lists.
func scopeIsoRequireExcludes[T any](t *testing.T, items []T, forbidden uuid.UUID, get func(T) uuid.UUID, msgAndArgs ...any) {
	t.Helper()
	require.False(t, scopeIsoContainsID(items, forbidden, get), msgAndArgs...)
}

// ---------------------------------------------------------------------------
// TestScopeIsolation
// ---------------------------------------------------------------------------

// TestScopeIsolation is the F1 acceptance test. See the file doc comment for
// the two-proof design (cross-tenant under AdminScope, narrow-scope under a
// same-tenant Buildings[1] grant) and for why the admin package is excluded.
func TestScopeIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)

	tenantA := testfixtures.NewTenant(t, ctx, pool, 90001)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 90002)

	cipher := scopeIsoCipher(t)

	// af: tenant A's OWN data on Buildings[1] — outside tenant.Scope's grant.
	// The narrow-scope subtests prove tenant A's narrow Scope cannot reach it.
	af := seedScopeIsoFixtures(t, ctx, pool, tenantA, 1, cipher)
	// bf: tenant B's data (building index 0; irrelevant under AdminScope).
	// The cross-tenant subtests prove tenant A's AdminScope cannot reach it.
	bf := seedScopeIsoFixtures(t, ctx, pool, tenantB, 0, cipher)

	scopeIsoInsertPlatformData(t, ctx, pool)

	narrowRange := store.TimeRange{From: scopeIsoEpoch, To: scopeIsoEpoch.Add(2 * time.Hour)}
	invalidScope := store.Scope{}

	// --- CompanyRepository (company-only) -----------------------------------
	t.Run("CompanyRepository", func(t *testing.T) {
		repo := postgres.NewCompanyRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, tenantB.Company.ID)
		require.ErrorIs(t, err, store.ErrNotFound)

		list, err := repo.List(ctx, tenantA.AdminScope, store.CompanyFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, tenantB.Company.ID, func(c model.Company) uuid.UUID { return c.ID })
	})

	// --- UserRepository (company-only) ---------------------------------------
	t.Run("UserRepository", func(t *testing.T) {
		repo := postgres.NewUserRepository(pool)
		theirUser := tenantB.Users[model.UserRoleCompanyAdmin]

		_, err := repo.Get(ctx, tenantA.AdminScope, theirUser.ID)
		require.ErrorIs(t, err, store.ErrNotFound)

		list, err := repo.List(ctx, tenantA.AdminScope, store.UserFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, theirUser.ID, func(u model.User) uuid.UUID { return u.ID })

		hist, err := repo.PasswordHistory(ctx, tenantA.AdminScope, theirUser.ID, 10)
		require.ErrorIs(t, err, store.ErrNotFound)
		require.Nil(t, hist)
	})

	// --- SessionRepository (company-only, joins through users) ---------------
	t.Run("SessionRepository", func(t *testing.T) {
		repo := postgres.NewSessionRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.sessionID)
		require.ErrorIs(t, err, store.ErrNotFound)

		list, err := repo.List(ctx, tenantA.AdminScope, store.SessionFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, bf.sessionID, func(s model.Session) uuid.UUID { return s.ID })
	})

	// --- AuditRepository (company-only) --------------------------------------
	t.Run("AuditRepository", func(t *testing.T) {
		repo := postgres.NewAuditRepository(pool)

		list, err := repo.List(ctx, tenantA.AdminScope, store.AuditFilter{})
		require.NoError(t, err)
		for _, e := range list {
			require.NotEqual(t, tenantB.Company.ID, *e.CompanyID, "tenant B's audit rows must never appear in tenant A's list")
		}
	})

	// --- BuildingRepository (building-scoped) ---------------------------------
	t.Run("BuildingRepository", func(t *testing.T) {
		repo := postgres.NewBuildingRepository(pool)

		// cross-tenant
		_, err := repo.Get(ctx, tenantA.AdminScope, tenantB.Buildings[0].ID)
		require.ErrorIs(t, err, store.ErrNotFound)
		crossList, err := repo.List(ctx, tenantA.AdminScope, store.BuildingFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, crossList, tenantB.Buildings[0].ID, func(b model.Building) uuid.UUID { return b.ID })
		_, err = repo.Contacts(ctx, tenantA.AdminScope, tenantB.Buildings[0].ID)
		require.ErrorIs(t, err, store.ErrNotFound)

		// narrow-scope: tenant A's own Buildings[1], outside tenant A's Scope
		_, err = repo.Get(ctx, tenantA.Scope, tenantA.Buildings[1].ID)
		require.ErrorIs(t, err, store.ErrNotFound)
		narrowList, err := repo.List(ctx, tenantA.Scope, store.BuildingFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, narrowList, tenantA.Buildings[1].ID, func(b model.Building) uuid.UUID { return b.ID })
		_, err = repo.Contacts(ctx, tenantA.Scope, tenantA.Buildings[1].ID)
		require.ErrorIs(t, err, store.ErrNotFound)

		_, err = repo.Get(ctx, invalidScope, tenantA.Buildings[0].ID)
		require.ErrorIs(t, err, store.ErrInvalidScope)
	})

	// --- AnalyzerRepository (building-scoped) ---------------------------------
	t.Run("AnalyzerRepository", func(t *testing.T) {
		repo := postgres.NewAnalyzerRepository(pool)
		theirAnalyzer := tenantB.Analyzers[0]

		_, err := repo.Get(ctx, tenantA.AdminScope, theirAnalyzer.ID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.GetByInstallation(ctx, tenantA.AdminScope, theirAnalyzer.Provider, theirAnalyzer.ProviderSubtype, theirAnalyzer.InstallationNumber)
		require.ErrorIs(t, err, store.ErrNotFound)
		crossList, err := repo.List(ctx, tenantA.AdminScope, store.AnalyzerFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, crossList, theirAnalyzer.ID, func(a model.Analyzer) uuid.UUID { return a.ID })

		outsideGrant := tenantA.Analyzers[2] // Buildings[1]
		_, err = repo.Get(ctx, tenantA.Scope, outsideGrant.ID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.GetByInstallation(ctx, tenantA.Scope, outsideGrant.Provider, outsideGrant.ProviderSubtype, outsideGrant.InstallationNumber)
		require.ErrorIs(t, err, store.ErrNotFound)
		narrowList, err := repo.List(ctx, tenantA.Scope, store.AnalyzerFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, narrowList, outsideGrant.ID, func(a model.Analyzer) uuid.UUID { return a.ID })
	})

	// --- PlantRepository (company-only: power_plants has no building_id) -----
	t.Run("PlantRepository", func(t *testing.T) {
		repo := postgres.NewPlantRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.plantID)
		require.ErrorIs(t, err, store.ErrNotFound)
		list, err := repo.List(ctx, tenantA.AdminScope, store.PlantFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, bf.plantID, func(p model.PowerPlant) uuid.UUID { return p.ID })

		_, err = repo.MonthlyTargets(ctx, tenantA.AdminScope, bf.plantID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Devices(ctx, tenantA.AdminScope, bf.plantID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.AlarmRecipients(ctx, tenantA.AdminScope, bf.plantID)
		require.ErrorIs(t, err, store.ErrNotFound)
	})

	// --- ReadingRepository (building-scoped via analyzers) --------------------
	t.Run("ReadingRepository", func(t *testing.T) {
		repo := postgres.NewReadingRepository(pool)

		_, err := repo.Range(ctx, tenantA.AdminScope, bf.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, _, err = repo.BoundaryReadings(ctx, tenantA.AdminScope, bf.analyzerID, model.ReadingKindLoadProfile, bf.readingsFrom, bf.readingsTo)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Latest(ctx, tenantA.AdminScope, bf.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.ErrorIs(t, err, store.ErrNotFound)

		_, err = repo.Range(ctx, tenantA.Scope, af.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, _, err = repo.BoundaryReadings(ctx, tenantA.Scope, af.analyzerID, model.ReadingKindLoadProfile, af.readingsFrom, af.readingsTo)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Latest(ctx, tenantA.Scope, af.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.ErrorIs(t, err, store.ErrNotFound)
	})

	// --- CursorRepository (building-scoped via analyzers) ----------------------
	t.Run("CursorRepository", func(t *testing.T) {
		repo := postgres.NewCursorRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.analyzerID, bf.cursorKind)
		require.ErrorIs(t, err, store.ErrNotFound)
		crossList, err := repo.List(ctx, tenantA.AdminScope, []uuid.UUID{bf.analyzerID})
		require.NoError(t, err)
		require.Empty(t, crossList, "an analyzer id not visible to the Scope contributes no rows to List")

		_, err = repo.Get(ctx, tenantA.Scope, af.analyzerID, af.cursorKind)
		require.ErrorIs(t, err, store.ErrNotFound)
		narrowList, err := repo.List(ctx, tenantA.Scope, []uuid.UUID{af.analyzerID})
		require.NoError(t, err)
		require.Empty(t, narrowList)
	})

	// --- AnomalyRepository (building-scoped via analyzers) ----------------------
	t.Run("AnomalyRepository", func(t *testing.T) {
		repo := postgres.NewAnomalyRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.anomalyID)
		require.ErrorIs(t, err, store.ErrNotFound)
		crossList, err := repo.List(ctx, tenantA.AdminScope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{bf.analyzerID}})
		require.NoError(t, err)
		require.Empty(t, crossList)

		_, err = repo.Get(ctx, tenantA.Scope, af.anomalyID)
		require.ErrorIs(t, err, store.ErrNotFound)
		narrowList, err := repo.List(ctx, tenantA.Scope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{af.analyzerID}})
		require.NoError(t, err)
		require.Empty(t, narrowList)
	})

	// --- ProductionRepository (company-only via power_plants) -------------------
	t.Run("ProductionRepository", func(t *testing.T) {
		repo := postgres.NewProductionRepository(pool)

		_, err := repo.Range(ctx, tenantA.AdminScope, bf.plantID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Latest(ctx, tenantA.AdminScope, bf.plantID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
	})

	// --- AnalyticsRepository (Consumption* building-scoped, Production*
	// company-only — see repository.go's own doc comment) ------------------------
	t.Run("AnalyticsRepository", func(t *testing.T) {
		repo := postgres.NewAnalyticsRepository(pool)

		crossHourly, err := repo.ConsumptionHourly(ctx, tenantA.AdminScope, []uuid.UUID{bf.analyzerID}, narrowRange)
		require.NoError(t, err)
		require.Empty(t, crossHourly)
		crossDaily, err := repo.ConsumptionDaily(ctx, tenantA.AdminScope, []uuid.UUID{bf.analyzerID}, narrowRange)
		require.NoError(t, err)
		require.Empty(t, crossDaily)
		crossMonthly, err := repo.ConsumptionMonthly(ctx, tenantA.AdminScope, []uuid.UUID{bf.analyzerID}, narrowRange)
		require.NoError(t, err)
		require.Empty(t, crossMonthly)
		crossYearly, err := repo.ConsumptionYearly(ctx, tenantA.AdminScope, []uuid.UUID{bf.analyzerID}, narrowRange)
		require.NoError(t, err)
		require.Empty(t, crossYearly)
		crossProdDaily, err := repo.ProductionDaily(ctx, tenantA.AdminScope, []uuid.UUID{bf.plantID}, narrowRange)
		require.NoError(t, err)
		require.Empty(t, crossProdDaily)
		crossProdMonthly, err := repo.ProductionMonthly(ctx, tenantA.AdminScope, []uuid.UUID{bf.plantID}, narrowRange)
		require.NoError(t, err)
		require.Empty(t, crossProdMonthly)

		narrowHourly, err := repo.ConsumptionHourly(ctx, tenantA.Scope, []uuid.UUID{af.analyzerID}, narrowRange)
		require.NoError(t, err)
		require.Empty(t, narrowHourly, "Buildings[1]'s analyzer must be invisible to the narrow Scope")
		narrowDaily, err := repo.ConsumptionDaily(ctx, tenantA.Scope, []uuid.UUID{af.analyzerID}, narrowRange)
		require.NoError(t, err)
		require.Empty(t, narrowDaily)
	})

	// --- PriceRepository (platform-wide: Scope narrows nothing) -----------------
	t.Run("PriceRepository", func(t *testing.T) {
		repo := postgres.NewPriceRepository(pool)

		_, err := repo.HourlyRange(ctx, invalidScope, narrowRange)
		require.ErrorIs(t, err, store.ErrInvalidScope)
		_, err = repo.Yekdem(ctx, invalidScope, 2026, 1)
		require.ErrorIs(t, err, store.ErrInvalidScope)

		fromA, err := repo.HourlyRange(ctx, tenantA.Scope, narrowRange)
		require.NoError(t, err)
		fromB, err := repo.HourlyRange(ctx, tenantB.Scope, narrowRange)
		require.NoError(t, err)
		require.Equal(t, len(fromA), len(fromB), "a platform table must return the identical rows under either tenant's Scope")
		require.NotEmpty(t, fromA, "this assertion is vacuous if the platform seed produced no rows")
	})

	// --- ForecastRepository (building-scoped via analyzers) -----------------------
	t.Run("ForecastRepository", func(t *testing.T) {
		repo := postgres.NewForecastRepository(pool)

		_, err := repo.Range(ctx, tenantA.AdminScope, bf.analyzerID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.LatestRun(ctx, tenantA.AdminScope, bf.analyzerID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Gaps(ctx, tenantA.AdminScope, bf.analyzerID, bf.forecastGeneratedAt)
		require.ErrorIs(t, err, store.ErrNotFound)

		_, err = repo.Range(ctx, tenantA.Scope, af.analyzerID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.LatestRun(ctx, tenantA.Scope, af.analyzerID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Gaps(ctx, tenantA.Scope, af.analyzerID, af.forecastGeneratedAt)
		require.ErrorIs(t, err, store.ErrNotFound)
	})

	// --- TariffRepository (building-scoped) ----------------------------------------
	t.Run("TariffRepository", func(t *testing.T) {
		repo := postgres.NewTariffRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.tariffID)
		require.ErrorIs(t, err, store.ErrNotFound)
		crossList, err := repo.List(ctx, tenantA.AdminScope, store.TariffFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, crossList, bf.tariffID, func(tar model.Tariff) uuid.UUID { return tar.ID })
		_, err = repo.Effective(ctx, tenantA.AdminScope, bf.buildingID, scopeIsoEpoch.Add(time.Hour))
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Taxes(ctx, tenantA.AdminScope, bf.tariffID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.ManualYekdem(ctx, tenantA.AdminScope, bf.tariffID)
		require.ErrorIs(t, err, store.ErrNotFound)

		_, err = repo.Get(ctx, tenantA.Scope, af.tariffID)
		require.ErrorIs(t, err, store.ErrNotFound)
		narrowList, err := repo.List(ctx, tenantA.Scope, store.TariffFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, narrowList, af.tariffID, func(tar model.Tariff) uuid.UUID { return tar.ID })
		_, err = repo.Effective(ctx, tenantA.Scope, af.buildingID, scopeIsoEpoch.Add(time.Hour))
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Taxes(ctx, tenantA.Scope, af.tariffID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.ManualYekdem(ctx, tenantA.Scope, af.tariffID)
		require.ErrorIs(t, err, store.ErrNotFound)
	})

	// --- TariffTemplateRepository (company-only) ------------------------------------
	t.Run("TariffTemplateRepository", func(t *testing.T) {
		repo := postgres.NewTariffTemplateRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.tariffTemplateID)
		require.ErrorIs(t, err, store.ErrNotFound)
		list, err := repo.List(ctx, tenantA.AdminScope, store.TariffTemplateFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, bf.tariffTemplateID, func(tt model.TariffTemplate) uuid.UUID { return tt.ID })
	})

	// --- SolarTariffRepository (company-only, prices a plant) ------------------------
	t.Run("SolarTariffRepository", func(t *testing.T) {
		repo := postgres.NewSolarTariffRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.solarTariffID)
		require.ErrorIs(t, err, store.ErrNotFound)
		list, err := repo.List(ctx, tenantA.AdminScope, store.SolarTariffFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, bf.solarTariffID, func(st model.SolarTariff) uuid.UUID { return st.ID })
		_, err = repo.Effective(ctx, tenantA.AdminScope, bf.plantID, scopeIsoEpoch.Add(time.Hour))
		require.ErrorIs(t, err, store.ErrNotFound)
	})

	// --- NationalTariffRepository (platform-wide) --------------------------------------
	t.Run("NationalTariffRepository", func(t *testing.T) {
		repo := postgres.NewNationalTariffRepository(pool)

		_, err := repo.List(ctx, invalidScope, store.NationalTariffFilter{})
		require.ErrorIs(t, err, store.ErrInvalidScope)

		fromA, err := repo.List(ctx, tenantA.Scope, store.NationalTariffFilter{})
		require.NoError(t, err)
		fromB, err := repo.List(ctx, tenantB.AdminScope, store.NationalTariffFilter{})
		require.NoError(t, err)
		require.NotEmpty(t, fromA)
		require.Equal(t, len(fromA), len(fromB))
	})

	// --- IcmalRepository (company-only import, building-tagged rows) -------------------
	t.Run("IcmalRepository", func(t *testing.T) {
		repo := postgres.NewIcmalRepository(pool)

		_, err := repo.GetImport(ctx, tenantA.AdminScope, bf.icmalImportID)
		require.ErrorIs(t, err, store.ErrNotFound)
		list, err := repo.ListImports(ctx, tenantA.AdminScope, store.IcmalFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, bf.icmalImportID, func(i model.IcmalImport) uuid.UUID { return i.ID })
		_, err = repo.ListRows(ctx, tenantA.AdminScope, bf.icmalImportID, store.Page{})
		require.ErrorIs(t, err, store.ErrNotFound)
	})

	// --- BillRepository (building-scoped, with several no-company_id children) --------
	t.Run("BillRepository", func(t *testing.T) {
		repo := postgres.NewBillRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.billID)
		require.ErrorIs(t, err, store.ErrNotFound)
		crossList, err := repo.List(ctx, tenantA.AdminScope, store.BillFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, crossList, bf.billID, func(b model.Bill) uuid.UUID { return b.ID })
		_, err = repo.Current(ctx, tenantA.AdminScope, model.BillScopeBuilding, bf.buildingID, "2026-01")
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Lines(ctx, tenantA.AdminScope, bf.billID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Members(ctx, tenantA.AdminScope, bf.billID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.HourlyDetail(ctx, tenantA.AdminScope, bf.billID)
		require.ErrorIs(t, err, store.ErrNotFound)

		_, err = repo.Get(ctx, tenantA.Scope, af.billID)
		require.ErrorIs(t, err, store.ErrNotFound)
		narrowList, err := repo.List(ctx, tenantA.Scope, store.BillFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, narrowList, af.billID, func(b model.Bill) uuid.UUID { return b.ID })
		_, err = repo.Current(ctx, tenantA.Scope, model.BillScopeBuilding, af.buildingID, "2026-01")
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Lines(ctx, tenantA.Scope, af.billID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Members(ctx, tenantA.Scope, af.billID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.HourlyDetail(ctx, tenantA.Scope, af.billID)
		require.ErrorIs(t, err, store.ErrNotFound)
	})

	// --- ReportRepository (building-scoped, building_id NOT NULL) ---------------------
	t.Run("ReportRepository", func(t *testing.T) {
		repo := postgres.NewReportRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.reportID)
		require.ErrorIs(t, err, store.ErrNotFound)
		crossList, err := repo.List(ctx, tenantA.AdminScope, store.ReportFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, crossList, bf.reportID, func(r model.Report) uuid.UUID { return r.ID })

		_, err = repo.Get(ctx, tenantA.Scope, af.reportID)
		require.ErrorIs(t, err, store.ErrNotFound)
		narrowList, err := repo.List(ctx, tenantA.Scope, store.ReportFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, narrowList, af.reportID, func(r model.Report) uuid.UUID { return r.ID })
	})

	// --- AlarmRepository (company-only: alarms carries no building_id) ----------------
	t.Run("AlarmRepository", func(t *testing.T) {
		repo := postgres.NewAlarmRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.alarmID)
		require.ErrorIs(t, err, store.ErrNotFound)
		list, err := repo.List(ctx, tenantA.AdminScope, store.AlarmFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, bf.alarmID, func(a model.Alarm) uuid.UUID { return a.ID })
		_, err = repo.Analyzers(ctx, tenantA.AdminScope, bf.alarmID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Channels(ctx, tenantA.AdminScope, bf.alarmID)
		require.ErrorIs(t, err, store.ErrNotFound)
		events, err := repo.ListEvents(ctx, tenantA.AdminScope, store.AlarmEventFilter{AlarmID: &bf.alarmID})
		require.NoError(t, err)
		require.Empty(t, events)
	})

	// --- CarbonRepository (factors company/platform; activities+reports
	// building-scoped) -----------------------------------------------------------------
	t.Run("CarbonRepository", func(t *testing.T) {
		repo := postgres.NewCarbonRepository(pool)

		_, err := repo.Factor(ctx, tenantA.AdminScope, bf.carbonFactorID)
		require.ErrorIs(t, err, store.ErrNotFound)
		factorList, err := repo.ListFactors(ctx, tenantA.AdminScope, store.EmissionFactorFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, factorList, bf.carbonFactorID, func(f model.EmissionFactor) uuid.UUID { return f.ID })
		_, err = repo.Conversions(ctx, tenantA.AdminScope, bf.carbonFactorID)
		require.ErrorIs(t, err, store.ErrNotFound)
		// SelectedActivities has no ErrNotFound path (repository.go carries
		// no such note for it, and the implementation just filters by scope
		// and returns whatever matches): a building the Scope cannot see
		// yields an empty slice with a nil error, never an error.
		crossSelected, err := repo.SelectedActivities(ctx, tenantA.AdminScope, bf.buildingID)
		require.NoError(t, err)
		require.Empty(t, crossSelected)
		_, err = repo.Activity(ctx, tenantA.AdminScope, bf.carbonActivityID)
		require.ErrorIs(t, err, store.ErrNotFound)
		activityList, err := repo.ListActivities(ctx, tenantA.AdminScope, store.CarbonActivityFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, activityList, bf.carbonActivityID, func(a model.CarbonActivity) uuid.UUID { return a.ID })
		reportList, err := repo.ListReports(ctx, tenantA.AdminScope, nil, store.Page{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, reportList, bf.carbonReportID, func(r model.CarbonReport) uuid.UUID { return r.ID })

		narrowSelected, err := repo.SelectedActivities(ctx, tenantA.Scope, af.buildingID)
		require.NoError(t, err)
		require.Empty(t, narrowSelected, "Buildings[1]'s selected activities must be invisible to the narrow Scope")
		_, err = repo.Activity(ctx, tenantA.Scope, af.carbonActivityID)
		require.ErrorIs(t, err, store.ErrNotFound)
		narrowActivityList, err := repo.ListActivities(ctx, tenantA.Scope, store.CarbonActivityFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, narrowActivityList, af.carbonActivityID, func(a model.CarbonActivity) uuid.UUID { return a.ID })
		narrowReportList, err := repo.ListReports(ctx, tenantA.Scope, nil, store.Page{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, narrowReportList, af.carbonReportID, func(r model.CarbonReport) uuid.UUID { return r.ID })
	})

	// --- ISO50001Repository (building-scoped project, with no-company_id
	// children) -----------------------------------------------------------------------
	t.Run("ISO50001Repository", func(t *testing.T) {
		repo := postgres.NewISO50001Repository(pool)

		_, err := repo.Project(ctx, tenantA.AdminScope, bf.buildingID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.ClauseDates(ctx, tenantA.AdminScope, bf.isoProjectID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Notes(ctx, tenantA.AdminScope, bf.isoProjectID, nil)
		require.ErrorIs(t, err, store.ErrNotFound)

		_, err = repo.Project(ctx, tenantA.Scope, af.buildingID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.ClauseDates(ctx, tenantA.Scope, af.isoProjectID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Notes(ctx, tenantA.Scope, af.isoProjectID, nil)
		require.ErrorIs(t, err, store.ErrNotFound)
	})

	// --- FileRepository (company-only, no building_id) ---------------------------------
	t.Run("FileRepository", func(t *testing.T) {
		repo := postgres.NewFileRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.fileID)
		require.ErrorIs(t, err, store.ErrNotFound)
		list, err := repo.List(ctx, tenantA.AdminScope, store.FileFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, bf.fileID, func(f model.StoredFile) uuid.UUID { return f.ID })
	})

	// --- IntegrationRepository (Definitions/Definition platform-wide;
	// Credential/ListCredentials company-only) -------------------------------------------
	t.Run("IntegrationRepository", func(t *testing.T) {
		repo := postgres.NewIntegrationRepository(pool, cipher)

		_, err := repo.Definitions(ctx, invalidScope)
		require.ErrorIs(t, err, store.ErrInvalidScope)
		defsA, err := repo.Definitions(ctx, tenantA.Scope)
		require.NoError(t, err)
		defsB, err := repo.Definitions(ctx, tenantB.Scope)
		require.NoError(t, err)
		require.Equal(t, len(defsA), len(defsB), "the platform catalogue is identical under either tenant's Scope")
		// Both tenants' own definitions (af's and bf's, distinct rows) must
		// be visible under EITHER tenant's Scope: the table is platform-wide.
		require.True(t, scopeIsoContainsID(defsA, af.integrationDefinitionID, func(d model.IntegrationDefinition) uuid.UUID { return d.ID }))
		require.True(t, scopeIsoContainsID(defsA, bf.integrationDefinitionID, func(d model.IntegrationDefinition) uuid.UUID { return d.ID }))

		_, err = repo.Definition(ctx, invalidScope, model.IntegrationProviderOSOS, "scope-iso-"+tenantB.Company.Name)
		require.ErrorIs(t, err, store.ErrInvalidScope)
		gotDef, err := repo.Definition(ctx, tenantA.Scope, model.IntegrationProviderOSOS, "scope-iso-"+tenantB.Company.Name)
		require.NoError(t, err)
		require.Equal(t, bf.integrationDefinitionID, gotDef.ID, "a platform definition tenant B created must be readable under tenant A's Scope too")

		// Credential is keyed by DEFINITION id, not by a credential surrogate
		// id: bf.integrationDefinitionID is a definition ONLY tenant B ever
		// created a credential against, so a non-ErrNotFound result here
		// could only be tenant B's own credential leaking through.
		_, err = repo.Credential(ctx, tenantA.AdminScope, bf.integrationDefinitionID)
		require.ErrorIs(t, err, store.ErrNotFound)
		list, err := repo.ListCredentials(ctx, tenantA.AdminScope)
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, bf.credentialID, func(c model.IntegrationCredential) uuid.UUID { return c.ID })
	})

	// --- SMTPRepository (company-only, one row per company keyed by
	// company_id — no id parameter anywhere on this interface, so the
	// isolation proof is on the CONTENT of what Get returns rather than on
	// ErrNotFound: both tenants have their own row, seeded with the identical
	// fixture shape by seedScopeIsoFixtures, so a leak would be invisible
	// unless the returned CompanyID is checked directly) -----------------------------
	t.Run("SMTPRepository", func(t *testing.T) {
		repo := postgres.NewSMTPRepository(pool, cipher)

		got, err := repo.Get(ctx, tenantA.AdminScope)
		require.NoError(t, err)
		require.Equal(t, tenantA.Company.ID, got.CompanyID, "tenant A's Get must never return tenant B's smtp_settings row")
	})

	// --- CalendarRepository (company-only) ------------------------------------------------
	t.Run("CalendarRepository", func(t *testing.T) {
		repo := postgres.NewCalendarRepository(pool)

		_, err := repo.Event(ctx, tenantA.AdminScope, bf.calendarEventID)
		require.ErrorIs(t, err, store.ErrNotFound)
		list, err := repo.ListEvents(ctx, tenantA.AdminScope, store.CalendarFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, bf.calendarEventID, func(e model.CalendarEvent) uuid.UUID { return e.ID })

		vac, err := repo.Vacations(ctx, tenantA.AdminScope, nil)
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, vac, bf.calendarVacationID, func(v model.CompanyVacation) uuid.UUID { return v.ID })

		// WeekendDays' rows have no independent surrogate id to exclude by
		// (company_id IS the row's identity beyond day_of_week), and both
		// tenants' seeding sets the identical value set ([0,6]) — so the
		// isolation proof is on the CompanyID column of what comes back, not
		// on presence/absence of a particular day value.
		days, err := repo.WeekendDays(ctx, tenantA.AdminScope)
		require.NoError(t, err)
		require.NotEmpty(t, days, "this assertion is vacuous if tenant A's own weekend days were never seeded")
		for _, d := range days {
			require.Equal(t, tenantA.Company.ID, d.CompanyID, "tenant B's weekend days must never appear in tenant A's WeekendDays")
		}
	})

	// --- OpsRepository (company-only) -----------------------------------------------------
	t.Run("OpsRepository", func(t *testing.T) {
		repo := postgres.NewOpsRepository(pool)

		_, err := repo.GetRun(ctx, tenantA.AdminScope, bf.jobRunID)
		require.ErrorIs(t, err, store.ErrNotFound)
		list, err := repo.ListRuns(ctx, tenantA.AdminScope, store.JobRunFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, list, bf.jobRunID, func(r model.JobRun) uuid.UUID { return r.ID })

		// Both tenants' seeding appends a message with the identical Category
		// ("scope-iso"), so the isolation proof is on the message's own ID
		// (bigserial, globally unique) rather than on its Category value.
		messages, err := repo.ListMessages(ctx, tenantA.AdminScope, store.MessageFilter{})
		require.NoError(t, err)
		for _, m := range messages {
			require.NotEqual(t, bf.opsMessageID, m.ID, "tenant B's operational message must never appear in tenant A's list")
		}
	})
}
