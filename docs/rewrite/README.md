> **Status:** implemented. Phases F0–F15 are built in this repository, and later work (F16 onwards)
> is recorded in [`docs/decisions.md`](../decisions.md), which amends these documents where they
> differ. Start from the repository [README](../../README.md).

# BCEM Energy — Rewrite Specification

This folder is the specification the rewrite was built from. It describes the BCEM Energy platform
as it existed before the rewrite, so the new system could be written from scratch on a new
technology stack without carrying the legacy code over.

Everything the legacy product does — every page, tab, filter, chart, table column, modal,
export, integration, scheduled job, calculation rule and role restriction — is described here in
terms of **intent and behaviour**, never as code to copy.

---

## Contents

| Order | Document | Purpose |
|-------|----------|---------|
| 1 | `01-project-context.md` | What the product is, who uses it, and an exhaustive inventory of every page and feature. |
| 2 | `02-domain-rules.md` | The exact calculation rules that must be reproduced. Energy, tariffs, billing, reactive penalty, carbon. |
| 3 | `03-target-architecture.md` | The new system's shape: Go backend, Postgres/TimescaleDB, Next.js, Python forecasting service. |
| 4 | `04-data-model.md` | Complete database schema with DDL. |
| 5 | `05-api-contract.md` | Every REST endpoint the Go backend exposes. |
| 6 | `06-integrations.md` | The external data sources, their protocols and field mappings. |
| 7 | `07-design-system.md` | Visual direction and the design workflow. |
| 8 | `08-migration.md` | Moving the running production system onto the new platform. |
| 9 | `09-implementation-plan.md` | The phased build plan (F0–F15) and each phase's acceptance criteria. |
| 10 | `10-removed-behaviours.md` | Legacy bugs and inconsistencies deliberately **not** carried over. |

Code comments cite these documents by number and section, e.g. `02 §3.1` is
`02-domain-rules.md` section 3.1.

## Appendix

| File | Contents |
|------|----------|
| `appendix/i18n-en.json` | Complete English UI string catalogue from the legacy app (~2,750 keys). The authoritative label inventory: if a string exists here, the corresponding feature must exist in the rewrite. |
| `appendix/i18n-tr.json` | Same catalogue in Turkish. Turkish is the default language. |
| `appendix/i18n-key-inventory.txt` | Flattened `key = value` listing for quick grepping. |
| `appendix/legacy-inventory.md` | Legacy route/endpoint/collection inventory with mapping to the new system. |
| `appendix/env-reference.md` | Configuration variables, old and new. |

---

## Ground rules for the rewrite

1. **Nothing is lost.** Every page, feature, integration and report in `01-project-context.md`
   must exist in the new product.
2. **Nothing is copied.** The new implementation is written fresh against the described behaviour.
3. **Calculations are contractual.** The formulas in `02-domain-rules.md` are exact. They are
   locked with golden-file tests.
4. **The design is not improvised.** UI work follows the design system in `design-system/`.
   See `07-design-system.md`.
5. **Known bugs are not reproduced.** See `10-removed-behaviours.md`.
