-- CompanyRepository queries. A company IS the tenant, so the scope predicate
-- is "id equals the Scope's own CompanyID" rather than a company_id column:
-- every query below takes the requested id and the scope's company_id as
-- separate arguments and requires both, so a mismatched pair returns no row
-- the same way a foreign tenant's row would on every other repository.

-- name: CompanyGet :one
select * from companies
where id = sqlc.arg(id)
  and id = sqlc.arg(company_id)
  and deleted_at is null;

-- name: CompanyList :many
select * from companies
where id = sqlc.arg(company_id)
  and (sqlc.arg(include_deleted)::boolean or deleted_at is null)
  and (sqlc.arg(name_contains)::text = '' or name ilike '%' || sqlc.arg(name_contains)::text || '%')
  and (sqlc.narg(sector)::text is null or sector = sqlc.narg(sector))
order by name
limit sqlc.arg(page_limit) offset sqlc.arg(page_offset);

-- name: CompanyCreate :one
-- coalesce(sqlc.narg(at)::timestamptz, now()): a caller that leaves CreatedAt at its zero
-- value gets the database's own now() rather than writing 0001-01-01.
insert into companies
    (id, name, address, total_area_m2, personnel_count, contact_name,
     contact_phone, sector, created_at, updated_at)
values (sqlc.arg(id), sqlc.arg(name), sqlc.arg(address), sqlc.arg(total_area_m2),
        sqlc.arg(personnel_count), sqlc.arg(contact_name), sqlc.arg(contact_phone),
        sqlc.arg(sector), coalesce(sqlc.narg(at)::timestamptz, now()), coalesce(sqlc.narg(at)::timestamptz, now()))
returning *;

-- name: CompanyUpdate :one
update companies
set name = sqlc.arg(name),
    address = sqlc.arg(address),
    total_area_m2 = sqlc.arg(total_area_m2),
    personnel_count = sqlc.arg(personnel_count),
    contact_name = sqlc.arg(contact_name),
    contact_phone = sqlc.arg(contact_phone),
    sector = sqlc.arg(sector),
    updated_at = sqlc.arg(updated_at)
where id = sqlc.arg(id)
  and id = sqlc.arg(company_id)
  and deleted_at is null
returning *;

-- name: CompanySoftDelete :execrows
update companies
set deleted_at = sqlc.arg(deleted_at)
where id = sqlc.arg(id)
  and id = sqlc.arg(company_id)
  and deleted_at is null;
