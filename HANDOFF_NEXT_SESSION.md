# Handoff — ekokod rewrite: F1–F8a COMPLETE → next phase F8b (reports)

> **STATUS (2026-09-23):** **F8a (bills and tariffs) is finished** on `phase/f8a-bills-tariffs` in
> `/home/personal/ekokod-f8-phase`, branched from `phase/f7-alarms`. The invoice dashboard, its
> export, ad-hoc generation with every §7.10 validation message, and the whole §7.11 tariff surface
> — form with PTF+YEKDEM and KBK, version history, templates, bulk assignment with its new history
> table, the icmal review-and-confirm import, solar tariffs and the admin default catalogue — all
> ship. 16 commits, each with its own mutation proof, followed by a whole-phase self-review that
> found four more defects and fixed them.
> **F8 was split on the user's call:** F8a = bills + tariffs (done), **F8b = reports** (01 §7.14,
> 05 §11) — not started.
> **Working mode:** no parallel agents, no subagents, no Workflow. One session runs one phase
> inline, writes this handoff at the end, and the user runs `/clear`.
> **Nothing has been pushed and `main` is untouched. Pushing and merging are the user's call** —
> including whether to merge `phase/f8a-bills-tariffs` into `phase/f7-alarms` first.

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/`
> içinde, faz planı `docs/rewrite/09-implementation-plan.md` (16 faz, F0–F15).
>
> **Durum:** F1–F8a tamam. F8, benim kararımla ikiye bölündü: **F8a (faturalar + tarifeler)**
> `phase/f8a-bills-tariffs` dalında (`/home/personal/ekokod-f8-phase`) bitti. **Sıradaki faz F8b
> (Raporlar) ve henüz planı yok.** F4'ün gerçek-fatura karşılaştırması ürün sahibinden fatura
> gelene kadar AÇIK. Hiçbir şey push edilmedi, `main`'e dokunulmadı; push ve merge kararı bana ait.
>
> **Çalışma düzeni (kesin):** paralel agent YOK, subagent YOK, Workflow YOK. Fazı bu oturumda tek
> başına yürüt (`superpowers:executing-plans` + TDD). Review'ları kendin yap: her task sonunda
> diff'i oku, kendi mutasyonunla guard'ın kırmızıya düştüğünü kanıtla; faz sonunda bir kez bütün-faz
> self-review (yetki/tenancy önce, sonra erişilebilirlik ve tasarım kuralları). Faz bitince
> `HANDOFF_NEXT_SESSION.md`'i F9 için yeniden yaz, bana yapıştırılacak promptu ver ve dur.
>
> Şu sırayla ilerle:
> 1. `/home/personal/ekokod-f8-phase/HANDOFF_NEXT_SESSION.md`'i baştan sona oku (START HERE,
>    "What F8a built", Deviations, "F8b must pick up", Open questions, Environment). F8a planını
>    (`docs/superpowers/plans/2026-09-23-f8a-bills-tariffs.md`, R233–R253) ve F7 planını
>    (R211–R232) referans olarak kullan.
> 2. F8b worktree'sini aç: `git worktree add -b phase/f8b-reports /home/personal/ekokod-f8b-phase phase/f8a-bills-tariffs`,
>    ardından `web/` içinde `pnpm install --frozen-lockfile`.
> 3. **F8b planını yaz** (`superpowers:writing-plans`, `docs/superpowers/plans/` altına; sıkı:
>    imzalar, kural tablosu, isimlendirilmiş testler, doğrulama komutları). Kaynaklar:
>    `09-implementation-plan.md` §F8 (rapor maddeleri), `01-project-context.md` §7.14,
>    `05-api-contract.md` §11, `04-data-model.md` §8 (reports tablosu F1'de hazır),
>    `07-design-system.md`, `design-system/bcem-energy/MASTER.md` ve sonra `OVERRIDES.md`.
>    Planlamadan önce legacy rapor kodunu oku:
>    `/mnt/c/Users/meren/Desktop/Work/bcem-apps/bcem-energy/src/app/api/cron/generate-reports/route.ts`
>    (852 satır) ve `src/app/components/reports/` (MonthlyReportTab 1858, YearlyReportTab 768,
>    ReportsArchiveTab 625 satır). **Bana iki şeyi sor:** (a) yıllık rapordaki "hedef üretim"
>    legacy'de `kapasite × 1500` sabitiyle uyduruluyor — F1'deki `power_plant_monthly_targets` /
>    `yearly_target_kwh` kullanılsın mı, yoksa hedef yoksa "veri yok" mu densin; (b) rapor
>    e-postası eki (PDF + Excel) şirket SMTP'si üzerinden gidiyor ve `internal/mail` şu an yalnızca
>    düz metin gönderiyor — multipart/ek desteği F8b'de mi yazılsın. Planı bir kez spesifikasyona +
>    legacy'ye karşı kendin gözden geçir, sonra uygula.
>
> **Ortam ve güvenlik:** bu WSL kullanıcısında toolchain `$HOME`'da değil; her komutun başına
> `export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`
> ekle. Docker soketine erişim için her docker/integration/e2e komutunu `sg docker -c "..."` ile
> sar (kullanıcı `docker` grubunda ama bu oturumun kimlik bilgileri grubu almıyor; `sudo` şifre
> istiyor). Sadece üzerinde çalıştığın paketleri/test hedeflerini koştur; **asla `make test` veya
> `go test ./...` çalıştırma.** Integration testleri için önce `sg docker -c "docker ps"` ile
> `ekokod-test-pg` ve `ekokod-test-redis` var mı bak (yoksa `sg docker -c "make test-db-up"` /
> `make test-redis-up`), sonra
> `sg docker -c "EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <paket> -tags=integration -count=1 -parallel 4"`.
> Web'de `pnpm test --maxWorkers=2`; Storybook/Playwright/`next build` aynı anda değil. Playwright
> tarayıcıları bu kullanıcıda kurulu değil: `PLAYWRIGHT_BROWSERS_PATH=/home/personal/.cache/ms-playwright`
> ile mevcut önbelleği kullan. e2e için
> `pnpm build && sg docker -c "PLAYWRIGHT_BROWSERS_PATH=/home/personal/.cache/ms-playwright E2E_API_PORT=18082 PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1"`
> — **18080 dolmusum'a ait, kullanma.** Başka projenin Gradle/java/vitest süreçlerine, 5432'deki
> `dolmusum-dev-postgres-1` container'ına ve dolmusum'un portlarına dokunma.

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
| `phase/f6b-screens` (`/home/personal/ekokod-f6b-phase`) | `064688b` | F6b complete |
| `phase/f7-alarms` (`/home/personal/ekokod-f7-phase`) | `c8cbca3` | F7 complete |
| **`phase/f8a-bills-tariffs`** (`/home/personal/ekokod-f8-phase`) | this handoff commit | **F8a complete** (plan `c635744`; 16 commits) |

Shared test containers (both tmpfs, recreate freely): `ekokod-test-pg` (TimescaleDB, :55432,
`make test-db-up`) and `ekokod-test-redis` (:56379, `make test-redis-up`). Both were recreated in
this session after a Docker Desktop restart; the e2e run creates and drops `ekokod_e2e`.

- Plan: `docs/superpowers/plans/2026-09-23-f8a-bills-tariffs.md` (rulings **R233–R253**, rule
  tables, file map, tasks 1–15, acceptance map, product-owner decisions D-1…D-4, open questions
  Q-D1…Q-D7).
- Ledger (git-ignored, in this worktree):
  `.superpowers/sdd/2026-09-23-f8a-bills-tariffs/progress.md` — every ruling, the integration debt
  and how it was cleared, and the self-review findings.
- Working tree clean, 16 commits on top of F7.

---

## What F8a built

### Go
- **`internal/domain/billing/dashboard.go`** — `BuildDashboard(rows, buildingBills, companyBills)`:
  pure, no I/O. Groups the period's analyzer invoices by building, sums them, nets per currency,
  and flags a building invoice that differs from the sum of its rows. Every 09 §F8 reconciliation
  claim is a table test here.
- **`internal/service/billing/dashboard.go`** — one read of the period's bills in scope plus the
  names, then the aggregate; and `Requests.DashboardExport` for the file.
- **`internal/render/dashboardxlsx` / `dashboardpdf`** — the same aggregate as the screen, in both
  locales; the PDF is byte-stable with a golden hash, like the invoice PDF.
- **`internal/service/tariff`** — `solar.go` (plant feed-in prices + the platform default
  catalogue), `history.go` (R241's bulk-assignment record), `BulkAssign`/`ApplyTemplate` now take
  the acting user and record exactly once.
- **`internal/service/jobs`** — `billing.generate` is watchable and carries `error_code` from the
  closed R113 set (R237); the two payload families decode separately.
- **`internal/store/postgres`** — `BulkAssignmentRepository`, `OpsRepository.RunByTaskID`, the
  admin catalogue's list/delete, migrations **00016** (`job_runs.task_id`) and **00017**
  (`tariff_bulk_assignments`).
- **Nineteen routes** (05 §6–§7) with their authorisation matrix rows, the tenancy sweep extended
  to four new families, and `Route.MaxBody` so one route (the icmal upload) can exceed 1 MiB.
- **Eight permissions** (R246) and the regenerated web fixture.

### Web (all under `web/src`)
- **Bills** (`/ekorm/bills`): the §7.10 dashboard (one month control, buildings table with
  subtotals, netting cards per currency, the plant section's explicit unavailable state), the
  XLSX/PDF export, and ad-hoc generation that watches its job and offers the finished invoice.
- **Tariffs** (`/ekorm/tariffs`): six permission-gated tabs — building tariffs (the §7.11 form with
  its mode switches and the version history), templates, bulk assignment with its history, the
  three-step icmal import, solar tariffs and the admin default catalogue.
- Two i18n namespaces (`bills`, `tariffs`), five new validation-code messages in `forms`, and two
  new `SCREENS` lines in `responsive.spec.ts`.

### The decisions that shaped the phase (D-1…D-4 in the plan)
- **The dashboard reads the `bills` table and nothing else** (R233). A bill's period follows the
  building's cutoff day, so a calendar-month consumption query would have disagreed with the
  invoice beside it.
- **A building invoice is not the sum of its analyzer invoices** (02 §6.11). Both are shown and the
  difference is named (R234) — the seed makes them differ on purpose so the note is exercised.
- **The plant section has no data source before F9** (R235): `available: false` with a reason,
  never a zero-filled table. Legacy's own section is a design-only placeholder.
- **Currencies are never added together** (R253): one netting summary per currency, no FX.

---

## Defects this phase found in earlier work

1. **`payloadScope` could not decode a billing payload.** The integration tasks carry bare Go field
   names and `BillingGeneratePayload` carries snake_case tags, so every poll of a bill job would
   have answered 404. Caught by a unit test written against a realistic payload.
2. **`TestScopeIsolation`'s two guards did their job**: the new `BulkAssignmentRepository` was not
   registered, and `OpsRepository.RunByTaskID` was never exercised there. Both now are — the latter
   with two tenants holding the *same* deterministic task id.
3. **A negative solar feed-in price answered 400 `invalid_parameters`** instead of 422 with the
   field that failed, which 05 §1 requires for body validation.

## Deviations from the F8a plan (all in the ledger)
- **`POST /national-tariff-schedule` is an upsert answering 204**, and there is no PATCH: the table
  already declares `(effective_from, user_group, voltage_level, term)` unique, and the handler had
  been echoing an all-zero uuid.
- The dashboard export lives on `billingsvc.Requests`, not the handler: the PDF needs the company
  name and `Requests` already owns the repository that resolves it.
- `dashboardpdf` is built on `go-pdf/fpdf` like the invoice PDF; the plan named
  `internal/render/table`, which renders CSV/XLSX.
- `tariffsvc.ImportResult` gained a `Buildings` map (ETSO → building) so the review screen can name
  the building each derivation belongs to.
- Both screens carry the `ScopePicker` themselves; §7.11 is per building and the Tariffs page had
  been depending on a selection made on another screen.
- The tab-gating code in `tariffs-page.tsx` was written before its test (recorded in the ledger);
  the gate was then proved by mutation rather than by a RED-first run.

## Environment surprises worth keeping
- **This WSL user has no toolchain in `$HOME`**: Go, Node and pnpm live under `/home/personal/.local`.
- **Docker needs `sg docker -c "…"`**: the user is in the `docker` group, but this login's
  credentials predate it and `sudo` wants a password. `sg` needs neither.
- **Playwright browsers are not installed for this user**; `PLAYWRIGHT_BROWSERS_PATH=/home/personal/.cache/ms-playwright`
  reuses the existing cache.
- The main checkout's `.git` lives on `/mnt/c` and its config file cannot be chmod'ed by this user:
  `git config` writes to the repo fail, so the identity lives in `~/.gitconfig`.

---

## F8b must pick up
- **Branching:** start from `phase/f8a-bills-tariffs` (see the prompt).
- **Reports (§7.14):** monthly, yearly and archive tabs with every table, chart, summary card and
  section; PDF and Excel; e-mail sending; `report.generate` and `report.deliver` jobs; and
  `GET /reports/preview` for the live tabs. The `reports` table and `ReportRepository` are F1's and
  still unused.
- **Seven endpoints of 05 §11** are missing entirely: `GET /reports`, `GET /reports/{id}`,
  `POST /reports/generate`, `GET /reports/{id}/pdf`, `GET /reports/{id}/excel`,
  `POST /reports/{id}/email`, `GET /reports/preview`.
- **Reuse, do not reinvent:** `internal/render/dashboardxlsx` and `dashboardpdf` are the shape a
  report renderer should take (same aggregate in, both locales, golden-hashed PDF);
  `internal/mail` sends in both locales through the company's SMTP settings and records a failed
  send the way R222 does; `ops.Triggerable` is the one place a job becomes operator-triggerable.
- **Do not repeat legacy's invented figures:** the yearly report's "target production" is
  `capacity × 1500` there, and its "actual" is read from the *building's* analyzers with a comment
  admitting the plant link was never built. F1 has `power_plant_monthly_targets` and
  `yearly_target_kwh`; anything they cannot answer is "veri yok" (R165's house style).
- **Screen conventions are set** (R195–R199, R229, R249): route → `*-page` container → views;
  `useApiMutation` for every write; `downloadFile` for every export; `mockApi` for container tests;
  stories stay data-free; every new screen needs a line in `tests/e2e/responsive.spec.ts`'s
  `SCREENS`; `enabled` goes in the **options** argument of `$api.useQuery`; a dialog that embeds a
  form passes `hideActions` so there is one save button.
- **Later phases:** F9 solar plant realtime/production, the iSolar screens **and iSolar alarm
  forwarding** (D-3), plus the plant↔production link the bills dashboard's plant section waits for;
  F12 public site (the `national-tariff-schedule` GET is already public for its calculator); F13
  the real `/anomaly/check` model; F15 CSP nonces, the supported-browser floor,
  `EKOKOD_TRUSTED_PROXIES` and rate limiting for the public routes.

---

## Open questions for the product owner (defaults ship)
- **F8a (new):** **Q-D1** R234 shows the building invoice beside the analyzer rows and names the
  difference — should the building invoice be the headline figure instead, with the rows as its
  breakdown? · **Q-D2** R235 leaves the dashboard's plant section empty until F9: should F9 add a
  plant↔analyzer link, or do plant figures come from iSolar only? · **Q-D3** R242 does not rebuild
  legacy's "clear tariff history": is there an operational need to delete old versions, given that
  issued bills reference them? · **Q-D4** R243 makes the admin default tariffs the national
  schedule rows; legacy also had a *solar* default catalogue with no table behind it — drop it for
  good? · **Q-D5** R251 ships one month control instead of §7.10's year + month pair — confirm. ·
  **Q-D6** R253 gives a two-currency month two netting summaries; should the screen instead ask the
  operator to pick a currency? · **Q-D7** F7's Q-C8 is still open: should `consumption.refresh` get
  a period-taking trigger?
- **F7:** Q-C1…Q-C7 unchanged (alarm edit roles, R213 visibility, non-firing evaluations, MaxDemandKw
  as "Güç Maks/Min", plain-text alarm mail, the frequency window's granularity, a Messages date
  filter).
- **F6b:** Q-B1 Monday-start weeks · Q-B2 integration credentials admin-only · Q-B3 year-over-year
  chart scope · Q-B4 R94's monthly-bucket divergence.
- **F6a:** Q-A1…Q-A5 unchanged. **F5:** Q1–Q7 unchanged.
- **F4 (unchanged):** real-invoice comparison OPEN; dated `billing_parameters` defaults (R106,
  reactive basis, exemptions, R118, tiering, PTF tolerance, rounding R112); R119, R111, R110, R114,
  R122, R135; OG transformer loss.
- **F1–F3 (unchanged):** Q1 ratio zero-case, Q3 weekend default, Q5–Q10; F2 device/integration
  items. Q4 is decided: no (R137).

## Binding rulings
F1 rulings 1–14, F2 R1–R53, F3 R54–R104, F4 R105–R135, F5 D1–D26, F6a R136–R189, F6b R190–R210,
F7 R211–R232, **F8a R233–R253** (`docs/superpowers/plans/2026-09-23-f8a-bills-tariffs.md`), plus
each phase's deviations.

## Process rules that paid off (keep them, inline)
- **Read legacy code and real data before writing rules.** The dashboard's plant section, the
  "clear history" endpoint and the invented production target were all settled by opening the
  legacy tree, not by reasoning from the spec.
- **Prove every guard red with your own mutation, restoring from a file backup.** Every task here
  did, and one mutation (`kbkT1` → `kbk_t_1`) exposed a field-name bug no other test would have
  caught.
- **A unit test against a fake proves the fake.** The billing payload decode bug passed every unit
  test until one used a realistic payload; the scope-isolation gaps only appeared once Docker came
  back and the integration suite ran.
- **When the environment blocks a gate, say so and keep the debt visible.** Docker was down for the
  first half of this session; every unrunnable test was written, compiled with
  `go vet -tags=integration`, and recorded as debt in the ledger — then run and fixed the moment
  the socket appeared.
- **Per-task gates run the whole touched package**, and the whole web suite before every commit.
- **Commit as soon as tests are green,** then the mutations, then the next task. Comments say WHY
  in 1–3 lines.

## Key paths
```
docs/superpowers/plans/2026-09-23-f8a-bills-tariffs.md            F8a plan (R233–R253, D-1…D-4, Q-D1…Q-D7)
docs/superpowers/plans/2026-09-18-f7-alarms-messages.md           F7 plan (R211–R232)
internal/domain/billing/dashboard.go                              the pure dashboard aggregate
internal/service/billing/dashboard.go                             the scoped read + the export
internal/render/dashboardxlsx/, internal/render/dashboardpdf/     the two dashboard renderers
internal/service/tariff/{solar.go,history.go,icmal.go,templates.go}
internal/service/jobs/jobs.go                                     R237's watchable billing job + error_code
internal/api/v1/{handlers_tariffs.go,handlers_billing.go,dto/tariffs_extra.go,dto/bills.go}
internal/store/postgres/migrations/{00016_job_run_task_id.sql,00017_tariff_bulk_assignments.sql}
internal/seed/tariffs.go                                          e2e bills, template, assignment, icmal, plant, catalogue
web/src/features/{bills,tariffs}/
web/tests/e2e/{bills,tariffs,responsive}.spec.ts
/home/personal/ekokod-f8-phase/.superpowers/sdd/2026-09-23-f8a-bills-tariffs/progress.md
```

## Verification at the F8a tip
- **Go `-race`, package by package:** domain/billing, domain/tariff, service/{billing,tariff,jobs,
  consumption,alarms}, render (+dashboardxlsx, dashboardpdf, invoicepdf, hourlyxlsx, table), auth,
  api/v1 (+kit, mw), store (+postgres, admin, pgerr, pgnum, redis), job, worker, seed — all `ok`.
- **Integration** (`-tags=integration`, shared Postgres/Redis): api/v1, service/billing,
  service/tariff, store/postgres (including migrations 00016/00017 round-trip and
  `TestScopeIsolation`), store/postgres/admin, seed, service/alarms, service/consumption,
  credentials — all `ok`.
- `golangci-lint run --concurrency 2 --build-tags=integration ./internal/...` → **0 issues**.
  `make check-generate` and `make openapi` → no diff.
- **Web:** `pnpm lint` 0, `pnpm typecheck` 0, `pnpm check:api` ok, `pnpm check:i18n-parity`
  **21 namespaces, 1260 keys**, `pnpm check:contrast` **75 pairs × 2 themes**,
  `pnpm test --maxWorkers=2` **203 files, 800 tests**, `pnpm build` ok.
- **a11y:** `pnpm storybook:build`, then the whole a11y project → **200 passed** (axe, keyboard,
  touch targets, reduced motion, overlay motion).
- **e2e:** `pnpm build` + `--project=e2e --workers=1` → **64 passed** over nine screens, including
  the 14 new bills/tariffs cases; the inherited `calendar › a company admin deletes the event
  again` flake passed in this run.
- No dev server, `next start`, Storybook, static server or `bin/ekokod-e2e` left running by this
  session; both test containers are up on 55432/56379 and may be removed freely.
