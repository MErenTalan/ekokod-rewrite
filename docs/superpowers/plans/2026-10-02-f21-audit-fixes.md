# F21 — Feature audit against real data (2026-10-02)

Branch `fix/f21-audit` (`/home/personal/ekokod-f21-audit`, cut from `main` @ 8a21a33).

**Why.** The product owner reported that pulling history failed for PM5340 and iSolar and that alarms
could not even be tried. Every main feature was exercised end to end on a local stack against the real
Artı Enerji PM5340 gateway (18 272 readings, 2025-12 → 2026-10) and through the web UI with Playwright,
with the legacy code (`bcem-energy`, `arti-scada`) as the reference for behaviour.

## Rulings

| # | Ruling | Where |
|---|---|---|
| R498 | Consumption aggregates ignore NULL registers: `first()/last()` take `filter (where x is not null)`. An edge row with a null register no longer nulls the whole bucket. | migration 00024 |
| R499 | Months and years are rolled up from `consumption_daily` at query time; `consumption_monthly/yearly` are dropped. A bucket closed by the next bucket's first index is not partial (`ClosedByBoundary`). | 00024, `analytics.sql`, `boundary.go` |
| R500 | Every fetch refreshes the aggregates over its own range. No policy-horizon clip, no 29-day threshold. | `consumption/refresh.go`, `ingest/fetch.go` |
| R501 | A day is billable only with an active-import index (or a suspect flag); otherwise it is a missing day and the bill is partial. | `billing/generate.go` |
| R502 | `last_reading_at` is set by any persisted reading, a backfill's included. | `ingest/fetch.go` |
| R503 | A job run's `finished_at` is the clock at finish, not the run's start. | `ingest`, `backfill` |
| R504 | The PM5340 integration definition is seeded (it needs no endpoints). | `seed/data/integration_definitions.json` |
| R505 | Alarm ratios are rounded to 2 dp in events and messages. | `domain/alarm/evaluate.go` |
| R506 | `power_plants.isolar_history_through`: a plant's daily history is fetched on any sync until it is marked through yesterday; unlinking clears it. | migration 00025, `solar/sync.go` |
| R507 | A first iSolar sample in the midnight hour is a baseline, not production. | `solar/intervals.go` |
| R508 | `BulkInsert` and `TouchLastReading` retry on deadlock (40P01); the touch only writes when it moves the value. | `store/postgres/deadlock.go` |
| R509 | The sanity rule's baseline is the largest delta of the past week; readings held by a streak are released when the streak proves sustained. | `ingest/validate.go` |
| R510 | Operator-facing messages are Turkish sentences naming the meter, never wire codes or task names. | `ingest/messages.go`, `consumption/anomalies.go` |
| R511 | The reactive status nets export the way the tariff does. | `analysis/reactive_status.go` |
| R512 | A building with only a building bill still gets its section on the bills dashboard and counts in the netting. | `domain/billing/dashboard.go` |
| R513 | No "generation is null" warning when the gateway reports no generation at all. | `integration/pm5340/source.go` |
| R514 | Every Messages row of an alarm firing starts with what fired; a rule without a channel is a warning, not an error; the run summary counts firings. | `service/alarms/{notify,run}.go` |

## Web fixes (no ruling ids)

- **Alarm dialog**: a reactive limit counted only after its unit select had been *changed*, so a rule with
  "15 % / 24 / saat" could not be saved. The unit now defaults on send; a half-entered limit says its period
  is missing; an untouched form shows no errors.
- **Alarm log dialog**: shows the rule's limits, why each event fired (`detail.lines`), and "no channel".
- **`useJob`**: queries were invalidated when a job was *queued*; they are now refetched when it *ends*
  (and once more 8 s later for the aggregate refresh). A generated bill / pulled history reaches the screen.
- **Empty refresh**: "0 of 0 finished" → "nothing to fetch: the provider does not offer this data".
- **Dashboard**: the only building is selected; the consumption panel says what to pick; ratios to 2 dp.
- **Messages**: category and related-record labels (camelCase keys, unknown codes shown as they are).
- **Settings › integrations**: verifying refreshes the list ("Son doğrulama" was stuck on "Hiç"); a gateway
  row shows its address.
- **Line chart**: a value between two gaps gets a dot (January vanished from the trend).
- **Invoice PDF / report**: register names, ratios as percentages, `kg CO2e/<unit>`.

## Verified on real data

44/44 backfill windows succeed, 0 rejected readings, positive run durations, `last_reading_at` set.
Monthly consumption equals the raw register differences (Dec 864.53, Jan 2 060.68, Apr 1 523.58,
May 1 847.26, Jun 1 448.92 partial, Sep 305.04 kWh). The May bill is exact to the kuruş (energy,
binomial power charge, demand overrun, capacitive penalty, VAT). All four alarm types were created,
dry-run and fired through the UI. Reports render as PDF and XLSX. 20 pages load without a console or
page error.

## Known, not changed

- A year's figure is the register difference across the year, gaps included, so it can exceed the sum
  of its months when the meter was silent for a while.
- The power alarm reads `max_demand_kw`. PM5340 reports its *peak* register there (constant until the
  meter resets), exactly as legacy did — a power-max rule on such a meter fires on every run.
- `GET /bills/latest?scope=company` answers 404 when no company bill exists; the dashboard card falls
  back, the browser console shows the 404.
- iSolar changes (R506, R507) are covered by tests and the arti-scada reference only: no credentials
  were available to run them live. A linked plant fetches up to 400 days on its first sync after deploy.

## Operational notes

- Migration 00024 waits for running aggregate refresh jobs before dropping the views (dropping a
  continuous aggregate under its own refresh crashed TimescaleDB 2.30 with signal 11) and schedules the
  new policies 10 minutes out.
- The server runs `ee91a8c`; `1f4cb0d` and this branch are not deployed.
