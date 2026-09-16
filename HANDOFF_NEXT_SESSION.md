# Handoff — ekokod rewrite: F1–F4 COMPLETE → next phase F5 (design system)

> **STATUS (2026-09-17):** F4 (tariff and billing engine) is finished on `phase/f4-billing-engine` in `/home/personal/ekokod-f4-phase`. All 12 tasks (0–11) are merged `--no-ff`, each with its own mutation proofs, plus one money and tenancy review of the whole phase.
> **F5's plan is written, reviewed and fixed** on `phase/f5-design-system` (`/home/personal/ekokod-f5-phase`, `36bffca`). No F5 task has started yet. F5 branches from the F3 tip and does not depend on F4.
> **Working mode:** there are no parallel agents or subagents. One session runs one phase inline, writes this handoff at the end, and the user runs `/clear`.
> **Nothing has been pushed and `main` is untouched. Pushing and merging to `main` are the user's call.**

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/` içinde, faz planı
> `docs/rewrite/09-implementation-plan.md` (16 faz, F0–F15).
>
> **Durum:** F1–F4 tamam. F4 `phase/f4-billing-engine` (`/home/personal/ekokod-f4-phase`) üzerinde bitti; gerçek-fatura
> karşılaştırması (§F4 kabul maddesi) ürün sahibinden fatura gelene kadar AÇIK. **Sıradaki faz F5 (design system).**
> F5 worktree'si `/home/personal/ekokod-f5-phase`, branch `phase/f5-design-system` (F3 ucundan açıldı, F4'ten bağımsız);
> planı yazıldı, review edildi, düzeltildi, hiçbir task başlamadı. Hiçbir şey push edilmedi, `main`'e dokunulmadı;
> push ve merge kararı bana ait.
>
> **Çalışma düzeni (kesin):** paralel agent YOK, subagent YOK, Workflow YOK. Fazı bu oturumda tek başına, plana göre
> inline yürüt (`superpowers:executing-plans` + TDD). Review'ları kendin yap: her task sonunda diff'i oku, kendi
> mutasyonunla guard'ın kırmızıya düştüğünü kanıtla; faz sonunda bir kez bütün-faz self-review (erişilebilirlik,
> token/tema tutarlılığı, tasarım kuralları). Faz bitince `HANDOFF_NEXT_SESSION.md`'i F6 için yeniden yaz, bana
> yapıştırılacak promptu ver ve dur.
>
> Şu sırayla ilerle:
> 1. `/home/personal/ekokod-f4-phase/HANDOFF_NEXT_SESSION.md`'i baştan sona oku (START HERE, F4 sonucu, Open questions,
>    Carried forward, Environment).
> 2. Planı oku: `/home/personal/ekokod-f5-phase/docs/superpowers/plans/2026-09-17-f5-design-system.md` (Task 1–8).
>    Plan review edilip düzeltildi; yeniden review ettirme. Tek bilinen kozmetik eksik: T6 "Consumes" satırı Popover,
>    useUiPreferences ve BreadcrumbItem'ı saymıyor — T6'ya gelince düzelt.
> 3. Task 1 → 8 sırasıyla inline yürüt; task branch'lerini aynı worktree'de `git checkout -b f5/task-N` ile aç,
>    bitince `phase/f5-design-system`'e `--no-ff` merge et. F5 bitince dur.
>
> Ortam: her komuttan önce `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
> F5 ağırlıkla `web/` (pnpm, Storybook, Playwright, axe); sadece üzerinde çalıştığın paketleri/test hedeflerini koş,
> asla `make test` / `go test ./...` koşma. Integration testi gerekirse paylaşımlı Postgres: `docker ps` ile
> `ekokod-test-pg` açık mı bak (yoksa `make test-db-up`; Docker kapalıysa benden başlatmamı iste), sonra
> `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <paket> -tags=integration -count=1 -parallel 4`.
> Başka projenin Gradle/java süreçlerine ve 5432 portundaki `dolmusum-dev-postgres-1` container'ına dokunma.
> Playwright/Storybook gibi ağır işlerden önce `ps -eo rss,comm | grep java` ve `free -g` ile belleğe bak.

---

## START HERE — exact state

| Branch / worktree | Head | State |
|---|---|---|
| `phase/f1-data-model` (main checkout `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite`) | `10d6577` | F1 complete |
| `phase/f2-integration-layer` | `7dbe1ea` | F2 complete |
| `phase/f3-consumption-engine` (`/home/personal/ekokod-f3-phase`) | `58597ec` | F3 complete |
| **`phase/f4-billing-engine`** (`/home/personal/ekokod-f4-phase`) | this handoff commit | **F4 complete** (tasks 0–11 merged) |
| `f4/task-0-shared-testdb` (`/home/personal/ekokod-f4-t0`), `f4/task-6` (`/home/personal/ekokod-f4-t6`) | merged | the worktrees can be removed; the other `f4/task-N` branches are merged local branches |
| **`phase/f5-design-system`** (`/home/personal/ekokod-f5-phase`) | `36bffca` | F5 plan ready, no task started |

`ekokod-test-pg` (the shared test Postgres on port 55432, tmpfs) was left running. It is lost when WSL or Docker restarts; recreate it with `make test-db-up`.

## What F4 built
- **`00014_billing_engine.sql`** (the only F4 migration) adds:
  - dated, platform-wide `billing_parameters` (R106): reactive basis and exemptions, reactive bands, tiering groups, mode, voltage levels and supply companies, PTF tolerance, overrun multiplier and `money_rounding_mode`;
  - tariff `power/reactive/distribution_price_source` (R119);
  - `tariff_extra_charges` (R126);
  - bill columns `currency`, `extra_charges_cost`, `demand_data_available`, `ptf_hours_expected` and `consumption_hours_missing`;
  - relaxed PTF CHECK constraints (I-10).

  The store side adds `BillingParameterRepository` (scoped, read-only), `AdminCatalogueRepository.UpsertBillingParameters`, `AdminBillingRepository.BillableBuildings`, and `TariffRepository.ExtraCharges`/`ReplaceExtraCharges`, which are covered by the isolation meta-test.
- **Pure domain packages:**
  - `internal/domain/tariff`: `Validate` (R128, collects every field error), `Resolve`/`ResolveParams` (I-15), `ValidateParams`, and `Price` (hourly consumption-weighted or period-average, with hour-weighted YEKDEM (I-17), manual YEKDEM and R109 tolerance).
  - `internal/domain/reactive`: `Evaluate`.
  - `internal/domain/billing`: `Period`, `DaysInPeriod`, `LatestClosedPeriodKey`, `Compute`, `ToModel`, `Round2` (R112), `AggregateQuantities`/`SumInstalledPower`/`AggregateHours` (R116) and `CombineCompany` (R115).
  - `internal/domain/tariff/icmal`: `Parse`, `DetectNumberFormat`, `ParseNumber`, `NormaliseHeader`, `HeaderScore` and `Analyse` (R129–R133, C-1, C-2, I-12).
- **Services:**
  - `consumption.Billing.PeriodConsumption`/`PeriodConsumptionAndRecord` (R107; window bounds must be Istanbul midnights).
  - `internal/service/billing` (`billingsvc`): `Generate` (analyzer, building and company scope; R113 `ComputeError` codes; supersede/`RecomputeFlagged`; chunking; hourly-detail repair), plus `Get`/`List`/`HourlyDetail`/`Latest`. `jobs.go` holds the job adapters `Dispatcher`, `JobGenerator` and `PDFRenderer`.
  - `internal/service/tariff` (`tariffsvc`): CRUD with validation first, templates, `BulkAssign` (all or nothing), `ReadSheet` (CSV `,`/`;` and XLSX), `Import` (writes no tariff), `GetImport` and `ApplyImport` (R134, I-15).
- **Rendering:** `internal/render` (`FormatMoney`), `render/invoicepdf` (deterministic fpdf output with Go fonts; tr/en labels) and `render/hourlyxlsx`. `internal/arch/render_test.go` guards that render imports no store, service or platform package.
- **Jobs:** `job.TypeBillingDispatch/Generate/RenderPDF` (generate uses a TaskID with no Retention). The scheduler runs `cfg.Schedule.Billing`, and the worker wires the full `BillingDeps`, `billingsvc` and the three adapters.
- **CLI:** `ekokod tool compare-invoice`, with the fixture `testdata/real-invoices/icmal_row_ck_anonymised.json` (CSV line 31 reproduces matrah 820028.45 and KDV 164005.69 to the kuruş).
- **Acceptance:** `internal/domain/billing/f4_acceptance_test.go` (`TestF4Acceptance`) lists every §F4 bullet and the test that proves it.
- **Evidence:** the per-task ledger with every mutation proof and deviation is `.superpowers/sdd/2026-09-17-f4-billing-engine/inline-ledger.md` in the main checkout (git-ignored).

### Deviations from the F4 plan (all recorded in the ledger)
- **`ActiveExportPartial` added:** `reactive.Input` and `billing.Quantities` gain it, because I-18's "at least one reporting member" cannot be derived from the known sum. Partial export on a non-exempt invoice produces `generation_data_missing`.
- **Nil tax rate unrepresentable:** `model.TariffTax.Rate` cannot be nil, so legacy's `?? 1` cannot happen. `TestValidateRejectsNilTaxRate` asserts a negative rate instead. **The F6 DTO must require the field.**
- **Missing base price:** a missing base month excludes only that row's base-dependent samples (energy and reactive KBK), not the whole ETSO group.
- **`tariffsvc` extras:** `Deps` gains `Params` (the overrun multiplier for C-2). `ApplyImport` turns a `kbk` source with a nil coefficient into `fixed`, which keeps the base tariff's column, so copies of fixed tariffs validate.
- **Job adapters location:** the adapters live in `internal/service/billing/jobs.go`, because `internal/job` must not import the store (same seam as `consumption.Refresher`).
- **PDF glyph test:** fpdf's ToUnicode CMap is an identity range, so the glyph test inflates `CIDToGIDMap` and checks each Turkish code point against U+2603 as a control. `invoicepdf.Document.Uncompressed` exists for tests only.
- **Dependencies:** added `github.com/xuri/excelize/v2 v2.10.1` and `github.com/go-pdf/fpdf v0.9.0`, and bumped `golang.org/x/image` to 0.46, `x/crypto` to 0.57 and `x/text` to 0.42.
- **Mutants that stayed green, and why:**
  - `ToLower` vs Turkish case in `NormaliseHeader` is equivalent, because the letter filter drops U+0307.
  - A `time.Now()` PDF creation date is caught by the golden hash, not the byte-stable test, because both renders fall in the same second.
  - Cleanup of the temp PDF on a failed write cannot be reached from a happy-path test.

## Open questions for the product owner (F4 ships the defaults)
- **The real-invoice comparison (§F4 acceptance) is still OPEN.** The PO supplies real invoices across free-market, regulated/Enerjisa, (PTF+YEKDEM)×KBK and custom-power-charge tariffs. Each becomes a `testdata/real-invoices/*.json` fixture, with every diff explained through `compare-invoice`. The supplier's `Fatura Tutarı` (the account-level rounded payable) is not explained and is not an oracle.
- **Dated `billing_parameters` defaults (R106):**
  - reactive basis `whole_quantity`;
  - sub-9 kW exemption;
  - `reactive_exempt_terms={monomial}`, which **the real icmal contradicts** (R118);
  - exempt groups `{residential,lighting}`;
  - tiering groups 8/30, voltage `{lv}`, supply `{incumbent}` (C-3), mode `split_at_threshold` (R121);
  - PTF tolerance 2 %, overrun ×2;
  - `money_rounding_mode=half_up`, although the icmal's CSV line 37 tie rounds half-even (R112).
- **Other PTF-tariff and billing rules:**
  - R119: PTF tariffs default to `kbk` price sources, but the real data shows flat prices.
  - R111: building demand overrun needs a coincident peak, and legacy never billed overrun.
  - R110: mid-period tariff change.
  - R114: automatic re-invoicing after an override (F8).
  - R122: generation double offset.
  - R135: period label for late cut-offs.
  - Transformer loss on OG sites.
- **Still open from F1–F3:**
  - Q1 ratio zero-case, Q3 weekend default, **Q4 calendar events and weekend split (F6 blocker)**, Q5–Q10.
  - F2 items: `plant_production` device id, ARIL WithoutMultiplier, commissioning date, OSOS multiplier, iSolar result codes, grid emission factor, ISO 50001 texts, GHG↔ISO table, national tariff schedule source.

## Carried forward — NOT F5
- **F6 (HTTP) wiring for §6/§7:** handlers, DTOs, RBAC and OpenAPI over `billingsvc` and `tariffsvc`.
  - Map `ComputeError` codes: `tariff_not_found` 422, `no_consumption_data` 422, `unresolved_anomaly` 409, `period_not_closed` 409, `billing_parameters_missing/invalid` 500 (operator), `*tariff.ValidationError` 422 with field codes, `ErrInvalidRequest` 400, `store.ErrNotFound` 404.
  - `/bills/{id}/pdf` renders on demand or serves `pdf_path`; `/bills/{id}/hourly.xlsx` uses `hourlyxlsx.Render`; `/bills/compute` enqueues `billing.generate` with `Force` (M-10).
  - The tariff DTO must require `vat_rate` and every tax `rate`.
- **Later phases:**
  - `/bills/dashboard*` netting goes to F8/F9, the solar tariff service to F9, the national schedule and public calculator to F12.
  - F2 R47/R48 go to F6.
  - F3: `plant_production_*` aggregates are never refreshed after a backfill (F9).
  - F8: drift detection after an override (R114).

## Binding rulings
F1 rulings 1–14, F2 R1–R53, F3 R54–R104 and F4 R105–R135 (in `docs/superpowers/plans/2026-09-17-f4-billing-engine.md` on `phase/f4-billing-engine`), plus the deviations above.

## Process rules that paid off (keep them, inline)
- **Read legacy code and real data before coding domain rules.** The icmal exposed four Critical rule errors in F4's first plan draft.
- **Prove every guard fails with your own mutation, run by script.** Record each mutant's old and new string, restore with a backup, and never use `git checkout -- <file>` or `git stash`. Several F4 mutants first stayed green and each exposed a weak test (the ε rule, VAT on an unrounded base, `(Ek)` label, effective_from, power source) or an oracle blind spot (a generator that bypassed `tariff.Price`, no exact-limit reactive).
- **For money code, pair a permanent property test with an independent oracle,** plus one end-to-end brute force through the service (200 random tariffs, Generate vs Compute).
- **Write cross-tenant proofs with the other tenant's `AdminScope` and a positive control.** Background work uses `store.SystemScope(companyID)`.
- **Keep comments to 1–3 lines** that say WHY and cite ruling ids.
- **Commit as soon as tests are green,** then run mutations, then merge `--no-ff`.

## Environment — read before running anything
- The toolchain lives in `$HOME/.local`. Prefix every command with `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
- WSL has 12 GB RAM and 4 GB swap. **Another project's Gradle/java daemons (dolmusum) hold about 5–6 GB; never kill them.** Check `ps -eo rss,comm | grep java` before heavy suites.
- Docker stops when WSL restarts. Check `docker ps`; if Docker is down, ask the user to start Docker Desktop, then run `make test-db-up`.
- Worktrees must be on the native filesystem (`/home/personal/...`). Working directly in the phase worktree with `git checkout -b` task branches is fine.
- `internal/job` integration takes about 140 s and `internal/scheduler` about 65 s.
- `make check-generate` needs a clean tree for `internal/store/postgres/sqlcgen`, so commit first.
- Commit trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.

## Key paths
```
docs/rewrite/                                                        the specification
docs/superpowers/plans/2026-09-17-f4-billing-engine.md                F4 plan (R105-R135) — on phase/f4-billing-engine
/home/personal/ekokod-f5-phase/docs/superpowers/plans/2026-09-17-f5-design-system.md   F5 plan (next phase)
internal/domain/{tariff,tariff/icmal,reactive,billing}/               F4 pure rules, goldens, property test
internal/service/{billing,tariff}/                                    F4 services + job adapters
internal/render/{invoicepdf,hourlyxlsx}/                              F4 renderers
testdata/real-invoices/                                               compare-invoice fixtures (PO invoices OPEN)
/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite/.superpowers/sdd/2026-09-17-f4-billing-engine/   ledger, legacy read, reviews (git-ignored)
```

## Verification at the F4 tip
Everything below ran sequentially, one package at a time:
- **Domain:** `domain/tariff`, `tariff/icmal`, `reactive` and `billing` pass with `-race`. `TestGolden` passes all 25 cases. The property test passes at `EKOKOD_PROPERTY_N=100000` in 8.6 s.
- **Integration (`-race -parallel 2`):**
  - Passed: `service/billing` 14 s, `service/tariff` 4 s, `service/consumption` 10 s, `job` 137 s, `store/postgres/admin` 4 s, `render/...`, `scheduler` 59 s, `worker` 35 s, `cli` 7 s.
  - `store/postgres` first failed `TestF2IntervalColumnSurvivesCompressedChunk`. That F2 test hard-coded `MigrateDownN(2)`, and 00014 moved "two steps down" above 00012. It now steps down to 00011 by computing from the embedded head (fix commit). After the fix, the whole package passes with `-race` in 60 s.
- **Tool:** `ekokod tool compare-invoice --fixture testdata/real-invoices/icmal_row_ck_anonymised.json` shows a diff of 0.00 on every line.
- **Build checks:** `make lint` reports 0 issues; `make build`, `make check-generate` and `internal/arch` pass.
