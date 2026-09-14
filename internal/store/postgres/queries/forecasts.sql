-- ForecastRepository's queries. forecasts and forecast_gaps have no
-- company_id: every method joins through analyzers. BulkInsert's
-- COPY-into-staging-then-upsert is plain SQL in forecasts.go, for the same
-- reason ReadingRepository.BulkInsert's is.

-- name: ForecastVisibleAnalyzerCount :one
select count(*) from analyzers
where id = any(sqlc.arg(analyzer_ids)::uuid[])
  and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null;

-- name: ForecastAnalyzerVisible :one
select exists (
    select 1 from analyzers
    where id = sqlc.arg(analyzer_id)
      and company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
      and deleted_at is null
);

-- name: ForecastRange :many
select * from forecasts
where analyzer_id = sqlc.arg(analyzer_id)
  and ts >= sqlc.arg(from_ts)::timestamptz
  and ts < sqlc.arg(to_ts)::timestamptz
order by ts, generated_at;

-- name: ForecastLatestRun :many
-- The most recent run (max generated_at) among the rows already inside the
-- window, per repository.go's LatestRun doc. When there is no row in the
-- window, latest.generated_at is null and the join below matches nothing —
-- an empty slice, not an error.
with latest as (
    select max(generated_at) as generated_at
    from forecasts
    where analyzer_id = sqlc.arg(analyzer_id)
      and ts >= sqlc.arg(from_ts)::timestamptz
      and ts < sqlc.arg(to_ts)::timestamptz
)
select f.* from forecasts f, latest
where f.analyzer_id = sqlc.arg(analyzer_id)
  and f.generated_at = latest.generated_at
  and f.ts >= sqlc.arg(from_ts)::timestamptz
  and f.ts < sqlc.arg(to_ts)::timestamptz
order by f.ts;

-- name: ForecastGapsByRun :many
select * from forecast_gaps
where analyzer_id = sqlc.arg(analyzer_id) and generated_at = sqlc.arg(generated_at)
order by gap_start;

-- name: ForecastInsertGaps :exec
-- NOTE: TestEveryFunctionSQLcMustTypeIsDeclared currently flags the
-- "forecast_gaps (" below as a call to an undeclared function — a known
-- false positive being fixed in parallel on f1/task-8c; see the task report.
insert into forecast_gaps (id, analyzer_id, generated_at, gap_start, gap_end, missing_hours)
select unnest(sqlc.arg(ids)::uuid[]),
       unnest(sqlc.arg(analyzer_ids)::uuid[]),
       unnest(sqlc.arg(generated_ats)::timestamptz[]),
       unnest(sqlc.arg(gap_starts)::timestamptz[]),
       unnest(sqlc.arg(gap_ends)::timestamptz[]),
       unnest(sqlc.arg(missing_hours)::int[]);
