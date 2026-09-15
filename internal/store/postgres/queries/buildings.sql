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
-- coalesce(sqlc.narg(at)::timestamptz, now()): a caller that leaves CreatedAt at its zero
-- value gets the database's own now() rather than writing 0001-01-01.
--
-- CONTROLLER RULING (fix round 1, second dispatch): every stored foreign key
-- must be validated IN SQL, inside the write statement — never by a
-- Go-level check alone (a sibling task's AdminScope write stored another
-- tenant's building id because its Go-level check used
-- Scope.AllowsBuilding(), which returns true for ANY id under AllBuildings
-- with no company_id check at all). responsible_user_id is embedded here as
-- an exists(...) gate on the insert itself, so the check cannot be skipped
-- by a call site that forgets it, and is provably tied to company_id.
insert into buildings
    (id, company_id, name, address, latitude, longitude, floors, personnel_count,
     total_area_m2, sector, responsible_user_id, bill_cutoff_day, created_at, updated_at)
select sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(name), sqlc.arg(address),
       sqlc.arg(latitude), sqlc.arg(longitude), sqlc.arg(floors), sqlc.arg(personnel_count),
       sqlc.arg(total_area_m2), sqlc.arg(sector), sqlc.arg(responsible_user_id),
       sqlc.arg(bill_cutoff_day), coalesce(sqlc.narg(at)::timestamptz, now()), coalesce(sqlc.narg(at)::timestamptz, now())
where (
    sqlc.arg(responsible_user_id)::uuid is null
    or exists (
        select 1 from users u
        where u.id = sqlc.arg(responsible_user_id)
          and u.company_id = sqlc.arg(company_id)
          and u.deleted_at is null
    )
)
returning *;

-- name: BuildingUpdate :one
-- Isolation: the responsible_user_id FK is validated IN THIS SAME STATEMENT
-- (see BuildingCreate's comment) rather than relying solely on a separate
-- Go-level pre-check.
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
where buildings.id = sqlc.arg(id)
  and buildings.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or buildings.id = any(sqlc.arg(building_ids)::uuid[]))
  and buildings.deleted_at is null
  and (
      sqlc.arg(responsible_user_id)::uuid is null
      or exists (
          select 1 from users u
          where u.id = sqlc.arg(responsible_user_id)
            and u.company_id = sqlc.arg(company_id)
            and u.deleted_at is null
      )
  )
returning *;

-- name: BuildingSoftDelete :execrows
update buildings
set deleted_at = sqlc.arg(deleted_at)
where id = sqlc.arg(id)
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null;

-- name: BuildingGetScopedForShare :one
-- Same predicate as BuildingGetScoped, but takes a FOR SHARE lock: used
-- INSIDE ReplaceContacts's transaction so a concurrent SoftDelete of the
-- same building cannot race the delete+insert that follows it. This is not
-- itself the isolation boundary for building_contacts — BuildingContactsDelete
-- and BuildingContactInsert below are each scoped on their own — it exists
-- so an empty replacement list still has something to return ErrNotFound
-- from, since delete-then-insert-nothing would otherwise run no query that
-- could fail on a foreign or invisible building.
select * from buildings
where id = sqlc.arg(id)
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null
for share;

-- name: BuildingContactsList :many
-- Isolation: building_contacts has no company_id and is reached ONLY by
-- joining to buildings IN THIS SAME QUERY — a LEFT JOIN, not a pre-check run
-- separately, so this predicate (company_id and the Scope's building
-- branch) is what an isolation test actually exercises, not a Go-level
-- lookup that would otherwise shadow it.
--
-- A building not visible to the Scope contributes ZERO rows (nothing here
-- to distinguish it from "no such building"): the caller reports
-- ErrNotFound. A visible building with no contacts yet contributes EXACTLY
-- ONE row with every c.* column NULL (c.id IS NULL is the sentinel the
-- caller checks); a visible building with contacts contributes one row per
-- contact, none of them NULL.
select c.id, c.building_id, c.name, c.phone, c.sort_order
from buildings b
left join building_contacts c on c.building_id = b.id
where b.id = sqlc.arg(building_id)
  and b.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
  and b.deleted_at is null
order by c.sort_order;

-- name: BuildingContactsDelete :exec
-- Isolation: scoped via a join to buildings IN THIS SAME STATEMENT (a
-- DELETE … USING), replacing what used to be a Go-level pre-check run on
-- the pool before the write. A foreign or otherwise invisible building_id
-- deletes zero rows rather than every contact under a well-known id.
delete from building_contacts c
using buildings b
where c.building_id = b.id
  and c.building_id = sqlc.arg(building_id)
  and b.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
  and b.deleted_at is null;

-- name: BuildingContactInsert :one
-- Isolation: the insert only runs when building_id's parent is visible to
-- the Scope, checked via exists(...) IN THIS SAME STATEMENT. An invisible
-- building inserts zero rows; the :one scan then reports ErrNoRows,
-- translated to ErrNotFound.
insert into building_contacts (id, building_id, name, phone, sort_order)
select sqlc.arg(id), sqlc.arg(building_id), sqlc.arg(name), sqlc.arg(phone), sqlc.arg(sort_order)
where exists (
    select 1 from buildings b
    where b.id = sqlc.arg(building_id)
      and b.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
      and b.deleted_at is null
)
returning *;
