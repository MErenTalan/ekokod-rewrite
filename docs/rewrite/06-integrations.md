# 06 — Integrations

Six external systems supply data. In the legacy platform each had its own bespoke handling
duplicated across setup and refresh paths, with divergent date parsing, multiplier logic and error
behaviour. The rewrite puts all of them behind **one interface** with **one ingestion pipeline**.

---

## 1. The common interface

```go
// internal/integration
type MeterDataSource interface {
    Provider() Provider
    Verify(ctx context.Context, creds Credentials) error
    DiscoverMeteringPoints(ctx context.Context, creds Credentials) ([]MeteringPoint, error)
    FetchReadings(ctx context.Context, creds Credentials, req FetchRequest) (FetchResult, error)
}

type FetchRequest struct {
    Point   MeteringPoint
    Kind    ReadingKind      // load_profile | daily | billing | reset | current_index
    From    time.Time        // resumption point from ingestion_cursors
    To      time.Time
}

type FetchResult struct {
    Readings   []domain.MeterReading  // already normalised and multiplied
    NextCursor *time.Time             // nil when the window is exhausted
    Warnings   []string
}
```

Rules that hold for **every** adapter:

1. **Normalise at the boundary.** An adapter returns canonical `domain.MeterReading` values with
   canonical register names, UTC timestamps and the meter multiplier already applied. No provider
   field names, no provider date formats and no provider quirks escape the adapter package.
2. **Never write to the database.** Adapters are pure clients. Persistence is the ingestion job's
   responsibility.
3. **Resumable.** `From` comes from `ingestion_cursors`. Adapters that paginate return a cursor and
   are called until exhausted.
4. **Idempotent upstream of persistence.** Re-fetching an overlapping window is harmless because
   persistence upserts on `(analyzer_id, ts, kind)`.
5. **Explicit errors.** Typed failures — `ErrAuth`, `ErrRateLimited`, `ErrUpstreamUnavailable`,
   `ErrMalformedPayload`, `ErrNotFound` — drive retry policy. Nothing is swallowed.
6. **Bounded.** Per-request timeout, overall job timeout, retry with exponential backoff and jitter,
   dead-letter after the retry budget.
7. **TLS.** Certificate verification is **on by default**. Providers with self-signed certificates
   are handled by pinning their certificate in configuration, not by disabling verification. ⚠️ The
   legacy system disabled verification globally for ARIL and PM5340; see
   `10-removed-behaviours.md` item 16.
8. **Testable.** Each adapter has recorded HTTP fixtures covering success, empty result, partial
   data, auth failure, malformed payload and pagination.

### Endpoint configuration

Provider endpoints are **URL templates stored in `integration_definitions.endpoints`**, so a new
distribution company can be onboarded without a code change. Placeholders are substituted at call
time. Each provider's placeholder set is listed below.

Credentials live in `integration_credentials`, encrypted with AES-256-GCM, and are **never returned
by the API in any form**.

---

## 2. OSOS

Turkish distribution companies' *Otomatik Sayaç Okuma Sistemi*. Multiple subtypes share one
protocol shape: `Baskent`, `Aydem`, `Meramedas`, and others configured per deployment.

### Endpoints

| Key | Method | Placeholders |
|-----|--------|--------------|
| `authentication` | POST | `{username_or_email}`, `{secret_password}` |
| `analyzers_list` | GET | `{secret_token}` |
| `hourly_values` | GET | `{secret_token}`, `{meter_month}` (`YYYY-MM`), `{from_date}`, `{installationNumber}` |
| `energy_values` | GET | `{secret_token}`, `{start_date}`, `{end_date}`, `{installationNumber}` |

### Flow

1. **Authenticate** — POST to the templated auth URL, receive `access_token`.
2. **Discover** — GET `analyzers_list`, receive `instalation_list`. Each entry maps to a metering
   point:

   | Provider field | Canonical field |
   |---|---|
   | `instalationNumber` | `installation_number` |
   | `customerName` | `customer_name` |
   | `customerAdress` | `address` |
   | `il` / `ilce` / `koyMahallesi` / `caddesiSokagi` | `province` / `district` / `neighbourhood` / `street` |
   | `tarifeTipi` / `tarifeTuru` / `tesisatTurTanim` | `tariff_type` / `tariff_kind` / `installation_kind` |
   | `kuruluGucu` | `installed_power_kw` |
   | `koordinatX` / `koordinatY` | `latitude` / `longitude` |
   | `meterNumber` / `meterModel` / `meterMultiplier` | `meter_number` / `meter_model` / `meter_multiplier` |
   | `muhatapNo` / `sayimNokTanim` | `counterparty_no` / `metering_point_name` |

3. **Fetch energy values** — GET `energy_values` for the window. Response carries cumulative index
   readings in the canonical OSOS register names (`t_top_kWh`, `t_ri_kVarh`, `t_rc_kVarh`,
   `t_t1_kWh`, `t_t2_kWh`, `t_t3_kWh`, `t_p_kW`, `u_top_kWh`, `u_ri_kVarh`, `u_rc_kVarh`,
   `u_u1_kWh`, `u_u2_kWh`, `u_u3_kWh`, `u_p_kW`) → mapped to the canonical register set.
4. **Fetch hourly values** — GET `hourly_values`. Response is
   `items[installationNumber][].valueList[]` with `meter_date`, `activeConsumption`,
   `activeGeneration`. These are **already-differenced consumption values, not indexes**, and are
   stored separately as a cross-check series rather than mixed into `meter_readings`. ⚠️ Legacy
   stored them in a parallel structure whose relationship to the index series was never reconciled;
   see `10-removed-behaviours.md` item 23.

### Details

- Date format in and out: `DD/MM/YYYY HH:mm:ss`.
- Numeric values arrive as strings, sometimes with thousands separators — parsed with an explicit
  locale-aware parser, never `parseFloat` on a raw string.
- Default windows: energy values, last 30 days; hourly values, current month from the first of the
  month. In the rewrite the window always comes from the ingestion cursor.
- Provider-level rate limits apply; requests to one provider are serialised per company.

---

## 3. GridBox

A metering data platform. Metering points are identified by **wiring number** (`wiringNo`), stored
in `installation_number`.

### Endpoints

| Key | Method | Placeholders |
|-----|--------|--------------|
| `token` | POST | credentials in body |
| `last_success_date` | GET | `{wiringNo}` |
| `last_endex` | GET | `{wiringNo}` |
| `load_profiles` | GET | `{wiringNo}`, `{startDate}`, `{endDate}` |
| `endexes` | GET | `{wiringNo}`, `{startDate}`, `{endDate}`, `{isBilling}` |
| `energy_values` | GET | `{wiringNo}`, `{startDate}`, `{endDate}` |

Responses are wrapped: `{ ResultStatus, ResultObject, ... }`. `ResultStatus = 1` means success;
anything else is an error and must be surfaced, not treated as an empty result.

### Flow

1. **Token** — POST credentials, receive a bearer token; sent as a header on every subsequent call.
2. Per wiring number:
   - `last_success_date` → the provider's own high-water mark, used to bound the fetch window.
   - `last_endex` → latest index snapshot; **also the source of the meter multiplier**.
   - `load_profiles` → interval readings → `reading_kind = load_profile`.
   - `endexes` with `isBilling=false` → daily index snapshots → `reading_kind = daily`.
   - `endexes` with `isBilling=true` → monthly billing index snapshots → `reading_kind = billing`.
     Fetched only when the company has enabled `use_billing_indexes`.
   - `energy_values` → reset/change events → `reading_kind = reset`.

### Multiplier resolution

GridBox exposes both raw and multiplied values. The multiplier is resolved in this order:

1. The `Multiplier` field on the last index response.
2. Derived from a load-profile row as `ActiveEndexWithMultiplier / ActiveEndex` when both are
   present and the denominator is non-zero.
3. Fall back to `1` **and record a warning** — silently assuming 1 can misprice an entire account.

The resolved multiplier is written to the metering point and stamped on every reading it produced.

### Field mapping

| GridBox field | Canonical register |
|---|---|
| `ActiveEndex` / `ActiveEndexWithMultiplier` | `active_import` |
| `ActiveEndexOut` / `ActiveEndexOutWithMultiplier` | `active_export` |
| `RI` / `ReactiveInductiveEndex` | `reactive_inductive_import` |
| `RC` / `ReactiveCapacitiveEndex` | `reactive_capacitive_import` |
| `RIOut` / `ReactiveInductiveEndexOut` | `reactive_inductive_export` |
| `RCOut` / `ReactiveCapacitiveEndexOut` | `reactive_capacitive_export` |
| `T1` / `T1Endex` / `T1WithMultiplier` | `t1_import` |
| `T2` / `T2Endex` / `T2WithMultiplier` | `t2_import` |
| `T3` / `T3Endex` / `T3WithMultiplier` | `t3_import` |
| `T1Out` / `T2Out` / `T3Out` | `t1_export` / `t2_export` / `t3_export` |
| `MaxDemand` / `MaxDemandWithMultiplier` | `max_demand_kw` |
| `ProfileDateTime` / `ProfileDate` / `EndexDate` / `ReadDate` | `ts` |
| `MeterSerialNumber` | `meter_serial` |

⚠️ The legacy mapper crossed `RI` and `RC` in one code path (mapping `RC` → inductive and `RI` →
capacitive) while mapping them correctly in another. The correct mapping is
`RI → inductive`, `RC → capacitive`. See `10-removed-behaviours.md` item 19.

Dates arrive as ISO 8601, with or without a timezone. A timestamp without an offset is interpreted
as **Europe/Istanbul local time**, never as UTC. ⚠️ See `10-removed-behaviours.md` item 20.

---

## 4. ARIL

A metering platform used for generation facilities and multi-site subscribers. Metering points are
**subscriptions** identified by `SubscriptionSerno`.

### Endpoints

| Key | Method | Body |
|-----|--------|------|
| `authentication` | POST | `{ UserCode, Password }` → token (returned either as a bare string or as `{access_token}`) |
| `analyzers_list` | POST | `{ PageNumber, PageSize }` → `{ ResultList: [...] }` |
| `owner_consumptions` | POST | `{ OwnerSerno, StartDate, EndDate, IncludeLoadProfiles, OwnerType, WithoutMultiplier, MergeResult }` |
| `current_endexes` | POST | `{ OwnerSerno, StartDate, EndDate, DefinitionType, EndexDirection }` |
| `end_of_month_endexes` | POST | period-scoped index snapshots |

Authentication header: `aril-service-token: <token>`.

### Owner types (`DefinitionType`)

| Value | Meaning |
|-------|---------|
| 2 | Subscriber (*abone*) |
| 11 | Street lighting (*aydınlatma*) |
| 15 | Generation facility (*üretim tesisi*) |

The discovered value is stored on the metering point and sent on subsequent consumption calls.

### Subscription mapping

| ARIL field | Canonical field |
|---|---|
| `SubscriptionSerno` | `installation_number` |
| `Title` | `customer_name` |
| `Address` | `address` |
| `MeterSerial` / `MeterBrand` | `meter_number` / `meter_model` |
| `Multiplier` | `meter_multiplier` |
| `InstalledPower` | `installed_power_kw` |
| `AccordPower` | contracted power (seeds the tariff) |
| `Etso` | `etso_code` |
| `LastEndexDate` / `LastProfileDate` | provider high-water marks |
| `DefinitionType` | `definition_type` |

### Load profile mapping

`LoadProfiles[]` entries carry `ProfileDate` as a **14-digit number** `yyyyMMddHHmmss`, parsed as
Europe/Istanbul local time.

| ARIL field | Canonical register |
|---|---|
| `TSum` | `active_import` |
| `ReactiveInductive` | `reactive_inductive_import` |
| `ReactiveCapasitive` (note the provider's spelling) | `reactive_capacitive_import` |
| `TSumOut` | `active_export` |
| `ReactiveInductiveOut` | `reactive_inductive_export` |
| `ReactiveCapasitiveOut` | `reactive_capacitive_export` |

All values are multiplied by the subscription's `Multiplier` at ingestion.

⚠️ ARIL load profiles do **not** carry a T1/T2/T3 split; the legacy code wrote `"0"` into those
registers, which silently produced zero time-of-use consumption for every ARIL meter. In the
rewrite these registers are `NULL`, and a multi-time tariff on an ARIL-fed metering point is
rejected at configuration time with a clear message. See `10-removed-behaviours.md` item 21.

### Max demand

`current_endexes` returns `MaxDemand` and `MaxDemandDate` per record. Max demand for a month is the
maximum across that month's records, stored on the monthly reading.

---

## 5. PM5340

A Schneider PM5340 power meter exposed through a small local HTTP service, typically on the
customer's own network. Configured per company with a base URL and an installation number.

### Endpoint

```
GET {base_url}/api/v1/readings?limit=500&sort=asc[&cursor=…][&start=…][&end=…]
→ { items: [...], limit, cursorNext, hasMore, total, sort, filters }
```

Cursor pagination; the adapter follows `cursorNext` while `hasMore` is true.

### Reading mapping

| PM5340 field | Canonical register |
|---|---|
| `meterDate` (ISO 8601 or `DD/MM/YYYY HH:mm[:ss]`) | `ts` |
| `activeImport_kWh` | `active_import` |
| `inductive_kvarh` | `reactive_inductive_import` |
| `capacitive_kvarh` | `reactive_capacitive_import` |
| `dmdKwPeak_kW` | `max_demand_kw` |
| `currentGeneration` | see below |
| `deviceId` / `deviceIp` | diagnostic metadata |

### Generation accumulation ⚠️

`currentGeneration` is an **interval** value (kW over a 15-minute window), not a cumulative
register. Every other register in the system is cumulative.

**Correct handling:** convert the interval power to interval energy
(`kW × interval_hours = kWh`) and store it in a dedicated `interval_generation_kwh` column, from
which the cumulative `active_export` register is derived by the ingestion pipeline as a running
sum anchored to the last known cumulative value.

The anchoring point must be persisted, not recomputed from the beginning, and it must be
**recomputable deterministically** if history is corrected.

> ⚠️ The legacy code maintained the running total in memory during a refresh by reading the last
> stored value out of the embedded array. Any gap, reordering or partial failure silently corrupted
> the cumulative series. See `10-removed-behaviours.md` item 22.

Values may be `null`; nulls are stored as `NULL`, never coerced to 0.

---

## 6. iSolarCloud (Sungrow)

Solar plant monitoring. Unlike the meter integrations, this feeds `plant_production`,
`power_plant_devices` and plant alarms.

### Regions

| Region | Gateway | Authorisation origin | Cloud id |
|--------|---------|----------------------|----------|
| `EU` | `https://gateway.isolarcloud.eu` | `https://web3.isolarcloud.eu` | 3 |
| `CN` | `https://gateway.isolarcloud.com.cn` | `https://www.isolarcloud.com` | 1 |
| `AU` | `https://augateway.isolarcloud.com` | `https://au.isolarcloud.com` | 2 |

### Credentials

`app_key`, `secret_key`, `app_id`, `region`, plus an OAuth `access_token` / `refresh_token` pair
with an expiry. All encrypted at rest.

### Authorisation flow

1. Build the authorisation URL from the region's authorisation origin, the `app_id` and the
   redirect URI.
2. The user authorises in iSolarCloud and is returned to `/integrations/isolar/callback` with a code.
3. Exchange the code at `/openapi/apiManage/token` for tokens.
4. Refresh at `/openapi/apiManage/refreshToken` **before** expiry. Token refresh is serialised per
   company with a lock so concurrent jobs cannot both refresh and invalidate each other's token.

Every platform call carries the app key, the signed secret header and the access token. Responses
are wrapped with a result code that must be checked; a non-success code is an error.

### Calls used

| Purpose | Operation |
|---------|-----------|
| List plants | `queryPowerStationList` |
| Plant detail | `getPowerStationDetail` |
| Devices for a plant | `getDeviceListByPsId` |
| Device real-time data | `getDeviceRealTimeData` |
| Plant real-time data | `getPowerStationRealTimeData` |
| Device day/month/year series | `getDevicePointDayMonthYearDataList` |
| Plant day/month/year series | `getPowerStationPointDayMonthYearDataList` |
| Plant minute series | `getPowerStationPointMinuteDataList` |
| Device minute series | `getDevicePointMinuteDataList` |
| Fault alarms | `getFaultAlarmInfo` |

### Measurement points

Values are addressed by numeric point ids. The catalogue used:

**Inverter — energy:** 1 Yield Today (Wh) · 87 Yield This Month (Wh) · 88 Yield This Year (Wh) ·
2 Total Yield (Wh)
**Inverter — power:** 24 Total Active Power (W) · 14 Total DC Power (W) · 43 Total Apparent Power
(VA) · 26 Power Factor
**Inverter — grid:** 18/19/20 Phase A/B/C Voltage (V) · 21/22/23 Phase A/B/C Current (A) ·
27 Grid Frequency (Hz)
**Inverter — MPPT:** 5/6 MPPT1 Voltage/Current · 7/8 MPPT2 Voltage/Current
**Weather station:** 2001 Daily Horizontal Irradiation (Wh/m²) · 2002 Total Horizontal Irradiation ·
2009 Ambient Temperature (°C) · 2010 Module Temperature (°C)
**Meter:** 8030 Forward Active Energy (Wh) and the remaining meter points as configured

Units are normalised at the adapter boundary: **Wh → kWh, W → kW**. Nothing downstream deals in
watt-hours.

### Alarm forwarding

Fault alarms are fetched on a schedule, translated to Turkish through a maintained message
dictionary, and forwarded to the plant's configured recipients. Sent alarm references are recorded
in `isolar_forwarded_alarms` so nothing is sent twice. ⚠️ The legacy implementation kept the sent
ids in an unbounded array on the plant document; here it is a table with a retention policy.

---

## 7. EPİAŞ (market prices)

The Turkish energy exchange transparency platform. Read-only; supplies PTF and YEKDEM.

### Authentication

A TGT ticket obtained from the EPİAŞ CAS endpoint with a username and password. The ticket is
cached until expiry and refreshed under a lock.

### Data

| Data | Granularity | Unit | Endpoint area |
|------|-------------|------|---------------|
| PTF (day-ahead market clearing price, MCP) | hourly | TL/MWh | electricity service, day-ahead market |
| YEKDEM unit cost | monthly | TL/MWh | electricity service, renewable support mechanism |

### Rules

- A scheduled job fetches and **persists** prices into `market_prices_hourly` and `yekdem_monthly`.
- **Billing never calls EPİAŞ.** It reads from local storage. ⚠️ The legacy bill path called the
  EPİAŞ API synchronously during invoice generation, which coupled invoice correctness to an
  external service's availability. See `10-removed-behaviours.md` item 24.
- Missing data is missing. If a period lacks prices, the invoice is flagged for review, not
  approximated from "the nearest available date". ⚠️ See `10-removed-behaviours.md` item 10.
- Backfill is supported for an explicit historical range, for onboarding and for migration.

---

## 8. Weather

Used by the renewable dashboard and as a forecasting covariate.

- One configurable provider, accessed through an adapter with the same shape as the others.
- Responses cached with a TTL appropriate to the data (current conditions ~15 min, forecast ~3 h).
- Coordinates come from the plant or building record. When coordinates are missing, the panel
  reports "location not configured" rather than defaulting to a city. ⚠️ The legacy UI hard-coded
  Ankara. See `10-removed-behaviours.md` item 32.
- In an air-gapped installation weather is unavailable; the panels must degrade cleanly and say so.

---

## 9. Ingestion pipeline

One pipeline serves every provider.

```
scheduler
  → integration.sync_analyzers (per company × credential)
      → Verify → DiscoverMeteringPoints → upsert analyzers
      → fan out: integration.fetch_readings (per analyzer × reading kind)
            → read ingestion_cursors
            → adapter.FetchReadings(from, to)   [loop while a cursor remains]
            → validate + normalise
            → COPY into staging → upsert into meter_readings
            → advance ingestion_cursors
            → enqueue consumption.refresh for the affected range
```

### Validation before persistence

A reading is rejected — recorded as a warning, not stored — when:

- the timestamp is unparseable, in the future beyond a small tolerance, or before the metering
  point's commissioning date;
- every register is null;
- a register value is negative;
- a register jumps by more than a configurable sanity multiple of the point's typical interval
  consumption (candidate corruption; flagged for review rather than silently accepted).

Rejections are counted per run and surfaced in the Messages screen.

### Backfill

`POST /integration-credentials/{id}/backfill` enqueues a historical pull over an explicit date
range, chunked into windows sized for the provider, rate-limited, and resumable. This is how a new
customer's history is loaded and how the migration re-fetches anything the legacy database is
missing.

### Health

Per metering point: last successful reading time, consecutive failure count and last error. A point
that has not produced data within its expected interval raises the data-communication alarm and
appears as *passive* on the map.
