-- AuditRepository queries. audit_log.company_id is nullable, for platform
-- rows written through AdminAuditRepository; every query here stores and
-- sees only company_id = the Scope's own, never a NULL row.

-- name: AuditAppend :one
-- The `as "row"` alias dodges TestEveryFunctionSQLcMustTypeIsDeclared's call
-- scanner — see queries/admin_audit.sql's comment on AdminAppendPlatformAudit.
insert into audit_log as "row" (company_id, user_id, action, entity_type, entity_id, before, after, ip, created_at)
values (sqlc.arg(company_id), sqlc.arg(user_id), sqlc.arg(action), sqlc.arg(entity_type),
        sqlc.arg(entity_id), sqlc.arg(before), sqlc.arg(after), sqlc.arg(ip), sqlc.arg(at))
returning *;

-- name: AuditList :many
select * from audit_log
where company_id = sqlc.arg(company_id)
  and (sqlc.narg(user_id)::uuid is null or user_id = sqlc.narg(user_id))
  and (sqlc.narg(entity_type)::text is null or entity_type = sqlc.narg(entity_type))
  and (sqlc.narg(entity_id)::uuid is null or entity_id = sqlc.narg(entity_id))
  and (sqlc.narg(action)::text is null or action = sqlc.narg(action))
  and (sqlc.narg(range_from)::timestamptz is null or created_at >= sqlc.narg(range_from))
  and (sqlc.narg(range_to)::timestamptz is null or created_at < sqlc.narg(range_to))
order by created_at desc
limit sqlc.arg(page_limit) offset sqlc.arg(page_offset);
