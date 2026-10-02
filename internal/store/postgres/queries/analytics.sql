-- AnalyticsRepository's queries, over the four continuous aggregates
-- (migration 00005). These are MATERIALISED reads for dashboards and
-- reports — never the billing-grade surface, which is ReadingRepository.
--
-- None of the aggregates carry a company_id: the consumption_* ones join
-- through analyzers, the plant_production_* ones through power_plants (which
-- has no building_id at all, per PlantRepository's doc). analyzer_ids /
-- plant_ids are REQUIRED POSITIONAL parameters (repository.go's
-- AnalyticsRepository doc): EMPTY or nil means NO ROWS, fail-closed,
-- identical to Scope.BuildingIDs — there is no "every id visible to scope"
-- form. `c.analyzer_id = any(sqlc.arg(analyzer_ids)::uuid[])` gets this for
-- free: a nil slice encodes to NULL::uuid[], and `x = any(NULL)` is NULL,
-- which the WHERE clause reads as false, matching zero rows; an empty (but
-- non-nil) slice encodes to '{}', and `x = any('{}')` is false outright.

-- name: AnalyticsConsumptionHourly :many
select c.* from consumption_hourly c
join analyzers a on a.id = c.analyzer_id
where a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
  and c.analyzer_id = any(sqlc.arg(analyzer_ids)::uuid[])
  and c.bucket >= sqlc.arg(from_ts)::timestamptz
  and c.bucket < sqlc.arg(to_ts)::timestamptz
order by c.analyzer_id, c.bucket;

-- name: AnalyticsConsumptionDaily :many
-- Buckets are Europe/Istanbul-local instants (migration 00005): a day
-- boundary here is midnight IN ISTANBUL, not UTC. A caller that constructs
-- TimeRange bounds from UTC calendar days will see this window's edges land
-- mid-bucket; see AnalyticsRepository's ConsumptionDaily doc comment.
select c.* from consumption_daily c
join analyzers a on a.id = c.analyzer_id
where a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
  and c.analyzer_id = any(sqlc.arg(analyzer_ids)::uuid[])
  and c.bucket >= sqlc.arg(from_ts)::timestamptz
  and c.bucket < sqlc.arg(to_ts)::timestamptz
order by c.analyzer_id, c.bucket;

-- name: AnalyticsConsumptionRollup :many
-- R499: months and years are rolled up from consumption_daily (real-time),
-- so the open period is whole and nothing waits for a refresh. width is
-- '1 month' or '1 year'; buckets are Europe/Istanbul-local, and from_ts/to_ts
-- must be bucket boundaries of width. The delta columns are NULL: the
-- repository differences boundaries itself (R471).
select
    d.analyzer_id,
    time_bucket(sqlc.arg(width)::interval, d.bucket, 'Europe/Istanbul')::timestamptz as bucket,
    (first(d.active_import_start, d.bucket) filter (where d.active_import_start is not null))::numeric as active_import_start,
    (last(d.active_import_end, d.bucket) filter (where d.active_import_end is not null))::numeric as active_import_end,
    null::numeric as active_consumption,
    null::numeric as inductive_consumption,
    null::numeric as capacitive_consumption,
    null::numeric as t1_consumption,
    null::numeric as t2_consumption,
    null::numeric as t3_consumption,
    null::numeric as active_generation,
    null::numeric as inductive_generation,
    null::numeric as capacitive_generation,
    null::numeric as t1_generation,
    null::numeric as t2_generation,
    null::numeric as t3_generation,
    max(d.max_demand_kw)::numeric as max_demand_kw,
    (last(d.active_index, d.bucket) filter (where d.active_index is not null))::numeric as active_index,
    (last(d.inductive_index, d.bucket) filter (where d.inductive_index is not null))::numeric as inductive_index,
    (last(d.capacitive_index, d.bucket) filter (where d.capacitive_index is not null))::numeric as capacitive_index,
    (last(d.t1_index, d.bucket) filter (where d.t1_index is not null))::numeric as t1_index,
    (last(d.t2_index, d.bucket) filter (where d.t2_index is not null))::numeric as t2_index,
    (last(d.t3_index, d.bucket) filter (where d.t3_index is not null))::numeric as t3_index,
    (last(d.active_generation_index, d.bucket) filter (where d.active_generation_index is not null))::numeric as active_generation_index,
    sum(d.reading_count)::bigint as reading_count,
    min(d.first_ts)::timestamptz as first_ts,
    (first(d.inductive_consumption_start, d.bucket) filter (where d.inductive_consumption_start is not null))::numeric as inductive_consumption_start,
    (first(d.capacitive_consumption_start, d.bucket) filter (where d.capacitive_consumption_start is not null))::numeric as capacitive_consumption_start,
    (first(d.t1_consumption_start, d.bucket) filter (where d.t1_consumption_start is not null))::numeric as t1_consumption_start,
    (first(d.t2_consumption_start, d.bucket) filter (where d.t2_consumption_start is not null))::numeric as t2_consumption_start,
    (first(d.t3_consumption_start, d.bucket) filter (where d.t3_consumption_start is not null))::numeric as t3_consumption_start,
    (first(d.active_generation_start, d.bucket) filter (where d.active_generation_start is not null))::numeric as active_generation_start,
    (first(d.inductive_generation_start, d.bucket) filter (where d.inductive_generation_start is not null))::numeric as inductive_generation_start,
    (first(d.capacitive_generation_start, d.bucket) filter (where d.capacitive_generation_start is not null))::numeric as capacitive_generation_start,
    (first(d.t1_generation_start, d.bucket) filter (where d.t1_generation_start is not null))::numeric as t1_generation_start,
    (first(d.t2_generation_start, d.bucket) filter (where d.t2_generation_start is not null))::numeric as t2_generation_start,
    (first(d.t3_generation_start, d.bucket) filter (where d.t3_generation_start is not null))::numeric as t3_generation_start,
    (last(d.inductive_generation_index, d.bucket) filter (where d.inductive_generation_index is not null))::numeric as inductive_generation_index,
    (last(d.capacitive_generation_index, d.bucket) filter (where d.capacitive_generation_index is not null))::numeric as capacitive_generation_index,
    (last(d.t1_generation_index, d.bucket) filter (where d.t1_generation_index is not null))::numeric as t1_generation_index,
    (last(d.t2_generation_index, d.bucket) filter (where d.t2_generation_index is not null))::numeric as t2_generation_index,
    (last(d.t3_generation_index, d.bucket) filter (where d.t3_generation_index is not null))::numeric as t3_generation_index
from consumption_daily d
join analyzers a on a.id = d.analyzer_id
where a.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or a.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and a.deleted_at is null
  and d.analyzer_id = any(sqlc.arg(analyzer_ids)::uuid[])
  and d.bucket >= sqlc.arg(from_ts)::timestamptz
  and d.bucket < sqlc.arg(to_ts)::timestamptz
group by d.analyzer_id, 2
order by d.analyzer_id, 2;

-- name: AnalyticsProductionDaily :many
-- Buckets are Europe/Istanbul-local instants, same reasoning as the
-- consumption side.
select p.* from plant_production_daily p
join power_plants pp on pp.id = p.plant_id
where pp.company_id = sqlc.arg(company_id)
  and pp.deleted_at is null
  and p.plant_id = any(sqlc.arg(plant_ids)::uuid[])
  and p.bucket >= sqlc.arg(from_ts)::timestamptz
  and p.bucket < sqlc.arg(to_ts)::timestamptz
order by p.plant_id, p.bucket;

-- name: AnalyticsProductionMonthly :many
-- materialized_only = true: the open month is ABSENT, same reasoning as the
-- consumption side. Buckets are Europe/Istanbul-local instants.
select p.* from plant_production_monthly p
join power_plants pp on pp.id = p.plant_id
where pp.company_id = sqlc.arg(company_id)
  and pp.deleted_at is null
  and p.plant_id = any(sqlc.arg(plant_ids)::uuid[])
  and p.bucket >= sqlc.arg(from_ts)::timestamptz
  and p.bucket < sqlc.arg(to_ts)::timestamptz
order by p.plant_id, p.bucket;
