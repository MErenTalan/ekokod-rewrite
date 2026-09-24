# F12a — Public Site Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans`. Inline, one session, no subagents.

**Goal:** The server side of the public site (09 §F12, 01 §7.19, 05 §17):
- the national tariff schedule seeded as data, from the legacy calculator's constants;
- the public bill calculator, with every charge line **including the power charge** and a documented rule for total vs T1/T2/T3;
- contact and demo-request endpoints with validation, rate limiting, spam protection and e-mail delivery.

The site pages, blog and SEO are **F12b**.

**Architecture:**
- **Calculator:** `internal/domain/tariff/public.go` is pure (`PublicBill`). It picks the schedule row in force on the period's start date for the (group, voltage, term), switching to the `*_plus` row above the daily threshold (legacy's low/high consumption prices). `POST /public/bill-calculator` loads the rows via `NationalTariffRepository.Effective` under a platform read.
- **Forms:** `internal/service/publicforms` validates the forms and mails them through the SMTP settings of the operator company named in config. `EKOKOD_PUBLIC_FORMS_COMPANY_ID` + `EKOKOD_PUBLIC_FORMS_TO`; when unset, the answer is 503 `forms_not_configured` (on-prem friendly).
- **Limits:** a new route limit mode, `PublicFormLimit`, gives 5 requests per 10 minutes per IP. The calculator uses the global API limit.

**Spec:** 09 §F12; 01 §7.19; 05 §6 (`GET /national-tariff-schedule`), §17; 02 §2.4 (bands), §7 (bill components); legacy `src/utils/calculateBill.tsx`, `src/app/site/billCalculate/page.tsx`, `src/app/api/{contact,request-demo}`.

## Open questions (defaults shipped)

| # | Question | Default | Cost if wrong |
|---|---|---|---|
| Q-H1 | The schedule's source and effective date. | **The active constant block of legacy `calculateBill.tsx`** (the commented-out block is an older period, not shipped). `effective_from = 2025-01-01`, source `"legacy public calculator (EPDK table, date not recorded)"`. Admins can add dated rows through `POST /national-tariff-schedule` | Prices shown for dates before the real effective date |
| Q-H2 | Legacy had two energy prices under one tariff (low/high daily use). | **Mapped onto the schema's `residential`/`residential_plus` and `commercial`/`commercial_plus` groups.** The base row carries `daily_threshold_kwh` (8 and 30 kWh/day); above it, the `*_plus` row prices the bill | — |
| Q-H3 | Is the power charge per month or per period? | **`power_price × contracted_kW × days / 30`**: a monthly price pro-rated by the period's days. The overuse charge = `(demand − contract) × overuse_price` when demand exceeds the contract | The pro-rating convention |
| Q-H4 | Total and T1/T2/T3 both entered. | **Documented rule.**<br>• Multi-time: T1/T2/T3 price the energy.<br>• Single-time: the total prices it.<br>• If both are given they must agree (sum within 0.01 kWh), else 422 `total_consumption: mismatch`.<br>• A single-time entry with only T values uses their sum; a multi-time entry needs all three | — |
| Q-H5 | Taxes. | **VAT on (energy + distribution + power + overuse)** at the row's `vat_rate`; no BTV, matching §7.19's output list | A real bill also has BTV |
| Q-H6 | Where do form submissions go? | E-mail to `EKOKOD_PUBLIC_FORMS_TO` through the SMTP settings of `EKOKOD_PUBLIC_FORMS_COMPANY_ID`; nothing is stored | A mail failure loses the lead (the visitor sees an error and the contact details) |

## Rulings (R350–R356)

| Id | Rule |
|---|---|
| R350 | **Seed:** 17 rows (`internal/seed/data/national_tariff_schedule.json`); `seed` prints `national tariff schedule: 17 rows`. |
| R351 | **Calculator input:** group ∈ {residential, commercial, industrial, agricultural, lighting}, voltage ∈ {lv, mv}, term ∈ {monomial, binomial} (binomial only on mv), `multi_time` bool, `start ≤ end` ≤ 366 days, consumption values ≥ 0 and < 10⁹, demand/contract ≥ 0.<br>No row in force → 422 `start: no_tariff`. Lighting has no T prices → multi-time is refused (`multi_time: not_available`). |
| R352 | **Output:** `{energy, distribution, power, overuse, vat_base, vat, total, vat_rate, days, band_rule, tariff: {effective_from, group_used, source}}`, each money value rounded to 2 dp (half-up) after an unrounded sum. `group_used` names the `*_plus` switch. |
| R353 | **Forms:** contact `{name ≤ 120, email, phone? ≤ 40, subject? ≤ 200, message 10–5000, website (honeypot, must be empty), elapsed_ms ≥ 2000}`; demo `{name, email, phone, company ≤ 200, role? ≤ 120, message? ≤ 2000, website, elapsed_ms}`. A honeypot or too-fast submission answers **202 with no mail** (the bot sees success). A valid one answers 202 after the mail is sent; SMTP failure → 502 `delivery_failed`. |
| R354 | **Mail:** plain text, Turkish; the subject `Web sitesi iletişim formu — <name>` or `Demo talebi — <company>`; `Reply-To` the visitor. The header-injection guard is the mail package's own (CR/LF refused). |
| R355 | **Limit:** `PublicFormLimit` 5 per 10 minutes per client IP, via `TrustedProxies` as the global limiter. 429 with Retry-After. |
| R356 | **Access:** the three public routes are `Access: Public`; no tenant data is read except the operator company's SMTP settings (SystemScope of that company). |

## Tasks
1. Seed the schedule (R350) + parser test + CLI expectation.
2. `PublicBill` pure calculator (R351, R352, Q-H2…Q-H5), including `TestPublicCalculator` with a hand-computed example with the power charge.
3. The calculator route + DTOs + OpenAPI.
4. Public forms service + routes + the limit mode + config (R353–R356).
5. Self-review, handoff.
