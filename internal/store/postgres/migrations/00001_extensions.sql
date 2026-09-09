-- +goose Up
-- TimescaleDB provides the hypertables and continuous aggregates that every
-- time-series table in this system depends on (see docs/rewrite/04-data-model.md).
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose Down
DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS pgcrypto;
DROP EXTENSION IF EXISTS timescaledb;
