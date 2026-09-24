#!/usr/bin/env bash
# Backs up an ekokod install (F15c R475): the database as pg_dump custom format
# and the artifacts volume as a tar, into backups/<UTC stamp>/ with a manifest.
# Redis is not backed up (transient queue); ML models are retrainable.
#
#   PG_EXEC          command prefix that runs a client inside the Postgres container
#                    (default: docker compose exec -T postgres)
#   ARTIFACTS_EXEC   command prefix that can read /var/lib/ekokod (default: docker compose
#                    exec -T api); set to "none" to skip the artifacts tar
#   ARTIFACTS_ROOT   default /var/lib/ekokod
#   DB_NAME DB_USER  default ekokod / ekokod
#   BACKUP_DIR       default ./backups
set -euo pipefail

PG_EXEC="${PG_EXEC:-docker compose exec -T postgres}"
ARTIFACTS_EXEC="${ARTIFACTS_EXEC:-docker compose exec -T api}"
ARTIFACTS_ROOT="${ARTIFACTS_ROOT:-/var/lib/ekokod}"
DB_NAME="${DB_NAME:-ekokod}"
DB_USER="${DB_USER:-ekokod}"
BACKUP_DIR="${BACKUP_DIR:-backups}"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
out="${BACKUP_DIR}/${stamp}"
mkdir -p "$out"

echo "==> Dumping database ${DB_NAME}"
# shellcheck disable=SC2086 # PG_EXEC is a command prefix and must split.
$PG_EXEC pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc --no-owner >"$out/db.dump"

if [ "$ARTIFACTS_EXEC" != "none" ]; then
  echo "==> Archiving artifacts"
  # shellcheck disable=SC2086
  $ARTIFACTS_EXEC tar -C "$ARTIFACTS_ROOT" -cf - . >"$out/artifacts.tar"
fi

{
  echo "created=${stamp}"
  echo "database=${DB_NAME}"
  # shellcheck disable=SC2086
  echo "timescaledb=$($PG_EXEC psql -U "$DB_USER" -d "$DB_NAME" -Atc "select extversion from pg_extension where extname = 'timescaledb'")"
  # shellcheck disable=SC2086
  echo "migration=$($PG_EXEC psql -U "$DB_USER" -d "$DB_NAME" -Atc "select max(version_id) from goose_db_version where is_applied")"
  (cd "$out" && { sha256sum ./*.dump ./*.tar 2>/dev/null || true; })
} >"$out/manifest.txt"

echo "==> ${out} ($(du -sh "$out" | cut -f1))"
