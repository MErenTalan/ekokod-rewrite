//go:build integration

package legacy_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// fingerprint is every row of every target table, in a stable order.
func fingerprint(t *testing.T, pool *pgxpool.Pool, tables []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range tables {
		var n int64
		var sum string
		id := pgx.Identifier{table}.Sanitize()
		row := "t::text"
		if table == "operational_messages" {
			row = "(to_jsonb(t) - 'id')::text" // Q-J15: a bigserial, renumbered by each replace
		}
		require.NoError(t, pool.QueryRow(context.Background(),
			fmt.Sprintf(`select count(*), md5(coalesce(string_agg(%s, '|' order by %s), '')) from %s t`, row, row, id)).Scan(&n, &sum), table)
		out[table] = fmt.Sprintf("%d:%s", n, sum)
	}
	return out
}

func transformedDir(t *testing.T, cipher *crypto.Cipher) string {
	t.Helper()
	extract := t.TempDir()
	_, err := legacy.Extract(context.Background(), carbonISOSource(t), extract)
	require.NoError(t, err)
	out := t.TempDir()
	_, err = legacy.Transform(extract, out, legacy.TransformOptions{Keys: legacy.Keys{Primary: "legacy-secret-key"}, Cipher: cipher, Now: now,
		Answers: map[string]map[string]string{legacy.AnswerTariffClass: {building2Hex: "og/industrial/binomial/private"}}})
	require.NoError(t, err)
	return out
}

// TestLoadIsIdempotent is 09 §F14's "every command is idempotent" for load (R414–R416, Q-J13).
func TestLoadIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	cipher, err := crypto.NewCipher(make([]byte, 32))
	require.NoError(t, err)
	dir := transformedDir(t, cipher)
	db := admin.NewLegacyLoader(pool)

	first, err := legacy.Load(ctx, db, dir)
	require.NoError(t, err)
	var tables []string
	for table := range first.Tables {
		tables = append(tables, table)
	}
	before := fingerprint(t, pool, tables)
	second, err := legacy.Load(ctx, db, dir)
	require.NoError(t, err)
	require.Equal(t, before, fingerprint(t, pool, tables), "a second load changes nothing")
	require.Equal(t, first.Tables["companies"].Loaded, second.Tables["companies"].Loaded)

	// Q-J13: the OSOS × 40 analyzer loads, its readings wait for an answer.
	mr := first.Tables["meter_readings"]
	require.Positive(t, mr.Withheld)
	require.Positive(t, mr.Loaded, "the ARIL analyzer's readings load")
	require.NotEmpty(t, first.Warnings)
	var readings int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from meter_readings where analyzer_id = $1`, legacy.ID("analyzers", "64f0000000000000000000a1")).Scan(&readings))
	require.Zero(t, readings)
	_, err = os.Stat(filepath.Join(dir, "load_report.json"))
	require.NoError(t, err)
	for table, want := range map[string]int{"tariffs": 3, "tariff_taxes": 2, "tariff_manual_yekdem": 1, "tariff_templates": 1, "smtp_settings": 1,
		"market_prices_hourly": 2, "yekdem_monthly": 1, "legacy_bills": 2, "legacy_reports": 1, "operational_messages": 2,
		"power_plants": 2, "power_plant_devices": 1, "solar_tariffs": 2, "plant_production_totals": 2, "alarms": 1, "alarm_channels": 2, "alarm_events": 1,
		"carbon_activities": 1, "carbon_selected_activities": 1, "emission_factors": 1, "emission_factor_conversions": 1, "carbon_reports": 1,
		"iso50001_projects": 1, "iso50001_clause_dates": 1, "iso50001_notes": 1} {
		require.Equal(t, want, first.Tables[table].Loaded, table)
	}
	smtp := postgres.NewSMTPRepository(pool, cipher)
	pass, err := smtp.OpenPassword(ctx, store.SystemScope(legacy.ID("companies", companyHex)))
	require.NoError(t, err, "the migrated SMTP password opens through the repository (R419)")
	require.Equal(t, "Şifre!2024", string(pass))

	// The re-sealed secret opens through the repository, under its own AAD.
	company := legacy.ID("companies", companyHex)
	repo := postgres.NewIntegrationRepository(pool, cipher)
	creds, err := repo.ListCredentials(ctx, store.SystemScope(company))
	require.NoError(t, err)
	require.Len(t, creds, 2)
	var opened bool
	for _, c := range creds {
		if len(c.SecretEnc) == 0 {
			continue
		}
		secret, _, err := repo.OpenSecret(ctx, store.SystemScope(company), c.ID)
		require.NoError(t, err)
		require.Equal(t, "Şifre!2024", string(secret))
		opened = true
	}
	require.True(t, opened)
}

func TestLoadRefusesAnUnknownColumn(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summary.json"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "companies.ndjson"), []byte(`{"id":"00000000-0000-0000-0000-000000000001","name":"X","nope":1}`+"\n"), 0o600))
	_, err := legacy.Load(context.Background(), admin.NewLegacyLoader(pool), dir)
	require.ErrorContains(t, err, `no column "nope"`)
}

func TestLoadRefusesADirectoryThatIsNotATransform(t *testing.T) {
	t.Parallel()
	_, err := legacy.Load(context.Background(), admin.NewLegacyLoader(testfixtures.NewIsolatedDB(t)), t.TempDir())
	require.ErrorContains(t, err, "not a transform output")
}
