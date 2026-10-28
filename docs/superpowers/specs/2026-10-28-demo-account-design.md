# F29 — Demo account (design)

Date: 2026-10-28. Branch `feat/f29-demo-account` (worktree `/home/personal/ekokod-f29-demo`, from `main` @8415257).

## Intent

The owner hands one demo login to prospective customers, who browse the panel on their own, and uses the same
login to record product videos. The account must show **every customer-facing page and feature, filled with
believable mock data**, let the visitor **actually do things** (create, edit, delete), and never touch real data,
real settings or anything outside the platform.

## Decided by the owner (2026-10-28)

- One shared demo login. Two prospects at the same time seeing each other's changes is fine.
- The visitor can write. Everything they change is temporary: the demo company returns to its initial state
  **every hour on the hour** (Europe/Istanbul), and the panel says so on every page.
- The demo lives in the same database as a separate company **tagged as demo**; it must not affect real data.
- Everything a customer would see is open: all of EKORM and EKOCM.
- Not in this work: opening new.ekokod.com to the public (the IP allowlist stays; M-01 and the owner's own
  security tests come first).
- At the end, explain to the owner how to get the demo login details.

## Goals

1. **Full surface.** The demo user sees what a company admin sees: every EKORM page (dashboard, consumption,
   load profile, bills, tariffs, financial analysis, solar plants, renewable energy, forecast/AI, alarms,
   messages, reports, ISO 50001, calendar, contact, settings tabs a company admin has) and every EKOCM page.
   No page or tab is empty.
2. **Writable, temporary.** Every write a company admin may do works for the demo user, except the four
   protections below. A reset converges the demo company back to its baseline.
3. **Contained.** No demo action reaches outside the platform: no e-mail, no SMS, no integration HTTP call
   (OSOS, GridBox, PM5340, iSolar, Aril…), whatever the visitor types into a form. No demo action or reset
   reads or writes a row of another company or of the platform catalogues.
4. **Honest.** A banner on every app page (EKORM, EKOCM, `/modules`): this is a demo, data is sample data,
   changes are not kept, next reset at HH:00.

## Non-goals

- A private sandbox per visitor; a public "try the demo" button; self-service demo sign-up.
- Platform-admin screens (all companies, integration definitions, SMTP, national defaults, job triggering).
- Faking platform-level data (EPİAŞ market prices, national tariff schedule, emission factor master catalogue,
  weather). The demo uses what the platform has; its tariffs use fixed prices so bills never depend on PTF.
- Removing the IP allowlist or the M-01 upgrades.

## Anti-goals (a failure even if it "works")

- A demo write or reset changes a row of a real company, or a platform catalogue row.
- A prospect sends real e-mail (alarm recipients, report e-mail) or makes the server call a URL they typed.
- One prospect locks everyone else out (changed password/e-mail, deactivated or demoted demo user, revoked
  sessions).
- A page in the demo is empty or errors.
- The hourly reset makes the server visibly slow or recomputes aggregates for other companies' data.

## How it works

### 1. The demo tag

Migration 00027 adds `companies.is_demo boolean not null default false`. `seed demo` sets it on the demo
company (fixed id `seed.DemoCompanyID`, unchanged). The tag is the single source of truth; code asks a small
cached checker ("is this company a demo company?") rather than comparing ids. Platform admins see the tag in
the companies list (a "Demo" chip) — *my call*.

### 2. The demo user's permissions

The `demo` role stops being read-only and holds exactly the company-admin (CA) permission set, scoped to its
own company as today. It keeps four protections (all enforced in the API, not only the UI):

- it cannot change its own name/e-mail/phone/password, cannot request a password reset (already so);
- nobody but a platform admin can edit, deactivate, demote or delete a `demo`-role user;
- it cannot list or revoke sessions (other prospects share them);
- the role cannot be assigned through the users screen (already so).

Users the visitor creates inside a demo company appear in the users list but **cannot log in** — the only
way into a demo company is the demo user (*my call*; one entry point to audit).

### 3. Containment (no outbound side effects)

Two choke points, both keyed on the company's demo tag:

- **Mail.** A decorator around `mail.Sender` drops every message whose SMTP settings belong to a demo company
  and reports success. The demo company gets seeded SMTP settings (fake host `smtp.demo.invalid`) so report
  e-mail, alarm notifications and plant-fault mail run their normal path and show success.
- **Integrations.** For a demo company, credential verification succeeds without a call; manual and scheduled
  fetch, history pull (backfill), analyzer refresh and iSolar sync/link finish as "succeeded, nothing new"
  without resolving an adapter. The scheduled fan-outs (`sync.dispatch`, solar dispatch/alarms) skip demo
  companies. Seeded credentials (OSOS Başkent, GridBox) show as verified with recent sync times.

Internal services stay real: billing engine, report rendering, alarm evaluation, carbon accrual, the ML
forecast service. They only read and write the demo company's own rows.

### 4. Baseline and hourly reset

The baseline is a deterministic generator (fixed uuid namespace, as today) relative to "now", so the data stays
current: last month's bill appears when a month turns, alarm events are "2 hours ago", readings run to the
current hour.

A reset (`demo.reset` task, cron `0 * * * *` Europe/Istanbul; also `ekokod seed demo --reset` for an
immediate one before a video) runs in **one transaction**:

1. Kept rows (time series and the rows they hang on) are restored, not deleted: the company row, the baseline
   buildings, analyzers, power plants and plant devices are updated back to baseline values (clearing
   `deleted_at`), the demo user is restored except its password. Their readings, plant production,
   provider hourly values, forecasts, cursors and anchors are left alone.
2. Every other demo-company row is deleted (FK-safe order), including rows the visitor created on kept
   parents; then the generator re-inserts the non-time-series baseline.
3. Rows hanging on non-baseline analyzers/plants (only possible if the visitor created one) are deleted
   including their time series; only then are the consumption aggregates refreshed over the affected range.
4. Files the visitor uploaded (stored files of the demo company not in the baseline) are removed from disk
   after commit.

Soft deletes make step 1 safe: deleting a building/analyzer/plant in the UI only sets `deleted_at`, and
`meter_readings → analyzers` has no cascade.

A schema guard test lists every table that reaches `companies` (directly or through a FK chain) and fails
when one is neither reset nor declared kept, so a future table cannot leak demo writes past the reset.

### 5. The banner

`GET /auth/me` gains `demo: { resets_at }` (null for non-demo companies). The app shell shows a slim,
non-dismissable banner above the content on every app page:

> **Demo hesabı** — Bu hesaptaki veriler örnektir. Yaptığınız değişiklikler kalıcı değildir; hesap her saat
> başı ilk haline döner. Sonraki sıfırlama: **15:00**

(English: "Demo account — The data in this account is sample data. Your changes are not kept; the account
returns to its initial state every hour on the hour. Next reset: 15:00.") The profile tab says why profile and
password fields are locked.

### 6. The mock dataset (*my call*, the owner can change names)

"Demo Enerji A.Ş." (sector Üretim, `demo@ekokod.com.tr`, unchanged), 13 months of history so yearly views,
ISO 50001 baselines and the financial comparison are full:

- **Buildings (3):** Ostim Fabrika (Ankara, OG, binomial, multi-time), Merkez Ofis (Ankara Çankaya, AG,
  single-time), Gebze Lojistik Depo (Kocaeli, prosumer with rooftop solar). Map coordinates real, addresses
  generic.
- **Analyzers (6):** factory main transformer, production line, compressor room; office main; warehouse main
  (with export register for netting); a cold-room sub-meter. Hourly readings with distinct curves,
  weekday/weekend/vacation shape, inductive/capacitive shares (one analyzer occasionally over the reactive
  limit so the reactive warning shows).
- **Solar (2 plants):** Gebze rooftop GES (netted against the warehouse meter) and Konya field GES (1 MWp,
  inverters, monthly targets, solar tariff, fault history). Hourly production with a seasonal solar curve.
- **Tariffs & bills:** a tariff per building (+ taxes, extra charges), one template and one bulk assignment, an
  icmal import; 12 months of building and analyzer bills computed **by the real billing engine** from the
  readings.
- **Alarms & messages:** five rules (power, daily consumption, reactive ratio, bill amount, no data), ~40 events
  over 30 days, operational messages, job-run history.
- **Reports:** monthly reports for the last three months per building and last year's yearly report.
- **Forecast:** synthetic forecasts for the past 30 and next 7 days so the pages are never empty when the ML
  service is down; real runs overwrite them.
- **ISO 50001:** a project for the factory with clause dates in progress, notes, a few documents.
- **EKOCM:** company profile, selected activities across scopes 1–3, 12 months of activity data, a carbon
  report, documents.
- **Calendar:** weekend days, 2026 public holidays as vacations, a few events (bakım günü, denetim).
- **Users:** the demo user plus four sample colleagues (building admin, read-only roles) who cannot log in.
- **Integrations tab:** two seeded, verified credentials (OSOS Başkent, GridBox) with fake secrets.

### 7. Getting the login (for the owner)

`ekokod seed demo` (needs `EKOKOD_DEMO_PASSWORD` on first run) creates or converges the demo company;
`--reset` resets now; `--password` rotates the demo password. On the server a script runs it inside the api
container with a generated password and writes the login to `/root/ekokod-new-demo.txt` (mode 600), next to
the admin file. Deploying is the owner's call (merge to `main`, `deploy-main.sh`).

## Constraints

- Go backend, Next.js web, Postgres+Timescale, asynq worker/scheduler, as today; no new services.
- Existing contracts stay: `/api/v1` shapes only gain fields; OpenAPI + generated client regenerated.
- Tenancy rules (ResolveScope, FK body-id checks from F28) apply to the demo user like to any CA.
- Reset must not refresh continuous aggregates for ranges it did not change, and never drops/creates views
  (TimescaleDB 2.30 segfault on drop-under-refresh).

## Settled during the builder read (*my calls*)

- A login by a visitor-created user of a demo company gets the same answer as a wrong password.
- iSolar "connect" in a demo company does not redirect to iSolar; it answers as linked with the seeded plants.
- An already-seeded old demo (2 analyzers, 180 days) is converged, not recreated: same ids where they exist,
  and the generator fills any missing hour in the whole 13-month window (backwards too), not only forward.
- Mock data is Turkish; the UI language switch still works (labels translate, names stay).

## Left to the builder

- Exact table order for the reset; how the cached demo checker is invalidated (a short TTL is fine).
- How much of the e2e fixture code (`internal/seed/{tariffs,solar,carbon,alarms,reports,iso50001}.go`) is
  generalised versus written fresh for the demo.
- Exact values and curves of the mock data, within "believable for a mid-size Turkish manufacturer".
- Banner styling within the existing shell components and theme tokens.

## Known and deferred

- **E-mail enumeration:** a visitor creating a user with an address already used by a real account gets
  "e-posta başka bir kullanıcıya ait" — the same answer any company admin gets today. Worth closing before the
  allowlist comes down (owner's security pass).
- The demo is reachable only from allow-listed IPs until the owner opens new.ekokod.com.
