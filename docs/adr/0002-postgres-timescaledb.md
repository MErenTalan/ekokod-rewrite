# ADR 0002 — PostgreSQL with TimescaleDB for everything, including readings

Status: accepted

## Context

Readings arrive every 15–60 minutes per meter, for thousands of meters over ten years. Dashboards need hourly, daily, monthly and yearly figures. Billing needs exact period boundaries. The legacy system kept readings in MongoDB and aggregated them in application code (04, 08).

## Decision

One PostgreSQL 16 database with TimescaleDB 2.30. `meter_readings` is a hypertable (compressed after 90 days). Four consumption continuous aggregates (hourly/daily/monthly/yearly) and two plant-production aggregates exist. Everything else is ordinary relational tables with company-scoped foreign keys. Queries are written in SQL and typed with sqlc.

## Consequences

One backup and one restore procedure (`pg_dump` plus Timescale's pre/post-restore) cover all data. Range queries are chunk-excluded (tested in `chunk_exclusion_integration_test.go`). Continuous aggregates must be refreshed after a backfill (`ekokod recompute consumption`). Changing an aggregate's definition means dropping and rebuilding it (migration 00021).
