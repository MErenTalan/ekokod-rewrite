package arch_test

import (
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
	return obj.Pkg().Path() == modulePath+"/internal/store" && obj.Name() == "Scope"
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
// The check walks every EXPORTED method declared on an EXPORTED named type
// whose name ends in "Repository", under ./internal/store/postgres/..., and
// requires that at least one parameter resolve to store.Scope. Methods with
// value and pointer receivers are both covered: types.Named.Method reports
// the methods declared with that named type as receiver either way.
//
// internal/store/postgres/admin is skipped. It is the deliberate, and the
// only, unscoped surface in the system — the exemption is matched with
// underPackage so that a future sibling package such as "…/postgres/adminui"
// cannot inherit it by prefix.
//
// No repositories exist yet, so today this guard passes over an empty set.
// That is expected: tasks 9-11 populate it. Its failure mode has been
// exercised by hand (see the task report) precisely because a guard that has
// never been seen to fail is not a guard.
func TestEveryStoreMethodIsScoped(t *testing.T) {
	const adminPkg = modulePath + "/internal/store/postgres/admin"

	for _, pkg := range loadTypedPackages(t, "./internal/store/postgres/...") {
		if underPackage(pkg.PkgPath, adminPkg) {
			continue
		}
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			typeName, ok := obj.(*types.TypeName)
			if !ok || !typeName.Exported() || !strings.HasSuffix(typeName.Name(), "Repository") {
				continue
			}
			named, ok := types.Unalias(typeName.Type()).(*types.Named)
			if !ok {
				continue
			}
			for i := range named.NumMethods() {
				method := named.Method(i)
				if !method.Exported() {
					continue
				}
				sig, ok := method.Type().(*types.Signature)
				if !ok {
					continue
				}
				scoped := false
				params := sig.Params()
				for j := range params.Len() {
					if isScopeType(params.At(j).Type()) {
						scoped = true
						break
					}
				}
				if !scoped {
					t.Errorf("%s.%s.%s has no store.Scope parameter: every exported repository method must be tenant-scoped (only %s may be unscoped)",
						pkg.PkgPath, typeName.Name(), method.Name(), adminPkg)
				}
			}
		}
	}
}
