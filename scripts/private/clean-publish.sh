#!/usr/bin/env bash
# clean-publish.sh -m "message" [ref]   (PRIVATE — never published)
#
# Publishes a snapshot of <ref> (default: main) to the shared repo
# (remote `clean` = git@github.com:MErenTalan/new-ekokod-clean.git) as ONE new
# commit on its main branch:
#   - private-only paths (private-paths.txt) are removed,
#   - the tree and the message must pass clean-scan.sh,
#   - the commit has no trailers and the configured user as author/committer,
#   - its parent is the current clean/main, so the shared history stays linear.
# Refuses if clean/main has commits not yet imported (run clean-import.sh).
# Add -n for a dry run (build + scan + show the diff, no commit, no push).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
top=$(git -C "$here" rev-parse --show-toplevel)
cd "$top"
CLEAN_URL=git@github.com:MErenTalan/new-ekokod-clean.git
STATE="$(git rev-parse --git-common-dir)/clean-sync"

msg=""; dry=0
while getopts "m:n" o; do case $o in m) msg=$OPTARG;; n) dry=1;; *) exit 2;; esac; done
shift $((OPTIND-1))
ref=${1:-main}
[ -n "$msg" ] || { echo "usage: $0 -m \"message\" [-n] [ref]" >&2; exit 2; }

# Remote `clean` and its push guard are set up by install-hooks.sh.
[ "$(git remote get-url clean 2>/dev/null)" = "$CLEAN_URL" ] || { echo "remote clean missing or wrong: run install-hooks.sh" >&2; exit 1; }
[ -f "$(git rev-parse --git-common-dir)/hooks/pre-push" ] || { echo "pre-push guard missing: run install-hooks.sh" >&2; exit 1; }
git fetch -q clean

last_clean=$(sed -n 's/^last_clean=//p' "$STATE" 2>/dev/null || true)
tip=$(git rev-parse -q --verify refs/remotes/clean/main || true)
if [ -n "$tip" ] && [ "$tip" != "$last_clean" ]; then
  echo "clean/main ($tip) has commits that were not imported here (last synced: ${last_clean:-none})." >&2
  echo "Run scripts/private/clean-import.sh, commit the result, then publish again." >&2
  exit 1
fi

work=$(mktemp -d); trap 'rm -rf "$work"' EXIT
mkdir "$work/tree"
git archive "$ref" | tar -x -C "$work/tree"
while read -r p; do case "$p" in \#*|'') continue;; esac; rm -rf "${work:?}/tree/$p"; done < "$here/private-paths.txt"
printf '%s\n' "$msg" > "$work/msg"
if ! bash "$here/clean-scan.sh" "$work/tree" "$work/msg"; then
  echo "REFUSED: the snapshot or the message contains forbidden text (see above)." >&2; exit 1
fi

export GIT_INDEX_FILE="$work/index"
git --work-tree="$work/tree" add -A -f .
tree=$(git write-tree)
unset GIT_INDEX_FILE
if [ -n "$tip" ] && [ "$(git rev-parse "$tip^{tree}")" = "$tree" ]; then
  echo "Nothing to publish: clean/main already has this exact tree."; exit 0
fi
if [ -n "$tip" ]; then git diff --stat "$tip" "$tree" | tail -5; else echo "(first commit of the shared repo)"; fi
[ "$dry" = 1 ] && { echo "dry run: nothing committed or pushed"; exit 0; }

name=$(git config user.name); email=$(git config user.email)
parent=(); [ -n "$tip" ] && parent=(-p "$tip")
commit=$(GIT_AUTHOR_NAME="$name" GIT_AUTHOR_EMAIL="$email" GIT_COMMITTER_NAME="$name" GIT_COMMITTER_EMAIL="$email" \
  git commit-tree "$tree" "${parent[@]}" -F "$work/msg")
CLEAN_PUBLISH=1 git push clean "$commit:refs/heads/main"
git fetch -q clean
{ echo "last_clean=$commit"; echo "last_private=$(git rev-parse "$ref")"; } > "$STATE"
echo "published $(git rev-parse --short "$commit") (from $ref @ $(git rev-parse --short "$ref"))"
