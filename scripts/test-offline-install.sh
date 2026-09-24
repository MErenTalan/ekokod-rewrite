#!/usr/bin/env bash
# F15c R480: installs the offline bundle into a scratch compose project, writes
# a marker row, re-runs install.sh from a second extraction (the upgrade path)
# and asserts the marker survived, a pre-upgrade backup exists and every
# service is healthy. install.sh never pulls or builds (--pull never
# --no-build), so a missing image fails here instead of reaching a registry.
# A host with networking physically disabled is still PENDING.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck disable=SC2012 # bundle names are ours: ekokod-offline-<version>.tar.gz
bundle="${1:-$(ls -t "$root"/dist/ekokod-offline-*.tar.gz 2>/dev/null | head -1)}"
[ -f "$bundle" ] || { echo "no bundle: run make offline-bundle first (or pass its path)" >&2; exit 1; }
# An optional second bundle is the upgrade target; default: the same bundle again.
upgrade="${2:-$bundle}"

export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-ekokod-offline-test}"
export EKOKOD_API_PORT="${EKOKOD_API_PORT:-18095}"
export EKOKOD_WEB_PORT="${EKOKOD_WEB_PORT:-13095}"
dir="$(mktemp -d)/ekokod"
mkdir -p "$dir"

cleanup() {
  (cd "$dir" && docker compose down -v --remove-orphans >/dev/null 2>&1) || true
  rm -rf "$(dirname "$dir")"
}
trap cleanup EXIT

healthy() {
  for _ in $(seq 120); do
    if curl -fsS "http://127.0.0.1:${EKOKOD_API_PORT}/health/ready" >/dev/null 2>&1 &&
      curl -fsS "http://127.0.0.1:${EKOKOD_WEB_PORT}/api/ping" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  (cd "$dir" && docker compose ps && docker compose logs --tail 30) >&2
  echo "FAIL: stack not healthy" >&2
  return 1
}

install() {
  if ! (cd "$dir" && ./install.sh >"$dir/install.log" 2>&1); then
    tail -20 "$dir/install.log" >&2
    (cd "$dir" && docker compose logs --tail 30 migrate api) >&2 || true
    echo "FAIL: install.sh" >&2
    return 1
  fi
}

psql_install() { (cd "$dir" && docker compose exec -T postgres psql -U ekokod -d ekokod -v ON_ERROR_STOP=1 -Atc "$1"); }

echo "==> Fresh install from $(basename "$bundle")"
tar -xzf "$bundle" -C "$dir"
install
healthy
psql_install "create table offline_install_marker (v text); insert into offline_install_marker values ('before-upgrade')"
users_before="$(psql_install "select count(*) from users")"

echo "==> Upgrade to $(basename "$upgrade"): extract over the install and re-run install.sh"
tar -xzf "$upgrade" -C "$dir"
install
healthy

[ "$(psql_install "select v from offline_install_marker")" = "before-upgrade" ] || { echo "FAIL: marker lost" >&2; exit 1; }
[ "$(psql_install "select count(*) from users")" = "$users_before" ] || { echo "FAIL: users changed on upgrade" >&2; exit 1; }
want="$(sed -n 's/^VERSION=//p' "$dir/.env.bundle")"
(cd "$dir" && docker compose ps --format '{{.Image}}' api) | grep -q ":${want}$" || { echo "FAIL: api not on ${want}" >&2; exit 1; }
ls "$dir"/backups/*/db.dump >/dev/null 2>&1 || { echo "FAIL: no pre-upgrade backup" >&2; exit 1; }
echo "==> Restore the pre-upgrade backup into the running install"
psql_install "delete from offline_install_marker" >/dev/null
backups=("$dir"/backups/*/)
backup="${backups[0]}"
(cd "$dir" && ./scripts/restore.sh --yes "$backup" >"$dir/restore.log" 2>&1) || { tail -20 "$dir/restore.log" >&2; echo "FAIL: restore.sh" >&2; exit 1; }
healthy
[ "$(psql_install "select v from offline_install_marker")" = "before-upgrade" ] || { echo "FAIL: restore did not bring the marker back" >&2; exit 1; }
(cd "$dir" && docker compose ps --format '{{.Service}} {{.State}} {{.Health}}')
echo "PASS: offline install, upgrade without data loss, pre-upgrade backup restored (networking-off host PENDING)"
