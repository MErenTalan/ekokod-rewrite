# F15b — Performance and Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans`. Inline, one session, no subagents.

**Goal:** 09 §F15's performance and observability items:
- every process exposes its metrics on an internal listener, off the public router;
- the job pipeline and queue have metrics;
- the dashboards and alert thresholds ship as files;
- hypertable queries are proven to exclude chunks;
- a repeatable load test runs against a scale dataset, with the results recorded.

**Architecture:**
- `internal/platform/metrics`: the internal `/metrics` listener, the asynq job middleware, and the queue-depth and integration-health collectors.
- `deploy/observability/`: the Grafana dashboard JSON and the Prometheus alert rules. A Go test proves every metric they reference is registered by the code.
- `ekokod tool seed-scale` builds the dataset (development/test only).
- `ekokod tool loadtest` is the load generator; `make load-test` and `make soak-test` wrap it.

**Spec:** 09 §F15; 03 §performance budget (dashboard interactive < 2 s); F1's `TestOneMillionReadingsChunkLayout`.

## Open questions (defaults shipped)

| # | Question | Default | Cost if wrong |
|---|---|---|---|
| Q-M1 | "Production volume" is not in the spec and there is no dump. | **Assumed: 100 analyzers with a year of hourly load profile (~0.9 M readings) and 20 buildings; 2× = 200 analyzers, 400 days (~1.9 M readings), 40 buildings, 10 concurrent users.** `seed-scale` takes the numbers as flags, so a rehearsal can re-run at the real figure from the inventory | The real site may be larger; rerun with its numbers |
| Q-M2 | The agreed p95 budget. | **500 ms p95 per read endpoint, < 1 % errors** under 2× load (the spec's only budget is "dashboard interactive under 2 s", and a dashboard makes several calls) | A tighter agreed budget needs a rerun, not new code |
| Q-M3 | Where `/metrics` is served. | A separate listener per process: `EKOKOD_METRICS_ADDR`, default `127.0.0.1:9464` for the API, `:9465` for the worker, `:9466` for the scheduler; empty disables it. Never on the public router | An operator scraping from another host sets the address |
| Q-M4 | The 24-hour soak. | `make soak-test` exists (`DURATION=24h`); **the run is PENDING** (this machine cannot hold a 24 h run) | Memory growth is unproven until it runs |

## Rulings (R460–R469)

| Id | Rule |
|---|---|
| R460 | **Metrics listener**: `/metrics` leaves the public API router. `metrics.Serve(ctx, addr, log)` serves only `/metrics` on the internal address. The API, the worker and the scheduler each start one (Q-M3). A test proves the public router 404s `/metrics`. |
| R461 | **Job metrics**: an asynq middleware records `ekokod_job_runs_total{type,outcome}` (`success`, `failure`, `skipped` = SkipRetry) and `ekokod_job_duration_seconds{type}` for every task. |
| R462 | **Queue depth**: a collector reads `asynq.Inspector.GetQueueInfo` on each scrape → `ekokod_queue_tasks{queue,state}` (pending, active, scheduled, retry, archived). Worker process. |
| R463 | **Integration health**: a collector reads the newest successful and newest failed ingestion `job_runs` per provider → `ekokod_integration_last_success_timestamp_seconds{provider}` and `ekokod_integration_last_failure_timestamp_seconds{provider}`. Worker process; query bounded by an index on `job_runs (job_type, status, finished_at)` if missing. |
| R464 | **Dashboards and alerts** in `deploy/observability/`: `grafana/ekokod-overview.json` (HTTP p95 by route, 5xx rate, job success rate and p95 duration by type, queue depth by state, integration last-success age) and `prometheus/alerts.yml`. Thresholds: API p95 > 1 s for 10 m; 5xx rate > 2 % for 5 m; job failure ratio > 10 % for 30 m; pending tasks > 500 for 15 m; retry tasks > 50 for 30 m; integration last success older than 26 h (the daily 03:00 pull); any process's metrics absent for 5 m. `TestObservabilityReferencesRegisteredMetrics` parses both files and checks every `ekokod_*` name. |
| R465 | **Chunk exclusion**: an integration test loads readings over ≥ 8 weekly chunks and EXPLAINs the repository's range queries (`meter_readings` by analyzer and time, `consumption_hourly` over a day), asserting that only the chunks overlapping the range are scanned. |
| R466 | **`ekokod tool seed-scale --analyzers N --buildings B --days D`**: refuses `EKOKOD_ENV=production`. Builds one company with B buildings and N analyzers (OSOS), a tariff per building, and hourly load-profile readings generated in SQL (`generate_series`), then refreshes the aggregates. Deterministic ids, idempotent. |
| R467 | **`ekokod tool loadtest --base URL --email --password --duration --users --p95-budget`**: each user logs in once, then loops a weighted read mix: dashboard summary, consumption hourly and daily, bills list, alarms, report archive, forecast read. It prints JSON per endpoint (count, errors, p50, p95, p99) and fails when any p95 exceeds the budget or errors exceed 1 %. |
| R468 | **`make load-test`** (`DURATION=2m USERS=10`) and **`make soak-test`** (`DURATION=24h`) run the tool against `$LOADTEST_BASE`. Job pipeline capacity is measured by timing `ekokod recompute bills` over the scale dataset. |
| R469 | **`docs/performance.md`**: the dataset, the measured numbers, the bottlenecks found and fixed, and the PENDING soak. |

## Tasks
1. R460 metrics listener (API, worker, scheduler) + test.
2. R461–R463 job, queue and integration collectors + tests.
3. R464 dashboards, alerts and guard test.
4. R465 chunk-exclusion test.
5. R466–R469 seed-scale, loadtest, make targets, a run here, performance doc, fixes.
6. Self-review, handoff.
