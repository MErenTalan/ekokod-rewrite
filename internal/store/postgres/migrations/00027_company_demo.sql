-- +goose Up
-- R540: the only demo marker. A demo company is writable by its shared demo
-- login, never reaches outside the platform and is reset every hour.
alter table companies add column is_demo boolean not null default false;

-- +goose Down
alter table companies drop column if exists is_demo;
