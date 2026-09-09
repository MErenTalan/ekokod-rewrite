# 10 — Removed Behaviours

> **Read this before starting the rewrite, and review it with the product owner.**

Every item below is a behaviour of the legacy system that is **deliberately not carried over**. Each
was verified in the legacy source, not inferred.

For each: what the old system did, why it is treated as a defect, and what the new system does
instead. **If any of these was intentional** — a deliberate commercial decision, a customer-specific
agreement, or a workaround for something not visible in the code — say so and it will be restored.

Severity: 🔴 changes money · 🟠 changes displayed data · 🟡 correctness or security hygiene

---

## Billing engine

### 1. 🔴 Meter reset produced an absurd consumption figure

**Was:** when the difference between two index readings was negative — meaning the meter had been
reset, rolled over or replaced — the consumption for that period was set to **the end register value
itself**.

```
if (endValue - startValue < 0) return endValue;
```

On a meter whose cumulative register reads, say, 4,800,000 kWh, a reset produced a period
consumption of 4,800,000 kWh and an invoice to match. The code logged a warning and continued.

**Why it is a defect:** the end register is a cumulative total since the meter's installation, not a
period consumption. There is no reading of the tariff under which this is correct.

**Now:** a reset inside a period is handled properly when a reset event is available; otherwise the
period is flagged `suspect`, its consumption is `NULL`, and **billing refuses to issue an invoice**
until an operator resolves it. See `02-domain-rules.md` §3.2.

---

### 2. 🔴 Tiered pricing was applied to multi-time tariffs using an averaged price

**Was:** for time-of-use (`multi_time`) tariffs whose daily average exceeded the tier threshold, the
low tier was priced at the **arithmetic mean of the T1, T2 and T3 prices**:

```
normalPrice = (t1 + t2 + t3) / 3
```

and the high tier at the overuse price — discarding the actual T1/T2/T3 consumption split entirely
for that period.

**Why it is a defect:** the mean of three time-of-use prices is not a price in any tariff schedule.
Its result depends only on the price list, not on when the customer actually consumed, which is the
entire point of a time-of-use tariff. Tiered pricing (*kademeli tarife*) is a single-rate mechanism.

**Now:** tiering applies only to `single_time` tariffs. Multi-time tariffs are always priced
`t1×p1 + t2×p2 + t3×p3`. See `02-domain-rules.md` §6.2.

---

### 3. 🔴 Distribution charge was billed on the low tier only

**Was:**

```
distributionCost = normalConsumption × distributionUnitCost
```

where `normalConsumption` is the **low tier quantity** when tiered pricing applied. All consumption
above the daily threshold was delivered with no distribution charge at all.

**Why it is a defect:** the distribution charge is levied per kWh delivered. There is no tier
structure in it. For any customer above the threshold, this systematically under-billed.

**Now:** `distribution_cost = net_consumption × unit_price`, over the whole quantity. See
`02-domain-rules.md` §6.3.

---

### 4. 🟡 A capacity charge line existed and was always zero

**Was:** `capacityCost` appeared in the invoice model, in the stored bill history, in the PDF and in
the API response. It was assigned `0` unconditionally, in every code path, with a comment noting
that a real calculation "could" be added.

**Why it is a defect:** a permanently zero line item on a customer-facing invoice is noise that
invites the question "why is this zero?" every time someone reads it.

**Now:** removed. If a genuine capacity charge is needed, it is added as a real tariff component
with a real formula.

---

### 5. 🔴 Reactive penalty was applied below 9 kW installed power

**Was:** the threshold table defaulted to inductive 0.33 / capacitive 0.20 and only changed at 9 kW
and 30 kW. Installations **below 9 kW** therefore fell through to the 0.33 / 0.20 defaults and were
penalised.

**Why it is a defect:** small installations are not subject to the reactive penalty regime.

**Now:** installed power below 9 kW is exempt. See `02-domain-rules.md` §6.6.

> ❓ Confirm this with the product owner — it is one of the open questions in
> `02-domain-rules.md` §11.

---

### 6. 🟡 A tax rate field was computed and then ignored

**Was:** the tariff carried both a scalar `other_taxes_rate` (default 3.35 %) and a list of named
taxes (`allTaxes`). The invoice code computed a value from the scalar, wrote it into the response,
and then built the actual total from the named list only. The two never agreed, and the scalar's
value was visible in API responses.

**Now:** only the named tax list exists. See `02-domain-rules.md` §6.7.

---

### 7. 🔴 Two different default VAT rates

**Was:** the tariff schema defaulted `vat_rate` to **18 %**; the invoice code fell back to 18 % when
the field was absent; the KBK derivation defaulted to **20 %**. Türkiye's electricity VAT rate has
been 20 % since July 2023.

**Why it is a defect:** a tariff missing its VAT rate silently produced an invoice understated by
roughly 2 % of the tax base, and two parts of the system disagreed about the same number.

**Now:** `vat_rate` is a required field on every tariff, with no default anywhere. A tariff without
one is rejected at creation and at migration. See `02-domain-rules.md` §6.8.

---

### 8. 🔴 Hours with no market price were billed at zero

**Was:** in hourly PTF+YEKDEM pricing, hours where no PTF value could be matched were counted into
`hoursMissing` and **contributed nothing to the energy cost**. The invoice was issued regardless.
The missing-hour count was returned in the API response but not surfaced in the UI and not gated on.

**Why it is a defect:** the customer consumed energy in those hours. Billing them at zero is silent
under-billing, and the size of the error is invisible.

**Now:** if the missing-hour ratio exceeds a configured tolerance (default 2 %) the invoice is not
issued — it is flagged for operator review. Below tolerance, the shortfall is recorded on the
invoice and shown. See `02-domain-rules.md` §7.1.

---

### 9. 🔴 KBK coefficients were conflated in dynamic price derivation

**Was:** in the period-average PTF path,

```
power_price          = basePrice × (kbk.energyKbk || kbk.powerPriceKbk || 1)
overuse_price        = basePrice × (kbk.overusePriceKbk || 1.5)
reactive_power_price = kbk.reactivePowerKbk || 1
```

Three separate defects: the power price fell back to the **energy** coefficient; the overuse
coefficient defaulted to a magic `1.5`; and the reactive price was returned as the **raw
coefficient**, never multiplied by the base price, so it was not a price at all.

**Now:** each coefficient drives only its own charge; a null coefficient means the charge does not
apply and its line is zero; no magic defaults. See `02-domain-rules.md` §7.2.

---

### 10. 🔴 Market prices fell back to "the nearest available date"

**Was:** when EPİAŞ returned no data for the invoice period, the system searched the database for
the nearest available price record — with no bound on how far away it could be — and used it, with
only a console warning.

**Why it is a defect:** PTF varies by hundreds of TL/MWh month to month. Pricing March with
September's data produces a wrong invoice that looks completely normal.

**Now:** missing price data flags the invoice for review. Prices are never substituted from another
period. See `02-domain-rules.md` §7.3 and `06-integrations.md` §7.

---

### 11. 🔴 The public bill calculator omitted the power charge and ignored T1/T2/T3

**Was:** in `calculateBill`, the power charge was computed (`powerCost = unitPowerCost ×
contractPower`) and then **never added to the total**; the capacity charge was hard-coded to zero;
and the energy cost used the total-consumption input **or** the T1/T2/T3 inputs, never both — so
entering a total silently discarded the time-of-use figures the user had just typed in. The whole
national tariff schedule was hard-coded as literal constants in the source, alongside a commented-out
older schedule.

**Now:** the calculator computes every charge line including the power charge; the input model is
explicit about which pricing mode is in use; and the schedule lives in
`national_tariff_schedule` with effective dates, maintainable without a code change. See
`04-data-model.md` §5.

---

## Data display

### 12. 🟠 Dashboard panels displayed fabricated data

**Was:** several panels on the renewable-energy dashboard and the modern dashboard rendered
hard-coded or randomly generated values presented as measurements — system efficiency, component
health, ROI, payback period, forecast accuracy, grid quality (voltage, frequency, power factor),
"AI insights" with specific percentages, and optimisation recommendations. The dashboard's top card
row shipped with literal placeholder labels (`Text1`, `Text2`, …) and invented figures.

**Why it is a defect:** a monitoring product that displays numbers it did not measure is worse than
one that displays nothing. A customer cannot tell which figures are real.

**Now:** every panel is backed by a real measurement or reports `available: false` with a reason.
Where a figure is derived from incomplete data it carries a `DataQualityBadge`. See
`05-api-contract.md` §9 and `07-design-system.md` §6.

---

### 13. 🟠 The sectoral comparison feature was built and then commented out

**Was:** `BuildingComparisonTable` — sector benchmarking with per-capita, per-area and monthly
rankings and CSV export — was fully implemented, had a working API endpoint and translations, and
was commented out of the dashboard.

**Now:** restored as a visible, working feature. See `01-project-context.md` §7.2.

---

### 14. 🟠 Load profile season labels were wrong

**Was:** in the load-profile page's label map, `summerWeekend` was labelled "Yaz – Hafta içi",
`summerWeekday` was labelled "Yaz – Hafta sonu" (the two swapped), and `autumnWeekday` was labelled
"**Kış** – Hafta sonu". Three of the eight seasonal profiles were mislabelled on the chart legend.

**Now:** labels are generated from the profile key, so a mismatch is structurally impossible.

---

### 15. 🟠 Division-by-zero was masked rather than handled

**Was:** reactive ratios guarded against a zero denominator with `totalActive === 0 ? 1 : totalActive`
— substituting 1 kWh of consumption, which turns any reactive energy in a zero-consumption period
into an enormous ratio and can trigger a spurious penalty. Per-capita and per-area comparison
metrics had no guard at all and could produce `Infinity`, which was then rendered.

**Now:** a zero denominator yields a null ratio, presented as "not applicable", and the record is
excluded from averages and rankings rather than poisoning them. See `02-domain-rules.md`
§3.3 and §10.4.

---

## Integrations

### 16. 🟡 TLS certificate verification was disabled

**Was:** the ARIL and PM5340 clients created their HTTPS agent with
`rejectUnauthorized: false`, globally, for every request, with a comment explaining it was for a
self-signed certificate.

**Why it is a defect:** it disables authentication of the server for all traffic to that integration,
including the credentials sent to it.

**Now:** verification is on. Providers with self-signed certificates have their certificate pinned
in configuration. See `06-integrations.md` §1.

---

### 17. 🟡 Secrets were written to the application log

**Was:** the OSOS and GridBox refresh endpoints logged the full `CRON_API_KEY` — both the incoming
value and the expected value — in plain text on every invocation:

```
console.log(`[OSOS REFRESH] expectedCronApiKey full: "${expectedCronApiKey}"`);
```

Since that key bypasses **all** authentication in the middleware, the logs contained a full
authentication bypass credential.

**Now:** no secret is ever logged. The shared-key mechanism is removed entirely — scheduled work
runs in-process and does not authenticate over HTTP. See `03-target-architecture.md` §4.2.

---

### 18. 🔴 GridBox refresh overwrote history instead of appending

**Was:**

```
a.energyValues.loadProfile = rawProfiles.map(...)
a.energyValues.daily       = res.data.ResultObject.map(...)
```

Each refresh **replaced** the entire stored series with whatever the fetched window returned. A
refresh with a short window discarded everything outside it; a provider returning a partial result
discarded the rest permanently.

**Why it is a defect:** it is unrecoverable data loss on the raw measurements the whole product is
built on.

**Now:** readings are upserted per `(analyzer, timestamp, kind)`. Ingestion only ever adds or
corrects individual readings; it can never remove a range. See `06-integrations.md` §9.

---

### 19. 🟠 GridBox inductive and capacitive registers were crossed

**Was:** one GridBox mapping path assigned `RC` → inductive and `RI` → capacitive, while another
path in the same codebase mapped them correctly. Which one applied depended on which fields the
provider happened to return.

**Why it is a defect:** it swaps the two reactive quantities, which have different penalty
thresholds (0.33/0.20 vs 0.20/0.15), so it can both create and hide a reactive penalty.

**Now:** one mapping, `RI → inductive`, `RC → capacitive`, in one place, with a fixture test. See
`06-integrations.md` §3.

---

### 20. 🟠 Timestamps without an offset were treated as UTC

**Was:** GridBox and several other providers return timestamps with no timezone offset. These were
passed to a date parser that interpreted them as UTC, shifting every reading by three hours in
Türkiye. Meanwhile OSOS timestamps in `DD/MM/YYYY HH:mm:ss` were parsed as local time.

**Why it is a defect:** a three-hour shift moves consumption between T1/T2/T3 bands and across day
and month boundaries — which changes invoices.

**Now:** every adapter declares its provider's timezone semantics explicitly. Offset-less timestamps
are Europe/Istanbul. See `06-integrations.md` §3.

---

### 21. 🟠 ARIL meters silently reported zero time-of-use consumption

**Was:** ARIL load profiles carry no T1/T2/T3 split. The mapper wrote the string `"0"` into all
three registers. Every ARIL-fed metering point therefore had T1 = T2 = T3 = 0 kWh forever, and a
multi-time tariff on such a meter produced an energy charge of zero.

**Now:** unavailable registers are `NULL`, not zero. A multi-time tariff on a metering point whose
source cannot supply the split is rejected at configuration time with an explanatory message. See
`06-integrations.md` §4.

---

### 22. 🔴 PM5340 cumulative generation was reconstructed in memory

**Was:** PM5340 reports generation as an interval value, not a cumulative register. The refresh job
read the last stored cumulative value out of the embedded array, then accumulated forward in memory
across the fetched batch. Any gap, out-of-order batch, duplicate fetch or partial failure corrupted
the cumulative series from that point onward — permanently, and invisibly.

**Now:** interval energy is stored as interval energy; the cumulative register is derived
deterministically by the pipeline from a persisted anchor and is fully recomputable. See
`06-integrations.md` §5.

---

### 23. 🟠 OSOS hourly values were stored in a parallel, unreconciled structure

**Was:** OSOS `hourly_values` returns already-differenced consumption, which was written into a
separate `hourlyValues` map on the analyzer document. It coexisted with the index-derived
consumption and the two were never compared. Different screens read different sources, so the same
meter could show different consumption on different pages.

**Now:** index readings are the single source of truth. Provider-supplied consumption is stored as a
labelled cross-check series and is used for validation and reconciliation, never mixed into the
primary figures. See `06-integrations.md` §2.

---

### 24. 🟡 Billing depended on a live external API

**Was:** invoice generation called the EPİAŞ transparency API synchronously, obtaining a CAS ticket
and fetching prices during the request. If EPİAŞ was slow, rate-limiting or down, invoice generation
failed or fell back to the nearest-date substitution described in item 10.

**Now:** prices are fetched by a scheduled job and stored. Billing reads only local data. See
`06-integrations.md` §7.

---

## Architecture and operations

### 25. 🟡 Scheduled work ran as OS cron entries calling HTTP endpoints

**Was:** `crontab` entries invoking `curl` against `/api/cron/*` with a shared API key that bypassed
all authentication middleware. No run history, no retry, no locking, no failure signal. A second
instance would run every job twice. If the container restarted mid-job, nothing knew.

**Now:** an in-process scheduler under a Postgres advisory lock enqueues typed jobs consumed by
workers, with run history, retries, dead-lettering and per-item error reporting. See
`03-target-architecture.md` §4.

---

### 26. 🟡 CPU-bound work ran inside HTTP request handlers

**Was:** consumption derivation, bill generation and PDF rendering executed inside route handlers,
with manual `setImmediate` yields sprinkled through the loops to stop the event loop from starving.

**Now:** all of it runs in worker processes. The API responds with a job id and the UI tracks
progress. See `03-target-architecture.md` §1.

---

### 27. 🟡 Errors were logged and swallowed

**Was:** a pervasive pattern of `try { … } catch (e) { console.error(e); }` around persistence,
integration calls and cache operations — leaving partially written state with no signal to anyone.

**Now:** every error is handled, returned or recorded as an operational message. Partial success is
modelled explicitly with processed/skipped/failed counts and per-item reasons. See
`03-target-architecture.md` §2.4.

---

### 28. 🟡 The SMS notification channel accepted input and did nothing

**Was:** alarm rules offered "SMS" as a notification type and accepted a list of phone numbers. No
SMS provider was ever integrated. Selecting SMS meant the alarm silently produced no notification.

**Now:** either implemented against a real gateway or hidden from the UI. It is not offered without
being wired up. See `01-project-context.md` §9.

---

### 29. 🟡 The carbon module ran as a second application against the same database

**Was:** the carbon footprint module was deployed as a separate Next.js application at `/ekocm`,
sharing the production database, the session secret and the encryption key, with its own copy of
overlapping code.

**Now:** one application, one deployment, one codebase. See `01-project-context.md` §1.

---

### 30. 🟡 Schema and interface disagreed, and a model name was misspelled

**Was:** several examples — the carbon activity TypeScript interface declared `endDate` optional
while the database schema required it, and declared fields (`mainCategory`, `type`) that the schema
did not have while the schema required fields (`subCategory`) the interface did not declare. The
forecast model was registered under the misspelled name `"AiPreidict"`, which worked only because an
explicit collection name was also passed.

**Now:** types are generated from the schema (`sqlc`), so the two cannot diverge.

---

### 31. 🟡 Consumption merging preserved stale values through a special case

**Was:** merging recomputed consumption over existing rows carried forward the old `maxDemand` when
the new row lacked one — a targeted patch for one field that hid the real problem, which was that
recomputation did not have access to all the source data.

**Now:** derived values are computed from the source, never merged from a previous derivation. There
is nothing to preserve because nothing is stateful. See `04-data-model.md` §4.3.

---

### 32. 🟡 The weather panel hard-coded a city

**Was:** the renewable dashboard's weather widget displayed "Ankara" as its location regardless of
where the plant or building actually was.

**Now:** coordinates come from the plant or building record; when they are missing the panel says
"location not configured". See `06-integrations.md` §8.

---

## Not defects — deliberately preserved

For clarity, these legacy behaviours **are** carried over:

- The `/ekorm` path prefix for the authenticated application and `/site` for the public site.
- Disabled navigation entries (Water, Gas, EV Drivers, AI alarms, Saving Actions) remaining visible
  and visibly disabled.
- The demo role and its synthetic dataset, including the demo invoice PDF.
- Self-registration being disabled by default.
- The theme customiser with all of its options.
- The billing cut-off day model, including day 31 clamping to the last day of a short month.
- The three generation-handling modes.
- Building and company invoices being computed on aggregated consumption rather than as the sum of
  individual invoices.
- Preferring utility `billing` index snapshots over load profile for the monthly time-of-use split.
