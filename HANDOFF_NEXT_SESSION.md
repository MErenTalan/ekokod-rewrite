# Handoff — ekokod rewrite: F1–F5 + F6a COMPLETE → next phase F6b (core application screens)

> **STATUS (2026-09-17):** F6a (auth, RBAC, `/api/v1` HTTP layer, OpenAPI client, mobile auth, `/ekorm` shell, e2e
> harness) is finished on `phase/f6-core-screens` in `/home/personal/ekokod-f6-phase`. It started from
> `phase/f5-design-system` with `phase/f4-billing-engine` merged in (`24b53ca`). All 18 implementation tasks are merged
> `--no-ff`, each with its own mutation proofs, followed by one whole-phase self-review (authorisation/tenancy first) and an
> acceptance run.
> **F6 was split by the user (R136):** F6a = this phase; **F6b = the screens** (Dashboard, Consumption, Load Profile,
> Settings with 8 tabs, Calendar) plus their e2e specs. **F6b has no written plan yet.** The next session writes it first.
> **Working mode:** no parallel agents, no subagents, no Workflow. One session runs one phase inline, writes this handoff at
> the end, and the user runs `/clear`.
> **Nothing has been pushed and `main` is untouched. Pushing and merging to `main` are the user's call.**

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/` içinde, faz planı
> `docs/rewrite/09-implementation-plan.md` (16 faz, F0–F15).
>
> **Durum:** F1–F5 ve F6a tamam. F6 ikiye bölündü (R136): F6a (auth, RBAC, `/api/v1` HTTP katmanı, OpenAPI istemcisi,
> mobil auth, `/ekorm` iskeleti, e2e altyapısı) `phase/f6-core-screens` üzerinde (`/home/personal/ekokod-f6-phase`) bitti.
> **Sıradaki faz F6b (ekranlar: Dashboard, Tüketim, Yük Profili, Ayarlar 8 sekme, Takvim + e2e spec'leri) ve henüz planı
> yok.** F4'ün gerçek-fatura karşılaştırması ürün sahibinden fatura gelene kadar AÇIK. Hiçbir şey push edilmedi, `main`'e
> dokunulmadı; push ve merge kararı bana ait.
>
> **Çalışma düzeni (kesin):** paralel agent YOK, subagent YOK, Workflow YOK. Fazı bu oturumda tek başına yürüt
> (`superpowers:executing-plans` + TDD). Review'ları kendin yap: her task sonunda diff'i oku, kendi mutasyonunla
> guard'ın kırmızıya düştüğünü kanıtla; faz sonunda bir kez bütün-faz self-review (yetki/tenancy önce, sonra
> erişilebilirlik ve tasarım kuralları). Faz bitince `HANDOFF_NEXT_SESSION.md`'i F7 için yeniden yaz, bana yapıştırılacak
> promptu ver ve dur.
>
> Şu sırayla ilerle:
> 1. `/home/personal/ekokod-f6-phase/HANDOFF_NEXT_SESSION.md`'i baştan sona oku (START HERE, "What F6a built",
>    Deviations, "F6b must pick up", Open questions, Environment). F6a planını
>    (`docs/superpowers/plans/2026-09-17-f6a-auth-api-skeleton.md`, R136–R189 ve izin matrisi) ve F5 planındaki D1–D26'yı
>    referans olarak kullan.
> 2. F6b worktree'sini aç: `git worktree add -b phase/f6b-screens /home/personal/ekokod-f6b-phase phase/f6-core-screens`,
>    `web/` içinde `pnpm install --frozen-lockfile`.
> 3. **F6b planını yaz** (`superpowers:writing-plans`, `docs/superpowers/plans/` altına; sıkı: imzalar, kural tablosu,
>    isimli testler, doğrulama komutları). Kaynak: 09 §F6, `01-project-context.md` §2 rol matrisi, §5, §6, §7.2–§7.4,
>    §7.15, §7.18, `05-api-contract.md`, `07-design-system.md`, `design-system/bcem-energy/MASTER.md` sonra `OVERRIDES.md`.
>    Legacy dashboard/consumption/load-profile/settings/calendar ekranlarını ve gerçek veriyi plan öncesi oku; ekranların
>    ihtiyaç duyduğu ama F6a'da olmayan API'leri (ör. tarife şablonları, toplu tarife, bill dashboard) açıkça listele ve
>    F6b'de mi yoksa sonraki fazda mı yapılacağını bana sor. Bu handoff'taki "F6b must pick up" listesinin tamamı planda
>    karşılanmalı. Planı kendin bir kez spec + legacy'ye karşı gözden geçir, sonra yürüt.
>
> Ortam: her komuttan önce `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
> Sadece üzerinde çalıştığın paketleri/test hedeflerini koş, asla `make test` / `go test ./...` koşma. Integration testi
> için paylaşımlı Postgres ve Redis: `docker ps` ile `ekokod-test-pg` ve `ekokod-test-redis` açık mı bak (yoksa
> `make test-db-up` / `make test-redis-up`; Docker kapalıysa benden başlatmamı iste), sonra
> `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <paket> -tags=integration -count=1 -parallel 4`.
> Web: `pnpm test --maxWorkers=2`, Storybook/Playwright/`next build` tek tek; ağır işlerden önce `free -m` ve
> `ps -eo rss,comm | grep java` ile belleğe bak (bellek darsa Playwright `--workers=1`). e2e:
> `pnpm build && pnpm test:e2e --workers=1` (gerçek API'yi kendisi ayağa kaldırır). Başka projenin Gradle/java
> süreçlerine, 5432'deki `dolmusum-dev-postgres-1` container'ına ve 8080'deki `dolmusum serve`'e dokunma.

---

## START HERE — exact state

| Branch / worktree | Head | State |
|---|---|---|
| `phase/f1-data-model` (main checkout `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite`) | `10d6577` | F1 complete |
| `phase/f2-integration-layer` | `7dbe1ea` | F2 complete |
| `phase/f3-consumption-engine` (`/home/personal/ekokod-f3-phase`) | `58597ec` | F3 complete |
| `phase/f4-billing-engine` (`/home/personal/ekokod-f4-phase`) | `cf79346` | F4 complete; real-invoice comparison OPEN |
| `phase/f5-design-system` (`/home/personal/ekokod-f5-phase`) | F5 handoff commit | F5 complete |
| **`phase/f6-core-screens`** (`/home/personal/ekokod-f6-phase`) | this handoff commit | **F6a complete** (F4 merged at `24b53ca`; plan `fa18df2`; tasks 1–18 + phase review) |
| `f6a/task-1` … `f6a/task-19` | merged | local branches only; can be deleted |

Shared test containers (both tmpfs, recreate freely): `ekokod-test-pg` (TimescaleDB, :55432, `make test-db-up`) and
`ekokod-test-redis` (:56379, `make test-redis-up`). The e2e run creates and drops the `ekokod_e2e` database in
`ekokod-test-pg` and uses Redis DBs 6/7.

## What F6a built

### Go: auth, tenancy and the HTTP API
- **`internal/auth`** — `Hasher` (bcrypt over HMAC-pepper, `legacy$` rehash path, `DummyVerify` per configured cost),
  `CheckPolicy` (R146 codes), `Tokens` (HS256 access JWT: `sub`, `sid`, `cid`, `aud` web|mobile, `typ=access`,
  `iss=ekokod`), opaque refresh tokens (SHA-256 at rest), `Fingerprint` (HMAC of User-Agent), `RoleSet`/`PermissionsFor`
  (R159, **the** permission table; the web reads it through `/auth/me` and a Go-checked fixture).
- **Store** — migration `00015_auth_sessions.sql` (sessions: client, remember, last_used_at, revoked_reason, rotated_from;
  users: ui_preferences ≤ 512, locale; `password_reset_tokens`); session `Rotate`/`RevokeWithReason`/`Touch`;
  `PasswordResetRepository`; admin `ListCompanies`, `SectorFigures`, integration-definition CRUD.
- **`internal/mail`** — SMTP sender (implicit TLS or STARTTLS, verified certs, refuses plaintext auth) and tr/en templates
  (`password_reset`, `smtp_test`).
- **Services** — `service/auth` (login, refresh with rotation + 30 s grace + reuse detection, device binding, logout(-all),
  sessions, change/forgot/reset password, profile, `ResolveScope`), `service/tenancy` (companies, users with assignable
  roles, SMTP), `service/assets` (buildings, analyzers, plants, analyzer refresh, sectoral comparison),
  `service/analysis` (consumption rows/summary/export, anomalies, load profile, generation, energy balance, reactive
  status), `service/billing/requests.go` (compute, PDF, hourly XLSX), `service/calendar`, `service/integrations`
  (definitions), `credentials` (iSolar OAuth state), `seed` (`e2e` fixtures, synthetic `demo` company with an hourly job).
- **`internal/api/v1`** — **one route table** (`routes.go` + `handlers_*.go`, 86 operations) drives the router, role
  checks, OpenAPI 3.1 (`openapi.json`, committed, `make openapi`), idempotency (Redis) and audit. `kit/` (bind with
  unknown-parameter/-field rejection, error envelope with tr/en catalogue, opaque cursors, cookies), `dto/`, `mw/`
  (Authn, RequireRoles, Scope with admin `company_id`, BodyLimit, ContentType, per-principal and Redis auth rate limits,
  Idempotency, Auditor). `internal/apiwire.Build` wires everything for `ekokod api`. `ekokod user create`,
  `ekokod seed e2e`, `ekokod seed demo`, `ekokod tool openapi`.
- **Guards** — `TestAuthorizationMatrix` (every route × six roles from an independent copy of 05), `TestMatrixCoversEveryRoute`,
  `TestReadOnlyRolesForbiddenOnEveryMutation` (+ HTTP twin), `TestBuildingAdminCannotReachOtherBuildingsAnyEndpoint`
  (every `{id}` route from `Table()`, foreign ids per path family — an unmapped new family fails), `TestEveryMutationAudited`,
  `TestCredentialsNeverLeak`, `TestRouteTableMatchesRouter`, `TestOpenAPIDocumentIsCurrent`, `TestRequestTypesAreConsistent`,
  `TestPermissionEnumMatchesTable`, `TestEnumComponentsAreNotNullable`, `TestPermissionsFixtureMatchesTable`, `internal/arch`
  API guard, scope-isolation meta test (new repositories registered).

### Web
- **API client** (`web/src/lib/api/`) — `schema.d.ts` generated by openapi-typescript (`pnpm gen:api`, CI `pnpm check:api`);
  paths carry the `/api/v1` prefix (`api.GET('/api/v1/auth/me')`). `client.ts`: typed browser client with a
  **single-flight refresh** on 401 `token_expired|token_rotated|unauthorized` and one retry, `device_mismatch` →
  `/auth/login?reason=device_mismatch`, other ended sessions → `/auth/login?next=…`; public auth endpoints never refresh.
  `query.ts`: `$api` (openapi-react-query) + `makeQueryClient` (no retry on 4xx). `server.ts`: `serverApi()` (internal URL,
  forwards cookie/UA/XFF, app locale as Accept-Language), `getSession()`/`getMe()`. `errors.ts`: `safeNext` (post-login
  target must be `/ekorm…`, no dot segments), `authRedirectFor`.
- **Same-origin proxy** — `src/app/api/v1/[...path]/route.ts` proxies to `EKOKOD_INTERNAL_API_URL` **at runtime**
  (keeps every `Set-Cookie`, strips hop-by-hop/encoding headers, refuses dot segments, 502 envelope when down).
- **Middleware** (`src/middleware.ts`) — `/` → `/ekorm`; `/auth/*` public; missing `ekokod_at` with `ekokod_rt` →
  server-side refresh forwarding cookie/UA/Accept-Language/XFF, fresh cookies copied onto the response **and** the
  forwarded request; `device_mismatch` → login with reason; `token_rotated` → one retry of the same URL (10 s
  `ekokod_retry` cookie); anything else → `/auth/login?next=…`.
- **Auth pages** (`src/app/auth/*`, `src/features/auth/*`) — split layout (brand panel ≥ 768 px, language + theme controls),
  login (legacy labels, remember device, focus-managed errors, `Retry-After` minutes, adopts profile `ui_preferences` and
  locale), forgot password (identical confirmation), reset password (live R146 rule list mirroring `policy.go`, server codes
  mapped, spent-token path, `referrer: no-referrer`), auth error, maintenance, register → login. Namespace `auth` (tr/en).
- **`/ekorm` shell** — `src/app/ekorm/layout.tsx` (server `getSession`, redirect with reason) → `SessionProvider`
  (`useSession().can(permission)`, preference sync) → `AppShell` (user from session, `navFor(permissions)`, admin
  `CompanySwitcher`). `nav-config.ts`: `/ekorm/*` hrefs, Calendar after Reports, per-leaf `permission`
  (solar plants, financial; everything else `nav.core`), disabled placeholders stay visible. `lib/selection`:
  `useSelection()` per-user localStorage store (company switch clears building+analyzer; building switch clears analyzer),
  `useScopeParams()` → `{company_id}` for admins. `features/preferences/sync-preferences.ts`: debounced (500 ms)
  `PATCH /profile` of `ekokod_ui` + locale (not for demo). `UserMenu` logout (POST, clear queries, `/auth/login`).
- **e2e** — Playwright projects `a11y` and `e2e` (webServer chosen by `PW_PROJECT`); `web/scripts/e2e-api.sh` (fresh
  `ekokod_e2e` DB, fixtures, demo, `ekokod api` on :18080); `next start` on `http://localhost:13000` (localhost because the
  cookies are Secure). `tests/e2e/auth.spec.ts` (11 tests): six roles × exact navigation + switcher only for admin,
  remember-me cookie lifetime, device mismatch, logout, transparent refresh, forgot password. CI job `e2e` (TimescaleDB +
  Redis services, 30 min). `make web-e2e`.
- **Evidence ledger** (every mutation, invalid attempts and survivors):
  `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite/.superpowers/sdd/2026-09-17-f6a/inline-ledger.md` (git-ignored).

### Deviations from the F6a plan (all in the ledger)
- Access token also carries `cid` (company) so the session row can be loaded under a Scope; role/company still come from
  the DB on every request (R140 intent kept).
- Middleware package is `internal/api/v1/mw`; the OpenAPI document lives at `internal/api/v1/openapi.json` (go:embed).
- **R168:** `next.config` rewrites were **replaced by a runtime route-handler proxy** — the build baked
  `localhost:8080` into `routes-manifest.json` while docker-compose sets `EKOKOD_INTERNAL_API_URL=http://api:8080` at
  runtime. The browser client also refreshes on `unauthorized` (the access cookie's Max-Age equals the token TTL, so an
  idle tab sends no token at all).
- **R189 (new):** the iSolar OAuth callback is `/api/v1/integrations/isolar/callback` (worker updated).
- R166 exempt reasons use F4's codes (`term`, `user_group`, `generation`, `below_kw`); R176 endpoint validation relaxed to
  snake_case keys → non-empty strings (absolute `http://` refused).
- Demo seed refreshes only the hourly/daily continuous aggregates over [earliest − 48 h, end + 1 h).
- Company create/delete write platform audit rows.
- `@tanstack/react-query` **5.102.8**, not the planned 5.103.1: the latter was under a day old and pnpm 12's
  `minimumReleaseAge` refused it; pnpm's auto-added exemption was reverted rather than bypass the supply-chain guard.
- Login form fields carry no "(Zorunlu)" marker (both are always required on sign-in).
- **Phase-review fixes (on `f6a/task-19`):** enum OpenAPI components were nullable because PATCH DTOs point at them
  (`Me.role` typed `null|string` in TypeScript) → interceptor + test; the layout lost the `device_mismatch` reason when the
  access cookie was still present → `getSession` keeps the code; `safeNext` rejects dot segments; **client IP is now the
  rightmost untrusted `X-Forwarded-For` hop** (the leftmost entry is client-written once the web proxy sits between a front
  proxy and the API, which let per-IP auth limits be dodged); scheduler integration tests lacked `Schedule.Demo` (the
  Task 13 cron entry made the scheduler fail to start there); `password_reset_tokens.token_hash` added to the F1
  secret-column allow-list as a one-way hash; the HTTP test harness no longer sets `InsecureSkipVerify` (arch guard);
  `testfixtures.InsertReadings` refreshes `consumption_hourly/daily` over the inserted range, retrying on SQLSTATE 55P03 —
  a TimescaleDB policy job in the cloned test DB could move the watermark past historical rows and hide them (root cause of
  the intermittent `TestWeekendAndVacationChangeLoadProfileHTTP`); excelize 2.11.0 and moby/go-archive 0.3.0 for
  GO-2026-5960/GO-2026-6253.

## F6b must pick up
- **Branching:** start from `phase/f6-core-screens` (see the prompt).
- **Screens (R136):** Dashboard (01 §7.2: three-column grid + full-width chart, map with active/passive markers from R163,
  reactive status card from R166, latest bill card from `/bills/latest`, sectoral comparison from R162 with ranks and a CSV
  export), Consumption (every §7.3 column — the API's column set is pinned by `TestConsumptionColumnSet`; the eight
  seasonal labels — keys exist from T10), Load Profile (profiles, statistics, XLSX export; weekday/weekend follows company
  weekend days and vacations, R137), Settings with 8 tabs **gated by `useSession().can(...)` permissions**
  (`settings.company(.edit)`, `settings.buildings`, `settings.plants`, `settings.users`, `settings.analyzers`,
  `settings.integrations`, `settings.smtp`, profile/sessions/change-password for everyone but demo), Calendar (events +
  vacations/weekend days, `calendar.edit`).
- **From F5 (not done in F6a):** wire `BuildingAnalyzerPicker` to `useSelection`; card-grid stagger with
  `--duration-stagger-step` (D26); map tile URL (Q5) or the coordinate list; every page on `PageHeader`/`FilterBar`/
  `PeriodFilterBar`/`MetricCard`/chart wrappers/`DataTable` + `ExportMenu`/`DataQualityBadge`; new namespaces the same way
  as `auth`/`shell`; read MASTER.md then OVERRIDES.md.
- **Scope in queries:** every scoped `$api.useQuery` must spread `useScopeParams()` into `query` (admin `company_id`), and
  query keys must include it so a company switch refetches.
- **e2e:** `tests/e2e/{dashboard,consumption,load-profile,settings,calendar}.spec.ts` on the existing harness (fixtures:
  company A with buildings A1/A2 and analyzers, company B, demo company with 180 days of readings). Add seed data the
  screens need to `seed e2e`, not ad hoc in specs.
- **APIs the screens will want that F6a did not build** (decide in the F6b plan, ask the user): tariff templates and bulk
  tariff (05 §6), `GET /bills/dashboard` (+ export), solar plant realtime/production (F9), reports (F8), alarms (F7).
- **Open from F6a:** `/ekorm/settings` and the other nav targets are 404 until F6b builds them; the e2e suite only covers
  auth. The admin company switcher lists up to 500 companies (no paging UI). The layout redirect to login does not carry
  `next` (the middleware covers the missing-cookie case).
- **Deployment note (F15):** the API derives client IPs from `X-Forwarded-For` only for `EKOKOD_TRUSTED_PROXIES`; the web
  container and the front proxy must both be listed there, and the front proxy must append (not pass through) XFF.
- **Later phases:** F7 extends `internal/mail`; F8 maps the bill DTO → `InvoiceLineView` and owns export/e-mail; F9 plant
  aggregates; F12 public site; F13 `/anomaly/check` (R165 placeholder) and calendar events as an ML covariate (R137);
  F15 CSP nonces and the supported-browser floor.

## Open questions for the product owner (defaults ship)
- **F6a:** **Q-A1** two-step verification and self-registration are not built (R151) — needed? · **Q-A2** force a password
  change at an admin-created user's first login? · **Q-A3** password-reset mail uses the user's company SMTP settings; no
  platform SMTP exists — add one? · **Q-A4** sectoral comparison is cross-tenant: peers hidden, ≥ 3 peers required (R162),
  grid factor 0.469 kg/kWh (`grid_electricity_tr_2022`) — acceptable? · **Q-A5** mobile sessions last 30 days and are
  device-bound like web (R141/R142) — OK? · **Q6 (F5)** Calendar sits top-level after Reports (R167 default).
- **F5:** Q1 customiser theme colour · Q2 RTL · Q3 English UI keeps `1.234,56` · Q4 light `primary` = emerald-700 ·
  Q5 production map tile source · Q7 legacy catalogues disagree on 115 keys. Browser floor for `light-dark()`: Chrome/Edge
  ≥ 123, Firefox ≥ 120, Safari ≥ 17.5.
- **F4 (unchanged):** real-invoice comparison OPEN; dated `billing_parameters` defaults (R106, reactive basis, exemptions,
  R118, tiering, PTF tolerance, rounding R112); R119, R111, R110, R114, R122, R135; OG transformer loss.
- **F1–F3 (unchanged):** Q1 ratio zero-case, Q3 weekend default, Q5–Q10; F2 device/integration items. Q4 (calendar events vs
  weekend split) is **decided: no** (R137).

## Binding rulings
F1 rulings 1–14, F2 R1–R53, F3 R54–R104, F4 R105–R135, F5 D1–D26, **F6a R136–R189**
(`docs/superpowers/plans/2026-09-17-f6a-auth-api-skeleton.md`; R189 and the amendments above are in the ledger), plus
each phase's deviations.

## Process rules that paid off (keep them, inline)
- **Read legacy code and real data before coding rules;** decide splits and blockers with the user up front (R136–R139).
- **Prove every guard red with your own mutation, by script, restoring from a backup.** Record invalid mutants honestly:
  F6a had several that failed to compile (shell-escaped `&&`, unused variables) or used a no-op test command; each was
  re-run properly. Survivors exposed real gaps (refresh without a credential, rotated refresh keeping cookies, a public
  auth endpoint guard) or were proven equivalent (defence in depth in SQL, a double permission check).
- **Derive sweeps from the route table,** and make the sweep fail when a new route has no fixture mapping.
- **Per-task gates must run the whole touched packages** (integration tag), not only `-run <new tests>`: the phase-end
  race runs found four regressions the targeted runs had missed (scheduler cron config, F1 secret-column guard, arch TLS
  guard, an aggregate-watermark flake).
- **Build and run the real artefact.** The standalone build smoke found the build-time rewrite bug; designing the e2e
  device-mismatch test found the lost reason; the e2e run itself found two test-helper bugs, not product bugs.
- **Supply chain:** never accept pnpm's auto-written `minimumReleaseAgeExclude`; pick an aged version.
- **Commit as soon as tests are green,** then mutations, then merge `--no-ff`. Comments say WHY in 1–3 lines.

## Environment — read before running anything
- Prefix every command with `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
- WSL: 12 GB RAM, 4 GB swap. Another project's Gradle/Kotlin daemons (dolmusum) come and go (up to ~6 GB) — never kill
  them. Check `free -m` and `ps -eo rss,comm | grep java` before Storybook, Playwright or `next build`; Playwright
  `--workers=1` when tight; run long Playwright work in the foreground.
- **Never `pkill -f <pattern>` from a Bash call whose own command line contains the pattern** — it kills the calling shell
  (exit 144). Stop processes with `pgrep -x <name>` + `kill`.
- Testcontainers' reaper (ryuk) sometimes fails to become ready on Docker Desktop; if a package's own container start
  times out, re-run with `TESTCONTAINERS_RYUK_DISABLED=true`.
- Go integration: shared Postgres + Redis as in the prompt. Web (in `web/`): `pnpm lint && pnpm typecheck && pnpm check:api
  && pnpm check:i18n-parity && pnpm check:contrast`, `pnpm test --maxWorkers=2` (~70 s), `pnpm storybook:build` (~10 s),
  `STORIES='Shell,Features/Auth' pnpm test:a11y --workers=1`, `pnpm build` (~50 s),
  `pnpm build && pnpm test:e2e --workers=1` (~20 s of tests after the API seeds). `make openapi` regenerates both the Go
  document and `schema.d.ts`. Never leave `next dev`/`next start`, `storybook dev`, `vitest --watch`, a static server or
  `bin/ekokod-e2e` running.
- Worktrees on the native filesystem (`/home/personal/...`) only; `/mnt/c` is slow.
- `make check-generate` needs a clean tree for `internal/store/postgres/sqlcgen`, so commit first.
- Commit trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.

## Key paths
```
docs/superpowers/plans/2026-09-17-f6a-auth-api-skeleton.md      F6a plan (R136–R188, permission matrix, acceptance map)
internal/auth/                                                   hashing, policy, tokens, roles/permissions
internal/service/{auth,tenancy,assets,analysis,calendar,integrations}/, internal/service/billing/requests.go
internal/api/v1/{routes.go,router.go,openapi.go,handlers_*.go}   route table and handlers
internal/api/v1/{kit,dto,mw}/, internal/api/v1/openapi.json       HTTP kit, DTOs, middleware, committed OpenAPI
internal/api/v1/*_integration_test.go                            HTTP harness, sweeps, acceptance tests
internal/apiwire/, internal/seed/{fixtures,demo}.go              wiring, e2e and demo fixtures
web/src/lib/api/, web/src/app/api/v1/[...path]/route.ts          typed client, server client, runtime proxy
web/src/middleware.ts, web/src/app/{auth,ekorm}/                 auth guard, auth pages, /ekorm shell
web/src/lib/{session,selection}/, web/src/features/{auth,preferences}/
web/scripts/e2e-api.sh, web/tests/e2e/, web/playwright.config.ts e2e harness
/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite/.superpowers/sdd/2026-09-17-f6a/inline-ledger.md
```

## Verification at the F6a tip
Everything below ran on the merged tip (tasks 1–18 plus the task-19 review fixes), one step at a time:
- **Acceptance (Go):** `TestAuthorizationMatrix`, `TestMatrixCoversEveryRoute`, `TestReadOnlyRolesForbiddenOnEveryMutation`
  (unit) and, with `-tags=integration`, `TestCredentialsNeverLeak`, `TestDeviceMismatchInvalidatesSession`,
  `TestMobileBearerAuthenticatesNonAuthEndpoint`, `TestDemoSeesOnlySyntheticData`,
  `TestBuildingAdminCannotReachOtherBuildingsAnyEndpoint`, `TestReadOnlyRolesForbiddenOnEveryMutationHTTP`,
  `TestEveryMutationAudited`, `TestWeekendAndVacationChangeLoadProfileHTTP`: all PASS.
- **Race, package by package** (`-tags=integration -race`, shared Postgres/Redis) over every Go package F6a touched
  (api, api/middleware, api/v1 and subpackages, apiwire, auth, cli, credentials, domain/comparison, domain/reactive,
  ingest, job, mail, platform/config, render/table, scheduler, seed, service/*, store, store/postgres,
  store/postgres/admin, testfixtures, worker): all `ok` after the review fixes. `internal/api/v1` full integration run
  6× in a row green after the aggregate fix. Two one-off failures under load were green on isolated re-runs (ingest
  `TestFetchEnqueuesTheHourlyBucketWhenTheLastReadingLandsOnAnHourBoundary`; a 181 s container start in worker).
- `internal/arch` once: ok. `golangci-lint run --concurrency 2 --build-tags=integration` over every touched tree:
  0 issues. `make check-generate`: exit 0. `make openapi`: no diff (86 operations, 62 paths). `make vuln`: exit 0.
- **Web:** `pnpm lint` (0 problems), `pnpm typecheck`, `pnpm check:api`, `pnpm check:i18n-parity` (**12 namespaces,
  291 keys**), `pnpm check:contrast` (70 pairs × 2 themes), `pnpm test --maxWorkers=2` (**118 files, 423 tests**),
  `pnpm build` (`/ekorm` 2.63 kB, `/auth/login` 7.13 kB, middleware 34.8 kB), `pnpm audit --audit-level=high` (no known
  vulnerabilities): all exit 0.
- **a11y** (after the last UI change in each area, foreground, one worker): `STORIES='Features/Auth'` 98 passed and 2
  touch-target failures on secondary links → fixed, touch-target spec re-run 13 passed; `STORIES='Shell'` 156 passed;
  `STORIES='Features/Auth/AuthMessage'` 46 passed after the Playwright project split.
- **e2e** at the tip: `pnpm build && PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1` → **11 passed**.
- Standalone SSR smoke (Task 17): `/ekorm` without cookies → 307 `/auth/login?next=%2Fekorm`; `/` → 307 `/ekorm`; a
  refresh-cookie-only request refreshed against the runtime API URL; `/api/v1/*` proxied to the runtime URL.
- No dev server, `next start`, static server or `bin/ekokod-e2e` left running.
