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

// f2matrixCase is one Test<P>FixtureMatrix table entry: its "name: ..."
// string plus every string literal anywhere inside that same table entry
// (including inside its nested "routes" func literal body) — I1: the
// literals list is what lets the guard require a case to actually NAME the
// fixture file it must serve, not merely have that file exist somewhere on
// disk (a file could exist only to satisfy an existence check while every
// case reuses a different fixture, or serves none at all).
type f2matrixCase struct {
	name     string
	literals []string
}

// f2matrixCases parses every *_test.go file in dir with go/parser (syntax
// only — no type info, so this has none of packages.Load's build cost)
// looking for a top-level function named testFunc, then walks that
// function's body collecting every table-driven case literal — the pattern
// every adapter's Test<P>FixtureMatrix uses (see e.g. gridbox/source_test.go's
// TestGridBoxFixtureMatrix). It returns nil, not an error, when testFunc is
// not found in dir, so the caller's own require.NotEmpty is what turns "the
// function does not exist" into a readable failure naming the provider and
// function, rather than a parser error several frames removed from the
// actual problem.
func f2matrixCases(t *testing.T, dir, testFunc string) []f2matrixCase {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "reading %s", dir)

	fset := token.NewFileSet()
	var cases []f2matrixCase
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
				name, hasName := f2matrixNameOf(t, cl, path)
				if !hasName {
					return true
				}
				cases = append(cases, f2matrixCase{
					name:     name,
					literals: f2matrixStringLiterals(t, cl, path),
				})
				return true
			})
		}
	}

	if !found {
		return nil
	}
	return cases
}

// f2matrixNameOf reports the "name: ..." string literal cl carries as a
// direct key-value element, if any — i.e. whether cl is itself a
// Test<P>FixtureMatrix table-entry literal, not some unrelated nested
// literal (e.g. a fake.Route{} built inside that entry's own routes func).
func f2matrixNameOf(t *testing.T, cl *ast.CompositeLit, path string) (string, bool) {
	t.Helper()
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
		return s, true
	}
	return "", false
}

// f2matrixStringLiterals collects every string literal anywhere inside n's
// subtree, including inside nested func literals — walking a case's own
// composite literal with this reaches every fake.Fixture(...) filename
// referenced by its "routes" closure.
func f2matrixStringLiterals(t *testing.T, n ast.Node, path string) []string {
	t.Helper()
	var out []string
	ast.Inspect(n, func(x ast.Node) bool {
		lit, ok := x.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, uerr := strconv.Unquote(lit.Value)
		require.NoError(t, uerr, "unquoting string literal %s in %s", lit.Value, path)
		out = append(out, s)
		return true
	})
	return out
}

// f2matrixFindCase returns the case named name, if present.
func f2matrixFindCase(cases []f2matrixCase, name string) (f2matrixCase, bool) {
	for _, c := range cases {
		if c.name == name {
			return c, true
		}
	}
	return f2matrixCase{}, false
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
// entry, its testdata lacks the fixture file, or (I1) that case's own table
// entry never actually references the fixture filename it is required to
// serve — a case whose composite literal is silent about
// "<provider>_<case>.json" could be reusing a DIFFERENT fixture (or none at
// all) while an orphaned file of the right name sits in testdata purely to
// satisfy an os.Stat check.
func TestAllSixAdaptersHaveTheFixtureMatrix(t *testing.T) {
	for provider, testFunc := range map[string]string{
		"osos": "TestOSOSFixtureMatrix", "gridbox": "TestGridBoxFixtureMatrix", "aril": "TestARILFixtureMatrix",
		"pm5340": "TestPM5340FixtureMatrix", "isolar": "TestISolarFixtureMatrix", "epias": "TestEPIASFixtureMatrix",
	} {
		t.Run(provider, func(t *testing.T) {
			dir := filepath.Join(repoRoot(t), "internal", "integration", provider)
			cases := f2matrixCases(t, dir, testFunc)
			require.NotEmpty(t, cases, "%s: found no table entries in %s (function missing or empty)", provider, testFunc)

			for _, required := range fake.RequiredCases {
				tc, hasCase := f2matrixFindCase(cases, required)
				require.True(t, hasCase,
					"%s/%s: %s has no subtest named %q (fake.RequiredCases)", provider, testFunc, provider, required)

				fixtureName := fmt.Sprintf("%s_%s.json", provider, required)
				fixturePath := filepath.Join(dir, "testdata", fixtureName)
				_, statErr := os.Stat(fixturePath)
				require.NoError(t, statErr, "%s/%s: case %q has no recorded fixture at %s", provider, testFunc, required, fixturePath)

				require.True(t, f2matrixContains(tc.literals, fixtureName),
					"%s/%s: case %q never references %q inside its own table entry — the file exists on disk but this "+
						"case does not prove it is the one served (I1)", provider, testFunc, required, fixtureName)
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
