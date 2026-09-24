# ADR 0008 — Delivery as an offline bundle for on-premises installs

Status: accepted

## Context

Customers install on their own servers, some without internet access. Upgrades must not lose data and must be reversible.

## Decision

`make offline-bundle` produces one tarball with every image (`docker save`), the compose file, `install.sh`, the backup/restore scripts and the docs. `install.sh` never pulls or builds. On an existing install it backs up first, then loads the new images and migrates. `scripts/test-offline-install.sh` proves fresh install → upgrade → no data loss. `scripts/test-restore.sh` proves the backup restores and serves.

## Consequences

Bundles are large (about 800 MB, mostly the ML and web images). A rollback is the previous bundle plus `restore.sh` from the pre-upgrade backup. Still to do: a run on a host with networking physically disabled.
