-- SMTP settings repository queries (migration 00008). Owned by Task 11c.
--
-- smtp_settings' primary key IS company_id -- one row per company, no id
-- column at all, and company_id is NOT NULL (`primary key references
-- companies(id)`), so unlike integration_credentials there is no
-- platform-wide (company_id NULL) row here to worry about. password_enc is
-- AES-256-GCM ciphertext; every query here reads/writes it as opaque bytes.
-- SMTPRepository.OpenPassword is the one place in Go that decrypts it, never
-- a query.

-- name: SMTPGet :one
select * from smtp_settings where company_id = $1;

-- SMTPUpsert's password_enc is sqlc.narg: a nil/empty password (Go seals
-- nothing and passes nil) must NOT wipe an existing row's stored secret. The
-- INSERT's own target list falls back to the existing row's password_enc via
-- a same-statement correlated subselect (never a separate read-then-write),
-- and the ON CONFLICT DO UPDATE branch falls back to the bare
-- `smtp_settings.password_enc` -- the just-locked, current row, not the
-- subselect's possibly-stale read -- so a concurrent password change can
-- never be clobbered by a concurrent nil-password host/port update.
--
-- password_enc is NOT NULL. When there is no existing row AND no password
-- was supplied, both fallbacks are NULL, the candidate tuple violates the
-- NOT NULL constraint, and Postgres raises 23502 before any conflict is
-- even considered (proven empirically: the not-null check runs on the
-- candidate tuple regardless of whether ON CONFLICT will fire) -- exactly
-- the "refuse a first insert with no password" rule, enforced by the schema
-- itself with no extra Go-side check or separate query.
-- name: SMTPUpsert :one
insert into smtp_settings (company_id, host, port, secure, username, password_enc, from_address)
select
    sqlc.arg(company_id)::uuid, sqlc.arg(host), sqlc.arg(port)::int, sqlc.arg(secure)::boolean,
    sqlc.arg(username),
    coalesce(
        sqlc.narg(password_enc)::bytea,
        (select s.password_enc from smtp_settings s where s.company_id = sqlc.arg(company_id)::uuid)
    ),
    sqlc.arg(from_address)::citext
on conflict (company_id) do update set
    host = excluded.host,
    port = excluded.port,
    secure = excluded.secure,
    username = excluded.username,
    password_enc = coalesce(sqlc.narg(password_enc)::bytea, smtp_settings.password_enc),
    from_address = excluded.from_address,
    updated_at = now()
returning *;

-- name: SMTPDelete :execrows
delete from smtp_settings where company_id = $1;
