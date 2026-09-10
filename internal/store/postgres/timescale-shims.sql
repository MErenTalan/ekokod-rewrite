-- sqlc-only declarations. NEVER applied to a real database: TimescaleDB
-- provides these. sqlc's catalogue does not know them, and without these
-- stubs it guesses -- e.g. `last(x,ts) - first(x,ts)` over numeric came out
-- as int32, silently truncating energy values.
--
-- It lives here rather than in migrations/ precisely so that goose can never
-- apply it: migrate.go embeds only `migrations/*.sql`. sqlc reads it because
-- sqlc.yaml lists it as the first entry of `schema:`, ahead of the migrations
-- directory, so the function signatures are in the catalogue before 00005's
-- continuous aggregates are parsed.
--
-- Extend this file by ADDING declarations when a new aggregate uses a
-- Timescale function not listed here (say, first()/last() over a non-numeric
-- column). Never weaken a return type to make a query compile.
-- sqlcgen_numeric_test.go fails if a numeric column stops generating as
-- pgtype.Numeric or an aggregate bucket stops generating as pgtype.Timestamptz,
-- which is what catches the removal of any declaration below.
create function time_bucket(bucket_width interval, ts timestamptz) returns timestamptz as $$ select ts $$ language sql;
create function first(value numeric, ordering timestamptz) returns numeric as $$ select value $$ language sql;
create function last(value numeric, ordering timestamptz) returns numeric as $$ select value $$ language sql;
