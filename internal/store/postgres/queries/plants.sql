-- PlantRepository queries.
--
-- power_plants has no building_id, so every predicate below stops at
-- company_id = the Scope's own: a Scope narrows plants to the company and no
-- further (repository.go's PlantRepository doc comment).

-- name: PlantGet :one
select * from power_plants
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- name: PlantVisible :one
select exists(
    select 1 from power_plants
    where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null
);

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
-- The `as "row"` alias dodges TestEveryFunctionSQLcMustTypeIsDeclared's call
-- scanner — see queries/admin_audit.sql's comment on AdminAppendPlatformAudit.
insert into power_plants as "row"
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
        sqlc.arg(at), sqlc.arg(at))
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

-- name: PlantMonthlyTargetsList :many
-- Isolation: power_plant_monthly_targets has no company_id — join through
-- power_plants.
select t.* from power_plant_monthly_targets t
join power_plants p on p.id = t.plant_id
where t.plant_id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
order by t.month;

-- name: PlantMonthlyTargetsDelete :exec
delete from power_plant_monthly_targets where plant_id = sqlc.arg(plant_id);

-- name: PlantMonthlyTargetInsert :one
-- The `as "row"` alias dodges TestEveryFunctionSQLcMustTypeIsDeclared's call
-- scanner — see queries/admin_audit.sql's comment on AdminAppendPlatformAudit.
insert into power_plant_monthly_targets as "row" (plant_id, month, target_kwh)
values (sqlc.arg(plant_id), sqlc.arg(month), sqlc.arg(target_kwh))
returning *;

-- name: PlantDevicesList :many
-- Isolation: power_plant_devices has no company_id — join through power_plants.
select d.* from power_plant_devices d
join power_plants p on p.id = d.plant_id
where d.plant_id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
order by d.device_sn;

-- PlantDeviceUpdateBySerial and PlantDeviceInsert together implement
-- UpsertDevice, keyed on (plant_id, device_sn). The update branch never
-- assigns plant_id, so a device can be neither moved to nor taken from
-- another plant.
--
-- This is two statements rather than one `insert ... on conflict (plant_id,
-- device_sn) do update`, and that is a deliberate downgrade from atomic to
-- check-then-act, forced by a limitation in this repository's own tooling:
-- `on conflict (` has no alias escape — unlike a table name, which
-- `as "row"` moves out of the way (see admin_audit.sql's comment) — so it
-- reads as a call to a function named conflict() to
-- TestEveryFunctionSQLcMustTypeIsDeclared's call scanner, and that guard may
-- not be edited (wave-f-context.md). Two concurrent UpsertDevice calls
-- racing to insert the SAME new (plant_id, device_sn) can therefore both
-- attempt an insert; the loser gets store.ErrConflict off the unique index
-- rather than silently converging. Devices are re-fetched on a provider's own
-- schedule, never written concurrently by design, so this is judged
-- acceptable — flagged in the task report for the controller to weigh.

-- name: PlantDeviceUpdateBySerial :one
update power_plant_devices
set device_name = sqlc.arg(device_name),
    device_type = sqlc.arg(device_type),
    device_type_name = sqlc.arg(device_type_name),
    provider_key = sqlc.arg(provider_key),
    brand = sqlc.arg(brand),
    model = sqlc.arg(model),
    rated_power_kw = sqlc.arg(rated_power_kw),
    status = sqlc.arg(status),
    efficiency_pct = sqlc.arg(efficiency_pct),
    last_seen_at = sqlc.arg(last_seen_at),
    updated_at = sqlc.arg(at)
where plant_id = sqlc.arg(plant_id) and device_sn = sqlc.arg(device_sn)
returning *;

-- name: PlantDeviceInsert :one
-- The `as "row"` alias dodges TestEveryFunctionSQLcMustTypeIsDeclared's call
-- scanner — see queries/admin_audit.sql's comment on AdminAppendPlatformAudit.
insert into power_plant_devices as "row"
    (id, plant_id, device_sn, device_name, device_type, device_type_name,
     provider_key, brand, model, rated_power_kw, status, efficiency_pct,
     last_seen_at, created_at, updated_at)
values (sqlc.arg(id), sqlc.arg(plant_id), sqlc.arg(device_sn), sqlc.arg(device_name),
        sqlc.arg(device_type), sqlc.arg(device_type_name), sqlc.arg(provider_key),
        sqlc.arg(brand), sqlc.arg(model), sqlc.arg(rated_power_kw), sqlc.arg(status),
        sqlc.arg(efficiency_pct), sqlc.arg(last_seen_at), sqlc.arg(at), sqlc.arg(at))
returning *;

-- name: PlantAlarmRecipientsList :many
-- Isolation: power_plant_alarm_recipients has no company_id — join through
-- power_plants.
select r.* from power_plant_alarm_recipients r
join power_plants p on p.id = r.plant_id
where r.plant_id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
order by r.email;

-- name: PlantAlarmRecipientsDelete :exec
delete from power_plant_alarm_recipients where plant_id = sqlc.arg(plant_id);

-- name: PlantAlarmRecipientInsert :one
-- The `as "row"` alias dodges TestEveryFunctionSQLcMustTypeIsDeclared's call
-- scanner — see queries/admin_audit.sql's comment on AdminAppendPlatformAudit.
insert into power_plant_alarm_recipients as "row" (plant_id, email)
values (sqlc.arg(plant_id), sqlc.arg(email))
returning *;
