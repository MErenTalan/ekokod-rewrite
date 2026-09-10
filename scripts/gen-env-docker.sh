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
#   only .env exists         → its POSTGRES_PASSWORD (if any and non-empty)
#                               is preserved and reused for the new
#                               .env.docker's EKOKOD_DB_URL, never rotated.
#                               Missing or empty → generate fresh (nothing
#                               to preserve, .env.docker doesn't exist
#                               either in this state).
#   only .env.docker exists  → the password already embedded in its
#                               EKOKOD_DB_URL is recovered and used to
#                               (re)create .env, rather than dead-ending
#                               (the compose error for a missing
#                               POSTGRES_PASSWORD tells the operator to run
#                               this script; it must actually be able to
#                               fix that state, not just regenerate a
#                               password that no longer matches the
#                               already-initialised database). If the DSN
#                               can't be parsed, this fails closed instead
#                               of inventing a password .env.docker (left
#                               untouched) doesn't have — see below.
# Whatever the entry state, .env and .env.docker agree on the password
# when this script finishes, and an already-initialised Postgres volume is
# never handed a password it wasn't created with.
#
# Every file this script writes goes to a same-filesystem mkstemp path,
# chmod 600 (belt-and-braces: mkstemp already creates at 0600, but this
# makes the intent explicit rather than relying on that default), then an
# atomic rename into place — so a python3 failure or an interrupted run
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
    """Return key's value from lines, or None if absent OR empty — an
    empty POSTGRES_PASSWORD= can never have initialised a live Postgres
    volume (docker-compose.yml's ${POSTGRES_PASSWORD:?...} refuses to
    start on an empty or unset value), so there is nothing to preserve by
    treating "" as a real value."""
    if lines is None:
        return None
    prefix = key + "="
    for line in lines:
        if line.startswith(prefix):
            value = line[len(prefix):]
            return value or None
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

# Resolve the one value that must never silently rotate. Because the
# both-exist state already returned above, at most one of env_lines /
# env_docker_lines is non-None here, so these are mutually exclusive:
postgres_password = None
source = None

if env_lines is not None:
    # Only .env exists: reuse its password if it has one; if not (e.g. an
    # offline bundle's .env staged with just VERSION so far), there is
    # nothing to recover from either file, so fall through to generate.
    postgres_password = extract(env_lines, "POSTGRES_PASSWORD")
    if postgres_password is not None:
        source = "preserved from existing .env"

elif env_docker_lines is not None:
    # Only .env.docker exists: the password must come from there — never
    # invent one, because .env.docker is left untouched below and a fresh
    # .env password would silently disagree with it (and with whatever
    # password any already-initialised Postgres volume was created
    # under).
    db_url = extract(env_docker_lines, "EKOKOD_DB_URL")
    match = re.search(r"://[^:@/]+:([^@]+)@", db_url or "")
    if match:
        postgres_password = match.group(1)
        source = "recovered from existing .env.docker's EKOKOD_DB_URL"
    else:
        # Fail closed and loudly (task-11 review round 3, finding A).
        # Name the file and the variable; never the DSN or any part of
        # it — it is by definition a value this script could not parse,
        # and this codebase has already had three separate findings
        # about connection strings leaking into error text.
        sys.stderr.write(
            f"error: {ENV_DOCKER_PATH} exists but its EKOKOD_DB_URL does not "
            "contain a password this script can parse out of the DSN's "
            f"userinfo.\n"
            f"Refusing to invent a new POSTGRES_PASSWORD for {ENV_PATH}: "
            f"{ENV_DOCKER_PATH} is left untouched by this script, so a freshly "
            "generated password would silently disagree with the database "
            f"credentials already embedded in {ENV_DOCKER_PATH} (and with "
            "whatever password any already-initialised Postgres volume was "
            "created under).\n"
            f"Fix EKOKOD_DB_URL's password in {ENV_DOCKER_PATH}, or delete "
            f"{ENV_DOCKER_PATH} to start over with a freshly generated "
            "password, then re-run.\n"
        )
        sys.exit(1)

if postgres_password is None:
    postgres_password = secrets.token_urlsafe(24)
    source = "freshly generated"

print(f"POSTGRES_PASSWORD: {source}")

# .env: keep every other line already there (e.g. VERSION, staged by
# scripts/offline-bundle.sh) and set POSTGRES_PASSWORD to the resolved
# value above. Rewriting even when .env already held that exact password
# is harmless (identical content) and keeps this branch uniform across
# all non-skip, non-error starting states.
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
