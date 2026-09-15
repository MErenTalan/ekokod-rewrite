-- ProductionRepository's queries. plant_production has no company_id: every
-- method's DATA READ carries the scope in its own join through power_plants
-- (which itself has no building_id — a Scope narrows a plant to the company
-- and no further, per repository.go's PlantRepository doc), never a bare
-- read guarded only by a separate Go-side check. BulkInsert's
-- COPY-into-staging-then-upsert is plain SQL in production.go, for the same
-- reason ReadingRepository.BulkInsert's is not here.

-- name: ProductionVisiblePlantIDs :many
-- Returns the subset of plant_ids visible to the scope, AND LOCKS them
-- (`for share`) for the rest of the caller's transaction — see
-- ReadingVisibleAnalyzerIDs in readings.sql for why a lock, not a plain
-- count, closes the TOCTOU gap a pre-check run before the write leaves open.
select id from power_plants
where id = any(sqlc.arg(plant_ids)::uuid[])
  and company_id = sqlc.arg(company_id)
  and deleted_at is null
order by id
for share;

-- name: ProductionValidDevicePairCount :one
-- Counts how many of the (plant_id, device_id) pairs in the batch are a real
-- device of that same plant. BulkInsert compares this against the number of
-- DISTINCT pairs in the batch: a mismatch means at least one row's device_id
-- does not belong to that row's plant_id. Not a tenant-scope check (the plant
-- itself is already locked by ProductionVisiblePlantIDs above) — this is a
-- referential-integrity check within an already-scoped plant.
select count(*) from (
    select unnest(sqlc.arg(plant_ids)::uuid[]) as plant_id,
           unnest(sqlc.arg(device_ids)::uuid[]) as device_id
) pairs
join power_plant_devices d on d.plant_id = pairs.plant_id and d.id = pairs.device_id;

-- name: ProductionPlantVisible :one
-- Used ONLY to choose an error, never to gate a data read — see
-- ReadingAnalyzerVisible's comment in readings.sql for the same reasoning.
select exists (
    select 1 from power_plants
    where id = sqlc.arg(plant_id) and company_id = sqlc.arg(company_id) and deleted_at is null
);

-- name: ProductionRange :many
-- The join through power_plants carries the scope IN THIS QUERY.
select pp.* from plant_production pp
join power_plants p on p.id = pp.plant_id
where pp.plant_id = sqlc.arg(plant_id)
  and pp.ts >= sqlc.arg(from_ts)::timestamptz
  and pp.ts < sqlc.arg(to_ts)::timestamptz
  and p.company_id = sqlc.arg(company_id)
  and p.deleted_at is null
order by pp.ts;

-- name: ProductionLatest :one
select pp.* from plant_production pp
join power_plants p on p.id = pp.plant_id
where pp.plant_id = sqlc.arg(plant_id)
  and pp.ts >= sqlc.arg(from_ts)::timestamptz
  and pp.ts < sqlc.arg(to_ts)::timestamptz
  and p.company_id = sqlc.arg(company_id)
  and p.deleted_at is null
order by pp.ts desc
limit 1;
