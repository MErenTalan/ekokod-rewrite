# F17 — EKOCM as its own frontend module

**Goal:** the carbon module leaves `/ekorm/carbon-footprint` (one page, 11 tabs) and becomes a sibling
module at `/ekocm/*` with its own layout, sidebar, brand accent and pages. Backend, API (`/api/v1/carbon/*`),
auth, session, roles and the public site stay shared and unchanged. Overrides F10b Q-F8 / R320.

**Working mode:** single session, inline, TDD, commit per task (memory: single-session-per-phase).
Env: `export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
Web gate per task: `pnpm typecheck && pnpm lint && pnpm test --maxWorkers=2` (+ `check:i18n-parity`, `check:contrast`
when messages/tokens change). `next build` with `NODE_OPTIONS=--max-old-space-size=4096` at tasks 3, 5 and 8.

## User decisions (2026-09-26)

| Id | Decision |
|---|---|
| U1 | Yearly report carbon summary stays in EKORM (consumption × catalogue grid factor). |
| U2 | Module entitlement: frontend hook only (`modulesFor(me)` → both); no backend field. |
| U3 | Post-login default (no `next`) is the `/modules` chooser; one module → skip straight to it. |
| U4 | EKOCM: own sidebar + one real page per former tab; old `?tab=` links 308-redirect. |

## Rulings (R485–R497)

| Id | Ruling | Cost if wrong |
|---|---|---|
| R485 | `web/src/modules/registry.ts` holds `MODULES = { ekorm, ekocm }`: `id`, `prefix`, `home`, `nav`, `brand` (icon, name key, accent). `moduleFor(pathname)` resolves by prefix. | Low |
| R486 | EKORM nav moves `components/shell/nav-config.ts` → `modules/ekorm/nav.ts`; EKOCM nav in `modules/ekocm/nav.ts`. Generic helpers (`isGroup`, `isActive`, `findTrail(entries, path)`, `navFor(entries, perms)`) stay in `components/shell/nav.ts`. The `carbon` leaf leaves EKORM's nav. | Low |
| R487 | Nav labels are full message paths (`shell.nav.bills`, `ekocm.nav.overview`), so each module owns its label catalogue; new `messages/{tr,en}/ekocm.json` (nav, brand, chooser). | Low |
| R488 | EKOCM routes: `/ekocm` overview; `/ekocm/activity-selection`, `/data-entry`, `/activity-status`, `/reports`, `/emission-factors`, `/company`, `/ghg-protocol`, `/iso-14064`, `/standards`, `/documents`. Sidebar groups: Data (selection, entry, status), Library (emission factors, company), Reference (ghg, iso, standards, documents). Every leaf needs `carbon.read`. | Low |
| R489 | `AppShell` takes `module: ModuleId`; logo, home link, nav and `data-module` come from the registry. Accent: `[data-module='ekocm']` rebinds `--color-brand` to a new `--color-module-ekocm` token (in contrast pairs). Primary/buttons stay platform green. | Low |
| R490 | `app/ekocm/layout.tsx` = same session guard as EKORM + `AppShell module="ekocm"`. Shared: root providers, `SessionProvider` (session, selection, preference sync). No EKORM feature imports under `app/ekocm/**` or `features/carbon/**`, and none of `features/carbon` under `app/ekorm/**` — enforced by ESLint `no-restricted-imports` overrides. | Med: the "EKORM providers not in EKOCM" claim rests on it |
| R491 | Layout gate: a module the user lacks (`modulesFor(me)`) redirects to `/modules`. Today everyone has both (U2). | Low |
| R492 | `safeNext` accepts `/modules` and any module prefix (`/ekorm`, `/ekocm`, with `/` or `?` after); default target is `/modules` (`DEFAULT_PATH`). Open-redirect regex kept. | High: open redirect |
| R493 | Middleware 308s (before the auth check, like `/site/*`): `/ekorm/carbon-footprint[?tab=x]` → EKOCM page for `x` (unknown → `/ekocm`), other query params kept; legacy root paths (`/consumption`, `/load-profile`, `/predict`, `/ai`, `/solar-plants`, `/renewable-energy`, `/financial-analysis`, `/bills`, `/bills-and-tariffs`→`/ekorm/bills`, `/tariffs`, `/alarms`, `/messages`, `/reports`, `/settings[/account]`→`/ekorm/settings`, `/iso-50001`, `/calendar`, and their sub-paths) → `/ekorm/...`; `/carbon-footprint` → `/ekocm`; legacy `/ekocm/auth/login` → `/auth/login?next=/ekocm`. Public site paths win (`/contact` stays the site's). | Med |
| R494 | Unauthenticated `/ekorm/*`, `/ekocm/*`, `/modules` keep today's behaviour: refresh, else `/auth/login?next=<path>`. | Low |
| R495 | `/modules` chooser: protected page, one card per `modulesFor(me)` entry; a single module redirects straight to its home. Site header "Platform" link, maintenance retry and login default go to `/modules`. | Low |
| R496 | `ModuleSwitcher` in the top bar of both modules: links to the other module's home (no re-login; same cookies). Hidden when the user has one module. | Low |
| R497 | `GridFactorKey` lives once in `domain/carbon`; `service/assets` and `service/renewable` reference it. | Low |

## Tasks

1. **Nav/registry refactor (no behaviour change)** — R485–R487 for EKORM only; tests: existing nav-config/sidebar/page-header/top-nav tests pass against moved modules; `findTrail`/`navFor` take entries.
2. **EKOCM skeleton** — `ekocm.json` (tr/en), `modules/ekocm/nav.ts`, `app/ekocm/layout.tsx`, brand token + contrast pair, `AppShell module` prop, `data-module`. Tests: EKOCM layout renders EKOCM nav only (no `Tüketim`), brand label; layout redirects without session; nav permission filter.
3. **Pages split** — `features/carbon/carbon-section.tsx` (replaces `carbon-page.tsx`): permission gate, building requirement, scope picker, `go(tab)` → `router.push(href)`; one `page.tsx` per route; remove `/ekorm/carbon-footprint` page and the EKORM `carbon` leaf. Tests: section map, go() targets, no-access state.
4. **Import boundary** — ESLint overrides (R490) + a vitest that walks `app/ekocm` and `features/carbon` imports (mutation-proved).
5. **Middleware + safeNext** — R492–R494 with `middleware.test.ts` cases (each redirect, query kept, unknown tab, `/contact` untouched, unauth `/ekocm` → login with next, `safeNext` open-redirect cases).
6. **Chooser, switcher, login target** — R491, R495, R496; `modules/access.ts` `modulesFor(me)`; tests for chooser (two cards / single-module redirect), switcher link, login default.
7. **Backend tidy** — R497; `go test ./internal/service/assets ./internal/service/renewable ./internal/domain/carbon` (+integration tags), golangci-lint on those.
8. **i18n + e2e + docs** — remove `shell.nav.carbonFootprint`; e2e: `carbon.spec` on new URLs, redirect spec, unauth-both-modules, switch without login; a11y-pages list; spec 01 §1/§7.16, legacy-inventory, 10-removed §29 note; handoff.

## Final verification (user's list)
- Unauthenticated `/ekorm/*` and `/ekocm/*` → login (e2e + middleware unit).
- One login → `/modules` → EKORM → switcher → EKOCM with no second login (e2e).
- EKOCM pages load no EKORM feature code (lint rule + import-walk test + build manifest check that `/ekocm` chunks hold no `features/consumption|bills|alarms…`).
- `/api/v1/carbon/*` role checks: existing Go handler tests (`carbon.read` all roles, `carbon.edit` A/CA) run and pass unchanged.
