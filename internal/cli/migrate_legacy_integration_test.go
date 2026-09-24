//go:build integration

package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/cli"
	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestMigrateLegacyPipelineThroughTheCLI runs transform → artifacts → load as an
// operator would, twice, against a migrated database (09 §F14: idempotent).
func TestMigrateLegacyPipelineThroughTheCLI(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	storageRoot, legacyRoot := t.TempDir(), t.TempDir()
	setValidEnv(t, map[string]string{"EKOKOD_LEGACY_ENCRYPTION_KEY": "legacy-secret-key", "EKOKOD_DB_URL": pool.Config().ConnString(),
		"EKOKOD_STORAGE_ROOT": storageRoot})

	src := legacy.NewMemSource()
	company, _ := bson.ObjectIDFromHex("64f000000000000000000c01")
	building, _ := bson.ObjectIDFromHex("64f000000000000000000b01")
	src.Add("companies", bson.D{{Key: "_id", Value: company}, {Key: "name", Value: "Acme"}})
	src.Add("buildings", bson.D{{Key: "_id", Value: building}, {Key: "company_id", Value: company}, {Key: "name", Value: "Merkez"},
		{Key: "billHistory", Value: bson.D{{Key: "2026-08", Value: bson.D{{Key: "totalCost", Value: 10.5}, {Key: "pdfPath", Value: "/opt/bills/a.pdf"}}}}}})
	extract := t.TempDir()
	_, err := legacy.Extract(context.Background(), src, extract)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(legacyRoot, "opt", "bills"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyRoot, "opt", "bills", "a.pdf"), []byte("%PDF-1.4 a"), 0o600))

	dir := t.TempDir()
	for run := 0; run < 2; run++ {
		var out bytes.Buffer
		require.NoError(t, cli.Execute(context.Background(), []string{"migrate", "legacy", "transform", "--extract", extract, "--out", dir, "--now", "2026-09-24T12:00:00Z"}, &out))
		out.Reset()
		require.NoError(t, cli.Execute(context.Background(), []string{"migrate", "legacy", "artifacts", "--dir", dir, "--source", legacyRoot}, &out))
		require.Contains(t, out.String(), `"legacy_bill": 1`)
		out.Reset()
		require.NoError(t, cli.Execute(context.Background(), []string{"migrate", "legacy", "load", "--dir", dir}, &out), out.String())
		require.Contains(t, out.String(), `"legacy_bills"`)
	}
	var bills, files int
	require.NoError(t, pool.QueryRow(context.Background(), `select (select count(*) from legacy_bills), (select count(*) from stored_files where owner_type = 'legacy_bill')`).Scan(&bills, &files))
	require.Equal(t, []int{1, 1}, []int{bills, files}, "twice through the pipeline, once in the database")
	_, err = os.Stat(filepath.Join(dir, "load_report.json"))
	require.NoError(t, err)
}
