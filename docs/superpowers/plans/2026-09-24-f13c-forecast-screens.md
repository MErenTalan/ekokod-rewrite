# F13c — Predict and AI Analysis Screens Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans`. Inline, one session, no subagents.

**Goal:** 01 §7.5 **Predict** at `/ekorm/forecast` (the nav entry already exists) and §7.6 **AI Analysis** at `/ekorm/ai`, on F13b's API.

**Architecture:** `web/src/features/forecast/`:
- **Predict:**
  - `predict-page` (container): ScopePicker + history range + horizon; stored run via `GET /forecast`; "Tahmin et" via `POST /forecast/run`;
  - `forecast-view` (view): status banner, `LineChart` (actuals + median `kind: 'forecast'` + p10–p90 band), model line, gaps table.
- **AI Analysis:** `ai-page` (container) with four tabs: daily, weekly, monthly, anomaly.
  - `prediction-table` (view) shows every forecast result as a table;
  - `anomaly-view` (view) shows the anomaly verdict.
- **Shared:** `forecast-status.ts` maps statuses to messages; the `forecast` messages namespace (tr/en).

**Spec:** 01 §7.5, §7.6; 09 §F13 acceptance ("median with the p10–p90 band, drawn neutral and dashed, and surfaces reported gaps"; "the UI shows forecasting as unavailable and no other feature breaks"); F13b R371–R375, Q-I11, Q-I12.

## Open questions (defaults shipped)

| # | Question | Default | Cost if wrong |
|---|---|---|---|
| Q-I15 | Route for Predict. | **`/ekorm/forecast`**: the nav entry and label already exist (§7.5 names `/ekorm/predict`, and that path redirects there) | — |
| Q-I16 | Who sees AI Analysis. | Nav permission **`forecast.run`** (A CA BA): every action on it runs the model. Read-only roles use Predict | Read-only users cannot open it |
| Q-I17 | The daily prediction. | `POST /forecast/run` with the horizon reaching the end of the target day (≤ 30 days ahead); the day's hours are shown; the total is the sum of medians, with an approximate band (Q-I12) | The run is stored, like any run |
| Q-I18 | The anomaly chart. | A bar chart of actual against expected, with the band and score in the table | — |

## Rulings (R380–R385)

| Id | Rule |
|---|---|
| R380 | **Status messages:** every status has its own message: `ok` with points, `ok` with no points ("empty result"), `none` (no stored run yet), `insufficient_data`, `no_data`, `model_error`, and **unavailable** (503 `forecast_unavailable`). |
| R381 | **Chart:** actuals are the consumption kind; the forecast median is `kind: 'forecast'` (neutral, dashed); the band is p10–p90 (`band`). One x axis is hourly timestamps across history and forecast. |
| R382 | **Gaps:** when a result carries gaps, a warning names their effect and a table lists start, end and missing hours. |
| R383 | **Model line:** model id + version, generated time, `fallback_from` ("… kullanılamadı, temel model kullanıldı"), used covariates. |
| R384 | **AI tabs:** every result is shown both as a table and as a chart. Weekly and monthly results are not stored (Q-I11). A 422 field error lands on its field. |
| R385 | **Degradation:** an unavailable ML service never breaks the page. Predict still shows actuals and any stored run; AI shows an unavailable alert. |

## Tasks
1. The `forecast` messages, `forecast-status.ts`, `forecast-view` (+ story, test) and `predict-page` (+ test), the `/ekorm/forecast` route, and the `/ekorm/predict` redirect.
2. The AI page: the daily/weekly/monthly tabs with `prediction-table`, the anomaly tab with `anomaly-view`, the nav entry, and the route (+ stories, tests).
3. The e2e spec `tests/e2e/predict.spec.ts` (ML unavailable in e2e → degradation), the a11y sweep, self-review, and the handoff.
