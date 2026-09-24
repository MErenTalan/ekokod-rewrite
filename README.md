# ekokod

Energy management platform (EKORM): meter data ingestion, consumption analytics, electricity
invoices computed to the kuruş, alarms, reports, solar plants, carbon/ISO 50001 and forecasting.
It is a Go rewrite of the legacy bcem-energy system.

| part | where |
|---|---|
| Go backend: one binary `ekokod` (`api`, `worker`, `scheduler`, `migrate`, `seed`, `recompute`, `tool`) | `cmd/ekokod`, `internal/` |
| Web app and public site (Next.js) | `web/` |
| Forecasting service (Python, no database access) | `ml/` |
| Design system | `design-system/` |
| Database migrations (PostgreSQL 16 + TimescaleDB) | `internal/store/postgres/migrations/` |

## Documentation

- **Specification**: `docs/rewrite/` (01 context … 10 removed behaviours). The phase plan is
  `docs/rewrite/09-implementation-plan.md`.
- **Operating it**: [`docs/runbook-operator.md`](docs/runbook-operator.md) covers install, upgrade,
  backup/restore, monitoring and incidents.
- **Migrating the legacy data**: [`docs/runbook-migration.md`](docs/runbook-migration.md).
- **Company administrators (Turkish)**: [`docs/admin-guide.md`](docs/admin-guide.md).
- **API reference**: [`docs/api/reference.md`](docs/api/reference.md), generated from
  `internal/api/v1/openapi.json`, which is also served at `/api/v1/openapi.json`.
- **Decisions**: [`docs/adr/`](docs/adr/README.md).
- **Reviews**: [`docs/security-review.md`](docs/security-review.md),
  [`docs/performance.md`](docs/performance.md), [`docs/accessibility.md`](docs/accessibility.md).
- **Phase plans and rulings**: `docs/superpowers/plans/`.

## Development

Requirements: Go (see `go.mod`), Node + pnpm, Docker, and Python/uv for `ml/`.

```bash
make env-docker && make up          # the whole stack: web on :3000, api on :8080
make test                           # Go unit tests
make test-db-up test-redis-up       # shared containers for integration tests, then:
EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' \
  go test ./internal/<pkg> -tags=integration
cd web && pnpm install && pnpm test && pnpm build && pnpm test:e2e   # e2e needs the test containers
make generate check-generate        # sqlc
make openapi api-docs               # OpenAPI document, web client types, API reference
make ci                             # what CI runs (without integration tests)
```

## Delivery

`make offline-bundle` builds `dist/ekokod-offline-<version>.tar.gz`, which installs and upgrades
without network access (`install.sh`). `make test-offline-install` and `make test-restore` prove
the install/upgrade path and backup/restore.
