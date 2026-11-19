#!/usr/bin/env bash
# Installed as .git/hooks/pre-push by install-hooks.sh (PRIVATE — never published).
# Blocks every push to the shared repo except the ones clean-publish.sh makes,
# and re-checks what those push: no private path in the tree, no forbidden
# text in any commit message.
remote_url=$2
case "$remote_url" in *new-ekokod-clean*) ;; *) exit 0;; esac
if [ "${CLEAN_PUBLISH:-}" != 1 ]; then
  echo "pre-push: pushes to new-ekokod-clean go only through scripts/private/clean-publish.sh" >&2
  exit 1
fi
here=$(git rev-parse --show-toplevel)/scripts/private
[ -f "$here/forbidden.txt" ] || here=$(git rev-parse --git-common-dir)/clean-guard
pat=$(grep -vE '^\s*(#|$)' "$here/forbidden.txt" | paste -sd'|')
paths=$(grep -vE '^\s*(#|$)' "$here/private-paths.txt" | sed 's#[.]#\\.#g; s#^#^#; s#$#(/|$)#' | paste -sd'|')
zero=0000000000000000000000000000000000000000
while read -r _lref lsha _rref rsha; do
  [ "$lsha" = "$zero" ] && { echo "pre-push: deleting refs on the shared repo is not allowed" >&2; exit 1; }
  range=$lsha; [ "$rsha" != "$zero" ] && range="$rsha..$lsha"
  for c in $(git rev-list "$range"); do
    if git log -1 --format=%B "$c" | grep -qiE -- "$pat"; then
      echo "pre-push: commit $c has forbidden text in its message" >&2; exit 1
    fi
    if git ls-tree -r --name-only "$c" | grep -qE -- "$paths"; then
      echo "pre-push: commit $c contains a private path" >&2; exit 1
    fi
  done
done
exit 0
