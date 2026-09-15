-- AdminIngestionRepository's one query. This is the platform's unscoped
-- surface (internal/store/postgres/admin): the scheduled dispatcher has no
-- tenant to offer, and must see every company's active credentials in one
-- pass — see admin/doc.go's sixth paragraph.

-- name: AdminIngestionActiveCredentials :many
-- Every is_active credential of a non-deleted company, joined to its
-- integration_definitions row for (provider, subtype). No secret column
-- (secret_enc, extra_enc) is selected — sqlcgen.AdminIngestionActiveCredentialsRow
-- carries only the columns named below, which is what keeps a future column
-- added to either table from silently starting to flow through this path.
select
    ic.id            as credential_id,
    ic.company_id    as company_id,
    ic.definition_id as definition_id,
    idef.provider    as provider,
    idef.subtype     as subtype
from integration_credentials ic
join integration_definitions idef on idef.id = ic.definition_id
join companies c on c.id = ic.company_id
where ic.is_active
  and c.deleted_at is null
order by ic.company_id, ic.id;
