-- Generated building reports (migration 00010). Query names are prefixed
-- Report…

-- Folded minor (fix round 1): a report of a soft-deleted building must not
-- stay readable through the report's own row.
-- name: ReportGet :one
select * from reports
where reports.id = sqlc.arg(id) and reports.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or reports.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and exists (select 1 from buildings b where b.id = reports.building_id and b.deleted_at is null);

-- name: ReportList :many
select * from reports
where reports.company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or reports.building_id = any(sqlc.arg(building_ids)::uuid[]))
  and (sqlc.narg(building_id)::uuid is null or reports.building_id = sqlc.narg(building_id))
  and (sqlc.narg(report_type)::report_type is null or reports.type = sqlc.narg(report_type))
  and (sqlc.narg(period)::text is null or reports.period = sqlc.narg(period))
  -- Statuses is compared as text[], not report_status[]: pgx has no codec
  -- for an array of this custom enum, and fails even on an empty slice.
  and (cardinality(sqlc.arg(statuses)::text[]) = 0 or reports.status::text = any(sqlc.arg(statuses)::text[]))
  and exists (select 1 from buildings b where b.id = reports.building_id and b.deleted_at is null)
order by reports.created_at desc, reports.id
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- Critical Finding 1 (task-11a fix round 1): building_id is NOT NULL on
-- reports, and is a stored foreign key — Scope.AllowsBuilding is an
-- in-memory grant check that cannot know which company owns a building, so
-- an AllBuildings Scope's Go-side pre-check alone could store ANOTHER
-- TENANT's building id here. Validated in SQL instead, exactly as
-- TariffCreate/BillCreate validate theirs.
-- name: ReportUpsert :one
insert into reports (
  id, company_id, building_id, type, period, plant_selection, payload, pdf_path, excel_path,
  email_subject, email_body, status, error_message, processed_at, created_at, updated_at
)
select
  gen_random_uuid(), sqlc.arg(company_id), sqlc.arg(building_id), sqlc.arg(report_type),
  sqlc.arg(period), sqlc.arg(plant_selection), sqlc.arg(payload), sqlc.narg(pdf_path),
  sqlc.narg(excel_path), sqlc.narg(email_subject), sqlc.narg(email_body), sqlc.arg(status),
  sqlc.narg(error_message), sqlc.narg(processed_at), sqlc.arg(created_at), sqlc.arg(created_at)
where exists (
  select 1 from buildings b
  where b.id = sqlc.arg(building_id) and b.company_id = sqlc.arg(company_id) and b.deleted_at is null
    and (sqlc.arg(all_buildings)::boolean or b.id = any(sqlc.arg(building_ids)::uuid[]))
)
on conflict (building_id, type, period) do update set
  plant_selection = excluded.plant_selection,
  payload = excluded.payload,
  pdf_path = excluded.pdf_path,
  excel_path = excluded.excel_path,
  email_subject = excluded.email_subject,
  email_body = excluded.email_body,
  status = excluded.status,
  error_message = excluded.error_message,
  processed_at = excluded.processed_at,
  updated_at = excluded.updated_at
where reports.company_id = sqlc.arg(company_id)
returning *;

-- name: ReportUpdateStatus :one
update reports set status = sqlc.arg(status), error_message = sqlc.narg(error_message),
  processed_at = sqlc.arg(processed_at), updated_at = sqlc.arg(processed_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
returning *;
