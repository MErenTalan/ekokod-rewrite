# Security review (09 §F15)

The evidence behind each item of 09 §F15's security review, and every residual risk accepted with it.
Re-run the commands before each release; update the dates and results here.

Reviewed: 2026-09-24, branch `phase/f15-hardening` (F15a, plan
`docs/superpowers/plans/2026-09-24-f15a-security-hardening.md`, rulings R450–R456).

## 1. Dependency audit

| Scope | Command | Result (2026-09-24) |
|---|---|---|
| Go modules | `make vuln` (`govulncheck ./...`) | **1 reachable finding: GO-2026-6452** (excelize panic on a crafted workbook, no fixed release) — **contained** (R450, below). No other reachable vulnerability |
| Web (pnpm) | `cd web && pnpm audit --audit-level=high` | No known vulnerabilities |
| ML service (Python, `ml/uv.lock`) | — | **PENDING**: no auditor is available offline on the build machine; run `uv pip audit`/`pip-audit` against `ml/uv.lock` on a connected machine before release |

CI runs `govulncheck` and `pnpm audit --audit-level=high` on every push (`.github/workflows/ci.yml`).

**GO-2026-6452 containment (R450).** The only upload-fed excelize reader is the supplier icmal
import (`internal/service/tariff/sheet.go`). A ~50 KB workbook whose shared-string table inflates past
excelize's 16 MiB in-memory limit and whose cell points at string −1 panics `GetRows`. The reader now
recovers and refuses the file as `unreadable_workbook` (422). `TestACraftedWorkbookIsRefusedNotAPanic`
builds that workbook and reproduces the real panic. When excelize ships a fix, upgrade and keep the test.

## 2. Secret handling

- Integration credentials and SMTP passwords are sealed with AES-256-GCM, with the row's own ids as
  additional data, so a ciphertext moved to another row does not open: `TestNoPlaintextSecretColumns`,
  `TestIntegrationCredentialSecretsAreBytea`, `TestIntegrationCredentialsAreNeverReturnedInPlaintext`,
  `TestConfiguredSecretNeverAppearsInListOrJSON`.
- Secrets never reach logs, job runs, messages or errors: `TestLoggerRedactsSecretAttributes`,
  `TestLoggerRedactsSecretsBehindLogValuer`, `TestNewCommandLoggerRedactsSecretLookingAttributes`,
  `TestIngestionSecretsNeverReachRunsCursorsMessagesOrLogs`, `TestHTTPErrorsNeverContainSubstitutedSecrets`,
  `TestCredentialsNeverFormatSecrets`.
- Passwords: bcrypt with a pepper; migrated legacy hashes stay `legacy$…` and are upgraded at login (R145).
- Configuration comes from the environment only; a missing secret stops start-up (`TestConfigRejectsMissingSecret`).
- The legacy migration decrypts legacy secrets in memory and re-seals them; plaintext is never written
  (F14 R410, R419).

## 3. Authorisation

- The route × role matrix covers every route and passes in full: `TestAuthorizationMatrix`, `TestMatrixCoversEveryRoute`.
- Tenancy: `TestScopeIsolation` / `TestScopeIsolationCoversEveryRepository` (every scoped repository),
  `TestBuildingAdminCannotReachOtherBuildingsAnyEndpoint` (every `{id}` route with a foreign id).
- Files are reachable only through their authorising handler: `TestFileAuthorization`.

## 4. Uploads and path traversal

- Type is decided by magic bytes, against a configured allow-list, with a streamed size cap that leaves
  nothing behind: `TestMagicBytesDecideNotTheExtension`, `TestTypeMustAlsoBeConfigured`,
  `TestTooLargeLeavesNothing`, `TestIcmalUploadTooLarge`, `TestIcmalUploadRejectsUnreadableFiles`.
- Stored paths are resolved inside the company directory, symlinks included: `TestPathTraversal`, `TestCleanName`.
- The migration's artifact copy refuses a legacy path that resolves outside its source root:
  `TestAnArtifactPathCannotLeaveItsSourceRoot`.

## 5. Rate limiting and client addresses

- Separate API and login limits (`EKOKOD_RATE_LIMIT_API`, `EKOKOD_RATE_LIMIT_AUTH`): `TestLoginRateLimited`, `TestParseRateLimit`.
- The client address is the rightmost hop not in `EKOKOD_TRUSTED_PROXIES`; with no trusted proxy the
  socket address is used and `X-Forwarded-For` is ignored: `TestClientIPTakesTheRightmostUntrustedHop`.
  **Set `EKOKOD_TRUSTED_PROXIES` to the reverse proxy's address** in production, or every client shares
  the proxy's limit bucket.

## 6. Audit log

Every mutating route writes an audit entry: `TestEveryMutationAudited` (runs over the route table).

## 7. Response headers and the Content Security Policy

- **API (R451)**: `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`,
  `Cross-Origin-Resource-Policy: same-origin`, HSTS when `EKOKOD_PUBLIC_URL` is HTTPS; JSON responses get
  `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'` (files do not: Chrome's PDF viewer
  refuses to render under one). Tests: `TestSecurityHeaders`, `TestAJSONResponseGetsTheLockedDownCSP`.
- **Web (R452)**: a fresh 128-bit nonce per request; `script-src 'self' 'nonce-…' 'strict-dynamic'`
  (no `unsafe-inline`/`unsafe-eval` in production), `object-src 'none'`, `base-uri 'self'`,
  `form-action 'self'`, `frame-ancestors 'none'`, the map tile origin in `img-src`/`connect-src`, the
  analytics origin only when that feature is on; plus `nosniff`, `strict-origin-when-cross-origin`,
  `DENY`, a closed `Permissions-Policy`, and HSTS on HTTPS. Unit tests `src/lib/security/csp.test.ts`,
  `src/middleware.test.ts`; e2e `tests/e2e/security-headers.spec.ts` visits the public site, login and the
  chart/map screens and fails on any CSP violation (proved by removing the nonce from the policy: every
  script was refused and the spec failed).

## 8. Supported browsers

The last two versions of Chrome, Edge, Firefox and Safari, and iOS Safari 16 or later
(`web/package.json` `browserslist`). Older browsers are not tested.

## Residual risks (accepted)

| Risk | Why accepted | Revisit when |
|---|---|---|
| GO-2026-6452 in excelize | no fixed release; the only reachable path is contained and tested | excelize ships a fix |
| `style-src 'unsafe-inline'` | React style attributes and chart styles need it; script execution stays nonce-only | a style-nonce strategy is worth its cost |
| `/metrics` is served by the API router | the reverse proxy must route only `/api/`, `/health/` and the web app; F15b restricts the endpoint | F15b (observability) |
| ML Python dependencies unaudited here | no offline auditor | before release, on a connected machine |
