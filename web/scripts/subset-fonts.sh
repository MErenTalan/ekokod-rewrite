#!/usr/bin/env bash
# Rebuilds src/styles/fonts/*.woff2 from pinned google/fonts variable TTFs (07 §3, plan D6).
# Needs fonttools + brotli: python3 -m venv ~/.cache/ekokod-fontenv && ~/.cache/ekokod-fontenv/bin/pip install fonttools brotli
set -euo pipefail

SHA=92345ac0dbb28d27dbd32f3a782e84c55eaac214
BASE="https://raw.githubusercontent.com/google/fonts/${SHA}/ofl"
PYFTSUBSET="${PYFTSUBSET:-$HOME/.cache/ekokod-fontenv/bin/pyftsubset}"
OUT="$(cd "$(dirname "$0")/.." && pwd)/src/styles/fonts"
UNICODES="U+0020-007E,U+00A0-017F,U+2013-2014,U+2018-201E,U+2022,U+2026,U+2030,U+2032-2033,U+2039-203A,U+20AC,U+20BA,U+2080-2089,U+2122,U+2190-2193,U+2212,U+2264-2265"
FEATURES="kern,liga,calt,tnum,lnum,case,ccmp,locl,mark,mkmk"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$OUT"

subset() { # <upstream dir> <upstream ttf> <output name>
  curl -sSfL "${BASE}/$1/$2" -o "$tmp/$3.ttf"
  curl -sSfL "${BASE}/$1/OFL.txt" -o "$OUT/$3.OFL.txt"
  "$PYFTSUBSET" "$tmp/$3.ttf" --unicodes="$UNICODES" --layout-features="$FEATURES" \
    --flavor=woff2 --output-file="$OUT/$3.woff2"
  echo "$3.woff2 $(wc -c < "$OUT/$3.woff2") bytes"
}

subset lexend 'Lexend%5Bwght%5D.ttf' lexend
subset sourcesans3 'SourceSans3%5Bwght%5D.ttf' source-sans-3
subset jetbrainsmono 'JetBrainsMono%5Bwght%5D.ttf' jetbrains-mono
