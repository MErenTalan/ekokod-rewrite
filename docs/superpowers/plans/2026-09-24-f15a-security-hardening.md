# F15a — Security Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans`. Inline, one session, no subagents.

**Goal:** 09 §F15's security review, done and recorded: dependency audit clean or mitigated, security
headers and a nonce-based CSP on every web page, API responses hardened, a browser floor declared, and
`docs/security-review.md` recording each item's evidence. F15b is performance and observability; F15c is
backup/restore, the offline bundle and the handover documents.

**Findings that shaped the plan (read before writing it):**
- `govulncheck ./...`: one reachable vulnerability — **GO-2026-6452**, a panic on a negative shared-string
  index in `github.com/xuri/excelize/v2@v2.11.0` (no fixed version), reached through
  `tariff.ReadSheet` (the uploaded supplier icmal workbook). `pnpm audit --audit-level=high`: clean. CI
  already runs both.
- The web app sends **no** CSP or security headers; the API sets `nosniff` only on ISO downloads.
- Already in place and green: rate limiting with `EKOKOD_TRUSTED_PROXIES` (`middleware.ClientIP`), the
  authorisation matrix and tenancy sweep, `TestEveryMutationAudited`, upload type sniffing + size cap,
  `storage.Open` path confinement.

## Open questions (defaults shipped)

| # | Question | Default | Cost if wrong |
|---|---|---|---|
| Q-L1 | GO-2026-6452 has no fixed excelize version. | Contain it at the untrusted boundary: every workbook read from an upload runs under a recover that turns a panic into a 422 `unreadable_workbook`; recorded in the security review until a fix ships | One malformed upload is refused instead of parsed |
| Q-L2 | CSP strictness. | `script-src 'self' 'nonce-…' 'strict-dynamic'` (no `unsafe-inline`/`unsafe-eval` in production), `style-src 'self' 'unsafe-inline'` (Next/React inline style attributes and chart styles), `object-src 'none'`, `base-uri 'self'`, `form-action 'self'`, `frame-ancestors 'none'`, `img-src 'self' data: blob:` + the map tile origin, `connect-src 'self'` + the map tile origin, `worker-src 'self' blob:` (MapLibre), `font-src 'self' data:`; the analytics script origin is added only when that flag is on. Development adds `'unsafe-eval'` (React refresh) | A third-party embed the PO wants later needs an allow-list entry |
| Q-L3 | Browser floor. | **F5's floor (M-18): Chrome/Edge ≥ 123, Firefox ≥ 120, Safari/iOS Safari ≥ 17.5** (`browserslist` in `web/package.json`), stated in the security review. (A first draft used "last two versions"; corrected in F15b after finding F5's recorded floor.) | Older browsers are not tested |
| Q-L4 | HSTS. | Sent by the web and the API only when `EKOKOD_PUBLIC_URL` is `https://…` (`max-age=31536000; includeSubDomains`); the reverse proxy may also set it | — |

## Rulings (R450–R456)

| Id | Rule |
|---|---|
| R450 | **Untrusted workbooks.** `tariff.ReadSheet` (and any other upload-fed excelize reader) recovers a panic into `ErrUnreadableWorkbook` → 422 `file: unreadable_workbook`. Test: a workbook whose cell references shared string `-1`. |
| R451 | **API headers** (`middleware.SecurityHeaders`, first in the chain): `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'` (the API serves JSON and files, never HTML), `Cross-Origin-Resource-Policy: same-origin`, HSTS per Q-L4. File downloads keep their own `Content-Type`/`Content-Disposition`. |
| R452 | **Web headers + CSP** in `middleware.ts` for **every** route (public site, auth, app, redirects): a fresh 128-bit base64 nonce per request, the policy of Q-L2 on the response, and the nonce passed on the request headers so Next applies it to its own scripts. Plus `X-Content-Type-Options`, `Referrer-Policy: strict-origin-when-cross-origin`, `X-Frame-Options: DENY`, `Permissions-Policy: camera=(), microphone=(), geolocation=(), payment=()`, HSTS per Q-L4. The matcher excludes `_next/static`, `_next/image` and `favicon`. |
| R453 | **No CSP violations**: an e2e spec visits the homepage, login, the dashboard, a map page and a chart page, fails on any `securitypolicyviolation` event or a console "Refused to" message, and asserts a nonce appears in the header and on Next's scripts. |
| R454 | **Browser floor** (Q-L3) in `web/package.json` `browserslist`. |
| R455 | **`docs/security-review.md`**: each 09 §F15 security item with its evidence (test names, commands, results, dates) and every accepted residual risk (GO-2026-6452, `style-src 'unsafe-inline'`). |
| R456 | **Gates:** `make vuln` and `pnpm audit --audit-level=high` in the phase-end verification; the ML service's Python dependencies are listed as PENDING (no auditor available offline here). |

## Tasks
1. R450 + test.
2. R451 + test.
3. R452 + unit tests (nonce per request, policy contents, matcher) + R453 e2e.
4. R454, R455, R456; whole-phase self-review; handoff.
