# F5 — Design System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A complete, token-driven, bilingual, accessible component library — primitives, charts, domain components and the application shell — shown in a gallery in both themes and both locales and guarded in CI. No product screens.

**Architecture:** Every colour is one `light-dark()` declaration in `web/src/styles/tokens.css`, published to Tailwind v4 through `@theme static` after wiping Tailwind's default palette. `color-scheme` on `<html data-theme>` picks light, dark or the OS preference with no script. Components live in `web/src/components/{ui,charts,shell,domain,map}` on Radix primitives and read only token utilities; ESLint rejects raw colours, default-palette classes, `outline-none` and `dark:`. Storybook 10 (nextjs-vite) is the gallery. Playwright runs axe, keyboard, clipping, touch-target, reduced-motion and viewport checks over the static Storybook build for every story × {light, dark} × {tr, en}. Vitest (jsdom) covers behaviour.

**Tech Stack (exact, pinned with `pnpm add -E`):** Next.js 15.5.25 · React 19.2.8 · next-intl 4.14.2 · Tailwind CSS 4.3.3 · TypeScript 5.9.3 · Vitest 5.0.0 (vite 8.2.2, jsdom 26) · radix-ui 1.6.7 · lucide-react 1.46.0 · class-variance-authority 0.7.1 · clsx 2.1.1 · tailwind-merge 3.7.0 · cmdk 1.1.1 · react-day-picker 9.14.0 · date-fns 4.4.0 · @tanstack/react-table 8.21.3 · recharts 3.10.1 · maplibre-gl 6.10.0 · storybook / @storybook/nextjs-vite / @storybook/addon-a11y / @storybook/addon-docs 10.6.0 · @playwright/test 1.63.0 · @axe-core/playwright 4.13.0 · axe-core 4.13.0 · @testing-library/user-event 14.6.7 · fontkit 2.0.4 · Node 24.20.0 · pnpm 12.3.4. If a version will not resolve, pick the nearest patch release, write it in the task report and do not change the major.

**Spec:** `docs/rewrite/09-implementation-plan.md` §F5 (scope, acceptance, verification: binding). `docs/rewrite/07-design-system.md` (all of it). `docs/rewrite/03-target-architecture.md` §5. `docs/rewrite/01-project-context.md` §5–§6 (sidebar, disabled entries, customiser). The appendix lives at `docs/rewrite/appendix/`, not at `docs/appendix/` as §F5 says.

---

## Global Constraints (every task's requirements implicitly include this section)

**Authority**
- **The §F5 acceptance criteria beat spec prose. Spec prose beats `MASTER.md`.** Where 07 and the skill's generated `MASTER.md` disagree, `design-system/bcem-energy/OVERRIDES.md` records the resolution (D1). Where a spec value fails a §F5 acceptance criterion, the criterion wins and the value changes (D3). Spec gaps are ruled in the table below. If you find a new gap, stop and report it. Do not improvise.
- **UI work starts with `ui-ux-pro-max`.** Every implementer reads `design-system/bcem-energy/MASTER.md` and then `OVERRIDES.md` before writing a component. For a focused concern, run `python3 "$UIPM/scripts/search.py" "<query>" --domain ux|chart|icons` with `UIPM=/home/personal/.claude/plugins/cache/ui-ux-pro-max-skill/ui-ux-pro-max/2.13.0/.claude/skills/ui-ux-pro-max`. Never pass `--force`.
- **No Go code.** F4 is running in parallel on `phase/f4-billing-engine`. F5 touches only `web/**`, `design-system/**`, `.github/workflows/ci.yml` (the `web` job), `Makefile` (the `web-*` targets), `.gitignore`, `scripts/check-i18n-parity.mjs` (removed, T1) and `docs/superpowers/**`. `HANDOFF_NEXT_SESSION.md` belongs to the F4 track: F5 never edits it.

**Design rules (07, as ruled)**
- **Only token utilities.** The palette comes from `tokens.css` (T1). Tailwind's default colours do not exist (`--color-*: initial`). Colour literals (`#hex`, `rgb(`, `hsl(`, `oklch(` …) may appear only in `web/src/styles/tokens.css`. ESLint enforces this in `src/**/*.{ts,tsx}`, and a vitest scan enforces it for `src/**/*.css` (D5). No `dark:` variant anywhere: dark mode comes from tokens (D4), and a component that needs dark-specific behaviour needs a new token.
- **Status = colour + icon + text.** `StatusBadge`, `Alert`, `Toast` and `DataQualityBadge` take a required text label and render a Lucide icon with `aria-hidden`. No emoji anywhere, including stories.
- **Energy colours (`consumption`, `generation`, `reactive-inductive`, `reactive-capacitive`, `cost`, `revenue`, `forecast`, `brand`) are graphics only.** Use them for lines, bars, swatches and markers, never as text colour (D3). Forecast is always `--color-forecast` and dashed. On `primary-subtle` use only `foreground` or `primary` text (M-1: `foreground-muted`/`foreground-subtle` on `primary-subtle` fail contrast in dark). ESLint bans `text-(brand|consumption|generation|reactive-inductive|reactive-capacitive|cost|revenue|forecast)` (T1 Step 5) so the graphics-only rule is enforced, not just prose.
- **Focus rings are global.** `:focus-visible { outline: 2px solid var(--color-ring); outline-offset: 2px }` lives in `globals.css`. Components never set an `outline-*` class.
- **Touch targets.** Every interactive element gets `pointer-coarse:min-h-11 pointer-coarse:min-w-11`, on itself or on a wrapper marked `data-touch-target`. On a fine pointer the visual minimum is 32 px (D13).
- **Motion.** Use CSS transitions with the duration custom properties from T1 (`duration-(--duration-hover)` and so on). Animate only `transform` and `opacity`, except for hover colour transitions. Exit is faster than entry. No GSAP. Never animate a table or a number. `prefers-reduced-motion` is handled globally in `globals.css`, and Recharts animation is handled through `chartAnimation()` (T5). Overlays (Dialog, Drawer, Popover, Tooltip, picker popovers) use the T1 `@keyframes`/`--animate-*` tokens (I-14): `data-[state=open]:animate-<x>-in data-[state=closed]:animate-<x>-out` — never hand-rolled keyframes, because Radix `Presence` only delays unmount for CSS animations, not transitions.
- **Numbers.** `formatNumber`, `formatCurrency` and `formatQuantity` from `@/lib/format` always use `tr-TR` separators (D9). Tabular numbers use `type-data` or `type-metric` (JetBrains Mono, ligatures off, M-3). API decimals arrive as strings and go to `Intl` as strings (D11). `formatDate(iso, locale)` / `formatMonth(iso, locale)` (`Europe/Istanbul`) are the only date formatters (M-19); next-intl's `format.number` is never used (it would apply `en` separators and break D9).
- **Component tests scope role queries with `within(container)`** when the role can come from a provider (`LiveAnnouncerProvider`, `Toaster`) that every `renderWithProviders` render includes (I-9).
- **Logical properties.** Use `ms-/me-/ps-/pe-/start-/end-/text-start`, never `ml-/mr-/pl-/pr-/left-/right-/text-left`. This keeps RTL cheap later (Q2). It is a review item, not a lint rule.
- **Every UI string lives in `web/messages/{tr,en}/<namespace>.json`.** Each task owns its namespaces (ownership table). A key matches `^[a-zA-Z][a-zA-Z0-9]*$`. Interpolation is ICU (`{count}`), never `{{count}}`. Take Turkish and English values from `docs/rewrite/appendix/i18n-{tr,en}.json` when a legacy string with the same meaning exists (D7).

**Component contract (every file in `src/components/**`)**
- Use kebab-case files and named exports. Every component file `x.tsx` has `x.test.tsx` and `x.stories.tsx` beside it (guarded by `stories-coverage.test.ts`, T2). Internal helper files start with `_` (tests required, stories not). Tests use `renderWithProviders`, `@testing-library/user-event` and `expectNoAxeViolations` (T2); every component test file has one axe test.
- Every component has a `'use client'` directive only if it uses state, effects, context or Radix. Pure presentational components stay server-compatible.
- Props use the exact names and types in this plan. Add optional props only if the task needs them, and list them in the report.
- Stories: `title` is `UI/<Name>`, `Charts/<Name>`, `Shell/<Name>`, `Domain/<Name>` or `Map/<Name>`. Every story that matters has a state: default, disabled, error, loading, empty. A component with a text label also has a `LongTurkishLabel` story (for example "Reaktif Endüktif Tüketim Oranı Eşik Değeri"). Intrinsic strings come from catalogues. Sample domain data may be literal Turkish.

**Resources (the machine is shared with F4)**
- **Command prefix:** `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"`.
- **At most 2 F5 implementers run at once, and never two heavy builds concurrently** (I-2). Builds run only in native worktrees under `/home/personal/`, never on `/mnt/c`.
- **Vitest runs with at most 2 workers:** `pnpm test --maxWorkers=2` (the config also sets it after T2). Never use watch mode.
- **Heavy steps are machine-wide serialised and capped at 3 GB (I-2).** Wrap `pnpm build`, `pnpm storybook:build` and `pnpm test:a11y` in `flock /home/personal/.ekokod-f5-heavy.lock <cmd>` so only one heavy step runs machine-wide. Run with `NODE_OPTIONS=--max-old-space-size=3072`; if a run OOMs at 3072, raise it only for that run while still holding the lock. Run each once per task gate, not per step.
- **Each worktree exports its own Storybook port (I-1):** `SB_PORT=60<NN>` (T1→6001, T2→6002, T3→6003, T4→6004, T5→6005, T6→6006, T7→6007; T8 runs alone on the merged phase tip and keeps the default 6007). `playwright.config.ts` reads `const port = Number(process.env.SB_PORT ?? 6007)` for both `webServer.command` and `baseURL`.
- **Never leave a server running.** No `next dev`, `storybook dev`, `vitest --watch` or `playwright --ui`. Playwright's `webServer` starts and stops the static server itself. Before you report, `pgrep -fa "ekokod-f5.*(next dev|storybook dev|vitest)|http.server 60[0-9][0-9]"` must print nothing (M-20: scoped to this project so it doesn't match other work on the shared machine).
- **Scoped a11y runs.** Per task: `SB_PORT=60<NN> STORIES='UI/Input,UI/Select' pnpm test:a11y` (comma-separated title prefixes). The full sweep runs only in T8 and in CI.
- **Worktree setup:**
  ```bash
  git worktree add -b f5/task-N /home/personal/ekokod-f5-tN phase/f5-design-system
  git config --global --add safe.directory /home/personal/ekokod-f5-tN
  cd /home/personal/ekokod-f5-tN/web && pnpm install --frozen-lockfile --config.package-import-method=hardlink
  pnpm exec playwright install chromium   # once per machine; reuses ~/.cache/ms-playwright if the revision matches
  ```
  Merge with `git merge --no-ff` into `phase/f5-design-system`. If `pnpm-lock.yaml` conflicts, take the phase branch's lockfile, run `pnpm install`, and commit the result.

**Task gate (every task, in its worktree's `web/`)**
```bash
pnpm lint && pnpm typecheck && pnpm check:i18n-parity && pnpm check:contrast   # all scripts added by T1 (I-3)
pnpm test --maxWorkers=2
flock /home/personal/.ekokod-f5-heavy.lock env NODE_OPTIONS=--max-old-space-size=3072 pnpm storybook:build   # script from T1
SB_PORT=60<NN> flock /home/personal/.ekokod-f5-heavy.lock env STORIES='<the task's title prefixes>' pnpm test:a11y   # script from T1
flock /home/personal/.ekokod-f5-heavy.lock env NODE_OPTIONS=--max-old-space-size=3072 pnpm build   # T1, T6, T8 only
```
- **Every guard is proven to fail** (each guard step names a mutation; the report records the red output, then the revert). **Commits** are conventional, one logical change each, ending with the `<TRAILERS>` the controller supplies at dispatch.

## Pre-flight (controller, no agent)

- [ ] **P1.** `git -C /home/personal/ekokod-f5-phase log --oneline -1` shows `58597ec` (or this plan's commit on top of it). `ls web/src` matches the F0 scaffold: `app/ components/health-status.tsx i18n/request.ts lib/api.ts`.
- [ ] **P2.** `ls docs/rewrite/appendix/i18n-{tr,en}.json` both exist; `python3 --version` (≥3.10) and `python3 -m venv --help` work (T1 fonts); `ls ~/.cache/ms-playwright` is noted.
- [ ] **P3.** Create the SDD ledger at `.superpowers/sdd/2026-09-17-f5-design-system/progress.md`, plus `global-constraints.md` (a verbatim copy of this section, the waves and ownership) for every implementer and reviewer.

## Spec gaps and rulings

| Id | Question | Ruling | Why | Cost if wrong |
|---|---|---|---|---|
| D1 | The skill's `MASTER.md` (glassmorphism, slate `#1E293B`, Fira Code/Fira Sans, GSAP `back.out` stagger, `--space-xs..3xl`) contradicts 07 §2–§4, §8, §10. | Persist `MASTER.md` verbatim with the exact §0 command. Add a hand-written `design-system/bcem-energy/OVERRIDES.md` that names each conflict and the binding 07 rule. Where they conflict, 07 plus this plan win. `MASTER.md` is never edited and never `--force`d. | §0 says 07 is "the starting point produced by that skill", and 07 documents deliberate overrides (typography). The criterion requires a skill-produced file. | One markdown file. |
| D2 | Slug: product renamed "ekokod", but the criterion names `design-system/bcem-energy/`. | `-p "BCEM Energy"` → `design-system/bcem-energy/`. | Acceptance wording is binding. | Directory rename. |
| D3 | 07 §2.2 light values fail 07 §2.4 and §9: white on `#059669` is 3.77:1, `foreground-subtle` `#6B7C76` on sunken is 3.91, dark `foreground-subtle` `#7A8F88` on raised is 4.48, `border-strong` is 1.68 as an input border, `generation` as text is 3.30. | Light: `primary` `#047857`, `primary-hover` `#065F46`, `ring` `#047857`, `foreground-subtle` `#5A6B65`. Dark: `foreground-subtle` `#82978F`. New tokens: `brand` (`#059669` / `#34D399`, graphics and logo only), `border-control` (`#7A8E87` / `#607F73`, form-control borders, 3:1), `on-danger` (`#FFFFFF` / `#1A0505`), `overlay` (scrim). Energy colours are graphics-only, checked at 3:1. All 65 declared pairs pass (lowest: warning on warning-subtle, 4.51). | "Contrast check passes for every token pair" is an acceptance criterion. The emerald-600 identity survives as `--color-brand`. | Six hex values. Q4 goes to the PO. |
| D4 | Two token blocks (`:root` / `[data-theme=dark]`) cannot express "follow the OS" without script or duplication. | One declaration per token: `--color-x: light-dark(LIGHT, DARK)`. `:root { color-scheme: light }`, `[data-theme=dark] { color-scheme: dark }`, `[data-theme=system] { color-scheme: light dark }`. Values are verbatim from 07 except D3. Shadows use `light-dark(rgb(...), transparent)`. | No flash, no inline script (CSP-friendly, F15), and system mode for free. The browser floor becomes Chrome 123 / Firefox 120 / Safari 17.5 (Tailwind v4 already needs 111/128/16.4). | Rewriting tokens.css into two blocks: mechanical. |
| D5 | The criterion says "zero raw hex in `components/`". | Wider scope: raw colour literals (hex, rgb, hsl, oklch, lab, lch, hwb) and Tailwind default-palette class names are banned in all of `src/**/*.{ts,tsx}` except `src/styles/**`, and in every `src/**/*.css` except `tokens.css`. The palette is also wiped, so a stray `bg-red-500` generates nothing. | A hex in `app/` is the same defect. | None. |
| D6 | "Self-host, subset to Latin + Latin Extended-A." | Subset the upstream `google/fonts` variable TTFs (pinned commit) with fonttools into one woff2 per family. Keep U+0020–007E, 00A0–017F, general punctuation, ₺ U+20BA, € U+20AC, subscripts U+2080–2089, arrows U+2190–2193, − U+2212, ≤≥. Features `kern,liga,calt,tnum,lnum,case,ccmp,locl,mark,mkmk`. Load through `next/font/local`. Commit the OFL licences. | One file per family with preload and a size-adjusted fallback (no CLS). `locl` keeps Turkish i/İ correct. `₺` and `tCO₂e` are units the spec requires. | Re-run one script. |
| D7 | §F5 says seed `next-intl` from the appendix, but the appendix is not importable: 2,746 tr / 2,751 en keys with 55/60 unmatched, i18next `{{count}}` syntax, keys with spaces and case-duplicates ("Carbon Footprint" / "CARBON FOOTPRINT"), and one array value. | No wholesale import. Catalogues are split per namespace (`messages/{tr,en}/<ns>.json`, statically imported by `messages/index.ts`). F5 seeds only the namespaces it renders, taking values from the appendix wherever a legacy string with that meaning exists. Later phases add their namespaces the same way. The parity check gains ICU-argument parity, a `{{` ban and a key-pattern rule. | A wholesale import would fail the parity check on day one and ship 2,700 unused keys under unusable names. | One import script later. The appendix remains the feature inventory. |
| D8 | Locale switching mechanism. | `NEXT_LOCALE` cookie read in `src/i18n/request.ts`. No URL prefix for the product. `setLocale` server action plus `router.refresh()`. `timeZone: 'Europe/Istanbul'`. Public-site routing is F12's call. | The product is authenticated (not indexed). The cookie keeps every F6 route unprefixed. | F12 adds middleware. |
| D9 | 07 says "Turkish number formatting" without mentioning the English UI. | Numbers and currency always use `tr-TR` (`1.234,56`, `₺1.234,56`) in both locales. Dates and labels follow the locale. | Invoices and tariffs are Turkish legal amounts. Two users of one company must never see different separators on the same figure. | One constant (Q3). |
| D10 | Which single chart library? | Recharts 3.10.1 (SVG). `GaugeChart` and `HeatmapChart` are hand-built SVG in the same theme module. ESLint `no-restricted-imports` forbids `recharts` outside `src/components/charts/**`. | SVG resolves `var(--color-*)` and `light-dark()` natively in both themes. Canvas libraries (ECharts) need a JS colour bridge and re-render on theme change. Recharts' `accessibilityLayer` gives keyboard tooltips. | Wrapper internals only. Pages never import the library. |
| D11 | Chart data vs. "no float in money/energy paths". | Chart props accept `number \| string \| null`. `Number()` is applied only to plot geometry. Data tables and tooltips format the original string with `formatNumber`. Plotting is not a calculation path. | Exact figures where people read them, geometry where they don't. | None. |
| D12 | "Every chart has a data table" vs `Sparkline`. | `Sparkline` is exempt. It is a supplementary glyph inside `MetricCard`, whose figure is shown as text. It is `role="img"` with an `aria-label` giving first, last, min and max. | A table per tile repeats the tile. | Add a toggle. |
| D13 | "Touch targets ≥ 44×44" vs a density-8 dashboard. | 44×44 applies under `pointer: coarse` (Tailwind `pointer-coarse:`). On a fine pointer, controls are at least 32 px tall (≥ WCAG 2.2 2.5.8's 24 px). Checked by `touch-targets.spec.ts` in a `hasTouch`/`isMobile` context. | These are touch targets. A 44 px mouse-only table row halves data density. | Class change in `buttonVariants` and inputs. |
| D14 | "Every form field has a visible label" vs the top-bar search. | Every field takes a required `label` rendered visibly. The only exception is `SearchInput` with `labelVisibility="hidden"` in the `TopBar`: leading search icon plus `aria-label`. Placeholders are never labels. | Standard global-search affordance. The exception is bounded to one call site. | One prop. |
| D15 | Customiser scope (01 §6: light/dark, LTR/RTL, theme colour, vertical/horizontal, boxed/full, sidebar collapse, card border/shadow, radius slider). | Built: theme `light\|dark\|system`, layout `vertical\|horizontal`, container `full\|boxed`, sidebar `expanded\|collapsed`, card `border\|shadow`, radius scale 0.5–1.5. Not built: RTL (no RTL locale, Q2) and theme colour (collides with brand/status/energy semantics, Q1). Persisted in cookie `ekokod_ui`, applied server-side as `<html data-*>`. F6 syncs it to the user profile. | "Nothing is lost" where it is coherent. The two omissions contradict 07 §1–§2. Cookie instead of `localStorage` so SSR renders the right theme and layout without a flash; F6 syncs to the profile (M-9). | Two presets later. |
| D16 | Gallery and a11y harness. | Storybook 10.6 `@storybook/nextjs-vite`. Playwright runs against `storybook-static` served by `python3 -m http.server` as its `webServer`. Globals `theme` and `locale` go in the URL. | Named in §F5 verification. A static build means no dev server. | — |
| D17 | Where domain components live and what data they take. | `src/components/domain/**`. They are presentational: typed view-model props in, callbacks out, no fetching, no generated client (the OpenAPI client is F6). View-model types are owned by F5 (e.g., `InvoiceLineView`). F8 maps F4's API shapes to them. | F4 is in flight. Coupling to its unmerged types would block F5. | A mapper in F8. |
| D18 | Sidebar placement of Water/Gas (07 §7: under Data Analysis; 01 §5: "Consumption → Water"). Calendar is not listed in either. | Follow 07 §7: Water, Gas and EV Drivers are disabled children of Data Analysis. No Calendar entry in F5 (Q6). The nav config is data (`nav-config.ts`), so F6 edits one array. | 07 is the layout authority. | One array edit. |
| D19 | Date values in pickers. | Calendar dates are ISO strings: `'YYYY-MM-DD'`, `{ from, to }`, `'YYYY-MM'`. Weeks start Monday. The date-fns `tr` / `enUS` locale follows next-intl. Never `Date` in props. | A `Date` crosses timezones and shifts by a day in Istanbul vs UTC. | Prop types. |
| D20 | Latest majors `react-day-picker` 10 and `@tanstack/react-table` 9 are recent. | Pin 9.14.0 and 8.21.3. Revisit in F15. | Well-known, stable APIs. Implementers must not guess new APIs. | Upgrade later. |
| D21 | "More than 6 series means a different visualisation." | A wrapper given more than 6 series renders `Alert tone="warning"` (`charts.tooManySeries`) instead of a chart. Tested. | Makes the rule observable. | — |
| D22 | CSV from `ExportMenu` / `Table`. | `toCsv` uses `;` as the delimiter, CRLF line endings and a UTF-8 BOM. Numbers use `tr-TR` formatting. `toCsv` prefixes `'` to text cells that start with `=`, `+`, `-`, `@`, TAB or CR (formula-injection guard), but not to cells already produced by `formatNumber` (M-12). | Turkish Excel parses `;` with a decimal comma. | One constant. |
| D23 | Map tile source default (03 §5.2: configurable, degrade to a coordinate list). | `NEXT_PUBLIC_MAP_TILE_URL` (style JSON or raster template). If unset, or the source fails to load within 8 s, or WebGL is unavailable, render the accessible coordinate list. No public OSM default (Q5). `MapMarker` is the marker type, not a component; markers render inside `Map` and as rows in `MapMarkerList` (M-11). | OSM's tile policy forbids unagreed heavy use. Offline installs have no tiles. | Set an env var. |
| D24 | Horizontal layout under `lg`. | Below 1024 px the horizontal layout collapses to the same overlay drawer as vertical. At ≥1024 px it renders a top nav whose groups are `DropdownMenu`s. | One mobile pattern. | — |
| D25 | Pagination's page-size control (I-6): `Popover` is a T2 file but `Select` is T3, and `Pagination` is T4. | The page-size control is a labelled native `<select>` with token classes (`border-border-control bg-surface rounded-md h-8 pointer-coarse:min-h-11`), not the T3 `Select`. | Avoids a T4→T3 dependency for one control. | Swap the markup later. |
| D26 | 07 §8 card-grid stagger (300–450 ms, 60 ms apart) has no token or ruling (M-10). | Add `--duration-stagger-step: 60ms` (T1). Ruling: `animation-delay: calc(var(--i) * var(--duration-stagger-step))` on page card grids — deferred to F6, since F5 ships no page that uses a card grid. | F5 has the token ready without inventing an unused component behaviour now. | Apply the class in F6. |

## Open questions (non-blocking; defaults above apply)

- **Q1** Should the customiser offer a theme colour? Default: no. Green is the fixed identity (07 §1) and other hues collide with the status and energy semantics (D15).
- **Q2** Is RTL needed? There is no RTL locale. Default: not built. Logical properties keep the cost low.
- **Q3** Should an English-UI user see `1,234.56`? Default: no, always `1.234,56` (D9).
- **Q4** Approve darkening light `primary` to emerald-700 `#047857` so button labels pass 4.5:1. The spec's `#059669` stays as `--color-brand` (D3).
- **Q5** What is the production map tile source for cloud installs (D23)?
- **Q6** Where does Calendar (01 §7.18) sit in the sidebar (D18)?
- **Q7** The legacy catalogues disagree on 115 keys (D7). Each later phase should reconcile its own namespace against the appendix.

## Execution waves (≤ 2 concurrent implementers)

A task starts only when every dependency is **merged** into `phase/f5-design-system` and the merged tip passes the gate.

| Wave | Tasks | Depends on | Starts |
|---|---|---|---|
| A | **T1** Foundation · **T2** Gallery, harness, Button | — (T1 owns every dependency install, `package.json` script and `pnpm-workspace.yaml` edit, I-3; T2 writes code from the start on a speculative base but runs `git merge phase/f5-design-system` as soon as T1 merges, and before its first `pnpm test`; T2 edits `messages/*/common.json` only after that merge) | immediately, both |
| B | **T3** Form controls · **T4** Containers, overlays, feedback, Table | T1, T2 | after A |
| C | **T5** Charts · **T6** Shell, preferences, customiser | T5: T4 · T6: T3, T4 | after B |
| D | **T7** Domain components + Map | T3, T4, T5 (`MetricCard` stories embed `Sparkline`) | takes the slot T5 frees |
| E | **T8** Acceptance sweep, §11 checklist, handoff | all | last |

## File ownership (parallel tasks never edit the same file)

| Task | Owns | Message namespaces |
|---|---|---|
| T1 | `design-system/**`, `web/src/styles/**`, `web/src/app/{globals.css,layout.tsx,page.tsx,_components/health-status*}`, `web/src/lib/{cn,format}.ts(+tests)`, `web/src/i18n/{request.ts,locale.ts,app-config.d.ts}`, `web/messages/**` (creates every namespace file, empty `{}` where not T1's), `web/scripts/**`, `web/eslint.config.mjs`, `web/tsconfig.json`, `scripts/check-i18n-parity.mjs` (delete), `web/package.json` (**all** deps and **all** scripts, I-3), `web/pnpm-workspace.yaml` (I-4) | `app`, `health`, `units` |
| T2 | `web/.storybook/**`, `web/playwright.config.ts`, `web/tests/a11y/{helpers.ts,fixtures,stories,keyboard,touch-targets,reduced-motion}.spec.ts`, `web/src/test/**`, `web/vitest.{config,setup}.ts`, `web/src/components/providers.tsx`, `web/src/components/stories-coverage.test.ts`, `web/src/components/ui/{button,icon-button,tooltip,visually-hidden,popover}.*` (I-6), `.github/workflows/ci.yml`, `Makefile`, `.gitignore`, `.dockerignore` (M-13) | `common` |
| T3 | `web/src/components/ui/{field,input,textarea,number-input,select,combobox,multi-select,checkbox,radio-group,switch,slider,search-input,date-picker,date-range-picker,month-picker,file-upload,file-list,form-error-summary}.*` | `forms` |
| T4 | `web/src/components/ui/{card,stat-tile,badge,status-badge,alert,toast,live-announcer,empty-state,skeleton,progress-bar,stepper,tabs,accordion,dialog,drawer,dropdown-menu,breadcrumb,pagination,table,data-table}.*` (no `popover`, moved to T2 per I-6), `web/src/lib/csv.ts(+test)`, `web/tests/a11y/overlay-motion.spec.ts`, `providers.tsx` (adds `Toaster` and `LiveAnnouncerProvider` only) | `feedback`, `table` |
| T5 | `web/src/components/charts/**` | `charts` |
| T6 | `web/src/components/shell/**`, `web/src/lib/ui-preferences.ts(+test)`, `web/src/i18n/actions.ts`, `web/src/app/layout.tsx` (html attributes + mounts `UiPreferencesProvider`, I-15), `web/src/app/(app)/**` (moves `page.tsx` and `_components/health-status*` in, I-19), `web/tests/a11y/shell.spec.ts`, `web/src/test/render.tsx` (adds `preferences?` option only, I-15), `.storybook/preview.tsx` (wraps `UiPreferencesProvider`, `theme` from globals, only, I-15) | `shell` |
| T7 | `web/src/components/domain/**`, `web/src/components/map/**` | `domain`, `map` |
| T8 | `design-system/bcem-energy/checklists/f5-predelivery.md`, `docs/superpowers/handoffs/2026-09-f5-design-system.md`, fixes routed back to owners' files by the controller | — |

## Task 1: Foundation — MASTER.md, tokens, fonts, guards, catalogues

**Interfaces — Produces (later tasks rely on these exact names; Consumes: nothing new):**
- Tailwind utilities: `bg|text|border|fill|stroke|ring-{primary,primary-hover,primary-subtle,on-primary,brand,background,surface,surface-raised,surface-sunken,foreground,foreground-muted,foreground-subtle,border,border-strong,border-control,ring,success,success-subtle,warning,warning-subtle,danger,danger-subtle,on-danger,info,info-subtle,consumption,generation,reactive-inductive,reactive-capacitive,cost,revenue,forecast,overlay,card-edge}`. Also `font-heading|font-sans|font-mono`, `shadow-sm|md|lg`, `rounded-sm|md|lg|full`, the composite typography utilities `type-display|h1|h2|h3|body|body-lg|small|caption|metric|data`, and the overlay-motion utilities `animate-{fade-in,fade-out,dialog-in,dialog-out,drawer-in-start,drawer-out-start,drawer-in-end,drawer-out-end,drawer-in-bottom,drawer-out-bottom,popover-in,popover-out}` (I-14).
- CSS custom properties: `--duration-hover` 150ms, `--duration-popover` 180ms, `--duration-dialog-in` 250ms, `--duration-dialog-out` 180ms, `--duration-tab` 200ms, `--duration-chart` 400ms, `--duration-stagger-step` 60ms (D26, M-10), `--radius-scale` (default 1).
- `cn(...inputs: ClassValue[]): string` from `@/lib/cn`.
- `@/lib/format`: `NUMBER_LOCALE = 'tr-TR'`; `type Decimalish = string | number | null | undefined`; `formatNumber(v: Decimalish, o?: { minFractionDigits?: number; maxFractionDigits?: number }): string` (`null` → `'—'`; default precision — string input: `minimumFractionDigits=0`, `maximumFractionDigits` = the string's own fraction digits capped at 20; number input: `maximumFractionDigits=3`, I-13); `formatCurrency(v: Decimalish, o?): string`; `type Unit = 'kWh' | 'MWh' | 'kW' | 'kVArh' | 'TRY' | 'tCO2e' | 'kgCO2e' | 'percent'`; `unitSymbol(u: Unit): string`; `formatQuantity(v: Decimalish, u: Unit, o?): string`; `formatDate(iso: string, locale: Locale): string`; `formatMonth(iso: string, locale: Locale): string` (both `Europe/Istanbul`, M-19).
- `@/i18n/locale`: `locales`, `type Locale = 'tr' | 'en'`, `defaultLocale`, `LOCALE_COOKIE = 'NEXT_LOCALE'`, `resolveLocale(v?: string): Locale`.
- `messages` from `web/messages/index.ts`: `{ tr: Messages; en: Messages }`, with `Messages` registered in next-intl's `AppConfig`, so an unknown key fails `pnpm typecheck`.
- `@/styles/fonts`: `fontVariables: string`.

- [ ] **Step 1: Persist the master design system (skill; acceptance criterion 1).**
```bash
UIPM=/home/personal/.claude/plugins/cache/ui-ux-pro-max-skill/ui-ux-pro-max/2.13.0/.claude/skills/ui-ux-pro-max
python3 "$UIPM/scripts/search.py" "energy monitoring sustainability analytics dashboard" \
  --design-system --density 8 --motion 4 --variance 5 -p "BCEM Energy" --persist --output-dir /home/personal/ekokod-f5-t1
test -f /home/personal/ekokod-f5-t1/design-system/bcem-energy/MASTER.md && head -12 /home/personal/ekokod-f5-t1/design-system/bcem-energy/MASTER.md
```
Write `design-system/bcem-energy/OVERRIDES.md` with one row per conflict: style (glassmorphism → flat surfaces with border/shadow in the dashboard; glass only on the public site, 07 §10), colours (→ 07 §2 as ruled by D3/D4), typography (→ Lexend / Source Sans 3 / JetBrains Mono, 07 §3), spacing names (→ Tailwind's 4 px scale = 07 §4 `--space-N` = `p-N`), shadows (→ 07 §4), motion (→ CSS transitions per 07 §8; no GSAP, no `back.out` overshoot), page pattern ("Real-Time/Operations Landing" → F12 only). Its first line: "Binding. Where MASTER.md disagrees, this file and docs/rewrite/07-design-system.md win."

- [ ] **Step 2: Write the guard tests first (red).**

| Test file › name | Assertion |
|---|---|
| `scripts/lib/contrast.test.ts` › `contrastRatio matches WCAG reference values` | `contrastRatio('#FFFFFF','#000000')` = 21; `contrastRatio('#767676','#FFFFFF')` rounds to 4.54 |
| › `parseTokens reads light and dark from light-dark()` | Given `--color-primary: light-dark(#047857, #34D399);` → `{ primary: { light: '#047857', dark: '#34D399' } }` |
| › `evaluate fails the spec's original primary pair` | Tokens with `primary` light `#059669` and `on-primary` `#FFFFFF` → one failure `on-primary/primary light 3.77 < 4.5` |
| › `evaluate reports a colour token no pair covers` | Adding `--color-orphan` → the problem list includes `uncovered token: orphan` |
| › `the committed tokens pass` | `evaluate(parseTokens(read('src/styles/tokens.css')), pairs, exempt)` → `[]` |
| `scripts/lib/i18n.test.ts` › `missing keys are reported per direction` | tr `{a:{x:'1'}}`, en `{a:{}}` → `['en/a.json missing a.x']` |
| › `ICU arguments must match` | tr `'{count} kayıt'`, en `'{total} records'` → one problem naming both argument sets |
| › `i18next double braces are rejected` | `'{{count}} yeni'` → problem `a.x uses {{…}}` |
| › `keys must be identifiers` | key `'Carbon Footprint'` → problem `invalid key` |
| › `a namespace file missing in one locale is reported` | tr has `forms.json`, en does not → problem |
| `src/styles/raw-color-lint.test.ts` (ESLint API `lintText`, `filePath: 'src/components/ui/x.tsx'`, 30 s timeout) › one test per case | Must error: `className="bg-[#fff]"`, `` `text-[rgb(0_0_0)]` ``, `className="bg-red-500"`, `className="hover:text-neutral-900"`, `className="text-consumption"` (M-1), `className="outline-none"`, `className="dark:bg-surface"`, `import { LineChart } from 'recharts'`. Must pass: `href="#main"`, `href="#add-row"`, `className="bg-primary text-on-primary"`, `className="fill-consumption"` |
| `src/styles/raw-color-css.test.ts` › `no CSS file except tokens.css holds a colour literal` | Walk `src/**/*.css`, skip `src/styles/tokens.css`, `RAW_COLOR` has zero matches |
| `src/styles/fonts.test.ts` › `each family covers Turkish and is subset` | For each woff2 (fontkit): `hasGlyphForCodePoint` is true for every char of `ğĞşŞıİçÇöÖüÜ0123456789.,%`, false for `Ж` (U+0416); file ≤ 150 000 bytes; `variationAxes.wght` exists. The test logs ₺/₂ coverage for the report |
| `src/lib/format.test.ts` › `formats with Turkish separators` | `formatNumber('1234.567890', { maxFractionDigits: 2 })` = `'1.234,57'` |
| › `keeps full precision for decimal strings` | `formatNumber('12345678901234567890.123456', { maxFractionDigits: 6 })` = `'12.345.678.901.234.567.890,123456'` |
| › `default precision follows the string input, not Intl's default 3` | `formatNumber('1234.567891')` = `'1.234,567891'` (I-13) |
| › `null is an em dash, never zero` | `formatNumber(null)` = `'—'`; `formatCurrency(undefined)` = `'—'` |
| › `currency and units` | `formatCurrency('1234.56')` = `'₺1.234,56'`; `formatQuantity('1234.5','tCO2e')` = `'1.234,5 tCO₂e'`; `formatQuantity('12.5','percent')` = `'%12,5'` |
| › `dates and months follow the locale, in Istanbul time` | `formatDate('2026-09-17','tr')` / `formatMonth('2026-09-01','en')` (M-19) |
| `src/i18n/locale.test.ts` › `unknown or missing cookie falls back to tr` | `resolveLocale(undefined)` = `'tr'`, `resolveLocale('de')` = `'tr'`, `resolveLocale('en')` = `'en'` |

`scripts/lib/*.test.ts`, `src/styles/raw-color-lint.test.ts` and `src/styles/fonts.test.ts` each start with `// @vitest-environment node` (M-2: avoids jsdom realm/`TextEncoder` mismatches for the ESLint API and fontkit Buffers). Run `pnpm test --maxWorkers=2 scripts src/styles src/lib src/i18n`. Expected: FAIL (modules missing).

- [ ] **Step 3: Tokens and globals (full code: exact values).** `web/src/styles/tokens.css`:
```css
/* 07-design-system.md §2–§4, §8 as ruled by plan D3/D4. The ONLY file in web/src allowed colour literals (D5). */
:root { color-scheme: light; --radius-scale: 1;
  --duration-hover: 150ms; --duration-popover: 180ms; --duration-dialog-in: 250ms; --duration-dialog-out: 180ms;
  --duration-tab: 200ms; --duration-chart: 400ms; --duration-stagger-step: 60ms; }
:root[data-theme='dark'] { color-scheme: dark; }
:root[data-theme='system'] { color-scheme: light dark; }

@theme static {
  --color-*: initial; --shadow-*: initial; --radius-*: initial;
  --color-primary: light-dark(#047857, #34D399);
  --color-primary-hover: light-dark(#065F46, #6EE7B7);
  --color-primary-subtle: light-dark(#D1FAE5, #064E3B);
  --color-on-primary: light-dark(#FFFFFF, #04211A);
  --color-brand: light-dark(#059669, #34D399);
  --color-background: light-dark(#F6FAF8, #0B1512);
  --color-surface: light-dark(#FFFFFF, #12201C);
  --color-surface-raised: light-dark(#FFFFFF, #172823);
  --color-surface-sunken: light-dark(#ECF3EF, #0A100E);
  --color-foreground: light-dark(#0B2A22, #E8F2EE);
  --color-foreground-muted: light-dark(#4B5D57, #9CB3AB);
  --color-foreground-subtle: light-dark(#5A6B65, #82978F);
  --color-border: light-dark(#DCE7E2, #23372F);
  --color-border-strong: light-dark(#B9CCC4, #354E44);
  --color-border-control: light-dark(#7A8E87, #607F73);
  --color-ring: light-dark(#047857, #34D399);
  --color-success: light-dark(#15803D, #4ADE80);   --color-success-subtle: light-dark(#DCFCE7, #052E16);
  --color-warning: light-dark(#B45309, #FBBF24);   --color-warning-subtle: light-dark(#FEF3C7, #3B2A06);
  --color-danger: light-dark(#B91C1C, #F87171);    --color-danger-subtle: light-dark(#FEE2E2, #3B0D0D);
  --color-on-danger: light-dark(#FFFFFF, #1A0505);
  --color-info: light-dark(#0369A1, #38BDF8);      --color-info-subtle: light-dark(#E0F2FE, #06283B);
  --color-consumption: light-dark(#0E7490, #22D3EE);
  --color-generation: light-dark(#16A34A, #4ADE80);
  --color-reactive-inductive: light-dark(#A16207, #FBBF24);
  --color-reactive-capacitive: light-dark(#7E22CE, #C084FC);
  --color-cost: light-dark(#B91C1C, #F87171);
  --color-revenue: light-dark(#15803D, #4ADE80);
  --color-forecast: light-dark(#64748B, #94A3B8);
  --color-overlay: light-dark(rgb(11 42 34 / 0.45), rgb(0 0 0 / 0.6));
  --color-card-edge: light-dark(transparent, #354E44); /* shadow-mode card edge; shadows vanish in dark */
  --radius-sm: calc(4px * var(--radius-scale)); --radius-md: calc(8px * var(--radius-scale));
  --radius-lg: calc(12px * var(--radius-scale)); --radius-full: 9999px;
  --shadow-sm: 0 1px 2px light-dark(rgb(11 42 34 / 0.06), transparent);
  --shadow-md: 0 4px 12px light-dark(rgb(11 42 34 / 0.08), transparent);
  --shadow-lg: 0 12px 32px light-dark(rgb(11 42 34 / 0.1), transparent);
  /* I-14: keyframes for overlays. Presence only delays unmount for CSS animations, not transitions. */
  --animate-fade-in: fade-in var(--duration-popover) ease-out;
  --animate-fade-out: fade-out var(--duration-popover) ease-in;
  --animate-dialog-in: dialog-in var(--duration-dialog-in) ease-out;
  --animate-dialog-out: dialog-out var(--duration-dialog-out) ease-in;
  --animate-drawer-in-start: drawer-in-start var(--duration-dialog-in) ease-out;
  --animate-drawer-out-start: drawer-out-start var(--duration-dialog-out) ease-in;
  --animate-drawer-in-end: drawer-in-end var(--duration-dialog-in) ease-out;
  --animate-drawer-out-end: drawer-out-end var(--duration-dialog-out) ease-in;
  --animate-drawer-in-bottom: drawer-in-bottom var(--duration-dialog-in) ease-out;
  --animate-drawer-out-bottom: drawer-out-bottom var(--duration-dialog-out) ease-in;
  --animate-popover-in: popover-in var(--duration-popover) ease-out;
  --animate-popover-out: popover-out var(--duration-popover) ease-in;
}
@keyframes fade-in { from { opacity: 0 } to { opacity: 1 } }
@keyframes fade-out { from { opacity: 1 } to { opacity: 0 } }
@keyframes dialog-in { from { opacity: 0; transform: scale(.96) } to { opacity: 1; transform: scale(1) } }
@keyframes dialog-out { from { opacity: 1; transform: scale(1) } to { opacity: 0; transform: scale(.96) } }
@keyframes drawer-in-start { from { transform: translateX(-100%) } to { transform: translateX(0) } }
@keyframes drawer-out-start { from { transform: translateX(0) } to { transform: translateX(-100%) } }
@keyframes drawer-in-end { from { transform: translateX(100%) } to { transform: translateX(0) } }
@keyframes drawer-out-end { from { transform: translateX(0) } to { transform: translateX(100%) } }
@keyframes drawer-in-bottom { from { transform: translateY(100%) } to { transform: translateY(0) } }
@keyframes drawer-out-bottom { from { transform: translateY(0) } to { transform: translateY(100%) } }
@keyframes popover-in { from { opacity: 0; transform: scale(.98) } to { opacity: 1; transform: scale(1) } }
@keyframes popover-out { from { opacity: 1; transform: scale(1) } to { opacity: 0; transform: scale(.98) } }
@theme inline {
  --font-heading: var(--font-lexend), ui-sans-serif, system-ui, sans-serif;
  --font-sans: var(--font-source-sans-3), ui-sans-serif, system-ui, sans-serif;
  --font-mono: var(--font-jetbrains-mono), ui-monospace, monospace;
}
@utility type-display { font-family: var(--font-heading); font-size: 32px; line-height: 40px; font-weight: 600; } @utility type-h1 { font-family: var(--font-heading); font-size: 24px; line-height: 32px; font-weight: 600; }
@utility type-h2 { font-family: var(--font-heading); font-size: 20px; line-height: 28px; font-weight: 600; } @utility type-h3 { font-family: var(--font-heading); font-size: 16px; line-height: 24px; font-weight: 600; }
@utility type-body { font-family: var(--font-sans); font-size: 14px; line-height: 20px; font-weight: 400; } @utility type-body-lg { font-family: var(--font-sans); font-size: 16px; line-height: 24px; font-weight: 400; }
@utility type-small { font-family: var(--font-sans); font-size: 13px; line-height: 18px; font-weight: 400; } @utility type-caption { font-family: var(--font-sans); font-size: 12px; line-height: 16px; font-weight: 500; }
@utility type-metric { font-family: var(--font-mono); font-size: 28px; line-height: 32px; font-weight: 600; font-variant-numeric: tabular-nums; font-variant-ligatures: none; } @utility type-data { font-family: var(--font-mono); font-size: 13px; line-height: 18px; font-weight: 400; font-variant-numeric: tabular-nums; font-variant-ligatures: none; }
```
`font-variant-ligatures: none` on `type-data`/`type-metric` (M-3) stops JetBrains Mono's `calt`/`liga` turning `>=`, `->`, `--` into ligatures in data cells; the subset in Step 6 keeps the features for code contexts elsewhere.
`web/src/app/globals.css`:
```css
@import 'tailwindcss';
@import '../styles/tokens.css';
@layer base {
  html { background: var(--color-background); color: var(--color-foreground); }
  body { @apply type-body antialiased; min-height: 100dvh; }
  :focus-visible { outline: 2px solid var(--color-ring); outline-offset: 2px; }
  button:not(:disabled), [role='button']:not([aria-disabled='true']), a[href], summary, select, label[for],
  [role='tab'], [role='menuitem'], [role='option'] { cursor: pointer; }
  :disabled, [aria-disabled='true'] { cursor: not-allowed; }
  @media (prefers-reduced-motion: reduce) {
    *, *::before, *::after { animation-duration: 0.01ms !important; animation-iteration-count: 1 !important;
      transition-duration: 0.01ms !important; scroll-behavior: auto !important; }
  }
}
```
If Tailwind 4.3.3 rejects `@apply` of a `@utility` inside `@layer base`, inline the `type-body` declarations. Do not change anything else.

- [ ] **Step 4: Contrast and parity scripts.** Node 24 runs `.ts` natively. Set `"allowImportingTsExtensions": true` in `tsconfig.json` and import with `.ts` extensions. `scripts/lib/contrast.ts` exports `contrastRatio`, `parseTokens(css): Record<string, { light: string; dark: string }>` (regex `/--color-([a-z-]+):\s*light-dark\(\s*(#[0-9A-Fa-f]{6})\s*,\s*(#[0-9A-Fa-f]{6})\s*\)/g`; also collects every `--color-<name>:` declared) and `evaluate(tokens, pairs, exempt): string[]`. `scripts/contrast-pairs.ts`:
```ts
export type Pair = readonly [fg: string, bg: string, min: number];
const TEXT = 4.5, UI = 3;
const surfaces = ['background', 'surface', 'surface-raised', 'surface-sunken'];
export const pairs: Pair[] = [
  ...['foreground', 'foreground-muted', 'foreground-subtle'].flatMap((f) => surfaces.map((b) => [f, b, TEXT] as const)),
  ['on-primary', 'primary', TEXT], ['on-primary', 'primary-hover', TEXT], ['primary', 'background', TEXT],
  ['primary', 'surface', TEXT], ['primary', 'primary-subtle', TEXT], ['foreground', 'primary-subtle', TEXT],
  ['on-danger', 'danger', TEXT],
  ...['success', 'warning', 'danger', 'info'].flatMap((s) => [
    [s, `${s}-subtle`, TEXT], [s, 'surface', TEXT], [s, 'background', TEXT], ['foreground', `${s}-subtle`, TEXT]] as const),
  ...['ring', 'border-control', 'brand', 'consumption', 'generation', 'reactive-inductive', 'reactive-capacitive',
    'cost', 'revenue', 'forecast'].flatMap((g) => ['background', 'surface', 'surface-raised'].map((b) => [g, b, UI] as const)),
];
export const exempt: Record<string, string> = {
  border: 'decorative divider; never a component boundary (WCAG 1.4.11 n/a)',
  'border-strong': 'decorative divider; controls use border-control',
  overlay: 'translucent scrim; content above it is on surface tokens',
  'card-edge': 'decorative card edge in shadow mode (transparent in light)',
};
```
`scripts/check-contrast.ts` prints one line per pair and theme (`PASS on-primary/primary light 5.48 ≥ 4.5`) and exits 1 on any problem. Move `scripts/check-i18n-parity.mjs` to `web/scripts/check-i18n-parity.ts` on top of `lib/i18n.ts` (`flatten`, `icuArgs`, `checkCatalogues(tr: Record<ns, object>, en: Record<ns, object>): string[]`). Read `messages/{tr,en}/*.json` with `fs`. `package.json` scripts: `"check:contrast": "node scripts/check-contrast.ts"`, `"check:i18n-parity": "node scripts/check-i18n-parity.ts"`.

- [ ] **Step 5: ESLint rules (full code).** Append to `web/eslint.config.mjs`:
```js
const RAW_COLOR = String.raw`(?<![\w&-])#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})(?![\w-])|\b(?:rgba?|hsla?|oklch|oklab|lab|lch|hwb)\(`;
const PALETTE = String.raw`(?:^|[\s:])(?:bg|text|border|ring|fill|stroke|outline|decoration|divide|accent|caret|placeholder|from|via|to|shadow)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|black|white)(?:-\d{2,3})?(?:\/\d+)?(?=\s|$)`;
const ENERGY_TEXT = String.raw`(?:^|[\s:])text-(?:brand|consumption|generation|reactive-inductive|reactive-capacitive|cost|revenue|forecast)(?=\s|$)`;
const FORBIDDEN = String.raw`(?:^|[\s:])(?:outline-none|outline-hidden)(?=\s|$)|(?:^|\s)dark:`;
const ban = (re, message) => [
  { selector: `Literal[value=/${re}/]`, message },
  { selector: `TemplateElement[value.raw=/${re}/]`, message },
];
config.push(
  { files: ['src/**/*.{ts,tsx}'], ignores: ['src/styles/**'], rules: {
    'no-restricted-syntax': ['error',
      ...ban(RAW_COLOR, 'Raw colour literal: use a design token (07 §2.4, plan D5).'),
      ...ban(PALETTE, 'Tailwind default palette: use a token utility (plan D5).'),
      ...ban(ENERGY_TEXT, 'Energy colours are graphics only, never text colour (plan D3, M-1).'),
      ...ban(FORBIDDEN, 'outline-none / dark: are forbidden: focus rings are global, dark mode is token-driven (plan D4).')],
    'no-restricted-imports': ['error', { paths: [
      { name: 'recharts', message: 'Use src/components/charts (07 §5, plan D10).' },
      { name: 'maplibre-gl', message: 'Use src/components/map (plan D23).' }] }],
  } },
  { files: ['src/components/charts/**'], rules: { 'no-restricted-imports': 'off' } },
  { files: ['src/components/map/**'], rules: { 'no-restricted-imports': 'off' } },
);
```
(The esquery regex literal cannot contain `/`. The `(?:\/\d+)?` is inside a `String.raw`, which the selector receives as `\/`. If esquery rejects it, drop the opacity suffix group and record that.) Exempt the test file's own fixtures with `ignores: ['src/styles/**']`, because the lint test lives there. Also add `storybook-static/**`, `playwright-report/**` and `test-results/**` to `ignores`.

- [ ] **Step 6: Fonts (D6).**
```bash
python3 -m venv /home/personal/.cache/ekokod-fontenv && /home/personal/.cache/ekokod-fontenv/bin/pip install fonttools brotli
```
`web/scripts/subset-fonts.sh` (mode 100755, shellcheck-clean) downloads `ofl/lexend/Lexend[wght].ttf`, `ofl/sourcesans3/SourceSans3[wght].ttf` and `ofl/jetbrainsmono/JetBrainsMono[wght].ttf` from `https://raw.githubusercontent.com/google/fonts/<SHA>/`. Resolve the SHA once with `git ls-remote https://github.com/google/fonts HEAD` and hard-code it. The script runs `pyftsubset` with `--unicodes=U+0020-007E,U+00A0-017F,U+2013-2014,U+2018-201E,U+2022,U+2026,U+2030,U+2032-2033,U+2039-203A,U+20AC,U+20BA,U+2080-2089,U+2122,U+2190-2193,U+2212,U+2264-2265 --layout-features=kern,liga,calt,tnum,lnum,case,ccmp,locl,mark,mkmk --flavor=woff2` into `src/styles/fonts/{lexend,source-sans-3,jetbrains-mono}.woff2`, and copies each `OFL.txt`. `src/styles/fonts.ts` uses `next/font/local` with `variable: '--font-lexend' | '--font-source-sans-3' | '--font-jetbrains-mono'`, `display: 'swap'` and weight ranges `'100 900'`, `'200 900'`, `'100 800'`. It exports `fontVariables`.

- [ ] **Step 7: Catalogues and locale.** Create every namespace file in the ownership table under `messages/tr/` and `messages/en/`. Move the current `app` and `health` objects into `app.json` and `health.json`. Fill `units.json` (`kWh, MWh, kW, kVArh, TRY, tCO2e, kgCO2e, percent` → display names such as `"kilovat saat"` / `"kilowatt-hour"`, used for `aria-label`s). Leave all other namespaces as `{}`. `messages/index.ts` statically imports all 22 files and exports `messages` and `type Messages = typeof messages.tr`. `src/i18n/app-config.d.ts`:
```ts
import type { Messages } from '../../messages';
import type { Locale } from './locale';
declare module 'next-intl' { interface AppConfig { Locale: Locale; Messages: Messages } }
```
Update `request.ts` to cookie-based per D8: `resolveLocale((await cookies()).get(LOCALE_COOKIE)?.value)`, `messages[locale]`, `timeZone: 'Europe/Istanbul'`. Update `layout.tsx`: `<html lang={locale} data-theme="system" className={fontVariables}>` and `<body>` with no colour classes. Move `health-status.tsx` (and its test) to `web/src/app/_components/health-status.tsx` (I-19: outside `src/components/**` so T2's `stories-coverage.test.ts` guard, which only scans that tree, never fires on this F0 diagnostic file; T6 wires it into `(app)/page.tsx`). Replace every default-palette class in `page.tsx` and `health-status.tsx` with tokens (`text-foreground-muted`, `border-danger bg-danger-subtle text-danger`, `border-border`). `health-status.test.tsx` imports `messages` from the `web/messages` package.

- [ ] **Step 8: All F5 dependencies and scripts (I-3: T1 owns every install and every `package.json` script, so T2's later merge never touches `package.json`/`pnpm-lock.yaml`/`pnpm-workspace.yaml`).**
```bash
pnpm add -E radix-ui@1.6.7 lucide-react@1.46.0 class-variance-authority@0.7.1 clsx@2.1.1 tailwind-merge@3.7.0 \
  cmdk@1.1.1 react-day-picker@9.14.0 date-fns@4.4.0 @tanstack/react-table@8.21.3 recharts@3.10.1 \
  maplibre-gl@6.10.0 react-is@19.2.8   # react-is: explicit recharts peer (M-5)
pnpm add -DE fontkit@2.0.4 vite@8.2.2 storybook@10.6.0 @storybook/nextjs-vite@10.6.0 \
  @storybook/addon-a11y@10.6.0 @storybook/addon-docs@10.6.0 @playwright/test@1.63.0 \
  @axe-core/playwright@4.13.0 axe-core@4.13.0 @testing-library/user-event@14.6.7   # vite pinned exact so vitest and Storybook share one Vite (M-5)
  # plus @types/fontkit if pnpm typecheck needs it
pnpm install
```
Read the install's "Ignored build scripts" line and add each entry (expected: `esbuild: true`, pulled in by `storybook`) to `allowBuilds` in `web/pnpm-workspace.yaml`, in the comment style of the existing entries (I-4). Acceptance: the install output lists no ignored builds, and a clean `pnpm install --frozen-lockfile` succeeds in a fresh worktree. Add `package.json` scripts `"storybook:build": "storybook build --quiet -o storybook-static"` and `"test:a11y": "playwright test tests/a11y/"` (T2 consumes both; I-3). Delete `scripts/check-i18n-parity.mjs`.

- [ ] **Step 9: Green, then prove each guard red.** Run the task gate (no Storybook yet — deps for T2's harness exist from Step 8 but `storybook:build`/`test:a11y` are proven by T2), `pnpm audit --audit-level=high` (I-5: only T1's gate and T8 run this, since only T1 installs deps; fix a high advisory with a `pnpm-workspace.yaml` `overrides` entry and a justification comment, as for `postcss` — never downgrade the gate), and `flock /home/personal/.ekokod-f5-heavy.lock env NODE_OPTIONS=--max-old-space-size=3072 pnpm build`. Then run these mutations one at a time, record the output, and revert each:

| Guard | Mutation | Must see |
|---|---|---|
| lint | `className="bg-red-500"` in `health-status.tsx` | `pnpm lint` error "Tailwind default palette" |
| contrast | `--color-primary: light-dark(#059669, …)` | `check:contrast` exits 1, `on-primary/primary light 3.77` |
| parity | Delete one key from `en/health.json` | `check:i18n-parity` exits 1, naming the key |
| typed keys | `t('nope')` in `page.tsx` | `pnpm typecheck` error |
| CSS scan | `color: #fff` in `globals.css` | `raw-color-css.test.ts` fails |

- [ ] **Step 10: Commit.** `feat(f5): task-1 — MASTER.md, tokens, fonts, contrast/lint/i18n guards` `<TRAILERS>`. The report includes the `check:contrast` summary line, the ₺/₂ glyph coverage and the `next build` route table.

## Task 2: Gallery, test harness and the canonical Button

**Interfaces**
- Consumes (from T1): token utilities, `cn`, `messages`, `fontVariables`, `globals.css`, and every devDependency (Storybook, Playwright, axe, fontkit — installed by T1 Step 8, I-3). T2 drafts code from the start against the speculative dependency set, but its first `pnpm test`/`pnpm lint`/a11y gate run only after `git merge phase/f5-design-system` brings T1's merged tip in (I-3). Before that merge, T2 does not run the harness against real dependencies.
- Produces:
  - `renderWithProviders(ui: ReactElement, o?: { locale?: Locale }): RenderResult & { user: UserEvent }` (`@/test/render`). It wraps `NextIntlClientProvider` (`timeZone: 'Europe/Istanbul'`, `onError` rethrows) plus `AppProviders`.
  - `expectNoAxeViolations(el: Element): Promise<void>` (`@/test/axe`; axe-core with `color-contrast` and `region` disabled).
  - `setMatchMedia(matches: (query: string) => boolean): void` (`@/test/match-media`).
  - `setMockPathname(path: string): void`, `mockRouter: { push, replace, refresh, back, prefetch }` (all `vi.fn()`) (`@/test/navigation`; backs the `vi.mock('next/navigation', …)` in `vitest.setup.ts`, I-8).
  - `AppProviders({ children })` (`@/components/providers`, `'use client'`; T2 adds `Tooltip.Provider delayDuration={300}`).
  - `Button`, `buttonVariants`, `ButtonProps` (code below); `IconButton({ label: string; icon: LucideIcon; variant?; size?; ...button })`; `Tooltip({ content: ReactNode; side?: 'top'|'right'|'bottom'|'left'; children: ReactElement })`; `VisuallyHidden`; `Popover({ trigger: ReactElement; children; align?; side? })` (Radix, `animate-popover-in`/`-out`, 180 ms; moved here from T4 per I-6 since T2 already owns the Tooltip popper motion and both T3 and T4 consume it).
  - Storybook globals `theme: 'light'|'dark'` and `locale: 'tr'|'en'`. Story URL: `iframe.html?id=<id>&viewMode=story&globals=theme:<t>;locale:<l>`.
  - `tests/a11y/helpers.ts`: `loadStories(): { id: string; title: string; name: string; tags: string[] }[]` (from `storybook-static/index.json`; `type: 'story'` entries minus tag `no-sweep`, filtered by env `STORIES` title prefixes), `gotoStory(page, id, { theme, locale })` (navigates, then waits until `window.__STORYBOOK_PREVIEW__.currentRender.phase === 'completed'` so `play` functions have finished; verify the property in 10.6 and record it), `THEMES`, `LOCALES`, `expectNoHorizontalScroll(page)`, `expectNoClippedText(page)`, `tabbables(page, scope?: Locator)` (I-7: scoped to the overlay when one is open).

- [ ] **Step 1: Config (deps and scripts come from T1 Step 8, I-3 — nothing to `pnpm add` here).** Add `web/storybook-static/`, `web/playwright-report/` and `web/test-results/` to `.gitignore`, and the same three paths to `.dockerignore` (M-13, keeps Storybook/Playwright output out of the Docker build context). `vitest.config.ts`: `test.maxWorkers: 2` and `test.exclude: ['tests/**', 'node_modules/**', 'storybook-static/**']`. `vitest.setup.ts`: `ResizeObserver` stub; `Element.prototype.{scrollIntoView,hasPointerCapture,releasePointerCapture}` stubs; `window.matchMedia` default (all false) installed through `setMatchMedia`; `vi.mock('next/navigation', …)` with `usePathname` returning a mutable `mockPathname` (default `'/'`) and `useRouter` returning `mockRouter` (I-8, backs `@storybook/nextjs-vite`'s equivalent mock in Storybook and lets `app-shell.test`/`language-switcher.test` render `useRouter()`/`usePathname()` without throwing).

- [ ] **Step 2: `.storybook/main.ts`** with framework `@storybook/nextjs-vite`, stories `../src/components/**/*.stories.tsx`, addons a11y and docs, and `core.disableTelemetry`. **`.storybook/preview.tsx`** (full):
```tsx
import type { Decorator, Preview } from '@storybook/nextjs-vite';
import { NextIntlClientProvider } from 'next-intl';
import '../src/app/globals.css';
import { messages } from '../messages';
import { AppProviders } from '../src/components/providers';
import { fontVariables } from '../src/styles/fonts';

const withAppContext: Decorator = (Story, { globals }) => {
  const theme = globals.theme === 'dark' ? 'dark' : 'light';
  const locale = globals.locale === 'en' ? 'en' : 'tr';
  const html = document.documentElement; // synchronous: axe must never see a stale theme
  html.dataset.theme = theme; html.lang = locale; html.classList.add(...fontVariables.split(' '));
  return (
    <NextIntlClientProvider locale={locale} messages={messages[locale]} timeZone="Europe/Istanbul"
      onError={(error) => { throw error; }}>
      <AppProviders><Story /></AppProviders>
    </NextIntlClientProvider>
  );
};

const preview: Preview = {
  decorators: [withAppContext],
  initialGlobals: { theme: 'light', locale: 'tr' },
  globalTypes: { theme: { toolbar: { title: 'Theme', items: ['light', 'dark'] } },
    locale: { toolbar: { title: 'Locale', items: ['tr', 'en'] } } },
  parameters: { layout: 'padded', nextjs: { appDirectory: true }, a11y: { test: 'error' } },
};
export default preview;
```
`nextjs.appDirectory: true` (I-8) is required for `@storybook/nextjs-vite` to mock `next/navigation` hooks; without it every `Shell/*` story throws "invariant expected app router to be mounted". T6 stories additionally set `parameters.nextjs.navigation.pathname` per story.

- [ ] **Step 3: Button tests first (red).** `button.test.tsx`:

| Test | Assertion |
|---|---|
| `renders a native button with type=button` | `getByRole('button', { name: 'Kaydet' })` has `type="button"` |
| `loading keeps focus, is busy and swallows clicks` | After focus and click: `onClick` not called, `aria-busy="true"`, `aria-disabled="true"`, still `toHaveFocus()`, `getByText(messages.tr.common.loading)` in the DOM |
| `disabled does not fire onClick` | `onClick` not called |
| `icons are decorative` | Every `svg` has `aria-hidden="true"` |
| `asChild renders the link with button styling` | `getByRole('link')` class contains `bg-primary` |
| `every variant and size has no axe violations` | Loop 4 × 3 → `expectNoAxeViolations` |

`icon-button.test.tsx`: `label is the accessible name` (`getByRole('button', { name: 'Bildirimler' })`); `tooltip shows the label on keyboard focus` (`user.tab()` → `findByRole('tooltip')` has text `Bildirimler`); axe.

- [ ] **Step 4: Canonical implementation (full code; T3–T7 copy this shape).** `src/components/ui/button.tsx`:
```tsx
'use client';
import { cva, type VariantProps } from 'class-variance-authority';
import { Loader2, type LucideIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { Slot } from 'radix-ui';
import type { ComponentPropsWithRef, MouseEvent, ReactNode } from 'react';
import { cn } from '@/lib/cn';

export const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 rounded-md font-sans font-semibold whitespace-nowrap select-none ' +
    'transition-colors duration-(--duration-hover) ease-out pointer-coarse:min-h-11 pointer-coarse:min-w-11 ' +
    'disabled:opacity-60 aria-disabled:opacity-60',
  {
    variants: {
      variant: {
        primary: 'bg-primary text-on-primary hover:bg-primary-hover',
        secondary: 'border border-border-control bg-surface text-foreground hover:bg-surface-sunken',
        ghost: 'text-foreground hover:bg-surface-sunken',
        danger: 'bg-danger text-on-danger hover:brightness-95',
      },
      size: { sm: 'h-8 px-3 type-small', md: 'h-9 px-4 type-body', lg: 'h-11 px-5 type-body-lg' },
    },
    defaultVariants: { variant: 'primary', size: 'md' },
  },
);

export type ButtonProps = ComponentPropsWithRef<'button'> & VariantProps<typeof buttonVariants> & {
  children: ReactNode;
  loading?: boolean;
  iconStart?: LucideIcon;
  iconEnd?: LucideIcon;
  /** Renders the single child (e.g. next/link) with button styling; icons and loading are ignored. */
  asChild?: boolean;
};

export function Button({ variant, size, loading = false, iconStart: IconStart, iconEnd: IconEnd, asChild = false,
  className, children, type = 'button', onClick, ...props }: ButtonProps) {
  const t = useTranslations('common');
  const classes = cn(buttonVariants({ variant, size }), className);
  if (asChild) return <Slot.Root className={classes} {...props}>{children}</Slot.Root>;
  const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
    if (loading) { event.preventDefault(); return; }
    onClick?.(event);
  };
  return (
    <button type={type} className={classes} aria-busy={loading || undefined}
      aria-disabled={loading || undefined} onClick={handleClick} {...props}>
      {loading ? <Loader2 aria-hidden className="size-4 animate-spin" />
        : IconStart ? <IconStart aria-hidden className="size-4" /> : null}
      <span>{children}</span>
      {loading ? <span className="sr-only">{t('loading')}</span> : null}
      {IconEnd && !loading ? <IconEnd aria-hidden className="size-4" /> : null}
    </button>
  );
}
```
`button.stories.tsx`: `title: 'UI/Button'` and stories `Variants` (all 4), `Sizes`, `Loading`, `Disabled`, `WithIcons`, `AsLink`, `LongTurkishLabel`. Labels come from `useTranslations('common')` inside `render`. `common.json` (tr/en, from the appendix `common.*` where it exists): `loading`, `save`, `cancel`, `close`, `apply`, `clear`, `search`, `add`, `edit`, `delete`, `actions`, `notifications`, `more`.

- [ ] **Step 5: Playwright harness.** `playwright.config.ts` (I-1, M-8): `testDir: 'tests'`, `workers: 2`, `fullyParallel: true`, `reporter: 'list'`, `projects: [{ name: 'a11y', testMatch: 'a11y/**' }]`, `const port = Number(process.env.SB_PORT ?? 6007)`, `use.baseURL: \`http://127.0.0.1:${port}\``, viewport 1280×800, Chromium project, `webServer: { command: \`python3 -m http.server ${port} --bind 127.0.0.1 --directory storybook-static\`, url: \`http://127.0.0.1:${port}/index.json\`, reuseExistingServer: false }`. F6 adds an `e2e` project under `tests/e2e/` and turns `webServer` into an array (handoff note, M-8). Specs (all under `tests/a11y/`):
  - `stories.spec.ts` (I-18: full matrix only where it earns its cost): for each component's **first** story and every story tagged `open`, run THEMES × LOCALES (2×2); every other story runs only the diagonal, `light/tr` and `dark/en`. That still proves every component in light/dark and tr/en. For each run: collect console errors and `pageerror`; `gotoStory`; `#storybook-root` visible; `body` has no class `sb-show-errordisplay`. Then `AxeBuilder.withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa']).exclude('#storybook-docs')`, with `.disableRules(['region','landmark-one-main','page-has-heading-one'])` unless the title starts with `Shell/`. Violations mapped to `id: targets` equal `[]`. Then `expectNoHorizontalScroll` (`scrollWidth <= innerWidth`), `expectNoClippedText` (no element under the root whose `scrollWidth > clientWidth + 1` while `overflow-x` is `hidden|clip` and `text-overflow` is not `ellipsis`), and errors equal `[]`.
  - `keyboard.spec.ts` (light/tr; I-7, rewritten so portalled overlays don't fail every `open`-tagged story): the walk scope is `[aria-modal="true"]` when one exists on the page, otherwise `#storybook-root` plus any `[data-radix-popper-content-wrapper]`. Press Tab repeatedly until focus leaves the scope or repeats. `tabbables(page, scope)` is computed over the same scope, and every element from it was reached. Every focused element has computed `outline-style !== 'none'` and `outline-width >= 2px`, and a non-empty bounding box. Stories whose `play` opens an overlay carry tag `open`; for those, keyboard.spec additionally asserts focus starts inside the overlay and that `Escape` returns focus to the opener. A T2 red-proof: add `tests/a11y/fixtures/dialog-fixture.stories.tsx` (a bare `@radix-ui/react-dialog` modal, tag `open`, no dependency on T4's `Dialog`) and prove it passes before T4 exists, so the harness logic is proven independently of the real component.
  - `touch-targets.spec.ts` (`hasTouch: true, isMobile: true`, 375×812): call `el.focus()` on each tabbable before measuring it (M-6: reveals the `sr-only` skip link, which is 1×1 until focused), or skip `[data-skip-link]` and assert its focused size separately. Every tabbable, or its closest `[data-touch-target]` or `label`, has a bounding box ≥ 44×44.
  - `reduced-motion.spec.ts`: `UI/Button` computed `transition-duration` is `0.15s`. With `page.emulateMedia({ reducedMotion: 'reduce' })` it is ≤ `0.001s`. `UI/IconButton`: after focusing, the tooltip content's `animation-duration` is ≤ `0.001s` under reduce.

- [ ] **Step 6: Gallery guard.** `src/components/stories-coverage.test.ts` › `every component file has a story and a test`: for every `src/components/**/*.tsx` not matching `*.test.tsx`, `*.stories.tsx`, `_*`, `providers.tsx`, the sibling `.stories.tsx` and `.test.tsx` exist. `health-status.tsx` lives at `src/app/_components/health-status.tsx` (T1, I-19), outside this glob, so it never needs an exclusion. Mutation: delete `button.stories.tsx` → red.

- [ ] **Step 7: CI and Makefile.** In the `web` job after `pnpm check:i18n-parity`, add `pnpm check:contrast`. Set `timeout-minutes: 45` on the `web` job (I-18: the full sweep, at roughly 300 stories with the reduced matrix, plus keyboard/touch/shell specs, comfortably exceeds the default). After `pnpm build`, add `pnpm exec playwright install --with-deps chromium`, `NODE_OPTIONS=--max-old-space-size=3072 pnpm storybook:build` (I-2), `pnpm test:a11y`, and `actions/upload-artifact@v4` of `web/playwright-report` `if: failure()`. Makefile: `web-lint` gains `pnpm check:contrast`. New target `web-a11y` runs `cd web && pnpm storybook:build && pnpm test:a11y`, added to `.PHONY` only — **not** to `ci` (M-13: `web-a11y` is slow and memory-heavy; a local `make ci`, including F4's, must not pay for it. CI still runs the full sweep through the workflow step above).

- [ ] **Step 8: Gate** (after merging the phase tip containing T1, and before this is run at all, per Wave A): `SB_PORT=6002 STORIES='UI/Button,UI/IconButton,UI/Tooltip,UI/VisuallyHidden' pnpm test:a11y` (M-16: the original list omitted `Tooltip` and `VisuallyHidden`). Prove the harness red: temporarily add `text-foreground-subtle bg-surface-sunken opacity-40` to the `Variants` story wrapper → axe `color-contrast` fails; temporarily add `style={{ outline: 'none' }}` to `Button` → `keyboard.spec` fails. Revert both. Commit `feat(f5): task-2 — Storybook gallery, Playwright a11y harness, Button/IconButton/Tooltip/Popover` `<TRAILERS>`.

## Task 3: Form controls

**Interfaces** — Consumes: T1 tokens, `cn`, and `@/lib/format`; T2 `Button`, `IconButton`, `Tooltip`, `Popover` (I-6: `Combobox`, `DatePicker`, `DateRangePicker` and `MonthPicker` popovers all use T2's `Popover`), test helpers. Produces (all under `src/components/ui/`; every field-like control takes the shared `FieldProps`):
```ts
// field.tsx — label/description/error wiring used by every control below
export type FieldProps = { label: string; description?: string; error?: string; required?: boolean; id?: string; disabled?: boolean };
export function Field(props: FieldProps & { labelVisibility?: 'visible' | 'hidden'; children: (ids: {
  controlId: string; describedBy: string | undefined; invalid: boolean }) => ReactNode }): ReactElement;
export type Option = { value: string; label: string; disabled?: boolean };
```
| Component | Props (in addition to `FieldProps`) |
|---|---|
| `Input` | `ComponentPropsWithRef<'input'>` minus `id` |
| `Textarea` | `ComponentPropsWithRef<'textarea'>`, `rows` default 4 |
| `NumberInput` | `value: string \| null; onValueChange(v: string \| null): void; min?: string; max?: string; fractionDigits?: number; unit?: Unit` — displays `formatNumber`, parses `1.234,56` → `'1234.56'` without floats |
| `Select` | `options: Option[]; value: string \| null; onValueChange(v: string): void; placeholder?: string; disabled?: boolean` (Radix Select) |
| `Combobox` | `options: Option[]; value: string \| null; onValueChange(v: string \| null): void; searchPlaceholder: string; emptyText: string` (cmdk in T2's `Popover`, `role="combobox"`; filters with `s.toLocaleLowerCase('tr').replaceAll('ı','i')` on both sides via cmdk `shouldFilter={false}`, I-11) |
| `MultiSelect` | `options: Option[]; value: string[]; onValueChange(v: string[]): void; searchPlaceholder: string; emptyText: string; maxChips?: number` (trigger shows chips as token-styled `span`s, not `Badge`, I-6; then `forms.moreSelected {count}`) |
| `Checkbox` | `checked: boolean \| 'indeterminate'; onCheckedChange(v: boolean): void` (label to the right, row is `data-touch-target`) |
| `Switch` | `checked: boolean; onCheckedChange(v: boolean): void` |
| `RadioGroup` | `options: Option[]; value: string; onValueChange(v: string): void; orientation?: 'horizontal' \| 'vertical'` (`label` renders as `<legend>` of a fieldset) |
| `Slider` | `value: number; onValueChange(v: number): void; min: number; max: number; step: number; formatValue?: (v: number) => string` |
| `SearchInput` | `value: string; onValueChange(v: string): void; labelVisibility?: 'visible' \| 'hidden'` (D14) |
| `DatePicker` | `value: string \| null /* YYYY-MM-DD */; onValueChange(v: string \| null): void; min?: string; max?: string` |
| `DateRangePicker` | `value: { from: string; to: string } \| null; onValueChange(v): void; presets?: { id: string; label: string; range: { from: string; to: string } }[]; min?; max?` |
| `MonthPicker` | `value: string \| null /* YYYY-MM */; onValueChange(v: string \| null): void; min?: string; max?: string` (12-month grid, year arrows) |
| `FileUpload` | `accept: string; maxSizeBytes: number; multiple?: boolean; onFilesSelected(files: File[]): void` (drop zone plus button; rejects oversize or wrong type with `error` text) |
| `FileList` | `files: { id: string; name: string; sizeBytes: number; status: 'uploading' \| 'done' \| 'error'; progress?: number; error?: string }[]; onRemove(id: string): void` (no `FieldProps`; renders its own `role="progressbar"` bar per uploading row — it does not use `ProgressBar`, I-6) |
| `FormErrorSummary` | `errors: { fieldId: string; message: string }[]; title: string` — `role="alert"`, `tabIndex={-1}`, focuses itself when `errors` goes from empty to non-empty, and each item links `#fieldId` (no `FieldProps`) |

Rules: the label is always rendered (visually hidden only for `SearchInput` with `hidden`). The error sits under the field with `id` wired through `aria-describedby`, and `aria-invalid` is set. The error text starts with an `AlertCircle` icon. Controls use `border-border-control`, `bg-surface` and `rounded-md`, 36 px tall with `pointer-coarse:min-h-11`. Picker popovers use `duration-(--duration-popover)`. Dates follow D19 (`date-fns` `tr`/`enUS` chosen from `useLocale()`, `weekStartsOn: 1`).

- [ ] **Step 1: Tests first (red).** One file per component, each with an axe test, plus:

| Test | Assertion |
|---|---|
| `field.test` › `label, description and error are wired` | `getByLabelText('E-posta')` is the input; `aria-describedby` contains both the description and error ids; `aria-invalid="true"` |
| `number-input.test` › `parses Turkish input to a decimal string` | Typing `1.234,56` then blurring calls `onValueChange('1234.56')` |
| › `keeps precision beyond float` | Typing `12345678901234567,891` → `'12345678901234567.891'` |
| › `empty is null, not zero` | Clearing → `onValueChange(null)` |
| › `rejects letters` | Typing `12a` → the value stays `'12'`; `error` text is not required from the component |
| `select.test` › `keyboard selects an option` | Focus the trigger, `{ArrowDown}{ArrowDown}{Enter}` → `onValueChange('b')` |
| `combobox.test` › `filters by Turkish text case-insensitively, including dotless ı` | Typing `istanbul`, `ISTANBUL` **and** `ıstanbul` each show `İstanbul Ofis` (`toLocaleLowerCase('tr').replaceAll('ı','i')` on both sides, `shouldFilter={false}`, I-11: `'İstanbul'.toLocaleLowerCase('tr')` alone is `'istanbul'`, never matching a literal `ıstanbul` query) |
| › `empty text is shown when nothing matches` | `getByText(emptyText)` |
| `multi-select.test` › `toggles values and summarises overflow` | Selecting 4 with `maxChips=2` → 2 chips plus `+2` text from `forms.moreSelected` |
| `radio-group.test` › `arrow keys move selection` | `{ArrowRight}` → `onValueChange('second')` |
| `switch.test` › `space toggles` | `onCheckedChange(true)` |
| `date-picker.test` › `emits ISO dates and never a Date` | Choosing the 15th → `onValueChange('2026-09-15')`; `typeof arg === 'string'` |
| › `week starts on Monday in both locales` | The first `columnheader`'s accessible name (`aria-label`, full weekday) is `Pazartesi` / `Monday` (I-12: this doesn't depend on `react-day-picker`'s short `"cccccc"` display format, whose tr short form is `"Pt"`, not `"Pzt"`) |
| `date-range-picker.test` › `preset applies its range` | Clicking the "Son 7 gün" preset → `onValueChange({ from, to })` |
| `month-picker.test` › `emits YYYY-MM and respects min` | `2026-03`; months before `min` are `aria-disabled` |
| `file-upload.test` › `rejects oversize files with a visible error` | A 2 MB file with `maxSizeBytes=1_000_000` → `onFilesSelected` not called, the error text is visible |
| `file-list.test` › `remove button is labelled with the file name` | `getByRole('button', { name: /fatura\.pdf/ })` |
| `form-error-summary.test` › `focuses and links to fields` | Rerender with errors → summary `toHaveFocus()`; the link `href="#email"` |
| `search-input.test` › `hidden label still names the field` | `getByRole('searchbox', { name: 'Ara' })` |

- [ ] **Step 2: Implement** until green. `forms.json` holds `required`, `optional`, `moreSelected` (`"+{count}"`), `noResults`, `clear`, `chooseDate`, `chooseMonth`, `previousMonth`, `nextMonth`, `previousYear`, `nextYear`, `dropFiles`, `browse`, `fileTooLarge` (`"{name} en fazla {max} olabilir"`), `fileTypeNotAllowed`, `uploading`, `removeFile` (`"{name} dosyasını kaldır"`), `errorSummaryTitle`, and the preset labels `last7Days`, `lastMonth`, `last6Months`, `thisYear`, `lastYear` (appendix values where present).
- [ ] **Step 3: Stories** for every component: default, error, disabled, `LongTurkishLabel`, and an `Open` state tagged `open` for the pickers (`play` opens the popover, I-7).
- [ ] **Step 4: Gate** with `STORIES='UI/Field,UI/Input,UI/Textarea,UI/NumberInput,UI/Select,UI/Combobox,UI/MultiSelect,UI/Checkbox,UI/Switch,UI/RadioGroup,UI/Slider,UI/SearchInput,UI/DatePicker,UI/DateRangePicker,UI/MonthPicker,UI/FileUpload,UI/FileList,UI/FormErrorSummary'`. Commit `feat(f5): task-3 — form controls` `<TRAILERS>`.

## Task 4: Containers, overlays, feedback, navigation and the data table

**Interfaces** — Consumes: T1, T2 (including `Popover`, moved from T4 to T2 per I-6). Produces (under `src/components/ui/`):

| Component | Props |
|---|---|
| `Card`, `CardHeader`, `CardTitle` (`as?: 'h2'\|'h3'`), `CardDescription`, `CardContent`, `CardFooter` | div props. `rounded-lg bg-surface-raised border border-border in-data-[card=shadow]:border-card-edge in-data-[card=shadow]:shadow-md` (in dark the shadow is transparent and `card-edge` draws the edge, D4) |
| `StatTile` | `label: string; value: string /* preformatted */; unit?: Unit; hint?: string` |
| `Badge` | `tone: 'neutral'\|'brand'\|'info'\|'success'\|'warning'\|'danger'; children: ReactNode` plus `ComponentPropsWithRef<'span'>` (M-4: T7's `DataQualityBadge` uses it as a focusable `Tooltip` trigger, needs `tabIndex`/`ref`) |
| `StatusBadge` | `status: 'success'\|'warning'\|'danger'\|'info'\|'neutral'; label: string` (icons `CheckCircle2`, `AlertTriangle`, `XCircle`, `Info`, `CircleDashed`) |
| `Alert` | `tone: 'info'\|'success'\|'warning'\|'danger'; title: string; children?: ReactNode; action?: ReactNode` (`role="alert"` for warning/danger, else `role="status"`) |
| `Toaster`, `useToast()` → `{ toast({ tone, title, description?, action?: { label: string; onClick(): void } }): void }` (M-4: return shape stated explicitly) | Radix Toast, bottom-end, 6 s, pauses on hover and focus |
| `LiveAnnouncerProvider`, `useAnnounce()` → `announce(message: string, politeness?: 'polite'\|'assertive'): void` | Two visually hidden live regions, `aria-live` plus `aria-atomic`, **no** `role` (I-9: an explicit `role="status"` here would make `getByRole('status')` ambiguous against `Alert`/`Toast`) |
| `EmptyState` | `icon?: LucideIcon; title: string; description: string /* what to do next */; action?: ReactNode` |
| `Skeleton` | `className` (size from the caller; `aria-hidden`); `SkeletonText({ lines: number })` |
| `ProgressBar` | `label: string; value: number \| null /* null = indeterminate */; max?: number; thresholds?: { value: number; label: string }[]; valueText?: string` |
| `Stepper` | `steps: { id: string; label: string; description?: string }[]; currentIndex: number` (`aria-current="step"`) |
| `Tabs` (`items: { value: string; label: string; content: ReactNode; disabled?: boolean }[]; value?; defaultValue?; onValueChange?`) | Radix Tabs, `duration-(--duration-tab)` |
| `Accordion` (`items: { value: string; title: string; content: ReactNode }[]; type: 'single'\|'multiple'`) | Radix |
| `Dialog` | `open: boolean; onOpenChange(o: boolean): void; title: string; description?: string; children: ReactNode; footer?: ReactNode; size?: 'sm'\|'md'\|'lg'; trigger?: ReactElement` (rendered via `Dialog.Trigger asChild`, I-10) (`animate-dialog-in`/`-out`, enter 250 ms / exit 180 ms, fade plus scale; close `IconButton` labelled `common.close`). When `trigger` is absent, record `document.activeElement` when `open` becomes true, and in `onCloseAutoFocus` call `event.preventDefault()` then focus the recorded element if still connected (I-10: Radix's own `onCloseAutoFocus` only knows `Dialog.Trigger`'s ref, so an externally-triggered dialog would otherwise drop focus to `body`) |
| `Drawer` | Dialog props (including `trigger?` and the same focus-restore fallback, I-10) plus `side: 'start'\|'end'\|'bottom'` (`animate-drawer-in-<side>`/`-out-<side>`, translate) |
| `DropdownMenu` (`trigger: ReactElement; items: ({ type: 'item'; label: string; icon?: LucideIcon; description?: string; onSelect(): void; disabled?: boolean; tone?: 'danger' } \| { type: 'separator' } \| { type: 'label'; label: string })[]`, exported as `DropdownMenuItem`, M-4: `description?` used by T6 TopNav for disabled items, type consumed by `DataTable.rowActions` and T6/T7) | Radix |
| `Breadcrumb` | `items: BreadcrumbItem[]` where `export type BreadcrumbItem = { label: string; href?: string }` (M-4: exported for T6's `PageHeader.breadcrumb?`) (last item `aria-current="page"`, `nav aria-label={t('feedback.breadcrumb')}`) |
| `Pagination` | `page: number /* 1-based */; pageCount: number; onPageChange(p: number): void; pageSize?: number; pageSizeOptions?: number[]; onPageSizeChange?(n: number): void; totalItems?: number` (D25: the page-size control is a labelled native `<select>` with token classes, not the T3 `Select`) |
| `Table` parts `TableContainer`, `Table`, `TableHeader`, `TableBody`, `TableRow`, `TableHead`, `TableCell` (`numeric?: boolean` → `text-end type-data`) | Semantic table; `TableContainer` is `overflow-auto` with `tabIndex={0}` and `role="region"` plus an `aria-label` |
| `DataTable<T>` | `columns: ColumnDef<T, unknown>[]; data: T[]; caption: string; getRowId(row: T): string; initialSorting?: SortingState; pageSize?: number \| false /* default 25 */; enableColumnVisibility?: boolean; rowActions?(row: T): DropdownMenu items; toolbar?: ReactNode; loading?: boolean; empty: { title: string; description: string; action?: ReactNode }; maxHeight?: string /* sticky header scroll */` |
| `getExportRows(table)` (`data-table.tsx`) and `toCsv(rows: string[][]): string` (`@/lib/csv`) | Visible columns only, formatted cell text; D22 |

Column meta: `meta: { numeric?: boolean; exportValue?(row: T): string }`. Sorting headers are buttons with `aria-sort` on the `th`. Loading renders 5 skeleton rows at row height (no layout shift).

- [ ] **Step 1: Tests first (red).** An axe test per file, plus:

| Test | Assertion |
|---|---|
| `status-badge.test` › `status is never colour alone` | Renders text `label` plus an `svg[aria-hidden]` |
| `alert.test` › `danger announces, info does not interrupt` | `role="alert"` vs `role="status"` |
| `toast.test` › `toast text reaches a live region` | After `toast({ title: 'Kaydedildi' })`, `findByText('Kaydedildi')` has a closest `[aria-live]` or `role="status"` ancestor (I-9: not a bare `getByRole('status')`, which is ambiguous once `LiveAnnouncerProvider` and Radix Toast regions are both mounted) |
| `live-announcer.test` › `assertive uses the alert region` | `announce('x','assertive')` → `[aria-live="assertive"]` text `x` |
| `dialog.test` › `traps focus, Escape closes, focus returns` | Open from a trigger → focus is inside; `Tab` ×10 stays inside; `{Escape}` → `onOpenChange(false)`; rerender closed → trigger `toHaveFocus()`. Test both paths (I-10): with `trigger` set, and without it (record `document.activeElement` before opening, assert it regains focus on close) |
| `drawer.test` › `has the dialog role and a title` | `getByRole('dialog', { name })` |
| › `without a trigger, focus restores on close` | Same as `dialog.test`'s no-trigger path (I-10) |
| `tabs.test` › `arrow keys move between tabs` | `{ArrowRight}` → second tab `aria-selected="true"` |
| `dropdown-menu.test` › `arrow and Enter select` | `onSelect` called |
| `breadcrumb.test` › `last item is the current page` | `aria-current="page"` and not a link |
| `pagination.test` › `announces page x of y and disables at bounds` | Previous is `disabled` on page 1; text `feedback.pageOf` |
| `progress-bar.test` › `null value is indeterminate` | No `aria-valuenow`; the threshold marker has its label |
| `empty-state.test` › `description is required text` | Visible title and description |
| `data-table.test` › `sorts by clicking a header` | Rows reorder; `th` `aria-sort="ascending"` |
| › `hides a column from the visibility menu` | Column header gone; `getExportRows` excludes it |
| › `paginates at pageSize` | 30 rows at `pageSize` 25 → 25 `tbody tr` |
| › `loading renders skeleton rows, empty renders EmptyState` | 5 `[data-skeleton-row]`; `getByText(empty.title)` |
| › `row actions are a labelled menu` | `getByRole('button', { name: /İşlemler/ })` opens a menu |
| `csv.test` › `semicolon, CRLF, BOM and quoting` | `toCsv([['a;b','1.234,5'],['"q"','x']])` = `'﻿"a;b";1.234,5\r\n"""q""";x'` |
| › `escapes formula-injection prefixes` | `toCsv([['=SUM(A1)','+1','-1','@x','plain']])` prefixes `'` to the `=`/`+`/`-`/`@`-leading cells but not `plain`; a `formatNumber`-produced cell such as `'-1.234,5'` is left as-is (M-12) |

`tests/a11y/overlay-motion.spec.ts`: the `UI/Dialog` `Open` story's content has `animation-duration` `0.25s`, and ≤ `0.001s` under `reducedMotion: 'reduce'`.

- [ ] **Step 2: Implement** until green. Add `Toaster` and `LiveAnnouncerProvider` to `providers.tsx`. Namespaces: `feedback.json` (`breadcrumb`, `close`, `pageOf` `"Sayfa {page} / {count}"`, `previousPage`, `nextPage`, `rowsPerPage`, `loading`, `dismiss`, `stepOf`, `notifications`); `table.json` (`columns`, `actions`, `sortAscending`, `sortDescending`, `noResults`, `export`). Use appendix values where present.
- [ ] **Step 3: Stories** for each: `Dialog`/`Drawer`/`DropdownMenu` include an `Open` story tagged `open` (a `play` function opens it, I-7) so the sweep checks portalled content. `DataTable` has `Default` (40 Turkish building rows, numeric columns), `Loading`, `Empty` and `ManyColumns` (horizontal scroll inside the container, never the page).
- [ ] **Step 4: Gate** with `STORIES='UI/Card,UI/StatTile,UI/Badge,UI/StatusBadge,UI/Alert,UI/Toast,UI/LiveAnnouncer,UI/EmptyState,UI/Skeleton,UI/ProgressBar,UI/Stepper,UI/Tabs,UI/Accordion,UI/Dialog,UI/Drawer,UI/DropdownMenu,UI/Breadcrumb,UI/Pagination,UI/Table,UI/DataTable'` (no `UI/Popover`: it is T2's, I-6) plus `overlay-motion.spec.ts`. Commit `feat(f5): task-4 — containers, overlays, feedback, navigation, data table` `<TRAILERS>`.

## Task 5: Chart wrappers

**Interfaces** — Consumes: T1 (`formatNumber`, `unitSymbol`, `Unit`, energy tokens), T2, T4 (`Table` parts, `Skeleton`, `EmptyState`, `Alert`, `Button`). Produces (`src/components/charts/`; pages import only the wrappers; `ChartFrame`, `ChartLegend`, `ChartTooltip`, `ChartDataTable` live in `_`-prefixed internal files, tested directly and shown through every wrapper story):
```ts
// _theme.ts (full code — the one place series look is decided)
export type SeriesKind = 'consumption' | 'generation' | 'reactiveInductive' | 'reactiveCapacitive' | 'cost'
  | 'revenue' | 'forecast' | 'current' | 'previous' | 'target';
const COLOR: Record<SeriesKind, string> = {
  consumption: 'var(--color-consumption)', generation: 'var(--color-generation)',
  reactiveInductive: 'var(--color-reactive-inductive)', reactiveCapacitive: 'var(--color-reactive-capacitive)',
  cost: 'var(--color-cost)', revenue: 'var(--color-revenue)', forecast: 'var(--color-forecast)',
  current: 'var(--color-brand)', previous: 'var(--color-foreground-subtle)', target: 'var(--color-foreground-muted)',
};
export const DASHES = ['0', '6 3', '2 3', '8 3 2 3', '1 3', '12 4'] as const;
export const MAX_SERIES = 6;
export type ChartSeries = { key: string; label: string; kind: SeriesKind; unit: Unit };
export type Datum = { x: string | number } & Record<string, number | string | null>;
export function seriesStyle(s: ChartSeries, index: number): { color: string; dash: string } {
  return { color: COLOR[s.kind], dash: s.kind === 'forecast' ? '6 4' : DASHES[index % DASHES.length] };
}
export function chartAnimation(reducedMotion: boolean) {
  return { isAnimationActive: !reducedMotion, animationDuration: 400, animationEasing: 'ease-out' as const };
}
export function toPlot(v: number | string | null): number | null { return v === null ? null : Number(v); } // D11
export const AXIS = { stroke: 'var(--color-border-strong)', tick: { fill: 'var(--color-foreground-muted)', fontSize: 12 } };
export const GRID = { stroke: 'var(--color-border)', strokeDasharray: '3 3' };
export const CURSOR = { fill: 'var(--color-surface-sunken)' }; // Recharts Tooltip cursor default (#ccc) ignores theme (M-15)
export const ACTIVE_DOT = { stroke: 'var(--color-surface)', fill: 'var(--color-brand)' }; // default activeDot fill (#fff) ignores theme (M-15)
```
Every wrapper passes `CURSOR` to `Tooltip.cursor` and `ACTIVE_DOT` to `Line`/`Area`'s `activeDot`, because those are inline SVG attributes Recharts does not theme itself (M-15).
- `usePrefersReducedMotion(): boolean` (`use-reduced-motion.ts`, a `matchMedia` subscription).
- `downsampleLttb(data: Datum[], seriesKey: string, threshold = 1000): Datum[]` (`downsample.ts`, largest-triangle-three-buckets on the first series, keeping first and last).
- `ChartFrame` props, shared by every wrapper as `BaseChartProps`:
```ts
export type BaseChartProps = {
  title: string; description: string; data: Datum[]; series: ChartSeries[];
  xLabel: string; formatX?: (x: string | number) => string;
  height?: number /* default 320; skeleton uses the same */; loading?: boolean;
  empty: { title: string; description: string; action?: ReactNode };
  footnote?: string; dataTableDefaultOpen?: boolean;
};
```
`ChartFrame` renders a `<figure aria-labelledby aria-describedby>`: title (`type-h3`), description, `ChartLegend` (a swatch drawn as an SVG line with the series dash, then label, then `(unit symbol)`), then the plot area at a fixed `height`. When loading it shows `Skeleton` at that height. With no data it shows `EmptyState`. With more than `MAX_SERIES` series it shows `Alert tone="warning"` (`role="alert"`, D21) in place of the chart — queried as `within(getByRole('figure', { name: title })).getByRole('alert')` (I-9: a warning `Alert` is `role="alert"`, so a bare `getByRole('status')` cannot find it). With more than 1000 points it downsamples and appends the `charts.downsampled` footnote (`"{shown} / {total} nokta gösteriliyor"`), with `{shown}` and `{total}` passed through `formatNumber` (M-7: so `1000`/`3000` render as `1.000`/`3.000`, matching every other number on the page). A toggle `Button` (`aria-expanded`, `aria-controls`) labelled `charts.showTable` / `charts.hideTable` opens `ChartDataTable`, which is `Table` with `caption=title`, columns `xLabel` then `label (unit)`, and cells `formatNumber(original string)` using `numeric`. It always covers the full, not downsampled, data. `ChartTooltip` lists **every** series at the hovered x with `formatQuantity`, using `CURSOR`/`ACTIVE_DOT` (M-15).

| Wrapper | Extra props | Rendering rules |
|---|---|---|
| `LineChart` | `band?: { lowerKey: string; upperKey: string; label: string }`; `directLabels?: boolean` (default true) | Line per series with `strokeDasharray`, end-of-line direct label; band = neutral `forecast` area at 15% opacity |
| `AreaChart` | `series: [ChartSeries]` (a one-element tuple, enforced by type) | 07 §5: area only for a single series |
| `BarChart` | `layout?: 'vertical' \| 'horizontal'`; `sort?: 'desc'`; `referenceLine?: { value: number; label: string }`; `diverging?: boolean` | Grouped; `diverging` draws a zero `ReferenceLine` and colours by sign using `consumption`/`generation` kinds from `series` |
| `StackedBarChart` | `layout?: 'vertical' \| 'horizontal'` | One stack; segment separators use a 1 px `surface` stroke |
| `ComboChart` | Replaces `series` with `bars: ChartSeries[]; lines: ChartSeries[]` (combined ≤ 6) | Bars first, lines on top, shared y-axis unless units differ (then right axis) |
| `GaugeChart` | Replaces `data/series` with `value: string \| null; min: number; max: number; unit: Unit; thresholds: { value: number; label: string; status: 'warning' \| 'danger' }[]; label: string` | Hand-built SVG semicircle with threshold ticks labelled in text; data table = value plus thresholds |
| `HeatmapChart` | Replaces `data/series` with `rows: string[]; columns: string[]; values: (string \| null)[][]; unit: Unit; kind: SeriesKind` | SVG grid; 5 buckets rendered as `color-mix(in oklab, <kind colour> N%, var(--color-surface))` with N ∈ {15,35,55,75,95}; bucket legend with ranges; null cells hatched with `border` stroke plus `charts.noData` in the table |
| `Sparkline` | `data: (string \| null)[]; kind: SeriesKind; label: string; width?: number; height?: number` | No frame, no table (D12). `role="img"`, `aria-label` = `charts.sparklineSummary` with first, last, min and max |

In tests, Recharts wrappers render inside `ResponsiveContainer initialDimension={{ width: 640, height }}`.

- [ ] **Step 1: Tests first (red).**

| Test | Assertion |
|---|---|
| `_theme.test` › `energy kinds map to their tokens and never change` | `seriesStyle({kind:'consumption'},0).color` = `'var(--color-consumption)'`; same for all 7 energy kinds |
| › `forecast is always dashed and neutral` | Any index → `{ color: 'var(--color-forecast)', dash: '6 4' }` |
| › `series at different indices get different dashes` | `seriesStyle(a,0).dash !== seriesStyle(b,1).dash` |
| › `reduced motion disables animation` | `chartAnimation(true).isAnimationActive === false`; `chartAnimation(false).animationDuration === 400` |
| `downsample.test` › `reduces to the threshold, keeps endpoints` | 5000 → 1000; first and last `x` preserved |
| `_chart-frame.test` › `figure is named and described` | `getByRole('figure', { name: title })` with `aria-describedby` → description |
| › `data table toggles and shows exact values with units` | Click `charts.showTable` → `aria-expanded="true"`; header `Tüketim (kWh)`; cell `1.234,567891` for input `'1234.567891'` |
| › `loading keeps the final height` | Skeleton `style.height` = `'320px'` |
| › `empty state explains what is missing` | `getByText(empty.title)`; no `svg.recharts-surface` |
| › `more than six series renders an alert, not a chart` | 7 series → `within(getByRole('figure', { name: title })).getByRole('alert')` text `charts.tooManySeries` (I-9) |
| › `over 1000 points downsamples and says so` | 3000 points → footnote text contains `1.000` and `3.000` (`formatNumber`, M-7); the table still has 3000 rows when opened |
| `_chart-legend.test` › `legend shows label, unit and dash sample` | Text `Üretim (kWh)`; swatch `line[stroke-dasharray="6 3"]` for series index 1 |
| `_chart-tooltip.test` › `lists every series at x with units` | Given a payload of 3 series → 3 rows, each with `formatQuantity` text |
| `line-chart.test` › `renders one path per series with the series dash` | 2 series → 2 `.recharts-line-curve`; the second has `stroke-dasharray="6 3"` |
| › `animation follows prefers-reduced-motion` | `setMatchMedia(q => q.includes('reduce'))` → the rendered `Line` receives `isAnimationActive=false` (assert via `vi.mock('recharts', …)` pass-through spy) |
| `area-chart.test` › `type rejects two series` | `// @ts-expect-error` on a two-element `series` (typecheck is the assertion) plus a runtime render of one series |
| `bar-chart.test` › `sort desc orders categories; reference line is labelled` | First category is the largest; text of `referenceLine.label` |
| `combo-chart.test` › `bars and lines both render` | `.recharts-bar-rectangle` count > 0 and `.recharts-line-curve` count = `lines.length` |
| `gauge-chart.test` › `thresholds are labelled in text; null value is no-data` | Threshold label text visible; `value=null` → `charts.noData` |
| `heatmap-chart.test` › `null cells are distinguishable without colour` | A null cell has a `pattern`/hatch fill; the table cell text is `charts.noData` |
| `sparkline.test` › `summary label` | `getByRole('img', { name: /En düşük/ })` |

- [ ] **Step 2: Implement** until green. `charts.json`: `showTable`, `hideTable`, `downsampled`, `tooManySeries`, `noData`, `sparklineSummary` (`"İlk {first}, son {last}, en düşük {min}, en yüksek {max}"`), `forecastBand`, `target`, `legend`, `valueAt` (`"{x}: {value}"`).
- [ ] **Step 3: Stories**, all with realistic Turkish energy data and units: `LineChart/ConsumptionVsLastYear`, `LineChart/LoadProfile24h` (3 profiles, 3 dashes), `LineChart/ForecastWithBand`, `AreaChart/SingleSeries`, `BarChart/YearOverYear` (`current`/`previous`), `BarChart/EmissionsByCategory` (horizontal, sorted), `BarChart/ImportExportBalance` (diverging), `BarChart/TargetVsActual`, `StackedBarChart/InvoiceComposition` (horizontal), `StackedBarChart/Scope123`, `ComboChart/ConsumptionVsGeneration`, `GaugeChart/ReactiveRatio`, `HeatmapChart/HourByWeekday`, `Sparkline/Default`, plus `Loading`, `Empty`, `TooManySeries`, `Downsampled` (3000 points). That covers every row of 07 §5's selection table.
- [ ] **Step 4: Gate** with `STORIES='Charts/'`. The `keyboard.spec` must reach the table toggle in every chart story. Commit `feat(f5): task-5 — chart wrappers with units, legends, tooltips, dashes and data tables` `<TRAILERS>`.

## Task 6: Application shell, UI preferences and theme customiser

**Interfaces** — Consumes: T1–T4 (`Drawer`, `DropdownMenu`, `Tooltip`, `Popover` (T2), `Breadcrumb` and `BreadcrumbItem` (T4), `RadioGroup`, `Switch`, `Slider`, `SearchInput`, `Badge`); `useUiPreferences` is produced below and consumed by every shell component. Produces:
```ts
// src/lib/ui-preferences.ts (full code)
export const UI_COOKIE = 'ekokod_ui';
export type UiPreferences = {
  theme: 'light' | 'dark' | 'system'; layout: 'vertical' | 'horizontal'; container: 'full' | 'boxed';
  sidebar: 'expanded' | 'collapsed'; card: 'border' | 'shadow'; radiusScale: number;
};
export const defaultUiPreferences: UiPreferences = {
  theme: 'system', layout: 'vertical', container: 'full', sidebar: 'expanded', card: 'border', radiusScale: 1,
};
const pick = <T extends string>(v: unknown, allowed: readonly T[], fallback: T): T =>
  typeof v === 'string' && (allowed as readonly string[]).includes(v) ? (v as T) : fallback;
export function parseUiPreferences(raw: string | undefined): UiPreferences {
  let o: Record<string, unknown> = {};
  try { o = raw ? (JSON.parse(decodeURIComponent(raw)) as Record<string, unknown>) : {}; } catch { o = {}; }
  const d = defaultUiPreferences;
  const r = typeof o.radiusScale === 'number' && Number.isFinite(o.radiusScale) ? o.radiusScale : d.radiusScale;
  return {
    theme: pick(o.theme, ['light', 'dark', 'system'], d.theme),
    layout: pick(o.layout, ['vertical', 'horizontal'], d.layout),
    container: pick(o.container, ['full', 'boxed'], d.container),
    sidebar: pick(o.sidebar, ['expanded', 'collapsed'], d.sidebar),
    card: pick(o.card, ['border', 'shadow'], d.card),
    radiusScale: Math.min(1.5, Math.max(0.5, Math.round(r * 4) / 4)),
  };
}
export function serializeUiPreferences(p: UiPreferences): string { return encodeURIComponent(JSON.stringify(p)); }
export function htmlAttributes(p: UiPreferences) {
  return { 'data-theme': p.theme, 'data-layout': p.layout, 'data-container': p.container, 'data-sidebar': p.sidebar,
    'data-card': p.card, style: { '--radius-scale': String(p.radiusScale) } as React.CSSProperties };
}
```
- `UiPreferencesProvider({ initial: UiPreferences; children })` and `useUiPreferences(): { prefs: UiPreferences; update(patch: Partial<UiPreferences>): void }`. `update` writes the cookie (`path=/; max-age=31536000; SameSite=Lax`) and mirrors the attributes onto `document.documentElement` immediately.
- `setLocale(locale: Locale): Promise<void>` (`src/i18n/actions.ts`, `'use server'`, validates with `resolveLocale`, sets `NEXT_LOCALE` for 1 year).
- `nav-config.ts` (full data; labels are `shell.nav.*` keys):
```ts
export type NavLeaf = { id: string; labelKey: string; href: string; icon: LucideIcon; disabled?: boolean };
export type NavGroup = { id: string; labelKey: string; icon: LucideIcon; children: NavLeaf[] };
export type NavEntry = NavLeaf | NavGroup;
export const navigation: NavEntry[] = [
  { id: 'dashboard', labelKey: 'dashboard', href: '/dashboard', icon: LayoutDashboard },
  { id: 'dataAnalysis', labelKey: 'dataAnalysis', icon: LineChart, children: [
    { id: 'consumption', labelKey: 'consumption', href: '/consumption', icon: Zap },
    { id: 'loadProfile', labelKey: 'loadProfile', href: '/load-profile', icon: Activity },
    { id: 'forecast', labelKey: 'forecast', href: '/forecast', icon: TrendingUp },
    { id: 'solarPlants', labelKey: 'solarPlants', href: '/solar-plants', icon: Sun },
    { id: 'financial', labelKey: 'financialAnalysis', href: '/financial', icon: Wallet },
    { id: 'renewable', labelKey: 'renewableEnergy', href: '/renewable', icon: Leaf },
    { id: 'water', labelKey: 'water', href: '/water', icon: Droplets, disabled: true },
    { id: 'gas', labelKey: 'gas', href: '/gas', icon: Flame, disabled: true },
    { id: 'evDrivers', labelKey: 'evDrivers', href: '/ev-drivers', icon: PlugZap, disabled: true } ] },
  { id: 'billsTariffs', labelKey: 'billsAndTariffs', icon: Receipt, children: [
    { id: 'bills', labelKey: 'bills', href: '/bills', icon: FileText }, { id: 'tariffs', labelKey: 'tariffs', href: '/tariffs', icon: Tags } ] },
  { id: 'alarms', labelKey: 'alarms', icon: Bell, children: [
    { id: 'alarmsManual', labelKey: 'manual', href: '/alarms', icon: BellRing }, { id: 'messages', labelKey: 'messages', href: '/messages', icon: MessageSquare },
    { id: 'alarmsAi', labelKey: 'ai', href: '/alarms/ai', icon: Sparkles, disabled: true } ] },
  { id: 'reports', labelKey: 'reports', href: '/reports', icon: FileBarChart },
  { id: 'settings', labelKey: 'settings', href: '/settings', icon: Settings },
  { id: 'carbon', labelKey: 'carbonFootprint', href: '/carbon', icon: Factory },
  { id: 'iso50001', labelKey: 'iso50001', href: '/iso-50001', icon: BadgeCheck },
  { id: 'savingActions', labelKey: 'savingActions', href: '/saving-actions', icon: PiggyBank, disabled: true },
  { id: 'contact', labelKey: 'contact', href: '/contact', icon: Mail },
];
```
(`shell.nav` values come from the appendix: `Dashboard` → "Yönetim Arayüzü", `Data Analysis` → "Veri Analizi", `Consumption` → "Tüketim", `Load Profile` → "Yük Profili", `Predict` → "Tüketim Tahmini", `Solar Power Plants` → "GES Santralleri", `Financial Analysis`, `Renewable Energy`, `Water` → "Su", `Gas` → "Doğal Gaz", `EV Drivers`, `Bills And Tariffs`, `Bills`, `Tariffs`, `Alarms`, `Manual`, `Messages`, `AI` → "Yapay Zeka", `Reports`, `Settings`, `Carbon Footprint`, `Iso 50001 Module`, `Saving Actions` → "Tasarruf Önerileri", `Contact`.)

| Component | Props / behaviour |
|---|---|
| `AppShell` | `{ children; user: { name: string; email: string; roleLabel: string }; notificationCount?: number }` (I-15: no `breadcrumb`/`initialPreferences` — a route-group `layout.tsx` never learns the current page, so a per-page prop there would force an F6 redesign). The breadcrumb is derived inside the shell from `usePathname()` and `navigation`, overridable per page via `PageHeader.breadcrumb?: BreadcrumbItem[]`. The shell reads preferences through `useUiPreferences()`, not a prop. Renders a skip link (`shell.skipToContent`) → `<main id="main-content" tabIndex={-1}>`. Layout is a 12-column grid with `gap-6` gutters; `data-container=boxed` → `max-w-[1600px] mx-auto`; at 2xl the content is capped at 1600 px and centred |
| `Sidebar` | `{ collapsed: boolean; onNavigate?(): void }`. `nav aria-label={shell.nav.label}`; groups are disclosure buttons (`aria-expanded`); the active leaf has `aria-current="page"` (from `usePathname`) and a `brand` start-edge bar. Disabled leaf: `<span role="link" aria-disabled="true" tabIndex={0}>` wrapped in `Tooltip content={t('shell.notYetAvailable')}`, `text-foreground-subtle`. Collapsed (≥lg) shows icons only with a `Tooltip` label each |
| `TopNav` | Horizontal layout at ≥lg: groups as `DropdownMenu`s; disabled items `disabled` with the same tooltip text in the menu item's description (D24) |
| `TopBar` | Logo (`brand`), `SearchInput labelVisibility="hidden"`, notifications `IconButton` plus `Badge` count (`aria-label` `shell.notificationsCount {count}`), `LanguageSwitcher`, `ThemeToggle`, `UserMenu`, customiser `IconButton` (`Settings2`), sidebar toggle `IconButton` (`PanelLeft`, `aria-expanded`) |
| Responsive | CSS-first, not JS-first (I-16: a `useMediaQuery`-only layout renders drawer mode on the server and on first client paint, then docks after hydration — a visible shift at ≥lg on every navigation). The docked sidebar is `hidden lg:flex` (collapsed or expanded from `data-sidebar` on `<html>`); the toggle that opens the `Drawer` is `lg:hidden`. The `Drawer` mounts only while open. `useMediaQuery('(min-width: 1024px)')` is used only to close an already-open drawer when the viewport crosses `lg`. `<md`: the search collapses to an `IconButton` that opens a `Popover` with `SearchInput`. The page body never scrolls sideways |
| `PageHeader` | `{ title: string; description?: string; actions?: ReactNode; breadcrumb?: BreadcrumbItem[] }` (`h1 type-h1`; `breadcrumb` overrides the shell's derived one, I-15; actions wrap under the title below `md`) |
| `FilterBar` | `{ children: ReactNode; onApply?(): void; activeCount?: number }`: `sticky top-0 z-10 bg-background`. `<sm`: a `Button` "Filtreler (n)" opens `Drawer side="bottom"` holding the children plus apply |
| `ThemeCustomizer` | `{ open: boolean; onOpenChange(o: boolean): void }`: `Drawer side="end"` with `RadioGroup` theme, `RadioGroup` layout, `RadioGroup` container, `Switch` sidebar collapsed, `RadioGroup` card, `Slider` radius 0.5–1.5 step 0.25, and a reset `Button` |
| `LanguageSwitcher` | `DropdownMenu` of Türkçe / English → `setLocale` then `router.refresh()`; the trigger `aria-label` is `shell.language` |
| `ThemeToggle` | Cycles light → dark → system; `aria-label` includes the current mode |
| `UserMenu` | Name, role, profile/logout items (callbacks are no-ops until F6) |

Next integration: `src/app/layout.tsx` reads `UI_COOKIE` with `cookies()` and spreads `htmlAttributes(parseUiPreferences(...))` onto `<html>` (keeping `lang` and `fontVariables`) and nests `NextIntlClientProvider` → `UiPreferencesProvider` → `AppProviders`. `UiPreferencesProvider` is mounted here only, never in `providers.tsx` (I-15, resolves the ambiguity about `AppProviders`' signature). Breakpoints use `useMediaQuery(query: string): boolean` (`shell/use-media-query.ts`). `src/app/(app)/layout.tsx` renders `AppShell` with placeholder user `{ name: 'Demo', … }` (F6 replaces it). Move `src/app/page.tsx` to `src/app/(app)/page.tsx` with `PageHeader` plus `HealthStatus` (component at `src/app/_components/health-status.tsx`, T1; I-19). T6 also edits `src/test/render.tsx` (new `preferences?: Partial<UiPreferences>` option, wraps `UiPreferencesProvider`) and `.storybook/preview.tsx` (wraps `UiPreferencesProvider` with `theme` taken from Storybook globals) — both are narrow, named exceptions to T2's ownership of those files (I-15).

- [ ] **Step 1: Tests first (red).**

| Test | Assertion |
|---|---|
| `ui-preferences.test` › `garbage and missing cookies give defaults` | `parseUiPreferences(undefined)`, `('%%%')`, `('{"theme":"neon"}')` → `defaultUiPreferences` |
| › `radius is clamped and quantised` | `radiusScale: 9` → 1.5; `0.61` → 0.5; `1.1` → 1 |
| › `round trip` | `parseUiPreferences(serializeUiPreferences(p))` deep-equals `p` |
| `theme-customizer.test` › `changing theme updates html and cookie` | Choose "Koyu" → `document.documentElement.dataset.theme === 'dark'`; `document.cookie` contains `ekokod_ui=` with `%22dark%22` |
| › `radius slider sets --radius-scale` | `ArrowRight` → `style.getPropertyValue('--radius-scale') === '1.25'` |
| `sidebar.test` › `groups and leaves match 07 §7 order` | The top-level accessible names equal `[Yönetim Arayüzü, Veri Analizi, Faturalar ve Tarifeler, Alarmlar, Raporlar, Ayarlar, Karbon Ayak İzi, ISO 50001 Modülü, Tasarruf Önerileri, İletişim]` |
| › `disabled entries are visible, not links, and explain why` | `Su`, `Doğal Gaz`, `EV Sürücüleri`, `Yapay Zeka`, `Tasarruf Önerileri`: `aria-disabled="true"`, no `href`; focusing one → `findByRole('tooltip')` text `shell.notYetAvailable` |
| › `active route is marked` | With `usePathname` mocked to `/load-profile` → that link `aria-current="page"`, its group expanded |
| `app-shell.test` › `skip link moves focus to main` | Activating the skip link → `#main-content` `toHaveFocus()` |
| › `the sidebar toggle opens a dialog` | Clicking the toggle → `getByRole('dialog', { name: shell.nav.label })` (I-16: visibility per width is CSS-only and stays in `shell.spec.ts`, which already checks it at all four widths; this test no longer depends on `setMatchMedia`) |
| `language-switcher.test` › `calls setLocale then refresh` | Mocked action called with `'en'`; `router.refresh` called |
| `filter-bar.test` › `small screens get a filter sheet with the active count` | Button text `Filtreler (2)` |
| `page-header.test` › `title is the page h1` | `getByRole('heading', { level: 1 })` |

`tests/a11y/shell.spec.ts` (story `Shell/AppShell` `Default`, and `Horizontal`, `Collapsed`, `Boxed`), for widths `[375, 768, 1024, 1440]` × `['light','dark']`:
  - `scrollWidth <= innerWidth` (no horizontal page scroll).
  - At 1024 and 1440 the `nav[aria-label]` is visible without interaction. At 375 and 768 it is hidden until the toggle is pressed, then a `dialog` contains it.
  - Hovering or focusing a disabled entry shows a tooltip with `shell.notYetAvailable`.
  - Axe (no disabled rules) finds zero violations. The skip link is the first tab stop.
  - A screenshot is saved to `test-results/shell-<w>-<theme>.png` (evidence for T8, not committed).

- [ ] **Step 2: Implement** until green. `shell.json`: `nav.*` (the keys above plus `label`), `notYetAvailable` ("Bu bölüm henüz kullanıma açılmadı" / "This section is not available yet"), `skipToContent`, `search`, `notificationsCount`, `language`, `theme.{light,dark,system,label,toggle}`, `customizer.{title,layout,vertical,horizontal,container,full,boxed,sidebarCollapsed,card,border,shadow,radius,reset}`, `userMenu.{profile,logout}`, `filters`, `filtersCount`, `toggleSidebar`.
- [ ] **Step 3: Stories:** `Shell/AppShell` (`Default`, `Horizontal`, `Collapsed`, `Boxed`, `WithFilterBar` containing `PageHeader`, `FilterBar` and a `StatTile` row), `Shell/ThemeCustomizer` (`Open`, tagged `open`, I-7), `Shell/PageHeader`, `Shell/FilterBar`, `Shell/Sidebar`, `Shell/TopBar`, `Shell/TopNav`, `Shell/LanguageSwitcher`, `Shell/ThemeToggle`, `Shell/UserMenu`. Stories use `parameters.nextjs.navigation.pathname`.
- [ ] **Step 4: Gate** with `SB_PORT=6006 STORIES='Shell/'` plus `pnpm exec playwright test tests/a11y/shell.spec.ts`, and `flock /home/personal/.ekokod-f5-heavy.lock env NODE_OPTIONS=--max-old-space-size=3072 pnpm build` (I-2; proves the RSC/client boundaries, cookies and fonts in a real build). Commit `feat(f5): task-6 — app shell, UI preferences, theme customiser, locale switch` `<TRAILERS>`.

## Task 7: Domain components and Map

**Interfaces** — Consumes: T1–T4 (`Combobox`, `Switch`, `DateRangePicker`, `Select`, `Card`, `StatusBadge`, `Badge`, `Tooltip`, `ProgressBar`, `DataTable`, `DropdownMenu`, `Alert`, `useAnnounce`, `toCsv`, `getExportRows`). Produces (`src/components/domain/`, all presentational per D17):

| Component | Props |
|---|---|
| `BuildingAnalyzerPicker` | `buildings: { id: string; name: string; active: boolean }[]; analyzers: { id: string; buildingId: string; name: string; active: boolean }[]; value: { buildingId: string \| null; analyzerId: string \| null }; onValueChange(v): void; activeOnly: boolean; onActiveOnlyChange(v: boolean): void`. Two `Combobox`es; the analyzer list is filtered to the building (and `active` when `activeOnly`); changing the building clears an analyzer that no longer belongs. "Remembered selection" is the caller's job (F6 store). The analyzer `Combobox` is `disabled` until a building is chosen, with description `domain.picker.chooseBuildingFirst` |
| `PeriodFilterBar` | `granularity: Granularity; onGranularityChange(g): void; range: { from: string; to: string }; onRangeChange(r): void; onApply(): void; today: string /* YYYY-MM-DD, injected — no clock in the component */; applying?: boolean; allowedGranularities?: Granularity[]`, where `export type Granularity = 'hourly' \| 'daily' \| 'monthly' \| 'yearly'` (M-4). Presets computed from `today`: last 7 days, last month, last 6 months (M-14: from the first day of the month 5 months before `today`'s month, to `today` — e.g. `today='2026-09-17'` → `{ from: '2026-04-01', to: '2026-09-17' }`), this year, last year. Renders its own control row (no dependency on T6); pages place it inside `FilterBar` |
| `MetricCard` | `label: string; value: string \| null; unit: Unit; delta?: { value: string /* signed decimal percent */; direction: 'up' \| 'down' \| 'flat'; sentiment: 'good' \| 'bad' \| 'neutral'; comparisonLabel: string }; sparkline?: ReactNode; quality?: DataQuality; loading?: boolean`. Value in `type-metric`; the delta shows an arrow icon, sign, `%` and text, coloured by `sentiment` (success/danger/foreground-muted), never by direction |
| `ReactiveStatusCard` | `inductive: { ratio: string \| null /* decimal fraction, e.g. '0.25'; displayed as %25 via formatQuantity(_, 'percent'), M-14 */; limit: string }; capacitive: { ratio: string \| null; limit: string }; penaltyApplied: boolean \| null; advisory?: string; period: string`. Each ratio is a `ProgressBar` with a threshold marker at the limit, plus a `StatusBadge` (within limit → success "Sınır içinde", over → danger "Sınır aşıldı", null → neutral `domain.reactive.noData`) |
| `InvoiceLineTable` | `lines: InvoiceLineView[]; currency: 'TRY'; totals: { label: string; amount: string }[]; caption: string` with `export type InvoiceLineView = { id: string; description: string; quantity: string \| null; unit: Unit \| null; unitPrice: string \| null; amount: string; kind: 'energy' \| 'distribution' \| 'tax' \| 'penalty' \| 'other' }`. Static `Table` (no pagination), numeric cells, the totals row in `tfoot`, `penalty` rows marked with a `Badge tone="danger"` text |
| `ExportMenu` | `formats?: ('csv' \| 'excel' \| 'pdf' \| 'email')[] /* default all */; onExport(format): void \| Promise<void>; busyFormat?: string \| null; disabled?: boolean`. `DropdownMenu` with icons; while busy the trigger shows `loading` and announces `domain.export.started`; blocks `onOpenChange(true)` while `busyFormat` is set (M-17: `Button loading` only swallows `click`, but Radix `DropdownMenu.Trigger` opens on `pointerdown`, so the menu would still open while busy) |
| `JobStatusBanner` | `job: { id: string; label: string; status: 'queued' \| 'running' \| 'succeeded' \| 'failed'; progress?: number \| null; message?: string; resultAction?: { label: string; onClick(): void } } \| null; onDismiss?(): void`. `Alert` (info/running, success, danger) with `ProgressBar`; a status transition calls `announce` (assertive on failure) |
| `DataQualityBadge` | `quality: DataQuality` with `export type DataQuality = { state: 'complete' } \| { state: 'estimated' \| 'incomplete' \| 'suspect'; reason: string; coverage?: string /* decimal percent */ }`. Renders nothing for `complete`; otherwise `Badge` (warning for estimated/incomplete, danger for suspect) with icon, text and a `Tooltip` holding `reason` and coverage. The badge is focusable (`tabIndex={0}`) so the tooltip is keyboard reachable |

`src/components/map/`: `Map({ markers: MapMarker[]; label: string; tileUrl?: string /* default process.env.NEXT_PUBLIC_MAP_TILE_URL */; height?: number; onMarkerSelect?(id: string): void })` with `MapMarker = { id: string; name: string; lat: number; lng: number; status: 'active' \| 'passive' }`. MapLibre is dynamically imported inside `useEffect` (never on the server). Markers are DOM buttons with `aria-label` "`name` — Aktif/Pasif" and status shape plus colour (filled circle `success` vs hollow ring `foreground-subtle`). Fallback per D23 is `MapMarkerList` (`_map-marker-list.tsx`) (a `Table` of name, status badge, coordinates via `formatNumber` to 5 digits, select button) with `Alert tone="info"` `map.tilesUnavailable`. The list is always available behind a `map.showList` toggle.

- [ ] **Step 1: Tests first (red).**

| Test | Assertion |
|---|---|
| `building-analyzer-picker.test` › `analyzers are filtered to the building` | After choosing building A, only A's analyzers are options |
| › `changing building clears a foreign analyzer` | `onValueChange({ buildingId: 'b', analyzerId: null })` |
| › `active only hides passive entries` | A passive analyzer is absent with `activeOnly` |
| `period-filter-bar.test` › `presets are computed from the injected today` | `today='2026-09-17'`: "Son 7 gün" → `{ from: '2026-09-11', to: '2026-09-17' }`; "Geçen yıl" → `{ from: '2025-01-01', to: '2025-12-31' }`; "Geçen ay" → `{ from: '2026-08-01', to: '2026-08-31' }`; "Son 6 ay" → `{ from: '2026-04-01', to: '2026-09-17' }` (M-14) |
| › `apply is a button that reports busy` | `applying` → `aria-busy` |
| `metric-card.test` › `delta colour follows sentiment, not direction` | `direction:'up', sentiment:'bad'` → delta element has class `text-danger` and an up-arrow `svg` |
| › `null value is an em dash and quality badge shows` | `—` plus `getByText('Tahmini')` |
| `reactive-status-card.test` › `over the limit is danger with text` | ratio `'0.25'`, limit `'0.20'` → `Sınır aşıldı` text |
| › `null ratio is no-data, never zero` | `domain.reactive.noData` text; no `%0` |
| `invoice-line-table.test` › `amounts are Turkish-formatted and totals in tfoot` | Cell `₺1.234,56`; `tfoot` contains the total |
| `export-menu.test` › `each format calls onExport` | Selecting "Excel" → `onExport('excel')` |
| `job-status-banner.test` › `failure is announced assertively` | Rerender `running` → `failed` → the assertive live region has text |
| `data-quality-badge.test` › `complete renders nothing` | `container` empty |
| › `reason is keyboard reachable` | `user.tab()` → tooltip text `reason` |
| `map.test` › `without a tile URL the coordinate list renders` | `getByRole('table')`; `map.tilesUnavailable` alert; no `maplibre-gl` import attempted (`vi.mock` spy not called) |
| › `markers carry status in text` | Row text `Merkez Bina` plus `Aktif` |

- [ ] **Step 2: Implement** until green. `domain.json`: `picker.*`, `period.*` (granularities plus presets, reusing `forms` preset wording where equal), `metric.*`, `reactive.*` (`inductive`, `capacitive`, `withinLimit`, `overLimit`, `noData`, `penaltyApplied`, `penaltyNotApplied`, `limit`), `invoice.*`, `export.*` (`csv`, `excel`, `pdf`, `email`, `started`, `menu`), `job.*`, `quality.*` (`estimated` "Tahmini", `incomplete` "Eksik veri", `suspect` "Şüpheli", `coverage`). `map.json`: `tilesUnavailable`, `showList`, `hideList`, `active` "Aktif", `passive` "Pasif", `coordinates`, `select`.
- [ ] **Step 3: Stories:** every component, with `Loading`/`Empty`/null variants, `MetricCard` with a `Sparkline` (T5), `Map/Fallback` (no tile URL; the only map story the sweep runs offline) and `Map/WithTiles` tagged `no-sweep` with a `tileUrl` arg (excluded from `loadStories` by tag).
- [ ] **Step 4: Gate** with `STORIES='Domain/,Map/'`. Commit `feat(f5): task-7 — domain components and map with coordinate-list fallback` `<TRAILERS>`.

## Task 8: Acceptance sweep, §11 pre-delivery checklist and handoff

**Interfaces** — Consumes: everything merged. Produces: evidence, the checklist file and the handoff. Fixes go to the owning task's files; the controller dispatches each fix as `f5/fix-<n>`.

- [ ] **Step 1: Full verification block** (from `09` §F5, verbatim plus the build), run on the merged phase tip in `/home/personal/ekokod-f5-phase/web`. Record the output. Fix re-runs (not this final evidence run) use `STORIES=<affected prefixes>` then `pnpm exec playwright test --last-failed`; only the run below is the full sweep (I-18):
```bash
pnpm install --frozen-lockfile
pnpm lint && pnpm typecheck && pnpm test --maxWorkers=2
pnpm run check:contrast
pnpm run check:i18n-parity
pnpm audit --audit-level=high   # I-5
flock /home/personal/.ekokod-f5-heavy.lock env NODE_OPTIONS=--max-old-space-size=3072 pnpm storybook:build
flock /home/personal/.ekokod-f5-heavy.lock env SB_PORT=6007 pnpm exec playwright test tests/a11y/
flock /home/personal/.ekokod-f5-heavy.lock env NODE_OPTIONS=--max-old-space-size=3072 pnpm build
git grep -nE "#[0-9a-fA-F]{6}\b" -- 'web/src/**' ':!web/src/styles/tokens.css' ':!web/src/styles/*.test.ts'   # expect no output
pgrep -fa "ekokod-f5.*(next dev|storybook dev|vitest)|http.server 60[0-9][0-9]" || echo "no servers left running"
git diff --stat 58597ec..HEAD -- '*.go' go.mod go.sum   # expect no output: F5 touched no Go (I-17)
make lint build web-lint web-test web-build web-audit   # I-17: the 09 phase-end gate's web part
flock /home/personal/.ekokod-f5-heavy.lock make test    # I-17, I-2: only while F4 is not running Docker tests; if memory doesn't allow it, record the last F3 SHA that proved it instead
```
- [ ] **Step 2: Run the §11 checklist** and write `design-system/bcem-energy/checklists/f5-predelivery.md`. Each item is a checked box followed by the evidence (command, test name or screenshot path) that proves it:

| §11 item | Evidence |
|---|---|
| `ui-ux-pro-max` invoked, master read | `MASTER.md` header (`Generated:`), `OVERRIDES.md`, and each task report's "read MASTER/OVERRIDES" line |
| Zero raw hex, tokens only | `pnpm lint` plus `raw-color-*.test.ts` plus the `git grep` above |
| Light and dark verified, contrast automated | `check:contrast` output (65 pairs × 2 themes) plus the `stories.spec` light/dark results |
| Turkish and English verified, no truncation | `stories.spec` tr/en × `expectNoClippedText` plus `LongTurkishLabel` stories |
| Keyboard reachable, visible ring | `keyboard.spec.ts` result |
| SVG icons, no emoji | `git grep -nP "[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]" -- web/src web/messages` → empty |
| `cursor-pointer`; hover 150–300 ms | The `globals.css` base rule; `reduced-motion.spec` asserts 0.15 s |
| Reduced motion honoured | `reduced-motion.spec.ts`, `overlay-motion.spec.ts`, `_theme.test` › reduced motion |
| Responsive 375/768/1024/1440, no horizontal scroll | `shell.spec.ts` plus the 8 screenshots |
| Skeletons sized, no layout shift | `_chart-frame.test` › loading height; `data-table.test` › skeleton rows |
| Empty states with next step | `EmptyState` required `description`; the `Empty` stories list |
| Error states near the failure | `field.test`, `form-error-summary.test`, `file-upload.test` |
| Charts: units, legend, tooltip, data table | T5 tests (`_chart-frame`, `_chart-legend`, `_chart-tooltip`) |
| Tabular figures, Turkish formatting | `format.test`, `type-data`/`type-metric` utilities, `invoice-line-table.test` |
| `DataQualityBadge` on incomplete-data figures | `data-quality-badge.test`, `metric-card.test` › quality |

Items that cannot be automated (the ≥ 8 px spacing between touch targets, visual quality of both themes) are checked by the controller from the Storybook build screenshots, and the file says so.

- [ ] **Step 3: Handoff** `docs/superpowers/handoffs/2026-09-f5-design-system.md` (not `HANDOFF_NEXT_SESSION.md`, which the F4 track owns). Contents: state and SHAs; rulings D1–D26; open questions Q1–Q7; instruct every future UI plan's Global Constraints to say "read `MASTER.md` **then `OVERRIDES.md`**" (M-9); the supported-browser floor, Chrome/Edge ≥123, Firefox ≥120, Safari ≥17.5, for F15 and the PO (M-18); flag the expected merge of `.github/workflows/ci.yml` and `Makefile` with `phase/f4-billing-engine` (M-13); what F6 must do (sync `ekokod_ui` to the user profile, replace the `(app)` placeholder user, role-filter `navigation`, the generated OpenAPI client plus TanStack Query, the `BuildingAnalyzerPicker` remembered-selection store, Calendar placement Q6, the card-grid stagger D26); what F8 must do (map F4's invoice DTO → `InvoiceLineView`); what F12 must do (public-site locale routing D8, spacious scale, glassmorphism allowed there only). Include the environment and resource rules.
- [ ] **Step 4: Commit** `docs(f5): task-8 — acceptance evidence, §11 pre-delivery checklist, F6 handoff` `<TRAILERS>`.

## Carried forward — explicitly NOT F5

Product screens, auth, the generated OpenAPI client, TanStack Query, role-filtered navigation and per-user preference sync → F6 · public site (spacious scale, glass accents, locale routing) → F12 · RTL and theme colour → only if Q1/Q2 say so · server-side CSV/Excel/PDF/e-mail generation → F8 (F5 ships `ExportMenu` and client `toCsv`) · CSP nonces → F15 (D4 adds no inline script).

## Self-review

### Acceptance criteria → task and proof (09 §F5)

| Criterion | Task | Proven by |
|---|---|---|
| `design-system/bcem-energy/MASTER.md` exists and was produced by the skill | T1 | Step 1 command output plus `head -12` showing `**Generated:**`; `OVERRIDES.md` (D1) |
| Zero raw hex in `components/`, enforced by a lint rule | T1 | `eslint.config.mjs` `no-restricted-syntax`; `raw-color-lint.test.ts` (must-error cases); mutation `bg-red-500` → red; T8 `git grep` |
| Contrast passes for every token pair in both themes | T1 | `pnpm check:contrast` (65 pairs × light/dark, coverage of every `--color-*`); `contrast.test.ts` › `evaluate fails the spec's original primary pair`; CI step (T2) |
| Every component renders in light/dark and tr/en | T2 harness, T3–T7 stories, T8 full run | `stories.spec.ts` (axe, no render error, no console error, no missing message, no clipped text) over every story × 2 themes × 2 locales; `stories-coverage.test.ts` |
| Keyboard navigation with a visible focus ring | T2, T3–T7 | `keyboard.spec.ts` over every story; per-component keyboard tests (dialog, tabs, select, menu, radio) |
| `prefers-reduced-motion` honoured, verified in a test | T2, T4, T5 | `reduced-motion.spec.ts`, `overlay-motion.spec.ts`, `_theme.test` › `reduced motion disables animation`, `line-chart.test` › animation |
| Shell correct at 375/768/1024/1440, no horizontal scroll | T6 | `shell.spec.ts` (4 widths × 2 themes, dock vs drawer, axe, screenshots) |
| Disabled nav entries render disabled with an explanatory tooltip | T6 | `sidebar.test` › disabled entries; `shell.spec.ts` tooltip assertion |
| Charts render with units, legend, tooltip, toggleable data table | T5 | `_chart-frame.test` › toggle; `_chart-legend.test`; `_chart-tooltip.test`; `keyboard.spec` reaches the toggle |
| §11 pre-delivery checklist completed and its output shown | T8 | `design-system/bcem-energy/checklists/f5-predelivery.md` plus the recorded verification output |

### Scope coverage (09 §F5 bullets)

Skill + `MASTER.md` → T1 Step 1 · tokens into Tailwind, no raw hex → T1 · subset fonts → T1 Step 6 · 07 §6 inventory on Radix → T2 (Button, IconButton, Tooltip), T3 (forms), T4 (containers, overlays, feedback, Breadcrumb, Pagination, Table), T6 (shell), T7 (domain, Map) · chart wrappers over one library → T5 · shell with groups, disabled entries, top bar, breadcrumb, page header, sticky filter bar, customiser → T6 · `next-intl` tr/en seeded from the appendix → T1 mechanism (D7, D8), namespaces T2–T7 · Storybook in both themes and locales → T2 · contrast check in CI → T1 script + T2 CI step · i18n parity in CI → T1 (extended) + existing CI step.

### Type-consistency check (names used across tasks)

`Unit`, `formatNumber`, `formatQuantity`, `unitSymbol`, `formatDate`, `formatMonth` (T1 → T3, T4, T5, T6, T7) · `renderWithProviders`, `expectNoAxeViolations`, `setMatchMedia`, `setMockPathname`, `mockRouter` (T2 → all) · `Button`, `IconButton`, `Tooltip`, `Popover` (T2 → T3–T7, I-6) · `Option`, `FieldProps` (T3) · `DataTable`, `getExportRows`, `toCsv`, `useAnnounce`, `Drawer`, `BreadcrumbItem`, `DropdownMenuItem` (T4 → T5–T7, M-4) · `ChartSeries`, `SeriesKind`, `BaseChartProps`, `chartAnimation`, `usePrefersReducedMotion` (T5) · `UiPreferences`, `useUiPreferences`, `parseUiPreferences`, `htmlAttributes`, `UI_COOKIE`, `navigation`, `setLocale` (T6) · `InvoiceLineView`, `DataQuality`, `MapMarker`, `Granularity` (T7, M-4). Every name appears with one spelling.

### Known risks, stated rather than hidden

1. **The spec contradicts itself in three places**: its light palette fails its own contrast rule (D3, Q4), the skill's `MASTER.md` contradicts 07 wholesale (D1; a future phase reading only `MASTER.md` would build slate glassmorphism in Fira), and the appendix catalogue is not importable (D7; every later UI phase reconciles its namespace).
2. **Toolchain compatibility is unverified**: Storybook 10.6 nextjs-vite with Next 15.5, vite 8 and Tailwind 4.3. If `storybook build` cannot process Tailwind v4 through PostCSS, add `@tailwindcss/vite` in `viteFinal` (T2) and record it. If the Playwright 1.63 browser revision differs from `chromium-1243`, `playwright install chromium` downloads about 150 MB.
3. **`light-dark()` raises the browser floor** to Safari 17.5 / Chrome 123 / Firefox 120 (D4). No supported-browser matrix exists in the spec; T8's handoff records it for F15 and the PO (M-18).
4. **Glyph coverage for ₺ and ₂** in Lexend and JetBrains Mono is unknown until T1 Step 6. Missing glyphs fall back to the system font. The T1 report states which.
5. **Sweep cost**: the plan's own story lists add up to roughly 250–300 stories across T3–T7, at least 4× that in raw axe page loads if every story ran the full theme×locale matrix. I-18 scopes the full 2×2 to each component's first story and every `open`-tagged story, with other stories running only the `light/tr`/`dark/en` diagonal — expect 15–25 min at 2 workers for the full sweep (T8, CI). Scoped `STORIES=` runs, plus `--last-failed` on fix re-runs, keep per-task gates and fix rounds short.
6. **Recharts in jsdom** needs `initialDimension` and a `ResizeObserver` stub. SVG-level assertions stay minimal. Behaviour is asserted through `_theme.ts`, the frame, the legend, the tooltip and the table.
