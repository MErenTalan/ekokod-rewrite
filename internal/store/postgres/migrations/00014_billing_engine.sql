-- +goose Up
-- F4 billing engine (R106, R119, R126, R127): dated billing parameters, tariff
-- price sources and extra charges, and the bill columns the engine persists.
create type reactive_penalty_basis as enum ('whole_quantity','excess_over_limit');
create type tiering_mode as enum ('split_at_threshold','whole_consumption_switch');
create type price_source as enum ('kbk','fixed');
create type extra_charge_basis as enum ('per_kwh','per_contracted_kw','per_max_demand_kw','fixed_per_period','pct_of_energy');
create type money_rounding_mode as enum ('half_up','half_even'); -- I-1

create table billing_parameters (
    effective_from                 date primary key,
    reactive_penalty_basis         reactive_penalty_basis not null default 'whole_quantity',
    reactive_exempt_below_kw       numeric(12,3) default 9,
    reactive_exempt_terms          tariff_term[] not null default '{monomial}',
    reactive_exempt_user_groups    distribution_user_group[] not null default '{residential,lighting}',
    reactive_generation_exempt_kwh numeric(18,6) not null default 1,
    reactive_bands                 jsonb not null,
    tiering_groups                 jsonb not null,
    tiering_mode                   tiering_mode not null default 'split_at_threshold',
    tiering_voltage_levels         voltage_level[] not null default '{lv}',       -- C-3/LBR F.3, was {lv,mv}
    tiering_supply_companies       supply_company[] not null default '{incumbent}', -- C-3
    ptf_missing_hour_tolerance     numeric(6,5) not null default 0.02
        check (ptf_missing_hour_tolerance between 0 and 1),
    demand_overrun_multiplier      numeric(8,4) not null default 2 check (demand_overrun_multiplier >= 0),
    money_rounding_mode            money_rounding_mode not null default 'half_up', -- I-1
    created_at                     timestamptz not null default now()
);
insert into billing_parameters (effective_from, reactive_bands, tiering_groups) values (
    '2000-01-01',
    '[{"min_kw":"9","max_kw":"30","inductive":"0.33","capacitive":"0.20"},{"min_kw":"30","max_kw":null,"inductive":"0.20","capacitive":"0.15"}]',
    '{"residential":"8","commercial":"30"}');

alter table tariffs
    add column power_price_source        price_source not null default 'kbk',
    add column reactive_price_source     price_source not null default 'kbk',
    add column distribution_price_source price_source not null default 'kbk';

-- I-10: 00006's checks reject a validated multi-time PTF tariff (kbk_t1..t3 only, no t1..t3_price)
-- and a single-time PTF tariff without a fixed price. Relax with a PTF exemption; Down restores
-- the originals from 00006_tariffs.sql exactly.
alter table tariffs drop constraint single_time_needs_price, drop constraint multi_time_needs_prices;
alter table tariffs add constraint single_time_needs_price
        check (price_type <> 'single_time' or use_ptf_yekdem or single_time_price is not null),
    add constraint multi_time_needs_prices
        check (price_type <> 'multi_time'
               or (case when use_ptf_yekdem
                        then kbk_t1 is not null and kbk_t2 is not null and kbk_t3 is not null
                        else t1_price is not null and t2_price is not null and t3_price is not null end));

create table tariff_extra_charges (
    id         uuid primary key default gen_random_uuid(),
    tariff_id  uuid not null references tariffs(id) on delete cascade,
    name       text not null,
    basis      extra_charge_basis not null,
    amount     numeric(18,6) not null,
    sort_order smallint not null default 0
);
create index on tariff_extra_charges (tariff_id);

alter table bills
    add column currency                  currency_code not null default 'TRY',
    add column extra_charges_cost        numeric(18,4) not null default 0,
    add column demand_data_available     boolean not null default true,
    add column ptf_hours_expected        integer,
    add column consumption_hours_missing integer;

-- +goose Down
alter table bills drop column consumption_hours_missing, drop column ptf_hours_expected,
    drop column demand_data_available, drop column extra_charges_cost, drop column currency;
drop table tariff_extra_charges;
alter table tariffs drop constraint single_time_needs_price, drop constraint multi_time_needs_prices;
alter table tariffs add constraint single_time_needs_price
        check (price_type <> 'single_time' or single_time_price is not null),
    add constraint multi_time_needs_prices
        check (price_type <> 'multi_time'
               or (t1_price is not null and t2_price is not null and t3_price is not null));
alter table tariffs drop column distribution_price_source, drop column reactive_price_source, drop column power_price_source;
drop table billing_parameters;
drop type money_rounding_mode; drop type extra_charge_basis; drop type price_source; drop type tiering_mode; drop type reactive_penalty_basis;
