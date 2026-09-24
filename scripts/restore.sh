#!/usr/bin/env bash
# Restores a backup made by backup.sh (F15c R476), following TimescaleDB's
# procedure: recreate the database, pre_restore, pg_restore, post_restore.
#
#   restore.sh --yes <backup dir>
#
# In a compose install the app services are stopped first and started after.
# PG_EXEC / ARTIFACTS_EXEC / ARTIFACTS_ROOT / DB_NAME / DB_USER as in backup.sh; set
# COMPOSE_SERVICES=none when restoring outside compose (test-restore.sh).
set -euo pipefail

[ "${1:-}" = "--yes" ] || { echo "usage: $0 --yes <backup dir>  (replaces the database)" >&2; exit 2; }
dir="${2:?backup dir required}"
[ -f "$dir/db.dump" ] || { echo "$dir/db.dump not found" >&2; exit 1; }

PG_EXEC="${PG_EXEC:-docker compose exec -T postgres}"
ARTIFACTS_EXEC="${ARTIFACTS_EXEC:-docker compose run --rm -T --no-deps --entrypoint sh api -c}"
ARTIFACTS_ROOT="${ARTIFACTS_ROOT:-/var/lib/ekokod}"
DB_NAME="${DB_NAME:-ekokod}"
DB_USER="${DB_USER:-ekokod}"
COMPOSE_SERVICES="${COMPOSE_SERVICES:-web api worker scheduler ml}"

(cd "$dir" && sha256sum -c --quiet <(grep -E '^[0-9a-f]{64} ' manifest.txt)) || { echo "checksum mismatch in $dir" >&2; exit 1; }

psql_admin() {
  # shellcheck disable=SC2086 # PG_EXEC is a command prefix and must split.
  $PG_EXEC psql -U "$DB_USER" -d postgres -v ON_ERROR_STOP=1 -qc "$1"
}
psql_db() {
  # shellcheck disable=SC2086
  $PG_EXEC psql -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 -qc "$1"
}

if [ "$COMPOSE_SERVICES" != "none" ]; then
  echo "==> Stopping ${COMPOSE_SERVICES}"
  # shellcheck disable=SC2086
  docker compose stop $COMPOSE_SERVICES
fi

echo "==> Recreating database ${DB_NAME}"
psql_admin "drop database if exists ${DB_NAME} with (force)"
psql_admin "create database ${DB_NAME} owner ${DB_USER}"
psql_db "create extension if not exists timescaledb"
psql_db "select timescaledb_pre_restore()"

echo "==> Restoring"
# --exit-on-error is not used: pg_restore reports the timescaledb extension,
# which already exists, as an error. Everything else is checked below.
# shellcheck disable=SC2086
$PG_EXEC pg_restore -U "$DB_USER" -d "$DB_NAME" --no-owner <"$dir/db.dump" 2>"$dir/restore.log" || true
psql_db "select timescaledb_post_restore()"
if grep -v 'extension "timescaledb" already exists\|errors ignored on restore\|Command was: CREATE EXTENSION\|while PROCESSING TOC\|from TOC entry .* EXTENSION timescaledb\|^$' "$dir/restore.log" | grep -qi 'error'; then
  echo "pg_restore reported errors, see $dir/restore.log" >&2
  exit 1
fi

if [ -f "$dir/artifacts.tar" ] && [ "$ARTIFACTS_EXEC" != "none" ]; then
  echo "==> Restoring artifacts"
  # shellcheck disable=SC2086
  $ARTIFACTS_EXEC "find ${ARTIFACTS_ROOT} -mindepth 1 -delete && tar -C ${ARTIFACTS_ROOT} -xf -" <"$dir/artifacts.tar"
fi

if [ "$COMPOSE_SERVICES" != "none" ]; then
  echo "==> Starting services"
  docker compose up -d --no-build --pull never
fi
echo "==> Restored ${dir}"
