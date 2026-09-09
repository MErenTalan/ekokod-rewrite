# 02 — Domain Rules

The calculation contract. These rules produce numbers that customers compare against real utility
invoices, so they are specified exactly.

**Implementation requirement:** every rule in this document is implemented as a **pure function in
a dependency-free `domain` package** — no database, no HTTP, no clock, no filesystem. Inputs in,
values out. Each is locked with golden-file tests before any storage or transport code is written.

Where the legacy system did something wrong, this document states the **correct** rule and marks
the difference with ⚠️. The full list of those differences, with rationale, is in
`10-removed-behaviours.md`.

Where a rule needs confirmation from the product owner before implementation, it is marked ❓ and
listed in §11.

---

## 1. Units, precision and time

| Concern | Rule |
|---------|------|
| Active energy | kWh |
| Reactive energy | kVArh |
| Power / demand | kW |
| Market prices | TL/MWh (PTF, YEKDEM) |
| Tariff prices | TL/kWh, except power charge which is TL/kW/month |
| Money | Decimal, never binary floating point. Store as `numeric(18,6)`. Round to 2 decimals only at presentation and at the invoice total. |
| Energy quantities | `numeric(18,6)`, presented at 3 decimals |
| Unit prices | `numeric(18,6)`, presented at 6 decimals |
| Ratios | Stored as fractions (0.33), presented as percentages (33 %) |
| Timestamps | Stored in UTC (`timestamptz`). All business logic — day boundaries, month boundaries, billing periods, time-of-use windows — evaluated in **Europe/Istanbul**. |
| DST | Türkiye has used permanent UTC+3 since 2016. Historical data before that crosses DST transitions; the conversion must use the tz database, not a fixed offset. |

**Rounding rule:** intermediate values are never rounded. Rounding happens once, at the point a
value is written to an invoice line or displayed. Half-up.

---

## 2. Meter readings

### 2.1 What a reading is

A reading is a timestamped snapshot of a meter's **cumulative registers**. Consumption is always
derived as a *difference between two readings* — never read directly.

Register set (canonical names used throughout the system):

| Register | Meaning |
|----------|---------|
| `active_import` | Cumulative active energy drawn from the grid (kWh) |
| `reactive_inductive_import` | Cumulative inductive reactive energy drawn (kVArh) |
| `reactive_capacitive_import` | Cumulative capacitive reactive energy drawn (kVArh) |
| `t1_import`, `t2_import`, `t3_import` | Cumulative active energy per time-of-use band (kWh) |
| `active_export` | Cumulative active energy delivered to the grid — i.e. generation (kWh) |
| `reactive_inductive_export` | Cumulative inductive reactive energy delivered (kVArh) |
| `reactive_capacitive_export` | Cumulative capacitive reactive energy delivered (kVArh) |
| `t1_export`, `t2_export`, `t3_export` | Cumulative export per time-of-use band (kWh) |
| `max_demand` | Maximum demand for the interval (kW) — **not cumulative** |

> Legacy field names, for reference when reading migration dumps: `t_top_kWh`, `t_ri_kVarh`,
> `t_rc_kVarh`, `t_t1_kWh`, `t_t2_kWh`, `t_t3_kWh`, `t_p_kW` for import; `u_top_kWh`, `u_ri_kVarh`,
> `u_rc_kVarh`, `u_u1_kWh`, `u_u2_kWh`, `u_u3_kWh`, `u_p_kW` for export. "T" = *tüketim*,
> "U" = *üretim*.

### 2.2 Meter multiplier

Some meters report scaled register values that must be multiplied by a **meter multiplier**
(`çarpan`) to obtain real energy. The multiplier is a property of the metering point.

**Rule:** the multiplier is applied **once, at ingestion**, and the value stored in
`meter_readings` is always the real, multiplied value. Downstream code never multiplies again.
The multiplier used for each reading is recorded on the reading so historical corrections are
auditable.

### 2.3 Reading classes

Providers deliver readings in up to four classes. All land in the same table, distinguished by a
`reading_kind` column:

| Kind | Meaning |
|------|---------|
| `load_profile` | Interval readings, 15 min or 1 h. **The primary source for all consumption derivation.** |
| `daily` | End-of-day index snapshots. |
| `billing` | End-of-billing-period index snapshots published by the utility. Where available these are more authoritative than load profile for the time-of-use split. |
| `reset` | Meter reset / replacement events. |

### 2.4 Time-of-use bands

| Band | Window (Europe/Istanbul) |
|------|--------------------------|
| T1 — *gündüz* (day) | 06:00 – 17:00 |
| T2 — *puant* (peak) | 17:00 – 22:00 |
| T3 — *gece* (night) | 22:00 – 06:00 |

Meters report T1/T2/T3 as separate registers; the system uses the meter's own bands and does not
re-derive them from timestamps. The windows above are documented for validation and for the public
bill calculator.

---

## 3. Consumption derivation

### 3.1 The core operation

For a period `[start, end)`:

```
reading_start = the last reading with ts <= start
reading_end   = the last reading with ts <= end
consumption_R = reading_end.R - reading_start.R      for each register R
```

If either reading is missing, or if `reading_start` and `reading_end` are the same reading, the
period yields **no row** — it is not emitted as zero.

### 3.2 Meter reset and replacement ⚠️

A negative difference means the register decreased: the meter was reset, rolled over, or replaced.

**Correct rule:**

1. If a `reset` reading exists inside the period, consumption is
   `(register_value_before_reset − reading_start) + (reading_end − register_value_after_reset)`.
2. Otherwise the period is marked **`suspect`**: consumption for the affected registers is recorded
   as `NULL`, the period is flagged, an operator message is written, and **the period is excluded
   from billing** until an operator resolves it (by registering the reset, entering the correct
   values, or accepting an override).

A suspect period must never silently produce a number that flows into an invoice.

> ⚠️ The legacy system, on a negative difference, used the **end register value itself** as the
> consumption. On a meter with a large cumulative register this produces an invoice of absurd
> magnitude. See `10-removed-behaviours.md` item 1.

### 3.3 Derived ratios

```
inductive_ratio  = reactive_inductive_import  / active_import       (0 when active_import = 0)
capacitive_ratio = reactive_capacitive_import / active_import       (0 when active_import = 0)
```

### 3.4 Aggregation levels

Consumption is materialised at four levels. Each is derived by the same difference operation over
its own period boundary, **not by summing the level below** — this keeps every level consistent
with the underlying registers even when readings are missing.

| Level | Boundary (Europe/Istanbul) | Label format |
|-------|---------------------------|--------------|
| Hourly | top of hour → next top of hour | `DD/MM/YYYY HH:00 - HH:00` |
| Daily | 00:00 → next 00:00 | `DD/MM/YYYY` |
| Monthly | 1st 00:00 → next month 1st 00:00 | `MM/YYYY` |
| Yearly | 1 Jan 00:00 → next 1 Jan 00:00 | `YYYY` |

The label formats above are the presentation format. Internally every row is keyed by a
`timestamptz` bucket start.

**Monthly from billing readings:** when a metering point has `billing`-kind readings covering a
month, the monthly row is derived from those instead of from load profile, because utility billing
snapshots carry the authoritative T1/T2/T3 split. The row records which source it came from.

### 3.5 Max demand

`max_demand` for a period is the **maximum** of the interval `max_demand` values within the period,
not a difference and not a sum. It is used for the demand-overrun charge.

For providers that expose a dedicated current-index endpoint (ARIL), max demand is taken from that
endpoint's `MaxDemand` field, taking the maximum per month.

### 3.6 Activity status

A metering point is **active** if its most recent reading is within the last 7 days; otherwise
**passive**. This drives the map markers and the data-communication alarm.

---

## 4. Tariff resolution

A building holds a list of tariffs, each with an `effective_from` date.

**Rule:** for a target date `D`, the applicable tariff is the one with the **greatest
`effective_from` that is ≤ D**. If several share that date, the most recently created one wins.
If no tariff satisfies the condition, there is **no applicable tariff** and billing for that period
fails with an explicit, surfaced error — it does not fall back to a default.

⚠️ Accepted date formats in legacy data are inconsistent (`dd-MM-yyyy`, `yyyy-MM-dd`, `YYYY-MM`,
`dd-MMM-yyyy` with Turkish or English month names, ISO 8601). The migration normalises all of them
to a `date` column. The new system accepts and stores only a proper date.

Solar plant tariffs resolve identically against the plant's own tariff history.

---

## 5. Invoice period

Each building has a **billing cut-off day** (`bill_cutoff_day`, 1–31).

For a target month `YYYY-MM`:

```
start = YYYY-MM-min(cutoff_day, days_in_that_month)      at 00:00
end   = next month, min(cutoff_day, days_in_next_month)  at 00:00
```

Examples with cut-off day 1: `2025-12` → 01-12-2025 to 01-01-2026.
With cut-off day 15: `2025-12` → 15-12-2025 to 15-01-2026.
With cut-off day 31 and a 30-day month: clamps to the last day of each month.

`days_in_period = end − start`, in days.

---

## 6. Bill computation

Inputs: the consumption row for the period, the applicable tariff, the metering point's installed
power, and the period start/end.

### 6.1 Net consumption and generation handling

The tariff declares a **generation usage type**:

| Type | Effect |
|------|--------|
| `none` | Generation is ignored. `net_consumption = active_import` |
| `subtract_from_consumption` | Generation is netted off energy. `net_consumption = max(0, active_import − active_export)` |
| `subtract_from_total` | Energy is billed in full, and a monetary credit is deducted from the invoice total. `net_consumption = active_import`, `generation_credit = active_export × generation_price_per_kwh` |

### 6.2 Energy charge

**Tiered pricing (*kademeli tarife*)** applies **only to single-rate (`single_time`) tariffs** for
the residential and commercial distribution-user groups. ⚠️

```
daily_average = net_consumption / days_in_period

threshold = tariff.overuse_threshold_kwh_per_day
            ?? 8   for residential user groups
            ?? 30  for commercial user groups

tiered = (user_group is residential or commercial)
         AND price_type = single_time
         AND daily_average > threshold
```

When `tiered` is true:

```
generation_offset   = active_export  if generation_usage_type = subtract_from_consumption, else 0
low_tier_limit      = max(0, days_in_period × threshold − generation_offset)
low_tier_kwh        = min(net_consumption, low_tier_limit)
high_tier_kwh       = max(0, net_consumption − low_tier_kwh)

energy_cost = low_tier_kwh × tariff.single_time_price
            + high_tier_kwh × tariff.overuse_price
```

When `tiered` is false:

```
price_type = single_time:
    energy_cost = net_consumption × tariff.single_time_price

price_type = multi_time:
    energy_cost = t1_kwh × tariff.t1_price
                + t2_kwh × tariff.t2_price
                + t3_kwh × tariff.t3_price
```

The **effective unit price** reported on the invoice is `energy_cost / net_consumption`
(0 when `net_consumption` is 0).

> ⚠️ The legacy system applied tiering to multi-time tariffs too, pricing the low tier at the
> arithmetic mean of the T1, T2 and T3 prices. That mean has no basis in the tariff schedule and
> is not reproduced. See `10-removed-behaviours.md` item 2.

### 6.3 Distribution charge ⚠️

```
distribution_cost = net_consumption × tariff.distribution_cost
```

The distribution charge applies to **all** consumed energy, both tiers.

> ⚠️ The legacy system multiplied the distribution unit price by the *low tier quantity only*,
> silently omitting the distribution charge on all above-threshold consumption. See
> `10-removed-behaviours.md` item 3.

### 6.4 Green energy charge

Applies only when the tariff carries green-energy prices:

```
green_energy_cost = net_consumption × (tariff.green_energy_price
                                     + tariff.green_energy_distribution_cost)
```

Otherwise zero.

### 6.5 Power charge and demand overrun

```
power_cost = tariff.contracted_power × tariff.power_unit_price

demand_overrun_cost = max_demand > contracted_power
                      ? (max_demand − contracted_power) × (tariff.power_unit_price × 2)
                      : 0
```

If `max_demand` is unknown (0 or null), the overrun charge is 0 and the invoice records that
demand data was unavailable for the period.

The power charge applies to binomial (`çift terimli`) tariffs. For monomial tariffs
`contracted_power` and `power_unit_price` are expected to be null and both figures are 0.

> ⚠️ A `capacity_cost` line existed in the legacy model and was hard-coded to 0 everywhere. It is
> removed. See `10-removed-behaviours.md` item 4.

### 6.6 Reactive penalty

```
inductive_ratio  = reactive_inductive_import  / max(net_consumption, ε)
capacitive_ratio = reactive_capacitive_import / max(net_consumption, ε)
```

Thresholds by installed power (`kurulu güç`, kW):

| Installed power | Inductive limit | Capacitive limit |
|-----------------|-----------------|------------------|
| < 9 kW | **exempt — no reactive penalty** ⚠️ | exempt |
| 9 kW ≤ P < 30 kW | 0.33 | 0.20 |
| P ≥ 30 kW | 0.20 | 0.15 |

The penalty is **not applied** when any of the following holds:

- the tariff term is **monomial** (`tek terimli`);
- the distribution user group is **residential** or **lighting**;
- there was **generation in the period** (`active_export > 1` kWh);
- installed power is below 9 kW.

When it does apply:

```
penalty = 0
if inductive_ratio  > inductive_limit:  penalty += reactive_inductive_import  × reactive_power_price
if capacitive_ratio > capacitive_limit: penalty += reactive_capacitive_import × reactive_power_price
```

❓ **Verify before implementing:** the formula above charges the *entire* reactive quantity once the
ratio is exceeded, which is what the legacy system did. Confirm against the current EPDK tariff
regulation whether only the *excess above the limit* should be charged. See §11.

> ⚠️ Legacy applied the 0.33 / 0.20 thresholds to installations below 9 kW rather than exempting
> them. See `10-removed-behaviours.md` item 5.

### 6.7 Additional taxes and funds

A tariff carries a list of named additional taxes, each with a rate (%). Examples in Turkish
electricity invoices: *Belediye Tüketim Vergisi* (BTV), *Enerji Fonu*, *TRT payı*.

```
for each tax in tariff.additional_taxes:
    tax.amount = energy_cost × tax.rate / 100

other_taxes_cost = Σ tax.amount
```

Each tax appears as its own named line on the invoice with its rate and amount.

> ⚠️ The legacy model also had a scalar `other_taxes_rate` field (defaulting to 3.35 %) that was
> computed but never used in the total once the named-tax list was introduced. The scalar field is
> removed; only the named list exists. See `10-removed-behaviours.md` item 6.

### 6.8 VAT

```
vat_base = energy_cost
         + distribution_cost
         + green_energy_cost
         + power_cost
         + demand_overrun_cost
         + reactive_penalty
         + other_taxes_cost

vat_cost = vat_base × tariff.vat_rate / 100
```

⚠️ `vat_rate` is a **required** field on every tariff. There is no default. The legacy code used
18 % in one place and 20 % in another; Türkiye's electricity VAT rate has been 20 % since July 2023
and has changed before, which is precisely why it must be a dated tariff property rather than a
constant. See `10-removed-behaviours.md` item 7.

### 6.9 Total

```
total = max(0, energy_cost
              + distribution_cost
              + green_energy_cost
              + power_cost
              + demand_overrun_cost
              + reactive_penalty
              + other_taxes_cost
              + vat_cost
              − generation_credit)
```

### 6.10 Invoice output

A computed invoice records, at minimum:

- Period: `month_key`, `start_date`, `end_date`, `days_in_period`
- Register readings at start and end for every register
- Quantities: `active_import`, `t1/t2/t3`, `reactive_inductive`, `reactive_capacitive`,
  `active_export`, `net_consumption`, `low_tier_kwh`, `high_tier_kwh`, `max_demand`
- Unit prices actually used, including the effective energy price
- Every charge line individually
- `inductive_ratio`, `capacitive_ratio`, the thresholds applied, and whether the penalty was applied
- `generation_usage_type`, `generation_credit`, `generation_price_per_kwh`
- Whether tiered pricing was applied
- Which tariff version was used (id + effective date)
- Whether PTF+YEKDEM pricing was used, and if so the hours matched/missing
- The list of analyzer ids included (for building and company invoices)

### 6.11 Building and company invoices

- **Building invoice** — consumption of all analyzers assigned to the building is summed register
  by register for the period, then the building's tariff is applied **once** to the aggregate.
  It is not the sum of the individual analyzer invoices.
- **Company invoice** — the same aggregation one level up, applying each building's own tariff and
  summing the resulting charges.

---

## 7. PTF + YEKDEM dynamic pricing

When a tariff has `use_ptf_yekdem = true`, the energy price is derived from market data instead of
being fixed.

### 7.1 Preferred method — hourly

```
for each hour h in the invoice period:
    if hourly consumption for h exists AND PTF for h exists:
        yekdem_h   = manual override for (year, month of h), if enabled, else published YEKDEM for that month
        unit_price = (ptf_h + yekdem_h) / 1000 × kbk.energy_kbk        # TL/MWh → TL/kWh
        cost      += consumption_h × unit_price
        hours_matched++
    else:
        hours_missing++
```

The result reports `hours_matched`, `hours_missing` and the average PTF. ⚠️ If
`hours_missing / total_hours` exceeds a configured tolerance (default 2 %), the invoice is **not
issued**; it is flagged for operator review. Legacy silently under-billed the missing hours.
See `10-removed-behaviours.md` item 8.

### 7.2 Fallback — period average

Used when hourly consumption is unavailable for the metering point:

```
ptf_avg     = mean of hourly PTF across the invoice period
yekdem      = manual override for the period's month if enabled, else published YEKDEM
base_price  = (ptf_avg + yekdem) / 1000                    # TL/kWh
```

Derived prices:

| Price | Formula |
|-------|---------|
| Energy unit price | `base_price × kbk.energy_kbk` |
| T1 / T2 / T3 unit prices | `base_price × kbk.t1_kbk` / `t2_kbk` / `t3_kbk` |
| Power unit price | `base_price × kbk.power_price_kbk` |
| Overuse unit price | `base_price × kbk.overuse_price_kbk` |
| Reactive power price | `base_price × kbk.reactive_power_kbk` |
| Distribution cost | `kbk.distribution_cost_tl_per_kwh` — a flat TL/kWh figure, **not** derived from PTF |

Every KBK coefficient is independent. A null coefficient means that charge does not apply and its
line is 0 — it does not fall back to another coefficient or to 1.

> ⚠️ Legacy computed the power price as `base_price × (energy_kbk || power_price_kbk || 1)`,
> conflating three distinct coefficients; defaulted the overuse coefficient to 1.5; and returned the
> reactive price as the raw coefficient without multiplying by the base price. All three are wrong.
> See `10-removed-behaviours.md` item 9.

### 7.3 EPİAŞ data

- **PTF (MCP)** — hourly day-ahead market clearing price, TL/MWh, from the EPİAŞ transparency
  platform. Authentication is a TGT ticket obtained from the EPİAŞ CAS endpoint with a username and
  password.
- **YEKDEM** — monthly unit cost, TL/MWh.
- Both are fetched by a scheduled job and **stored locally**. Bill computation reads from local
  storage and never calls EPİAŞ synchronously. If data for a period is missing, the invoice is
  flagged, not silently approximated. ⚠️ Legacy fell back to "the nearest available data point",
  which could be months away. See `10-removed-behaviours.md` item 10.

---

## 8. Deriving KBK coefficients from an icmal

An **icmal** is a supplier-issued billing summary. Uploading it lets the system reverse-engineer a
customer's KBK coefficients from what they were actually charged.

### 8.1 Input columns

Column headers vary between suppliers; the parser matches known aliases case- and
diacritic-insensitively. Numbers use Turkish formatting (`1.234,56`).

| Field | Turkish header aliases |
|-------|------------------------|
| Accounting period (`YYYYMM`) | Muhasebe Dönemi / Muhasebe Donemi / Dönem |
| ETSO code | ETSO Kodu / ETSO |
| Total kWh | Toplam Kwh / Toplam kWh |
| T0 / T1 / T2 / T3 kWh | T0 Kwh … T3 Kwh |
| Total energy charge | Enerji Bedeli Toplam / Enerji Bedeli |
| Total distribution charge | Toplam Dağıtım Bedeli / Dağıtım Bedeli |
| Reactive charge | Reaktif Bedel / Reaktif Bedeli |
| Power charge | Güç Bedeli |
| Demand-overrun charge | Güç Aşım Bedeli |
| Inductive / capacitive kVArh | Reaktif Induktif Kwh / Reaktif Kapasitif Kwh |
| Demand | Demant / Talep / Demand |
| VAT base | KDV Matrahı / Matrah |
| VAT | KDV |
| BTV / Energy fund / TRT | Btv Bedeli / Enerji Fonu / Trt Bedeli |
| Price difference | Fiyat Farkı |
| Correction amount | Düzeltme Tutarı |
| Cancelled invoice flag | Fatura İptal Mi? |
| Term | Terim (Tek Terimli / Çift Terimli) |
| Voltage level | AG/OG |
| Time type | TekZaman/Üç Zaman |
| First / last reading date | İlk Okuma Tarihi / Son Okuma Tarihi |
| Billing cycle name | Fatura Döngü Adı |
| Accrual multiplier | Tahakkuk Çarpanı |

Rows flagged as cancelled invoices are excluded.

### 8.2 Per-row derivation

For each row, `base_price = (PTF_month_avg + YEKDEM_month) / 1000` TL/kWh:

```
energy_kbk (unadjusted) = energy_charge / (total_kwh × base_price)

energy_kbk (adjusted)   = (energy_charge − price_difference − correction_amount)
                          / (total_kwh × base_price)
```

The adjusted variant is used when the row carries price differences or corrections. Both variants'
back-calculation errors are computed and reported.

```
reactive_power_kbk = reactive_charge / ((inductive_kvarh + capacitive_kvarh) × base_price)
                     — only when the row has a reactive charge

power_price_kbk    = power_charge / (demand × base_price)
                     — only when the row has a power charge and demand > 0;
                       otherwise a warning is emitted

distribution_cost_tl_per_kwh = total_distribution_charge / total_kwh

vat_rate         = vat / vat_base × 100
other_taxes_rate = (btv + energy_fund + trt) / vat_base × 100
```

The overuse coefficient cannot be derived from an icmal, because the overrun quantity is not
present in the file. It is reported as *requires manual entry*.

### 8.3 Aggregation across periods

Multiple rows for the same building are combined using the **median** (not the mean) to suppress
outliers. Reported alongside each aggregate:

- Standard deviation of the samples.
- A **stability flag** for the power coefficient: stable when `stddev / median < 0.1`. When
  unstable, a warning states that PTF-based power pricing may not be appropriate for this customer.
- Row count and number of periods matched.
- Deduplicated warnings from the individual rows.

Aggregates are rounded to 4 decimal places; VAT and other-tax rates to 2.

### 8.4 Validation

Each derived coefficient is validated by back-calculating the energy charge and comparing to the
actual. **Target: under 2 % error.** Results above that are surfaced to the operator, who confirms
or rejects before the coefficients are written to the tariff. Nothing is written automatically.

---

## 9. Carbon accounting

### 9.1 Emission from an activity

```
emission_kgco2e = quantity × unit_conversion_multiplier × emission_factor.base_factor
```

Every activity record stores the factor key, the factor value used and the conversion multiplier,
so historical figures remain reproducible when the factor catalogue is updated.

### 9.2 Scope and category mapping

The GHG Protocol scope and ISO 14064 category are derived from the activity's sub-category, not
entered by the user:

| Sub-category | GHG scope | ISO 14064 category |
|---|---|---|
| Space heating, process combustion, other combustion | Scope 1 | Category 1 |
| Process emissions, fugitive emissions | Scope 1 | Category 1 |
| Passenger transport (own fleet) | Scope 1 | Category 1 |
| Grid electricity | Scope 2 | Category 2 |
| Purchased heating/cooling, purchased steam | Scope 2 | Category 2 |
| Business travel, employee commuting | Scope 3 | Category 3 |
| Inbound freight, outbound freight | Scope 3 | Category 3 |
| Purchased goods, capital goods | Scope 3 | Category 4 |
| End-of-life of sold products | Scope 3 | Category 5 |
| Waste disposal | Scope 3 | Category 4 |
| Electricity generation (own, exported) | Scope 1 | Category 1 |

The authoritative mapping is the one embedded in the legacy `GHG_ISO_MAPPING` table; it is migrated
verbatim as seed data and the table above documents its shape.

### 9.3 Grid electricity emission factor

`0.45 kg CO₂e / kWh` — Türkiye grid average, used in reports and in the automated daily carbon job.

⚠️ This must be a **dated, editable configuration value**, not a constant. The national grid factor
changes annually. Reports state the factor used and its source year.

### 9.4 Report figures

```
consumption_emission_t     = total_consumption_kwh × grid_factor / 1000
production_reduction_t     = total_generation_kwh × grid_factor / 1000
net_emission_t             = consumption_emission_t − production_reduction_t
```

---

## 10. Report metrics

### 10.1 Monthly report

Per building (or set of buildings) and month:

| Figure | Definition |
|--------|------------|
| Total consumption | Sum of `active_import` for the month across selected buildings |
| T1 / T2 / T3 | Same, per band |
| Previous-year consumption | Same month, previous year |
| Daily average consumption | Total consumption ÷ days in month |
| Rooftop production | Sum of rooftop plant generation for the month |
| Utility-scale production | Sum of grid-connected plant generation for the month |
| Total production | Rooftop + utility-scale, filtered by the plant selection |
| Daily average production | Total production ÷ days in month |
| Electricity bill | Sum of the building invoices' totals for the month |
| Bill components | Energy, distribution, taxes |
| Previous-year bill | Same month, previous year |
| Reactive penalty | Sum of reactive penalty across the invoices |
| Inductive / capacitive ratio | Weighted by active consumption |
| Purchase price | Effective energy unit price from the tariff |
| Feed-in price | From the plant's solar tariff |

### 10.2 Yearly report

Per building and year:

- Twelve monthly rows: consumption, rooftop generation, bill, reactive penalty; plus a yearly total
  row and average-daily rows.
- Year-over-year comparison for consumption and bill.
- Solar: total yearly production, target production (from the plant's yearly target), **target
  achievement rate** = actual ÷ target × 100, average daily production; and a per-plant breakdown.
- Consumption vs. production: total consumption, total production, **share met by solar** =
  min(production, consumption) ÷ consumption × 100, and the complementary grid share.
- Carbon section as in §9.4.

### 10.3 Load profile statistics

For each profile (weekday, weekend, and the eight season × day-type combinations), over the
selected date range, for each hour 0–23: the mean consumption across all matching days. Then:

```
max, min, hour_of_max, mean, stddev, range = max − min
load_factor = mean / max          (0 when max = 0)
```

Weekday/weekend classification uses the **company's vacation configuration**: the configured
non-working weekdays plus any date falling inside a defined vacation period counts as "weekend".

Seasons (Northern hemisphere, meteorological):
winter = Dec–Feb, spring = Mar–May, summer = Jun–Aug, autumn = Sep–Nov.

### 10.4 Sectoral comparison

For a building, against all buildings sharing its `sector`:

```
daily_consumption      = mean daily active_import over the trailing 30 days
monthly_consumption    = active_import over the trailing calendar month
co2_emission           = monthly_consumption × grid_factor
consumption_per_capita = monthly_consumption / personnel_count
consumption_per_area   = monthly_consumption / total_area
```

Each metric is reported with the building's value, the sector average, and the building's rank
within the sector. Buildings with a null or zero `personnel_count` / `total_area` are excluded from
the corresponding per-capita / per-area average and rank rather than producing an infinite value. ⚠️

---

## 11. Open questions for the product owner

These must be answered before the corresponding code is written. Each is marked ❓ above.

1. **Reactive penalty base (§6.6).** Charge the entire reactive quantity once the ratio limit is
   exceeded (legacy behaviour), or only the excess above the limit? This materially changes invoice
   amounts.
2. **Reactive exemption below 9 kW (§6.6).** Confirm that installations under 9 kW installed power
   are exempt, as the rewrite assumes.
3. **Tiered pricing user groups (§6.2).** Legacy applied tiering only to `residental` and
   `commercial`. Confirm whether `residentialPlus` and `commercialPlus` should also be tiered, and
   at what thresholds.
4. **Missing-hour tolerance (§7.1).** Confirm 2 % as the threshold above which a PTF-priced invoice
   is withheld for review.
5. **Grid emission factor (§9.3).** Confirm the current official value and its source, and whether
   historical reports should be recomputed with period-appropriate factors or keep 0.45.
