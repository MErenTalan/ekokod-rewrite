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

-- name: SessionRevoke :execrows
-- revoked_at = coalesce(revoked_at, $at): re-revoking an already-revoked
-- session must keep the ORIGINAL instant, because replay detection compares
-- against it — overwriting it with a later "re-revoke" time would make a
-- genuine replay look like it happened before the session was revoked.
update sessions
set revoked_at = coalesce(sessions.revoked_at, sqlc.arg(at))
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

-- name: SessionCreate :one
-- Isolation: sess.UserID must be a user of company_id, checked by inserting
-- only when that user is visible; zero rows scan as ErrNoRows → ErrNotFound.
-- client, remember and rotated_from are F6a's rotation bookkeeping (R141).
insert into sessions (id, user_id, refresh_token_hash, device_fingerprint, user_agent, ip,
                      expires_at, revoked_at, created_at, client, remember, rotated_from)
select sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(refresh_token_hash), sqlc.arg(device_fingerprint),
       sqlc.arg(user_agent), sqlc.arg(ip), sqlc.arg(expires_at), sqlc.arg(revoked_at),
       coalesce(sqlc.narg(at)::timestamptz, now()), sqlc.arg(client), sqlc.arg(remember), sqlc.narg(rotated_from)
where exists (
    select 1 from users
    where id = sqlc.arg(user_id) and company_id = sqlc.arg(company_id) and deleted_at is null
)
returning *;

-- name: SessionRevokeLiveWithReason :execrows
-- Only a live session is revoked, so the first reason recorded is kept.
update sessions
set revoked_at = sqlc.arg(at), revoked_reason = sqlc.arg(reason)
where sessions.id = sqlc.arg(id)
  and sessions.revoked_at is null
  and sessions.user_id in (select users.id from users where users.company_id = sqlc.arg(company_id) and users.deleted_at is null);

-- name: SessionVisible :one
select exists(
    select 1 from sessions s join users u on u.id = s.user_id
    where s.id = sqlc.arg(id) and u.company_id = sqlc.arg(company_id) and u.deleted_at is null
);

-- name: SessionRevokeAllForUserWithReason :execrows
update sessions
set revoked_at = sqlc.arg(at), revoked_reason = sqlc.arg(reason)
where sessions.user_id = sqlc.arg(user_id)
  and sessions.revoked_at is null
  and (sqlc.narg(except_id)::uuid is null or sessions.id <> sqlc.narg(except_id))
  and sessions.user_id in (select users.id from users where users.company_id = sqlc.arg(company_id) and users.deleted_at is null);

-- name: SessionTouch :execrows
-- Matches every visible session (so a visible one never reads as missing) but
-- only moves last_used_at when it is unset or five minutes stale.
update sessions
set last_used_at = case
        when sessions.last_used_at is null or sessions.last_used_at < sqlc.arg(at)::timestamptz - interval '5 minutes'
        then sqlc.arg(at)::timestamptz
        else sessions.last_used_at end
where sessions.id = sqlc.arg(id)
  and sessions.user_id in (select users.id from users where users.company_id = sqlc.arg(company_id) and users.deleted_at is null);

-- name: SessionGetForUpdate :one
-- Rotate locks the old row so two concurrent refreshes cannot both rotate it.
select s.* from sessions s
join users u on u.id = s.user_id
where s.id = sqlc.arg(id) and u.company_id = sqlc.arg(company_id) and u.deleted_at is null
for update of s;
