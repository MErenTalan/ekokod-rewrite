> **Status (2026-09-24):** implemented. Phases F0–F15 are built in this repository. The documents below
> are the specification the code was built from; later rulings amend them in `docs/superpowers/plans/`.
> Start from the repository [README](../../README.md).

# BCEM Energy — Full Rewrite Documentation Set

This folder is the **complete, self-contained specification** for rebuilding the BCEM Energy
platform from zero on a new technology stack. It is written to be handed to a fresh AI coding
session that has **no access to the legacy codebase**.

Everything the legacy product does — every page, tab, filter, chart, table column, modal,
export, integration, scheduled job, calculation rule and role restriction — is described here in
terms of **intent and behaviour**, never as code to copy.

---

## Copy this folder into the new repository

```
new-repo/
└── docs/
    ├── README.md
    ├── master_prompt.md
    ├── 01-project-context.md
    ├── 02-domain-rules.md
    ├── 03-target-architecture.md
    ├── 04-data-model.md
    ├── 05-api-contract.md
    ├── 06-integrations.md
    ├── 07-design-system.md
    ├── 08-migration.md
    ├── 09-implementation-plan.md
    ├── 10-removed-behaviours.md
    └── appendix/
```

---

## Reading order

| Order | Document | Purpose |
|-------|----------|---------|
| 1 | `master_prompt.md` | The prompt to paste into the new AI session. Rules of engagement, non-negotiables, workflow. |
| 2 | `01-project-context.md` | What the product is, who uses it, and an exhaustive inventory of every page and feature. |
| 3 | `02-domain-rules.md` | The exact calculation rules that must be reproduced. Energy, tariffs, billing, reactive penalty, carbon. |
| 4 | `03-target-architecture.md` | The new system's shape: Go backend, Postgres/TimescaleDB, Next.js, Python AI service. |
| 5 | `04-data-model.md` | Complete database schema with DDL. |
| 6 | `05-api-contract.md` | Every REST endpoint the Go backend exposes. |
| 7 | `06-integrations.md` | The six external data sources, their protocols and field mappings. |
| 8 | `07-design-system.md` | Visual direction and the mandatory design workflow. |
| 9 | `08-migration.md` | Moving the running production system onto the new platform. |
| 10 | `09-implementation-plan.md` | The phased build plan. This is what you execute. |
| 11 | `10-removed-behaviours.md` | Legacy bugs and inconsistencies deliberately **not** carried over. **Review this before starting** — if any of them was intentional, it must be re-added. |

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
2. **Nothing is copied.** No legacy source is available and none should be reconstructed.
   The new implementation is written fresh against the described behaviour.
3. **Calculations are contractual.** The formulas in `02-domain-rules.md` are exact. They are
   locked with golden-file tests before any UI work begins.
4. **The design is not improvised.** All UI work goes through the `ui-ux-pro-max` skill.
   See `07-design-system.md`.
5. **Known bugs are not reproduced.** See `10-removed-behaviours.md`.
