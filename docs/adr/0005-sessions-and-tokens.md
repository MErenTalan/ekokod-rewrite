# ADR 0005 — Server-side sessions behind short-lived access cookies

Status: accepted

## Context

Browser users and a mobile app both authenticate. Sessions must be revocable per device ("log out all"). The legacy system kept long-lived tokens in local storage.

## Decision

Login creates a server-side session row. The browser gets an HttpOnly, Secure, SameSite access cookie (`ekokod_at`, 15 min) and a refresh cookie (24 h, or 30 days with "remember this device"). Mobile clients get the same pair as bearer tokens. Refresh rotates tokens and detects reuse. Passwords use bcrypt with a pepper and a history of 5. Rate limits apply per client address, which is resolved through `EKOKOD_TRUSTED_PROXIES`.

## Consequences

Revocation is immediate at the next refresh and within 15 minutes for an access token. The CSRF surface is limited by SameSite plus the JSON-only API. See `docs/security-review.md`.
