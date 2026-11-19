#!/usr/bin/env bash
# clean-import.sh   (PRIVATE — never published)
#
# Brings commits other developers pushed to the shared repo (clean/main) into
# the CURRENT branch of this private repo, as uncommitted changes:
#   git diff <last synced clean commit>..clean/main | git apply --3way
# Review, run the tests, commit (any message), then publish as usual.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
cd "$(git -C "$here" rev-parse --show-toplevel)"
STATE="$(git rev-parse --git-common-dir)/clean-sync"
git fetch -q clean
last=$(sed -n 's/^last_clean=//p' "$STATE" 2>/dev/null || true)
tip=$(git rev-parse refs/remotes/clean/main)
[ -n "$last" ] || { echo "no sync state in $STATE" >&2; exit 1; }
[ "$last" = "$tip" ] && { echo "Nothing to import: clean/main is $tip."; exit 0; }
git diff --quiet && git diff --cached --quiet || { echo "working tree not clean; commit or stash first" >&2; exit 1; }

echo "Importing from the shared repo:"
git log --format='  %h %an  %s' "$last..$tip"
git diff --binary "$last" "$tip" | git apply --3way --index
sed -i "s/^last_clean=.*/last_clean=$tip/" "$STATE"
echo
echo "Applied and staged. Review, test and commit, e.g.:"
echo "  git commit -m \"merge shared repo up to $(git rev-parse --short "$tip")\""
