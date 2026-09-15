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

-- name: SMTPUpsert :one
insert into smtp_settings (company_id, host, port, secure, username, password_enc, from_address)
values (
    sqlc.arg(company_id)::uuid, sqlc.arg(host), sqlc.arg(port)::int, sqlc.arg(secure)::boolean,
    sqlc.arg(username), sqlc.arg(password_enc)::bytea, sqlc.arg(from_address)::citext
)
on conflict (company_id) do update set
    host = excluded.host,
    port = excluded.port,
    secure = excluded.secure,
    username = excluded.username,
    password_enc = excluded.password_enc,
    from_address = excluded.from_address,
    updated_at = now()
returning *;

-- name: SMTPDelete :execrows
delete from smtp_settings where company_id = $1;
