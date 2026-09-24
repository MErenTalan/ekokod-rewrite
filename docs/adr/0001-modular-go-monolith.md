# ADR 0001 — A modular Go monolith with one binary

Status: accepted

## Context

The legacy system (bcem-energy) was a PHP/Node mix with logic spread across controllers, cron scripts and the browser. The rewrite had one small team, one week of calendar time per few phases, and customers who install on-premises, sometimes air-gapped (01, 03).

## Decision

One Go module and one binary, `ekokod`, whose subcommands are the processes: `api`, `worker`, `scheduler`, `migrate`, `seed`, `recompute`, `tool`. The code is layered `domain` (pure rules, no I/O) → `service` (use cases) → `store` (repositories) → `api`/`worker`. `internal/arch` tests enforce the layering: domain imports no project package and no I/O, services see repositories only through interfaces, and money never touches `float64`.

## Consequences

One image serves every role, so an install is `docker compose` plus one tarball. Boundaries are compile-time and test-enforced rather than network hops. Scaling is per process role (more workers), not per module. A module that later needs to become its own service already has an interface seam.
