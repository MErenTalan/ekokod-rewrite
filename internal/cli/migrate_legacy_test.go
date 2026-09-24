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
)

func legacyExtract(t *testing.T) string {
	t.Helper()
	src := legacy.NewMemSource()
	id, _ := bson.ObjectIDFromHex("64f000000000000000000c01")
	src.Add("companies", bson.D{{Key: "_id", Value: id}, {Key: "name", Value: "Acme"}})
	dir := t.TempDir()
	_, err := legacy.Extract(context.Background(), src, dir)
	require.NoError(t, err)
	return dir
}

func TestMigrateLegacyInventoryReadsAnExtract(t *testing.T) {
	var out bytes.Buffer
	err := cli.Execute(context.Background(), []string{"migrate", "legacy", "inventory", "--from-extract", legacyExtract(t)}, &out)
	require.NoError(t, err)
	require.Contains(t, out.String(), "companies: 1")
}

func TestMigrateLegacyTransformNeedsTheLegacyKey(t *testing.T) {
	setValidEnv(t, map[string]string{"EKOKOD_LEGACY_ENCRYPTION_KEY": ""})
	var out bytes.Buffer
	err := cli.Execute(context.Background(), []string{"migrate", "legacy", "transform", "--extract", legacyExtract(t), "--out", t.TempDir()}, &out)
	require.ErrorContains(t, err, "legacy key")
}

func TestMigrateLegacyTransformWritesTheStagingFiles(t *testing.T) {
	setValidEnv(t, map[string]string{"EKOKOD_LEGACY_ENCRYPTION_KEY": "legacy-secret-key"})
	dest := t.TempDir()
	var out bytes.Buffer
	err := cli.Execute(context.Background(), []string{"migrate", "legacy", "transform", "--extract", legacyExtract(t), "--out", dest, "--now", "2026-09-24T12:00:00Z"}, &out)
	require.NoError(t, err)
	require.Contains(t, out.String(), `"companies"`)
	b, err := os.ReadFile(filepath.Join(dest, "companies.ndjson"))
	require.NoError(t, err)
	require.Contains(t, string(b), `"name":"Acme"`)
}
