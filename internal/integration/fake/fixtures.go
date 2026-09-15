package fake

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// RequiredCases is the F2 acceptance matrix every adapter test must cover.
var RequiredCases = []string{"success", "empty", "partial", "auth_failure", "malformed", "rate_limited", "pagination"}

// Fixture reads internal/integration/<provider>/testdata/<name> relative to
// the module root.
func Fixture(t *testing.T, provider, name string) []byte {
	t.Helper()
	path := filepath.Join(moduleRoot(t), "internal", "integration", provider, "testdata", name)
	body, err := os.ReadFile(path)
	require.NoError(t, err, "reading fixture %s", path)
	return body
}

// FixtureFile is one fixture file discovered by FixtureFiles.
type FixtureFile struct {
	Path string
	Body []byte
}

// fixtureExtensions are the file types TestFixturesAreSanitised scans (M5:
// .xml/.csv/.txt fixtures are sanitised too, not just .json).
var fixtureExtensions = map[string]bool{
	".json": true,
	".xml":  true,
	".csv":  true,
	".txt":  true,
}

// FixtureFiles globs internal/integration/*/testdata/**/*.{json,xml,csv,txt}
// relative to the module root. It returns nil (never an error) when
// internal/integration does not yet exist or has no provider directories —
// TestFixturesAreSanitised passes vacuously on that empty tree today.
func FixtureFiles(t *testing.T) []FixtureFile {
	t.Helper()

	integrationDir := filepath.Join(moduleRoot(t), "internal", "integration")
	entries, err := os.ReadDir(integrationDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		require.NoError(t, err)
	}

	var files []FixtureFile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		testdataDir := filepath.Join(integrationDir, entry.Name(), "testdata")
		if _, statErr := os.Stat(testdataDir); statErr != nil {
			continue
		}

		walkErr := filepath.WalkDir(testdataDir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !fixtureExtensions[strings.ToLower(filepath.Ext(path))] {
				return nil
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			files = append(files, FixtureFile{Path: path, Body: body})
			return nil
		})
		require.NoError(t, walkErr)
	}
	return files
}

// moduleRoot locates the module root by walking up to go.mod, so Fixture and
// FixtureFiles work regardless of the calling package's own directory depth.
func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("fake: could not find go.mod walking up from %s", dir)
		}
		dir = parent
	}
}
