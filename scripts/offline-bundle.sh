#!/usr/bin/env bash
# Builds an air-gapped install bundle: container images, the compose file, a
# configuration template and an install script, in one tarball.
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
OUT_DIR="${OUT_DIR:-dist}"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

echo "==> Building images"
docker compose build

echo "==> Saving images"
mkdir -p "$STAGE/images"
# Unquoted on purpose: each image must arrive as its own argument.
# shellcheck disable=SC2046
docker save $(docker compose config --images) -o "$STAGE/images/ekokod-images.tar"

echo "==> Staging deployment files"
cp docker-compose.yml "$STAGE/"
cp .env.example "$STAGE/env.template"
cp scripts/install.sh "$STAGE/install.sh" 2>/dev/null || cat > "$STAGE/install.sh" <<'INSTALL'
#!/usr/bin/env bash
# Offline installer. Idempotent: safe to re-run for upgrades.
set -euo pipefail
command -v docker >/dev/null || { echo "docker is required"; exit 1; }
docker load -i images/ekokod-images.tar
[ -f .env.docker ] || { cp env.template .env.docker; echo "Edit .env.docker, then re-run."; exit 1; }
docker compose up -d
docker compose run --rm migrate migrate up
docker compose run --rm api seed
docker compose ps
INSTALL
chmod +x "$STAGE/install.sh"

mkdir -p "$OUT_DIR"
BUNDLE="$OUT_DIR/ekokod-offline-$VERSION.tar.gz"
tar -czf "$BUNDLE" -C "$STAGE" .
echo "==> $BUNDLE ($(du -h "$BUNDLE" | cut -f1))"
