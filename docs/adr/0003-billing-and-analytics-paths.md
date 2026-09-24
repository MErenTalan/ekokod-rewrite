# ADR 0003 — Two consumption paths: billing reads boundaries, analytics reads aggregates

Status: accepted

## Context

An invoice must equal the utility's figure to the kuruş. It is computed from the meter index at the exact period boundary. Charts need speed over millions of rows. The first design let the aggregates compute `last − first` inside each bucket. That undercounted every bucket by one interval, and a meter that reports hourly got zero hourly consumption (F3 Q6, found at scale in F15b).

## Decision

`consumption.Billing` reads boundary readings from `meter_readings` (02 §3.1) and records suspect periods. `consumption.Analytics` reads only the continuous aggregates. It has no reading repository, which a guard test enforces. Since F15q (R470–R472), the aggregates keep each register's first value and the first reading's time. The repository differences consecutive bucket boundaries with a one-bucket look-ahead, so the two paths agree when readings sit on bucket edges.

## Consequences

Dashboards stay fast and now match invoices except where readings are missing. Across a data gap, analytics falls back to the bucket's own readings and never absorbs the gap. Composed month-to-date rows are marked `Partial`.
