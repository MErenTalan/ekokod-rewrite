package cli_test

import (
	"bytes"
	"context"
	"testing"

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

func TestSeedPrintsTargetEnvironmentAndMaskedHost(t *testing.T) {
	var out bytes.Buffer
	setValidEnv(t, map[string]string{"EKOKOD_ENV": "development"})

	err := cli.Execute(context.Background(), []string{"seed"}, &out)
	require.NoError(t, err)
	require.Contains(t, out.String(), "development")
	require.Contains(t, out.String(), "localhost:5432")
	require.NotContains(t, out.String(), ":p@", "the DSN password must never be printed")
}

// TestSchedulerCommandExitsCleanlyWhenDisabled exercises the
// config.Scheduler.Enabled gate documented at
// internal/scheduler/scheduler.go:26-28: it is the caller's responsibility
// to honour it, and this must not require a database connection to verify.
func TestSchedulerCommandExitsCleanlyWhenDisabled(t *testing.T) {
	var out bytes.Buffer
	setValidEnv(t, map[string]string{"EKOKOD_SCHEDULER_ENABLED": "false"})

	err := cli.Execute(context.Background(), []string{"scheduler"}, &out)
	require.NoError(t, err)
}
