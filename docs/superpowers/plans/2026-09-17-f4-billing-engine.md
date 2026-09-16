# F4 — Tariff and Billing Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An invoice for an analyzer, a building or a company is computed from invoice-grade consumption, a dated tariff and locally stored market prices by one set of pure, golden-locked functions. No invoice is issued over a suspect, missing, unsettled or unpriced period. Every configurable regulatory rule that the product owner has not confirmed is a dated parameter, not a constant.

**Architecture:** Three pure packages carry every rule: `internal/domain/tariff` (validation, resolution, PTF+YEKDEM pricing, dated billing parameters), `internal/domain/reactive` (the penalty) and `internal/domain/billing` (period, invoice assembly, rounding, building/company aggregation). `internal/domain/tariff/icmal` parses supplier summaries and derives coefficients. The services add I/O around them. `internal/service/consumption` gains an explicit-window billing entry point for cut-off periods. `internal/service/billing` generates and persists bills. `internal/service/tariff` handles tariff CRUD, templates and icmal import. `internal/render` produces PDF and XLSX. Two asynq tasks run it all in the background. HTTP stays in F6.

**Tech Stack:** Go 1.27.1 · shopspring/decimal v1.4.0 · pgx/v5 · sqlc v1.30.0 · goose v3.28.0 · asynq v0.26.0 · testify · testcontainers-go v0.44.0 · **new:** `github.com/go-pdf/fpdf v0.9.0`, `golang.org/x/image/font/gofont` (Go fonts: Turkish glyphs, BSD licence, no font file to vendor), `github.com/xuri/excelize/v2 v2.10.1`

**Spec:** `docs/rewrite/09-implementation-plan.md` §F4 (binding scope, acceptance, verification). `docs/rewrite/02-domain-rules.md` §1, §4–§8, §11 (authoritative for rules, amended by this plan's rulings where real data contradicts them). `docs/rewrite/04-data-model.md` §4.4, §4.6, §5, §6. `docs/rewrite/05-api-contract.md` §6, §7. `docs/rewrite/06-integrations.md` §7. `docs/rewrite/10-removed-behaviours.md` items 1–11, 24. Legacy evidence: `.superpowers/sdd/2026-09-17-f4-billing-engine/legacy-billing-read.md` in the main checkout (cited below as **LBR** with its section numbers).

---

## Global Constraints

Every task's requirements implicitly include this section.

**Authority**
- `02-domain-rules.md` is authoritative for rules, `04-data-model.md` for columns, and the migrations on disk for built schema. **Where a spec rule disagrees with a §F4 acceptance criterion, the acceptance criterion wins.** **Where a spec rule is contradicted by the real supplier icmal (LBR §G.1) and the product owner has not confirmed it, the rule becomes a dated billing parameter whose default is the spec value** (user decision 2026-09-16: §11 questions become configurable). Every such case is in the rulings table. An implementer who finds a new gap stops and reports it. Do not improvise.
- F1–F3 rulings (F1 1–14, R1–R104) still bind. F4 rulings continue at **R105**.

**Purity and layering**
- `internal/domain/...` imports nothing outside `internal/domain` and does no I/O: no `context`, no `time.Now()`, no logger. The instant, the `*time.Location`, parameters and market data are arguments. `internal/domain/billing` may import `internal/domain/energy`, `internal/domain/tariff`, `internal/domain/reactive` and `internal/domain/model`. The existing `internal/arch` guard enforces this.
- `internal/render/...` is new. It may import `internal/domain/...` and third-party rendering libraries. It must not import `internal/store`, `internal/service` or `internal/platform`. Task 9 adds the guard.
- **No HTTP in F4 (R105).** Service methods back every `05 §6/§7` endpoint. Handlers, DTOs, RBAC and OpenAPI are F6.

**Money, energy and time**
- Money, energy, prices, ratios and coefficients are `decimal.Decimal`. **`float64` in any F4 path is a defect.** Task 1 adds `./internal/domain/tariff/...`, `./internal/domain/reactive/...`, `./internal/domain/billing/...` and `./internal/render/...` to `internal/arch` `floatGuardPatterns` and to `.golangci.yml` `no-float-money` (change both or neither).
- **Rounding (R112, proven exact on every row of the real icmal):** intermediate values are never rounded. An invoice **line amount** is `round2(quantity × unit price)` half-up, computed from the unrounded quantity and unrounded price. A named tax is `round2(energy_cost × rate / 100)`, where `energy_cost` is the sum of the rounded energy lines. `vat_base` is the sum of the rounded lines. `vat = round2(vat_base × vat_rate / 100)`. `total = max(0, vat_base + vat − generation_credit)`, where `generation_credit` is itself a rounded line. Division is carried to `DivisionScale = 20`. Use `decimal.Decimal.Round(2)`: shopspring rounds half away from zero, which is half-up for the non-negative amounts. A negative line (a credit) is rounded on its absolute value and then negated. Test that explicitly.
- **Unavailable is `nil`, never zero.** A nil register, a nil coefficient, a nil max demand and a nil ratio each mean "not known". Zero means "measured zero".
- Timestamps are stored in UTC and evaluated in `Europe/Istanbul` through the tz database. Packages that need the zone import `time/tzdata`.

**Correctness guards**
- **Every guard is proven to fail.** Each test that guards a rule names the mutation that turns it red, and the task report records the red output. A mutation that stays green is an Important finding.
- **Brute-force property test for money code (F3 lesson).** Task 4 lands a permanent fixed-seed property test over `billing.Compute`. The reviewer of Tasks 3, 4 and 7 runs a ≥100 000-scenario version, and the controller verifies that the permanent test's oracle catches the listed mutants.
- **Cross-tenant proofs use the other tenant's `AdminScope` with a positive control** (F1 ruling 4).

**Store and migrations**
- F4 adds exactly **one** migration, `00014_billing_engine.sql` (Task 1). Any other schema need stops and reports.
- Run `make generate` after any query change and commit the output. `make check-generate` is part of the gate.
- Integration tests use `testfixtures.NewIsolatedDB(t)` / `NewTenant` / `StartRedis` / `DiscardLogger`. **Once Task 0 is merged, export `EKOKOD_TEST_PG_DSN` to the shared container** (`make test-db-up`). A `wait until ready` failure or SQLSTATE `55006` is Docker load: retry the package once.

**Speed rules (user directive 2026-09-16: whole app this week, no loss of quality)**
- **Tests while iterating:** `go test ./<own pkg> -run <TestName>` without `-race`. **Pre-commit gate:** the task's own packages once with `-race`, plus `golangci-lint run --concurrency 2 ./<own pkgs>/...`. **Never** `make test`, `go test ./...` or another task's packages. `internal/arch` runs only in tasks that change a guard.
- **Commit as soon as tests are green**, before the mutation proofs. Commit the proofs afterwards.
- **Comments:** one to three lines, and only to say WHY. No ruling histories, no "Task N did X" narration. Cite a ruling id (`// R108:`) instead of restating it. Doc comments on exported identifiers stay, and stay short.
- **No placeholder lines on invoices.** A charge that does not apply produces no line, except where a test below names a zero line.

**Process**
- Phase branch `phase/f4-billing-engine` (from `phase/f3-consumption-engine@58597ec`). Task branches `f4/task-N` in native worktrees:
  ```bash
  git worktree add -b f4/task-N /home/personal/ekokod-f4-tN phase/f4-billing-engine
  git config --global --add safe.directory /home/personal/ekokod-f4-tN
  ```
  Merge with `git merge --no-ff`. The Agent tool's `isolation: "worktree"` does not work on this mount.
- Environment prefix for every command:
  `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`
- Commit trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`

---

## Rulings (R105–R135)

Each row gives the question, the ruling, and the cost if the ruling is wrong. "PO" marks a question the product owner confirms later. Its default is what ships.

| Id | Question | Ruling | Cost if wrong |
|---|---|---|---|
| R105 | §F4 scope says "API endpoints from §6 and §7" | Service methods for every §6/§7 endpoint. HTTP is F6 (same as F3's R77). `/bills/dashboard*` netting needs F9 plant data and goes to F8/F9. Solar tariffs and the public national schedule go to F9/F12. `/buildings/bulk-tariff/history` has no table: F8 derives it from `tariffs.created_at` | F6 wires more |
| R106 | §11 Q1–Q4 are unconfirmed | They become a **dated, platform-wide `billing_parameters` row** (Task 1). The row resolves by the period start, like a tariff. Defaults: reactive basis `whole_quantity` (legacy + icmal), exempt below 9 kW (spec), tiering groups `{residential: 8, commercial: 30}` (spec/legacy), missing-hour tolerance 0.02 (spec). The PO changes a value by inserting a new dated row. PO | none: config |
| R107 | Cut-off periods are not calendar months (F3 carry-forward 3) | `consumption.Billing.PeriodConsumption` takes explicit contiguous windows. Each window is resolved like a Monthly bucket (billing → load_profile → daily precedence), per boundary instant (R96), so adjacent periods telescope. It settles 72 h after `To`. For cut-off day 1 the result is **identical** to `Consumption` at Monthly (tested) | wrong kWh at non-1 cut-offs |
| R108 | Hourly PTF with a gap, generation netting and a non-matching hourly sum | Quantity is always the period's net consumption from R107. The hourly method yields a **consumption-weighted unit price** `Σ(kwh_h × unit_h) / Σ kwh_h` over matched hours, and `energy_cost = net × that price`. Hours are matched only where a sound single-hour Billing row **and** a PTF exist. `hours_missing = expected − matched`, split into price-missing and consumption-missing. Spec §7.1's `Σ kwh_h × unit_h` equals this exactly when nothing is missing and there is no generation. This avoids legacy's zero-billing of missing hours (removed-behaviour 8) and legacy's gross-kWh energy with net-kWh distribution (LBR E.2 #12) | energy line off by the gap-weighted price difference |
| R109 | Missing-hour tolerance breached, or YEKDEM missing for a needed month | The bill is computed with what exists and persisted as `flagged` with `flag_reason = ptf_data_missing`. It is never `issued`. Prices are never substituted from another period (removed-behaviour 10) | none |
| R110 | Which tariff applies to a period (spec §4 "date D") | D = the period's start **date** in Istanbul. No proration on a mid-period tariff change. Legacy used the 28th of the month (LBR A.1). PO | wrong tariff for a mid-period change |
| R111 | Max demand for a cut-off window, and for a building | Analyzer: `Row.MaxDemandKw` from R107. `billing`-kind peaks count only when the window is exactly an Istanbul calendar month (they are month peaks). Building: the member's value when there is exactly one member, otherwise **nil** (a coincident peak cannot be built from per-meter maxima; summing maxima would over-bill). Nil means an overrun of 0 and `demand_data_available = false`. PO: legacy never billed an overrun (R85), so invoices change | building overrun not billed |
| R112 | Rounding | See Global Constraints. Every positive row of the real icmal satisfies it with 0.00 residual (LBR §G.1) | kuruş drift |
| R113 | What is a failure and what is a flag | **Not persisted** (`ComputeError.Code`): `tariff_not_found`, `no_consumption_data` (a member has no Billing row for the window, or a required register is nil: active import always; T1–T3 on a multi-time tariff), `unresolved_anomaly`, `period_not_closed`, `billing_parameters_missing`. **Persisted as `flagged`:** `ptf_data_missing`, `reactive_data_missing`, `installed_power_missing`, `generation_data_missing`. Clean → `issued` | an operator sees a flagged bill rather than a job error |
| R114 | Recompute and supersede | `Generate` with an existing live bill and `Force=false` returns that bill unchanged (`Created=false`). With `Force=true` it supersedes (`BillRepository.Supersede`). The scheduled job never forces. Automatic detection of drift after an override (F3 carry-forward 7) is F8: the operator recomputes with force. PO | a stale invoice stays until forced |
| R115 | Company invoice | Each building is computed in memory with its own tariff, window and parameters (`Generate` for scope company does not read stored building bills). The company bill sums quantities and charges and merges lines by `(code, rate_pct)`. VAT is the sum of building VATs, not recomputed. `tariff_id` is null. Members are every analyzer of every building. Legacy summed per-analyzer bills instead (LBR E.2 #23) | company totals ≠ building totals by kuruş |
| R116 | Building aggregation | Registers are summed per register. A register nil in any member is nil in the aggregate (K4). Installed power is the sum of members' `installed_power_kw`, nil if any member is nil. The hourly series per hour is the sum when **every** member has that hour, otherwise the hour is consumption-missing | reactive band from a partial sum |
| R117 | Reactive for zero net consumption (F3 carry-forward 5) | The displayed ratio stays nil (R54). The limit test uses spec §6.6's ε form: net = 0 with reactive > 0 counts as exceeded, and net = 0 with reactive = 0 does not. Nothing ever substitutes 1 (removed-behaviour 15) | penalty on a zero-consumption month |
| R118 | Monomial reactive exemption: the icmal shows monomial rows penalised (LBR E.2 #2) | Parameter `reactive_exempt_terms`, default `{monomial}`, because an acceptance criterion names the monomial exemption. PO, with the data evidence attached | real penalties zeroed until PO flips it |
| R119 | Reactive, power and distribution unit prices on PTF tariffs: the icmal shows flat regulated prices (2.6455 TL/kVArh; power TL/kW unrelated to PTF), while spec §7.2 derives them from KBK | New tariff columns `power_price_source`, `reactive_price_source`, `distribution_price_source` (`kbk` or `fixed`), default `kbk` (spec). `kbk` means `base × coefficient`, or for distribution `kbk_distribution_cost_tl_per_kwh`. A nil coefficient means **no line**, with no fallback (acceptance). `fixed` means the tariff's own column. This is an explicit choice, not a fallback | PTF-indexed penalty where the regulation is flat |
| R120 | Tiering on PTF tariffs | Never. Kademe is a regulated-tariff mechanism. With the acceptance rule "null coefficient → zero line", a commercial PTF customer above 30 kWh/day would get its high tier free. `kbk_overuse_price` is stored but unused by billing. Tiering also requires `price_type = single_time`, `voltage_level ∈ params.tiering_voltage_levels` (default both, spec silent; the legacy calculator tiered only AG: PO) and `user_group ∈ params.tiering_groups` | a regulated PTF+kademe contract, if one exists |
| R121 | Tiering mode (legacy calculator switched the whole consumption; engine split it) | Parameter `tiering_mode`, default `split_at_threshold` (spec). PO | large difference for tiered customers |
| R122 | Generation offset subtracted twice from the low tier (spec copies legacy, LBR E.2 #28) | Spec formula kept verbatim. PO question attached | low tier undersized for net-metering |
| R123 | Multi-time tariff with `subtract_from_consumption` (spec silent) | Each band is scaled by `net / active_import` (pro rata), so the bands sum to net. With `active_import = 0` the bands are 0 | band allocation |
| R124 | Multi-time PTF tariff and the hourly method | The hourly method applies only to `single_time` PTF tariffs. Multi-time PTF tariffs use the period-average `base × kbk_tN` band prices. A nil `kbk_tN` means no line for that band **and** a `tariff_invalid` validation error at creation (a multi-time PTF tariff must carry all three) | TOU PTF customers |
| R125 | Demand overrun multiplier (hard-coded ×2) and overrun without a contract | Parameter `demand_overrun_multiplier`, default 2. A binomial tariff must carry both `contracted_power_kw` and `power_unit_price`, and a monomial tariff neither. Validation rejects otherwise (legacy charged the whole max demand ×2 when the contract was unset, LBR E.2 #25) | custom overrun contracts → extra charges |
| R126 | Custom tariffs with additional power or fixed charges (PO statement 2026-09-16) | New table `tariff_extra_charges(name, basis, amount)`, basis ∈ `per_kwh` (× net), `per_contracted_kw`, `per_max_demand_kw` (nil demand → no line, `demand_data_available=false`), `fixed_per_period`, `pct_of_energy`. Each is its own line `extra:<name>`, inside the VAT base, before named taxes. Taxes stay % of energy | contract shapes beyond these five |
| R127 | Currency | A PTF tariff must be `TRY`. Other tariffs bill in their own currency, with no FX. New `bills.currency` column. The PDF prints the currency | USD contracts |
| R128 | Tariff without `vat_rate` (acceptance) | Service input carries `VatRate *decimal.Decimal`. `tariff.Validate` rejects nil with field error `vat_rate` before any I/O. Validation also rejects: a nil tax rate (legacy `?? 1`), a single-time tariff without a price, a multi-time tariff missing a band price, a PTF tariff without `kbk_energy`, a `subtract_from_total` tariff without a generation price, and R124/R125/R127 | rejected tariffs |
| R129 | İcmal numbers: spec says Turkish format, the real file is US format (`"53,337.25"`) | Per-file detection over every numeric cell: `^\d{1,3}(\.\d{3})*,\d+$` means Turkish, `^\d{1,3}(,\d{3})*\.\d+$` means US. Both seen is an error. Neither: Turkish (spec). Delimiter `,` or `;` is detected from the header row | misparsed coefficients |
| R130 | İcmal reversals are negative rows, not cancelled flags (LBR E.2 #17) | Rows flagged cancelled are excluded (spec). All other rows are **summed signed per (ETSO, period)**. A group whose total kWh ≤ 0 is dropped with a warning | KBK from a reversed period |
| R131 | İcmal other-tax derivation: spec §8.2 divides by the VAT base, but §6.7 applies taxes to energy, and the icmal's BTV is exactly 5 % (or 1 %) of energy | Each non-zero tax column yields a named tax `rate = amount / energy_charge × 100` (BTV, Enerji Fonu, TRT), with median and stability like coefficients. No `other_taxes_rate` (removed-behaviour 6) | tax mis-derived |
| R132 | İcmal power and reactive derivation: `power_price_kbk = power/(demand × base)` is dimensionally meaningless (LBR E.2 #5) | Derive `power_unit_price = power_charge / demand_kw` (TL/kW, fixed source) and `reactive_unit_price` (TL/kVArh, fixed source). Also report `reactive_kbk = reactive_unit_price / base` for operators who choose `kbk`. For reactive, a group with only inductive > 0 uses inductive, only capacitive > 0 uses capacitive, both > 0 is skipped with warning `reactive_register_ambiguous` | power KBK unavailable |
| R133 | İcmal time type: every real row says "Tek Zamanlı" yet carries T1–T3 kWh | `is_multi_time` comes from the "TekZaman/Üç Zaman" column (`Tek` → false, `Üç` → true). Nil when the column is absent. Never inferred from T1–T3 > 0 (LBR E.2 #16) | wrong tariff type |
| R134 | İcmal apply | Nothing is written without an explicit per-building confirmation carrying `effective_from` (never the import date, LBR E.2 #21). Apply copies the building's applicable tariff at `effective_from` (or a full base definition supplied in the confirmation when none exists), sets `use_ptf_yekdem`, the derived coefficients and prices with their sources per R119/R132, and replaces taxes with the derived named taxes. It creates a **new tariff version** through `tariff.Validate` | none |
| R135 | Period label for late cut-off days (the supplier labels 31-10→30-11 as 202511; spec §5 labels it 2025-10) | Spec §5 kept. PO question | label only |

### Open questions for the product owner (F4 ships the default)
1. R106 defaults; R118 monomial exemption vs icmal evidence; R119 flat reactive/power/distribution prices on PTF tariffs; R121 tiering mode; R120 tiering voltage levels.
2. R111: demand overrun is now real (legacy billed zero), building overrun needs a coincident peak.
3. R110 mid-period tariff change; R114 automatic re-invoicing after an override; R122 generation double offset; R135 period label.
4. Transformer loss on OG sites (`Trafo Kaybı T0 Kwh` is included in the supplier's total kWh and in the reactive denominator exclusion, LBR E.2 #4, #35): not modelled in F4.
5. **Real-invoice comparison (§F4 acceptance) stays OPEN:** the user supplies real invoices across all tariff kinds (free-market, regulated/Enerjisa, (PTF+YEKDEM)×KBK, custom power charges). F4 ships `ekokod tool compare-invoice` and one fixture built from an anonymised real icmal row.

---

## Execution waves and dependencies

At most **3 agents** at once (foreign Gradle daemons hold ~6 GB). At most **2** run integration tests, and only against the shared container once Task 0 is merged.

| Task | Depends on | Docker | Review |
|---|---|---|---|
| 0 Shared test Postgres | — (running) | yes | controller |
| 1 Migration 00014, model, store | 0 merged | yes | sonnet (+AdminScope proofs) |
| 2 `domain/tariff` + `domain/reactive` | 1's model types (start on the speculative base `f4/task-1` once its model commit lands) | no | **opus** + brute force |
| 3 `domain/billing`: period and analyzer invoice | 2 | no | **opus** + brute force |
| 4 Aggregation, golden corpus, permanent property test | 3 | no | **opus** |
| 5 `domain/tariff/icmal` + anonymised fixture | 1's model types | no | sonnet |
| 6 `consumption.Billing.PeriodConsumption` | — | yes | **opus** |
| 7 `service/billing` | 1, 3, 6 (4 for aggregation) | yes | **opus** |
| 8 `service/tariff` (CRUD, templates, bulk, icmal import) | 1, 2, 5 | yes | sonnet |
| 9 `render`: PDF + hourly XLSX | 1 | no | sonnet |
| 10 Jobs, scheduler, worker wiring | 7, 9 | yes | sonnet |
| 11 compare-invoice tool, acceptance suite, verification, handoff | all | yes | controller |

Start order: **6 immediately** (independent). Then **1** when Task 0 merges. **2 and 5** go on 1's model commit. After that, each task is dispatched as soon as its dependencies are review-clean (speculative bases allowed, then rebase at merge).

## File ownership (parallel tasks never edit the same file)
- Task 1: `internal/store/postgres/migrations/00014_billing_engine.sql`, `internal/domain/model/{tariffs,bills,billing_parameters,enums}.go`, `internal/store/repository.go`, `internal/store/postgres/{tariffs,bills,billing_parameters}.go`, `internal/store/postgres/queries/*`, `sqlcgen/*`, `internal/store/postgres/admin/catalogue.go`, `internal/arch/*`, `.golangci.yml`.
- Task 6: `internal/service/consumption/{period.go,period_test.go,period_integration_test.go}` and the minimal refactor inside `billing.go` / `anomalies.go`.
- Tasks 2–5: only their own package directories. Task 9: `internal/render/...` plus one arch guard test file `internal/arch/render_test.go`. Task 10: `internal/job/billing*.go`, `internal/scheduler/scheduler.go`, `internal/worker/wiring.go`, `internal/platform/config/*` (only if a new key is needed). Task 11: `internal/cli/tool*.go`, `testdata/real-invoices/`, `internal/domain/billing/f4_acceptance_test.go`, handoff.

---

## Task 1: Migration 00014, model and store

**Files:**
- Create: `internal/store/postgres/migrations/00014_billing_engine.sql`, `internal/domain/model/billing_parameters.go`, `internal/store/postgres/billing_parameters.go`, `internal/store/postgres/billing_parameters_integration_test.go`, `internal/store/postgres/migrations_billing_engine_integration_test.go`
- Modify: `internal/domain/model/{tariffs.go,bills.go,enums.go}`, `internal/store/repository.go`, `internal/store/postgres/{tariffs.go,bills.go}` + their integration tests, `internal/store/postgres/queries/{tariffs,bills,billing_parameters}.sql`, `internal/store/postgres/admin/catalogue.go` (+ test), `internal/store/postgres/scope_isolation_integration_test.go`, `internal/arch/arch_test.go` (float guard list), `.golangci.yml`.

**Interfaces — Produces:**
```go
// model/enums.go
type ReactivePenaltyBasis string // "whole_quantity" | "excess_over_limit"
type TieringMode string          // "split_at_threshold" | "whole_consumption_switch"
type PriceSource string          // "kbk" | "fixed"
type ExtraChargeBasis string     // "per_kwh" | "per_contracted_kw" | "per_max_demand_kw" | "fixed_per_period" | "pct_of_energy"
// each with a Valid() bool method and exported consts, following the file's existing enum pattern.

// model/billing_parameters.go
type ReactiveBand struct {
	MinKw      decimal.Decimal
	MaxKw      *decimal.Decimal // nil = unbounded
	Inductive  decimal.Decimal
	Capacitive decimal.Decimal
}
type BillingParameters struct {
	EffectiveFrom               time.Time
	ReactivePenaltyBasis        ReactivePenaltyBasis
	ReactiveExemptBelowKw       *decimal.Decimal // nil = no size exemption
	ReactiveExemptTerms         []TariffTerm
	ReactiveExemptUserGroups    []DistributionUserGroup
	ReactiveGenerationExemptKwh decimal.Decimal
	ReactiveBands               []ReactiveBand
	TieringGroups               map[DistributionUserGroup]decimal.Decimal // group → kWh/day threshold
	TieringMode                 TieringMode
	TieringVoltageLevels        []VoltageLevel
	PTFMissingHourTolerance     decimal.Decimal // fraction, 0..1
	DemandOverrunMultiplier     decimal.Decimal
	CreatedAt                   time.Time
}

// model/tariffs.go — Tariff gains:
	PowerPriceSource        PriceSource
	ReactivePriceSource     PriceSource
	DistributionPriceSource PriceSource
type TariffExtraCharge struct {
	ID        uuid.UUID
	TariffID  uuid.UUID
	Name      string
	Basis     ExtraChargeBasis
	Amount    decimal.Decimal // TL per basis unit, or percent for pct_of_energy
	SortOrder int16
}

// model/bills.go — Bill gains:
	Currency                CurrencyCode
	ExtraChargesCost        decimal.Decimal
	DemandDataAvailable     bool
	PtfHoursExpected        *int32
	ConsumptionHoursMissing *int32

// store/repository.go
type BillingParameterRepository interface {
	// Effective returns the row with the greatest effective_from ≤ on (date), or ErrNotFound.
	// Platform-wide table: the Scope is validated and narrows nothing (PriceRepository's rule).
	Effective(ctx context.Context, s Scope, on time.Time) (model.BillingParameters, error)
	List(ctx context.Context, s Scope) ([]model.BillingParameters, error)
}
// TariffRepository gains:
	ExtraCharges(ctx context.Context, s Scope, tariffID uuid.UUID) ([]model.TariffExtraCharge, error)
	ReplaceExtraCharges(ctx context.Context, s Scope, tariffID uuid.UUID, charges []model.TariffExtraCharge) ([]model.TariffExtraCharge, error)
// AdminCatalogueRepository gains:
	UpsertBillingParameters(ctx context.Context, p model.BillingParameters) (model.BillingParameters, error)
```

**Migration content (exact):**
```sql
-- +goose Up
create type reactive_penalty_basis as enum ('whole_quantity','excess_over_limit');
create type tiering_mode as enum ('split_at_threshold','whole_consumption_switch');
create type price_source as enum ('kbk','fixed');
create type extra_charge_basis as enum ('per_kwh','per_contracted_kw','per_max_demand_kw','fixed_per_period','pct_of_energy');

create table billing_parameters (
    effective_from                 date primary key,
    reactive_penalty_basis         reactive_penalty_basis not null default 'whole_quantity',
    reactive_exempt_below_kw       numeric(12,3) default 9,
    reactive_exempt_terms          tariff_term[] not null default '{monomial}',
    reactive_exempt_user_groups    distribution_user_group[] not null default '{residential,lighting}',
    reactive_generation_exempt_kwh numeric(18,6) not null default 1,
    reactive_bands                 jsonb not null,
    tiering_groups                 jsonb not null,
    tiering_mode                   tiering_mode not null default 'split_at_threshold',
    tiering_voltage_levels         voltage_level[] not null default '{lv,mv}',
    ptf_missing_hour_tolerance     numeric(6,5) not null default 0.02
        check (ptf_missing_hour_tolerance between 0 and 1),
    demand_overrun_multiplier      numeric(8,4) not null default 2 check (demand_overrun_multiplier >= 0),
    created_at                     timestamptz not null default now()
);
insert into billing_parameters (effective_from, reactive_bands, tiering_groups) values (
    '2000-01-01',
    '[{"min_kw":"9","max_kw":"30","inductive":"0.33","capacitive":"0.20"},{"min_kw":"30","max_kw":null,"inductive":"0.20","capacitive":"0.15"}]',
    '{"residential":"8","commercial":"30"}');

alter table tariffs
    add column power_price_source        price_source not null default 'kbk',
    add column reactive_price_source     price_source not null default 'kbk',
    add column distribution_price_source price_source not null default 'kbk';

create table tariff_extra_charges (
    id         uuid primary key default gen_random_uuid(),
    tariff_id  uuid not null references tariffs(id) on delete cascade,
    name       text not null,
    basis      extra_charge_basis not null,
    amount     numeric(18,6) not null,
    sort_order smallint not null default 0
);
create index on tariff_extra_charges (tariff_id);

alter table bills
    add column currency                  currency_code not null default 'TRY',
    add column extra_charges_cost        numeric(18,4) not null default 0,
    add column demand_data_available     boolean not null default true,
    add column ptf_hours_expected        integer,
    add column consumption_hours_missing integer;

-- +goose Down
alter table bills drop column consumption_hours_missing, drop column ptf_hours_expected,
    drop column demand_data_available, drop column extra_charges_cost, drop column currency;
drop table tariff_extra_charges;
alter table tariffs drop column distribution_price_source, drop column reactive_price_source, drop column power_price_source;
drop table billing_parameters;
drop type extra_charge_basis; drop type price_source; drop type tiering_mode; drop type reactive_penalty_basis;
```
JSON decimals are strings (05 §1). Decode them with `decimal.NewFromString`, never through `float64` (the float guard catches this).

- [ ] **Step 1: Failing tests first.**
  - `TestMigration00014RoundTrips`: up → down → up on `NewEmptyDB`, and the seeded row exists after up.
  - `TestBillingParametersEffectivePicksGreatestNotAfter`: rows 2000-01-01 (seed) and 2026-01-01 → `Effective(2025-12-31)` = seed, `Effective(2026-01-01)` = new. Mutation: `<=` → `<`.
  - `TestBillingParametersDecodeBandsAndGroupsExactly`: bands and groups round-trip to exact decimals (`0.33` ≠ `0.3300000001`). Mutation: decode through `float64` → the float guard or this test red.
  - `TestUpsertBillingParametersIsIdempotentOnEffectiveFrom`.
  - `TestTariffPriceSourcesRoundTrip` and `TestTariffExtraChargesReplaceAndRead` (replace twice → only the second set).
  - `TestReplaceExtraChargesRefusesAnotherTenantsTariff`: tenant B's `AdminScope` gets `ErrNotFound`, with a positive control (tenant A's own call succeeds). Mutation: drop the tariffs join predicate.
  - `TestBillNewColumnsRoundTrip` through Create and Supersede (currency USD, extra charges, `DemandDataAvailable=false`, hour counts).
  - Extend `scope_isolation_integration_test.go` with `ExtraCharges` / `ReplaceExtraCharges` (every scoped method is covered by that test by rule).
- [ ] **Step 2:** Run with `-run` against the shared DB → FAIL.
- [ ] **Step 3:** Write the migration, model, queries, `make generate`, repositories. Add the four domain dirs and `internal/render/...` to both float guards.
- [ ] **Step 4:** `go test ./internal/store/postgres/ ./internal/store/postgres/admin/ -tags=integration -race -parallel 4 -run 'Billing|Tariff|Bill|Migration00014|ScopeIsolation'`, then `go test ./internal/arch/`, `make check-generate`, lint on the touched packages → PASS. Commit `feat(f4): billing parameters, tariff price sources and extra charges, bill columns (00014)`.
- [ ] **Step 5:** Mutation proofs recorded (the four named above plus: remove `ExtraCharges` from the isolation test list → the isolation meta-test goes red if one exists, otherwise note that). Commit.

---

## Task 2: `internal/domain/tariff` and `internal/domain/reactive`

**Files:** Create `internal/domain/tariff/{doc.go,validate.go,resolve.go,params.go,pricing.go}` with `_test.go` for each. Create `internal/domain/reactive/{reactive.go,reactive_test.go}`.

**Interfaces — Consumes:** Task 1's model types. `energy.Window` from `internal/domain/energy`.
**Produces:**
```go
package tariff

const DivisionScale int32 = 20

type Draft struct {
	Tariff       model.Tariff
	VatRate      *decimal.Decimal
	Taxes        []model.TariffTax
	ExtraCharges []model.TariffExtraCharge
	ManualYekdem []model.TariffManualYekdem
}
type ValidationError struct{ Fields map[string]string } // field → machine code, e.g. "vat_rate": "required"
func (e *ValidationError) Error() string
// Validate applies R128 (+R124, R125, R127) and returns the tariff with VatRate set.
func Validate(d Draft) (model.Tariff, error)

// Resolve: greatest EffectiveFrom whose Istanbul date ≤ on's Istanbul date; tie → latest CreatedAt; deleted rows ignored.
func Resolve(versions []model.Tariff, on time.Time, loc *time.Location) (model.Tariff, bool)
func ResolveParams(sets []model.BillingParameters, on time.Time, loc *time.Location) (model.BillingParameters, bool)
// ValidateParams: bands contiguous from (ReactiveExemptBelowKw or 0) upward, last band unbounded, limits > 0,
// tolerance in [0,1], multiplier ≥ 0, every tiering group threshold > 0, enums valid.
func ValidateParams(p model.BillingParameters) error

type YearMonth struct {
	Year  int
	Month time.Month
}
type Market struct {
	PTF          map[time.Time]decimal.Decimal // key: UTC hour start; TL/MWh
	Yekdem       map[YearMonth]decimal.Decimal // published; TL/MWh
	ManualYekdem map[YearMonth]decimal.Decimal // the tariff's own entries
}
type HourConsumption struct {
	Hour time.Time       // UTC hour start
	Kwh  decimal.Decimal // a sound single-hour Billing row
}
type HourDetail struct {
	Hour                                   time.Time
	Kwh, PTF, Yekdem, Kbk, UnitPrice, Cost decimal.Decimal // unrounded
}
type Method string
const (
	MethodHourly        Method = "hourly"
	MethodPeriodAverage Method = "period_average"
)
type Pricing struct {
	Method          Method
	BasePrice       *decimal.Decimal // (mean PTF over priced hours + YEKDEM(period month)) / 1000; nil if no PTF hour or no YEKDEM
	EnergyUnitPrice *decimal.Decimal // hourly: Σcost/Σkwh (nil when Σkwh = 0 → falls back to BasePrice×kbk_energy); average: BasePrice×kbk_energy
	T1, T2, T3      *decimal.Decimal // BasePrice × kbk_tN; nil when the coefficient is nil
	PowerUnitPrice, ReactiveUnitPrice, DistributionUnitPrice *decimal.Decimal // R119
	PTFAverage, YekdemUsed                  *decimal.Decimal
	HoursExpected, HoursMatched             int
	HoursMissingPrice, HoursMissingConsumption int
	Hourly   []HourDetail
	Complete bool   // false → ptf_data_missing (R109)
	Reason   string // "", "ptf_hours_over_tolerance", "yekdem_missing", "no_ptf"
}
// Price computes PTF+YEKDEM pricing for t (UsePtfYekdem must be true, else error).
// hourlyAvailable=true selects MethodHourly when t.PriceType is single_time (R124).
func Price(t model.Tariff, periodMonth YearMonth, period energy.Window, hours []HourConsumption, hourlyAvailable bool,
	m Market, p model.BillingParameters, loc *time.Location) (Pricing, error)
// FixedUnitPrices returns R119's non-energy prices for a non-PTF tariff (always the fixed columns).
```
```go
package reactive

type Input struct {
	NetConsumption        decimal.Decimal
	Inductive, Capacitive *decimal.Decimal
	ActiveExport          *decimal.Decimal
	InstalledPowerKw      *decimal.Decimal
	Term                  model.TariffTerm
	UserGroup             model.DistributionUserGroup
	UnitPrice             *decimal.Decimal // nil → no penalty line even when exceeded (R119)
	Params                model.BillingParameters
}
type Result struct {
	Exempt                          bool
	ExemptReason                    string // "term", "user_group", "generation", "below_kw"
	InductiveRatio, CapacitiveRatio *decimal.Decimal // nil when net is 0 (R117)
	InductiveLimit, CapacitiveLimit *decimal.Decimal
	InductiveKvarhCharged, CapacitiveKvarhCharged *decimal.Decimal // nil = side not penalised
	InductivePenalty, CapacitivePenalty decimal.Decimal // unrounded; zero when not penalised
	Applied bool
	Missing []string // "installed_power", "reactive_inductive", "reactive_capacitive"
}
func Evaluate(in Input) Result
```

**Rules to implement (reference for the tests):**
- Validate (R128): `vat_rate` required, `vat_rate ≥ 0`. `single_time` needs `single_time_price` unless PTF. `multi_time` needs `t1/t2/t3_price` unless PTF, where it needs `kbk_t1..t3`. PTF needs `kbk_energy` and currency `TRY`. `subtract_from_total` needs `generation_price_per_kwh`. `binomial` needs `contracted_power_kw` and `power_unit_price` (fixed source) or `kbk_power_price` (kbk source on PTF). `monomial` must have neither. A PTF tariff with `power_price_source=kbk` uses `kbk_power_price` (nil means no power line, which is valid). Every tax rate ≥ 0 and name non-empty. Every extra charge has a valid basis and a non-empty name. Enums valid. Manual YEKDEM months 1–12 with no duplicates. Errors collect **all** fields, not the first.
- Reactive order: exemption checks first (term ∈ params terms → "term"; user group ∈ params groups → "user_group"; export > params generation kWh → "generation"; installed power < params `ReactiveExemptBelowKw` → "below_kw"). Then installed power nil → `Missing installed_power`, not applied. Band by installed power (`MinKw ≤ P < MaxKw`). No band (below the first band with the exemption disabled) → `Missing installed_power`. Per side: register nil → `Missing reactive_<side>`, and that side is skipped. Exceeded if net > 0 and `reg / net > limit` (strict), or net = 0 and reg > 0 (R117). Charged quantity: `whole_quantity` → reg. `excess_over_limit` → `reg − limit × net`. UnitPrice nil → the charged quantity is still reported and the penalty is 0.
- Price: expected hours = every Istanbul hour start in `[period.From, period.To)` (23/25 on DST days before 2016). YEKDEM for a month = the manual entry if `t.UseManualYekdem` and one exists, else published. None → `Complete=false`, `Reason=yekdem_missing`. Base uses the **period month** (`periodMonth`, the month of the period key). Hourly unit_h = `(ptf_h + yekdem(month of h in Istanbul)) / 1000 × kbk_energy`. Missing price ratio (average method) or `(expected − matched)/expected` (hourly method) > tolerance → `Complete=false`.

- [ ] **Step 1: Failing tests (table-driven, exact decimals):**
  - `TestValidateRejectsMissingVatRate` (acceptance). `TestValidateCollectsEveryFieldError`. `TestValidateRejectsNilTaxRate`. `TestValidateBinomialNeedsContractAndPrice` / `TestValidateMonomialRejectsPowerFields` (R125). `TestValidatePtfNeedsTRYAndEnergyKbk`. `TestValidateMultiTimePtfNeedsAllBandKbks` (R124).
  - `TestResolvePicksGreatestEffectiveFromNotAfterDate`. `TestResolveTieBreaksOnLatestCreatedAt`. `TestResolveUsesIstanbulDate` (effective 2026-03-01 and `on` = 2026-02-28T21:30Z, which is 03-01 00:30 local → applies). `TestResolveNoTariffReturnsFalse`.
  - `TestValidateParamsRejectsGapInBands`. `TestValidateParamsDefaultsFromMigrationAreValid` (hand-built copy of the seed).
  - `TestPriceHourlyWeightsByConsumption`: 3 hours, kwh 10/20/30, PTF 1000/2000/3000, YEKDEM 500, kbk 1.1 → unit_h = 1.65/2.75/3.85, Σcost = 196.0, price = 196/60 = 3.2666… exact at scale 20.
  - `TestPriceHourlyUsesEachHoursOwnMonthYekdem` (a period across a month boundary). `TestPriceHourlyManualYekdemOverridesPublished`.
  - `TestPriceHourlyMissingHoursOverToleranceIsIncomplete` (48 of 744 missing = 6.45 % > 2 %). `TestPriceHourlyAtToleranceIsComplete` (exactly 2 % → complete: strict `>`).
  - `TestPriceCountsPriceMissingAndConsumptionMissingSeparately`.
  - `TestPricePeriodAverageDerivesEachCoefficientIndependently`: set kbk power 2, reactive nil, distribution nil, t-kbks nil → Power = base×2, Reactive nil, Distribution nil (acceptance "no fallback"). Mutation: `Reactive = base × coalesce(kbk_reactive, kbk_energy)` → red.
  - `TestPriceFixedSourcesUseTariffColumns` (R119). `TestPriceMissingYekdemIsIncompleteNeverZero`. `TestPriceMultiTimeNeverUsesHourlyMethod` (R124). `TestPriceExpectedHoursOnDSTDay` (2015-03-29 → 23).
  - Reactive: `TestReactiveExemptMonomial`, `TestReactiveExemptResidential`, `TestReactiveExemptLighting`, `TestReactiveExemptGeneration` (export 1.0001 exempt, 1 not), `TestReactiveExemptBelow9kW` (8.999 exempt, 9 not), `TestReactiveBandsAt30kW`, `TestReactiveWholeQuantityBasis`, `TestReactiveExcessOverLimitBasis`, `TestReactiveZeroNetWithReactiveIsExceeded` (R117, ratio nil), `TestReactiveStrictGreaterThanLimit`, `TestReactiveMissingInstalledPower`, `TestReactiveNilRegisterSkipsThatSideOnly`, `TestReactiveExemptionDisabledUsesParamsBands` (exempt nil + first band min 0 → 5 kW penalised).
- [ ] **Step 2:** `go test ./internal/domain/tariff/ ./internal/domain/reactive/` → FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** PASS with `-race`, lint. Commit `feat(f4): tariff validation, resolution, PTF+YEKDEM pricing and reactive penalty`.
- [ ] **Step 5:** Mutation proofs (at least: vat check removed; `>` → `>=` on the tolerance; YEKDEM of the period start month used for every hour; coefficient fallback; strict `>` on the reactive limit; below-kW exemption removed; ε rule replaced by ratio 1). Commit.

---

## Task 3: `internal/domain/billing` — period and analyzer invoice

**Files:** Create `internal/domain/billing/{doc.go,period.go,compute.go,lines.go,flags.go}` + `period_test.go`, `compute_test.go`, `rounding_test.go`.

**Interfaces — Consumes:** `tariff.Pricing`, `tariff.DivisionScale`, `reactive.Evaluate`, model types.
**Produces:**
```go
package billing

// Period implements 02 §5 for key "YYYY-MM" (R135: spec label).
func Period(periodKey string, cutoffDay int, loc *time.Location) (energy.Window, error)
// DaysInPeriod = whole Istanbul calendar days between From and To (never +1, LBR E.1 #12).
func DaysInPeriod(w energy.Window, loc *time.Location) int
// LatestClosedPeriodKey returns the latest key whose period end + settle ≤ now (for the scheduler).
func LatestClosedPeriodKey(cutoffDay int, now time.Time, settle time.Duration, loc *time.Location) string

type Quantities struct {
	ActiveImport                  *decimal.Decimal
	T1, T2, T3                    *decimal.Decimal
	ReactiveInductive, ReactiveCapacitive *decimal.Decimal
	ActiveExport                  *decimal.Decimal
	MaxDemandKw                   *decimal.Decimal
	IndexStart, IndexEnd          map[string]*decimal.Decimal // register name → index; copied to the bill
}
type Input struct {
	PeriodKey        string
	Period           energy.Window
	Days             int
	Tariff           model.Tariff
	Taxes            []model.TariffTax
	ExtraCharges     []model.TariffExtraCharge
	Params           model.BillingParameters
	Quantities       Quantities
	InstalledPowerKw *decimal.Decimal
	Pricing          *tariff.Pricing // required iff Tariff.UsePtfYekdem
}
type Line struct {
	Code      string // energy | energy_low_tier | energy_high_tier | energy_t1 | energy_t2 | energy_t3 | distribution | green_energy | power | demand_overrun | reactive_inductive | reactive_capacitive | extra:<name> | tax:<name> | vat | generation_credit
	Quantity  *decimal.Decimal
	Unit      string // kWh | kVArh | kW | period | %
	UnitPrice *decimal.Decimal
	RatePct   *decimal.Decimal
	Amount    decimal.Decimal // rounded 2 dp (R112)
}
type Flag string
const (
	FlagPTFDataMissing        Flag = "ptf_data_missing"
	FlagReactiveDataMissing   Flag = "reactive_data_missing"
	FlagInstalledPowerMissing Flag = "installed_power_missing"
	FlagGenerationDataMissing Flag = "generation_data_missing"
)
type Invoice struct {
	PeriodKey string; Period energy.Window; Days int
	TariffID *uuid.UUID; TariffEffectiveFrom *time.Time; Currency model.CurrencyCode
	ActiveImport, T1, T2, T3, Inductive, Capacitive, ActiveExport, NetConsumption decimal.Decimal
	LowTierKwh, HighTierKwh decimal.Decimal; TieredApplied bool
	MaxDemandKw *decimal.Decimal; DemandDataAvailable bool
	IndexStart, IndexEnd map[string]*decimal.Decimal
	EffectiveEnergyPrice, LowTierPrice, HighTierPrice *decimal.Decimal
	EnergyCost, DistributionCost, GreenEnergyCost, PowerCost, DemandOverrunCost, ReactivePenalty,
	ExtraChargesCost, OtherTaxesCost, VatBase, VatCost, GenerationCredit, TotalCost decimal.Decimal
	Reactive reactive.Result; ReactivePowerPrice *decimal.Decimal
	GenerationUsage model.GenerationUsage; GenerationPricePerKwh *decimal.Decimal
	PtfYekdemUsed bool; Pricing *tariff.Pricing
	Lines []Line
	Flags []Flag // sorted, unique
}
var ErrInvalidInput = errors.New("billing: invalid input")
// Compute returns ErrInvalidInput (wrapped, with the reason) for a nil ActiveImport, nil T1–T3 on a multi-time tariff,
// a PTF tariff without Pricing, or Days ≤ 0. Every other data problem becomes a Flag.
func Compute(in Input) (Invoice, error)
// ToModel maps an Invoice onto model.Bill + []model.BillLine (+ hourly rows) for persistence; labels are line codes (localised at render).
func ToModel(inv Invoice, scope model.BillScope) (model.Bill, []model.BillLine, []model.BillHourlyDetail)
```

**Computation order (spec §6, amended):**
1. Net per §6.1. For `subtract_from_*` with `ActiveExport` nil → treat export as unknown: `FlagGenerationDataMissing`, export = 0 for arithmetic.
2. Energy. PTF: single-time → `EnergyUnitPrice × net` (never tiered, R120). Multi-time → bands (R123 pro-rata) × `T1/T2/T3` (nil price → no line). Fixed: tiering per §6.2 with R120 gates and `params.TieringGroups` thresholds (tariff `overuse_threshold_kwh_per_day` overrides). `whole_consumption_switch` prices all net at `overuse_price` when tiered. Multi-time fixed → bands × band prices, **never tiered, never averaged**.
3. Distribution = `net × unit` over **all** net (removed-behaviour 3). Unit per R119 on PTF, else `tariff.DistributionCost`. Nil unit → no line.
4. Green energy per §6.4.
5. Power = `contracted × power unit` (binomial only). Overrun = `(max − contracted) × power unit × params.DemandOverrunMultiplier` when max > contracted. Max nil → `DemandDataAvailable=false`, no overrun line.
6. Reactive via `reactive.Evaluate`. `Missing` installed_power → `FlagInstalledPowerMissing`. Missing register → `FlagReactiveDataMissing`. Both only when not exempt. Lines per side when charged quantity > 0 and a unit price exists.
7. Extra charges (R126) in `SortOrder`.
8. Named taxes on the rounded energy cost (R112). 9. VAT. 10. Generation credit (`subtract_from_total`) and total. Pricing incomplete → `FlagPTFDataMissing`.
`EffectiveEnergyPrice = energy_cost_unrounded / net` (nil when net = 0).

- [ ] **Step 1: Failing tests (hand-computed, exact):**
  - `TestPeriodCutoffExamples` (02 §5: `2025-12`/1 → 01-12-2025..01-01-2026; /15 → 15-12..15-01; /31 in Feb 2026 → 28-02..31-03 and the days). `TestDaysInPeriodNeverAddsOne` (Dec cut-off 1 = 31). `TestLatestClosedPeriodKey` (cut-off 1, now 2026-02-03T23:59 Istanbul → "2025-12"; 2026-02-04T00:00 → "2026-01").
  - `TestSingleRateUntiered`, `TestTieredResidentialSplit` (acceptance), `TestTieredCommercialSplit`, `TestTieringThresholdOverride`, `TestTieringWholeConsumptionSwitchMode`, `TestTieringSkippedOutsideVoltageLevels`.
  - `TestMultiTimeNeverTieredNeverAveraged` (acceptance, daily average far above threshold → three band lines at their own prices, no tier lines). Mutations: the average price → red; tiering allowed → red.
  - `TestDistributionChargedOnTotalNotLowTier` (acceptance). Mutation: `low_tier × unit` → red.
  - `TestGenerationNone`, `TestGenerationSubtractFromConsumption` (incl. the §6.2 low-tier offset, R122), `TestGenerationSubtractFromTotal` (credit after VAT, total clamps at 0), `TestGenerationMultiTimeProRata` (R123), `TestGenerationExportNilFlags`.
  - `TestDemandOverrunUsesMultiplier`, `TestDemandNilMeansNoOverrunAndUnavailable`, `TestMonomialHasNoPowerLines`.
  - `TestReactiveAppliedAddsLinesAndVatBase`, `TestReactiveExemptionsProduceNoLines` (monomial, residential, lighting, generation, sub-9 kW).
  - `TestNamedTaxesAreRoundedPercentOfRoundedEnergy`, `TestVatOnRoundedBase`, `TestTotalEqualsVatBasePlusVatMinusCredit`.
  - `TestIcmalRowAssemblyMatchesSupplierToTheKurus`: take one anonymised real row's energy, distribution, reactive and power amounts as fixed-price inputs (unit = amount/qty at scale 20), with BTV 5 % and VAT 20 %. Assert `vat_base`, `vat` and `total` equal the supplier's KDV Matrahı / KDV / Fatura Tutarı exactly. Task 5 supplies the numbers in `internal/domain/tariff/icmal/testdata`; until then use the two rows quoted in LBR §G.1 checks.
  - `TestPTFSingleRateUsesPricingEnergyUnitPrice`, `TestPTFMultiTimeUsesBandPrices`, `TestPTFNilCoefficientProducesNoLine` (acceptance: reactive kbk nil with source kbk → no reactive line even though exceeded; power kbk nil → no power line). Mutation: fallback to `kbk_energy` → red.
  - `TestPTFIncompleteFlags`, `TestComputeRejectsNilActiveImport`, `TestComputeRejectsMultiTimeNilBands`.
  - `TestExtraChargesEachBasis` (5 bases, nil max demand → no `per_max_demand_kw` line + unavailable).
  - `TestNegativeCreditRoundsHalfUpOnAbsoluteValue`.
- [ ] **Step 2:** FAIL. **Step 3:** implement. **Step 4:** PASS `-race`, lint, commit `feat(f4): billing period and analyzer invoice computation`.
- [ ] **Step 5:** Mutation proofs for every acceptance-named rule. Commit.

---

## Task 4: Aggregation, golden corpus and the permanent property test

**Files:** Create `internal/domain/billing/{aggregate.go,aggregate_test.go,golden_test.go,property_test.go}`, `internal/domain/billing/testdata/golden/*.json`.

**Produces:**
```go
// AggregateQuantities sums per register (R116): nil in any member → nil. MaxDemandKw: the member's when len==1, else nil (R111).
// Index maps are summed per register with the same nil rule.
func AggregateQuantities(members []Quantities) Quantities
// SumInstalledPower returns nil if any member is nil or members is empty.
func SumInstalledPower(members []*decimal.Decimal) *decimal.Decimal
// AggregateHours sums per hour only where every member series has that hour (R116); the other hours are reported as missing.
func AggregateHours(members [][]tariff.HourConsumption) (sum []tariff.HourConsumption, missing []time.Time)
// CombineCompany sums building invoices (R115): quantities, charges, VAT and total; lines merged by (Code, RatePct);
// flags are the union; TariffID nil; Period = the hull of the building periods; Currency must match, else ErrInvalidInput.
func CombineCompany(periodKey string, buildings []Invoice) (Invoice, error)
```
**Golden format:** one JSON file per case `{ "input": <Input>, "expected": <Invoice subset: lines [{code, quantity, unit_price, rate_pct, amount}], totals, flags, tiered_applied, reactive_applied> }`, decimals as strings. `go test -run TestGolden -update` rewrites the expected output. **Reviewing a regenerated file is a hand check against the spec formula, recorded in the report.**

**Cases (every §F4 acceptance item, one file each):** `single_rate_untiered`, `single_rate_tiered_residential`, `single_rate_tiered_commercial`, `multi_time_no_tiering`, `generation_none`, `generation_subtract_from_consumption`, `generation_subtract_from_total`, `reactive_applied`, `reactive_exempt_monomial`, `reactive_exempt_residential`, `reactive_exempt_lighting`, `reactive_exempt_generation`, `reactive_exempt_below_9kw`, `demand_overrun`, `named_taxes`, `ptf_hourly`, `ptf_period_average`, `ptf_manual_yekdem_override`, `ptf_missing_hours_flagged`, `kbk_null_coefficient_zero_line`, `extra_charges_custom_power`, `icmal_row_assembly_real` (anonymised), `company_two_buildings`.

- [ ] **Step 1:** Tests:
  - `TestGolden` over the corpus.
  - `TestBuildingInvoiceIsNotSumOfAnalyzerInvoicesWhenTiered` (acceptance): two residential analyzers each below the threshold, aggregate above. Assert `building.TotalCost ≠ Σ analyzer.TotalCost` and the building is tiered.
  - `TestAggregateNilRegisterPropagates`, `TestAggregateMaxDemandOnlyForSingleMember`, `TestAggregateHoursRequiresEveryMember`, `TestCombineCompanyMergesLinesByCodeAndRate`, `TestCombineCompanyRejectsMixedCurrency`.
  - `TestComputePropertyInvariants`: fixed seed 20260917, 5 000 scenarios in the default run, `-short` 500, env `EKOKOD_PROPERTY_N` for reviewers (≥ 100 000). Random tariffs across every shape (fixed/PTF × single/multi × monomial/binomial × 3 generation modes × every user group × both voltage levels × both reactive bases × taxes × extra charges × nil coefficients) and random quantities including nils and zeros. **Oracle = an independent straight-line reimplementation in the test file** (`oracleCompute`) written from spec §6 and the rulings, not from `compute.go`, plus these invariants: every amount has ≤ 2 decimals; `VatBase = Σ amounts of non-vat, non-credit lines`; `TotalCost = max(0, VatBase + VatCost − GenerationCredit)`; a `distribution` line's quantity equals `NetConsumption`; `LowTierKwh + HighTierKwh = NetConsumption` when tiered; no tier line on multi-time or PTF; no `reactive_*` line when `Reactive.Exempt`; a nil coefficient with source `kbk` means no such line; toggling an unrelated coefficient never changes another line (metamorphic pair).
- [ ] **Step 2:** FAIL. **Step 3:** implement aggregation, write the oracle and the corpus. **Step 4:** PASS, commit `test(f4): golden corpus, aggregation and billing property test`.
- [ ] **Step 5: Oracle mutant proof (controller re-verifies):** each of these production mutants must turn `TestComputePropertyInvariants` red within the default 5 000 scenarios. Record the first failing seed for each: (a) distribution on low tier; (b) multi-time tier at the average band price; (c) `kbk_reactive` falling back to `kbk_energy`; (d) VAT on the unrounded base; (e) named tax on distribution + energy; (f) credit subtracted before VAT; (g) reactive `>=`; (h) overrun multiplier ignored; (i) R123 pro-rata removed; (j) PTF tariff tiered. Commit.

---

## Task 5: `internal/domain/tariff/icmal` and the anonymised fixture

**Files:** Create `internal/domain/tariff/icmal/{doc.go,columns.go,numbers.go,parse.go,derive.go}` + tests, `internal/domain/tariff/icmal/testdata/ck_icmal_2025_11_12_anonymised.csv`, `internal/domain/tariff/icmal/testdata/README.md` (source, anonymisation steps, the synthetic base prices used).

**Produces:**
```go
package icmal

type Table struct {
	Header []string
	Rows   [][]string
}
type NumberFormat int
const (
	FormatTurkish NumberFormat = iota
	FormatUS
)
func DetectNumberFormat(t Table) (NumberFormat, error) // R129
func ParseNumber(s string, f NumberFormat) (*decimal.Decimal, error) // "", "-", "BOŞ" → nil
// NormaliseHeader: Turkish-aware lower case (unicode.TurkishCase), fold ı→i ğ→g ü→u ş→s ö→o ç→c, drop spaces and punctuation.
func NormaliseHeader(s string) string
type Warning struct {
	Row  int // 1-based data row, 0 = file level
	Code string
	Text string
}
type Parsed struct {
	Rows     []model.IcmalRow // BuildingID nil; Raw holds the original cells keyed by original header
	Format   NumberFormat
	Warnings []Warning
}
// Parse maps aliases (02 §8.1 + LBR C.1; the first matching column wins on duplicate headers), requires the
// period and total kWh columns, sets IsMultiTime per R133, IsCancelled from "Fatura İptal Mi?" (evet/yes/true, Turkish-aware).
func Parse(t Table) (Parsed, error)

type Coefficient struct {
	Value            *decimal.Decimal // median, rounded 4 dp (rates 2 dp)
	Samples          int
	StdDev           *decimal.Decimal // population
	Stable           *bool            // stddev/median < 0.1; nil when fewer than 2 samples
	BackCalcErrorPct *decimal.Decimal // energy only: max over groups
}
type Analysis struct {
	EtsoCode                                              string
	Periods                                               []string
	EnergyKbk, DistributionTlPerKwh, PowerUnitPrice       Coefficient
	ReactiveUnitPrice, ReactiveKbk, VatRate               Coefficient
	Taxes                                                 map[string]Coefficient // "BTV", "Enerji Fonu", "TRT"
	OveruseRequiresManualEntry                            bool // always true (02 §8.2)
	IsMultiTime                                           *bool
	Term, VoltageLevel                                    *string
	WithinTolerance                                       bool // every group's energy back-calc error < 2 %
	Warnings                                              []Warning
}
// Analyse groups by ETSO, then nets rows signed per (ETSO, period) excluding cancelled (R130), derives per 02 §8.2 as
// amended by R131/R132, aggregates by median (§8.3), and back-calculates each group with the MEDIAN energy KBK (§8.4, never per-row circular).
// base maps period "YYYYMM" → (PTF month average + YEKDEM) / 1000; a missing month → warning, group skipped.
func Analyse(rows []model.IcmalRow, base map[string]decimal.Decimal) []Analysis
```
**Anonymised fixture:** from `/mnt/c/Users/meren/Desktop/Work/bcem-apps/bcem-energy/icmalverileri.csv` (read with `timeout`). **Drop** the columns İl, İlçe, Ad Soyad (both), VKN (both), TCKN (both), Hesap No, Sayac Seri Numarası, Sayaç No, Dağıtım Hizmet Noktası No, Pmum Sayaç Id, Fatura Seri No and Fatura No. **Replace** each distinct ETSO code with `40ZTEST` + a 9-digit zero-padded sequence in first-seen order. Keep every other column and value byte-identical. Commit only the anonymised file. **The controller greps the fixture for 10- and 11-digit numbers and the original ETSO prefixes before merging.**

- [ ] **Step 1:** Tests: `TestDetectUSFormatOnRealFixture`, `TestDetectTurkishFormat`, `TestDetectMixedFormatsIsError`, `TestParseNumberTurkish` (`1.234,56`, `0,125` = 0.125, `1.234` = 1234), `TestParseNumberUS`, `TestNormaliseHeaderTurkishDotlessI` ("Fatura İptal Mi?" and "FATURA IPTAL MI" match), `TestParseRealFixtureRowCountAndTotals` (45 rows; Σ Toplam Kwh and Σ Fatura Tutarı equal the hand sums recorded in the README), `TestParseTimeTypeFromColumnNotBands` (R133), `TestParseCancelledRowsExcluded` (a synthetic "Evet" row), `TestAnalyseNetsReversalRowsPerPeriod` (R130), `TestAnalyseMedianAndPopulationStdDev`, `TestAnalysePowerStabilityFlag`, `TestAnalyseBTVIsPercentOfEnergy` (R131: 5.00 on commercial rows, 1.00 on the industrial ETSO), `TestAnalyseReactiveUnitPriceFromRealRows` (2.6455), `TestAnalyseBackCalculatesWithMedianNotPerRow` (mutation: per-row KBK → error always 0 → the test that injects an outlier period expects > 2 % → red), `TestAnalyseRealFixtureWithinTwoPercent` (acceptance: synthetic base prices declared in the README).
- [ ] **Steps 2–4:** FAIL → implement → PASS `-race`, lint, commit `feat(f4): icmal parser and KBK derivation with anonymised real fixture`.
- [ ] **Step 5:** Mutation proofs. Commit.

---

## Task 6: `consumption.Billing.PeriodConsumption` (R107)

**Files:** Create `internal/service/consumption/{period.go,period_test.go,period_integration_test.go}`. Modify `billing.go` / `anomalies.go` only to thread an explicit-window mode through the existing internals.

**Interfaces — Produces:**
```go
// PeriodRequest asks for invoice-grade consumption over explicit contiguous windows (R107).
type PeriodRequest struct {
	AnalyzerIDs []uuid.UUID
	Windows     []energy.Window // sorted, contiguous (Windows[i].To == Windows[i+1].From), 1..13, each ≥ 1 h
}
// PeriodConsumption is Consumption at Monthly semantics over req.Windows.
func (b *Billing) PeriodConsumption(ctx context.Context, sc store.Scope, req PeriodRequest) ([]Row, error)
// PeriodConsumptionAndRecord additionally writes suspect-period and missing_readings anomalies for req.Windows,
// exactly as ConsumptionAndRecord does for Monthly buckets.
func (b *Billing) PeriodConsumptionAndRecord(ctx context.Context, sc store.Scope, req PeriodRequest) ([]Row, error)
```
**Semantics:**
- Validation: `ErrInvalidRequest` for a bad Scope, empty/duplicate/too many analyzers, an empty/unsorted/non-contiguous window list, more than 13 windows, a hull span > `MaxRequestSpan` or cells > `MaxCells`. All checked before any I/O.
- Boundary precedence = Monthly (billing → load_profile → daily). Tolerance cap = half the smaller width of the explicit windows meeting at the instant. The first `From` and the last `To` use their own window's width.
- Settle = `SettleDelayMonthly` after `To`.
- Max-demand kinds = `MaxDemandKindsFor(Monthly)`, minus `billing` unless the window equals `energy.Bucket(Monthly, w.From, istanbul)` exactly (R111).
- Gap and pre-install rules as Monthly. Resolved gap overrides only for exactly that window.
- **Existing `Consumption`/`ConsumptionAndRecord` behaviour and every existing test stay unchanged.**

- [ ] **Step 1:** Tests:
  - `TestPeriodRequestValidation` (pure, table).
  - `TestPeriodConsumptionCutoffOneEqualsMonthly` (integration: the same fixture with a reset, a gap and billing snapshots → `reflect.DeepEqual` rows for 3 calendar months). Mutation: tolerance from the window width halved again → red.
  - `TestPeriodConsumptionCutoff15Telescopes`: Σ of two adjacent 15th-to-15th windows = the single 2-period window per register.
  - `TestPeriodConsumptionIgnoresBillingKindMaxDemandOffCalendar`.
  - `TestPeriodConsumptionNegativeDeltaIsNilAndSuspect`.
  - `TestPeriodConsumptionAndRecordWritesMissingReadingsForTheWindow`.
  - `TestPeriodConsumptionDropsUnsettledWindow` (now = To + 71h59m → absent; + 72h → present).
  - `TestPeriodConsumptionIsolatesTenants` (other tenant's AdminScope, positive control).
- [ ] **Steps 2–4:** FAIL → implement → `go test ./internal/service/consumption/ -tags=integration -race -parallel 4` (the whole package, because the refactor touches shared internals) → PASS. Commit `feat(f4): explicit-window billing consumption for cut-off periods`.
- [ ] **Step 5:** Mutation proofs + the reviewer's brute-force comparison `PeriodConsumption(cutoff=1) == Consumption(Monthly)` over ≥ 1 000 random reading sets. Commit.

---

## Task 7: `internal/service/billing`

**Files:** Create `internal/service/billing/{doc.go,service.go,generate.go,inputs.go,read.go,errors.go}` + `service_test.go` (pure helpers) + `generate_integration_test.go`, `read_integration_test.go`. Modify `internal/arch` service guard only if the new package needs an entry.

**Interfaces — Consumes:** Task 6 `PeriodConsumptionAndRecord` and `Consumption` (Hourly), Task 3/4 domain, Task 1 repositories.
**Produces:**
```go
package billing // import alias billingsvc at call sites

type ConsumptionReader interface {
	PeriodConsumptionAndRecord(ctx context.Context, sc store.Scope, req consumption.PeriodRequest) ([]consumption.Row, error)
	Consumption(ctx context.Context, sc store.Scope, req consumption.SeriesRequest) ([]consumption.Row, error)
}
type Deps struct {
	Consumption ConsumptionReader
	Buildings   store.BuildingRepository
	Analyzers   store.AnalyzerRepository
	Tariffs     store.TariffRepository
	Params      store.BillingParameterRepository
	Prices      store.PriceRepository
	Anomalies   store.AnomalyRepository
	Bills       store.BillRepository
	Ops         store.OpsRepository
	Clock       clock.Clock
	Log         *slog.Logger
}
func New(d Deps) (*Service, error)

type GenerateRequest struct {
	Scope     model.BillScope // analyzer | building | company
	SubjectID uuid.UUID       // analyzer id, building id, or company id (must equal sc.CompanyID)
	PeriodKey string
	Force     bool
}
type GenerateResult struct {
	Bill       model.Bill
	Created    bool
	Superseded *uuid.UUID
}
var ErrInvalidRequest = errors.New("billing: invalid request")
type ComputeError struct {
	Code   string // R113 codes
	Detail map[string]string
}
func (e *ComputeError) Error() string
const (
	CodeTariffNotFound = "tariff_not_found"; CodeNoConsumptionData = "no_consumption_data"
	CodeUnresolvedAnomaly = "unresolved_anomaly"; CodePeriodNotClosed = "period_not_closed"
	CodeBillingParametersMissing = "billing_parameters_missing"
)
func (s *Service) Generate(ctx context.Context, sc store.Scope, req GenerateRequest) (GenerateResult, error)
// Reads backing 05 §7.
func (s *Service) Get(ctx context.Context, sc store.Scope, id uuid.UUID) (model.Bill, []model.BillLine, []model.BillMember, error)
func (s *Service) List(ctx context.Context, sc store.Scope, f store.BillFilter) ([]model.Bill, error)
func (s *Service) HourlyDetail(ctx context.Context, sc store.Scope, id uuid.UUID) ([]model.BillHourlyDetail, error)
func (s *Service) Latest(ctx context.Context, sc store.Scope, scope model.BillScope, subjectID uuid.UUID) (model.Bill, error)
```
**Generate algorithm (analyzer/building):**
1. Validate the request (period key format, scope, SubjectID) → `ErrInvalidRequest`. Analyzer → its building (an analyzer without a building → `ErrInvalidRequest`). Members = the analyzer, or every non-deleted analyzer of the building (none → `no_consumption_data`).
2. `window = billing.Period(key, building.BillCutoffDay)`. `now < window.To + SettleDelayMonthly` → `period_not_closed`.
3. Existing live bill (`Bills.Current`) and `!Force` → return it, `Created=false`.
4. Tariff: `Tariffs.Effective(building, window.From)` → `ErrNotFound` → `tariff_not_found`. Load its taxes, extra charges and manual YEKDEM. `Params.Effective(window.From)` → `billing_parameters_missing`.
5. `PeriodConsumptionAndRecord(members, [window])`. Any member without a row → `no_consumption_data`. Any row with `Suspect` non-empty, or any unresolved anomaly for a member overlapping the window (`Anomalies.List{AnalyzerIDs, Unresolved, Range: window}`) → `unresolved_anomaly` with the anomaly ids in Detail.
6. Quantities from rows (`energy.Register` → `Quantities` field map in `inputs.go`), aggregated with `AggregateQuantities` for a building. Installed power from analyzers (`SumInstalledPower`).
7. PTF tariff: `Consumption(Hourly, members, window)` → keep only rows with `Window` exactly one hour and a non-nil ActiveImport. `hourlyAvailable` = at least one member returned any hourly row. Hours via `AggregateHours`. Prices via `Prices.HourlyRange(window)` and `Prices.Yekdem` for each month touched. `tariff.Price(...)`.
8. `billing.Compute` → `ToModel` → status `issued`, or `flagged` with `flag_reason` = comma-joined flags. Members recorded. `Bills.Create`, or `Supersede` when Force and a live bill exists. PTF hourly → `ReplaceHourlyDetail`.
9. A flagged bill appends an operational message (`Ops.AppendMessage`, kind per the existing message conventions) naming the bill and flags.

**Company:** for every non-deleted building of `sc.CompanyID` run steps 2–7 in memory (each building's own window and tariff). The first `ComputeError` aborts with that code and the building id in Detail. `CombineCompany` → persist scope `company`.

- [ ] **Step 1:** Tests (integration unless noted; fixtures via `NewTenant` + readings + tariff + parameters seed):
  - `TestGenerateAnalyzerIssuesBillWithLinesAndMembers`.
  - `TestGenerateBlocksOnUnresolvedAnomaly` (acceptance), plus `TestGenerateBlocksOnSuspectRow`.
  - `TestGenerateNoTariffFails`, `TestGeneratePeriodNotClosed`, `TestGenerateMissingMemberRowFails` (building with one silent analyzer).
  - `TestGeneratePTFMissingHoursFlagsInsteadOfIssuing` (acceptance).
  - `TestGeneratePTFHourlyPersistsHourlyDetail`, `TestGenerateIsIdempotentWithoutForce`, `TestGenerateForceSupersedes`.
  - `TestGenerateBuildingAppliesTariffOnceToAggregate`, `TestGenerateCompanyUsesEachBuildingsOwnTariffAndWindow` (cut-off 1 and 15).
  - `TestGenerateNeverReadsAnalyticsRows` (reflection guard as F3's R61: `Deps` has no `AnalyticsRepository` field, and `ConsumptionReader` is satisfied by `*consumption.Billing` only; F3 carry-forward 1).
  - `TestGenerateIsolatesTenants` (other tenant's AdminScope: its analyzer id → `ErrNotFound`, positive control).
  - `TestGenerateFlaggedBillAppendsOpsMessage`. `TestGenerateCutoff15UsesWindowConsumptionNotCalendarMonth` (LBR E.2 #7).
- [ ] **Steps 2–4:** FAIL → implement → `go test ./internal/service/billing/ -tags=integration -race -parallel 4` → PASS. Commit `feat(f4): billing service — generate, supersede, reads`.
- [ ] **Step 5:** Mutation proofs (anomaly check removed; settle check removed; `Force` ignored; analytics substitution impossible; tenant predicate). The reviewer runs a brute-force end-to-end of 200 random tariffs × fixtures through `Generate` vs `billing.Compute` on the same inputs. Commit.

---

## Task 8: `internal/service/tariff`

**Files:** Create `internal/service/tariff/{doc.go,service.go,templates.go,bulk.go,icmal.go,sheet.go}` + `sheet_test.go` (pure) + `service_integration_test.go`, `icmal_integration_test.go`. `go get github.com/xuri/excelize/v2@v2.10.1`.

**Produces:**
```go
type Input struct {
	Tariff       model.Tariff
	VatRate      *decimal.Decimal
	Taxes        []model.TariffTax
	ExtraCharges []model.TariffExtraCharge
	ManualYekdem []model.TariffManualYekdem
}
type Definition struct {
	Tariff       model.Tariff
	Taxes        []model.TariffTax
	ExtraCharges []model.TariffExtraCharge
	ManualYekdem []model.TariffManualYekdem
}
func New(d Deps) (*Service, error) // Deps: Tariffs, Templates, Buildings, Analyzers, Icmal, Prices, Clock, Log
func (s *Service) Create(ctx context.Context, sc store.Scope, in Input) (Definition, error) // tariff.Validate first → *tariff.ValidationError
func (s *Service) Update(ctx context.Context, sc store.Scope, id uuid.UUID, in Input) (Definition, error)
func (s *Service) Get(ctx context.Context, sc store.Scope, id uuid.UUID) (Definition, error)
func (s *Service) List(ctx context.Context, sc store.Scope, f store.TariffFilter) ([]model.Tariff, error)
func (s *Service) Delete(ctx context.Context, sc store.Scope, id uuid.UUID) error
func (s *Service) Applicable(ctx context.Context, sc store.Scope, buildingID uuid.UUID, on time.Time) (Definition, error)
// Templates: the payload is the JSON of Input (decimals as strings) and is validated with tariff.Validate on write (LBR E.2 #33).
func (s *Service) CreateTemplate(ctx context.Context, sc store.Scope, name string, description *string, in Input, isDefault bool) (model.TariffTemplate, error)
func (s *Service) UpdateTemplate(ctx context.Context, sc store.Scope, id uuid.UUID, name string, description *string, in Input, isDefault bool) (model.TariffTemplate, error)
func (s *Service) DeleteTemplate(ctx context.Context, sc store.Scope, id uuid.UUID) error
func (s *Service) ListTemplates(ctx context.Context, sc store.Scope, f store.TariffTemplateFilter) ([]model.TariffTemplate, error)
func (s *Service) ApplyTemplate(ctx context.Context, sc store.Scope, templateID uuid.UUID, buildingIDs []uuid.UUID, effectiveFrom time.Time) ([]Definition, error)
func (s *Service) BulkAssign(ctx context.Context, sc store.Scope, in Input, buildingIDs []uuid.UUID) ([]Definition, error) // all-or-nothing: any invisible building → ErrNotFound, nothing written
func (s *Service) CurrentForBuildings(ctx context.Context, sc store.Scope, on time.Time) (map[uuid.UUID]*Definition, error)
// İcmal
func ReadSheet(fileName string, content []byte) (icmal.Table, error) // .csv (delimiter per R129) or .xlsx (first sheet); header row = first row with ≥3 known aliases
type ImportResult struct {
	Import   model.IcmalImport
	Analyses []icmal.Analysis
	Unmatched []string // ETSO codes with no analyzer
}
func (s *Service) Import(ctx context.Context, sc store.Scope, uploadedBy uuid.UUID, fileName string, content []byte) (ImportResult, error)
func (s *Service) GetImport(ctx context.Context, sc store.Scope, id uuid.UUID) (ImportResult, error) // re-reads the stored result JSON
type ApplyConfirmation struct {
	BuildingID    uuid.UUID
	EtsoCode      string
	EffectiveFrom time.Time
	Base          *Input // required when the building has no applicable tariff at EffectiveFrom
}
func (s *Service) ApplyImport(ctx context.Context, sc store.Scope, importID uuid.UUID, confirmations []ApplyConfirmation) ([]Definition, error) // R134; status → applied
```
Import: parse → match ETSO to `analyzers.etso_code` → building (several buildings for one ETSO → warning, unmatched) → base prices per period from `Prices.HourlyRange` (month average of stored PTF) + `Prices.Yekdem` (missing → Analyse warning) → `icmal.Analyse` → store rows (`BuildingID` set) and a `result` JSON (analyses + unmatched) with status `analysed`. **No tariff is written.**

- [ ] **Step 1:** Tests: `TestCreateRejectsMissingVatRateBeforeAnyWrite` (acceptance; a counting fake repo proves zero calls), `TestCreatePersistsTaxesExtraChargesManualYekdem`, `TestApplicableFallsBackToCompanyWideTariff`, `TestTemplatePayloadValidatedOnWrite`, `TestApplyTemplateCarriesPowerAndTaxes` (LBR E.2 #33), `TestBulkAssignIsAllOrNothing` (AdminScope of another tenant's building in the list → nothing written, positive control), `TestReadSheetCSVCommaAndSemicolon`, `TestReadSheetXLSX` (generated in-test with excelize), `TestImportRealFixtureAnalysesWithoutWritingTariffs`, `TestApplyImportRequiresConfirmationAndEffectiveFrom`, `TestApplyImportCreatesNewVersionWithSources` (R119/R132: power fixed, reactive fixed, distribution kbk), `TestImportIsolatesTenants`.
- [ ] **Steps 2–4:** FAIL → implement → PASS `-tags=integration -race -parallel 4` on this package → commit `feat(f4): tariff service, templates, bulk assignment and icmal import`.
- [ ] **Step 5:** Mutation proofs. Commit.

---

## Task 9: `internal/render` — invoice PDF and hourly XLSX

**Files:** Create `internal/render/invoicepdf/{pdf.go,labels.go,pdf_test.go,testdata/}`, `internal/render/hourlyxlsx/{xlsx.go,xlsx_test.go}`, `internal/arch/render_test.go`. `go get github.com/go-pdf/fpdf@v0.9.0 golang.org/x/image`.

**Produces:**
```go
package invoicepdf
type Member struct {
	AnalyzerID                         uuid.UUID
	Name, InstallationNumber           string
}
type Document struct {
	Bill         model.Bill
	Lines        []model.BillLine
	Members      []Member
	CompanyName  string
	BuildingName string // empty for company scope
	Locale       string // "tr" (default) | "en"
}
// Render is deterministic: identical Documents give identical bytes.
func Render(d Document) ([]byte, error)

package hourlyxlsx
func Render(b model.Bill, rows []model.BillHourlyDetail, locale string) ([]byte, error) // one sheet: time (Istanbul), kWh, PTF, YEKDEM, KBK, unit price, cost; totals row
```
**Determinism recipe:** `fpdf.New("P","mm","A4","")`, `AddUTF8FontFromBytes("Go", "", goregular.TTF)` and `"B"` with `gobold.TTF`, `SetCreationDate(d.Bill.ComputedAt)`, `SetModificationDate(d.Bill.ComputedAt)`, `SetCatalogSort(true)`, `SetCompression(false)` in tests (true in production is fine if still byte-stable: test both). Money formatted `1.234,56` (tr) / `1,234.56` (en) from the decimal string, never through float. Sections: header (company, building, scope, period key, dates, days, tariff effective date, currency); readings table (index start/end per register); quantities; the lines table in `SortOrder`; reactive detail (ratios, limits, applied); PTF block when used (hours expected/matched/missing, average PTF, YEKDEM); members list (building/company); flags banner when status is `flagged` ("ÖNİZLEME — kesilmedi / PREVIEW — not issued"). Labels come from a `map[locale]map[code]string` in `labels.go` with Turkish and English for every line code. A missing key is a test failure.

- [ ] **Step 1:** Tests: `TestRenderIsByteStable` (render twice → equal; compression on and off). `TestRenderContainsTurkishGlyphs` (compression off: the ToUnicode CMap of the embedded font contains `011F 015F 0131 0130 00FC 00F6 00E7 011E 015E 00DC 00D6 00C7`). `TestRenderGoldenHash` (sha256 of `testdata/analyzer_invoice.pdf.sha256`, `-update` flag). `TestRenderAllScopes` (analyzer, building with members, company). `TestLabelsCoverEveryLineCode` (walks the Task 3 code list plus the `extra:`/`tax:` prefixes). `TestMoneyFormatNeverUsesFloat` (a value with 18 significant digits formats exactly). `TestXLSXHasOneRowPerHourAndTotals`. `TestRenderPackagesImportNoStoreServicePlatform` (arch).
- [ ] **Steps 2–4:** FAIL → implement → PASS → commit `feat(f4): deterministic invoice PDF and hourly XLSX rendering`.
- [ ] **Step 5:** Mutation proofs (a `time.Now()` creation date → byte-stable red; a dropped Turkish font → glyph test red). Commit.

---

## Task 10: `billing.dispatch`, `billing.generate`, `billing.render_pdf`, scheduler and worker wiring

**Files:** Create `internal/job/{billing.go,billing_test.go,billing_integration_test.go}`, `internal/store/postgres/admin/billing.go` (+ integration test). Modify `internal/store/repository.go` (the admin interface only), `internal/job/task.go` (Handlers fields + Register), `internal/scheduler/scheduler.go` (+ test), `internal/worker/wiring.go` (+ test).

**Produces:**
```go
// store
type BillableBuilding struct {
	CompanyID, BuildingID uuid.UUID
	CutoffDay             int
}
type AdminBillingRepository interface {
	// BillableBuildings: every non-deleted building of a non-deleted company that has at least one non-deleted analyzer.
	BillableBuildings(ctx context.Context) ([]BillableBuilding, error)
}
// job
const (
	TypeBillingDispatch  = "billing.dispatch"
	TypeBillingGenerate  = "billing.generate"
	TypeBillingRenderPDF = "billing.render_pdf"
)
type BillingGeneratePayload struct {
	CompanyID uuid.UUID       `json:"company_id"`
	Scope     model.BillScope `json:"scope"`
	SubjectID uuid.UUID       `json:"subject_id"`
	PeriodKey string          `json:"period_key"`
}
type BillingRenderPayload struct {
	CompanyID uuid.UUID `json:"company_id"`
	BillID    uuid.UUID `json:"bill_id"`
}
func BillingGenerateTaskID(p BillingGeneratePayload) string // "billing.generate:<scope>:<subject>:<period>"
func NewBillingDispatchTask(o TaskOptions) (*asynq.Task, error)
func NewBillingGenerateTask(p BillingGeneratePayload, o TaskOptions) (*asynq.Task, error) // TaskID, NO Retention (R53), Timeout 10m
func NewBillingRenderTask(p BillingRenderPayload, o TaskOptions) (*asynq.Task, error)
type BillingDispatcher interface{ Dispatch(ctx context.Context) error }
type BillingGenerator interface{ Generate(ctx context.Context, p BillingGeneratePayload) error }
type BillingRenderer interface{ Render(ctx context.Context, p BillingRenderPayload) error }
// Handlers gains BillingDispatch BillingDispatcher; BillingGenerate BillingGenerator; BillingRender BillingRenderer (each registered only when non-nil).
```
- **Dispatch:** for each billable building, `key = billing.LatestClosedPeriodKey(cutoff, now, SettleDelayMonthly, istanbul)`. Enqueue building generate (a TaskID conflict = already queued, not an error). Per company, enqueue one company generate for the latest key closed for **all** its buildings (the minimum across buildings).
- **Generate adapter:** `billingsvc.Generate(SystemScope(company), req{Force:false})`, with a `job_runs` row (`OpsRepository.StartRun/FinishRun`). A `ComputeError` → run status `failed`, recorded with its code, **SkipRetry** (the data will not fix itself inside the retry window). Other errors go through `ClassifyForRetry`. On `Created`, enqueue a render.
- **Render adapter:** `Get` → `invoicepdf.Render` → write `cfg.Storage.Root/bills/<company>/<bill id>.pdf` (0o640, created atomically via temp + rename) → `SetPDFPath`.
- **Scheduler:** entry `{Cron: cfg.Schedule.Billing, Task: billing.dispatch}`.
- **Worker `build`:** construct `consumption.NewBilling` (with Locker, Analyzers, Users), `billingsvc.New`, the three adapters, and set the Handlers fields. `worker_test` asserts registration.

- [ ] **Step 1:** Tests: `TestBillingGenerateTaskIDIsDeterministicAndHasNoRetention`, `TestDispatchEnqueuesLatestClosedPeriodPerBuilding` (fake clock; cut-off 1 and 15), `TestDispatchCompanyKeyIsMinimumAcrossBuildings`, `TestGenerateHandlerComputeErrorSkipsRetryAndRecordsRun`, `TestGenerateHandlerEnqueuesRenderOnCreate`, `TestRenderHandlerWritesFileAndPath`, `TestBillableBuildingsExcludesDeletedAndAnalyzerless` (admin, integration), `TestSchedulerRegistersBillingDispatch`, `TestWorkerRegistersBillingHandlers`.
- [ ] **Steps 2–4:** FAIL → implement → PASS on `./internal/job/ ./internal/scheduler/ ./internal/worker/ ./internal/store/postgres/admin/` (integration `-parallel 4`; `internal/job` ≈150 s) → commit `feat(f4): billing dispatch, generate and render jobs wired into worker and scheduler`.
- [ ] **Step 5:** Mutation proofs. Commit.

---

## Task 11: `compare-invoice`, acceptance suite, verification block, handoff

**Files:** Create `internal/cli/tool.go`, `internal/cli/tool_compare_invoice.go` (+ test), `testdata/real-invoices/README.md`, `testdata/real-invoices/icmal_row_ck_anonymised.json`, `internal/domain/billing/f4_acceptance_test.go`. Modify `HANDOFF_NEXT_SESSION.md`.

**Produces:** `ekokod tool compare-invoice --fixture <file.json>`. Fixture: `{"description", "input": <billing.Input JSON>, "expected": {"lines": [{"code","amount"}], "vat_base", "vat", "total"}, "explanations": {"<code>": "text"}}`. Output: a table `code | expected | computed | diff | explanation`. Exit 1 when any |diff| > 0.00 without an explanation.

- [ ] **Step 1:** `TestCompareInvoiceMatchesAnonymisedIcmalRowExactly`, `TestCompareInvoiceExitsNonZeroOnUnexplainedDiff`. `f4_acceptance_test.go` holds one named test per §F4 acceptance bullet. Each either calls or references (by a `t.Run` on the same helper) the task test that proves it, so `go test ./internal/domain/billing -run TestF4Acceptance -v` lists them all. Items that are not domain-level (anomaly block, PTF flagging, icmal round-trip, PDFs) are listed with their package and test name in a comment table and run in the verification block.
- [ ] **Step 2:** Verification block (controller, sequential per package, nothing else running):
  ```bash
  go test ./internal/domain/tariff/... ./internal/domain/reactive/... ./internal/domain/billing/... -race -v
  go test ./internal/domain/billing -run TestGolden -v
  EKOKOD_PROPERTY_N=100000 go test ./internal/domain/billing -run TestComputePropertyInvariants
  for p in ./internal/service/billing/ ./internal/service/tariff/ ./internal/service/consumption/ ./internal/job/ ./internal/store/postgres/ ./internal/store/postgres/admin/ ./internal/render/...; do
    EKOKOD_TEST_PG_DSN=... go test $p -tags=integration -race -count=1 -parallel 2 || break; done
  go run ./cmd/ekokod tool compare-invoice --fixture testdata/real-invoices/icmal_row_ck_anonymised.json
  make lint build check-generate && go test ./internal/arch/
  ```
- [ ] **Step 3:** Handoff for F5/F6: F4 state, the rulings, the PO questions (including the **open real-invoice acceptance item**), and the F6 wiring list (HTTP for §6/§7, the error codes → 409/422 mapping, `/bills/{id}/pdf` on demand). Commit.

---

## Carried forward — explicitly NOT F4
- HTTP handlers, RBAC and OpenAPI for §6/§7 (F6). `/bills/dashboard*` netting (F8/F9). Solar tariff service (F9). National schedule and public calculator (F12, removed-behaviour 11).
- Automatic re-invoice detection after overrides (R114 → F8). Transformer-loss modelling (PO Q4). F2 R47/R48 (F6). F3 note: `plant_production_*` aggregates never refreshed after backfill (F9).

## Self-review
- **Spec coverage:** §F4 scope → tariff resolution / PTF / manual YEKDEM / KBK (Tasks 2, 5), reactive + sub-9 kW (2), period / net / generation / tiering / distribution / green / power / overrun / taxes / VAT / total / output (3), building and company (4, 7), icmal parser (5), jobs (10), PDF (9), hourly detail and Excel (7, 9), API-backing services (7, 8). Every acceptance bullet: goldens (4), distribution-on-total (3), multi-time no averaging (3), vat_rate rejected (2, 8), PTF tolerance flag (2, 7), KBK independence (2, 3), anomaly block (7), icmal round-trip + 2 % (5), building ≠ Σ analyzers (4), PDF Turkish + byte-stable (9), real-invoice comparison (11: tool + anonymised real row; full comparison OPEN, PO).
- **F3 carry-forward 1–10:** 1 (7: guard), 2 (7: anomaly + missing row), 3 (6, R107), 4 (R111, PO Q2), 5 (R117), 6 (3: nil band → error, nil reactive → flag), 7 (R114), 8 (R105), 9 (F6), 10 (F9).
- **Placeholder scan:** none intended. Golden expected values are generated and then hand-checked per the Task 4 rule.
- **Name consistency:** `PeriodRequest`, `PeriodConsumptionAndRecord`, `tariff.Pricing`, `billing.Input/Invoice/Compute/ToModel`, `AggregateQuantities`, `CombineCompany`, `BillingParameterRepository.Effective`, and `BillableBuilding` are used identically across Tasks 1–11.
