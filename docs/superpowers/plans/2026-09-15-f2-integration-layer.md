# F2 — Integration Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Meter data from OSOS, GridBox, ARIL and PM5340, plant data from iSolarCloud, and market prices from EPİAŞ reach the database through one `MeterDataSource` contract and one pipeline. Ingestion is reliable, resumable, idempotent and bounded, and no secret ever reaches a log, an error or an API response.

**Architecture:** Adapters under `internal/integration/<provider>` are pure HTTP clients. They normalise at the boundary and never import the store. Two shared seams sit under them. `internal/integration/httpx` owns timeouts, retries, rate limits, certificate pinning and URL templates. `internal/integration/normalize` owns decimal and time parsing. The pipeline in `internal/ingest` (plus `ingest/generation`, `ingest/backfill` and `ingest/production`) is the only code that writes readings. It works through F1's scoped repositories and uses `internal/store/postgres/admin` for platform-wide work. `internal/credentials` owns the write-only credential surface and iSolarCloud OAuth, and `internal/marketdata` owns the EPİAŞ price sync. `internal/job` declares task types and payloads and depends on handler interfaces only, never on the store. `internal/worker` wires everything for `ekokod worker`, and the scheduler enqueues the cron entries.

**Tech Stack:** Go 1.27.1 · asynq v0.26.0 · pgx/v5 v5.11.0 · sqlc v1.30.0 · goose v3.28.0 · shopspring/decimal v1.4.0 · golang.org/x/time/rate v0.16.0 · go-redis v9 · testify · testcontainers-go v0.44.0 · `net/http/httptest` (TLS)

**Spec:** `docs/rewrite/09-implementation-plan.md` §F2 (binding scope, acceptance and verification). `docs/rewrite/06-integrations.md` (authoritative for provider behaviour). `docs/rewrite/03-target-architecture.md` §2.2, §2.4, §4, §7. `docs/rewrite/02-domain-rules.md` §1–§3, §7.3. `docs/rewrite/04-data-model.md` §3, §4, §11 (authoritative DDL). `docs/rewrite/10-removed-behaviours.md` items 10, 16–24, 27. `docs/rewrite/05-api-contract.md` §15. `docs/rewrite/appendix/legacy-inventory.md` §4–§5.

---

## Global Constraints

Every task's requirements implicitly include this section.

**Authority**
- **`06-integrations.md` is authoritative for provider behaviour; `04-data-model.md` for every column.** Where this plan's prose disagrees with either, the spec wins. Where the spec disagrees with a task's stated acceptance test, the acceptance test wins (F0/F1 standing ruling). Every place the spec is silent is marked **"spec silent — ruling: X"** in the "Spec gaps and rulings" table below. An implementer who finds a new gap stops and reports it; do not improvise (09 §Working agreements).

**Tenancy (F1 lessons, non-negotiable)**
- **`ctx` first, `store.Scope` second** on every service method that touches tenant data, and on every repository method (F1 contract). Validate the scope (`Scope.Valid()`) before any I/O.
- **Background jobs act for a whole company through `store.SystemScope(companyID)`** (Task 5). It is the only scope constructor F2 adds. No code builds `store.Scope{AllBuildings: true}` by hand.
- **Platform-wide work goes only through the `store.Admin*` interfaces** implemented in `internal/store/postgres/admin`. Platform work means the EPİAŞ price sync, the scheduled credential dispatch and platform job journals. Never borrow a tenant's Scope for platform work.
- **`Scope.AllowsBuilding` never validates a stored foreign key.** It answers "may this principal see building X"; it is not a check that a row's `building_id` exists or belongs to the company. That check lives in the SQL of the write.
- **A required positional id list that is empty means NO rows** (`CursorRepository.List`, `AnalyticsRepository.*`). F2 code never passes an empty list expecting "all", and never skips a call because a list is empty and then treats the skip as "all".
- **Batches are all-or-nothing, and duplicate keys inside a batch are refused** (`ReadingRepository.BulkInsert`, `ProductionRepository.BulkInsert`, `AdminMarketDataRepository.*` return `ErrConflict`). The pipeline therefore deduplicates **before** calling them (Task 10 rule D1). It never relies on `on conflict` to arbitrate between two rows of one batch.

**Money, energy and time**
- **Money and energy are `decimal.Decimal`, never `float64`.** The same applies to prices, kWh, kW, multipliers and temperatures that flow into `plant_production`. JSON numbers are decoded with `UseNumber()`/`json.Number` or as strings, never into `float64` or `any`.
- **The float guards are extended to every F2 tree.** This is Task 1, Step 6. `internal/arch` `floatGuardPatterns` and `.golangci.yml` `no-float-money` both gain `./internal/integration/...`, `./internal/ingest/...`, `./internal/marketdata/...` and `./internal/credentials/...`. "Change both or neither" still holds.
- **Timestamps are stored in UTC and evaluated in `Europe/Istanbul`.** An offset-less provider timestamp is Europe/Istanbul local time, never UTC (removed-behaviour 20). `internal/integration/normalize` imports `time/tzdata`, so the zone exists in a scratch container.
- **The meter multiplier is applied exactly once, in the adapter** (02 §2.2). `MeterReading.MultiplierApplied` records it. Nothing downstream multiplies.
- **Unavailable registers are `nil`, never zero** (removed-behaviour 21). A `null` from a provider is `nil`.

**Secrets and TLS**
- **Secrets are never logged and never appear in error text, job_runs, cursors or operational messages.** Plaintext travels only as `integration.Secret` (Task 1). Its `Format`/`String`/`GoString`/`LogValue`/`MarshalJSON` all render `[REDACTED]`. httpx errors never include a URL, a query string, a request body or a response body (Task 2). Operator-facing text also passes `secret.Redact(msg, fragments)` with the credential's fragments.
- **TLS verification is never disabled.** `InsecureSkipVerify` does not appear in `internal/`. Self-signed provider certificates are pinned through `config.External.PinnedCerts` (`EKOKOD_PINNED_CERTS`, host → base64 DER). Adapters never accept an `*http.Client`; they take an `*httpx.Pool`, which alone builds transports.
- **Tests never hit real provider endpoints.** Every adapter test runs against `fake.NewTLSServer` (Task 4), an `httptest` **TLS** server whose certificate is pinned through the same `PinnedCerts` path production uses. Fixtures live in `internal/integration/<provider>/testdata/`. They are hand-authored from `06-integrations.md`'s field tables and the legacy TypeScript response interfaces, and they are **sanitised**: no real credential, token, installation number, company or customer name, address, e-mail, phone, IBAN or TCKN. `fake.TestFixturesAreSanitised` enforces this.

**Guards**
- **Every guard is proven to fail.** A guard seen only to pass is not evidence (F1 standing ruling). Each guard task step names the mutation that must turn it red, and the task report records the observed failure output.
- **Adapters are pure** (06 §1 rule 2). The new guard `TestAdaptersDoNotImportTheStore` (Task 1) holds packages `internal/integration/...` to no import of `internal/store/...`, `github.com/jackc/pgx/...` or `internal/ingest/...`.
- **`internal/job` still does not import `internal/store`.** This is the existing guard `TestTheJobPackageDoesNotImportTheStore`. Handlers reach the store only through interfaces declared in `internal/job`.

**Store and migrations**
- **Migration numbers are pre-assigned** (table below): `00012` and `00013`, both owned by Task 5. Never renumber a merged migration and never reuse a number.
- **Run `sqlc generate` (`make generate`) after every migration or query change and commit the output.** `make check-generate` must pass. Two F1 Criticals came from skipping this.
- **Integration tests use `testfixtures.NewIsolatedDB(t)`, `testfixtures.NewTenant`, `testfixtures.StartRedis` and `testfixtures.DiscardLogger`.** Never hand-roll a container helper. **The container-readiness trap:** a readiness timeout surfaces as an arbitrary test failing. Before believing a "flaky" test, check whether its output contains `wait until ready` or `context deadline exceeded` from testcontainers. Capture the full output on the first failure; never grep a failing run for `ok|FAIL` only.

**Process**
- **Branch `phase/f2-integration-layer`**, created from `phase/f1-data-model` **after F1 is complete** (see Pre-flight). Parallel tasks use branches `f2/task-N` in worktrees on the native filesystem:
  ```bash
  git worktree add -b f2/task-N /home/personal/ekokod-f2-tN phase/f2-integration-layer
  git config --global --add safe.directory /home/personal/ekokod-f2-tN
  ```
  Merge with `git merge --no-ff`. The Agent tool's `isolation: "worktree"` does not work on this mount.
- **Toolchain:** prefix every command with `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"`. **Docker must be up:** verify with `docker ps` before any integration test.
- **Every task ends green:**
  ```bash
  make lint && make test && make build && make check-generate
  go test ./... -tags=integration -race -count=1
  ```
- **Commit after every task.** Messages are conventional commits, and every commit carries exactly these trailers (task briefs may carry a stale session URL; always use this one):
  ```bash
  git commit -m "<type>(f2): <subject>" -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_015h7sadWHtFFEpdqhw4pkmz"
  ```
  Below, `<TRAILERS>` in a commit step means exactly that second `-m` argument.

---

## Pre-flight (controller, before Task 1; no agent)

F2 is planned against the interfaces in `internal/store/repository.go`. At the time of writing, F1 Tasks 10, 11a, 11b, 11c, 12b and 13 were still in flight on branches. Before creating the F2 branch:

- [ ] **P1.** F1's ledger shows the final whole-branch review done and its fixes merged. `phase/f1-data-model` passes the full gate.
- [ ] **P2.** The following declarations exist on `phase/f1-data-model`, with these signatures. Any drift: update this plan's Interfaces blocks before dispatch.
  ```bash
  git grep -n "BulkInsert(ctx context.Context, s Scope, rows \[\]model.MeterReading)" phase/f1-data-model -- internal/store/repository.go
  git grep -n "RecordSuccess(ctx context.Context, s Scope, analyzerID uuid.UUID, kind model.ReadingKind, lastTs, at time.Time)" phase/f1-data-model -- internal/store/repository.go
  git grep -n "type AdminMarketDataRepository interface\|type AdminJournalRepository interface\|type IntegrationRepository interface\|type OpsRepository interface\|type AnomalyRepository interface\|type ProductionRepository interface" phase/f1-data-model -- internal/store/repository.go
  git grep -n "func New[A-Za-z]*Repository(pool \*pgxpool.Pool" phase/f1-data-model -- internal/store/postgres internal/store/postgres/admin
  ```
  Expected: the constructors `postgres.NewReadingRepository`, `NewCursorRepository`, `NewAnomalyRepository`, `NewAnalyzerRepository`, `NewPlantRepository`, `NewProductionRepository`, `NewOpsRepository`, `NewIntegrationRepository(pool, cipher)`, `admin.NewMarketDataRepository` and `admin.NewJournalRepository`. Record the exact names in the Wave A brief.
- [ ] **P3.** The last migration on `phase/f1-data-model` is `00011_alarms.sql`: `git ls-tree --name-only phase/f1-data-model internal/store/postgres/migrations/ | tail -1`. If F1's fix wave added a migration, shift F2's numbers upward in the table below **before** dispatching Task 5.
- [ ] **P4.** Create the branch: `git checkout -b phase/f2-integration-layer phase/f1-data-model`.
- [ ] **P5.** Raise the **BLOCKER** below with the product owner, and the four non-blocking questions in "Open questions". F2 proceeds either way; only the plant-level branch of Task 13 is affected.

---

## ⛔ BLOCKER — `plant_production` primary key (pending the product owner)

`04-data-model.md` §4.5 declares `plant_production.device_id` nullable, but it sits inside the primary key `(plant_id, ts, device_id)`, so Postgres makes it NOT NULL. **A plant-level production sample that carries no device serial cannot be stored.** Whether iSolarCloud reports such samples (`getPowerStationPointMinuteDataList`, `getPowerStationPointDayMonthYearDataList` are plant-level calls) is the open product-owner question from `HANDOFF_NEXT_SESSION.md`.

**How F2 proceeds without guessing:**
1. **Device-attributed samples are stored.** The source is `getDevicePointMinuteDataList`, keyed by `ps_key`, resolved to `power_plant_devices.id` through `PlantRepository.Devices`.
2. **Plant-level samples are QUARANTINED, never stored and never attributed to a fabricated or "first" device.** `production.Store` (Task 13) counts them as `Quarantined`. It writes exactly one operational message per call: kind `job`, category `plant-production`, status `warning`, code `plant_level_production_unstorable`. Its metadata carries `{plant_id, samples, from, to}`, with no values that could be mistaken for stored data. The job-run detail carries the same count.
3. **No migration changes the primary key in F2.** If the product owner answers "yes, plant-level totals exist", the fix is a new migration (next free number) that changes the key, for example to a surrogate `device_key uuid not null default '00000000-0000-0000-0000-000000000000'` or a partial unique index. It is cheap while the table is empty. Task 13's quarantine branch is then replaced by a store call; the test `TestPlantLevelProductionIsQuarantinedNotFabricated` flips to assert storage.
4. **`isolar.sync_plant` is F9's job** (legacy-inventory §4: "F2 adapter, F9 features"). F2 delivers the adapter, OAuth and the quarantine-aware `production.Store` seam, so the blocker cannot leak wrong data into F9.

---

## Spec gaps and rulings

Each row is referenced by id from the task that implements it. **"Cost if wrong"** is what changing the ruling later would take.

| Id | Spec passage | Ruling | Cost if wrong |
|---|---|---|---|
| R1 | 06 §5 "store it in a dedicated `interval_generation_kwh` column … anchoring point must be persisted"; 04 has no such column or table | Spec silent on DDL. Ruling: migration `00012` adds nullable `meter_readings.interval_generation_kwh numeric(18,4)` and table `generation_anchors` (Task 5). | A column rename while tables are small |
| R2 | 06 §2 "stored separately as a cross-check series"; 04 has no table | Spec silent on DDL. Ruling: migration `00013` creates hypertable `provider_hourly_values` (Task 5). | Table rename |
| R3 | 06 §1 `FetchRequest{Point, Kind, From, To}` | The adapter must stamp `AnalyzerID` and apply the multiplier, so `FetchRequest` also carries `AnalyzerID uuid.UUID` and `Multiplier decimal.Decimal`. `FetchResult` gains `HourlyValues`, `ResolvedMultiplier` and typed `Warnings []Warning` (not `[]string`). | Mechanical signature edits |
| R4 | 06 §1 "Adapters that paginate return a cursor" with `NextCursor *time.Time` | The adapter follows provider pagination internally, up to a page budget of 10 pages per call. `NextCursor` = timestamp of the last reading returned; it is inclusive, and the overlap is harmless by idempotency. The pipeline refuses a `NextCursor` that is not strictly after `req.From` (`ErrMalformedPayload`, "cursor did not advance"). | Loop-guard tweak |
| R5 | 06 §1 rule 6 "timeout, retry … backoff and jitter", no values | Spec silent. Ruling, see "Provider defaults": per-request timeout 30 s (EPİAŞ 20 s, CAS 15 s); in-client max 3 attempts; backoff `min(30s, 1s·2^(n-1))` with full jitter in [d/2, d]; `Retry-After` honoured when ≤ 120 s, otherwise returned as `ErrRateLimited`. Task level: `asynq.MaxRetry(cfg.Worker.MaxRetries)` (default 5), delay `min(30m, 30s·2^n)` with jitter in [d/2, d] or `Retry-After`; archived (dead-letter) on exhaustion. `ErrAuth`, `ErrMalformedPayload` and `ErrNotFound` are `asynq.SkipRetry`. | Constants |
| R6 | 06 §2 "requests to one provider are serialised per company"; no rates given | Spec silent on rates. Ruling: see "Provider defaults" table. Serialisation is per request (not per task) through a distributed `lock.Locker` (Redis `SET NX PX`), plus an in-process token bucket. | Constants |
| R7 | 06 §2 energy_values placeholders | The legacy templates use `{dataType}` with 1 = load_profile, 2 = daily, 3 = reset (bcem-energy `osos/refresh/route.ts:309-311`); the spec's table omits it. Ruling: supported; the substitution fails loudly if a template lacks `{dataType}`. | None |
| R8 | 06 §2 does not say whether OSOS registers are multiplied | Spec silent. Ruling: raw, multiplied by the analyzer's `meter_multiplier` (discovered `meterMultiplier`), per 02 §2.2. **Verify against a real OSOS response in F14's live check.** | Remove one multiply; re-ingest |
| R9 | 06 §4 "All values are multiplied by the subscription's Multiplier at ingestion"; legacy sent `WithoutMultiplier: false` | Ruling: send `WithoutMultiplier: true` and multiply once in the adapter. Otherwise multiplied-by-provider values would be multiplied again. **Open question Q2** (verify with a real response). | Flip one flag; re-ingest |
| R10 | 06 §2 "numbers … sometimes with thousands separators" | Spec silent on locale. Ruling: `normalize.ProviderNumber`. If both `,` and `.` are present, the right-most is the decimal separator. A lone `,` is the decimal separator. A lone `.` is a decimal point unless it recurs, in which case it is a thousands separator. A space or `'` is a thousands separator. Table-tested. | Parser table |
| R11 | 06 §9 "before the metering point's commissioning date" | `analyzers` has no commissioning column. Ruling: the rule is not applied in F2. Recorded as **Open question Q3**; no migration is invented. | A column + one validator line |
| R12 | 06 §9 "in the future beyond a small tolerance" | Ruling: 15 min, `EKOKOD_INGEST_FUTURE_TOLERANCE`. | Env default |
| R13 | 06 §9 "configurable sanity multiple of the point's typical interval consumption" | Ruling: `EKOKOD_INGEST_SANITY_MULTIPLE` default `10`. "Typical" = median positive delta of that register, same kind, over the 7 days before the window. Needs ≥ 24 deltas, otherwise the check is skipped. The limit is `max(multiple × median × elapsed_intervals, 1)`. At most 3 consecutive rejections per analyzer per run; after that, readings are accepted and an `error` message "sustained register jump — review" is written, so a genuine step never silently discards data forever. | Validator constants |
| R14 | 06 §9 "rejected … recorded as a warning" vs. 04 §4.4 anomaly reasons | A jump is a rejection (not stored, counted `sanity_jump`), **not** an anomaly: 04's reason list has no such value. A **negative delta between cumulative readings is stored AND creates an unresolved `consumption_anomalies` row** with reason `negative_delta` (F2 acceptance), unless a `reset`-kind reading exists in `(prev.ts, cur.ts]`. A **negative register value** is rejected (`negative_value`). | Validator branch |
| R15 | 06 §5 interval hours | Ruling: each PM5340 row is one 15-minute window, so `interval_generation_kwh = currentGeneration × 0.25` regardless of row spacing. A null `currentGeneration` stays NULL and adds nothing to the running sum (the cumulative carries forward; warning `generation_interval_null`). With no anchor, one is created at `(first_ts − 15m, 0, 'initial')`. | Constant |
| R16 | 03 §4.1 job table lacks a dispatcher and a backfill job | Ruling: task `integration.sync_dispatch` (platform; enumerates active credentials via the new `AdminIngestionRepository`) and `integration.backfill`. Both are declared in Task 1. | Rename |
| R17 | 09 §F2 "`consumption.refresh` for the affected range"; F3 owns the handler | Ruling: F2 declares `TypeConsumptionRefresh` and its payload, and records `affected_from`/`affected_to` in each fetch run's `job_runs.detail`. F2 **does not enqueue it**: `ingest.Deps.ConsumptionRefresh` is `nil` in F2 wiring, because no handler exists and archived orphan tasks would accumulate. F3 wires it. Backfills older than 30 days write an `info` message naming the F1 `start_offset` fact. | F3 sets one field |
| R18 | 06 §9 initial window when no cursor exists | Ruling: `EKOKOD_INGEST_INITIAL_LOOKBACK` default 30 days (legacy energy default). GridBox is additionally bounded below by `last_success_date`. | Env default |
| R19 | 06 §9 cursor on an empty window | Ruling: the cursor advances only to the max `ts` actually persisted. An empty window never advances it, so a provider returning an empty body during an outage cannot skip data. | Loop line |
| R20 | 06 §2–§6 "rate limiting" and "pagination" fixtures for providers without such APIs | Rate limiting: HTTP 429 fixtures for all six. Pagination: ARIL `PageNumber/PageSize`, PM5340 `cursorNext`, iSolar `page/size` + `rowCount`. For OSOS, GridBox and EPİAŞ, "pagination" is window chunking: a range longer than `MaxWindow` becomes several calls merged. | Fixture rename |
| R21 | 06 §6 "signed secret header" | The legacy sends `x-access-key: <secret_key>`, `appkey` in the body and `Authorization: Bearer <access_token>` (bcem-energy `isolarClient.ts:197-233`); no HMAC signature exists. Ruling: reproduce that. A non-`"1"` `result_code` is an error. HTTP 401/403, or a `result_msg` matching `(?i)token|auth|appkey|access`, is `ErrAuth`; anything else is `ErrUpstreamUnavailable`. **Verify codes in F14.** | Mapping table |
| R22 | 06 §6 authorisation URL | Legacy format `{authorize_origin}/#/authorized-app?cloudId={cloud_id}&applicationId={app_id}&redirectUrl={redirect}` (`isolarClient.ts:683`). iSolar documents no `state`. Ruling: the HMAC-signed state rides inside the redirect URI's own query, `redirectUrl=<callback>?state=<signed>`. It is signed with HMAC-SHA256 under a key derived as `HMAC(EKOKOD_JWT_SIGNING_KEY, "isolar-oauth-state/v1")` and expires after 10 min. Token refresh starts 5 min before expiry (the legacy used 60 s); `expires_in` defaults to 7200 when absent. | Constants |
| R23 | 09 §F2 "write-only credential API surface"; auth/RBAC is F6 | Ruling: F2 delivers the service layer (`internal/credentials`) whose read model has **no secret field** and is proven so. HTTP routes (`05` §15, including `/integrations/isolar/callback`) are mounted in F6 with the authorisation matrix. | F6 mounts handlers |
| R24 | 07 §7 EPİAŞ endpoints | Not in `integration_definitions` (no enum value). Ruling: package defaults `https://giris.epias.com.tr/cas/v1/tickets` and `https://seffaflik.epias.com.tr/electricity-service` with `/v1/markets/dam/data/mcp` and `/v1/renewables/data/unit-cost` (legacy `epiasService.ts:111,148,237,332`), overridable only through `epias.Options` (tests). The TGT is cached 90 min and invalidated on `ErrAuth`. The default sync window is local days `[today−7, tomorrow+1)` plus YEKDEM for the last 3 months. Missing hours are reported, never filled (removed-behaviour 10). | Constants |
| R25 | 06 §3 GridBox `ResultStatus ≠ 1` | "Surfaced, not treated as empty". Ruling: `ErrUpstreamUnavailable` (retryable within budget). The token endpoint answering HTTP 400/401 with `invalid_grant` is `ErrAuth`. | Mapping |
| R26 | 06 §4 ARIL `current_endexes` field mapping | Spec silent. Ruling from the legacy interface (`arilService.ts:10-19`): `T_Active → active_import`, `T_Inductive → reactive_inductive_import`, `T_Capacitive → reactive_capacitive_import`, `MaxDemand → max_demand_kw`, kind `current_index`. For kind `billing` (end_of_month_endexes), `max_demand_kw` is the max `MaxDemand` of that local month's current endexes. Page size 1000 (legacy). | Mapping |
| R27 | 06 §9 discovery: new analyzers' state | Spec silent. Ruling: a newly discovered point is created with `IsActive=false` and `BuildingID=nil`; only active analyzers are fetched. A multiplier change on an existing analyzer is applied **and** written as a `warning` message (it changes bills). | One default |
| R28 | 06 §5 PM5340 base URL scheme | Customer-local service. Ruling: `http` and `https` are both accepted; `https` is always verified (pinnable). Recorded in the credential view so an operator sees the transport. | Validator |
| R29 | Status semantics of `job_runs` | Ruling: `success` when no page failed, even with rejections (rejections are `skipped`). `partial` when a page failed after ≥ 1 page persisted, or when some analyzers failed in a sync. `failed` when nothing persisted and an error occurred. | Enum mapping |
| R30 | 06 §4 ARIL `AccordPower`; `MeteringPoint.ContractedPowerKw` (plan) has no destination — `model.Analyzer` (`internal/domain/model/buildings.go`) has no `ContractedPowerKw` field, and F2 adds no migration beyond `00012`/`00013`. The only `ContractedPowerKw` in F1 belongs to the unrelated `tariffs` table. | Found during F2 planning (preflight S1/S8), not a spec gap the spec itself flags. Ruling: the adapter still parses the field into `MeteringPoint.ContractedPowerKw *decimal.Decimal` (Task 8 rule 2) — it is real provider data, and refusing to parse it would also drop it from `Raw`. The **pipeline never persists it**: Task 10's `SyncAnalyzers` Create/Update step copies every other descriptive `MeteringPoint` field to `model.Analyzer` but explicitly skips `ContractedPowerKw`, with a code comment citing this ruling. No invented migration or fabricated `analyzers` column. | One migration (`analyzers` gains a column) + one mapping line, once the product owner confirms it is wanted and distinct from `tariffs.ContractedPowerKw` |
| R31 | Task 4 review (opus): brief's sanitiser regexes miss spaced IBAN/national phone/bare host and false-positive on result_code/currencyCode/decimals/FX | Sanitiser is stricter than the brief's regexes on leaks; key match is a credential-shaped-key rule only (whole-key province match), not the brief's per-field regex list; FX placeholders skip shape rules. The brief's own acceptance cases still pass. | Adapter false positive → fixture tweak |
| R32 | 06 §1 rule 6 retry semantics silent on non-idempotent auth/refresh POSTs | `Request.NoRetry` for non-idempotent auth/refresh POSTs (exactly one attempt). | A transient failure on a token call fails the task once; the job retry layer retries later |
| R33 | 06 transport/proxy behaviour is silent | Transports use `http.ProxyFromEnvironment`. | An env proxy intercepts traffic; TLS verification + pinning still end-to-end, so confidentiality holds; ops can unset the env |
| R34 | R4's `NextCursor` definition needs refinement once windows are chunked (Task 3/10) | `NextCursor` = end of the last fully covered chunk (not the timestamp of the last reading); auxiliary series are fetched for the covered part before returning. | Re-fetch overlap on resume (idempotent upserts absorb it) |
| R35 | 06 §2 OSOS hourly `meter_date` semantics silent | OSOS hourly `meter_date` = hour START (per `HourlyValue` docs); verify against real responses in F14. | Off-by-one-hour re-ingest |
| R36 | Cross-cutting finding (Task 3): `normalize.Chunk` never spans more than one Istanbul day → Task 10's `Chunk(From,To,MaxWindow)` would call adapters day by day (~30× calls, OSOS login per day, month hourly download per day) | `normalize.Chunk` boundaries stay aligned to Istanbul midnights, but a window spans up to `max` (whole days when `max ≥ 24h`; sub-day windows only when `max < 24h`); property test must include `max ≥ 30d`. Fix on f2/task-3 (fix round 1 after completion) before Tasks 10/12/15 start. | Re-plan windowing in Task 10 |

### Open questions (non-blocking; F2 proceeds on the ruling)

- **Q1 = the BLOCKER above.**
- **Q2 (R9):** Does ARIL `owner_consumptions` with `WithoutMultiplier:false` already apply the multiplier? Verify with one real response in F14.
- **Q3 (R11):** Should `analyzers` gain a commissioning date so 06 §9's rule can be enforced?
- **Q4 (R8, R21):** OSOS multiplier semantics and iSolar non-success result codes; verify against real responses in F14's live checklist.
- **Q5 (R30):** Should `analyzers` gain a `contracted_power_kw` column so ARIL's (and any other provider's) contracted-power figure can be stored, or does it belong solely on `tariffs`?

---

## Provider defaults

The values come from R5/R6. `Rate` is requests per second in one process; `Serialise` means a distributed per-company request lock. `MaxWindow` is the largest `[From, To)` one adapter call covers.

| Provider | Request timeout | Rate / burst | Serialise per company | MaxWindow | Kinds fetched |
|---|---|---|---|---|---|
| `osos` | 30 s | 1 / 1 | yes (spec) | 30 d (hourly: one calendar month) | load_profile, daily, reset (+ hourly cross-check) |
| `gridbox` | 30 s | 2 / 2 | yes | 30 d | load_profile, daily, reset, current_index, billing (iff `use_billing_indexes`) |
| `aril` | 30 s | 2 / 2 | yes | 30 d | load_profile, current_index, billing |
| `pm5340` | 30 s | 5 / 5 | no (customer-local) | 7 d | load_profile |
| `isolar` | 30 s | 1 / 1 | yes | 1 d minute series, 31 d day series | n/a (plant data) |
| `epias` | 20 s (CAS 15 s) | 1 per 3 s / 1 | global lock `epias` | 30 d PTF, 3 months YEKDEM | n/a (prices) |

Common to all: 3 attempts in-client; `Retry-After` cap 120 s; EPİAŞ 429 without `Retry-After` waits 65 s (legacy). The response body cap is 50 MiB (a larger body is `ErrMalformedPayload`). Lock TTL is 60 s; lock wait uses the request's context.

---

## Migration number assignment

| File | Contents | Task | Depends on |
|---|---|---|---|
| `00001`–`00011` | F1 schema (**exists, do not modify**) | — | — |
| `00012_pm5340_generation.sql` | `alter table meter_readings add column interval_generation_kwh numeric(18,4)`; table `generation_anchors` | 5 | 00004 |
| `00013_provider_hourly_values.sql` | hypertable `provider_hourly_values` | 5 | 00003 |

---

## Execution waves

At most 5 concurrent agents. A wave starts only after every task it depends on is **merged** into `phase/f2-integration-layer` and the merged tip passes the gate. After merging Task 5, run `make generate && make check-generate` on the phase tip before starting Wave C.

**Wave placement (S11 speed ruling, applied):** Tasks 3, 4 and 5 each state zero dependency on any other F2 task (Task 3: `shopspring/decimal` only; Task 4: "nothing from Tasks 1–3"; Task 5: F1 only, and Task 1's own text independently confirms "Task 1 has no dependency on Task 5"). They move into Wave A alongside Task 1 — four tasks, within the 5-concurrent-agent budget, with no shared files (File ownership table below). Task 12 (EPİAŞ) consumes only Tasks 1–4, never Task 5 or Task 10, so once Wave A and B are merged it is exactly as ready as Tasks 6–9; it moves to Wave C. That makes Wave C six ready tasks against a 5-concurrent budget: the controller dispatches Tasks 6, 7, 8, 9, 10 first (they fill the budget and are the longer poles — 10 is consumed by three later tasks, 6/7/8/9 gate the adapter fixture matrix), and starts Task 12 the moment any one of those five merges and frees a slot. This is a scheduling detail, not a dependency change — Task 12 does not depend on 6, 7, 8, 9 or 10, and nothing in Wave D waits on Task 12 any later than it already did. Task 13 (iSolar) keeps its stated Task 5 dependency and stays in Wave D; moving it was considered (S11) and rejected because, unlike Task 12, F14's isolar client genuinely exercises the F1 production/plant seams Task 5 does not touch, so the lower-confidence preflight suggestion is not taken up.

**Task 14 gets its own wave (S3, applied).** The only same-wave dependency the preflight scan found was Task 14 (Wave D) needing Task 13's (also Wave D) `internal/integration/isolar/token.go` before it could even compile, which the original plan patched with a controller sub-merge rule ("merge Task 13's first commit before dispatching Task 14"). Removing Task 14 from Wave D and giving it a solo wave (E) removes the special case entirely: by the time Wave E starts, all of Wave D — including the whole of Task 13, not just its first commit — is already merged, via the ordinary "a wave starts only after every task it depends on is merged" rule that governs every other wave. Task 16 then moves to Wave F and Task 17 to Wave G. Task 14 also gains a real (previously undeclared) dependency on Task 10's `ingest.Enqueuer` (S6); since Task 10 is in Wave C, well before Task 14's new Wave E, this is not a new ordering problem.

| Wave | Tasks (parallel) | Depends on |
|---|---|---|
| A | 1, 3, 4, 5 | Pre-flight |
| B | 2 | A (Task 1 for sentinels/`Provider`; Task 4 for the canonical `lock.Locker`/`lock.Lease`, which `httpx` now aliases — R2) |
| C | 6, 7, 8, 9, 10, 12 | A, B (2–5 for the meter adapters and the pipeline; 1–4 for EPİAŞ — never 5 or 10). **6 ready tasks, 5-concurrent budget:** dispatch 6, 7, 8, 9, 10 first; start 12 as soon as a slot frees. |
| D | 11, 13, 15 | C (10, for 11 and 15); A (1, 5) for all three |
| E | 14 | D fully merged (all of Task 13, not just `token.go` — no sub-merge rule); C (10, for `ingest.Enqueuer`); A (1, 4, 5) |
| F | 16 | A–E |
| G | 17 | F |

## File ownership (parallel tasks never edit the same file)

| Task | Owns (creates or modifies) — nobody else touches these |
|---|---|
| 1 | `internal/integration/{doc.go,source.go,credentials.go,errors.go,registry.go}` + `_test.go` files; `internal/{ingest,marketdata,credentials}/doc.go`; `internal/job/integration.go`, `internal/job/integration_test.go`, `internal/job/retry.go`, `internal/job/retry_test.go`; **modifies** `internal/job/task.go`, `internal/job/server.go`, `internal/arch/arch_test.go`, `.golangci.yml` |
| 2 | `internal/integration/httpx/**` |
| 3 | `internal/integration/normalize/**` |
| 4 | `internal/integration/fake/**`; `internal/platform/lock/**` |
| 5 | migrations `00012`, `00013`; `internal/store/postgres/queries/{generation.sql,provider_series.sql,admin_ingestion.sql}`; `internal/store/postgres/{generation.go,provider_series.go}` + tests; `internal/store/postgres/admin/ingestion.go` + test; `internal/store/postgres/sqlcgen/**` (generated); **modifies** `internal/store/repository.go`, `internal/store/scope.go`, `internal/store/scope_test.go`, `internal/domain/model/timeseries.go`, `internal/domain/model/operations.go`, `internal/store/postgres/readings.go`, `internal/store/postgres/queries/readings.sql`, `internal/store/postgres/readings_integration_test.go`, `internal/store/postgres/admin/doc.go` (records the sixth Admin interface, see Task 5's Files list) |
| 6 | `internal/integration/osos/**` |
| 7 | `internal/integration/gridbox/**` |
| 8 | `internal/integration/aril/**` |
| 9 | `internal/integration/pm5340/**` |
| 10 | `internal/ingest/*.go` (package `ingest` only, not subdirectories; **excludes** `internal/ingest/doc.go`, created by Task 1 in Wave A and left unchanged here — no overlap with Task 1's row above) |
| 11 | `internal/ingest/generation/**` |
| 12 | `internal/integration/epias/**`; `internal/marketdata/**` (**excludes** `internal/marketdata/doc.go`, created by Task 1 and left unchanged) |
| 13 | `internal/integration/isolar/**`; `internal/ingest/production/**` |
| 14 | `internal/credentials/**` (**excludes** `internal/credentials/doc.go`, created by Task 1 and left unchanged) |
| 15 | `internal/ingest/backfill/**` |
| 16 | `internal/worker/**`; **modifies** `internal/cli/worker.go`, `internal/scheduler/scheduler.go`, `internal/scheduler/elector.go`, `internal/scheduler/*_test.go`, `internal/platform/config/{config.go,load.go,config_test.go}`, `docs/rewrite/appendix/env-reference.md`, `.env.example` |
| 17 | `internal/job/ingestion_integration_test.go`, `internal/arch/adapter_matrix_test.go`, `HANDOFF_NEXT_SESSION.md` |

## Naming prefixes (F1's collision lesson)

Several F1 and F2 tasks share `internal/store/postgres` and `internal/job`. In those packages, every **new unexported identifier, test helper and sqlc query name** carries its owner's prefix:

| Package | Owner | Prefix for new helpers | sqlc query name prefix |
|---|---|---|---|
| `internal/store/postgres` | Task 5 | `f2gen…`, `f2series…` | `Generation…`, `ProviderHourly…`, `ReadingsF2…` |
| `internal/store/postgres/admin` | Task 5 | `adminIngest…` | `AdminIngestion…` |
| `internal/job` (package `job`) | Task 1 | `integ…` | — |
| `internal/job` (package `job_test`) | Task 17 | `accept…` | — |
| `internal/arch` | Task 1 / Task 17 | `f2guard…` / `f2matrix…` | — |

Packages owned by one task need no prefix. Test fixture files are named `<provider>_<case>.json` inside the adapter's own `testdata/`.

---

## File Structure

```
internal/integration/
  doc.go source.go credentials.go errors.go registry.go     # Task 1: the contract
  httpx/  client.go pool.go retry.go ratelimit.go pin.go template.go classify.go   # Task 2
  normalize/  decimal.go time.go units.go window.go           # Task 3 (window.go: the shared range-chunking helper, S7)
  fake/  server.go fixtures.go responders.go sanitise.go sanitise_test.go   # Task 4
  osos/ gridbox/ aril/ pm5340/  source.go mapping.go wire.go testdata/   # Tasks 6-9
  epias/  client.go ticket.go parse.go testdata/             # Task 12
  isolar/  client.go oauth.go series.go alarms.go testdata/  # Task 13
internal/platform/lock/  lock.go memory.go redis.go          # Task 4
internal/ingest/                                             # Task 10: the pipeline
  service.go deps.go dispatch.go sync.go fetch.go dedupe.go validate.go anomaly.go report.go
  generation/ accumulate.go hook.go                          # Task 11
  backfill/   backfill.go                                    # Task 15
  production/ store.go                                       # Task 13 (blocker-aware)
internal/marketdata/  sync.go completeness.go                # Task 12
internal/credentials/ service.go view.go open.go isolar_oauth.go state.go   # Task 14
internal/worker/      wiring.go                              # Task 16
internal/job/         integration.go retry.go                # Task 1
internal/store/postgres/migrations/ 00012_pm5340_generation.sql 00013_provider_hourly_values.sql  # Task 5
internal/store/postgres/ generation.go provider_series.go admin/ingestion.go   # Task 5
```

---
## Task 1: The integration contract, the job seam and the extended guards

**Files:**
- Create: `internal/integration/doc.go`, `source.go`, `credentials.go`, `errors.go`, `registry.go`, `credentials_test.go`, `errors_test.go`, `registry_test.go`
- Create: placeholder `internal/ingest/doc.go`, `internal/marketdata/doc.go`, `internal/credentials/doc.go` (package clause + doc comment only; Tasks 10, 12, 14 keep them unchanged)
- Create: `internal/job/integration.go`, `internal/job/integration_test.go`, `internal/job/retry.go`, `internal/job/retry_test.go`
- Modify: `internal/job/task.go` (`Handlers` fields, `Register`), `internal/job/server.go` (`RetryDelayFunc`)
- Modify: `internal/arch/arch_test.go`, `.golangci.yml`

**Interfaces:**
- Consumes: `model.MeterReading`, `model.ReadingKind`, `model.IntegrationProvider` (F1). The OSOS cross-check series is declared here as `integration.HourlyValue` (no store type), so Task 1 has no dependency on Task 5; the pipeline (Task 10) converts it to `model.ProviderHourlyValue`.
- Produces, used by every later task:

```go
package integration

type Provider string

const (
	ProviderOSOS    Provider = "osos"
	ProviderGridBox Provider = "gridbox"
	ProviderARIL    Provider = "aril"
	ProviderPM5340  Provider = "pm5340"
	ProviderISolar  Provider = "isolar"
	ProviderEPIAS   Provider = "epias" // not in integration_provider: platform credential from config
)

// ModelProvider maps to the SQL enum; ok is false for ProviderEPIAS.
func (p Provider) ModelProvider() (model.IntegrationProvider, bool)

type MeterDataSource interface { // 06 §1, verbatim
	Provider() Provider
	Verify(ctx context.Context, creds Credentials) error
	DiscoverMeteringPoints(ctx context.Context, creds Credentials) ([]MeteringPoint, error)
	FetchReadings(ctx context.Context, creds Credentials, req FetchRequest) (FetchResult, error)
}

// Planner is what the pipeline needs beyond 06's interface: which kinds to
// ask for and how wide one call may be (Provider defaults table).
type Planner interface {
	Kinds(creds Credentials) []model.ReadingKind
	MaxWindow(kind model.ReadingKind) time.Duration
}

type Adapter interface { MeterDataSource; Planner }

type MeteringPoint struct {
	InstallationNumber string
	CustomerName, Address, Province, District, Neighbourhood, Street *string
	TariffType, TariffKind, InstallationKind                         *string
	InstalledPowerKw, ContractedPowerKw                              *decimal.Decimal
	MeterNumber, MeterModel                                          *string
	MeterMultiplier                                                  *decimal.Decimal // nil: provider did not report
	CounterpartyNo, MeteringPointName, EtsoCode                      *string
	Latitude, Longitude                                              *decimal.Decimal
	DefinitionType                                                   *int16
	ProviderHighWater                                                map[model.ReadingKind]time.Time
}

type FetchRequest struct {
	Point      MeteringPoint
	AnalyzerID uuid.UUID       // R3
	Multiplier decimal.Decimal // R3: the analyzer's stored meter_multiplier
	Kind       model.ReadingKind
	From, To   time.Time // half-open [From, To), UTC
}

type MultiplierSource string

const (
	MultiplierFromLastEndex   MultiplierSource = "last_endex"
	MultiplierFromLoadProfile MultiplierSource = "load_profile_ratio"
	MultiplierFallbackOne     MultiplierSource = "fallback_one"
)

type ResolvedMultiplier struct { Value decimal.Decimal; Source MultiplierSource }

type Warning struct {
	Code   string // one of the Warn* constants
	Detail string // operator text; never a secret, never a raw payload
}

const (
	WarnMultiplierFallback    = "multiplier_fallback"
	WarnUnparseableRow        = "unparseable_row"
	WarnGenerationIntervalNil = "generation_interval_null"
	WarnMissingPriceHours     = "missing_price_hours"
)

type FetchResult struct {
	Readings           []model.MeterReading // normalised, multiplied, UTC
	ResolvedMultiplier *ResolvedMultiplier
	NextCursor         *time.Time // R4
	Warnings           []Warning
	HourlyValues       []HourlyValue // OSOS only (06 §2, R2); never mixed into Readings
}

// HourlyValue is provider-differenced consumption: a cross-check, not an index.
type HourlyValue struct {
	Ts                time.Time // hour start, UTC
	ActiveConsumption *decimal.Decimal
	ActiveGeneration  *decimal.Decimal
}
```

```go
// credentials.go
type Secret struct{ b []byte }

func NewSecret(b []byte) Secret // copies b
func (s Secret) Reveal() string
func (s Secret) IsZero() bool
func (s Secret) Format(f fmt.State, verb rune)       // writes "[REDACTED]" for every verb
func (s Secret) String() string                      // "[REDACTED]"
func (s Secret) GoString() string                    // "integration.Secret([REDACTED])"
func (s Secret) LogValue() slog.Value                // "[REDACTED]"
func (s Secret) MarshalJSON() ([]byte, error)        // `"[REDACTED]"`
func (s Secret) MarshalText() ([]byte, error)        // "[REDACTED]"

type Settings struct {
	UseBillingIndexes bool `json:"use_billing_indexes"`
}

type Credentials struct {
	CredentialID, CompanyID uuid.UUID
	Provider                Provider
	Subtype                 string
	Endpoints               map[string]string // integration_definitions.endpoints (templates)
	Username                string
	Secret                  Secret            // password / secret key
	Extra                   map[string]Secret // e.g. "app_key", "secret_key", "app_id", "access_token", "refresh_token"
	Settings                Settings
	BaseURL                 string // pm5340_url
	Region                  string // isolar_region
	TokenExpiresAt          *time.Time
}

// Fragments returns every secret's plaintext for secret.Redact. Never log it.
func (c Credentials) Fragments() []string
```

```go
// errors.go — 06 §1 rule 5
var (
	ErrAuth                = errors.New("integration: authentication failed")
	ErrRateLimited         = errors.New("integration: rate limited")
	ErrUpstreamUnavailable = errors.New("integration: upstream unavailable")
	ErrMalformedPayload    = errors.New("integration: malformed payload")
	ErrNotFound            = errors.New("integration: not found")
)

type Error struct {
	Kind       error // one of the sentinels
	Provider   Provider
	Op         string // endpoint key, e.g. "load_profiles" — never a URL
	HTTPStatus int
	RetryAfter time.Duration
}

func (e *Error) Error() string // "gridbox load_profiles: integration: rate limited (HTTP 429)"
func (e *Error) Unwrap() error // Kind
func Retryable(err error) bool // ErrRateLimited or ErrUpstreamUnavailable
func RetryAfter(err error) (time.Duration, bool)

// registry.go
func NewRegistry(adapters ...Adapter) (*Registry, error) // duplicate provider → error
func (r *Registry) Source(p Provider) (Adapter, error)   // unknown → wraps ErrNotFound
```

```go
// internal/job/integration.go
const (
	TypeIntegrationSyncDispatch  = "integration.sync_dispatch"  // R16, platform
	TypeIntegrationSyncAnalyzers = "integration.sync_analyzers"
	TypeIntegrationFetchReadings = "integration.fetch_readings"
	TypeIntegrationBackfill      = "integration.backfill"       // R16
	TypeEPIASSyncPrices          = "epias.sync_prices"
	TypeConsumptionRefresh       = "consumption.refresh"        // R17: declared, not handled, in F2
)

type Window struct{ From, To time.Time }

type SyncAnalyzersPayload struct{ CompanyID, CredentialID uuid.UUID }
type FetchReadingsPayload struct {
	CompanyID, CredentialID, AnalyzerID uuid.UUID
	Kind   model.ReadingKind
	Window *Window // nil: cursor-driven
}
type BackfillPayload struct {
	CompanyID, CredentialID uuid.UUID
	AnalyzerIDs []uuid.UUID // empty: every ACTIVE analyzer of the credential (resolved at run time; never "no filter" in a query)
	Kinds       []model.ReadingKind
	From, To    time.Time
}
type SyncPricesPayload struct{ Window *Window }
type ConsumptionRefreshPayload struct {
	CompanyID, AnalyzerID uuid.UUID
	From, To time.Time
}

type TaskOptions struct{ MaxRetry int }

func NewSyncDispatchTask(o TaskOptions) (*asynq.Task, error)                          // Unique 1h, Timeout 10m, QueueDefault
func NewSyncAnalyzersTask(p SyncAnalyzersPayload, o TaskOptions) (*asynq.Task, error) // Unique 1h, Timeout 10m
func NewFetchReadingsTask(p FetchReadingsPayload, o TaskOptions) (*asynq.Task, error) // Window==nil: Unique 1h; else TaskID "backfill:<analyzer>:<kind>:<from unix>:<to unix>", Retention 30d; Timeout 10m
func NewBackfillTask(p BackfillPayload, o TaskOptions) (*asynq.Task, error)           // Timeout 30m, QueueLow
func NewSyncPricesTask(p SyncPricesPayload, o TaskOptions) (*asynq.Task, error)       // Unique 1h, Timeout 10m
// Decode<Name>(t *asynq.Task) (<Name>Payload, error) for each; unknown JSON fields refused.

type Ingestion interface {
	Dispatch(ctx context.Context) error
	SyncAnalyzers(ctx context.Context, p SyncAnalyzersPayload) error
	FetchReadings(ctx context.Context, p FetchReadingsPayload) error
}
type Backfiller interface{ Backfill(ctx context.Context, p BackfillPayload) error }
type PriceSyncer interface{ SyncPrices(ctx context.Context, p SyncPricesPayload) error }

// retry.go
// ClassifyForRetry wraps err with asynq.SkipRetry for ErrAuth,
// ErrMalformedPayload and integration.ErrNotFound; returns err unchanged otherwise.
func ClassifyForRetry(err error) error
// RetryDelay is asynq.Config.RetryDelayFunc: RetryAfter(err) if present,
// else min(30m, 30s·2^n) jittered into [d/2, d] with rand.Int64N (no floats).
func RetryDelay(n int, err error, t *asynq.Task) time.Duration
```

`task.go` changes: `Handlers` gains `Ingestion Ingestion`, `Backfill Backfiller`, `Prices PriceSyncer`. `Register` always registers noop. It registers each integration type **only when its field is non-nil**, through adapters that decode the payload, call the method, and return `ClassifyForRetry(err)`. `server.go` sets `RetryDelayFunc: RetryDelay`.

- [ ] **Step 1: Write the failing tests for Secret, errors, registry and the job seam**

`internal/integration/credentials_test.go`:

```go
// TestCredentialsNeverFormatSecrets pins 03 §7 "never logged" for the one
// type that carries plaintext. Every rendering path a developer might use by
// accident is exercised; a new path that leaks must be added here.
func TestCredentialsNeverFormatSecrets(t *testing.T) {
	const pw = "FIXTURE-PLAINTEXT-7f3a"
	c := integration.Credentials{
		Username: "fixture-user",
		Secret:   integration.NewSecret([]byte(pw)),
		Extra:    map[string]integration.Secret{"access_token": integration.NewSecret([]byte(pw + "-tok"))},
	}
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil)) // bare handler on purpose: Secret must not rely on redaction by key
	logger.Info("creds", slog.Any("c", c), slog.Any("s", c.Secret))
	js, err := json.Marshal(c)
	require.NoError(t, err)
	for name, rendered := range map[string]string{
		"%v": fmt.Sprintf("%v", c), "%+v": fmt.Sprintf("%+v", c), "%#v": fmt.Sprintf("%#v", c),
		"%s": fmt.Sprintf("%s", c.Secret), "%q": fmt.Sprintf("%q", c.Secret), "%x": fmt.Sprintf("%x", c.Secret),
		"json": string(js), "slog": logBuf.String(),
		"error": fmt.Errorf("wrap: %v", c).Error(),
	} {
		require.NotContains(t, rendered, pw, "secret leaked through %s", name)
	}
	require.Equal(t, pw, c.Secret.Reveal())
}
```

`internal/integration/errors_test.go`: `TestErrorUnwrapsToItsKind` (table: each sentinel, `errors.Is` true for its own kind, false for the others); `TestRetryableClassification` (RateLimited and UpstreamUnavailable true; Auth, Malformed and NotFound false; a plain `errors.New` false); `TestRetryAfterOnlyFromRateLimited`. `registry_test.go`: `TestRegistryRefusesDuplicateProviders`, `TestRegistryUnknownProviderIsNotFound`.

`internal/job/integration_test.go`: `TestIntegrationPayloadsRoundTrip` (every constructor → Decode returns an equal payload); `TestDecodeRefusesUnknownFields`; `TestFetchTaskWithWindowHasDeterministicTaskID` (same payload twice gives the same `asynq.TaskID`; a different window gives a different one); `TestRegisterSkipsNilIntegrationHandlers` (a `job.Handlers{Log: …}` with nil integration fields registers no handler for `TypeIntegrationFetchReadings`: `mux.Handler(asynq.NewTask(job.TypeIntegrationFetchReadings, nil))` returns the not-found handler); `TestRegisterRoutesFetchReadingsToIngestion` (a fake `Ingestion` receives the decoded payload).

`internal/job/retry_test.go`:

```go
func TestClassifyForRetrySkipsNonRetryableKinds(t *testing.T) {
	for _, tc := range []struct{ err error; skip bool }{
		{&integration.Error{Kind: integration.ErrAuth}, true},
		{&integration.Error{Kind: integration.ErrMalformedPayload}, true},
		{&integration.Error{Kind: integration.ErrNotFound}, true},
		{&integration.Error{Kind: integration.ErrRateLimited}, false},
		{&integration.Error{Kind: integration.ErrUpstreamUnavailable}, false},
		{errors.New("db down"), false},
	} {
		require.Equal(t, tc.skip, errors.Is(job.ClassifyForRetry(tc.err), asynq.SkipRetry), "%v", tc.err)
	}
}

func TestRetryDelayIsBoundedAndHonoursRetryAfter(t *testing.T) {
	for n := 0; n < 20; n++ {
		d := job.RetryDelay(n, errors.New("x"), nil)
		require.GreaterOrEqual(t, d, 15*time.Second)
		require.LessOrEqual(t, d, 30*time.Minute)
	}
	ra := &integration.Error{Kind: integration.ErrRateLimited, RetryAfter: 90 * time.Second}
	require.Equal(t, 90*time.Second, job.RetryDelay(3, ra, nil))
}
```

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/integration/ ./internal/job/ -race -run 'TestCredentials|TestError|TestRetry|TestRegistry|TestIntegration|TestDecode|TestFetchTask|TestRegister|TestClassify' -v
```
Expected: FAIL — package `internal/integration` does not exist; `job.NewFetchReadingsTask` undefined.

- [ ] **Step 3: Implement the contract and the job seam** exactly as the Interfaces block pins it. `Secret.Format` ignores the verb and writes `[REDACTED]`. `Error.Error()` uses only `Provider`, `Op`, `Kind` and `HTTPStatus`. Payload decoding uses `json.NewDecoder(bytes.NewReader(p)).DisallowUnknownFields()`.

- [ ] **Step 4: Run them and watch them pass** (same command). Expected: PASS.

- [ ] **Step 5: Write the new and widened guards in `internal/arch/arch_test.go`**

1. `TestAdaptersDoNotImportTheStore`: load `./internal/integration/...` with `NeedImports`; fail on any import under `internal/store`, `internal/ingest`, `internal/credentials`, `internal/marketdata` or `github.com/jackc/pgx`. Use `require.NotEmpty` on the package list (a vacuous pass is a failure).
2. Widen `TestStoreAndIntegrationDoNotImportAPIOrService` so it really loads `./internal/integration/...` as its name claims (today it loads only `./internal/store/...`).
3. `TestIntegrationTreesDoNotParseFloats`: load typed packages for `./internal/integration/...`, `./internal/ingest/...`, `./internal/marketdata/...` and `./internal/credentials/...`. Walk every non-test file's AST. Fail on (a) a call to `strconv.ParseFloat`, and (b) a call to `json.Unmarshal` or `(*json.Decoder).Decode` whose argument's type (through pointers, slices, maps and struct fields) contains `interface{}`/`any` or a float. **Exempt exactly one file by path, `internal/integration/httpx/ratelimit.go`**: `rate.Limit` is a float type, and converting a `time.Duration` to it is the only float there. Name the exemption in a comment.
4. `TestNoTLSConfigDisablesVerification`: an AST walk over `internal/` (non-test **and** test files) fails on any composite literal of `crypto/tls.Config` that sets `InsecureSkipVerify`, `VerifyPeerCertificate` or `VerifyConnection`. The existing string guard misses a `tls.Config` built through reflection or a variable key; this one sees fields.

- [ ] **Step 6: Extend the float guards to the F2 trees**

In `internal/arch/arch_test.go`:

```go
var floatGuardPatterns = []string{
	"./internal/domain/...",
	"./internal/store/...",
	"./internal/integration/...",
	"./internal/ingest/...",
	"./internal/marketdata/...",
	"./internal/credentials/...",
}
```

The packages under the last four patterns do not exist yet. `loadTypedPackages` requires a non-empty match, so **create `internal/integration/doc.go` in this task**. For `ingest`, `marketdata` and `credentials`, add a placeholder `doc.go` (package clause and doc comment only) owned by Task 1, which Tasks 10, 12 and 14 then keep. In `.golangci.yml`, `no-float-money.files` gains `"**/internal/integration/**"`, `"**/internal/ingest/**"`, `"**/internal/marketdata/**"` and `"**/internal/credentials/**"`. Update both comments ("Change both or neither").

- [ ] **Step 7: Prove every guard fails, then restore**

Apply each mutation in a scratch commit, run it, confirm FAIL naming the offender, then `git checkout -- .`:

| Guard | Mutation |
|---|---|
| `TestAdaptersDoNotImportTheStore` | add `_ "github.com/MErenTalan/ekokod-rewrite/internal/store"` to `internal/integration/doc.go` |
| `TestStoreAndIntegrationDoNotImportAPIOrService` | add `_ ".../internal/api"` to `internal/integration/doc.go` |
| `TestIntegrationTreesDoNotParseFloats` | add `var _, _ = strconv.ParseFloat("1", 64)` in `internal/integration/registry.go`; separately `var m map[string]any; _ = json.Unmarshal(nil, &m)` |
| `TestNoTLSConfigDisablesVerification` | add `var _ = &tls.Config{VerifyPeerCertificate: nil}` to `internal/integration/doc.go` |
| `TestNoFloatFieldsInModelOrStore` | add `type f2guardProbe struct{ Kwh float64 }` to `internal/integration/source.go` |
| depguard `no-float-money` | add `_ "math/big"` to `internal/ingest/doc.go`; `golangci-lint run ./internal/ingest/...` fails |
| `TestCredentialsNeverFormatSecrets` | make `Secret.GoString` return `string(s.b)` |

```bash
go test ./internal/arch/ -race -run 'TestAdapters|TestStoreAndIntegration|TestIntegrationTrees|TestNoTLSConfig|TestNoFloatFields' -v
```
Record each observed failure line in the task report.

- [ ] **Step 8: Gate and commit**

```bash
make lint && make test && make build && make check-generate
git add internal/integration internal/job internal/arch internal/ingest/doc.go internal/marketdata/doc.go internal/credentials/doc.go .golangci.yml
git commit -m "feat(f2): integration contract, job seam and float/TLS/purity guards" <TRAILERS>
```

---

## Task 2: `httpx` — timeouts, retries, rate limits, pinning, URL templates

**Files:**
- Create: `internal/integration/httpx/{pool.go,client.go,retry.go,ratelimit.go,pin.go,template.go,classify.go}`
- Test: `internal/integration/httpx/{client_test.go,pin_test.go,template_test.go,retry_test.go}`

**Interfaces:**
- Consumes: `integration.Provider`, `integration.Error` and the sentinels (Task 1); `lock.Locker`/`lock.Lease` **by direct type alias, not a structural redeclaration** (Task 4; R2). The original draft declared `httpx.Locker`/`httpx.Lease` as their own named interfaces "structurally identical" to `lock.Locker`/`lock.Lease` — Go does not treat two differently-named interfaces as the same type for method-set satisfaction when a method's *return* type is one of those named interfaces, so `*lock.Memory`/`*lock.Redis` did not actually implement `httpx.Locker` (preflight review). The fix: `lock` (Task 4, Wave A — merged before this task starts) is the canonical owner of `Locker`/`Lease`; `httpx` imports it and aliases:

```go
package httpx

import lock "github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"

// Locker and Lease are aliases (`type X = lock.X`), not new types: lock's
// own Memory and Redis implementations satisfy httpx.Locker without an
// adapter anywhere in the plan (R2).
type Locker = lock.Locker
type Lease = lock.Lease

type PoolOptions struct {
	PinnedCerts map[string]string // config.External.PinnedCerts: host -> base64(DER)
	Locker      Locker            // nil: no cross-process serialisation (tests, PM5340)
	Sleep       func(ctx context.Context, d time.Duration) error // nil: real timer; tests inject
	Now         func() time.Time
}
func NewPool(o PoolOptions) (*Pool, error) // invalid base64/DER → error naming the HOST only

type ClientConfig struct {
	Provider       integration.Provider
	LimiterKey     string        // e.g. "osos:<company uuid>"; shared limiter per key
	Every          time.Duration // one request per Every
	Burst          int
	RequestTimeout time.Duration // default 30s
	MaxAttempts    int           // default 3
	BaseBackoff    time.Duration // default 1s
	MaxBackoff     time.Duration // default 30s
	MaxRetryAfter  time.Duration // default 120s
	DefaultRateLimitWait time.Duration // used when 429 has no Retry-After; default = BaseBackoff schedule; EPİAŞ sets 65s
	SerializeKey   string        // non-empty: hold Pool.Locker for each request; TTL 60s
	MaxBodyBytes   int64         // default 50 MiB
}
func (p *Pool) Client(c ClientConfig) *Client

type Param struct {
	Value  string
	Secret bool // true: value never appears in any error text
}
type Request struct {
	Op          string            // endpoint key for errors, e.g. "authentication"
	Method      string
	Template    string            // may contain {placeholders}
	Params      map[string]Param
	Header      http.Header       // values may be secret: never rendered in errors
	Body        []byte
	ContentType string
}
type Response struct { Status int; Header http.Header; Body []byte }

// Do sends req with limiter → optional serialise lock → timeout → retry.
// Returns *integration.Error for every non-2xx outcome after retries; a 2xx is returned as-is.
func (c *Client) Do(ctx context.Context, req Request) (Response, error)

// Expand substitutes placeholders: url.PathEscape before '?', url.QueryEscape after.
// Missing param, unused param, or a leftover '{…}' → error (no request sent).
func Expand(template string, params map[string]Param) (*url.URL, error)
```

- Produces: `httpx.NewPool`, `(*Pool).Client`, `(*Client).Do`, `httpx.Expand`, `httpx.Param`, `httpx.Request`, `httpx.Response`, `httpx.Locker`, `httpx.Lease`. Adapters call nothing else.

Classification (`classify.go`): 401/403 → `ErrAuth`; 404 → `ErrNotFound`; 429 → `ErrRateLimited` (parse `Retry-After` as seconds or HTTP-date); 408, 5xx, network errors and timeouts → `ErrUpstreamUnavailable` (retryable); TLS/x509 verification failure → `ErrUpstreamUnavailable` **not retried in-client**; other 4xx → `ErrMalformedPayload`; body over `MaxBodyBytes` → `ErrMalformedPayload`. Transport errors are **never wrapped as causes**: `*url.Error` embeds the URL, which may carry a password (OSOS puts credentials in the auth URL). Only the classification survives, the same rule as F1's "DSN did not parse" branch.

- [ ] **Step 1: Write the failing tests**

`template_test.go`: `TestExpandSubstitutesPathAndQuery` (`https://h/{wiringNo}/x?start={startDate}` with `wiringNo="A/B"` and `startDate="01/09/2026 00:00:00"` gives `A%2FB` and `01%2F09%2F2026+00%3A00%3A00`); `TestExpandRefusesMissingUnusedAndLeftoverPlaceholders`.

`client_test.go` (all servers are `httptest.NewTLSServer`, pinned through `PoolOptions.PinnedCerts` built from `srv.Certificate().Raw`; Sleep injected to record delays, never sleeping):

```go
// TestHTTPErrorsNeverContainSubstitutedSecrets pins removed-behaviour 17 for
// the transport: OSOS's legacy auth URL carries the password in the query.
func TestHTTPErrorsNeverContainSubstitutedSecrets(t *testing.T) {
	const pw = "FIXTURE-PW-91c2"
	for _, status := range []int{401, 404, 429, 500, 400} {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"echo":"` + r.URL.Query().Get("p") + `"}`)) // provider echoes the secret back
		}))
		c := httpxTestPool(t, srv).Client(httpx.ClientConfig{Provider: integration.ProviderOSOS, LimiterKey: "t", Every: time.Millisecond, Burst: 1, MaxAttempts: 1})
		_, err := c.Do(context.Background(), httpx.Request{
			Op: "authentication", Method: http.MethodPost,
			Template: srv.URL + "/auth?u={u}&p={p}",
			Params:   map[string]httpx.Param{"u": {Value: "fixture"}, "p": {Value: pw, Secret: true}},
			Header:   http.Header{"Authorization": {"Bearer " + pw}},
		})
		require.Error(t, err)
		require.NotContains(t, err.Error(), pw)
		require.NotContains(t, fmt.Sprintf("%+v", err), pw)
		require.NotContains(t, err.Error(), srv.URL, "no URL in error text")
		srv.Close()
	}
	// network failure path: a closed server yields *url.Error internally
	srv := httptest.NewTLSServer(http.NotFoundHandler()); pool := httpxTestPool(t, srv); srv.Close()
	_, err := pool.Client(httpx.ClientConfig{Provider: "osos", LimiterKey: "t", Every: time.Millisecond, Burst: 1, MaxAttempts: 1}).
		Do(context.Background(), httpx.Request{Op: "authentication", Method: "GET", Template: srv.URL + "/?p={p}", Params: map[string]httpx.Param{"p": {Value: pw, Secret: true}}})
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
	require.NotContains(t, err.Error(), pw)
}
```

Also:
- `TestStatusClassification` (table of status → sentinel).
- `TestRetriesUpstreamFailuresWithJitteredBackoff`: 500, 500, 200 → success; 3 requests; recorded sleeps within `[0.5s,1s]` then `[1s,2s]`.
- `TestDoesNotRetryAuthOrMalformed`: exactly 1 request.
- `TestRateLimitedHonoursRetryAfterUpToCap`: `Retry-After: 7` → one sleep of exactly 7s then success; `Retry-After: 600` → no sleep, `ErrRateLimited` with `RetryAfter()==600s`.
- `TestDefaultRateLimitWait`: 429 without header, `DefaultRateLimitWait: 65s` → a sleep of 65s.
- `TestRequestTimeoutIsEnforced`: handler blocks; `RequestTimeout: 50ms` → `ErrUpstreamUnavailable` within 2s.
- `TestLimiterSpacesRequestsPerKey`: a fake `Now/Sleep` shows the second request on the same key waits `Every`; a different key does not.
- `TestSerializeKeyHoldsTheLockPerRequest`: a fake Locker records Acquire/Release pairs around each request, in order, and Release happens even on error.
- `TestBodyLargerThanCapIsMalformed`.

`pin_test.go` (uses `fake.NewTLSServer`, Task 4 — merged in Wave A before this task starts. Every call mints a **fresh** self-signed certificate, R4: Go's own `httptest.NewTLSServer` reuses one built-in localhost cert across every server in the process, which makes a naive "pin the other server's cert" test pass for the wrong reason — both servers would present the identical certificate):

```go
func TestUnpinnedSelfSignedServerIsRefused(t *testing.T) {
	srv := fake.NewTLSServer(t)
	pool, err := httpx.NewPool(httpx.PoolOptions{}) // system roots only
	require.NoError(t, err)
	_, err = pool.Client(httpx.ClientConfig{Provider: "aril", LimiterKey: "k", Every: time.Millisecond, Burst: 1, MaxAttempts: 3}).
		Do(context.Background(), httpx.Request{Op: "analyzers_list", Method: "GET", Template: srv.URL})
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
}

func TestPinnedHostAcceptsItsCertificate(t *testing.T) { /* pin srv.Cert (fake.Server's own field) → 200 */ }

// TestPinnedHostRejectsADifferentCertificate is the R4 fix: srvA and srvB are
// two SEPARATE fake.NewTLSServer calls, so — unlike two httptest.NewTLSServer
// calls — their certificates are provably distinct (require.NotEqual on the
// DER bytes is the test's own precondition check). Pinning srvB's cert under
// srvA's host is therefore observably wrong, and the mutation in Step 5(b)
// below can actually be caught.
func TestPinnedHostRejectsADifferentCertificate(t *testing.T) {
	srvA := fake.NewTLSServer(t, route200)
	srvB := fake.NewTLSServer(t, route200)
	require.NotEqual(t, srvA.Cert.Raw, srvB.Cert.Raw, "precondition: two fake servers must have distinct certs")
	pool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: map[string]string{hostOf(srvA.URL): base64.StdEncoding.EncodeToString(srvB.Cert.Raw)}})
	require.NoError(t, err)
	_, err = pool.Client(httpx.ClientConfig{Provider: "aril", LimiterKey: "k", Every: time.Millisecond, Burst: 1, MaxAttempts: 1}).
		Do(context.Background(), httpx.Request{Op: "analyzers_list", Method: "GET", Template: srvA.URL})
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
	require.Empty(t, srvA.Requests(), "the TLS handshake must fail before any HTTP request reaches the handler")
}

// TestPoolNeverAcceptsAnUnverifiedCertificate replaces a white-box struct
// inspection (R5): asserting that the transport's skip-verification field is false
// requires writing that field's literal name into this test file, which
// trips the existing substring guard TestNoTLSVerificationBypass
// (internal/arch/arch_test.go) — it scans every .go file under internal/,
// test files included, exempting only arch_test.go itself — and Task 17's
// filtered grep. This version proves the same property BEHAVIOURALLY instead:
// a request to a server that is neither pinned nor system-trusted must fail,
// no matter what else is pinned.
func TestPoolNeverAcceptsAnUnverifiedCertificate(t *testing.T) {
	pinned := fake.NewTLSServer(t, route200)
	unrelated := fake.NewTLSServer(t, route200)
	pool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: map[string]string{hostOf(pinned.URL): base64.StdEncoding.EncodeToString(pinned.Cert.Raw)}})
	require.NoError(t, err)
	_, err = pool.Client(httpx.ClientConfig{Provider: "aril", LimiterKey: "k", Every: time.Millisecond, Burst: 1, MaxAttempts: 1}).
		Do(context.Background(), httpx.Request{Op: "analyzers_list", Method: "GET", Template: unrelated.URL})
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
}
```

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/integration/httpx/ -race -v
```
Expected: FAIL — package not found / undefined `httpx.NewPool`.

- [ ] **Step 3: Implement.**
  - `pin.go`: a pinned host gets a dedicated `*http.Transport` whose `TLSClientConfig` is `{RootCAs: poolWithOnly(pinnedCert), ServerName: host, MinVersion: tls.VersionTLS12}`. Other hosts share a transport with system roots and `MinVersion: tls.VersionTLS12`. A `RoundTripper` selects by `req.URL.Hostname()`. httptest servers listen on `127.0.0.1`, and their certificate's IP SAN covers it.
  - `ratelimit.go`: `map[string]*rate.Limiter` behind a mutex, created with `rate.Every(c.Every)`; the wait uses the injected Sleep for testability.
  - `retry.go`: jitter via `rand.Int64N`.

- [ ] **Step 4: Run and watch them pass** (same command, then `-count=3`).

- [ ] **Step 5: Prove the secret and pinning guards fail.** Apply each mutation, confirm FAIL, restore:
  (a) wrap the transport error with `%w` in `classify.go`: `TestHTTPErrorsNeverContainSubstitutedSecrets` FAILS on the network-failure path;
  (b) make the pinned transport trust **any** certificate in the pins map instead of only the host's own (build one shared pool from every pin): `TestPinnedHostRejectsADifferentCertificate` FAILS;
  (c) set `MinVersion` to zero **and** make the shared (unpinned-host) transport accept any certificate in `pin.go`: `TestPoolNeverAcceptsAnUnverifiedCertificate` FAILS.
  Record the observed outputs.

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
git add internal/integration/httpx
git commit -m "feat(f2): httpx client with retries, jitter, rate limits, pinning and secret-free errors" <TRAILERS>
```

---

## Task 3: `normalize` — decimals, provider dates, units and the one range-chunking helper

**Files:**
- Create: `internal/integration/normalize/{decimal.go,time.go,units.go,window.go}`
- Test: `internal/integration/normalize/{decimal_test.go,time_test.go,units_test.go,window_test.go}`

**Interfaces:**
- Consumes: `shopspring/decimal` only. No project imports beyond `internal/domain/model` (for nothing but documentation; import it only if needed).
- Produces:

```go
package normalize

import _ "time/tzdata" // Europe/Istanbul must exist in a scratch image

var Istanbul *time.Location // time.LoadLocation("Europe/Istanbul"); panics at init if missing

var ErrEmpty = errors.New("normalize: empty value")

// ProviderNumber parses provider numeric strings per R10. Never via float.
func ProviderNumber(raw string) (decimal.Decimal, error)
// OptionalNumber: "", "null", "-" → (nil, nil).
func OptionalNumber(raw string) (*decimal.Decimal, error)
// JSONNumber converts a json.Number (decoder with UseNumber) exactly.
func JSONNumber(n json.Number) (decimal.Decimal, error)
func OptionalJSONNumber(n *json.Number) (*decimal.Decimal, error)

// LocalLayout parses raw in Europe/Istanbul and returns UTC.
func LocalLayout(layout, raw string) (time.Time, error)
// ISO8601 honours an explicit offset/Z; an offset-less value is Europe/Istanbul (removed-behaviour 20).
func ISO8601(raw string) (time.Time, error)
// OSOSDate parses "DD/MM/YYYY HH:mm:ss" as Istanbul local.
func OSOSDate(raw string) (time.Time, error)
// FormatOSOSDate renders a UTC instant as Istanbul "DD/MM/YYYY HH:mm:ss".
func FormatOSOSDate(t time.Time) string
// ARILProfileDate parses the 14-digit yyyyMMddHHmmss number as Istanbul local.
func ARILProfileDate(n int64) (time.Time, error)
// PM5340Date accepts ISO 8601 (offset or not) or "DD/MM/YYYY HH:mm[:ss]".
func PM5340Date(raw string) (time.Time, error)

// Multiply returns v × m, nil-preserving. The one multiplication point per adapter.
func Multiply(v *decimal.Decimal, m decimal.Decimal) *decimal.Decimal
func WhToKWh(v *decimal.Decimal) *decimal.Decimal // ÷ 1000, exact
func WToKW(v *decimal.Decimal) *decimal.Decimal   // ÷ 1000, exact
func KWFor15MinToKWh(v *decimal.Decimal) *decimal.Decimal // × 0.25 (R15)

// Window is a half-open time range [From, To). It is deliberately a plain
// struct, not job.Window (Task 1): normalize has no F2-task dependency
// (Consumes: shopspring/decimal only, above), and it must stay that way
// regardless of wave order. Callers that need job.Window (backfill, Task 15)
// convert with one field-for-field copy.
type Window struct{ From, To time.Time }

// Chunk splits [from, to) into consecutive half-open windows of at most max,
// each aligned to Europe/Istanbul local midnight so a window never splits a
// local day (06 §2 "requests to one provider are serialised per company";
// windows are also the pagination unit for OSOS, GridBox and EPİAŞ, R20).
//
// S7 (preflight): the plan originally implemented range chunking three
// times — inline in Task 10's pipeline, inline in Task 12's EPİAŞ client,
// and as backfill.Windows in Task 15, only the last of which was tested for
// gap/overlap/DST correctness. Chunk is now the ONE implementation. It lives
// here, not in internal/ingest or internal/ingest/backfill, because Task 3 is
// the earliest task that can own it without creating a same-wave dependency:
// after the S11 wave move, Task 3 is Wave A; Tasks 10 and 12 (Wave C) and
// Task 15 (Wave D) all consume it from a strictly earlier wave. Putting it in
// Task 1 or Task 5 (also Wave A) instead would have worked too, but Task 3
// already owns Istanbul-local-day arithmetic (Istanbul, ISO8601), so a second
// implementation of "local midnight" next to it would itself have been the
// kind of duplication this fix removes.
func Chunk(from, to time.Time, max time.Duration) []Window
```

- [ ] **Step 1: Write the failing tests**

```go
// TestOffsetlessTimestampIsIstanbulLocal is the unit half of the F2
// acceptance criterion "an offset-less provider timestamp is stored as the
// correct UTC instant for Europe/Istanbul". 2015-03-29 crosses the last
// Turkish DST change, proving the tz database, not a fixed +03:00, is used.
func TestOffsetlessTimestampIsIstanbulLocal(t *testing.T) {
	for _, tc := range []struct{ raw string; want time.Time }{
		{"2026-09-14T10:15:00", time.Date(2026, 9, 14, 7, 15, 0, 0, time.UTC)},
		{"2026-09-14T10:15:00Z", time.Date(2026, 9, 14, 10, 15, 0, 0, time.UTC)},
		{"2026-09-14T10:15:00+02:00", time.Date(2026, 9, 14, 8, 15, 0, 0, time.UTC)},
		{"2015-01-10T10:00:00", time.Date(2015, 1, 10, 8, 0, 0, 0, time.UTC)}, // winter, UTC+2
		{"2015-07-10T10:00:00", time.Date(2015, 7, 10, 7, 0, 0, 0, time.UTC)}, // summer, UTC+3
	} {
		got, err := normalize.ISO8601(tc.raw)
		require.NoError(t, err, tc.raw)
		require.True(t, tc.want.Equal(got), "%s: want %s got %s", tc.raw, tc.want, got)
		require.Equal(t, time.UTC, got.Location())
	}
}
```

Also:
- `TestOSOSDateRoundTrip`.
- `TestARILProfileDate`: `20260914101500` → `07:15Z`; 13 digits → error.
- `TestPM5340DateFormats`: ISO, `14/09/2026 10:15`, `14/09/2026 10:15:30`.
- `TestProviderNumberTable`, table-driven, every row of R10: `"1.234,56"→1234.56`, `"1,234.56"→1234.56`, `"1234,5"→1234.5`, `"1.234.567"→1234567`, `"12.5"→12.5`, `"1 234,5"→1234.5`, `"-3,25"→-3.25`, `"abc"`→error, `""`→`ErrEmpty`, `"123456789012.3456"` exact.
- `TestJSONNumberIsExact`: `json.Number("9007199254740993.0001")` has its string form preserved.
- `TestMultiplyPreservesNil`.
- `TestUnitConversionsAreExact`: `1500 Wh → 1.5 kWh`; `2.4 kW × 15 min → 0.6 kWh`.
- `window_test.go`, `TestChunkCoversRangeWithoutGapOrOverlap` (property, 200 random `[from, to)` ranges and `max` values: windows contiguous, union equals the input range, each window ≤ `max`); `TestChunkAlignsToIstanbulMidnight` (a range starting mid-day and a `max` of 24h still produce boundaries at Istanbul-local midnight, not `from + max`); `TestChunkEmptyRangeIsEmpty` (`from == to` → no windows).

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/integration/normalize/ -race -v
```
Expected: FAIL — package missing.

- [ ] **Step 3: Implement.** `ProviderNumber` normalises the string and calls `decimal.NewFromString`; it never touches `strconv.ParseFloat`, which Task 1's guard would catch. `ISO8601` tries `time.RFC3339Nano`, then the offset-less layouts `2006-01-02T15:04:05.999999999` and `2006-01-02 15:04:05` via `time.ParseInLocation(…, Istanbul)`, and returns `.UTC()`. `Chunk` walks `from` forward one Istanbul-local-midnight-aligned step at a time (`time.Date(y,m,d+1,0,0,0,0,Istanbul)` for the first boundary, then `+24h` wall-clock steps thereafter, each capped at `max` and at `to`) — never `from.Add(max)` blindly, which would drift across a DST change and stop landing on local midnight.

- [ ] **Step 4: Run and watch them pass.**

- [ ] **Step 5: Prove the timezone test fails.** Replace `ParseInLocation(…, Istanbul)` with `time.Parse`. `TestOffsetlessTimestampIsIstanbulLocal` must FAIL on the first row. Then replace `Istanbul` with `time.FixedZone("TR", 3*3600)`: it must FAIL on the 2015-01-10 row. Restore. Separately for `Chunk`: replace the Istanbul-midnight boundary with `from.Add(24 * time.Hour)`: `TestChunkAlignsToIstanbulMidnight` FAILS. Restore.

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
git add internal/integration/normalize
git commit -m "feat(f2): exact provider number, Istanbul-aware timestamp normalisation and the shared range-chunking helper" <TRAILERS>
```

---

## Task 4: The fake-provider harness, fixture sanitisation guard and the distributed lock

**Files:**
- Create: `internal/integration/fake/{server.go,responders.go,fixtures.go,sanitise.go}`
- Test: `internal/integration/fake/{server_test.go,sanitise_test.go}`
- Create: `internal/platform/lock/{lock.go,memory.go,redis.go}`
- Test: `internal/platform/lock/{memory_test.go,redis_integration_test.go}`

**Interfaces:**
- Consumes: nothing from Tasks 1–3 (the harness exposes pins as `map[string]string`, the same shape as `httpx.PoolOptions.PinnedCerts`).
- Produces:

```go
package fake

type RecordedRequest struct { Method, Path, RawQuery string; Header http.Header; Body []byte }

type Server struct {
	URL  string             // https://127.0.0.1:port
	Pins map[string]string  // {"127.0.0.1": base64(DER)} — pass to httpx.PoolOptions.PinnedCerts
	Cert *x509.Certificate  // this server's own certificate; R4, see below
}
// NewTLSServer starts an httptest TLS server routed by method+path; unmatched
// → 404 and t.Errorf. EVERY CALL mints a fresh ECDSA self-signed certificate
// (IP SAN 127.0.0.1) via crypto/ecdsa + x509.CreateCertificate, set on
// httptest.NewUnstartedServer(...).TLS before StartTLS (R4): Go's own
// httptest.NewTLSServer reuses ONE built-in certificate across every server
// in the process, so two httptest.NewTLSServer calls are byte-identical
// certificates — a "pin server B's cert under server A's host, expect
// rejection" test cannot observe anything with them, because it would
// actually be pinning server A's own certificate. Two NewTLSServer calls
// here are provably distinct (Task 2's TestPinnedHostRejectsADifferentCertificate
// depends on this).
func NewTLSServer(t *testing.T, routes ...Route) *Server
func (s *Server) Requests() []RecordedRequest

type Responder func(w http.ResponseWriter, r *http.Request)
type Route struct {
	Method, Path string
	Match        func(r *http.Request) bool // optional extra predicate (query/body)
	Respond      Responder
}
func JSON(status int, body []byte) Responder
func Raw(status int, contentType string, body []byte) Responder
func RateLimited(retryAfter string) Responder // 429 (+ Retry-After when non-empty)
func Sequence(rs ...Responder) Responder       // n-th call → n-th responder; last repeats

// Fixture reads internal/integration/<provider>/testdata/<name> relative to the module root.
func Fixture(t *testing.T, provider, name string) []byte

// RequiredCases is the F2 acceptance matrix every adapter test must cover.
var RequiredCases = []string{"success", "empty", "partial", "auth_failure", "malformed", "rate_limited", "pagination"}
```

```go
package lock

// lock is the CANONICAL owner of the Locker/Lease contract (R2). httpx
// (Task 2, which starts only after this task's Wave A merges) imports this
// package and aliases httpx.Locker = lock.Locker, httpx.Lease = lock.Lease —
// a type alias, not a structurally-identical redeclaration, which is what
// makes *Memory and *Redis satisfy httpx.Locker with no adapter anywhere in
// the plan (the original draft had two separately-named interfaces with
// identical method sets, which Go does NOT treat as satisfying each other
// when a method's return type is one of those named interfaces — preflight
// review). Ownership sits here, not in httpx, because after the S11 wave
// move this package has zero F2-task dependencies and is Wave A; if httpx
// (Wave B) owned the canonical type, Task 4 would need to import a
// not-yet-merged Task 2 package to implement it.
var ErrNotAcquired = errors.New("lock: not acquired")
type Locker interface { Acquire(ctx context.Context, key string, ttl time.Duration) (Lease, error) }
type Lease interface { Release(ctx context.Context) error }

// Memory is for unit tests and single-process use. now is injectable for TTL tests (nil: time.Now).
func NewMemory(now func() time.Time) *Memory
// Redis: SET key token NX PX ttl; waits by polling 50ms→1s (doubling) until ctx is done;
// Release deletes only if the stored token matches (Lua compare-and-delete). Keys are prefixed "ekokod:lock:".
func NewRedis(client goredis.UniversalClient) *Redis

// Consumer is a one-shot, NON-BLOCKING atomic consume (R3). Unlike Locker,
// which waits by polling until ctx is done, Consume never waits: there is
// nothing to wait FOR once a key is already gone. It exists for single-use
// tokens (the OAuth state nonce, Task 14) where a replay must fail fast, not
// queue behind the request that already consumed it — using Locker.Acquire
// with no Release for this (the original draft) made a replayed OAuth
// callback block for the lock's TTL, or forever under context.Background(),
// instead of returning an error.
type Consumer interface {
	// Consume marks key as used for ttl and reports whether THIS call is the
	// one that marked it. found=true: first use — proceed. found=false: the
	// key was already consumed (or, for Memory, is still within a previous
	// consume's ttl) — the caller rejects immediately, no waiting.
	Consume(ctx context.Context, key string, ttl time.Duration) (found bool, err error)
}
// *Memory and *Redis both implement Consumer:
// Memory.Consume: mutex-guarded map[string]expiry, one lookup-and-set, no loop.
// Redis.Consume: SET key 1 NX PX ttl — the NX failure IS the "already used"
// signal; there is no separate poll/wait path to accidentally introduce.
```

**Fixture provenance and sanitisation rules** (`sanitise.go`, enforced by `TestFixturesAreSanitised` over `internal/integration/*/testdata/**`):
- Fixtures are **hand-authored** from `06-integrations.md` field tables and the legacy response interfaces in `bcem-energy/src/utils/isolar/isolarTypes.ts`, `src/utils/arilService.ts`, `src/app/api/integration/{osos,gridbox,aril,pm5340}/refresh/route.ts` and `src/utils/epias/epiasService.ts`. **Never captured from a live endpoint.** A future captured response must pass this guard before commit.
- Allowed placeholder vocabulary:

| Kind | Allowed form |
|---|---|
| installation / wiring / subscription number, meter serial, ps_id, ps_key, device SN | `FX` + digits/underscores (e.g. `FX0000001`, `FX1001_14_1_1`) |
| customer / company / plant / title names | start with `Fixture ` |
| address, street, district, neighbourhood | start with `Fixture ` |
| tokens, passwords, keys, TGT, codes | start with `FIXTURE-` or `TGT-FIXTURE-` |
| e-mail | `@example.com` |
| hostnames in any URL | `127.0.0.1`, `example.invalid` |
| coordinates | latitude `39.000000`–`39.999999`, longitude `32.000000`–`32.999999` |

- Rejected anywhere: a string value under a key matching `(?i)pass|secret|token|key|tgt|code|auth` not in the allowed form; a JWT shape `eyJ[A-Za-z0-9_-]{10,}\.`; a TCKN-like `\b[1-9]\d{10}\b`; an IBAN `TR\d{24}`; a phone `\+?90\s?\d{3}`; e-mails not at `example.com`; URLs whose host is not allowed; values under name/address keys not starting `Fixture `; installation-like keys (`instalationNumber`, `wiringNo`, `SubscriptionSerno`, `OwnerSerno`, `ps_id`, `ps_key`, `device_sn`, `MeterSerial`, `meterNumber`) not starting `FX`.

- [ ] **Step 1: Write the failing tests**

`sanitise_test.go`:

```go
// TestFixturesAreSanitised walks every adapter's testdata. It passes
// vacuously on an empty tree today; Task 17's matrix guard proves the tree
// is populated. The rule set itself is proven by TestSanitiserRejects.
func TestFixturesAreSanitised(t *testing.T) {
	files := fake.FixtureFiles(t) // glob internal/integration/*/testdata/**/*.json
	for _, f := range files {
		for _, v := range fake.Violations(f.Path, f.Body) {
			t.Errorf("%s: %s", f.Path, v)
		}
	}
}

func TestSanitiserRejects(t *testing.T) {
	for name, body := range map[string]string{
		"real token":     `{"access_token":"a1b2c3d4e5"}`,
		"jwt":            `{"x":"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig"}`,
		"real name":      `{"customerName":"Acme Tekstil A.Ş."}`,
		"installation":   `{"instalationNumber":"40012345"}`,
		"email":          `{"mail":"ali@firma.com.tr"}`,
		"host":           `{"url":"https://gateway.isolarcloud.eu/x"}`,
		"tckn":           `{"note":"12345678901"}`,
		"iban":           `{"iban":"TR330006100519786457841326"}`,
		"password field": `{"Password":"hunter2"}`,
	} {
		require.NotEmpty(t, fake.Violations("probe.json", []byte(body)), name)
	}
	require.Empty(t, fake.Violations("ok.json", []byte(
		`{"customerName":"Fixture Customer 1","instalationNumber":"FX0000001","access_token":"FIXTURE-TOKEN-1","url":"https://127.0.0.1/x"}`)))
}
```

`server_test.go`:
- `TestFakeServerIsTLSAndRecordsRequests`: `NewTLSServer` URL is `https`; a request via a client whose `RootCAs` holds only the decoded pin succeeds; a plain `http.Client{}` fails with an x509 error, proving tests cannot pass without pinning.
- `TestTwoFakeServersHaveDistinctCertificates` (R4): two `NewTLSServer` calls give `!bytes.Equal(a.Cert.Raw, b.Cert.Raw)`; each server's own `Pins` value decodes back to its own `Cert.Raw`.
- `TestSequenceRepeatsLastResponder`.
- `TestUnmatchedRouteFailsTheTest`: run in a sub-`testing.T` via `testing.RunTests` or a recorded `t`; assert failure.

`memory_test.go`: `TestMemoryLockIsExclusiveUntilReleased`, `TestMemoryLockExpiresAfterTTL` (inject `now`), `TestAcquireRespectsContext`; `TestMemoryConsumeIsSingleUse` (first `Consume` → `found=true`; a second `Consume` on the same key before ttl → `found=false`; after `now` advances past ttl → `found=true` again); `TestMemoryConsumeNeverBlocks` (a second `Consume` on an unexpired key returns within a few milliseconds under `-race`, never waiting for ctx or ttl — unlike `Acquire`, there is no polling loop to time).

`redis_integration_test.go` (`//go:build integration`, `testfixtures.StartRedis`): `TestRedisLockExclusiveAcrossClients` (two `goredis.Client`s; the second `Acquire` with a 200ms ctx returns `ErrNotAcquired`/ctx error; after Release it succeeds); `TestRedisReleaseDoesNotDeleteAnotherHoldersLock` (the TTL expires, B acquires, A's stale Release leaves B's key present); `TestRedisConsumeIsSingleUseAndNonBlocking` (first `Consume` → `found=true`; a second `Consume` on the same key, called with `context.Background()` (no deadline at all), still returns `found=false` in well under 1s — proving `SET NX` is used, not a poll loop).

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/integration/fake/ ./internal/platform/lock/ -race -v
go test ./internal/platform/lock/ -tags=integration -race -count=1 -run TestRedisLock -v
```
Expected: FAIL — packages missing.

- [ ] **Step 3: Implement** per the Interfaces block. `Fixture` locates the module root by walking up to `go.mod`. `NewTLSServer` generates its ECDSA key and self-signed cert with a fresh `crypto/rand` read per call; `httptest.NewUnstartedServer(...).TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}` before `StartTLS()`.

- [ ] **Step 4: Run and watch them pass.**

- [ ] **Step 5: Prove the guards fail.** (a) Temporarily drop the installation-key rule; `TestSanitiserRejects/installation` must FAIL. (b) Add `internal/integration/fake/testdata/leak.json` with `{"access_token":"abc123"}`, change `FixtureFiles`' glob to include `fake/testdata`, and confirm `TestFixturesAreSanitised` FAILS naming the file. (c) Change Redis Release to an unconditional `DEL`; `TestRedisReleaseDoesNotDeleteAnotherHoldersLock` must FAIL. (d) Cache `NewTLSServer`'s certificate in a package-level variable instead of generating fresh per call; `TestTwoFakeServersHaveDistinctCertificates` must FAIL. (e) Make `Memory.Consume` re-check after a short sleep instead of a single atomic lookup-and-set (i.e. give it the same poll shape as `Acquire`); `TestMemoryConsumeNeverBlocks` must FAIL (or at least take orders of magnitude longer). Restore all five.

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
go test ./internal/platform/lock/ -tags=integration -race -count=1 -v
git add internal/integration/fake internal/platform/lock
git commit -m "feat(f2): pinned TLS fake-provider harness with per-server certs, fixture sanitiser, and a distributed lock with a non-blocking single-use consume" <TRAILERS>
```

---

## Task 5: Store seams — PM5340 generation, the OSOS cross-check series, credential dispatch, `SystemScope`

**Files:**
- Create: `internal/store/postgres/migrations/00012_pm5340_generation.sql`, `00013_provider_hourly_values.sql`
- Create: `internal/store/postgres/queries/{generation.sql,provider_series.sql,admin_ingestion.sql}`
- Create: `internal/store/postgres/{generation.go,provider_series.go}`, `internal/store/postgres/admin/ingestion.go`
- Test: `internal/store/postgres/{generation_integration_test.go,provider_series_integration_test.go,migrations_f2_integration_test.go}`, `internal/store/postgres/admin/ingestion_integration_test.go`
- Modify: `internal/store/repository.go` (append an **F2** section before the ADMIN banner, and one Admin interface at the end), `internal/store/scope.go`, `internal/store/scope_test.go`
- Modify: `internal/domain/model/timeseries.go`, `internal/domain/model/operations.go`
- Modify: `internal/store/postgres/readings.go`, `queries/readings.sql`, `readings_integration_test.go` (carry `interval_generation_kwh`)
- Modify: `internal/store/postgres/admin/doc.go` — F1's comment names "the five interfaces" and says "THE LIST OF METHODS IS CLOSED". `AdminIngestionRepository` below is a genuine sixth: it exists for exactly the reason the other five do (its caller — the scheduled dispatcher — has no tenant to offer a Scope), so it belongs in the same closed list, not as an exception to it. Add its paragraph (same shape as the other five: what it does, why it cannot take a Scope) and change "five" to "six" everywhere in the comment.
- Generated: `internal/store/postgres/sqlcgen/**`

**Interfaces:**
- Consumes: F1's `ReadingRepository`, `scrubErr`, the numeric conversion pair, `testfixtures.NewIsolatedDB`/`NewTenant`.
- Produces:

```go
// internal/store/scope.go
// SystemScope is the scope a background job uses when it acts for a whole
// company (ingestion, backfill, OAuth callback after state verification).
// It is explicit widening: AllBuildings is true. A zero companyID yields an
// invalid scope, which every repository rejects.
func SystemScope(companyID uuid.UUID) Scope

// internal/domain/model/timeseries.go
type MeterReading struct {
	// … F1 fields unchanged …
	// IntervalGenerationKwh is PM5340's interval energy (06 §5, R1); nil for every other provider.
	IntervalGenerationKwh *decimal.Decimal
}
type GenerationAnchor struct {
	AnalyzerID   uuid.UUID
	AnchorTs     time.Time
	ActiveExport decimal.Decimal // cumulative export at AnchorTs
	Source       string          // "initial" | "operator" | "migration"
	UpdatedAt    time.Time
}
type ProviderHourlyValue struct {
	AnalyzerID        uuid.UUID
	Ts                time.Time
	ActiveConsumption *decimal.Decimal
	ActiveGeneration  *decimal.Decimal
	SourceProvider    IntegrationProvider
	IngestedAt        time.Time
}

// internal/domain/model/operations.go
// CredentialRef identifies an active credential for platform dispatch. It
// carries NO ciphertext.
type CredentialRef struct {
	CredentialID, CompanyID, DefinitionID uuid.UUID
	Provider IntegrationProvider
	Subtype  string
}

// internal/store/repository.go — F2 section
// GenerationRepository — Isolation: generation_anchors has no company_id; join through analyzers.
type GenerationRepository interface {
	// Anchor returns ErrNotFound when the analyzer has none or is not visible.
	Anchor(ctx context.Context, s Scope, analyzerID uuid.UUID) (model.GenerationAnchor, error)
	// SetAnchor upserts on analyzer_id. Not visible → ErrNotFound, nothing written.
	SetAnchor(ctx context.Context, s Scope, a model.GenerationAnchor) error
}
// ProviderSeriesRepository reads and writes provider_hourly_values — the
// labelled cross-check series (06 §2, removed-behaviour 23). NEVER read by
// consumption or billing.
type ProviderSeriesRepository interface {
	// UpsertHourly is idempotent on (analyzer_id, ts); duplicate keys within rows → ErrConflict before any
	// round trip; any analyzer not visible → ErrNotFound, nothing written.
	UpsertHourly(ctx context.Context, s Scope, rows []model.ProviderHourlyValue) (inserted, updated int, err error)
	HourlyRange(ctx context.Context, s Scope, analyzerID uuid.UUID, r TimeRange) ([]model.ProviderHourlyValue, error)
}

// ADMIN section
// AdminIngestionRepository lists the credentials the scheduled dispatcher
// must fan out to. It cannot take a Scope: the dispatcher acts for no tenant
// and must see every tenant's active credentials. Implemented by F2 Task 5.
type AdminIngestionRepository interface {
	// ActiveCredentials returns every is_active credential of a non-deleted
	// company, ordered by (company_id, credential id). No secret columns are selected.
	ActiveCredentials(ctx context.Context) ([]model.CredentialRef, error)
}
```

Constructors: `postgres.NewGenerationRepository(pool)`, `postgres.NewProviderSeriesRepository(pool)`, `admin.NewIngestionRepository(pool)`.

- [ ] **Step 1: Write the failing migration tests**

`migrations_f2_integration_test.go` (R8: the original draft cited `TestMigrationsRoundTrip`, which does not exist on `phase/f1-data-model`, and a shim-declaration guard, `TestShimDeclaresEveryTimescaleFunctionTheMigrationsUse`, that was planned in F1's handoff notes but never actually landed as code — S9/S10. Neither is relied on below):
- `TestF2MeterReadingsHasIntervalGenerationColumn`: `information_schema.columns` shows `interval_generation_kwh` `numeric` with precision 18, scale 4, nullable.
- `TestF2GenerationAnchorsTable`: columns, PK `analyzer_id`, check `active_export >= 0`.
- `TestF2ProviderHourlyValuesIsAHypertable`: `timescaledb_information.dimensions` gives a 30-day time interval.
- `TestF2IntervalColumnSurvivesCompressedChunk`: insert an old reading with `interval_generation_kwh = 0.6250`, `compress_chunk` on its chunk, read back exactly `0.6250`; then prove the `add column`/`drop column` pair round-trips by running `migrate down 2` (rolls back `00013` then `00012` — a single `down 1` only reverts `00013`, R8) followed by `migrate up` (replays both), and re-reading the same row.

The real F1 reversibility guards are `TestMigrateUpDownUp` (`internal/store/postgres/postgres_integration_test.go`, full up/down/up) and `TestMigrationsLeaveNoTablesBehind` (`internal/store/postgres/migrations_roundtrip_integration_test.go`, a full down leaves no tables); run both alongside the tests above — they are what actually proves `00012`/`00013` reversibility, not a `TestMigrationsRoundTrip` that was never written.

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/store/postgres/ -tags=integration -race -count=1 -run 'TestF2|TestMigrat|TestHypertablesAreConfigured' -v
```
Expected: FAIL — column/table missing. (The filter now actually matches `TestMigrateUpDownUp` and `TestMigrationsLeaveNoTablesBehind`, which `-run 'TestF2|TestMigrations'` did not.)

- [ ] **Step 3: Write the migrations**

```sql
-- 00012_pm5340_generation.sql
-- +goose Up
alter table meter_readings add column interval_generation_kwh numeric(18,4);

create table generation_anchors (
    analyzer_id   uuid primary key references analyzers(id),
    anchor_ts     timestamptz   not null,
    active_export numeric(18,4) not null check (active_export >= 0),
    source        text          not null check (source in ('initial','operator','migration')),
    updated_at    timestamptz   not null default now()
);

-- +goose Down
drop table if exists generation_anchors;
alter table meter_readings drop column if exists interval_generation_kwh;
```

```sql
-- 00013_provider_hourly_values.sql
-- +goose Up
create table provider_hourly_values (
    analyzer_id        uuid        not null references analyzers(id),
    ts                 timestamptz not null,
    active_consumption numeric(18,4),
    active_generation  numeric(18,4),
    source_provider    integration_provider not null,
    ingested_at        timestamptz not null default now(),
    primary key (analyzer_id, ts)
);
select create_hypertable('provider_hourly_values', 'ts', chunk_time_interval => interval '30 days');

-- +goose Down
drop table if exists provider_hourly_values;
```

**If TimescaleDB refuses `add column`/`drop column` on the columnstore-enabled `meter_readings` inside a transaction** (compare F1's SQLSTATE 25001 on `00005`), add `-- +goose NO TRANSACTION` to `00012` and record the SQLSTATE in the report. If it refuses outright, **stop and report**; do not work around it with a side table, because that would change R1.

- [ ] **Step 4: Regenerate and run the migration tests**

```bash
make generate
go test ./internal/store/postgres/ -tags=integration -race -count=1 -run 'TestF2|TestMigrat|TestHypertablesAreConfigured' -v
```
Expected: PASS. `create_hypertable` runs as plain SQL inside the migration itself, never through sqlc, so it needs no `timescale-shims.sql` entry — sqlc's catalogue simply does not see it, the same way it never sees the migration files' DDL directly. There is no F1 guard that cross-checks shim declarations against migration usage (`TestShimDeclaresEveryTimescaleFunctionTheMigrationsUse` was planned in F1's handoff notes but never implemented — S10); `00012`/`00013`'s correctness rests on `TestMigrateUpDownUp` and `TestMigrationsLeaveNoTablesBehind` against the real TimescaleDB test container, both run above.

- [ ] **Step 5: Write the failing repository tests**

`generation_integration_test.go`:
- `TestGenerationAnchorRoundTrip`.
- `TestGenerationAnchorIsScoped`: tenant B's scope on tenant A's analyzer gives `ErrNotFound` for both methods, and nothing is written.
- `TestGenerationAnchorRefusesNarrowScopeOnOtherBuilding`: `tenant.Scope` (building 0 only) on an analyzer of building 1 gives `ErrNotFound`.

`readings_integration_test.go` (append, helpers prefixed `f2gen`): `TestReadingsCarryIntervalGenerationKwh` (BulkInsert then Range returns the exact decimal; nil stays nil; a re-upsert changing only `active_export` keeps `interval_generation_kwh`).

`provider_series_integration_test.go`: `TestProviderHourlyUpsertIsIdempotent` (same rows twice → `inserted` then `updated`, count unchanged); `TestProviderHourlyRefusesDuplicateKeysInBatch` (`ErrConflict`, nothing written); `TestProviderHourlyAllOrNothingOnInvisibleAnalyzer`; `TestProviderHourlyRangeRejectsInvalidRange`.

`admin/ingestion_integration_test.go`:

```go
func TestActiveCredentialsSpansTenantsAndCarriesNoSecret(t *testing.T) {
	// two tenants, each with one active and one inactive credential, plus a
	// credential on a soft-deleted company. Expect exactly the two active
	// ones of live companies, ordered by company_id. Reflect over
	// model.CredentialRef: no field named *Enc, *Secret*, *Token*, *Password*.
}
```

`scope_test.go`: `TestSystemScopeGrantsWholeCompany` (`AllowsBuilding(any non-nil)` true; `Valid`); `TestSystemScopeOfNilCompanyIsInvalid`.

- [ ] **Step 6: Run and watch them fail; implement.** Queries are named `GenerationAnchorGet`, `GenerationAnchorUpsert`, `ProviderHourlyVisibleAnalyzerIDs`, `ProviderHourlyRange`, `AdminIngestionActiveCredentials`, and `ReadingsF2…` for any readings query that changes. The anchor write embeds the tenant predicate in the write statement itself (F1 `ba2fb97` pattern): `insert … select … from analyzers a where a.id = $1 and a.company_id = $2 and a.deleted_at is null and (all_buildings or a.building_id = any(building_ids))`, with `RowsAffected == 0 → ErrNotFound`. **Do not** pre-check visibility with `Scope.AllowsBuilding(storedBuildingID)`, which would be using AllowsBuilding to validate a stored FK. Duplicate detection in a batch happens in Go before the transaction and names only the key.

  **`UpsertHourly` (R7):** the original draft named sqlc queries `ProviderHourlyStageCopy`/`ProviderHourlyUpsertFromStage`, but a per-call temporary table cannot appear in sqlc's schema catalogue — `sqlc.yaml` builds its catalogue from `timescale-shims.sql` + `migrations/` only, exactly the reason F1's `ReadingRepository.BulkInsert` (`internal/store/postgres/readings.go`) bypasses sqlc for its own staging table. `UpsertHourly` follows that file's pattern exactly, in `provider_series.go`, prefix `f2series…`:
  1. `ProviderHourlyVisibleAnalyzerIDs` (a real sqlc query, mirroring `ReadingVisibleAnalyzerIDs`) locks the batch's distinct analyzer ids `FOR SHARE` under the caller's `Scope` (company + building predicate) inside one transaction. Fewer visible ids than distinct input ids → the whole batch is refused with `ErrNotFound`, nothing written — **this is the tenant predicate for this write**, evaluated once per batch, not embedded in the upsert SQL.
  2. `f2seriesCreateProviderHourlyStaging` — a plain SQL constant (not sqlc, not a migration; `on commit drop`, unseen by sqlc's catalogue) creating a temp table with `provider_hourly_values`' writable columns.
  3. `tx.CopyFrom` into that staging table.
  4. `f2seriesUpsertProviderHourlyFromStaging` — a plain SQL constant: `insert … select … from provider_hourly_values_staging on conflict (analyzer_id, ts) do update … returning (xmax = 0) as inserted`, counted into `inserted`/`updated`.
  Duplicate `(analyzer_id, ts)` keys inside the caller's own batch are still refused in Go before any of this, per the Global Constraints "batches are all-or-nothing" rule — the same defence `BulkInsert` uses.

- [ ] **Step 7: Run everything, including the F1 guards**

```bash
make generate && make check-generate
go test ./internal/store/... -tags=integration -race -count=1 -v
go test ./internal/arch/ -race -run 'TestEveryStoreMethodIsScoped|TestNoFloatFieldsInModelOrStore' -v
```
Expected: PASS. `TestEveryStoreMethodIsScoped` inspects more methods than before (record both counts). `admin.IngestionRepository` is exempt by package.

- [ ] **Step 8: Prove the new isolation fails when broken.** Remove the `company_id` predicate from `GenerationAnchorUpsert`, regenerate, and confirm `TestGenerationAnchorIsScoped` FAILS. Then remove the building predicate only, and confirm `TestGenerationAnchorRefusesNarrowScopeOnOtherBuilding` FAILS. **(S4)** `UpsertHourly` makes the same isolation claim ("any analyzer not visible → `ErrNotFound`, nothing written") and needs its own failing-first proof: remove the `company_id` predicate from `ProviderHourlyVisibleAnalyzerIDs` (make it a tautology, e.g. `company_id = company_id`), regenerate, and confirm `TestProviderHourlyAllOrNothingOnInvisibleAnalyzer` FAILS (another tenant's analyzer is now "visible" and the batch wrongly succeeds). Restore and regenerate after each.

- [ ] **Step 9: Gate and commit**

```bash
make lint && make test && make build && make check-generate
go test ./... -tags=integration -race -count=1
git add internal/store internal/domain/model
git commit -m "feat(f2): generation anchors, provider hourly series, credential dispatch listing and SystemScope" <TRAILERS>
```

---

## Adapter task template (Tasks 6, 7, 8, 9 — and the fixture half of 12 and 13)

Each adapter task follows these steps exactly. The per-provider task below supplies only what differs: endpoints, mapping, quirks and the named behaviour tests.

**Files (per provider `<p>`):** `internal/integration/<p>/{source.go,wire.go,mapping.go,source_test.go,mapping_test.go}` and `internal/integration/<p>/testdata/<p>_<case>.json` for every case in `fake.RequiredCases`, plus any extra fixtures named below.

**Construction (same shape for every meter adapter):**

```go
func New(pool *httpx.Pool, o Options) *Source   // Options{Clock clock.Clock; PageBudget int (default 10)}
var _ integration.Adapter = (*Source)(nil)
```

Each call builds its client with `pool.Client(httpx.ClientConfig{Provider: …, LimiterKey: "<p>:" + creds.CompanyID.String(), Every/Burst/RequestTimeout: per "Provider defaults", SerializeKey: "<p>:" + creds.CompanyID.String() where serialised})`. Endpoints come **only** from `creds.Endpoints` templates via `httpx.Request.Template`/`Params`; secrets are `Param{Secret: true}`. JSON is decoded into structs whose numeric fields are `json.Number` or `string`, never `float64`, which `TestIntegrationTreesDoNotParseFloats` enforces. `wire.go` holds only provider-shaped structs; `mapping.go` holds only pure functions from wire structs to `model.MeterReading`/`integration.MeteringPoint`.

**Normalisation obligations (06 §1):**
- set `AnalyzerID = req.AnalyzerID`, `Kind = req.Kind`, `SourceProvider`, `MultiplierApplied`;
- every register goes through `normalize.Multiply` exactly once;
- timestamps go through `normalize`;
- readings outside `[req.From, req.To)` are dropped;
- a row that fails to parse is skipped with `Warning{Code: WarnUnparseableRow, Detail: "<field> at row <n>"}` (the partial case);
- a body that is not the expected shape at all is `&integration.Error{Kind: ErrMalformedPayload}`;
- an empty list is an empty result, **not** an error — except GridBox `ResultStatus ≠ 1` (R25).

**Step 1 — Write the fixture matrix test first** (`source_test.go`):

```go
// Test<P>FixtureMatrix is the F2 acceptance criterion "every adapter has
// recorded fixtures covering success, empty result, partial data,
// authentication failure, malformed payload, rate limiting, and pagination".
// The subtest names are exactly fake.RequiredCases; Task 17's guard checks them.
func Test<P>FixtureMatrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		routes  func(t *testing.T) []fake.Route
		wantErr error                       // nil for success-like cases
		check   func(t *testing.T, res integration.FetchResult)
	}{
		{name: "success", …},
		{name: "empty", …},        // zero readings, nil error, nil NextCursor
		{name: "partial", …},      // k valid readings + ≥1 WarnUnparseableRow
		{name: "auth_failure", …, wantErr: integration.ErrAuth},
		{name: "malformed", …, wantErr: integration.ErrMalformedPayload},
		{name: "rate_limited", …}, // fake.Sequence(fake.RateLimited("1"), success) → success; recorded sleep 1s
		{name: "pagination", …},   // R20: provider pages or window chunks merged, in order, no duplicates
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t, tc.routes(t)...)
			src := <p>.New(<p>TestPool(t, srv), <p>.Options{Clock: clock.NewFake(fixtureNow)})
			res, err := src.FetchReadings(context.Background(), <p>TestCreds(srv), <p>TestRequest())
			if tc.wantErr != nil { require.ErrorIs(t, err, tc.wantErr); return }
			require.NoError(t, err)
			tc.check(t, res)
		})
	}
}
```

`<p>TestPool` builds `httpx.NewPool(httpx.PoolOptions{PinnedCerts: srv.Pins, Sleep: recordingSleep})`. **No test constructs an `http.Client`.**

Also required in every adapter:
- `TestIdempotent<P>RefetchYieldsIdenticalReadings`: the same fixture served twice gives `require.Equal` result slices, after sorting by `(Ts, Kind)` and comparing decimals with `Equal`. Named for 09's `-run TestIdempotent` verification.
- `Test<P>VerifyMapsAuthFailure`.
- `Test<P>DiscoverMapsEveryField`: every row of the 06 mapping table asserted by name.
- `Test<P>NeverReturnsReadingsOutsideWindow`.
- `Test<P>ErrorsCarryNoCredential`: the auth-failure fixture's server echoes the password; `err.Error()` does not contain it.

**Step 2** run `go test ./internal/integration/<p>/ -race -v` → FAIL (package missing). **Step 3** author sanitised fixtures (Task 4 vocabulary) and implement. **Step 4** PASS, then `go test ./internal/integration/fake/ -run TestFixturesAreSanitised -v` → PASS. **Step 5** prove the named behaviour test fails by the mutation the task names. **Step 6** gate and commit:

```bash
make lint && make test && make build
git add internal/integration/<p>
git commit -m "feat(f2): <p> adapter with fixture matrix" <TRAILERS>
```

---

## Task 6: OSOS adapter

**Files:** per template, `<p>` = `osos`. Extra fixtures: `osos_hourly_values.json`, `osos_discover.json`.

**Interfaces:**
- Consumes: Tasks 1–4. `creds.Username`, `creds.Secret`, `creds.Endpoints` keys `authentication`, `analyzers_list`, `hourly_values`, `energy_values`.
- Produces: `osos.New(pool *httpx.Pool, o osos.Options) *osos.Source`. `Kinds` → `[load_profile, daily, reset]`. `MaxWindow` → 30 days.

**Behaviour (06 §2, R7, R8, R10):**
1. Auth: `POST` template `authentication` with `{username_or_email}`, `{secret_password}` (Secret). Response `{"access_token": "…"}`. HTTP 401/403, or 200 with an empty `access_token`, is `ErrAuth`. The token is cached for the duration of one `FetchReadings`/`Discover` call only.
2. Discover: `GET analyzers_list` with `{secret_token}` → `instalation_list[]`. Map every field of the 06 §2 table; `kuruluGucu`, `koordinatX/Y` and `meterMultiplier` go through `normalize.ProviderNumber`.
3. Readings: `GET energy_values` with `{secret_token}`, `{start_date}`, `{end_date}` (`normalize.FormatOSOSDate`), `{installationNumber}`, `{dataType}` (`load_profile→1`, `daily→2`, `reset→3`). Response `{"energy": [ {meter_date, meter_serial_no, t_top_kWh, t_ri_kVarh, t_rc_kVarh, t_t1_kWh, t_t2_kWh, t_t3_kWh, t_p_kW, u_top_kWh, u_ri_kVarh, u_rc_kVarh, u_u1_kWh, u_u2_kWh, u_u3_kWh, u_p_kW} ]}` (legacy `osos/refresh/route.ts:314-331`). Map: `t_top→active_import`, `t_ri→reactive_inductive_import`, `t_rc→reactive_capacitive_import`, `t_t1..3→t1..3_import`, `t_p_kW→max_demand_kw`, `u_top→active_export`, `u_ri→reactive_inductive_export`, `u_rc→reactive_capacitive_export`, `u_u1..3→t1..3_export`. `u_p_kW` has no canonical register (02 §2.1 has one `max_demand`), so it is dropped with a code comment citing 02 §2.1. Multiply by `req.Multiplier` (R8).
4. Hourly cross-check (kind `load_profile` calls only): `GET hourly_values` once per calendar month intersecting the window (`{meter_month}`=`YYYY-MM`, `{from_date}`=`FormatOSOSDate(max(From, month start))`). Response `items[<installationNumber>][].valueList[] {meter_date, activeConsumption, activeGeneration}` → `FetchResult.HourlyValues`, **never** `Readings` (removed-behaviour 23).

**Named behaviour tests:**
- `TestOSOSHourlyValuesNeverMixIntoReadings`: fixture with both series; `Readings` contains only `energy` rows and `HourlyValues` only hourly rows.
- `TestOSOSThousandsSeparatorsParseExactly`: `"12.345,678"` → `12345.678` × multiplier `"40"` → `493827.12`.
- `TestOSOSRequestsDataTypePerKind`: recorded query carries `dataType=1|2|3`.
- `TestOSOSDateIsIstanbulLocal`: `meter_date "01/09/2026 00:15:00"` → `2026-08-31T21:15:00Z`.
- Pagination (R20): a 45-day request makes two `energy_values` calls (30 + 15 days), merged and sorted, with `NextCursor` nil.

**Mutation to prove:** map `u_ri` to `reactive_capacitive_export`; `Test<P>DiscoverMapsEveryField`'s reading-side companion `TestOSOSRegisterMapping` must FAIL. Add that test explicitly: one fixture row with 14 distinct values, asserted register by register.

---

## Task 7: GridBox adapter

**Files:** per template, `<p>` = `gridbox`. Extra fixtures: `gridbox_last_endex_multiplier.json`, `gridbox_load_profile_ratio.json`, `gridbox_no_multiplier.json`, `gridbox_result_status_error.json`, `gridbox_offsetless.json`.

**Interfaces:**
- Consumes: Tasks 1–4. Endpoint keys `token`, `last_success_date`, `last_endex`, `load_profiles`, `endexes`, `energy_values`. `creds.Settings.UseBillingIndexes`.
- Produces: `gridbox.New(pool, gridbox.Options) *gridbox.Source`. `Kinds(creds)` → `[load_profile, daily, reset, current_index]`, plus `billing` iff `UseBillingIndexes`. `MaxWindow` 30 days. Also exported pure function `gridbox.ResolveMultiplier(lastEndex *LastEndex, profiles []LoadProfileRow) integration.ResolvedMultiplier` (tested directly).

**Behaviour (06 §3, R25):**
1. Token: `POST token`, form body `username`, `password` (Secret), `grant_type=password` (legacy `gridbox/refresh/route.ts:93-113`). A 400/401 with `invalid_grant` is `ErrAuth`. `Authorization: Bearer <token>` goes on every call.
2. Every response is `{ResultStatus, ResultObject}`. `ResultStatus != 1` → `ErrUpstreamUnavailable` with `Op` set, and no readings.
3. `last_success_date` bounds `From` from below when it is later (`ProviderHighWater[kind]`); `last_endex` gives the multiplier source 1 and a `current_index` reading.
4. Kind → call: `load_profile→load_profiles`, `daily→endexes isBilling=false`, `billing→endexes isBilling=true`, `reset→energy_values`, `current_index→last_endex`. `{startDate}`/`{endDate}` are `YYYY-MM-DD` Istanbul local dates (legacy `getDate90DaysAgo`, `getCurrentDay`).
5. Multiplier resolution in order: (1) `Multiplier` on the last endex; (2) `ActiveEndexWithMultiplier / ActiveEndex` from the first load-profile row where both are present and the denominator is non-zero (decimal division, `DivRound(…, 6)`); (3) `1` with `Warning{Code: WarnMultiplierFallback}`. The resolved value is stamped as `MultiplierApplied` on every reading and returned in `FetchResult.ResolvedMultiplier`. When a `…WithMultiplier` field is present the adapter uses it **as-is** (already multiplied); otherwise it multiplies the raw field. Either way the multiplier is applied exactly once.
6. Field mapping: exactly the 06 §3 table, **`RI → inductive`, `RC → capacitive`**, in one function `mapRegisters`.
7. Timestamps: `ProfileDateTime`/`ProfileDate`/`EndexDate`/`ReadDate` via `normalize.ISO8601` (offset-less = Istanbul).

**Named behaviour tests (acceptance):**

```go
// TestGridBoxMultiplierResolution is the F2 acceptance criterion "GridBox
// multiplier resolution is tested for all three paths, including the
// fallback-to-1 case which must emit a warning".
func TestGridBoxMultiplierResolution(t *testing.T) {
	t.Run("last_endex", func(t *testing.T) { /* Multiplier "80" → Value 80, Source MultiplierFromLastEndex, no warning; readings stamped 80 */ })
	t.Run("derived_from_load_profile", func(t *testing.T) { /* no Multiplier; ActiveEndex 12.5, ActiveEndexWithMultiplier 500 → 40, Source MultiplierFromLoadProfile */ })
	t.Run("zero_denominator_skips_to_next_row", func(t *testing.T) { /* first row ActiveEndex 0 ignored */ })
	t.Run("fallback_to_one_warns", func(t *testing.T) {
		// neither source present → Value 1, Source MultiplierFallbackOne, and
		// res.Warnings contains exactly one WarnMultiplierFallback naming the wiring number.
	})
}

// TestGridBoxReactiveRegistersAreNotCrossed is the F2 acceptance criterion
// and removed-behaviour 19. Distinct values make a swap observable.
func TestGridBoxReactiveRegistersAreNotCrossed(t *testing.T) {
	// one load-profile row: RI=111, RC=222, RIOut=333, RCOut=444, multiplier 1
	// require ReactiveInductiveImport==111, ReactiveCapacitiveImport==222,
	//         ReactiveInductiveExport==333, ReactiveCapacitiveExport==444.
	// Repeat with the long-name variants ReactiveInductiveEndex/ReactiveCapacitiveEndex(Out)
	// — the legacy crossed exactly one of the two paths.
}
```

- `TestGridBoxOffsetlessTimestampIsIstanbul` (fixture `ProfileDate "2026-09-14T10:15:00"` → `07:15Z`).
- `TestGridBoxResultStatusErrorIsSurfaced` (`ResultStatus: 0` → `ErrUpstreamUnavailable`, zero readings).
- `TestGridBoxBillingOnlyWhenEnabled` (recorded requests contain `isBilling=true` iff the setting is on).

**Mutation to prove:** swap `RI`/`RC` in the long-name path only; `TestGridBoxReactiveRegistersAreNotCrossed` FAILS. Separately delete the fallback warning append; `…/fallback_to_one_warns` FAILS.

---

## Task 8: ARIL adapter

**Files:** per template, `<p>` = `aril`. Extra fixtures: `aril_token_bare_string.json`, `aril_token_object.json`, `aril_current_endexes.json`, `aril_end_of_month.json`, `aril_subscriptions_page1.json`, `aril_subscriptions_page2.json`.

**Interfaces:**
- Consumes: Tasks 1–4. Endpoint keys `authentication`, `analyzers_list`, `owner_consumptions`, `current_endexes`, `end_of_month_endexes`.
- Produces: `aril.New(pool, aril.Options) *aril.Source`. `Kinds` → `[load_profile, current_index, billing]`. `MaxWindow` 30 days.

**Behaviour (06 §4, R9, R26):**
1. Auth: `POST {UserCode, Password}`. The token comes back as a bare JSON string **or** `{access_token}`; both are accepted. A 401 or an empty token is `ErrAuth`. The header is `aril-service-token: <token>`.
2. Discover: `POST analyzers_list {PageNumber, PageSize: 1000}` → `ResultList[]`. Pages advance until a page has fewer than 1000 rows or the page budget is hit, which is the pagination case. Map the 06 §4 subscription table. `LastEndexDate`/`LastProfileDate` → `ProviderHighWater`; `AccordPower` → `ContractedPowerKw` (R30: captured on `MeteringPoint` and left in `Raw`; Task 10's `SyncAnalyzers` does **not** persist it — `model.Analyzer` has no matching column); `DefinitionType` → `DefinitionType`.
3. `load_profile`: `POST owner_consumptions {OwnerSerno, StartDate, EndDate ("YYYY-MM-DD HH:mm:ss" Istanbul), IncludeLoadProfiles: true, OwnerType: DefinitionType, WithoutMultiplier: true (R9), MergeResult: false}`. `LoadProfiles[]`: `ProfileDate` via `normalize.ARILProfileDate`. `TSum→active_import`, `ReactiveInductive→reactive_inductive_import`, `ReactiveCapasitive→reactive_capacitive_import`, `TSumOut→active_export`, `ReactiveInductiveOut→reactive_inductive_export`, `ReactiveCapasitiveOut→reactive_capacitive_export`, each × `req.Multiplier`. **T1/T2/T3 import and export stay nil.**
4. `current_index`: `POST current_endexes {OwnerSerno, StartDate, EndDate, DefinitionType, EndexDirection: 0}` → per R26.
5. `billing`: `POST end_of_month_endexes` for the window; `max_demand_kw` = max `MaxDemand` of the same local month's current endexes (a second call within the same `FetchReadings`).
6. A response with `ErrorCode != 0` on a data call → `ErrUpstreamUnavailable`. The legacy returned `[]` and swallowed it (removed-behaviour 27).

**Named behaviour tests:**

```go
// TestARILTimeOfUseRegistersAreNull is the F2 acceptance criterion "ARIL
// rows produce NULL, not 0, in the T1/T2/T3 registers" (removed-behaviour 21).
func TestARILTimeOfUseRegistersAreNull(t *testing.T) {
	// success fixture → every reading: T1Import, T2Import, T3Import,
	// T1Export, T2Export, T3Export are nil (require.Nil, not decimal.Zero),
	// while ActiveImport is non-nil.
}
```

- `TestARILAcceptsBothTokenShapes`.
- `TestARILRequestsWithoutMultiplierAndMultipliesOnce`: recorded body has `"WithoutMultiplier":true`; `TSum "12.5"` × 40 → `500`.
- `TestARILMaxDemandIsMonthlyMaximum`.
- `TestARILProfileDateIsIstanbulLocal`.
- `TestARILErrorCodeIsNotSwallowed`.

**Mutation to prove:** set `T1Import: decimalPtr("0")` in `mapping.go`; `TestARILTimeOfUseRegistersAreNull` FAILS.

---

## Task 9: PM5340 adapter

**Files:** per template, `<p>` = `pm5340`. Extra fixtures: `pm5340_page1.json`, `pm5340_page2.json`, `pm5340_null_generation.json`, `pm5340_ddmmyyyy.json`.

**Interfaces:**
- Consumes: Tasks 1–5 (`model.MeterReading.IntervalGenerationKwh`). `creds.BaseURL`; no endpoint templates (06 §5 fixes the path).
- Produces: `pm5340.New(pool, pm5340.Options) *pm5340.Source`. `Kinds` → `[load_profile]`. `MaxWindow` 7 days. PM5340 has no discovery API: **spec silent — ruling:** `DiscoverMeteringPoints` returns `nil, nil`, and the credential service creates the single analyzer from the configured installation number (Task 14).

**Behaviour (06 §5, R15, R28):**
1. **Two separate URL templates (S5), not one.** `httpx.Expand` refuses a template with an unbound placeholder or an unused param (Task 2), so the four-parameter fetch template and a `Verify` call cannot share one string — `Verify`'s only need is "does this base URL and secret work", not a real page of data. `fetchTemplate = strings.TrimSuffix(creds.BaseURL, "/") + "/api/v1/readings?limit={limit}&sort=asc&start={start}&end={end}"`, plus `&cursor={cursor}` when paging, all through `httpx.Expand`, params `{limit, sort, start, end[, cursor]}` (a real fetch: `GET …/api/v1/readings?limit=500&sort=asc&start=<RFC3339 UTC>&end=<RFC3339 UTC>[&cursor=…]`). `verifyTemplate = strings.TrimSuffix(creds.BaseURL, "/") + "/api/v1/readings?limit={limit}"`, the ONLY param `{limit}` (value `"1"`); `Verify` calls this one, never `fetchTemplate`. Follow `cursorNext` while `hasMore`, up to the page budget; if the budget is hit with `hasMore` still true, `NextCursor` = last reading's ts (R4).
2. Map: `meterDate` via `normalize.PM5340Date`; `activeImport_kWh→active_import`, `inductive_kvarh→reactive_inductive_import`, `capacitive_kvarh→reactive_capacitive_import`, `dmdKwPeak_kW→max_demand_kw`, each × `req.Multiplier`. `currentGeneration` (kW) → `IntervalGenerationKwh = normalize.KWFor15MinToKWh` (nil stays nil, plus `WarnGenerationIntervalNil`). **`ActiveExport` is left nil**: the cumulative register is derived by Task 11, never in the adapter (removed-behaviour 22). `deviceId`/`deviceIp` go to `Raw` only.
3. `null` register values stay nil.

**Named behaviour tests:**
- `TestPM5340NeverDerivesCumulativeExport`: every reading has `ActiveExport == nil`, and `IntervalGenerationKwh == currentGeneration × 0.25`.
- `TestPM5340NullsStayNull`.
- `TestPM5340FollowsCursorPages`: the pagination case; the second request carries `cursor=<cursorNext>`.
- `TestPM5340AcceptsBothDateFormats`.
- `TestPM5340PlainHTTPBaseURLIsAllowedAndHTTPSIsVerified`: an http fake succeeds; an https fake without a pin fails.
- `TestPM5340VerifyUsesItsOwnMinimalTemplate` (S5): the recorded `Verify` request's path is `/api/v1/readings?limit=1` with no `sort`/`start`/`end`/`cursor` params; a `FetchReadings` call against the same fake, by contrast, always carries all of `sort`/`start`/`end`.

**Mutation to prove:** accumulate `currentGeneration` into `ActiveExport` in the adapter; `TestPM5340NeverDerivesCumulativeExport` FAILS.

---

## Task 10: The ingestion pipeline — dispatch, sync, fetch, validation, anomalies, run records

**Files:**
- Create: `internal/ingest/{service.go,deps.go,dispatch.go,sync.go,fetch.go,dedupe.go,validate.go,anomaly.go,report.go}` (keep Task 1's `doc.go`)
- Test: `internal/ingest/{dedupe_test.go,validate_test.go,anomaly_test.go}` (unit), `internal/ingest/ingest_integration_test.go`, `internal/ingest/fakesource_test.go` (an in-memory `integration.Adapter` for tests)

**Interfaces:**
- Consumes: Task 1 (`integration.*`, `job.*Payload`, `job.New*Task`), Task 3 (`normalize.Chunk`, `normalize.Window` — the shared range-chunking helper, S7), Task 5 (`store.SystemScope`, `ProviderSeriesRepository`, `AdminIngestionRepository`, `model.ProviderHourlyValue`), F1 repositories (`Analyzer`, `Reading`, `Cursor`, `Anomaly`, `Ops`, `AdminJournal`).
- Produces (`var _ job.Ingestion = (*ingest.Service)(nil)`):

```go
package ingest

// CredentialOpener is satisfied structurally by *credentials.Service (Task 14).
type CredentialOpener interface {
	Open(ctx context.Context, s store.Scope, credentialID uuid.UUID) (integration.Credentials, error)
}
// SourceResolver is satisfied by *integration.Registry.
type SourceResolver interface {
	Source(p integration.Provider) (integration.Adapter, error)
}
// Enqueuer is satisfied by *job.Client.
type Enqueuer interface {
	Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}
// PostPersistHook runs after a page is persisted and before the cursor advances.
// Task 11's *generation.Accumulator implements it for pm5340.
type PostPersistHook interface {
	AfterPersist(ctx context.Context, s store.Scope, a model.Analyzer, kind model.ReadingKind, from, to time.Time) error
}
// ConsumptionRefreshEnqueuer is nil in F2 (R17).
type ConsumptionRefreshEnqueuer interface {
	EnqueueConsumptionRefresh(ctx context.Context, p job.ConsumptionRefreshPayload) error
}

type Deps struct {
	Analyzers      store.AnalyzerRepository
	Readings       store.ReadingRepository
	Cursors        store.CursorRepository
	Anomalies      store.AnomalyRepository
	Ops            store.OpsRepository
	ProviderSeries store.ProviderSeriesRepository
	AdminIngestion store.AdminIngestionRepository
	AdminJournal   store.AdminJournalRepository
	Credentials    CredentialOpener
	Sources        SourceResolver
	Enqueuer       Enqueuer
	Hooks          map[model.IntegrationProvider][]PostPersistHook
	ConsumptionRefresh ConsumptionRefreshEnqueuer
	Clock          clock.Clock
	Log            *slog.Logger
}

type Options struct {
	FutureTolerance time.Duration // R12, default 15m
	SanityMultiple  decimal.Decimal // R13, default 10
	InitialLookback time.Duration // R18, default 720h
	MaxRetry        int           // passed to job.TaskOptions
	MaxPagesPerRun  int           // loop guard, default 1000
}

func New(d Deps, o Options) (*Service, error) // nil required dep → error naming the field
func (s *Service) Dispatch(ctx context.Context) error
func (s *Service) SyncAnalyzers(ctx context.Context, p job.SyncAnalyzersPayload) error
func (s *Service) FetchReadings(ctx context.Context, p job.FetchReadingsPayload) error

// Pure functions (unit-tested):
type RejectReason string
const (
	RejectUnparseableTs RejectReason = "unparseable_timestamp" // zero Ts after normalisation
	RejectFuture        RejectReason = "future_timestamp"
	RejectAllNull       RejectReason = "all_registers_null"
	RejectNegative      RejectReason = "negative_value"
	RejectSanityJump    RejectReason = "sanity_jump"
	RejectConflictingDuplicate RejectReason = "conflicting_duplicate"
)
type Rejection struct { Ts time.Time; Kind model.ReadingKind; Reason RejectReason; Register string }

func Dedupe(rows []model.MeterReading) (kept []model.MeterReading, rejected []Rejection)            // rule D1
func Validate(prev *model.MeterReading, history []model.MeterReading, rows []model.MeterReading, now time.Time, o Options) (valid []model.MeterReading, rejected []Rejection)
type NegativeDelta struct { Register string; PrevTs, CurTs time.Time }
func DetectNegativeDeltas(prev *model.MeterReading, rows []model.MeterReading, resets []model.MeterReading) []NegativeDelta
```

**Rules the implementation must follow:**

- **D1 dedupe:** rows sharing `(AnalyzerID, Ts, Kind)` with identical registers keep one copy. Rows sharing the key with **different** register values are all rejected as `conflicting_duplicate`: the batch must never ask `BulkInsert` to arbitrate, and it would refuse with `ErrConflict` anyway.
- **Validation (06 §9, R11–R14):**
  - zero `Ts` → reject;
  - `Ts > now + FutureTolerance` → reject;
  - every register nil (`MaxDemandKw` and `IntervalGenerationKwh` count as registers) → reject;
  - any register < 0 → reject;
  - sanity jump per R13 against `prev` (the last accepted reading) and `history` (7 days before the window);
  - the commissioning-date rule is **not** applied (R11).
- **Negative delta (R14):** for each cumulative register (the 12 import/export registers, not `MaxDemandKw` or `IntervalGenerationKwh`), compare consecutive readings of the same kind in ts order, starting from `prev`. `cur < prev` → a `NegativeDelta`, unless a `reset`-kind reading exists with `prev.Ts < reset.Ts <= cur.Ts`. The reading is **stored**. `Service` creates `consumption_anomalies{Reason: "negative_delta", PeriodStart: PrevTs, PeriodEnd: CurTs, Detail: {"register": …, "kind": …}}`, skipping creation when `Anomalies.List(AnomalyFilter{AnalyzerIDs: []uuid.UUID{id}, Reason: ptr("negative_delta"), Range: &TimeRange{From: PrevTs, To: CurTs.Add(time.Nanosecond)}})` already holds one with the same period and register. Resolved ones count too, so a re-fetch never resurrects a resolved anomaly.
- **FetchReadings loop:**
  1. `sc := store.SystemScope(p.CompanyID)`; `analyzer := Analyzers.Get`; an inactive or soft-deleted analyzer → run `success`, processed 0, skipped 1, message `info` "analyzer inactive".
  2. `creds := Credentials.Open(sc, p.CredentialID)`; `src := Sources.Source(creds.Provider)`; `analyzer.Provider` must match, else `ErrMalformedPayload` (misconfiguration, no retry).
  3. `StartRun(job_type=TypeIntegrationFetchReadings, scope={"analyzer_id","kind","window"})`.
  4. Window: `p.Window` if set; else `From = cursor.LastTs` or `now − InitialLookback` when the cursor is `ErrNotFound`; `To = now`. Then split with `normalize.Chunk(From, To, src.MaxWindow(kind))` (S7 — the one range-chunking implementation, Task 3; Task 12's EPİAŞ pagination and Task 15's backfill windows use the same function).
  5. For each chunk, page loop: `res := src.FetchReadings`. On error: `Cursors.RecordFailure(sc, id, kind, redacted(err), now)`, `FinishRun` (`partial` if any page persisted, else `failed`) with `errText = redacted(err)`, message `error`, and **return err** (the job layer classifies retry).
  6. Per page: `Dedupe` → load `prev` (`Readings.Latest(sc, id, TimeRange{chunkFrom − 35 days, firstTs}, kind)`) and `history` (`Readings.Range` 7 days) once per chunk → `Validate` → `Readings.BulkInsert(valid)` → hooks for `analyzer.Provider` → `DetectNegativeDeltas` (resets from the page plus `Readings.Range(sc, id, [minTs, maxTs], reset)`) → anomalies → `ProviderSeries.UpsertHourly(convert(res.HourlyValues))` → `if res.ResolvedMultiplier != nil && !equal(analyzer.MeterMultiplier)`: `Analyzers.Update` plus `warning` message "meter multiplier changed" (R27) → `Cursors.RecordSuccess(sc, id, kind, maxPersistedTs, now)` **only if** ≥ 1 row persisted (R19) → `Analyzers.TouchLastReading(maxPersistedTs)`.
  7. `NextCursor`: nil → next chunk; else it must be after `req.From` (R4), else `ErrMalformedPayload`.
  8. `FinishRun(success|partial|failed per R29, processed = inserted+updated, skipped = len(rejected), failed = failed pages, detail = {"rejections_by_reason", "warnings_by_code", "affected_from", "affected_to", "anomalies_created"})`. If `skipped > 0`: one `warning` message, kind `job`, category `analyzer-refresh`, message `"<n> readings rejected"`, metadata = counts by reason. Warning codes from adapters are counted, and each **distinct** code yields one `warning` message (e.g. `multiplier_fallback`).
- **SyncAnalyzers:**
  - Open creds; `src.Verify`; on error: run `failed`, message `error`, return `ClassifyForRetry`-able err.
  - `DiscoverMeteringPoints`; for each point: `GetByInstallation(sc, provider, subtype, no)` → `ErrNotFound` ⇒ `Create` (R27 defaults) else `Update` descriptive fields — every field of `MeteringPoint` **except `ContractedPowerKw`** (R30: `model.Analyzer` has no matching column; the value stays visible only in the adapter's own `Raw`/fixture data, never silently dropped from the point struct itself); per-point failure counts `failed` with the reason in detail and **does not abort**.
  - Then `Analyzers.List(sc, AnalyzerFilter{Providers: []…{provider}, IsActive: ptr(true)})` filtered in Go by `ProviderSubtype == creds.Subtype`; enqueue `job.NewFetchReadingsTask` per analyzer × `src.Kinds(creds)`. `asynq.ErrDuplicateTask` counts `skipped`.
  - `FinishRun(processed = enqueued analyzers, skipped, failed)`.
  - PM5340 (Discover returns nil): the enqueue step still runs over existing analyzers.
- **Dispatch:** `AdminJournal.StartPlatformRun(TypeIntegrationSyncDispatch)`; `AdminIngestion.ActiveCredentials`; for each ref whose provider is a meter provider (not `isolar`): enqueue `NewSyncAnalyzersTask`; `FinishPlatformRun(processed, skipped (duplicates), failed)`.
- **Redaction:** every text stored in `job_runs.error`, `ingestion_cursors.last_error` or `operational_messages` is `secret.Redact(err.Error(), creds.Fragments())`. `err.Error()` is already URL-free by Task 2. This is the single helper `redacted(creds, err)` in `report.go`.

- [ ] **Step 1: Write the failing unit tests**

`dedupe_test.go`: `TestDedupeKeepsIdenticalDuplicatesOnce`, `TestDedupeRejectsConflictingDuplicatesEntirely`.

`validate_test.go`, table-driven, one row per R11–R14 reason:
- `TestValidateRejectsFutureBeyondTolerance` (now+14m kept, now+16m rejected);
- `TestValidateRejectsAllNull`;
- `TestValidateRejectsNegativeValue`;
- `TestValidateSanityJump` (median 2.0 over 30 deltas, multiple 10, delta 25 rejected, delta 19 kept; 23 history deltas → check skipped);
- `TestValidateSanityJumpStopsAfterThreeConsecutive` (4th jump accepted and flagged in the returned rejections as `Reason: RejectSanityJump, Register: "sustained"`, which `Service` turns into the `error` message);
- `TestValidateDoesNotApplyCommissioningDate` (a reading from 1990 is kept, R11 pinned).

`anomaly_test.go`:

```go
func TestDetectNegativeDeltas(t *testing.T) {
	prev := reading("2026-09-01T00:00:00Z", map[string]string{"active_import": "1000", "t1_import": "400"})
	rows := []model.MeterReading{
		reading("2026-09-01T01:00:00Z", map[string]string{"active_import": "1010", "t1_import": "390"}), // t1 decreases
		reading("2026-09-01T02:00:00Z", map[string]string{"active_import": "5", "t1_import": "391"}),   // active resets
	}
	got := ingest.DetectNegativeDeltas(&prev, rows, nil)
	require.ElementsMatch(t, []ingest.NegativeDelta{
		{Register: "t1_import", PrevTs: ts("2026-09-01T00:00:00Z"), CurTs: ts("2026-09-01T01:00:00Z")},
		{Register: "active_import", PrevTs: ts("2026-09-01T01:00:00Z"), CurTs: ts("2026-09-01T02:00:00Z")},
	}, got)
	// a reset reading at 01:30 suppresses the active_import delta only
	resets := []model.MeterReading{resetAt("2026-09-01T01:30:00Z")}
	require.Len(t, ingest.DetectNegativeDeltas(&prev, rows, resets), 1)
	// nil registers never produce a delta
}
```

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/ingest/ -race -run 'TestDedupe|TestValidate|TestDetect' -v
```
Expected: FAIL — undefined.

- [ ] **Step 3: Implement the pure functions; run them and watch them pass.**

- [ ] **Step 4: Write the failing integration tests** (`//go:build integration`; `pool := testfixtures.NewIsolatedDB(t)`; `tn := testfixtures.NewTenant(t, ctx, pool, 1)`; a real `postgres.New…Repository(pool)` for each dep; `fakeAdapter` from `fakesource_test.go` serving scripted pages; a `recordingEnqueuer`; `clock.NewFake`; `credentialOpenerFunc` returning fixture credentials). Helper identifiers start with `ingestTest`.

```go
// TestIngestionSameWindowTwiceProducesNoDuplicateRows is the F2 acceptance
// criterion "fetching the same window twice produces no duplicate rows".
func TestIngestionSameWindowTwiceProducesNoDuplicateRows(t *testing.T) {
	// 96 load-profile readings in one page; FetchReadings twice with the same Window.
	// require: count(*) from meter_readings where analyzer_id=… == 96 after both runs;
	// run 1 job_runs.processed == 96 (inserted), run 2 processed == 96 (updated);
	// anomalies count unchanged between runs.
}

// TestIngestionInterruptedFetchResumesFromCursor is the F2 acceptance
// criterion "interrupting a fetch and re-running resumes from the cursor and
// loses nothing".
func TestIngestionInterruptedFetchResumesFromCursor(t *testing.T) {
	// fake adapter: page 1 (00:00–05:45) OK with NextCursor=05:45; page 2 returns
	// &integration.Error{Kind: ErrUpstreamUnavailable}.
	// Run 1: returns error; cursor.LastTs == 05:45; job_runs status "partial";
	//        cursor.ConsecutiveFailures == 1; last_error has no credential fragment.
	// Swap the adapter's script to succeed for page 2 (06:00–23:45).
	// Run 2 (Window nil, cursor-driven): the FIRST request's From == 05:45 exactly;
	//        after it: 96 distinct rows 00:00–23:45, none duplicated; ConsecutiveFailures == 0.
}

// TestIngestionOneAnalyzerFailureDoesNotAbortTheRun is the F2 acceptance
// criterion "one analyzer failing does not abort the run; the job_runs row
// records processed/skipped/failed counts and the failure reason".
func TestIngestionOneAnalyzerFailureDoesNotAbortTheRun(t *testing.T) {
	// Part A — sync run: discovery returns 3 points; Create for point 2 is made
	// to fail (installation_number collides with a soft-deleted analyzer's
	// unique key held by ANOTHER tenant, or a fake AnalyzerRepository wrapper
	// failing on that installation number). require sync job_runs:
	// status "partial", processed == 2, failed == 1, detail names point 2 and its reason;
	// the two good analyzers were enqueued (recordingEnqueuer) — the run continued.
	// Part B — per-analyzer isolation: FetchReadings for analyzers A, B, C where
	// B's adapter script fails every page. A and C: rows stored, runs "success".
	// B: run "failed", failed == 1, error text == the redacted reason,
	// cursor.ConsecutiveFailures == 1; A's and C's cursors advanced.
}

// TestIngestionNegativeIndexDeltaCreatesUnresolvedAnomaly is the F2
// acceptance criterion "a negative index delta creates an unresolved
// consumption_anomalies row".
func TestIngestionNegativeIndexDeltaCreatesUnresolvedAnomaly(t *testing.T) {
	// readings active_import 1000 @00:00, 990 @01:00. require: both rows stored;
	// Anomalies.List(Unresolved: true) has exactly one row, reason "negative_delta",
	// period [00:00, 01:00], ResolvedAt nil. Fetch the same window again → still one.
	// Resolve it, fetch again → still one (resolved) and no new unresolved row.
}
```

Also:
- `TestIngestionRejectionsAreCountedAndSurfaced`: 2 future and 1 negative-value reading → `skipped == 3`, one `warning` message with metadata counts.
- `TestIngestionEmptyWindowDoesNotAdvanceCursor` (R19).
- `TestIngestionFailureMessageContainsNoSecret`: the adapter error's `Detail` includes the password fragment; `job_runs.error`, `ingestion_cursors.last_error` and every operational message for the company contain no fragment.
- `TestIngestionUsesSystemScopeAndCannotTouchAnotherTenant`: a payload with company A and tenant B's analyzer id → `ErrNotFound`, no rows written to either.
- `TestIngestionMultiplierChangeIsAppliedAndReported`.
- `TestDispatchEnqueuesOneSyncPerActiveMeterCredential` (isolar credential skipped; platform run row with `company_id` NULL).
- `TestIngestionHooksRunBeforeCursorAdvance`: a hook that fails leaves the cursor untouched, and the run returns the error.

- [ ] **Step 5: Run and watch them fail; implement `service.go`, `sync.go`, `fetch.go`, `dispatch.go`, `report.go`; run until they pass**

```bash
go test ./internal/ingest/ -tags=integration -race -count=1 -v
```

- [ ] **Step 6: Prove the acceptance tests can fail.** Apply each mutation, confirm FAIL, restore:

| Test | Mutation |
|---|---|
| `…SameWindowTwice…` | skip `Dedupe` and feed the fake a page containing one conflicting duplicate: the batch is refused and the run reports failure instead of 96 (proves D1 is load-bearing); separately, change `RecordSuccess`'s `lastTs` to `now` |
| `…InterruptedFetchResumes…` | advance the cursor to `req.To` before the page loop |
| `…OneAnalyzerFailure…` | `return err` on the first per-point failure in sync |
| `…NegativeIndexDelta…` | skip anomaly creation when a `reset` row exists **anywhere** in the batch (drop the ts-window condition) |
| `…ContainsNoSecret` | store `err.Error()` without `secret.Redact` and make the fake adapter error embed the fragment |

- [ ] **Step 7: Gate and commit**

```bash
make lint && make test && make build
go test ./... -tags=integration -race -count=1
git add internal/ingest
git commit -m "feat(f2): ingestion pipeline with validation, anomalies, resumable cursors and run records" <TRAILERS>
```

---

## Task 11: PM5340 cumulative generation — anchored, deterministic, recomputable

**Files:**
- Create: `internal/ingest/generation/{accumulate.go,hook.go}`
- Test: `internal/ingest/generation/accumulate_test.go` (unit), `internal/ingest/generation/generation_integration_test.go`

**Interfaces:**
- Consumes: Task 5 (`GenerationRepository`, `MeterReading.IntervalGenerationKwh`), F1 `ReadingRepository`, Task 10 `ingest.PostPersistHook`.
- Produces:

```go
package generation

// Accumulate is the pure core: running sum from base, skipping nil intervals.
// rows must be sorted by Ts ascending; it returns one cumulative value per row.
func Accumulate(base decimal.Decimal, rows []model.MeterReading) []decimal.Decimal

type Accumulator struct{ /* Readings store.ReadingRepository; Anchors store.GenerationRepository; Clock clock.Clock */ }
func New(readings store.ReadingRepository, anchors store.GenerationRepository, c clock.Clock) *Accumulator
var _ ingest.PostPersistHook = (*Accumulator)(nil)

// AfterPersist recomputes active_export for every load_profile row with ts >= from,
// up to now, anchored on the last derived row before from (or on the anchor).
func (a *Accumulator) AfterPersist(ctx context.Context, s store.Scope, an model.Analyzer, kind model.ReadingKind, from, to time.Time) error
// Recompute rebuilds the whole series from the persisted anchor — the
// "recomputable deterministically if history is corrected" requirement.
func (a *Accumulator) Recompute(ctx context.Context, s store.Scope, analyzerID uuid.UUID) error
```

**Algorithm (06 §5, R15):**
1. Only `kind == load_profile` and provider `pm5340`; anything else is a no-op.
2. `anchor := Anchors.Anchor`. On `ErrNotFound`: read the first row in `[from − 7d, to]`, `SetAnchor{AnchorTs: first.Ts − 15m, ActiveExport: 0, Source: "initial"}`.
3. Base: `Readings.Latest(s, id, TimeRange{anchor.AnchorTs, from}, load_profile)`. If its `ActiveExport != nil`, base = `(its ActiveExport, its Ts)`; else base = `(anchor.ActiveExport, anchor.AnchorTs)`.
4. Read `Readings.Range(s, id, [baseTs+1ns, now+FutureTolerance), load_profile)` in 7-day slices, ascending (bounded queries only, 04 §14). Compute `Accumulate`, set `ActiveExport` on each row, write back with `Readings.BulkInsert` in batches of ≤ 5000 (idempotent upsert; no duplicate keys, because Range returns unique keys).
5. A nil `IntervalGenerationKwh` adds nothing; that row's `ActiveExport` carries the running value forward (R15).

- [ ] **Step 1: Write the failing unit test**

```go
func TestAccumulateSkipsNilAndIsExact(t *testing.T) {
	rows := rowsWithIntervals("0.25", "", "0.50", "0.125") // "" = nil
	got := generation.Accumulate(decimal.RequireFromString("100"), rows)
	want := []string{"100.25", "100.25", "100.75", "100.875"}
	for i := range want {
		require.True(t, decimal.RequireFromString(want[i]).Equal(got[i]), "row %d: %s", i, got[i])
	}
}
```

- [ ] **Step 2: Write the failing integration tests** (real DB, `NewTenant`; the tenant's analyzer is switched to provider `pm5340` via `Analyzers.Update`; rows written through `ReadingRepository.BulkInsert` exactly as Task 10 writes them, with `ActiveExport` nil, then `AfterPersist`)

```go
// The three tests below are the F2 acceptance criterion "PM5340 interval
// generation accumulates into the cumulative register correctly across a
// gap, an out-of-order batch and a duplicate fetch" (removed-behaviour 22).
// Expected values are written out by hand, never computed by Accumulate.

func TestGenerationAccumulatesAcrossAGap(t *testing.T) {
	// anchor 1000 @ 09:45. Batch 1: 10:00=1.0kWh, 10:15=1.0.  AfterPersist.
	// Gap: 10:30 and 10:45 never arrive. Batch 2: 11:00=0.5, 11:15=0.5. AfterPersist.
	// require active_export: 10:00=1001, 10:15=1002, 11:00=1002.5, 11:15=1003.
	// Recompute(); require identical values.
}

func TestGenerationOutOfOrderBatchRecomputesLaterRows(t *testing.T) {
	// anchor 0 @ 09:45. Batch 1 (later): 11:00=2, 11:15=2 → 2, 4.
	// Batch 2 (earlier, arrives second): 10:00=1, 10:15=1.
	// require: 10:00=1, 10:15=2, 11:00=4, 11:15=6 — later rows were rewritten.
}

func TestGenerationDuplicateFetchIsStable(t *testing.T) {
	// Batch A persisted and accumulated; the same batch fetched again (upsert sets
	// active_export NULL on those rows, then AfterPersist). require values identical
	// to after the first pass, byte-for-byte as numeric text; row count unchanged.
}
```

Also:
- `TestGenerationRecomputeAfterHistoryCorrection`: change one interval value, `Recompute`, and every later cumulative shifts by exactly the correction.
- `TestGenerationNullIntervalStaysNull`: the row keeps `interval_generation_kwh IS NULL`, and its `active_export` carries forward.
- `TestGenerationCreatesInitialAnchorOnce`.
- `TestGenerationIsScoped`: another tenant's scope → `ErrNotFound`, nothing rewritten.

- [ ] **Step 3: Run and watch them fail**

```bash
go test ./internal/ingest/generation/ -race -v
go test ./internal/ingest/generation/ -tags=integration -race -count=1 -v
```
Expected: FAIL — undefined.

- [ ] **Step 4: Implement; run and watch them pass.**

- [ ] **Step 5: Prove the acceptance tests fail.** Apply each, confirm FAIL, restore:
  (a) recompute only rows in `[from, to]` instead of `[from, now)`: `TestGenerationOutOfOrderBatchRecomputesLaterRows` FAILS;
  (b) treat a nil interval as the previous row's interval: `TestGenerationNullIntervalStaysNull` FAILS;
  (c) take the base from the last row **at or before `to`** instead of strictly before `from`: `TestGenerationDuplicateFetchIsStable` FAILS (double counting);
  (d) drop the `ts > baseTs` bound so the base row is re-added: `TestGenerationAccumulatesAcrossAGap` FAILS.
  Record the outputs.

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
go test ./internal/ingest/... -tags=integration -race -count=1
git add internal/ingest/generation
git commit -m "feat(f2): anchored, recomputable PM5340 cumulative generation" <TRAILERS>
```

---

## Task 12: EPİAŞ client and the `epias.sync_prices` job

**Files:**
- Create: `internal/integration/epias/{client.go,ticket.go,parse.go,wire.go}`, `internal/integration/epias/testdata/epias_<case>.json` (all `fake.RequiredCases` + `epias_yekdem.json`, `epias_items_shape.json`)
- Test: `internal/integration/epias/client_test.go`
- Create: `internal/marketdata/{sync.go,completeness.go}` (keep Task 1's `doc.go`)
- Test: `internal/marketdata/completeness_test.go`, `internal/marketdata/sync_integration_test.go`

**Interfaces:**
- Consumes: Tasks 1–4 (including `normalize.Chunk`, S7 — the one range-chunking implementation, shared with Task 10's pipeline and Task 15's backfill), never Task 5 or Task 10; F1 `store.AdminMarketDataRepository` (`UpsertHourlyPrices`, `UpsertYekdem`, which refuse duplicate keys and a zero ts), `store.AdminJournalRepository`.
- Produces:

```go
package epias

type Options struct {
	CASURL    string // default "https://giris.epias.com.tr/cas/v1/tickets"
	BaseURL   string // default "https://seffaflik.epias.com.tr/electricity-service"
	Username  string
	Password  integration.Secret
	TicketTTL time.Duration // default 90m (R24)
	Clock     clock.Clock
	Locker    httpx.Locker // global key "epias:tgt"; nil only in unit tests
}
func New(pool *httpx.Pool, o Options) (*Client, error) // empty Username/Password → error naming the env var, not the value

// HourlyPTF returns one MarketPrice per published hour in [from, to), UTC, TL/MWh.
// Requests are chunked to 30 days via normalize.Chunk (R20 pagination; S7 — the
// shared range-chunking helper, not a second local implementation). Missing
// hours are NOT filled.
func (c *Client) HourlyPTF(ctx context.Context, from, to time.Time) ([]model.MarketPrice, []integration.Warning, error)
// YekdemUnitCost returns one YekdemMonthly per published (year, month) intersecting [from, to).
func (c *Client) YekdemUnitCost(ctx context.Context, from, to time.Time) ([]model.YekdemMonthly, error)
```

```go
package marketdata

type PriceSource interface { // *epias.Client
	HourlyPTF(ctx context.Context, from, to time.Time) ([]model.MarketPrice, []integration.Warning, error)
	YekdemUnitCost(ctx context.Context, from, to time.Time) ([]model.YekdemMonthly, error)
}
type Syncer struct{ /* Source PriceSource; Market store.AdminMarketDataRepository; Journal store.AdminJournalRepository; Clock clock.Clock; Log *slog.Logger */ }
func New(src PriceSource, market store.AdminMarketDataRepository, journal store.AdminJournalRepository, c clock.Clock, log *slog.Logger) *Syncer
var _ job.PriceSyncer = (*Syncer)(nil)
func (s *Syncer) SyncPrices(ctx context.Context, p job.SyncPricesPayload) error

// MissingHours returns, per Istanbul-local day in [from, to), the expected hour
// starts (23/24/25 via the tz database) absent from prices.
func MissingHours(prices []model.MarketPrice, from, to time.Time) map[string][]time.Time // key "2006-01-02"
```

**Behaviour (06 §7, 02 §7.3, R24, removed-behaviours 10 and 24):**
- **CAS:** `POST CASURL`, form `username`, `password` (both `httpx.Param{Secret: true}` for the password), expect HTTP 201. The ticket is the `TGT-…` match in the `Location` header or the body (legacy `epiasService.ts:124-139`). A 401, or a 201 with no TGT, is `ErrAuth`. The ticket is cached in-process for `TicketTTL`; obtaining one holds `Locker("epias:tgt")` and re-checks the cache after acquiring (double-checked). A data call returning 401 invalidates the cache and retries **once** with a new ticket.
- **Data:** `POST BaseURL + /v1/markets/dam/data/mcp`, JSON `{"startDate": "YYYY-MM-DDT00:00:00+03:00", "endDate": …}`, header `TGT: <ticket>` (Secret). Parse `items[] {date, hour?, price}`, falling back to `body.dayAheadMCPList[] {date, price}`, with `json.Number` for price. `date` via `normalize.ISO8601`. Duplicate `date` within one response → `ErrMalformedPayload`. YEKDEM: `/v1/renewables/data/unit-cost`, `items[]`/`body.renewableSMUnitCostList[] {period|date, unitCost}` → `(year, month)` in Istanbul.
- **429:** `ClientConfig.DefaultRateLimitWait = 65s` (legacy), `Every: 3s`, `Burst: 1`, `RequestTimeout: 20s` (CAS 15s).
- **Sync job:**
  - Window = `p.Window` or `[Istanbul today − 7 days 00:00, Istanbul tomorrow + 1 day 00:00)`; YEKDEM window = the first day of (month − 3) → the first day of next month.
  - A platform run through `AdminJournal.StartPlatformRun(job_type = "epias.sync_prices")`.
  - `UpsertHourlyPrices` per `normalize.Chunk(window, 30*24h)` chunk (S7; processed += rows); `UpsertYekdem`.
  - `MissingHours` over the window, **excluding hours after the last published day** (day-ahead for tomorrow may legitimately be unpublished before 14:00). Missing hours inside published days → one `AppendPlatformMessage{Kind: "job", Category: "market-prices", Status: "warning", Message: "<n> PTF hours missing", Metadata: {"days": {...}}}`, and the run is `partial`. **No value is fabricated; billing flags the invoice** (F4).
  - Error → `FinishPlatformRun(failed, errText = secret.Redact(err.Error(), []string{password, ticket}))`, and return the error.
  - **Billing never calls this client:** the guard is Task 17's `TestBillingPathDoesNotImportEPIAS`, which exists vacuously until F4 creates `internal/domain/billing`/`internal/service/billing`. It walks `./internal/...` packages whose path contains `billing` and passes when none exist. It is proven by a temporary `internal/service/billing/probe.go` importing `epias`.

- [ ] **Step 1: Write the failing tests**

- `client_test.go`: `TestEPIASFixtureMatrix`, per the adapter template, calling `HourlyPTF`: success = 24 prices; empty; partial = one row with a non-numeric price skipped with a warning; auth_failure = CAS 401; malformed = HTML body; rate_limited = 429 then 200 with a recorded sleep of 65s; pagination = a 45-day range gives two POSTs whose bodies carry contiguous windows.
- `TestIdempotentEPIASRefetchYieldsIdenticalPrices`.
- `TestEPIASTicketIsCachedAndRefreshedOnce`: two data calls cause one CAS call; a data 401 causes exactly one more CAS call, then success.
- `TestEPIASErrorsCarryNoPasswordOrTicket`.
- `TestEPIASPricesAreExactDecimals`: `"2345.6789"` is preserved.
- `completeness_test.go`: `TestMissingHoursUsesIstanbulDays` (a UTC-bucketed day would be wrong: `2026-09-14` local starts at `2026-09-13T21:00Z`); `TestMissingHoursPre2016DSTDays` (R6 — corrected against Go's tzdata, `go run` verified: `2015-03-29` expects 23 hours [spring-forward]; `2015-10-25` expects 24 hours [an ordinary day — Turkey postponed its scheduled 2015 fall-back]; `2015-11-08` expects 25 hours [the actual, delayed fall-back]. The original draft claimed `2015-10-25` was the 25-hour day; Turkey's government moved that year's clock change to 2015-11-08 by decree, so `2015-10-25` is 24 hours like any other day, and asserting 25 there would fail against a correct implementation that actually consults the tz database. All three dates are asserted so the test also documents the anomaly rather than silently landing on a lucky pair).
- `sync_integration_test.go` (`//go:build integration`, `NewIsolatedDB`, `admin.NewMarketDataRepository`, `admin.NewJournalRepository`, a fake `PriceSource`):
  - `TestSyncPricesIsIdempotent`: run twice → `market_prices_hourly` count unchanged; platform run rows have `company_id` NULL.
  - `TestSyncPricesReportsMissingHoursWithoutFilling`: 23 of 24 hours → 23 rows, one `warning` platform message, run `partial`; no row for the missing hour.
  - `TestSyncPricesBackfillWindow`: an explicit 60-day window writes every day.

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/integration/epias/ ./internal/marketdata/ -race -v
go test ./internal/marketdata/ -tags=integration -race -count=1 -v
```

- [ ] **Step 3: Implement; Step 4: watch them pass.**

- [ ] **Step 5: Prove it.** (a) Bucket `MissingHours` days in UTC: `TestMissingHoursUsesIstanbulDays` FAILS. (b) Fill a missing hour with the previous hour's price in `SyncPrices`: `TestSyncPricesReportsMissingHoursWithoutFilling` FAILS. (c) Remove the cache re-check after acquiring the lock and run `TestEPIASTicketIsCachedAndRefreshedOnce` with two concurrent goroutines: it FAILS with 2 CAS calls. Restore.

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
go test ./internal/marketdata/ -tags=integration -race -count=1
git add internal/integration/epias internal/marketdata
git commit -m "feat(f2): EPİAŞ client and idempotent price sync that reports, never fills, missing hours" <TRAILERS>
```

---

## Task 13: iSolarCloud adapter and the blocker-aware production store

**Files:**
- Create: `internal/integration/isolar/{token.go,client.go,auth.go,plants.go,series.go,alarms.go,wire.go}`, `internal/integration/isolar/testdata/isolar_<case>.json` (all `fake.RequiredCases` + `isolar_token.json`, `isolar_refresh.json`, `isolar_devices_page{1,2}.json`, `isolar_device_minute.json`, `isolar_plant_minute.json`, `isolar_faults.json`). `token.go` no longer needs a special early sub-merge (S3): Task 14 now runs in its own wave (E) after the whole of Wave D, including all of Task 13, is merged — see the Execution waves note.
- Test: `internal/integration/isolar/{client_test.go,auth_test.go,series_test.go}`
- Create: `internal/ingest/production/store.go`
- Test: `internal/ingest/production/store_integration_test.go`

**Interfaces:**
- Consumes: Tasks 1–5; F1 `PlantRepository.Devices`, `ProductionRepository.BulkInsert`, `OpsRepository.AppendMessage`.
- Produces:

```go
package isolar

// token.go
type Token struct {
	AccessToken, RefreshToken integration.Secret
	ExpiresAt                 time.Time // now + expires_in (default 7200s)
}
type ProductionSample struct {
	PSID     string
	PSKey    *string // nil ⇒ plant-level sample (BLOCKER)
	DeviceSN *string
	Ts       time.Time // UTC
	ProductionKwh, ActivePowerKw, IrradianceWm2, ModuleTempC, AmbientTempC *decimal.Decimal // Wh→kWh, W→kW already applied
}
type Plant struct { PSID, Name string; InstalledKw *decimal.Decimal }
type Device struct { PSKey, DeviceSN string; DeviceType *int32; DeviceName *string }
type Fault struct { Ref, PSID string; PSKey *string; Code, Message string; OccurredAt time.Time }

func New(pool *httpx.Pool, o Options) *Client // Options{Clock clock.Clock}
// Endpoints come from creds.Endpoints (integration_definitions subtype = region EU|CN|AU).
func (c *Client) AuthorizeURL(creds integration.Credentials, redirectURI string) (string, error) // R22 format; app_id from creds.Extra["app_id"]
func (c *Client) ExchangeCode(ctx context.Context, creds integration.Credentials, code, redirectURI string) (Token, error)
func (c *Client) Refresh(ctx context.Context, creds integration.Credentials) (Token, error)
func (c *Client) Verify(ctx context.Context, creds integration.Credentials) error // queryPowerStationList page 1 size 1
func (c *Client) Plants(ctx context.Context, creds integration.Credentials) ([]Plant, error)              // page/size 50, follows rowCount
func (c *Client) Devices(ctx context.Context, creds integration.Credentials, psID string) ([]Device, error) // page/size 100, pageList
func (c *Client) DeviceMinuteSeries(ctx context.Context, creds integration.Credentials, psKeys []string, from, to time.Time) ([]ProductionSample, error)
func (c *Client) PlantMinuteSeries(ctx context.Context, creds integration.Credentials, psID string, from, to time.Time) ([]ProductionSample, error) // PSKey nil
func (c *Client) Faults(ctx context.Context, creds integration.Credentials, from, to time.Time) ([]Fault, error)

// Point ids (06 §6): 1 yield today Wh, 24 total active power W, 2001 irradiation Wh/m²,
// 2009 ambient °C, 2010 module °C. Series requests use minute_interval "60" (legacy default) unless Options overrides.
```

```go
package production

type Deps struct { Plants store.PlantRepository; Production store.ProductionRepository; Ops store.OpsRepository }
type Result struct { Inserted, Updated, Quarantined, UnknownDevice int }
// Store persists device-attributed samples and QUARANTINES plant-level ones (BLOCKER).
func Store(ctx context.Context, s store.Scope, d Deps, plantID uuid.UUID, samples []isolar.ProductionSample) (Result, error)
```

**Behaviour (06 §6, R21, R22):**
- Every call is POST JSON with `appkey` (= `creds.Extra["app_key"]`) in the body, header `x-access-key: creds.Extra["secret_key"]` (Secret), `Authorization: Bearer creds.Extra["access_token"]` (Secret) on platform calls, and `Content-Type: application/json`.
- The response must be `application/json` (else `ErrMalformedPayload`) and shaped `{result_code, result_msg, result_data}`; `"1"` means success, otherwise R21.
- Minute series `result_data` is keyed by ps_key/ps_id (`isolarTypes.ts:147,221`).
- Units: point 1 Wh → `ProductionKwh` via `normalize.WhToKWh`; point 24 W → `ActivePowerKw` via `WToKW`.
- `production.Store`:
  1. Resolve `Devices(sc, plantID)` into a `map[ProviderKey]id` (fall back to `DeviceSN`).
  2. Sample with `PSKey == nil` → `Quarantined++`.
  3. Unknown key → `UnknownDevice++` (device sync is F9; never auto-create here).
  4. Dedupe `(plant, ts, device)` per Task 10 rule D1; `BulkInsert` the rest.
  5. If `Quarantined > 0`: exactly one `AppendMessage` per BLOCKER item 2. If `UnknownDevice > 0`: one `warning` message category `plant-production`, code `unknown_device`.

- [ ] **Step 1: Write the failing tests**

- `TestISolarFixtureMatrix`, per the template, exercised on `DeviceMinuteSeries` (plus `pagination` on `Devices`). `auth_failure` = `result_code "E00003", result_msg "er_token_login_invalid"` → `ErrAuth`; `rate_limited` = HTTP 429.
- `TestIdempotentISolarRefetchYieldsIdenticalSamples`.
- `TestISolarAuthorizeURLFormat`: exact string for region EU with a fixture app id and a redirect carrying `?state=`; the redirect is query-escaped once.
- `TestISolarExchangeAndRefreshParseTokens`: `expires_in` absent → +7200s from the fake clock.
- `TestISolarUnitsAreNormalised`: 1500 Wh → 1.5 kWh; 2400 W → 2.4 kW.
- `TestISolarHeadersCarrySecretsOnlyInHeaders`: recorded request has `x-access-key` and the bearer header; `err` text on failure contains neither.
- `TestISolarPlantSeriesHasNoDeviceKey`.

`store_integration_test.go`:

```go
// TestPlantLevelProductionIsQuarantinedNotFabricated pins the BLOCKER ruling:
// until the product owner answers, a plant-level sample is never written and
// never attributed to a device.
func TestPlantLevelProductionIsQuarantinedNotFabricated(t *testing.T) {
	// tenant plant with two devices (UpsertDevice). samples: 4 device-attributed
	// (2 per device), 3 plant-level (PSKey nil).
	// require Result{Inserted: 4, Quarantined: 3}; plant_production row count == 4;
	// every stored device_id ∈ the two device ids and each has exactly 2 rows;
	// exactly one operational message category "plant-production" whose metadata
	// has samples == 3 and code "plant_level_production_unstorable".
}
```

Also `TestProductionStoreIsIdempotent`, `TestProductionUnknownDeviceIsSkippedAndReported` and `TestProductionStoreRefusesAnotherTenantsPlant`.

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/integration/isolar/ -race -v
go test ./internal/ingest/production/ -tags=integration -race -count=1 -v
```

- [ ] **Step 3: Implement; Step 4: watch them pass.**

- [ ] **Step 5: Prove the blocker guard fails.** Attribute plant-level samples to the plant's first device; `TestPlantLevelProductionIsQuarantinedNotFabricated` FAILS on the row count and on the per-device count. Restore.

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
go test ./internal/ingest/production/ -tags=integration -race -count=1
git add internal/integration/isolar internal/ingest/production
git commit -m "feat(f2): iSolarCloud adapter and production store that quarantines plant-level samples" <TRAILERS>
```

---

## Task 14: Write-only credential service and iSolarCloud OAuth

**Files:**
- Create: `internal/credentials/{service.go,view.go,open.go,settings.go,isolar_oauth.go,state.go}` (keep Task 1's `doc.go`)
- Test: `internal/credentials/{view_test.go,state_test.go,settings_test.go}`, `internal/credentials/credentials_integration_test.go`

**Interfaces:**
- Consumes: Task 1, Task 4 (`lock.Locker` and `lock.Consumer`, imported directly from `internal/platform/lock` — R2/R3; not through `httpx`, which credentials has no other reason to depend on), Task 5 (`SystemScope`), Task 10 (`ingest.Enqueuer` — S6: the original draft's Consumes line omitted this despite `Deps.Enqueuer` being typed `ingest.Enqueuer`, an undeclared dependency the preflight scan flagged), Task 13 (`isolar.Token`, fully merged — Task 14 is now Wave E, strictly after Wave D), F1 `IntegrationRepository` (`Definition`, `ListCredentials`, `UpsertCredential`, `OpenSecret`, `DeleteCredential`, `RecordVerification`), `AnalyzerRepository`. Task 13's live client calls are still reached **through the structural interface below**, with no import of `isolar` beyond its `Token` type.
- Produces (`Service` satisfies `ingest.CredentialOpener` structurally):

```go
package credentials

// View is the ONLY read model of a credential. It has no field that can hold
// secret material (TestViewCarriesNoSecretMaterial).
type View struct {
	ID, DefinitionID uuid.UUID
	Provider         model.IntegrationProvider
	Subtype          string
	Username         *string
	HasSecret        bool
	ExtraKeys        []string // names only, e.g. ["app_id","app_key","secret_key"]
	Settings         json.RawMessage
	PM5340URL        *string
	IsolarRegion     *string
	IsActive         bool
	TokenExpiresAt, LastVerifiedAt *time.Time
	UpdatedAt        time.Time
}

type Input struct {
	Provider     model.IntegrationProvider
	Subtype      string
	Username     *string
	Secret       *integration.Secret           // nil on Update = unchanged (05 §15 PATCH)
	Extra        map[string]integration.Secret // merged key by key on Update; a zero Secret deletes the key
	Settings     json.RawMessage               // validated by settings.go
	PM5340URL    *string
	InstallationNumber *string                 // pm5340 only: creates/updates its single analyzer
	IsolarRegion *string
	IsActive     *bool
}

type Verifier interface { // *integration.Registry adapters, plus an isolar shim built in Task 16
	Verify(ctx context.Context, creds integration.Credentials) error
}
type VerifierResolver interface {
	Verifier(p integration.Provider) (Verifier, error)
}
type ISolarTokens interface { // *isolar.Client
	AuthorizeURL(creds integration.Credentials, redirectURI string) (string, error)
	ExchangeCode(ctx context.Context, creds integration.Credentials, code, redirectURI string) (isolar.Token, error)
	Refresh(ctx context.Context, creds integration.Credentials) (isolar.Token, error)
}
```

`credentials` imports `internal/integration/isolar` for `isolar.Token` only (allowed: `credentials` is not an adapter). **Wave placement (S3):** Task 14 is Wave E, dispatched only once Wave D — all of Task 13, not just a `token.go` first commit — is fully merged. The original draft kept Task 14 inside Wave D alongside Task 13 and patched the resulting same-wave dependency with a controller sub-merge rule ("merge Task 13's first commit before dispatching Task 14"); giving Task 14 its own later wave removes the special case rather than working around it — the ordinary "a wave starts only after every task it depends on is merged" rule now suffices.

```go
type Deps struct {
	Integrations store.IntegrationRepository
	Analyzers    store.AnalyzerRepository
	Verifiers    VerifierResolver
	ISolar       ISolarTokens
	Enqueuer     ingest.Enqueuer // *job.Client
	Locker       lock.Locker     // R2: the canonical type (Task 4), not httpx.Locker — credentials
	                              // has no other reason to import httpx. Used only for the
	                              // ISolarAccessToken refresh critical section below, which IS a
	                              // genuine mutual-exclusion lock (bounded, released on every path).
	Nonces       lock.Consumer   // R3: the OAuth state nonce's single-use consume — NEVER Locker.
	                              // Acquiring-and-never-releasing a Locker lease (the original draft)
	                              // makes a replayed callback block for the lease TTL, or forever
	                              // under context.Background(); Consume is non-blocking by
	                              // construction, so a replay fails immediately.
	Clock        clock.Clock
	StateKey     []byte // HMAC(JWTSigningKey, "isolar-oauth-state/v1"), derived in Task 16
	RedirectURI  string // EKOKOD_ISOLAR_REDIRECT_URL or PublicURL+"/integrations/isolar/callback"
	MaxRetry     int
}
func New(d Deps) (*Service, error)

func (s *Service) Configure(ctx context.Context, sc store.Scope, in Input) (View, error)
func (s *Service) Update(ctx context.Context, sc store.Scope, id uuid.UUID, in Input) (View, error)
func (s *Service) List(ctx context.Context, sc store.Scope) ([]View, error)
func (s *Service) Delete(ctx context.Context, sc store.Scope, id uuid.UUID) error
func (s *Service) Verify(ctx context.Context, sc store.Scope, id uuid.UUID) (View, error)
func (s *Service) Discover(ctx context.Context, sc store.Scope, id uuid.UUID) (taskID string, err error) // enqueues SyncAnalyzers
func (s *Service) Backfill(ctx context.Context, sc store.Scope, id uuid.UUID, in BackfillInput) (taskID string, err error)
type BackfillInput struct { AnalyzerIDs []uuid.UUID; Kinds []model.ReadingKind; From, To time.Time }
func (s *Service) Open(ctx context.Context, sc store.Scope, id uuid.UUID) (integration.Credentials, error)

// OAuth (06 §6 authorisation flow, R22)
func (s *Service) ISolarAuthorizeURL(ctx context.Context, sc store.Scope, id uuid.UUID) (string, error)
// ISolarCallback takes NO Scope: the signed state is what identifies the company, as a
// login produces a Scope. A bad, expired or replayed-after-use state → ErrInvalidState.
func (s *Service) ISolarCallback(ctx context.Context, code, state string) error
// ISolarAccessToken refreshes when expiring within 5 min, under lock "isolar:token:<company>",
// re-reading the credential after acquiring the lock (double-checked) — 06 §6 step 4.
func (s *Service) ISolarAccessToken(ctx context.Context, sc store.Scope, id uuid.UUID) (integration.Secret, error)

var ErrInvalidState = errors.New("credentials: invalid oauth state")
var ErrInvalidSettings = errors.New("credentials: invalid settings")
```

**Rules:**
- `Extra` is sealed as a JSON object `{"key": "plaintext"}` through `UpsertCredential(…, secret, extra)`. Plaintext buffers are zeroed after sealing (best effort; documented).
- `settings.go` allows only `use_billing_indexes` (bool, gridbox). Any other key → `ErrInvalidSettings`, and any key matching `(?i)pass|secret|token|key` gets a dedicated message ("secrets are not settings"). This stops a secret reaching the one JSON column the API returns.
- `PM5340URL` must parse with scheme `http`/`https` and a host (R28).
- `Open` builds `integration.Credentials`: endpoints from `Definition`, secrets from `OpenSecret`, and it verifies the credential belongs to `sc` (`ErrNotFound` otherwise). For isolar it also fills `Extra["access_token"]` via `ISolarAccessToken`.
- `Verify` → `RecordVerification` on success; on `ErrAuth` it returns an error whose text is redacted.
- State token: `base64url(companyID || credentialID || expiry unix || nonce) + "." + base64url(HMAC-SHA256)`, compared with `hmac.Equal`, 10-minute expiry.
- **Single use (R3).** The original draft made the nonce single-use by `Locker.Acquire(ttl=10m)` and never releasing it — but `Locker.Acquire` **waits by polling until ctx is done** (Task 4), so a replayed state would block for up to 10 minutes, or forever under `context.Background()`, instead of failing. Single-use is not a locking problem at all: it is an atomic, non-blocking CONSUME. `ISolarCallback` calls `s.deps.Nonces.Consume(ctx, "isolar:state:<nonce>", ttl=10m)` (`lock.Consumer`, R2/Task 4). `found==false` — the nonce was already consumed (or `Consume` itself failed) — maps immediately to `ErrInvalidState`, fail-closed on any error too. `ISolarCallback` wraps this call in its own short internal timeout (2s, independent of the caller's ctx) precisely so that even a broken `Consumer` backend cannot turn a replay into a hang — `TestStateIsSingleUse` runs under a short deadline for the same reason and asserts the call returns in well under that deadline, not merely before it times out.

- [ ] **Step 1: Write the failing tests**

```go
// TestViewCarriesNoSecretMaterial pins 05 §15 "Never returns secrets" at the
// type level: no future field can quietly carry one. R1: an earlier draft's
// `(?i)…secret$…` name regex rejected the plan's OWN HasSecret bool field —
// a bool cannot itself carry secret material, so the check below inspects
// field TYPES and VALUES, never a name pattern, and names the one allowed
// bool explicitly instead of trying to describe it with a regex.
func TestViewCarriesNoSecretMaterial(t *testing.T) {
	typ := reflect.TypeOf(credentials.View{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Type.Kind() == reflect.Bool {
			require.Equal(t, "HasSecret", f.Name,
				"a bool cannot carry secret material, but name every one here so a reviewer sees it")
			continue
		}
		require.NotEqual(t, reflect.TypeOf(integration.Secret{}), f.Type, "field %s", f.Name)
		require.NotEqual(t, reflect.TypeOf([]byte{}), f.Type, "no raw bytes: %s", f.Name)
	}
}
```

- `view_test.go`: also `TestHasSecretIsTheOnlySecretSignal`.
- `state_test.go`: `TestStateRoundTrip`, `TestStateRejectsTamperExpiryAndWrongKey`; `TestStateIsSingleUse` (R3 — first `ISolarCallback` for a state succeeds; calling it again with the SAME state, under a `context.WithTimeout(ctx, 2*time.Second)`, returns `ErrInvalidState` and `require.Less(t, elapsed, 500*time.Millisecond)`: a hang would fail the deadline, not silently pass by timing out).
- `settings_test.go`: `TestSettingsRejectSecretsAndUnknownKeys`.
- `credentials_integration_test.go` (`NewIsolatedDB`, `NewTenant`, `crypto.NewCipher(32 bytes)`, fake verifier, fake `ISolarTokens`, `lock.NewMemory()` for both `Deps.Locker` and `Deps.Nonces` — `*lock.Memory` implements both `lock.Locker` and `lock.Consumer`, R2/R3, `recordingEnqueuer`):
  - `TestConfiguredSecretNeverAppearsInListOrJSON`: configure with password `FIXTURE-PW-…`; `json.Marshal(List())` and `fmt.Sprintf("%+v", view)` contain no fragment; the DB `secret_enc` is not the plaintext.
  - `TestUpdateWithoutSecretKeepsIt`: `Open` still returns the original.
  - `TestUpdateMergesExtraKeys`.
  - `TestOpenIsScoped`: tenant B → `ErrNotFound`.
  - `TestPM5340ConfigureCreatesItsAnalyzer`.
  - `TestDiscoverEnqueuesSyncAnalyzers`.
  - `TestISolarCallbackStoresTokensForTheStatesCompany`.
  - `TestISolarAccessTokenRefreshesOnceUnderConcurrency`: 10 goroutines, token expiring in 1 min → exactly 1 `Refresh` call; all get the new token.
  - `TestISolarAccessTokenDoesNotRefreshFreshToken`.

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/credentials/ -race -v
go test ./internal/credentials/ -tags=integration -race -count=1 -v
```

- [ ] **Step 3: Implement; Step 4: watch them pass.**

- [ ] **Step 5: Prove it.** (a) Add `Secret []byte` to `View`: `TestViewCarriesNoSecretMaterial` FAILS. (b) Remove the post-lock re-read in `ISolarAccessToken`: `TestISolarAccessTokenRefreshesOnceUnderConcurrency` FAILS with >1 refresh (run `-count=5`). (c) Compare the HMAC with `==` on strings **and** skip the expiry check: `TestStateRejectsTamperExpiryAndWrongKey` FAILS on the expiry case. (d) Revert `ISolarCallback`'s single-use check to `Locker.Acquire(ttl=10m)` with no release (the original draft's approach): `TestStateIsSingleUse` FAILS by exceeding its 500ms bound (it blocks instead of returning `ErrInvalidState`) — this is the R3 regression this plan exists to prevent. Restore all four.

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
go test ./internal/credentials/ -tags=integration -race -count=1
git add internal/credentials
git commit -m "feat(f2): write-only credential service and iSolarCloud OAuth with locked refresh" <TRAILERS>
```

---

## Task 15: Backfill — explicit range, chunked, resumable

**Files:**
- Create: `internal/ingest/backfill/backfill.go`
- Test: `internal/ingest/backfill/backfill_test.go`, `internal/ingest/backfill/backfill_integration_test.go`

**Interfaces:**
- Consumes: Task 1 (`job.BackfillPayload`, `job.NewFetchReadingsTask`, `integration.Planner`), Task 3 (`normalize.Chunk` — S7), Task 5 (`SystemScope`), Task 10 (`ingest.Enqueuer`, `ingest.SourceResolver`, `ingest.CredentialOpener`), F1 `AnalyzerRepository`, `OpsRepository`.
- Produces:

```go
package backfill

// Windows splits [from, to) into consecutive half-open windows of at most
// max, aligned to Istanbul-local midnight so a window never splits a local
// day. It is a thin wrapper over normalize.Chunk (S7 — the ONE range-chunking
// implementation in F2, shared with Task 10's pipeline and Task 12's EPİAŞ
// pagination): Windows exists at all only because callers here want
// job.Window, not normalize.Window, and the two are structurally identical
// but distinct types (normalize has zero F2-task dependencies and must stay
// that way, so it cannot import internal/job to return job.Window directly).
func Windows(from, to time.Time, max time.Duration) []job.Window {
	chunks := normalize.Chunk(from, to, max)
	windows := make([]job.Window, len(chunks))
	for i, c := range chunks {
		windows[i] = job.Window{From: c.From, To: c.To}
	}
	return windows
}

type Deps struct {
	Analyzers   store.AnalyzerRepository
	Ops         store.OpsRepository
	Credentials ingest.CredentialOpener
	Sources     ingest.SourceResolver
	Enqueuer    ingest.Enqueuer
	Clock       clock.Clock
	MaxRetry    int
}
func New(d Deps) *Backfiller
var _ job.Backfiller = (*Backfiller)(nil)
func (b *Backfiller) Backfill(ctx context.Context, p job.BackfillPayload) error
```

**Behaviour (06 §9 Backfill):**
- Validate: `From < To`, `To <= now`, span ≤ 5 years (**spec silent — ruling:** 5 years bounds a mistaken request); otherwise `ErrMalformedPayload`.
- Analyzers: `p.AnalyzerIDs` if non-empty (each must be visible: `Analyzers.List(sc, AnalyzerFilter{IDs: ids})`, and a missing id → run `partial` naming it); **if empty, list the credential's active analyzers explicitly** (never pass an empty `IDs` filter meaning "all").
- Kinds: `p.Kinds` ∩ `src.Kinds(creds)`, else all of `src.Kinds(creds)`.
- For each analyzer × kind × `Windows(From, To, src.MaxWindow(kind))`: enqueue `NewFetchReadingsTask` with `Window` set. Its deterministic `TaskID` makes a re-run skip windows already queued or retained, and `asynq.ErrTaskIDConflict` counts `skipped`. This is the resumability: every window is an independent idempotent task, and a fetch with a `Window` never moves the cursor backwards (F1 `RecordSuccess` stores `greatest`).
- Run record: `processed = windows enqueued`, `skipped = conflicts`. If `From < now − 30 days`, one `info` message: "Backfilled range before <now−30d> is not in the consumption aggregates until consumption.refresh runs for it (F3)" (R17; the F1 `start_offset` fact).

- [ ] **Step 1: Write the failing tests**
  - `backfill_test.go`: `TestWindowsWrapsNormalizeChunk` (the gap/overlap/DST-alignment property test itself lives in Task 3's `TestChunkCoversRangeWithoutGapOrOverlap`/`TestChunkAlignsToIstanbulMidnight`, S7 — this test only proves the `job.Window` conversion is faithful: same boundaries, same count, as `normalize.Chunk` for a handful of representative ranges).
  - `backfill_integration_test.go`:
    - `TestBackfillEnqueuesEveryWindowOnce`: 45 days × 2 analyzers × 1 kind at 30-day max gives 4 tasks with distinct deterministic TaskIDs.
    - `TestBackfillRerunIsResumable`: the enqueuer returns `ErrTaskIDConflict` for the first 2 → run `processed 2, skipped 2`.
    - `TestBackfillEmptyAnalyzerListMeansActiveAnalyzersNotAll`: an inactive analyzer is not enqueued.
    - `TestBackfillRejectsInvertedAndFutureRanges`.
    - `TestBackfillWarnsAboutAggregateHorizon`.

- [ ] **Step 2: Run and watch them fail; Step 3: implement; Step 4: watch them pass**

```bash
go test ./internal/ingest/backfill/ -race -v
go test ./internal/ingest/backfill/ -tags=integration -race -count=1 -v
```

- [ ] **Step 5: Prove it.** (a) Drop the `To`/`From` field copy in `Windows` (return zero-value `job.Window`s): `TestWindowsWrapsNormalizeChunk` FAILS — the Istanbul-alignment property itself is proven in Task 3 (S7), so it is not re-proven here. (b) Pass `AnalyzerFilter{IDs: p.AnalyzerIDs}` straight through when empty, so every analyzer including inactive ones is listed: `TestBackfillEmptyAnalyzerListMeansActiveAnalyzersNotAll` FAILS. Restore.

- [ ] **Step 6: Gate and commit**

```bash
make lint && make test && make build
go test ./internal/ingest/... -tags=integration -race -count=1
git add internal/ingest/backfill
git commit -m "feat(f2): chunked, resumable backfill over deterministic window tasks" <TRAILERS>
```

---

## Task 16: Worker and scheduler wiring, configuration knobs, and the scheduler defects F1 handed to F2

**Files:**
- Create: `internal/worker/wiring.go`, `internal/worker/wiring_test.go`, `internal/worker/wiring_integration_test.go`
- Modify: `internal/cli/worker.go`, `internal/scheduler/scheduler.go`, `internal/scheduler/elector.go`, `internal/scheduler/scheduler_integration_test.go`, `internal/scheduler/elector_integration_test.go`
- Modify: `internal/platform/config/config.go`, `load.go`, `config_test.go`; `docs/rewrite/appendix/env-reference.md`; `.env.example`

**Interfaces:**
- Consumes: everything from Tasks 1–15.
- Produces:

```go
package worker

type Built struct {
	Handlers *job.Handlers
	Close    func() // closes the job client and the lock's redis client
}
// Build constructs every F2 dependency from configuration. It is the single
// wiring point; internal/cli/worker.go calls it and Task 17's acceptance test calls it.
func Build(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) (Built, error)
```

```go
// config additions
type Ingest struct {
	SanityMultiple  decimal.Decimal // EKOKOD_INGEST_SANITY_MULTIPLE, default "10", must be > 1
	FutureTolerance time.Duration   // EKOKOD_INGEST_FUTURE_TOLERANCE, default 15m, > 0
	InitialLookback time.Duration   // EKOKOD_INGEST_INITIAL_LOOKBACK, default 720h, > 0
}
// Config gains `Ingest Ingest`. EKOKOD_JOB_MAX_RETRIES switches from intVal to nonNegativeInt
// (F2 is the consuming phase that load.go's comment defers this to).
```

**`Build` wires:**
- `crypto.NewCipher(cfg.Security.EncryptionKey)`.
- `postgres.New<Name>Repository(pool)` for Reading, Cursor, Anomaly, Analyzer, Plant, Production and Ops (names confirmed in Pre-flight P2), plus Task 5's `postgres.NewGenerationRepository(pool)`, `postgres.NewProviderSeriesRepository(pool)` and `NewIntegrationRepository(pool, cipher)`; `admin.NewIngestionRepository`, `NewMarketDataRepository`, `NewJournalRepository`.
- `redis.New(ctx, cfg.Redis, log)` → `lock.NewRedis(client)`. R2: this value is passed **directly** wherever a `Locker` (or `Consumer`) is wanted — `httpx.Locker` is a type alias for `lock.Locker` (Task 2), so `*lock.Redis` already satisfies it; there is no `worker.httpxLocker` adapter anywhere in F2.
- `httpx.NewPool(PoolOptions{PinnedCerts: cfg.External.PinnedCerts, Locker: redisLock})`.
- `integration.NewRegistry(osos.New(pool…), gridbox.New(…), aril.New(…), pm5340.New(…))`.
- `isolar.New`, `epias.New(Options{Username: cfg.External.EPIASUsername, Password: integration.NewSecret([]byte(cfg.External.EPIASPassword)), Locker: redisLock})`.
- `job.NewClient(cfg.Redis)`.
- `credentials.New(Deps{…, Locker: redisLock, Nonces: redisLock /* *lock.Redis implements lock.Consumer too, R3 */, Enqueuer: jobClient, StateKey: hmacSHA256(cfg.Security.JWTSigningKey, "isolar-oauth-state/v1"), RedirectURI: cfg.External.ISolarRedirect or strings.TrimSuffix(cfg.HTTP.PublicURL, "/") + "/integrations/isolar/callback"})`.
- `ingest.New(Deps{…, Hooks: {pm5340: {generation.New(…)}}, ConsumptionRefresh: nil /* R17 */}, Options{from cfg.Ingest, MaxRetry: cfg.Worker.MaxRetries})`.
- `backfill.New`, `marketdata.New`.
- Returns `job.Handlers{Log, Ingestion: ingestSvc, Backfill: backfiller, Prices: syncer}`.

**Scheduler (`scheduler.go`):**
- `entries()` returns the noop heartbeat, `{Cron: cfg.Schedule.Ingestion, Task: job.NewSyncDispatchTask(TaskOptions{MaxRetry: cfg.Worker.MaxRetries})}` and `{Cron: cfg.Schedule.EPIAS, Task: job.NewSyncPricesTask(SyncPricesPayload{}, …)}`. Timezone stays `cfg.Timezone` (Europe/Istanbul, 03 §4.2).
- **Carried-forward defects** (F1 plan "Carried forward"; owner "the phase that registers real cron entries" = F2):
  1. `elector.go` reads `pg_advisory_unlock`'s boolean with `QueryRow(...).Scan` and logs a `warning` when it is `false`.
  2. The lead loop backs off exponentially on consecutive `lead` panics or errors (10 s doubling to 5 min), with a failure counter in the log.
  3. `scheduler.go` builds the asynq scheduler with `asynq.NewSchedulerFromRedisClient` over a caller-owned `goredis.Client` closed on every return path, so a bad cron entry no longer leaks a client per retry.

- [ ] **Step 1: Write the failing tests**
  - `config_test.go`: `TestIngestDefaults`; `TestIngestSanityMultipleMustExceedOne`; `TestJobMaxRetriesRejectsNegative`; `TestConfigCheckListsIngestVariables`.
  - `wiring_test.go` (unit): `TestRedirectURIDerivedFromPublicURL`; `TestStateKeyIsDomainSeparated` (≠ the raw JWT key; stable across calls).
  - `wiring_integration_test.go` (`StartPostgres`/`NewIsolatedDB` + `StartRedis`, config from `config.Load` with a map lookup):

```go
// TestWorkerRegistersEveryF2Handler proves the wiring the acceptance suite relies on.
func TestWorkerRegistersEveryF2Handler(t *testing.T) {
	built, err := worker.Build(ctx, cfg, pool, testfixtures.DiscardLogger())
	require.NoError(t, err)
	defer built.Close()
	mux := asynq.NewServeMux()
	job.Register(mux, built.Handlers)
	for _, typ := range []string{job.TypeIntegrationSyncDispatch, job.TypeIntegrationSyncAnalyzers,
		job.TypeIntegrationFetchReadings, job.TypeIntegrationBackfill, job.TypeEPIASSyncPrices} {
		_, pattern := mux.Handler(asynq.NewTask(typ, nil))
		require.Equal(t, typ, pattern, "no handler for %s", typ)
	}
	_, pattern := mux.Handler(asynq.NewTask(job.TypeConsumptionRefresh, nil))
	require.Empty(t, pattern, "R17: consumption.refresh has no F2 handler")
}
```

  - `scheduler_integration_test.go`: `TestSchedulerEntriesIncludeIngestionAndPrices` (internal test: crons equal `cfg.Schedule.Ingestion`/`EPIAS`, task types match); `TestSchedulerReleasesRedisClientOnRegisterFailure` (an invalid cron → `Run` returns an error; `goredis` pool stats show 0 open connections after return).
  - `elector_integration_test.go`: `TestElectorLogsFailedUnlock`; `TestElectorBacksOffOnRepeatedLeadFailure` (a fake clock records increasing waits).

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/platform/config/ ./internal/worker/ -race -v
go test ./internal/worker/ ./internal/scheduler/ -tags=integration -race -count=1 -v
```

- [ ] **Step 3: Implement.** In `cli/worker.go`, replace `job.Register(mux, &job.Handlers{Log: log})` with pool creation (`postgres.NewPool`, deferred close) → `worker.Build` → `defer built.Close()` → `job.Register(mux, built.Handlers)`. Keep the broker-health and signal-handling structure exactly as it is. `env-reference.md` gains three rows under the worker section (`BCEM_INGEST_*` naming, per the doc's convention; F0 swaps the prefix). `.env.example` gains the defaults.

- [ ] **Step 4: Watch them pass.**

- [ ] **Step 5: Prove it.** (a) Remove `Backfill:` from the `Build` return: `TestWorkerRegistersEveryF2Handler` FAILS naming `integration.backfill`. (b) Restore `asynq.NewScheduler(opt, …)`: `TestSchedulerReleasesRedisClientOnRegisterFailure` FAILS. Restore.

- [ ] **Step 6: Run the process for real.**

```bash
make build && make up
docker compose logs worker scheduler | grep -E "worker started|scheduler running" 
docker compose exec redis redis-cli -n 1 KEYS 'asynq:{default}:*' | head
make down
```
Expected: both processes start, and `scheduler running … entries=3`. Record the output.

- [ ] **Step 7: Gate and commit**

```bash
make lint && make test && make build && make check-generate
go test ./... -tags=integration -race -count=1
git add internal/worker internal/cli internal/scheduler internal/platform/config docs/rewrite/appendix/env-reference.md .env.example
git commit -m "feat(f2): wire ingestion, backfill and price sync into worker and scheduler; fix carried scheduler defects" <TRAILERS>
```

---

## Task 17: Acceptance suite, the F2 verification block, and the handoff

**Files:**
- Create: `internal/job/ingestion_integration_test.go` (package `job_test`), `internal/arch/adapter_matrix_test.go`
- Modify: `HANDOFF_NEXT_SESSION.md`

**Interfaces:**
- Consumes: `worker.Build`, `job.Register`, `job.NewServer`, `job.NewClient`, `fake.NewTLSServer`, `testfixtures.*`.
- Produces: the evidence for every F2 acceptance criterion, end to end through a real asynq worker, Redis and TimescaleDB. `job_test` imports `internal/store/...` only from `_test.go`, which `TestTheJobPackageDoesNotImportTheStore` does not load (it loads without `Tests`). **Assert that in a comment and keep the file `_test.go`.**

- [ ] **Step 1: Write the matrix and billing guards** (`internal/arch/adapter_matrix_test.go`)

```go
// TestAllSixAdaptersHaveTheFixtureMatrix is the structural half of the F2
// acceptance criterion "every adapter has recorded fixtures covering …". It
// fails if an adapter's test file lacks a subtest for any fake.RequiredCases
// entry, or its testdata lacks the fixture file.
func TestAllSixAdaptersHaveTheFixtureMatrix(t *testing.T) {
	for provider, testFunc := range map[string]string{
		"osos": "TestOSOSFixtureMatrix", "gridbox": "TestGridBoxFixtureMatrix", "aril": "TestARILFixtureMatrix",
		"pm5340": "TestPM5340FixtureMatrix", "isolar": "TestISolarFixtureMatrix", "epias": "TestEPIASFixtureMatrix",
	} {
		// parse internal/integration/<provider>/*_test.go with go/parser; find testFunc;
		// collect string literals of `name:` fields in its table; require ⊇ fake.RequiredCases;
		// require internal/integration/<provider>/testdata/<provider>_<case>.json exists for each case.
	}
}

// TestBillingPathDoesNotImportEPIAS pins removed-behaviour 24 ahead of F4.
func TestBillingPathDoesNotImportEPIAS(t *testing.T) { /* Task 12 description */ }
```

Then run `go test ./internal/integration/fake/ -run TestFixturesAreSanitised -v` and confirm it now inspects > 0 files (`t.Logf` the count; `require.Positive`, the F1 Task 9 lesson that a guard over an empty set proves nothing).

- [ ] **Step 2: Write the end-to-end acceptance tests** (`//go:build integration`)

Common setup `acceptEnv(t)`:
- `pool := testfixtures.NewIsolatedDB(t)`; `tn := testfixtures.NewTenant(t, ctx, pool, 17)`; Redis via `testfixtures.RedisConfig(t)`.
- A `fake.NewTLSServer` per provider under test, with `cfg.External.PinnedCerts = srv.Pins`.
- An integration definition inserted with `admin.NewCatalogueRepository(pool).UpsertIntegrationDefinitions`, whose endpoint templates point at `srv.URL`.
- The credential configured through `credentials.Service.Configure`, never raw SQL.
- `built := worker.Build(...)`; `server, mux := job.NewServer(...)`; `job.Register(mux, built.Handlers)`; `server.Start(mux)`, with `t.Cleanup` Stop/Shutdown.
- Wait for completion with `require.Eventually` polling `job_runs` (bound 60 s, tick 200 ms; never `time.Sleep`).

```go
func TestIngestionEndToEndSameWindowTwiceProducesNoDuplicates(t *testing.T)      // gridbox fixture; enqueue FetchReadings with Window twice; count rows == fixture rows
func TestIngestionEndToEndInterruptedFetchResumesFromCursor(t *testing.T)        // pm5340 fake: page 2 returns 500 ×3 (exhausts in-client attempts) then OK after a flag flip; asynq retries; final rows complete, no dupes; second attempt's first request start == cursor
func TestIngestionEndToEndOneAnalyzerFailureDoesNotAbortTheRun(t *testing.T)     // osos: three activated analyzers, one installation returns 500; sync run success/processed 3; two fetch runs success, one failed with reason; others' rows present
func TestIngestionGridBoxOffsetlessTimestampStoredAsUTCInstant(t *testing.T)     // stored ts for "2026-09-14T10:15:00" == 2026-09-14T07:15:00Z, read back via `select ts at time zone 'UTC'`
func TestIngestionARILTimeOfUseRegistersStoredAsNull(t *testing.T)              // `select count(*) … where t1_import is not null or t2_import is not null or t3_import is not null` == 0 and active_import not null
func TestIngestionPM5340CumulativeGenerationThroughTheWorker(t *testing.T)       // gap + out-of-order + duplicate, driven by three Window fetches; hand-written expected active_export
func TestIngestionNegativeDeltaThroughTheWorkerCreatesAnomaly(t *testing.T)
func TestIngestionSecretsNeverReachRunsCursorsMessagesOrLogs(t *testing.T)     // auth-failure fixtures echo the password; a slog JSON handler via logging.New into a buffer; grep all four sinks
func TestEPIASSyncThroughTheWorkerIsIdempotent(t *testing.T)
func TestAuthFailureIsNotRetried(t *testing.T)                                   // asynq task archived after 1 attempt (SkipRetry); inspector shows Retried == 0
```

- [ ] **Step 3: Run and watch them fail, then fix wiring gaps only.** A failure that needs a behaviour change is a defect in the owning task. Open a fix round on that task's branch; do not patch around it here.

```bash
go test ./internal/job/ -tags=integration -race -count=1 -run 'TestIngestion|TestEPIAS|TestAuthFailure' -v
go test ./internal/arch/ -race -run 'TestAllSixAdapters|TestBillingPath' -v
```

- [ ] **Step 4: Prove the matrix guard fails.** Rename the `pagination` subtest in `TestARILFixtureMatrix` to `paging`: `TestAllSixAdaptersHaveTheFixtureMatrix` FAILS naming `aril/pagination`. Restore.

- [ ] **Step 5: Run the full gate three times** (catches load-dependent flakes; check any failure for the readiness trap first)

```bash
make lint && make test && make build && make check-generate
go test ./... -tags=integration -race -count=3 2>&1 | tee /tmp/f2-full-count3.txt; echo "exit=${PIPESTATUS[0]}"
```

- [ ] **Step 6: Run the F2 verification block verbatim** (09 §F2) and record the real output:

```bash
go test ./internal/integration/... -v
go test ./internal/job/... -run TestIngestion -tags=integration -v
go test ./internal/integration/... -run TestIdempotent -v
grep -rn "InsecureSkipVerify" internal/ && exit 1 || echo "no TLS bypass"
```

The `grep` line matches `internal/arch/arch_test.go`, whose guard necessarily contains the literal, plus the TLS AST guard's own literal. **Ruling:** run it as `grep -rn "InsecureSkipVerify" internal/ --include=*.go | grep -v "^internal/arch/"`; expected empty, then `no TLS bypass`. Record both the raw and the filtered output, and say why they differ.

- [ ] **Step 7: Update the handoff and commit**

`HANDOFF_NEXT_SESSION.md` for F3 must record:
- what F2 delivered;
- every R-ruling that was exercised or changed;
- the BLOCKER status;
- Q2–Q4;
- that `consumption.refresh` is declared but unhandled (F3 wires `ingest.Deps.ConsumptionRefresh`, reads `affected_from/affected_to` from fetch runs, and must refresh aggregates for backfilled history older than 30 days);
- the F14 live-verification checklist (OSOS multiplier, ARIL `WithoutMultiplier`, iSolar result codes, EPİAŞ response field names).

```bash
git add internal/job/ingestion_integration_test.go internal/arch/adapter_matrix_test.go HANDOFF_NEXT_SESSION.md
git commit -m "test(f2): end-to-end ingestion acceptance suite and F2 handoff" <TRAILERS>
```

---

## Carried forward — explicitly NOT F2

- **`isolar.sync_plant`, `isolar.fetch_alarms`, device sync and alarm forwarding to recipients / `isolar_forwarded_alarms`**: F9 (legacy-inventory §4). F2 supplies `isolar.Client`, `production.Store` and OAuth.
- **`consumption.refresh` handler and suspect-period resolution**: F3 (R17).
- **HTTP routes for 05 §15 and `/analyzers/{id}/refresh`, `/job-runs/{type}/trigger`**: F6, with the authorisation matrix (R23).
- **Weather adapter (06 §8)**: not in 09 §F2's adapter list; it belongs to the phase that builds the renewable dashboard (F9).
- **Data-communication alarm from ingestion health**: F7 reads `ingestion_cursors`/`analyzers.last_reading_at`.
- **`plant_production` primary-key migration**: on the product owner's answer (BLOCKER).
- **Billing's "flag when prices are missing"**: F4 reads `market_prices_hourly`; F2 only guarantees absence is visible (removed-behaviour 10).
- **Metrics** (03 §8 "integration call latency and error rate by provider"): 09 §F2 does not list them. **Spec silent — ruling:** deferred to F15 hardening; `httpx` exposes no metrics hook in F2 (YAGNI).

---

## Self-review

### Acceptance criteria → tests (09 §F2)

| # | Criterion | Claimed by (task: test) |
|---|---|---|
| 1 | Fixtures: success, empty, partial, auth failure, malformed, rate limiting, pagination, per adapter | T6 `TestOSOSFixtureMatrix`; T7 `TestGridBoxFixtureMatrix`; T8 `TestARILFixtureMatrix`; T9 `TestPM5340FixtureMatrix`; T13 `TestISolarFixtureMatrix`; T12 `TestEPIASFixtureMatrix`; structural guard T17 `TestAllSixAdaptersHaveTheFixtureMatrix`; sanitisation T4 `TestFixturesAreSanitised` |
| 2 | Same window twice → no duplicate rows | T10 `TestIngestionSameWindowTwiceProducesNoDuplicateRows`; T17 `TestIngestionEndToEndSameWindowTwiceProducesNoDuplicates`; per adapter `TestIdempotent*` (T6–T9, T12, T13) |
| 3 | Interrupt + re-run resumes from cursor, loses nothing | T10 `TestIngestionInterruptedFetchResumesFromCursor`; T17 `TestIngestionEndToEndInterruptedFetchResumesFromCursor` |
| 4 | One analyzer failing does not abort; job_runs counts + reason | T10 `TestIngestionOneAnalyzerFailureDoesNotAbortTheRun`; T17 `TestIngestionEndToEndOneAnalyzerFailureDoesNotAbortTheRun` |
| 5 | GridBox multiplier: three paths, fallback warns | T7 `TestGridBoxMultiplierResolution` (`last_endex`, `derived_from_load_profile`, `fallback_to_one_warns`) |
| 6 | GridBox RI→inductive, RC→capacitive | T7 `TestGridBoxReactiveRegistersAreNotCrossed` |
| 7 | Offset-less timestamp stored as correct UTC instant | T3 `TestOffsetlessTimestampIsIstanbulLocal`; T7 `TestGridBoxOffsetlessTimestampIsIstanbul`; T17 `TestIngestionGridBoxOffsetlessTimestampStoredAsUTCInstant` |
| 8 | ARIL T1/T2/T3 NULL not 0 | T8 `TestARILTimeOfUseRegistersAreNull`; T17 `TestIngestionARILTimeOfUseRegistersStoredAsNull` |
| 9 | PM5340 accumulation across gap, out-of-order batch, duplicate fetch | T11 `TestGenerationAccumulatesAcrossAGap`, `TestGenerationOutOfOrderBatchRecomputesLaterRows`, `TestGenerationDuplicateFetchIsStable`; T9 `TestPM5340NeverDerivesCumulativeExport`; T17 `TestIngestionPM5340CumulativeGenerationThroughTheWorker` |
| 10 | Negative index delta → unresolved `consumption_anomalies` row | T10 `TestDetectNegativeDeltas`, `TestIngestionNegativeIndexDeltaCreatesUnresolvedAnomaly`; T17 `TestIngestionNegativeDeltaThroughTheWorkerCreatesAnomaly` |
| 11 | No test passes with TLS verification disabled | existing `TestNoTLSVerificationBypass`; T1 `TestNoTLSConfigDisablesVerification`; T2 `TestUnpinnedSelfSignedServerIsRefused`, `TestPinnedHostRejectsADifferentCertificate`, `TestPoolNeverAcceptsAnUnverifiedCertificate` (R5 — behavioural, not a white-box `InsecureSkipVerify` field check); T4 `TestFakeServerIsTLSAndRecordsRequests`, `TestTwoFakeServersHaveDistinctCertificates` (R4); T17 Step 6 grep |

### Scope coverage (09 §F2 bullets)

- `MeterDataSource` + typed errors → T1.
- HTTP timeouts, retry, backoff, jitter, rate limiting, pinning, URL templates → T2.
- Six adapters → T6, T7, T8, T9, T12, T13.
- Pipeline with cursors, COPY-staging upsert (F1 `BulkInsert`, extended in T5), validation and rejection reporting → T10.
- Backfill → T15 (meters), T12 (prices).
- Credential encryption + write-only surface → T14 (F1 `IntegrationRepository` seals).
- iSolar OAuth with per-company locked refresh → T13 + T14.
- EPİAŞ sync → T12.
- Scheduling → T16.

### Guards added or widened, each with its proving mutation

`TestAdaptersDoNotImportTheStore`, `TestStoreAndIntegrationDoNotImportAPIOrService` (widened), `TestIntegrationTreesDoNotParseFloats`, `TestNoTLSConfigDisablesVerification`, float field guard + depguard reach (T1); `TestHTTPErrorsNeverContainSubstitutedSecrets` and the pinning trio (T2); `TestFixturesAreSanitised`/`TestSanitiserRejects`, `TestTwoFakeServersHaveDistinctCertificates`, `TestMemoryConsumeNeverBlocks`/`TestRedisConsumeIsSingleUseAndNonBlocking` (T4); new isolation predicates for `GenerationAnchorUpsert` **and** `ProviderHourlyVisibleAnalyzerIDs` (T5, S4); `TestViewCarriesNoSecretMaterial` (T14, R1 — type/value-based, not a name regex), `TestStateIsSingleUse` (T14, R3 — bounded by a short ctx deadline, so a hang fails rather than passing slowly); `TestWorkerRegistersEveryF2Handler` (T16); `TestAllSixAdaptersHaveTheFixtureMatrix`, `TestBillingPathDoesNotImportEPIAS` (T17).

### Type consistency (checked by name across tasks)

`integration.Adapter`/`Planner`/`Credentials`/`Secret`/`HourlyValue`/`Warning`/`ResolvedMultiplier` (T1 → T6–T15). `httpx.Pool`/`ClientConfig`/`Request`/`Param` (T2 → adapters, T16). `lock.Locker`/`lock.Lease`/`lock.Consumer` (T4 → T2 as a type alias — R2 — and directly into T12, T14, T16 — R3). `normalize.Chunk`/`normalize.Window` (T3 → T10, T12, T15 — S7, the one range-chunking implementation). `store.SystemScope`, `store.GenerationRepository`, `store.ProviderSeriesRepository`, `store.AdminIngestionRepository`, `model.CredentialRef`, `model.GenerationAnchor`, `model.ProviderHourlyValue`, `MeterReading.IntervalGenerationKwh` (T5 → T9, T10, T11, T16). `ingest.PostPersistHook`/`CredentialOpener`/`SourceResolver`/`Enqueuer` (T10 → T11, T14, T15). `job.*Payload`, `job.New*Task`, `job.TaskOptions`, `job.ClassifyForRetry`, `job.RetryDelay`, `job.Ingestion`/`Backfiller`/`PriceSyncer` (T1 → T10, T12, T15, T16, T17). `isolar.Token` (T13 → T14, fully merged — Wave E starts only after Wave D). `worker.Build` (T16 → T17).

### Known risks, stated rather than hidden

1. **Timescale `add/drop column` on a columnstore-enabled hypertable** (T5 Step 3) is the one schema operation F1 never exercised; T5 stops and reports rather than reshaping R1.
2. **Provider response shapes for OSOS, iSolar result codes and EPİAŞ field names** come from legacy code, not from recorded live traffic (none can be captured without real credentials). Fixtures encode the rulings; F14's live checklist verifies them (Q4).
3. **Wave C is the widest merge** (five adapters — 6, 7, 8, 9, 12 — plus the pipeline, T10; S11 moved T12 here). That is six tasks against the 5-concurrent-agent budget: dispatch 6/7/8/9/10 first, start 12 as its sixth slot frees. The adapters share no files, but all six add fixtures that `TestFixturesAreSanitised` reads; merge them one at a time and run the sanitiser after each.
4. **The end-to-end suite (T17) starts Redis and uses one Postgres clone per test.** If readiness timeouts appear under `-count=3`, apply the F1 watch item (`TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1`) only on a second observation, and record it.
