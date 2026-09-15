-- ForecastRepository's queries. forecasts and forecast_gaps have no
-- company_id: every method's DATA READ carries the scope in its own join
-- through analyzers, never a bare read guarded only by a separate Go-side
-- check. BulkInsert's COPY-into-staging-then-upsert is plain SQL in
-- forecasts.go, for the same reason ReadingRepository.BulkInsert's is.

-- name: ForecastVisibleAnalyzerIDs :many
-- Returns the subset of analyzer_ids visible to the scope, AND LOCKS them
-- (`for share`) for the rest of the caller's transaction — see
-- ReadingVisibleAnalyzerIDs in readings.sql. BulkInsert and RecordGaps both
-- run this inside their transaction, never on the bare pool before one
-- begins, and both compare the result's length against the batch's DISTINCT
-- analyzer ids before writing anything.
select id from analyzers
where id = any(sqlc.arg(analyzer_ids)::uuid[])
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null
for share;

-- name: ForecastAnalyzerVisible :one
-- Used ONLY to choose an error, never to gate a data read — see
-- ReadingAnalyzerVisible's comment in readings.sql for the same reasoning.
select exists (
    select 1 from analyzers
    where id = sqlc.arg(analyzer_id)
      and company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
      and deleted_at is null
);

-- name: ForecastRange :many
-- The join through analyzers carries the scope IN THIS QUERY.
select f.* from forecasts f
join analyzers a on a.id = f.analyzer_id
where f.analyzer_id = sqlc.arg(analyzer_id)
  and f.ts >= sqlc.arg(from_ts)::timestamptz
  and f.ts < sqlc.arg(to_ts)::timestamptz
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
order by f.ts, f.generated_at;

-- name: ForecastLatestRun :many
-- The most recent run (max generated_at) among the rows already inside the
-- window AND visible to the scope, per repository.go's LatestRun doc. When
-- there is no such row, latest.generated_at is null and the final select's
-- equality join matches nothing — an empty slice, not an error. Both the CTE
-- and the final select carry the scope join independently.
with latest as (
    select max(f.generated_at) as generated_at
    from forecasts f
    join analyzers a on a.id = f.analyzer_id
    where f.analyzer_id = sqlc.arg(analyzer_id)
      and f.ts >= sqlc.arg(from_ts)::timestamptz
      and f.ts < sqlc.arg(to_ts)::timestamptz
      and a.company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
      and a.deleted_at is null
)
select f.* from forecasts f
join analyzers a on a.id = f.analyzer_id
cross join latest
where f.analyzer_id = sqlc.arg(analyzer_id)
  and f.generated_at = latest.generated_at
  and f.ts >= sqlc.arg(from_ts)::timestamptz
  and f.ts < sqlc.arg(to_ts)::timestamptz
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
order by f.ts;

-- name: ForecastGapsByRun :many
select fg.* from forecast_gaps fg
join analyzers a on a.id = fg.analyzer_id
where fg.analyzer_id = sqlc.arg(analyzer_id)
  and fg.generated_at = sqlc.arg(generated_at)
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
order by fg.gap_start;

-- name: ForecastInsertGaps :exec
-- NOTE: TestEveryFunctionSQLcMustTypeIsDeclared currently flags the
-- "forecast_gaps (" below as a call to an undeclared function — a known
-- false positive being fixed in parallel on f1/task-8c; see the task report.
-- The batch-visibility check is ForecastVisibleAnalyzerIDs, run with `for
-- share` inside the SAME transaction as this insert (forecasts.go) — this
-- statement itself carries no further scope predicate because every
-- analyzer_id it writes has already been locked-and-verified visible in that
-- same transaction.
insert into forecast_gaps (id, analyzer_id, generated_at, gap_start, gap_end, missing_hours)
select unnest(sqlc.arg(ids)::uuid[]),
       unnest(sqlc.arg(analyzer_ids)::uuid[]),
       unnest(sqlc.arg(generated_ats)::timestamptz[]),
       unnest(sqlc.arg(gap_starts)::timestamptz[]),
       unnest(sqlc.arg(gap_ends)::timestamptz[]),
       unnest(sqlc.arg(missing_hours)::int[]);
