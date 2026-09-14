-- AnomalyRepository's queries. consumption_anomalies has no company_id:
-- every method joins through analyzers. Resolve additionally requires the
-- resolver to be a user of the scope's company, checked by AnomalyUserVisible.

-- name: AnomalyGet :one
select an.* from consumption_anomalies an
join analyzers a on a.id = an.analyzer_id
where an.id = sqlc.arg(id)
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null;

-- name: AnomalyList :many
select an.* from consumption_anomalies an
join analyzers a on a.id = an.analyzer_id
where a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
  and (cardinality(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')) = 0 or an.analyzer_id = any(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')))
  and (not sqlc.arg(unresolved)::boolean or an.resolved_at is null)
  and (sqlc.narg(reason)::text is null or an.reason = sqlc.narg(reason))
  and (sqlc.narg(from_ts)::timestamptz is null or an.period_start >= sqlc.narg(from_ts))
  and (sqlc.narg(to_ts)::timestamptz is null or an.period_start < sqlc.narg(to_ts))
order by an.period_start desc, an.id
limit sqlc.arg(page_limit)
offset sqlc.arg(page_offset);

-- name: AnomalyAnalyzerVisible :one
select exists (
    select 1 from analyzers
    where id = sqlc.arg(analyzer_id)
      and company_id = sqlc.arg(company_id)
      and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
      and deleted_at is null
);

-- name: AnomalyCreate :one
-- NOTE: TestEveryFunctionSQLcMustTypeIsDeclared currently flags the
-- "consumption_anomalies (" below as a call to an undeclared function — a
-- known false positive (the guard has no notion of an INSERT's target
-- column list) being fixed in parallel on f1/task-8c. Left as ordinary SQL
-- rather than restructured around it; see the task report.
insert into consumption_anomalies (analyzer_id, period_start, period_end, reason, detail)
select a.id, sqlc.arg(period_start)::timestamptz, sqlc.arg(period_end)::timestamptz, sqlc.arg(reason)::text, sqlc.arg(detail)
from analyzers a
where a.id = sqlc.arg(analyzer_id)
  and a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
returning *;

-- name: AnomalyUserVisible :one
select exists (
    select 1 from users
    where id = sqlc.arg(user_id) and company_id = sqlc.arg(company_id) and deleted_at is null
);

-- name: AnomalyResolve :one
update consumption_anomalies an set
    resolved_at = sqlc.arg(at)::timestamptz,
    resolved_by = sqlc.arg(resolved_by),
    resolution = sqlc.arg(resolution)::text,
    override_values = sqlc.arg(override_values)
where an.id = sqlc.arg(id)
  and an.analyzer_id in (
      select a.id from analyzers a
      where a.company_id = sqlc.arg(company_id)
        and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
        and a.deleted_at is null
  )
returning *;
