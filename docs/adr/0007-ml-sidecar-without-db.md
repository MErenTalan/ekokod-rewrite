# ADR 0007 — Forecasting in a Python sidecar with no database access

Status: accepted

## Context

Forecasting and anomaly detection are best served by the Python ML ecosystem. A second process with database credentials would double the attack surface and bypass tenancy rules.

## Decision

`ml/` is a small Python HTTP service. The Go side sends it the history it needs and stores what it returns. The service has no database driver or credentials (`python -m ekokod_ml.nodb` proves it in the image). It authenticates callers with `EKOKOD_ML_API_KEY` and keeps models in its own volume.

## Consequences

Tenancy stays entirely in Go. Models are disposable (`ekokod recompute forecasts`), so ML state is not backed up. With the ML service down, forecast screens report "unavailable" and nothing else is affected.
