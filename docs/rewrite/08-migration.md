# 08 — Migration and Cutover

Moving the running production system onto the new platform, on the **same server**, without losing
history and without a period where the customer has no working product.

---

## 1. Strategy — decided

**Migrate raw data. Recompute everything derived.**

| Category | Action |
|----------|--------|
| Configuration — companies, users, buildings, analyzers, plants, tariffs, alarms, integration credentials, SMTP, calendar, ISO 50001 content, carbon activities, emission factors | **Transformed and loaded** into the new schema. |
| Raw measurements — meter index readings (load profile, daily, billing, reset), plant production, market prices | **Transformed and loaded** into the hypertables. |
| Derived data — consumption aggregates, invoices, reports, forecasts, carbon accruals | **Not migrated. Recomputed** by the new engine from the raw data. |
| File artifacts — historical invoice PDFs, report PDFs and Excel files, uploaded documents | **Copied**, with their metadata registered in `stored_files`. Old invoice PDFs are preserved as issued, alongside the recomputed figures. |

### Why recompute

The legacy derived data is the untrustworthy part: consumption rows written by a job that
frequently failed halfway, invoices computed by code with the defects catalogued in
`10-removed-behaviours.md`, reports built on both. Carrying them forward would import exactly the
problems the rewrite exists to remove.

### The consequence, stated plainly

**Recomputed historical invoices will differ from the legacy ones.** The corrections in
`02-domain-rules.md` — distribution charged on total rather than low-tier consumption, no tiering on
multi-time tariffs, meter-reset periods withheld instead of inflated, PTF gaps flagged instead of
silently under-billed — change numbers. Some invoices will go up, some down.

This is handled deliberately, not discovered later:

1. Every legacy invoice is loaded into a **comparison table**, not into `bills`.
2. The new engine recomputes the same period.
3. A **reconciliation report** lists every difference above tolerance, with the reason.
4. The product owner reviews it **before cutover** and decides, per case, whether the new figure is
   correct or a rule needs revisiting.
5. Legacy invoice PDFs remain downloadable, marked as *issued by the previous system*.

---

## 2. Phases

```
1. Prepare      snapshot, inventory, staging environment
2. Extract      Mongo → newline-delimited JSON, immutable, checksummed
3. Transform    normalise, validate, map to the new schema
4. Load         configuration first, then time series
5. Recompute    consumption → invoices → reports → carbon
6. Reconcile    compare against legacy; produce the report
7. Verify       automated checks + manual review
8. Cut over     switch traffic, keep rollback available
9. Decommission after the agreed observation period
```

Steps 2–6 run against a **copy**. The production system stays untouched and serving until step 8.

---

## 3. Preparation

### Snapshot

```bash
mongodump --uri "$LEGACY_MONGODB_URI" --out /backup/pre-migration-$(date +%F)
tar czf /backup/artifacts-$(date +%F).tar.gz /opt/bills /opt/reports /opt/documents
sha256sum /backup/*.tar.gz > /backup/checksums.txt
```

The snapshot is copied off the machine before anything else happens. **A migration without a
verified off-host backup does not start.**

### Inventory

`bcem migrate inventory` reads the legacy database and prints, without modifying anything:

- Companies, users by role, buildings, analyzers by provider, plants.
- Per analyzer: reading count by kind, earliest and latest timestamp, gaps longer than 24 h,
  detected negative index deltas, missing multipliers.
- Tariffs: count, distinct `effectiveFrom` formats, tariffs missing a VAT rate, tariffs in
  PTF+YEKDEM mode missing an energy KBK.
- Invoices: count by period and scope, and the total amount per period (the reconciliation baseline).
- Carbon activities, emission factors, ISO 50001 notes and files.
- Artifact files: count and total size per directory, and files referenced in the database but
  missing on disk (and the reverse).

The inventory is the migration's specification. **Nothing is written until it has been reviewed.**

### Capacity

The new stack runs alongside the old one during the parallel period. Confirm before starting: disk
headroom for the Postgres copy plus WAL plus the artifact copy, and enough RAM for both database
engines. If the host cannot hold both, the migration runs on a second host and the artifacts are
synchronised at cutover.

---

## 4. Extraction

`bcem migrate extract` writes one gzipped newline-delimited JSON file per collection to a staging
directory, with a manifest recording document counts and a SHA-256 per file.

Extraction is **read-only** against the legacy database and re-runnable. It never transforms.

Collections and their fates:

| Legacy collection | Destination |
|---|---|
| `companies` | `companies`, `integration_credentials`, `company_weekend_days`, `company_vacations`, `calendar_events` |
| `users` | `users`, `user_password_history` |
| `buildings` | `buildings`, `building_contacts`, `tariffs`, `tariff_taxes`, `tariff_manual_yekdem`; `billHistory` → comparison table |
| `analyzers` | `analyzers`; `energyValues.*` → `meter_readings`; `hourlyValues` → cross-check series; `billHistory` → comparison table |
| `consumptions` | **discarded** — recomputed |
| `tariffs` (standalone) | `tariffs` where not already embedded; `national_tariff_schedule` for defaults |
| `tarifftemplates` | `tariff_templates` |
| `powerplants` | `power_plants`, `power_plant_monthly_targets`, `power_plant_devices`, `power_plant_alarm_recipients`, `solar_tariffs`; `santral_values` → `plant_production` |
| `alarms` | `alarms`, `alarm_analyzers`, `alarm_channels`, `alarm_events`, `alarm_fired_bills` |
| `reports` | comparison table; **recomputed** |
| `carbonfootprint` | `carbon_activities`, `carbon_selected_activities` |
| `carbonreporthistories` | `carbon_reports` + artifact registration |
| `companyemissionfactors` | `emission_factors`, `emission_factor_conversions` |
| `epiashistories` | `market_prices_hourly`, `yekdem_monthly` |
| `integrations` | `integration_definitions` |
| `smtpsettings` | `smtp_settings` |
| `logs` | `operational_messages` (retained for a configurable window) |
| `aipredict` | **discarded** — regenerated |

---

## 5. Transformation

`bcem migrate transform` reads the extract and writes Postgres `COPY`-ready files. Every record
passes validation; anything that fails is written to a rejection file with its reason. **Rejections
are reviewed before loading, never silently dropped.**

### Identity mapping

Mongo `ObjectId` → deterministic UUIDv5 derived from a fixed namespace plus the hex id, so the
transform is repeatable and cross-references resolve without a lookup table. The original id is kept
in a `legacy_id` column on every migrated row for traceability.

### Normalisations

| Legacy | New |
|--------|-----|
| Dates as strings in `dd-MM-yyyy`, `yyyy-MM-dd`, `YYYY-MM`, `dd-MMM-yyyy` (Turkish or English month names), ISO 8601 | `date` / `timestamptz`. Ambiguous values are rejected, never guessed. |
| Naive timestamps with no offset | Interpreted as **Europe/Istanbul**, converted to UTC. |
| Numbers stored as strings, sometimes with thousands separators | `numeric`, parsed with an explicit locale-aware parser. |
| `Mixed` / free-form objects | Mapped to typed columns. Anything unmappable goes to the rejection file. |
| Turkish enum values (`mesken`, `ticarethane`, `sanayi`, `tarim`, `aydinlatma`, `ag`, `og`, `tek zamanlı`) | The English enums in `04-data-model.md`. |
| `"enc:"`-prefixed encrypted credentials | Decrypted with the legacy key, re-encrypted with the new key. The legacy key is **required** at transform time and is not stored in the new system. |
| `energyValues.loadProfile[]` and friends — arrays inside a document | One `meter_readings` row per entry, with `reading_kind` set from the array it came from. |
| `billHistory` keyed by `"YYYY-MM"` | `legacy_bills` comparison table. |

### Meter-reading specifics

- **Multiplier.** Legacy readings were stored inconsistently — some pre-multiplied, some raw. The
  transform determines per analyzer and per source whether the multiplier was applied, using the
  provider rules in `06-integrations.md`, and normalises everything to **multiplied** values,
  recording `multiplier_applied` on each row. Analyzers where this cannot be determined confidently
  are listed for manual confirmation. **This is the single highest-risk step in the migration and
  it is reviewed by hand.**
- **Duplicates.** Same `(analyzer, ts, kind)` appearing more than once: keep the last value, count
  the duplicates, and report them.
- **Negative deltas.** Detected during transform and written to `consumption_anomalies` as
  unresolved, so the recompute withholds those periods rather than inventing numbers.
- **Out-of-range.** Timestamps in the future or before the metering point's first plausible reading
  are rejected.
- **ARIL T1/T2/T3.** Legacy wrote `"0"` for these; the transform converts `0` on ARIL-sourced rows
  to `NULL` (see `06-integrations.md` §4).

### Tariff specifics

- Both the embedded `building.tariffs[]` history and the standalone `tariffs` collection are read;
  duplicates are reconciled by `(building, effective_from)` with the more complete record winning.
- Tariffs missing a VAT rate are **rejected** and listed for the product owner to supply, since
  `vat_rate` is now required.
- PTF+YEKDEM tariffs missing `kbk_energy` are rejected for the same reason.
- The legacy scalar `other_taxes_rate` is converted into a single named tax line ("Diğer vergi ve
  fonlar") when no named tax list exists, so historical behaviour is representable.

---

## 6. Loading

`bcem migrate load` runs inside a transaction per stage and is re-runnable.

Order (foreign keys dictate it):

```
companies → users → buildings → building_contacts → analyzers → power_plants
  → integration_definitions → integration_credentials → smtp_settings
  → tariffs → tariff_taxes → tariff_manual_yekdem → tariff_templates → solar_tariffs
  → alarms → alarm_analyzers → alarm_channels
  → emission_factors → emission_factor_conversions → carbon_selected_activities
  → iso50001_projects → iso50001_clause_dates → iso50001_notes
  → market_prices_hourly → yekdem_monthly
  → meter_readings          (COPY, chunked, indexes and compression added after)
  → plant_production        (COPY)
  → consumption_anomalies
  → legacy_bills, legacy_reports   (comparison tables)
  → stored_files                   (after the artifact copy)
```

Time-series loading: `COPY` into an unlogged staging table, then
`insert … on conflict (analyzer_id, ts, kind) do update`, in chunks of one month per analyzer, with
progress reporting. Continuous aggregates and the compression policy are created **after** the bulk
load, then the aggregates are refreshed over the full history.

### Artifacts

```bash
bcem migrate artifacts \
  --source /opt/bills,/opt/reports,/opt/documents \
  --dest   /var/lib/bcem/files
```

Copies files into the new tenant-scoped layout, registers each in `stored_files` with its checksum,
and reports files referenced in the database but absent on disk, and files on disk referenced by
nothing.

---

## 7. Recomputation

Once raw data is loaded:

```bash
bcem recompute consumption --from 2020-01-01          # refresh all aggregates
bcem recompute bills       --from 2020-01-01 --scope all
bcem recompute reports     --from 2020-01-01
bcem recompute carbon      --from 2020-01-01
bcem recompute forecasts                              # current horizon only
```

Each is idempotent, resumable, progress-reporting and parallelised per company. Failures are
collected and reported rather than aborting the run.

---

## 8. Reconciliation

`bcem migrate reconcile` produces the report the product owner reviews before cutover.

### Checks

| Check | Tolerance |
|-------|-----------|
| Row counts per entity, legacy vs. new | exact |
| Meter readings per analyzer per month | exact, after accounting for reported duplicates and rejections |
| Monthly active consumption per analyzer | ±0.1 % |
| Monthly invoice total per analyzer | ±1 % |
| Monthly invoice total per building | ±1 % |
| Reactive penalty applied — yes/no per period | exact match required |
| Annual consumption per building | ±0.1 % |
| Annual invoice total per building | ±1 % |
| Carbon emission per building per year | ±1 % |
| Artifact file count and total bytes | exact |

### Output

A per-company report: an overall pass/fail, a table of every out-of-tolerance figure with the
legacy value, the new value, the absolute and percentage difference, and — critically — an
**attributed reason**, one of:

- `distribution_on_total_consumption` — corrected per `02-domain-rules.md` §6.3
- `no_tiering_on_multi_time` — §6.2
- `meter_reset_withheld` — §3.2
- `reactive_exempt_below_9kw` — §6.6
- `vat_rate_corrected` — §6.8
- `ptf_gap_flagged` — §7.1
- `capacity_cost_removed` — §6.5
- `unexplained` — **requires investigation before cutover**

Any `unexplained` difference blocks cutover. Every other reason is a decision for the product owner
to accept or contest.

---

## 9. Verification

Before cutover, all of the following must hold:

- [ ] Reconciliation shows zero `unexplained` differences.
- [ ] Every difference reason has been reviewed and accepted in writing.
- [ ] Every migration rejection has been reviewed; nothing was silently dropped.
- [ ] Every analyzer whose multiplier could not be determined automatically has been confirmed
      by hand.
- [ ] Every integration authenticates against the live provider (`verify` passes for each).
- [ ] A fresh data pull ingests correctly for every provider and lands new readings.
- [ ] An invoice generated by the new system for the current month is compared against the real
      utility invoice and agrees within the accepted tolerance.
- [ ] Every user can log in (a forced password reset is issued if hashes are not portable).
- [ ] Role-based access verified for one user of each role.
- [ ] Alarms evaluate and deliver e-mail through the migrated SMTP settings.
- [ ] Reports generate, render to PDF and Excel, and send.
- [ ] Historical invoice and report PDFs download.
- [ ] Carbon totals match the reconciliation.
- [ ] ISO 50001 notes and files are present, and the export archive builds.
- [ ] Scheduled jobs run under the leader lock and record `job_runs` rows.
- [ ] Backup and **restore** have been tested end to end on a scratch host.

---

## 10. Cutover

### Parallel period

The new stack runs on the same host on different internal ports, behind the same reverse proxy,
reachable at a staging hostname. During the parallel period both systems ingest — the new one from
the providers directly, the old one as before — and a nightly reconciliation runs on the previous
day's data. Length: at least one full billing cycle, so a complete invoice period is generated by
both and compared.

### Switch

1. Announce the maintenance window.
2. Stop the legacy scheduler (`crontab -r` for the BCEM entries) and the legacy app.
3. Run a final incremental extract → transform → load for anything written since the last run.
4. Run the final reconciliation; abort on any `unexplained` difference.
5. Repoint the reverse proxy to the new stack.
6. Start the new scheduler.
7. Smoke test: login, dashboard, consumption, generate an invoice, download a PDF, trigger an alarm
   evaluation.
8. Watch job runs and error rates for 24 hours.

### Rollback

Until decommissioning, rollback is: stop the new stack, repoint the proxy, restart the legacy app
and reinstall its cron entries. The legacy database is **not modified at any point**, so rollback is
always available and always clean. The trigger for rollback is agreed in advance and written down.

### Decommission

After the agreed observation period (recommended: one further billing cycle) and an explicit
sign-off: final legacy backup, archive off-host, remove the legacy containers and images, reclaim
the volumes.

---

## 11. Deliverables

The migration is a phase of the build, not an afterthought. It ships as:

| Deliverable | Description |
|-------------|-------------|
| `bcem migrate inventory` | Read-only survey and risk report. |
| `bcem migrate extract` | Mongo → NDJSON with a checksummed manifest. |
| `bcem migrate transform` | Normalisation, validation, rejection reporting. |
| `bcem migrate load` | Ordered, transactional, re-runnable load. |
| `bcem migrate artifacts` | File copy and registration. |
| `bcem recompute …` | Derived-data regeneration. |
| `bcem migrate reconcile` | The reconciliation report, HTML and CSV. |
| `docs/runbook-migration.md` | Step-by-step operator runbook with expected durations, checkpoints, and the rollback procedure. |
| Migration test suite | The whole pipeline exercised against an anonymised production dump in CI. |

**Every one of these commands is idempotent and re-runnable.** A migration that can only be run once
is a migration that cannot be rehearsed, and it will be rehearsed at least three times before it is
run for real.
