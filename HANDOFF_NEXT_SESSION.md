# Handoff — ekokod rewrite: F1, F2 and F3 COMPLETE → next phase F4 (tariff and billing engine)

> **STATUS:** F3 is finished. `phase/f3-consumption-engine` is at `8887867`, branched from F2's tip `7dbe1ea`.
> Every task went through per-task review (opus for money/tenancy work), fix rounds, and a two-pass whole-branch final review with fix waves.
> **Nothing has been pushed; `main` is untouched. Push and merge-to-main are the user's call.**
> **F4 has no written plan yet** — the next session writes it first. F4 is the highest-risk phase of the project.

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/` içinde, faz planı
> `docs/rewrite/09-implementation-plan.md`. Toplam 16 faz var (F0-F15).
>
> **Durum: F1, F2 ve F3 tamamlandı.** F3'ün ucu `phase/f3-consumption-engine` branch'inde (`8887867`).
> Hiçbir şey push edilmedi, `main`'e dokunulmadı. Push ve `main`'e merge kararı bana ait, sen yapma.
> **Sıradaki faz F4 (tarife ve faturalandırma motoru) ve henüz planı yok.** Projenin en riskli fazı.
>
> Şu sırayla ilerle:
>
> 1. **`HANDOFF_NEXT_SESSION.md`'i baştan sona oku** (`phase/f3-consumption-engine` üzerinde). Özellikle "START HERE",
>    "F4 must pick up", "Open questions", "Process rules" ve "Environment" bölümleri.
> 2. **F4 planını yaz:** `superpowers:writing-plans` ile, `docs/rewrite/09-implementation-plan.md` §F4 ve ilgili spec
>    bölümlerinden (`02-domain-rules.md` §4-§8, `04-data-model.md`, `06-integrations.md` EPİAŞ, `05-api-contract.md` §6-§7),
>    `docs/superpowers/plans/` altına. Planı yazmadan önce legacy faturalama kodunu oku (`src/app/api/bill/route.ts`,
>    `src/utils/reactivePenalty.ts`) — F3'te legacy okuması üç kural hatasını plan aşamasında yakaladı. "F4 must pick up"
>    listesinin tamamı planda karşılanmalı. Sonra planı opus bir reviewer'a ve ayrı bir sonnet pre-flight isim taramasına
>    ver, bulguları tek bir karar dosyasına yaz, düzelttir.
> 3. **`phase/f3-consumption-engine`'in ucundan `phase/f4-billing-engine` branch'ini aç** ve F4'ü
>    `superpowers:subagent-driven-development` ile yürüt: her görev için taze implementer, ardından task review, gerekirse
>    fix turu. **Bu yöntemi onaylıyorum, sorma, devam et.**
> 4. **Paralellik ve bellek:** aynı anda en fazla 4 agent, Docker'lı test koşan en fazla 2. Agent'lar `make test` /
>    `go test ./...` koşmasın; sadece kendi paketlerini, integration'ı `-parallel 4` ile, `internal/arch`'ı sonda bir kez.
>    Tam suite yalnızca entegrasyon birleşimlerinde, başka Docker işi yokken. Büyük bir dalgadan önce
>    `ps -eo rss,comm | grep java` ile başka projenin Gradle daemon'larına bak; varsa bana sor (F3'te WSL iki kez OOM ile çöktü).
>    - implementer'lar sonnet; para/izolasyon/güvenlik taşıyan task review'ler ve onların re-review'leri opus;
>      küçük test-only fix'lerde re-review koltuğu yerine controller diff'i okuyup bir mutasyonu kendisi koşsun;
>    - implementer'lar testler yeşillenir yeşillenmez commit etsin (mutasyon kanıtlarından önce), reviewer'lar raporu adım adım yazsın.
> 5. **Paralel iş için native-filesystem worktree kullan** (`/home/personal/...`); Agent tool'un `isolation: "worktree"`
>    özelliği bu mount'ta ÇALIŞMIYOR — Environment bölümündeki tarifi kullan.
> 6. **Para kodunda reviewer'dan kaba kuvvet özellik testi iste:** F3'te üç opus re-review, her seferinde yeni bir
>    "işaretsiz yanlış sayı" sınıfı buldu; bitiren şey, rastgele senaryolarda "ya nil+şüphe ya da tam doğru değer"
>    değişmezini tarayan bir özellik testi oldu. Kalıcı testin oracle'ının da mutasyonları yakaladığını controller doğrulasın.
> 7. **Her guard'ın başarısız olabildiğini kanıtlat:** reviewer kendi mutasyonunu koşsun; yeşil kalan mutasyon = Important.
>    Tenant'lar arası izolasyon kanıtları **AdminScope** ile ve pozitif kontrolle.
> 8. **F4 bitince dur**, handoff'u F5 için güncelle ve bana yapıştırılacak prompt ver.
>
> Ortam:
> - Her komuttan önce `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
> - Docker seanslar arası ve WSL yeniden başlayınca kapanıyor; önce `docker ps` ile doğrula. Kapalıysa benden başlatmamı iste.

---

## START HERE — exact state

| Branch | Head | State |
|---|---|---|
| `phase/f1-data-model` | `10d6577` | F1 complete. |
| `phase/f2-integration-layer` | `7dbe1ea` | F2 complete. |
| `phase/f3-consumption-engine` | `8887867` (+ this handoff commit) | F3 complete. Gates at the tip: lint 0, build, check-generate, unit -race ×2 (41 packages), integration ×2 run SEQUENTIALLY per package (18 packages; two Docker-load retries: service/consumption, platform/lock), F3 verification block passes (energy 99 PASS, loadprofile 14, consumption integration 194 PASS / 0 FAIL, TestGolden 9 cases). |
| `f3/*` task and fix branches | — | merged; safe to delete. |

Working copies: the main checkout `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite` still sits on `phase/f1-data-model`;
`/home/personal/ekokod-f3-phase` is the F3 phase worktree. Create F4's branch from `phase/f3-consumption-engine`.
The F3 SDD workspace (git-ignored) is `.superpowers/sdd/2026-09-16-f3-consumption-engine/` in the MAIN checkout.

### What F3 delivered
- `internal/domain/energy` (pure): register/kind/reading/window types; Istanbul buckets (hourly = UTC hour, whole-hour offsets since 1910); `Difference` (§3.1); `SelectBoundary` (binary search); `Derive` (§3.2 reset segmentation with R55/R90-R93); `Ratio`/`Ratios` (nil for zero/negative/nil denominators, `DivisionScale` 20); `MaxDemandOf` + `MaxDemandKindsFor(level)` (R101); `ActivityStatus` (7 days). A permanent fixed-seed property test proves "nil + suspicion, or exactly the true consumption" over random meter swaps and catches every known money mutation; a 9-file golden corpus (`TestGolden`).
- `internal/domain/loadprofile` (pure): weekday/weekend with company weekend days + vacations, meteorological seasons, per-hour means, statistics (population stddev, load factor 0 when max is 0).
- `internal/service/consumption`:
  - `Analytics.Consumption` — continuous aggregates only; composes every unmaterialised month/year from `consumption_daily` as `Partial` (R94); never negative (R102).
  - `Billing.Consumption` — `meter_readings` only; boundaries resolved per boundary INSTANT at Daily/Monthly/Yearly with capped tolerances and telescoping (R96), §3.1 at Hourly; never emits an open or future bucket (R98); request bounds (R99).
  - `Billing.ConsumptionAndRecord` / `ListAnomalies` / `ResolveAnomaly` — suspect-period and `missing_readings` anomalies (R59, R60, R97, R103), dedup under a lock, `reset_registered` re-derives the period in memory before writing, overrides substituted only for exact buckets.
  - Pure helpers `Summarise`, `GenerationRows`, `Balance`, `ExportRows` (suspect registers never rendered as numbers).
  - `Refresher` for `consumption.refresh` (R100).
- `internal/service/loadprofile`: calendar resolution + R87 hourly values (`ActiveImportStart(h+1) − ActiveImportStart(h)`).
- Refresh path: `store.AdminAggregateRepository` (`CALL refresh_continuous_aggregate`, closed view set), `job.NewConsumptionRefreshTask` (debounced id + `ProcessIn`), ingestion enqueues backfills older than 29 days for `load_profile` runs, one global refresh lock, per-view policy-horizon clipping.
- Guards: `internal/service` layer boundaries (transitive), float guard over `./internal/service/...`, R61 reflection guard (Analytics can never hold a reading repository, Billing never an aggregate repository).
- Acceptance: all 8 §F3 criteria have a named in-package acceptance test (`f3_acceptance_test.go`) proven by a production mutation.

### F4 must pick up
1. **Billing only from `Billing` rows, never `Analytics` rows.** Composed/Partial analytics rows are display-only.
2. **Refuse to invoice any period with an unresolved `consumption_anomalies` row** — both `negative_delta`/`meter_reset` suspect periods and `missing_readings` gaps (R97/R103; gaps exist only at Daily/Monthly/Yearly). A billing month must be fully covered by sound rows; a missing row is not zero.
3. **Never bill an open or unsettled period** — `Billing` drops a bucket until `now >= bucket.To + SettleDelay(level)` (R98, R104: Hourly 2 h, Daily 36 h, Monthly/Yearly 72 h), so a month is billable from the 4th; requests are capped at 400 days and `MaxCells = 50 000` analyzer×bucket cells (R99, R104); F4's cut-off-day periods (`billCutoffDay`) are NOT calendar months: decide how a cut-off period maps onto Daily rows (sum of sound Daily rows telescopes within the level) and write the ruling.
4. **Max demand is real now (R65/R85/R101)** — legacy billed demand overrun as structurally zero. Invoices will change; raise with the product owner BEFORE F4 bills anything. Monthly max demand counts `billing` rows (ARIL's monthly maximum).
5. **Ratios are nil for zero consumption (R54)** — the reactive-penalty rule must decide what a nil ratio means (never substitute 1 — that is the legacy defect in `reactivePenalty.ts:47`).
6. **K4:** a row whose two boundaries are of different kinds carries only registers both kinds report; others are nil (unreported, not zero). T1/T2/T3 splits may be nil on such rows.
7. **Superseded gap overrides (R103(3)):** once real readings arrive, the real derivation replaces an operator override — an invoice already issued from the override no longer matches. Re-invoicing is F4's concern.
8. **HTTP is still F6.** F3's services are not constructed by `worker.Build`; F6 wires them and maps `consumption.ErrInvalidRequest`/`loadprofile.ErrInvalidRequest` to 422, resolves `building_id` to analyzer ids (R89), and pages multi-year reports (R99's 400-day cap).
9. Carried from F2 still open: R47 OAuth nonce not session-bound (F6); R48 `ErrConfig` surfacing (F6).
10. **F9 note:** `plant_production_*` continuous aggregates are never refreshed after a backfill (the F3 refresh path covers only the four consumption views).

### Open questions for the product owner
- Q1 ratio zero-case (spec says 0, acceptance says null — F3 implemented null).
- Q3 default weekend days (implemented Saturday+Sunday when none configured).
- **Q4 — F6 BLOCKER:** do calendar events change the weekday/weekend split? (F3: no; `calendar_events` has no non-working flag.)
- Q5 `contracted_power_kw` destination.
- Q6 the analytics hourly chart is all zeros for a 1-hour meter under the spec's aggregate definition.
- Q7 `load_profile` boundary look-back (now capped by R96 at Daily+, unbounded at Hourly per §3.1).
- Q8 uncovered register rollovers become operator work. Q9 a meter swap with no reset row cannot be detected (should a meter-serial change force a suspect period?). Q10 daily-only meters have no analytics series.
- From F2/F1, still open: `plant_production` device-id blocker; ARIL `WithoutMultiplier`; commissioning date; OSOS multiplier / iSolar result codes; reactive penalty base; sub-9 kW exemption; tiered pricing groups; 2 % missing-hour tolerance; grid emission factor; ISO 50001 texts; GHG↔ISO table; national tariff schedule source. **§11's five questions BLOCK F4.**

---

## Binding rulings
F1 rulings 1-14 and F2 rulings R1-R53 still bind. F3 rulings **R54-R104** are in the F3 plan's rulings table
(`docs/superpowers/plans/2026-09-16-f3-consumption-engine.md`), each with "cost if wrong". Several were reversed during
execution — the table holds the final text: I-16 → R93, R88 → R94, R63/R95 → R96, R65 → R101, R97 → R103. R74 (queued
PM5340 recompute) was deferred to F9 and Task 12 withdrawn.

## Process rules that paid off (keep them)
- **Read the legacy code before planning** — it found the real "substituted 1", confirmed no rollover handling existed, and showed max demand was never produced.
- **Pre-flight name scan** (sonnet) in parallel with the plan review (opus): the scan found 7 names that did not exist.
- **Brute-force property tests in re-reviews of money code:** three opus rounds each found a new unflagged-wrong-number class until a 1M-scenario property test closed it; then make the property test permanent AND verify its oracle catches the known mutants (F3's first permanent version missed one because the oracle accepted the mutant's number).
- **Per-boundary-instant resolution** (R96) is what makes adjacent periods telescope — apply the same idea to F4's cut-off-day periods.
- **Controller-verified test-only fixes:** read the diff and run one mutation yourself instead of a re-review seat.
- **Commit-early + incremental reports** survive WSL restarts.
- **Never `git checkout -- <file>` / `git stash` to revert a mutation;** restore from a backup copy or a targeted edit and check `git diff`.

## Environment — read before running anything
- Toolchain in `$HOME/.local`; prefix every command with
  `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
- `C:\Users\meren\.wslconfig`: 12 GB RAM, 4 GB swap, 10 CPUs. Other projects' Gradle daemons can hold 4+ GB — check before big waves.
- Docker stops on WSL restart: check `docker ps`; if down, ask the user to start Docker Desktop.
- Worktrees: `git worktree add -b <branch> /home/personal/<name> <base>` then `git config --global --add safe.directory /home/personal/<name>`.
- Integration tests: `TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <pkgs> -tags=integration -race -count=1 -parallel 4`. **A whole-repo integration run was killed twice for low memory at F3's end** — run the phase-end suite sequentially per package with `GOFLAGS=-p=1 GOMEMLIMIT=1500MiB -parallel 2` (about 10 minutes). `wait until ready` / SQLSTATE 55006 = Docker load: retry that package once.
- `internal/job` integration takes ~150 s (real asynq delays).
- The SDD workspace lives in the MAIN checkout's `.superpowers/sdd/` (git-ignored; it does not propagate to worktrees).
- Commit trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.

## Key paths
```
docs/rewrite/                                                    the specification
docs/superpowers/plans/2026-09-16-f3-consumption-engine.md        F3 plan (rulings R54-R104)
.superpowers/sdd/2026-09-16-f3-consumption-engine/                F3 ledger (progress.md), reviews, reports (main checkout)
internal/domain/energy/  internal/domain/loadprofile/
internal/service/consumption/  internal/service/loadprofile/
internal/store/postgres/admin/aggregates.go  internal/job/consumption.go  internal/ingest/fetch.go  internal/worker/wiring.go
```
