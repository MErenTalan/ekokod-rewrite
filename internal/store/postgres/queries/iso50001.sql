-- ISO 50001 repository queries (migration 00007). Owned by Task 11b.
--
-- iso50001_clause_dates and iso50001_notes have no company_id: every query
-- against them carries the scope check IN THE SAME STATEMENT, through a join
-- to iso50001_projects and then to buildings (never a separate Go-level
-- visibility check run before the real query). Reads join all the way to
-- buildings so a project under a soft-deleted building disappears too
-- (global-constraints.md's soft-delete rule). Writes either embed
-- `where exists (<scoped parent>)` / `using <parent> where <scope>` in one
-- statement, or, for Replace* (a delete+insert pair), lock the parent with
-- `for share` inside the same transaction as the delete+insert.

-- name: ISO50001GetProject :one
select p.* from iso50001_projects p
join buildings b on b.id = p.building_id
where p.building_id = $1
  and p.company_id = $2
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]));

-- name: ISO50001EnsureProject :one
-- building_id is unique on iso50001_projects, so this is a natural
-- insert-or-return-existing upsert, not a check-then-insert: ON CONFLICT DO
-- UPDATE with a no-op SET (touching updated_at) is what makes RETURNING work
-- whether the row was just created or already existed.
insert into iso50001_projects (id, company_id, building_id)
select gen_random_uuid(), sqlc.arg(company_id)::uuid, sqlc.arg(building_id)::uuid
where exists (
    select 1 from buildings b
    where b.id = sqlc.arg(building_id)::uuid
      and b.company_id = sqlc.arg(company_id)::uuid
      and b.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
)
on conflict (building_id) do update set updated_at = now()
returning *;

-- name: ISO50001ProjectVisibleForShare :one
select p.id from iso50001_projects p
join buildings b on b.id = p.building_id
where p.id = $1
  and p.company_id = $2
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]))
for share of p;

-- ISO50001ProjectVisible is ISO50001ProjectVisibleForShare without the lock,
-- for the read-only existence/visibility check ClauseDates and Notes run
-- before listing their (parentless, company_id-free) child rows: a project id
-- that does not exist, or is not visible to the Scope, must return
-- ErrNotFound rather than the empty list a plain "many" query would give it.
-- name: ISO50001ProjectVisible :one
select p.id from iso50001_projects p
join buildings b on b.id = p.building_id
where p.id = $1
  and p.company_id = $2
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]));

-- name: ISO50001ListClauseDates :many
select cd.* from iso50001_clause_dates cd
join iso50001_projects p on p.id = cd.project_id
join buildings b on b.id = p.building_id
where cd.project_id = $1
  and p.company_id = $2
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]))
order by cd.clause_id;

-- name: ISO50001DeleteClauseDates :exec
delete from iso50001_clause_dates where project_id = $1;

-- One jsonb array parameter, unpacked with jsonb_array_elements: see
-- carbon.sql's CarbonInsertConversions for why (no multi-array unnest in
-- sqlc's catalogue, and jsonb_to_recordset's column-list alias trips
-- TestEveryFunctionSQLcMustTypeIsDeclared's relation/call scan).
-- name: ISO50001InsertClauseDates :exec
insert into iso50001_clause_dates (project_id, clause_id, start_date, end_date)
select $1, elem->>'clause_id', (elem->>'start_date')::date, (elem->>'end_date')::date
from jsonb_array_elements(sqlc.arg(dates)::jsonb) as elem;

-- name: ISO50001ListNotes :many
select n.* from iso50001_notes n
join iso50001_projects p on p.id = n.project_id
join buildings b on b.id = p.building_id
where n.project_id = $1
  and p.company_id = $2
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and (sqlc.narg(clause_id)::text is null or n.clause_id = sqlc.narg(clause_id)::text)
order by n.created_at, n.id;

-- name: ISO50001CreateNote :one
insert into iso50001_notes (id, project_id, clause_id, title, body, created_by)
select
    coalesce(nullif(sqlc.arg(id)::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
             gen_random_uuid()),
    sqlc.arg(project_id)::uuid, sqlc.arg(clause_id), sqlc.narg(title)::text, sqlc.arg(body),
    sqlc.narg(created_by)::uuid
where exists (
    select 1 from iso50001_projects p
    join buildings b on b.id = p.building_id
    where p.id = sqlc.arg(project_id)::uuid
      and p.company_id = sqlc.arg(company_id)::uuid
      and b.deleted_at is null
      and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]))
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
returning *;

-- name: ISO50001UpdateNote :one
-- project_id is never in the SET list: a note stays under the project it was
-- created in.
update iso50001_notes n set
    clause_id = sqlc.arg(clause_id),
    title = sqlc.narg(title)::text,
    body = sqlc.arg(body),
    updated_at = now()
from iso50001_projects p
join buildings b on b.id = p.building_id
where n.id = sqlc.arg(id)::uuid
  and n.project_id = p.id
  and p.company_id = sqlc.arg(company_id)::uuid
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]))
returning n.*;

-- name: ISO50001DeleteNote :execrows
delete from iso50001_notes n
using iso50001_projects p, buildings b
where n.id = sqlc.arg(id)::uuid
  and n.project_id = p.id
  and p.building_id = b.id
  and p.company_id = sqlc.arg(company_id)::uuid
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]));
