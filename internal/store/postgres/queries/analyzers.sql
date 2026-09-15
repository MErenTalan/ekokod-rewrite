-- AnalyzerRepository queries.
--
-- Every read below shares one visibility predicate: an analyzer is visible to
-- a Scope when all_buildings is true (whatever building_id holds, including
-- NULL), or when building_id is not null and is one of building_ids. That
-- single formula is also what makes AnalyzerFilter.Unassigned fail closed for
-- a narrow Scope with no extra code: a NULL building_id can never be `any`
-- of a concrete list, so adding "and building_id is null" for Unassigned
-- combines with the predicate to select nothing unless all_buildings is set.

-- name: AnalyzerGet :one
select * from analyzers
where id = sqlc.arg(id)
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean
       or (building_id is not null and building_id = any(sqlc.arg(building_ids)::uuid[])))
  and deleted_at is null;

-- name: AnalyzerGetByInstallation :one
select * from analyzers
where provider = sqlc.arg(provider)
  and provider_subtype = sqlc.arg(provider_subtype)
  and installation_number = sqlc.arg(installation_number)
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean
       or (building_id is not null and building_id = any(sqlc.arg(building_ids)::uuid[])))
  and deleted_at is null;

-- name: AnalyzerList :many
select * from analyzers
where company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean
       or (building_id is not null and building_id = any(sqlc.arg(building_ids)::uuid[])))
  and (cardinality(sqlc.arg(filter_ids)::uuid[]) = 0 or id = any(sqlc.arg(filter_ids)::uuid[]))
  and (sqlc.narg(filter_building_id)::uuid is null or building_id = sqlc.narg(filter_building_id))
  and (not sqlc.arg(unassigned)::boolean or building_id is null)
  -- The PARAMETER is cast text[] -> integration_provider[]; the COLUMN is
  -- left uncast. See UserList's comment on the roles filter in
  -- queries/users.sql for why this order matters: providers leads
  -- `analyzers (provider, provider_subtype, installation_number)`, and
  -- casting the column itself would defeat that index on every list call.
  and (cardinality(sqlc.arg(providers)::text[]) = 0
       or provider = any(sqlc.arg(providers)::text[]::integration_provider[]))
  and (sqlc.narg(is_active)::boolean is null or is_active = sqlc.narg(is_active))
  and (sqlc.arg(include_deleted)::boolean or deleted_at is null)
order by installation_number
limit sqlc.arg(page_limit) offset sqlc.arg(page_offset);

-- name: AnalyzerCreate :one
-- coalesce(sqlc.narg(at)::timestamptz, now()): a caller that leaves CreatedAt at its zero
-- value gets the database's own now() rather than writing 0001-01-01.
--
-- CONTROLLER RULING (fix round 1, second dispatch): a stored foreign key
-- must be validated IN SQL, inside the write statement — never by a
-- Go-level check alone. A sibling task's AdminScope write stored another
-- tenant's building id because its Go-level check used
-- Scope.AllowsBuilding(), which returns true for ANY id under AllBuildings
-- with no company_id check at all. The WHERE clause below embeds BOTH the
-- building_id FK check (company_id, the Scope's building branch, and
-- deleted_at) AND the folded-minor rule that a nil building_id may only be
-- written under an AllBuildings Scope, so this insert cannot write a
-- building_id this exact Scope could not itself see.
insert into analyzers
    (id, company_id, building_id, provider, provider_subtype, installation_number,
     customer_name, address, province, district, neighbourhood, street,
     tariff_type, tariff_kind, installation_kind, installed_power_kw, meter_number,
     meter_model, meter_multiplier, counterparty_no, metering_point_name,
     latitude, longitude, etso_code, definition_type, is_active, created_at, updated_at)
select sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(building_id), sqlc.arg(provider),
       sqlc.arg(provider_subtype), sqlc.arg(installation_number), sqlc.arg(customer_name),
       sqlc.arg(address), sqlc.arg(province), sqlc.arg(district), sqlc.arg(neighbourhood),
       sqlc.arg(street), sqlc.arg(tariff_type), sqlc.arg(tariff_kind), sqlc.arg(installation_kind),
       sqlc.arg(installed_power_kw), sqlc.arg(meter_number), sqlc.arg(meter_model),
       sqlc.arg(meter_multiplier), sqlc.arg(counterparty_no), sqlc.arg(metering_point_name),
       sqlc.arg(latitude), sqlc.arg(longitude), sqlc.arg(etso_code), sqlc.arg(definition_type),
       sqlc.arg(is_active), coalesce(sqlc.narg(at)::timestamptz, now()), coalesce(sqlc.narg(at)::timestamptz, now())
where (
    (sqlc.arg(building_id)::uuid is null and sqlc.arg(all_buildings)::boolean)
    or (
        sqlc.arg(building_id)::uuid is not null
        and exists (
            select 1 from buildings b
            where b.id = sqlc.arg(building_id)
              and b.company_id = sqlc.arg(company_id)
              and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
              and b.deleted_at is null
        )
    )
)
returning *;

-- name: AnalyzerUpdate :one
-- Isolation, two DIFFERENT building_id checks in one statement: the WHERE
-- clause's bare `building_id` (no alias — it names THIS table, the row's
-- CURRENT, already-stored value) gates whether the row itself is visible to
-- the Scope, exactly like every other scoped method; the embedded exists(...)
-- clause below additionally validates sqlc.arg(building_id), the NEW value
-- this statement is about to WRITE, the same way AnalyzerCreate does — see
-- its comment. Before this fix round, only the row's current value was
-- checked in SQL, and the new value was validated solely by a Go-level
-- pre-check the SQL itself did not enforce.
update analyzers
set building_id = sqlc.arg(building_id),
    customer_name = sqlc.arg(customer_name),
    address = sqlc.arg(address),
    province = sqlc.arg(province),
    district = sqlc.arg(district),
    neighbourhood = sqlc.arg(neighbourhood),
    street = sqlc.arg(street),
    tariff_type = sqlc.arg(tariff_type),
    tariff_kind = sqlc.arg(tariff_kind),
    installation_kind = sqlc.arg(installation_kind),
    installed_power_kw = sqlc.arg(installed_power_kw),
    meter_number = sqlc.arg(meter_number),
    meter_model = sqlc.arg(meter_model),
    meter_multiplier = sqlc.arg(meter_multiplier),
    counterparty_no = sqlc.arg(counterparty_no),
    metering_point_name = sqlc.arg(metering_point_name),
    latitude = sqlc.arg(latitude),
    longitude = sqlc.arg(longitude),
    etso_code = sqlc.arg(etso_code),
    definition_type = sqlc.arg(definition_type),
    is_active = sqlc.arg(is_active),
    updated_at = sqlc.arg(updated_at)
where analyzers.id = sqlc.arg(id)
  and analyzers.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean
       or (analyzers.building_id is not null and analyzers.building_id = any(sqlc.arg(building_ids)::uuid[])))
  and analyzers.deleted_at is null
  and (
      (sqlc.arg(building_id)::uuid is null and sqlc.arg(all_buildings)::boolean)
      or (
          sqlc.arg(building_id)::uuid is not null
          and exists (
              select 1 from buildings b
              where b.id = sqlc.arg(building_id)
                and b.company_id = sqlc.arg(company_id)
                and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
                and b.deleted_at is null
          )
      )
  )
returning *;

-- name: AnalyzerSoftDelete :execrows
update analyzers
set deleted_at = sqlc.arg(deleted_at)
where id = sqlc.arg(id)
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean
       or (building_id is not null and building_id = any(sqlc.arg(building_ids)::uuid[])))
  and deleted_at is null;

-- name: AnalyzerTouchLastReading :execrows
update analyzers
set last_reading_at = greatest(coalesce(last_reading_at, '-infinity'::timestamptz), sqlc.arg(at))
where id = sqlc.arg(id)
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean
       or (building_id is not null and building_id = any(sqlc.arg(building_ids)::uuid[])))
  and deleted_at is null;
