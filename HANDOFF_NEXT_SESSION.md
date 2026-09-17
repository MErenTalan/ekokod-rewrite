# Handoff — ekokod rewrite: F1–F6 COMPLETE → next phase F7 (alarms and messages)

> **STATUS (2026-09-18):** **F6b (the screens) is finished** on `phase/f6b-screens` in
> `/home/personal/ekokod-f6b-phase`, branched from `phase/f6-core-screens` (F6a). Dashboard, Consumption, Load Profile,
> Settings (8 tabs) and Calendar are built, together with the Go additions the screens needed (`GET /jobs/{id}`,
> `GET /consumption/grouped`, GridBox wiring numbers, `company_id` in OpenAPI, two permissions, e2e seed data). All 14
> plan tasks are committed with their own mutation proofs, followed by one whole-phase self-review
> (authorisation/tenancy first, then 07 §11 accessibility and design) and a full acceptance run.
> **F6 as a whole (F6a + F6b) is now done, so F7 is next.**
> **Working mode:** no parallel agents, no subagents, no Workflow. One session runs one phase inline, writes this handoff
> at the end, and the user runs `/clear`.
> **Nothing has been pushed and `main` is untouched. Pushing and merging to `main` are the user's call** — including
> whether to merge `phase/f6b-screens` into `phase/f6-core-screens` first.

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/` içinde, faz planı
> `docs/rewrite/09-implementation-plan.md` (16 faz, F0–F15).
>
> **Durum:** F1–F5 ve F6 (F6a auth/API/iskelet + F6b ekranlar) tamam. F6b `phase/f6b-screens` dalında
> (`/home/personal/ekokod-f6b-phase`) bitti. **Sıradaki faz F7 (Alarmlar ve Mesajlar) ve henüz planı yok.** F4'ün
> gerçek-fatura karşılaştırması ürün sahibinden fatura gelene kadar AÇIK. Hiçbir şey push edilmedi, `main`'e
> dokunulmadı; push ve merge kararı bana ait.
>
> **Çalışma düzeni (kesin):** paralel agent YOK, subagent YOK, Workflow YOK. Fazı bu oturumda tek başına yürüt
> (`superpowers:executing-plans` + TDD). Review'ları kendin yap: her task sonunda diff'i oku, kendi mutasyonunla
> guard'ın kırmızıya düştüğünü kanıtla; faz sonunda bir kez bütün-faz self-review (yetki/tenancy önce, sonra
> erişilebilirlik ve tasarım kuralları). Faz bitince `HANDOFF_NEXT_SESSION.md`'i F8 için yeniden yaz, bana yapıştırılacak
> promptu ver ve dur.
>
> Şu sırayla ilerle:
> 1. `/home/personal/ekokod-f6b-phase/HANDOFF_NEXT_SESSION.md`'i baştan sona oku (START HERE, "What F6b built",
>    Deviations, "F7 must pick up", Open questions, Environment). F6b planını
>    (`docs/superpowers/plans/2026-09-17-f6b-core-screens.md`, R190–R210) ve F6a planını (R136–R189 + izin matrisi)
>    referans olarak kullan.
> 2. F7 worktree'sini aç: `git worktree add -b phase/f7-alarms /home/personal/ekokod-f7-phase phase/f6b-screens`,
>    ardından `web/` içinde `pnpm install --frozen-lockfile`.
> 3. **F7 planını yaz** (`superpowers:writing-plans`, `docs/superpowers/plans/` altına; sıkı: imzalar, kural tablosu,
>    isimlendirilmiş testler, doğrulama komutları). Kaynaklar: `09-implementation-plan.md` §F7,
>    `01-project-context.md` §7.12 (dört alarm türü ve ayarları), §7.13 (Mesajlar), §7.14/§7.16 (iş geçmişi),
>    `05-api-contract.md`, `07-design-system.md`, `design-system/bcem-energy/MASTER.md` ve sonra `OVERRIDES.md`.
>    Planlamadan önce legacy alarm/mesaj kodunu ve gerçek veriyi oku. **SMS kararını bana sor** (gerçek bir gateway'e
>    karşı mı yazılacak, yoksa arayüzden tamamen mi kaldırılacak — yarım bırakılmayacak, 09 §F7 böyle diyor); ayrıca
>    alarm/mesaj ekranlarının ihtiyaç duyduğu ama mevcut API'de olmayan uçları açıkça listele ve F7'de mi yoksa sonraki
>    fazda mı yapılacağını bana sor. Planı bir kez spesifikasyona + legacy'ye karşı kendin gözden geçir, sonra uygula.
>
> **Ortam ve güvenlik:** her komutun başına
> `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB` ekle.
> Sadece üzerinde çalıştığın paketleri/test hedeflerini koştur; **asla `make test` veya `go test ./...` çalıştırma.**
> Entegrasyon testleri için önce `docker ps` ile `ekokod-test-pg` ve `ekokod-test-redis` var mı bak (yoksa
> `make test-db-up` / `make test-redis-up`; Docker kapalıysa bana söyle), sonra
> `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <paket> -tags=integration -count=1 -parallel 4`.
> Web'de `pnpm test --maxWorkers=2`; Storybook/Playwright/`next build` aynı anda değil, teker teker. Ağır işlerden önce
> `free -m` ve `ps -eo rss,comm | grep java` kontrol et (bellek sıkışıksa Playwright `--workers=1`), e2e için
> `pnpm build && pnpm test:e2e --workers=1`. Başka projenin Gradle/java süreçlerine, 5432'deki
> `dolmusum-dev-postgres-1` container'ına ve 8080'deki dolmusum serve'e dokunma.

---

## START HERE — exact state

| Branch / worktree | Head | State |
|---|---|---|
| `phase/f1-data-model` (main checkout `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite`) | `10d6577` | F1 complete |
| `phase/f2-integration-layer` (`/home/personal/ekokod-f2-phase`) | `9919bd3` | F2 complete |
| `phase/f3-consumption-engine` (`/home/personal/ekokod-f3-phase`) | `58597ec` | F3 complete |
| `phase/f4-billing-engine` (`/home/personal/ekokod-f4-phase`) | `cf79346` | F4 complete; real-invoice comparison OPEN |
| `phase/f5-design-system` (`/home/personal/ekokod-f5-phase`) | `a162d7a` | F5 complete |
| `phase/f6-core-screens` (`/home/personal/ekokod-f6-phase`) | `09e07bd` | F6a complete |
| **`phase/f6b-screens`** (`/home/personal/ekokod-f6b-phase`) | this handoff commit | **F6b complete** (plan `1529c97`; tasks 1–14 + phase review) |

Shared test containers (both tmpfs, recreate freely): `ekokod-test-pg` (TimescaleDB, :55432, `make test-db-up`) and
`ekokod-test-redis` (:56379, `make test-redis-up`). The e2e run creates and drops the `ekokod_e2e` database in
`ekokod-test-pg` and uses Redis DBs 6/7.

- Plan: `docs/superpowers/plans/2026-09-17-f6b-core-screens.md` (rulings **R190–R210**, file map, tasks 1–14,
  acceptance map, PO questions Q-B1…Q-B3).
- Ledger (git-ignored, in the primary checkout):
  `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite/.superpowers/sdd/2026-09-17-f6b/inline-ledger.md` — every task's
  mutations, deviations and gates, plus the phase-end review findings.
- Working tree clean, 30 commits on top of F6a. Everything at the tip is green: see **Verification at the F6b tip**.

---

## What F6b built

### Go
- **`GET /jobs/{id}`** (R192, `internal/service/jobs`, `handlers_jobs.go`): the state of a job the user started, read
  from asynq's Inspector across the `critical`/`default`/`low` queues. Roles A, CA, BA. One indistinguishable 404 for
  unknown, unwatchable, foreign-company, out-of-scope-analyzer, and company-wide jobs seen by a building-scoped role.
  Status: pending/scheduled/aggregating → `queued`, active/retry → `running`, completed → `succeeded`, archived →
  `failed`; the response never carries `last_err`. `refresh_analyzer`, `sync_analyzers` and `backfill` gained
  `asynq.Retention(time.Hour)` so a finished task stays queryable.
- **`GET /consumption/grouped`** (R193, `internal/domain/grouping`, `internal/service/analysis/grouped.go`): daily rows
  bucketed by `daily | week | day_type | season | season_day_type`, optional `compare=previous`, range ≤ 366 days, with
  statistics (total, average 3 dp half-even, peak, valley). Day types come from the company calendar through
  `loadprofile.CalendarConfig` (new exported method), so Consumption and Load Profile can never disagree (R137). Weeks
  start Monday and are labelled by that Monday — a **deliberate divergence from legacy** (Q-B1).
- **GridBox wiring numbers** (R210, `internal/credentials`): `wiring_numbers` (1–50, `^[A-Za-z0-9._-]{1,100}$`) and
  `building_id` on credential create/update; each number is upserted as an analyzer, and numbers left out of a later
  call are never deleted. PM5340 now honours `building_id` too.
- **`company_id` in OpenAPI** (R190) for every non-public route, so the typed client can send the admin scope without
  casts; array enums are moved onto `items`, with a guard test.
- **Permissions** (R191): `settings.analyzers.edit` (A, CA) and `anomaly.check` (A, CA, BA), with the web fixture
  regenerated from `auth.PermissionsFor`.
- **Seed** (R194): `ekokod seed e2e` fills 75 days of hourly readings for A1/A2/B1 and one issued building bill, so every
  screen has real data in e2e. The demo company keeps its F6a behaviour; `seed.fillHourly` is shared by both.
- **Test infrastructure** (`b34faba`): continuous-aggregate refresh policies are dropped from the test template and from
  every clone. They fired inside short-lived clones and materialised a monthly bucket for one analyzer while another
  stayed R94-composed — a deterministic failure on the F6a tip too.

### Web (all under `web/src`)
- **Shared plumbing:** `lib/api/{types,problem,mutation,download}.ts` (R198, R199), `lib/dates.ts`,
  `lib/format-period.ts`, `test/api-mock.ts`, `features/scope/{use-assets,scope-picker}` (R196),
  `features/jobs/{use-job,refresh-actions}` (R192), `components/ui/stagger-grid.tsx` (D26), `Map` `focusId` (R204),
  `middleware.ts` `x-ekokod-path` + a layout redirect that keeps `next` (R208).
- **Dashboard** (R204, R205): map panel with building/analyzer tabs and marker selection, building list with search and
  marker filtering, latest-bill card, reactive status with the all-buildings dialog, consumption panel with the 50-point
  cap and year-over-year bars, sectoral comparison with ranks and CSV.
- **Consumption** (R206): the full §7.3 column set (29 columns), summary cards, series chart, detailed graphs over
  `/consumption/grouped`, an alarm-check dialog that stays honest about the missing model (R165), CSV/Excel and print.
- **Load Profile** (R207): labels derived from the profile key, daily/seasonal/compare (max 6, D21)/details tabs, Excel
  export, and the weekend caption that links to the calendar.
- **Settings** (R200, R201, R209, R210): 8 permission-gated tabs — account (demo read-only), integrations, company
  (+ admin company list and integration credentials), buildings, plants, analyzers, users, SMTP.
- **Calendar** (R202, R203): hand-built month/week/day/agenda views, an event dialog with the six data colours, and the
  vacation dialog that owns weekend days and periods.
- **e2e:** `tests/e2e/{dashboard,consumption,load-profile,settings,calendar,responsive}.spec.ts` on top of F6a's
  `auth.spec.ts` — 39 tests.

### Phase-end review findings, all fixed with guards
1. **Tenancy (`f93583c`):** `jobs.Get` ignored `AnalyzerIDs` (backfill) and jobs that name no analyzer (credential sync),
   so a building-scoped role could watch a job outside its buildings.
2. **Page-level horizontal scroll (`0cc757b`):** `overflow-auto` clipped the 29-column table visually but left its width
   in the document's scroll area (the page scrolled 2428 px into emptiness) → `contain-paint` on `TableContainer`. The
   consumption average rendered at the division scale (`1.635,49478947368421052632`) → energy scale (R161) plus
   wrapping. `StaggerGrid` items could not shrink below their content. The calendar's seven day columns now scroll in
   place. Guard: `tests/e2e/responsive.spec.ts` (4 widths × 5 screens).
3. **Accessibility (`04ee79e`):** dark-theme muted text on a selected row was 4.37:1 → `primary-subtle` darkened and the
   contrast gate extended to secondary text on every subtle surface (75 pairs). Calendar chips and short time-grid
   blocks now keep 44 px (D13). The three controlled dialog stories follow the house pattern (the story owns the opener).
4. **Errors were invisible (`5c026a1`):** no screen surfaced a failed GET, so a failed request looked exactly like an
   empty screen → `QueryErrorToaster` says it once per query. Reading that code showed the retry policy tested an HTTP
   status the thrown error envelope never carries, so every 4xx was retried twice; both now read the envelope's `code`.

---

## Deviations from the F6b plan (all in the ledger)
- An unknown `group_by` answers **422 `validation_failed`** (the DTO enum, R154), not the plan's 400.
- `wiring_numbers` uses `validate:"dive,min=1"`: `dive,required` trips `TestRequestTypesAreConsistent`, which reads
  `required` anywhere in the tag. `credentials.Deps` gained `Buildings` (apiwire, worker and four test call sites).
- R194's seed data lives in `seed.E2EData`, called only by `ekokod seed e2e`; putting it in `E2EFixtures` broke four Go
  tests written against empty fixtures.
- Message keys are camelCase throughout (`check:i18n-parity` rejects anything else), so `months.1` and
  `roles.company_admin` became camelCase keys or reuse `shell.roles`.
- Story/container split: `*-page` and `*-panel` containers are exempt from the story-coverage rule and carry container
  tests instead.
- One unreproduced Go failure is recorded honestly: a single combined `credentials + api/v1 + worker` run reported FAIL
  for `api/v1` with the test name lost to an over-filtered grep; three identical re-runs and every later run, including
  the phase-end race and integration runs, were green.
- Two a11y harness rules were relaxed **with the reason written down**: axe measures the settled state (a fade in flight
  reads as low contrast), and the keyboard walk knows Chromium keeps a `type=time` input active while Tab crosses its
  inner fields.

---

## F7 must pick up
- **Branching:** start from `phase/f6b-screens` (see the prompt).
- **Alarms (01 §7.12):** all four types with their full settings; `alarm.evaluate` and `alarm.notify` jobs; notification
  frequency suppression; per-invoice deduplication for the invoice alarm; rule CRUD, event history and dry-run
  evaluation. F1 already has the alarm tables and F6a the `alarms.*` permissions — read both before designing anything.
- **SMS:** decide with the user, then build it against a real gateway or remove it from the UI. 09 §F7 forbids offering
  it unimplemented.
- **Mail:** delivery through the company's SMTP settings (`internal/mail`; the SMTP settings UI ships in F6b, R172),
  templates in both locales; a failed send is recorded on the alarm event and surfaced in Messages without failing the
  job.
- **Messages screen (§7.13)** over `operational_messages`, with search and both filters.
- **`job_runs` history for admins, with manual triggering** (§7.14/§7.16). `GET /jobs/{id}` (R192) already models "a job
  the user started": reuse its watchable allow-list, its scope checks and the web `useJob` hook instead of inventing a
  second convention; `RefreshActions` is the UI precedent.
- **Screen conventions are set** (R195–R199): route → `*-page` container → views; `ScopePicker` for selection;
  `useApiMutation` for every write; `downloadFile` for every export; `mockApi` for container tests; stories stay
  data-free; and every new screen needs a line in `tests/e2e/responsive.spec.ts`'s `SCREENS` list.
- **Never invent a verdict.** R165's placeholder ("Tahmin modeli şu anda kullanılamıyor") is the house style for
  anything the ML service cannot answer yet.
- **Deployment note (F15):** the API derives client IPs from `X-Forwarded-For` only for `EKOKOD_TRUSTED_PROXIES`; the web
  container and the front proxy must both be listed there, and the front proxy must append (not pass through) XFF.
- **Later phases:** F8 bills/tariffs/reports UI (tariff templates, bulk tariff, `GET /bills/dashboard` — deferred there
  by the user on 2026-09-17); F9 solar plant realtime/production and the iSolar screens; F12 public site; F13 the real
  `/anomaly/check` model and calendar events as an ML covariate (R137); F15 CSP nonces and the supported-browser floor.

---

## Open questions for the product owner (defaults ship)
- **F6b:** **Q-B1** detailed-graph weeks are Monday-start calendar weeks labelled by their Monday; legacy used
  week-of-month (`01-04-25`) — OK? · **Q-B2** integration credential **creation** stays admin-only as in legacy, while
  company admins manage existing ones — should CAs set up new integrations too? · **Q-B3** the dashboard year-over-year
  chart appears only for monthly granularity inside one calendar year — enough? · **Q-B4 (new)** R94's accepted
  divergence: a materialised monthly bucket is last−first inside the month (743 h) while a composed one is
  closing-to-closing (744 h); test databases no longer materialise, but production will — confirm the documented rule.
- **F6a:** **Q-A1** two-step verification and self-registration are not built (R151) · **Q-A2** force a password change
  at an admin-created user's first login? · **Q-A3** password-reset mail uses the company SMTP settings; no platform SMTP
  exists · **Q-A4** sectoral comparison is cross-tenant: peers hidden, ≥ 3 peers required (R162), grid factor
  0.469 kg/kWh · **Q-A5** mobile sessions last 30 days and are device-bound (R141/R142) · **Q6 (F5)** Calendar sits
  top-level after Reports (R167).
- **F5:** Q1 customiser theme colour · Q2 RTL · Q3 English UI keeps `1.234,56` · Q4 light `primary` = emerald-700 ·
  Q5 production map tile source · Q7 legacy catalogues disagree on 115 keys. Browser floor for `light-dark()`:
  Chrome/Edge ≥ 123, Firefox ≥ 120, Safari ≥ 17.5. **Note:** dark `primary-subtle` is now `#053F30` (was `#064E3B`) so
  secondary text on a selected row reaches 4.5:1.
- **F4 (unchanged):** real-invoice comparison OPEN; dated `billing_parameters` defaults (R106, reactive basis,
  exemptions, R118, tiering, PTF tolerance, rounding R112); R119, R111, R110, R114, R122, R135; OG transformer loss.
- **F1–F3 (unchanged):** Q1 ratio zero-case, Q3 weekend default, Q5–Q10; F2 device/integration items. Q4 (calendar
  events vs the weekend split) is **decided: no** (R137).

## Binding rulings
F1 rulings 1–14, F2 R1–R53, F3 R54–R104, F4 R105–R135, F5 D1–D26, F6a R136–R189, **F6b R190–R210**
(`docs/superpowers/plans/2026-09-17-f6b-core-screens.md`), plus each phase's deviations.

## Process rules that paid off (keep them, inline)
- **Read legacy code and real data before writing rules,** and list the APIs a screen needs but the backend lacks so the
  user can decide. F6b's five gaps were settled that way in one round (grouped consumption and job status in F6b,
  tariffs → F8, iSolar → F9, GridBox wiring numbers in F6b).
- **Prove every guard red with your own mutation, restoring from a file backup — never `git checkout` a file whose fix
  is not committed yet** (it silently reverted a finished fix once this phase). Survivors are findings: this phase they
  exposed a reactive advisory read from `rows[0]`, an unasserted chart cap, a test that sliced by the very constant it
  guarded, and a 403 test that only passed because of its own timeout.
- **Per-task gates run the whole touched package,** and the whole web suite before every commit (the story-coverage
  guard was committed red twice before that rule).
- **Run the real artefact.** Four of the six phase-end findings only appear in a built app at a real viewport
  (`pnpm build` + Playwright at 375/768/1024/1440); none is visible to jsdom tests.
- **When a UI test fails, ask whether it is the test or the product.** Three e2e failures this phase were my own
  matchers (`Cuma` also matches `Cumartesi`, a strict-mode row match, a click before the grid had rendered).
- **Commit as soon as tests are green,** then the mutations, then the next task. Comments say WHY in 1–3 lines.

## Environment — read before running anything
- Prefix every command with `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
- WSL: 12 GB RAM, 4 GB swap. Another project's Gradle/Kotlin daemons (dolmusum) come and go (up to ~6 GB) — never kill
  them, and never touch `dolmusum-dev-postgres-1` on 5432 or the dolmusum serve on 8080. Check `free -m` before
  Storybook, Playwright or `next build`; run long Playwright work in the foreground with `--workers=1`.
- **Never `pkill -f <pattern>` from a Bash call whose own command line contains the pattern** — it kills the calling
  shell (exit 144; harmless but noisy, seen repeatedly this phase). Use `pgrep -x` + `kill`.
- Go integration: shared Postgres + Redis as in the prompt. Test databases have **no** continuous-aggregate refresh
  policies (`internal/testfixtures`); a test that needs materialised data must refresh explicitly.
- Web (in `web/`): `pnpm lint && pnpm typecheck && pnpm check:api && pnpm check:i18n-parity && pnpm check:contrast`,
  `pnpm test --maxWorkers=2` (~100 s), `pnpm storybook:build` (~30 s), a11y in chunks
  (`STORIES='Features/Dashboard,Features/Consumption' pnpm test:a11y --workers=1`, ~3–4 min each), `pnpm build` (~60 s),
  `pnpm build && PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1` (~60 s). `make openapi` regenerates
  both the Go document and `schema.d.ts`. Never leave `next dev`/`next start`, `storybook dev`, `vitest --watch`, a
  static server or `bin/ekokod-e2e` running.
- Worktrees on the native filesystem (`/home/personal/...`) only; `/mnt/c` is slow.
- `make check-generate` needs a clean tree for `internal/store/postgres/sqlcgen`, so commit first.
- Commit trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.

## Key paths
```
docs/superpowers/plans/2026-09-17-f6b-core-screens.md            F6b plan (R190–R210, acceptance map)
docs/superpowers/plans/2026-09-17-f6a-auth-api-skeleton.md       F6a plan (R136–R189, permission matrix)
internal/domain/grouping/                                        consumption grouping (R193)
internal/service/{jobs,analysis,loadprofile}/, internal/credentials/
internal/api/v1/{routes.go,openapi.go,handlers_jobs.go}          route table, OpenAPI generator, job status
internal/seed/{readings.go,fixtures.go,demo.go}                  e2e and demo data (R194)
internal/testfixtures/containers.go                              isolated template DB, no refresh policies
web/src/features/{dashboard,consumption,load-profile,settings,calendar,scope,jobs}/
web/src/lib/api/{query.ts,query-errors.tsx,mutation.ts,download.ts,problem.ts}
web/src/components/ui/{table.tsx,stagger-grid.tsx}, web/src/styles/{tokens.css,event-palette.ts}
web/tests/e2e/                                                   7 specs, 39 tests (incl. responsive.spec.ts)
web/tests/a11y/                                                  stories, keyboard, touch-target and shell sweeps
/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite/.superpowers/sdd/2026-09-17-f6b/inline-ledger.md
```

## Verification at the F6b tip
- **Go `-race`, package by package:** auth, domain/grouping, service/{jobs,analysis,loadprofile}, api/v1 (+kit, mw), job,
  credentials, seed, scheduler, worker, arch — all `ok`.
- **Integration** (`-tags=integration`, shared Postgres/Redis): api/v1, credentials, seed, service/analysis `ok`;
  09 §F6 acceptance `TestAuthorizationMatrix` and `TestCredentialsNeverLeak` PASS.
- `golangci-lint run --concurrency 2 --build-tags=integration ./internal/...` → **0 issues**.
  `make check-generate` and `make openapi` → no diff.
- **Web:** `pnpm lint` 0, `pnpm typecheck` 0, `pnpm check:api` ok, `pnpm check:i18n-parity` **17 namespaces, 861 keys**,
  `pnpm check:contrast` **75 pairs × 2 themes**, `pnpm test --maxWorkers=2` **180 files, 644 tests**,
  `pnpm audit --audit-level=high` no known vulnerabilities, `pnpm build` ok.
- **a11y** (`pnpm storybook:build`, then chunks with `--workers=1`): Dashboard+Consumption 212, LoadProfile 98,
  Calendar 124, Settings 228, Shell 156, Jobs/Scope/Table/Domain 216 — all green.
- **e2e:** `pnpm build && PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1` → **39 passed**
  (auth, dashboard, consumption, load-profile, settings, calendar, responsive).
- No dev server, `next start`, Storybook, static server or `bin/ekokod-e2e` left running.
