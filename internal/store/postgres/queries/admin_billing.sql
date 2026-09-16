-- AdminBillingRepository queries: platform-wide discovery for the billing
-- dispatcher, called only from internal/store/postgres/admin/billing.go.

-- name: AdminBillableBuildings :many
select b.company_id, b.id as building_id, b.bill_cutoff_day
from buildings b
join companies c on c.id = b.company_id
where b.deleted_at is null and c.deleted_at is null
  and exists (select 1 from analyzers a where a.building_id = b.id and a.deleted_at is null)
order by b.company_id, b.id;
