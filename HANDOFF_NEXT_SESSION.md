# Handoff — ekokod rewrite: F21 audit fixes merged into `main` and deployed

> **STATUS (2026-10-02 17:15):** F21 audited every main feature against the real PM5340 gateway and
> through the web UI, and fixed what it found (R498–R514 + web fixes). Plan and rulings:
> `docs/superpowers/plans/2026-10-02-f21-audit-fixes.md`. At the user's request the branch
> `fix/f21-audit` was merged into `main` (**`de2dfeb`**) and deployed with `deploy-main.sh`:
> new.ekokod.com runs `de2dfeb`, migrations 23 → 25 applied, every container healthy, no error in the
> first minutes of worker/scheduler/api logs. Rollback material: `/opt/ekokod-new.prev-20261002-171105`
> and `/root/ekokod-new-pre-de2dfeb-20261002-171105.dump`. **Nothing is pushed to a remote.**
>
> **After the deploy, on the server:** each linked iSolar plant pulls its history on its first sync;
> the PM5340 history is pulled from Ayarlar › Şirket › entegrasyon › "Geçmiş veriyi çek". Neither was
> watched to completion on the server.
>
> **Open, for the product owner to decide** (see the plan's "Known, not changed"): a year's figure
> includes silent gaps; the power alarm reads PM5340's peak-demand register (as legacy did); iSolar
> changes could not be run live (no credentials); reading the server's `job_runs` was not permitted
> in the audit session, so the server-side errors were reproduced locally instead.

---

## Paste this as the first message of the next session (only when there is new input)

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/`
> içinde. F0–F21 `main`'de (`/home/personal/ekokod-f17-ekocm` worktree'si `main`'i tutuyor). **F21 (gerçek
> veriyle özellik denetimi ve düzeltmeler) merge edildi ve new.ekokod.com'a deploy edildi (`de2dfeb`).**
>
> Önce `/home/personal/ekokod-f17-ekocm/HANDOFF_NEXT_SESSION.md` ve
> `docs/superpowers/plans/2026-10-02-f21-audit-fixes.md`'yi oku. Sonra şu yeni girdiyle çalış:
> **<buraya ne getirdiğini yaz — örn. "sunucuda geçmiş veriyi çektim, şunu gördüm: …">**.
>
> **Çalışma düzeni (kesin):** paralel agent YOK, subagent YOK, Workflow YOK. İşi bu oturumda kendin
> yürüt (`superpowers:executing-plans` + TDD). Her task sonunda diff'i oku, guard'ı kendi mutasyonunla
> kırmızıya düşür. Push, merge, `main`, Docker restart ve başka projelerin container/port/süreçleri YOK.
>
> **Ortam:** her komutun başına `export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
> Docker: `sg docker -c /yol/betik.sh` (betik dosyası yaz). **Asla `make test` / `go test ./...`.**
> Integration: `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <paket> -tags=integration -count=1 -parallel 4`.
> Web: `pnpm test --maxWorkers=2`; `next build` için `NODE_OPTIONS=--max-old-space-size=4096`, ardından `pnpm check:module-chunks`;
> Playwright ön planda, parçalar halinde (araç zaman aşımı 10 dk), `sg docker` içinde
> `PLAYWRIGHT_BROWSERS_PATH=/home/personal/.cache/ms-playwright E2E_API_PORT=18082 PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1 <dosyalar>`.

---

## F21 — what changed (2026-10-02)

Rulings R498–R514 and the web fixes are listed in `docs/superpowers/plans/2026-10-02-f21-audit-fixes.md`.
- **Consumption**: migration 00024 (null-safe hourly/daily aggregates; monthly/yearly dropped, rolled up
  from daily at query time); every fetch refreshes its own range.
- **Bills**: a day without an active index is a missing day (January was failing outright); a building
  with only a building bill appears on the dashboard; invoice PDF names its registers.
- **Ingest**: `last_reading_at` after a backfill, honest run times, deadlock retry, a sanity rule that no
  longer rejects ordinary load, Turkish operator messages, the PM5340 definition in the seed.
- **iSolar**: migration 00025 (`isolar_history_through`), history on any sync, midnight baseline.
- **Alarms**: the dialog could not save a reactive rule; log dialog and Messages now say what fired.
- **Web**: finished jobs refetch the screen (`useJob`), verification refreshes the credential list,
  dashboard/messages/chart polish.
- Gates: `make lint` 0 issues, `check-generate`, `check-api-docs`, every touched Go package with
  `-tags=integration`, web lint + typecheck + i18n parity + vitest 1260+, `next build`, module chunks,
  **e2e green** in four foreground chunks (one stale assertion fixed: "Platforma Git" leads to `/modules`
  since F17). A lone FAIL in a large Go chunk is the testcontainers Redis readiness timeout under load.
- How it was tested: a local `ekokod api` + `worker` (no scheduler) on a scratch database in the shared
  test Postgres (`ekokod-test-pg`, port 55432) and Redis db 5/6, `next dev -p 13100`, a PM5340
  credential pointing at the real gateway, then `job-runs/alarm.evaluate/trigger?company_id=…` as
  the platform admin for the scheduled path. Playwright scripts drove the UI; nothing of that is in the repo.
- Lessons: check `git status` before `git add -A` (two stray log files had to be filtered out of three
  commits); never drop a continuous aggregate while its refresh job runs (TimescaleDB 2.30 segfaults).

## F17 — what changed (2026-09-26)

User decisions U1–U4, rulings R485–R497: `docs/superpowers/plans/2026-09-26-f17-ekocm-module.md`. ADR 0009.
- `web/src/modules/`: `registry.ts` (MODULES), `ids.ts`, `ekorm/nav.ts`, `ekocm/{nav,sections,metadata}.ts`,
  `access.ts` (`modulesFor(me)` — frontend-only hook, everyone gets both, U2), `module-layout.tsx`, `module-chooser.tsx`,
  `redirects.ts`, `feature-owners.json`, `boundary.test.ts`.
- `/ekocm` + 10 section pages (`features/carbon/section-page.tsx`); `/modules` chooser is the default post-login
  target (U3); `ModuleSwitcher` in both top bars; `safeNext` accepts `/modules`, `/ekorm`, `/ekocm`.
- Middleware 308s: `/ekorm/carbon-footprint?tab=x` → its `/ekocm` page, legacy root paths → `/ekorm/…`,
  `/carbon-footprint` → `/ekocm`, `/ekocm/auth/*` → `/auth/login?next=/ekocm`.
- Boundary guards: ESLint patterns, import-walk vitest, `pnpm check:module-chunks` (CI + `make web-build`).
- Backend: only `GridFactorKey` consolidated in `domain/carbon` (R497). The yearly report's carbon summary stays in EKORM (U1).
- Gates: lint, typecheck, check:api, i18n, contrast, vitest 1218/1218, `next build`, module chunks, Go touched packages
  (unit + integration) + golangci-lint 0, `internal/arch`, **e2e 182/182** in four foreground chunks.
- Fixed two e2e drifts that had been in `main` since `c836484` (F16 UI polish): the sidebar labels now ignore the
  aria-hidden "Yakında" badge, and the job-trigger button has its readable name.

## Before F17 — START HERE — exact state (2026-09-24 ~18:00)

| Branch / worktree | State |
|---|---|
| `phase/f9-solar` … `phase/f14-migration` (`/home/personal/ekokod-f{9..14}-phase`) | done; each later branch carries the earlier ones |
| **`phase/f15-hardening`** (`/home/personal/ekokod-f15-phase`) | **F15a + F15b + F15q + F15c done**; tip = this commit |

### What this session did
1. **F15q** (plan `docs/superpowers/plans/2026-09-24-f15q-boundary-differencing.md`, R470–R474):
   migration 00021 rebuilds the four consumption aggregates with `first_ts`, a `first()` per register
   and a `last()` per export register. `AnalyticsRepository` reads one bucket each side and
   differences consecutive boundaries (`internal/store/postgres/boundary.go`), so every consumer
   (analytics series, carbon, forecast, renewable, load profile) is corrected. F3's "differ by the step"
   criterion is re-ruled to "agree" (R472; 09 §F3 and 04 §4.3 amended). api/v1 expectations 230→240, 920→960.
2. **F15c** (plan `…-f15c-packaging-handover.md`, R475–R484):
   - `scripts/{backup,restore,test-restore}.sh`, `make backup|restore|test-restore`. test-restore restores
     onto a clean TimescaleDB container: row counts equal, API ready, signed-in load run passes.
   - Compose: `ekokod-ml:`/`ekokod-web:` tags, overridable host ports, metrics on `0.0.0.0:946x`;
     **postgres healthcheck over TCP** (a socket check passed during the image's init server → fresh
     installs failed at migrate — found by the install test). `install.sh` upgrade path (backup first,
     `--pull never --no-build`). `scripts/test-offline-install.sh`: fresh install → cross-version upgrade →
     no data loss → pre-upgrade backup restored in compose mode.
   - `ekokod tool openapi --format markdown` → `docs/api/reference.md`; `make api-docs`, `check-api-docs` (in `ci`).
   - a11y: `tests/e2e/a11y-pages.spec.ts` (60 page×role checks, 0 findings); Storybook sweep 2,730 + 1,186
     passed. **Fixed:** `/auth/error` and `/auth/maintenance` crashed server-side (`buttonVariants` from a
     client module → `button-variants.ts`); the AI page lost its heading for read-only users.
   - Docs: `docs/runbook-operator.md`, `docs/admin-guide.md` (TR), `docs/adr/0001–0008`, `docs/accessibility.md`,
     README; security review / migration runbook / spec README brought current.
3. **Gates:** golangci-lint `./...` 0; vet clean; govulncheck nothing reachable; web 1162 unit tests,
   typecheck, lint, check:api, i18n, contrast; **full e2e with the worker 174/174**.
   Ledgers: `.superpowers/sdd/2026-09-24-f15{q,c}-…/progress.md` (git-ignored).
4. Local leftovers (ours, safe to delete): `dist/ekokod-offline-f15c-test{,2}.tar.gz` (~800 MB each),
   images `ekokod:f15c-test*`, `ekokod-ml:f15c-test*`, `ekokod-web:f15c-test*`, `bin/`.

### PENDING — needs something this machine does not have
See **`SENDEN-GEREKENLER.md`**: production dump + host (three rehearsals, rollback), live providers,
real invoices (F4), restore on a physical second host, a networking-off install, 24 h soak,
screen-reader passes, ML dependency audit on a connected machine.

## Open questions (defaults shipped)
- **F15q** R470–R474 (the PO should confirm R472), **F15c** R475–R484 (R484: admin guide in Turkish).
- **F15a** Q-L1…Q-L4, **F15b** Q-M1…Q-M4.
- **F14b** Q-J7…Q-J15 and **F14c** Q-K1…Q-K7 — first: Q-J7, Q-J9, Q-J12, Q-K5.
- **Unchanged:** Q-E1…Q-E7, Q-D1…Q-D7, Q-C1…Q-C7, Q-B1…Q-B4, Q-A1…Q-A5, F5 Q1–Q7, F10–F13's lists.

## Binding rulings
F1 1–14, F2 R1–R53, F3 R54–R104, F4 R105–R135, F5 D1–D26, F6a R136–R189, F6b R190–R210,
F7 R211–R232, F8a R233–R253, F8b R254–R275 + E-1…E-3, F9 R276–R299 + F-1…F-3, F10 R300–R327,
F11 R330–R346, F12 R350–R356, F13 R360–R385, F14a R400–R412, F14b R413–R429, F14c R430–R444,
F15a R450–R456, F15b R460–R469, **F15q R470–R474, F15c R475–R484**. New work continues from R490.

## Process rules that paid off
- **Run the real stack with a worker before calling a phase done.** This session's first full e2e run
  found two product defects and seven drifted specs that unit tests could not see.
- **A per-task gate is the whole touched package, including `internal/cli`.**
- **Prove every guard red with your own mutation,** restoring from a file backup. When a mutation shows
  nothing, check that it compiled (twice this session a mutation did not compile, and once a fixture had
  no case to exercise it).
- **When code came before its test,** say so and prove the test by mutation.
- **Idempotency tests compare bytes:** a `bson.M` marshals in random key order — use sorted JSON.
- **Nullable DTO fields are optional:** no `required:"true"` on pointers. **ICU:** double `''` before `{`.

## Environment surprises worth keeping
- Docker was hung ~00:25–09:55 and came back after a restart: the test containers were `Exited` and the
  test Postgres was empty (tmpfs). Start them; migrations run per test DB.
- Another user's `go test ./...` and testcontainers run on this Docker: a `docker.sock … context deadline
  exceeded` in a container-owning package is load, not code — rerun the package alone.
- This worktree does not record file modes: a new `scripts/*.sh` needs `git update-index --chmod=+x`
  (`make check-script-modes`).
- `golangci-lint` wants `gofmt -s` (composite literal simplification), not just `gofmt`.
- Each new worktree needs `git config --global --add safe.directory <path>` (done for f15).
- The shims `/tmp/claude-1003/bin/{pgq,psql,redis-cli}` let scripts run without the Docker CLI;
  rebuild `pgq` from `.superpowers/sdd/2026-09-23-f9-solar-renewable-financial/shim/pgq` if `/tmp` was cleaned.
- `pdftoppm` is missing; the Read tool opens PDFs.
- `shellcheck` is not installed; CI runs it. A static binary unpacked to `/tmp/claude-1003/shellcheck-v0.10.0/` works
  (GitHub release v0.10.0). The Bash tool times out at 10 min: chunk Playwright runs (the Storybook sweep
  by title prefix, `--grep 'Features/[A-C]'` etc.).
- `a11y-pages.spec` writes every finding to `web/a11y-results/` (Playwright wipes `test-results/`).

## Key paths (F14)
```
internal/migrate/legacy/        inventory, extract, transform_*.go, load.go, artifacts.go, reconcile*.go, answers.go
internal/migrate/recompute/     recompute orchestration
internal/store/postgres/admin/  legacyload.go (loader), reconcile.go (read-only queries), aggregates.go (+plant refresh)
internal/cli/                   migrate_legacy.go, recompute.go
internal/store/postgres/migrations/00020_legacy.sql
docs/runbook-migration.md, scripts/migration-rehearsal.sh, make migration-rehearsal
```
