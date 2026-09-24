# F13a — ML Service (Python) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans`. Inline, one session, no subagents.

**Goal:** The FastAPI service of 03 §6. It is stateless apart from model artifacts, has **no database driver**, and implements the §6.3 contract exactly. Every response adds `model_id` and `model_version`.

**Architecture:** `ml/` is a uv project (`ekokod_ml` package).
- **Contract:** `schemas.py` (pydantic).
- **Pure logic:** `gaps.py`, `features.py`, `anomaly.py`.
- **Models:** `models/` holds one `Forecaster` protocol with these implementations:
  - `seasonal_naive`: the statistical baseline, always available;
  - `random_forest`: scikit-learn over engineered features; quantiles from the per-tree spread;
  - `lstm`: needs the optional `torch` extra;
  - `chronos`: an HTTP adapter to the external foundation-model service.
- **Routing:** `registry.py` chooses a model and falls back to the baseline.
- **App:** `api.py` is the FastAPI app with shared-secret auth.
- **Image:** `ml/Dockerfile` (python:3.12-slim, `uv sync --frozen --no-dev`, non-root).

**Spec:** 03 §6.1–6.4; 09 §F13; 01 §7.5–7.6. Legacy called an external `chronos-service:8000/predict`; the legacy Python source is not in the repo.

## Open questions (defaults shipped)

| # | Question | Default | Cost if wrong |
|---|---|---|---|
| Q-I1 | LSTM needs torch (≈1 GB). | **Optional extra `lstm`.** Without torch the model is listed with `available: false`, and a request for it falls back to the baseline (the response says so). The image ships without it | LSTM unused until an operator builds with the extra |
| Q-I2 | Chronos (the foundation model) hosting. | **HTTP adapter to `ML_CHRONOS_URL`** (the legacy sidecar contract `{history, horizon}` → quantiles). Unset → unavailable | — |
| Q-I3 | Auth header. | `Authorization: Bearer <EKOKOD_ML_API_KEY>`, compared in constant time; `/health` is open | — |
| Q-I4 | Minimum history. | `ML_MIN_HISTORY_HOURS` = 336 (two weeks) for hourly, 28 points for daily; below it → `insufficient_data` with no values | Fewer forecasts for new meters |
| Q-I5 | Stateless vs trained models. | Each forecast fits on the history it is given (stateless). `POST /v1/train` fits a **pooled** random forest on a supplied multi-series dataset and saves `rf-<version>.joblib`; `random_forest` uses the newest pooled artifact when present | — |
| Q-I6 | Model failure. | The requested model raises → **baseline fallback** with `fallback_from` in the response; the baseline raises → `status: model_error` | — |
| Q-I7 | Anomaly method. | `robust_zscore_same_hour_of_week`: expected = the median of the same hour-of-week values; the band is ±3.5 × 1.4826 × MAD; `score` = \|actual − expected\| / (1.4826 × MAD) | Other detectors later, named in `method` |

## Rulings (R360–R369)

| Id | Rule |
|---|---|
| R360 | **Forecast statuses:** `ok`, `insufficient_data`, `no_data` (empty or all-null history), `model_error`. Only `ok` carries values. Every response carries `model_id`, `model_version`, `gaps`, `used_covariates`. |
| R361 | **Gaps:** expected steps are absent or `null` between the first and last history point. Each run of missing steps → `{start, end, missing_hours}`, where start and end are the first and last missing timestamps. Daily steps count 24 h. |
| R362 | **Quantile order:** p10 ≤ median ≤ p90 at every step; values ≥ 0 (consumption); `len` = horizon. Timestamps continue the history's step. |
| R363 | **Features:** hour, day of week, month, `is_weekend`, day_type (covariate: `workday`/`weekend`/`holiday`/`half_day`), vacation flag, temperature (weather covariate), lag-24/lag-168, rolling mean 24/168. `used_covariates` names the covariates actually used. |
| R364 | **Models endpoint:** `GET /v1/models` → `[{id, version, available, trained_at, default}]`. |
| R365 | **Train:** `POST /v1/train {model: "random_forest", series: [{series_id, history, covariates}]}` → `{model_id, model_version, trained_at, samples}`. Refuses fewer than 1 usable sample with 422. |
| R366 | **No DB client:** a test asserts that no installed distribution is a database driver (psycopg*, pymongo, sqlalchemy, asyncpg, pymysql, mysqlclient, redis, motor, cassandra-driver, clickhouse*). The image check is the Docker command in 09 §F13 (PENDING while Docker hangs). |
| R367 | **Limits:** history ≤ 17 544 points (two years hourly), horizon ≤ 744 (31 days hourly) or 62 daily → 422. |
| R368 | **Anomaly:** fewer than 8 same-hour-of-week points → `is_anomaly: false`, `method: insufficient_history`, score null. |
| R369 | **Config:** `EKOKOD_ML_API_KEY` (required, ≥ 16 chars), `ML_MODEL_DIR` (default `/var/lib/ekokod-ml`), `ML_DISABLED_MODELS` (comma list), `ML_DEFAULT_MODEL` (default `random_forest`). |

## Tasks
1. Project scaffold, schemas, gaps (R361), statuses (R360) + tests.
2. Features (R363), baseline and random forest behind the protocol, registry with fallback (Q-I6, R362) + tests, including "baseline works with every heavier model disabled".
3. LSTM (optional) and Chronos adapters, models endpoint (R364), train (R365) + tests.
4. Anomaly (Q-I7, R368), FastAPI app with auth (Q-I3) and limits (R367), OpenAPI + tests.
5. Dockerfile, no-DB-client test (R366), compose service, handoff.
