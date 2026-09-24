# Operator runbook — running ekokod

For whoever installs, upgrades, backs up and monitors an ekokod install. The legacy data migration
has its own runbook: [`runbook-migration.md`](runbook-migration.md). Company administrators use
[`admin-guide.md`](admin-guide.md).

## 1. What runs

One `docker compose` project (`docker-compose.yml`):

| service | image | role | state |
|---|---|---|---|
| `postgres` | `timescale/timescaledb:2.30.0-pg16` | database (TimescaleDB for readings) | volume `postgres-data` |
| `redis` | `redis:7.4.11-alpine` | job queue and cache (append-only file) | volume `redis-data` |
| `migrate` | `ekokod:<version>` | runs `ekokod migrate up` once, then exits; the app services wait for it | — |
| `api` | `ekokod:<version>` | HTTP API on `:8080` (host port `EKOKOD_API_PORT`, default 8080) | volume `artifacts` (reports, uploads) |
| `worker` | `ekokod:<version>` | background jobs (ingestion, bills, reports, alarms, forecasts) | volume `artifacts` |
| `scheduler` | `ekokod:<version>` | cron that enqueues the periodic jobs; leader-elected, so two are safe | — |
| `ml` | `ekokod-ml:<version>` | forecasting/anomaly service; no database access | volume `ml-models` |
| `web` | `ekokod-web:<version>` | Next.js site and app on `:3000` (host port `EKOKOD_WEB_PORT`, default 3000) | — |

Put a TLS-terminating reverse proxy in front of `web` (and `api` if mobile clients call it
directly). List its addresses in `EKOKOD_TRUSTED_PROXIES` so rate limits and audit logs see the
real client address.

## 2. Install (offline bundle)

On a machine with network access and the source: `make offline-bundle` →
`dist/ekokod-offline-<version>.tar.gz`. It holds every image (including `ml` and `web`), the compose
file, `install.sh`, `scripts/{gen-env-docker,backup,restore}.sh` and these docs.

On the target (Docker + the compose plugin, no network needed):

```bash
mkdir -p /opt/ekokod && tar -xzf ekokod-offline-<version>.tar.gz -C /opt/ekokod
cd /opt/ekokod && ./install.sh
```

A fresh install generates `.env` (compose's `POSTGRES_PASSWORD`, `VERSION`) and `.env.docker`
(the app's secrets) with fresh random values, starts the stack with `--pull never --no-build`, and
loads the reference data (`ekokod seed`). Then:

1. Edit `.env.docker` for the site: `EKOKOD_ENV=production`, `EKOKOD_PUBLIC_URL`,
   `EKOKOD_CORS_ORIGINS`, `EKOKOD_TRUSTED_PROXIES`, EPİAŞ credentials, `EKOKOD_ML_API_KEY`
   (the same value is used by `ml`). `docker compose up -d` applies the changes.
2. `docker compose exec api ekokod config:check` prints every resolved value with secrets masked.
3. Create the first platform administrator:
   `docker compose exec -e EKOKOD_NEW_USER_PASSWORD='…' api ekokod user create --help`.

**Keep `.env` and `.env.docker` safe.** Losing `EKOKOD_ENCRYPTION_KEY` makes every stored
integration credential (OSOS, SMTP, iSolar) unreadable. Keep a copy apart from the backups.

## 3. Upgrade

```bash
cd /opt/ekokod
tar -xzf ekokod-offline-<new version>.tar.gz -C .   # over the install; .env is not in the bundle
./install.sh
```

With `.env.docker` present and Postgres running, `install.sh` **backs up first** (§4), loads the
new images, switches `VERSION`, starts the stack (migrations run before the app services), and
re-runs the idempotent seed. `scripts/test-offline-install.sh` proves this path: a row written
before the upgrade is still there after it.

**Rollback.** If a migration fails, the old containers keep running on the old images. Put the
previous `VERSION` back in `.env`, then `./scripts/restore.sh --yes backups/<pre-upgrade stamp>`.

## 4. Backup and restore

```bash
make backup            # or ./scripts/backup.sh in the install directory
```

This writes `backups/<UTC stamp>/` with `db.dump` (`pg_dump -Fc`), `artifacts.tar` (the
`artifacts` volume) and `manifest.txt` (version, TimescaleDB version, last migration, sha256s).
Redis is not backed up: the queue is transient and the scheduler re-creates periodic jobs. ML
models can be rebuilt with `ekokod recompute forecasts`. Copy `backups/` off the host. A daily
cron entry is enough: readings are re-fetched from the providers for up to
`EKOKOD_INGEST_INITIAL_LOOKBACK`.

```bash
./scripts/restore.sh --yes backups/<stamp>   # or: make restore BACKUP=backups/<stamp>
```

This stops the app services, checks the checksums, recreates the database, and runs TimescaleDB's
`timescaledb_pre_restore()` → `pg_restore` → `timescaledb_post_restore()`. It then restores the
artifacts and starts the stack. `make test-restore` proves the procedure end to end: it restores
onto a clean TimescaleDB container and checks row counts, migrations, `/health/ready` and a
signed-in load run. **Still to do:** a restore onto a separate physical host.

## 5. Health, metrics and alerts

- `GET /health/live`: the process is up. `GET /health/ready`: database, Redis and migrations
  are OK (compose's healthcheck uses it).
- Metrics (Prometheus) listen inside the compose network only: `api:9464`, `worker:9465`,
  `scheduler:9466`. Scrape config: `deploy/observability/prometheus/scrape.example.yml`.
  Dashboard: `deploy/observability/grafana/ekokod-overview.json`.
- Alert rules are in `deploy/observability/prometheus/alerts.yml`:

| alert | fires when | first look |
|---|---|---|
| `EkokodAPILatencyHigh` | API p95 > 1 s for 10 min | `docker compose logs api`, Postgres load, the slow route in the dashboard |
| `EkokodAPIErrorRateHigh` | 5xx > 2 % | api logs (JSON, `request_id`) |
| `EkokodJobFailureRateHigh` | job failures > 10 % over 30 min | worker logs; the Messages tab lists failed jobs per company |
| `EkokodQueueBacklog` / `EkokodQueueRetries` | > 500 pending / > 50 retrying | worker running? `EKOKOD_WORKER_CONCURRENCY`; provider outage |
| `EkokodIntegrationStale` | a provider has had no success for 26 h | the provider's credentials in company settings; provider status |
| `EkokodProcessDown` | api/worker/scheduler not scraped | `docker compose ps`; the worker exits on purpose when Redis dies and is restarted |

## 6. Routine operations

| task | command |
|---|---|
| status | `docker compose ps` |
| logs | `docker compose logs -f --tail 200 api worker scheduler` |
| configuration check | `docker compose exec api ekokod config:check` |
| migrations applied | `docker compose exec api ekokod migrate status` |
| rebuild derived data after a fix or a backfill | `docker compose exec worker ekokod recompute consumption\|bills\|reports\|carbon\|forecasts --help` |
| refresh the consumption aggregates after a large backfill | `recompute consumption` (it refreshes every continuous aggregate over the range) |
| create a user | `ekokod user create` (password from `EKOKOD_NEW_USER_PASSWORD`) |
| version | `docker compose exec api ekokod version` |

Schedules (Europe/Istanbul) come from `EKOKOD_SCHEDULE_*`. Defaults: ingestion 03:00, forecasts
04:00, carbon 04:30, bills 05:00, EPİAŞ 14:00, alarms hourly, monthly reports on the 2nd at 06:00,
yearly on 3 January, iSolar sync every 15 min.

## 7. Incidents

- **Readings stopped arriving.** Check `EkokodIntegrationStale` and the company's integration
  settings. When the provider is back, the next ingestion run catches up over the lookback window.
- **Figures look wrong after a data correction.** Run `ekokod recompute consumption` for the
  affected range, then `bills`/`reports`. Recompute is idempotent and skips finished work unless you
  pass `--force`.
- **Disk filling.** The readings hypertable compresses after `EKOKOD_COMPRESSION_AFTER` (90 days).
  An optional retention is `EKOKOD_READING_RETENTION`. Backups under `backups/` are yours to rotate.
- **Rate limit locking users out.** `EKOKOD_RATE_LIMIT_AUTH` (default 5 per 15 min per address).
  Behind a proxy, check `EKOKOD_TRUSTED_PROXIES`, or every user shares the proxy's address.

## 8. Security notes

See [`security-review.md`](security-review.md): secrets only in `.env.docker` (never in the
image), the CSP, the rate limits, uploads (type allow-list, 30 MB), and the accepted residual risks.
