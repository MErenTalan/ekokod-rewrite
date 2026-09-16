package arch_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// renderForbidden: renderers take domain values only (Task 9).
var renderForbidden = []string{
	modulePath + "/internal/store",
	modulePath + "/internal/service",
	modulePath + "/internal/platform",
}

// TestRenderPackagesImportNoStoreServicePlatform keeps internal/render/...
// free of the store, service and platform layers, transitively.
func TestRenderPackagesImportNoStoreServicePlatform(t *testing.T) {
	pkgs := f2guardLoadPackagesWithDeps(t, "./internal/render/...")
	require.GreaterOrEqual(t, len(pkgs), 3, "the guard walked too few packages to mean anything")
	for _, pkg := range pkgs {
		if chain := f2guardTransitiveOffender(pkg, renderForbidden, map[string]bool{pkg.PkgPath: true}); chain != nil {
			t.Errorf("%s transitively imports a forbidden package via %s", pkg.PkgPath, strings.Join(chain, " -> "))
		}
	}
}
