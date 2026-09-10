-- +goose NO TRANSACTION
-- This file is applied outside a transaction. That is not a preference: goose's
-- default transactional apply was tried first and TimescaleDB rejected it with
-- "CREATE MATERIALIZED VIEW ... WITH DATA cannot run inside a transaction block"
-- (SQLSTATE 25001), because creating a continuous aggregate refreshes it, and a
-- refresh commits its own work. The cost is that a failure part-way through
-- leaves the earlier aggregates created; the down section is written so that
-- rolling back from a partial state is safe, since every drop is `if exists`.
-- +goose Up
-- Consumption and production continuous aggregates. Transcribed from
-- docs/rewrite/04-data-model.md section 4.3, whose consumption_hourly
-- definition is authoritative for the column list.
--
-- TWO DELIBERATE DEVIATIONS FROM THE SPEC'S FENCED SQL, both recorded in the
-- task report:
--
-- 1. Every computed column carries an explicit ::numeric cast. In Postgres this
--    is a no-op — numeric minus numeric is already numeric, and max(numeric) is
--    already numeric — so no stored value changes. It exists for sqlc, which
--    does not know TimescaleDB's first()/last() and otherwise guesses the column
--    type: a spike generated `ActiveConsumption int32` for an energy quantity,
--    which no test or linter would catch and which would surface as wrong kWh on
--    an invoice. reading_count is left uncast because count(*) is a bigint and a
--    row count is not a numeric quantity.
--
-- 2. consumption_daily, _monthly and _yearly are defined DIRECTLY over
--    meter_readings rather than hierarchically over the level below. TimescaleDB
--    2.30 does permit first()/last() over a continuous aggregate — that was
--    tested, it is not a platform limitation — but consumption_hourly exposes a
--    start value only for active_import. A hierarchical roll-up could therefore
--    reproduce "last minus first across the whole bucket" for exactly one of the
--    twelve delta columns and would have to sum intra-hour deltas for the other
--    eleven, which drops every inter-bucket step: a day would lose 24 steps
--    instead of 1, and the four levels would no longer mean the same thing.
--    Section 4.3's own escape clause — "or defined directly over the hypertable
--    where boundary semantics require it" — is what applies here. Exposing the
--    missing start values would mean adding columns the spec does not define,
--    which this migration is not free to do.
--
-- BUCKET TIMEZONE. Every bucket of a day or wider takes 'Europe/Istanbul'
-- explicitly. 02-domain-rules.md section 1 evaluates all day, month and billing
-- boundaries in Istanbul local time, and time_bucket without a timezone buckets
-- in UTC: at UTC+3 the row labelled 1 January would actually cover 03:00 on the
-- 1st to 03:00 on the 2nd, so local 00:00-03:00 consumption would be filed under
-- the previous day, and a "January" total would drop the first three hours of
-- January while picking up the first three of February. consumption_hourly is
-- deliberately left timezone-free: Istanbul is a whole number of hours from UTC,
-- so the argument would be a no-op there, and omitting it keeps that aggregate a
-- fixed-width bucket.
--
-- REAL-TIME AGGREGATION IS ON FOR THE DAILY-OR-FINER VIEWS ONLY.
-- TimescaleDB never materialises a bucket the refresh window only partly covers,
-- at any end_offset, so the hour, day, month or year currently in progress is
-- never in the materialised data. On a materialized_only view that means a query
-- for "today" returns no row at all — absent, not stale, which is the same silent
-- undercount as an unrefreshed backfill. consumption_hourly, consumption_daily
-- and plant_production_daily therefore set materialized_only = false: the query
-- unions the materialised buckets with a live scan above the materialisation
-- watermark, and for those three the un-materialised tail is at most one open
-- bucket — a day of raw readings for one analyzer, which is cheap.
--
-- consumption_monthly, consumption_yearly and plant_production_monthly keep
-- materialized_only = true deliberately. Their watermark sits at the end of the
-- last CLOSED month or year, so a live tail would be months of raw readings: in
-- September, consumption_yearly would rescan nine months per analyzer on every
-- read. A multi-million-row scan behind an innocuous dashboard query is a worse
-- outcome than composing the figure explicitly.
--
-- CONSTRAINT ON TASK 10 AND ANYTHING THAT READS THESE VIEWS: month-to-date and
-- year-to-date MUST be composed — the closed buckets from consumption_monthly or
-- consumption_yearly, PLUS the open period taken from consumption_daily or from
-- meter_readings. The open month and the open year are not in those views and
-- must never be assumed to be. Do not "fix" a missing month-to-date row by
-- flipping materialized_only; that trades a visible gap for an invisible table
-- scan, and migrations_timeseries_integration_test.go asserts the current
-- setting of all six views.
--
-- OPERATOR NOTE - AFTER ANY HISTORICAL BACKFILL, REFRESH EVERY AGGREGATE ONCE.
-- A refresh policy only ever materialises [now - start_offset, now - end_offset].
-- Any bucket that was never inside a refresh window is not stale, it is ABSENT:
-- the query succeeds and quietly returns fewer rows than the truth. Real-time
-- aggregation does NOT rescue this, which is the part worth knowing — it unions
-- in raw rows only from ABOVE the materialisation watermark, and backfilled
-- history sits below it, so old data is missing from consumption_hourly,
-- consumption_daily and plant_production_daily exactly as it is from the three
-- materialized_only views. Loading ten years of legacy readings would leave
-- consumption_yearly holding five years, consumption_monthly one, and
-- consumption_daily ninety days. After a backfill, run once per aggregate,
-- oldest bucket width last:
--
--     call refresh_continuous_aggregate('consumption_hourly',       NULL, now());
--     call refresh_continuous_aggregate('consumption_daily',        NULL, now());
--     call refresh_continuous_aggregate('consumption_monthly',      NULL, now());
--     call refresh_continuous_aggregate('consumption_yearly',       NULL, now());
--     call refresh_continuous_aggregate('plant_production_daily',   NULL, now());
--     call refresh_continuous_aggregate('plant_production_monthly', NULL, now());
--
-- This migration cannot do it: there is no data to refresh at migration time,
-- and a refresh commits its own work.
--
-- Refresh offsets: the spec fixes consumption_hourly (start 30 days, end 1 hour,
-- schedule 30 minutes). The others keep end_offset at 1 hour rather than one
-- bucket width so that a day, month or year is materialised as soon as it
-- closes; a bucket-width end_offset would delay each level by a further whole
-- bucket. It does not make the bucket currently in progress visible — Timescale
-- does not materialise a bucket the refresh window only partly covers, and with
-- materialized_only the unmaterialised part of the view is absent rather than
-- computed live. start_offset covers the late-arriving-data window at that
-- granularity, and schedule_interval grows with the bucket so a yearly roll-up
-- is not recomputed every thirty minutes.

create materialized view consumption_hourly
with (timescaledb.continuous, timescaledb.materialized_only = false) as
select
    analyzer_id,
    time_bucket('1 hour', ts) as bucket,
    first(active_import, ts)::numeric      as active_import_start,
    last(active_import, ts)::numeric       as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric             as active_consumption,
    (last(reactive_inductive_import, ts)  - first(reactive_inductive_import, ts))::numeric  as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric                     as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric                     as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric                     as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric             as active_generation,
    (last(reactive_inductive_export, ts)  - first(reactive_inductive_export, ts))::numeric  as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric                     as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric                     as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric                     as t3_generation,
    max(max_demand_kw)::numeric                                              as max_demand_kw,
    last(active_import, ts)::numeric              as active_index,
    last(reactive_inductive_import, ts)::numeric  as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric                  as t1_index,
    last(t2_import, ts)::numeric                  as t2_index,
    last(t3_import, ts)::numeric                  as t3_index,
    last(active_export, ts)::numeric              as active_generation_index,
    count(*)                                      as reading_count
from meter_readings
where kind = 'load_profile'
group by analyzer_id, bucket;

select add_continuous_aggregate_policy('consumption_hourly',
    start_offset      => interval '30 days',
    end_offset        => interval '1 hour',
    schedule_interval => interval '30 minutes');

create materialized view consumption_daily
with (timescaledb.continuous, timescaledb.materialized_only = false) as
select
    analyzer_id,
    time_bucket('1 day', ts, 'Europe/Istanbul') as bucket,
    first(active_import, ts)::numeric      as active_import_start,
    last(active_import, ts)::numeric       as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric             as active_consumption,
    (last(reactive_inductive_import, ts)  - first(reactive_inductive_import, ts))::numeric  as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric                     as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric                     as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric                     as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric             as active_generation,
    (last(reactive_inductive_export, ts)  - first(reactive_inductive_export, ts))::numeric  as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric                     as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric                     as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric                     as t3_generation,
    max(max_demand_kw)::numeric                                              as max_demand_kw,
    last(active_import, ts)::numeric              as active_index,
    last(reactive_inductive_import, ts)::numeric  as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric                  as t1_index,
    last(t2_import, ts)::numeric                  as t2_index,
    last(t3_import, ts)::numeric                  as t3_index,
    last(active_export, ts)::numeric              as active_generation_index,
    count(*)                                      as reading_count
from meter_readings
where kind = 'load_profile'
group by analyzer_id, bucket;

select add_continuous_aggregate_policy('consumption_daily',
    start_offset      => interval '90 days',
    end_offset        => interval '1 hour',
    schedule_interval => interval '1 hour');

create materialized view consumption_monthly
with (timescaledb.continuous) as
select
    analyzer_id,
    time_bucket('1 month', ts, 'Europe/Istanbul') as bucket,
    first(active_import, ts)::numeric      as active_import_start,
    last(active_import, ts)::numeric       as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric             as active_consumption,
    (last(reactive_inductive_import, ts)  - first(reactive_inductive_import, ts))::numeric  as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric                     as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric                     as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric                     as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric             as active_generation,
    (last(reactive_inductive_export, ts)  - first(reactive_inductive_export, ts))::numeric  as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric                     as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric                     as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric                     as t3_generation,
    max(max_demand_kw)::numeric                                              as max_demand_kw,
    last(active_import, ts)::numeric              as active_index,
    last(reactive_inductive_import, ts)::numeric  as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric                  as t1_index,
    last(t2_import, ts)::numeric                  as t2_index,
    last(t3_import, ts)::numeric                  as t3_index,
    last(active_export, ts)::numeric              as active_generation_index,
    count(*)                                      as reading_count
from meter_readings
where kind = 'load_profile'
group by analyzer_id, bucket;

select add_continuous_aggregate_policy('consumption_monthly',
    start_offset      => interval '1 year',
    end_offset        => interval '1 hour',
    schedule_interval => interval '6 hours');

create materialized view consumption_yearly
with (timescaledb.continuous) as
select
    analyzer_id,
    time_bucket('1 year', ts, 'Europe/Istanbul') as bucket,
    first(active_import, ts)::numeric      as active_import_start,
    last(active_import, ts)::numeric       as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric             as active_consumption,
    (last(reactive_inductive_import, ts)  - first(reactive_inductive_import, ts))::numeric  as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric                     as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric                     as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric                     as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric             as active_generation,
    (last(reactive_inductive_export, ts)  - first(reactive_inductive_export, ts))::numeric  as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric                     as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric                     as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric                     as t3_generation,
    max(max_demand_kw)::numeric                                              as max_demand_kw,
    last(active_import, ts)::numeric              as active_index,
    last(reactive_inductive_import, ts)::numeric  as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric                  as t1_index,
    last(t2_import, ts)::numeric                  as t2_index,
    last(t3_import, ts)::numeric                  as t3_index,
    last(active_export, ts)::numeric              as active_generation_index,
    count(*)                                      as reading_count
from meter_readings
where kind = 'load_profile'
group by analyzer_id, bucket;

select add_continuous_aggregate_policy('consumption_yearly',
    start_offset      => interval '5 years',
    end_offset        => interval '1 hour',
    schedule_interval => interval '1 day');

-- Solar production roll-ups (section 4.5: "with plant_production_daily /
-- _monthly continuous aggregates"). Defined directly over plant_production for
-- the same reason as above and one more: avg(efficiency_pct) does not roll up —
-- an average of daily averages is not the monthly average unless every day
-- carries the same number of samples.

create materialized view plant_production_daily
with (timescaledb.continuous, timescaledb.materialized_only = false) as
select
    plant_id,
    time_bucket('1 day', ts, 'Europe/Istanbul') as bucket,
    sum(production_kwh)::numeric   as production_kwh,
    max(active_power_kw)::numeric  as max_active_power_kw,
    avg(efficiency_pct)::numeric   as avg_efficiency_pct
from plant_production
group by plant_id, bucket;

select add_continuous_aggregate_policy('plant_production_daily',
    start_offset      => interval '90 days',
    end_offset        => interval '1 hour',
    schedule_interval => interval '1 hour');

create materialized view plant_production_monthly
with (timescaledb.continuous) as
select
    plant_id,
    time_bucket('1 month', ts, 'Europe/Istanbul') as bucket,
    sum(production_kwh)::numeric   as production_kwh,
    max(active_power_kw)::numeric  as max_active_power_kw,
    avg(efficiency_pct)::numeric   as avg_efficiency_pct
from plant_production
group by plant_id, bucket;

select add_continuous_aggregate_policy('plant_production_monthly',
    start_offset      => interval '1 year',
    end_offset        => interval '1 hour',
    schedule_interval => interval '6 hours');

-- +goose Down
-- Reverse creation order, and deliberately without `cascade`. Each refresh
-- policy and each materialisation hypertable is dropped with its view, so there
-- is nothing to unregister by hand — but a forgotten drop here leaves a view
-- standing whose base table 00004's down section then cannot drop, which is
-- what the reversibility guard in migrations_roundtrip_integration_test.go
-- checks for.
drop materialized view if exists plant_production_monthly;
drop materialized view if exists plant_production_daily;
drop materialized view if exists consumption_yearly;
drop materialized view if exists consumption_monthly;
drop materialized view if exists consumption_daily;
drop materialized view if exists consumption_hourly;
