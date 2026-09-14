-- CursorRepository's queries. ingestion_cursors has no company_id: every
-- method joins through analyzers.

-- name: CursorGet :one
-- A single query covers BOTH "analyzer not visible" and "no cursor yet": both
-- yield zero rows, and repository.go's doc says both return ErrNotFound.
select c.* from ingestion_cursors c
join analyzers a on a.id = c.analyzer_id
where c.analyzer_id = sqlc.arg(analyzer_id)
  and c.kind = sqlc.arg(kind)::reading_kind
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null;

-- name: CursorList :many
-- analyzer_ids EMPTY means "every analyzer visible to the scope" — this
-- narrows, it never widens past the scope's own join predicate, matching the
-- BuildingFilter.IDs convention (repository.go). A non-empty list narrows
-- further; ids outside the scope contribute no rows.
select c.* from ingestion_cursors c
join analyzers a on a.id = c.analyzer_id
where a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
  and (cardinality(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')) = 0 or c.analyzer_id = any(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')))
order by c.analyzer_id, c.kind;

-- name: CursorRecordSuccess :execrows
-- The select-from-analyzers source is the visibility check: an invisible
-- analyzer_id makes the source empty, so nothing is inserted or updated and
-- execrows reports 0, which the repository reads as ErrNotFound.
--
-- NOTE: TestEveryFunctionSQLcMustTypeIsDeclared currently flags
-- "ingestion_cursors (" and "conflict (" below as calls to undeclared
-- functions — a known false positive (no notion of INSERT-target or ON
-- CONFLICT syntax) being fixed in parallel on f1/task-8c. Left as ordinary
-- SQL; see the task report.
insert into ingestion_cursors (analyzer_id, kind, last_ts, last_success_at, consecutive_failures)
select a.id, sqlc.arg(kind)::reading_kind, sqlc.arg(last_ts)::timestamptz, sqlc.arg(at)::timestamptz, 0
from analyzers a
where a.id = sqlc.arg(analyzer_id)
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
on conflict (analyzer_id, kind) do update set
    last_ts = excluded.last_ts,
    last_success_at = excluded.last_success_at,
    consecutive_failures = 0;

-- name: CursorRecordFailure :execrows
insert into ingestion_cursors (analyzer_id, kind, last_error, last_error_at, consecutive_failures)
select a.id, sqlc.arg(kind)::reading_kind, sqlc.arg(message)::text, sqlc.arg(at)::timestamptz, 1
from analyzers a
where a.id = sqlc.arg(analyzer_id)
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
on conflict (analyzer_id, kind) do update set
    last_error = excluded.last_error,
    last_error_at = excluded.last_error_at,
    consecutive_failures = ingestion_cursors.consecutive_failures + 1;
