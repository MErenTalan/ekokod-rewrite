-- Integration definition administration (05 §15, F6a R176): platform rows,
-- written only by the admin role.

-- name: AdminIntegrationDefinitionCreate :one
insert into integration_definitions (id, provider, subtype, endpoints)
values (sqlc.arg(id), sqlc.arg(provider), sqlc.arg(subtype), sqlc.arg(endpoints))
returning *;

-- name: AdminIntegrationDefinitionUpdate :one
update integration_definitions
set subtype = sqlc.arg(subtype), endpoints = sqlc.arg(endpoints), updated_at = now()
where id = sqlc.arg(id)
returning *;

-- name: AdminIntegrationDefinitionDeleteUnused :execrows
-- Refuses a definition any credential still references (R176).
delete from integration_definitions d
where d.id = sqlc.arg(id)
  and not exists (select 1 from integration_credentials c where c.definition_id = d.id);

-- name: AdminIntegrationDefinitionExists :one
select exists(select 1 from integration_definitions where id = sqlc.arg(id));
