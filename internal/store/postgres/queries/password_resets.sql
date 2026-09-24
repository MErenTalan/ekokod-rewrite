-- PasswordResetRepository queries. password_reset_tokens has no company_id —
-- every query joins through users.

-- name: PasswordResetInvalidateUnused :exec
update password_reset_tokens
set used_at = sqlc.arg(at)
where password_reset_tokens.user_id = sqlc.arg(user_id)
  and password_reset_tokens.used_at is null
  and password_reset_tokens.user_id in (select users.id from users where users.company_id = sqlc.arg(company_id) and users.deleted_at is null);

-- name: PasswordResetCreate :one
insert into password_reset_tokens (id, user_id, token_hash, expires_at, created_at)
select sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(expires_at), coalesce(sqlc.narg(at)::timestamptz, now())
where exists (
    select 1 from users
    where id = sqlc.arg(user_id) and company_id = sqlc.arg(company_id) and deleted_at is null
)
returning *;

-- name: PasswordResetVisible :one
select exists(
    select 1 from password_reset_tokens p join users u on u.id = p.user_id
    where p.id = sqlc.arg(id) and u.company_id = sqlc.arg(company_id) and u.deleted_at is null
);

-- name: PasswordResetMarkUsed :execrows
update password_reset_tokens
set used_at = sqlc.arg(at)
where password_reset_tokens.id = sqlc.arg(id)
  and password_reset_tokens.used_at is null
  and password_reset_tokens.user_id in (select users.id from users where users.company_id = sqlc.arg(company_id) and users.deleted_at is null);
