# 03 — Target Architecture

---

## 1. System shape

```
                         ┌──────────────────────────────┐
   Browser ──────────────│  Next.js 15 (SSR + client)   │
   Mobile app ───┐       │  public site + dashboard     │
                 │       └──────────────┬───────────────┘
                 │                      │ REST/JSON, cookie or bearer
                 └──────────────────────┤
                                        ▼
                         ┌──────────────────────────────┐
                         │  Go API service              │
                         │  http → service → domain     │
                         └───┬───────────┬──────────┬───┘
                             │           │          │
              ┌──────────────┘           │          └──────────────┐
              ▼                          ▼                         ▼
   ┌────────────────────┐   ┌────────────────────┐   ┌────────────────────┐
   │ PostgreSQL 16      │   │ Redis 7            │   │ Python ML service  │
   │ + TimescaleDB      │   │ cache + job queue  │   │ FastAPI            │
   └────────────────────┘   └─────────┬──────────┘   └────────────────────┘
              ▲                       │
              │                       ▼
              │        ┌──────────────────────────────┐
              └────────│  Go worker service           │
                       │  ingestion, compute, PDF,    │
                       │  reports, alarms, scheduler  │
                       └──────────────┬───────────────┘
                                      │ HTTPS
                                      ▼
                       OSOS · GridBox · ARIL · PM5340
                       iSolarCloud · EPİAŞ
```

The API service and the worker service are **the same binary** started with different flags
(`serve api`, `serve worker`, `serve scheduler`). One codebase, one set of domain rules, separate
processes so that heavy computation never competes with request latency.

---

## 2. Go backend

### 2.1 Package layout

```
cmd/
  bcem/                      # single binary; subcommands: api, worker, scheduler, migrate, seed, tool
internal/
  domain/                    # PURE business rules — no I/O, no DB, no clock, no HTTP
    energy/                  #   reading normalisation, consumption derivation, aggregation
    tariff/                  #   tariff resolution, PTF+YEKDEM pricing, KBK derivation
    billing/                 #   invoice composition
    reactive/                #   reactive penalty
    carbon/                  #   emission calculation, scope mapping
    loadprofile/             #   profile statistics
    report/                  #   report metric composition
  store/                     # persistence; one package per aggregate
    postgres/                #   pgx + sqlc generated queries, migrations
  integration/               # one adapter per provider behind a common interface
    osos/ gridbox/ aril/ pm5340/ isolar/ epias/
  job/                       # job definitions + handlers (asynq)
  scheduler/                 # cron definitions, leader election
  api/                       # HTTP: routing, DTOs, auth middleware, RBAC, validation
    v1/
  auth/                      # sessions, JWT, password policy, device binding
  pdf/                       # invoice and report rendering
  xlsx/                      # Excel rendering
  mail/                      # SMTP delivery, templates
  storage/                   # file artifact storage
  ml/                        # client for the Python service
  platform/                  # config, logging, telemetry, errors, crypto, ids
migrations/                  # SQL migrations (goose or atlas)
```

### 2.2 Dependency rule

```
api ──▶ service ──▶ domain
         │
         └──▶ store, integration, job, mail, pdf, storage
```

- `domain` imports nothing from the project. It is unit-testable with no fixtures beyond plain data.
- `store` and `integration` depend on `domain` types, never the reverse.
- `api` handlers contain **no business logic**: decode, authorise, call a service, encode.
- No package imports `api`.

This rule is enforced in CI with an import-boundary linter.

### 2.3 Chosen libraries

| Concern | Choice | Why |
|---------|--------|-----|
| HTTP router | `chi` | Small, stdlib-compatible, good middleware story |
| Postgres driver | `pgx/v5` | Native protocol, `numeric` support, `COPY` for bulk ingest |
| Query generation | `sqlc` | Type-safe queries from SQL; no ORM magic over the hypertables |
| Migrations | `goose` | Plain SQL, up/down, embeddable in the binary |
| Job queue | `asynq` | Redis-backed, retries, scheduling, dead-letter queue, web UI |
| Decimal | `shopspring/decimal` | Exact money arithmetic |
| Validation | `go-playground/validator` | Struct-tag validation on request DTOs |
| Logging | `log/slog` | Structured, stdlib |
| Metrics | `prometheus/client_golang` | Standard scrape endpoint |
| Tracing | OpenTelemetry | Optional, off by default in on-prem installs |
| PDF | `maroto` or direct `gofpdf` | Server-side, no headless browser dependency in an air-gapped install |
| Excel | `excelize` | Mature |
| Testing | stdlib + `testify` + `testcontainers-go` | Real Postgres in integration tests |

### 2.4 Error handling

- A single `platform/errors` type carrying a machine code, an HTTP status, a user-facing message
  (localised) and a wrapped cause.
- **No silent failures.** Every error is either handled deliberately, returned, or recorded as an
  operational message visible in the Messages screen. `log and continue` is not acceptable in job
  handlers — partial success is modelled explicitly (`processed`, `skipped`, `failed` counts with
  per-item reasons).

### 2.5 Multi-tenancy

Every tenant-scoped query is parameterised by `company_id`, and the accessible building set is
resolved once per request from the authenticated principal. The repository layer takes an explicit
`Scope` value; there is no "unscoped" query method outside the admin package.

---

## 3. Data layer

Detail in `04-data-model.md`. Architectural points:

- **`meter_readings` is a TimescaleDB hypertable** partitioned by time and spaced by
  `analyzer_id`. One row per (analyzer, timestamp, kind). This is the single source of truth for
  everything energy-related.
- **Consumption is not stored as a separate maintained collection.** Hourly, daily, monthly and
  yearly consumption are **continuous aggregates** over the hypertable, refreshed incrementally by
  TimescaleDB policies. There is nothing to keep in sync and nothing to grow unbounded.
- **Invoices are stored** — they are business documents, not derived views, and must remain
  reproducible exactly as issued even if inputs are later corrected.
- **Compression** — hypertable chunks older than 90 days are compressed. Retention of raw readings
  is configurable and defaults to unlimited (customers need multi-year history).
- **No `jsonb` for anything queried.** `jsonb` is permitted only for opaque provider payloads kept
  for forensic purposes, in a separate column that no business logic reads.

---

## 4. Background work

### 4.1 Job model

Every unit of background work is an `asynq` task with a typed payload:

| Job | Trigger |
|-----|---------|
| `integration.sync_analyzers` | scheduled + manual |
| `integration.fetch_readings` | fan-out from sync, one task per analyzer |
| `consumption.refresh` | after ingestion, per analyzer + affected range |
| `billing.generate` | scheduled + manual, per building + period |
| `billing.render_pdf` | after generate |
| `report.generate` | scheduled + manual, per building + period |
| `report.deliver` | after generate, when e-mail delivery is requested |
| `alarm.evaluate` | scheduled, per alarm rule |
| `alarm.notify` | fan-out from evaluate |
| `isolar.sync_plant` | scheduled + manual, per plant |
| `isolar.fetch_alarms` | scheduled |
| `epias.sync_prices` | scheduled |
| `forecast.run` | scheduled, per analyzer |
| `carbon.daily_accrual` | scheduled, per building |

Properties every job must have:

- **Idempotent** — keyed by `(job type, scope, period)`; re-running replaces rather than duplicates.
- **Resumable** — ingestion records a high-water mark per analyzer per reading kind and resumes
  from it.
- **Bounded** — explicit timeout, explicit max retries with exponential backoff, dead-letter on
  exhaustion.
- **Isolated** — one analyzer's failure fails only its own task.
- **Recorded** — a `job_runs` row with start, end, status, scope, counts and error detail.

### 4.2 Scheduling

The `scheduler` process holds a **Postgres advisory lock** for leader election and enqueues tasks
on their cron schedule. Only the leader enqueues; any number of workers consume. There is no OS
cron, no `curl`, and no shared API key.

Default schedules (all configurable, all timezone Europe/Istanbul):

| Job | Schedule |
|-----|----------|
| `epias.sync_prices` | `0 14 * * *` (after day-ahead publication) |
| `integration.sync_analyzers` | `0 3 * * *` |
| `alarm.evaluate` | `0 * * * *` |
| `billing.generate` | `0 5 * * *` (only buildings whose period has closed) |
| `forecast.run` | `0 4 * * *` |
| `carbon.daily_accrual` | `30 4 * * *` |
| `report.generate` (monthly) | `0 6 2 * *` |
| `report.generate` (yearly) | `0 7 3 1 *` |

Every scheduled job is also manually triggerable from an operator screen with an explicit scope and
period.

---

## 5. Frontend

### 5.1 Structure

```
app/
  (public)/                  # marketing site — SSR/SSG, indexable
    page.tsx  about/  references/  toolkit/  documents/  pricing/
    request-demo/  contact/  blog/  bill-calculator/
  (auth)/                    # login, forgot password, two-step
  (app)/                     # authenticated product — client-rendered under a shared shell
    dashboard/  consumption/  load-profile/  forecast/  solar-plants/
    financial/  renewable/  bills/  tariffs/  alarms/  messages/
    reports/  settings/  carbon/  iso-50001/  calendar/
components/
  ui/                        # design-system primitives
  charts/                    # chart wrappers with a shared theme
  map/                       # map wrapper
features/                    # one folder per domain feature: hooks, api client, components
lib/
  api/                       # generated client from the backend's OpenAPI spec
  i18n/                      # tr + en catalogues
```

### 5.2 Decisions

- **Data fetching** — TanStack Query against a **generated client**. The Go service publishes an
  OpenAPI 3.1 document; the TypeScript client is generated from it in CI. Request and response types
  are never hand-written.
- **Auth** — httpOnly, `SameSite=Strict`, `Secure` cookies for the browser. Bearer tokens for the
  mobile app. Token refresh is transparent.
- **Charts** — one charting library across the whole product with a single shared theme derived from
  the design tokens. Legacy used ApexCharts, Recharts, DevExtreme and Chart.js simultaneously; that
  is not repeated.
- **Maps** — MapLibre GL with an OSM raster or vector tile source. The tile endpoint is configurable
  so on-premise installs can point at an internal tile server; the map degrades to a coordinate
  list when no tile source is reachable.
- **State** — server state in TanStack Query. Client state (selected building/analyzer, date range,
  theme, language) in a small typed store persisted to `localStorage`, scoped per user.
- **i18n** — `next-intl`. The Turkish catalogue is authoritative; every key must exist in both
  locales, enforced by a CI check.
- **Accessibility** — keyboard-navigable, labelled form controls, WCAG AA contrast in both themes,
  charts accompanied by their data table.
- **Performance budget** — dashboard interactive under 2 s on a mid-range laptop over a typical
  connection; route-level code splitting; charts and the map loaded lazily.

---

## 6. Python ML service

### 6.1 Scope

Forecasting and anomaly detection only. Nothing else.

### 6.2 Hard boundary ⚠️

**The ML service has no database credentials and no database driver.** In the legacy system it
connected to MongoDB directly, queried collections by hand, and duplicated business logic. That is
removed.

The Go service sends the ML service everything it needs in the request body: the historical series,
the covariates (weather, day type, vacation flags), and the horizon. The ML service returns
predictions. It is stateless apart from model artifacts on disk.

### 6.3 Interface

```
POST /v1/forecast
  { series_id, granularity, horizon,
    history: [{ts, value}],
    covariates: {weather: [...], day_type: [...], vacation: [...]} }
  → { status, timestamps[], median[], p10[], p90[],
      used_covariates[], gaps: [{start, end, missing_hours}] }

POST /v1/anomaly
  { series_id, ts, actual, history: [{ts, value}] }
  → { is_anomaly, score, expected, lower, upper, method }

GET  /v1/models          → available models, versions, training dates
POST /v1/train           → retrain from a supplied dataset
GET  /health             → liveness/readiness
```

- Versioned path (`/v1`), stable contract, documented with OpenAPI.
- Authenticated with a shared secret over the internal network.
- Every response carries the model id and version used.
- `status` is explicit: `ok`, `insufficient_data`, `no_data`, `model_error`.

### 6.4 Models

Carried over from the legacy service: a seasonal-naive/statistical baseline, a random-forest model
over engineered features (calendar, weather, day type, rolling means), an LSTM, and an external
foundation-model forecaster (Chronos). The Go service picks a model per request or accepts the
service's default; the response always states what was used.

**A statistical baseline must always be available** so that forecasting degrades rather than fails
when heavier models are unavailable.

---

## 7. Security

| Area | Requirement |
|------|-------------|
| Passwords | bcrypt (cost ≥ 12) with a server-side pepper from configuration. Password history retained to block reuse. Policy enforced server-side. |
| Sessions | Short-lived access token + rotating refresh token. Device fingerprint bound to the session; mismatch invalidates. |
| Integration credentials | Encrypted at rest with AES-256-GCM using a key from configuration. **Never** returned by any API, in any form, to any role. Write-only from the UI's perspective. |
| Transport | TLS terminated at the reverse proxy. HSTS enabled. |
| Authorisation | Enforced in the service layer, not the UI. Every endpoint has an explicit permission declaration and a test proving each role's access. |
| Rate limiting | Per-IP and per-principal. Stricter buckets for auth endpoints and ML endpoints. |
| File paths | All artifact paths are constructed from validated components and confirmed to resolve inside the storage root. No user input reaches a path directly. |
| Uploads | Size limit (30 MB), extension and content-type allow-list, magic-byte sniffing, stored outside any web root, served through an authorising handler. |
| Secrets | From environment or a mounted secrets file. Never committed, never logged. Startup fails if a required secret is missing — no insecure defaults. |
| Audit | Every mutation records who, what, when and from where. |
| SQL | Parameterised queries only; `sqlc`-generated. |
| Dependencies | `govulncheck` and `npm audit` in CI. |

---

## 8. Observability

- **Structured logs** — JSON, with request id, principal, company id and job id propagated.
- **Metrics** — HTTP latency and error rate by route; job duration, success and failure by type;
  integration call latency and error rate by provider; queue depth; database pool saturation.
- **Health** — `/health/live` (process up) and `/health/ready` (database, Redis and migrations OK).
- **Operational messages** — every job run and every alarm evaluation writes a record visible in
  the Messages screen, so a customer administrator can see what the system did without shell access.

---

## 9. Configuration

Single source: environment variables, with a documented, validated schema and a `config:check`
subcommand. Full list in `appendix/env-reference.md`.

Principles: no secret has a default; every non-secret has a sensible default; the process refuses to
start on an invalid configuration rather than degrading silently.

---

## 10. Deployment

### 10.1 Docker Compose

Services: `api`, `worker`, `scheduler`, `web` (Next.js), `postgres` (TimescaleDB image), `redis`,
`ml`, `proxy` (Caddy or nginx).

- Only the proxy publishes ports. Everything else is on the internal network.
- Named volumes for Postgres data, Redis AOF, file artifacts and ML model artifacts.
- Health checks and dependency ordering on every service.
- Migrations run as a one-shot job before `api` and `worker` start.
- Resource limits set explicitly.

### 10.2 Offline / air-gapped install

The legacy product shipped an offline install bundle; this capability is preserved and improved.

`make offline-bundle` produces a single tarball containing all container images as `docker save`
archives, the compose file, an install script, a configuration template, and the seed data
(emission factors, national tariff schedule, integration provider definitions).

`install.sh` on the target host: verifies prerequisites, loads images, generates secrets, writes
the configuration, starts the stack, runs migrations, seeds, creates the first admin user, and
prints a summary. It is idempotent and re-runnable for upgrades.

### 10.3 Backups

A scheduled `pg_dump` plus an artifact-directory archive, written to a configurable location, with
a documented and **tested** restore procedure. The legacy system had no verified restore path.

---

## 11. Testing strategy

| Level | Scope | Requirement |
|-------|-------|-------------|
| Domain unit tests | `internal/domain/**` | Every rule in `02-domain-rules.md`. **Golden-file tests** with real anonymised fixtures. This is the highest-priority suite and is written before the code it covers. |
| Store integration tests | `internal/store/**` | Against a real Postgres+TimescaleDB via testcontainers. Includes continuous-aggregate correctness. |
| Integration adapter tests | `internal/integration/**` | Against recorded HTTP fixtures per provider, covering success, partial data, auth failure, malformed payloads and pagination. |
| Job tests | `internal/job/**` | Idempotency, resumability and isolated failure, explicitly. |
| API contract tests | `internal/api/**` | Every endpoint × every role, asserting the authorisation matrix. |
| Frontend component tests | `components/**` | Rendering, empty states, error states. |
| End-to-end | Playwright | The critical paths: login, dashboard, consumption analysis, invoice generation and download, report generation, alarm creation, carbon data entry. |
| Migration verification | `08-migration.md` | Reconciliation of migrated data against legacy figures. |

CI runs everything on every change. A pull request that reduces domain coverage fails.
