#!/usr/bin/env bash
# Builds an air-gapped install bundle: container images, the compose file, a
# configuration template and an install script, in one tarball.
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo none)}"
DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
# Exported so the `docker compose build`/`config`/`save` calls below — and
# the image tag docker-compose.yml's `${VERSION:-dev}` interpolates —
# actually agree on the same value, rather than each silently falling back
# to "dev" on its own. Without this, the images get saved under this
# VERSION's tag while the staged compose file (run later, on a disconnected
# machine, with no VERSION in its environment) would ask Docker for
# "ekokod:dev" instead — a tag that was never loaded.
export VERSION COMMIT DATE
OUT_DIR="${OUT_DIR:-dist}"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

echo "==> Building images ($VERSION)"
docker compose build

echo "==> Saving images"
mkdir -p "$STAGE/images"
# Unquoted on purpose: each image must arrive as its own argument.
# shellcheck disable=SC2046
docker save $(docker compose config --images) -o "$STAGE/images/ekokod-images.tar"

echo "==> Staging deployment files"
cp docker-compose.yml "$STAGE/"
cp .env.example "$STAGE/env.template"
mkdir -p "$STAGE/scripts"
cp scripts/gen-env-docker.sh "$STAGE/scripts/"
chmod +x "$STAGE/scripts/gen-env-docker.sh"
# VERSION (not a secret) travels with the bundle so the target machine's
# `docker compose` resolves the same image tag that was just saved above.
# gen-env-docker.sh preserves this line and adds a freshly generated
# POSTGRES_PASSWORD to the same file at install time.
echo "VERSION=$VERSION" > "$STAGE/.env"
cat > "$STAGE/install.sh" <<'INSTALL'
#!/usr/bin/env bash
# Offline installer. Idempotent: safe to re-run for upgrades.
set -euo pipefail
command -v docker >/dev/null || { echo "docker is required"; exit 1; }
[ -f images/ekokod-images.tar ] || {
  echo "images/ekokod-images.tar not found — run this script from the directory the bundle tarball was extracted into" >&2
  exit 1
}
docker load -i images/ekokod-images.tar
# Generates .env.docker (app secrets) and adds a fresh POSTGRES_PASSWORD to
# this directory's .env, alongside the VERSION already staged in it by
# offline-bundle.sh. No manual edit required — refuses to touch either
# file if .env.docker already exists (e.g. an upgrade re-run).
./scripts/gen-env-docker.sh
# `docker compose up -d` already runs the migrate service to completion
# (api/worker/scheduler all depend on migrate's service_completed_successfully)
# before starting the other services, so migrations do not need a second,
# separate run here.
docker compose up -d
docker compose run --rm api seed
docker compose ps
INSTALL
chmod +x "$STAGE/install.sh"

mkdir -p "$OUT_DIR"
BUNDLE="$OUT_DIR/ekokod-offline-$VERSION.tar.gz"
tar -czf "$BUNDLE" -C "$STAGE" .
echo "==> $BUNDLE ($(du -h "$BUNDLE" | cut -f1))"
