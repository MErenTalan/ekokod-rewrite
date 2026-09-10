# F1 — Data Model and Migration Framework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The complete ekokod schema exists as ordered, reversible goose migrations; every table is reachable through a `Scope`-parameterised repository backed by `sqlc`-generated types; reference data loads idempotently through `ekokod seed`; and a fixture factory produces a whole tenant for every later phase.

**Architecture:** Migrations stay plain SQL under `internal/store/postgres/migrations/`, embedded in the binary and applied by the existing `ekokod migrate` command from F0. `sqlc` generates row types and query methods into `internal/store/postgres/sqlcgen/`; hand-written repositories in `internal/store/postgres/` wrap them and are the only exported surface. Entity structs live in `internal/domain/model` (pure data — no I/O, guarded by the existing arch test). Every repository method takes an explicit `store.Scope`; an AST arch test proves no unscoped method exists outside `internal/store/postgres/admin`.

**Tech Stack:** Go 1.27.1 · PostgreSQL 16 + TimescaleDB 2.30 (`timescale/timescaledb:2.30.0-pg16`) · pgx/v5 v5.11.0 · goose v3.28.0 · sqlc (pin in Task 7) · shopspring/decimal · google/uuid · testify · testcontainers-go v0.44.0

**Spec:** `docs/rewrite/04-data-model.md` (authoritative DDL), `docs/rewrite/09-implementation-plan.md` §F1, `docs/rewrite/03-target-architecture.md` §2.5, §3, `docs/rewrite/02-domain-rules.md` §1–§3

---

## Global Constraints

Every task's requirements implicitly include this section.

- **`docs/rewrite/04-data-model.md` is authoritative for every column name, type, constraint and index.** Where this plan's prose disagrees with that file, **the spec file wins**. Where the spec file disagrees with a task's stated acceptance test, the acceptance test wins (F0 standing ruling — the plan's own sample code carried a live defect at least four times in F0).
- **Money and energy are `numeric`, never `float`/`double`.** In Go they are `decimal.Decimal`, never `float64`. A `float64` in a monetary or energy path is a build-breaking defect; `.golangci.yml`'s `no-float-money` depguard rule is extended to the new packages in Task 8.
- **Timestamps are `timestamptz`, stored UTC**, business logic evaluated in `Europe/Istanbul`. Migrations never store a fixed UTC+3 offset.
- **Every tenant-scoped table carries `company_id` directly**, even when reachable through a parent (spec §Conventions).
- **Soft delete is `deleted_at timestamptz`** and exists only on `companies`, `buildings`, `analyzers`, `users` (plus the tables the spec explicitly gives one). Every query filters it.
- **Every migration has a working `down`.** `up → down → up` on an empty database must succeed. The one exception already in the tree is `00001_extensions.sql`, whose down is a deliberate no-op with the rationale recorded inline — do not change it.
- **Migration numbers are pre-assigned by this plan** (00002–00011) so that tasks may be written in parallel without renumbering collisions. Never renumber; never reuse a number.
- **No secret is ever logged and no error text ever embeds a DSN or password.** New store code routes errors through the package's existing `scrubErr`.
- **`internal/domain` imports nothing from the project and performs no I/O.** Enforced by `internal/arch/arch_test.go`.
- **Every task ends green:** `make lint && make test && make build`, plus `go test ./... -tags=integration -race -count=1` for tasks that touch SQL or the store.
- **Commit after every task**, conventional-commit messages, ending with the attribution trailers configured for this session. **The task briefs may carry a stale `Claude-Session:` URL — always override it with this session's.**
- Toolchain: `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"` before every command. Working directory `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite`. Branch `phase/f1-data-model`.
- **Docker must be up** before any integration test. Verify with `docker ps`; do not assume.

---

## Migration number assignment

Pre-assigned so parallel tasks never collide. A task owns its numbers exclusively.

| File | Contents | Task | Depends on |
|---|---|---|---|
| `00001_extensions.sql` | pgcrypto, timescaledb, citext (**exists, do not modify**) | — | — |
| `00002_tenancy.sql` | companies, user_role, users, user_password_history, sessions, audit_log | 1 | 00001 |
| `00003_buildings_plants.sql` | buildings, building_contacts, integration_provider, analyzers, panel_orientation, power_plants, power_plant_monthly_targets, power_plant_devices, power_plant_alarm_recipients | 1 | 00002 |
| `00004_timeseries.sql` | reading_kind, meter_readings (+hypertable, compression), ingestion_cursors, consumption_anomalies, plant_production (+hypertable, compression), market_prices_hourly, yekdem_monthly, forecasts, forecast_gaps | 2 | 00003 |
| `00005_continuous_aggregates.sql` | consumption_hourly/_daily/_monthly/_yearly, plant_production_daily/_monthly, refresh policies | 2 | 00004 |
| `00006_tariffs.sql` | tariff enums, tariffs, tariff_taxes, tariff_manual_yekdem, tariff_templates, solar_tariffs, national_tariff_schedule, icmal_imports, icmal_rows | 3 | 00003 |
| `00007_carbon_iso.sql` | carbon enums, emission_factors, emission_factor_conversions, carbon_selected_activities, carbon_activities, carbon_reports, iso50001_projects, iso50001_clause_dates, iso50001_notes | 4 | 00003 |
| `00008_operations.sql` | stored_files, integration_definitions, integration_credentials, smtp_settings, calendar_events, company_weekend_days, company_vacations, job_runs, operational_messages | 5 | 00003 |
| `00009_bills.sql` | bill_scope, bill_status, bills, bill_lines, bill_members, bill_hourly_detail | 6 | 00006 |
| `00010_reports.sql` | report_type, report_status, plant_selection, reports | 6 | 00003 |
| `00011_alarms.sql` | alarm_type, period_unit, notify_channel, alarms, alarm_analyzers, alarm_channels, alarm_events, alarm_fired_bills, isolar_forwarded_alarms | 6 | 00009 |

**Execution waves** (see `superpowers:dispatching-parallel-agents`):

- **Wave A:** Task 1 (alone — every later migration builds on its foreign keys).
- **Wave B:** Tasks 2, 3, 4, 5 in parallel (all depend only on 00003).
- **Wave C:** Task 6 (needs 00006 for `bills.tariff_id`).
- **Wave D:** Task 7 (sqlc needs the complete schema).
- **Wave E:** Task 8 (Scope, interfaces, fixtures — every repository task consumes it).
- **Wave F:** Tasks 9, 10, 11 in parallel.
- **Wave G:** Tasks 12, 13 in parallel.

---

## File Structure

```
internal/store/postgres/migrations/
  00002_tenancy.sql 00003_buildings_plants.sql 00004_timeseries.sql
  00005_continuous_aggregates.sql 00006_tariffs.sql 00007_carbon_iso.sql
  00008_operations.sql 00009_bills.sql 00010_reports.sql 00011_alarms.sql
internal/store/postgres/
  migration_roundtrip_integration_test.go   # up -> down -> up, per-migration
  schema/schema.sql                          # sqlc catalogue input (Task 7)
  sqlcgen/                                   # GENERATED — never hand-edited
  queries/*.sql                              # sqlc query definitions, one file per aggregate
  tenancy.go buildings.go analyzers.go plants.go        # Task 9 repositories
  readings.go cursors.go anomalies.go production.go prices.go forecasts.go  # Task 10
  tariffs.go bills.go reports.go alarms.go carbon.go iso50001.go files.go integrations.go ops.go  # Task 11
  admin/admin.go                             # the ONLY unscoped query surface
internal/store/
  scope.go                                   # store.Scope + repository interfaces
  errors.go                                  # shared not-found / conflict sentinels
internal/domain/model/                       # pure entity structs, no I/O
internal/testfixtures/                       # container helpers + tenant factory
internal/seed/                               # seed loader + embedded datasets
sqlc.yaml
```

One file = one aggregate. `sqlcgen/` is generated output and is never hand-edited; a CI check proves it is current.

---

## Task 1: Tenancy and buildings migrations, plus the round-trip harness

**Files:**
- Create: `internal/store/postgres/migrations/00002_tenancy.sql`, `internal/store/postgres/migrations/00003_buildings_plants.sql`
- Create: `internal/store/postgres/migration_roundtrip_integration_test.go`

**Interfaces:**
- Consumes: the F0 `migrate` command and embedded `migrations` FS in `internal/store/postgres/migrate.go`.
- Produces: tables `companies`, `users`, `user_password_history`, `sessions`, `audit_log`, `buildings`, `building_contacts`, `analyzers`, `power_plants`, `power_plant_monthly_targets`, `power_plant_devices`, `power_plant_alarm_recipients`; enums `user_role`, `integration_provider`, `panel_orientation`. Every later task's foreign keys point here. **Column names and types are exactly as in `04-data-model.md` §2 and §3 — transcribe, do not invent.**

- [ ] **Step 1: Write the failing round-trip test**

`internal/store/postgres/migration_roundtrip_integration_test.go`, build tag `//go:build integration`:

```go
// TestMigrationsRoundTrip is the F1 acceptance criterion "migrate up on an
// empty database creates the full schema; down reverses it cleanly; up again
// succeeds". It runs against a real TimescaleDB container because the schema
// depends on Timescale functions no in-process stub provides.
func TestMigrationsRoundTrip(t *testing.T) {
	dsn := startPostgres(t)
	require.NoError(t, postgres.MigrateUp(context.Background(), dsn, discardLogger()))
	require.NoError(t, postgres.MigrateDownAll(context.Background(), dsn, discardLogger()))
	require.NoError(t, postgres.MigrateUp(context.Background(), dsn, discardLogger()))
}

// TestMigrationsLeaveNoTablesBehind proves `down` is genuinely reversible
// rather than merely non-erroring: after down, no table this phase created
// may remain. Extensions are exempt — 00001's down is a deliberate no-op.
func TestMigrationsLeaveNoTablesBehind(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	require.NoError(t, postgres.MigrateDownAll(ctx, dsn, discardLogger()))

	pool := newPool(t, dsn)
	var remaining []string
	rows, err := pool.Query(ctx, `select tablename from pg_tables
		where schemaname = 'public' and tablename <> 'goose_db_version'`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		remaining = append(remaining, name)
	}
	require.NoError(t, rows.Err())
	require.Empty(t, remaining, "down migrations must drop every table they created")
}
```

If `MigrateDownAll` does not exist on the F0 surface, add it in this task next to `MigrateUp` in `internal/store/postgres/migrate.go`, using goose's `DownTo(ctx, db, dir, 0)`, and route its errors through the existing `scrubErr`.

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/store/postgres/ -tags=integration -race -run TestMigrations -v
```
Expected: FAIL — `MigrateDownAll` undefined, or the "no tables behind" assertion lists nothing because no tables were created yet.

- [ ] **Step 3: Write `00002_tenancy.sql`**

Transcribe `04-data-model.md` §2 verbatim into a goose migration. Structure:

```sql
-- +goose Up
create table companies ( ... );
create unique index on companies (lower(name)) where deleted_at is null;
create type user_role as enum (...);
create table users ( ... );
...

-- +goose Down
drop table if exists audit_log;
drop table if exists sessions;
drop table if exists user_password_history;
drop table if exists users;
drop type if exists user_role;
drop table if exists companies;
```

Down drops in reverse dependency order. Do **not** use `cascade` — a `cascade` would hide a missing drop and let the "no tables behind" test pass while silently destroying an object another migration owns.

- [ ] **Step 4: Write `00003_buildings_plants.sql`**

Transcribe `04-data-model.md` §3 verbatim, same up/down shape. `analyzers.provider` uses the `integration_provider` enum created in this file. Note `buildings.bill_cutoff_day` carries `check (bill_cutoff_day between 1 and 31)` and `analyzers.meter_multiplier` defaults to 1 — both are load-bearing for later phases.

- [ ] **Step 5: Run the round-trip tests**

```bash
go test ./internal/store/postgres/ -tags=integration -race -count=1 -run TestMigrations -v
```
Expected: PASS, both tests.

- [ ] **Step 6: Prove the guard can fail**

Temporarily delete one `drop table` line from `00003`'s down section, re-run `TestMigrationsLeaveNoTablesBehind`, and confirm it FAILS naming that table. Restore the line and confirm it passes again. (F0 standing ruling: prove a guard can fail.) Record the observed failure output in the task report.

- [ ] **Step 7: Full gate and commit**

```bash
make lint && make test && make build
go test ./... -tags=integration -race -count=1
git add internal/store/postgres
git commit -m "feat(f1): tenancy and building migrations with a reversibility harness"
```

---

## Task 2: Time-series hypertables and continuous aggregates

**Files:**
- Create: `internal/store/postgres/migrations/00004_timeseries.sql`, `internal/store/postgres/migrations/00005_continuous_aggregates.sql`
- Modify: `internal/store/postgres/migration_roundtrip_integration_test.go` (add the hypertable and aggregate assertions below)

**Interfaces:**
- Consumes: `analyzers`, `power_plants`, `power_plant_devices`, `users` from Task 1.
- Produces: `meter_readings`, `ingestion_cursors`, `consumption_anomalies`, `plant_production`, `market_prices_hourly`, `yekdem_monthly`, `forecasts`, `forecast_gaps`, enum `reading_kind`; continuous aggregates `consumption_hourly`, `consumption_daily`, `consumption_monthly`, `consumption_yearly`, `plant_production_daily`, `plant_production_monthly`. Tasks 10 and 13 query these by exactly these names.

**This is the highest-risk task in the phase.** Everything energy-related rests on `meter_readings`.

- [ ] **Step 1: Write the failing hypertable assertions**

Append to the round-trip test file:

```go
// TestHypertablesAreConfigured pins the partitioning decisions from
// 04-data-model.md §4. These are not cosmetic: chunk_time_interval and the
// space partition determine whether a one-month single-analyzer query reads
// four chunks or the whole table.
func TestHypertablesAreConfigured(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	for _, want := range []struct {
		table       string
		numDims     int
		chunkInterv time.Duration
	}{
		{"meter_readings", 2, 7 * 24 * time.Hour},
		{"plant_production", 1, 30 * 24 * time.Hour},
		{"market_prices_hourly", 1, 90 * 24 * time.Hour},
		{"forecasts", 1, 30 * 24 * time.Hour},
	} {
		var dims int
		require.NoError(t, pool.QueryRow(ctx,
			`select num_dimensions from timescaledb_information.hypertables
			 where hypertable_name = $1`, want.table).Scan(&dims),
			"%s must be a hypertable", want.table)
		require.Equal(t, want.numDims, dims, "%s dimension count", want.table)

		var interval time.Duration
		require.NoError(t, pool.QueryRow(ctx,
			`select time_interval from timescaledb_information.dimensions
			 where hypertable_name = $1 and dimension_type = 'Time'`, want.table).Scan(&interval))
		require.Equal(t, want.chunkInterv, interval, "%s chunk interval", want.table)
	}
}

// TestCompressionPoliciesExist covers the F1 acceptance criterion that a
// chunk older than the threshold compresses. Task 13 proves compression
// actually runs and that queries still return correct rows from a compressed
// chunk; this test only proves the policy was installed by the migration.
func TestCompressionPoliciesExist(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	for _, table := range []string{"meter_readings", "plant_production"} {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`select count(*) from timescaledb_information.jobs
			 where proc_name = 'policy_compression' and hypertable_name = $1`, table).Scan(&n))
		require.Equal(t, 1, n, "%s must have exactly one compression policy", table)
	}
}

// TestContinuousAggregatesExist pins the aggregate names Task 10's analytics
// repositories and Task 13's correctness test both depend on.
func TestContinuousAggregatesExist(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	for _, name := range []string{
		"consumption_hourly", "consumption_daily", "consumption_monthly",
		"consumption_yearly", "plant_production_daily", "plant_production_monthly",
	} {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`select count(*) from timescaledb_information.continuous_aggregates
			 where view_name = $1`, name).Scan(&n))
		require.Equal(t, 1, n, "continuous aggregate %s must exist", name)

		var policies int
		require.NoError(t, pool.QueryRow(ctx,
			`select count(*) from timescaledb_information.jobs
			 where proc_name = 'policy_refresh_continuous_aggregate'
			   and hypertable_name = $1`, name).Scan(&policies))
		require.Equal(t, 1, policies, "%s must have a refresh policy", name)
	}
}
```

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/store/postgres/ -tags=integration -race -run 'TestHypertables|TestCompression|TestContinuousAggregates' -v
```
Expected: FAIL — `meter_readings` is not a hypertable (no rows in `timescaledb_information.hypertables`).

- [ ] **Step 3: Write `00004_timeseries.sql`**

Transcribe `04-data-model.md` §4.1, §4.2, §4.4, §4.5, §4.6 and §12. Points that are easy to get wrong and are load-bearing:

- `meter_readings` primary key is `(analyzer_id, ts, kind)` and the hypertable call passes **both** `partitioning_column => 'analyzer_id'` with `number_partitions => 16` **and** `chunk_time_interval => interval '7 days'`.
- `create_hypertable` must run **after** the table and **before** the compression settings.
- `alter table meter_readings set (timescaledb.compress, compress_segmentby = 'analyzer_id, kind', compress_orderby = 'ts desc')` then `select add_compression_policy('meter_readings', interval '90 days')`.
- `plant_production` primary key is `(plant_id, ts, device_id)`; `device_id` is nullable in the spec's column list but is part of the primary key — transcribe as written and let the round-trip test surface any conflict rather than "fixing" it silently. If Postgres rejects a nullable primary-key column, **stop and report it as a spec conflict in the task report** instead of choosing a resolution unilaterally.
- Goose needs `-- +goose StatementBegin` / `-- +goose StatementEnd` around any statement containing a semicolon inside a function body. Plain `select create_hypertable(...)` calls do not need it; add it only where goose's parser actually fails.

Down section drops the tables in reverse dependency order. Dropping a hypertable drops its chunks; the compression policies go with it.

- [ ] **Step 4: Write `00005_continuous_aggregates.sql`**

Transcribe `04-data-model.md` §4.3. The full `consumption_hourly` definition is given verbatim in the spec — copy every column, including all six `*_index` columns and `reading_count`. Then:

- `consumption_daily`, `consumption_monthly`, `consumption_yearly` follow the same column shape at `1 day` / `1 month` / `1 year` buckets. Roll each up from the level below **where TimescaleDB permits it**; where a hierarchical aggregate is rejected (Timescale forbids `first()`/`last()` over a continuous aggregate in some versions), define it directly over `meter_readings` and **record which form you used and why in the task report** — Task 13's correctness test must know which it is.
- `plant_production_daily` and `plant_production_monthly` aggregate `sum(production_kwh)`, `max(active_power_kw)`, `avg(efficiency_pct)` grouped by `plant_id` and bucket.
- Each aggregate gets `add_continuous_aggregate_policy` with the offsets from the spec (`consumption_hourly`: start 30 days, end 1 hour, schedule 30 minutes; scale the others proportionally and state the chosen values in the report).
- Continuous aggregates cannot be created inside a transaction block in some Timescale versions. If goose's default transactional apply fails, add `-- +goose NO TRANSACTION` at the top of the file. Try transactional first; only add the directive if it actually fails, and say so in the report.

Down section: `drop materialized view if exists <name>;` in reverse creation order. The refresh policies drop with the view.

- [ ] **Step 5: Run the whole store suite**

```bash
go test ./internal/store/postgres/ -tags=integration -race -count=1 -v
```
Expected: PASS — including `TestMigrationsRoundTrip` and `TestMigrationsLeaveNoTablesBehind` from Task 1, which now cover ten migrations.

- [ ] **Step 6: Full gate and commit**

```bash
make lint && make test && make build
go test ./... -tags=integration -race -count=1
git add internal/store/postgres
git commit -m "feat(f1): meter reading and production hypertables with continuous aggregates"
```

---

## Task 3: Tariff and icmal migrations

**Files:**
- Create: `internal/store/postgres/migrations/00006_tariffs.sql`
- Modify: `internal/store/postgres/migration_roundtrip_integration_test.go` (add the constraint tests below)

**Interfaces:**
- Consumes: `companies`, `buildings`, `users`, `power_plants` from Task 1.
- Produces: enums `price_type`, `tariff_term`, `energy_type`, `voltage_level`, `supply_company`, `generation_usage`, `currency_code`, `distribution_user_group`; tables `tariffs`, `tariff_taxes`, `tariff_manual_yekdem`, `tariff_templates`, `solar_tariffs`, `national_tariff_schedule`, `icmal_imports`, `icmal_rows`. Task 6's `bills.tariff_id` references `tariffs(id)`.

- [ ] **Step 1: Write the failing constraint tests**

The four `tariffs` check constraints are business rules, not decoration — they are the database's half of the F4 billing contract. Prove each one rejects what it must:

```go
// TestTariffConstraintsRejectIncompleteRows proves the four check constraints
// in 04-data-model.md §5 actually fire. Each is a business rule: a
// single_time tariff with no price, or a PTF tariff with no KBK, would
// produce a silently wrong invoice in F4.
func TestTariffConstraintsRejectIncompleteRows(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	var companyID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into companies (name) values ('acme') returning id`).Scan(&companyID))

	insert := func(t *testing.T, overrides string, args ...any) error {
		t.Helper()
		_, err := pool.Exec(ctx, `insert into tariffs (
			company_id, effective_from, voltage_level, user_group, price_type,
			term, supply_company, distribution_cost, reactive_power_price, vat_rate`+
			overrides, append([]any{companyID}, args...)...)
		return err
	}

	t.Run("single_time without a price", func(t *testing.T) {
		err := insert(t, `) values ($1,'2026-01-01','lv','residential','single_time',
			'monomial','incumbent',0,0,20)`)
		require.ErrorContains(t, err, "single_time_needs_price")
	})

	t.Run("multi_time without all three band prices", func(t *testing.T) {
		err := insert(t, `, t1_price, t2_price) values ($1,'2026-01-01','lv','residential',
			'multi_time','monomial','incumbent',0,0,20,1,1)`)
		require.ErrorContains(t, err, "multi_time_needs_prices")
	})

	t.Run("subtract_from_total without a generation price", func(t *testing.T) {
		err := insert(t, `, single_time_price, generation_usage) values ($1,'2026-01-01','lv',
			'residential','single_time','monomial','incumbent',0,0,20,1,'subtract_from_total')`)
		require.ErrorContains(t, err, "subtract_from_total_needs_price")
	})

	t.Run("ptf without kbk_energy", func(t *testing.T) {
		err := insert(t, `, single_time_price, use_ptf_yekdem) values ($1,'2026-01-01','lv',
			'residential','single_time','monomial','incumbent',0,0,20,1,true)`)
		require.ErrorContains(t, err, "ptf_needs_energy_kbk")
	})
}

// TestVatRateHasNoDefault guards the spec's explicit "REQUIRED, no default"
// on tariffs.vat_rate: a defaulted VAT rate would silently price every
// invoice at whatever the default happened to be.
func TestVatRateHasNoDefault(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	var def *string
	require.NoError(t, pool.QueryRow(ctx,
		`select column_default from information_schema.columns
		 where table_name = 'tariffs' and column_name = 'vat_rate'`).Scan(&def))
	require.Nil(t, def, "tariffs.vat_rate must have no default")
}
```

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/store/postgres/ -tags=integration -race -run 'TestTariff|TestVatRate' -v
```
Expected: FAIL — relation `tariffs` does not exist.

- [ ] **Step 3: Write `00006_tariffs.sql`**

Transcribe `04-data-model.md` §5 including the "İcmal imports" subsection. All four named check constraints keep their spec names exactly (`single_time_needs_price`, `multi_time_needs_prices`, `subtract_from_total_needs_price`, `ptf_needs_energy_kbk`) — the tests assert on those names. `vat_rate` is `not null` with **no** default.

Down section drops in reverse dependency order, dropping the eight enum types after the tables that use them.

- [ ] **Step 4: Run and watch them pass**

```bash
go test ./internal/store/postgres/ -tags=integration -race -count=1 -v
```
Expected: PASS.

- [ ] **Step 5: Full gate and commit**

```bash
make lint && make test && make build
git add internal/store/postgres
git commit -m "feat(f1): tariff, template and icmal import migrations"
```

---

## Task 4: Carbon and ISO 50001 migrations

**Files:**
- Create: `internal/store/postgres/migrations/00007_carbon_iso.sql`
- Modify: `internal/store/postgres/migration_roundtrip_integration_test.go` (add the partial-index test below)

**Interfaces:**
- Consumes: `companies`, `buildings`, `users` from Task 1.
- Produces: enums `carbon_scope`, `carbon_status`; tables `emission_factors`, `emission_factor_conversions`, `carbon_selected_activities`, `carbon_activities`, `carbon_reports`, `iso50001_projects`, `iso50001_clause_dates`, `iso50001_notes`. Task 12 seeds `emission_factors` and the ISO clause texts.

- [ ] **Step 1: Write the failing uniqueness tests**

Two indexes in this section encode rules that are easy to transcribe wrongly:

```go
// TestEmissionFactorKeyUniquenessSpansTheMasterCatalogue proves the
// coalesce-based unique index from 04-data-model.md §9: a company may
// override a master-catalogue key, but may not define the same key twice,
// and two companies may each hold their own copy of one key.
func TestEmissionFactorKeyUniqueness(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	insert := func(company *uuid.UUID, key string) error {
		_, err := pool.Exec(ctx, `insert into emission_factors
			(company_id, key, label, main_category, base_factor, base_unit)
			values ($1,$2,'l','c',1,'kg')`, company, key)
		return err
	}
	var a, b uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `insert into companies (name) values ('a') returning id`).Scan(&a))
	require.NoError(t, pool.QueryRow(ctx, `insert into companies (name) values ('b') returning id`).Scan(&b))

	require.NoError(t, insert(nil, "diesel"), "the master catalogue may hold a key")
	require.Error(t, insert(nil, "diesel"), "the master catalogue may not hold it twice")
	require.NoError(t, insert(&a, "diesel"), "a company may override a master key")
	require.Error(t, insert(&a, "diesel"), "a company may not hold the same key twice")
	require.NoError(t, insert(&b, "diesel"), "a second company keeps its own copy")
}

// TestAutomatedCarbonActivitiesAreUniquePerDay guards the partial unique
// index "one automated record per building, activity type and day". Manual
// records are deliberately exempt.
func TestAutomatedCarbonActivitiesAreUniquePerDay(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)
	insert := func(automated bool) error {
		_, err := pool.Exec(ctx, `insert into carbon_activities
			(company_id, building_id, main_category, sub_category, activity_type,
			 period_start, period_end, quantity, unit, emission_kgco2e, scope,
			 iso_category, is_automated)
			values ($1,$2,'energy','grid','electricity','2026-01-01','2026-01-01',
			        1,'kWh',1,'scope_2','cat1',$3)`, companyID, buildingID, automated)
		return err
	}
	require.NoError(t, insert(true))
	require.Error(t, insert(true), "a second automated row for the same day must be rejected")
	require.NoError(t, insert(false), "manual rows are exempt from the partial index")
	require.NoError(t, insert(false), "and may repeat")
}
```

`seedCompanyAndBuilding` is a small helper in the same test file that inserts one company and one building and returns both ids. Write it in this task; Task 8's fixture factory supersedes it later.

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/store/postgres/ -tags=integration -race -run 'TestEmissionFactor|TestAutomatedCarbon' -v
```
Expected: FAIL — relation `emission_factors` does not exist.

- [ ] **Step 3: Write `00007_carbon_iso.sql`**

Transcribe `04-data-model.md` §9 and §10. Watch for:
- `emission_factors.sub_categories` and `category_path` are `text[] not null default '{}'`.
- The unique index is on `(coalesce(company_id,'00000000-0000-0000-0000-000000000000'::uuid), key)`.
- `carbon_activities`'s automated-uniqueness index is **partial** (`where is_automated`).
- `iso50001_clause_dates` carries the named `valid_range` check.

- [ ] **Step 4: Run and watch them pass, then gate and commit**

```bash
go test ./internal/store/postgres/ -tags=integration -race -count=1 -v
make lint && make test && make build
git add internal/store/postgres
git commit -m "feat(f1): carbon module and iso 50001 migrations"
```

---

## Task 5: Files, integrations, calendar and operations migrations

**Files:**
- Create: `internal/store/postgres/migrations/00008_operations.sql`
- Modify: `internal/store/postgres/migration_roundtrip_integration_test.go`

**Interfaces:**
- Consumes: `companies`, `users` from Task 1.
- Produces: `stored_files`, `integration_definitions`, `integration_credentials`, `smtp_settings`, `calendar_events`, `company_weekend_days`, `company_vacations`, `job_runs`, `operational_messages`. Task 12 seeds `integration_definitions`.

- [ ] **Step 1: Write the failing test**

`integration_credentials` holds AES-256-GCM ciphertext and is the table most likely to leak a secret if typed wrongly. Prove the encrypted columns are `bytea` and that no plaintext secret column exists:

```go
// TestIntegrationCredentialSecretsAreBytea guards the spec's requirement
// that provider secrets are stored as AES-256-GCM ciphertext. A text column
// here would invite storing a plaintext password, and the API must never
// return these at all.
func TestIntegrationCredentialSecretsAreBytea(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	for table, columns := range map[string][]string{
		"integration_credentials": {"secret_enc", "extra_enc"},
		"smtp_settings":           {"password_enc"},
	} {
		for _, column := range columns {
			var dataType string
			require.NoError(t, pool.QueryRow(ctx,
				`select data_type from information_schema.columns
				 where table_name = $1 and column_name = $2`, table, column).Scan(&dataType),
				"%s.%s must exist", table, column)
			require.Equal(t, "bytea", dataType, "%s.%s must hold ciphertext", table, column)
		}
	}

	var plaintext int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from information_schema.columns
		 where table_name in ('integration_credentials','smtp_settings')
		   and column_name in ('password','secret','api_key','token')`).Scan(&plaintext))
	require.Zero(t, plaintext, "no plaintext secret column may exist")
}
```

- [ ] **Step 2: Run and watch it fail; Step 3: write `00008_operations.sql`**

Transcribe `04-data-model.md` §11. `operational_messages.id` and `audit_log.id` are `bigserial`, not uuid — transcribe as written. `job_runs.scope` and `.detail` are `jsonb`.

- [ ] **Step 4: Run, gate and commit**

```bash
go test ./internal/store/postgres/ -tags=integration -race -count=1 -v
make lint && make test && make build
git add internal/store/postgres
git commit -m "feat(f1): file, integration, calendar and operations migrations"
```

---

## Task 6: Bill, report and alarm migrations

**Files:**
- Create: `internal/store/postgres/migrations/00009_bills.sql`, `internal/store/postgres/migrations/00010_reports.sql`, `internal/store/postgres/migrations/00011_alarms.sql`
- Modify: `internal/store/postgres/migration_roundtrip_integration_test.go`

**Interfaces:**
- Consumes: `companies`, `buildings`, `analyzers` (Task 1), `tariffs` (Task 3).
- Produces: enums `bill_scope`, `bill_status`, `report_type`, `report_status`, `plant_selection`, `alarm_type`, `period_unit`, `notify_channel`; tables `bills`, `bill_lines`, `bill_members`, `bill_hourly_detail`, `reports`, `alarms`, `alarm_analyzers`, `alarm_channels`, `alarm_events`, `alarm_fired_bills`, `isolar_forwarded_alarms`.

Written as one task because `alarm_fired_bills` references `bills`, so the three files must land in a fixed order.

- [ ] **Step 1: Write the failing supersede test**

The `bills` partial unique index is the rule that makes recomputation safe. It is worth its own test:

```go
// TestOneLiveBillPerScopeAndPeriod covers 04-data-model.md §14: the unique
// index enforces one live bill per scope and period, and recomputation
// supersedes rather than deletes, preserving what was issued.
func TestOneLiveBillPerScopeAndPeriod(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, postgres.MigrateUp(ctx, dsn, discardLogger()))
	pool := newPool(t, dsn)

	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)
	insert := func(status string) (uuid.UUID, error) {
		var id uuid.UUID
		err := pool.QueryRow(ctx, `insert into bills
			(company_id, building_id, scope, period_key, period_start, period_end,
			 days_in_period, index_start, index_end, status)
			values ($1,$2,'building','2026-01','2026-01-01','2026-02-01',31,'{}','{}',$3)
			returning id`, companyID, buildingID, status).Scan(&id)
		return id, err
	}

	first, err := insert("issued")
	require.NoError(t, err)
	_, err = insert("draft")
	require.Error(t, err, "a second live bill for the same scope and period must be rejected")

	_, err = pool.Exec(ctx, `update bills set status = 'superseded' where id = $1`, first)
	require.NoError(t, err)
	_, err = insert("draft")
	require.NoError(t, err, "superseding the previous bill must free the period")

	var kept int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from bills`).Scan(&kept))
	require.Equal(t, 2, kept, "the superseded bill must be preserved, not deleted")
}
```

- [ ] **Step 2: Run and watch it fail; Step 3: write the three migrations**

Transcribe `04-data-model.md` §6 (bills), §8 (reports) and §7 (alarms) in that file order. The bills unique index is:

```sql
create unique index on bills (scope, coalesce(analyzer_id, building_id, company_id), period_key)
    where status <> 'superseded';
```

`00011_alarms.sql`'s down must drop `alarm_fired_bills` before `00009`'s down drops `bills`; because goose runs downs in reverse numeric order this happens naturally — do not add cross-file drops.

- [ ] **Step 4: Run the whole store suite, gate and commit**

```bash
go test ./internal/store/postgres/ -tags=integration -race -count=1 -v
make lint && make test && make build
go test ./... -tags=integration -race -count=1
git add internal/store/postgres
git commit -m "feat(f1): bill, report and alarm migrations"
```

At the end of this task the schema is complete: `TestMigrationsRoundTrip` and `TestMigrationsLeaveNoTablesBehind` now cover all eleven migrations.

---

## Task 7: sqlc configuration and generated types

**Files:**
- Create: `sqlc.yaml`, `internal/store/postgres/queries/.gitkeep`
- Create: `internal/store/postgres/schema/schema.sql` (only if the spike in Step 1 requires it)
- Create: `internal/store/postgres/sqlcgen/` (generated)
- Modify: `Makefile` (`generate` target, `tools` target), `.github/workflows/ci.yml` (drift check), `.gitignore`

**Interfaces:**
- Consumes: every migration from Tasks 1–6.
- Produces: package `sqlcgen` with a row struct per table and a `Queries` type; `postgres.New(pool)` returning a struct embedding `*sqlcgen.Queries`. Tasks 9–11 build every repository on these.

- [ ] **Step 1: Spike — can sqlc read the migrations directly?**

sqlc builds its catalogue by parsing DDL. Our migrations contain TimescaleDB function calls (`select create_hypertable(...)`, `add_compression_policy`, `add_continuous_aggregate_policy`) and `create materialized view ... with (timescaledb.continuous)`, none of which sqlc's Postgres parser knows.

Run the spike before writing anything:

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
cat > sqlc.yaml <<'YAML'
version: "2"
sql:
  - engine: postgresql
    schema: internal/store/postgres/migrations
    queries: internal/store/postgres/queries
    gen:
      go:
        package: sqlcgen
        out: internal/store/postgres/sqlcgen
        sql_package: pgx/v5
        emit_pointers_for_null_types: true
YAML
sqlc generate
```

**Decision procedure — record which branch you took and the exact error in the task report:**

- **If it succeeds:** keep `schema:` pointed at the migrations directory. Delete the `schema/schema.sql` line from the File Structure above; it is not needed. Go to Step 2.
- **If it fails on Timescale functions or the continuous-aggregate views:** add `internal/store/postgres/schema/schema.sql`, produced by migrating a real container and dumping the plain-DDL subset, and point `schema:` at that file instead. Then add the drift check in Step 5 — without it the dump silently rots away from the migrations, which is a worse failure than the one being worked around.

Do **not** resolve this by hand-maintaining a parallel schema file with no drift check. Do **not** delete Timescale statements from the migrations to please sqlc.

- [ ] **Step 2: Write one query file and prove generation works end to end**

`internal/store/postgres/queries/buildings.sql`:

```sql
-- name: GetBuilding :one
select * from buildings
where id = $1 and company_id = $2 and deleted_at is null;

-- name: ListBuildingsForScope :many
select * from buildings
where company_id = $1
  and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null
order by name;
```

The `all_buildings` / `building_ids` pair is the SQL half of `store.Scope` from Task 8. Every scoped list query in Tasks 9–11 uses exactly this shape, so get it right here.

- [ ] **Step 3: Run generation and compile**

```bash
sqlc generate
go build ./...
```
Expected: `internal/store/postgres/sqlcgen/` contains `models.go`, `db.go` and `buildings.sql.go`; the build is clean.

Verify by inspection that money and energy columns generated as `pgtype.Numeric` and **not** `float64`. If any generated as a float, add a `overrides:` entry in `sqlc.yaml` mapping that column to `pgtype.Numeric` and regenerate. A `float64` here would defeat the phase's central numeric constraint.

- [ ] **Step 4: Wire the Makefile**

```make
SQLC_VERSION := v1.30.0

tools:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	# ... existing tool installs unchanged

generate: ## Regenerate sqlc types
	sqlc generate
```

Replace the F0 placeholder `@echo "no generators configured yet"`. Pin the version — F0's Task 13 ruling 22 established that `@latest` in one place and a pin in another is a defect.

- [ ] **Step 5: Add the CI drift check**

`sqlcgen/` is committed (so a fresh clone builds with no sqlc installed) and therefore can go stale. Add a CI step that regenerates and fails on a diff:

```yaml
- name: sqlc is current
  run: |
    sqlc generate
    git diff --exit-code -- internal/store/postgres/sqlcgen
```

If Step 1 took the `schema.sql` branch, the same job must also regenerate `schema.sql` from a migrated container and `git diff --exit-code` it. Pin sqlc to `$SQLC_VERSION` in CI, matching the Makefile.

- [ ] **Step 6: Prove the drift check can fail**

Hand-edit one line in a generated file, run the CI command locally, confirm it fails; revert and confirm it passes. Record the output in the report.

- [ ] **Step 7: Gate and commit**

```bash
make lint && make test && make build
git add sqlc.yaml Makefile .github internal/store/postgres
git commit -m "feat(f1): sqlc configuration, generated types and a drift check"
```

---

## Task 8: `store.Scope`, repository interfaces, container helpers and the tenant fixture factory

**Files:**
- Create: `internal/store/scope.go`, `internal/store/errors.go`, `internal/store/repository.go`
- Create: `internal/domain/model/*.go`
- Create: `internal/testfixtures/containers.go`, `internal/testfixtures/tenant.go`
- Modify: `internal/arch/arch_test.go` (the scope guard), `.golangci.yml` (extend `no-float-money`)
- Modify: `internal/store/redis/scrub.go`, `internal/store/postgres/scrub.go`, `internal/job/scrub.go` (deferred minors, see Step 6)

**Interfaces:**
- Consumes: `sqlcgen` from Task 7.
- Produces — Tasks 9–13 all build on these exact names:

```go
package store

// Scope is the tenant boundary every repository read and write is
// parameterised by. It is a value, not a context key, so that a missing
// scope is a compile error rather than a runtime nil.
type Scope struct {
	CompanyID uuid.UUID

	// BuildingIDs is the set of buildings the principal may see. An EMPTY
	// slice means NO buildings — never "all". Widening must be explicit.
	BuildingIDs []uuid.UUID

	// AllBuildings grants the whole company. It is a separate flag precisely
	// so that "all buildings" can never be expressed accidentally by an
	// empty or nil BuildingIDs slice, which is the fail-open shape this
	// type exists to prevent.
	AllBuildings bool
}

// Valid reports whether the scope can be used. A zero CompanyID is never
// valid; every repository method rejects an invalid scope before touching
// the database.
func (s Scope) Valid() bool { return s.CompanyID != uuid.Nil }

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalidScope = errors.New("invalid scope")
)
```

- [ ] **Step 1: Write the failing scope tests**

```go
func TestEmptyBuildingSetGrantsNothing(t *testing.T) {
	s := store.Scope{CompanyID: uuid.New()}
	require.True(t, s.Valid())
	require.False(t, s.AllBuildings, "the zero value must not grant every building")
	require.Empty(t, s.BuildingIDs)
}

func TestZeroScopeIsInvalid(t *testing.T) {
	require.False(t, store.Scope{}.Valid(), "a scope with no company must never be usable")
}
```

- [ ] **Step 2: Write `scope.go`, `errors.go` and the model structs**

`internal/domain/model` holds one plain struct per aggregate — fields mirroring the columns, `decimal.Decimal` for every money and energy column, `uuid.UUID` for ids, `time.Time` for timestamps, pointer or `*T` for nullable columns. No methods with I/O; no imports from elsewhere in the project. Add `github.com/shopspring/decimal` to `go.mod`.

`internal/store/repository.go` declares one interface per aggregate. Every method takes `ctx context.Context` first and `s Scope` second:

```go
type BuildingRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.Building, error)
	List(ctx context.Context, s Scope, f BuildingFilter) ([]model.Building, error)
	Create(ctx context.Context, s Scope, b model.Building) (model.Building, error)
	Update(ctx context.Context, s Scope, b model.Building) (model.Building, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error
}
```

Declare the interfaces for every aggregate Tasks 9–11 implement. Tasks 9–11 must not invent new method names — if one is missing, add it here first.

- [ ] **Step 3: Write the scope arch guard**

Extend `internal/arch/arch_test.go` with an AST test — the existing file already loads packages with `golang.org/x/tools/go/packages`, so follow its established pattern rather than a textual scan:

```go
// TestEveryStoreMethodIsScoped enforces 03-target-architecture.md §2.5:
// "there is no unscoped query method outside the admin package". Without
// this, one method that forgets its Scope parameter is a silent
// cross-tenant read, which is the single worst defect class this schema
// can have.
func TestEveryStoreMethodIsScoped(t *testing.T) {
	// Load ./internal/store/postgres/... with syntax, skip the admin
	// subpackage, and for every EXPORTED method on an exported type whose
	// name ends in "Repository", assert that one parameter's type resolves
	// to store.Scope.
}
```

- [ ] **Step 4: Prove the scope guard can fail**

Add a temporary exported repository method with no `Scope` parameter, run the guard, confirm it FAILS naming that method; then confirm the same method under `internal/store/postgres/admin` PASSES. Delete both and re-run. Record both observations in the report — this is the guard the whole tenancy model rests on.

- [ ] **Step 5: Write the container helpers and the tenant factory**

`internal/testfixtures/containers.go` centralises what is currently duplicated across four test files:

```go
// StartPostgres starts a migrated TimescaleDB container and returns its DSN.
func StartPostgres(t *testing.T) string

// StartRedis starts a redis container and returns its URI.
//
// The redis wait strategy is REPLACED, not appended: testcontainers'
// redis module hardcodes a 10-second budget on the port check
// (modules/redis@v0.44.0/redis.go:72) where the postgres module leaves the
// library default of 60s. Under `go test ./... -race -count=3` the WSL2
// Docker daemon serves the per-poll `GET /containers/<id>/json` slowly
// enough to exhaust 10s while redis has already logged readiness. Moving
// this helper here must preserve that override.
func StartRedis(t *testing.T) string
```

Move the existing helpers out of `internal/store/redis`, `internal/store/postgres`, `internal/job` and `internal/scheduler` into this package and have those tests call it. **Re-run `go test ./... -tags=integration -race -count=3` afterwards** and confirm it is still green — this refactor touches the exact code path the phase's known flake lived in.

`internal/testfixtures/tenant.go` produces a complete tenant:

```go
type Tenant struct {
	Company   model.Company
	Users     map[model.UserRole]model.User // one user of EVERY role
	Buildings []model.Building              // at least two, to make scope isolation testable
	Analyzers []model.Analyzer              // at least two per building
	Plants    []model.PowerPlant
	Tariffs   []model.Tariff
	Scope     store.Scope                   // scoped to Buildings[0] only
	AdminScope store.Scope                  // AllBuildings: true
}

// NewTenant inserts a complete tenant and returns it. It is deterministic
// given the same seed so that failures reproduce.
func NewTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, seed int64) Tenant
```

**Two buildings and a `Scope` covering only the first is not incidental** — it is what makes Task 13's scope-isolation test able to prove a leak.

- [ ] **Step 6: Fold in the F1-owned deferred minors**

Both are in the store layer this task owns:

**(a) `scrubErr` builds its error with `%s`, so `errors.Is`/`errors.As` cannot see through it** (`internal/store/redis/scrub.go:28`, `internal/store/postgres/scrub.go:29,40`). The F0 final review established that the `%w` change can be made alone — the claimed coupling to the scheduler's `isCleanShutdown` narrowing does not exist — and that the real trade-off is that `%w` exposes the unscrubbed cause through `errors.Unwrap`.

**Ruling for this task:** do not use `%w`. Introduce a small error type that keeps the redacted text as its `Error()` while still supporting traversal:

```go
type scrubbed struct {
	msg   string // already redacted; the ONLY text ever printed
	cause error
}

func (e *scrubbed) Error() string { return e.msg }
func (e *scrubbed) Unwrap() error { return e.cause }
```

Tests must assert **both** halves: `errors.Is(err, pgx.ErrNoRows)` succeeds, **and** `err.Error()` never contains the password. Document inline that `errors.Unwrap(err).Error()` still yields unredacted text and is a deliberate, caller-visible act.

**(b) `scrubParseErr` is character-identical in `internal/job/scrub.go` and `internal/store/redis/scrub.go`.** Move it to `internal/platform/secret` (which both may import; `internal/job` must not import `internal/store`) and delete both copies. `internal/store/postgres/scrub.go` has genuinely diverged and keeps its own implementation — do not force it into the shared helper.

- [ ] **Step 7: Extend `no-float-money` and gate**

`.golangci.yml`'s `no-float-money` depguard rule was written in F0 to target packages that did not exist yet. Point it at `internal/domain/model`, `internal/store/postgres` and `internal/store`. Confirm it fires by temporarily declaring a `float64` money field, then revert.

```bash
make lint && make test && make build
go test ./... -tags=integration -race -count=3
git add internal/store internal/domain internal/testfixtures internal/arch internal/job internal/platform .golangci.yml
git commit -m "feat(f1): scoped repository contract, fixture factory and store error traversal"
```

---

## Task 9: Tenancy, building, analyzer and plant repositories

**Files:**
- Create: `internal/store/postgres/tenancy.go`, `buildings.go`, `analyzers.go`, `plants.go`
- Create: `internal/store/postgres/queries/{companies,users,sessions,audit,buildings,analyzers,plants}.sql`
- Test: `internal/store/postgres/tenancy_integration_test.go`, `buildings_integration_test.go`, `analyzers_integration_test.go`, `plants_integration_test.go`

**Interfaces:**
- Consumes: `store.Scope`, `store.*Repository`, `model.*` and `testfixtures.NewTenant` from Task 8; `sqlcgen` from Task 7.
- Produces: `postgres.NewBuildingRepository(pool)` etc., each satisfying its `store` interface. Task 13's scope-isolation test uses these.

- [ ] **Step 1: Write the failing tests, scope isolation first**

For **every** repository in this task, the first test written is the isolation test — not the happy path:

```go
func TestBuildingRepositoryNeverReturnsAnotherCompanysRows(t *testing.T) {
	ctx := context.Background()
	pool := newMigratedPool(t)
	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewBuildingRepository(pool)

	_, err := repo.Get(ctx, mine.AdminScope, theirs.Buildings[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound,
		"another company's building must be indistinguishable from a missing one")

	got, err := repo.List(ctx, mine.AdminScope, store.BuildingFilter{})
	require.NoError(t, err)
	for _, b := range got {
		require.Equal(t, mine.Company.ID, b.CompanyID)
	}
}

func TestBuildingRepositoryHonoursANarrowScope(t *testing.T) {
	ctx := context.Background()
	pool := newMigratedPool(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewBuildingRepository(pool)

	got, err := repo.List(ctx, tenant.Scope, store.BuildingFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1, "a scope naming one building must not return the others")
	require.Equal(t, tenant.Buildings[0].ID, got[0].ID)
}

func TestBuildingRepositoryRejectsAnInvalidScope(t *testing.T) {
	repo := postgres.NewBuildingRepository(newMigratedPool(t))
	_, err := repo.List(context.Background(), store.Scope{}, store.BuildingFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}
```

Then the CRUD tests: create, get, update, soft-delete, and a test proving a soft-deleted row disappears from `Get` and `List`.

- [ ] **Step 2: Run and watch them fail; Step 3: implement**

Each repository is a thin wrapper over `sqlcgen`: validate the scope, call the generated query, map rows to `model` types, translate `pgx.ErrNoRows` to `store.ErrNotFound` and unique violations to `store.ErrConflict` — through the package's `scrubErr` so no DSN can reach a caller.

Every list query uses the `all_buildings` / `building_ids` pair from Task 7 Step 2. Every query filters `deleted_at is null` on tables that have it.

- [ ] **Step 4: Run, gate and commit**

```bash
go test ./internal/store/postgres/ -tags=integration -race -count=1 -v
make lint && make test && make build
git add internal/store/postgres
git commit -m "feat(f1): tenancy, building, analyzer and plant repositories"
```

---

## Task 10: Time-series repositories

**Files:**
- Create: `internal/store/postgres/readings.go`, `cursors.go`, `anomalies.go`, `production.go`, `prices.go`, `forecasts.go`
- Create: matching `queries/*.sql` and `*_integration_test.go` files

**Interfaces:**
- Consumes: Task 8's contract, Task 7's `sqlcgen`, the hypertables and aggregates from Task 2.
- Produces:

```go
// BulkInsertReadings uses COPY into a staging table followed by an
// idempotent `insert ... on conflict do update`, per 04-data-model.md §14.
// Row-by-row inserts are not acceptable here: F2 ingests millions of rows.
func (r *ReadingRepository) BulkInsert(ctx context.Context, s store.Scope, rows []model.MeterReading) (inserted, updated int, err error)

// Range returns readings for one analyzer over [from, to). A query with no
// time bound is a bug (04-data-model.md §14) and this signature makes one
// impossible to express.
func (r *ReadingRepository) Range(ctx context.Context, s store.Scope, analyzerID uuid.UUID, from, to time.Time, kind model.ReadingKind) ([]model.MeterReading, error)

// BoundaryReadings returns the last reading at or before each of start and
// end — the exact operation 02-domain-rules.md §3.1 specifies for billing.
// It reads meter_readings directly, NEVER a continuous aggregate.
func (r *ReadingRepository) BoundaryReadings(ctx context.Context, s store.Scope, analyzerID uuid.UUID, start, end time.Time) (startReading, endReading *model.MeterReading, err error)
```

- [ ] **Step 1: Write the failing tests**

Beyond scope isolation (same three tests as Task 9, for each repository), these behaviours are specified and must be pinned:

```go
// TestBulkInsertIsIdempotent covers 04-data-model.md §14: re-ingesting the
// same window must update, never duplicate. F2's resumable ingestion
// depends entirely on this.
func TestBulkInsertIsIdempotent(t *testing.T) {
	// insert 100 readings, insert the SAME 100 again,
	// require row count == 100 and updated == 100 on the second call
}

// TestBoundaryReadingsYieldsNoRowWhenAReadingIsMissing covers
// 02-domain-rules.md §3.1: "If either reading is missing, or if
// reading_start and reading_end are the same reading, the period yields no
// row — it is not emitted as zero." A zero here becomes a wrong invoice.
func TestBoundaryReadingsYieldsNoRowWhenAReadingIsMissing(t *testing.T) {
	// with no reading at or before start, require startReading == nil
	// and NOT a zero-valued struct
}

// TestReadingsStoreMultipliedValues covers 02-domain-rules.md §2.2: the
// multiplier is applied once at ingestion and the multiplier used is
// recorded on the row.
func TestReadingsStoreMultipliedValues(t *testing.T) { /* ... */ }
```

- [ ] **Step 2: Run and watch them fail; Step 3: implement**

`BulkInsert` uses `pgx`'s `CopyFrom` into a temporary staging table, then one `insert ... select ... on conflict (analyzer_id, ts, kind) do update`. Return real `inserted`/`updated` counts — the spec's job model requires explicit `processed`/`skipped`/`failed` counts, and guessing them is not acceptable.

Analytics reads go to the continuous aggregates; billing-grade reads go to `meter_readings`. Name them so the difference is impossible to miss (`analytics.go` vs `BoundaryReadings`), per `04-data-model.md` §4.3's explicit instruction that "this distinction must be explicit in the code".

- [ ] **Step 4: Run, gate and commit**

```bash
go test ./internal/store/postgres/ -tags=integration -race -count=1 -v
make lint && make test && make build
git add internal/store/postgres
git commit -m "feat(f1): reading, cursor, anomaly, production, price and forecast repositories"
```

---

## Task 11: Tariff, bill, report, alarm, carbon, ISO, file, integration and operations repositories

**Files:**
- Create: `internal/store/postgres/tariffs.go`, `bills.go`, `reports.go`, `alarms.go`, `carbon.go`, `iso50001.go`, `files.go`, `integrations.go`, `ops.go`
- Create: matching `queries/*.sql` and `*_integration_test.go` files
- Create: `internal/store/postgres/admin/admin.go`

**Interfaces:**
- Consumes: Task 8's contract, Task 7's `sqlcgen`, migrations from Tasks 3–6.
- Produces: one repository per aggregate, plus the **only** unscoped surface in the codebase:

```go
// Package admin holds the only unscoped query surface in the system
// (03-target-architecture.md §2.5). Every method here can read across
// tenants; that is the point, and it is why this package is exempted from
// TestEveryStoreMethodIsScoped and why nothing outside operator tooling may
// import it.
package admin
```

- [ ] **Step 1: Write the failing tests**

Scope isolation for every repository (as Task 9), plus:

```go
// TestSupersedeReplacesRatherThanDeletes covers 04-data-model.md §14.
func TestSupersedeReplacesRatherThanDeletes(t *testing.T) { /* ... */ }

// TestIntegrationCredentialsAreNeverReturnedInPlaintext proves the
// repository returns ciphertext or a decrypted value only through an
// explicit method, and that the ordinary Get never carries the secret.
func TestIntegrationCredentialsAreNeverReturnedInPlaintext(t *testing.T) { /* ... */ }

// TestEffectiveTariffPicksTheLatestNotAfterTheDate pins tariff resolution
// by effective_from — the wrong row here misprices every invoice.
func TestEffectiveTariffPicksTheLatestNotAfterTheDate(t *testing.T) { /* ... */ }
```

- [ ] **Step 2: Run and watch them fail; Step 3: implement**

Same shape as Task 9. `integration_credentials.secret_enc` and `smtp_settings.password_enc` are sealed and opened with `internal/platform/crypto`'s existing AES-256-GCM helpers from F0; the ordinary `Get` must not return plaintext.

Add exactly one `admin` method for now — enough to prove the guard's exemption works — and no more. Every method added there is a permanent cross-tenant hole.

- [ ] **Step 4: Confirm the scope guard still passes with `admin` present**

```bash
go test ./internal/arch/ -race -run TestEveryStoreMethodIsScoped -v
```
Expected: PASS, with the `admin` package skipped and every other repository method scoped.

- [ ] **Step 5: Run, gate and commit**

```bash
go test ./internal/store/postgres/ -tags=integration -race -count=1 -v
make lint && make test && make build
git add internal/store/postgres
git commit -m "feat(f1): tariff, bill, report, alarm, carbon, iso, file and operations repositories"
```

---

## Task 12: Seed loader

**Files:**
- Create: `internal/seed/seed.go`, `internal/seed/data/*.json`, `internal/seed/seed_integration_test.go`
- Modify: `internal/cli/seed.go` (replace the F0 stub)

**Interfaces:**
- Consumes: repositories from Tasks 9 and 11.
- Produces: `seed.Load(ctx, pool, log) (seed.Result, error)` where `Result` carries per-dataset counts.

Datasets, per `04-data-model.md` §13: emission-factor master catalogue, GHG↔ISO mapping, integration provider definitions, national tariff schedule, ISO 50001 clause texts and templates. Data files are embedded with `go:embed` so the offline bundle carries them.

- [ ] **Step 1: Write the failing idempotency test**

```go
// TestSeedRunTwiceProducesTheSameRowCounts is the F1 acceptance criterion.
// It is the whole contract: the offline bundle re-runs seed on every
// upgrade, so a second run that duplicated rows would corrupt the factor
// catalogue every release.
func TestSeedRunTwiceProducesTheSameRowCounts(t *testing.T) {
	ctx := context.Background()
	pool := newMigratedPool(t)

	first, err := seed.Load(ctx, pool, discardLogger())
	require.NoError(t, err)
	countsAfterFirst := tableCounts(t, ctx, pool)

	second, err := seed.Load(ctx, pool, discardLogger())
	require.NoError(t, err)
	require.Equal(t, countsAfterFirst, tableCounts(t, ctx, pool),
		"a second seed run must not change any row count")
	require.Equal(t, first.Total(), second.Total())
}

// TestSeedRepairsAModifiedRow proves seeding is idempotent by CONVERGENCE,
// not by skipping work: an operator who edits a master factor must get the
// shipped value back, otherwise "idempotent" would just mean "does nothing
// the second time" and a corrupted catalogue would never heal.
func TestSeedRepairsAModifiedRow(t *testing.T) { /* mutate a factor, re-seed, assert restored */ }
```

- [ ] **Step 2: Run and watch it fail; Step 3: implement**

Every insert is `insert ... on conflict (<natural key>) do update set ...`. The master catalogue's natural key is `(coalesce(company_id, nil-uuid), key)` — the same expression as the unique index from Task 4. Company-owned overrides (`company_id is not null`) must **never** be touched by seeding.

`ekokod seed` prints per-dataset counts and exits non-zero on any failure.

- [ ] **Step 4: Run, gate and commit**

```bash
go test ./internal/seed/ -tags=integration -race -count=1 -v
make lint && make test && make build
git add internal/seed internal/cli
git commit -m "feat(f1): idempotent reference-data seed loader"
```

---

## Task 13: Performance and acceptance suite

**Files:**
- Create: `internal/store/postgres/performance_integration_test.go`, `internal/store/postgres/aggregates_integration_test.go`
- Modify: `Makefile` (a `test-perf` target), `HANDOFF_NEXT_SESSION.md`

**Interfaces:**
- Consumes: everything.
- Produces: the evidence for the remaining F1 acceptance criteria.

These tests are slow. Guard them with `testing.Short()` and give them their own Make target so the ordinary integration run stays fast; CI runs them on a schedule, not per push.

- [ ] **Step 1: Write the 1,000,000-row test**

```go
// TestOneMillionReadingsChunkLayout is the F1 acceptance criterion:
// "Inserting 1,000,000 synthetic readings across 100 analyzers completes and
// produces the expected chunk count; a 1-month range query for one analyzer
// touches only the expected chunks (verified with EXPLAIN)."
func TestOneMillionReadingsChunkLayout(t *testing.T) {
	if testing.Short() { t.Skip("slow: inserts 1M rows") }
	// 100 analyzers x 10,000 readings, spanning ~1 year at 15-minute intervals,
	// inserted through ReadingRepository.BulkInsert in batches.
	//
	// Then assert:
	//   - chunk count from timescaledb_information.chunks matches the count
	//     implied by a 7-day chunk_time_interval and 16 space partitions
	//     over the generated span. COMPUTE the expectation from the span and
	//     the configured interval; do not hardcode a number observed from a
	//     run, or the test pins whatever the code happens to do.
	//   - `explain (format json)` for a one-month single-analyzer query names
	//     strictly fewer chunks than the total, and none outside the range.
}
```

The `EXPLAIN` assertion is the one that matters: it is the difference between chunk exclusion working and the query reading the whole table. Assert on the plan, not on wall-clock time — a timing assertion would be exactly the kind of load-sensitive bound that produced this phase's known flake.

- [ ] **Step 2: Write the continuous-aggregate correctness test**

```go
// TestContinuousAggregatesMatchAHandComputedFixture is the F1 acceptance
// criterion. It uses a SMALL fixture whose expected values are written out
// by hand in the test, not computed by the same SQL under test.
func TestContinuousAggregatesMatchAHandComputedFixture(t *testing.T) {
	// Insert readings with known register values across three hours, e.g.
	// active_import 100, 150, 175 at 00:00, 00:30, 01:00.
	// refresh_continuous_aggregate, then require:
	//   hour 0 active_consumption == 75  (175-100 is NOT the hour-0 answer;
	//   the bucket holds 100 and 150, so 50 — state the expected value
	//   explicitly and derive it by hand from 02-domain-rules.md §3.1.)
	//
	// Also assert the boundary-semantics caveat from 04-data-model.md §4.3:
	// the aggregate's sum across buckets is LESS than the domain function's
	// answer over the same span, because the aggregate misses the step
	// between buckets. Pin that difference — it is a documented, deliberate
	// property, and a future change that silently "fixes" it would break
	// the billing/analytics split.
}
```

- [ ] **Step 3: Write the compression test**

```go
// TestCompressedChunksStillReturnCorrectRows is the F1 acceptance criterion
// "compression policy compresses a chunk older than the threshold, and
// queries still return correct results from it".
func TestCompressedChunksStillReturnCorrectRows(t *testing.T) {
	// Insert readings older than the 90-day threshold, run
	// `select compress_chunk(c) from show_chunks('meter_readings', older_than => interval '90 days') c`,
	// assert timescaledb_information.chunks reports is_compressed,
	// then re-read through ReadingRepository.Range and require byte-identical
	// values to what was written.
}
```

Call `compress_chunk` directly rather than waiting for the background policy — waiting on a Timescale background worker is a timing dependency, and this phase has already paid for one of those.

- [ ] **Step 4: Write the cross-tenant proof**

```go
// TestScopeIsolation is named in the F1 verification block
// (`go test ./internal/store/postgres -run TestScopeIsolation`). It is the
// phase's single most important test: it walks EVERY repository and proves
// none returns another company's rows.
func TestScopeIsolation(t *testing.T) {
	// Build two complete tenants with testfixtures.NewTenant.
	// For every repository, call every read method with tenant A's scope and
	// tenant B's ids, and require ErrNotFound or an empty result.
	// Table-driven, so a repository added in a later phase that forgets to
	// register here is visible as an omission.
}
```

- [ ] **Step 5: Run the full suite**

```bash
make lint && make test && make build
go test ./... -tags=integration -race -count=1
go test ./internal/store/postgres/ -tags=integration -race -count=1 -run 'TestOneMillion|TestContinuousAggregatesMatch|TestCompressed|TestScopeIsolation' -v
```

- [ ] **Step 6: Run the F1 acceptance block verbatim**

From `docs/rewrite/09-implementation-plan.md` §F1, adapted to the `ekokod` binary name:

```bash
make migrate && ./bin/ekokod migrate down --all && ./bin/ekokod migrate up
go test ./internal/store/... -tags=integration -v
go test ./internal/store/postgres -tags=integration -run TestScopeIsolation -v
psql -c "select hypertable_name, num_chunks from timescaledb_information.hypertables;"
```

Record the real output of each in the task report. **Do not paste a report's claim — run each yourself** (F0 standing ruling: do not trust a report, re-run the gate).

- [ ] **Step 7: Update the handoff and commit**

Rewrite `HANDOFF_NEXT_SESSION.md` for F2: what F1 delivered, every ruling made, the deferred minors with owning phases, and the open product-owner questions (which now block F4 and should be raised).

```bash
git add internal/store/postgres Makefile HANDOFF_NEXT_SESSION.md
git commit -m "test(f1): performance, aggregate correctness and scope isolation suite"
```

---

## Carried forward — explicitly NOT F1

Verified still open against the current tree. Each belongs to a later phase; recorded here so they are not lost.

- **`internal/scheduler/elector.go:152`** — `pg_advisory_unlock`'s boolean is discarded (`Exec`, not `QueryRow(...).Scan`), so a `false` (this session never held the lock) is indistinguishable from a clean release. **Owner: the phase that adds the first real scheduled job.**
- **`internal/scheduler/elector.go:161-163`** — a deterministically panicking `lead` is re-invoked every `retry` (10s in production) forever: no backoff, no failure counter, no circuit breaker, and the log fills at 10s cadence. **Owner: same.**
- **`internal/scheduler/scheduler.go:69-80`** — `asynq.NewScheduler` eagerly builds and owns a go-redis client; both early-return paths leak it, and `Shutdown()` would not help because asynq returns early for a never-started scheduler. Combined with the retry loop above, a bad cron entry leaks one client and pool every 10 seconds. The fix is `asynq.NewSchedulerFromRedisClient` with a caller-owned client. **Owner: the phase that registers real cron entries** — currently unreachable, since only the noop task is registered.
- **`internal/api/middleware/ratelimit.go:30`** — `Retry-After: 60` is a literal, wrong for the 15-minute auth window. Line 24's reap interval is a hardcoded 10 minutes, and line 65 uses `time.Tick`, so the reaper goroutine and its ticker are unstoppable for the process lifetime. **Owner: F6 (auth).**
- **Config knobs still unbounded** (`internal/platform/config/load.go:387-392`, deliberate): `EKOKOD_JOB_MAX_RETRIES`, `EKOKOD_DB_MAX_CONNS`, `EKOKOD_DB_MIN_CONNS`, `EKOKOD_PASSWORD_HISTORY_SIZE` use plain `intVal`; `EKOKOD_COMPRESSION_AFTER` and `EKOKOD_READING_RETENTION` accept negatives. **`EKOKOD_COMPRESSION_AFTER` and `EKOKOD_READING_RETENTION` become live in F1** — if either is wired to a Timescale policy in Task 2, bound it in that task.
- **`internal/platform/logging` redaction is key-based only** — `redactAttr` never scans attribute values, so every `slog.String("error", err.Error())` depends on the error having been scrubbed at construction. Phase-wide invariant; re-audit whenever a new error path reaches a log.
- **`check-i18n-parity.mjs`** compares key paths but not leaf value types. **Owner: the first phase that adds real UI copy.**

---

## Self-review notes

**Spec coverage.** Every section of `04-data-model.md` maps to a task: §1 exists (00001), §2–§3 → Task 1, §4 and §12 → Task 2, §5 → Task 3, §6 and §8 → Task 6, §7 → Task 6, §9–§10 → Task 4, §11 → Task 5, §13 → Task 12, §14 → Tasks 10 and 13. All seven F1 acceptance criteria are claimed by a named test: up/down/up → Task 1; 1M rows and `EXPLAIN` → Task 13; aggregate correctness → Task 13; compression → Task 13; every repository method covered → Tasks 9–11; cross-tenant proof → Task 13; seed twice → Task 12.

**Known risks, stated rather than hidden.**
1. **sqlc versus TimescaleDB** is the single largest unknown. Task 7 Step 1 is a spike with a written decision procedure and a mandatory drift check on the fallback path, rather than a guess baked into the plan.
2. **Hierarchical continuous aggregates** may be rejected by Timescale 2.30 for `first()`/`last()`. Task 2 Step 4 says to try, fall back to defining over the hypertable, and *record which form was used* — because Task 13's correctness test depends on knowing.
3. **`plant_production`'s nullable `device_id` inside its primary key** is a probable spec defect. Task 2 Step 3 instructs the implementer to report it rather than resolve it unilaterally.
4. **Task 11 is the largest task.** If its review round is slow, split it at the file boundaries already listed; the repositories are independent of one another.

**Type consistency.** `store.Scope`, `store.ErrNotFound`, `store.ErrConflict`, `store.ErrInvalidScope`, `model.*`, `testfixtures.NewTenant`, `testfixtures.StartPostgres`, `testfixtures.StartRedis` and the `all_buildings`/`building_ids` query pair are defined once (Tasks 7 and 8) and referenced by those exact names in Tasks 9–13.
