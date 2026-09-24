package legacy_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
)

// TestWriteFixtureExtract writes the full fixture as an extract for a rehearsal
// dry run (docs/runbook-migration.md): EKOKOD_WRITE_FIXTURE_EXTRACT=<dir>.
func TestWriteFixtureExtract(t *testing.T) {
	dir := os.Getenv("EKOKOD_WRITE_FIXTURE_EXTRACT")
	if dir == "" {
		t.Skip("EKOKOD_WRITE_FIXTURE_EXTRACT is not set")
	}
	_, err := legacy.Extract(context.Background(), carbonISOSource(t), dir)
	require.NoError(t, err)
}
