-- F9 solar queries. Every table here is plant-parented and company-scoped
-- through power_plants in the same statement (PlantRepository's rule).

-- name: SolarPlantVisibleForShare :one
select id from power_plants
where id = sqlc.arg(plant_id) and company_id = sqlc.arg(company_id) and deleted_at is null
for share;

-- name: TotalsDeleteRange :execrows
delete from plant_production_totals t
using power_plants p
where t.plant_id = p.id and t.plant_id = sqlc.arg(plant_id)
  and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
  and t.ts >= sqlc.arg(from_ts)::timestamptz and t.ts < sqlc.arg(to_ts)::timestamptz;

-- name: TotalsRange :many
select t.plant_id, t.ts, t.production_kwh, t.active_power_kw, t.basis
from plant_production_totals t
join power_plants p on p.id = t.plant_id
where t.plant_id = sqlc.arg(plant_id)
  and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
  and t.ts >= sqlc.arg(from_ts)::timestamptz and t.ts < sqlc.arg(to_ts)::timestamptz
order by t.ts;

-- name: TotalsFirstTs :one
select min(t.ts)::timestamptz as first_ts
from plant_production_totals t
join power_plants p on p.id = t.plant_id
where t.plant_id = sqlc.arg(plant_id)
  and p.company_id = sqlc.arg(company_id) and p.deleted_at is null;

-- name: DeviceProductionDeleteRange :execrows
delete from plant_production pp
using power_plants p
where pp.plant_id = p.id and pp.plant_id = sqlc.arg(plant_id)
  and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
  and pp.ts >= sqlc.arg(from_ts)::timestamptz and pp.ts < sqlc.arg(to_ts)::timestamptz;

-- name: PlantDeviceSnapshotUpdate :execrows
update power_plant_devices d
set snapshot_at = sqlc.narg(snapshot_at), fault_status = sqlc.narg(fault_status),
    active_power_kw = sqlc.narg(active_power_kw), yield_today_kwh = sqlc.narg(yield_today_kwh),
    yield_month_kwh = sqlc.narg(yield_month_kwh), yield_year_kwh = sqlc.narg(yield_year_kwh),
    yield_total_kwh = sqlc.narg(yield_total_kwh), status = sqlc.narg(status),
    last_seen_at = coalesce(sqlc.narg(snapshot_at), d.last_seen_at), updated_at = now()
from power_plants p
where d.plant_id = p.id and d.id = sqlc.arg(id) and d.plant_id = sqlc.arg(plant_id)
  and p.company_id = sqlc.arg(company_id) and p.deleted_at is null;

-- name: PlantSetSyncState :execrows
update power_plants
set isolar_last_sync_at = sqlc.arg(at), isolar_last_sync_error = sqlc.narg(error_code)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- name: PlantSetIsolarLink :one
update power_plants
set isolar_ps_id = sqlc.narg(isolar_ps_id), isolar_ps_key = sqlc.narg(isolar_ps_key),
    isolar_ps_name = sqlc.narg(isolar_ps_name), isolar_installed_kw = sqlc.narg(isolar_installed_kw),
    isolar_linked_at = sqlc.narg(isolar_linked_at), isolar_credential_id = sqlc.narg(isolar_credential_id),
    total_capacity_kw = sqlc.narg(total_capacity_kw), updated_at = now()
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null
returning *;

-- name: PlantListLinked :many
select * from power_plants
where company_id = sqlc.arg(company_id) and deleted_at is null and isolar_ps_id is not null
order by name;

-- name: FaultUpsert :execrows
insert into plant_faults (plant_id, ref, code, name, level, type, device_name, occurred_at, closed_at, first_seen_at)
select sqlc.arg(plant_id), sqlc.arg(ref), sqlc.arg(code), sqlc.arg(name), sqlc.narg(level), sqlc.narg(type),
       sqlc.narg(device_name), sqlc.arg(occurred_at), sqlc.narg(closed_at), sqlc.arg(first_seen_at)
where exists (select 1 from power_plants p
              where p.id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null)
on conflict (plant_id, ref) do update set
    name = excluded.name, level = excluded.level, type = excluded.type,
    device_name = excluded.device_name, closed_at = excluded.closed_at;

-- name: FaultList :many
select f.* from plant_faults f
join power_plants p on p.id = f.plant_id
where f.plant_id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
order by f.occurred_at desc, f.ref
limit sqlc.arg(page_limit) offset sqlc.arg(page_offset);

-- name: FaultCount :one
select count(*) from plant_faults f
join power_plants p on p.id = f.plant_id
where f.plant_id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null;

-- name: FaultUnforwarded :many
select f.* from plant_faults f
join power_plants p on p.id = f.plant_id
where f.plant_id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null
  and f.occurred_at >= sqlc.arg(since)
  and not exists (select 1 from isolar_forwarded_alarms a where a.plant_id = f.plant_id and a.alarm_ref = f.ref)
order by f.occurred_at, f.ref;

-- name: FaultClaim :many
-- R287: the insert that wins is the only sender.
insert into isolar_forwarded_alarms (plant_id, alarm_ref)
select sqlc.arg(plant_id), unnest(sqlc.arg(refs)::text[])
where exists (select 1 from power_plants p
              where p.id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id) and p.deleted_at is null)
on conflict do nothing
returning alarm_ref;

-- name: FaultRelease :execrows
delete from isolar_forwarded_alarms a
using power_plants p
where a.plant_id = p.id and a.plant_id = sqlc.arg(plant_id) and p.company_id = sqlc.arg(company_id)
  and a.alarm_ref = any(sqlc.arg(refs)::text[]);

-- name: FaultPrune :execrows
delete from plant_faults f
using power_plants p
where f.plant_id = p.id and p.company_id = sqlc.arg(company_id) and f.occurred_at < sqlc.arg(before);

-- name: FaultForwardedPrune :execrows
delete from isolar_forwarded_alarms a
using power_plants p
where a.plant_id = p.id and p.company_id = sqlc.arg(company_id) and a.sent_at < sqlc.arg(before);
