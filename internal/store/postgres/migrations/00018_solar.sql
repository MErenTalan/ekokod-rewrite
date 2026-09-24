-- +goose NO TRANSACTION
-- Applied outside a transaction: TimescaleDB refuses to create a continuous
-- aggregate inside one (same as 00005).
-- +goose Up
-- F9 (R276): plant-level production gets its own table so device rows and
-- plant totals can never be summed together. The plant aggregates are rebuilt
-- over it; they were empty (nothing wrote plant_production before F9).
create table plant_production_totals (
    plant_id        uuid        not null references power_plants(id),
    ts              timestamptz not null,
    production_kwh  numeric(16,4),
    active_power_kw numeric(14,4),
    basis           text        not null check (basis in ('plant_meter','inverter_sum','daily_total')),
    updated_at      timestamptz not null default now(),
    primary key (plant_id, ts)
);
select create_hypertable('plant_production_totals','ts', chunk_time_interval => interval '30 days');
alter table plant_production_totals set (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'plant_id',
    timescaledb.compress_orderby   = 'ts desc'
);
select add_compression_policy('plant_production_totals', interval '90 days');

drop materialized view plant_production_monthly;
drop materialized view plant_production_daily;

create materialized view plant_production_daily
with (timescaledb.continuous, timescaledb.materialized_only = false) as
select
    plant_id,
    time_bucket('1 day', ts, 'Europe/Istanbul') as bucket,
    sum(production_kwh)::numeric  as production_kwh,
    max(active_power_kw)::numeric as max_active_power_kw
from plant_production_totals
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
    sum(production_kwh)::numeric  as production_kwh,
    max(active_power_kw)::numeric as max_active_power_kw
from plant_production_totals
group by plant_id, bucket;

select add_continuous_aggregate_policy('plant_production_monthly',
    start_offset      => interval '1 year',
    end_offset        => interval '1 hour',
    schedule_interval => interval '6 hours');

-- R280, R288, R289.
alter table power_plants
    add column isolar_credential_id   uuid references integration_credentials(id),
    add column netting_analyzer_id    uuid references analyzers(id),
    add column isolar_last_sync_at    timestamptz,
    add column isolar_last_sync_error text;

-- R282: the last realtime snapshot per device.
alter table power_plant_devices
    add column snapshot_at     timestamptz,
    add column fault_status    integer,
    add column active_power_kw numeric(14,4),
    add column yield_today_kwh numeric(16,4),
    add column yield_month_kwh numeric(16,4),
    add column yield_year_kwh  numeric(16,4),
    add column yield_total_kwh numeric(18,4);

-- R286: fetched iSolar faults, for the alarms tab and forwarding.
create table plant_faults (
    plant_id      uuid        not null references power_plants(id) on delete cascade,
    ref           text        not null,
    code          text        not null,
    name          text        not null,
    level         integer,
    type          integer,
    device_name   text,
    occurred_at   timestamptz not null,
    closed_at     timestamptz,
    first_seen_at timestamptz not null default now(),
    primary key (plant_id, ref)
);
create index on plant_faults (plant_id, occurred_at desc);

-- +goose Down
drop table if exists plant_faults;
alter table power_plant_devices
    drop column if exists yield_total_kwh,
    drop column if exists yield_year_kwh,
    drop column if exists yield_month_kwh,
    drop column if exists yield_today_kwh,
    drop column if exists active_power_kw,
    drop column if exists fault_status,
    drop column if exists snapshot_at;
alter table power_plants
    drop column if exists isolar_last_sync_error,
    drop column if exists isolar_last_sync_at,
    drop column if exists netting_analyzer_id,
    drop column if exists isolar_credential_id;

drop materialized view if exists plant_production_monthly;
drop materialized view if exists plant_production_daily;

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

drop table if exists plant_production_totals;
