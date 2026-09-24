package legacy_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

const bcryptHash = "$2b$10$abcdefghijklmnopqrstuuO2Yq1xh9hIYJ0.1m6qYdV7lLrTXb5nS"

func transformSource(t *testing.T) *legacy.MemSource {
	src := fixtureSource(t, t.TempDir())
	src.Add("integrations",
		bson.D{{Key: "_id", Value: oid("64f000000000000000000f01")}, {Key: "type", Value: "OSOS"}, {Key: "subType", Value: "Baskent"}, {Key: "endpoints", Value: bson.D{{Key: "token", Value: "https://osos.example/token"}}}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000000f02")}, {Key: "type", Value: "ARIL"}, {Key: "subType", Value: "Aril"}},
	)
	// Replace the fixture's companies/users with credential- and hash-carrying ones.
	src.Replace("companies", bson.D{{Key: "_id", Value: oid(companyHex)}, {Key: "name", Value: "Acme"},
		{Key: "integrations", Value: bson.A{
			bson.D{{Key: "type", Value: "OSOS"}, {Key: "subType", Value: "Baskent"}, {Key: "username", Value: "acme"}, {Key: "password", Value: vecPrimary}},
			bson.D{{Key: "type", Value: "ARIL"}, {Key: "subType", Value: "Aril"}, {Key: "username", Value: "acme-aril"}},
			bson.D{{Key: "type", Value: "PM5340"}, {Key: "subType", Value: "Nowhere"}, {Key: "password", Value: "x"}},
		}},
		{Key: "vacations", Value: bson.D{{Key: "weekend", Value: bson.A{int32(0), int32(6)}}}}})
	src.Replace("users",
		bson.D{{Key: "_id", Value: oid("64f000000000000000000d01")}, {Key: "name", Value: "Ayşe"}, {Key: "email", Value: " Ayse@Acme.TEST "}, {Key: "phone", Value: "0555"},
			{Key: "password", Value: bcryptHash}, {Key: "passwordHistory", Value: bson.A{bcryptHash}}, {Key: "userType", Value: "company-admin"}, {Key: "company", Value: oid(companyHex)}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000000d02")}, {Key: "name", Value: "X"}, {Key: "email", Value: "x@acme.test"}, {Key: "password", Value: "plaintext"},
			{Key: "userType", Value: "company-admin"}, {Key: "company", Value: oid(companyHex)}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000000d03")}, {Key: "name", Value: "Y"}, {Key: "email", Value: "y@acme.test"}, {Key: "password", Value: bcryptHash},
			{Key: "userType", Value: "superuser"}, {Key: "company", Value: oid(companyHex)}},
	)
	return src
}

func runTransform(t *testing.T, extract string, cipher *crypto.Cipher) (string, legacy.TransformResult) {
	t.Helper()
	out := t.TempDir()
	res, err := legacy.Transform(extract, out, legacy.TransformOptions{Keys: legacy.Keys{Primary: "legacy-secret-key"}, Cipher: cipher, Now: now})
	require.NoError(t, err)
	return out, res
}

func rows(t *testing.T, dir, table string) []map[string]any {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, table+".ndjson"))
	require.NoError(t, err, table)
	defer func() { _ = f.Close() }()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var m map[string]any
		require.NoError(t, json.Unmarshal(sc.Bytes(), &m))
		out = append(out, m)
	}
	return out
}

func TestTransformIsIdempotentAndLosesNothing(t *testing.T) {
	extract := t.TempDir()
	_, err := legacy.Extract(context.Background(), transformSource(t), extract)
	require.NoError(t, err)
	cipher, err := crypto.NewCipher(make([]byte, 32))
	require.NoError(t, err)
	a, res := runTransform(t, extract, cipher)
	b, _ := runTransform(t, extract, cipher)

	// R412: byte-identical, except sealed secrets (random nonces), which open to the same value.
	entries, err := os.ReadDir(a)
	require.NoError(t, err)
	for _, e := range entries {
		if e.Name() == "integration_credentials.ndjson" {
			continue
		}
		x, _ := os.ReadFile(filepath.Join(a, e.Name()))
		y, _ := os.ReadFile(filepath.Join(b, e.Name()))
		require.Equal(t, string(x), string(y), e.Name())
	}
	credA, credB := rows(t, a, "integration_credentials"), rows(t, b, "integration_credentials")
	require.Len(t, credA, 2)
	require.Equal(t, credA[0]["id"], credB[0]["id"])

	// R404: every document is accepted or rejected.
	for collection, tally := range res.Summary {
		require.Positive(t, tally.Accepted+tally.Rejected, collection)
	}
	require.Equal(t, legacy.Tally{Accepted: 1, Rejected: 2}, res.Summary["users"])
	require.Equal(t, legacy.Tally{Accepted: 2, Rejected: 1}, res.Summary["integration_credentials"])
	rejects, err := os.ReadFile(filepath.Join(a, "rejects.ndjson"))
	require.NoError(t, err)
	for _, reason := range []string{"not_bcrypt", "unknown_role", "definition_unknown"} {
		require.Contains(t, string(rejects), `"reason":"`+reason+`"`)
	}
}

func TestTransformedRowsCarryTheNewSchemasShape(t *testing.T) {
	extract := t.TempDir()
	_, err := legacy.Extract(context.Background(), transformSource(t), extract)
	require.NoError(t, err)
	cipher, err := crypto.NewCipher(make([]byte, 32))
	require.NoError(t, err)
	out, _ := runTransform(t, extract, cipher)

	users := rows(t, out, "users")
	require.Equal(t, "ayse@acme.test", users[0]["email"])
	require.Equal(t, "legacy$"+bcryptHash, users[0]["password_hash"], "R145 verifies and upgrades it at login")
	require.Equal(t, "company_admin", users[0]["role"])
	require.Equal(t, legacy.ID("companies", companyHex).String(), users[0]["company_id"])
	require.Len(t, rows(t, out, "user_password_history"), 1)

	require.Len(t, rows(t, out, "company_weekend_days"), 2)
	cred := rows(t, out, "integration_credentials")[0]
	token := cred["secret_enc"].(string)
	opened, err := cipher.Open(token, postgres.IntegrationCredentialAAD(uuid.MustParse(cred["company_id"].(string)), uuid.MustParse(cred["definition_id"].(string))))
	require.NoError(t, err)
	require.Equal(t, "Şifre!2024", string(opened))
	require.Equal(t, legacy.ID("integrations", "64f000000000000000000f01").String(), cred["definition_id"])

	analyzers := rows(t, out, "analyzers")
	require.Len(t, analyzers, 2)
	require.Equal(t, "osos", analyzers[0]["provider"])
	require.Equal(t, "40", analyzers[0]["meter_multiplier"], "the meter's own multiplier")
	require.Equal(t, false, analyzers[0]["multiplier_confirmed"], "load refuses its readings until confirmed (Q-J5)")
	require.Equal(t, legacy.ID("buildings", buildingHex).String(), analyzers[0]["building_id"])
	require.NotEmpty(t, rows(t, out, "meter_readings"))
	require.Len(t, rows(t, out, "consumption_anomalies"), 1, "the fixture's index drop")

	manual, err := os.ReadFile(filepath.Join(out, "manual_multipliers.csv"))
	require.NoError(t, err)
	require.Contains(t, string(manual), "64f0000000000000000000a1", "OSOS × 40 needs a hand decision (Q-J5)")
	require.Equal(t, 2, strings.Count(string(manual), "\n"), "a header and one analyzer")

	ids := rows(t, out, "legacy_ids")
	require.NotEmpty(t, ids)
	require.Contains(t, ids, map[string]any{"collection": "analyzers", "legacy_id": "64f0000000000000000000a1", "table_name": "analyzers", "new_id": legacy.ID("analyzers", "64f0000000000000000000a1").String()})
}

func TestTransformNeedsTheLegacyKey(t *testing.T) {
	cipher, err := crypto.NewCipher(make([]byte, 32))
	require.NoError(t, err)
	_, err = legacy.Transform(t.TempDir(), t.TempDir(), legacy.TransformOptions{Cipher: cipher, Now: now})
	require.ErrorContains(t, err, "legacy key")
}
