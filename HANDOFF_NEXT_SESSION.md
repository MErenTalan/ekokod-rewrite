# Handoff — ekokod rewrite, phase F1 (Data model and migration framework)

> **STATUS: F1 IN PROGRESS.** This file is kept current as the phase runs, so that it is
> useful even if the session ends unexpectedly. The "Where the work stands" table is the
> truth; anything below it that contradicts the table is stale.

Branch `phase/f1-data-model`, branched from `main` at `6975b20` (F0 merged).
**Nothing has been pushed.** `main` is still local-only ahead of `origin/main`, and the push
decision is the user's — do not push.

---

## Where the work stands

| Task | State |
|---|---|
| — | pre-F1: known flake fixed (`e85a04c`) — **the F0 diagnosis was wrong**, see below |
| 1 — tenancy + buildings migrations, reversibility harness | ✅ complete, review clean |
| 2 — hypertables + continuous aggregates | ✅ complete (3 fix rounds, 2 scoped re-reviews) |
| 3 — tariffs + icmal | ✅ complete, review clean |
| 4 — carbon + ISO 50001 | ✅ complete, review clean |
| 5 — files/integrations/calendar/operations | ✅ complete, review clean |
| 6 — bills + reports + alarms | ✅ complete, review clean |
| 7 — sqlc config, generated types, uuid override | 🔄 fix round 2 in flight |
| 8a — `store.Scope`, arch guard, scrub fixes | ✅ complete, re-review clean |
| 8b — domain model, decimal conversion, float guard, fixtures | 🔄 in flight (**critical path**) |
| 9 — tenancy/building/analyzer/plant repositories | ⬜ not started |
| 10 — time-series repositories | ⬜ not started |
| 11 — remaining repositories + `admin` | ⬜ not started |
| 12 — seed loader | ⬜ not started |
| 13 — performance + acceptance suite | ⬜ not started |
| — final whole-branch review | ⬜ not started |

**All eleven migrations exist and are green.** `migrate up → down → up` passes across the
complete schema, verified by the controller personally on the merged result.

## Key paths

```
docs/rewrite/                                          the specification
docs/rewrite/04-data-model.md                          AUTHORITATIVE DDL for F1
docs/superpowers/plans/2026-09-10-f1-data-model.md     the F1 task plan (13 tasks, 7 waves)
.superpowers/sdd/2026-09-10-f1-data-model/             git-ignored workspace
  progress.md                                          THE LEDGER — read the tail first
  task-N-brief.md / task-N-report.md                   per-task briefs and reports
  sqlc-spike-findings.md                               controller-run spike, see below
  timescale-shims-PROVEN.sql                           proven sqlc shim
```

---

## THE FLAKE THE F0 HANDOFF DESCRIBED WAS MISDIAGNOSED

F0 recorded it as `internal/scheduler` integration tests failing ~1 in 3 under parallel load,
and told F1 to widen whichever `require.Eventually` bound was failing.

**That was wrong.** Under `go test ./... -tags=integration -race -count=3` the scheduler
package passes (134.9s for three runs, ~45s each, matching its healthy baseline). The package
that failed was `internal/store/redis`, and the failure was not an `Eventually` bound at all:

```
wait until ready: external check: check target: retries: 93 address: localhost:32936:
get state: Get ".../containers/<id>/json": context deadline exceeded
```

Redis had already logged "Ready to accept connections". What timed out was the **Docker API
call** that testcontainers' port check issues on every poll. Root cause was an asymmetry:
`modules/redis@v0.44.0/redis.go:72` hardcodes `WithStartupTimeout(10s)`, while
`modules/postgres@v0.44.0/wait_strategies.go:24` leaves the library default of 60s. Fixed in
`e85a04c` by restoring 60s for redis (REPLACING the strategy — appending leaves the 10s one
in place and still fails).

### …and the same class recurred on postgres during F1

The controller's own full-suite run then failed with `TestEmissionFactorKeyUniqueness` at
62.32s (5.34s in isolation). Reproduced deterministically by running the postgres package
**twice concurrently**:

```
retries: 520 ... context deadline exceeded
--- FAIL: TestHypertablesAreConfigured (61.07s)
```

61.07s = the postgres module's 60s default, exhausted by 520 Docker API polls. Postgres never
tripped it before because F1 grew that package from ~9 integration tests to ~23, each starting
its own container. Assigned to Task 8b: raise to 180s in `testfixtures.StartPostgres`.

**The property that makes this family so expensive: a container-startup failure surfaces as an
arbitrary test failing, so it presents as a different flaky test every run.** That is exactly
why F0 pinned it on the wrong package. If you see a new "flaky test", check whether the failure
is actually a container readiness timeout before believing the test name.

**Structural fix, recorded not done:** the cause is one container per test. A shared container
per package with per-test isolation is the real answer, but several tests genuinely need a
virgin database (the migrate round-trip, the reversibility guard). This is the lever if suite
time or flake rate worsens.

---

## Standing rulings that keep paying off (carried from F0, all reconfirmed in F1)

- **Requirements and tests beat the plan's prose and sample code.** Held repeatedly again.
- **Verify interfaces against the real tree BEFORE dispatching.** The F1 pre-flight scan caught
  six real conflicts, including four parallel tasks that would have collided on one test file
  and a proposed test that duplicated an existing one verbatim.
- **Do not trust a report — re-run the gate yourself.** This caught: a "no sleeps" claim that
  was false, a task's SQLSTATE claim that was true of the run but not of the committed
  assertions, and a full-suite flake no subagent had seen.
- **Prove a guard can fail.** Non-negotiable now — see the next section for why.
- **Scale the reviewer to the risk.** Opus reviewers found every Critical in this phase.

### NEW STANDING RULING — a guard that has only been seen to pass is not evidence

**Five guards in this phase turned out to be weaker than their names.** This is the phase's
dominant defect class, and it is worth carrying forward as a first-class suspicion:

1. `no-float-money` (from F0) — depguard is an **import** linter and cannot see a struct field.
   The "money is never float64" invariant was never mechanically enforced at field level.
2. The reversibility guard — queried `pg_tables` only, so a forgotten `drop materialized view`
   would pass silently. Widened to `pg_class` + `pg_type`.
3. `TestEveryStoreMethodIsScoped` — matched only *exported* types named `*Repository`, so it
   inspected **zero methods** and passed. The idiomatic unexported `buildingRepository`
   satisfying an exported interface was invisible.
4. The sqlc drift check — `git diff --exit-code` **ignores untracked files**, so a new
   generated file that was never committed passes CI.
5. `TestShimDeclaresEveryTimescaleFunctionTheMigrationsUse` — the guard added to close this very
   class only examines functions the shim *already* declares, so a newly-used undeclared
   function is never checked. (Fix in flight.)

---

## Decisions already made — do not re-litigate

| Decision | Value |
|---|---|
| Go module path | `github.com/MErenTalan/ekokod-rewrite` |
| Binary name | `ekokod` (spec says `bcem`; only the name changed) |
| Env var prefix | `EKOKOD_` |
| Branch | `phase/f1-data-model` |
| Execution method | `superpowers:subagent-driven-development`, user approved plan + parallel subagents |
| Commit trailers | `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>` + the session's own `Claude-Session:` URL — **task briefs carry a STALE session URL, always override** |
| Parallelism | isolated git worktrees on the **native** filesystem (see Environment) |

---

## Environment — read before running anything

- **Toolchain in `$HOME/.local`; shell state does not persist between tool calls.** Prefix every
  command: `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"`
  Go 1.27.1, Node 24.20.0, pnpm 12.3.4, golangci-lint v2.13.2, govulncheck v1.8.0,
  **sqlc v1.30.0** (added in F1, pinned in `Makefile` and CI).
- **Docker stops between sessions — always verify with `docker ps`.** If down, ask the user to
  run: `! "/mnt/c/Program Files/Docker/Docker/Docker Desktop.exe" &`
- **Git worktree isolation fails on this DrvFs mount** with "dubious ownership", because a linked
  worktree's gitdir lives on `/mnt/c`. The Agent tool's `isolation: "worktree"` therefore does
  **not** work here. What does work, and is how F1 ran four implementers concurrently:
  ```bash
  git worktree add -b f1/task-N /home/personal/ekokod-wt-tN phase/f1-data-model
  git config --global --add safe.directory /home/personal/ekokod-wt-tN
  ```
  Then dispatch a normal (non-isolated) agent told to `cd` there. Merge with `--no-ff`.
- **`/mnt/c` (DrvFs) is BROKEN for stock `pnpm install` and `next build`.** Session 5's
  native-filesystem mirror at `/home/personal/ekokod-web-native` plus a `web/node_modules`
  symlink is on disk. **Do not re-engineer this.**
- **`shellcheck` is NOT installed locally**; CI enforces it.

---

## F1-specific technical facts a successor must know

### sqlc

- sqlc **parses the goose migrations directly** — `create_hypertable`, compression settings and
  `create materialized view ... with (timescaledb.continuous)` all pass. There is deliberately
  **no `schema.sql`**, no pg_dump step and no schema-drift check.
- **`internal/store/postgres/timescale-shims.sql` is load-bearing and never applied to a real
  database.** Without it, aggregate columns mistype — measured: `active_consumption`, an
  **energy** value, generated as `int32`. Silent kWh truncation that no build, lint or migration
  test would catch.
- **The `::numeric` casts in `00005` are load-bearing and look redundant.** They are what keep
  aggregate value columns numeric. Do not "tidy" them away.
- UUIDs are `google/uuid.UUID` / `*uuid.UUID` via override. **A SQL NULL decodes as a nil
  pointer, never `uuid.Nil`** — proven against a real container. No pgx codec registration was
  needed; `pool.go` is untouched.
- Generated code keeps `pgtype.Numeric`. `internal/domain/model` uses `decimal.Decimal`, and
  Task 8b provides **one audited conversion pair** which must convert via string/exponent,
  **never via float64**.
- `postgres.DB` holds a **named unexported** `q *sqlcgen.Queries` — deliberately NOT embedded,
  because embedding promoted unscoped query methods onto the store's exported surface.

### Continuous aggregates

- **All six are defined directly over the base hypertable; none is hierarchical.** Not a
  platform limitation — a roll-up from `consumption_hourly` is *wrong by a whole hour* for the
  seven registers with an index column, and loses every inter-hour step for the five without.
- **Buckets of one day or wider carry `'Europe/Istanbul'`.** UTC bucketing violated
  `02-domain-rules.md` §1 (day/month boundaries evaluated in Istanbul) — it attributed local
  00:00–03:00 to the previous day. `consumption_hourly` is deliberately timezone-free.
- **`materialized_only` is split:** `false` on `consumption_hourly`, `consumption_daily`,
  `plant_production_daily` (open-bucket tail is bounded); `true` on the monthly/yearly views
  (their tail would be months of raw readings — `consumption_yearly` would scan nine months per
  analyzer in September).
- **CONSTRAINT ON TASK 10:** month-to-date and year-to-date must be **composed explicitly**
  (closed buckets + open period from `consumption_daily` or `meter_readings`). They are NOT
  present in the monthly/yearly views. Do not "fix" a missing row by flipping `materialized_only`.
- Timescale does **not** materialise a partially-covered bucket at any `end_offset`.
- **`start_offset` permanently caps materialised history.** After a legacy backfill an operator
  must run `call refresh_continuous_aggregate('<view>', NULL, now());` per aggregate, or old
  data is silently absent — the query returns rows, just fewer than the truth. Runbook item for
  the data migration; nothing enforces it.
- Plant aggregate column names (the implementer's invention; the spec names none):
  `production_kwh`, `max_active_power_kw`, `avg_efficiency_pct`.
- `00005` requires `-- +goose NO TRANSACTION` (SQLSTATE 25001).

### Spec defects found by execution

1. **`04-data-model.md` §4.5 is not executable as written.** `add_compression_policy('plant_production', ...)`
   errors "columnstore not enabled on hypertable" — the spec enables compression on
   `meter_readings` but never on `plant_production`. The migration adds the missing
   `alter table ... set (timescaledb.compress, ...)`.
2. **`plant_production`'s primary key `(plant_id, ts, device_id)` includes a column the spec
   declares nullable.** Postgres silently promotes it to NOT NULL, so **plant-level production
   rows with no attributed device are unstorable**. Transcribed as written. **BLOCKS F2** — if
   iSolar reports plant-level totals without a device serial, F2 needs a hypertable primary-key
   change. Cheap while empty, expensive later. **Raise with the product owner.**
3. §3's preamble ("every tenant-scoped table carries `company_id` directly") is contradicted by
   the spec's own DDL for `building_contacts`, `power_plant_monthly_targets`,
   `power_plant_devices`, `power_plant_alarm_recipients` and `plant_production`. Transcribed as
   written. **Consequence for repositories:** `building_contacts` and `power_plant_devices` have
   surrogate ids and so ARE addressable without a tenant reference — they must always be reached
   through a parent-scoped query.
4. §1 lists `btree_gin`; F0's `00001` omitted it. Added in F1.

### Repository conventions Tasks 9–11 must follow

- Every method takes `ctx` first, `store.Scope` second. Reject an invalid scope before touching
  the database.
- Use `Scope`'s authorisation branch. **Never write `if len(s.BuildingIDs) > 0 { ...ANY($1)... }`**
  — that is fail-open, since an empty slice drops the predicate.
- A nil/empty `BuildingIds` encodes to `NULL::uuid[]` and matches nothing, so a scope granting no
  buildings returns **zero rows**. That is the designed fail-closed semantic; rely on it.
- `audit_log` has no FKs (deliberate — an audit trail must survive its subject). Every join to
  `companies`/`users` must be a **LEFT JOIN**, and the read model must render a missing subject
  rather than dropping the row.
- Errors route through the package's `scrubErr`. Scrubbed errors are traversable by `errors.Is`
  via an unexported `scrubbed` type whose `Error()` is the redacted text; `secret.IsScrubbed` is
  the positive fingerprint. **The "DSN did not parse" branch deliberately attaches NO cause** —
  `*url.Error` embeds its whole input, i.e. the password. There is a test guarding this; do not
  "fix" the inconsistency.

### HARD REQUIREMENT FOR TASK 9

`TestEveryStoreMethodIsScoped` currently inspects **0 methods** and passes — the correct
pre-repository baseline (the exemption was measured to suppress exactly 5, all inside `sqlcgen`,
none in package `postgres`). **Task 9 must flip its `t.Log` of the inspected count to
`require.Positive`.** If it does not, the guard is green over an empty set for the rest of the
project and a narrowing bug in it is indistinguishable from having nothing to check.

---

## Open questions for the product owner (block F4, plus one that blocks F2)

`docs/rewrite/02-domain-rules.md` §11, unchanged from F0:

1. **Reactive penalty base** — entire reactive quantity charged (legacy) or only the excess?
2. **Sub-9 kW exemption** — confirm installations under 9 kW are exempt.
3. **Tiered pricing user groups** — should `residentialPlus`/`commercialPlus` be tiered, at what
   daily kWh thresholds?
4. **Missing-hour tolerance** — confirm 2 % as the withhold-for-review threshold.
5. **Grid emission factor** — confirm the current official Türkiye value and source year, and
   whether historical reports are recomputed or keep 0.45.

**NEW, blocks F2:** does iSolarCloud report plant-level production without a device serial? If
so, `plant_production`'s primary key must change before any real ingestion (see spec defect 2).
