-- ProductionRepository's queries. plant_production has no company_id: every
-- method joins through power_plants, which itself has no building_id — a
-- Scope narrows a plant to the company and no further (repository.go's
-- PlantRepository doc). BulkInsert's COPY-into-staging-then-upsert is plain
-- SQL in production.go, for the same reason ReadingRepository.BulkInsert's is
-- not here.

-- name: ProductionVisiblePlantCount :one
select count(*) from power_plants
where id = any(sqlc.arg(plant_ids)::uuid[])
  and company_id = sqlc.arg(company_id)
  and deleted_at is null;

-- name: ProductionValidDevicePairCount :one
-- Counts how many of the (plant_id, device_id) pairs in the batch are a real
-- device of that same plant. BulkInsert compares this against the number of
-- DISTINCT pairs in the batch: a mismatch means at least one row's device_id
-- does not belong to that row's plant_id.
select count(*) from (
    select unnest(sqlc.arg(plant_ids)::uuid[]) as plant_id,
           unnest(sqlc.arg(device_ids)::uuid[]) as device_id
) pairs
join power_plant_devices d on d.plant_id = pairs.plant_id and d.id = pairs.device_id;

-- name: ProductionPlantVisible :one
select exists (
    select 1 from power_plants
    where id = sqlc.arg(plant_id) and company_id = sqlc.arg(company_id) and deleted_at is null
);

-- name: ProductionRange :many
select * from plant_production
where plant_id = sqlc.arg(plant_id)
  and ts >= sqlc.arg(from_ts)::timestamptz
  and ts < sqlc.arg(to_ts)::timestamptz
order by ts;

-- name: ProductionLatest :one
select * from plant_production
where plant_id = sqlc.arg(plant_id)
  and ts >= sqlc.arg(from_ts)::timestamptz
  and ts < sqlc.arg(to_ts)::timestamptz
order by ts desc
limit 1;
