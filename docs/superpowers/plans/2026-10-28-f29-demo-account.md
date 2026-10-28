# F29 Demo Account Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans (the owner's mode: one session, inline, no subagents). Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A shared, writable demo login whose company shows every customer-facing page with believable mock data, resets to its baseline every hour, and never reaches outside the platform or touches another company.

**Architecture:** `companies.is_demo` tags the demo company; a cached `demo.Checker` answers "is this company a demo?". The `demo` role gets the company-admin permission set plus four lock-outs. Two decorators (mail sender, integration adapters + iSolar client) short-circuit every outbound effect for demo companies. A deterministic baseline generator (`internal/seed/demo*.go`) writes the dataset; an hourly `demo.reset` job purges the demo company's mutable rows in one transaction (time series and their parents are kept) and re-runs the generator. `/auth/me` carries `demo.resets_at`; the web shell shows a banner.

**Tech Stack:** Go 1.27, pgx/sqlc, TimescaleDB 2.30, asynq, Next.js 15 + next-intl, vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-10-28-demo-account-design.md`

## Global Constraints

- Rulings for this phase are **R540–R559** (F28 is unmerged; its numbers are avoided).
- Env prefix for every command: `export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
- Integration tests: `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <pkg> -tags=integration -count=1 -parallel 4`. Never `make test` / `go test ./...`. A task's gate runs the **whole** touched packages.
- Never refresh a continuous aggregate with `NULL, NULL` bounds in demo code; never drop/create views.
- Banner copy (tr): `Demo hesabı` / `Bu hesaptaki veriler örnektir. Yaptığınız değişiklikler kalıcı değildir; hesap her saat başı ilk haline döner.` / `Sonraki sıfırlama: {time}`. (en): `Demo account` / `The data in this account is sample data. Your changes are not kept; the account returns to its initial state every hour on the hour.` / `Next reset: {time}`.
- Profile lock caption (tr): `Demo hesabında profil ve şifre değiştirilemez.` (en): `Profile and password cannot be changed on the demo account.`
- Demo identity stays: company id `seed.DemoCompanyID`, user `demo@ekokod.com.tr`, company name `Demo Enerji A.Ş.`.
- Comments 1–3 lines, WHY only, cite rulings.

## Rulings

| id | ruling | cost if wrong |
|---|---|---|
| R540 | `companies.is_demo` is the only demo marker; code asks `demo.Checker`, never compares ids (except the seed). Checker caches per company for 1 min, errors are not cached. | a second marker drifts |
| R541 | Role `demo` holds exactly CA's permission set; `IsReadOnly(demo)` is false; `assignable[demo]` = `assignable[CA]`. | demo shows less than a customer |
| R542 | Demo lock-outs: profile/password/forgot-password (existing), `Sessions`/`RevokeSession`/`LogoutAll` → 403 for role demo; a `demo`-role user can be edited/deleted only by platform admin (falls out of R541's `assignable`). | one visitor locks everyone out |
| R543 | Login of a non-`demo` user whose company `is_demo` → `ErrInvalidCredentials` (same as a wrong password, after the hash check). | visitor-made accounts become entry points |
| R544 | `mail.GuardDemo` drops a message whose `cfg.CompanyID` is a demo company and returns nil; a checker error is returned (fail closed). | prospects send real mail |
| R545 | `integration.GuardDemo` wraps every meter adapter: for a demo company `Verify`→nil, `DiscoverMeteringPoints`→seeded points, `FetchReadings`→empty result; `Planner` passes through. `isolar.GuardDemo` does the same for the iSolar client (`Verify`/`Refresh`/`ExchangeCode` → fake token, data calls → empty, `Plants` → the seeded plants, `AuthorizeURL` → `redirectURI?code=demo`). Applied in both api and worker wiring. | outbound call / SSRF from demo |
| R546 | Reset kept set: the demo company row; baseline buildings, analyzers, plants, plant devices; the demo user and its sessions; `audit_log`; every time series of baseline analyzers/plants. Everything else of the demo company is purged and regenerated. Non-baseline analyzers/plants lose their time series too. | lost history or leaked visitor rows |
| R547 | Purge is one transaction; generation runs after it. A Redis lock (`demo:reset`, 10 min) keeps resets single. A few seconds of partial data at HH:00 is accepted. | overlapping resets corrupt state |
| R548 | After a purge that deleted readings/production, refresh `consumption_hourly`/`consumption_daily` (and plant views) only over `[min(ts)-48h, max(ts)+1h]` of what was deleted. | prod-wide aggregate recompute hourly |
| R549 | Schema guard: every table with a `company_id` column or an FK path to `companies`/`analyzers`/`power_plants`/`users` is listed in `demoTables` as purge, kept, or cascade. | a future table leaks past reset |
| R550 | Baseline is deterministic (uuid v5 under `demoNamespace`, mulberry32 noise) and relative to `now`; history window 400 days; readings hourly. New analyzer ids (`analyzer/v2/<n>`); the old `analyzer/1`, `analyzer/2` become non-baseline and are purged on the first reset. | stale or random demo |
| R551 | Demo tariffs use fixed prices (no PTF dependency); bills are produced by `billing.Service.Generate` for the last 12 closed months. | empty bills on a server without EPİAŞ |
| R552 | `/auth/me` adds `demo: {resets_at}` (next full hour, UTC instant) for demo companies, `null` otherwise. | banner lies |
| R553 | Schedule `EKOKOD_SCHEDULE_DEMO_RESET` default `0 * * * *`; `ekokod seed demo --reset` runs the same reset now; `--password` sets the demo user's password (policy-checked). | video shoot hits a reset |

## Review Focus

1. A visitor deletes a baseline building that has analyzers and bills, then the reset runs → building, analyzers, bills all back; readings count unchanged. (Task 8 test `TestResetRestoresDeletedBaseline`.)
2. A visitor creates an analyzer, a building, an iSolar credential and uploads an ISO document → reset removes the rows and the file on disk. (Task 8 `TestResetRemovesVisitorRows`.)
3. A second company's rows are byte-identical before/after a reset and a demo-user write session. (Task 8 `TestResetLeavesOtherCompaniesAlone`.)
4. Alarm rule with an external recipient fires for the demo company → no SMTP dial, message row says success. (Task 3 `TestGuardDemoDropsDemoMail` + Task 7 alarm evaluate test.)
5. Demo user hits every mutation route CA can → none is 403 except the R542 lock-outs. (Task 2 `TestDemoMutatesLikeCompanyAdminHTTP`.)

---

### Task 1: Demo tag and checker (R540)

**Files:**
- Create: `internal/store/postgres/migrations/00027_company_demo.sql`
- Modify: `internal/store/postgres/queries/companies.sql` (no change needed for `select *`; add nothing), `internal/store/postgres/queries/admin_tenant.sql` (add `AdminCompanySetDemo`), `internal/store/postgres/tenancy.go` (`companyFromRow` maps `IsDemo`), `internal/store/postgres/admin/tenant.go` (`SetDemo`), `internal/domain/model/tenancy.go` (`Company.IsDemo bool`), regenerated `internal/store/postgres/sqlcgen/*`
- Create: `internal/demo/checker.go`, `internal/demo/checker_test.go`
- Test: `internal/store/postgres/admin/tenant_integration_test.go`

**Interfaces:**
- Produces: `model.Company.IsDemo bool`; `(*admin.TenantRepository).SetDemo(ctx, companyID uuid.UUID, on bool) error`;
  `package demo`: `type Check func(ctx context.Context, companyID uuid.UUID) (bool, error)`; `func NewChecker(load Check, ttl time.Duration, now func() time.Time) *Checker`; `func (c *Checker) IsDemo(ctx context.Context, companyID uuid.UUID) (bool, error)`; `func FromCompanies(repo store.CompanyRepository) Check` (loads with `store.SystemScope(id)`, `ErrNotFound` → false).

- [ ] **Step 1:** Migration: `alter table companies add column is_demo boolean not null default false;` down drops it. `make generate`.
- [ ] **Step 2: Failing tests.** `TestCheckerCachesAndExpires` (loader called once for two calls within ttl, again after ttl), `TestCheckerDoesNotCacheErrors`, `TestCheckerUnknownCompanyIsNotDemo`; integration `TestSetDemoFlag` (SetDemo true → `CompanyRepository.Get(...).IsDemo == true`).
- [ ] **Step 3:** Implement; `internal/demo` imports only `store` interfaces, `model`, uuid.
- [ ] **Step 4:** Gate: `go test ./internal/demo ./internal/store/postgres/... -tags=integration` (whole packages), `make check-generate`, lint on touched packages. `internal/arch` once (new package).
- [ ] **Step 5:** Commit `feat(f29): companies.is_demo and a cached demo checker (R540)`.

### Task 2: Demo role = company admin, with lock-outs (R541–R543)

**Files:**
- Modify: `internal/auth/roles.go` (after `permissionTable`: every set holding CA also holds demo; `IsReadOnly` drops demo), `internal/service/tenancy/users.go` (`assignable[UserRoleDemo] = assignable[UserRoleCompanyAdmin]`), `internal/service/auth/sessions.go` (`Sessions`, `RevokeSession`, `LogoutAll` → `perr.Forbidden` for role demo), `internal/service/auth/login.go` (R543 after the company load; `Deps` gains nothing — `company.IsDemo` is on the row)
- Test: `internal/auth/permissions_test.go`, `internal/service/auth/service_integration_test.go`, `internal/api/v1/demo_http_integration_test.go`, `internal/api/v1/tenancy_sweep_integration_test.go` (drop demo from the read-only matrix), `internal/api/v1/authz_matrix_test.go`, `web/src/lib/session/permissions.fixture.json` (regenerate the demo row the way the file is produced today)

**Interfaces:**
- Consumes: `model.Company.IsDemo` (Task 1).

- [ ] **Step 1: Failing tests.**
  - `TestDemoHoldsCompanyAdminPermissions`: `PermissionsFor(demo)` equals `PermissionsFor(company_admin)`; `IsReadOnly(demo) == false`.
  - `TestDemoMutatesLikeCompanyAdminHTTP` (in `demo_http_integration_test.go`, demo company seeded with `is_demo`): create tariff 201, create alarm 201, PATCH company 200, create user 201; `PATCH /auth/me` 403, `POST /auth/change-password` 403, `GET /auth/sessions` 403, `DELETE /auth/sessions/{id}` 403, `POST /auth/logout-all` 403; PATCH/DELETE of the demo user via `/users/{id}` 403.
  - `TestLoginRefusesVisitorUsersOfDemoCompany`: user created by demo in the demo company → login 401 `invalid_credentials`; the demo user logs in 200; a CA of a normal company is unaffected.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. Update every existing test that asserts demo is read-only (grep `UserRoleDemo`, `demo@`, `'demo'` in tests) to the new rule.
- [ ] **Step 4:** Gate whole packages: `./internal/auth/... ./internal/service/auth/... ./internal/service/tenancy/... ./internal/api/v1/...` with `-tags=integration`; web `pnpm test --maxWorkers=2 src/lib/session src/components/shell`.
- [ ] **Step 5:** Commit `feat(f29): the demo role acts as a company admin with lock-outs (R541–R543)`.

### Task 3: Containment — mail and integrations (R544, R545)

**Files:**
- Create: `internal/mail/demo.go`, `internal/mail/demo_test.go`, `internal/integration/demo.go`, `internal/integration/demo_test.go`, `internal/integration/isolar/demo.go`, `internal/integration/isolar/demo_test.go`
- Modify: `internal/worker/wiring.go:328-340,490,523,555`, `internal/apiwire/wiring.go:105,271,429-436` (wrap senders, adapters, iSolar clients with one shared `*demo.Checker` built from `demo.FromCompanies(postgres.NewCompanyRepository(pool))`, ttl 1 min)

**Interfaces:**
- Consumes: `demo.Check` (Task 1).
- Produces: `func mail.GuardDemo(inner Sender, isDemo demo.Check) Sender`; `func integration.GuardDemo(a Adapter, isDemo demo.Check) Adapter`; `func isolar.GuardDemo(c *Client, isDemo demo.Check) *Guarded` where `*Guarded` has every exported method of `*Client` with the same signature (so it satisfies `solar.Adapter`, the credentials iSolar dep and `credentials.Verifier`). `integration.DemoPoints []MeteringPoint` and `isolar.DemoPlants []Plant` are the seeded values Task 5 writes too.

- [ ] **Step 1: Failing tests.**
  - `TestGuardDemoDropsDemoMail`: inner spy not called for a demo company, err nil; called once for a normal company; checker error → returned, inner not called.
  - `TestGuardDemoAdapterNeverCallsInnerForDemo`: spy adapter; for demo creds `Verify` nil, `FetchReadings` empty `FetchResult` nil err, `DiscoverMeteringPoints` == `DemoPoints`; inner call count 0; for normal creds every call reaches inner once; `Provider()`/`Kinds`/`MaxWindow` pass through.
  - `TestISolarGuardDemo`: against an `httptest.Server` that fails the test on any request: demo creds → `Verify` nil, `Refresh` returns a token expiring in 24 h, `Plants` == `DemoPlants`, `AuthorizeURL(creds, "https://x/cb")` == `https://x/cb?code=demo`.
  - `TestWiringGuardsEveryAdapter` (worker `wiring_internal_test.go`): every registry adapter's dynamic type is the guard type.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement. Check the iSolar callback path: with code `demo` the `ExchangeCode` guard returns a fake token, so `ISolarCallback` completes; state verification stays unchanged.
- [ ] **Step 4:** Gate: `./internal/mail/... ./internal/integration/... ./internal/worker/... ./internal/apiwire/... ./internal/credentials/... ./internal/ingest/...` with `-tags=integration`.
- [ ] **Step 5:** Commit `feat(f29): demo companies never send mail or call integrations (R544, R545)`.

### Task 4: Purge and the schema guard (R546, R548, R549)

**Files:**
- Create: `internal/store/postgres/admin/demo.go`, `internal/store/postgres/admin/demo_integration_test.go`

**Interfaces:**
- Produces:
  ```go
  type DemoKeep struct {
      Buildings, Analyzers, Plants, Devices []uuid.UUID
      User uuid.UUID
  }
  type PurgeResult struct {
      Files        []string        // stored_files paths deleted (relative, for the storage layer)
      ReadingRange *store.TimeRange // nil when no reading was deleted
      PlantRange   *store.TimeRange
  }
  func NewDemoRepository(pool *pgxpool.Pool) *DemoRepository
  func (r *DemoRepository) Purge(ctx context.Context, companyID uuid.UUID, keep DemoKeep) (PurgeResult, error)
  // DemoTables is R549's classification, exported for the guard test.
  var DemoTables map[string]DemoTableKind // "purge" | "kept" | "cascade" | "platform"
  ```
- `Purge` refuses (error) unless `companies.is_demo` is true for `companyID` — checked inside the transaction.

Order inside the transaction (one `pgx.Tx`):
1. `select is_demo ... for update` → refuse when false.
2. Detach kept rows from purgeable ones: `buildings.responsible_user_id = null`; `analyzers.building_id = null` where the building is not kept; `power_plants.isolar_credential_id = null`, `netting_analyzer_id = null` where not kept.
3. Delete (by company, or by kept/non-kept parent id): `alarm_fired_bills`, `alarms` (cascades), `bills` (cascades), `reports`, `legacy_reports`, `legacy_bills`, `tariff_bulk_assignments`, `solar_tariffs`, `tariffs` (cascades), `tariff_templates`, `icmal_imports` (cascades), `carbon_reports`, `carbon_activities`, `carbon_selected_activities`, `emission_factors where company_id = $1` (cascades), `iso50001_projects` (cascades), `calendar_events`, `company_vacations`, `company_weekend_days`, `job_runs`, `operational_messages`, `consumption_anomalies` (by demo analyzers), `plant_faults`, `isolar_forwarded_alarms`, `power_plant_monthly_targets`, `power_plant_alarm_recipients`, `building_contacts`, `smtp_settings`, `integration_credentials`, `stored_files` (collect paths), time series of non-kept analyzers (`meter_readings`, `provider_hourly_values`, `forecasts`, `forecast_gaps`, `ingestion_cursors`, `generation_anchors`; collect min/max ts), time series of non-kept plants/devices (`plant_production`, `plant_production_totals`), `power_plant_devices` not kept, `power_plants` not kept, `analyzers` not kept, `buildings` not kept, `users` other than the kept user (cascades sessions/history/resets).
4. Commit. Aggregate refresh (R548) happens in Task 8, outside the transaction.

- [ ] **Step 1: Failing tests.**
  - `TestDemoTablesCoverSchema`: query `information_schema.columns` (column `company_id`) ∪ tables with an FK (`pg_constraint`, contype `f`) to `companies`, `analyzers`, `power_plants`, `users`, `buildings`, `bills`, `alarms`, `tariffs`, `iso50001_projects`, `icmal_imports`, `emission_factors`, `power_plant_devices`, `sessions`; every name must be a key of `DemoTables`; add a fake table in the test DB → the test reports it (prove the guard once by mutation).
  - `TestPurgeRefusesNonDemoCompany`: normal company → error, nothing deleted.
  - `TestPurgeKeepsKeptRowsAndSeries`: seed a demo company (is_demo) with 2 kept + 1 extra analyzer (readings on all), a bill, a tariff, an alarm with event, a visitor user with a session, a stored file; Purge → kept analyzers' reading counts unchanged, extra analyzer and its readings gone, bill/tariff/alarm/visitor user gone, demo user and its session present, `Files` holds the file path, `ReadingRange` covers the extra analyzer's readings.
  - `TestPurgeLeavesOtherCompanies`: a second company with the same kinds of rows → `count(*)` per table for it unchanged.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement with plain SQL in `admin` (the unscoped package; arch allows it).
- [ ] **Step 4:** Gate `./internal/store/postgres/admin/...` `-tags=integration`; `internal/arch`.
- [ ] **Step 5:** Commit `feat(f29): demo purge with a schema coverage guard (R546, R549)`.

### Task 5: Baseline generator — sites, series, people (R550)

**Files:**
- Modify: `internal/seed/demo.go` (identity, `Demo`, `ExtendDemo`, `DemoJob` stay; body moves to the new files), `internal/seed/readings.go` (profiles gain `export` and `solarKwp` fields; `fillHourly` takes a `[]readingProfile` directly)
- Create: `internal/seed/demo_sites.go`, `internal/seed/demo_people.go`, `internal/seed/demo_series.go`, `internal/seed/demo_generator_integration_test.go`

**Interfaces:**
- Produces:
  ```go
  type DemoDeps struct {
      Pool    *pgxpool.Pool
      Hasher  auth.Hasher
      Cipher  *crypto.Cipher        // integration secrets and SMTP password
      Storage storage.Store         // demo documents
      Billing *billing.Service      // Task 6
      Reports *reportsvc.Service    // Task 7
      Log     *slog.Logger
  }
  func DemoBaseline() demoBaseline           // ids and static values, no I/O
  func (b demoBaseline) Keep() admin.DemoKeep
  func ensureDemoSites(ctx, d DemoDeps, b demoBaseline, now time.Time) error   // company (is_demo via admin.SetDemo), buildings, analyzers, plants, devices: create or overwrite to baseline, clear deleted_at
  func ensureDemoPeople(ctx, d DemoDeps, b demoBaseline, password string, now time.Time) error // demo user (password only when creating or when password != ""), 4 colleagues, calendar, credentials, SMTP
  func ensureDemoSeries(ctx, d DemoDeps, b demoBaseline, now time.Time) (int, error) // readings + plant production, fill missing hours in [now-400d, now]
  ```

Baseline values (exact):
- Company: name `Demo Enerji A.Ş.`, sector `Üretim`, area `18500.00`, personnel `240`, address `Ostim OSB, Ankara`, contact `Demo Kullanıcı`.
- Buildings: `Ostim Fabrika` (39.9714, 32.7434, area 12000, 180 people, cutoff 1), `Merkez Ofis` (39.9087, 32.8597, Çankaya, area 1500, 45 people), `Gebze Lojistik Depo` (40.8028, 29.4307, Kocaeli, area 5000, 15 people).
- Analyzers (provider OSOS subtype `Baskent`, multiplier 1, active): `DEMO-1001` factory main (power 400 kW, profile day 160 / night 70), `DEMO-1002` production line (day 90 / night 20), `DEMO-1003` compressor room (day 45 / night 30, inductive share 28–36 % so the reactive warning shows), `DEMO-2001` office main (day 35 / night 6), `DEMO-3001` warehouse main (day 40 / night 12, export when the 250 kWp rooftop curve exceeds load), `DEMO-3002` cold room (day 18 / night 16).
- Plants: `Gebze Çatı GES` (250 kWp, rooftop, netting analyzer `DEMO-3001`, 2 inverters), `Konya Arazi GES` (1000 kWp, field, 4 inverters, iSolar-linked to the seeded iSolar credential with ps id from `isolar.DemoPlants`). Production hourly from a clear-sky curve × seasonal factor × mulberry32 cloud noise.
- People: colleagues `Ayşe Yılmaz` (building_admin, Ostim Fabrika), `Mehmet Kaya` (company_readonly_admin), `Zeynep Demir` (building_readonly_admin, Merkez Ofis), `Can Öztürk` (building_admin, Gebze Lojistik Depo), e-mails `<first>.<last>@demo-enerji.example` (ASCII), random unusable password hash.
- Calendar: weekend days Sat+Sun; vacations = 2026 and 2025 Turkish public holidays (Ramazan Bayramı 2026-03-20..22, Kurban Bayramı 2026-05-27..30, 23 Nisan, 1 Mayıs, 19 Mayıs, 15 Temmuz, 30 Ağustos, 29 Ekim, 1 Ocak, and the 2025 equivalents); events `Planlı bakım` (factory, monthly 2nd Saturday), `ISO 50001 iç denetimi` (in 3 weeks).
- Credentials: OSOS Başkent (username `demo.osos`, secret `demo-secret`), GridBox, iSolar (fake tokens, expiry +30 d), all verified `now-2h`, last sync `now-20m`. SMTP: host `smtp.demo.invalid`, port 587, user `bildirim@demo-enerji.example`.

- [ ] **Step 1: Failing tests** (integration, own isolated DB): `TestDemoBaselineIsDeterministic` (two `DemoBaseline()` equal; ids stable); `TestEnsureDemoSitesConverges` (run, mutate a building name and soft-delete an analyzer, run again → baseline values, `deleted_at` null, no duplicate rows); `TestDemoSeriesFillsWindow` (fake now; each analyzer has 400×24 readings ±1; second run inserts 0; warehouse analyzer has non-zero export on a sunny weekend noon; plant production zero at 02:00 local); `TestDemoPeopleCannotLogIn` (colleague login refused — relies on Task 2).
- [ ] **Step 2:** FAIL. **Step 3:** Implement; reuse `fillHourly`, `mulberry32`, e2e helpers where they fit; bounded aggregate refresh only (Global Constraints).
- [ ] **Step 4:** Gate `./internal/seed/...` `-tags=integration`.
- [ ] **Step 5:** Commit `feat(f29): demo baseline — sites, series and people (R550)`.

### Task 6: Baseline generator — tariffs and bills (R551)

**Files:** Create `internal/seed/demo_money.go`; test in `internal/seed/demo_generator_integration_test.go`.

**Interfaces:** Produces `func ensureDemoMoney(ctx, d DemoDeps, b demoBaseline, now time.Time) error`.

Values: Ostim Fabrika tariff `OG Sanayi Çift Terimli Üç Zamanlı` (voltage MV, group industrial, three-time fixed prices day 3.1250 / peak 4.6875 / night 1.9531, distribution 1.0834, contracted 400 kW, power unit 32.10, reactive 3.4937); Merkez Ofis `AG Ticarethane Tek Terimli Tek Zamanlı` (single 3.4547, distribution 2.4794); Gebze `AG Sanayi Tek Zamanlı — Mahsuplaşmalı` (single 3.2010, generation usage per F26 netting). Taxes BTV 5 % (1 % industrial), KDV 20. One tariff template (`Sanayi OG şablonu`) with a bulk assignment to Ostim Fabrika; one icmal import with 12 rows for Ostim Fabrika; solar tariffs: Gebze feed-in 2.0000, Konya plant MWh price per F20 (2.1500/kWh). Bills: `billing.Service.Generate` for each of the last 12 closed months, scope building and analyzer, status issued.

- [ ] **Step 1: Failing test** `TestDemoMoneyBills`: 3 buildings × 12 months building bills and 6 × 12 analyzer bills exist, every `TotalCost > 0`, the compressor analyzer has a non-zero reactive line in at least one month, Gebze bills carry an export/netting line; a second run creates nothing new.
- [ ] **Step 2–4:** FAIL → implement → gate `./internal/seed/...`, `./internal/service/billing/...` `-tags=integration`.
- [ ] **Step 5:** Commit `feat(f29): demo tariffs and engine-computed bills (R551)`.

### Task 7: Baseline generator — operations and content

**Files:** Create `internal/seed/demo_content.go`, `internal/seed/demo_files/` (3 small real PDFs: `enerji-politikasi.pdf`, `enerji-gozden-gecirme.pdf`, `karbon-dogrulama.pdf`, each < 20 KB, embedded with `//go:embed`); test in the same integration file.

**Interfaces:** Produces `func ensureDemoContent(ctx, d DemoDeps, b demoBaseline, now time.Time) error`.

Content:
- Alarms (5): `Fabrika güç aşımı` (power > 380 kW on DEMO-1001, mail to `enerji@demo-enerji.example`), `Günlük tüketim sınırı` (office > 900 kWh/day), `Reaktif oran` (DEMO-1003 inductive > 20 %), `Fatura tutarı` (Ostim bill > 450 000 TL), `Veri gelmiyor` (any analyzer silent > 6 h). ~40 events over the last 30 days distributed deterministically, a few acknowledged; matching operational messages; job runs (fetch success hourly for the last 24 h, one partial yesterday, billing monthly).
- Anomalies: 6 over 90 days on DEMO-1001/1002, two resolved.
- Forecasts: hourly for the last 30 days and next 7 days per analyzer = baseline profile × 0.97–1.03 (only hours with no forecast row).
- Reports: monthly for the last 3 closed months × 3 buildings and the last closed year, built with `d.Reports.Build` like `ensureReports`.
- ISO 50001: project for Ostim Fabrika (baseline year last year, target −8 %), clause dates with 60 % complete, 4 notes, 2 documents (the embedded PDFs stored via `d.Storage`).
- EKOCM: selected activities (scope 1 natural gas + diesel forklift + company cars; scope 2 grid electricity; scope 3 business travel + employee commute), 12 months of activity rows with platform factors, one carbon report for last year, one document.

- [ ] **Step 1: Failing test** `TestDemoContentFillsEveryScreen`: counts per table as listed above (alarms 5, events ≥ 35, reports 10, forecasts cover now+7 d, iso project 1 with notes 4, carbon activities ≥ 60, stored files 3, messages ≥ 30); second run adds nothing.
- [ ] **Step 2–4:** FAIL → implement → gate `./internal/seed/...` `-tags=integration`.
- [ ] **Step 5:** Commit `feat(f29): demo alarms, reports, forecasts, ISO 50001 and carbon content`.

### Task 8: Reset orchestration, job, schedule, CLI (R547, R548, R553)

**Files:**
- Modify: `internal/seed/demo.go` (`Demo` = sites → people → series → money → content; `ResetDemo`), `internal/job/demo.go` (`TypeDemoReset`, `NewDemoResetTask`, `DemoResetter`), `internal/job/task.go` (handler registration), `internal/scheduler/scheduler.go` (entry), `internal/platform/config/{config.go,load.go}` (`Schedule.DemoReset`, env `EKOKOD_SCHEDULE_DEMO_RESET`, default `0 * * * *`), `internal/worker/wiring.go` (`DemoJob` gets `DemoDeps` + lock), `internal/cli/seed.go` (`--reset`, `--password`), `.env.example`/docs listing schedule envs
- Test: `internal/seed/demo_reset_integration_test.go`, `internal/scheduler/scheduler_integration_test.go`, `internal/platform/config/config_test.go`, `internal/cli/seed_test.go` if present

**Interfaces:**
- Produces: `func ResetDemo(ctx context.Context, d DemoDeps, lock Locker, now time.Time) (ResetResult, error)` where `type Locker interface{ TryLock(ctx, key string, ttl time.Duration) (unlock func(), ok bool, err error) }` (match the existing Redis lock helper's shape; use it directly if one exists); `ResetResult{Purged admin.PurgeResult; Readings int}`. `ResetDemo` = lock → `admin.DemoRepository.Purge(DemoCompanyID, DemoBaseline().Keep())` → bounded aggregate refresh over `ReadingRange`/`PlantRange` → `d.Storage` removes `Files` → generator. A missing demo company is a no-op.
- `seed demo --reset` calls `ResetDemo`; `--password X` sets the demo password (policy-checked) on an existing demo.

- [ ] **Step 1: Failing tests** (Review Focus 1–3):
  - `TestResetRestoresDeletedBaseline`: Demo seeded; via repositories soft-delete `Ostim Fabrika`, rename `DEMO-1002`, delete a tariff → `ResetDemo` → baseline restored, readings count per analyzer unchanged, bills count back to 108.
  - `TestResetRemovesVisitorRows`: add a building, an analyzer with 10 readings, an iSolar credential, a stored file on disk, a visitor user with a session → reset → all gone, file missing on disk, `consumption_daily` has no row for the visitor analyzer.
  - `TestResetLeavesOtherCompaniesAlone`: a populated second company; `md5(string_agg(t::text, ',' order by ...))` per table equal before/after.
  - `TestResetKeepsDemoSessions`: a demo session survives.
  - `TestResetSkipsWhenLocked`: lock held → returns without purging.
  - Scheduler test lists `demo.reset` at `0 * * * *`; config test reads the env.
- [ ] **Step 2–4:** FAIL → implement → gate `./internal/seed/... ./internal/job/... ./internal/scheduler/... ./internal/platform/config/... ./internal/cli/... ./internal/worker/...` `-tags=integration`.
- [ ] **Step 5:** Measure `ResetDemo` wall time on the test DB and write it in the commit body. Commit `feat(f29): hourly demo reset (R547, R548, R553)`.

### Task 9: API field and web banner (R552)

**Files:**
- Modify (admin chip, spec §1): the platform-admin company DTO gains `is_demo`; the admin companies table shows a `Demo` chip.
- Modify: `internal/api/v1/dto/auth.go` (`Me.Demo *DemoInfo`; `DemoInfo{ResetsAt time.Time json:"resets_at" required:"true"}`), the handler that builds `Me` (fills it when the company `IsDemo`: next full hour after `clock.Now()`), `internal/api/v1/openapi.json` (`make openapi`), `docs/api/reference.md` (`make api-docs`), `web/src/lib/api/schema.d.ts` (`pnpm gen:api`)
- Create: `web/src/components/shell/demo-banner.tsx`, `web/src/components/shell/demo-banner.test.tsx`, `web/e2e/demo.spec.ts`
- Modify: `web/src/components/shell/app-shell.tsx` (banner above content), `web/src/app/modules/page.tsx` (banner), `web/src/features/settings/settings-page.tsx` (caption under the read-only profile), `web/src/messages/{tr,en}/*.json` (keys `demo.banner.title`, `demo.banner.body`, `demo.banner.next`, `settings.profile.demoLocked`), the e2e seed so the Playwright stack has the demo company (`ekokod seed demo` in its setup)

**Interfaces:** Produces `DemoBanner({ resetsAt }: { resetsAt: string })` — renders nothing when `resetsAt` is absent; time shown as `HH:mm` in Europe/Istanbul via the app's date helpers; `role="status"`; uses existing theme tokens (warning surface), no dismiss.

- [ ] **Step 1: Failing tests.** Go: `TestMeCarriesDemoResetTime` (fake clock 14:20 Istanbul → `resets_at` 15:00 Istanbul; normal company → `demo` null). Web: `demo-banner.test.tsx` (renders the three tr strings with `15:00`; renders nothing without `resetsAt`). e2e `demo.spec.ts`: log in as demo → banner visible on `/ekorm` and `/ekocm`; create an alarm through the UI → success toast; settings profile shows the lock caption; sidebar shows `Finansal Analiz` and `GES Santralleri`.
- [ ] **Step 2–4:** FAIL → implement → gates: `./internal/api/v1/...` `-tags=integration`, `make check-api-docs`, `pnpm lint && pnpm typecheck && pnpm test --maxWorkers=2`, i18n parity test, `NODE_OPTIONS=--max-old-space-size=4096 pnpm build && pnpm check:module-chunks`, e2e `demo.spec.ts` in the foreground.
- [ ] **Step 5:** Commit `feat(f29): demo banner and me.demo (R552)`.

### Task 10: Phase end

- [ ] Local smoke: scratch DB in `ekokod-test-pg`, `ekokod migrate up`, `ekokod seed`, `EKOKOD_DEMO_PASSWORD=… ekokod seed demo`, `ekokod api` + `next dev -p 13100`; log in as demo and open every EKORM/EKOCM page with a Playwright script; any empty page or console error is a bug to fix before closing. Then `ekokod seed demo --reset` and re-check.
- [ ] Full gates package by package (sequential), `internal/arch`, `make lint`, `make check-generate`, `make check-api-docs`, web lint/typecheck/vitest/build, e2e in foreground chunks.
- [ ] Whole-phase self-review (tenancy and containment first): grep for any new outbound path (`http.`, `smtp`, `Dial`) reachable with a demo company id.
- [ ] Server script `/home/personal/deploy-demo.sh` (outside the repo, like the other deploy scripts): after a `deploy-main.sh` deploy, generates a password, runs `docker compose -p ekokod exec -T api ekokod seed demo` with `EKOKOD_DEMO_PASSWORD`, writes `/root/ekokod-new-demo.txt` (600) with URL, e-mail and password. Running it is the owner's call.
- [ ] Rewrite `HANDOFF_NEXT_SESSION.md`, update memory, give the owner the Turkish summary including how to read the login (`ssh.exe root@ekokod.com cat /root/ekokod-new-demo.txt`).

## Execution record (2026-10-28/29, from the git-ignored ledger)

Rulings made while executing, each with its cost if wrong:

- Task 2: Ruling: DELETE /users/{demo user} by the demo login answers 409 cannot_modify_self (existing guard runs before R542's role guard) — still refused — cost if wrong: none, only the status differs
- Task 2: Ruling: seed.Demo now tags the company is_demo (R540) — T2's HTTP tests need the tag before T5 rewrites the generator — cost if wrong: none
- Task 2: Ruling: route gating gets demo via auth.WithDemo in router.go for every non-SelfService RoleGated route instead of editing ~80 role sets — one place, R150 self-service writes stay closed — cost if wrong: a CA route that must exclude demo needs SelfService or an explicit opt-out
- Task 2: Ruling: profile lock caption reuses the existing 'Demo hesabı paylaşımlıdır; profil ve şifre değiştirilemez.' instead of the plan's new string — same meaning, already translated — cost if wrong: one string
- Task 3: Ruling: integration.DemoPoints dropped — a demo DiscoverMeteringPoints returns no points, so sync never upserts/overwrites the seeded analyzers — cost if wrong: none
- Task 3: Ruling: guards take a plain func(ctx, uuid) (bool, error) instead of demo.Check so mail/integration never import store (cycle risk); isolar.GuardDemo also takes now() for fake token expiry — cost if wrong: none
- Task 3: Ruling: apiwire guards only the default-built adapters/clients; Options overrides are test seams and stay unwrapped; mail is wrapped always — cost if wrong: a test fake bypasses the guard (tests only)
- Task 4: Ruling: DemoKeep has no Devices list — devices of kept plants are kept, others go with their plant (device ids come from sync upserts, not the generator) — cost if wrong: a visitor-made device on a kept plant survives a reset (no UI creates one)
- Task 4: Ruling: purge SQL uses pgx.NamedArgs so each statement binds only what it references; DemoTables gains kinds 'series' and 'record' (audit_log) — cost if wrong: none
- Task 5: Ruling: RED not observed before implementation (tests only failed to compile) — replaced by two mutation proofs: Restore removed → TestEnsureDemoSitesConverges fails (pkey conflict); netting disabled → TestDemoSeriesFillsWindow fails after tightening the export assertion to a weekend-noon export step — cost if wrong: none, both assertions proven.
- Task 5: Ruling: TestDemoPeopleCannotLogIn not added in seed — Task 2's TestLoginRefusesVisitorUsersOfDemoCompany covers every non-demo user of a demo company; seed test asserts 5 live users and the demo role — cost if wrong: none.
- Task 5: Ruling: credentials only for definitions the platform already has (OSOS Baskent / GridBox when a platform admin created them; iSolar EU + PM5340 from the catalogue); the demo never creates a definition — cost if wrong: the Integrations tab shows 2 credentials on a server without OSOS/GridBox definitions.
- Task 5: Ruling: "last sync −20 m" is set on the iSolar plant only (SetSyncState); meter credentials have no sync column — cost if wrong: none visible.
- Task 5: Ruling: a demo-specific demoMeter generator (export register, rooftop netting, per-analyzer curves, holidays) instead of widening fillHourly's readingProfile — e2e fixtures stay byte-identical — cost if wrong: two generators to maintain.
- Task 5: Ruling: forward-only fill (no backward fill): R550's new analyzer ids start empty — cost if wrong: none.
- Task 5: Ruling: compressor inductive share 26–34 % only Jun–Aug (10–16 % otherwise) instead of the plan's constant 28–36 % — the spec says "occasionally over the reactive limit" — cost if wrong: reactive penalty shows only in summer bills.
- Task 5: Ruling: plant views refresh over whole Istanbul months padded by a day — the monthly bucket refuses a smaller window — cost if wrong: each hourly extension recomputes the current month of the plant views (few plants).
- Task 5: Ruling: admin.DemoRepository.Restore undeletes the kept rows (repositories never see deleted rows); the generator then overwrites them through the scoped repos — cost if wrong: none.
- Task 5: Ruling: /tariffs/applicable dropped from TestDemoSeesOnlySyntheticData until Task 6 re-adds tariffs — cost if wrong: Task 6 must restore it.
- Task 5: Ruling: an old pre-F29 demo makes demo.extend fail hourly (new analyzers absent) until `seed demo` runs; the deploy script runs it right after deploy — cost if wrong: one failing job per hour until then.
- Task 6: Ruling: demo readings carry cumulative T1/T2/T3 import registers (06–17 / 17–22 / 22–06) — the engine refuses a multi-time tariff without band registers, and real OSOS meters report them — cost if wrong: none.
- Task 6: Ruling: aggregate refreshes run in Istanbul-midnight-aligned windows of ≤31 days (plant views: one padded month per window) — a single 400-day refresh hit the statement timeout under load and would on the server's first seed — cost if wrong: a few more refresh statements.
- Task 6: Ruling: Gebze's distribution price 2.2850 and the factory's "Sayaç okuma bedeli" 125 TL extra charge are my values (the plan named none) — cost if wrong: cosmetic.
- Task 6: Ruling: the icmal import mirrors the factory's 12 engine-computed building bills (rows + a stable analysis) — cost if wrong: cosmetic.
- Task 6: Ruling: /tariffs/applicable restored in TestDemoSeesOnlySyntheticData (closes the Task 5 ruling).
- Task 7: Ruling: all three demo documents are ISO 50001 files (5.2, 6.3, 9.1) — EKOCM has no document routes (Q-F9), its documents page is an EmptyState by design — cost if wrong: one empty EKOCM page in the demo.
- Task 7: Ruling: the PDFs are rendered at seed time with fpdf + Go fonts, not committed binaries — cost if wrong: none.
- Task 7: Ruling: "Fatura tutarı > 450000 TL" became invoice_increase 15% and "günlük tüketim" became reactive_limit ActiveConsumptionMax 900 kWh / 24 h — the alarm types are percent- and register-based, there is no absolute bill threshold — cost if wrong: alarm names differ from the plan.
- Task 7: Ruling: no acknowledge concept exists, so the 40 events are a delivered/failed mix (2 failed) — cost if wrong: none.
- Task 7: Ruling: reports reuse seed's reportService(pool, now) instead of a DemoDeps.Reports field — cost if wrong: none.
- Task 7: Ruling: forecasts share one generated_at (today's Istanbul midnight, or the stored run's when rows exist) so LatestRun shows one run with a fresh "generated" date — a constant 2025 date would show in the UI's model line — cost if wrong: a seed rerun across midnight without a reset keeps the older run date.
- Task 7: Ruling: automated carbon rows are monthly for closed months and daily for this month, written directly (carbon.Accrue iterates every company) — cost if wrong: on the 1st, the real daily accrual adds last month's final day once until the next hourly reset.
- Task 7: Ruling: anomalies are 4 open + 2 resolved "accepted" on DEMO-1001/1002 with the R60 detail; the ISO project's savings target lives in a note (no target field) — cost if wrong: cosmetic.
- Task 7: Ruling: Demo() refuses an empty DemoDeps.Storage.Root — files would otherwise land in the working directory — cost if wrong: none.
- Task 8: Ruling: ResetDemo uses the existing lock.Locker (Acquire under a 1 s deadline = try-lock) instead of a new TryLock interface; ResetResult gains Found/Skipped — cost if wrong: none.
- Task 8: Ruling: files are removed by deleting <root>/<area>/<demo company> in every area (storage.Store.RemoveCompany), not by Purge's stored_path list — this also drops soft-deleted uploads and generated bill/report PDFs whose rows the purge removed — cost if wrong: none; another company's directory is never touched (mutation-proved).
- Task 8: Ruling: demo.extend takes the same demo:reset lock and stands aside during a reset; demo.reset has MaxRetry 0 (the next hour replaces a failed reset) — cost if wrong: a manual reset that collides with the :15 extend prints "skipped" and must be rerun.
- Task 8: Ruling: TestResetKeepsDemoSessions folded into TestResetRemovesVisitorRows (same session setup) — saves one full demo seed in the suite — cost if wrong: none.
- Task 8: Ruling: `--password` exists as R553 says, but its help points to EKOKOD_DEMO_PASSWORD, which the deploy script uses (argv is visible in the process list) — cost if wrong: none.
- Task 9: Ruling: the banner keys live in the shell namespace (shell.demo.banner.*) — no new namespace file for three strings — cost if wrong: a key rename.
- Task 9: Ruling: the profile lock caption already existed since F6b (settings.account.demoReadOnly); reused it and widened the copy to "profil, şifre ve oturumlar" instead of adding settings.profile.demoLocked — cost if wrong: none.
- Task 9: Ruling: DemoBanner draws its own warning surface with role="status" instead of <Alert tone="warning"> (which is role="alert" and would interrupt a screen reader on every page) — cost if wrong: none.
- Task 9: Ruling: resets_at is the next full hour, matching the default EKOKOD_SCHEDULE_DEMO_RESET; a custom schedule makes the banner time approximate — cost if wrong: a misleading time only on a non-default schedule.
- Task 9: Ruling: dto.Company gains is_demo for every reader of the company DTO, not only the platform list — one shared DTO, the flag is not sensitive — cost if wrong: none.
- Task 9: Ruling: the e2e spec lives at web/tests/e2e/demo.spec.ts (the repo's e2e dir), not web/e2e/; auth.spec's demo row now expects the company-admin navigation (R541 made demo's permissions identical) and dashboard.spec's demo building is "Ostim Fabrika" — both stale since Tasks 3/5 — cost if wrong: none.
- Task 10: Ruling: demo writes company-scope bills (108 → 120) — the EKORM dashboard reads /bills/latest?scope=company and the real dispatcher writes them — cost if wrong: 12 extra demo rows.
- Task 10: Ruling: one GHG report per demo building — EKOCM lists reports for the selected building only — cost if wrong: two extra demo reports.
- Task 10: Ruling: added carbon.mains.equivalence (tr "Eşdeğerlik", en "Equivalence") — platform equivalence factors broke every company's emission-factors page, not only the demo's — cost if wrong: none.
- Task 10: Ruling: dashboard YoY pairs last year by calendar month (was by index) — pre-existing bug the demo's mid-year history exposed; affects every customer — cost if wrong: none, test pins it.
- Task 10: Ruling: demo analyzers' customer_name = their own name — the UI labels analyzers by customer_name, so six identical titles were indistinguishable — cost if wrong: the analyzers tab's "Müşteri adı" column shows meter names.
- Task 10: Ruling: demo office tariff = EPDK 2025 commercial LV 2.8731 with overuse 3.4547 — without an overuse price every office/company bill was flagged "İncelenmeli" — cost if wrong: office bills price differently than before (they were never issued).
- Task 10: Ruling: Soğuk oda reports a zero export register (Gebze's netting tariff covers the building) — a nil register flagged every Gebze/company bill, marked every Gebze consumption row "Eksik veri" and blanked the renewable table — cost if wrong: Financial Analysis now counts Gebze's billed export as unpriced production ("gelir eksik" note, small double count with the plant's own production) — an F27 model gap left for the user.
- Task 10: Ruling: settings.spec demo row → company admin's tabs — R541 gives the demo company_admin permissions; the row still pinned the old read-only demo — cost if wrong: none.
- Task 10: Ruling: the smoke ran on the e2e stack (scripts/e2e-api.sh: fresh DB, migrate, seed, seed demo, api + next start) with a throwaway Playwright spec instead of a hand-built scratch DB + next dev — same steps, production build, repeatable — cost if wrong: none.
- Task 10: Ruling: deploy-demo.sh runs `seed demo` in a one-off worker container (`run --rm --no-deps`) instead of `exec api` — the 400-day first seed stays out of the live api's 1 GB limit — and keeps the existing password unless `--rotate` (re-setting the same one fails R146's reuse check; rotating locks out prospects holding the old one) — cost if wrong: one flag to remember.
- Task 10: Ruling: deploy-demo.sh also runs `seed demo --reset` once — an older pre-F29 demo has no is_demo tag, so its leftovers would otherwise wait for the next HH:00 — cost if wrong: ~20 s.

Deferred minors:

- Final: minor (deferred): AI Analysis anomaly tab defaults to today + the previous hour, which is "the future" between 00:00 and 01:00 Istanbul (predict.spec:28 fails in that window).
- Final: minor (deferred): Financial Analysis counts a netted building's billed export as unpriced production on top of the plant's own production (F27 model; visible on demo Gebze as "gelir eksik").

- Final review: self-review (no subagent tool) — containment: every SMTP sender (api 1, worker 3) and every meter/iSolar adapter (api + worker) is wrapped; SMTP routes are platform-admin only (WithDemo adds demo only where CA is); password-reset mail uses the user's own company SMTP (no platform fallback); public/contact forms go only to ekokod's fixed inbox; ML and weather are not integration settings. No Critical/Important finding.
