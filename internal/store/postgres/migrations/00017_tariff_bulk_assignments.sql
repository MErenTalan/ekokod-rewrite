-- +goose Up
-- R241: 05 §6 asks for a bulk-assignment history and nothing in the schema
-- records one. A tariff version only knows its own building, so it cannot
-- answer "which buildings were assigned together, from which template" — the
-- question the screen asks. The row is written AFTER the assignment and lists
-- the buildings that actually received a version, never the ones requested.
create table tariff_bulk_assignments (
    id             uuid primary key default gen_random_uuid(),
    company_id     uuid not null references companies(id),
    template_id    uuid references tariff_templates(id) on delete set null,
    tariff_name    text,
    effective_from date not null,
    building_ids   uuid[] not null,
    created_by     uuid references users(id),
    created_at     timestamptz not null default now()
);
create index on tariff_bulk_assignments (company_id, created_at desc);

-- +goose Down
drop table if exists tariff_bulk_assignments;
