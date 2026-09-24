# F14c — Recompute, Reconcile, Runbook, Rehearsal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans`. Inline, one session, no subagents.

**Goal:** 08 §7–§8 and §11's remaining deliverables:
- `ekokod recompute consumption|bills|reports|carbon|forecasts`: idempotent, resumable, progress-reporting, parallel per company, failures collected (never abort the run);
- `ekokod migrate legacy reconcile`: the per-company HTML + CSV report with an attributed reason on every out-of-tolerance difference;
- `docs/runbook-migration.md` (durations, checkpoints, rollback);
- `make migration-rehearsal` (extract → transform → artifacts → load → recompute → reconcile).

**Architecture:**
- `internal/migrate/recompute`: orchestration over narrow interfaces (the worker's job handlers, admin repositories). The CLI (`internal/cli/recompute.go`) builds the worker graph with `worker.Build` and calls the handlers in-process, so every recomputed bill/report/accrual/forecast writes the same `job_runs` rows and uses the same deterministic task ids as production.
- `internal/migrate/legacy/reconcile*.go`: pure comparison + attribution + HTML/CSV rendering; the data comes from `admin.ReconcileRepository` (read-only SQL) plus the transform directory (`summary.json`, `load_report.json`, `artifacts_report.json`, `meter_readings.ndjson`, `notes.ndjson`) and the extract (`manifest.json`, `carbonfootprint`).

**Spec:** 08 §7, §8, §10, §11; 09 §F14; 02 §3.2, §6.2–§6.8, §7.1; 10-removed-behaviours items 1–8.

## Open questions (defaults shipped)

| # | Question | Default | Cost if wrong |
|---|---|---|---|
| Q-K1 | Does `recompute bills` supersede bills that already exist? | **No, unless `--force`.** Without it a rerun converges (Generate keeps a live bill; a flagged one is retried, I-8), so the command is idempotent and resumable; `--force` re-issues everything (new rows, old ones superseded) | An operator who fixed a tariff must pass `--force` |
| Q-K2 | Which bill scopes? | Building **and** analyzer bills for every billable building (legacy has both histories to compare), then company bills — all periods from `--from` to each building's latest closed period | Longer recompute |
| Q-K3 | The accrual's 400-day lookback (R311) blocks a historical recompute. | The job API keeps it; `recompute carbon` calls a new operator-only `carbon.Service.Recompute(day)` without the lower bound (still never today or later) | — |
| Q-K4 | `recompute forecasts` when the ML service is down. | The failure is collected and printed; exit code 1 only when every company failed | — |
| Q-K5 | Reasons beyond 08 §8's list. | Four more, each a deliberate rule of the rewrite, never `unexplained`: `monomial_power_charge_dropped` (R125 / F14b note), `reactive_not_applicable` (02 §6.6: monomial, residential/lighting, generation in the period), `automated_carbon_recomputed` (legacy daily carbon used an emission rate of 1), `multiplier_unconfirmed` (Q-J13 withheld readings). The runbook lists all twelve | The PO sees four more reasons to accept |
| Q-K6 | Which consumption figure is "legacy monthly consumption per analyzer"? | Legacy `consumptions` is discarded (08 §4); the only legacy figure left is the analyzer bill history's `totalActiveKWh`, compared with the recomputed analyzer bill's `active_import` for the same period key | — |
| Q-K7 | Report location and format. | `--out reports/` → `reconciliation.html` (all companies, one section each, overall pass/fail on top) and `reconciliation.csv` (one row per difference); exit code 2 when any `unexplained` exists (cutover blocker, 08 §8) | — |

## Rulings (R430–R444)

| Id | Rule |
|---|---|
| R430 | **recompute consumption `--from --to`**: refresh `consumption_hourly → daily → monthly → yearly`, then `plant_production_daily → monthly`, in one-month windows (bounded refresh transactions), printing each window. Idempotent by construction (a refresh converges). |
| R431 | **recompute bills `--from YYYY-MM [--to] [--force] [--parallel 2]`**: companies in parallel (bounded), inside a company sequentially: each billable building's periods from `--from` to its latest closed period (R233 cut-off) → building bill, then each of its analyzers' bills, then the company bill per key. Each call is `JobGenerator.Generate` (job_runs row, render enqueued for new bills). A failure is recorded `{company, scope, subject, period, error}` and the run continues. |
| R432 | **recompute reports `--from YYYY-MM`**: monthly reports per billable building and period, yearly reports per closed year, through `ReportGenerator.Generate` (selection `all`). |
| R433 | **recompute carbon `--from DATE`**: every day from `--from` to yesterday through `Service.Recompute` (Q-K3). |
| R434 | **recompute forecasts**: `ForecastJobs.RunTask` once (current horizon, every analyzer). |
| R435 | **Output**: every recompute prints progress lines and a JSON summary `{processed, failed, failures[]}` and writes it to `--report FILE` when given; exit 1 when anything failed. |
| R436 | **Reconcile tolerances (08 §8)**: row counts exact; readings per analyzer-month exact; monthly consumption per analyzer ±0.1 %; monthly invoice per analyzer ±1 %; monthly invoice per building ±1 %; reactive penalty applied exact; annual consumption per building ±0.1 %; annual invoice per building ±1 %; carbon per building-year ±1 %; artifact count and bytes exact. A zero legacy value compares by absolute difference (> 0.01). |
| R437 | **Row counts**: per legacy collection, manifest count = accepted + rejected (the transform summary), and per table load `lines = loaded + withheld`; any mismatch is `unexplained`. |
| R438 | **Readings**: per analyzer and Istanbul month, transform output lines vs `meter_readings` rows; a withheld analyzer's months carry `multiplier_unconfirmed`. |
| R439 | **Invoice attribution, component by component**. A component differs when \|Δ\| > max(0.5 TL, 0.5 % of the legacy total). Reasons: energy — `no_tiering_on_multi_time` (multi-time tariff and legacy `isTwoPartCalculation`), `ptf_gap_flagged` (new bill flagged for PTF gaps); distribution — `distribution_on_total_consumption` (legacy distribution ≈ price × legacy low-tier kWh within 1 %); capacity — `capacity_cost_removed` (legacy `capacityCost` > 0); power — `monomial_power_charge_dropped` (monomial tariff, legacy `powerCost` > 0); reactive — `reactive_exempt_below_9kw` (installed power < 9 kW), `reactive_not_applicable` (monomial, residential/lighting group, or active export > 1 kWh); VAT — `vat_rate_corrected` (legacy implied rate ≠ tariff rate by > 0.1 pt; otherwise VAT and other taxes are derived and carry their base's reasons). A differing component with no matching condition adds `unexplained`. A new bill that is absent or flagged for a meter reset gives `meter_reset_withheld`; flagged for PTF gaps, `ptf_gap_flagged`; absent for any other reason, `unexplained`. |
| R440 | **Carbon**: legacy = Σ `emissionCo2e` of every legacy activity (manual + automated) by `date` year; new = Σ `emission_kgco2e` by `period_start` year; a year with legacy automated rows carries `automated_carbon_recomputed`. |
| R441 | **Artifacts**: `artifacts_report.json` copied + already there = `stored_files` rows with legacy owner types and their bytes; referenced-but-missing files are listed (not a pass/fail figure: they were never on disk). |
| R442 | **Report**: per company: pass/fail (fail ⇔ any `unexplained`), counts per reason, and a table of every out-of-tolerance figure (check, subject, period, legacy, new, Δ, Δ %, reasons). Global section: row counts, artifacts. CSV has the same rows with a company column. HTML is self-contained (inline CSS, no script). |
| R443 | **Runbook** `docs/runbook-migration.md`: preconditions (off-host backup, capacity), the command sequence with expected durations (measured on the fixture, to be replaced by rehearsal timings), checkpoints, the manual answers (multipliers, tariff classes, ISO buildings, templates), the verification list (08 §9), cutover (08 §10) and a tested rollback procedure (stop new stack, repoint proxy, restart legacy, reinstall cron; legacy DB never written). |
| R444 | **`make migration-rehearsal`**: runs the whole chain against `$LEGACY_URI`/`$LEGACY_DB` (or `EXTRACT=dir`) into a scratch database, timing each step into `reports/rehearsal-<date>.txt`. The three real rehearsals stay PENDING until a dump exists. |

## Tasks
1. `internal/migrate/recompute` + `carbon.Service.Recompute` + CLI `ekokod recompute …` (unit tests with fakes: period enumeration, failures collected, no force by default, parallel bound).
2. `admin.ReconcileRepository` (read-only queries) + integration test.
3. Reconcile logic: attribution (a test per reason), checks, report model.
4. Rendering (HTML/CSV) + CLI `migrate legacy reconcile` + an end-to-end integration test (fixture → transform → load → bills generated → reconcile: a matching bill passes, a doctored one is `unexplained`, a tiered multi-time one is attributed).
5. Runbook + `make migration-rehearsal` (script) + a dry run on the fixture.
6. Whole-phase self-review, handoff.
