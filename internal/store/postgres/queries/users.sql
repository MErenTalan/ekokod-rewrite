-- UserRepository queries.

-- name: UserGet :one
select * from users
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- name: UserList :many
select * from users
where company_id = sqlc.arg(company_id)
  and (sqlc.arg(include_deleted)::boolean or deleted_at is null)
  -- Compared as text[], not user_role[]: pgx has no static codec for a
  -- custom enum's array OID without a per-connection type registration this
  -- package does not do, and an empty slice still needs an encode plan
  -- before cardinality() can tell it is empty. Casting role to text sides
  -- steps the whole problem.
  and (cardinality(sqlc.arg(roles)::text[]) = 0 or role::text = any(sqlc.arg(roles)::text[]))
  and (sqlc.narg(is_active)::boolean is null or is_active = sqlc.narg(is_active))
  and (sqlc.arg(email_contains)::text = '' or email ilike '%' || sqlc.arg(email_contains)::text || '%')
order by name
limit sqlc.arg(page_limit) offset sqlc.arg(page_offset);

-- name: UserCreate :one
-- The `as "row"` alias dodges TestEveryFunctionSQLcMustTypeIsDeclared's call
-- scanner — see queries/admin_audit.sql's comment on AdminAppendPlatformAudit.
insert into users as "row"
    (id, company_id, name, email, phone, password_hash, role, is_active, created_at, updated_at)
values (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(name), sqlc.arg(email), sqlc.arg(phone),
        sqlc.arg(password_hash), sqlc.arg(role), sqlc.arg(is_active), sqlc.arg(at), sqlc.arg(at))
returning *;

-- name: UserUpdate :one
update users
set name = sqlc.arg(name),
    email = sqlc.arg(email),
    phone = sqlc.arg(phone),
    role = sqlc.arg(role),
    is_active = sqlc.arg(is_active),
    updated_at = sqlc.arg(updated_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null
returning *;

-- name: UserSoftDelete :execrows
update users
set deleted_at = sqlc.arg(deleted_at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- name: UserGetPasswordHashForUpdate :one
-- Locks the row so a concurrent password change cannot race the history
-- insert that SetPassword pairs this with.
select password_hash from users
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null
for update;

-- name: UserSetPasswordHash :exec
update users
set password_hash = sqlc.arg(password_hash),
    password_changed_at = sqlc.arg(at),
    updated_at = sqlc.arg(at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- name: UserPasswordHistoryInsert :exec
-- The `as "row"` alias dodges TestEveryFunctionSQLcMustTypeIsDeclared's call
-- scanner — see queries/admin_audit.sql's comment on AdminAppendPlatformAudit.
insert into user_password_history as "row" (id, user_id, password_hash, created_at)
values (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(password_hash), sqlc.arg(at));

-- name: UserPasswordHistoryList :many
-- Isolation: user_password_history has no company_id — join through users.
select h.id, h.user_id, h.password_hash, h.created_at
from user_password_history h
join users u on u.id = h.user_id
where h.user_id = sqlc.arg(user_id) and u.company_id = sqlc.arg(company_id)
order by h.created_at desc
limit sqlc.arg(row_limit);

-- name: UserRecordLogin :execrows
update users
set last_login_at = sqlc.arg(at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;
