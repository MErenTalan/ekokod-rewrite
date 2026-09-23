//go:build integration

package cli_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/cli"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestCLISeedLoadsRealDatasets is the end-to-end check that
// TestSeedAllowsProductionWithExplicitYesPastTheGuard, TestSeedOnNonProductionRunsWithoutYes
// and TestSeedPrintsTargetEnvironmentAndMaskedDatabaseURL (commands_test.go)
// deliberately cannot be, since those stay fast and DB-less by pointing at
// an unreachable fixture DSN: this one runs `ekokod seed` against a REAL,
// migrated database and proves the whole command — config, the production
// guard, opening the pool, seed.Load, and the printed per-dataset counts —
// works end to end, and that running it TWICE is safe (the CLI-level
// expression of seed.Load's idempotency, whose full proof lives in
// internal/seed/seed_integration_test.go).
func TestCLISeedLoadsRealDatasets(t *testing.T) {
	dsn := testfixtures.StartPostgres(t)

	env := map[string]string{
		"EKOKOD_ENV":                       "development",
		"EKOKOD_PUBLIC_URL":                "http://localhost:3000",
		"EKOKOD_DB_URL":                    dsn,
		"EKOKOD_REDIS_URL":                 "redis://localhost:6379/0",
		"EKOKOD_ENCRYPTION_KEY":            "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		"EKOKOD_JWT_SIGNING_KEY":           "jwt-signing-key-at-least-32-chars-long!!",
		"EKOKOD_PASSWORD_PEPPER":           "password-pepper-at-least-32-chars-long!!",
		"EKOKOD_DEVICE_FINGERPRINT_SECRET": "device-fingerprint-secret-32-chars-min!!",
		"EKOKOD_STORAGE_ROOT":              "/tmp/ekokod",
		"EKOKOD_EPIAS_USERNAME":            "u",
		"EKOKOD_EPIAS_PASSWORD":            "p",
		"EKOKOD_ML_API_KEY":                "k",
		"EKOKOD_CORS_ORIGINS":              "http://localhost:3000",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}

	var first bytes.Buffer
	require.NoError(t, cli.Execute(context.Background(), []string{"seed"}, &first))
	require.Contains(t, first.String(), "emission factors: 186 rows (196 conversions)")
	require.Contains(t, first.String(), "integration definitions: 3 rows")
	require.Contains(t, first.String(), "national tariff schedule: 0 rows (source data not in repository)")

	// A second run converges rather than duplicating or erroring: prints the
	// SAME counts.
	// R317: the loaded catalogue verifies against the shipped one.
	var verify bytes.Buffer
	require.NoError(t, cli.Execute(context.Background(), []string{"seed", "--only", "emission-factors", "--verify"}, &verify))
	require.Contains(t, verify.String(), "emission factors: 186/186 match")
	pool := testfixtures.NewPool(t, dsn)
	_, err := pool.Exec(context.Background(), `update emission_factors set base_factor = base_factor + 1
		where company_id is null and key = 'grid_electricity_tr_2022'`)
	require.NoError(t, err)
	verify.Reset()
	err = cli.Execute(context.Background(), []string{"seed", "--only", "emission-factors", "--verify"}, &verify)
	require.Error(t, err)
	require.Contains(t, verify.String(), "changed [grid_electricity_tr_2022]")

	var second bytes.Buffer
	require.NoError(t, cli.Execute(context.Background(), []string{"seed"}, &second))
	require.Contains(t, second.String(), "emission factors: 186 rows (196 conversions)")
	require.Contains(t, second.String(), "integration definitions: 3 rows")
	require.Contains(t, second.String(), "national tariff schedule: 0 rows (source data not in repository)")
}
