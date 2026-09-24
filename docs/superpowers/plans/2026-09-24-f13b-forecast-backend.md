# F13b — Forecasting Backend (Go) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans`. Inline, one session, no subagents.

**Goal:** The Go half of 09 §F13:
- an ML client with timeouts, retries and graceful degradation;
- the forecast service, which builds the §6.3 request from the database;
- the `forecast.run` job, which persists to `forecasts` and `forecast_gaps`;
- 05 §16's endpoints: `GET /forecast`, `POST /forecast/run`, `/forecast/weekly`, `/forecast/monthly` and `/anomaly/check`.

**Architecture:**
- `internal/ml`: an HTTP client; every failure to reach the service or get an answer is `ml.ErrUnavailable`.
- `internal/service/forecast`:
  - history comes from `AnalyticsRepository.ConsumptionHourly`/`Daily` (active consumption);
  - covariates come from `CalendarRepository` (weekend days → `day_type`, vacations → `vacation`);
  - results persist through `ForecastRepository`.
- `internal/job/forecast.go`: the `forecast.run` job. The scheduler runs it on `Schedule.Forecast` (config already has it, 04:00). Ops can trigger it.
- API: `handlers_forecast.go` and `dto/forecast.go`, with permissions `forecast.read` (every role, scoped) and `forecast.run` (A, CA, BA).

**Spec:** 09 §F13; 05 §16; 03 §6.3; 01 §7.5–7.6; F13a (the service contract as built).

## Open questions (defaults shipped)

| # | Question | Default | Cost if wrong |
|---|---|---|---|
| Q-I8 | Weather covariate. | **Not sent.** No hourly weather history is stored (the Open-Meteo client is forecast-only), so a temperature feature would exist for the horizon but not for the history. Archive ingestion is later work | Less accurate forecasts in weather-driven buildings |
| Q-I9 | Holidays in `day_type`. | **workday/weekend from the company's weekend days only.** No public-holiday dataset exists in the rewrite; calendar events are free-form | Holidays forecast as workdays |
| Q-I10 | History window. | Hourly: the **last 8 weeks** up to the last complete hour. Daily (monthly endpoint): the **last 180 days** | — |
| Q-I11 | Which calls persist. | `POST /forecast/run` and the job persist a run (`generated_at` = now). `weekly`/`monthly` are **on-demand and not stored** | — |
| Q-I12 | Daily totals for the AI screen. | The UI sums the hourly median for the day; the band is the sum of p10 and of p90, labelled an approximate range | The summed band is wider than a true daily quantile |
| Q-I13 | Anomaly `actual`. | Optional in the request. When absent, the stored hourly consumption at `ts` is used; if that is missing too → 422 `actual: required` | — |
| Q-I14 | Job horizon. | 168 h (one week) per active analyzer with any consumption in the window; failures are isolated per analyzer and counted in the run | — |

## Rulings (R370–R379)

| Id | Rule |
|---|---|
| R370 | **Client:** `Authorization: Bearer <EKOKOD_ML_API_KEY>`, per-call timeout = `EKOKOD_ML_TIMEOUT`, **2 retries** with 250 ms/750 ms backoff on network errors and 502/503/504 only. Any other non-2xx, or a decode error → `ErrUnavailable` wrapping the cause; 422 → `ErrRejected`. |
| R371 | **Degradation:** `ErrUnavailable` → API 503 `forecast_unavailable` (messages tr/en). `GET /forecast` never calls the ML service, so stored forecasts stay readable. |
| R372 | **Run:** `POST /forecast/run {analyzer_id, horizon_hours 1–744}` → `{status, model_id, model_version, generated_at?, points[{ts, median, p10, p90}], gaps[], used_covariates[], fallback_from?}`. Only `ok` persists rows; gaps persist on every status that carries them. |
| R373 | **Read:** `GET /forecast?analyzer_id&from&to` (≤ 62 days) → the latest run covering the window (`LatestRun`), its gaps, its `generated_at`, model id and version; `status: none` when no run exists. |
| R374 | **Weekly:** `{analyzer_id, week_start}`. The start must be a Monday within 30 days ahead; the horizon covers to week_start + 7 d, and the week's 168 points are returned. **Monthly:** `{analyzer_id, month YYYY-MM}` on daily granularity; the month must end within 62 days of the last complete day, and the month's days are returned. |
| R375 | **Anomaly:** `{analyzer_id, ts, actual?}` → the ML answer plus the `actual` used. History = the 12 weeks before `ts`. |
| R376 | **Job:** `forecast.run` iterates companies (SystemScope) and their active analyzers, isolating failures. One `job_runs` row per company, with processed/skipped/failed counts. ML unavailable → the run fails with the reason and the task retries per the job policy. |
| R377 | **Scope:** every endpoint resolves the analyzer through the caller's Scope first (404 outside it). The `forecast` and `anomaly` families join the tenancy sweep. |
| R378 | **Timestamps:** requests to ML are UTC RFC 3339. Day types and vacations are computed on Europe/Istanbul dates. |
| R379 | **Values:** history values are the hourly active consumption (kWh). A nil bucket is a null point, so it becomes a gap in ML. |

## Tasks
1. `internal/ml` client (R370) + tests including `TestGracefulDegradation`.
2. Forecast service: request building (Q-I8…Q-I10, R378, R379), run/read/weekly/monthly/anomaly (R372–R375) + unit tests with fakes.
3. Job + scheduler + ops + worker wiring (R376) + tests.
4. API: DTOs, handlers, permissions, messages, OpenAPI, authz matrix, sweep (R371, R377) + integration test (PENDING Docker).
5. Self-review, handoff.
