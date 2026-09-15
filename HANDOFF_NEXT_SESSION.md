# Handoff — ekokod rewrite: F1 and F2 COMPLETE → next phase F3 (consumption engine)

> **STATUS:** F1 and F2 are finished. `phase/f2-integration-layer` is at `97f7d51`, branched from F1's tip `10d6577`.
> Both phases went through per-task review, fix rounds and a two-pass whole-branch final review with fix waves.
> **Nothing has been pushed; `main` is untouched. Push and merge-to-main are the user's call.**
> **F3 has no written plan yet** — the next session writes it first.

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/` içinde, faz planı
> `docs/rewrite/09-implementation-plan.md`. Toplam 16 faz var (F0-F15).
>
> **Durum: F1 ve F2 tamamlandı.** F2'nin ucu `phase/f2-integration-layer` branch'inde (`97f7d51`); F1'in ucu `10d6577`.
> Hiçbir şey push edilmedi, `main`'e dokunulmadı. Push ve `main`'e merge kararı bana ait, sen yapma.
> **Sıradaki faz F3 (consumption engine) ve henüz planı yok.**
>
> Şu sırayla ilerle:
>
> 1. **`HANDOFF_NEXT_SESSION.md`'i baştan sona oku** (bu dosya `phase/f2-integration-layer` üzerinde). Özellikle
>    "START HERE", "F3 must pick up", "Binding rulings", "Process rules" ve "Environment" bölümlerini.
> 2. **F3 planını yaz:** `superpowers:writing-plans` ile, `docs/rewrite/09-implementation-plan.md` §F3 ve ilgili spec
>    bölümlerinden, `docs/superpowers/plans/` altına. "F3 must pick up" listesinin tamamı planda karşılanmalı. Sonra planı
>    plan-document-reviewer prompt'u ile review ettir, bulguları düzelttir ve pre-flight conflict taramasını yap.
> 3. **`phase/f2-integration-layer`'ın ucundan `phase/f3-consumption-engine` branch'ini aç** ve F3'ü
>    `superpowers:subagent-driven-development` ile yürüt: her görev için taze implementer, ardından task review, gerekirse
>    fix turu. **Bu yöntemi onaylıyorum, sorma, devam et.**
> 4. **Aynı anda en fazla 5 agent** (Docker'lı test koşan en fazla 3). Hızlı ol ama review'leri atlama:
>    - implementer'lar sonnet; izolasyon/güvenlik/veri kaybı riski taşıyan task review'ler opus; scoped re-review'ler sonnet;
>    - büyük transcript'li agent'ı küçük bir fix için resume etme, taze ve ucuz agent ver;
>    - bağımlılığı henüz merge edilmemiş görevleri spekülatif base üzerinde başlat, ama base'in brief'teki "Consumes"
>      listesini içerdiğini önce doğrula (`go build` + tipleri grep'le).
> 5. **Paralel iş için native-filesystem worktree kullan** (`/home/personal/...`); Agent tool'un `isolation: "worktree"`
>    özelliği bu mount'ta ÇALIŞMIYOR — Environment bölümündeki tarifi kullan.
> 6. **Subagent'lar arka plan komutu beklerken turlarını bitiriyor** (bu fazda 3 kez oldu). Raporsuz "bekliyorum" mesajı
>    gelirse hemen SendMessage ile dürt: gate'leri ön planda koşsun, commit etsin, raporlasın.
> 7. **Her guard'ın başarısız olabildiğini kanıtlat:** reviewer kendi mutasyonunu koşsun; yeşil kalan mutasyon = Important.
>    Tenant'lar arası izolasyon kanıtları **AdminScope** ile ve pozitif kontrolle yapılmalı.
> 8. **F3 bitince dur**, handoff'u F4 için güncelle ve bana yapıştırılacak prompt ver.
>
> Ortam:
> - Her komuttan önce `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=4 GOMEMLIMIT=2GiB`.
> - Docker seanslar arası ve WSL yeniden başlayınca kapanıyor; önce `docker ps` ile doğrula. Kapalıysa benden başlatmamı iste.

---

## START HERE — exact state

| Branch | Head | State |
|---|---|---|
| `phase/f1-data-model` | `10d6577` | F1 complete. |
| `phase/f2-integration-layer` | `97f7d51` | F2 complete; full integration suite green; F2 verification block (09 §F2) passes. |
| `f1/*`, `f2/*` task branches | — | merged; safe to delete. |

Working copies: the main checkout `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite` sits on `phase/f1-data-model`;
`/home/personal/ekokod-f2-phase` is the F2 phase worktree. Create F3's branch from `phase/f2-integration-layer`.

### What F2 delivered
- `internal/integration`: the `MeterDataSource`/`Adapter` contract, `Secret` (pointer-held; `[REDACTED]` through every
  formatting path), typed `Error` with `ErrAuth`, `ErrConfig`, `ErrRateLimited`, `ErrUpstreamUnavailable`,
  `ErrMalformedPayload`, `ErrNotFound`.
- `integration/httpx`: the single HTTP seam — per-host certificate pinning (pin-only trust, proven by a subprocess test),
  per-attempt timeout + distributed lock + rate limiter, jittered backoff, Retry-After cap, strict URL templates,
  redirects refused, `Request.NoRetry` (R32), errors that never carry a URL, body, header or secret.
- `integration/normalize`: exact decimals, Istanbul-local timestamps, units, `Chunk` (R36: midnight-aligned windows that
  span up to `max` whole days).
- `integration/fake` (per-server certificates, fixture sanitiser with word-tokenised PII keys) and
  `internal/platform/lock` (memory + Redis, owner-only release, atomic single-use consume).
- Adapters: OSOS, GridBox, ARIL, PM5340, iSolarCloud (+ production store that quarantines plant-level samples),
  EPİAŞ (+ `internal/marketdata` price sync).
- Store seams: migrations `00012` (interval generation + `generation_anchors`) and `00013` (`provider_hourly_values`),
  `GenerationRepository`, `ProviderSeriesRepository`, `admin.IngestionRepository`, `store.SystemScope`.
- `internal/ingest`: dispatch, analyzer sync, fetch (dedupe, R13 sanity rule, attribution filter, anomalies, cursor rules),
  run records, redacted operator messages; `ingest/generation` (PM5340 cumulative, locked per analyzer, pre-anchor
  derivation); `ingest/backfill` (deterministic window tasks, `Force` re-run).
- `internal/credentials`: write-only credential service and iSolarCloud OAuth (HMAC single-use state, locked refresh,
  a zero refresh token never overwrites the stored one).
- `internal/worker.Build`: the single wiring point; the scheduler/elector defects F1 handed over are fixed;
  config knobs `EKOKOD_INGEST_*`, `EKOKOD_EPIAS_BASE_URL`, `EKOKOD_EPIAS_CAS_URL`, `EKOKOD_JOB_MAX_RETRIES` (0 allowed).
- Test infrastructure: parallel-safe (protected template DB, `NewEmptyDB`, `-parallel 8`, `TESTCONTAINERS_HOST_OVERRIDE`);
  `internal/store/postgres` went from 4m41s to ~33 s.
- Acceptance: all 11 §F2 criteria mapped to end-to-end tests through a real worker, Redis and TimescaleDB.

### F3 must pick up
1. **`consumption.refresh` is declared but unhandled** (R17): `job.TypeConsumptionRefresh` exists and nothing registers a
   handler or enqueues it. F3 wires `ingest.Deps.ConsumptionRefresh`, reads `affected_from`/`affected_to` from fetch runs
   and refreshes the continuous aggregates, including history older than the 30-day real-time horizon.
2. **PM5340 cumulative-generation recompute cost** (parked): an old backfill recomputes every later row inside one
   post-persist hook (~35k rows per analyzer-year). Decide whether it becomes its own queued, bounded task.
3. **OAuth state is not bound to the browser session** (R47): the F3/F6 HTTP callback must bind the nonce to the user's
   session cookie.
4. **Error mapping:** `integration.ErrConfig` (R48) must surface as a configuration problem, never "authentication failed";
   `credentials.Configure` returns an `ErrConflict`-matching error for an existing definition (R45) — callers use `Update`.
5. **Deferred minors from the two final-review passes** live in
   `.superpowers/sdd/2026-09-15-f2-integration-layer/final-review-A-report.md` and `final-review-B-report.md`
   (warning `Detail` dropped by the pipeline, `NextCursor` doc inconsistency across adapters, EPİAŞ TGT re-login per
   process, OSOS logging in on every fetch, EPİAŞ PTF chunk-seam overlap inflating counts, and the pass-B leftovers).
   Triage them when F3 touches the same files.

### Open questions for the product owner
- **BLOCKER (unchanged):** `plant_production`'s primary key requires a device id, so iSolar plant-level samples are
  quarantined, counted and reported rather than stored.
- Q2 ARIL `WithoutMultiplier` semantics (R9/R38 — the highest-value F14 check); Q3 analyzer commissioning date (R11);
  Q4 OSOS multiplier and iSolar result codes (R8, R21); Q5 `contracted_power_kw` column (R30).
- PM5340's analyzer is created **active** on operator configure (R27 applies only to discovered analyzers) — confirm.
- Still open from F1: reactive penalty base, sub-9 kW exemption, tiered pricing groups, 2 % missing-hour tolerance, grid
  emission factor, ISO 50001 clause texts, a GHG↔ISO mapping table, the national tariff schedule source.

### F14 live-verification checklist (fixtures encode rulings, not confirmed traffic)
OSOS multiplier and hourly `meter_date` hour-start (R35); ARIL `WithoutMultiplier` and wire field names (R38); GridBox
`last_success_date` shape and `endDate` inclusivity (R37/R50); iSolar wire shape (R41; `install_power` vs
`installed_power`), result codes (R21), redirect_uri/state shape (R46); EPİAŞ response field names.

---

## Binding rulings
F1's rulings 1–14 (tenancy, FK validation in SQL, AdminScope proofs, batches, `TimeRange`, secrets, atomic upserts,
naming) still bind — see the F1 ledger. Plan rulings R1–R36 are in the F2 plan's "Spec gaps and rulings" table.
Rulings made during F2 execution (full text with "cost if wrong" in the F2 ledger):

- **R37/R50** GridBox spec gaps (verify in F14); `last_success_date` never raises `From`, may only cap `To`.
- **R38** ARIL spec gaps isolated in code with F14 comments.
- **R39** A PTF backfill does not backfill YEKDEM (trailing three months from now; historical YEKDEM is a separate action).
- **R40** iSolar endpoints come from the seeded `integration_definitions.json` (gateway + relative paths, `cloud_id`).
- **R41** The legacy iSolar TypeScript is the wire-shape authority; F14 confirms.
- **R42** iSolar exports `MaxWindow` (1 d minute / 31 d day) and splits minute requests into ≤3 h sub-windows.
- **R43** One quarantine message per `Store` call; metadata carries the fetch window.
- **R44** Non-cursor list paging derives pages from `rowCount` with a hard cap; exceeding it errors, never truncates.
- **R45** `Configure` refuses an existing (company, definition) credential with `ErrConflict`.
- **R46** The token exchange's `redirect_uri` is byte-identical to the authorize request's.
- **R47** OAuth state is not session-bound yet — the F3/F6 handler binds it to the session.
- **R48** `integration.ErrConfig`: non-retryable, never an auth failure; every adapter uses it for configuration problems.
- **R49** ARIL max demand is keyed by `MaxDemandDate` over the whole billing month, so a resumed fetch never nulls it.
- **R51** GridBox falls back to the analyzer's stored multiplier; the pipeline persists only provider-resolved multipliers.
- **R52** The generation hook is serialised per analyzer; rows older than the anchor are derived (an initial anchor moves
  back and cumulative values shift uniformly; an operator/meter anchor derives backwards).
- **R53** Backfill `Force` re-mints task ids; windows skipped by an existing task id make the run `partial`.
- Process rulings: speculative bases are allowed once the "Consumes" list is verified; the final whole-branch review is
  split into a production pass and a tests pass with split fix waves; test-only integration commits with recorded
  mutation proofs merge without a separate review seat.

## Process rules that paid off (keep them)
- **Prove every guard can fail.** Reviewers run their own mutations; a mutation that stays green is an Important finding.
  This caught: a `ParseFloat` guard evadable through a function value, a TLS guard evadable through an embedded field, an
  adapter fixture-matrix guard that only checked file existence, an end-to-end resume test that passed with the cursor
  ignored, and two silent wrong-key wirings in the worker.
- **The adapter review patterns** (15 items) in `.superpowers/sdd/2026-09-15-f2-integration-layer/adapter-patterns.md`
  are the fastest way to review any new provider; item 15 (serialise every client) was missed by two adapters in a row.
- **Trial-merge speculative bases early** — that is how the float-guard/sanitiser clash and the `MigrateDownN` guard hit
  were found before integration.
- **Carry a defect pattern found in one review to still-running siblings by message** (SerializeKey, `NoRetry`, patterns).
- **Stalls:** subagents end their turn waiting on background runs; nudge immediately to finish gates in the foreground.
- **Never redo an action the permission classifier denied for an agent** — fix the base instead.
- **Reverting a mutation:** use a targeted edit, never `git checkout -- <file>` when that file also holds uncommitted work.

## Environment — read before running anything
- Toolchain in `$HOME/.local`; prefix every command with
  `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=4 GOMEMLIMIT=2GiB`.
- `C:\Users\meren\.wslconfig` is in effect (12 GB RAM, 10 CPUs, `autoMemoryReclaim`). Keep at most 3 agents running
  Docker-backed suites at once; sweep leftover containers (`docker ps -a`) when readiness timeouts start appearing.
- Docker stops on WSL restart: check `docker ps`; if it is down ask the user to start Docker Desktop.
- Worktrees: `git worktree add -b <branch> /home/personal/<name> <base>` then
  `git config --global --add safe.directory /home/personal/<name>`. The Agent tool's `isolation: "worktree"` fails here.
- Integration tests: `go test ./... -tags=integration -race -count=1 -parallel 8`. A failure containing `wait until ready`
  or SQLSTATE `55006` is Docker load — retry that package once before believing it.
- After any migration or query change run `sqlc generate`; `make check-generate` is part of the gate.
- The legacy repo `/mnt/c/Users/meren/Desktop/Work/bcem-apps/bcem-energy` is a slow mount: read files directly by path and
  put `timeout` on every grep; never search `node_modules`, `.next` or `public`.
- Commit trailers: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` plus the session's own
  `Claude-Session:` URL.

## Key paths
```
docs/rewrite/                                                   the specification
docs/superpowers/plans/2026-09-10-f1-data-model.md               F1 plan
docs/superpowers/plans/2026-09-15-f2-integration-layer.md        F2 plan (rulings table R1–R36)
.superpowers/sdd/2026-09-10-f1-data-model/                       F1 SDD workspace (ledger: progress.md)
.superpowers/sdd/2026-09-15-f2-integration-layer/                F2 SDD workspace — progress.md (ledger, all rulings),
                                                                 adapter-patterns.md, provider-defaults.md, task reports,
                                                                 final-review-A/B reports
internal/integration/  internal/ingest/  internal/credentials/  internal/marketdata/  internal/worker/  internal/store/
```
