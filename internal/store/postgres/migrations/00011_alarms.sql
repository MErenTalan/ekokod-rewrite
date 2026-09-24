-- +goose Up
-- Alarms, transcribed from docs/rewrite/04-data-model.md section 7.
-- `alarm_fired_bills` references `bills` (migration 00009), so this file must
-- land after it; goose runs `down` in reverse numeric order, so this file's
-- down (which drops alarm_fired_bills) runs before 00009's down (which drops
-- bills) with no cross-file drops needed.

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

-- +goose Down
-- Reverse dependency order, and deliberately without `cascade`: a cascade
-- would mask a drop this migration forgot — the round-trip guard would still
-- see an empty schema — while silently destroying objects a later migration
-- owns. Enum types are dropped after the tables whose columns use them.
drop table if exists isolar_forwarded_alarms;
drop table if exists alarm_fired_bills;
drop table if exists alarm_events;
drop table if exists alarm_channels;
drop table if exists alarm_analyzers;
drop table if exists alarms;
drop type if exists notify_channel;
drop type if exists period_unit;
drop type if exists alarm_type;
