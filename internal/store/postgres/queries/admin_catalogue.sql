-- AdminCatalogueRepository queries (store.AdminCatalogueRepository). Owned by
-- Task 11b, called only from internal/store/postgres/admin/catalogue.go.
--
-- Every write here is platform-wide and unscoped by design (see
-- repository.go's AdminCatalogueRepository doc). national_tariff_schedule and
-- integration_definitions have no company_id at all, so nothing to guard;
-- emission_factors and emission_factor_conversions hold both platform and
-- company-owned rows, so these two queries are careful to touch ONLY a
-- platform row (company_id null), in one atomic statement, never a
-- company-owned one — see the doc comments below.

-- One jsonb array parameter, unpacked with jsonb_array_elements: see
-- carbon.sql's CarbonInsertConversions for why (no multi-array unnest in
-- sqlc's catalogue, and jsonb_to_recordset's column-list alias trips
-- TestEveryFunctionSQLcMustTypeIsDeclared's relation/call scan).
-- name: AdminUpsertNationalTariffSchedule :execrows
insert into national_tariff_schedule
    (id, effective_from, user_group, voltage_level, term, energy_price, t1_price, t2_price,
     t3_price, distribution_price, power_price, overuse_price, daily_threshold_kwh, vat_rate, source)
select
    coalesce(nullif((elem->>'id')::uuid, '00000000-0000-0000-0000-000000000000'::uuid), gen_random_uuid()),
    (elem->>'effective_from')::date,
    (elem->>'user_group')::distribution_user_group,
    (elem->>'voltage_level')::voltage_level,
    (elem->>'term')::tariff_term,
    (elem->>'energy_price')::numeric,
    (elem->>'t1_price')::numeric,
    (elem->>'t2_price')::numeric,
    (elem->>'t3_price')::numeric,
    (elem->>'distribution_price')::numeric,
    (elem->>'power_price')::numeric,
    (elem->>'overuse_price')::numeric,
    (elem->>'daily_threshold_kwh')::numeric,
    (elem->>'vat_rate')::numeric,
    elem->>'source'
from jsonb_array_elements(sqlc.arg(entries)::jsonb) as elem
on conflict (effective_from, user_group, voltage_level, term) do update set
    energy_price = excluded.energy_price,
    t1_price = excluded.t1_price,
    t2_price = excluded.t2_price,
    t3_price = excluded.t3_price,
    distribution_price = excluded.distribution_price,
    power_price = excluded.power_price,
    overuse_price = excluded.overuse_price,
    daily_threshold_kwh = excluded.daily_threshold_kwh,
    vat_rate = excluded.vat_rate,
    source = excluded.source;

-- AdminUpsertPlatformFactor writes ONLY a platform factor: company_id is
-- hard-coded null in the inserted row (never taken from a parameter), so the
-- (coalesce(company_id, zero uuid), key) conflict target can only ever match
-- another company_id-null row. A company-owned factor sharing the same key
-- lives at a different coalesce value and is untouched.
-- name: AdminUpsertPlatformFactor :one
insert into emission_factors
    (id, company_id, key, label, main_category, sub_categories, category_path,
     base_factor, base_unit, fuel_type, vehicle_type, scope, iso_category,
     status, source, source_year, source_url)
values (
    coalesce(nullif(sqlc.arg(id)::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
             gen_random_uuid()),
    null, sqlc.arg(key), sqlc.arg(label), sqlc.arg(main_category),
    sqlc.arg(sub_categories)::text[], sqlc.arg(category_path)::text[],
    sqlc.arg(base_factor)::numeric, sqlc.arg(base_unit), sqlc.narg(fuel_type)::text,
    sqlc.narg(vehicle_type)::text, sqlc.narg(scope)::carbon_scope, sqlc.narg(iso_category)::text,
    sqlc.narg(status)::text, sqlc.narg(source)::text, sqlc.narg(source_year)::smallint,
    sqlc.narg(source_url)::text
)
on conflict (coalesce(company_id, '00000000-0000-0000-0000-000000000000'::uuid), key)
do update set
    label = excluded.label,
    main_category = excluded.main_category,
    sub_categories = excluded.sub_categories,
    category_path = excluded.category_path,
    base_factor = excluded.base_factor,
    base_unit = excluded.base_unit,
    fuel_type = excluded.fuel_type,
    vehicle_type = excluded.vehicle_type,
    scope = excluded.scope,
    iso_category = excluded.iso_category,
    status = excluded.status,
    source = excluded.source,
    source_year = excluded.source_year,
    source_url = excluded.source_url,
    updated_at = now()
returning *;

-- AdminPlatformFactorOwnerForShare locks the factor row inside
-- ReplacePlatformConversions' transaction and reports whether it is a
-- PLATFORM factor (company_id is null). A company-owned factor's id, or one
-- that does not exist, yields no row: the repository returns ErrNotFound and
-- replaces nothing.
-- name: AdminPlatformFactorOwnerForShare :one
select id from emission_factors where id = $1 and company_id is null for share;

-- name: AdminDeleteFactorConversions :exec
delete from emission_factor_conversions where factor_id = $1;

-- name: AdminInsertFactorConversions :exec
insert into emission_factor_conversions (factor_id, unit, multiplier, label)
select $1, elem->>'unit', (elem->>'multiplier')::numeric, elem->>'label'
from jsonb_array_elements(sqlc.arg(conversions)::jsonb) as elem;

-- One jsonb array parameter, unpacked with jsonb_array_elements: see
-- carbon.sql's CarbonInsertConversions for why (no multi-array unnest in
-- sqlc's catalogue, and jsonb_to_recordset's column-list alias trips
-- TestEveryFunctionSQLcMustTypeIsDeclared's relation/call scan).
-- name: AdminUpsertIntegrationDefinitions :execrows
insert into integration_definitions (id, provider, subtype, endpoints)
select coalesce(nullif((elem->>'id')::uuid, '00000000-0000-0000-0000-000000000000'::uuid), gen_random_uuid()),
       (elem->>'provider')::integration_provider, elem->>'subtype', elem->'endpoints'
from jsonb_array_elements(sqlc.arg(defs)::jsonb) as elem
on conflict (provider, subtype) do update set
    endpoints = excluded.endpoints,
    updated_at = now();

-- billing_parameters is platform-wide and dated (R106); keyed on effective_from.
-- name: AdminUpsertBillingParameters :one
insert into billing_parameters (
  effective_from, reactive_penalty_basis, reactive_exempt_below_kw, reactive_exempt_terms,
  reactive_exempt_user_groups, reactive_generation_exempt_kwh, reactive_bands, tiering_groups,
  tiering_mode, tiering_voltage_levels, tiering_supply_companies, ptf_missing_hour_tolerance,
  demand_overrun_multiplier, money_rounding_mode
) values (
  sqlc.arg(effective_from), sqlc.arg(reactive_penalty_basis), sqlc.narg(reactive_exempt_below_kw),
  sqlc.arg(reactive_exempt_terms)::text[]::tariff_term[],
  sqlc.arg(reactive_exempt_user_groups)::text[]::distribution_user_group[],
  sqlc.arg(reactive_generation_exempt_kwh), sqlc.arg(reactive_bands), sqlc.arg(tiering_groups),
  sqlc.arg(tiering_mode), sqlc.arg(tiering_voltage_levels)::text[]::voltage_level[],
  sqlc.arg(tiering_supply_companies)::text[]::supply_company[],
  sqlc.arg(ptf_missing_hour_tolerance), sqlc.arg(demand_overrun_multiplier), sqlc.arg(money_rounding_mode)
)
on conflict (effective_from) do update set
  reactive_penalty_basis = excluded.reactive_penalty_basis,
  reactive_exempt_below_kw = excluded.reactive_exempt_below_kw,
  reactive_exempt_terms = excluded.reactive_exempt_terms,
  reactive_exempt_user_groups = excluded.reactive_exempt_user_groups,
  reactive_generation_exempt_kwh = excluded.reactive_generation_exempt_kwh,
  reactive_bands = excluded.reactive_bands,
  tiering_groups = excluded.tiering_groups,
  tiering_mode = excluded.tiering_mode,
  tiering_voltage_levels = excluded.tiering_voltage_levels,
  tiering_supply_companies = excluded.tiering_supply_companies,
  ptf_missing_hour_tolerance = excluded.ptf_missing_hour_tolerance,
  demand_overrun_multiplier = excluded.demand_overrun_multiplier,
  money_rounding_mode = excluded.money_rounding_mode
returning effective_from, reactive_penalty_basis, reactive_exempt_below_kw,
  reactive_exempt_terms::text[] as reactive_exempt_terms,
  reactive_exempt_user_groups::text[] as reactive_exempt_user_groups,
  reactive_generation_exempt_kwh, reactive_bands, tiering_groups, tiering_mode,
  tiering_voltage_levels::text[] as tiering_voltage_levels,
  tiering_supply_companies::text[] as tiering_supply_companies,
  ptf_missing_hour_tolerance, demand_overrun_multiplier, money_rounding_mode, created_at;
