-- AdminSolarRepository: platform-wide discovery for the iSolar ticks (R288),
-- called only from internal/store/postgres/admin/solar.go.

-- name: AdminLinkedPlants :many
select p.company_id, p.id as plant_id
from power_plants p
join companies c on c.id = p.company_id
where p.deleted_at is null and c.deleted_at is null and p.isolar_ps_id is not null
order by p.company_id, p.id;
