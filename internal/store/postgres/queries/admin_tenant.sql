-- AdminTenantRepository queries: the platform operator's view across tenants.

-- name: AdminCompanyList :many
select * from companies
where (sqlc.arg(include_deleted)::boolean or deleted_at is null)
  and (sqlc.arg(name_contains)::text = '' or name ilike '%' || sqlc.arg(name_contains)::text || '%')
  and (sqlc.narg(sector)::text is null or sector = sqlc.narg(sector))
order by lower(name)
limit sqlc.arg(page_limit) offset sqlc.arg(page_offset);

-- name: AdminCompanySetDemo :execrows
update companies set is_demo = sqlc.arg(is_demo) where id = sqlc.arg(id) and deleted_at is null;
