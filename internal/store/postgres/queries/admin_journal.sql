-- AdminJournalRepository queries (store.AdminJournalRepository). Owned by
-- Task 11b, called only from internal/store/postgres/admin/journal.go.
--
-- job_runs.company_id and operational_messages.company_id are nullable
-- precisely so a platform-wide job — run for no tenant — has somewhere to
-- write. Every statement here writes or touches company_id IS NULL rows
-- only, so a tenant's run or message (company_id set) is never reachable
-- from this, the unscoped, surface.

-- name: AdminStartPlatformRun :one
insert into job_runs (id, company_id, job_type, scope, status)
values (
    coalesce(nullif(sqlc.arg(id)::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
             gen_random_uuid()),
    null, sqlc.arg(job_type), sqlc.arg(scope)::jsonb, sqlc.arg(status)
)
returning *;

-- name: AdminFinishPlatformRun :one
update job_runs set
    finished_at = sqlc.arg(at)::timestamptz,
    status = sqlc.arg(status),
    processed = sqlc.arg(processed)::int,
    skipped = sqlc.arg(skipped)::int,
    failed = sqlc.arg(failed)::int,
    error = sqlc.narg(error_text)::text,
    detail = sqlc.narg(detail)::jsonb
where id = sqlc.arg(id)::uuid and company_id is null
returning *;

-- name: AdminAppendPlatformMessage :one
insert into operational_messages
    (company_id, kind, category, status, message, detail, related_type, related_id, metadata)
values (
    null, sqlc.arg(kind), sqlc.arg(category), sqlc.arg(status), sqlc.arg(message),
    sqlc.narg(detail)::text, sqlc.narg(related_type)::text, sqlc.narg(related_id)::uuid,
    sqlc.narg(metadata)::jsonb
)
returning *;
