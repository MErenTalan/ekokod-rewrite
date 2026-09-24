package arch_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAPIDoesNotImportStoreImplementations keeps handlers on services (R181):
// internal/api/... may reach internal/store (interfaces, sentinels) but never
// the postgres implementations or the unscoped admin package, directly or
// transitively. internal/apiwire is the one place that constructs them.
func TestAPIDoesNotImportStoreImplementations(t *testing.T) {
	forbidden := []string{modulePath + "/internal/store/postgres"}
	pkgs := f2guardLoadPackagesWithDeps(t, "./internal/api/...")
	require.NotEmpty(t, pkgs)
	for _, pkg := range pkgs {
		if chain := f2guardTransitiveOffender(pkg, forbidden, map[string]bool{pkg.PkgPath: true}); chain != nil {
			t.Errorf("%s reaches a store implementation via %s", pkg.PkgPath, strings.Join(chain, " -> "))
		}
	}
}
