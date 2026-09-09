package cli_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestEverySubcommandIsRegistered(t *testing.T) {
	root := cli.NewRoot(&bytes.Buffer{})

	registered := map[string]bool{}
	for _, cmd := range root.Commands() {
		registered[cmd.Name()] = true
	}

	for _, name := range []string{"api", "worker", "scheduler", "migrate", "seed", "config:check", "version"} {
		require.True(t, registered[name], "subcommand %q must be registered", name)
	}
}

func TestApiCommandFailsFastOnInvalidConfiguration(t *testing.T) {
	var out bytes.Buffer
	t.Setenv("EKOKOD_DB_URL", "")
	t.Setenv("EKOKOD_ENCRYPTION_KEY", "")

	err := cli.Execute(context.Background(), []string{"api"}, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "EKOKOD_DB_URL")
}

func TestUnknownSubcommandIsAnError(t *testing.T) {
	var out bytes.Buffer
	err := cli.Execute(context.Background(), []string{"definitely-not-a-command"}, &out)
	require.Error(t, err)
}

// setValidEnv sets a complete, valid configuration in the process
// environment so cli.Execute reaches the command's own logic instead of
// failing in config.FromEnv. Mirrors internal/api/router_test.go's
// testConfig.
//
// It is made hermetic (task 9 review, Minor-6) by clearing every ambient
// EKOKOD_* variable not explicitly listed below: without that, a
// developer's own exported EKOKOD_DB_URL/EKOKOD_REDIS_URL (or similar)
// would silently override the values this helper sets, and the test would
// exercise whatever that developer happens to have configured rather than
// the fixed values below.
//
// This helper's DB/Redis values point at unreachable localhost ports on
// purpose, so it must never be combined with a test that actually dials
// them: `api`, `worker`, and an *enabled* `scheduler` all do. It is safe
// with `seed` (opens no connection in F0) and a *disabled* `scheduler`
// (returns before opening one).
func setValidEnv(t *testing.T, overrides map[string]string) {
	t.Helper()
	env := map[string]string{
		"EKOKOD_ENV":                       "development",
		"EKOKOD_PUBLIC_URL":                "http://localhost:3000",
		"EKOKOD_DB_URL":                    "postgres://u:p@localhost:5432/ekokod?sslmode=disable",
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
	for k, v := range overrides {
		env[k] = v
	}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if _, set := env[k]; !set && strings.HasPrefix(k, "EKOKOD_") {
			t.Setenv(k, "")
		}
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

// TestSeedRefusesProductionWithoutYes pins the F0 seed footgun guard: F0
// defines zero datasets, so this is the only behaviour of `ekokod seed`
// that matters yet, and it must never silently run against production. The
// command must refuse before ever touching a database (there is nothing to
// seed in F0), so this test needs no database.
func TestSeedRefusesProductionWithoutYes(t *testing.T) {
	var out bytes.Buffer
	setValidEnv(t, map[string]string{"EKOKOD_ENV": "production"})

	err := cli.Execute(context.Background(), []string{"seed"}, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "production")
	require.Contains(t, out.String(), "production", "the target environment must be printed before refusing")
}

func TestSeedAllowsProductionWithExplicitYes(t *testing.T) {
	var out bytes.Buffer
	setValidEnv(t, map[string]string{"EKOKOD_ENV": "production"})

	err := cli.Execute(context.Background(), []string{"seed", "--yes"}, &out)
	require.NoError(t, err)
}

func TestSeedOnNonProductionRunsWithoutYes(t *testing.T) {
	var out bytes.Buffer
	setValidEnv(t, map[string]string{"EKOKOD_ENV": "development"})

	err := cli.Execute(context.Background(), []string{"seed"}, &out)
	require.NoError(t, err)
	require.Contains(t, out.String(), "no reference datasets", "F0 defines zero datasets")
}

// TestSeedPrintsTargetEnvironmentAndMaskedDatabaseURL pins the switch away
// from the hand-rolled maskedDBHost (task 9 review, Minor-1/Minor-2) to
// reusing config's already-tested redactDSN via cfg.Resolved()'s
// EKOKOD_DB_URL row. That row masks the password but keeps the host and
// database name readable.
func TestSeedPrintsTargetEnvironmentAndMaskedDatabaseURL(t *testing.T) {
	var out bytes.Buffer
	setValidEnv(t, map[string]string{"EKOKOD_ENV": "development"})

	err := cli.Execute(context.Background(), []string{"seed"}, &out)
	require.NoError(t, err)
	require.Contains(t, out.String(), "development")
	require.Contains(t, out.String(), "localhost:5432")
	require.Contains(t, out.String(), "ekokod", "the database name should be visible, not just the host")
	require.NotContains(t, out.String(), ":p@", "the DSN password must never be printed")
}

// TestSchedulerCommandExitsCleanlyWhenDisabled exercises the
// config.Scheduler.Enabled gate documented at
// internal/scheduler/scheduler.go:26-28: it is the caller's responsibility
// to honour it, and this must not require a database connection to verify.
//
// The context carries a 2s timeout (task 9 review, Minor-6), not
// context.Background(): today this test is a real guard because the DSN
// setValidEnv sets points at an unreachable localhost:5432, so a regression
// that moved NewPool above the Enabled gate would surface as a NewPool
// error. But on a machine that happens to have a real Postgres listening on
// 5432, that same regression would make scheduler.Run block forever on a
// context nothing ever cancels, and the test would hang to go test's
// 10-minute panic timeout instead of failing fast and legibly.
func TestSchedulerCommandExitsCleanlyWhenDisabled(t *testing.T) {
	var out bytes.Buffer
	setValidEnv(t, map[string]string{"EKOKOD_SCHEDULER_ENABLED": "false"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cli.Execute(ctx, []string{"scheduler"}, &out)
	require.NoError(t, err)
}
