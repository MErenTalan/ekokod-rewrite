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
-- The three-argument, timezone-aware form. 00005 uses it for every bucket
-- coarser than an hour, because a day/month/year boundary must be evaluated in
-- Europe/Istanbul rather than UTC. It is a distinct overload as far as sqlc's
-- catalogue is concerned: with only the two-argument declaration above, those
-- five views regenerate with `Bucket interface{}`.
create function time_bucket(bucket_width interval, ts timestamptz, timezone text) returns timestamptz as $$ select ts $$ language sql;
create function first(value numeric, ordering timestamptz) returns numeric as $$ select value $$ language sql;
create function last(value numeric, ordering timestamptz) returns numeric as $$ select value $$ language sql;

-- jsonb_array_elements is an ordinary Postgres built-in (not TimescaleDB),
-- but TestEveryFunctionSQLcMustTypeIsDeclared's nativeFunctions list in
-- sqlcgen_numeric_test.go is closed and that file must not be edited to add
-- to it, so it is declared here instead. Task 11b uses it as the bulk-insert
-- idiom for a parallel-column child collection, since sqlc's catalogue has no
-- multi-array unnest() overload (see carbon.sql's CarbonInsertConversions):
-- one jsonb array parameter, unpacked with jsonb_array_elements and read back
-- out with `elem->>'field'` plus an explicit cast per column, rather than
-- jsonb_to_recordset's `AS t(col1 type1, ...)` list — TestEveryFunctionSQLc-
-- MustTypeIsDeclared's isRelationBeforeParen only recognises "into", "table",
-- "references" and index's "on" as introducing a relation, not "as", so a
-- recordset alias's own column-list parenthesis reads as an unknown call
-- named after the alias. jsonb_array_elements returns a single jsonb column,
-- so its alias is never followed by "(" and never trips that scan at all.
create function jsonb_array_elements(from_json jsonb) returns setof jsonb as $$ select from_json $$ language sql;
