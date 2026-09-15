-- ReadingRepository's queries. meter_readings has no company_id (see
-- repository.go's "ROWS WITHOUT company_id" header): every method's DATA
-- READ carries the scope in its own join — never a bare `select * from
-- meter_readings` guarded only by a separate Go-side check. The one
-- exception in shape is BulkInsert's COPY-into-staging-then-upsert, which is
-- NOT here: a per-call temporary table cannot appear in sqlc's schema
-- catalogue (it does not exist until the transaction that creates it), so
-- that statement is built and run as plain SQL in readings.go; what IS here
-- for it is ReadingVisibleAnalyzerIDs, the `for share` row lock that closes
-- the gap a plain, unlocked pre-check would leave open for a concurrent
-- reassignment of one of the batch's analyzers.

-- name: ReadingVisibleAnalyzerIDs :many
-- Returns the subset of analyzer_ids visible to the scope, AND LOCKS them
-- (`for share`) for the rest of the caller's transaction: BulkInsert compares
-- the result against the DISTINCT analyzer ids in the batch, and the lock
-- means a concurrent reassignment or soft-delete of one of them cannot slip
-- in between this check and the write that follows it in the same
-- transaction — the exact TOCTOU gap an unlocked count-only check leaves
-- open. A mismatch in the caller means at least one row's analyzer is not
-- visible, and the whole batch is refused.
select id from analyzers
where id = any(sqlc.arg(analyzer_ids)::uuid[])
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null
for share;

-- name: ReadingAnalyzerVisible :one
-- Used ONLY to pick an error, never to gate a data read: Range/
-- BoundaryReadings/Latest run their own scoped query FIRST and unconditionally,
-- and call this only when that query came back empty, to tell "the analyzer
-- itself is not visible" (ErrNotFound) from "visible, nothing in this window"
-- (empty/nil). It is deliberately never the sole reason a row is or is not
-- returned.
select exists (
    select 1 from analyzers
    where id = sqlc.arg(analyzer_id)
      and company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
      and deleted_at is null
);

-- name: ReadingRange :many
-- The join through analyzers carries the scope IN THIS QUERY: a caller
-- supplying another tenant's real analyzer_id gets zero rows because the
-- join excludes it, not because some earlier, separate check happened to
-- catch it first.
select mr.* from meter_readings mr
join analyzers a on a.id = mr.analyzer_id
where mr.analyzer_id = sqlc.arg(analyzer_id)
  and mr.kind = sqlc.arg(kind)::reading_kind
  and mr.ts >= sqlc.arg(from_ts)::timestamptz
  and mr.ts < sqlc.arg(to_ts)::timestamptz
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
order by mr.ts;

-- name: ReadingBoundaryAtOrBefore :one
-- The one hypertable read bounded on one side only, per repository.go's
-- BoundaryReadings doc: the last reading at or before `at`, however long ago.
-- Scoped the same way ReadingRange is, for the same reason.
select mr.* from meter_readings mr
join analyzers a on a.id = mr.analyzer_id
where mr.analyzer_id = sqlc.arg(analyzer_id)
  and mr.kind = sqlc.arg(kind)::reading_kind
  and mr.ts <= sqlc.arg(at)::timestamptz
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
order by mr.ts desc
limit 1;

-- name: ReadingLatest :one
select mr.* from meter_readings mr
join analyzers a on a.id = mr.analyzer_id
where mr.analyzer_id = sqlc.arg(analyzer_id)
  and mr.kind = sqlc.arg(kind)::reading_kind
  and mr.ts >= sqlc.arg(from_ts)::timestamptz
  and mr.ts < sqlc.arg(to_ts)::timestamptz
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
order by mr.ts desc
limit 1;
