package arch_test

import (
	"go/ast"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

const modulePath = "github.com/MErenTalan/ekokod-rewrite"

// underPackage reports whether pkgPath is target itself or a true
// subpackage of it (target followed by "/"). A bare strings.HasPrefix
// match with no trailing-slash requirement would also match any sibling
// package whose path merely starts with the same characters — e.g.
// target "internal/api" would wrongly match a future "internal/apikeys"
// or "internal/apiclient" package, silently exempting it from whichever
// guard is doing the matching. That is a false negative in exactly the
// class of bug these architecture guards exist to catch, so every
// package-path comparison in this file must go through this helper
// rather than a raw HasPrefix call.
func underPackage(pkgPath, target string) bool {
	return pkgPath == target || strings.HasPrefix(pkgPath, target+"/")
}

// domainAllowedImports is the ALLOW-list of non-project packages
// internal/domain may import: a base of I/O-free stdlib packages that do
// pure computation or hold data (never reach outside the process), plus the
// two third-party libraries the domain layer is actually built on — decimal
// money and uuid identifiers.
//
// This was a DENY-list (forbiddenInDomain, 9 stdlib paths matched by exact
// equality) until F1's final review pass B, I2. A denylist only refuses the
// imports someone thought to name in advance, and it is provably incomplete:
// the review's mutation probe added an internal/domain/model import of
// github.com/jackc/pgx/v5 — a real Postgres driver, the exact shape of I/O
// this guard exists to keep out — and the denylist let it straight through,
// because pgx was never on the list and never could be by construction; a
// denylist has no way to refuse "any third-party package" up front, only
// named ones. An allow-list inverts the default: nothing gets in unless it
// is named here, so a future github.com/redis/go-redis import in a domain
// service fails the moment it lands, not the moment someone remembers to
// deny it.
//
// .golangci.yml's `domain-is-pure` depguard rule is switched to
// `list-mode: strict` with a matching `allow:` list at the same time. CHANGE
// BOTH OR NEITHER — the same rule this file's floatGuardPatterns comment
// already states for the float guard, extended to this guard because the
// review's second probe (a bare os/signal import) showed the two checks had
// already drifted: depguard's prefix-matched deny caught os/signal, this
// test's exact-equality deny did not, and neither caught pgx.
var domainAllowedImports = map[string]bool{
	// stdlib: computation and data-structure packages only. None of these
	// reach a file, a socket, a process or a clock's wall time in a way that
	// makes results depend on the outside world at declaration time — "time"
	// itself is the borderline case, allowed because the domain layer holds
	// and compares time.Time values throughout (TimeRange, EffectiveFrom, …)
	// without itself calling time.Now() to decide anything.
	"bytes":         true,
	"cmp":           true,
	"encoding/json": true,
	"errors":        true,
	"fmt":           true,
	"maps":          true,
	"math":          true,
	"net/netip":     true,
	"slices":        true,
	"sort":          true,
	"strconv":       true,
	"strings":       true,
	"time":          true,
	"unicode":       true,
	"unicode/utf8":  true,

	// third-party: exactly what the domain layer is built on.
	"github.com/google/uuid":        true,
	"github.com/shopspring/decimal": true,
}

func loadPackages(t *testing.T, pattern string) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedImports, Dir: repoRoot(t)}
	pkgs, err := packages.Load(cfg, pattern)
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	return pkgs
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(wd, "..", "..")
}

// TestDomainHasNoProjectImports is named in the F0 acceptance criteria.
func TestDomainHasNoProjectImports(t *testing.T) {
	for _, pkg := range loadPackages(t, "./internal/domain/...") {
		for imported := range pkg.Imports {
			if underPackage(imported, modulePath) &&
				!underPackage(imported, modulePath+"/internal/domain") {
				t.Errorf("%s imports %s: internal/domain must not import project packages", pkg.PkgPath, imported)
			}
		}
	}
}

func TestDomainHasNoIOImports(t *testing.T) {
	for _, pkg := range loadPackages(t, "./internal/domain/...") {
		for imported := range pkg.Imports {
			if underPackage(imported, modulePath) {
				// A project import's own purity is TestDomainHasNoProjectImports'
				// job (it must be under internal/domain itself, or that test
				// already fails it); domainAllowedImports covers only the
				// non-project surface.
				continue
			}
			if !domainAllowedImports[imported] {
				t.Errorf("%s imports %s: not on internal/domain's import allow-list (domainAllowedImports in this file) — "+
					"internal/domain must be free of I/O, so an import is refused by default and must be added to the "+
					"allow-list, and to .golangci.yml's domain-is-pure allow list, ONLY after confirming it does no I/O",
					pkg.PkgPath, imported)
			}
		}
	}
}

func TestOnlyTheCLIImportsTheAPIPackage(t *testing.T) {
	allowed := map[string]bool{
		modulePath + "/internal/cli": true,
		modulePath + "/internal/api": true,
	}
	for _, pkg := range loadPackages(t, "./...") {
		if allowed[pkg.PkgPath] || underPackage(pkg.PkgPath, modulePath+"/internal/api") {
			continue
		}
		for imported := range pkg.Imports {
			if underPackage(imported, modulePath+"/internal/api") {
				t.Errorf("%s imports %s: only internal/cli may wire the HTTP layer", pkg.PkgPath, imported)
			}
		}
	}
}

// TestStoreAndIntegrationDoNotImportAPIOrService is widened for F2: its name
// has always claimed to cover internal/integration, but until this guard
// gained an ./internal/integration/... pattern it only ever loaded
// ./internal/store/.... internal/integration is the other package this
// codebase requires to stay free of the HTTP layer (06 §1 rule 2: adapters
// are pure clients, and an import of internal/api would let one reach for
// an HTTP request/response type instead of its own).
func TestStoreAndIntegrationDoNotImportAPIOrService(t *testing.T) {
	for _, pattern := range []string{"./internal/store/...", "./internal/integration/..."} {
		for _, pkg := range loadPackages(t, pattern) {
			for imported := range pkg.Imports {
				require.False(t, underPackage(imported, modulePath+"/internal/api"),
					"%s must not import the HTTP layer", pkg.PkgPath)
			}
		}
	}
}

// f2guardAdapterForbidden are the packages TestAdaptersDoNotImportTheStore
// forbids internal/integration/... from importing: the store layer and the
// pgx driver it sits on (an adapter that reached either could write to the
// database directly, violating 06 §1 rule 2 "adapters never write"), and
// internal/ingest, internal/credentials and internal/marketdata, which
// depend ON internal/integration and would form an import cycle — or, short
// of an actual cycle, exactly the coupling that makes an adapter no longer
// a pure client — if internal/integration ever imported back into them.
var f2guardAdapterForbidden = []string{
	modulePath + "/internal/store",
	modulePath + "/internal/ingest",
	modulePath + "/internal/credentials",
	modulePath + "/internal/marketdata",
	"github.com/jackc/pgx",
}

// f2guardLoadPackagesWithDeps is loadPackages' counterpart for a guard that
// must see the FULL transitive import graph, not just each package's direct
// imports. packages.Load with only NeedImports (loadPackages' mode)
// populates pkg.Imports with direct dependencies, but those dependency
// Package objects are themselves left as incomplete stubs — their own
// .Imports is empty — so a recursive walk over them finds nothing. NeedDeps
// tells the loader to fully populate every transitively reached package
// too, so pkg.Imports[x].Imports is real data all the way down (I4).
func f2guardLoadPackagesWithDeps(t *testing.T, pattern string) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps,
		Dir:  repoRoot(t),
	}
	pkgs, err := packages.Load(cfg, pattern)
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	return pkgs
}

// f2guardTransitiveOffender walks pkg's full import graph (direct and
// indirect, via pkg.Imports which f2guardLoadPackagesWithDeps populated
// transitively) and returns the import chain from pkg to the first package
// matching one of forbidden, or nil if none is reachable. visited prevents
// revisiting a package already walked (import graphs are DAGs but commonly
// diamond-shaped) and must be seeded with pkg.PkgPath by the caller.
func f2guardTransitiveOffender(pkg *packages.Package, forbidden []string, visited map[string]bool) []string {
	// Deterministic order: map iteration order is random, and a test that
	// sometimes reports package A and sometimes package B for the same
	// violation (both present) makes the failure message flaky to read,
	// even though the test itself fails reliably either way.
	paths := make([]string, 0, len(pkg.Imports))
	for path := range pkg.Imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		for _, f := range forbidden {
			if underPackage(path, f) {
				return []string{path}
			}
		}
		if visited[path] {
			continue
		}
		visited[path] = true
		if chain := f2guardTransitiveOffender(pkg.Imports[path], forbidden, visited); chain != nil {
			return append([]string{path}, chain...)
		}
	}
	return nil
}

// TestAdaptersDoNotImportTheStore enforces 06 §1 rule 2, "adapters are pure
// clients; persistence is the ingestion job's responsibility": nothing under
// internal/integration may import the store layer, the pgx driver, or any
// of the three packages that themselves depend on internal/integration —
// TRANSITIVELY, not just directly (I4): internal/integration ->
// internal/f2probehelper -> internal/store must be caught exactly like
// internal/integration -> internal/store, because a helper package one hop
// away is just as much a route to the database as importing it directly.
func TestAdaptersDoNotImportTheStore(t *testing.T) {
	pkgs := f2guardLoadPackagesWithDeps(t, "./internal/integration/...")
	require.NotEmpty(t, pkgs, "the guard walked zero packages: it would pass whatever the code said")
	for _, pkg := range pkgs {
		if chain := f2guardTransitiveOffender(pkg, f2guardAdapterForbidden, map[string]bool{pkg.PkgPath: true}); chain != nil {
			t.Errorf("%s transitively imports a forbidden package via %s: adapters must be pure — no store, no pgx, no dependency (direct or transitive) on the packages that depend on internal/integration",
				pkg.PkgPath, strings.Join(chain, " -> "))
		}
	}
}

// TestNoTLSVerificationBypass guards removed-behaviour item 16.
//
// Deviation from the brief: the brief's sample walk has no exemption for
// its own source file. Since this test's own code necessarily contains the
// literal string "InsecureSkipVerify" (the substring it searches for), an
// unmodified copy of the brief's walk fails against itself the moment it
// runs — verified live: it does, with arch_test.go named as the offending
// file. The requirement ("InsecureSkipVerify does not appear" in real code)
// wins over the sample's walk, so this file is exempted by its own path,
// obtained via runtime.Caller rather than a hardcoded string, so the
// exemption cannot silently widen if the file is ever renamed or moved.
func TestNoTLSVerificationBypass(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) must resolve this test's own file")
	root := filepath.Join(repoRoot(t), "internal")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if path == thisFile {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), "InsecureSkipVerify") {
			t.Errorf("%s contains InsecureSkipVerify: TLS verification is never disabled", path)
		}
		return nil
	})
	require.NoError(t, err)
}

// loggerHandlerConstructionAllowlist names the only non-test file permitted
// to construct a bare slog.NewJSONHandler/slog.NewTextHandler.
// internal/platform/logging/logging.go (internal/platform/logging/logging.go)
// is where that construction legitimately happens: New() wraps whichever
// bare handler it builds in redactHandler (internal/platform/logging/redact.go)
// before returning it, so that file is the one place a bare handler is
// supposed to exist. Every other call site must go through logging.New.
//
// This is deliberately a single exact file path, not a directory or package
// prefix, so a new file added anywhere else — including a new file inside
// internal/platform/logging itself — is not silently exempted.
//
// Why this matters: redactAttr (internal/platform/logging/redact.go:63-76)
// is key-based only. It calls isSecretKey(a.Key) and never inspects the
// attribute's value, so a slog.String("error", err.Error()) call is only
// safe because logging.New's handler wraps the underlying handler; a call
// site that builds its own slog.NewJSONHandler/slog.NewTextHandler bypasses
// that wrapping entirely and every attribute value it logs — including
// error strings that may contain secrets — reaches the log unredacted.
//
// The walk covers all of internal/, not just internal/cli, because the
// requirement ("every long-running command's logger must go through
// logging.New") is not scoped to the CLI package: any future package that
// wires up a real (non-test) logger is bound by the same rule.
var loggerHandlerConstructionAllowlist = map[string]bool{
	"internal/platform/logging/logging.go": true,
}

// TestOnlyLoggingPackageConstructsBareSlogHandlers is the carried-forward
// guard from task 9's review (Minor-4): before newCommandLogger existed,
// reverting a call site's logger construction to a bare
// slog.NewJSONHandler compiled and passed the entire test suite silently,
// because internal/cli's unit test for newCommandLogger only pinned the
// helper itself, not every caller. This test pins every caller, repo-wide.
func TestOnlyLoggingPackageConstructsBareSlogHandlers(t *testing.T) {
	root := filepath.Join(repoRoot(t), "internal")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot(t), path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if loggerHandlerConstructionAllowlist[rel] {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), "slog.NewJSONHandler") || strings.Contains(string(content), "slog.NewTextHandler") {
			t.Errorf("%s constructs a bare slog handler: every long-running command's logger must go through logging.New (internal/platform/logging/logging.go), whose redacting handler is the only thing keeping secret-looking attribute values out of the logs", rel)
		}
		return nil
	})
	require.NoError(t, err)
}

// loadTypedPackages is loadPackages' counterpart for the guards that must
// reason about resolved Go types rather than import paths alone. The extra
// modes are what make TestEveryStoreMethodIsScoped an AST/type check instead
// of a textual scan: NeedTypes gives the package's declared objects,
// NeedSyntax and NeedTypesInfo give the files and the type information for
// them, and NeedDeps|NeedImports are required for a parameter's type to
// resolve to the *types.Named for store.Scope in a *different* package
// rather than to an unresolved placeholder.
//
// It fails the test on any load or type error. A guard that walks zero
// packages, or walks packages whose parameter types could not be resolved,
// passes vacuously and protects nothing — so a broken load must be a red
// test, never a quiet green one.
func loadTypedPackages(t *testing.T, pattern string) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Dir: repoRoot(t),
	}
	pkgs, err := packages.Load(cfg, pattern)
	require.NoError(t, err)
	require.NotEmpty(t, pkgs, "pattern %q matched no packages: the guard would pass vacuously", pattern)
	for _, pkg := range pkgs {
		for _, pkgErr := range pkg.Errors {
			t.Errorf("loading %s: %v", pkg.PkgPath, pkgErr)
		}
	}
	return pkgs
}

// isScopeType reports whether typ is store.Scope (or a pointer to it),
// matched by the type's *identity* — the defining package path plus the
// object name — and not by its spelling at the call site. A textual check
// for "store.Scope" in the source would be defeated by an import alias
// (`import st "…/internal/store"` → `st.Scope`), by a dot-import, and by a
// local type named Scope in some other package; each of those would let an
// unscoped method through, which is the exact failure this guard exists to
// prevent.
func isScopeType(typ types.Type) bool {
	return isNamed(typ, modulePath+"/internal/store", "Scope")
}

// isContextType reports whether typ is context.Context, matched the same way
// and for the same reasons as isScopeType.
func isContextType(typ types.Type) bool {
	return isNamed(typ, "context", "Context")
}

func isNamed(typ types.Type, pkgPath, name string) bool {
	if ptr, ok := types.Unalias(typ).(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	named, ok := types.Unalias(typ).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == pkgPath && obj.Name() == name
}

// signatureTakes reports whether any parameter of sig satisfies match.
func signatureTakes(sig *types.Signature, match func(types.Type) bool) bool {
	params := sig.Params()
	for i := range params.Len() {
		if match(params.At(i).Type()) {
			return true
		}
	}
	return false
}

// methodSetOf returns the full method set of named, INCLUDING methods
// promoted from embedded fields.
//
// named.NumMethods() would be the obvious call and is wrong here: it reports
// only explicitly declared methods. A repository written as
// `type buildingRepository struct { *pgxpool.Pool }` declares nothing, yet
// exposes Query, Exec and Ping — every one of them an unscoped, exported,
// context-taking database method reachable on the repository value. That is
// not a false positive to be filtered out; it is precisely the cross-tenant
// hole this guard exists to catch, and the fix for it is to make the pool a
// named field instead of an embedded one.
//
// The method set is taken on the POINTER type so that methods with pointer
// receivers are included; for an interface, whose pointer has an empty
// method set, the named type is used directly.
func methodSetOf(named *types.Named) *types.MethodSet {
	if _, isInterface := named.Underlying().(*types.Interface); isInterface {
		return types.NewMethodSet(named)
	}
	return types.NewMethodSet(types.NewPointer(named))
}

// TestEveryStoreMethodIsScoped enforces docs/rewrite/03-target-architecture.md
// §2.5: "The repository layer takes an explicit Scope value; there is no
// 'unscoped' query method outside the admin package."
//
// One repository method that forgets its store.Scope parameter is a silent
// cross-tenant read: it compiles, its own tests pass (they only ever look at
// one tenant's data), and nothing in the system objects until a customer sees
// another customer's meter readings. That is the worst defect class this
// schema can have, and it is invisible to every other check in this file, so
// it gets a type-resolving guard rather than a textual one.
//
// WHAT IS INSPECTED, and why the predicate is shaped this way. Every exported
// method — declared or promoted — of every named type under
// ./internal/store/postgres/... that takes a context.Context must also take a
// store.Scope.
//
//   - The type may be UNEXPORTED. The first draft of this guard required an
//     exported type whose name ended in "Repository", which would have missed
//     the most idiomatic shape in Go: an unexported `buildingRepository`
//     struct satisfying an exported `store.BuildingRepository` interface. It
//     also missed BuildingRepo, BuildingStore and plain Buildings. Nothing is
//     gained by guessing at names, so the name test is gone entirely.
//   - The method must be EXPORTED, and this is settled, not provisional. The
//     guard checks the package's EXTERNAL CONTRACT, which is where a
//     cross-tenant hole can actually be reached from; an unexported method is
//     callable only from inside internal/store/postgres. Widening to
//     unexported methods would flag ordinary internal plumbing — scan,
//     queryRow and friends — and the only cure for those false positives
//     would be loosening the guard, which is how guards die. An unexported
//     type is still covered, because its methods reach other packages through
//     an exported interface.
//   - context.Context is the proxy for "this method talks to the database".
//     It is not a perfect proxy, but a repository method that does I/O without
//     a context would already be violating a different, older rule in this
//     codebase.
//
// internal/store/postgres/admin is skipped. It is the deliberate, and the
// only, unscoped surface in the system — the exemption is matched with
// underPackage so that a future sibling such as "…/postgres/adminui" cannot
// inherit it by prefix.
//
// storeScopeAllowlist names the exported package-level functions and
// func-typed variables under internal/store/postgres (outside admin and
// sqlcgen, which are already whole-package exemptions) that legitimately
// take a context.Context with no store.Scope. Every one of them is
// infrastructure — pool construction, migrations, health — not a
// repository read or write, so binding rule "every exported method is
// tenant-scoped" does not apply to them in the first place.
//
// This is an EXACT-NAME allowlist, deliberately: a prefix or pattern match
// would widen itself the first time someone chose a name that happened to
// match it, which is exactly the class of silent widening this whole file
// exists to prevent. A name added here without actually being
// infrastructure is a code-review-visible one-line addition, which is the
// point — nothing here can be evaded merely by adding a new function.
//
// F1 final review pass B, I1, added this list together with the function/var
// walk below. Before it, TestEveryStoreMethodIsScoped resolved
// *types.TypeName entries and their method sets ONLY: it never looked at
// *types.Func or *types.Var in a package's scope at all, so an exported
// package-level function or a package-level variable of function type,
// taking a context.Context and reading across tenants with no store.Scope,
// passed with no error whatsoever — proven by the review's mutation probe,
// which added exactly that shape (an unscoped free function AND an unscoped
// func-typed var) and watched both sail through while a control method in
// the same file was correctly flagged. A free function is the most natural
// shape for a future worker or ingest helper ("load every analyzer for
// polling"), which is precisely the surface F2 is about to add.
var storeScopeAllowlist = map[string]bool{
	"NewPool":           true, // internal/store/postgres/pool.go
	"Ping":              true, // internal/store/postgres/pool.go
	"MigrateUp":         true, // internal/store/postgres/migrate.go
	"MigrateDownAll":    true, // internal/store/postgres/migrate.go
	"MigrateDownN":      true, // internal/store/postgres/migrate.go (F2 task-5: rolls back the N most recent migrations; migration infrastructure like MigrateUp/MigrateDownAll, not a repository read or write)
	"MigrateStatus":     true, // internal/store/postgres/migrate.go
	"PendingMigrations": true, // internal/store/postgres/migrate.go
	"MigrationsCheck":   true, // internal/store/postgres/health.go (no ctx param today; listed so an added one stays exempt without a guard edit)
}

// inspectContextTakingSignature applies TestEveryStoreMethodIsScoped's rule
// — a context.Context parameter implies a store.Scope parameter — to one
// method, package-level function, or func-typed variable's signature. It
// reports whether sig was INSPECTED (took a context.Context at all; a
// signature with none says nothing about tenant scoping and is not counted,
// same as before this function existed) so callers can accumulate the
// anti-vacuity count across all three shapes uniformly.
func inspectContextTakingSignature(t *testing.T, sig *types.Signature, pkgPath, label, adminPkg, sqlcgenPkg string) int {
	t.Helper()
	if !signatureTakes(sig, isContextType) {
		return 0
	}
	if !signatureTakes(sig, isScopeType) {
		t.Errorf("%s.%s takes a context.Context but no store.Scope: every exported repository surface must be tenant-scoped "+
			"(only %s, the deliberate unscoped surface; %s, sqlc's generated primitives; and storeScopeAllowlist's named "+
			"infrastructure functions, are exempt)",
			pkgPath, label, adminPkg, sqlcgenPkg)
	}
	return 1
}

// internal/store/postgres/sqlcgen is skipped too, for a different reason.
// It is sqlc's generated output: DBTX is the raw pgx driver interface
// (Exec/Query/QueryRow), and Queries holds one method per .sql file entry.
// Those methods ARE the unscoped primitive — they are what a scoped
// repository is built FROM, in the same way that a repository is built from
// a *pgxpool.Pool, which this guard has never flagged either. Requiring a
// store.Scope parameter on them is not possible: their signatures are
// generated from the SQL, and store.Scope is a Go type sqlc knows nothing
// about. Flagging them would leave only two outcomes — a permanently red
// guard, or hand-editing generated code — and neither closes a hole.
//
// What the exemption does NOT do is let that surface escape. sqlcgen is
// imported only by package postgres, whose own DB type keeps the generated
// query set in a named, unexported field precisely so the methods are not
// promoted onto an exported type (see the note on DB in
// internal/store/postgres/db.go — an earlier draft embedded it, and this
// guard is what caught that). So the guard's real job here is unchanged: it
// ensures nothing outside admin RE-EXPORTS the unscoped primitive, and it
// still inspects package postgres itself. Both exemptions are matched with
// underPackage rather than a bare prefix, so a future "…/postgres/sqlcgenx"
// cannot inherit either one by accident.
//
// WHAT THIS GUARD CANNOT DO, and who does it instead. This checks SIGNATURES,
// not USAGE. A method can take a store.Scope, satisfy this guard completely,
// and then never reference the parameter when it builds its SQL — a
// cross-tenant read that is invisible to any static check at this level.
// Nothing here will ever catch that, and no amount of widening the predicate
// would change it.
//
// That half is covered behaviourally by Task 13's TestScopeIsolation, which
// walks every repository with tenant A's scope and tenant B's ids and
// requires that nothing comes back. The division of labour is deliberate:
// GUARD FOR THE SIGNATURE HERE, TEST FOR THE BEHAVIOUR THERE. Neither is
// sufficient alone — a signature check cannot see intent, and a behavioural
// test only covers the repositories someone remembered to enumerate, which is
// exactly what this guard makes impossible to forget.
//
// The inspected-method count is asserted positive, not merely logged: Task 9
// introduces the first repositories under internal/store/postgres, so an
// empty walk is no longer legitimate. Before Task 9, the guard legitimately
// inspected zero methods and a narrowing bug in the predicate above looked
// identical to "there is nothing to check" — the same failure mode this
// require now closes for good, since a future narrowing bug that walked zero
// packages or zero methods would fail this line rather than pass silently.
func TestEveryStoreMethodIsScoped(t *testing.T) {
	const (
		adminPkg   = modulePath + "/internal/store/postgres/admin"
		sqlcgenPkg = modulePath + "/internal/store/postgres/sqlcgen"
	)

	inspected := 0
	for _, pkg := range loadTypedPackages(t, "./internal/store/postgres/...") {
		if underPackage(pkg.PkgPath, adminPkg) || underPackage(pkg.PkgPath, sqlcgenPkg) {
			continue
		}
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)

			switch decl := obj.(type) {
			case *types.TypeName:
				// A declared or promoted method on a named type — the
				// original shape this guard checked, unchanged.
				if decl.IsAlias() {
					continue
				}
				named, ok := types.Unalias(decl.Type()).(*types.Named)
				if !ok {
					continue
				}
				methods := methodSetOf(named)
				for i := range methods.Len() {
					method, ok := methods.At(i).Obj().(*types.Func)
					if !ok || !method.Exported() {
						continue
					}
					sig, ok := method.Type().(*types.Signature)
					if !ok {
						continue
					}
					inspected += inspectContextTakingSignature(t, sig, pkg.PkgPath, decl.Name()+"."+method.Name(), adminPkg, sqlcgenPkg)
				}

			case *types.Func:
				// An exported PACKAGE-LEVEL FUNCTION — the I1 gap. A free
				// function such as `func LoadAnalyzersForPolling(ctx
				// context.Context, pool *pgxpool.Pool) (...)` has no
				// receiver, so it was invisible to the TypeName/method-set
				// walk above no matter how that walk was extended; it has
				// to be checked on its own terms.
				if !decl.Exported() || storeScopeAllowlist[decl.Name()] {
					continue
				}
				sig, ok := decl.Type().(*types.Signature)
				if !ok {
					continue
				}
				inspected += inspectContextTakingSignature(t, sig, pkg.PkgPath, decl.Name(), adminPkg, sqlcgenPkg)

			case *types.Var:
				// An exported package-level VARIABLE OF FUNCTION TYPE — the
				// other I1 gap: `var LoadAnalyzersForPolling = func(ctx
				// context.Context, ...) (...) {...}` is a *types.Var whose
				// Type() is a *types.Signature, not a *types.Func at all, so
				// neither the TypeName branch above nor a naive
				// *types.Func-only fix would have caught it. This is not a
				// hypothetical: the review's mutation probe added exactly
				// this shape and it passed silently before this branch
				// existed.
				if !decl.Exported() || storeScopeAllowlist[decl.Name()] {
					continue
				}
				sig, ok := decl.Type().(*types.Signature)
				if !ok {
					continue
				}
				inspected += inspectContextTakingSignature(t, sig, pkg.PkgPath, decl.Name(), adminPkg, sqlcgenPkg)
			}
		}
	}

	t.Logf("inspected %d exported context-taking method(s), package-level function(s) and func-typed variable(s) under internal/store/postgres", inspected)
	// storeScopeGuardMinInspected is a DERIVED minimum, not a bare >0 check:
	// measured at 187 (methods only, before this guard could see funcs or
	// vars at all — I1's own before-state). require.Positive would pass
	// identically whether this walk inspects 187 things or 1, so a future
	// narrowing bug that dropped most of the walk down to a handful of
	// trivial signatures would still show green. The floor leaves headroom
	// for a repository shrinking (a method removed, a type merged) while
	// still failing loudly if the walk itself breaks.
	const storeScopeGuardMinInspected = 170
	require.GreaterOrEqual(t, inspected, storeScopeGuardMinInspected,
		"inspected implausibly few exported context-taking signatures (%d, floor %d) under internal/store/postgres: "+
			"the guard walked far less than it did when the floor was measured and would pass whatever the code said",
		inspected, storeScopeGuardMinInspected)
}

// TestTheJobPackageDoesNotImportTheStore pins the layering that
// internal/platform/secret's existence depends on: scrubParseErr lived in
// both internal/job and internal/store/redis as identical copies precisely
// because internal/job may not reach into internal/store to share it. With
// nothing enforcing that, the "obvious" fix to a future duplication is the
// import that this guard forbids.
func TestTheJobPackageDoesNotImportTheStore(t *testing.T) {
	for _, pkg := range loadPackages(t, "./internal/job/...") {
		for imported := range pkg.Imports {
			require.False(t, underPackage(imported, modulePath+"/internal/store"),
				"%s imports %s: internal/job must not depend on the store layer; shared helpers belong in internal/platform", pkg.PkgPath, imported)
		}
	}
}

// floatGuardPatterns are the package trees TestNoFloatFieldsInModelOrStore
// walks: the whole domain layer, where money and energy are declared and
// computed, and the whole store layer, where they are read and written. They
// are the places a float field would do damage; a float in an HTTP DTO is a
// presentation bug, a float here is a wrong invoice.
//
// Both are RECURSIVE patterns, deliberately. A package added under either tree
// later — internal/domain/billing, internal/domain/tariff, a new store
// subpackage — is covered the moment it exists, with no edit here. An earlier
// version listed ./internal/domain/model/... alone, so a float field in a new
// internal/domain/billing package passed while .golangci.yml claimed the two
// halves covered the same trees.
//
// .golangci.yml's no-float-money depguard rule lists exactly these six trees
// (**/internal/domain/**, **/internal/store/**, **/internal/integration/**,
// **/internal/ingest/**, **/internal/marketdata/** and
// **/internal/credentials/**). Change both or neither.
//
// The four F2 trees were added once internal/integration/doc.go and the
// three placeholder doc.go files existed to give each pattern something
// non-empty to match — loadTypedPackages requires a non-empty match, so a
// pattern added before its package exists would fail the guard for the
// wrong reason (a broken load, not a real violation).
var floatGuardPatterns = []string{
	"./internal/domain/...",
	"./internal/store/...",
	"./internal/integration/...",
	"./internal/ingest/...",
	"./internal/marketdata/...",
	"./internal/credentials/...",
}

// floatCarrierPackages are the FOREIGN packages whose struct types the float
// guard descends into. They are the packages whose structs exist to carry a
// column value, so a float inside one of their structs IS a float column value
// held in a field: pgtype.Float8 is what sqlc generates for a nullable
// `double precision` column, and pgtype.Float4, pgtype.Point/Box/Circle/Line
// and sql.NullFloat64 are the same thing in other shapes.
//
// Walking the whole package rather than deny-listing Float4, Float8 and
// NullFloat64 by name is the point: a deny-list is complete only until the
// next wrapper someone did not think of, and the guard would say nothing when
// it went stale.
var floatCarrierPackages = map[string]bool{
	"github.com/jackc/pgx/v5/pgtype": true,
	"database/sql":                   true,
	"database/sql/driver":            true,
}

// floatGuardMinFields is the anti-vacuity floor. Measured at 1581 struct
// fields when the reach was widened to the two recursive trees above; the
// floor leaves ~5% headroom so that deleting a genuinely dead struct does not
// turn the guard red, while a guard that silently stopped loading
// internal/store (~40% of the count) or internal/domain/model cannot pass.
// The count only grows as Tasks 9-11 add repositories; raise the floor when
// it has grown well past this.
const floatGuardMinFields = 1500

// TestNoFloatFieldsInModelOrStore enforces the invariant this project has
// asserted since F0: no struct field in the domain layer or the store layer is,
// or contains, a float.
//
// WHY THIS IS A TEST AND NOT A LINTER RULE. .golangci.yml's `no-float-money`
// is a depguard rule, and depguard is an IMPORT linter. It can forbid
// math/big; it cannot see `Total float64`, because no import is involved in
// declaring one. This test is the field-level half.
//
// WHAT IT CHECKS. Every field of every struct type declared under
// floatGuardPatterns — package-level types, types declared inside functions,
// and anonymous structs used as field types — resolved through the type
// checker, never matched by spelling. From the field's type it follows:
//
//   - pointers, slices, arrays, channels, and maps (key AND element);
//   - anonymous structs, field by field;
//   - aliases, via types.Unalias (`type Money = float64`);
//   - EVERY named type's underlying type, whichever package declares it, so a
//     defined type (`type Money float64`, or a foreign `type Rate float64`)
//     is caught, and so is an embedded field whose type is one;
//   - EVERY named type's type arguments, so `Box[float64]` and
//     `sql.Null[float64]` are caught even when Box's own declaration is clean;
//   - into the fields of a named STRUCT when the struct is declared anywhere
//     in this module (not only in the walked trees: a money type declared in
//     internal/platform and held here is still this codebase's choice) or in
//     one of floatCarrierPackages (pgtype.Float8, sql.NullFloat64, …).
//
// WHAT IT DOES NOT CHECK, deliberately.
//
//   - The internals of foreign structs outside floatCarrierPackages. A field
//     holding *pgxpool.Pool or *redis.Client reaches connection pools, TLS
//     configuration and retry policies; a float in there is a backoff jitter
//     or a histogram bucket, not money, and descending into it would make this
//     guard's colour depend on the private fields of whatever dependency
//     version is pinned. The only cure for such a false positive would be an
//     exemption list, which is how guards die. A foreign struct that exists to
//     CARRY a value lives in a carrier package and is walked.
//   - Interfaces and function types: a field of type `any` or `func() float64`
//     holds no float statically.
//   - Local variables, function parameters and return values: this is the
//     FIELD-level guard, matching where the schema's numeric columns land.
//   - _test.go files and files behind build tags: packages are loaded without
//     tests, so this guards production code only.
//
// The guard asserts it inspected at least floatGuardMinFields fields. A guard
// that walks nothing passes for the same reason a guard that finds nothing
// does, and this phase has already found seven guards that were weaker than
// their names implied — this one among them, before its reach and its named
// type handling were fixed.
func TestNoFloatFieldsInModelOrStore(t *testing.T) {
	inspected := 0

	for _, pattern := range floatGuardPatterns {
		for _, pkg := range loadTypedPackages(t, pattern) {
			if pkg.TypesInfo == nil {
				t.Errorf("%s: loaded without type information; the float guard cannot inspect it", pkg.PkgPath)
				continue
			}
			for _, file := range pkg.Syntax {
				ast.Inspect(file, func(n ast.Node) bool {
					st, ok := n.(*ast.StructType)
					if !ok || st.Fields == nil {
						return true
					}
					for _, field := range st.Fields.List {
						typ := pkg.TypesInfo.TypeOf(field.Type)
						if typ == nil {
							// A field whose type did not resolve is not a
							// pass: it is a hole in the guard, and
							// loadTypedPackages has already failed the test
							// on any load error that could cause one.
							t.Errorf("%s: could not resolve the type of field %s",
								pkg.Fset.Position(field.Pos()), fieldNames(field))
							continue
						}
						inspected++
						if kind, isFloat := floatKindIn(typ); isFloat {
							t.Errorf("%s: field %s in %s resolves to %s: "+
								"money and energy are numeric in SQL and decimal.Decimal in Go, "+
								"never a float — a float64 holds ~15-17 significant decimal digits "+
								"and this schema's numeric(18,6) and numeric(18,8) columns hold 18",
								pkg.Fset.Position(field.Pos()), fieldNames(field), pkg.PkgPath, kind)
						}
					}
					return true
				})
			}
		}
	}

	require.GreaterOrEqual(t, inspected, floatGuardMinFields,
		"inspected implausibly few struct fields (%d, floor %d): the guard walked far less than "+
			"it did when the floor was measured and would pass whatever the code said",
		inspected, floatGuardMinFields)
	t.Logf("inspected %d struct field(s) across %v for float types", inspected, floatGuardPatterns)
}

// fieldNames renders a field's names for an error message, covering the
// embedded case where there are none.
func fieldNames(field *ast.Field) string {
	if len(field.Names) == 0 {
		return "<embedded>"
	}
	names := make([]string, 0, len(field.Names))
	for _, ident := range field.Names {
		names = append(names, ident.Name)
	}
	return strings.Join(names, ", ")
}

// floatKindIn reports whether typ is, or contains, a float32 or float64, and
// if so names the float kind and the named types it was reached through. See
// TestNoFloatFieldsInModelOrStore for exactly what is followed and why.
//
// The `seen` set, keyed by the fully qualified type string, stops recursion
// through self-referential named types (`type node struct{ next *node }`) and
// avoids re-walking a type reached along two paths. Keying by string rather
// than *types.Named identity matters for generics: two instantiations of the
// same generic type are not guaranteed to share a pointer.
func floatKindIn(typ types.Type) (string, bool) {
	return floatKindInSeen(typ, map[string]bool{})
}

func floatKindInSeen(typ types.Type, seen map[string]bool) (string, bool) {
	if typ == nil {
		return "", false
	}

	switch t := types.Unalias(typ).(type) {
	case *types.Basic:
		// Untyped float constants cannot be a field type, so Float32/Float64
		// are the only two kinds that matter here.
		if t.Kind() == types.Float32 || t.Kind() == types.Float64 {
			return t.Name(), true
		}
		return "", false
	case *types.Pointer:
		return floatKindInSeen(t.Elem(), seen)
	case *types.Slice:
		return floatKindInSeen(t.Elem(), seen)
	case *types.Array:
		return floatKindInSeen(t.Elem(), seen)
	case *types.Chan:
		return floatKindInSeen(t.Elem(), seen)
	case *types.Map:
		if kind, ok := floatKindInSeen(t.Key(), seen); ok {
			return kind, true
		}
		return floatKindInSeen(t.Elem(), seen)
	case *types.Struct:
		for i := range t.NumFields() {
			if kind, ok := floatKindInSeen(t.Field(i).Type(), seen); ok {
				return kind, true
			}
		}
		return "", false
	case *types.Named:
		key := types.TypeString(t, nil)
		if seen[key] {
			return "", false
		}
		seen[key] = true

		through := func(kind string) string {
			return kind + " through " + types.TypeString(t, func(p *types.Package) string { return p.Name() })
		}

		if args := t.TypeArgs(); args != nil {
			for i := range args.Len() {
				if kind, ok := floatKindInSeen(args.At(i), seen); ok {
					return through(kind), true
				}
			}
		}

		// For an instantiated generic, Underlying is already substituted, so
		// Box[float64]'s underlying is struct{ V float64 }.
		underlying := t.Underlying()
		if _, isStruct := underlying.(*types.Struct); isStruct && !floatGuardDescendsInto(t) {
			return "", false
		}
		if kind, ok := floatKindInSeen(underlying, seen); ok {
			return through(kind), true
		}
		return "", false
	default:
		// Interfaces, signatures and type parameters hold no float
		// statically; an instantiation's type arguments are checked above.
		return "", false
	}
}

// floatGuardDescendsInto reports whether the float guard walks the fields of
// the named struct type t: yes for any struct declared in this module or in a
// floatCarrierPackages package, no for every other foreign struct.
func floatGuardDescendsInto(t *types.Named) bool {
	pkg := t.Obj().Pkg()
	if pkg == nil {
		return false
	}
	return underPackage(pkg.Path(), modulePath) || floatCarrierPackages[pkg.Path()]
}

// f2guardCallFloatSkip names source files, relative to the module root and
// slash-separated, that TestIntegrationTreesDoNotParseFloats does not walk.
//
// internal/integration/httpx/ratelimit.go (added by Task 2, in Wave B —
// this file does not exist yet when Task 1 runs) legitimately converts a
// time.Duration to golang.org/x/time/rate.Limit, a float64-based type
// required by that library's own rate-limiter API. That is the only float
// in the file, it is not money, energy or anything this guard exists to
// catch, and there is no way to express "rate.Limit is fine but nothing
// else is" other than exempting the one file where it appears. No other
// file is, or should ever be, exempt.
//
// internal/integration/fake/sanitise.go (added by Task 4; integration
// commit ruling) is the fixture-fake's log/response scanner: it decodes an
// arbitrary, unknown-shaped JSON body with json.Decoder.UseNumber() into
// `any` purely to walk the resulting tree looking for PII-shaped keys —
// UseNumber means any JSON number in that tree is held as json.Number, not
// parsed as a float, so the interface-decode-target hazard this guard
// flags does not apply to it. It never touches money, energy or provider
// values: it is test-harness log hygiene, not a decode path any real
// value flows through. No other file is, or should ever be, exempt.
var f2guardCallFloatSkip = map[string]bool{
	"internal/integration/httpx/ratelimit.go": true,
	"internal/integration/fake/sanitise.go":   true,
}

// f2guardIsJSONDecoderRecv reports whether fn is a method whose receiver is
// *encoding/json.Decoder — the precise target of the "(*json.Decoder).Decode"
// half of TestIntegrationTreesDoNotParseFloats, as opposed to some other
// package's unrelated Decode method that also happens to live in a package
// literally named encoding/json (there is none, but matching on the
// receiver type rather than only the package+name pair is the same
// discipline isNamed already applies to isScopeType and isContextType).
func f2guardIsJSONDecoderRecv(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	return isNamed(sig.Recv().Type(), "encoding/json", "Decoder")
}

// f2guardIsJSONNumberRecv reports whether fn is a method whose receiver is
// encoding/json.Number — the target of the (M3) json.Number.Float64 case:
// json.Number exists specifically so a caller can hold a JSON number as a
// string and defer parsing it (typically into decimal.Decimal), and calling
// its own Float64() throws that away, parsing straight into the float this
// whole guard tree exists to keep out of money/energy/provider values.
func f2guardIsJSONNumberRecv(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	return isNamed(sig.Recv().Type(), "encoding/json", "Number")
}

// f2guardJSONTargetHazard reports whether typ — the static type of an
// argument passed to json.Unmarshal or (*json.Decoder).Decode — is, or
// contains, a float32/float64 or an interface type. It is reached through
// pointers, slices, arrays, maps (key AND element) and struct fields, and
// through a named type's underlying type, mirroring floatKindInSeen's
// traversal above but with one addition: encoding/json decodes a JSON
// number straight into a float64 the moment the target is `any`/
// `interface{}` (map[string]any, a bare `any` field, …) without a single
// struct field ever spelling "float" — a route TestNoFloatFieldsInModelOrStore
// cannot see because it inspects field TYPES, not decode CALL SITES.
func f2guardJSONTargetHazard(typ types.Type, seen map[string]bool) (string, bool) {
	if typ == nil {
		return "", false
	}
	switch t := types.Unalias(typ).(type) {
	case *types.Basic:
		if t.Kind() == types.Float32 || t.Kind() == types.Float64 {
			return t.Name(), true
		}
		return "", false
	case *types.Interface:
		return "an interface type (any JSON number decoded into it becomes an untyped float64)", true
	case *types.Pointer:
		return f2guardJSONTargetHazard(t.Elem(), seen)
	case *types.Slice:
		return f2guardJSONTargetHazard(t.Elem(), seen)
	case *types.Array:
		return f2guardJSONTargetHazard(t.Elem(), seen)
	case *types.Map:
		if kind, ok := f2guardJSONTargetHazard(t.Key(), seen); ok {
			return kind, true
		}
		return f2guardJSONTargetHazard(t.Elem(), seen)
	case *types.Struct:
		for i := range t.NumFields() {
			if kind, ok := f2guardJSONTargetHazard(t.Field(i).Type(), seen); ok {
				return kind, true
			}
		}
		return "", false
	case *types.Named:
		key := types.TypeString(t, nil)
		if seen[key] {
			return "", false
		}
		seen[key] = true
		if args := t.TypeArgs(); args != nil {
			for i := range args.Len() {
				if kind, ok := f2guardJSONTargetHazard(args.At(i), seen); ok {
					return kind, true
				}
			}
		}
		return f2guardJSONTargetHazard(t.Underlying(), seen)
	case *types.TypeParam:
		// I2: a generic decode helper — func decode[T any](b []byte) (T,
		// error) { var v T; return v, json.Unmarshal(b, &v) } — has a
		// json.Unmarshal call whose target's STATIC type, inside the
		// generic function's own body, is *T, a type parameter: the
		// switch above cannot see through it to whatever concrete type an
		// individual call site instantiates T with (that substitution
		// only exists at each instantiation's call site, not inside the
		// generic body being walked here). Flagging every type parameter
		// as a hazard is the conservative fix the finding names as an
		// accepted alternative to walking every instantiation's type
		// arguments via TypesInfo.Instances: a generic decode helper in
		// these trees must decode into a concrete, non-generic type (or
		// be written non-generically), never leave T's safety for a caller
		// to get right silently.
		return "a generic type parameter (its instantiation cannot be statically ruled safe here; decode into a concrete type instead)", true
	default:
		return "", false
	}
}

// f2guardCheckJSONTarget resolves arg's static type and fails t if it is a
// hazard under f2guardJSONTargetHazard.
func f2guardCheckJSONTarget(t *testing.T, pkg *packages.Package, arg ast.Expr) {
	t.Helper()
	typ := pkg.TypesInfo.TypeOf(arg)
	if typ == nil {
		t.Errorf("%s: could not resolve the type of this decode target", pkg.Fset.Position(arg.Pos()))
		return
	}
	if kind, bad := f2guardJSONTargetHazard(typ, map[string]bool{}); bad {
		t.Errorf("%s: decode target resolves to %s: money, energy and provider numbers are decimal.Decimal, json.Number or a string end to end, never a float and never an untyped interface",
			pkg.Fset.Position(arg.Pos()), kind)
	}
}

// f2guardCheckFmtScanFloatArgs (M2) flags any fmt.Sscan/Sscanf/Sscanln
// argument whose static type is a pointer to float32/float64 — the same
// hazard TestNoFloatFieldsInModelOrStore forbids in a struct field, reached
// here through a scan target instead.
func f2guardCheckFmtScanFloatArgs(t *testing.T, pkg *packages.Package, args []ast.Expr) {
	t.Helper()
	for _, arg := range args {
		typ := pkg.TypesInfo.TypeOf(arg)
		if typ == nil {
			continue
		}
		ptr, ok := typ.Underlying().(*types.Pointer)
		if !ok {
			continue
		}
		basic, ok := ptr.Elem().Underlying().(*types.Basic)
		if !ok {
			continue
		}
		if basic.Kind() == types.Float32 || basic.Kind() == types.Float64 {
			t.Errorf("%s: fmt.Sscan/Sscanf/Sscanln into a %s: money and energy are decimal.Decimal end to end, never parsed as a float",
				pkg.Fset.Position(arg.Pos()), typ)
		}
	}
}

// TestIntegrationTreesDoNotParseFloats extends the money/energy purity rule
// from struct fields (TestNoFloatFieldsInModelOrStore) to the two routes
// that let a float enter this codebase WITHOUT ever being named in a struct
// field: strconv.ParseFloat, and decoding JSON into a target whose type is,
// or contains, float32/float64 or an untyped interface (which lets
// encoding/json decode a JSON number as float64 with no static type ever
// spelling "float"). It walks every non-test file of the four F2 trees that
// carry provider numbers: internal/integration, internal/ingest,
// internal/marketdata and internal/credentials.
func TestIntegrationTreesDoNotParseFloats(t *testing.T) {
	patterns := []string{
		"./internal/integration/...",
		"./internal/ingest/...",
		"./internal/marketdata/...",
		"./internal/credentials/...",
	}

	filesInspected := 0
	for _, pattern := range patterns {
		for _, pkg := range loadTypedPackages(t, pattern) {
			for _, file := range pkg.Syntax {
				pos := pkg.Fset.Position(file.Pos())
				rel, err := filepath.Rel(repoRoot(t), pos.Filename)
				require.NoError(t, err)
				rel = filepath.ToSlash(rel)
				if f2guardCallFloatSkip[rel] {
					continue
				}
				filesInspected++

				ast.Inspect(file, func(n ast.Node) bool {
					switch node := n.(type) {
					case *ast.SelectorExpr:
						// M2: matching on the *types.Func a bare
						// SelectorExpr resolves to — not only when it is
						// immediately called — catches strconv.ParseFloat
						// and json.Number.Float64 used as a METHOD VALUE,
						// e.g. `pf := strconv.ParseFloat; pf(s, 64)`: at
						// the call site `pf(s, 64)`, call.Fun is a bare
						// *ast.Ident with no static link back to
						// strconv.ParseFloat, but the assignment
						// `strconv.ParseFloat` itself is a SelectorExpr
						// whose Uses[Sel] already resolves to the same
						// *types.Func regardless of whether it is being
						// called or merely referenced — closing the
						// method-value gap the previous CallExpr-only walk
						// left open (carried from Task 1, closed here).
						fn, ok := pkg.TypesInfo.Uses[node.Sel].(*types.Func)
						if !ok || fn.Pkg() == nil {
							return true
						}
						switch {
						case fn.Pkg().Path() == "strconv" && fn.Name() == "ParseFloat":
							t.Errorf("%s: references strconv.ParseFloat: money and energy are decimal.Decimal end to end, never parsed as a float",
								pkg.Fset.Position(node.Pos()))
						case fn.Pkg().Path() == "encoding/json" && fn.Name() == "Float64" && f2guardIsJSONNumberRecv(fn):
							t.Errorf("%s: references json.Number.Float64: decode provider numbers as decimal.Decimal or keep them as json.Number/string, never parse them as a float",
								pkg.Fset.Position(node.Pos()))
						}
					case *ast.CallExpr:
						sel, ok := node.Fun.(*ast.SelectorExpr)
						if !ok {
							return true
						}
						fn, ok := pkg.TypesInfo.Uses[sel.Sel].(*types.Func)
						if !ok || fn.Pkg() == nil {
							return true
						}
						switch {
						case fn.Pkg().Path() == "encoding/json" && fn.Name() == "Unmarshal" && len(node.Args) == 2:
							f2guardCheckJSONTarget(t, pkg, node.Args[1])
						case fn.Pkg().Path() == "encoding/json" && fn.Name() == "Decode" && len(node.Args) == 1 && f2guardIsJSONDecoderRecv(fn):
							f2guardCheckJSONTarget(t, pkg, node.Args[0])
						case fn.Pkg().Path() == "encoding/json" && fn.Name() == "Token" && f2guardIsJSONDecoderRecv(fn):
							t.Errorf("%s: calls (*json.Decoder).Token: token-by-token JSON numbers decode as float64 with no static type to catch — parse provider numbers through a typed struct with decimal.Decimal/json.Number fields instead",
								pkg.Fset.Position(node.Pos()))
						// M2: fmt.Sscan/Sscanf/Sscanln decode straight into
						// whatever pointer type the caller passes — a
						// *float32/*float64 argument is exactly as
						// float-typed as the struct fields
						// TestNoFloatFieldsInModelOrStore already forbids,
						// and plausible for Turkish "1,5"-style provider
						// numbers.
						case fn.Pkg().Path() == "fmt" && (fn.Name() == "Sscan" || fn.Name() == "Sscanf" || fn.Name() == "Sscanln"):
							f2guardCheckFmtScanFloatArgs(t, pkg, node.Args)
						}
					}
					return true
				})
			}
		}
	}

	require.Positive(t, filesInspected,
		"the guard walked zero non-test files across the F2 trees: it would pass whatever the code said")
}

// f2guardTLSDangerousFields are the crypto/tls.Config fields that, set in a
// composite literal, bypass certificate verification: InsecureSkipVerify
// disables it outright, and VerifyPeerCertificate/VerifyConnection replace
// Go's own verification with caller code that can just as easily always
// return nil.
var f2guardTLSDangerousFields = map[string]bool{
	"InsecureSkipVerify":    true,
	"VerifyPeerCertificate": true,
	"VerifyConnection":      true,
}

// f2guardTLSDangerousSelection reports whether selection resolves to one of
// crypto/tls.Config's dangerous fields, checked through the SELECTED
// FIELD's own declaring package and name (selection.Obj()) rather than
// selection.Recv() (the receiver expression's type). This is what also
// catches the field reached through an embedded wrapper struct — `type w
// struct{ tls.Config }; var p w; p.VerifyConnection = f` has Recv() == w
// (the wrapper), never tls.Config, but Obj() is still the *types.Var for
// tls.Config's own VerifyConnection field, promoted through the embedding
// (M1).
func f2guardTLSDangerousSelection(selection *types.Selection) bool {
	if selection.Kind() != types.FieldVal {
		return false
	}
	v, ok := selection.Obj().(*types.Var)
	if !ok || v.Pkg() == nil || v.Pkg().Path() != "crypto/tls" {
		return false
	}
	return f2guardTLSDangerousFields[v.Name()]
}

// f2guardLoadTypedPackagesWithTests is loadTypedPackages' counterpart that
// also loads _test.go files (via packages.Config.Tests), for guards that
// must see test code too — TestNoTLSConfigDisablesVerification's mutation
// must be caught wherever it appears, including in a test fixture that
// builds its own tls.Config. Loading with Tests:true produces extra
// synthetic package variants (the in-package test binary, the external
// _test package); a real file may therefore be visited more than once,
// which only means a real violation is reported more than once, never
// missed.
func f2guardLoadTypedPackagesWithTests(t *testing.T, pattern string) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Dir:   repoRoot(t),
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, pattern)
	require.NoError(t, err)
	require.NotEmpty(t, pkgs, "pattern %q matched no packages: the guard would pass vacuously", pattern)
	for _, pkg := range pkgs {
		for _, pkgErr := range pkg.Errors {
			t.Errorf("loading %s: %v", pkg.PkgPath, pkgErr)
		}
	}
	return pkgs
}

// TestNoTLSConfigDisablesVerification closes the gap
// TestNoTLSVerificationBypass leaves open. That guard is a substring scan
// for "InsecureSkipVerify" in file text, so it misses a tls.Config built
// with the dangerous field set through a variable holding the field's name,
// through reflection, or under any spelling that does not literally contain
// that identifier as a substring of the source text (none exists in Go
// syntax today, but the point of a type-resolving guard is not to depend on
// that staying true). This guard instead resolves every composite literal's
// TYPE through the type checker and inspects its keyed fields directly:
// it fails on ANY internal/ composite literal of crypto/tls.Config, test
// file or not, that sets InsecureSkipVerify, VerifyPeerCertificate or
// VerifyConnection — regardless of what value it sets the field to, because
// even a VerifyPeerCertificate func that happens to always return nil is
// exactly the shape a bypass takes.
func TestNoTLSConfigDisablesVerification(t *testing.T) {
	pkgs := f2guardLoadTypedPackagesWithTests(t, "./internal/...")

	filesInspected := 0
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			filesInspected++
			ast.Inspect(file, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CompositeLit:
					if !isNamed(pkg.TypesInfo.TypeOf(node), "crypto/tls", "Config") {
						return true
					}
					for _, elt := range node.Elts {
						kv, ok := elt.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						key, ok := kv.Key.(*ast.Ident)
						if !ok {
							continue
						}
						if f2guardTLSDangerousFields[key.Name] {
							t.Errorf("%s: tls.Config sets %s: TLS verification is never disabled (06 §1 rule 7, removed-behaviour 16)",
								pkg.Fset.Position(kv.Pos()), key.Name)
						}
					}
				case *ast.AssignStmt:
					// I3: a composite literal only catches a tls.Config
					// built and filled in one expression. `cfg :=
					// &tls.Config{}; cfg.VerifyConnection = f` sets the
					// same dangerous field through a field assignment
					// after construction, which the composite-literal walk
					// above never sees. TypesInfo.Selections resolves the
					// LHS SelectorExpr to the *types.Var field it actually
					// writes — by type, not by the string "tls" appearing
					// anywhere — so this also catches the field being set
					// through a renamed import or a value reached via
					// another pointer to the same struct. M1:
					// f2guardTLSDangerousSelection checks the FIELD's own
					// declaring package (selection.Obj()), not
					// selection.Recv(), which is what additionally catches
					// the field through an embedded wrapper struct (Recv()
					// would report the wrapper type, never tls.Config).
					for _, lhs := range node.Lhs {
						sel, ok := lhs.(*ast.SelectorExpr)
						if !ok {
							continue
						}
						selection, ok := pkg.TypesInfo.Selections[sel]
						if !ok || !f2guardTLSDangerousSelection(selection) {
							continue
						}
						t.Errorf("%s: tls.Config field %s assigned after construction: TLS verification is never disabled (06 §1 rule 7, removed-behaviour 16)",
							pkg.Fset.Position(node.Pos()), sel.Sel.Name)
					}
				case *ast.UnaryExpr:
					// M1: `fp := &c.VerifyPeerCertificate; *fp = ...` never
					// appears as an AssignStmt whose LHS is a
					// SelectorExpr — the LHS is `*fp`, a dereferenced local
					// variable with no static link back to tls.Config — so
					// catching the eventual write would need data-flow
					// analysis this syntax-driven guard does not do.
					// Flagging the ADDRESS-OF expression itself is
					// sufficient: no legitimate, non-bypassing caller has a
					// reason to take the address of one of these fields.
					if node.Op != token.AND {
						return true
					}
					sel, ok := node.X.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					selection, ok := pkg.TypesInfo.Selections[sel]
					if !ok || !f2guardTLSDangerousSelection(selection) {
						return true
					}
					t.Errorf("%s: takes the address of tls.Config field %s: TLS verification is never disabled (06 §1 rule 7, removed-behaviour 16)",
						pkg.Fset.Position(node.Pos()), sel.Sel.Name)
				}
				return true
			})
		}
	}

	require.Positive(t, filesInspected,
		"the guard walked zero files under internal/: it would pass whatever the code said")
}
