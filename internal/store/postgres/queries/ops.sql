-- Tenant job-run and operational-message repository queries (migration
-- 00008). Owned by Task 11b.
--
-- job_runs.company_id and operational_messages.company_id are NULLABLE, for
-- platform-wide work (AdminJournalRepository writes those). Every query here
-- filters company_id = the caller's Scope explicitly — never `company_id is
-- null or …` — so a platform row can never be read or written through this,
-- the tenant-scoped, surface.

-- name: OpsStartRun :one
insert into job_runs (id, company_id, job_type, scope, status)
values (
    coalesce(nullif(sqlc.arg(id)::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
             gen_random_uuid()),
    sqlc.arg(company_id)::uuid, sqlc.arg(job_type), sqlc.arg(scope)::jsonb, sqlc.arg(status)
)
returning *;

-- name: OpsFinishRun :one
update job_runs set
    finished_at = sqlc.arg(at)::timestamptz,
    status = sqlc.arg(status),
    processed = sqlc.arg(processed)::int,
    skipped = sqlc.arg(skipped)::int,
    failed = sqlc.arg(failed)::int,
    error = sqlc.narg(error_text)::text,
    detail = sqlc.narg(detail)::jsonb
where id = sqlc.arg(id)::uuid and company_id = sqlc.arg(company_id)::uuid
returning *;

-- name: OpsGetRun :one
select * from job_runs where id = $1 and company_id = sqlc.arg(company_id)::uuid;

-- name: OpsListRuns :many
select * from job_runs
where company_id = sqlc.arg(company_id)::uuid
  and (sqlc.narg(job_type)::text is null or job_type = sqlc.narg(job_type)::text)
  and (sqlc.narg(statuses)::text[] is null or cardinality(sqlc.narg(statuses)::text[]) = 0
       or status = any(sqlc.narg(statuses)::text[]))
  and (not sqlc.arg(running)::boolean or finished_at is null)
  and (sqlc.narg(range_from)::timestamptz is null or started_at >= sqlc.narg(range_from)::timestamptz)
  and (sqlc.narg(range_to)::timestamptz is null or started_at < sqlc.narg(range_to)::timestamptz)
order by started_at desc, id
limit sqlc.arg(limit_val)::int offset sqlc.arg(offset_val)::int;

-- name: OpsAppendMessage :one
insert into operational_messages
    (company_id, kind, category, status, message, detail, related_type, related_id, metadata)
values (
    sqlc.arg(company_id)::uuid, sqlc.arg(kind), sqlc.arg(category), sqlc.arg(status),
    sqlc.arg(message), sqlc.narg(detail)::text, sqlc.narg(related_type)::text,
    sqlc.narg(related_id)::uuid, sqlc.narg(metadata)::jsonb
)
returning *;

-- name: OpsListMessages :many
select * from operational_messages
where company_id = sqlc.arg(company_id)::uuid
  and (sqlc.narg(kinds)::text[] is null or cardinality(sqlc.narg(kinds)::text[]) = 0
       or kind = any(sqlc.narg(kinds)::text[]))
  and (sqlc.narg(categories)::text[] is null or cardinality(sqlc.narg(categories)::text[]) = 0
       or category = any(sqlc.narg(categories)::text[]))
  and (sqlc.narg(statuses)::text[] is null or cardinality(sqlc.narg(statuses)::text[]) = 0
       or status = any(sqlc.narg(statuses)::text[]))
  and (sqlc.narg(related_type)::text is null or related_type = sqlc.narg(related_type)::text)
  and (sqlc.narg(related_id)::uuid is null or related_id = sqlc.narg(related_id)::uuid)
  and (sqlc.narg(range_from)::timestamptz is null or created_at >= sqlc.narg(range_from)::timestamptz)
  and (sqlc.narg(range_to)::timestamptz is null or created_at < sqlc.narg(range_to)::timestamptz)
  -- R226: a case-insensitive SUBSTRING search over message and detail.
  -- strpos on lower() rather than ilike: ilike would read '%' and '_' in the
  -- user's own search term as wildcards, and escaping them correctly is a
  -- trap this avoids entirely. An empty q filters nothing.
  and (sqlc.arg(q)::text = ''
       or strpos(lower(message), lower(sqlc.arg(q)::text)) > 0
       or strpos(lower(coalesce(detail, '')), lower(sqlc.arg(q)::text)) > 0)
order by created_at desc, id
limit sqlc.arg(limit_val)::int offset sqlc.arg(offset_val)::int;
