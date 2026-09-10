-- name: GetBuilding :one
select * from buildings
where id = $1 and company_id = $2 and deleted_at is null;

-- name: ListBuildingsForScope :many
select * from buildings
where company_id = $1
  and (sqlc.arg(all_buildings)::boolean or id = any(sqlc.arg(building_ids)::uuid[]))
  and deleted_at is null
order by name;
