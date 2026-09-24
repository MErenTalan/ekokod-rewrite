# 04 — Data Model

PostgreSQL 16 + TimescaleDB. All DDL below is the target schema; it is authoritative for column
names, types and constraints. Migrations are plain SQL under `migrations/`.

Conventions:
- `id uuid primary key default gen_random_uuid()` everywhere.
- `created_at`, `updated_at` as `timestamptz not null default now()`.
- Money and energy as `numeric`. Never `float`/`double`.
- Timestamps as `timestamptz`, stored UTC.
- Soft delete only where a business reason exists (companies, buildings, analyzers, users); it is
  `deleted_at timestamptz` and every query filters it.
- Every tenant-scoped table carries `company_id` directly, even when reachable via a parent —
  this makes row-level scoping cheap and makes accidental cross-tenant joins impossible to miss.

---

## 1. Extensions

```sql
create extension if not exists "pgcrypto";
create extension if not exists "timescaledb";
create extension if not exists "btree_gin";
```

---

## 2. Tenancy, identity and access

```sql
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
```

---

## 3. Buildings, plants and metering points

```sql
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
```

---

## 4. Time series — the core of the rewrite

### 4.1 Meter readings

```sql
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
```

**Why this fixes the growth problem.** In the legacy model every reading was appended to an array
inside one document per meter, so a single meter's document grew past 5 MB and every read loaded the
entire history. Here each reading is one row in a time-partitioned, compressed table, and every
query touches only the chunks in range.

### 4.2 Ingestion high-water marks

```sql
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
```

Ingestion resumes from `last_ts`. Nothing is refetched from the beginning.

### 4.3 Consumption — continuous aggregates

Consumption is **derived, never stored as a maintained collection**. `first()` and `last()` give the
register values at the period boundaries.

```sql
create materialized view consumption_hourly
with (timescaledb.continuous) as
select
    analyzer_id,
    time_bucket('1 hour', ts) as bucket,
    first(active_import, ts)              as active_import_start,
    last(active_import, ts)               as active_import_end,
    last(active_import, ts) - first(active_import, ts)             as active_consumption,
    last(reactive_inductive_import, ts)  - first(reactive_inductive_import, ts)  as inductive_consumption,
    last(reactive_capacitive_import, ts) - first(reactive_capacitive_import, ts) as capacitive_consumption,
    last(t1_import, ts) - first(t1_import, ts)                     as t1_consumption,
    last(t2_import, ts) - first(t2_import, ts)                     as t2_consumption,
    last(t3_import, ts) - first(t3_import, ts)                     as t3_consumption,
    last(active_export, ts) - first(active_export, ts)             as active_generation,
    last(reactive_inductive_export, ts)  - first(reactive_inductive_export, ts)  as inductive_generation,
    last(reactive_capacitive_export, ts) - first(reactive_capacitive_export, ts) as capacitive_generation,
    last(t1_export, ts) - first(t1_export, ts)                     as t1_generation,
    last(t2_export, ts) - first(t2_export, ts)                     as t2_generation,
    last(t3_export, ts) - first(t3_export, ts)                     as t3_generation,
    max(max_demand_kw)                                             as max_demand_kw,
    last(active_import, ts)              as active_index,
    last(reactive_inductive_import, ts)  as inductive_index,
    last(reactive_capacitive_import, ts) as capacitive_index,
    last(t1_import, ts)                  as t1_index,
    last(t2_import, ts)                  as t2_index,
    last(t3_import, ts)                  as t3_index,
    last(active_export, ts)              as active_generation_index,
    count(*)                             as reading_count
from meter_readings
where kind = 'load_profile'
group by analyzer_id, bucket;

select add_continuous_aggregate_policy('consumption_hourly',
    start_offset      => interval '30 days',
    end_offset        => interval '1 hour',
    schedule_interval => interval '30 minutes');
```

`consumption_daily`, `consumption_monthly` and `consumption_yearly` follow the same shape,
hierarchically rolled up from the level below where TimescaleDB permits it, or defined directly over
the hypertable where boundary semantics require it.

**Boundary semantics.** A continuous aggregate buckets by wall time and computes
`last − first` *within* the bucket, which misses the step between the last reading of one bucket
and the first of the next (for a meter that reports hourly, the whole hourly figure). The aggregates
therefore also keep each register's first value and the first reading's instant, and the analytics
repository differences consecutive bucket boundaries with a one-bucket look-ahead (F15q R470–R471):
with readings on the bucket edges it agrees with billing. Where exactness matters — billing
above all — consumption is computed by the **domain function** using the readings at the true period
boundaries (§3.1 of `02-domain-rules.md`), reading directly from `meter_readings`. The continuous
aggregates serve charts, tables and analytics, where the difference is immaterial and the speed is
essential.

This distinction must be explicit in the code: `analytics.Consumption(...)` reads aggregates;
`billing.Consumption(...)` reads the hypertable. They are different functions with different
guarantees.

### 4.4 Suspect periods

```sql
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
```

Billing refuses to issue an invoice for a period with an unresolved anomaly.

### 4.5 Solar production

```sql
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
select add_compression_policy('plant_production', interval '90 days');
```

with `plant_production_daily` / `_monthly` continuous aggregates.

### 4.6 Market prices

```sql
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
```

---

## 5. Tariffs

```sql
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
```

### İcmal imports

```sql
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
```

---

## 6. Invoices

```sql
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
```

---

## 7. Alarms

```sql
create type alarm_type as enum (
    'reactive_limit',
    'data_communication',
    'current_voltage_power',
    'invoice_increase'
);
create type period_unit   as enum ('hours','days');
create type notify_channel as enum ('email','sms');

create table alarms (
    id          uuid primary key default gen_random_uuid(),
    company_id  uuid not null references companies(id),
    name        text not null,
    type        alarm_type not null,
    is_enabled  boolean not null default true,

    -- reactive_limit
    inductive_ratio_threshold  numeric(6,3),
    inductive_period_value     integer,
    inductive_period_unit      period_unit,
    capacitive_ratio_threshold numeric(6,3),
    capacitive_period_value    integer,
    capacitive_period_unit     period_unit,
    active_consumption_max     numeric(18,4),
    active_consumption_max_period_value integer,
    active_consumption_max_period_unit  period_unit,
    active_consumption_min     numeric(18,4),
    active_consumption_min_period_value integer,
    active_consumption_min_period_unit  period_unit,

    -- data_communication
    communication_threshold_hours integer,

    -- current_voltage_power
    voltage_max numeric(12,3),
    voltage_min numeric(12,3),
    power_max   numeric(12,3),
    power_min   numeric(12,3),

    -- invoice_increase
    invoice_threshold_pct numeric(6,3),

    notification_frequency_value integer,
    notification_frequency_unit  period_unit,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    deleted_at timestamptz
);
create index on alarms (company_id) where deleted_at is null;

create table alarm_analyzers (
    alarm_id    uuid not null references alarms(id) on delete cascade,
    analyzer_id uuid not null references analyzers(id) on delete cascade,
    primary key (alarm_id, analyzer_id)
);

create table alarm_channels (
    alarm_id uuid not null references alarms(id) on delete cascade,
    channel  notify_channel not null,
    target   text not null,             -- email address or phone number
    primary key (alarm_id, channel, target)
);

create table alarm_events (
    id           uuid primary key default gen_random_uuid(),
    alarm_id     uuid not null references alarms(id) on delete cascade,
    analyzer_id  uuid references analyzers(id),
    triggered_at timestamptz not null default now(),
    message      text not null,
    detail       jsonb,
    notified_at  timestamptz,
    notification_error text
);
create index on alarm_events (alarm_id, triggered_at desc);
create index on alarm_events (analyzer_id, triggered_at desc);

-- Prevents an invoice firing the same alarm twice.
create table alarm_fired_bills (
    alarm_id uuid not null references alarms(id) on delete cascade,
    bill_id  uuid not null references bills(id) on delete cascade,
    primary key (alarm_id, bill_id)
);

-- iSolar alarm forwarding deduplication.
create table isolar_forwarded_alarms (
    plant_id  uuid not null references power_plants(id) on delete cascade,
    alarm_ref text not null,
    sent_at   timestamptz not null default now(),
    primary key (plant_id, alarm_ref)
);
```

---

## 8. Reports

```sql
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
```

`payload` is the only place `jsonb` holds business data. It is a **rendered snapshot** — the report
as issued — and is never queried by field. Figures needed for filtering or listing are duplicated
into columns.

---

## 9. Carbon module

```sql
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
```

---

## 10. ISO 50001

```sql
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
```

---

## 11. Files, integrations, calendar and operations

```sql
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
```

---

## 12. Forecasts

```sql
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
```

---

## 13. Seed data

Loaded by `bcem seed`, shipped in the offline bundle:

| Dataset | Source |
|---------|--------|
| Emission factor master catalogue | Migrated from the legacy carbon module's factor list (thousands of entries: fuels, vehicles, flights, freight, refrigerants, materials, waste). |
| GHG ↔ ISO 14064 category mapping | Migrated from the legacy mapping table. |
| Integration provider definitions | Provider + subtype + endpoint templates for every distribution company currently configured. |
| National tariff schedule | The published tariff schedule backing the public bill calculator, with effective dates. |
| ISO 50001 clause texts and templates | Clause titles, descriptions and downloadable template files for clauses 5–9. |
| Blog articles | The two existing Markdown articles and their images. |

---

## 14. Index and performance notes

- Every foreign key used in a filter has an index.
- Hypertable queries always include a `ts` range so chunk exclusion applies. A query without a time
  bound on `meter_readings` is a bug.
- Bulk ingestion uses `COPY` into a staging table followed by an idempotent
  `insert … on conflict do update`, not row-by-row inserts.
- Continuous-aggregate refresh policies are tuned so that the most recent hours are current within
  30 minutes, while older buckets refresh less frequently.
- The `bills` unique index enforces one live bill per scope and period; recomputation supersedes the
  previous bill rather than deleting it, preserving what was issued.
