# F3 — Consumption Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every consumption figure the product shows or bills is derived from meter registers by one auditable set of pure functions, a negative register difference can never become an invoice number, and the analytics path and the billing path are two different functions with two different guarantees.

**Architecture:** `internal/domain/energy` and `internal/domain/loadprofile` are dependency-free pure packages: registers in, derived values out, no I/O, no clock, no timezone lookup beyond `time.Location` values handed to them. Above them, a new `internal/service` layer is introduced by this phase. `internal/service/consumption` owns the two consumption paths that `04-data-model.md` §4.3 demands be explicit in code — `analytics.Consumption` reads F1's continuous aggregates through `store.AnalyticsRepository` and never touches `meter_readings`; `billing.Consumption` reads `meter_readings` at true period boundaries through `store.ReadingRepository.BoundaryReadings` and never touches an aggregate. The same service owns suspect periods: it writes `consumption_anomalies` rows and operator messages when a period is indeterminate, and it exposes listing and operator resolution. `internal/service/loadprofile` resolves a company's weekend and vacation configuration and feeds the pure profile functions. `consumption.refresh` — declared by F2 and left unhandled — gets a handler that calls `refresh_continuous_aggregate` through a new platform-scoped admin repository, and the F2 ingestion pipeline's affected range is finally wired to enqueue it.

**Tech Stack:** Go 1.27.1 · shopspring/decimal v1.4.0 · pgx/v5 v5.11.0 · sqlc v1.30.0 · goose v3.28.0 · asynq v0.26.0 · TimescaleDB continuous aggregates · testify · testcontainers-go v0.44.0 · `time/tzdata`

**Spec:** `docs/rewrite/09-implementation-plan.md` §F3 (binding scope, acceptance and verification). `docs/rewrite/02-domain-rules.md` §1, §2, §3, §10.3 (authoritative for derivation rules). `docs/rewrite/04-data-model.md` §4.1, §4.3, §4.4, §11 (authoritative for every column). `docs/rewrite/03-target-architecture.md` §2.1, §2.2, §7. `docs/rewrite/05-api-contract.md` §5 (the surface the services must be able to serve). `docs/rewrite/10-removed-behaviours.md` item 1 (the legacy defect this phase exists to prevent).

---

## Global Constraints

Every task's requirements implicitly include this section.

**Authority**
- **`02-domain-rules.md` is authoritative for derivation rules; `04-data-model.md` for every column; the migrations already on disk for every built schema fact.** Where this plan's prose disagrees with the spec, the spec wins. **Where the spec disagrees with a phase acceptance criterion in `09-implementation-plan.md` §F3, the acceptance criterion wins** (F0/F1 standing ruling; it decides R54 below). Every place the spec is silent is marked **"spec silent — ruling: X"** in the "Spec gaps and rulings" table. An implementer who finds a new gap stops and reports it; do not improvise.
- **A built schema fact beats a spec sentence.** `materialized_only`, refresh offsets and the `where kind = 'load_profile'` filter on the continuous aggregates are what migration `00005` actually created and what `migrations_timeseries_integration_test.go` already pins. Never "fix" the schema to make a figure appear; compose the answer in the service (R61, R62).

**Purity and layering**
- **`internal/domain/...` imports nothing from the project and does no I/O.** No `context`, no `time.Now()`, no repository, no logger. The current instant, the company's vacation configuration, the analyzer's metadata and the `*time.Location` are parameters. Enforced by the existing `internal/arch` guards.
- **`internal/service/...` is introduced by this phase.** It may import `internal/domain/...`, `internal/store`, `internal/job` and `internal/platform/...`. It must not import `internal/api` or `internal/ingest`. `internal/store/...` and `internal/integration/...` must not import `internal/service/...`. Task 1 extends `internal/arch` to enforce both directions (R76).
- **No HTTP in F3.** `05-api-contract.md` §5 names ten endpoints; F3 builds the service methods that back them and nothing above. Handlers, routing, DTOs, RBAC and the OpenAPI document are F6. A task that adds a file under `internal/api/` is out of scope (R77).

**Money, energy and time**
- **Energy, power, ratios and multipliers are `decimal.Decimal`, never `float64`.** JSON and SQL decode through `pgnum`/`decimal`, never through `float64` or `any`. Task 1 extends `internal/arch` `floatGuardPatterns` and `.golangci.yml` `no-float-money` to `./internal/domain/energy/...`, `./internal/domain/loadprofile/...` and `./internal/service/...` — change both or neither.
- **Intermediate values are never rounded** (02 §1). Rounding happens once, when a value is written to an invoice line or displayed — neither of which happens in F3. No `Round`, `Truncate` or `StringFixed` inside `internal/domain/energy` or `internal/domain/loadprofile`.
- **An unavailable quantity is `nil`, never zero** (removed-behaviour 21, and 02 §6.5 for max demand). A missing register, an indeterminate consumption and an undefined ratio are all `nil`. Zero means "measured zero".
- **Timestamps are stored UTC and evaluated in `Europe/Istanbul`.** Bucket boundaries are computed through the tz database, never a fixed `+03:00` offset (02 §1: pre-2016 data crosses real DST transitions). Packages that need the zone import `time/tzdata`.
- **The meter multiplier was applied once, at ingestion** (02 §2.2). Nothing in F3 multiplies again. A test asserts that `meter_readings.multiplier_applied` is read for audit and never re-applied.

**Correctness guards**
- **Every guard is proven to fail.** A guard seen only to pass is not evidence (F1/F2 standing ruling). Each guard step names the mutation that must turn it red, and the task report records the observed failure output.
- **The negative-difference rule is the reason this phase exists** (`10-removed-behaviours.md` item 1: the legacy system returned the end register value itself as the consumption). Every task that touches derivation carries a test proving a negative delta without a reset produces `nil` consumption, a suspect period and an operator message — never a number.
- **The two consumption paths are proven to be different functions.** A guard test asserts `analytics.Consumption` never reaches `meter_readings` and `billing.Consumption` never reaches a `consumption_*` view, and an integration test shows the two differ at a bucket boundary by exactly the missing step (09 §F3 acceptance).
- **Every aggregation level derives from its own boundaries.** A test asserts that the hourly rows of a day do not sum to the daily row when a reading is missing mid-day, and that the daily figure is still correct — this is the assertion that "summing the level below is not how they are produced" (09 §F3 acceptance).

**Tenancy**
- **`ctx` first, `store.Scope` second** on every service method that touches tenant data. Validate with `Scope.Valid()` before any I/O.
- **`meter_readings`, `consumption_anomalies` and the `consumption_*` views carry no `company_id`.** Their tenant predicate is a join through `analyzers` and lives in F1's SQL. F3 never writes a tenant-blind query and never re-implements the predicate in Go.
- **Cross-tenant proofs use the other tenant's `AdminScope`** with a positive control, never a narrow `Scope` (F1 ruling 4 — a narrow scope hides a broken `company_id` predicate).
- **Background work acts through `store.SystemScope(companyID)`**; platform-wide work — the continuous-aggregate refresh — goes through a `store.Admin*` interface implemented in `internal/store/postgres/admin` (R72).

**Store and migrations**
- **F3 adds no migration.** Every table, view and policy it needs was built by F1 (`00004`, `00005`) and F2 (`00012`, `00013`). A task that believes it needs a migration stops and reports.
- **Run `make generate` after any query change and commit the output.** `make check-generate` is part of the gate.
- **Integration tests use `testfixtures.NewIsolatedDB(t)` / `NewEmptyDB`, `NewTenant`, `StartRedis` and `DiscardLogger`.** Never hand-roll a container helper. A failure containing `wait until ready` or SQLSTATE `55006` is Docker load: retry that package once before believing it.
- **`refresh_continuous_aggregate` cannot run inside a transaction** (SQLSTATE 25001). The refresh path issues it as a top-level statement on a pool connection, never inside a `pgx.Tx`.

**Process**
- **Branch `phase/f3-consumption-engine`**, created from `phase/f2-integration-layer` after F2 is complete (see Pre-flight). Parallel tasks use branches `f3/task-N` in worktrees on the native filesystem:
  ```bash
  git worktree add -b f3/task-N /home/personal/ekokod-f3-tN phase/f3-consumption-engine
  git config --global --add safe.directory /home/personal/ekokod-f3-tN
  ```
  Merge with `git merge --no-ff`. The Agent tool's `isolation: "worktree"` does not work on this mount.
- **Toolchain:** prefix every command with
  `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=4 GOMEMLIMIT=2GiB`.
  **Docker must be up:** verify with `docker ps` before any integration test. At most 3 agents run Docker-backed suites at once.
- **Every task ends green:**
  ```bash
  make lint && make test && make build && make check-generate
  go test ./... -tags=integration -race -count=1 -parallel 8
  ```
- **Commit after every task**, conventional commits, with the trailers the controller supplies at dispatch time. Below, `<TRAILERS>` in a commit step means exactly those trailer lines.

---

## Pre-flight (controller, before Task 1; no agent)

- [ ] **P1.** Verify F2 is complete and green at `phase/f2-integration-layer`:
      `git log --oneline -3` shows the F2 handoff commit, `go build ./...` passes, `docker ps` shows a running daemon.
- [ ] **P2.** Verify every signature this plan's Consumes blocks depend on still exists, by name:
      ```bash
      git grep -n "BoundaryReadings\|ConsumptionHourly\|type AnomalyRepository\|type CalendarRepository" -- internal/store/repository.go
      git grep -n "TypeConsumptionRefresh\|ConsumptionRefreshPayload\|ConsumptionRefreshEnqueuer" -- internal/job internal/ingest
      git grep -n "ConsumptionRefresh: nil" -- internal/worker
      git grep -n "floatGuardPatterns\|storeScopeAllowlist\|floatGuardMinFields" -- internal/arch/arch_test.go
      ```
      A missing name means the plan is stale: stop and re-plan that task, do not improvise.
- [ ] **P3.** Verify F3 needs no migration: the last migration on disk is `00013`, and `consumption_hourly|daily|monthly|yearly` plus `consumption_anomalies`, `company_weekend_days`, `company_vacations` all exist:
      `ls internal/store/postgres/migrations | tail -3` and
      `git grep -n "create materialized view consumption_" -- internal/store/postgres/migrations`.
- [ ] **P4.** Create the phase branch and the SDD workspace:
      ```bash
      git checkout -b phase/f3-consumption-engine phase/f2-integration-layer
      ```
      Workspace (git-ignored, primary checkout): `.superpowers/sdd/2026-09-16-f3-consumption-engine/` with `progress.md` (the ledger) and `global-constraints.md` (a verbatim copy of this plan's Global Constraints + waves + ownership + prefixes, handed to every implementer and reviewer).
- [ ] **P5.** Record the open product-owner questions (below) in the ledger. **None of them block F3.**

---

## Open questions (non-blocking; F3 proceeds on the ruling)

- **Q1 — the §3.3 ratio zero-case.** `02-domain-rules.md` §3.3 says the ratio is `0` when `active_import = 0`; `09-implementation-plan.md` §F3's acceptance criterion says a zero-consumption period "produces a null ratio, not a ratio computed against a substituted 1". The acceptance criterion wins (R54). The spec sentence should be corrected; raise with the product owner.
- **Q2 — the `missing_readings` anomaly reason.** `consumption_anomalies.reason` allows it, but §3.1 says a period with missing boundary readings yields no row at all. R59 rules where it is written.
- **Q3 — company default weekend days.** `01-project-context.md` says Saturday and Sunday are the default; no migration seeds them. R69 rules the fallback.
- **Q4 — calendar events vs vacations.** F2's acceptance wording implies a calendar *event* changes weekday/weekend classification; `02-domain-rules.md` §10.3 names only weekend days and vacation periods. R70 rules it; the F2 wording should be corrected.
- **Q5 — `contracted_power_kw`.** Still unresolved from F2 (it lives on `tariffs`, not `analyzers`). F3 does not need it: max demand is reported, the overrun charge is F4's.

---

## Spec gaps and rulings

Ruling ids continue F2's sequence (F2 ended at R53) so a code comment citing an R-id is unambiguous across phases.

| Id | Spec passage | Ruling | Cost if wrong |
|---|---|---|---|
| R54 | 02 §3.3 "inductive_ratio = … (0 when active_import = 0)" vs 09 §F3 acceptance "A zero-consumption period produces a **null** ratio, not a ratio computed against a substituted 1" | Direct conflict; the acceptance criterion wins (F0/F1 standing ruling). **A ratio is `*decimal.Decimal` and is `nil` when active-import consumption is nil, zero or negative.** Never `0`, never computed against a substituted `1`. Raise Q1 with the product owner. | One field type + the ratio fixtures |
| R55 | 02 §3.2 "`(register_value_before_reset − reading_start) + (reading_end − register_value_after_reset)`"; 04 §4.1 gives a `reset`-kind row the same columns as any other reading, with no before/after pair | Spec silent on which rows supply the two values. **Ruling: `register_value_after_reset` is the `reset` row's own register value. `register_value_before_reset` is the register value of the last non-`reset` reading with `ts < reset.ts` and `ts >= reading_start.ts`; when no such reading exists, it is `reading_start`'s value, so the pre-reset segment contributes zero rather than a guess.** A `reset` row whose own register value is `nil` for a register makes that register suspect (`meter_reset`), never zero. | The derivation formula + three golden fixtures |
| R56 | 02 §3.2 describes exactly one reset in a period | Spec silent on several. **Ruling: the period is split at every `reset` row inside `[start, end)` and consumption is the sum of the per-segment differences, computed with the same before/after rule (R55). Resets are processed in `ts` order; a segment with a negative difference of its own makes the register suspect.** | Loop shape + one fixture |
| R57 | 09 §F3 acceptance names "a rollover" as a case distinct from the two reset cases; no spec text defines a rollover | Spec silent. **Ruling: F3 never infers a register modulus and never reconstructs a wrap arithmetically.** A rollover is a negative difference like any other: with a covering `reset` row it takes R55's path (the row's before-value is near the modulus and its after-value is small), without one it is `negative_delta`-suspect. The "rollover" golden fixture is therefore a wrap **with** a reset row, and a second fixture proves an uncovered wrap yields `nil` + suspect, not a `10^n`-corrected number. | One fixture's framing |
| R58 | 02 §3.2 "consumption for the affected registers is recorded as `NULL`" | **Ruling: suspicion is per register.** A period whose `active_import` is suspect still reports a derived `t1_import` if that register is sound. The `Derivation` carries a per-register `Suspicion`, and the anomaly row's `detail` lists the affected register names. Any register being suspect makes the whole period unbillable — that check is F4's, keyed off the unresolved anomaly row, not off a period-level flag F3 invents. | `detail` shape + F4 reads one field |
| R59 | 04 §4.4 `reason ∈ 'meter_reset','negative_delta','missing_readings'` vs 02 §3.1 "the period yields no row — it is not emitted as zero" | **Ruling:** `negative_delta` = a negative difference with no covering reset. `meter_reset` = a reset row exists but cannot be applied (a nil before/after value for that register). `missing_readings` = written **only by the billing path**, when a caller asks for a specific billing period and a boundary reading is absent; the analytics path and range queries simply omit the bucket, exactly as §3.1 says. Ingestion's own reading-to-reading `negative_delta` anomalies (F2, `internal/ingest/validate.go`) are a different check on the same table and are never deduplicated against F3's period-level rows except by the `(analyzer, period_start, reason)` key. | One branch + Q2 |
| R60 | 02 §3.2 "an operator message is written"; no format given | **Ruling: the operator message is an `OpsRepository.AppendMessage` entry at level `error`, with a stable code `consumption.suspect_period`, plus the same fields in `consumption_anomalies.detail`.** `detail` = `{"registers":["active_import"],"period_start":…,"period_end":…,"delta":"-1234.5000","reset_rows":0,"code":"consumption.suspect_period"}`. Decimals are strings. No secret, no raw payload, no SQL. | `detail` shape |
| R61 | 04 §4.3 "They are different functions with different guarantees" | **Ruling: the separation is enforced, not documented.** `internal/service/consumption` splits into `analytics.go` and `billing.go`; the analytics path receives only `store.AnalyticsRepository`, the billing path only `store.ReadingRepository` (+ anomalies/ops), through two distinct dependency structs. A guard test constructs each path with the other's repository set to a fake that fails the test if called. No flag, no shared "mode" parameter. | Two constructors |
| R62 | 02 §3.4 "the monthly row is derived from `billing`-kind readings when present"; migration `00005` filters every aggregate to `kind = 'load_profile'` | **Ruling: the preference is a billing-path rule only.** `analytics.Consumption` at monthly granularity reports what `consumption_monthly` holds and records `Source: "load_profile"`. `billing.Consumption` applies the preference and records `Source: "billing"` or `"load_profile"` per period. No migration, no view change. | A field + the monthly test's framing |
| R63 | 02 §3.4 "when a metering point has `billing`-kind readings covering a month" | Spec silent on "covering". **Ruling: covered means §3.1 boundary selection restricted to `kind = 'billing'` returns two distinct rows for `[month_start, month_end)` — i.e. `BoundaryReadings(analyzer, 'billing', start, end)` yields non-nil start and end readings with different `ts`.** Otherwise the month falls back to `load_profile`. A partially covered month never mixes kinds within one period. | One predicate + one fixture |
| R64 | 04 §4.1 `reading_kind` has five values; 02 §2.3 documents four | **Ruling: `current_index` never participates in §3.1 differencing.** It is a max-demand and diagnostics source only. The billing path's kind parameter is `load_profile`, `daily` or `billing`; `reset` rows are read separately as reset evidence; `current_index` is read only by the max-demand query. | A `WHERE kind IN` list |
| R65 | 02 §3.5 "the maximum of the interval `max_demand` values within the period"; "for ARIL … taken from that endpoint's `MaxDemand` field, taking the maximum per month" | **Ruling: one provider-agnostic rule.** Max demand is `MAX(max_demand_kw)` over readings in `[start, end)` of kinds `load_profile` and `current_index` with a non-nil value; `nil` when none. F2's ARIL adapter already writes its `MaxDemand`/`MaxDemandDate` as `current_index` rows (R49), so the monthly maximum falls out of the same rule and F3 adds no provider branch. The implementer verifies this by reading `internal/integration/aril` before implementing; if ARIL does not land in `current_index`, stop and report. | A provider branch, if the premise is wrong |
| R66 | 02 §3.6 "active if its most recent reading is within the last 7 days"; `analyzers.is_active` exists and means something else | **Ruling: `energy.ActivityStatus(lastReadingAt *time.Time, now time.Time) Status` with a fixed 7×24 h constant, exported as `energy.ActivityWindow`.** It is independent of `analyzers.is_active` (operator enablement) and of the configurable data-communication alarm threshold (F7). A code comment states all three are different. `nil` last reading is `passive`. | A constant |
| R67 | 02 §10.3 "stddev" with no divisor | Spec silent. **Ruling: population standard deviation (divisor `n`, the 24 hourly means), because the 24 points are the whole curve, not a sample of it.** Stated in the doc comment and asserted by a hand-computed fixture. | One divisor + the profile fixtures |
| R68 | 02 §10.3 "the mean consumption across all matching days" — which register is unstated | Spec silent. **Ruling: the load profile averages active-import consumption only.** Generation profiles are not F3 (§10.1's rooftop metrics are F9). The domain function takes `[]HourValue` and never names a register, so a later phase can feed it export values without a change. | Nothing in the domain layer; one service call site |
| R69 | 02 §10.3 "the configured non-working weekdays"; no migration seeds any; 01 §context says the default is Saturday and Sunday | **Ruling: the pure classifier takes the weekend-day set as a parameter and applies exactly what it is given. The service resolves it: `CalendarRepository.WeekendDays` when the company has rows, otherwise the documented default `{Saturday, Sunday}`.** The default lives in the service, is named `loadprofile.DefaultWeekendDays`, and is asserted by a test that a company with no rows still splits Sat/Sun into `weekend`. | One default + one test |
| R70 | 02 §10.3 names weekend days and vacation periods; 09 §F2's acceptance wording also implies a *calendar event* changes the split; `calendar_events` has no non-working flag | **Ruling: `calendar_events` never affects classification.** Only `company_weekend_days` and `company_vacations` do. A guard test creates a calendar event over a working day and asserts the day stays a weekday. Raise Q4 to correct the F2 wording. | One test |
| R71 | 04 §4.3 refresh policies; `refresh_continuous_aggregate` takes a time range only and cannot be filtered by analyzer | **Ruling: the refresh job is per company+window, not per analyzer.** The handler expands `[From, To)` to whole buckets per level, refreshes finest-first (`hourly → daily → monthly → yearly`), and serialises refreshes **per view** with `platform/lock` so two concurrent jobs never refresh the same view. The task id is deterministic in `(level-set, expandedFrom, expandedTo)` so a burst of analyzers ingesting the same window collapses to one task; `AnalyzerID` stays in the payload for the journal only. | A dedup key + one lock |
| R72 | 03 §2.2 layering; `TestEveryStoreMethodIsScoped` | **Ruling: the refresh is platform work and goes through `store.AdminAggregateRepository` implemented in `internal/store/postgres/admin`,** which the scope guard already exempts. It is never exposed on a tenant-scoped repository, because a refresh cannot be restricted to one tenant's rows and a scoped signature would be a lie. | One interface move |
| R73 | F2 wrote an `info` message promising that a pre-30-day backfill enters the aggregates "once `consumption.refresh` runs for it (F3)" | **Ruling: F3 makes that promise true.** `ingest.Service.FetchReadings` enqueues `consumption.refresh` whenever the run persisted rows and the accumulator's affected range is non-nil; an empty range never enqueues. The window is the affected range, not the requested window, so a backfill of old data refreshes exactly the buckets it touched, below every policy watermark. An integration test backfills a 200-day-old window, asserts the aggregate is **absent**, runs the job, and asserts it is **present**. | The enqueue call site |
| R74 | F2 parked the PM5340 post-persist recompute: one hook call can rewrite ~35 k rows per analyzer-year | **Ruling: the hook stops recomputing inline past a bounded span.** When the rows after the anchor exceed `EKOKOD_GENERATION_RECOMPUTE_INLINE_MAX` (default 5 000), the hook enqueues `generation.recompute` with the analyzer and the start instant, and the handler walks forward in bounded batches of the same size, committing per batch and re-enqueueing a continuation until it reaches the tip. Inline behaviour below the threshold is unchanged. | One knob + one task type |
| R75 | 03 §2.1's package tree has no `internal/service`; 09's §F3 verification runs `go test ./internal/service/consumption/...` | Spec inconsistency. **Ruling: the verification block wins — `internal/service/<area>` is the layer's home** (`consumption`, `loadprofile` in F3). The architecture doc's tree should gain the entry; raise it with the spec owner. | A directory rename while the tree is two packages |
| R76 | 03 §2.2 "api ──▶ service ──▶ domain"; `internal/arch` has no service guard because no service existed | **Ruling: Task 1 adds `TestServiceLayerImportBoundaries`** — `internal/service/...` may not import `internal/api/...` or `internal/ingest/...`; `internal/store/...` and `internal/integration/...` may not import `internal/service/...` — and extends `floatGuardPatterns` and `.golangci.yml`'s matching lists with `./internal/service/...`. Change both or neither. | Guard edits |
| R77 | 05 §5 lists `/consumption/export` and `/load-profile/export` (CSV/XLSX) | **Ruling: F3 produces the rows and the stable column order; it renders no file.** `ExportRows`/`ExportMatrix` return ordered columns and values as strings; the CSV/XLSX encoder and the HTTP surface are F6/F8. No `excelize` import in F3. | One adapter in F8 |
| R78 | 05 §5 `/generation` "same shape as `/consumption`" and `/energy-balance` "generation, consumption, grid import and grid export for the range" | **Ruling:** generation series = the export registers through the same two paths and the same derivation; energy balance = `{consumption: active_import, generation: active_export, grid_import: active_import, grid_export: active_export}` derived from registers only. Plant-production-based balance (inverter data, self-consumption ratios) is F9 and is not approximated here. | One struct |
| R79 | 02 §1 "intermediate values are never rounded"; `pgnum` passes excess scale through unrounded | **Ruling: F3 rounds nowhere.** Derived values keep the scale arithmetic gives them; storage rounding is Postgres's when a value is written to a `numeric(18,4)` column, and presentation rounding is F6's. A `Round`/`StringFixed` call inside `internal/domain/...` fails review. | Nothing |
| R80 | 09 §F3 verification runs `go test ./internal/domain/... -run TestGolden`; the codebase has no golden-file pattern | Spec silent on the mechanism. **Ruling: golden files are JSON under `internal/domain/energy/testdata/golden/<case>.json`, each holding `input` and `want` together, regenerated with `go test ./internal/domain/energy -run TestGolden -update`.** `-update` rewrites only `want`; a golden file is never written by the code under test at assert time. A test asserting a golden file must fail when a single expected value is edited — proven by mutation. | Fixture layout |

### Legacy behaviours, confirmed by reading the running system

The legacy TypeScript was read for this plan (`src/utils/consumption.ts`, `src/workers/consumption-worker.ts`,
`src/app/(DashboardLayout)/load-profile/page.tsx`, `src/utils/reactivePenalty.ts`, `src/utils/arilService.ts`).
It confirms four things and forces six more rulings.

Confirmed:
- **The defect is exactly as `10-removed-behaviours.md` item 1 describes.** `calculateConsumptionWithReset` (`consumption.ts:428-445`) returns `endValue` whenever `end - start < 0`, with no threshold — any negative delta, however small, becomes the full cumulative end register value.
- **There is no rollover handling in the legacy system at all** — no modulus, no wraparound. R57 is therefore not a simplification of legacy behaviour; it is the first time the case is handled deliberately.
- **The "substituted 1" the acceptance criterion warns about is real code**: `reactivePenalty.ts:47` does `const safeActive = totalActive === 0 ? 1 : totalActive`, so a zero-consumption period reports the raw reactive quantity as if it were a ratio. R54 exists to make that impossible.
- **Legacy already derives every level independently** from the raw readings rather than summing the level below (`consumption.ts:519-788`), and drops empty buckets instead of zero-filling them — matching 02 §3.4 and §3.1.

| Id | Legacy behaviour | Ruling | Cost if wrong |
|---|---|---|---|
| R81 | `loadFactor = average / max` with no zero guard, rendering literal `NaN` (`load-profile/page.tsx:333`); 02 §10.3 says "0 when max = 0" | **Ruling: load factor follows §10.3 literally and is `decimal.Zero` when max is zero** — deliberately *unlike* R54's ratios, because no acceptance criterion overrides §10.3 here. Both rulings are cited in the code comment so the asymmetry is visibly intentional. | One branch |
| R82 | Seasons are fixed calendar dates (21 Mar / 21 Jun / 23 Sep / 21 Dec, `load-profile/page.tsx:452-485`); 02 §10.3 says meteorological (Dec-Feb, Mar-May, Jun-Aug, Sep-Nov) | **Ruling: the spec wins — meteorological month boundaries.** Recorded as a deliberate divergence: a legacy report regenerated under F3 moves days between seasonal profiles near the equinoxes. F14's comparison tooling must expect it. | Profile bucketing + an F14 expectation |
| R83 | The legacy hour axis is 1-24 (`hour 00` remapped to `24`, `page.tsx:406-409`) | **Ruling: hours are 0-23 everywhere in F3**, matching `time.Time.Hour()` and the aggregates. F6 may render a 1-24 axis; the data never carries hour 24. | One off-by-one in F6 |
| R84 | Legacy statistics are computed per hour over that hour's population across days; the spec's `hour_of_max` only makes sense over the 24-point curve | **Ruling: statistics are per profile over the 24 hourly means** (max, min, hour_of_max, mean, stddev, range, load factor), as 02 §10.3 and `01-project-context.md` describe. The legacy per-hour spread is not reproduced. | The statistics shape |
| R85 | `maxDemand` has no producer in legacy (`Consumption.ts:85` defaults 0; `arilService.processMaxDemand` is dead code), so `powerOveruseCost` is effectively always 0 (`bill/route.ts:2174-2181`) | **Ruling: F3 produces max demand for real (R65).** Consequence, recorded now rather than discovered in F14: once F4 uses it, invoices carry a demand-overrun charge where legacy billed zero. This is a correction, not a regression — raise it with the product owner before F4 bills anything. | A product-owner conversation, not code |
| R86 | Legacy classifies weekend as JS Saturday/Sunday and never reads `Company.vacations` (`page.tsx:414-416`; confirmed absent from every computation path) | **Ruling: F3 implements the spec's vacation-aware classification (R69, R70).** Load profiles will differ from legacy for any company that configured vacations. Deliberate. | An F14 expectation |

---

## Execution waves

At most 5 concurrent agents, at most 3 running Docker-backed suites. A wave starts only after every task it depends on is **merged** into `phase/f3-consumption-engine` and the merged tip passes the gate. Two placement decisions are deliberate: Task 6 (the store seam) runs in Wave A although nothing else needs it until Wave C, because its integration test — a backfilled bucket that is absent until refreshed — is the longest-running piece of evidence in the phase and must not sit on the critical path; and Task 1 pre-adds the arch allowlist entry Task 6 needs, so the two never edit `internal/arch/arch_test.go` in the same wave.

| Wave | Tasks (parallel) | Depends on |
|---|---|---|
| A | 1, 5, 6 | Pre-flight |
| B | 2, 3 | A (Task 1 for the energy types and the guards) |
| C | 4, 7, 10, 11 | B (2, 3 for 4 and 7); A (5 for 10; 6 for 7 and 11) |
| D | 8, 9, 12 | C (7 for 8 and 9; 11 for 12) |
| E | 13 | A–D fully merged |

## File ownership (parallel tasks never edit the same file)

| Task | Files it alone may touch |
|---|---|
| 1 | `internal/domain/energy/{doc,types,period,difference}.go` + their tests, `internal/arch/arch_test.go`, `.golangci.yml` |
| 2 | `internal/domain/energy/{reset,suspect}.go` + their tests |
| 3 | `internal/domain/energy/{ratio,demand,activity}.go` + their tests |
| 4 | `internal/domain/energy/golden_test.go`, `internal/domain/energy/testdata/**` |
| 5 | `internal/domain/loadprofile/**` |
| 6 | `internal/store/repository.go` (the `AdminAggregateRepository` block only), `internal/store/postgres/admin/aggregates.go` + its test |
| 7 | `internal/service/consumption/{service,analytics,billing,doc}.go` + their tests |
| 8 | `internal/service/consumption/{anomalies,resolve}.go` + their tests |
| 9 | `internal/service/consumption/{summary,generation,balance,export}.go` + their tests |
| 10 | `internal/service/loadprofile/**` |
| 11 | `internal/job/{consumption.go,task.go}`, `internal/ingest/{fetch.go,deps.go}`, `internal/worker/wiring.go`, `internal/platform/config` (the refresh knobs) |
| 12 | `internal/ingest/generation/**`, `internal/job/generation.go` |
| 13 | `internal/acceptance/f3_*_test.go`, `docs/`, `HANDOFF_NEXT_SESSION.md` |

## Naming prefixes (F1's collision lesson)

| Shared package | Owning task | Identifier prefix | Notes |
|---|---|---|---|
| `internal/domain/energy` | 1 | types unprefixed (`Registers`, `Reading`, `Window`, `Level`, `Derivation`) | Tasks 2 and 3 add functions only, never a second core type |
| `internal/service/consumption` | 7 | `Service`, `Deps`, `AnalyticsDeps`, `BillingDeps` | Task 8 prefixes `Anomaly*`, Task 9 prefixes `Summary*`, `Generation*`, `Balance*`, `Export*` |
| `internal/job` | 11 | `ConsumptionRefresh*` | Task 12 uses `GenerationRecompute*` |
| `internal/arch/arch_test.go` | 1 | — | No other task edits this file in F3; Task 1 pre-adds Task 6's allowlist entry |

## File Structure

```
internal/domain/energy/                 PURE (Tasks 1-4)
  doc.go                                package doc: §3 rule map, purity contract
  types.go            (T1)              Registers, Reading, register ids, nil-is-not-zero helpers
  period.go           (T1)              Level (hourly|daily|monthly|yearly), Window, Istanbul bucket boundaries
  difference.go       (T1)              §3.1 boundary differencing, missing/identical rules
  reset.go            (T2)              §3.2 reset segmentation (R55, R56, R57)
  suspect.go          (T2)              Suspicion, Reason, per-register suspicion (R58, R59, R60)
  ratio.go            (T3)              §3.3 ratios (R54)
  demand.go           (T3)              §3.5 max demand (R65)
  activity.go         (T3)              §3.6 activity status (R66)
  golden_test.go      (T4)              TestGolden + -update (R80)
  testdata/golden/    (T4)              normal, missing, identical, reset_with_event, reset_without_event, rollover
internal/domain/loadprofile/            PURE (Task 5)
  classify.go                           weekday/weekend (R69, R70), season
  profile.go                            per-hour means over matching days (R68)
  statistics.go                         max/min/hour_of_max/mean/stddev/range/load_factor (R67)
internal/service/consumption/           (Tasks 7-9)
  service.go, doc.go                    Service, Deps, the two dependency sets (R61)
  analytics.go                          aggregates only
  billing.go                            meter_readings only, monthly billing-kind preference (R62, R63)
  anomalies.go, resolve.go    (T8)      suspect period creation, listing, operator resolution
  summary.go, generation.go, balance.go, export.go  (T9)   §5's remaining shapes (R77, R78)
internal/service/loadprofile/           (Task 10)
  service.go                            resolves weekend days + vacations, calls the pure package
internal/store/                         (Task 6)
  repository.go                         + AdminAggregateRepository
  postgres/admin/aggregates.go          CALL refresh_continuous_aggregate, no transaction (R72)
internal/job/consumption.go             (T11) task constructor, decoder, handler interface
internal/job/generation.go              (T12) generation.recompute (R74)
internal/ingest/fetch.go                (T11) enqueue on a non-empty affected range (R73)
internal/worker/wiring.go               (T11) ConsumptionRefresh: nil -> real; new handler
internal/acceptance/                    (T13) the F3 end-to-end suite
```

---

## Task 1: The energy contract, the Istanbul buckets and the extended guards

**Files:**
- Create: `internal/domain/energy/doc.go`, `internal/domain/energy/types.go`, `internal/domain/energy/period.go`, `internal/domain/energy/difference.go`
- Test: `internal/domain/energy/period_test.go`, `internal/domain/energy/difference_test.go`, `internal/arch/arch_test.go` (extend)
- Modify: `internal/arch/arch_test.go`, `.golangci.yml`

**Interfaces:**
- Consumes: nothing from the project. `github.com/shopspring/decimal`, `github.com/google/uuid`, stdlib and `time/tzdata` only — all already in `domainAllowedImports` (`internal/arch/arch_test.go:61-87`).
- Produces:

```go
// Package energy implements docs/rewrite/02-domain-rules.md §2 and §3.
// It is pure: no I/O, no clock, no package-level state. The current instant,
// the location and every reading are parameters.
package energy

// Register names one cumulative register on a meter reading (02 §2.1).
type Register string

const (
	ActiveImport             Register = "active_import"
	ReactiveInductiveImport  Register = "reactive_inductive_import"
	ReactiveCapacitiveImport Register = "reactive_capacitive_import"
	T1Import                 Register = "t1_import"
	T2Import                 Register = "t2_import"
	T3Import                 Register = "t3_import"
	ActiveExport             Register = "active_export"
	ReactiveInductiveExport  Register = "reactive_inductive_export"
	ReactiveCapacitiveExport Register = "reactive_capacitive_export"
	T1Export                 Register = "t1_export"
	T2Export                 Register = "t2_export"
	T3Export                 Register = "t3_export"
)

// AllRegisters is every register in a stable order. Iteration order of a map is
// never relied on: every exported result that ranges over registers uses this.
func AllRegisters() []Register

// Kind mirrors model.ReadingKind (04 §4.1). current_index never takes part in
// §3.1 differencing (R64); reset rows are evidence, not boundaries.
type Kind string

const (
	KindLoadProfile  Kind = "load_profile"
	KindDaily        Kind = "daily"
	KindBilling      Kind = "billing"
	KindReset        Kind = "reset"
	KindCurrentIndex Kind = "current_index"
)

// Reading is one timestamped snapshot. A nil register value means the meter did
// not report it; it is never treated as zero (removed-behaviour 21).
type Reading struct {
	TS          time.Time
	Kind        Kind
	Values      map[Register]*decimal.Decimal
	MaxDemandKw *decimal.Decimal
}

// Value returns the register's value, or nil when absent.
func (r *Reading) Value(reg Register) *decimal.Decimal

// Level is an aggregation level (02 §3.4).
type Level string

const (
	Hourly  Level = "hourly"
	Daily   Level = "daily"
	Monthly Level = "monthly"
	Yearly  Level = "yearly"
)

// Window is half-open: [From, To).
type Window struct{ From, To time.Time }

func (w Window) Valid() bool
func (w Window) Contains(ts time.Time) bool // From <= ts < To

// Bucket returns the level-sized bucket containing ts, evaluated in loc
// (02 §3.4). Boundaries come from the tz database, never a fixed offset.
func Bucket(level Level, ts time.Time, loc *time.Location) Window

// Buckets returns every level-sized bucket intersecting w, ascending.
func Buckets(level Level, w Window, loc *time.Location) []Window

// Suspicion records why a register could not be derived (02 §3.2, R58-R60).
type Suspicion struct {
	Reason    Reason
	Delta     *decimal.Decimal // the negative difference that triggered it, when known
	ResetRows int              // reset rows found inside the window
}

type Reason string

const (
	ReasonNegativeDelta   Reason = "negative_delta"
	ReasonMeterReset      Reason = "meter_reset"
	ReasonMissingReadings Reason = "missing_readings"
)

// Derivation is the result for one window.
//
// Emitted is false for §3.1's "no row" cases: a missing boundary reading, or a
// start and end that are the same reading. A caller must not turn !Emitted into
// a zero row.
type Derivation struct {
	Window  Window
	Emitted bool
	Source  Kind
	Values  map[Register]*decimal.Decimal // nil entry = suspect or unreported
	Suspect map[Register]Suspicion
}

// SuspectRegisters returns the suspect registers in AllRegisters order.
func (d Derivation) SuspectRegisters() []Register

// Difference implements 02 §3.1 for one window with no reset evidence.
// start and end are the readings selected by "the last reading with ts <= bound".
// Either may be nil. A negative difference is left suspect with
// ReasonNegativeDelta; applying reset evidence is Derive's job (Task 2).
func Difference(w Window, start, end *Reading) Derivation
```

**Rules the implementation must follow:**
- `Bucket` for `Monthly` and `Yearly` must land on the Istanbul-local 1st/1 Jan at 00:00 and convert back to UTC; adding `time.Hour*24*30` anywhere is wrong. Use `time.Date(y, m, 1, 0, 0, 0, 0, loc)`.
- `Difference` returns `Emitted: false` when `start == nil`, when `end == nil`, or when `start.TS.Equal(end.TS) && start.Kind == end.Kind` (the same reading was selected for both bounds, 02 §3.1).
- A register whose value is nil on either boundary yields a nil result value with **no** suspicion — the meter simply did not report it (removed-behaviour 21). Suspicion is only for a *derivable but wrong* difference.
- `Difference` never rounds (R79) and never mutates its inputs.

- [ ] **Step 1: Write the failing tests**

```go
func TestBucketMonthlyUsesIstanbulCalendar(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	// 2025-03-01T00:00+03:00 is 2025-02-28T21:00Z: a UTC-based implementation
	// would put this instant in February.
	ts := time.Date(2025, 2, 28, 22, 30, 0, 0, time.UTC)
	got := energy.Bucket(energy.Monthly, ts, loc)
	require.Equal(t, time.Date(2025, 3, 1, 0, 0, 0, 0, loc), got.From.In(loc))
	require.Equal(t, time.Date(2025, 4, 1, 0, 0, 0, 0, loc), got.To.In(loc))
}

func TestBucketDailyCrossesAHistoricalDSTTransition(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Istanbul")
	// 2015-03-29: Türkiye still observed DST; the local day is 23 hours long.
	w := energy.Bucket(energy.Daily, time.Date(2015, 3, 29, 12, 0, 0, 0, loc), loc)
	require.Equal(t, 23*time.Hour, w.To.Sub(w.From))
}

func TestDifferenceEmitsNothingWhenABoundaryIsMissing(t *testing.T) {
	w := energy.Window{From: t0, To: t0.Add(time.Hour)}
	require.False(t, energy.Difference(w, nil, readingAt(t0.Add(time.Hour), "100")).Emitted)
	require.False(t, energy.Difference(w, readingAt(t0, "90"), nil).Emitted)
}

func TestDifferenceEmitsNothingWhenBothBoundariesAreTheSameReading(t *testing.T) {
	r := readingAt(t0, "100")
	d := energy.Difference(energy.Window{From: t0, To: t0.Add(time.Hour)}, r, r)
	require.False(t, d.Emitted, "identical boundary readings must not become a zero row")
	require.Empty(t, d.Values)
}

func TestDifferenceSubtractsPerRegisterAndKeepsNilNil(t *testing.T) {
	start := &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{
		energy.ActiveImport: dec("1000.5000"), energy.T1Import: dec("400.0000"),
	}}
	end := &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{
		energy.ActiveImport: dec("1012.7500"), // T1Import absent at the end boundary
	}}
	d := energy.Difference(energy.Window{From: t0, To: t0.Add(time.Hour)}, start, end)
	require.True(t, d.Emitted)
	require.Equal(t, "12.25", d.Values[energy.ActiveImport].String())
	require.Nil(t, d.Values[energy.T1Import], "an unreported register is nil, never zero")
	require.Empty(t, d.Suspect, "an unreported register is not suspect")
}

func TestDifferenceLeavesANegativeDeltaSuspectAndNeverReturnsTheEndValue(t *testing.T) {
	start := readingAt(t0, "999000.0000")
	end := readingAt(t0.Add(time.Hour), "12.5000")
	d := energy.Difference(energy.Window{From: t0, To: t0.Add(time.Hour)}, start, end)
	require.True(t, d.Emitted)
	require.Nil(t, d.Values[energy.ActiveImport], "removed-behaviour 1: never the end register value")
	require.Equal(t, energy.ReasonNegativeDelta, d.Suspect[energy.ActiveImport].Reason)
	require.Equal(t, "-998987.5", d.Suspect[energy.ActiveImport].Delta.String())
}
```

- [ ] **Step 2: Run and watch them fail**

Run: `go test ./internal/domain/energy/... -run 'TestBucket|TestDifference' -v`
Expected: FAIL — package does not compile (`undefined: energy.Bucket`).

- [ ] **Step 3: Implement `types.go`, `period.go`, `difference.go`.**
`doc.go` carries the package doc mapping each exported function to its spec section. `period.go` imports `time/tzdata` so the zone resolves in a scratch container. Keep `Difference` a single pass over `AllRegisters()`.

- [ ] **Step 4: Run and watch them pass.** `go test ./internal/domain/energy/... -v`

- [ ] **Step 5: Extend the guards and prove each new one fails.**
  - `internal/arch/arch_test.go`: add `"./internal/service/..."` to `floatGuardPatterns` and to `TestIntegrationTreesDoNotParseFloats`'s tree list; add the exact-match entry `"RefreshConsumption"` to `storeScopeAllowlist` with the comment `// R72: a continuous-aggregate refresh covers every tenant's buckets; a scoped signature would be a lie. Implemented in internal/store/postgres/admin (Task 6).`
  - Add `TestServiceLayerImportBoundaries`: walks `./internal/service/...` and fails on an import of `internal/api` or `internal/ingest`; walks `./internal/store/...` and `./internal/integration/...` and fails on an import of `internal/service`.
  - `.golangci.yml`: add `./internal/service/...` to the `no-float-money` and `domain-is-pure` tree lists — change both or neither.

| Guard | Mutation that must turn it red |
|---|---|
| `TestServiceLayerImportBoundaries` | create `internal/service/probe/probe.go` importing `.../internal/ingest`; expect FAIL naming the import. Delete the probe. |
| `floatGuardPatterns` incl. service | add `type probe struct{ X float64 }` to that same probe file; expect FAIL naming `probe.X`. Delete. |
| `storeScopeAllowlist` entry | the entry is inert until Task 6; assert only that `go test ./internal/arch/...` stays green with it present, and record that Task 6 proves it. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build && make check-generate
git add internal/domain/energy internal/arch/arch_test.go .golangci.yml
git commit -m "feat(f3): task-1 — energy contract, Istanbul buckets, §3.1 differencing, service-layer guards" <TRAILERS>
```

---

## Task 2: Reset segmentation and suspect classification (§3.2)

**Files:**
- Create: `internal/domain/energy/reset.go`, `internal/domain/energy/suspect.go`
- Test: `internal/domain/energy/reset_test.go`, `internal/domain/energy/suspect_test.go`

**Interfaces:**
- Consumes: Task 1's `Reading`, `Window`, `Derivation`, `Suspicion`, `Reason`, `Difference`.
- Produces:

```go
// Derive implements 02 §3.1 together with §3.2.
//
// start and end are the boundary readings (§3.1). resets are the kind=reset
// readings whose TS falls inside w, in any order; Derive sorts them.
//
// With reset evidence the period is split at every reset and consumption is the
// sum of the segment differences (R55, R56):
//
//	(before_reset - reading_start) + (reading_end - after_reset)
//
// after_reset is the reset row's own value. before_reset is the value of the
// last non-reset reading strictly before the reset, taken from priors; when no
// such reading exists it is reading_start's value, so the pre-reset segment
// contributes zero rather than a guess.
//
// Without usable evidence the register is suspect and its value is nil. A
// negative difference NEVER becomes a number (removed-behaviour 1).
func Derive(w Window, start, end *Reading, resets []Reading, priors []Reading) Derivation
```

**Rules the implementation must follow:**
- Sort `resets` by `TS`; ignore any whose `TS` is outside `w`.
- Per register, walk the segments in order. A segment whose own difference is negative makes that register suspect with `ReasonNegativeDelta` and discards the whole period's value for it.
- A reset row whose value for a register is nil makes that register suspect with `ReasonMeterReset` (R55) — never zero, never skipped silently.
- `Suspicion.ResetRows` records how many reset rows were inside the window, so the operator message can say "a reset exists but could not be applied" versus "no reset at all".
- Registers untouched by the problem keep their derived values (R58).
- No rounding, no modulus, no rollover arithmetic (R57).

- [ ] **Step 1: Write the failing tests**

```go
func TestDeriveAppliesTheResetFormula(t *testing.T) {
	// start 900 -> before reset 1000 (+100), meter resets to 0 -> end 40 (+40)
	start := readingAt(t0, "900")
	before := readingAt(t0.Add(30*time.Minute), "1000")
	reset := resetAt(t0.Add(40*time.Minute), "0")
	end := readingAt(t0.Add(time.Hour), "40")
	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*before})
	require.True(t, d.Emitted)
	require.Equal(t, "140", d.Values[energy.ActiveImport].String())
	require.Empty(t, d.Suspect)
}

func TestDeriveWithAResetThatDoesNotRestartAtZero(t *testing.T) {
	// A replacement meter starts at 5, not 0: the formula uses the row's own value.
	d := energy.Derive(win(t0, time.Hour), readingAt(t0, "900"), readingAt(t0.Add(time.Hour), "45"),
		[]energy.Reading{*resetAt(t0.Add(40*time.Minute), "5")}, []energy.Reading{*readingAt(t0.Add(30*time.Minute), "1000")})
	require.Equal(t, "140", d.Values[energy.ActiveImport].String())
}

func TestDeriveWithoutAResetEventIsSuspect(t *testing.T) {
	d := energy.Derive(win(t0, time.Hour), readingAt(t0, "999000"), readingAt(t0.Add(time.Hour), "12.5"), nil, nil)
	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonNegativeDelta, d.Suspect[energy.ActiveImport].Reason)
	require.Zero(t, d.Suspect[energy.ActiveImport].ResetRows)
}

func TestDeriveWithAnUnusableResetRowIsSuspectForThatRegisterOnly(t *testing.T) {
	reset := resetAt(t0.Add(40*time.Minute), "0")
	delete(reset.Values, energy.T1Import) // the reset row reports no T1
	start := &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: vals("900", "400")}
	end := &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindLoadProfile, Values: vals("40", "10")}
	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*readingAt(t0.Add(30*time.Minute), "1000")})
	require.Equal(t, "140", d.Values[energy.ActiveImport].String(), "a sound register still derives")
	require.Nil(t, d.Values[energy.T1Import])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.T1Import].Reason)
	require.Equal(t, 1, d.Suspect[energy.T1Import].ResetRows)
}

func TestDeriveHandlesTwoResetsInOnePeriod(t *testing.T) { /* 900->1000 reset 0 ->60 reset 0 ->25 == 185 */ }

func TestDeriveWithNoReadingBeforeTheResetContributesZeroForTheFirstSegment(t *testing.T) {
	// No prior between start and the reset: before_reset == reading_start, so the
	// first segment is 0 and only the post-reset segment counts.
	d := energy.Derive(win(t0, time.Hour), readingAt(t0, "900"), readingAt(t0.Add(time.Hour), "40"),
		[]energy.Reading{*resetAt(t0.Add(10*time.Minute), "0")}, nil)
	require.Equal(t, "40", d.Values[energy.ActiveImport].String())
}
```

- [ ] **Step 2: Run and watch them fail.** `go test ./internal/domain/energy/... -run TestDerive -v` → FAIL, `undefined: energy.Derive`.
- [ ] **Step 3: Implement `reset.go` and `suspect.go`.**
- [ ] **Step 4: Run and watch them pass.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestDeriveWithoutAResetEventIsSuspect` | return the end reading's value when the difference is negative (the legacy defect, removed-behaviour 1): expect FAIL with `12.5` observed. Restore. |
| `TestDeriveWithAnUnusableResetRowIsSuspectForThatRegisterOnly` | treat a nil reset value as `decimal.Zero`: expect FAIL — `T1Import` derives a number instead of being suspect. Restore. |
| `TestDeriveAppliesTheResetFormula` | drop the pre-reset segment (return only `end - after`): expect FAIL, `40` ≠ `140`. Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
git add internal/domain/energy
git commit -m "feat(f3): task-2 — §3.2 reset segmentation and per-register suspicion" <TRAILERS>
```

---

## Task 3: Ratios, max demand and activity status (§3.3, §3.5, §3.6)

**Files:**
- Create: `internal/domain/energy/ratio.go`, `internal/domain/energy/demand.go`, `internal/domain/energy/activity.go`
- Test: one `_test.go` per file

**Interfaces:**
- Consumes: Task 1's types.
- Produces:

```go
// Ratio divides two quantities and is nil whenever the result would be
// meaningless: a nil or non-positive denominator, or a nil numerator (R54).
// 02 §3.3 says "0 when active_import = 0"; 09 §F3's acceptance criterion
// requires a null ratio, and the acceptance criterion wins. A ratio is never
// computed against a substituted 1.
func Ratio(numerator, denominator *decimal.Decimal) *decimal.Decimal

// Ratios returns the inductive and capacitive ratios of a derivation (§3.3),
// both relative to derived active-import consumption.
func Ratios(d Derivation) (inductive, capacitive *decimal.Decimal)

// MaxDemand is the maximum of the interval max_demand values in the window —
// never a difference and never a sum (§3.5, R65). Readings of kind reset are
// ignored; nil when no reading carries a value.
func MaxDemand(w Window, readings []Reading) *decimal.Decimal

// ActivityWindow is §3.6's fixed 7 days. It is deliberately NOT the configurable
// data-communication alarm threshold (F7) and NOT analyzers.is_active, which is
// operator enablement (R66).
const ActivityWindow = 7 * 24 * time.Hour

type Status string

const (
	StatusActive  Status = "active"
	StatusPassive Status = "passive"
)

// ActivityStatus reports §3.6's status. A nil last reading is passive.
func ActivityStatus(lastReadingAt *time.Time, now time.Time) Status
```

- [ ] **Step 1: Write the failing tests**

```go
func TestRatioIsNilWhenConsumptionIsZero(t *testing.T) {
	require.Nil(t, energy.Ratio(dec("12.5"), dec("0")))
	require.Nil(t, energy.Ratio(dec("12.5"), nil))
	require.Nil(t, energy.Ratio(nil, dec("100")))
}

func TestRatioIsAnUnroundedFraction(t *testing.T) {
	require.Equal(t, "0.33", energy.Ratio(dec("33"), dec("100")).String())
}

func TestRatiosOfASuspectDerivationAreNil(t *testing.T) { /* active suspect -> both nil */ }

func TestMaxDemandTakesTheMaximumNotTheDifference(t *testing.T) {
	rs := []energy.Reading{demandAt(t0, "12.5"), demandAt(t0.Add(15*time.Minute), "40.25"), demandAt(t0.Add(30*time.Minute), "9")}
	require.Equal(t, "40.25", energy.MaxDemand(win(t0, time.Hour), rs).String())
}

func TestMaxDemandIsNilWhenNoReadingCarriesOne(t *testing.T) { require.Nil(t, energy.MaxDemand(win(t0, time.Hour), []energy.Reading{*readingAt(t0, "100")})) }

func TestMaxDemandIncludesCurrentIndexReadingsAndExcludesResets(t *testing.T) { /* R64, R65 */ }

func TestActivityStatusUsesSevenDays(t *testing.T) {
	now := time.Date(2025, 3, 14, 9, 0, 0, 0, time.UTC)
	within := now.Add(-energy.ActivityWindow + time.Minute)
	outside := now.Add(-energy.ActivityWindow - time.Minute)
	require.Equal(t, energy.StatusActive, energy.ActivityStatus(&within, now))
	require.Equal(t, energy.StatusPassive, energy.ActivityStatus(&outside, now))
	require.Equal(t, energy.StatusPassive, energy.ActivityStatus(nil, now))
}
```

- [ ] **Step 2: Run and watch them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run and watch them pass.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestRatioIsNilWhenConsumptionIsZero` | substitute `decimal.NewFromInt(1)` for a zero denominator (the exact defect the acceptance criterion names): expect FAIL — a ratio of `12.5` appears. Restore. |
| `TestMaxDemandTakesTheMaximumNotTheDifference` | return `last - first`: expect FAIL. Restore. |
| `TestActivityStatusUsesSevenDays` | change `ActivityWindow` to 30 days: expect FAIL on the `outside` case. Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
git add internal/domain/energy
git commit -m "feat(f3): task-3 — §3.3 ratios, §3.5 max demand, §3.6 activity status" <TRAILERS>
```

---

## Task 4: The golden-file corpus (09 §F3 acceptance criterion 1)

**Files:**
- Create: `internal/domain/energy/golden_test.go`, `internal/domain/energy/testdata/golden/{normal,missing_readings,identical_boundaries,reset_with_event,reset_without_event,rollover}.json`
- Test: the same file

**Interfaces:**
- Consumes: Tasks 1-3 (`Derive`, `Difference`, `Ratios`, `MaxDemand`).
- Produces: `TestGolden`, run by the phase verification block `go test ./internal/domain/... -run TestGolden -v`.

**Rules the implementation must follow (R80):**
- One JSON file per case, holding `name`, `doc` (one line saying which spec rule the case proves), `input` (window, boundary readings, resets, priors) and `want` (values, suspect map, ratios, max demand) together.
- `-update` rewrites only `want`. The harness never writes a golden file during a normal run and never writes `input`.
- Decimals are strings on both sides. A missing value is JSON `null`, never `0` and never omitted.
- The six cases are exactly the six the acceptance criterion names: a normal period, a period with missing readings, a period whose boundary readings are identical, a meter reset with a reset event, a meter reset without one, and a rollover (R57: a wrap **with** a reset row; the uncovered wrap is the `reset_without_event` case).

- [ ] **Step 1: Write the failing test and the six `input` blocks with hand-computed `want` blocks.**

```go
var update = flag.Bool("update", false, "rewrite the want block of each golden file")

func TestGolden(t *testing.T) {
	for _, path := range mustGlob(t, "testdata/golden/*.json") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			var c goldenCase
			mustReadJSON(t, path, &c)
			got := runCase(t, c.Input) // Derive + Ratios + MaxDemand
			if *update {
				c.Want = got
				mustWriteJSON(t, path, c)
				return
			}
			require.Equal(t, c.Want, got, "%s: %s", c.Name, c.Doc)
		})
	}
}
```

Hand-computed `want` for `normal.json` (the numbers are computed by hand in the task report, not by running the code):
```json
{
  "name": "normal",
  "doc": "02 §3.1: a period with both boundaries present differences every register",
  "input": { "window": {"from": "2025-03-14T00:00:00+03:00", "to": "2025-03-14T01:00:00+03:00"},
             "start": {"ts": "2025-03-14T00:00:00+03:00", "kind": "load_profile",
                       "values": {"active_import": "1000.5000", "reactive_inductive_import": "120.0000", "t1_import": "400.0000"}},
             "end":   {"ts": "2025-03-14T01:00:00+03:00", "kind": "load_profile",
                       "values": {"active_import": "1012.7500", "reactive_inductive_import": "124.0450", "t1_import": "412.2500"}} },
  "want":  { "emitted": true,
             "values": {"active_import": "12.25", "reactive_inductive_import": "4.045", "t1_import": "12.25"},
             "suspect": {},
             "inductive_ratio": "0.3302040816326530612",
             "capacitive_ratio": null,
             "max_demand_kw": null }
}
```

- [ ] **Step 2: Run and watch it fail.** `go test ./internal/domain/energy -run TestGolden -v` → FAIL (harness missing, then mismatches).
- [ ] **Step 3: Implement the harness.** No `-update` run is allowed to produce the first `want`: the six `want` blocks are authored by hand from the spec and the test must pass without `-update`.
- [ ] **Step 4: Run and watch them pass.**
- [ ] **Step 5: Prove the harness is load-bearing.**

| Test | Mutation |
|---|---|
| `TestGolden` | edit one digit of `want.values.active_import` in `normal.json`: expect FAIL naming the case and the doc line. Restore. |
| `TestGolden` | in `reset_without_event.json`, change `want.values.active_import` from `null` to a number: expect FAIL. Restore. |
| `-update` safety | run with `-update` after corrupting an `input` value; assert (in the task report) that `input` is unchanged in the diff and only `want` moved. `git checkout` the testdata afterwards. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
git add internal/domain/energy
git commit -m "test(f3): task-4 — golden-file corpus for the six §3.1/§3.2 cases" <TRAILERS>
```

---

## Task 5: `internal/domain/loadprofile` — classification, profiles and statistics (§10.3)

**Files:**
- Create: `internal/domain/loadprofile/{doc,classify,profile,statistics}.go`
- Test: `internal/domain/loadprofile/{classify,profile,statistics}_test.go`

**Interfaces:**
- Consumes: nothing from the project (pure). `decimal`, stdlib.
- Produces:

```go
// Package loadprofile implements docs/rewrite/02-domain-rules.md §10.3.
// It is pure: the company's weekend days and vacation periods are parameters,
// resolved by internal/service/loadprofile (R69).
package loadprofile

type DayType string

const (
	Weekday DayType = "weekday"
	Weekend DayType = "weekend"
)

type Season string

const (
	Winter Season = "winter" // Dec, Jan, Feb — meteorological, per §10.3 (R82)
	Spring Season = "spring" // Mar, Apr, May
	Summer Season = "summer" // Jun, Jul, Aug
	Autumn Season = "autumn" // Sep, Oct, Nov
)

// Key is a profile identifier: "weekday", "weekend", and the eight
// "<season>_<daytype>" combinations.
type Key string

// DateRange is an inclusive [Start, End] range of calendar dates (company_vacations).
type DateRange struct{ Start, End time.Time }

// Config is the company's classification configuration.
// WeekendDays is applied exactly as given; the service supplies the default
// when the company configured none (R69). Vacations are inclusive date ranges.
// calendar_events never participate (R70).
type Config struct {
	WeekendDays map[time.Weekday]bool
	Vacations   []DateRange
	Location    *time.Location
}

// Classify reports whether a calendar date is a weekday or a weekend for this
// company: a configured non-working weekday, or any date inside a vacation
// period, is Weekend (§10.3).
func Classify(day time.Time, cfg Config) DayType

// SeasonOf returns the meteorological season of a date (R82).
func SeasonOf(day time.Time, loc *time.Location) Season

// KeysFor returns the profile keys a day contributes to: its day type and its
// season+day-type combination.
func KeysFor(dt DayType, s Season) []Key

// HourValue is one hourly consumption figure. Day is the Istanbul-local
// calendar date the hour belongs to; Hour is 0-23 (R83).
type HourValue struct {
	Day   time.Time
	Hour  int
	Value decimal.Decimal
}

// Profile holds the mean consumption for each hour of the day, or nil for an
// hour no matching day reported (R68: values are active-import consumption;
// the package never names a register).
type Profile struct {
	Key   Key
	Hours [24]*decimal.Decimal
	Days  int // distinct days that contributed
}

// Build groups values by profile and averages each hour across matching days.
func Build(values []HourValue, cfg Config) map[Key]Profile

// Statistics are computed over the 24 hourly means — not over each hour's
// spread across days (R84). Every field is nil when the profile has no hours.
type Statistics struct {
	Max        *decimal.Decimal
	Min        *decimal.Decimal
	HourOfMax  *int
	Mean       *decimal.Decimal
	StdDev     *decimal.Decimal // population standard deviation, divisor n (R67)
	Range      *decimal.Decimal // Max - Min
	LoadFactor *decimal.Decimal // Mean / Max; decimal.Zero when Max is zero (R81)
}

func Stats(p Profile) Statistics
```

**Rules the implementation must follow:**
- `Classify` compares dates, never instants: convert to `cfg.Location`, take the calendar date, and test vacation ranges inclusively on both ends.
- A day with no matching values contributes nothing; an hour with no values is `nil`, not zero (an idle building that reported zero is `decimal.Zero`, which is different).
- `Mean` is the mean of the non-nil hourly means. `HourOfMax` is the lowest hour index attaining the maximum, so the result is deterministic under ties.
- `StdDev` uses divisor `n` over the same non-nil hourly means (R67), and the doc comment says so.
- No rounding anywhere (R79). Division for means and load factor uses `decimal.Decimal.DivRound` with a scale of at least 20, documented as "precision, not presentation".

- [ ] **Step 1: Write the failing tests**

```go
func TestClassifyUsesTheConfiguredWeekendDays(t *testing.T) {
	cfg := loadprofile.Config{WeekendDays: map[time.Weekday]bool{time.Friday: true}, Location: ist}
	require.Equal(t, loadprofile.Weekend, loadprofile.Classify(date(2025, 3, 14), cfg)) // a Friday
	require.Equal(t, loadprofile.Weekday, loadprofile.Classify(date(2025, 3, 15), cfg)) // a Saturday
}

func TestClassifyTreatsAVacationDayAsWeekend(t *testing.T) {
	cfg := loadprofile.Config{
		WeekendDays: map[time.Weekday]bool{time.Saturday: true, time.Sunday: true},
		Vacations:   []loadprofile.DateRange{{Start: date(2025, 4, 23), End: date(2025, 4, 23)}},
		Location:    ist,
	}
	require.Equal(t, loadprofile.Weekend, loadprofile.Classify(date(2025, 4, 23), cfg), "a national holiday inside a vacation period")
	require.Equal(t, loadprofile.Weekday, loadprofile.Classify(date(2025, 4, 24), cfg))
}

func TestClassifyIncludesBothEndsOfAVacationRange(t *testing.T) { /* Start and End both Weekend; End+1 Weekday */ }

func TestSeasonOfUsesMeteorologicalMonths(t *testing.T) {
	require.Equal(t, loadprofile.Winter, loadprofile.SeasonOf(date(2025, 12, 1), ist))
	require.Equal(t, loadprofile.Winter, loadprofile.SeasonOf(date(2025, 2, 28), ist))
	require.Equal(t, loadprofile.Spring, loadprofile.SeasonOf(date(2025, 3, 1), ist), "R82: not the 21 March legacy cutoff")
}

func TestBuildAveragesEachHourAcrossMatchingDays(t *testing.T) {
	// Two weekdays: hour 9 = 10 and 20 -> mean 15. Hour 10 reported once -> 7.
	got := loadprofile.Build([]loadprofile.HourValue{
		{Day: date(2025, 3, 10), Hour: 9, Value: d("10")},
		{Day: date(2025, 3, 11), Hour: 9, Value: d("20")},
		{Day: date(2025, 3, 11), Hour: 10, Value: d("7")},
	}, weekdayCfg)
	p := got["weekday"]
	require.Equal(t, "15", p.Hours[9].String())
	require.Equal(t, "7", p.Hours[10].String())
	require.Nil(t, p.Hours[0], "an hour no day reported stays nil")
	require.Equal(t, 2, p.Days)
}

func TestBuildPutsADayInBothItsDayTypeAndItsSeasonProfile(t *testing.T) {
	got := loadprofile.Build([]loadprofile.HourValue{{Day: date(2025, 1, 8), Hour: 3, Value: d("5")}}, weekdayCfg)
	require.Contains(t, got, loadprofile.Key("weekday"))
	require.Contains(t, got, loadprofile.Key("winter_weekday"))
	require.NotContains(t, got, loadprofile.Key("weekend"))
}

func TestStatsMatchHandComputedValues(t *testing.T) {
	// hours 0..3 = 10, 20, 30, 40; the other 20 hours are nil.
	// mean = 25, max = 40 (hour 3), min = 10, range = 30,
	// population variance = ((15^2)+(5^2)+(5^2)+(15^2))/4 = 125, stddev = 11.1803398874989484820...
	// load factor = 25/40 = 0.625
	s := loadprofile.Stats(profileWith(map[int]string{0: "10", 1: "20", 2: "30", 3: "40"}))
	require.Equal(t, "25", s.Mean.String())
	require.Equal(t, "40", s.Max.String())
	require.Equal(t, 3, *s.HourOfMax)
	require.Equal(t, "30", s.Range.String())
	require.True(t, s.StdDev.StringFixed(10) == "11.1803398875", s.StdDev.String())
	require.Equal(t, "0.625", s.LoadFactor.String())
}

func TestStatsLoadFactorIsZeroWhenTheProfileIsFlatZero(t *testing.T) {
	s := loadprofile.Stats(profileWith(map[int]string{0: "0", 1: "0"}))
	require.True(t, s.LoadFactor.IsZero(), "R81: §10.3 says 0 when max = 0, unlike R54's ratios")
}

func TestStatsHourOfMaxIsTheLowestHourOnATie(t *testing.T) { /* 5,5 -> hour 0 */ }
```

- [ ] **Step 2: Run and watch them fail.**
- [ ] **Step 3: Implement.** `doc.go` states the R67/R68/R81/R82/R83/R84 rulings and their spec citations.
- [ ] **Step 4: Run and watch them pass.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestClassifyTreatsAVacationDayAsWeekend` | drop the vacation branch: expect FAIL. Restore. |
| `TestStatsMatchHandComputedValues` | use divisor `n-1` for the standard deviation: expect FAIL (`12.909...` ≠ `11.180...`). Restore. |
| `TestSeasonOfUsesMeteorologicalMonths` | implement the legacy 21 March cutoff: expect FAIL on 1 March. Restore. |
| `TestBuildAveragesEachHourAcrossMatchingDays` | treat a missing hour as zero when averaging: expect FAIL (`p.Hours[0]` becomes `0`). Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
git add internal/domain/loadprofile
git commit -m "feat(f3): task-5 — load profile classification, averaging and statistics" <TRAILERS>
```

---

## Task 6: The continuous-aggregate refresh seam (R71, R72, R73's storage half)

**Files:**
- Create: `internal/store/postgres/admin/aggregates.go`
- Modify: `internal/store/repository.go` (add `AggregateView` and `AdminAggregateRepository` only)
- Test: `internal/store/postgres/admin/aggregates_integration_test.go`

**Interfaces:**
- Consumes: F1's migrations `00004`/`00005`, `testfixtures.NewIsolatedDB`, `testfixtures.NewTenant`, the arch allowlist entry Task 1 pre-added.
- Produces:

```go
// store

// AggregateView names one continuous aggregate built by migration 00005.
// It is a closed set: a caller can never pass a view name, so the refresh
// statement is never assembled from caller input.
type AggregateView string

const (
	ViewConsumptionHourly   AggregateView = "consumption_hourly"
	ViewConsumptionDaily    AggregateView = "consumption_daily"
	ViewConsumptionMonthly  AggregateView = "consumption_monthly"
	ViewConsumptionYearly   AggregateView = "consumption_yearly"
)

// ConsumptionViews are the four consumption aggregates, finest first. A refresh
// walks them in this order so a coarser view never materialises from a finer
// view that has not been refreshed yet.
func ConsumptionViews() []AggregateView

// AdminAggregateRepository refreshes continuous aggregates.
//
// This is platform work, not tenant work: refresh_continuous_aggregate takes a
// time range and nothing else, so the call necessarily covers every tenant's
// buckets in that range. A Scope parameter would be a lie, so this lives with
// the other Admin* interfaces (R72) and is implemented in postgres/admin.
type AdminAggregateRepository interface {
	// Refresh materialises [r.From, r.To) of view. It must not run inside a
	// transaction: refresh_continuous_aggregate commits its own work
	// (SQLSTATE 25001 otherwise).
	Refresh(ctx context.Context, view AggregateView, r TimeRange) error
}
```

**Rules the implementation must follow:**
- The statement is `CALL refresh_continuous_aggregate($1::regclass, $2, $3)` executed on the pool, never on a `pgx.Tx`, and the first parameter is bound from the `AggregateView` constant — never string-formatted.
- An unknown `AggregateView` is rejected with a sentinel error before any I/O.
- `Refresh` validates `r.Valid()` first and returns `ErrInvalidRange` otherwise (F1's contract for every method taking a `TimeRange`).
- Errors go through `pgerr.Translate` like every other repository.

- [ ] **Step 1: Write the failing integration test** (`//go:build integration`)

```go
// The central proof of the phase's refresh story: a bucket written below the
// refresh policy's start_offset is ABSENT from the aggregate — not stale — and
// only an explicit refresh materialises it (04 §4.3; migration 00005's operator note).
func TestRefreshMaterialisesABucketOlderThanThePolicyWindow(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ten := testfixtures.NewTenant(t, ctx, pool, 1)
	analyzer := ten.Analyzers[0]

	// 200 days ago: far outside consumption_hourly's 30-day start_offset.
	base := time.Now().Add(-200 * 24 * time.Hour).Truncate(time.Hour)
	insertReadings(t, pool, analyzer.ID, base, []string{"1000", "1012.5"}) // one hour apart

	an := postgres.NewAnalyticsRepository(pool)
	before, err := an.ConsumptionHourly(ctx, ten.AdminScope, []uuid.UUID{analyzer.ID}, store.TimeRange{From: base, To: base.Add(2 * time.Hour)})
	require.NoError(t, err)
	require.Empty(t, before, "a backfilled bucket below the watermark is absent, not stale")

	repo := admin.NewAggregateRepository(pool)
	require.NoError(t, repo.Refresh(ctx, store.ViewConsumptionHourly, store.TimeRange{From: base.Add(-time.Hour), To: base.Add(2 * time.Hour)}))

	after, err := an.ConsumptionHourly(ctx, ten.AdminScope, []uuid.UUID{analyzer.ID}, store.TimeRange{From: base, To: base.Add(2 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.Equal(t, "12.5", after[0].ActiveConsumption.String())
}

func TestRefreshRejectsAnUnknownViewBeforeTouchingTheDatabase(t *testing.T) { /* sentinel, no query executed */ }
func TestRefreshRejectsAnInvalidRange(t *testing.T)                         { /* ErrInvalidRange */ }
func TestRefreshIsRejectedInsideATransaction(t *testing.T)                  { /* documents SQLSTATE 25001 */ }
func TestRefreshWalksTheFourViewsFinestFirst(t *testing.T)                  { /* ConsumptionViews order */ }
```

- [ ] **Step 2: Run and watch them fail.** `go test ./internal/store/postgres/admin/... -tags=integration -run TestRefresh -v`
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run and watch them pass.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestRefreshMaterialisesABucketOlderThanThePolicyWindow` | make `Refresh` a no-op returning nil: expect FAIL — the post-refresh read is still empty. This is the proof the whole refresh story rests on. Restore. |
| `TestRefreshRejectsAnUnknownViewBeforeTouchingTheDatabase` | build the statement with `fmt.Sprintf`: expect FAIL (an unknown view reaches the database). Restore — and note in the report that this mutation is also the injection hazard the closed set exists to prevent. |
| arch allowlist (Task 1) | remove the `"RefreshConsumption"` entry and rename the method to a scoped-looking name: confirm `TestEveryStoreMethodIsScoped` catches an unscoped `context.Context` method outside `admin`. Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build && make check-generate
go test ./internal/store/... -tags=integration -race -count=1
git add internal/store
git commit -m "feat(f3): task-6 — platform-scoped continuous-aggregate refresh seam" <TRAILERS>
```

---

## Task 7: `internal/service/consumption` — the two paths (04 §4.3, R61, R62, R63)

**Files:**
- Create: `internal/service/consumption/{doc,service,analytics,billing}.go`
- Test: `internal/service/consumption/{analytics,billing}_test.go`, `internal/service/consumption/paths_integration_test.go`

**Interfaces:**
- Consumes: Task 1-3's `energy` package; `store.AnalyticsRepository`, `store.ReadingRepository`, `store.AnomalyRepository`, `store.OpsRepository`, `store.Scope`, `store.TimeRange`; `platform/clock`.
- Produces:

```go
package consumption

// AnalyticsDeps is everything the analytics path may reach. It deliberately
// has no reading repository: the analytics path must not be able to read the
// hypertable (R61).
type AnalyticsDeps struct {
	Analytics store.AnalyticsRepository
	Log       *slog.Logger
}

// BillingDeps is everything the billing path may reach. It deliberately has no
// analytics repository: the billing path must not be able to read an aggregate.
type BillingDeps struct {
	Readings  store.ReadingRepository
	Anomalies store.AnomalyRepository
	Ops       store.OpsRepository
	Clock     clock.Clock
	Log       *slog.Logger
}

type Service struct { /* unexported */ }

func New(a AnalyticsDeps, b BillingDeps) (*Service, error)

// SeriesRequest asks for one analyzer's series at one level.
// AnalyzerIDs is required and positional: empty means no rows, never "all".
type SeriesRequest struct {
	AnalyzerIDs []uuid.UUID
	Level       energy.Level
	Range       store.TimeRange
}

// Row is one period. Values carries every register's consumption; a nil entry
// is unreported or suspect. Source records which reading kind produced the row
// (R62): "load_profile" or "billing".
type Row struct {
	AnalyzerID      uuid.UUID
	Window          energy.Window
	Values          map[energy.Register]*decimal.Decimal
	InductiveRatio  *decimal.Decimal
	CapacitiveRatio *decimal.Decimal
	MaxDemandKw     *decimal.Decimal
	Source          string
	Suspect         map[energy.Register]energy.Suspicion
}

// Analytics serves charts and tables from the continuous aggregates. It is
// fast and slightly understates a period: a bucket misses the step between the
// last reading of one bucket and the first of the next (04 §4.3).
func (s *Service) Analytics(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error)

// Billing derives consumption from meter_readings at the true period
// boundaries (02 §3.1, §3.2). It is exact, slower, and the only path that may
// feed an invoice. A suspect period returns nil values and a Suspicion; it
// never returns a number (removed-behaviour 1). Writing the anomaly row and the
// operator message is Task 8's Derive-and-record wrapper.
func (s *Service) Billing(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error)
```

**Rules the implementation must follow:**
- `Analytics` maps `Level` to the matching `AnalyticsRepository` method and converts `model.ConsumptionBucket` to `Row`; it computes ratios with `energy.Ratio` from the bucket's own values and sets `Source: "load_profile"` always (R62). It never calls `energy.Derive`.
- `Billing` walks `energy.Buckets(level, range, istanbul)`; for each window it calls `BoundaryReadings` for the selected kind, reads reset rows for the window (`Range(..., KindReset)`) and the priors needed by R55, then `energy.Derive`. `current_index` is never a boundary kind (R64).
- Monthly billing-kind preference (R63): for `Monthly`, first try `BoundaryReadings(..., KindBilling, ...)`; when both boundaries are non-nil and their timestamps differ, derive from those and set `Source: "billing"`; otherwise fall back to `KindLoadProfile`. A period never mixes kinds.
- Max demand comes from `energy.MaxDemand` over `Range(..., KindLoadProfile)` plus `Range(..., KindCurrentIndex)` for the window (R65).
- Both methods validate `sc.Valid()`, a non-empty `AnalyzerIDs` and `req.Range.Valid()` before any I/O.

- [ ] **Step 1: Write the failing tests** — unit tests with fakes for the path separation, integration tests for the real difference.

```go
// R61: the separation is structural, not documentary.
func TestAnalyticsPathCannotReachTheHypertable(t *testing.T) {
	svc, _ := consumption.New(
		consumption.AnalyticsDeps{Analytics: fakeAnalytics{rows: oneBucket()}, Log: testLog(t)},
		consumption.BillingDeps{Readings: failingReadings{t}, Anomalies: noAnomalies{}, Ops: noOps{}, Clock: clock.Fixed(now), Log: testLog(t)},
	)
	_, err := svc.Analytics(ctx, scope, req)
	require.NoError(t, err) // failingReadings fails the test if any method is called
}

func TestBillingPathCannotReachAnAggregate(t *testing.T) { /* symmetric: failingAnalytics */ }

// 09 §F3 acceptance: the two paths differ at a bucket boundary by the expected step.
func TestBillingAndAnalyticsDifferByTheBucketBoundaryStep(t *testing.T) {
	// Readings at 09:00=1000, 09:45=1030, 10:00=1040 (the 09:45->10:00 step is 10).
	// analytics (last-first inside the 09:00 bucket) = 1030-1000 = 30
	// billing   (boundary 09:00 -> boundary 10:00)   = 1040-1000 = 40
	require.Equal(t, "30", analyticsRow.Values[energy.ActiveImport].String())
	require.Equal(t, "40", billingRow.Values[energy.ActiveImport].String())
	require.Equal(t, "10", sub(billingRow, analyticsRow).String(), "exactly the step the aggregate misses")
}

// 09 §F3 acceptance: levels derive from their own boundaries, never by summing below.
func TestDailyDoesNotEqualTheSumOfItsHoursWhenAReadingIsMissing(t *testing.T) {
	// A whole hour of readings is missing mid-day: two hourly rows are absent,
	// but the daily row still spans 00:00 -> 00:00 and is correct.
	require.NotEqual(t, sumOf(hourly), daily.Values[energy.ActiveImport])
	require.Equal(t, "240", daily.Values[energy.ActiveImport].String())
}

func TestMonthlyPrefersBillingKindReadingsWhenPresent(t *testing.T) {
	require.Equal(t, "billing", row.Source)
	require.Equal(t, "5000", row.Values[energy.ActiveImport].String(), "the utility snapshot, not the load profile")
}

func TestMonthlyFallsBackToLoadProfileWhenBillingReadingsDoNotCoverTheMonth(t *testing.T) { /* R63: one billing reading only */ }
func TestBillingRefusesAnEmptyAnalyzerList(t *testing.T)                                  { /* fail-closed, no rows, no query */ }
func TestBillingIsolatesTenants(t *testing.T)                                             { /* the other tenant's AdminScope + a positive control */ }
```

- [ ] **Step 2: Run and watch them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run and watch them pass.** Unit first, then `go test ./internal/service/... -tags=integration -race -count=1`.
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestAnalyticsPathCannotReachTheHypertable` | give `Analytics` a fallback to `Readings` when the aggregate returns no rows: expect FAIL from `failingReadings`. Restore. |
| `TestBillingAndAnalyticsDifferByTheBucketBoundaryStep` | make `Billing` read the aggregate: expect FAIL — the difference collapses to 0. Restore. |
| `TestMonthlyPrefersBillingKindReadingsWhenPresent` | drop the billing-kind attempt: expect FAIL — `Source` is `load_profile`. Restore. |
| `TestBillingIsolatesTenants` | remove the scope argument from one repository call: expect FAIL — the other tenant's rows appear. Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
go test ./internal/service/... -tags=integration -race -count=1
git add internal/service/consumption
git commit -m "feat(f3): task-7 — the analytics and billing consumption paths" <TRAILERS>
```

---

## Task 8: Suspect periods — creation, listing and operator resolution (§3.2, §4.4)

**Files:**
- Create: `internal/service/consumption/{anomalies,resolve}.go`
- Test: `internal/service/consumption/anomalies_integration_test.go`

**Interfaces:**
- Consumes: Task 7's `Service`, `BillingDeps`; `store.AnomalyRepository`, `store.OpsRepository`, `store.ReadingRepository`.
- Produces:

```go
// BillingAndRecord runs Billing and, for every suspect register it finds,
// writes one consumption_anomalies row and one operator message per period
// (R59, R60). It is the only method that writes. Re-running it for the same
// period does not create a second row for the same (analyzer, period, reason).
func (s *Service) BillingAndRecord(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error)

type AnomalyListRequest struct {
	AnalyzerIDs []uuid.UUID
	Unresolved  bool
	Range       *store.TimeRange
	Page        store.Page
}

func (s *Service) Anomalies(ctx context.Context, sc store.Scope, req AnomalyListRequest) ([]model.ConsumptionAnomaly, error)

// ResolutionMode is 04 §4.4's closed set.
type ResolutionMode string

const (
	// ResolveByRegisteringReset writes a kind=reset reading so later derivations
	// apply §3.2's formula instead of reporting the period suspect.
	ResolveByRegisteringReset ResolutionMode = "reset_registered"
	// ResolveByOverride stores operator-supplied values in override_values.
	ResolveByOverride ResolutionMode = "manual_override"
	// ResolveByAccepting records that the operator accepts the NULL. The period
	// stays unbillable-by-value but no longer blocks the invoice.
	ResolveByAccepting ResolutionMode = "accepted"
)

type Resolution struct {
	Mode       ResolutionMode
	ResetTS    *time.Time                          // required for ResolveByRegisteringReset
	ResetAfter map[energy.Register]decimal.Decimal // the meter's values after the reset
	Overrides  map[energy.Register]decimal.Decimal // required for ResolveByOverride
}

func (s *Service) ResolveAnomaly(ctx context.Context, sc store.Scope, id, resolvedBy uuid.UUID, r Resolution) (model.ConsumptionAnomaly, error)
```

**Rules the implementation must follow:**
- `detail` is exactly R60's shape; decimals are strings; the operator message uses code `consumption.suspect_period` at level `error`.
- Dedup: before creating, list unresolved anomalies for `(analyzer, period_start)` and skip when one with the same reason exists. Two concurrent runs must not create two rows — rely on the list-then-create being inside one transaction-free idempotent check plus the partial index, and assert the behaviour with a concurrent test.
- `ResolveByRegisteringReset` writes the reset reading through `Readings.BulkInsert` with `Kind: reset` **and** marks the anomaly resolved; if the insert fails, the anomaly is not marked resolved.
- `ResolveByOverride` validates that every supplied register is one of the anomaly's suspect registers; an unknown register is `ErrValidation`.
- Resolution is scoped: resolving another tenant's anomaly is `ErrNotFound`, never a permission error that reveals existence.

- [ ] **Step 1: Write the failing tests**

```go
func TestASuspectPeriodWritesAnAnomalyAndAnOperatorMessage(t *testing.T) {
	rows, err := svc.BillingAndRecord(ctx, ten.Scope, req)
	require.NoError(t, err)
	require.Nil(t, rows[0].Values[energy.ActiveImport], "never a number")

	an, _ := anomalies.List(ctx, ten.AdminScope, store.AnomalyFilter{AnalyzerIDs: ids, Unresolved: true})
	require.Len(t, an, 1)
	require.Equal(t, "negative_delta", an[0].Reason)
	require.JSONEq(t, `{"code":"consumption.suspect_period","registers":["active_import"],...}`, string(an[0].Detail))

	msgs, _ := ops.ListMessages(ctx, ten.AdminScope, /* … */)
	require.Equal(t, "consumption.suspect_period", msgs[0].Code)
	require.Equal(t, model.MessageLevelError, msgs[0].Level)
}

func TestRerunningTheSamePeriodDoesNotCreateASecondAnomaly(t *testing.T) { /* call twice, still 1 */ }

func TestRegisteringAResetMakesTheNextDerivationSound(t *testing.T) {
	// Resolve with the reset the meter actually had; re-run Billing; the period
	// now derives a number and is no longer suspect.
	require.Equal(t, "140", after[0].Values[energy.ActiveImport].String())
	require.Empty(t, after[0].Suspect)
}

func TestAnOverrideMustNameASuspectRegister(t *testing.T)       { /* ErrValidation */ }
func TestResolvingAnotherTenantsAnomalyIsNotFound(t *testing.T) { /* AdminScope of tenant B + positive control */ }
func TestAcceptedLeavesTheValueNullButClearsTheBlock(t *testing.T) { /* resolved_at set, values still nil */ }
```

- [ ] **Step 2: Run and watch them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run and watch them pass.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestASuspectPeriodWritesAnAnomalyAndAnOperatorMessage` | write the anomaly but skip the operator message: expect FAIL. Restore. |
| `TestRerunningTheSamePeriodDoesNotCreateASecondAnomaly` | remove the dedup check: expect FAIL with 2 rows. Restore. |
| `TestRegisteringAResetMakesTheNextDerivationSound` | mark the anomaly resolved without inserting the reset reading: expect FAIL — the re-run is still suspect. Restore. |
| `TestResolvingAnotherTenantsAnomalyIsNotFound` | pass `store.SystemScope(other.CompanyID)` inside `ResolveAnomaly`: expect FAIL — the resolve succeeds. Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
go test ./internal/service/... -tags=integration -race -count=1
git add internal/service/consumption
git commit -m "feat(f3): task-8 — suspect period creation, listing and operator resolution" <TRAILERS>
```

---

## Task 9: Summary, generation, energy balance and the export shapes (05 §5, R77, R78)

**Files:**
- Create: `internal/service/consumption/{summary,generation,balance,export}.go`
- Test: one `_test.go` per file plus `export_golden_test.go`

**Interfaces:**
- Consumes: Task 7's `Service`, `Row`, `SeriesRequest`.
- Produces:

```go
// Summary totals a series (05 §5 /consumption/summary). Every field is nil
// when no row contributed one; a total is never a zero standing in for "no data".
type Summary struct {
	Totals      map[energy.Register]*decimal.Decimal
	Averages    map[energy.Register]*decimal.Decimal
	Peak        *Row // the row with the highest active-import consumption
	Valley      *Row // the lowest non-nil one
	MaxDemandKw *decimal.Decimal
	Rows        int
	Suspect     int // rows carrying at least one suspect register
}

func (s *Service) Summary(ctx context.Context, sc store.Scope, req SeriesRequest, path Path) (Summary, error)

// Path selects which consumption path a helper reads through, so a caller can
// never accidentally summarise billing numbers from the aggregates.
type Path string

const (
	PathAnalytics Path = "analytics"
	PathBilling   Path = "billing"
)

// Generation is the export-register series (05 §5 /generation), the same shape
// as a consumption series (R78).
func (s *Service) Generation(ctx context.Context, sc store.Scope, req SeriesRequest, path Path) ([]Row, error)

// Balance is 05 §5 /energy-balance, derived from registers only. Inverter-based
// self-consumption is F9 and is not approximated here (R78).
type Balance struct {
	Window      energy.Window
	Consumption *decimal.Decimal // active_import
	Generation  *decimal.Decimal // active_export
	GridImport  *decimal.Decimal
	GridExport  *decimal.Decimal
}

func (s *Service) EnergyBalance(ctx context.Context, sc store.Scope, req SeriesRequest, path Path) ([]Balance, error)

// ExportRows renders a series as ordered columns and string cells for
// /consumption/export. F3 renders no file: the CSV/XLSX encoder is F6/F8 (R77).
type Export struct {
	Columns []string
	Rows    [][]string // decimals as unrounded strings; an unavailable value is ""
}

func (s *Service) ExportRows(rows []Row) Export
```

- [ ] **Step 1: Write the failing tests**

```go
func TestSummaryLeavesATotalNilWhenNoRowReportedTheRegister(t *testing.T)
func TestSummaryCountsSuspectRowsAndExcludesThemFromTotals(t *testing.T)
func TestPeakAndValleyIgnoreSuspectRows(t *testing.T)
func TestGenerationReadsTheExportRegisters(t *testing.T)
func TestEnergyBalanceDerivesFromRegistersOnly(t *testing.T)
func TestExportColumnsAreStableAndUnrounded(t *testing.T) {
	e := svc.ExportRows(rows)
	require.Equal(t, []string{"analyzer_id", "period_start", "period_end", "active_import", /* … */}, e.Columns)
	require.Equal(t, "12.25", e.Rows[0][3])
	require.Equal(t, "", e.Rows[1][3], "an unavailable value is empty, never 0")
}
```

- [ ] **Step 2-4: Red, implement, green.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestSummaryCountsSuspectRowsAndExcludesThemFromTotals` | add suspect rows' nil values as zero: expect FAIL on the count. Restore. |
| `TestExportColumnsAreStableAndUnrounded` | render an unavailable value as `0`: expect FAIL. Restore. |
| `TestExportColumnsAreStableAndUnrounded` | round to 3 decimals: expect FAIL (R79). Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
git add internal/service/consumption
git commit -m "feat(f3): task-9 — summary, generation, energy balance and export shapes" <TRAILERS>
```

---

## Task 10: `internal/service/loadprofile` — resolving the company's calendar (R69, R70)

**Files:**
- Create: `internal/service/loadprofile/{doc,service}.go`
- Test: `internal/service/loadprofile/service_integration_test.go`

**Interfaces:**
- Consumes: Task 5's `loadprofile` domain package; Task 7's `consumption.Service` (the analytics path, for hourly values); `store.CalendarRepository`.
- Produces:

```go
package loadprofile

// DefaultWeekendDays is applied when a company has configured no
// company_weekend_days rows (R69; 01-project-context.md's stated default).
var DefaultWeekendDays = map[time.Weekday]bool{time.Saturday: true, time.Sunday: true}

type Deps struct {
	Calendar    store.CalendarRepository
	Hourly      HourlySource // the analytics path, narrowed
	Location    *time.Location
	Log         *slog.Logger
}

// HourlySource is the narrow interface over consumption.Service.Analytics that
// this package needs, so the service layer does not import a sibling wholesale.
type HourlySource interface {
	Analytics(ctx context.Context, sc store.Scope, req consumption.SeriesRequest) ([]consumption.Row, error)
}

type Request struct {
	AnalyzerIDs []uuid.UUID
	Range       store.TimeRange
	Keys        []domainlp.Key // empty means every profile
}

type Result struct {
	Profiles   map[domainlp.Key]domainlp.Profile
	Statistics map[domainlp.Key]domainlp.Statistics
	Config     ConfigUsed // what the service resolved, so a caller can show it
}

type ConfigUsed struct {
	WeekendDays   []time.Weekday
	WeekendSource string // "company" or "default"
	Vacations     int
}

func New(d Deps) (*Service, error)
func (s *Service) Profiles(ctx context.Context, sc store.Scope, req Request) (Result, error)
```

**Rules the implementation must follow:**
- Resolve the configuration first: `Calendar.WeekendDays` (falling back to `DefaultWeekendDays` and recording `WeekendSource: "default"`), then `Calendar.Vacations` over the request range widened to whole days.
- Feed the domain package hourly **active-import** values only (R68), taken from the analytics path (load profiles are analysis, not billing).
- A row whose active-import consumption is nil contributes nothing; it is not a zero hour.
- `calendar_events` are never read (R70).

- [ ] **Step 1: Write the failing tests**

```go
// 09 §F3 acceptance: a configured vacation moves a date from weekday to weekend.
func TestAVacationMovesADayIntoTheWeekendProfile(t *testing.T) {
	before, _ := svc.Profiles(ctx, ten.Scope, req)
	require.Equal(t, 5, before.Profiles["weekday"].Days)

	_, err := calendar.CreateVacation(ctx, ten.Scope, model.CompanyVacation{StartDate: wed, EndDate: wed})
	require.NoError(t, err)

	after, _ := svc.Profiles(ctx, ten.Scope, req)
	require.Equal(t, 4, after.Profiles["weekday"].Days)
	require.Equal(t, 3, after.Profiles["weekend"].Days)
	require.NotEqual(t, before.Statistics["weekday"].Mean.String(), after.Statistics["weekday"].Mean.String())
}

func TestACompanyWithNoConfiguredWeekendDaysGetsSaturdayAndSunday(t *testing.T) {
	res, _ := svc.Profiles(ctx, ten.Scope, req)
	require.Equal(t, "default", res.Config.WeekendSource)
	require.Positive(t, res.Profiles["weekend"].Days)
}

func TestACalendarEventDoesNotChangeTheSplit(t *testing.T) { /* R70 */ }
func TestProfilesAreTenantIsolated(t *testing.T)           { /* other tenant's AdminScope + positive control */ }
func TestStatisticsMatchTheHandComputedFixture(t *testing.T) { /* 09 §F3 acceptance */ }
```

- [ ] **Step 2-4: Red, implement, green.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestAVacationMovesADayIntoTheWeekendProfile` | ignore vacations when building the config: expect FAIL — the counts do not move. Restore. |
| `TestACompanyWithNoConfiguredWeekendDaysGetsSaturdayAndSunday` | return an empty weekend set instead of the default: expect FAIL — no weekend profile. Restore. |
| `TestACalendarEventDoesNotChangeTheSplit` | fold calendar events into the vacation list: expect FAIL. Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
go test ./internal/service/... -tags=integration -race -count=1
git add internal/service/loadprofile
git commit -m "feat(f3): task-10 — load profile service over the company calendar" <TRAILERS>
```

---

## Task 11: `consumption.refresh` — the task, the handler, the enqueue seam (R71, R73)

**Files:**
- Create: `internal/job/consumption.go`, `internal/service/consumption/refresh.go`
- Modify: `internal/job/task.go`, `internal/ingest/fetch.go`, `internal/ingest/deps.go` (comment only), `internal/worker/wiring.go`, `internal/platform/config` (two knobs)
- Test: `internal/job/consumption_test.go`, `internal/service/consumption/refresh_integration_test.go`, `internal/ingest/fetch_refresh_test.go`, `internal/worker/wiring_test.go`

**Interfaces:**
- Consumes: Task 6's `store.AdminAggregateRepository` and `store.ConsumptionViews()`; F2's `job.ConsumptionRefreshPayload`, `job.Handlers`, `job.ClassifyForRetry`, `ingest.Deps.ConsumptionRefresh`, `platform/lock`.
- Produces:

```go
// internal/job

// NewConsumptionRefreshTask builds the task. The task id is deterministic in
// the expanded window (R71) so a burst of analyzers ingesting the same window
// collapses to one refresh: asynq drops a duplicate id while the first is
// pending. AnalyzerID is carried for the journal only and is NOT part of the id.
func NewConsumptionRefreshTask(p ConsumptionRefreshPayload) (*asynq.Task, error)
func DecodeConsumptionRefresh(t *asynq.Task) (ConsumptionRefreshPayload, error)
func ConsumptionRefreshTaskID(companyID uuid.UUID, from, to time.Time) string

// Refresher is the handler seam. internal/job still does not import the store.
type Refresher interface {
	RefreshConsumption(ctx context.Context, p ConsumptionRefreshPayload) error
}

// Handlers gains:
//   ConsumptionRefresh Refresher
// and Register gains the guarded line:
//   if h.ConsumptionRefresh != nil { mux.HandleFunc(TypeConsumptionRefresh, h.integHandleConsumptionRefresh) }

// internal/service/consumption

type RefreshDeps struct {
	Aggregates store.AdminAggregateRepository
	Ops        store.OpsRepository
	Locker     lock.Locker
	Clock      clock.Clock
	Log        *slog.Logger
}

type Refresher struct { /* unexported */ }

func NewRefresher(d RefreshDeps) (*Refresher, error)

// RefreshConsumption expands [From, To) to whole buckets per level and
// refreshes the four consumption views finest-first, holding a per-view lock so
// two jobs never refresh the same view at once (R71). It runs outside any
// transaction (R72). A failure on one view aborts the rest and is retryable.
func (r *Refresher) RefreshConsumption(ctx context.Context, p job.ConsumptionRefreshPayload) error
```

**Rules the implementation must follow:**
- Window expansion uses `energy.Bucket` per level in `Europe/Istanbul`: the refreshed range is `[Bucket(level, From).From, Bucket(level, To-1ns).To)`. A yearly refresh therefore covers whole years — this is correct and is why the finest-first order matters.
- The lock key is `consumption.refresh:<view>`; the lease is renewed for long refreshes and released on every path.
- `ingest.Service.FetchReadings` enqueues **once per run**, after `finishRun`, when `deps.ConsumptionRefresh != nil`, the run persisted rows and the accumulator's affected range is non-nil. An empty range never enqueues (R73). An enqueue failure is logged and written as a warning message; it never fails the fetch run.
- New config knobs: `EKOKOD_CONSUMPTION_REFRESH_ENABLED` (default `true`) and `EKOKOD_CONSUMPTION_REFRESH_LOCK_TTL` (default `10m`). Both documented in the env docs alongside F2's `EKOKOD_INGEST_*`.
- `worker/wiring.go`: `ingestDeps.ConsumptionRefresh` becomes a small adapter over `*job.Client`, and `job.Handlers` gains the refresher. The existing wiring test that pins the graph is extended, not replaced.

- [ ] **Step 1: Write the failing tests**

```go
func TestConsumptionRefreshTaskIDIsDeterministicInTheWindowOnly(t *testing.T) {
	a := job.ConsumptionRefreshTaskID(company, from, to)
	require.Equal(t, a, job.ConsumptionRefreshTaskID(company, from, to))
	require.NotEqual(t, a, job.ConsumptionRefreshTaskID(company, from, to.Add(time.Hour)))
	// the analyzer is not part of the id: two analyzers, one refresh
	t1, _ := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{CompanyID: company, AnalyzerID: a1, From: from, To: to})
	t2, _ := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{CompanyID: company, AnalyzerID: a2, From: from, To: to})
	require.Equal(t, taskIDOf(t1), taskIDOf(t2))
}

// R73, the phase's operational promise made true.
func TestABackfillTwoHundredDaysOldEntersTheAggregateAfterTheJobRuns(t *testing.T) {
	// persist readings 200 days back through the real ingest pipeline,
	// assert the aggregate is empty, run the handler, assert the bucket exists.
}

func TestFetchEnqueuesExactlyOneRefreshForTheAffectedRange(t *testing.T) {
	require.Len(t, enqueuer.calls, 1)
	require.Equal(t, affectedFrom, enqueuer.calls[0].From)
	require.Equal(t, affectedTo, enqueuer.calls[0].To)
}

func TestFetchDoesNotEnqueueWhenNothingWasPersisted(t *testing.T) { require.Empty(t, enqueuer.calls) }

func TestRefreshHoldsAPerViewLock(t *testing.T) {
	// Two concurrent RefreshConsumption calls on the same view: the second
	// observes the lock. Uses a deterministic signal, never a sleep.
}

func TestRefreshRefreshesTheFourViewsFinestFirst(t *testing.T)     { /* recorded call order */ }
func TestRefreshExpandsTheRangeToWholeBuckets(t *testing.T)         { /* a 10-minute range refreshes the whole hour/day/month/year */ }
func TestWorkerRegistersTheConsumptionRefreshHandler(t *testing.T) { /* graph test: removing the wiring line fails it */ }
func TestAnEnqueueFailureDoesNotFailTheFetchRun(t *testing.T)      { /* warning message written, run still success */ }
```

- [ ] **Step 2-4: Red, implement, green.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestABackfillTwoHundredDaysOldEntersTheAggregateAfterTheJobRuns` | make the handler refresh only `[now-30d, now)`: expect FAIL — the old bucket never appears. Restore. |
| `TestConsumptionRefreshTaskIDIsDeterministicInTheWindowOnly` | put `AnalyzerID` into the id: expect FAIL — two analyzers produce two refreshes. Restore. |
| `TestFetchEnqueuesExactlyOneRefreshForTheAffectedRange` | enqueue the requested window instead of the affected range: expect FAIL. Restore. |
| `TestRefreshHoldsAPerViewLock` | drop the lock acquisition: expect FAIL from the concurrency assertion. Restore. |
| `TestWorkerRegistersTheConsumptionRefreshHandler` | revert `ConsumptionRefresh` to `nil` in `wiring.go`: expect FAIL. Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build && make check-generate
go test ./internal/job/... ./internal/ingest/... ./internal/worker/... ./internal/service/... -tags=integration -race -count=1
git add internal/job internal/ingest internal/worker internal/service internal/platform docs
git commit -m "feat(f3): task-11 — consumption.refresh task, handler, enqueue seam and worker wiring" <TRAILERS>
```

---

## Task 12: The PM5340 recompute becomes a bounded queued task (F2 carry-forward, R74)

**Files:**
- Create: `internal/job/generation.go`
- Modify: `internal/ingest/generation/*` (the post-persist hook), `internal/job/task.go`, `internal/worker/wiring.go`, `internal/platform/config`
- Test: `internal/ingest/generation/recompute_integration_test.go`, `internal/job/generation_test.go`

**Interfaces:**
- Consumes: F2's `ingest/generation` accumulate/anchor logic and its per-analyzer lock; Task 11's handler-registration pattern.
- Produces:

```go
// internal/job
const TypeGenerationRecompute = "generation.recompute"

type GenerationRecomputePayload struct {
	CompanyID  uuid.UUID
	AnalyzerID uuid.UUID
	From       time.Time // recompute forward from this instant
}

type GenerationRecomputer interface {
	RecomputeGeneration(ctx context.Context, p GenerationRecomputePayload) error
}
```

**Rules the implementation must follow (R74):**
- The post-persist hook counts the rows after the affected anchor. Below `EKOKOD_GENERATION_RECOMPUTE_INLINE_MAX` (default `5000`) behaviour is unchanged — the hook recomputes inline, exactly as F2 proved it correct.
- At or above the threshold the hook enqueues `generation.recompute` and returns; the ingestion run does not block.
- The handler recomputes in batches of the same size under the existing per-analyzer lock, committing each batch, then re-enqueues a continuation from the last processed instant until it reaches the tip. A crash mid-way leaves a consistent prefix; re-running is idempotent.
- The cumulative values produced by the queued path are **identical** to the inline path — this is the test that matters.

- [ ] **Step 1: Write the failing tests**

```go
func TestTheQueuedRecomputeProducesTheSameValuesAsTheInlinePath(t *testing.T) {
	// Same fixture twice: once with the threshold high (inline), once low (queued).
	require.Equal(t, inlineValues, queuedValues)
}

func TestALargeBackfillEnqueuesInsteadOfRecomputingInline(t *testing.T) {
	require.Len(t, enqueuer.calls, 1)
	require.Less(t, hookDuration, 2*time.Second, "the hook returns promptly")
}

func TestASmallBackfillStillRecomputesInline(t *testing.T) { require.Empty(t, enqueuer.calls) }
func TestAContinuationResumesFromTheLastProcessedInstant(t *testing.T)
func TestTheRecomputeHandlerHoldsThePerAnalyzerLock(t *testing.T)
```

- [ ] **Step 2-4: Red, implement, green.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestTheQueuedRecomputeProducesTheSameValuesAsTheInlinePath` | drop the carry between batches: expect FAIL — the values diverge at the first batch boundary. Restore. |
| `TestALargeBackfillEnqueuesInsteadOfRecomputingInline` | compare with `>` instead of `>=` at the threshold, using a fixture exactly at it: expect FAIL. Restore. |
| `TestTheRecomputeHandlerHoldsThePerAnalyzerLock` | drop the lock: expect FAIL (R52's concurrency proof, re-run on the queued path). Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
go test ./internal/ingest/... ./internal/job/... -tags=integration -race -count=1
git add internal/ingest internal/job internal/worker internal/platform docs
git commit -m "feat(f3): task-12 — bounded queued PM5340 generation recompute" <TRAILERS>
```

---

## Task 13: Acceptance suite, the F3 verification block and the handoff

**Files:**
- Create: `internal/acceptance/f3_consumption_test.go`, `internal/acceptance/f3_loadprofile_test.go`
- Modify: `HANDOFF_NEXT_SESSION.md`, `docs/rewrite/` (spec-defect notes only, if the controller approves)

**Interfaces:**
- Consumes: every prior task, through the real worker, a real Redis and a real TimescaleDB.

**Rules the implementation must follow:**
- One test per acceptance criterion in `09-implementation-plan.md` §F3, named after it, with a comment quoting the criterion.
- The suite drives the real wiring (`worker.Build`), never a hand-assembled graph, so a wiring regression fails here.
- The task report records the verbatim output of the phase verification block:
  ```bash
  go test ./internal/domain/energy/... -v
  go test ./internal/domain/loadprofile/... -v
  go test ./internal/service/consumption/... -tags=integration -v
  go test ./internal/domain/... -run TestGolden -v
  ```

- [ ] **Step 1: Write the acceptance tests** — one per criterion:

| §F3 criterion | Test |
|---|---|
| Golden-file tests for six derivation cases | `TestF3GoldenCorpusCoversTheSixCases` (asserts all six files exist and run) |
| Negative delta without a reset → NULL + suspect + message | `TestF3NegativeDeltaProducesNullSuspectAndMessage` |
| Each level derives from its own boundaries | `TestF3LevelsDeriveFromTheirOwnBoundaries` |
| billing vs analytics differ by the expected step | `TestF3BillingAndAnalyticsDifferByTheBoundaryStep` |
| Monthly prefers billing-kind readings | `TestF3MonthlyPrefersBillingReadings` |
| Load profile statistics match hand-computed values | `TestF3LoadProfileStatisticsMatchHandComputedValues` |
| A vacation moves a date weekday → weekend | `TestF3VacationMovesADayToTheWeekendProfile` |
| Zero consumption → null ratio | `TestF3ZeroConsumptionProducesANullRatio` |

- [ ] **Step 2-4: Red, implement, green.**
- [ ] **Step 5: Run the full suite twice** (`-count=1`, then again) and record both results; a package failing with `wait until ready` is retried once before being believed.
- [ ] **Step 6: Rewrite `HANDOFF_NEXT_SESSION.md` for F4** — state, rulings R54-R86, what F4 must pick up (max demand now real, R85's invoice consequence; the five §11 product-owner questions that block F4; unresolved anomalies blocking invoices), the environment section, and a paste-ready prompt. Commit.

```bash
make lint && make test && make build && make check-generate
go test ./... -tags=integration -race -count=1 -parallel 8
git add internal/acceptance HANDOFF_NEXT_SESSION.md
git commit -m "test(f3): task-13 — F3 acceptance suite, verification block and F4 handoff" <TRAILERS>
```

---

## Carried forward — explicitly NOT F3

- **HTTP handlers, routing, DTOs, RBAC, the OpenAPI document and the generated TypeScript client** for §5's ten endpoints — F6 (R77). F3 leaves service methods with the exact shapes those handlers need.
- **CSV/XLSX rendering** of `/consumption/export` and `/load-profile/export` — F6/F8 consume `Export` (R77).
- **The demand-overrun charge, reactive penalties, tiered pricing and every invoice number** — F4. F3 supplies max demand (R65, R85) and the unresolved-anomaly signal F4 must refuse to bill over.
- **Binding the OAuth state nonce to the session cookie** (F2's R47) — needs an HTTP session, so F6.
- **Surfacing `integration.ErrConfig` as a configuration problem rather than an auth failure** (F2's R48) — needs the HTTP error mapper, so F6.
- **Inverter-based energy balance and self-consumption ratios** — F9 (R78).
- **The data-communication alarm's configurable threshold** — F7. F3 ships only §3.6's fixed 7-day activity status (R66).
- **F2's deferred minors** (`final-review-A-report.md`, `final-review-B-report.md`: warning `Detail` dropped by the pipeline, `NextCursor` doc inconsistency, EPİAŞ TGT re-login per process, OSOS login per fetch, EPİAŞ PTF chunk-seam overlap) — triage only when an F3 task opens the same file.

---

## Self-review

### Acceptance criteria → tests (09 §F3)

| Criterion | Proven by |
|---|---|
| Golden-file tests: normal, missing readings, identical boundaries, reset with event, reset without event, rollover | Task 4's six `testdata/golden/*.json` + `TestGolden`; Task 13's `TestF3GoldenCorpusCoversTheSixCases` |
| Negative delta without a reset → NULL, suspect, operator message, asserted | Task 2 `TestDeriveWithoutAResetEventIsSuspect`; Task 8 `TestASuspectPeriodWritesAnAnomalyAndAnOperatorMessage`; Task 13 `TestF3NegativeDeltaProducesNullSuspectAndMessage` |
| Every level derives from its own boundaries; summing below is asserted not to be how | Task 7 `TestDailyDoesNotEqualTheSumOfItsHoursWhenAReadingIsMissing`; Task 13 `TestF3LevelsDeriveFromTheirOwnBoundaries` |
| billing vs analytics differ at a bucket boundary by the expected step | Task 7 `TestBillingAndAnalyticsDifferByTheBucketBoundaryStep`; Task 13 `TestF3BillingAndAnalyticsDifferByTheBoundaryStep` |
| Monthly prefers `billing`-kind readings | Task 7 `TestMonthlyPrefersBillingKindReadingsWhenPresent` + the fallback test (R63); Task 13 `TestF3MonthlyPrefersBillingReadings` |
| Load profile statistics match hand-computed values | Task 5 `TestStatsMatchHandComputedValues`; Task 10 `TestStatisticsMatchTheHandComputedFixture`; Task 13 |
| Vacation days move a date weekday → weekend | Task 10 `TestAVacationMovesADayIntoTheWeekendProfile`; Task 13 |
| Zero consumption → null ratio, not a substituted 1 | Task 3 `TestRatioIsNilWhenConsumptionIsZero` (mutation: substitute 1, the literal legacy defect); Task 13 |

### Scope coverage (09 §F3 bullets)

`internal/domain/energy` §2-§3 → Tasks 1-4. The two explicit consumption paths → Task 7. `consumption.refresh` triggered by ingestion over the affected range → Task 11. Suspect-period creation, listing and operator resolution → Task 8. `internal/domain/loadprofile` §10.3 with the company's vacation configuration → Tasks 5 and 10. API endpoints from 05 §5 → the service methods backing all ten (Tasks 7, 8, 9, 10), with the HTTP layer deliberately deferred to F6 (R77).

### Guards added, each with its proving mutation

`TestServiceLayerImportBoundaries` (probe importing `internal/ingest`); the float guard over `./internal/service/...` (a `float64` field in a probe); `TestAnalyticsPathCannotReachTheHypertable` / `TestBillingPathCannotReachAnAggregate` (cross-wiring the deps); `TestRefreshMaterialisesABucketOlderThanThePolicyWindow` (no-op refresh); `TestRefreshRejectsAnUnknownViewBeforeTouchingTheDatabase` (`fmt.Sprintf` statement building); `TestRerunningTheSamePeriodDoesNotCreateASecondAnomaly` (dedup removed); `TestRegisteringAResetMakesTheNextDerivationSound` (resolve without writing the reset row); `TestConsumptionRefreshTaskIDIsDeterministicInTheWindowOnly` (analyzer in the id); `TestRefreshHoldsAPerViewLock` (lock dropped); `TestWorkerRegistersTheConsumptionRefreshHandler` (wiring reverted to nil); `TestTheQueuedRecomputeProducesTheSameValuesAsTheInlinePath` (carry dropped); `TestGolden` (one digit edited); `TestStatsMatchHandComputedValues` (n-1 divisor); `TestSeasonOfUsesMeteorologicalMonths` (legacy 21 March cutoff); `TestBillingIsolatesTenants` and `TestResolvingAnotherTenantsAnomalyIsNotFound` (scope removed, with positive controls).

### Type consistency (checked by name across tasks)

`energy.Register`, `energy.Reading`, `energy.Window`, `energy.Level`, `energy.Derivation`, `energy.Suspicion`, `energy.Reason` — produced by Task 1, consumed by 2, 3, 4, 7, 8, 9, 11. `energy.Derive` — produced by 2, consumed by 7, 8. `energy.Ratio`/`MaxDemand`/`ActivityStatus` — produced by 3, consumed by 7, 9. `loadprofile.Config`/`Profile`/`Statistics`/`Key` — produced by 5, consumed by 10, 13. `store.AggregateView`/`ConsumptionViews`/`AdminAggregateRepository` — produced by 6, consumed by 11. `consumption.Service`/`Row`/`SeriesRequest`/`Path` — produced by 7, consumed by 8, 9, 10, 13. `job.ConsumptionRefreshPayload` (F2) and `job.Refresher`/`NewConsumptionRefreshTask`/`ConsumptionRefreshTaskID` — produced by 11, consumed by the worker wiring and 13. `job.GenerationRecomputePayload`/`GenerationRecomputer` — produced by 12.

### Known risks, stated rather than hidden

1. **R54 overrides a spec sentence.** If the product owner says the ratio really must be `0`, one field type and the ratio fixtures change; nothing else moves. The conflict is recorded as Q1.
2. **R55's before/after reading rule is inferred**, not stated by the spec. It is the plan's most load-bearing inference; Task 2's fixtures make it explicit and a different ruling is a contained change to `reset.go` plus three golden files.
3. **R65 assumes F2's ARIL adapter writes max demand as `current_index` rows.** Task 7's implementer verifies this before implementing and stops if it is false. If it is false, F3 gains a provider branch.
4. **The refresh covers every tenant's buckets in the window** (R71/R72) — this is a property of TimescaleDB, not a design choice. Under heavy multi-tenant ingestion the refresh load is shared; the deterministic task id is what keeps it from multiplying.
5. **R85 changes money.** Once max demand is real, F4's demand-overrun charge stops being structurally zero. Nothing in F3 bills anything, so the risk is entirely in F4's court — but it must not be discovered there.
6. **`internal/service` is a new layer** the architecture doc does not list (R75). If the spec owner rejects the name, it is a directory rename while the tree is two packages.
