# ADR 0006 — Next.js web app with an API client generated from OpenAPI

Status: accepted

## Context

The UI is large (about 20 screens, 6 roles, Turkish and English, light and dark) and must meet WCAG 2.1 AA (07 §9). The API is the only source of truth for data and permissions.

## Decision

A Next.js (App Router, standalone output) app in `web/`. Its API types are generated from the OpenAPI document that the Go route table produces (`make openapi`). The UI kit follows the design system in `design-system/` with Storybook stories. Every story is swept by axe in both themes and both locales, and every page by `tests/e2e/a11y-pages.spec.ts`. Permissions come from `/me` and only hide UI; the API enforces them.

## Consequences

An API change that breaks the client fails `pnpm check:api` and typecheck. Accessibility regressions fail CI's a11y sweep. The static API reference `docs/api/reference.md` is generated from the same document.
