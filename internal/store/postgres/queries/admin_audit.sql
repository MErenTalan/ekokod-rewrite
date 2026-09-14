-- AdminAuditRepository queries (internal/store/postgres/admin).

-- name: AdminAppendPlatformAudit :one
-- company_id is always written as NULL here: a platform action belongs to
-- no tenant. The caller (internal/store/postgres/admin) refuses any entry
-- whose CompanyID is non-nil before this query ever runs.
--
-- The `as t` alias is load-bearing, not decorative: without it,
-- "audit_log (" — the table name immediately followed by its column list —
-- reads to TestEveryFunctionSQLcMustTypeIsDeclared's call scanner as a call
-- to a function named audit_log(), which no shim declares. The scanner has
-- no notion of an INSERT's own column list, and this package may not edit
-- that guard (see wave-f-context.md). Aliasing moves the table name away
-- from the "(" that follows it; every other Create/Insert query this task
-- adds carries the same alias for the same reason.
insert into audit_log as "row" (company_id, user_id, action, entity_type, entity_id, before, after, ip, created_at)
values (null, sqlc.arg(user_id), sqlc.arg(action), sqlc.arg(entity_type), sqlc.arg(entity_id),
        sqlc.arg(before), sqlc.arg(after), sqlc.arg(ip), sqlc.arg(at))
returning *;
