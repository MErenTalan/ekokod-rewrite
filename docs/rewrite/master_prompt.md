# Master Prompt — BCEM Energy Rewrite

> Paste this document as the first message of the new session.

---

## Your assignment

You are rebuilding **BCEM Energy** (product name in the UI: *EKORM* for the energy platform,
*Eko-CM* for the carbon module) — a multi-tenant energy management and monitoring platform for
the Turkish electricity market.

A previous version of this product exists and runs in production. It works, but it is
structurally unsound: the database grows without bound, scheduled calculations are unreliable,
the integration layer is inconsistent, and heavy computation blocks the request path. **You are
not migrating that code. You are rebuilding the product.**

The `docs/` folder in this repository is your complete specification. It describes what the
product does. It deliberately contains no legacy source code, and you will not have access to
any. Design and write everything fresh.

---

## The stack (decided — do not re-litigate)

| Layer | Technology |
|-------|------------|
| Backend | **Go** (1.23+) — REST/JSON API, all business logic, all scheduled work |
| Database | **PostgreSQL 16 + TimescaleDB** — hypertables for time series, relational for everything else |
| Cache / queue | **Redis 7** |
| Frontend | **Next.js 15 + React 19 + TypeScript + Tailwind CSS** |
| ML service | **Python (FastAPI)** — forecasting and anomaly detection only, no database access |
| Packaging | **Docker Compose**, single host, with an offline/air-gapped install bundle |

Go was chosen because the workload is computation-heavy: hundreds of meters, sub-hourly index
readings, continuous consumption derivation, monthly bill recomputation across historical
tariffs, and PDF/Excel generation.

---

## Non-negotiables

### 1. Feature parity is absolute

`01-project-context.md` is an exhaustive inventory: every page, every tab, every filter, every
chart, every table column, every modal, every export button, every role restriction. Every item
in it must exist in the finished product.

`appendix/i18n-en.json` and `appendix/i18n-tr.json` are the legacy UI string catalogues (~2,750
keys). **Treat them as a checklist.** If a string exists there, the feature it labels must exist
in the rewrite. When you finish a module, grep its key namespace and confirm every key has a home.

If you believe something in the inventory should be dropped, **ask** — do not decide silently.

### 2. Do not reproduce the legacy implementation

You are describing the same *product*, not the same *program*. Structure, naming, module
boundaries, algorithms and data flow are yours to design well. The legacy system's specific
shortcomings that you must **not** recreate:

- Unbounded arrays and `Mixed`-typed blobs embedded in single database documents.
- Business logic living inside HTTP handlers.
- CPU-bound work executed inside request/response cycles.
- Scheduled jobs implemented as OS cron entries calling HTTP endpoints with a shared API key.
- Six integrations each with their own bespoke, divergent data handling.
- Silent failures: `catch` blocks that log and continue, leaving partial state behind.

### 3. Calculations are a contract

`02-domain-rules.md` specifies energy derivation, tariff resolution, bill composition, reactive
penalty, PTF+YEKDEM dynamic pricing and carbon accounting **exactly**. These produce invoices
that customers compare against their real utility bills.

Implement them as **pure functions in a dependency-free domain package**, and lock them with
golden-file tests using the fixtures in Phase 4 *before* wiring any storage or HTTP layer.

### 4. Known legacy bugs are not carried over

`10-removed-behaviours.md` lists behaviours that were wrong in the legacy system and are
deliberately excluded. Read it before you start. Do not reintroduce them.

### 5. The design is not improvised

All user-facing UI work **must** go through the `ui-ux-pro-max` skill. Do not invent a visual
language, do not fall back on default component-library styling, and do not carry over the
legacy admin-template look.

The product's identity is **green energy**: renewables, efficiency, carbon reduction, measurable
environmental impact. `07-design-system.md` contains the direction and the required workflow.
Every phase that touches UI names this explicitly — honour it.

### 6. Turkish is the primary language

Default locale is Turkish; English is fully supported. Domain vocabulary (*endeks*, *reaktif
ceza*, *YEKDEM*, *PTF*, *icmal*, *tesisat numarası*, *dağıtım bedeli*) is Turkish and stays
Turkish in the UI. Code identifiers, database columns, API fields and comments are English.

---

## How to work

### Follow the plan

`09-implementation-plan.md` defines phases F0–F15. Execute them **in order**. Each phase states
its scope, prerequisites, tasks, acceptance criteria and verification commands.

- Do not start a phase before its prerequisites are met.
- Do not declare a phase complete before its acceptance criteria are demonstrably satisfied —
  run the verification commands and show the output.
- If a phase turns out to be wrong or underspecified, stop and say so. Do not improvise around it.

### Verify before you claim

Never report something as working without having run it. Tests, builds, migrations, API calls —
show the actual output. If something fails, say it failed and show the error.

### Ask when it matters

The specification is detailed but not omniscient. When a decision would materially change the
result and the documents do not settle it, ask. When it is a routine judgement call, decide,
state your assumption in your response, and continue.

---

## What "done" looks like

- Every page and feature in `01-project-context.md` exists and works against real data.
- Every calculation in `02-domain-rules.md` passes its golden-file tests.
- All six integrations ingest data reliably, resumably and idempotently.
- Scheduled work runs inside the Go service under a leader lock — no OS cron, no HTTP self-calls.
- No table or row grows without bound; time series live in hypertables with rollups.
- The migration in `08-migration.md` runs against a production dump and produces a reconciliation
  report showing consumption and billing figures within tolerance of the legacy system.
- `docker compose up` brings the whole stack online, and the offline bundle installs on a host
  with no internet access.

---

## Start here

1. Read `docs/README.md`, then every document in the order it lists.
2. Read `10-removed-behaviours.md` carefully and confirm you understand what is excluded.
3. Begin **Phase F0** of `09-implementation-plan.md`.
