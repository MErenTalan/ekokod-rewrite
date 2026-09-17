-- Tariffs, tariff templates, solar tariffs, the national tariff schedule and
-- icmal imports (migration 00006). Query names are prefixed with the
-- aggregate they belong to (Tariff…, TariffTemplate…, SolarTariff…,
-- NationalTariff…, Icmal…) per the wave-f convention.

-- name: TariffGet :one
select * from tariffs
where tariffs.id = sqlc.arg(id) and tariffs.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or tariffs.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and tariffs.deleted_at is null
  -- Folded minor (fix round 1): a tariff of a soft-deleted building must not
  -- stay readable through the tariff's own row.
  and (tariffs.building_id is null or exists (select 1 from buildings b where b.id = tariffs.building_id and b.deleted_at is null));

-- name: TariffList :many
select * from tariffs
where tariffs.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or tariffs.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and (cardinality(sqlc.arg(ids)::uuid[]) = 0 or tariffs.id = any(sqlc.arg(ids)::uuid[]))
  and (sqlc.narg(building_id)::uuid is null or tariffs.building_id = sqlc.narg(building_id))
  and (not sqlc.arg(company_wide)::boolean or tariffs.building_id is null)
  and (sqlc.narg(effective_on)::date is null or tariffs.effective_from <= sqlc.narg(effective_on))
  and (sqlc.arg(include_deleted)::boolean or tariffs.deleted_at is null)
  and (tariffs.building_id is null or exists (select 1 from buildings b where b.id = tariffs.building_id and b.deleted_at is null))
order by tariffs.effective_from desc, tariffs.id
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- name: TariffCreate :one
insert into tariffs (
  id, company_id, building_id, name, effective_from, currency, energy_type,
  voltage_level, user_group, price_type, term, supply_company,
  single_time_price, t1_price, t2_price, t3_price, overuse_price,
  overuse_threshold_kwh_per_day, distribution_cost, reactive_power_price,
  green_energy_price, green_energy_distribution_cost, contracted_power_kw,
  power_unit_price, generation_usage, generation_price_per_kwh, vat_rate,
  use_ptf_yekdem, kbk_energy, kbk_t1, kbk_t2, kbk_t3, kbk_power_price,
  kbk_overuse_price, kbk_reactive_power, kbk_distribution_cost_tl_per_kwh,
  use_manual_yekdem, created_by, created_at, updated_at,
  power_price_source, reactive_price_source, distribution_price_source
)
select
  gen_random_uuid(), sqlc.arg(company_id), sqlc.narg(building_id), sqlc.narg(name),
  sqlc.arg(effective_from), sqlc.arg(currency), sqlc.arg(energy_type),
  sqlc.arg(voltage_level), sqlc.arg(user_group), sqlc.arg(price_type), sqlc.arg(term),
  sqlc.arg(supply_company), sqlc.narg(single_time_price), sqlc.narg(t1_price),
  sqlc.narg(t2_price), sqlc.narg(t3_price), sqlc.narg(overuse_price),
  sqlc.narg(overuse_threshold_kwh_per_day), sqlc.arg(distribution_cost),
  sqlc.arg(reactive_power_price), sqlc.narg(green_energy_price),
  sqlc.narg(green_energy_distribution_cost), sqlc.narg(contracted_power_kw),
  sqlc.narg(power_unit_price), sqlc.arg(generation_usage), sqlc.narg(generation_price_per_kwh),
  sqlc.arg(vat_rate), sqlc.arg(use_ptf_yekdem), sqlc.narg(kbk_energy), sqlc.narg(kbk_t1),
  sqlc.narg(kbk_t2), sqlc.narg(kbk_t3), sqlc.narg(kbk_power_price), sqlc.narg(kbk_overuse_price),
  sqlc.narg(kbk_reactive_power), sqlc.narg(kbk_distribution_cost_tl_per_kwh),
  sqlc.arg(use_manual_yekdem), sqlc.narg(created_by), sqlc.arg(created_at), sqlc.arg(created_at),
  sqlc.arg(power_price_source), sqlc.arg(reactive_price_source), sqlc.arg(distribution_price_source)
where
  -- Critical Finding 1 (task-11a fix round 1): a stored building_id is a
  -- foreign key and MUST be validated in SQL against the Scope's company,
  -- deleted_at and building branch. Scope.AllowsBuilding is an in-memory
  -- grant check that cannot know which company owns a building id — an
  -- AllBuildings Scope must not be able to store ANOTHER TENANT's building
  -- id just because the in-memory check answers "yes" for every id.
  (sqlc.narg(building_id)::uuid is null or exists (
    select 1 from buildings b
    where b.id = sqlc.narg(building_id) and b.company_id = sqlc.arg(company_id) and b.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
  ))
  -- Important Finding 4: created_by is a stored foreign key too, and must
  -- name a user of the same company rather than being stored as given.
  and (sqlc.narg(created_by)::uuid is null or exists (
    select 1 from users u where u.id = sqlc.narg(created_by) and u.company_id = sqlc.arg(company_id)
  ))
returning *;

-- name: TariffUpdate :one
update tariffs set
  building_id = sqlc.narg(building_id), name = sqlc.narg(name),
  effective_from = sqlc.arg(effective_from), currency = sqlc.arg(currency),
  energy_type = sqlc.arg(energy_type), voltage_level = sqlc.arg(voltage_level),
  user_group = sqlc.arg(user_group), price_type = sqlc.arg(price_type), term = sqlc.arg(term),
  supply_company = sqlc.arg(supply_company), single_time_price = sqlc.narg(single_time_price),
  t1_price = sqlc.narg(t1_price), t2_price = sqlc.narg(t2_price), t3_price = sqlc.narg(t3_price),
  overuse_price = sqlc.narg(overuse_price),
  overuse_threshold_kwh_per_day = sqlc.narg(overuse_threshold_kwh_per_day),
  distribution_cost = sqlc.arg(distribution_cost), reactive_power_price = sqlc.arg(reactive_power_price),
  green_energy_price = sqlc.narg(green_energy_price),
  green_energy_distribution_cost = sqlc.narg(green_energy_distribution_cost),
  contracted_power_kw = sqlc.narg(contracted_power_kw), power_unit_price = sqlc.narg(power_unit_price),
  generation_usage = sqlc.arg(generation_usage), generation_price_per_kwh = sqlc.narg(generation_price_per_kwh),
  vat_rate = sqlc.arg(vat_rate), use_ptf_yekdem = sqlc.arg(use_ptf_yekdem),
  kbk_energy = sqlc.narg(kbk_energy), kbk_t1 = sqlc.narg(kbk_t1), kbk_t2 = sqlc.narg(kbk_t2),
  kbk_t3 = sqlc.narg(kbk_t3), kbk_power_price = sqlc.narg(kbk_power_price),
  kbk_overuse_price = sqlc.narg(kbk_overuse_price), kbk_reactive_power = sqlc.narg(kbk_reactive_power),
  kbk_distribution_cost_tl_per_kwh = sqlc.narg(kbk_distribution_cost_tl_per_kwh),
  use_manual_yekdem = sqlc.arg(use_manual_yekdem), updated_at = sqlc.arg(updated_at),
  power_price_source = sqlc.arg(power_price_source), reactive_price_source = sqlc.arg(reactive_price_source),
  distribution_price_source = sqlc.arg(distribution_price_source)
where tariffs.id = sqlc.arg(id) and tariffs.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or tariffs.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and tariffs.deleted_at is null
  -- Critical Finding 1: the NEW building_id this write stores must be
  -- validated exactly as TariffCreate validates it — moving a tariff to
  -- another tenant's building is the same stored-foreign-key bug as
  -- creating one there.
  and (sqlc.narg(building_id)::uuid is null or exists (
    select 1 from buildings b
    where b.id = sqlc.narg(building_id) and b.company_id = sqlc.arg(company_id) and b.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
  ))
returning *;

-- name: TariffSoftDelete :execrows
update tariffs set deleted_at = sqlc.arg(deleted_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null;

-- TariffBuildingVisible checks that a building is visible to the Scope
-- before Effective is allowed to fall back to a company-wide tariff.
-- name: TariffBuildingVisible :one
select exists (
  select 1 from buildings
  where id = sqlc.arg(building_id) and company_id = sqlc.arg(company_id) and deleted_at is null
    and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]))
) as visible;

-- Important Finding 5: the day boundary is Europe/Istanbul (converted by the
-- Go caller — see tariffTimeToDate in pgtime.go — before effective_on ever
-- reaches this query) and ties on effective_from resolve deterministically:
-- the schema has no unique index on (building_id, effective_from), so two
-- rows CAN share one, and "the latest one written" must always mean the
-- same row, not whichever the query planner happens to return first.
-- name: TariffEffectiveForBuilding :one
select * from tariffs
where company_id = sqlc.arg(company_id) and building_id = sqlc.arg(building_id)
  and deleted_at is null and effective_from <= sqlc.arg(effective_on)
order by effective_from desc, created_at desc, id desc
limit 1;

-- name: TariffEffectiveCompanyWide :one
select * from tariffs
where company_id = sqlc.arg(company_id) and building_id is null
  and deleted_at is null and effective_from <= sqlc.arg(effective_on)
order by effective_from desc, created_at desc, id desc
limit 1;

-- name: TariffVisible :one
select exists (
  select 1 from tariffs
  where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
    and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
    and deleted_at is null
) as visible;

-- Important Finding 1: tariff_taxes has no company_id of its own, and the
-- Go-side requireVisible check that guards Taxes/ReplaceTaxes runs OUTSIDE
-- the write itself — every statement below re-validates that tariff_id
-- names a tariff visible to the Scope, so the guard survives even if the
-- Go-side pre-check were ever skipped.
-- name: TariffTaxList :many
select tt.* from tariff_taxes tt
where tt.tariff_id = sqlc.arg(tariff_id)
  and exists (
    select 1 from tariffs t where t.id = tt.tariff_id and t.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or t.building_id = any(sqlc.arg(building_ids)::uuid[]))
      and t.deleted_at is null
  )
order by tt.sort_order, tt.id;

-- name: TariffTaxDeleteForTariff :exec
delete from tariff_taxes
where tariff_id = sqlc.arg(tariff_id)
  and exists (
    select 1 from tariffs t where t.id = sqlc.arg(tariff_id) and t.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or t.building_id = any(sqlc.arg(building_ids)::uuid[]))
      and t.deleted_at is null
  );

-- name: TariffTaxInsert :one
insert into tariff_taxes (id, tariff_id, name, rate, sort_order)
select gen_random_uuid(), sqlc.arg(tariff_id), sqlc.arg(name), sqlc.arg(rate), sqlc.arg(sort_order)
where exists (
  select 1 from tariffs t where t.id = sqlc.arg(tariff_id) and t.company_id = sqlc.arg(company_id)
    and (sqlc.arg(all_buildings)::boolean or t.building_id = any(sqlc.arg(building_ids)::uuid[]))
    and t.deleted_at is null
)
returning *;

-- name: TariffManualYekdemList :many
select tmy.* from tariff_manual_yekdem tmy
where tmy.tariff_id = sqlc.arg(tariff_id)
  and exists (
    select 1 from tariffs t where t.id = tmy.tariff_id and t.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or t.building_id = any(sqlc.arg(building_ids)::uuid[]))
      and t.deleted_at is null
  )
order by tmy.year, tmy.month;

-- name: TariffManualYekdemDeleteForTariff :exec
delete from tariff_manual_yekdem
where tariff_id = sqlc.arg(tariff_id)
  and exists (
    select 1 from tariffs t where t.id = sqlc.arg(tariff_id) and t.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or t.building_id = any(sqlc.arg(building_ids)::uuid[]))
      and t.deleted_at is null
  );

-- name: TariffManualYekdemInsert :one
insert into tariff_manual_yekdem (tariff_id, year, month, value)
select sqlc.arg(tariff_id), sqlc.arg(year), sqlc.arg(month), sqlc.arg(value)
where exists (
  select 1 from tariffs t where t.id = sqlc.arg(tariff_id) and t.company_id = sqlc.arg(company_id)
    and (sqlc.arg(all_buildings)::boolean or t.building_id = any(sqlc.arg(building_ids)::uuid[]))
    and t.deleted_at is null
)
returning *;

-- name: TariffTemplateGet :one
select * from tariff_templates where id = sqlc.arg(id) and company_id = sqlc.arg(company_id);

-- name: TariffTemplateList :many
select * from tariff_templates
where company_id = sqlc.arg(company_id)
  and (sqlc.arg(name_contains) = '' or name ilike '%' || sqlc.arg(name_contains) || '%')
  and (sqlc.narg(is_default)::boolean is null or is_default = sqlc.narg(is_default))
order by name
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- name: TariffTemplateCreate :one
insert into tariff_templates (id, company_id, name, description, is_default, payload, created_at, updated_at)
values (gen_random_uuid(), sqlc.arg(company_id), sqlc.arg(name), sqlc.narg(description),
        sqlc.arg(is_default), sqlc.arg(payload), sqlc.arg(created_at), sqlc.arg(created_at))
returning *;

-- name: TariffTemplateUpdate :one
update tariff_templates set
  name = sqlc.arg(name), description = sqlc.narg(description), is_default = sqlc.arg(is_default),
  payload = sqlc.arg(payload), updated_at = sqlc.arg(updated_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
returning *;

-- name: TariffTemplateDelete :execrows
delete from tariff_templates where id = sqlc.arg(id) and company_id = sqlc.arg(company_id);

-- name: SolarTariffGet :one
select * from solar_tariffs
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- name: SolarTariffList :many
select * from solar_tariffs
where company_id = sqlc.arg(company_id)
  and (sqlc.narg(plant_id)::uuid is null or plant_id = sqlc.narg(plant_id))
  and (sqlc.narg(effective_on)::date is null or effective_from <= sqlc.narg(effective_on))
  and (sqlc.arg(include_deleted)::boolean or deleted_at is null)
order by effective_from desc, id
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- name: SolarTariffCreate :one
insert into solar_tariffs (id, company_id, plant_id, effective_from, feed_in_tariff, purchase_price, currency, notes, created_at)
select gen_random_uuid(), sqlc.arg(company_id), sqlc.arg(plant_id), sqlc.arg(effective_from),
       sqlc.arg(feed_in_tariff), sqlc.narg(purchase_price), sqlc.arg(currency), sqlc.narg(notes), sqlc.arg(created_at)
where exists (
  select 1 from power_plants p where p.id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
)
returning *;

-- name: SolarTariffSoftDelete :execrows
update solar_tariffs set deleted_at = sqlc.arg(deleted_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- Important Finding 5: same Istanbul day boundary (converted by the Go
-- caller) and deterministic tiebreak as TariffEffectiveForBuilding.
-- name: SolarTariffEffective :one
select * from solar_tariffs
where company_id = sqlc.arg(company_id) and plant_id = sqlc.arg(plant_id)
  and deleted_at is null and effective_from <= sqlc.arg(effective_on)
order by effective_from desc, created_at desc, id desc
limit 1;

-- name: SolarTariffPlantVisible :one
select exists (
  select 1 from power_plants
  where id = sqlc.arg(plant_id) and company_id = sqlc.arg(company_id) and deleted_at is null
) as visible;

-- name: NationalTariffList :many
select * from national_tariff_schedule
where (sqlc.narg(user_group)::distribution_user_group is null or user_group = sqlc.narg(user_group))
  and (sqlc.narg(voltage_level)::voltage_level is null or voltage_level = sqlc.narg(voltage_level))
  and (sqlc.narg(term)::tariff_term is null or term = sqlc.narg(term))
  and (sqlc.narg(effective_on)::date is null or effective_from <= sqlc.narg(effective_on))
order by effective_from desc, id
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- Important Finding 5: same Istanbul day boundary and deterministic tiebreak.
-- name: NationalTariffEffective :one
select * from national_tariff_schedule
where user_group = sqlc.arg(user_group) and voltage_level = sqlc.arg(voltage_level) and term = sqlc.arg(term)
  and effective_from <= sqlc.arg(effective_on)
order by effective_from desc, created_at desc, id desc
limit 1;

-- name: IcmalImportCreate :one
insert into icmal_imports (id, company_id, uploaded_by, file_name, row_count, status, result, created_at)
select gen_random_uuid(), sqlc.arg(company_id), sqlc.narg(uploaded_by), sqlc.arg(file_name),
       sqlc.arg(row_count), sqlc.arg(status), sqlc.narg(result), sqlc.arg(created_at)
where sqlc.narg(uploaded_by)::uuid is null or exists (
  select 1 from users u where u.id = sqlc.narg(uploaded_by) and u.company_id = sqlc.arg(company_id)
)
returning *;

-- name: IcmalImportGet :one
select * from icmal_imports where id = sqlc.arg(id) and company_id = sqlc.arg(company_id);

-- name: IcmalImportList :many
select * from icmal_imports
where company_id = sqlc.arg(company_id)
  and (sqlc.narg(status)::text is null or status = sqlc.narg(status))
  and (sqlc.narg(range_from)::timestamptz is null or created_at >= sqlc.narg(range_from))
  and (sqlc.narg(range_to)::timestamptz is null or created_at < sqlc.narg(range_to))
order by created_at desc, id
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- name: IcmalImportUpdateResult :one
update icmal_imports set status = sqlc.arg(status), result = sqlc.arg(result)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
returning *;

-- name: IcmalImportVisible :one
select exists (
  select 1 from icmal_imports where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
) as visible;

-- name: IcmalUploaderVisible :one
select exists (
  select 1 from users where id = sqlc.arg(user_id) and company_id = sqlc.arg(company_id)
) as visible;

-- Important Finding 1: icmal_rows has no company_id of its own, and the
-- Go-side IcmalImportVisible/IcmalVisibleBuildingCount checks that guard
-- InsertRows run OUTSIDE this statement, before the transaction the loop
-- inserts inside even begins. Re-validating both the parent import AND (when
-- set) the row's own building here means the guard holds even if the Go-side
-- pre-checks were ever skipped.
-- name: IcmalRowInsert :one
insert into icmal_rows (
  id, import_id, building_id, period, etso_code, total_kwh, t0_kwh, t1_kwh, t2_kwh, t3_kwh,
  energy_charge, distribution_charge, reactive_charge, power_charge, overuse_charge,
  inductive_kvarh, capacitive_kvarh, demand_kw, vat_base, vat, btv, energy_fund, trt,
  price_difference, correction_amount, is_cancelled, term, voltage_level, is_multi_time, raw
)
select
  gen_random_uuid(), sqlc.arg(import_id), sqlc.narg(building_id), sqlc.arg(period), sqlc.narg(etso_code),
  sqlc.narg(total_kwh), sqlc.narg(t0_kwh), sqlc.narg(t1_kwh), sqlc.narg(t2_kwh), sqlc.narg(t3_kwh),
  sqlc.narg(energy_charge), sqlc.narg(distribution_charge), sqlc.narg(reactive_charge), sqlc.narg(power_charge),
  sqlc.narg(overuse_charge), sqlc.narg(inductive_kvarh), sqlc.narg(capacitive_kvarh), sqlc.narg(demand_kw),
  sqlc.narg(vat_base), sqlc.narg(vat), sqlc.narg(btv), sqlc.narg(energy_fund), sqlc.narg(trt),
  sqlc.narg(price_difference), sqlc.narg(correction_amount), sqlc.arg(is_cancelled), sqlc.narg(term),
  sqlc.narg(voltage_level), sqlc.narg(is_multi_time), sqlc.narg(raw)
where exists (
    select 1 from icmal_imports i where i.id = sqlc.arg(import_id) and i.company_id = sqlc.arg(company_id)
  )
  and (sqlc.narg(building_id)::uuid is null or exists (
    select 1 from buildings b
    where b.id = sqlc.narg(building_id) and b.company_id = sqlc.arg(company_id) and b.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
  ))
returning *;

-- name: IcmalRowList :many
select ir.* from icmal_rows ir
where ir.import_id = sqlc.arg(import_id)
  and exists (select 1 from icmal_imports i where i.id = ir.import_id and i.company_id = sqlc.arg(company_id))
order by ir.id
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- name: IcmalVisibleBuildingCount :one
select count(*) from buildings
where company_id = sqlc.arg(company_id) and deleted_at is null
  and id = any(sqlc.arg(check_ids)::uuid[])
  and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]));

-- tariff_extra_charges has no company_id: every statement re-validates the
-- tariff against the Scope, like tariff_taxes.
-- name: TariffExtraChargeList :many
select tec.* from tariff_extra_charges tec
where tec.tariff_id = sqlc.arg(tariff_id)
  and exists (
    select 1 from tariffs t where t.id = tec.tariff_id and t.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or t.building_id = any(sqlc.arg(building_ids)::uuid[]))
      and t.deleted_at is null
  )
order by tec.sort_order, tec.id;

-- name: TariffExtraChargeDeleteForTariff :exec
delete from tariff_extra_charges
where tariff_id = sqlc.arg(tariff_id)
  and exists (
    select 1 from tariffs t where t.id = sqlc.arg(tariff_id) and t.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or t.building_id = any(sqlc.arg(building_ids)::uuid[]))
      and t.deleted_at is null
  );

-- name: TariffExtraChargeInsert :one
insert into tariff_extra_charges (id, tariff_id, name, basis, amount, sort_order)
select gen_random_uuid(), sqlc.arg(tariff_id), sqlc.arg(name), sqlc.arg(basis), sqlc.arg(amount), sqlc.arg(sort_order)
where exists (
  select 1 from tariffs t where t.id = sqlc.arg(tariff_id) and t.company_id = sqlc.arg(company_id)
    and (sqlc.arg(all_buildings)::boolean or t.building_id = any(sqlc.arg(building_ids)::uuid[]))
    and t.deleted_at is null
)
returning *;
