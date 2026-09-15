-- AdminAuthRepository queries (internal/store/postgres/admin). These are the
-- ONLY unscoped reads in the schema: a login and a refresh both carry a
-- credential and no tenant, so there is no Scope to filter by.

-- name: AdminUserByEmail :one
-- users.email is citext, so the match is case-insensitive. A soft-deleted
-- user, or a user whose company is soft-deleted, is excluded here rather
-- than left for the caller to notice.
select u.* from users u
join companies c on c.id = u.company_id
where u.email = $1 and u.deleted_at is null and c.deleted_at is null;

-- name: AdminSessionByRefreshTokenHash :one
-- Revoked and expired sessions ARE returned — rejecting them is the caller's
-- job (see store.SessionRepository.Revoke's doc). Only a soft-deleted user or
-- a soft-deleted company excludes the row, because a session naming either
-- one has nothing for the caller to act on.
select
    s.id, s.user_id, s.refresh_token_hash, s.device_fingerprint, s.user_agent, s.ip,
    s.expires_at, s.revoked_at, s.created_at as session_created_at,
    u.company_id, u.name, u.email, u.phone, u.password_hash, u.password_changed_at,
    u.role, u.is_active, u.last_login_at, u.created_at as user_created_at,
    u.updated_at, u.deleted_at
from sessions s
join users u on u.id = s.user_id
join companies c on c.id = u.company_id
where s.refresh_token_hash = $1 and u.deleted_at is null and c.deleted_at is null;
