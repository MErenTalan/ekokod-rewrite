# F15c — backup/restore, offline bundle, accessibility audit, handover docs

09 §F15's remaining acceptance criteria: backup **restored** and serving; offline install reaching a
healthy stack; `install.sh` re-run upgrades without data loss; a11y audit with no critical/serious
findings; every document in `docs/` current. Things this machine cannot do (a real scratch host,
a host with networking physically off) are **PENDING**, never faked.

## Rulings

| id | ruling | cost if wrong |
|----|--------|---------------|
| R475 | Backup = `pg_dump -Fc` of the `ekokod` database + a tar of the `artifacts` volume (`/var/lib/ekokod`), written to `backups/<UTC stamp>/` with a `manifest.txt` (version, sha256s). Redis is not backed up: the queue is transient and the scheduler re-enqueues; ML models are retrainable (`ekokod recompute forecasts`). | a queued but unrun job is lost at restore — the scheduler's next tick re-creates it |
| R476 | Restore follows TimescaleDB's procedure: stop app services, recreate the DB, `create extension timescaledb`, `timescaledb_pre_restore()`, `pg_restore`, `timescaledb_post_restore()`, restore artifacts, start. Refuses to run without `--yes`. | an operator restores over live data by accident — guarded by `--yes` |
| R477 | `scripts/backup.sh`/`restore.sh` take `PG_EXEC` / `ARTIFACTS_EXEC` command prefixes (default: `docker compose exec -T postgres` / `… api`), so `scripts/test-restore.sh` runs the **same scripts** against the shared test Postgres (source) and a fresh throwaway Timescale container (the "clean host"), then proves: migrations up to date, every table's row count equal, `ekokod api` on the restored DB answers `/health/ready` and serves a logged-in consumption request. A real second host is PENDING. | the scratch container shares the kernel/Docker with the source — a host-level difference (disk, Docker version) is untested until the PENDING run |
| R478 | Compose: `ml` and `web` get explicit `image: ekokod-ml:${VERSION}` / `ekokod-web:${VERSION}` so the bundle's `docker save` and the target's `up` agree; host ports become `${EKOKOD_API_PORT:-8080}` / `${EKOKOD_WEB_PORT:-3000}`; the Go services listen for metrics on `0.0.0.0:946x` inside the network (F15b note) and nothing publishes them. | none |
| R479 | `install.sh`: fresh install → generate env, `up -d --pull never --no-build`, seed. Existing install (`.env.docker` present) → **backup first** (R475), load images, `up -d --pull never --no-build` (migrate runs first), seed (idempotent). `--pull never` is how the test proves no registry access. | an upgrade whose migration fails leaves the old DB + a fresh backup; rollback is `restore.sh` from that backup (runbook) |
| R480 | `scripts/test-offline-install.sh` installs the bundle into a scratch compose project (`ekokod-offline-test`, alternate ports), writes a marker row, re-runs `install.sh` from a second extraction (upgrade), and asserts the marker survived and every service is healthy; then tears the project down with its volumes. A host with networking disabled is PENDING. | as R477 |
| R481 | Page a11y: `tests/e2e/a11y-pages.spec.ts` logs in as the demo admin and runs axe (wcag2a/aa, 21aa) on every `/ekorm` route and every `/auth` page; **critical and serious** fail the test (09 §F15's bar), moderate/minor are listed in `docs/accessibility.md`. The Storybook sweep (`make web-a11y`) runs once at phase end. | a moderate finding ships — recorded, not hidden |
| R482 | API reference: `ekokod tool openapi --format markdown` renders the generated OpenAPI document to `docs/api/reference.md` (the JSON itself stays `internal/api/v1/openapi.json`, `make openapi`, served at `/api/v1/openapi.json`); `make api-docs` regenerates both, `make check-api-docs` fails when they drift (same pattern as `check-generate`). | none |
| R483 | ADRs in `docs/adr/` (MADR-lite: context, decision, consequences) for the decisions the handover reader needs: modular Go monolith; Postgres + TimescaleDB; billing vs analytics paths (incl. F15q); asynq jobs + leader-elected scheduler; server-side sessions; Next.js web with a generated API client; ML sidecar without DB access; offline bundle delivery. | none |
| R484 | The administrator guide is written in **Turkish** (its readers are the customer's company admins); the operator runbook, ADRs and API reference are English like the rest of `docs/`. | a Turkish-only guide for a non-Turkish operator — the runbook covers operations in English |

## Tasks
1. **Backup/restore** — `scripts/{backup,restore,test-restore}.sh`, `make backup|restore|test-restore`; run `test-restore.sh` for real.
2. **Offline bundle** — compose changes (R478), bundle includes backup/restore scripts + docs, new `install.sh` (R479), `scripts/test-offline-install.sh` (R480); build and run it.
3. **API reference** — markdown renderer + test (golden on a tiny doc), `make api-docs`, `check-api-docs`.
4. **A11y** — page sweep spec, fix what it finds, Storybook sweep, `docs/accessibility.md`.
5. **Docs** — `docs/runbook-operator.md`, `docs/admin-guide.md` (TR), `docs/adr/*`, README + every `docs/` file reviewed for currency.
6. **Phase end** — self-review (authorisation/tenancy first, then a11y/design), Go gates, full e2e with the worker, handoff, `docs/SENDEN-GEREKENLER.md`.

## Verification
```bash
make backup && ./scripts/test-restore.sh
make offline-bundle && ./scripts/test-offline-install.sh
make check-api-docs
pnpm exec playwright test tests/e2e/a11y-pages.spec.ts && make web-a11y
```
