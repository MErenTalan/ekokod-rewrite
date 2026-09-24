#!/usr/bin/env bash
# F15c R477: proves a backup restores onto a clean Postgres and the restored
# system serves. Source: a fresh database in the shared test Postgres
# (make test-db-up), migrated and seeded. Target: a throwaway TimescaleDB
# container standing in for the scratch host. backup.sh and restore.sh are the
# scripts an operator runs; only their exec prefixes differ.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

src_container="${SRC_CONTAINER:-ekokod-test-pg}"
src_db="ekokod_restore_src"
target="ekokod-restore-target"
target_port="${TARGET_PORT:-55439}"
api_port="${API_PORT:-18091}"
image="$(docker inspect -f '{{.Config.Image}}' "$src_container")"
work="$(mktemp -d)"
bin="${EKOKOD_BIN:-$root/bin/ekokod}"
password="Guvenli!Sifre-42"
api_pid=""

cleanup() {
  if [ -n "$api_pid" ]; then kill "$api_pid" 2>/dev/null || true; fi
  docker rm -f "$target" >/dev/null 2>&1 || true
  docker exec "$src_container" psql -U ekokod -d postgres -qc "drop database if exists ${src_db} with (force)" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

[ -x "$bin" ] || { echo "build the binary first: go build -o bin/ekokod ./cmd/ekokod (or set EKOKOD_BIN)" >&2; exit 1; }

app_env() {
  export EKOKOD_ENV=development EKOKOD_LOG_FORMAT=text EKOKOD_LOG_LEVEL=warn
  export EKOKOD_DB_URL="$1" EKOKOD_DB_MIN_CONNS=1
  export EKOKOD_HTTP_ADDR="127.0.0.1:${api_port}" EKOKOD_PUBLIC_URL=http://localhost:13000
  export EKOKOD_REDIS_URL=redis://localhost:56379/0 EKOKOD_REDIS_CACHE_DB=8 EKOKOD_REDIS_QUEUE_DB=9
  export EKOKOD_RATE_LIMIT_AUTH=1000/15min EKOKOD_RATE_LIMIT_API=100000/min
  export EKOKOD_ENCRYPTION_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
  export EKOKOD_JWT_SIGNING_KEY=restore-jwt-signing-key-0123456789abcdef
  export EKOKOD_PASSWORD_PEPPER=e2e-password-pepper-0123456789abcdef
  export EKOKOD_DEVICE_FINGERPRINT_SECRET=restore-device-fingerprint-0123456789
  export EKOKOD_STORAGE_ROOT="$work/artifacts-restored"
  export EKOKOD_EPIAS_USERNAME=x EKOKOD_EPIAS_PASSWORD=x EKOKOD_ML_API_KEY=x EKOKOD_SCHEDULER_ENABLED=false
  export EKOKOD_E2E_PASSWORD="$password" EKOKOD_DEMO_PASSWORD="$password"
}

echo "==> Source: ${src_db} in ${src_container}"
docker exec "$src_container" psql -U ekokod -d postgres -v ON_ERROR_STOP=1 -qc "drop database if exists ${src_db} with (force)"
docker exec "$src_container" psql -U ekokod -d postgres -v ON_ERROR_STOP=1 -qc "create database ${src_db}"
src_port="$(docker port "$src_container" 5432/tcp | head -1 | sed 's/.*://')"
app_env "postgres://ekokod:ekokod@localhost:${src_port}/${src_db}?sslmode=disable"
"$bin" migrate up >/dev/null
"$bin" seed >/dev/null
"$bin" seed demo >/dev/null
for v in consumption_hourly consumption_daily consumption_monthly consumption_yearly; do
  docker exec "$src_container" psql -U ekokod -d "$src_db" -qc "call refresh_continuous_aggregate('$v', null, now())" >/dev/null
done
mkdir -p "$work/artifacts/reports/2026"
echo "restore-probe" >"$work/artifacts/reports/2026/probe.txt"

echo "==> Backup"
PG_EXEC="docker exec -i $src_container" ARTIFACTS_EXEC="env" ARTIFACTS_ROOT="$work/artifacts" \
  DB_NAME="$src_db" BACKUP_DIR="$work/backups" ./scripts/backup.sh
dir="$(ls -d "$work"/backups/*/)"

echo "==> Clean host: ${target} (${image})"
docker run -d --name "$target" -e POSTGRES_USER=ekokod -e POSTGRES_PASSWORD=ekokod -e POSTGRES_DB=postgres \
  -p "127.0.0.1:${target_port}:5432" --tmpfs /var/lib/postgresql/data "$image" >/dev/null
for _ in $(seq 60); do
  docker exec "$target" pg_isready -U ekokod -d postgres >/dev/null 2>&1 && break
  sleep 1
done
sleep 2

echo "==> Restore"
mkdir -p "$work/artifacts-restored"
PG_EXEC="docker exec -i $target" ARTIFACTS_EXEC="sh -c" ARTIFACTS_ROOT="$work/artifacts-restored" \
  COMPOSE_SERVICES=none ./scripts/restore.sh --yes "$dir"

echo "==> Verify"
counts() {
  docker exec "$1" psql -U ekokod -d "$2" -Atc "select string_agg(format('select %L, count(*) from %I.%I', schemaname||'.'||relname, schemaname, relname), ' union all ' order by relname) from pg_stat_user_tables where schemaname = 'public'" |
    { read -r q; docker exec "$1" psql -U ekokod -d "$2" -AtF' ' -c "$q order by 1"; }
  for v in consumption_hourly consumption_daily consumption_monthly consumption_yearly; do
    echo "$v $(docker exec "$1" psql -U ekokod -d "$2" -Atc "select count(*) from $v")"
  done
  echo "meter_readings $(docker exec "$1" psql -U ekokod -d "$2" -Atc "select count(*) from meter_readings")"
}
counts "$src_container" "$src_db" >"$work/src.txt"
counts "$target" ekokod >"$work/dst.txt"
if ! diff -u "$work/src.txt" "$work/dst.txt"; then echo "FAIL: row counts differ" >&2; exit 1; fi
echo "row counts equal: $(wc -l <"$work/src.txt") relations, $(awk '/^meter_readings /{print $2}' "$work/src.txt") readings"
grep -q restore-probe "$work/artifacts-restored/reports/2026/probe.txt" || { echo "FAIL: artifacts not restored" >&2; exit 1; }

app_env "postgres://ekokod:ekokod@localhost:${target_port}/ekokod?sslmode=disable"
"$bin" migrate status | tail -3
if "$bin" migrate status | grep -qi pending; then echo "FAIL: pending migrations after restore" >&2; exit 1; fi
"$bin" api >"$work/api.log" 2>&1 &
api_pid=$!
for _ in $(seq 60); do
  curl -fsS "http://127.0.0.1:${api_port}/health/ready" >/dev/null 2>&1 && break
  sleep 1
done
curl -fsS "http://127.0.0.1:${api_port}/health/ready" || { cat "$work/api.log" >&2; echo "FAIL: restored API not ready" >&2; exit 1; }
echo
"$bin" tool loadtest --base "http://127.0.0.1:${api_port}" --email demo@ekokod.com.tr --password "$password" \
  --users 2 --duration 5s --report "$work/load.json" >/dev/null || { cat "$work/load.json" >&2; echo "FAIL: restored API does not serve" >&2; exit 1; }
python3 -c "import json,sys; r=json.load(open('$work/load.json')); print('served', r['requests'], 'requests, pass =', r['pass'])"
echo "PASS: backup restored onto a clean Postgres and serves (a real scratch host is PENDING)"
