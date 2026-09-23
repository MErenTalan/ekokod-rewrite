# F8a — Bills and Tariffs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this
> plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This phase runs **inline in
> one session** — no parallel agents, no subagents, no Workflow (user's standing rule).

**Goal:** The billing and tariff experience of 01 §7.10 and §7.11: a month's invoice dashboard whose
totals are the rows it shows, ad-hoc invoice generation that says exactly why it could not bill,
per-analyzer/building/company PDF and hourly PTF Excel downloads, and the full tariff surface —
create/edit with PTF+YEKDEM and KBK, version history, templates, bulk assignment with its history,
the icmal import with its review-and-confirm step, solar tariffs and the admin default-tariff
catalogue.

**Architecture:** Almost all of the billing and tariff *domain* already exists (F4): `internal/domain/
billing` computes invoices, `internal/domain/tariff` validates definitions and prices them,
`internal/domain/tariff/icmal` derives KBK coefficients, and `internal/service/tariff` already owns
templates, bulk assignment and the three-step icmal flow. F8a therefore adds **one new pure
aggregate** (`internal/domain/billing/dashboard.go` — bill rows in, dashboard out, no I/O), **one new
service read** (`internal/service/billing/dashboard.go`), **two renderers** (dashboard XLSX and PDF),
**seventeen HTTP routes** over services that already exist, **two small migrations**
(`job_runs.task_id`, `tariff_bulk_assignments`), and **two screens** following the conventions F6a/
F6b/F7 fixed (R195–R199, R229): route → `*-page` container → views, `useApiMutation` for writes,
`downloadFile` for exports, `mockApi` for container tests, data-free stories.

**Tech Stack:** Go 1.27 (asynq, pgx/sqlc, shopspring/decimal, swaggest OpenAPI, excelize, gofpdf),
Postgres 17 + TimescaleDB, Redis, Next.js 15 App Router + React 19, TanStack Query via
`openapi-react-query`, Tailwind v4, Vitest + Storybook + Playwright.

**Spec:**
- `docs/rewrite/09-implementation-plan.md` §F8 — scope, acceptance criteria, verification
- `docs/rewrite/01-project-context.md` §7.10 (Bills), §7.11 (Tariffs)
- `docs/rewrite/02-domain-rules.md` §4 (tariff resolution), §6 (bill computation, especially §6.10
  invoice output and §6.11 building/company invoices), §7 (PTF+YEKDEM), §8 (icmal derivation)
- `docs/rewrite/05-api-contract.md` §1 (conventions), §6 (tariffs + icmal), §7 (billing)
- `docs/rewrite/04-data-model.md` §6 (tariffs), §9 (bills), §11 (operations) — migrated in F1
- `docs/rewrite/07-design-system.md` (§7 shell, §8 components, §11 accessibility checklist), then
  `design-system/bcem-energy/MASTER.md`, then `design-system/bcem-energy/OVERRIDES.md` (binding)
- Predecessor plans: `2026-09-17-f4-billing-engine.md` (R105–R135), `2026-09-17-f6a-auth-api-skeleton.md`
  (R136–R189 + the permission matrix), `2026-09-17-f6b-core-screens.md` (R190–R210),
  `2026-09-18-f7-alarms-messages.md` (R211–R232)

---

## Global Constraints

- **Branch/worktree:** `phase/f8a-bills-tariffs` in `/home/personal/ekokod-f8-phase`, branched from
  `phase/f7-alarms`. Nothing is pushed; `main` is untouched; merging is the user's call.
- **This session runs as the `s2personal` user, whose `$HOME` has no toolchain.** Prefix every
  command with
  `export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
  (The F7 handoff's `$HOME/.local/...` form resolves to nothing here.)
- **Never run `make test` or `go test ./...`.** Run only the packages a task touches; the per-task
  gate is the **whole touched package**, not a `-run` filter (targeted filters missed four
  regressions in F6a).
- **Integration tests** need `ekokod-test-pg` and `ekokod-test-redis`. `docker ps` first; if Docker
  is unavailable (it was at the start of this session), tell the user and keep going with the unit
  and web work — never claim an integration gate that did not run. When the containers are up:
  `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <pkg> -tags=integration -count=1 -parallel 4`.
- Test databases have **no continuous-aggregate refresh policies**; a test needing materialised data
  refreshes explicitly. `internal/store/postgres` is one of this phase's integration packages —
  run it (F6b's handoff forgot to, and two migration tests were red for a phase).
- **Web:** `pnpm test --maxWorkers=2`; Storybook, Playwright and `next build` one at a time, never
  concurrently. Check `free -m` and `ps -eo rss,comm | grep java` before heavy work; Playwright
  `--workers=1` when memory is tight. e2e runs with `E2E_API_PORT=18082` — **18080 is dolmusum's**.
  Never leave `next dev`/`next start`, `storybook dev`, `vitest --watch`, a static server or
  `bin/ekokod-e2e` running.
- **Do not touch** another project's Gradle/java/vitest processes, `dolmusum-dev-postgres-1` on
  5432, or dolmusum's ports.
- **Money and energy cross the API as strings** (05 §1); `dto.Decimal` refuses JSON numbers.
  Timestamps render in Europe/Istanbul (R161). Dates on the wire are ISO `YYYY-MM-DD`; the screens
  may *render* `DD-MM-YYYY` (§7.11) but never send it.
- **404, never 403, for anything outside the caller's scope** (05 §1): one indistinguishable answer
  for unknown, foreign-company and out-of-scope ids.
- **Message keys are camelCase** (`pnpm check:i18n-parity` rejects anything else).
- Read `design-system/bcem-energy/MASTER.md`, then `OVERRIDES.md` before writing any UI: OVERRIDES
  and 07 win wherever MASTER disagrees (flat surfaces, no hover lift, no GSAP, emerald tokens,
  Lexend/Source Sans 3/JetBrains Mono).
- **Comments say WHY in 1–3 lines.** Commit as soon as tests are green, then prove the guards red
  with your own mutation, then the next task.
- **Prove every guard red by mutation, restoring from a file backup** — never `git checkout` a file
  whose fix is not committed yet.
- Commit trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.
- Every new screen needs a line in `web/tests/e2e/responsive.spec.ts`'s `SCREENS` list.
- `make check-generate` needs a clean tree for `internal/store/postgres/sqlcgen` — commit first.
  `make openapi` regenerates both the Go document and `web/src/lib/api/schema.d.ts`.
- **Test helper and seed constant names in this plan are indicative, not authoritative.** Every Go test here is
  written against the fixture and harness helpers the file it joins already uses — `newHarness`,
  `h.Get`/`h.Post`, the `roleCompanyAdmin`-style constants, `billingFixture`, `ptr`. Read the file
  before appending; add a helper only where none exists, once, in the file that needs it.

---

## Product-owner decisions taken on 2026-09-23

Asked before planning, with the evidence that prompted each question.

| # | Question | Decision |
|---|---|---|
| D-1 | **`GET /bills/dashboard`** — deferred to this phase by the user on 2026-09-17; where do the totals come from? Legacy computes them in the browser from `billHistory` maps embedded on the analyzer and building documents (`BillsTab.tsx:530-560`), which the rewrite does not have. | **Open the endpoint in F8a and aggregate server-side from the `bills` table.** Rows are the period's analyzer bills; totals are the sums of the rows shown, proved by a test. The solar-plants section has no data source until F9 and renders an explicit unavailable state rather than an invented number (legacy's own plants section is a design-only placeholder — `BillsTab.tsx:892`: *"GES SANTRALLERİ — placeholder (sadece tasarım) / Henüz veri yok"*). |
| D-2 | **Report PDF/Excel and e-mail** — 09 §F8 wants them; `internal/render` knows only the invoice PDF and the hourly Excel. | **Full scope, but in F8b.** Reports (01 §7.14, 05 §11, `report.generate`/`report.deliver`) leave F8a entirely. |
| D-3 | **Phase size** — 24 missing endpoints and three large screens; legacy's `TariffsTab.tsx` alone is 3 529 lines. | **Split.** F8a = bills + tariffs (this plan). F8b = reports. Same rhythm as F6a/F6b. |
| D-4 | Which spec items with **no API and no data model** are in scope? | **All four asked for are in F8a:** bulk-assignment history (new table), the admin default-tariff tab, `GET /bills/dashboard/export`, and the solar-tariffs tab. |

---

## Binding rulings (R233–R252)

Numbering continues from F7's R232.

| # | Ruling |
|---|---|
| **R233** | **The dashboard has exactly one data source: the `bills` table.** `GET /bills/dashboard?year&month` reads the period's non-superseded bills and derives every figure from them — consumption, prices, charges, production and netting. No consumption-series or generation-series query joins in: a bill's period follows the building's cutoff day (`Bill.PeriodStart/PeriodEnd`), so a calendar-month series would silently disagree with the invoice it sits next to. |
| **R234** | **A building total is the sum of its analyzer rows; a building *invoice* is not.** 02 §6.11 is explicit that a building bill applies the tariff once to the aggregate, so it may legitimately differ from the sum of the analyzer bills. The dashboard shows the analyzer rows and their sum as the building's total row (this is what "totals reconcile with the sum of its rows" is tested against), and shows the building-scope bill — when one exists — as its own labelled figure with its own download. A difference above ₺0.01 is displayed as an explicit note, never silently reconciled and never hidden. |
| **R235** | **The solar-plants section is unavailable before F9, and says so.** `power_plants` has no analyzer link and no production of its own in the schema; the only real generation today belongs to building analyzers. The endpoint returns `"plants": {"available": false, "reason": "no_plant_production_source"}` and the screen renders R165's explicit unavailable state naming F9. No fabricated plant row, no zero-filled table. |
| **R236** | **Netting is arithmetic over the same rows.** `total_consumption = Σ active_import`, `total_production = Σ active_export`, `net = consumption − production`, `status = net_consumption` when `net ≥ 0` else `net_production`, `total_invoice = Σ total_cost`. `efficiency_pct = production / consumption × 100` rounded to 2 dp and **omitted** (not zero) when consumption is zero. The period shown is the bills' own `period_key`. |
| **R237** | **`GET /jobs/{id}` gains `error_code`**, drawn from the closed R113 code set (`tariff_not_found`, `no_consumption_data`, `unresolved_anomaly`, `period_not_closed`, `billing_parameters_missing`, `billing_parameters_invalid`) and from nowhere else. R192 still holds: the worker's own error text never leaves the process. The link is a new `job_runs.task_id` column (migration 00016) written by the billing generator; `billing.generate` joins `jobs.watchable`, scoped by the payload's company and subject. |
| **R238** | **Ad-hoc generation is compute → watch → download.** `POST /bills/compute` (F4) returns job ids; the screen polls `GET /jobs/{id}`; on success it reads `GET /bills?scope&…&period` for the id and downloads `GET /bills/{id}/pdf`. Every §7.10 validation message is reachable: the four selection messages are client-side guards, the data messages are `error_code` values (R237), and "lack of permission to download" is a hidden action plus the API's own 404/403. |
| **R239** | **The dashboard export renders the same aggregate the screen shows.** `GET /bills/dashboard/export?year&month&format=xlsx|pdf` calls the same service method as the screen and renders its result, so the file and the page cannot disagree. Default `format=xlsx`. |
| **R240** | **The tariff endpoints are thin HTTP over the F4 service.** Templates, bulk assignment and the icmal flow already exist in `internal/service/tariff`; handlers map DTO ↔ service types and nothing else. No validation, derivation or copying logic is duplicated in `internal/api`. |
| **R241** | **Bulk-assignment history is recorded, not inferred.** Migration 00017 adds `tariff_bulk_assignments` (company, optional template, tariff name, effective_from, the building ids that actually received a version, created_by, created_at). `BulkAssign` and `ApplyTemplate` record one row **after** the assignment, listing only the buildings that succeeded — a partial failure records what happened, never what was requested. |
| **R242** | **Legacy's "clear tariff history" is not rebuilt.** The legacy endpoint of the same name (`/api/buildings/bulk-tariff/history`) is a `DELETE` that truncates a building's tariff list to its active version. Every issued bill references `tariff_id` + `tariff_effective_from`; deleting versions destroys the evidence an invoice is defended with. `GET /buildings/bulk-tariff/history` in 05 §6 is the assignment-history view of R241. Recorded as a deviation with an open question. |
| **R243** | **Admin default tariffs are the `national_tariff_schedule` table.** The legacy "Default tariffs (admin only)" tab edits a platform-level catalogue with no company; the rewrite already has exactly that table (00006), already seeded, and already destined for F12's public calculator. Admin-only write routes are added (`POST`/`PATCH`/`DELETE`); `GET /national-tariff-schedule` stays public as 05 §6 lists it. Legacy's `energy_source=solar` variant of the tab has no target table and is **not** built: solar prices are per plant (R244). |
| **R244** | **Solar tariffs are plant-scoped and soft-deleted.** `GET/POST /solar-tariffs`, `DELETE /solar-tariffs/{id}` over the F1 repository; read A CA CR, write A CA. `effective_from` is ISO on the wire; the screen renders `DD-MM-YYYY` (§7.11) through `formatDate`. A plant outside the caller's scope answers 404. |
| **R245** | **The icmal writes nothing without a per-building confirmation.** `POST /icmal-imports` analyses and stores; `POST /icmal-imports/{id}/apply` takes an explicit `confirmations[]` and writes one new tariff version per confirmed building (02 §8.4, R134). An empty list is 422 `no_confirmations`. The screen is a three-step stepper — upload → review → confirm — whose confirm action is disabled until at least one building is checked, and every coefficient renders its value, sample count, stability and warnings (§7.11). |
| **R246** | **Eight new permissions** extend R159's table: `bills.read`, `tariffs.read`, `tariffs.edit`, `tariffs.templates.read`, `tariffs.bulk.read`, `tariffs.icmal`, `tariffs.defaults`, `solar_tariffs.read`. Writes to templates, bulk assignment and solar tariffs ride on `tariffs.edit`; the rule table below is authoritative. |
| **R247** | **Read-only roles see no write action and demo refuses every mutation** (R150/R232's shape): the tariff form is disabled for `*-readonly-admin`, the ad-hoc generation card is hidden without `bills.compute`, and the demo role's mutations are refused by the API as they already are elsewhere. |
| **R248** | **Decimals are strings end to end.** The tariff form holds every price as a string and sends it as one; no `parseFloat` touches a price in the web layer (`lib/decimal.ts` is the only converter, and only for display). A blank optional price is omitted from the body, never sent as `"0"`. |
| **R249** | **The tariff form is one form with mode switches**, as legacy had it: the PTF+YEKDEM toggle reveals the KBK fieldset and hides the fixed-price fields, `term=binomial` reveals contracted power and the power price, `price_type=multi_time` swaps the single price for T1/T2/T3. Switching a mode clears the fields of the abandoned mode from the draft, so a fixed T1 price can never be submitted on a PTF tariff. Every server field code from `tariff.Validate` maps to a message on that field. |
| **R250** | **The "bill listing table" and the dashboard's buildings section are one table.** §7.10 describes them separately, but their columns are the same set and legacy renders exactly one table (`BillsTab.tsx:705-890`: date, building, analyzer, installation no, ETSO, consumption, price, invoice, download). The rewrite ships one table with a month total row, which is what both descriptions ask for. |
| **R251** | **One month control, not two.** §7.10 asks for a year selector and a month selector; the design system's `month-picker` carries the year inside it, and legacy's year selector is commented out in its own source. One `MonthPicker` is the whole selection; `year` and `month` still cross the API separately as 05 §7 specifies. |
| **R252** | **A superseded bill stays downloadable.** Recomputation supersedes (F4); the dashboard shows the live bill, and `GET /bills/{id}/pdf` on the superseded id still renders — proved by an integration test in this phase, because the dashboard is what makes a recomputation visible to a user. |

---

## Rule table — permissions (extends R159)

| Permission | A | CA | CR | BA | BR | Why |
|---|---|---|---|---|---|---|
| `bills.read` | ✓ | ✓ | ✓ | ✓ | ✓ | 05 §7 lists every bill read as `scope`; the scope filter does the narrowing |
| `bills.compute` (F4) | ✓ | ✓ | — | ✓ | — | unchanged |
| `tariffs.read` | ✓ | ✓ | ✓ | ✓ | ✓ | 05 §6 `scope` |
| `tariffs.edit` | ✓ | ✓ | — | — | — | 05 §6 write rows are A CA; covers tariffs, templates, bulk assignment and solar tariffs |
| `tariffs.templates.read` | ✓ | ✓ | ✓ | — | — | 05 §6 `GET /tariff-templates` is A CA CR |
| `tariffs.bulk.read` | ✓ | ✓ | ✓ | — | — | 05 §6 `/buildings/bulk-tariff/current` and `/history` are A CA CR |
| `tariffs.icmal` | ✓ | ✓ | — | — | — | 05 §6 icmal rows are A CA |
| `tariffs.defaults` | ✓ | — | — | — | — | R243, platform catalogue |
| `solar_tariffs.read` | ✓ | ✓ | ✓ | — | — | 05 §6 `GET /solar-tariffs` is A CA CR |

The demo role holds the read permissions and no write one (R232's shape).

## Rule table — routes (05 §6 and §7)

| # | Method | Pattern | Roles | Idempotency | Notes |
|---|---|---|---|---|---|
| 1 | GET | `/bills/dashboard` | scope | — | `?year&month`; R233–R236 |
| 2 | GET | `/bills/dashboard/export` | scope | — | `?year&month&format=xlsx\|pdf`; R239; raw file |
| 3 | GET | `/tariff-templates` | A CA CR | — | `?limit&offset` |
| 4 | POST | `/tariff-templates` | A CA | yes | body = name, description, is_default, tariff fields |
| 5 | PATCH | `/tariff-templates/{id}` | A CA | yes | full replace |
| 6 | DELETE | `/tariff-templates/{id}` | A CA | yes | hard delete (no invoice references a template) |
| 7 | POST | `/tariff-templates/{id}/apply` | A CA | yes | `building_ids[]`, `effective_from`; records R241 |
| 8 | GET | `/buildings/bulk-tariff/current` | A CA CR | — | every building's tariff in force today |
| 9 | POST | `/buildings/bulk-tariff` | A CA | yes | `building_ids[]` + tariff fields; records R241 |
| 10 | GET | `/buildings/bulk-tariff/history` | A CA CR | — | R241's table, newest first |
| 11 | GET | `/solar-tariffs` | A CA CR | — | `?plant_id` |
| 12 | POST | `/solar-tariffs` | A CA | yes | R244 |
| 13 | DELETE | `/solar-tariffs/{id}` | A CA | yes | soft delete |
| 14 | POST | `/icmal-imports` | A CA | no (`NoIdempotency`, multipart) | `multipart/form-data`, field `file`, ≤ 10 MiB |
| 15 | GET | `/icmal-imports/{id}` | A CA | — | the stored analysis |
| 16 | POST | `/icmal-imports/{id}/apply` | A CA | yes | `confirmations[]`; R245 |
| 17 | GET | `/national-tariff-schedule` | public | — | already in 05 §6; read-only, no scope |
| 18 | POST | `/national-tariff-schedule` | A | yes | R243 |
| 19 | PATCH | `/national-tariff-schedule/{id}` | A | yes | R243 |
| 20 | DELETE | `/national-tariff-schedule/{id}` | A | yes | R243 |

Rows 17–20 are platform routes: `PlatformAudit: true`, no company scope.

---

## File map

**Go — new**

| File | Responsibility |
|---|---|
| `internal/domain/billing/dashboard.go` | `Dashboard(rows []Row) Result` — the pure aggregate: building grouping, totals, netting (R233–R236). No I/O, no clock |
| `internal/domain/billing/dashboard_test.go` | reconciliation table tests, including the R234 divergence and the zero-consumption efficiency case |
| `internal/service/billing/dashboard.go` | loads the period's bills, buildings and analyzers for a scope and feeds the pure aggregate |
| `internal/render/dashboardxlsx/xlsx.go` | the dashboard as a workbook (buildings sheet + summary sheet) |
| `internal/render/dashboardpdf/pdf.go` | the dashboard as one PDF, over `internal/render/table` |
| `internal/service/tariff/solar.go` | solar tariff CRUD over the F1 repository (R244) |
| `internal/service/tariff/history.go` | `RecordBulkAssignment` + `BulkHistory` (R241) |
| `internal/service/tariff/defaults.go` | national-schedule list/create/update/delete for the admin tab (R243) |
| `internal/api/v1/handlers_tariffs.go` | `tariffRoutes()` rows 3–20 + handlers |
| `internal/api/v1/dto/tariffs_extra.go` | template, bulk, icmal, solar and national-schedule DTOs |
| `internal/store/postgres/migrations/00016_job_run_task_id.sql` | `job_runs.task_id` + index (R237) |
| `internal/store/postgres/migrations/00017_tariff_bulk_assignments.sql` | R241's table |

**Go — modified**

| File | Change |
|---|---|
| `internal/auth/roles.go` | eight permissions (R246) |
| `internal/api/v1/dto/auth.go` | `Permission.Enum()` gains the eight names |
| `internal/api/v1/dto/bills.go` | dashboard request/response DTOs |
| `internal/api/v1/dto/jobs.go` | `Job.ErrorCode` (R237) |
| `internal/api/v1/handlers_billing.go` | dashboard + export rows and handlers |
| `internal/api/v1/routes.go` | `Table()` concatenates `tariffRoutes()` |
| `internal/api/v1/router.go` | `Handlers` gains `Tariffs *tariff.Service` (if absent) and `Catalogue` for the admin routes |
| `internal/apiwire/wiring.go` | builds the tariff service with its new repositories |
| `internal/service/jobs/jobs.go` | `billing.generate` watchable + `ErrorCode` from `job_runs` (R237) |
| `internal/service/billing/jobs.go` | writes `task_id` on the run it starts |
| `internal/service/tariff/service.go`, `templates.go` | record bulk history (R241) |
| `internal/store/repository.go` | `OpsRepository.RunByTaskID`, `BulkAssignmentRepository`, `SolarTariff` filter use, `CatalogueRepository` list/delete |
| `internal/store/postgres/queries/{operations,tariffs,admin_catalogue}.sql` | the new queries |
| `internal/store/postgres/{ops.go,tariffs.go,admin/catalogue.go}` | the new repository methods |
| `internal/seed/fixtures.go` | e2e bills for the dashboard, a template, a bulk assignment, an icmal import |

**Web — new** (all under `web/src`)

| File | Responsibility |
|---|---|
| `app/ekorm/bills/page.tsx` | route shell → `BillsPage` |
| `app/ekorm/tariffs/page.tsx` | route shell → `TariffsPage` |
| `features/bills/bills-page.tsx` + `.test.tsx` | container: month selection, dashboard query, generation card |
| `features/bills/dashboard-table.tsx` + `.test.tsx` + `.stories.tsx` | §7.10 buildings table with totals (R250) |
| `features/bills/netting-summary.tsx` + `.test.tsx` + `.stories.tsx` | the netting cards (R236) and the plants unavailable state (R235) |
| `features/bills/generate-card.tsx` + `.test.tsx` + `.stories.tsx` | ad-hoc generation: analyzer multi-select, month, three download actions |
| `features/bills/generation-status.ts` + `.test.ts` | the compute → watch → download state machine and its message mapping (R238) |
| `features/tariffs/tariffs-page.tsx` + `.test.tsx` | container: the six tabs and their permissions |
| `features/tariffs/tariff-form.tsx` + `.test.tsx` + `.stories.tsx` | the §7.11 form with its mode switches (R249) |
| `features/tariffs/tariff-draft.ts` + `.test.ts` | draft ↔ request mapping, mode clearing, field-code → message map |
| `features/tariffs/tariff-history.tsx` + `.test.tsx` + `.stories.tsx` | the version list with edit/delete |
| `features/tariffs/templates-tab.tsx` + `.test.tsx` + `.stories.tsx` | template CRUD + apply |
| `features/tariffs/bulk-tab.tsx` + `.test.tsx` + `.stories.tsx` | current state, multi-select apply, history |
| `features/tariffs/icmal-tab.tsx` + `.test.tsx` + `.stories.tsx` | the three-step import (R245) |
| `features/tariffs/solar-tab.tsx` + `.test.tsx` + `.stories.tsx` | plant selector + history + add form (R244) |
| `features/tariffs/defaults-tab.tsx` + `.test.tsx` + `.stories.tsx` | the admin catalogue (R243) |
| `messages/tr/bills.json`, `messages/en/bills.json` | bills namespace |
| `messages/tr/tariffs.json`, `messages/en/tariffs.json` | tariffs namespace |
| `tests/e2e/bills.spec.ts`, `tests/e2e/tariffs.spec.ts` | 09 §F8's e2e verification line (the two F8a specs) |

**Web — modified:** `components/shell/nav-config.ts` (permissions on the two existing leaves),
`lib/session/permissions.fixture.json` (regenerated), `lib/api/schema.d.ts` (`make openapi`),
`tests/e2e/responsive.spec.ts` (two `SCREENS` lines), `messages/index.ts` (two namespaces).

---
## Review Focus

Five input classes 01 §7.10/§7.11 implies but never states, each pinned to the task that owns the
code. Its silence is not permission for the input to break the screen.

1. **Mixed currencies in one month.** R127 forces TRY only on PTF tariffs; a fixed tariff may be
   USD or EUR, so one company's bills can carry two currencies. Totals must never add across them.
   → Task 2 (`TestDashboardKeepsCurrenciesApart`) and R253.
2. **A bill whose building or analyzer was soft-deleted after it was issued.** The invoice still
   exists and must still be listed and downloadable, with the name it was issued under.
   → Task 2 (`TestDashboardKeepsRowsWithoutLiveParents`).
3. **A period with a company- or building-scope bill but no analyzer bills.** The table must not
   show a non-zero total over an empty body. → Task 2 (`TestDashboardTotalsFollowTheRowsShown`).
4. **An icmal upload that is not a spreadsheet, is empty, or is enormous.** A 0-byte file, a PDF
   renamed `.xlsx` and a 20 MiB workbook must each answer a specific 4xx, never a panic, a 500 or a
   hung request. → Task 7 (`TestIcmalUploadRejectsUnreadableFiles`, `TestIcmalUploadTooLarge`).
5. **A bulk assignment naming a building outside the caller's scope.** Nothing may be written for
   any building in the call, and the history must not claim the ones that would have succeeded.
   → Task 6 (`TestBulkAssignRefusesForeignBuildingAndRecordsNothing`).

---

## Task 1: Permissions and navigation gating

**Files:**
- Modify: `internal/auth/roles.go` (`permissionTable`)
- Modify: `internal/api/v1/dto/auth.go` (`Permission.Enum`)
- Modify: `web/src/components/shell/nav-config.ts` (the `billsTariffs` group's two leaves)
- Modify: `web/src/lib/session/permissions.fixture.json` (regenerated from Go)
- Test: `internal/auth/permissions_test.go`, `internal/auth/permissions_fixture_test.go`,
  `web/src/components/shell/nav-config.test.ts`

**Interfaces:**
- Consumes: `auth.Roles`, `auth.AllRoles`, `auth.PermissionsFor`, `auth.AllPermissions` (unchanged).
- Produces: the eight permission names of R246, referenced by every later route row:
  `"bills.read"`, `"tariffs.read"`, `"tariffs.edit"`, `"tariffs.templates.read"`,
  `"tariffs.bulk.read"`, `"tariffs.icmal"`, `"tariffs.defaults"`, `"solar_tariffs.read"`.

- [ ] **Step 1: Write the failing test**

Append to `internal/auth/permissions_test.go` (read the file first and reuse its existing table
shape and role constants):

```go
func TestF8aPermissionsFollowTheRuleTable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		permission string
		roles      []model.UserRole
	}{
		{"bills.read", []model.UserRole{roleA, roleCA, roleCR, roleBA, roleBR}},
		{"tariffs.read", []model.UserRole{roleA, roleCA, roleCR, roleBA, roleBR}},
		{"tariffs.edit", []model.UserRole{roleA, roleCA}},
		{"tariffs.templates.read", []model.UserRole{roleA, roleCA, roleCR}},
		{"tariffs.bulk.read", []model.UserRole{roleA, roleCA, roleCR}},
		{"tariffs.icmal", []model.UserRole{roleA, roleCA}},
		{"tariffs.defaults", []model.UserRole{roleA}},
		{"solar_tariffs.read", []model.UserRole{roleA, roleCA, roleCR}},
	} {
		t.Run(tc.permission, func(t *testing.T) {
			for _, r := range model.UserRoles() {
				want := slices.Contains(tc.roles, r)
				got := slices.Contains(auth.PermissionsFor(r), tc.permission)
				require.Equalf(t, want, got, "%s for %s", tc.permission, r)
			}
		})
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

```bash
export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB
go test ./internal/auth/... -run TestF8aPermissions -v
```

Expected: FAIL — every row reports `false` where `true` is wanted.

- [ ] **Step 3: Add the eight rows**

In `internal/auth/roles.go`, inside `permissionTable`, after the F7 block:

```go
	// F8a R246. Reads follow 05 §6/§7's `scope` marker; every tariff write —
	// versions, templates, bulk assignment, solar tariffs — rides on
	// tariffs.edit, because 05 §6 gives them all the same A CA row.
	"bills.read":             AllRoles,
	"tariffs.read":           AllRoles,
	"tariffs.edit":           Roles(roleA, roleCA),
	"tariffs.templates.read": Roles(roleA, roleCA, roleCR),
	"tariffs.bulk.read":      Roles(roleA, roleCA, roleCR),
	"tariffs.icmal":          Roles(roleA, roleCA),
	"tariffs.defaults":       Roles(roleA),
	"solar_tariffs.read":     Roles(roleA, roleCA, roleCR),
```

- [ ] **Step 4: Extend the DTO enum**

In `internal/api/v1/dto/auth.go`, add the eight names to `Permission.Enum()` in the same sorted
position the existing list uses.

- [ ] **Step 5: Run the whole auth package**

```bash
go test ./internal/auth/... -race -count=1
```

Expected: PASS, except `permissions_fixture_test.go` which now reports the web fixture is stale.

- [ ] **Step 6: Regenerate the web permission fixture**

Run the generator the fixture test names in its failure message (F6a added it; it writes
`web/src/lib/session/permissions.fixture.json`), then re-run `go test ./internal/auth/... -race`.

Expected: PASS.

- [ ] **Step 7: Gate the two nav leaves**

In `web/src/components/shell/nav-config.ts`, the `billsTariffs` group:

```ts
  { id: 'billsTariffs', labelKey: 'billsAndTariffs', icon: Receipt, children: [
    { id: 'bills', labelKey: 'bills', href: '/ekorm/bills', icon: FileText, permission: 'bills.read' },
    { id: 'tariffs', labelKey: 'tariffs', href: '/ekorm/tariffs', icon: Tags, permission: 'tariffs.read' } ] },
```

- [ ] **Step 8: Web test for the gating**

Append to `web/src/components/shell/nav-config.test.ts`, matching the file's existing helper style:

```ts
it('hides bills and tariffs from a principal without their permissions', () => {
  const entries = navFor([]);
  const ids = entries.flatMap((e) => (isGroup(e) ? e.children.map((c) => c.id) : [e.id]));
  expect(ids).not.toContain('bills');
  expect(ids).not.toContain('tariffs');
});
```

- [ ] **Step 9: Run the web suite**

```bash
cd web && pnpm test --maxWorkers=2 -- src/components/shell
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/auth internal/api/v1/dto/auth.go web/src/components/shell web/src/lib/session/permissions.fixture.json
git commit -m "feat(f8a): task 1 — eight bills/tariffs permissions and nav gating

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 11: Prove the guard red**

Back up `internal/auth/roles.go`, change `"tariffs.edit"` to `AllRoles`, re-run
`go test ./internal/auth/...` (expect FAIL naming `tariffs.edit for building-readonly-admin`),
restore from the backup, re-run (expect PASS).

---

## Task 2: `internal/domain/billing` — the dashboard aggregate

**Files:**
- Create: `internal/domain/billing/dashboard.go`
- Test: `internal/domain/billing/dashboard_test.go`

**Interfaces:**
- Consumes: `model.Bill`, `model.BillScope`, `decimal.Decimal`; the package's existing rounding
  helper conventions in `aggregate.go` (read it first — reuse, do not re-derive).
- Produces, used by Task 3 and Task 4:

```go
// DashboardRow is one analyzer's invoice as the dashboard lists it.
type DashboardRow struct {
	BillID                                  uuid.UUID
	BuildingID                              *uuid.UUID
	AnalyzerID                              *uuid.UUID
	BuildingName, AnalyzerName              string
	InstallationNumber, EtsoCode            string
	PeriodKey                               string
	Consumption                             decimal.Decimal
	ConsumptionPrice, ProductionPrice       *decimal.Decimal
	Production                              decimal.Decimal
	Invoice                                 decimal.Decimal
	Currency                                model.CurrencyCode
	Superseded                              bool
}

// DashboardBuilding is one building's rows and their sums (R234).
type DashboardBuilding struct {
	BuildingID              uuid.UUID
	BuildingName            string
	Rows                    []DashboardRow
	TotalConsumption        decimal.Decimal
	TotalProduction         decimal.Decimal
	TotalInvoice            decimal.Decimal
	Currency                model.CurrencyCode
	// BuildingBill is the building-scope invoice for the period when one
	// exists; DivergesFromRows is true when it differs from TotalInvoice by
	// more than ₺0.01 (02 §6.11 makes that legitimate, so it is shown, not
	// reconciled).
	BuildingBill     *DashboardRow
	DivergesFromRows bool
}

// DashboardNetting is R236, one per currency present (R253).
type DashboardNetting struct {
	Currency         model.CurrencyCode
	TotalConsumption decimal.Decimal
	TotalProduction  decimal.Decimal
	Net              decimal.Decimal
	NetStatus        string // "net_consumption" | "net_production"
	TotalInvoice     decimal.Decimal
	EfficiencyPct    *decimal.Decimal // nil when consumption is zero
	PeriodKey        string
	CompanyBillID    *uuid.UUID
}

// DashboardResult is the whole month.
type DashboardResult struct {
	Buildings []DashboardBuilding
	Netting   []DashboardNetting // one entry per currency present (R253)
}

// BuildDashboard groups rows by building, sums them and nets them. It is pure:
// every figure is arithmetic over rows, so the screen, the export and the test
// can never disagree.
func BuildDashboard(rows []DashboardRow, buildingBills []DashboardRow, companyBills []DashboardRow) DashboardResult
```

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/billing/dashboard_test.go` (package `billing_test`, the convention the
other tests in this package use):

```go
func TestDashboardTotalsFollowTheRowsShown(t *testing.T) {
	t.Parallel()
	b := uuid.New()
	rows := []billing.DashboardRow{
		analyzerRow(b, "A", "120.5", "310.25"),
		analyzerRow(b, "B", "80.5", "199.75"),
	}
	got := billing.BuildDashboard(rows, nil, nil)
	require.Len(t, got.Buildings, 1)
	// The acceptance criterion of 09 §F8: the total IS the sum of the rows.
	require.Equal(t, "201", got.Buildings[0].TotalConsumption.String())
	require.Equal(t, "510", got.Buildings[0].TotalInvoice.String())
	require.Len(t, got.Netting, 1)
	require.Equal(t, "510", got.Netting[0].TotalInvoice.String())
}

func TestDashboardShowsBuildingBillDivergenceInsteadOfHidingIt(t *testing.T) {
	t.Parallel()
	b := uuid.New()
	rows := []billing.DashboardRow{analyzerRow(b, "A", "100", "250"), analyzerRow(b, "B", "100", "250")}
	// 02 §6.11: the building invoice prices the aggregate once, so it may
	// legitimately differ from the sum of the analyzer invoices.
	building := []billing.DashboardRow{buildingRow(b, "200", "480")}
	got := billing.BuildDashboard(rows, building, nil)
	require.Equal(t, "500", got.Buildings[0].TotalInvoice.String())
	require.NotNil(t, got.Buildings[0].BuildingBill)
	require.Equal(t, "480", got.Buildings[0].BuildingBill.Invoice.String())
	require.True(t, got.Buildings[0].DivergesFromRows)
}

func TestDashboardKeepsCurrenciesApart(t *testing.T) {
	t.Parallel()
	try, usd := analyzerRow(uuid.New(), "A", "100", "250"), analyzerRow(uuid.New(), "B", "100", "250")
	usd.Currency = model.CurrencyUSD
	got := billing.BuildDashboard([]billing.DashboardRow{try, usd}, nil, nil)
	require.Len(t, got.Netting, 2, "one netting summary per currency; TRY and USD are never added")
	byCurrency := map[model.CurrencyCode]string{}
	for _, n := range got.Netting {
		byCurrency[n.Currency] = n.TotalInvoice.String()
	}
	require.Equal(t, "250", byCurrency[model.CurrencyTRY])
	require.Equal(t, "250", byCurrency[model.CurrencyUSD])
}

func TestDashboardKeepsRowsWithoutLiveParents(t *testing.T) {
	t.Parallel()
	// A building soft-deleted after the invoice was issued: the service passes
	// the name it resolved (or an empty one), and the row still counts.
	row := analyzerRow(uuid.New(), "", "50", "120")
	row.BuildingName = ""
	got := billing.BuildDashboard([]billing.DashboardRow{row}, nil, nil)
	require.Len(t, got.Buildings, 1)
	require.Equal(t, "120", got.Buildings[0].TotalInvoice.String())
}

func TestDashboardNettingAndEfficiency(t *testing.T) {
	t.Parallel()
	b := uuid.New()
	consume := analyzerRow(b, "A", "1000", "2500")
	produce := analyzerRow(b, "B", "0", "0")
	produce.Production = decimal.RequireFromString("250")
	got := billing.BuildDashboard([]billing.DashboardRow{consume, produce}, nil, nil)
	n := got.Netting[0]
	require.Equal(t, "750", n.Net.String())
	require.Equal(t, "net_consumption", n.NetStatus)
	require.Equal(t, "25", n.EfficiencyPct.String())
}

func TestDashboardEfficiencyIsAbsentWithoutConsumption(t *testing.T) {
	t.Parallel()
	row := analyzerRow(uuid.New(), "A", "0", "0")
	row.Production = decimal.RequireFromString("40")
	got := billing.BuildDashboard([]billing.DashboardRow{row}, nil, nil)
	require.Nil(t, got.Netting[0].EfficiencyPct, "no ratio exists against zero consumption")
	require.Equal(t, "net_production", got.Netting[0].NetStatus)
}
```

Add the two local helpers at the bottom of the file (nowhere else needs them):

```go
func analyzerRow(buildingID uuid.UUID, name, kwh, cost string) billing.DashboardRow {
	id := uuid.New()
	return billing.DashboardRow{BillID: uuid.New(), BuildingID: &buildingID, AnalyzerID: &id, AnalyzerName: name,
		BuildingName: "Bina", PeriodKey: "2026-08", Consumption: decimal.RequireFromString(kwh),
		Production: decimal.Zero, Invoice: decimal.RequireFromString(cost), Currency: model.CurrencyTRY}
}

func buildingRow(buildingID uuid.UUID, kwh, cost string) billing.DashboardRow {
	return billing.DashboardRow{BillID: uuid.New(), BuildingID: &buildingID, BuildingName: "Bina", PeriodKey: "2026-08",
		Consumption: decimal.RequireFromString(kwh), Production: decimal.Zero,
		Invoice: decimal.RequireFromString(cost), Currency: model.CurrencyTRY}
}
```

- [ ] **Step 2: Run them and watch them fail**

```bash
go test ./internal/domain/billing/... -run TestDashboard -v
```

Expected: FAIL — `undefined: billing.BuildDashboard`.

- [ ] **Step 3: Implement the aggregate**

Create `internal/domain/billing/dashboard.go`. The whole file is arithmetic over the rows it is
given; it opens no repository and reads no clock, which is what makes 09 §F8's reconciliation
criterion a table test.

```go
// Package-level note in the file header: the dashboard is derived, never
// stored. Its only input is the period's bills (R233), so the figure on the
// screen and the figure in the invoice are the same number.

const divergenceTolerance = "0.01" // ₺, R234

// BuildDashboard groups rows by building and sums them.
func BuildDashboard(rows, buildingBills, companyBills []DashboardRow) DashboardResult {
	order := make([]uuid.UUID, 0, len(rows))
	byBuilding := map[uuid.UUID]*DashboardBuilding{}
	for _, r := range rows {
		key := uuid.Nil
		if r.BuildingID != nil {
			key = *r.BuildingID
		}
		bld := byBuilding[key]
		if bld == nil {
			bld = &DashboardBuilding{BuildingID: key, BuildingName: r.BuildingName, Currency: r.Currency,
				TotalConsumption: decimal.Zero, TotalProduction: decimal.Zero, TotalInvoice: decimal.Zero}
			byBuilding[key], order = bld, append(order, key)
		}
		bld.Rows = append(bld.Rows, r)
		bld.TotalConsumption = bld.TotalConsumption.Add(r.Consumption)
		bld.TotalProduction = bld.TotalProduction.Add(r.Production)
		bld.TotalInvoice = bld.TotalInvoice.Add(r.Invoice)
	}
	// ... attach the building bill and set DivergesFromRows, then net per
	// currency; write the rest in the same shape as aggregate.go.
}
```

Netting per currency (R253) walks the rows once, keeps a `map[model.CurrencyCode]*DashboardNetting`
with its own insertion order, and computes `EfficiencyPct` as
`production.Div(consumption).Mul(hundred).Round(2)` only when `consumption.IsPositive()`.

- [ ] **Step 4: Run the whole package**

```bash
go test ./internal/domain/billing/... -race -count=1
```

Expected: PASS (F4's existing golden and property tests included).

- [ ] **Step 5: Commit**

```bash
git add internal/domain/billing
git commit -m "feat(f8a): task 2 — the dashboard aggregate, pure over the period's bills

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 6: Prove the guards red**

Back up `dashboard.go`. (a) Make the currency map a single bucket keyed by nothing —
`TestDashboardKeepsCurrenciesApart` must fail. (b) Replace `DivergesFromRows` with a constant
`false` — `TestDashboardShowsBuildingBillDivergence...` must fail. (c) Return `decimal.Zero` for
`EfficiencyPct` instead of `nil` — the zero-consumption test must fail. Restore after each,
re-run, expect PASS.

---
## Task 3: `GET /bills/dashboard` — service, DTO and route

**Files:**
- Create: `internal/service/billing/dashboard.go`
- Create: `internal/service/billing/dashboard_integration_test.go`
- Modify: `internal/api/v1/dto/bills.go` (dashboard DTOs)
- Modify: `internal/api/v1/handlers_billing.go` (route row + handler)
- Test: `internal/api/v1/billing_http_integration_test.go` (append)

**Interfaces:**
- Consumes: `billing.BuildDashboard` (Task 2); `store.BillRepository.List` with
  `store.BillFilter{PeriodKey, BillScope, Page}`; `store.BuildingRepository.List`;
  `store.AnalyzerRepository.List`; `store.Scope` from `mw.ScopeFrom(r)`.
- Produces, used by Task 4 and Task 10:

```go
// DashboardInput selects the month. Year and Month are 05 §7's own query
// parameters; the period key they form is what bills are stored under.
type DashboardInput struct {
	Year  int
	Month int
}

// Dashboard reads the period's bills in scope and aggregates them (R233).
// `domain` is how this package already imports internal/domain/billing
// (generate.go:13) — the service package is itself called `billing`.
func (s *Service) Dashboard(ctx context.Context, sc store.Scope, in DashboardInput) (domain.DashboardResult, error)
```

DTO (in `internal/api/v1/dto/bills.go`):

```go
// BillDashboardRequest is GET /bills/dashboard.
type BillDashboardRequest struct {
	Year  int `query:"year" json:"-" validate:"required,min=2000,max=2100" required:"true"`
	Month int `query:"month" json:"-" validate:"required,min=1,max=12" required:"true"`
}

// BillDashboardRow is one invoice line of the §7.10 table.
type BillDashboardRow struct {
	BillID             uuid.UUID  `json:"bill_id" required:"true"`
	BuildingID         *uuid.UUID `json:"building_id"`
	AnalyzerID         *uuid.UUID `json:"analyzer_id"`
	BuildingName       string     `json:"building_name" required:"true"`
	AnalyzerName       string     `json:"analyzer_name" required:"true"`
	InstallationNumber string     `json:"installation_number" required:"true"`
	EtsoCode           string     `json:"etso_code" required:"true"`
	PeriodKey          string     `json:"period_key" required:"true"`
	Consumption        Decimal    `json:"consumption" required:"true"`
	Production         Decimal    `json:"production" required:"true"`
	ConsumptionPrice   *Decimal   `json:"consumption_price"`
	ProductionPrice    *Decimal   `json:"production_price"`
	Invoice            Decimal    `json:"invoice" required:"true"`
	Currency           string     `json:"currency" required:"true"`
}

// BillDashboardBuilding is one building's section (R234).
type BillDashboardBuilding struct {
	BuildingID       uuid.UUID          `json:"building_id" required:"true"`
	BuildingName     string             `json:"building_name" required:"true"`
	Rows             []BillDashboardRow `json:"rows" required:"true"`
	TotalConsumption Decimal            `json:"total_consumption" required:"true"`
	TotalProduction  Decimal            `json:"total_production" required:"true"`
	TotalInvoice     Decimal            `json:"total_invoice" required:"true"`
	Currency         string             `json:"currency" required:"true"`
	BuildingBill     *BillDashboardRow  `json:"building_bill"`
	DivergesFromRows bool               `json:"diverges_from_rows" required:"true"`
}

// BillDashboardNetting is R236.
type BillDashboardNetting struct {
	Currency         string     `json:"currency" required:"true"`
	TotalConsumption Decimal    `json:"total_consumption" required:"true"`
	TotalProduction  Decimal    `json:"total_production" required:"true"`
	Net              Decimal    `json:"net" required:"true"`
	NetStatus        string     `json:"net_status" required:"true" enum:"net_consumption,net_production"`
	TotalInvoice     Decimal    `json:"total_invoice" required:"true"`
	EfficiencyPct    *Decimal   `json:"efficiency_pct"`
	PeriodKey        string     `json:"period_key" required:"true"`
	CompanyBillID    *uuid.UUID `json:"company_bill_id"`
}

// BillDashboardPlants is R235: the section exists, its data source does not.
type BillDashboardPlants struct {
	Available bool   `json:"available" required:"true"`
	Reason    string `json:"reason,omitempty"`
}

// BillDashboard is GET /bills/dashboard.
type BillDashboard struct {
	Period    string                  `json:"period" required:"true"`
	Buildings []BillDashboardBuilding `json:"buildings" required:"true"`
	Plants    BillDashboardPlants     `json:"plants" required:"true"`
	Netting   []BillDashboardNetting  `json:"netting" required:"true"`
}
```

- [ ] **Step 1: Write the failing service integration test**

Create `internal/service/billing/dashboard_integration_test.go` with the `//go:build integration`
tag and the fixture helpers `generate_integration_test.go` already uses (read it first):

```go
func TestDashboardReadsOnlyThePeriodAndOnlyInScope(t *testing.T) {
	// Two analyzers in one building with August bills, one with a July bill,
	// and one building in another company. August must return two rows.
	...
	got, err := svc.Dashboard(ctx, scope, billing.DashboardInput{Year: 2026, Month: 8})
	require.NoError(t, err)
	require.Len(t, got.Buildings, 1)
	require.Len(t, got.Buildings[0].Rows, 2)
	require.Equal(t, "2026-08", got.Netting[0].PeriodKey)
}

func TestDashboardExcludesSupersededBills(t *testing.T) {
	// Recompute supersedes; the dashboard shows the live bill only, once.
	...
	require.Len(t, got.Buildings[0].Rows, 1)
	require.Equal(t, live.ID, got.Buildings[0].Rows[0].BillID)
}
```

- [ ] **Step 2: Run it and watch it fail**

```bash
docker ps   # ekokod-test-pg / ekokod-test-redis; if Docker is down, say so and stop here
EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' \
EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
go test ./internal/service/billing/... -tags=integration -run TestDashboard -count=1 -v
```

Expected: FAIL — `svc.Dashboard undefined`.

- [ ] **Step 3: Implement the service read**

`internal/service/billing/dashboard.go`:

```go
// Dashboard is R233: one read of the period's bills, then arithmetic. The
// analyzer and building names come from the asset repositories the same scope
// can see; a bill whose parent was soft-deleted keeps its row with whatever
// name resolves, because the invoice was still issued (Review Focus 2).
func (s *Service) Dashboard(ctx context.Context, sc store.Scope, in DashboardInput) (billing.DashboardResult, error) {
	period := fmt.Sprintf("%04d-%02d", in.Year, in.Month)
	analyzerScope, buildingScope, companyScope := model.BillScopeAnalyzer, model.BillScopeBuilding, model.BillScopeCompany
	rows, err := s.rowsFor(ctx, sc, period, &analyzerScope)
	...
	return domain.BuildDashboard(rows, buildingRows, companyRows), nil
}
```

`rowsFor` pages through `store.BillFilter{PeriodKey: &period, BillScope: scope, Page: ...}` with the
package's existing paging helper shape, resolves names from maps built with one `Analyzers.List`
and one `Buildings.List` call, and maps each `model.Bill` to a `domain.DashboardRow` —
`Consumption: b.ActiveImport`, `Production: b.ActiveExport`,
`ConsumptionPrice: b.EffectiveEnergyPrice`, `ProductionPrice: b.GenerationPricePerKwh`,
`Invoice: b.TotalCost`, `Superseded: b.Status == model.BillStatusSuperseded` (always false here,
because the filter excludes them by default).

- [ ] **Step 4: Run the service package**

```bash
EKOKOD_TEST_PG_DSN=... go test ./internal/service/billing/... -tags=integration -count=1
go test ./internal/service/billing/... -race -count=1
```

Expected: PASS both.

- [ ] **Step 5: Write the failing HTTP test**

Append to `internal/api/v1/billing_http_integration_test.go`, reusing its harness:

```go
func TestBillDashboardReturnsTheMonth(t *testing.T) {
	h := newHarness(t)
	...
	res := h.as(seed.E2ECompanyAdminEmail).do(http.MethodGet, "/bills/dashboard?year=2026&month=8", nil)
	require.Equal(t, http.StatusOK, res.status)
	var body dto.BillDashboard
	res.json(t, &body)
	require.Equal(t, "2026-08", body.Period)
	require.False(t, body.Plants.Available, "R235: no plant production source before F9")
	require.Equal(t, "no_plant_production_source", body.Plants.Reason)
	require.Equal(t, body.Buildings[0].TotalInvoice, sumInvoices(body.Buildings[0].Rows))
}

func TestBillDashboardIsScopedLikeEveryOtherRead(t *testing.T) {
	// A building admin sees only their own building's section.
	...
}

func TestSupersededBillPDFStaysDownloadable(t *testing.T) {
	// R252: recompute, then ask for the OLD bill's PDF.
	...
	res := h.as(seed.E2ECompanyAdminEmail).do(http.MethodGet, "/bills/"+old.ID.String()+"/pdf", nil)
	require.Equal(t, http.StatusOK, res.status)
	require.Equal(t, "application/pdf", res.header.Get("Content-Type"))
}
```

- [ ] **Step 6: Add the route row and handler**

In `billingRoutes()`:

```go
		{Method: http.MethodGet, Pattern: "/bills/dashboard", OperationID: "bills.dashboard", Tag: "bills", Access: RoleGated,
			Roles: all, Summary: "The month's invoice dashboard: analyzer rows per building and the netting summary.",
			Request: dto.BillDashboardRequest{}, Response: dto.BillDashboard{}, Status: http.StatusOK,
			Handler: (*Handlers).billDashboard},
```

Handler: `serve(...)` → `h.Billing.Dashboard(...)` → map to the DTO; `Plants` is always
`dto.BillDashboardPlants{Available: false, Reason: "no_plant_production_source"}` with a one-line
comment pointing at R235 and F9.

**Route ordering matters:** `/bills/dashboard` must be registered before `/bills/{id}`, exactly as
`/bills/latest` already is, or chi resolves `dashboard` as an id. `route_table_test.go` has the
guard — run it.

- [ ] **Step 7: Run the API package**

```bash
go test ./internal/api/v1/... -race -count=1
EKOKOD_TEST_PG_DSN=... go test ./internal/api/v1/... -tags=integration -count=1
make openapi && git diff --stat   # regenerates openapi.json and web schema.d.ts
```

Expected: PASS; the OpenAPI diff shows exactly the new operation.

- [ ] **Step 8: Commit**

```bash
git add internal/service/billing internal/api/v1 web/src/lib/api/schema.d.ts
git commit -m "feat(f8a): task 3 — GET /bills/dashboard over the period's bills

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 9: Prove the guards red**

Back up `dashboard.go`. (a) Drop the `PeriodKey` filter — `TestDashboardReadsOnlyThePeriod...`
must fail. (b) Set `IncludeSuperseded: true` — the supersede test must fail. (c) Hard-code
`Plants.Available = true` — the HTTP test must fail. Restore, re-run, expect PASS.

---

## Task 4: `GET /bills/dashboard/export` — one file, the same numbers

**Files:**
- Create: `internal/render/dashboardxlsx/xlsx.go` + `xlsx_test.go`
- Create: `internal/render/dashboardpdf/pdf.go` + `pdf_test.go`
- Modify: `internal/api/v1/dto/bills.go` (`BillDashboardExportRequest`)
- Modify: `internal/api/v1/handlers_billing.go` (route row + handler)

**Interfaces:**
- Consumes: `billing.DashboardResult` (Task 2), `(*billingsvc.Service).Dashboard` (Task 3),
  `internal/render/table` (the PDF table primitive F4 already uses), `internal/render/money.go`.
- Produces:

```go
// dashboardxlsx — `billing` here is internal/domain/billing, imported under
// its own name: a render package has no service import to clash with.
func Render(res billing.DashboardResult, period string, loc string) ([]byte, error)

// dashboardpdf
func Render(res billing.DashboardResult, period, companyName string, loc string) ([]byte, error)
```

`loc` is the request locale (`"tr"`/`"en"`), as `hourlyxlsx` already takes one — read it first and
copy its label approach rather than inventing a second one.

- [ ] **Step 1: Write the failing renderer tests**

`internal/render/dashboardxlsx/xlsx_test.go` — assert on the parsed workbook, not on bytes:

```go
func TestWorkbookHasABuildingsSheetAndASummarySheet(t *testing.T) {
	body, err := dashboardxlsx.Render(fixtureResult(), "2026-08", "tr")
	require.NoError(t, err)
	f, err := excelize.OpenReader(bytes.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, []string{"Faturalar", "Özet"}, f.GetSheetList())
	rows, err := f.GetRows("Faturalar")
	require.NoError(t, err)
	require.Equal(t, "Toplam", rows[len(rows)-1][0], "the total row is the last row of the sheet")
}

func TestWorkbookTotalEqualsTheSumOfItsRows(t *testing.T) {
	// The file may not disagree with the screen (R239).
	...
}
```

- [ ] **Step 2: Run them and watch them fail**

```bash
go test ./internal/render/... -run TestWorkbook -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement both renderers**

XLSX: one sheet of the §7.10 columns (date, building, analyzer, installation no, ETSO,
consumption, price, invoice) with a per-building subtotal row and a final total row, then a summary
sheet carrying the netting figures and, when `Plants.Available` is false, one line saying the plant
section has no data source yet — the file must not be quietly missing a section the screen shows.

PDF: `internal/render/table` over the same rows, A4 landscape, the invoice PDF's header block
reused for the company name and period.

- [ ] **Step 4: Add the route**

```go
		{Method: http.MethodGet, Pattern: "/bills/dashboard/export", OperationID: "bills.dashboard.export", Tag: "bills",
			Access: RoleGated, Roles: all, Summary: "The whole dashboard as one XLSX or PDF.",
			Request: dto.BillDashboardExportRequest{}, Status: http.StatusOK, RawContentType: "application/octet-stream",
			Handler: (*Handlers).billDashboardExport},
```

```go
// BillDashboardExportRequest is GET /bills/dashboard/export.
type BillDashboardExportRequest struct {
	Year   int    `query:"year" json:"-" validate:"required,min=2000,max=2100" required:"true"`
	Month  int    `query:"month" json:"-" validate:"required,min=1,max=12" required:"true"`
	Format string `query:"format" json:"-" validate:"omitempty,oneof=xlsx pdf" enum:"xlsx,pdf"`
}
```

The handler calls the **same** `h.Billing.Dashboard` as Task 3 and then one of the two renderers,
so R239 holds by construction; filename `fatura-panosu-2026-08.xlsx` / `.pdf`.

- [ ] **Step 5: HTTP test**

Append to `internal/api/v1/billing_http_integration_test.go`:

```go
func TestDashboardExportDefaultsToXlsxAndMatchesTheScreen(t *testing.T) {
	...
	require.Equal(t, xlsxType, res.header.Get("Content-Type"))
	// Same request, JSON: the totals in the file and on the screen agree.
}
```

- [ ] **Step 6: Run the packages**

```bash
go test ./internal/render/... ./internal/api/v1/... -race -count=1
EKOKOD_TEST_PG_DSN=... go test ./internal/api/v1/... -tags=integration -count=1
make openapi
```

- [ ] **Step 7: Commit**

```bash
git add internal/render internal/api/v1 web/src/lib/api/schema.d.ts
git commit -m "feat(f8a): task 4 — dashboard export renders the same aggregate as the screen

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 8: Prove the guard red**

Back up the XLSX renderer, make the total row write a hard-coded `0` — the total test must fail.
Restore, re-run, expect PASS.

---

## Task 5: The compute job becomes watchable, with a machine-readable failure

**Files:**
- Create: `internal/store/postgres/migrations/00016_job_run_task_id.sql`
- Modify: `internal/store/postgres/queries/operations.sql`, `internal/store/postgres/ops.go`,
  `internal/store/repository.go`
- Modify: `internal/service/billing/jobs.go` (write the task id)
- Modify: `internal/service/jobs/jobs.go` (watchable + `ErrorCode`)
- Modify: `internal/api/v1/dto/jobs.go`, `internal/api/v1/handlers_jobs.go`
- Test: `internal/service/jobs/jobs_test.go`, `internal/store/postgres/ops_integration_test.go`,
  `internal/api/v1/jobs_http_integration_test.go`

**Interfaces:**
- Consumes: `model.JobRun`, `store.OpsRepository.StartRun/FinishRun`, `asynq.TaskInfo`.
- Produces, used by Task 10:

```go
// store
type OpsRepository interface {
	...
	// RunByTaskID is the newest run an enqueued task produced, or ErrNotFound.
	RunByTaskID(ctx context.Context, s Scope, taskID string) (model.JobRun, error)
}

// jobs
type View struct {
	ID          string
	Type        string
	Status      Status
	CompletedAt *time.Time
	// ErrorCode is one of billing's R113 codes, or "". Never free text (R237).
	ErrorCode string
}
```

- [ ] **Step 1: Write the migration**

`00016_job_run_task_id.sql`, in F1's style — a Down that drops exactly what the Up added:

```sql
-- +goose Up
-- R237: the screen that enqueued a job needs to know WHY it failed, and the
-- reason lives in job_runs.detail. asynq's task id is the only handle the
-- screen holds, so the run records it.
alter table job_runs add column task_id text;
create index on job_runs (task_id) where task_id is not null;

-- +goose Down
drop index if exists job_runs_task_id_idx;
alter table job_runs drop column if exists task_id;
```

- [ ] **Step 2: Run the migration round-trip guard**

```bash
EKOKOD_TEST_PG_DSN=... go test ./internal/store/postgres/... -tags=integration -run TestMigrations -count=1 -v
```

Expected: PASS (F1's guard migrates up and down and compares the schema).

- [ ] **Step 3: Write the failing store and service tests**

`ops_integration_test.go`: `TestRunByTaskIDReturnsTheNewestRun` — two runs with the same task id,
the newer one wins; a foreign company's run is `ErrNotFound`.

`internal/service/jobs/jobs_test.go` (unit, with the fake inspector the file already has):

```go
func TestBillingGenerateIsWatchable(t *testing.T) {
	// Before F8a the allow-list held only the three integration jobs, so the
	// bills screen could not watch the job it had just started.
	...
	require.Equal(t, jobs.Failed, v.Status)
	require.Equal(t, "tariff_not_found", v.ErrorCode)
}

func TestErrorCodeIsNeverFreeText(t *testing.T) {
	// A run whose detail carries {"error": "dial tcp ..."} yields no code at
	// all — R192 holds: the worker's own text never leaves the process.
	...
	require.Empty(t, v.ErrorCode)
}
```

- [ ] **Step 4: Implement**

- `internal/service/billing/jobs.go`: `StartRun` gains `TaskID: job.BillingGenerateTaskID(p)`. The
  id is **deterministic from the payload** (`internal/job/billing.go:41`), so nothing new travels
  with the task and the id in Redis and the id in Postgres are the same string by construction.
- `internal/service/jobs/jobs.go`: add `job.TypeBillingGenerate` to `watchable`; extend
  `payloadScope` with the billing payload's `Scope`/`SubjectID` so the scope check still answers
  404 for a foreign job (a building-scope job is allowed when the building is in scope, an
  analyzer-scope job when the analyzer is); and, **only when the task failed**, read
  `Ops.RunByTaskID(ctx, sc, job.BillingGenerateTaskID(payload))` and accept its `detail.code`
  **only if it is in the R113 set**. `Deps` gains `Ops store.OpsRepository`, validated in `New`.
- `dto.Job` gains `ErrorCode string json:"error_code,omitempty"`.

- [ ] **Step 5: Run the touched packages**

```bash
go test ./internal/service/jobs/... ./internal/service/billing/... ./internal/api/v1/... -race -count=1
EKOKOD_TEST_PG_DSN=... go test ./internal/store/postgres/... ./internal/api/v1/... -tags=integration -count=1
make check-generate && make openapi
```

- [ ] **Step 6: Commit**

```bash
git add internal/store internal/service internal/api web/src/lib/api/schema.d.ts
git commit -m "feat(f8a): task 5 — bills.compute is watchable and reports an R113 code

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 7: Prove the guards red**

Back up `jobs.go`; let any `detail.code` through instead of checking the R113 set —
`TestErrorCodeIsNeverFreeText` must fail. Restore, re-run, expect PASS.

---
## Task 6: Tariff templates, bulk assignment and its history

**Files:**
- Create: `internal/store/postgres/migrations/00017_tariff_bulk_assignments.sql`
- Create: `internal/service/tariff/history.go` + `history_integration_test.go`
- Create: `internal/api/v1/handlers_tariffs.go`, `internal/api/v1/dto/tariffs_extra.go`
- Modify: `internal/store/repository.go`, `internal/store/postgres/queries/tariffs.sql`,
  `internal/store/postgres/tariffs.go`, `internal/domain/model/tariffs.go`
- Modify: `internal/service/tariff/service.go` (`BulkAssign` records), `templates.go`
  (`ApplyTemplate` records), `internal/api/v1/routes.go`, `router.go`, `internal/apiwire/wiring.go`
- Test: `internal/service/tariff/service_integration_test.go`,
  `internal/api/v1/tariffs_http_integration_test.go` (new file, harness reused)

**Interfaces:**
- Consumes: `(*tariff.Service).CreateTemplate/UpdateTemplate/DeleteTemplate/ListTemplates/ApplyTemplate/BulkAssign/CurrentForBuildings`
  — all of them already exist (F4); this task adds no business logic to them beyond the recording.
- Produces:

```go
// model
type TariffBulkAssignment struct {
	ID            uuid.UUID
	CompanyID     uuid.UUID
	TemplateID    *uuid.UUID
	TariffName    *string
	EffectiveFrom time.Time
	BuildingIDs   []uuid.UUID
	CreatedBy     *uuid.UUID
	CreatedAt     time.Time
}

// store
type BulkAssignmentRepository interface {
	Create(ctx context.Context, s Scope, a model.TariffBulkAssignment) (model.TariffBulkAssignment, error)
	List(ctx context.Context, s Scope, p Page) ([]model.TariffBulkAssignment, error)
}

// service
func (s *Service) BulkHistory(ctx context.Context, sc store.Scope, p store.Page) ([]model.TariffBulkAssignment, error)
```

`BulkAssign` and `ApplyTemplate` gain a trailing `by uuid.UUID` parameter (the acting principal) so
the history can name who did it; every existing caller passes `uuid.Nil` for a system action.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
-- R241: 05 §6 asks for a bulk-assignment history and nothing in the schema
-- records one. A tariff version cannot answer "which buildings were assigned
-- together, from which template" — it only knows its own building.
create table tariff_bulk_assignments (
    id             uuid primary key default gen_random_uuid(),
    company_id     uuid not null references companies(id),
    template_id    uuid references tariff_templates(id) on delete set null,
    tariff_name    text,
    effective_from date not null,
    building_ids   uuid[] not null,
    created_by     uuid references users(id),
    created_at     timestamptz not null default now()
);
create index on tariff_bulk_assignments (company_id, created_at desc);

-- +goose Down
drop table if exists tariff_bulk_assignments;
```

- [ ] **Step 2: Run the round-trip guard, watch it pass, then write the failing service test**

```bash
EKOKOD_TEST_PG_DSN=... go test ./internal/store/postgres/... -tags=integration -run TestMigrations -count=1
```

Then append to `internal/service/tariff/service_integration_test.go`:

```go
func TestBulkAssignRecordsWhatItWrote(t *testing.T) {
	...
	defs, err := svc.BulkAssign(ctx, scope, input, []uuid.UUID{b1, b2}, actor)
	require.NoError(t, err)
	require.Len(t, defs, 2)
	hist, err := svc.BulkHistory(ctx, scope, store.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, hist, 1)
	require.ElementsMatch(t, []uuid.UUID{b1, b2}, hist[0].BuildingIDs)
	require.Equal(t, actor, *hist[0].CreatedBy)
}

func TestBulkAssignRefusesForeignBuildingAndRecordsNothing(t *testing.T) {
	// Review Focus 5. BulkAssign checks visibility before it writes anything;
	// this pins that the history does not claim the buildings that would
	// otherwise have succeeded.
	_, err := svc.BulkAssign(ctx, scope, input, []uuid.UUID{b1, foreign}, actor)
	require.ErrorIs(t, err, store.ErrNotFound)
	hist, err := svc.BulkHistory(ctx, scope, store.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, hist)
	tariffs, err := svc.List(ctx, scope, store.TariffFilter{BuildingID: &b1})
	require.NoError(t, err)
	require.Empty(t, tariffs, "no building gets a version when one of them is out of scope")
}

func TestApplyTemplateRecordsTheTemplateItUsed(t *testing.T) {
	...
	require.Equal(t, tpl.ID, *hist[0].TemplateID)
}
```

- [ ] **Step 3: Run it and watch it fail**

```bash
EKOKOD_TEST_PG_DSN=... go test ./internal/service/tariff/... -tags=integration -run TestBulk -count=1 -v
```

Expected: FAIL — `BulkHistory` undefined and `BulkAssign` takes three arguments.

- [ ] **Step 4: Implement the repository and the recording**

sqlc queries in `tariffs.sql` (`TariffBulkAssignmentCreate`, `TariffBulkAssignmentList`), the
repository in `internal/store/postgres/tariffs.go` next to `TariffTemplateRepository`, then in
`internal/service/tariff/history.go`:

```go
// RecordBulkAssignment writes what actually happened, after the assignment
// (R241): a call that failed halfway records the buildings that got a version,
// never the ones that were asked for.
func (s *Service) RecordBulkAssignment(ctx context.Context, sc store.Scope, a model.TariffBulkAssignment) error
```

`BulkAssign` calls it with the ids of the definitions it created; `ApplyTemplate` passes the
template id through.

- [ ] **Step 5: Run the service package**

```bash
go test ./internal/service/tariff/... -race -count=1
EKOKOD_TEST_PG_DSN=... go test ./internal/service/tariff/... ./internal/store/postgres/... -tags=integration -count=1
```

- [ ] **Step 6: Write the failing HTTP tests**

Create `internal/api/v1/tariffs_http_integration_test.go` with the harness the other
`*_http_integration_test.go` files use:

```go
func TestTemplateCrudAndApply(t *testing.T) {
	// create → list → patch → apply to two buildings → history shows one row
}

func TestTemplateWriteIsRefusedForReadonlyAdmin(t *testing.T) {
	res := h.as(seed.E2ECompanyReadonlyEmail).do(http.MethodPost, "/tariff-templates", body)
	require.Equal(t, http.StatusForbidden, res.status)
}

func TestBulkCurrentListsEveryBuildingsTariff(t *testing.T) {
	// A building with no tariff appears with a null tariff, not missing:
	// "which buildings have no tariff" is the question the screen asks.
}
```

- [ ] **Step 7: Add the routes**

`internal/api/v1/handlers_tariffs.go` — rows 3–10 of the route rule table, then register
`tariffRoutes()` in `Table()`. Every handler is a `serve(...)` wrapper mapping DTO ↔ service types
(R240). `GET /buildings/bulk-tariff/current` returns
`dto.Page[dto.BuildingTariffState]{BuildingID, BuildingName, TariffID *uuid.UUID, TariffName *string, EffectiveFrom *Date, UsePtfYekdem bool}`.

- [ ] **Step 8: Run the API package**

```bash
go test ./internal/api/v1/... -race -count=1
EKOKOD_TEST_PG_DSN=... go test ./internal/api/v1/... -tags=integration -count=1
make check-generate && make openapi
```

- [ ] **Step 9: Commit**

```bash
git add internal/store internal/domain/model internal/service/tariff internal/api web/src/lib/api/schema.d.ts
git commit -m "feat(f8a): task 6 — tariff templates, bulk assignment and its recorded history

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 10: Prove the guards red**

Back up `service.go`; move the `RecordBulkAssignment` call **before** the write loop —
`TestBulkAssignRefusesForeignBuildingAndRecordsNothing` must fail. Restore, re-run, expect PASS.

---

## Task 7: The icmal import — upload, analysis, confirm

**Files:**
- Modify: `internal/api/v1/routes.go` (`Route.MaxBody`), `internal/api/v1/router.go` (per-route
  body limit)
- Modify: `internal/api/v1/handlers_tariffs.go` (rows 14–16), `internal/api/v1/dto/tariffs_extra.go`
- Test: `internal/api/v1/tariffs_http_integration_test.go`, `internal/api/v1/router_test.go`

**Interfaces:**
- Consumes: `(*tariff.Service).Import(ctx, sc, uploadedBy, fileName, content) (ImportResult, error)`,
  `GetImport`, `ApplyImport(ctx, sc, importID, []ApplyConfirmation)`, `icmal.Analysis`,
  `icmal.Coefficient`, `icmal.Warning` — all existing (F4).
- Produces:

```go
// dto
type IcmalCoefficient struct {
	Value            *Decimal `json:"value"`
	Samples          int      `json:"samples" required:"true"`
	StdDev           *Decimal `json:"std_dev"`
	Stable           *bool    `json:"stable"`
	BackCalcErrorPct *Decimal `json:"back_calc_error_pct"`
}

type IcmalAnalysis struct {
	EtsoCode                 string                      `json:"etso_code" required:"true"`
	BuildingID               *uuid.UUID                  `json:"building_id"`
	BuildingName             *string                     `json:"building_name"`
	Periods                  []string                    `json:"periods" required:"true"`
	EnergyKbk                IcmalCoefficient            `json:"energy_kbk" required:"true"`
	DistributionTlPerKwh     IcmalCoefficient            `json:"distribution_tl_per_kwh" required:"true"`
	PowerUnitPrice           IcmalCoefficient            `json:"power_unit_price" required:"true"`
	ImpliedContractedPowerKw *Decimal                    `json:"implied_contracted_power_kw"`
	ReactiveUnitPrice        IcmalCoefficient            `json:"reactive_unit_price" required:"true"`
	ReactiveKbk              IcmalCoefficient            `json:"reactive_kbk" required:"true"`
	VatRate                  IcmalCoefficient            `json:"vat_rate" required:"true"`
	Taxes                    map[string]IcmalCoefficient `json:"taxes" required:"true"`
	IsMultiTime              *bool                       `json:"is_multi_time"`
	Term                     *string                     `json:"term"`
	VoltageLevel             *string                     `json:"voltage_level"`
	WithinTolerance          bool                        `json:"within_tolerance" required:"true"`
	Warnings                 []IcmalWarning              `json:"warnings" required:"true"`
}

type IcmalImport struct {
	ID        uuid.UUID       `json:"id" required:"true"`
	FileName  string          `json:"file_name" required:"true"`
	Status    string          `json:"status" required:"true" enum:"pending,analysed,applied"`
	RowCount  int             `json:"row_count" required:"true"`
	Analyses  []IcmalAnalysis `json:"analyses" required:"true"`
	Unmatched []string        `json:"unmatched" required:"true"`
	Warnings  []IcmalWarning  `json:"warnings" required:"true"`
	CreatedAt time.Time       `json:"created_at" required:"true"`
}

type IcmalApplyRequest struct {
	ID            uuid.UUID          `path:"id" json:"-"`
	Confirmations []IcmalConfirmation `json:"confirmations" validate:"required,min=1,max=500,dive" required:"true"`
}

type IcmalConfirmation struct {
	BuildingID    uuid.UUID `json:"building_id" validate:"required" required:"true"`
	EtsoCode      string    `json:"etso_code" validate:"required,max=40" required:"true"`
	EffectiveFrom Date      `json:"effective_from" validate:"required" required:"true"`
	// Base is required when the building has no applicable tariff at
	// EffectiveFrom; the service says so with its own error.
	Base *TariffFields `json:"base,omitempty"`
}
```

- [ ] **Step 1: Write the failing router test for the per-route body limit**

Append to `internal/api/v1/router_test.go`:

```go
func TestBodyLimitIsPerRoute(t *testing.T) {
	// The global 1 MiB cap is right for JSON and wrong for a spreadsheet
	// upload; a per-route MaxBody lifts it for exactly one route.
	big := strings.Repeat("a", 2<<20)
	res := h.as(seed.E2ECompanyAdminEmail).do(http.MethodPost, "/tariffs", map[string]any{"name": big})
	require.Equal(t, http.StatusRequestEntityTooLarge, res.status, "an ordinary route keeps the 1 MiB cap")
}
```

- [ ] **Step 2: Implement the per-route limit**

`routes.go`: `MaxBody int64` on `Route`. `router.go`: delete `r.Use(mw.BodyLimit(maxBody))` and put
`mw.BodyLimit(cmp.Or(rt.MaxBody, int64(maxBody)))` at the head of each route's chain — an inner
`MaxBytesReader` cannot lift an outer one, so the global `Use` has to go.

- [ ] **Step 3: Write the failing upload tests**

```go
func TestIcmalUploadAnalysesWithoutWritingATariff(t *testing.T) {
	ca := h.as(seed.E2ECompanyAdminEmail)
	// upload() is the one new helper this file needs: it builds a
	// multipart/form-data body and posts it with the right Content-Type.
	res := ca.upload(t, "/icmal-imports", "icmal.xlsx", icmalWorkbook(t))
	require.Equal(t, http.StatusCreated, res.status)
	var imp dto.IcmalImport
	res.json(t, &imp)
	require.Equal(t, "analysed", imp.Status)
	require.NotEmpty(t, imp.Analyses)
	// 02 §8.4: nothing is written until the operator confirms.
	tariffs := ca.do(http.MethodGet, "/tariffs?building_id="+b1.String(), nil)
	require.Equal(t, 0, pageTotal(t, tariffs))
}

func TestIcmalUploadRejectsUnreadableFiles(t *testing.T) {
	// Review Focus 4: empty file, and a PDF renamed .xlsx.
	for _, tc := range []struct{ name string; body []byte }{
		{"empty.xlsx", nil},
		{"not-a-sheet.xlsx", []byte("%PDF-1.7\n")},
	} {
		res := ca.upload(t, "/icmal-imports", tc.name, tc.body)
		require.Equal(t, http.StatusUnprocessableEntity, res.status, tc.name)
		require.Equal(t, "validation_failed", res.code(t))
	}
}

func TestIcmalUploadTooLarge(t *testing.T) {
	res := ca.upload(t, "/icmal-imports", "big.xlsx", bytes.Repeat([]byte("a"), 11<<20))
	require.Equal(t, http.StatusRequestEntityTooLarge, res.status)
}

func TestIcmalApplyRefusesAnEmptyConfirmationList(t *testing.T) {
	res := ca.do(http.MethodPost, "/icmal-imports/"+imp.ID.String()+"/apply", map[string]any{"confirmations": []any{}})
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
}

func TestIcmalApplyWritesOneVersionPerConfirmedBuilding(t *testing.T) {
	// The acceptance criterion of 09 §F8: no coefficient without confirmation.
	...
	require.Equal(t, 1, pageTotal(t, ca.do(http.MethodGet, "/tariffs?building_id="+b1.String(), nil)))
	require.Equal(t, 0, pageTotal(t, ca.do(http.MethodGet, "/tariffs?building_id="+b2.String(), nil)))
}
```

The sample workbook: reuse the icmal fixture `internal/domain/tariff/icmal` already tests against
(`internal/service/tariff/sheet_test.go` builds one) rather than committing a new binary.

- [ ] **Step 4: Add the three routes**

```go
		{Method: http.MethodPost, Pattern: "/icmal-imports", OperationID: "icmal.create", Tag: "tariffs", Access: RoleGated,
			Roles: aca, Entity: "icmal_import", Multipart: true, NoIdempotency: true, MaxBody: 10 << 20,
			Summary: "Upload and analyse an icmal; writes no tariff.", Response: dto.IcmalImport{},
			Status: http.StatusCreated, Handler: (*Handlers).createIcmalImport},
```

plus `GET /icmal-imports/{id}` and `POST /icmal-imports/{id}/apply`. The create handler reads the
multipart form with `r.ParseMultipartForm(10 << 20)`, takes the `file` part, and maps
`ErrInvalidRequest` from `ReadSheet`/`Parse` onto 422 `validation_failed` with
`{"file": ["unreadable"]}`.

- [ ] **Step 5: Run the API package**

```bash
go test ./internal/api/v1/... -race -count=1
EKOKOD_TEST_PG_DSN=... go test ./internal/api/v1/... -tags=integration -count=1
make openapi
```

- [ ] **Step 6: Commit**

```bash
git add internal/api web/src/lib/api/schema.d.ts
git commit -m "feat(f8a): task 7 — icmal upload, analysis and confirmed apply

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 7: Prove the guards red**

Back up the handler; let `apply` fall through with an empty confirmation list (skip the `min=1`
validation by removing the tag) — the empty-list test must fail. Restore, re-run, expect PASS.

---

## Task 8: Solar tariffs

**Files:**
- Create: `internal/service/tariff/solar.go` + tests in `service_integration_test.go`
- Modify: `internal/api/v1/handlers_tariffs.go` (rows 11–13), `dto/tariffs_extra.go`
- Modify: `internal/service/tariff/service.go` (`Deps.SolarTariffs`), `internal/apiwire/wiring.go`

**Interfaces:**
- Consumes: `store.SolarTariffRepository` (F1: `Get`, `List`, `Create`, `Delete`),
  `store.SolarTariffFilter{PlantID, EffectiveOn, IncludeDeleted, Page}`, `store.PlantRepository`
  for the scope check.
- Produces:

```go
func (s *Service) ListSolarTariffs(ctx context.Context, sc store.Scope, plantID uuid.UUID, p store.Page) ([]model.SolarTariff, error)
func (s *Service) CreateSolarTariff(ctx context.Context, sc store.Scope, t model.SolarTariff) (model.SolarTariff, error)
func (s *Service) DeleteSolarTariff(ctx context.Context, sc store.Scope, id uuid.UUID) error
```

- [ ] **Step 1: Write the failing tests**

```go
func TestSolarTariffRequiresAVisiblePlant(t *testing.T) {
	_, err := svc.CreateSolarTariff(ctx, scope, model.SolarTariff{PlantID: foreignPlant, FeedInTariff: d("2.5")})
	require.ErrorIs(t, err, store.ErrNotFound, "a foreign plant answers exactly like an unknown one")
}

func TestSolarTariffHistoryIsNewestFirstAndHidesDeleted(t *testing.T) {
	...
}

func TestSolarTariffRejectsANegativeFeedIn(t *testing.T) {
	_, err := svc.CreateSolarTariff(ctx, scope, model.SolarTariff{PlantID: plant, FeedInTariff: d("-1")})
	require.ErrorIs(t, err, tariff.ErrInvalidRequest)
}
```

- [ ] **Step 2: Run them and watch them fail**

```bash
EKOKOD_TEST_PG_DSN=... go test ./internal/service/tariff/... -tags=integration -run TestSolar -count=1 -v
```

- [ ] **Step 3: Implement**

`solar.go` validates (plant visible, `feed_in_tariff >= 0`, `purchase_price` optional and `>= 0`,
currency valid, `effective_from` non-zero) and delegates. The plant check is one
`Plants.Get(ctx, sc, plantID)` — the repository's own scope rules then answer 404 for anything
foreign (R244).

- [ ] **Step 4: Add the routes and run**

Rows 11–13; `GET` requires `plant_id`. Then:

```bash
go test ./internal/service/tariff/... ./internal/api/v1/... -race -count=1
EKOKOD_TEST_PG_DSN=... go test ./internal/service/tariff/... ./internal/api/v1/... -tags=integration -count=1
make openapi
```

- [ ] **Step 5: Commit**

```bash
git add internal/service/tariff internal/api internal/apiwire web/src/lib/api/schema.d.ts
git commit -m "feat(f8a): task 8 — plant feed-in tariffs

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 6: Prove the guard red**

Remove the `Plants.Get` check — `TestSolarTariffRequiresAVisiblePlant` must fail. Restore.

---

## Task 9: The admin default-tariff catalogue

**Files:**
- Create: `internal/service/tariff/defaults.go` + tests
- Modify: `internal/store/postgres/queries/admin_catalogue.sql`,
  `internal/store/postgres/admin/catalogue.go`, `internal/store/repository.go`
- Modify: `internal/api/v1/handlers_tariffs.go` (rows 17–20), `dto/tariffs_extra.go`

**Interfaces:**
- Consumes: `model.NationalTariffScheduleEntry`, `admin.CatalogueRepository.UpsertNationalTariffSchedule`
  (existing, used by the seeder).
- Produces:

```go
// store (admin)
func (r *CatalogueRepository) ListNationalTariffSchedule(ctx context.Context, f store.NationalTariffFilter) ([]model.NationalTariffScheduleEntry, error)
func (r *CatalogueRepository) DeleteNationalTariffScheduleEntry(ctx context.Context, id uuid.UUID) error

// service
func (s *Service) ListDefaults(ctx context.Context, f store.NationalTariffFilter) ([]model.NationalTariffScheduleEntry, error)
func (s *Service) UpsertDefault(ctx context.Context, e model.NationalTariffScheduleEntry) (model.NationalTariffScheduleEntry, error)
func (s *Service) DeleteDefault(ctx context.Context, id uuid.UUID) error
```

`store.NationalTariffFilter{EffectiveOn *time.Time, UserGroup *model.DistributionUserGroup, VoltageLevel *model.VoltageLevel, Term *model.TariffTerm, Page Page}` —
the four filters the legacy admin tab offers, and no more.

- [ ] **Step 1: Write the failing tests**

`internal/store/postgres/admin/catalogue_integration_test.go`:

```go
func TestNationalScheduleListFiltersAndOrders(t *testing.T) {
	// newest effective_from first; the user-group filter narrows
}
```

`internal/api/v1/tariffs_http_integration_test.go`:

```go
func TestNationalScheduleIsPublicToReadAndAdminToWrite(t *testing.T) {
	require.Equal(t, http.StatusOK, h.client(uaChrome).do(http.MethodGet, "/national-tariff-schedule", nil).status)
	require.Equal(t, http.StatusForbidden, h.as(seed.E2ECompanyAdminEmail).do(http.MethodPost, "/national-tariff-schedule", body).status)
	require.Equal(t, http.StatusCreated, h.as(seed.E2EAdminEmail).do(http.MethodPost, "/national-tariff-schedule", body).status)
}

func TestNationalScheduleWriteIsAuditedAtPlatformLevel(t *testing.T) {
	// PlatformAudit: the audit row carries a NULL company_id (R156).
}
```

- [ ] **Step 2–4: Implement, run, commit**

sqlc queries → repository methods → service → four route rows (`PlatformAudit: true`, no scope
middleware for the public GET). Then:

```bash
go test ./internal/api/v1/... -race -count=1
EKOKOD_TEST_PG_DSN=... go test ./internal/store/postgres/... ./internal/api/v1/... -tags=integration -count=1
make check-generate && make openapi
git add internal/store internal/service/tariff internal/api web/src/lib/api/schema.d.ts
git commit -m "feat(f8a): task 9 — admin default tariffs over the national schedule

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 5: Prove the guard red**

Change the POST row's roles to `aca` — `TestNationalScheduleIsPublicToReadAndAdminToWrite` must
fail on the company-admin line. Restore, re-run, expect PASS.

---
## Task 10: Web — the Bills screen

**Files:**
- Create: `web/src/app/ekorm/bills/page.tsx`
- Create: `web/src/features/bills/bills-page.tsx` + `.test.tsx`
- Create: `web/src/features/bills/dashboard-table.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/bills/netting-summary.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/bills/generate-card.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/bills/generation-status.ts` + `.test.ts`
- Create: `web/src/features/bills/_fixture.ts`
- Create: `web/messages/tr/bills.json`, `web/messages/en/bills.json`
- Modify: `web/messages/index.ts`

**Interfaces:**
- Consumes: `$api.useQuery('get', '/api/v1/bills/dashboard')`, `'/api/v1/analyzers'`,
  `'/api/v1/buildings'`, `'/api/v1/jobs/{id}'`; `useApiMutation('post', '/api/v1/bills/compute')`;
  `downloadFile` from `@/lib/api/download`; `useScopeParams`, `useSession().can`;
  `MonthPicker`, `DataTable`, `MultiSelect`, `StatTile`, `EmptyState`, `Alert` from
  `@/components/ui`.
- Produces (used by Task 15's e2e):

```ts
// generation-status.ts
export type GenerationPhase =
  | { kind: 'idle' }
  | { kind: 'queued'; jobIds: string[] }
  | { kind: 'done'; billIds: string[] }
  | { kind: 'failed'; messageKey: BillsMessageKey };

/** Maps an R113 error_code onto the §7.10 sentence for that failure. */
export function messageKeyForErrorCode(code: string | undefined): BillsMessageKey;

/** The client-side guards of §7.10, run before anything is enqueued. */
export function validateSelection(input: {
  analyzerIds: string[]; buildingId: string | null; period: string | null; scope: 'analyzer' | 'building' | 'company';
}): BillsMessageKey | null;
```

- [ ] **Step 1: Write the failing unit test for the message mapping**

`web/src/features/bills/generation-status.test.ts`:

```ts
describe('§7.10 validation messages', () => {
  it('names every selection failure before enqueueing anything', () => {
    expect(validateSelection({ analyzerIds: [], buildingId: 'b', period: '2026-08', scope: 'analyzer' })).toBe('noAnalyzerSelected');
    expect(validateSelection({ analyzerIds: ['a'], buildingId: 'b', period: null, scope: 'analyzer' })).toBe('noMonthSelected');
    expect(validateSelection({ analyzerIds: [], buildingId: null, period: '2026-08', scope: 'building' })).toBe('noBuildingSelected');
    expect(validateSelection({ analyzerIds: ['a'], buildingId: 'b', period: '2026-8', scope: 'analyzer' })).toBe('invalidMonthFormat');
    expect(validateSelection({ analyzerIds: ['a'], buildingId: 'b', period: '2026-08', scope: 'analyzer' })).toBeNull();
  });

  it('maps every R113 code the API can answer with', () => {
    expect(messageKeyForErrorCode('tariff_not_found')).toBe('noTariffForBuilding');
    expect(messageKeyForErrorCode('no_consumption_data')).toBe('noConsumptionData');
    expect(messageKeyForErrorCode('unresolved_anomaly')).toBe('unresolvedAnomaly');
    expect(messageKeyForErrorCode('period_not_closed')).toBe('periodNotClosed');
    // An unknown code must still say something, never an empty toast.
    expect(messageKeyForErrorCode('something_new')).toBe('generationFailed');
    expect(messageKeyForErrorCode(undefined)).toBe('generationFailed');
  });
});
```

- [ ] **Step 2: Run it and watch it fail**

```bash
cd web && pnpm test --maxWorkers=2 -- src/features/bills
```

Expected: FAIL — module not found.

- [ ] **Step 3: Implement `generation-status.ts`**

Pure functions only — no React, no fetch. `validateSelection` checks in the order §7.10 lists the
messages; `messageKeyForErrorCode` is a record with a default of `generationFailed`.

- [ ] **Step 4: Write the failing container tests**

`web/src/features/bills/bills-page.test.tsx`, with `mockApi` exactly as
`features/alarms/alarms-page.test.tsx` uses it:

```tsx
it('shows the month totals as the sum of the rows it renders', async () => {
  mockApi({ 'get:/api/v1/bills/dashboard': dashboardFixture });
  render(<BillsPage />);
  expect(await screen.findByRole('row', { name: /Toplam/ })).toHaveTextContent('510,00');
});

it('says the plant section has no data source yet instead of showing an empty table', async () => {
  mockApi({ 'get:/api/v1/bills/dashboard': { ...dashboardFixture, plants: { available: false, reason: 'no_plant_production_source' } } });
  render(<BillsPage />);
  expect(await screen.findByText(/santral üretimi/i)).toBeInTheDocument();
  expect(screen.queryByRole('table', { name: /santral/i })).not.toBeInTheDocument();
});

it('shows the divergence note when a building invoice differs from its rows', async () => {
  ...
  expect(await screen.findByRole('note')).toHaveTextContent(/bina faturası/i);
});

it('offers no generation card to a principal without bills.compute', async () => {
  renderWithSession(<BillsPage />, { permissions: ['bills.read'] });
  expect(screen.queryByRole('button', { name: /fatura oluştur/i })).not.toBeInTheDocument();
});

it('tells the operator which data condition stopped the generation', async () => {
  mockApi({
    'post:/api/v1/bills/compute': { job_ids: ['billing.generate:analyzer:x:2026-08'] },
    'get:/api/v1/jobs/{id}': { id: 'billing.generate:analyzer:x:2026-08', type: 'billing.generate', status: 'failed', error_code: 'tariff_not_found' },
  });
  ...
  expect(await screen.findByRole('alert')).toHaveTextContent(/tarife bulunamadı/i);
});

it('renders the empty state with a retry when the month has no bills', async () => {
  mockApi({ 'get:/api/v1/bills/dashboard': { period: '2026-08', buildings: [], netting: [], plants: { available: false } } });
  render(<BillsPage />);
  expect(await screen.findByRole('button', { name: /yeniden dene/i })).toBeInTheDocument();
});
```

- [ ] **Step 5: Build the screen**

- `bills-page.tsx`: `PageHeader`, one `MonthPicker` (R251), the dashboard query (`enabled` in the
  **options** argument, R199), `DashboardTableView`, `NettingSummaryView`, and — behind
  `can('bills.compute')` — `GenerateCardView`. Loading uses `Skeleton`, the empty month uses
  `EmptyState` with a retry that calls `refetch`.
- `dashboard-table.tsx`: the §7.10/R250 columns with a per-building subtotal row and a month total
  row, a PDF action per row and an Excel action for PTF-priced rows, plus "hepsini indir" wired to
  `GET /bills/dashboard/export`.
- `netting-summary.tsx`: `StatTile`s for consumption, production, net, invoice and efficiency (the
  tile is omitted, not zeroed, when `efficiency_pct` is absent), the `net_status` badge, and the
  plants section's explicit unavailable state (R235) naming F9.
- `generate-card.tsx`: `MultiSelect` over analyzers showing installation number, customer name and
  province/district; the building context fixed for a building-scoped principal; three actions
  (analyzer bill, building bill, company bill) each running compute → watch → download.

Design: flat `surface` cards with a 1 px border, no hover lift, numbers in the `type-data`
tabular-figures utility, status as colour **and** icon **and** text (07 §2.4, OVERRIDES).

- [ ] **Step 6: Stories, i18n, and the full web gate**

Stories are data-free and own their opener (R196). Add `bills.json` in both locales with camelCase
keys, register the namespace in `messages/index.ts`, then:

```bash
cd web && pnpm lint && pnpm typecheck && pnpm check:api && pnpm check:i18n-parity && pnpm check:contrast && pnpm test --maxWorkers=2
```

Expected: all green; i18n parity reports one more namespace.

- [ ] **Step 7: Commit**

```bash
git add web/src/app/ekorm/bills web/src/features/bills web/messages
git commit -m "feat(f8a): task 10 — the Bills screen: dashboard, netting and ad-hoc generation

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 8: Prove the guards red**

Back up `netting-summary.tsx`; render `0` instead of omitting the efficiency tile — the
efficiency test must fail. Back up `bills-page.tsx`; drop the `can('bills.compute')` guard — the
permission test must fail. Restore each, re-run, expect PASS.

---

## Task 11: Web — the Tariffs screen: the form and the version history

**Files:**
- Create: `web/src/app/ekorm/tariffs/page.tsx`
- Create: `web/src/features/tariffs/tariffs-page.tsx` + `.test.tsx`
- Create: `web/src/features/tariffs/tariff-form.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/tariffs/tariff-draft.ts` + `.test.ts`
- Create: `web/src/features/tariffs/tariff-history.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/tariffs/_fixture.ts`
- Create: `web/messages/tr/tariffs.json`, `web/messages/en/tariffs.json`

**Interfaces:**
- Consumes: `$api.useQuery('get', '/api/v1/tariffs')`, `'/api/v1/tariffs/{id}'`;
  `useApiMutation` for `post/patch/delete /api/v1/tariffs{,/{id}}`; `Tabs`, `Field`,
  `NumberInput`, `Select`, `Switch`, `DatePicker`, `FormErrorSummary` from `@/components/ui`.
- Produces:

```ts
// tariff-draft.ts
export type TariffDraft = { /* every §7.11 field, prices as strings (R248) */ };
export function emptyDraft(): TariffDraft;
export function draftFrom(t: Tariff): TariffDraft;
/** Clears the fields the abandoned mode owns, so a PTF tariff can never carry a fixed T1 price. */
export function applyMode(draft: TariffDraft, change: Partial<Pick<TariffDraft, 'usePtfYekdem' | 'priceType' | 'term'>>): TariffDraft;
export function toTariffRequest(draft: TariffDraft): TariffFieldsBody;
/** Server field code → message key, for `tariff.Validate`'s codes. */
export function messageForFieldCode(field: string, code: string): string;
```

- [ ] **Step 1: Write the failing draft tests**

```ts
it('reveals the KBK fields and drops the fixed prices when PTF+YEKDEM is switched on', () => {
  const d = applyMode({ ...emptyDraft(), singleTimePrice: '2.35', t1Price: '1.9' }, { usePtfYekdem: true });
  expect(d.usePtfYekdem).toBe(true);
  expect(d.singleTimePrice).toBe('');
  expect(d.t1Price).toBe('');
});

it('drops the KBK fields when PTF+YEKDEM is switched off', () => {
  const d = applyMode({ ...emptyDraft(), usePtfYekdem: true, kbkEnergy: '1.08' }, { usePtfYekdem: false });
  expect(d.kbkEnergy).toBe('');
});

it('clears contracted power and the power price when the term becomes monomial', () => {
  // The server refuses them with must_be_empty (R125), so the form must not send them.
  const d = applyMode({ ...emptyDraft(), term: 'binomial', contractedPowerKw: '250', powerUnitPrice: '40' }, { term: 'monomial' });
  expect(d.contractedPowerKw).toBe('');
  expect(d.powerUnitPrice).toBe('');
});

it('omits blank optional prices instead of sending "0"', () => {
  const body = toTariffRequest({ ...emptyDraft(), overusePrice: '' });
  expect(body).not.toHaveProperty('overuse_price');
});

it('maps every code tariff.Validate can return', () => {
  expect(messageForFieldCode('kbk_energy', 'required')).toBe('errors.kbkEnergyRequired');
  expect(messageForFieldCode('currency', 'must_be_try')).toBe('errors.currencyMustBeTry');
  expect(messageForFieldCode('contracted_power_kw', 'must_be_empty')).toBe('errors.contractedPowerMustBeEmpty');
  expect(messageForFieldCode('taxes[2].rate', 'negative')).toBe('errors.rateNegative');
});
```

- [ ] **Step 2: Run and watch fail, then implement `tariff-draft.ts`**

Pure module; prices stay strings end to end (R248). `messageForFieldCode` normalises the indexed
field names (`taxes[2].rate` → `taxes.rate`) before the lookup.

- [ ] **Step 3: Write the failing form tests**

```tsx
it('refuses to save a PTF tariff without an energy KBK and says which field', async () => {
  // 09 §F8's acceptance criterion, on the client as well as the server.
  render(<TariffFormView draft={{ ...emptyDraft(), usePtfYekdem: true }} onSubmit={onSubmit} />);
  await user.click(screen.getByRole('button', { name: /kaydet/i }));
  expect(onSubmit).not.toHaveBeenCalled();
  expect(await screen.findByRole('alert')).toHaveTextContent(/enerji kbk/i);
});

it('shows the KBK fieldset only in PTF+YEKDEM mode', async () => {
  render(<TariffFormView draft={emptyDraft()} onSubmit={onSubmit} />);
  expect(screen.queryByLabelText(/enerji kbk/i)).not.toBeInTheDocument();
  await user.click(screen.getByRole('switch', { name: /ptf/i }));
  expect(screen.getByLabelText(/enerji kbk/i)).toBeInTheDocument();
});

it('renders the server field errors on their own fields', async () => {
  // 422 {"fields": {"kbk_distribution_cost_tl_per_kwh": "required"}}
  ...
  expect(screen.getByLabelText(/dağıtım.*kbk/i)).toHaveAccessibleDescription(/zorunlu/i);
});

it('is read-only for a company-readonly-admin', () => {
  renderWithSession(<TariffsPage />, { permissions: ['tariffs.read'] });
  expect(screen.queryByRole('button', { name: /kaydet/i })).not.toBeInTheDocument();
});
```

- [ ] **Step 4: Build the form and the history**

`tariff-form.tsx` renders the §7.11 fieldsets in the spec's order — identity, classification,
prices (mode-dependent), power, reactive, distribution, generation usage, VAT and other taxes, the
additional-tax list, and the PTF fieldset with the manual YEKDEM table and its enable switch. Every
field is a labelled `Field` with `FormErrorSummary` at the top on submit (07 §9, §11).

`tariff-history.tsx` lists the building's versions newest first with effective date, name, price
type, term, a PTF badge, and edit/delete actions gated on `tariffs.edit`. The delete dialog says
plainly that historical invoices keep the version they were priced with (R242's reasoning).

- [ ] **Step 5: Web gate and commit**

```bash
cd web && pnpm lint && pnpm typecheck && pnpm check:i18n-parity && pnpm test --maxWorkers=2
git add web/src/app/ekorm/tariffs web/src/features/tariffs web/messages
git commit -m "feat(f8a): task 11 — the tariff form with its mode switches and version history

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 6: Prove the guard red**

Back up `tariff-draft.ts`; make `applyMode` keep the abandoned mode's fields — the two clearing
tests must fail. Restore, re-run, expect PASS.

---

## Task 12: Web — templates and bulk assignment tabs

**Files:**
- Create: `web/src/features/tariffs/templates-tab.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/tariffs/bulk-tab.tsx` + `.test.tsx` + `.stories.tsx`
- Modify: `web/src/features/tariffs/tariffs-page.tsx` (two more tabs)

**Interfaces:**
- Consumes: `/api/v1/tariff-templates` (list/create/patch/delete/apply),
  `/api/v1/buildings/bulk-tariff/current`, `/api/v1/buildings/bulk-tariff`,
  `/api/v1/buildings/bulk-tariff/history`; the `TariffFormView` of Task 11 inside the template
  dialog (one form, two callers — never a second copy).

- [ ] **Step 1: Write the failing tests**

```tsx
it('applies a template to the buildings the operator checked, and to no others', async () => {
  ...
  expect(apply).toHaveBeenCalledWith(expect.objectContaining({ body: { building_ids: [b1], effective_from: '2026-09-01' } }));
});

it('refuses to apply with nothing selected', async () => {
  await user.click(screen.getByRole('button', { name: /uygula/i }));
  expect(apply).not.toHaveBeenCalled();
  expect(await screen.findByRole('alert')).toHaveTextContent(/en az bir bina/i);
});

it('shows buildings that have no tariff at all in the current-state table', async () => {
  mockApi({ 'get:/api/v1/buildings/bulk-tariff/current': { items: [{ building_id: 'b', building_name: 'Bina', tariff_id: null }], total: 1 } });
  expect(await screen.findByRole('cell', { name: /tarife yok/i })).toBeInTheDocument();
});

it('lists the assignment history newest first with the building count', async () => {
  ...
});
```

- [ ] **Step 2–4: Implement, gate, commit**

Writes are gated on `tariffs.edit`, reads on `tariffs.templates.read` / `tariffs.bulk.read`; the
select-all control announces the count through the `LiveAnnouncer` (07 §11).

```bash
cd web && pnpm test --maxWorkers=2 -- src/features/tariffs && pnpm lint && pnpm typecheck
git add web/src/features/tariffs web/messages
git commit -m "feat(f8a): task 12 — template and bulk-assignment tabs

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 5: Prove the guard red**

Remove the empty-selection guard — `refuses to apply with nothing selected` must fail. Restore.

---

## Task 13: Web — the icmal import wizard

**Files:**
- Create: `web/src/features/tariffs/icmal-tab.tsx` + `.test.tsx` + `.stories.tsx`
- Modify: `web/src/features/tariffs/tariffs-page.tsx`

**Interfaces:**
- Consumes: `FileUpload`, `Stepper`, `Table`, `Checkbox`, `Badge` from `@/components/ui`;
  `POST /api/v1/icmal-imports` (multipart), `GET /api/v1/icmal-imports/{id}`,
  `POST /api/v1/icmal-imports/{id}/apply`.

- [ ] **Step 1: Write the failing tests**

```tsx
it('writes nothing until the operator confirms a building', async () => {
  // 09 §F8's acceptance criterion, on the client.
  ...
  expect(screen.getByRole('button', { name: /onayla ve uygula/i })).toBeDisabled();
  await user.click(screen.getByRole('checkbox', { name: /Bina A/ }));
  expect(screen.getByRole('button', { name: /onayla ve uygula/i })).toBeEnabled();
  expect(applyMutation).not.toHaveBeenCalled();
});

it('shows each coefficient with its sample count, stability and warnings', async () => {
  ...
  expect(await screen.findByText(/6 dönem/i)).toBeInTheDocument();
  expect(screen.getByText(/kararsız/i)).toBeInTheDocument();
  expect(screen.getByRole('status')).toHaveTextContent(/güç fiyatı dönemler arasında kararsız/i);
});

it('lists unmatched ETSO codes instead of silently dropping them', async () => {
  mockApi({ 'post:/api/v1/icmal-imports': { ...importFixture, unmatched: ['40Z0000000123A'] } });
  expect(await screen.findByText('40Z0000000123A')).toBeInTheDocument();
});

it('says what went wrong when the file cannot be read', async () => {
  mockApi({ 'post:/api/v1/icmal-imports': { status: 422, body: { error: { code: 'validation_failed', fields: { file: ['unreadable'] } } } } });
  expect(await screen.findByRole('alert')).toHaveTextContent(/dosya okunamadı/i);
});
```

- [ ] **Step 2–4: Implement, gate, commit**

Three `Stepper` steps — *Yükle*, *İncele*, *Onayla*. Step 2 is one row per analysed ETSO code with
its matched building, its coefficients (value, sample count, stability badge, back-calculation
error) and its warnings; a coefficient outside 02 §8.4's 2 % target carries a `warning` badge with
text, never a colour alone. The confirm step sends only the checked rows.

```bash
cd web && pnpm test --maxWorkers=2 -- src/features/tariffs && pnpm lint && pnpm typecheck
git add web/src/features/tariffs web/messages
git commit -m "feat(f8a): task 13 — the icmal review-and-confirm wizard

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 5: Prove the guard red**

Enable the confirm button unconditionally — the first test must fail. Restore.

---

## Task 14: Web — solar tariffs and the admin default catalogue

**Files:**
- Create: `web/src/features/tariffs/solar-tab.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/tariffs/defaults-tab.tsx` + `.test.tsx` + `.stories.tsx`
- Modify: `web/src/features/tariffs/tariffs-page.tsx` (final tab set and its permissions)

- [ ] **Step 1: Write the failing tests**

```tsx
it('asks for a plant before it shows any tariff history', async () => {
  render(<SolarTabView />);
  expect(await screen.findByText(/santral seçin/i)).toBeInTheDocument();
});

it('offers an add-first-tariff action when the plant has none', async () => {
  mockApi({ 'get:/api/v1/solar-tariffs': { items: [], total: 0 } });
  expect(await screen.findByRole('button', { name: /ilk tarifeyi ekle/i })).toBeInTheDocument();
});

it('renders effective dates as DD-MM-YYYY and sends them as ISO', async () => {
  // §7.11 asks for DD-MM-YYYY on screen; 05 §1 requires ISO on the wire.
  ...
  expect(screen.getByRole('cell', { name: '01-09-2026' })).toBeInTheDocument();
  expect(create).toHaveBeenCalledWith(expect.objectContaining({ body: expect.objectContaining({ effective_from: '2026-09-01' }) }));
});

it('hides the default-tariff tab from a company admin', () => {
  renderWithSession(<TariffsPage />, { permissions: ['tariffs.read', 'tariffs.edit'] });
  expect(screen.queryByRole('tab', { name: /varsayılan tarifeler/i })).not.toBeInTheDocument();
});
```

- [ ] **Step 2–4: Implement, gate, commit**

The tab set: *Bina tarifeleri* (`tariffs.read`), *Şablonlar* (`tariffs.templates.read`), *Toplu
atama* (`tariffs.bulk.read`), *İcmal içe aktarma* (`tariffs.icmal`), *Solar tarifeler*
(`solar_tariffs.read`), *Varsayılan tarifeler* (`tariffs.defaults`). A tab whose permission is
missing is not rendered at all.

```bash
cd web && pnpm lint && pnpm typecheck && pnpm check:i18n-parity && pnpm check:contrast && pnpm test --maxWorkers=2
git add web/src/features/tariffs web/messages
git commit -m "feat(f8a): task 14 — solar tariffs and the admin default catalogue

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 5: Prove the guard red**

Drop the `tariffs.defaults` gate — the hidden-tab test must fail. Restore.

---

## Task 15: Seed, e2e, accessibility and the F8a acceptance run

**Files:**
- Modify: `internal/seed/fixtures.go` (bills for the dashboard month, a template, a bulk
  assignment, an analysed icmal import, a solar tariff, two national-schedule rows)
- Create: `web/tests/e2e/bills.spec.ts`, `web/tests/e2e/tariffs.spec.ts`
- Modify: `web/tests/e2e/responsive.spec.ts` (two `SCREENS` lines)

- [ ] **Step 1: Extend the seed**

The e2e company gets, for one closed month: three analyzer bills in two buildings, one
building-scope bill that **differs** from its analyzer sum (so the R234 note is exercised for
real), one company bill, one tariff template, one bulk assignment row, one analysed icmal import
with two matched buildings and one unmatched ETSO code, one plant with a solar tariff, and two
national-schedule entries. Every figure is a real computed bill, not a literal: run the billing
service over seeded readings exactly as the F4 seed already does.

- [ ] **Step 2: Write the e2e specs**

`bills.spec.ts`: a company admin opens `/ekorm/bills`, picks the seeded month, sees the buildings
table with its total row, sees the netting summary, sees the plants section's unavailable note,
downloads the dashboard export (assert the response is an attachment), and runs an ad-hoc
generation that ends in a downloadable PDF. A building admin sees only their own building.

`tariffs.spec.ts`: create a tariff, switch it to PTF+YEKDEM and watch the save be refused without
an energy KBK, then fill it and save; see the new version at the top of the history; create a
template and apply it to two buildings; open the icmal tab, upload the seeded workbook, confirm one
building and watch exactly one new version appear; and confirm the default-tariff tab is absent for
a company admin.

- [ ] **Step 3: Add the two responsive lines**

```ts
  ['bills', '/ekorm/bills'],
  ['tariffs', '/ekorm/tariffs'],
```

- [ ] **Step 4: Run the accessibility chunk**

```bash
cd web && pnpm storybook:build
STORIES='Features/Bills,Features/Tariffs' pnpm test:a11y --workers=1
```

Expected: all pass. Fix what it names — colour-only status, a missing label, a control under 44 px
at coarse pointer — rather than suppressing a rule.

- [ ] **Step 5: Run the whole acceptance set**

```bash
export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB

# Go, package by package, with the race detector
go test ./internal/domain/billing/... ./internal/domain/tariff/... ./internal/service/billing/... \
        ./internal/service/tariff/... ./internal/service/jobs/... ./internal/render/... \
        ./internal/auth/... ./internal/api/v1/... ./internal/store/... ./internal/seed/... ./internal/arch/... -race -count=1

# Integration, on the shared containers
EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' \
EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
go test ./internal/api/v1/... ./internal/service/billing/... ./internal/service/tariff/... \
        ./internal/store/postgres/... ./internal/seed/... -tags=integration -count=1 -parallel 4

golangci-lint run --concurrency 2 --build-tags=integration ./internal/...
make check-generate && make openapi   # both must leave the tree clean

# 09 §F8's own verification block, for the parts F8a owns. Two deviations,
# both deliberate: the spec names ./internal/pdf/... and ./internal/xlsx/...,
# which F4 built as ./internal/render/{invoicepdf,hourlyxlsx} — run the
# packages that exist, not a vacuous green. The report lines
# (domain/report, service/report, reports.spec.ts) belong to F8b and are NOT
# run here; claiming them green would be a lie.
go test ./internal/domain/billing/... -v
go test ./internal/render/... -v
cd web && pnpm exec playwright test tests/e2e/bills.spec.ts tests/e2e/tariffs.spec.ts

cd web && pnpm lint && pnpm typecheck && pnpm check:api && pnpm check:i18n-parity && pnpm check:contrast \
  && pnpm test --maxWorkers=2 && pnpm audit --audit-level=high && pnpm build
free -m && ps -eo rss,comm | grep java    # before Playwright
E2E_API_PORT=18082 PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1
```

- [ ] **Step 6: Commit and write the handoff**

```bash
git add internal/seed web/tests
git commit -m "test(f8a): task 15 — seed, e2e specs and the F8a acceptance run

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

Then rewrite `HANDOFF_NEXT_SESSION.md` for **F8b (reports)** and hand the user a paste-ready
Turkish prompt.

---

## Acceptance map — 09 §F8 (the F8a half)

| Criterion | Where it is proved |
|---|---|
| The invoice dashboard totals reconcile with the sum of its rows, verified in a test | Task 2 `TestDashboardTotalsFollowTheRowsShown`; Task 3 HTTP assertion; Task 10 container test |
| Every validation message in §7.10 is reachable and displayed | Task 10 `generation-status.test.ts` (all seven) + the container test for the failed job; Task 5 supplies the codes |
| A tariff switched to PTF+YEKDEM mode reveals the KBK fields and refuses to save without an energy KBK | Task 11 form tests; `internal/domain/tariff` already enforces it server-side (`validate_test.go`) |
| The icmal import never writes a coefficient without explicit user confirmation — tested | Task 7 `TestIcmalUploadAnalysesWithoutWritingATariff` + `TestIcmalApplyWritesOneVersionPerConfirmedBuilding`; Task 13 client test |
| A recomputed invoice supersedes rather than deletes its predecessor, and the old PDF stays downloadable | Task 3 `TestSupersededBillPDFStaysDownloadable` (F4 owns the supersede itself) |
| Monthly/yearly report figures, report PDF/Excel sections, report e-mail | **F8b** — explicitly out of this plan (D-2/D-3) |

---

## Open questions for the product owner (defaults ship)

- **Q-D1** R234: when a building invoice differs from the sum of its analyzer invoices, the
  dashboard shows both and says so. Should the building invoice instead be the headline figure,
  with the analyzer rows presented as its breakdown?
- **Q-D2** R235: the solar-plants section of the dashboard stays empty until F9. Is a plant ↔
  analyzer link wanted in F9, or do plant figures come from iSolar only?
- **Q-D3** R242: legacy's "clear tariff history" is not rebuilt. Is there an operational need to
  remove old tariff versions, given that issued bills reference them?
- **Q-D4** R243: admin default tariffs are the national schedule rows. Legacy also had a *solar*
  default catalogue with no table behind it — drop it for good?
- **Q-D5** R251: one month control instead of the spec's year + month pair — confirm.
- **Q-D6** R253/Review Focus 1: a company with bills in two currencies gets two netting summaries.
  Should the screen instead refuse to mix and ask the operator to pick a currency?
- **Q-D7** Q-C8 from F7 is still open: should `consumption.refresh` get a period-taking trigger?
  It is not part of F8a's scope and is not built here.

## Binding rulings added while planning

- **R253** **Currencies are never added together.** R127 forces TRY on PTF tariffs, but a fixed
  tariff may be USD or EUR, so one company's month can hold two currencies. The dashboard groups
  every total by currency and returns one netting summary per currency present; nothing sums across
  them, and no conversion is invented (R127: there is no FX in this system).
