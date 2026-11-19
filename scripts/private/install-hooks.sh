#!/usr/bin/env bash
# install-hooks.sh   (PRIVATE — never published)
# Installs the pre-push guard for the shared repo into the common .git (all
# worktrees), with a copy of the scan lists so worktrees on older branches,
# which lack scripts/private, are guarded too. Re-run after editing the lists.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
common=$(git -C "$here" rev-parse --path-format=absolute --git-common-dir)
mkdir -p "$common/clean-guard"
cp "$here/forbidden.txt" "$here/private-paths.txt" "$here/allow.txt" "$common/clean-guard/"
cp "$here/pre-push-hook.sh" "$common/hooks/pre-push"
chmod +x "$common/hooks/pre-push" 2>/dev/null || true
# `git config` cannot write .git/config on the Windows mount (chmod), so append it.
cfg="$common/config"
grep -q '^\[remote "clean"\]' "$cfg" || printf '[remote "clean"]\n\turl = git@github.com:MErenTalan/new-ekokod-clean.git\n\tfetch = +refs/heads/*:refs/remotes/clean/*\n\tpush = refs/heads/__never__:refs/heads/__never__\n' >> "$cfg"
echo "installed: $common/hooks/pre-push (+ clean-guard lists), remote clean"
