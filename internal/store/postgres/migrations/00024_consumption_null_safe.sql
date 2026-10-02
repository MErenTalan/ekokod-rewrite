-- +goose NO TRANSACTION
-- Not transactional for the same reason as 00005: creating a continuous
-- aggregate refreshes it, and a refresh commits its own work.
-- +goose Up
-- R498: first()/last() return the edge ROW's value, NULL included, so one
-- reading without a register (a PM5340 row carrying only generation) blanked
-- the whole bucket. Every first/last now skips NULLs. R499: months and years
-- are rolled up from consumption_daily at query time, so their aggregates go.
-- Every policy here starts ten minutes out, so no refresh job is in flight
-- should a neighbouring migration drop its view straight away (see below).
-- A view must not be dropped under its own refresh job: TimescaleDB 2.30
-- crashed the server (signal 11 in the policy worker) when 00021's freshly
-- created views were dropped while their first policy run was still going.
-- Unschedule the jobs and wait for a running one to finish first.
-- +goose StatementBegin
do $$
declare
    running int;
begin
    perform alter_job(job_id, scheduled => false)
       from timescaledb_information.jobs
      where proc_name = 'policy_refresh_continuous_aggregate'
        and hypertable_name like 'consumption\_%';
    perform pg_sleep(0.5);
    for i in 1..1200 loop
        select count(*) into running
          from timescaledb_information.jobs j
          join timescaledb_information.job_stats s using (job_id)
         where j.proc_name = 'policy_refresh_continuous_aggregate'
           and j.hypertable_name like 'consumption\_%'
           and s.job_status = 'Running';
        exit when running = 0;
        perform pg_sleep(0.1);
    end loop;
end $$;
-- +goose StatementEnd
drop materialized view if exists consumption_yearly;
drop materialized view if exists consumption_monthly;
drop materialized view if exists consumption_daily;
drop materialized view if exists consumption_hourly;

create materialized view consumption_hourly
with (timescaledb.continuous, timescaledb.materialized_only = false) as
select
    analyzer_id,
    time_bucket('1 hour', ts) as bucket,
    first(active_import, ts) filter (where active_import is not null)::numeric as active_import_start,
    last(active_import, ts) filter (where active_import is not null)::numeric as active_import_end,
    (last(active_import, ts) filter (where active_import is not null) - first(active_import, ts) filter (where active_import is not null))::numeric as active_consumption,
    (last(reactive_inductive_import, ts) filter (where reactive_inductive_import is not null) - first(reactive_inductive_import, ts) filter (where reactive_inductive_import is not null))::numeric as inductive_consumption,
    (last(reactive_capacitive_import, ts) filter (where reactive_capacitive_import is not null) - first(reactive_capacitive_import, ts) filter (where reactive_capacitive_import is not null))::numeric as capacitive_consumption,
    (last(t1_import, ts) filter (where t1_import is not null) - first(t1_import, ts) filter (where t1_import is not null))::numeric as t1_consumption,
    (last(t2_import, ts) filter (where t2_import is not null) - first(t2_import, ts) filter (where t2_import is not null))::numeric as t2_consumption,
    (last(t3_import, ts) filter (where t3_import is not null) - first(t3_import, ts) filter (where t3_import is not null))::numeric as t3_consumption,
    (last(active_export, ts) filter (where active_export is not null) - first(active_export, ts) filter (where active_export is not null))::numeric as active_generation,
    (last(reactive_inductive_export, ts) filter (where reactive_inductive_export is not null) - first(reactive_inductive_export, ts) filter (where reactive_inductive_export is not null))::numeric as inductive_generation,
    (last(reactive_capacitive_export, ts) filter (where reactive_capacitive_export is not null) - first(reactive_capacitive_export, ts) filter (where reactive_capacitive_export is not null))::numeric as capacitive_generation,
    (last(t1_export, ts) filter (where t1_export is not null) - first(t1_export, ts) filter (where t1_export is not null))::numeric as t1_generation,
    (last(t2_export, ts) filter (where t2_export is not null) - first(t2_export, ts) filter (where t2_export is not null))::numeric as t2_generation,
    (last(t3_export, ts) filter (where t3_export is not null) - first(t3_export, ts) filter (where t3_export is not null))::numeric as t3_generation,
    max(max_demand_kw)::numeric as max_demand_kw,
    last(active_import, ts) filter (where active_import is not null)::numeric as active_index,
    last(reactive_inductive_import, ts) filter (where reactive_inductive_import is not null)::numeric as inductive_index,
    last(reactive_capacitive_import, ts) filter (where reactive_capacitive_import is not null)::numeric as capacitive_index,
    last(t1_import, ts) filter (where t1_import is not null)::numeric as t1_index,
    last(t2_import, ts) filter (where t2_import is not null)::numeric as t2_index,
    last(t3_import, ts) filter (where t3_import is not null)::numeric as t3_index,
    last(active_export, ts) filter (where active_export is not null)::numeric as active_generation_index,
    count(*) as reading_count,
    min(ts)::timestamptz as first_ts,
    first(reactive_inductive_import, ts) filter (where reactive_inductive_import is not null)::numeric as inductive_consumption_start,
    first(reactive_capacitive_import, ts) filter (where reactive_capacitive_import is not null)::numeric as capacitive_consumption_start,
    first(t1_import, ts) filter (where t1_import is not null)::numeric as t1_consumption_start,
    first(t2_import, ts) filter (where t2_import is not null)::numeric as t2_consumption_start,
    first(t3_import, ts) filter (where t3_import is not null)::numeric as t3_consumption_start,
    first(active_export, ts) filter (where active_export is not null)::numeric as active_generation_start,
    first(reactive_inductive_export, ts) filter (where reactive_inductive_export is not null)::numeric as inductive_generation_start,
    first(reactive_capacitive_export, ts) filter (where reactive_capacitive_export is not null)::numeric as capacitive_generation_start,
    first(t1_export, ts) filter (where t1_export is not null)::numeric as t1_generation_start,
    first(t2_export, ts) filter (where t2_export is not null)::numeric as t2_generation_start,
    first(t3_export, ts) filter (where t3_export is not null)::numeric as t3_generation_start,
    last(reactive_inductive_export, ts) filter (where reactive_inductive_export is not null)::numeric as inductive_generation_index,
    last(reactive_capacitive_export, ts) filter (where reactive_capacitive_export is not null)::numeric as capacitive_generation_index,
    last(t1_export, ts) filter (where t1_export is not null)::numeric as t1_generation_index,
    last(t2_export, ts) filter (where t2_export is not null)::numeric as t2_generation_index,
    last(t3_export, ts) filter (where t3_export is not null)::numeric as t3_generation_index
from meter_readings
where kind = 'load_profile'
group by analyzer_id, bucket;

select add_continuous_aggregate_policy('consumption_hourly',
    start_offset      => interval '30 days',
    end_offset        => interval '1 hour',
    schedule_interval => interval '30 minutes',
    initial_start     => now() + interval '10 minutes');

create materialized view consumption_daily
with (timescaledb.continuous, timescaledb.materialized_only = false) as
select
    analyzer_id,
    time_bucket('1 day', ts, 'Europe/Istanbul') as bucket,
    first(active_import, ts) filter (where active_import is not null)::numeric as active_import_start,
    last(active_import, ts) filter (where active_import is not null)::numeric as active_import_end,
    (last(active_import, ts) filter (where active_import is not null) - first(active_import, ts) filter (where active_import is not null))::numeric as active_consumption,
    (last(reactive_inductive_import, ts) filter (where reactive_inductive_import is not null) - first(reactive_inductive_import, ts) filter (where reactive_inductive_import is not null))::numeric as inductive_consumption,
    (last(reactive_capacitive_import, ts) filter (where reactive_capacitive_import is not null) - first(reactive_capacitive_import, ts) filter (where reactive_capacitive_import is not null))::numeric as capacitive_consumption,
    (last(t1_import, ts) filter (where t1_import is not null) - first(t1_import, ts) filter (where t1_import is not null))::numeric as t1_consumption,
    (last(t2_import, ts) filter (where t2_import is not null) - first(t2_import, ts) filter (where t2_import is not null))::numeric as t2_consumption,
    (last(t3_import, ts) filter (where t3_import is not null) - first(t3_import, ts) filter (where t3_import is not null))::numeric as t3_consumption,
    (last(active_export, ts) filter (where active_export is not null) - first(active_export, ts) filter (where active_export is not null))::numeric as active_generation,
    (last(reactive_inductive_export, ts) filter (where reactive_inductive_export is not null) - first(reactive_inductive_export, ts) filter (where reactive_inductive_export is not null))::numeric as inductive_generation,
    (last(reactive_capacitive_export, ts) filter (where reactive_capacitive_export is not null) - first(reactive_capacitive_export, ts) filter (where reactive_capacitive_export is not null))::numeric as capacitive_generation,
    (last(t1_export, ts) filter (where t1_export is not null) - first(t1_export, ts) filter (where t1_export is not null))::numeric as t1_generation,
    (last(t2_export, ts) filter (where t2_export is not null) - first(t2_export, ts) filter (where t2_export is not null))::numeric as t2_generation,
    (last(t3_export, ts) filter (where t3_export is not null) - first(t3_export, ts) filter (where t3_export is not null))::numeric as t3_generation,
    max(max_demand_kw)::numeric as max_demand_kw,
    last(active_import, ts) filter (where active_import is not null)::numeric as active_index,
    last(reactive_inductive_import, ts) filter (where reactive_inductive_import is not null)::numeric as inductive_index,
    last(reactive_capacitive_import, ts) filter (where reactive_capacitive_import is not null)::numeric as capacitive_index,
    last(t1_import, ts) filter (where t1_import is not null)::numeric as t1_index,
    last(t2_import, ts) filter (where t2_import is not null)::numeric as t2_index,
    last(t3_import, ts) filter (where t3_import is not null)::numeric as t3_index,
    last(active_export, ts) filter (where active_export is not null)::numeric as active_generation_index,
    count(*) as reading_count,
    min(ts)::timestamptz as first_ts,
    first(reactive_inductive_import, ts) filter (where reactive_inductive_import is not null)::numeric as inductive_consumption_start,
    first(reactive_capacitive_import, ts) filter (where reactive_capacitive_import is not null)::numeric as capacitive_consumption_start,
    first(t1_import, ts) filter (where t1_import is not null)::numeric as t1_consumption_start,
    first(t2_import, ts) filter (where t2_import is not null)::numeric as t2_consumption_start,
    first(t3_import, ts) filter (where t3_import is not null)::numeric as t3_consumption_start,
    first(active_export, ts) filter (where active_export is not null)::numeric as active_generation_start,
    first(reactive_inductive_export, ts) filter (where reactive_inductive_export is not null)::numeric as inductive_generation_start,
    first(reactive_capacitive_export, ts) filter (where reactive_capacitive_export is not null)::numeric as capacitive_generation_start,
    first(t1_export, ts) filter (where t1_export is not null)::numeric as t1_generation_start,
    first(t2_export, ts) filter (where t2_export is not null)::numeric as t2_generation_start,
    first(t3_export, ts) filter (where t3_export is not null)::numeric as t3_generation_start,
    last(reactive_inductive_export, ts) filter (where reactive_inductive_export is not null)::numeric as inductive_generation_index,
    last(reactive_capacitive_export, ts) filter (where reactive_capacitive_export is not null)::numeric as capacitive_generation_index,
    last(t1_export, ts) filter (where t1_export is not null)::numeric as t1_generation_index,
    last(t2_export, ts) filter (where t2_export is not null)::numeric as t2_generation_index,
    last(t3_export, ts) filter (where t3_export is not null)::numeric as t3_generation_index
from meter_readings
where kind = 'load_profile'
group by analyzer_id, bucket;

select add_continuous_aggregate_policy('consumption_daily',
    start_offset      => interval '90 days',
    end_offset        => interval '1 hour',
    schedule_interval => interval '1 hour',
    initial_start     => now() + interval '10 minutes');

-- +goose Down
-- A view must not be dropped under its own refresh job: TimescaleDB 2.30
-- crashed the server (signal 11 in the policy worker) when 00021's freshly
-- created views were dropped while their first policy run was still going.
-- Unschedule the jobs and wait for a running one to finish first.
-- +goose StatementBegin
do $$
declare
    running int;
begin
    perform alter_job(job_id, scheduled => false)
       from timescaledb_information.jobs
      where proc_name = 'policy_refresh_continuous_aggregate'
        and hypertable_name like 'consumption\_%';
    perform pg_sleep(0.5);
    for i in 1..1200 loop
        select count(*) into running
          from timescaledb_information.jobs j
          join timescaledb_information.job_stats s using (job_id)
         where j.proc_name = 'policy_refresh_continuous_aggregate'
           and j.hypertable_name like 'consumption\_%'
           and s.job_status = 'Running';
        exit when running = 0;
        perform pg_sleep(0.1);
    end loop;
end $$;
-- +goose StatementEnd
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
    schedule_interval => interval '30 minutes',
    initial_start     => now() + interval '10 minutes');

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
    schedule_interval => interval '1 hour',
    initial_start     => now() + interval '10 minutes');

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
    schedule_interval => interval '6 hours',
    initial_start     => now() + interval '10 minutes');

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
    schedule_interval => interval '1 day',
    initial_start     => now() + interval '10 minutes');
