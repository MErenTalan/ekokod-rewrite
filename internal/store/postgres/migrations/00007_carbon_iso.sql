-- +goose Up
-- Carbon accounting and ISO 50001 modules. Transcribed from
-- docs/rewrite/04-data-model.md sections 9 and 10, which are authoritative for
-- column names, types and constraints.

create type carbon_scope    as enum ('scope_1','scope_2','scope_3');
create type carbon_status   as enum ('pending','approved','rejected');

create table emission_factors (
    id             uuid primary key default gen_random_uuid(),
    company_id     uuid references companies(id),   -- null = platform master catalogue
    key            text not null,
    label          text not null,
    main_category  text not null,
    sub_categories text[] not null default '{}',
    category_path  text[] not null default '{}',
    base_factor    numeric(18,8) not null,
    base_unit      text not null,
    fuel_type      text,
    vehicle_type   text,
    scope          carbon_scope,
    iso_category   text,
    status         text,
    source         text,
    source_year    smallint,
    source_url     text,
    created_at     timestamptz not null default now(),
    updated_at     timestamptz not null default now()
);
create unique index on emission_factors (coalesce(company_id,'00000000-0000-0000-0000-000000000000'::uuid), key);

create table emission_factor_conversions (
    factor_id  uuid not null references emission_factors(id) on delete cascade,
    unit       text not null,
    multiplier numeric(18,8) not null,
    label      text not null,
    primary key (factor_id, unit)
);

create table carbon_selected_activities (
    company_id  uuid not null references companies(id),
    building_id uuid not null references buildings(id),
    activity_key text not null,
    primary key (building_id, activity_key)
);

create table carbon_activities (
    id             uuid primary key default gen_random_uuid(),
    company_id     uuid not null references companies(id),
    building_id    uuid not null references buildings(id),
    main_category  text not null,
    sub_category   text not null,
    activity_type  text not null,
    period_start   date not null,
    period_end     date not null,
    quantity       numeric(18,6) not null,
    unit           text not null,
    factor_id      uuid references emission_factors(id),
    factor_key     text,
    factor_value   numeric(18,8),
    conversion_multiplier numeric(18,8) not null default 1,
    emission_kgco2e numeric(18,6) not null,
    scope          carbon_scope not null,
    iso_category   text not null,
    description    text,
    details        jsonb,
    status         carbon_status not null default 'pending',
    is_automated   boolean not null default false,
    created_by     uuid references users(id),
    created_at     timestamptz not null default now(),
    updated_at     timestamptz not null default now()
);
create index on carbon_activities (building_id, period_start);
create index on carbon_activities (company_id, scope);
-- One automated record per building, activity type and day.
create unique index on carbon_activities (building_id, activity_type, period_start)
    where is_automated;

create table carbon_reports (
    id            uuid primary key default gen_random_uuid(),
    company_id    uuid not null references companies(id),
    building_id   uuid not null references buildings(id),
    name          text not null,
    report_type   text not null check (report_type in ('ghg','iso')),
    period        text not null,
    payload       jsonb not null,
    pdf_path      text,
    created_at    timestamptz not null default now()
);

create table iso50001_projects (
    id          uuid primary key default gen_random_uuid(),
    company_id  uuid not null references companies(id),
    building_id uuid not null references buildings(id),
    created_at  timestamptz not null default now(),
    updated_at  timestamptz not null default now()
);
create unique index on iso50001_projects (building_id);

create table iso50001_clause_dates (
    project_id uuid not null references iso50001_projects(id) on delete cascade,
    clause_id  text not null,           -- '5', '6', '7', '8', '9'
    start_date date,
    end_date   date,
    primary key (project_id, clause_id),
    constraint valid_range check (start_date is null or end_date is null or start_date <= end_date)
);

create table iso50001_notes (
    id         uuid primary key default gen_random_uuid(),
    project_id uuid not null references iso50001_projects(id) on delete cascade,
    clause_id  text not null,           -- '5.1', '6.3', ...
    title      text,
    body       text not null,
    created_by uuid references users(id),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);
create index on iso50001_notes (project_id, clause_id);

-- +goose Down
-- Reverse dependency order, and deliberately without `cascade`: a cascade would
-- mask a drop this migration forgot — the round-trip guard would still see an
-- empty schema — while silently destroying objects a later migration owns.
-- Enum types are dropped after the tables whose columns use them.
drop table if exists iso50001_notes;
drop table if exists iso50001_clause_dates;
drop table if exists iso50001_projects;
drop table if exists carbon_reports;
drop table if exists carbon_activities;
drop table if exists carbon_selected_activities;
drop table if exists emission_factor_conversions;
drop table if exists emission_factors;
drop type if exists carbon_status;
drop type if exists carbon_scope;
