# F6b — Core application screens (Dashboard, Consumption, Load Profile, Settings, Calendar) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans, **inline, one session, no subagents,
> no Workflow** (user ruling 2026-09-17). TDD per task; commit as soon as tests are green; then prove every new
> guard red with your own mutation (restore from a backup copy, never `git checkout -- file` / `git stash`); then
> read the task diff; merge `f6b/task-N` into `phase/f6b-screens` with `--no-ff`. Steps use `- [ ]`.

**Goal:** The product's spine on top of F6a: the Dashboard, Consumption, Load Profile, Settings (8 tabs) and Calendar
screens, the two small APIs they still need, e2e fixtures with real readings, and the `dashboard`, `consumption`,
`load-profile`, `settings` and `calendar` Playwright specs.

**Architecture:** Go gets four narrow additions (OpenAPI `company_id`, two permissions, `GET /jobs/{id}`,
`GET /consumption/grouped` over a new pure `internal/domain/grouping`) and richer `seed e2e` data. Each web screen is a
server `page.tsx` → a client container in `web/src/features/<screen>/` that reads the API through `$api` with
`useScopeParams()` → presentational Views (typed props, stories under `Features/<Screen>`) built only from F5
components. Shared plumbing (scope picker wired to `useSelection`, job polling, downloads, API error mapping, test API
mock) lands first.

**Tech stack:** Go 1.27 · chi · asynq (Inspector) · shopspring/decimal · Next.js 15 · React 19 · TanStack Query 5 ·
openapi-fetch / openapi-react-query · Recharts wrappers (F5) · Radix (F5) · next-intl · vitest · Playwright.

**Spec:** `docs/rewrite/09-implementation-plan.md` §F6; `01-project-context.md` §2 (role + settings-tab matrix), §5, §6,
§7.2, §7.3, §7.4, §7.15, §7.18; `05-api-contract.md` §1, §3–§6, §14–§16; `02-domain-rules.md` §3.6, §10.3, §10.4;
`07-design-system.md` (all); `design-system/bcem-energy/MASTER.md` **then** `OVERRIDES.md`; `10-removed-behaviours.md`
item 13 (sectoral comparison restored), item 14 (seasonal labels). Rulings: F5 D1–D26
(`2026-09-17-f5-design-system.md`), F6a R136–R189 + permission matrix (`2026-09-17-f6a-auth-api-skeleton.md`), F3 R82
(meteorological seasons). Handoff: `HANDOFF_NEXT_SESSION.md` "F6b must pick up".

## Global Constraints

- Money and energy are decimal strings end to end; the web never does float arithmetic on them. `Number()` only for
  plot geometry (D11); tables/tooltips format the original string with `formatNumber`/`formatQuantity` (`tr-TR`, D9).
- Timestamps evaluated in `Europe/Istanbul`; date props are ISO strings, weeks start Monday (D19).
- HTTP handlers: decode, authorise, call a service, encode. `internal/domain` has no I/O.
- Every scoped `$api.useQuery`/`useMutation` spreads `useScopeParams()` into `params.query` (R139); the query key
  therefore changes on a company switch.
- Read-only roles see no mutation controls (`useSession().can`), and the API still enforces (01 §2).
- Tokens only: no raw colour literals or default-palette classes outside `src/styles/**` (D5); `recharts` only inside
  `src/components/charts/**` (D10); every chart has units, legend, tooltip, data table (07 §5); > 6 series → the D21
  alert, so never pass more than 6.
- Every UI string in `messages/{tr,en}/<ns>.json`, registered in `messages/index.ts` (D7); Turkish default; wording from
  `docs/rewrite/appendix/i18n-tr.json` where a legacy string with that meaning exists.
- Loading = skeleton at final size; every list/table/chart has an empty state with a next step; every mutation toasts
  success/error; 422 `details` → field errors + `FormErrorSummary` (07 §9, 01 §6).
- Status = colour + icon + text; energy colours graphics-only; forecast neutral and dashed (07 §2.4).
- 375 / 768 / 1024 / 1440 px without page-level horizontal scroll; tables scroll inside their container (07 §7).
- UI work reads `MASTER.md` then `OVERRIDES.md` first (07 §0); OVERRIDES wins.
- Machine rules: prefix every command with
  `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`; run only
  touched packages, never `make test` / `go test ./...`; Go integration against the shared containers:
  `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <pkg> -tags=integration -count=1 -parallel 4`;
  web `pnpm test --maxWorkers=2`; Storybook / Playwright / `next build` one at a time, foreground, after `free -m` and
  `ps -eo rss,comm | grep java`; never touch foreign Gradle/java, `dolmusum-dev-postgres-1` or `:8080`.
- A task's gate runs the **whole** touched Go packages (integration tag) and the whole web unit suite, not only its new
  tests. Comments 1–3 lines, WHY only, cite ruling ids.

---

## Rulings (binding for F6b)

"Cost" = cost if the ruling is wrong. User decisions of 2026-09-17: R192, R193 built here; tariff templates / bulk
tariff / solar tariffs / icmal / `/bills/dashboard` → F8; iSolar link modal → F9.

| ID | Question | Ruling | Why | Cost |
|---|---|---|---|---|
| R190 | `company_id` is accepted by every authenticated route (R139) but absent from `openapi.json`, so the typed client rejects it. | `BuildOpenAPI` adds an optional query parameter `company_id` (`format: uuid`, description "Admin only: act for this company (R139)") to every operation whose route `Access != Public`. `schema.d.ts` regenerated. `TestOpenAPIDeclaresCompanyIDOnScopedRoutes`. | Typed scope params without casts. | Generator edit. |
| R191 | Permissions missing for two F6b controls: analyzer PATCH is A CA while `settings.analyzers` includes BA/BR; `POST /anomaly/check` is A CA BA. | Add `settings.analyzers.edit` (A, CA) and `anomaly.check` (A, CA, BA) to `auth.PermissionsFor`, the `Permission` enum, `permissions.fixture.json`, `TestPermissionsMatrix`. Amends R159. | UI gates on permissions only, never on role names (R159). | Two table rows. |
| R192 | Refresh feedback (01 §7.2 "progress and success/error feedback"). | `GET /jobs/{id}` (roles A CA BA, `Entity: job`, no audit). Looks the id up in queues `critical`, `default`, `low` through `asynq.Inspector.GetTaskInfo`. 404 `not_found` when: not found; type not in {`integration.refresh_analyzer`, `integration.sync_analyzers`, `integration.backfill`}; payload `CompanyID` ≠ scope company; payload `AnalyzerID` (refresh) not visible in scope (`Analyzers.Get`). Status: `pending`/`scheduled`/`aggregating` → `queued`; `active`/`retry` → `running`; `completed` → `succeeded`; `archived` → `failed`. Response `{id, type, status, completed_at?}` — never `last_err` (may carry provider text). Those three task constructors gain `asynq.Retention(time.Hour)` so a finished task stays queryable. Web polls every 2 s until terminal, gives up after 5 min with a neutral "still running" message. A refresh "succeeds" when its fetch tasks are enqueued; the success text says data appears within minutes. | User decision; honest about what the job does. | Retention memory in Redis (1 h). |
| R193 | Consumption "Detailed graphs" (01 §7.3): grouping + previous period + statistics. | `GET /consumption/grouped?analyzer_id|building_id&from&to&group_by&compare` (roles = GET /consumption). `group_by` ∈ `daily`, `week`, `day_type`, `season`, `season_day_type`; `compare=previous` optional; range ≤ 366 days else 400 `invalid_parameters`. Input = daily rows from `analysis.Rows` (building summed, R160). Keys: `daily` → `YYYY-MM-DD`; `week` → Monday date `YYYY-MM-DD` (legacy used week-of-month; Monday weeks per D19 — deliberate divergence); `day_type` → `weekday`\|`weekend` via `loadprofile.Classify` with the company config (weekend days + vacations, R137; events never); `season` → `<season>-<YYYY>` with meteorological seasons (R82), December counted in the winter that starts that December (`winter-2025` = Dec 2025–Feb 2026); `season_day_type` → `<season>-<YYYY>-<day_type>`. Group value per register (`active_import`, `reactive_inductive_import`, `reactive_capacitive_import`) = sum of non-nil daily values (nil when none); `days` = rows in the group; `partial` = any row partial, any nil register, or any suspect register. Groups ordered by key (day_type: weekday, weekend; season: chronological). `statistics` over the groups' `active_import`: `total`, `average` (total ÷ groups with a value, 3 dp half-even), `peak {key,value}`, `valley {key,value}`. `compare=previous` adds `previous {from,to,groups,statistics}` for the same number of days ending the day before `from`. | User decision; same weekend/season rules as the load profile, decimal in Go. | Legacy week labels differ. |
| R194 | e2e fixtures have no readings, so screens would only show empty states. | `seed e2e` fills hourly `load_profile` readings for A1, A2, B1 from `now − 75 days` (first run) or their last reading, up to the current hour, with the demo generator (profile index 0/1/2 → day/night 120/50, 20/8, 60/25) and refreshes hourly/daily aggregates; inserts one `issued` **building** bill for A1, previous calendar month, `total_cost 48250.75`, `active_import 18450.500`, lines energy/distribution/VAT, only when `Current` finds none. The demo company stays as F6a built it. Four `Üretim` buildings (A1, A2, B1, demo) keep the sectoral comparison available (≥ 3 peers, R162). | Handoff: "add seed data to `seed e2e`, not ad hoc in specs"; dashboard acceptance needs real seeded data. | Seed runtime ~2 s. |
| R195 | Screen structure. | `src/app/ekorm/<route>/page.tsx` (server: `generateMetadata` title, renders `<XPage/>`) → `src/features/<screen>/<screen>-page.tsx` (client container: queries, mutations, selection) → Views (props in, callbacks out, stories `Features/<Screen>/<View>`). Views never import `$api`. Tests: Views with plain props; containers with `mockApi` (T4). | D17 carried to screens; stories stay data-free for the a11y sweep. | — |
| R196 | Building/analyzer selection (F5 carry). | `ScopePicker` = `BuildingAnalyzerPicker` fed by `useAssets()` (`GET /buildings?include=analyzer_count,active_status&limit=500`, `GET /analyzers?limit=500`) and bound to `useSelection()`. When the store holds no (or an invisible) building, the first building by Turkish-collated name is selected; a building with exactly one analyzer auto-selects it (legacy `BuildingList`). "Active only" is page state. Dashboard, Consumption, Load Profile share the store, so selection survives navigation (01 §6). | Legacy parity + 01 §6. | — |
| R197 | Dates and defaults. | `istanbulToday(now = new Date()): string` (`Intl.DateTimeFormat('en-CA', {timeZone:'Europe/Istanbul'})`). Consumption: granularity `daily`, range `last6Months` (01 §6). Load profile: range `last6Months`. Dashboard chart: `monthly`, `thisYear`. Filters edit a draft; queries run on Apply only. | 01 §6 defaults; no fetch per keystroke. | — |
| R198 | Downloads and print. | `downloadFile(path, query, fallbackName)` fetches through the browser `api` client (refresh-on-401 applies), reads `Content-Disposition` filename, saves via an object URL. Consumption CSV/Excel → `/consumption/export?format=csv|xlsx`; load profile Excel → `/load-profile/export`; sectoral comparison CSV client-side with `toCsv` (D22), file `<building>-sektor-karsilastirma.csv`. Print = `window.print()`; shell chrome, filter bar and actions carry `print:hidden`. | 01 §6 exports; one download path. | — |
| R199 | Mutations. | `useApiMutation(method, path, {onSuccessToast, invalidate})` wraps `$api.useMutation`: success → toast (`success`), error → toast (`danger`, API `error.message`); 422 → `fieldErrors(body): Record<string,string>` (first code per field, translated via `forms.errors.<code>` with fallback `forms.errors.invalid`) for inline errors + `FormErrorSummary`. Deletes ask in a `Dialog` (`common.confirmDelete`). No `Idempotency-Key` sent. Invalidation by path prefix. | 01 §6 toasts; 07 §9 errors. | — |
| R200 | Settings routing and gating. | `/ekorm/settings?tab=<id>`; ids and permission: `account` (all), `integrations` (`settings.integrations`), `company` (`settings.company`), `buildings` (`settings.buildings`), `plants` (`settings.plants`), `analyzers` (`settings.analyzers`), `users` (`settings.users`), `smtp` (`settings.smtp`), in that order (01 §2 table). Missing/unknown/forbidden tab → `account`; tab change `router.replace` keeps other params. Edit controls: company `settings.company.edit`; buildings/plants/users `write`; analyzer assign/multiplier/power/coordinates `settings.analyzers.edit`; analyzer refresh `analyzers.refresh`; credentials list actions `integrations.credentials`; credential **creation** and company list CRUD `admin.companies`; definitions `settings.integrations`; SMTP `settings.smtp`. Account tab for `demo`: profile read-only with an info alert; no password form, no session list (shared account). | 01 §2 matrix + R159/R191; the iSolar callback already redirects to `?tab=company`. | — |
| R201 | Credential creation for company admins. | Credential setup (provider/subtype from `GET /integration-definitions`, which is A only) is built for admins, as in legacy (`AdminCompanyTab`). CA sees the configured list with verify, discover, backfill, replace secret and delete (05 §15). Form per provider: `osos`/`aril` username + password; `gridbox` + `settings.use_billing_indexes` switch; `pm5340` `pm5340_url` (https) + `installation_number`; `isolar` extra `app_key`, `secret_key`, `app_id` + `isolar_region`, then "iSolarCloud'a bağlan" → `GET /integrations/isolar/authorize-url` → `window.location.assign(url)`; `?isolar=ok|error` on return → toast. Discovered analyzers arrive unassigned; the success toast links to `?tab=analyzers&unassigned=1`. | Legacy flow; API keeps secrets write-only (R177). | CA setup UI later if asked. |
| R202 | Calendar views without a new dependency. | Hand-built: **month** (Mon-first 6×7 grid, up to 3 chips + "+N daha" popover), **week**/**day** (00–24 time grid, 48 px/h, all-day row, events positioned by Istanbul minutes, overlaps split into lanes by a greedy interval algorithm `layoutLanes(events)`), **agenda** (days in range with events, chronological). Toolbar: previous / today / next, view `RadioGroup` as segmented control, range title. Visible range drives `GET /calendar/events?from&to`. Weekend days (company config) and vacation periods shade the month/week columns and carry a text label ("Tatil" / period description). All-day event = `starts_at` D 00:00+03:00, `ends_at` (last day + 1) 00:00+03:00. | react-big-calendar would add its own CSS/visual defaults (07 §0, §6). | ~1 task of layout code. |
| R203 | Event colours are data (`^#[0-9a-f]{6}$`) but raw hex is banned outside `src/styles/**` (D5). | `src/styles/event-palette.ts`: six named colours (emerald `#059669`, cyan `#0e7490`, amber `#a16207`, violet `#7e22ce`, red `#b91c1c`, slate `#64748b`) stored as hex. Rendered only as a 4 px start border + 8 px dot via inline `style` from the value; the chip text stays `text-foreground` on `surface`. Unknown stored colours render the same way. | Colour is a user category, never the only carrier of meaning; contrast untouched. | — |
| R204 | Dashboard composition (01 §7.2). | `lg`: 12 columns — map panel `col-span-6`, building list `col-span-3`, bill + reactive cards `col-span-3`; consumption panel and sectoral comparison full width; below `lg` single column in the same order. Card grids use the D26 stagger. **Map panel:** `Tabs` Binalar / Analizörler; header "Aktif N · Pasif M" from R163 fields; markers only for entities with both coordinates, plus the note "N kaydın konumu yok"; marker select = select building (analyzer map: select analyzer + its building); `Map` gains `focusId?: string` (fly to and outline the marker; list fallback scrolls to and highlights the row). **Building list:** Turkish-normalised search over name + address, active-only, select-all/deselect-all checkboxes filter the map markers (legacy), row click selects the building, expanded row lists analyzers as a radio group, "Haritada göster" icon button sets `focusId`. **Latest bill:** `GET /bills/latest?scope=building&subject_id=<selected building>`; no building selected and `settings.company` → `scope=company&subject_id=<company>`; 404 → empty state "Henüz hesaplanmış fatura yok" with link to Bills; shows period (`formatMonth`), `active_import` kWh, `total_cost` ₺, status badge, link `/ekorm/bills?bill=<id>`. **Reactive:** `GET /consumption/reactive-status?month&building_id?`; `MonthPicker` (max current month) + `Select` "Tüm binalar" / selected building; `ReactiveStatusCard` fed by the highest inductive and highest capacitive analyzers (their own limits); names of that analyzer and building under each ratio; advisory "Sınırlar içinde" when no analyzer has `penalty_applies`, else "Kompanzasyon (güç faktörü düzeltmesi) gerekli"; "Tüm binalar" `Dialog` with a sortable `DataTable` (building, analyzer, inductive %, capacitive %, limits, exempt reason, penalty, "Faturayı gör" → `/ekorm/bills?building_id=…&period=YYYY-MM`). **Consumption panel:** `PeriodFilterBar` + `Switch`es active/inductive/capacitive; `LineChart` series kinds `consumption`/`reactiveInductive`/`reactiveCapacitive` over the first 50 rows with `Alert tone=info` "Grafik ilk 50 noktayı gösteriyor (toplam N)" when more; `DataTable` page size 10 with total; year-over-year `BarChart` (kinds `current`/`previous`, active kWh) when granularity is `monthly` and the range lies in one calendar year (previous year same months fetched), else caption "Yıllık karşılaştırma aylık görünümde gösterilir"; `RefreshActions` for the selected analyzer. **Sectoral comparison:** R205. | Spec layout; legacy behaviours kept. | — |
| R205 | Sectoral comparison table. | For the selected building: `GET /buildings/{id}/comparison`. Table rows "<building>" and "Sektör ortalaması"; columns daily consumption (kWh/gün), monthly (kWh/ay), CO₂ (kg/ay), per capita (kWh/kişi·ay), per area (kWh/m²·ay); rank lines "Kişi başı: 4 bina içinde 2." for per capita, per area, monthly (`rank`/`ranked`; `ranked=0` → "sıralanamadı"); header "Sektör: Üretim · 4 bina". States: no building → hint; 409 `building_sector_missing` → alert with link to `?tab=buildings` when `write`; `available=false` → "Sektörde karşılaştırma için yeterli bina yok (en az 3)". `ExportMenu formats=['csv']` → R198 CSV with the table's header, the two rows and three rank rows. | 01 §7.2, 10 item 13, R162. | — |
| R206 | Consumption table columns (01 §7.3; `TestConsumptionColumnSet`). | In order: period; indexes `active_import_index`, `reactive_inductive_import_index`, `reactive_capacitive_import_index`, `t1_import_index`, `t2_import_index`, `t3_import_index`, `active_export_index`, `reactive_inductive_export_index`, `reactive_capacitive_export_index`, `t1_export_index` (U1), `t2_export_index` (U2), `t3_export_index` (U3); consumption `active_import` (kWh), `reactive_inductive_import` (kVArh), `reactive_capacitive_import` (kVArh), `inductive_ratio` (%), `capacitive_ratio` (%), `t1_import`, `t2_import`, `t3_import`; generation `active_export`, `reactive_inductive_export`, `reactive_capacitive_export`, `t1_export` (U1), `t2_export` (U2), `t3_export` (U3); `max_demand_kw`. All visible by default; column visibility on. Period cell = `formatPeriod(period_start, granularity, locale)` (hourly `14.03.2025 09:00`, daily `14.03.2025`, monthly `Mart 2025`, yearly `2025`) + `DataQualityBadge` (`incomplete` when `partial`, `suspect` when `suspect_registers` non-empty, reason lists the registers). Ratios shown with `fractionToPercent`. Row action "Alarm kontrolü" (analyzer subject, granularity hourly or daily, `anomaly.check`) → `POST /anomaly/check {analyzer_id, ts: period_start, actual: active_import}` → `Dialog`: date, actual kWh, and either the verdict (F13) or `Alert tone=info` "Tahmin modeli şu anda kullanılamıyor" for `available=false` (R165). | 01 §7.3 full set; 10 item 12 (no fake verdict). | — |
| R207 | Load profile screen (01 §7.4). | Filter: `ScopePicker` (analyzer required; building-only → `EmptyState` "Analizör seçin"), `DateRangePicker` (`last6Months`), Apply. One `GET /load-profile` (all ten keys) + `GET /load-profile/statistics`. Tabs `daily` / `seasonal` / `compare` / `details`. **Labels come from the key**: `profileLabel(t, key)` = `t('dayType.<d>')` for `weekday`/`weekend`, else `` `${t('season.<s>')} – ${t('dayType.<d>')}` `` (tr: Kış/İlkbahar/Yaz/Sonbahar, Hafta içi/Hafta sonu; en: Winter/Spring/Summer/Autumn, Weekday/Weekend). Daily: two `LineChart`s (x `00:00`…`23:00`, kWh) with the legacy notes and "N gün" from `days`. Seasonal: eight charts in a 2-column grid, each with its note. Compare: `MultiSelect` of the ten profiles (default weekday + weekend; the 7th selection is refused with the description "En fazla 6 profil", D21) → one `LineChart` (distinct dash patterns come from `seriesStyle`). Details: `DataTable` rows per profile: max, min, hour of max (`HH:00`), mean, stddev (kWh), range, load factor (%, `fractionToPercent`) + `ExportMenu formats=['excel']` → `/load-profile/export`. Caption under the filter: "Hafta sonu günleri: Cumartesi, Pazar (şirket takvimi \| varsayılan) · N tatil dönemi" with a link to `/ekorm/calendar`. | 10 item 14 structurally; R137. | — |
| R208 | Layout redirect without `next` (F6a open item). | `src/middleware.ts` sets request header `x-ekokod-path` (pathname + search) on every forwarded `/ekorm` request; `app/ekorm/layout.tsx` redirects a missing session to `authRedirectFor(code, path)` so `next` survives. | Handoff open item. | — |
| R209 | Admin company switcher lists ≤ 500 companies (F6a open item). | Kept. Settings › Company (admin) is the paged company list (`limit` 50 + "Daha fazla yükle" by `next_cursor`) with a "Bu şirket adına çalış" action that sets `useSelection().companyId`. | Enough for launch; paged list exists where it matters. | Switcher search later. |
| R210 | 01 §7.15: GridBox setup enters wiring numbers and picks a target building, but GridBox has no discovery endpoint (06 §3) and `credentials.Configure` upserts an analyzer only for PM5340, unassigned. | `POST`/`PATCH /integration-credentials` accept `wiring_numbers: []string` (gridbox only; 1–50 entries, each `^[A-Za-z0-9._-]{1,100}$`, unique) and `building_id` (gridbox + pm5340 only, must be a visible live building else 422 `details.building_id=["invalid"]`). Every wiring number is upserted as an analyzer of the credential's (provider, subtype) assigned to `building_id` when given; an existing analyzer with that installation number keeps its building unless `building_id` is supplied; numbers left out of a later call are **not** deleted (no silent data loss — the Analyzers tab deactivates). PM5340 keeps `installation_number` and now honours `building_id`. Any other provider sending either field → 400 `invalid_body`. | User decision 2026-09-17; GridBox customers must be onboardable from the UI. | One service method; wrong assignment is fixable in the Analyzers tab. |

---

## File map

```
internal/api/v1/openapi.go (+ openapi_test.go)                   R190
internal/auth/permissions.go (+ _test), web/src/lib/session/permissions.fixture.json   R191
internal/job/{analyzer_refresh.go,integration.go}                 Retention (R192)
internal/service/jobs/{jobs.go,jobs_test.go}                      R192 (Inspector behind an interface)
internal/api/v1/{handlers_jobs.go,dto/jobs.go}                    R192
internal/domain/grouping/{grouping.go,grouping_test.go}           R193 pure
internal/service/loadprofile/service.go                           + CalendarConfig
internal/service/analysis/grouped.go (+ _test)                    R193
internal/api/v1/{handlers_analysis.go,dto/analysis.go}            + consumption.grouped
internal/api/v1/*_integration_test.go                             HTTP tests for jobs/grouped; matrix rows
internal/seed/{fixtures.go,readings.go}, internal/seed/demo.go    R194 (generator extracted)
internal/apiwire/wiring.go                                        jobs service + inspector
web/src/lib/api/{schema.d.ts,download.ts,mutation.ts,problem.ts}  R190, R198, R199
web/src/lib/dates.ts                                              R197
web/src/test/api-mock.ts                                          container tests
web/src/features/scope/{use-assets.ts,scope-picker.tsx}           R196
web/src/features/jobs/{use-job.ts,refresh-actions.tsx}            R192
web/src/components/ui/stagger-grid.tsx, web/src/app/globals.css   D26
web/src/components/map/map.tsx                                    focusId (R204)
web/src/middleware.ts, web/src/app/ekorm/layout.tsx               R208
web/src/app/ekorm/page.tsx                                        Dashboard
web/src/features/dashboard/{dashboard-page.tsx,map-panel.tsx,building-list.tsx,latest-bill-card.tsx,
  reactive-panel.tsx,consumption-panel.tsx,sector-comparison.tsx}
web/src/app/ekorm/consumption/page.tsx
web/src/features/consumption/{consumption-page.tsx,summary-cards.tsx,consumption-table.tsx,columns.ts,
  alarm-check-dialog.tsx,detailed-graphs.tsx,format-period.ts}
web/src/app/ekorm/load-profile/page.tsx
web/src/features/load-profile/{load-profile-page.tsx,profile-label.ts,daily-tab.tsx,seasonal-tab.tsx,compare-tab.tsx,details-tab.tsx}
web/src/app/ekorm/settings/page.tsx
web/src/features/settings/{settings-page.tsx,tabs.ts,account-tab.tsx,company-tab.tsx,company-list.tsx,
  credentials-section.tsx,credential-form.tsx,integrations-tab.tsx,smtp-tab.tsx,buildings-tab.tsx,building-form.tsx,
  plants-tab.tsx,plant-form.tsx,analyzers-tab.tsx,users-tab.tsx,user-form.tsx}
web/src/app/ekorm/calendar/page.tsx
web/src/features/calendar/{calendar-page.tsx,range.ts,layout-lanes.ts,month-view.tsx,time-grid-view.tsx,
  agenda-view.tsx,event-dialog.tsx,vacation-dialog.tsx}, web/src/styles/event-palette.ts
web/messages/{tr,en}/{dashboard,consumption,loadProfile,settings,calendar}.json (+ common/forms/domain additions)
web/tests/e2e/{dashboard,consumption,load-profile,settings,calendar}.spec.ts, fixtures.ts additions
```

Each View gets `*.stories.tsx` (title `Features/<Screen>/<View>`) and each container/View a `*.test.tsx` beside it.
In the interface blocks below, a trailing `…` in a props type stands for the shared form/list props
`{ loading?: boolean; submitting?: boolean; fieldErrors?: Record<string, string>; onCancel?(): void }` — spell them out in code.

---

## Task 1 — OpenAPI `company_id` and permissions (R190, R191)

**Files:** Modify `internal/api/v1/openapi.go`, `internal/api/v1/openapi_test.go`, `internal/auth/permissions.go`,
`internal/auth/permissions_test.go`, `internal/api/v1/dto` (Permission enum source if separate), `internal/api/v1/openapi.json`,
`web/src/lib/session/permissions.fixture.json`, `web/src/lib/api/schema.d.ts`, `web/src/lib/session/permissions.test.ts`.

**Produces:** `paths['/api/v1/buildings']['get']['parameters']['query']['company_id']?: string` (and on every non-public
operation); permissions `'settings.analyzers.edit'` (admin, company_admin) and `'anomaly.check'` (admin, company_admin,
building_admin).

- [ ] **Failing tests:**
  - `TestOpenAPIDeclaresCompanyIDOnScopedRoutes` (`openapi_test.go`): for every `Table()` row with `Access != Public`, the
    built document's operation has exactly one `in: query, name: company_id` parameter, not required, `format: uuid`;
    public rows (`auth.login`, `system.openapi`, `isolar.callback`, …) have none.
  - `TestPermissionsMatrix` literal slices gain the two permissions (A, CA both; BA `anomaly.check` only; CR, BR, D neither).
  - `TestPermissionsFixtureMatchesTable` goes red until the fixture is updated (existing guard).
  - web `permissions.test.ts`: `fixture.building_admin` contains `anomaly.check` and not `settings.analyzers.edit`.
- [ ] Red: `go test ./internal/api/v1/ ./internal/auth/ -count=1 -run 'TestOpenAPI|TestPermissions'`.
- [ ] Implement; `make openapi` (regenerates `openapi.json` + `schema.d.ts`); update fixture.
- [ ] Green gate: `go test ./internal/auth/ ./internal/api/v1/... -count=1` (unit) and integration run of `./internal/api/v1/`;
  `golangci-lint run --concurrency 2 ./internal/auth/... ./internal/api/...`; web `pnpm typecheck && pnpm check:api && pnpm test --maxWorkers=2`.
- [ ] Commit `feat(f6b): task-1 — company_id in OpenAPI, analyzer-edit and anomaly-check permissions`.
- [ ] **Mutations:** add `company_id` to public routes too; give `building_admin` `settings.analyzers.edit`.

## Task 2 — `GET /jobs/{id}` (R192)

**Files:** Create `internal/service/jobs/jobs.go`, `jobs_test.go`, `internal/api/v1/handlers_jobs.go`, `dto/jobs.go`,
`internal/api/v1/jobs_http_integration_test.go`. Modify `internal/job/analyzer_refresh.go`, `internal/job/integration.go`
(Retention), `internal/apiwire/wiring.go`, `authz_matrix_test.go` (row `"GET /jobs/{id}": "A CA BA"`),
`tenancy_sweep_integration_test.go` (`idFor("job")`), `openapi.json`, `schema.d.ts`.

**Interfaces:**
```go
package jobs
type Status string // "queued" | "running" | "succeeded" | "failed"
type View struct{ ID, Type string; Status Status; CompletedAt *time.Time }
type Inspector interface{ GetTaskInfo(queue, id string) (*asynq.TaskInfo, error) }
type Deps struct{ Inspector Inspector; Analyzers store.AnalyzerRepository; Queues []string }
func New(d Deps) (*Service, error)
func (s *Service) Get(ctx context.Context, sc store.Scope, id string) (View, error) // store.ErrNotFound for every hidden case

package dto
type JobIDPath struct{ ID string `path:"id" validate:"required,max=128"` }
type Job struct{ ID string `json:"id" required:"true"`; Type string `json:"type" required:"true"`
  Status string `json:"status" required:"true" enum:"queued,running,succeeded,failed"`; CompletedAt *time.Time `json:"completed_at,omitempty"` }
```
- [ ] **Failing tests:**
  - `TestJobStatusMapping` (unit, fake inspector): states pending, scheduled, aggregating → `queued`; active, retry →
    `running`; completed (CompletedAt set) → `succeeded` with `CompletedAt`; archived → `failed`.
  - `TestJobHiddenOutsideScope` (unit): payload `CompanyID` of another company → `ErrNotFound`; refresh payload whose
    `AnalyzerID` the fake `Analyzers.Get` reports `ErrNotFound` (BA scope) → `ErrNotFound`; type `billing.generate` → `ErrNotFound`;
    `asynq.ErrTaskNotFound` in every queue → `ErrNotFound`; the id found in `low` after `critical`/`default` miss → view.
  - `TestRefreshTasksAreRetained` (integration, `internal/job`, shared Redis): enqueue each of `NewRefreshAnalyzerTask`,
    `NewSyncAnalyzersTask`, `NewBackfillTask` through `job.NewClient` and assert the returned `*asynq.TaskInfo.Retention ==
    time.Hour` (asynq keeps task options private, so the enqueued info is the only observable).
  - `TestJobStatusHTTP` (integration, shared Redis): CA posts `/analyzers/{A1}/refresh` with a configured fake credential
    (fixture helper used by `TestAnalyzerRefreshEnqueues`) → 202 `job_id`; `GET /jobs/{job_id}` → 200 `status=queued`,
    `type=integration.refresh_analyzer`; BA of A1 → 200; CA of company B → 404; CR → 403; response body has no `last_err`/`error` key.
- [ ] Red → implement (Inspector from `asynq.NewInspector(job.RedisOpt(cfg.Redis))`, closed in `Built.Close`) → `make openapi`.
- [ ] Green gate: `./internal/job/ ./internal/service/jobs/ ./internal/api/v1/ ./internal/apiwire/` integration; lint.
- [ ] Commit `feat(f6b): task-2 — job status endpoint with tenant checks and retained refresh tasks`.
- [ ] **Mutations:** skip the `CompanyID` comparison; map `retry` to `failed`; drop the analyzer visibility check (BA sweep or unit red).

## Task 3 — `GET /consumption/grouped` (R193)

**Files:** Create `internal/domain/grouping/{grouping.go,grouping_test.go}`, `internal/service/analysis/grouped.go`,
`grouped_test.go`. Modify `internal/service/loadprofile/service.go` (+ test), `internal/service/analysis/service.go`
(Profiles interface gains `CalendarConfig`), `internal/api/v1/handlers_analysis.go`, `dto/analysis.go`,
`analysis_http_integration_test.go`, `authz_matrix_test.go` (`"GET /consumption/grouped": "A CA CR BA BR D"`), sweep
mapping, `openapi.json`, `schema.d.ts`.

**Interfaces:**
```go
package grouping // imports only domain/loadprofile and decimal
type By string // "daily" | "week" | "day_type" | "season" | "season_day_type"
type Day struct{ Date time.Time; Active, Inductive, Capacitive *decimal.Decimal; Partial bool }
type Bucket struct{ Key string; Days int; Active, Inductive, Capacitive *decimal.Decimal; Partial bool }
type Extreme struct{ Key string; Value decimal.Decimal }
type Statistics struct{ Total, Average *decimal.Decimal; Peak, Valley *Extreme }
func Valid(by By) bool
func Group(days []Day, by By, cfg loadprofile.Config) []Bucket
func Stats(buckets []Bucket) Statistics

package loadprofile // service
func (s *Service) CalendarConfig(ctx context.Context, sc store.Scope, r store.TimeRange) (domainlp.Config, error)

package analysis
type GroupedInput struct{ Subject; From, To time.Time; By grouping.By; ComparePrevious bool }
type GroupedPeriod struct{ From, To time.Time; Buckets []grouping.Bucket; Statistics grouping.Statistics }
type Grouped struct{ Current GroupedPeriod; Previous *GroupedPeriod }
func (s *Service) Grouped(ctx context.Context, sc store.Scope, in GroupedInput) (Grouped, error)
```
DTO: `ConsumptionGroupedQuery{analyzer_id?, building_id?, from, to (dates), group_by (enum), compare? (enum "previous")}` →
`ConsumptionGrouped{group_by, current: GroupedPeriod, previous?: GroupedPeriod}`; `GroupedPeriod{from, to, groups: [{key, days,
active_import?, reactive_inductive_import?, reactive_capacitive_import?, partial}], statistics: {total?, average?, peak?{key,value},
valley?{key,value}}}`.

- [ ] **Failing tests** (`grouping_test.go`, Istanbul location, weekend Sat+Sun unless stated):
  - `TestGroupWeekStartsMonday`: days 2025-03-09 (Sun) and 2025-03-10 (Mon) with active `"10"`, `"20"` → keys `2025-03-03`, `2025-03-10`.
  - `TestGroupDayTypeUsesCompanyConfig`: weekend {Fri, Sat}, vacation 2025-03-12 (Wed); days Wed 12 (`"5"`), Thu 13 (`"7"`), Fri 14 (`"11"`),
    Sun 16 (`"13"`) → `weekday` = `"20"` (Thu + Sun), days 2; `weekend` = `"16"` (Wed vacation + Fri), days 2.
  - `TestGroupSeasonDecemberStartsWinter`: 2025-12-15 and 2026-01-15 → one key `winter-2025`; 2026-03-01 → `spring-2026`;
    order `winter-2025`, `spring-2026`.
  - `TestGroupSeasonDayTypeKeys`: 2025-07-05 (Sat) → `summer-2025-weekend`.
  - `TestGroupSumsNonNilAndMarksPartial`: inductive nil on one of two days → inductive = other day's value, `Partial=true`;
    all nil → nil.
  - `TestStatsAverageHalfEven`: bucket actives `"1"`, `"1"`, `"2"` → total `"4"`, average `"1.333"`, peak key of `"2"`,
    valley = first bucket with `"1"`; a bucket with nil active is ignored.
  - `TestCalendarConfigMatchesProfiles` (service/loadprofile unit with fakes): the config used by `Profiles` equals `CalendarConfig`.
  - `TestGroupedPreviousPeriod` (analysis unit, fake Series): from 2025-03-10 to 2025-03-16 → previous 2025-03-03..2025-03-09;
    range of 367 days → `invalid_parameters`.
  - `TestConsumptionGroupedHTTP` (integration): demo-style readings for A1 over two weeks; `group_by=day_type&compare=previous`
    → 200, keys `weekday`, `weekend`, sums equal the sums of `/consumption?granularity=daily` rows computed in the test with
    `decimal`; BA with A2's analyzer → 404; `group_by=month` → 400.
- [ ] Red → implement → `make openapi`.
- [ ] Green gate: `./internal/domain/grouping/ -race`, `./internal/service/loadprofile/ ./internal/service/analysis/ ./internal/api/v1/`
  integration; `internal/arch` (domain import guard) once; lint.
- [ ] Commit `feat(f6b): task-3 — grouped consumption with company calendar and meteorological seasons`.
- [ ] **Mutations:** Sunday-start weeks; count calendar events as vacation (config from events); treat nil as zero in sums;
  December in the previous winter.

## Task 3b — GridBox wiring numbers and target building (R210)

**Files:** Modify `internal/credentials/service.go` (`ConfigureInput`/`UpdateInput`, `upsertPM5340Analyzer` →
`upsertManualAnalyzers`), `internal/credentials/service_test.go`, `internal/credentials/credentials_integration_test.go`,
`internal/api/v1/dto/assets.go` (credential DTOs live there), `internal/api/v1/handlers_integrations` section of
`handlers_tenancy.go`/wherever the credential handlers sit (`git grep -n "integration_credentials.create"`),
`internal/api/v1/credentials_leak_integration_test.go` (new fields carry no secrets), `openapi.json`, `schema.d.ts`.

**Interfaces:**
```go
// credentials
type ConfigureInput struct{ …existing…; WiringNumbers []string; BuildingID *uuid.UUID }
type UpdateInput struct{ …existing…; WiringNumbers []string; BuildingID *uuid.UUID }
func (s *Service) upsertManualAnalyzers(ctx context.Context, sc store.Scope, def model.IntegrationDefinition,
    numbers []string, buildingID *uuid.UUID) error // pm5340: exactly one number (existing retire-and-recreate rule)
```
DTO additions: `wiring_numbers []string \`json:"wiring_numbers,omitempty" validate:"omitempty,max=50,dive,required,max=100"\``
and `building_id *uuid.UUID \`json:"building_id,omitempty"\`` on create and update.

- [ ] **Failing tests:**
  - `TestGridboxWiringNumbersCreateAnalyzers` (integration): configure gridbox with `["1001","1002"]` and building A1 → two active
    analyzers of (gridbox, subtype) with those installation numbers, both `building_id = A1`; re-running with `["1001","1002","1003"]`
    → three analyzers, no duplicates; re-running with `["1001"]` → the other two still exist (nothing deleted).
  - `TestWiringNumbersRejectedForOtherProviders` (HTTP): `provider=osos` with `wiring_numbers` → 400 `invalid_body`.
  - `TestCredentialBuildingMustBeVisible` (HTTP): CA of company A passing company B's building id → 422
    `details.building_id=["invalid"]`, and no analyzer row written.
  - `TestPM5340HonoursBuildingID` (integration): configure pm5340 with `installation_number` + building A2 → analyzer assigned to A2.
  - `TestGridboxWiringNumberFormat` (unit): `"1001; drop"`, `""`, 101 chars, duplicates → `ErrInvalidWiringNumber`/422 codes.
  - `TestCredentialsNeverLeak` still green with the two new fields exercised.
- [ ] Red → implement → `make openapi`.
- [ ] Green gate: `./internal/credentials/ ./internal/api/v1/ ./internal/service/assets/` integration; lint.
- [ ] Commit `feat(f6b): task-3b — GridBox wiring numbers and target building on credential setup`.
- [ ] **Mutations:** skip the building visibility check; delete analyzers missing from a later `wiring_numbers` list.

## Task 4 — e2e fixture readings and bill (R194) + web plumbing

Go part and the shared web plumbing every screen task consumes.

**Files (Go):** Create `internal/seed/readings.go`; modify `internal/seed/demo.go` (use it), `internal/seed/fixtures.go`,
`internal/seed/fixtures_integration_test.go`.
**Files (web):** Create `web/src/lib/dates.ts`, `web/src/lib/api/{types.ts,download.ts,mutation.ts,problem.ts}`,
`web/src/test/api-mock.ts`, `web/src/features/scope/{use-assets.ts,scope-picker.tsx}`,
`web/src/features/jobs/{use-job.ts,refresh-actions.tsx}`, `web/src/components/ui/stagger-grid.tsx` (+ tests, stories for
`ScopePicker` View, `RefreshActions`, `StaggerGrid`), `web/messages/{tr,en}/{dashboard,consumption,loadProfile,settings,calendar}.json`
(seeded with the keys this task uses; later tasks append). Modify `web/messages/index.ts`, `web/src/app/globals.css`
(stagger keyframes), `web/src/components/map/map.tsx` (`focusId`), `web/src/middleware.ts`, `web/src/app/ekorm/layout.tsx`
(R208) and their tests, `web/messages/{tr,en}/forms.json` (`errors.*` codes: `required`, `invalid`, `email`, `min`, `max`,
`oneof`, `not_assignable`, `gtefield`, `password_*` reuse from auth).

**Interfaces:**
```go
package seed
type readingProfile struct{ day, night int64 } // 0: 120/50, 1: 20/8, 2: 60/25
func fillHourly(ctx context.Context, pool *pgxpool.Pool, sc store.Scope, ids []uuid.UUID, profiles []int, history time.Duration, now time.Time) (int, error)
```
```ts
// lib/api/types.ts — the only place schema aliases are declared; every feature imports from here
import type { components } from './schema';
type S = components['schemas'];
export type Building = S['Building']; export type BuildingDetail = S['BuildingDetail']; export type Analyzer = S['Analyzer'];
export type ConsumptionRow = S['ConsumptionRow']; export type ConsumptionSummary = S['ConsumptionSummary'];
export type ConsumptionGrouped = S['ConsumptionGrouped']; export type LoadProfiles = S['LoadProfiles'];
export type LoadProfileStatistics = S['LoadProfileStatistics']; export type ReactiveStatus = S['ReactiveStatus'];
export type ReactiveAnalyzer = S['ReactiveAnalyzer']; export type Bill = S['Bill']; export type BuildingComparison = S['BuildingComparison'];
export type CalendarEvent = S['CalendarEvent']; export type Vacations = S['Vacations']; export type VacationPeriod = S['VacationPeriod'];
export type Company = S['Company']; export type CompanyDetail = S['CompanyDetail']; export type User = S['User'];
export type Plant = S['Plant']; export type PlantDetail = S['PlantDetail']; export type PlantDevice = S['PlantDevice'];
export type SMTPSettings = S['SMTPSettings']; export type IntegrationDefinition = S['IntegrationDefinition'];
export type IntegrationCredential = S['IntegrationCredential']; export type Session = S['Session']; export type Me = S['Me'];
export type Role = S['Role']; export type Provider = S['Analyzer']['provider']; export type TariffSummary = S['TariffSummary'];
export type AnomalyCheck = S['AnomalyCheck']; export type Job = S['Job'];
// …plus the request aliases used by forms (BuildingCreateRequest, PlantUpdateRequest, VacationsPutRequest, …).
/** A next-intl namespace function, narrowed to what Views need. */
export type Translator = (key: string, values?: Record<string, string | number>) => string;

// lib/dates.ts
export function istanbulToday(now?: Date): string;                       // 'YYYY-MM-DD'
export function addDays(iso: string, days: number): string;
export function formatHour(hour: number): string;                        // 9 → '09:00'
// lib/api/problem.ts
export function fieldErrors(body: unknown, t: (code: string) => string): Record<string, string>;
export function errorMessage(body: unknown, fallback: string): string;
// lib/api/mutation.ts
export function useApiMutation<M extends 'post'|'put'|'patch'|'delete', P extends PathsWithMethod<paths, M>>(
  method: M, path: P, o?: { success?: string; invalidate?: string[] }): UseMutationResult<…> & { fieldErrors: Record<string,string> };
// lib/api/download.ts
export async function downloadFile(path: string, query: Record<string, string | undefined>, fallbackName: string): Promise<void>;
// test/api-mock.ts
export type MockRoute = unknown | ((req: Request) => Response | Promise<Response>);
export function mockApi(routes: Record<string, MockRoute>): { calls: Request[]; restore(): void }; // key 'GET /api/v1/buildings'; unmatched → 404 envelope
// features/scope
export type Assets = { buildings: Building[]; analyzers: Analyzer[]; loading: boolean };
export function useAssets(): Assets;
export function ScopePicker(props: { requireAnalyzer?: boolean }): JSX.Element;   // R196
// features/jobs
export function useJob(jobId: string | null): { job: JobView | null };            // polls GET /jobs/{id} every 2 s, stops on terminal or 5 min
export function RefreshActions(props: { analyzerId: string | null }): JSX.Element | null; // null without analyzers.refresh
// components/ui/stagger-grid.tsx
export function StaggerGrid(props: { className?: string; children: ReactNode }): JSX.Element; // sets --i per child (D26)
// map.tsx
export type MapProps = { …; focusId?: string };
```
- [ ] **Read first:** `design-system/bcem-energy/MASTER.md`, then `OVERRIDES.md`; invoke the `ui-ux-pro-max` skill once for
  focused queries (`--domain chart "energy dashboard load profile"`, `--domain ux "settings tabs dense forms"`) and note any
  rule that is not already in 07/OVERRIDES in the ledger (no persist, no `--force`).
- [ ] **Failing tests:**
  - Go `TestE2EFixturesHaveReadingsAndBill` (integration): run `E2EFixtures` twice at a fixed clock → A1/A2/B1 each have
    readings from `now−75d` to the current hour with no duplicate `(analyzer, ts, kind)`; `consumption_daily` has rows for A1
    yesterday; exactly one current building bill for A1 with `period_key` = previous month and `total_cost` `48250.75`.
  - Go `TestSeedDemoIdempotentAndExtends` (existing) stays green after the generator extraction.
  - `dates.test.ts`: `istanbulToday(new Date('2025-03-14T22:30:00Z'))` → `'2025-03-15'`; `addDays('2025-02-28', 1)` → `'2025-03-01'`.
  - `problem.test.ts`: `{error:{code:'validation_failed',details:{name:['required'],'contacts[0].phone':['max']}}}` →
    `{name:'Zorunlu alan', 'contacts[0].phone':'Çok uzun'}`; unknown code → `'Geçersiz değer'`; non-envelope → fallback.
  - `download.test.ts` (mockApi): `Content-Disposition: attachment; filename="tuketim.csv"` → anchor `download="tuketim.csv"`,
    `URL.revokeObjectURL` called; query `company_id` forwarded.
  - `mutation.test.tsx`: success → toast title from `success`; 422 → `fieldErrors` populated and a danger toast with the API message;
    invalidates queries whose key path starts with `/api/v1/buildings`.
  - `scope-picker.test.tsx`: store empty + buildings [`B Depo`, `A Fabrika`] → selects `A Fabrika`; building with one analyzer →
    analyzer auto-selected; stored building not in the list → replaced; admin with `companyId` → both requests carry `company_id`.
  - `use-job.test.tsx` (fake timers): queued → running → succeeded stops polling (3 calls); 404 → `failed`; after 5 min → message
    `domain.job.stillRunning`.
  - `refresh-actions.test.tsx`: renders nothing without `analyzers.refresh`; disabled with description when `analyzerId` null;
    click "Saatlik değerleri yenile" posts `{mode:'hourly'}` then shows `JobStatusBanner`; 409 `integration_not_configured` → danger toast with API message.
  - `stagger-grid.test.tsx`: children get `style="--i: 0|1|2"` and class `stagger-item`; globals.css rule is inside
    `@media (prefers-reduced-motion: no-preference)` (asserted by reading the CSS file text).
  - `map.test.tsx` addition: fallback list with `focusId="b2"` marks row `b2` `aria-current="true"`.
  - `middleware.test.ts` addition: forwarded `/ekorm/consumption?x=1` request carries `x-ekokod-path: /ekorm/consumption?x=1`;
    `layout.test.tsx`: no session + header → `redirect('/auth/login?next=%2Fekorm%2Fconsumption%3Fx%3D1')`.
- [ ] Red → implement → green gate: `./internal/seed/ ./internal/job/` integration; web `pnpm lint && pnpm typecheck &&
  pnpm check:i18n-parity && pnpm check:contrast && pnpm test --maxWorkers=2`.
- [ ] Commit `feat(f6b): task-4 — e2e readings and bill, scope picker, job polling, downloads, mutation helpers`.
- [ ] **Mutations:** `istanbulToday` using UTC date; `ScopePicker` not spreading scope params; `useJob` not stopping on `succeeded`;
  fixtures inserting a second bill on re-run.

## Task 5 — Dashboard: map panel, building list, latest bill, reactive card (R204)

**Files:** Create `web/src/features/dashboard/{dashboard-page.tsx,map-panel.tsx,building-list.tsx,latest-bill-card.tsx,
reactive-panel.tsx}` + stories/tests; modify `web/src/app/ekorm/page.tsx` (replace the health view; keep `HealthStatus`
component for the maintenance page only if still referenced, otherwise delete it and its `health` namespace usage in
`/ekorm`), `messages/{tr,en}/dashboard.json`.

**Interfaces (Views):**
```ts
export type DashboardBuilding = { id: string; name: string; address: string | null; lat: string | null; lng: string | null;
  analyzerCount: number; status: 'active' | 'passive' };
export type DashboardAnalyzer = { id: string; buildingId: string | null; label: string; installationNumber: string;
  address: string | null; lat: string | null; lng: string | null; status: 'active' | 'passive' };
export function MapPanelView(p: { buildings: DashboardBuilding[]; analyzers: DashboardAnalyzer[]; visibleBuildingIds: string[];
  focusId?: string; onSelectBuilding(id: string): void; onSelectAnalyzer(id: string): void; loading: boolean }): JSX.Element;
export function BuildingListView(p: { buildings: DashboardBuilding[]; analyzers: DashboardAnalyzer[]; selectedBuildingId?: string;
  selectedAnalyzerId?: string; checkedIds: string[]; onCheckedChange(ids: string[]): void; onSelectBuilding(id: string): void;
  onSelectAnalyzer(id: string): void; onFocus(id: string): void; loading: boolean }): JSX.Element;
export function LatestBillCardView(p: { bill: Bill | null; loading: boolean; locale: Locale }): JSX.Element;
export type ReactiveRow = ReactiveAnalyzer & { analyzerLabel: string; buildingName: string };
export function ReactivePanelView(p: { month: string; maxMonth: string; onMonthChange(m: string): void; buildingFilter: 'all' | string;
  buildings: { id: string; name: string }[]; onBuildingFilterChange(v: string): void; rows: ReactiveRow[];
  highestInductive?: ReactiveRow; highestCapacitive?: ReactiveRow; loading: boolean }): JSX.Element;
```
- [ ] **Failing tests:**
  - `map-panel.test.tsx`: header text `Aktif 2 · Pasif 1`; a building without coordinates is excluded from markers and the note
    `1 kaydın konumu yok` shows; unchecked building's marker hidden; "Analizörler" tab lists analyzer markers labelled by installation number.
  - `building-list.test.tsx`: search `ığdır` matches `IĞDIR Depo` (Turkish normalisation); "Yalnızca aktif" hides passive;
    "Tümünü kaldır" → `onCheckedChange([])`, label flips to "Tümünü seç"; expanding a building with two analyzers shows a radio group;
    "Haritada göster" calls `onFocus('b1')`; empty search → empty state "Aramanızla eşleşen bina yok".
  - `latest-bill-card.test.tsx`: bill period `2025-02` total `48250.75` active `18450.500` → `Şubat 2025`, `₺48.250,75`,
    `18.450,5 kWh`, badge `Kesinleşti`; `null` → empty state with link to `/ekorm/bills`.
  - `reactive-panel.test.tsx`: highest inductive `0.35` limit `0.20` → "Sınır aşıldı" + advisory `Kompanzasyon (güç faktörü düzeltmesi) gerekli`;
    no `penalty_applies` → `Sınırlar içinde`; exempt `below_kw` shows `Kurulu güç sınırın altında — muaf`; "Tüm binalar" dialog sorts by
    capacitive ratio descending on header click; bill link `href="/ekorm/bills?building_id=b1&period=2025-02"`.
  - `dashboard-page.test.tsx` (mockApi): BA user → buildings request without `company_id`; admin with switched company → every request
    carries `company_id=c-b`; latest bill without selection for CA uses `scope=company&subject_id=<company>`; selecting a building
    in the list updates `useSelection().buildingId` and refetches latest bill with `scope=building`.
  - axe clean for each View story test (`expectNoAxeViolations`).
- [ ] Implement with `Card`, `Tabs`, `Map`, `Checkbox`, `SearchInput`, `RadioGroup`, `IconButton`, `ReactiveStatusCard`,
  `MonthPicker`, `Select`, `Dialog`, `DataTable`, `StatusBadge`, `EmptyState`, `Skeleton`, `StaggerGrid`.
- [ ] Green gate: lint, typecheck, i18n parity, contrast, `pnpm test --maxWorkers=2`.
- [ ] Commit `feat(f6b): task-5 — dashboard map, building list, latest bill and reactive status`.
- [ ] **Mutations:** latest bill always `scope=company` (BA test red); reactive advisory from the first row instead of `penalty_applies` of any row.

## Task 6 — Dashboard: consumption panel, sectoral comparison, `dashboard.spec` (R204, R205)

**Files:** Create `web/src/features/dashboard/{consumption-panel.tsx,sector-comparison.tsx}` + stories/tests;
`web/tests/e2e/dashboard.spec.ts`; modify `dashboard-page.tsx`, `tests/e2e/fixtures.ts` (building/analyzer names,
bill figures), `messages/{tr,en}/dashboard.json`.

**Interfaces:**
```ts
export function ConsumptionPanelView(p: { rows: ConsumptionRow[]; previousYear: ConsumptionRow[] | null; granularity: Granularity;
  range: DateRange; today: string; onApply(g: Granularity, r: DateRange): void; loading: boolean; locale: Locale;
  refresh: ReactNode }): JSX.Element;
export function sectorCsvRows(c: BuildingComparison, buildingName: string, t: Translator): string[][];
export function SectorComparisonView(p: { buildingName: string | null; comparison: BuildingComparison | null;
  error: 'sector_missing' | null; canEditBuildings: boolean; loading: boolean; onExport(): void }): JSX.Element;
```
- [ ] **Failing tests:**
  - `consumption-panel.test.tsx`: 60 monthly-free daily rows → chart data length 50 and alert `Grafik ilk 50 noktayı gösteriyor (toplam 60)`;
    table caption total `60 kayıt`, first page 10 rows; toggling "Endüktif" off removes that series from the chart legend;
    monthly granularity in 2025 with `previousYear` rows → year-over-year chart with legend `2025` and `2024`; daily granularity → caption
    `Yıllık karşılaştırma aylık görünümde gösterilir`.
  - `sector-comparison.test.tsx`: fixture comparison (daily value `412.5` avg `380`, monthly `12375` avg `11400.25` rank 3/4, per capita
    rank 2/4, per area ranked 0) → cells `412,5`, `11.400,25`, rank lines `Aylık tüketim: 4 bina içinde 3.`, `Birim alan başına: sıralanamadı`;
    `available=false` → `Sektörde karşılaştırma için yeterli bina yok (en az 3)`; `sector_missing` with edit → link `?tab=buildings`.
  - `sector-csv.test.ts`: `toCsv(sectorCsvRows(...))` starts with BOM, uses `;`, first row headers with units, row 2 building name, row 3
    `Sektör ortalaması`, numbers `tr-TR` (`11.400,25`).
  - `dashboard.spec.ts` (e2e): (1) CA of company A: map header `Aktif 2 · Pasif 0`; list shows `A1 Fabrika` and `A2 Depo`, not `B1 Ofis`;
    latest bill card shows `₺48.250,75` after selecting `A1 Fabrika`; sectoral table shows `Sektör: Üretim · 4 bina` and a rank line;
    "Dışa aktar › CSV" downloads `A1 Fabrika-sektor-karsilastirma.csv` whose text contains `Sektör ortalaması`. (2) BA: list has only
    `A1 Fabrika`. (3) demo: dashboard shows `Demo Fabrika` and no E2E building names. (4) consumption panel table non-empty for A1.
- [ ] Green gate: unit suite; then foreground `free -m`; `pnpm build && PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1 tests/e2e/dashboard.spec.ts tests/e2e/auth.spec.ts`.
- [ ] Commit `feat(f6b): task-6 — dashboard consumption panel, sectoral comparison with CSV, dashboard e2e`.
- [ ] **Mutations:** CSV delimiter `,`; drop the 50-point cap; rank rendered from `ranked` instead of `rank`.

## Task 7 — Consumption screen and `consumption.spec` (01 §7.3, R193, R206)

**Files:** Create `web/src/app/ekorm/consumption/page.tsx`, `web/src/features/consumption/{consumption-page.tsx,summary-cards.tsx,
series-chart.tsx,consumption-table.tsx,columns.ts,alarm-check-dialog.tsx,detailed-graphs.tsx,format-period.ts}` + stories/tests,
`web/tests/e2e/consumption.spec.ts`; `messages/{tr,en}/consumption.json`.

**Interfaces:**
```ts
export const CONSUMPTION_COLUMNS: readonly { key: keyof ConsumptionRow | 'period'; labelKey: string; unit?: Unit | 'ratio' }[]; // R206 order
export function formatPeriod(periodStart: string, g: Granularity, locale: Locale): string;
export function SummaryCardsView(p: { summary: ConsumptionSummary | null; loading: boolean }): JSX.Element;
export type SeriesToggles = { active: boolean; inductive: boolean; capacitive: boolean };
export function SeriesChartView(p: { rows: ConsumptionRow[]; granularity: Granularity; locale: Locale; show: SeriesToggles;
  onShowChange(s: SeriesToggles): void; loading: boolean }): JSX.Element;   // LineChart, kinds consumption/reactiveInductive/reactiveCapacitive
export function ConsumptionTableView(p: { rows: ConsumptionRow[]; granularity: Granularity; locale: Locale; loading: boolean;
  canCheckAlarm: boolean; onCheckAlarm(row: ConsumptionRow): void; onExport(f: 'csv' | 'excel'): void; onPrint(): void;
  busyFormat: ExportFormat | null }): JSX.Element;
export function AlarmCheckDialogView(p: { open: boolean; onOpenChange(o: boolean): void; row: ConsumptionRow | null;
  result: AnomalyCheck | null; loading: boolean; locale: Locale; granularity: Granularity }): JSX.Element;
export type GroupBy = 'daily' | 'week' | 'day_type' | 'season' | 'season_day_type';
export function DetailedGraphsView(p: { data: ConsumptionGrouped | null; groupBy: GroupBy; onGroupByChange(g: GroupBy): void;
  show: { active: boolean; inductive: boolean; capacitive: boolean }; onShowChange(s: …): void; loading: boolean; locale: Locale }): JSX.Element;
```
- [ ] **Failing tests:**
  - `columns.test.ts`: `CONSUMPTION_COLUMNS.map(c => c.key)` equals the literal R206 list (29 entries); every `key` except `period`
    exists in a `ConsumptionRow` type instance built from `schema.d.ts` (type-level `satisfies`).
  - `format-period.test.ts`: `('2025-03-14T09:00:00+03:00','hourly','tr')` → `14.03.2025 09:00`; daily → `14.03.2025`; monthly tr →
    `Mart 2025`, en → `March 2025`; yearly → `2025`.
  - `consumption-table.test.tsx`: renders the 29 Turkish header labels (literal list incl. `Aktif endeks`, `U1 endeksi`, `Endüktif oran (%)`,
    `Maks. demant (kW)`); ratio `0.2512` → `%25,12`; `partial` row shows badge `Eksik veri`; `suspect_registers:['active_import']` →
    `Şüpheli` with reason containing `Aktif`; row action `Alarm kontrolü` absent when `canCheckAlarm=false`; "Yazdır" calls `onPrint`.
  - `alarm-check-dialog.test.tsx`: `{available:false, reason:'ml_service_unavailable'}` → `Tahmin modeli şu anda kullanılamıyor` and
    actual `1.234,5 kWh`; no verdict text rendered.
  - `summary-cards.test.tsx`: totals active `18450.5`, inductive `3321.09`, capacitive `553.5`, average active `615.02` → four
    `MetricCard`s with those formatted values; `suspect_rows: 2` → quality badge on each card.
  - `detailed-graphs.test.tsx`: `day_type` data → x labels `Hafta içi`, `Hafta sonu`; `season` keys `winter-2025` → `Kış 2025`;
    week key → `Hafta 10.03.2025`; previous period present → comparison chart legend `Seçili dönem`, `Önceki dönem`; statistics tiles
    Toplam/Ortalama/Tepe/Dip with key labels; `season_day_type` with all three registers → 6 series (no D21 alert).
  - `series-chart.test.tsx`: three series drawn with units `kWh`/`kVArh` in the legend; turning "Kapasitif" off removes it from the
    legend and the data table; empty rows → chart empty state `Seçilen dönem için veri yok`.
  - `consumption-page.test.tsx` (mockApi): the filter bar renders `ScopePicker`, `PeriodFilterBar` and `RefreshActions` (T4) for the
    selected analyzer; Apply fires `/consumption`, `/consumption/summary` with `analyzer_id`, `granularity=daily`,
    `from`,`to` = last 6 months; building selected without analyzer → `building_id`; detailed tab requests `/consumption/grouped`
    with `group_by=daily&compare=previous` only when the tab is opened; export Excel calls `downloadFile('/api/v1/consumption/export', {format:'xlsx', …})`.
  - `consumption.spec.ts` (e2e): CA selects `A1 Fabrika`, daily, last 7 days via preset → table visible with ≥ 1 row and all 29 headers;
    CSV export produces a download ending `.csv`; "Detaylı grafikler" with "Hafta içi / hafta sonu" shows the data table rows `Hafta içi`
    and `Hafta sonu`; BR opens `/ekorm/consumption` and sees `A2 Depo` only in the building picker; alarm check as CA shows the
    unavailable message.
- [ ] Green gate: unit suite; foreground build + `tests/e2e/consumption.spec.ts`.
- [ ] Commit `feat(f6b): task-7 — consumption screen with full columns, exports, alarm check and detailed graphs`.
- [ ] **Mutations:** drop one column from `CONSUMPTION_COLUMNS`; format ratios without `fractionToPercent`; fetch grouped eagerly on page load.

## Task 8 — Load profile screen and `load-profile.spec` (01 §7.4, R207)

**Files:** Create `web/src/app/ekorm/load-profile/page.tsx`, `web/src/features/load-profile/{load-profile-page.tsx,profile-label.ts,
daily-tab.tsx,seasonal-tab.tsx,compare-tab.tsx,details-tab.tsx}` + stories/tests, `web/tests/e2e/load-profile.spec.ts`;
`messages/{tr,en}/loadProfile.json`.

**Interfaces:**
```ts
export const PROFILE_KEYS = ['weekday','weekend','winter_weekday','winter_weekend','spring_weekday','spring_weekend',
  'summer_weekday','summer_weekend','autumn_weekday','autumn_weekend'] as const;
export type ProfileKey = (typeof PROFILE_KEYS)[number];
export function profileLabel(t: Translator, key: ProfileKey): string;
export function profileData(profiles: Record<string, (string | null)[]>, keys: ProfileKey[]): Datum[]; // x = '00:00'…'23:00'
export function DailyTabView(p: { data: LoadProfiles | null; loading: boolean }): JSX.Element;
export function SeasonalTabView(p: { data: LoadProfiles | null; loading: boolean }): JSX.Element;
export function CompareTabView(p: { data: LoadProfiles | null; selected: ProfileKey[]; onSelectedChange(k: ProfileKey[]): void; loading: boolean }): JSX.Element;
export function DetailsTabView(p: { stats: LoadProfileStatistics | null; loading: boolean; onExport(): void; busy: boolean }): JSX.Element;
```
- [ ] **Failing tests:**
  - `profile-label.test.ts` (**09 acceptance, 10 item 14**): tr labels exactly `Hafta içi`, `Hafta sonu`, `Kış – Hafta içi`, `Kış – Hafta sonu`,
    `İlkbahar – Hafta içi`, `İlkbahar – Hafta sonu`, `Yaz – Hafta içi`, `Yaz – Hafta sonu`, `Sonbahar – Hafta içi`, `Sonbahar – Hafta sonu`
    in `PROFILE_KEYS` order; en `Weekday`, `Weekend`, `Winter – Weekday`, … `Autumn – Weekend`.
  - `profile-data.test.ts`: 24 points, x `00:00`…`23:00`, nulls kept as `null`.
  - `seasonal-tab.test.tsx`: eight chart figures named by `profileLabel`, each followed by its note (`Bu grafik, yaz mevsimindeki hafta sonu …`
    under `Yaz – Hafta sonu`).
  - `compare-tab.test.tsx`: selecting a 7th profile is refused and the description `En fazla 6 profil` is visible; chart legend lists the selection.
  - `details-tab.test.tsx`: stats for weekday `{max:'42.5',min:'10.25',hour_of_max:14,mean:'25.125',stddev:'8.3',range:'32.25',load_factor:'0.5912'}`
    → row `Hafta içi | 42,5 | 10,25 | 14:00 | 25,125 | 8,3 | 32,25 | %59,12`; export button calls `onExport`.
  - `load-profile-page.test.tsx` (mockApi): building only → `Analizör seçin` and no request; Apply → one `/load-profile` and one
    `/load-profile/statistics` with `analyzer_id`, last-6-months dates; config caption `Hafta sonu günleri: Cuma, Cumartesi (şirket takvimi) · 1 tatil dönemi`
    for `weekend_days [5,6]`, `weekend_source company`, `vacations 1`.
  - `load-profile.spec.ts` (e2e): CA selects A1, Apply; "Mevsimsel" tab shows the headings of the seasons present in the last six months with
    correct labels (assert `Yaz – Hafta sonu` label text maps to the `summer_weekend` figure via `data-profile-key`); "Detaylar" table shows ten
    rows and Excel export downloads `.xlsx`; demo user sees `Demo Fabrika` analyzers and non-empty weekday chart table.
- [ ] Green gate: unit suite; foreground build + `tests/e2e/load-profile.spec.ts`.
- [ ] Commit `feat(f6b): task-8 — load profile screen with key-derived labels, compare and statistics`.
- [ ] **Mutations:** swap the summer weekday/weekend label order in `profileLabel` (label test red); allow a 7th compare profile.

## Task 9 — Settings shell, Account tab, SMTP and Integrations tabs (R200)

**Files:** Create `web/src/app/ekorm/settings/page.tsx`, `web/src/features/settings/{settings-page.tsx,tabs.ts,account-tab.tsx,
smtp-tab.tsx,integrations-tab.tsx}` + stories/tests; `messages/{tr,en}/settings.json`.

**Interfaces:**
```ts
export type SettingsTabId = 'account' | 'integrations' | 'company' | 'buildings' | 'plants' | 'analyzers' | 'users' | 'smtp';
export const SETTINGS_TABS: readonly { id: SettingsTabId; permission: Permission | null }[]; // R200 order
export function visibleTabs(can: (p: Permission) => boolean): SettingsTabId[];
export function resolveTab(requested: string | null, visible: SettingsTabId[]): SettingsTabId;
export function AccountTabView(p: { profile: Me; readOnly: boolean; sessions: Session[] | null; onSaveProfile(v: ProfileUpdateRequest): void;
  onChangePassword(v: ChangePasswordRequest): void; onRevokeSession(id: string): void; onLogoutAll(): void;
  fieldErrors: Record<string, string>; saving: 'profile' | 'password' | null }): JSX.Element;
export function SmtpTabView(p: { settings: SMTPSettings | null; onSave(v: SMTPPutRequest): void; onTest(to: string): void; … }): JSX.Element;
export function IntegrationsTabView(p: { definitions: IntegrationDefinition[]; onCreate(v): void; onUpdate(id, v): void; onDelete(id): void; … }): JSX.Element;
```
- [ ] **Failing tests:**
  - `tabs.test.ts` (**09 acceptance "sees exactly the tabs"**): with `permissions.fixture.json` — admin: all eight in order; company_admin and
    company_readonly_admin: `account, company, buildings, plants, analyzers, users`; building_admin and building_readonly_admin:
    `account, analyzers`; demo: `account`. `resolveTab('smtp', visible(company_admin))` → `account`; `resolveTab(null, …)` → `account`.
  - `account-tab.test.tsx`: name/e-mail/phone fields prefilled; save sends only changed fields; `readOnly` (demo) → inputs disabled,
    info alert, no password form, no sessions list; password form shows the R146 rule list (reuse `PasswordRules`) and maps
    `details.current_password=['invalid']` to `Mevcut şifre hatalı`; sessions table marks the current session `Bu cihaz` and hides its revoke
    action; "Tüm oturumları kapat" asks for confirmation.
  - `smtp-tab.test.tsx`: `has_password=true` → password field placeholder-free with description `Kayıtlı şifre korunur; değiştirmek için yazın`,
    submit without password omits the field; test-send dialog posts `{to}`; 422 `smtp_test_failed` → danger toast with API message.
  - `integrations-tab.test.tsx`: table lists provider, subtype, endpoint count; form endpoint rows are limited to the twelve R176 keys
    (Select) with https URL inputs; `http://` shows `Adres https:// ile başlamalı` before submit; delete 409 `definition_in_use` → toast message.
  - `settings-page.test.tsx`: `?tab=users` for BA → renders Account and `router.replace` to `?tab=account`; switching tab keeps `isolar=ok`
    absent and other params; `?isolar=ok` → success toast once and the param removed.
- [ ] Green gate: lint/typecheck/i18n/contrast/unit suite.
- [ ] Commit `feat(f6b): task-9 — settings shell with permission-gated tabs, account, SMTP and integration definitions`.
- [ ] **Mutations:** gate `buildings` on `write` instead of `settings.buildings` (CR loses the tab → tabs test red); show the password form to demo.

## Task 10 — Settings: Company tab, company list (admin), integration credentials (R201, R209)

**Files:** Create `web/src/features/settings/{company-tab.tsx,company-list.tsx,credentials-section.tsx,credential-form.tsx}` +
stories/tests; modify `settings-page.tsx`, `messages/{tr,en}/settings.json`.

**Interfaces:**
```ts
export function CompanyTabView(p: { company: CompanyDetail | null; canEdit: boolean; onSave(v: CompanyUpdateRequest): void; … }): JSX.Element; // analyzer_counts table
export function CompanyListView(p: { companies: Company[]; hasMore: boolean; onLoadMore(): void; activeCompanyId?: string;
  onActAs(id: string | undefined): void; onCreate(v: CompanyCreateRequest): void; onUpdate(id, v): void; onDelete(id): void; ownCompanyId: string; … }): JSX.Element;
export type CredentialDraft = { provider: Provider; subtype: string; username?: string; secret?: string; pm5340_url?: string;
  installation_number?: string; isolar_region?: string; extra?: { app_key?: string; secret_key?: string; app_id?: string };
  use_billing_indexes?: boolean; wiring_numbers?: string[]; building_id?: string | null };   // R210
export function toCredentialRequest(d: CredentialDraft): IntegrationCredentialCreateRequest;
export function CredentialFormView(p: { definitions: IntegrationDefinition[]; value: CredentialDraft; onChange(d: CredentialDraft): void;
  onSubmit(): void; fieldErrors: Record<string,string>; mode: 'create' | 'update' }): JSX.Element;
export function CredentialsSectionView(p: { credentials: IntegrationCredential[]; canCreate: boolean; canManage: boolean;
  onVerify(id): void; onDiscover(id): void; onBackfill(id, v: BackfillRequest): void; onAuthorizeIsolar(id): void; onDelete(id): void;
  jobs: Record<string, JobView | null>; … }): JSX.Element;
```
- [ ] **Failing tests:**
  - `credential-form.test.ts(x)`: `toCredentialRequest` for `pm5340` → `{provider:'pm5340', subtype, pm5340_url, installation_number,
    building_id?}` (no username/secret keys); `gridbox` with switch on and two wiring numbers → `{settings:{use_billing_indexes:true},
    wiring_numbers:['1001','1002'], building_id}`; `osos` → neither `wiring_numbers` nor `building_id` (R210); `isolar` →
    `extra {app_key, secret_key, app_id}` + `isolar_region`; the form shows only the fields of the chosen provider (wiring-number chip
    input and building `Select` for gridbox, building `Select` for pm5340); a duplicate or malformed wiring number shows an inline error
    before submit; update mode leaves an empty secret out of the request.
  - `credentials-section.test.tsx`: `has_secret` → `Kayıtlı` badge and never a secret value; "Analizörleri keşfet" posts discover and shows the
    `JobStatusBanner` via `jobs[id]`; success banner action links to `?tab=analyzers&unassigned=1`; iSolar row "iSolarCloud'a bağlan" calls
    `onAuthorizeIsolar`; `canManage=false` (CR never sees the section — assert the container hides it without `integrations.credentials`).
  - `company-tab.test.tsx`: analyzer counts rendered `OSOS / Baskent — 2`; `canEdit=false` → no form controls.
  - `company-list.test.tsx`: delete of own company disabled with tooltip `Kendi şirketinizi silemezsiniz`; "Bu şirket adına çalış" calls
    `onActAs('c-b')`; active company row marked; "Daha fazla yükle" visible only when `hasMore`.
  - container test: admin opening Company tab lists companies (`/companies?limit=50`), then `next_cursor` request; `onAuthorizeIsolar` →
    `window.location.assign` with the URL from `/integrations/isolar/authorize-url?credential_id=…`.
- [ ] Green gate: unit suite.
- [ ] Commit `feat(f6b): task-10 — company tab, admin company list and integration credentials`.
- [ ] **Mutations:** include an empty `secret: ''` in update requests; show credential creation to company admins.

## Task 11 — Settings: Buildings and Solar Power Plants tabs

**Files:** Create `web/src/features/settings/{buildings-tab.tsx,building-form.tsx,plants-tab.tsx,plant-form.tsx}` + stories/tests;
modify `settings-page.tsx`, messages.

**Interfaces:**
```ts
export type BuildingDraft = { name: string; address: string; latitude: string | null; longitude: string | null; floors: string;
  contacts: { name: string; phone: string }[]; personnel_count: string; total_area_m2: string | null; responsible_user_id: string | null;
  sector: string; bill_cutoff_day: string };
export function toBuildingRequest(d: BuildingDraft, original?: BuildingDetail): BuildingCreateRequest | BuildingUpdateRequest; // PATCH: changed fields only; clearing responsible → clear_responsible_user
export function BuildingsTabView(p: { buildings: Building[]; canEdit: boolean; onCreate(): void; onEdit(id): void; onDelete(id): void; loading: boolean }): JSX.Element;
export function BuildingFormView(p: { value: BuildingDraft; onChange(d): void; users: User[]; tariffHistory: TariffSummary[];
  onSubmit(): void; fieldErrors: Record<string,string>; submitting: boolean; mode: 'create' | 'edit' }): JSX.Element;
export type PlantDraft = { …PlantCreateRequest fields as strings…; monthly_targets: (string | null)[]; alarm_recipients: string[] };
export function toPlantRequest(d: PlantDraft, original?: PlantDetail): PlantCreateRequest | PlantUpdateRequest;
export function PlantsTabView(…): JSX.Element; export function PlantFormView(p: { …; devices: PlantDevice[] }): JSX.Element;
```
- [ ] **Failing tests:**
  - `building-form.test.ts(x)`: `toBuildingRequest` create: numbers stay strings for decimals (`total_area_m2:'5000.5'`), integers parsed
    (`floors: 3`), empty optional → omitted; edit clearing responsible → `{clear_responsible_user: true}` and no `responsible_user_id`;
    contacts add/remove rows (max 10) replace the list; `bill_cutoff_day` 0 or 32 → inline `1 ile 31 arasında olmalı` before submit; 422
    `details.responsible_user_id=['invalid']` → field error; tariff history rows `01.01.2025 — Sanayi OG` with link `Tarifeleri yönet` → `/ekorm/tariffs?building_id=…`
    and no tariff form (F8).
  - `buildings-tab.test.tsx`: `canEdit=false` → no "Bina ekle", no row actions; delete 409 `building_has_analyzers` → toast with API message.
  - `plant-form.test.ts(x)`: twelve month inputs labelled `Ocak`…`Aralık`; `toPlantRequest` sends `monthly_targets` as twelve decimal
    strings; leaving any month empty blocks submit with the inline error `12 ayın hedefi girilmeli` (never a substituted zero);
    orientation `Select` has the eight options `Kuzey`…`Güneybatı`; the alarm-recipient chip input rejects `abc` with `Geçerli bir e-posta girin`;
    the devices table is read-only under the note `Cihazlar iSolarCloud bağlantısıyla içe aktarılır`.
  - containers: responsible user options come from `/users?limit=500&is_active=true`; admin acting for company B sends `company_id` on
    POST `/buildings`.
- [ ] Green gate: unit suite.
- [ ] Commit `feat(f6b): task-11 — buildings and solar plant settings`.
- [ ] **Mutations:** PATCH sending unchanged fields (changed-fields test red); allow 11 monthly targets.

## Task 12 — Settings: Analyzers and Users tabs, `settings.spec`

**Files:** Create `web/src/features/settings/{analyzers-tab.tsx,users-tab.tsx,user-form.tsx}` + stories/tests,
`web/tests/e2e/settings.spec.ts`; modify `settings-page.tsx`, messages.

**Interfaces:**
```ts
export type AnalyzerFilters = { q: string; buildingId: string | 'all' | 'unassigned'; provider: Provider | 'all' };
export function AnalyzersTabView(p: { analyzers: Analyzer[]; buildings: Building[]; filters: AnalyzerFilters; onFiltersChange(f): void;
  canEdit: boolean; canRefresh: boolean; onAssign(id: string, buildingId: string | null): void;
  onUpdate(id: string, v: AnalyzerUpdateRequest): void; onRefresh(id: string, mode: 'hourly' | 'energy'): void;
  jobs: Record<string, JobView | null>; loading: boolean }): JSX.Element;
export function assignableRoles(me: Role): Role[]; // admin → 5 roles; company_admin → 4 (no admin)
export function UsersTabView(p: { users: User[]; me: Me; canEdit: boolean; onCreate(v: UserCreateRequest): void; onUpdate(id, v): void; onDelete(id): void; … }): JSX.Element;
```
- [ ] **Failing tests:**
  - `analyzers-tab.test.tsx`: columns `Tesisat no`, `Müşteri adı`, `Sayaç no`, `Sayaç modeli`, `Çarpan`, `İl / İlçe`, `Tarife tipi`, `Kurulu güç (kW)`,
    `Bina`, `Son veri`, `Durum`; `?unassigned=1` initial filter → request `unassigned=true`; BA (`canEdit=false`, `canRefresh=true`) sees refresh
    actions but no assign control; assign dialog `Select` of buildings + "Atamayı kaldır" → `{unassign_building:true}`; passive status badge text `Pasif`.
  - `user-form.test.tsx`: `assignableRoles('company_admin')` has no `admin`; role labels `Şirket Yöneticisi` etc.; create requires password and shows
    `PasswordRules`; 409 `email_taken` → field error on e-mail; own row: delete disabled and role/active controls disabled (`Kendi hesabınızı değiştiremezsiniz`).
  - `settings.spec.ts` (e2e): for each of the six fixture users, the settings tab list equals the `tabs.test.ts` literal lists; CA creates building
    `E2E Geçici Bina` (name, cutoff day 5), sees it listed, deletes it with confirmation; CR sees Buildings without `Bina ekle`; BA sees Analyzers with
    `Saatlik değerleri yenile` and gets the `integration_not_configured` toast; admin opens SMTP tab; the credentials section response bodies
    captured with `page.on('response')` contain none of the credential secret strings the spec typed (secret never echoed).
- [ ] Green gate: unit suite; foreground build + `tests/e2e/settings.spec.ts`.
- [ ] Commit `feat(f6b): task-12 — analyzers and users settings, settings e2e over six roles`.
- [ ] **Mutations:** `assignableRoles('company_admin')` including `admin`; BA seeing the assign control.

## Task 13 — Calendar screen and `calendar.spec` (01 §7.18, R202, R203)

**Files:** Create `web/src/app/ekorm/calendar/page.tsx`, `web/src/features/calendar/{calendar-page.tsx,range.ts,layout-lanes.ts,
month-view.tsx,time-grid-view.tsx,agenda-view.tsx,event-dialog.tsx,vacation-dialog.tsx}` + stories/tests,
`web/src/styles/event-palette.ts`, `web/tests/e2e/calendar.spec.ts`; `messages/{tr,en}/calendar.json`.

**Interfaces:**
```ts
export type CalendarView = 'month' | 'week' | 'day' | 'agenda';
export function visibleRange(view: CalendarView, anchor: string): { from: string; to: string }; // inclusive dates; month = Mon before 1st … 42 days; agenda = anchor … +30 d
export function shiftAnchor(view: CalendarView, anchor: string, dir: -1 | 1): string;
export type TimedEvent = { id: string; startMin: number; endMin: number };  // minutes within the day, Istanbul
export function layoutLanes(events: TimedEvent[]): Record<string, { lane: number; lanes: number }>;
export function toEventRequest(d: EventDraft): CalendarEventFields;      // all-day → 00:00+03:00 / next day 00:00+03:00
export function isNonWorkingDay(date: string, weekendDays: number[], periods: VacationPeriod[]): { weekend: boolean; vacation?: VacationPeriod };
export function MonthView(p: { anchor: string; events: CalendarEvent[]; weekendDays: number[]; periods: VacationPeriod[]; canEdit: boolean;
  onSelectDate(d: string): void; onSelectEvent(e: CalendarEvent): void; locale: Locale }): JSX.Element;
export function TimeGridView(p: { days: string[]; events: CalendarEvent[]; …same callbacks }): JSX.Element;
export function AgendaView(p: { from: string; to: string; events: CalendarEvent[]; onSelectEvent(e): void; locale: Locale }): JSX.Element;
export function EventDialogView(p: { open: boolean; value: EventDraft; onChange(d): void; onSubmit(): void; onDelete?(): void; readOnly: boolean; … }): JSX.Element;
export function VacationDialogView(p: { open: boolean; weekendDays: number[]; periods: VacationPeriod[]; onSave(v: VacationsPutRequest): void; readOnly: boolean; … }): JSX.Element;
```
- [ ] **Failing tests:**
  - `range.test.ts`: `visibleRange('month','2025-03-14')` → `{from:'2025-02-24', to:'2025-04-06'}`; week → `2025-03-10…2025-03-16`; day → same day;
    `shiftAnchor('month','2025-01-31',1)` → `'2025-02-28'`.
  - `layout-lanes.test.ts`: [09:00–10:00, 09:30–11:00, 10:00–10:30] → lanes 0,1,0 with `lanes` 2 for all three; disjoint events → lanes 1.
  - `event-request.test.ts`: all-day 2025-03-14..2025-03-15 → `starts_at '2025-03-14T00:00:00+03:00'`, `ends_at '2025-03-16T00:00:00+03:00'`;
    timed end ≤ start → inline `Bitiş başlangıçtan sonra olmalı`.
  - `month-view.test.tsx`: weekend days `[0,6]` → Saturday/Sunday cells carry text `Hafta sonu` (visually hidden) and shading class; vacation period
    shows its description; four events on one day → 3 chips + `+1 daha` popover; `canEdit=false` → clicking a date does not open the create dialog.
  - `vacation-dialog.test.tsx`: day checkboxes Pazartesi…Pazar map to API days (Pazar = 0); selecting all seven shows warning
    `Tüm günler tatil olarak işaretlendi`; period end before start → inline error; save sends `{weekend_days:[5,6], periods:[…]}`.
  - `calendar-page.test.tsx` (mockApi): view switch to week requests `from=2025-03-10&to=2025-03-16` (inclusive → API `to` per contract);
    BR has no "Etkinlik ekle"; created event → invalidates events query.
  - `calendar.spec.ts` (e2e): CA creates an all-day event `E2E Bakım Günü` today (colour Camgöbeği) → chip visible in month view; opens
    "Tatil yönetimi", sets weekend days to Cuma + Cumartesi, saves; `/ekorm/load-profile` for A1 shows `Hafta sonu günleri: Cuma, Cumartesi (şirket takvimi)`;
    restores Cumartesi + Pazar (cleanup); BR sees the event and no add button.
- [ ] Green gate: unit suite; foreground build + `tests/e2e/calendar.spec.ts`.
- [ ] Commit `feat(f6b): task-13 — calendar with month/week/day/agenda, events and vacation configuration`.
- [ ] **Mutations:** Sunday-first month grid (range test red); `layoutLanes` ignoring overlaps; all-day end not exclusive.

## Task 14 — Phase end: review, acceptance, a11y, handoff

- [ ] **Whole-phase self-review — authorisation/tenancy first:** `git diff 09e07bd..HEAD -- internal/` — `/jobs/{id}` hides every
  foreign/unknown case identically (404, no timing-relevant branch leaking type); grouped endpoint resolves the subject through scoped
  services; matrix + sweep rows exist for both routes; `git grep -n "company_id" web/src/features` shows only `useScopeParams`; every
  mutation control is behind `can(...)` (`git grep -n "useApiMutation" web/src/features` and read each call site); no secret or `last_err`
  rendered; credentials section never displays `extra`/secret values. Then **accessibility and design rules:** 07 §11 checklist item by item
  with evidence (grep for raw colours = lint; `pnpm check:contrast`; keyboard/a11y sweep below; 375/768/1024/1440 screenshots of the five
  screens via the e2e harness saved to the ledger, checking no page-level horizontal scroll with
  `document.documentElement.scrollWidth <= innerWidth`; empty/error states read once per screen; `DataQualityBadge` on partial/suspect rows).
  Fix findings with tests.
- [ ] **Acceptance evidence** (real output in the ledger `.superpowers/sdd/2026-09-17-f6b/inline-ledger.md`):
  - 09 §F6 verification: `go test ./internal/api/... -run TestAuthorizationMatrix -v`; `-tags=integration -run TestCredentialsNeverLeak -v`;
    `pnpm exec playwright test tests/e2e/auth.spec.ts tests/e2e/dashboard.spec.ts` and `consumption`/`load-profile` specs; `pnpm run check:i18n-parity`.
  - Full e2e suite: `pnpm build && PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1` (all six specs).
  - Go race runs package by package for every touched package (`auth`, `api/v1` + subpackages, `apiwire`, `job`, `service/jobs`,
    `service/analysis`, `service/loadprofile`, `service/assets`, `credentials`, `domain/grouping`, `seed`, `scheduler`, `worker`); `internal/arch` once;
    `golangci-lint run --concurrency 2 --build-tags=integration` on touched trees; `make check-generate`; `make openapi` clean.
  - Web: `pnpm lint && pnpm typecheck && pnpm check:api && pnpm check:i18n-parity && pnpm check:contrast && pnpm test --maxWorkers=2`;
    `pnpm storybook:build`; `STORIES='Features/Dashboard,Features/Consumption' pnpm test:a11y --workers=1`, then
    `STORIES='Features/LoadProfile,Features/Calendar'`, then `STORIES='Features/Settings'`, then `STORIES='Shell'` (foreground chunks);
    `pnpm build`; `pnpm audit --audit-level=high`.
- [ ] Sweep leftovers (`pgrep -af 'vitest|next-server|storybook|ekokod-e2e'`), stop only this session's processes.
- [ ] Rewrite `HANDOFF_NEXT_SESSION.md` for **F7** (state, what F6b built, deviations, rulings R190–R209, F7 must pick up — alarms/messages
  screens and `/job-runs` may reuse `/jobs/{id}` conventions; carry the F6a "Later phases"/deployment notes; PO questions incl. week-label
  divergence R193 and admin-only credential creation R201; environment; paste-ready Turkish prompt); commit; give the prompt; stop.

---

## Acceptance map (09 §F6, F6b part)

| 09 acceptance item | Delivered by |
|---|---|
| Six roles see exactly the navigation **and tabs** | F6a auth.spec (nav); T9 `tabs.test.ts` + T12 `settings.spec` (tabs) |
| Dashboard renders with real seeded data; markers active/passive | T4 (R194 data), T5 map header/markers, T6 `dashboard.spec` |
| Sectoral comparison visible, populated, ranked, exports CSV | T6 `sector-comparison.test`, `sector-csv.test`, `dashboard.spec` |
| Consumption table shows every §7.3 column | T7 `columns.test`, `consumption-table.test`, `consumption.spec` (API: F6a `TestConsumptionColumnSet`) |
| All eight seasonal labels correct, asserted in a test | T8 `profile-label.test` (+ `load-profile.spec`) |
| Calendar/vacation change `/load-profile` weekday/weekend (R137) | F6a `TestWeekendAndVacationChangeLoadProfileHTTP`; T13 `calendar.spec` end to end through the UI |
| Consumption "Detailed graphs" (§7.3) | T3 API, T7 UI |
| Refresh actions with progress and success/error feedback (§7.2, §7.3) | T2 API, T4 `RefreshActions`, T5–T7, T12 |
| Settings tabs with visibility matrix and CRUD (§7.15) | T9–T12 |

## Handoff "F6b must pick up" → tasks

| Item | Task |
|---|---|
| Screens Dashboard / Consumption / Load Profile / Settings (8 tabs, permission-gated) / Calendar | T5–T13 |
| `BuildingAnalyzerPicker` wired to `useSelection` | T4 `ScopePicker` |
| Card-grid stagger `--duration-stagger-step` (D26) | T4 `StaggerGrid`, used T5/T8 |
| Map tile URL (Q5) or coordinate list | T5 (Map with fallback; `focusId`) |
| Pages on PageHeader / FilterBar / PeriodFilterBar / MetricCard / charts / DataTable + ExportMenu / DataQualityBadge | T5–T13 |
| New namespaces like `auth`/`shell`; MASTER then OVERRIDES | T4 (namespaces, design read), every UI task |
| Scope params in every query | Global constraint; T4 `mockApi` tests assert `company_id` |
| e2e specs dashboard / consumption / load-profile / settings / calendar; seed data in `seed e2e` | T6, T7, T8, T12, T13; T4 (R194) |
| APIs the screens want (templates, bulk tariff, bills dashboard, plant realtime, reports, alarms) | Decided: F8 / F9 / F7 (user 2026-09-17); grouped, jobs and GridBox wiring numbers built (T2, T3, T3b) |
| Open: settings 404, e2e only auth, switcher 500 companies, layout redirect without `next` | T9 (settings route), T6–T13 (specs), R209/T10, R208/T4 |
| Deployment note (trusted proxies) and later-phase notes | Carried into the F7 handoff (T14) |

## Open questions for the product owner (defaults ship)

- **Q-B1** Detailed-graph weeks are Monday-start calendar weeks labelled by their Monday; legacy used week-of-month (`01-04-25`). OK? (R193)
- **Q-B2** Integration credential **creation** stays admin-only as in legacy; company admins manage existing ones. Should company admins
  also set up new integrations? (R201)
- **Q-B3** The dashboard year-over-year chart appears only for monthly granularity inside one calendar year. Enough? (R204)
- Carried: F6a Q-A1…Q-A5, F5 Q1–Q7, F4 real-invoice comparison and billing parameters, F1–F3 items.
