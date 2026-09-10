-- +goose Up
-- Reports, transcribed from docs/rewrite/04-data-model.md section 8. `payload`
-- is the only place this table holds business data in jsonb: it is a
-- rendered snapshot of the report as issued, never queried by field, so any
-- figure needed for filtering or listing is duplicated into a column.

create type report_type   as enum ('monthly','yearly');
create type report_status as enum ('pending','completed','error');
create type plant_selection as enum ('all','grid','rooftop');

create table reports (
    id            uuid primary key default gen_random_uuid(),
    company_id    uuid not null references companies(id),
    building_id   uuid not null references buildings(id),
    type          report_type not null,
    period        text not null,                -- 'YYYY-MM' or 'YYYY'
    plant_selection plant_selection not null default 'all',
    payload       jsonb not null,               -- all computed figures for rendering
    pdf_path      text,
    excel_path    text,
    email_subject text,
    email_body    text,
    status        report_status not null default 'pending',
    error_message text,
    processed_at  timestamptz,
    created_at    timestamptz not null default now(),
    updated_at    timestamptz not null default now()
);
create unique index on reports (building_id, type, period);
create index on reports (company_id, created_at desc);

-- +goose Down
-- Reverse dependency order, and deliberately without `cascade`: a cascade
-- would mask a drop this migration forgot — the round-trip guard would still
-- see an empty schema — while silently destroying objects a later migration
-- owns. Enum types are dropped after the tables whose columns use them.
drop table if exists reports;
drop type if exists plant_selection;
drop type if exists report_status;
drop type if exists report_type;
