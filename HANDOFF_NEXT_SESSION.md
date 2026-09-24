# Handoff — ekokod rewrite: F1–F13 COMPLETE, F14a (legacy pipeline core) done, F14b next (Docker-bound gates of F9–F14 pending)

> **STATUS (2026-09-24, ~00:50 Istanbul):** F9 (solar plants, renewable energy, financial analysis)
> is on `phase/f9-solar` in `/home/personal/ekokod-f9-phase`, branched from `phase/f8b-reports`.
> 23 commits after the plan `de5d2d1`. Tasks 1–15 are **complete** and ledgered. **Task 16** (seed, e2e,
> a11y) is **half done**. Its first e2e run found three real defects; they are fixed and committed
> (`9c746ca`). Then **the Docker daemon stopped answering** and every `refresh_continuous_aggregate`
> in the test Postgres hung, so nothing after those fixes has been re-verified. **Task 17** (the
> whole-phase self-review) has **not started**.
> **Next session:** a long unattended run (5–6 h, the user is away). It should finish F9, then do **as many
> of F10–F15 as fit**, one phase after another, **never stopping to ask**.
> **Nothing is pushed and `main` is untouched. Pushing and merging are the user's call.**

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/`
> içinde, faz planı `docs/rewrite/09-implementation-plan.md` (16 faz, F0–F15).
>
> **Bu oturum uzun ve gözetimsiz (5–6 saat); ben bilgisayar başında değilim.** Hiçbir şey için
> durup bana soru SORMA. Karar gereken her yerde önerilen/en makul seçeneği seç. Seçenekleri, seçtiğini
> ve yanlışsa bedelini planın "Open questions (defaults shipped)" bölümüne ve ledger'a `Ruling:`
> olarak yaz, sonra devam et. Geri alınamaz ya da dışa dönük bir işlem gerekirse onu YAPMA; kaydet
> ve etrafından dolaş. Bunlar: push, merge, `main`'e dokunmak, başka projenin
> container/port/süreçlerine dokunmak, Docker'ı yeniden başlatmak.
>
> **Durum:** F1–F13 kodu tamam, F14a (legacy inventory/extract/transform) tamam; F14b sırada.
> Son dal: `phase/f14-migration` (`/home/personal/ekokod-f14-phase`). Docker daemon hâlâ askıda:
> F9–F14'ün Docker'a bağlı kapıları (integration testleri, e2e, image build) PENDING — listesi
> bu dosyanın "START HERE" bölümünde. Hiçbir şey push edilmedi, `main`'e dokunulmadı.
>
> **Çalışma düzeni (kesin):** paralel agent YOK, subagent YOK, Workflow YOK. Her fazı bu oturumda
> kendin yürüt (`superpowers:executing-plans` + TDD). Her task sonunda diff'i oku ve guard'ın
> kırmızıya düştüğünü kendi mutasyonunla kanıtla. Faz sonunda bir bütün-faz self-review yap (önce
> yetki/tenancy, sonra erişilebilirlik ve tasarım) ve worker'lı gerçek e2e koş.
>
> **Sıra:**
> 1. `/home/personal/ekokod-f14-phase/HANDOFF_NEXT_SESSION.md`'i baştan sona oku; önce "START HERE" ve
>    "Docker hang". Docker ayağa kalktıysa PENDING kapıları sırayla koş (F9 → F14).
> 2. **F14b'yi planla ve uygula** (F14a planı ve ledger'ı: `docs/superpowers/plans/2026-09-24-f14a-legacy-pipeline.md`,
>    `.superpowers/sdd/2026-09-24-f14a-legacy-pipeline/progress.md`), sonra **F14c**, sonra **F15**.
> 2b. (Eski sıra, yalnızca referans:) **F9'u bitir:**
>    - Ortamı kontrol et ("Docker hang" bölümü).
>    - Doğrulanmamış kapıları koş.
>    - Task 16'yı tamamla: yeni e2e spec'leri, worker'lı tam e2e, yeni story'lerin a11y koşusu. Ardından `task-done` çalıştır.
>    - Task 17'yi yap: bütün-faz self-review. Kritik/önemli hataları RED→GREEN düzelt, küçükleri ertelenenlere yaz. Doğrulama bloğunu çalıştır.
>    - Bu dosyanın F9 bölümünü "tamam" olarak güncelle ve commit et.
> 3. **Sonra sırayla F10 → F11 → F12 → F13 → F14 → F15.** Her faz için:
>    - Bir önceki fazın dalından yeni bir worktree/dal aç. Örnek:
>      `git worktree add -b phase/f10-carbon /home/personal/ekokod-f10-phase phase/f9-solar`,
>      sonra `git config --global --add safe.directory …`, sonra `web/` içinde `pnpm install --frozen-lockfile`.
>    - Legacy kodu ve spesifikasyonu okuyup sıkı bir plan yaz (`superpowers:writing-plans`).
>    - Planın soracağı soruları önerilen seçenekle kendin cevapla ve "Open questions (defaults shipped)" altına yaz.
>    - Planı bir kez spesifikasyona karşı gözden geçir.
>    - Uygula, self-review yap, e2e koş.
>    - Bu dosyayı güncelle, commit et, sonraki faza geç.
> 4. Süre ya da bağlam biterken yarım task bırakma; son temiz task sınırında dur.
>    - Ledger'a ve bu dosyaya tam durumu yaz: "START HERE" tablosu, yarım kalan iş, sıradaki adım.
>    - Yeni oturum için Türkçe yapıştırma promptunu güncelle.
>    - Commit et ve dur.
>
> **Ortam ve güvenlik:**
> - Her komutun başına şunu ekle:
>   `export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
>   JVM/Gradle kullanılırsa heap'i sınırla.
> - Docker komutlarını `sg docker -c "…"` ile sar. `$PATH`'i `sg` string'inin içine gömme: Windows
>   yollarındaki parantez `sh`'i bozuyor. Gerekirse komutu bir betik dosyasına yaz ve
>   `sg docker -c /yol/betik.sh` ile çalıştır.
> - **Asla `make test` veya `go test ./...` çalıştırma.**
> - Integration testleri:
>   - Önce `sg docker -c "docker ps -a"`; Exited ise `sg docker -c "docker start ekokod-test-pg ekokod-test-redis"`.
>   - Sonra: `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <paket> -tags=integration -count=1 -parallel 4`.
>   - `internal/scheduler`, `internal/worker` ve `internal/job` kendi container'larını açar; bunları tek tek koş.
> - Web: `pnpm test --maxWorkers=2`.
> - Playwright: `PLAYWRIGHT_BROWSERS_PATH=/home/personal/.cache/ms-playwright`, e2e portu
>   `E2E_API_PORT=18082` (18080 dolmusum'un). Ağır Playwright koşularını ön planda ve parça parça çalıştır.
> - Storybook, Playwright ve `next build` aynı anda çalışmasın.
> - Başka projenin container/port/süreçlerine dokunma.

---

## START HERE — exact state (2026-09-24 ~03:45)

| Branch / worktree | Head | State |
|---|---|---|
| `phase/f9-solar` (`/home/personal/ekokod-f9-phase`) | `fa70092` | F9 code + self-review done; Docker-bound gates PENDING (see "F9 remaining") |
| `phase/f10-carbon` (`/home/personal/ekokod-f10-phase`) | `f95a67d` | F10 done (carbon backend + screens); Docker-bound gates PENDING |
| `phase/f11-iso50001` (`/home/personal/ekokod-f11-phase`) | `32f7427` | F11 done (ISO 50001 backend + screens); Docker-bound gates PENDING |
| `phase/f12-site` (`/home/personal/ekokod-f12-phase`) | `9c61937` | **F12 done** (F12a backend + F12b public site); API-backed public e2e PENDING (Docker) |
| `phase/f13-ml` (`/home/personal/ekokod-f13-phase`) | `5efd328` | **F13 done** (ML service, Go forecasting, Predict + AI screens); Docker-bound gates PENDING |
| **`phase/f14-migration`** (`/home/personal/ekokod-f14-phase`) | this commit | **F14a done** (legacy inventory/extract/transform); **F14b next** (load, comparison tables, remaining collections, artifacts) |

- Plans and ledgers:
  - F10a `docs/superpowers/plans/2026-09-24-f10a-carbon-backend.md` (R300–R319);
  - F10b `…-f10b-carbon-screens.md` (R320–R327);
  - F11a `…-f11a-iso50001-backend.md` (R330–R341);
  - F11b `…-f11b-iso50001-screens.md` (R342–R346);
  - each with `.superpowers/sdd/<plan>/progress.md` (rulings, mutations, PENDING lists).
- **Docker is still hung** (since 00:25). When it is back, run in this order:
  1. F9 "remaining".
  2. F10a integration (store carbon, api carbon + sweep, cli verify, seed, worker, scheduler).
  3. F11a integration (`TestFileAuthorization`, `TestISOProjectFlow`, sweep iso50001, seed counts).
  4. e2e: `carbon.spec.ts`, `iso50001.spec.ts`, `responsive.spec.ts`, then the full suite with the worker.
- F11 also maps the F10 carbon validation codes to specific form messages (`web/src/lib/api/problem.ts`).
- F12a plan `docs/superpowers/plans/2026-09-24-f12a-public-backend.md` (R350–R356, Q-H1…Q-H6).
- F12b plan `docs/superpowers/plans/2026-09-24-f12b-public-site.md` (Q-H7…Q-H11); ledger lists every ruling. Done:
  - `(site)` route group at the root (`/`, `/about`, `/references`, `/documents`, `/toolkit`, `/pricing` behind its flag, `/request-demo`, `/contact`, `/blog`, `/blog/[slug]`, `/bill-calculator`); legacy `/site/*` → 308;
  - only genuine legacy content (template testimonials, placeholder leaders and unverifiable metrics dropped);
  - blog: `marked` 18, articles in `web/content/blog/`, images WebP in `web/public/site/`;
  - forms validate R353 client-side; calculator shows every line + Q-H3/Q-H4 rules;
  - SEO: title template, OG, canonical, JSON-LD, sitemap, robots; analytics = self-hosted script via env (off).
  - Verified: web unit 1118/1118, site a11y sweep 186/186, smoke of API-free public e2e 18/18 (next start, API absent), Lighthouse mobile perf 96–98 / a11y 100 / BP 100 / SEO 100, LCP 2.5–2.8 s simulated (measured ~100 ms), CLS 0.
  - PENDING (Docker): `PW_PROJECT=e2e pnpm exec playwright test tests/e2e/public/` (signed-in header, calculator vs seeded schedule, contact 503, honeypot 202, Q-H4); F12a integration tests.
- F13 plans: `…-f13a-ml-service.md` (R360–R369, Q-I1…Q-I7), `…-f13b-forecast-backend.md` (R370–R379, Q-I8…Q-I14), `…-f13c-forecast-screens.md` (R380–R385, Q-I15…Q-I18); ledgers under `.superpowers/sdd/`.
  - `ml/`: FastAPI service, uv project (**uv lives at `/home/personal/.local/uv/uv`**, installed from its PyPI wheel; system python has no pip). `cd ml && /home/personal/.local/uv/uv run pytest -q` → 39 passed, 1 skipped (LSTM needs the optional `lstm` extra; verified once with ephemeral CPU torch).
  - Models: seasonal-naive baseline (always), random forest, LSTM (extra), Chronos (sidecar URL). No DB client: `python -m ekokod_ml.nodb`, CI job `ml`.
  - Go: `internal/ml` client (TestGracefulDegradation), `internal/service/forecast` (run/read/weekly/monthly/anomaly, job `forecast.run` at `Schedule.Forecast`), routes `GET /forecast`, `POST /forecast/{run,weekly,monthly}`, `/anomaly/check` now real (still `available:false` when ML is down).
  - Web: `/ekorm/forecast` (Predict), `/ekorm/ai` (AI Analysis, `forecast.run` only), `/ekorm/predict` redirects.
  - PENDING (Docker): `docker build ml` + in-image nodb; `go test -tags=integration ./internal/api/v1 -run 'Forecast|AnomalyCheck'`; `./internal/worker -run ForecastRun`; e2e `tests/e2e/predict.spec.ts`.
  - Known pre-existing minor: the Messages page trigger list shows 4 of 10 triggerable jobs.
- F14 is split: **F14a** (done) `docs/superpowers/plans/2026-09-24-f14a-legacy-pipeline.md` (R400–R412, Q-J1…Q-J6); **F14b** load + `legacy_ids`/`legacy_bills`/`legacy_reports` migration + the remaining collections (tariffs, plants, alarms, carbon, ISO 50001, EPİAŞ, SMTP, logs, templates) + `migrate legacy artifacts`; **F14c** `ekokod recompute …`, `migrate legacy reconcile` (HTML/CSV, attributed reasons), `docs/runbook-migration.md`, `make migration-rehearsal`.
  - `internal/migrate/legacy`: ids (UUIDv5), normalisation (dates/numbers/enums), rejection ledger with the input = accepted + rejected invariant, legacy crypto (aes-256-cbc, fallback key) + re-sealing under the credential AAD, read-only `Source` (Mongo adapter behind a write-refusing command monitor; `MemSource`; `ExtractSource`), deterministic extract + manifest, inventory, transform (integrations, companies + weekend/vacations/events/credentials, users with `legacy$` bcrypt, buildings + contacts, analyzers, meter readings via the ingestion mappers `osos.MapStoredRow`/`gridbox.MapStoredRow`, index drops → `consumption_anomalies`).
  - CLI: `ekokod migrate legacy inventory [--uri --db | --from-extract DIR] [--artifacts a,b] [--json f]`, `extract --uri --db --out`, `transform --extract --out [--confirmations manual_multipliers.csv] [--now RFC3339]` (needs `EKOKOD_LEGACY_ENCRYPTION_KEY`, optional `EKOKOD_LEGACY_ENCRYPTION_KEY_FALLBACK`).
  - Key rulings: legacy ARIL values were stored pre-multiplied; legacy OSOS never applied `meterMultiplier`, so OSOS × m>1 is **undetermined** → `manual_multipliers.csv`, `multiplier_confirmed=false` on the analyzer row (F14b load must refuse those readings until answered). Two-digit years are ambiguous; d/m/y reads day-first (legacy never wrote month-first). `ekokod migrate` stays the schema migrator; legacy lives under `migrate legacy`.
  - BLOCKED here (not faked): no legacy dump/Mongo/Docker → the MongoSource integration test, a real inventory, and the **three rehearsals** of 09 §F14.

## Docker hang (read before any integration/e2e)
- **What happened (at ~00:25):**
  - The Docker daemon stopped answering: `docker ps` and `curl --unix-socket /var/run/docker.sock http://localhost/_ping` both time out.
  - The test Postgres still accepted connections and `select 1`. Go integration tests that do not refresh aggregates still passed.
  - Every `refresh_continuous_aggregate` hung, and `pg_terminate_backend` was ignored.
  - Stuck sessions were left in `ekokod_e2e` and in an `isolated_db_*`, plus `DROP DATABASE` calls waiting on them.
  - It looks like an I/O stall in the Docker VM.
  - **Docker was not restarted,** because it hosts other projects' containers.
- **First check:**
  1. Ping Docker: `timeout 15 sg docker -c "curl -s --unix-socket /var/run/docker.sock http://localhost/_ping"` should print `OK`.
  2. List the Postgres sessions:
     `/tmp/claude-1003/bin/pgq 'postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' "select pid, datname, state, now()-query_start, left(query,60) from pg_stat_activity where datname is not null"`.
     If `/tmp` was cleaned, rebuild it: `go build -o /tmp/claude-1003/bin/pgq ./.superpowers/sdd/2026-09-23-f9-solar-renewable-financial/shim/pgq`.
- **If it recovered** (the user may have restarted the machine): the containers may be Exited. Start them and continue normally.
- **If it is still hung:**
  - Do not restart Docker. Wait and re-check now and then.
  - Meanwhile, do the work that needs no aggregate refresh: planning, pure/unit tests, web unit tests, lint, typecheck and the self-review reading.
  - Record every unrun gate as pending in the ledger and the handoff. Never claim them green.
- **Running e2e without the Docker CLI:**
  - `web/scripts/e2e-api.sh` uses `psql`/`redis-cli` when they are on `PATH`, otherwise `docker exec`.
  - Git-ignored stand-ins live in the ledger folder's `shim/` (`psql`, `rediscli`, `pgq`). Build them into a bin dir and put it first on `PATH`.
  - E2E still needs Postgres to be unstuck, because it refreshes aggregates.

## F9 remaining (updated 2026-09-24 ~01:50)
**Code and self-review are done. Only the gates that need Docker are still pending.**
- **Done this session** (commits `dba8c58`, `2c2f820`, `5e55688`, `76ab358`):
  - i18n parity fix.
  - Deactivated iSolar credential now stops link, sync and fault fetch (new closed code `credential_inactive`).
  - Worker wires an asynq inspector, so an archived plant sync is replaced by the `*/15` dispatch.
  - 4 a11y fixes.
  - Self-review written to the ledger (`Final:` lines, 4 deferred minors).
- **Green this session:** Go lint, check-generate/openapi, Go unit tests, web lint/typecheck/i18n/unit tests, `pnpm build`, F9 a11y stories (292/292).
- **PENDING — needs Docker/Postgres healthy:**
  1. `-race` integration: `internal/api/v1`, `internal/apiwire`, `internal/seed`, `internal/cli/...`, `internal/service/{renewable,solar,jobs,financial}`, `internal/worker` (`TestBuildGraphOpensNamedClosers` now expects `job-inspector`).
  2. E2E: `solar-plants`, `renewable`, `financial`, `bills`, then the full suite with the worker.
  3. Then `task-done PLAN 16 757c73f -- <e2e cmd>` and mark Task 17 complete in the ledger.

## What F9 built (short)
- **Data:**
  - `plant_production_totals`, with daily and monthly aggregates over it.
  - iSolar link columns on `power_plants`, plus `netting_analyzer_id`.
  - Device snapshot columns.
  - `plant_faults`.
  - Four R293 equivalence rows in `emission_factors` (186 total).
- **iSolar adapter:**
  - Converts cumulative yield to intervals.
  - Reads the plant daily series and device realtime.
  - Reads faults with occurrence refs.
- **Jobs:**
  - `isolar.dispatch_sync` (`*/15`).
  - `isolar.sync_plant` (400-day backfill on link).
  - `isolar.fetch_alarms` (`10 * * * *`); faults are forwarded once by mail (claim/release) with a Turkish translation.
- **Routes:**
  - Plant: realtime, production, export, devices, alarms, revenue, sync, link, unlink, recipients.
  - `/weather`: Open-Meteo, optional; an air-gapped install says so.
  - Eight `/renewable/*` panels. Every field's source is listed in `renewable.Sources`, checked by `TestNoSyntheticData`.
  - `/financial/{summary,monthly}`. R294: net = cost − revenue, always.
  - The bills dashboard plant section (R290).
- **Web:**
  - `/ekorm/solar-plants`: four tabs; the iSolar link dialog and netting select live in settings.
  - `/ekorm/renewable-energy`: summary, production tab, seven panels, energy balance, weather.
  - `/ekorm/financial-analysis`: headline, netting card, tariff panel, chart, 12-month table.
  - Nav gates: `renewable.read`, `nav.financial`, `nav.solar_plants`.
- **Seed:** the e2e grid plant is linked to a stub credential, so its sync fails honestly. It has two inverters, 60 days of totals, two faults, a netting analyzer and a feed-in tariff.

## Defects F9 found in earlier work
- **F2:**
  - The cumulative "yield today" point was stored as an interval.
  - Plants were asked for inverter points.
  - The fault ref was the fault type.
- **F8a/T8:** `bills.spec.ts` still asserted R235's "no data source" after R290 replaced it.
- **F8b:** the report's grid-factor unit display (see above).

## Open questions (defaults shipped)
- **F9:** F-1…F-3 were answered by the product owner:
  - Netting analyzer only.
  - A separate totals table.
  - A BA may read company plants in the archive.
- **Unchanged:** Q-E1…Q-E7, Q-D1…Q-D7, Q-C1…Q-C7, Q-B1…Q-B4, Q-A1…Q-A5, F5 Q1–Q7.
- **F4:** the real-invoice comparison is still OPEN.
- **New phases:** add each phase's questions here with the default you shipped.

## Binding rulings
F1 1–14, F2 R1–R53, F3 R54–R104, F4 R105–R135, F5 D1–D26, F6a R136–R189, F6b R190–R210,
F7 R211–R232, F8a R233–R253, F8b R254–R275 + E-1…E-3, **F9 R276–R299 + F-1…F-3** (plan above), plus each
phase's ledgered rulings. New phases continue from **R300**.

## Later phases (09-implementation-plan.md)
- F10 carbon footprint. Reuse R262's grid-factor lookup (company override first).
- F11 ISO 50001.
- F12 public marketing site.
- F13 ML service. Owns `/anomaly/check`.
- F14 migration and cutover tooling. Depends on F1–F11.
- F15 hardening, packaging, handover. Owns CSP nonces, the browser floor, `EKOKOD_TRUSTED_PROXIES` and rate limits.

## Process rules that paid off
- **Run the real stack with a worker before calling a phase done.** F9's e2e run found three defects no unit test caught: an API panic, a failure shown without its reason, and a misleading unit.
- **A per-task gate is the whole touched package, including `internal/cli`.** T13 missed the CLI's pinned factor count.
- **Prove every guard red with your own mutation,** restoring from a file backup. When a mutation shows nothing, check that it compiled.
- **When code came before its test,** say so and prove the test by mutation.
- **Nullable DTO fields are optional:** no `required:"true"` on pointers.
- **ICU:** an apostrophe before `{` must be doubled (`''`).

## Environment surprises worth keeping
- The Docker hang described above.
- Docker restarts leave the test containers `Exited`.
- Container-owning packages time out when they run together; run them one at a time.
- `sg docker -c "…$PATH…"` breaks on Windows path parentheses; use a script file instead.
- Each new worktree needs `git config --global --add safe.directory <path>`.
- `pdftoppm` is missing; the Read tool opens PDFs.

## Key paths (F9)
```
docs/superpowers/plans/2026-09-23-f9-solar-renewable-financial.md   plan (R276–R299)
internal/store/postgres/migrations/00018_solar.sql                  schema
internal/integration/isolar/, internal/integration/weather/         adapters
internal/service/{solar,renewable,financial,weather}/               services
internal/api/v1/{handlers_solar.go,handlers_panels.go,handlers_renewable.go,dto/solar.go,dto/renewable.go}
internal/seed/solar.go                                              e2e solar fixtures
web/src/features/{solar-plants,renewable,financial,weather}/        screens
web/tests/e2e/{solar-plants,renewable,financial,bills}.spec.ts
```
