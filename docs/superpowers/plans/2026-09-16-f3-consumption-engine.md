# F3 — Consumption Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every consumption figure the product shows or bills is derived from meter registers by one auditable set of pure functions, a negative register difference can never become an invoice number, and the analytics path and the billing path are two different functions with two different guarantees.

**Architecture:** `internal/domain/energy` and `internal/domain/loadprofile` are dependency-free pure packages: registers in, derived values out, no I/O (the embedded tz database is data, not I/O), no clock, no timezone lookup beyond `time.Location` values handed to them. Above them, a new `internal/service` layer is introduced by this phase. `internal/service/consumption` owns the two consumption paths that `04-data-model.md` §4.3 demands be explicit in code — `Analytics.Consumption` reads F1's continuous aggregates through `store.AnalyticsRepository` and never touches `meter_readings`; `Billing.Consumption` reads `meter_readings` at true period boundaries through `store.ReadingRepository.BoundaryReadings` and never touches an aggregate. There is no shared `Service` struct: `consumption.Analytics` and `consumption.Billing` are two separate types, each constructed alone (R61). The same package owns suspect periods: it writes `consumption_anomalies` rows and operator messages when a period is indeterminate, and it exposes listing and operator resolution. `internal/service/loadprofile` resolves a company's weekend and vacation configuration and feeds the pure profile functions. `consumption.refresh` — declared by F2 and left unhandled — gets a handler that calls `refresh_continuous_aggregate` through a new platform-scoped admin repository, and the F2 ingestion pipeline's affected range is finally wired to enqueue it.

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
- **Energy, power, ratios and multipliers are `decimal.Decimal`, never `float64`.** JSON and SQL decode through `pgnum`/`decimal`, never through `float64` or `any`. `internal/arch`'s `floatGuardPatterns` and `.golangci.yml`'s `no-float-money` tree list already cover `./internal/domain/...` recursively (energy and loadprofile included); Task 1 adds `./internal/service/...` to both — change both or neither. (`internal/service/...` is never added to `domain-is-pure`: that guard's allowed-imports contract does not fit a layer that legitimately imports `internal/store`, `internal/job` and `internal/platform`.)
- **Division is carried to `DivisionScale` (20) places; nothing else rounds** (02 §1). `energy.DivisionScale` and `loadprofile.DivisionScale` are both `int32 = 20`; every `DivRound`/`PowWithPrecision` call in the domain layer uses one of them. Presentation rounding happens once, when a value is written to an invoice line or displayed — neither of which happens in F3. No `Round`, `Truncate` or `StringFixed` inside `internal/domain/energy` or `internal/domain/loadprofile`.
- **An unavailable quantity is `nil`, never zero** (removed-behaviour 21, and 02 §6.5 for max demand; also `10-removed-behaviours.md` item 15 for a null ratio or per-capita metric on a zero denominator). A missing register, an indeterminate consumption and an undefined ratio are all `nil`. Zero means "measured zero".
- **Timestamps are stored UTC and evaluated in `Europe/Istanbul`.** Bucket boundaries are computed through the tz database, never a fixed `+03:00` offset (02 §1: pre-2016 data crosses real DST transitions). Packages that need the zone import `time/tzdata`.
- **The meter multiplier was applied once, at ingestion** (02 §2.2). Nothing in F3 multiplies again. Task 7's `internal/service/consumption/convert.go` round-trip test (`MultiplierApplied = 10` leaves register values unchanged) is this test; `meter_readings.multiplier_applied` is read for audit and never re-applied.

**Correctness guards**
- **Every guard is proven to fail.** A guard seen only to pass is not evidence (F1/F2 standing ruling). Each guard step names the mutation that must turn it red, and the task report records the observed failure output.
- **The negative-difference rule is the reason this phase exists** (`10-removed-behaviours.md` item 1: the legacy system returned the end register value itself as the consumption). Every task that touches derivation carries a test proving a negative delta without a reset produces `nil` consumption — never a number. Only Tasks 8 and 13 actually write a suspect period and an operator message (M-10): the other derivation tasks (1, 2, 3, 7) prove `nil` and `Suspicion`, since nothing below Task 8 has an `OpsRepository` to write to.
- **The two consumption paths are proven to be different functions.** A guard test asserts `Analytics.Consumption` never reaches `meter_readings` and `Billing.Consumption` never reaches a `consumption_*` view, and an integration test shows the two differ at a bucket boundary by exactly the missing step (09 §F3 acceptance).
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
- **Q4 — calendar events vs vacations — ESCALATED AS AN F6 BLOCKER.** F2's acceptance wording implies a calendar *event* changes weekday/weekend classification; `02-domain-rules.md` §10.3 names only weekend days and vacation periods. R70 rules it for F3 (`calendar_events` has no non-working flag, so F3 cannot honour the F2 wording even if it wanted to); the F2 wording should be corrected. F6 must not build a calendar-event-driven classification feature without a ruling from the product owner first — that is the blocker.
- **Q5 — `contracted_power_kw`.** Still unresolved from F2 (it lives on `tariffs`, not `analyzers`). F3 does not need it: max demand is reported, the overrun charge is F4's.
- **Q6 — the analytics hourly chart is all zeros for a 1-hour meter.** R87's hourly load-profile value (`ActiveImportStart(h+1) − ActiveImportStart(h)`) is what F3 feeds the load profile; but the *analytics* chart's own §4.3 aggregate definition (last-minus-first reading inside the bucket) is structurally zero for a meter that reports once per hour, and ~25% low for a 15-minute meter. F3 keeps §4.3's analytics semantics unchanged because acceptance criterion 4 requires the documented step-loss behaviour, but the defect is real and must be raised with the product owner / F6.
- **Q7 — the `load_profile` boundary fallback has no staleness bound.** R63's `billing`-kind boundary selection is bounded by `BillingSnapshotTolerance` (72 h): a `billing` snapshot older than that is treated as not covering the month and the period falls back to `load_profile`. The `load_profile` fallback itself has no equivalent bound — §3.1's "last reading with `ts <= bound`" selection will pick an arbitrarily old boundary reading if nothing more recent exists, with no freshness check. Raised for symmetry; not blocking, since no acceptance criterion asks for a look-back bound on `load_profile`.
- **Q8 — an uncovered register rollover becomes operator work.** R57 rules that a rollover without a covering `reset` row is `negative_delta`-suspect like any other negative delta: an operator must register a reset resolution even though the meter did not fail, it wrapped. Raise with the product owner: an uncovered wrap becomes operator work.
- **Q9 — a meter swap with no reset row.** §3.2 detects a reset only when a register goes DOWN. A replacement meter whose register starts above the old one's reading is indistinguishable from real consumption, and a property test showed it over-bills. F2's R13 sanity-jump rule rejects the most extreme jumps at ingestion; nothing in F3 can detect the rest. Raise with the product owner: should a meter-serial change (`meter_readings.meter_serial`) force a suspect period? (Not F3 scope — recorded for F4/F7.)
- **Q10 — daily-only meters in analytics.** The continuous aggregates read `kind = 'load_profile'` only, so an analyzer that reports only `daily` readings has no analytics series (charts, load profile) even though R95 gives it billing figures. Raise with the product owner and F6: either an aggregate over `daily` readings or an explicit "no interval data" state in the UI.

---

## Spec gaps and rulings

Ruling ids continue F2's sequence (F2 ended at R53) so a code comment citing an R-id is unambiguous across phases.

| Id | Spec passage | Ruling | Cost if wrong |
|---|---|---|---|
| R54 | 02 §3.3 "inductive_ratio = … (0 when active_import = 0)" vs 09 §F3 acceptance "A zero-consumption period produces a **null** ratio, not a ratio computed against a substituted 1"; also `10-removed-behaviours.md` item 15 (null for zero denominators of ratios and per-capita metrics) | Direct conflict; the acceptance criterion wins (F0/F1 standing ruling). **A ratio is `*decimal.Decimal` and is `nil` when active-import consumption is nil, zero or negative.** Never `0`, never computed against a substituted `1`. Raise Q1 with the product owner. | One field type + the ratio fixtures |
| R55 | 02 §3.2 "`(register_value_before_reset − reading_start) + (reading_end − register_value_after_reset)`"; 04 §4.1 gives a `reset`-kind row the same columns as any other reading, with no before/after pair | Spec silent on which rows supply the two values. **Ruling: `register_value_after_reset` is the `reset` row's own register value. `register_value_before_reset` is the register value of the last `load_profile`-kind reading (a "prior") with `ts < reset.ts`, supplied to `Derive` by the caller regardless of the window's own boundary kind (a billing-kind derivation still looks for `load_profile` priors, because those are the readings dense enough to have one). When no such prior reading exists, that register is suspect (`meter_reset`) for this reset — it is never assumed to be `reading_start`'s value, and the pre-reset segment is never silently contributed as zero.** A `reset` row whose own register value is `nil` for a register makes that register suspect (`meter_reset`), never zero. | The derivation formula + three golden fixtures |
| R56 | 02 §3.2 describes exactly one reset in a period | Spec silent on several. **Ruling: the period is split at every `reset` row inside `[start, end)` and consumption is the sum of the per-segment differences, computed with the same before/after rule (R55). Resets are processed in `ts` order; a segment with a negative difference of its own makes the register suspect.** | Loop shape + one fixture |
| R57 | 09 §F3 acceptance names "a rollover" as a case distinct from the two reset cases; no spec text defines a rollover | Spec silent. **Ruling: F3 never infers a register modulus and never reconstructs a wrap arithmetically.** A rollover is a negative difference like any other: with a covering `reset` row it takes R55's path (the row's before-value is near the modulus and its after-value is small), without one it is `negative_delta`-suspect. The "rollover" golden fixture is therefore a wrap **with** a reset row, and a second fixture proves an uncovered wrap yields `nil` + suspect, not a `10^n`-corrected number. Raise with the product owner: an uncovered wrap becomes operator work (Q8). | One fixture's framing |
| R58 | 02 §3.2 "consumption for the affected registers is recorded as `NULL`" | **Ruling: suspicion is per register.** A period whose `active_import` is suspect still reports a derived `t1_import` if that register is sound. The `Derivation` carries a per-register `Suspicion`, and the anomaly row's `detail` lists the affected register names. Any register being suspect makes the whole period unbillable — that check is F4's, keyed off the unresolved anomaly row, not off a period-level flag F3 invents. | `detail` shape + F4 reads one field |
| R59 | 04 §4.4 `reason ∈ 'meter_reset','negative_delta','missing_readings'` vs 02 §3.1 "the period yields no row — it is not emitted as zero" | **Ruling:** `negative_delta` = a negative difference with no covering reset. `meter_reset` = a reset row exists but cannot be applied (a nil before/after value for that register). `missing_readings` = written **only by the billing path**, when a caller asks for a specific billing period and a boundary reading is absent; the analytics path and range queries simply omit the bucket, exactly as §3.1 says. Ingestion's own reading-to-reading `negative_delta` anomalies (F2, `internal/ingest/anomaly.go`'s `DetectNegativeDeltas`, written in `internal/ingest/fetch.go:283-300`) are a different check on the same table, with a different `detail` shape — **F2: one row per affected register, `{"register":"active_import","kind":"load_profile"}`; F3: one row per suspect period, R60's shape** — and are never deduplicated against F3's period-level rows except by the `(analyzer, period_start, period_end, reason)` key (C-6). `ResolveAnomaly` (Task 8) also resolves any F2 ingestion `negative_delta` row for the same analyzer whose `[period_start, period_end]` lies within the resolved F3 period, applying the same resolution to both (I-5): an operator who registers a reset for a billing period should not also have to separately clear the ingestion-time row F2 wrote for the same event. | One branch + Q2 |
| R60 | 02 §3.2 "an operator message is written"; no format given | **Ruling: the operator message is one `store.OpsRepository.AppendMessage` call with `model.OperationalMessage{Kind: "system", Category: "consumption-suspect-period", Status: "error", Message: <one human-readable sentence>, RelatedType: ptr("consumption_anomaly"), RelatedID: &anomaly.ID, Metadata: <the same detail JSON as the anomaly row>}`.** `OperationalMessage` has no `Code` field and no level enum — `Kind`/`Category`/`Status` are the plain-text columns migration `00008` actually built, and `Status == "error"` is the assertion, not a `Code` or `Level` field the type does not have. `detail` (identical on the anomaly row's `detail` column and in the message's `Metadata`) is per-register: `{"code":"consumption.suspect_period","period_start":…,"period_end":…,"registers":{"active_import":{"reason":"negative_delta","delta":"-1234.5","reset_rows":0}}}` (M-12). Decimals are strings. No secret, no raw payload, no SQL. | `detail` shape |
| R61 | 04 §4.3 "They are different functions with different guarantees" | **Ruling: the separation is enforced structurally, not documented.** `internal/service/consumption` defines two independent types, each constructed alone with no shared `Service`: `consumption.Analytics` (`NewAnalytics(AnalyticsDeps) (*Analytics, error)`, method `Consumption(ctx, sc, req)`) receives only `store.AnalyticsRepository`; `consumption.Billing` (`NewBilling(BillingDeps) (*Billing, error)`, method `Consumption(ctx, sc, req)`) receives only `store.ReadingRepository` (+ anomalies/ops). A guard test constructs each type alone — there is no other-repository field to set to a poisoned fake, because the other repository is not a field on that type at all. No flag, no shared "mode" parameter. Task 8's write-and-list-and-resolve methods live on `*Billing`: `ConsumptionAndRecord`, `ListAnomalies`, `ResolveAnomaly`. | Two constructors |
| R62 | 02 §3.4 "the monthly row is derived from `billing`-kind readings when present"; migration `00005` filters every aggregate to `kind = 'load_profile'` | **Ruling: the preference is a billing-path rule only.** `Analytics.Consumption` at monthly granularity reports what `consumption_monthly` holds and records `Source: energy.KindLoadProfile` (M-3: `Row.Source` is `energy.Kind`, not a bare string). `Billing.Consumption` applies the preference and records `Source: energy.KindBilling` or `energy.KindLoadProfile` per period. No migration, no view change. | A field + the monthly test's framing |
| R63 | 02 §3.4 "when a metering point has `billing`-kind readings covering a month" | Spec silent on "covering". **Ruling: covered means each of the month's two `billing`-kind boundary readings (`BoundaryReadings(analyzer, 'billing', start, end)`) is non-nil AND lies within `BillingSnapshotTolerance` (72 h, `72 * time.Hour`) of its own bound — i.e. `bound − 72h <= reading.TS <= bound` for both the start and end boundary.** A `billing` reading that exists but is stale (an ARIL snapshot older than 72 h of the month boundary) does not count as covering; the month falls back to `load_profile` exactly as if no `billing` reading existed at all. A test covers a missing January snapshot (no `billing` reading near the boundary at all) alongside a stale-snapshot test. A partially covered month never mixes kinds within one period. Known risk (Q7): the `load_profile` fallback itself has no equivalent staleness bound — §3.1's boundary selection is unbounded look-back there, deliberately, since no acceptance criterion asks for one. | One predicate + one fixture |
| R64 | 04 §4.1 `reading_kind` has five values; 02 §2.3 documents four | **Ruling: `current_index` never participates in §3.1 differencing.** It is a diagnostics source only — and, re-ruled by C-3/R65, it is NOT a max-demand source either: `energy.MaxDemandKinds` excludes it (the misattribution risk R65 explains). The billing path's kind parameter is `load_profile`, `daily` or `billing`; `reset` rows are read separately as reset evidence; `current_index` is read by neither the boundary query nor the max-demand query in F3. | A `WHERE kind IN` list |
| R65 | 02 §3.5 "the maximum of the interval `max_demand` values within the period"; "for ARIL … taken from that endpoint's `MaxDemand` field, taking the maximum per month" | **Ruling: one provider-agnostic rule, re-ruled from an earlier draft (C-3).** Max demand is `MAX(max_demand_kw)` over readings in `[start, end)` of kinds `load_profile`, `daily` and `billing` only, via an exported allowlist `energy.MaxDemandKinds`. `current_index` is EXCLUDED: `internal/integration/aril/source.go`'s `fetchCurrentIndex` (~line 540-563) stamps a `current_index` reading at its own `ProfileDate`, which can carry the *previous* month's peak into the wrong period if read directly — exactly the misattribution F2's R49 already fixed for the persisted reading itself. `reset` rows are excluded too (evidence, not boundaries or demand sources). ARIL's authoritative monthly max instead arrives on `billing`-kind rows: `fetchBilling` (`internal/integration/aril/source.go:~682-733`) computes `maxByMonth` from the `current_endexes` `MaxDemand` field, keyed to the **Istanbul month of the row's own `ts`**, and writes it onto the `billing` reading it emits for that month — verified by reading the adapter, not assumed. Task 3's `MaxDemand` function and tests: `billing` rows are counted, `current_index` and `reset` rows are ignored. | A provider branch, if the premise is wrong |
| R66 | 02 §3.6 "active if its most recent reading is within the last 7 days"; `analyzers.is_active` exists and means something else | **Ruling: `energy.ActivityStatus(lastReadingAt *time.Time, now time.Time) Status` with a fixed 7×24 h constant, exported as `energy.ActivityWindow`.** It is independent of `analyzers.is_active` (operator enablement) and of the configurable data-communication alarm threshold (F7). A code comment states all three are different. `nil` last reading is `passive`. | A constant |
| R67 | 02 §10.3 "stddev" with no divisor | Spec silent. **Ruling: population standard deviation (divisor `n`, the 24 hourly means), because the 24 points are the whole curve, not a sample of it.** Stated in the doc comment and asserted by a hand-computed fixture. | One divisor + the profile fixtures |
| R68 | 02 §10.3 "the mean consumption across all matching days" — which register is unstated | Spec silent. **Ruling: the load profile averages active-import consumption only.** Generation profiles are not F3 (§10.1's rooftop metrics are F9). The domain function takes `[]HourValue` and never names a register, so a later phase can feed it export values without a change. | Nothing in the domain layer; one service call site |
| R69 | 02 §10.3 "the configured non-working weekdays"; no migration seeds any; 01 §context says the default is Saturday and Sunday | **Ruling: the pure classifier takes the weekend-day set as a parameter and applies exactly what it is given. The service resolves it: `CalendarRepository.WeekendDays` when the company has rows, otherwise the documented default `{Saturday, Sunday}`.** The default lives in the service, is named `loadprofile.DefaultWeekendDays`, and is asserted by a test that a company with no rows still splits Sat/Sun into `weekend`. | One default + one test |
| R70 | 02 §10.3 names weekend days and vacation periods; **the "calendar event changes the split" wording is `09-implementation-plan.md` §F6 line 434, not §F2** (corrected citation); `calendar_events` has no non-working flag | **Ruling: `calendar_events` never affects classification.** Only `company_weekend_days` and `company_vacations` do; this holds for F3 regardless, because `calendar_events` has no field to classify by. **Q4 is escalated as an F6 blocker**: §F6 line 434 is the acceptance wording that implies a calendar-event-driven split, and F6 must not build that without a product-owner ruling. The guard test's name and a leading comment mark it **provisional**: `TestACalendarEventDoesNotChangeTheSplitUnderR70` with the comment "revisit when F6 rules on Q4". | One test |
| R71 | 04 §4.3 refresh policies; `refresh_continuous_aggregate` takes a time range only and cannot be filtered by analyzer | **Ruling: the refresh job is per window, not per company and not per analyzer** (re-ruled from an earlier draft's "per company+window" framing, I-6) — `refresh_continuous_aggregate` covers every tenant's buckets in the range regardless of who triggered it (R72), so deduplicating across companies is correct, not a bug. The handler expands `[From, To)` to whole buckets per level, refreshes finest-first (`hourly → daily → monthly → yearly`), and serialises refreshes **per view** with `platform/lock`. **The task id is `"consumption.refresh:" + hourly-expanded From + "/" + To` (RFC3339 UTC) — no company, no analyzer** — built with `asynq.TaskID` **without** `asynq.Retention`, so the id deduplicates only while the task is pending/active/retrying; a completed refresh never blocks a later one for the same window (F2's R53 trap came from `Retention` keeping a completed task's id reserved). Cost, stated rather than hidden: an archived (retries-exhausted) refresh blocks that exact window until the archive entry is deleted. `NewConsumptionRefreshTask(p ConsumptionRefreshPayload, o TaskOptions)`'s copy source is `NewFetchReadingsTask`'s windowed branch (`internal/job/integration.go:206-221`), not `NewSyncPricesTask`. `AnalyzerID` stays in the payload for the journal only, never part of the id, so a burst of analyzers ingesting the same window collapses to one task. | A dedup key + one lock |
| R72 | 03 §2.2 layering; `TestEveryStoreMethodIsScoped` | **Ruling: the refresh is platform work and goes through `store.AdminAggregateRepository` implemented in `internal/store/postgres/admin`,** which the scope guard already exempts. It is never exposed on a tenant-scoped repository, because a refresh cannot be restricted to one tenant's rows and a scoped signature would be a lie. | One interface move |
| R73 | F2 wrote an `info` message promising that a pre-30-day backfill enters the aggregates "once `consumption.refresh` runs for it (F3)" | **Ruling: F3 makes that promise true, amended to only fire where it is needed (I-13).** `ingest.Service.FetchReadings` enqueues `consumption.refresh` only when the run persisted rows AND `acc.affectedFrom` is earlier than `now − 30 days` — `consumption_hourly`'s `start_offset`, the smallest of the four policy windows, so a range fully inside it is live data the aggregates already cover in real time and needs no refresh. An empty affected range, or one entirely inside the last 30 days, never enqueues. Tests cover both sides of the threshold. The window enqueued is the affected range, not the requested window, so a backfill of old data refreshes exactly the buckets it touched, below every policy watermark. An integration test backfills a 200-day-old window, asserts the aggregate is **absent**, runs the job, and asserts it is **present**. | The enqueue call site |
| R74 | F2 parked the PM5340 post-persist recompute: one hook call can rewrite ~35 k rows per analyzer-year | **Re-ruled — DEFERRED TO F9 (I-10). F3 does not change the PM5340 hook.** The inline recompute stays exactly as F2 built it: it already commits in `writeBatchSize = 5000`-row batches (`internal/ingest/generation/hook.go:37`), so a single call is already bounded per commit even though it can still process ~35k rows end to end. A queued recompute needs a pending-marker protocol so an inline hook running concurrently on the same analyzer does not read a stale base while the queued continuation is mid-flight — that protocol belongs with F9's solar generation work, not F3's consumption engine. Cost if this is wrong: a slow ingest task after an operator triggers a historical PM5340 backfill. **Task 12 is WITHDRAWN**; its heading stays with one line, "Withdrawn — R74 deferred to F9." | One knob + one task type, deferred |
| R75 | 03 §2.1's package tree has no `internal/service`; 09's §F3 verification runs `go test ./internal/service/consumption/...` | Spec inconsistency. **Ruling: the verification block wins — `internal/service/<area>` is the layer's home** (`consumption`, `loadprofile` in F3). The architecture doc's tree should gain the entry; raise it with the spec owner. | A directory rename while the tree is two packages |
| R76 | 03 §2.2 "api ──▶ service ──▶ domain"; `internal/arch` has no service guard because no service existed | **Ruling, split across two existing/new guards rather than one new test covering both directions:** Task 1 adds `TestServiceLayerImportBoundaries`, which checks only the **service → api/ingest** direction — `internal/service/...` may not import `internal/api/...` or `internal/ingest/...`. The **store/integration → service** direction is NOT a new test: Task 1 extends the existing `TestStoreAndIntegrationDoNotImportAPIOrService` (`internal/arch/arch_test.go:161`, which already walks `./internal/store/...` and `./internal/integration/...` for an `internal/api` import) to also fail on an import of `internal/service`, so the two trees this guard already loads are checked for both forbidden imports in one pass instead of a duplicate loader. Task 1 also extends `floatGuardPatterns` and `.golangci.yml`'s matching `no-float-money` lists with `./internal/service/...`. Change both or neither. | Guard edits |
| R77 | 05 §5 lists `/consumption/export` and `/load-profile/export` (CSV/XLSX) | **Ruling: F3 produces the rows and the stable column order; it renders no file.** Task 9's `ExportRows` covers `/consumption/export`. Task 10 defines `ExportMatrix` for `/load-profile/export`: `Columns []string` = `hour, <profile keys…>` (one column per requested profile key, in request order), `Rows [][]string` has exactly 24 rows (hour 0-23, R83), every cell an unrounded decimal string, and `""` for a `nil` hour. The CSV/XLSX encoder and the HTTP surface are F6/F8. No `excelize` import in F3. | One adapter in F8 |
| R78 | 05 §5 `/generation` "same shape as `/consumption`" and `/energy-balance` "generation, consumption, grid import and grid export for the range" | **Ruling:** generation series = the export registers through the same two paths and the same derivation; energy balance = `{consumption: active_import, generation: active_export, grid_import: active_import, grid_export: active_export}` derived from registers only. **`grid_import` is identically equal to `consumption` and `grid_export` is identically equal to `generation` until F9 adds inverter data** — there is no self-consumption model in F3 to make them differ, so the four fields are two distinct register reads presented under four names, by design, not an oversight. Plant-production-based balance (inverter data, self-consumption ratios) is F9 and is not approximated here; the balance endpoint is register-only by design. | One struct |
| R79 | 02 §1 "intermediate values are never rounded"; `pgnum` passes excess scale through unrounded | **Ruling: division is carried to `DivisionScale` (20) places; nothing else rounds (C-5).** `energy.DivisionScale int32 = 20` (Task 3, in `ratio.go`) and `loadprofile.DivisionScale int32 = 20` (Task 5) bound every `DivRound`/`PowWithPrecision` call; a subtraction, addition or comparison never rounds at all, because it never needs to. Storage rounding is Postgres's when a value is written to a `numeric(18,4)` column, and presentation rounding is F6's. A `Round`/`StringFixed` call inside `internal/domain/...` fails review, and a `DivRound`/`PowWithPrecision` call at any scale other than `DivisionScale` fails review too. | Nothing |
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
| R81 | `loadFactor = average / max` with no zero guard, rendering literal `NaN` (`load-profile/page.tsx:333`); 02 §10.3 says "0 when max = 0" | **Ruling: load factor follows §10.3 literally and is `decimal.Zero` when max is zero** — deliberately *unlike* R54's ratios. `10-removed-behaviours.md` item 15 (null for zero denominators of ratios and per-capita metrics) does not cover load factor: item 15 is about a ratio or a per-capita figure standing in for "no data" with a number, but §10.3 states 0-when-max-is-zero as the load profile's own definition, not a removed defect, and no acceptance criterion overrides it the way R54's does for the ratio. Both R54 and item 15 are cited in the code comment alongside R81 so the asymmetry is visibly intentional, not an inconsistency. | One branch |
| R82 | Seasons are fixed calendar dates (21 Mar / 21 Jun / 23 Sep / 21 Dec, `load-profile/page.tsx:452-485`); 02 §10.3 says meteorological (Dec-Feb, Mar-May, Jun-Aug, Sep-Nov) | **Ruling: the spec wins — meteorological month boundaries.** Recorded as a deliberate divergence: a legacy report regenerated under F3 moves days between seasonal profiles near the equinoxes. F14's comparison tooling must expect it. | Profile bucketing + an F14 expectation |
| R83 | The legacy hour axis is 1-24 (`hour 00` remapped to `24`, `page.tsx:406-409`) | **Ruling: hours are 0-23 everywhere in F3**, matching `time.Time.Hour()` and the aggregates. F6 may render a 1-24 axis; the data never carries hour 24. | One off-by-one in F6 |
| R84 | Legacy statistics are computed per hour over that hour's population across days; the spec's `hour_of_max` only makes sense over the 24-point curve | **Ruling: statistics are per profile over the 24 hourly means** (max, min, hour_of_max, mean, stddev, range, load factor), as 02 §10.3 and `01-project-context.md` describe. The legacy per-hour spread is not reproduced. | The statistics shape |
| R85 | `maxDemand` has no producer in legacy (`Consumption.ts:85` defaults 0; `arilService.processMaxDemand` is dead code), so `powerOveruseCost` is effectively always 0 (`bill/route.ts:2174-2181`) | **Ruling: F3 produces max demand for real (R65).** Consequence, recorded now rather than discovered in F14: once F4 uses it, invoices carry a demand-overrun charge where legacy billed zero. This is a correction, not a regression — raise it with the product owner before F4 bills anything. | A product-owner conversation, not code |
| R86 | Legacy classifies weekend as JS Saturday/Sunday and never reads `Company.vacations` (`page.tsx:414-416`; confirmed absent from every computation path) | **Ruling: F3 implements the spec's vacation-aware classification (R69, R70).** Load profiles will differ from legacy for any company that configured vacations. Deliberate. | An F14 expectation |

### New rulings from the fix-round-1 review

| Id | Spec passage | Ruling | Cost if wrong |
|---|---|---|---|
| R87 | 02 §10.3 feeds the load profile from hourly consumption; migration `00005`'s `consumption_hourly.active_consumption` is `last(active_import) − first(active_import)` inside the bucket (04 §4.3) | **New ruling (C-7). Hour h's load-profile value is `ActiveImportStart(h+1) − ActiveImportStart(h)` across two CONSECUTIVE `consumption_hourly` rows (each bucket's first reading, `model.ConsumptionBucket.ActiveImportStart`) — never that hour's own `active_consumption` column.** This equals §3.1's boundary differencing whenever the meter reports on the hour (true for both 15-minute and 1-hour meters: a 1-hour meter's single reading in the hour is both hour h's `ActiveImportStart` and hour h+1's boundary via the one-row look-ahead). It is `nil` when either bucket is missing or the difference is negative; **no anomaly is written from this analysis-side computation** (only the billing path writes anomalies, R58). Rationale: `active_consumption` is structurally **ZERO** for a 1-hour meter (its one reading is both `first()` and `last()`) and **~25% low** for a 15-minute meter (it misses the last quarter-hour's step into the next hour — the same "missing one step" effect the phase's own guard proves for billing vs analytics). **Q6** is raised because the *analytics* hourly chart itself still uses §4.3's `active_consumption` aggregate and is therefore all zeros for a 1-hour meter; F3 keeps that because acceptance criterion 4 requires the documented step-loss behaviour, but the defect must go to the product owner / F6. | The load profile silently reports zero for every 1-hour-metered building |
| R88 | Migration `00005`'s own comment: "month-to-date and year-to-date MUST be composed — the closed buckets from `consumption_monthly` or `consumption_yearly`, PLUS the open period taken from `consumption_daily` or from `meter_readings`"; `consumption_monthly`/`consumption_yearly` are `materialized_only = true` and never contain the still-open period | **New ruling (I-4). `Analytics.Consumption` at `Monthly`/`Yearly` composes the current open period from `consumption_daily` only — never `meter_readings`, which would break R61's structural separation.** Each register's open-period value = the closing index of the LAST `consumption_daily` bucket inside the open period, minus the closing index of the last `consumption_daily` bucket before the open period began; `nil` when either closing index is missing. The composed row sets `Row.Partial = true` so a caller can tell it apart from a closed, materialized row. Task 7 owns the composition and its test (a mid-month figure composed from daily buckets, checked against the true figure once the month closes and `consumption_monthly` materializes it). | Month-to-date/year-to-date reads as silently absent, or a caller is tempted to read `meter_readings` directly and breaks R61 |
| R89 | 05 §5 accepts `analyzer_id` or `building_id` as the series-request key | **New ruling. F3 services take analyzer ids only** — `SeriesRequest.AnalyzerIDs`, `AnomalyListRequest.AnalyzerIDs` and every other F3 request shape have no building field. Resolving "every analyzer under this building" is F6's job: it calls `store.AnalyzerRepository` to list a building's analyzers inside the caller's own `store.Scope`, then passes that slice of ids into the F3 service methods. F3 adds no building-aware overload and no `BuildingRepository` dependency anywhere in `internal/service/consumption` or `internal/service/loadprofile`. | One helper lands in F6 instead of F3 |
| R90 | 02 §3.2 is silent on a reset that shares its instant with a boundary reading (found by the Task 2 review: a reset at `end.TS` with a non-reset end reading carrying the OLD meter's value billed the old running total unflagged — 1110 instead of ~110) | **Ruling:** when an in-window reset row has `TS == end.TS` and `end` is not itself a reset row, `end` counts as post-reset evidence for a register only if its value equals the reset row's value (`decimal.Equal`); otherwise that register is `meter_reset`-suspect. A reset at `start.TS` lies outside the evidence window `(start.TS, end.TS]` and is never applied. At a shared instant the reading's side of the meter swap is unknowable, so the only safe rule is to flag. | An operator resolves a few extra suspect windows at reset instants |
| R91 | R55's `before_reset` lookup (Task 2 review: skipping a prior that lacks the register and using an older one silently dropped the usage in between — 65 billed instead of 85) | **Ruling:** `before_reset` is the value of the single LAST prior strictly after `start.TS` (or after the previous reset) and strictly before the reset. If that reading lacks the register, the register is `meter_reset`-suspect — never an older reading. `priors` and `resets` are REQUIRED pre-sorted ascending by `ts` (documented precondition; lookups use `sort.Search`; `Derive` has a no-reset fast path). Task 7's readers must therefore order by `ts` in SQL. When both reasons apply to one register, `meter_reset` wins. | More suspect windows on meters with sparse registers; a caller passing unsorted input gets wrong boundaries |
| R92 | Task 2 re-review (round 1): the start-side mirror of R90 still billed unflagged — a load_profile start of 1010 (old meter) and a reset row to 5000 at the same instant, end 5040, billed 4030 (true 40); a zero-width pair (start 1010 at T, end = the reset row at T) billed 3990; and R91's "`meter_reset` wins" held only at the end boundary | **Ruling (amends R90 and R91):** (1) **Same-instant boundaries never emit**: `start.TS.Equal(end.TS)` → not emitted, whatever the kinds (a zero-width pair measures nothing). (2) **Start side of R90**: when a reset row has `TS == start.TS` and `start` is not itself a reset row, `start` counts as post-reset evidence for a register only if its value equals the reset row's value; otherwise that register is `meter_reset`-suspect. (3) At either boundary the check uses the LAST reset row at that instant. (4) **`meter_reset` wins everywhere**: if any reset-evidence problem occurs for a register anywhere in the window, its reason is `meter_reset`, regardless of which segment first went negative. (5) `priors` must be `load_profile` readings and `resets` must be `reset` readings — a documented precondition; `Derive` ignores (skips) any element of the wrong kind rather than trusting it. | More suspect windows at reset instants; none billed wrong |
| R93 | Task 2 re-review round 2 — a 1M-scenario property test found I-16 over-bills: a reset row that omits a register while both boundaries report it lets a meter swap bill a plain difference (e.g. T1 billed 4610) | **Ruling (reverses I-16):** if a reset row inside the evidence window (or at either boundary instant, R90/R92) omits a register that both boundary readings report, that register is `meter_reset`-suspect — never a plain difference across the reset. Consequence for Task 8: `ResolveAnomaly` with `reset_registered` must supply an after-value for EVERY register the window's boundary readings report (validated → `ErrInvalidRequest`), so registering a reset never turns previously sound registers suspect. Also: a boundary reading of kind `reset` is always treated as reset evidence at its instant, whether or not the caller also passed it in `resets`. The Task 2 unusable-reset test's reason returns to `meter_reset`. | Operators must enter complete reset rows; more suspect windows when a provider's reset row is partial |
| R94 | R88 composed only the trailing open period; the Task 7 review showed a closed month the 6-hourly monthly policy has not refreshed yet silently disappears from a multi-month request (Jan and Feb unrefreshed → March only), and the composed month's max demand came from its last day only (42 vs a true 96) | **Ruling (amends R88):** `Analytics` at Monthly/Yearly composes EVERY requested bucket that is absent from the materialised view but has `consumption_daily` buckets, clock-free: each register = the last daily bucket's closing index in the period − the closing index of the last daily bucket before the period (nil when either is missing); `MaxDemandKw` = the maximum over ALL the period's daily buckets; `Row.Partial = true`. Analytics queries with the request range widened to whole buckets of the level, so a request starting mid-bucket never drops or swaps its first row. | A composed row can differ from the later materialised one by the boundary step; it is marked Partial |
| R95 | R64 lists `daily` as a billing boundary kind, but Task 7 used only `load_profile` and `billing`; OSOS and GridBox fetch `daily` for every analyzer, so a meter without load profile got no billing rows at any level | **Ruling:** `daily` is a FALLBACK boundary kind at Daily, Monthly and Yearly levels — never Hourly. Precedence per period, never mixing kinds within a period: Monthly `billing` (R63, 72 h) → `load_profile` → `daily`; Daily and Yearly `load_profile` → `daily`. A `daily` boundary reading must lie within `[bound − 36h, bound]` (`DailySnapshotTolerance`), else no row. The Analytics path has no daily fallback: the continuous aggregates are built from `load_profile` only (migration `00005`), so a daily-only meter has no analytics rows — recorded as Q10. | One more fallback branch; a daily-only meter's hourly billing figures stay absent |
| R96 | Task 7 re-review round 1 — per-PERIOD boundary selection has unflagged money effects: K1 at Daily level a 36 h daily tolerance exceeds the 24 h bucket, so a missing snapshot moves two days into one row; K2 adjacent months billed from different kinds leave a gap or an overlap (6000 or 6400 against a true 6200); K3 a stale load_profile end (no tolerance, Q7) beats a fresh daily pair (1500 instead of 5000) | **Ruling (amends R63 and R95; §3.1 unchanged at Hourly):** at Daily, Monthly and Yearly levels boundaries are resolved **per boundary instant, not per period**: each bucket boundary resolves to exactly ONE reading, chosen by kind precedence (Monthly: `billing` → `load_profile` → `daily`; Daily and Yearly: `load_profile` → `daily`), where a candidate of a kind counts only if it lies within `[bound − tol, bound]` with `tol = min(kindTolerance, bucketWidth / 2)` (`billing` 72 h, `load_profile` 36 h, `daily` 36 h; so every kind is capped at 12 h at Daily level). The SAME resolved reading is the end of one period and the start of the next, so consumption telescopes within a level — no kWh is billed twice or dropped between neighbours. A period may therefore start and end on different kinds; `Row.Source` is the END boundary's kind (documented). A boundary with no candidate within tolerance yields no row for the periods on both sides of it (never a telescoped guess across the gap). At Hourly level §3.1 applies unchanged: `load_profile` only, no tolerance. Reset evidence and priors are unaffected (R90-R93). | Months with a data gap at a boundary produce no billing row instead of a mis-attributed one; F4 must surface that |
| R97 | R59 assigns `missing_readings` to the billing path, but Task 8 found no mechanism: `Billing.Consumption` drops unemitted buckets before `ConsumptionAndRecord` sees them; meanwhile R96 makes a boundary with no candidate yield no row on both sides, and 04 §4.4 has F4 refuse to invoice only over an unresolved anomaly — so a data gap would leave no trace F4 can see | **Ruling:** `ConsumptionAndRecord` writes one `missing_readings` anomaly for every requested bucket that produced no row (the analyzer had readings of some boundary kind within the request's look-back, but the bucket's start or end boundary resolved to no reading, or both resolved to the same reading). `detail` = R60's shape with `"boundaries": ["start"|"end"]` naming the unresolved side(s) and no register map. Deduplicated and locked exactly like the other reasons. An analyzer with no readings at all in the look-back produces no anomalies (nothing to bill, nothing missing). No anomaly is written for a bucket that is still open or in the future (`bucket.To` after the deps clock's now) or that ends before the analyzer's first reading of any boundary kind (pre-installation). Resolution: `manual_override` may supply any register (all are unavailable); `accepted` is allowed; `reset_registered` is `ErrInvalidRequest`. A resolved `manual_override` makes `Billing.Consumption` emit the row from the override values with `Row.Resolution` set. | One more reason branch; an operator must clear gap months before F4 bills them |
| R98 | Final review A I-1: `Billing` returned an open period as if closed (January requested on Jan 30 22:00 → one row, 2872, `Partial=false`); `doc.go` claimed Billing only reads closed periods | **Ruling:** `Billing.Consumption` never emits a bucket whose `To` is after the deps clock's now — open and future buckets are dropped (not partial rows; an invoice-grade figure is complete or absent). `Analytics` keeps `Partial` composition for display. | A caller wanting month-to-date consumption uses Analytics |
| R99 | Final review A I-4/I-5: request size unbounded (a 10-year Yearly Billing request loads ~0.5 GB per analyzer; `AnalyzerIDs` unbounded; load profile range uncapped) and duplicate analyzer ids double totals (5744 vs 2872) | **Ruling:** every service request (Analytics, Billing, loadprofile) is validated before I/O: `AnalyzerIDs` unique and at most `MaxAnalyzersPerRequest = 50`; the range span at most `MaxRequestSpan = 400 days` at every level (multi-year reports page by year in F6); violations → the package's `ErrInvalidRequest`. | F6 pages long reports |
| R100 | Final review A I-2/I-3: a task-id conflict also matches an ACTIVE refresh, so rows committed after the running task passed a view are never materialised; a deduplicated enqueue logs a tenant warning; every 30-day chunk recomputes a whole year of `consumption_yearly` for every tenant; runs of kinds the views ignore still enqueue; lock contention exhausts retries and archives the window; lock TTL shorter than a yearly refresh; release with a cancelled ctx | **Ruling:** (1) ingest enqueues only when the run persisted `load_profile` rows (the only kind the aggregates read); (2) the task id carries a one-minute debounce slot (`floor(now, 1m)`) and the task is enqueued with `ProcessIn(1 min)`, so a burst collapses while pending but a later burst after a task started always gets its own task; `ErrTaskIDConflict` maps to success (no warning); (3) the refresher refreshes a view only for the part of the window OLDER than that view's own policy `start_offset` (hourly 30 d, daily 90 d, monthly 1 y, yearly 5 y) — the policy already covers the rest — and skips the view entirely when nothing is older; (4) one global lock `consumption.refresh` serialises refreshes; on `ErrNotAcquired` the handler re-enqueues itself with `ProcessIn(1 min)` and returns nil (no retry budget consumed, never archived); (5) lock TTL default 30 min; release with `context.WithoutCancel`; (6) the enqueue threshold becomes 29 days (one day of margin inside the hourly policy window, so a new analyzer's oldest hour is never lost). | Refresh latency up to a few minutes after a backfill |
| R101 | Final review A Minor: Billing max demand counted `daily` and `billing` rows at every level, so a day's or month's peak landed inside one hour or day | **Ruling (amends R65):** max-demand kinds depend on the level: Hourly `load_profile`; Daily `load_profile` + `daily`; Monthly and Yearly `load_profile` + `daily` + `billing`. `energy.MaxDemand` takes the allowed kinds as a parameter; `energy.MaxDemandKinds` remains the Monthly/Yearly set. | One parameter |
| R102 | Final review A Minor: Analytics composed and materialised rows can be negative after a meter swap (−49880) | **Ruling:** the Analytics path never returns a negative consumption: a negative register consumption (materialised or composed) is returned as nil for that register, with no suspicion recorded (Analytics does not write anomalies; the Billing path is where suspicion lives). | A chart gap instead of a negative bar |
| R103 | Task 8 re-review round 1: RC-1 R97's filters read look-back data already clamped, so whether a gap anomaly (and a resolved gap override) appears depends on the request range; RI-4 at Hourly a gap override double-bills because §3.1 absorbs the missing hour into the next row (400 billed for 300 kWh); RI-5 a gap override keeps replacing the value after real data is backfilled and hides a new negative delta; RI-2 two concurrent resolves both succeed; RI-3 `BillingDeps.Users` nil lets a non-company resolver write a reset reading | **Ruling (amends R97):** (1) `missing_readings` anomalies are written only at Daily, Monthly and Yearly levels — where R96 yields no row on either side of an unresolved boundary and nothing is absorbed; at Hourly, §3.1 absorbs a missing hour into the next emitted row, so no gap anomaly is written and gap overrides are refused (`ErrInvalidRequest`). (2) Gap detection and gap-override lookup are range-independent: "the analyzer has readings" and "pre-installation" are decided from an UNCLAMPED existence query (earliest reading of any boundary kind), never from the clamped look-back pool. (3) A resolved gap override applies only while the bucket still produces no row from real readings; once real boundaries resolve it, the real derivation wins (and, if suspect, is recorded as a suspect period in the normal way). (4) `ResolveAnomaly` takes the anomaly's own lock and re-reads it inside the lock; an already-resolved anomaly → `ErrConflict`. (5) `BillingDeps.Users` is required for `ResolveAnomaly` (validated before any read or write). | A superseded gap override no longer matches an invoice already issued from it — an F4 re-invoicing concern, recorded for the handoff |
| R104 | Final fix X re-review: R98 closes a bucket by the clock, not by whether its end-boundary data has arrived — at Feb 1 06:00, before the Feb 1 snapshot is ingested, a daily-only meter bills January as 300 instead of 310 with `Partial=false`, and a false `missing_readings` anomaly is written just after midnight at Daily level. Also: `MaxBuckets` is unreachable under R99's span cap, while a legal 50 analyzers × 400 days hourly request still holds ~0.7-1.4 GB | **Ruling (amends R98 and R99):** (1) a Billing bucket is closed only when `now >= bucket.To + SettleDelay(level)`, with `SettleDelay` = Hourly 2 h, Daily 36 h, Monthly and Yearly 72 h (each ≥ that level's boundary tolerance, so an in-tolerance late snapshot can still arrive); the same predicate gates gap anomalies (R97/R103) and gap-override emission. (2) `MaxBuckets` is replaced by `MaxCells = 50 000` — `len(AnalyzerIDs) × bucket count at Level` — validated before any I/O on both consumption paths. | An invoice for a month can be produced from the 4th of the next month; smaller hourly batches for many analyzers |

---

## Execution waves

At most 5 concurrent agents, at most 3 running Docker-backed suites. A wave starts only after every task it depends on is **merged** into `phase/f3-consumption-engine` and the merged tip passes the gate. **Tasks 1 and 5 already ran speculatively and are complete** (✅ below; their fix-round edits land as part of merging them, not as new tasks). Task 6 (the store seam) still runs in Wave A although nothing else needs it until Wave C, because its integration test — a backfilled bucket that is absent until refreshed — is the longest-running piece of evidence in the phase and must not sit on the critical path. (An earlier draft also said Task 1 pre-adds an arch-allowlist entry Task 6 needs; that entry is inert and gone — `internal/store/postgres/admin` is already exempted wholesale from `TestEveryStoreMethodIsScoped` by package path, C-1c — so there is no allowlist coordination between Task 1 and Task 6 to schedule around.) Task 11 splits into **11a** (job/handler/refresher plumbing, needs 1 and 6) and **11b** (the ingest enqueue + wiring, needs 11a) so the refresh handler and the enqueue call site are reviewed and merged separately (M-13). Task 10 moves to Wave D because it consumes Task 7's `consumption.Analytics` type (I-7). Task 12 is withdrawn (R74 deferred to F9) and does not appear in any wave.

| Wave | Tasks | Depends on |
|---|---|---|
| A | 1 ✅, 5 ✅, 6 | Pre-flight |
| B | 2, 3 | 1 |
| C | 4, 7, 10, 11a | 2+3 (4, 7); 6 (7, 11a); 1 (11a); 5 (10 — it reads `store.AnalyticsRepository` directly under R87, so it no longer waits for Task 7) |
| D | 8, 9, 11b | 7 (8, 9); 11a (11b) |
| E | 13 | all |

## File ownership (parallel tasks never edit the same file)

| Task | Files it alone may touch |
|---|---|
| 1 | `internal/domain/energy/{doc,types,period,difference}.go` + their tests, `internal/arch/arch_test.go`, `.golangci.yml`, `internal/service/doc.go` |
| 2 | `internal/domain/energy/{reset,suspect,select}.go` + their tests |
| 3 | `internal/domain/energy/{ratio,demand,activity}.go` + their tests |
| 4 | `internal/domain/energy/golden_test.go`, `internal/domain/energy/testdata/**` |
| 5 | `internal/domain/loadprofile/**` |
| 6 | `internal/store/repository.go` (the `AdminAggregateRepository` block only), `internal/store/postgres/admin/aggregates.go` + its test |
| 7 | `internal/service/consumption/{doc,analytics,billing,convert}.go` + their tests |
| 8 | `internal/service/consumption/{anomalies,resolve}.go` + their tests; `internal/service/consumption/billing.go` (override-substitution addition only, C-6 — Task 7 has already merged by this wave) |
| 9 | `internal/service/consumption/{summary,generation,balance,export}.go` + their tests |
| 10 | `internal/service/loadprofile/**` |
| 11a | `internal/job/{consumption.go,task.go}`, `internal/service/consumption/refresh.go` + its test, `internal/worker/wiring.go` (the `job.Handlers.ConsumptionRefresh` registration only), `internal/worker/wiring_integration_test.go` |
| 11b | `internal/ingest/{fetch.go,deps.go}`, `internal/worker/wiring.go` (the `ingestDeps.ConsumptionRefresh` adapter only — depends on 11a, never edited concurrently with it), `internal/platform/config` (the refresh knobs), `.env.example` (M-6) |
| ~~12~~ | withdrawn — no files (R74 deferred to F9) |
| 13 | `internal/service/consumption/*_acceptance_test.go`, `internal/service/loadprofile/*_acceptance_test.go` (in-package, `//go:build integration`, no `internal/acceptance` — M-5), `docs/`, `HANDOFF_NEXT_SESSION.md` |

## Naming prefixes (F1's collision lesson)

| Shared package | Owning task | Identifier prefix | Notes |
|---|---|---|---|
| `internal/domain/energy` | 1 | types unprefixed (`Register`, `Reading`, `Window`, `Level`, `Derivation`) | Tasks 2 and 3 add functions only, never a second core type |
| `internal/service/consumption` | 7 | `Analytics`, `Billing`, `AnalyticsDeps`, `BillingDeps`, `Row`, `SeriesRequest` (no shared `Service`, R61) | Task 8 prefixes `Anomaly*`; Task 9 prefixes `Summary*`, `Generation*`, `Balance*`, `Export*`; both live on `*Billing` or free functions, never a new core type |
| `internal/job` | 11a | `ConsumptionRefresh*` | `Refresher`/`RefreshDeps`/`NewRefresher` also exist in `internal/service/consumption` (Task 11a) — `job.Refresher` is the handler-seam interface, `consumption.Refresher` is the concrete implementation; do not conflate the two when reading a stack trace |
| `internal/arch/arch_test.go` | 1 | — | No other task edits this file in F3 |

## File Structure

```
internal/domain/energy/                 PURE (Tasks 1-4)
  doc.go                                package doc: §3 rule map, purity contract
  types.go            (T1)              Register, Reading, register ids, nil-is-not-zero helpers
  period.go           (T1)              Level (hourly|daily|monthly|yearly), Window, Istanbul bucket boundaries
  difference.go       (T1)              §3.1 boundary differencing, missing/identical rules
  reset.go            (T2)              §3.2 reset segmentation (R55, R56, R57)
  suspect.go          (T2)              Suspicion, Reason, per-register suspicion (R58, R59, R60)
  select.go           (T2)              SelectBoundary — "the last reading with ts <= bound" (I-17)
  ratio.go            (T3)              §3.3 ratios (R54), DivisionScale
  demand.go           (T3)              §3.5 max demand (R65), MaxDemandKinds
  activity.go         (T3)              §3.6 activity status (R66)
  golden_test.go      (T4)              TestGolden + -update (R80)
  testdata/golden/    (T4)              normal, missing, identical, reset_with_event, reset_without_event, rollover
internal/domain/loadprofile/            PURE (Task 5)
  doc.go, classify.go                   weekday/weekend (R69, R70), season
  profile.go                            per-hour means over matching days (R68), DivisionScale
  statistics.go                         max/min/hour_of_max/mean/stddev/range/load_factor (R67)
internal/service/consumption/           (Tasks 7-9, 11a)
  doc.go                                the two-type split, no shared Service (R61)
  analytics.go              (T7)        Analytics/NewAnalytics — aggregates only
  billing.go                (T7)        Billing/NewBilling — meter_readings only, monthly billing-kind preference (R62, R63), open-period composition (R88)
  convert.go                (T7)        toEnergyReading/toModelKind register round-trip (I-11)
  anomalies.go, resolve.go    (T8)      suspect period creation (ConsumptionAndRecord), listing (ListAnomalies), operator resolution (ResolveAnomaly)
  summary.go, generation.go, balance.go, export.go  (T9)   §5's remaining shapes (R77, R78)
  refresh.go                 (T11a)     Refresher/RefreshDeps/NewRefresher — RefreshConsumption (R71, R72)
internal/service/loadprofile/           (Task 10)
  doc.go, service.go                    resolves weekend days + vacations, calls the pure package
internal/store/                         (Task 6)
  repository.go                         + AdminAggregateRepository
  postgres/admin/aggregates.go          CALL refresh_continuous_aggregate, no transaction (R72)
internal/job/consumption.go             (T11a) task constructor, decoder, job.Refresher handler seam
internal/ingest/fetch.go                (T11b) enqueue on a non-empty, >30-day-old affected range, in both finishRun and failFetchRun (R73, I-12, I-13)
internal/worker/wiring.go               (T11a) ConsumptionRefresh: nil -> real; new handler; (T11b) ingest.Deps adapter wiring
internal/service/consumption/*_acceptance_test.go, internal/service/loadprofile/*_acceptance_test.go   (T13) the F3 end-to-end suite, in-package under //go:build integration (M-5)
```

---

## Task 1: The energy contract, the Istanbul buckets and the extended guards

**Files:**
- Create: `internal/domain/energy/doc.go`, `internal/domain/energy/types.go`, `internal/domain/energy/period.go`, `internal/domain/energy/difference.go`, `internal/service/doc.go` (a placeholder so the new layer's tree is non-empty ahead of Task 7/10's real packages — C-1b)
- Test: `internal/domain/energy/period_test.go`, `internal/domain/energy/difference_test.go`, `internal/domain/energy/helpers_test.go` (I-9: this task owns the package's ONE shared-test-helper file — `readingAt`, `demandAt`, `resetAt`, `win`, `dec`, etc.; Tasks 2, 3 and 4 reuse it and never redefine a helper it already has), `internal/arch/arch_test.go` (extend)
- Modify: `internal/arch/arch_test.go`, `.golangci.yml`

**Interfaces:**
- Consumes: nothing from the project. `github.com/shopspring/decimal`, `github.com/google/uuid` and stdlib are already in `domainAllowedImports` (`internal/arch/arch_test.go:61-87`); `time/tzdata` is **not** — `period.go` needs it (so the tz database resolves in a scratch container with no system tzdata), and this task is what adds the blank import to `domainAllowedImports` and to `.golangci.yml`'s `domain-is-pure` allow list.
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
- `internal/domain/energy` is the tree's first domain package with its own tests. `.golangci.yml`'s `domain-is-pure` files list gains `!**/internal/domain/**/*_test.go` (excluding domain test files from the lint rule), matching the Go-level guard (`internal/arch/arch_test.go`'s `loadPackages`, which never sets `packages.Config.Tests: true` and so never inspects `_test.go` files either) — a domain test file legitimately imports `testing` and testify, neither of which is production arithmetic code. State this once here; Task 5 relies on the same exclusion without repeating the edit.

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
  - `internal/arch/arch_test.go`: add `"./internal/service/..."` to `floatGuardPatterns` and to `TestIntegrationTreesDoNotParseFloats`'s tree list. **No `storeScopeAllowlist` edit**: `internal/store/postgres/admin` (Task 6) is already exempted wholesale from `TestEveryStoreMethodIsScoped` by package path (the guard's `adminPkg` check), so an allowlist entry for `RefreshConsumption` would be inert (C-1c) — do not add one.
  - Add `TestServiceLayerImportBoundaries`: walks `./internal/service/...` and fails on an import of `internal/api` or `internal/ingest`. This is the ONLY direction this new test checks (R76).
  - Extend the EXISTING `TestStoreAndIntegrationDoNotImportAPIOrService` (`internal/arch/arch_test.go:161`, which already walks `./internal/store/...` and `./internal/integration/...` for an `internal/api` import) to also fail on an import of `internal/service` — this is the store/integration → service direction, and it belongs on the guard that already loads those two trees, not duplicated on `TestServiceLayerImportBoundaries` (R76).
  - `.golangci.yml`: add `./internal/service/...` to the `no-float-money` tree list only — **not** `domain-is-pure` (C-1c: `internal/service` legitimately imports `internal/store`, `internal/job` and `internal/platform`, which `domain-is-pure`'s allow list would have to be widened to admit, defeating the guard). Also add `!**/internal/domain/**/*_test.go` to `domain-is-pure`'s files list and `time/tzdata$` to its allow list (see Rules, above) — these two are real edits this task makes to `domain-is-pure`, just not the service-tree one.

| Guard | Mutation that must turn it red |
|---|---|
| `TestServiceLayerImportBoundaries` | create `internal/service/probe/probe.go` importing `.../internal/ingest`; expect FAIL naming the import. Delete the probe. |
| `TestStoreAndIntegrationDoNotImportAPIOrService` (extended) | create `internal/store/probe/probe.go` importing `.../internal/service`; expect FAIL naming the import. Delete the probe. |
| `floatGuardPatterns` incl. service | add `type probe struct{ X float64 }` to that same probe file; expect FAIL naming `probe.X`. Delete. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build && make check-generate
git add internal/domain/energy internal/arch/arch_test.go .golangci.yml internal/service
git commit -m "feat(f3): task-1 — energy contract, Istanbul buckets, §3.1 differencing, service-layer guards" <TRAILERS>
```

---

## Task 2: Reset segmentation and suspect classification (§3.2)

**Files:**
- Create: `internal/domain/energy/reset.go`, `internal/domain/energy/suspect.go`, `internal/domain/energy/select.go` (I-17)
- Test: `internal/domain/energy/reset_test.go`, `internal/domain/energy/suspect_test.go`, `internal/domain/energy/select_test.go`

**Interfaces:**
- Consumes: Task 1's `Reading`, `Window`, `Derivation`, `Suspicion`, `Reason`, `Difference`. `Suspicion` and `Reason` stay defined in Task 1's `types.go`; `suspect.go` in this task holds helper functions only — it never redeclares either type (I-19).
- Produces:

```go
// Derive implements 02 §3.1 together with §3.2.
//
// start and end are the boundary readings (§3.1), already selected by
// SelectBoundary below. resets are the kind=reset readings with TS in
// (start.TS, end.TS] — strictly after the start boundary and at or before the
// end boundary (I-18, matching F2's R14 exactly) — in any order; Derive sorts
// them. A reset between start.TS and w.From, or exactly at end.TS, IS evidence
// even though it sits outside [w.From, w.To): what matters is the boundary
// READINGS actually selected, not the nominal window.
//
// With reset evidence the period is split at every reset and consumption is the
// sum of the segment differences (R55, R56):
//
//	(before_reset - reading_start) + (reading_end - after_reset)
//
// after_reset is the reset row's own value. before_reset is the value of the
// last load_profile-kind reading strictly before the reset (a "prior",
// supplied by the caller regardless of the window's own boundary kind, R55).
// When no such prior exists, the register is suspect (ReasonMeterReset) for
// that reset — it is NEVER assumed to be reading_start's value, and the
// pre-reset segment is never silently contributed as zero.
//
// Without usable evidence the register is suspect and its value is nil. A
// negative difference NEVER becomes a number (removed-behaviour 1).
func Derive(w Window, start, end *Reading, resets []Reading, priors []Reading) Derivation

// SelectBoundary returns "the last reading with ts <= bound" from readings,
// which callers must pre-sort ascending and pre-filter to the kind they want
// (I-17). Returns nil when no reading qualifies.
func SelectBoundary(readings []Reading, bound time.Time) *Reading
```

**Rules the implementation must follow:**
- Resets relevant to a window are those with `TS` in `(start.TS, end.TS]` — NOT "inside `w`" — because the readings actually selected as boundaries, not the nominal `[w.From, w.To)`, define which resets are evidence (I-18). Sort `resets` by `TS`.
- Per register, walk the segments in order. A segment whose own difference is negative makes that register suspect with `ReasonNegativeDelta` and discards the whole period's value for it.
- A reset row whose value for a register is nil makes that register suspect with `ReasonMeterReset` (R55) — never zero, never skipped silently.
- A register with no prior (`load_profile`-kind reading before the reset) makes that register suspect with `ReasonMeterReset` too (R55, I-1) — the old "falls back to reading_start, contributing zero" behaviour is gone.
- `Suspicion.ResetRows` records how many reset rows were inside the window, so the operator message can say "a reset exists but could not be applied" versus "no reset at all".
- Registers untouched by the problem keep their derived values (R58).
- No rounding, no modulus, no rollover arithmetic (R57).
- `SelectBoundary` is a pure linear/binary scan with no I/O; it does not filter by kind itself — the caller (Task 7) pre-filters `readings` to the kind it wants before calling.

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
	// R93 (reverses I-16): a reset row that omits T1 while both boundaries report
	// it is unusable evidence for T1, so T1 is meter_reset-suspect.
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.T1Import].Reason)
	require.Equal(t, 1, d.Suspect[energy.T1Import].ResetRows)
}

func TestDeriveHandlesTwoResetsInOnePeriod(t *testing.T) { /* 900->1000 reset 0 ->60 reset 0 ->25 == 185 */ }

// I-1: inverts the earlier "…ContributesZeroForTheFirstSegment" framing. A
// missing before-reset value makes the register suspect — it is never
// assumed to be reading_start's value, and the pre-reset segment is never
// silently contributed as zero.
func TestDeriveIsSuspectWhenNoReadingPrecedesTheReset(t *testing.T) {
	// No prior between reading_start and the reset.
	d := energy.Derive(win(t0, time.Hour), readingAt(t0, "900"), readingAt(t0.Add(time.Hour), "40"),
		[]energy.Reading{*resetAt(t0.Add(10*time.Minute), "0")}, nil)
	require.Nil(t, d.Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonMeterReset, d.Suspect[energy.ActiveImport].Reason)
}

// I-18: the reset evidence window is (start.TS, end.TS], matching F2's R14 —
// not [w.From, w.To). A reset exactly at end.TS counts; a reset between
// start.TS and w.From (before the window nominally begins, because
// start.TS < w.From when no reading landed exactly on the boundary) also
// counts, because it is evidence for the segment the boundary READING
// actually spans.
func TestDeriveTreatsAResetExactlyAtTheEndBoundaryAsEvidence(t *testing.T) {
	start := readingAt(t0, "900")
	end := readingAt(t0.Add(time.Hour), "40")
	reset := resetAt(t0.Add(time.Hour), "0") // exactly at end.TS
	d := energy.Derive(win(t0, time.Hour), start, end, []energy.Reading{*reset}, []energy.Reading{*readingAt(t0.Add(30*time.Minute), "1000")})
	require.Equal(t, "140", d.Values[energy.ActiveImport].String(), "reset at end.TS is (start.TS, end.TS]-inclusive")
}

func TestDeriveTreatsAResetBeforeTheWindowAsEvidenceWhenTheStartBoundaryPrecedesIt(t *testing.T) {
	// start's own TS is BEFORE w.From (no reading landed exactly on the
	// boundary); a reset between start.TS and w.From is still evidence.
	w := win(t0, time.Hour) // w.From = t0
	start := readingAt(t0.Add(-30*time.Minute), "900")
	reset := resetAt(t0.Add(-10*time.Minute), "0") // between start.TS and w.From
	end := readingAt(t0.Add(time.Hour), "40")
	d := energy.Derive(w, start, end, []energy.Reading{*reset}, []energy.Reading{*readingAt(t0.Add(-20*time.Minute), "1000")})
	require.Equal(t, "140", d.Values[energy.ActiveImport].String())
}

func TestSelectBoundaryReturnsTheLastReadingAtOrBeforeTheBound(t *testing.T) {
	bound := t0.Add(time.Hour)
	readings := []energy.Reading{*readingAt(bound.Add(-time.Nanosecond), "10"), *readingAt(bound, "20"), *readingAt(bound.Add(time.Nanosecond), "30")}
	got := energy.SelectBoundary(readings[:2], bound) // exactly at bound
	require.Equal(t, "20", got.Value(energy.ActiveImport).String())
}

func TestSelectBoundaryExcludesAReadingOneNanosecondAfterTheBound(t *testing.T) {
	bound := t0.Add(time.Hour)
	readings := []energy.Reading{*readingAt(bound.Add(-time.Nanosecond), "10")}
	got := energy.SelectBoundary(readings, bound.Add(time.Nanosecond)) // the +1ns reading is not in this slice
	require.Equal(t, "10", got.Value(energy.ActiveImport).String())
}

func TestSelectBoundaryReturnsNilWhenNothingQualifies(t *testing.T) {
	bound := t0
	readings := []energy.Reading{*readingAt(bound.Add(time.Nanosecond), "10")}
	require.Nil(t, energy.SelectBoundary(readings, bound))
}
```

- [ ] **Step 2: Run and watch them fail.** `go test ./internal/domain/energy/... -run 'TestDerive|TestSelectBoundary' -v` → FAIL, `undefined: energy.Derive`.
- [ ] **Step 3: Implement `reset.go`, `suspect.go` and `select.go`.**
- [ ] **Step 4: Run and watch them pass.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestDeriveWithoutAResetEventIsSuspect` | return the end reading's value when the difference is negative (the legacy defect, removed-behaviour 1): expect FAIL with `12.5` observed. Restore. |
| `TestDeriveWithAnUnusableResetRowIsSuspectForThatRegisterOnly` | treat a nil reset value as `decimal.Zero`: expect FAIL — `T1Import` derives a number instead of being suspect. Restore. |
| `TestDeriveAppliesTheResetFormula` | drop the pre-reset segment (return only `end - after`): expect FAIL, `40` ≠ `140`. Restore. |
| `TestDeriveIsSuspectWhenNoReadingPrecedesTheReset` | fall back to `reading_start`'s value when no prior exists (the old, retired ruling): expect FAIL — `active_import` derives `40` instead of being suspect. Restore. |
| `TestDeriveTreatsAResetExactlyAtTheEndBoundaryAsEvidence` | use a half-open `[start.TS, end.TS)` evidence window instead of `(start.TS, end.TS]`: expect FAIL — the reset at `end.TS` is dropped and `active_import` goes suspect instead of `140`. Restore. |
| `TestSelectBoundaryExcludesAReadingOneNanosecondAfterTheBound` | use `<` instead of `<=`: expect FAIL on the exactly-at-bound case in the previous test. Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
git add internal/domain/energy
git commit -m "feat(f3): task-2 — §3.2 reset segmentation, per-register suspicion and boundary selection" <TRAILERS>
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
// DivisionScale is the only scale any division in this package uses (C-5,
// R79). Nothing else rounds: a subtraction, addition or comparison never
// needs to.
const DivisionScale int32 = 20

// Ratio divides two quantities and is nil whenever the result would be
// meaningless: a nil or non-positive denominator, or a nil numerator (R54).
// 02 §3.3 says "0 when active_import = 0"; 09 §F3's acceptance criterion
// requires a null ratio, and the acceptance criterion wins. A ratio is never
// computed against a substituted 1. Division uses DivRound(…, DivisionScale).
func Ratio(numerator, denominator *decimal.Decimal) *decimal.Decimal

// Ratios returns the inductive and capacitive ratios of a derivation (§3.3),
// both relative to derived active-import consumption.
func Ratios(d Derivation) (inductive, capacitive *decimal.Decimal)

// MaxDemandKinds is the closed, exported set of reading kinds MaxDemand reads
// (C-3, re-ruled from an earlier draft). billing carries ARIL's authoritative
// monthly maximum (see R65); load_profile and daily carry the interval's own
// max_demand_kw. current_index is EXCLUDED: it is stamped at its own
// ProfileDate and can carry the previous month's peak into the wrong period
// (the misattribution F2's R49 already fixed for the persisted reading).
// reset is excluded too (evidence, not a demand source).
var MaxDemandKinds = []Kind{KindLoadProfile, KindDaily, KindBilling}

// MaxDemand is the maximum of the interval max_demand values in the window —
// never a difference and never a sum (§3.5, R65) — over readings whose Kind is
// in MaxDemandKinds. Readings of any other kind (current_index, reset) are
// ignored even if they carry a non-nil MaxDemandKw; nil when no reading
// counted carries a value.
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
	// C-5: division is carried to DivisionScale (20) places. 4.045/12.25 and
	// 1/3 are hand-verified by long division (shown in the task report, not
	// in the fixture): 809/2450 (4.045/12.25 reduced) = 0.33020408163265306122,
	// and 1/3 = 0.33333333333333333333, both to exactly 20 fractional digits.
	require.Equal(t, "0.33020408163265306122", energy.Ratio(dec("4.045"), dec("12.25")).String())
	require.Equal(t, "0.33333333333333333333", energy.Ratio(dec("1"), dec("3")).String())
}

func TestRatiosOfASuspectDerivationAreNil(t *testing.T) { /* active suspect -> both nil */ }

func TestMaxDemandTakesTheMaximumNotTheDifference(t *testing.T) {
	rs := []energy.Reading{demandAt(t0, "12.5"), demandAt(t0.Add(15*time.Minute), "40.25"), demandAt(t0.Add(30*time.Minute), "9")}
	require.Equal(t, "40.25", energy.MaxDemand(win(t0, time.Hour), rs).String())
}

func TestMaxDemandIsNilWhenNoReadingCarriesOne(t *testing.T) { require.Nil(t, energy.MaxDemand(win(t0, time.Hour), []energy.Reading{*readingAt(t0, "100")})) }

// C-3: billing carries ARIL's authoritative monthly max; current_index and
// reset are excluded even when they carry a non-nil MaxDemandKw.
func TestMaxDemandCountsBillingRowsAndExcludesCurrentIndexAndReset(t *testing.T) {
	w := win(t0, time.Hour)
	billing := demandAt(t0, "40.25")
	billing.Kind = energy.KindBilling
	currentIndex := demandAt(t0.Add(15*time.Minute), "999") // excluded: R65/C-3
	currentIndex.Kind = energy.KindCurrentIndex
	reset := demandAt(t0.Add(30*time.Minute), "999")
	reset.Kind = energy.KindReset
	require.Equal(t, "40.25", energy.MaxDemand(w, []energy.Reading{billing, currentIndex, reset}).String())
}

func TestActivityStatusUsesSevenDays(t *testing.T) {
	// I-8: literal offsets, not arithmetic on ActivityWindow itself, so the
	// test still means something if ActivityWindow's own value is wrong.
	require.Equal(t, 7*24*time.Hour, energy.ActivityWindow)
	now := time.Date(2025, 3, 14, 9, 0, 0, 0, time.UTC)
	within := now.Add(-6 * 24 * time.Hour)
	outside := now.Add(-8 * 24 * time.Hour)
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
| `TestRatioIsAnUnroundedFraction` | round the result to 4 decimal places before returning: expect FAIL — `"0.3302"` ≠ `"0.33020408163265306122"`. Restore. |
| `TestMaxDemandTakesTheMaximumNotTheDifference` | return `last - first`: expect FAIL. Restore. |
| `TestMaxDemandCountsBillingRowsAndExcludesCurrentIndexAndReset` | include `current_index` in the kind filter: expect FAIL — `999` appears instead of `40.25`. Restore. |
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
- Consumes: Tasks 1-3 (`Derive`, `Difference`, `Ratios`, `MaxDemand`); Task 2's `energy.SelectBoundary` (I-14, I-17).
- Produces: `TestGolden`, run by the phase verification block `go test ./internal/domain/... -run TestGolden -v`.

**Rules the implementation must follow (R80, I-14):**
- One JSON file per case, holding `name`, `doc` (one line saying which spec rule the case proves), `input` and `want` together.
- **`input` holds RAW readings for the window PLUS look-back — never pre-selected boundaries** (I-14). Its shape is `{"window": {...}, "readings": [{"ts","kind","values"}, ...], "resets": [{"ts","kind","values"}, ...]}`. `readings` spans from before `window.From` to at/after `window.To`, exactly like the pool `Billing.Consumption` fetches once per (analyzer, kind) in production (I-17). The harness selects `start`/`end` from `readings` with `energy.SelectBoundary(readings, bound)` itself — so **the selection logic is golden-tested**, not assumed — and passes `readings` again as `Derive`'s `priors` argument, exactly as Task 7's `Billing.Consumption` reuses the same fetched pool for both purposes.
- **`want.values` lists every register in `AllRegisters()` order with an explicit `null`** for one the case's readings never reported (I-14) — never omitted, never `0`.
- `-update` rewrites only `want`. The harness never writes a golden file during a normal run and never writes `input`.
- Decimals are strings on both sides.
- The six cases are exactly the six the acceptance criterion names: a normal period, a period with missing readings, a period whose boundary readings are identical, a meter reset with a reset event, a meter reset without one, and a rollover (R57: a wrap **with** a reset row; the uncovered wrap is the `reset_without_event` case).

- [ ] **Step 1: Write the failing test and the six `input` blocks with hand-computed `want` blocks.**

```go
var update = flag.Bool("update", false, "rewrite the want block of each golden file")

func TestGolden(t *testing.T) {
	for _, path := range mustGlob(t, "testdata/golden/*.json") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			var c goldenCase
			mustReadJSON(t, path, &c)
			got := runCase(t, c.Input) // SelectBoundary + Derive + Ratios + MaxDemand
			if *update {
				c.Want = got
				mustWriteJSON(t, path, c)
				return
			}
			require.Equal(t, c.Want, got, "%s: %s", c.Name, c.Doc)
		})
	}
}

// runCase is the ONE place selection meets derivation, mirroring
// Billing.Consumption's query shape (I-17): select boundaries from the same
// pool that also serves as Derive's priors.
func runCase(t *testing.T, in goldenInput) goldenWant {
	start := energy.SelectBoundary(in.Readings, in.Window.From)
	end := energy.SelectBoundary(in.Readings, in.Window.To)
	d := energy.Derive(in.Window, start, end, in.Resets, in.Readings)
	inductive, capacitive := energy.Ratios(d)
	return goldenWant{
		Emitted:  d.Emitted,
		Values:   valuesInAllRegistersOrder(d.Values), // explicit null per I-14
		Suspect:  d.Suspect,
		Inductive: inductive, Capacitive: capacitive,
		MaxDemandKw: energy.MaxDemand(in.Window, in.Readings),
	}
}
```

Hand-computed `want` for `normal.json` (the numbers are computed by hand in the task report, not by running the code):
```json
{
  "name": "normal",
  "doc": "02 §3.1: a period with both boundaries present differences every register",
  "input": { "window": {"from": "2025-03-14T00:00:00+03:00", "to": "2025-03-14T01:00:00+03:00"},
             "readings": [
               {"ts": "2025-03-14T00:00:00+03:00", "kind": "load_profile",
                "values": {"active_import": "1000.5000", "reactive_inductive_import": "120.0000", "t1_import": "400.0000"}},
               {"ts": "2025-03-14T01:00:00+03:00", "kind": "load_profile",
                "values": {"active_import": "1012.7500", "reactive_inductive_import": "124.0450", "t1_import": "412.2500"}}
             ],
             "resets": [] },
  "want":  { "emitted": true,
             "values": {
               "active_import": "12.25", "reactive_inductive_import": "4.045", "reactive_capacitive_import": null,
               "t1_import": "12.25", "t2_import": null, "t3_import": null,
               "active_export": null, "reactive_inductive_export": null, "reactive_capacitive_export": null,
               "t1_export": null, "t2_export": null, "t3_export": null
             },
             "suspect": {},
             "inductive_ratio": "0.33020408163265306122",
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
| `TestGolden` | select boundaries with `<` instead of `<=` in the harness's own call site (bypassing `SelectBoundary`'s own contract): expect FAIL on a fixture with a reading exactly at `window.To`. Restore. |
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
- Test: `internal/domain/loadprofile/{classify,profile,statistics}_test.go`, `internal/domain/loadprofile/helpers_test.go` (I-9: this package's one shared-test-helper file, already created — matching Task 1's `energy` package)

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

// DivisionScale is this package's only division scale (C-5, matching
// energy.DivisionScale). Nothing else rounds.
const DivisionScale int32 = 20

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
- No rounding anywhere (R79). `loadprofile.DivisionScale int32 = 20` is this package's only division scale (C-5, matching `energy.DivisionScale`): means and load factor use `DivRound(…, DivisionScale)`, and the standard deviation's square root uses `variance.PowWithPrecision(decimal.New(5, -1), DivisionScale)` (M-7) — `decimal.New(5, -1)` is exactly `1/2` expressed without a `float64` literal anywhere in the domain layer. Every division is documented as "precision, not presentation".

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
- Consumes: F1's migrations `00004`/`00005`, `testfixtures.NewIsolatedDB`, `testfixtures.NewTenant`. No arch-allowlist coordination with Task 1 is needed: `internal/store/postgres/admin` is already exempted wholesale from `TestEveryStoreMethodIsScoped` by package path (C-1c) — there is no `storeScopeAllowlist` entry to rely on.
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
- The statement is `CALL refresh_continuous_aggregate($1::regclass, $2::timestamptz, $3::timestamptz)` (I-15) executed on the pool, never on a `pgx.Tx`, and the first parameter is bound from the `AggregateView` constant — never string-formatted.
- An unknown `AggregateView` is rejected with `admin.ErrUnknownView` before any I/O.
- `Refresh` validates `r.Valid()` first and returns `ErrInvalidRange` otherwise (F1's contract for every method taking a `TimeRange`).
- Errors go through `pgerr.Translate` like every other repository.

- [ ] **Step 1: Write the failing integration test** (`//go:build integration`)

C-2: the original single-scenario proof conflated two different views' behaviour. `consumption_monthly` is `materialized_only = true` (no real-time union at all); `consumption_hourly` is `materialized_only = false` (a real-time union serves everything ABOVE the materialisation watermark). The phase's refresh story needs a proof for each:

```go
// (a) consumption_monthly is materialized_only = true (migration 00005): a
// closed bucket is materialised or it does not exist, period — there is no
// live tail to fall back on at any offset.
func TestRefreshMaterialisesAClosedMonthlyBucketTwoYearsOld(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ten := testfixtures.NewTenant(t, ctx, pool, 1)
	analyzer := ten.Analyzers[0]

	// A closed calendar month, two years back — both readings inside it.
	loc, _ := time.LoadLocation("Europe/Istanbul")
	now := time.Now().In(loc)
	monthStart := time.Date(now.Year()-2, now.Month(), 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, 0)
	insertReadingsAt(t, pool, analyzer.ID,
		reading{ts: monthStart, value: "1000"},
		reading{ts: monthEnd.Add(-time.Hour), value: "1240"})

	an := postgres.NewAnalyticsRepository(pool)
	before, err := an.ConsumptionMonthly(ctx, ten.AdminScope, []uuid.UUID{analyzer.ID}, store.TimeRange{From: monthStart.UTC(), To: monthEnd.UTC()})
	require.NoError(t, err)
	require.Empty(t, before, "materialized_only=true: the bucket is absent, not stale, until refreshed")

	repo := admin.NewAggregateRepository(pool)
	require.NoError(t, repo.Refresh(ctx, store.ViewConsumptionMonthly, store.TimeRange{From: monthStart.UTC(), To: monthEnd.UTC()}))

	after, err := an.ConsumptionMonthly(ctx, ten.AdminScope, []uuid.UUID{analyzer.ID}, store.TimeRange{From: monthStart.UTC(), To: monthEnd.UTC()})
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.Equal(t, "240", after[0].ActiveConsumption.String())
}

// (b) consumption_hourly is materialized_only = false: a real-time union
// serves everything ABOVE the materialisation watermark, so this test must
// first PUSH the watermark past the old bucket with its own refresh (over an
// empty table — a refresh advances the "already refreshed" bookkeeping even
// when there is nothing to materialise, exactly as a scheduled refresh does
// over time), and only THEN insert the old readings — otherwise the
// real-time union would serve them live and the test would prove nothing
// about materialisation at all.
func TestRefreshMaterialisesAnHourlyBucketBelowAWatermarkThatHasAlreadyMoved(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ten := testfixtures.NewTenant(t, ctx, pool, 1)
	analyzer := ten.Analyzers[0]
	repo := admin.NewAggregateRepository(pool)
	now := time.Now()

	// Move the watermark forward on an EMPTY table.
	require.NoError(t, repo.Refresh(ctx, store.ViewConsumptionHourly, store.TimeRange{From: now.Add(-400 * 24 * time.Hour), To: now.Add(-time.Hour)}))

	// NOW insert two readings 30 minutes apart inside one old hour, below
	// the watermark that refresh just advanced past.
	base := now.Add(-390 * 24 * time.Hour).Truncate(time.Hour)
	insertReadingsAt(t, pool, analyzer.ID,
		reading{ts: base, value: "1000"},
		reading{ts: base.Add(30 * time.Minute), value: "1012.5"})

	an := postgres.NewAnalyticsRepository(pool)
	before, err := an.ConsumptionHourly(ctx, ten.AdminScope, []uuid.UUID{analyzer.ID}, store.TimeRange{From: base, To: base.Add(time.Hour)})
	require.NoError(t, err)
	require.Empty(t, before, "below the watermark the real-time union does not see it either")

	require.NoError(t, repo.Refresh(ctx, store.ViewConsumptionHourly, store.TimeRange{From: base, To: base.Add(time.Hour)}))

	after, err := an.ConsumptionHourly(ctx, ten.AdminScope, []uuid.UUID{analyzer.ID}, store.TimeRange{From: base, To: base.Add(time.Hour)})
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.Equal(t, "12.5", after[0].ActiveConsumption.String())
}

// M-8: proves the sentinel fires before any I/O, using a CLOSED pool — any
// attempt to reach the database at all would surface as a "conn closed"
// error, not the sentinel, so a passing test proves no I/O happened.
func TestRefreshRejectsAnUnknownViewBeforeTouchingTheDatabase(t *testing.T) {
	closedPool := testfixtures.NewIsolatedDB(t)
	closedPool.Close()
	repo := admin.NewAggregateRepository(closedPool)
	err := repo.Refresh(ctx, store.AggregateView("not_a_real_view"), store.TimeRange{From: t0, To: t0.Add(time.Hour)})
	require.ErrorIs(t, err, admin.ErrUnknownView)
}

func TestRefreshRejectsAnInvalidRange(t *testing.T)             { /* ErrInvalidRange */ }
func TestRefreshWalksTheFourViewsFinestFirst(t *testing.T)      { /* ConsumptionViews order */ }
```

- [ ] **Step 2: Run and watch them fail.** `go test ./internal/store/postgres/admin/... -tags=integration -run TestRefresh -v`
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run and watch them pass.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestRefreshMaterialisesAClosedMonthlyBucketTwoYearsOld` | make `Refresh` a no-op returning nil: expect FAIL — the post-refresh read is still empty. This is one of the two proofs the whole refresh story rests on. Restore. |
| `TestRefreshMaterialisesAnHourlyBucketBelowAWatermarkThatHasAlreadyMoved` | make `Refresh` a no-op returning nil: expect FAIL — the post-refresh read is still empty. Restore. |
| `TestRefreshRejectsAnUnknownViewBeforeTouchingTheDatabase` | remove the closed-set validation (accept any `AggregateView` string and build the statement with `fmt.Sprintf` instead of the fixed four-constant switch): expect FAIL — the closed pool now returns a connection error instead of `admin.ErrUnknownView`, proving an unknown view would otherwise reach the database. Restore — and note in the report that this mutation is also the injection hazard the closed set exists to prevent (I-8). |

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
- Create: `internal/service/consumption/{doc,analytics,billing,convert}.go`
- Test: `internal/service/consumption/{analytics,billing,convert}_test.go`, `internal/service/consumption/paths_integration_test.go`, `internal/service/consumption/helpers_test.go` (I-9: this task owns the package's ONE shared-test-helper file; Tasks 8, 9 and 11a reuse it and never redefine a helper it already has, matching Task 1's `energy` package and Task 5's `loadprofile` package, both of which already own their own `helpers_test.go`)

**Interfaces:**
- Consumes: Task 1-3's `energy` package (including `energy.SelectBoundary`, `energy.DivisionScale`, `energy.MaxDemandKinds`); `store.AnalyticsRepository`, `store.ReadingRepository`, `store.AnomalyRepository`, `store.OpsRepository`, `store.Scope`, `store.TimeRange`; `platform/clock`; `model.MeterReading`, `model.ReadingKind` (for `convert.go`).
- Produces:

```go
package consumption

// ErrInvalidRequest is this package's fail-closed validation sentinel.
// store.ErrValidation does NOT exist (pre-flight finding): F3 defines its
// own, here, because Analytics and Billing both validate before any I/O.
var ErrInvalidRequest = errors.New("consumption: invalid request")

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

// Analytics and Billing are two independent types with NO shared Service
// struct (R61): each is constructed alone, so there is no other-repository
// field on either type to poison in a guard test — the structure itself is
// the proof.
type Analytics struct{ /* unexported */ }
type Billing struct{ /* unexported */ }

func NewAnalytics(d AnalyticsDeps) (*Analytics, error)
func NewBilling(d BillingDeps) (*Billing, error)

// SeriesRequest asks for one analyzer's series at one level.
// AnalyzerIDs is required and positional: empty means no rows, never "all".
// MaxBuckets = 10000: a request whose [Range.From, Range.To) at Level would
// expand past this many buckets fails ErrInvalidRequest before any I/O
// (I-17) — a runaway range is a client error, not a slow query.
const MaxBuckets = 10000

type SeriesRequest struct {
	AnalyzerIDs []uuid.UUID
	Level       energy.Level
	Range       store.TimeRange
}

// Row is one period. Values carries every register's consumption; a nil entry
// is unreported or suspect. Source records which reading kind produced the row
// (R62) — energy.Kind, not a bare string, so an invalid value cannot compile.
// Indexes carries the register's CLOSING index for the period (05 §5 "every
// index field", Task 9): Analytics fills it from the bucket's own closing
// index columns, Billing from the end boundary reading. Partial is true for
// an open (still-in-progress) period Analytics composes from consumption_daily
// (R88) — never set by Billing, which only ever reads closed periods.
// Resolution is populated by Task 8's override-substitution logic in
// billing.go (C-6): a register present here was covered by a resolved
// manual_override anomaly for this window, and its Values entry is the
// operator-supplied figure, not a derived one. Task 7 itself always leaves
// this nil — there is no anomaly-reading logic in Task 7's own Consumption.
type Row struct {
	AnalyzerID      uuid.UUID
	Window          energy.Window
	Values          map[energy.Register]*decimal.Decimal
	Indexes         map[energy.Register]*decimal.Decimal
	InductiveRatio  *decimal.Decimal
	CapacitiveRatio *decimal.Decimal
	MaxDemandKw     *decimal.Decimal
	Source          energy.Kind
	Suspect         map[energy.Register]energy.Suspicion
	Partial         bool
	Resolution      map[energy.Register]string
}

// BillingSnapshotTolerance bounds R63's "covering": a billing-kind boundary
// reading older than this relative to its own bound does not count as
// covering the month (I-3).
const BillingSnapshotTolerance = 72 * time.Hour

// Consumption serves charts and tables from the continuous aggregates. It is
// fast and slightly understates a period: a bucket misses the step between the
// last reading of one bucket and the first of the next (04 §4.3). At
// Monthly/Yearly, the current OPEN period is composed from consumption_daily
// closing indexes (R88) — never from meter_readings, which would break R61.
func (a *Analytics) Consumption(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error)

// Consumption derives consumption from meter_readings at the true period
// boundaries (02 §3.1, §3.2). It is exact, slower, and the only path that may
// feed an invoice. A suspect period returns nil values and a Suspicion; it
// never returns a number (removed-behaviour 1). Writing the anomaly row and the
// operator message is Task 8's ConsumptionAndRecord wrapper, on the same type.
func (b *Billing) Consumption(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error)

// toEnergyReading and toModelKind convert between model.MeterReading/
// model.ReadingKind and energy.Reading/energy.Kind (I-11). Every
// energy.Register round-trips; MultiplierApplied is read for audit only and
// never re-applied (Global Constraints).
func toEnergyReading(r model.MeterReading) energy.Reading
func toModelKind(k energy.Kind) model.ReadingKind
```

**Rules the implementation must follow:**
- `Analytics.Consumption` maps `Level` to the matching `AnalyticsRepository` method and converts `model.ConsumptionBucket` to `Row` via `toEnergyReading`'s sibling conversions; it computes ratios with `energy.Ratio` from the bucket's own values (using `energy.DivisionScale`) and sets `Source: energy.KindLoadProfile` always (R62). It never calls `energy.Derive`.
- **Open period composition (R88):** at `Monthly`/`Yearly`, `Analytics.Consumption` additionally composes the current open period from `consumption_daily`: each register's value = the closing index of the last `consumption_daily` bucket inside the open period, minus the closing index of the last bucket before it; `nil` when either is missing. The composed `Row` has `Partial: true`. This never reads `meter_readings`.
- `Billing.Consumption`'s query shape (I-17): for each analyzer and each kind it needs, it reads the kind **ONCE** over `[lookback, range.To + 1ns)` — `lookback` is the `ts` of `BoundaryReadings`' own start reading for the FIRST bucket in the request — then selects each bucket's start/end boundary **in memory** with `energy.SelectBoundary(readings, bound)` (readings pre-sorted ascending BY `ts` — `ReadingRepository.Range` orders by `mr.ts`; never concatenate two reads without re-establishing the order, R91 — and pre-filtered to the kind; the priors slice passed to `Derive` is `load_profile` only and will include the start boundary reading itself, which R91 excludes by `TS > start.TS`). This replaces one query per bucket with one query per (analyzer, kind) pair. A request whose bucket count at `Level` exceeds `MaxBuckets` (10000) fails `ErrInvalidRequest` before any I/O.
- For each window, `Billing` also reads reset rows for the window (`Range(..., KindReset)`, evidence window `(start.TS, end.TS]`, I-18) and `load_profile`-kind priors (R55) before calling `energy.Derive`. `current_index` is never a boundary kind (R64).
- Monthly billing-kind preference, staleness-bounded (R63): for `Monthly`, first try `BoundaryReadings(..., KindBilling, ...)`; the month counts as covered only when BOTH the start and end boundary readings are non-nil AND each lies within `BillingSnapshotTolerance` (72h) of its own bound. Otherwise the month falls back to `KindLoadProfile`. A period never mixes kinds.
- Max demand comes from `energy.MaxDemand` over readings of kinds `energy.MaxDemandKinds` (`load_profile`, `daily`, `billing` — C-3; NOT `current_index`) in the window.
- Both methods validate `sc.Valid()`, a non-empty `AnalyzerIDs`, `req.Range.Valid()` and the `MaxBuckets` cap, returning `ErrInvalidRequest`, before any I/O.
- `!Derivation.Emitted ⇒ Row.Values == nil && Row.Suspect == nil` (M-9): a `!Emitted` window never becomes a zero-valued or empty-but-present `Row` map — the whole row is simply not emitted into the result slice.
- Every `time.Time` comparison in this package uses `.Equal`, never `==` (M-15): a `time.Time` carries a monotonic reading that `==` compares and a decoded-from-Postgres value never has.

- [ ] **Step 1: Write the failing tests** — unit tests with fakes for the path separation, integration tests for the real difference.

```go
// R61: the separation is structural. Analytics has no ReadingRepository field
// at all — there is nothing to poison, no fallback path to accidentally take.
// I-8: the fake AnalyticsRepository returns NO rows deliberately — this is
// the one condition under which a "fall back to Readings when the aggregate
// is empty" mutation would actually have somewhere to fall back TO, so it is
// the fixture that makes that mutation's absence provable, not an
// accidental oversight.
func TestAnalyticsPathCannotReachTheHypertable(t *testing.T) {
	a, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: fakeAnalytics{rows: nil}, Log: testLog(t)})
	require.NoError(t, err)
	rows, err := a.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows) // exactly the fake's zero rows; nothing else could have contributed
}

// Symmetric: Billing has no AnalyticsRepository field at all.
func TestBillingPathCannotReachAnAggregate(t *testing.T) {
	b, err := consumption.NewBilling(consumption.BillingDeps{Readings: fakeReadings{}, Anomalies: noAnomalies{}, Ops: noOps{}, Clock: clock.NewFake(now), Log: testLog(t)})
	require.NoError(t, err)
	_, err = b.Consumption(ctx, scope, req)
	require.NoError(t, err) // only fakeReadings was ever called; there is no aggregate dependency to have reached
}

// 09 §F3 acceptance: the two paths differ at a bucket boundary by the expected step.
func TestBillingAndAnalyticsDifferByTheBucketBoundaryStep(t *testing.T) {
	// Readings at 09:00=1000, 09:45=1030, 10:00=1040 (the 09:45->10:00 step is 10).
	// analytics (last-first inside the 09:00 bucket) = 1030-1000 = 30
	// billing   (boundary 09:00 -> boundary 10:00)   = 1040-1000 = 40
	require.Equal(t, "30", analyticsRow.Values[energy.ActiveImport].String())
	require.Equal(t, "40", billingRow.Values[energy.ActiveImport].String())
	require.Equal(t, "10", sub(billingRow, analyticsRow).String(), "exactly the step the aggregate misses")
}

// C-8 / 09 §F3 acceptance: levels derive from their own boundaries, never by
// summing below. Hourly readings for one Istanbul day, 00:00 through 24:00
// (25 timestamps, 24 hourly windows), fully specified so the daily figure and
// the sum of the sound hourly rows are both exact:
//
//	00:00=1000 01:00=1010 02:00=1020 03:00=1030 04:00=1040 05:00=1050
//	06:00=1060 07:00=1070 08:00=1080 09:00=1090 10:00=1100 11:00=1050
//	12:00=1160 13:00=1170 14:00=1180 15:00=1190 16:00=1200 17:00=1205
//	18:00=1210 19:00=1215 20:00=1220 21:00=1225 22:00=1230 23:00=1235
//	24:00=1240
//
// Hour 10 ([10:00,11:00)) is 1050-1100 = -50: a negative delta with no
// covering reset, so that hourly row is suspect (nil). Every other hourly
// window is non-negative. The daily window ([00:00,24:00)) derives directly
// from its own two boundaries: 1240-1000 = 240 — untouched by hour 10's
// problem, because the daily row never sums the hours below it.
//
// Sum of the 23 non-suspect hourly rows: telescoping sum of all 24 windows'
// deltas always equals the daily total (240) regardless of intermediate
// values, so excluding hour 10's own delta (-50) from that total gives
// 240 - (-50) = 290. 290 != 240 is the assertion that summing the level below
// is NOT how the daily figure is produced.
func TestDailyDoesNotEqualTheSumOfItsHoursWhenOneHourIsSuspect(t *testing.T) {
	hourly, daily := billingHourlyAndDailyFor(t, dailyFixtureReadings) // the 25 readings above
	require.Nil(t, hourly[10].Values[energy.ActiveImport], "hour 10's negative delta with no reset is suspect")
	require.Equal(t, "240", daily.Values[energy.ActiveImport].String())
	require.Equal(t, "290", sumOfNonNilHourly(hourly, energy.ActiveImport).String())
	require.NotEqual(t, "240", sumOfNonNilHourly(hourly, energy.ActiveImport).String())
}

func TestMonthlyPrefersBillingKindReadingsWhenPresent(t *testing.T) {
	require.Equal(t, energy.KindBilling, row.Source)
	require.Equal(t, "5000", row.Values[energy.ActiveImport].String(), "the utility snapshot, not the load profile")
}

func TestMonthlyFallsBackToLoadProfileWhenBillingReadingsDoNotCoverTheMonth(t *testing.T) { /* R63: one billing reading only */ }

// I-3: a billing snapshot that exists but is stale does not count as covering.
func TestMonthlyFallsBackWhenTheOnlyBillingSnapshotIsOlderThanTheTolerance(t *testing.T) {
	// January's billing-kind boundary reading is dated 4 days (>72h) before
	// month-end: it exists, but it is stale, so the month falls back to
	// load_profile exactly as if no billing reading existed at all.
	require.Equal(t, energy.KindLoadProfile, row.Source)
}

// R88: the current (still-open) month is composed from consumption_daily
// closing indexes, never from meter_readings.
func TestOpenMonthIsComposedFromDailyClosingIndexes(t *testing.T) {
	rows, err := analytics.Consumption(ctx, scope, consumption.SeriesRequest{AnalyzerIDs: ids, Level: energy.Monthly, Range: rangeCoveringTheOpenMonth})
	require.NoError(t, err)
	open := rows[len(rows)-1]
	require.True(t, open.Partial)
	require.Equal(t, "expectedOpenMonthValue", open.Values[energy.ActiveImport].String())
}

func TestBillingRefusesAnEmptyAnalyzerList(t *testing.T)                                  { /* fail-closed, no rows, no query */ }
func TestBillingRefusesARequestOverTheMaxBucketsCap(t *testing.T)                          { /* ErrInvalidRequest, no I/O */ }
func TestBillingIsolatesTenants(t *testing.T)                                             { /* the other tenant's AdminScope + a positive control */ }

// I-11: every register round-trips, and the multiplier is read for audit only.
func TestConvertRoundTripsEveryRegisterAndNeverReappliesTheMultiplier(t *testing.T) {
	for _, reg := range energy.AllRegisters() {
		mr := model.MeterReading{Kind: model.ReadingKindLoadProfile, MultiplierApplied: 10}
		setRegister(&mr, reg, dec("123.4500"))
		er := toEnergyReading(mr)
		require.Equal(t, "123.45", er.Value(reg).String(), "MultiplierApplied=10 leaves the value unchanged — it was applied once, at ingestion")
	}
	require.Equal(t, model.ReadingKindBilling, toModelKind(energy.KindBilling))
}
```

- [ ] **Step 2: Run and watch them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run and watch them pass.** Unit first, then `go test ./internal/service/... -tags=integration -race -count=1`.
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestAnalyticsPathCannotReachTheHypertable` | add a `Readings store.ReadingRepository` field to `AnalyticsDeps` and a fallback that reads it when the aggregate fake returns no rows (which it always does here, I-8): expect FAIL — this is exactly the shape R61 forbids; the guard is the type definition itself plus this test failing to compile/pass without the forbidden field. Restore. |
| `TestBillingAndAnalyticsDifferByTheBucketBoundaryStep` | make `Billing.Consumption` read the aggregate: expect FAIL — the difference collapses to 0. Restore. |
| `TestDailyDoesNotEqualTheSumOfItsHoursWhenOneHourIsSuspect` | compute the daily row as the sum of the hourly `Billing` rows instead of from its own two boundaries: expect FAIL — `290` appears where `240` is wanted. Restore. |
| `TestMonthlyPrefersBillingKindReadingsWhenPresent` | drop the billing-kind attempt: expect FAIL — `Source` is `energy.KindLoadProfile`. Restore. |
| `TestMonthlyFallsBackWhenTheOnlyBillingSnapshotIsOlderThanTheTolerance` | drop the `BillingSnapshotTolerance` check (treat "reading exists" as "covers"): expect FAIL — `Source` is `energy.KindBilling` with a stale value. Restore. |
| `TestOpenMonthIsComposedFromDailyClosingIndexes` | read `meter_readings` for the open period instead of composing from `consumption_daily`: expect FAIL from a poisoned/failing `Readings` fake on the `Analytics` type (which has none — the mutation does not compile without adding one, which is itself the violation). Restore. |
| `TestBillingIsolatesTenants` | remove the scope argument from one repository call: expect FAIL — the other tenant's rows appear. Restore. |
| `TestConvertRoundTripsEveryRegisterAndNeverReappliesTheMultiplier` | multiply the value by `MultiplierApplied` again in `toEnergyReading`: expect FAIL — `1234.5` appears instead of `123.45`. Restore. |

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
- Modify: `internal/service/consumption/billing.go` (add the resolved-`manual_override` override-substitution read to `Billing.Consumption` and populate `Row.Resolution` — C-6; `billing.go` is Task 7's file, already merged by the time this task runs, in a later wave)
- Test: `internal/service/consumption/anomalies_integration_test.go`

**Interfaces:**
- Consumes: Task 7's `Billing`, `BillingDeps`, `Row`; `store.AnomalyRepository`, `store.OpsRepository`, `store.ReadingRepository`; `internal/platform/lock` (`lock.NewRedis(client)`, no lease renewal — Acquire/Release only); F2's `internal/ingest/anomaly.go` (for I-5's overlap resolution).
- Produces:

```go
// ConsumptionAndRecord runs Consumption and, for every suspect register it
// finds, writes one consumption_anomalies row and one operator message per
// period (R59, R60, C-4). It is the only method on Billing that writes.
// Re-running it for the same period does not create a second row for the
// same (analyzer, period_start, period_end, reason) — C-6.
func (b *Billing) ConsumptionAndRecord(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error)

type AnomalyListRequest struct {
	AnalyzerIDs []uuid.UUID
	Unresolved  bool
	Range       *store.TimeRange
	Page        store.Page
}

func (b *Billing) ListAnomalies(ctx context.Context, sc store.Scope, req AnomalyListRequest) ([]model.ConsumptionAnomaly, error)

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
	ResetAfter map[energy.Register]decimal.Decimal // the meter's values after the reset; a register absent/nil here carries no reset evidence for that register (I-16)
	Overrides  map[energy.Register]decimal.Decimal // required for ResolveByOverride
}

// ResolveAnomaly also resolves any F2 ingestion negative_delta anomaly for the
// same analyzer whose [period_start, period_end] lies within this F3 period,
// applying the SAME resolution to both (I-5) — an operator registering a
// reset for a billing period should not separately have to clear the
// ingestion-time row F2 wrote for the same event.
func (b *Billing) ResolveAnomaly(ctx context.Context, sc store.Scope, id, resolvedBy uuid.UUID, r Resolution) (model.ConsumptionAnomaly, error)
```

**Rules the implementation must follow:**
- `detail` and the operator message are exactly R60/C-4's shape: one `OpsRepository.AppendMessage` call with `model.OperationalMessage{Kind: "system", Category: "consumption-suspect-period", Status: "error", Message: <sentence>, RelatedType: ptr("consumption_anomaly"), RelatedID: &anomaly.ID, Metadata: <detail>}`. There is no `Code` field and no level enum on `OperationalMessage` — assert `Category` and `Status == "error"`, never a `Code` or `Level` field.
- **Dedup (C-6):** before creating, list BOTH resolved and unresolved anomalies for `(analyzer, period_start, period_end, reason)` — matching F2's `negativeDeltaAnomalyExists` exactly, not just unresolved rows — and skip creation when one already exists. Serialise the check-then-create per `(analyzer, period_start)` with `platform/lock` (`lock.NewRedis`, TTL 30s): the partial index on the table is NOT unique, so no database-level uniqueness is claimed; the lock is what actually prevents two concurrent runs from creating two rows. Assert the behaviour with a concurrent test.
- `Billing.Consumption` (Task 7's `billing.go`, extended here) reads resolved `manual_override` anomalies covering the requested window and substitutes `override_values` for those registers in the row's `Values`, recording `Row.Resolution[register] = "manual_override"`. A resolved `accepted` anomaly leaves `Values` nil for its registers (still no number) but still records `Row.Resolution[register] = "accepted"`, so a caller can badge either kind without inferring it from the anomaly table separately.
- `ResolveByRegisteringReset` writes the reset reading through `Readings.BulkInsert` with `Kind: reset`, `SourceProvider` = the analyzer's own provider and `MultiplierApplied = 1` (I-16: an operator-entered value is already real, never re-multiplied) **and** marks the anomaly resolved; if the insert fails, the anomaly is not marked resolved. A register absent or nil in `ResetAfter` carries no reset evidence for that register — it is still derived by plain difference afterward and is suspect only if that difference is itself negative (I-16).
- `ResolveByOverride` validates that every supplied register is one of the anomaly's suspect registers; an unknown register is `consumption.ErrInvalidRequest` (pre-flight: `store.ErrValidation` does not exist).
- Resolution is scoped: resolving another tenant's anomaly is `store.ErrNotFound`, never a permission error that reveals existence.

- [ ] **Step 1: Write the failing tests**

```go
func TestASuspectPeriodWritesAnAnomalyAndAnOperatorMessage(t *testing.T) {
	rows, err := billing.ConsumptionAndRecord(ctx, ten.Scope, req)
	require.NoError(t, err)
	require.Nil(t, rows[0].Values[energy.ActiveImport], "never a number")

	an, _ := billing.ListAnomalies(ctx, ten.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: ids, Unresolved: true})
	require.Len(t, an, 1)
	require.Equal(t, "negative_delta", an[0].Reason)
	require.JSONEq(t, `{"code":"consumption.suspect_period","period_start":"...","period_end":"...","registers":{"active_import":{"reason":"negative_delta","delta":"-1234.5","reset_rows":0}}}`, string(an[0].Detail))

	msgs, _ := ops.ListMessages(ctx, ten.AdminScope, /* … */)
	require.Equal(t, "consumption-suspect-period", msgs[0].Category)
	require.Equal(t, "error", msgs[0].Status)
}

func TestRerunningTheSamePeriodDoesNotCreateASecondAnomaly(t *testing.T) { /* call twice, still 1 */ }
func TestConcurrentRunsForTheSamePeriodStillCreateOnlyOneAnomaly(t *testing.T) { /* two goroutines, platform/lock serializes, still 1 */ }

func TestRegisteringAResetMakesTheNextDerivationSound(t *testing.T) {
	// Resolve with the reset the meter actually had; re-run Consumption; the period
	// now derives a number and is no longer suspect.
	require.Equal(t, "140", after[0].Values[energy.ActiveImport].String())
	require.Empty(t, after[0].Suspect)
}

func TestAnOverrideMustNameASuspectRegister(t *testing.T)       { /* consumption.ErrInvalidRequest */ }
func TestResolvingAnotherTenantsAnomalyIsNotFound(t *testing.T) { /* AdminScope of tenant B + positive control */ }
func TestAcceptedLeavesTheValueNullButClearsTheBlock(t *testing.T) { /* resolved_at set, values still nil, Row.Resolution["active_import"]=="accepted" */ }

// C-6: Billing.Consumption itself, not just ConsumptionAndRecord, reflects a
// resolved override.
func TestConsumptionSubstitutesAResolvedManualOverride(t *testing.T) {
	rows, err := billing.Consumption(ctx, ten.Scope, req) // NOT ConsumptionAndRecord
	require.NoError(t, err)
	require.Equal(t, "999.0000", rows[0].Values[energy.ActiveImport].String())
	require.Equal(t, "manual_override", rows[0].Resolution[energy.ActiveImport])
}

// I-5: resolving an F3 period also clears F2's own ingestion-time row for the
// same event.
func TestResolvingAnF3PeriodAlsoResolvesTheOverlappingF2IngestionAnomaly(t *testing.T) {
	// F2's negativeDeltaAnomalyExists wrote a negative_delta row at ingestion
	// for the same analyzer, with PeriodStart/PeriodEnd inside the F3 period
	// being resolved here.
	_, err := billing.ResolveAnomaly(ctx, ten.Scope, f3AnomalyID, operatorID, consumption.Resolution{Mode: consumption.ResolveByRegisteringReset, ResetTS: &resetTS, ResetAfter: after})
	require.NoError(t, err)
	f2Anomaly, _ := anomalyRepo.Get(ctx, ten.Scope, f2AnomalyID)
	require.NotNil(t, f2Anomaly.ResolvedAt, "F2's ingestion-time row for the same event is resolved too")
}
```

- [ ] **Step 2: Run and watch them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run and watch them pass.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestASuspectPeriodWritesAnAnomalyAndAnOperatorMessage` | write the anomaly but skip the operator message: expect FAIL. Restore. |
| `TestRerunningTheSamePeriodDoesNotCreateASecondAnomaly` | dedup against unresolved rows only, not resolved+unresolved: expect FAIL once the first run's anomaly is resolved and a second run recreates it. Restore. |
| `TestConcurrentRunsForTheSamePeriodStillCreateOnlyOneAnomaly` | drop the `platform/lock` acquisition: expect FAIL with 2 rows from the race. Restore. |
| `TestRegisteringAResetMakesTheNextDerivationSound` | mark the anomaly resolved without inserting the reset reading: expect FAIL — the re-run is still suspect. Restore. |
| `TestConsumptionSubstitutesAResolvedManualOverride` | read overrides only in `ConsumptionAndRecord`, not in `Consumption`: expect FAIL — `Consumption` alone still returns the suspect `nil`. Restore. |
| `TestResolvingAnF3PeriodAlsoResolvesTheOverlappingF2IngestionAnomaly` | resolve only the F3 row: expect FAIL — the F2 row's `ResolvedAt` stays nil. Restore. |
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
- Consumes: Task 7's `Row` (with `Indexes` already populated by `Analytics.Consumption`/`Billing.Consumption`), `SeriesRequest`.
- Produces: **pure helpers over `[]Row` — no `Service`, no `Path` enum, no I/O (M-14, I-2).** The caller already chose `Analytics.Consumption` or `Billing.Consumption` and passes the resulting rows; a `Path` parameter would let a caller accidentally summarise billing numbers from the aggregates by picking the wrong enum value, so there is nothing to pick — the type of the value the caller already has settles it.

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

// Summarise totals rows the caller already fetched from EITHER path. A
// register reported only by suspect rows has a nil Total (never a zero
// standing in for "no data").
func Summarise(rows []Row) Summary

// GenerationRows narrows a series to the export registers only (05 §5
// /generation, R78) — same Row shape, same Window per element, Values
// limited to ActiveExport/ReactiveInductiveExport/ReactiveCapacitiveExport/
// T1Export/T2Export/T3Export. No new fetch: the caller already has the rows.
func GenerationRows(rows []Row) []Row

// BalanceRow is 05 §5 /energy-balance, derived from registers only.
// Inverter-based self-consumption is F9 and is not approximated here (R78).
// grid_import is identically consumption and grid_export is identically
// generation until F9 (R78) — four names over two register reads.
type BalanceRow struct {
	Window      energy.Window
	Consumption *decimal.Decimal // active_import
	Generation  *decimal.Decimal // active_export
	GridImport  *decimal.Decimal // == Consumption
	GridExport  *decimal.Decimal // == Generation
}

// Balance derives one BalanceRow per input Row — active_import and
// active_export are already both present in Row.Values, so this needs no
// second fetch either.
func Balance(rows []Row) []BalanceRow

// ExportRows renders a series as ordered columns and string cells for
// /consumption/export. F3 renders no file: the CSV/XLSX encoder is F6/F8 (R77).
type Export struct {
	Columns []string
	Rows    [][]string // decimals as unrounded strings; an unavailable value is ""
}

func ExportRows(rows []Row) Export
```

- [ ] **Step 1: Write the failing tests**

```go
func TestSummariseLeavesATotalNilWhenNoRowReportedTheRegister(t *testing.T)
func TestSummariseGivesANilTotalWhenARegisterIsReportedOnlyBySuspectRows(t *testing.T) {
	// Every row that carries active_import is suspect for it; every other row
	// simply never reports it. The total must be nil, not the sum of the
	// sound registers' worth of zero contribution from the suspect rows.
	rows := []consumption.Row{
		{Values: map[energy.Register]*decimal.Decimal{}, Suspect: map[energy.Register]energy.Suspicion{energy.ActiveImport: {Reason: energy.ReasonNegativeDelta}}},
		{Values: map[energy.Register]*decimal.Decimal{}, Suspect: map[energy.Register]energy.Suspicion{energy.ActiveImport: {Reason: energy.ReasonNegativeDelta}}},
	}
	s := consumption.Summarise(rows)
	require.Nil(t, s.Totals[energy.ActiveImport])
	require.Equal(t, 2, s.Suspect)
}
func TestSummariseCountsSuspectRowsAndExcludesThemFromTotals(t *testing.T)
func TestPeakAndValleyIgnoreSuspectRows(t *testing.T)
func TestGenerationRowsReadsTheExportRegistersOnly(t *testing.T)
func TestBalanceDerivesFromRegistersOnly(t *testing.T)
func TestExportColumnsAreStableAndUnrounded(t *testing.T) {
	// I-8: 12.2525 (4 decimal places) — "12.25" would not observably change
	// under a "round to 3 decimals" mutation, so it would not prove anything.
	e := consumption.ExportRows(rows)
	require.Equal(t, []string{"analyzer_id", "period_start", "period_end", "active_import", /* … */}, e.Columns)
	require.Equal(t, "12.2525", e.Rows[0][3])
	require.Equal(t, "", e.Rows[1][3], "an unavailable value is empty, never 0")
}
```

- [ ] **Step 2-4: Red, implement, green.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestSummariseGivesANilTotalWhenARegisterIsReportedOnlyBySuspectRows` | treat a suspect row's nil value as zero when summing: expect FAIL — the total becomes `"0"` instead of nil. Restore. |
| `TestSummariseCountsSuspectRowsAndExcludesThemFromTotals` | count a row as suspect only when EVERY register is suspect: expect FAIL on `Suspect`. (Adding suspect rows' nil values as zero is a numeric no-op on a mixed fixture; that mutation is caught by the all-suspect-register nil-total test instead.) Restore. |
| `TestExportColumnsAreStableAndUnrounded` | render an unavailable value as `0`: expect FAIL. Restore. |
| `TestExportColumnsAreStableAndUnrounded` | round to 3 decimals: expect FAIL — `"12.253"` (or `"12.252"`) ≠ `"12.2525"` (I-8: the fixture carries a 4th decimal digit precisely so this mutation is observable). Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
git add internal/service/consumption
git commit -m "feat(f3): task-9 — summary, generation, energy balance and export shapes" <TRAILERS>
```

---

## Task 10: `internal/service/loadprofile` — resolving the company's calendar (R69, R70, R87)

**Files:**
- Create: `internal/service/loadprofile/{doc,service}.go`
- Test: `internal/service/loadprofile/service_integration_test.go`

**Interfaces:**
- Consumes: Task 5's `loadprofile` domain package; `store.AnalyticsRepository` DIRECTLY (R87 — not Task 7's `consumption.Analytics`/`Row`, whose `Values[ActiveImport]` is the bucket's own `active_consumption`, the wrong figure for a load profile, see below); `store.CalendarRepository`.
- Produces:

```go
package loadprofile

// DefaultWeekendDays is applied when a company has configured no
// company_weekend_days rows (R69; 01-project-context.md's stated default).
var DefaultWeekendDays = map[time.Weekday]bool{time.Saturday: true, time.Sunday: true}

type Deps struct {
	Calendar    store.CalendarRepository
	Hourly      HourlySource // raw consumption_hourly buckets, narrowed
	Location    *time.Location
	Log         *slog.Logger
}

// HourlySource is the narrow interface over store.AnalyticsRepository this
// package needs (R87): raw consumption_hourly buckets, so this package can
// read each bucket's OWN ActiveImportStart rather than internal/service/
// consumption's Row.Values, which holds the bucket's active_consumption
// (last-first inside the bucket) — structurally ZERO for a meter that reports
// once per hour and the wrong figure even for a 15-minute meter.
type HourlySource interface {
	ConsumptionHourly(ctx context.Context, sc store.Scope, analyzerIDs []uuid.UUID, r store.TimeRange) ([]model.ConsumptionBucket, error)
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
- **Hourly values, per R87:** fetch `consumption_hourly` buckets for the request range plus one trailing hour (so the last requested hour has a successor to difference against), sort by `Bucket` ascending, and for each pair of buckets exactly one hour apart compute `ActiveImportStart(h+1) − ActiveImportStart(h)`. The value is `nil` when either bucket is missing or the difference is negative — this analysis-side computation never writes an anomaly (only the billing path does, R58). This equals §3.1 boundary differencing whenever the meter reports on the hour; it is NEVER the bucket's own `ActiveConsumption` column (structurally zero for a 1-hour meter, ~25% low for a 15-minute one — R87's own rationale). **Q6** notes the `/analytics` chart endpoint itself is unaffected by this fix and still shows the zeroed figure for a 1-hour meter — that is a separate, raised defect, not something this task corrects.
- Feed the domain package the R87 hourly values as active-import consumption only (R68) — the package never names a register.
- A missing or negative-delta hour contributes nothing; it is not a zero hour.
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

// PROVISIONAL: revisit when F6 rules on Q4 (R70's guard test name and this
// comment both flag it — F6 line 434's acceptance wording implies a
// calendar-event-driven split that this test currently forbids).
func TestACalendarEventDoesNotChangeTheSplitUnderR70(t *testing.T) { /* R70 */ }
func TestProfilesAreTenantIsolated(t *testing.T)           { /* other tenant's AdminScope + positive control */ }
func TestStatisticsMatchTheHandComputedFixture(t *testing.T) { /* 09 §F3 acceptance */ }

// R87: hour h's value is ActiveImportStart(h+1) - ActiveImportStart(h), never
// the bucket's own ActiveConsumption. A 1-hour-metered analyzer (one reading
// per hour) makes the bug this proves against obvious: ActiveConsumption
// would be zero for every hour of that analyzer.
func TestHourlyValuesUseConsecutiveStartIndexesNotTheBucketsOwnConsumption(t *testing.T) {
	// A 1-hour meter: one reading per hour, e.g. 00:00=1000, 01:00=1010, 02:00=1025.
	// ActiveConsumption for each bucket (last-first inside the bucket) is 0,
	// because the bucket's only reading is both first() and last(). The
	// correct hourly values are hour0=10, hour1=15.
	require.Equal(t, "10", got.Profiles["weekday"].Hours[0].String())
	require.Equal(t, "15", got.Profiles["weekday"].Hours[1].String())
}
```

- [ ] **Step 2-4: Red, implement, green.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestAVacationMovesADayIntoTheWeekendProfile` | ignore vacations when building the config: expect FAIL — the counts do not move. Restore. |
| `TestACompanyWithNoConfiguredWeekendDaysGetsSaturdayAndSunday` | return an empty weekend set instead of the default: expect FAIL — no weekend profile. Restore. |
| `TestACalendarEventDoesNotChangeTheSplitUnderR70` | fold calendar events into the vacation list: expect FAIL. Restore. |
| `TestHourlyValuesUseConsecutiveStartIndexesNotTheBucketsOwnConsumption` | read `bucket.ActiveConsumption` instead of differencing consecutive `ActiveImportStart` values: expect FAIL — both hours read `"0"`. Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
go test ./internal/service/... -tags=integration -race -count=1
git add internal/service/loadprofile
git commit -m "feat(f3): task-10 — load profile service over the company calendar" <TRAILERS>
```

---

## Task 11a: `consumption.refresh` — the task, the handler, `consumption.Refresher` (R71, R72)

**Files:**
- Create: `internal/job/consumption.go`, `internal/service/consumption/refresh.go`
- Modify: `internal/job/task.go`, `internal/worker/wiring.go` (the `job.Handlers.ConsumptionRefresh` registration line only)
- Test: `internal/job/consumption_test.go`, `internal/service/consumption/refresh_integration_test.go`, `internal/worker/wiring_integration_test.go` (invert the existing R17 assertion)

**Interfaces:**
- Consumes: Task 1's `energy.Bucket`; Task 6's `store.AdminAggregateRepository` and `store.ConsumptionViews()`; F2's `job.ConsumptionRefreshPayload`, `job.Handlers`, `job.ClassifyForRetry`, `job.TaskOptions`; `internal/job/integration.go`'s `NewFetchReadingsTask` windowed branch (`internal/job/integration.go:206-221`) as the copy source — **NOT** `NewSyncPricesTask`; `internal/platform/lock` (real constructor `lock.NewRedis(client)`; **no lease-renewal API exists** — pre-flight finding).
- Produces:

```go
// internal/job

// NewConsumptionRefreshTask builds the task. Copy source: NewFetchReadingsTask's
// windowed branch, not NewSyncPricesTask (I-6). The task id is deterministic
// in the expanded window ONLY — no company, no analyzer (R71, I-6) — so a
// burst of analyzers AND companies ingesting the same window collapses to one
// refresh (refresh_continuous_aggregate covers every tenant's buckets in the
// range regardless of who triggered it, R72). Built with asynq.TaskID and NO
// asynq.Retention: the id deduplicates only while pending/active/retrying, so
// a COMPLETED refresh never blocks a later one for the same window (F2's R53
// trap came from Retention keeping a completed task's id reserved). Cost: an
// archived (retries-exhausted) refresh blocks that exact window until the
// archive entry is deleted.
func NewConsumptionRefreshTask(p ConsumptionRefreshPayload, o TaskOptions) (*asynq.Task, error)
func DecodeConsumptionRefresh(t *asynq.Task) (ConsumptionRefreshPayload, error)

// ConsumptionRefreshTaskID is "consumption.refresh:" + hourly-expanded From +
// "/" + To, both RFC3339 UTC (I-6). It is NOT a function of CompanyID or
// AnalyzerID, even though both remain fields on ConsumptionRefreshPayload for
// the journal.
func ConsumptionRefreshTaskID(from, to time.Time) string

// Refresher is the handler seam. internal/job still does not import the
// store. (M-4: this is job.Refresher — the interface a handler satisfies —
// distinct from consumption.Refresher below, the concrete implementation.)
type Refresher interface {
	RefreshConsumption(ctx context.Context, p ConsumptionRefreshPayload) error
}

// Handlers gains:
//   ConsumptionRefresh Refresher
// and Register gains the guarded line:
//   if h.ConsumptionRefresh != nil { mux.HandleFunc(TypeConsumptionRefresh, h.integHandleConsumptionRefresh) }

// internal/service/consumption

// RefreshDeps.LockTTL is a plain field this task sets to a literal
// 10*time.Minute default; Task 11b introduces the actual
// EKOKOD_CONSUMPTION_REFRESH_LOCK_TTL env knob and wires the configured
// value into this same field in worker/wiring.go — a one-line change on top
// of what this task builds, not a new construction path.
type RefreshDeps struct {
	Aggregates store.AdminAggregateRepository
	Ops        store.OpsRepository
	Locker     lock.Locker // lock.NewRedis(client) in production
	LockTTL    time.Duration
	Clock      clock.Clock
	Log        *slog.Logger
}

// Refresher is consumption.Refresher — the concrete implementation
// job.Refresher's interface is satisfied by (M-4).
type Refresher struct{ /* unexported */ }

func NewRefresher(d RefreshDeps) (*Refresher, error)

// RefreshConsumption expands [From, To) to whole buckets per level and
// refreshes the four consumption views finest-first, holding a per-view lock
// (key "consumption.refresh:<view>", TTL RefreshDeps.LockTTL, acquired via
// RefreshDeps.Locker.Acquire — NOT renewed, since internal/platform/lock has
// no renewal API; an expired lease can at worst allow a second, idempotent
// refresh of the same view, pre-flight finding) so two jobs never refresh the
// same view at once (R71), released on every path. It runs outside any
// transaction (R72). A failure on one view aborts the rest and is retryable.
func (r *Refresher) RefreshConsumption(ctx context.Context, p job.ConsumptionRefreshPayload) error
```

**Rules the implementation must follow:**
- Window expansion uses `energy.Bucket(level, ts, loc)` **with the `loc` argument** (M-2 — an earlier draft wrote `Bucket(level, From)`, missing it): the refreshed range for one level is `[energy.Bucket(level, p.From, istanbul).From, energy.Bucket(level, p.To.Add(-time.Nanosecond), istanbul).To)`. A yearly refresh therefore covers whole years — this is correct and is why the finest-first order matters.
- The lock is acquired and released around each view's refresh individually (not once for all four), so one view's failure does not hold a lock on the others.
- `worker/wiring.go`: `job.Handlers` gains `ConsumptionRefresh: consumptionRefresher` (an adapter is NOT needed here — `*consumption.Refresher` satisfies `job.Refresher` directly). The EXISTING `wiring_integration_test.go` assertion (lines 83-84, `require.Empty(t, pattern, "R17: consumption.refresh has no F2 handler")`) is **INVERTED** (I-9): it now asserts the pattern IS registered, since F3 is exactly the phase that gives `consumption.refresh` its handler.

- [ ] **Step 1: Write the failing tests**

```go
func TestConsumptionRefreshTaskIDIsDeterministicInTheWindowOnly(t *testing.T) {
	a := job.ConsumptionRefreshTaskID(from, to)
	require.Equal(t, a, job.ConsumptionRefreshTaskID(from, to))
	require.NotEqual(t, a, job.ConsumptionRefreshTaskID(from, to.Add(time.Hour)))
	// neither the analyzer nor the company is part of the id: two analyzers
	// in two different companies over the same window still collapse to one.
	t1, _ := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{CompanyID: companyA, AnalyzerID: a1, From: from, To: to}, job.TaskOptions{})
	t2, _ := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{CompanyID: companyB, AnalyzerID: a2, From: from, To: to}, job.TaskOptions{})
	require.Equal(t, taskIDOf(t1), taskIDOf(t2))
}

// I-6: no Retention means a COMPLETED refresh never blocks a later one for
// the same window — the F2 R53 trap this ruling exists to avoid repeating.
func TestConsumptionRefreshTaskHasNoRetentionOption(t *testing.T) {
	tk, err := job.NewConsumptionRefreshTask(job.ConsumptionRefreshPayload{From: from, To: to}, job.TaskOptions{})
	require.NoError(t, err)
	require.False(t, hasRetentionOption(tk), "a completed refresh must never block a later one for the same window")
}

func TestRefreshHoldsAPerViewLock(t *testing.T) {
	// Two concurrent RefreshConsumption calls on the same view: the second
	// observes the lock. Uses a deterministic signal, never a sleep.
}

func TestRefreshReleasesTheLockEvenWhenTheRefreshFails(t *testing.T) { /* Aggregates.Refresh returns an error; lock still released */ }
func TestRefreshRefreshesTheFourViewsFinestFirst(t *testing.T)      { /* recorded call order */ }
func TestRefreshExpandsTheRangeToWholeBuckets(t *testing.T)          { /* a 10-minute range refreshes the whole hour/day/month/year */ }
func TestWorkerRegistersTheConsumptionRefreshHandler(t *testing.T)  { /* graph test: removing the wiring line fails it */ }

// I-9: the existing F2 guard is inverted, not deleted — F3 makes the pattern
// it complained about finally true.
func TestConsumptionRefreshNowHasAnF3Handler(t *testing.T) {
	// formerly asserted require.Empty(...) with the comment "R17: consumption.refresh
	// has no F2 handler" (wiring_integration_test.go:83-84). Now asserts the pattern
	// IS present in the built mux.
	require.NotEmpty(t, pattern)
}
```

- [ ] **Step 2-4: Red, implement, green.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestConsumptionRefreshTaskIDIsDeterministicInTheWindowOnly` | put `AnalyzerID` or `CompanyID` into the id: expect FAIL — two analyzers/companies produce two ids. Restore. |
| `TestConsumptionRefreshTaskHasNoRetentionOption` | add `asynq.Retention(30*24*time.Hour)` (copying `NewFetchReadingsTask`'s windowed branch too literally): expect FAIL. Restore. |
| `TestRefreshHoldsAPerViewLock` | drop the lock acquisition: expect FAIL from the concurrency assertion. Restore. |
| `TestRefreshReleasesTheLockEvenWhenTheRefreshFails` | release the lock only on the success path: expect FAIL — the second attempt after a failure deadlocks/times out. Restore. |
| `TestWorkerRegistersTheConsumptionRefreshHandler` | revert `job.Handlers.ConsumptionRefresh` to `nil` in `wiring.go` (I-8: name the field explicitly): expect FAIL. Restore. |
| `TestConsumptionRefreshNowHasAnF3Handler` | revert the wiring: expect FAIL — the inverted assertion catches a regression back to F2's state exactly as the original caught the gap. Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build && make check-generate
go test ./internal/job/... ./internal/worker/... ./internal/service/... -tags=integration -race -count=1
git add internal/job internal/worker internal/service
git commit -m "feat(f3): task-11a — consumption.refresh task, handler and consumption.Refresher" <TRAILERS>
```

---

## Task 11b: the ingest enqueue seam (R73, I-12, I-13)

**Files:**
- Modify: `internal/ingest/fetch.go`, `internal/ingest/deps.go` (comment only), `internal/worker/wiring.go` (the `ingestDeps.ConsumptionRefresh` adapter, and wiring the `EKOKOD_CONSUMPTION_REFRESH_LOCK_TTL` config value into `RefreshDeps.LockTTL` — depends on 11a, never edited concurrently with it), `internal/platform/config` (the refresh knobs)
- Test: `internal/ingest/fetch_refresh_test.go`
- Also touches: `.env.example` (M-6)

**Interfaces:**
- Consumes: Task 11a's `job.NewConsumptionRefreshTask`, `job.Refresher`; F2's `ingest.Deps.ConsumptionRefresh`, `fetchAccumulator` (its `affectedFrom`/`affectedTo` fields — pre-flight finding: these are `fetchAccumulator` fields, not a separate type), `Service.failFetchRun` (`internal/ingest/fetch.go:405`).
- Produces: no new exported type — this task wires an existing seam and adds two config knobs.

```go
// internal/platform/config

// EKOKOD_CONSUMPTION_REFRESH_ENABLED gates the enqueue call site added below.
// Default true.
ConsumptionRefreshEnabled bool

// EKOKOD_CONSUMPTION_REFRESH_LOCK_TTL is wired into
// consumption.RefreshDeps.LockTTL (Task 11a) in worker/wiring.go. Default 10m.
ConsumptionRefreshLockTTL time.Duration
```

**Rules the implementation must follow:**
- **Both finish paths enqueue (I-12):** `ingest.Service.FetchReadings` enqueues `consumption.refresh` after a normal `finishRun` success, AND `failFetchRun` (`internal/ingest/fetch.go:405`) also enqueues when `acc.affectedFrom != nil` — a partial run that persisted some rows before failing still touched real buckets that need refreshing. Both call sites share one unexported helper so the threshold and gating logic live in exactly one place.
- **Threshold (R73, amended by I-13):** enqueue only when `deps.ConsumptionRefresh != nil`, `config.ConsumptionRefreshEnabled`, the run persisted rows (or, for the `failFetchRun` path, `acc.affectedFrom != nil`), AND `acc.affectedFrom` is earlier than `now − 30 days` — `consumption_hourly`'s `start_offset`, the smallest of the four policy windows. Live data (an affected range entirely inside the last 30 days) needs no refresh: the real-time union already serves it. Tests cover both sides of the threshold.
- An empty affected range never enqueues, on either path.
- The window enqueued is the affected range (`acc.affectedFrom`/`acc.affectedTo`), not the requested window, so a backfill of old data refreshes exactly the buckets it touched.
- An enqueue failure is logged and written as a warning message; it never fails the fetch run (which has already succeeded or already failed for its own reason by the time the enqueue call happens).
- `.env.example` gains `EKOKOD_CONSUMPTION_REFRESH_ENABLED` and `EKOKOD_CONSUMPTION_REFRESH_LOCK_TTL`, documented alongside F2's `EKOKOD_INGEST_*` (M-6).
- `worker/wiring.go`: `ingestDeps.ConsumptionRefresh` becomes a small adapter over `*job.Client` that calls `job.NewConsumptionRefreshTask`; the same edit wires `config.ConsumptionRefreshLockTTL` into the `consumption.RefreshDeps.LockTTL` field Task 11a left as a literal default. The existing wiring test that pins the graph is extended, not replaced.

- [ ] **Step 1: Write the failing tests**

```go
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

// I-13: the threshold's two sides.
func TestFetchDoesNotEnqueueWhenTheAffectedRangeIsEntirelyWithinTheLastThirtyDays(t *testing.T) {
	// acc.affectedFrom = now - 10 days: inside consumption_hourly's own
	// real-time window, needs no refresh.
	require.Empty(t, enqueuer.calls)
}
func TestFetchEnqueuesWhenTheAffectedRangeStartsBeforeThirtyDaysAgo(t *testing.T) {
	// acc.affectedFrom = now - 31 days: crosses the threshold.
	require.Len(t, enqueuer.calls, 1)
}

// I-12: the partial-run path enqueues too.
func TestFailFetchRunStillEnqueuesWhenSomeRowsWerePersistedBeforeTheFailure(t *testing.T) {
	// acc.affectedFrom is non-nil (some chunks succeeded) when a later chunk
	// fails and failFetchRun is called.
	require.Len(t, enqueuer.calls, 1)
}
func TestFailFetchRunDoesNotEnqueueWhenNothingWasEverPersisted(t *testing.T) {
	require.Empty(t, enqueuer.calls) // acc.affectedFrom is nil: the run failed before persisting anything
}

func TestAnEnqueueFailureDoesNotFailTheFetchRun(t *testing.T) { /* warning message written, run still success */ }
func TestConsumptionRefreshEnabledFalseDisablesTheEnqueueEntirely(t *testing.T) { /* config gate */ }
```

- [ ] **Step 2-4: Red, implement, green.**
- [ ] **Step 5: Prove the guards fail.**

| Test | Mutation |
|---|---|
| `TestABackfillTwoHundredDaysOldEntersTheAggregateAfterTheJobRuns` | make the handler refresh only `[now-30d, now)`: expect FAIL — the old bucket never appears. Restore. |
| `TestFetchEnqueuesExactlyOneRefreshForTheAffectedRange` | enqueue the requested window instead of the affected range: expect FAIL. Restore. |
| `TestFetchDoesNotEnqueueWhenTheAffectedRangeIsEntirelyWithinTheLastThirtyDays` | drop the 30-day threshold (enqueue on any non-empty affected range): expect FAIL — a call appears for live data. Restore. |
| `TestFailFetchRunStillEnqueuesWhenSomeRowsWerePersistedBeforeTheFailure` | enqueue only from the success path, not `failFetchRun`: expect FAIL — no call recorded. Restore. |
| `TestAnEnqueueFailureDoesNotFailTheFetchRun` | propagate the enqueue error as the run's own error: expect FAIL — the run reports failure despite a successful fetch. Restore. |

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build && make check-generate
go test ./internal/ingest/... ./internal/worker/... -tags=integration -race -count=1
git add internal/ingest internal/worker internal/platform .env.example
git commit -m "feat(f3): task-11b — ingest enqueue seam, R73 threshold and config knobs" <TRAILERS>
```

---

## Task 12: The PM5340 recompute becomes a bounded queued task (F2 carry-forward, R74)

Withdrawn — R74 deferred to F9.

---

## Task 13: Acceptance suite, the F3 verification block and the handoff

**Files:**
- Create: `internal/service/consumption/f3_acceptance_test.go`, `internal/service/loadprofile/f3_acceptance_test.go` — **in-package, under `//go:build integration`** (M-5: no `internal/acceptance` package; F1/F2's `internal/acceptance` pattern is NOT used in F3 — the suite lives next to the code it exercises, matching how every other integration test in this phase is already built).
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
| Negative delta without a reset → NULL + suspect + message | `TestF3NegativeDeltaProducesNullSuspectAndMessage` |
| Each level derives from its own boundaries | `TestF3LevelsDeriveFromTheirOwnBoundaries` |
| billing vs analytics differ by the expected step | `TestF3BillingAndAnalyticsDifferByTheBoundaryStep` |
| Monthly prefers billing-kind readings | `TestF3MonthlyPrefersBillingReadings` |
| Load profile statistics match hand-computed values | `TestF3LoadProfileStatisticsMatchHandComputedValues` |
| A vacation moves a date weekday → weekend | `TestF3VacationMovesADayToTheWeekendProfile` |
| Zero consumption → null ratio | `TestF3ZeroConsumptionProducesANullRatio` |

M-5 also drops `TestF3GoldenCorpusCoversTheSixCases`: Task 4's `TestGolden` already runs and asserts all six golden files every time `go test ./internal/domain/energy -run TestGolden` runs (part of the phase verification block above); a second, acceptance-layer test that merely re-asserts "the six files exist and run" would be redundant with a test that already fails loudly if a file goes missing or a case regresses.

- [ ] **Step 2-4: Red, implement, green.**
- [ ] **Step 5: Run the full suite twice** (`-count=1`, then again) and record both results; a package failing with `wait until ready` is retried once before being believed.
- [ ] **Step 6: Rewrite `HANDOFF_NEXT_SESSION.md` for F4** — state, rulings R54-R89, what F4 must pick up (max demand now real, R85's invoice consequence; the open product-owner questions Q1-Q8, with Q4 escalated as an F6 blocker; Task 12 withdrawn, R74 deferred to F9; unresolved anomalies blocking invoices), the environment section, and a paste-ready prompt. Commit.

```bash
make lint && make test && make build && make check-generate
go test ./... -tags=integration -race -count=1 -parallel 8
git add internal/service/consumption internal/service/loadprofile HANDOFF_NEXT_SESSION.md docs
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
- **The PM5340 post-persist recompute becoming a bounded queued task** (Task 12, WITHDRAWN) — F9, alongside that phase's solar generation work, per R74's re-ruling. The inline recompute (already batched at `writeBatchSize = 5000`) is unchanged in F3; a queued continuation needs a pending-marker protocol so inline hooks do not read a stale base while a continuation is mid-flight, and that protocol belongs with F9.
- **F2's deferred minors** (`final-review-A-report.md`, `final-review-B-report.md`: warning `Detail` dropped by the pipeline, `NextCursor` doc inconsistency, EPİAŞ TGT re-login per process, OSOS login per fetch, EPİAŞ PTF chunk-seam overlap) — triage only when an F3 task opens the same file.

---

## Self-review

### Acceptance criteria → tests (09 §F3)

| Criterion | Proven by |
|---|---|
| Golden-file tests: normal, missing readings, identical boundaries, reset with event, reset without event, rollover | Task 4's six `testdata/golden/*.json` + `TestGolden` (M-5: no separate acceptance-layer restatement — `TestGolden` already fails loudly on any of the six) |
| Negative delta without a reset → NULL, suspect, operator message, asserted | Task 2 `TestDeriveWithoutAResetEventIsSuspect`; Task 8 `TestASuspectPeriodWritesAnAnomalyAndAnOperatorMessage`; Task 13 `TestF3NegativeDeltaProducesNullSuspectAndMessage` |
| Every level derives from its own boundaries; summing below is asserted not to be how | Task 7 `TestDailyDoesNotEqualTheSumOfItsHoursWhenOneHourIsSuspect` (C-8); Task 13 `TestF3LevelsDeriveFromTheirOwnBoundaries` |
| billing vs analytics differ at a bucket boundary by the expected step | Task 7 `TestBillingAndAnalyticsDifferByTheBucketBoundaryStep`; Task 13 `TestF3BillingAndAnalyticsDifferByTheBoundaryStep` |
| Monthly prefers `billing`-kind readings | Task 7 `TestMonthlyPrefersBillingKindReadingsWhenPresent` + the staleness-bounded fallback test (R63, I-3); Task 13 `TestF3MonthlyPrefersBillingReadings` |
| Load profile statistics match hand-computed values | Task 5 `TestStatsMatchHandComputedValues`; Task 10 `TestStatisticsMatchTheHandComputedFixture`; Task 13 |
| Vacation days move a date weekday → weekend | Task 10 `TestAVacationMovesADayIntoTheWeekendProfile`; Task 13 |
| Zero consumption → null ratio, not a substituted 1 | Task 3 `TestRatioIsNilWhenConsumptionIsZero` (mutation: substitute 1, the literal legacy defect); Task 13 |

### Scope coverage (09 §F3 bullets)

`internal/domain/energy` §2-§3 → Tasks 1-4. The two explicit consumption paths → Task 7 (`Analytics`/`Billing`, R61). `consumption.refresh` triggered by ingestion over the affected range → Tasks 11a (task/handler/refresher) and 11b (the enqueue seam and the R73 threshold). Suspect-period creation, listing and operator resolution → Task 8. `internal/domain/loadprofile` §10.3 with the company's vacation configuration → Tasks 5 and 10. API endpoints from 05 §5 → the service methods backing all ten (Tasks 7, 8, 9, 10), with the HTTP layer deliberately deferred to F6 (R77). Task 12 (the PM5340 recompute becoming a bounded queued task) is WITHDRAWN — R74 deferred to F9.

### Guards added, each with its proving mutation

`TestServiceLayerImportBoundaries` (probe importing `internal/ingest`); the extended `TestStoreAndIntegrationDoNotImportAPIOrService` (probe importing `internal/service`, R76); the float guard over `./internal/service/...` (a `float64` field in a probe); `TestAnalyticsPathCannotReachTheHypertable` / `TestBillingPathCannotReachAnAggregate` (R61's structural separation — no other-repository field exists to poison); `TestSelectBoundaryExcludesAReadingOneNanosecondAfterTheBound` (`<` vs `<=`); `TestRefreshMaterialisesAClosedMonthlyBucketTwoYearsOld` and `TestRefreshMaterialisesAnHourlyBucketBelowAWatermarkThatHasAlreadyMoved` (no-op refresh, C-2's two proofs); `TestRefreshRejectsAnUnknownViewBeforeTouchingTheDatabase` (`fmt.Sprintf` statement building, against a closed pool); `TestDailyDoesNotEqualTheSumOfItsHoursWhenOneHourIsSuspect` (compute the daily row as the sum of the hourly rows, C-8); `TestMonthlyFallsBackWhenTheOnlyBillingSnapshotIsOlderThanTheTolerance` (drop the staleness check, I-3); `TestOpenMonthIsComposedFromDailyClosingIndexes` (read `meter_readings` for the open period, R88); `TestRerunningTheSamePeriodDoesNotCreateASecondAnomaly` and `TestConcurrentRunsForTheSamePeriodStillCreateOnlyOneAnomaly` (dedup and the `platform/lock` serialisation removed, C-6); `TestConsumptionSubstitutesAResolvedManualOverride` (override read only in the record-and-write wrapper, C-6); `TestResolvingAnF3PeriodAlsoResolvesTheOverlappingF2IngestionAnomaly` (I-5's overlap dropped); `TestRegisteringAResetMakesTheNextDerivationSound` (resolve without writing the reset row); `TestConsumptionRefreshTaskIDIsDeterministicInTheWindowOnly` (analyzer or company in the id, I-6); `TestConsumptionRefreshTaskHasNoRetentionOption` (`asynq.Retention` added back, I-6); `TestRefreshHoldsAPerViewLock` and `TestRefreshReleasesTheLockEvenWhenTheRefreshFails` (lock dropped or leaked); `TestWorkerRegistersTheConsumptionRefreshHandler` and the inverted `TestConsumptionRefreshNowHasAnF3Handler` (wiring reverted to nil, I-9); `TestFetchDoesNotEnqueueWhenTheAffectedRangeIsEntirelyWithinTheLastThirtyDays` (I-13's threshold dropped); `TestFailFetchRunStillEnqueuesWhenSomeRowsWerePersistedBeforeTheFailure` (I-12's second call site dropped); `TestGolden` (one digit edited); `TestRatioIsAnUnroundedFraction` (rounded to 4 places, C-5); `TestMaxDemandCountsBillingRowsAndExcludesCurrentIndexAndReset` (`current_index` included, C-3); `TestStatsMatchHandComputedValues` (n-1 divisor); `TestSeasonOfUsesMeteorologicalMonths` (legacy 21 March cutoff); `TestHourlyValuesUseConsecutiveStartIndexesNotTheBucketsOwnConsumption` (read `ActiveConsumption` instead of differencing starts, R87); `TestDeriveIsSuspectWhenNoReadingPrecedesTheReset` (fall back to `reading_start`, I-1); `TestBillingIsolatesTenants` and `TestResolvingAnotherTenantsAnomalyIsNotFound` (scope removed, with positive controls).

### Type consistency (checked by name across tasks)

`energy.Register`, `energy.Reading`, `energy.Window`, `energy.Level`, `energy.Derivation`, `energy.Suspicion`, `energy.Reason` — produced by Task 1, consumed by 2, 3, 4, 7, 8, 9, 10, 11a. `energy.Derive`, `energy.SelectBoundary` — produced by 2, consumed by 7, 8. `energy.Ratio`/`energy.DivisionScale`/`energy.MaxDemand`/`energy.MaxDemandKinds`/`energy.ActivityStatus` — produced by 3, consumed by 7, 9. `loadprofile.Config`/`Profile`/`Statistics`/`Key`/`DivisionScale` — produced by 5, consumed by 10, 13. `store.AggregateView`/`ConsumptionViews`/`AdminAggregateRepository` — produced by 6, consumed by 11a. `consumption.Analytics`/`Billing`/`Row`/`SeriesRequest`/`ErrInvalidRequest` (no `Service`, no `Path` — R61, M-14) — produced by 7, consumed by 8, 9, 10, 13. `consumption.Refresher`/`RefreshDeps`/`NewRefresher` — produced by 11a, consumed by 11b's wiring. `job.ConsumptionRefreshPayload` (F2) and `job.Refresher`/`NewConsumptionRefreshTask`/`ConsumptionRefreshTaskID` — produced by 11a, consumed by 11b and the worker wiring.

### Known risks, stated rather than hidden

1. **R54 overrides a spec sentence.** If the product owner says the ratio really must be `0`, one field type and the ratio fixtures change; nothing else moves. The conflict is recorded as Q1.
2. **R55's before/after reading rule is inferred**, not stated by the spec. It is the plan's most load-bearing inference; Task 2's fixtures make it explicit and a different ruling is a contained change to `reset.go` plus three golden files.
3. **The refresh covers every tenant's buckets in the window** (R71/R72) — this is a property of TimescaleDB, not a design choice. Under heavy multi-tenant ingestion the refresh load is shared, and the fix round made the dedup key window-only rather than per-company (I-6) precisely because that sharing is correct, not a bug; the deterministic task id is what keeps it from multiplying.
4. **R85 changes money.** Once max demand is real, F4's demand-overrun charge stops being structurally zero. Nothing in F3 bills anything, so the risk is entirely in F4's court — but it must not be discovered there.
5. **`internal/service` is a new layer** the architecture doc does not list (R75). If the spec owner rejects the name, it is a directory rename while the tree is two packages.
6. **The `load_profile` boundary fallback has no staleness bound** (Q7) — unlike R63's 72h `billing`-snapshot tolerance, a `load_profile` boundary reading can be arbitrarily old with no freshness check. Not blocking: no acceptance criterion asks for one.
7. **The analytics hourly chart is all zeros for a 1-hour meter** (Q6, R87) — F3 corrects the LOAD PROFILE's hourly figure but deliberately leaves the `/analytics` endpoint's own aggregate-based hourly figure as §4.3 defines it, because acceptance criterion 4 requires that documented step-loss behaviour. The defect is real and is raised, not silently left for F14 to discover.
8. **An uncovered register rollover becomes operator work** (Q8, R57) — the meter did not fail, it wrapped, but F3 cannot tell the difference from an ordinary negative delta without inventing modulus inference (explicitly out of scope), so an operator must resolve it as if it were a reset.
