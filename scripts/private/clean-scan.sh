#!/usr/bin/env bash
# clean-scan.sh <dir> [message-file] — exits 1 if anything in <dir> (or the
# commit message) matches forbidden.txt and is not covered by allow.txt.
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
dir=${1:?dir}; msg=${2:-}
pat=$(grep -vE '^\s*(#|$)' "$here/forbidden.txt" | paste -sd'|')
allow=$(grep -vE '^\s*(#|$)' "$here/allow.txt" 2>/dev/null | paste -sd'|')
filter() { if [ -n "$allow" ]; then grep -vE -- "$allow"; else cat; fi; }
hits=$(cd "$dir" && grep -rInE -i --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=.next -- "$pat" . | sed 's#^\./##' | filter)
names=$(cd "$dir" && find . -path ./.git -prune -o -path ./web/node_modules -prune -o -print | sed 's#^\./##' | grep -iE -- "$pat|^(CLAUDE|AGENTS)\.md$|/\.claude(/|$)|^\.claude" | sed 's/^/PATH: /' | filter)
mhits=""; [ -n "$msg" ] && mhits=$(grep -nE -i -- "$pat" "$msg" | sed 's/^/MESSAGE:/')
out=$(printf '%s\n%s\n%s\n' "$names" "$hits" "$mhits" | sed '/^$/d')
if [ -n "$out" ]; then printf '%s\n' "$out"; echo "clean-scan: $(printf '%s\n' "$out" | wc -l) hit(s)" >&2; exit 1; fi
echo "clean-scan: OK" >&2
