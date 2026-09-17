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

	for _, name := range []string{"api", "worker", "scheduler", "migrate", "seed", "config:check", "version", "tool", "user"} {
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
// purpose, so it must never be combined with a test that actually needs a
// live connection to succeed: `api`, `worker`, and an *enabled* `scheduler`
// all do. It is safe with a *disabled* `scheduler` (returns before opening
// one) and with `seed` PROVIDED the test only checks behaviour that happens
// before or during the dial (the production --yes guard, and what is
// printed before the dial fails) — `seed` opens a real connection since F1
// (see internal/cli/seed.go), so a test that needs `seed` to actually
// SUCCEED needs a live database instead (see
// internal/cli/seed_integration_test.go).
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

// TestSeedRefusesProductionWithoutYes pins the seed footgun guard: `ekokod
// seed` must never silently run against production. The command must
// refuse before ever touching a database (the guard check runs before
// config.FromEnv's DB URL is ever dialled — see newSeedCmd), so this test
// needs no database despite F1 seed.Load doing real writes.
func TestSeedRefusesProductionWithoutYes(t *testing.T) {
	var out bytes.Buffer
	setValidEnv(t, map[string]string{"EKOKOD_ENV": "production"})

	err := cli.Execute(context.Background(), []string{"seed"}, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "production")
	require.Contains(t, out.String(), "production", "the target environment must be printed before refusing")
}

// TestSeedAllowsProductionWithExplicitYesPastTheGuard pins that
// production + --yes gets PAST the production guard: setValidEnv's DSN
// points at an unreachable localhost:5432 (deliberately, so this test needs
// no real database), so the command still fails — but on a database dial,
// never on the production refusal. Renamed from
// TestSeedAllowsProductionWithExplicitYes (F0, when seed.Load did not exist
// and there was nothing to dial): F1's seed.Load performs real writes, so a
// unit-tier test asserting the WHOLE command succeeds would need a live
// database; TestCLISeedLoadsRealDatasets (seed_integration_test.go) is
// that test. This one stays a fast, DB-less guard check.
func TestSeedAllowsProductionWithExplicitYesPastTheGuard(t *testing.T) {
	var out bytes.Buffer
	setValidEnv(t, map[string]string{"EKOKOD_ENV": "production"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := cli.Execute(ctx, []string{"seed", "--yes"}, &out)
	require.Error(t, err, "the fixture DSN points at an unreachable database")
	require.NotContains(t, err.Error(), "refusing to seed the production database",
		"--yes must have gotten past the production guard; the error must be a dial failure, not the guard")
	require.Contains(t, out.String(), "production", "the target environment is still printed before dialling")
}

// TestSeedOnNonProductionRunsWithoutYes pins that a non-production
// environment never hits the production guard at all: like
// TestSeedAllowsProductionWithExplicitYesPastTheGuard, the fixture DSN is
// unreachable, so the command still errors — but never with the
// production-refusal message.
func TestSeedOnNonProductionRunsWithoutYes(t *testing.T) {
	var out bytes.Buffer
	setValidEnv(t, map[string]string{"EKOKOD_ENV": "development"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := cli.Execute(ctx, []string{"seed"}, &out)
	require.Error(t, err, "the fixture DSN points at an unreachable database")
	require.NotContains(t, err.Error(), "refusing to seed the production database",
		"non-production must never hit the --yes guard at all")
}

// TestSeedPrintsTargetEnvironmentAndMaskedDatabaseURL pins the switch away
// from the hand-rolled maskedDBHost (task 9 review, Minor-1/Minor-2) to
// reusing config's already-tested redactDSN via cfg.Resolved()'s
// EKOKOD_DB_URL row. That row masks the password but keeps the host and
// database name readable. The command still fails overall (the fixture DSN
// is unreachable — deliberately, so this needs no real database), but the
// environment and masked DSN must already be on out before that failure.
func TestSeedPrintsTargetEnvironmentAndMaskedDatabaseURL(t *testing.T) {
	var out bytes.Buffer
	setValidEnv(t, map[string]string{"EKOKOD_ENV": "development"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := cli.Execute(ctx, []string{"seed"}, &out)
	require.Error(t, err)
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
