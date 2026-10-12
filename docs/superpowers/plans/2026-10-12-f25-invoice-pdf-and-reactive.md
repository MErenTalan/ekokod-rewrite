# F25 — Invoice PDF redesign + reactive rules per EPDK 10961 (2026-10-12)

Branch `fix/f25-invoice-pdf` (worktree `/home/personal/ekokod-f25-invoice`, from `main` @b5ece58).
The owner found the invoice PDF ugly and asked whether reactive penalties are complete. Checking the
engine against EPDK Kurul Kararı 10961 (RG 30.04.2022/31825, amending "Tarife Uygulamalarına İlişkin
Usul ve Esaslar" Madde 13) found five differences; the owner chose to fix them in this branch.

## Rulings

| # | Ruling | Cost if wrong | Where |
|---|---|---|---|
| R526 | Migration 00026 adds a `billing_parameters` row effective **2022-05-01** (the decision's own effective date was not in the text found) copying the latest earlier row, with: `reactive_exempt_terms = {}` (13/1 names no tariff term — monomial subscribers pay), exempt groups `{residential, residential_plus, martyrs_families, lighting, public_lighting}` (mesken + aydınlatma), `reactive_exempt_below_kw = 15` inclusive (R527), bands `[15,50) 0.33/0.20`, `[50,∞) 0.20/0.15` (13/2-3 say 50 kVA; installed power is stored in kW and is used as is), the three new flags on. Basis stays `whole_quantity` (13/4 confirms). Generation exemption unchanged (13/5 only drops reactive a generator *supplies*; we have no register to split it). Single-phase exemption is not modelled: single-phase connections are below 15 kW, which the size exemption covers. 35-day cap not modelled: periods are ≤ 31 days. | Monomial / 9–15 kW / 30–50 kW subscribers billed differently than the distributor | `migrations/00026_reactive_epdk_2022.sql` |
| R527 | New column `reactive_exempt_inclusive` (default false = legacy "< 9 kW"): true makes the size exemption "≤ value" (13/1 "15 kW'a (dâhil) kadar"). | A 15.000 kW installation penalised | `reactive.Evaluate`, params store/admin |
| R528 | New column `reactive_charge_higher_side`: when both sides exceed their limits, only the side with the larger measured register is billed (13/6); a tie bills inductive. | Double reactive charge | `reactive.Evaluate` |
| R529 | New column `reactive_first_violation_waived`: the first reactive violation of a calendar year (the year of `PeriodKey`) is printed and deducted on the same invoice (13/1). `Input.ReactivePriorViolation` = a live bill of the same scope+subject earlier in that year has `reactive_penalty_applied`; analyzer bills look at the analyzer's history, building bills and each building inside a company bill at the building's. The deduction is line `reactive_waiver` = −(reactive lines), inside the VAT base; `ReactivePenalty` is the net (0), `ReactivePenaltyApplied` stays true so later months see the violation. Known limits: recomputing an earlier month does not re-evaluate later months; a year's first bill computed here counts as first even if the distributor saw an earlier one; analyzer bills are on demand only (the monthly job makes building and company bills), so an analyzer's history is only as complete as the analyzer bills someone generated. | Penalty charged on a waivable first violation, or waived twice | `billing.Compute`, `service/billing`, `BillRepository.ReactiveViolationBefore` |
| R530 | Invoice PDF redesign: emerald (#047857) header with the amount due, subscriber and tariff cards (tariff name/group/voltage/term/contracted power, installed power — `Document` gains `Tariff`, `InstalledPowerKw`; analyzer bills list their own meter), KPI tiles with reactive ratio bars, full-width zebra line table ending VAT base → VAT → credit → total (VAT base used to print under VAT), reactive status chip, PTF box, index table with a difference column, footer on every page (computed date, bill no, page x/y, "not an official invoice"). | Cosmetic | `render/invoicepdf`, `service/billing/jobs.go` |

## Tasks

1. **Reactive rules (R526–R529).** Tests first: `reactive` (inclusive exemption at 15.000, higher-side
   pick, tie → inductive, flags off = legacy), `billing.Compute` (waiver line −penalty, VAT base net,
   prior violation → no waiver, param off → no waiver), property-test oracle extended, store
   integration (`ReactiveViolationBefore` true/false/other-year/superseded; params round-trip the new
   columns; 00026 row values), service integration (second violating month in a year is charged).
2. **PDF (R530).** Pure helpers tested (`ReactiveStatus`, line split, month label); golden hash,
   all scopes × locales, byte-stable; PNG review of building/analyzer/company/flagged/waived samples.

Gates: touched packages whole with `-tags=integration` (`domain/reactive`, `domain/billing`,
`domain/tariff`, `store/postgres` (+`/admin`), `service/billing`, `render/invoicepdf`), `internal/arch`,
golangci-lint on touched packages.

## Status (2026-10-12)

Both tasks done on `fix/f25-invoice-pdf` (311741d reactive, 0d68ef7 PDF, then a company-chip fix).
Gates green: `domain/reactive`, `domain/billing` (property oracle extended; two engine mutations
caught), `domain/tariff`, `store/postgres` whole incl. `TestScopeIsolationCoversEveryRepository`,
`store/postgres/admin`, `migrate/legacy`, `service/billing` (prior-violation mutation caught),
`worker`, `render/...`, `internal/arch`, golangci-lint. Visual review: `go test ./internal/render/invoicepdf
-run TestWriteSamples -samples=DIR` writes building, waived analyzer, English company and flagged samples.

Not done / for the owner:
- Not merged, not deployed. Deploying runs migration 00026; existing bills are not recomputed —
  a period priced under the new rules needs a forced regeneration.
- Before deploying, check new.ekokod.com has no `billing_parameters` row dated after 2022-05-01 (it would
  shadow R526's row): `select effective_from from billing_parameters order by 1;` (the read was not
  permitted from this session).
- Confirm against a real invoice: 50 kVA band applied to installed kW, the 2022-05-01 date, and whether
  a later EPDK decision changed Madde 13.
