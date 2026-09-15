# Appendix — Configuration Reference

## Legacy configuration (for the migration)

These variables exist on the production host today. The migration tooling needs several of them.

| Variable | Purpose | Needed by the migration |
|----------|---------|-------------------------|
| `MONGODB_URI` | Legacy database connection | **Yes** — extract |
| `MONGO_INITDB_ROOT_USERNAME` / `_PASSWORD` | Legacy database credentials | **Yes** |
| `ENCRYPTION_SECRET_KEY` | Key used to encrypt integration credentials and as the bcrypt pepper | **Yes** — decrypting credentials and understanding the password hashes |
| `NEXTAUTH_SECRET` | Session signing secret | No |
| `NEXTAUTH_URL` | Public application URL | Reference only |
| `PRODUCTION_DOMAIN` | Public domain used by the cron entries | Reference only |
| `CRON_API_KEY` | Shared key that bypassed all authentication | **Removed** — the mechanism does not exist in the new system |
| `AI_SERVICE_URL` / `AI_SERVICE_API_KEY` | Legacy ML service | Reference only |
| `EPIAS_USERNAME` / `EPIAS_PASSWORD` | EPİAŞ transparency platform credentials | **Yes** — carried over |
| `REDIS_URL` / `REDIS_HOST` / `REDIS_PORT` | Redis | Reference only |
| `GOOGLE_ANALYTICS_SRC` / `GOOGLE_ANALYTICS_ID` / `NEXT_PUBLIC_GOOGLE_ANALYTICS_SHEET` | Analytics | Carried over, behind a flag |
| `REPORTS_STORAGE_PATH` | Report artifact root (default `/opt/reports`) | **Yes** — artifact copy |
| `DEVICE_FINGERPRINT_SECRET` | Device-binding pepper | No |

Artifact directories on the host: `/opt/bills`, `/opt/reports`, `/opt/documents`,
`/opt/bcem-energy` (application), `/opt/carbon` (the separate carbon application).

---

## New configuration

Loaded from the environment, validated at startup. **The process refuses to start on an invalid
configuration.** No secret has a default.

### Core

| Variable | Required | Default | Purpose |
|----------|:--------:|---------|---------|
| `BCEM_ENV` | | `production` | `development` \| `staging` \| `production` |
| `BCEM_LOG_LEVEL` | | `info` | `debug` \| `info` \| `warn` \| `error` |
| `BCEM_LOG_FORMAT` | | `json` | `json` \| `text` |
| `BCEM_TIMEZONE` | | `Europe/Istanbul` | Business timezone for all period logic |
| `BCEM_DEFAULT_LOCALE` | | `tr` | `tr` \| `en` |

### HTTP

| Variable | Required | Default | Purpose |
|----------|:--------:|---------|---------|
| `BCEM_HTTP_ADDR` | | `:8080` | API listen address |
| `BCEM_PUBLIC_URL` | ✅ | | Canonical external URL; used in e-mails and OAuth redirects |
| `BCEM_CORS_ORIGINS` | | *(none)* | Comma-separated allowed origins |
| `BCEM_TRUSTED_PROXIES` | | *(none)* | CIDRs whose `X-Forwarded-For` is honoured |
| `BCEM_RATE_LIMIT_API` | | `120/min` | Per-principal API bucket |
| `BCEM_RATE_LIMIT_AUTH` | | `5/15min` | Per IP+email auth bucket |

### Database

| Variable | Required | Default | Purpose |
|----------|:--------:|---------|---------|
| `BCEM_DB_URL` | ✅ | | `postgres://user:pass@host:5432/bcem?sslmode=require` |
| `BCEM_DB_MAX_CONNS` | | `25` | Pool size |
| `BCEM_DB_MIN_CONNS` | | `5` | |
| `BCEM_DB_MAX_CONN_LIFETIME` | | `1h` | |
| `BCEM_DB_STATEMENT_TIMEOUT` | | `30s` | |
| `BCEM_READING_RETENTION` | | *(unlimited)* | Raw reading retention; unset means keep everything |
| `BCEM_COMPRESSION_AFTER` | | `90d` | Hypertable compression threshold |

### Redis

| Variable | Required | Default | Purpose |
|----------|:--------:|---------|---------|
| `BCEM_REDIS_URL` | ✅ | | `redis://host:6379/0` |
| `BCEM_REDIS_CACHE_DB` | | `0` | |
| `BCEM_REDIS_QUEUE_DB` | | `1` | Separated from cache so a cache flush cannot drop jobs |

### Security

| Variable | Required | Default | Purpose |
|----------|:--------:|---------|---------|
| `BCEM_ENCRYPTION_KEY` | ✅ | | 32-byte base64 AES-256-GCM key for credentials at rest |
| `BCEM_JWT_SIGNING_KEY` | ✅ | | Access-token signing key |
| `BCEM_PASSWORD_PEPPER` | ✅ | | Server-side bcrypt pepper |
| `BCEM_DEVICE_FINGERPRINT_SECRET` | ✅ | | HMAC key for device binding |
| `BCEM_ACCESS_TOKEN_TTL` | | `15m` | |
| `BCEM_REFRESH_TOKEN_TTL` | | `24h` | `720h` when "remember me" is used |
| `BCEM_BCRYPT_COST` | | `12` | |
| `BCEM_PASSWORD_HISTORY_SIZE` | | `5` | |
| `BCEM_LEGACY_ENCRYPTION_KEY` | migration only | | The legacy `ENCRYPTION_SECRET_KEY`, used once during migration |

### Workers and scheduler

| Variable | Required | Default | Purpose |
|----------|:--------:|---------|---------|
| `BCEM_WORKER_CONCURRENCY` | | `10` | |
| `BCEM_JOB_MAX_RETRIES` | | `5` | Must be zero or greater |
| `BCEM_JOB_TIMEOUT` | | `30m` | Per-task ceiling |
| `BCEM_INGEST_SANITY_MULTIPLE` | | `10` | Register-jump rejection multiple (R13); must be > 1 |
| `BCEM_INGEST_FUTURE_TOLERANCE` | | `15m` | How far into the future a reading's timestamp may sit |
| `BCEM_INGEST_INITIAL_LOOKBACK` | | `720h` | First-ever fetch lookback (30 days, R18) |
| `BCEM_SCHEDULER_ENABLED` | | `true` | Set false on secondary instances |
| `BCEM_SCHEDULE_INGESTION` | | `0 3 * * *` | |
| `BCEM_SCHEDULE_EPIAS` | | `0 14 * * *` | |
| `BCEM_SCHEDULE_ALARMS` | | `0 * * * *` | |
| `BCEM_SCHEDULE_BILLING` | | `0 5 * * *` | |
| `BCEM_SCHEDULE_FORECAST` | | `0 4 * * *` | |
| `BCEM_SCHEDULE_CARBON` | | `30 4 * * *` | |
| `BCEM_SCHEDULE_REPORTS_MONTHLY` | | `0 6 2 * *` | |
| `BCEM_SCHEDULE_REPORTS_YEARLY` | | `0 7 3 1 *` | |

### Storage

| Variable | Required | Default | Purpose |
|----------|:--------:|---------|---------|
| `BCEM_STORAGE_ROOT` | ✅ | | Root for all file artifacts |
| `BCEM_UPLOAD_MAX_BYTES` | | `31457280` | 30 MB |
| `BCEM_UPLOAD_ALLOWED_TYPES` | | *(see docs)* | MIME allow-list |

### External services

| Variable | Required | Default | Purpose |
|----------|:--------:|---------|---------|
| `BCEM_EPIAS_USERNAME` | ✅ | | EPİAŞ transparency platform |
| `BCEM_EPIAS_PASSWORD` | ✅ | | |
| `BCEM_EPIAS_CAS_URL` | | `https://giris.epias.com.tr/cas/v1/tickets` | EPİAŞ CAS ticket endpoint; must be https (M3, F2 final review B) |
| `BCEM_EPIAS_BASE_URL` | | `https://seffaflik.epias.com.tr/electricity-service` | EPİAŞ electricity-service base URL; must be https |
| `BCEM_ML_URL` | | `http://ml:8000` | Python service |
| `BCEM_ML_API_KEY` | ✅ | | Shared secret for the ML service |
| `BCEM_ML_TIMEOUT` | | `60s` | |
| `BCEM_WEATHER_PROVIDER` | | *(none)* | Unset disables weather features cleanly |
| `BCEM_WEATHER_API_KEY` | | | Required when a provider is set |
| `BCEM_MAP_TILE_URL` | | *(none)* | Tile source; unset degrades the map to a coordinate list |
| `BCEM_ISOLAR_REDIRECT_URL` | | derived | OAuth callback |
| `BCEM_PINNED_CERTS` | | *(none)* | `host=base64(der)` pairs for self-signed provider certificates |

### Feature flags

| Variable | Default | Purpose |
|----------|---------|---------|
| `BCEM_FEATURE_SELF_REGISTRATION` | `false` | Public sign-up |
| `BCEM_FEATURE_PRICING_PAGE` | `false` | Public pricing page |
| `BCEM_FEATURE_SMS_ALARMS` | `false` | Only true when an SMS gateway is configured |
| `BCEM_FEATURE_ANALYTICS` | `false` | Disabled in on-premise installs |
| `BCEM_FEATURE_TRACING` | `false` | OpenTelemetry export |

### Frontend (build-time)

| Variable | Required | Purpose |
|----------|:--------:|---------|
| `NEXT_PUBLIC_API_URL` | ✅ | API base URL |
| `NEXT_PUBLIC_APP_BASE_PATH` | | `/ekorm` |
| `NEXT_PUBLIC_MAP_TILE_URL` | | Tile source |
| `NEXT_PUBLIC_ANALYTICS_ID` | | Only when the analytics flag is on |

---

## `bcem config:check`

Validates the whole configuration, prints each resolved value with secrets masked, and exits
non-zero on any problem. It is the first thing the install script runs and the first thing an
operator runs when something is wrong.
