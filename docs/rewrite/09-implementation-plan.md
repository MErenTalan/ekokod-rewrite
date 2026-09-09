# 09 — Implementation Plan

> **For agentic workers:** This is a **master plan**. It defines sixteen phases (F0–F15), each of
> which is a subsystem large enough to need its own task-level plan.
>
> **At the start of every phase**, produce a task-level plan for that phase using the
> `superpowers:writing-plans` skill, then execute it with `superpowers:subagent-driven-development`
> or `superpowers:executing-plans`. Do not attempt a phase without first decomposing it into
> bite-sized, individually testable tasks.

**Goal:** Rebuild BCEM Energy — a multi-tenant energy management platform for the Turkish
electricity market — from zero on Go + PostgreSQL/TimescaleDB + Next.js, with complete feature
parity, correct calculations, reliable ingestion, and a verified migration from the running
production system.

**Architecture:** A single Go binary run as three processes (API, worker, scheduler) over
PostgreSQL 16 with TimescaleDB hypertables and continuous aggregates, with Redis for cache and
job queue, a Next.js 15 frontend consuming a generated OpenAPI client, and a stateless Python
FastAPI service for forecasting. All business rules live in a dependency-free `domain` package
locked by golden-file tests.

**Tech stack:** Go 1.23+ · chi · pgx/v5 · sqlc · goose · asynq · shopspring/decimal · excelize ·
PostgreSQL 16 + TimescaleDB · Redis 7 · Next.js 15 · React 19 · TypeScript · Tailwind CSS ·
Radix UI · TanStack Query · MapLibre GL · Python 3.12 + FastAPI · Docker Compose

**Spec:** `docs/01-project-context.md` through `docs/08-migration.md`, plus
`docs/10-removed-behaviours.md`.

---

## Global constraints

Every task's requirements implicitly include this section.

- **Go 1.23 or newer.** Module path `github.com/<org>/bcem`.
- **Node 22 LTS or newer.** Package manager `pnpm`.
- **PostgreSQL 16 with TimescaleDB 2.x.** No feature may depend on a later major version.
- **Python 3.12** for the ML service.
- **Money and energy are `decimal`/`numeric`.** A `float64` in a monetary or energy calculation
  path is a build-breaking defect.
- **Timestamps are `timestamptz`, stored UTC, evaluated in `Europe/Istanbul`.**
- **`internal/domain` imports nothing from the project** and has no I/O. Enforced by an
  import-boundary linter in CI.
- **HTTP handlers contain no business logic.** Decode, authorise, call a service, encode.
- **No secret is ever logged.** No secret has a default value. Startup fails on a missing secret.
- **TLS verification is never disabled.** Self-signed provider certificates are pinned in
  configuration.
- **Turkish is the default locale**, English fully supported. Every UI string exists in both
  catalogues; a missing key fails CI.
- **All UI work invokes the `ui-ux-pro-max` skill first.** See `docs/07-design-system.md` §0.
- **Every phase ends green:** `make lint test build` passes and the acceptance criteria are
  demonstrated with real command output.
- **Commit frequently**, one logical change per commit, conventional-commit messages.

---

## Phase dependency graph

```
F0 ──▶ F1 ──▶ F2 ──▶ F3 ──▶ F4 ──┬──▶ F7 ──▶ F8
                │                 ├──▶ F9
                │                 ├──▶ F10
                ▼                 └──▶ F11
               F13 (ML)
                                  F5 ──▶ F6 ──▶ (all UI phases)
                                          └──▶ F12
F14 depends on F1–F11 · F15 depends on everything
```

F5 (design system) may start in parallel with F2–F4; it blocks F6 and every subsequent UI phase.
F13 (ML service) may start any time after F3.

---

## F0 — Foundation

**Goal:** A running, tested, deployable skeleton. No business features.

**Prerequisites:** none.

### Scope

- Repository layout exactly as specified in `03-target-architecture.md` §2.1.
- `cmd/bcem` with subcommands `api`, `worker`, `scheduler`, `migrate`, `seed`, `config:check`,
  `version`.
- `internal/platform`: typed configuration loaded and validated from the environment, structured
  logging (`slog`), the error type from §2.4, request-id propagation, AES-256-GCM helpers, UUID
  generation, a `Clock` interface so time is injectable.
- `internal/store/postgres`: pgx pool, health check, goose migration runner embedded in the binary.
- `internal/api`: chi router, request-id / logging / recovery / CORS / rate-limit middleware,
  `/health/live`, `/health/ready`, `/version`, `/metrics`.
- Redis client with connection pooling and a health check.
- `asynq` client, server and a `noop` task proving the round trip.
- Scheduler process with Postgres advisory-lock leader election.
- Docker Compose with `postgres` (TimescaleDB image), `redis`, `api`, `worker`, `scheduler`,
  and a one-shot `migrate` job with health checks and dependency ordering.
- `Makefile`: `dev up down lint test test-integration build migrate seed generate offline-bundle`.
- CI: build, vet, `golangci-lint`, unit tests, integration tests with testcontainers,
  `govulncheck`, and the import-boundary check.
- Next.js app scaffolded with TypeScript, Tailwind, ESLint, Prettier, `next-intl` with empty `tr`
  and `en` catalogues, and one page that calls `/health/ready` and renders the result.

### Acceptance criteria

- [ ] `docker compose up` brings every service to healthy.
- [ ] `curl localhost:8080/health/ready` returns 200 with database, Redis and migration status.
- [ ] `make test` passes; coverage is reported.
- [ ] Starting the API with a required secret unset exits non-zero with a message naming the
      variable.
- [ ] Two scheduler instances started simultaneously: exactly one acquires leadership; killing it
      transfers leadership to the other within the configured interval — demonstrated in a test.
- [ ] An `asynq` task enqueued by the API is executed by the worker and its result observed.
- [ ] A test asserting `internal/domain` has no project imports passes.
- [ ] The Next.js page renders the health response.

### Verification

```bash
make lint && make test && make test-integration
docker compose up -d && docker compose ps
curl -s localhost:8080/health/ready | jq
go test ./internal/platform/... -run TestConfigRejectsMissingSecret -v
go test ./internal/scheduler/... -run TestLeaderElection -v
```

---

## F1 — Data model and migration framework

**Goal:** The complete schema exists, is versioned, and is proven correct.

**Prerequisites:** F0.

### Scope

- Every table, type, index and constraint from `04-data-model.md`, as ordered goose migrations
  with working `down` migrations.
- Hypertables: `meter_readings`, `plant_production`, `market_prices_hourly`, `forecasts`, with
  partitioning, chunk intervals, compression settings and compression policies.
- Continuous aggregates `consumption_hourly`, `consumption_daily`, `consumption_monthly`,
  `consumption_yearly`, `plant_production_daily`, `plant_production_monthly`, with refresh policies.
- `sqlc` configuration and generated types for every table.
- Repository interfaces and Postgres implementations for every aggregate, each taking an explicit
  `Scope` (company + accessible building set). **No unscoped query method exists outside
  `internal/store/postgres/admin`.**
- Seed loader (`bcem seed`) with idempotent loading of: emission-factor master catalogue,
  GHG↔ISO mapping, integration provider definitions, national tariff schedule, ISO 50001 clause
  texts and templates.
- Test fixtures: a factory package producing a complete tenant (company, users of every role,
  buildings, analyzers, plants, tariffs) for use by every later phase.

### Acceptance criteria

- [ ] `bcem migrate up` on an empty database creates the full schema; `down` reverses it cleanly;
      `up` again succeeds.
- [ ] Inserting 1,000,000 synthetic readings across 100 analyzers completes and produces the
      expected chunk count; a 1-month range query for one analyzer touches only the expected
      chunks (verified with `EXPLAIN`).
- [ ] Continuous aggregates return correct values against a hand-computed fixture.
- [ ] Compression policy compresses a chunk older than the threshold, and queries still return
      correct results from it.
- [ ] Every repository method is covered by an integration test against a real database.
- [ ] A test proves a scoped repository cannot return another company's rows.
- [ ] `bcem seed` run twice produces the same row counts.

### Verification

```bash
make migrate && bcem migrate down --all && bcem migrate up
go test ./internal/store/... -tags=integration -v
go test ./internal/store/postgres -run TestScopeIsolation -v
psql -c "select hypertable_name, num_chunks from timescaledb_information.hypertables;"
```

---

## F2 — Integration layer

**Goal:** All six providers ingest reliably, resumably and idempotently.

**Prerequisites:** F1.

### Scope

- The `MeterDataSource` interface and shared infrastructure: HTTP client with timeouts, retry with
  exponential backoff and jitter, typed errors, per-provider rate limiting, certificate pinning,
  URL-template substitution.
- Adapters: `osos`, `gridbox`, `aril`, `pm5340`, `isolar`, `epias` — each implementing exactly the
  behaviour in `06-integrations.md`, including its field mapping, date semantics and multiplier
  rules.
- The ingestion pipeline: `integration.sync_analyzers` → `integration.fetch_readings`, with
  `ingestion_cursors`, `COPY`-into-staging then upsert, validation and rejection reporting.
- Reading validation per `06-integrations.md` §9.
- Backfill job with explicit range, chunking and resumability.
- Credential encryption and the write-only credential API surface.
- iSolarCloud OAuth: authorisation URL, callback exchange, refresh under a per-company lock.
- EPİAŞ price sync job writing to `market_prices_hourly` and `yekdem_monthly`.

### Acceptance criteria

- [ ] Every adapter has recorded fixtures covering: success, empty result, partial data,
      authentication failure, malformed payload, rate limiting, and pagination — all passing.
- [ ] Fetching the same window twice produces no duplicate rows.
- [ ] Interrupting a fetch and re-running resumes from the cursor and loses nothing.
- [ ] One analyzer failing does not abort the run; the `job_runs` row records
      processed/skipped/failed counts and the failure reason.
- [ ] GridBox multiplier resolution is tested for all three paths, including the fallback-to-1
      case which must emit a warning.
- [ ] GridBox `RI`→inductive and `RC`→capacitive is asserted by an explicit test.
- [ ] An offset-less provider timestamp is stored as the correct UTC instant for Europe/Istanbul.
- [ ] ARIL rows produce `NULL`, not `0`, in the T1/T2/T3 registers.
- [ ] PM5340 interval generation accumulates into the cumulative register correctly across a gap,
      an out-of-order batch and a duplicate fetch.
- [ ] A negative index delta creates an unresolved `consumption_anomalies` row.
- [ ] No test passes with TLS verification disabled.

### Verification

```bash
go test ./internal/integration/... -v
go test ./internal/job/... -run TestIngestion -tags=integration -v
go test ./internal/integration/... -run TestIdempotent -v
grep -rn "InsecureSkipVerify" internal/ && exit 1 || echo "no TLS bypass"
```

---

## F3 — Consumption engine

**Goal:** Correct energy derivation from raw readings, at every aggregation level.

**Prerequisites:** F1, F2.

### Scope

- `internal/domain/energy`: pure functions implementing `02-domain-rules.md` §2–§3 — boundary
  reading selection, register differencing, meter-reset handling, ratios, max demand, activity
  status.
- The two distinct consumption paths made explicit in code:
  `analytics.Consumption(...)` reading continuous aggregates, and `billing.Consumption(...)`
  reading `meter_readings` at true period boundaries (`04-data-model.md` §4.3).
- `consumption.refresh` job triggered by ingestion over the affected range.
- Suspect-period handling: creation, listing and operator resolution.
- `internal/domain/loadprofile`: profile averaging and statistics per `02-domain-rules.md` §10.3,
  using the company's vacation configuration for weekday/weekend classification.
- API endpoints from `05-api-contract.md` §5.

### Acceptance criteria

- [ ] **Golden-file tests** for consumption derivation against hand-computed fixtures covering:
      a normal period, a period with missing readings, a period whose boundary readings are
      identical, a meter reset with a reset event, a meter reset without one, and a rollover.
- [ ] A negative delta without a reset event yields `NULL` consumption, a `suspect` flag and an
      operational message — asserted, not assumed.
- [ ] Hourly, daily, monthly and yearly figures each derive from their own boundaries; summing
      the level below is asserted **not** to be how they are produced.
- [ ] `billing.Consumption` and `analytics.Consumption` are shown to differ at a bucket boundary
      by the expected step, documenting the trade-off in a test.
- [ ] Monthly consumption prefers `billing`-kind readings when present.
- [ ] Load profile statistics match hand-computed values for a fixture with known weekday/weekend
      and seasonal distribution.
- [ ] Vacation days configured on the company move a date from weekday to weekend in the profile
      split.
- [ ] A zero-consumption period produces a null ratio, not a ratio computed against a substituted 1.

### Verification

```bash
go test ./internal/domain/energy/... -v
go test ./internal/domain/loadprofile/... -v
go test ./internal/service/consumption/... -tags=integration -v
go test ./internal/domain/... -run TestGolden -v
```

---

## F4 — Tariff and billing engine

**Goal:** Invoices that match real utility invoices. The highest-risk phase in the project.

**Prerequisites:** F3.

### Scope

- `internal/domain/tariff`: tariff resolution by effective date; PTF+YEKDEM hourly and
  period-average pricing; manual YEKDEM override; KBK derivation from icmal with median
  aggregation, stability metrics and back-calculation validation.
- `internal/domain/reactive`: the penalty rules including the sub-9 kW exemption.
- `internal/domain/billing`: invoice period from the cut-off day; net consumption and the three
  generation modes; tiered energy pricing; distribution; green energy; power and demand-overrun;
  named taxes; VAT; total; the complete invoice output structure.
- Building and company aggregation.
- İcmal parser: alias-tolerant column matching, Turkish number parsing, cancelled-row exclusion.
- `billing.generate` and `billing.render_pdf` jobs.
- PDF invoice rendering for analyzer, building and company scope.
- Hourly PTF detail persistence and Excel export.
- API endpoints from `05-api-contract.md` §6 and §7.

### Acceptance criteria

- [ ] **Golden-file tests** for every rule in `02-domain-rules.md` §4–§7. At minimum:
      single-rate untiered · single-rate tiered residential · single-rate tiered commercial ·
      multi-time (asserting **no** tiering) · each of the three generation modes · reactive penalty
      applied · reactive exempt for monomial, residential, lighting, generation present, and
      sub-9 kW · demand overrun · named taxes · PTF hourly · PTF period-average · manual YEKDEM
      override.
- [ ] A test asserts distribution is charged on **total** consumption, not the low tier.
- [ ] A test asserts a multi-time tariff never uses an averaged T1/T2/T3 price.
- [ ] A test asserts a tariff without `vat_rate` is rejected at creation.
- [ ] A test asserts PTF missing-hours above tolerance flags the invoice instead of issuing it.
- [ ] A test asserts each KBK coefficient drives only its own charge and a null coefficient
      produces a zero line, with no fallback and no magic default.
- [ ] A test asserts an unresolved `consumption_anomalies` row blocks invoice issue.
- [ ] İcmal parsing round-trips a real anonymised file; derived coefficients back-calculate the
      energy charge within 2 %.
- [ ] Building invoice ≠ sum of analyzer invoices where tiering applies — asserted deliberately.
- [ ] Generated PDFs render correct Turkish characters and are byte-stable for identical input.
- [ ] **At least one invoice computed by the engine is compared against a real utility invoice
      supplied by the product owner**, and the difference is explained line by line.

### Verification

```bash
go test ./internal/domain/tariff/... -v
go test ./internal/domain/reactive/... -v
go test ./internal/domain/billing/... -v
go test ./internal/domain/billing -run TestGolden -v
go test ./internal/service/billing/... -tags=integration -v
bcem tool compare-invoice --fixture testdata/real-invoices/<file>.json
```

> Do not proceed to F5 until the real-invoice comparison has been reviewed and accepted.

---

## F5 — Design system

**Goal:** A complete, token-driven component library. No product screens yet.

**Prerequisites:** F0. May run in parallel with F2–F4.

### Scope

> **Start by invoking the `ui-ux-pro-max` skill** and persisting the master design system, exactly
> as specified in `docs/07-design-system.md` §0. Read the persisted `MASTER.md` before writing any
> component. This is not optional and is not a formality — the visual direction comes from the
> skill, not from your defaults.

- Design tokens as CSS custom properties for light and dark, per `07-design-system.md` §2, wired
  into the Tailwind theme. **No raw hex in any component.**
- Self-hosted, subset fonts: Lexend, Source Sans 3, JetBrains Mono (Latin + Latin Extended-A).
- The full component inventory from `07-design-system.md` §6, built on Radix primitives.
- Chart wrappers over a single charting library, themed from the tokens, with units, legends,
  tooltips, dash-pattern differentiation and a data-table equivalent built in.
- The application shell: sidebar with the specified groups and disabled entries, top bar,
  breadcrumb, page header, sticky filter bar, theme customiser.
- `next-intl` set up with the `tr` and `en` catalogues seeded from
  `docs/appendix/i18n-tr.json` and `i18n-en.json`.
- Storybook (or an equivalent component gallery) covering every component in both themes and both
  locales.
- Automated contrast checking over every token pair, in CI.
- The i18n key-parity check in CI.

### Acceptance criteria

- [ ] `design-system/bcem-energy/MASTER.md` exists and was produced by the skill.
- [ ] Zero raw hex values in `components/` — enforced by a lint rule.
- [ ] Contrast check passes for every token pair in both themes.
- [ ] Every component renders correctly in light and dark, and in Turkish and English.
- [ ] Keyboard navigation works through every interactive component with a visible focus ring.
- [ ] `prefers-reduced-motion` is honoured, verified in a test.
- [ ] Shell renders correctly at 375, 768, 1024 and 1440 px with no horizontal page scroll.
- [ ] Disabled navigation entries render disabled with an explanatory tooltip.
- [ ] Charts render with units, legend, tooltip and a toggleable data table.
- [ ] The pre-delivery checklist in `07-design-system.md` §11 is completed and its output shown.

### Verification

```bash
pnpm lint && pnpm test
pnpm run check:contrast
pnpm run check:i18n-parity
pnpm storybook:build
pnpm exec playwright test tests/a11y/
```

---

## F6 — Core application screens

**Goal:** Login, dashboard, consumption, load profile, and settings — the product's spine.

**Prerequisites:** F3, F5.

### Scope

- Auth: login with remember-me, device binding, forgot password, reset, change password, session
  list. Backend sessions, token rotation, password policy and history.
- Authorisation middleware and the full role matrix from `01-project-context.md` §2, with a
  contract test per endpoint per role.
- Generated TypeScript API client from the OpenAPI document, wired into CI.
- **Dashboard** (`01-project-context.md` §7.2): building map, analyzer map, building list, latest
  bill card, monthly reactive penalty card, consumption chart with the refresh actions, and the
  **restored** sectoral comparison table with CSV export.
- **Consumption** (§7.3): filter bar, summary cards, both tabs, the full column set, the alarm-check
  row action, and CSV/Excel/print export.
- **Load Profile** (§7.4): all four tabs, all eight seasonal profiles with correct labels, the
  comparison overlay, and the statistics tab.
- **Settings** (§7.15): all tabs with the visibility matrix, including building, analyzer, plant,
  user, company, integration-definition, integration-credential and SMTP management.
- **Calendar** (§7.18): month/week/day/agenda views, event CRUD with colour and all-day, weekend-day
  configuration and vacation periods. The vacation configuration is consumed by the load-profile
  weekday/weekend split (F3) and the ML day-type covariates (F13), so it must be correct here.
- **Mobile API** (§7.1): `/mobile/auth/login`, `/refresh` and `/me` issuing and rotating bearer
  tokens, with every other endpoint accepting bearer authentication alongside cookies.
- Demo role: the synthetic dataset served by every data endpoint.

### Acceptance criteria

- [ ] A user of each of the six roles can log in and sees exactly the navigation and tabs the
      matrix specifies.
- [ ] Every mutating endpoint returns 403 for both read-only roles — one test per endpoint.
- [ ] A `building-admin` cannot read another building's data through any endpoint, including by
      supplying an explicit id — tested.
- [ ] Session device-fingerprint mismatch invalidates the session and redirects with the reason.
- [ ] The dashboard renders with real seeded data; map markers reflect active/passive correctly.
- [ ] The sectoral comparison table is visible, populated, ranked, and exports CSV.
- [ ] Consumption table shows every column listed in §7.3.
- [ ] All eight load-profile seasonal labels are correct — asserted in a test, since three were
      wrong in the legacy product.
- [ ] Integration credentials are never present in any API response — asserted by a test that
      greps the serialised response for the secret value.
- [ ] Demo login sees only synthetic data; no real company id appears in any response.
- [ ] A calendar event and a vacation period created here change the weekday/weekend classification
      returned by `/load-profile` — asserted end to end.
- [ ] A mobile bearer token authenticates against a non-auth endpoint, and an expired or tampered
      token is rejected with the documented error code.

### Verification

```bash
go test ./internal/api/... -run TestAuthorizationMatrix -v
go test ./internal/api/... -run TestCredentialsNeverLeak -v
pnpm exec playwright test tests/e2e/auth.spec.ts tests/e2e/dashboard.spec.ts
pnpm exec playwright test tests/e2e/consumption.spec.ts tests/e2e/load-profile.spec.ts
pnpm run check:i18n-parity
```

---

## F7 — Alarms and messages

**Goal:** Monitoring that fires correctly and notifies reliably.

**Prerequisites:** F4, F6.

### Scope

- All four alarm types with their full settings, per `01-project-context.md` §7.12.
- `alarm.evaluate` and `alarm.notify` jobs; notification-frequency suppression; per-invoice
  deduplication for the invoice alarm.
- E-mail delivery through the company's SMTP settings, with templates in both locales.
- SMS: either implemented against a real gateway, or removed from the UI. **Not offered
  unimplemented.** Decide with the product owner and record the decision.
- Alarm rule CRUD, event history, and dry-run evaluation.
- **Messages** screen (§7.13) over `operational_messages`, with search and both filters.
- `job_runs` history surfaced to admins, with manual job triggering.

### Acceptance criteria

- [ ] Each alarm type fires on a fixture that should trigger it and stays silent on one that
      should not — one test per type, per direction.
- [ ] Notification frequency suppresses a second notification inside the window and allows one
      after it.
- [ ] The invoice alarm fires once per invoice and never twice, across repeated evaluations.
- [ ] The data-communication alarm fires when the last reading is older than the threshold.
- [ ] A failed e-mail send is recorded on the alarm event and surfaced in Messages; it does not
      fail the job.
- [ ] Messages filters return the expected sets on a seeded fixture.
- [ ] A manually triggered job appears in `job_runs` with its scope and result.

### Verification

```bash
go test ./internal/domain/alarm/... -v
go test ./internal/job/... -run TestAlarm -tags=integration -v
go test ./internal/mail/... -v
pnpm exec playwright test tests/e2e/alarms.spec.ts tests/e2e/messages.spec.ts
```

---

## F8 — Bills, tariffs and reports UI

**Goal:** The full billing and reporting experience.

**Prerequisites:** F4, F6.

### Scope

- **Bills** (§7.10): the invoice dashboard with the buildings, plants and netting sections and
  every column; ad-hoc generation with the analyzer multi-select; analyzer/building/company PDF
  download; download-all; hourly PTF Excel export; every validation message listed.
- **Tariffs** (§7.11): the full create/edit form including PTF+YEKDEM mode and the KBK fields;
  tariff history; templates with apply; bulk assignment with current-state and history views;
  the icmal import flow with its review-and-confirm step.
- **Reports** (§7.14): monthly tab, yearly tab and archive tab with every table, chart, summary
  card and section described; PDF and Excel download; e-mail sending.
- `report.generate` and `report.deliver` jobs; PDF and Excel rendering; e-mail body composition.

### Acceptance criteria

- [ ] The invoice dashboard totals reconcile with the sum of its rows, verified in a test.
- [ ] Every validation message in §7.10 is reachable and displayed.
- [ ] A tariff switched to PTF+YEKDEM mode reveals the KBK fields and refuses to save without an
      energy KBK.
- [ ] The icmal import never writes a coefficient without explicit user confirmation — tested.
- [ ] Monthly and yearly report figures match hand-computed values on a fixture.
- [ ] Report PDF and Excel contain every section listed in §7.14.
- [ ] Report e-mail sends with attachments through the SMTP settings.
- [ ] A recomputed invoice supersedes rather than deletes its predecessor, and the old PDF stays
      downloadable.

### Verification

```bash
go test ./internal/domain/report/... -v
go test ./internal/service/report/... -tags=integration -v
go test ./internal/pdf/... -v && go test ./internal/xlsx/... -v
pnpm exec playwright test tests/e2e/bills.spec.ts tests/e2e/tariffs.spec.ts tests/e2e/reports.spec.ts
```

---

## F9 — Solar, renewable and financial analysis

**Goal:** The generation side of the product, with **no fabricated data**.

**Prerequisites:** F2 (iSolar adapter), F4, F6.

### Scope

- **Solar Power Plants** (§7.7): all four tabs, plant selector, manual sync, connection status,
  summary and revenue cards, production charts, device table with search, alarm list with Turkish
  translation and forwarding.
- Plant management and the iSolar link modal in Settings.
- **Renewable Energy** (§7.8): the production tab and every panel of the detailed tab.
- **Financial Analysis** (§7.9): headline cards, offset balance, tariff panel, yearly chart,
  monthly detail table.
- Weather adapter and its panel.

### Acceptance criteria

- [ ] Every renewable panel is backed by a real measurement, or returns `available: false` with a
      reason and renders an explicit unavailable state. **A test enumerates every panel's data
      source and fails if any is a constant or a random value.**
- [ ] Efficiency, ROI, payback, grid quality and forecast-accuracy figures each trace to a
      documented formula over real data, or are marked unavailable.
- [ ] iSolar units are normalised — no Wh or W crosses the adapter boundary; asserted in a test.
- [ ] Plant alarms are forwarded once and only once across repeated evaluations.
- [ ] Financial figures reconcile against the underlying invoices and production for a fixture.
- [ ] With no coordinates configured, the weather panel says "location not configured" and does
      not display any city.

### Verification

```bash
go test ./internal/service/renewable/... -run TestNoSyntheticData -v
go test ./internal/integration/isolar/... -v
go test ./internal/service/financial/... -tags=integration -v
pnpm exec playwright test tests/e2e/solar-plants.spec.ts tests/e2e/renewable.spec.ts tests/e2e/financial.spec.ts
```

---

## F10 — Carbon footprint module

**Goal:** Eko-CM as a first-class module of the single application.

**Prerequisites:** F3, F6.

### Scope

- `internal/domain/carbon`: emission calculation, unit conversion, scope and ISO category mapping.
- Emission factor catalogue: master seed, per-company copy, override and reset.
- All screens from §7.16: overview, activity selection, data entry with the cascading detail
  fields, activity status with approval workflow, reporting, database, company details, GHG
  Protocol, ISO 14064, reporting standards, documents.
- `carbon.daily_accrual` job creating automated activities from electricity consumption and solar
  generation, idempotent per building/type/day.
- GHG and ISO 14064 report generation with PDF output.

### Acceptance criteria

- [ ] Emission for each of the eight main categories computes correctly against hand-checked
      fixtures.
- [ ] Scope and ISO category are derived automatically and never entered by the user.
- [ ] Each activity stores the factor value and conversion multiplier used, so a later catalogue
      change does not alter historical figures — asserted by changing a factor and re-reading an
      old record.
- [ ] The daily accrual job run twice for the same day creates exactly one record per
      building/type.
- [ ] The grid emission factor is a dated configuration value, not a constant; reports state the
      factor and its source year.
- [ ] Overview totals reconcile with the sum of the activity records.
- [ ] Both report types generate and render to PDF.
- [ ] The master factor catalogue migrated from the legacy system loads completely, with a row
      count matching the source.

### Verification

```bash
go test ./internal/domain/carbon/... -v
go test ./internal/job/... -run TestCarbonAccrual -tags=integration -v
pnpm exec playwright test tests/e2e/carbon.spec.ts
bcem seed --only emission-factors --verify
```

---

## F11 — ISO 50001 module

**Goal:** The compliance workbench, complete.

**Prerequisites:** F6.

### Scope

- Clause tree for TS EN ISO 50001:2018 clauses 5–9 with every sub-clause, title and description
  from §7.17, seeded in both locales.
- Notes: add, list, edit, update, delete, with titles and dates.
- File upload: 30 MB limit, type allow-list, magic-byte sniffing, stored outside any web root,
  served through an authorising handler, delete with confirmation.
- Clause date setting with validation, and the Gantt chart with per-clause status.
- Progress calculation and the summary page.
- Downloadable clause templates.
- Streamed zip export of all notes and files.

### Acceptance criteria

- [ ] All nineteen sub-clauses render with their full text in both locales.
- [ ] Progress percentage matches a hand-computed value for a fixture.
- [ ] Gantt renders correct statuses (not started / in progress / completed / expired) and shows
      the documented message when dates are insufficient.
- [ ] A file over 30 MB is rejected with a clear message; a file with a disallowed magic-byte
      signature is rejected even with an allowed extension.
- [ ] Uploaded files are not reachable by direct URL without authorisation — tested.
- [ ] The export archive contains every note and every file and streams without buffering the
      whole archive in memory.
- [ ] Date validation rejects a start after an end, and requires both.

### Verification

```bash
go test ./internal/service/iso50001/... -tags=integration -v
go test ./internal/storage/... -run TestPathTraversal -v
go test ./internal/api/... -run TestFileAuthorization -v
pnpm exec playwright test tests/e2e/iso50001.spec.ts
```

---

## F12 — Public marketing site

**Goal:** The public site rebuilt with the new design language.

**Prerequisites:** F5.

### Scope

- All pages from §7.19 with their full section inventory.
- Markdown-based blog with category filters, article pages, GFM and raw HTML support, and the two
  migrated legacy articles with their images.
- Contact and demo-request forms with server-side validation, rate limiting, spam protection and
  e-mail delivery.
- The public bill calculator backed by `national_tariff_schedule`, computing **every** charge line
  including the power charge.
- SEO: metadata, Open Graph, sitemap, robots, structured data.
- Analytics, behind a configuration flag and disabled in on-premise installs.

### Acceptance criteria

- [ ] Every page in §7.19 exists and renders in Turkish and English.
- [ ] Lighthouse: performance ≥ 90, accessibility ≥ 95, SEO ≥ 95 on the homepage.
- [ ] LCP < 2.5 s and CLS < 0.1 on the homepage, measured.
- [ ] The bill calculator's output is verified against a hand-computed example including the power
      charge — which the legacy calculator omitted.
- [ ] Entering a total consumption and T1/T2/T3 values produces a defined, documented result
      rather than silently discarding one of them.
- [ ] Contact and demo forms deliver e-mail and are rate limited.
- [ ] Both legacy blog articles render with their images.
- [ ] Pricing and self-registration remain disabled behind their flags by default.

### Verification

```bash
pnpm build && pnpm exec lighthouse http://localhost:3000 --output=json
pnpm exec playwright test tests/e2e/public/
go test ./internal/domain/tariff/... -run TestPublicCalculator -v
```

---

## F13 — ML service

**Goal:** Forecasting and anomaly detection behind a clean contract.

**Prerequisites:** F3.

### Scope

- FastAPI service implementing `03-target-architecture.md` §6.3 exactly. **No database driver is
  installed in the image** — this is the boundary that must not be crossed.
- Models: statistical baseline, random forest over engineered features, LSTM, and the external
  foundation-model forecaster, all behind one interface with a `model_id` and version in every
  response.
- Feature engineering from the covariates supplied in the request: calendar, weather, day type,
  vacation flags, rolling means.
- Gap detection and reporting in the forecast response.
- Anomaly detection with an explicit method name in the response.
- Go client with timeouts, retries and graceful degradation.
- `forecast.run` job persisting to `forecasts` and `forecast_gaps`.
- **Predict** (§7.5) and **AI Analysis** (§7.6) screens.

### Acceptance criteria

- [ ] The service image contains no database client — asserted by inspecting the built image.
- [ ] Forecast responses always carry `model_id`, `model_version` and an explicit `status`.
- [ ] Gaps in the supplied history are detected and reported with start, end and missing hours.
- [ ] `insufficient_data` is returned rather than a low-quality forecast below the configured
      minimum history.
- [ ] The statistical baseline works with every heavier model disabled.
- [ ] The Go client degrades gracefully when the service is unreachable: the UI shows forecasting
      as unavailable and no other feature breaks.
- [ ] The Predict screen renders the median with the p10–p90 band, drawn neutral and dashed, and
      surfaces reported gaps.

### Verification

```bash
cd ml && pytest -v
docker run --rm --entrypoint pip bcem-ml list | grep -Ei "psycopg|pymongo|sqlalchemy" && exit 1 || echo "no db client"
go test ./internal/ml/... -v
go test ./internal/ml/... -run TestGracefulDegradation -v
pnpm exec playwright test tests/e2e/predict.spec.ts
```

---

## F14 — Migration and cutover tooling

**Goal:** A rehearsed, reversible migration of the production system.

**Prerequisites:** F1–F11.

### Scope

Everything in `08-migration.md` §11:

- `bcem migrate inventory | extract | transform | load | artifacts`
- `bcem recompute consumption | bills | reports | carbon | forecasts`
- `bcem migrate reconcile` producing the HTML and CSV report with attributed difference reasons.
- `legacy_bills` and `legacy_reports` comparison tables.
- `docs/runbook-migration.md`: the operator runbook with expected durations, checkpoints and the
  rollback procedure.
- The full pipeline exercised in CI against an anonymised production dump.

### Acceptance criteria

- [ ] Every command is idempotent: running it twice produces identical results.
- [ ] `inventory` runs read-only against the legacy database — proven by a test asserting no write
      operation is issued.
- [ ] Every rejected record appears in the rejection file with a reason; nothing is dropped
      silently — asserted by comparing input count to loaded + rejected count.
- [ ] Ambiguous dates are rejected, never guessed.
- [ ] Multiplier normalisation is correct for every provider; analyzers that cannot be determined
      are listed for manual confirmation rather than assumed.
- [ ] Legacy encrypted credentials decrypt with the legacy key and re-encrypt with the new key,
      and the resulting credential authenticates against the live provider.
- [ ] Reconciliation runs against the anonymised dump and produces zero `unexplained` differences;
      every other difference carries one of the documented reasons.
- [ ] Artifact copy registers every file and reports both orphan directions.
- [ ] The rehearsal has been run end to end **at least three times** on a copy, with timings
      recorded in the runbook.
- [ ] The rollback procedure has been executed on the rehearsal environment and verified.

### Verification

```bash
go test ./internal/migrate/... -v
go test ./internal/migrate/... -run TestIdempotent -tags=integration -v
bcem migrate inventory --uri "$LEGACY_URI" > /tmp/inventory.txt
make migration-rehearsal            # extract → transform → load → recompute → reconcile
open reports/reconciliation.html
```

---

## F15 — Hardening, packaging and handover

**Goal:** Production-ready, operable, and installable offline.

**Prerequisites:** everything.

### Scope

- Performance: load test the API and job pipeline at 2× expected production volume; profile and
  fix the top bottlenecks; verify hypertable query plans use chunk exclusion.
- Security review: dependency audit, secret handling, authorisation matrix re-verification, upload
  handling, path traversal, rate limiting, audit log completeness.
- Full accessibility audit against `07-design-system.md` §9.
- Observability: dashboards for HTTP latency and errors, job success and duration, queue depth,
  integration health; alerting thresholds.
- Backup and **restore** procedure, tested on a scratch host.
- Offline bundle: `make offline-bundle`, `install.sh`, upgrade path, tested on a host with
  networking disabled.
- Documentation: operator runbook, administrator guide, API reference published from the OpenAPI
  document, architecture decision records.

### Acceptance criteria

- [ ] Load test at 2× production volume: p95 API latency under the agreed budget; no job backlog
      growth; no memory growth over a 24-hour soak.
- [ ] `govulncheck` and `pnpm audit` report no high or critical findings.
- [ ] The authorisation matrix test suite passes in full.
- [ ] Accessibility audit passes with no critical or serious findings.
- [ ] A backup is taken and **restored** onto a clean host, and the restored system serves
      correctly.
- [ ] The offline bundle installs on a host with networking disabled and reaches a healthy stack.
- [ ] Re-running `install.sh` on an existing install upgrades it without data loss.
- [ ] Every document in `docs/` is current with the shipped system.

### Verification

```bash
make load-test && make soak-test
govulncheck ./... && pnpm audit --audit-level=high
go test ./internal/api/... -run TestAuthorizationMatrix -v
pnpm exec playwright test tests/a11y/ --reporter=list
make offline-bundle && ./scripts/test-offline-install.sh
make backup && ./scripts/test-restore.sh
```

---

## Working agreements

**Per phase**

1. Read the relevant specification documents in full before planning.
2. Produce a task-level plan with `superpowers:writing-plans`.
3. Execute it with `superpowers:subagent-driven-development` or
   `superpowers:executing-plans`.
4. Write the test first. Watch it fail. Implement. Watch it pass. Commit.
5. Run the phase's verification commands and **show the output**.
6. Report completion only when every acceptance checkbox is genuinely ticked.

**Per UI phase**

Invoke `ui-ux-pro-max`, read the persisted master design system, then build. Complete the
pre-delivery checklist in `07-design-system.md` §11 before calling the phase done.

**When something is wrong**

If a specification is unclear, contradictory or appears mistaken, stop and say so with the specific
passage and the problem. Do not improvise around it. If a phase turns out larger than described,
say so before starting rather than half-finishing it.

**Definition of done, for the whole project**

Every item in `master_prompt.md` §"What done looks like" is satisfied and demonstrated.
