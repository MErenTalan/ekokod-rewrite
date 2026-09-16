# Handoff — ekokod rewrite: F1–F3 COMPLETE, F4 (billing engine) IN PROGRESS, F5 plan READY

> **STATUS (2026-09-17):** F4's plan is written, reviewed and fixed. Task 0 is merged. Task 6 is implemented on its own branch but **not reviewed or merged**. Every other F4 task (1–5, 7–11) is not started.
> **Working mode changed:** there are no more parallel or sub agents. One Opus session executes one phase inline, writes this handoff at the end, and the user runs `/clear`.
> **Nothing has been pushed; `main` is untouched. Push and merge-to-main are the user's call.**

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/` içinde, faz planı
> `docs/rewrite/09-implementation-plan.md` (16 faz, F0–F15).
>
> **Durum:** F1–F3 tamam. **F4 (tarife ve faturalandırma motoru) devam ediyor.** Faz worktree'si
> `/home/personal/ekokod-f4-phase`, branch `phase/f4-billing-engine`. Hiçbir şey push edilmedi, `main`'e dokunulmadı;
> push ve merge kararı bana ait.
>
> **Çalışma düzeni (kesin):** paralel agent YOK, subagent YOK, Workflow YOK. Fazı bu oturumda tek başına, plana göre
> inline yürüt (`superpowers:executing-plans` + TDD). Review'ları kendin yap: her task sonunda diff'i oku, kendi
> mutasyonunla guard'ın kırmızıya düştüğünü kanıtla; faz sonunda bir kez para/tenancy odaklı bütün-faz self-review.
> Faz bitince `HANDOFF_NEXT_SESSION.md`'i F5 için yeniden yaz, bana yapıştırılacak promptu ver ve dur.
>
> Şu sırayla ilerle:
> 1. `/home/personal/ekokod-f4-phase/HANDOFF_NEXT_SESSION.md`'i baştan sona oku (START HERE, Task 6 rulings,
>    Next steps, Open questions, Environment).
> 2. Planı oku: `docs/superpowers/plans/2026-09-17-f4-billing-engine.md` (rulings R105–R135, Task 0–11; test isimleri
>    ve beklenen değerler plandadır). Plan spec'e ve gerçek icmal verisine karşı review edilip düzeltildi; yeniden review ettirme.
> 3. **Task 6'yı kapat:** `f4/task-6` branch'i (`/home/personal/ekokod-f4-t6`, commit `6a54ea8` + `68dc877`) review
>    edilmedi. Diff'i `4d54767..68dc877` oku, handoff'taki "Task 6 rulings"i uygula, 1–2 kendi mutasyonunu koş, sonra
>    `phase/f4-billing-engine`'e `--no-ff` merge et.
> 4. Sonra Task 1 → 2 → 3 → 4 → 5 → 7 → 8 → 9 → 10 → 11 sırasıyla inline devam et; F4 bitince dur.
>
> Ortam: her komuttan önce `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
> Integration testleri paylaşımlı Postgres ile: `docker ps` ile `ekokod-test-pg` açık mı bak (yoksa `make test-db-up`;
> Docker kapalıysa benden başlatmamı iste), sonra
> `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <paket> -tags=integration -count=1 -parallel 4`.
> Asla `make test` / `go test ./...` koşma; sadece üzerinde çalıştığın paketleri test et. Başka projenin Gradle/java
> süreçlerine ve 5432 portundaki `dolmusum-dev-postgres-1` container'ına dokunma.

---

## START HERE — exact state

| Branch / worktree | Head | State |
|---|---|---|
| `phase/f1-data-model` (main checkout `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite`) | `10d6577` | F1 complete |
| `phase/f2-integration-layer` (tip `7dbe1ea`) | — | F2 complete |
| `phase/f3-consumption-engine` (`/home/personal/ekokod-f3-phase`) | `58597ec` | F3 complete |
| **`phase/f4-billing-engine`** (`/home/personal/ekokod-f4-phase`) | this handoff commit | Plan `10ae75d` → fixes `4b50539`, `1dfac7a`; Task 0 merged `4d54767` |
| `f4/task-0-shared-testdb` (`/home/personal/ekokod-f4-t0`) | `28e2114` | merged; worktree can be removed |
| **`f4/task-6`** (`/home/personal/ekokod-f4-t6`) | `68dc877` (base `4d54767`) | **implemented, NOT reviewed, NOT merged** |
| `phase/f5-design-system` (`/home/personal/ekokod-f5-phase`) | `36bffca` | F5 plan written, reviewed, fixed; no task started. Branched from F3 tip, independent of F4 |

The shared test container `ekokod-test-pg` (port 55432, tmpfs) was left running. It is lost on a WSL/Docker restart, so recreate it with `make test-db-up`.

### Done in F4 so far
- **Task 0 (merged):** integration tests can share one long-lived Postgres via `EKOKOD_TEST_PG_DSN`, about 2× faster with no container per test binary. The template DB name carries a migration fingerprint, is built under an advisory lock and renamed atomically, and clone names carry a per-process salt. The `StartPostgres*` helpers still boot their own container, because the scheduler's advisory-lock tests are server-wide.
- **Legacy read:** `.superpowers/sdd/2026-09-17-f4-billing-engine/legacy-billing-read.md` in the MAIN checkout (git-ignored). It found a **real supplier icmal** at `/mnt/c/Users/meren/Desktop/Work/bcem-apps/bcem-energy/icmalverileri.csv`. **It contains PII.** Task 5 commits only an anonymised copy (the columns to drop are listed in the plan).
- **Plan review and fix:** 4 Critical and 18 Important findings were accepted and are folded into the plan. The decision file is `plan-fix1-decisions.md` and the review is `plan-review-opus.md`, both in the same SDD folder. You do not need to read them unless a plan line is unclear.
- **Task 6 (`f4/task-6`):** `consumption.Billing.PeriodConsumption` / `PeriodConsumptionAndRecord` over explicit cut-off windows (R107):
  - A billing-kind boundary counts only at Istanbul month-start instants (C-4).
  - Reset resolution works for cut-off windows (I-6).
  - `Row.StartIndexes` and `Row.SpanFrom/SpanTo` added (I-4, I-3).
  - 11 unit tests and 4 integration tests, plus a permanent 1 000-scenario pure brute force against `ConsumptionAndRecord(Monthly)`. 25 mutations turn red; the package is green with `-race`; lint shows 0.
  - Report: `task-6-report.md` in the SDD folder.

### Task 6 rulings to apply while reviewing it
1. `PeriodRequest` validation must require every window bound to be Istanbul midnight. The implementer instead kept sub-day windows and rejected them only at reset time. Add the check and a test.
2. An anomaly window that is exactly a calendar day, hour or year re-derives with that level's rules on reset. Accepted.
3. On reset, the tolerance cap uses the window's own width. Accepted.
4. `MaxCells` cannot be reached for `PeriodRequest` (50×13). Keep the check anyway.
5. `GenerationRows` does not copy `StartIndexes`/`SpanFrom`/`SpanTo`. Add them only if a later task needs them there.

## Next steps (in order, inline)
1. Review and merge Task 6 (see above).
2. **Task 1:** migration `00014_billing_engine.sql`, model and store. This covers billing parameters (including `money_rounding_mode`, `tiering_supply_companies`), tariff price sources, extra charges, relaxed tariff CHECKs, new bill columns, `BillLine*` constants and `AdminBillingRepository.BillableBuildings`.
3. **Tasks 2 → 3 → 4:** pure `domain/tariff`, `domain/reactive` and `domain/billing`, then the golden corpus and the permanent property test. Verify yourself that its oracle catches the listed mutants.
4. **Task 5:** `domain/tariff/icmal` plus the anonymised fixture. Grep the fixture for PII before committing.
5. **Tasks 7 → 8 → 9 → 10 → 11:** the billing service, tariff service and icmal import, render (PDF/XLSX), jobs and wiring, then the compare-invoice tool, the acceptance suite, the verification block and the F5 handoff.
6. Phase-end suite: run package by package, sequentially. Then `make test-db-down` if nothing else needs the container.

## Open questions for the product owner (F4 ships the defaults)
Full list in the plan's "Open questions" section. The headline items:
- **Real-invoice comparison (a §F4 acceptance item) stays OPEN.** The user will supply real invoices across free-market, regulated/Enerjisa, (PTF+YEKDEM)×KBK and custom-power-charge tariffs. F4 ships `ekokod tool compare-invoice` and one fixture built from an anonymised real icmal row.
- Every §11 question is a dated `billing_parameters` row (R106): reactive basis, sub-9 kW exemption, tiering groups, mode, voltage levels and supply companies, missing-hour tolerance, overrun multiplier, rounding mode. The real icmal's tie row rounds half-even while the spec says half-up.
- The monomial reactive exemption is contradicted by real data (R118).
- The real icmal shows flat reactive, power and distribution prices on PTF tariffs (R119).
- Demand overrun is now real; legacy billed zero (R111).
- Also open: mid-period tariff change (R110), the period label for late cut-offs (R135), OG transformer loss.
- Still open from F1–F3: Q1 ratio zero-case, Q3 weekend default, **Q4 calendar events and weekend split (F6 blocker)**, Q5–Q10, plus the F2 items (`plant_production` device id, ARIL WithoutMultiplier, commissioning date, OSOS multiplier, iSolar result codes, grid emission factor, ISO 50001 texts, GHG↔ISO table, national tariff schedule source).

## Binding rulings
F1 rulings 1–14, F2 R1–R53 and F3 R54–R104 (the F3 plan's rulings table) still bind. F4 rulings **R105–R135** are in the F4 plan, plus the Task 6 rulings above.

## Carried forward — NOT F4
- HTTP for §6/§7 is F6. `/bills/dashboard*` netting is F8/F9. The solar tariff service is F9. The national schedule and public calculator are F12.
- F2 R47/R48 (F6). F3: `plant_production_*` aggregates are never refreshed after a backfill (F9).
- F5: its plan's T6 "Consumes" line omits Popover, useUiPreferences and BreadcrumbItem. This is cosmetic; fix it when F5 starts.

## Process rules that paid off (keep them, inline)
- **Read the legacy code AND real data before coding money rules.** The F4 plan's first draft had 4 Critical rule errors that only the real icmal exposed: (Ek) correction rows, a flat power price, tiering on free-market customers, and cut-off windows picking month-start snapshots.
- **Brute-force property tests for money code**, with an oracle proven to catch the known mutants.
- **Per-boundary-instant resolution** (R96/R107) makes adjacent periods telescope.
- Prove every guard fails (your own mutation). Revert with a targeted edit or backup, never `git checkout -- <file>` / `git stash`.
- Cross-tenant proofs use the other tenant's `testfixtures.Tenant.AdminScope` field with a positive control. The store constructor is `store.SystemScope(companyID)`.
- Comments: 1–3 lines, WHY only, cite ruling ids. The F1–F3 code's long narrative comments are not the style to copy.

## Environment — read before running anything
- Toolchain lives in `$HOME/.local`; prefix every command with
  `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
- WSL: 12 GB RAM, 4 GB swap. **Another project's Gradle/java daemons (dolmusum) hold about 6 GB. Never kill them.** Check `ps -eo rss,comm | grep java` before heavy suites.
- Docker stops on a WSL restart. Check `docker ps`; if Docker is down, ask the user to start Docker Desktop, then run `make test-db-up`.
- Worktrees must live on the native filesystem: `git worktree add -b <branch> /home/personal/<name> <base>` then `git config --global --add safe.directory /home/personal/<name>`. Working directly in `/home/personal/ekokod-f4-phase` is fine.
- `internal/job` integration takes about 150 s (real asynq delays).
- Commit trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.

## Key paths
```
docs/rewrite/                                                  the specification
docs/superpowers/plans/2026-09-17-f4-billing-engine.md          F4 plan (rulings R105-R135, Tasks 0-11)
docs/superpowers/plans/2026-09-16-f3-consumption-engine.md      F3 plan (rulings R54-R104)
/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite/.superpowers/sdd/2026-09-17-f4-billing-engine/   ledger, legacy read, reviews, Task 6 report (git-ignored)
/home/personal/ekokod-f5-phase/docs/superpowers/plans/2026-09-17-f5-design-system.md           F5 plan (next phase)
internal/service/consumption/  internal/store/repository.go  internal/domain/model/  internal/testfixtures/containers.go
```
