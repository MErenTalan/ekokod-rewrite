-- +goose NO TRANSACTION
-- Not transactional for the same reason as 00005: creating a continuous
-- aggregate refreshes it, and a refresh commits its own work.
-- +goose Up
-- F15q R470: the consumption aggregates gain what boundary differencing
-- (02 §3.1) needs: first_ts, a first() per register (*_start) and a last()
-- for the five export registers that had none. A continuous aggregate's
-- query cannot be altered, so all four are dropped and rebuilt; existing
-- columns keep their names and meaning. The rebuild re-reads meter_readings.
drop materialized view if exists consumption_yearly;
drop materialized view if exists consumption_monthly;
drop materialized view if exists consumption_daily;
drop materialized view if exists consumption_hourly;

create materialized view consumption_hourly
with (timescaledb.continuous, timescaledb.materialized_only = false) as
select
    analyzer_id,
    time_bucket('1 hour', ts) as bucket,
    first(active_import, ts)::numeric as active_import_start,
    last(active_import, ts)::numeric as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric as active_consumption,
    (last(reactive_inductive_import, ts) - first(reactive_inductive_import, ts))::numeric as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric as active_generation,
    (last(reactive_inductive_export, ts) - first(reactive_inductive_export, ts))::numeric as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric as t3_generation,
    max(max_demand_kw)::numeric as max_demand_kw,
    last(active_import, ts)::numeric as active_index,
    last(reactive_inductive_import, ts)::numeric as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric as t1_index,
    last(t2_import, ts)::numeric as t2_index,
    last(t3_import, ts)::numeric as t3_index,
    last(active_export, ts)::numeric as active_generation_index,
    count(*) as reading_count,
    min(ts)::timestamptz as first_ts,
    first(reactive_inductive_import, ts)::numeric as inductive_consumption_start,
    first(reactive_capacitive_import, ts)::numeric as capacitive_consumption_start,
    first(t1_import, ts)::numeric as t1_consumption_start,
    first(t2_import, ts)::numeric as t2_consumption_start,
    first(t3_import, ts)::numeric as t3_consumption_start,
    first(active_export, ts)::numeric as active_generation_start,
    first(reactive_inductive_export, ts)::numeric as inductive_generation_start,
    first(reactive_capacitive_export, ts)::numeric as capacitive_generation_start,
    first(t1_export, ts)::numeric as t1_generation_start,
    first(t2_export, ts)::numeric as t2_generation_start,
    first(t3_export, ts)::numeric as t3_generation_start,
    last(reactive_inductive_export, ts)::numeric as inductive_generation_index,
    last(reactive_capacitive_export, ts)::numeric as capacitive_generation_index,
    last(t1_export, ts)::numeric as t1_generation_index,
    last(t2_export, ts)::numeric as t2_generation_index,
    last(t3_export, ts)::numeric as t3_generation_index
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
    first(active_import, ts)::numeric as active_import_start,
    last(active_import, ts)::numeric as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric as active_consumption,
    (last(reactive_inductive_import, ts) - first(reactive_inductive_import, ts))::numeric as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric as active_generation,
    (last(reactive_inductive_export, ts) - first(reactive_inductive_export, ts))::numeric as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric as t3_generation,
    max(max_demand_kw)::numeric as max_demand_kw,
    last(active_import, ts)::numeric as active_index,
    last(reactive_inductive_import, ts)::numeric as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric as t1_index,
    last(t2_import, ts)::numeric as t2_index,
    last(t3_import, ts)::numeric as t3_index,
    last(active_export, ts)::numeric as active_generation_index,
    count(*) as reading_count,
    min(ts)::timestamptz as first_ts,
    first(reactive_inductive_import, ts)::numeric as inductive_consumption_start,
    first(reactive_capacitive_import, ts)::numeric as capacitive_consumption_start,
    first(t1_import, ts)::numeric as t1_consumption_start,
    first(t2_import, ts)::numeric as t2_consumption_start,
    first(t3_import, ts)::numeric as t3_consumption_start,
    first(active_export, ts)::numeric as active_generation_start,
    first(reactive_inductive_export, ts)::numeric as inductive_generation_start,
    first(reactive_capacitive_export, ts)::numeric as capacitive_generation_start,
    first(t1_export, ts)::numeric as t1_generation_start,
    first(t2_export, ts)::numeric as t2_generation_start,
    first(t3_export, ts)::numeric as t3_generation_start,
    last(reactive_inductive_export, ts)::numeric as inductive_generation_index,
    last(reactive_capacitive_export, ts)::numeric as capacitive_generation_index,
    last(t1_export, ts)::numeric as t1_generation_index,
    last(t2_export, ts)::numeric as t2_generation_index,
    last(t3_export, ts)::numeric as t3_generation_index
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
    first(active_import, ts)::numeric as active_import_start,
    last(active_import, ts)::numeric as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric as active_consumption,
    (last(reactive_inductive_import, ts) - first(reactive_inductive_import, ts))::numeric as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric as active_generation,
    (last(reactive_inductive_export, ts) - first(reactive_inductive_export, ts))::numeric as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric as t3_generation,
    max(max_demand_kw)::numeric as max_demand_kw,
    last(active_import, ts)::numeric as active_index,
    last(reactive_inductive_import, ts)::numeric as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric as t1_index,
    last(t2_import, ts)::numeric as t2_index,
    last(t3_import, ts)::numeric as t3_index,
    last(active_export, ts)::numeric as active_generation_index,
    count(*) as reading_count,
    min(ts)::timestamptz as first_ts,
    first(reactive_inductive_import, ts)::numeric as inductive_consumption_start,
    first(reactive_capacitive_import, ts)::numeric as capacitive_consumption_start,
    first(t1_import, ts)::numeric as t1_consumption_start,
    first(t2_import, ts)::numeric as t2_consumption_start,
    first(t3_import, ts)::numeric as t3_consumption_start,
    first(active_export, ts)::numeric as active_generation_start,
    first(reactive_inductive_export, ts)::numeric as inductive_generation_start,
    first(reactive_capacitive_export, ts)::numeric as capacitive_generation_start,
    first(t1_export, ts)::numeric as t1_generation_start,
    first(t2_export, ts)::numeric as t2_generation_start,
    first(t3_export, ts)::numeric as t3_generation_start,
    last(reactive_inductive_export, ts)::numeric as inductive_generation_index,
    last(reactive_capacitive_export, ts)::numeric as capacitive_generation_index,
    last(t1_export, ts)::numeric as t1_generation_index,
    last(t2_export, ts)::numeric as t2_generation_index,
    last(t3_export, ts)::numeric as t3_generation_index
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
    first(active_import, ts)::numeric as active_import_start,
    last(active_import, ts)::numeric as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric as active_consumption,
    (last(reactive_inductive_import, ts) - first(reactive_inductive_import, ts))::numeric as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric as active_generation,
    (last(reactive_inductive_export, ts) - first(reactive_inductive_export, ts))::numeric as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric as t3_generation,
    max(max_demand_kw)::numeric as max_demand_kw,
    last(active_import, ts)::numeric as active_index,
    last(reactive_inductive_import, ts)::numeric as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric as t1_index,
    last(t2_import, ts)::numeric as t2_index,
    last(t3_import, ts)::numeric as t3_index,
    last(active_export, ts)::numeric as active_generation_index,
    count(*) as reading_count,
    min(ts)::timestamptz as first_ts,
    first(reactive_inductive_import, ts)::numeric as inductive_consumption_start,
    first(reactive_capacitive_import, ts)::numeric as capacitive_consumption_start,
    first(t1_import, ts)::numeric as t1_consumption_start,
    first(t2_import, ts)::numeric as t2_consumption_start,
    first(t3_import, ts)::numeric as t3_consumption_start,
    first(active_export, ts)::numeric as active_generation_start,
    first(reactive_inductive_export, ts)::numeric as inductive_generation_start,
    first(reactive_capacitive_export, ts)::numeric as capacitive_generation_start,
    first(t1_export, ts)::numeric as t1_generation_start,
    first(t2_export, ts)::numeric as t2_generation_start,
    first(t3_export, ts)::numeric as t3_generation_start,
    last(reactive_inductive_export, ts)::numeric as inductive_generation_index,
    last(reactive_capacitive_export, ts)::numeric as capacitive_generation_index,
    last(t1_export, ts)::numeric as t1_generation_index,
    last(t2_export, ts)::numeric as t2_generation_index,
    last(t3_export, ts)::numeric as t3_generation_index
from meter_readings
where kind = 'load_profile'
group by analyzer_id, bucket;

select add_continuous_aggregate_policy('consumption_yearly',
    start_offset      => interval '5 years',
    end_offset        => interval '1 hour',
    schedule_interval => interval '1 day');

-- +goose Down
-- Back to 00005's definitions.
drop materialized view if exists consumption_yearly;
drop materialized view if exists consumption_monthly;
drop materialized view if exists consumption_daily;
drop materialized view if exists consumption_hourly;

create materialized view consumption_hourly
with (timescaledb.continuous, timescaledb.materialized_only = false) as
select
    analyzer_id,
    time_bucket('1 hour', ts) as bucket,
    first(active_import, ts)::numeric as active_import_start,
    last(active_import, ts)::numeric as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric as active_consumption,
    (last(reactive_inductive_import, ts) - first(reactive_inductive_import, ts))::numeric as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric as active_generation,
    (last(reactive_inductive_export, ts) - first(reactive_inductive_export, ts))::numeric as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric as t3_generation,
    max(max_demand_kw)::numeric as max_demand_kw,
    last(active_import, ts)::numeric as active_index,
    last(reactive_inductive_import, ts)::numeric as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric as t1_index,
    last(t2_import, ts)::numeric as t2_index,
    last(t3_import, ts)::numeric as t3_index,
    last(active_export, ts)::numeric as active_generation_index,
    count(*) as reading_count
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
    first(active_import, ts)::numeric as active_import_start,
    last(active_import, ts)::numeric as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric as active_consumption,
    (last(reactive_inductive_import, ts) - first(reactive_inductive_import, ts))::numeric as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric as active_generation,
    (last(reactive_inductive_export, ts) - first(reactive_inductive_export, ts))::numeric as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric as t3_generation,
    max(max_demand_kw)::numeric as max_demand_kw,
    last(active_import, ts)::numeric as active_index,
    last(reactive_inductive_import, ts)::numeric as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric as t1_index,
    last(t2_import, ts)::numeric as t2_index,
    last(t3_import, ts)::numeric as t3_index,
    last(active_export, ts)::numeric as active_generation_index,
    count(*) as reading_count
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
    first(active_import, ts)::numeric as active_import_start,
    last(active_import, ts)::numeric as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric as active_consumption,
    (last(reactive_inductive_import, ts) - first(reactive_inductive_import, ts))::numeric as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric as active_generation,
    (last(reactive_inductive_export, ts) - first(reactive_inductive_export, ts))::numeric as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric as t3_generation,
    max(max_demand_kw)::numeric as max_demand_kw,
    last(active_import, ts)::numeric as active_index,
    last(reactive_inductive_import, ts)::numeric as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric as t1_index,
    last(t2_import, ts)::numeric as t2_index,
    last(t3_import, ts)::numeric as t3_index,
    last(active_export, ts)::numeric as active_generation_index,
    count(*) as reading_count
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
    first(active_import, ts)::numeric as active_import_start,
    last(active_import, ts)::numeric as active_import_end,
    (last(active_import, ts) - first(active_import, ts))::numeric as active_consumption,
    (last(reactive_inductive_import, ts) - first(reactive_inductive_import, ts))::numeric as inductive_consumption,
    (last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts))::numeric as capacitive_consumption,
    (last(t1_import, ts) - first(t1_import, ts))::numeric as t1_consumption,
    (last(t2_import, ts) - first(t2_import, ts))::numeric as t2_consumption,
    (last(t3_import, ts) - first(t3_import, ts))::numeric as t3_consumption,
    (last(active_export, ts) - first(active_export, ts))::numeric as active_generation,
    (last(reactive_inductive_export, ts) - first(reactive_inductive_export, ts))::numeric as inductive_generation,
    (last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts))::numeric as capacitive_generation,
    (last(t1_export, ts) - first(t1_export, ts))::numeric as t1_generation,
    (last(t2_export, ts) - first(t2_export, ts))::numeric as t2_generation,
    (last(t3_export, ts) - first(t3_export, ts))::numeric as t3_generation,
    max(max_demand_kw)::numeric as max_demand_kw,
    last(active_import, ts)::numeric as active_index,
    last(reactive_inductive_import, ts)::numeric as inductive_index,
    last(reactive_capacitive_import, ts)::numeric as capacitive_index,
    last(t1_import, ts)::numeric as t1_index,
    last(t2_import, ts)::numeric as t2_index,
    last(t3_import, ts)::numeric as t3_index,
    last(active_export, ts)::numeric as active_generation_index,
    count(*) as reading_count
from meter_readings
where kind = 'load_profile'
group by analyzer_id, bucket;

select add_continuous_aggregate_policy('consumption_yearly',
    start_offset      => interval '5 years',
    end_offset        => interval '1 hour',
    schedule_interval => interval '1 day');

