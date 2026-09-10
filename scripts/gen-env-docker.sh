#!/usr/bin/env bash
# Generates the two files docker compose and the app need for a local or
# offline install:
#   .env         — Compose's own interpolation file. Holds POSTGRES_PASSWORD
#                  (and, for an offline bundle, VERSION — staged separately
#                  by scripts/offline-bundle.sh before this script runs, and
#                  preserved here rather than overwritten).
#   .env.docker  — the app's runtime config (env_file for api/worker/
#                  scheduler/migrate), mirroring .env.example with
#                  container hostnames and freshly generated secrets.
# EKOKOD_DB_URL in .env.docker embeds the same POSTGRES_PASSWORD written to
# .env so the two agree by construction.
#
# Idempotent and safe to re-run: refuses to touch anything once .env.docker
# already exists. Never leaves a partial file behind (Important-3 of the
# task-11 review): both files are written to a mktemp path, chmod 600, then
# renamed into place, so a python3 failure or an interrupted run cannot
# leave a sticky empty/partial file that a later run's overwrite-guard
# would then protect forever.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [ -f .env.docker ]; then
  echo ".env.docker already exists; not overwriting it. Delete it (and .env, if you want a fresh POSTGRES_PASSWORD too) first if you want fresh secrets." >&2
  exit 0
fi

command -v python3 >/dev/null || { echo "python3 is required to generate secrets" >&2; exit 1; }

# Staged in the repo root itself (not the system tmpdir) so the final
# `mv` is a same-filesystem rename — atomic, not a cross-device copy that
# could be interrupted mid-write.
TMP_ENV="$(mktemp ./.env.XXXXXX)"
TMP_ENV_DOCKER="$(mktemp ./.env.docker.XXXXXX)"
trap 'rm -f "$TMP_ENV" "$TMP_ENV_DOCKER"' EXIT

python3 - .env "$TMP_ENV" "$TMP_ENV_DOCKER" <<'PY'
import base64, os, secrets, sys

existing_env_path, out_env_path, out_env_docker_path = sys.argv[1], sys.argv[2], sys.argv[3]

# Preserve any lines already in .env (e.g. VERSION, staged by
# scripts/offline-bundle.sh for an offline bundle) other than a prior
# POSTGRES_PASSWORD, which is always regenerated fresh here.
existing_lines = []
if os.path.exists(existing_env_path):
    with open(existing_env_path) as f:
        existing_lines = [
            line.rstrip("\n") for line in f
            if line.strip() and not line.startswith("POSTGRES_PASSWORD=")
        ]

postgres_password = secrets.token_urlsafe(24)

with open(out_env_path, "w") as f:
    f.write("\n".join([*existing_lines, f"POSTGRES_PASSWORD={postgres_password}"]) + "\n")

with open(out_env_docker_path, "w") as f:
    f.write(f"""EKOKOD_ENV=development
EKOKOD_LOG_LEVEL=debug
EKOKOD_LOG_FORMAT=text
EKOKOD_TIMEZONE=Europe/Istanbul
EKOKOD_DEFAULT_LOCALE=tr
EKOKOD_HTTP_ADDR=:8080
EKOKOD_PUBLIC_URL=http://localhost:3000
EKOKOD_CORS_ORIGINS=http://localhost:3000
EKOKOD_RATE_LIMIT_API=120/min
EKOKOD_RATE_LIMIT_AUTH=5/15min
EKOKOD_DB_URL=postgres://ekokod:{postgres_password}@postgres:5432/ekokod?sslmode=disable
EKOKOD_DB_MAX_CONNS=25
EKOKOD_DB_MIN_CONNS=5
EKOKOD_REDIS_URL=redis://redis:6379/0
EKOKOD_REDIS_CACHE_DB=0
EKOKOD_REDIS_QUEUE_DB=1
EKOKOD_ENCRYPTION_KEY={base64.b64encode(os.urandom(32)).decode()}
EKOKOD_JWT_SIGNING_KEY={secrets.token_urlsafe(48)}
EKOKOD_PASSWORD_PEPPER={secrets.token_urlsafe(48)}
EKOKOD_DEVICE_FINGERPRINT_SECRET={secrets.token_urlsafe(48)}
EKOKOD_STORAGE_ROOT=/var/lib/ekokod
EKOKOD_EPIAS_USERNAME=development-only
EKOKOD_EPIAS_PASSWORD=development-only
EKOKOD_ML_URL=http://ml:8000
EKOKOD_ML_API_KEY=development-only
""")
PY

# Best-effort: some filesystems (e.g. this repo's own dev mount, a 9p/
# drvfs WSL mount onto a Windows drive) reject chmod entirely while
# already exposing every file as world-readable/writable regardless, so a
# failure here is not a regression on those and must not abort the run.
# On a real Linux filesystem (CI, production) this reliably narrows the
# secrets' permissions before they are ever visible under their final name.
chmod 600 "$TMP_ENV" "$TMP_ENV_DOCKER" || true
mv "$TMP_ENV" .env
mv "$TMP_ENV_DOCKER" .env.docker

echo "wrote .env and .env.docker"
