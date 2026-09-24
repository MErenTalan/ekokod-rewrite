-- +goose Up
-- File storage, integration credentials, calendar and operations tables.
-- Transcribed from docs/rewrite/04-data-model.md section 11, which is
-- authoritative for column names, types and constraints.

create table stored_files (
    id           uuid primary key default gen_random_uuid(),
    company_id   uuid not null references companies(id),
    owner_type   text not null,   -- 'iso50001','carbon','report','bill','plant'
    owner_id     uuid,
    clause_id    text,
    original_name text not null,
    stored_path  text not null,
    content_type text not null,
    size_bytes   bigint not null,
    checksum     text not null,
    uploaded_by  uuid references users(id),
    created_at   timestamptz not null default now(),
    deleted_at   timestamptz
);
create index on stored_files (owner_type, owner_id) where deleted_at is null;

-- Provider catalogue: which endpoints each provider/subtype exposes. Admin-managed.
create table integration_definitions (
    id         uuid primary key default gen_random_uuid(),
    provider   integration_provider not null,
    subtype    text not null,
    endpoints  jsonb not null default '{}',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);
create unique index on integration_definitions (provider, subtype);

-- Per-company credentials. Secrets encrypted; never returned by the API.
create table integration_credentials (
    id            uuid primary key default gen_random_uuid(),
    company_id    uuid not null references companies(id),
    definition_id uuid not null references integration_definitions(id),
    username      text,
    secret_enc    bytea,           -- AES-256-GCM
    extra_enc     bytea,           -- provider-specific secrets (iSolar keys, tokens)
    settings      jsonb not null default '{}',   -- non-secret options, e.g. use_billing_indexes
    pm5340_url    text,
    isolar_region text,
    token_expires_at timestamptz,
    last_verified_at timestamptz,
    is_active     boolean not null default true,
    created_at    timestamptz not null default now(),
    updated_at    timestamptz not null default now()
);
create unique index on integration_credentials (company_id, definition_id);

create table smtp_settings (
    company_id uuid primary key references companies(id) on delete cascade,
    host       text not null,
    port       integer not null,
    secure     boolean not null default false,
    username   text not null,
    password_enc bytea not null,
    from_address citext not null,
    updated_at timestamptz not null default now()
);

create table calendar_events (
    id          uuid primary key default gen_random_uuid(),
    company_id  uuid not null references companies(id),
    title       text not null,
    starts_at   timestamptz not null,
    ends_at     timestamptz not null,
    all_day     boolean not null default false,
    colour      text,
    created_by  uuid references users(id),
    created_at  timestamptz not null default now()
);
create index on calendar_events (company_id, starts_at);

create table company_weekend_days (
    company_id uuid not null references companies(id) on delete cascade,
    day_of_week smallint not null check (day_of_week between 0 and 6),  -- 0 = Sunday
    primary key (company_id, day_of_week)
);

create table company_vacations (
    id          uuid primary key default gen_random_uuid(),
    company_id  uuid not null references companies(id) on delete cascade,
    start_date  date not null,
    end_date    date not null,
    description text,
    constraint valid_range check (start_date <= end_date)
);
create index on company_vacations (company_id, start_date);

-- Job run history. Backs the Messages screen and operator diagnostics.
create table job_runs (
    id           uuid primary key default gen_random_uuid(),
    company_id   uuid,
    job_type     text not null,
    scope        jsonb not null default '{}',
    started_at   timestamptz not null default now(),
    finished_at  timestamptz,
    status       text not null default 'running',  -- running|success|partial|failed
    processed    integer not null default 0,
    skipped      integer not null default 0,
    failed       integer not null default 0,
    error        text,
    detail       jsonb
);
create index on job_runs (job_type, started_at desc);
create index on job_runs (company_id, started_at desc);

-- The Messages screen.
create table operational_messages (
    id           bigserial primary key,
    company_id   uuid,
    kind         text not null,     -- 'alarm' | 'job' | 'system'
    category     text not null,     -- 'bill-generation', 'analyzer-refresh', ...
    status       text not null,     -- 'success' | 'error' | 'warning' | 'info'
    message      text not null,
    detail       text,
    related_type text,
    related_id   uuid,
    metadata     jsonb,
    created_at   timestamptz not null default now()
);
create index on operational_messages (company_id, created_at desc);
create index on operational_messages (kind, created_at desc);
create index on operational_messages (status, created_at desc);

-- +goose Down
-- Reverse dependency order, and deliberately without `cascade`: a cascade
-- would mask a drop this migration forgot while silently destroying objects a
-- later migration owns.
drop table if exists operational_messages;
drop table if exists job_runs;
drop table if exists company_vacations;
drop table if exists company_weekend_days;
drop table if exists calendar_events;
drop table if exists smtp_settings;
drop table if exists integration_credentials;
drop table if exists integration_definitions;
drop table if exists stored_files;
