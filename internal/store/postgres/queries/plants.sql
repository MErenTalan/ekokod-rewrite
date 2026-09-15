-- PlantRepository queries.
--
-- power_plants has no building_id, so every predicate below stops at
-- company_id = the Scope's own: a Scope narrows plants to the company and no
-- further (repository.go's PlantRepository doc comment).

-- name: PlantGet :one
select * from power_plants
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- name: PlantList :many
select * from power_plants
where company_id = sqlc.arg(company_id)
  and (cardinality(sqlc.arg(filter_ids)::uuid[]) = 0 or id = any(sqlc.arg(filter_ids)::uuid[]))
  and (sqlc.narg(plant_kind)::text is null or plant_kind = sqlc.narg(plant_kind))
  and (sqlc.narg(isolar_linked)::boolean is null
       or (sqlc.narg(isolar_linked)::boolean and isolar_ps_id is not null)
       or (not sqlc.narg(isolar_linked)::boolean and isolar_ps_id is null))
  and (sqlc.arg(include_deleted)::boolean or deleted_at is null)
order by name
limit sqlc.arg(page_limit) offset sqlc.arg(page_offset);

-- name: PlantCreate :one
-- coalesce(sqlc.narg(at)::timestamptz, now()): a caller that leaves CreatedAt at its zero
-- value gets the database's own now() rather than writing 0001-01-01.
insert into power_plants
    (id, company_id, name, installation_number, plant_kind, pv_brand_model,
     panel_power_w, panel_efficiency_pct, panel_count, string_count, orientation,
     tilt_angle_deg, total_capacity_kw, yearly_target_kwh, installation_date,
     address, latitude, longitude, isolar_ps_id, isolar_ps_key, isolar_ps_name,
     isolar_installed_kw, isolar_linked_at, created_at, updated_at)
values (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(name), sqlc.arg(installation_number),
        sqlc.arg(plant_kind), sqlc.arg(pv_brand_model), sqlc.arg(panel_power_w),
        sqlc.arg(panel_efficiency_pct), sqlc.arg(panel_count), sqlc.arg(string_count),
        sqlc.arg(orientation), sqlc.arg(tilt_angle_deg), sqlc.arg(total_capacity_kw),
        sqlc.arg(yearly_target_kwh), sqlc.arg(installation_date), sqlc.arg(address),
        sqlc.arg(latitude), sqlc.arg(longitude), sqlc.arg(isolar_ps_id), sqlc.arg(isolar_ps_key),
        sqlc.arg(isolar_ps_name), sqlc.arg(isolar_installed_kw), sqlc.arg(isolar_linked_at),
        coalesce(sqlc.narg(at)::timestamptz, now()), coalesce(sqlc.narg(at)::timestamptz, now()))
returning *;

-- name: PlantUpdate :one
update power_plants
set name = sqlc.arg(name),
    installation_number = sqlc.arg(installation_number),
    plant_kind = sqlc.arg(plant_kind),
    pv_brand_model = sqlc.arg(pv_brand_model),
    panel_power_w = sqlc.arg(panel_power_w),
    panel_efficiency_pct = sqlc.arg(panel_efficiency_pct),
    panel_count = sqlc.arg(panel_count),
    string_count = sqlc.arg(string_count),
    orientation = sqlc.arg(orientation),
    tilt_angle_deg = sqlc.arg(tilt_angle_deg),
    total_capacity_kw = sqlc.arg(total_capacity_kw),
    yearly_target_kwh = sqlc.arg(yearly_target_kwh),
    installation_date = sqlc.arg(installation_date),
    address = sqlc.arg(address),
    latitude = sqlc.arg(latitude),
    longitude = sqlc.arg(longitude),
    isolar_ps_id = sqlc.arg(isolar_ps_id),
    isolar_ps_key = sqlc.arg(isolar_ps_key),
    isolar_ps_name = sqlc.arg(isolar_ps_name),
    isolar_installed_kw = sqlc.arg(isolar_installed_kw),
    isolar_linked_at = sqlc.arg(isolar_linked_at),
    updated_at = sqlc.arg(updated_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null
returning *;

-- name: PlantSoftDelete :execrows
update power_plants
set deleted_at = sqlc.arg(deleted_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- name: PlantGetForShare :one
-- Same predicate as PlantGet, but takes a FOR SHARE lock: used INSIDE each
-- Replace… method's transaction so a concurrent SoftDelete of the same plant
-- cannot race the delete+insert that follows it. This is not itself the
-- isolation boundary for the child tables below — each Delete/Insert query
-- is scoped on its own — it exists so an empty replacement list still has
-- something to return ErrNotFound from, since delete-then-insert-nothing
-- would otherwise run no query that could fail on a foreign or invisible
-- plant.
select * from power_plants
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null
for share;

-- name: PlantMonthlyTargetsList :many
-- Isolation: power_plant_monthly_targets has no company_id and is reached
-- ONLY by joining to power_plants IN THIS SAME QUERY — a LEFT JOIN, not a
-- pre-check run separately, so this predicate is what an isolation test
-- actually exercises.
--
-- A plant not visible to the Scope contributes ZERO rows: the caller
-- reports ErrNotFound. A visible plant with no targets yet contributes
-- EXACTLY ONE row with t.plant_id (and every other t.* column) NULL — the
-- sentinel the caller checks; a visible plant with targets contributes one
-- row per target, none of them NULL.
select t.plant_id, t.month, t.target_kwh
from power_plants p
left join power_plant_monthly_targets t on t.plant_id = p.id
where p.id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
order by t.month;

-- name: PlantMonthlyTargetsDelete :exec
-- Isolation: scoped via a join to power_plants IN THIS SAME STATEMENT (a
-- DELETE … USING), replacing what used to be a Go-level pre-check run on
-- the pool before the write. A foreign or otherwise invisible plant_id
-- deletes zero rows rather than every target under a well-known id.
delete from power_plant_monthly_targets t
using power_plants p
where t.plant_id = p.id
  and t.plant_id = sqlc.arg(plant_id)
  and p.company_id = sqlc.arg(company_id)
  and p.deleted_at is null;

-- name: PlantMonthlyTargetInsert :one
-- Isolation: the insert only runs when plant_id's parent is visible to the
-- Scope, checked via exists(...) IN THIS SAME STATEMENT. An invisible plant
-- inserts zero rows; the :one scan then reports ErrNoRows, translated to
-- ErrNotFound.
insert into power_plant_monthly_targets (plant_id, month, target_kwh)
select sqlc.arg(plant_id), sqlc.arg(month), sqlc.arg(target_kwh)
where exists (
    select 1 from power_plants p
    where p.id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
)
returning *;

-- name: PlantDevicesList :many
-- Isolation: power_plant_devices has no company_id and is reached ONLY by
-- joining to power_plants IN THIS SAME QUERY — a LEFT JOIN, not a pre-check
-- run separately, so this predicate is what an isolation test actually
-- exercises.
--
-- A plant not visible to the Scope contributes ZERO rows: the caller
-- reports ErrNotFound. A visible plant with no devices yet contributes
-- EXACTLY ONE row with d.id (and every other d.* column) NULL — the
-- sentinel the caller checks; a visible plant with devices contributes one
-- row per device, none of them NULL.
select d.id, d.plant_id, d.device_sn, d.device_name, d.device_type, d.device_type_name,
       d.provider_key, d.brand, d.model, d.rated_power_kw, d.status, d.efficiency_pct,
       d.last_seen_at, d.created_at, d.updated_at
from power_plants p
left join power_plant_devices d on d.plant_id = p.id
where p.id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
order by d.device_sn;

-- name: PlantDeviceUpsert :one
-- UpsertDevice, keyed on (plant_id, device_sn): ONE atomic
-- `insert … on conflict (plant_id, device_sn) do update`, now that the SQL
-- call scanner (Task 8c fix round 1, a4746e7) recognises an ON CONFLICT
-- target for what it is instead of reading it as a call to a function named
-- conflict(). Two concurrent UpsertDevice calls racing to create the SAME
-- new device now converge on one row, because the whole statement is a
-- single INSERT and Postgres itself serialises the two around the unique
-- index rather than this package checking-then-acting across two round
-- trips (the fix-round-1 finding this replaces).
--
-- Isolation: the insert/update only runs when plant_id's parent is visible
-- to the Scope, checked via exists(...) IN THIS SAME STATEMENT — including
-- on the update path, since a SELECT that returns no candidate row also
-- proposes no conflict. An invisible plant upserts zero rows; the :one scan
-- then reports ErrNoRows, translated to ErrNotFound. The DO UPDATE branch
-- never assigns plant_id, so a device can be neither moved to nor taken
-- from another plant even by an update this exists() check let through.
insert into power_plant_devices
    (id, plant_id, device_sn, device_name, device_type, device_type_name,
     provider_key, brand, model, rated_power_kw, status, efficiency_pct,
     last_seen_at, created_at, updated_at)
select sqlc.arg(id), sqlc.arg(plant_id), sqlc.arg(device_sn), sqlc.arg(device_name),
       sqlc.arg(device_type), sqlc.arg(device_type_name), sqlc.arg(provider_key),
       sqlc.arg(brand), sqlc.arg(model), sqlc.arg(rated_power_kw), sqlc.arg(status),
       sqlc.arg(efficiency_pct), sqlc.arg(last_seen_at),
       coalesce(sqlc.narg(at)::timestamptz, now()) as created_at, coalesce(sqlc.narg(at)::timestamptz, now()) as updated_at
where exists (
    select 1 from power_plants p
    where p.id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
)
on conflict (plant_id, device_sn) do update
set device_name = excluded.device_name,
    device_type = excluded.device_type,
    device_type_name = excluded.device_type_name,
    provider_key = excluded.provider_key,
    brand = excluded.brand,
    model = excluded.model,
    rated_power_kw = excluded.rated_power_kw,
    status = excluded.status,
    efficiency_pct = excluded.efficiency_pct,
    last_seen_at = excluded.last_seen_at,
    updated_at = excluded.updated_at
returning *;

-- name: PlantAlarmRecipientsList :many
-- Isolation: power_plant_alarm_recipients has no company_id and is reached
-- ONLY by joining to power_plants IN THIS SAME QUERY — a LEFT JOIN, not a
-- pre-check run separately, so this predicate is what an isolation test
-- actually exercises.
--
-- A plant not visible to the Scope contributes ZERO rows: the caller
-- reports ErrNotFound. A visible plant with no recipients yet contributes
-- EXACTLY ONE row with r.plant_id (and r.email) NULL — the sentinel the
-- caller checks; a visible plant with recipients contributes one row per
-- recipient, none of them NULL.
select r.plant_id, r.email
from power_plants p
left join power_plant_alarm_recipients r on r.plant_id = p.id
where p.id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
order by r.email;

-- name: PlantAlarmRecipientsDelete :exec
-- Isolation: scoped via a join to power_plants IN THIS SAME STATEMENT (a
-- DELETE … USING), replacing what used to be a Go-level pre-check run on
-- the pool before the write. A foreign or otherwise invisible plant_id
-- deletes zero rows rather than every recipient under a well-known id.
delete from power_plant_alarm_recipients r
using power_plants p
where r.plant_id = p.id
  and r.plant_id = sqlc.arg(plant_id)
  and p.company_id = sqlc.arg(company_id)
  and p.deleted_at is null;

-- name: PlantAlarmRecipientInsert :one
-- Isolation: the insert only runs when plant_id's parent is visible to the
-- Scope, checked via exists(...) IN THIS SAME STATEMENT. An invisible plant
-- inserts zero rows; the :one scan then reports ErrNoRows, translated to
-- ErrNotFound.
insert into power_plant_alarm_recipients (plant_id, email)
select sqlc.arg(plant_id), sqlc.arg(email)
where exists (
    select 1 from power_plants p
    where p.id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
)
returning *;
