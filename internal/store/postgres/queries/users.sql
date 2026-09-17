-- UserRepository queries.

-- name: UserGet :one
select * from users
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- name: UserList :many
select * from users
where company_id = sqlc.arg(company_id)
  and (sqlc.arg(include_deleted)::boolean or deleted_at is null)
  -- pgx has no static codec for a custom enum's array OID without a
  -- per-connection type registration this package does not do, and an empty
  -- slice still needs an encode plan before cardinality() can tell it is
  -- empty — so the PARAMETER is bound as text[] (a plain []string) and cast
  -- to user_role[] only inside SQL, once bound. The COLUMN itself is left
  -- uncast, so an index on role (should one ever be added) stays usable; an
  -- array element that is not a valid user_role raises a loud Postgres
  -- error rather than silently matching nothing.
  and (cardinality(sqlc.arg(roles)::text[]) = 0 or role = any(sqlc.arg(roles)::text[]::user_role[]))
  and (sqlc.narg(is_active)::boolean is null or is_active = sqlc.narg(is_active))
  and (sqlc.arg(email_contains)::text = '' or email ilike '%' || sqlc.arg(email_contains)::text || '%')
order by name
limit sqlc.arg(page_limit) offset sqlc.arg(page_offset);

-- name: UserCreate :one
-- coalesce(sqlc.narg(at)::timestamptz, now()): a caller that leaves CreatedAt at its zero
-- value gets the database's own now() rather than writing 0001-01-01.
insert into users
    (id, company_id, name, email, phone, password_hash, role, is_active, ui_preferences, locale, created_at, updated_at)
values (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(name), sqlc.arg(email), sqlc.arg(phone),
        sqlc.arg(password_hash), sqlc.arg(role), sqlc.arg(is_active), sqlc.narg(ui_preferences),
        coalesce(nullif(sqlc.arg(locale)::text, ''), 'tr'),
        coalesce(sqlc.narg(at)::timestamptz, now()), coalesce(sqlc.narg(at)::timestamptz, now()))
returning *;

-- name: UserUpdate :one
update users
set name = sqlc.arg(name),
    email = sqlc.arg(email),
    phone = sqlc.arg(phone),
    role = sqlc.arg(role),
    is_active = sqlc.arg(is_active),
    ui_preferences = sqlc.narg(ui_preferences),
    -- an unset (empty) locale keeps the stored one, so pre-F6 callers stay valid
    locale = coalesce(nullif(sqlc.arg(locale)::text, ''), locale),
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
insert into user_password_history (id, user_id, password_hash, created_at)
values (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(password_hash), sqlc.arg(at));

-- name: UserPasswordHistoryList :many
-- Isolation: user_password_history has no company_id — join through users.
-- u.deleted_at is null matters here specifically: without it, a soft-deleted
-- user's password history stays reachable through this join forever.
select h.id, h.user_id, h.password_hash, h.created_at
from user_password_history h
join users u on u.id = h.user_id
where h.user_id = sqlc.arg(user_id) and u.company_id = sqlc.arg(company_id) and u.deleted_at is null
order by h.created_at desc
limit sqlc.arg(row_limit);

-- name: UserRecordLogin :execrows
update users
set last_login_at = sqlc.arg(at)
where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null;

-- name: UserVisible :one
-- The User-prefixed parent-visibility lookup UserRepository.PasswordHistory
-- uses. Session-prefixed SessionUserVisible (queries/sessions.sql) exists
-- for SessionRepository's own use and is a different query in the same
-- shape — ruling 5 in wave-f-context.md is that a parent-visibility lookup
-- lives under its OWN repository's prefix, not borrowed across one.
select exists(
    select 1 from users
    where id = sqlc.arg(id) and company_id = sqlc.arg(company_id) and deleted_at is null
);
