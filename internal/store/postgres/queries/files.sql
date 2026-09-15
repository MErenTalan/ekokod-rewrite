-- File metadata repository queries (migration 00008). Owned by Task 11b.
--
-- stored_files carries company_id directly and has no building_id at all
-- (like power_plants), so Scope narrows it to company_id only — there is no
-- building set to intersect against.

-- name: FileGet :one
select * from stored_files where id = $1 and company_id = $2 and deleted_at is null;

-- name: FileList :many
select * from stored_files
where company_id = sqlc.arg(company_id)::uuid
  and (sqlc.arg(include_deleted)::boolean or deleted_at is null)
  and (sqlc.narg(owner_type)::text is null or owner_type = sqlc.narg(owner_type)::text)
  and (sqlc.narg(owner_id)::uuid is null or owner_id = sqlc.narg(owner_id)::uuid)
  and (sqlc.narg(clause_id)::text is null or clause_id = sqlc.narg(clause_id)::text)
order by created_at desc, id
limit sqlc.arg(limit_val)::int offset sqlc.arg(offset_val)::int;

-- uploaded_by, when set, must be a user of company_id: the `and (... is null
-- or exists (...))` clause makes that atomic with the insert.
-- name: FileCreate :one
insert into stored_files
    (id, company_id, owner_type, owner_id, clause_id, original_name, stored_path,
     content_type, size_bytes, checksum, uploaded_by)
select
    coalesce(nullif(sqlc.arg(id)::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
             gen_random_uuid()),
    sqlc.arg(company_id)::uuid, sqlc.arg(owner_type), sqlc.narg(owner_id)::uuid,
    sqlc.narg(clause_id)::text, sqlc.arg(original_name), sqlc.arg(stored_path),
    sqlc.arg(content_type), sqlc.arg(size_bytes)::bigint, sqlc.arg(checksum),
    sqlc.narg(uploaded_by)::uuid
where (
    sqlc.narg(uploaded_by)::uuid is null
    or exists (
        select 1 from users u
        where u.id = sqlc.narg(uploaded_by)::uuid
          and u.company_id = sqlc.arg(company_id)::uuid
          and u.deleted_at is null
    )
)
returning *;

-- name: FileSoftDelete :execrows
update stored_files set deleted_at = sqlc.arg(at)::timestamptz
where id = sqlc.arg(id)::uuid and company_id = sqlc.arg(company_id)::uuid and deleted_at is null;
