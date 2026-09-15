-- +goose Up
-- provider_hourly_values is OSOS's own labelled hourly cross-check series
-- (06 §2, removed-behaviour 23): a second, provider-shaped copy of hourly
-- consumption/generation figures kept ONLY so an operator can compare it
-- against meter_readings' own hourly figures. NEVER read by consumption or
-- billing — see repository.go's ProviderSeriesRepository doc.
--
-- No company_id: like meter_readings, every read and write joins through
-- analyzers for the tenant predicate (repository.go's "ROWS WITHOUT
-- company_id" header).
create table provider_hourly_values (
    analyzer_id        uuid        not null references analyzers(id),
    ts                 timestamptz not null,
    active_consumption numeric(18,4),
    active_generation  numeric(18,4),
    source_provider    integration_provider not null,
    ingested_at        timestamptz not null default now(),
    primary key (analyzer_id, ts)
);
select create_hypertable('provider_hourly_values', 'ts', chunk_time_interval => interval '30 days');

-- +goose Down
drop table if exists provider_hourly_values;
