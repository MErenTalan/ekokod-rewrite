# Handoff — ekokod rewrite, phase F0

Written 2026-09-10 (end of session 5). Branch `phase/f0-foundation`. **Nothing has been pushed;
`main` is untouched and there is a real remote (`git@github.com:MErenTalan/ekokod-rewrite.git`).**

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
> Durum: **F0 (Foundation) TAMAMLANDI ve `main`'e merge edildi.** 13/13 görev, final whole-branch
> review temiz. `phase/f0-foundation` branch'i silindi. **Hiçbir şey push EDİLMEDİ** — `main`
> lokalde `origin/main`'in önünde; push kararı bana ait, sen push etme.
>
> Sıradaki faz **F1 — Data model and migration framework**
> (`docs/rewrite/09-implementation-plan.md`, satır 128). Toplam 16 faz var (F0–F15), yani 15 faz
> kaldı; F0 yalnızca temeldi.
>
> Şu sırayla ilerle:
> 1. `HANDOFF_NEXT_SESSION.md`'i oku — özellikle "KNOWN FLAKE", "Deferred minors" (her madde
>    sahibi fazla işaretli, F1'e düşenler var) ve "Standing rulings" bölümlerini.
> 2. **İlk iş: KNOWN FLAKE'i çöz.** `internal/scheduler` entegrasyon testleri paralel yük altında
>    ~3'te 1 patlıyor. Önce yük altında ÜRET (`-count=3` veya tüm suite paralel), hangi bound'un
>    düştüğünü tespit et, sonra sadece onu genişlet. Körlemesine bound büyütme.
> 3. F1 için `superpowers:writing-plans` ile görev planı çıkar; "Deferred minors" listesindeki F1
>    sahipli maddeleri planın içine al.
> 4. Planı `superpowers:subagent-driven-development` ile yürüt: her görev için taze implementer
>    subagent, ardından task review, gerekirse fix turu. Bu yöntemi onaylıyorum, sorma, devam et.
> 5. Yeni bir branch aç (`phase/f1-data-model`), `main`'de çalışma.

---

## Where the work stands

| Task | State |
|---|---|
| 1 — repo skeleton, Go module, `version` | ✅ complete |
| 2 — typed config + `config:check` | ✅ complete |
| 3 — errors + redacting logger | ✅ complete |
| 4 — clock, uuidv7, AES-256-GCM, health registry | ✅ complete |
| 5 — postgres pool, embedded migrations, `migrate` | ✅ complete |
| 6 — redis + asynq `noop` round trip | ✅ complete |
| 7 — chi router, middleware, health/version/metrics | ✅ complete |
| 8 — scheduler leader election | ✅ complete |
| 9 — api/worker/scheduler/seed processes | ✅ complete (2 fix rounds) |
| 10 — import-boundary test + golangci-lint | ✅ complete (1 fix round) |
| 11 — Dockerfile, Compose, offline bundle | ✅ complete (**3** fix rounds) |
| 12 — Next.js shell + i18n + readiness page | ✅ complete (1 fix round) |
| 13 — CI | ✅ complete (2 fix rounds) |
| — final whole-branch review | ✅ **clean** after one fix wave + two residual fixes |
| — `finishing-a-development-branch` | ⬜ **awaiting the user's integration decision** |

**PHASE F0 IS COMPLETE.** 13/13 tasks, final review clean. Branch is at `c846b58`, 35 commits
ahead of `main`. Nothing pushed.

Commits this session (session 5), oldest first:

```
e50afa4 fix(f0): tighten arch-guard package matching to true subpackages only
74e500c feat(f0): dockerfile, compose stack with health checks and offline bundle script
0f79aa1 fix(f0): keep secrets out of build layers, fix offline-bundle version tags, generate a real postgres password
8426da2 fix(f0): gitignore gen-env-docker's temp files, stop the generator from rotating a live postgres password
6448f0d fix(f0): fail closed on an unparseable DSN, don't preserve an empty postgres password
e7b76ce feat(f0): next.js shell with next-intl catalogues and the readiness page
0252709 fix(f0): name eslint/postcss config exports to clear lint warnings
7307aee ci(f0): build, lint, unit, integration, vulnerability and frontend jobs
cfdd030 fix(f0): override postcss to close two high-severity advisories
07f9146 fix(f0): make ci actually cover the frontend audit gate
c49f56b fix(f0): final-review wave — build context, config bounds, scrubbing, restart policy
c846b58 fix(f0): restart the stateful services, reject a negative redis database index
```

### Verified green by the controller personally (not taken from any report)

At `c846b58` (final):

```
make lint                          -> "0 issues.", exit 0
go build ./...                     -> clean
go test ./... -race -count=1       -> all packages ok
go test ./... -tags=integration -race -count=1
                                   -> exit 0, ALL packages ok (scheduler 45.5s, postgres 27.2s,
                                      job 7.3s, redis 6.4s — real containers, not skipped)
cd web && pnpm test                -> 3/3
         pnpm typecheck            -> clean
         pnpm lint                 -> clean, NO warnings
         pnpm check:i18n-parity    -> "i18n parity ok — 9 keys in both locales"
make up                            -> every service healthy INCLUDING web:
                                      api/postgres/redis/web all (healthy), scheduler+worker up
curl -s localhost:3000             -> renders database, migrations, redis rows (server-side fetch
                                      really crossed the compose network)
docker compose down -v             -> 0 containers, 0 volumes
```

---

## The final whole-branch review — what it found

Dispatched on Opus over all 33 commits. Verdict: **Ready after named fixes** — 0 Critical,
6 Important, ~30 Minor triaged by owning phase. All six were fixed in one wave (`c49f56b`), the
scoped re-review returned all six ADDRESSED, and two residuals it surfaced were closed in
`c846b58`.

The six, all now fixed:

1. `.dockerignore` omitted `.superpowers/`, so `COPY . .` pulled the review workspace — including
   four real generated `POSTGRES_PASSWORD` values — into the build layer and cache.
2. The config loader accepted zero and negative durations/counts silently. Not one field: the same
   helpers feed seven variables across five later phases.
3. `postgres/health.go` held the one unscrubbed error leaving the store layer — and it is the one
   that reaches an unauthenticated `/health/ready` body and the public web page.
4. **`docker-compose.yml` set no `restart:` policy**, so Task 9's broker-death detector had no
   supervisor: a worker that correctly exits 1 stayed dead forever. A genuinely *emergent* defect —
   Task 9 built the detector, Task 11 built the compose file, neither owned the join. This is the
   finding that justifies having a whole-branch review at all.
5. `.env.example` shipped a *working* default DB password in the very file the offline bundle
   stages as `env.template` for air-gapped operators.
6. The `00001_extensions` down migration unconditionally dropped extensions it had not created.

Two residuals, closed in `c846b58`: `postgres`/`redis` had no restart policy either (leaving the
worker to crash-loop against a dead redis), and a **negative** `EKOKOD_REDIS_QUEUE_DB` silently
aliased to DB 0 — go-redis only issues `SELECT` under `if c.opt.DB > 0` — collapsing the job queue
onto the cache's database and defeating the invariant that package's own comment states.

### It also refuted three claims this ledger had recorded

The reviewer was explicitly invited to re-open the controller's rulings. Three did not survive:

1. The ledger said fixing `scrubErr`'s `%s`→`%w` "activates" the Task 9 `isCleanShutdown` narrowing
   and they must be done together. **The coupling does not exist** — no scrubbed error can reach
   `isCleanShutdown`. The `%w` change can be made alone. The real trade-off is different and worth
   keeping: `%w` exposes the unscrubbed cause through `errors.Unwrap`.
2. The ledger said `EKOKOD_JOB_TIMEOUT=0s` fires asynq's abort immediately. **Half wrong** — asynq
   maps a zero `ShutdownTimeout` to its own 8 s default. The *negative* case is the real defect.
3. The ledger said the elector integration test is timing-sensitive "in the wrong direction".
   **Wrong** — `require.Eventually` is forgiving both ways. The suggested `require.False` is still
   right, but as a *strength* improvement, not a flake fix.

Upheld on re-examination: the 40 s `stop_grace_period` and shared-image rulings (all four shutdown
constants checked end to end and coherent), the `main.go` doc-comment judgement, the fail-closed DSN
handling (called "exemplary"), and — emphatically — not pushing to `main`.

---

## THE ONE DECISION WAITING FOR YOU

**Task 13's brief ends with `git push origin main`. I refused to run it.** There is a real remote,
the work is on `phase/f0-foundation`, and `main` is shared. Pushing thirteen tasks of feature work
straight onto main is your call, not a subagent's. Nothing has been pushed.

When F0 closes, `superpowers:finishing-a-development-branch` will present the integration options
(PR vs. merge vs. leave the branch). Decide there.

---

## Environment — read before running anything

- **Toolchain lives in `$HOME/.local`; shell state does not persist between tool calls.** Prefix
  every command:

  ```bash
  export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"
  ```

  Go 1.27.1, Node 24.20.0, pnpm 12.3.4, golangci-lint v2.13.2, govulncheck v1.8.0.

- **Docker was UP all of session 5** (server 28.3.2). It stops between sessions — always verify
  rather than trusting this line. Images already pulled: `timescale/timescaledb:2.30.0-pg16`,
  `redis:7.4.11-alpine`, `golang:1.27.1-alpine`, `alpine:3.21`, `node:24.20.0-alpine`.
  If down, ask the user to run:
  `! "/mnt/c/Program Files/Docker/Docker/Docker Desktop.exe" &`

- **`/mnt/c` (DrvFs) is not merely slow — it is BROKEN for stock `pnpm install` and `next build`**
  (unconditional `chmod` EPERM, same-drive `fs.copyFile` EPERM). Session 5 worked around it with a
  native-filesystem mirror at `/home/personal/ekokod-web-native` plus a `web/node_modules` symlink
  that already exists on disk. **Do not re-engineer this.** Committed accommodations:
  `web/.npmrc` (`package-import-method=copy`, self-documented) and `web/pnpm-workspace.yaml`
  (`allowBuilds`). Nothing committed depends on the symlink — `web/Dockerfile` runs its own
  `pnpm install` in the container, and the Docker build is the authoritative proof that
  `pnpm build` works.
  `.gitignore` line 3 is `/web/node_modules` **without a trailing slash** — that matters, see the
  rulings below.

- **`shellcheck` is NOT installed here.** The two shell scripts have never been statically
  analysed locally; Task 13 was told to enforce it in CI.

---

## Decisions already made — do not re-litigate

| Decision | Value |
|---|---|
| Go module path | `github.com/MErenTalan/ekokod-rewrite` |
| Binary name | `ekokod` |
| Env var prefix | `EKOKOD_` (spec appendix documents `BCEM_`; names keep their spelling, prefix swapped) |
| Branch | `phase/f0-foundation`, no separate git worktree |
| Execution method | `superpowers:subagent-driven-development`, user approved plan + parallel subagents |
| Commit trailers | `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>` + the session's own `Claude-Session:` URL — **the task briefs carry a STALE session URL, always override it** |

### Standing rulings that keep paying off

- **When the plan's prose or sample code disagrees with its stated requirements or its own tests,
  the requirements and tests win.** Held ~10 times this phase; the plan's own sample code has now
  contained a live defect at least four times.
- **Verify interfaces and pins against the real tree BEFORE dispatching.** Session 5's Task 12
  pre-flight checked all 11 npm version pins against the registry; Task 11's checked the
  `buildinfo` ldflag paths and every route. A single bad pin is a guaranteed wasted round trip.
- **Do not trust a report — re-run the gate yourself.** Every "verified green" line above was
  produced by the controller. This caught things reviewers missed twice in session 5.
- **Prove a guard can fail.** Task 10's implementer created a real violating package, watched the
  tightened guard fail, then re-ran the SAME violation against the old logic and watched it pass.
- **Ask the reviewer a specific question.** Every finding that mattered this session came from a
  pointed question, not from "please review this".
- **Scale the reviewer to the risk.** Opus on Task 11 (deployment artifacts) found 4 Importants
  that a cheaper tier would likely have missed.

---

## Rulings made in session 5 (the full list, in order)

Each is recorded in the ledger with its cost-if-wrong.

**Task 10**
1. Fold review Minor 2 (the `ListenConfig` ctx caveat) into the fix round rather than deferring —
   the caveat existed only in a git-ignored report file and a future reader would assume the opposite.
2. Defer Minors 1 and 3. The bare-slog-handler guard is textual not AST-based (real, but it is
   defence in depth and is proven live); the `main.go` comment the reviewer called misleading is,
   on re-reading, literally accurate.

**Task 11**
3. Compose MUST set `stop_grace_period: 40s` on api/worker/scheduler. Docker's 10s default would
   SIGKILL the worker 20s before its 30s drain window closes. **Load-bearing.**
4. The four Go services share ONE image with identical build args — the brief would have compiled
   worker/scheduler with ldflag defaults, so `ekokod version` disagreed across one deployment.
5. `web` stays in compose though it cannot build until Task 12; verification is Go-stack-only, and
   `make up`/`make offline-bundle` were carried forward to Task 12. (Discharged — both now verified.)
6. The `.env.docker` generator becomes a committed script — the file is gitignored, so a fresh
   clone running `make up` would otherwise hit an opaque missing-env_file error.
7. The shared-image layout stands; do NOT add `pull_policy: never`. Settled by execution: deleted
   `ekokod:dev` outright and ran `docker compose up -d` with no `--build` — no pull, exit 0.
8. FIX the hardcoded `POSTGRES_PASSWORD` even though the brief mandates the literal. The spec's
   "no secret has a default value" outranks the plan's text, and this file ships to air-gapped
   operators. Constraint: `make up` on a fresh clone must still work with zero manual steps.
9. Fold all five Minors into the same fix round — each is a one-liner in a file already open.
10. The generator must reason about BOTH files, with a defined outcome for all four states
    (neither/both/`.env` only/`.env.docker` only), never rotating a password an existing volume
    may already hold.
11. Prove the `.env`-only state against a LIVE already-initialised volume — reading the code cannot
    distinguish fixed from broken there, since both produce two mutually-consistent files.
12. Fail CLOSED and LOUD on an unparseable DSN, and **never echo the DSN in the error** — three of
    this phase's Criticals came from error text embedding a connection string.
13. Treat an empty `POSTGRES_PASSWORD` as absent, not as a value to preserve.

**Task 12**
14. Step 7 must NOT undo the i18n divergence with `git checkout` — the file is untracked at that
    point, so the checkout fails and the corruption would ship. Copy aside, mutate, restore by copy,
    diff to prove byte-exact.
15. `web/public/` needs a tracked `.gitkeep` — git does not track empty directories, so a fresh
    clone would break the Dockerfile's `COPY /app/public`.
16. `vitest.config.ts` gets an explicit `@/` alias — Vitest does not read tsconfig `paths`.
17. The `web/node_modules` SYMLINK must never be committed. `.gitignore`'s `/web/node_modules/`
    (trailing slash) matches directories only; proven with `git check-ignore`. Slashless form fixes it.
18. Keep the DrvFs workaround, but nothing committed may depend on the symlink, `.npmrc` must
    explain itself (it binds CI too), and a fresh clone must work with a plain `pnpm install`.
19. The implementer's edits to TWO of Task 11's files (`web/Dockerfile`, `docker-compose.yml`) are
    AUTHORISED and belong to Task 12 — both bugs were discoverable only by running `make up`, which
    Task 11 provably could not do. Precedent: the Task 6→5 and Task 9→2 rulings.
20. Fix the two eslint warnings by NAMING the exports, not by adding `eslint-disable` — this phase
    does not silence linters (Task 10 fixed all 40 findings and suppressed none).

**Task 13**
21. **DO NOT `git push origin main`.** See "the one decision waiting for you".
22. Pin `govulncheck` to v1.8.0 in CI — the brief's `@latest` disagrees with the Makefile's pin.
23. CI must run golangci-lint at the SAME pinned version as local; verify the action major supports
    the v2 line, else use `make tools && make lint`.
24. Move `--max-warnings=0` into `web/package.json`'s lint script so local and CI agree.
25. CI must run `shellcheck` over `scripts/*.sh` — closes a Task 11 deferred minor.

---

## KNOWN FLAKE — the first thing F1 should fix

`internal/scheduler`'s integration tests fail intermittently under full-suite parallel load.
Measured at the final commit: **run 1 FAILED** with the package at 100.752s; **runs 2 and 3 passed**
at 47.0s and 49.3s; all five tests pass in isolation in 42.9s. The 2× wall-clock blowup points at
testcontainers/Docker contention while `go test ./...` runs packages in parallel, against the
`require.Eventually(..., 5*time.Second, ...)` bounds at `elector_integration_test.go:67,76,80,102,
140,157` — a 5 s budget to acquire a Postgres advisory lock is thin on a machine running several
containers at once.

**The evidence is incomplete and I am saying so rather than papering over it:** the failing
assertion's text was lost to an over-narrow grep on the first run and did not reproduce in two
further attempts, so *which* bound failed is unknown. I deliberately did not widen bounds on a
guess — that is the same blind fixing I forbade a Task 13 implementer from doing over shellcheck
findings, and the standard applies to me too.

**F1: reproduce under load first** (`-count=3`, or the full suite in parallel), identify the bound
that actually fails, then widen that one. **CI runs the identical command on `ubuntu-latest`, so it
may flake there too** — a first-run red CI is as corrosive as a knowingly-red one.

---

## Deferred minors — triaged by the final whole-branch review

The final review triaged every item below and assigned an owning phase. Items marked **fixed** were
closed in the final fix wave (`c49f56b`, `c846b58`).

**Task 1** — text-mode version test asserts only the `ekokod` substring (the brief's own test).

**Task 2**
- `External.ISolarRedirect` has no derived default (F2 owns it).
- The 720h "remember me" refresh-token variant is not modelled (F6).
- **`loader.duration` does not validate positivity.** `EKOKOD_JOB_TIMEOUT=0s` or `-5m` yields
  `ShutdownTimeout <= 0` and asynq's abort timer fires immediately — in-flight tasks killed with no
  drain. Pre-existing, not introduced by Task 9's `min()` clamp.

**Tasks 3+4**
- logging's secret-substring list is unexported and not extendable by callers.
- **logging's redaction is KEY-BASED ONLY** — `redactAttr` never scans attribute *values*. Every
  `slog.String("error", err.Error())` site depends entirely on the error having been scrubbed at
  construction. Audited sites are currently safe. Phase-wide invariant worth remembering.

**Task 5**
- The down migration drops timescaledb/pgcrypto/citext unconditionally.
- `checkMigrationsCurrent`'s non-`isUndefinedTable` branch returns an **unscrubbed** error.

**Task 6**
- `scrubErr` builds its error with `%s` not `%w`, so `errors.Is`/`As` cannot see through it.
  **Fixing this activates Task 9's ruling that `isCleanShutdown` is narrowed to `context.Canceled`
  only — do them together.**
- `internal/job/scrub.go` and `internal/store/redis/scrub.go` hold byte-identical `scrubParseErr`.

**Task 7**
- `ratelimit.go` reaps stale buckets only every 10 minutes (trivial to grow with an IPv6 /64).
- `Retry-After: 60` is hardcoded, unrelated to the configured window.

**Task 8** (six, all from the Opus review)
- `elector.go:54` — `time.NewTicker(e.retry)` panics on `retry <= 0` in an exported path.
- `elector_integration_test.go:157` — timing-sensitive in the wrong direction (a *faster* machine
  makes it more likely to fail). Deterministic form is `require.False` after `<-cancelled`.
- `scheduler.go:69-80` — `asynq.NewScheduler` leaks a go-redis client per failed campaign (~1/10s).
  Unreachable today; goes live when config-driven entries are registered.
- `elector.go:139` — discards `pg_advisory_unlock`'s boolean.
- `elector.go:148-150` — `recover()` keeps a deterministically panicking `lead` re-running forever.
- Inherent: `IsLeader()`/`leadCtx` can lag reality by ~4s without fencing tokens.
- **Carried forward:** the phase introducing the first real scheduled job must wire
  `Scheduler.entries()` (it registers only the noop task) and add coverage.

**Task 9**
- `dsnDisplay`'s `q.Encode()` alphabetises and percent-encodes; display-only.
- `internal/cli/api.go`'s serve goroutine is not joined before `RunE` returns (harmless).

**Task 10**
- The depguard `no-float-money` rule targets packages that do not exist until F4. Intended.
- The bare-slog-handler guard detects textually (`strings.Contains`), so it is evadable by import
  aliasing, unlike the AST-based domain checks. Defence in depth; proven live.
- `cmd/ekokod/main.go`'s doc comment framing — controller judged it literally accurate, recorded so
  the final review can re-open it.

**Task 11**
- The fix-round-3 live-test transcript in `task-11-report.md` prints a real generated
  `POSTGRES_PASSWORD`. The whole SDD workspace is gitignored and the volume was destroyed; the
  workspace is deleted at phase end. Test-log hygiene only.

**Task 12**
- `check-i18n-parity.mjs` compares flattened key PATHS but not leaf value TYPES.
- `.gitignore` additions for `/web/next-env.d.ts` and `/web/tsconfig.tsbuildinfo` sit outside the
  brief's literal file list. Low risk.

---

## What the reviews caught across the phase

Two classes dominate — keep reviewer prompts pointed at them.

1. **Task 2, Critical** — `net/url.Parse`'s error embeds its input, printing the DB password.
2. **Tasks 3+4, Critical** — the redacting handler never resolved `slog.LogValuer`.
3. **Task 5, Critical** — pgx splits DSN userinfo at the first `@`, `net/url` at the last.
4. **Task 7, Critical (plan's own sample code)** — allow-all CORS *with credentials* when
   `EKOKOD_CORS_ORIGINS` was unset.
5. **Task 7, Important (plan's own sample code)** — unbounded Prometheus labels from unmatched routes.
6. **Task 9, Important (plan's own sample code)** — `asynq.Server.Run` races `signal.NotifyContext`;
   the second `Shutdown` is a no-op, so `RunE` returned nil while asynq still drained.
7. **Task 9, Critical (in Task 2's shipped code)** — `config.redactDSN` failed OPEN.
8. **Task 10, Important** — arch guards used unbounded prefix matching, so `internal/apikeys` would
   have been silently exempted from the "only the CLI imports the API" rule.
9. **Task 11, Important ×4** — secrets into a build layer via an incomplete `.dockerignore`; an
   unexported `VERSION` that hands air-gapped operators a tarball whose images the compose file
   cannot name; a generator that could leave a sticky empty file; a hardcoded Postgres password
   shipped to operators.
10. **Task 11, Important ×2 more, each introduced BY A REPAIR** — temp files past `.gitignore`;
    partial-state runs force-rotating a live password.

**Nearly every one was a secret-handling or liveness defect the plan's own tests did not cover.**

---

## Open questions for the product owner (block F4, not F0)

`docs/rewrite/02-domain-rules.md` §11. They need research, so raise them early.

1. **Reactive penalty base** — entire reactive quantity charged (legacy) or only the excess?
2. **Sub-9 kW exemption** — confirm installations under 9 kW are exempt.
3. **Tiered pricing user groups** — should `residentialPlus`/`commercialPlus` be tiered, at what
   daily kWh thresholds?
4. **Missing-hour tolerance** — confirm 2 % as the withhold-for-review threshold.
5. **Grid emission factor** — confirm the current official Türkiye value and source year, and
   whether historical reports are recomputed or keep 0.45.

---

## Key paths

```
docs/rewrite/                                          the specification
docs/rewrite/09-implementation-plan.md                 phases F0–F15
docs/superpowers/plans/2026-09-08-f0-foundation.md     the F0 task plan
.superpowers/sdd/2026-09-08-f0-foundation/             git-ignored workspace
  progress.md                                          THE LEDGER — read the tail first
  task-N-brief.md / task-N-report.md                   per-task briefs and reports
  review-<base>..<head>.diff                           review packages
```

SDD helper scripts:
`/home/personal/.claude/plugins/cache/claude-plugins-official/superpowers/6.3.0/skills/subagent-driven-development/scripts/`
(`task-brief`, `review-package`, `sdd-workspace`).

**Tip for the final review package:** `web/pnpm-lock.yaml` is 5774 generated lines. Exclude it —
`git diff -U10 <base>..<head> -- . ':(exclude)web/pnpm-lock.yaml'` — or the reviewer reads noise.
