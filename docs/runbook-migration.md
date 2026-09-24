# Runbook — migrating the legacy system (bcem-energy → ekokod)

This is the operator's procedure for 08-migration.md: moving the running legacy system (MongoDB +
files) onto the new stack **on the same host**, with a rollback that is always available.

- **The legacy database is never written.** Every `ekokod migrate legacy` command reads it (the
  Mongo adapter refuses any write command) or reads files produced from it.
- **Every command is idempotent.** Running it again converges; nothing is duplicated.
- **Nothing is dropped silently.** Each rejected record is in `rejects.ndjson` with a reason, and
  every collection is checked for `input = accepted + rejected`.

> **Rehearse first.** The whole chain runs at least **three times** on a copy (§8) before the real
> cutover. The durations below are placeholders until the first rehearsal replaces them.

---

## 1. Roles and inputs

| Who | Provides |
|---|---|
| Operator | Runs the commands, keeps the log, performs the cutover and any rollback |
| Product owner (PO) | Answers the `manual_*.csv` questions, reviews the rejections, accepts each reconciliation reason in writing |
| Legacy maintainer | The legacy encryption key(s) (`EKOKOD_LEGACY_ENCRYPTION_KEY`, optional `…_FALLBACK`), read-only Mongo credentials, artifact directories |

Inputs: `LEGACY_URI` + `LEGACY_DB` (a **read-only** Mongo user), the legacy artifact roots
(`/opt/bills`, `/opt/reports`, `/opt/documents`, and the legacy app directory that holds `uploads/`),
the new stack's configuration (`EKOKOD_*`; `ekokod config check` validates it).

## 2. Preconditions — do not start without them

1. **Off-host backup, verified** (08 §3):
   ```bash
   mongodump --uri "$LEGACY_URI" --out /backup/pre-migration-$(date +%F)
   tar czf /backup/artifacts-$(date +%F).tar.gz /opt/bills /opt/reports /opt/documents /opt/bcem/uploads
   sha256sum /backup/*.tar.gz > /backup/checksums.txt
   ```
   Copy `/backup` off the machine and verify the checksums there.
2. **Capacity**: disk for the Postgres copy + WAL + the artifact copy; RAM for both database engines.
   If the host cannot hold both, use a second host and synchronise artifacts at cutover.
3. The new stack is installed, `ekokod config check` passes, and the target database is empty or a
   previous rehearsal's (every step converges).

## 3. The sequence

Run from the new stack's host with its environment loaded. Each step prints a summary; keep the
terminal log. `DIR=/srv/migration/<date>`.

| # | Step | Command | Checkpoint | Duration (fixture / rehearsal) |
|---|---|---|---|---|
| 1 | Inventory | `ekokod migrate legacy inventory --uri "$LEGACY_URI" --db "$LEGACY_DB" --artifacts /opt/bills,/opt/reports,/opt/documents --json $DIR/inventory.json` | **Review it with the PO before continuing** (08 §3): counts, analyzers by provider, gaps, negative deltas, missing multipliers, tariffs missing VAT/KBK, the invoice baseline, orphaned files | < 1 s / _TBD_ |
| 2 | Extract | `ekokod migrate legacy extract --uri … --db … --out $DIR/extract` | `manifest.json` lists every collection with a count and SHA-256 | < 1 s / _TBD_ |
| 3 | Transform | `ekokod migrate legacy transform --extract $DIR/extract --out $DIR/out [--answers answers.csv] [--confirmations manual_multipliers.csv] --now <RFC3339>` | `summary.json`; review `rejects.ndjson`, `notes.ndjson`, every `manual_*.csv` (§4) | < 1 s / _TBD_ |
| 4 | Artifacts | `ekokod migrate legacy artifacts --dir $DIR/out --source /opt/bills,/opt/reports,/opt/documents,/opt/bcem` | `artifacts_report.json`: referenced-but-missing and unreferenced files, both reviewed | < 1 s / _TBD_ |
| 5 | Schema + reference data | `ekokod migrate up && ekokod seed` | `migrate status` has no pending migration | 1 s / _TBD_ |
| 6 | Load | `ekokod migrate legacy load --dir $DIR/out` | `load_report.json`: `lines = loaded + withheld` per table; withheld readings = unconfirmed multipliers | 1 s / _TBD_ (dominated by `meter_readings`) |
| 7 | Recompute | `ekokod recompute consumption --from <first day>`; `recompute bills --from <first month>`; `recompute reports --from <first month>`; `recompute carbon --from <first day>`; `recompute forecasts` | each prints `{processed, failed, failures[]}`; failures are data problems to review, the run never aborts | 1–2 s / _TBD_ |
| 8 | Reconcile | `ekokod migrate legacy reconcile --dir $DIR/out --extract $DIR/extract --out $DIR/reports` | `reconciliation.html` passes: **zero `unexplained`** (§5) | < 1 s / _TBD_ |

`make migration-rehearsal` runs steps 1–8 against a scratch database and times each one into
`reports/rehearsal-<timestamp>.txt` (variables: `FROM`, `LEGACY_URI`/`LEGACY_DB` or `EXTRACT`,
`ARTIFACT_SOURCES`, `ANSWERS`, `CONFIRMATIONS`, and the scratch `EKOKOD_DB_URL`). It refuses
`EKOKOD_ENV=production`.

A **dry run without a dump** (to check the tooling): `EKOKOD_WRITE_FIXTURE_EXTRACT=/tmp/fx go test
./internal/migrate/legacy -run TestWriteFixtureExtract`, then `EXTRACT=/tmp/fx FROM=2026-07 make
migration-rehearsal`. The fixture has almost no readings, so bills report `no_consumption_data` and
reconciliation fails — that is the expected result.

## 4. Manual answers (the PO decides, the operator records)

The transform never guesses. What it cannot determine it lists; the answers go back into step 3.

| File | Question | How to answer |
|---|---|---|
| `manual_multipliers.csv` | Was this analyzer's stored reading already multiplied? (legacy OSOS never applied `meterMultiplier`) | fill the `answer` column with `raw` or `multiplied`; rerun transform with `--confirmations`. Until then load **withholds** its readings |
| `manual_tariff_class.csv` | voltage / user group / term / supply company of a building whose embedded tariffs never stored them | `answers.csv` row `tariff_class,<building legacy id>,lv/commercial/monomial/private` |
| `manual_tariff_template_class.csv` | the same for a template (defaulted to `lv/commercial/monomial/private`, flagged) | `tariff_template_class,<template legacy id>,…` |
| `manual_iso_building.csv` | which building a legacy user's ISO 50001 work belongs to | `iso_building,<user legacy id>,<building legacy id>` |

`answers.csv` has the header `kind,legacy_id,answer`. Rejections to review in `rejects.ndjson` include
`vat_rate_missing`, `kbk_energy_missing`, `classification_unknown`, `ambiguous_date`, `not_bcrypt`
(those users get a forced password reset), `decrypt_failed`, `outside_retention` (logs older than
`--logs-days`, default 180).

## 5. Reconciliation reasons

Every out-of-tolerance figure carries one or more reasons. **`unexplained` blocks cutover.** Every
other reason is a deliberate correction the PO accepts (or contests) in writing.

| Reason | Meaning (spec) |
|---|---|
| `distribution_on_total_consumption` | legacy charged distribution on the low tier only; now on all consumption (02 §6.3) |
| `no_tiering_on_multi_time` | legacy tiered multi-time tariffs at the mean of T1–T3; now no tiering (02 §6.2) |
| `meter_reset_withheld` | a negative index delta: the period is withheld until an operator resolves it (02 §3.2) |
| `reactive_exempt_below_9kw` | installations below 9 kW are exempt from the reactive penalty (02 §6.6) |
| `reactive_not_applicable` | monomial tariff, residential/lighting group, or generation in the period (02 §6.6) |
| `vat_rate_corrected` | the tariff's dated VAT rate replaces legacy's 18 %/20 % constants (02 §6.8) |
| `ptf_gap_flagged` | missing PTF hours flag the invoice instead of under-billing (02 §7.1) |
| `capacity_cost_removed` | the legacy capacity line is gone (02 §6.5) |
| `monomial_power_charge_dropped` | a monomial tariff has no power charge (R125); legacy charged contracted × unit price |
| `automated_carbon_recomputed` | legacy's daily carbon rows used an emission rate of 1; recomputed with the grid factor |
| `multiplier_unconfirmed` | readings withheld until the multiplier question is answered (§4) |
| `derived_discarded` | `consumptions`, `aipredicts`, `epiasdailyaverages`: derived data, recomputed not migrated |
| `unexplained` | **investigate and fix before cutover** |

Tolerances (08 §8): row counts and readings exact; consumption ±0.1 %; invoices ±1 % per analyzer,
per building and per year; carbon ±1 % per building-year; artifact count and bytes exact.

## 6. Verification before cutover (08 §9)

- [ ] Reconciliation shows zero `unexplained`; every reason accepted in writing by the PO.
- [ ] Every rejection reviewed; every manual question answered (no `manual_*.csv` left with rows).
- [ ] `POST /api/v1/integration-credentials/{id}/verify` (Settings → Integrations → Verify) passes for every credential against the live provider.
- [ ] A fresh data pull lands new readings for every provider.
- [ ] This month's invoice for one building matches the real utility invoice within tolerance.
- [ ] Every user can log in (forced resets issued for `not_bcrypt` users); one user per role checked.
- [ ] Alarms evaluate and mail through the migrated SMTP settings.
- [ ] Reports render to PDF and Excel and send; historical (legacy) invoice and report PDFs download.
- [ ] Carbon totals match the reconciliation; ISO 50001 notes and files are present; the export builds.
- [ ] Scheduled jobs run under the leader lock and write `job_runs`.
- [ ] Backup **and restore** tested end to end on a scratch host.

## 7. Cutover (08 §10)

1. Announce the maintenance window.
2. Stop the legacy scheduler (remove the BCEM crontab entries) and the legacy app.
3. Final incremental run: steps 2, 3, 4, 6, 7 (the steps converge, so they only add what changed).
4. Final reconciliation (step 8); **abort on any `unexplained`**.
5. Repoint the reverse proxy to the new stack.
6. Start the new scheduler.
7. Smoke test: login, dashboard, consumption, generate an invoice, download a PDF, trigger an alarm evaluation.
8. Watch job runs and error rates for 24 hours.

## 8. Rollback

Available until decommissioning, because the legacy database is never modified:

1. Stop the new stack (`api`, `worker`, `scheduler`).
2. Repoint the reverse proxy to the legacy app.
3. Start the legacy app and reinstall its crontab entries.
4. Verify: legacy login, its dashboard, and that its cron writes a new `logs` document.

The rollback **trigger** is agreed in advance and written here before cutover: _(to be filled by the
PO and operator)_. Rehearse the rollback once on the rehearsal environment and record it in §9.

## 9. Rehearsal log

| # | Date | Dump | Duration per step | Reconciliation | Rollback exercised | Notes |
|---|---|---|---|---|---|---|
| 1 | _PENDING_ | anonymised production dump | | | | needs a dump + a host |
| 2 | _PENDING_ | | | | | |
| 3 | _PENDING_ | | | | | |

## 10. Decommission

After one further billing cycle and an explicit sign-off: a final legacy backup archived off-host,
then remove the legacy containers and images and reclaim the volumes.
