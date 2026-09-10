# Handoff — ekokod rewrite, phase F0

Written 2026-09-10 (end of session 4) because the controller's context filled up. Everything below
is current as of commit `3869485` on branch `phase/f0-foundation`. **Working tree is clean.**

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon
> `docs/rewrite/` içinde, faz planı `docs/rewrite/09-implementation-plan.md`. Şu an **Faz F0**
> yürütülüyor: görev planı `docs/superpowers/plans/2026-09-08-f0-foundation.md`, ilerleme kaydı
> `.superpowers/sdd/2026-09-08-f0-foundation/progress.md` (ledger).
>
> Önce `HANDOFF_NEXT_SESSION.md` dosyasını oku, sonra ledger'ın sonunu oku, sonra kaldığın yerden
> devam et. Yürütme yöntemi `superpowers:subagent-driven-development`: her görev için taze bir
> implementer subagent, ardından task review, gerekirse fix turu. Ben plan + paralel subagent
> yöntemini onaylıyorum, devam et.
>
> Kaldığın yer: **Task 10 kodu bitti ve controller tarafından bağımsız doğrulandı (`3869485`), ama
> REVIEW HENÜZ DISPATCH EDİLMEDİ.** Önce Task 10 review'ünü gönder — aşağıdaki "Task 10 — review
> dispatch notes" bölümündeki spesifik soruları promptuna mutlaka koy. Sonra Task 11'e geç.

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
| 9 — api/worker/scheduler/seed processes | ✅ complete, review clean after **2** fix rounds |
| 10 — import-boundary test + golangci-lint | 🟡 **code done + controller-verified, REVIEW NOT DISPATCHED** |
| 11 — Dockerfile, Compose, offline bundle | ⬜ not started |
| 12 — Next.js shell + i18n + readiness page | ⬜ not started |
| 13 — CI | ⬜ not started |

Commits added this session:

```
3869485 feat(f0): enforce the dependency rule with an import-boundary test and depguard
08bc088 fix(f0): make config.redactDSN fail closed on every DSN shape
442afaa fix(f0): worker liveness/health-check, shutdown-timeout split, scheduler DeadlineExceeded, DSN-parser removal
55c7a50 feat(f0): api, worker, scheduler and seed processes with graceful shutdown
```

### Verified green at `3869485` (run by the controller personally, not taken from a report)

```
gofmt -l ./cmd ./internal        -> empty
go vet ./...                     -> clean
go vet -tags=integration ./...   -> clean
go build ./...                   -> clean
go test ./... -race -count=1     -> 14 packages ok, 0 failures
make lint                        -> "0 issues.", exit 0
make vuln                        -> no vulnerabilities
```

**`make lint` is finally a REAL gate.** It was a vacuous no-op (a `.PHONY` target with no recipe)
for this entire phase. The controller proved it can fail: injecting an unchecked `fmt.Fprintln` into
`internal/cli/version.go` produced
`internal/cli/version.go:17:16: Error return value of fmt.Fprintln is not checked (errcheck)` and
`make: *** [Makefile:38: lint] Error 1` (exit 2). File restored, tree verified clean.

---

## Immediate next step — dispatch the Task 10 review

Task 10's code is committed and the controller verified the two hard requirements personally, but
**no reviewer has looked at it.** Do that first. Diff to review: `08bc088..3869485` (18 files,
+377/-46).

### Task 10 — review dispatch notes

The unusual thing about this diff is that **fixing 40 pre-existing golangci-lint findings changed
code belonging to tasks 1–9, which were already reviewed and closed.** That is the review's main
risk surface. The controller read every hunk and found no behaviour regression, but a second pair of
eyes is exactly the point. Put these in the reviewer prompt:

1. **Did any lint fix change behaviour in already-closed code?** Specifically audit:
   `cmd/ekokod/main.go` (`os.Exit` → `run() int`), `internal/cli/api.go`
   (`net.Listen` → `(&net.ListenConfig{}).Listen(ctx, ...)`), `internal/store/postgres/migrate.go`
   (`defer db.Close()` → `defer func(){ _ = db.Close() }()`), and `internal/cli/configcheck.go` /
   `seed.go` (Fprintf return values now checked). The controller's readings, to be confirmed or
   refuted: the `main.go` change **fixes a real bug** (the old shape had `defer stop()` in the same
   scope as `os.Exit(1)`, and `os.Exit` runs no deferred calls, so the signal handler was never
   unregistered on the error path); and the `ListenConfig.Listen` ctx governs only address
   resolution and does **not** affect the returned Listener, so cancelling `cmd.Context()` cannot
   close the listener out from under the server.
2. **Were any test assertions weakened?** ~40 lines of test files changed
   (`httptest.NewRequest` → `NewRequestWithContext`). The controller read them and found no
   assertion removed or loosened — have the reviewer confirm independently.
3. **Is the `.golangci.yml` honest?** Check that no linter was disabled, no broad exclusion added,
   and that the `errcheck` `exclude-functions` entry is narrow. A config that reaches "0 issues" by
   not looking is the failure mode here.
4. **Are the arch guards real or tautological?** Six tests in `internal/arch/arch_test.go`. The
   controller already proved two of them live (the domain-import guard and the logger guard). Ask
   about the other four — particularly whether `TestOnlyTheCLIImportsTheAPIPackage`'s allow-list is
   so permissive that it would not catch a new violator, and whether the self-exemption the
   implementer added (`runtime.Caller(0)`, because the test file contains the literal strings it
   searches for) is narrow enough that a real offender in another file still trips it.
5. **Is the logger guard's exemption narrow?** It exempts `internal/platform/logging/logging.go` by
   exact path. Confirm that is the only legitimate constructor site and that the exemption cannot be
   widened accidentally.

Scale: sonnet is fine. This is a mechanical diff, not a concurrency one.

---

## Then Task 11 — and what it inherits

Task 11 (Dockerfile, Compose, offline bundle) carries four obligations from earlier tasks.

1. **Docker must be running.** See the environment section — it is currently DOWN.
2. **Run the deferred integration verification.** Every integration test written so far passed at
   `fdb119f`, but nothing has been run since — tasks 9 and 10 changed code, so re-run
   `go test ./... -tags=integration -race -count=1` once Docker is up. In particular `internal/job`'s
   integration test now exercises a changed `job.NewServer` signature.
3. **Exercise what Task 9 could not.** `api` and `worker` graceful shutdown against live
   Postgres/Redis/asynq was never verified by execution because Docker was down. Task 11's compose
   stack is the first place that can. Also add the **enabled-scheduler path** of `ekokod scheduler` —
   the Task 9 reviewer named it the highest-risk untested path in that diff.
4. **`terminationGracePeriodSeconds` / `stop_grace_period` must be ≥ 30 s.** Task 9 ruled that the
   worker's drain window is `min(EKOKOD_JOB_TIMEOUT, 30s)` via `job.MaxShutdownGrace`. Task 11's
   manifests must agree with that number or the drain gets SIGKILLed anyway.
5. **Deployment invariant:** if a transaction-pooling proxy (PgBouncer `pool_mode = transaction`) is
   ever placed between the app and Postgres, **session-level advisory locks are unsound and leader
   election breaks silently.** No pooler exists in the repo today.

Pre-flight for Tasks 11–13 has not been done. **Do it before dispatching** — verifying interfaces
against the real tree caught a guaranteed compile error in Task 9's brief and saved a round trip.

---

## Environment — read before running anything

- **Toolchain lives in `$HOME/.local`, and shell state does not persist between tool calls.**
  Prefix every command that needs Go or Node with:

  ```bash
  export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"
  ```

  Installed: Go 1.27.1, Node 24.20.0 LTS, pnpm 12.3.4. `make tools` has installed
  golangci-lint v2.13.2 and govulncheck v1.8.0 into `$HOME/go/bin`.

- **Docker Desktop is DOWN** — it stops between sessions, so ALWAYS verify rather than trusting this
  line. Symptoms when down: `docker info` prints "The command 'docker' could not be found in this
  WSL 2 distro", `/var/run/docker.sock` is absent, `/mnt/wsl/` holds only `resolv.conf`, and
  `tasklist.exe` shows no Docker process. It must be started from Windows — ask the user to run:

  ```
  ! "/mnt/c/Program Files/Docker/Docker/Docker Desktop.exe" &
  ```

  When up (server 28.3.2) these images are already pulled: `timescale/timescaledb:2.30.0-pg16`,
  `redis:7.4.11-alpine`. Tasks 11 and 13 will also want `golang:1.27.1-alpine` and `alpine:3.21`.

- The repo lives on `/mnt/c` (Windows drive under WSL), so Go builds, `golangci-lint` and
  `pnpm install` are slower than native. That is expected — be patient with timeouts rather than
  assuming a hang. A full `golangci-lint run` takes minutes on the first pass.

---

## Decisions already made — do not re-litigate

| Decision | Value |
|---|---|
| Go module path | `github.com/MErenTalan/ekokod-rewrite` |
| Binary name | `ekokod` |
| Env var prefix | `EKOKOD_` (spec appendix documents `BCEM_`; every name keeps its spelling with the prefix swapped) |
| Branch | `phase/f0-foundation`, no separate git worktree |
| Execution method | `superpowers:subagent-driven-development` — user approved plan + parallel subagents |
| Versions | Go 1.27.1 · chi v5.3.2 · pgx/v5 v5.11.0 · goose v3.28.0 · asynq v0.26.0 · testcontainers v0.44.0 · x/tools v0.50.0 · golangci-lint v2.13.2 · govulncheck v1.8.0 · PG16 + TimescaleDB 2.30 · Redis 7.4 · Next 15.5.25 · React 19.2.8 · TS 5.9.3 · Tailwind 4.3.3 · next-intl 4.14.2 |
| Commit trailers | `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>` and the session's own `Claude-Session:` URL |

### Standing ruling used repeatedly

**When the plan's prose or sample code disagrees with the plan's stated requirements or its own
tests, the requirements and tests win.** This has now held **seven times** this phase, and **three
times the plan's own sample code contained a live defect** (see "What the reviews caught", items
5, 6, 7). Treat the brief's sample code as fallible, not authoritative.

### Process notes that repeatedly paid off

- **Verify interfaces against the real tree before dispatching.** Task 9's brief called
  `postgres.MigrationsCheck(cfg.DB.URL)` when the real signature takes a `*pgxpool.Pool` — a
  guaranteed compile error caught in the dispatch. Do the same for Tasks 11–13.
- **Do not trust a report; re-run the gate yourself.** Every "verified green" line in this handoff
  was produced by the controller, not copied from a subagent.
- **Prove a guard can fail.** Both of Task 10's hard requirements were confirmed by deliberately
  breaking the code and watching the guard fire, then restoring. A gate never observed failing is
  not a gate.
- **Ask the reviewer a specific question rather than "verify the fix."** Every round that produced a
  real finding this session did so from a pointed question.
- **Scale the reviewer model to the risk.** Task 8 (concurrency) and Task 9 (process lifecycle) went
  to Opus and both found real defects; mechanical diffs went to Sonnet and were fine. Task 12
  (Next.js) and Task 11 (Docker/Compose) are integration-heavy — Sonnet minimum.

---

## Session 4's rulings (full text in the ledger; these are the ones a reviewer might re-open)

1. **Docker down → Task 9 accepted without live-dependency exercise.** Its brief defines no
   integration test, `go vet -tags=integration` is clean, and it touches no package that owns one.
   Carried to Task 11.
2. **Task 9 owns the four carried-forward requirements**, all now satisfied and pinned.
3. **`worker` must be able to learn its broker is dead** (asynq surfaces nothing after `Start`
   succeeds). Wired `HealthCheckFunc`; exits non-zero after 4 consecutive failures (~60 s). Threshold
   is a **named constant, deliberately not a new env var** — a config field would drag in
   `.env.example`, `Resolved` rows and the env-reference appendix, which is Task 2's surface.
4. **`EKOKOD_JOB_TIMEOUT` was doing two unrelated jobs** (max task runtime vs max process exit time).
   Split: asynq's `ShutdownTimeout` is now `min(cfg.Worker.Timeout, job.MaxShutdownGrace=30s)`.
5. **`isCleanShutdown` narrowed to `context.Canceled` only.** `net.timeoutError.Is` returns true for
   `context.DeadlineExceeded`, so accepting it would let a real scheduler failure exit 0 once the
   already-ledgered `%s`→`%w` fix lands on `postgres/scrub.go`.
6. **`config.redactDSN` failed OPEN — Critical, fixed.** Both early exits were a bare `return raw`,
   and `dsnDisplay` prefixes the mask glyph unconditionally, so the password printed in clear while
   *looking* redacted. Reproduced by the controller across five DSN shapes; four leaked. Now fails
   closed on every path, and strips `password`/`sslpassword` query parameters too.
7. **Fixing Task 2's code from Task 9 was authorised** (precedent: the Task 6 ruling that authorised
   editing Task 5 for the secret extraction), because Task 9's own fix newly wired the same exposure
   into a second command.
8. **No third re-review for Task 9 fix round 2**, because the controller reproduced the Critical
   end-to-end with the real binary across all five DSN shapes and read the full diff.
9. **Task 10 fixed all 40 lint findings rather than suppressing any.** Silencing a linter to get a
   green build was explicitly forbidden in the dispatch.

---

## Deferred minor findings — the final whole-branch review must triage these

Nothing here blocks a task, but several go live in later phases.

**Task 1**
- Text-mode version test asserts only the `ekokod` substring (the brief's own test).

**Task 2**
- `External.ISolarRedirect` has no derived default (env-reference says "derived"); belongs to F2.
- The 720h "remember me" refresh-token variant is not modelled in config; belongs to F6.
- **`loader.duration` does not validate positivity.** It accepts `"0s"`, and `time.ParseDuration`
  accepts negatives, so `EKOKOD_JOB_TIMEOUT=0s` or `-5m` yields `ShutdownTimeout <= 0` and asynq's
  abort timer fires immediately — in-flight tasks killed with no drain at all. **Not** introduced by
  Task 9's `min()` clamp; the old code passed the value straight through with the same result.

**Tasks 3+4**
- logging's secret-substring list is unexported and not extendable by callers.
- **logging's redaction is KEY-BASED ONLY.** `redactAttr` tests `isSecretKey(a.Key)` and never scans
  attribute *values*. This is a phase-wide invariant worth remembering: every
  `slog.String("error", err.Error())` site depends entirely on the error having been scrubbed **at
  construction**. Audited sites are currently safe.

**Task 5**
- The down migration drops timescaledb/pgcrypto/citext unconditionally, so it would remove an
  extension that existed before the migration ran. Safe under the fresh-container assumption; F1
  owns the real schema and should revisit.
- `internal/store/postgres/health.go` — the non-`isUndefinedTable` branch of
  `checkMigrationsCurrent` returns `fmt.Errorf("read schema version: %w", err)` **unscrubbed**,
  unlike every other error path. Practically inert, but inconsistent.

**Task 6**
- `scrubErr` builds its error with `%s` rather than `%w`, so `errors.Is`/`As` cannot see through a
  scrubbed error. **Fixing this activates Task 9's ruling 5** — do them together.
- `internal/job/scrub.go` and `internal/store/redis/scrub.go` still hold byte-identical
  `scrubParseErr` bodies — a candidate to fold into `secret`.

**Task 7**
- `ratelimit.go` reaps stale buckets only every 10 minutes, so many distinct source addresses within
  one window (trivial with an IPv6 /64) grow the bucket map before any reap. Brief-specified design.
- `ratelimit.go`'s `Retry-After: 60` is hardcoded and unrelated to the configured window.

**Task 8** (six, all from the Opus review)
- `elector.go:54` — `time.NewTicker(e.retry)` panics on `retry <= 0` in an exported path, while
  `monitor` guards the identical value. `NewElector(pool, key, 0, log)` panics.
- `elector_integration_test.go:157` — `require.Eventually(!IsLeader(), 5s, 50ms)` is timing-sensitive
  **in the wrong direction**: a *faster* machine makes it more likely to fail. Deterministic form is
  `require.False` immediately after `<-cancelled`. 3/3 PASS today, so no live flake.
- `scheduler.go:69-80` — `asynq.NewScheduler` allocates a go-redis client that the error returns
  never shut down; `elector.Run` swallows the error and re-campaigns every 10 s, leaking one client
  per 10 s. Unreachable today; goes live when later phases register config-driven entries.
- `elector.go:139` — discards `pg_advisory_unlock`'s boolean. Session advisory locks are counted, so
  a failed unlock could leave the lock held while `IsLeader()` is false.
- `elector.go:148-150` — `recover()` around `lead`'s panic keeps the process alive, so a
  deterministic panic repeats every 10 s forever, logged without a stack.
- Inherent: `IsLeader()`/`leadCtx` can lag reality by up to ~4 s. Inherent to advisory-lock election
  without fencing tokens.
- **Carried forward:** the phase that introduces the first real scheduled job must wire
  `Scheduler.entries()` (it currently registers only the noop task) and add coverage.

**Task 9**
- `dsnDisplay`'s `q.Encode()` alphabetises and percent-encodes query parameters, so `config:check`'s
  DSN row may not render in the operator's original spelling. Display-only; verified it cannot
  affect a real connection (`requiredDSN` returns the raw value for connecting).
- `internal/cli/api.go`'s serve goroutine is not joined before `RunE` returns. Harmless (buffered
  `errCh`, one `errors.Is` then return) but a `WaitGroup` would be tidier.

**Task 10**
- The depguard `no-float-money` rule targets `internal/domain/billing` and `internal/domain/tariff`,
  which do not exist yet, so it is inert until F4. Intended.

---

## What the reviews have caught so far

The same two classes keep appearing. Keep the reviewer prompts pointed at them.

1. **Task 2, Critical** — `net/url.Parse`'s error embeds its whole input, so wrapping it with `%w`
   printed the database password in clear from `config:check`.
2. **Tasks 3+4, Critical** — the redacting slog handler never resolved `slog.LogValuer`, so a secret
   nested behind an innocuous key was logged in full.
3. **Tasks 3+4, Important** — `health.Run` blocked forever on a check that ignores its context.
4. **Task 5, Critical** — pgx splits DSN userinfo at the first `@` while `net/url` splits at the
   last, so a password containing an unescaped `@` leaked a fragment into a DNS error.
5. **Task 7, Critical (in the plan's own sample code)** — the brief's `cors.go` would have shipped
   allow-all-origins **with credentials** whenever `EKOKOD_CORS_ORIGINS` was unset.
6. **Task 7, Important (in the plan's own sample code)** — chi returns an empty route pattern for
   unmatched paths, so scanner traffic would mint unbounded Prometheus labels.
7. **Task 9, Important (in the plan's own sample code)** — `asynq.Server.Run` installs its own
   signal handler, racing `signal.NotifyContext`; whichever `Shutdown` arrived second returned as a
   **no-op**, so `RunE` could return nil while asynq was still draining, killing in-flight tasks.
8. **Task 9, Critical (in Task 2's shipped code)** — `config.redactDSN` failed open, printing the
   database password in clear behind a mask glyph for every DSN shape whose credential is not in URL
   userinfo. Reachable via `ekokod config:check`.
9. **Task 9, Important** — `worker` could never learn its broker had died; it stayed alive forever
   with the exit code never set and no probe surface.

**Every one was a secret-handling or liveness defect that the plan's own tests did not cover, and
three were defects in the plan's own sample code.**

---

## Open questions for the product owner (block F4, not F0)

`docs/rewrite/02-domain-rules.md` §11 lists five questions that must be answered before the billing
engine is written. They do not block F0–F3, but the answers take research, so raise them early.

1. **Reactive penalty base.** Once the ratio limit is exceeded, is the *entire* reactive quantity
   charged (legacy behaviour) or only the excess above the limit?
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
  progress.md                                          THE LEDGER — read the tail first
  task-N-brief.md                                      per-task briefs, already generated for 1–13
  task-N-report.md                                     implementer reports
  task-9-review.md, task-9-rereview.md                 task 9's review artefacts
  review-<base>..<head>.diff                           review packages
```

SDD helper scripts:
`/home/personal/.claude/plugins/cache/claude-plugins-official/superpowers/6.3.0/skills/subagent-driven-development/scripts/`
(`task-brief`, `review-package`, `sdd-workspace`).
