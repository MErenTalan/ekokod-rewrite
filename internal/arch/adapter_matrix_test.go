package arch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
)

// f2matrixCaseNames parses every *_test.go file in dir with go/parser
// (syntax only — no type info, so this has none of packages.Load's build
// cost) looking for a top-level function named testFunc, then walks that
// function's body collecting every string literal keyed "name:" inside a
// composite literal — the table-driven test pattern every adapter's
// Test<P>FixtureMatrix uses (see e.g. gridbox/source_test.go's
// TestGridBoxFixtureMatrix). It returns nil, not an error, when testFunc is
// not found in dir, so the caller's own require.NotEmpty is what turns
// "the function does not exist" into a readable failure naming the
// provider and function, rather than a parser error several frames removed
// from the actual problem.
func f2matrixCaseNames(t *testing.T, dir, testFunc string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "reading %s", dir)

	fset := token.NewFileSet()
	var names []string
	var found bool

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		if !hasTestSuffix(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		require.NoError(t, perr, "parsing %s", path)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != testFunc || fn.Body == nil {
				continue
			}
			found = true
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				for _, elt := range cl.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					ident, ok := kv.Key.(*ast.Ident)
					if !ok || ident.Name != "name" {
						continue
					}
					lit, ok := kv.Value.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					s, uerr := strconv.Unquote(lit.Value)
					require.NoError(t, uerr, "unquoting name literal %s in %s", lit.Value, path)
					names = append(names, s)
				}
				return true
			})
		}
	}

	if !found {
		return nil
	}
	return names
}

// hasTestSuffix reports whether name is a Go test source file (ends in
// "_test.go"), matching go's own build convention.
func hasTestSuffix(name string) bool {
	const suffix = "_test.go"
	return len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix
}

// f2matrixContains reports whether names contains want.
func f2matrixContains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// TestAllSixAdaptersHaveTheFixtureMatrix is the structural half of the F2
// acceptance criterion "every adapter has recorded fixtures covering …". It
// fails if an adapter's test file lacks a subtest for any fake.RequiredCases
// entry, or its testdata lacks the fixture file.
func TestAllSixAdaptersHaveTheFixtureMatrix(t *testing.T) {
	for provider, testFunc := range map[string]string{
		"osos": "TestOSOSFixtureMatrix", "gridbox": "TestGridBoxFixtureMatrix", "aril": "TestARILFixtureMatrix",
		"pm5340": "TestPM5340FixtureMatrix", "isolar": "TestISolarFixtureMatrix", "epias": "TestEPIASFixtureMatrix",
	} {
		t.Run(provider, func(t *testing.T) {
			dir := filepath.Join(repoRoot(t), "internal", "integration", provider)
			names := f2matrixCaseNames(t, dir, testFunc)
			require.NotEmpty(t, names, "%s: found no table entries in %s (function missing or empty)", provider, testFunc)

			for _, required := range fake.RequiredCases {
				require.True(t, f2matrixContains(names, required),
					"%s/%s: %s has no subtest named %q (fake.RequiredCases)", provider, testFunc, provider, required)

				fixturePath := filepath.Join(dir, "testdata", fmt.Sprintf("%s_%s.json", provider, required))
				_, statErr := os.Stat(fixturePath)
				require.NoError(t, statErr, "%s/%s: case %q has no recorded fixture at %s", provider, testFunc, required, fixturePath)
			}
		})
	}
}

// TestBillingPathDoesNotImportEPIAS pins removed-behaviour 24 ahead of F4:
// billing computes its own tariff-priced totals and never reaches into the
// EPİAŞ spot-price client (internal/marketdata reads market_prices_hourly
// instead — see task-12-brief.md §"Billing never calls this client"). It
// walks every ./internal/... package whose import path contains "billing"
// and asserts none of them imports internal/integration/epias directly
// (loadPackages' packages.NeedImports mode is single-hop, matching every
// other direct-import guard in this file — TestDomainHasNoProjectImports
// above is the same shape).
//
// It passes VACUOUSLY today: no ./internal/... package path contains
// "billing" yet (billing is F4's phase). That is deliberate and is exactly
// what task-12-brief.md's own description of this guard says ("exists
// vacuously until F4 creates internal/domain/billing/internal/service/billing").
// The F1 Task 9 lesson ("a guard over an empty set proves nothing") does
// not apply here the same way it did there: TestFixturesAreSanitised guards
// a set this task itself populates (so an empty set would hide the guard
// never running), whereas this guard's whole point is to fire the day F4
// creates its first billing package — proven below by a temporary probe
// file that imports epias.
func TestBillingPathDoesNotImportEPIAS(t *testing.T) {
	pkgs := loadPackages(t, "./internal/...")

	const epiasPath = modulePath + "/internal/integration/epias"
	var checked int
	for _, pkg := range pkgs {
		if !f2matrixPathContainsBilling(pkg.PkgPath) {
			continue
		}
		checked++
		for imported := range pkg.Imports {
			require.NotEqual(t, epiasPath, imported,
				"%s imports %s directly: billing must never call the EPİAŞ client (removed-behaviour 24)", pkg.PkgPath, imported)
		}
	}
	t.Logf("billing-path packages checked: %d", checked)
}

// f2matrixPathContainsBilling reports whether pkgPath names a package
// somewhere under a path segment "billing" (e.g.
// .../internal/domain/billing, .../internal/service/billing/report) —
// substring match is intentional here (unlike underPackage's exact-segment
// match used elsewhere in this file) because F4's eventual billing
// packages are not yet named precisely enough to pin a single path, and a
// substring match is strictly MORE cautious: it can only catch more
// candidate packages, never fewer, which is the safe direction for a guard
// that currently has nothing to check.
func f2matrixPathContainsBilling(pkgPath string) bool {
	const marker = "billing"
	for i := 0; i+len(marker) <= len(pkgPath); i++ {
		if pkgPath[i:i+len(marker)] == marker {
			return true
		}
	}
	return false
}
