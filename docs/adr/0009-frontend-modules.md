# ADR 0009 — EKORM and EKOCM as sibling frontend modules of one app

Status: accepted (F17, 2026-09-26)

## Context

Legacy ran the carbon module as a second Next.js app at `/ekocm`. The rewrite had folded it into EKORM as one tabbed page at `/ekorm/carbon-footprint`. The product owner wants it back as a separate module, but without a second deployment, login or API.

## Decision

`web/src/modules/registry.ts` lists the modules. Each module has a prefix, a home, a nav and a brand. `app/ekorm` and `app/ekocm` are real route segments whose layouts share `ModuleLayout`, which provides the session guard, the entitlement hook `modulesFor(me)` and the `AppShell` for that module. The backend, `/api/v1/*`, auth cookies, roles and `SessionProvider` are shared. A sign-in without `next` lands on `/modules`, and the top bar switches between modules on the same session. Three guards keep module code apart:

- ESLint `no-restricted-imports` patterns, driven by `feature-owners.json`;
- an import-walk vitest;
- `pnpm check:module-chunks` over the built client chunks.

## Consequences

A new module is a registry entry, a nav file, a layout and a line in `feature-owners.json`. Per-user module grants need only `modulesFor` (and later a `modules` field on `/auth/me`); today everyone gets both. Old carbon URLs 308 to their `/ekocm` pages.
