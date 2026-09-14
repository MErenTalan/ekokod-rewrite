-- Generated building reports (migration 00010). Query names are prefixed
-- Report…

-- name: ReportGet :one
select * from reports
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]));

-- name: ReportList :many
select * from reports
where company_id = sqlc.arg(company_id)
  and (sqlc.arg(all_buildings)::boolean or building_id = any(sqlc.arg(building_ids)::uuid[]))
  and (sqlc.narg(building_id)::uuid is null or building_id = sqlc.narg(building_id))
  and (sqlc.narg(report_type)::report_type is null or type = sqlc.narg(report_type))
  and (sqlc.narg(period)::text is null or period = sqlc.narg(period))
  -- Statuses is compared as text[], not report_status[]: pgx has no codec
  -- for an array of this custom enum, and fails even on an empty slice.
  and (cardinality(sqlc.arg(statuses)::text[]) = 0 or status::text = any(sqlc.arg(statuses)::text[]))
order by created_at desc, id
limit sqlc.arg(limit_val) offset sqlc.arg(offset_val);

-- name: ReportUpsert :one
insert into reports (
  id, company_id, building_id, type, period, plant_selection, payload, pdf_path, excel_path,
  email_subject, email_body, status, error_message, processed_at, created_at, updated_at
) values (
  gen_random_uuid(), sqlc.arg(company_id), sqlc.arg(building_id), sqlc.arg(report_type),
  sqlc.arg(period), sqlc.arg(plant_selection), sqlc.arg(payload), sqlc.narg(pdf_path),
  sqlc.narg(excel_path), sqlc.narg(email_subject), sqlc.narg(email_body), sqlc.arg(status),
  sqlc.narg(error_message), sqlc.narg(processed_at), sqlc.arg(created_at), sqlc.arg(created_at)
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
