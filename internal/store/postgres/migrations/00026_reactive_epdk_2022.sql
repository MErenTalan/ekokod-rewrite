-- +goose Up
-- R526–R529: reactive rules of EPDK Kurul Kararı 10961 (RG 30.04.2022, Usul ve
-- Esaslar m.13). The flags default to the legacy behaviour so older rows keep
-- pricing as before; the 2022-05-01 row turns them on.
alter table billing_parameters
    add column reactive_exempt_inclusive       boolean not null default false,
    add column reactive_charge_higher_side     boolean not null default false,
    add column reactive_first_violation_waived boolean not null default false;

insert into billing_parameters (
    effective_from, reactive_penalty_basis, reactive_exempt_below_kw, reactive_exempt_inclusive,
    reactive_exempt_terms, reactive_exempt_user_groups, reactive_generation_exempt_kwh, reactive_bands,
    reactive_charge_higher_side, reactive_first_violation_waived, tiering_groups, tiering_mode,
    tiering_voltage_levels, tiering_supply_companies, ptf_missing_hour_tolerance,
    demand_overrun_multiplier, money_rounding_mode)
select '2022-05-01', 'whole_quantity', 15, true,
       '{}', '{residential,residential_plus,martyrs_families,lighting,public_lighting}',
       reactive_generation_exempt_kwh,
       '[{"min_kw":"15","max_kw":"50","inductive":"0.33","capacitive":"0.20"},{"min_kw":"50","max_kw":null,"inductive":"0.20","capacitive":"0.15"}]',
       true, true, tiering_groups, tiering_mode,
       tiering_voltage_levels, tiering_supply_companies, ptf_missing_hour_tolerance,
       demand_overrun_multiplier, money_rounding_mode
from billing_parameters
where effective_from < '2022-05-01'
order by effective_from desc
limit 1
on conflict (effective_from) do nothing;

-- +goose Down
delete from billing_parameters where effective_from = '2022-05-01';
alter table billing_parameters
    drop column if exists reactive_exempt_inclusive,
    drop column if exists reactive_charge_higher_side,
    drop column if exists reactive_first_violation_waived;
