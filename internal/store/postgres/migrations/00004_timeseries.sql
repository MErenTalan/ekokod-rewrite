-- +goose Up
-- Time-series core. Transcribed from docs/rewrite/04-data-model.md sections
-- 4.1, 4.2, 4.4, 4.5, 4.6 and 12, which are authoritative for column names,
-- types and constraints. Everything energy-related in the product rests on
-- meter_readings, so the partitioning decisions below are load-bearing rather
-- than cosmetic: chunk_time_interval and the space partition decide whether a
-- one-month single-analyzer query reads four chunks or the whole table.
--
-- Ordering within this migration matters. A table must exist before
-- create_hypertable converts it, compression settings may only be set on a
-- hypertable, and a compression policy may only be added once compression is
-- configured.

-- 4.1 Meter readings ---------------------------------------------------------

create type reading_kind as enum ('load_profile','daily','billing','reset','current_index');

create table meter_readings (
    analyzer_id                uuid          not null references analyzers(id),
    ts                         timestamptz   not null,
    kind                       reading_kind  not null,

    -- import registers (consumption), already multiplied by meter_multiplier
    active_import              numeric(18,4),
    reactive_inductive_import  numeric(18,4),
    reactive_capacitive_import numeric(18,4),
    t1_import                  numeric(18,4),
    t2_import                  numeric(18,4),
    t3_import                  numeric(18,4),

    -- export registers (generation)
    active_export              numeric(18,4),
    reactive_inductive_export  numeric(18,4),
    reactive_capacitive_export numeric(18,4),
    t1_export                  numeric(18,4),
    t2_export                  numeric(18,4),
    t3_export                  numeric(18,4),

    -- interval, not cumulative
    max_demand_kw              numeric(14,4),

    meter_serial               text,
    multiplier_applied         numeric(12,6) not null default 1,
    source_provider            integration_provider not null,
    ingested_at                timestamptz   not null default now(),
    raw                        jsonb,          -- forensic only; no business logic reads this

    primary key (analyzer_id, ts, kind)
);

select create_hypertable('meter_readings', 'ts',
                         partitioning_column => 'analyzer_id',
                         number_partitions   => 16,
                         chunk_time_interval => interval '7 days');

create index on meter_readings (analyzer_id, kind, ts desc);

alter table meter_readings set (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'analyzer_id, kind',
    timescaledb.compress_orderby   = 'ts desc'
);
select add_compression_policy('meter_readings', interval '90 days');

-- 4.2 Ingestion high-water marks --------------------------------------------

create table ingestion_cursors (
    analyzer_id      uuid         not null references analyzers(id),
    kind             reading_kind not null,
    last_ts          timestamptz,
    last_success_at  timestamptz,
    last_error       text,
    last_error_at    timestamptz,
    consecutive_failures integer not null default 0,
    primary key (analyzer_id, kind)
);

-- 4.4 Suspect periods --------------------------------------------------------

create table consumption_anomalies (
    id           uuid primary key default gen_random_uuid(),
    analyzer_id  uuid        not null references analyzers(id),
    period_start timestamptz not null,
    period_end   timestamptz not null,
    reason       text        not null,   -- 'meter_reset','negative_delta','missing_readings'
    detail       jsonb,
    resolved_at  timestamptz,
    resolved_by  uuid references users(id),
    resolution   text,                   -- 'reset_registered','manual_override','accepted'
    override_values jsonb,
    created_at   timestamptz not null default now()
);
create index on consumption_anomalies (analyzer_id, period_start)
    where resolved_at is null;

-- 4.5 Solar production -------------------------------------------------------

create table plant_production (
    plant_id         uuid        not null references power_plants(id),
    ts               timestamptz not null,
    device_id        uuid        references power_plant_devices(id),
    production_kwh   numeric(16,4),
    active_power_kw  numeric(14,4),
    efficiency_pct   numeric(6,3),
    irradiance_wm2   numeric(10,3),
    module_temp_c    numeric(6,2),
    ambient_temp_c   numeric(6,2),
    source           text not null default 'isolar',
    primary key (plant_id, ts, device_id)
);
select create_hypertable('plant_production','ts', chunk_time_interval => interval '30 days');

-- The spec's 4.5 block adds the compression policy without first enabling
-- compression, which TimescaleDB 2.30 rejects with "columnstore not enabled on
-- hypertable". The settings below are the minimum needed to make the spec's own
-- add_compression_policy call executable, and mirror 4.1's choice: segment by
-- the identity columns that every query filters on, order by descending time.
alter table plant_production set (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'plant_id, device_id',
    timescaledb.compress_orderby   = 'ts desc'
);
select add_compression_policy('plant_production', interval '90 days');

-- 4.6 Market prices ----------------------------------------------------------

create table market_prices_hourly (
    ts        timestamptz not null,     -- hour start, Europe/Istanbul semantics, stored UTC
    ptf       numeric(14,4) not null,   -- TL/MWh
    fetched_at timestamptz not null default now(),
    primary key (ts)
);
select create_hypertable('market_prices_hourly','ts', chunk_time_interval => interval '90 days');

create table yekdem_monthly (
    year       smallint not null,
    month      smallint not null check (month between 1 and 12),
    value      numeric(14,4) not null,  -- TL/MWh
    fetched_at timestamptz not null default now(),
    primary key (year, month)
);

-- 12. Forecasts --------------------------------------------------------------

create table forecasts (
    analyzer_id  uuid not null references analyzers(id),
    ts           timestamptz not null,
    generated_at timestamptz not null,
    horizon_hours integer not null,
    median       numeric(18,4) not null,
    p10          numeric(18,4),
    p90          numeric(18,4),
    model_id     text not null,
    model_version text,
    primary key (analyzer_id, ts, generated_at)
);
select create_hypertable('forecasts','ts', chunk_time_interval => interval '30 days');

create table forecast_gaps (
    id          uuid primary key default gen_random_uuid(),
    analyzer_id uuid not null references analyzers(id),
    generated_at timestamptz not null,
    gap_start   timestamptz not null,
    gap_end     timestamptz not null,
    missing_hours integer not null
);

-- +goose Down
-- Reverse dependency order, and deliberately without `cascade`: a cascade would
-- mask a drop this migration forgot — the round-trip guard would still see an
-- empty schema — while silently destroying objects a later migration owns.
-- Dropping a hypertable takes its chunks, its compression settings and its
-- background policy jobs with it, so there is nothing to unregister by hand.
-- The reading_kind enum is dropped after every table whose columns use it.
drop table if exists forecast_gaps;
drop table if exists forecasts;
drop table if exists yekdem_monthly;
drop table if exists market_prices_hourly;
drop table if exists plant_production;
drop table if exists consumption_anomalies;
drop table if exists ingestion_cursors;
drop table if exists meter_readings;
drop type if exists reading_kind;
