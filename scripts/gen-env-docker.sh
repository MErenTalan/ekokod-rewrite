#!/usr/bin/env bash
# Generates .env.docker: the same variables as .env.example, pointed at the
# compose network's container hostnames, with freshly generated development
# secrets. Never overwrites an existing .env.docker (it holds generated
# secrets that must not be regenerated out from under a running stack).
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [ -f .env.docker ]; then
  echo ".env.docker already exists; not overwriting it. Delete it first if you want fresh secrets." >&2
  exit 0
fi

python3 - <<'PY' > .env.docker
import base64, os, secrets
print(f"""EKOKOD_ENV=development
EKOKOD_LOG_LEVEL=debug
EKOKOD_LOG_FORMAT=text
EKOKOD_TIMEZONE=Europe/Istanbul
EKOKOD_DEFAULT_LOCALE=tr
EKOKOD_HTTP_ADDR=:8080
EKOKOD_PUBLIC_URL=http://localhost:3000
EKOKOD_CORS_ORIGINS=http://localhost:3000
EKOKOD_DB_URL=postgres://ekokod:ekokod@postgres:5432/ekokod?sslmode=disable
EKOKOD_REDIS_URL=redis://redis:6379/0
EKOKOD_ENCRYPTION_KEY={base64.b64encode(os.urandom(32)).decode()}
EKOKOD_JWT_SIGNING_KEY={secrets.token_urlsafe(48)}
EKOKOD_PASSWORD_PEPPER={secrets.token_urlsafe(48)}
EKOKOD_DEVICE_FINGERPRINT_SECRET={secrets.token_urlsafe(48)}
EKOKOD_STORAGE_ROOT=/var/lib/ekokod
EKOKOD_EPIAS_USERNAME=development-only
EKOKOD_EPIAS_PASSWORD=development-only
EKOKOD_ML_URL=http://ml:8000
EKOKOD_ML_API_KEY=development-only
""", end="")
PY

echo "wrote .env.docker"
