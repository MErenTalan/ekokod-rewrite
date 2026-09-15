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

-- ISO50001ListClauseDates is the SINGLE parent-driven statement ClauseDates()
-- runs (fix round 1, Important 3): it starts from iso50001_projects, not
-- iso50001_clause_dates, and LEFT JOINs the (parentless, company_id-free)
-- child table, so a visible project with zero clause dates still returns
-- exactly one row (clause_id/start_date/end_date all null — the sentinel
-- the repository turns into an empty slice), while an invisible or
-- nonexistent project id returns ZERO rows (ErrNotFound). There is no
-- separate Go-level pre-check query any more (ISO50001ProjectVisible is
-- gone): this one statement's own company_id predicate is the only thing
-- standing between a caller and another tenant's clause dates.
-- name: ISO50001ListClauseDates :many
select cd.clause_id, cd.start_date, cd.end_date
from iso50001_projects p
join buildings b on b.id = p.building_id
left join iso50001_clause_dates cd on cd.project_id = p.id
where p.id = sqlc.arg(project_id)::uuid
  and p.company_id = sqlc.arg(company_id)::uuid
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]))
order by cd.clause_id;

-- ISO50001DeleteClauseDates and ISO50001InsertClauseDates both carry the
-- company_id/building predicate directly, joined through
-- iso50001_projects/buildings, rather than trusting the caller
-- (ReplaceClauseDates) to have already verified visibility via the FOR
-- SHARE lock alone (fix round 1, Important 2 — same shape as
-- carbon.sql's CarbonDeleteConversions/CarbonInsertConversions).
-- name: ISO50001DeleteClauseDates :exec
delete from iso50001_clause_dates cd
using iso50001_projects p, buildings b
where cd.project_id = p.id
  and p.building_id = b.id
  and cd.project_id = sqlc.arg(project_id)::uuid
  and p.company_id = sqlc.arg(company_id)::uuid
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]));

-- One jsonb array parameter, unpacked with jsonb_array_elements: see
-- carbon.sql's CarbonInsertConversions for why (no multi-array unnest in
-- sqlc's catalogue, and jsonb_to_recordset's column-list alias trips
-- TestEveryFunctionSQLcMustTypeIsDeclared's relation/call scan). The insert
-- selects `p.id`, not the raw `$1`-shaped parameter, and p is filtered to
-- (id, company_id) joined to a not-deleted, scope-visible building: a batch
-- cannot land under a project_id/company_id pair that does not jointly
-- identify a real, visible project.
-- name: ISO50001InsertClauseDates :exec
insert into iso50001_clause_dates (project_id, clause_id, start_date, end_date)
select p.id, elem->>'clause_id', (elem->>'start_date')::date, (elem->>'end_date')::date
from jsonb_array_elements(sqlc.arg(dates)::jsonb) as elem, iso50001_projects p
join buildings b on b.id = p.building_id
where p.id = sqlc.arg(project_id)::uuid
  and p.company_id = sqlc.arg(company_id)::uuid
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]));

-- ISO50001ListNotes is the SINGLE parent-driven statement Notes() runs (fix
-- round 1, Important 3): the optional clause_id filter is in the LEFT
-- JOIN's own ON clause, not the WHERE clause, so "project visible, no notes
-- match the filter" still returns the project's sentinel row (all n.*
-- columns null) rather than being filtered away entirely — zero rows means
-- the PROJECT itself is not visible (ErrNotFound), never "no notes matched".
-- name: ISO50001ListNotes :many
select n.id, n.project_id, n.clause_id, n.title, n.body, n.created_by, n.created_at, n.updated_at
from iso50001_projects p
join buildings b on b.id = p.building_id
left join iso50001_notes n on n.project_id = p.id
  and (sqlc.narg(clause_id)::text is null or n.clause_id = sqlc.narg(clause_id)::text)
where p.id = sqlc.arg(project_id)::uuid
  and p.company_id = sqlc.arg(company_id)::uuid
  and b.deleted_at is null
  and (sqlc.arg(all_buildings)::boolean or p.building_id = any(sqlc.arg(building_ids)::uuid[]))
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
