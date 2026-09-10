-- +goose Up
-- TimescaleDB provides the hypertables and continuous aggregates that every
-- time-series table in this system depends on (see docs/rewrite/04-data-model.md).
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose Down
-- Deliberately a no-op. The inverse of a conditional create is not an
-- unconditional drop: the Up above is explicitly a no-op when an extension is
-- already present, so it did not necessarily install any of these, and
-- DROP EXTENSION IF EXISTS would destroy extensions a DBA pre-provisioned or
-- that another schema in a shared database depends on — cascading through
-- every hypertable built on timescaledb. Postgres records no per-migration
-- provenance for an extension, and inferring it (pg_depend, an extra
-- bookkeeping table) would be more state to keep correct than the drop is
-- worth. Removing these is an operator decision, taken deliberately, not a
-- side effect of `ekokod migrate down`.
SELECT 1;
