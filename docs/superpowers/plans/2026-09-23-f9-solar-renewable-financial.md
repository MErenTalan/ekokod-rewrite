# F9 — Solar, Renewable and Financial Analysis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this
> plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This phase runs **inline in
> one session**: no parallel agents, no subagents, no Workflow (the user's standing rule).

**Goal:** Build the generation side of the product with **no fabricated data**: Solar Power Plants
(01 §7.7, four tabs), plant management with the iSolar link modal and an optional netting analyzer,
iSolar sync and fault-alarm forwarding, the bills dashboard's plant section (R235 → real), Renewable
Energy (§7.8, both tabs, every detailed panel real or explicitly unavailable), Financial Analysis
(§7.9) and the weather adapter and panel.

**Architecture:** Part A is iSolar. The F2 adapter is corrected: yield-today is cumulative, plant
points are 83022/83025, and fault refs are unique per occurrence. It gains realtime, daily-series and
richer fault calls. A new `internal/service/solar` owns sync (device rows → `plant_production`, plant
totals → new `plant_production_totals`), faults and forwarding, and the read models behind the 05 §8
routes. The two plant aggregates are rebuilt over the totals table, so reports (F8b) and financial
analysis read plant production without code changes.

Part B is analysis. `internal/service/renewable` and `internal/service/financial` each have a
**pure** panel builder and a loader. Renewable is analyzer-based (meter export), as §7.8 says.
`internal/integration/weather` is an Open-Meteo adapter behind a Redis cache. The web gets three new
screens, the settings link modal, and the bills plant section.

**Tech Stack:** Go 1.27 (asynq, pgx/sqlc, shopspring/decimal, swaggest OpenAPI, excelize),
Postgres 17 + TimescaleDB, Redis, Next.js 15 + React 19, TanStack Query via `openapi-react-query`,
Tailwind v4, Vitest + Storybook + Playwright.

**Spec:**
- `docs/rewrite/09-implementation-plan.md` §F9 (scope, acceptance, verification)
- `docs/rewrite/01-project-context.md` §2 (roles), §5 (routes), §7.7–§7.10, §7.15 (Solar tab), §8, §9
- `docs/rewrite/05-api-contract.md` §1, §4 (`/power-plants`), §8, §9
- `docs/rewrite/06-integrations.md` §6 (iSolarCloud), §8 (weather)
- `docs/rewrite/04-data-model.md` §3, §4.5; `10-removed-behaviours.md` items 12, 15, 32
- `docs/rewrite/07-design-system.md` §5, §6, §9, §11; `design-system/bcem-energy/MASTER.md`, then `OVERRIDES.md`
- Legacy (read while planning; data sources recorded in "Legacy panel inventory" below):
  `src/app/(DashboardLayout)/{solar-plants,renewable-energy,financial-analysis}/page.tsx`,
  `src/app/components/{isolar,renewable-energy}/*`, `src/app/api/integration/isolar/*`,
  `src/app/api/cron/alarm-check/route.ts` (iSolar part), `src/utils/isolar/{isolarClient,isolarPoints,alarmTranslator,isolarTypes}.ts`
- Predecessors: F8b `2026-09-23-f8b-reports.md` (R254–R275), F8a (R233–R253), F7 (R211–R232, D-3), F2 (R1–R53)

---

## Global Constraints

- **Branch/worktree:** `phase/f9-solar` in `/home/personal/ekokod-f9-phase`, from `phase/f8b-reports`.
  Nothing is pushed, `main` is untouched, and merging is the user's call.
- Prefix every command with
  `export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
- Wrap Docker, integration and e2e commands in `sg docker -c "…"`. **Never run `make test` or `go test ./...`.**
- **Per-task gate:** the **whole touched package**, `-race`, plus integration for touched store/service/api
  packages, plus `golangci-lint run --concurrency 2 --build-tags=integration <pkgs>`.
- **Integration:** run `sg docker -c "docker ps -a"` first. If the containers are Exited, run `docker start ekokod-test-pg ekokod-test-redis`. Then
  `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <pkg> -tags=integration -count=1 -parallel 4`.
  Run `internal/scheduler`, `internal/worker` and `internal/job` one at a time. Test DBs have no aggregate refresh policies, so refresh explicitly.
- **Web:** `pnpm test --maxWorkers=2`. Run Storybook, Playwright and `next build` one at a time, in the foreground.
  `PLAYWRIGHT_BROWSERS_PATH=/home/personal/.cache/ms-playwright`, `E2E_API_PORT=18082` (18080 is dolmusum's).
- **Energy and money are `decimal`** and cross the API as strings. **Missing is not zero**: nullable + "veri yok".
- **No fabricated data (10 item 12):** every figure is a measurement, a documented formula over
  measurements, or `{available:false, reason}` rendered as an explicit unavailable state. **No Wh or W past the adapter.**
- **Currencies never add (R253/R270):** money is a list keyed by currency.
- **404, never 403, outside scope.** Plants are **company-level**: A CA CR only, never building-scoped (F9 decision a).
- Istanbul calendar days/months/years everywhere; timestamps render Europe/Istanbul.
- Message keys camelCase, every string `tr` + `en`. UI: invoke `ui-ux-pro-max` first, read MASTER then OVERRIDES
  (flat surfaces, emerald tokens, Lexend/Source Sans 3/JetBrains Mono). Every chart has units, legend,
  tooltips and a data table (07 §5). Every new screen gets a `SCREENS` line.
- Screen conventions R195–R199, R229, R249: route → `*-page` → views; `useApiMutation`; `downloadFile`;
  `mockApi` **refuses what the API refuses**; data-free stories; an open-dialog story needs trigger + play.
- Comments: WHY in 1–3 lines, cite ruling ids. Commit when tests are green, then prove the guard red by
  your own mutation, **restoring from a file backup**. Trailer `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.
- `make check-generate` needs a clean tree (commit first). `make openapi` regenerates `openapi.json` + `web/src/lib/api/schema.d.ts`.
- Helper names below are indicative. Use the helpers the joined test file already has.

---

## Product-owner decisions taken on 2026-09-23

| # | Question | Decision |
|---|---|---|
| F-1 | Plant ↔ building/analyzer link (Q-E1, Q-D2) | **Rooftop solar ≠ GES plant.** Rooftop production stays the buildings' metered export (R256 unchanged). iSolar plants are mostly standalone commercial plants that sell their output; they get only an **optional netting analyzer** ("mahsuplaşma analizörü"), never a building link. |
| F-2 | Plant-level (device-less) iSolar samples, quarantined since F2 | **Separate table** `plant_production_totals` with its own aggregates. Device rows stay for the device view. |
| F-3 | A BA reading a worker-generated building report with company plants (Q-E7) | **Acceptable, unchanged.** |

---

## Legacy panel inventory (the data source of every panel, as read on 2026-09-23)

| Screen / panel | Legacy source | Verdict |
|---|---|---|
| Solar: summary cards (power, yields) | live `getDeviceRealTimeData`, Σ inverter points 24/1/87/88/2 | real → device snapshots (R282) |
| Solar: capacity utilisation | **hard-coded `96`** (`solar-plants/page.tsx:302`) | fabricated → power ÷ capacity (R282) |
| Solar: revenue cards | yields × **latest** solar tariff, any date | wrong tariff → per-day effective tariff (R283) |
| Solar: history chart | live plant points `p83022` (cumulative daily yield, differenced), day/month/year series | real → totals table (R276–R279); legacy demo branch invents yearly values (`42000 + y×2000`) |
| Solar: devices | live realtime, `dev_fault_status` 1/2/4 | real (R285) |
| Solar: alarms | live `getFaultAlarmInfo`, labels; forwarding dedup by `fault_code` (a type code) | dedup defect → occurrence ref (R286–R287) |
| Solar: weather | Open-Meteo, **Ankara default**, visibility **constant 10** | removed item 32 → R291 |
| Solar: environmental | yieldTotal × 0.997 kg/kWh, 0.054 trees/kWh, 0.404 coal, 0.12 car | undocumented factors → grid factor + seeded equivalences (R293) |
| Renewable: production tab | analyzer consumption rows (`activeGeneration`, `indGeneration`, `capGeneration`) | real → `/generation` (F3) |
| Renewable: detailed tab | **not rendered at all**. The tab re-keys the same chart. The ten components use `Math.random` (RealTime, EnergyBalance, Efficiency, Forecast, GridInteraction, Analytics) or constants (SystemStatus health `78`, FinancialMetrics) | rebuilt per R292, each field measured, derived or unavailable |
| Financial: consumption, cost | `analyzer.billHistory[month]` (`totalActiveKWh`, `totalCost`) | real → bills (R294) |
| Financial: production | `plant.santral_values.monthly` | real → plant aggregates (R294) |
| Financial: revenue / net | reads the **previous render's** sale price, and `netCost` ignores revenue when consumption > production | defects → R294 |
| Financial: tariff panel | T1–T3 plain average, average feed-in across plants | → lists per building / plant (R294) |

**F2 defects found while planning (fixed in Task 2):** (1) `mapPoint` stores point 1 (Yield Today,
cumulative since midnight) as interval `production_kwh`, and `plant_production_daily` sums them.
Production would be inflated many times over. (2) `PlantMinuteSeries` asks plants for inverter
points `1,24,…`, but plant points are `83022`/`83025`. (3) `Fault.Ref = fault_code`, a fault
*type*, so a recurring fault would be forwarded once and never again.

---

## Binding rulings (R276–R299)

Numbering continues from F8b's R275.

| # | Ruling |
|---|---|
| **R276** | **Two production tables (F-2).** `plant_production` keeps device rows (PK `(plant_id, ts, device_id)`) for per-device data. New hypertable `plant_production_totals (plant_id, ts, production_kwh, active_power_kw, basis, updated_at)`, PK `(plant_id, ts)`, `basis ∈ {plant_meter, inverter_sum, daily_total}`. `plant_production_daily` / `_monthly` are **dropped and recreated over the totals table** (they are empty: nothing ever wrote production) with the same column names; `avg_efficiency_pct` is removed (no source). Every plant figure (reports, financial, solar history, revenue) reads totals only. Result: no double counting. |
| **R277** | **Yield-today is cumulative (F2 defect 1).** The adapter returns `YieldTodayKwh` (not an interval). The sync fetches **whole Istanbul days** and derives interval kWh per series: first sample of the day = its own value; each later sample = value − previous; a negative delta (counter reset or glitch) → that interval is null and counted `yield_reset`; a null sample leaves the next delta anchored to the last non-null. Pure `solar.Intervals(samples) ([]Interval, resets int)`. |
| **R278** | **A day is replaced, never merged.** `ProductionTotals.ReplaceDays(plant, days, rows)` and `Production.ReplaceDeviceDays(plant, days, rows)` delete `[day, day+1)` for the plant and insert the rows in one transaction. Re-running any window yields the same rows. |
| **R279** | **Which source fills a day.** Days before yesterday (Istanbul) come from `PlantDailySeries` (point 83022 end-of-day, one row at local midnight, `basis=daily_total`). Today and yesterday come from minute series: plant samples (83022/83025) → `plant_meter`. If the plant series returns no samples for a day, Σ device intervals per timestamp → `inverter_sum`. If neither exists, nothing is written ("veri yok"). The API exposes `basis` per point, and the screen shows a `DataQualityBadge` when a range mixes bases. |
| **R280** | **Credentials.** `power_plants.isolar_credential_id` (new, nullable FK) is set at link time. The credential must be the plant's company's, provider `isolar`, active, or 422 `credential_id: not_isolar`. Every iSolar call for a plant goes through `credentials.Open(SystemScope(company), credential_id)`, which already serialises token refresh. Unlink clears the ps fields and the credential id and keeps stored production. |
| **R281** | **Link imports.** `POST /plants/{id}/isolar-link {credential_id, ps_id}`: the ps_id must appear in `isolar.Plants(creds)` (else 422 `ps_id: not_found`). A ps_id linked to another plant → 409 `isolar_plant_already_linked`. The link sets `isolar_ps_id/ps_name/installed_kw/linked_at`, fills `total_capacity_kw` only when null, and upserts devices. Then it enqueues `isolar.sync_plant` with `backfill_days = 400` (daily series in 31-day chunks = 13 calls). `plant.name` is never overwritten. |
| **R282** | **Realtime = device snapshots.** The sync calls `DeviceRealtime` for inverter types 1 and 14 (points 24, 1, 87, 88, 2 plus `dev_fault_status`) and stores them on `power_plant_devices` (`snapshot_at, active_power_kw, yield_today_kwh, yield_month_kwh, yield_year_kwh, yield_total_kwh, status`). `GET /plants/{id}/realtime` = Σ over inverters that have a snapshot, `as_of` = oldest `snapshot_at`, `stale = now − as_of > 2h`. `capacity_utilisation_pct = active_power ÷ (isolar_installed_kw ?? total_capacity_kw) × 100`, 1 dp, null when either is null or capacity is 0. A yield that no inverter reports is null. `connection`: `connected` when `isolar_last_sync_at` is ≤ 45 min old and `isolar_last_sync_error` is null; `error` with the closed code when the last sync failed; `never_synced` before the first sync. |
| **R283** | **Revenue.** Σ over Istanbul days of daily production (totals daily aggregate) × the feed-in tariff effective that day, grouped by currency. Periods: `daily` = today, `monthly` = month-to-date, `yearly` = year-to-date, `total` = every stored day, carrying `since` = first stored day. Days with production but no tariff → `unpriced_days`, and the figure is `partial`. No solar tariff at all → `{available:false, reason:"no_solar_tariff"}`. Money is rounded half-up to 2 dp at the end only. |
| **R284** | **Production series.** `GET /plants/{id}/production?granularity=hour|day|month&from&to` (Istanbul dates, `to` exclusive): `hour` = Σ totals rows per local hour (range ≤ 31 days); `day` = daily aggregate (≤ 366 days); `month` = monthly aggregate, closed months + open month composed from the daily aggregate (≤ 10 years). Each point: `ts, production_kwh, basis`. §7.7's day/month/year selector maps to hour/day/month. `/production/export` = the same series as xlsx (sheet "Üretim", columns Tarih, Üretim (kWh), Kaynak). |
| **R285** | **Devices.** From `power_plant_devices` with snapshot fields. `status`: 4 → `normal`, 2 → `alarm`, 1 → `fault`, anything else or a snapshot older than 2 h → `offline`; no snapshot → null. `q` filters device name or serial, case-insensitively. The device count is the list length. |
| **R286** | **Faults (F2 defect 3).** `Fault.Ref = ps_id + "|" + (ps_key ?? "") + "|" + fault_code + "|" + create_time`. `isolar.fetch_alarms` stores faults in new `plant_faults` (upsert on `(plant_id, ref)`: name, code, level, type, device name, occurred_at, closed_at from `over_time`, first_seen_at). The list (newest first, paged) carries `message_tr`, which is legacy's dictionary (`alarmTranslator.ts` exact maps + substring rules) ported to `solar.TranslateFault`. Its `translated` flag is false when no rule matched, and the original text is then kept. Level and type labels come from i18n keys. The fetch window is the last 7 days. |
| **R287** | **Forwarding once and only once.** For each fault not yet forwarded: `insert into isolar_forwarded_alarms … on conflict do nothing returning alarm_ref` claims it. Only claimed refs are mailed, one mail per plant per run (Turkish, R222's shape) through the company's SMTP. If the send fails, the claims are deleted (so the next run retries) and a Messages row is written (category `isolar-alarm`, code `smtp_not_configured` / `delivery_failed`). No recipients → nothing is claimed or sent. Retention: forwarded refs and faults older than 180 days are deleted at the end of each run (180 ≫ the 7-day window, so nothing is resent). |
| **R288** | **Jobs.** `isolar.dispatch_sync` (`*/15 * * * *`, `EKOKOD_SCHEDULE_ISOLAR_SYNC`) enqueues `isolar.sync_plant` for every linked plant of every company. `isolar.fetch_alarms` (`10 * * * *`, `EKOKOD_SCHEDULE_ISOLAR_ALARMS`) runs per company. Both ticks join `ops.Triggerable`. `isolar.sync_plant` payload `{company_id, plant_id, backfill_days}`, task id `isolar.sync_plant:<plant>:<backfill_days>`, enqueued through `EnqueueReplacingFinished`, watchable (A CA CR of the company). A sync failure records `isolar_last_sync_error` (closed codes `isolar_auth`, `isolar_unavailable`, `isolar_not_linked`, `credential_missing`) and a job_runs code. One plant's failure never fails another plant's task. |
| **R289** | **Netting analyzer (F-1).** `power_plants.netting_analyzer_id` (nullable FK) on `POST/PATCH /power-plants`. It must be a non-deleted analyzer of the same company, else 422 `netting_analyzer_id: not_found`. It is used only by the bills plant section (R290) and never scopes a plant to a building. |
| **R290** | **Bills dashboard plant section (replaces R235).** A CA CR only. For BA/BR the answer is `{available:false, reason:"plants_not_in_scope"}`, and the screen hides the section. One row per company plant: `plant_name`, `analyzer_name` + `installation_number` of the netting analyzer (null when none), `production_kwh` for the Istanbul calendar month (monthly aggregate, open month composed), `production_price` = feed-in effective on the month's first day (with currency), `consumption_price` = the netting analyzer's analyzer-scope bill `effective_energy_price` for that `period_key` (null otherwise), and `invoice_amount` = production × production_price (sale value). The download is that analyzer bill's PDF when it exists. Totals: `total_production_kwh`, `total_invoice` per currency. No plants → `available:true, rows:[]`. |
| **R291** | **Weather.** `EKOKOD_WEATHER_PROVIDER` ∈ {"", `open-meteo`}. Empty → `{available:false, reason:"weather_not_configured"}` (air-gapped). Base URL `EKOKOD_WEATHER_BASE_URL` (https, default `https://api.open-meteo.com`). `GET /weather?plant_id|building_id` (exactly one) takes coordinates from that record. Missing coordinates → `reason:"location_not_configured"`, and **no location name**. Redis cache keyed by coordinates rounded to 2 dp: current 15 min, forecast 3 h. Provider error → `reason:"weather_unavailable"`. Fields: temperature °C, humidity %, wind km/h, pressure hPa, visibility km (null when absent), uv index (today's max), precipitation chance % (today's max), 7 days (date, min, max, weather code, precipitation %, `shortwave_radiation_sum` MJ/m²), `potential` per day: `high` ≥ 20, `medium` ≥ 10, else `low` MJ/m², with `potential_basis: "shortwave_radiation_sum_mj_m2"`. Weather codes map to i18n keys (legacy's code bands). |
| **R292** | **Renewable detailed panels (analyzer-based, §7.8).** Inputs are only: hourly/daily consumption aggregates of the selected analyzer (or of the building's analyzers), its analyzer-scope bills, its latest reading time, forecasts, the grid factor and equivalence rows, and weather. The panel/field table below is the contract. Anything else is `unavailable` with the given reason. `TestNoSyntheticData` enforces it. |
| **R293** | **Environmental.** CO₂ avoided (kg) = generation kWh × grid factor (R262's lookup, **not** legacy 0.997). Equivalences come from platform `emission_factors` rows seeded with source and year (`main_category = 'equivalence'`): `equiv_tree_co2_kg_per_year` (trees = CO₂ ÷ factor), `equiv_coal_kg_per_kwh` (coal kg = kWh × factor), `equiv_car_co2_kg_per_km` (km = CO₂ ÷ factor), `equiv_home_heating_kwh_per_year` (homes = kWh ÷ factor). A missing row makes only that equivalence unavailable. Each carries its caption, factor, unit, source and year. |
| **R294** | **Financial Analysis (§7.9).** Company-wide, A CA CR; `year` required, `month` optional. Per Istanbul month m: `consumption_kwh` = Σ `net_consumption` and `cost` = Σ `total_cost` (per currency) of the company's **building-scope, non-superseded** bills with `period_key = m` (R255's rows). `production_kwh` = Σ monthly aggregate over every company plant. `revenue` = Σ plant production × that plant's feed-in effective on m's first day (per currency; unpriced plants → `partial`). `offset_kwh` = production − consumption; `grid_purchase_kwh` = max(consumption − production, 0); `grid_sale_kwh` = max(production − consumption, 0), labelled "mahsuplaşma (hesaplanan)". `net` = cost − revenue per currency (**always**, fixing legacy). Null consumption or production → the derived figures are null. Year row = Σ months with data plus `with_data/of` coverage (R258). Headline: `analyzer_count`, `plant_count`, the period totals and the offset. Tariff panel: per building, the tariff in force today (single: price; multi: T1/T2/T3) with currency; per plant, the feed-in in force today; empty lists → `purchase_missing` / `sale_missing` flags. |
| **R295** | **Routes.** Nav hrefs become 01 §5's `/ekorm/solar-plants`, `/ekorm/financial-analysis`, `/ekorm/renewable-energy` (nav-config currently says `/ekorm/financial`, `/ekorm/renewable`). |
| **R296** | **Permissions.** New `plants.read` (A CA CR), `plants.manage` (A CA: link, unlink, sync, recipients), `financial.read` (A CA CR), `renewable.read` (all six roles). `nav.financial` stays the financial nav gate, `nav.solar_plants` the solar one. Renewable nav gets `renewable.read`. |
| **R297** | **Demo.** Renewable routes serve the demo company's seeded analyzers like every scope route. Solar and financial routes are not demo routes (05 §8–§9 roles). |
| **R298** | **Request limits.** `ps_id` non-empty ≤ 64 characters. Recipients: 0–20 addresses, each `net/mail`-valid, deduplicated case-insensitively. Production ranges per R284 → 422 `range_too_long`. Renewable `from/to` ≤ 366 days, `analyzer_id` or `building_id` required (exactly one). Unknown query parameters → 400. |
| **R299** | **Units at the boundary (09 acceptance).** Every `isolar` exported type carries kWh/kW only. `TestNoWattsCrossTheAdapter` reflects over the exported structs (no field named `*Wh`/`*W` without `k`), and each mapping test feeds Wh/W fixtures and asserts ÷1000. |

### R292 panel table (the `TestNoSyntheticData` contract)

"gen" = Σ `active_generation` (kWh), "imp" = Σ `active_consumption` (kWh), both over the selected
analyzers. "Bills" = analyzer-scope bills, non-superseded.

| Panel (route) | Field | Source / formula | Unavailable reason |
|---|---|---|---|
| overview | active, inductive, capacitive generation; average; total | range Σ / mean over buckets of `active_generation`, `inductive_generation`, `capacitive_generation` | — |
| realtime | current_power_kw | last complete hour's gen ÷ 1 h | `no_recent_data` if none in 24 h |
| realtime | today_kwh, series_24h, max_power_kw, avg_power_kw | hourly gen today / last 24 h; max / mean of hourly gen | — |
| realtime | status | `producing` if last complete hour gen > 0, `idle` if = 0, `no_data` if none in 24 h | — |
| realtime | system_efficiency_pct | — | `no_irradiance_or_capacity_measurement` |
| energy-balance (existing `/energy-balance`) | generation, consumption, grid import, grid export | F3 as built | battery filter: `no_battery_measurement` |
| analytics.financial | import_price, export_price | latest bill's `effective_energy_price`, `generation_price_per_kwh` (+currency, period) | `no_bill` |
| analytics.financial | today_import_cost, today_export_revenue, net | today imp × import_price, today gen × export_price, their difference | `no_bill` |
| analytics.financial | month/year earnings, total savings | Σ bills' generation value (gen × `generation_price_per_kwh`) over bills in the period | `no_bill` |
| analytics.financial | roi_pct, payback_years, bill_savings | — | `no_investment_cost` / `no_self_consumption_measurement` |
| environmental | co2_avoided_kg + equivalences | R293 over gen in range | per factor: `no_factor` |
| weather | all | R291, building coordinates | R291 reasons |
| system-status | monitoring | `healthy` when last reading ≤ 26 h old, `attention` ≤ 72 h, else `critical`; carries `last_reading_at` | — |
| system-status | grid_connection | `healthy` when imp or gen has data in the last 26 h, else `critical` | — |
| system-status | solar_panels, inverter, battery, security | — | `no_component_telemetry` |
| system-status | overall | worst of the **available** components | — |
| forecast | estimated_consumption (hourly 24 h / daily 7 d / weekly 4 w) | Σ `median` of the latest forecast run | `no_forecasts` |
| forecast | accuracy daily/weekly/overall | `100 − MAPE` over hours with both forecast (latest run before the hour) and actual imp > 0, in the last 1 / 7 / 30 days | `no_forecasts` / `no_overlap` |
| forecast | estimated_generation, net_excess, weather_impact | — | `no_generation_forecast` |
| efficiency | every field and recommendations | — | `no_irradiance_or_capacity_measurement` (recommendations: empty list + reason) |
| grid-interaction | direction | last complete hour: gen > imp → `export`, imp > gen → `import`, equal → `balanced` | `no_recent_data` |
| grid-interaction | today_import, today_export | today Σ imp / gen | — |
| grid-interaction | power_factor | `imp / sqrt(imp² + ind²)` over today's hourly buckets (3 dp) | `no_reactive_data` |
| grid-interaction | voltage, frequency | — | `no_power_quality_measurement` |
| analytics | peak_generation (kWh + hour), average_generation, trend (daily gen) | range buckets | — |
| analytics | peak_hour insight | hour of day with the highest mean hourly gen over the range | `no_data` |
| analytics | data_availability_pct ("system reliability") | hours with a reading ÷ hours in range × 100 | — |
| analytics | total_savings | = analytics.financial.total_savings | `no_bill` |
| analytics | efficiency, efficiency_change_30d, consumption_optimisation, maintenance insights | — | `no_irradiance_or_capacity_measurement` |

---

## Rule table: permissions (extends R296)

| Permission | A | CA | CR | BA | BR | D |
|---|---|---|---|---|---|---|
| `plants.read` | ✓ | ✓ | ✓ | — | — | — |
| `plants.manage` | ✓ | ✓ | — | — | — | — |
| `financial.read` | ✓ | ✓ | ✓ | — | — | — |
| `renewable.read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

## Rule table: routes

| # | Method | Pattern | Roles | Answer |
|---|---|---|---|---|
| 1 | GET | `/plants/{id}/realtime` | A CA CR | `PlantRealtime` (R282) |
| 2 | GET | `/plants/{id}/production` | A CA CR | `PlantProductionSeries` (R284) |
| 3 | GET | `/plants/{id}/production/export` | A CA CR | xlsx |
| 4 | GET | `/plants/{id}/devices` | A CA CR | `{items: PlantDeviceView[]}` (R285) |
| 5 | GET | `/plants/{id}/alarms` | A CA CR | `Page[PlantFault]` (R286) |
| 6 | POST | `/plants/{id}/sync` | A CA | 202 `{job_id}` (R288) |
| 7 | GET | `/plants/{id}/revenue` | A CA CR | `PlantRevenue` (R283) |
| 8 | GET | `/integrations/isolar/plants?credential_id` | A CA | `{items:[{ps_id, name, installed_kw, linked_plant_id}]}` |
| 9 | POST | `/plants/{id}/isolar-link` | A CA | `PowerPlant` + `job_id` (R281) |
| 10 | DELETE | `/plants/{id}/isolar-link` | A CA | 204 |
| 11 | PUT | `/plants/{id}/alarm-recipients` | A CA | `{emails}` (R298) |
| 12 | GET | `/financial/summary?year&month` | A CA CR | `FinancialSummary` (R294) |
| 13 | GET | `/financial/monthly?year` | A CA CR | `{items: FinancialMonth[], total}` |
| 14–21 | GET | `/renewable/{overview,realtime,grid-interaction,environmental,efficiency,forecast,analytics,system-status}` | all | panel DTOs (R292); §7.8's "financial gains" panel is the `financial` object inside `/renewable/analytics` (05 §9 has no own route) |
| 22 | GET | `/weather?plant_id|building_id` | scope (plant → A CA CR) | `Weather` (R291) |

`POST/PATCH /power-plants` gain `netting_analyzer_id`. `GET /bills/dashboard`'s `plants` becomes R290.

## Rule table: jobs

| Type | Payload | Task id | Watchable |
|---|---|---|---|
| `isolar.dispatch_sync` | none | none | no |
| `isolar.sync_plant` | `{company_id, plant_id, backfill_days}` | `isolar.sync_plant:<plant>:<backfill_days>` | yes (A CA CR) |
| `isolar.fetch_alarms` | none (all companies) | none | no |

---

## File map

```
internal/store/postgres/migrations/00018_solar.sql            totals table, rebuilt aggregates, plant/device columns, plant_faults
internal/store/postgres/queries/solar.sql (+ sqlcgen)         totals, device snapshot, faults, forwarded claims
internal/store/postgres/{production.go,plants.go,faults.go}   repository methods
internal/store/repository.go                                  ProductionTotalsRepository, FaultRepository, PlantRepository additions
internal/domain/model/{plants.go,timeseries.go}               new fields/types
internal/integration/isolar/{series.go,realtime.go,daily.go,alarms.go,wire.go}   F2 fixes + new calls
internal/service/solar/{intervals.go,translate.go,revenue.go}  pure: R277, R286, R283
internal/service/solar/{sync.go,faults.go,reads.go,link.go,service.go}
internal/ingest/production/                                   deleted (superseded by solar.sync; its quarantine test removed with it)
internal/job/isolar.go                                        three task types
internal/integration/weather/openmeteo.go                     R291 adapter
internal/service/weather/weather.go                           cache + coordinates
internal/service/renewable/{panels.go,sources.go,service.go}  R292 (pure panels + loader)
internal/service/financial/{financial.go,service.go}          R294
internal/api/v1/{handlers_solar.go,handlers_analysis2.go,dto/solar.go,dto/renewable.go,dto/financial.go}
internal/service/billing (dashboard plants)                   R290
internal/seed/{solar.go,data/emission_factors.json}           demo/e2e plant data, equivalence factors
web/src/features/solar-plants/*                               screen, 4 tabs
web/src/features/settings/{isolar-link-dialog,plant-form}.tsx link modal, netting analyzer
web/src/features/bills/plants-section.tsx                     R290
web/src/features/renewable/*, web/src/features/financial/*, web/src/features/weather/*
web/src/app/ekorm/{solar-plants,renewable-energy,financial-analysis}/page.tsx
web/tests/e2e/{solar-plants,renewable,financial}.spec.ts
```

---

## Review Focus

1. **A day re-synced while the previous sync's rows exist.** Expect identical rows, no double sum, and aggregates equal to Σ of the day. Test: `TestSyncTwiceSameRows` (Task 4).
2. **A yield-today counter that resets mid-day or skips a sample.** Expect no negative production and no inflated day. Test: `TestIntervalsResetAndGap` (Task 3).
3. **Two alarm runs overlapping, or a send failing.** Expect exactly one mail per fault, with a retry after a failure. Tests: `TestForwardOnceAcrossRuns`, `TestForwardFailureReleasesClaims` (Task 5).
4. **A BA calling a plant route with a real plant id, or a CA with another company's plant.** Expect 404. Test: the tenancy sweep family `plants` (Task 7).
5. **A tariff change inside the month, or production in two currencies.** Expect revenue split per day and never added across currencies. Test: `TestRevenueTariffChangeMidMonth` (Task 3), `TestFinancialReconcilesFixture` (Task 13).

---

## Part A — iSolar and the Solar Power Plants screen

### Task 1: Migration 00018, model and store

**Files:** migration `00018_solar.sql`; `queries/solar.sql`; `sqlcgen`; `store/repository.go`;
`store/postgres/{production.go,plants.go,faults.go}`; model; tests
`store/postgres/solar_integration_test.go`, `migrations_solar_integration_test.go`, `TestScopeIsolation` additions,
the aggregates-settings assertion in `migrations_timeseries_integration_test.go`.

**Interfaces (produces):**
```go
// model
type PlantProductionTotal struct { PlantID uuid.UUID; Ts time.Time; ProductionKwh, ActivePowerKw *decimal.Decimal; Basis string }
type PlantFault struct { PlantID uuid.UUID; Ref, Code, Name string; Level, Type *int32; DeviceName *string
    OccurredAt time.Time; ClosedAt *time.Time; FirstSeenAt time.Time }
// PowerPlant += IsolarCredentialID, NettingAnalyzerID *uuid.UUID; IsolarLastSyncAt *time.Time; IsolarLastSyncError *string
// PlantDevice += SnapshotAt *time.Time; ActivePowerKw, YieldTodayKwh, YieldMonthKwh, YieldYearKwh, YieldTotalKwh *decimal.Decimal; FaultStatus *int32
type ProductionTotalsRepository interface {
    ReplaceDays(ctx, s Scope, plantID uuid.UUID, days []time.Time, rows []model.PlantProductionTotal) (int, error)
    Range(ctx, s Scope, plantID uuid.UUID, r TimeRange) ([]model.PlantProductionTotal, error)
    FirstDay(ctx, s Scope, plantID uuid.UUID) (*time.Time, error)
}
// ProductionRepository += ReplaceDeviceDays(ctx, s, plantID, days []time.Time, rows []model.PlantProduction) (int, error)
// PlantRepository += UpdateDeviceSnapshot(ctx, s, d model.PlantDevice) error; SetSyncState(ctx, s, plantID, at time.Time, errCode *string) error
//                    SetIsolarLink(ctx, s, p model.PowerPlant) (model.PowerPlant, error)  // ps fields + credential id, nil clears
//                    ListLinked(ctx, s Scope) ([]model.PowerPlant, error)
type FaultRepository interface {
    Upsert(ctx, s Scope, faults []model.PlantFault) (int, error)
    List(ctx, s Scope, plantID uuid.UUID, p Page) ([]model.PlantFault, int, error)
    Unforwarded(ctx, s Scope, plantID uuid.UUID, since time.Time) ([]model.PlantFault, error)
    Claim(ctx, s Scope, plantID uuid.UUID, refs []string) ([]string, error)   // insert … on conflict do nothing returning
    Release(ctx, s Scope, plantID uuid.UUID, refs []string) error
    Prune(ctx, s Scope, before time.Time) (int, error)
}
```
Migration: `plant_production_totals` (hypertable, 30-day chunks, compression segmentby `plant_id`);
drop and recreate the two aggregates over it (`sum(production_kwh)`, `max(active_power_kw) as max_active_power_kw`, same policies);
`power_plants` += `isolar_credential_id uuid references integration_credentials(id)`, `netting_analyzer_id uuid references analyzers(id)`,
`isolar_last_sync_at timestamptz`, `isolar_last_sync_error text`; `power_plant_devices` += the snapshot columns;
`plant_faults (plant_id … on delete cascade, ref text, … primary key (plant_id, ref))`. Down reverses (round-trip guard).

- [ ] Write failing tests. `TestMigration00018RoundTrip`. `TestReplaceDaysReplacesOnlyThoseDays` (day D has 3 rows, D+1 has 2; replace D with 1 row → D has 1, D+1 has 2). `TestProductionDailyAggregatesTotalsOnly` (a device row of 5 kWh + a total row of 7 kWh → daily = 7). `TestClaimReturnsOnlyNewRefs` (claim {a,b}, then {b,c} → {c}). `TestPruneKeepsRecent`. Plus a TestScopeIsolation entry for each new method (another company's plant → ErrNotFound / no rows).
- [ ] Run and see red, then implement (sqlc queries, `make generate`), then green, then the whole `store/postgres` + `store/postgres/admin` integration run, then `internal/arch` (secret-column guard).
- [ ] Mutations: drop the delete in ReplaceDays → the first test is red; aggregate over `plant_production` → the second is red. Commit.

### Task 2: iSolar adapter: F2 fixes and new calls

**Files:** `internal/integration/isolar/{series.go,realtime.go,daily.go,alarms.go,wire.go}`, tests + `testdata/*.json`.

**Produces:**
```go
type YieldSample struct { PSKey *string; Ts time.Time; YieldTodayKwh, ActivePowerKw *decimal.Decimal } // R277: cumulative
func (c *Client) DeviceMinuteSeries(ctx, creds, psKeys []string, from, to time.Time) ([]YieldSample, error)   // points "1,24"
func (c *Client) PlantMinuteSeries(ctx, creds, psID string, from, to time.Time) ([]YieldSample, error)      // points "83022,83025", PSKey nil
type DailyYield struct { Day time.Time; Kwh *decimal.Decimal }                                              // Day = Istanbul midnight
func (c *Client) PlantDailySeries(ctx, creds, psID string, from, to time.Time) ([]DailyYield, error)         // query_type "1" (day), data_point "p83022", data_type "2", start/end "yyyyMMdd", ≤ MaxWindowDay
type DeviceSnapshot struct { PSKey, DeviceSN string; FaultStatus *int32; At time.Time
    ActivePowerKw, YieldTodayKwh, YieldMonthKwh, YieldYearKwh, YieldTotalKwh *decimal.Decimal }
func (c *Client) DeviceRealtime(ctx, creds, deviceType int32, psKeys []string) ([]DeviceSnapshot, error)    // point_id_list 24,1,87,88,2
// Fault += Level, Type *int32; DeviceName *string; ClosedAt *time.Time; Ref per R286
```
`ProductionSample` and its irradiance fields go away (the only consumer, `internal/ingest/production`, is deleted in Task 4).
New ops use the definition-key convention (R40): add `get_device_real_time_data`,
`get_power_station_point_day_month_year_data_list` to `seed/data/integration_definitions.json`'s isolar row (the F2 row is authoritative for keys and paths).

- [ ] Failing tests: `TestDeviceMinuteSeriesReturnsCumulativeYieldInKwh` (p1 "12500" Wh → 12.5), `TestPlantMinuteSeriesAsksForPlantPoints` (the request body `points == "83022,83025"`), `TestPlantDailySeriesOneRowPerDay`, `TestDeviceRealtimeNormalisesUnits` (p24 "3200" W → 3.2 kW, p2 Wh → kWh, dev_fault_status kept), `TestFaultRefIsPerOccurrence` (same fault_code, two create_times → two refs), `TestNoWattsCrossTheAdapter` (reflection, R299).
- [ ] Red, then implement, then green, then `go test -race ./internal/integration/...`. Mutation: map p1 without ÷1000 → red. Commit.

### Task 3: `internal/service/solar`: pure rules

**Files:** `intervals.go`, `translate.go`, `revenue.go` + tests.

**Produces:**
```go
type Interval struct { Ts time.Time; Kwh *decimal.Decimal; PowerKw *decimal.Decimal }
func Intervals(samples []isolar.YieldSample) (out []Interval, resets int)            // one series, sorted; R277
func SumByTimestamp(series ...[]Interval) []Interval                                 // inverter_sum
type DayTariff struct { From time.Time; Price decimal.Decimal; Currency model.CurrencyCode }
type Revenue struct { Currency model.CurrencyCode; Amount decimal.Decimal }
func RevenueOver(days map[time.Time]decimal.Decimal, tariffs []DayTariff) (out []Revenue, unpriced int) // R283
func TranslateFault(name, code string) (text string, translated bool)                // R286
func StatusOf(fault *int32, at *time.Time, now time.Time) *string                     // R285
func Utilisation(power, capacity *decimal.Decimal) *decimal.Decimal                   // R282
```
- [ ] Failing table tests with exact values. `TestIntervalsResetAndGap`: samples 0:00→1.0, 1:00→3.5, 2:00→null, 3:00→5.0, 4:00→0.2 give intervals 1.0, 2.5, null, 1.5, null with resets=1. `TestRevenueTariffChangeMidMonth`: days 1–10 at 10 kWh × 2.00 TRY and days 11–20 at 10 kWh × 2.50 TRY give 450.00 TRY. `TestRevenueTwoCurrencies` gives two entries. `TestRevenueUnpricedDay` gives unpriced=1. `TestTranslateFaultExactAndFallback` ("电网掉电" → "şebeke kesintisi" translated; unknown text → untranslated). `TestUtilisationNilAndZero`.
- [ ] Red, then implement, then green. Mutation: first-of-day interval = 0 → red. Commit.

### Task 4: Sync: `isolar.sync_plant`, dispatch, link backfill

**Files:** `service/solar/{service.go,sync.go}`, `internal/job/isolar.go`, worker/scheduler/config/ops wiring,
`service/jobs` watchable, delete `internal/ingest/production`, tests
(`sync_test.go` with a fake adapter, `sync_integration_test.go`, job tests, `TestTriggerableIncludesIsolarTicks`, scheduler cron test).

**Produces:**
```go
type Adapter interface { /* the five Client methods above + Plants, Devices */ }
type Deps struct { Plants store.PlantRepository; Production store.ProductionRepository; Totals store.ProductionTotalsRepository
    Faults store.FaultRepository; Solar store.SolarTariffRepository; Analytics store.AnalyticsRepository; Analyzers store.AnalyzerRepository
    Bills store.BillRepository; Creds CredentialOpener; ISolar Adapter; Ops store.OpsRepository  // job_runs are written the way billing's generator does
    SMTP store.SMTPRepository; Mail mail.Sender; Clock clock.Clock; Enqueue job.Enqueuer }
func New(d Deps) *Service
func (s *Service) SyncPlant(ctx context.Context, companyID, plantID uuid.UUID, backfillDays int) (SyncResult, error)
func (s *Service) DispatchSync(ctx context.Context) (int, error)
type SyncResult struct { DevicesUpserted, Snapshots, TotalRows, DeviceRows, Resets int; Basis map[string]int }
```
Steps inside SyncPlant, each day independent (R278/R279): upsert devices; realtime snapshots; for
yesterday and today: device minute series per day → Intervals → ReplaceDeviceDays; plant minute series →
Intervals → totals (`plant_meter`), else SumByTimestamp(devices) (`inverter_sum`); ReplaceDays. If
backfillDays > 0: PlantDailySeries in ≤31-day chunks → `daily_total` rows for days < yesterday. Then SetSyncState.
Failures map to R288 codes and never leak adapter text (R192).

- [ ] Failing tests. `TestSyncWritesPlantMeterTotals`. `TestSyncFallsBackToInverterSum`. `TestSyncBackfillUsesDailyTotalsBeforeYesterday`. `TestSyncTwiceSameRows` (integration: aggregates after 2 runs = after 1). `TestSyncAuthFailureRecordsCode`. `TestDispatchEnqueuesEveryLinkedPlant`. `TestSyncPlantTaskIDIsStable`.
- [ ] Wire: `job.Handlers.SolarSync`, `SolarDispatch`, `SolarAlarms`. Config `ISolarSync`, `ISolarAlarms` schedules. Scheduler entries. `ops.Triggerable` += `isolar.dispatch_sync`, `isolar.fetch_alarms`. Worker builds `solar.Service` with the real `isolar.Client` and `credentials.Service`.
- [ ] Gates: the solar package, job (alone), worker (alone), scheduler (alone), ops, jobs, config, `internal/arch`. Mutation: skip ReplaceDays' delete → `TestSyncTwiceSameRows` red. Commit.

### Task 5: Faults and forwarding: `isolar.fetch_alarms`

**Files:** `service/solar/faults.go` + tests (unit + integration), job handler.

**Produces:** `func (s *Service) FetchAlarms(ctx context.Context) (FetchResult, error)`. For every company with an
active isolar credential: `Faults(creds, now−7d, now)` → match `ps_id` to that company's linked plants (unknown → skipped count) →
`Faults.Upsert` → per plant with recipients: `Unforwarded` → `Claim` → one mail (subject `iSolar Alarm Bildirimi: <plant> - <n> yeni alarm`,
body lines `[<type> - <level>] <translated> | Zaman: <Istanbul time>`) → on send error `Release` + Messages row. Then `Prune(now−180d)`.

- [ ] Failing tests. `TestForwardOnceAcrossRuns` (two runs over the same faults → one Send). `TestForwardFailureReleasesClaims` (Send fails → next run sends). `TestForwardNoRecipientsClaimsNothing`. `TestFaultForAnotherCompanysPlantIsIgnored`. `TestFaultMailNeverLeaksSMTPPassword` (reuse F7's assertion).
- [ ] Red, green, gates. Mutation: send before claim → the first test red. Commit.

### Task 6: Read models, link, recipients

**Files:** `service/solar/{reads.go,link.go}` + tests; `service/assets/plants.go` (netting analyzer, R289).

**Produces:**
```go
func (s *Service) Realtime(ctx, sc store.Scope, plantID uuid.UUID) (Realtime, error)            // R282
func (s *Service) ProductionSeries(ctx, sc, plantID, gran string, from, to time.Time) (Series, error) // R284
func (s *Service) ExportProduction(ctx, sc, plantID, gran, from, to, locale string) ([]byte, string, error)
func (s *Service) Devices(ctx, sc, plantID, q string) ([]DeviceView, error)                   // R285
func (s *Service) Alarms(ctx, sc, plantID, p store.Page) ([]FaultView, int, error)            // R286
func (s *Service) Revenue(ctx, sc, plantID) (RevenueView, error)                              // R283
func (s *Service) AccountPlants(ctx, sc, credentialID uuid.UUID) ([]AccountPlant, error)
func (s *Service) Link(ctx, sc, plantID, credentialID uuid.UUID, psID string) (model.PowerPlant, string /*job id*/, error) // R280–R281
func (s *Service) Unlink(ctx, sc, plantID uuid.UUID) error
func (s *Service) SetRecipients(ctx, sc, plantID uuid.UUID, emails []string) ([]string, error) // R298
func (s *Service) EnqueueSync(ctx, sc, plantID uuid.UUID) (string, error)
func (s *Service) BillPlants(ctx, sc, year, month int) (BillPlants, error)                    // R290, used by Task 8
```
- [ ] Failing tests with exact values. `TestRealtimeSumsInvertersAndUtilisation` (3.2 + 1.8 kW on 10 kW → 50.0 %). `TestRealtimeStaleAfterTwoHours`. `TestProductionSeriesRangeLimits`. `TestProductionMonthComposesOpenMonth`. `TestDevicesSearchSerialCaseInsensitive`. `TestLinkRejectsForeignCredential`, `TestLinkConflictWhenLinkedElsewhere`, `TestLinkKeepsPlantName`. `TestNettingAnalyzerMustBeSameCompany`. `TestRevenueTotalSinceFirstDay`.
- [ ] Red, green, gates (solar, assets, integration). Mutation: utilisation denominator = total_capacity first → red. Commit.

### Task 7: Routes, permissions, OpenAPI

**Files:** `api/v1/handlers_solar.go`, `dto/solar.go`, `dto/assets.go` (netting), `routes.go`, `auth/permissions*`,
`authz_matrix_test.go`, `tenancy_sweep_integration_test.go` (family `plants`), `solar_http_integration_test.go`, `openapi.json`, schema.

- [ ] Failing tests. The matrix rows for routes 1–11. `TestSolarRoutesHideOtherCompanysPlant` (404). `TestBuildingAdminGets404OnPlantRoutes`. `TestSyncAnswers202WithWatchableJob`. `TestProductionExportIsXlsx`. `TestUnknownQueryParamIs400`.
- [ ] Implement, `make openapi`, `check:api`, gates. Mutation: drop the role gate on `/plants/{id}/sync` → matrix red. Commit.

### Task 8: Bills dashboard plant section (R290)

**Files:** `handlers_billing.go` (`dashboardDTO` gets plants from `solar.BillPlants`), `dto/bills.go`, billing HTTP test,
`web/src/features/bills/{plants-section.tsx,.test.tsx,.stories.tsx}`, `bills-page.tsx`, `_fixture.ts`, i18n `bills`.

- [ ] Failing tests. `TestBillDashboardPlantRows` (plant with netting analyzer and bill: exact production, prices, invoice amount; plant without: null analyzer fields). `TestBillDashboardPlantsHiddenForBA`. The web test renders rows, totals per currency and the no-analyzer "—", and the section is absent for `plants_not_in_scope`.
- [ ] Implement, gates (api integration, web). Update the old R235 assertions (ledger the supersession). Commit.

### Task 9: Web: `/ekorm/solar-plants`

**Files:** `web/src/features/solar-plants/{solar-plants-page,plant-selector,overview-tab,devices-tab,history-tab,alarms-tab,connection-status,summary-cards,revenue-cards}.tsx`
(+ tests + stories), `app/ekorm/solar-plants/page.tsx`, `nav-config.ts` (R295 hrefs, R296 permissions), i18n `solarPlants`, `responsive.spec.ts` SCREENS.

Behaviour: empty state when no plant is linked, with a link to `/ekorm/settings/account?tab=plants`. Selector across linked plants
(no "all plants" aggregate: legacy summed mixed-freshness live data; one plant at a time). "Verileri güncelle" →
`POST /sync` → watch the job → invalidate. Connection chip from `realtime.connection`. Overview: six summary cards
(null → "veri yok"; stale → badge with `as_of`), four revenue cards (per currency, `since` on total,
`partial` badge, unavailable state), production history (last 30 days daily) and current month daily charts, the environmental and weather panels (Task 12/10 components; this task renders their unavailable states until they exist, Part B wires them).
Devices: table + search + count + empty state. History: granularity toggle hour/day/month, date range, chart + table,
Excel download (`downloadFile`). Alarms: paged list, level/type chips, untranslated marker.

- [ ] Failing component tests for each tab (null-safe, units, unavailable states, search, download call). mockApi refuses a range over R284 limits. Stories data-free.
- [ ] Implement (ui-ux-pro-max first), then `pnpm lint typecheck test --maxWorkers=2 check:i18n-parity check:contrast`. Commit.

### Task 10: Web: settings plant tab: iSolar link modal and netting analyzer

**Files:** `features/settings/{isolar-link-dialog.tsx,.test.tsx,.stories.tsx}`, `plant-form.tsx` (+ netting analyzer select, link status block), `plants-tab.tsx`, i18n `settings`.

Modal: choose an iSolar credential (company's isolar credentials) → `GET /integrations/isolar/plants` → list with
installed kW and "başka santrale bağlı" disabled rows → confirm → `POST /isolar-link` → toast + watch sync job.
Unlink with confirmation. The recipients editor keeps saving through `PATCH /power-plants/{id}` (F6), which is one save path;
`PUT /alarm-recipients` exists for 05 §8 parity and shares `SetRecipients`' validation (R298).

- [ ] Failing tests: list, disabled linked rows, 409 message, the no-credential state linking to the company tab, the netting analyzer select listing company analyzers. Implement, web gates. Commit.

---

## Part B — Weather, Renewable Energy, Financial Analysis

### Task 11: Weather adapter, cache, route

**Files:** `internal/integration/weather/openmeteo.go` + test (httptest TLS), `internal/service/weather/weather.go` + test,
config (`EKOKOD_WEATHER_BASE_URL`), handler + DTO + matrix, worker/API wiring, `web/src/features/weather/weather-panel.tsx` (+ test, stories).

**Produces:** `func (s *Service) For(ctx, sc store.Scope, plantID, buildingID *uuid.UUID) (Weather, error)`, where `Weather{Available bool; Reason string; Current *Current; Days []Day}`.

- [ ] Failing tests. `TestWeatherNotConfigured`. `TestWeatherLocationNotConfiguredHasNoName` (the JSON has no `location` key). `TestWeatherCachesCurrentFor15Minutes` (second call: no HTTP). `TestPotentialBands` (19.9 → medium, 20 → high, 9.9 → low). `TestProviderErrorIsUnavailable`. Web: the unavailable states render their sentences, and no city is ever printed.
- [ ] Implement, gates. Mutation: default coordinates when missing → red. Commit.

### Task 12: Renewable panels (pure) and `TestNoSyntheticData`

**Files:** `internal/service/renewable/{panels.go,sources.go}` + `panels_test.go`, `nosynthetic_test.go`.

**Produces:** one builder per panel, `func BuildRealtime(in Inputs, now time.Time) Realtime` etc., `Inputs{Hourly, Daily []model.ConsumptionBucket; Bills []model.Bill; LastReadingAt *time.Time; Forecasts []model.Forecast; GridFactor *Factor; Equivalences map[string]Factor}`; every numeric field `*decimal.Decimal` plus `Unavailable map[string]string`.
`sources.go` exports `Sources map[string]map[string]Source` (panel → json field → `{Kind: "measured"|"derived"|"unavailable", From string}`), mirroring the R292 table.

`TestNoSyntheticData`: (1) every JSON field of every panel type appears in `Sources`, and every `Sources` entry exists (reflection). (2) Build every panel from fixture A and from A with every register ×2: each measured/derived field changes, so no constants. (3) Build twice from A: deep-equal, so no randomness. (4) Each unavailable field is null and has its reason. (5) Build from empty inputs: no field is a non-null zero unless its source says `derived` of an empty sum ("missing is not zero").

- [ ] Failing tests, including exact-value tests per panel from the R292 formulas (e.g. `TestPowerFactorFromTodaysBuckets`: imp 30, ind 40 → 0.600; `TestAccuracyIsHundredMinusMAPE`; `TestSystemStatusWorstOfAvailable`). Implement, then green. Mutation: return `decimal.NewFromInt(78)` for a health figure → (2) red. Commit.

### Task 13: Renewable + financial services and routes

**Files:** `service/renewable/service.go` (+ integration test), `service/financial/{financial.go,service.go}` (+ unit + `financial_integration_test.go`),
handlers + DTOs + matrix + tenancy sweep (`renewable` scope incl. BA/demo; `financial` A CA CR), OpenAPI, seeded equivalence rows (`emission_factors.json`, source + year).

`TestFinancialReconcilesFixture` (integration, the 09 acceptance): 2 buildings × 3 months of bills (TRY) and 2 plants with
daily totals and solar tariffs (one changing in month 2). It asserts every month's consumption = Σ fixture bill `net_consumption`,
cost = Σ `total_cost`, production = Σ totals, revenue = Σ production × effective feed-in, net = cost − revenue, offset/grid split, and year row = Σ months.
Plus `TestFinancialMonthWithoutBillsIsNull`, `TestFinancialNeverAddsCurrencies`, `TestRenewableScopeBuildingAdminOwnAnalyzerOnly`.

- [ ] Red, green, gates (both services, api integration, arch). Mutation: net only when production ≥ consumption (legacy) → reconciliation red. Commit.

### Task 14: Web: `/ekorm/renewable-energy`

**Files:** `web/src/features/renewable/{renewable-page,filter,production-tab,detailed-tab,panels/*}.tsx` + tests + stories, app route, i18n `renewable`, SCREENS.

Filter bar (building → analyzer, date range, period). Summary cards from `/renewable/overview`. Production tab: `/generation` chart
(active/inductive/capacitive toggles) + table. Detailed tab: the eight panels + energy balance (existing `/energy-balance`) + weather (Task 11).
Every unavailable field renders R165's explicit state with the reason's sentence. Charts: units, legends, table equivalents.

- [ ] Failing tests per panel (null-safe, reason sentences, no "0" for missing), then implement, then web gates. Commit.

### Task 15: Web: `/ekorm/financial-analysis`

**Files:** `web/src/features/financial/{financial-page,headline-cards,offset-card,tariff-panel,yearly-chart,monthly-table}.tsx` + tests + stories, route, i18n `financial`, SCREENS.

- [ ] Failing tests: per-currency lines, the explicit purchase/sale-missing messages, the coverage "12 ayın 9'u", the total row, the chart's data table. Implement, web gates. Commit.

### Task 16: Seed, e2e, accessibility, acceptance run

**Files:** `internal/seed/solar.go` (a linked grid plant with credential stub, devices with snapshots, 60 days of totals, faults, one netting analyzer, solar tariffs, equivalences), e2e specs
`solar-plants.spec.ts`, `renewable.spec.ts`, `financial.spec.ts`, a11y story run.

e2e (with the worker; the iSolar sync is not reachable in e2e, so the "Update data" case asserts the job failure state `credential_missing`/`isolar_unavailable` renders a message, not a crash).

- [ ] Write the specs, run the full e2e with a worker, then the a11y run for the new stories, then fix, then commit.

### Task 17: Whole-phase self-review and handoff

Tenancy/authorization first (every new route × six roles, plant ids across companies, the BA bills section), then no-synthetic-data audit (grep `rand`, literal numbers in panel code), money/currency, accessibility and design rules, then the Verification block below.
Rewrite `HANDOFF_NEXT_SESSION.md` for F10 and give the user the paste-ready Turkish prompt.

---

## Verification (the acceptance run)

```bash
go test ./internal/service/renewable/... -run TestNoSyntheticData -v
go test -race ./internal/integration/isolar/... ./internal/integration/weather/... ./internal/service/solar/... ./internal/service/weather/... ./internal/service/renewable/... ./internal/service/financial/...
sg docker -c "…integration env… go test ./internal/service/financial/... ./internal/service/solar/... ./internal/store/postgres/... ./internal/api/v1/... -tags=integration -count=1 -parallel 4"
# one at a time: ./internal/job ./internal/worker ./internal/scheduler (integration)
golangci-lint run --concurrency 2 --build-tags=integration ./internal/...
make check-generate && make openapi   # no diff
cd web && pnpm lint && pnpm typecheck && pnpm check:api && pnpm check:i18n-parity && pnpm check:contrast && pnpm test --maxWorkers=2 && pnpm build
pnpm exec playwright test tests/e2e/solar-plants.spec.ts tests/e2e/renewable.spec.ts tests/e2e/financial.spec.ts   # with E2E_API_PORT=18082, worker
```

## Acceptance map (09 §F9)

| Criterion | Where |
|---|---|
| Every renewable panel real or `available:false`; a test enumerates sources | R292, Task 12 `TestNoSyntheticData` |
| Efficiency, ROI, payback, grid quality, forecast accuracy trace to a formula or unavailable | R292 table (efficiency/ROI/payback unavailable; PF + accuracy formulas) |
| iSolar units normalised; asserted | R299, Task 2 |
| Plant alarms forwarded once and only once | R287, Task 5 |
| Financial figures reconcile against invoices and production | R294, Task 13 `TestFinancialReconcilesFixture` |
| No coordinates → "location not configured", no city | R291, Task 11 |

## Open questions for the product owner (defaults ship)

- **Q-F1** ROI and payback need an investment cost, and the data model has none. Should a plant (or building) get `investment_cost` + currency?
- **Q-F2** Financial Analysis nets company-wide production against consumption (legacy). Should only plants with a netting analyzer be offset?
- **Q-F3** The iSolar wire details added here (`data_type "2"`, `query_type "1"`, realtime field names) follow legacy code, not a live capture. Verify them in F14 with a real account.
- **Q-F4** Forwarded fault mails go to the plant's recipients only. Should company admins also be copied?
