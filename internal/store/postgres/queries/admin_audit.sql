-- AdminAuditRepository queries (internal/store/postgres/admin).

-- name: AdminAppendPlatformAudit :one
-- company_id is always written as NULL here: a platform action belongs to
-- no tenant. The caller (internal/store/postgres/admin) refuses any entry
-- whose CompanyID is non-nil before this query ever runs.
--
-- coalesce(sqlc.narg(at)::timestamptz, now()): a caller that leaves CreatedAt at its zero
-- value gets the database's own now() rather than writing 0001-01-01.
insert into audit_log (company_id, user_id, action, entity_type, entity_id, before, after, ip, created_at)
values (null, sqlc.arg(user_id), sqlc.arg(action), sqlc.arg(entity_type), sqlc.arg(entity_id),
        sqlc.arg(before), sqlc.arg(after), sqlc.arg(ip), coalesce(sqlc.narg(at)::timestamptz, now()))
returning *;
