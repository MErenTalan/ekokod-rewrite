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
// ONE admin repository (admin.NewMarketDataRepository) plus raw SQL inserts
// matching other integration tests' own pattern, purely as SETUP for the
// platform-wide-table subtests, never as a test subject.
//
// FIX ROUND 1 (task-13b-fix1-findings.md). Every subtest below now pairs
// each cross-tenant/narrow-scope ErrNotFound or exclusion assertion with a
// POSITIVE CONTROL: the identical read, re-run under the OWNING Scope,
// asserted to succeed and (for a List) to contain the row — a query that
// always answers "not found" or "no rows" (e.g. an accidental `and false`)
// now fails here instead of passing vacuously as "isolated". See
// scopeIsoAssertGetNotFound/scopeIsoAssertListExcludes below.
// TestScopeIsolationCoversEveryRepository also gained a METHOD-level phase:
// every READ method (by mechanical name-prefix classification) of every
// non-admin interface in repository.go must be called inside that
// interface's own subtest here, or be named in scopeIsoReadMethodExemptions
// with a reason.
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
	"strings"
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
	"NewCompanyRepository":          true,
	"NewUserRepository":             true,
	"NewSessionRepository":          true,
	"NewPasswordResetRepository":    true,
	"NewAuditRepository":            true,
	"NewBuildingRepository":         true,
	"NewAnalyzerRepository":         true,
	"NewPlantRepository":            true,
	"NewReadingRepository":          true,
	"NewCursorRepository":           true,
	"NewAnomalyRepository":          true,
	"NewProductionRepository":       true,
	"NewProductionTotalsRepository": true,
	"NewFaultRepository":            true,
	"NewAnalyticsRepository":        true,
	"NewPriceRepository":            true,
	"NewBillingParameterRepository": true,
	"NewForecastRepository":         true,
	"NewTariffRepository":           true,
	"NewTariffTemplateRepository":   true,
	"NewBulkAssignmentRepository":   true,
	"NewSolarTariffRepository":      true,
	"NewNationalTariffRepository":   true,
	"NewIcmalRepository":            true,
	"NewBillRepository":             true,
	"NewReportRepository":           true,
	"NewAlarmRepository":            true,
	"NewCarbonRepository":           true,
	"NewISO50001Repository":         true,
	"NewFileRepository":             true,
	"NewIntegrationRepository":      true,
	"NewSMTPRepository":             true,
	"NewCalendarRepository":         true,
	"NewOpsRepository":              true,
	"NewGenerationRepository":       true,
	"NewProviderSeriesRepository":   true,
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
	t.Parallel()
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

	// --- Method-level omission guard (folded Minor 1, fix round 1) --------
	// Constructor-level coverage above says every REPOSITORY is exercised;
	// this phase says every READ METHOD of every one of them is too. A
	// subtest is matched to the interface it covers by the "repo :=
	// postgres.New<X>Repository(...)" convention every subtest in this file
	// follows (see the file's own doc comment) — this guard depends on that
	// convention holding.
	repoGoPath := filepath.Join(dir, "..", "repository.go")
	interfaces := scopeIsoParseRepositoryInterfaces(t, repoGoPath)
	require.NotEmpty(t, interfaces, "the repository.go interface scan found nothing — the scan itself is broken")

	covered := scopeIsoParseSubtestCoverage(t, thisFile)

	var uncoveredReads []string
	for _, iface := range interfaces {
		for _, m := range iface.methods {
			if scopeIsoIsWriteMethod(m) {
				continue // writes are exempt as a category (Task 9-11's own tests own write isolation)
			}
			key := iface.name + "." + m
			if scopeIsoReadMethodExemptions[key] != "" {
				continue
			}
			if !covered[iface.name][m] {
				uncoveredReads = append(uncoveredReads, key)
			}
		}
	}
	sort.Strings(uncoveredReads)
	require.Empty(t, uncoveredReads,
		"read method(s) of a scoped repository are never called by TestScopeIsolation's own subtest for that "+
			"repository, and are not named in scopeIsoReadMethodExemptions with a reason: %v", uncoveredReads)
}

func isTestGoFile(name string) bool {
	return len(name) > len("_test.go") && name[len(name)-len("_test.go"):] == "_test.go"
}

// scopeIsoWriteMethodPrefixes classifies a repository method as a WRITE by
// name prefix, mechanically, rather than hand-maintaining a per-interface
// read/write split that silently drifts as repository.go grows. Every write
// method across all 29 non-admin interfaces (checked by hand against
// repository.go's full method list during this fix round) starts with one
// of these; a method matching none of them is a READ, and
// TestScopeIsolationCoversEveryRepository's method-level phase requires it
// to be called (or explicitly exempted in scopeIsoReadMethodExemptions).
var scopeIsoWriteMethodPrefixes = []string{
	"Create", "Update", "SoftDelete", "Delete", "Replace", "Append",
	"BulkInsert", "Insert", "Set", "Record", "Revoke", "Start", "Finish",
	"Mark", "Resolve", "Touch", "Upsert", "Supersede", "Ensure", "Rotate",
}

func scopeIsoIsWriteMethod(name string) bool {
	for _, p := range scopeIsoWriteMethodPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// scopeIsoReadMethodExemptions lists a READ method (key "InterfaceName.Method")
// TestScopeIsolation's own subtest for that interface deliberately does not
// call, with the reason. Empty by design: every read method of every
// non-admin interface is exercised somewhere in TestScopeIsolation as of
// this fix round (see task-13b-report.md's coverage table) — an entry here
// is meant to be a reviewed, documented exception, never a silent gap.
var scopeIsoReadMethodExemptions = map[string]string{}

// scopeIsoInterfaceMethods is one non-admin *Repository interface's name and
// its exported method names, as found in repository.go.
type scopeIsoInterfaceMethods struct {
	name    string
	methods []string
}

// scopeIsoParseRepositoryInterfaces AST-scans repoGoPath (internal/store/
// repository.go) for every `type XRepository interface { ... }` declaration
// whose name is NOT prefixed "Admin" (the single unscoped surface, excluded
// by design exactly as the constructor-level scan above excludes package
// admin) and returns each one's exported method names.
func scopeIsoParseRepositoryInterfaces(t *testing.T, repoGoPath string) []scopeIsoInterfaceMethods {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, repoGoPath, nil, 0)
	require.NoError(t, err)
	require.Equal(t, "store", file.Name.Name, "%s is not in package store — the scan path is wrong", repoGoPath)

	var out []scopeIsoInterfaceMethods
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			it, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			name := ts.Name.Name
			if !strings.HasSuffix(name, "Repository") || strings.HasPrefix(name, "Admin") {
				continue
			}
			var methods []string
			for _, m := range it.Methods.List {
				for _, n := range m.Names { // embedded interfaces have no Names; none exist here
					if n.IsExported() {
						methods = append(methods, n.Name)
					}
				}
			}
			out = append(out, scopeIsoInterfaceMethods{name: name, methods: methods})
		}
	}
	return out
}

// scopeIsoParseSubtestCoverage AST-scans testFilePath (this file) for
// TestScopeIsolation's own FuncDecl, walks every `t.Run("Name", func(t
// *testing.T) {...})` inside it, and returns, per interface name, the set
// of method names called on a variable that subtest constructed via
// `postgres.New<X>Repository(...)` — including calls reached only through a
// nested closure (e.g. scopeIsoAssertGetNotFound's `get` callback), since
// ast.Inspect walks the whole subtree regardless of nesting depth.
func scopeIsoParseSubtestCoverage(t *testing.T, testFilePath string) map[string]map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, testFilePath, nil, 0)
	require.NoError(t, err)

	var testFn *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "TestScopeIsolation" {
			testFn = fn
			break
		}
	}
	require.NotNil(t, testFn, "TestScopeIsolation itself was not found by the AST scan")

	covered := map[string]map[string]bool{}
	ast.Inspect(testFn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Run" || len(call.Args) != 2 {
			return true
		}
		lit, ok := call.Args[1].(*ast.FuncLit)
		if !ok {
			return true
		}
		scopeIsoCollectSubtestCoverage(lit.Body, covered)
		return true
	})
	return covered
}

// scopeIsoCollectSubtestCoverage walks one t.Run subtest body: first every
// "<ident> := postgres.New<X>Repository(...)" assignment, recording ident ->
// X; then every "<ident>.Method(...)" call for an ident found in the first
// pass, recording X.Method as covered.
func scopeIsoCollectSubtestCoverage(body ast.Node, covered map[string]map[string]bool) {
	repoVars := map[string]string{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "postgres" || !strings.HasPrefix(sel.Sel.Name, "New") || !strings.HasSuffix(sel.Sel.Name, "Repository") {
			return true
		}
		repoVars[ident.Name] = strings.TrimPrefix(sel.Sel.Name, "New")
		return true
	})
	if len(repoVars) == 0 {
		return
	}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		recv, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		iface, ok := repoVars[recv.Name]
		if !ok {
			return true
		}
		if covered[iface] == nil {
			covered[iface] = map[string]bool{}
		}
		covered[iface][sel.Sel.Name] = true
		return true
	})
}

// ---------------------------------------------------------------------------
// Shared fixtures
// ---------------------------------------------------------------------------

var scopeIsoEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// scopeIsoClosedYearEpoch is a year definitely CLOSED relative to this
// phase's clock — unlike scopeIsoEpoch's year (2026), which is the CURRENT
// year and therefore never materialised by refresh_continuous_aggregate
// (Important Finding 2: consumption_yearly is materialized_only = true, and
// the bucket "now" is still inside is never in the materialized data — the
// same trap TestAnalyticsConsumptionMonthlyAndYearlyRequireRefresh's own
// closedYearEpoch avoids).
var scopeIsoClosedYearEpoch = time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)

// scopeIsoCompanyWideTariffFrom/On are the Important Finding 4 / HANDOFF
// ruling 7 company-wide tariff's effective_from and the date Effective is
// resolved at in the TariffRepository subtest. Both predate
// testfixtures.NewTenant's own building-specific tariffs (effective_from
// scopeIsoEpoch, 2026-01-01), so at scopeIsoCompanyWideTariffOn the
// building-specific lookup finds nothing and TariffRepository.Effective
// falls back to the company-wide row — the documented exception under a
// narrow Scope.
var (
	scopeIsoCompanyWideTariffFrom = tariffFixtureDate(2025, time.January, 1)
	scopeIsoCompanyWideTariffOn   = tariffFixtureDate(2025, time.June, 1)
)

func scopeIsoDec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func scopeIsoContainsID[T any](items []T, id uuid.UUID, get func(T) uuid.UUID) bool {
	for _, it := range items {
		if get(it) == id {
			return true
		}
	}
	return false
}

// scopeIsoAssertGetNotFound calls get(deniedScope) and requires
// store.ErrNotFound, THEN calls get(ownerScope) and requires success — the
// positive control (Important Finding 5) that keeps the ErrNotFound
// assertion from passing vacuously if the underlying query always answers
// "not found" (e.g. a mutated predicate that excludes everyone, not just
// the other tenant).
func scopeIsoAssertGetNotFound[T any](t *testing.T, get func(store.Scope) (T, error), deniedScope, ownerScope store.Scope, msgAndArgs ...any) {
	t.Helper()
	_, err := get(deniedScope)
	require.ErrorIs(t, err, store.ErrNotFound, msgAndArgs...)
	_, err = get(ownerScope)
	require.NoError(t, err, msgAndArgs...)
}

// scopeIsoAssertListExcludes calls list(deniedScope), requires forbiddenID
// absent, THEN calls list(ownerScope) and requires forbiddenID PRESENT — the
// List-shaped equivalent of scopeIsoAssertGetNotFound: a List that always
// returns nothing for anyone would otherwise satisfy scopeIsoRequireExcludes
// vacuously.
func scopeIsoAssertListExcludes[T any](t *testing.T, list func(store.Scope) ([]T, error), deniedScope, ownerScope store.Scope, forbiddenID uuid.UUID, get func(T) uuid.UUID, msgAndArgs ...any) {
	t.Helper()
	items, err := list(deniedScope)
	require.NoError(t, err, msgAndArgs...)
	scopeIsoRequireExcludes(t, items, forbiddenID, get, msgAndArgs...)
	ownItems, err := list(ownerScope)
	require.NoError(t, err, msgAndArgs...)
	require.True(t, scopeIsoContainsID(ownItems, forbiddenID, get), msgAndArgs...)
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
	bulkAssignmentID uuid.UUID

	icmalImportID uuid.UUID

	deviceID uuid.UUID

	// Fix round 1 additions (task-13b-fix1-findings.md).

	// alarmEventNullAnalyzerID is a company-level alarm event (AnalyzerID
	// nil) on the SAME alarm as alarmEventID — Important Finding 1: the
	// alarm_events -> alarms company_id predicate is the ONLY thing that
	// protects it, since AlarmEventList's NULL-analyzer branch checks
	// only all_buildings, never company_id directly.
	alarmEventNullAnalyzerID uuid.UUID

	// companyWideTariffID, companyBillID and unassignedAnalyzerID are
	// Important Finding 4 / HANDOFF ruling 7 rows: building_id NULL,
	// visible only to AllBuildings.
	companyWideTariffID  uuid.UUID
	companyBillID        uuid.UUID
	unassignedAnalyzerID uuid.UUID

	// smtpPassword is the plaintext password seeded for THIS tenant's
	// smtp_settings row — controller ruling (final review A fix round): it
	// must be DISTINCT per tenant, or a SMTPGet tautology that returns the
	// physically-first smtp_settings row (company_id is the table's own
	// primary key, so a :one tautology just returns row 1) would be
	// invisible whenever the first-seeded tenant happens to check its own
	// Get/OpenPassword only — see the SMTPRepository subtest below.
	smtpPassword []byte
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
	companyID := tenant.Company.ID

	// --- unassigned analyzer (Important Finding 4 / HANDOFF ruling 7) ------
	// building_id NULL: a narrow Scope may never CREATE one (that guard is
	// Task 9's own), but AdminScope may, and this fixture proves a narrow
	// Scope can never READ one either.
	analyzerRepo := postgres.NewAnalyzerRepository(pool)
	unassigned, err := analyzerRepo.Create(ctx, adminScope, model.Analyzer{
		CompanyID: companyID, Provider: model.IntegrationProviderOSOS,
		ProviderSubtype: "scope-iso", InstallationNumber: "SCOPE-ISO-UNASSIGNED-" + tenant.Company.Name,
		MeterMultiplier: decimal.RequireFromString("1"), IsActive: true,
		CreatedAt: scopeIsoEpoch, UpdatedAt: scopeIsoEpoch,
	})
	require.NoError(t, err)
	f.unassignedAnalyzerID = unassigned.ID

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
		// Important Finding 2: consumption_yearly proof rows, in a year
		// (scopeIsoClosedYearEpoch) definitely closed relative to this
		// phase's clock — scopeIsoEpoch's own year (2026) never is, so the
		// yearly continuous aggregate would never materialise anything for
		// it no matter how thoroughly it is refreshed.
		readingsRow(f.analyzerID, scopeIsoClosedYearEpoch, model.ReadingKindLoadProfile, "2000.0000", "1.000000"),
		readingsRow(f.analyzerID, scopeIsoClosedYearEpoch.Add(time.Hour), model.ReadingKindLoadProfile, "2010.0000", "1.000000"),
	})
	require.NoError(t, err)

	f.cursorKind = model.ReadingKindLoadProfile
	cursorRepo := postgres.NewCursorRepository(pool)
	require.NoError(t, cursorRepo.RecordSuccess(ctx, adminScope, f.analyzerID, f.cursorKind, scopeIsoEpoch, scopeIsoEpoch.Add(time.Minute)))

	// --- generation anchor, provider hourly values (analyzer-parented, F2) --
	// Both tables (migrations 00012/00013) have no company_id at all —
	// isolation is entirely the analyzers join, exactly like readings and
	// cursors above. Ts values reuse f.readingsFrom/f.readingsTo's window
	// (scopeIsoEpoch, already the readingRepo/narrowRange window) rather than
	// a new field, since every subtest below reads inside that same range.
	genRepo := postgres.NewGenerationRepository(pool)
	require.NoError(t, genRepo.SetAnchor(ctx, adminScope, model.GenerationAnchor{
		AnalyzerID:   f.analyzerID,
		AnchorTs:     f.readingsFrom,
		ActiveExport: decimal.RequireFromString("500.0000"),
		Source:       "initial",
	}))

	seriesRepo := postgres.NewProviderSeriesRepository(pool)
	activeGeneration := decimal.RequireFromString("12.0000")
	_, _, err = seriesRepo.UpsertHourly(ctx, adminScope, []model.ProviderHourlyValue{
		{
			AnalyzerID:       f.analyzerID,
			Ts:               f.readingsFrom,
			ActiveGeneration: &activeGeneration,
			SourceProvider:   model.IntegrationProviderOSOS,
		},
	})
	require.NoError(t, err)

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
	// R276: the plant aggregates and totals reads use plant_production_totals.
	seedTotal(t, ctx, pool, f.plantID, scopeIsoEpoch, "5.0000")
	seedTotal(t, ctx, pool, f.plantID, scopeIsoEpoch.Add(time.Hour), "6.0000")
	_, err = postgres.NewFaultRepository(pool).Upsert(ctx, adminScope, []model.PlantFault{{
		PlantID: f.plantID, Ref: "scope-iso-fault", Code: "10", Name: "fault", OccurredAt: scopeIsoEpoch, FirstSeenAt: scopeIsoEpoch,
	}})
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
	_, err = tariffRepo.ReplaceExtraCharges(ctx, adminScope, f.tariffID, []model.TariffExtraCharge{
		{Name: "scope-iso-extra", Basis: model.ExtraChargeBasisFixedPerPeriod, Amount: scopeIsoDec("1.000")},
	})
	require.NoError(t, err)

	// Important Finding 4 / HANDOFF ruling 7: a company-wide tariff
	// (building_id nil) is visible only to AllBuildings; its effective_from
	// predates every building-specific tariff testfixtures.NewTenant seeds,
	// so TariffRepository.Effective can fall back to it (see
	// scopeIsoCompanyWideTariffFrom/On's own doc comment).
	companyWideTariff, err := tariffRepo.Create(ctx, adminScope, tariffFixtureRow(companyID, nil, scopeIsoCompanyWideTariffFrom))
	require.NoError(t, err)
	f.companyWideTariffID = companyWideTariff.ID

	tariffTemplateRepo := postgres.NewTariffTemplateRepository(pool)
	template, err := tariffTemplateRepo.Create(ctx, adminScope, model.TariffTemplate{
		CompanyID: companyID, Name: "scope-iso template", Payload: []byte(`{}`), CreatedAt: scopeIsoEpoch, UpdatedAt: scopeIsoEpoch,
	})
	require.NoError(t, err)
	f.tariffTemplateID = template.ID

	bulkRepo := postgres.NewBulkAssignmentRepository(pool)
	assignment, err := bulkRepo.Create(ctx, adminScope, model.TariffBulkAssignment{
		CompanyID: companyID, TemplateID: &template.ID, EffectiveFrom: scopeIsoEpoch,
		BuildingIDs: []uuid.UUID{f.buildingID},
	})
	require.NoError(t, err)
	f.bulkAssignmentID = assignment.ID

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

	// Important Finding 4 / HANDOFF ruling 7: a company-level bill
	// (building_id AND analyzer_id both nil) is visible only to AllBuildings.
	companyBill, err := billRepo.Create(ctx, adminScope, billFixtureRow(companyID, nil, nil, model.BillScopeCompany, "2026-01"), nil, nil)
	require.NoError(t, err)
	f.companyBillID = companyBill.ID

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

	// Important Finding 1: a NULL-analyzer event (a company-level condition)
	// on the SAME alarm. AlarmEventList's own query only checks all_buildings
	// for this branch, never company_id — the alarms-parent company_id
	// predicate is the ONLY thing that can exclude it cross-tenant, which
	// f.alarmEventID (analyzer set) cannot prove because the analyzer join's
	// OWN company_id check shadows it.
	evNull, err := alarmRepo.CreateEvent(ctx, adminScope, model.AlarmEvent{AlarmID: alarm.ID, AnalyzerID: nil, TriggeredAt: scopeIsoEpoch.Add(time.Minute), Message: "scope-iso null-analyzer fired"})
	require.NoError(t, err)
	f.alarmEventNullAnalyzerID = evNull.ID

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
	// Controller ruling (final review A fix round): the password must be
	// DISTINCT per tenant (never the shared literal "scope-iso-pw" both
	// tenants used to write), or a SMTPGet tautology's first-row return is
	// invisible to a proof that only checks each tenant's OWN Get/
	// OpenPassword. See scopeIsoFixtures.smtpPassword.
	f.smtpPassword = []byte("scope-iso-pw-" + tenant.Company.Name)
	_, err = smtpRepo.Upsert(ctx, adminScope, smtpSettingsFixture(companyID), f.smtpPassword)
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
	// Both tenants seed the SAME task id on purpose: a deterministic billing
	// task id is identical for two companies billing the same period, so
	// RunByTaskID must answer from the caller's company and no other (R237).
	scopeIsoTaskID := "billing.generate:company:scope-iso:2026-08"
	run, err := opsRepo.StartRun(ctx, adminScope, model.JobRun{CompanyID: &companyID, JobType: "scope-iso-job",
		TaskID: &scopeIsoTaskID})
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
	t.Parallel()
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

	// Important Finding 2: consumption_monthly, consumption_yearly and
	// plant_production_monthly are materialized_only = true — refreshed ONCE
	// here, after both af and bf are seeded, exactly as
	// analyticsRefresh (analytics_integration_test.go, same package) does
	// for its own tests: outside a transaction, via pool.Exec's implicit
	// autocommit. consumption_monthly's January-2026 bucket (scopeIsoEpoch)
	// is already closed relative to this phase's clock; consumption_yearly
	// needs scopeIsoClosedYearEpoch's rows instead (see that var's own doc
	// comment) since 2026 itself is still the current year.
	analyticsRefresh(t, ctx, pool, "consumption_monthly")
	analyticsRefresh(t, ctx, pool, "consumption_yearly")
	analyticsRefresh(t, ctx, pool, "plant_production_monthly")

	narrowRange := store.TimeRange{From: scopeIsoEpoch, To: scopeIsoEpoch.Add(2 * time.Hour)}
	invalidScope := store.Scope{}

	// --- CompanyRepository (company-only) -----------------------------------
	t.Run("CompanyRepository", func(t *testing.T) {
		repo := postgres.NewCompanyRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Company, error) { return repo.Get(ctx, s, tenantB.Company.ID) },
			tenantA.AdminScope, tenantB.AdminScope, "CompanyRepository.Get cross-tenant")

		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Company, error) { return repo.List(ctx, s, store.CompanyFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, tenantB.Company.ID,
			func(c model.Company) uuid.UUID { return c.ID }, "CompanyRepository.List cross-tenant")
	})

	// --- UserRepository (company-only) ---------------------------------------
	t.Run("UserRepository", func(t *testing.T) {
		repo := postgres.NewUserRepository(pool)
		theirUser := tenantB.Users[model.UserRoleCompanyAdmin]

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.User, error) { return repo.Get(ctx, s, theirUser.ID) },
			tenantA.AdminScope, tenantB.AdminScope, "UserRepository.Get cross-tenant")

		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.User, error) { return repo.List(ctx, s, store.UserFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, theirUser.ID,
			func(u model.User) uuid.UUID { return u.ID }, "UserRepository.List cross-tenant")

		hist, err := repo.PasswordHistory(ctx, tenantA.AdminScope, theirUser.ID, 10)
		require.ErrorIs(t, err, store.ErrNotFound)
		require.Nil(t, hist)
		ownHist, err := repo.PasswordHistory(ctx, tenantB.AdminScope, theirUser.ID, 10)
		require.NoError(t, err, "positive control: tenant B's own AdminScope must read its own user's password history")
		require.NotEmpty(t, ownHist, "this assertion is vacuous if tenant B's own SetPassword seed never ran")
	})

	// --- SessionRepository (company-only, joins through users) ---------------
	t.Run("SessionRepository", func(t *testing.T) {
		repo := postgres.NewSessionRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Session, error) { return repo.Get(ctx, s, bf.sessionID) },
			tenantA.AdminScope, tenantB.AdminScope, "SessionRepository.Get cross-tenant")

		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Session, error) { return repo.List(ctx, s, store.SessionFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, bf.sessionID,
			func(s model.Session) uuid.UUID { return s.ID }, "SessionRepository.List cross-tenant")
	})

	// --- AuditRepository (company-only) --------------------------------------
	t.Run("AuditRepository", func(t *testing.T) {
		repo := postgres.NewAuditRepository(pool)

		list, err := repo.List(ctx, tenantA.AdminScope, store.AuditFilter{})
		require.NoError(t, err)
		sawOwn := false
		for _, e := range list {
			// Folded Minor 2: CompanyID is nullable (audit_log has no FKs,
			// per wave-f-context.md ruling 10) — a platform-level entry with
			// a nil CompanyID must not panic this assertion.
			if e.CompanyID == nil {
				continue
			}
			require.NotEqual(t, tenantB.Company.ID, *e.CompanyID, "tenant B's audit rows must never appear in tenant A's list")
			if *e.CompanyID == tenantA.Company.ID {
				sawOwn = true
			}
		}
		require.True(t, sawOwn, "positive control: tenant A's own AdminScope must see at least one of its own audit rows")
	})

	// --- BuildingRepository (building-scoped) ---------------------------------
	t.Run("BuildingRepository", func(t *testing.T) {
		repo := postgres.NewBuildingRepository(pool)

		// cross-tenant
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Building, error) { return repo.Get(ctx, s, tenantB.Buildings[0].ID) },
			tenantA.AdminScope, tenantB.AdminScope, "BuildingRepository.Get cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Building, error) { return repo.List(ctx, s, store.BuildingFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, tenantB.Buildings[0].ID,
			func(b model.Building) uuid.UUID { return b.ID }, "BuildingRepository.List cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.BuildingContact, error) {
				return repo.Contacts(ctx, s, tenantB.Buildings[0].ID)
			},
			tenantA.AdminScope, tenantB.AdminScope, "BuildingRepository.Contacts cross-tenant")

		// narrow-scope: tenant A's own Buildings[1], outside tenant A's Scope
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Building, error) { return repo.Get(ctx, s, tenantA.Buildings[1].ID) },
			tenantA.Scope, tenantA.AdminScope, "BuildingRepository.Get narrow-scope")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Building, error) { return repo.List(ctx, s, store.BuildingFilter{}) },
			tenantA.Scope, tenantA.AdminScope, tenantA.Buildings[1].ID,
			func(b model.Building) uuid.UUID { return b.ID }, "BuildingRepository.List narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.BuildingContact, error) {
				return repo.Contacts(ctx, s, tenantA.Buildings[1].ID)
			},
			tenantA.Scope, tenantA.AdminScope, "BuildingRepository.Contacts narrow-scope")

		_, err := repo.Get(ctx, invalidScope, tenantA.Buildings[0].ID)
		require.ErrorIs(t, err, store.ErrInvalidScope)
	})

	// --- AnalyzerRepository (building-scoped) ---------------------------------
	t.Run("AnalyzerRepository", func(t *testing.T) {
		repo := postgres.NewAnalyzerRepository(pool)
		theirAnalyzer := tenantB.Analyzers[0]

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Analyzer, error) { return repo.Get(ctx, s, theirAnalyzer.ID) },
			tenantA.AdminScope, tenantB.AdminScope, "AnalyzerRepository.Get cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Analyzer, error) {
				return repo.GetByInstallation(ctx, s, theirAnalyzer.Provider, theirAnalyzer.ProviderSubtype, theirAnalyzer.InstallationNumber)
			},
			tenantA.AdminScope, tenantB.AdminScope, "AnalyzerRepository.GetByInstallation cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Analyzer, error) { return repo.List(ctx, s, store.AnalyzerFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, theirAnalyzer.ID,
			func(a model.Analyzer) uuid.UUID { return a.ID }, "AnalyzerRepository.List cross-tenant")

		outsideGrant := tenantA.Analyzers[2] // Buildings[1]
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Analyzer, error) { return repo.Get(ctx, s, outsideGrant.ID) },
			tenantA.Scope, tenantA.AdminScope, "AnalyzerRepository.Get narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Analyzer, error) {
				return repo.GetByInstallation(ctx, s, outsideGrant.Provider, outsideGrant.ProviderSubtype, outsideGrant.InstallationNumber)
			},
			tenantA.Scope, tenantA.AdminScope, "AnalyzerRepository.GetByInstallation narrow-scope")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Analyzer, error) { return repo.List(ctx, s, store.AnalyzerFilter{}) },
			tenantA.Scope, tenantA.AdminScope, outsideGrant.ID,
			func(a model.Analyzer) uuid.UUID { return a.ID }, "AnalyzerRepository.List narrow-scope")

		// Important Finding 4 / HANDOFF ruling 7: an unassigned analyzer
		// (building_id nil) is visible only to AllBuildings.
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Analyzer, error) { return repo.Get(ctx, s, bf.unassignedAnalyzerID) },
			tenantA.AdminScope, tenantB.AdminScope, "AnalyzerRepository.Get cross-tenant unassigned analyzer")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Analyzer, error) { return repo.Get(ctx, s, af.unassignedAnalyzerID) },
			tenantA.Scope, tenantA.AdminScope, "AnalyzerRepository.Get narrow-scope unassigned analyzer")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Analyzer, error) { return repo.List(ctx, s, store.AnalyzerFilter{}) },
			tenantA.Scope, tenantA.AdminScope, af.unassignedAnalyzerID,
			func(a model.Analyzer) uuid.UUID { return a.ID }, "AnalyzerRepository.List narrow-scope unassigned analyzer")
	})

	// --- PlantRepository (company-only: power_plants has no building_id) -----
	t.Run("PlantRepository", func(t *testing.T) {
		repo := postgres.NewPlantRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.PowerPlant, error) { return repo.Get(ctx, s, bf.plantID) },
			tenantA.AdminScope, tenantB.AdminScope, "PlantRepository.Get cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.PowerPlant, error) { return repo.List(ctx, s, store.PlantFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, bf.plantID,
			func(p model.PowerPlant) uuid.UUID { return p.ID }, "PlantRepository.List cross-tenant")

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.PlantMonthlyTarget, error) {
				return repo.MonthlyTargets(ctx, s, bf.plantID)
			},
			tenantA.AdminScope, tenantB.AdminScope, "PlantRepository.MonthlyTargets cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.PlantDevice, error) { return repo.Devices(ctx, s, bf.plantID) },
			tenantA.AdminScope, tenantB.AdminScope, "PlantRepository.Devices cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.PlantAlarmRecipient, error) {
				return repo.AlarmRecipients(ctx, s, bf.plantID)
			},
			tenantA.AdminScope, tenantB.AdminScope, "PlantRepository.AlarmRecipients cross-tenant")
	})

	// --- ReadingRepository (building-scoped via analyzers) --------------------
	t.Run("ReadingRepository", func(t *testing.T) {
		repo := postgres.NewReadingRepository(pool)

		_, err := repo.Range(ctx, tenantA.AdminScope, bf.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownRange, err := repo.Range(ctx, tenantB.AdminScope, bf.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.NoError(t, err)
		require.NotEmpty(t, ownRange, "positive control")

		_, _, err = repo.BoundaryReadings(ctx, tenantA.AdminScope, bf.analyzerID, model.ReadingKindLoadProfile, bf.readingsFrom, bf.readingsTo)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownStart, ownEnd, err := repo.BoundaryReadings(ctx, tenantB.AdminScope, bf.analyzerID, model.ReadingKindLoadProfile, bf.readingsFrom, bf.readingsTo)
		require.NoError(t, err)
		require.NotNil(t, ownStart)
		require.NotNil(t, ownEnd)

		_, err = repo.Latest(ctx, tenantA.AdminScope, bf.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownLatest, err := repo.Latest(ctx, tenantB.AdminScope, bf.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.NoError(t, err)
		require.NotNil(t, ownLatest)

		_, err = repo.Range(ctx, tenantA.Scope, af.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownNarrowRange, err := repo.Range(ctx, tenantA.AdminScope, af.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowRange, "positive control")

		_, _, err = repo.BoundaryReadings(ctx, tenantA.Scope, af.analyzerID, model.ReadingKindLoadProfile, af.readingsFrom, af.readingsTo)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownNarrowStart, ownNarrowEnd, err := repo.BoundaryReadings(ctx, tenantA.AdminScope, af.analyzerID, model.ReadingKindLoadProfile, af.readingsFrom, af.readingsTo)
		require.NoError(t, err)
		require.NotNil(t, ownNarrowStart)
		require.NotNil(t, ownNarrowEnd)

		_, err = repo.Latest(ctx, tenantA.Scope, af.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownNarrowLatest, err := repo.Latest(ctx, tenantA.AdminScope, af.analyzerID, narrowRange, model.ReadingKindLoadProfile)
		require.NoError(t, err)
		require.NotNil(t, ownNarrowLatest)
	})

	// --- CursorRepository (building-scoped via analyzers) ----------------------
	t.Run("CursorRepository", func(t *testing.T) {
		repo := postgres.NewCursorRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.analyzerID, bf.cursorKind)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Get(ctx, tenantB.AdminScope, bf.analyzerID, bf.cursorKind)
		require.NoError(t, err, "positive control")

		crossList, err := repo.List(ctx, tenantA.AdminScope, []uuid.UUID{bf.analyzerID})
		require.NoError(t, err)
		require.Empty(t, crossList, "an analyzer id not visible to the Scope contributes no rows to List")
		ownCrossList, err := repo.List(ctx, tenantB.AdminScope, []uuid.UUID{bf.analyzerID})
		require.NoError(t, err)
		require.NotEmpty(t, ownCrossList, "positive control")

		_, err = repo.Get(ctx, tenantA.Scope, af.analyzerID, af.cursorKind)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Get(ctx, tenantA.AdminScope, af.analyzerID, af.cursorKind)
		require.NoError(t, err, "positive control")

		narrowList, err := repo.List(ctx, tenantA.Scope, []uuid.UUID{af.analyzerID})
		require.NoError(t, err)
		require.Empty(t, narrowList)
		ownNarrowList, err := repo.List(ctx, tenantA.AdminScope, []uuid.UUID{af.analyzerID})
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowList, "positive control")
	})

	// --- GenerationRepository (analyzer-parented, no company_id; F2) -------
	t.Run("GenerationRepository", func(t *testing.T) {
		repo := postgres.NewGenerationRepository(pool)

		_, err := repo.Anchor(ctx, tenantA.AdminScope, bf.analyzerID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Anchor(ctx, tenantB.AdminScope, bf.analyzerID)
		require.NoError(t, err, "positive control")

		_, err = repo.Anchor(ctx, tenantA.Scope, af.analyzerID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Anchor(ctx, tenantA.AdminScope, af.analyzerID)
		require.NoError(t, err, "positive control")
	})

	// --- ProviderSeriesRepository (analyzer-parented, no company_id; F2) ---
	t.Run("ProviderSeriesRepository", func(t *testing.T) {
		repo := postgres.NewProviderSeriesRepository(pool)

		_, err := repo.HourlyRange(ctx, tenantA.AdminScope, bf.analyzerID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownCrossRange, err := repo.HourlyRange(ctx, tenantB.AdminScope, bf.analyzerID, narrowRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownCrossRange, "positive control")

		_, err = repo.HourlyRange(ctx, tenantA.Scope, af.analyzerID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownNarrowRange, err := repo.HourlyRange(ctx, tenantA.AdminScope, af.analyzerID, narrowRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowRange, "positive control")
	})

	// --- AnomalyRepository (building-scoped via analyzers) ----------------------
	t.Run("AnomalyRepository", func(t *testing.T) {
		repo := postgres.NewAnomalyRepository(pool)

		_, err := repo.Get(ctx, tenantA.AdminScope, bf.anomalyID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Get(ctx, tenantB.AdminScope, bf.anomalyID)
		require.NoError(t, err, "positive control")

		crossList, err := repo.List(ctx, tenantA.AdminScope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{bf.analyzerID}})
		require.NoError(t, err)
		require.Empty(t, crossList)
		ownCrossList, err := repo.List(ctx, tenantB.AdminScope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{bf.analyzerID}})
		require.NoError(t, err)
		require.NotEmpty(t, ownCrossList, "positive control")

		_, err = repo.Get(ctx, tenantA.Scope, af.anomalyID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Get(ctx, tenantA.AdminScope, af.anomalyID)
		require.NoError(t, err, "positive control")

		narrowList, err := repo.List(ctx, tenantA.Scope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{af.analyzerID}})
		require.NoError(t, err)
		require.Empty(t, narrowList)
		ownNarrowList, err := repo.List(ctx, tenantA.AdminScope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{af.analyzerID}})
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowList, "positive control")
	})

	// --- ProductionRepository (company-only via power_plants) -------------------
	t.Run("ProductionRepository", func(t *testing.T) {
		repo := postgres.NewProductionRepository(pool)

		_, err := repo.Range(ctx, tenantA.AdminScope, bf.plantID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownRange, err := repo.Range(ctx, tenantB.AdminScope, bf.plantID, narrowRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownRange, "positive control")

		_, err = repo.Latest(ctx, tenantA.AdminScope, bf.plantID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownLatest, err := repo.Latest(ctx, tenantB.AdminScope, bf.plantID, narrowRange)
		require.NoError(t, err)
		require.NotNil(t, ownLatest)
	})

	// --- F9 solar repositories (company-only via power_plants) -----------------
	t.Run("ProductionTotalsRepository", func(t *testing.T) {
		repo := postgres.NewProductionTotalsRepository(pool)
		_, err := repo.Range(ctx, tenantA.AdminScope, bf.plantID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		own, err := repo.Range(ctx, tenantB.AdminScope, bf.plantID, narrowRange)
		require.NoError(t, err)
		require.NotEmpty(t, own, "positive control")
		_, err = repo.FirstDay(ctx, tenantA.AdminScope, bf.plantID)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.ReplaceDays(ctx, tenantA.AdminScope, bf.plantID, []time.Time{scopeIsoEpoch}, nil)
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = postgres.NewProductionRepository(pool).ReplaceDeviceDays(ctx, tenantA.AdminScope, bf.plantID, []time.Time{scopeIsoEpoch}, nil)
		require.ErrorIs(t, err, store.ErrNotFound)
		own, err = repo.Range(ctx, tenantB.AdminScope, bf.plantID, narrowRange)
		require.NoError(t, err)
		require.NotEmpty(t, own, "a refused replace deleted nothing")
	})

	t.Run("FaultRepository", func(t *testing.T) {
		repo := postgres.NewFaultRepository(pool)
		_, _, err := repo.List(ctx, tenantA.AdminScope, bf.plantID, store.Page{Limit: 10})
		require.ErrorIs(t, err, store.ErrNotFound)
		own, _, err := repo.List(ctx, tenantB.AdminScope, bf.plantID, store.Page{Limit: 10})
		require.NoError(t, err)
		require.NotEmpty(t, own, "positive control")
		_, err = repo.Unforwarded(ctx, tenantA.AdminScope, bf.plantID, scopeIsoEpoch.AddDate(-1, 0, 0))
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Claim(ctx, tenantA.AdminScope, bf.plantID, []string{"scope-iso-fault"})
		require.ErrorIs(t, err, store.ErrNotFound)
		_, err = repo.Upsert(ctx, tenantA.AdminScope, []model.PlantFault{{PlantID: bf.plantID, Ref: "x", Code: "1", Name: "x", OccurredAt: scopeIsoEpoch}})
		require.ErrorIs(t, err, store.ErrNotFound)
		claimed, err := repo.Claim(ctx, tenantB.AdminScope, bf.plantID, []string{"iso-claim"})
		require.NoError(t, err)
		require.Equal(t, []string{"iso-claim"}, claimed)
		require.NoError(t, repo.Release(ctx, tenantA.AdminScope, bf.plantID, []string{"iso-claim"}))
		again, err := repo.Claim(ctx, tenantB.AdminScope, bf.plantID, []string{"iso-claim"})
		require.NoError(t, err)
		require.Empty(t, again, "still claimed: another company's release deleted nothing")
		_, err = repo.Prune(ctx, tenantA.AdminScope, scopeIsoEpoch.AddDate(10, 0, 0))
		require.NoError(t, err)
		own, _, err = repo.List(ctx, tenantB.AdminScope, bf.plantID, store.Page{Limit: 10})
		require.NoError(t, err)
		require.NotEmpty(t, own, "another company's prune deleted nothing")
	})

	t.Run("PlantRepositorySolar", func(t *testing.T) {
		repo := postgres.NewPlantRepository(pool)
		now := scopeIsoEpoch
		require.ErrorIs(t, repo.UpdateDeviceSnapshot(ctx, tenantA.AdminScope, model.PlantDevice{ID: bf.deviceID, PlantID: bf.plantID, SnapshotAt: &now}), store.ErrNotFound)
		require.ErrorIs(t, repo.SetSyncState(ctx, tenantA.AdminScope, bf.plantID, now, nil), store.ErrNotFound)
		_, err := repo.SetIsolarLink(ctx, tenantA.AdminScope, model.PowerPlant{ID: bf.plantID, CompanyID: tenantA.Company.ID})
		require.ErrorIs(t, err, store.ErrNotFound)
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.PowerPlant, error) { return repo.ListLinked(ctx, s) },
			tenantA.AdminScope, tenantB.AdminScope, bf.plantID,
			func(p model.PowerPlant) uuid.UUID { return p.ID }, "PlantRepository.ListLinked cross-tenant")
	})

	// --- AnalyticsRepository (Consumption* building-scoped, Production*
	// company-only — see repository.go's own doc comment) ------------------------
	t.Run("AnalyticsRepository", func(t *testing.T) {
		repo := postgres.NewAnalyticsRepository(pool)

		// Important Finding 2: daily/monthly/yearly buckets are
		// Europe/Istanbul-LOCAL instants (migration 00005) — narrowRange's
		// [00:00,02:00) UTC window never overlaps the actual bucket instant
		// (which can start the PREVIOUS UTC day), so every window below is
		// widened by a day on each side instead, exactly as
		// analytics_integration_test.go's own tests do. Only hourly has no
		// timezone dependence and can keep narrowRange.
		dailyRange := store.TimeRange{From: scopeIsoEpoch.Add(-24 * time.Hour), To: scopeIsoEpoch.Add(48 * time.Hour)}
		monthRange := store.TimeRange{From: scopeIsoEpoch.Add(-24 * time.Hour), To: scopeIsoEpoch.AddDate(0, 1, 0).Add(24 * time.Hour)}
		yearRange := store.TimeRange{From: scopeIsoClosedYearEpoch.Add(-24 * time.Hour), To: scopeIsoClosedYearEpoch.AddDate(1, 0, 0).Add(24 * time.Hour)}

		crossHourly, err := repo.ConsumptionHourly(ctx, tenantA.AdminScope, []uuid.UUID{bf.analyzerID}, narrowRange)
		require.NoError(t, err)
		require.Empty(t, crossHourly)
		ownHourly, err := repo.ConsumptionHourly(ctx, tenantB.AdminScope, []uuid.UUID{bf.analyzerID}, narrowRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownHourly, "positive control: tenant B's own AdminScope must see its own hourly bucket")

		crossDaily, err := repo.ConsumptionDaily(ctx, tenantA.AdminScope, []uuid.UUID{bf.analyzerID}, dailyRange)
		require.NoError(t, err)
		require.Empty(t, crossDaily)
		ownDaily, err := repo.ConsumptionDaily(ctx, tenantB.AdminScope, []uuid.UUID{bf.analyzerID}, dailyRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownDaily, "positive control: tenant B's own AdminScope must see its own daily bucket")

		crossMonthly, err := repo.ConsumptionMonthly(ctx, tenantA.AdminScope, []uuid.UUID{bf.analyzerID}, monthRange)
		require.NoError(t, err)
		require.Empty(t, crossMonthly)
		ownMonthly, err := repo.ConsumptionMonthly(ctx, tenantB.AdminScope, []uuid.UUID{bf.analyzerID}, monthRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownMonthly, "positive control: tenant B's own AdminScope must see its own monthly bucket")

		crossYearly, err := repo.ConsumptionYearly(ctx, tenantA.AdminScope, []uuid.UUID{bf.analyzerID}, yearRange)
		require.NoError(t, err)
		require.Empty(t, crossYearly)
		ownYearly, err := repo.ConsumptionYearly(ctx, tenantB.AdminScope, []uuid.UUID{bf.analyzerID}, yearRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownYearly, "positive control: tenant B's own AdminScope must see its own yearly bucket")

		crossProdDaily, err := repo.ProductionDaily(ctx, tenantA.AdminScope, []uuid.UUID{bf.plantID}, dailyRange)
		require.NoError(t, err)
		require.Empty(t, crossProdDaily)
		ownProdDaily, err := repo.ProductionDaily(ctx, tenantB.AdminScope, []uuid.UUID{bf.plantID}, dailyRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownProdDaily, "positive control: tenant B's own AdminScope must see its own production daily bucket")

		crossProdMonthly, err := repo.ProductionMonthly(ctx, tenantA.AdminScope, []uuid.UUID{bf.plantID}, monthRange)
		require.NoError(t, err)
		require.Empty(t, crossProdMonthly)
		ownProdMonthly, err := repo.ProductionMonthly(ctx, tenantB.AdminScope, []uuid.UUID{bf.plantID}, monthRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownProdMonthly, "positive control: tenant B's own AdminScope must see its own production monthly bucket")

		narrowHourly, err := repo.ConsumptionHourly(ctx, tenantA.Scope, []uuid.UUID{af.analyzerID}, narrowRange)
		require.NoError(t, err)
		require.Empty(t, narrowHourly, "Buildings[1]'s analyzer must be invisible to the narrow Scope")
		ownNarrowHourly, err := repo.ConsumptionHourly(ctx, tenantA.AdminScope, []uuid.UUID{af.analyzerID}, narrowRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowHourly, "positive control: tenant A's own AdminScope must see Buildings[1]'s hourly bucket")

		narrowDaily, err := repo.ConsumptionDaily(ctx, tenantA.Scope, []uuid.UUID{af.analyzerID}, dailyRange)
		require.NoError(t, err)
		require.Empty(t, narrowDaily)
		ownNarrowDaily, err := repo.ConsumptionDaily(ctx, tenantA.AdminScope, []uuid.UUID{af.analyzerID}, dailyRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowDaily, "positive control: tenant A's own AdminScope must see Buildings[1]'s daily bucket")

		// Important Finding 2: ConsumptionMonthly/Yearly had NO narrow-scope
		// case at all before this fix.
		narrowMonthly, err := repo.ConsumptionMonthly(ctx, tenantA.Scope, []uuid.UUID{af.analyzerID}, monthRange)
		require.NoError(t, err)
		require.Empty(t, narrowMonthly, "Buildings[1]'s analyzer must be invisible to the narrow Scope")
		ownNarrowMonthly, err := repo.ConsumptionMonthly(ctx, tenantA.AdminScope, []uuid.UUID{af.analyzerID}, monthRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowMonthly, "positive control: tenant A's own AdminScope must see Buildings[1]'s monthly bucket")

		narrowYearly, err := repo.ConsumptionYearly(ctx, tenantA.Scope, []uuid.UUID{af.analyzerID}, yearRange)
		require.NoError(t, err)
		require.Empty(t, narrowYearly, "Buildings[1]'s analyzer must be invisible to the narrow Scope")
		ownNarrowYearly, err := repo.ConsumptionYearly(ctx, tenantA.AdminScope, []uuid.UUID{af.analyzerID}, yearRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowYearly, "positive control: tenant A's own AdminScope must see Buildings[1]'s yearly bucket")
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

		// Folded Minor 4: Yekdem itself was only ever exercised with an
		// invalid Scope before this fix — never proven to return the SAME
		// row under either tenant's Scope.
		yekA, err := repo.Yekdem(ctx, tenantA.Scope, 2026, 1)
		require.NoError(t, err)
		yekB, err := repo.Yekdem(ctx, tenantB.Scope, 2026, 1)
		require.NoError(t, err)
		require.True(t, yekA.Value.Equal(yekB.Value), "a platform table must return the identical row under either tenant's Scope")
		require.True(t, yekA.FetchedAt.Equal(yekB.FetchedAt))
	})

	// --- BillingParameterRepository (platform-wide: Scope narrows nothing) ------
	t.Run("BillingParameterRepository", func(t *testing.T) {
		repo := postgres.NewBillingParameterRepository(pool)
		_, err := repo.Effective(ctx, invalidScope, scopeIsoEpoch)
		require.ErrorIs(t, err, store.ErrInvalidScope)
		_, err = repo.List(ctx, invalidScope)
		require.ErrorIs(t, err, store.ErrInvalidScope)

		pA, err := repo.Effective(ctx, tenantA.Scope, scopeIsoEpoch)
		require.NoError(t, err)
		pB, err := repo.Effective(ctx, tenantB.Scope, scopeIsoEpoch)
		require.NoError(t, err)
		require.True(t, pA.EffectiveFrom.Equal(pB.EffectiveFrom), "a platform table must return the identical row under either tenant's Scope")
		listA, err := repo.List(ctx, tenantA.Scope)
		require.NoError(t, err)
		listB, err := repo.List(ctx, tenantB.Scope)
		require.NoError(t, err)
		require.NotEmpty(t, listA)
		require.Equal(t, len(listA), len(listB))
	})

	// --- ForecastRepository (building-scoped via analyzers) -----------------------
	t.Run("ForecastRepository", func(t *testing.T) {
		repo := postgres.NewForecastRepository(pool)

		_, err := repo.Range(ctx, tenantA.AdminScope, bf.analyzerID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownRange, err := repo.Range(ctx, tenantB.AdminScope, bf.analyzerID, narrowRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownRange, "positive control")

		_, err = repo.LatestRun(ctx, tenantA.AdminScope, bf.analyzerID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownLatestRun, err := repo.LatestRun(ctx, tenantB.AdminScope, bf.analyzerID, narrowRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownLatestRun, "positive control")

		_, err = repo.Gaps(ctx, tenantA.AdminScope, bf.analyzerID, bf.forecastGeneratedAt)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownGaps, err := repo.Gaps(ctx, tenantB.AdminScope, bf.analyzerID, bf.forecastGeneratedAt)
		require.NoError(t, err)
		require.NotEmpty(t, ownGaps, "positive control")

		_, err = repo.Range(ctx, tenantA.Scope, af.analyzerID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownNarrowRange, err := repo.Range(ctx, tenantA.AdminScope, af.analyzerID, narrowRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowRange, "positive control")

		_, err = repo.LatestRun(ctx, tenantA.Scope, af.analyzerID, narrowRange)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownNarrowLatestRun, err := repo.LatestRun(ctx, tenantA.AdminScope, af.analyzerID, narrowRange)
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowLatestRun, "positive control")

		_, err = repo.Gaps(ctx, tenantA.Scope, af.analyzerID, af.forecastGeneratedAt)
		require.ErrorIs(t, err, store.ErrNotFound)
		ownNarrowGaps, err := repo.Gaps(ctx, tenantA.AdminScope, af.analyzerID, af.forecastGeneratedAt)
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowGaps, "positive control")
	})

	// --- TariffRepository (building-scoped) ----------------------------------------
	t.Run("TariffRepository", func(t *testing.T) {
		repo := postgres.NewTariffRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Tariff, error) { return repo.Get(ctx, s, bf.tariffID) },
			tenantA.AdminScope, tenantB.AdminScope, "TariffRepository.Get cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Tariff, error) { return repo.List(ctx, s, store.TariffFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, bf.tariffID,
			func(tar model.Tariff) uuid.UUID { return tar.ID }, "TariffRepository.List cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Tariff, error) {
				return repo.Effective(ctx, s, bf.buildingID, scopeIsoEpoch.Add(time.Hour))
			},
			tenantA.AdminScope, tenantB.AdminScope, "TariffRepository.Effective cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.TariffTax, error) { return repo.Taxes(ctx, s, bf.tariffID) },
			tenantA.AdminScope, tenantB.AdminScope, "TariffRepository.Taxes cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.TariffManualYekdem, error) { return repo.ManualYekdem(ctx, s, bf.tariffID) },
			tenantA.AdminScope, tenantB.AdminScope, "TariffRepository.ManualYekdem cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.TariffExtraCharge, error) { return repo.ExtraCharges(ctx, s, bf.tariffID) },
			tenantA.AdminScope, tenantB.AdminScope, "TariffRepository.ExtraCharges cross-tenant")
		// Important Finding 4: tenant B's company-wide tariff must be
		// invisible even to tenant A's AdminScope — company_id, not the
		// building branch, is what protects it here.
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Tariff, error) { return repo.Get(ctx, s, bf.companyWideTariffID) },
			tenantA.AdminScope, tenantB.AdminScope, "TariffRepository.Get cross-tenant company-wide")

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Tariff, error) { return repo.Get(ctx, s, af.tariffID) },
			tenantA.Scope, tenantA.AdminScope, "TariffRepository.Get narrow-scope")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Tariff, error) { return repo.List(ctx, s, store.TariffFilter{}) },
			tenantA.Scope, tenantA.AdminScope, af.tariffID,
			func(tar model.Tariff) uuid.UUID { return tar.ID }, "TariffRepository.List narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Tariff, error) {
				return repo.Effective(ctx, s, af.buildingID, scopeIsoEpoch.Add(time.Hour))
			},
			tenantA.Scope, tenantA.AdminScope, "TariffRepository.Effective narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.TariffTax, error) { return repo.Taxes(ctx, s, af.tariffID) },
			tenantA.Scope, tenantA.AdminScope, "TariffRepository.Taxes narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.TariffManualYekdem, error) { return repo.ManualYekdem(ctx, s, af.tariffID) },
			tenantA.Scope, tenantA.AdminScope, "TariffRepository.ManualYekdem narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.TariffExtraCharge, error) { return repo.ExtraCharges(ctx, s, af.tariffID) },
			tenantA.Scope, tenantA.AdminScope, "TariffRepository.ExtraCharges narrow-scope")

		// Important Finding 4 / HANDOFF ruling 7: a company-wide tariff
		// (building_id nil) is visible only to AllBuildings; Effective still
		// resolves it for a building the narrow Scope DOES grant, when no
		// building-specific tariff is effective yet — the documented
		// exception (see scopeIsoCompanyWideTariffFrom/On's own doc comment).
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Tariff, error) { return repo.Get(ctx, s, af.companyWideTariffID) },
			tenantA.Scope, tenantA.AdminScope, "TariffRepository.Get narrow-scope company-wide")
		narrowListExcludesCompanyWide, err := repo.List(ctx, tenantA.Scope, store.TariffFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, narrowListExcludesCompanyWide, af.companyWideTariffID, func(tar model.Tariff) uuid.UUID { return tar.ID })

		resolved, err := repo.Effective(ctx, tenantA.Scope, tenantA.Buildings[0].ID, scopeIsoCompanyWideTariffOn)
		require.NoError(t, err, "Effective must resolve the company-wide tariff for a building the narrow Scope DOES grant")
		require.Equal(t, af.companyWideTariffID, resolved.ID)
	})

	// --- TariffTemplateRepository (company-only) ------------------------------------
	t.Run("TariffTemplateRepository", func(t *testing.T) {
		repo := postgres.NewTariffTemplateRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.TariffTemplate, error) { return repo.Get(ctx, s, bf.tariffTemplateID) },
			tenantA.AdminScope, tenantB.AdminScope, "TariffTemplateRepository.Get cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.TariffTemplate, error) {
				return repo.List(ctx, s, store.TariffTemplateFilter{})
			},
			tenantA.AdminScope, tenantB.AdminScope, bf.tariffTemplateID,
			func(tt model.TariffTemplate) uuid.UUID { return tt.ID }, "TariffTemplateRepository.List cross-tenant")
	})

	// --- BulkAssignmentRepository (company-only, R241's history) ---------------------
	t.Run("BulkAssignmentRepository", func(t *testing.T) {
		repo := postgres.NewBulkAssignmentRepository(pool)

		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.TariffBulkAssignment, error) {
				return repo.List(ctx, s, store.Page{Limit: 100})
			},
			tenantA.AdminScope, tenantB.AdminScope, bf.bulkAssignmentID,
			func(a model.TariffBulkAssignment) uuid.UUID { return a.ID }, "BulkAssignmentRepository.List cross-tenant")

		_, err := repo.List(ctx, invalidScope, store.Page{Limit: 100})
		require.ErrorIs(t, err, store.ErrInvalidScope)
	})

	// --- SolarTariffRepository (company-only, prices a plant) ------------------------
	t.Run("SolarTariffRepository", func(t *testing.T) {
		repo := postgres.NewSolarTariffRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.SolarTariff, error) { return repo.Get(ctx, s, bf.solarTariffID) },
			tenantA.AdminScope, tenantB.AdminScope, "SolarTariffRepository.Get cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.SolarTariff, error) { return repo.List(ctx, s, store.SolarTariffFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, bf.solarTariffID,
			func(st model.SolarTariff) uuid.UUID { return st.ID }, "SolarTariffRepository.List cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.SolarTariff, error) {
				return repo.Effective(ctx, s, bf.plantID, scopeIsoEpoch.Add(time.Hour))
			},
			tenantA.AdminScope, tenantB.AdminScope, "SolarTariffRepository.Effective cross-tenant")
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

		// Important Finding 3: Effective was never exercised at all before
		// this fix — platform-wide (no company_id), so isolation is
		// invalid-scope rejection plus identical resolution under either
		// tenant's Scope.
		_, err = repo.Effective(ctx, invalidScope, model.UserGroupCommercial, model.VoltageLevelLV, model.TariffTermMonomial, scopeIsoEpoch)
		require.ErrorIs(t, err, store.ErrInvalidScope)

		effA, err := repo.Effective(ctx, tenantA.Scope, model.UserGroupCommercial, model.VoltageLevelLV, model.TariffTermMonomial, scopeIsoEpoch)
		require.NoError(t, err)
		effB, err := repo.Effective(ctx, tenantB.AdminScope, model.UserGroupCommercial, model.VoltageLevelLV, model.TariffTermMonomial, scopeIsoEpoch)
		require.NoError(t, err)
		require.Equal(t, effA.ID, effB.ID, "both tenants must resolve the identical national tariff row")
	})

	// --- IcmalRepository (company-only import, building-tagged rows) -------------------
	t.Run("IcmalRepository", func(t *testing.T) {
		repo := postgres.NewIcmalRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.IcmalImport, error) { return repo.GetImport(ctx, s, bf.icmalImportID) },
			tenantA.AdminScope, tenantB.AdminScope, "IcmalRepository.GetImport cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.IcmalImport, error) { return repo.ListImports(ctx, s, store.IcmalFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, bf.icmalImportID,
			func(i model.IcmalImport) uuid.UUID { return i.ID }, "IcmalRepository.ListImports cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.IcmalRow, error) {
				return repo.ListRows(ctx, s, bf.icmalImportID, store.Page{})
			},
			tenantA.AdminScope, tenantB.AdminScope, "IcmalRepository.ListRows cross-tenant")
	})

	// --- BillRepository (building-scoped, with several no-company_id children) --------
	t.Run("BillRepository", func(t *testing.T) {
		repo := postgres.NewBillRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Bill, error) { return repo.Get(ctx, s, bf.billID) },
			tenantA.AdminScope, tenantB.AdminScope, "BillRepository.Get cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Bill, error) { return repo.List(ctx, s, store.BillFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, bf.billID,
			func(b model.Bill) uuid.UUID { return b.ID }, "BillRepository.List cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Bill, error) {
				return repo.Current(ctx, s, model.BillScopeBuilding, bf.buildingID, "2026-01")
			},
			tenantA.AdminScope, tenantB.AdminScope, "BillRepository.Current cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.BillLine, error) { return repo.Lines(ctx, s, bf.billID) },
			tenantA.AdminScope, tenantB.AdminScope, "BillRepository.Lines cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.BillMember, error) { return repo.Members(ctx, s, bf.billID) },
			tenantA.AdminScope, tenantB.AdminScope, "BillRepository.Members cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.BillHourlyDetail, error) { return repo.HourlyDetail(ctx, s, bf.billID) },
			tenantA.AdminScope, tenantB.AdminScope, "BillRepository.HourlyDetail cross-tenant")
		// Important Finding 4: tenant B's company-level bill must be
		// invisible even to tenant A's AdminScope.
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Bill, error) { return repo.Get(ctx, s, bf.companyBillID) },
			tenantA.AdminScope, tenantB.AdminScope, "BillRepository.Get cross-tenant company-level")

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Bill, error) { return repo.Get(ctx, s, af.billID) },
			tenantA.Scope, tenantA.AdminScope, "BillRepository.Get narrow-scope")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Bill, error) { return repo.List(ctx, s, store.BillFilter{}) },
			tenantA.Scope, tenantA.AdminScope, af.billID,
			func(b model.Bill) uuid.UUID { return b.ID }, "BillRepository.List narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Bill, error) {
				return repo.Current(ctx, s, model.BillScopeBuilding, af.buildingID, "2026-01")
			},
			tenantA.Scope, tenantA.AdminScope, "BillRepository.Current narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.BillLine, error) { return repo.Lines(ctx, s, af.billID) },
			tenantA.Scope, tenantA.AdminScope, "BillRepository.Lines narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.BillMember, error) { return repo.Members(ctx, s, af.billID) },
			tenantA.Scope, tenantA.AdminScope, "BillRepository.Members narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.BillHourlyDetail, error) { return repo.HourlyDetail(ctx, s, af.billID) },
			tenantA.Scope, tenantA.AdminScope, "BillRepository.HourlyDetail narrow-scope")

		// Important Finding 4 / HANDOFF ruling 7: a company-level bill
		// (building_id AND analyzer_id both nil) is visible only to
		// AllBuildings.
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Bill, error) { return repo.Get(ctx, s, af.companyBillID) },
			tenantA.Scope, tenantA.AdminScope, "BillRepository.Get narrow-scope company-level")
		narrowListExcludesCompanyBill, err := repo.List(ctx, tenantA.Scope, store.BillFilter{})
		require.NoError(t, err)
		scopeIsoRequireExcludes(t, narrowListExcludesCompanyBill, af.companyBillID, func(b model.Bill) uuid.UUID { return b.ID })
	})

	// --- ReportRepository (building-scoped, building_id NOT NULL) ---------------------
	t.Run("ReportRepository", func(t *testing.T) {
		repo := postgres.NewReportRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Report, error) { return repo.Get(ctx, s, bf.reportID) },
			tenantA.AdminScope, tenantB.AdminScope, "ReportRepository.Get cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Report, error) { return repo.List(ctx, s, store.ReportFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, bf.reportID,
			func(r model.Report) uuid.UUID { return r.ID }, "ReportRepository.List cross-tenant")

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Report, error) { return repo.Get(ctx, s, af.reportID) },
			tenantA.Scope, tenantA.AdminScope, "ReportRepository.Get narrow-scope")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Report, error) { return repo.List(ctx, s, store.ReportFilter{}) },
			tenantA.Scope, tenantA.AdminScope, af.reportID,
			func(r model.Report) uuid.UUID { return r.ID }, "ReportRepository.List narrow-scope")

		// Count answers the archive's total: another tenant's and an
		// out-of-scope building's reports never add to it.
		narrow, err := repo.Count(ctx, tenantA.Scope, store.ReportFilter{})
		require.NoError(t, err)
		wide, err := repo.Count(ctx, tenantA.AdminScope, store.ReportFilter{})
		require.NoError(t, err)
		require.Less(t, narrow, wide, "ReportRepository.Count narrow-scope counts the out-of-scope building's report")
		foreign, err := repo.Count(ctx, tenantB.AdminScope, store.ReportFilter{BuildingID: &af.buildingID})
		require.NoError(t, err)
		require.Zero(t, foreign, "ReportRepository.Count cross-tenant")
	})

	// --- AlarmRepository (company-only: alarms carries no building_id) ----------------
	t.Run("AlarmRepository", func(t *testing.T) {
		repo := postgres.NewAlarmRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.Alarm, error) { return repo.Get(ctx, s, bf.alarmID) },
			tenantA.AdminScope, tenantB.AdminScope, "AlarmRepository.Get cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.Alarm, error) { return repo.List(ctx, s, store.AlarmFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, bf.alarmID,
			func(a model.Alarm) uuid.UUID { return a.ID }, "AlarmRepository.List cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.AlarmAnalyzer, error) { return repo.Analyzers(ctx, s, bf.alarmID) },
			tenantA.AdminScope, tenantB.AdminScope, "AlarmRepository.Analyzers cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.AlarmChannel, error) { return repo.Channels(ctx, s, bf.alarmID) },
			tenantA.AdminScope, tenantB.AdminScope, "AlarmRepository.Channels cross-tenant")

		// --- ListEvents: Important Finding 1. AlarmEventList's `alarms`
		// parent predicate (a.company_id = ...) is the ONLY thing protecting
		// a NULL-analyzer event (a company-level condition) — its own branch
		// checks only all_buildings, never company_id directly. Filtering by
		// bf.alarmID and asserting emptiness alone would be shadowed by the
		// an.company_id check on the analyzer join for bf.alarmEventID
		// (analyzer set), which is why bf.alarmEventNullAnalyzerID is seeded
		// and checked explicitly here.
		events, err := repo.ListEvents(ctx, tenantA.AdminScope, store.AlarmEventFilter{AlarmID: &bf.alarmID})
		require.NoError(t, err)
		require.Empty(t, events, "tenant B's alarm events, including its NULL-analyzer one, must never appear under tenant A's AdminScope")
		ownEvents, err := repo.ListEvents(ctx, tenantB.AdminScope, store.AlarmEventFilter{AlarmID: &bf.alarmID})
		require.NoError(t, err)
		require.True(t, scopeIsoContainsID(ownEvents, bf.alarmEventID, func(e model.AlarmEvent) uuid.UUID { return e.ID }))
		require.True(t, scopeIsoContainsID(ownEvents, bf.alarmEventNullAnalyzerID, func(e model.AlarmEvent) uuid.UUID { return e.ID }),
			"positive control: tenant B's own AdminScope must see its own NULL-analyzer event")

		// --- narrow-scope half: ListEvents is the one AlarmRepository
		// method that DOES narrow by building (through the event's
		// analyzer, when set) — unlike Get/List/Analyzers/Channels, which
		// repository.go's own doc comment says are company-only (alarms
		// carries no building_id); that classification does not extend to
		// ListEvents' analyzer join. af.alarmEventID's analyzer sits on
		// Buildings[1] (outside the narrow Scope's grant); af's
		// NULL-analyzer event has no building at all.
		narrowEvents, err := repo.ListEvents(ctx, tenantA.Scope, store.AlarmEventFilter{AlarmID: &af.alarmID})
		require.NoError(t, err)
		require.Empty(t, narrowEvents,
			"af's alarm's events — one on Buildings[1]'s analyzer, one with no analyzer at all — must both be invisible to the narrow Scope")
		ownNarrowEvents, err := repo.ListEvents(ctx, tenantA.AdminScope, store.AlarmEventFilter{AlarmID: &af.alarmID})
		require.NoError(t, err)
		require.True(t, scopeIsoContainsID(ownNarrowEvents, af.alarmEventID, func(e model.AlarmEvent) uuid.UUID { return e.ID }))
		require.True(t, scopeIsoContainsID(ownNarrowEvents, af.alarmEventNullAnalyzerID, func(e model.AlarmEvent) uuid.UUID { return e.ID }),
			"positive control: tenant A's own AdminScope must see Buildings[1]'s NULL-analyzer event")
	})

	// --- CarbonRepository (factors company/platform; activities+reports
	// building-scoped) -----------------------------------------------------------------
	t.Run("CarbonRepository", func(t *testing.T) {
		repo := postgres.NewCarbonRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.EmissionFactor, error) { return repo.Factor(ctx, s, bf.carbonFactorID) },
			tenantA.AdminScope, tenantB.AdminScope, "CarbonRepository.Factor cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.EmissionFactor, error) {
				return repo.ListFactors(ctx, s, store.EmissionFactorFilter{})
			},
			tenantA.AdminScope, tenantB.AdminScope, bf.carbonFactorID,
			func(f model.EmissionFactor) uuid.UUID { return f.ID }, "CarbonRepository.ListFactors cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.EmissionFactorConversion, error) {
				return repo.Conversions(ctx, s, bf.carbonFactorID)
			},
			tenantA.AdminScope, tenantB.AdminScope, "CarbonRepository.Conversions cross-tenant")

		// SelectedActivities has no ErrNotFound path (repository.go carries
		// no such note for it, and the implementation just filters by scope
		// and returns whatever matches): a building the Scope cannot see
		// yields an empty slice with a nil error, never an error.
		crossSelected, err := repo.SelectedActivities(ctx, tenantA.AdminScope, bf.buildingID)
		require.NoError(t, err)
		require.Empty(t, crossSelected)
		ownCrossSelected, err := repo.SelectedActivities(ctx, tenantB.AdminScope, bf.buildingID)
		require.NoError(t, err)
		require.NotEmpty(t, ownCrossSelected, "positive control")

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.CarbonActivity, error) { return repo.Activity(ctx, s, bf.carbonActivityID) },
			tenantA.AdminScope, tenantB.AdminScope, "CarbonRepository.Activity cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.CarbonActivity, error) {
				return repo.ListActivities(ctx, s, store.CarbonActivityFilter{})
			},
			tenantA.AdminScope, tenantB.AdminScope, bf.carbonActivityID,
			func(a model.CarbonActivity) uuid.UUID { return a.ID }, "CarbonRepository.ListActivities cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.CarbonReport, error) { return repo.ListReports(ctx, s, nil, store.Page{}) },
			tenantA.AdminScope, tenantB.AdminScope, bf.carbonReportID,
			func(r model.CarbonReport) uuid.UUID { return r.ID }, "CarbonRepository.ListReports cross-tenant")

		narrowSelected, err := repo.SelectedActivities(ctx, tenantA.Scope, af.buildingID)
		require.NoError(t, err)
		require.Empty(t, narrowSelected, "Buildings[1]'s selected activities must be invisible to the narrow Scope")
		ownNarrowSelected, err := repo.SelectedActivities(ctx, tenantA.AdminScope, af.buildingID)
		require.NoError(t, err)
		require.NotEmpty(t, ownNarrowSelected, "positive control")

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.CarbonActivity, error) { return repo.Activity(ctx, s, af.carbonActivityID) },
			tenantA.Scope, tenantA.AdminScope, "CarbonRepository.Activity narrow-scope")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.CarbonActivity, error) {
				return repo.ListActivities(ctx, s, store.CarbonActivityFilter{})
			},
			tenantA.Scope, tenantA.AdminScope, af.carbonActivityID,
			func(a model.CarbonActivity) uuid.UUID { return a.ID }, "CarbonRepository.ListActivities narrow-scope")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.CarbonReport, error) { return repo.ListReports(ctx, s, nil, store.Page{}) },
			tenantA.Scope, tenantA.AdminScope, af.carbonReportID,
			func(r model.CarbonReport) uuid.UUID { return r.ID }, "CarbonRepository.ListReports narrow-scope")
	})

	// --- ISO50001Repository (building-scoped project, with no-company_id
	// children) -----------------------------------------------------------------------
	t.Run("ISO50001Repository", func(t *testing.T) {
		repo := postgres.NewISO50001Repository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.ISO50001Project, error) { return repo.Project(ctx, s, bf.buildingID) },
			tenantA.AdminScope, tenantB.AdminScope, "ISO50001Repository.Project cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.ISO50001ClauseDate, error) {
				return repo.ClauseDates(ctx, s, bf.isoProjectID)
			},
			tenantA.AdminScope, tenantB.AdminScope, "ISO50001Repository.ClauseDates cross-tenant")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.ISO50001Note, error) { return repo.Notes(ctx, s, bf.isoProjectID, nil) },
			tenantA.AdminScope, tenantB.AdminScope, "ISO50001Repository.Notes cross-tenant")

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.ISO50001Project, error) { return repo.Project(ctx, s, af.buildingID) },
			tenantA.Scope, tenantA.AdminScope, "ISO50001Repository.Project narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.ISO50001ClauseDate, error) {
				return repo.ClauseDates(ctx, s, af.isoProjectID)
			},
			tenantA.Scope, tenantA.AdminScope, "ISO50001Repository.ClauseDates narrow-scope")
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) ([]model.ISO50001Note, error) { return repo.Notes(ctx, s, af.isoProjectID, nil) },
			tenantA.Scope, tenantA.AdminScope, "ISO50001Repository.Notes narrow-scope")
	})

	// --- FileRepository (company-only, no building_id) ---------------------------------
	t.Run("FileRepository", func(t *testing.T) {
		repo := postgres.NewFileRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.StoredFile, error) { return repo.Get(ctx, s, bf.fileID) },
			tenantA.AdminScope, tenantB.AdminScope, "FileRepository.Get cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.StoredFile, error) { return repo.List(ctx, s, store.FileFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, bf.fileID,
			func(f model.StoredFile) uuid.UUID { return f.ID }, "FileRepository.List cross-tenant")
	})

	// --- IntegrationRepository (Definitions/Definition platform-wide;
	// Credential/ListCredentials/OpenSecret company-only) -------------------------------------------
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
		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.IntegrationCredential, error) {
				return repo.Credential(ctx, s, bf.integrationDefinitionID)
			},
			tenantA.AdminScope, tenantB.AdminScope, "IntegrationRepository.Credential cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.IntegrationCredential, error) { return repo.ListCredentials(ctx, s) },
			tenantA.AdminScope, tenantB.AdminScope, bf.credentialID,
			func(c model.IntegrationCredential) uuid.UUID { return c.ID }, "IntegrationRepository.ListCredentials cross-tenant")

		// Important Finding 3: OpenSecret is keyed by the credential's OWN
		// surrogate id (query IntegrationGetCredential: id, company_id) — a
		// DIFFERENT query from Credential's (IntegrationGetCredentialByDefinition,
		// keyed by definition_id). The reviewer's tautology on
		// IntegrationGetCredential's company_id predicate stayed green
		// because OpenSecret was never exercised here at all before this fix.
		secretDenied, extraDenied, err := repo.OpenSecret(ctx, tenantA.AdminScope, bf.credentialID)
		require.ErrorIs(t, err, store.ErrNotFound)
		require.Nil(t, secretDenied)
		require.Nil(t, extraDenied)
		secretOwn, _, err := repo.OpenSecret(ctx, tenantB.AdminScope, bf.credentialID)
		require.NoError(t, err, "positive control: tenant B's own AdminScope must decrypt its own credential secret")
		require.Equal(t, []byte("scope-iso-secret"), secretOwn)
	})

	// --- SMTPRepository (company-only, one row per company keyed by
	// company_id — no id parameter anywhere on this interface, so the
	// isolation proof is on the CONTENT of what Get/OpenPassword return
	// rather than on ErrNotFound: both tenants have their own row, seeded
	// with the identical fixture shape by seedScopeIsoFixtures, so a leak
	// would be invisible unless the returned content is checked directly) --
	//
	// Controller ruling (final review A fix round, re-review of task-13b):
	// smtp_settings is keyed BY company_id (its own primary key), so a
	// tautologised SMTPGet `(company_id = $1 or true)` still returns exactly
	// ONE row — Postgres' own row order for a `:one` with no ORDER BY, which
	// in practice is the physically-first row (tenant A's, seeded first).
	// The PRE-fix version of this subtest checked only tenant A's OWN
	// Get/OpenPassword against a hardcoded expected value, so that
	// tautology was invisible: tenant A's Get happened to return tenant A's
	// own row anyway, by luck of insertion order, not because the predicate
	// held. This is now symmetric — BOTH tenants call Get and OpenPassword
	// under their own AdminScope, and each must see its OWN row/password,
	// with per-tenant passwords required to be distinct — so a first-row
	// tautology fails tenant B's assertions even though it would still pass
	// tenant A's.
	t.Run("SMTPRepository", func(t *testing.T) {
		repo := postgres.NewSMTPRepository(pool, cipher)

		require.NotEqual(t, af.smtpPassword, bf.smtpPassword,
			"fixture setup bug: per-tenant SMTP passwords must differ, or a first-row tautology would be invisible below")

		gotA, err := repo.Get(ctx, tenantA.AdminScope)
		require.NoError(t, err)
		require.Equal(t, tenantA.Company.ID, gotA.CompanyID, "tenant A's Get must never return tenant B's smtp_settings row")

		gotB, err := repo.Get(ctx, tenantB.AdminScope)
		require.NoError(t, err)
		require.Equal(t, tenantB.Company.ID, gotB.CompanyID, "tenant B's Get must never return tenant A's smtp_settings row")

		// Important Finding 3: OpenPassword has no id parameter at all (one
		// row per company, keyed by the Scope's OWN company_id) — there is
		// no way to hand it another tenant's id, so its isolation proof is
		// that it decrypts to exactly what THIS company's own Upsert stored,
		// for BOTH tenants (not just the one that happens to seed first).
		passwordA, err := repo.OpenPassword(ctx, tenantA.AdminScope)
		require.NoError(t, err)
		require.Equal(t, af.smtpPassword, passwordA)

		passwordB, err := repo.OpenPassword(ctx, tenantB.AdminScope)
		require.NoError(t, err)
		require.Equal(t, bf.smtpPassword, passwordB)
	})

	// --- CalendarRepository (company-only) ------------------------------------------------
	t.Run("CalendarRepository", func(t *testing.T) {
		repo := postgres.NewCalendarRepository(pool)

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.CalendarEvent, error) { return repo.Event(ctx, s, bf.calendarEventID) },
			tenantA.AdminScope, tenantB.AdminScope, "CalendarRepository.Event cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.CalendarEvent, error) {
				return repo.ListEvents(ctx, s, store.CalendarFilter{})
			},
			tenantA.AdminScope, tenantB.AdminScope, bf.calendarEventID,
			func(e model.CalendarEvent) uuid.UUID { return e.ID }, "CalendarRepository.ListEvents cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.CompanyVacation, error) { return repo.Vacations(ctx, s, nil) },
			tenantA.AdminScope, tenantB.AdminScope, bf.calendarVacationID,
			func(v model.CompanyVacation) uuid.UUID { return v.ID }, "CalendarRepository.Vacations cross-tenant")

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

		scopeIsoAssertGetNotFound(t,
			func(s store.Scope) (model.JobRun, error) { return repo.GetRun(ctx, s, bf.jobRunID) },
			tenantA.AdminScope, tenantB.AdminScope, "OpsRepository.GetRun cross-tenant")
		scopeIsoAssertListExcludes(t,
			func(s store.Scope) ([]model.JobRun, error) { return repo.ListRuns(ctx, s, store.JobRunFilter{}) },
			tenantA.AdminScope, tenantB.AdminScope, bf.jobRunID,
			func(r model.JobRun) uuid.UUID { return r.ID }, "OpsRepository.ListRuns cross-tenant")

		// Both tenants' seeding appends a message with the identical Category
		// ("scope-iso"), so the isolation proof is on the message's own ID
		// (bigserial, globally unique) rather than on its Category value.
		messages, err := repo.ListMessages(ctx, tenantA.AdminScope, store.MessageFilter{})
		require.NoError(t, err)
		sawOwn := false
		for _, m := range messages {
			require.NotEqual(t, bf.opsMessageID, m.ID, "tenant B's operational message must never appear in tenant A's list")
			if m.ID == af.opsMessageID {
				sawOwn = true
			}
		}
		require.True(t, sawOwn, "positive control: tenant A's own AdminScope must see its own operational message")

		// R237: the task id is deterministic per subject and period, so two
		// companies can hold the same one. The read must stay in its tenant.
		const sharedTaskID = "billing.generate:company:scope-iso:2026-08"
		own, err := repo.RunByTaskID(ctx, tenantA.AdminScope, sharedTaskID)
		require.NoError(t, err, "positive control: tenant A sees its own run")
		require.Equal(t, af.jobRunID, own.ID)
		other, err := repo.RunByTaskID(ctx, tenantB.AdminScope, sharedTaskID)
		require.NoError(t, err)
		require.Equal(t, bf.jobRunID, other.ID, "each tenant sees only its own run for the shared task id")
		_, err = repo.RunByTaskID(ctx, invalidScope, sharedTaskID)
		require.ErrorIs(t, err, store.ErrInvalidScope)
	})
}
