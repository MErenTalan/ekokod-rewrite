# Handoff — ekokod rewrite, phase F0

Written 2026-09-09 because the session hit its API rate limit mid-task. Everything below is
current as of commit `8443cf8` on branch `phase/f0-foundation`.

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon
> `docs/rewrite/` içinde, faz planı `docs/rewrite/09-implementation-plan.md`. Şu an **Faz F0**
> yürütülüyor: görev planı `docs/superpowers/plans/2026-09-08-f0-foundation.md`, ilerleme kaydı
> `.superpowers/sdd/2026-09-08-f0-foundation/progress.md` (ledger).
>
> Önce `HANDOFF_NEXT_SESSION.md` dosyasını oku, sonra ledger'ı oku, sonra kaldığın yerden devam et.
> Yürütme yöntemi `superpowers:subagent-driven-development`: her görev için taze bir implementer
> subagent, ardından task review, gerekirse fix turu. Ben plan + paralel subagent yöntemini
> onayladım, devam et.
>
> Kaldığın yer: **Task 5 fix round 1**, tek bir eksikle — aşağıdaki "Immediate next step".

---

## Where the work stands

Branch `phase/f0-foundation` (base `020330d` on `main`). `main` is untouched.

| Task | State |
|---|---|
| 1 — repo skeleton, Go module, `version` | ✅ complete, review clean |
| 2 — typed config + `config:check` | ✅ complete, review clean after 1 fix round |
| 3 — errors + redacting logger | ✅ complete, review clean after 1 fix round |
| 4 — clock, uuidv7, AES-256-GCM, health registry | ✅ complete, review clean after 1 fix round |
| 5 — postgres pool, embedded migrations, `migrate` | 🟡 fix round 1 interrupted — one test missing |
| 6 — redis + asynq `noop` round trip | ⬜ not started |
| 7 — chi router, middleware, health/version/metrics | ⬜ not started |
| 8 — scheduler leader election | ⬜ not started |
| 9 — api/worker/scheduler/seed processes | ⬜ not started |
| 10 — import-boundary test + golangci-lint | ⬜ not started |
| 11 — Dockerfile, Compose, offline bundle | ⬜ not started |
| 12 — Next.js shell + i18n + readiness page | ⬜ not started |
| 13 — CI | ⬜ not started |

Commits so far:

```
8443cf8 fix(f0): scrub dsn secrets from postgres errors, serialise goose setup   ← partial
8658fda feat(f0): postgres pool, embedded goose migrations and migrate command
0bd2b57 fix(f0): resolve LogValuer before redaction, bound health.Run by wall-clock timeout
a0e4666 feat(f0): clock, uuidv7 ids, aes-256-gcm cipher and health registry
2ad6923 feat(f0): application error type and redacting structured logger
69aba7c fix(f0): stop leaking DSN passwords, mark DSNs secret, complete .env.example
38437a7 feat(f0): typed configuration with validation and config:check
1d37dd1 docs(f0): apply pre-flight rulings to the foundation plan
9b3d024 feat(f0): scaffold go module, cli root and version command
020330d docs: add rewrite specification and the F0 foundation plan
```

The tree at `8443cf8` is green: `gofmt -l ./cmd ./internal` empty, `go vet ./...` clean,
`go vet -tags=integration ./...` clean, `go test ./...` all pass.

---

## Immediate next step

Task 5's fix round is one item short. Dispatch a fresh implementer (the previous one is gone) with
the task-5 brief and this single finding:

> Write the regression test `TestErrorsNeverContainTheDSNPassword` in
> `internal/store/postgres` (a normal unit test, no database needed). It must call `NewPool` with an
> unreachable host and a distinctive password **containing an unescaped `@`** — for example
> `postgres://ekokod_user:my@pass@127.0.0.1:59999/ekokod?sslmode=disable` — and assert the returned
> error text contains neither the whole password nor any fragment of it (`my`, `pass`). The package
> currently has no non-integration test file at all. `internal/store/postgres/scrub.go` already
> implements the scrubbing this test locks down.

Then run the scoped re-review over `8658fda..HEAD` against the six task-5 findings listed in the
ledger, and continue with Task 6.

---

## Environment — read before running anything

- **Toolchain lives in `$HOME/.local`, and shell state does not persist between tool calls.**
  Prefix every command that needs Go or Node with:

  ```bash
  export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"
  ```

  Installed: Go 1.27.1, Node 24.20.0 LTS, pnpm 12.3.4. `~/.bashrc` already exports this PATH, so
  interactive shells are fine; tool-invoked shells are not guaranteed to source it.

- **Docker Desktop is currently NOT running.** `/var/run/docker.sock` is absent and
  `/mnt/wsl/docker-desktop` is unmounted. Ask the user to start Docker Desktop (with WSL
  integration enabled for `Ubuntu-24.04` — they enabled it once already, so it should just work).
  Check with `ls -l /var/run/docker.sock`.

- **Integration tests not yet run** (all written, none executed — no Docker):
  - `internal/store/postgres`: `TestMigrateUpDownUp`, `TestTimescaleExtensionIsInstalled`,
    `TestPoolCheckReportsHealth`, `TestMigrationsCheckReportsPendingThenCurrent`,
    `TestStatementTimeoutIsApplied`
  - Tasks 6, 8 and 11 will add more.

  When Docker returns, run them as one batch:

  ```bash
  go test ./... -tags=integration -race -count=1 -timeout 20m
  ```

  Images to pre-pull: `timescale/timescaledb:2.30.0-pg16`, `redis:7.4.11-alpine`,
  `golang:1.27.1-alpine`, `alpine:3.21`.

- The repo lives on `/mnt/c` (Windows drive under WSL), so Go builds and `pnpm install` are slower
  than native. That is expected.

---

## Decisions already made — do not re-litigate

| Decision | Value |
|---|---|
| Go module path | `github.com/MErenTalan/ekokod-rewrite` |
| Binary name | `ekokod` |
| Env var prefix | `EKOKOD_` (the spec appendix documents `BCEM_`; every name keeps its spelling with the prefix swapped) |
| Branch | `phase/f0-foundation`, no separate git worktree |
| Execution method | `superpowers:subagent-driven-development` — user approved plan + parallel subagents |
| Versions | Go 1.27.1 · chi v5.3.2 · pgx/v5 v5.11.0 · goose v3.28.0 · asynq v0.26.0 · testcontainers v0.44.0 · PG16 + TimescaleDB 2.30 · Redis 7.4 · Next 15.5.25 · React 19.2.8 · TS 5.9.3 · Tailwind 4.3.3 · next-intl 4.14.2 |
| Commit trailers | `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>` and the session's own `Claude-Session:` URL |

Rulings recorded during execution (full text with cost-if-wrong in the ledger):

1. Work on a branch in place rather than a separate worktree.
2. Tasks 3 and 4 were dispatched as one batch to a single implementer.
3. When the plan's prose code and the plan's own tests disagree, **the tests win**. This has
   happened three times so far and each time the test was right.
4. `MigrationsCheck` takes `*pgxpool.Pool` and reads `goose_db_version` through it, instead of
   opening a fresh `sql.DB` per readiness poll. `PendingMigrations(ctx, dsn)` stays for CLI use.
5. Commits already carrying the trailer `Co-Authored-By: Claude Sonnet 5` are left alone; the exact
   required trailer is enforced going forward.
6. Docker-dependent integration tests are written but not run, marked NOT RUN, and batched for when
   Docker returns. No task is reported complete on unrun tests.
7. The interrupted task-5 fix round was committed rather than discarded, with the incompleteness
   stated in the commit message.

Four plan defects found by the pre-flight scan were fixed in the plan text itself (commit
`1d37dd1`): the `scheduler.New` signature, task 11 building the `web` service before task 12
creates it, `docker save` argument quoting in the offline bundle, and the missing
`@eslint/eslintrc` dependency plus the deprecated `next lint` script.

---

## What the reviews have caught so far

Worth knowing, because the same classes keep appearing and are worth watching for:

1. **Task 2, Critical** — `net/url.Parse`'s error embeds its whole input, so wrapping it with `%w`
   printed the database password in clear from `config:check`. Fixed.
2. **Tasks 3+4, Critical** — the redacting slog handler never resolved `slog.LogValuer`, so a secret
   nested behind an innocuous key was logged in full. Fixed.
3. **Tasks 3+4, Important** — `health.Run` blocked forever on a check that ignores its context,
   which would hang `/health/ready`. Fixed with a shared wall-clock deadline.
4. **Task 5, Critical** — pgx splits DSN userinfo at the first `@` while `net/url` splits at the
   last, so a password containing an unescaped `@` leaked a fragment into a DNS error, onto stderr
   and into the `/health/ready` JSON body. Fixed by `internal/store/postgres/scrub.go`; the
   regression test is the one item still open.

**Every one of these was a secret-handling or liveness defect that the plan's own tests did not
cover.** Keep the reviewer prompts pointed at those two areas.

---

## Open questions for the product owner (block F4, not F0)

`docs/rewrite/02-domain-rules.md` §11 lists five questions that must be answered before the billing
engine is written. They do not block F0–F3, but the answers take research, so they are worth
raising early:

1. **Reactive penalty base.** Once the ratio limit is exceeded, is the *entire* reactive quantity
   charged (legacy behaviour) or only the excess above the limit? This materially changes invoice
   amounts.
2. **Sub-9 kW exemption.** Confirm installations under 9 kW installed power are exempt from the
   reactive penalty, as the rewrite assumes.
3. **Tiered pricing user groups.** Legacy tiered only `residential` and `commercial`. Should
   `residentialPlus` and `commercialPlus` also be tiered, and at what daily kWh thresholds?
4. **Missing-hour tolerance.** Confirm 2 % as the threshold above which a PTF-priced invoice is
   withheld for operator review instead of issued.
5. **Grid emission factor.** Confirm the current official Türkiye value and its source year, and
   whether historical reports should be recomputed with period-appropriate factors or keep 0.45.

---

## Key paths

```
docs/rewrite/                                          the specification (11 documents + appendix)
docs/rewrite/09-implementation-plan.md                 phases F0–F15
docs/superpowers/plans/2026-09-08-f0-foundation.md     the F0 task plan being executed
.superpowers/sdd/2026-09-08-f0-foundation/             git-ignored workspace
  progress.md                                          THE LEDGER — read this first
  task-N-brief.md                                      per-task briefs, already generated for 1–13
  task-N-report.md                                     implementer reports
  review-<base>..<head>.diff                           review packages
```

The SDD helper scripts live at
`/home/personal/.claude/plugins/cache/claude-plugins-official/superpowers/6.3.0/skills/subagent-driven-development/scripts/`
(`task-brief`, `review-package`, `sdd-workspace`).
