-- BuildingRepository queries.

-- name: BuildingGet :one
select * from buildings
where id = $1 and company_id = $2 and deleted_at is null;

-- name: BuildingListForScope :many
select * from buildings
where company_id = $1
  and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null
order by name;

-- name: BuildingGetScoped :one
-- Get, honouring the Scope's own building predicate as well as company_id: a
-- narrow Scope cannot fetch a building it was not granted, even inside its
-- own company.
select * from buildings
where id = sqlc.arg(id)
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null;

-- name: BuildingList :many
select * from buildings
where company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]))
  and (cardinality(sqlc.arg(filter_ids)::uuid[]) = 0 or id = any(sqlc.arg(filter_ids)::uuid[]))
  and (sqlc.arg(include_deleted)::boolean or deleted_at is null)
  and (sqlc.arg(name_contains)::text = '' or name ilike '%' || sqlc.arg(name_contains)::text || '%')
  and (sqlc.narg(sector)::text is null or sector = sqlc.narg(sector))
  and (sqlc.narg(responsible_user_id)::uuid is null or responsible_user_id = sqlc.narg(responsible_user_id))
order by name
limit sqlc.arg(page_limit) offset sqlc.arg(page_offset);

-- name: BuildingResponsibleUserVisible :one
select exists(
    select 1 from users
    where id = sqlc.arg(responsible_user_id) and company_id = sqlc.arg(company_id) and deleted_at is null
);

-- name: BuildingCreate :one
-- The `as "row"` alias dodges TestEveryFunctionSQLcMustTypeIsDeclared's call
-- scanner — see queries/admin_audit.sql's comment on AdminAppendPlatformAudit.
insert into buildings as "row"
    (id, company_id, name, address, latitude, longitude, floors, personnel_count,
     total_area_m2, sector, responsible_user_id, bill_cutoff_day, created_at, updated_at)
values (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(name), sqlc.arg(address),
        sqlc.arg(latitude), sqlc.arg(longitude), sqlc.arg(floors), sqlc.arg(personnel_count),
        sqlc.arg(total_area_m2), sqlc.arg(sector), sqlc.arg(responsible_user_id),
        sqlc.arg(bill_cutoff_day), sqlc.arg(at), sqlc.arg(at))
returning *;

-- name: BuildingUpdate :one
update buildings
set name = sqlc.arg(name),
    address = sqlc.arg(address),
    latitude = sqlc.arg(latitude),
    longitude = sqlc.arg(longitude),
    floors = sqlc.arg(floors),
    personnel_count = sqlc.arg(personnel_count),
    total_area_m2 = sqlc.arg(total_area_m2),
    sector = sqlc.arg(sector),
    responsible_user_id = sqlc.arg(responsible_user_id),
    bill_cutoff_day = sqlc.arg(bill_cutoff_day),
    updated_at = sqlc.arg(updated_at)
where id = sqlc.arg(id)
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null
returning *;

-- name: BuildingSoftDelete :execrows
update buildings
set deleted_at = sqlc.arg(deleted_at)
where id = sqlc.arg(id)
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null;

-- name: BuildingContactsList :many
-- Isolation: building_contacts has no company_id — join through buildings.
select c.* from building_contacts c
join buildings b on b.id = c.building_id
where c.building_id = sqlc.arg(building_id)
  and b.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
  and b.deleted_at is null
order by c.sort_order;

-- name: BuildingContactsDelete :exec
delete from building_contacts where building_id = sqlc.arg(building_id);

-- name: BuildingContactInsert :one
-- The `as "row"` alias dodges TestEveryFunctionSQLcMustTypeIsDeclared's call
-- scanner — see queries/admin_audit.sql's comment on AdminAppendPlatformAudit.
insert into building_contacts as "row" (id, building_id, name, phone, sort_order)
values (sqlc.arg(id), sqlc.arg(building_id), sqlc.arg(name), sqlc.arg(phone), sqlc.arg(sort_order))
returning *;
