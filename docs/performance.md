# Performance (09 §F15)

Measured on the development machine (WSL2, 8 CPUs, TimescaleDB 2.30 / PG16 in Docker on tmpfs, the API
and the load generator on the same host). Re-run on the production host before sign-off; the commands
are below.

## Dataset (F15b Q-M1)

The spec states no production volume and no dump exists yet. Assumed production: 100 analyzers with a
year of hourly load profile, 20 buildings. **2×:**

```bash
EKOKOD_SCALE_PASSWORD=… ekokod tool seed-scale --analyzers 200 --buildings 40 --days 400
```

→ 1 920 200 readings; built in **4 m 20 s** including the refresh of the four consumption aggregates.
Once the inventory gives the real figures, rerun with them.

## API under 2× load (Q-M2: budget p95 ≤ 500 ms per endpoint, < 1 % errors)

`ekokod tool loadtest --duration 2m --users 10` (`make load-test`), read mix weighted towards the
consumption screens:

| Endpoint | Requests | Errors | p50 ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|
| consumption hourly (building, day) | 31 189 | 0 | 16.1 | 22.7 | 26.6 |
| consumption daily (building, month) | 23 401 | 0 | 14.5 | 20.7 | 24.7 |
| consumption monthly (analyzer, year) | 15 496 | 0 | 6.8 | 11.7 | 14.5 |
| buildings | 7 653 | 0 | 4.9 | 9.4 | 12.0 |
| analyzers | 7 789 | 0 | 5.6 | 10.2 | 12.6 |
| bills | 7 695 | 0 | 4.5 | 8.8 | 11.1 |
| alarms | 7 996 | 0 | 4.7 | 9.1 | 11.7 |
| reports | 7 868 | 0 | 5.2 | 9.7 | 12.1 |

**109 087 requests in 2 minutes (909 req/s), no errors, every p95 under 25 ms** — well inside the budget
and the spec's "dashboard interactive under 2 s".

## Job pipeline

`ekokod recompute bills` over the 2× dataset for two months (40 building + 200 analyzer + 1 company bill
per month = 482 bills), `--force` so every bill is computed:

| Setting | Time | Throughput |
|---|---|---|
| before F15b (companies in parallel; the dataset is one company → sequential) | 257 s | 1.9 bills/s |
| **F15b: buildings in parallel inside a company, `--parallel 4`** | **138 s** | **3.5 bills/s** |

A month of bills for the 2× estate takes about 70 s; five years of history about 70 minutes. The
monthly scheduled run (`billing.dispatch`) is bounded by the worker's concurrency instead.

## Hypertable plans

`TestRangeQueriesExcludeChunks` (integration): over ten weekly `meter_readings` chunks, the repository's
one-day `ReadingRange` plans against **one** chunk and `ConsumptionHourly` against the day's; making the
time predicate non-sargable made it scan all eleven, so the test guards the property.

## Findings

- **Fixed:** `recompute bills` did not parallelise within a company (above).
- **Open — F3 Q6, confirmed at scale:** the consumption API's *hourly* series is `last − first` inside each
  hour bucket, so it is **zero for a meter that reports once an hour** and about 25 % low for a 15-minute
  meter; daily and monthly figures lose one reading interval at each bucket end (about 4 % of a day for
  an hourly meter). F3's R87 already computes the load profile from consecutive bucket boundaries; the
  analytics series needs the same (`index(h) − index(h−1)` across consecutive buckets). Billing is not
  affected — it reads boundary readings (F3 R61). **This needs the product owner's attention before
  cutover:** legacy OSOS load profiles are hourly.

  **Design notes for the fix (not started in F15b):** 02 §3.1's boundary differencing is
  `consumption[b0, b1) = boundary(b1) − boundary(b0)`, with `boundary(t)` = the last reading with
  `ts ≤ t`. Per bucket that is the next bucket's first reading when it sits exactly on the edge, else this
  bucket's last. The aggregates keep a first value only for `active_import` (`active_import_start`); every
  other register has only `last()` (`*_index`). So a correct fix for all twelve registers needs a
  migration that adds `first()` per register to the four consumption aggregates (a rebuild: cheap before
  cutover), then the analytics service differences consecutive buckets with a one-bucket look-ahead, as
  R87 does for the load profile. A look-behind over `last()` alone would shift an hourly meter's
  consumption by one hour. Tests to update: F3's "analytics and billing differ by exactly the missing
  step" acceptance test encodes the current behaviour and must be re-ruled.

## PENDING

- **24-hour soak** (`make soak-test`, `DURATION=24h`): no memory-growth or backlog proof until it runs on a
  host that can hold it.
- The same numbers on the production host and with the real volume from the inventory.
