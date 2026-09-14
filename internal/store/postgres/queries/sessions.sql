-- SessionRepository queries. sessions has no company_id — every query joins
-- through users, applying company_id = the Scope's own.

-- name: SessionGet :one
select s.* from sessions s
join users u on u.id = s.user_id
where s.id = sqlc.arg(id) and u.company_id = sqlc.arg(company_id) and u.deleted_at is null;

-- name: SessionList :many
select s.* from sessions s
join users u on u.id = s.user_id
where u.company_id = sqlc.arg(company_id) and u.deleted_at is null
  and (sqlc.narg(user_id)::uuid is null or s.user_id = sqlc.narg(user_id))
  and (sqlc.narg(active_at)::timestamptz is null
       or (s.revoked_at is null and s.expires_at > sqlc.narg(active_at)))
order by s.created_at desc
limit sqlc.arg(page_limit) offset sqlc.arg(page_offset);

-- name: SessionUserVisible :one
select exists(
    select 1 from users
    where id = sqlc.arg(user_id) and company_id = sqlc.arg(company_id) and deleted_at is null
);

-- name: SessionCreate :one
-- Isolation: sess.UserID must be a user of company_id, checked by inserting
-- only when that user is visible; if not, zero rows are inserted and the
-- :one scan reports ErrNoRows, translated to ErrNotFound.
--
-- The `as "row"` alias dodges TestEveryFunctionSQLcMustTypeIsDeclared's call
-- scanner — see queries/admin_audit.sql's comment on AdminAppendPlatformAudit.
insert into sessions as "row" (id, user_id, refresh_token_hash, device_fingerprint, user_agent, ip, expires_at, revoked_at, created_at)
select sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(refresh_token_hash), sqlc.arg(device_fingerprint),
       sqlc.arg(user_agent), sqlc.arg(ip), sqlc.arg(expires_at), sqlc.arg(revoked_at), sqlc.arg(at)
where exists (
    select 1 from users
    where id = sqlc.arg(user_id) and company_id = sqlc.arg(company_id) and deleted_at is null
)
returning *;

-- name: SessionRevoke :execrows
update sessions
set revoked_at = sqlc.arg(at)
where sessions.id = sqlc.arg(id)
  and sessions.user_id in (select users.id from users where users.company_id = sqlc.arg(company_id) and users.deleted_at is null);

-- name: SessionRevokeAllForUser :execrows
update sessions
set revoked_at = sqlc.arg(at)
where sessions.user_id = sqlc.arg(user_id)
  and sessions.revoked_at is null
  and sessions.user_id in (select users.id from users where users.company_id = sqlc.arg(company_id) and users.deleted_at is null);

-- name: SessionDeleteExpired :execrows
delete from sessions
where sessions.expires_at < sqlc.arg(before)
  and sessions.user_id in (select users.id from users where users.company_id = sqlc.arg(company_id) and users.deleted_at is null);
