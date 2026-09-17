# F6a — Auth, RBAC, API surface, OpenAPI client and app skeleton — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans, **inline, one session, no subagents,
> no Workflow** (user ruling 2026-09-17). TDD per task; commit as soon as tests are green; then prove every new
> guard red with your own mutation (restore from a backup copy, never `git checkout -- file` / `git stash`); then
> read the task diff; merge `f6a/task-N` into `phase/f6-core-screens` with `--no-ff`. Steps use `- [ ]`.

**Goal:** Everything F6's screens stand on: sessions and tokens, the six-role authorisation matrix with scope
resolution, every F6 JSON endpoint (plus F4's bills/tariffs HTTP), the mobile API, the demo company, a generated
OpenAPI 3.1 document and TypeScript client, and a session-aware `/ekorm` web shell with auth pages and an e2e
harness. **F6b** (next session) builds Dashboard, Consumption, Load Profile, Settings and Calendar screens on it.

**Architecture:** `internal/auth` (pure crypto/policy/permission tables) → `internal/service/{auth,tenancy,assets,
analysis,calendar}` (business rules over scoped repositories) → `internal/api/v1` (one route table drives chi
routing, authorisation, OpenAPI and the contract tests; handlers decode, authorise, call a service, encode) →
`internal/apiwire` (the API process's single wiring point, like `internal/worker`). The web app gets a generated
`openapi-typescript` schema, `openapi-fetch` + TanStack Query, a Next middleware that guards `/ekorm/*` and
refreshes tokens, and a Playwright `e2e` project that boots the real Go API.

**Tech stack:** Go 1.27 · chi · pgx/sqlc · goose · go-redis · asynq · `github.com/golang-jwt/jwt/v5` ·
`golang.org/x/crypto/bcrypt` · `github.com/swaggest/openapi-go` (openapi31) · `github.com/go-playground/validator/v10`
· Next.js 15 · `@tanstack/react-query` 5 · `openapi-fetch` · `openapi-react-query` · `openapi-typescript` 7 · Playwright.

**Spec:** `docs/rewrite/09-implementation-plan.md` §F6; `01-project-context.md` §2, §5, §6, §7.1–§7.4, §7.15, §7.18;
`05-api-contract.md` §1–§7, §14–§16, §18; `03-target-architecture.md` §2, §5, §7; `02-domain-rules.md` §3.6, §6.6,
§10.3, §10.4; `appendix/env-reference.md`; `appendix/legacy-inventory.md`. UI tasks: read
`design-system/bcem-energy/MASTER.md`, then `OVERRIDES.md`, then `07-design-system.md`.
Handoffs: `HANDOFF_NEXT_SESSION.md` (F5) "F6 must pick up"; F4 handoff "Carried forward".

## Global constraints (verbatim from 09 unless noted)

- Money and energy are `decimal`/`numeric`; a `float64` in a monetary or energy path is a build-breaking defect.
- Timestamps `timestamptz`, stored UTC, evaluated in `Europe/Istanbul`.
- `internal/domain` imports nothing from the project and has no I/O.
- HTTP handlers contain no business logic: decode, authorise, call a service, encode.
- No secret is ever logged; no secret has a default; startup fails on a missing secret.
- TLS verification is never disabled.
- Turkish default, English fully supported; every UI string in both catalogues; missing key fails CI.
- All UI work reads `MASTER.md` then `OVERRIDES.md` (ui-ux-pro-max design system of record).
- Repository layer takes an explicit `Scope`; no unscoped method outside `internal/store/postgres/admin` (03 §2.5).
- JSON `snake_case`; money/energy as strings; dates `YYYY-MM-DD`; timestamps ISO 8601 with offset (05 §1).
- Read-only roles enforced server-side (01 §2). Integration credentials never returned, in any form, to any role (03 §7).
- Machine rules: prefix every command with
  `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`;
  run only the packages you touch; never `make test` / `go test ./...`; integration tests against the shared DB:
  `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <pkg> -tags=integration -count=1 -parallel 4`;
  web `pnpm test --maxWorkers=2`; Playwright/`next build` one at a time in the foreground after `free -m`.
- Comments 1–3 lines, WHY only, cite ruling ids.

---

## Rulings (binding for F6a and F6b)

Numbering continues the phase series (F4 ended at R135). "Cost" = cost if the ruling is wrong.

| ID | Question | Ruling | Why | Cost |
|---|---|---|---|---|
| R136 | F6 scope | Split: **F6a** = this plan; **F6b** = Dashboard, Consumption, Load Profile, Settings (8 tabs), Calendar screens + `dashboard/consumption/load-profile/settings/calendar` e2e specs. | User decision. | — |
| R137 | Q4 (F6 blocker): do calendar events change the weekday/weekend split? | **No.** Only company weekend days and vacation periods do (R70 stands). 09 §F6's acceptance line is satisfied as "a weekend-day change and a vacation period change `/load-profile`". Events stay display-only (F13 may use them as an `is_event` covariate, as legacy ML did). | User decision; legacy load profile used fixed Sat/Sun; legacy ML kept events separate. | Migration + service change. |
| R138 | Demo role | A deterministic synthetic **demo company in the DB** (`ekokod seed demo`). Demo users belong to it; scope is their own company; every mutating route excludes `demo` from its role set. No per-endpoint mock branch. | User decision; one code path. | Demo shows stale data if not extended (R186). |
| R139 | Admin cross-company access | `company_id` query parameter accepted on every authenticated route. Admin: absent → own company; present → that company if live, else 404 `not_found`. Any other role: absent or own id → own company; another id → 404. | User decision; legacy `companyId`. | One middleware. |
| R140 | Access token | JWT HS256 signed with `Security.JWTSigningKey`; claims `iss="ekokod"`, `sub`=user id, `sid`=session id, `aud` = `"web"`\|`"mobile"`, `typ="access"`, `iat`, `exp` (`AccessTokenTTL`, default 15m); no leeway. Every authenticated request ALSO loads session and user: revoked/expired session → 401 `session_revoked`; inactive/deleted user → 401 `session_revoked`; role and company always come from the DB row, never from claims. | Revocation, logout-all, role change and device mismatch must take effect now, not in 15 min. | +2 PK queries/request. |
| R141 | Refresh token and rotation | Opaque 32 random bytes, base64url (no padding); stored as lowercase hex SHA-256 in `sessions.refresh_token_hash`. Lifetime is **absolute from login**: web 24h (`RefreshTokenTTL`), web remember 720h (`RefreshTokenRememberTTL`, new config, default 720h), mobile always 720h. Rotation inserts a new session row (same `expires_at`, `remember`, `client`, `device_fingerprint`, `rotated_from` = old id) and revokes the old with `revoked_reason='rotated'`, in one transaction. A `rotated` token presented ≤ 30 s after its rotation → 401 `token_rotated`, nothing revoked (two tabs refreshing at once); later → **reuse**: revoke every session of the user (`reuse_detected`) and 401 `session_revoked`. Unknown/expired/other-revoked → 401 `session_revoked`. | Theft detection without logging out concurrent tabs. | Wrong grace = spurious logouts. |
| R142 | Device binding | Fingerprint = hex HMAC-SHA256(`DeviceFingerprintSecret`, `User-Agent`), stored on the session. On authenticate and on refresh, a mismatch revokes that session (`device_mismatch`), clears cookies, and returns 401 `device_mismatch`; the web redirects to `/auth/login?reason=device_mismatch`. Applies to web and mobile. | 01 §7.1; legacy middleware. | Browser upgrade forces re-login (accepted, as legacy). |
| R143 | Cookies and bearer | `ekokod_at` (Max-Age = access TTL) and `ekokod_rt` (Max-Age = remaining lifetime when `remember`, session cookie otherwise); both `Path=/; HttpOnly; Secure; SameSite=Strict`. `Authorization: Bearer` takes precedence; an invalid bearer never falls back to cookies. | 05 §1; localhost counts as secure in Chromium so `Secure` holds in dev/e2e. | — |
| R144 | CSRF | No CSRF token. SameSite=Strict plus: every non-GET route requires `Content-Type: application/json` (multipart only where a route declares it) else 415 `unsupported_media_type`; bodyless POSTs accept an empty body with no content type. | Forms cannot send JSON cross-site. | — |
| R145 | Password hashing | `bcrypt(cost=BcryptCost, base64std(HMAC-SHA256(PasswordPepper, password)))`. A stored hash prefixed `legacy$` verifies as `bcrypt.Compare(rest, password)` and is re-hashed on a successful login (F14 writes that prefix). | Pepper without bcrypt's 72-byte truncation; F14 needs no auth change. | Re-hash on migration. |
| R146 | Password policy (legacy `passwordPolicy.ts`) | Violations collected together (422 `validation_failed`, `details.password = [codes]`): `password_too_short` (< 10 runes), `password_too_long` (> 128 runes), `password_complexity` (needs lower, upper, digit and one of ``!"#$%&'()*+,-./:;<=>?@[\]^_`{|}~`` or space), `password_repeat` (4+ identical consecutive runes), `password_common` (case-insensitive contains `1234`,`abcd`,`qwer`,`asdf`,`zxcv`,`password`,`passw0rd`,`1111`,`0000`), `password_personal` (contains, case-insensitive with Turkish lower-casing, any ≥3-rune word of the user's name, the e-mail local part split on `.`,`_`,`-`,`+`, or the company name), `password_reused` (matches the current hash or any of the last `PasswordHistorySize`=5 history hashes). | Legacy parity; 01 §7.1. | Users rejected wrongly. |
| R147 | Login failure and rate limit | Unknown e-mail, wrong password, inactive user → identical 401 `invalid_credentials` (unknown e-mail still runs one bcrypt compare against a fixed dummy hash). Fixed-window Redis limiter `RateLimitAuth` (default 5 per 15 min) keyed `auth:<route>:<client ip>:<lower(email)>` for login/mobile login/forgot, `auth:reset:<client ip>` for reset → 429 `rate_limited`, `Retry-After` = seconds to window end. Counts every attempt, success included. | 05 §2. | — |
| R148 | Forgot/reset password | `POST /auth/forgot-password` always 202 `{}`. For a live active non-demo user: invalidate their unused tokens, create one (32 bytes, SHA-256 stored, 1h), mail `{PublicURL}/auth/reset-password?token=…` in the user's locale through **their company's** SMTP settings. No SMTP settings or a send failure → operational message (`kind=system`, `category=auth`, `status=failed`, no token or link in any text) and still 202. `POST /auth/reset-password {token,password}`: invalid/used/expired → 400 `reset_token_invalid`; policy (R146) → 422; success: set password, mark used, revoke every session (`password_change`) → 204. | 05 §2; no platform SMTP exists. | PO may want platform SMTP (Q-A3). |
| R149 | Change password | `POST /auth/change-password {current_password,new_password}`: wrong current → 422 `details.current_password=["invalid"]`; policy → 422; success revokes every OTHER session → 204. | 05 §2. | — |
| R150 | Self-service routes and read-only roles | Exempt from "mutations 403 for read-only roles": `auth.refresh`, `auth.logout`, `auth.logout_all`, `auth.change_password`, `auth.sessions.revoke`, `profile.update`, `mobile.refresh` — they touch only the caller's own account. **Demo** is still 403 on `auth.change_password` and `profile.update` (a shared account). The exemption list is literal in `TestReadOnlyRolesForbiddenOnEveryMutation`. | 05 lists `PATCH /profile` for "all"; 09 acceptance says every mutation → 403; this reconciles both. | — |
| R151 | Registration and two-step | Neither is built. No `/auth/register` API route (`Features.SelfRegistration` stays false; the web `/auth/register` redirects to `/auth/login`). No `/auth/two-steps` screen: legacy had no backend and a code screen that verifies nothing would be offered unimplemented (09 §F7's SMS rule, applied here). | 01 §7.1 keeps registration disabled; 10 spirit. | PO Q-A1. |
| R152 | Error envelope | `{"error":{"code","message","details","request_id"}}`. `message` from a Go catalogue keyed by `code` (`tr` default; `en` when `Accept-Language` prefers English). Rate-limit, recoverer, 404/405 routing and body-limit errors use the envelope. Every code F6a emits has both locales (test). | 05 §1. | — |
| R153 | Pagination | `?limit` (default 50, max 500, else 400 `invalid_limit`) and opaque `?cursor` = base64url(JSON `{"o":offset}`) (bad → 400 `invalid_cursor`). Handlers request `limit+1` rows; `next_cursor` is set only when `limit+1` came back. `total` omitted in F6a. Each list route's max is `min(500, repository cap − 1)`; the executor checks each repository's `…PageMaxLimit` constant. | 05 §1; store pages by offset. | Cursor opacity only. |
| R154 | Unknown input | Unknown query parameter → 400 `unknown_parameter` (`details.parameter`); unknown JSON field or malformed JSON → 400 `invalid_body`; body > 1 MiB → 413 `body_too_large`. `company_id`, `limit`, `cursor` are accepted where applicable. Validation (`validate` tags) → 422 with `details.<json field> = [codes]`. | 05 §1. | — |
| R155 | Idempotency | Optional `Idempotency-Key` (≤ 128 chars) on every mutating route **except** `auth.*` and `mobile.*` (responses carry tokens/cookies). Redis key `idem:<user id>:<operation id>:<sha256(key)>`, 24h. First request stores `pending` (SET NX); completion stores status, content type, body (≤ 1 MiB) and SHA-256 of the request body, for 2xx and 4xx only. Replay with same body hash → stored response + header `Idempotent-Replay: true`; different body → 422 `idempotency_key_mismatch`; pending → 409 `idempotency_in_progress`. | 05 §1. | — |
| R156 | Audit | Every 2xx mutating request appends `audit_log`: `action` = operation id, `entity_type` = route's entity, `entity_id` = `{id}` path parameter when it parses as UUID, `user_id`, `ip`, company = resolved scope company. Integration-definition routes append through `AdminAuditRepository.AppendPlatform`. Successful logins append `auth.login`. `before/after` stay null in F6a. An audit write failure is logged and does not fail the request. | 03 §7. | Richer audit later. |
| R157 | Single route table | `internal/api/v1` has one `[]Route` literal. The router, authorisation, OpenAPI document, idempotency and audit all read it. `TestRouteTableMatchesRouter` walks chi and fails on any route not in the table (and vice versa). The expected role matrix is written **independently** in `authz_matrix_test.go` from 05's tables. | 05 §1 "every endpoint declares a required permission"; one source. | — |
| R158 | Scope resolution | A → `Scope{CompanyID: company_id param or own, AllBuildings: true}`; CA/CR → own company, AllBuildings; BA/BR → own company, `BuildingIDs` = every live building with `responsible_user_id = user` (listed per request through `BuildingRepository.List` under `store.SystemScope(company)` with `ResponsibleUserID`, all pages); D → own (demo) company, AllBuildings. | 01 §2 scope rules. | — |
| R159 | Permissions | `auth.PermissionsFor(role) []string` is the single table the API returns in `/auth/me` and the web filters nav and Settings tabs with. Table below (Permission matrix). | 01 §2 settings matrix + 05 role columns. | — |
| R160 | Consumption by building | `GET /consumption` requires exactly one of `analyzer_id` / `building_id` (else 400 `invalid_parameters`). Building rows are summed per window across the building's live analyzers: a register's building value is the sum when every analyzer has a non-nil value for it, else nil and the row `partial=true`; ratios recomputed from the sums (nil when active is zero or nil); `max_demand` = max of non-nil; indexes null. | UI needs building totals; JS has no decimal arithmetic. | — |
| R161 | Time and decimals in DTOs | `dto.Decimal` marshals `decimal.Decimal.String()` as a JSON string and rejects JSON numbers on input; `dto.Date` is `YYYY-MM-DD`; every `time.Time` in a response is rendered in `Europe/Istanbul` RFC 3339 (`2025-03-14T09:00:00+03:00`). Query `from`/`to` are dates, converted to the half-open Istanbul range `[from 00:00, to+1 00:00)`. | 05 §1. | — |
| R162 | Sectoral comparison (02 §10.4) | Peers = every live building of every live company whose `lower(trim(sector))` equals the building's, **including the building**. `daily_consumption` = mean daily `active_import` over the trailing 30 full Istanbul days ending yesterday, over days with data; `monthly_consumption` = previous full calendar month; `co2_emission_kg` = monthly × platform factor `grid_electricity_tr_2022`.`base_factor` (0.469 kg/kWh); per-capita/per-area skip null or zero denominators; average = arithmetic mean over peers with a value; rank ascending (1 = lowest consumption), competition ranking (1,1,3), peers without a value unranked. Null sector → 409 `building_sector_missing`. Fewer than 3 peers with a monthly value → `available=false, reason="sector_too_small"`. The response carries only the building's own figures, averages, ranks and counts — no peer id, name or company. Peer figures are read through a new `AdminSectorRepository` (closed-list justification: peers are other tenants). | Spec formula; legacy ranks ascending; ≥3 peers so an average cannot reveal one other tenant's consumption. | PO Q-A4. |
| R163 | Activity status (02 §3.6) | Analyzer `activity_status = "active"` iff `last_reading_at ≥ now − 7×24h`, else `"passive"`; building active iff any live analyzer is active. `GET /buildings?include=analyzer_count,active_status` adds `analyzer_count`, `active_analyzer_count`, `activity_status`. | Spec. | — |
| R164 | Analyzer refresh | `POST /analyzers/{id}/refresh {"mode":"hourly"\|"energy"}` (A CA BA) → 409 `integration_not_configured` when no credential exists for the analyzer's (provider, subtype); else enqueue `integration.refresh_analyzer` `{company_id, analyzer_id, mode}` (unique 1 min) → 202 `{"job_id"}`. Worker handler (in `internal/ingest`) opens the credential and enqueues cursor-driven `integration.fetch_readings` for `load_profile` (hourly) or every other kind the adapter's `Kinds` returns (energy). | 01 §7.2 refresh actions; keeps adapters out of the API process. | — |
| R165 | `POST /anomaly/check` before F13 | Returns 200 `{"available":false,"reason":"ml_service_unavailable"}`; never a synthesised verdict. | 05 §9 note; 10 item 12. | F13 replaces. |
| R166 | Reactive status (not in 05; needed by 01 §7.2 card) | `GET /consumption/reactive-status?month=YYYY-MM[&building_id]` (scope roles): per analyzer in scope (or in the building): `inductive_ratio`, `capacitive_ratio`, `installed_power_kw`, `inductive_limit`, `capacitive_limit`, `exempt_reason` (`below_min_power`\|`monomial`\|`user_group`\|`generation`\|null), `penalty_applies`; plus `highest_inductive` / `highest_capacitive` `{analyzer_id, building_id, ratio}`. Uses `reactive.Evaluate` with the effective billing parameters and, when `tariffsvc.Applicable` finds one, the tariff's term and user group; without a tariff only the size exemption is evaluated. Month = current Istanbul month when omitted (open month, Analytics rows). | Card needs thresholds the client cannot compute. | Added to OpenAPI. |
| R167 | Web routes | Authenticated app under `/ekorm/*` (01 §5); nav hrefs gain `/ekorm`; `/` redirects to `/ekorm` until F12; Calendar is a top-level nav entry after Reports (Q6 default); `/ekorm` renders the F0 health status in the shell until F6b's dashboard; `/ekorm/settings` is Settings. | 01 §5, legacy inventory. | One array edit. |
| R168 | Web ↔ API | Same origin: Next `rewrites` `/api/v1/:path*` → `${EKOKOD_INTERNAL_API_URL}/api/v1/:path*`. Server code calls the internal URL forwarding `cookie`, `user-agent`, `accept-language`, `x-forwarded-for`. `src/middleware.ts`: public paths `/auth/*`, `/api/*`, `/_next/*`, static files; if `ekokod_at` is missing and `ekokod_rt` present → server-side `POST /api/v1/auth/refresh` and copy its `Set-Cookie` onto the response and the forwarded request cookies; 401 `device_mismatch` → redirect `/auth/login?reason=device_mismatch` (cookies cleared); any other failure or no cookies → `/auth/login?next=<path>`. The browser client retries a request once after a single-flight refresh on 401 `token_expired`/`token_rotated`. | 03 §5.2 transparent refresh. | — |
| R169 | UI preferences sync | Migration adds `users.ui_preferences text` (≤ 512 chars, opaque `ekokod_ui` cookie value) and `users.locale text check in ('tr','en')` default `'tr'`. `/auth/me` returns both; `PATCH /profile` accepts both. After login the web writes `ekokod_ui` / `NEXT_LOCALE` from the profile; later changes PATCH the profile. The cookie stays the SSR source. | F5 handoff; 01 §6 "persist per user"; not jsonb (03 §3). | — |
| R170 | Selection store | `useSelection()` (React context + `useSyncExternalStore`) persisted to `localStorage["ekokod:selection:<userId>"]` = `{companyId?, buildingId?, analyzerId?}`; admin company switch clears building and analyzer; query hooks append `company_id` for admins when set. | 01 §6, 03 §5.2. | — |
| R171 | e2e harness | Playwright projects `a11y` and `e2e`; `webServer` becomes an array chosen by `PW_PROJECT` (`a11y` → static Storybook; `e2e` → `scripts/e2e-api.sh` on :18080 and `next start -p 13000`). `e2e-api.sh`: `go build`, fresh database `ekokod_e2e` in `ekokod-test-pg` (drop+create), Redis `ekokod-test-redis` (:56379, `make test-redis-up`), `ekokod migrate up`, `ekokod seed`, `ekokod seed e2e`, `ekokod seed demo`, exec `ekokod api`. | F5 handoff; real stack. | — |
| R172 | SMTP settings | `GET /smtp-settings` never returns the password (`has_password` bool); `PUT` upserts (empty password keeps the stored one; first save without password → 422); `POST /smtp-settings/test {"to"}` sends a fixed tr/en message: `secure=true` → implicit TLS; else STARTTLS when advertised; certificate always verified; failure → 422 `smtp_test_failed` with a scrubbed message. All three A only; admin targets a tenant with `company_id` (R139). | 05 §3; 01 §2. | — |
| R173 | Companies | `GET/POST /companies` A (list via new `AdminTenantRepository.ListCompanies`); `GET /companies/{id}` A CA CR (non-admin: own id only, else 404) incl. `analyzer_counts` by provider; `PATCH` A CA; `DELETE` A soft-deletes, 409 `cannot_delete_own_company` for the admin's own company. | 05 §3. | — |
| R174 | Users | Assignable roles — A: admin, company_admin, company_readonly_admin, building_admin, building_readonly_admin; CA: the last four. Demo users only via `seed demo`. `POST /users` requires a policy-valid `password` (R146, deny-list from the new user). Cannot delete self or change own role/is_active → 409 `cannot_modify_self`; duplicate e-mail → 409 `email_taken`; a role, `is_active` or e-mail change or a delete revokes all the user's sessions (`admin_change`). | 05 §3 "role options constrained". | — |
| R175 | Buildings | `DELETE` → 409 `building_has_analyzers` while live analyzers are attached. `responsible_user_id` must be a live user of the company (store refuses → 422 `details.responsible_user_id=["invalid"]`). `bill_cutoff_day` 1–31. Contacts are replaced as a list on create/update. | 05 §4; 01 §7.15. | — |
| R176 | Integration definitions | A only; platform audit (R156); `endpoints` must be a JSON object whose keys ⊂ {`authentication`,`analyzer_list`,`hourly_values`,`energy_values`,`token`,`last_index`,`indexes`,`last_success_date`,`load_profiles`,`owner_consumptions`,`current_indexes`,`end_of_month_indexes`} and whose values are `https://` URL templates; delete → 409 `definition_in_use` while a credential references it. | 01 §7.15; 05 §15. | — |
| R177 | Credential secrecy | Credential routes return `credentials.View` only. `TestCredentialsNeverLeak` configures one credential per provider with sentinel secrets and scans every response body produced by the whole API integration suite (a recorder wrapped around the router) for each sentinel, its base64 and its hex. | 09 §F6 acceptance. | — |
| R178 | Tariffs in F6a | `GET/POST /tariffs`, `GET/PATCH/DELETE /tariffs/{id}`, `GET /tariffs/applicable` over `tariffsvc`. The DTO **requires** `vat_rate` and every tax `rate` (F4 deviation). Templates, bulk assignment, icmal import (F8) and solar tariffs (F9) are not in F6a (legacy inventory). `*tariff.ValidationError` → 422 with its field codes. | F4 handoff; legacy inventory. | — |
| R179 | Bills in F6a | `GET /bills`, `GET /bills/{id}`, `GET /bills/{id}/pdf`, `GET /bills/{id}/hourly-detail?format=json\|xlsx`, `POST /bills/compute`, `GET /bills/latest`. PDF: serve `pdf_path` when the file exists under `Storage.Root`, else render with `invoicepdf` into the response and enqueue `billing.render_pdf` (a GET never writes). Compute → enqueue `billing.generate` per target with `Force` → 202 `{"job_ids":[]}`; company scope needs an AllBuildings scope (BA → 403 `forbidden`). `ComputeError` mapping: `tariff_not_found` 422, `no_consumption_data` 422, `unresolved_anomaly` 409, `period_not_closed` 409, `ptf_data_missing` 422, `billing_parameters_missing`/`_invalid` 500, `ErrInvalidRequest` 400, `store.ErrNotFound` 404. `/bills/dashboard*` → F8. | F4 handoff; 05 §7. | — |
| R180 | Per-principal rate limit | After authentication, the in-memory bucket (`RateLimitAPI`) is also applied per user id (F0 comment promised it). | F0 middleware note. | — |
| R181 | API process wiring | `internal/apiwire.Build(ctx, cfg, pool, log) (Built, error)` constructs every repository, service and the router; `internal/api/...` may not import `internal/store/postgres/...` (new arch guard). | Keeps handlers thin; mirrors `internal/worker`. | — |
| R182 | First users | `ekokod user create --email --name --role --company-id|--company-name` reads the password from `EKOKOD_NEW_USER_PASSWORD` (never a flag, never a default); creates the company when `--company-name` names none. Refuses `demo`. | No admin can exist otherwise (legacy `seed-admin`). | — |
| R183 | e2e/test fixtures | `ekokod seed e2e` (refused when `Env=production`) creates, idempotently with fixed UUIDs: platform company `Ekokod Platform` + admin; `E2E Company A` with buildings A1 (responsible = BA user), A2 (responsible = BR user), analyzers per building; `E2E Company B` with one building/analyzer; users for all five non-demo roles; password from `EKOKOD_E2E_PASSWORD`. The same builder (`internal/seed/fixtures.go`) serves Go API integration tests. | One fixture for Go and Playwright. | — |
| R184 | Mail package | `internal/mail`: `Sender.Send(ctx, model.SMTPSettings, password []byte, Message) error` with `Message{To, Subject, Text, HTML}`; templates `password_reset` and `smtp_test` in tr/en (`embed`). No logging of recipient bodies or passwords. F7 extends it. | Needed by R148/R172. | — |
| R185 | OpenAPI | `swaggest/openapi-go` `openapi31.Reflector` over the route table; `GET /api/v1/openapi.json` serves the embedded committed `api/openapi.json`; `ekokod tool openapi` regenerates it; `TestOpenAPIDocumentIsCurrent` fails on drift. Web `pnpm gen:api` → `src/lib/api/schema.d.ts` (committed); CI runs `pnpm gen:api && git diff --exit-code`. | 05 intro "generated in CI, never hand-written". | — |
| R187 | F2 R47: bind the iSolar OAuth state to the browser | `GET /integrations/isolar/authorize-url` also sets cookie `ekokod_oauth` = hex SHA-256 of the `state` it put in the URL (`HttpOnly; Secure; SameSite=Lax; Path=/api/v1/integrations/isolar/callback; Max-Age` = state TTL). The public callback requires that cookie to equal SHA-256(`state` query) (constant-time) before calling `credentials.ISolarCallback`, then expires it. `SameSite=Lax` because the provider's redirect is a cross-site top-level navigation that never carries the `Strict` session cookies. | F2 handoff item 3. | — |
| R188 | F2 R48: configuration errors | Any credential route error matching `integration.ErrConfig` → 422 `integration_config_invalid`; provider authentication failure → 422 `integration_auth_failed`; the two are never conflated. `credentials.Configure`'s `ErrConflict` for an existing definition → 409 `integration_already_configured`. | F2 handoff item 4. | — |
| R186 | Demo freshness | `seed demo` fills hourly readings from 180 days ago (first run) or the last stored reading up to the current hour, idempotently, then refreshes aggregates for that range. Job `demo.extend` (scheduler `EKOKOD_SCHEDULE_DEMO`, default `15 * * * *`) runs the same fill; no-op when the demo company is absent. | Map markers stay "active"; 7-day rule. | — |

### Permission matrix (R159)

`W` = `write` (can see mutation controls). Every role also has `nav.core` (dashboard, consumption, load profile,
forecast, renewable, bills, tariffs, alarms, messages, reports, carbon, iso-50001, calendar, settings/account).

| Permission | admin | company_admin | company_readonly_admin | building_admin | building_readonly_admin | demo |
|---|:-:|:-:|:-:|:-:|:-:|:-:|
| `write` | ✅ | ✅ | | ✅ | | |
| `nav.solar_plants`, `nav.financial` | ✅ | ✅ | ✅ | | | |
| `admin.companies` (company switcher, company list) | ✅ | | | | | |
| `settings.integrations` | ✅ | | | | | |
| `settings.company` | ✅ | ✅ | ✅ | | | |
| `settings.company.edit` | ✅ | ✅ | | | | |
| `settings.buildings`, `settings.plants`, `settings.users` | ✅ | ✅ | ✅ | | | |
| `settings.analyzers` | ✅ | ✅ | ✅ | ✅ | ✅ | |
| `settings.smtp` | ✅ | | | | | |
| `analyzers.refresh`, `bills.compute` | ✅ | ✅ | | ✅ | | |
| `calendar.edit` | ✅ | ✅ | | | | |
| `integrations.credentials` | ✅ | ✅ | | | | |

---

## File map

```
internal/auth/                     password.go policy.go token.go refresh.go fingerprint.go roles.go permissions.go (+ _test)
internal/mail/                     sender.go templates.go templates/*.tmpl (+ _test, fake SMTP server in test)
internal/store/postgres/migrations/00015_auth_sessions.sql
internal/store/postgres/queries/auth.sql, admin_tenant.sql, admin_sector.sql, admin_definitions.sql
internal/store/{repository.go}     + PasswordResetRepository, AdminTenantRepository, AdminSectorRepository, session/user additions
internal/store/postgres/{tenancy.go,password_reset.go}, admin/{auth.go,tenant.go,sector.go,catalogue.go}
internal/domain/comparison/        comparison.go (+ _test)          pure R162 averaging/ranking
internal/service/auth/             service.go login.go refresh.go password.go scope.go (+ _test, _integration_test)
internal/service/tenancy/          companies.go users.go profile.go smtp.go
internal/service/assets/           buildings.go analyzers.go plants.go comparison.go
internal/service/analysis/         building_rows.go reactive_status.go
internal/service/calendar/         calendar.go
internal/job/analyzer_refresh.go, internal/job/demo.go; internal/ingest/refresh_analyzer.go
internal/api/v1/                   routes.go (the table) kit/{bind.go,errors.go,messages.go,page.go,respond.go}
                                   middleware/{authn.go,authz.go,scope.go,idempotency.go,audit.go,authlimit.go,contenttype.go}
                                   dto/{common.go,auth.go,tenancy.go,assets.go,analysis.go,tariffs.go,bills.go,calendar.go,integrations.go}
                                   handlers_*.go  openapi.go  authz_matrix_test.go  route_table_test.go
internal/api/router.go             mounts /api/v1
internal/apiwire/wiring.go         R181
internal/seed/{demo.go,fixtures.go}; internal/cli/{user.go,seed.go(+subcommands),tool_openapi.go}
api/openapi.json                   generated, committed, embedded (package internal/api/v1 via go:embed from api/)
internal/arch/api_test.go          TestAPIDoesNotImportStoreImplementations
web/src/lib/api/{schema.d.ts,client.ts,server.ts,query.tsx,errors.ts}
web/src/middleware.ts; web/next.config.ts (rewrites)
web/src/app/auth/{layout.tsx,login,forgot-password,reset-password,error,maintenance,register}/page.tsx
web/src/app/ekorm/{layout.tsx,page.tsx}; web/src/app/page.tsx (redirect)
web/src/lib/session/{session-provider.tsx,permissions.ts}; web/src/lib/selection/selection-store.ts
web/src/components/shell/{nav-config.ts,company-switcher.tsx}; web/src/features/auth/*
web/messages/{tr,en}/auth.json
web/tests/e2e/{auth.spec.ts,fixtures.ts}; web/scripts/e2e-api.sh; web/playwright.config.ts
Makefile (test-redis-up/down, openapi, web-e2e), .github/workflows CI additions
```

`internal/testfixtures`: add `SharedRedisConfig(t)` honouring `EKOKOD_TEST_REDIS_URL` (falls back to `StartRedis`).

---

## Task 1 — `internal/auth` primitives

**Files:** Create `internal/auth/{password.go,policy.go,token.go,refresh.go,fingerprint.go,roles.go,permissions.go}` + tests. Modify `go.mod` (add `github.com/golang-jwt/jwt/v5`, promote `golang.org/x/crypto` to direct).

**Interfaces (Produces):**
```go
package auth
type Role = model.UserRole // reuse enum values: admin, company_admin, company_readonly_admin, building_admin, building_readonly_admin, demo
type RoleSet uint8
func Roles(rs ...model.UserRole) RoleSet
func (s RoleSet) Has(r model.UserRole) bool
var (AllRoles RoleSet /* six */; WriteRoles RoleSet /* A CA BA */)
func IsReadOnly(r model.UserRole) bool // CR, BR, demo

type Hasher struct{ Pepper []byte; Cost int }
func (h Hasher) Hash(password string) (string, error)
func (h Hasher) Verify(stored, password string) (ok, needsRehash bool) // R145, constant-time; legacy$ → needsRehash
func (h Hasher) DummyVerify(password string)                         // R147 timing

type PolicyInput struct{ Password, Name, Email, CompanyName string; CurrentHash string; History []string }
func CheckPolicy(h Hasher, in PolicyInput) []string // R146 codes in fixed order; nil when valid

type Claims struct{ UserID, SessionID uuid.UUID; Audience string; IssuedAt, ExpiresAt time.Time }
type Tokens struct{ Key []byte; TTL time.Duration; Now func() time.Time }
func (t Tokens) Issue(userID, sessionID uuid.UUID, audience string) (token string, exp time.Time, err error)
func (t Tokens) Parse(token string) (Claims, error) // ErrTokenExpired, ErrTokenInvalid
var ErrTokenExpired, ErrTokenInvalid error

func NewRefreshToken() (plain string, hash string, err error)
func HashRefreshToken(plain string) string
func Fingerprint(secret []byte, userAgent string) string

func PermissionsFor(r model.UserRole) []string // sorted; the R159 table
```

- [ ] **Step 1 — failing tests** (`internal/auth/*_test.go`):
  - `TestHasherRoundTripAndPepper`: `Hash("Ekokod!2026x")` verifies; the same password under a different pepper does not; hash starts with `$2a$`.
  - `TestHasherLegacyPrefixNeedsRehash`: stored `"legacy$"+bcrypt("Eski!Parola99")` → `(true,true)`; wrong password → `(false,false)`.
  - `TestHasherHandlesPasswordsLongerThan72Bytes`: two 100-byte passwords differing only at byte 90 do **not** verify against each other.
  - `TestCheckPolicy` table: `"Kisa!1a"` → `[password_too_short]`; `"abcdefghijK1"` → `[password_complexity, password_common]`; `"Aaaa!2345xyz"` → `[password_repeat]`; `"Qwer!9876zz"` → `[password_common]`; name `"Mehmet Yılmaz"`, `"Yılmaz!2026ab"` → `[password_personal]`; email `ayse.kaya@x.com`, `"Kaya#2026abc"` → `[password_personal]`; company `"Bcem Enerji"`, `"Enerji!2026x"` → `[password_personal]`; history containing hash of `"Gecmis!2025ab"` → `[password_reused]`; `"Guvenli!Sifre-42"` → nil; 129 runes → contains `password_too_long`.
  - `TestTokensIssueParse`: parse of issued token returns same ids and audience; `Now` advanced past TTL → `ErrTokenExpired`; one flipped signature byte → `ErrTokenInvalid`; token signed with `alg=none` or HS384 or another key → `ErrTokenInvalid`; `typ` claim other than `access` → `ErrTokenInvalid`.
  - `TestRefreshTokenShape`: 43 base64url chars, `HashRefreshToken(plain)==hash`, 64 hex chars, two calls differ.
  - `TestFingerprintIsKeyedHMAC`: same UA same secret equal; different secret or UA differ; 64 hex chars.
  - `TestPermissionsMatrix`: literal expected slices for each of the six roles exactly as the R159 table.
- [ ] **Step 2 — run, see them fail:** `go test ./internal/auth/ -count=1` → FAIL (undefined).
- [ ] **Step 3 — implement.** JWT: `jwt.NewWithClaims(jwt.SigningMethodHS256, …)`; parse with `jwt.WithValidMethods([]string{"HS256"})`, `jwt.WithIssuer("ekokod")`, `jwt.WithTimeFunc(t.Now)`, `jwt.WithExpirationRequired()`, map `jwt.ErrTokenExpired` → `ErrTokenExpired`, everything else → `ErrTokenInvalid`. Personal-fragment matching lower-cases with `strings.ToLowerSpecial(unicode.TurkishCase, …)`.
- [ ] **Step 4 — green:** `go test ./internal/auth/ -race -count=1`; `golangci-lint run --concurrency 2 ./internal/auth/...`.
- [ ] **Step 5 — commit** `feat(f6a): task-1 — auth primitives (hashing, policy, tokens, fingerprint, permissions)`.
- [ ] **Mutations (prove red):** drop the pepper from `Hash`; accept `alg` HS384; remove `password_personal` email split on `.`; give `building_admin` `settings.users`.

## Task 2 — Store: migration 00015 and auth/tenant repositories

**Files:** Create `internal/store/postgres/migrations/00015_auth_sessions.sql`, `queries/auth.sql`, `queries/admin_tenant.sql`, `internal/store/postgres/password_reset.go`, `admin/tenant.go`. Modify `internal/store/repository.go`, `internal/domain/model/tenancy.go`, `internal/store/postgres/tenancy.go`, `admin/auth.go`, `scope_isolation_integration_test.go`; regenerate sqlc (`make generate` / `sqlc generate`).

**Migration (Up):**
```sql
alter table sessions add column client text not null default 'web' check (client in ('web','mobile')),
  add column remember boolean not null default false,
  add column last_used_at timestamptz,
  add column revoked_reason text check (revoked_reason in ('rotated','logout','logout_all','password_change','device_mismatch','reuse_detected','admin_change')),
  add column rotated_from uuid references sessions(id) on delete set null;
alter table users add column ui_preferences text check (char_length(ui_preferences) <= 512),
  add column locale text not null default 'tr' check (locale in ('tr','en'));
create table password_reset_tokens (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  token_hash text not null unique,
  expires_at timestamptz not null,
  used_at timestamptz,
  created_at timestamptz not null default now());
create index on password_reset_tokens (user_id, created_at desc);
```
Down drops the table and columns in reverse order (no cascade).

**Interfaces (Produces):**
```go
// model.Session gains: Client string; Remember bool; LastUsedAt *time.Time; RevokedReason *string; RotatedFrom *uuid.UUID
// model.User gains: UIPreferences *string; Locale string
type PasswordReset struct{ ID, UserID uuid.UUID; TokenHash string; ExpiresAt time.Time; UsedAt *time.Time; CreatedAt time.Time } // model

// SessionRepository additions (Isolation: join through users):
Rotate(ctx, s Scope, oldID uuid.UUID, next model.Session, at time.Time) (model.Session, error) // one tx: insert next (rotated_from=oldID) + revoke old 'rotated'; old already revoked → ErrConflict
RevokeWithReason(ctx, s Scope, id uuid.UUID, reason string, at time.Time) error
RevokeAllForUserWithReason(ctx, s Scope, userID uuid.UUID, reason string, except *uuid.UUID, at time.Time) (int64, error)
Touch(ctx, s Scope, id uuid.UUID, at time.Time) error // last_used_at = at only when null or older than at-5min

// PasswordResetRepository (new, scoped; Isolation: join through users)
Create(ctx, s Scope, r model.PasswordReset) (model.PasswordReset, error) // invalidates the user's unused tokens (used_at = now) in the same tx
MarkUsed(ctx, s Scope, id uuid.UUID, at time.Time) error                // already used → ErrConflict

// AdminAuthRepository addition
PasswordResetByTokenHash(ctx, hash string) (model.PasswordReset, model.User, error)

// AdminTenantRepository (new) — admin role lists tenants; no single tenant to scope to
ListCompanies(ctx, f CompanyFilter) ([]model.Company, error)
```
`UserRepository.Update` persists `UIPreferences`/`Locale`; `Revoke`/`RevokeAllForUser` keep working (reason `logout`/`logout_all`).

- [ ] **Step 1 — failing integration tests** (`internal/store/postgres/auth_sessions_integration_test.go`, `admin/tenant_integration_test.go`):
  - `TestSessionRotateInsertsAndRevokesAtomically`: rotate → new row `rotated_from=old`, old `revoked_reason='rotated'`; rotating the old again → `ErrConflict`, no third row.
  - `TestSessionRevokeAllExcept`: three sessions, except #2 → returns 2, #2 still live.
  - `TestSessionTouchThrottles`: touch at t → last_used=t; touch at t+1m → unchanged; t+6m → updated.
  - `TestPasswordResetCreateInvalidatesPrevious` and `TestPasswordResetByTokenHashReturnsUser` (deleted user → ErrNotFound).
  - `TestUserPreferencesRoundTrip`: `locale='en'`, 512-char prefs stored; 513 chars → error.
  - `TestAdminListCompaniesSeesEveryTenant`: two tenants, both listed; soft-deleted excluded.
  - Extend `TestScopeIsolation…` (the isolation meta-test) for `PasswordResetRepository.Create/MarkUsed`, `SessionRepository.Rotate/RevokeWithReason/RevokeAllForUserWithReason/Touch` with tenant B's ids → `ErrNotFound`, nothing written.
  - Migration round-trip test picks 00015 up automatically; confirm it runs.
- [ ] **Step 2 — red:** run `./internal/store/postgres/ ./internal/store/postgres/admin/` integration with the shared DB (command in Global constraints) → FAIL.
- [ ] **Step 3 — implement** SQL + sqlc + repository methods.
- [ ] **Step 4 — green:** same integration command; `make check-generate` after committing; `go test ./internal/arch/ -run TestEveryStoreMethodIsScoped -count=1`.
- [ ] **Step 5 — commit** `feat(f6a): task-2 — migration 00015, session rotation, reset tokens, admin tenant list`.
- [ ] **Mutations:** drop `rotated_from` guard so double-rotate succeeds; make `Touch` unthrottled; make `PasswordResetByTokenHash` return deleted users.

## Task 3 — `internal/mail` sender and templates

**Files:** Create `internal/mail/{sender.go,templates.go,templates/password_reset.{tr,en}.txt,templates/smtp_test.{tr,en}.txt}` + `sender_test.go` (in-process fake SMTP server over TLS with a test CA).

**Interfaces:**
```go
type Message struct{ To []string; Subject, Text string }
type Sender interface{ Send(ctx context.Context, cfg model.SMTPSettings, password []byte, m Message) error }
func NewSMTPSender(dialTimeout time.Duration, roots *x509.CertPool /* nil = system */) Sender
func PasswordReset(locale, name, link string) Message // To filled by caller
func SMTPTest(locale string) Message
```
- [ ] **Tests:** `TestSMTPSenderImplicitTLS` (secure=true, fake server records `MAIL FROM`=`FromAddress`, `RCPT TO`, subject); `TestSMTPSenderStartTLS` (secure=false, server advertises STARTTLS → upgraded, AUTH only after TLS); `TestSMTPSenderRejectsUntrustedCertificate` (roots without the test CA → error, no AUTH sent); `TestSMTPSenderRefusesAuthWithoutTLS` (server without STARTTLS, password set → error `smtp: refusing to authenticate over plaintext`); `TestTemplatesBothLocales` (tr subject `"Şifre sıfırlama"`, en `"Password reset"`, link present in text); header injection: subject containing `\r\n` → error.
- [ ] Implement with `net/smtp` + `crypto/tls` (`ServerName` = host, `MinVersion` TLS 1.2). Run `go test ./internal/mail/ -race -count=1`; lint; commit `feat(f6a): task-3 — SMTP mail sender with TLS-only auth`.
- [ ] **Mutations:** skip STARTTLS; set `InsecureSkipVerify` (also caught by `TestNoTLSVerificationBypass`).

## Task 4 — `internal/service/auth`

**Files:** Create `internal/service/auth/{service.go,login.go,refresh.go,sessions.go,password.go,scope.go}`, `service_integration_test.go`, `scope_integration_test.go`. Modify `internal/platform/config` (add `Security.RefreshTokenRememberTTL`, env `EKOKOD_REFRESH_TOKEN_REMEMBER_TTL`, default 720h, must be ≥ RefreshTokenTTL; add `Schedule.Demo` env `EKOKOD_SCHEDULE_DEMO` default `15 * * * *`) and `appendix/env-reference.md`.

**Interfaces:**
```go
type Deps struct {
  Users store.UserRepository; Sessions store.SessionRepository; Resets store.PasswordResetRepository
  Companies store.CompanyRepository; Buildings store.BuildingRepository; SMTP store.SMTPRepository
  Ops store.OpsRepository; Audit store.AuditRepository; AdminAuth store.AdminAuthRepository
  Mail mail.Sender; Hasher auth.Hasher; Tokens auth.Tokens; FingerprintSecret []byte
  RefreshTTL, RememberTTL, MobileTTL time.Duration; HistorySize int; PublicURL string; Clock clock.Clock
}
type Client string // "web" | "mobile"
type LoginInput struct{ Email, Password, UserAgent string; IP *netip.Addr; Remember bool; Client Client }
type Issued struct{ AccessToken string; AccessExpires time.Time; RefreshToken string; SessionExpires time.Time; Remember bool; Principal Principal }
type Principal struct{ User model.User; Company model.Company; SessionID uuid.UUID; Client Client }

func New(d Deps) (*Service, error)
func (s *Service) Login(ctx, in LoginInput) (Issued, error)                         // ErrInvalidCredentials
func (s *Service) Refresh(ctx, refreshToken, userAgent string, client Client) (Issued, error) // ErrTokenRotated, ErrSessionRevoked, ErrDeviceMismatch
func (s *Service) Authenticate(ctx, accessToken, userAgent string) (Principal, error)  // ErrTokenExpired, ErrTokenInvalid, ErrSessionRevoked, ErrDeviceMismatch
func (s *Service) Logout(ctx, p Principal) error
func (s *Service) LogoutAll(ctx, p Principal) error
func (s *Service) Sessions(ctx, p Principal) ([]SessionView, error) // active only; Current bool
func (s *Service) RevokeSession(ctx, p Principal, id uuid.UUID) error
func (s *Service) ChangePassword(ctx, p Principal, current, next string) error     // *PolicyError{Codes}, ErrCurrentPasswordInvalid, ErrDemoForbidden
func (s *Service) ForgotPassword(ctx, email, locale string) error                  // never reveals existence
func (s *Service) ResetPassword(ctx, token, password string) error                 // ErrResetTokenInvalid, *PolicyError
func (s *Service) ResolveScope(ctx, p Principal, companyParam *uuid.UUID) (store.Scope, error) // R139/R158; ErrNotFound
```
Errors are `*platform/errors.Error` values with the R-codes (`invalid_credentials` 401, `token_expired` 401, `token_invalid` 401, `token_rotated` 401, `session_revoked` 401, `device_mismatch` 401, `reset_token_invalid` 400, `validation_failed` 422, `forbidden` 403).

- [ ] **Failing tests** (shared DB + `seed` fixtures from Task 5's builder are not yet available: use `testfixtures.NewTenant` plus direct `UserRepository.Create`):
  - `TestLoginIssuesSessionBoundToDevice`: session row `client=web`, fingerprint = `auth.Fingerprint(secret, ua)`, expires = now+24h; remember → now+720h; mobile → now+720h and `remember=true`.
  - `TestLoginFailuresAreIndistinguishable`: unknown e-mail, wrong password, inactive user → same error value and code; `DummyVerify` called for unknown e-mail (counter on a test hasher wrapper).
  - `TestLoginRehashesLegacyHash`: user with `legacy$…` hash logs in → stored hash no longer has the prefix and still verifies.
  - `TestRefreshRotatesWithAbsoluteExpiry`: login at T, refresh at T+1h → new token, new session id, `expires_at` unchanged, old `rotated`.
  - `TestRefreshGraceThenReuseDetection`: refresh with the old token at +10 s → `token_rotated`, all sessions intact; at +31 s → `session_revoked` and every session of the user `reuse_detected`.
  - `TestRefreshDeviceMismatchRevokes` and `TestAuthenticateDeviceMismatchRevokes`: other UA → `device_mismatch`; session `revoked_reason='device_mismatch'`; retry with the original UA → `session_revoked`.
  - `TestAuthenticateUsesLiveRoleAndRejectsRevoked`: change role in DB → principal shows new role; revoke → `session_revoked`; deactivate user → `session_revoked`.
  - `TestChangePasswordRevokesOthersKeepsCurrent`; `TestChangePasswordRejectsHistory` (5-deep history, 6th-oldest allowed).
  - `TestForgotPasswordSendsOnlyForActiveUsers`: fake `mail.Sender` records one message containing `https://app.example/auth/reset-password?token=` for an active user; nothing for unknown/inactive/demo; company without SMTP → operational message with `status=failed` whose text contains neither the token nor `reset-password`.
  - `TestResetPasswordSingleUse`: success revokes sessions (`password_change`); second use → `reset_token_invalid`; expired (clock +61m) → `reset_token_invalid`.
  - `TestResolveScope` (table): admin no param → own company AllBuildings; admin param company B → B; admin param unknown uuid → ErrNotFound; CA param B → ErrNotFound; CA param own → own; BA → `BuildingIDs` exactly the buildings with `responsible_user_id` = user (two of three); BA with none → empty non-nil slice, `AllBuildings=false`; demo → own company AllBuildings.
- [ ] Implement; run `go test ./internal/service/auth/ -tags=integration …` (shared DB); `-race` once; lint; commit `feat(f6a): task-4 — auth service (login, rotation, device binding, passwords, scope)`.
- [ ] **Mutations:** remove the 30 s grace; skip `rotated` reuse revocation; take role from claims; widen BA scope when empty to AllBuildings; send reset mail for inactive users.

## Task 5 — API kit, middleware, route table skeleton, OpenAPI, seed fixtures

**Files:** Create `internal/api/v1/{routes.go,openapi.go,route_table_test.go,authz_matrix_test.go}`, `internal/api/v1/kit/{bind.go,errors.go,messages.go,page.go,respond.go}` (+ tests), `internal/api/v1/middleware/{authn.go,authz.go,scope.go,idempotency.go,audit.go,authlimit.go,contenttype.go}` (+ tests), `internal/api/v1/dto/common.go`, `internal/seed/fixtures.go`, `internal/cli/tool_openapi.go`, `api/openapi.json`, `internal/testfixtures` `SharedRedisConfig`. Modify `internal/api/router.go` (mount `/api/v1`; error envelope for 404/405/rate limit/recoverer), `Makefile` (`test-redis-up/down` → `ekokod-test-redis` on 56379 with `redis:7.4.11-alpine`; `openapi` target), `go.mod` (swaggest openapi-go + jsonschema-go, validator/v10).

**Interfaces:**
```go
package v1
type Access int // Public, Authenticated, RoleGated
type Route struct {
  Method, Pattern, OperationID, Summary, Tag, Entity string
  Access Access; Roles auth.RoleSet   // RoleGated only
  SelfService bool                    // R150
  Idempotent bool                     // default true for non-GET except auth/mobile
  Request, Response any; Status int   // OpenAPI; Response nil → raw (pdf/xlsx/csv) with RawContentType
  RawContentType string; Multipart bool; ListMax int32
  Handler func(h *Handlers) http.HandlerFunc
}
func Table() []Route
func NewRouter(h *Handlers, mw Middleware) chi.Router  // Middleware bundles Authn, Scope, Idem, Audit, AuthLimit
func BuildOpenAPI() ([]byte, error)                     // deterministic (sorted), JSON 2-space indent

package kit
func Bind[T any](r *http.Request, dst *T, allowedExtraQuery ...string) error // path:"", query:"", json body; R154
func WriteJSON(w http.ResponseWriter, r *http.Request, status int, v any)
func WriteError(w http.ResponseWriter, r *http.Request, err error)           // R152 envelope, localised
type PageRequest struct{ Limit int32 `query:"limit"`; Cursor string `query:"cursor"` }
func (p PageRequest) Resolve(max int32) (store.Page, error)                  // limit+1 fetch
func PageOf[T any](items []T, limit int32, offset int32) Page[T]             // {items, next_cursor}
func Messages() map[string]map[string]string                                 // code → {tr,en}

package middleware (v1)
func FromContext(ctx) (authsvc.Principal, store.Scope, bool)
```
**Route table initial content:** Task 5 ships only `GET /openapi.json` (Public) — no placeholder handlers — plus `NewRouterForTable(routes []Route, h *Handlers, mw Middleware)` so the kit and the matrix harness are tested on synthetic rows before real endpoints exist. Each later task adds its rows and appends its expected rows to `authz_matrix_test.go`.

**`authz_matrix_test.go` harness:** `expected := map[string]string{ "GET /buildings": "A CA CR BA BR D", "POST /buildings": "A CA", … }` (literal strings from 05). For each table route × six roles: build the router from `Table()` with every `Handler` replaced by a 204 sentinel, a fake `Authn` that yields a principal of that role, and a fake `Scope`; assert 204 when the role is in `expected`, 403 otherwise; `Public`/`Authenticated` routes assert no 403 for any role. Also: every table route has an `expected` entry and vice versa (`TestMatrixCoversEveryRoute`).
`TestReadOnlyRolesForbiddenOnEveryMutation`: every non-GET route not in the literal R150 list → CR, BR, D get 403 (named subtests `POST /buildings/company_readonly_admin`…).

- [ ] **Failing tests:**
  - kit: `TestBindRejectsUnknownQuery` (`?foo=1` → 400 `unknown_parameter`, details.parameter="foo"); `TestBindRejectsUnknownJSONField`; `TestBindValidation422` (`validate:"required"` missing → 422 `details.name=["required"]`); `TestBindDecimalRejectsJSONNumber` (`{"total_area_m2":12.5}` → 400; `"12.5"` ok); `TestPageCursorRoundTrip` (limit 2, repo returns 3 → 2 items + cursor decoding to offset 2; returns 2 → no cursor; `limit=501` → 400 `invalid_limit`; cursor `"zz"` → 400 `invalid_cursor`); `TestErrorEnvelopeLocalised` (`Accept-Language: en-US,en;q=0.9` → English message; none → Turkish; unknown error → 500 `internal` without cause text, `request_id` echoed); `TestEveryMessageCodeHasBothLocales`.
  - middleware: `TestAuthnBearerPrecedenceAndCodes` (bad bearer + valid cookie → 401 `token_invalid`; expired → `token_expired`; missing → 401 `unauthorized`); `TestContentTypeRequired` (POST text/plain → 415; POST no body no type → passes); `TestIdempotencyReplayMismatchInProgress` (shared Redis: replay returns same body + `Idempotent-Replay: true`; different body → 422 `idempotency_key_mismatch`; concurrent second while pending → 409; 500 not stored); `TestAuthLimitPerIPAndEmail` (limit 5/15m: 6th login same ip+email → 429 with `Retry-After` > 0; other email → 200); `TestAuditAppendsOnSuccessOnly` (fake repo: 201 → one entry with operation id and uuid path id; 422 → none); `TestPrincipalRateLimit` (R180).
  - router: `TestUnmatchedRouteUsesEnvelope` (GET `/api/v1/nope` → 404 envelope code `not_found`).
  - openapi: `TestOpenAPIDocumentIsCurrent` (`BuildOpenAPI()` equals `api/openapi.json` byte-for-byte; message says run `make openapi`); `TestOpenAPIIs31AndDecimalIsString` (`openapi == "3.1.0"`, `dto.Decimal` schema `type: string`).
  - `TestRouteTableMatchesRouter`, `TestMatrixCoversEveryRoute`.
  - arch: `TestAPIDoesNotImportStoreImplementations` in `internal/arch/api_test.go` (`internal/api/...` must not import `internal/store/postgres/...` transitively).
  - seed: `TestE2EFixturesIdempotent` (integration: run twice → same ids, one row each; production env → error).
- [ ] **Middleware order** (outer → inner): existing RequestID → Recoverer → Logger → CORS (add `Idempotency-Key` to allowed headers, expose `Retry-After`, `Idempotent-Replay`) → Metrics → IP RateLimit → `/api/v1`: BodyLimit(1 MiB) → ContentType → AuthLimit (buffers and restores the body to read `email`) → Authn → PrincipalRateLimit → Authz (403 before any company lookup) → Scope (R139/R158) → Idempotency → Audit → handler.
- [ ] Implement. `openapi.go` maps `path`/`query`/`json`/`validate:"required"`/`enum`/`format` tags; security schemes `cookieAuth` (apiKey in cookie `ekokod_at`) and `bearerAuth`; every error response references a shared `Error` schema; operation ids from the table.
- [ ] Green: `go test ./internal/api/... ./internal/seed/ ./internal/arch/ -run 'TestAPIDoesNot|TestEveryStore' -count=1` (integration where tagged); lint; commit `feat(f6a): task-5 — API kit, middleware, route table, OpenAPI generator, fixtures`.
- [ ] **Mutations:** allow unknown query params; store 5xx idempotent responses; let authz treat an empty RoleSet as allow-all; remove `company_id` from BA… (scope middleware accepting another company for CA); drop one expected-matrix row (matrix-coverage test red).

## Task 6 — Auth and mobile endpoints, API wiring, `user create`

**Files:** Create `internal/api/v1/{handlers_auth.go,handlers_mobile.go}`, `dto/auth.go`, `internal/apiwire/wiring.go` (+ `wiring_integration_test.go`), `internal/cli/user.go`, `internal/api/v1/auth_http_integration_test.go`. Modify `internal/cli/api.go` (use `apiwire.Build`), `internal/cli/root.go`, `api/openapi.json`.

**Routes (05 §2, §3 profile, §18):**

| Method | Path | Access / roles | Op id | Notes |
|---|---|---|---|---|
| POST | /auth/login | Public, AuthLimit | auth.login | `{email,password,remember_me}` → 200 `MeResponse`, sets cookies (R143) |
| POST | /auth/refresh | Public (cookie `ekokod_rt`), SelfService | auth.refresh | 204 + cookies; errors clear cookies |
| POST | /auth/logout | Authenticated, SelfService | auth.logout | 204, clears cookies |
| POST | /auth/logout-all | Authenticated, SelfService | auth.logout_all | 204 |
| GET | /auth/me | Authenticated | auth.me | `MeResponse{id,name,email,phone,role,locale,ui_preferences,company{id,name},permissions[],session_id}` |
| POST | /auth/forgot-password | Public, AuthLimit | auth.forgot_password | 202 `{}` |
| POST | /auth/reset-password | Public, AuthLimit | auth.reset_password | 204 |
| POST | /auth/change-password | Roles A CA CR BA BR, SelfService | auth.change_password | 204 |
| GET | /auth/sessions | Authenticated | auth.sessions.list | `{items:[{id,client,user_agent,ip,created_at,last_used_at,expires_at,current}]}` |
| DELETE | /auth/sessions/{id} | Authenticated, SelfService | auth.sessions.revoke | 204; another user's id → 404 |
| GET | /profile | Authenticated | profile.get | |
| PATCH | /profile | Roles A CA CR BA BR, SelfService | profile.update | `{name?,email?,phone?,locale?,ui_preferences?}`; email taken → 409 |
| POST | /mobile/auth/login | Public, AuthLimit | mobile.login | → `{access_token,refresh_token,token_type:"Bearer",expires_in,user:MeResponse}` |
| POST | /mobile/auth/refresh | Public, SelfService | mobile.refresh | `{refresh_token}` → same shape |
| GET | /mobile/auth/me | Authenticated | mobile.me | `MeResponse` |

- [ ] **Failing HTTP integration tests** (real router via `apiwire.Build` with shared DB + shared Redis, `seed e2e` fixtures, `httptest.Server`):
  - `TestWebLoginMeRefreshLogout`: login → `Set-Cookie ekokod_at` (Max-Age 900) and `ekokod_rt` (no Max-Age); `/auth/me` role `company_admin`, permissions equal `auth.PermissionsFor`; refresh → new cookies, old refresh cookie replayed after 31 s (fake clock) → 401 `session_revoked`; logout → `/auth/me` 401.
  - `TestRememberMeCookieLifetime`: `remember_me=true` → `ekokod_rt` Max-Age 2592000.
  - `TestDeviceMismatchInvalidatesSession` (09 acceptance): me with another UA → 401 `device_mismatch`, `Set-Cookie ekokod_at=; Max-Age=0`; original UA again → 401 `session_revoked`.
  - `TestMobileBearerAuthenticatesNonAuthEndpoint` (09 acceptance): mobile login → bearer on `GET /profile` → 200; token with one flipped char → 401 `token_invalid`; fake clock +16 min → 401 `token_expired`; mobile refresh rotates.
  - `TestLoginRateLimited`: 5 bad attempts → 401 each; 6th → 429 `rate_limited`.
  - `TestForgotPasswordAlways202` (unknown and known e-mail both 202, identical bodies).
  - `TestProfileUpdateDemoForbidden` (demo → 403) and read-only role allowed (CR → 200).
  - `TestUserCreateCLI`: runs the cobra command with env password → user row with role admin, hash verifies; `--role demo` → error; missing env → error.
- [ ] Implement handlers (decode → service → encode only), cookie helpers in `handlers_auth.go`, `apiwire.Build` (repositories, services, redis, job client, mail sender; `Close`). Regenerate `api/openapi.json` (`make openapi`). Append the 15 expected rows to the matrix test.
- [ ] Green: `go test ./internal/api/... ./internal/apiwire/ ./internal/cli/ -tags=integration …`; lint; commit `feat(f6a): task-6 — auth, profile and mobile endpoints, API wiring, user create CLI`.
- [ ] **Mutations:** fall back to cookie when bearer invalid; omit `Secure` on cookies (assert attribute test red); allow demo PATCH /profile.

## Task 7 — Companies, users, SMTP settings (05 §3)

**Files:** Create `internal/service/tenancy/{companies.go,users.go,smtp.go}` (+ integration tests), `internal/api/v1/{handlers_tenancy.go,dto/tenancy.go}`. Modify `apiwire`, `api/openapi.json`, matrix test.

| Method | Path | Roles | Op id |
|---|---|---|---|
| GET | /companies | A | companies.list |
| POST | /companies | A | companies.create |
| GET | /companies/{id} | A CA CR | companies.get (`analyzer_counts: [{provider, subtype, count}]`) |
| PATCH | /companies/{id} | A CA | companies.update |
| DELETE | /companies/{id} | A | companies.delete (R173) |
| GET | /users | A CA CR | users.list (`?role&is_active&q`) |
| POST | /users | A CA | users.create (R174) |
| PATCH | /users/{id} | A CA | users.update |
| DELETE | /users/{id} | A CA | users.delete |
| GET | /smtp-settings | A | smtp.get (404 when none) |
| PUT | /smtp-settings | A | smtp.upsert |
| POST | /smtp-settings/test | A | smtp.test |

- [ ] **Failing tests:** `TestCompanyAdminCannotReadOtherCompany` (GET /companies/{B} as CA of A → 404; with `?company_id=B` → 404); `TestAdminCompanyLifecycle` (create → list contains → delete → list excludes; delete own → 409 `cannot_delete_own_company`); `TestUserRoleOptions` (CA creating `admin` → 422 `details.role=["not_assignable"]`; CA creating `building_admin` → 201; A creating `demo` → 422); `TestUserSelfModification` (delete self → 409 `cannot_modify_self`); `TestUserRoleChangeRevokesSessions`; `TestUserCreateEnforcesPolicy` (weak password → 422 with codes); `TestSMTPPasswordNeverReturned` (PUT with password `"Sentinel-SMTP-9f2c"` → GET body lacks it, `has_password=true`); `TestSMTPTestSend` (fake SMTP server reachable via a test `mail.Sender` → 204; failing sender → 422 `smtp_test_failed`).
- [ ] Implement; `make openapi`; append 12 matrix rows; green; commit `feat(f6a): task-7 — companies, users and SMTP settings endpoints`.
- [ ] **Mutations:** let CA assign `admin`; return `password_enc` length > 0 as `password` field; skip session revocation on role change.

## Task 8 — Buildings, analyzers, plants, analyzer refresh (05 §4)

**Files:** Create `internal/service/assets/{buildings.go,analyzers.go,plants.go}` (+ integration tests), `internal/job/analyzer_refresh.go`, `internal/ingest/refresh_analyzer.go` (+ test), `internal/api/v1/{handlers_assets.go,dto/assets.go}`. Modify `job.Handlers`/`Register`, `internal/worker/wiring.go`, `apiwire`, openapi, matrix.

| Method | Path | Roles | Op id / notes |
|---|---|---|---|
| GET | /buildings | A CA CR BA BR D | buildings.list `?include=analyzer_count,active_status&q&sector` (R163) |
| POST | /buildings | A CA | buildings.create (contacts[], R175) |
| GET | /buildings/{id} | A CA CR BA BR D | buildings.get (contacts, `tariff_history:[{id,effective_from,name}]`) |
| PATCH | /buildings/{id} | A CA | buildings.update |
| DELETE | /buildings/{id} | A CA | buildings.delete (409 `building_has_analyzers`) |
| GET | /analyzers | A CA CR BA BR D | analyzers.list `?building_id&provider&is_active&q&unassigned` (+ `activity_status`) |
| GET | /analyzers/{id} | A CA CR BA BR D | analyzers.get |
| PATCH | /analyzers/{id} | A CA | analyzers.update (`building_id`, `meter_multiplier`, `installed_power_kw`, `latitude`, `longitude` only) |
| POST | /analyzers/{id}/refresh | A CA BA | analyzers.refresh (R164) → 202 `{job_id}` |
| GET | /power-plants | A CA CR | plants.list |
| POST | /power-plants | A CA | plants.create (`monthly_targets[12]`, R-orientation enum) |
| GET | /power-plants/{id} | A CA CR | plants.get (devices, monthly targets, alarm recipients) |
| PATCH | /power-plants/{id} | A CA | plants.update |
| DELETE | /power-plants/{id} | A CA | plants.delete |

- [ ] **Failing tests:** `TestBuildingAdminSeesOnlyResponsibleBuildings` (list returns A1 only; GET A2 → 404; `?company_id=B` → 404); `TestBuildingListActiveStatus` (analyzer last reading now−6d → active, other building analyzer now−8d → passive; counts 1/1 and 1/0); `TestBuildingDeleteWithAnalyzers409`; `TestAnalyzerPatchIgnoresUnlistedFields` (body with `installation_number` → 400 `invalid_body`); `TestAnalyzerAssignToForeignBuilding404` (CA of A assigns to B's building → 404, row unchanged); `TestAnalyzerRefreshEnqueues` (fake enqueuer receives `integration.refresh_analyzer` payload with mode `hourly`; no credential → 409 `integration_not_configured`; BR → 403); `TestRefreshAnalyzerHandlerFansOut` (ingest unit test with fake adapter `Kinds` = load_profile, daily, billing: hourly → one fetch task `load_profile`; energy → `daily`,`billing`); `TestPlantMonthlyTargetsRequireTwelve` (11 → 422); `TestBuildingAdminCannotSeePlants` (BA GET /power-plants → 403).
- [ ] Implement; wire worker handler; `make openapi`; matrix rows; green (`./internal/service/assets/ ./internal/ingest/ ./internal/job/ ./internal/api/...`); commit `feat(f6a): task-8 — buildings, analyzers, plants and analyzer refresh`.
- [ ] **Mutations:** analyzer PATCH accepting `building_id` without a visibility check; activity threshold 8 days; BA list via AllBuildings.

## Task 9 — Sectoral comparison (02 §10.4, R162)

**Files:** Create `internal/domain/comparison/{comparison.go,comparison_test.go}`, `internal/store/postgres/queries/admin_sector.sql`, `admin/sector.go` (+ integration test), `internal/service/assets/comparison.go` (+ test). Modify `store/repository.go` (AdminSectorRepository + doc justification), `admin/doc.go` list, handlers/dto, openapi, matrix.

**Interfaces:**
```go
package comparison
type Figures struct{ BuildingID uuid.UUID; Daily, Monthly *decimal.Decimal; Personnel *int32; AreaM2 *decimal.Decimal }
type Metric struct{ Value, Average *decimal.Decimal; Rank, Ranked int }
type Result struct{ Available bool; Reason string; Daily, Monthly, CO2, PerCapita, PerArea Metric; Peers int }
func Compare(self uuid.UUID, peers []Figures, gridFactor decimal.Decimal) Result

// store
type SectorFigures struct{ BuildingID uuid.UUID; PersonnelCount *int32; TotalAreaM2 *decimal.Decimal; DailySum decimal.Decimal; DaysWithData int32; MonthlySum *decimal.Decimal }
type AdminSectorRepository interface {
  // SectorFigures: live buildings of live companies with lower(trim(sector)) = lower(trim(sector)); sums consumption_daily active_import of live analyzers over daily and month ranges.
  SectorFigures(ctx context.Context, sector string, daily, month TimeRange) ([]SectorFigures, error)
}
```
Route: `GET /buildings/{id}/comparison` (A CA CR BA BR D) → `{available, reason, sector, peers, daily_consumption{value,average,rank,ranked}, monthly_consumption{…}, co2_emission_kg{…}, consumption_per_capita{…}, consumption_per_area{…}}`; CSV export is client-side from these figures (F6b).

- [ ] **Failing tests:** `TestCompareRanksAscendingWithTies` (monthly 100,100,300 → self(100) rank 1, ranked 3; self 300 → rank 3); `TestCompareExcludesZeroDenominators` (personnel 0 and nil excluded from per-capita average and rank); `TestCompareSmallSector` (2 peers with monthly → `available=false`, reason `sector_too_small`); `TestCompareCO2` (monthly 1000, factor 0.469 → `469`, average of 1000 and 2000 → `703.5`); `TestSectorFiguresCrossTenantAggregates` (integration: tenants A and B same sector case/space-different → both buildings; deleted company excluded; other sector excluded); `TestComparisonResponseLeaksNoPeerIdentity` (HTTP: response JSON contains no tenant-B building id, company id or name); `TestComparisonSectorMissing409`.
- [ ] Implement; green (`./internal/domain/comparison/ -race`, integration for store and api); commit `feat(f6a): task-9 — sectoral comparison across tenants without peer identity`.
- [ ] **Mutations:** descending rank; include zero-personnel buildings; threshold 2 peers; add peer ids to the response DTO.

## Task 10 — Consumption, load profile, generation, anomalies, reactive status (05 §5, §16 anomaly check)

**Files:** Create `internal/service/analysis/{building_rows.go,reactive_status.go}` (+ tests), `internal/api/v1/{handlers_analysis.go,dto/analysis.go}`. Modify apiwire, openapi, matrix.

| Method | Path | Roles | Notes |
|---|---|---|---|
| GET | /consumption | A CA CR BA BR D | `analyzer_id|building_id`, `granularity=hourly|daily|monthly|yearly`, `from`, `to` (dates, R161); rows: `period_start`, `period_end`, `analyzer_id?`, every register value + closing index (active/inductive/capacitive import, t1–t3, active/inductive/capacitive export, u1–u3 — the 01 §7.3 column set), `inductive_ratio`, `capacitive_ratio`, `max_demand_kw`, `partial`, `suspect_registers[]` |
| GET | /consumption/summary | same | `consumption.Summarise` → totals, average, peak/valley with period |
| GET | /consumption/export | same | `?format=csv|xlsx`; CSV from `consumption.ExportRows` (formula-injection safe), XLSX via excelize, `Content-Disposition` |
| GET | /consumption/anomalies | same | `billing.ListAnomalies` |
| POST | /consumption/anomalies/{id}/resolve | A CA | `ResolveAnomaly` modes `reset_registered|manual_override|accepted` |
| GET | /consumption/reactive-status | same as GET /consumption | R166 |
| GET | /load-profile | same | `analyzer_id`, `from`, `to`, `profiles=weekday,weekend,winter_weekday,…` → `{profiles:{key:[24 decimal|null]}, config:{weekend_days[], weekend_source, vacations}}` |
| GET | /load-profile/statistics | same | per key `{max,min,hour_of_max,mean,stddev,range,load_factor}` |
| GET | /load-profile/export | same | XLSX hourly matrix (rows = hours 0–23, columns = keys) |
| GET | /generation | same | `consumption.GenerationRows` |
| GET | /energy-balance | same | `consumption.Balance` |
| POST | /anomaly/check | A CA BA | R165 |

`profiles` accepts exactly the ten keys `weekday, weekend, winter_weekday, winter_weekend, spring_weekday, spring_weekend, summer_weekday, summer_weekend, autumn_weekday, autumn_weekend`; others → 400 `invalid_parameters`.

- [ ] **Failing tests:** `TestSumBuildingRows` (two analyzers, one nil inductive → building inductive nil + partial; ratios recomputed `"0.25"`); `TestConsumptionRequiresExactlyOneSubject`; `TestConsumptionColumnSet` (JSON row keys equal the literal 01 §7.3 list — 09 acceptance backing F6b); `TestConsumptionForeignAnalyzer404` (BA with A2's analyzer → 404); `TestConsumptionExportCSVHeaderAndInjection` (header row literal; a cell `=cmd` is prefixed); `TestLoadProfileKeysAndLabels` (all ten keys present; unknown `summer_holiday` → 400) — the calendar-driven acceptance test is Task 12's `TestWeekendAndVacationChangeLoadProfileHTTP`; `TestReactiveStatusThresholds` (installed 25 kW, inductive ratio 0.35 → limit 0.33, `penalty_applies=true`; 8 kW → `exempt_reason=below_min_power`; highest inductive analyzer reported); `TestAnomalyCheckUnavailable`.
- [ ] Implement; `make openapi`; matrix rows; green; commit `feat(f6a): task-10 — consumption, load profile, generation and reactive status endpoints`.
- [ ] **Mutations:** building sum treating nil as zero; accept unknown profile key; reactive limit comparison `>=`.

## Task 11 — Tariffs and bills HTTP (F4 carry, R178/R179)

**Files:** Create `internal/api/v1/{handlers_tariffs.go,handlers_bills.go,dto/tariffs.go,dto/bills.go}` (+ integration tests). Modify apiwire (tariffsvc, billingsvc read side, job client, `Storage.Root`), openapi, matrix.

| Method | Path | Roles |
|---|---|---|
| GET | /tariffs | A CA CR BA BR D (`?building_id`) |
| POST | /tariffs | A CA |
| GET | /tariffs/{id} | A CA CR BA BR D |
| PATCH | /tariffs/{id} | A CA |
| DELETE | /tariffs/{id} | A CA |
| GET | /tariffs/applicable | A CA CR BA BR D (`building_id`, `date`) |
| GET | /bills | A CA CR BA BR D (`scope`, `building_id`, `analyzer_id`, `period`, `status`) |
| GET | /bills/{id} | A CA CR BA BR D (lines, members) |
| GET | /bills/{id}/pdf | A CA CR BA BR D |
| GET | /bills/{id}/hourly-detail | A CA CR BA BR D (`format=json|xlsx`) |
| POST | /bills/compute | A CA BA (`{scope, building_ids?, analyzer_ids?, period:"YYYY-MM", force}`) |
| GET | /bills/latest | A CA CR BA BR D (`scope`, `subject_id`) |

- [ ] **Failing tests:** `TestTariffDTORequiresVatAndTaxRates` (missing `vat_rate` → 422 `details.vat_rate=["required"]`; a tax without `rate` → 422 `details["taxes[0].rate"]`); `TestTariffValidationErrorFieldCodes` (a `*tariff.ValidationError` surfaces its field codes); `TestComputeErrorMapping` (table over the eight codes → status); `TestBillPDFServesStoredOrRenders` (stored file bytes served; missing file → `%PDF` rendered, enqueuer got `billing.render_pdf`, no file written by the handler); `TestBillHourlyDetailXLSX` (content type `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`); `TestComputeCompanyScopeForbiddenForBuildingAdmin` (403); `TestBillsForeignBuilding404` (BA → bill of A2 → 404).
- [ ] Implement; openapi; matrix; green; commit `feat(f6a): task-11 — tariffs and bills HTTP over tariffsvc and billingsvc`.
- [ ] **Mutations:** map `unresolved_anomaly` to 422; write the PDF from the GET handler; make `vat_rate` optional.

## Task 12 — Calendar and integrations (05 §14, §15)

**Files:** Create `internal/service/calendar/calendar.go` (+ test), `internal/store/postgres/queries/admin_definitions.sql` + `admin/catalogue.go` additions, `internal/api/v1/{handlers_calendar.go,handlers_integrations.go,dto/calendar.go,dto/integrations.go}`, `internal/api/v1/credentials_leak_integration_test.go`. Modify repository.go (`AdminCatalogueRepository.CreateIntegrationDefinition/UpdateIntegrationDefinition/DeleteIntegrationDefinition`), apiwire (credentials.Service needs the same deps the worker builds: extract `credentials` construction into a shared helper `worker.CredentialDeps(...)`? No — build it in apiwire from the same packages; add `TestAPIAndWorkerCredentialDepsAgree` comparing StateKey/RedirectURI), openapi, matrix.

| Method | Path | Roles | Notes |
|---|---|---|---|
| GET | /calendar/events | A CA CR BA BR D | `from`,`to` required |
| POST | /calendar/events | A CA | `{title,starts_at,ends_at,all_day,colour}`; `ends_at > starts_at` else 422; colour `^#[0-9a-f]{6}$` |
| PATCH | /calendar/events/{id} | A CA | |
| DELETE | /calendar/events/{id} | A CA | |
| GET | /calendar/vacations | A CA CR BA BR D | `{weekend_days:[0..6], weekend_source, periods:[{id,start_date,end_date,description}]}` |
| PUT | /calendar/vacations | A CA | replaces weekend days and periods in one service call (store tx per repo; the service deletes and recreates periods; `end_date ≥ start_date`; days unique 0–6) |
| GET | /integration-definitions | A | |
| POST | /integration-definitions | A | R176 |
| PATCH | /integration-definitions/{id} | A | |
| DELETE | /integration-definitions/{id} | A | 409 `definition_in_use` |
| GET | /integration-credentials | A CA | `credentials.View` list |
| POST | /integration-credentials | A CA | `{provider,subtype,username,secret,extra{},settings,pm5340_url,installation_number,isolar_region}` |
| PATCH | /integration-credentials/{id} | A CA | omitted secret unchanged |
| DELETE | /integration-credentials/{id} | A CA | |
| POST | /integration-credentials/{id}/verify | A CA | |
| POST | /integration-credentials/{id}/discover | A CA | 202 `{job_id}` |
| POST | /integration-credentials/{id}/backfill | A CA | `{analyzer_ids?,kinds?,from,to,force}` → 202 |
| GET | /integrations/isolar/authorize-url | A CA | `?credential_id` → `{url}` |
| GET | /integrations/isolar/callback | Public | `code`,`state` → 302 to `/ekorm/settings?tab=company&isolar=ok|error` |

- [ ] **Failing tests:** `TestCalendarReadOnlyForBuildingRoles` (BA GET 200, POST 403); `TestVacationsReplace` (put weekend [5,6] + one period → get returns them, `weekend_source=company`); `TestWeekendAndVacationChangeLoadProfileHTTP` (09 acceptance, R137: seeded two-week hourly data; GET /load-profile `weekday` hour-10 mean `"10"` with Sat/Sun weekend; PUT vacations making Wednesday a vacation → weekday mean changes to the hand-computed value; a calendar **event** on Thursday changes nothing — asserted equal); `TestDefinitionDeleteInUse409`; `TestDefinitionEndpointsValidated` (`http://` value → 422; unknown key → 422); `TestCredentialsNeverLeak` (R177: create one credential per provider with secrets `SENTINEL-<provider>-SECRET`, extra `SENTINEL-<provider>-EXTRA`, then exercise list/get/patch/verify(fake verifier)/discover responses and every response recorded by the suite's recorder; assert none contains any sentinel, its base64 or hex); `TestIsolarCallbackRedirects` (bad state → redirect with `isolar=error`); `TestIsolarCallbackRequiresBrowserBinding` (R187: valid state without the `ekokod_oauth` cookie, or with a cookie for another state → `isolar=error` and the nonce is NOT consumed — a later correct call still succeeds; authorize-url response sets the cookie with `SameSite=Lax` and the callback path); `TestCredentialErrorMapping` (R188: fake verifier returning a wrapped `integration.ErrConfig` → 422 `integration_config_invalid`; an auth failure → 422 `integration_auth_failed`; second POST for the same definition → 409 `integration_already_configured`).
- [ ] Implement; openapi; matrix; green; commit `feat(f6a): task-12 — calendar, integration definitions and credentials endpoints`.
- [ ] **Mutations:** include `ExtraKeys` values; let CA manage definitions; make calendar events count as vacation days in the service (R137 test red).

## Task 13 — Demo company (R138, R186)

**Files:** Create `internal/seed/demo.go` (+ integration test), `internal/job/demo.go`, `internal/cli/seed.go` subcommands `seed demo` and `seed e2e`, `internal/api/v1/demo_integration_test.go`. Modify scheduler (demo schedule), worker handlers, apiwire.

**Demo content (fixed UUIDs in `internal/seed/demo.go`):** company `Demo Enerji A.Ş.` (sector `Üretim`, area 12500.00, personnel 180); building `Demo Fabrika` (Ankara 39.925533, 32.866287; cutoff day 1; sector `Üretim`); analyzers `DEMO-0001` (installed 250 kW, multiplier 1) and `DEMO-0002` (installed 45 kW) with provider `pm5340` subtype `demo`; hourly `load_profile` readings with cumulative active/inductive/capacitive import registers generated by a deterministic PRNG (seed 42000, base load curve weekday 06–18 higher, weekend 40 %), inductive ≈ 18–26 % of active, capacitive ≈ 2–6 %; weekend days Sat+Sun; one vacation period (29 Oct); tariff (single-rate, energy `3.4547`, distribution `2.4794`, reactive `3.4937`, VAT 20, BTV 5, binomial term, contracted 200 kW, power price 25.5) effective 180 days ago; users: `demo@ekokod.com.tr` role demo, password from `EKOKOD_DEMO_PASSWORD` (required; seed fails without it). Bills for closed months are generated by the scheduled billing job, not by the seed.

- [ ] **Failing tests:** `TestSeedDemoIdempotentAndExtends` (run at T and T+3h (fake clock) → readings contiguous hourly with no duplicates, last reading = T+3h hour); `TestDemoSeesOnlySyntheticData` (09 acceptance: seed e2e + demo; login as demo; GET every scope route in the table that needs no path id plus `/buildings/{demo}`, `/analyzers/{demo}`, `/consumption?building_id=demo…`, `/load-profile…`, `/buildings/{demo}/comparison`; responses contain none of E2E Company A/B company, building, analyzer or user ids; `?company_id=<A>` → 404); `TestDemoMutationsForbidden` (POST /buildings, PATCH /profile, POST /analyzers/{id}/refresh → 403); `TestDemoExtendJobNoopWithoutDemoCompany`.
- [ ] Implement; green; commit `feat(f6a): task-13 — synthetic demo company, extend job, demo isolation tests`.
- [ ] **Mutations:** allow `?company_id` for demo; generate readings with `time.Now()` instead of the clock (idempotency test red).

## Task 14 — Cross-cutting authorisation sweep (09 acceptance)

**Files:** Create `internal/api/v1/tenancy_sweep_integration_test.go`. No production code unless a test finds a hole.

- [ ] `TestBuildingAdminCannotReachOtherBuildingsAnyEndpoint`: for every table route with an `{id}` or `building_id`/`analyzer_id` parameter, call it as BA of A1 with A2's and company B's ids (and `?company_id=B`) → 404 (or 403 where the role set denies); the test derives the list from `Table()` and fails if a new such route has no fixture id mapping (`idFor(entity)`).
- [ ] `TestReadOnlyRolesForbiddenOnEveryMutationHTTP`: the Task 5 unit matrix re-run against the real router with real principals for CR and BR (403 before any service call — assert no audit row written).
- [ ] `TestEveryMutationAudited`: one successful call per mutating route family writes exactly one `audit_log` row with the operation id.
- [ ] Green; commit `test(f6a): task-14 — tenancy and read-only sweeps over the full route table`.
- [ ] **Mutation:** remove `BuildingIDs` narrowing from one analysis handler's scope (sweep red).

## Task 15 — Web: generated client, TanStack Query, Next middleware

**Files:** Modify `web/package.json` (deps `@tanstack/react-query@5.103.1`, `openapi-fetch@0.17.0`, `openapi-react-query@0.5.4`; dev `openapi-typescript@7.13.0`; scripts `gen:api`, `check:api`), `web/next.config.ts` (rewrites, R168), `web/src/components/providers.tsx` (QueryClientProvider). Create `web/src/lib/api/{schema.d.ts,client.ts,server.ts,query.ts,errors.ts}` (+ tests), `web/src/middleware.ts` (+ `middleware.test.ts`). Modify CI (`.github/workflows/*.yml` web job: `pnpm check:api`), `Makefile` `web-lint`.

**Interfaces:**
```ts
// client.ts
export const api: Client<paths>;                         // baseUrl '/api/v1', credentials 'same-origin', Accept-Language from NEXT_LOCALE
export function refreshOnce(): Promise<boolean>;         // single-flight POST /auth/refresh
// query.ts
export const $api: OpenapiQueryClient<paths>;            // openapi-react-query over api
// server.ts
export async function serverApi(): Promise<Client<paths>>; // internal URL, forwards cookie/user-agent/accept-language/x-forwarded-for
export async function getMe(): Promise<MeResponse | null>;
// errors.ts
export type ApiErrorCode = components['schemas']['Error']['error']['code'];
export function authRedirectFor(code: string, next: string): string | null; // device_mismatch → '/auth/login?reason=device_mismatch'; session_revoked|token_invalid → '/auth/login?next=…'
// middleware.ts
export const config = { matcher: ['/((?!_next/|api/|favicon.ico|fonts/|.*\\..*).*)'] };
```
- [ ] **Failing tests (vitest):** `client.test.ts` — 401 `token_expired` → one refresh call → original request retried once (two concurrent 401s → one refresh); refresh 401 → `window.location.assign('/auth/login?next=…')`; `device_mismatch` → `/auth/login?reason=device_mismatch`. `middleware.test.ts` — no cookies on `/ekorm/consumption` → redirect `/auth/login?next=%2Fekorm%2Fconsumption`; `ekokod_rt` only → fetch to `${INTERNAL}/api/v1/auth/refresh` with forwarded cookie+UA, response carries both `Set-Cookie`s, request continues; refresh 401 `device_mismatch` → redirect with reason and expiring cookies; `/auth/login` passes through; `/` → redirect `/ekorm`. `schema.test.ts` — type-level: `paths['/auth/me']['get']` exists (compile via `pnpm typecheck`).
- [ ] Implement; `pnpm gen:api`; `pnpm lint && pnpm typecheck && pnpm test --maxWorkers=2`; commit `feat(f6a): task-15 — generated API client, TanStack Query and auth middleware`.
- [ ] **Mutations:** refresh per request (not single-flight); middleware forwarding no User-Agent (device-binding test red).

## Task 16 — Web: auth pages

Read `design-system/bcem-energy/MASTER.md`, then `OVERRIDES.md`, `07-design-system.md` (§ auth/split layout, forms, feedback).

**Files:** Create `web/src/app/auth/{layout.tsx,login/page.tsx,forgot-password/page.tsx,reset-password/page.tsx,error/page.tsx,maintenance/page.tsx,register/page.tsx}`, `web/src/features/auth/{login-form.tsx,forgot-password-form.tsx,reset-password-form.tsx,password-rules.tsx,auth-split-layout.tsx}` (+ stories + tests), `web/messages/{tr,en}/auth.json`, register in `messages/index.ts`. Wording source: `docs/rewrite/appendix/i18n-tr.json` `auth.*`, `password.*` (not importable, D7).

- [ ] **Failing tests:** `login-form.test.tsx` — labels `E-posta`, `Şifre`, checkbox `Bu Cihazı Hatırla`, link `Şifremi Unuttum`; submit posts `{email,password,remember_me:true}`; 401 `invalid_credentials` → `Alert` with `E-posta veya şifre hatalı.` and focus moves to it; 429 → rate-limit message with minutes from `Retry-After`; success → `router.replace(next ?? '/ekorm')` and prefs sync server action called with `ui_preferences`/`locale`; `?reason=device_mismatch` renders `Oturumunuz başka bir cihazda kullanıldığı için sonlandırıldı. Lütfen tekrar giriş yapın.`; `next` outside `/ekorm` ignored (open-redirect guard: `//evil.com`, `https://evil.com`, `/auth/login` → `/ekorm`). `reset-password-form.test.tsx` — client rules list mirrors R146 codes; 422 codes mapped to messages; success → login with `?reason=password_reset`. `forgot-password-form.test.tsx` — always shows the same confirmation. `register/page` → redirect `/auth/login`. axe clean in story tests; `pnpm check:i18n-parity`.
- [ ] Implement with `Field`, `Input`, `Checkbox`, `Button`, `Alert`, `FormErrorSummary`; split layout (brand panel hidden < 768 px). `pnpm lint && pnpm typecheck && pnpm check:i18n-parity && pnpm check:contrast && pnpm test --maxWorkers=2`; storybook build + scoped a11y `STORIES='Features/Auth' pnpm test:a11y --workers=1`; commit `feat(f6a): task-16 — login, forgot and reset password pages`.
- [ ] **Mutation:** accept absolute `next` URLs (open-redirect test red).

## Task 17 — Web: session-aware `/ekorm` shell

**Files:** Move `web/src/app/(app)/{layout.tsx,page.tsx}` → `web/src/app/ekorm/{layout.tsx,page.tsx}`; create `web/src/app/page.tsx` (redirect), `web/src/lib/session/{session-provider.tsx,permissions.ts}` (+ tests), `web/src/lib/selection/selection-store.ts` (+ test), `web/src/components/shell/company-switcher.tsx` (+ story/test), `web/src/features/preferences/sync-preferences.ts` (+ test). Modify `nav-config.ts` (+ test: `/ekorm` hrefs, Calendar entry, `permission?: string` per leaf), `app-shell.tsx`/`sidebar.tsx`/`top-nav.tsx`/`user-menu.tsx` (real user, logout, filtered nav), `messages/{tr,en}/shell.json` (calendar, company switcher, logout keys), `_story-user.ts`.

**Interfaces:**
```ts
export function SessionProvider(props: { me: MeResponse; children: ReactNode }): JSX.Element;
export function useSession(): { me: MeResponse; can(permission: Permission): boolean };
export type Permission = MeResponse['permissions'][number];
export function navFor(permissions: readonly string[]): NavEntry[]; // drops leaves whose permission is missing; drops empty groups
export function useSelection(): { companyId?: string; buildingId?: string; analyzerId?: string; set(p: Partial<Selection>): void };
```
Nav leaf permissions: `solarPlants`, `financial` → `nav.solar_plants`/`nav.financial`; everything else `nav.core`; disabled placeholders stay visible and disabled (01 §5).

- [ ] **Failing tests:** `permissions.test.ts` — `navFor(PermissionsFor(building_admin))` (fixture `web/src/lib/session/permissions.fixture.json` = `{role: permissions[]}`; Go `TestPermissionsFixtureMatchesTable` in `internal/auth` reads that file and fails when it differs from `PermissionsFor`, so the two cannot drift) lacks `solarPlants`,`financial`; admin has all; demo lacks both. `layout.test.tsx` (server component unit with mocked `getMe`) — null → `redirect('/auth/login')`; user rendered in `UserMenu` (name, role label `Şirket Yöneticisi`). `user-menu.test.tsx` — logout posts `/auth/logout` then navigates `/auth/login`. `company-switcher.test.tsx` — only rendered with `admin.companies`; choosing a company stores `companyId` and clears building/analyzer. `selection-store.test.ts` — per-user key isolation; corrupted JSON ignored. `sync-preferences.test.ts` — preference change PATCHes `/profile` debounced 500 ms. Update `shell.spec` stories if hrefs changed (a11y harness).
- [ ] Implement; lint/typecheck/i18n/contrast/tests; `pnpm build`; standalone SSR smoke: start `node .next/standalone/server.js` with `EKOKOD_INTERNAL_API_URL` pointing at a local `ekokod api` (e2e stack from Task 18 may be used) and `curl -I /ekorm` without cookies → 307 to `/auth/login?next=%2Fekorm`; commit `feat(f6a): task-17 — session-aware /ekorm shell with role-filtered navigation`.
- [ ] **Mutation:** `navFor` ignoring permissions (test red).

## Task 18 — e2e harness and `auth.spec`

**Files:** Modify `web/playwright.config.ts` (R171), `web/package.json` (`test:e2e`), `Makefile` (`web-e2e`), CI workflow (e2e job with postgres+redis services, 30 min). Create `web/scripts/e2e-api.sh`, `web/tests/e2e/{fixtures.ts,auth.spec.ts}`.

- [ ] **`auth.spec.ts` tests (09 acceptance):**
  - `each of the six roles logs in and sees exactly its navigation` — for each fixture user: login via UI, land on `/ekorm`, sidebar leaf ids equal the literal expected list per role (admin/CA/CR: all core + solar + financial; BA/BR/demo: core only), `admin` sees company switcher, others do not.
  - `remember me keeps a persistent refresh cookie` — cookie `ekokod_rt` `expires` ≈ now+30 d; without → session cookie (`expires === -1`).
  - `device mismatch forces re-login with the reason` — login, then a new browser context with the same cookies and a different `userAgent` opens `/ekorm` → URL `/auth/login?reason=device_mismatch` and the reason text is visible.
  - `logout returns to login and protects /ekorm` — after logout, `/ekorm/settings` → login with `next`.
  - `expired access token refreshes transparently` — delete `ekokod_at` cookie, reload `/ekorm` → still in the app, new `ekokod_at` present.
  - `forgot password always confirms` — unknown e-mail shows the confirmation.
- [ ] Run in the foreground: `free -m`; `cd web && pnpm build && PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1`; commit `test(f6a): task-18 — Playwright e2e project with the real API and auth.spec`.
- [ ] **Mutation:** web middleware skipping the refresh branch (transparent refresh test red).

## Task 19 — Phase end: acceptance, self-review, handoff

- [ ] **Whole-phase self-review (authorisation/tenancy first):** read `git diff 24b53ca..HEAD -- internal/api internal/service internal/auth internal/store internal/apiwire`; check every route row's role set against 05; every handler resolves ids through scoped services; no `SystemScope`/`AllBuildings` constructed outside `service/auth.ResolveScope`, seed and jobs (`git grep -n 'AllBuildings: true\|SystemScope(' internal/api internal/service`); `admin` package imported only by `apiwire`, `worker`, `cli`, `seed`; secrets: `git grep -n 'password\|secret' internal/api/v1/dto` returns only write-only request fields; error messages never include causes. Fix findings with tests.
- [ ] **Acceptance evidence** (real output recorded in the ledger): `go test ./internal/api/... -run TestAuthorizationMatrix -v`; `-run TestCredentialsNeverLeak -v -tags=integration`; `-run 'TestDeviceMismatch|TestMobileBearer|TestDemoSees|TestBuildingAdminCannotReach|TestWeekendAndVacationChangeLoadProfileHTTP' -tags=integration -v`; `pnpm check:i18n-parity`; `pnpm exec playwright test tests/e2e/auth.spec.ts`; `internal/arch` once; package-by-package `-race` for every touched Go package; `golangci-lint run --concurrency 2` on touched trees; `make check-generate`; `make openapi` clean diff; `pnpm lint typecheck test build check:api check:contrast`; storybook build + scoped a11y for changed stories; `pnpm audit --audit-level=high`; `govulncheck` (make vuln) once.
- [ ] Mark 09 §F6 acceptance items delivered by F6a vs F6b in the handoff. **F6b must pick up** (carry verbatim): the F5 items not in F6a — `BuildingAnalyzerPicker` wired to `useSelection`, card-grid stagger (D26), map tile URL (Q5), every page on `PageHeader`/`FilterBar`/`PeriodFilterBar`/`MetricCard`/chart wrappers/`DataTable`+`ExportMenu`/`DataQualityBadge`; Settings tabs gated by `permissions`; the eight seasonal label test; sectoral CSV export; e2e specs `dashboard`, `consumption`, `load-profile` (+ settings, calendar).
- [ ] Rewrite `HANDOFF_NEXT_SESSION.md` for **F6b** (state, what F6a built, deviations, rulings R136–R186, F6b must pick up, PO questions, environment, paste-ready Turkish prompt); commit; stop.

---

## Acceptance map (09 §F6)

| 09 acceptance item | Delivered by |
|---|---|
| Six roles log in and see exactly the navigation and tabs | F6a T17/T18 (navigation); F6b (Settings tabs) |
| Every mutating endpoint 403 for both read-only roles, one test per endpoint | T5 matrix + T14 HTTP sweep |
| Building-admin cannot read another building via any endpoint | T14 (+ per-task tests) |
| Device-fingerprint mismatch invalidates and redirects with reason | T4, T6, T18 |
| Dashboard renders with real seeded data; markers active/passive | F6b (API: T8 R163, T13 demo) |
| Sectoral comparison visible, populated, ranked, CSV | F6b (API: T9) |
| Consumption table shows every §7.3 column | F6b (API: T10 `TestConsumptionColumnSet`) |
| Eight seasonal labels correct | F6b (keys: T10) |
| Integration credentials never in any response | T12 |
| Demo sees only synthetic data | T13 |
| Calendar/vacation changes `/load-profile` weekday/weekend (R137 reading) | T12 |
| Mobile bearer authenticates; expired/tampered rejected with codes | T6 |

## Open questions for the product owner (defaults ship)

- **Q-A1** Two-step verification and self-registration are not built (R151). Needed?
- **Q-A2** Should an admin-created user be forced to change the password at first login?
- **Q-A3** Password-reset e-mail uses the user's company SMTP settings; a platform SMTP is not configured anywhere. Add one?
- **Q-A4** Sectoral comparison is cross-tenant; F6a hides peers and requires ≥ 3 peers (R162). Acceptable, and is 0.469 kg/kWh (`grid_electricity_tr_2022`) the factor to show?
- **Q-A5** Mobile sessions last 30 days and are device-bound like web (R141/R142). OK?
- **Q6 (F5)** Calendar sits top-level after Reports (R167).
