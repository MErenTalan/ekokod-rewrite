-- +goose Up
-- R555: a building's saved bill window for one period key (the utility's
-- real reading dates); unsaved keys follow the cut-off day (R556).
create table bill_periods (
    company_id   uuid        not null references companies(id),
    building_id  uuid        not null references buildings(id),
    period_key   char(7)     not null,
    period_start timestamptz not null,
    period_end   timestamptz not null,
    created_by   uuid        references users(id),
    created_at   timestamptz not null default now(),
    updated_at   timestamptz not null default now(),
    primary key (building_id, period_key),
    check (period_start < period_end),
    check (period_end - period_start <= interval '62 days'),
    check (date_trunc('hour', period_start) = period_start and date_trunc('hour', period_end) = period_end)
);
create index on bill_periods (company_id);

-- R562: true = never deduct the first reactive violation for this tariff.
alter table tariffs add column reactive_waiver_disabled boolean not null default false;

-- +goose Down
alter table tariffs drop column if exists reactive_waiver_disabled;
drop table if exists bill_periods;
