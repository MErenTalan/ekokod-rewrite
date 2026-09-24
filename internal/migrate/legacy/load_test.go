package legacy_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

type noDB struct{}

func (noDB) Describe(context.Context, string) (admin.LegacyTable, error) {
	return admin.LegacyTable{}, errors.New("the database must not be reached")
}

func (noDB) Upsert(context.Context, string, admin.LegacyTable, []string, [][]byte, bool) (int64, error) {
	return 0, errors.New("the database must not be reached")
}

// A table file load does not know would be skipped silently (R416).
func TestLoadRefusesAFileItDoesNotKnow(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summary.json"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mystery_table.ndjson"), []byte("{}\n"), 0o600))
	_, err := legacy.Load(context.Background(), noDB{}, dir)
	require.ErrorContains(t, err, "mystery_table")
}

// Every file the transform writes is either loaded or a known side file.
func TestLoadKnowsEveryFileTheTransformWrites(t *testing.T) {
	out, _, _ := transformWith(t, carbonISOSource(t), nil)
	entries, err := os.ReadDir(out)
	require.NoError(t, err)
	for _, e := range entries {
		require.NoError(t, legacy.LoadKnows(e.Name()), e.Name())
	}
}
