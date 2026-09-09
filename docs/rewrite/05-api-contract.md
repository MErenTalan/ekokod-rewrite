# 05 — API Contract

The Go service exposes one versioned REST API at `/api/v1`. It publishes an OpenAPI 3.1 document at
`/api/v1/openapi.json`; the frontend's TypeScript client is generated from it in CI and never
hand-written.

---

## 1. Conventions

### Format

- JSON in, JSON out. `Content-Type: application/json; charset=utf-8`.
- Field names `snake_case`.
- Timestamps ISO 8601 with offset: `2025-03-14T09:00:00+03:00`.
- Dates `YYYY-MM-DD`. Periods `YYYY-MM` (month) and `YYYY` (year).
- Money and energy as **strings** carrying exact decimals (`"1234.567890"`) — never JSON numbers,
  which lose precision.
- Enums are the lowercase snake_case values defined in `04-data-model.md`.

### Authentication

| Client | Mechanism |
|--------|-----------|
| Browser | httpOnly `Secure` `SameSite=Strict` cookies: short-lived access token + rotating refresh token. |
| Mobile | `Authorization: Bearer <access token>`, refreshed via `/auth/refresh`. |
| Internal services | `X-Service-Key` on the internal network only. |

### Errors

```json
{
  "error": {
    "code": "tariff_not_found",
    "message": "Seçilen dönem için geçerli bir tarife bulunamadı.",
    "details": { "building_id": "…", "period": "2025-03" },
    "request_id": "01HQ…"
  }
}
```

| Status | Use |
|--------|-----|
| 400 | Malformed request |
| 401 | Missing or invalid credentials |
| 403 | Authenticated but not permitted |
| 404 | Not found, or found but outside the caller's scope (never leak existence) |
| 409 | Conflict — duplicate, or state does not allow the operation |
| 422 | Validation failed; `details` carries per-field messages |
| 429 | Rate limited; `Retry-After` set |
| 500 | Unexpected; `request_id` correlates with the logs |

`message` is localised via `Accept-Language` (`tr` default, `en` supported). `code` is stable and
machine-readable.

### Pagination

Cursor-based on every collection:

```
GET /api/v1/…?limit=50&cursor=eyJ0cyI6…
→ { "items": [...], "next_cursor": "…", "total": 1234 }
```

`limit` defaults to 50, maximum 500. `total` is omitted when counting is expensive.

### Filtering and sorting

`?building_id=…&from=2025-01-01&to=2025-03-31&sort=-created_at`. Unknown parameters are rejected
with 400 rather than ignored.

### Idempotency

Every mutating endpoint accepts `Idempotency-Key`. Replaying a key within 24 hours returns the
original response.

### Authorisation

Every endpoint declares a required permission. The service resolves the caller's accessible company
and building set once per request. Read-only roles are rejected on every mutating endpoint with 403.
There is a contract test per endpoint per role.

In the tables below: **A** = admin, **CA** = company_admin, **CR** = company_readonly_admin,
**BA** = building_admin, **BR** = building_readonly_admin, **D** = demo.

---

## 2. Authentication

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| POST | `/auth/login` | public | Email + password (+ `remember_me`). Sets cookies or returns tokens. |
| POST | `/auth/refresh` | authenticated | Rotates the refresh token, issues a new access token. |
| POST | `/auth/logout` | authenticated | Revokes the current session. |
| POST | `/auth/logout-all` | authenticated | Revokes every session for the user. |
| GET | `/auth/me` | authenticated | Current principal: id, name, email, role, company, permissions. |
| POST | `/auth/forgot-password` | public | Sends a reset link. Always 202, never reveals whether the address exists. |
| POST | `/auth/reset-password` | public | Token + new password. Enforces policy and history. |
| POST | `/auth/change-password` | authenticated | Current + new password. |
| GET | `/auth/sessions` | authenticated | Active sessions with device and last-used. |
| DELETE | `/auth/sessions/{id}` | authenticated | Revoke one session. |

Rate limits: 5 attempts / 15 min per IP+email on `login`, `forgot-password` and `reset-password`.

---

## 3. Companies, users, settings

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/companies` | A | List all companies. |
| POST | `/companies` | A | Create. |
| GET | `/companies/{id}` | A CA CR | Detail, including analyzer counts by provider. |
| PATCH | `/companies/{id}` | A CA | Update. |
| DELETE | `/companies/{id}` | A | Soft delete. |
| GET | `/users` | A CA CR | Users in scope. |
| POST | `/users` | A CA | Create. Role options constrained by the caller's own role. |
| PATCH | `/users/{id}` | A CA | Update. |
| DELETE | `/users/{id}` | A CA | Soft delete. |
| GET | `/profile` | all | Own profile. |
| PATCH | `/profile` | all | Update own name, email, phone. |
| GET | `/smtp-settings` | A | Per-company SMTP configuration (password never returned). |
| PUT | `/smtp-settings` | A | Upsert. |
| POST | `/smtp-settings/test` | A | Send a test message. |

---

## 4. Buildings, analyzers, plants

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/buildings` | A CA CR BA BR D | Buildings in scope. `?include=analyzer_count,active_status`. |
| POST | `/buildings` | A CA | Create. |
| GET | `/buildings/{id}` | scope | Detail with contacts and tariff history summary. |
| PATCH | `/buildings/{id}` | A CA | Update. |
| DELETE | `/buildings/{id}` | A CA | Soft delete. Rejected with 409 while analyzers are attached. |
| GET | `/buildings/{id}/comparison` | scope | Sectoral comparison figures and ranks (`02-domain-rules.md` §10.4). |
| GET | `/analyzers` | scope | Analyzers in scope. Filters: `building_id`, `provider`, `is_active`, `q`. |
| GET | `/analyzers/{id}` | scope | Detail. |
| PATCH | `/analyzers/{id}` | A CA | Update assignable fields (building, multiplier, installed power, coordinates). |
| POST | `/analyzers/{id}/refresh` | A CA BA | Enqueue an on-demand data pull. Returns a job id. |
| GET | `/power-plants` | A CA CR | Plants in scope. |
| POST | `/power-plants` | A CA | Create. |
| GET | `/power-plants/{id}` | A CA CR | Detail with devices and monthly targets. |
| PATCH | `/power-plants/{id}` | A CA | Update. |
| DELETE | `/power-plants/{id}` | A CA | Soft delete. |

---

## 5. Consumption and analysis

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/consumption` | scope | Consumption series. Required: `analyzer_id` or `building_id`, `granularity` (`hourly|daily|monthly|yearly`), `from`, `to`. Returns rows with every index, consumption and generation field plus ratios and max demand. Served from continuous aggregates. |
| GET | `/consumption/summary` | scope | Totals and averages for the range: active, inductive, capacitive, generation, peak, valley. |
| GET | `/consumption/export` | scope | Same query, returns CSV or XLSX (`?format=`). |
| GET | `/consumption/anomalies` | scope | Unresolved suspect periods. |
| POST | `/consumption/anomalies/{id}/resolve` | A CA | Resolve: register a reset, supply an override, or accept. |
| GET | `/load-profile` | scope | Averaged 24-hour profiles. `?analyzer_id&from&to&profiles=weekday,weekend,winter_weekday,…`. Returns 24 hourly means per requested profile. |
| GET | `/load-profile/statistics` | scope | Per profile: max, min, hour of max, mean, stddev, range, load factor. |
| GET | `/load-profile/export` | scope | XLSX of the hourly matrix. |
| GET | `/generation` | scope | Generation series from export registers, same shape as `/consumption`. |
| GET | `/energy-balance` | scope | Generation, consumption, grid import and grid export for the range. |

---

## 6. Tariffs

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/tariffs` | scope | Tariff history. `?building_id=`. |
| POST | `/tariffs` | A CA | Create a tariff version, with taxes and manual YEKDEM entries. |
| GET | `/tariffs/{id}` | scope | Detail. |
| PATCH | `/tariffs/{id}` | A CA | Update. |
| DELETE | `/tariffs/{id}` | A CA | Soft delete. |
| GET | `/tariffs/applicable` | scope | The tariff in force. `?building_id&date=`. |
| GET | `/tariff-templates` | A CA CR | Company templates. |
| POST | `/tariff-templates` | A CA | Create. |
| PATCH | `/tariff-templates/{id}` | A CA | Update. |
| DELETE | `/tariff-templates/{id}` | A CA | Delete. |
| POST | `/tariff-templates/{id}/apply` | A CA | Apply to a set of buildings from a given effective date. |
| GET | `/buildings/bulk-tariff/current` | A CA CR | Every building's current tariff. |
| POST | `/buildings/bulk-tariff` | A CA | Assign one tariff definition to many buildings. |
| GET | `/buildings/bulk-tariff/history` | A CA CR | Bulk assignment history. |
| GET | `/solar-tariffs` | A CA CR | `?plant_id=`. Tariff history for a plant. |
| POST | `/solar-tariffs` | A CA | Create. |
| DELETE | `/solar-tariffs/{id}` | A CA | Soft delete. |
| GET | `/national-tariff-schedule` | public | The published schedule backing the public calculator. |

### İcmal import

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| POST | `/icmal-imports` | A CA | Upload a CSV/XLSX. Parses, matches rows to buildings, derives coefficients, returns the analysis **without applying anything**. |
| GET | `/icmal-imports/{id}` | A CA | The analysis: per-coefficient value, sample count, standard deviation, stability flag, back-calculation error, warnings. |
| POST | `/icmal-imports/{id}/apply` | A CA | Write the confirmed coefficients into a new tariff version. Requires an explicit per-building confirmation list. |

---

## 7. Billing

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/bills` | scope | List bills. Filters: `scope`, `building_id`, `analyzer_id`, `period`, `status`. |
| GET | `/bills/{id}` | scope | Full bill with all lines and members. |
| GET | `/bills/{id}/pdf` | scope | The invoice PDF. Generated on demand if absent. |
| GET | `/bills/{id}/hourly-detail` | scope | Per-hour PTF detail. `?format=json|xlsx`. |
| POST | `/bills/compute` | A CA BA | Compute (or recompute) bills for a scope and period. Body: `scope`, target ids, `period`, `force`. Returns a job id. Recomputation supersedes rather than deletes. |
| GET | `/bills/dashboard` | scope | The month's invoice dashboard: per-building analyzer rows, per-plant rows, and the netting summary. `?year&month`. |
| GET | `/bills/dashboard/export` | scope | The whole dashboard as a single PDF or XLSX. |
| GET | `/bills/latest` | scope | Most recent bill for the scope — powers the dashboard card. |

Compute failures are surfaced explicitly with a machine code: `tariff_not_found`,
`no_consumption_data`, `unresolved_anomaly`, `ptf_data_missing`, `period_not_closed`.

---

## 8. Solar plants and iSolarCloud

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/plants/{id}/realtime` | A CA CR | Current active power, today's yield, month, year, total, capacity utilisation. |
| GET | `/plants/{id}/production` | A CA CR | Production series. `?granularity&from&to`. |
| GET | `/plants/{id}/production/export` | A CA CR | XLSX. |
| GET | `/plants/{id}/devices` | A CA CR | Device list with status, power and last update. |
| GET | `/plants/{id}/alarms` | A CA CR | Fault alarms, translated. |
| POST | `/plants/{id}/sync` | A CA | Enqueue an on-demand sync. |
| GET | `/plants/{id}/revenue` | A CA CR | Daily, monthly, yearly and total revenue from the feed-in tariff. |
| GET | `/integrations/isolar/plants` | A CA | Plants available on the connected iSolarCloud account, for linking. |
| POST | `/plants/{id}/isolar-link` | A CA | Bind a plant to an iSolarCloud plant; imports capacity, name and devices. |
| DELETE | `/plants/{id}/isolar-link` | A CA | Unlink. |
| PUT | `/plants/{id}/alarm-recipients` | A CA | Set alarm forwarding recipients. |

---

## 9. Financial and renewable analysis

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/financial/summary` | A CA CR | Headline figures for a year (and optional month): analyzer count, plant count, consumption, production, revenue, cost, offset balance, tariff prices in force. |
| GET | `/financial/monthly` | A CA CR | Per-month breakdown: consumption, production, grid purchase, grid sale, cost, revenue, net. |
| GET | `/renewable/overview` | scope | Generation summary cards for the range. |
| GET | `/renewable/realtime` | scope | Current power, today's generation, system efficiency, 24-hour series. |
| GET | `/renewable/grid-interaction` | scope | Flow direction, today's import and export, grid quality, financial impact. |
| GET | `/renewable/environmental` | scope | CO₂ avoided plus equivalences (trees, coal, car km, home heating). |
| GET | `/renewable/efficiency` | scope | Overall and per-component efficiency, trend series, recommendations. |
| GET | `/renewable/forecast` | scope | Estimated generation, estimated consumption, net excess, accuracy metrics. |
| GET | `/renewable/analytics` | scope | Peak/average generation, efficiency, savings, trend, insights. |
| GET | `/renewable/system-status` | scope | Overall health plus per-component status. |
| GET | `/weather` | scope | Current conditions and 7-day forecast for a plant or building location, plus a generation-potential rating. |

> Every one of these returns real values or an explicit `"available": false` with a reason.
> None of them returns synthesised or placeholder data. See `10-removed-behaviours.md` item 12.

---

## 10. Alarms and messages

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/alarms` | scope | Alarm rules with their analyzers and channels. |
| POST | `/alarms` | A CA BA | Create. Body validated per alarm type. |
| GET | `/alarms/{id}` | scope | Detail. |
| PATCH | `/alarms/{id}` | A CA BA | Update, including the enable/disable toggle. |
| DELETE | `/alarms/{id}` | A CA BA | Soft delete. |
| GET | `/alarms/{id}/events` | scope | Firing history. |
| POST | `/alarms/{id}/evaluate` | A CA | Evaluate now, without sending notifications (dry run) unless `notify=true`. |
| GET | `/messages` | scope | Operational messages. Filters: `kind`, `status`, `q`, `from`, `to`. |
| GET | `/job-runs` | A CA | Job execution history with counts and errors. |
| POST | `/job-runs/{type}/trigger` | A | Manually trigger a job for an explicit scope and period. |

---

## 11. Reports

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/reports` | scope | Archive. Filters: `type`, `building_id`, `year`. |
| GET | `/reports/{id}` | scope | Full report payload for on-screen rendering. |
| POST | `/reports/generate` | A CA BA | Generate for buildings + period + plant selection. Returns a job id. |
| GET | `/reports/{id}/pdf` | scope | PDF. |
| GET | `/reports/{id}/excel` | scope | XLSX. |
| POST | `/reports/{id}/email` | A CA BA | Send to a given address with the prepared subject and body. |
| GET | `/reports/preview` | scope | Compute report figures without persisting — powers the live monthly/yearly tabs. |

---

## 12. Carbon module

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/carbon/overview` | scope | Totals, category distribution, scope distribution, monthly series with prior-year comparison, recent activities. |
| GET | `/carbon/activity-catalogue` | scope | The full category → sub-category → activity-type tree with scope and ISO mapping. |
| GET | `/carbon/selected-activities` | scope | The building's declared scope. |
| PUT | `/carbon/selected-activities` | A CA | Replace the declared scope. |
| GET | `/carbon/activities` | scope | Recorded activities. Filters: `building_id`, `from`, `to`, `scope`, `status`, `type`. |
| POST | `/carbon/activities` | A CA | Create. Emission computed server-side. |
| PATCH | `/carbon/activities/{id}` | A CA | Update; emission recomputed. |
| DELETE | `/carbon/activities/{id}` | A CA | Delete. |
| POST | `/carbon/activities/{id}/status` | A CA | Approve or reject. |
| GET | `/carbon/emission-factors` | scope | The company's factor catalogue with conversions. |
| PATCH | `/carbon/emission-factors/{id}` | A CA | Override a factor for this company. |
| POST | `/carbon/emission-factors/reset` | A CA | Restore from the platform master catalogue. |
| GET | `/carbon/reports` | scope | Report history. |
| POST | `/carbon/reports` | A CA | Generate a GHG or ISO 14064 report for a period. |
| GET | `/carbon/reports/{id}/pdf` | scope | PDF. |

---

## 13. ISO 50001

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/iso50001/{building_id}` | scope | Project state: clause dates, per-clause note and file counts, overall progress. |
| PUT | `/iso50001/{building_id}/dates` | A CA BA | Set clause start/end dates. Validates ordering and completeness. |
| GET | `/iso50001/{building_id}/clauses/{clause}/notes` | scope | Notes for a clause. |
| POST | `/iso50001/{building_id}/clauses/{clause}/notes` | A CA BA | Add a note. |
| PATCH | `/iso50001/notes/{id}` | A CA BA | Update. |
| DELETE | `/iso50001/notes/{id}` | A CA BA | Delete. |
| GET | `/iso50001/{building_id}/clauses/{clause}/files` | scope | Files for a clause. |
| POST | `/iso50001/{building_id}/clauses/{clause}/files` | A CA BA | Upload (multipart, ≤ 30 MB, type allow-list). |
| DELETE | `/iso50001/files/{id}` | A CA BA | Delete. |
| GET | `/iso50001/{building_id}/export` | scope | Zip archive of every note and file. Streamed. |
| GET | `/iso50001/templates` | authenticated | Downloadable clause templates. |

---

## 14. Calendar

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/calendar/events` | scope | Events in a range. |
| POST | `/calendar/events` | A CA | Create. |
| PATCH | `/calendar/events/{id}` | A CA | Update. |
| DELETE | `/calendar/events/{id}` | A CA | Delete. |
| GET | `/calendar/vacations` | scope | Weekend day configuration and vacation periods. |
| PUT | `/calendar/vacations` | A CA | Replace the configuration. |

---

## 15. Integrations (administration)

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/integration-definitions` | A | Provider catalogue. |
| POST | `/integration-definitions` | A | Create a provider/subtype with its endpoint templates. |
| PATCH | `/integration-definitions/{id}` | A | Update. |
| DELETE | `/integration-definitions/{id}` | A | Delete; 409 while credentials reference it. |
| GET | `/integration-credentials` | A CA | Configured integrations for the company. **Never returns secrets** — only provider, subtype, username, status, last verification. |
| POST | `/integration-credentials` | A CA | Configure an integration. Secrets are write-only. |
| PATCH | `/integration-credentials/{id}` | A CA | Update; omitted secret fields are left unchanged. |
| DELETE | `/integration-credentials/{id}` | A CA | Remove. |
| POST | `/integration-credentials/{id}/verify` | A CA | Test authentication against the provider. |
| POST | `/integration-credentials/{id}/discover` | A CA | Discover metering points and create/update analyzers. |
| POST | `/integration-credentials/{id}/backfill` | A CA | Enqueue a historical pull for an explicit date range. |
| GET | `/integrations/isolar/authorize-url` | A CA | Build the iSolarCloud OAuth authorisation URL. |
| GET | `/integrations/isolar/callback` | public | OAuth callback; exchanges the code for tokens. |

---

## 16. Forecasting

| Method | Path | Roles | Purpose |
|--------|------|-------|---------|
| GET | `/forecast` | scope | Stored forecast for an analyzer. `?analyzer_id&from&to`. Returns median, p10, p90 and any gaps. |
| POST | `/forecast/run` | A CA BA | Run a forecast now for an analyzer and horizon. |
| POST | `/forecast/weekly` | A CA BA | Weekly forecast from a week start. |
| POST | `/forecast/monthly` | A CA BA | Monthly forecast for a month. |
| POST | `/anomaly/check` | A CA BA | Check one actual value against the model. |

---

## 17. Public endpoints

Unauthenticated, rate limited, no tenant data.

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/public/contact` | Contact form. |
| POST | `/public/demo-request` | Demo request form. |
| POST | `/public/bill-calculator` | Estimate a bill from the national tariff schedule. |
| GET | `/public/blog` | Article list with category filter. |
| GET | `/public/blog/{slug}` | Article content. |
| GET | `/public/references` | Customer references. |
| GET | `/public/pricing` | Pricing packages. |

---

## 18. Mobile

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/mobile/auth/login` | Returns access + refresh tokens. |
| POST | `/mobile/auth/refresh` | Rotates tokens. |
| GET | `/mobile/auth/me` | Profile and permissions. |

All other endpoints are shared with the web client; mobile authenticates with a bearer token.

---

## 19. Operational

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/health/live` | Process liveness. |
| GET | `/health/ready` | Database, Redis and migration state. |
| GET | `/metrics` | Prometheus, internal network only. |
| GET | `/api/v1/openapi.json` | The specification. |
| GET | `/version` | Build version, commit, build time. |
