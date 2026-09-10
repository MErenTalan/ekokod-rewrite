-- +goose Up
-- Tenancy, identity and access. Transcribed from docs/rewrite/04-data-model.md
-- section 2, which is authoritative for column names, types and constraints.
-- Every later migration's foreign keys point at companies and users.

create table companies (
    id                uuid primary key default gen_random_uuid(),
    name              text        not null,
    address           text,
    total_area_m2     numeric(14,2),
    personnel_count   integer,
    contact_name      text,
    contact_phone     text,
    sector            text,
    created_at        timestamptz not null default now(),
    updated_at        timestamptz not null default now(),
    deleted_at        timestamptz
);
create unique index on companies (lower(name)) where deleted_at is null;

create type user_role as enum (
    'admin',
    'company_admin',
    'company_readonly_admin',
    'building_admin',
    'building_readonly_admin',
    'demo'
);

create table users (
    id                  uuid primary key default gen_random_uuid(),
    company_id          uuid        not null references companies(id),
    name                text        not null,
    email               citext      not null,
    phone               text,
    password_hash       text        not null,
    password_changed_at timestamptz,
    role                user_role   not null,
    is_active           boolean     not null default true,
    last_login_at       timestamptz,
    created_at          timestamptz not null default now(),
    updated_at          timestamptz not null default now(),
    deleted_at          timestamptz
);
create unique index on users (email) where deleted_at is null;
create index on users (company_id);

-- Blocks reuse of recent passwords.
create table user_password_history (
    id            uuid primary key default gen_random_uuid(),
    user_id       uuid        not null references users(id) on delete cascade,
    password_hash text        not null,
    created_at    timestamptz not null default now()
);
create index on user_password_history (user_id, created_at desc);

create table sessions (
    id                 uuid primary key default gen_random_uuid(),
    user_id            uuid        not null references users(id) on delete cascade,
    refresh_token_hash text        not null,
    device_fingerprint text,
    user_agent         text,
    ip                 inet,
    expires_at         timestamptz not null,
    revoked_at         timestamptz,
    created_at         timestamptz not null default now()
);
create index on sessions (user_id);
create unique index on sessions (refresh_token_hash);

create table audit_log (
    id          bigserial primary key,
    company_id  uuid,
    user_id     uuid,
    action      text        not null,   -- 'building.update', 'tariff.create', ...
    entity_type text        not null,
    entity_id   uuid,
    before      jsonb,
    after       jsonb,
    ip          inet,
    created_at  timestamptz not null default now()
);
create index on audit_log (company_id, created_at desc);
create index on audit_log (entity_type, entity_id, created_at desc);

-- +goose Down
-- Reverse dependency order, and deliberately without `cascade`: a cascade would
-- mask a drop this migration forgot — the round-trip guard would still see an
-- empty schema — while silently destroying objects a later migration owns.
drop table if exists audit_log;
drop table if exists sessions;
drop table if exists user_password_history;
drop table if exists users;
drop type if exists user_role;
drop table if exists companies;
