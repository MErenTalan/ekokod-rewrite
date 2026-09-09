# Handoff — ekokod rewrite, phase F0

Written 2026-09-09 (session 3) because the account API session limit was hit (resets **21:00
Europe/Istanbul**). Everything below is current as of commit `fdb119f` on branch
`phase/f0-foundation`.

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
> onaylıyorum, devam et.
>
> Kaldığın yer: **Task 9 hiç başlamadı** — implementer rate limit'e takılıp hiçbir dosya yazmadan
> düştü, ağaç temiz. Task 9'u sıfırdan dispatch et. Task 9 briefine ek olarak taşınan **dört zorunlu
> gereksinim** aşağıdaki "Task 9 — carried-forward requirements" bölümünde; bunları dispatch
> promptuna mutlaka koy.

---

## Where the work stands

Branch `phase/f0-foundation` (base `020330d` on `main`). `main` is untouched.

| Task | State |
|---|---|
| 1 — repo skeleton, Go module, `version` | ✅ complete, review clean |
| 2 — typed config + `config:check` | ✅ complete, review clean after 1 fix round |
| 3 — errors + redacting logger | ✅ complete, review clean after 1 fix round |
| 4 — clock, uuidv7, AES-256-GCM, health registry | ✅ complete, review clean after 1 fix round |
| 5 — postgres pool, embedded migrations, `migrate` | ✅ complete, review clean after 1 fix round |
| 6 — redis + asynq `noop` round trip | ✅ complete, review clean after 1 fix round |
| 7 — chi router, middleware, health/version/metrics | ✅ complete, review clean after 1 fix round |
| 8 — scheduler leader election | ✅ complete, review clean after 1 fix round |
| 9 — api/worker/scheduler/seed processes | ⬜ **NOT STARTED** — dispatch failed, no files written |
| 10 — import-boundary test + golangci-lint | ⬜ not started |
| 11 — Dockerfile, Compose, offline bundle | ⬜ not started |
| 12 — Next.js shell + i18n + readiness page | ⬜ not started |
| 13 — CI | ⬜ not started |

Commits on the branch:

```
fdb119f test(f0): pin fresh-asynq.Scheduler-per-term invariant in the scheduler
c1dddf0 feat(f0): scheduler with postgres advisory-lock leader election
5e7c38f fix(f0): bound metrics route-label cardinality on unmatched paths
1aeb94b feat(f0): chi router with request-id, logging, recovery, cors, rate limit and metrics
ea294bd refactor(f0): extract shared password-redaction primitives to internal/platform/secret
5491645 feat(f0): redis client and asynq queue with a noop round-trip task
471ec1a fix(f0): add dsn-scrub regression test, verify postgres integration suite
c6da3f1 docs: add next-session handoff for phase F0
8443cf8 fix(f0): scrub dsn secrets from postgres errors, serialise goose setup
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

### Verified green at `fdb119f` (run by the controller immediately before this handoff)

```
gofmt -l ./cmd ./internal        -> empty
go vet ./...                     -> clean
go vet -tags=integration ./...   -> clean
go build ./...                   -> clean
go test ./... -race -count=1     -> 16 packages ok, 0 failures
```

**All integration tests have now been RUN and pass.** The backlog of unrun Docker tests that the
previous handoff carried is cleared: postgres (5 tests), redis + job round trip, and scheduler
(4 tests, green at `-race -count=3`).

---

## Immediate next step — Task 9

Dispatch a fresh implementer with `.superpowers/sdd/2026-09-08-f0-foundation/task-9-brief.md`.
Nothing was written by the failed attempt; start clean.

### Task 9 — carried-forward requirements (DO NOT DROP THESE)

Four earlier reviewers flagged items as living outside their diffs and belonging to Task 9. The
controller ruled each one Task 9's. They are **not** in the brief — they must be put in the dispatch
prompt explicitly, and the implementer must report on each:

1. **`api.Deps.Log` must be the redacting logger** from `logging.New`, not a bare
   `slog.NewJSONHandler`. That redaction is the only thing keeping secrets out of the logs.
2. **Set `http.Server.MaxHeaderBytes`.** The `X-Request-Id` middleware echoes a client-supplied
   header; unbounded, a client can send an enormous one.
3. **`scheduler.Run` returns `context.Canceled` on a NORMAL shutdown**, not `nil` — it returns the
   elector's `ctx.Err()`. The call site must treat that as a clean exit, not a crash or non-zero exit.
4. **`config.Scheduler.Enabled` gating is deliberately left to the caller** (documented at
   `internal/scheduler/scheduler.go:26-28`). Honour it, or the scheduler runs on every replica role.

### Care areas worth putting in the Task 9 dispatch

- Graceful shutdown must actually terminate within 30 s: in-flight HTTP drained not cut, asynq stops
  consuming before it waits, a second signal must not deadlock, no goroutine outlives the command.
- Resource cleanup on **every** exit path including startup failure — a leaked pgx pool holding the
  scheduler advisory lock blocks every other replica.
- Startup errors must not print secrets; route through `internal/platform/secret`.
- `seed` writes to a database: it must not silently run against the wrong environment, and must be
  idempotent or clearly refuse to run twice.

---

## Environment — read before running anything

- **Toolchain lives in `$HOME/.local`, and shell state does not persist between tool calls.**
  Prefix every command that needs Go or Node with:

  ```bash
  export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"
  ```

  Installed: Go 1.27.1, Node 24.20.0 LTS, pnpm 12.3.4.

- **Docker Desktop is DOWN as of session 4** — it stops between sessions, so ALWAYS verify rather
  than trusting this line. Symptoms when down: `docker info` prints "The command 'docker' could not
  be found in this WSL 2 distro", `/var/run/docker.sock` is absent, `/mnt/wsl/` holds only
  `resolv.conf`, and `tasklist.exe` shows no Docker process. It has to be started from Windows.
  When up (server 28.3.2) these images are already pulled: `timescale/timescaledb:2.30.0-pg16`,
  `redis:7.4.11-alpine`; tasks 11 and 13 will also want `golang:1.27.1-alpine` and `alpine:3.21`.

- The repo lives on `/mnt/c` (Windows drive under WSL), so Go builds and `pnpm install` are slower
  than native. That is expected — be patient with timeouts rather than assuming a hang.

- **`make lint` is currently a no-op** — the target is declared `.PHONY` with no recipe body, so
  `make lint test build` has been passing vacuously on the lint leg all phase. **Task 10 owns
  defining it. When Task 10 closes, verify the gate is real** (i.e. that `make lint` can actually
  fail) before marking it complete.

---

## Decisions already made — do not re-litigate

| Decision | Value |
|---|---|
| Go module path | `github.com/MErenTalan/ekokod-rewrite` |
| Binary name | `ekokod` |
| Env var prefix | `EKOKOD_` (spec appendix documents `BCEM_`; every name keeps its spelling with the prefix swapped) |
| Branch | `phase/f0-foundation`, no separate git worktree |
| Execution method | `superpowers:subagent-driven-development` — user approved plan + parallel subagents |
| Versions | Go 1.27.1 · chi v5.3.2 · pgx/v5 v5.11.0 · goose v3.28.0 · asynq v0.26.0 · testcontainers v0.44.0 · PG16 + TimescaleDB 2.30 · Redis 7.4 · Next 15.5.25 · React 19.2.8 · TS 5.9.3 · Tailwind 4.3.3 · next-intl 4.14.2 |
| Commit trailers | `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>` and the session's own `Claude-Session:` URL |

### Standing ruling used repeatedly

**When the plan's prose or sample code disagrees with the plan's stated requirements or its own
tests, the requirements and tests win.** This has now held **six times** this phase — and twice the
plan's own sample code contained a live security defect (see "What the reviews caught", items 5 and
6). Treat the brief's sample code as fallible, not authoritative.

### Every ruling made in session 3 (with cost if wrong)

Full text is in the ledger; these are the ones a reviewer might otherwise re-open.

1. **Docker returned → integration tests run per-task, not batched at the end.** Cost: a few extra
   minutes of container startup per task.
2. **Task 6 concern 1 — scrubbing the Redis URL out of parse errors, deviating from the brief's
   literal `%w` wrapping.** Accepted; closes the same class the task-2 and task-5 reviews each
   flagged Critical. Cost: an error message carries less detail than the plan drafted.
3. **Task 6 concern 2 — extra `internal/job/noop_test.go` beyond the brief's file list.** Accepted;
   the brief's own integration test registers an inline handler and never exercises `job.Register` /
   `Handlers.Noop`, both Produces-listed. Cost: one small test file more than the plan listed.
4. **Task 6 concern 3 — go-redis's internal stderr logger audited (no leaks) but not silenced.**
   Accepted as-is. Cost: a future go-redis version could log a URL; lint task and final review are
   the backstop.
5. **Task 6 — the duplication finding is load-bearing, extract to `internal/platform/secret`.**
   Authorised editing completed Task 5 code because the extraction is behaviour-preserving and Task
   5's regression test plus the postgres integration suite gate it. Cost: a behaviour change in
   postgres scrubbing, caught by that existing test.
6. **Task 7 — CORS: the brief's own sample code was a live security defect.** Verified against
   vendored `go-chi/cors@v1.2.2` (`cors.go:131-134`): an empty `AllowedOrigins` with no
   `AllowOriginFunc` sets `allowedOriginsAll = true`, and a `"*"` entry does the same — so with
   `AllowCredentials: true`, a default or wildcard `CORSOrigins` yields allow-all-with-credentials.
   The implementer's `AllowOriginFunc` allow-list is accepted. Cost: CORS differs from the plan's
   draft only in the restrictive direction.
7. **Task 7 — metrics cardinality finding enters the fix loop even though the brief mandates the
   defective line.** It is an unauthenticated remote memory-exhaustion vector. Cost: unmatched
   requests aggregate under one label, which is the intended behaviour anyway.
8. **Task 8 — `Scheduler.entries()` wiring none of `config.Schedule`'s eight cron fields is accepted
   for F0.** No job types exist yet; the fields are already validated by task 2's config, so nothing
   is silently dead. **Carried forward: the phase that introduces the first real scheduled job must
   wire `entries()` and add coverage.** Cost: an empty cron table in F0, which is the intended state.
9. **Task 8 — the Important coverage finding entered the fix loop despite an overall "Approved"
   verdict**, because an Important finding triggers the loop regardless. Cost: one extra fix round.
10. **Reviewer ⚠️ "cannot verify" items resolved by the controller each time** rather than left
    hanging — Task 7's three and Task 8's three. The two live ones became Task 9 requirements above.

---

## Deferred minor findings — the final whole-branch review must triage these

Point the final reviewer at this list. Nothing here blocks a task, but several go live in later
phases.

**Task 1**
- Text-mode version test asserts only the `ekokod` substring (the brief's own test).
- `Makefile` `.PHONY` lists `lint` before task 10 defines the target — see the `make lint` no-op note.

**Task 2**
- `External.ISolarRedirect` has no derived default (env-reference says "derived"); belongs to the
  iSolar integration in F2.
- The 720h "remember me" refresh-token variant is not modelled in config; belongs to F6.

**Tasks 3+4**
- logging's secret-substring list is unexported and not extendable by callers.

**Task 5**
- The down migration drops timescaledb/pgcrypto/citext unconditionally, so it would remove an
  extension that existed before the migration ran. Safe under the fresh-container assumption; F1
  owns the real schema and should revisit.
- `internal/store/postgres/health.go:52` — the non-`isUndefinedTable` branch of
  `checkMigrationsCurrent` returns `fmt.Errorf("read schema version: %w", err)` **unscrubbed**,
  unlike every other error path. Practically inert (runs only against an already-pinged pool whose
  host resolved), but inconsistent with the established pattern.

**Task 6**
- `scrubErr` builds its error with `%s` rather than `%w`, so `errors.Is`/`As` cannot see through a
  scrubbed error. No caller depends on the chain today.
- `internal/job/scrub.go` and `internal/store/redis/scrub.go` still hold byte-identical
  `scrubParseErr` bodies — outside the extraction's scope, but a candidate to fold into `secret`.

**Task 7**
- `ratelimit.go` reaps stale buckets only every 10 minutes, so many distinct source addresses within
  one window (trivial with an IPv6 /64) grow the bucket map before any reap. Brief-specified design.
- `ratelimit.go`'s `Retry-After: 60` is hardcoded and unrelated to the configured window
  (`RateLimitAuth` defaults to `5/15min`, so 60 understates the real wait). Matches the brief.
- The implementer's report claimed to cover each care area but never discussed request-ID
  sanitisation or metrics cardinality — the area where the Important defect then was.

**Task 8** (six, all from the Opus review)
- `elector.go:54` — `time.NewTicker(e.retry)` panics on `retry <= 0` in an exported path, while
  `monitor` guards the identical value at `:164-167`. `NewElector(pool, key, 0, log)` panics.
- `elector_integration_test.go:157` — `require.Eventually(!IsLeader(), 5s, 50ms)` is timing-sensitive
  **in the wrong direction**: after the ping fails `Run` re-campaigns immediately and can re-acquire
  before the first poll, so a *faster* machine makes it more likely to fail and no timeout increase
  can fix it. Deterministic form is `require.False` immediately after `<-cancelled` (what test 2
  already does at `:131`). 3/3 PASS today, so no live flake.
- `scheduler.go:69-80` — `asynq.NewScheduler` allocates a go-redis client that the error returns at
  `:75`/`:79` never shut down. `elector.Run` swallows the error and re-campaigns every 10s, so a
  persistent failure leaks one client per 10s. Unreachable today (constant `@every 1h`); goes live
  when later phases register config-driven entries.
- `elector.go:139` — discards `pg_advisory_unlock`'s boolean. Session advisory locks are counted; a
  failed unlock on a surviving, later re-acquired connection could reach counter 2, leaving the lock
  held while `IsLeader()` is false and blocking every replica until the session ends. Largely
  self-healing; scanning and logging `false` would surface it.
- `elector.go:148-150` — `recover()` around `lead`'s panic is beyond the brief and is **not** what
  makes the lock safe (the deferred unlock already runs while unwinding). Its only effect is keeping
  the process alive, so a deterministic panic repeats every 10s forever, logged without a stack.
- Inherent: `IsLeader()`/`leadCtx` can lag reality by up to ~4s. Postgres frees the lock the instant
  the session dies, so a follower may already be leading while this replica still reports true.
  Inherent to advisory-lock election without fencing tokens; F0 acceptance wording says "exactly one
  instance leads", so it is recorded deliberately.

**Deployment invariant (task 11 + final review)**
- If a transaction-pooling proxy (PgBouncer `pool_mode = transaction`) is ever placed between the app
  and Postgres, **session-level advisory locks are unsound and leader election breaks silently.** No
  pooler exists in the repo today.

---

## What the reviews have caught so far

The same two classes keep appearing. Keep the reviewer prompts pointed at them.

1. **Task 2, Critical** — `net/url.Parse`'s error embeds its whole input, so wrapping it with `%w`
   printed the database password in clear from `config:check`.
2. **Tasks 3+4, Critical** — the redacting slog handler never resolved `slog.LogValuer`, so a secret
   nested behind an innocuous key was logged in full.
3. **Tasks 3+4, Important** — `health.Run` blocked forever on a check that ignores its context, which
   would hang `/health/ready`.
4. **Task 5, Critical** — pgx splits DSN userinfo at the first `@` while `net/url` splits at the
   last, so a password containing an unescaped `@` leaked a fragment into a DNS error, onto stderr
   and into the `/health/ready` JSON body.
5. **Task 7, Critical (in the plan's own sample code)** — the brief's `cors.go` would have shipped
   allow-all-origins **with credentials** whenever `EKOKOD_CORS_ORIGINS` was unset.
6. **Task 7, Important (in the plan's own sample code)** — chi returns an empty route pattern for
   unmatched paths, so the metrics middleware used the raw URL path as a Prometheus label. Scanner
   traffic (`/wp-admin`, `/.env`, random strings) would each mint a permanent histogram label that
   never evicts — unbounded attacker-controlled memory growth, and the exact opposite of what that
   code's own doc comment claimed.

**Every one was a secret-handling or liveness defect that the plan's own tests did not cover, and
two were defects in the plan's own sample code.**

---

## Open questions for the product owner (block F4, not F0)

`docs/rewrite/02-domain-rules.md` §11 lists five questions that must be answered before the billing
engine is written. They do not block F0–F3, but the answers take research, so raise them early.

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
  task-N-report.md                                     implementer reports (1,2,3-4,5,6,7,8)
  review-<base>..<head>.diff                           review packages
```

SDD helper scripts:
`/home/personal/.claude/plugins/cache/claude-plugins-official/superpowers/6.3.0/skills/subagent-driven-development/scripts/`
(`task-brief`, `review-package`, `sdd-workspace`).

---

## Process notes that saved real time this session

- **Verify interfaces before dispatching.** The plan's pre-flight table said `buildinfo` lived at
  `internal/platform/buildinfo`; it is at `internal/buildinfo`. Catching that in the dispatch avoided
  a guaranteed compile error and a wasted round trip. Do the same check for Tasks 10–13.
- **Scale the reviewer model to the risk.** Task 8's leader election went to Opus and that review
  found the unpinned asynq-restart invariant plus six real Minors; the small 4 KB metrics fix went to
  Haiku and was fine. Task 12 (Next.js) and Task 11 (Docker/Compose) are integration-heavy — Sonnet
  minimum.
- **Ask the re-reviewer a specific question rather than "verify the fix".** Asking whether a
  regression test asserted the raw path was *absent* (not merely that the constant was *present*),
  and whether a **timeout** was credible RED evidence, both produced mechanistic answers traced
  through vendored library source instead of a rubber stamp.
- **Do not let an implementer grade its own completeness.** Task 6's implementer claimed
  `internal/job/scrub.go` had nothing to rewire; that claim was true, but it was verified against the
  code rather than accepted.
