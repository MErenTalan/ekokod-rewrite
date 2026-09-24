-- +goose Up
-- F14b (R413, Q-J2): legacy traceability and the comparison tables 08 §1 asks
-- for. Legacy invoices and reports are never loaded into bills/reports: they
-- are recomputed, and reconcile (F14c) compares against these rows.
create table legacy_ids (
    collection text not null,
    legacy_id  text not null,
    table_name text not null,
    new_id     uuid not null,
    primary key (collection, legacy_id, table_name)
);
create index on legacy_ids (new_id);

create table legacy_bills (
    id                       uuid primary key,
    company_id               uuid not null references companies(id),
    building_id              uuid not null references buildings(id),
    analyzer_id              uuid references analyzers(id),
    scope                    text not null check (scope in ('building','analyzer')),
    period                   char(7) not null,   -- YYYY-MM, the legacy billHistory key
    start_date               date,
    end_date                 date,
    total_active_kwh         numeric(18,4),
    t1_kwh                   numeric(18,4),
    t2_kwh                   numeric(18,4),
    t3_kwh                   numeric(18,4),
    inductive_kvarh          numeric(18,4),
    capacitive_kvarh         numeric(18,4),
    energy_cost              numeric(18,4),
    distribution_cost        numeric(18,4),
    capacity_cost            numeric(18,4),
    power_cost               numeric(18,4),
    green_energy_cost        numeric(18,4),
    reactive_penalty         numeric(18,4),
    reactive_penalty_applied boolean,
    vat_cost                 numeric(18,4),
    other_taxes_cost         numeric(18,4),
    total_cost               numeric(18,4),
    pdf_path                 text,
    payload                  jsonb not null,
    constraint analyzer_scope check ((scope = 'analyzer') = (analyzer_id is not null))
);
create unique index on legacy_bills (scope, building_id, analyzer_id, period) nulls not distinct;
create index on legacy_bills (company_id, period);

create table legacy_reports (
    id          uuid primary key,
    company_id  uuid not null references companies(id),
    building_id uuid references buildings(id),
    report_type text not null,
    period      text not null,
    payload     jsonb not null,
    pdf_path    text,
    excel_path  text,
    created_at  timestamptz
);
create index on legacy_reports (company_id, period);

-- +goose Down
drop table if exists legacy_reports;
drop table if exists legacy_bills;
drop table if exists legacy_ids;
