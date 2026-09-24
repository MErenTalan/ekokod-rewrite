package legacy_test

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
)

func oid(hex string) bson.ObjectID {
	id, err := bson.ObjectIDFromHex(hex)
	if err != nil {
		panic(err)
	}
	return id
}

func memSource() *legacy.MemSource {
	src := legacy.NewMemSource()
	src.Add("companies",
		bson.D{{Key: "_id", Value: oid("64f000000000000000000002")}, {Key: "name", Value: "B Şirketi"}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000000001")}, {Key: "name", Value: "A Şirketi"}, {Key: "vacations", Value: bson.D{{Key: "weekend", Value: bson.A{int32(0), int32(6)}}}}},
	)
	src.Add("users", bson.D{{Key: "_id", Value: oid("64f000000000000000000010")}, {Key: "email", Value: "a@x.test"}})
	return src
}

func TestExtractIsDeterministicOrderedAndChecksummed(t *testing.T) {
	ctx := context.Background()
	a, b := t.TempDir(), t.TempDir()
	m1, err := legacy.Extract(ctx, memSource(), a)
	require.NoError(t, err)
	m2, err := legacy.Extract(ctx, memSource(), b)
	require.NoError(t, err)
	require.Equal(t, m1, m2)
	for _, name := range []string{"companies.ndjson.gz", "users.ndjson.gz", "manifest.json"} {
		x, err := os.ReadFile(filepath.Join(a, name))
		require.NoError(t, err)
		y, err := os.ReadFile(filepath.Join(b, name))
		require.NoError(t, err)
		require.Equal(t, x, y, "%s is byte-identical across runs (R405)", name)
	}

	require.Equal(t, []legacy.ManifestEntry{
		{Collection: "companies", Count: 2, SHA256: sha(t, filepath.Join(a, "companies.ndjson.gz"))},
		{Collection: "users", Count: 1, SHA256: sha(t, filepath.Join(a, "users.ndjson.gz"))},
	}, m1.Collections)

	lines := gunzipLines(t, filepath.Join(a, "companies.ndjson.gz"))
	require.Len(t, lines, 2)
	var first map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	require.Equal(t, map[string]any{"$oid": "64f000000000000000000001"}, first["_id"], "documents in _id order, canonical extended JSON")
	require.Contains(t, lines[0], `"A Şirketi"`)

	docs := 0
	require.NoError(t, legacy.ReadExtract(a, "companies", func(doc bson.M) error {
		docs++
		return nil
	}))
	require.Equal(t, 2, docs)
	require.NoError(t, legacy.VerifyManifest(a), "the checksums verify")
	require.NoError(t, os.WriteFile(filepath.Join(a, "users.ndjson.gz"), []byte("tampered"), 0o600))
	require.Error(t, legacy.VerifyManifest(a), "a changed file fails verification")
}

func sha(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func gunzipLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	var out []string
	sc := bufio.NewScanner(gz)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}

func TestTheMongoGuardRefusesEveryWrite(t *testing.T) {
	for _, cmd := range []string{"find", "getMore", "count", "aggregate", "listCollections", "hello", "isMaster", "ping", "saslStart", "saslContinue", "killCursors", "endSessions", "buildInfo"} {
		require.True(t, legacy.ReadOnlyCommand(cmd), cmd)
	}
	for _, cmd := range []string{"insert", "update", "delete", "findAndModify", "create", "drop", "dropDatabase", "createIndexes", "renameCollection", "bulkWrite", "mapReduce"} {
		require.False(t, legacy.ReadOnlyCommand(cmd), cmd)
	}
	g := legacy.NewGuard()
	g.Observe("find")
	require.NoError(t, g.Err())
	g.Observe("insert")
	require.ErrorContains(t, g.Err(), "insert")
}
