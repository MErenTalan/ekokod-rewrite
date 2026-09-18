# Handoff — ekokod rewrite: F1–F7 COMPLETE → next phase F8 (bills, tariffs and reports UI)

> **STATUS (2026-09-18):** **F7 (alarms and messages) is finished** on `phase/f7-alarms` in
> `/home/personal/ekokod-f7-phase`, branched from `phase/f6b-screens`. All four alarm types evaluate, the
> hourly dispatch → evaluate → notify pipeline runs, e-mail goes out through the company's SMTP settings in
> both locales, and the Alarms and Messages screens ship with a job-history tab. 13 plan tasks are committed
> with their own mutation proofs, followed by a whole-phase self-review and a full acceptance run.
> **Working mode:** no parallel agents, no subagents, no Workflow. One session runs one phase inline, writes
> this handoff at the end, and the user runs `/clear`.
> **Nothing has been pushed and `main` is untouched. Pushing and merging are the user's call** — including
> whether to merge `phase/f7-alarms` into `phase/f6b-screens` first.

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/` içinde,
> faz planı `docs/rewrite/09-implementation-plan.md` (16 faz, F0–F15).
>
> **Durum:** F1–F7 tamam. F7 (Alarmlar ve Mesajlar) `phase/f7-alarms` dalında
> (`/home/personal/ekokod-f7-phase`) bitti. **Sıradaki faz F8 (Faturalar, Tarifeler ve Raporlar arayüzü) ve
> henüz planı yok.** F4'ün gerçek-fatura karşılaştırması ürün sahibinden fatura gelene kadar AÇIK. Hiçbir şey
> push edilmedi, `main`'e dokunulmadı; push ve merge kararı bana ait.
>
> **Çalışma düzeni (kesin):** paralel agent YOK, subagent YOK, Workflow YOK. Fazı bu oturumda tek başına yürüt
> (`superpowers:executing-plans` + TDD). Review'ları kendin yap: her task sonunda diff'i oku, kendi mutasyonunla
> guard'ın kırmızıya düştüğünü kanıtla; faz sonunda bir kez bütün-faz self-review (yetki/tenancy önce, sonra
> erişilebilirlik ve tasarım kuralları). Faz bitince `HANDOFF_NEXT_SESSION.md`'i F9 için yeniden yaz, bana
> yapıştırılacak promptu ver ve dur.
>
> Şu sırayla ilerle:
> 1. `/home/personal/ekokod-f7-phase/HANDOFF_NEXT_SESSION.md`'i baştan sona oku (START HERE, "What F7 built",
>    Deviations, "F8 must pick up", Open questions, Environment). F7 planını
>    (`docs/superpowers/plans/2026-09-18-f7-alarms-messages.md`, R211–R232) ve F6b planını (R190–R210)
>    referans olarak kullan.
> 2. F8 worktree'sini aç: `git worktree add -b phase/f8-billing-ui /home/personal/ekokod-f8-phase phase/f7-alarms`,
>    ardından `web/` içinde `pnpm install --frozen-lockfile`.
> 3. **F8 planını yaz** (`superpowers:writing-plans`, `docs/superpowers/plans/` altına; sıkı: imzalar, kural
>    tablosu, isimlendirilmiş testler, doğrulama komutları). Kaynaklar: `09-implementation-plan.md` §F8,
>    `01-project-context.md` §7.10 (Faturalar), §7.11 (Tarifeler), §7.14 (Raporlar), `05-api-contract.md` §7–§8
>    ve §11, `07-design-system.md`, `design-system/bcem-energy/MASTER.md` ve sonra `OVERRIDES.md`.
>    Planlamadan önce legacy fatura/tarife/rapor kodunu ve gerçek veriyi oku. **F8'in iki büyük kararını bana
>    sor:** (a) `GET /bills/dashboard` ucu F8'de mi açılacak (F6b'de bana ertelenmişti) ve fatura panosunun
>    toplamları hangi kaynaktan gelecek; (b) rapor PDF/Excel üretimi bu fazda mı yoksa yalnızca ekranlar mı
>    (09 §F8 ikisini de istiyor, ama `internal/render` şu an yalnızca fatura PDF'i ve saatlik Excel biliyor).
>    Ayrıca ekranların ihtiyaç duyduğu ama mevcut API'de olmayan uçları açıkça listele ve F8'de mi yoksa
>    sonraki fazda mı yapılacağını bana sor. Planı bir kez spesifikasyona + legacy'ye karşı kendin gözden geçir,
>    sonra uygula.
>
> **Ortam ve güvenlik:** her komutun başına
> `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB` ekle.
> Sadece üzerinde çalıştığın paketleri/test hedeflerini koştur; **asla `make test` veya `go test ./...` çalıştırma.**
> Entegrasyon testleri için önce `docker ps` ile `ekokod-test-pg` ve `ekokod-test-redis` var mı bak (yoksa
> `make test-db-up` / `make test-redis-up`; Docker kapalıysa bana söyle), sonra
> `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <paket> -tags=integration -count=1 -parallel 4`.
> Web'de `pnpm test --maxWorkers=2`; Storybook/Playwright/`next build` aynı anda değil, teker teker. Ağır
> işlerden önce `free -m` ve `ps -eo rss,comm | grep java` kontrol et (bellek sıkışıksa Playwright `--workers=1`),
> e2e için `pnpm build && E2E_API_PORT=18082 PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1`
> — **18080 dolmusum'a ait, kullanma.** Başka projenin Gradle/java/vitest süreçlerine, 5432'deki
> `dolmusum-dev-postgres-1` container'ına ve dolmusum'un portlarına dokunma.

---

## START HERE — exact state

| Branch / worktree | Head | State |
|---|---|---|
| `phase/f1-data-model` (main checkout `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite`) | `10d6577` | F1 complete |
| `phase/f2-integration-layer` (`/home/personal/ekokod-f2-phase`) | `9919bd3` | F2 complete |
| `phase/f3-consumption-engine` (`/home/personal/ekokod-f3-phase`) | `58597ec` | F3 complete |
| `phase/f4-billing-engine` (`/home/personal/ekokod-f4-phase`) | `cf79346` | F4 complete; real-invoice comparison OPEN |
| `phase/f5-design-system` (`/home/personal/ekokod-f5-phase`) | `a162d7a` | F5 complete |
| `phase/f6-core-screens` (`/home/personal/ekokod-f6-phase`) | `09e07bd` | F6a complete |
| `phase/f6b-screens` (`/home/personal/ekokod-f6b-phase`) | `064688b` | F6b complete |
| **`phase/f7-alarms`** (`/home/personal/ekokod-f7-phase`) | this handoff commit | **F7 complete** (plan `c0fed62`; tasks 1–13) |

Shared test containers (both tmpfs, recreate freely): `ekokod-test-pg` (TimescaleDB, :55432, `make test-db-up`)
and `ekokod-test-redis` (:56379, `make test-redis-up`). The e2e run creates and drops `ekokod_e2e` in
`ekokod-test-pg` and uses Redis DBs 6/7.

- Plan: `docs/superpowers/plans/2026-09-18-f7-alarms-messages.md` (rulings **R211–R232**, rule tables, file map,
  tasks 1–13, acceptance map, product-owner decisions D-1…D-5, open questions Q-C1…Q-C8).
- Ledger (git-ignored, in the primary checkout):
  `/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite/.superpowers/sdd/2026-09-18-f7/inline-ledger.md` — every
  mutation, the three honest survivors, the two real defects e2e found, and the deviations.
- Working tree clean, 14 commits on top of F6b.

---

## What F7 built

### Go
- **`internal/domain/alarm`** — pure evaluation, no repository, no clock, no I/O. `Validate` (settings per
  type), `EvaluateReactive` / `EvaluateComms` / `EvaluatePower` / `EvaluateInvoice`, `Suppressed` (frequency)
  and `Describe` (sentences + the curated event detail). Every 09 §F7 acceptance criterion is a table test here.
- **`internal/service/alarms`** — scoped CRUD with R213's intersection, the dry run (R223), the real firing
  path (`Run`/`fire`), notification delivery (`Notify`) and the platform dispatch tick.
- **`internal/service/ops`** — the Messages read, the job-run list, and allow-listed triggering (R220).
- **`internal/job/alarm.go`** — `alarm.dispatch` (hourly, `config.Schedule.Alarms`, 01 §8) → `alarm.evaluate`
  (one company, TaskID per company per hour, R225) → `alarm.notify` (one event, TaskID per event).
- **`internal/mail`** — `AlarmFired` in tr and en, plain text (R221).
- **Ten routes** (05 §10) with their authorisation matrix rows and the tenancy sweep extended to alarms.
- **Store**: `MessageFilter.Q` (substring search over message and detail, `strpos(lower(...))`),
  `AlarmEventFilter.Notified`, and `AdminAlarmRepository.CompaniesWithEnabledAlarms` for the dispatcher.
- **Six permissions** (R228) and the regenerated web fixture.

### Web (all under `web/src`)
- **Alarms** (`/ekorm/alarms`): the §7.12 list with the active/passive filter, one type-switching dialog
  (R229), the details+log dialog and the dry-run dialog.
- **Messages** (`/ekorm/messages`): the §7.13 table with search and both filters, plus the job-history tab
  (D-4) with R220's trigger buttons, watched through `GET /jobs/{id}`.
- Two i18n namespaces (`alarms`, `messages`), `formatDateTime`, and two new `SCREENS` lines in
  `responsive.spec.ts`.

### The two decisions that shaped the phase (see D-1…D-5 in the plan)
- **SMS stays in the UI and is never sent silently** (R211): numbers are stored, the dialog says plainly that
  SMS is not active yet, and every firing records the non-delivery in Messages with a count, never a number.
- **Voltage is gone** (R212): no provider reports voltage or current — verified against 3926 real load-profile
  rows and against `model.MeterReading` — so the type is a power alarm, a voltage threshold is refused with
  422, and a stored one (migration 08 will import legacy rules) is never rendered back.

---

## Defects this phase found in earlier work

1. **`MarkNotified` stamped a zero time instead of NULL**, so "fired and nobody was told" — the state
   `model.AlarmEvent` documents — was unreachable and every failed delivery read back as delivered. Found by
   the e2e run after the unit tests passed against a fake that special-cased zero. Fixed in the store.
2. **Two F1 migration tests were red at the F6b tip** (`TestContinuousAggregatesExist`,
   `TestRefreshPolicyOffsetsAreConfigured`): they assert what migration 00005 declares for production but ran
   against the policy-stripped template F6b's `b34faba` introduced. They now migrate their own database. F6b's
   handoff never listed `internal/store/postgres` among the integration packages it ran — **run it.**
3. **A de-duplicated trigger answered 500.** R225 works, but the operator saw "Beklenmeyen bir hata oluştu".
   Now 409 `job_already_queued`.

---

## Deviations from the F7 plan (all in the ledger)
- `TestPermissionsMatrix` already held a literal copy of the whole table, so it was extended rather than
  duplicated by the per-permission test the plan sketched.
- Message search is `strpos(lower(...))`, not `ilike`: escaping `%` and `_` in the user's own term is a trap
  avoided entirely. `strpos` had to be declared in `nativeFunctions` (F1's own guard demanded it).
- **`consumption.refresh` is NOT triggerable** (Q-C8): its payload needs an explicit From/To, and a
  no-argument button would invent a window the operator never chose.
- `Describe` stores its rendered lines in the event's curated detail (a fifth key), so the notifier repeats
  what fired rather than re-deriving it from data that has since moved.
- The e2e API port must be overridden: **dolmusum holds 18080**; use `E2E_API_PORT=18082`.

---

## Known flake, NOT F7's
`calendar.spec.ts › a company admin deletes the event again` fails in roughly two of three sequential full e2e
runs and passes in isolation. **Proved independent of F7**: it fails the same way with the alarms and messages
specs excluded from the run (38 passed, 1 failed). Machine load from another project is the likely cause. If
F8 touches the calendar, fix it properly; otherwise re-run before concluding anything from it.

---

## F8 must pick up
- **Branching:** start from `phase/f7-alarms` (see the prompt).
- **Bills (§7.10):** the invoice dashboard with the buildings, plants and netting sections and every column;
  ad-hoc generation with the analyzer multi-select; analyzer/building/company PDF download; download-all;
  hourly PTF Excel export; every validation message listed. `GET /bills/dashboard` was **deferred here by the
  user on 2026-09-17** — ask before designing around it.
- **Tariffs (§7.11):** the full create/edit form including PTF+YEKDEM mode and the KBK fields; tariff history;
  templates with apply; bulk assignment with current-state and history views; the icmal import flow with its
  review-and-confirm step.
- **Reports (§7.14):** monthly, yearly and archive tabs; PDF and Excel; e-mail sending. `report.generate` and
  `report.deliver` jobs. `internal/render` currently knows only the invoice PDF and the hourly Excel, so ask
  whether rendering is in scope for F8 or the screens come first.
- **Reuse, do not reinvent:** `internal/mail` now sends in both locales through the company's SMTP settings —
  report e-mail should go the same way, and a failed send should be recorded the way R222 records one.
  `ops.Triggerable` is the one place a job becomes operator-triggerable; a period-taking trigger belongs there.
- **Screen conventions are set** (R195–R199, R229): route → `*-page` container → views; `useApiMutation` for
  every write; `downloadFile` for every export; `mockApi` for container tests; stories stay data-free and own
  their opener; every new screen needs a line in `tests/e2e/responsive.spec.ts`'s `SCREENS` list; `enabled`
  goes in the **options** argument of `$api.useQuery`, not the params object.
- **Never invent a verdict.** R165's placeholder and R216/R219's "karar verilemedi" are the house style for
  anything the data cannot answer.
- **Later phases:** F9 solar plant realtime/production, the iSolar screens **and iSolar alarm forwarding**
  (D-3: `isolar.fetch_alarms`, `GET /plants/{id}/alarms`, `PUT /plants/{id}/alarm-recipients`;
  `isolar_forwarded_alarms` and `MarkIsolarForwarded` are already in F1 and unused); F12 public site; F13 the
  real `/anomaly/check` model; F15 CSP nonces, the supported-browser floor and `EKOKOD_TRUSTED_PROXIES`.

---

## Open questions for the product owner (defaults ship)
- **F7 (new):** **Q-C1** `alarms.edit` includes building_admin (05 §10 says A CA BA), so a BA can create a
  company-level rule over their own meters — company-admin and above instead? · **Q-C2** R213 makes a rule
  visible to a BA when ONE of its analyzers is in scope, and editable too; legacy behaved the same way —
  should such a rule be read-only for them? · **Q-C3** R219 writes no event for a non-firing evaluation; is
  the run summary in Messages enough? · **Q-C4** R214 reads power from `MaxDemandKw` (interval maximum
  demand), the closest thing to legacy's `t_p_kW` — confirm that is what "Güç Maks/Min" means · **Q-C5** R221
  drops legacy's HTML alarm mail for plain text; want the branded HTML body back later (needs a multipart
  sender)? · **Q-C6** the frequency window is per (rule, analyzer) pair; legacy never enforced it at all —
  should one noisy meter suppress the rule's other meters instead? · **Q-C7** Messages has no date-range
  filter although the API takes `from`/`to` — wanted? · **Q-C8** `consumption.refresh` is not triggerable
  because it needs a period; add a period-taking trigger in F8?
- **F6b:** Q-B1 Monday-start detailed-graph weeks · Q-B2 integration credential creation stays admin-only ·
  Q-B3 year-over-year chart only for monthly granularity inside one calendar year · Q-B4 R94's monthly-bucket
  divergence (743 h vs 744 h).
- **F6a:** Q-A1 two-step verification and self-registration are not built · Q-A2 force a password change at
  first login? · Q-A3 password-reset mail uses company SMTP; no platform SMTP · Q-A4 sectoral comparison is
  cross-tenant (peers hidden, ≥ 3 peers, 0.469 kg/kWh) · Q-A5 mobile sessions last 30 days, device-bound.
- **F5:** Q1 customiser theme colour · Q2 RTL · Q3 English UI keeps `1.234,56` · Q4 light `primary` =
  emerald-700 · Q5 production map tile source · Q6 Calendar sits top-level after Reports · Q7 legacy
  catalogues disagree on 115 keys.
- **F4 (unchanged):** real-invoice comparison OPEN; dated `billing_parameters` defaults (R106, reactive basis,
  exemptions, R118, tiering, PTF tolerance, rounding R112); R119, R111, R110, R114, R122, R135; OG transformer loss.
- **F1–F3 (unchanged):** Q1 ratio zero-case, Q3 weekend default, Q5–Q10; F2 device/integration items. Q4
  (calendar events vs the weekend split) is **decided: no** (R137).

## Binding rulings
F1 rulings 1–14, F2 R1–R53, F3 R54–R104, F4 R105–R135, F5 D1–D26, F6a R136–R189, F6b R190–R210,
**F7 R211–R232** (`docs/superpowers/plans/2026-09-18-f7-alarms-messages.md`), plus each phase's deviations.

## Process rules that paid off (keep them, inline)
- **Read legacy code and real data before writing rules.** F7's two hardest decisions — SMS and voltage — were
  settled by grepping the legacy tree and opening the real analyzer export, not by reasoning from the spec.
- **Prove every guard red with your own mutation, restoring from a file backup.** Three survivors this phase
  were findings, not noise: a second line of defence nothing exercised, a test that passed through a different
  path than the one it named, and a redundancy whose real risk was drift.
- **A passing unit test against a fake proves the fake.** Both real defects this phase were found by the e2e
  run after the unit tests were green; one of them existed *because* the fake was kinder than the database.
- **Run the real artefact.** `pnpm build` + Playwright at four widths; and when a UI test fails, ask whether it
  is the test or the product — two of this phase's five e2e failures were my own matchers, two were real bugs,
  and one was an inherited flake proved independent by re-running without the new specs.
- **Per-task gates run the whole touched package**, and the whole web suite before every commit.
- **Commit as soon as tests are green,** then the mutations, then the next task. Comments say WHY in 1–3 lines.

## Environment — read before running anything
- Prefix every command with `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
- WSL: 12 GB RAM, 4 GB swap. **Another project (dolmusum) runs Gradle, vitest and servers on this machine** —
  never kill them, never touch `dolmusum-dev-postgres-1` on 5432, and **do not use port 18080**: it is theirs.
  Load averages above 6 are normal there; Playwright and vitest simply take longer.
- **Never `pkill -f <pattern>` from a Bash call whose own command line contains the pattern** — it kills the
  calling shell. Use `pgrep -x` + `kill`.
- Go integration: shared Postgres + Redis as in the prompt. Test databases have **no** continuous-aggregate
  refresh policies; a test that needs materialised data refreshes explicitly, and a test that asserts the
  policies themselves must migrate its own database (see `policyDB` in
  `internal/store/postgres/migrations_timeseries_integration_test.go`).
- Web (in `web/`): `pnpm lint && pnpm typecheck && pnpm check:api && pnpm check:i18n-parity && pnpm check:contrast`,
  `pnpm test --maxWorkers=2` (~2 min), `pnpm storybook:build` (~1 min), a11y in chunks
  (`STORIES='Features/Alarms,Features/Messages' pnpm test:a11y --workers=1`, ~2.5 min each), `pnpm build` (~1 min),
  `pnpm build && E2E_API_PORT=18082 PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1` (~2 min).
  `make openapi` regenerates both the Go document and `schema.d.ts`. Never leave `next dev`/`next start`,
  `storybook dev`, `vitest --watch`, a static server or `bin/ekokod-e2e` running.
- Worktrees on the native filesystem (`/home/personal/...`) only; `/mnt/c` is slow.
- `make check-generate` needs a clean tree for `internal/store/postgres/sqlcgen`, so commit first.
- Commit trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.

## Key paths
```
docs/superpowers/plans/2026-09-18-f7-alarms-messages.md           F7 plan (R211–R232, D-1…D-5, Q-C1…Q-C8)
docs/superpowers/plans/2026-09-17-f6b-core-screens.md             F6b plan (R190–R210)
internal/domain/alarm/                                            pure evaluation (alarm, evaluate, window, frequency, message)
internal/service/alarms/                                          CRUD, evaluate, run, notify, dispatch, job adapters
internal/service/ops/                                             messages, job runs, allow-listed triggering
internal/job/alarm.go, internal/scheduler/scheduler.go            the three tasks and the hourly tick
internal/mail/templates/alarm_fired.*.txt                         the alarm e-mail, both locales
internal/api/v1/{handlers_alarms.go,handlers_ops.go,dto/{alarms,ops}.go}
internal/store/postgres/{alarms.go,ops.go,admin/alarms.go}
internal/seed/alarms.go                                           e2e rules, events, messages and job runs
web/src/features/{alarms,messages}/
web/tests/e2e/{alarms,messages,responsive}.spec.ts
/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite/.superpowers/sdd/2026-09-18-f7/inline-ledger.md
```

## Verification at the F7 tip
- **Go `-race`, package by package:** domain/alarm, service/{alarms,ops}, mail, job, scheduler, worker, auth,
  api/v1 (+kit, mw), store (+postgres, pgerr, pgnum, redis), seed, arch — all `ok`.
- **Integration** (`-tags=integration`, shared Postgres/Redis): api/v1, service/alarms, store/postgres, seed,
  credentials — all `ok`.
- `golangci-lint run --concurrency 2 --build-tags=integration ./internal/...` → **0 issues**.
  `make check-generate` and `make openapi` → no diff.
- **Web:** `pnpm lint` 0, `pnpm typecheck` 0, `pnpm check:api` ok, `pnpm check:i18n-parity` **19 namespaces,
  978 keys**, `pnpm check:contrast` **75 pairs × 2 themes**, `pnpm test --maxWorkers=2` **189 files, 702 tests**,
  `pnpm audit --audit-level=high` no known vulnerabilities, `pnpm build` ok.
- **a11y:** `pnpm storybook:build`, then `STORIES='Features/Alarms,Features/Messages'` → **132 passed**.
- **e2e:** `pnpm build && E2E_API_PORT=18082 … --workers=1` → **49 passed, 1 failed** — the failure is the
  inherited `calendar › a company admin deletes the event again` flake, proved independent of F7 by a control
  run without the new specs (38 passed, 1 failed, same test). All 11 new alarm and message e2e tests pass, and
  `responsive.spec.ts` now walks seven screens at four widths.
- No dev server, `next start`, Storybook, static server or `bin/ekokod-e2e` left running by this session.
