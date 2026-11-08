# F30 — Custom bill periods (date range preview + saved period), Fuga findings

Branch `feat/f30-custom-bill-period` (worktree `/home/personal/ekokod-f30-period`, from main @29fc8e2).
Trigger: Fuga Mobilya (MEDAŞ/GridBox, SKTT PTF tariff) bills did not match MEPAŞ; meter reads on 17/18/16 of
the month at ~15:30–16:15, so a fixed cut-off day cannot follow them. Findings: memory `fuga-invoice-2026-11-09`.

## Owner decisions (2026-11-09)
- Both: preview any range, and "save as the official period" of a key.
- Bounds: date + optional hour (default 00:00); hour-aligned only.
- A bound inside a reading gap: warning only (today's partial-bill behaviour), no estimate.

## Rulings

| id | ruling | cost if wrong |
|---|---|---|
| R554 | Period key stays the START month (existing `billing.Period` convention: cut-off 16 → 2026-08 = 08-16→09-16). A saved period's default key = month of its start (Istanbul); the UI may pick the previous/next key. | bills relabelled vs the dashboard |
| R555 | New table `bill_periods(building_id, period_key, period_start, period_end, created_by, timestamps)`, unique (building_id, period_key); start < end; both hour-aligned; span ≤ 62 days. Company-tenant through buildings. | |
| R556 | Resolved window of key K for a building: saved(K) if any; else From = saved(K−1).To if saved, else cut-off From; To = saved(K+1).From if saved, else cut-off To. From ≥ To → `period_conflict`. Neighbours stay contiguous without rewriting them. | double-billed or unbilled hours |
| R557 | Saving K rejects overlap with saved K−1/K+1 (`period_conflict`); after save it force-regenerates K's building and analyzer bills and the live bills of K±1 whose resolved window changed (any status: computed bills are `issued` by default). Company bill: regenerated when the caller has all buildings. Delete = back to the cut-off rule + same regeneration. | stale bills |
| R558 | Consumption explicit windows accept hour-aligned bounds (was midnight-only); boundary rule unchanged (last reading ≤ bound). partialRows slices the window into days clipped to [From, To). | |
| R559 | Preview = computeSubject over an explicit window, nothing persisted (no anomalies recorded, `Open: true`), returns the bill shape + lines + missing days + `boundary_warnings` (bound before the first / after the last reading of a member). Max span 62 days, To ≤ now. | |
| R560 | Invoice shows the energy line even when PTF is missing (`energy` amount 0 + flag text "PTF/YEKDEM verisi yok") and a "birim fiyat" summary: energy unit + distribution unit = all-in unit (Turkish: "Elektrik enerji bedeli", "Dağıtım bedeli", "Toplam birim fiyat"). | manager's "tarife bedeli yok" |
| R561 | Days in period for hour bounds = calendar-date difference (unchanged `DaysInPeriod`). | tier threshold |
| R562 | Tariff flag `reactive_first_violation_waiver` (default true); false = never deduct the first violation even when params waive it (owner: off for Fuga, data starts mid-year). Tariff form checkbox. | Sep 2026 reactive 4864.21 deducted |
| R563 | EPİAŞ unit-cost now returns several `version`s per `period` and RFC3339 periods (live 2026-11-09) → parse RFC3339, keep the latest version per month; today's parser fails the whole run as malformed. | no YEKDEM ever stored |
| R564 | `ekokod recompute prices --from YYYY-MM-DD [--to]` backfills PTF over the days and YEKDEM over the covered months (`SyncPricesPayload.YekdemWindow`); the scheduled run keeps R39's trailing window. | Jul 2026 YEKDEM never fetched |

## Tasks
1. **Domain** `billing.ResolvePeriod(key, cutoff, prev, cur, next *SavedPeriod, loc)` + `ValidateSavedPeriod` (hour-aligned, span, order). Tests: Fuga keys (2026-07 saved 07-17 16:00→08-18 16:00, 2026-08 unsaved → 08-18 16:00→09-16 00:00 cut-off end), conflict, month-end cut-off.
2. **Migration 00028 + store** `BillPeriodRepository` (Get, ListForKeys(building, keys), Upsert, Delete), sqlc, scope isolation test entry (TestScopeIsolation), secret-column guard unaffected.
3. **Consumption** hour-aligned explicit windows (R558) + partialRows clipping; tests with a 15:30→15:30 window over 15-min data and a gap at the start.
4. **Billing service** computeSubject takes a resolved window (R556); `Preview` (R559); `SavePeriod`/`DeletePeriod` (R557). Integration tests: Fuga-shaped data reproduces index difference 37520.34 − 17237.10 for an hourly window; gap at the start → missing days + warning.
5. **API** `POST /bills/preview`, `GET/PUT/DELETE /buildings/{id}/bill-periods/{key}`; OpenAPI + schema.d.ts; authz = bills generate permission; demo company allowed (no external calls).
6. **Web** bills page "Özel dönem" card: scope picker, from/to date + optional time, Önizle → invoice-line-table + warnings; "Bu dönemi kaydet" with key select; bill header shows hours when not midnight. Form-error rule (field marked, toast, dialog closes only on success).
7. **Invoice text** (R560) in PDF + UI; golden PDF sample.
8. Phase end: package gates, internal/arch, e2e of the bills page, handoff.

## PTF check against the MEPAŞ bills (2026-11-09, live EPİAŞ data)
Implied (PTF+YEKDEM): 3285.26 (07-17→08-18), 3450.72 (08-18→09-16). Hourly-arithmetic PTF over the period + latest
YEKDEM: 3235.5 (−1.5 %) and 3307.0 (−4.2 %); consumption-weighted (our R108): −6.5 % / −9.2 %; previous-month and
bill-month monthly averages also miss; neither a constant factor nor a constant adder fits. → Not plain SKTT:
owner asks Uğur Bey about a special contract (manager's instruction).

## Owner / ops (not code)
- EPİAŞ transparency credentials into `/opt/ekokod-new/.env.docker` (now `dev`) → PTF/YEKDEM backfill, then verify the SKTT average method against 3285.26 / 3450.72 TL/MWh (implied by the MEPAŞ bills at KBK 1.0938).
- Fuga tariff: reactive price source `fixed` 3.4937 (now `kbk` → base × 3.49); analyzer installed power (bill: "Sözleşme Gücü 30252").
- GridBox (osos.meramedas.com.tr) unreachable from the server since 2026-11-06 (timeout; reachable from the owner's PC) → ask MEDAŞ about an IP block, then re-fetch 08-07→08-21.
- Reactive first-violation waiver: MEPAŞ charged Sep 2026 (15.49 %); our data starts June → open.
