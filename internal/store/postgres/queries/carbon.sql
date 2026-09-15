-- Carbon repository queries (migration 00007). Owned by Task 11b.
--
-- emission_factors' natural key is (coalesce(company_id, zero uuid), key) --
-- the unique index from migration 00007. CarbonUpsertFactor and, in
-- admin/catalogue.go, AdminUpsertPlatformFactor both target it with a single
-- atomic INSERT ... ON CONFLICT ... DO UPDATE, never a separate
-- check-then-insert.
--
-- CarbonUpsertFactor additionally guards against a caller-supplied id
-- belonging to another company's (or the platform's) factor: the
-- "where target.ok" clause makes the INSERT select zero rows in that case,
-- so the query returns no rows and the repository translates that into
-- ErrNotFound, atomically with the write itself.

-- name: CarbonGetFactor :one
select * from emission_factors
where id = $1
  and (company_id is null or company_id = $2);

-- name: CarbonListFactors :many
select * from emission_factors
where (company_id = sqlc.arg(company_id)::uuid
       or (sqlc.arg(include_platform)::boolean and company_id is null))
  and (sqlc.narg(keys)::text[] is null or cardinality(sqlc.narg(keys)::text[]) = 0
       or key = any(sqlc.narg(keys)::text[]))
  and (sqlc.narg(main_category)::text is null or main_category = sqlc.narg(main_category)::text)
  and (sqlc.narg(scope)::carbon_scope is null or scope = sqlc.narg(scope)::carbon_scope)
order by company_id nulls first, key
limit sqlc.arg(limit_val)::int offset sqlc.arg(offset_val)::int;

-- CarbonFactorOwnerForShare locks the parent row INSIDE the caller's
-- transaction for ReplaceConversions, so the ownership check and the
-- delete+insert that follows it see a consistent row and cannot race a
-- concurrent write to the same factor. The company_id predicate is IN THE
-- LOCK QUERY ITSELF: a platform factor or another company's factor matches
-- zero rows here, so the caller never reaches delete/insert at all — the
-- refusal does not depend on a Go-level equality check on a returned value
-- (fix round 1, Important 2).
-- name: CarbonFactorOwnerForShare :one
select id from emission_factors
where id = sqlc.arg(id)::uuid and company_id = sqlc.arg(company_id)::uuid
for share;

-- CarbonBuildingVisibleForShare is CarbonFactorOwnerForShare's counterpart
-- for ReplaceSelectedActivities: it locks the building row so the visibility
-- check and the delete+insert that follows are atomic together.
-- name: CarbonBuildingVisibleForShare :one
select id from buildings
where id = sqlc.arg(building_id)::uuid
  and company_id = sqlc.arg(company_id)::uuid
  and deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]))
for share;

-- name: CarbonUpsertFactor :one
with target as (
    select
        coalesce(nullif(sqlc.arg(id)::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
                 gen_random_uuid()) as id,
        not exists (
            select 1 from emission_factors ef
            where ef.id = sqlc.arg(id)::uuid
              and ef.company_id is distinct from sqlc.arg(company_id)::uuid
        ) as ok
)
insert into emission_factors
    (id, company_id, key, label, main_category, sub_categories, category_path,
     base_factor, base_unit, fuel_type, vehicle_type, scope, iso_category,
     status, source, source_year, source_url)
select
    target.id, sqlc.arg(company_id)::uuid, sqlc.arg(key), sqlc.arg(label),
    sqlc.arg(main_category), sqlc.arg(sub_categories)::text[], sqlc.arg(category_path)::text[],
    sqlc.arg(base_factor)::numeric, sqlc.arg(base_unit), sqlc.narg(fuel_type)::text,
    sqlc.narg(vehicle_type)::text, sqlc.narg(scope)::carbon_scope, sqlc.narg(iso_category)::text,
    sqlc.narg(status)::text, sqlc.narg(source)::text, sqlc.narg(source_year)::smallint,
    sqlc.narg(source_url)::text
from target
where target.ok
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

-- CarbonListConversions is the SINGLE parent-driven statement Conversions()
-- runs (fix round 1, Important 3): it starts from emission_factors, not
-- emission_factor_conversions, and LEFT JOINs the (parentless, company_id-
-- free) child table, so a visible factor with zero conversions still
-- returns exactly one row (unit/multiplier/label all null — the sentinel
-- the repository turns into an empty slice), while an invisible or
-- nonexistent factor id returns ZERO rows (ErrNotFound). There is no
-- separate Go-level pre-check query any more: this one statement's own
-- `(f.company_id is null or f.company_id = $2)` predicate is the only thing
-- standing between a caller and another company's conversions, so
-- tautologising it is directly test-visible.
-- name: CarbonListConversions :many
select f.id as factor_id, c.unit, c.multiplier, c.label
from emission_factors f
left join emission_factor_conversions c on c.factor_id = f.id
where f.id = sqlc.arg(factor_id)::uuid
  and (f.company_id is null or f.company_id = sqlc.arg(company_id)::uuid)
order by c.unit;

-- CarbonDeleteConversions and CarbonInsertConversions both carry the
-- company_id predicate directly, joined through emission_factors, rather
-- than trusting the caller (ReplaceConversions) to have already verified
-- ownership in Go: even if that Go-level check were bypassed, neither
-- statement can touch a row it does not own (fix round 1, Important 2).
-- name: CarbonDeleteConversions :exec
delete from emission_factor_conversions c
using emission_factors f
where f.id = c.factor_id
  and c.factor_id = sqlc.arg(factor_id)::uuid
  and f.company_id = sqlc.arg(company_id)::uuid;

-- CarbonInsertConversions takes the whole batch as one jsonb array: sqlc's
-- (deliberately minimal) built-in catalogue has no multi-array unnest
-- overload, so a single jsonb parameter unpacked with jsonb_array_elements
-- and read back with `elem->>'field'` plus a per-column cast is this
-- codebase's bulk-insert idiom for a parallel-column child collection (see
-- timescale-shims.sql's jsonb_array_elements declaration for why this shape,
-- and not jsonb_to_recordset's `AS t(col type, ...)`). The insert selects
-- `f.id`, not the raw `$1` parameter, and f is filtered to
-- (id, company_id): a batch cannot land under a factor_id/company_id pair
-- that does not jointly identify a real, owned row.
-- name: CarbonInsertConversions :exec
insert into emission_factor_conversions (factor_id, unit, multiplier, label)
select f.id, elem->>'unit', (elem->>'multiplier')::numeric, elem->>'label'
from jsonb_array_elements(sqlc.arg(conversions)::jsonb) as elem, emission_factors f
where f.id = sqlc.arg(factor_id)::uuid and f.company_id = sqlc.arg(company_id)::uuid;

-- The scope's building narrowing is embedded here rather than checked with
-- Scope.AllowsBuilding in Go: AllowsBuilding returns true for ANY id under
-- AllBuildings, including another tenant's, so it must never be the thing
-- standing between a caller and a row — only a predicate inside the query
-- itself does that reliably.
-- name: CarbonListSelectedActivities :many
select csa.* from carbon_selected_activities csa
join buildings b on b.id = csa.building_id
where csa.company_id = sqlc.arg(company_id)::uuid
  and csa.building_id = sqlc.arg(building_id)::uuid
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or csa.building_id = any(sqlc.arg(building_ids)::uuid[]))
order by csa.activity_key;

-- name: CarbonDeleteSelectedActivities :exec
delete from carbon_selected_activities where company_id = $1 and building_id = $2;

-- The alias is a bare name, not a parenthesised column list: unnest's
-- single-array-argument form returns exactly one column, and aliasing it as
-- `as activity_key` (rather than `as t(activity_key)`) means the alias
-- itself is never followed by "(" — see timescale-shims.sql's
-- jsonb_array_elements declaration for why that distinction matters here.
-- name: CarbonInsertSelectedActivities :exec
insert into carbon_selected_activities (company_id, building_id, activity_key)
select $1, $2, activity_key from unnest(sqlc.arg(keys)::text[]) as activity_key;

-- name: CarbonGetActivity :one
select ca.* from carbon_activities ca
join buildings b on b.id = ca.building_id
where ca.id = $1
  and ca.company_id = $2
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or ca.building_id = any(sqlc.arg(building_ids)::uuid[]));

-- name: CarbonListActivities :many
select ca.* from carbon_activities ca
join buildings b on b.id = ca.building_id
where ca.company_id = sqlc.arg(company_id)::uuid
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or ca.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and (sqlc.narg(filter_building_ids)::uuid[] is null or cardinality(sqlc.narg(filter_building_ids)::uuid[]) = 0
       or ca.building_id = any(sqlc.narg(filter_building_ids)::uuid[]))
  -- scopes/statuses are text[], not carbon_scope[]/carbon_status[]: pgx has
  -- no array codec for a custom enum type without explicit registration, so
  -- the comparison casts the column to text instead of typing the parameter
  -- array as the enum.
  and (sqlc.narg(scopes)::text[] is null or cardinality(sqlc.narg(scopes)::text[]) = 0
       or ca.scope::text = any(sqlc.narg(scopes)::text[]))
  and (sqlc.narg(statuses)::text[] is null or cardinality(sqlc.narg(statuses)::text[]) = 0
       or ca.status::text = any(sqlc.narg(statuses)::text[]))
  and (sqlc.narg(activity_type)::text is null or ca.activity_type = sqlc.narg(activity_type)::text)
  and (sqlc.narg(period_from)::date is null or ca.period_start >= sqlc.narg(period_from)::date)
  and (sqlc.narg(period_to)::date is null or ca.period_end <= sqlc.narg(period_to)::date)
  and (sqlc.narg(is_automated)::boolean is null or ca.is_automated = sqlc.narg(is_automated)::boolean)
order by ca.period_start desc, ca.id
limit sqlc.arg(limit_val)::int offset sqlc.arg(offset_val)::int;

-- name: CarbonCreateActivity :one
insert into carbon_activities
    (id, company_id, building_id, main_category, sub_category, activity_type,
     period_start, period_end, quantity, unit, factor_id, factor_key, factor_value,
     conversion_multiplier, emission_kgco2e, scope, iso_category, description,
     details, status, is_automated, created_by)
select
    coalesce(nullif(sqlc.arg(id)::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
             gen_random_uuid()),
    sqlc.arg(company_id)::uuid, sqlc.arg(building_id)::uuid, sqlc.arg(main_category),
    sqlc.arg(sub_category), sqlc.arg(activity_type), sqlc.arg(period_start)::date,
    sqlc.arg(period_end)::date, sqlc.arg(quantity)::numeric, sqlc.arg(unit),
    sqlc.narg(factor_id)::uuid, sqlc.narg(factor_key)::text, sqlc.narg(factor_value)::numeric,
    sqlc.arg(conversion_multiplier)::numeric, sqlc.arg(emission_kgco2e)::numeric,
    sqlc.arg(scope)::carbon_scope, sqlc.arg(iso_category), sqlc.narg(description)::text,
    sqlc.narg(details)::jsonb, sqlc.arg(status)::carbon_status, false, sqlc.narg(created_by)::uuid
where exists (
    select 1 from buildings b
    where b.id = sqlc.arg(building_id)::uuid
      and b.company_id = sqlc.arg(company_id)::uuid
      and b.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
)
and (
    sqlc.narg(created_by)::uuid is null
    or exists (
        select 1 from users u
        where u.id = sqlc.narg(created_by)::uuid
          and u.company_id = sqlc.arg(company_id)::uuid
          and u.deleted_at is null
    )
)
-- factor_id is a stored FK (fix round 1, Important 1, R1): null (no
-- factor — a manual activity), a platform factor, or one of the caller's
-- own is allowed; another company's factor id is refused, atomically with
-- the write.
and (
    sqlc.narg(factor_id)::uuid is null
    or exists (
        select 1 from emission_factors f
        where f.id = sqlc.narg(factor_id)::uuid
          and (f.company_id is null or f.company_id = sqlc.arg(company_id)::uuid)
    )
)
returning *;

-- name: CarbonUpdateActivity :one
-- building_id is deliberately not in the SET list: as with
-- ISO50001Repository.UpdateNote's project_id, an activity stays under the
-- building it was created in, so an update cannot move it into a building
-- outside the caller's scope.
update carbon_activities set
    main_category = sqlc.arg(main_category),
    sub_category = sqlc.arg(sub_category),
    activity_type = sqlc.arg(activity_type),
    period_start = sqlc.arg(period_start)::date,
    period_end = sqlc.arg(period_end)::date,
    quantity = sqlc.arg(quantity)::numeric,
    unit = sqlc.arg(unit),
    factor_id = sqlc.narg(factor_id)::uuid,
    factor_key = sqlc.narg(factor_key)::text,
    factor_value = sqlc.narg(factor_value)::numeric,
    conversion_multiplier = sqlc.arg(conversion_multiplier)::numeric,
    emission_kgco2e = sqlc.arg(emission_kgco2e)::numeric,
    scope = sqlc.arg(scope)::carbon_scope,
    iso_category = sqlc.arg(iso_category),
    description = sqlc.narg(description)::text,
    details = sqlc.narg(details)::jsonb,
    status = sqlc.arg(status)::carbon_status,
    updated_at = now()
where carbon_activities.id = sqlc.arg(id)::uuid
  and carbon_activities.company_id = sqlc.arg(company_id)::uuid
  and exists (
      select 1 from buildings b
      where b.id = carbon_activities.building_id
        and b.deleted_at is null
        and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
  )
  -- factor_id is a stored FK (fix round 1, Important 1, R1): see
  -- CarbonCreateActivity's identical clause.
  and (
      sqlc.narg(factor_id)::uuid is null
      or exists (
          select 1 from emission_factors f
          where f.id = sqlc.narg(factor_id)::uuid
            and (f.company_id is null or f.company_id = sqlc.arg(company_id)::uuid)
      )
  )
returning *;

-- name: CarbonDeleteActivity :execrows
delete from carbon_activities
where id = sqlc.arg(id)::uuid
  and company_id = sqlc.arg(company_id)::uuid
  and exists (
      select 1 from buildings b
      where b.id = carbon_activities.building_id
        and b.deleted_at is null
        and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
  );

-- name: CarbonUpsertAutomatedActivity :one
-- Keyed on the partial unique index (building_id, activity_type,
-- period_start) where is_automated. is_automated is hard-coded true: this
-- path exists only for the derived-from-meter-data recomputation.
insert into carbon_activities
    (id, company_id, building_id, main_category, sub_category, activity_type,
     period_start, period_end, quantity, unit, factor_id, factor_key, factor_value,
     conversion_multiplier, emission_kgco2e, scope, iso_category, description,
     details, status, is_automated, created_by)
select
    gen_random_uuid(),
    sqlc.arg(company_id)::uuid, sqlc.arg(building_id)::uuid, sqlc.arg(main_category),
    sqlc.arg(sub_category), sqlc.arg(activity_type), sqlc.arg(period_start)::date,
    sqlc.arg(period_end)::date, sqlc.arg(quantity)::numeric, sqlc.arg(unit),
    sqlc.narg(factor_id)::uuid, sqlc.narg(factor_key)::text, sqlc.narg(factor_value)::numeric,
    sqlc.arg(conversion_multiplier)::numeric, sqlc.arg(emission_kgco2e)::numeric,
    sqlc.arg(scope)::carbon_scope, sqlc.arg(iso_category), sqlc.narg(description)::text,
    sqlc.narg(details)::jsonb, sqlc.arg(status)::carbon_status, true, sqlc.narg(created_by)::uuid
where exists (
    select 1 from buildings b
    where b.id = sqlc.arg(building_id)::uuid
      and b.company_id = sqlc.arg(company_id)::uuid
      and b.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
)
and (
    sqlc.narg(created_by)::uuid is null
    or exists (
        select 1 from users u
        where u.id = sqlc.narg(created_by)::uuid
          and u.company_id = sqlc.arg(company_id)::uuid
          and u.deleted_at is null
    )
)
-- factor_id is a stored FK (fix round 1, Important 1, R1): see
-- CarbonCreateActivity's identical clause.
and (
    sqlc.narg(factor_id)::uuid is null
    or exists (
        select 1 from emission_factors f
        where f.id = sqlc.narg(factor_id)::uuid
          and (f.company_id is null or f.company_id = sqlc.arg(company_id)::uuid)
    )
)
on conflict (building_id, activity_type, period_start) where is_automated
do update set
    main_category = excluded.main_category,
    sub_category = excluded.sub_category,
    quantity = excluded.quantity,
    unit = excluded.unit,
    factor_id = excluded.factor_id,
    factor_key = excluded.factor_key,
    factor_value = excluded.factor_value,
    conversion_multiplier = excluded.conversion_multiplier,
    emission_kgco2e = excluded.emission_kgco2e,
    scope = excluded.scope,
    iso_category = excluded.iso_category,
    description = excluded.description,
    details = excluded.details,
    status = excluded.status,
    updated_at = now()
returning *;

-- name: CarbonSetActivityStatus :one
update carbon_activities set status = sqlc.arg(status)::carbon_status, updated_at = sqlc.arg(at)::timestamptz
where id = sqlc.arg(id)::uuid
  and company_id = sqlc.arg(company_id)::uuid
  and exists (
      select 1 from buildings b
      where b.id = carbon_activities.building_id
        and b.deleted_at is null
        and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
  )
returning *;

-- name: CarbonCreateReport :one
insert into carbon_reports (id, company_id, building_id, name, report_type, period, payload, pdf_path)
select
    coalesce(nullif(sqlc.arg(id)::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
             gen_random_uuid()),
    sqlc.arg(company_id)::uuid, sqlc.arg(building_id)::uuid, sqlc.arg(name),
    sqlc.arg(report_type), sqlc.arg(period), sqlc.arg(payload)::jsonb, sqlc.narg(pdf_path)::text
where exists (
    select 1 from buildings b
    where b.id = sqlc.arg(building_id)::uuid
      and b.company_id = sqlc.arg(company_id)::uuid
      and b.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
)
returning *;

-- name: CarbonListReports :many
select cr.* from carbon_reports cr
join buildings b on b.id = cr.building_id
where cr.company_id = sqlc.arg(company_id)::uuid
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or cr.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and (sqlc.narg(building_id)::uuid is null or cr.building_id = sqlc.narg(building_id)::uuid)
order by cr.created_at desc, cr.id
limit sqlc.arg(limit_val)::int offset sqlc.arg(offset_val)::int;
