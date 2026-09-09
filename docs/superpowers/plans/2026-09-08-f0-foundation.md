# F0 — Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A running, tested, deployable skeleton of the ekokod platform — single Go binary with `api`/`worker`/`scheduler`/`migrate`/`seed`/`config:check`/`version` subcommands, Postgres+TimescaleDB, Redis, asynq queue, leader-elected scheduler, Docker Compose, CI and a Next.js shell — with **no business features**.

**Architecture:** One Go module (`github.com/MErenTalan/ekokod-rewrite`) producing one binary (`ekokod`) run as three processes. `internal/platform` holds config/logging/errors/clock/ids/crypto; `internal/store` holds Postgres (pgx + embedded goose migrations) and Redis; `internal/api` holds the chi router and health/version/metrics endpoints; `internal/job` holds asynq task definitions and handlers; `internal/scheduler` holds Postgres-advisory-lock leader election. `internal/domain` exists but is empty and is guarded by an import-boundary test. The Next.js app lives in `web/` and renders the API's readiness report.

**Tech Stack:** Go 1.27.1 · chi v5.3.2 · pgx/v5 v5.11.0 · goose v3.28.0 · asynq v0.26.0 · go-redis v9 · prometheus/client_golang · cobra · testify · testcontainers-go v0.44.0 · PostgreSQL 16 + TimescaleDB 2.30 · Redis 7.4 · Next.js 15.5.25 · React 19.2.8 · TypeScript 5.9.3 · Tailwind CSS 4.3.3 · next-intl 4.14.2 · pnpm 12.3.4 · Docker Compose

**Spec:** `docs/rewrite/09-implementation-plan.md` §F0, `docs/rewrite/03-target-architecture.md` §2, §4, §7–§10, `docs/rewrite/appendix/env-reference.md`

## Global Constraints

- **Go 1.27.1** (spec floor is 1.23+). Module path `github.com/MErenTalan/ekokod-rewrite`. Binary name `ekokod`.
- **Environment variable prefix is `EKOKOD_`** — `appendix/env-reference.md` uses `BCEM_`; every variable keeps its documented name with the prefix swapped (`BCEM_DB_URL` → `EKOKOD_DB_URL`). No other renaming.
- **Node 24.20.0 LTS**, package manager **pnpm 12.3.4**. Frontend lives in `web/`.
- **PostgreSQL 16 with TimescaleDB 2.30** (`timescale/timescaledb:2.30.0-pg16`). **Redis 7.4** (`redis:7.4.11-alpine`). No feature may depend on a later major version.
- **Money and energy are `decimal`/`numeric`.** A `float64` in a monetary or energy calculation path is a build-breaking defect. (No such code in F0 — the constraint is enforced from F3.)
- **Timestamps are `timestamptz`, stored UTC, evaluated in `Europe/Istanbul`.**
- **`internal/domain` imports nothing from the project** and performs no I/O. Enforced by `internal/arch/arch_test.go` and by golangci-lint `depguard` in CI.
- **HTTP handlers contain no business logic.** Decode, authorise, call a service, encode.
- **No secret is ever logged. No secret has a default value. Startup fails on a missing secret** with a message naming the variable.
- **TLS verification is never disabled.** `InsecureSkipVerify` must not appear anywhere in `internal/`.
- **Turkish is the default locale** (`EKOKOD_DEFAULT_LOCALE=tr`), English fully supported. Every UI string exists in both `web/messages/tr.json` and `web/messages/en.json`; a missing key fails CI.
- **Every task ends green:** `make lint test build` passes.
- **Commit after every task**, conventional-commit messages, ending with the attribution trailers configured for this session.
- Toolchain is installed under `~/.local` and is on `PATH` via `~/.bashrc`: `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"`. Every shell command in this plan assumes that `PATH`.
- Working directory for all commands: `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite`.

---

## File Structure

```
go.mod  go.sum  Makefile  .gitignore  .editorconfig  .golangci.yml  .dockerignore
.env.example  docker-compose.yml  Dockerfile
cmd/ekokod/main.go                          # entrypoint: calls internal/cli.Execute
internal/
  buildinfo/buildinfo.go                    # version/commit/date, injected via -ldflags
  cli/root.go api.go worker.go scheduler.go migrate.go seed.go configcheck.go version.go
  platform/
    config/config.go load.go parse.go resolved.go   # typed config + validation + masked dump
    errors/errors.go                                # machine code + HTTP status + i18n key + cause
    logging/logging.go context.go redact.go         # slog setup, ctx fields, secret redaction
    clock/clock.go fake.go                          # injectable time
    id/id.go                                        # UUIDv7
    crypto/aesgcm.go                                # AES-256-GCM seal/open
    health/health.go                                # named checks + aggregate report
  store/
    postgres/pool.go migrate.go health.go
    postgres/migrations/00001_extensions.sql
    redis/redis.go
  api/router.go health.go version.go
  api/middleware/requestid.go logging.go recoverer.go cors.go ratelimit.go metrics.go
  job/task.go noop.go client.go server.go
  scheduler/elector.go scheduler.go
  domain/doc.go                             # empty package; guarded by arch test
  arch/arch_test.go                         # import-boundary enforcement
web/
  package.json tsconfig.json next.config.ts postcss.config.mjs eslint.config.mjs
  .prettierrc vitest.config.ts Dockerfile
  src/app/layout.tsx src/app/page.tsx src/app/globals.css
  src/components/health-status.tsx src/components/health-status.test.tsx
  src/i18n/request.ts src/lib/api.ts
  messages/tr.json messages/en.json
scripts/offline-bundle.sh scripts/check-i18n-parity.mjs
.github/workflows/ci.yml
```

Responsibilities: one file = one concern. `internal/cli/*.go` only wires; all logic lives in the packages it wires. `internal/platform/*` is dependency-free infrastructure usable by every later phase. `internal/api/middleware/*` is one middleware per file.

---

## Task 1: Repository skeleton, Go module, `version` command

**Files:**
- Create: `go.mod`, `.gitignore`, `.editorconfig`, `.dockerignore`, `Makefile`
- Create: `cmd/ekokod/main.go`, `internal/buildinfo/buildinfo.go`, `internal/cli/root.go`, `internal/cli/version.go`
- Test: `internal/cli/version_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `buildinfo.Info{Version, Commit, Date, Go string}`, `buildinfo.Get() Info`; `cli.Execute(ctx context.Context, args []string, out io.Writer) error`; `cli.NewRoot(out io.Writer) *cobra.Command`.

- [ ] **Step 1: Initialise the module and directory skeleton**

```bash
cd /mnt/c/Users/meren/Desktop/Work/ekokod-rewrite
go mod init github.com/MErenTalan/ekokod-rewrite
mkdir -p cmd/ekokod internal/{buildinfo,cli,domain,arch}
mkdir -p internal/platform/{config,errors,logging,clock,id,crypto,health}
mkdir -p internal/store/postgres/migrations internal/store/redis
mkdir -p internal/api/middleware internal/job internal/scheduler
mkdir -p scripts .github/workflows
```

- [ ] **Step 2: Write the failing test**

`internal/cli/version_test.go`:

```go
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestVersionCommandPrintsJSON(t *testing.T) {
	var out bytes.Buffer
	err := cli.Execute(context.Background(), []string{"version", "--json"}, &out)
	require.NoError(t, err)

	var got map[string]string
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Contains(t, got, "version")
	require.Contains(t, got, "commit")
	require.Contains(t, got, "date")
	require.NotEmpty(t, got["go"])
}

func TestVersionCommandPrintsText(t *testing.T) {
	var out bytes.Buffer
	err := cli.Execute(context.Background(), []string{"version"}, &out)
	require.NoError(t, err)
	require.Contains(t, out.String(), "ekokod")
}
```

- [ ] **Step 3: Run it and watch it fail**

```bash
go test ./internal/cli/... -run TestVersionCommand -v
```
Expected: FAIL — package `cli` does not exist.

- [ ] **Step 4: Implement `internal/buildinfo/buildinfo.go`**

```go
// Package buildinfo exposes the values stamped into the binary at link time.
package buildinfo

import "runtime"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Info describes the running build.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Go      string `json:"go"`
}

// Get returns the build information stamped into this binary.
func Get() Info {
	return Info{Version: version, Commit: commit, Date: date, Go: runtime.Version()}
}
```

- [ ] **Step 5: Implement `internal/cli/root.go` and `internal/cli/version.go`**

`root.go`:

```go
// Package cli wires the ekokod subcommands. It contains no business logic.
package cli

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// NewRoot builds the root command with every subcommand attached.
func NewRoot(out io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "ekokod",
		Short:         "ekokod energy management platform",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(out)
	root.SetErr(out)
	root.AddCommand(newVersionCmd())
	return root
}

// Execute runs the CLI with the given arguments, writing output to out.
func Execute(ctx context.Context, args []string, out io.Writer) error {
	root := NewRoot(out)
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}
```

`version.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := buildinfo.Get()
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				return enc.Encode(info)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "ekokod %s (commit %s, built %s, %s)\n",
				info.Version, info.Commit, info.Date, info.Go)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	return cmd
}
```

`cmd/ekokod/main.go`:

```go
// Command ekokod is the single binary for the ekokod platform.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/MErenTalan/ekokod-rewrite/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := cli.Execute(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ekokod: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Add dependencies and run the tests**

```bash
go get github.com/spf13/cobra@latest github.com/stretchr/testify@latest
go mod tidy
go test ./internal/cli/... -v
```
Expected: PASS.

- [ ] **Step 7: Write `.gitignore`, `.editorconfig`, `.dockerignore`**

`.gitignore`:

```
/bin/
/dist/
/web/node_modules/
/web/.next/
/web/out/
/web/coverage/
.env
.env.local
*.tar
*.tar.gz
coverage.out
```

`.editorconfig`:

```
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true

[*.go]
indent_style = tab

[*.{ts,tsx,js,mjs,json,css,md,yml,yaml}]
indent_style = space
indent_size = 2
```

`.dockerignore`:

```
.git
bin
dist
web/node_modules
web/.next
docs
*.tar
*.tar.gz
```

- [ ] **Step 8: Write the initial `Makefile`**

```makefile
SHELL := /bin/bash
MODULE := github.com/MErenTalan/ekokod-rewrite
BIN := bin/ekokod
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(MODULE)/internal/buildinfo.version=$(VERSION) \
           -X $(MODULE)/internal/buildinfo.commit=$(COMMIT) \
           -X $(MODULE)/internal/buildinfo.date=$(DATE)

.PHONY: build test test-integration lint fmt tidy

build: ## Build the ekokod binary
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/ekokod

test: ## Run unit tests
	go test ./... -race -coverprofile=coverage.out -covermode=atomic

test-integration: ## Run integration tests (requires Docker)
	go test ./... -tags=integration -race -count=1

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy
```

- [ ] **Step 9: Verify build and tests**

```bash
make build && ./bin/ekokod version && make test
```
Expected: binary prints `ekokod dev (commit <sha>, built <date>, go1.27.1)`; tests pass.

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): scaffold go module, cli root and version command

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---

## Task 2: Typed configuration and `config:check`

**Files:**
- Create: `internal/platform/config/config.go`, `parse.go`, `resolved.go`, `load.go`
- Create: `internal/cli/configcheck.go`
- Create: `.env.example`
- Test: `internal/platform/config/config_test.go`, `internal/platform/config/parse_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `config.Load(lookup func(string) (string, bool)) (*config.Config, error)` — accumulates every problem and returns them joined; never panics.
  - `config.FromEnv() (*config.Config, error)` — `Load(os.LookupEnv)`.
  - `config.Config` with fields `Env`, `LogLevel`, `LogFormat`, `Timezone *time.Location`, `DefaultLocale`, `HTTP`, `DB`, `Redis`, `Security`, `Worker`, `Schedule`, `Storage`, `External`, `Features`.
  - `(*config.Config).Resolved() []config.Resolved` where `Resolved{Name, Value string, Secret bool, Source string}` and secret values are already masked.
  - `config.RateLimit{Limit int, Window time.Duration}`.

- [ ] **Step 1: Write the failing tests**

`internal/platform/config/config_test.go`:

```go
package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/stretchr/testify/require"
)

// valid returns a complete, minimal environment.
func valid() map[string]string {
	return map[string]string{
		"EKOKOD_PUBLIC_URL":                "https://ekokod.example.com",
		"EKOKOD_DB_URL":                    "postgres://u:p@localhost:5432/ekokod?sslmode=disable",
		"EKOKOD_REDIS_URL":                 "redis://localhost:6379/0",
		"EKOKOD_ENCRYPTION_KEY":            "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", // 32 bytes
		"EKOKOD_JWT_SIGNING_KEY":           "jwt-signing-key-at-least-32-chars-long!!",
		"EKOKOD_PASSWORD_PEPPER":           "password-pepper-at-least-32-chars-long!!",
		"EKOKOD_DEVICE_FINGERPRINT_SECRET": "device-fingerprint-secret-32-chars-min!!",
		"EKOKOD_STORAGE_ROOT":              "/var/lib/ekokod",
		"EKOKOD_EPIAS_USERNAME":            "epias-user",
		"EKOKOD_EPIAS_PASSWORD":            "epias-pass",
		"EKOKOD_ML_API_KEY":                "ml-api-key",
	}
}

func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}

func TestLoadAppliesDocumentedDefaults(t *testing.T) {
	cfg, err := config.Load(lookupFrom(valid()))
	require.NoError(t, err)

	require.Equal(t, config.EnvProduction, cfg.Env)
	require.Equal(t, "Europe/Istanbul", cfg.Timezone.String())
	require.Equal(t, "tr", cfg.DefaultLocale)
	require.Equal(t, ":8080", cfg.HTTP.Addr)
	require.Equal(t, 25, cfg.DB.MaxConns)
	require.Equal(t, 5, cfg.DB.MinConns)
	require.Equal(t, 30*time.Second, cfg.DB.StatementTimeout)
	require.Equal(t, 1, cfg.Redis.QueueDB)
	require.Equal(t, 10, cfg.Worker.Concurrency)
	require.Equal(t, 5, cfg.Worker.MaxRetries)
	require.Equal(t, 12, cfg.Security.BcryptCost)
	require.Equal(t, 15*time.Minute, cfg.Security.AccessTokenTTL)
	require.True(t, cfg.Scheduler.Enabled)
	require.Equal(t, "0 3 * * *", cfg.Schedule.Ingestion)
	require.Equal(t, config.RateLimit{Limit: 120, Window: time.Minute}, cfg.HTTP.RateLimitAPI)
	require.Equal(t, config.RateLimit{Limit: 5, Window: 15 * time.Minute}, cfg.HTTP.RateLimitAuth)
	require.False(t, cfg.Features.SelfRegistration)
	require.False(t, cfg.Features.PricingPage)
	require.False(t, cfg.Features.SMSAlarms)
}

// TestConfigRejectsMissingSecret is named in the F0 acceptance criteria.
func TestConfigRejectsMissingSecret(t *testing.T) {
	for _, name := range []string{
		"EKOKOD_ENCRYPTION_KEY",
		"EKOKOD_JWT_SIGNING_KEY",
		"EKOKOD_PASSWORD_PEPPER",
		"EKOKOD_DEVICE_FINGERPRINT_SECRET",
		"EKOKOD_DB_URL",
		"EKOKOD_REDIS_URL",
		"EKOKOD_STORAGE_ROOT",
		"EKOKOD_PUBLIC_URL",
		"EKOKOD_EPIAS_PASSWORD",
	} {
		t.Run(name, func(t *testing.T) {
			env := valid()
			delete(env, name)
			_, err := config.Load(lookupFrom(env))
			require.Error(t, err)
			require.Contains(t, err.Error(), name, "the error must name the missing variable")
		})
	}
}

func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	env := valid()
	delete(env, "EKOKOD_DB_URL")
	delete(env, "EKOKOD_REDIS_URL")
	env["EKOKOD_ENV"] = "banana"

	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	msg := err.Error()
	require.Contains(t, msg, "EKOKOD_DB_URL")
	require.Contains(t, msg, "EKOKOD_REDIS_URL")
	require.Contains(t, msg, "EKOKOD_ENV")
}

func TestEncryptionKeyMustBe32Bytes(t *testing.T) {
	env := valid()
	env["EKOKOD_ENCRYPTION_KEY"] = "c2hvcnQ=" // "short"
	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	require.Contains(t, err.Error(), "32 bytes")
}

func TestResolvedMasksSecretsAndNamesSource(t *testing.T) {
	cfg, err := config.Load(lookupFrom(valid()))
	require.NoError(t, err)

	var sawSecret, sawDefault bool
	for _, r := range cfg.Resolved() {
		if r.Secret {
			sawSecret = true
			require.NotContains(t, r.Value, "password-pepper", "secret value must never be printed")
			require.NotContains(t, r.Value, "epias-pass")
			require.Contains(t, r.Value, "•")
		}
		if r.Name == "EKOKOD_HTTP_ADDR" {
			sawDefault = true
			require.Equal(t, "default", r.Source)
		}
		if r.Name == "EKOKOD_DB_URL" {
			require.Equal(t, "env", r.Source)
			require.NotContains(t, r.Value, ":p@", "DSN password must be redacted")
		}
	}
	require.True(t, sawSecret)
	require.True(t, sawDefault)
}

func TestInvalidCronIsRejected(t *testing.T) {
	env := valid()
	env["EKOKOD_SCHEDULE_BILLING"] = "not a cron"
	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	require.Contains(t, err.Error(), "EKOKOD_SCHEDULE_BILLING")
}

func TestWeatherAPIKeyRequiredWhenProviderSet(t *testing.T) {
	env := valid()
	env["EKOKOD_WEATHER_PROVIDER"] = "openweather"
	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	require.Contains(t, err.Error(), "EKOKOD_WEATHER_API_KEY")

	env["EKOKOD_WEATHER_API_KEY"] = "k"
	cfg, err := config.Load(lookupFrom(env))
	require.NoError(t, err)
	require.Equal(t, "openweather", cfg.External.WeatherProvider)
}

func TestSMSFlagRequiresNothingButDefaultsOff(t *testing.T) {
	cfg, err := config.Load(lookupFrom(valid()))
	require.NoError(t, err)
	require.False(t, cfg.Features.SMSAlarms)
	require.False(t, strings.Contains(cfg.String(), "epias-pass"), "String() must never leak a secret")
}
```

`internal/platform/config/parse_test.go`:

```go
package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseRateLimit(t *testing.T) {
	cases := map[string]RateLimit{
		"120/min":   {Limit: 120, Window: time.Minute},
		"5/15min":   {Limit: 5, Window: 15 * time.Minute},
		"1000/hour": {Limit: 1000, Window: time.Hour},
		"10/s":      {Limit: 10, Window: time.Second},
	}
	for in, want := range cases {
		got, err := parseRateLimit(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}

	for _, bad := range []string{"", "120", "abc/min", "120/", "-1/min", "0/min", "120/fortnight"} {
		_, err := parseRateLimit(bad)
		require.Error(t, err, bad)
	}
}

func TestRedactDSN(t *testing.T) {
	require.Equal(t,
		"postgres://user:••••@db:5432/ekokod?sslmode=require",
		redactDSN("postgres://user:s3cret@db:5432/ekokod?sslmode=require"))
	require.Equal(t, "redis://redis:6379/0", redactDSN("redis://redis:6379/0"))
	require.Equal(t, "not a url", redactDSN("not a url"))
}
```

- [ ] **Step 2: Run the tests and watch them fail**

```bash
go test ./internal/platform/config/... -v
```
Expected: FAIL — package does not compile.

- [ ] **Step 3: Implement `internal/platform/config/config.go`**

Define the types. Every variable name matches `appendix/env-reference.md` with the `EKOKOD_` prefix.

```go
// Package config loads and validates the whole application configuration from
// the environment. The process refuses to start on an invalid configuration:
// no secret has a default, and every problem is reported at once.
package config

import (
	"fmt"
	"strings"
	"time"
)

// Environment is the deployment environment.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// LogFormat selects the slog handler.
type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

// RateLimit is a token bucket expressed as "<limit>/<window>", e.g. "120/min".
type RateLimit struct {
	Limit  int
	Window time.Duration
}

// Config is the fully resolved configuration.
type Config struct {
	Env           Environment
	LogLevel      string
	LogFormat     LogFormat
	Timezone      *time.Location
	DefaultLocale string

	HTTP      HTTP
	DB        DB
	Redis     Redis
	Security  Security
	Worker    Worker
	Scheduler Scheduler
	Schedule  Schedule
	Storage   Storage
	External  External
	Features  Features

	resolved []Resolved
}

type HTTP struct {
	Addr           string
	PublicURL      string
	CORSOrigins    []string
	TrustedProxies []string
	RateLimitAPI   RateLimit
	RateLimitAuth  RateLimit
}

type DB struct {
	URL              string
	MaxConns         int
	MinConns         int
	MaxConnLifetime  time.Duration
	StatementTimeout time.Duration
	ReadingRetention time.Duration // zero means unlimited
	CompressionAfter time.Duration
}

type Redis struct {
	URL     string
	CacheDB int
	QueueDB int
}

type Security struct {
	EncryptionKey           []byte // 32 bytes, AES-256-GCM
	JWTSigningKey           []byte
	PasswordPepper          []byte
	DeviceFingerprintSecret []byte
	AccessTokenTTL          time.Duration
	RefreshTokenTTL         time.Duration
	BcryptCost              int
	PasswordHistorySize     int
	LegacyEncryptionKey     []byte // migration only; may be empty
}

type Worker struct {
	Concurrency int
	MaxRetries  int
	Timeout     time.Duration
}

type Scheduler struct {
	Enabled bool
}

// Schedule holds cron expressions, all evaluated in Config.Timezone.
type Schedule struct {
	Ingestion      string
	EPIAS          string
	Alarms         string
	Billing        string
	Forecast       string
	Carbon         string
	ReportsMonthly string
	ReportsYearly  string
}

type Storage struct {
	Root         string
	UploadMax    int64
	AllowedTypes []string
}

type External struct {
	EPIASUsername   string
	EPIASPassword   string
	MLURL           string
	MLAPIKey        string
	MLTimeout       time.Duration
	WeatherProvider string
	WeatherAPIKey   string
	MapTileURL      string
	ISolarRedirect  string
	PinnedCerts     map[string]string // host -> base64(DER)
}

type Features struct {
	SelfRegistration bool
	PricingPage      bool
	SMSAlarms        bool
	Analytics        bool
	Tracing          bool
}

// Resolved returns every configuration variable with its resolved value,
// secrets masked, for `ekokod config:check`.
func (c *Config) Resolved() []Resolved { return c.resolved }

// String renders the configuration with every secret masked. It is safe to log.
func (c *Config) String() string {
	var b strings.Builder
	for _, r := range c.resolved {
		fmt.Fprintf(&b, "%s=%s (%s)\n", r.Name, r.Value, r.Source)
	}
	return b.String()
}
```

`resolved.go`:

```go
package config

import (
	"fmt"
	"net/url"
	"strings"
)

// Resolved is one configuration variable as the process actually sees it.
type Resolved struct {
	Name   string
	Value  string // already masked when Secret is true
	Secret bool
	Source string // "env" or "default"
}

const maskGlyph = "••••••••"

func maskSecret(raw string) string {
	if raw == "" {
		return "(unset)"
	}
	return fmt.Sprintf("%s (len=%d)", maskGlyph, len(raw))
}

// redactDSN removes the password from a URL-shaped DSN, leaving it readable.
func redactDSN(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, hasPassword := u.User.Password(); !hasPassword {
		return raw
	}
	u.User = url.UserPassword(u.User.Username(), "")
	out := u.String()
	// url.String renders an empty password as "user:@host"; make it explicit.
	return strings.Replace(out, ":@", ":"+maskGlyph[:4]+"@", 1)
}
```

- [ ] **Step 4: Implement `internal/platform/config/parse.go`**

```go
package config

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var rateWindows = map[string]time.Duration{
	"s": time.Second, "sec": time.Second, "second": time.Second,
	"m": time.Minute, "min": time.Minute, "minute": time.Minute,
	"h": time.Hour, "hour": time.Hour,
	"d": 24 * time.Hour, "day": 24 * time.Hour,
}

// parseRateLimit parses "<limit>/<count><unit>", e.g. "120/min" or "5/15min".
func parseRateLimit(in string) (RateLimit, error) {
	limitPart, windowPart, ok := strings.Cut(strings.TrimSpace(in), "/")
	if !ok {
		return RateLimit{}, fmt.Errorf("expected <limit>/<window>, got %q", in)
	}
	limit, err := strconv.Atoi(strings.TrimSpace(limitPart))
	if err != nil || limit <= 0 {
		return RateLimit{}, fmt.Errorf("limit must be a positive integer, got %q", limitPart)
	}

	windowPart = strings.TrimSpace(windowPart)
	digits := 0
	for digits < len(windowPart) && windowPart[digits] >= '0' && windowPart[digits] <= '9' {
		digits++
	}
	count := 1
	if digits > 0 {
		count, _ = strconv.Atoi(windowPart[:digits])
	}
	unit, known := rateWindows[strings.ToLower(windowPart[digits:])]
	if !known || count <= 0 {
		return RateLimit{}, fmt.Errorf("unknown window %q in %q", windowPart, in)
	}
	return RateLimit{Limit: limit, Window: time.Duration(count) * unit}, nil
}

// parseKey decodes a base64 key and checks its byte length.
func parseKey(raw string, wantBytes int) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("must be base64: %w", err)
	}
	if len(key) != wantBytes {
		return nil, fmt.Errorf("must decode to exactly %d bytes, got %d", wantBytes, len(key))
	}
	return key, nil
}

// parsePinnedCerts parses "host=base64der,host2=base64der".
func parsePinnedCerts(raw string) (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		host, cert, ok := strings.Cut(pair, "=")
		if !ok || host == "" || cert == "" {
			return nil, fmt.Errorf("expected host=base64der pairs, got %q", pair)
		}
		if _, err := base64.StdEncoding.DecodeString(cert); err != nil {
			return nil, fmt.Errorf("certificate for %q is not base64: %w", host, err)
		}
		out[host] = cert
	}
	return out, nil
}
```

- [ ] **Step 5: Implement `internal/platform/config/load.go`**

The loader accumulates errors, records every resolved value, and validates cron
expressions with `github.com/robfig/cron/v3`.

```go
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

type loader struct {
	lookup   func(string) (string, bool)
	errs     []error
	resolved []Resolved
}

func (l *loader) fail(name string, err error) {
	l.errs = append(l.errs, fmt.Errorf("%s: %w", name, err))
}

func (l *loader) record(name, value string, secret bool, fromEnv bool) {
	source := "default"
	if fromEnv {
		source = "env"
	}
	l.resolved = append(l.resolved, Resolved{Name: name, Value: value, Secret: secret, Source: source})
}

// raw returns the raw value and whether it came from the environment.
func (l *loader) raw(name string) (string, bool) {
	v, ok := l.lookup(name)
	return strings.TrimSpace(v), ok && strings.TrimSpace(v) != ""
}

func (l *loader) str(name, def string) string {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		v = def
	}
	l.record(name, v, false, fromEnv)
	return v
}

func (l *loader) required(name string) string {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.fail(name, errors.New("is required but not set"))
	}
	l.record(name, v, false, fromEnv)
	return v
}

func (l *loader) requiredDSN(name string) string {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.fail(name, errors.New("is required but not set"))
		l.record(name, "", false, false)
		return ""
	}
	if _, err := url.Parse(v); err != nil {
		l.fail(name, fmt.Errorf("is not a valid URL: %w", err))
	}
	l.record(name, redactDSN(v), false, true)
	return v
}

// secret reads a required secret. Secrets never have defaults and are masked.
func (l *loader) secret(name string, minLen int) []byte {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.fail(name, errors.New("is required but not set (secrets have no default)"))
		l.record(name, maskSecret(""), true, false)
		return nil
	}
	if len(v) < minLen {
		l.fail(name, fmt.Errorf("must be at least %d characters", minLen))
	}
	l.record(name, maskSecret(v), true, true)
	return []byte(v)
}

func (l *loader) optionalSecret(name string) []byte {
	v, fromEnv := l.raw(name)
	l.record(name, maskSecret(v), true, fromEnv)
	if !fromEnv {
		return nil
	}
	return []byte(v)
}

// key reads a required base64 key of an exact byte length.
func (l *loader) key(name string, wantBytes int) []byte {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.fail(name, fmt.Errorf("is required but not set (must be base64 of %d bytes)", wantBytes))
		l.record(name, maskSecret(""), true, false)
		return nil
	}
	key, err := parseKey(v, wantBytes)
	if err != nil {
		l.fail(name, err)
	}
	l.record(name, maskSecret(v), true, true)
	return key
}

func (l *loader) intVal(name string, def int) int {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, strconv.Itoa(def), false, false)
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.fail(name, fmt.Errorf("must be an integer, got %q", v))
		n = def
	}
	l.record(name, v, false, true)
	return n
}

func (l *loader) int64Val(name string, def int64) int64 {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, strconv.FormatInt(def, 10), false, false)
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		l.fail(name, fmt.Errorf("must be an integer, got %q", v))
		n = def
	}
	l.record(name, v, false, true)
	return n
}

func (l *loader) boolVal(name string, def bool) bool {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, strconv.FormatBool(def), false, false)
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.fail(name, fmt.Errorf("must be true or false, got %q", v))
		b = def
	}
	l.record(name, v, false, true)
	return b
}

func (l *loader) duration(name string, def time.Duration) time.Duration {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, def.String(), false, false)
		return def
	}
	d, err := parseDuration(v)
	if err != nil {
		l.fail(name, err)
		d = def
	}
	l.record(name, v, false, true)
	return d
}

// optionalDuration returns zero when unset, which callers read as "unlimited".
func (l *loader) optionalDuration(name string) time.Duration {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, "(unlimited)", false, false)
		return 0
	}
	d, err := parseDuration(v)
	if err != nil {
		l.fail(name, err)
	}
	l.record(name, v, false, true)
	return d
}

// parseDuration accepts Go durations plus a day suffix ("90d").
func parseDuration(v string) (time.Duration, error) {
	if strings.HasSuffix(v, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
		if err != nil || days < 0 {
			return 0, fmt.Errorf("must be a duration such as 90d or 1h30m, got %q", v)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("must be a duration such as 30s or 1h, got %q", v)
	}
	return d, nil
}

func (l *loader) enum(name, def string, allowed ...string) string {
	v := l.str(name, def)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	l.fail(name, fmt.Errorf("must be one of %s, got %q", strings.Join(allowed, ", "), v))
	return def
}

func (l *loader) csv(name string, def []string) []string {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, strings.Join(def, ","), false, false)
		return def
	}
	l.record(name, v, false, true)
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (l *loader) cronExpr(name, def string) string {
	v := l.str(name, def)
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := parser.Parse(v); err != nil {
		l.fail(name, fmt.Errorf("is not a valid cron expression: %w", err))
	}
	return v
}

func (l *loader) location(name, def string) *time.Location {
	v := l.str(name, def)
	loc, err := time.LoadLocation(v)
	if err != nil {
		l.fail(name, fmt.Errorf("is not a known timezone: %w", err))
		loc = time.UTC
	}
	return loc
}

func (l *loader) rateLimit(name, def string) RateLimit {
	v := l.str(name, def)
	rl, err := parseRateLimit(v)
	if err != nil {
		l.fail(name, err)
	}
	return rl
}

// FromEnv loads the configuration from the process environment.
func FromEnv() (*Config, error) { return Load(os.LookupEnv) }

// Load reads and validates the configuration, reporting every problem at once.
func Load(lookup func(string) (string, bool)) (*Config, error) {
	l := &loader{lookup: lookup}
	c := &Config{}

	c.Env = Environment(l.enum("EKOKOD_ENV", string(EnvProduction),
		string(EnvDevelopment), string(EnvStaging), string(EnvProduction)))
	c.LogLevel = l.enum("EKOKOD_LOG_LEVEL", "info", "debug", "info", "warn", "error")
	c.LogFormat = LogFormat(l.enum("EKOKOD_LOG_FORMAT", string(LogFormatJSON),
		string(LogFormatJSON), string(LogFormatText)))
	c.Timezone = l.location("EKOKOD_TIMEZONE", "Europe/Istanbul")
	c.DefaultLocale = l.enum("EKOKOD_DEFAULT_LOCALE", "tr", "tr", "en")

	c.HTTP = HTTP{
		Addr:           l.str("EKOKOD_HTTP_ADDR", ":8080"),
		PublicURL:      l.required("EKOKOD_PUBLIC_URL"),
		CORSOrigins:    l.csv("EKOKOD_CORS_ORIGINS", nil),
		TrustedProxies: l.csv("EKOKOD_TRUSTED_PROXIES", nil),
		RateLimitAPI:   l.rateLimit("EKOKOD_RATE_LIMIT_API", "120/min"),
		RateLimitAuth:  l.rateLimit("EKOKOD_RATE_LIMIT_AUTH", "5/15min"),
	}

	c.DB = DB{
		URL:              l.requiredDSN("EKOKOD_DB_URL"),
		MaxConns:         l.intVal("EKOKOD_DB_MAX_CONNS", 25),
		MinConns:         l.intVal("EKOKOD_DB_MIN_CONNS", 5),
		MaxConnLifetime:  l.duration("EKOKOD_DB_MAX_CONN_LIFETIME", time.Hour),
		StatementTimeout: l.duration("EKOKOD_DB_STATEMENT_TIMEOUT", 30*time.Second),
		ReadingRetention: l.optionalDuration("EKOKOD_READING_RETENTION"),
		CompressionAfter: l.duration("EKOKOD_COMPRESSION_AFTER", 90*24*time.Hour),
	}

	c.Redis = Redis{
		URL:     l.requiredDSN("EKOKOD_REDIS_URL"),
		CacheDB: l.intVal("EKOKOD_REDIS_CACHE_DB", 0),
		QueueDB: l.intVal("EKOKOD_REDIS_QUEUE_DB", 1),
	}

	c.Security = Security{
		EncryptionKey:           l.key("EKOKOD_ENCRYPTION_KEY", 32),
		JWTSigningKey:           l.secret("EKOKOD_JWT_SIGNING_KEY", 32),
		PasswordPepper:          l.secret("EKOKOD_PASSWORD_PEPPER", 32),
		DeviceFingerprintSecret: l.secret("EKOKOD_DEVICE_FINGERPRINT_SECRET", 32),
		AccessTokenTTL:          l.duration("EKOKOD_ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:         l.duration("EKOKOD_REFRESH_TOKEN_TTL", 24*time.Hour),
		BcryptCost:              l.intVal("EKOKOD_BCRYPT_COST", 12),
		PasswordHistorySize:     l.intVal("EKOKOD_PASSWORD_HISTORY_SIZE", 5),
		LegacyEncryptionKey:     l.optionalSecret("EKOKOD_LEGACY_ENCRYPTION_KEY"),
	}
	if c.Security.BcryptCost < 12 {
		l.fail("EKOKOD_BCRYPT_COST", errors.New("must be at least 12"))
	}

	c.Worker = Worker{
		Concurrency: l.intVal("EKOKOD_WORKER_CONCURRENCY", 10),
		MaxRetries:  l.intVal("EKOKOD_JOB_MAX_RETRIES", 5),
		Timeout:     l.duration("EKOKOD_JOB_TIMEOUT", 30*time.Minute),
	}
	c.Scheduler = Scheduler{Enabled: l.boolVal("EKOKOD_SCHEDULER_ENABLED", true)}
	c.Schedule = Schedule{
		Ingestion:      l.cronExpr("EKOKOD_SCHEDULE_INGESTION", "0 3 * * *"),
		EPIAS:          l.cronExpr("EKOKOD_SCHEDULE_EPIAS", "0 14 * * *"),
		Alarms:         l.cronExpr("EKOKOD_SCHEDULE_ALARMS", "0 * * * *"),
		Billing:        l.cronExpr("EKOKOD_SCHEDULE_BILLING", "0 5 * * *"),
		Forecast:       l.cronExpr("EKOKOD_SCHEDULE_FORECAST", "0 4 * * *"),
		Carbon:         l.cronExpr("EKOKOD_SCHEDULE_CARBON", "30 4 * * *"),
		ReportsMonthly: l.cronExpr("EKOKOD_SCHEDULE_REPORTS_MONTHLY", "0 6 2 * *"),
		ReportsYearly:  l.cronExpr("EKOKOD_SCHEDULE_REPORTS_YEARLY", "0 7 3 1 *"),
	}

	c.Storage = Storage{
		Root:      l.required("EKOKOD_STORAGE_ROOT"),
		UploadMax: l.int64Val("EKOKOD_UPLOAD_MAX_BYTES", 31457280),
		AllowedTypes: l.csv("EKOKOD_UPLOAD_ALLOWED_TYPES", []string{
			"application/pdf",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.ms-excel",
			"text/csv",
			"image/png",
			"image/jpeg",
		}),
	}

	c.External = External{
		EPIASUsername:   l.required("EKOKOD_EPIAS_USERNAME"),
		EPIASPassword:   string(l.secret("EKOKOD_EPIAS_PASSWORD", 1)),
		MLURL:           l.str("EKOKOD_ML_URL", "http://ml:8000"),
		MLAPIKey:        string(l.secret("EKOKOD_ML_API_KEY", 1)),
		MLTimeout:       l.duration("EKOKOD_ML_TIMEOUT", 60*time.Second),
		WeatherProvider: l.str("EKOKOD_WEATHER_PROVIDER", ""),
		MapTileURL:      l.str("EKOKOD_MAP_TILE_URL", ""),
		ISolarRedirect:  l.str("EKOKOD_ISOLAR_REDIRECT_URL", ""),
	}
	if c.External.WeatherProvider != "" {
		c.External.WeatherAPIKey = string(l.secret("EKOKOD_WEATHER_API_KEY", 1))
	} else {
		l.record("EKOKOD_WEATHER_API_KEY", "(unset)", true, false)
	}
	if raw, ok := l.raw("EKOKOD_PINNED_CERTS"); ok {
		certs, err := parsePinnedCerts(raw)
		if err != nil {
			l.fail("EKOKOD_PINNED_CERTS", err)
		}
		c.External.PinnedCerts = certs
		l.record("EKOKOD_PINNED_CERTS", fmt.Sprintf("%d pinned host(s)", len(certs)), false, true)
	} else {
		l.record("EKOKOD_PINNED_CERTS", "(none)", false, false)
	}

	c.Features = Features{
		SelfRegistration: l.boolVal("EKOKOD_FEATURE_SELF_REGISTRATION", false),
		PricingPage:      l.boolVal("EKOKOD_FEATURE_PRICING_PAGE", false),
		SMSAlarms:        l.boolVal("EKOKOD_FEATURE_SMS_ALARMS", false),
		Analytics:        l.boolVal("EKOKOD_FEATURE_ANALYTICS", false),
		Tracing:          l.boolVal("EKOKOD_FEATURE_TRACING", false),
	}

	c.resolved = l.resolved
	if len(l.errs) > 0 {
		return nil, fmt.Errorf("invalid configuration:\n  %w", errors.Join(l.errs...))
	}
	return c, nil
}
```

- [ ] **Step 6: Add the cron dependency and run the tests**

```bash
go get github.com/robfig/cron/v3@latest
go mod tidy
go test ./internal/platform/config/... -v
```
Expected: PASS.

- [ ] **Step 7: Implement `internal/cli/configcheck.go`**

```go
package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/spf13/cobra"
)

func newConfigCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "config:check",
		Aliases: []string{"config-check"},
		Short:   "Validate the configuration and print every resolved value with secrets masked",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "VARIABLE\tVALUE\tSOURCE")
			for _, r := range cfg.Resolved() {
				fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.Value, r.Source)
			}
			return w.Flush()
		},
	}
}
```

Register it in `root.go`: `root.AddCommand(newConfigCheckCmd())`.

- [ ] **Step 8: Write `.env.example`**

Every variable from the loader, with the documented defaults present and commented, and placeholder values for secrets. Generate the key placeholders with:

```bash
python3 -c "import base64,os;print(base64.b64encode(os.urandom(32)).decode())"
```

```
# Core
EKOKOD_ENV=development
EKOKOD_LOG_LEVEL=debug
EKOKOD_LOG_FORMAT=text
EKOKOD_TIMEZONE=Europe/Istanbul
EKOKOD_DEFAULT_LOCALE=tr

# HTTP
EKOKOD_HTTP_ADDR=:8080
EKOKOD_PUBLIC_URL=http://localhost:3000
EKOKOD_CORS_ORIGINS=http://localhost:3000
EKOKOD_RATE_LIMIT_API=120/min
EKOKOD_RATE_LIMIT_AUTH=5/15min

# Database
EKOKOD_DB_URL=postgres://ekokod:ekokod@postgres:5432/ekokod?sslmode=disable
EKOKOD_DB_MAX_CONNS=25
EKOKOD_DB_MIN_CONNS=5

# Redis
EKOKOD_REDIS_URL=redis://redis:6379/0
EKOKOD_REDIS_CACHE_DB=0
EKOKOD_REDIS_QUEUE_DB=1

# Security — replace every value below before any non-development use
EKOKOD_ENCRYPTION_KEY=<base64 of 32 random bytes>
EKOKOD_JWT_SIGNING_KEY=<random string, at least 32 characters>
EKOKOD_PASSWORD_PEPPER=<random string, at least 32 characters>
EKOKOD_DEVICE_FINGERPRINT_SECRET=<random string, at least 32 characters>

# Storage
EKOKOD_STORAGE_ROOT=/var/lib/ekokod

# External services
EKOKOD_EPIAS_USERNAME=<epias username>
EKOKOD_EPIAS_PASSWORD=<epias password>
EKOKOD_ML_URL=http://ml:8000
EKOKOD_ML_API_KEY=<shared secret with the ML service>
```

- [ ] **Step 9: Verify `config:check` both ways**

```bash
make build
env -i PATH="$PATH" ./bin/ekokod config:check; echo "exit=$?"     # expect non-zero, names every missing variable
set -a && source .env.example && set +a && ./bin/ekokod config:check | head -20
```
Expected: first invocation exits 1 listing `EKOKOD_DB_URL`, `EKOKOD_ENCRYPTION_KEY`, … ; second prints a table with secrets shown as `•••••••• (len=…)`.

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): typed configuration with validation and config:check

Every variable from appendix/env-reference.md under the EKOKOD_ prefix.
Secrets have no defaults, startup reports every problem at once, and
config:check prints resolved values with secrets masked.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---
## Task 3: Errors and structured logging with redaction

**Files:**
- Create: `internal/platform/errors/errors.go`
- Create: `internal/platform/logging/logging.go`, `context.go`, `redact.go`
- Test: `internal/platform/errors/errors_test.go`, `internal/platform/logging/logging_test.go`

**Interfaces:**
- Consumes: `config.Config` (for level and format).
- Produces:
  - `errors.Error{Code string, HTTPStatus int, MessageKey string, Params map[string]any, Cause error}` with `New(code string, status int, messageKey string) *Error`, `Wrap(err error, code string, status int, messageKey string) *Error`, `(*Error).WithParams(map[string]any) *Error`, `(*Error).Error() string`, `(*Error).Unwrap() error`, `(*Error).Is(target error) bool`.
  - Sentinels: `errors.NotFound`, `errors.Unauthorized`, `errors.Forbidden`, `errors.Validation`, `errors.Conflict`, `errors.Unavailable`, `errors.Internal` (each an `*Error` used as a prototype and matched by `Code`).
  - Helpers: `errors.CodeOf(err error) string`, `errors.StatusOf(err error) int`, `errors.MessageKeyOf(err error) string`.
  - `logging.New(level, format string, w io.Writer) *slog.Logger` — installs the redacting, context-aware handler.
  - `logging.WithField(ctx context.Context, key, value string) context.Context` and `logging.Fields(ctx) []slog.Attr`.
  - Constants `logging.FieldRequestID = "request_id"`, `FieldPrincipal = "principal"`, `FieldCompany = "company_id"`, `FieldJob = "job_id"`.

- [ ] **Step 1: Write the failing tests**

`internal/platform/errors/errors_test.go`:

```go
package errors_test

import (
	stderrors "errors"
	"net/http"
	"testing"

	apperrors "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/stretchr/testify/require"
)

func TestErrorCarriesCodeStatusAndMessageKey(t *testing.T) {
	err := apperrors.New("tariff_not_found", http.StatusNotFound, "errors.tariff.notFound").
		WithParams(map[string]any{"buildingId": "b-1"})

	require.Equal(t, "tariff_not_found", apperrors.CodeOf(err))
	require.Equal(t, http.StatusNotFound, apperrors.StatusOf(err))
	require.Equal(t, "errors.tariff.notFound", apperrors.MessageKeyOf(err))
	require.Contains(t, err.Error(), "tariff_not_found")
}

func TestWrapPreservesCause(t *testing.T) {
	cause := stderrors.New("connection refused")
	err := apperrors.Wrap(cause, "db_unavailable", http.StatusServiceUnavailable, "errors.db.unavailable")

	require.ErrorIs(t, err, cause)
	require.Equal(t, "db_unavailable", apperrors.CodeOf(err))
	require.Contains(t, err.Error(), "connection refused")
}

func TestSentinelsMatchByCode(t *testing.T) {
	err := apperrors.New(apperrors.NotFound.Code, http.StatusNotFound, "errors.generic.notFound")
	require.ErrorIs(t, err, apperrors.NotFound)
	require.NotErrorIs(t, err, apperrors.Forbidden)
}

func TestUnknownErrorDefaultsToInternal(t *testing.T) {
	err := stderrors.New("boom")
	require.Equal(t, "internal", apperrors.CodeOf(err))
	require.Equal(t, http.StatusInternalServerError, apperrors.StatusOf(err))
	require.Equal(t, "errors.generic.internal", apperrors.MessageKeyOf(err))
}
```

`internal/platform/logging/logging_test.go`:

```go
package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	"github.com/stretchr/testify/require"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &out))
	return out
}

func TestLoggerRedactsSecretAttributes(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New("info", "json", &buf)

	log.Info("integration configured",
		slog.String("password", "s3cret"),
		slog.String("api_key", "abcdef"),
		slog.String("authorization", "Bearer xyz"),
		slog.String("encryption_key", "aGVsbG8="),
		slog.String("provider", "gridbox"))

	line := buf.String()
	require.NotContains(t, line, "s3cret")
	require.NotContains(t, line, "abcdef")
	require.NotContains(t, line, "Bearer xyz")
	require.NotContains(t, line, "aGVsbG8=")
	require.Contains(t, line, "gridbox")
	require.Contains(t, line, "[REDACTED]")
}

func TestLoggerRedactsNestedGroups(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New("info", "json", &buf)
	log.Info("credentials", slog.Group("credential", slog.String("secret", "hunter2")))
	require.NotContains(t, buf.String(), "hunter2")
}

func TestLoggerEmitsContextFields(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New("debug", "json", &buf)

	ctx := logging.WithField(context.Background(), logging.FieldRequestID, "req-1")
	ctx = logging.WithField(ctx, logging.FieldCompany, "company-9")
	log.InfoContext(ctx, "handled")

	got := decode(t, &buf)
	require.Equal(t, "req-1", got["request_id"])
	require.Equal(t, "company-9", got["company_id"])
}

func TestLevelIsHonoured(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New("warn", "json", &buf)
	log.Info("should not appear")
	require.Empty(t, buf.String())
	log.Warn("should appear")
	require.Contains(t, buf.String(), "should appear")
}
```

- [ ] **Step 2: Run the tests and watch them fail**

```bash
go test ./internal/platform/errors/... ./internal/platform/logging/... -v
```
Expected: FAIL — packages do not exist.

- [ ] **Step 3: Implement `internal/platform/errors/errors.go`**

```go
// Package errors defines the single application error type: a machine code, an
// HTTP status, a localisable message key and a wrapped cause. Handlers map it
// to a response; jobs map it to an operational message. Nothing is swallowed.
package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
)

// Error is the application error type.
type Error struct {
	Code       string
	HTTPStatus int
	MessageKey string
	Params     map[string]any
	Cause      error
}

// New creates an Error with no cause.
func New(code string, status int, messageKey string) *Error {
	return &Error{Code: code, HTTPStatus: status, MessageKey: messageKey}
}

// Wrap creates an Error carrying cause.
func Wrap(cause error, code string, status int, messageKey string) *Error {
	return &Error{Code: code, HTTPStatus: status, MessageKey: messageKey, Cause: cause}
}

// WithParams attaches parameters for message interpolation.
func (e *Error) WithParams(params map[string]any) *Error {
	clone := *e
	clone.Params = params
	return &clone
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return e.Code
}

// Unwrap exposes the cause to errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.Cause }

// Is matches another *Error by code, so sentinels work with errors.Is.
func (e *Error) Is(target error) bool {
	var t *Error
	if !stderrors.As(target, &t) {
		return false
	}
	return t.Code == e.Code
}

// Sentinels. Use them as prototypes: New(NotFound.Code, NotFound.HTTPStatus, key).
var (
	NotFound     = New("not_found", http.StatusNotFound, "errors.generic.notFound")
	Unauthorized = New("unauthorized", http.StatusUnauthorized, "errors.generic.unauthorized")
	Forbidden    = New("forbidden", http.StatusForbidden, "errors.generic.forbidden")
	Validation   = New("validation_failed", http.StatusUnprocessableEntity, "errors.generic.validation")
	Conflict     = New("conflict", http.StatusConflict, "errors.generic.conflict")
	Unavailable  = New("unavailable", http.StatusServiceUnavailable, "errors.generic.unavailable")
	Internal     = New("internal", http.StatusInternalServerError, "errors.generic.internal")
)

func as(err error) *Error {
	var e *Error
	if stderrors.As(err, &e) {
		return e
	}
	return Internal
}

// CodeOf returns the machine code, defaulting to "internal".
func CodeOf(err error) string { return as(err).Code }

// StatusOf returns the HTTP status, defaulting to 500.
func StatusOf(err error) int { return as(err).HTTPStatus }

// MessageKeyOf returns the i18n key, defaulting to the generic internal key.
func MessageKeyOf(err error) string { return as(err).MessageKey }
```

- [ ] **Step 4: Implement `internal/platform/logging/redact.go`**

```go
package logging

import (
	"context"
	"log/slog"
	"strings"
)

// Redacted replaces the value of any attribute whose key looks like a secret.
const Redacted = "[REDACTED]"

// secretKeyParts are matched case-insensitively as substrings of an attribute key.
var secretKeyParts = []string{
	"password", "passwd", "secret", "token", "api_key", "apikey",
	"authorization", "credential", "pepper", "private_key", "encryption_key",
	"signing_key", "session", "cookie", "tgt", "ticket",
}

func isSecretKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range secretKeyParts {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

// redactHandler wraps a handler, redacting secret-looking attributes and
// appending the context fields carried by WithField.
type redactHandler struct{ inner slog.Handler }

func (h redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return redactHandler{inner: h.inner.WithAttrs(redactAttrs(attrs))}
}

func (h redactHandler) WithGroup(name string) slog.Handler {
	return redactHandler{inner: h.inner.WithGroup(name)}
}

func (h redactHandler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	out.AddAttrs(Fields(ctx)...)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

func redactAttrs(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, redactAttr(a))
	}
	return out
}

func redactAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindGroup {
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(redactAttrs(a.Value.Group())...)}
	}
	if isSecretKey(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	return a
}
```

- [ ] **Step 5: Implement `internal/platform/logging/context.go`**

```go
package logging

import (
	"context"
	"log/slog"
)

// Field names propagated through context into every log record.
const (
	FieldRequestID = "request_id"
	FieldPrincipal = "principal"
	FieldCompany   = "company_id"
	FieldJob       = "job_id"
)

type fieldsKey struct{}

// WithField returns a context carrying an additional log field.
func WithField(ctx context.Context, key, value string) context.Context {
	existing := Fields(ctx)
	next := make([]slog.Attr, len(existing), len(existing)+1)
	copy(next, existing)
	next = append(next, slog.String(key, value))
	return context.WithValue(ctx, fieldsKey{}, next)
}

// Fields returns the log fields carried by ctx.
func Fields(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	fields, _ := ctx.Value(fieldsKey{}).([]slog.Attr)
	return fields
}
```

- [ ] **Step 6: Implement `internal/platform/logging/logging.go`**

```go
// Package logging configures the application's structured logger: JSON or text,
// context fields propagated automatically, and secret-looking attributes
// redacted before they reach the output.
package logging

import (
	"io"
	"log/slog"
	"strings"
)

// New builds a logger. level is debug|info|warn|error, format is json|text.
func New(level, format string, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}

	var base slog.Handler
	if strings.EqualFold(format, "text") {
		base = slog.NewTextHandler(w, opts)
	} else {
		base = slog.NewJSONHandler(w, opts)
	}
	return slog.New(redactHandler{inner: base})
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
```

- [ ] **Step 7: Run the tests**

```bash
go test ./internal/platform/errors/... ./internal/platform/logging/... -v
```
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): application error type and redacting structured logger

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---

## Task 4: Clock, identifiers, AES-256-GCM and the health registry

**Files:**
- Create: `internal/platform/clock/clock.go`, `fake.go`
- Create: `internal/platform/id/id.go`
- Create: `internal/platform/crypto/aesgcm.go`
- Create: `internal/platform/health/health.go`
- Test: `internal/platform/clock/clock_test.go`, `internal/platform/id/id_test.go`, `internal/platform/crypto/aesgcm_test.go`, `internal/platform/health/health_test.go`

**Interfaces:**
- Produces:
  - `clock.Clock` interface `{ Now() time.Time; Since(time.Time) time.Duration }`; `clock.System() Clock`; `clock.NewFake(t time.Time) *clock.Fake` with `Advance(d time.Duration)` and `Set(t time.Time)`.
  - `id.New() string` — UUIDv7 as a canonical string; `id.Parse(s string) (uuid.UUID, error)`.
  - `crypto.Cipher` with `crypto.NewCipher(key []byte) (*Cipher, error)`, `(*Cipher).Seal(plaintext, aad []byte) (string, error)`, `(*Cipher).Open(token string, aad []byte) ([]byte, error)`.
  - `health.Check{Name string, Fn func(context.Context) error}`, `health.Result{Name, Status, Error string, DurationMS int64}`, `health.Report{Status string, Checks []Result}`, `health.Run(ctx context.Context, timeout time.Duration, checks ...Check) Report`. `Report.Status` is `"ok"` or `"degraded"`; `Result.Status` is `"ok"` or `"failed"`.

- [ ] **Step 1: Write the failing tests**

`internal/platform/crypto/aesgcm_test.go`:

```go
package crypto_test

import (
	"crypto/rand"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/stretchr/testify/require"
)

func key(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	_, err := rand.Read(k)
	require.NoError(t, err)
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	c, err := crypto.NewCipher(key(t))
	require.NoError(t, err)

	token, err := c.Seal([]byte("gridbox-password"), []byte("company-1"))
	require.NoError(t, err)
	require.NotContains(t, token, "gridbox-password")

	got, err := c.Open(token, []byte("company-1"))
	require.NoError(t, err)
	require.Equal(t, "gridbox-password", string(got))
}

func TestSealProducesDifferentCiphertextEachTime(t *testing.T) {
	c, err := crypto.NewCipher(key(t))
	require.NoError(t, err)

	a, err := c.Seal([]byte("same"), nil)
	require.NoError(t, err)
	b, err := c.Seal([]byte("same"), nil)
	require.NoError(t, err)
	require.NotEqual(t, a, b, "a fresh nonce must be used for every seal")
}

func TestOpenRejectsWrongKey(t *testing.T) {
	c1, err := crypto.NewCipher(key(t))
	require.NoError(t, err)
	c2, err := crypto.NewCipher(key(t))
	require.NoError(t, err)

	token, err := c1.Seal([]byte("secret"), nil)
	require.NoError(t, err)
	_, err = c2.Open(token, nil)
	require.Error(t, err)
}

func TestOpenRejectsTamperedTokenAndWrongAAD(t *testing.T) {
	c, err := crypto.NewCipher(key(t))
	require.NoError(t, err)

	token, err := c.Seal([]byte("secret"), []byte("company-1"))
	require.NoError(t, err)

	tampered := []byte(token)
	tampered[len(tampered)-2] ^= 'A'
	_, err = c.Open(string(tampered), []byte("company-1"))
	require.Error(t, err)

	_, err = c.Open(token, []byte("company-2"))
	require.Error(t, err, "authenticated data must be bound to the ciphertext")
}

func TestNewCipherRejectsWrongKeyLength(t *testing.T) {
	_, err := crypto.NewCipher(make([]byte, 16))
	require.Error(t, err)
	require.Contains(t, err.Error(), "32")
}
```

`internal/platform/id/id_test.go`:

```go
package id_test

import (
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/id"
	"github.com/stretchr/testify/require"
)

func TestNewIsUniqueParsableAndTimeOrdered(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	previous := ""
	for i := 0; i < 1000; i++ {
		got := id.New()
		_, err := id.Parse(got)
		require.NoError(t, err)
		require.NotContains(t, seen, got)
		seen[got] = struct{}{}
		require.Greater(t, got, previous, "UUIDv7 must sort by creation time")
		previous = got
	}
}
```

`internal/platform/clock/clock_test.go`:

```go
package clock_test

import (
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/stretchr/testify/require"
)

func TestFakeClockIsDeterministic(t *testing.T) {
	start := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	c := clock.NewFake(start)

	require.Equal(t, start, c.Now())
	c.Advance(90 * time.Minute)
	require.Equal(t, start.Add(90*time.Minute), c.Now())
	require.Equal(t, 90*time.Minute, c.Since(start))
}

func TestSystemClockMoves(t *testing.T) {
	c := clock.System()
	first := c.Now()
	time.Sleep(time.Millisecond)
	require.True(t, c.Now().After(first))
}
```

`internal/platform/health/health_test.go`:

```go
package health_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/stretchr/testify/require"
)

func TestAllChecksPass(t *testing.T) {
	rep := health.Run(context.Background(), time.Second,
		health.Check{Name: "database", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "redis", Fn: func(context.Context) error { return nil }},
	)
	require.Equal(t, "ok", rep.Status)
	require.Len(t, rep.Checks, 2)
	require.Equal(t, "ok", rep.Checks[0].Status)
}

func TestFailingCheckDegradesTheReport(t *testing.T) {
	rep := health.Run(context.Background(), time.Second,
		health.Check{Name: "database", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "redis", Fn: func(context.Context) error { return errors.New("connection refused") }},
	)
	require.Equal(t, "degraded", rep.Status)
	require.Equal(t, "failed", rep.Checks[1].Status)
	require.Contains(t, rep.Checks[1].Error, "connection refused")
}

func TestSlowCheckTimesOutWithoutBlockingTheReport(t *testing.T) {
	rep := health.Run(context.Background(), 50*time.Millisecond,
		health.Check{Name: "slow", Fn: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}},
	)
	require.Equal(t, "degraded", rep.Status)
	require.Equal(t, "failed", rep.Checks[0].Status)
}

func TestResultsKeepCheckOrder(t *testing.T) {
	rep := health.Run(context.Background(), time.Second,
		health.Check{Name: "a", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "b", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "c", Fn: func(context.Context) error { return nil }},
	)
	require.Equal(t, []string{"a", "b", "c"},
		[]string{rep.Checks[0].Name, rep.Checks[1].Name, rep.Checks[2].Name})
}
```

- [ ] **Step 2: Run the tests and watch them fail**

```bash
go test ./internal/platform/... -v
```
Expected: FAIL — `clock`, `id`, `crypto`, `health` do not exist.

- [ ] **Step 3: Implement the four packages**

`internal/platform/clock/clock.go`:

```go
// Package clock makes time injectable so business logic and jobs are testable.
package clock

import "time"

// Clock reports the current time.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

type systemClock struct{}

func (systemClock) Now() time.Time                  { return time.Now() }
func (systemClock) Since(t time.Time) time.Duration { return time.Since(t) }

// System returns a Clock backed by the operating system.
func System() Clock { return systemClock{} }
```

`internal/platform/clock/fake.go`:

```go
package clock

import (
	"sync"
	"time"
)

// Fake is a Clock whose time only moves when the test moves it.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake positioned at t.
func NewFake(t time.Time) *Fake { return &Fake{now: t} }

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *Fake) Since(t time.Time) time.Duration { return f.Now().Sub(t) }

// Advance moves the clock forward.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Set positions the clock at t.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}
```

`internal/platform/id/id.go`:

```go
// Package id generates the platform's primary keys: time-ordered UUIDv7.
package id

import "github.com/google/uuid"

// New returns a new time-ordered identifier.
func New() string {
	return uuid.Must(uuid.NewV7()).String()
}

// Parse validates an identifier.
func Parse(s string) (uuid.UUID, error) { return uuid.Parse(s) }
```

`internal/platform/crypto/aesgcm.go`:

```go
// Package crypto seals integration credentials at rest with AES-256-GCM.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// Cipher seals and opens values with a single AES-256-GCM key.
type Cipher struct{ aead cipher.AEAD }

// NewCipher builds a Cipher from a 32-byte key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be exactly 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("build aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("build gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Seal encrypts plaintext, binding aad to the ciphertext, and returns
// base64(nonce || ciphertext).
func (c *Cipher) Seal(plaintext, aad []byte) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, aad)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Open reverses Seal. It fails if the key, the ciphertext or aad differ.
func (c *Cipher) Open(token string, aad []byte) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}
	if len(raw) < c.aead.NonceSize() {
		return nil, errors.New("token is too short to contain a nonce")
	}
	nonce, ciphertext := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("decryption failed: wrong key, tampered ciphertext or wrong associated data")
	}
	return plaintext, nil
}
```

`internal/platform/health/health.go`:

```go
// Package health runs named readiness checks concurrently and aggregates them.
package health

import (
	"context"
	"sync"
	"time"
)

// Check is one named readiness probe.
type Check struct {
	Name string
	Fn   func(ctx context.Context) error
}

// Result is the outcome of a single check.
type Result struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// Report aggregates every check.
type Report struct {
	Status string   `json:"status"`
	Checks []Result `json:"checks"`
}

// Healthy reports whether every check passed.
func (r Report) Healthy() bool { return r.Status == "ok" }

// Run executes the checks concurrently, each bounded by timeout, and returns
// results in the order the checks were given.
func Run(ctx context.Context, timeout time.Duration, checks ...Check) Report {
	results := make([]Result, len(checks))

	var wg sync.WaitGroup
	for i, check := range checks {
		wg.Add(1)
		go func(i int, check Check) {
			defer wg.Done()
			checkCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			started := time.Now()
			err := check.Fn(checkCtx)
			res := Result{Name: check.Name, Status: "ok", DurationMS: time.Since(started).Milliseconds()}
			if err != nil {
				res.Status = "failed"
				res.Error = err.Error()
			}
			results[i] = res
		}(i, check)
	}
	wg.Wait()

	report := Report{Status: "ok", Checks: results}
	for _, r := range results {
		if r.Status != "ok" {
			report.Status = "degraded"
			break
		}
	}
	return report
}
```

- [ ] **Step 4: Add the uuid dependency and run the tests**

```bash
go get github.com/google/uuid@latest
go mod tidy
go test ./internal/platform/... -race -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): clock, uuidv7 ids, aes-256-gcm cipher and health registry

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---
## Task 5: Postgres pool, embedded migrations and the `migrate` command

**Files:**
- Create: `internal/store/postgres/pool.go`, `migrate.go`, `health.go`
- Create: `internal/store/postgres/migrations/00001_extensions.sql`
- Create: `internal/cli/migrate.go`
- Test: `internal/store/postgres/postgres_integration_test.go` (build tag `integration`)

**Interfaces:**
- Consumes: `config.DB`, `*slog.Logger`.
- Produces:
  - `postgres.NewPool(ctx context.Context, cfg config.DB, log *slog.Logger) (*pgxpool.Pool, error)`
  - `postgres.Ping(ctx context.Context, pool *pgxpool.Pool) error`
  - `postgres.MigrateUp(ctx context.Context, dsn string, log *slog.Logger) error`
  - `postgres.MigrateDownAll(ctx context.Context, dsn string, log *slog.Logger) error`
  - `postgres.MigrateStatus(ctx context.Context, dsn string, out io.Writer) error`
  - `postgres.PendingMigrations(ctx context.Context, dsn string) (int, error)` — 0 means the schema is current.
  - `postgres.PoolCheck(pool *pgxpool.Pool) health.Check` and `postgres.MigrationsCheck(dsn string) health.Check`

- [ ] **Step 1: Write the failing integration test**

`internal/store/postgres/postgres_integration_test.go`:

```go
//go:build integration

package postgres_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// startPostgres boots TimescaleDB and returns its DSN.
func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "timescale/timescaledb:2.30.0-pg16",
		tcpostgres.WithDatabase("ekokod"),
		tcpostgres.WithUsername("ekokod"),
		tcpostgres.WithPassword("ekokod"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return dsn
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestMigrateUpDownUp(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)
	log := discardLogger()

	require.NoError(t, postgres.MigrateUp(ctx, dsn, log))

	pending, err := postgres.PendingMigrations(ctx, dsn)
	require.NoError(t, err)
	require.Zero(t, pending)

	require.NoError(t, postgres.MigrateDownAll(ctx, dsn, log))

	pending, err = postgres.PendingMigrations(ctx, dsn)
	require.NoError(t, err)
	require.Positive(t, pending, "after down --all every migration is pending again")

	require.NoError(t, postgres.MigrateUp(ctx, dsn, log))
	pending, err = postgres.PendingMigrations(ctx, dsn)
	require.NoError(t, err)
	require.Zero(t, pending)
}

func TestTimescaleExtensionIsInstalled(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 4, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second}, discardLogger())
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	var version string
	require.NoError(t, pool.QueryRow(ctx,
		`select extversion from pg_extension where extname = 'timescaledb'`).Scan(&version))
	require.NotEmpty(t, version)
}

func TestPoolCheckReportsHealth(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 4, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second}, discardLogger())
	require.NoError(t, err)

	require.NoError(t, postgres.Ping(ctx, pool))
	pool.Close()
	require.Error(t, postgres.Ping(ctx, pool), "a closed pool must report unhealthy")
}

func TestStatementTimeoutIsApplied(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)

	pool, err := postgres.NewPool(ctx, config.DB{URL: dsn, MaxConns: 2, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 250 * time.Millisecond}, discardLogger())
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	_, err = pool.Exec(ctx, "select pg_sleep(2)")
	require.Error(t, err, "the configured statement timeout must cancel a long query")
}
```

> `tcpostgres.BasicWaitStrategies()` already blocks until Postgres accepts connections, so no extra wait strategy is needed.

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/store/postgres/... -tags=integration -v
```
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the first migration**

`internal/store/postgres/migrations/00001_extensions.sql`:

```sql
-- +goose Up
-- TimescaleDB provides the hypertables and continuous aggregates that every
-- time-series table in this system depends on (see docs/rewrite/04-data-model.md).
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose Down
DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS pgcrypto;
DROP EXTENSION IF EXISTS timescaledb;
```

- [ ] **Step 4: Implement `internal/store/postgres/pool.go`**

```go
// Package postgres owns the database connection pool and the embedded schema
// migrations. It contains no business logic.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens the connection pool and verifies it can reach the database.
func NewPool(ctx context.Context, cfg config.DB, log *slog.Logger) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = int32(cfg.MinConns)
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = time.Minute

	if cfg.StatementTimeout > 0 {
		if poolCfg.ConnConfig.RuntimeParams == nil {
			poolCfg.ConnConfig.RuntimeParams = map[string]string{}
		}
		poolCfg.ConnConfig.RuntimeParams["statement_timeout"] =
			fmt.Sprintf("%d", cfg.StatementTimeout.Milliseconds())
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Info("database pool ready",
		slog.Int("max_conns", cfg.MaxConns),
		slog.Int("min_conns", cfg.MinConns),
		slog.Duration("statement_timeout", cfg.StatementTimeout))
	return pool, nil
}

// Ping verifies the pool can reach the database.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("database pool is not initialised")
	}
	return pool.Ping(ctx)
}
```

- [ ] **Step 5: Implement `internal/store/postgres/migrate.go`**

```go
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for goose
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const migrationsDir = "migrations"

func openSQL(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open migration connection: %w", err)
	}
	return db, nil
}

func provider(dsn string) (*sql.DB, error) {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return nil, fmt.Errorf("set goose dialect: %w", err)
	}
	return openSQL(dsn)
}

// MigrateUp applies every pending migration.
func MigrateUp(ctx context.Context, dsn string, log *slog.Logger) error {
	db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	before, _ := goose.GetDBVersionContext(ctx, db)
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	after, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	log.Info("migrations applied", slog.Int64("from", before), slog.Int64("to", after))
	return nil
}

// MigrateDownAll rolls every migration back.
func MigrateDownAll(ctx context.Context, dsn string, log *slog.Logger) error {
	db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := goose.DownToContext(ctx, db, migrationsDir, 0); err != nil {
		return fmt.Errorf("roll back migrations: %w", err)
	}
	log.Info("migrations rolled back")
	return nil
}

// MigrateStatus writes the migration status table to out.
func MigrateStatus(ctx context.Context, dsn string, out io.Writer) error {
	db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetLogger(goose.NewLogger(out))
	if err := goose.StatusContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("read migration status: %w", err)
	}
	return nil
}

// PendingMigrations counts migrations that have not been applied. Zero means
// the schema is current, which is what readiness reports.
func PendingMigrations(ctx context.Context, dsn string) (int, error) {
	db, err := provider(dsn)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	migrations, err := goose.CollectMigrations(migrationsDir, 0, goose.MaxVersion)
	if err != nil {
		return 0, fmt.Errorf("collect migrations: %w", err)
	}
	current, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}

	pending := 0
	for _, m := range migrations {
		if m.Version > current {
			pending++
		}
	}
	return pending, nil
}
```

> `goose.NewLogger` does not exist in every goose release. If it is absent in v3.28.0, use `goose.SetLogger(log.New(out, "", 0))` with the standard library `log` package instead, and adjust the import.

- [ ] **Step 6: Implement `internal/store/postgres/health.go`**

```go
package postgres

import (
	"context"
	"fmt"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolCheck reports whether the database is reachable.
func PoolCheck(pool *pgxpool.Pool) health.Check {
	return health.Check{
		Name: "database",
		Fn:   func(ctx context.Context) error { return Ping(ctx, pool) },
	}
}

// MigrationsCheck reports whether the schema is current.
func MigrationsCheck(dsn string) health.Check {
	return health.Check{
		Name: "migrations",
		Fn: func(ctx context.Context) error {
			pending, err := PendingMigrations(ctx, dsn)
			if err != nil {
				return err
			}
			if pending > 0 {
				return fmt.Errorf("%d migration(s) pending", pending)
			}
			return nil
		},
	}
}
```

- [ ] **Step 7: Implement `internal/cli/migrate.go`**

```go
package cli

import (
	"os"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/spf13/cobra"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply, roll back or inspect database migrations",
	}

	up := &cobra.Command{
		Use:   "up",
		Short: "Apply every pending migration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			log := logging.New(cfg.LogLevel, string(cfg.LogFormat), os.Stderr)
			return postgres.MigrateUp(cmd.Context(), cfg.DB.URL, log)
		},
	}

	var all bool
	down := &cobra.Command{
		Use:   "down",
		Short: "Roll migrations back (requires --all)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			log := logging.New(cfg.LogLevel, string(cfg.LogFormat), os.Stderr)
			if !all {
				return errMigrateDownNeedsAll
			}
			return postgres.MigrateDownAll(cmd.Context(), cfg.DB.URL, log)
		},
	}
	down.Flags().BoolVar(&all, "all", false, "roll back every migration")

	status := &cobra.Command{
		Use:   "status",
		Short: "Show which migrations have been applied",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			return postgres.MigrateStatus(cmd.Context(), cfg.DB.URL, cmd.OutOrStdout())
		},
	}

	cmd.AddCommand(up, down, status)
	return cmd
}
```

Declare the sentinel next to it:

```go
var errMigrateDownNeedsAll = apperrors.New("migrate_down_needs_all", http.StatusBadRequest,
	"errors.migrate.downNeedsAll")
```

with imports `net/http` and `apperrors "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"`. Register `newMigrateCmd()` in `root.go`.

- [ ] **Step 8: Add dependencies and run the integration test**

```bash
go get github.com/jackc/pgx/v5@v5.11.0 github.com/pressly/goose/v3@v3.28.0
go get github.com/testcontainers/testcontainers-go@v0.44.0 \
       github.com/testcontainers/testcontainers-go/modules/postgres@v0.44.0
go mod tidy
go test ./internal/store/postgres/... -tags=integration -v -timeout 15m
```
Expected: PASS, with the TimescaleDB container starting and the extension query returning a version.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): postgres pool, embedded goose migrations and migrate command

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---

## Task 6: Redis client, asynq queue and the `noop` round trip

**Files:**
- Create: `internal/store/redis/redis.go`
- Create: `internal/job/task.go`, `noop.go`, `client.go`, `server.go`
- Test: `internal/store/redis/redis_integration_test.go`, `internal/job/job_integration_test.go` (build tag `integration`)

**Interfaces:**
- Consumes: `config.Redis`, `config.Worker`, `*slog.Logger`, `health.Check`.
- Produces:
  - `redis.New(ctx context.Context, cfg config.Redis, log *slog.Logger) (*redis.Client, error)` (package name `ekoredis` on import to avoid clashing with `go-redis`), `redis.Check(client *goredis.Client) health.Check`. The asynq connection options live in `job.RedisOpt(cfg config.Redis) (asynq.RedisClientOpt, error)` so the queue database choice sits with the queue package.
  - `job.TypeNoop = "system.noop"`; `job.NoopPayload{Message string}`; `job.NewNoopTask(message string) (*asynq.Task, error)`.
  - `job.NewClient(cfg config.Redis) (*job.Client, error)` with `Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)` and `Close() error`.
  - `job.NewServer(cfg config.Redis, worker config.Worker, log *slog.Logger) (*asynq.Server, *asynq.ServeMux, error)` and `job.Register(mux *asynq.ServeMux, h *job.Handlers)`.
  - `job.Handlers{Log *slog.Logger}` with method `Noop(ctx context.Context, t *asynq.Task) error`.

- [ ] **Step 1: Write the failing integration test**

`internal/job/job_integration_test.go`:

```go
//go:build integration

package job_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func startRedis(t *testing.T) config.Redis {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7.4.11-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	uri, err := container.ConnectionString(ctx)
	require.NoError(t, err)
	return config.Redis{URL: uri, CacheDB: 0, QueueDB: 1}
}

func TestNoopTaskRoundTrip(t *testing.T) {
	cfg := startRedis(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	server, mux, err := job.NewServer(cfg, config.Worker{Concurrency: 2, MaxRetries: 1, Timeout: time.Minute}, log)
	require.NoError(t, err)

	handled := make(chan string, 1)
	mux.HandleFunc(job.TypeNoop, func(ctx context.Context, task *asynq.Task) error {
		payload, err := job.DecodeNoop(task)
		if err != nil {
			return err
		}
		handled <- payload.Message
		return nil
	})

	go func() { _ = server.Run(mux) }()
	t.Cleanup(server.Shutdown)

	client, err := job.NewClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	task, err := job.NewNoopTask("hello from the api")
	require.NoError(t, err)
	info, err := client.Enqueue(context.Background(), task)
	require.NoError(t, err)
	require.Equal(t, job.TypeNoop, info.Type)

	select {
	case got := <-handled:
		require.Equal(t, "hello from the api", got)
	case <-time.After(20 * time.Second):
		t.Fatal("worker did not execute the enqueued task")
	}
}

func TestQueueUsesTheConfiguredDatabase(t *testing.T) {
	cfg := startRedis(t)
	opt, err := job.RedisOpt(cfg)
	require.NoError(t, err)
	require.Equal(t, cfg.QueueDB, opt.DB, "the queue must not share the cache database")
}
```

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/job/... -tags=integration -v
```
Expected: FAIL — package `job` does not exist.

- [ ] **Step 3: Implement `internal/store/redis/redis.go`**

```go
// Package redis owns the cache connection. The job queue uses a separate
// logical database so a cache flush can never drop queued work.
package redis

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	goredis "github.com/redis/go-redis/v9"
)

// New opens the cache client and verifies it can reach Redis.
func New(ctx context.Context, cfg config.Redis, log *slog.Logger) (*goredis.Client, error) {
	opts, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	opts.DB = cfg.CacheDB

	client := goredis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	log.Info("redis ready", slog.Int("cache_db", cfg.CacheDB))
	return client, nil
}

// Check reports whether Redis is reachable.
func Check(client *goredis.Client) health.Check {
	return health.Check{
		Name: "redis",
		Fn: func(ctx context.Context) error {
			if client == nil {
				return fmt.Errorf("redis client is not initialised")
			}
			return client.Ping(ctx).Err()
		},
	}
}
```

- [ ] **Step 4: Implement `internal/job/task.go` and `noop.go`**

`task.go`:

```go
// Package job defines the platform's background tasks and their handlers.
// Every task is idempotent, bounded, isolated and recorded.
package job

import (
	"fmt"
	"log/slog"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/hibiken/asynq"
	goredis "github.com/redis/go-redis/v9"
)

// Task type names. Later phases add integration, billing, alarm and report tasks.
const (
	TypeNoop = "system.noop"
)

// Queue names, ordered by priority when the worker picks work.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

// RedisOpt builds the asynq connection options, pointing at the queue database.
func RedisOpt(cfg config.Redis) (asynq.RedisClientOpt, error) {
	opts, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		return asynq.RedisClientOpt{}, fmt.Errorf("parse redis url: %w", err)
	}
	return asynq.RedisClientOpt{
		Addr:     opts.Addr,
		Username: opts.Username,
		Password: opts.Password,
		DB:       cfg.QueueDB,
	}, nil
}

// Handlers holds the dependencies every task handler needs.
type Handlers struct {
	Log *slog.Logger
}

// Register attaches every handler to the mux.
func Register(mux *asynq.ServeMux, h *Handlers) {
	mux.HandleFunc(TypeNoop, h.Noop)
}
```

`noop.go`:

```go
package job

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// NoopPayload is the payload of the system.noop task, which exists to prove the
// enqueue → execute round trip end to end.
type NoopPayload struct {
	Message string `json:"message"`
}

// NewNoopTask builds a system.noop task.
func NewNoopTask(message string) (*asynq.Task, error) {
	payload, err := json.Marshal(NoopPayload{Message: message})
	if err != nil {
		return nil, fmt.Errorf("encode noop payload: %w", err)
	}
	return asynq.NewTask(TypeNoop, payload, asynq.Queue(QueueLow)), nil
}

// DecodeNoop reads a system.noop payload.
func DecodeNoop(task *asynq.Task) (NoopPayload, error) {
	var payload NoopPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return NoopPayload{}, fmt.Errorf("decode noop payload: %w", err)
	}
	return payload, nil
}

// Noop handles system.noop.
func (h *Handlers) Noop(ctx context.Context, task *asynq.Task) error {
	payload, err := DecodeNoop(task)
	if err != nil {
		return err
	}
	h.Log.InfoContext(ctx, "noop task executed", slog.String("message", payload.Message))
	return nil
}
```

- [ ] **Step 5: Implement `internal/job/client.go` and `server.go`**

`client.go`:

```go
package job

import (
	"context"
	"fmt"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/hibiken/asynq"
)

// Client enqueues tasks.
type Client struct{ inner *asynq.Client }

// NewClient opens a queue client.
func NewClient(cfg config.Redis) (*Client, error) {
	opt, err := RedisOpt(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{inner: asynq.NewClient(opt)}, nil
}

// Enqueue schedules a task for execution.
func (c *Client) Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	info, err := c.inner.EnqueueContext(ctx, task, opts...)
	if err != nil {
		return nil, fmt.Errorf("enqueue %s: %w", task.Type(), err)
	}
	return info, nil
}

// Close releases the client's connections.
func (c *Client) Close() error { return c.inner.Close() }
```

`server.go`:

```go
package job

import (
	"context"
	"log/slog"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/hibiken/asynq"
)

// NewServer builds the worker server and an empty mux. Callers register
// handlers on the mux before running the server.
func NewServer(redisCfg config.Redis, worker config.Worker, log *slog.Logger) (*asynq.Server, *asynq.ServeMux, error) {
	opt, err := RedisOpt(redisCfg)
	if err != nil {
		return nil, nil, err
	}

	server := asynq.NewServer(opt, asynq.Config{
		Concurrency: worker.Concurrency,
		Queues: map[string]int{
			QueueCritical: 6,
			QueueDefault:  3,
			QueueLow:      1,
		},
		ShutdownTimeout: worker.Timeout,
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			// Never swallowed: every failure is logged with its task type and
			// surfaced as an operational message from F7 onwards.
			log.ErrorContext(ctx, "task failed",
				slog.String("task_type", task.Type()),
				slog.String("error", err.Error()))
		}),
		Logger: asynqLogger{log: log},
	})
	return server, asynq.NewServeMux(), nil
}

// asynqLogger adapts slog to asynq's logger interface.
type asynqLogger struct{ log *slog.Logger }

func (l asynqLogger) Debug(args ...any) { l.log.Debug(sprint(args)) }
func (l asynqLogger) Info(args ...any)  { l.log.Info(sprint(args)) }
func (l asynqLogger) Warn(args ...any)  { l.log.Warn(sprint(args)) }
func (l asynqLogger) Error(args ...any) { l.log.Error(sprint(args)) }
func (l asynqLogger) Fatal(args ...any) { l.log.Error(sprint(args)) }
```

Add the small helper in the same file:

```go
func sprint(args []any) string { return strings.TrimSpace(fmt.Sprintln(args...)) }
```

with imports `fmt` and `strings`.

- [ ] **Step 6: Add dependencies and run the tests**

```bash
go get github.com/hibiken/asynq@v0.26.0 github.com/redis/go-redis/v9@latest
go get github.com/testcontainers/testcontainers-go/modules/redis@v0.44.0
go mod tidy
go test ./internal/job/... ./internal/store/redis/... -tags=integration -v -timeout 10m
```
Expected: PASS — the worker executes the enqueued task and the queue uses DB 1.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): redis client and asynq queue with a noop round-trip task

Queue uses a separate redis database from the cache so a cache flush
cannot drop queued work.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---
## Task 7: HTTP router, middleware, health, version and metrics

**Files:**
- Create: `internal/api/router.go`, `health.go`, `version.go`
- Create: `internal/api/middleware/requestid.go`, `logging.go`, `recoverer.go`, `cors.go`, `ratelimit.go`, `metrics.go`
- Test: `internal/api/router_test.go`, `internal/api/middleware/ratelimit_test.go`, `internal/api/middleware/recoverer_test.go`

**Interfaces:**
- Consumes: `config.Config`, `*slog.Logger`, `health.Check`, `buildinfo.Info`, `logging.WithField`.
- Produces:
  - `api.Deps{Cfg *config.Config, Log *slog.Logger, Build buildinfo.Info, ReadyChecks []health.Check}`
  - `api.NewRouter(d Deps) http.Handler` mounting `GET /health/live`, `GET /health/ready`, `GET /version`, `GET /metrics`.
  - `middleware.RequestID(next http.Handler) http.Handler` — reads or generates `X-Request-Id`, echoes it, puts it in the context via `logging.WithField`.
  - `middleware.Logger(log *slog.Logger) func(http.Handler) http.Handler`
  - `middleware.Recoverer(log *slog.Logger) func(http.Handler) http.Handler`
  - `middleware.CORS(origins []string) func(http.Handler) http.Handler`
  - `middleware.RateLimit(limit config.RateLimit, trustedProxies []string) func(http.Handler) http.Handler`
  - `middleware.Metrics(next http.Handler) http.Handler` and `middleware.MetricsHandler() http.Handler`

- [ ] **Step 1: Write the failing tests**

`internal/api/router_test.go`:

```go
package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/api"
	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/stretchr/testify/require"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	env := map[string]string{
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
	cfg, err := config.Load(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	require.NoError(t, err)
	return cfg
}

func newTestRouter(t *testing.T, checks ...health.Check) http.Handler {
	t.Helper()
	return api.NewRouter(api.Deps{
		Cfg:         testConfig(t),
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Build:       buildinfo.Get(),
		ReadyChecks: checks,
	})
}

func TestHealthLiveAlwaysReturns200(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHealthReadyReportsEachDependency(t *testing.T) {
	router := newTestRouter(t,
		health.Check{Name: "database", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "redis", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "migrations", Fn: func(context.Context) error { return nil }},
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var report health.Report
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	require.Equal(t, "ok", report.Status)
	require.Len(t, report.Checks, 3)
}

func TestHealthReadyReturns503WhenADependencyIsDown(t *testing.T) {
	router := newTestRouter(t,
		health.Check{Name: "database", Fn: func(context.Context) error { return errors.New("connection refused") }},
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "connection refused")
}

func TestVersionEndpoint(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/version", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "version")
}

func TestMetricsEndpointExposesHTTPMetrics(t *testing.T) {
	router := newTestRouter(t)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health/live", nil))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "ekokod_http_request_duration_seconds")
}

func TestRequestIDIsEchoedAndGenerated(t *testing.T) {
	router := newTestRouter(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	require.NotEmpty(t, rec.Header().Get("X-Request-Id"))

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("X-Request-Id", "caller-supplied-id")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, "caller-supplied-id", rec.Header().Get("X-Request-Id"))
}

func TestCORSAllowsTheConfiguredOriginOnly(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodOptions, "/health/live", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, "http://localhost:3000", rec.Header().Get("Access-Control-Allow-Origin"))

	req = httptest.NewRequest(http.MethodOptions, "/health/live", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}
```

`internal/api/middleware/ratelimit_test.go`:

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/middleware"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestRateLimitBlocksAfterTheLimitAndIsPerClient(t *testing.T) {
	limited := middleware.RateLimit(config.RateLimit{Limit: 2, Window: time.Minute}, nil)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	call := func(ip string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = ip + ":12345"
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req)
		return rec.Code
	}

	require.Equal(t, http.StatusOK, call("10.0.0.1"))
	require.Equal(t, http.StatusOK, call("10.0.0.1"))
	require.Equal(t, http.StatusTooManyRequests, call("10.0.0.1"))
	require.Equal(t, http.StatusOK, call("10.0.0.2"), "the limit is per client, not global")
}

func TestRateLimitIgnoresForwardedForFromUntrustedProxies(t *testing.T) {
	limited := middleware.RateLimit(config.RateLimit{Limit: 1, Window: time.Minute}, nil)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	call := func(forwarded string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.9:1111"
		req.Header.Set("X-Forwarded-For", forwarded)
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req)
		return rec.Code
	}

	require.Equal(t, http.StatusOK, call("1.1.1.1"))
	require.Equal(t, http.StatusTooManyRequests, call("2.2.2.2"),
		"a spoofed X-Forwarded-For must not reset the bucket when the proxy is untrusted")
}
```

`internal/api/middleware/recoverer_test.go`:

```go
package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/middleware"
	"github.com/stretchr/testify/require"
)

func TestRecovererTurnsAPanicInto500AndLogsIt(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := middleware.Recoverer(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	require.NotPanics(t, func() {
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), "internal")
	require.Contains(t, buf.String(), "panic")
	require.NotContains(t, rec.Body.String(), "boom", "the panic value must not reach the client")
}
```

- [ ] **Step 2: Run them and watch them fail**

```bash
go test ./internal/api/... -v
```
Expected: FAIL — packages do not exist.

- [ ] **Step 3: Implement the middleware package**

`requestid.go`:

```go
// Package middleware holds the HTTP middleware chain. Nothing here contains
// business logic.
package middleware

import (
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/id"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
)

// HeaderRequestID is the header carrying the correlation id.
const HeaderRequestID = "X-Request-Id"

// RequestID reads or generates a request id, echoes it and puts it in context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(HeaderRequestID)
		if requestID == "" {
			requestID = id.New()
		}
		w.Header().Set(HeaderRequestID, requestID)
		ctx := logging.WithField(r.Context(), logging.FieldRequestID, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

`logging.go`:

```go
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Logger writes one structured record per request.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			log.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(started)))
		})
	}
}
```

`recoverer.go`:

```go
package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recoverer converts a panic into a 500 without leaking the panic value.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.ErrorContext(r.Context(), "panic recovered",
						slog.Any("panic", recovered),
						slog.String("stack", string(debug.Stack())),
						slog.String("path", r.URL.Path))

					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"code":"internal","message_key":"errors.generic.internal"}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
```

`cors.go`:

```go
package middleware

import (
	"net/http"

	"github.com/go-chi/cors"
)

// CORS allows exactly the configured origins. With none configured, no
// cross-origin request is allowed.
func CORS(origins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", HeaderRequestID, "Accept-Language"},
		ExposedHeaders:   []string{HeaderRequestID},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
```

`ratelimit.go`:

```go
package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"golang.org/x/time/rate"
)

// RateLimit applies a per-client token bucket. The client is the peer address
// unless the peer is a trusted proxy, in which case the left-most
// X-Forwarded-For entry is used. F6 adds a per-principal bucket on top.
func RateLimit(limit config.RateLimit, trustedProxies []string) func(http.Handler) http.Handler {
	trusted := parseCIDRs(trustedProxies)
	buckets := &bucketSet{
		limiters: map[string]*bucketEntry{},
		rate:     rate.Limit(float64(limit.Limit) / limit.Window.Seconds()),
		burst:    limit.Limit,
	}
	go buckets.reapEvery(10 * time.Minute)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !buckets.allow(clientKey(r, trusted)) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"code":"rate_limited","message_key":"errors.generic.rateLimited"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type bucketEntry struct {
	limiter *rate.Limiter
	seen    time.Time
}

type bucketSet struct {
	mu       sync.Mutex
	limiters map[string]*bucketEntry
	rate     rate.Limit
	burst    int
}

func (b *bucketSet) allow(key string) bool {
	b.mu.Lock()
	entry, ok := b.limiters[key]
	if !ok {
		entry = &bucketEntry{limiter: rate.NewLimiter(b.rate, b.burst)}
		b.limiters[key] = entry
	}
	entry.seen = time.Now()
	b.mu.Unlock()
	return entry.limiter.Allow()
}

func (b *bucketSet) reapEvery(d time.Duration) {
	for range time.Tick(d) {
		cutoff := time.Now().Add(-d)
		b.mu.Lock()
		for key, entry := range b.limiters {
			if entry.seen.Before(cutoff) {
				delete(b.limiters, key)
			}
		}
		b.mu.Unlock()
	}
}

func parseCIDRs(raw []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(raw))
	for _, item := range raw {
		if _, network, err := net.ParseCIDR(item); err == nil {
			nets = append(nets, network)
		}
	}
	return nets
}

// clientKey identifies the caller, honouring X-Forwarded-For only from a
// trusted proxy so a spoofed header cannot reset someone else's bucket.
func clientKey(r *http.Request, trusted []*net.IPNet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	for _, network := range trusted {
		if peer != nil && network.Contains(peer) {
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				if comma := indexByte(forwarded, ','); comma > 0 {
					return trimSpace(forwarded[:comma])
				}
				return trimSpace(forwarded)
			}
		}
	}
	return host
}
```

Replace `indexByte`/`trimSpace` with `strings.IndexByte` and `strings.TrimSpace` and import `strings`.

`metrics.go`:

```go
package middleware

import (
	"net/http"
	"strconv"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "ekokod_http_request_duration_seconds",
	Help:    "HTTP request duration by route, method and status.",
	Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
}, []string{"route", "method", "status"})

// Metrics records request duration per route.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		pattern := r.URL.Path
		if rc := chiRoutePattern(r); rc != "" {
			pattern = rc
		}
		requestDuration.WithLabelValues(pattern, r.Method, strconv.Itoa(ww.Status())).
			Observe(time.Since(started).Seconds())
	})
}

// MetricsHandler serves the Prometheus scrape endpoint.
func MetricsHandler() http.Handler { return promhttp.Handler() }
```

Implement `chiRoutePattern` in the same file:

```go
func chiRoutePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		return rctx.RoutePattern()
	}
	return ""
}
```

with `"github.com/go-chi/chi/v5"` imported. Using the route *pattern* rather than the raw path keeps metric cardinality bounded once path parameters exist.

- [ ] **Step 4: Implement `internal/api/health.go`, `version.go`, `router.go`**

`health.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
)

const readyCheckTimeout = 3 * time.Second

func liveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func readyHandler(checks []health.Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := health.Run(r.Context(), readyCheckTimeout, checks...)
		status := http.StatusOK
		if !report.Healthy() {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, report)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
```

`version.go`:

```go
package api

import (
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
)

func versionHandler(info buildinfo.Info) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, info)
	}
}
```

`router.go`:

```go
// Package api exposes the HTTP surface. Handlers decode, authorise, call a
// service and encode — they contain no business logic.
package api

import (
	"log/slog"
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/middleware"
	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/go-chi/chi/v5"
)

// Deps are everything the HTTP surface needs.
type Deps struct {
	Cfg         *config.Config
	Log         *slog.Logger
	Build       buildinfo.Info
	ReadyChecks []health.Check
}

// NewRouter builds the HTTP handler.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer(d.Log))
	r.Use(middleware.Logger(d.Log))
	r.Use(middleware.CORS(d.Cfg.HTTP.CORSOrigins))
	r.Use(middleware.Metrics)
	r.Use(middleware.RateLimit(d.Cfg.HTTP.RateLimitAPI, d.Cfg.HTTP.TrustedProxies))

	r.Get("/health/live", liveHandler())
	r.Get("/health/ready", readyHandler(d.ReadyChecks))
	r.Get("/version", versionHandler(d.Build))
	r.Handle("/metrics", middleware.MetricsHandler())

	return r
}
```

- [ ] **Step 5: Add dependencies and run the tests**

```bash
go get github.com/go-chi/chi/v5@v5.3.2 github.com/go-chi/cors@latest \
       github.com/prometheus/client_golang@latest golang.org/x/time@latest
go mod tidy
go test ./internal/api/... -race -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): chi router with request-id, logging, recovery, cors, rate limit and metrics

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---

## Task 8: Scheduler with Postgres advisory-lock leader election

**Files:**
- Create: `internal/scheduler/elector.go`, `scheduler.go`
- Test: `internal/scheduler/elector_integration_test.go` (build tag `integration`)

**Interfaces:**
- Consumes: `*pgxpool.Pool`, `config.Schedule`, `job.RedisOpt`, `*slog.Logger`.
- Produces:
  - `scheduler.NewElector(pool *pgxpool.Pool, lockKey int64, retry time.Duration, log *slog.Logger) *scheduler.Elector`
  - `(*Elector).Run(ctx context.Context, lead func(ctx context.Context) error) error` — blocks; calls `lead` with a context cancelled when leadership is lost.
  - `(*Elector).IsLeader() bool`
  - `scheduler.LockKeyScheduler int64 = 0x656B6F6B6F64` (ASCII "ekokod")
  - `scheduler.New(cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) *scheduler.Scheduler` and `(*Scheduler).Run(ctx context.Context) error` — registers the cron entries and runs them only while leader.

- [ ] **Step 1: Write the failing test**

`internal/scheduler/elector_integration_test.go`:

```go
//go:build integration

package scheduler_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/scheduler"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "timescale/timescaledb:2.30.0-pg16",
		tcpostgres.WithDatabase("ekokod"),
		tcpostgres.WithUsername("ekokod"),
		tcpostgres.WithPassword("ekokod"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return dsn
}

func newPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool, err := postgres.NewPool(context.Background(), config.DB{
		URL: dsn, MaxConns: 4, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second,
	}, log)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// TestLeaderElection is named in the F0 acceptance criteria: exactly one of two
// simultaneously started instances leads, and killing it transfers leadership.
func TestLeaderElection(t *testing.T) {
	dsn := startPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	const retry = 200 * time.Millisecond

	first := scheduler.NewElector(newPool(t, dsn), scheduler.LockKeyScheduler, retry, log)
	second := scheduler.NewElector(newPool(t, dsn), scheduler.LockKeyScheduler, retry, log)

	firstCtx, stopFirst := context.WithCancel(context.Background())
	secondCtx, stopSecond := context.WithCancel(context.Background())
	t.Cleanup(stopSecond)

	lead := func(ctx context.Context) error { <-ctx.Done(); return nil }
	go func() { _ = first.Run(firstCtx, lead) }()
	go func() { _ = second.Run(secondCtx, lead) }()

	require.Eventually(t, func() bool { return first.IsLeader() != second.IsLeader() },
		5*time.Second, 50*time.Millisecond, "exactly one instance must hold leadership")

	leaderIsFirst := first.IsLeader()
	require.False(t, first.IsLeader() && second.IsLeader(), "leadership must be exclusive")

	// Kill the leader; the follower must take over.
	if leaderIsFirst {
		stopFirst()
		require.Eventually(t, second.IsLeader, 5*time.Second, 50*time.Millisecond,
			"leadership must transfer when the leader stops")
	} else {
		stopSecond()
		require.Eventually(t, first.IsLeader, 5*time.Second, 50*time.Millisecond,
			"leadership must transfer when the leader stops")
		stopFirst()
	}
}

func TestLeadContextIsCancelledWhenTheProcessStops(t *testing.T) {
	dsn := startPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	elector := scheduler.NewElector(newPool(t, dsn), scheduler.LockKeyScheduler, 100*time.Millisecond, log)
	ctx, cancel := context.WithCancel(context.Background())

	cancelled := make(chan struct{})
	go func() {
		_ = elector.Run(ctx, func(leadCtx context.Context) error {
			<-leadCtx.Done()
			close(cancelled)
			return nil
		})
	}()

	require.Eventually(t, elector.IsLeader, 5*time.Second, 50*time.Millisecond)
	cancel()

	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the lead function's context was not cancelled on shutdown")
	}
	require.False(t, elector.IsLeader())
}
```

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/scheduler/... -tags=integration -v
```
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `internal/scheduler/elector.go`**

```go
// Package scheduler enqueues periodic work. Exactly one instance is active at a
// time, elected with a Postgres session-scoped advisory lock. There is no OS
// cron and no shared API key.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LockKeyScheduler is the advisory-lock key for scheduler leadership.
// The value spells "ekokod" in ASCII.
const LockKeyScheduler int64 = 0x656B6F6B6F64

// Elector acquires and holds scheduler leadership.
type Elector struct {
	pool   *pgxpool.Pool
	key    int64
	retry  time.Duration
	log    *slog.Logger
	leader atomic.Bool
}

// NewElector builds an Elector that retries acquisition every retry interval.
func NewElector(pool *pgxpool.Pool, key int64, retry time.Duration, log *slog.Logger) *Elector {
	return &Elector{pool: pool, key: key, retry: retry, log: log}
}

// IsLeader reports whether this instance currently holds leadership.
func (e *Elector) IsLeader() bool { return e.leader.Load() }

// Run campaigns for leadership until ctx is done. While leading it calls lead
// with a context that is cancelled as soon as leadership is lost.
func (e *Elector) Run(ctx context.Context, lead func(ctx context.Context) error) error {
	ticker := time.NewTicker(e.retry)
	defer ticker.Stop()

	for {
		if err := e.attempt(ctx, lead); err != nil && ctx.Err() == nil {
			e.log.Error("leader election attempt failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// attempt tries once to become leader and, if successful, leads until the lock
// or the context is lost.
func (e *Elector) attempt(ctx context.Context, lead func(ctx context.Context) error) error {
	conn, err := e.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	var acquired bool
	if err := conn.QueryRow(ctx, "select pg_try_advisory_lock($1)", e.key).Scan(&acquired); err != nil {
		return fmt.Errorf("try advisory lock: %w", err)
	}
	if !acquired {
		return nil
	}

	e.leader.Store(true)
	e.log.Info("scheduler leadership acquired", slog.Int64("lock_key", e.key))
	defer func() {
		e.leader.Store(false)
		// Release on a background context so shutdown still frees the lock.
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(releaseCtx, "select pg_advisory_unlock($1)", e.key); err != nil {
			e.log.Warn("advisory unlock failed; the lock is released when the session ends",
				slog.String("error", err.Error()))
		}
		e.log.Info("scheduler leadership released")
	}()

	leadCtx, cancelLead := context.WithCancel(ctx)
	defer cancelLead()
	go e.watchConnection(leadCtx, conn, cancelLead)

	return lead(leadCtx)
}

// watchConnection cancels leadership if the connection holding the lock dies.
func (e *Elector) watchConnection(ctx context.Context, conn interface {
	Ping(context.Context) error
}, cancel context.CancelFunc) {
	ticker := time.NewTicker(e.retry)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancelPing := context.WithTimeout(ctx, e.retry)
			err := conn.Ping(pingCtx)
			cancelPing()
			if err != nil {
				e.log.Warn("lost the connection holding scheduler leadership",
					slog.String("error", err.Error()))
				cancel()
				return
			}
		}
	}
}
```

- [ ] **Step 4: Implement `internal/scheduler/scheduler.go`**

```go
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Entry is one cron-scheduled task.
type Entry struct {
	Cron string
	Task *asynq.Task
}

// Scheduler enqueues the periodic tasks while it holds leadership.
type Scheduler struct {
	cfg     *config.Config
	log     *slog.Logger
	elector *Elector
}

// New builds the scheduler process.
func New(cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) *Scheduler {
	return &Scheduler{
		cfg:     cfg,
		log:     log,
		elector: NewElector(pool, LockKeyScheduler, 10*time.Second, log),
	}
}

// entries returns the cron schedule. Later phases add their own tasks here;
// F0 registers only the noop task so the wiring is exercised end to end.
func (s *Scheduler) entries() ([]Entry, error) {
	noop, err := job.NewNoopTask("scheduled heartbeat")
	if err != nil {
		return nil, err
	}
	return []Entry{{Cron: "@every 1h", Task: noop}}, nil
}

// Run campaigns for leadership and schedules tasks while leading.
func (s *Scheduler) Run(ctx context.Context) error {
	opt, err := job.RedisOpt(s.cfg.Redis)
	if err != nil {
		return err
	}
	entries, err := s.entries()
	if err != nil {
		return err
	}

	return s.elector.Run(ctx, func(leadCtx context.Context) error {
		asynqScheduler := asynq.NewScheduler(opt, &asynq.SchedulerOpts{
			Location: s.cfg.Timezone,
			Logger:   schedulerLogger{log: s.log},
		})
		for _, entry := range entries {
			if _, err := asynqScheduler.Register(entry.Cron, entry.Task); err != nil {
				return fmt.Errorf("register %s at %q: %w", entry.Task.Type(), entry.Cron, err)
			}
		}
		if err := asynqScheduler.Start(); err != nil {
			return fmt.Errorf("start scheduler: %w", err)
		}
		s.log.Info("scheduler running", slog.Int("entries", len(entries)),
			slog.String("timezone", s.cfg.Timezone.String()))

		<-leadCtx.Done()
		asynqScheduler.Shutdown()
		return nil
	})
}

type schedulerLogger struct{ log *slog.Logger }

func (l schedulerLogger) Debug(args ...any) { l.log.Debug(fmt.Sprintln(args...)) }
func (l schedulerLogger) Info(args ...any)  { l.log.Info(fmt.Sprintln(args...)) }
func (l schedulerLogger) Warn(args ...any)  { l.log.Warn(fmt.Sprintln(args...)) }
func (l schedulerLogger) Error(args ...any) { l.log.Error(fmt.Sprintln(args...)) }
func (l schedulerLogger) Fatal(args ...any) { l.log.Error(fmt.Sprintln(args...)) }
```

- [ ] **Step 5: Run the tests**

```bash
go test ./internal/scheduler/... -tags=integration -race -v -timeout 15m
```
Expected: PASS — `TestLeaderElection` shows exclusive leadership and transfer.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): scheduler with postgres advisory-lock leader election

Exactly one instance schedules; leadership transfers when the leader stops.
No OS cron, no HTTP self-calls, no shared API key.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---
## Task 9: `api`, `worker`, `scheduler` and `seed` processes with graceful shutdown

**Files:**
- Create: `internal/cli/api.go`, `worker.go`, `scheduler.go`, `seed.go`
- Modify: `internal/cli/root.go` (register the new commands)
- Test: `internal/cli/commands_test.go`

**Interfaces:**
- Consumes: everything built in Tasks 2–8.
- Produces: `ekokod api`, `ekokod worker`, `ekokod scheduler`, `ekokod seed` subcommands. Each loads the configuration, builds its dependencies, runs until `SIGINT`/`SIGTERM`, and shuts down within 30 s.

- [ ] **Step 1: Write the failing test**

`internal/cli/commands_test.go`:

```go
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
```

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/cli/... -run 'TestEverySubcommand|TestApiCommandFailsFast' -v
```
Expected: FAIL — only `version`, `config:check` and `migrate` are registered.

- [ ] **Step 3: Implement `internal/cli/api.go`**

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/api"
	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	ekoredis "github.com/MErenTalan/ekokod-rewrite/internal/store/redis"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/spf13/cobra"
)

const shutdownGrace = 30 * time.Second

func newAPICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "api",
		Short: "Run the HTTP API server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			log := logging.New(cfg.LogLevel, string(cfg.LogFormat), os.Stderr)
			ctx := cmd.Context()

			pool, err := postgres.NewPool(ctx, cfg.DB, log)
			if err != nil {
				return err
			}
			defer pool.Close()

			cache, err := ekoredis.New(ctx, cfg.Redis, log)
			if err != nil {
				return err
			}
			defer func() { _ = cache.Close() }()

			router := api.NewRouter(api.Deps{
				Cfg:   cfg,
				Log:   log,
				Build: buildinfo.Get(),
				ReadyChecks: []health.Check{
					postgres.PoolCheck(pool),
					ekoredis.Check(cache),
					postgres.MigrationsCheck(cfg.DB.URL),
				},
			})

			server := &http.Server{
				Addr:              cfg.HTTP.Addr,
				Handler:           router,
				ReadHeaderTimeout: 10 * time.Second,
				ReadTimeout:       60 * time.Second,
				WriteTimeout:      120 * time.Second,
				IdleTimeout:       90 * time.Second,
			}

			errCh := make(chan error, 1)
			go func() {
				log.Info("api listening", slog.String("addr", cfg.HTTP.Addr),
					slog.String("env", string(cfg.Env)))
				if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errCh <- fmt.Errorf("http server: %w", err)
				}
			}()

			select {
			case err := <-errCh:
				return err
			case <-ctx.Done():
				log.Info("api shutting down")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
				defer cancel()
				return server.Shutdown(shutdownCtx)
			}
		},
	}
}
```

- [ ] **Step 4: Implement `internal/cli/worker.go`, `scheduler.go`, `seed.go`**

`worker.go`:

```go
package cli

import (
	"log/slog"
	"os"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	"github.com/spf13/cobra"
)

func newWorkerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worker",
		Short: "Run the background worker",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			log := logging.New(cfg.LogLevel, string(cfg.LogFormat), os.Stderr)

			server, mux, err := job.NewServer(cfg.Redis, cfg.Worker, log)
			if err != nil {
				return err
			}
			job.Register(mux, &job.Handlers{Log: log})

			errCh := make(chan error, 1)
			go func() {
				log.Info("worker started", slog.Int("concurrency", cfg.Worker.Concurrency))
				errCh <- server.Run(mux)
			}()

			select {
			case err := <-errCh:
				return err
			case <-cmd.Context().Done():
				log.Info("worker shutting down")
				server.Shutdown()
				return nil
			}
		},
	}
}
```

`scheduler.go`:

```go
package cli

import (
	"os"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	"github.com/MErenTalan/ekokod-rewrite/internal/scheduler"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/spf13/cobra"
)

func newSchedulerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scheduler",
		Short: "Run the periodic task scheduler (leader-elected)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			log := logging.New(cfg.LogLevel, string(cfg.LogFormat), os.Stderr)

			if !cfg.Scheduler.Enabled {
				log.Warn("scheduler disabled by EKOKOD_SCHEDULER_ENABLED=false; exiting")
				return nil
			}

			pool, err := postgres.NewPool(cmd.Context(), cfg.DB, log)
			if err != nil {
				return err
			}
			defer pool.Close()

			err = scheduler.New(cfg, pool, log).Run(cmd.Context())
			if cmd.Context().Err() != nil {
				return nil // a cancelled context is a clean shutdown
			}
			return err
		},
	}
}
```

`seed.go` — F0 has nothing to seed; the command exists, validates its flags and
reports that no dataset is selected. F1 fills in the datasets.

```go
package cli

import (
	"fmt"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/spf13/cobra"
)

// seedDatasets grows in F1 (emission factors, GHG↔ISO mapping, integration
// definitions, national tariff schedule, ISO 50001 clause texts).
var seedDatasets = map[string]func(cmd *cobra.Command, cfg *config.Config) error{}

func newSeedCmd() *cobra.Command {
	var only string
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Load reference datasets idempotently",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			if only != "" {
				loader, ok := seedDatasets[only]
				if !ok {
					return fmt.Errorf("unknown dataset %q; available: %s", only, availableDatasets())
				}
				return loader(cmd, cfg)
			}
			if len(seedDatasets) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no reference datasets are defined yet")
				return nil
			}
			for name, loader := range seedDatasets {
				fmt.Fprintf(cmd.OutOrStdout(), "seeding %s\n", name)
				if err := loader(cmd, cfg); err != nil {
					return fmt.Errorf("seed %s: %w", name, err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "seed a single dataset by name")
	return cmd
}

func availableDatasets() string {
	if len(seedDatasets) == 0 {
		return "(none)"
	}
	names := make([]string, 0, len(seedDatasets))
	for name := range seedDatasets {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
```

with `sort` and `strings` imported.

- [ ] **Step 5: Register the commands in `root.go`**

```go
	root.AddCommand(
		newVersionCmd(),
		newConfigCheckCmd(),
		newMigrateCmd(),
		newAPICmd(),
		newWorkerCmd(),
		newSchedulerCmd(),
		newSeedCmd(),
	)
```

- [ ] **Step 6: Run the tests and the binary**

```bash
go test ./internal/cli/... -v
make build && ./bin/ekokod --help
```
Expected: PASS; `--help` lists api, config:check, migrate, scheduler, seed, version, worker.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): api, worker, scheduler and seed processes with graceful shutdown

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---

## Task 10: Architecture boundary test and golangci-lint

**Files:**
- Create: `internal/domain/doc.go`
- Create: `internal/arch/arch_test.go`
- Create: `.golangci.yml`
- Modify: `Makefile` (add `lint`, `tools`, `vuln`)

**Interfaces:**
- Produces: a test that fails the build if `internal/domain` gains a project import or an I/O import, or if any package outside `internal/cli` imports `internal/api`; plus a repository-wide assertion that `InsecureSkipVerify` does not appear.

- [ ] **Step 1: Write the failing test**

`internal/arch/arch_test.go`:

```go
package arch_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

const modulePath = "github.com/MErenTalan/ekokod-rewrite"

// forbiddenInDomain are packages that would give the domain layer I/O.
var forbiddenInDomain = []string{
	"net/http", "net", "database/sql", "os", "os/exec",
	"log", "log/slog", "io/ioutil", "path/filepath",
}

func loadPackages(t *testing.T, pattern string) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedImports, Dir: repoRoot(t)}
	pkgs, err := packages.Load(cfg, pattern)
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	return pkgs
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(wd, "..", "..")
}

// TestDomainHasNoProjectImports is named in the F0 acceptance criteria.
func TestDomainHasNoProjectImports(t *testing.T) {
	for _, pkg := range loadPackages(t, "./internal/domain/...") {
		for imported := range pkg.Imports {
			if strings.HasPrefix(imported, modulePath) &&
				!strings.HasPrefix(imported, modulePath+"/internal/domain") {
				t.Errorf("%s imports %s: internal/domain must not import project packages", pkg.PkgPath, imported)
			}
		}
	}
}

func TestDomainHasNoIOImports(t *testing.T) {
	for _, pkg := range loadPackages(t, "./internal/domain/...") {
		for imported := range pkg.Imports {
			for _, forbidden := range forbiddenInDomain {
				if imported == forbidden {
					t.Errorf("%s imports %s: internal/domain must be free of I/O", pkg.PkgPath, imported)
				}
			}
		}
	}
}

func TestOnlyTheCLIImportsTheAPIPackage(t *testing.T) {
	allowed := map[string]bool{
		modulePath + "/internal/cli": true,
		modulePath + "/internal/api": true,
	}
	for _, pkg := range loadPackages(t, "./...") {
		if allowed[pkg.PkgPath] || strings.HasPrefix(pkg.PkgPath, modulePath+"/internal/api") {
			continue
		}
		for imported := range pkg.Imports {
			if strings.HasPrefix(imported, modulePath+"/internal/api") {
				t.Errorf("%s imports %s: only internal/cli may wire the HTTP layer", pkg.PkgPath, imported)
			}
		}
	}
}

func TestStoreAndIntegrationDoNotImportAPIOrService(t *testing.T) {
	for _, pkg := range loadPackages(t, "./internal/store/...") {
		for imported := range pkg.Imports {
			require.False(t, strings.HasPrefix(imported, modulePath+"/internal/api"),
				"%s must not import the HTTP layer", pkg.PkgPath)
		}
	}
}

// TestNoTLSVerificationBypass guards removed-behaviour item 16.
func TestNoTLSVerificationBypass(t *testing.T) {
	root := filepath.Join(repoRoot(t), "internal")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), "InsecureSkipVerify") {
			t.Errorf("%s contains InsecureSkipVerify: TLS verification is never disabled", path)
		}
		return nil
	})
	require.NoError(t, err)
}
```

- [ ] **Step 2: Create the empty domain package and run the test**

`internal/domain/doc.go`:

```go
// Package domain holds the platform's business rules as pure functions:
// no database, no HTTP, no clock, no filesystem. Sub-packages arrive with the
// phases that need them — energy (F3), tariff, billing, reactive (F4),
// alarm (F7), carbon (F10), loadprofile and report.
//
// The dependency rule is enforced by internal/arch/arch_test.go.
package domain
```

```bash
go get golang.org/x/tools@latest
go mod tidy
go test ./internal/arch/... -v
```
Expected: PASS (the rules hold on the current tree).

- [ ] **Step 3: Prove the guard actually catches a violation**

```bash
cat > internal/domain/violation_test_helper.go <<'GO'
package domain

import _ "github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
GO
go test ./internal/arch/... -run TestDomainHasNoProjectImports
```
Expected: FAIL naming the forbidden import. Then remove the file:

```bash
rm internal/domain/violation_test_helper.go
go test ./internal/arch/... -v
```
Expected: PASS again.

- [ ] **Step 4: Write `.golangci.yml`**

```yaml
version: "2"

run:
  timeout: 5m
  build-tags:
    - integration

linters:
  enable:
    - bodyclose
    - depguard
    - errcheck
    - errorlint
    - gocritic
    - govet
    - ineffassign
    - misspell
    - noctx
    - revive
    - rowserrcheck
    - sqlclosecheck
    - staticcheck
    - unconvert
    - unused
    - wastedassign

  settings:
    depguard:
      rules:
        domain-is-pure:
          files:
            - "**/internal/domain/**"
          deny:
            - pkg: "net/http"
              desc: "internal/domain performs no I/O"
            - pkg: "database/sql"
              desc: "internal/domain performs no I/O"
            - pkg: "os"
              desc: "internal/domain performs no I/O"
            - pkg: "log/slog"
              desc: "internal/domain does not log"
            - pkg: "github.com/MErenTalan/ekokod-rewrite/internal/store"
              desc: "internal/domain imports nothing from the project"
            - pkg: "github.com/MErenTalan/ekokod-rewrite/internal/api"
              desc: "internal/domain imports nothing from the project"
        no-float-money:
          files:
            - "**/internal/domain/billing/**"
            - "**/internal/domain/tariff/**"
          deny:
            - pkg: "math/big"
              desc: "use github.com/shopspring/decimal for monetary arithmetic"
    revive:
      rules:
        - name: exported
          disabled: false
    errcheck:
      exclude-functions:
        - (net/http.ResponseWriter).Write

formatters:
  enable:
    - gofmt
    - goimports

issues:
  max-issues-per-linter: 0
  max-same-issues: 0
```

- [ ] **Step 5: Extend the Makefile**

```makefile
GOLANGCI_VERSION := v2.13.2

.PHONY: tools lint vuln

tools: ## Install pinned developer tools into $(GOPATH)/bin
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@latest

lint: ## Run gofmt check, go vet and golangci-lint
	@test -z "$$(gofmt -l ./cmd ./internal)" || (echo "gofmt needed:"; gofmt -l ./cmd ./internal; exit 1)
	go vet ./...
	golangci-lint run

vuln: ## Scan dependencies for known vulnerabilities
	govulncheck ./...
```

- [ ] **Step 6: Run the full gate**

```bash
make tools
make lint && make test && make build
make vuln
```
Expected: all pass; `govulncheck` reports no vulnerabilities.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): enforce the dependency rule with an import-boundary test and depguard

internal/domain may import nothing from the project and perform no I/O;
only internal/cli may wire the HTTP layer; InsecureSkipVerify is banned.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---
## Task 11: Dockerfile, Docker Compose and the `up`/`down` targets

**Files:**
- Create: `Dockerfile`, `docker-compose.yml`, `.env.docker`
- Modify: `Makefile` (`up`, `down`, `dev`, `logs`, `migrate`, `seed`, `generate`, `offline-bundle`)
- Create: `scripts/offline-bundle.sh`

**Interfaces:**
- Produces: `docker compose up -d` brings `postgres`, `redis`, `migrate` (one-shot), `api`, `worker`, `scheduler` and `web` to healthy, with only `api` (8080) and `web` (3000) published.

- [ ] **Step 1: Write the Dockerfile**

```dockerfile
# syntax=docker/dockerfile:1

FROM golang:1.27.1-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates tzdata
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build \
      -ldflags "-s -w \
        -X github.com/MErenTalan/ekokod-rewrite/internal/buildinfo.version=${VERSION} \
        -X github.com/MErenTalan/ekokod-rewrite/internal/buildinfo.commit=${COMMIT} \
        -X github.com/MErenTalan/ekokod-rewrite/internal/buildinfo.date=${DATE}" \
      -o /out/ekokod ./cmd/ekokod

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -u 10001 ekokod \
 && mkdir -p /var/lib/ekokod && chown ekokod:ekokod /var/lib/ekokod
ENV TZ=Europe/Istanbul
COPY --from=build /out/ekokod /usr/local/bin/ekokod
USER ekokod
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ekokod"]
CMD ["api"]
```

> Alpine rather than distroless: on-premise and air-gapped operators need a shell
> and `wget` for health checks and diagnosis without shipping extra tooling.

- [ ] **Step 2: Write `web/Dockerfile`** (used by Task 12; create it now so compose is complete)

```dockerfile
# syntax=docker/dockerfile:1

FROM node:24.20.0-alpine AS deps
WORKDIR /app
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

FROM node:24.20.0-alpine AS build
WORKDIR /app
RUN corepack enable
COPY --from=deps /app/node_modules ./node_modules
COPY web/ ./
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

FROM node:24.20.0-alpine
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 TZ=Europe/Istanbul
RUN adduser -D -u 10001 ekokod
COPY --from=build --chown=ekokod:ekokod /app/.next/standalone ./
COPY --from=build --chown=ekokod:ekokod /app/.next/static ./.next/static
COPY --from=build --chown=ekokod:ekokod /app/public ./public
USER ekokod
EXPOSE 3000
CMD ["node", "server.js"]
```

- [ ] **Step 3: Write `docker-compose.yml`**

```yaml
name: ekokod

x-ekokod-env: &ekokod-env
  env_file: [.env.docker]

services:
  postgres:
    image: timescale/timescaledb:2.30.0-pg16
    environment:
      POSTGRES_USER: ekokod
      POSTGRES_PASSWORD: ekokod
      POSTGRES_DB: ekokod
      TZ: Europe/Istanbul
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ekokod -d ekokod"]
      interval: 5s
      timeout: 5s
      retries: 20
    deploy:
      resources:
        limits: { cpus: "4", memory: 4g }

  redis:
    image: redis:7.4.11-alpine
    command: ["redis-server", "--appendonly", "yes"]
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 20
    deploy:
      resources:
        limits: { cpus: "1", memory: 512m }

  migrate:
    build:
      context: .
      args: { VERSION: "${VERSION:-dev}", COMMIT: "${COMMIT:-none}", DATE: "${DATE:-unknown}" }
    <<: *ekokod-env
    command: ["migrate", "up"]
    depends_on:
      postgres: { condition: service_healthy }
    restart: "no"

  api:
    build:
      context: .
      args: { VERSION: "${VERSION:-dev}", COMMIT: "${COMMIT:-none}", DATE: "${DATE:-unknown}" }
    <<: *ekokod-env
    command: ["api"]
    ports: ["8080:8080"]
    volumes:
      - artifacts:/var/lib/ekokod
    depends_on:
      postgres: { condition: service_healthy }
      redis: { condition: service_healthy }
      migrate: { condition: service_completed_successfully }
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health/ready"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 15s
    deploy:
      resources:
        limits: { cpus: "2", memory: 1g }

  worker:
    build:
      context: .
    <<: *ekokod-env
    command: ["worker"]
    volumes:
      - artifacts:/var/lib/ekokod
    depends_on:
      postgres: { condition: service_healthy }
      redis: { condition: service_healthy }
      migrate: { condition: service_completed_successfully }
    deploy:
      resources:
        limits: { cpus: "4", memory: 2g }

  scheduler:
    build:
      context: .
    <<: *ekokod-env
    command: ["scheduler"]
    depends_on:
      postgres: { condition: service_healthy }
      redis: { condition: service_healthy }
      migrate: { condition: service_completed_successfully }
    deploy:
      resources:
        limits: { cpus: "1", memory: 512m }

  web:
    build:
      context: .
      dockerfile: web/Dockerfile
    environment:
      NEXT_PUBLIC_API_URL: http://localhost:8080
      EKOKOD_INTERNAL_API_URL: http://api:8080
      TZ: Europe/Istanbul
    ports: ["3000:3000"]
    depends_on:
      api: { condition: service_healthy }
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:3000/api/ping"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 20s

volumes:
  postgres-data:
  redis-data:
  artifacts:
```

- [ ] **Step 4: Write `.env.docker`**

Same variables as `.env.example`, with container hostnames and generated
development secrets. Generate them with:

```bash
python3 - <<'PY' > .env.docker
import base64, os, secrets
print(f"""EKOKOD_ENV=development
EKOKOD_LOG_LEVEL=debug
EKOKOD_LOG_FORMAT=text
EKOKOD_TIMEZONE=Europe/Istanbul
EKOKOD_DEFAULT_LOCALE=tr
EKOKOD_HTTP_ADDR=:8080
EKOKOD_PUBLIC_URL=http://localhost:3000
EKOKOD_CORS_ORIGINS=http://localhost:3000
EKOKOD_DB_URL=postgres://ekokod:ekokod@postgres:5432/ekokod?sslmode=disable
EKOKOD_REDIS_URL=redis://redis:6379/0
EKOKOD_ENCRYPTION_KEY={base64.b64encode(os.urandom(32)).decode()}
EKOKOD_JWT_SIGNING_KEY={secrets.token_urlsafe(48)}
EKOKOD_PASSWORD_PEPPER={secrets.token_urlsafe(48)}
EKOKOD_DEVICE_FINGERPRINT_SECRET={secrets.token_urlsafe(48)}
EKOKOD_STORAGE_ROOT=/var/lib/ekokod
EKOKOD_EPIAS_USERNAME=development-only
EKOKOD_EPIAS_PASSWORD=development-only
EKOKOD_ML_URL=http://ml:8000
EKOKOD_ML_API_KEY=development-only
""", end="")
PY
```

Add `.env.docker` to `.gitignore` — it holds generated secrets and must never be committed.

- [ ] **Step 5: Extend the Makefile**

```makefile
.PHONY: up down dev logs ps migrate seed generate offline-bundle

up: ## Build and start the whole stack
	docker compose up -d --build

down: ## Stop the stack and remove volumes
	docker compose down -v

dev: up logs ## Start the stack and follow the logs

logs:
	docker compose logs -f api worker scheduler

ps:
	docker compose ps

migrate: ## Apply migrations inside the stack
	docker compose run --rm migrate migrate up

seed: ## Load reference datasets inside the stack
	docker compose run --rm api seed

generate: ## Regenerate sqlc types and the OpenAPI client (populated from F1)
	@echo "no generators configured yet"

offline-bundle: ## Build the air-gapped install bundle
	./scripts/offline-bundle.sh
```

- [ ] **Step 6: Write `scripts/offline-bundle.sh`**

```bash
#!/usr/bin/env bash
# Builds an air-gapped install bundle: container images, the compose file, a
# configuration template and an install script, in one tarball.
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
OUT_DIR="${OUT_DIR:-dist}"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

echo "==> Building images"
docker compose build

echo "==> Saving images"
mkdir -p "$STAGE/images"
# Unquoted on purpose: each image must arrive as its own argument.
# shellcheck disable=SC2046
docker save $(docker compose config --images) -o "$STAGE/images/ekokod-images.tar"

echo "==> Staging deployment files"
cp docker-compose.yml "$STAGE/"
cp .env.example "$STAGE/env.template"
cp scripts/install.sh "$STAGE/install.sh" 2>/dev/null || cat > "$STAGE/install.sh" <<'INSTALL'
#!/usr/bin/env bash
# Offline installer. Idempotent: safe to re-run for upgrades.
set -euo pipefail
command -v docker >/dev/null || { echo "docker is required"; exit 1; }
docker load -i images/ekokod-images.tar
[ -f .env.docker ] || { cp env.template .env.docker; echo "Edit .env.docker, then re-run."; exit 1; }
docker compose up -d
docker compose run --rm migrate migrate up
docker compose run --rm api seed
docker compose ps
INSTALL
chmod +x "$STAGE/install.sh"

mkdir -p "$OUT_DIR"
BUNDLE="$OUT_DIR/ekokod-offline-$VERSION.tar.gz"
tar -czf "$BUNDLE" -C "$STAGE" .
echo "==> $BUNDLE ($(du -h "$BUNDLE" | cut -f1))"
```

```bash
chmod +x scripts/offline-bundle.sh
```

- [ ] **Step 7: Bring the stack up and verify**

```bash
# The web service is created in Task 12; verify the Go stack only.
docker compose up -d --build postgres redis migrate api worker scheduler
docker compose ps
curl -s localhost:8080/health/ready | python3 -m json.tool
curl -s localhost:8080/version | python3 -m json.tool
docker compose logs scheduler | grep -i "leadership acquired"
docker compose exec -T redis redis-cli -n 1 keys 'asynq*' | head
```
Expected: every service `healthy` or `exited (0)` for `migrate`; readiness reports
`database`, `redis` and `migrations` all `ok`; the scheduler logs leadership.

- [ ] **Step 8: Prove leadership is exclusive in the running stack**

```bash
docker compose up -d --scale scheduler=2
sleep 15
docker compose logs scheduler | grep -c "leadership acquired"   # expect 1
docker compose up -d --scale scheduler=1
```

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): dockerfile, compose stack with health checks and offline bundle script

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---

## Task 12: Next.js shell with i18n and the readiness page

**Files:**
- Create: `web/package.json`, `tsconfig.json`, `next.config.ts`, `postcss.config.mjs`, `eslint.config.mjs`, `.prettierrc`, `vitest.config.ts`, `vitest.setup.ts`
- Create: `web/src/app/layout.tsx`, `page.tsx`, `globals.css`, `web/src/app/api/ping/route.ts`
- Create: `web/src/components/health-status.tsx`, `health-status.test.tsx`
- Create: `web/src/i18n/request.ts`, `web/src/lib/api.ts`
- Create: `web/messages/tr.json`, `web/messages/en.json`
- Create: `scripts/check-i18n-parity.mjs`
- Modify: `Makefile` (`web-install`, `web-lint`, `web-test`, `web-build`)

**Interfaces:**
- Produces: a page that fetches `/health/ready` from the API and renders each check; `pnpm check:i18n-parity` failing when a key exists in one locale and not the other.

- [ ] **Step 1: Write the failing component test**

`web/src/components/health-status.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { HealthStatus } from './health-status';
import messages from '../../messages/tr.json';

function renderWithIntl(ui: React.ReactNode) {
  return render(
    <NextIntlClientProvider locale="tr" messages={messages}>
      {ui}
    </NextIntlClientProvider>,
  );
}

describe('HealthStatus', () => {
  it('renders each dependency with its status', () => {
    renderWithIntl(
      <HealthStatus
        report={{
          status: 'ok',
          checks: [
            { name: 'database', status: 'ok', duration_ms: 3 },
            { name: 'redis', status: 'ok', duration_ms: 1 },
          ],
        }}
      />,
    );

    expect(screen.getByText('database')).toBeInTheDocument();
    expect(screen.getByText('redis')).toBeInTheDocument();
    expect(screen.getAllByText('ok')).toHaveLength(2);
  });

  it('shows the failure reason when a check failed', () => {
    renderWithIntl(
      <HealthStatus
        report={{
          status: 'degraded',
          checks: [{ name: 'redis', status: 'failed', error: 'connection refused', duration_ms: 5 }],
        }}
      />,
    );

    expect(screen.getByText(/connection refused/)).toBeInTheDocument();
  });

  it('renders an explicit unavailable state when the API cannot be reached', () => {
    renderWithIntl(<HealthStatus report={null} />);
    expect(screen.getByText(messages.health.unreachable)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Scaffold the workspace and run the test to watch it fail**

```bash
mkdir -p web/src/{app,components,i18n,lib} web/messages web/public
cd web && pnpm init && cd ..
```

`web/package.json`:

```json
{
  "name": "ekokod-web",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "next dev",
    "build": "next build",
    "start": "next start",
    "lint": "eslint .",
    "typecheck": "tsc --noEmit",
    "test": "vitest run",
    "check:i18n-parity": "node ../scripts/check-i18n-parity.mjs"
  },
  "dependencies": {
    "next": "15.5.25",
    "next-intl": "4.14.2",
    "react": "19.2.8",
    "react-dom": "19.2.8"
  },
  "devDependencies": {
    "@eslint/eslintrc": "^3.3.1",
    "@tailwindcss/postcss": "4.3.3",
    "@testing-library/jest-dom": "^6.6.3",
    "@testing-library/react": "^16.3.0",
    "@types/node": "^24.0.0",
    "@types/react": "^19.2.0",
    "@types/react-dom": "^19.2.0",
    "@vitejs/plugin-react": "^5.0.0",
    "eslint": "9.39.5",
    "eslint-config-next": "15.5.25",
    "jsdom": "^26.0.0",
    "prettier": "3.9.6",
    "tailwindcss": "4.3.3",
    "typescript": "5.9.3",
    "vitest": "5.0.0"
  },
  "packageManager": "pnpm@12.3.4"
}
```

```bash
cd web && pnpm install && pnpm test; cd ..
```
Expected: FAIL — `health-status.tsx` does not exist.

- [ ] **Step 3: Write the configuration files**

`web/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["dom", "dom.iterable", "ES2022"],
    "allowJs": false,
    "skipLibCheck": true,
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "preserve",
    "incremental": true,
    "plugins": [{ "name": "next" }],
    "paths": { "@/*": ["./src/*"] }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules"]
}
```

`web/next.config.ts`:

```ts
import createNextIntlPlugin from 'next-intl/plugin';
import type { NextConfig } from 'next';

const withNextIntl = createNextIntlPlugin('./src/i18n/request.ts');

const nextConfig: NextConfig = {
  output: 'standalone',
  reactStrictMode: true,
  poweredByHeader: false,
};

export default withNextIntl(nextConfig);
```

`web/postcss.config.mjs`:

```js
export default { plugins: { '@tailwindcss/postcss': {} } };
```

`web/eslint.config.mjs`:

```js
import { FlatCompat } from '@eslint/eslintrc';

const compat = new FlatCompat({ baseDirectory: import.meta.dirname });

export default [
  ...compat.extends('next/core-web-vitals', 'next/typescript'),
  { ignores: ['.next/**', 'node_modules/**'] },
];
```

`web/vitest.config.ts`:

```ts
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    setupFiles: ['./vitest.setup.ts'],
    globals: true,
  },
});
```

`web/vitest.setup.ts`:

```ts
import '@testing-library/jest-dom/vitest';
```

`web/.prettierrc`:

```json
{ "singleQuote": true, "semi": true, "printWidth": 100, "trailingComma": "all" }
```

- [ ] **Step 4: Write the i18n catalogues and parity check**

`web/messages/tr.json`:

```json
{
  "app": {
    "name": "ekokod",
    "tagline": "Enerji yönetim platformu"
  },
  "health": {
    "title": "Sistem durumu",
    "subtitle": "API ve bağımlılıklarının anlık durumu",
    "check": "Bileşen",
    "status": "Durum",
    "duration": "Süre",
    "unreachable": "API'ye ulaşılamıyor",
    "refresh": "Yenile"
  }
}
```

`web/messages/en.json`:

```json
{
  "app": {
    "name": "ekokod",
    "tagline": "Energy management platform"
  },
  "health": {
    "title": "System status",
    "subtitle": "Live status of the API and its dependencies",
    "check": "Component",
    "status": "Status",
    "duration": "Duration",
    "unreachable": "The API is unreachable",
    "refresh": "Refresh"
  }
}
```

`scripts/check-i18n-parity.mjs`:

```js
#!/usr/bin/env node
// Fails when the tr and en catalogues do not contain exactly the same keys.
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', 'web', 'messages');

const flatten = (obj, prefix = '') =>
  Object.entries(obj).flatMap(([key, value]) => {
    const path = prefix ? `${prefix}.${key}` : key;
    return value && typeof value === 'object' ? flatten(value, path) : [path];
  });

const load = (locale) => new Set(flatten(JSON.parse(readFileSync(join(root, `${locale}.json`), 'utf8'))));

const tr = load('tr');
const en = load('en');

const missingInEn = [...tr].filter((k) => !en.has(k));
const missingInTr = [...en].filter((k) => !tr.has(k));

if (missingInEn.length || missingInTr.length) {
  if (missingInEn.length) console.error(`Missing in en.json:\n  ${missingInEn.join('\n  ')}`);
  if (missingInTr.length) console.error(`Missing in tr.json:\n  ${missingInTr.join('\n  ')}`);
  process.exit(1);
}

console.log(`i18n parity ok — ${tr.size} keys in both locales`);
```

- [ ] **Step 5: Implement the app**

`web/src/i18n/request.ts`:

```ts
import { getRequestConfig } from 'next-intl/server';

export const locales = ['tr', 'en'] as const;
export const defaultLocale = 'tr';

export default getRequestConfig(async () => {
  const locale = defaultLocale;
  return { locale, messages: (await import(`../../messages/${locale}.json`)).default };
});
```

`web/src/lib/api.ts`:

```ts
export type HealthCheck = {
  name: string;
  status: 'ok' | 'failed';
  error?: string;
  duration_ms: number;
};

export type HealthReport = {
  status: 'ok' | 'degraded';
  checks: HealthCheck[];
};

const apiBase =
  process.env.EKOKOD_INTERNAL_API_URL ?? process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080';

/** Reads the API's readiness report. Returns null when the API is unreachable. */
export async function fetchHealth(): Promise<HealthReport | null> {
  try {
    const response = await fetch(`${apiBase}/health/ready`, { cache: 'no-store' });
    return (await response.json()) as HealthReport;
  } catch {
    return null;
  }
}
```

`web/src/components/health-status.tsx`:

```tsx
'use client';

import { useTranslations } from 'next-intl';

import type { HealthReport } from '@/lib/api';

export function HealthStatus({ report }: { report: HealthReport | null }) {
  const t = useTranslations('health');

  if (!report) {
    return (
      <p role="status" className="rounded-lg border border-red-300 bg-red-50 p-4 text-red-900">
        {t('unreachable')}
      </p>
    );
  }

  return (
    <table className="w-full border-collapse text-left text-sm">
      <thead>
        <tr className="border-b">
          <th className="py-2">{t('check')}</th>
          <th className="py-2">{t('status')}</th>
          <th className="py-2">{t('duration')}</th>
        </tr>
      </thead>
      <tbody>
        {report.checks.map((check) => (
          <tr key={check.name} className="border-b last:border-0">
            <td className="py-2">{check.name}</td>
            <td className="py-2">
              {check.status}
              {check.error ? <span className="ml-2 text-red-700">{check.error}</span> : null}
            </td>
            <td className="py-2">{check.duration_ms} ms</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
```

`web/src/app/layout.tsx`:

```tsx
import { NextIntlClientProvider } from 'next-intl';
import { getLocale, getMessages } from 'next-intl/server';
import type { Metadata } from 'next';

import './globals.css';

export const metadata: Metadata = {
  title: 'ekokod',
  description: 'Enerji yönetim platformu',
};

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const locale = await getLocale();
  const messages = await getMessages();

  return (
    <html lang={locale}>
      <body className="min-h-screen bg-white text-neutral-900 antialiased">
        <NextIntlClientProvider messages={messages}>{children}</NextIntlClientProvider>
      </body>
    </html>
  );
}
```

`web/src/app/page.tsx`:

```tsx
import { getTranslations } from 'next-intl/server';

import { HealthStatus } from '@/components/health-status';
import { fetchHealth } from '@/lib/api';

export const dynamic = 'force-dynamic';

export default async function HomePage() {
  const [t, report] = await Promise.all([getTranslations('health'), fetchHealth()]);

  return (
    <main className="mx-auto max-w-2xl p-8">
      <h1 className="text-2xl font-semibold">{t('title')}</h1>
      <p className="mt-1 mb-6 text-neutral-600">{t('subtitle')}</p>
      <HealthStatus report={report} />
    </main>
  );
}
```

`web/src/app/globals.css`:

```css
@import 'tailwindcss';
```

`web/src/app/api/ping/route.ts` (used by the compose health check):

```ts
export const dynamic = 'force-dynamic';

export function GET() {
  return Response.json({ status: 'ok' });
}
```

- [ ] **Step 6: Run the frontend gate**

```bash
cd web
pnpm test
pnpm typecheck
pnpm lint
pnpm check:i18n-parity
pnpm build
cd ..
```
Expected: all pass; the parity check prints the shared key count.

- [ ] **Step 7: Prove the parity check catches a real divergence**

```bash
cd web
python3 -c "
import json
d=json.load(open('messages/tr.json'))
d['health']['onlyInTurkish']='x'
json.dump(d,open('messages/tr.json','w'),ensure_ascii=False,indent=2)
"
pnpm check:i18n-parity; echo "exit=$?"   # expect exit=1 naming health.onlyInTurkish
git checkout messages/tr.json
cd ..
```

- [ ] **Step 8: Extend the Makefile and verify the page against the running API**

```makefile
.PHONY: web-install web-lint web-test web-build

web-install:
	cd web && pnpm install --frozen-lockfile

web-lint:
	cd web && pnpm lint && pnpm typecheck && pnpm check:i18n-parity

web-test:
	cd web && pnpm test

web-build:
	cd web && pnpm build
```

```bash
make up
curl -s localhost:3000 | grep -i "database"
```
Expected: the rendered page contains the `database`, `redis` and `migrations` rows.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(f0): next.js shell with next-intl catalogues and the readiness page

Turkish is the default locale; a CI check fails when the tr and en
catalogues diverge.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
```

---

## Task 13: Continuous integration

**Files:**
- Create: `.github/workflows/ci.yml`
- Modify: `Makefile` (`ci` aggregate target)

**Interfaces:**
- Produces: a workflow running on every push and pull request with jobs `go`, `integration` and `web`, each failing the build on any violation.

- [ ] **Step 1: Write `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

env:
  GO_VERSION: "1.27.1"
  NODE_VERSION: "24.20.0"

jobs:
  go:
    name: Go build, lint and unit tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: gofmt
        run: test -z "$(gofmt -l ./cmd ./internal)" || (gofmt -l ./cmd ./internal; exit 1)

      - name: go vet
        run: go vet ./...

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v2.13.2

      - name: Import-boundary check
        run: go test ./internal/arch/... -v

      - name: Unit tests
        run: go test ./... -race -coverprofile=coverage.out -covermode=atomic

      - name: Coverage summary
        run: go tool cover -func=coverage.out | tail -1

      - name: Build
        run: make build

      - name: govulncheck
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...

  integration:
    name: Integration tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: Integration tests (testcontainers)
        run: go test ./... -tags=integration -race -count=1 -timeout 20m

  web:
    name: Frontend
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: 12.3.4
      - uses: actions/setup-node@v4
        with:
          node-version: ${{ env.NODE_VERSION }}
          cache: pnpm
          cache-dependency-path: web/pnpm-lock.yaml

      - run: pnpm install --frozen-lockfile
      - run: pnpm lint
      - run: pnpm typecheck
      - run: pnpm check:i18n-parity
      - run: pnpm test
      - run: pnpm build
      - run: pnpm audit --audit-level=high
```

- [ ] **Step 2: Add the aggregate target to the Makefile**

```makefile
.PHONY: ci

ci: lint test build web-lint web-test web-build ## Everything CI runs, except integration tests
```

- [ ] **Step 3: Run the full local gate**

```bash
make ci
make test-integration
make vuln
```
Expected: all green.

- [ ] **Step 4: Commit and push**

```bash
git add -A
git commit -m "$(cat <<'EOF'
ci(f0): build, lint, unit, integration, vulnerability and frontend jobs

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01KcPJpPqmuaTpCZWUNRCG6h
EOF
)"
git push origin main
```

---

## F0 acceptance verification

Run these after Task 13 and paste the output into the phase report.

```bash
make lint && make test && make test-integration
make up && docker compose ps
curl -s localhost:8080/health/ready | python3 -m json.tool
env -i PATH="$PATH" ./bin/ekokod api; echo "exit=$?"          # non-zero, names the missing variable
go test ./internal/platform/config/... -run TestConfigRejectsMissingSecret -v
go test ./internal/scheduler/... -tags=integration -run TestLeaderElection -v
go test ./internal/job/... -tags=integration -run TestNoopTaskRoundTrip -v
go test ./internal/arch/... -run TestDomainHasNoProjectImports -v
curl -s localhost:3000 | grep -c "database"
```

Map each to the F0 checklist in `docs/rewrite/09-implementation-plan.md`:

| Acceptance criterion | Proven by |
|---|---|
| `docker compose up` brings every service healthy | Task 11 Step 7 |
| `/health/ready` returns 200 with database, Redis and migration status | Task 7 tests + Task 11 Step 7 |
| `make test` passes; coverage reported | Task 13 Step 3, CI coverage summary |
| Missing secret exits non-zero naming the variable | Task 2 `TestConfigRejectsMissingSecret` + `ekokod api` run |
| Exactly one scheduler leads; leadership transfers | Task 8 `TestLeaderElection` + Task 11 Step 8 |
| An enqueued asynq task is executed by the worker | Task 6 `TestNoopTaskRoundTrip` |
| `internal/domain` has no project imports | Task 10 `TestDomainHasNoProjectImports` |
| The Next.js page renders the health response | Task 12 Steps 6 and 8 |

---

## Self-review notes

- **Spec coverage.** Every bullet of F0's scope maps to a task: layout and subcommands (1, 9), platform package (2, 3, 4), store/postgres (5), api (7), Redis and asynq (6), scheduler (8), compose (11), Makefile (1, 10, 11, 12, 13), CI (13), Next.js (12).
- **Deliberately deferred, with the phase that owns it:** `sqlc` generation and the full schema (F1); the `generate` target is a stub that prints its status rather than pretending; `seed` has no datasets yet (F1); OpenTelemetry tracing is configuration-only until F15; the ML service is F13 and is not in the F0 compose file.
- **Naming.** `BCEM_*` → `EKOKOD_*` throughout; this plan is the record of that decision, and `docs/rewrite/appendix/env-reference.md` should be read with the prefix substituted.
