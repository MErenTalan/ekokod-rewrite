# F10a — Carbon Footprint Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this
> plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This phase runs **inline in
> one session**: no parallel agents, no subagents, no Workflow (the user's standing rule).

**Goal:** Eko-CM's server side as a first-class module:
- the pure carbon domain: catalogue, GHG/ISO mapping, emission, pro-rata, grouping and 02 §9.4 figures;
- the per-company factor catalogue: override and reset;
- building activity selection;
- activities CRUD with server-side emission and an approval workflow;
- the overview read model;
- `carbon.daily_accrual`;
- GHG and ISO 14064 reports with PDF output;
- every 05 §12 route;
- `ekokod seed --only emission-factors --verify`.

The screens (§7.16) are **F10b**, a separate plan written after this one lands.

**Architecture:** `internal/domain/carbon` is pure. It owns:
- the 8 main / 18 sub-category tree;
- legacy `GHG_ISO_MAPPING`, verbatim;
- `Emission`, `ProRata`, the report grouping and the §9.4 block.

`internal/service/carbon` loads and saves through F1's `store.CarbonRepository`, extended in Task 2.
It resolves the **effective** catalogue: platform rows, shadowed by the company's own rows with the same key.

The accrual job lists companies with `AdminTenantRepository` and works per company under
`store.SystemScope`. It reads the daily consumption aggregate (`ActiveConsumption`, `ActiveGeneration`).

Reports snapshot their figures into `carbon_reports.payload` and render the PDF on demand. The PDF goes
through a new `reportpdf.RenderView(reportview.Document, …)`, so the same fpdf pipeline and fonts as
F8b are used.

**Tech Stack:** Go 1.27 (pgx/sqlc, shopspring/decimal, asynq, swaggest OpenAPI, go-pdf/fpdf), Postgres 17 + TimescaleDB.

**Spec:**
- `docs/rewrite/09-implementation-plan.md` §F10 (scope, acceptance, verification).
- `docs/rewrite/01-project-context.md` §7.16.
- `docs/rewrite/02-domain-rules.md` §9.
- `docs/rewrite/05-api-contract.md` §12.
- `docs/rewrite/04-data-model.md` §9.
- `docs/rewrite/10-removed-behaviours.md` items 29, 30.
- Legacy, read while planning:
  - `src/app/api/carbon-footprint/{route.ts,emission-factors/route.ts,reports/route.ts,selectionActivities/route.ts}`
  - `src/app/api/cron/daily-carbon/route.ts`
  - `src/app/components/carbon-footprint/{constants.ts,ActivityEntryModal.tsx,reporting/Reporting.tsx}`
  - `src/utils/types.ts` (`CATEGORY_MAPPING`, `GHG_ISO_MAPPING`)
  - `src/utils/db/models/CarbonActivity.ts`
- Predecessors:
  - F9 `2026-09-23-f9-solar-renewable-financial.md` (R276–R299);
  - F8b (R254–R275; **R262** is the grid-factor lookup, company override first);
  - F1 (the carbon schema and `CarbonRepository`, with 182 + 4 platform factors seeded).

## Global Constraints

- **Branch/worktree:** `phase/f10-carbon` in `/home/personal/ekokod-f10-phase`, from `phase/f9-solar`.
  Nothing is pushed, `main` is untouched, and merging is the user's call.
- **Shell prefix:** start every command with
  `export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
- **Docker:** wrap Docker commands in `sg docker -c "…"`. **Never run `make test` or `go test ./...`.**
- **Per-task gate:**
  - the **whole touched package** with `-race`;
  - integration tests for touched store/service/api packages;
  - `golangci-lint run --concurrency 2 --build-tags=integration <pkgs>`.
- **Integration env:** `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <pkg> -tags=integration -count=1 -parallel 4`.
  - Run `internal/scheduler`, `internal/worker` and `internal/job` one at a time.
  - **If the Docker daemon is still hung** (see HANDOFF "Docker hang"), record integration runs as PENDING in the ledger. Never claim them green.
- **Numbers:**
  - Energy, factors and emissions are `decimal`, and cross the API as strings.
  - Emissions are **kg CO₂e**. Stored values have 6 dp.
  - Tonnes appear only in the §9.4 block (`_t` fields).
- **Scope:** 404 outside it, never 403. A building admin sees only their buildings' activities, selections and reports.
- **Dates:** Istanbul calendar dates everywhere. `period_start`/`period_end` are dates, inclusive.
- **Code style:**
  - Comments give the WHY in 1–3 lines and cite ruling ids.
  - Commit when green, then prove each guard red with your own mutation, restoring from a file backup.
  - Commit trailer: `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.
- **Generation:** `make check-generate` needs a clean tree. `make openapi` regenerates `openapi.json` and `web/src/lib/api/schema.d.ts`.
- Helper names below are indicative. Use the helpers the joined test file already has.

## Open questions (defaults shipped)

The user is away; each question below was answered with the recommended option and ledgered.

| # | Question | Default shipped | Cost if wrong |
|---|---|---|---|
| Q-F1 | 02 §9.2's table says waste → ISO category 4, but legacy `GHG_ISO_MAPPING` says **category_6**. The spec says the legacy mapping is authoritative. | **category_6** (legacy, verbatim) | One mapping row, plus a re-save of affected activities |
| Q-F2 | Seeded passenger-transport factors carry `scope_3` on some rows, while the mapping says scope 1. | **The mapping wins**: scope and ISO come from the sub-category (R301) | Display only; the factor rows are unchanged |
| Q-F3 | How is automated rooftop generation (meter export) accounted? | **Zero emission.** `details.avoided_kgco2e` = kWh × grid factor. §9.4 reports show the reduction explicitly (R311) | Inventory totals would differ if the owner wants netting |
| Q-F4 | Do pending and rejected activities count? | **Rejected never counts.** Pending counts, and reports state how many are pending (R309) | A report may include unapproved figures, and says so |
| Q-F5 | What does an empty selection mean? | **None selected**. Data entry shows the empty state (spec §7.16). This differs from legacy's "all by default" | A user must declare once |
| Q-F6 | Is the report PDF stored? | **Rendered on demand** from the stored payload snapshot; `pdf_path` stays null (R312) | CPU per download; the figures never change |
| Q-F7 | Which records are editable? | Automated records are read-only except for status. Any edit of a manual record resets it to **pending** (R306) | One re-approval per edit |

## Binding rulings (R300–R319)

| Id | Rule |
|---|---|
| **R300** | **Catalogue.** 8 main categories (`cat_stationary`, `cat_mobility`, `cat_logistics`, `cat_gas_emissions`, `cat_electricity`, `cat_external_energy`, `cat_products`, `cat_waste`) and 18 sub-categories, with keys and membership exactly as legacy `CATEGORY_MAPPING`. Order: legacy order. |
| **R301** | **Mapping (Q-F1, Q-F2).** Scope and ISO category come only from the sub-category, via legacy `GHG_ISO_MAPPING`, verbatim. A factor's own `scope`/`iso_category` is display metadata. |
| **R302** | **Effective catalogue.** The platform rows (company_id null) plus the company's own rows. A company row with the same key **shadows** the platform row. Each list item carries `overridden` (a company row exists) and `platform_base_factor` (null when there is no platform row). Only `status` null/`active` factors can be used for new entries. |
| **R303** | **Override.** `PATCH /carbon/emission-factors/{id}` takes `{base_factor, source?, source_year?, source_url?}`.<br>`id` may be a platform factor or the company's own. On a platform id, the company gets a shadow copy of every descriptive field and the conversions, plus the new value. `base_factor` must be > 0 and ≤ 1 000 000; `source_year` must be 1990–2100.<br>The change is audited. Historical activities keep their stored values. |
| **R304** | **Reset.** `POST /carbon/emission-factors/reset` deletes every company-owned factor; conversions cascade. Activities keep their snapshot, because migration 00019 makes `carbon_activities.factor_id` `on delete set null`. Audited. |
| **R305** | **Create activity.** `{building_id, sub_category, factor_key, quantity, unit, period_start, period_end, description?, details?}`.<br>The building must be in scope, else 404. `sub_category` must be one of R300's keys **and selected** for the building, else 422 `not_selected`. `factor_key` must resolve in the effective catalogue, be usable, and list `sub_category` in `sub_categories`, else 422 `factor_key: not_found`/`not_applicable`. `unit` must be the factor's `base_unit` (multiplier 1) or one of its conversions, else 422 `unit: not_supported`.<br>Limits: `0 < quantity ≤ 1e12`; `period_start ≤ period_end`, span ≤ 366 days, `period_end` ≤ today (Istanbul); `description` ≤ 500; `details` is a flat object of ≤ 10 string values of ≤ 100 chars.<br>Stored: `main_category` (from R300), `activity_type = sub_category`, factor id, key and value, the multiplier, and `emission_kgco2e = round6(quantity × multiplier × base_factor)`. `details` gains `factor_source` and `factor_source_year` (snapshot), plus scope/ISO (R301). `status = pending`, `created_by` = the user. |
| **R306** | **Update/delete.** PATCH takes the same body minus `building_id`, recomputes everything and resets status to pending. An automated record answers 409 `automated` to PATCH and DELETE. |
| **R307** | **Status.** `POST /carbon/activities/{id}/status {status: approved|rejected}` from any status. Idempotent, audited. |
| **R308** | **Pro-rata.** An activity's share of a window [from, to] is `emission × overlap_days / span_days`, with inclusive Istanbul dates and the final sum rounded to 6 dp. The same rule applies to quantities. |
| **R309** | **What counts (Q-F4).** Rejected activities never count. Pending ones count, and every aggregate reports `pending_count`. |
| **R310** | **Overview.** `GET /carbon/overview?building_id&year` (`year` defaults to the current Istanbul year). All figures are R308 shares over the year of R309's activities overlapping it:<br>• `total_kgco2e`, `activity_count`, and `registered_count` (the building's selected sub-categories);<br>• `highest_source {sub_category, kgco2e}` (null when there are none);<br>• `by_category` (8 main categories, zeros included) and `by_scope` (3);<br>• `monthly` (12 months: `current`, and `previous` from the same rule over year−1);<br>• `recent` (5 newest by `created_at`), `pending_count`.<br>`total = Σ by_category = Σ monthly.current` (tested). |
| **R311** | **`carbon.daily_accrual` (Q-F3).** Payload `{day?: "YYYY-MM-DD"}`; empty means yesterday (Istanbul). A given day must be in the past and ≤ 400 days back. For every live company, every live building, and its analyzers' daily aggregate for that day:<br>• If any `ActiveConsumption` is non-null, upsert `sub_grid_electricity`: quantity = Σ, unit `kWh`, factor = R262's grid factor, emission = kWh × factor, status approved, description "Günlük şebeke tüketimi (otomatik)".<br>• If Σ `ActiveGeneration` > 0, upsert `sub_elec_generation`: emission **0**, factor = grid factor, `details.avoided_kgco2e` = kWh × factor, `details.source = "meter_export"`.<br>• With no grid factor, grid rows are skipped and the company gets an ops message.<br>Idempotency is F1's partial unique index plus `UpsertAutomatedActivity`. The run is journaled in `job_runs`. The cron is `config.Schedule.Carbon` (default `30 4 * * *`). |
| **R312** | **Reports (Q-F6).** `POST /carbon/reports {building_id, report_type: ghg|iso, from, to, name?}`: span ≤ 366 days, `to` ≤ today, name ≤ 120, defaulting to `"GHG Protocol 2026-01-01 – 2026-06-30"` / `"ISO 14064 …"`.<br>The payload snapshot `CarbonReportPayload` holds:<br>• company name, building name and address, period, `generated_at`;<br>• `groups`: GHG → `scope_1..3`; ISO → `category_1..6`, zero groups included. Each group has a total and per-sub-category lines (sorted by key);<br>• `total_kgco2e`, `pending_count`;<br>• the §9.4 block `{consumption_kwh, generation_kwh, grid_factor, grid_factor_unit, grid_factor_source, grid_factor_year, consumption_emission_t, production_reduction_t, net_emission_t}`, built from R308 shares of the automated grid/generation activities with the **current** grid factor. It is null when there is no automated data or no factor.<br>`GET /carbon/reports/{id}/pdf` renders the payload (tr or en by `Accept-Language`/`locale`). |
| **R313** | **Dated grid factor.** The grid factor is an `emission_factors` row with `source_year`, editable by R303. Every report states the value and its source year. |
| **R314** | **Permissions.** `carbon.read` = all roles (scope narrows it); `carbon.edit` = A CA (05 §12). Selection writes are A CA too. |
| **R315** | **Selection.** `GET/PUT /carbon/selected-activities?building_id`. The PUT body is `{activity_keys: []}`: each key must be one of R300's, else 422 `activity_keys: unknown`. Keys are deduplicated. The answer is the sorted list. |
| **R316** | **Catalogue route.** `GET /carbon/activity-catalogue` returns `[{key, subs:[{key, scope, iso_category}]}]`: static, from the domain. |
| **R317** | **Seed verify.** `ekokod seed --only emission-factors --verify` compares the platform rows with the embedded JSON (key set and `base_factor`). It prints `emission factors: N/N match` and exits non-zero listing missing, extra and changed keys. `--only` accepts `emission-factors` alone for now. |
| **R318** | **Migration 00019.** `carbon_activities.factor_id` references `emission_factors(id) on delete set null`. |
| **R319** | **Activity list.** `GET /carbon/activities?building_id&from&to&scope&status&type&automated&page_size&cursor`. `from`/`to` select **overlap**. Order: `period_start desc, created_at desc`. Paged like other lists (`kit.ResolvePage`). |

## Rule table: routes (05 §12)

| # | Method | Pattern | Permission | Answer |
|---|---|---|---|---|
| 1 | GET | `/carbon/overview` | carbon.read | `CarbonOverview` (R310) |
| 2 | GET | `/carbon/activity-catalogue` | carbon.read | `{items: CatalogueMain[]}` (R316) |
| 3 | GET | `/carbon/selected-activities` | carbon.read | `{activity_keys}` |
| 4 | PUT | `/carbon/selected-activities` | carbon.edit | `{activity_keys}` (R315) |
| 5 | GET | `/carbon/activities` | carbon.read | `Page[CarbonActivity]` (R319) |
| 6 | POST | `/carbon/activities` | carbon.edit | 201 `CarbonActivity` (R305) |
| 7 | PATCH | `/carbon/activities/{id}` | carbon.edit | `CarbonActivity` (R306) |
| 8 | DELETE | `/carbon/activities/{id}` | carbon.edit | 204 |
| 9 | POST | `/carbon/activities/{id}/status` | carbon.edit | `CarbonActivity` (R307) |
| 10 | GET | `/carbon/emission-factors` | carbon.read | `{items: EmissionFactorView[]}` (R302); filters `sub_category`, `main_category`, `q` |
| 11 | PATCH | `/carbon/emission-factors/{id}` | carbon.edit | `EmissionFactorView` (R303) |
| 12 | POST | `/carbon/emission-factors/reset` | carbon.edit | 204 (R304) |
| 13 | GET | `/carbon/reports` | carbon.read | `Page[CarbonReportSummary]` (`?building_id`) |
| 14 | POST | `/carbon/reports` | carbon.edit | 201 `CarbonReportSummary` (R312) |
| 15 | GET | `/carbon/reports/{id}/pdf` | carbon.read | `application/pdf` |

## Rule table: jobs

| Type | Payload | Task id | Watchable |
|---|---|---|---|
| `carbon.daily_accrual` | `{day?}` | `carbon.daily_accrual:<day>` (Retention 1 h, R192) | ops history (A CA) |

## File map

```
internal/domain/carbon/{catalogue.go,emission.go,aggregate.go,report.go} + tests       Task 1
internal/store/postgres/migrations/00019_carbon.sql, queries/carbon.sql, carbon.go,
  store/repository.go (DeleteCompanyFactors, Report, filter Overlap*)                  Task 2
internal/service/carbon/{service.go,factors.go,selection.go}                           Task 3
internal/service/carbon/{activities.go,overview.go}                                    Task 4
internal/service/carbon/accrual.go, internal/job/carbon.go, scheduler/worker/ops       Task 5
internal/service/carbon/reports.go, internal/render/carbonview/view.go,
  internal/render/reportpdf/pdf.go (RenderView)                                         Task 6
internal/api/v1/{handlers_carbon.go,dto/carbon.go}, routes, auth/roles.go, openapi     Task 7
internal/cli/seed.go (--only/--verify), internal/seed/carbon.go (e2e fixtures)         Task 8
```

## Review Focus

1. **A factor overridden after an activity was saved.** The activity's stored `emission_kgco2e`, `factor_value` and multiplier must not change. Also re-read after reset: `factor_id` becomes null and nothing else changes (Task 2 + 3 tests).
2. **An activity spanning a month or year boundary.** Pro-rata overlap must be exact, with no double counting across the overview's months or across two adjacent report windows (Task 1).
3. **A building admin with an activity/report/factor id of another building or company.** Answer 404, including `/pdf` and `status` (Task 7 HTTP matrix).
4. **The accrual re-run after late-arriving data.** It converges on one row with the new quantity, never two. The generation row is not created when export is 0 (Task 5).
5. **Unit choice.** A conversion unit multiplies, and the base unit multiplies by 1. A unit not on the factor is refused, and so is an archived factor (Task 3/4).

---

## Task 1: Pure carbon domain

**Files:** create `internal/domain/carbon/{catalogue.go,emission.go,aggregate.go,report.go}` with `_test.go` for each.

**Produces:**
```go
type Scope string   // "scope_1".."scope_3" (model.CarbonScope values)
type Sub struct{ Key, Main string; Scope model.CarbonScope; ISO string }
func Mains() []string                         // R300 order
func Subs(main string) []Sub
func SubByKey(key string) (Sub, bool)
func Emission(quantity, multiplier, factor decimal.Decimal) decimal.Decimal // round 6
type Dated struct{ Start, End time.Time; Value decimal.Decimal }           // dates, inclusive
func Share(d Dated, from, to time.Time) decimal.Decimal                      // R308, unrounded
type Record struct{ Sub string; Start, End time.Time; KgCO2e decimal.Decimal; Status model.CarbonStatus; Automated bool; Quantity decimal.Decimal }
type Overview struct{ Total decimal.Decimal; Count int; ByMain map[string]decimal.Decimal; ByScope map[model.CarbonScope]decimal.Decimal; Monthly [12]decimal.Decimal; Highest *SubTotal; Pending int }
type SubTotal struct{ Sub string; KgCO2e decimal.Decimal }
func BuildYear(records []Record, year int) Overview                         // R309 + R308, Istanbul months
type Group struct{ Key string; Total decimal.Decimal; Lines []SubTotal }
func Groups(records []Record, kind string /*ghg|iso*/, from, to time.Time) (groups []Group, total decimal.Decimal, pending int)
type Figures94 struct{ ConsumptionKwh, GenerationKwh, Factor, ConsumptionT, ReductionT, NetT decimal.Decimal }
func Report94(records []Record, from, to time.Time, factor decimal.Decimal) *Figures94 // nil without automated records
```

- [ ] **Failing tests (hand-checked fixtures):**
  - `TestMappingIsLegacyVerbatim`: all 18 subs; waste is `category_6`; passenger transport is scope_1/category_1.
  - `TestEveryMainCategoryEmission`: one fixture per main category, e.g. natural gas 1000 m³ × 1 × 2.06672 = 2066.72; diesel litres; freight tkm; the grid 1234.5 kWh × 0.469 = 578.9805; waste tonnes; each product per unit.
  - `TestShareInclusiveDays`: 1–31 Jan inside the window gives 100%; the window 16–31 Jan gives 16/31; no overlap gives 0.
  - `TestBuildYearReconciles`: records inside the year sum to `Total = ΣByMain = ΣMonthly`; a Dec–Jan record splits by days; rejected records are excluded; pending ones are counted.
  - `TestGroupsIsoAndGhg`: all six ISO groups are present, including zeros, and the lines are sorted.
  - `TestReport94`: 10 000 kWh consumption, 2 000 kWh generation, factor 0.469 → 4.69 t / 0.938 t / net 3.752 t; no automated records → nil.
- [ ] Implement. Run `go test -race ./internal/domain/carbon/`. Commit `feat(f10): carbon domain — catalogue, mapping, emission, pro-rata, aggregates (R300–R301, R308–R310, §9.4)`.

## Task 2: Migration 00019 and store additions

**Files:**
- `migrations/00019_carbon.sql`, with the FK re-created `on delete set null` (R318);
- `queries/carbon.sql`: `CarbonDeleteCompanyFactors :execrows`, `CarbonGetReport :one` (scoped like `CarbonListReports`), overlap predicates `(sqlc.narg(overlap_from)::date is null or ca.period_end >= …) and (… ca.period_start <= …)`, order `period_start desc, created_at desc, id`;
- `sqlcgen`; `store/repository.go`; `postgres/carbon.go`; integration tests in `carbon_integration_test.go`.

**Produces:**
```go
DeleteCompanyFactors(ctx, s Scope) (int64, error)          // only s.CompanyID's own
Report(ctx, s Scope, id uuid.UUID) (model.CarbonReport, error)
// CarbonActivityFilter gains OverlapFrom, OverlapTo *time.Time
```
- [ ] **Failing integration tests:**
  - reset deletes only the company's rows, never platform rows or another company's;
  - an activity referencing a deleted company factor survives with `factor_id` null and unchanged value and emission;
  - `Report` answers ErrNotFound across companies and for a BA outside their buildings;
  - overlap filtering;
  - the `TestScopeIsolation` entry for the new methods.
- [ ] Implement, `make generate`, run the integration tests (or PENDING if Docker hangs), lint. Commit.

## Task 3: Service — factors and selection

**Files:** `internal/service/carbon/{service.go,factors.go,selection.go}` + unit tests (fake repos embedding the interface) + integration test.

**Produces:**
```go
type Deps struct{ Carbon store.CarbonRepository; Buildings store.BuildingRepository; Analyzers store.AnalyzerRepository; Analytics store.AnalyticsRepository; Companies store.CompanyRepository; Audit store.AuditRepository; Ops store.OpsRepository; Tenants store.AdminTenantRepository; Clock clock.Clock }
func New(d Deps) *Service
type FactorView struct{ model.EmissionFactor; Conversions []model.EmissionFactorConversion; Overridden bool; PlatformBaseFactor *decimal.Decimal }
func (s *Service) Factors(ctx, sc, f FactorQuery) ([]FactorView, error)          // R302
func (s *Service) Effective(ctx, sc, key string) (FactorView, error)             // ErrNotFound
func (s *Service) OverrideFactor(ctx, sc, id uuid.UUID, in FactorOverride) (FactorView, error) // R303
func (s *Service) ResetFactors(ctx, sc) error                                     // R304
func (s *Service) GridFactor(ctx, sc) (*FactorView, error)                        // R262/R313
func (s *Service) Selected(ctx, sc, buildingID uuid.UUID) ([]string, error)
func (s *Service) SetSelected(ctx, sc, buildingID uuid.UUID, keys []string) ([]string, error) // R315
```
- [ ] **Failing tests:**
  - a shadow hides the platform row and sets `overridden` and the platform value;
  - an override of a platform id copies the conversions;
  - an override of another company's id → 404;
  - `base_factor` 0 → 422;
  - reset, then list → platform values;
  - unknown selection key → 422;
  - a BA outside their building → 404.
- [ ] Implement, gate, commit.

## Task 4: Service — activities and overview

**Files:** `internal/service/carbon/{activities.go,overview.go}` + tests.

**Produces:**
```go
type ActivityInput struct{ BuildingID uuid.UUID; SubCategory, FactorKey, Unit string; Quantity decimal.Decimal; PeriodStart, PeriodEnd time.Time; Description *string; Details map[string]string }
func (s *Service) CreateActivity(ctx, sc, userID uuid.UUID, in ActivityInput) (model.CarbonActivity, error) // R305
func (s *Service) UpdateActivity(ctx, sc, id uuid.UUID, in ActivityInput) (model.CarbonActivity, error)   // R306
func (s *Service) DeleteActivity(ctx, sc, id uuid.UUID) error
func (s *Service) SetStatus(ctx, sc, id uuid.UUID, st model.CarbonStatus) (model.CarbonActivity, error)  // R307
func (s *Service) Activities(ctx, sc, q ActivityQuery) ([]model.CarbonActivity, error)                   // R319
func (s *Service) Overview(ctx, sc, buildingID uuid.UUID, year int) (Overview, error)                   // R310
```
- [ ] **Failing tests:**
  - every R305 refusal: not selected, not applicable, archived factor, bad unit, quantity 0, end < start, span 367, future end, details too long;
  - the conversion multiplier is applied;
  - scope and ISO come from the mapping;
  - the snapshot is in `details`;
  - PATCH resets to pending;
  - automated → 409;
  - **Review Focus 1:** override the factor after create, re-read the activity, and its values are unchanged;
  - overview reconciliation over stored records (integration).
- [ ] Implement, gate, commit.

## Task 5: `carbon.daily_accrual`

**Files:**
- `internal/service/carbon/accrual.go`;
- `internal/job/carbon.go` (type, payload, `NewCarbonAccrualTask`, handler registration in `task.go`);
- `scheduler.go` entry on `Schedule.Carbon`;
- `worker/wiring.go`;
- `service/ops` (the job listed and triggerable with `day`);
- tests.

**Produces:**
- `func (s *Service) Accrue(ctx context.Context, day time.Time) (AccrualResult, error)`
- `AccrualResult{Companies, Buildings, GridRows, GenerationRows int; Skipped map[string]int}`

- [ ] **Failing tests:**
  - **integration:** two runs for the same day → exactly one grid row and one generation row per building (acceptance);
  - late data re-run → the quantity updates;
  - export 0 → no generation row;
  - no factor → grid skipped plus an ops message;
  - a building without analyzers → nothing;
  - rejects a future day and a day > 400 days back;
  - the scheduler entries test includes carbon;
  - the worker registers the handler.
- [ ] Implement, gate (job/worker/scheduler one at a time), commit.

## Task 6: Reports and PDF

**Files:**
- `internal/service/carbon/reports.go`;
- `internal/render/carbonview/view.go` (payload → `reportview.Document`: title, header lines, one section per group with a sub-category table, the §9.4 section with factor value + unit + source year, and a pending note);
- `internal/render/reportpdf/pdf.go`: extract `RenderView(view reportview.Document, title string, at time.Time) ([]byte, error)`, which `Render` then calls;
- tests with a deterministic PDF (uncompressed, fixed time), asserting text content.

**Produces:**
- `func (s *Service) CreateReport(ctx, sc, in ReportInput) (model.CarbonReport, error)`
- `func (s *Service) Reports(ctx, sc, buildingID *uuid.UUID, p store.Page) ([]model.CarbonReport, error)`
- `func (s *Service) ReportPDF(ctx, sc, id uuid.UUID, locale string) ([]byte, string, error)`

- [ ] **Failing tests:**
  - the GHG payload groups by scope, the ISO payload by category (all six);
  - the pending count;
  - §9.4 figures with the factor and its source year;
  - a span > 366 days or a future `to` → 422;
  - the PDF contains the title, building, factor year and total (tr and en);
  - F8b's golden report PDF is unchanged after the `RenderView` extraction.
- [ ] Implement, gate, commit.

## Task 7: Routes, DTOs, permissions, OpenAPI

**Files:**
- `internal/api/v1/handlers_carbon.go`, `dto/carbon.go`, `router.go`/routes registration;
- `auth/roles.go` (`carbon.read`, `carbon.edit`) + the permissions fixture test;
- `apiwire/wiring.go` (`carbon.New`);
- HTTP tests (`carbon_http_integration_test.go`): every route × roles A, CA, CR, BA (own/other building), BR, plus another company's ids → 404;
- `make openapi`.

- [ ] **Failing HTTP tests:**
  - the role matrix of the route table;
  - 422 shapes for R305/R312;
  - the PDF content type.
- [ ] Implement, gate, `make openapi` (the committed diff is the new routes only), commit.

## Task 8: Seed verify and e2e fixtures

**Files:**
- `internal/cli/seed.go` (`--only emission-factors`, `--verify`) + `seed_integration_test.go`;
- `internal/seed/carbon.go`: the e2e company's first building selects 6 sub-categories, with 4 manual activities (pending, approved, rejected, one spanning two months) and 30 days of automated rows via `Accrue`.

- [ ] **Failing tests:**
  - verify on a seeded DB → exit 0, `186/186 match`;
  - after one platform row is altered → non-zero, naming the key.
- [ ] Implement, gate, commit.

## Task 9: Whole-phase self-review and handoff

- Tenancy first: every route × six roles, and ids across companies. Then money/decimal and rounding, then the Review Focus list.
- Run the Verification block. Ledger `Final:` lines. Update `HANDOFF_NEXT_SESSION.md` (F10a done, F10b next) and commit.

---

## Verification

```bash
go test -race ./internal/domain/carbon/... ./internal/service/carbon/... ./internal/render/...
sg docker … go test ./internal/store/postgres/... ./internal/service/carbon/... ./internal/api/v1/... ./internal/cli/... -tags=integration -count=1 -parallel 4
# one at a time: ./internal/job ./internal/worker ./internal/scheduler -run 'Carbon|Registers|Entries'
golangci-lint run --concurrency 2 --build-tags=integration ./internal/...
make check-generate && make openapi   # no diff
ekokod seed --only emission-factors --verify
```

## Acceptance map (09 §F10)

| Criterion | Where |
|---|---|
| Eight categories compute correctly | Task 1 `TestEveryMainCategoryEmission` |
| Scope/ISO derived, never entered | R301, Task 1 + Task 4 (no scope field in the input DTO) |
| Stored factor + multiplier survive a catalogue change | Task 4 Review Focus 1, Task 2 reset test |
| Accrual twice → one record per building/type | Task 5 |
| Grid factor dated, reports state factor + year | R313, Task 6 |
| Overview reconciles with activity records | Task 1 + Task 4 integration |
| Both report types render to PDF | Task 6 (F10a); download button in F10b |
| Master catalogue loads completely, count matches source | F1 seed (182 legacy + 4 R293) + Task 8 `--verify` |
| e2e `carbon.spec.ts` | **F10b** |
