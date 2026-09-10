-- +goose Up
-- Buildings, plants and metering points. Transcribed from
-- docs/rewrite/04-data-model.md section 3, which is authoritative for column
-- names, types and constraints.

create table buildings (
    id               uuid primary key default gen_random_uuid(),
    company_id       uuid        not null references companies(id),
    name             text        not null,
    address          text,
    latitude         numeric(9,6),
    longitude        numeric(9,6),
    floors           integer,
    personnel_count  integer,
    total_area_m2    numeric(14,2),
    sector           text,
    responsible_user_id uuid references users(id),
    bill_cutoff_day  smallint    not null default 1 check (bill_cutoff_day between 1 and 31),
    created_at       timestamptz not null default now(),
    updated_at       timestamptz not null default now(),
    deleted_at       timestamptz
);
create index on buildings (company_id) where deleted_at is null;
create index on buildings (responsible_user_id) where deleted_at is null;
create index on buildings (sector) where deleted_at is null;

create table building_contacts (
    id          uuid primary key default gen_random_uuid(),
    building_id uuid not null references buildings(id) on delete cascade,
    name        text,
    phone       text,
    sort_order  smallint not null default 0
);

create type integration_provider as enum ('osos','gridbox','aril','pm5340','isolar');

create table analyzers (
    id                  uuid primary key default gen_random_uuid(),
    company_id          uuid        not null references companies(id),
    building_id         uuid        references buildings(id),
    provider            integration_provider not null,
    provider_subtype    text        not null,          -- 'Baskent', 'Aydem', 'Meramedas', ...
    installation_number text        not null,          -- tesisat no / wiring no / subscription id
    customer_name       text,
    address             text,
    province            text,                          -- il
    district            text,                          -- ilce
    neighbourhood       text,                          -- koy/mahalle
    street              text,                          -- cadde/sokak
    tariff_type         text,                          -- tarife tipi
    tariff_kind         text,                          -- tarife turu
    installation_kind   text,                          -- tesisat tur tanimi
    installed_power_kw  numeric(12,3),                 -- kurulu guc
    meter_number        text,
    meter_model         text,
    meter_multiplier    numeric(12,6) not null default 1,
    counterparty_no     text,                          -- muhatap no
    metering_point_name text,                          -- sayim nokta tanimi
    latitude            numeric(9,6),
    longitude           numeric(9,6),
    etso_code           text,
    definition_type     smallint,                      -- ARIL owner type
    last_reading_at     timestamptz,
    is_active           boolean     not null default false,
    created_at          timestamptz not null default now(),
    updated_at          timestamptz not null default now(),
    deleted_at          timestamptz
);
create unique index on analyzers (provider, provider_subtype, installation_number)
    where deleted_at is null;
create index on analyzers (company_id) where deleted_at is null;
create index on analyzers (building_id) where deleted_at is null;
create index on analyzers (last_reading_at desc);

create type panel_orientation as enum ('n','s','e','w','ne','se','nw','sw');

create table power_plants (
    id                     uuid primary key default gen_random_uuid(),
    company_id             uuid        not null references companies(id),
    name                   text        not null,
    installation_number    text,
    plant_kind             text        not null default 'rooftop',  -- 'rooftop' | 'grid'
    pv_brand_model         text,
    panel_power_w          numeric(10,2),
    panel_efficiency_pct   numeric(6,3),
    panel_count            integer,
    string_count           integer,
    orientation            panel_orientation,
    tilt_angle_deg         numeric(6,2),
    total_capacity_kw      numeric(12,3),
    yearly_target_kwh      numeric(16,3),
    installation_date      date,
    address                text,
    latitude               numeric(9,6),
    longitude              numeric(9,6),
    -- iSolarCloud link
    isolar_ps_id           text,
    isolar_ps_key          text,
    isolar_ps_name         text,
    isolar_installed_kw    numeric(12,3),
    isolar_linked_at       timestamptz,
    created_at             timestamptz not null default now(),
    updated_at             timestamptz not null default now(),
    deleted_at             timestamptz
);
create index on power_plants (company_id) where deleted_at is null;
create unique index on power_plants (isolar_ps_id) where isolar_ps_id is not null;

create table power_plant_monthly_targets (
    plant_id   uuid     not null references power_plants(id) on delete cascade,
    month      smallint not null check (month between 1 and 12),
    target_kwh numeric(16,3) not null default 0,
    primary key (plant_id, month)
);

create table power_plant_devices (
    id               uuid primary key default gen_random_uuid(),
    plant_id         uuid not null references power_plants(id) on delete cascade,
    device_sn        text not null,
    device_name      text,
    device_type      integer,
    device_type_name text,
    provider_key     text,                      -- iSolar ps_key
    brand            text,
    model            text,
    rated_power_kw   numeric(12,3),
    status           text,
    efficiency_pct   numeric(6,3),
    last_seen_at     timestamptz,
    created_at       timestamptz not null default now(),
    updated_at       timestamptz not null default now()
);
create unique index on power_plant_devices (plant_id, device_sn);

create table power_plant_alarm_recipients (
    plant_id uuid   not null references power_plants(id) on delete cascade,
    email    citext not null,
    primary key (plant_id, email)
);

-- +goose Down
-- Reverse dependency order, and deliberately without `cascade`: a cascade would
-- mask a drop this migration forgot — the round-trip guard would still see an
-- empty schema — while silently destroying objects a later migration owns.
-- Enum types are dropped after the tables whose columns use them.
drop table if exists power_plant_alarm_recipients;
drop table if exists power_plant_devices;
drop table if exists power_plant_monthly_targets;
drop table if exists power_plants;
drop type if exists panel_orientation;
drop table if exists analyzers;
drop type if exists integration_provider;
drop table if exists building_contacts;
drop table if exists buildings;
