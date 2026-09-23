#!/usr/bin/env bash
# R171: the real API behind the Playwright e2e project. A fresh database in the shared test
# Postgres, reference data plus the e2e and demo fixtures, then `ekokod api` in the foreground
# (Playwright stops it). Never points at anything but the throwaway test containers.
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"

pg_base="${E2E_PG_BASE:-postgres://ekokod:ekokod@localhost:55432}"
db_name="${E2E_DB_NAME:-ekokod_e2e}"
api_port="${E2E_API_PORT:-18080}"

sql() {
  if command -v psql >/dev/null 2>&1; then
    psql "${pg_base}/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -qc "$1"
  else
    docker exec ekokod-test-pg psql -U ekokod -d postgres -v ON_ERROR_STOP=1 -qc "$1"
  fi
}
sql "DROP DATABASE IF EXISTS ${db_name} WITH (FORCE)"
sql "CREATE DATABASE ${db_name}"

export EKOKOD_ENV=development
export EKOKOD_LOG_FORMAT=text
export EKOKOD_LOG_LEVEL="${E2E_LOG_LEVEL:-warn}"
export EKOKOD_HTTP_ADDR="127.0.0.1:${api_port}"
export EKOKOD_PUBLIC_URL="${E2E_WEB_URL:-http://localhost:13000}"
export EKOKOD_DB_URL="${pg_base}/${db_name}?sslmode=disable"
export EKOKOD_DB_MIN_CONNS=1
export EKOKOD_REDIS_URL="${E2E_REDIS_URL:-redis://localhost:56379/0}"
export EKOKOD_REDIS_CACHE_DB=6
export EKOKOD_REDIS_QUEUE_DB=7
# One browser, many logins from 127.0.0.1: the production auth limit would lock the suite out.
export EKOKOD_RATE_LIMIT_AUTH=1000/15min
export EKOKOD_RATE_LIMIT_API=10000/min
export EKOKOD_TRUSTED_PROXIES=127.0.0.1/32,::1/128
export EKOKOD_ENCRYPTION_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
export EKOKOD_JWT_SIGNING_KEY=e2e-jwt-signing-key-0123456789abcdef
export EKOKOD_PASSWORD_PEPPER=e2e-password-pepper-0123456789abcdef
export EKOKOD_DEVICE_FINGERPRINT_SECRET=e2e-device-fingerprint-0123456789abc
export EKOKOD_STORAGE_ROOT="${E2E_STORAGE_ROOT:-$(mktemp -d)}"
export EKOKOD_EPIAS_USERNAME=e2e
export EKOKOD_EPIAS_PASSWORD=e2e
export EKOKOD_ML_API_KEY=e2e
export EKOKOD_SCHEDULER_ENABLED=false
export EKOKOD_E2E_PASSWORD="${EKOKOD_E2E_PASSWORD:-Guvenli!Sifre-42}"
export EKOKOD_DEMO_PASSWORD="${EKOKOD_DEMO_PASSWORD:-Guvenli!Sifre-42}"

bin="${root}/bin/ekokod-e2e"
go build -o "$bin" ./cmd/ekokod
"$bin" migrate up >/dev/null
"$bin" seed >/dev/null
"$bin" seed e2e
"$bin" seed demo
# F8b: a worker beside the API, so a job the screens start really runs
# (report generation, delivery, and R267's finished-job answer). Both are
# stopped together when Playwright stops this script.
"$bin" worker &
worker_pid=$!
"$bin" api &
api_pid=$!
trap 'kill "$worker_pid" "$api_pid" 2>/dev/null || true' EXIT INT TERM
wait "$api_pid"
