-- +goose Up
-- A bill may be generated before its period has full data; the Istanbul
-- calendar days it could not cover are listed on the bill (YYYY-MM-DD).
alter table bills add column missing_days text[] not null default '{}';

-- +goose Down
alter table bills drop column if exists missing_days;
