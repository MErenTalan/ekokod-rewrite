# ADR 0004 — Background jobs on Redis (asynq); one leader-elected scheduler

Status: accepted

## Context

Ingestion, invoices, reports, alarms, forecasts and carbon accruals run periodically or on demand. They must retry, must not run twice, and must be visible to users (the Messages tab).

## Decision

Jobs are asynq tasks on Redis with deterministic task ids for idempotency. Every run is recorded in `job_runs`. The scheduler is a cron process that takes a Postgres advisory lock (leader election), so running two is safe. The worker exits when it loses Redis, and the supervisor restarts it.

## Consequences

Redis holds only transient queue state and is not backed up. A lost queued task is re-created by the next scheduler tick. Metrics expose queue depth, retries and job outcomes. Alerts fire on backlog and failure rate.
