package metrics_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var metricName = regexp.MustCompile(`ekokod_[a-z0-9_]+`)

// TestObservabilityReferencesRegisteredMetrics is R464's guard: every ekokod_*
// series the dashboard and the alerts query is one the code registers, so a
// renamed metric cannot leave a panel silently empty or an alert never firing.
func TestObservabilityReferencesRegisteredMetrics(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	registered := map[string]bool{}
	decl := regexp.MustCompile(`(?:Name:\s*|NewDesc\()"(ekokod_[a-z0-9_]+)"`)
	require.NoError(t, filepath.WalkDir(filepath.Join(root, "internal"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, err := os.ReadFile(p)
		for _, m := range decl.FindAllStringSubmatch(string(b), -1) {
			registered[m[1]] = true
		}
		return err
	}))
	require.GreaterOrEqual(t, len(registered), 6, "the guard found the code's metrics")

	files := []string{"deploy/observability/prometheus/alerts.yml", "deploy/observability/grafana/ekokod-overview.json"}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(root, f))
		require.NoError(t, err)
		refs := metricName.FindAllString(string(b), -1)
		require.NotEmpty(t, refs, f)
		for _, ref := range refs {
			base := ref
			for _, suffix := range []string{"_bucket", "_count", "_sum"} {
				base = strings.TrimSuffix(base, suffix)
			}
			require.True(t, registered[base], "%s references %s, which no code registers", f, ref)
		}
	}
}
