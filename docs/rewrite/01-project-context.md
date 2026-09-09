# 01 — Project Context

Complete product description and feature inventory. This is the "what must exist" document.
Calculation detail lives in `02-domain-rules.md`; this document describes purpose, screens and
behaviour.

---

## 1. What the product is

BCEM Energy is a **multi-tenant energy management platform for commercial and industrial
electricity consumers in Turkey**. Customers are organisations that operate one or more
buildings/facilities, each metered by one or more electricity meters, and increasingly also
operate rooftop or utility-scale solar plants.

The platform answers five questions for them:

1. **How much energy did we use, when, and where?** Meter data is pulled automatically from
   distribution-company data platforms and converted into hourly/daily/monthly/yearly consumption.
2. **What will our bill be, and is it correct?** The platform reproduces the Turkish electricity
   tariff structure and produces a shadow invoice per meter, per building and per company, which
   the customer compares against the real utility invoice.
3. **How much are we producing, and what is it worth?** Solar production is tracked against
   targets, offset against consumption, and valued using feed-in tariffs.
4. **Is anything wrong right now?** Alarms watch reactive-power ratios, consumption bounds,
   voltage/power limits, data-communication gaps and unexpected invoice jumps.
5. **What is our environmental position?** A carbon footprint module (GHG Protocol + ISO 14064)
   and an ISO 50001 energy-management-system compliance workbench.

The product is sold both as a hosted service and as an **on-premise installation on the
customer's own server**, including fully offline/air-gapped installs. Both delivery modes must
remain supported.

### Product naming in the UI

- The authenticated energy platform is branded **EKORM** and served under the `/ekorm` path prefix.
- The carbon footprint module is branded **Eko-CM**. In the legacy deployment it ran as a
  *separate application* mounted at `/ekocm`, sharing the same database and session secret.
  **In the rewrite it is a first-class module of the single application** — same codebase, same
  API, same auth, no second deployment.
- The public marketing site lives under `/site`.

---

## 2. Users and roles

Six role values exist. Every API call and every screen is filtered by the caller's role.

| Role | Scope | Can write | Notes |
|------|-------|-----------|-------|
| `admin` | Everything, all companies | Yes | Platform operator. Only role that can manage integration definitions, companies and SMTP settings. |
| `company-admin` | Own company: all its buildings, analyzers, plants, users | Yes | The normal customer administrator. |
| `company-readonly-admin` | Own company | **No** | Identical visibility to `company-admin`, all mutations blocked. |
| `building-admin` | Only buildings where they are the assigned responsible user | Yes (within scope) | Cannot see company-level settings, other buildings, user management or power plants. |
| `building-readonly-admin` | Same as `building-admin` | **No** | |
| `demo` | Synthetic dataset only | No | Sees a fixed mock company with two mock analyzers and a mock invoice PDF. Never touches real data. Used for sales demos and the public "request demo" flow. |

### Scope resolution rules

- **`admin`** — unrestricted.
- **`company-admin` / `company-readonly-admin`** — accessible buildings are all buildings whose
  `company_id` equals the user's company. Accessible analyzers are all analyzers belonging to
  those buildings. Accessible plants likewise.
- **`building-admin` / `building-readonly-admin`** — accessible buildings are those whose
  *responsible user* field equals this user's id. Everything else derives from that set.
- **`demo`** — all data-returning endpoints serve the demo dataset regardless of parameters.

Read-only roles must be enforced **server-side**, not merely by hiding buttons.

### Settings tab visibility

| Tab | admin | company-admin | company-readonly | building-admin | building-readonly |
|-----|:-----:|:-------------:|:----------------:|:--------------:|:-----------------:|
| Account | ✅ | ✅ | ✅ | ✅ | ✅ |
| Integrations (integration *definitions*) | ✅ | ❌ | ❌ | ❌ | ❌ |
| Company | ✅ | ✅ (read+edit own) | ✅ (read) | ❌ | ❌ |
| Buildings | ✅ | ✅ | ✅ (read) | ❌ | ❌ |
| Solar Power Plants | ✅ | ✅ | ✅ (read) | ❌ | ❌ |
| Analyzers | ✅ | ✅ | ✅ (read) | ✅ (own) | ✅ (read, own) |
| Users List | ✅ | ✅ | ✅ (read) | ❌ | ❌ |
| SMTP Settings | ✅ | ❌ | ❌ | ❌ | ❌ |

---

## 3. Domain glossary

Turkish domain terms used throughout the UI and in this specification.

| Term | Meaning |
|------|---------|
| **Analizör / Analyzer** | A metering point. In practice a smart electricity meter identified by its *tesisat numarası* (installation number). One building has one or more. |
| **Tesisat numarası** | Installation number — the utility's identifier for a connection point. For GridBox this is the *wiring number*; for ARIL the *subscription identifier*. Unique per integration provider. |
| **Endeks (index)** | The cumulative register reading on a meter (kWh, kVArh). Consumption is always the *difference between two index readings*, never a directly reported value. |
| **Yük profili (load profile)** | Time-stamped index readings at a fixed interval (typically 15 min or 1 h). The primary raw data. |
| **T1 / T2 / T3** | Time-of-use registers. T1 = *gündüz* (day, 06:00–17:00), T2 = *puant* (peak, 17:00–22:00), T3 = *gece* (night, 22:00–06:00). |
| **Aktif enerji** | Real energy, kWh. Direction "T" = consumed from grid, "U" = exported to grid (generation). |
| **Reaktif enerji** | Reactive energy, kVArh. *Endüktif* (inductive, RI) and *kapasitif* (capacitive, RC). |
| **Reaktif ceza** | Reactive penalty. Charged when inductive or capacitive reactive energy exceeds a regulated ratio of active energy. |
| **Kurulu güç** | Installed power (kW). Determines which reactive-penalty threshold band applies. |
| **Anlaşma gücü / Sözleşme gücü** | Contracted power (kW). Basis of the power charge and of the demand-overrun charge. |
| **Demant / Max demand** | The highest measured 15-minute average power in the period (kW). |
| **Dağıtım bedeli** | Distribution charge, TL/kWh. |
| **Güç bedeli** | Power charge = contracted power × unit power price. |
| **Güç aşım bedeli** | Demand-overrun charge, applied when max demand exceeds contracted power. |
| **Tek zamanlı / Çok zamanlı** | Single-rate vs. time-of-use (three-rate) pricing. |
| **Tek terimli / Çift terimli** (monomial/binomial) | Whether the tariff has a separate power charge component. Reactive penalty does not apply to monomial subscribers. |
| **Kademeli tarife** | Tiered pricing: residential and commercial low-voltage single-rate subscribers pay a low rate up to a daily kWh threshold and a higher rate above it. |
| **PTF** | *Piyasa Takas Fiyatı* — the Turkish day-ahead market clearing price, hourly, TL/MWh. Published by EPİAŞ. |
| **YEKDEM** | The renewable energy support-mechanism cost component, monthly, TL/MWh. Published by EPİAŞ. |
| **KBK** | *Kurulca Belirlenen Katsayı* — a per-customer multiplier applied to (PTF + YEKDEM) to derive the actual contracted energy price. Separate coefficients exist for energy, power, overrun and reactive. |
| **İcmal** | A summary billing statement (CSV/Excel) issued by the supplier, listing per-period charges. Used to reverse-engineer a customer's KBK coefficients. |
| **GES** | *Güneş Enerjisi Santrali* — solar power plant. |
| **Çatı GES** | Rooftop solar plant. |
| **Mahsuplaşma / Offsetting** | Netting self-generated energy against consumption. |
| **Feed-in tariff** | Price paid for energy exported to the grid, TL/kWh. |
| **OSOS** | *Otomatik Sayaç Okuma Sistemi* — the automatic meter reading systems operated by Turkish distribution companies. |
| **Fatura kesim günü** | Billing cut-off day of month. Defines the invoice period boundary for a building. |
| **Muhatap no** | The utility's counterparty/account number for a connection. |
| **ETSO** | Market participant code, used to match icmal rows to buildings. |

---

## 4. Core entities

Conceptual model (physical schema is in `04-data-model.md`).

```
Company ──┬── Building ──┬── Analyzer ── MeterReading (time series)
          │              └── Tariff (temporal, versioned by effective date)
          │              └── Bill (per period)
          │              └── CarbonInventory ── CarbonActivity
          │              └── ISO50001 Project (notes, files, dates per clause)
          ├── PowerPlant ──┬── PlantProduction (time series)
          │                ├── PlantDevice (inverters, meters, weather stations)
          │                └── SolarTariff (temporal)
          ├── User
          ├── IntegrationCredential (per provider, encrypted)
          ├── SMTPSettings
          ├── Vacation calendar + Events
          └── Alarm ── AlarmEvent
```

Key relationships and rules:

- A **Company** is the tenant boundary. Everything is scoped to it.
- A **Building** belongs to exactly one company and carries **a history of tariffs**, each with an
  effective-from date. Historical bills must be computed with the tariff in force at the time.
- An **Analyzer** belongs to at most one building (it may be temporarily unassigned) and to exactly
  one integration provider (`subIntegration`, e.g. `Baskent`, `Aydem`, `Meramedas`, `PM5340`).
  `(provider, installation_number)` is unique.
- A **PowerPlant** belongs to a company, optionally linked to an iSolarCloud plant, and carries its
  own tariff history (feed-in and purchase prices).
- **Alarms** are attached to a set of analyzers (or, for iSolar alarms, to a plant).
- **Users** belong to a company and carry a role.

---

## 5. Application map

```
/                          → redirects to /ekorm
/ekorm                     → Dashboard (authenticated)
/ekorm/consumption         → Consumption analysis (electricity)
/ekorm/load-profile        → Load profile analysis
/ekorm/predict             → Consumption forecasting
/ekorm/solar-plants        → Solar plants (iSolarCloud live monitoring)
/ekorm/financial-analysis  → Financial analysis
/ekorm/renewable-energy    → Renewable energy dashboard
/ekorm/bills               → Bills
/ekorm/tariffs             → Tariffs
/ekorm/alarms              → Alarm rules
/ekorm/messages            → System / alarm / job message log
/ekorm/reports             → Monthly, yearly and archived reports
/ekorm/settings/account    → Settings (tabbed)
/ekorm/carbon-footprint    → Carbon footprint module (Eko-CM)
/ekorm/iso-50001           → ISO 50001 compliance workbench
/ekorm/calendar            → Company calendar, events and vacation configuration

/auth/login                → Login
/auth/forgot-password      → Password reset request
/auth/two-steps            → Two-step verification screen
/auth/error                → Auth error
/auth/maintenance          → Maintenance notice

/site/homepage             → Public landing page
/site/about                → About
/site/references           → Customer references
/site/toolkit              → Source/toolkit page
/site/documents            → Documents
/site/pricing              → Pricing
/site/request-demo         → Demo request form
/site/contact              → Contact + map + form
/site/blog                 → Blog listing
/site/blog/detail/[slug]   → Blog article
/site/billCalculate        → Public bill calculator
```

**Disabled / placeholder navigation items** that appear in the sidebar but are not implemented and
must remain visibly disabled: *Consumption → Water*, *Consumption → Gas*, *EV Drivers*,
*Alarm → AI*, *Saving Actions*.

---

## 6. Cross-cutting UI behaviour

These apply to every authenticated screen.

- **Layout** — collapsible left sidebar with grouped navigation, top bar with search, notifications,
  language switcher, dark/light toggle and user menu. Breadcrumb under the top bar on inner pages.
- **Theme customizer** — a settings drawer offering: light/dark, LTR/RTL, theme colour, vertical or
  horizontal layout, boxed or full-width container, sidebar collapse, card style (border or shadow),
  and a border-radius slider. Preferences persist per user.
- **Language** — Turkish (default) and English, switchable at runtime, persisted.
- **Building/Analyzer selectors** — most analysis pages start with a building selector, then an
  analyzer selector filtered to that building. Selection persists while navigating between analysis
  pages within a session.
- **Date range** — start/end date pickers plus a period granularity selector
  (Hourly / Daily / Monthly / Yearly), defaulting to the last 6 months.
- **Loading states** — skeletons, never blank screens. Charts show their own loading indicator.
- **Empty states** — every list, table and chart has an explicit empty state with a hint about what
  to do next.
- **Exports** — analysis tables export to CSV and Excel; reports and bills export to PDF and Excel.
- **Notifications** — inline toasts for success/error on every mutation.

---

## 7. Module inventory

### 7.1 Authentication

**Login** (`/auth/login`)
- Email + password, "remember this device" checkbox, forgot-password link.
- Session lifetime: 1 day normally, 30 days with "remember" checked.
- Sessions are bound to a device fingerprint derived from the user agent; a mismatch invalidates
  the session and forces re-login with a `device_mismatch` reason shown to the user.
- Split layout: form on one side, brand/marketing panel on the other.

**Password policy** — enforced on registration and on change: minimum length, character-class
requirements, and rejection of any password matching the user's recent password history.

**Forgot password** — email-based reset flow.

**Two-step verification** — a code-entry screen exists in the UI. (Legacy did not have a working
backend for it; see `10-removed-behaviours.md`.)

**Registration** — the public self-registration route existed but was deliberately disabled and
redirected to the marketing homepage. Keep it disabled by default, behind a configuration flag.

**Mobile authentication** — a separate token-based flow for a companion mobile app:
`POST /mobile/auth/login` returns a bearer token; `GET /mobile/auth/me` returns the profile.
Mobile clients authenticate every API call with `Authorization: Bearer …`.

---

### 7.2 Dashboard (`/ekorm`)

The landing screen after login. A three-column grid above a full-width chart.

**Building map** (left, largest panel)
- Interactive map with a marker per building, positioned by latitude/longitude.
- Marker style distinguishes **active** (data received within the last 7 days) from **passive**.
- Header shows active and passive counts.
- Marker popup: building name, status, analyzer count, address.
- Selecting a building in the list centres and highlights it on the map.
- An equivalent **Analyzer map** view exists showing per-meter markers with installation number,
  address and status.

**Building list** (centre)
- Searchable, scrollable list of accessible buildings with analyzer counts.
- "Active only" filter, select-all / deselect-all.
- Expanding a building reveals its analyzers; selecting one drives the other dashboard widgets.
- "Centre on map" action per row.

**Latest bill card** (right, top)
- Most recent computed invoice for the selected scope: period, total consumption, total amount.
- Empty state when no invoice exists yet.

**Monthly reactive penalty card** (right, below)
- For the current month across the selected analyzers: whether a reactive penalty was applied,
  the inductive and capacitive ratios, the applicable thresholds, and the installed power band.
- Highlights the analyzer and the building with the highest inductive ratio and the highest
  capacitive ratio.
- Advisory message: within limits, or "power factor correction required".
- Filter for all buildings vs. a single building; link through to the relevant invoice.

**Consumption chart** (full width, bottom)
- Electricity consumption over the selected period.
- Period selector (hourly/daily/monthly/yearly), date range, and toggles for which series to draw
  (active / inductive / capacitive).
- Paginated data table beneath the chart with a per-page indicator and total count; caps at the
  first 50 points for the chart with a notice.
- Year-over-year comparison (this year vs. previous year) where data allows.
- Actions: **Refresh energy values** and **Refresh hourly values** — trigger an on-demand pull from
  the analyzer's integration provider, with progress and success/error feedback.

**Sectoral comparison table** (present in the codebase, currently hidden — must be restored as a
working, visible feature)
- Compares the selected building against the average of all buildings in the same sector on:
  daily consumption, monthly consumption, CO₂ emission, consumption per capita, consumption per
  unit area.
- Shows the building's rank among peers for per-capita, per-area and monthly consumption.
- CSV download of the comparison.

---

### 7.3 Consumption (`/ekorm/consumption`)

The primary analysis screen for electricity. Water and gas are placeholders.

**Filter bar** — building, analyzer, period granularity (hourly/daily/monthly/yearly), start date,
end date, Search button. Plus **Refresh Hourly Values** and **Refresh Energy Values** actions that
re-pull from the integration.

**Summary cards** — active consumption, inductive consumption, capacitive consumption, average,
each for the selected range.

**Tab: Consumption**
- Chart of consumption over the period, with per-series visibility toggles.
- Full data table. Columns: period label, active index, inductive index, capacitive index,
  T1/T2/T3 index, active generation index, inductive/capacitive generation index, U1/U2/U3 index,
  active (kWh), inductive (kVArh), capacitive (kVArh), inductive ratio, capacitive ratio,
  T1/T2/T3 consumption, active generation, inductive generation, capacitive generation,
  U1/U2/U3 generation, max demand.
- Row action: **Alarm check** — runs the AI anomaly check for that row and opens a modal showing
  the actual consumption, the predicted range, and whether the consumption is flagged abnormal.
- Exports: CSV, Excel, print.

**Tab: Detailed graphs**
- Multi-series comparison chart with grouping modes: daily, by week, weekday vs. weekend,
  by season, and by season × week.
- Comparison of current period against previous period.
- Statistics: total, average, peak, valley.

---

### 7.4 Load Profile (`/ekorm/load-profile`)

Analyses the *shape* of consumption across the day rather than totals.

**Filter bar** — building, analyzer, date range. Period granularity defaults to hourly.

**Tab: Daily**
- Average 24-hour load curve, split into **weekday** and **weekend** profiles.
- Explanatory note under each chart.

**Tab: Seasonal**
- Eight profiles: winter/spring/summer/autumn × weekday/weekend, each an average 24-hour curve.
- Each chart has its own explanatory note.

**Tab: Compare**
- User selects any subset of the available profiles (weekday, weekend, and the eight seasonal
  profiles) and overlays them on a single 24-hour chart.

**Tab: Details**
- Per-profile statistics: maximum, minimum, hour of maximum, average, standard deviation, range,
  and **load factor** (average load ÷ peak load).
- Export of the underlying hourly matrix.

---

### 7.5 Predict (`/ekorm/predict`)

Consumption forecasting driven by the ML service.

- Building and analyzer selection, historical date range.
- Chart overlaying **historical actuals** with the **forecast**, drawn as a median line with a
  10th–90th percentile confidence band.
- Forecast horizon selectable; the underlying job produces hourly forecasts.
- **Missing-data reporting**: when the historical series has gaps, the forecast response reports
  each gap (start, end, number of missing hours) and these are surfaced to the user, because they
  degrade forecast quality.
- Forecast status is explicit: success, no data, insufficient data, model error, empty result.

---

### 7.6 AI Analysis (`/ekorm/ai`)

A more technical companion screen to Predict, aimed at operators.

- Analyzer selector and a target date (up to 30 days ahead).
- **Daily prediction** — predicted consumption for the target date with the day of week shown.
- **Weekly prediction** — a week's forecast starting from a chosen week start.
- **Monthly prediction** — a month's forecast for the selected month.
- **Anomaly check** — the user enters or loads an actual consumption value for a timestamp; the
  service returns whether it is anomalous, with the deviation score.
- Each result is shown both as a table and as a chart.

---

### 7.7 Solar Power Plants (`/ekorm/solar-plants`)

Live monitoring of solar plants linked to **iSolarCloud**.

Empty state when no plant is linked: explanatory message and a button through to Settings.

**Plant selector** + **Update Data** action (manual sync), with a connection-status indicator
(connected / connection error).

**Tab: Overview**
- Summary cards: active power, capacity utilisation, yield today, yield this month, yield this
  year, yield total.
- Revenue cards: daily, monthly, yearly and total revenue, computed from the plant's feed-in tariff.
- Charts: production history, and daily production for the current month.

**Tab: Devices**
- Inverter list with device count. Columns: device name, serial number, type, status,
  power/generation, last update.
- Search over device name and serial number.
- Empty state when no devices are returned.

**Tab: History**
- Historical production chart with selectable granularity (day / month / year).
- Excel export of the historical series.

**Tab: Alarms**
- Fault/alarm records pulled from iSolarCloud, translated into Turkish.
- Alarm forwarding: configured e-mail recipients receive new alarms; already-forwarded alarm ids
  are remembered so nothing is sent twice.

---

### 7.8 Renewable Energy (`/ekorm/renewable-energy`)

A broader renewable dashboard, driven by meter *export* (generation) registers rather than the
inverter cloud.

**Filter bar** — building, analyzer, date range.

**Summary cards** — active generation (kWh), inductive generation (kVArh), capacitive generation,
average generation, total active generation.

**Tab: Production data**
- Rooftop solar production chart over the selected period.
- Data table with generation registers.

**Tab: Detailed**
A set of analysis panels:

- **Real-time generation** — current power, today's generation, system efficiency, a 24-hour
  generation chart, maximum and average power, active/inactive status.
- **Energy balance** — total generation, total consumption, grid import, grid export, with filters
  for all / generation / consumption / grid / battery.
- **Financial gains** — today's earnings, this month, this year, total savings, ROI (annual),
  payback period, import tariff, export tariff, bill savings, excess-energy revenue, today's
  import cost, today's export revenue, net gain/cost.
- **Environmental impact** — total contribution plus equivalents: trees planted, coal saved,
  car-usage avoided, home-heating equivalent, each with an explanatory caption.
- **Weather** — current conditions for the plant location (temperature, humidity, wind, pressure,
  visibility, UV index, precipitation chance), a 7-day forecast, and a solar generation potential
  rating derived from the forecast.
- **System status** — overall health (healthy / attention / critical) plus per-component status for
  solar panels, inverter, battery system, grid connection, monitoring system, security system;
  total power generation and average efficiency; manual refresh.
- **Forecast** — estimated generation, estimated consumption and net excess, over hourly / daily /
  weekly horizons; forecast-accuracy metrics (daily, weekly, overall) and a weather-impact readout.
- **Efficiency** — overall efficiency plus solar-panel, inverter, battery and grid efficiency; an
  efficiency trend chart; and prioritised optimisation recommendations (panel cleaning, inverter
  optimisation, battery temperature) each with a priority level and expected benefit.
- **Grid interaction** — current flow direction (import / export / balanced), today's import and
  export, grid quality (voltage, frequency, power factor), tariff information and the resulting
  financial impact.
- **Analytics** — peak generation, average generation, system efficiency, total savings; a trend
  analysis chart; generation-efficiency, system-reliability and energy-savings metrics; and
  generated insights (peak generation hour, efficiency change over 30 days, consumption
  optimisation, maintenance required).

> **Important:** in the legacy system several of these panels were driven by hard-coded or randomly
> generated values rather than real measurements. In the rewrite every panel must be backed by real
> data, or must clearly declare itself unavailable. See `10-removed-behaviours.md`.

---

### 7.9 Financial Analysis (`/ekorm/financial-analysis`)

Company-wide financial view combining consumption cost and solar revenue.

- **Filters** — year selector, month selector (empty = whole year).
- **Headline cards** — total analyzers, total solar plants, electricity consumption, electricity
  production, revenue (monthly or yearly), cost (monthly or yearly).
- **Offset balance card** — net position: excess production or excess consumption for the period.
- **Tariff information panel** — the grid purchase price in force (from the building tariff) and the
  grid sale price (from the solar tariff), with the currency; explicit messages when either tariff
  is missing.
- **Yearly analysis chart** — consumption, production and net cost by month.
- **Monthly detail table** — per month: total consumption, total production, grid purchase,
  grid sale, cost, revenue, net amount; with a yearly total row.

---

### 7.10 Bills (`/ekorm/bills`)

The invoicing screen. Two modes coexist.

**Mode A — invoice dashboard for a month**
- Year selector and month selector.
- **Buildings section**: one row per analyzer within each building — date, building name, analyzer
  name, installation number, consumption (kWh/month), consumption price (₺/kWh), production price
  (₺/kWh), invoice amount (₺/month), and a download action. Per-building total invoice row, and a
  grand total across buildings. "Download all" action.
- **Solar plants section**: one row per plant — plant name, analyzer name, installation number,
  production (kWh/month), consumption price, production price, invoice amount, download action.
  Total production and total invoice rows. "Download all".
- **Netting summary section**: total consumption, total production, net consumption or net
  production, total invoice, invoice period, status (net consumption / net production), efficiency,
  and a combined invoice download.
- Explicit states for: no month selected, loading, and no data for the selected month with a retry.

**Mode B — ad-hoc bill generation**
- Multi-select analyzer picker showing installation number, customer name and province/district;
  building context is fixed when the user is building-scoped.
- Month picker.
- **Download bill** — generates and downloads the per-analyzer PDF invoice.
- **Download building bill** — generates the aggregated building invoice PDF (sum of all its
  analyzers, one page).
- Company-level invoice generation also exists (aggregating all buildings of a company).
- Validation messages for: no analyzer selected, no month selected, no building selected, no tariff
  found for the building, invalid month format, no consumption data for a given analyzer, and lack
  of permission to download.

**Bill listing table** — date, building, analyzer, installation number, ETSO, consumption (kWh),
price (TL/kWh), invoice (TL), with a month total row and per-row download.

**Hourly bill detail export** — for analyzers priced on PTF+YEKDEM, an Excel export listing every
hour of the period with: date, hour, consumption, PTF, YEKDEM, KBK, unit price and cost; plus a
summary block with total consumption, total cost, average PTF, average YEKDEM, hours matched,
hours missing and the period.

**Invoice PDF** — a formal, printable Turkish invoice document containing the customer and
installation identification, the period, the meter index readings at start and end, every charge
line, the tax base, and the total. Generated for analyzer, building and company scope.

---

### 7.11 Tariffs (`/ekorm/tariffs` and `/ekorm/bills-and-tariffs`)

**Tab: Tariffs (building tariffs)**

Create/edit form fields:
- Effective date, currency (TL / USD / EUR), tariff name.
- Energy type: grid energy or green energy.
- Distribution type: LV (`ag`) or MV (`og`).
- Distribution system user group: residential, residential-plus, commercial, commercial-plus,
  industrial, agricultural, lighting, martyrs' families, public lighting.
- Price type: single-time or multi-time.
- Term: monomial or binomial.
- Supply company: incumbent (`attendant`) or private.
- Prices: single-time price, or T1/T2/T3 prices; power charge (TL/month/kW); overuse charge;
  reactive power charge (TL/kVArh); distribution cost (TL/kWh); green energy price and green
  energy distribution cost; contracted power (kW) and power unit price (₺/kW); daily overuse
  threshold (kWh/day, overriding the default); VAT rate; other taxes and funds rate; and an
  arbitrary list of additional named taxes with their rates.
- **Generation usage type**: `none`, `subtract_from_consumption`, or `subtract_from_total`
  (the latter requires a generation price per kWh).
- **PTF+YEKDEM mode**: a toggle that switches the tariff from fixed prices to dynamic
  (PTF + YEKDEM) × KBK pricing, revealing the KBK coefficient fields — energy KBK, power price KBK,
  overuse price KBK, reactive power KBK, T1/T2/T3 KBK, distribution cost (TL/kWh), plus a manual
  YEKDEM override table (year, month, value in TL/MWh) with an enable switch.

Behaviour:
- A building holds a **list** of tariffs, each with an effective-from date. The tariff history is
  displayed and editable. Historical invoices always use the tariff in force at the time.
- **Tariff templates** — named, reusable tariff configurations scoped to a company, optionally
  marked as the default for new buildings. Create, edit, delete, apply.
- **Bulk tariff assignment** — apply a tariff (or a template) to many buildings at once; view the
  current tariff of every building; view bulk-assignment history.
- **İcmal import** — upload a supplier's icmal CSV/Excel, match rows to buildings by ETSO or
  installation number, and automatically derive that customer's KBK coefficients, distribution cost
  per kWh, VAT rate and other-taxes rate from the actual billed amounts. The import screen shows,
  per derived coefficient, the value, the number of periods matched, the stability of the estimate,
  and any warnings (e.g. "power price is unstable across periods — PTF-based calculation may not be
  appropriate"). The user reviews and confirms before the coefficients are written to the tariff.

**Tab: Solar tariffs**
- Plant selector, then a tariff history table for that plant: effective date, feed-in tariff,
  purchase price, currency, notes.
- Add-tariff form with the same fields; validation on required fields and date format `DD-MM-YYYY`.
- Empty state with an "add first tariff" call to action.

**Tab: Default tariffs (admin only)**
- Platform-level default tariff definitions available to all companies as a starting point.

---

### 7.12 Alarms (`/ekorm/alarms`)

Rule management for automated monitoring.

**List** — name, type, applied analyzers, enabled/disabled toggle, actions (edit, delete, view
details, view logs). Filter by active/passive.

**Four alarm types**, each with its own settings form:

1. **Reactive Limit Detection Alarm**
   - Inductive ratio threshold (%) evaluated over a period (value + unit: days or hours).
   - Capacitive ratio threshold (%) over its own period.
   - Active consumption maximum, with its own period.
   - Active consumption minimum, with its own period.
2. **Data Communication Alarm**
   - Communication threshold in hours: fire if no new reading has arrived within this window.
3. **Current – Voltage – Power Alarm**
   - Voltage maximum (V), voltage minimum (V), power maximum (kW), power minimum (kW).
4. **Invoice Alarm**
   - Invoice increase threshold (%): fire when the latest invoice exceeds the previous invoice by
     more than this percentage. Each invoice triggers at most one alarm (fired invoice ids are
     remembered).

**Common settings for every type**
- Analyzer multi-select (which meters the rule applies to).
- Notification frequency (value + days/hours) — suppresses repeat notifications within the window.
- Notification channels: e-mail and/or SMS.
- E-mail recipients (comma-separated) and SMS numbers (comma-separated).
- Enabled/disabled.

**Alarm log** — per rule, a timestamped history of evaluations and firings with message and detail.

---

### 7.13 Messages (`/ekorm/messages`)

A unified operational log for the tenant.

- Search over message content.
- Filter by type: all / alarms / automated transactions (scheduled jobs) / system.
- Filter by status: success / error / warning / information.
- Table of messages with type badge, status badge, message, detail, related entity and timestamp.
- Manual refresh.

---

### 7.14 Reports (`/ekorm/reports`)

**Tab: Monthly report**
- Building multi-select (with select-all) and month/period selection.
- Solar plant selection: all plants / rooftop only / utility-scale only, with a counter and a note
  that rooftop production is always included.
- **Information table** — report period, building(s), electricity tariff, purchase price, average
  purchase price, rooftop feed-in price, utility-scale feed-in price, monthly total consumption,
  daily average consumption, rooftop monthly production, utility-scale monthly production, total
  production, daily average production, electricity bill, reactive penalty.
- **Summary metric cards** — total consumption, total production, total bill, reactive penalty,
  each with a year-over-year delta.
- **Charts** — monthly electricity consumption (kWh/month) and monthly electricity bill (TL/month),
  each comparing the current year against the previous year.
- **Downloads** — PDF, Excel, and **send by e-mail** (recipient address, with a summary of period
  and buildings shown before sending).

**Tab: Yearly report**
- Building multi-select and year selection; utility-scale plant selection with counter.
- **Consumption table** — one row per month: monthly electricity consumption (kWh/month), rooftop
  solar generation (kWh/month), monthly electricity bill (TL/month), reactive penalty (TL/month);
  with a yearly total row plus average daily consumption and average daily rooftop production rows.
- **Solar production report** — total yearly production, targeted production, target achievement
  rate, average daily production.
- **Summary cards** — yearly consumption, yearly production, yearly bill, target achievement.
- **Charts** — yearly electricity consumption/production, yearly electricity bill, and a
  target-vs-actual production comparison per plant.
- **Final comparison section** — yearly consumption vs. yearly production, and the share met by
  solar vs. taken from the grid.
- **Carbon emission section** — emissions from electricity consumption, emission reduction from
  electricity generation, net emission, all in tonnes/year, with the emission factor used and a
  note that it is the Turkey grid average.
- **Downloads** — PDF, Excel, e-mail.

**Tab: Reports archive**
- Filters: report type (all / monthly / yearly) and building (all / specific).
- Total report count.
- Reports grouped by year, then by type, showing the yearly report and the monthly reports for that
  year with a month count.
- Per report: availability status and download actions (PDF, Excel).
- Empty state when no report matches the filters.

---

### 7.15 Settings (`/ekorm/settings/account`)

Tabbed. Tab visibility follows the matrix in §2.

**Account tab** — personal information (name, e-mail, phone) with validation, and change password
(current, new, confirm) enforcing the password policy and history.

**Integrations tab (admin only)** — management of *integration definitions*: the provider
catalogue. Each definition has a type (`OSOS`, `GRIDBOX`, `ARIL`, `PM5340`, `ISOLAR`), a subtype
(the specific distribution company, e.g. `Baskent`, `Aydem`, `Meramedas`), and the set of endpoint
URL templates that provider requires (authentication, analyzer list, hourly values, energy values,
token, last index, indexes, last success date, load profiles, owner consumptions, current indexes,
end-of-month indexes). CRUD with confirmation on delete.

**Company tab** — company details: name, address, total area, personnel count, contact person,
number of analyzers grouped by integration. For `admin`, a company administration view that lists
all companies with create/edit/delete, and per company the ability to **set up an integration**:
choose provider, enter credentials (username/password, or PM5340 URL and installation number, or
iSolarCloud app key/secret/app id/region), choose the target building, and for GridBox enter the
wiring numbers and choose whether to use billing indexes. Credentials are encrypted at rest and
never returned to the client.

**Buildings tab** — building CRUD. Fields: name, address, latitude, longitude, floors, contact
persons (multiple: name + phone), personnel count, total area (m²), responsible user, sector,
billing cut-off day (1–31), and the tariff history. Delete requires confirmation.

**Solar Power Plants tab** — plant CRUD. Fields: plant name, installation number, PV brand/model,
panel power (W), panel efficiency (%), PV count, string count, panel orientation (N/S/E/W/NE/SE/NW/SW),
panel tilt angle (°), monthly target production (twelve values), yearly target production (kWh/year),
total installed capacity (kW), installation date, address, latitude, longitude, inverters (id, brand,
model, power, status, efficiency), and the iSolarCloud link. The **iSolar plant link modal** lists
the plants available on the connected iSolarCloud account and binds one to this plant, importing its
installed power, plant name and device list. Alarm e-mail recipients for the plant are configured here.

**Analyzers tab** — the metering points. List with installation number, customer name, meter number,
meter model, meter multiplier, province/district, tariff type, installed power, connected building
and last data date. Assign or reassign an analyzer to a building. Trigger a manual data refresh.

**Users List tab** — user CRUD within the company: name, e-mail, phone, role. Delete with
confirmation. Role options are limited by the acting user's own role.

**SMTP Settings tab (admin only)** — per-company outbound mail configuration: host, port, secure
flag, username, password, from-address; with a test-send action.

---

### 7.16 Carbon Footprint — Eko-CM (`/ekorm/carbon-footprint`)

A corporate carbon accounting module implementing **GHG Protocol** scopes and **ISO 14064**
categories. It has its own left sidebar and requires a building to be selected before any action.

**Overview**
- Cards: total carbon footprint, total activity count, registered activities, highest emission source.
- Emission distribution bar chart by category.
- Scope analysis radial chart (Scope 1 / 2 / 3 proportions).
- Monthly carbon emissions chart comparing the current year against the previous year.
- Recent activities table: activity, category, date, emission (kgCO₂e), scope; with "see all".

**Activity Selection**
- The organisation declares which emission sources apply to it, from the full catalogue of eight
  main categories and their sub-categories:
  - *Sabit Yanma* (stationary combustion): space heating, process combustion, other combustion
  - *Hareketlilik* (mobility): passenger transport, business travel, employee commuting
  - *Taşımacılık* (logistics): inbound freight, outbound freight
  - *Gaz Salımları*: process emissions, fugitive emissions
  - *Elektrik*: grid electricity, electricity generation
  - *Harici Enerji Kaynakları*: purchased heating/cooling, purchased steam
  - *Ürün ve Hizmetler*: purchased goods, capital goods, end-of-life of sold products
  - *Atık*: waste disposal
- Selected items become the available inputs on the Data Entry screen.

**Data Entry**
- One entry card per selected activity type, each showing its record count.
- Entry modal: transaction period (start and end date), quantity with unit, description/notes,
  and any activity-specific detail fields (e.g. vehicle type → fuel type → sub-type cascades for
  mobility, flight route type and class for air travel, freight direction for logistics).
- The applicable GHG Protocol scope and ISO 14064 category are displayed for the chosen activity,
  derived automatically from the sub-category.
- Emission is computed on save from the emission factor catalogue.
- Empty state directing the user to Activity Selection when nothing has been selected.

**Activity Status**
- All recorded activities with their approval status: *Onay Bekliyor* (pending), *Onaylandı*
  (approved), *Reddedildi* (rejected). Filter, edit, delete, approve/reject.

**Reporting**
- Generate a carbon report for a period. Two report types: **GHG Protocol** and **ISO 14064**.
- Report history per building: name, created date, period, report type, with PDF download.

**Database (emission factors)**
- The emission factor catalogue, seeded per company from the platform's master list and then
  editable by the company.
- Each factor: key, label, main category, sub-categories, category path, base factor, base unit,
  fuel type, bus type, unit conversions (unit, multiplier, label), scope, category, status, and
  metadata (source, year, source URL).
- Units supported include m³, litre, kWh, kg, tonne, passenger-km, tonne-km and others.
- The master catalogue is large (thousands of factors covering fuels, vehicles, flights, freight,
  refrigerants, materials and waste streams) and **must be migrated as seed data**.

**Company Details** — organisational information used in report headers.

**GHG Protocol** and **ISO 14064** — reference pages explaining the standards, the scope/category
mapping used by the module, and how each activity type maps into them.

**Reporting Standards** — comparison and guidance page.

**Documents** — document storage for the carbon module.

**Automated daily carbon accounting** — a scheduled job creates carbon activity records from the
platform's own electricity consumption and solar generation data, so grid electricity and
self-generation appear in the inventory without manual entry.

---

### 7.17 ISO 50001 Module (`/ekorm/iso-50001`)

A guided compliance workbench for the **TS EN ISO 50001:2018** energy management standard.

**Project home / summary page**
- Overall project progress bar with a completion percentage.
- **Gantt chart** of the project timeline: one bar per main clause, with project start and end,
  and per-item status (not started / in progress / completed / expired). Explicit message when
  insufficient date data exists to draw it.
- Per-clause summary of the notes and files recorded, with messages for "no notes added" and
  "no files uploaded".
- **Download ISO 50001 folder** — packages every uploaded file and every note into a single
  downloadable archive, with progress feedback and error handling.
- **Update calendar** action.

**Calendar modal** — set start and end dates for each main clause. Validates that every clause has
both dates and that start precedes end.

**Checklist / accordion**
The standard's clauses 5 through 9, as an accordion:

- **5. Leadership** — 5.1 Leadership and Commitment · 5.2 Energy Policy · 5.3 Organizational Roles,
  Responsibilities and Authorities
- **6. Planning** — 6.1 Actions to Address Risks and Opportunities · 6.2 Objectives, Energy Targets
  and Planning · 6.3 Energy Review · 6.4 Energy Performance Indicators (EnPIs) · 6.5 Energy Baseline
  (EnB) · 6.6 Planning for Collection of Energy Data
- **7. Support** — 7.1 Resources · 7.2 Competence · 7.3 Awareness · 7.4 Communication ·
  7.5 Documented Information
- **8. Operation** — 8.1 Operational Planning and Control · 8.2 Design · 8.3 Procurement
- **9. Performance Evaluation** — 9.1 Monitoring, Measurement, Analysis and Evaluation ·
  9.2 Internal Audit · 9.3 Management Review

Every sub-clause shows its full explanatory text from the standard and provides:
- **Notes** — add, list, edit, update, cancel and delete titled notes with dates.
- **File upload** — attach evidence documents, maximum 30 MB each, with a listing, remove action
  and delete confirmation.
- **Downloadable templates** on selected clauses — spreadsheet and document templates supporting
  that clause (significant energy uses / Pareto analysis, energy consumption analysis / baseline
  and performance formula, CUSUM charts, regression analysis instruction).

Footer note on every screen: all clauses are based on TS EN ISO 50001:2018, and the tool is for
guidance only.

---

### 7.18 Calendar (`/ekorm/calendar`)

- Month / week / day / agenda views with previous, next and today controls.
- Add, update and delete events: title, start, end, all-day flag, colour.
- **Vacation management** — configure which weekdays count as non-working days (default Saturday
  and Sunday), and define special vacation periods (start date, end date, description).
- Vacation data feeds the ML service's day-type features and the load-profile weekday/weekend split.

---

### 7.19 Public marketing site (`/site/*`)

Fully rebuilt with the new design language; same information architecture.

**Homepage** — hero section, feature banner, information strip, features grid, carbon-module
features, references, documents, news, source toolkit, testimonials, leadership, FAQ, pricing
teaser, demo call-to-action, contact, footer. Announcement bar above the header. Responsive
header with a mobile drawer.

**About** — hero, mission, product, partners, vision, process, metrics, leadership.

**References** — customer references with success stories.

**Documents** — downloadable documents listing.

**Toolkit** — source/toolkit feature listing.

**Pricing** — package comparison with a "most popular" marker and per-package feature lists;
contact-us call to action. *(Disabled in the legacy deployment — keep behind a flag.)*

**Request demo** — hero plus a lead-capture form. Submissions are e-mailed to the operator.

**Contact** — address, phone, e-mail, opening hours, an embedded map, and a contact form.
Submissions are e-mailed.

**Blog** — listing with category filters (all / electricity / natural gas), article cards, and
article detail pages rendered from Markdown with images, GitHub-flavoured Markdown and raw HTML
support. Legacy articles ("Elektrik faturam neden yüksek" parts 1 and 2) must be migrated.

**Bill calculator** (`/site/billCalculate`) — a public, unauthenticated estimator:
- Inputs: subscriber group (residential / commercial / industrial / agricultural / lighting),
  subscriber definition (LV / MV), term type (single / double), tariff type, start date, end date,
  total consumption, T1, T2, T3, demand, contracted power.
- Output: energy cost, distribution cost, demand-overrun cost, VAT and total bill.
- Prices come from the **published national tariff schedule**, which must be a maintainable,
  versioned dataset in the database with an effective date — not hard-coded constants.

---

## 8. Background and scheduled work

All of the following ran as OS cron entries invoking HTTP endpoints in the legacy system. In the
rewrite they are **jobs owned by the Go service**, executed by workers, scheduled internally under
a leader lock, individually observable and individually re-runnable.

| Job | Legacy schedule | Purpose |
|-----|-----------------|---------|
| **Refresh analyzers** | daily 03:00 | For every company and every integration: authenticate, list metering points, pull new load-profile and index readings since the last successful reading, and persist them. |
| **Calculate consumptions** | on demand / after refresh | Derive hourly, daily, monthly and yearly consumption from raw index readings. |
| **Generate bills** | daily 05:00 | For every building whose billing cut-off day has passed, compute the invoice for the closed period at analyzer, building and company level; render PDFs; store them. |
| **Alarm check** | hourly | Evaluate every enabled alarm rule; fire notifications subject to the frequency limit; write log entries. Includes fetching iSolarCloud fault alarms and forwarding them. |
| **Generate reports** | monthly / yearly | Produce monthly and yearly reports per building, render PDF and Excel, prepare the e-mail body, store artifacts. |
| **AI predict** | daily | Produce hourly consumption forecasts per analyzer via the forecasting service and store them. |
| **Daily carbon** | daily | Create carbon activity records from electricity consumption and solar generation. |
| **EPİAŞ sync** | daily | Fetch PTF (hourly) and YEKDEM (monthly) prices and store them for dynamic pricing. |

Job requirements in the rewrite:

- **Idempotent** — re-running a job for the same period produces the same result and never
  duplicates rows.
- **Resumable** — a failed integration pull resumes from the last successfully persisted reading.
- **Isolated failure** — one failing analyzer, building or company does not abort the run.
- **Observable** — every run records start, end, scope, counts processed/skipped/failed, and errors,
  visible in the Messages screen.
- **Manually triggerable** — an operator can re-run any job for any scope and period.

---

## 9. Notifications and e-mail

- **Alarm e-mails** — sent to the recipients configured on the alarm rule, using the company's SMTP
  settings; include the analyzer, the rule, the measured values, the thresholds and the timestamp.
- **iSolar alarm forwarding** — plant fault alarms are translated to Turkish and forwarded to the
  plant's configured recipients, deduplicated by alarm id.
- **Report e-mails** — monthly and yearly reports sent as PDF/Excel attachments with a pre-composed
  HTML body summarising the period's key figures.
- **Contact and demo-request e-mails** — public form submissions delivered to the operator.
- **SMS** — an alarm channel exists in the data model and UI. It was never wired to a provider.
  The rewrite must either implement it against a real SMS gateway or hide the option; it must not
  silently accept numbers and do nothing.

---

## 10. Generated file artifacts

| Artifact | Legacy location | Notes |
|----------|-----------------|-------|
| Invoice PDFs | `/opt/bills` | One per analyzer/building/company per period. |
| Report PDFs and Excel files | `/opt/reports` | Monthly and yearly. |
| Uploaded documents (ISO 50001 evidence, carbon module documents) | `/opt/documents` | User-uploaded. |
| ISO 50001 export archives | generated on demand | Zip of all notes and files. |

In the rewrite these live under a single configurable storage root with a deterministic, tenant-
scoped path layout, and every path is validated against traversal. File metadata lives in the
database; the filesystem holds only bytes.

---

## 11. Why this is being rebuilt

Concrete, measured problems with the legacy system. The new architecture is judged on whether it
eliminates them.

1. **Unbounded document growth.** Time-series data is stored as arrays embedded inside single
   documents: raw index readings, hourly values, and derived hourly/daily/monthly/yearly consumption
   rows all accumulate inside one document per meter. Average document size exceeds 5 MB and grows
   without limit. Every read pulls the entire history into memory.
2. **Derived data stored twice.** Consumption is recomputed from raw readings and then persisted as
   its own growing collection, which must be kept in sync and is frequently stale.
3. **Blocking computation.** Consumption derivation and bill generation run inside HTTP handlers.
   The code is littered with manual event-loop yields, which is a symptom, not a solution.
4. **Unreliable scheduling.** Jobs are OS cron entries curl-ing HTTP endpoints with a shared API
   key. There is no run history, no retry, no locking, and no way to tell whether a run succeeded.
5. **Inconsistent integrations.** Six providers, each with its own bespoke date parsing, field
   mapping, multiplier handling and error behaviour, duplicated across setup and refresh paths.
6. **Silent partial failure.** Errors are logged and swallowed at many levels, leaving partially
   written state and no signal to the operator.
7. **No test coverage on the calculations that matter.** Billing rules — the feature customers check
   against real invoices — have no automated verification.
8. **Fabricated UI data.** Several dashboard panels display hard-coded or randomly generated
   figures presented as measurements.
