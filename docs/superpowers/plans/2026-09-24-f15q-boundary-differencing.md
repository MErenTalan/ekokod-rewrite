# F15q — analytics consumption by boundary differencing (F3 Q6)

**Goal:** the consumption aggregates' per-bucket consumption is `boundary(b1) − boundary(b0)` (02 §3.1),
so an hourly meter's hourly series is its real consumption, not zero, and every level telescopes.
Billing is untouched (it reads boundary readings, F3 R61).

## Rulings

| id | ruling | cost if wrong |
|----|--------|---------------|
| R470 | Migration 00021 drops and recreates the four `consumption_*` aggregates with `first_ts` (`min(ts)`), a `first()` for every one of the twelve registers (`*_start`) and a `last()` for the five export registers that had none (`*_generation_index`). Existing columns keep their names and meaning. Down restores 00005's definitions. The rebuild refreshes from `meter_readings` at migration time (cheap before cutover). | a production DB that already holds years of readings pays one full rebuild on upgrade — acceptable, F14's runbook already refreshes every aggregate after the load |
| R471 | `AnalyticsRepository.Consumption{Hourly,Daily,Monthly,Yearly}` query one bucket before and after the requested range and rewrite every `*Consumption`/`*Generation` field with the boundary difference: `start = own first` when `first_ts == bucket start`, else the adjacent previous bucket's `last`, else own first; `end = next bucket's first` when the next bucket is adjacent and its `first_ts` is exactly its start, else own `last`. Nil when either side is nil. Buckets outside the request are dropped afterwards. Every caller (analytics series, carbon accrual, forecast, renewable, load profile) gets the corrected figure without change. | a meter with a gap: the bucket after a gap falls back to own first (understates, as before) instead of absorbing the gap — conservative, never inflates |
| R472 | F3's acceptance criterion "billing and analytics differ at a bucket boundary by the expected step" is re-ruled: when readings sit on the bucket edges the two paths **agree** (difference 0). The test is renamed `TestF3BillingAndAnalyticsAgreeOnBoundaryAlignedReadings`; 09 §F3 and 04 §4.3 are amended. | none — the old behaviour was the defect |
| R473 | R94's composed monthly/yearly rows (from daily closing indexes) are unchanged: closing-to-closing over the same boundaries telescopes, shifted by one interval, and the row is already `Partial`. | a composed month differs from the later materialised one by (first interval − last interval) — visible only until the refresh lands |
| R474 | `admin_sector.sql` (sector benchmark: last index − first start over a range of daily buckets) is unchanged: it loses one interval at the range start only. Recorded, not fixed. | a sector benchmark understates by one interval per analyzer per range (<0.1 % for a month) |

## Tasks

1. **Migration 00021** + `make generate`; `TestConsumptionAggregatesExposeBoundaryColumns` (integration, store/postgres) asserts the new columns on all four views; the round-trip migration test stays green.
2. **`boundaryDifference`** (pure, `internal/store/postgres/boundary.go`) + unit tests:
   hourly meter (readings on the hour, 100/110/125 → 10, 15), 15-min meter (full hour, not ¾), offset meter
   (:05 readings telescope), gap (fallback to own first), nil register, last bucket (no successor → own last).
   Wire into the four repository methods with the widened query; `TestAnalyticsHourlyMeterIsNotZero` (integration).
3. **Acceptance re-rule** (R472) + fix every integration test whose expected numbers encoded last−first:
   run `internal/service/consumption`, `internal/store/postgres`, `internal/service/{carbon,forecast,renewable,loadprofile,report,analysis,billing}`,
   `internal/seed`, `internal/migrate/recompute`, `internal/api/...` with `-tags=integration`.
4. **Docs:** 04 §4.3, 09 §F3, `docs/performance.md` Findings → fixed; ledger.

## Verification
Per task: the whole touched package with `-race`; `golangci-lint run --concurrency 2`; mutation: drop the
look-ahead (end = own last) → the hourly-meter test must go red.
