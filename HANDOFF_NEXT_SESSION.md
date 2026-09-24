# Handoff — ekokod rewrite: F0–F14 COMPLETE, F15a (security) + F15b (performance, observability) done, F15c next

> **STATUS (2026-09-24, ~11:40 Istanbul):** every phase F0–F14 is built. Docker came back this
> session and **every Docker-bound gate of F9–F14 is green** (integration `-race` package by package,
> the full e2e suite with the worker 112/112, the ML image + in-image nodb). F14 is done in three
> parts: F14a (inventory/extract/transform), F14b (load, comparison tables, remaining collections,
> artifacts), F14c (recompute, reconcile, runbook, rehearsal target).
> **F15a (security) and F15b (performance + observability) are done** on `phase/f15-hardening`
> (`/home/personal/ekokod-f15-phase`). **Next: F15c** (backup/restore, offline bundle, a11y audit,
> handover docs). **First, a product decision is waiting — F3 Q6** (see "Open questions").
> **Nothing is pushed and `main` is untouched. Pushing and merging are the user's call.**

---

## Paste this as the first message of the new session

> Bu, bcem-energy'nin (yeni adıyla ekokod) Go ile yeniden yazım projesi. Spesifikasyon `docs/rewrite/`
> içinde, faz planı `docs/rewrite/09-implementation-plan.md` (16 faz, F0–F15).
>
> **Bu oturum uzun ve gözetimsiz; ben bilgisayar başında değilim.** Hiçbir şey için durup bana soru
> SORMA. Karar gereken her yerde önerilen/en makul seçeneği seç; seçenekleri, seçtiğini ve yanlışsa
> bedelini planın "Open questions (defaults shipped)" bölümüne ve ledger'a `Ruling:` olarak yaz, devam et.
> Geri alınamaz ya da dışa dönük bir işlem gerekirse YAPMA; kaydet ve etrafından dolaş: push, merge,
> `main`'e dokunmak, başka projenin container/port/süreçlerine dokunmak, Docker'ı yeniden başlatmak.
>
> **Durum:** F0–F14 tamam; F9–F14'ün Docker kapıları yeşil; **F15a (güvenlik) ve F15b (performans +
> gözlemlenebilirlik) tamam**. Son dal
> `phase/f15-hardening` (`/home/personal/ekokod-f15-phase`). Hiçbir şey push edilmedi.
>
> **Çalışma düzeni (kesin):** paralel agent YOK, subagent YOK, Workflow YOK. Fazı bu oturumda kendin
> yürüt (`superpowers:executing-plans` + TDD). Her task sonunda diff'i oku ve guard'ın kırmızıya
> düştüğünü kendi mutasyonunla kanıtla. Faz sonunda bütün-faz self-review (önce yetki/tenancy, sonra
> erişilebilirlik ve tasarım) ve worker'lı gerçek e2e koş.
>
> **Sıra:**
> 1. `/home/personal/ekokod-f15-phase/HANDOFF_NEXT_SESSION.md`'i baştan sona oku.
> 2. **F15c'yi planla ve uygula** (09 §F15): yedekleme ve **geri yükleme** (`make backup`,
>    `scripts/test-restore.sh`; scratch host yoksa ayrı bir Postgres container'ına geri yükleyip
>    servisin sağlıklı açıldığını kanıtla, gerçek host'u PENDING yaz), offline bundle (`make
>    offline-bundle`, `install.sh` upgrade yolu, `scripts/test-offline-install.sh`; ML imajı ve web imajı
>    bundle'da mı; docker-compose'da `EKOKOD_*_METRICS_ADDR=0.0.0.0:946x` — F15b notu), tam a11y denetimi
>    (07 §9; Storybook a11y + sayfa bazında axe), dokümantasyon (operatör runbook, yönetici kılavuzu,
>    OpenAPI'den statik API referansı, ADR'ler; `docs/` altındaki her belge güncel olmalı).
>    **ÖNCE (F15c'den önce, kendi planıyla — "F15q"):** F3 Q6 düzeltmesi. Saatlik tüketim serisi
>    saatlik sayaçlarda sıfır, günlük/aylıkta her kovada bir aralık kayıp (legacy OSOS yük profilleri
>    saatlik). Tasarım notları `docs/performance.md` "Findings"te: dört consumption aggregate'ine her
>    register için `first()` ekleyen bir migration + analytics'te bir kova ileriye bakan sınır farkı
>    (R87 gibi); F3'ün "analytics ile billing tam bir adım farklı" kabul testi yeniden kararlanmalı.
>    Önerilen düzeltmeyi uygula, Ruling olarak yaz; faturalama etkilenmiyor.
> 3. Süre ya da bağlam biterken yarım task bırakma; son temiz task sınırında dur: ledger + bu dosya
>    ("START HERE", yarım iş, sıradaki adım) + Türkçe yapıştırma promptu güncel, commit et, dur.
>
> **Ortam ve güvenlik:**
> - Her komutun başına: `export PATH="/home/personal/.local/go/bin:/home/personal/.local/node/bin:/home/personal/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`. JVM/Gradle kullanılırsa heap'i sınırla.
> - Docker komutlarını `sg docker -c "…"` ile sar; `$PATH`'i `sg` string'ine gömme (Windows yollarındaki
>   parantez `sh`'i bozar) — betik dosyası yaz, `sg docker -c /yol/betik.sh` ile çalıştır.
> - **Asla `make test` veya `go test ./...` çalıştırma.**
> - Integration: önce `sg docker -c "docker ps -a"`; Exited ise `sg docker -c "docker start ekokod-test-pg ekokod-test-redis"`.
>   Sonra `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <paket> -tags=integration -count=1 -parallel 4`.
>   `internal/scheduler`, `internal/worker`, `internal/job`, `internal/platform/lock` kendi container'larını açar; tek tek koş.
> - Web: `pnpm test --maxWorkers=2`; `next build` için `NODE_OPTIONS=--max-old-space-size=4096`.
> - Playwright: `PLAYWRIGHT_BROWSERS_PATH=/home/personal/.cache/ms-playwright`, `E2E_API_PORT=18082`, `PW_PROJECT=e2e`;
>   önce `pnpm build`; ağır koşular ön planda. Storybook, Playwright ve `next build` aynı anda çalışmasın.
> - Başka projenin container/port/süreçlerine dokunma (dolmusum-* container'ları ve başka kullanıcıların `go test` süreçleri var).

---

## START HERE — exact state (2026-09-24 ~11:40)

| Branch / worktree | Head | State |
|---|---|---|
| `phase/f9-solar` … `phase/f13-ml` (`/home/personal/ekokod-f9-phase` … `-f13-phase`) | unchanged | code done; their Docker gates were run **on the F14 tip** this session (the tip carries them) and fixed there |
| `phase/f14-migration` (`/home/personal/ekokod-f14-phase`) | `d07b99d` + handoff | **F14 done** (a + b + c) |
| **`phase/f15-hardening`** (`/home/personal/ekokod-f15-phase`) | this commit | **F15a + F15b done** (plans `…-f15a-security-hardening.md` R450–R456, `…-f15b-performance-observability.md` R460–R469; ledgers under `.superpowers/sdd/2026-09-24-f15{a,b}-…/`); **F15c next** |

### What this session did
1. **Gates (Docker back):** the F9–F14a integration suite ran with `-race` package by package; e2e
   with the worker; the ML image. Defects the first runs found, all fixed:
   - `8202435` — API harness had no upload allow-list; the tenancy sweep sent `{}` to ISO note update;
     the carbon test lacked the master catalogue and expected a client emission to be ignored (the
     strict decoder refuses it); seed pinned 0 national-schedule rows (R350 ships 17).
   - `3a5aea2` — `forecast.AnomalyResult` embedded float64s (arch no-float guard); the scheduler test
     lacked F13's Forecast cron.
   - `be0a05e` — e2e 112/112: MetricCard's value/unit had no real space; the bills plants table had no
     caption; seven specs brought up to what shipped; Playwright now passes `EKOKOD_PUBLIC_URL`.
   - ML image builds; `python -m ekokod_ml.nodb` in the image prints `database clients: none`.
2. **F14b** (plan `docs/superpowers/plans/2026-09-24-f14b-legacy-load.md`, R413–R429, Q-J7…Q-J15):
   migration 00020 (`legacy_ids`, `legacy_bills`, `legacy_reports`); `ekokod migrate legacy load`
   (jsonb COPY + MERGE on the PK; hypertables insert…on conflict; defaults on insert, never clobbered;
   withheld readings for unconfirmed multipliers); transforms for tariffs (+ classification answers),
   templates, SMTP, EPİAŞ, bill history, reports, logs, plants, alarms, carbon, ISO 50001;
   `ekokod migrate legacy artifacts`. Self-review fixed a cross-company responsible user (F14a), the
   credential/plant load order, and silently skipped table files.
3. **F14c** (plan `…-f14c-recompute-reconcile.md`, R430–R444, Q-K1…Q-K7): `ekokod recompute
   consumption|bills|reports|carbon|forecasts` (worker handlers in-process, bounded parallel,
   failures collected, idempotent without `--force`); `ekokod migrate legacy reconcile` (HTML/CSV/JSON,
   component-by-component attribution, `unexplained` blocks); `docs/runbook-migration.md`;
   `make migration-rehearsal` (dry-run on the fixture: all 11 steps executed).
   Ledgers: `.superpowers/sdd/2026-09-24-f14b-legacy-load/progress.md`, `…/2026-09-24-f14c-recompute-reconcile/progress.md` (git-ignored, local, in the F14 worktree).

### PENDING — needs something this machine does not have
- **An anonymised production dump + a host:** the MongoSource integration test, a real inventory, the
  **three rehearsals** with timings (runbook §9), the rollback exercised on the rehearsal environment.
- **Live providers:** migrated credentials authenticating (08 §9).
- **F4:** the real-invoice comparison is still OPEN.

## Open questions (defaults shipped)
- **⚠ F3 Q6 — needs a product decision (confirmed at scale in F15b):** the consumption API's hourly series
  is `last − first` inside each hour, so it is **zero for meters that report hourly** (legacy OSOS load
  profiles are hourly) and ~25 % low for 15-minute meters; daily/monthly lose one interval per bucket.
  Billing is unaffected. Recommended fix and evidence: `docs/performance.md` → Findings.
- **F15a** Q-L1…Q-L4, **F15b** Q-M1…Q-M4 (volume assumption, 500 ms p95 budget, metrics addresses, soak PENDING).
- **F14b** Q-J7…Q-J15 and **F14c** Q-K1…Q-K7: in their plans; the ones the PO should look at first:
  Q-J7 (tariff classification answers), Q-J9 (ISO work moves from a user to a building), Q-J12 (plant
  monthly roll-ups dropped), Q-K5 (four reasons added to 08 §8's list).
- **F9:** F-1…F-3 answered by the PO. **Unchanged:** Q-E1…Q-E7, Q-D1…Q-D7, Q-C1…Q-C7, Q-B1…Q-B4,
  Q-A1…Q-A5, F5 Q1–Q7, F10–F13's Q-F/Q-G/Q-H/Q-I lists in their plans.

## Binding rulings
F1 1–14, F2 R1–R53, F3 R54–R104, F4 R105–R135, F5 D1–D26, F6a R136–R189, F6b R190–R210,
F7 R211–R232, F8a R233–R253, F8b R254–R275 + E-1…E-3, F9 R276–R299 + F-1…F-3, F10 R300–R327,
F11 R330–R346, F12 R350–R356, F13 R360–R385, F14a R400–R412, **F14b R413–R429, F14c R430–R444**, plus
each phase's ledgered rulings, **F15a R450–R456, F15b R460–R469**. **F15c continues from R470.**

## F15 (09-implementation-plan.md §F15) — what to plan
Performance (2× load on API + jobs, chunk exclusion in hypertable plans), security review (dependency
audit, secrets, authorisation matrix re-verification, uploads, path traversal, rate limits, audit-log
completeness), full a11y audit (07 §9), observability (dashboards: HTTP latency/errors, job success and
duration, queue depth, integration health; alert thresholds), backup **and restore** tested on a scratch
host, offline bundle (`make offline-bundle`, `install.sh`, upgrade path, tested with networking off),
documentation (operator runbook, admin guide, API reference from the OpenAPI document, ADRs). Also owns
CSP nonces, the browser floor, `EKOKOD_TRUSTED_PROXIES` and rate limits. Things this machine cannot do
(a real scratch host, networking off) are recorded PENDING, never faked.

## Process rules that paid off
- **Run the real stack with a worker before calling a phase done.** This session's first full e2e run
  found two product defects and seven drifted specs that unit tests could not see.
- **A per-task gate is the whole touched package, including `internal/cli`.**
- **Prove every guard red with your own mutation,** restoring from a file backup. When a mutation shows
  nothing, check that it compiled (twice this session a mutation did not compile, and once a fixture had
  no case to exercise it).
- **When code came before its test,** say so and prove the test by mutation.
- **Idempotency tests compare bytes:** a `bson.M` marshals in random key order — use sorted JSON.
- **Nullable DTO fields are optional:** no `required:"true"` on pointers. **ICU:** double `''` before `{`.

## Environment surprises worth keeping
- Docker was hung ~00:25–09:55 and came back after a restart: the test containers were `Exited` and the
  test Postgres was empty (tmpfs). Start them; migrations run per test DB.
- Another user's `go test ./...` and testcontainers run on this Docker: a `docker.sock … context deadline
  exceeded` in a container-owning package is load, not code — rerun the package alone.
- This worktree does not record file modes: a new `scripts/*.sh` needs `git update-index --chmod=+x`
  (`make check-script-modes`).
- `golangci-lint` wants `gofmt -s` (composite literal simplification), not just `gofmt`.
- Each new worktree needs `git config --global --add safe.directory <path>` (done for f15).
- The shims `/tmp/claude-1003/bin/{pgq,psql,redis-cli}` let scripts run without the Docker CLI;
  rebuild `pgq` from `.superpowers/sdd/2026-09-23-f9-solar-renewable-financial/shim/pgq` if `/tmp` was cleaned.
- `pdftoppm` is missing; the Read tool opens PDFs.

## Key paths (F14)
```
internal/migrate/legacy/        inventory, extract, transform_*.go, load.go, artifacts.go, reconcile*.go, answers.go
internal/migrate/recompute/     recompute orchestration
internal/store/postgres/admin/  legacyload.go (loader), reconcile.go (read-only queries), aggregates.go (+plant refresh)
internal/cli/                   migrate_legacy.go, recompute.go
internal/store/postgres/migrations/00020_legacy.sql
docs/runbook-migration.md, scripts/migration-rehearsal.sh, make migration-rehearsal
```
