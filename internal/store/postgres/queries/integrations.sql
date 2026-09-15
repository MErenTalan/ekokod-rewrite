-- Integration catalogue and credential repository queries (migration 00008).
-- Owned by Task 11b.
--
-- integration_definitions is platform-wide (no company_id at all): reads are
-- scoped (Scope is validated, in Go, before the query runs) but the query
-- itself carries no tenant predicate because there is nothing to predicate
-- on — 03-target-architecture's "reads are scoped, writes are admin" rule for
-- platform data.
--
-- integration_credentials.secret_enc / extra_enc are AES-256-GCM ciphertext.
-- Every query here selects the whole row including the ciphertext columns;
-- IntegrationRepository.OpenSecret is the one place in Go that decrypts them,
-- never a query.

-- name: IntegrationListDefinitions :many
select * from integration_definitions order by provider, subtype;

-- name: IntegrationGetDefinition :one
select * from integration_definitions where provider = $1 and subtype = $2;

-- name: IntegrationGetCredential :one
select * from integration_credentials where id = $1 and company_id = $2;

-- IntegrationRepository.Credential is keyed by DEFINITION id, not the
-- credential's own id: integration_credentials(company_id, definition_id) is
-- unique, so a company has at most one credential per definition, and that
-- is the natural lookup this method's own signature promises.
-- name: IntegrationGetCredentialByDefinition :one
select * from integration_credentials where definition_id = $1 and company_id = $2;

-- name: IntegrationListCredentials :many
select * from integration_credentials where company_id = $1 order by created_at, id;

-- IntegrationUpsertCredential's definition_id must name a real, existing
-- integration_definitions row: the `where exists (...)` clause makes that
-- atomic with the insert, rather than a separate check-then-insert. A bad
-- definition_id makes the INSERT select zero rows, and the repository
-- translates that into ErrNotFound.
-- name: IntegrationUpsertCredential :one
insert into integration_credentials
    (id, company_id, definition_id, username, secret_enc, extra_enc, settings,
     pm5340_url, isolar_region, is_active)
select
    coalesce(nullif(sqlc.arg(id)::uuid, '00000000-0000-0000-0000-000000000000'::uuid),
             gen_random_uuid()),
    sqlc.arg(company_id)::uuid, sqlc.arg(definition_id)::uuid, sqlc.narg(username)::text,
    sqlc.narg(secret_enc)::bytea, sqlc.narg(extra_enc)::bytea, sqlc.arg(settings)::jsonb,
    sqlc.narg(pm5340_url)::text, sqlc.narg(isolar_region)::text, sqlc.arg(is_active)::boolean
where exists (
    select 1 from integration_definitions d where d.id = sqlc.arg(definition_id)::uuid
)
-- On conflict, a nil secret/extra parameter means "leave it as it was", not
-- "wipe it": UpsertCredential is also how a caller updates username/settings
-- alone, without re-supplying the secret, so `coalesce(excluded.x, x)` keeps
-- the stored ciphertext when the incoming value is NULL (fix round 1, folded
-- minor).
on conflict (company_id, definition_id) do update set
    username = excluded.username,
    secret_enc = coalesce(excluded.secret_enc, integration_credentials.secret_enc),
    extra_enc = coalesce(excluded.extra_enc, integration_credentials.extra_enc),
    settings = excluded.settings,
    pm5340_url = excluded.pm5340_url,
    isolar_region = excluded.isolar_region,
    is_active = excluded.is_active,
    updated_at = now()
returning *;

-- name: IntegrationDeleteCredential :execrows
delete from integration_credentials where id = $1 and company_id = $2;

-- name: IntegrationRecordVerification :execrows
update integration_credentials set
    last_verified_at = sqlc.arg(verified_at)::timestamptz,
    token_expires_at = sqlc.narg(token_expires_at)::timestamptz,
    updated_at = now()
where id = sqlc.arg(id)::uuid and company_id = sqlc.arg(company_id)::uuid;
