-- AnalyticsRepository's queries, over the six continuous aggregates
-- (migration 00005). These are MATERIALISED reads for dashboards and
-- reports — never the billing-grade surface, which is ReadingRepository.
--
-- None of the six aggregates carry a company_id: the consumption_* ones join
-- through analyzers, the plant_production_* ones through power_plants (which
-- has no building_id at all, per PlantRepository's doc). analyzer_ids /
-- plant_ids EMPTY means "every id visible to the scope" — the same
-- never-widens-past-scope convention as CursorList.

-- name: AnalyticsConsumptionHourly :many
select c.* from consumption_hourly c
join analyzers a on a.id = c.analyzer_id
where a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
  and (cardinality(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')) = 0 or c.analyzer_id = any(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')))
  and c.bucket >= sqlc.arg(from_ts)::timestamptz
  and c.bucket < sqlc.arg(to_ts)::timestamptz
order by c.analyzer_id, c.bucket;

-- name: AnalyticsConsumptionDaily :many
select c.* from consumption_daily c
join analyzers a on a.id = c.analyzer_id
where a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
  and (cardinality(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')) = 0 or c.analyzer_id = any(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')))
  and c.bucket >= sqlc.arg(from_ts)::timestamptz
  and c.bucket < sqlc.arg(to_ts)::timestamptz
order by c.analyzer_id, c.bucket;

-- name: AnalyticsConsumptionMonthly :many
-- materialized_only = true (migration 00005): the open month is ABSENT from
-- this view, not stale. A caller wanting month-to-date must compose this with
-- the open period from consumption_daily or meter_readings; this repository
-- does not do that composition, and must not be "fixed" by flipping
-- materialized_only (see migration 00005's own header).
select c.* from consumption_monthly c
join analyzers a on a.id = c.analyzer_id
where a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
  and (cardinality(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')) = 0 or c.analyzer_id = any(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')))
  and c.bucket >= sqlc.arg(from_ts)::timestamptz
  and c.bucket < sqlc.arg(to_ts)::timestamptz
order by c.analyzer_id, c.bucket;

-- name: AnalyticsConsumptionYearly :many
-- materialized_only = true: the open year is ABSENT, same as
-- AnalyticsConsumptionMonthly above.
select c.* from consumption_yearly c
join analyzers a on a.id = c.analyzer_id
where a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
  and (cardinality(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')) = 0 or c.analyzer_id = any(coalesce(sqlc.arg(analyzer_ids)::uuid[], '{}')))
  and c.bucket >= sqlc.arg(from_ts)::timestamptz
  and c.bucket < sqlc.arg(to_ts)::timestamptz
order by c.analyzer_id, c.bucket;

-- name: AnalyticsProductionDaily :many
select p.* from plant_production_daily p
join power_plants pp on pp.id = p.plant_id
where pp.company_id = sqlc.arg(company_id)
  and pp.deleted_at is null
  and (cardinality(coalesce(sqlc.arg(plant_ids)::uuid[], '{}')) = 0 or p.plant_id = any(coalesce(sqlc.arg(plant_ids)::uuid[], '{}')))
  and p.bucket >= sqlc.arg(from_ts)::timestamptz
  and p.bucket < sqlc.arg(to_ts)::timestamptz
order by p.plant_id, p.bucket;

-- name: AnalyticsProductionMonthly :many
-- materialized_only = true: the open month is ABSENT, same reasoning as the
-- consumption side.
select p.* from plant_production_monthly p
join power_plants pp on pp.id = p.plant_id
where pp.company_id = sqlc.arg(company_id)
  and pp.deleted_at is null
  and (cardinality(coalesce(sqlc.arg(plant_ids)::uuid[], '{}')) = 0 or p.plant_id = any(coalesce(sqlc.arg(plant_ids)::uuid[], '{}')))
  and p.bucket >= sqlc.arg(from_ts)::timestamptz
  and p.bucket < sqlc.arg(to_ts)::timestamptz
order by p.plant_id, p.bucket;
