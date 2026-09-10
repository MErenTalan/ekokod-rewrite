#!/usr/bin/env bash
# Generates/reconciles the two files docker compose and the app need for a
# local or offline install:
#   .env         — Compose's own interpolation file. Holds POSTGRES_PASSWORD
#                  (and, for an offline bundle, VERSION, staged separately
#                  by scripts/offline-bundle.sh before this script runs).
#   .env.docker  — the app's runtime config (env_file for api/worker/
#                  scheduler/migrate), mirroring .env.example with
#                  container hostnames and freshly generated secrets.
#
# Reconciles all four possible starting states so a partial run — one file
# present, the other missing, whether from an interrupted first run or a
# deliberate partial secret rotation — never forces a *Postgres password*
# rotation. Postgres only reads POSTGRES_PASSWORD when it initialises an
# empty data directory: on every later boot against an existing volume it
# is a silent no-op, so handing the app a freshly generated password when
# one already exists doesn't rotate anything in Postgres — it just breaks
# the app's ability to authenticate, with no indication why. So:
#   neither file exists      → generate both fresh, one new password.
#   both files exist         → untouched, exit 0.
#   only .env exists         → its POSTGRES_PASSWORD (if any) is preserved
#                               and reused for the new .env.docker's
#                               EKOKOD_DB_URL, never rotated.
#   only .env.docker exists  → the password already embedded in its
#                               EKOKOD_DB_URL is recovered and used to
#                               (re)create .env, rather than dead-ending
#                               (the compose error for a missing
#                               POSTGRES_PASSWORD tells the operator to run
#                               this script; it must actually be able to
#                               fix that state, not just regenerate a
#                               password that no longer matches the
#                               already-initialised database).
# Whatever the entry state, .env and .env.docker agree on the password
# when this script finishes, and an already-initialised Postgres volume is
# never handed a password it wasn't created with.
#
# Every file this script writes is created at mode 0600 directly (not
# chmod'd after the fact) in a same-filesystem temp path and moved into
# place with an atomic rename, so a python3 failure or an interrupted run
# can never leave a sticky empty/partial file behind.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

command -v python3 >/dev/null || { echo "python3 is required to generate secrets" >&2; exit 1; }

python3 - <<'PY'
import base64, os, re, secrets, sys, tempfile

ENV_PATH = ".env"
ENV_DOCKER_PATH = ".env.docker"


def read_lines(path):
    """Return the file's non-blank lines, or None if it doesn't exist."""
    if not os.path.exists(path):
        return None
    with open(path) as f:
        return [line.rstrip("\n") for line in f if line.strip()]


def extract(lines, key):
    if lines is None:
        return None
    prefix = key + "="
    for line in lines:
        if line.startswith(prefix):
            return line[len(prefix):]
    return None


def write_atomic(path, content):
    # Same directory as the destination, so the final os.replace is an
    # atomic rename rather than a cross-filesystem copy that could be
    # interrupted mid-write. The random suffix mktemp-style name is
    # covered by .gitignore's `.env.*` (task-11 review round 2, finding
    # 10) even in the case this process is killed before the replace.
    directory = os.path.dirname(os.path.abspath(path)) or "."
    fd, tmp_path = tempfile.mkstemp(prefix=os.path.basename(path) + ".", dir=directory)
    try:
        try:
            os.chmod(tmp_path, 0o600)
        except OSError:
            # Some filesystems (this repo's own dev mount: a 9p/drvfs WSL
            # mount onto a Windows drive) reject chmod outright while
            # already exposing every file as world-readable/writable
            # regardless, so this must not abort the run. On a real Linux
            # filesystem (CI, production) this narrows the permissions
            # before the content is ever visible under its final name.
            pass
        with os.fdopen(fd, "w") as f:
            f.write(content)
        os.replace(tmp_path, path)
    except BaseException:
        try:
            os.unlink(tmp_path)
        except OSError:
            pass
        raise


env_lines = read_lines(ENV_PATH)
env_docker_lines = read_lines(ENV_DOCKER_PATH)

if env_lines is not None and env_docker_lines is not None:
    print(".env and .env.docker already exist; leaving both untouched.")
    sys.exit(0)

# Resolve the one value that must never silently rotate, in priority
# order: an existing .env wins (it's the file Compose/Postgres actually
# use), then recover it from .env.docker's EKOKOD_DB_URL if that's all
# that's left, and only generate fresh if neither has ever run before.
postgres_password = extract(env_lines, "POSTGRES_PASSWORD")
source = "preserved from existing .env"

if postgres_password is None and env_docker_lines is not None:
    db_url = extract(env_docker_lines, "EKOKOD_DB_URL")
    match = re.search(r"://[^:@/]+:([^@]+)@", db_url or "")
    if match:
        postgres_password = match.group(1)
        source = "recovered from existing .env.docker's EKOKOD_DB_URL"

if postgres_password is None:
    postgres_password = secrets.token_urlsafe(24)
    source = "freshly generated"

print(f"POSTGRES_PASSWORD: {source}")

# .env: keep every other line already there (e.g. VERSION, staged by
# scripts/offline-bundle.sh) and set POSTGRES_PASSWORD to the resolved
# value above. Rewriting even when .env already held that exact password
# is harmless (identical content) and keeps this branch uniform across
# all three non-skip starting states.
kept = [line for line in (env_lines or []) if not line.startswith("POSTGRES_PASSWORD=")]
write_atomic(ENV_PATH, "\n".join([*kept, f"POSTGRES_PASSWORD={postgres_password}"]) + "\n")
print("wrote .env")

# .env.docker: only ever created, never rewritten once it exists — an
# existing one (the only way to reach this line with env_docker_lines
# already set is the "only .env missing" state) keeps its own encryption
# key, JWT signing key, password pepper and device fingerprint secret
# exactly as they were.
if env_docker_lines is None:
    write_atomic(ENV_DOCKER_PATH, f"""EKOKOD_ENV=development
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
    print("wrote .env.docker")
else:
    print(".env.docker already exists; left untouched")
PY
