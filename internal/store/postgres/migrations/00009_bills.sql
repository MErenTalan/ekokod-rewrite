-- +goose Up
-- Bills, transcribed from docs/rewrite/04-data-model.md section 6. `bills` is
-- authoritative for column names, types, precision and constraints; the
-- partial unique index below is the rule that makes recomputation safe (see
-- section 14): one live bill per scope and period, and recomputing supersedes
-- the previous bill rather than deleting it.

create type bill_scope  as enum ('analyzer','building','company');
create type bill_status as enum ('draft','issued','flagged','superseded');

create table bills (
    id              uuid primary key default gen_random_uuid(),
    company_id      uuid not null references companies(id),
    building_id     uuid references buildings(id),
    analyzer_id     uuid references analyzers(id),
    scope           bill_scope not null,
    period_key      char(7) not null,          -- YYYY-MM
    period_start    timestamptz not null,
    period_end      timestamptz not null,
    days_in_period  integer not null,

    tariff_id       uuid references tariffs(id),
    tariff_effective_from date,

    -- quantities
    active_import        numeric(18,4) not null default 0,
    t1_kwh               numeric(18,4) not null default 0,
    t2_kwh               numeric(18,4) not null default 0,
    t3_kwh               numeric(18,4) not null default 0,
    inductive_kvarh      numeric(18,4) not null default 0,
    capacitive_kvarh     numeric(18,4) not null default 0,
    active_export        numeric(18,4) not null default 0,
    net_consumption      numeric(18,4) not null default 0,
    low_tier_kwh         numeric(18,4) not null default 0,
    high_tier_kwh        numeric(18,4) not null default 0,
    max_demand_kw        numeric(14,4),
    tiered_applied       boolean not null default false,

    -- register readings at the boundaries
    index_start          jsonb not null,
    index_end            jsonb not null,

    -- unit prices actually used
    effective_energy_price numeric(18,6),
    low_tier_price       numeric(18,6),
    high_tier_price      numeric(18,6),

    -- charges
    energy_cost          numeric(18,4) not null default 0,
    distribution_cost    numeric(18,4) not null default 0,
    green_energy_cost    numeric(18,4) not null default 0,
    power_cost           numeric(18,4) not null default 0,
    demand_overrun_cost  numeric(18,4) not null default 0,
    reactive_penalty     numeric(18,4) not null default 0,
    other_taxes_cost     numeric(18,4) not null default 0,
    vat_base             numeric(18,4) not null default 0,
    vat_cost             numeric(18,4) not null default 0,
    generation_credit    numeric(18,4) not null default 0,
    total_cost           numeric(18,4) not null default 0,

    -- reactive detail
    inductive_ratio      numeric(10,6),
    capacitive_ratio     numeric(10,6),
    inductive_threshold  numeric(10,6),
    capacitive_threshold numeric(10,6),
    reactive_penalty_applied boolean not null default false,
    reactive_power_price numeric(18,6),

    -- generation handling
    generation_usage     generation_usage not null default 'none',
    generation_price_per_kwh numeric(18,6),

    -- dynamic pricing
    ptf_yekdem_used      boolean not null default false,
    ptf_hours_matched    integer,
    ptf_hours_missing    integer,
    ptf_average          numeric(14,4),
    yekdem_used          numeric(14,4),

    status               bill_status not null default 'draft',
    flag_reason          text,
    pdf_path             text,
    computed_at          timestamptz not null default now(),
    created_at           timestamptz not null default now(),
    updated_at           timestamptz not null default now()
);
create unique index on bills (scope, coalesce(analyzer_id, building_id, company_id), period_key)
    where status <> 'superseded';
create index on bills (company_id, period_key);
create index on bills (building_id, period_key);

create table bill_lines (
    id          uuid primary key default gen_random_uuid(),
    bill_id     uuid not null references bills(id) on delete cascade,
    code        text not null,   -- 'energy','distribution','power','overrun','reactive','vat','tax:BTV',...
    label       text not null,   -- as printed, localised at render time
    quantity    numeric(18,4),
    unit        text,
    unit_price  numeric(18,6),
    rate_pct    numeric(6,3),
    amount      numeric(18,4) not null,
    sort_order  smallint not null default 0
);
create index on bill_lines (bill_id);

-- Which analyzers were rolled into a building/company bill.
create table bill_members (
    bill_id     uuid not null references bills(id) on delete cascade,
    analyzer_id uuid not null references analyzers(id),
    primary key (bill_id, analyzer_id)
);

-- Per-hour detail for PTF-priced bills; feeds the hourly Excel export.
create table bill_hourly_detail (
    bill_id     uuid        not null references bills(id) on delete cascade,
    ts          timestamptz not null,
    consumption numeric(18,4) not null,
    ptf         numeric(14,4) not null,
    yekdem      numeric(14,4) not null,
    kbk         numeric(12,4) not null,
    unit_price  numeric(18,6) not null,
    cost        numeric(18,4) not null,
    primary key (bill_id, ts)
);

-- +goose Down
-- Reverse dependency order, and deliberately without `cascade`: a cascade
-- would mask a drop this migration forgot — the round-trip guard would still
-- see an empty schema — while silently destroying objects a later migration
-- owns. Enum types are dropped after the tables whose columns use them.
drop table if exists bill_hourly_detail;
drop table if exists bill_members;
drop table if exists bill_lines;
drop table if exists bills;
drop type if exists bill_status;
drop type if exists bill_scope;
