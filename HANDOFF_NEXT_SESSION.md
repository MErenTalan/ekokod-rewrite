# Handoff — ekokod rewrite: F1–F5 COMPLETE → next phase F6 (core application screens)

> **STATUS (2026-09-17):** F5 (design system) is finished on `phase/f5-design-system` in `/home/personal/ekokod-f5-phase`.
> All 8 tasks are merged `--no-ff`, each with its own mutation proofs, plus one whole-phase self-review (accessibility,
> token/theme consistency, design rules) and a full acceptance sweep.
> F4 (billing engine) is finished on `phase/f4-billing-engine` (`/home/personal/ekokod-f4-phase`). F4 and F5 both branch
> from the F3 tip `58597ec` and **merge cleanly** (`git merge-tree`, no conflicted path) — except this file, which F4
> also rewrote: take F5's version.
> **F6 has no written plan yet.** The next session writes it first.
> **Working mode:** no parallel agents, no subagents, no Workflow. One session runs one phase inline, writes this handoff at
> the end, and the user runs `/clear`.
> **Nothing has been pushed and `main` is untouched. Pushing and merging to `main` are the user's call.**

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/` içinde, faz planı
> `docs/rewrite/09-implementation-plan.md` (16 faz, F0–F15).
>
> **Durum:** F1–F5 tamam. F4 `phase/f4-billing-engine` (`/home/personal/ekokod-f4-phase`), F5 `phase/f5-design-system`
> (`/home/personal/ekokod-f5-phase`) üzerinde bitti; ikisi de F3 ucundan (`58597ec`) açıldı ve çakışmasız birleşiyor
> (tek istisna `HANDOFF_NEXT_SESSION.md`: F5'inkini al). F4'ün gerçek-fatura karşılaştırması ürün sahibinden fatura
> gelene kadar AÇIK. **Sıradaki faz F6 (çekirdek uygulama ekranları) ve henüz planı yok.** Hiçbir şey push edilmedi,
> `main`'e dokunulmadı; push ve merge kararı bana ait.
>
> **Çalışma düzeni (kesin):** paralel agent YOK, subagent YOK, Workflow YOK. Fazı bu oturumda tek başına yürüt
> (`superpowers:executing-plans` + TDD). Review'ları kendin yap: her task sonunda diff'i oku, kendi mutasyonunla
> guard'ın kırmızıya düştüğünü kanıtla; faz sonunda bir kez bütün-faz self-review (yetki/tenancy önce). Faz bitince
> `HANDOFF_NEXT_SESSION.md`'i F7 için yeniden yaz, bana yapıştırılacak promptu ver ve dur.
>
> Şu sırayla ilerle:
> 1. `/home/personal/ekokod-f5-phase/HANDOFF_NEXT_SESSION.md`'i baştan sona oku (START HERE, F5 sonucu, "F6 must pick
>    up", Open questions, Environment). F4 ayrıntıları için `/home/personal/ekokod-f4-phase/HANDOFF_NEXT_SESSION.md`'in
>    "What F4 built", "Deviations" ve "Carried forward" bölümlerini de oku.
> 2. F6 worktree'sini aç: `git worktree add -b phase/f6-core-screens /home/personal/ekokod-f6-phase phase/f5-design-system`,
>    sonra orada `git merge --no-ff phase/f4-billing-engine` (çakışma yalnızca `HANDOFF_NEXT_SESSION.md` → F5'inki).
>    `web/` içinde `pnpm install --frozen-lockfile`.
> 3. **F6 planını yaz** (`superpowers:writing-plans`, `docs/superpowers/plans/` altına; sıkı: imzalar, kural tablosu,
>    isimli testler, doğrulama komutları). Kaynak: 09 §F6, `01-project-context.md` §2 rol matrisi ve §7.1–§7.4, §7.15,
>    §7.18, `05-api-contract.md`, `03-target-architecture.md`. Legacy auth/settings/calendar kodunu ve gerçek veriyi plan
>    öncesi oku. F6 çok büyük: planda F6a (auth, RBAC, OpenAPI istemcisi, mobil API, uygulama iskeleti) ve F6b (ekranlar,
>    takvim) olarak bölmeyi değerlendir ve bölümü bana sor. Bu handoff'taki "F6 must pick up" listesinin tamamı planda
>    karşılanmalı. Planı kendin bir kez spec + legacy'ye karşı gözden geçir, sonra yürüt.
>
> Ortam: her komuttan önce `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
> Sadece üzerinde çalıştığın paketleri/test hedeflerini koş, asla `make test` / `go test ./...` koşma. Integration testi
> için paylaşımlı Postgres: `docker ps` ile `ekokod-test-pg` açık mı bak (yoksa `make test-db-up`; Docker kapalıysa benden
> başlatmamı iste), sonra
> `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <paket> -tags=integration -count=1 -parallel 4`.
> Web: `pnpm test --maxWorkers=2`, Storybook/Playwright/`next build` tek tek; ağır işlerden önce `free -m` ve
> `ps -eo rss,comm | grep java` ile belleğe bak (bellek darsa Playwright `--workers=1`). Başka projenin Gradle/java
> süreçlerine, 5432'deki `dolmusum-dev-postgres-1` container'ına ve 8080'deki `dolmusum serve`'e dokunma.

---

## START HERE — exact state

| Branch / worktree | Head | State |
|---|---|---|
| `phase/f1-data-model` (main checkout `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite`) | `10d6577` | F1 complete |
| `phase/f2-integration-layer` | `7dbe1ea` | F2 complete |
| `phase/f3-consumption-engine` (`/home/personal/ekokod-f3-phase`) | `58597ec` | F3 complete |
| `phase/f4-billing-engine` (`/home/personal/ekokod-f4-phase`) | `cf79346` | F4 complete; real-invoice comparison OPEN |
| **`phase/f5-design-system`** (`/home/personal/ekokod-f5-phase`) | this handoff commit | **F5 complete** (tasks 1–8 merged) |
| `f5/task-1` … `f5/task-8` | merged | local branches only; can be deleted |

`ekokod-test-pg` (shared test Postgres, port 55432, tmpfs) may or may not be running; recreate with `make test-db-up`.

## What F5 built (`web/`)
- **Design system of record:** `design-system/bcem-energy/MASTER.md` (generated by `ui-ux-pro-max`, never edit) and
  `OVERRIDES.md` (binding; where MASTER disagrees, OVERRIDES + `docs/rewrite/07-design-system.md` win). **Every UI plan
  must say "read MASTER.md, then OVERRIDES.md".**
- **Tokens:** `web/src/styles/tokens.css` — one `light-dark()` declaration per colour; Tailwind v4 `@theme static` with
  the default palette, shadows and radii wiped. `<html data-theme="light|dark|system">` drives `color-scheme` (no script).
  Composite type utilities `type-display…type-data`, duration tokens, overlay keyframe tokens (`animate-dialog-in` …).
  Light `primary` is emerald-700 `#047857`; spec `#059669` survives as `brand` (graphics/logo only, Q4).
- **Fonts:** self-hosted subset woff2 (Lexend, Source Sans 3, JetBrains Mono) via `next/font/local`;
  `web/scripts/subset-fonts.sh` rebuilds them (pinned google/fonts SHA). JetBrains Mono has no ₺ glyph (falls back).
- **Guards (all proven red by mutation):** ESLint bans raw colour literals, Tailwind palette classes, energy colours as
  text, `outline-none|outline-hidden|outline-0|[outline:none]`, `dark:`, and `recharts`/`maplibre-gl` outside their
  wrappers; a vitest scan bans colour literals in CSS; `pnpm check:contrast` (70 pairs × 2 themes);
  `pnpm check:i18n-parity` (keys, ICU arguments, `{{}}` ban, identifier keys); `stories-coverage.test.ts` (every
  component has a story and a test).
- **i18n:** per-namespace catalogues `web/messages/{tr,en}/<ns>.json` (app, health, units, common, forms, feedback, table,
  charts, shell, domain, map — 218 keys), statically imported by `messages/index.ts`, typed through next-intl `AppConfig`
  (an unknown key fails `pnpm typecheck`). Locale is the `NEXT_LOCALE` cookie (no URL prefix); `setLocale` server action.
  Numbers/currency always `tr-TR` via `@/lib/format` (`formatNumber/Currency/Quantity/Date/Month/Bytes`); decimals travel
  as strings (`@/lib/decimal`: `compareDecimal`, `fractionToPercent`).
- **Components** (`web/src/components`, all with stories + tests + axe):
  - `ui/`: Button, IconButton, Tooltip, Popover, VisuallyHidden, Field, Input, Textarea, NumberInput, Select, Combobox,
    MultiSelect, Checkbox, Switch, RadioGroup, Slider, SearchInput, DatePicker, DateRangePicker, MonthPicker, FileUpload,
    FileList, FormErrorSummary, Card, StatTile, Badge, StatusBadge, Alert, Toast (`useToast`), LiveAnnouncer
    (`useAnnounce`), EmptyState, Skeleton, ProgressBar, Stepper, Tabs, Accordion, Dialog, Drawer, DropdownMenu,
    Breadcrumb, Pagination, Table parts, DataTable (`getExportRows`) + `@/lib/csv` `toCsv`.
  - `charts/`: LineChart, AreaChart, BarChart, StackedBarChart, ComboChart (Recharts 3), GaugeChart, HeatmapChart,
    Sparkline (hand-built SVG); shared ChartFrame (figure, legend, skeleton, empty state, >6-series alert, LTTB
    downsampling footnote, toggleable exact data table).
  - `shell/`: AppShell, Sidebar, TopNav, TopBar, PageHeader (route-derived breadcrumb), FilterBar, ThemeCustomizer,
    LanguageSwitcher, ThemeToggle, UserMenu, `nav-config.ts` (07 §7 IA as data, longest-match active route).
  - `domain/`: BuildingAnalyzerPicker, PeriodFilterBar (`periodPresets(today)`), MetricCard, ReactiveStatusCard,
    InvoiceLineTable (`InvoiceLineView`), ExportMenu, JobStatusBanner (`JobView`), DataQualityBadge (`DataQuality`).
  - `map/`: Map (MapLibre only with `NEXT_PUBLIC_MAP_TILE_URL` loading within 8 s and WebGL; otherwise the accessible
    coordinate list).
- **UI preferences:** `@/lib/ui-preferences` (cookie `ekokod_ui`: theme, layout, container, sidebar, card, radiusScale) and
  `@/lib/ui-preferences-provider` (`UiPreferencesProvider`, `useUiPreferences`). The root layout renders them as
  `<html data-*>` on the server (verified by an SSR smoke test of the standalone build).
- **App wiring:** `src/app/layout.tsx` → `NextIntlClientProvider` → `UiPreferencesProvider` → `AppProviders` (Tooltip,
  LiveAnnouncer, Toaster). `src/app/(app)/layout.tsx` renders `AppShell` with a placeholder user; `(app)/page.tsx` is the
  F0 health page inside the shell.
- **Gallery and harness:** Storybook 10.6 (`@storybook/nextjs-vite`), globals `theme`/`locale`, story
  `parameters.preferences` for layout variants. Playwright over the static build (`tests/a11y/`): `stories.spec`
  (axe WCAG 2.2 AA, clipping, horizontal scroll, console errors; full theme×locale for each component's first story and
  `open` stories, diagonal for the rest), `keyboard.spec` (every tabbable reached, visible ring; overlays: focus inside,
  arrow-key item ring, Escape restores focus), `touch-targets.spec` (44×44 under `pointer: coarse`),
  `reduced-motion.spec`, `overlay-motion.spec`, `shell.spec` (4 layouts × 375/768/1024/1440 × light/dark). Story tags:
  `open` (play opens an overlay), `modal-popup` (Radix Select/DropdownMenu aria-hide the page), `no-sweep`.
- **CI/Make:** CI `web` job adds `check:contrast`, Storybook build, `test:a11y` (45 min timeout, report artifact on
  failure). `make web-lint` includes contrast; `make web-a11y` exists but is not part of `make ci`.
- **Evidence:** per-task ledger with every mutation proof and deviation:
  `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite/.superpowers/sdd/2026-09-17-f5-design-system/inline-ledger.md`
  (git-ignored). §11 pre-delivery checklist: `design-system/bcem-energy/checklists/f5-predelivery.md`.

### Deviations from the F5 plan (all in the ledger)
- Inline execution: one worktree with `f5/task-N` branches; handoff is this file (not `docs/superpowers/handoffs/…`);
  T8 ran no `make test` and no Go `make lint build` (F5 changed no Go: `git diff --stat 58597ec..HEAD -- '*.go' go.mod go.sum` is empty).
- **jsdom shims in `vitest.setup.ts`:** nwsapi 2.2.27 recurses to a stack overflow on `:modal`/`:fullscreen`/`:open`
  selectors (every Radix popper took ~15 s) → short-circuited; Recharts' ResponsiveContainer measures 0×0 → sized.
  Recharts draw/grow animations own `stroke-dasharray`/bar paths, so path assertions render under reduced motion.
- `toCsv` leaves any Turkish-formatted number (including `-1`) untouched; only non-numeric text starting with
  `= + - @ TAB CR` is prefixed.
- Page-level axe rules (region, landmark-one-main, page-has-heading-one) apply to `Shell/AppShell` only.
- `UiPreferencesProvider` lives in `src/lib/`; the breadcrumb is rendered by `PageHeader`; `boxed` = `max-w-7xl`.
- IconButton `tooltip={false}` for overlay close buttons (a focus tooltip swallowed the first Escape).
- Pre-existing F0 bug fixed: `fetchHealth` now rejects bodies without `checks[]` (another service on :8080 crashed `/`).
- Radix quirks the tests encode: radio arrow keys check only while the key is held; react-day-picker's `thead` is
  `aria-hidden`; Radix Dialog sets no `aria-modal`; Radix Toast announces a copy through its own `role=status`.

## F6 must pick up
- **Branching:** start from `phase/f5-design-system` and merge `phase/f4-billing-engine` (see the prompt).
- **From F5:** replace the `(app)` placeholder user with the session user; role-filter `navigation` (`nav-config.ts`) by
  the 01 §2 matrix; sync `ekokod_ui` to the user profile (the cookie stays the SSR source); generated OpenAPI TypeScript
  client + TanStack Query; the remembered building/analyzer selection store behind `BuildingAnalyzerPicker`; Calendar's
  sidebar placement (Q6); card-grid stagger with `--duration-stagger-step` (D26); add an `e2e` Playwright project under
  `tests/e2e/` next to `a11y` and turn `webServer` into an array (the a11y project serves `storybook-static`); map pages
  set `NEXT_PUBLIC_MAP_TILE_URL` (Q5) or live with the coordinate list; every new page uses `PageHeader`, `FilterBar`,
  `PeriodFilterBar`, `MetricCard`, the chart wrappers, `DataTable` + `ExportMenu`, and marks estimated figures with
  `DataQualityBadge`. New UI plans: read MASTER.md then OVERRIDES.md; add their message namespaces the same way (the
  appendix `docs/rewrite/appendix/i18n-{tr,en}.json` is a source of wording, not importable, D7/Q7).
- **From F4 (HTTP wiring for §6/§7, see the F4 handoff for detail):** handlers, DTOs, RBAC and OpenAPI over `billingsvc`
  and `tariffsvc`; `ComputeError` → HTTP mapping (`tariff_not_found` 422, `no_consumption_data` 422,
  `unresolved_anomaly` 409, `period_not_closed` 409, `billing_parameters_*` 500, `*tariff.ValidationError` 422 with field
  codes, `ErrInvalidRequest` 400, `store.ErrNotFound` 404); `/bills/{id}/pdf`, `/bills/{id}/hourly.xlsx`,
  `/bills/compute` → `billing.generate` with `Force`; the tariff DTO must require `vat_rate` and every tax `rate`.
- **From F1–F3:** F2 R47/R48; **Q4 calendar events and weekend split is an F6 blocker** (the calendar and vacation
  configuration feed the load-profile weekday/weekend split).
- **Later phases:** F8 maps F4's bill DTO → `InvoiceLineView` and owns server-side CSV/Excel/PDF/e-mail export and
  override drift (R114); F9 `plant_production_*` aggregate refresh; F12 public site (locale routing D8, spacious scale,
  glass accents allowed there only); F15 CSP nonces and the supported-browser floor.

## Open questions for the product owner (defaults ship)
- **F5:** Q1 customiser theme colour (default no) · Q2 RTL (not built) · Q3 English UI keeps `1.234,56` (default yes) ·
  **Q4 approve light `primary` = emerald-700 `#047857`** (spec `#059669` fails 4.5:1 on white; kept as `brand`) ·
  Q5 production map tile source · Q6 Calendar's place in the sidebar · Q7 legacy catalogues disagree on 115 keys.
- **Browser floor (for F15/PO):** `light-dark()` needs Chrome/Edge ≥ 123, Firefox ≥ 120, Safari ≥ 17.5.
- **F4 (unchanged):** the real-invoice comparison is OPEN; dated `billing_parameters` defaults (R106, reactive basis,
  exemptions — the real icmal contradicts `reactive_exempt_terms={monomial}`, R118 —, tiering, PTF tolerance,
  `money_rounding_mode=half_up` vs the icmal's half-even tie, R112); R119, R111, R110, R114, R122, R135; OG transformer loss.
- **F1–F3 (unchanged):** Q1 ratio zero-case, Q3 weekend default, **Q4 calendar/weekend split (F6 blocker)**, Q5–Q10;
  F2 device/integration items (see the F4 handoff).
- The spec (09 §F4) says F5 waits for the real-invoice review; the user explicitly ordered F5 to proceed.

## Binding rulings
F1 rulings 1–14, F2 R1–R53, F3 R54–R104, F4 R105–R135 (F4 plan on `phase/f4-billing-engine`), F5 D1–D26
(`docs/superpowers/plans/2026-09-17-f5-design-system.md`), plus the deviations above and in each phase's ledger.

## Process rules that paid off (keep them, inline)
- **Read legacy code and real data before coding domain rules** (F4: four Critical rule errors caught at plan time).
- **Prove every guard red with your own mutation, by script, restoring from a backup** (never `git checkout -- file` or
  `git stash`). F5 mutants exposed three weak tests (announcer regions, a clipped-text mutant on an `<input>`, the
  lint ban missing `outline-0`) and one weak harness relaxation was proven harmless the same way.
- **Look at the UI, not only the tests.** Playwright screenshots in both themes caught a misleading dashed legend for
  bars and a clipped chart label that every automated check passed. An SSR smoke test of the standalone build
  (`node .next/standalone/server.js` + `curl` with cookies) caught a page crash no unit test covered.
- **Systematic debugging over guessing:** a CPU profile of the vitest worker (`test.execArgv: ['--cpu-prof']` with the
  forks pool) found the nwsapi recursion in minutes.
- **Commit as soon as tests are green,** then mutations, then merge `--no-ff`. Keep comments to 1–3 lines saying WHY.
- For money/tenancy code (F6 RBAC): cross-tenant proofs with the other tenant's scope and a positive control.

## Environment — read before running anything
- Prefix every command with `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
- WSL: 12 GB RAM, 4 GB swap. **Another project's Gradle/Kotlin daemons (dolmusum) come and go and reached ~6.4 GB with
  swap nearly full during F5 — never kill them.** Check `free -m` and `ps -eo rss,comm | grep java` before Storybook,
  Playwright or `next build`; run Playwright with `--workers=1` when memory is tight.
  **Run long Playwright/Storybook work in the foreground:** the harness's low-memory guard killed two background sweeps
  in F5, while the same work in foreground chunks (one worker) completed. `dolmusum serve` listens on :8080
  (the default API URL), so the web app's health page sees a foreign JSON when the ekokod API is not running.
- Worktrees on the native filesystem (`/home/personal/...`) only; `/mnt/c` is slow.
- Web commands (in `web/`): `pnpm lint && pnpm typecheck && pnpm check:i18n-parity && pnpm check:contrast`,
  `pnpm test --maxWorkers=2` (~60 s), `NODE_OPTIONS=--max-old-space-size=3072 pnpm storybook:build` (~40 s),
  `SB_PORT=6007 STORIES='UI/Button,Charts/' pnpm test:a11y` (scoped; the full sweep is ~20 min at 2 workers),
  `pnpm build`. Never leave `next dev`, `storybook dev`, `vitest --watch` or a static server running.
- Fonts need fonttools: `python3 -m venv --without-pip ~/.cache/ekokod-fontenv` + `get-pip.py` + `pip install fonttools brotli`
  (system Python has no `ensurepip`); already set up.
- `make check-generate` needs a clean tree for `internal/store/postgres/sqlcgen`, so commit first.
- Commit trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.

## Key paths
```
docs/rewrite/                                                      specification (07 design system, 09 phase plan)
docs/superpowers/plans/2026-09-17-f5-design-system.md              F5 plan (D1–D26, Q1–Q7)
design-system/bcem-energy/{MASTER.md,OVERRIDES.md}                 design system of record
design-system/bcem-energy/checklists/f5-predelivery.md             07 §11 checklist with evidence
web/src/styles/tokens.css, web/src/app/globals.css                 tokens, base layer, reduced motion, focus ring
web/src/components/{ui,charts,shell,domain,map}/                   component library (+ stories, tests)
web/src/lib/{format,decimal,csv,cn,ui-preferences*}.ts(x)          formatting and preferences
web/messages/{tr,en}/*.json, web/messages/index.ts                 catalogues
web/scripts/{check-contrast,check-i18n-parity}.ts, contrast-pairs.ts   CI checks
web/tests/a11y/                                                    Playwright a11y harness
/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite/.superpowers/sdd/2026-09-17-f5-design-system/inline-ledger.md
```

## Verification at the F5 tip
Everything below ran on the merged tip (T1–T7 plus the T8 review fixes), one step at a time:
- `pnpm install --frozen-lockfile`, `pnpm lint` (0 problems), `pnpm typecheck`, `pnpm test --maxWorkers=2`
  (**98 files, 302 tests**), `pnpm check:contrast` (**70 pairs × 2 themes**, lowest generation/background light 3.13),
  `pnpm check:i18n-parity` (**11 namespaces, 218 keys**), `pnpm audit --audit-level=high` (no known vulnerabilities),
  `pnpm storybook:build`: all exit 0.
- **Full a11y sweep: 1,102 Playwright checks over 224 stories, 0 failures.** A two-worker background run was killed by
  the harness's low-memory guard (foreign Gradle daemons ~5–6 GB) after 381 passes, and so was a chunked background run.
  The final evidence therefore ran in the **foreground**, one worker, in chunks: UI A–D 266, UI E–N 162, UI P–V 250,
  Charts 124, Shell 110, Domain/Map/Fixtures 154, shell + reduced-motion + overlay-motion specs 36. Run long Playwright
  work in the foreground the same way.
- `make web-lint web-test web-build web-audit`: exit 0 (`next build` → `/` 1.97 kB, 124 kB first load).
- SSR smoke test (standalone server + `curl` with cookies): `<html lang data-theme data-layout data-container
  data-sidebar data-card style=--radius-scale>` server-rendered from `ekokod_ui`/`NEXT_LOCALE`.
- `git grep` for `#rrggbb` in `web/src` outside `tokens.css` and for emoji: no matches.
  `git diff --stat 58597ec..HEAD -- '*.go' go.mod go.sum`: empty (F5 touched no Go, so no Go gate was re-run).
- No dev server, watcher or static server left running.
