-- ReadingRepository's queries. meter_readings has no company_id (see
-- repository.go's "ROWS WITHOUT company_id" header): every method joins
-- through analyzers, or checks analyzer visibility explicitly before reading
-- the hypertable directly, which is why the *_range/*_latest queries below do
-- not themselves join anything.
--
-- BulkInsert's COPY-into-staging-then-upsert is NOT here: a per-call
-- temporary table cannot appear in sqlc's schema catalogue (it does not exist
-- until the transaction that creates it), so that statement is built and run
-- as plain SQL in readings.go. The queries below back only the pre-write
-- visibility check and the three plain reads.

-- name: ReadingVisibleAnalyzerCount :one
-- Counts how many of analyzer_ids are visible to the scope. BulkInsert
-- compares this against the number of DISTINCT analyzer ids in the batch: a
-- mismatch means at least one row's analyzer is not visible, and the whole
-- batch is refused.
select count(*) from analyzers
where id = any(sqlc.arg(analyzer_ids)::uuid[])
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null;

-- name: ReadingAnalyzerVisible :one
select exists (
    select 1 from analyzers
    where id = sqlc.arg(analyzer_id)
      and company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
      and deleted_at is null
);

-- name: ReadingRange :many
select * from meter_readings
where analyzer_id = sqlc.arg(analyzer_id)
  and kind = sqlc.arg(kind)::reading_kind
  and ts >= sqlc.arg(from_ts)::timestamptz
  and ts < sqlc.arg(to_ts)::timestamptz
order by ts;

-- name: ReadingBoundaryAtOrBefore :one
-- The one hypertable read bounded on one side only, per repository.go's
-- BoundaryReadings doc: the last reading at or before `at`, however long ago.
select * from meter_readings
where analyzer_id = sqlc.arg(analyzer_id)
  and kind = sqlc.arg(kind)::reading_kind
  and ts <= sqlc.arg(at)::timestamptz
order by ts desc
limit 1;

-- name: ReadingLatest :one
select * from meter_readings
where analyzer_id = sqlc.arg(analyzer_id)
  and kind = sqlc.arg(kind)::reading_kind
  and ts >= sqlc.arg(from_ts)::timestamptz
  and ts < sqlc.arg(to_ts)::timestamptz
order by ts desc
limit 1;
