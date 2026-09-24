# Architecture decision records

Short records of the decisions a maintainer needs to know about. Each one gives the context, the decision and its consequences. The binding rulings behind them are in `docs/rewrite/` and in the phase plans under `docs/superpowers/plans/`.

- [0001 — A modular Go monolith with one binary](0001-modular-go-monolith.md)
- [0002 — PostgreSQL with TimescaleDB for everything, including readings](0002-postgres-timescaledb.md)
- [0003 — Two consumption paths: billing reads boundaries, analytics reads aggregates](0003-billing-and-analytics-paths.md)
- [0004 — Background jobs on Redis (asynq); one leader-elected scheduler](0004-jobs-asynq-leader-scheduler.md)
- [0005 — Server-side sessions behind short-lived access cookies](0005-sessions-and-tokens.md)
- [0006 — Next.js web app with an API client generated from OpenAPI](0006-nextjs-web-generated-client.md)
- [0007 — Forecasting in a Python sidecar with no database access](0007-ml-sidecar-without-db.md)
- [0008 — Delivery as an offline bundle for on-premises installs](0008-offline-bundle-delivery.md)
