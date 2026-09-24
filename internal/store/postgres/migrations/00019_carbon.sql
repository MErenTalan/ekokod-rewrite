-- +goose Up
-- F10a (R318): a company factor reset (R304) must not delete or block the
-- activities computed from it; they keep their factor key, value and multiplier.
alter table carbon_activities drop constraint carbon_activities_factor_id_fkey;
alter table carbon_activities add constraint carbon_activities_factor_id_fkey
    foreign key (factor_id) references emission_factors(id) on delete set null;

-- +goose Down
alter table carbon_activities drop constraint carbon_activities_factor_id_fkey;
alter table carbon_activities add constraint carbon_activities_factor_id_fkey
    foreign key (factor_id) references emission_factors(id);
