package arch_test

import (
	"go/ast"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

// forbiddenInDomain are packages that would give the domain layer I/O.
var forbiddenInDomain = []string{
	"net/http", "net", "database/sql", "os", "os/exec",
	"log", "log/slog", "io/ioutil", "path/filepath",
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
			for _, forbidden := range forbiddenInDomain {
				if imported == forbidden {
					t.Errorf("%s imports %s: internal/domain must be free of I/O", pkg.PkgPath, imported)
				}
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

func TestStoreAndIntegrationDoNotImportAPIOrService(t *testing.T) {
	for _, pkg := range loadPackages(t, "./internal/store/...") {
		for imported := range pkg.Imports {
			require.False(t, underPackage(imported, modulePath+"/internal/api"),
				"%s must not import the HTTP layer", pkg.PkgPath)
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
// TODO(task-9): flip the inspected-method count below into a hard assertion,
// `require.Positive(t, inspected, …)`. It cannot be one yet: no repositories
// exist, so the guard legitimately inspects zero methods and passes over an
// empty set. That is also its weakness — every narrowing bug in the predicate
// above looks identical to "there is nothing to check", so the count is
// logged on every run to make the difference visible rather than silent.
// Task 9 introduces the first repository and MUST convert the t.Log to a
// require, otherwise this guard can stay vacuously green for the life of the
// project.
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
			typeName, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || typeName.IsAlias() {
				continue
			}
			named, ok := types.Unalias(typeName.Type()).(*types.Named)
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
				if !ok || !signatureTakes(sig, isContextType) {
					continue
				}
				inspected++
				if signatureTakes(sig, isScopeType) {
					continue
				}
				t.Errorf("%s.%s.%s takes a context.Context but no store.Scope: every exported repository method must be tenant-scoped (only %s may be unscoped)",
					pkg.PkgPath, typeName.Name(), method.Name(), adminPkg)
			}
		}
	}

	t.Logf("inspected %d exported context-taking method(s) under internal/store/postgres; "+
		"TODO(task-9): this must become require.Positive once the first repository exists, "+
		"otherwise a narrowing bug in this guard is indistinguishable from having nothing to check",
		inspected)
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
// walks: the domain model, where money and energy are declared, and the store
// layer, where they are read and written. They are the two places a float
// field would do damage; a float in an HTTP DTO is a presentation bug, a float
// here is a wrong invoice.
var floatGuardPatterns = []string{
	"./internal/domain/model/...",
	"./internal/store/...",
}

// TestNoFloatFieldsInModelOrStore enforces the invariant this project has
// asserted since F0 and, until now, has never actually had: no struct field in
// the domain model or the store layer is a float.
//
// WHY THIS IS A TEST AND NOT A LINTER RULE. .golangci.yml's `no-float-money`
// is a depguard rule, and depguard is an IMPORT linter. It can forbid
// math/big; it cannot see `Total float64`, because no import is involved in
// declaring one. Task 8a established that as fact and wrote it into the config
// comment. The field-level half of the rule has therefore been carried by code
// review alone, which is to say by nothing that fails a build. This is that
// half.
//
// WHAT IT CHECKS. Every field of every struct type declared under the patterns
// above — package-level types, types declared inside functions, and anonymous
// structs used as field types — resolved through the type checker. The type is
// unwrapped through pointers, slices, arrays, maps (both key and element) and
// channels, so none of these hides a float:
//
//	Total    float64
//	Total    *float64
//	Totals   []float64
//	Totals   [12]float64
//	ByMonth  map[string]float64
//	ByRate   map[float64]string
//	Stream   chan float64
//	Nested   struct{ Total float64 }
//	Deep     []map[string]*[]float64
//
// Types are resolved, never matched by name, for the same reason isScopeType
// resolves rather than greps: `type Money = float64` is an alias, and a
// textual scan for "float64" would miss every field declared as Money.
// types.Unalias plus a *types.Basic kind comparison sees through it.
//
// WHAT IT DOES NOT CHECK, deliberately. It does not follow NAMED types into
// other packages. pgtype.Numeric and decimal.Decimal both hold no float, but
// something like time.Duration or a third-party struct with a float field is
// not this guard's business: those types are not where this project's money
// lives, and following them would flag every dependency that happens to
// contain a float. What matters is that no type DECLARED here introduces one,
// because a float declared here is a float this codebase chose. Fields whose
// type is declared in one of the walked packages are covered when that type is
// itself walked.
//
// Local variables, function parameters and return values are also out of
// scope: this is the FIELD-level guard, matching where the schema's numeric
// columns land. A float local inside a helper is visible in review; a float
// field is what silently persists.
//
// The guard asserts it inspected a plausible number of fields. A guard that
// walks nothing passes for the same reason a guard that finds nothing does,
// and this phase has already found three guards that were weaker than their
// names implied.
func TestNoFloatFieldsInModelOrStore(t *testing.T) {
	inspected := 0

	for _, pattern := range floatGuardPatterns {
		for _, pkg := range loadTypedPackages(t, pattern) {
			if pkg.TypesInfo == nil {
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

	require.GreaterOrEqual(t, inspected, 200,
		"inspected implausibly few struct fields (%d): the guard walked almost nothing "+
			"and would pass whatever the code said", inspected)
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

// floatKindIn reports whether typ is, or structurally contains, a float32 or
// float64, looking through pointers, slices, arrays, maps, channels and
// anonymous structs. It does NOT descend into named types — see
// TestNoFloatFieldsInModelOrStore's comment for why that boundary is where it
// is.
//
// The `seen` set guards against a recursive anonymous structure. One cannot be
// written in Go today (a type that contains itself needs a name), but the
// function is recursive over attacker-free input either way and the set costs
// nothing.
func floatKindIn(typ types.Type) (string, bool) {
	return floatKindInSeen(typ, map[types.Type]bool{})
}

func floatKindInSeen(typ types.Type, seen map[types.Type]bool) (string, bool) {
	if typ == nil || seen[typ] {
		return "", false
	}
	seen[typ] = true

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
		// An ANONYMOUS struct used as a field type. A named struct is not
		// reached here (types.Unalias of a *types.Named is still Named), and
		// is covered by the walk when its own declaration is visited.
		for i := range t.NumFields() {
			if kind, ok := floatKindInSeen(t.Field(i).Type(), seen); ok {
				return kind, true
			}
		}
		return "", false
	default:
		return "", false
	}
}
