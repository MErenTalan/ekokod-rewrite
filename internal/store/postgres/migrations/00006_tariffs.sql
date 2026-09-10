-- +goose Up
-- Tariffs, templates and icmal imports. Transcribed from
-- docs/rewrite/04-data-model.md section 5, including the "İcmal imports"
-- subsection, which is authoritative for column names, types and
-- constraints.

create type price_type    as enum ('single_time','multi_time');
create type tariff_term   as enum ('monomial','binomial');
create type energy_type   as enum ('grid_energy','green_energy');
create type voltage_level as enum ('lv','mv');                 -- ag / og
create type supply_company as enum ('incumbent','private');    -- gorevli / serbest
create type generation_usage as enum ('none','subtract_from_consumption','subtract_from_total');
create type currency_code as enum ('TRY','USD','EUR');

create type distribution_user_group as enum (
    'residential','residential_plus','commercial','commercial_plus',
    'industrial','agricultural','lighting','martyrs_families','public_lighting'
);

create table tariffs (
    id                    uuid primary key default gen_random_uuid(),
    company_id            uuid not null references companies(id),
    building_id           uuid references buildings(id),
    name                  text,
    effective_from        date not null,
    currency              currency_code not null default 'TRY',
    energy_type           energy_type   not null default 'grid_energy',
    voltage_level         voltage_level not null,
    user_group            distribution_user_group not null,
    price_type            price_type    not null,
    term                  tariff_term   not null,
    supply_company        supply_company not null,

    -- fixed prices (TL/kWh unless noted)
    single_time_price     numeric(18,6),
    t1_price              numeric(18,6),
    t2_price              numeric(18,6),
    t3_price              numeric(18,6),
    overuse_price         numeric(18,6),
    overuse_threshold_kwh_per_day numeric(12,3),
    distribution_cost     numeric(18,6) not null,
    reactive_power_price  numeric(18,6) not null,
    green_energy_price    numeric(18,6),
    green_energy_distribution_cost numeric(18,6),
    contracted_power_kw   numeric(12,3),
    power_unit_price      numeric(18,6),          -- TL/kW/month

    -- generation handling
    generation_usage      generation_usage not null default 'none',
    generation_price_per_kwh numeric(18,6),

    -- taxes
    vat_rate              numeric(6,3) not null,  -- REQUIRED, no default

    -- dynamic pricing
    use_ptf_yekdem        boolean not null default false,
    kbk_energy            numeric(12,4),
    kbk_t1                numeric(12,4),
    kbk_t2                numeric(12,4),
    kbk_t3                numeric(12,4),
    kbk_power_price       numeric(12,4),
    kbk_overuse_price     numeric(12,4),
    kbk_reactive_power    numeric(12,4),
    kbk_distribution_cost_tl_per_kwh numeric(18,6),
    use_manual_yekdem     boolean not null default false,

    created_by            uuid references users(id),
    created_at            timestamptz not null default now(),
    updated_at            timestamptz not null default now(),
    deleted_at            timestamptz,

    constraint single_time_needs_price
        check (price_type <> 'single_time' or single_time_price is not null),
    constraint multi_time_needs_prices
        check (price_type <> 'multi_time'
               or (t1_price is not null and t2_price is not null and t3_price is not null)),
    constraint subtract_from_total_needs_price
        check (generation_usage <> 'subtract_from_total'
               or generation_price_per_kwh is not null),
    constraint ptf_needs_energy_kbk
        check (not use_ptf_yekdem or kbk_energy is not null)
);
create index on tariffs (building_id, effective_from desc) where deleted_at is null;
create index on tariffs (company_id) where deleted_at is null;

create table tariff_taxes (
    id         uuid primary key default gen_random_uuid(),
    tariff_id  uuid not null references tariffs(id) on delete cascade,
    name       text not null,          -- 'BTV', 'Enerji Fonu', 'TRT Payı'
    rate       numeric(6,3) not null,  -- percent, applied to the energy charge
    sort_order smallint not null default 0
);

create table tariff_manual_yekdem (
    tariff_id uuid     not null references tariffs(id) on delete cascade,
    year      smallint not null,
    month     smallint not null check (month between 1 and 12),
    value     numeric(14,4) not null,  -- TL/MWh
    primary key (tariff_id, year, month)
);

create table tariff_templates (
    id          uuid primary key default gen_random_uuid(),
    company_id  uuid not null references companies(id),
    name        text not null,
    description text,
    is_default  boolean not null default false,
    payload     jsonb not null,   -- a full tariff definition, applied by copying into `tariffs`
    created_at  timestamptz not null default now(),
    updated_at  timestamptz not null default now()
);
create unique index on tariff_templates (company_id, lower(name));

create table solar_tariffs (
    id             uuid primary key default gen_random_uuid(),
    company_id     uuid not null references companies(id),
    plant_id       uuid not null references power_plants(id),
    effective_from date not null,
    feed_in_tariff numeric(18,6) not null,   -- TL/kWh
    purchase_price numeric(18,6),
    currency       currency_code not null default 'TRY',
    notes          text,
    created_at     timestamptz not null default now(),
    deleted_at     timestamptz
);
create index on solar_tariffs (plant_id, effective_from desc) where deleted_at is null;

-- The published national tariff schedule that backs the public bill calculator.
create table national_tariff_schedule (
    id              uuid primary key default gen_random_uuid(),
    effective_from  date not null,
    user_group      distribution_user_group not null,
    voltage_level   voltage_level not null,
    term            tariff_term not null,
    energy_price    numeric(18,6) not null,
    t1_price        numeric(18,6),
    t2_price        numeric(18,6),
    t3_price        numeric(18,6),
    distribution_price numeric(18,6) not null,
    power_price     numeric(18,6),
    overuse_price   numeric(18,6),
    daily_threshold_kwh numeric(12,3),
    vat_rate        numeric(6,3) not null,
    source          text,
    created_at      timestamptz not null default now()
);
create unique index on national_tariff_schedule
    (effective_from, user_group, voltage_level, term);

create table icmal_imports (
    id            uuid primary key default gen_random_uuid(),
    company_id    uuid not null references companies(id),
    uploaded_by   uuid references users(id),
    file_name     text not null,
    row_count     integer not null default 0,
    status        text not null default 'pending',  -- pending|analysed|applied|rejected
    result        jsonb,        -- derived coefficients, stats, warnings
    created_at    timestamptz not null default now()
);

create table icmal_rows (
    id            uuid primary key default gen_random_uuid(),
    import_id     uuid not null references icmal_imports(id) on delete cascade,
    building_id   uuid references buildings(id),
    period        char(6) not null,               -- YYYYMM
    etso_code     text,
    total_kwh     numeric(18,4),
    t0_kwh        numeric(18,4),
    t1_kwh        numeric(18,4),
    t2_kwh        numeric(18,4),
    t3_kwh        numeric(18,4),
    energy_charge numeric(18,4),
    distribution_charge numeric(18,4),
    reactive_charge numeric(18,4),
    power_charge  numeric(18,4),
    overuse_charge numeric(18,4),
    inductive_kvarh numeric(18,4),
    capacitive_kvarh numeric(18,4),
    demand_kw     numeric(14,4),
    vat_base      numeric(18,4),
    vat           numeric(18,4),
    btv           numeric(18,4),
    energy_fund   numeric(18,4),
    trt           numeric(18,4),
    price_difference numeric(18,4),
    correction_amount numeric(18,4),
    is_cancelled  boolean not null default false,
    term          text,
    voltage_level text,
    is_multi_time boolean,
    raw           jsonb
);
create index on icmal_rows (import_id);

-- +goose Down
-- Reverse dependency order, and deliberately without `cascade`: a cascade
-- would mask a drop this migration forgot — the round-trip guard would still
-- see an empty schema — while silently destroying objects a later migration
-- owns. Enum types are dropped after the tables whose columns use them.
drop table if exists icmal_rows;
drop table if exists icmal_imports;
drop table if exists national_tariff_schedule;
drop table if exists solar_tariffs;
drop table if exists tariff_templates;
drop table if exists tariff_manual_yekdem;
drop table if exists tariff_taxes;
drop table if exists tariffs;
drop type if exists distribution_user_group;
drop type if exists currency_code;
drop type if exists generation_usage;
drop type if exists supply_company;
drop type if exists voltage_level;
drop type if exists energy_type;
drop type if exists tariff_term;
drop type if exists price_type;
