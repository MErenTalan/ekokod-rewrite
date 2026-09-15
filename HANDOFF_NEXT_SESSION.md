# Handoff — ekokod rewrite, phase F1 (Data model and migration framework) — session 2

> **STATUS: F1 is in its last mile, paused at the user's request so the next session starts clean.**
> Every repository in the project exists and has passed task review; every in-flight agent finished
> and nothing is running. What remains: the Wave F integration commit (merging `f1/task-11b` and
> `f1/task-11c`), Task 12b, Task 13b, and the F1 final review.
> **The F2 plan is written but NOT reviewed.**
> Nothing has been pushed; `main` has not been touched. **Push and merge-to-main are the user's call.**

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/`
> içinde, faz planı `docs/rewrite/09-implementation-plan.md`. Toplam 16 faz var (F0-F15).
>
> **Durum: F1 son adımlarında.** Branch `phase/f1-data-model`. Tüm repository'ler yazıldı ve task
> review'den geçti; çalışan agent yok. Kalan işler, sırasıyla:
> 1. Wave F entegrasyon commit'i (`f1/task-11b` ve `f1/task-11c` merge'ü dahil),
> 2. tam integration suite,
> 3. Task 12b ve Task 13b (paralel),
> 4. F1 final whole-branch review.
>
> **F2 planı yazıldı ama review edilmedi.** Hiçbir şey push edilmedi, `main`'e dokunulmadı. Push ve
> `main`'e merge kararı bana ait, sen yapma.
>
> Şu sırayla ilerle:
>
> 1. **`HANDOFF_NEXT_SESSION.md`'i baştan sona oku.** Özellikle şu bölümleri: "START HERE", "Binding
>    rulings from session 2", "Process rules that paid off" ve "Environment". Sonra
>    `.superpowers/sdd/2026-09-10-f1-data-model/progress.md` (ledger) dosyasının son ~150 satırını oku.
> 2. **İşleri `superpowers:subagent-driven-development` ile yürüt.** Her görev için taze implementer,
>    ardından task review, gerekirse fix turu. **Bu yöntemi onaylıyorum, sorma, devam et.**
> 3. **Aynı anda en fazla 5 agent çalıştır.** Hızlı ol ama review'leri atlama, gereksiz token harcama.
>    - İmplementer'lar sonnet olsun.
>    - İzolasyon veya güvenlik riski taşıyan task review'ler opus olsun.
>    - Scoped re-review'ler sonnet olsun.
>    - Büyük transcript'li bir agent'ı küçük bir fix için resume etme; taze ve ucuz bir agent ver.
> 4. **Paralel iş için native-filesystem worktree kullan.** Agent tool'un `isolation: "worktree"`
>    özelliği bu mount'ta ÇALIŞMIYOR. Environment bölümündeki tarifi kullan.
> 5. **Subagent'lar arka plan komutu beklerken turlarını bitiriyor.** Bu oturumda 9 kez oldu.
>    Raporsuz "bekliyorum" mesajı gelirse hemen SendMessage ile dürt: gate'leri ön planda koşsun,
>    commit etsin, raporlasın.
> 6. **Her guard'ın başarısız olabildiğini kanıtlat.** Bir guard'ı yalnızca geçerken görmek kanıt
>    değil. Bu fazda 12 guard adından zayıf çıktı. Tenant'lar arası izolasyon kanıtları **AdminScope**
>    ile yapılmalı.
> 7. **F1 bitince durma, F2'ye geç.**
>    - Önce F2 planını review ettir (`writing-plans` skill'inin plan-document-reviewer prompt'u).
>    - `phase/f1-data-model`'in ucundan `phase/f2-integration-layer` branch'ini aç ve F2'yi yürüt.
>    - **F2 bittiğinde dur**, handoff'u güncelle ve bana yapıştırılacak prompt ver.
>
> Ortam:
> - Her komuttan önce `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"`.
> - Docker seanslar arası ve WSL yeniden başlayınca kapanıyor; önce `docker ps` ile doğrula. Kapalıysa
>   benden başlatmamı iste.

---

## START HERE — exact state

### Branches

| Branch | Head | State |
|---|---|---|
| `phase/f1-data-model` | `dfae237` (+ this handoff commit) | 8a, 8b, 8c, 9, 10, 11a, 12a and 13a are merged. Gates verified at `dfae237`: check-generate, vet, lint 0 issues, make test, build — all green. The FULL integration suite has not run since the merges; it runs in the integration commit. |
| `f1/task-11b` | `c306e84` | Carbon, ISO 50001, files, integrations, ops, admin catalogue/journal. **Complete and review-clean** (1 fix round). NOT merged on purpose: it add/add-conflicts with Task 10 on `admin/numeric_copy.go`, which the integration commit unifies. |
| `f1/task-11c` | `469d059` | Calendar + SMTP repositories. **Complete and review-clean.** Branched from `b54ebfd` (11b's pre-fix head); its files do not overlap 11b's. |
| other `f1/task-*` | — | Already merged; they can be deleted. |

No worktrees remain besides the main checkout; `f1/task-11b` and `f1/task-11c` exist as branches only.

### Task table

| Task | State |
|---|---|
| 1–7 | ✅ complete (session 1) |
| 8a | ✅ complete |
| 8b | ✅ complete. The task review ran this session: 6 Important findings, 1 fix round, re-review clean. |
| 8c (inserted) | ✅ complete. Shared plumbing: `pgerr.Translate`, `testfixtures.NewIsolatedDB` (template-clone DB per test, ~120 ms), SQL-scanner guard lexer. 2 fix rounds. |
| 9 | ✅ complete. Tenancy/buildings/analyzers/plants + admin auth/audit. `require.Positive` hard requirement done. |
| 10 | ✅ complete. Readings (COPY bulk insert), cursors, anomalies, production, prices, forecasts, analytics + admin market data. |
| 11a (split) | ✅ complete. Tariffs, bills, reports, alarms. 2 fix rounds; one Critical fixed (cross-tenant FK storage). |
| 11b (split) | ✅ complete, unmerged (merged by the integration commit) |
| 11c (inserted) | ✅ complete, unmerged |
| 12a (split) | ✅ complete. Embedded seed datasets: 182 emission factors, 3 iSolar integration definitions, an empty national tariff file. |
| 12b | ⬜ not started. Loader via `store.AdminCatalogueRepository` + CLI. Notes: `wave-g-task-notes.md` §Task 12b. |
| 13a (split) | ✅ complete and merged. The EXPLAIN runs over the SQL `repo.Range` actually sends (traced pool). 1M rows ≈ 85 s. |
| 13b | ⬜ not started. `TestScopeIsolation` over every repository + the F1 acceptance block run. Notes: `wave-g-task-notes.md` §Task 13. |
| Wave F integration commit | ⬜ not started. Brief ready: `.superpowers/sdd/2026-09-10-f1-data-model/wave-f-integration-brief.md` |
| F1 final whole-branch review | ⬜ not started |
| F2 plan | 📝 written (`docs/superpowers/plans/2026-09-15-f2-integration-layer.md`, 2503 lines, 17 tasks / 6 waves), **not reviewed** |

### Remaining work, in order

1. **Verify** `docker ps` and `git log --oneline -3` match this handoff.
2. *(11b and 13a are closed — nothing to do.)*
3. **Wave F integration commit** (one agent, sonnet; brief `wave-f-integration-brief.md`):
   - merge `f1/task-11b`, then `f1/task-11c` (13a is already merged);
   - move the numeric conversion pair into `internal/store/postgres/internal/pgnum`, delete the `admin/numeric_copy.go` copies (Tasks 10 and 11b each made one: an add/add conflict, and 11b's is partial);
   - Task 9 residual minors;
   - Task 10 residual (cross-tenant readings/forecast tests → AdminScope);
   - the **FULL integration suite** once.
4. **Task 12b and Task 13b in parallel** (notes in `wave-g-task-notes.md`).
5. **F1 final whole-branch review** (opus, `requesting-code-review/code-reviewer.md`, package from `git merge-base main HEAD`). Point it at every `minor (deferred)` and `parked` line in the ledger (~27 deferred minors from session 2 alone). ONE fix dispatch, one scoped re-review, adjudicate residuals.
6. **Rewrite this handoff for F2**, including the "Rulings I made" list.
7. **F2:**
   - review the plan;
   - create `phase/f2-integration-layer` from F1's tip (do NOT merge to main);
   - run the plan's pre-flight;
   - execute. **Stop when F2 is finished** and give the user the handoff prompt.

---

## Binding rulings from session 2 (every later task depends on these)

The full text, each with "cost if wrong", is in the ledger as `Ruling:` lines (from line 759 on). These rules bind all repository code and all later phases:

1. **Only package `internal/store/postgres/admin` is unscoped** (spec 03 §2.5, overrides the plan's "exactly one admin method"). `repository.go` declares 5 `Admin*` interfaces: auth lookups (user by email, session by refresh-token hash, which returns session + user), platform audit append, market-data writes, catalogue writes, platform journal. Who may IMPORT `admin` is an F6 decision.
2. **`Scope.AllowsBuilding` must never validate a stored foreign key.** Under `AllBuildings` it is true for ANY id, including another tenant's. Every stored FK id is validated **in SQL** (company, `deleted_at`, building branch), inside the write statement or the same transaction. This was a proven Critical in 11a: tenant A stored B's building id and then blocked B's own bills.
3. **Tables without `company_id` carry the tenant predicate in their own SQL.** Never a tenant-blind query behind a Go check; never a pre-check outside the transaction. Go pre-checks that *additionally* shadow an in-SQL predicate are accepted as defence in depth. **A guard miss that reports success** (an `:exec` write filtered to zero rows) **is a defect**: use `:one … RETURNING true` or check rows affected.
4. **Cross-tenant proofs use the other tenant's `AdminScope`.** With a narrow Scope the building predicate hides a broken `company_id` predicate (proven twice). Narrow-scope proofs use `Scope` against a same-company building outside the grant.
5. **Id lists:** a REQUIRED positional id list that is empty or nil returns **no rows** (fail-closed, like `Scope.BuildingIDs`). An OPTIONAL filter-struct field that is empty means "no narrowing within the Scope".
6. **Batches:** all-or-nothing. A batch with duplicate natural keys is **refused before the DB** with an `ErrConflict`-matching error naming the key (never a silent arbitrary winner).
7. **NULL `building_id` rows** (company-wide tariff, company-level bill/report, unassigned analyzer, NULL-analyzer alarm events) are visible only to `AllBuildings`. Exception: `TariffRepository.Effective(building)` may resolve to a company-wide tariff once the building is visible. A narrow Scope may not create a building or an unassigned analyzer.
8. **`TimeRange`** is half-open with `Valid()`. Every method taking one returns `ErrInvalidRange` before any DB call. `Latest` also takes a TimeRange (no unbounded hypertable read). `BoundaryReadings` takes `kind`.
9. **Tariff `Effective`:** convert `on` to `Europe/Istanbul` before taking the date; tiebreak `effective_from desc, created_at desc, id desc`.
10. **Secrets:** AES-256-GCM with associated data bound to the row owner; only `Open*` methods decrypt. An update with a nil secret/password **keeps** the stored one; a first insert with none is refused.
11. **Upserts are single atomic statements**, never UPDATE-then-INSERT.
12. **Naming:** sqlc query names and package-wide helpers carry the owning file's aggregate prefix (three parallel branches collided on `distinctUUIDs`).
13. **Seed (Task 12):** only datasets with a table are seeded.
    - 182 legacy emission factors, with `iso_category` validated against the legacy `GHG_ISO_MAPPING`.
    - Integration definitions: iSolar only; the other providers' endpoints live in Mongo.
    - National tariff schedule: the file ships **empty** because the data is Mongo-only. Never invent numbers.
    - Seed writes go only through `admin`.
14. **Task 13 hand value:** hour-0 `active_consumption` = **50** (the plan's "75" is a typo). Materialisation proofs must read materialised data only (`consumption_hourly` is `materialized_only = false`).

## Process rules that paid off (keep them)

- **Prove every guard can fail.** 12 guards in F1 were weaker than their names, including:
  - an isolation test that exercised a Go check instead of the SQL;
  - a no-leak test whose inputs never contained the secret;
  - an aggregate proof that passed with the refresh deleted;
  - scanner fixes that opened new evasions;
  - cross-tenant tests shadowed by a narrow Scope.
- **Reviewers must run their own mutations**, not re-run the implementer's.
- **Trial-merge before merging parallel branches.** A throwaway detached worktree plus iterative `go vet` surfaced the only compile collision early.
- **Carry a review's defect pattern to still-running siblings by message.** It saved whole fix rounds.
- **Fresh cheap agent for small fixes.** Resuming 500k–700k-token transcripts is waste.
- **Subagent stall:** 9 times this session a subagent ended its turn "waiting on a background run". Nudge immediately.
- **Controller crash:** the controller process was killed once (exit 137) with 5 agents and many Docker containers running. Docker and WSL then restarted. If it recurs, keep concurrency at 4.

## Spec defects found by execution (session 1 list continues)

5. **`04-data-model.md` §13 lists five seed datasets, but only three have tables.** ISO 50001 clause texts/templates and a standalone GHG↔ISO 14064 table are not seeded. → product-owner question.
6. **§13 says the factor catalogue is "thousands" of entries.** The legacy source has **182**.

## Open questions for the product owner

- Session 1's five (`02-domain-rules.md` §11): reactive penalty base, sub-9 kW exemption, tiered pricing groups, 2 % missing-hour tolerance, grid emission factor.
- **Blocks F2 (the F2 plan routes around it):** does iSolarCloud report plant-level production without a device serial? `plant_production`'s PK has a NOT NULL `device_id`. The plan holds back and counts plant-level samples and reports them via an operational message.
- **New:** where do ISO 50001 clause texts/templates live? Is a GHG↔ISO mapping table wanted? Where does the national tariff schedule data come from? Should analyzers get a commissioning date (F2 plan)?

## F2 plan — what the reviewer should know

`docs/superpowers/plans/2026-09-15-f2-integration-layer.md`:
- **Structure:** 17 tasks in waves A:1 · B:2–5 · C:6–10 · D:11–15 · E:16 · F:17.
- **Migrations 00012–00013** (PM5340 interval energy + anchor table; OSOS hourly cross-check), both owned by Task 5.
- **Spec-silent rulings R1–R29** in the plan:
  - 30 s request timeout, 3 attempts, 1–30 s jittered backoff, 5 task retries capped at 30 min;
  - a reading more than 15 min in the future is rejected; a register jump >10× the median is rejected;
  - 30-day first-run lookback; an empty window never advances the cursor;
  - newly discovered analyzers are created inactive;
  - `consumption.refresh` is declared but not enqueued (F3); HTTP routes → F6, metrics → F15.
- **Acceptance mapping:** all 11 §F2 acceptance criteria map to named tests.
- **Check at F14 against real responses:** ARIL `WithoutMultiplier` semantics, OSOS multiplier, iSolar error codes, EPİAŞ field names.
- **Precondition:** F1 finished and merged into the F2 branch base.

---

## Key paths

```
docs/rewrite/                                   the specification (04-data-model.md authoritative for F1 DDL)
docs/superpowers/plans/2026-09-10-f1-data-model.md          F1 plan
docs/superpowers/plans/2026-09-15-f2-integration-layer.md   F2 plan (unreviewed)
.superpowers/sdd/2026-09-10-f1-data-model/      git-ignored SDD workspace
  progress.md                   THE LEDGER — read the tail first
  global-constraints.md         constraints block handed to every reviewer
  wave-f-context.md             binding repository rules (read by every repository task)
  wave-f-task-notes.md          per-task notes for 9/10/11a/11b
  wave-f-integration-brief.md   the integration commit brief (next task)
  wave-g-task-notes.md          Task 12b + 13 notes
  legacy-seed-sources.md        legacy data map for Task 12
  task-N-brief.md / -report.md / -fixK-findings.md / review-*.diff
internal/store/repository.go    the contract (34 *Repository interfaces, 5 Admin*)
internal/store/postgres/        repositories; admin/ = only unscoped surface; internal/pgerr = error translation
internal/testfixtures/          NewIsolatedDB, NewTenant, StartPostgres*, StartRedis
internal/seed/                  12a datasets (loader = 12b)
```

## Environment — read before running anything

- **Toolchain in `$HOME/.local`**; shell state does not persist. Prefix every command with `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"`. Go 1.27.1, sqlc v1.30.0, golangci-lint v2.13.2.
- **Docker stops between sessions and on WSL restart.** Verify with `docker ps`. If it is down, ask the user to run `! "/mnt/c/Program Files/Docker/Docker/Docker Desktop.exe" &`.
- **Worktrees:** Agent-tool `isolation: "worktree"` fails on this DrvFs mount ("dubious ownership"). Use:
  ```bash
  git worktree add -b f1/task-N /home/personal/ekokod-wt-tN <base>
  git config --global --add safe.directory /home/personal/ekokod-wt-tN
  ```
  Then dispatch a normal agent told to work there. Merge with `--no-ff`.
- **After any migration or query change: `sqlc generate`.** `make check-generate` runs in `make ci` and catches untracked generated files.
- **Integration tests:** `go test ./... -tags=integration -race -count=1`. Repository tests use `testfixtures.NewIsolatedDB(t)`, so one container per package. Perf tests need `EKOKOD_PERF=1` (`make test-perf`); the 1M-row test takes ~85 s.
- **Container-readiness trap (three layers, session 1):**
  - postgres budget is 180 s via `WithWaitStrategyAndDeadline`;
  - wait strategies REPLACE, never append;
  - a "flaky test" under load is usually a readiness timeout, so read the full failure text.
- **`/mnt/c` breaks `pnpm install` / `next build`.** Use the native mirror at `/home/personal/ekokod-web-native`; do not re-engineer it.
- **The legacy repo** is at `/mnt/c/Users/meren/Desktop/Work/bcem-apps/bcem-energy`, a slow mount. Never search `node_modules`/`.next`/`public`; `timeout` every grep.
- **Commit trailers:** `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` plus the session's own `Claude-Session:` URL. Briefs carry stale URLs, so always override them.

## Decisions already made — do not re-litigate

| Decision | Value |
|---|---|
| Go module | `github.com/MErenTalan/ekokod-rewrite` |
| Binary / env prefix | `ekokod` / `EKOKOD_` |
| Method | `superpowers:subagent-driven-development`, user-approved, parallel cap **5** |
| Phase chaining | each phase branches from the previous phase's tip; no merge to main, no push (user's call) |
| Stop point | stop after F2 is finished; give a handoff prompt |

Session 1's rulings (sqlc reads migrations directly, Timescale shim file, `::numeric` casts, split `materialized_only`, Istanbul day buckets, UUID override, readiness budgets) still stand. See the ledger lines before 759 and git history.
