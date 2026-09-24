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
mkdir -p "$STAGE/scripts" "$STAGE/docs"
cp scripts/gen-env-docker.sh scripts/backup.sh scripts/restore.sh "$STAGE/scripts/"
chmod +x "$STAGE"/scripts/*.sh
cp docs/runbook-operator.md docs/admin-guide.md "$STAGE/docs/" 2>/dev/null || true
# VERSION (not a secret) travels as .env.bundle, never .env: extracting an
# upgrade over an install must not overwrite .env's POSTGRES_PASSWORD.
# install.sh merges it into .env so compose resolves the saved image tag.
echo "VERSION=$VERSION" > "$STAGE/.env.bundle"
cat > "$STAGE/install.sh" <<'INSTALL'
#!/usr/bin/env bash
# Offline installer (F15c R479). A fresh directory installs; a directory that
# already holds an install (.env.docker present) upgrades it: backup first,
# then the new images, migrations and an idempotent seed. Never pulls or builds.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
command -v docker >/dev/null || { echo "docker is required"; exit 1; }
[ -f images/ekokod-images.tar ] || {
  echo "images/ekokod-images.tar not found — run this script from the directory the bundle tarball was extracted into" >&2
  exit 1
}
bundle_version="$(sed -n 's/^VERSION=//p' .env.bundle)"

if [ -f .env.docker ] && docker compose ps --status running --quiet postgres 2>/dev/null | grep -q .; then
  echo "==> Existing install found: backing up before the upgrade"
  ./scripts/backup.sh
fi

docker load -i images/ekokod-images.tar
# The bundle's version replaces the one in .env; POSTGRES_PASSWORD is kept.
touch .env
grep -v '^VERSION=' .env > .env.tmp || true
echo "VERSION=${bundle_version}" >> .env.tmp
mv .env.tmp .env
# Generates .env.docker and POSTGRES_PASSWORD on a fresh install; leaves an
# existing install's secrets untouched.
./scripts/gen-env-docker.sh
# migrate runs to completion before api/worker/scheduler start (depends_on).
docker compose up -d --pull never --no-build --remove-orphans
docker compose run --rm --no-deps api seed
docker compose ps
INSTALL
chmod +x "$STAGE/install.sh"

mkdir -p "$OUT_DIR"
BUNDLE="$OUT_DIR/ekokod-offline-$VERSION.tar.gz"
tar -czf "$BUNDLE" -C "$STAGE" .
echo "==> $BUNDLE ($(du -h "$BUNDLE" | cut -f1))"
