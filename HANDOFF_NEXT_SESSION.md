# Handoff — ekokod rewrite: F1–F8b COMPLETE → next phase F9 (solar, renewable, financial)

> **STATUS (2026-09-23):** **F8b (reports) is finished** on `phase/f8b-reports` in
> `/home/personal/ekokod-f8b-phase`, branched from `phase/f8a-bills-tariffs`. The monthly, yearly
> and archive tabs of 01 §7.14, the combined live preview, one persisted report per building with
> its PDF and workbook (every §7.14 section, both locales), e-mail delivery with both files
> attached through the company's SMTP settings, and the `report.generate` / `report.deliver` jobs
> with their monthly and yearly schedules — all ship. 29 commits, each proved by its own mutation,
> then a whole-phase self-review (and a real e2e run with a worker) that found and fixed defects in
> this phase **and in F4, F6b, F7 and F8a** (below).
> **F8 is done** (F8a bills + tariffs, F8b reports). **F9 is next and has no plan yet.**
> **Working mode:** no parallel agents, no subagents, no Workflow. One session runs one phase
> inline, writes this handoff at the end, and the user runs `/clear`.
> **Nothing has been pushed and `main` is untouched. Pushing and merging are the user's call** —
> including the order in which the phase branches are merged.

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/`
> içinde, faz planı `docs/rewrite/09-implementation-plan.md` (16 faz, F0–F15).
>
> **Durum:** F1–F8 tamam (F8a faturalar + tarifeler, F8b raporlar). F8b `phase/f8b-reports`
> dalında (`/home/personal/ekokod-f8b-phase`) bitti. **Sıradaki faz F9 (Güneş santralleri,
> yenilenebilir enerji, finansal analiz) ve henüz planı yok.** F4'ün gerçek-fatura karşılaştırması
> ürün sahibinden fatura gelene kadar AÇIK. Hiçbir şey push edilmedi, `main`'e dokunulmadı; push ve
> merge kararı bana ait.
>
> **Çalışma düzeni (kesin):** paralel agent YOK, subagent YOK, Workflow YOK. Fazı bu oturumda tek
> başına yürüt (`superpowers:executing-plans` + TDD). Review'ları kendin yap: her task sonunda
> diff'i oku, kendi mutasyonunla guard'ın kırmızıya düştüğünü kanıtla; faz sonunda bir kez bütün-faz
> self-review (yetki/tenancy önce, sonra erişilebilirlik ve tasarım kuralları) ve worker'lı gerçek
> e2e koşusu. Faz bitince `HANDOFF_NEXT_SESSION.md`'i F10 için yeniden yaz, bana yapıştırılacak
> promptu ver ve dur.
>
> Şu sırayla ilerle:
> 1. `/home/personal/ekokod-f8b-phase/HANDOFF_NEXT_SESSION.md`'i baştan sona oku (START HERE,
>    "What F8b built", "Defects this phase found in earlier work", Deviations, "F9 must pick up",
>    Open questions, Environment). F8b planını (`docs/superpowers/plans/2026-09-23-f8b-reports.md`,
>    R254–R275, E-1…E-3) ve F8a planını (R233–R253) referans al.
> 2. F9 worktree'sini aç: `git worktree add -b phase/f9-solar /home/personal/ekokod-f9-phase phase/f8b-reports`,
>    `git config --global --add safe.directory /home/personal/ekokod-f9-phase`, ardından `web/`
>    içinde `pnpm install --frozen-lockfile`.
> 3. **F9 planını yaz** (`superpowers:writing-plans`, `docs/superpowers/plans/` altına; sıkı:
>    imzalar, kural tablosu, isimlendirilmiş testler, doğrulama komutları). Kaynaklar:
>    `09-implementation-plan.md` §F9, `01-project-context.md` §7.7–§7.9, `05-api-contract.md` §8–§9,
>    `06-integrations.md` (iSolarCloud, hava durumu), `04-data-model.md` §3 ve §4.5,
>    `10-removed-behaviours.md` (uydurma veri maddeleri), `07-design-system.md`,
>    `design-system/bcem-energy/MASTER.md` ve sonra `OVERRIDES.md`. Planlamadan önce legacy kodu oku
>    (`bcem-apps/bcem-energy/src/app/components/` altındaki solar/renewable/financial bileşenleri ve
>    `src/app/api/` altındaki isolar rotaları) ve her panelin veri kaynağını tek tek çıkar.
>    **Bana üç şeyi sor:** (a) santral↔bina/sayaç bağı kurulsun mu — raporların "çatı GES üretimi"
>    şu an binanın sayaç ihracatı (R256, Q-E1) ve fatura panosunun santral bölümü veri kaynağı
>    bekliyor (Q-D2); (b) iSolarCloud'un cihazsız (santral düzeyinde) örnekleri: F2'den beri
>    karantinada, `plant_production.device_id` NOT NULL — şema değişsin mi, yoksa yalnızca cihaz
>    örnekleri mi kullanılsın; (c) bir bina yöneticisi, arşivdeki kendi bina raporunda şirketin
>    santral üretimini görsün mü (Q-E7). Planı bir kez spesifikasyona + legacy'ye karşı kendin
>    gözden geçir, sonra uygula.
>
> **Ortam ve güvenlik:** her komutun başına
> `export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`
> ekle. Docker/integration/e2e komutlarını `sg docker -c "…"` ile sar. **Asla `make test` veya
> `go test ./...` çalıştırma.** Integration: önce `sg docker -c "docker ps -a"` — Docker yeniden
> başladığında `ekokod-test-pg`/`ekokod-test-redis` **Exited** kalıyor, `sg docker -c "docker start
> ekokod-test-pg ekokod-test-redis"` ile kaldır; sonra
> `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <paket> -tags=integration -count=1 -parallel 4`.
> Kendi container'ını açan paketleri (`internal/scheduler`, `internal/worker`, `internal/job`)
> diğerleriyle aynı anda değil, tek tek koştur (birlikte koşunca Docker soketinde zaman aşımı
> oluyor). Web'de `pnpm test --maxWorkers=2`; Playwright için
> `PLAYWRIGHT_BROWSERS_PATH=/home/personal/.cache/ms-playwright`, e2e portu `E2E_API_PORT=18082`
> (18080 dolmusum'un); e2e artık API'nin yanında bir worker da başlatıyor ve Redis 6/7'yi
> temizliyor. Storybook/Playwright/`next build` aynı anda değil. Başka projenin
> container/port/süreçlerine dokunma.

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
| `phase/f8a-bills-tariffs` (`/home/personal/ekokod-f8-phase`) | `0a1b71c` | F8a complete |
| **`phase/f8b-reports`** (`/home/personal/ekokod-f8b-phase`) | this handoff commit | **F8b complete** (plan `f0de6d9`; 29 commits) |

- Plan: `docs/superpowers/plans/2026-09-23-f8b-reports.md` (rulings **R254–R275**, product-owner
  decisions **E-1…E-3**, rule tables, payload v1, tasks 1–15, acceptance map, open questions
  **Q-E1…Q-E6**; Q-E7 added during execution).
- Ledger (git-ignored): `.superpowers/sdd/2026-09-23-f8b-reports/progress.md` — every ruling, every
  mutation, every finding, the self-review.
- Working tree clean. Both test containers are up on 55432/56379 and may be recreated freely.

---

## What F8b built

### Go
- **`internal/domain/report`** — pure: `BuildMonthly`, `BuildYearly`, `PlantTarget`, `DeltaOf`.
  Nullable figures with coverage (`with_data`/`of`), `excluded` for the plant selection,
  per-currency money, R259 deltas, R260 targets, R261 shares, R262 carbon (not clamped). Payload v1
  with a `version`. Hand-computed fixtures for both reports (09 §F8 acceptance).
- **`internal/service/report`** — `Service` (scoped reads → domain; `Preview`, `Build`,
  `Validate`), `Requests` (API side: `Enqueue`, `EnqueueDelivery`, `Archive`, `Get`, `File`),
  `Generator`, `Deliverer`, `Dispatcher`, `Files` (store temp-then-rename, confined reads, render
  on demand), `EmailContent`/`EmailText` (html/template, escaped).
- **`internal/render/reportview`** (one section list) → **`reportpdf`** (byte-stable, golden hashes,
  drawn bar charts) and **`reportxlsx`** (report sheet + data sheet + native charts).
- **`internal/mail`** — multipart: attachments (RFC 2231 names) and an HTML alternative; the plain
  single-part output is byte-for-byte unchanged (golden test).
- **`internal/job`** — `report.dispatch_monthly` / `_yearly`, `report.generate`, `report.deliver`;
  **`EnqueueReplacingFinished`** (a deliberate re-run replaces a finished task holding its id).
- **`internal/service/jobs`** — report jobs are watchable; a finished task is answered from its
  `job_runs` row (R267); closed code set extended.
- **Store** — `ReportFilter.Year`, `ReportRepository.Count` (+ scope-isolation coverage).
- **API** — seven 05 §11 routes, three permissions (`reports.read|generate|email`), matrix rows,
  tenancy sweep family `reports`, OpenAPI regenerated, trigger enum extended.
- **Scheduler/ops/worker** — both ticks scheduled (`0 6 2 * *`, `0 7 3 1 *`) and triggerable; the
  worker builds the generator, deliverer and dispatcher.
- **Seed** — a utility-scale plant with a yearly target and monthly production, and a completed
  report for A1's previous month.

### Web (`web/src/features/reports`, `/ekorm/reports`)
- Selection (buildings with select-all, month/year, plant selection with counter and the rooftop
  note; the plant picker only for `nav.solar_plants`), the monthly view (15-row information table,
  four cards with YoY, two grouped bar charts), the yearly view (consumption table, solar section,
  cards, three charts, final comparison, carbon), per-building generation → watch → PDF/Excel/e-mail
  (dialog with summary), the archive (filters, total, year grouping, availability, paging).
- `reports` i18n namespace (22 namespaces, 1423 keys), stories for every view, `SCREENS` line,
  `tests/e2e/reports.spec.ts` (6 cases incl. the R267 bill regression).

### Decisions (E-1…E-3)
- **E-1** target = `yearly_target_kwh`, else the sum of all twelve monthly targets, else "veri yok";
  never `capacity × 1500`.
- **E-2** multipart mail in F8b.
- **E-3** combined live preview; one report per building for files and e-mail.

---

## Defects this phase found in earlier work (all fixed, each with a test that failed first)

1. **F8a — a finished bill job answered 404.** `billing.generate` has no asynq `Retention` (R53), so
   asynq deletes it on success; the bills screen said "failed" for every successful generation.
   Now answered from its `job_runs` row (R267).
2. **F6b — percent-encoded path values were never decoded.** chi returns the escaped segment when
   the URL has a `RawPath`; the browser sends a task id's colons as `%3A`, so `GET /jobs/{id}`
   answered 404 for every billing/report/deliver job a screen watched. `kit.Bind` unescapes.
3. **F4/F8a — an archived failure blocked every re-run.** A failed generation is archived under its
   deterministic id and asynq refused the next enqueue as a duplicate, silently, for the archive's
   lifetime. `EnqueueReplacingFinished` now replaces a finished holder (bills and reports).
4. **F7 — the scheduler's integration test could not pass** since F7 (its config lacked the alarm
   cron). F8a's verification list never ran `internal/scheduler`.
5. **F8a seed — "the first plant" was ambiguous** once a second plant existed; a re-run tariffed the
   wrong one.
6. **F8b itself, found by the e2e run with a real worker:** null arrays in the payload (fixed:
   every array filled), the archive's first page sending `cursor=0` (fixed), a building-scoped
   principal seeing company plants in a preview (fixed: none, and naming one is 404), the archive
   silently truncated at one page (fixed: cursor paging), and the EmailDialog stories rendering an
   empty canvas (fixed: trigger + play).

## Deviations from the F8b plan (all ledgered with their cost)
- `internal/render/reportview` added so the PDF and workbook print one section list.
- "Every section" in the PDF is proved with a recording drawer (UTF-8 fonts glyph-encode the text);
  the drawing itself is covered by byte-stability, golden hashes and a manual look.
- The API side lives on `report.Requests` (like `billing.Requests`), not on `Service`.
- Figure gained `excluded`; Monthly/Yearly gained `partial`; Yearly gained `daily_utility`.
- A chart with no values becomes the sentence "Bu grafik için veri yok." (07 §5).
- The payload DTO is filled by a JSON round-trip and guarded by
  `TestReportPayloadDTOKeepsEveryField` and `TestReportPayloadHasNoNullArrays`.
- Several jobs are watched with one `useQueries` (`useReportJobs`), not one `useJob` per row.
- The plan's route-order mutation was moot (Go's mux prefers the literal `/reports/preview`).
- Config needed no change: the two report schedules were already in F0's config.

## Environment surprises worth keeping
- **Docker restarts leave the two test containers `Exited`** — happened twice this session.
  `sg docker -c "docker start ekokod-test-pg ekokod-test-redis"`.
- **Container-owning packages time out together** (`internal/scheduler`, `internal/worker`,
  `internal/job` start their own Redis/Postgres): run them one at a time.
- **e2e now runs a worker** beside the API (`web/scripts/e2e-api.sh`, trap-killed) and flushes Redis
  DBs 6/7 at start, the way it already recreates `ekokod_e2e`. Jobs a screen starts really finish.
- `pdftoppm` is not installed; the Read tool opens PDFs directly for a visual check.
- Each new worktree needs `git config --global --add safe.directory <path>`.

---

## F9 must pick up
- **Branching:** from `phase/f8b-reports` (see the prompt).
- **Scope (09 §F9):** Solar Power Plants (§7.7) four tabs, plant management + iSolar link modal,
  Renewable Energy (§7.8) both tabs, Financial Analysis (§7.9), weather adapter/panel — **no
  fabricated data**; `TestNoSyntheticData` enumerates every panel's source.
- **The data gap F8 leaves behind:** `plant_production` is only filled by F9's `isolar.sync_plant`
  (F2 built the blocker-aware store seam; plant-level samples are quarantined). Once it runs, the
  reports' utility-scale figures and targets fill in on their own; the bills dashboard's plant
  section (R235) needs F9's answer to Q-D2/Q-E1.
- **iSolar alarm forwarding** (D-3, F7) and deduplication by alarm id.
- **Reuse, do not reinvent:** `reportview`-style one-source rendering; `job.EnqueueReplacingFinished`
  for any deterministic-id job a user can re-run; `useReportJobs` for multiple jobs;
  `R258` coverage figures and `excluded`; `DataQualityBadge` for partial data; the plant picker's
  `nav.solar_plants` gate matches `GET /power-plants` (A CA CR).
- **Screen conventions** (R195–R199, R229, R249): route → `*-page` → views; `useApiMutation`;
  `downloadFile`; `mockApi` — **make mocks refuse what the API refuses** (the `cursor=0` defect
  passed a permissive mock); data-free stories; an already-open dialog story needs trigger + play;
  a `SCREENS` line per new screen.
- **Later phases:** F10 carbon (the report's grid-factor lookup is R162's), F12 public site, F13 ML
  (`/anomaly/check`), F15 CSP nonces, browser floor, `EKOKOD_TRUSTED_PROXIES`, rate limits.

---

## Open questions for the product owner (defaults ship)
- **F8b (new):** **Q-E1** rooftop production is the buildings' metered export (R256) — add a
  plant↔building link in F9? · **Q-E2** the bill follows the period key while consumption is the
  calendar month (R255) — pro-rate instead? · **Q-E3** a net exporter shows a negative net emission
  (R262) — confirm. · **Q-E4** scheduled reports are generated but never e-mailed; add a company
  report-recipient list? · **Q-E5** stored files are Turkish, English is rendered on demand — a
  company report language? · **Q-E6** twelve zero monthly targets are "no target" — confirm. ·
  **Q-E7** a report the worker generates carries the company's plants, and the building's BA can
  read it in the archive, although a BA's live preview shows no plants — acceptable?
- **F8a:** Q-D1…Q-D7 unchanged. **F7:** Q-C1…Q-C7 unchanged. **F6b:** Q-B1…Q-B4. **F6a:**
  Q-A1…Q-A5. **F5:** Q1–Q7. **F4:** real-invoice comparison OPEN and its listed items.
  **F1–F3:** unchanged; F2's plant-level iSolar sample question is F9's.

## Deferred minors (from the self-review)
- A pending report's `processed_at` is its enqueue time (`UpdateStatus` stamps it).
- The screen's default month/year uses the browser's clock, not Istanbul's.
- A failed regeneration marks the report "error" and hides its still-valid older files.

## Binding rulings
F1 rulings 1–14, F2 R1–R53, F3 R54–R104, F4 R105–R135, F5 D1–D26, F6a R136–R189, F6b R190–R210,
F7 R211–R232, F8a R233–R253, **F8b R254–R275 + E-1…E-3**
(`docs/superpowers/plans/2026-09-23-f8b-reports.md`), plus each phase's ledgered deviations.

## Process rules that paid off (keep them, inline)
- **Run the real stack with a worker before calling a phase done.** Five of this phase's defects
  (two of them in F6b and F8a) only appeared once jobs really ran end to end.
- **Prove every guard red with your own mutation, restoring from a file backup** — and when a
  mutation shows nothing, check it compiled (one here failed to build and printed no result).
- **A test that compares a renderer against the same source it renders from cannot see that source
  lose a section** — add one independent assertion.
- **When the code came first, say so in the ledger and prove each test by mutation.**
- **Per-task gates run the whole touched package**; the whole web suite before each commit.

## Key paths
```
docs/superpowers/plans/2026-09-23-f8b-reports.md                   F8b plan (R254–R275, E-1…E-3, Q-E1…Q-E6)
internal/domain/report/                                            the pure monthly/yearly reports
internal/service/report/                                           reads, preview, generate, files, deliver, dispatch, e-mail
internal/render/{reportview,reportpdf,reportxlsx}/                 one section list, two renderers
internal/mail/{sender.go,multipart.go}                             multipart mail
internal/job/{report.go,requeue.go}                                report jobs; EnqueueReplacingFinished
internal/service/jobs/jobs.go                                      watchable report jobs; R267 run fallback
internal/api/v1/{handlers_reports.go,dto/reports.go}               the seven routes
internal/api/v1/kit/bind.go                                        path values unescaped
internal/seed/reports.go                                           grid plant, production, a completed report
web/src/features/reports/                                          the screen
web/tests/e2e/reports.spec.ts, web/scripts/e2e-api.sh              e2e with a worker
/home/personal/ekokod-f8b-phase/.superpowers/sdd/2026-09-23-f8b-reports/progress.md
```

## Verification at the F8b tip
- **Go `-race`:** domain/..., mail, render/..., service/..., job, worker, scheduler, platform/config,
  auth, api/..., apiwire, store/..., seed — **45 packages ok**; `internal/arch` ok.
- **Integration** (`-tags=integration`): service/report, api/v1, store/postgres (incl.
  `TestScopeIsolation`), store/postgres/admin, seed, service/{billing,alarms,tariff,consumption,jobs}
  ok; scheduler, worker, job ok when run one at a time.
- `golangci-lint run --concurrency 2 --build-tags=integration ./internal/...` → **0 issues**;
  `make check-generate` and `make openapi` → no diff.
- **Web:** lint 0, typecheck 0, `check:api` ok, `check:i18n-parity` **22 namespaces, 1423 keys**,
  `check:contrast` **75 pairs × 2 themes**, `pnpm test --maxWorkers=2` **211 files, 847 tests**,
  `pnpm build` ok.
- **a11y:** full run 2074 passed with the six EmailDialog stories failing on an empty canvas;
  fixed, and `Features/Reports` re-run **90/90**.
- **e2e** (with a worker): **70 passed** over ten screens, including the responsive sweep of
  `/ekorm/reports` and the R267 bill regression.
- No dev server, `next start`, Storybook, static server, `bin/ekokod-e2e` or worker left running.
