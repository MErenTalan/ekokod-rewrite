# ekokod API reference

Generated from the OpenAPI document (`ekokod tool openapi`) by `make api-docs`; do not edit.
Version 1 · 182 operations · 263 schemas. Authentication: a session cookie (`ekokod_at`) or a mobile bearer token; operations marked *public* need neither.
Errors 400, 401, 403, 429 and 500 apply to every operation and use the `Error` schema.

## alarms

### `GET /api/v1/alarms`

Alarm rules with their analyzers and channels. `alarms.list`

| parameter | in | required | type |
|---|---|---|---|
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `type` | query |  | string: reactive_limit \| data_communication \| current_voltage_power \| invoice_increase? |
| `is_enabled` | query |  | boolean? |
| `analyzer_id` | query |  | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`AlarmPage`](#schema-alarmpage), 404

### `POST /api/v1/alarms`

Create a rule; the body is validated per alarm type. `alarms.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`AlarmFields`](#schema-alarmfields)

Responses: 201 [`Alarm`](#schema-alarm), 404, 409, 422

### `GET /api/v1/alarms/{id}`

One rule. `alarms.get`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Alarm`](#schema-alarm), 404

### `PATCH /api/v1/alarms/{id}`

Replace a rule, including the enable toggle. `alarms.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`AlarmUpdateRequest`](#schema-alarmupdaterequest)

Responses: 200 [`Alarm`](#schema-alarm), 404, 409, 422

### `DELETE /api/v1/alarms/{id}`

Soft delete a rule. `alarms.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `POST /api/v1/alarms/{id}/evaluate`

Evaluate now; a dry run unless notify=true. `alarms.evaluate`

| parameter | in | required | type |
|---|---|---|---|
| `notify` | query |  | boolean |
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`AlarmEvaluation`](#schema-alarmevaluation), 404, 409, 422

### `GET /api/v1/alarms/{id}/events`

Firing history. `alarms.events.list`

| parameter | in | required | type |
|---|---|---|---|
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `from` | query |  | string (date-time)? |
| `to` | query |  | string (date-time)? |
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`AlarmEventPage`](#schema-alarmeventpage), 404

## analyzers

### `GET /api/v1/analyzers`

Analyzers in scope. `analyzers.list`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | query |  | `UuidUUID` |
| `unassigned` | query |  | boolean |
| `provider` | query |  | string[] |
| `is_active` | query |  | boolean? |
| `q` | query |  | string |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`AnalyzerPage`](#schema-analyzerpage), 404

### `GET /api/v1/analyzers/{id}`

One analyzer. `analyzers.get`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Analyzer`](#schema-analyzer), 404

### `PATCH /api/v1/analyzers/{id}`

Update an analyzer's building, multiplier, installed power or coordinates. `analyzers.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`AnalyzerUpdateRequest`](#schema-analyzerupdaterequest)

Responses: 200 [`Analyzer`](#schema-analyzer), 404, 409, 422

### `POST /api/v1/analyzers/{id}/refresh`

Enqueue an on-demand pull of hourly or energy values. `analyzers.refresh`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`AnalyzerRefreshRequest`](#schema-analyzerrefreshrequest)

Responses: 202 [`JobAccepted`](#schema-jobaccepted), 404, 409, 422

## auth

### `POST /api/v1/auth/change-password`

Change the current user's password; other sessions are revoked. `auth.change_password`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`ChangePasswordRequest`](#schema-changepasswordrequest)

Responses: 204, 404, 409, 422

### `POST /api/v1/auth/forgot-password`

Send a password reset link; always 202. *public* · `auth.forgot_password`

Body: [`ForgotPasswordRequest`](#schema-forgotpasswordrequest)

Responses: 202, 409, 422

### `POST /api/v1/auth/login`

Sign in with e-mail and password; sets the session cookies. *public* · `auth.login`

Body: [`LoginRequest`](#schema-loginrequest)

Responses: 200 [`Me`](#schema-me), 409, 422

### `POST /api/v1/auth/logout`

Revoke the current session. `auth.logout`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `POST /api/v1/auth/logout-all`

Revoke every session of the current user. `auth.logout_all`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `GET /api/v1/auth/me`

The current principal with its permissions. `auth.me`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Me`](#schema-me), 404

### `POST /api/v1/auth/refresh`

Rotate the refresh cookie and issue a new access cookie. *public* · `auth.refresh`

Responses: 204, 409, 422

### `POST /api/v1/auth/reset-password`

Set a new password with a reset token. *public* · `auth.reset_password`

Body: [`ResetPasswordRequest`](#schema-resetpasswordrequest)

Responses: 204, 409, 422

### `GET /api/v1/auth/sessions`

Active sessions of the current user. `auth.sessions.list`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`SessionList`](#schema-sessionlist), 404

### `DELETE /api/v1/auth/sessions/{id}`

Revoke one of the current user's sessions. `auth.sessions.revoke`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

## bills

### `GET /api/v1/bills`

Bills in scope. `bills.list`

| parameter | in | required | type |
|---|---|---|---|
| `scope` | query |  | string: analyzer \| building \| company |
| `building_id` | query |  | `UuidUUID` |
| `analyzer_id` | query |  | `UuidUUID` |
| `period` | query |  | string |
| `status` | query |  | string[] |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`BillPage`](#schema-billpage), 404

### `POST /api/v1/bills/compute`

Enqueue bill computation; recomputation supersedes. `bills.compute`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`BillComputeRequest`](#schema-billcomputerequest)

Responses: 202 [`BillComputeAccepted`](#schema-billcomputeaccepted), 404, 409, 422

### `GET /api/v1/bills/dashboard`

The month's invoice dashboard: analyzer rows per building, their totals and the netting summary. `bills.dashboard`

| parameter | in | required | type |
|---|---|---|---|
| `year` | query | yes | integer |
| `month` | query | yes | integer |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`BillDashboard`](#schema-billdashboard), 404

### `GET /api/v1/bills/dashboard/export`

The whole dashboard as one XLSX or PDF. `bills.dashboard.export`

| parameter | in | required | type |
|---|---|---|---|
| `year` | query | yes | integer |
| `month` | query | yes | integer |
| `format` | query |  | string: xlsx \| pdf |
| `company_id` | query |  | `UuidUUID` |

Responses: 200, 404

### `GET /api/v1/bills/latest`

The most recent bill of a subject. `bills.latest`

| parameter | in | required | type |
|---|---|---|---|
| `scope` | query | yes | string: analyzer \| building \| company |
| `subject_id` | query | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Bill`](#schema-bill), 404

### `GET /api/v1/bills/{id}`

A bill with lines and members. `bills.get`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`BillDetail`](#schema-billdetail), 404

### `GET /api/v1/bills/{id}/hourly-detail`

Per-hour PTF detail as JSON or XLSX. `bills.hourly_detail`

| parameter | in | required | type |
|---|---|---|---|
| `format` | query |  | string: json \| xlsx |
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`BillHours`](#schema-billhours), 404

### `GET /api/v1/bills/{id}/pdf`

The invoice PDF, rendered on demand when not yet stored. `bills.pdf`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200, 404

## buildings

### `GET /api/v1/buildings`

Buildings in scope; include=analyzer_count,active_status. `buildings.list`

| parameter | in | required | type |
|---|---|---|---|
| `include` | query |  | string: analyzer_count \| active_status[] |
| `q` | query |  | string |
| `sector` | query |  | string |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`BuildingPage`](#schema-buildingpage), 404

### `POST /api/v1/buildings`

Create a building. `buildings.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`BuildingCreateRequest`](#schema-buildingcreaterequest)

Responses: 201 [`BuildingDetail`](#schema-buildingdetail), 404, 409, 422

### `GET /api/v1/buildings/{id}`

A building with contacts and tariff history. `buildings.get`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`BuildingDetail`](#schema-buildingdetail), 404

### `PATCH /api/v1/buildings/{id}`

Update a building. `buildings.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`BuildingUpdateRequest`](#schema-buildingupdaterequest)

Responses: 200 [`BuildingDetail`](#schema-buildingdetail), 404, 409, 422

### `DELETE /api/v1/buildings/{id}`

Soft-delete a building with no analyzers. `buildings.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `GET /api/v1/buildings/{id}/comparison`

Sectoral comparison figures and ranks (02 §10.4). `buildings.comparison`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`BuildingComparison`](#schema-buildingcomparison), 404

## calendar

### `GET /api/v1/calendar/events`

Events overlapping a date range. `calendar.events.list`

| parameter | in | required | type |
|---|---|---|---|
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`CalendarEvents`](#schema-calendarevents), 404

### `POST /api/v1/calendar/events`

Create an event. `calendar.events.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`CalendarEventFields`](#schema-calendareventfields)

Responses: 201 [`CalendarEvent`](#schema-calendarevent), 404, 409, 422

### `PATCH /api/v1/calendar/events/{id}`

Replace an event. `calendar.events.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`CalendarEventUpdateRequest`](#schema-calendareventupdaterequest)

Responses: 200 [`CalendarEvent`](#schema-calendarevent), 404, 409, 422

### `DELETE /api/v1/calendar/events/{id}`

Delete an event. `calendar.events.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `GET /api/v1/calendar/vacations`

Weekend days and vacation periods. `calendar.vacations.get`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Vacations`](#schema-vacations), 404

### `PUT /api/v1/calendar/vacations`

Replace weekend days and vacation periods. `calendar.vacations.put`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`VacationsPutRequest`](#schema-vacationsputrequest)

Responses: 200 [`Vacations`](#schema-vacations), 404, 409, 422

## carbon

### `GET /api/v1/carbon/activities`

Recorded activities; from/to select overlap. `carbon.activities`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | query |  | `UuidUUID` |
| `from` | query |  | [`Date`](#schema-date) |
| `to` | query |  | [`Date`](#schema-date) |
| `scope` | query |  | string: scope_1 \| scope_2 \| scope_3? |
| `status` | query |  | string: pending \| approved \| rejected? |
| `type` | query |  | string? |
| `automated` | query |  | boolean? |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`CarbonActivityPage`](#schema-carbonactivitypage), 404

### `POST /api/v1/carbon/activities`

Record an activity; the emission is computed here. `carbon.activities.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`CarbonActivityCreateRequest`](#schema-carbonactivitycreaterequest)

Responses: 201 [`CarbonActivity`](#schema-carbonactivity), 404, 409, 422

### `PATCH /api/v1/carbon/activities/{id}`

Edit a manual activity; recomputed and back to pending. `carbon.activities.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`CarbonActivityUpdateRequest`](#schema-carbonactivityupdaterequest)

Responses: 200 [`CarbonActivity`](#schema-carbonactivity), 404, 409, 422

### `DELETE /api/v1/carbon/activities/{id}`

Delete a manual activity. `carbon.activities.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `POST /api/v1/carbon/activities/{id}/status`

Approve or reject an activity. `carbon.activities.status`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`CarbonStatusRequest`](#schema-carbonstatusrequest)

Responses: 200 [`CarbonActivity`](#schema-carbonactivity), 404, 409, 422

### `GET /api/v1/carbon/activity-catalogue`

The main/sub-category tree with the derived GHG scope and ISO 14064 category. `carbon.activity_catalogue`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`CarbonCatalogue`](#schema-carboncatalogue), 404

### `GET /api/v1/carbon/emission-factors`

The company's effective factor catalogue with conversions. `carbon.emission_factors`

| parameter | in | required | type |
|---|---|---|---|
| `sub_category` | query |  | string? |
| `main_category` | query |  | string? |
| `q` | query |  | string? |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`EmissionFactorList`](#schema-emissionfactorlist), 404

### `POST /api/v1/carbon/emission-factors/reset`

Drop every company override. `carbon.emission_factors.reset`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `PATCH /api/v1/carbon/emission-factors/{id}`

Override a factor for this company. `carbon.emission_factors.override`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`EmissionFactorOverrideRequest`](#schema-emissionfactoroverriderequest)

Responses: 200 [`EmissionFactorView`](#schema-emissionfactorview), 404, 409, 422

### `GET /api/v1/carbon/overview`

A building's year: totals, categories, scopes, months against the year before, recent records. `carbon.overview`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | query | yes | `UuidUUID` |
| `year` | query |  | integer? |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`CarbonOverview`](#schema-carbonoverview), 404

### `GET /api/v1/carbon/reports`

Report history. `carbon.reports`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | query |  | `UuidUUID` |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`CarbonReportSummaryPage`](#schema-carbonreportsummarypage), 404

### `POST /api/v1/carbon/reports`

Generate a GHG Protocol or ISO 14064 report for a period. `carbon.reports.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`CarbonReportRequest`](#schema-carbonreportrequest)

Responses: 201 [`CarbonReportSummary`](#schema-carbonreportsummary), 404, 409, 422

### `GET /api/v1/carbon/reports/{id}/pdf`

The report as PDF. `carbon.reports.pdf`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200, 404

### `GET /api/v1/carbon/selected-activities`

The building's declared sub-categories. `carbon.selected_activities`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | query | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`CarbonSelection`](#schema-carbonselection), 404

### `PUT /api/v1/carbon/selected-activities`

Replace the building's declared sub-categories. `carbon.selected_activities.put`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | query | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`CarbonSelectionRequest`](#schema-carbonselectionrequest)

Responses: 200 [`CarbonSelection`](#schema-carbonselection), 404, 409, 422

## companies

### `GET /api/v1/companies`

Every company (platform operator). `companies.list`

| parameter | in | required | type |
|---|---|---|---|
| `q` | query |  | string |
| `sector` | query |  | string? |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`CompanyPage`](#schema-companypage), 404

### `POST /api/v1/companies`

Create a company. `companies.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`CompanyCreateRequest`](#schema-companycreaterequest)

Responses: 201 [`Company`](#schema-company), 404, 409, 422

### `GET /api/v1/companies/{id}`

A company with analyzer counts by provider. `companies.get`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`CompanyDetail`](#schema-companydetail), 404

### `PATCH /api/v1/companies/{id}`

Update a company. `companies.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`CompanyUpdateRequest`](#schema-companyupdaterequest)

Responses: 200 [`Company`](#schema-company), 404, 409, 422

### `DELETE /api/v1/companies/{id}`

Soft-delete a company. `companies.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

## consumption

### `GET /api/v1/consumption`

Consumption series for an analyzer or a building (summed). `consumption.series`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `granularity` | query | yes | string: hourly \| daily \| monthly \| yearly |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ConsumptionSeries`](#schema-consumptionseries), 404

### `GET /api/v1/consumption/anomalies`

Suspect periods. `consumption.anomalies`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `unresolved` | query |  | boolean |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`AnomalyPage`](#schema-anomalypage), 404

### `POST /api/v1/consumption/anomalies/{id}/resolve`

Resolve a suspect period: register a reset, override, or accept. `consumption.anomalies.resolve`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`AnomalyResolveRequest`](#schema-anomalyresolverequest)

Responses: 200 [`Anomaly`](#schema-anomaly), 404, 409, 422

### `GET /api/v1/consumption/export`

The series as CSV or XLSX. `consumption.export`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `granularity` | query | yes | string: hourly \| daily \| monthly \| yearly |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `format` | query | yes | string: csv \| xlsx |
| `company_id` | query |  | `UuidUUID` |

Responses: 200, 404

### `GET /api/v1/consumption/grouped`

Consumption grouped by week, day type or season, with the previous period (R193). `consumption.grouped`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `group_by` | query | yes | string: daily \| week \| day_type \| season \| season_day_type |
| `compare` | query |  | string: previous |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ConsumptionGrouped`](#schema-consumptiongrouped), 404

### `GET /api/v1/consumption/reactive-status`

Reactive ratios, limits and penalty status for a month (R166). `consumption.reactive_status`

| parameter | in | required | type |
|---|---|---|---|
| `month` | query |  | string |
| `building_id` | query |  | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ReactiveStatus`](#schema-reactivestatus), 404

### `GET /api/v1/consumption/summary`

Totals, averages, peak and valley for the range. `consumption.summary`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `granularity` | query | yes | string: hourly \| daily \| monthly \| yearly |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ConsumptionSummary`](#schema-consumptionsummary), 404

### `GET /api/v1/energy-balance`

Consumption, generation, grid import and export. `energy_balance`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `granularity` | query | yes | string: hourly \| daily \| monthly \| yearly |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`EnergyBalance`](#schema-energybalance), 404

### `GET /api/v1/generation`

Generation series from the export registers. `generation.series`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `granularity` | query | yes | string: hourly \| daily \| monthly \| yearly |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ConsumptionSeries`](#schema-consumptionseries), 404

## financial

### `GET /api/v1/financial/monthly`

Twelve months and the year row; a month without data is null, never zero. `financial.monthly`

| parameter | in | required | type |
|---|---|---|---|
| `year` | query | yes | integer |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`FinancialMonthly`](#schema-financialmonthly), 404

### `GET /api/v1/financial/summary`

Company-wide consumption, cost, production, revenue and net for a year or month, with the tariffs in force. `financial.summary`

| parameter | in | required | type |
|---|---|---|---|
| `year` | query | yes | integer |
| `month` | query |  | integer? |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`FinancialSummary`](#schema-financialsummary), 404

## forecast

### `POST /api/v1/anomaly/check`

Check one hour's consumption against the model; available=false when the ML service is down. `anomaly.check`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`AnomalyCheckRequest`](#schema-anomalycheckrequest)

Responses: 200 [`AnomalyCheck`](#schema-anomalycheck), 404, 409, 422

### `GET /api/v1/forecast`

The latest stored forecast covering the window, with its gaps; never calls the ML service. `forecast.read`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query | yes | `UuidUUID` |
| `from` | query | yes | string (date-time)? |
| `to` | query | yes | string (date-time)? |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Forecast`](#schema-forecast), 404

### `POST /api/v1/forecast/monthly`

A daily forecast for one month (not stored). `forecast.monthly`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`ForecastMonthlyRequest`](#schema-forecastmonthlyrequest)

Responses: 200 [`Forecast`](#schema-forecast), 404, 409, 422

### `POST /api/v1/forecast/run`

Forecast an analyzer now and store the run. `forecast.run`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`ForecastRunRequest`](#schema-forecastrunrequest)

Responses: 200 [`Forecast`](#schema-forecast), 404, 409, 422

### `POST /api/v1/forecast/weekly`

An hourly forecast for one week (not stored). `forecast.weekly`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`ForecastWeeklyRequest`](#schema-forecastweeklyrequest)

Responses: 200 [`Forecast`](#schema-forecast), 404, 409, 422

## integrations

### `GET /api/v1/integration-credentials`

Configured integrations; never secrets. `integration_credentials.list`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`IntegrationCredentials`](#schema-integrationcredentials), 404

### `POST /api/v1/integration-credentials`

Configure an integration; secrets are write-only. `integration_credentials.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`IntegrationCredentialCreateRequest`](#schema-integrationcredentialcreaterequest)

Responses: 201 [`IntegrationCredential`](#schema-integrationcredential), 404, 409, 422

### `PATCH /api/v1/integration-credentials/{id}`

Update; omitted secrets are kept. `integration_credentials.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`IntegrationCredentialUpdateRequest`](#schema-integrationcredentialupdaterequest)

Responses: 200 [`IntegrationCredential`](#schema-integrationcredential), 404, 409, 422

### `DELETE /api/v1/integration-credentials/{id}`

Remove an integration. `integration_credentials.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `POST /api/v1/integration-credentials/{id}/backfill`

Enqueue a historical pull. `integration_credentials.backfill`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`BackfillRequest`](#schema-backfillrequest)

Responses: 202 [`JobAccepted`](#schema-jobaccepted), 404, 409, 422

### `POST /api/v1/integration-credentials/{id}/discover`

Discover metering points. `integration_credentials.discover`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 202 [`JobAccepted`](#schema-jobaccepted), 404, 409, 422

### `POST /api/v1/integration-credentials/{id}/verify`

Test authentication against the provider. `integration_credentials.verify`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`IntegrationCredential`](#schema-integrationcredential), 404, 409, 422

### `GET /api/v1/integration-definitions`

The provider catalogue. `integration_definitions.list`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`IntegrationDefinitions`](#schema-integrationdefinitions), 404

### `POST /api/v1/integration-definitions`

Add a provider/subtype. `integration_definitions.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`IntegrationDefinitionCreateRequest`](#schema-integrationdefinitioncreaterequest)

Responses: 201 [`IntegrationDefinition`](#schema-integrationdefinition), 404, 409, 422

### `PATCH /api/v1/integration-definitions/{id}`

Update a definition. `integration_definitions.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`IntegrationDefinitionUpdateRequest`](#schema-integrationdefinitionupdaterequest)

Responses: 200 [`IntegrationDefinition`](#schema-integrationdefinition), 404, 409, 422

### `DELETE /api/v1/integration-definitions/{id}`

Delete an unused definition. `integration_definitions.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `GET /api/v1/integrations/isolar/authorize-url`

The iSolarCloud authorisation URL; binds the flow to this browser (R187). `isolar.authorize_url`

| parameter | in | required | type |
|---|---|---|---|
| `credential_id` | query | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`AuthorizeURL`](#schema-authorizeurl), 404

### `GET /api/v1/integrations/isolar/callback`

OAuth callback; redirects to Settings. *public* · `isolar.callback`

Responses: 302

### `GET /api/v1/integrations/isolar/plants`

Plants on the connected iSolarCloud account, for linking. `isolar.plants`

| parameter | in | required | type |
|---|---|---|---|
| `credential_id` | query | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ISolarAccountPlants`](#schema-isolaraccountplants), 404

## iso50001

### `GET /api/v1/iso50001/clauses`

TS EN ISO 50001:2018 clauses 5–9 with every sub-clause's text, in the request locale. `iso50001.clauses`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ISOClauses`](#schema-isoclauses), 404

### `GET /api/v1/iso50001/files/{id}`

The evidence file, served only through this authorising handler. `iso50001.files.download`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200, 404

### `DELETE /api/v1/iso50001/files/{id}`

Delete an evidence file. `iso50001.files.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `PATCH /api/v1/iso50001/notes/{id}`

Edit a note. `iso50001.notes.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`ISONoteUpdateRequest`](#schema-isonoteupdaterequest)

Responses: 200 [`ISONote`](#schema-isonote), 404, 409, 422

### `DELETE /api/v1/iso50001/notes/{id}`

Delete a note. `iso50001.notes.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `GET /api/v1/iso50001/templates`

Downloadable clause templates. `iso50001.templates`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ISOTemplates`](#schema-isotemplates), 404

### `GET /api/v1/iso50001/templates/{id}`

One template file. `iso50001.template`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200, 404

### `GET /api/v1/iso50001/{building_id}`

Project state: clause dates and Gantt statuses, per-sub-clause counts, progress. `iso50001.project`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ISOProject`](#schema-isoproject), 404

### `GET /api/v1/iso50001/{building_id}/clauses/{clause}/files`

A sub-clause's evidence files. `iso50001.files`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | path | yes | `UuidUUID` |
| `clause` | path | yes | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ISOFiles`](#schema-isofiles), 404

### `POST /api/v1/iso50001/{building_id}/clauses/{clause}/files`

Upload one evidence file (multipart field `file`; size and type checked, bytes sniffed). `iso50001.files.upload`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | path | yes | `UuidUUID` |
| `clause` | path | yes | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 201 [`ISOFile`](#schema-isofile), 404, 409, 422

### `GET /api/v1/iso50001/{building_id}/clauses/{clause}/notes`

A sub-clause's notes, newest first. `iso50001.notes`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | path | yes | `UuidUUID` |
| `clause` | path | yes | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ISONotes`](#schema-isonotes), 404

### `POST /api/v1/iso50001/{building_id}/clauses/{clause}/notes`

Add a note. `iso50001.notes.create`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | path | yes | `UuidUUID` |
| `clause` | path | yes | string |
| `company_id` | query |  | `UuidUUID` |

Body: [`ISONoteCreateRequest`](#schema-isonotecreaterequest)

Responses: 201 [`ISONote`](#schema-isonote), 404, 409, 422

### `PUT /api/v1/iso50001/{building_id}/dates`

Set every main clause's start and end dates. `iso50001.dates`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`ISODatesRequest`](#schema-isodatesrequest)

Responses: 200 [`ISOProject`](#schema-isoproject), 404, 409, 422

### `GET /api/v1/iso50001/{building_id}/export`

Every note and file as a zip, streamed. `iso50001.export`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200, 404

## jobs

### `GET /api/v1/jobs/{id}`

The state of a background job this user started (R192). `jobs.get`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Job`](#schema-job), 404

## load-profile

### `GET /api/v1/load-profile`

Averaged 24-hour profiles. `load_profile.profiles`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query | yes | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `profiles` | query |  | string[] |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`LoadProfiles`](#schema-loadprofiles), 404

### `GET /api/v1/load-profile/export`

The hourly matrix as XLSX. `load_profile.export`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query | yes | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `profiles` | query |  | string[] |
| `company_id` | query |  | `UuidUUID` |

Responses: 200, 404

### `GET /api/v1/load-profile/statistics`

Per-profile statistics. `load_profile.statistics`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query | yes | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `profiles` | query |  | string[] |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`LoadProfileStatistics`](#schema-loadprofilestatistics), 404

## messages

### `GET /api/v1/job-runs`

Job execution history with counts and errors. `jobRuns.list`

| parameter | in | required | type |
|---|---|---|---|
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `job_type` | query |  | string? |
| `status` | query |  | string: running \| success \| partial \| failed? |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`JobRunPage`](#schema-jobrunpage), 404

### `POST /api/v1/job-runs/{type}/trigger`

Trigger an allow-listed job for the caller's company. `jobRuns.trigger`

| parameter | in | required | type |
|---|---|---|---|
| `type` | path | yes | string: alarm.evaluate \| billing.dispatch \| integration.sync_dispatch \| epias.sync_prices \| report.dispatch_monthly \| report.dispatch_yearly \| isolar.dispatch_sync \| isolar.fetch_alarms \| carbon.daily_accrual \| forecast.run |
| `company_id` | query |  | `UuidUUID` |

Responses: 202 [`JobAccepted`](#schema-jobaccepted), 404, 409, 422

### `GET /api/v1/messages`

Operational messages for the tenant. `messages.list`

| parameter | in | required | type |
|---|---|---|---|
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `kind` | query |  | string: alarm \| job \| system? |
| `status` | query |  | string: success \| error \| warning \| info? |
| `q` | query |  | string |
| `from` | query |  | string (date-time)? |
| `to` | query |  | string (date-time)? |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`MessagePage`](#schema-messagepage), 404

## mobile

### `POST /api/v1/mobile/auth/login`

Mobile sign-in; returns bearer and refresh tokens. *public* · `mobile.login`

Body: [`MobileLoginRequest`](#schema-mobileloginrequest)

Responses: 200 [`MobileTokens`](#schema-mobiletokens), 409, 422

### `GET /api/v1/mobile/auth/me`

The current principal for the mobile app. `mobile.me`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Me`](#schema-me), 404

### `POST /api/v1/mobile/auth/refresh`

Rotate mobile tokens. *public* · `mobile.refresh`

Body: [`MobileRefreshRequest`](#schema-mobilerefreshrequest)

Responses: 200 [`MobileTokens`](#schema-mobiletokens), 409, 422

## power-plants

### `PUT /api/v1/plants/{id}/alarm-recipients`

Set the plant's alarm forwarding recipients. `plants.alarm_recipients`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`AlarmRecipientsRequest`](#schema-alarmrecipientsrequest)

Responses: 200 [`AlarmRecipients`](#schema-alarmrecipients), 404, 409, 422

### `GET /api/v1/plants/{id}/alarms`

iSolar fault alarms, newest first, translated. `plants.alarms`

| parameter | in | required | type |
|---|---|---|---|
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`PlantFaultPage`](#schema-plantfaultpage), 404

### `GET /api/v1/plants/{id}/devices`

Devices with status, power and last update; q searches name and serial. `plants.devices`

| parameter | in | required | type |
|---|---|---|---|
| `q` | query |  | string |
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`PlantDevices`](#schema-plantdevices), 404

### `POST /api/v1/plants/{id}/isolar-link`

Bind a plant to an iSolarCloud plant; imports it and backfills. `plants.isolar_link`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`ISolarLinkRequest`](#schema-isolarlinkrequest)

Responses: 200 [`ISolarLinked`](#schema-isolarlinked), 404, 409, 422

### `DELETE /api/v1/plants/{id}/isolar-link`

Unlink; stored production stays. `plants.isolar_unlink`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `GET /api/v1/plants/{id}/production`

Production series by hour, day or month. `plants.production`

| parameter | in | required | type |
|---|---|---|---|
| `granularity` | query | yes | string: hour \| day \| month |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`PlantProductionSeries`](#schema-plantproductionseries), 404

### `GET /api/v1/plants/{id}/production/export`

The production series as a workbook. `plants.production.export`

| parameter | in | required | type |
|---|---|---|---|
| `granularity` | query | yes | string: hour \| day \| month |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200, 404

### `GET /api/v1/plants/{id}/realtime`

Current power, yields and capacity utilisation from the inverters' last snapshots. `plants.realtime`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`PlantRealtime`](#schema-plantrealtime), 404

### `GET /api/v1/plants/{id}/revenue`

Daily, monthly, yearly and total revenue at the feed-in tariff effective each day. `plants.revenue`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`PlantRevenue`](#schema-plantrevenue), 404

### `POST /api/v1/plants/{id}/sync`

Enqueue an iSolar sync; returns the job to watch. `plants.sync`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 202 [`JobAccepted`](#schema-jobaccepted), 404, 409, 422

### `GET /api/v1/power-plants`

Power plants of the company in scope. `plants.list`

| parameter | in | required | type |
|---|---|---|---|
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`PlantPage`](#schema-plantpage), 404

### `POST /api/v1/power-plants`

Create a power plant. `plants.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`PlantCreateRequest`](#schema-plantcreaterequest)

Responses: 201 [`PlantDetail`](#schema-plantdetail), 404, 409, 422

### `GET /api/v1/power-plants/{id}`

A power plant with devices, monthly targets and alarm recipients. `plants.get`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`PlantDetail`](#schema-plantdetail), 404

### `PATCH /api/v1/power-plants/{id}`

Update a power plant. `plants.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`PlantUpdateRequest`](#schema-plantupdaterequest)

Responses: 200 [`PlantDetail`](#schema-plantdetail), 404, 409, 422

### `DELETE /api/v1/power-plants/{id}`

Soft-delete a power plant. `plants.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

## profile

### `GET /api/v1/profile`

The current user's profile. `profile.get`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Me`](#schema-me), 404

### `PATCH /api/v1/profile`

Update the current user's name, e-mail, phone, locale and UI preferences. `profile.update`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`ProfileUpdateRequest`](#schema-profileupdaterequest)

Responses: 200 [`Me`](#schema-me), 404, 409, 422

## public

### `POST /api/v1/public/bill-calculator`

Estimate a bill from the national tariff schedule, power charge included. *public* · `public.bill_calculator`

Body: [`PublicBillRequest`](#schema-publicbillrequest)

Responses: 200 [`PublicBill`](#schema-publicbill), 409, 422

### `POST /api/v1/public/contact`

Contact form; e-mailed to the operator. *public* · `public.contact`

Body: [`PublicContactRequest`](#schema-publiccontactrequest)

Responses: 202, 409, 422

### `POST /api/v1/public/demo-request`

Demo request form; e-mailed to the operator. *public* · `public.demo_request`

Body: [`PublicDemoRequest`](#schema-publicdemorequest)

Responses: 202, 409, 422

## renewable

### `GET /api/v1/renewable/analytics`

Peaks, trend, data availability and the financial gains from bills. `renewable.analytics`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`RenewableAnalytics`](#schema-renewableanalytics), 404

### `GET /api/v1/renewable/efficiency`

Efficiency figures; each states why it is unavailable. `renewable.efficiency`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`RenewableEfficiency`](#schema-renewableefficiency), 404

### `GET /api/v1/renewable/environmental`

CO₂ avoided at the grid factor and seeded equivalences with their sources. `renewable.environmental`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`RenewableEnvironmental`](#schema-renewableenvironmental), 404

### `GET /api/v1/renewable/forecast`

Consumption forecast sums and accuracy from stored forecast runs. `renewable.forecast`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`RenewableForecast`](#schema-renewableforecast), 404

### `GET /api/v1/renewable/grid-interaction`

Today's import and export, power factor and the latest bill's prices. `renewable.grid_interaction`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`RenewableGridInteraction`](#schema-renewablegridinteraction), 404

### `GET /api/v1/renewable/overview`

Generation totals over the range from the export registers. `renewable.overview`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`RenewableOverview`](#schema-renewableoverview), 404

### `GET /api/v1/renewable/realtime`

The last complete hour's generation and the last 24 hours. `renewable.realtime`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`RenewableRealtime`](#schema-renewablerealtime), 404

### `GET /api/v1/renewable/system-status`

Monitoring and grid connection health from the last reading. `renewable.system_status`

| parameter | in | required | type |
|---|---|---|---|
| `analyzer_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `from` | query | yes | [`Date`](#schema-date) |
| `to` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`RenewableSystemStatus`](#schema-renewablesystemstatus), 404

### `GET /api/v1/weather`

Current conditions and seven days for a plant's or building's coordinates; never a default city. `weather.get`

| parameter | in | required | type |
|---|---|---|---|
| `plant_id` | query |  | `UuidUUID` |
| `building_id` | query |  | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Weather`](#schema-weather), 404

## reports

### `GET /api/v1/reports`

The report archive, newest first, with the whole match's count. `reports.list`

| parameter | in | required | type |
|---|---|---|---|
| `type` | query |  | string: monthly \| yearly? |
| `building_id` | query |  | `UuidUUID` |
| `year` | query |  | integer? |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ReportPage`](#schema-reportpage), 404

### `POST /api/v1/reports/generate`

Generate one report per building; returns each building's job. `reports.generate`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`ReportGenerateRequest`](#schema-reportgeneraterequest)

Responses: 202 [`ReportGenerateAccepted`](#schema-reportgenerateaccepted), 404, 409, 422

### `GET /api/v1/reports/preview`

Compute report figures for buildings and a period without persisting them. `reports.preview`

| parameter | in | required | type |
|---|---|---|---|
| `type` | query | yes | string: monthly \| yearly |
| `period` | query | yes | string |
| `plant_selection` | query | yes | string: all \| grid \| rooftop |
| `building_ids` | query | yes | `UuidUUID`[] |
| `plant_ids` | query |  | `UuidUUID`[] |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`ReportPayload`](#schema-reportpayload), 404

### `GET /api/v1/reports/{id}`

A stored report with its figures. `reports.get`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Report`](#schema-report), 404

### `POST /api/v1/reports/{id}/email`

E-mail the report with its PDF and workbook attached. `reports.email`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`ReportEmailRequest`](#schema-reportemailrequest)

Responses: 202 [`ReportEmailAccepted`](#schema-reportemailaccepted), 404, 409, 422

### `GET /api/v1/reports/{id}/excel`

The report as a workbook, in the reader's language. `reports.excel`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200, 404

### `GET /api/v1/reports/{id}/pdf`

The report as PDF, in the reader's language. `reports.pdf`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200, 404

## smtp

### `GET /api/v1/smtp-settings`

SMTP settings of the company in scope; the password is never returned. `smtp.get`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`SMTPSettings`](#schema-smtpsettings), 404

### `PUT /api/v1/smtp-settings`

Create or replace SMTP settings. `smtp.upsert`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`SMTPPutRequest`](#schema-smtpputrequest)

Responses: 200 [`SMTPSettings`](#schema-smtpsettings), 404, 409, 422

### `POST /api/v1/smtp-settings/test`

Send a test message with the stored settings. `smtp.test`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`SMTPTestRequest`](#schema-smtptestrequest)

Responses: 204, 404, 409, 422

## system

### `GET /api/v1/openapi.json`

The OpenAPI 3.1 document of this API. *public* · `system.openapi`

Responses: 200 string

## tariffs

### `POST /api/v1/buildings/bulk-tariff`

Assign one tariff definition to many buildings. `bulk_tariff.assign`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`BulkTariffRequest`](#schema-bulktariffrequest)

Responses: 200 [`BulkTariffResult`](#schema-bulktariffresult), 404, 409, 422

### `GET /api/v1/buildings/bulk-tariff/current`

Every building's tariff in force today. `bulk_tariff.current`

| parameter | in | required | type |
|---|---|---|---|
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`BuildingTariffStatePage`](#schema-buildingtariffstatepage), 404

### `GET /api/v1/buildings/bulk-tariff/history`

Bulk assignment history, newest first. `bulk_tariff.history`

| parameter | in | required | type |
|---|---|---|---|
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`BulkTariffAssignmentPage`](#schema-bulktariffassignmentpage), 404

### `POST /api/v1/icmal-imports`

Upload and analyse an icmal; writes no tariff. `icmal.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Responses: 201 [`IcmalImport`](#schema-icmalimport), 404, 409, 422

### `GET /api/v1/icmal-imports/{id}`

A stored icmal analysis. `icmal.get`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`IcmalImport`](#schema-icmalimport), 404

### `POST /api/v1/icmal-imports/{id}/apply`

Write the confirmed coefficients into a new tariff version per building. `icmal.apply`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`IcmalApplyRequest`](#schema-icmalapplyrequest)

Responses: 200 [`BulkTariffResult`](#schema-bulktariffresult), 404, 409, 422

### `GET /api/v1/national-tariff-schedule`

The published national tariff schedule. *public* · `national_tariff.list`

| parameter | in | required | type |
|---|---|---|---|
| `user_group` | query |  | string |
| `voltage_level` | query |  | string |
| `term` | query |  | string |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |

Responses: 200 [`NationalTariffPage`](#schema-nationaltariffpage)

### `POST /api/v1/national-tariff-schedule`

Publish a default tariff row, keyed by date, user group, voltage level and term. `national_tariff.upsert`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`NationalTariffFields`](#schema-nationaltarifffields)

Responses: 204, 404, 409, 422

### `DELETE /api/v1/national-tariff-schedule/{id}`

Remove a published default tariff row. `national_tariff.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `GET /api/v1/solar-tariffs`

A plant's feed-in tariff history. `solar_tariffs.list`

| parameter | in | required | type |
|---|---|---|---|
| `plant_id` | query | yes | `UuidUUID` |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`SolarTariffPage`](#schema-solartariffpage), 404

### `POST /api/v1/solar-tariffs`

Add a feed-in tariff to a plant. `solar_tariffs.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`SolarTariffFields`](#schema-solartarifffields)

Responses: 201 [`SolarTariff`](#schema-solartariff), 404, 409, 422

### `DELETE /api/v1/solar-tariffs/{id}`

Soft-delete a feed-in tariff. `solar_tariffs.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `GET /api/v1/tariff-templates`

The company's tariff templates. `tariff_templates.list`

| parameter | in | required | type |
|---|---|---|---|
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`TariffTemplatePage`](#schema-tarifftemplatepage), 404

### `POST /api/v1/tariff-templates`

Create a tariff template. `tariff_templates.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`TariffTemplateFields`](#schema-tarifftemplatefields)

Responses: 201 [`TariffTemplate`](#schema-tarifftemplate), 404, 409, 422

### `PATCH /api/v1/tariff-templates/{id}`

Replace a tariff template. `tariff_templates.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`TariffTemplateUpdateRequest`](#schema-tarifftemplateupdaterequest)

Responses: 200 [`TariffTemplate`](#schema-tarifftemplate), 404, 409, 422

### `DELETE /api/v1/tariff-templates/{id}`

Delete a tariff template. `tariff_templates.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

### `POST /api/v1/tariff-templates/{id}/apply`

Apply a template to a set of buildings. `tariff_templates.apply`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`TariffTemplateApplyRequest`](#schema-tarifftemplateapplyrequest)

Responses: 200 [`BulkTariffResult`](#schema-bulktariffresult), 404, 409, 422

### `GET /api/v1/tariffs`

Tariff history. `tariffs.list`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | query |  | `UuidUUID` |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`TariffSummaryItemPage`](#schema-tariffsummaryitempage), 404

### `POST /api/v1/tariffs`

Create a tariff version with taxes, extra charges and manual YEKDEM. `tariffs.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`TariffFields`](#schema-tarifffields)

Responses: 201 [`Tariff`](#schema-tariff), 404, 409, 422

### `GET /api/v1/tariffs/applicable`

The tariff in force for a building on a date. `tariffs.applicable`

| parameter | in | required | type |
|---|---|---|---|
| `building_id` | query | yes | `UuidUUID` |
| `date` | query | yes | [`Date`](#schema-date) |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Tariff`](#schema-tariff), 404

### `GET /api/v1/tariffs/{id}`

A tariff version. `tariffs.get`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`Tariff`](#schema-tariff), 404

### `PATCH /api/v1/tariffs/{id}`

Replace a tariff version. `tariffs.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`TariffUpdateRequest`](#schema-tariffupdaterequest)

Responses: 200 [`Tariff`](#schema-tariff), 404, 409, 422

### `DELETE /api/v1/tariffs/{id}`

Soft-delete a tariff version. `tariffs.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

## users

### `GET /api/v1/users`

Users of the company in scope. `users.list`

| parameter | in | required | type |
|---|---|---|---|
| `role` | query |  | [`Role`](#schema-role)[] |
| `is_active` | query |  | boolean? |
| `q` | query |  | string |
| `limit` | query |  | integer? |
| `cursor` | query |  | string |
| `company_id` | query |  | `UuidUUID` |

Responses: 200 [`UserPage`](#schema-userpage), 404

### `POST /api/v1/users`

Create a user; role options depend on the caller's role. `users.create`

| parameter | in | required | type |
|---|---|---|---|
| `company_id` | query |  | `UuidUUID` |

Body: [`UserCreateRequest`](#schema-usercreaterequest)

Responses: 201 [`User`](#schema-user), 404, 409, 422

### `PATCH /api/v1/users/{id}`

Update a user; role, status and e-mail changes end their sessions. `users.update`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Body: [`UserUpdateRequest`](#schema-userupdaterequest)

Responses: 200 [`User`](#schema-user), 404, 409, 422

### `DELETE /api/v1/users/{id}`

Soft-delete a user. `users.delete`

| parameter | in | required | type |
|---|---|---|---|
| `id` | path | yes | `UuidUUID` |
| `company_id` | query |  | `UuidUUID` |

Responses: 204, 404, 409, 422

## Schemas

### <a id="schema-alarm"></a>`Alarm`

| field | type | required |
|---|---|---|
| `analyzers` | [`AlarmAnalyzer`](#schema-alarmanalyzer)[] | yes |
| `channels` | [`AlarmChannel`](#schema-alarmchannel)[] | yes |
| `created_at` | string (date-time) | yes |
| `id` | `UuidUUID` | yes |
| `is_enabled` | boolean | yes |
| `name` | string | yes |
| `notification_frequency_unit` | string? |  |
| `notification_frequency_value` | integer? |  |
| `settings` | [`AlarmSettings`](#schema-alarmsettings) | yes |
| `type` | string: reactive_limit \| data_communication \| current_voltage_power \| invoice_increase | yes |
| `updated_at` | string (date-time) | yes |

### <a id="schema-alarmanalyzer"></a>`AlarmAnalyzer`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` |  |
| `id` | `UuidUUID` | yes |
| `installation_number` | string | yes |

### <a id="schema-alarmchannel"></a>`AlarmChannel`

| field | type | required |
|---|---|---|
| `channel` | string: email \| sms | yes |
| `target` | string | yes |

### <a id="schema-alarmevaluation"></a>`AlarmEvaluation`

| field | type | required |
|---|---|---|
| `analyzers` | [`AlarmEvaluationAnalyzer`](#schema-alarmevaluationanalyzer)[] | yes |
| `dry_run` | boolean | yes |
| `evaluated_at` | string (date-time) | yes |
| `notifications_sent` | integer | yes |

### <a id="schema-alarmevaluationanalyzer"></a>`AlarmEvaluationAnalyzer`

| field | type | required |
|---|---|---|
| `analyzer_id` | `UuidUUID` | yes |
| `breaches` | [`AlarmEvaluationBreach`](#schema-alarmevaluationbreach)[] | yes |
| `fired` | boolean | yes |
| `installation_number` | string | yes |
| `no_verdict` | string[] | yes |

### <a id="schema-alarmevaluationbreach"></a>`AlarmEvaluationBreach`

| field | type | required |
|---|---|---|
| `field` | string | yes |
| `measured` | [`Decimal`](#schema-decimal) | yes |
| `message` | string | yes |
| `threshold` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-alarmevent"></a>`AlarmEvent`

| field | type | required |
|---|---|---|
| `analyzer_id` | `UuidUUID` |  |
| `detail` | any |  |
| `id` | `UuidUUID` | yes |
| `message` | string | yes |
| `notification_error` | string? |  |
| `notified_at` | string (date-time)? |  |
| `triggered_at` | string (date-time) | yes |

### <a id="schema-alarmeventpage"></a>`AlarmEventPage`

| field | type | required |
|---|---|---|
| `items` | [`AlarmEvent`](#schema-alarmevent)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-alarmfields"></a>`AlarmFields`

| field | type | required |
|---|---|---|
| `analyzer_ids` | `UuidUUID`[] | yes |
| `channels` | [`AlarmChannel`](#schema-alarmchannel)[] |  |
| `is_enabled` | boolean |  |
| `name` | string | yes |
| `notification_frequency_unit` | string: hours \| days? |  |
| `notification_frequency_value` | integer? |  |
| `settings` | [`AlarmSettings`](#schema-alarmsettings) |  |
| `type` | string: reactive_limit \| data_communication \| current_voltage_power \| invoice_increase | yes |

### <a id="schema-alarmpage"></a>`AlarmPage`

| field | type | required |
|---|---|---|
| `items` | [`Alarm`](#schema-alarm)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-alarmrecipients"></a>`AlarmRecipients`

| field | type | required |
|---|---|---|
| `emails` | string[] | yes |

### <a id="schema-alarmrecipientsrequest"></a>`AlarmRecipientsRequest`

| field | type | required |
|---|---|---|
| `emails` | string[] | yes |

### <a id="schema-alarmsettings"></a>`AlarmSettings`

| field | type | required |
|---|---|---|
| `active_consumption_max` | [`Decimal`](#schema-decimal) |  |
| `active_consumption_max_period_unit` | string: hours \| days? |  |
| `active_consumption_max_period_value` | integer? |  |
| `active_consumption_min` | [`Decimal`](#schema-decimal) |  |
| `active_consumption_min_period_unit` | string: hours \| days? |  |
| `active_consumption_min_period_value` | integer? |  |
| `capacitive_period_unit` | string: hours \| days? |  |
| `capacitive_period_value` | integer? |  |
| `capacitive_ratio_threshold` | [`Decimal`](#schema-decimal) |  |
| `communication_threshold_hours` | integer? |  |
| `inductive_period_unit` | string: hours \| days? |  |
| `inductive_period_value` | integer? |  |
| `inductive_ratio_threshold` | [`Decimal`](#schema-decimal) |  |
| `invoice_threshold_pct` | [`Decimal`](#schema-decimal) |  |
| `power_max` | [`Decimal`](#schema-decimal) |  |
| `power_min` | [`Decimal`](#schema-decimal) |  |
| `voltage_max` | [`Decimal`](#schema-decimal) |  |
| `voltage_min` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-alarmupdaterequest"></a>`AlarmUpdateRequest`

| field | type | required |
|---|---|---|
| `analyzer_ids` | `UuidUUID`[] | yes |
| `channels` | [`AlarmChannel`](#schema-alarmchannel)[] |  |
| `is_enabled` | boolean |  |
| `name` | string | yes |
| `notification_frequency_unit` | string: hours \| days? |  |
| `notification_frequency_value` | integer? |  |
| `settings` | [`AlarmSettings`](#schema-alarmsettings) |  |
| `type` | string: reactive_limit \| data_communication \| current_voltage_power \| invoice_increase | yes |

### <a id="schema-analyzer"></a>`Analyzer`

| field | type | required |
|---|---|---|
| `activity_status` | string: active \| passive | yes |
| `address` | string? |  |
| `building_id` | `UuidUUID` |  |
| `customer_name` | string? |  |
| `district` | string? |  |
| `id` | `UuidUUID` | yes |
| `installation_number` | string | yes |
| `installed_power_kw` | [`Decimal`](#schema-decimal) |  |
| `is_active` | boolean | yes |
| `last_reading_at` | string (date-time)? |  |
| `latitude` | [`Decimal`](#schema-decimal) |  |
| `longitude` | [`Decimal`](#schema-decimal) |  |
| `meter_model` | string? |  |
| `meter_multiplier` | [`Decimal`](#schema-decimal) | yes |
| `meter_number` | string? |  |
| `provider` | string: osos \| gridbox \| aril \| pm5340 \| isolar | yes |
| `provider_subtype` | string | yes |
| `province` | string? |  |
| `tariff_type` | string? |  |

### <a id="schema-analyzercount"></a>`AnalyzerCount`

| field | type | required |
|---|---|---|
| `count` | integer | yes |
| `provider` | string | yes |
| `subtype` | string | yes |

### <a id="schema-analyzerpage"></a>`AnalyzerPage`

| field | type | required |
|---|---|---|
| `items` | [`Analyzer`](#schema-analyzer)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-analyzerrefreshrequest"></a>`AnalyzerRefreshRequest`

| field | type | required |
|---|---|---|
| `mode` | string: hourly \| energy | yes |

### <a id="schema-analyzerupdaterequest"></a>`AnalyzerUpdateRequest`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` |  |
| `installed_power_kw` | [`Decimal`](#schema-decimal) |  |
| `latitude` | [`Decimal`](#schema-decimal) |  |
| `longitude` | [`Decimal`](#schema-decimal) |  |
| `meter_multiplier` | [`Decimal`](#schema-decimal) |  |
| `unassign_building` | boolean |  |

### <a id="schema-anomaly"></a>`Anomaly`

| field | type | required |
|---|---|---|
| `analyzer_id` | `UuidUUID` | yes |
| `created_at` | string (date-time) | yes |
| `detail` | any |  |
| `id` | `UuidUUID` | yes |
| `period_end` | string (date-time) | yes |
| `period_start` | string (date-time) | yes |
| `reason` | string | yes |
| `resolution` | string? |  |
| `resolved_at` | string (date-time)? |  |
| `resolved_by` | `UuidUUID` |  |

### <a id="schema-anomalycheck"></a>`AnomalyCheck`

| field | type | required |
|---|---|---|
| `actual` | [`Decimal`](#schema-decimal) |  |
| `available` | boolean | yes |
| `expected` | [`Decimal`](#schema-decimal) |  |
| `is_anomaly` | boolean? |  |
| `lower` | [`Decimal`](#schema-decimal) |  |
| `method` | string |  |
| `model_id` | string |  |
| `model_version` | string |  |
| `reason` | string |  |
| `score` | number? |  |
| `upper` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-anomalycheckrequest"></a>`AnomalyCheckRequest`

| field | type | required |
|---|---|---|
| `actual` | [`Decimal`](#schema-decimal) |  |
| `analyzer_id` | `UuidUUID` | yes |
| `ts` | string (date-time) | yes |

### <a id="schema-anomalypage"></a>`AnomalyPage`

| field | type | required |
|---|---|---|
| `items` | [`Anomaly`](#schema-anomaly)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-anomalyresolverequest"></a>`AnomalyResolveRequest`

| field | type | required |
|---|---|---|
| `mode` | string: reset_registered \| manual_override \| accepted | yes |
| `overrides` | object |  |
| `reset_after` | object |  |
| `reset_ts` | string (date-time)? |  |

### <a id="schema-authorizeurl"></a>`AuthorizeURL`

| field | type | required |
|---|---|---|
| `url` | string | yes |

### <a id="schema-backfillrequest"></a>`BackfillRequest`

| field | type | required |
|---|---|---|
| `analyzer_ids` | `UuidUUID`[] |  |
| `force` | boolean |  |
| `from` | string (date-time) | yes |
| `kinds` | string[] |  |
| `to` | string (date-time) | yes |

### <a id="schema-balancerow"></a>`BalanceRow`

| field | type | required |
|---|---|---|
| `consumption` | [`Decimal`](#schema-decimal) |  |
| `generation` | [`Decimal`](#schema-decimal) |  |
| `grid_export` | [`Decimal`](#schema-decimal) |  |
| `grid_import` | [`Decimal`](#schema-decimal) |  |
| `period_end` | string (date-time) | yes |
| `period_start` | string (date-time) | yes |

### <a id="schema-bill"></a>`Bill`

| field | type | required |
|---|---|---|
| `active_import` | [`Decimal`](#schema-decimal) | yes |
| `analyzer_id` | `UuidUUID` |  |
| `building_id` | `UuidUUID` |  |
| `capacitive_kvarh` | [`Decimal`](#schema-decimal) | yes |
| `capacitive_ratio` | [`Decimal`](#schema-decimal) |  |
| `computed_at` | string (date-time) | yes |
| `currency` | string | yes |
| `days_in_period` | integer (int32) | yes |
| `distribution_cost` | [`Decimal`](#schema-decimal) | yes |
| `energy_cost` | [`Decimal`](#schema-decimal) | yes |
| `flag_reason` | string? |  |
| `has_pdf` | boolean | yes |
| `id` | `UuidUUID` | yes |
| `inductive_kvarh` | [`Decimal`](#schema-decimal) | yes |
| `inductive_ratio` | [`Decimal`](#schema-decimal) |  |
| `max_demand_kw` | [`Decimal`](#schema-decimal) |  |
| `net_consumption` | [`Decimal`](#schema-decimal) | yes |
| `period_end` | string (date-time) | yes |
| `period_key` | string | yes |
| `period_start` | string (date-time) | yes |
| `power_cost` | [`Decimal`](#schema-decimal) | yes |
| `ptf_yekdem_used` | boolean | yes |
| `reactive_penalty` | [`Decimal`](#schema-decimal) | yes |
| `reactive_penalty_applied` | boolean | yes |
| `scope` | string: analyzer \| building \| company | yes |
| `status` | string: draft \| issued \| flagged \| superseded | yes |
| `tariff_id` | `UuidUUID` |  |
| `total_cost` | [`Decimal`](#schema-decimal) | yes |
| `vat_cost` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-billcomputeaccepted"></a>`BillComputeAccepted`

| field | type | required |
|---|---|---|
| `job_ids` | string[] | yes |

### <a id="schema-billcomputerequest"></a>`BillComputeRequest`

| field | type | required |
|---|---|---|
| `analyzer_ids` | `UuidUUID`[] |  |
| `building_ids` | `UuidUUID`[] |  |
| `force` | boolean |  |
| `period` | string | yes |
| `scope` | string: analyzer \| building \| company | yes |

### <a id="schema-billdashboard"></a>`BillDashboard`

| field | type | required |
|---|---|---|
| `buildings` | [`BillDashboardBuilding`](#schema-billdashboardbuilding)[] | yes |
| `netting` | [`BillDashboardNetting`](#schema-billdashboardnetting)[] | yes |
| `period` | string | yes |
| `plants` | [`BillDashboardPlants`](#schema-billdashboardplants) | yes |

### <a id="schema-billdashboardbuilding"></a>`BillDashboardBuilding`

| field | type | required |
|---|---|---|
| `building_bill` | [`BillDashboardRow`](#schema-billdashboardrow) |  |
| `building_id` | `UuidUUID` | yes |
| `building_name` | string | yes |
| `currency` | string: TRY \| USD \| EUR | yes |
| `diverges_from_rows` | boolean | yes |
| `rows` | [`BillDashboardRow`](#schema-billdashboardrow)[] | yes |
| `total_consumption` | [`Decimal`](#schema-decimal) | yes |
| `total_invoice` | [`Decimal`](#schema-decimal) | yes |
| `total_production` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-billdashboardnetting"></a>`BillDashboardNetting`

| field | type | required |
|---|---|---|
| `company_bill_id` | `UuidUUID` |  |
| `currency` | string: TRY \| USD \| EUR | yes |
| `efficiency_pct` | [`Decimal`](#schema-decimal) |  |
| `net` | [`Decimal`](#schema-decimal) | yes |
| `net_status` | string: net_consumption \| net_production | yes |
| `period_key` | string | yes |
| `total_consumption` | [`Decimal`](#schema-decimal) | yes |
| `total_invoice` | [`Decimal`](#schema-decimal) | yes |
| `total_production` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-billdashboardplantrow"></a>`BillDashboardPlantRow`

| field | type | required |
|---|---|---|
| `analyzer_name` | string? |  |
| `bill_id` | `UuidUUID` |  |
| `consumption_price` | [`Decimal`](#schema-decimal) |  |
| `currency` | string: TRY \| USD \| EUR? |  |
| `installation_number` | string? |  |
| `invoice_amount` | [`Decimal`](#schema-decimal) |  |
| `plant_id` | `UuidUUID` | yes |
| `plant_name` | string | yes |
| `production_kwh` | [`Decimal`](#schema-decimal) |  |
| `production_price` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-billdashboardplants"></a>`BillDashboardPlants`

| field | type | required |
|---|---|---|
| `available` | boolean | yes |
| `reason` | string: plants_not_in_scope |  |
| `rows` | [`BillDashboardPlantRow`](#schema-billdashboardplantrow)[] | yes |
| `total_invoice` | [`MoneyAmount`](#schema-moneyamount)[] | yes |
| `total_production_kwh` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-billdashboardrow"></a>`BillDashboardRow`

| field | type | required |
|---|---|---|
| `analyzer_id` | `UuidUUID` |  |
| `analyzer_name` | string | yes |
| `bill_id` | `UuidUUID` | yes |
| `building_id` | `UuidUUID` |  |
| `building_name` | string | yes |
| `consumption` | [`Decimal`](#schema-decimal) | yes |
| `consumption_price` | [`Decimal`](#schema-decimal) |  |
| `currency` | string: TRY \| USD \| EUR | yes |
| `etso_code` | string | yes |
| `installation_number` | string | yes |
| `invoice` | [`Decimal`](#schema-decimal) | yes |
| `period_key` | string | yes |
| `production` | [`Decimal`](#schema-decimal) | yes |
| `production_price` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-billdetail"></a>`BillDetail`

| field | type | required |
|---|---|---|
| `active_import` | [`Decimal`](#schema-decimal) | yes |
| `analyzer_id` | `UuidUUID` |  |
| `building_id` | `UuidUUID` |  |
| `capacitive_kvarh` | [`Decimal`](#schema-decimal) | yes |
| `capacitive_ratio` | [`Decimal`](#schema-decimal) |  |
| `computed_at` | string (date-time) | yes |
| `currency` | string | yes |
| `days_in_period` | integer (int32) | yes |
| `distribution_cost` | [`Decimal`](#schema-decimal) | yes |
| `energy_cost` | [`Decimal`](#schema-decimal) | yes |
| `flag_reason` | string? |  |
| `has_pdf` | boolean | yes |
| `id` | `UuidUUID` | yes |
| `inductive_kvarh` | [`Decimal`](#schema-decimal) | yes |
| `inductive_ratio` | [`Decimal`](#schema-decimal) |  |
| `lines` | [`BillLine`](#schema-billline)[] | yes |
| `max_demand_kw` | [`Decimal`](#schema-decimal) |  |
| `member_analyzer_ids` | `UuidUUID`[] | yes |
| `net_consumption` | [`Decimal`](#schema-decimal) | yes |
| `period_end` | string (date-time) | yes |
| `period_key` | string | yes |
| `period_start` | string (date-time) | yes |
| `power_cost` | [`Decimal`](#schema-decimal) | yes |
| `ptf_yekdem_used` | boolean | yes |
| `reactive_penalty` | [`Decimal`](#schema-decimal) | yes |
| `reactive_penalty_applied` | boolean | yes |
| `scope` | string: analyzer \| building \| company | yes |
| `status` | string: draft \| issued \| flagged \| superseded | yes |
| `tariff_id` | `UuidUUID` |  |
| `total_cost` | [`Decimal`](#schema-decimal) | yes |
| `vat_cost` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-billhour"></a>`BillHour`

| field | type | required |
|---|---|---|
| `consumption` | [`Decimal`](#schema-decimal) | yes |
| `cost` | [`Decimal`](#schema-decimal) | yes |
| `kbk` | [`Decimal`](#schema-decimal) | yes |
| `ptf` | [`Decimal`](#schema-decimal) | yes |
| `ts` | string (date-time) | yes |
| `unit_price` | [`Decimal`](#schema-decimal) | yes |
| `yekdem` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-billhours"></a>`BillHours`

| field | type | required |
|---|---|---|
| `items` | [`BillHour`](#schema-billhour)[] | yes |

### <a id="schema-billline"></a>`BillLine`

| field | type | required |
|---|---|---|
| `amount` | [`Decimal`](#schema-decimal) | yes |
| `code` | string | yes |
| `label` | string | yes |
| `quantity` | [`Decimal`](#schema-decimal) |  |
| `rate_pct` | [`Decimal`](#schema-decimal) |  |
| `unit` | string? |  |
| `unit_price` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-billpage"></a>`BillPage`

| field | type | required |
|---|---|---|
| `items` | [`Bill`](#schema-bill)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-building"></a>`Building`

| field | type | required |
|---|---|---|
| `active_analyzer_count` | integer? |  |
| `activity_status` | string: active \| passive? |  |
| `address` | string? |  |
| `analyzer_count` | integer? |  |
| `bill_cutoff_day` | integer | yes |
| `created_at` | string (date-time) | yes |
| `floors` | integer? |  |
| `id` | `UuidUUID` | yes |
| `latitude` | [`Decimal`](#schema-decimal) |  |
| `longitude` | [`Decimal`](#schema-decimal) |  |
| `name` | string | yes |
| `personnel_count` | integer? |  |
| `responsible_user_id` | `UuidUUID` |  |
| `sector` | string? |  |
| `total_area_m2` | [`Decimal`](#schema-decimal) |  |
| `updated_at` | string (date-time) | yes |

### <a id="schema-buildingcomparison"></a>`BuildingComparison`

| field | type | required |
|---|---|---|
| `available` | boolean | yes |
| `co2_emission_kg` | [`ComparisonMetric`](#schema-comparisonmetric) | yes |
| `consumption_per_area` | [`ComparisonMetric`](#schema-comparisonmetric) | yes |
| `consumption_per_capita` | [`ComparisonMetric`](#schema-comparisonmetric) | yes |
| `daily_consumption` | [`ComparisonMetric`](#schema-comparisonmetric) | yes |
| `monthly_consumption` | [`ComparisonMetric`](#schema-comparisonmetric) | yes |
| `peers` | integer | yes |
| `reason` | string: sector_too_small? |  |
| `sector` | string | yes |

### <a id="schema-buildingcreaterequest"></a>`BuildingCreateRequest`

| field | type | required |
|---|---|---|
| `address` | string? |  |
| `bill_cutoff_day` | integer? |  |
| `clear_responsible_user` | boolean |  |
| `contacts` | [`Contact`](#schema-contact)[] |  |
| `floors` | integer? |  |
| `latitude` | [`Decimal`](#schema-decimal) |  |
| `longitude` | [`Decimal`](#schema-decimal) |  |
| `name` | string | yes |
| `personnel_count` | integer? |  |
| `responsible_user_id` | `UuidUUID` |  |
| `sector` | string? |  |
| `total_area_m2` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-buildingdetail"></a>`BuildingDetail`

| field | type | required |
|---|---|---|
| `active_analyzer_count` | integer? |  |
| `activity_status` | string: active \| passive? |  |
| `address` | string? |  |
| `analyzer_count` | integer? |  |
| `bill_cutoff_day` | integer | yes |
| `contacts` | [`Contact`](#schema-contact)[] | yes |
| `created_at` | string (date-time) | yes |
| `floors` | integer? |  |
| `id` | `UuidUUID` | yes |
| `latitude` | [`Decimal`](#schema-decimal) |  |
| `longitude` | [`Decimal`](#schema-decimal) |  |
| `name` | string | yes |
| `personnel_count` | integer? |  |
| `responsible_user_id` | `UuidUUID` |  |
| `sector` | string? |  |
| `tariff_history` | [`TariffSummary`](#schema-tariffsummary)[] | yes |
| `total_area_m2` | [`Decimal`](#schema-decimal) |  |
| `updated_at` | string (date-time) | yes |

### <a id="schema-buildingpage"></a>`BuildingPage`

| field | type | required |
|---|---|---|
| `items` | [`Building`](#schema-building)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-buildingtariffstate"></a>`BuildingTariffState`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` | yes |
| `building_name` | string | yes |
| `effective_from` | [`Date`](#schema-date) |  |
| `tariff_id` | `UuidUUID` |  |
| `tariff_name` | string? |  |
| `use_ptf_yekdem` | boolean | yes |

### <a id="schema-buildingtariffstatepage"></a>`BuildingTariffStatePage`

| field | type | required |
|---|---|---|
| `items` | [`BuildingTariffState`](#schema-buildingtariffstate)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-buildingupdaterequest"></a>`BuildingUpdateRequest`

| field | type | required |
|---|---|---|
| `address` | string? |  |
| `bill_cutoff_day` | integer? |  |
| `clear_responsible_user` | boolean |  |
| `contacts` | [`Contact`](#schema-contact)[] |  |
| `floors` | integer? |  |
| `latitude` | [`Decimal`](#schema-decimal) |  |
| `longitude` | [`Decimal`](#schema-decimal) |  |
| `name` | string? |  |
| `personnel_count` | integer? |  |
| `responsible_user_id` | `UuidUUID` |  |
| `sector` | string? |  |
| `total_area_m2` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-bulktariffassignment"></a>`BulkTariffAssignment`

| field | type | required |
|---|---|---|
| `building_ids` | `UuidUUID`[] | yes |
| `created_at` | string (date-time) | yes |
| `created_by` | `UuidUUID` |  |
| `effective_from` | [`Date`](#schema-date) | yes |
| `id` | `UuidUUID` | yes |
| `tariff_name` | string? |  |
| `template_id` | `UuidUUID` |  |

### <a id="schema-bulktariffassignmentpage"></a>`BulkTariffAssignmentPage`

| field | type | required |
|---|---|---|
| `items` | [`BulkTariffAssignment`](#schema-bulktariffassignment)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-bulktariffrequest"></a>`BulkTariffRequest`

| field | type | required |
|---|---|---|
| `building_ids` | `UuidUUID`[] | yes |
| `tariff` | [`TariffFields`](#schema-tarifffields) | yes |

### <a id="schema-bulktariffresult"></a>`BulkTariffResult`

| field | type | required |
|---|---|---|
| `building_ids` | `UuidUUID`[] | yes |
| `tariff_ids` | `UuidUUID`[] | yes |

### <a id="schema-calendarevent"></a>`CalendarEvent`

| field | type | required |
|---|---|---|
| `all_day` | boolean | yes |
| `colour` | string? |  |
| `ends_at` | string (date-time) | yes |
| `id` | `UuidUUID` | yes |
| `starts_at` | string (date-time) | yes |
| `title` | string | yes |

### <a id="schema-calendareventfields"></a>`CalendarEventFields`

| field | type | required |
|---|---|---|
| `all_day` | boolean |  |
| `colour` | string? |  |
| `ends_at` | string (date-time) | yes |
| `starts_at` | string (date-time) | yes |
| `title` | string | yes |

### <a id="schema-calendareventupdaterequest"></a>`CalendarEventUpdateRequest`

| field | type | required |
|---|---|---|
| `all_day` | boolean |  |
| `colour` | string? |  |
| `ends_at` | string (date-time) | yes |
| `starts_at` | string (date-time) | yes |
| `title` | string | yes |

### <a id="schema-calendarevents"></a>`CalendarEvents`

| field | type | required |
|---|---|---|
| `items` | [`CalendarEvent`](#schema-calendarevent)[] | yes |

### <a id="schema-carbonactivity"></a>`CarbonActivity`

| field | type | required |
|---|---|---|
| `activity_type` | string | yes |
| `building_id` | `UuidUUID` | yes |
| `conversion_multiplier` | [`Decimal`](#schema-decimal) | yes |
| `created_at` | string (date-time) | yes |
| `created_by` | `UuidUUID` |  |
| `description` | string? |  |
| `details` | object | yes |
| `emission_kgco2e` | [`Decimal`](#schema-decimal) | yes |
| `factor_id` | `UuidUUID` |  |
| `factor_key` | string? |  |
| `factor_value` | [`Decimal`](#schema-decimal) |  |
| `id` | `UuidUUID` | yes |
| `is_automated` | boolean | yes |
| `iso_category` | string | yes |
| `main_category` | string | yes |
| `period_end` | [`Date`](#schema-date) | yes |
| `period_start` | [`Date`](#schema-date) | yes |
| `quantity` | [`Decimal`](#schema-decimal) | yes |
| `scope` | string: scope_1 \| scope_2 \| scope_3 | yes |
| `status` | string: pending \| approved \| rejected | yes |
| `sub_category` | string | yes |
| `unit` | string | yes |
| `updated_at` | string (date-time) | yes |

### <a id="schema-carbonactivitycreaterequest"></a>`CarbonActivityCreateRequest`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` | yes |
| `description` | string? |  |
| `details` | object |  |
| `factor_key` | string | yes |
| `period_end` | [`Date`](#schema-date) | yes |
| `period_start` | [`Date`](#schema-date) | yes |
| `quantity` | [`Decimal`](#schema-decimal) | yes |
| `sub_category` | string | yes |
| `unit` | string | yes |

### <a id="schema-carbonactivitypage"></a>`CarbonActivityPage`

| field | type | required |
|---|---|---|
| `items` | [`CarbonActivity`](#schema-carbonactivity)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-carbonactivityupdaterequest"></a>`CarbonActivityUpdateRequest`

| field | type | required |
|---|---|---|
| `description` | string? |  |
| `details` | object |  |
| `factor_key` | string | yes |
| `period_end` | [`Date`](#schema-date) | yes |
| `period_start` | [`Date`](#schema-date) | yes |
| `quantity` | [`Decimal`](#schema-decimal) | yes |
| `sub_category` | string | yes |
| `unit` | string | yes |

### <a id="schema-carbonamount"></a>`CarbonAmount`

| field | type | required |
|---|---|---|
| `key` | string | yes |
| `kgco2e` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-carboncatalogue"></a>`CarbonCatalogue`

| field | type | required |
|---|---|---|
| `items` | [`CarbonCatalogueMain`](#schema-carboncataloguemain)[] | yes |

### <a id="schema-carboncataloguemain"></a>`CarbonCatalogueMain`

| field | type | required |
|---|---|---|
| `key` | string | yes |
| `subs` | [`CarbonCatalogueSub`](#schema-carboncataloguesub)[] | yes |

### <a id="schema-carboncataloguesub"></a>`CarbonCatalogueSub`

| field | type | required |
|---|---|---|
| `iso_category` | string | yes |
| `key` | string | yes |
| `scope` | string: scope_1 \| scope_2 \| scope_3 | yes |

### <a id="schema-carbonmonth"></a>`CarbonMonth`

| field | type | required |
|---|---|---|
| `current_kgco2e` | [`Decimal`](#schema-decimal) | yes |
| `month` | integer | yes |
| `previous_kgco2e` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-carbonoverview"></a>`CarbonOverview`

| field | type | required |
|---|---|---|
| `activity_count` | integer | yes |
| `by_category` | [`CarbonAmount`](#schema-carbonamount)[] | yes |
| `by_scope` | [`CarbonAmount`](#schema-carbonamount)[] | yes |
| `highest_source` | [`CarbonAmount`](#schema-carbonamount) |  |
| `monthly` | [`CarbonMonth`](#schema-carbonmonth)[] | yes |
| `pending_count` | integer | yes |
| `recent` | [`CarbonActivity`](#schema-carbonactivity)[] | yes |
| `registered_count` | integer | yes |
| `total_kgco2e` | [`Decimal`](#schema-decimal) | yes |
| `year` | integer | yes |

### <a id="schema-carbonreportrequest"></a>`CarbonReportRequest`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` | yes |
| `from` | [`Date`](#schema-date) | yes |
| `name` | string? |  |
| `report_type` | string: ghg \| iso | yes |
| `to` | [`Date`](#schema-date) | yes |

### <a id="schema-carbonreportsummary"></a>`CarbonReportSummary`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` | yes |
| `created_at` | string (date-time) | yes |
| `id` | `UuidUUID` | yes |
| `name` | string | yes |
| `period` | string | yes |
| `report_type` | string: ghg \| iso | yes |

### <a id="schema-carbonreportsummarypage"></a>`CarbonReportSummaryPage`

| field | type | required |
|---|---|---|
| `items` | [`CarbonReportSummary`](#schema-carbonreportsummary)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-carbonselection"></a>`CarbonSelection`

| field | type | required |
|---|---|---|
| `activity_keys` | string[] | yes |

### <a id="schema-carbonselectionrequest"></a>`CarbonSelectionRequest`

| field | type | required |
|---|---|---|
| `activity_keys` | string[] | yes |

### <a id="schema-carbonstatusrequest"></a>`CarbonStatusRequest`

| field | type | required |
|---|---|---|
| `status` | string: approved \| rejected | yes |

### <a id="schema-changepasswordrequest"></a>`ChangePasswordRequest`

| field | type | required |
|---|---|---|
| `current_password` | string | yes |
| `new_password` | string | yes |

### <a id="schema-company"></a>`Company`

| field | type | required |
|---|---|---|
| `address` | string? |  |
| `contact_name` | string? |  |
| `contact_phone` | string? |  |
| `created_at` | string (date-time) | yes |
| `id` | `UuidUUID` | yes |
| `name` | string | yes |
| `personnel_count` | integer? |  |
| `sector` | string? |  |
| `total_area_m2` | [`Decimal`](#schema-decimal) |  |
| `updated_at` | string (date-time) | yes |

### <a id="schema-companycreaterequest"></a>`CompanyCreateRequest`

| field | type | required |
|---|---|---|
| `address` | string? |  |
| `contact_name` | string? |  |
| `contact_phone` | string? |  |
| `name` | string | yes |
| `personnel_count` | integer? |  |
| `sector` | string? |  |
| `total_area_m2` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-companydetail"></a>`CompanyDetail`

| field | type | required |
|---|---|---|
| `address` | string? |  |
| `analyzer_counts` | [`AnalyzerCount`](#schema-analyzercount)[] | yes |
| `contact_name` | string? |  |
| `contact_phone` | string? |  |
| `created_at` | string (date-time) | yes |
| `id` | `UuidUUID` | yes |
| `name` | string | yes |
| `personnel_count` | integer? |  |
| `sector` | string? |  |
| `total_area_m2` | [`Decimal`](#schema-decimal) |  |
| `updated_at` | string (date-time) | yes |

### <a id="schema-companypage"></a>`CompanyPage`

| field | type | required |
|---|---|---|
| `items` | [`Company`](#schema-company)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-companyref"></a>`CompanyRef`

| field | type | required |
|---|---|---|
| `id` | `UuidUUID` | yes |
| `name` | string | yes |

### <a id="schema-companyupdaterequest"></a>`CompanyUpdateRequest`

| field | type | required |
|---|---|---|
| `address` | string? |  |
| `contact_name` | string? |  |
| `contact_phone` | string? |  |
| `name` | string? |  |
| `personnel_count` | integer? |  |
| `sector` | string? |  |
| `total_area_m2` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-comparisonmetric"></a>`ComparisonMetric`

| field | type | required |
|---|---|---|
| `average` | [`Decimal`](#schema-decimal) |  |
| `rank` | integer | yes |
| `ranked` | integer | yes |
| `value` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-consumptiongrouped"></a>`ConsumptionGrouped`

| field | type | required |
|---|---|---|
| `current` | [`GroupedPeriod`](#schema-groupedperiod) | yes |
| `group_by` | string | yes |
| `previous` | [`GroupedPeriod`](#schema-groupedperiod) |  |

### <a id="schema-consumptionrow"></a>`ConsumptionRow`

| field | type | required |
|---|---|---|
| `active_export` | [`Decimal`](#schema-decimal) |  |
| `active_export_index` | [`Decimal`](#schema-decimal) |  |
| `active_import` | [`Decimal`](#schema-decimal) |  |
| `active_import_index` | [`Decimal`](#schema-decimal) |  |
| `analyzer_id` | `UuidUUID` |  |
| `capacitive_ratio` | [`Decimal`](#schema-decimal) |  |
| `inductive_ratio` | [`Decimal`](#schema-decimal) |  |
| `max_demand_kw` | [`Decimal`](#schema-decimal) |  |
| `partial` | boolean | yes |
| `period_end` | string (date-time) | yes |
| `period_start` | string (date-time) | yes |
| `reactive_capacitive_export` | [`Decimal`](#schema-decimal) |  |
| `reactive_capacitive_export_index` | [`Decimal`](#schema-decimal) |  |
| `reactive_capacitive_import` | [`Decimal`](#schema-decimal) |  |
| `reactive_capacitive_import_index` | [`Decimal`](#schema-decimal) |  |
| `reactive_inductive_export` | [`Decimal`](#schema-decimal) |  |
| `reactive_inductive_export_index` | [`Decimal`](#schema-decimal) |  |
| `reactive_inductive_import` | [`Decimal`](#schema-decimal) |  |
| `reactive_inductive_import_index` | [`Decimal`](#schema-decimal) |  |
| `source` | string | yes |
| `suspect_registers` | string[] | yes |
| `t1_export` | [`Decimal`](#schema-decimal) |  |
| `t1_export_index` | [`Decimal`](#schema-decimal) |  |
| `t1_import` | [`Decimal`](#schema-decimal) |  |
| `t1_import_index` | [`Decimal`](#schema-decimal) |  |
| `t2_export` | [`Decimal`](#schema-decimal) |  |
| `t2_export_index` | [`Decimal`](#schema-decimal) |  |
| `t2_import` | [`Decimal`](#schema-decimal) |  |
| `t2_import_index` | [`Decimal`](#schema-decimal) |  |
| `t3_export` | [`Decimal`](#schema-decimal) |  |
| `t3_export_index` | [`Decimal`](#schema-decimal) |  |
| `t3_import` | [`Decimal`](#schema-decimal) |  |
| `t3_import_index` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-consumptionseries"></a>`ConsumptionSeries`

| field | type | required |
|---|---|---|
| `items` | [`ConsumptionRow`](#schema-consumptionrow)[] | yes |

### <a id="schema-consumptionsummary"></a>`ConsumptionSummary`

| field | type | required |
|---|---|---|
| `averages` | [`Registers`](#schema-registers) | yes |
| `max_demand_kw` | [`Decimal`](#schema-decimal) |  |
| `peak` | [`PeriodValue`](#schema-periodvalue) |  |
| `rows` | integer | yes |
| `suspect_rows` | integer | yes |
| `totals` | [`Registers`](#schema-registers) | yes |
| `valley` | [`PeriodValue`](#schema-periodvalue) |  |

### <a id="schema-contact"></a>`Contact`

| field | type | required |
|---|---|---|
| `name` | string? |  |
| `phone` | string? |  |

### <a id="schema-date"></a>`Date`

string (date)

### <a id="schema-decimal"></a>`Decimal`

string

### <a id="schema-emissionfactorconversion"></a>`EmissionFactorConversion`

| field | type | required |
|---|---|---|
| `label` | string | yes |
| `multiplier` | [`Decimal`](#schema-decimal) | yes |
| `unit` | string | yes |

### <a id="schema-emissionfactorlist"></a>`EmissionFactorList`

| field | type | required |
|---|---|---|
| `items` | [`EmissionFactorView`](#schema-emissionfactorview)[] | yes |

### <a id="schema-emissionfactoroverriderequest"></a>`EmissionFactorOverrideRequest`

| field | type | required |
|---|---|---|
| `base_factor` | [`Decimal`](#schema-decimal) | yes |
| `source` | string? |  |
| `source_url` | string? |  |
| `source_year` | integer? |  |

### <a id="schema-emissionfactorview"></a>`EmissionFactorView`

| field | type | required |
|---|---|---|
| `base_factor` | [`Decimal`](#schema-decimal) | yes |
| `base_unit` | string | yes |
| `category_path` | string[] | yes |
| `conversions` | [`EmissionFactorConversion`](#schema-emissionfactorconversion)[] | yes |
| `fuel_type` | string? |  |
| `id` | `UuidUUID` | yes |
| `iso_category` | string? |  |
| `key` | string | yes |
| `label` | string | yes |
| `main_category` | string | yes |
| `overridden` | boolean | yes |
| `platform_base_factor` | [`Decimal`](#schema-decimal) |  |
| `scope` | string? |  |
| `source` | string? |  |
| `source_url` | string? |  |
| `source_year` | integer? |  |
| `status` | string? |  |
| `sub_categories` | string[] | yes |
| `updated_at` | string (date-time) | yes |
| `vehicle_type` | string? |  |

### <a id="schema-energybalance"></a>`EnergyBalance`

| field | type | required |
|---|---|---|
| `items` | [`BalanceRow`](#schema-balancerow)[] | yes |

### <a id="schema-equivalencefactor"></a>`EquivalenceFactor`

| field | type | required |
|---|---|---|
| `factor` | [`Decimal`](#schema-decimal) | yes |
| `key` | string: equiv_tree_co2_kg_per_year \| equiv_coal_kg_per_kwh \| equiv_car_co2_kg_per_km \| equiv_home_heating_kwh_per_year | yes |
| `source` | string | yes |
| `unit` | string | yes |
| `year` | integer? |  |

### <a id="schema-error"></a>`Error`

| field | type | required |
|---|---|---|
| `error` | [`ErrorBody`](#schema-errorbody) | yes |

### <a id="schema-errorbody"></a>`ErrorBody`

| field | type | required |
|---|---|---|
| `code` | string | yes |
| `details` | object |  |
| `message` | string | yes |
| `request_id` | string |  |

### <a id="schema-financialbuildingtariff"></a>`FinancialBuildingTariff`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` | yes |
| `building_name` | string | yes |
| `currency` | string | yes |
| `price_type` | string: single_time \| multi_time | yes |
| `single` | [`Decimal`](#schema-decimal) |  |
| `t1` | [`Decimal`](#schema-decimal) |  |
| `t2` | [`Decimal`](#schema-decimal) |  |
| `t3` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-financialcoverage"></a>`FinancialCoverage`

| field | type | required |
|---|---|---|
| `consumption_months` | integer | yes |
| `of` | integer | yes |
| `production_months` | integer | yes |

### <a id="schema-financialmonth"></a>`FinancialMonth`

| field | type | required |
|---|---|---|
| `consumption_kwh` | [`Decimal`](#schema-decimal) |  |
| `cost` | [`MoneyAmount`](#schema-moneyamount)[] | yes |
| `grid_purchase_kwh` | [`Decimal`](#schema-decimal) |  |
| `grid_sale_kwh` | [`Decimal`](#schema-decimal) |  |
| `month` | integer | yes |
| `net` | [`MoneyAmount`](#schema-moneyamount)[] | yes |
| `offset_kwh` | [`Decimal`](#schema-decimal) |  |
| `production_kwh` | [`Decimal`](#schema-decimal) |  |
| `revenue` | [`MoneyAmount`](#schema-moneyamount)[] | yes |
| `revenue_partial` | boolean | yes |

### <a id="schema-financialmonthly"></a>`FinancialMonthly`

| field | type | required |
|---|---|---|
| `coverage` | [`FinancialCoverage`](#schema-financialcoverage) | yes |
| `items` | [`FinancialMonth`](#schema-financialmonth)[] | yes |
| `total` | [`FinancialMonth`](#schema-financialmonth) | yes |

### <a id="schema-financialplantfeedin"></a>`FinancialPlantFeedIn`

| field | type | required |
|---|---|---|
| `currency` | string | yes |
| `plant_id` | `UuidUUID` | yes |
| `plant_name` | string | yes |
| `price` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-financialsummary"></a>`FinancialSummary`

| field | type | required |
|---|---|---|
| `analyzer_count` | integer | yes |
| `figures` | [`FinancialMonth`](#schema-financialmonth) | yes |
| `month` | integer? |  |
| `of` | integer | yes |
| `plant_count` | integer | yes |
| `tariffs` | [`FinancialTariffs`](#schema-financialtariffs) | yes |
| `with_data` | integer | yes |
| `year` | integer | yes |

### <a id="schema-financialtariffs"></a>`FinancialTariffs`

| field | type | required |
|---|---|---|
| `purchase` | [`FinancialBuildingTariff`](#schema-financialbuildingtariff)[] | yes |
| `purchase_missing` | boolean | yes |
| `sale` | [`FinancialPlantFeedIn`](#schema-financialplantfeedin)[] | yes |
| `sale_missing` | boolean | yes |

### <a id="schema-forecast"></a>`Forecast`

| field | type | required |
|---|---|---|
| `fallback_from` | string? |  |
| `gaps` | [`ForecastGap`](#schema-forecastgap)[] | yes |
| `generated_at` | string (date-time)? |  |
| `model_id` | string? |  |
| `model_version` | string? |  |
| `points` | [`ForecastPoint`](#schema-forecastpoint)[] | yes |
| `status` | string: ok \| insufficient_data \| no_data \| model_error \| none | yes |
| `used_covariates` | string[] | yes |

### <a id="schema-forecastgap"></a>`ForecastGap`

| field | type | required |
|---|---|---|
| `end` | string (date-time) | yes |
| `missing_hours` | integer | yes |
| `start` | string (date-time) | yes |

### <a id="schema-forecastmonthlyrequest"></a>`ForecastMonthlyRequest`

| field | type | required |
|---|---|---|
| `analyzer_id` | `UuidUUID` | yes |
| `month` | string | yes |

### <a id="schema-forecastpoint"></a>`ForecastPoint`

| field | type | required |
|---|---|---|
| `median` | [`Decimal`](#schema-decimal) | yes |
| `p10` | [`Decimal`](#schema-decimal) |  |
| `p90` | [`Decimal`](#schema-decimal) |  |
| `ts` | string (date-time) | yes |

### <a id="schema-forecastrunrequest"></a>`ForecastRunRequest`

| field | type | required |
|---|---|---|
| `analyzer_id` | `UuidUUID` | yes |
| `horizon_hours` | integer | yes |

### <a id="schema-forecastweeklyrequest"></a>`ForecastWeeklyRequest`

| field | type | required |
|---|---|---|
| `analyzer_id` | `UuidUUID` | yes |
| `week_start` | [`Date`](#schema-date) | yes |

### <a id="schema-forgotpasswordrequest"></a>`ForgotPasswordRequest`

| field | type | required |
|---|---|---|
| `email` | string (email) | yes |

### <a id="schema-groupedbucket"></a>`GroupedBucket`

| field | type | required |
|---|---|---|
| `active_import` | [`Decimal`](#schema-decimal) |  |
| `days` | integer | yes |
| `key` | string | yes |
| `partial` | boolean | yes |
| `reactive_capacitive_import` | [`Decimal`](#schema-decimal) |  |
| `reactive_inductive_import` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-groupedextreme"></a>`GroupedExtreme`

| field | type | required |
|---|---|---|
| `key` | string | yes |
| `value` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-groupedperiod"></a>`GroupedPeriod`

| field | type | required |
|---|---|---|
| `from` | [`Date`](#schema-date) | yes |
| `groups` | [`GroupedBucket`](#schema-groupedbucket)[] | yes |
| `statistics` | [`GroupedStatistics`](#schema-groupedstatistics) | yes |
| `to` | [`Date`](#schema-date) | yes |

### <a id="schema-groupedstatistics"></a>`GroupedStatistics`

| field | type | required |
|---|---|---|
| `average` | [`Decimal`](#schema-decimal) |  |
| `peak` | [`GroupedExtreme`](#schema-groupedextreme) |  |
| `total` | [`Decimal`](#schema-decimal) |  |
| `valley` | [`GroupedExtreme`](#schema-groupedextreme) |  |

### <a id="schema-isoclause"></a>`ISOClause`

| field | type | required |
|---|---|---|
| `id` | string | yes |
| `subs` | [`ISOSubClause`](#schema-isosubclause)[] | yes |
| `title` | string | yes |

### <a id="schema-isoclausestate"></a>`ISOClauseState`

| field | type | required |
|---|---|---|
| `clause_id` | string | yes |
| `end` | [`Date`](#schema-date) |  |
| `start` | [`Date`](#schema-date) |  |
| `status` | string: not_started \| in_progress \| completed \| expired? |  |

### <a id="schema-isoclauses"></a>`ISOClauses`

| field | type | required |
|---|---|---|
| `items` | [`ISOClause`](#schema-isoclause)[] | yes |

### <a id="schema-isocount"></a>`ISOCount`

| field | type | required |
|---|---|---|
| `clause_id` | string | yes |
| `files` | integer | yes |
| `notes` | integer | yes |

### <a id="schema-isodaterow"></a>`ISODateRow`

| field | type | required |
|---|---|---|
| `clause_id` | string | yes |
| `end` | [`Date`](#schema-date) |  |
| `start` | [`Date`](#schema-date) |  |

### <a id="schema-isodatesrequest"></a>`ISODatesRequest`

| field | type | required |
|---|---|---|
| `clauses` | [`ISODateRow`](#schema-isodaterow)[] | yes |

### <a id="schema-isofile"></a>`ISOFile`

| field | type | required |
|---|---|---|
| `clause_id` | string | yes |
| `content_type` | string | yes |
| `created_at` | string (date-time) | yes |
| `id` | `UuidUUID` | yes |
| `name` | string | yes |
| `size_bytes` | integer (int64) | yes |

### <a id="schema-isofiles"></a>`ISOFiles`

| field | type | required |
|---|---|---|
| `items` | [`ISOFile`](#schema-isofile)[] | yes |

### <a id="schema-isonote"></a>`ISONote`

| field | type | required |
|---|---|---|
| `body` | string | yes |
| `clause_id` | string | yes |
| `created_at` | string (date-time) | yes |
| `created_by` | `UuidUUID` |  |
| `id` | `UuidUUID` | yes |
| `title` | string? |  |
| `updated_at` | string (date-time) | yes |

### <a id="schema-isonotecreaterequest"></a>`ISONoteCreateRequest`

| field | type | required |
|---|---|---|
| `body` | string | yes |
| `title` | string? |  |

### <a id="schema-isonoteupdaterequest"></a>`ISONoteUpdateRequest`

| field | type | required |
|---|---|---|
| `body` | string | yes |
| `clause_id` | string | yes |
| `title` | string? |  |

### <a id="schema-isonotes"></a>`ISONotes`

| field | type | required |
|---|---|---|
| `items` | [`ISONote`](#schema-isonote)[] | yes |

### <a id="schema-isoproject"></a>`ISOProject`

| field | type | required |
|---|---|---|
| `clauses` | [`ISOClauseState`](#schema-isoclausestate)[] | yes |
| `counts` | [`ISOCount`](#schema-isocount)[] | yes |
| `gantt_available` | boolean | yes |
| `progress` | integer | yes |
| `project_end` | [`Date`](#schema-date) |  |
| `project_start` | [`Date`](#schema-date) |  |

### <a id="schema-isosubclause"></a>`ISOSubClause`

| field | type | required |
|---|---|---|
| `description` | string | yes |
| `id` | string | yes |
| `template` | string? |  |
| `title` | string | yes |

### <a id="schema-isotemplate"></a>`ISOTemplate`

| field | type | required |
|---|---|---|
| `clauses` | string[] | yes |
| `description` | string | yes |
| `file_name` | string | yes |
| `id` | string | yes |

### <a id="schema-isotemplates"></a>`ISOTemplates`

| field | type | required |
|---|---|---|
| `items` | [`ISOTemplate`](#schema-isotemplate)[] | yes |

### <a id="schema-isolaraccountplant"></a>`ISolarAccountPlant`

| field | type | required |
|---|---|---|
| `installed_kw` | [`Decimal`](#schema-decimal) |  |
| `linked_plant_id` | `UuidUUID` |  |
| `name` | string | yes |
| `ps_id` | string | yes |

### <a id="schema-isolaraccountplants"></a>`ISolarAccountPlants`

| field | type | required |
|---|---|---|
| `items` | [`ISolarAccountPlant`](#schema-isolaraccountplant)[] | yes |

### <a id="schema-isolarlinkrequest"></a>`ISolarLinkRequest`

| field | type | required |
|---|---|---|
| `credential_id` | `UuidUUID` | yes |
| `ps_id` | string | yes |

### <a id="schema-isolarlinked"></a>`ISolarLinked`

| field | type | required |
|---|---|---|
| `job_id` | string | yes |
| `plant` | [`PlantDetail`](#schema-plantdetail) | yes |

### <a id="schema-icmalanalysis"></a>`IcmalAnalysis`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` |  |
| `distribution_tl_per_kwh` | [`IcmalCoefficient`](#schema-icmalcoefficient) | yes |
| `energy_kbk` | [`IcmalCoefficient`](#schema-icmalcoefficient) | yes |
| `etso_code` | string | yes |
| `implied_contracted_power_kw` | [`Decimal`](#schema-decimal) |  |
| `is_multi_time` | boolean? |  |
| `overuse_requires_manual_entry` | boolean | yes |
| `periods` | string[] | yes |
| `power_unit_price` | [`IcmalCoefficient`](#schema-icmalcoefficient) | yes |
| `reactive_kbk` | [`IcmalCoefficient`](#schema-icmalcoefficient) | yes |
| `reactive_unit_price` | [`IcmalCoefficient`](#schema-icmalcoefficient) | yes |
| `taxes` | object | yes |
| `term` | string? |  |
| `vat_rate` | [`IcmalCoefficient`](#schema-icmalcoefficient) | yes |
| `voltage_level` | string? |  |
| `warnings` | [`IcmalWarning`](#schema-icmalwarning)[] | yes |
| `within_tolerance` | boolean | yes |

### <a id="schema-icmalapplyrequest"></a>`IcmalApplyRequest`

| field | type | required |
|---|---|---|
| `confirmations` | [`IcmalConfirmation`](#schema-icmalconfirmation)[] | yes |

### <a id="schema-icmalcoefficient"></a>`IcmalCoefficient`

| field | type | required |
|---|---|---|
| `back_calc_error_pct` | [`Decimal`](#schema-decimal) |  |
| `samples` | integer | yes |
| `stable` | boolean? |  |
| `std_dev` | [`Decimal`](#schema-decimal) |  |
| `value` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-icmalconfirmation"></a>`IcmalConfirmation`

| field | type | required |
|---|---|---|
| `base` | [`TariffFields`](#schema-tarifffields) |  |
| `building_id` | `UuidUUID` | yes |
| `effective_from` | [`Date`](#schema-date) | yes |
| `etso_code` | string | yes |

### <a id="schema-icmalimport"></a>`IcmalImport`

| field | type | required |
|---|---|---|
| `analyses` | [`IcmalAnalysis`](#schema-icmalanalysis)[] | yes |
| `created_at` | string (date-time) | yes |
| `file_name` | string | yes |
| `id` | `UuidUUID` | yes |
| `row_count` | integer | yes |
| `status` | string: pending \| analysed \| applied | yes |
| `unmatched` | string[] | yes |
| `warnings` | [`IcmalWarning`](#schema-icmalwarning)[] | yes |

### <a id="schema-icmalwarning"></a>`IcmalWarning`

| field | type | required |
|---|---|---|
| `code` | string | yes |
| `row` | integer | yes |
| `text` | string | yes |

### <a id="schema-integrationcredential"></a>`IntegrationCredential`

| field | type | required |
|---|---|---|
| `definition_id` | `UuidUUID` | yes |
| `extra_keys` | string[] | yes |
| `has_secret` | boolean | yes |
| `id` | `UuidUUID` | yes |
| `is_active` | boolean | yes |
| `isolar_region` | string? |  |
| `last_verified_at` | string (date-time)? |  |
| `pm5340_url` | string? |  |
| `provider` | string | yes |
| `settings` | any |  |
| `subtype` | string | yes |
| `token_expires_at` | string (date-time)? |  |
| `updated_at` | string (date-time) | yes |
| `username` | string? |  |

### <a id="schema-integrationcredentialcreaterequest"></a>`IntegrationCredentialCreateRequest`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` |  |
| `extra` | object |  |
| `installation_number` | string? |  |
| `is_active` | boolean? |  |
| `isolar_region` | string? |  |
| `pm5340_url` | string? |  |
| `provider` | string: osos \| gridbox \| aril \| pm5340 \| isolar | yes |
| `secret` | string? |  |
| `settings` | any |  |
| `subtype` | string | yes |
| `username` | string? |  |
| `wiring_numbers` | string[] |  |

### <a id="schema-integrationcredentialupdaterequest"></a>`IntegrationCredentialUpdateRequest`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` |  |
| `extra` | object |  |
| `installation_number` | string? |  |
| `is_active` | boolean? |  |
| `isolar_region` | string? |  |
| `pm5340_url` | string? |  |
| `secret` | string? |  |
| `settings` | any |  |
| `username` | string? |  |
| `wiring_numbers` | string[] |  |

### <a id="schema-integrationcredentials"></a>`IntegrationCredentials`

| field | type | required |
|---|---|---|
| `items` | [`IntegrationCredential`](#schema-integrationcredential)[] | yes |

### <a id="schema-integrationdefinition"></a>`IntegrationDefinition`

| field | type | required |
|---|---|---|
| `endpoints` | any | yes |
| `id` | `UuidUUID` | yes |
| `provider` | string: osos \| gridbox \| aril \| pm5340 \| isolar | yes |
| `subtype` | string | yes |
| `updated_at` | string (date-time) | yes |

### <a id="schema-integrationdefinitioncreaterequest"></a>`IntegrationDefinitionCreateRequest`

| field | type | required |
|---|---|---|
| `endpoints` | any | yes |
| `provider` | string: osos \| gridbox \| aril \| pm5340 \| isolar | yes |
| `subtype` | string | yes |

### <a id="schema-integrationdefinitionupdaterequest"></a>`IntegrationDefinitionUpdateRequest`

| field | type | required |
|---|---|---|
| `endpoints` | any | yes |
| `subtype` | string | yes |

### <a id="schema-integrationdefinitions"></a>`IntegrationDefinitions`

| field | type | required |
|---|---|---|
| `items` | [`IntegrationDefinition`](#schema-integrationdefinition)[] | yes |

### <a id="schema-job"></a>`Job`

| field | type | required |
|---|---|---|
| `completed_at` | string (date-time)? |  |
| `error_code` | string: tariff_not_found \| no_consumption_data \| unresolved_anomaly \| period_not_closed \| ptf_data_missing \| billing_parameters_missing \| billing_parameters_invalid \| report_building_not_found \| report_not_ready \| smtp_not_configured \| delivery_failed \| isolar_auth \| isolar_unavailable \| isolar_not_linked \| credential_missing \| credential_inactive |  |
| `id` | string | yes |
| `status` | string: queued \| running \| succeeded \| failed | yes |
| `type` | string | yes |

### <a id="schema-jobaccepted"></a>`JobAccepted`

| field | type | required |
|---|---|---|
| `job_id` | string | yes |

### <a id="schema-jobrun"></a>`JobRun`

| field | type | required |
|---|---|---|
| `error` | string? |  |
| `failed` | integer (int32) | yes |
| `finished_at` | string (date-time)? |  |
| `id` | `UuidUUID` | yes |
| `job_type` | string | yes |
| `processed` | integer (int32) | yes |
| `scope` | any | yes |
| `skipped` | integer (int32) | yes |
| `started_at` | string (date-time) | yes |
| `status` | string: running \| success \| partial \| failed | yes |

### <a id="schema-jobrunpage"></a>`JobRunPage`

| field | type | required |
|---|---|---|
| `items` | [`JobRun`](#schema-jobrun)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-loadprofileconfig"></a>`LoadProfileConfig`

| field | type | required |
|---|---|---|
| `vacations` | integer | yes |
| `weekend_days` | integer[] | yes |
| `weekend_source` | string: company \| default | yes |

### <a id="schema-loadprofilestatistics"></a>`LoadProfileStatistics`

| field | type | required |
|---|---|---|
| `config` | [`LoadProfileConfig`](#schema-loadprofileconfig) | yes |
| `statistics` | object | yes |

### <a id="schema-loadprofiles"></a>`LoadProfiles`

| field | type | required |
|---|---|---|
| `config` | [`LoadProfileConfig`](#schema-loadprofileconfig) | yes |
| `days` | object | yes |
| `profiles` | object | yes |

### <a id="schema-locale"></a>`Locale`

string: tr \| en

### <a id="schema-loginrequest"></a>`LoginRequest`

| field | type | required |
|---|---|---|
| `email` | string (email) | yes |
| `password` | string | yes |
| `remember_me` | boolean |  |

### <a id="schema-me"></a>`Me`

| field | type | required |
|---|---|---|
| `company` | [`CompanyRef`](#schema-companyref) | yes |
| `email` | string | yes |
| `id` | `UuidUUID` | yes |
| `locale` | [`Locale`](#schema-locale) | yes |
| `name` | string | yes |
| `permissions` | [`Permission`](#schema-permission)[] | yes |
| `phone` | string? |  |
| `role` | [`Role`](#schema-role) | yes |
| `session_id` | `UuidUUID` | yes |
| `ui_preferences` | string? |  |

### <a id="schema-message"></a>`Message`

| field | type | required |
|---|---|---|
| `category` | string | yes |
| `created_at` | string (date-time) | yes |
| `detail` | string? |  |
| `id` | integer (int64) | yes |
| `kind` | string: alarm \| job \| system | yes |
| `message` | string | yes |
| `related_id` | `UuidUUID` |  |
| `related_type` | string? |  |
| `status` | string: success \| error \| warning \| info | yes |

### <a id="schema-messagepage"></a>`MessagePage`

| field | type | required |
|---|---|---|
| `items` | [`Message`](#schema-message)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-mobileloginrequest"></a>`MobileLoginRequest`

| field | type | required |
|---|---|---|
| `email` | string (email) | yes |
| `password` | string | yes |

### <a id="schema-mobilerefreshrequest"></a>`MobileRefreshRequest`

| field | type | required |
|---|---|---|
| `refresh_token` | string | yes |

### <a id="schema-mobiletokens"></a>`MobileTokens`

| field | type | required |
|---|---|---|
| `access_token` | string | yes |
| `expires_in` | integer (int64) | yes |
| `refresh_token` | string | yes |
| `token_type` | string: Bearer | yes |
| `user` | [`Me`](#schema-me) | yes |

### <a id="schema-moneyamount"></a>`MoneyAmount`

| field | type | required |
|---|---|---|
| `amount` | [`Decimal`](#schema-decimal) | yes |
| `currency` | string: TRY \| USD \| EUR | yes |

### <a id="schema-nationaltariff"></a>`NationalTariff`

| field | type | required |
|---|---|---|
| `created_at` | string (date-time) | yes |
| `daily_threshold_kwh` | [`Decimal`](#schema-decimal) |  |
| `distribution_price` | [`Decimal`](#schema-decimal) | yes |
| `effective_from` | [`Date`](#schema-date) | yes |
| `energy_price` | [`Decimal`](#schema-decimal) | yes |
| `id` | `UuidUUID` | yes |
| `overuse_price` | [`Decimal`](#schema-decimal) |  |
| `power_price` | [`Decimal`](#schema-decimal) |  |
| `source` | string? |  |
| `t1_price` | [`Decimal`](#schema-decimal) |  |
| `t2_price` | [`Decimal`](#schema-decimal) |  |
| `t3_price` | [`Decimal`](#schema-decimal) |  |
| `term` | string: monomial \| binomial | yes |
| `user_group` | string: residential \| residential_plus \| commercial \| commercial_plus \| industrial \| agricultural \| lighting \| martyrs_families \| public_lighting | yes |
| `vat_rate` | [`Decimal`](#schema-decimal) | yes |
| `voltage_level` | string: lv \| mv | yes |

### <a id="schema-nationaltarifffields"></a>`NationalTariffFields`

| field | type | required |
|---|---|---|
| `daily_threshold_kwh` | [`Decimal`](#schema-decimal) |  |
| `distribution_price` | [`Decimal`](#schema-decimal) | yes |
| `effective_from` | [`Date`](#schema-date) | yes |
| `energy_price` | [`Decimal`](#schema-decimal) | yes |
| `overuse_price` | [`Decimal`](#schema-decimal) |  |
| `power_price` | [`Decimal`](#schema-decimal) |  |
| `source` | string? |  |
| `t1_price` | [`Decimal`](#schema-decimal) |  |
| `t2_price` | [`Decimal`](#schema-decimal) |  |
| `t3_price` | [`Decimal`](#schema-decimal) |  |
| `term` | string: monomial \| binomial | yes |
| `user_group` | string: residential \| residential_plus \| commercial \| commercial_plus \| industrial \| agricultural \| lighting \| martyrs_families \| public_lighting | yes |
| `vat_rate` | [`Decimal`](#schema-decimal) | yes |
| `voltage_level` | string: lv \| mv | yes |

### <a id="schema-nationaltariffpage"></a>`NationalTariffPage`

| field | type | required |
|---|---|---|
| `items` | [`NationalTariff`](#schema-nationaltariff)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-periodvalue"></a>`PeriodValue`

| field | type | required |
|---|---|---|
| `active_import` | [`Decimal`](#schema-decimal) | yes |
| `period_start` | string (date-time) | yes |

### <a id="schema-permission"></a>`Permission`

string: admin.companies \| alarms.edit \| alarms.evaluate \| alarms.read \| analyzers.refresh \| anomaly.check \| bills.compute \| bills.read \| calendar.edit \| carbon.edit \| carbon.read \| financial.read \| forecast.read \| forecast.run \| integrations.credentials \| iso50001.edit \| iso50001.read \| jobs.runs.read \| jobs.trigger \| messages.read \| nav.core \| nav.financial \| nav.solar_plants \| plants.manage \| plants.read \| renewable.read \| reports.email \| reports.generate \| reports.read \| settings.analyzers \| settings.analyzers.edit \| settings.buildings \| settings.company \| settings.company.edit \| settings.integrations \| settings.plants \| settings.smtp \| settings.users \| solar_tariffs.read \| tariffs.bulk.read \| tariffs.defaults \| tariffs.edit \| tariffs.icmal \| tariffs.read \| tariffs.templates.read \| write

### <a id="schema-plant"></a>`Plant`

| field | type | required |
|---|---|---|
| `address` | string? |  |
| `created_at` | string (date-time) | yes |
| `id` | `UuidUUID` | yes |
| `installation_date` | [`Date`](#schema-date) |  |
| `installation_number` | string? |  |
| `isolar_credential_id` | `UuidUUID` |  |
| `isolar_installed_kw` | [`Decimal`](#schema-decimal) |  |
| `isolar_last_sync_at` | string (date-time)? |  |
| `isolar_last_sync_error` | string? |  |
| `isolar_linked_at` | string (date-time)? |  |
| `isolar_ps_id` | string? |  |
| `isolar_ps_name` | string? |  |
| `latitude` | [`Decimal`](#schema-decimal) |  |
| `longitude` | [`Decimal`](#schema-decimal) |  |
| `name` | string | yes |
| `netting_analyzer_id` | `UuidUUID` |  |
| `orientation` | string: n \| s \| e \| w \| ne \| se \| nw \| sw? |  |
| `panel_count` | integer? |  |
| `panel_efficiency_pct` | [`Decimal`](#schema-decimal) |  |
| `panel_power_w` | [`Decimal`](#schema-decimal) |  |
| `plant_kind` | string: rooftop \| grid | yes |
| `pv_brand_model` | string? |  |
| `string_count` | integer? |  |
| `tilt_angle_deg` | [`Decimal`](#schema-decimal) |  |
| `total_capacity_kw` | [`Decimal`](#schema-decimal) |  |
| `yearly_target_kwh` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-plantcreaterequest"></a>`PlantCreateRequest`

| field | type | required |
|---|---|---|
| `address` | string? |  |
| `alarm_recipients` | string[] |  |
| `clear_netting_analyzer` | boolean |  |
| `installation_date` | [`Date`](#schema-date) |  |
| `installation_number` | string? |  |
| `latitude` | [`Decimal`](#schema-decimal) |  |
| `longitude` | [`Decimal`](#schema-decimal) |  |
| `monthly_targets` | [`Decimal`](#schema-decimal)[] |  |
| `name` | string | yes |
| `netting_analyzer_id` | `UuidUUID` |  |
| `orientation` | string: n \| s \| e \| w \| ne \| se \| nw \| sw? |  |
| `panel_count` | integer? |  |
| `panel_efficiency_pct` | [`Decimal`](#schema-decimal) |  |
| `panel_power_w` | [`Decimal`](#schema-decimal) |  |
| `plant_kind` | string: rooftop \| grid? |  |
| `pv_brand_model` | string? |  |
| `string_count` | integer? |  |
| `tilt_angle_deg` | [`Decimal`](#schema-decimal) |  |
| `total_capacity_kw` | [`Decimal`](#schema-decimal) |  |
| `yearly_target_kwh` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-plantdetail"></a>`PlantDetail`

| field | type | required |
|---|---|---|
| `address` | string? |  |
| `alarm_recipients` | string[] | yes |
| `created_at` | string (date-time) | yes |
| `devices` | [`PlantDevice`](#schema-plantdevice)[] | yes |
| `id` | `UuidUUID` | yes |
| `installation_date` | [`Date`](#schema-date) |  |
| `installation_number` | string? |  |
| `isolar_credential_id` | `UuidUUID` |  |
| `isolar_installed_kw` | [`Decimal`](#schema-decimal) |  |
| `isolar_last_sync_at` | string (date-time)? |  |
| `isolar_last_sync_error` | string? |  |
| `isolar_linked_at` | string (date-time)? |  |
| `isolar_ps_id` | string? |  |
| `isolar_ps_name` | string? |  |
| `latitude` | [`Decimal`](#schema-decimal) |  |
| `longitude` | [`Decimal`](#schema-decimal) |  |
| `monthly_targets` | [`Decimal`](#schema-decimal)[] | yes |
| `name` | string | yes |
| `netting_analyzer_id` | `UuidUUID` |  |
| `orientation` | string: n \| s \| e \| w \| ne \| se \| nw \| sw? |  |
| `panel_count` | integer? |  |
| `panel_efficiency_pct` | [`Decimal`](#schema-decimal) |  |
| `panel_power_w` | [`Decimal`](#schema-decimal) |  |
| `plant_kind` | string: rooftop \| grid | yes |
| `pv_brand_model` | string? |  |
| `string_count` | integer? |  |
| `tilt_angle_deg` | [`Decimal`](#schema-decimal) |  |
| `total_capacity_kw` | [`Decimal`](#schema-decimal) |  |
| `yearly_target_kwh` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-plantdevice"></a>`PlantDevice`

| field | type | required |
|---|---|---|
| `brand` | string? |  |
| `device_name` | string? |  |
| `device_sn` | string | yes |
| `efficiency_pct` | [`Decimal`](#schema-decimal) |  |
| `id` | `UuidUUID` | yes |
| `last_seen_at` | string (date-time)? |  |
| `model` | string? |  |
| `rated_power_kw` | [`Decimal`](#schema-decimal) |  |
| `status` | string? |  |

### <a id="schema-plantdeviceview"></a>`PlantDeviceView`

| field | type | required |
|---|---|---|
| `active_power_kw` | [`Decimal`](#schema-decimal) |  |
| `device_name` | string? |  |
| `device_sn` | string | yes |
| `device_type` | integer? |  |
| `id` | `UuidUUID` | yes |
| `last_update` | string (date-time)? |  |
| `status` | string: normal \| alarm \| fault \| offline? |  |
| `yield_today_kwh` | [`Decimal`](#schema-decimal) |  |
| `yield_total_kwh` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-plantdevices"></a>`PlantDevices`

| field | type | required |
|---|---|---|
| `items` | [`PlantDeviceView`](#schema-plantdeviceview)[] | yes |

### <a id="schema-plantfault"></a>`PlantFault`

| field | type | required |
|---|---|---|
| `closed_at` | string (date-time)? |  |
| `code` | string | yes |
| `device_name` | string? |  |
| `level` | integer? |  |
| `message_tr` | string | yes |
| `name` | string | yes |
| `occurred_at` | string (date-time) | yes |
| `ref` | string | yes |
| `translated` | boolean | yes |
| `type` | integer? |  |

### <a id="schema-plantfaultpage"></a>`PlantFaultPage`

| field | type | required |
|---|---|---|
| `items` | [`PlantFault`](#schema-plantfault)[] | yes |
| `next_cursor` | string? |  |
| `total` | integer | yes |

### <a id="schema-plantpage"></a>`PlantPage`

| field | type | required |
|---|---|---|
| `items` | [`Plant`](#schema-plant)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-plantproductionpoint"></a>`PlantProductionPoint`

| field | type | required |
|---|---|---|
| `basis` | string: plant_meter \| inverter_sum \| daily_total? |  |
| `production_kwh` | [`Decimal`](#schema-decimal) |  |
| `ts` | string (date-time) | yes |

### <a id="schema-plantproductionseries"></a>`PlantProductionSeries`

| field | type | required |
|---|---|---|
| `granularity` | string: hour \| day \| month | yes |
| `mixed_basis` | boolean | yes |
| `points` | [`PlantProductionPoint`](#schema-plantproductionpoint)[] | yes |

### <a id="schema-plantrealtime"></a>`PlantRealtime`

| field | type | required |
|---|---|---|
| `active_power_kw` | [`Decimal`](#schema-decimal) |  |
| `as_of` | string (date-time)? |  |
| `capacity_kw` | [`Decimal`](#schema-decimal) |  |
| `capacity_utilisation_pct` | [`Decimal`](#schema-decimal) |  |
| `connection` | string: connected \| error \| never_synced | yes |
| `connection_error` | string? |  |
| `inverter_count` | integer | yes |
| `last_sync_at` | string (date-time)? |  |
| `stale` | boolean | yes |
| `yield_month_kwh` | [`Decimal`](#schema-decimal) |  |
| `yield_today_kwh` | [`Decimal`](#schema-decimal) |  |
| `yield_total_kwh` | [`Decimal`](#schema-decimal) |  |
| `yield_year_kwh` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-plantrevenue"></a>`PlantRevenue`

| field | type | required |
|---|---|---|
| `available` | boolean | yes |
| `daily` | [`RevenuePeriod`](#schema-revenueperiod) |  |
| `monthly` | [`RevenuePeriod`](#schema-revenueperiod) |  |
| `reason` | string: no_solar_tariff |  |
| `total` | [`RevenuePeriod`](#schema-revenueperiod) |  |
| `yearly` | [`RevenuePeriod`](#schema-revenueperiod) |  |

### <a id="schema-plantupdaterequest"></a>`PlantUpdateRequest`

| field | type | required |
|---|---|---|
| `address` | string? |  |
| `alarm_recipients` | string[] |  |
| `clear_netting_analyzer` | boolean |  |
| `installation_date` | [`Date`](#schema-date) |  |
| `installation_number` | string? |  |
| `latitude` | [`Decimal`](#schema-decimal) |  |
| `longitude` | [`Decimal`](#schema-decimal) |  |
| `monthly_targets` | [`Decimal`](#schema-decimal)[] |  |
| `name` | string? |  |
| `netting_analyzer_id` | `UuidUUID` |  |
| `orientation` | string: n \| s \| e \| w \| ne \| se \| nw \| sw? |  |
| `panel_count` | integer? |  |
| `panel_efficiency_pct` | [`Decimal`](#schema-decimal) |  |
| `panel_power_w` | [`Decimal`](#schema-decimal) |  |
| `plant_kind` | string: rooftop \| grid? |  |
| `pv_brand_model` | string? |  |
| `string_count` | integer? |  |
| `tilt_angle_deg` | [`Decimal`](#schema-decimal) |  |
| `total_capacity_kw` | [`Decimal`](#schema-decimal) |  |
| `yearly_target_kwh` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-profilestatistics"></a>`ProfileStatistics`

| field | type | required |
|---|---|---|
| `hour_of_max` | integer? |  |
| `load_factor` | [`Decimal`](#schema-decimal) |  |
| `max` | [`Decimal`](#schema-decimal) |  |
| `mean` | [`Decimal`](#schema-decimal) |  |
| `min` | [`Decimal`](#schema-decimal) |  |
| `range` | [`Decimal`](#schema-decimal) |  |
| `stddev` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-profileupdaterequest"></a>`ProfileUpdateRequest`

| field | type | required |
|---|---|---|
| `email` | string (email)? |  |
| `locale` | [`Locale`](#schema-locale) |  |
| `name` | string? |  |
| `phone` | string? |  |
| `ui_preferences` | string? |  |

### <a id="schema-publicbill"></a>`PublicBill`

| field | type | required |
|---|---|---|
| `basis` | string: total \| bands | yes |
| `days` | integer | yes |
| `distribution` | [`Decimal`](#schema-decimal) | yes |
| `energy` | [`Decimal`](#schema-decimal) | yes |
| `overuse` | [`Decimal`](#schema-decimal) | yes |
| `power` | [`Decimal`](#schema-decimal) | yes |
| `tariff` | [`PublicBillTariff`](#schema-publicbilltariff) | yes |
| `total` | [`Decimal`](#schema-decimal) | yes |
| `vat` | [`Decimal`](#schema-decimal) | yes |
| `vat_base` | [`Decimal`](#schema-decimal) | yes |
| `vat_rate` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-publicbillrequest"></a>`PublicBillRequest`

| field | type | required |
|---|---|---|
| `contract_power` | [`Decimal`](#schema-decimal) |  |
| `demand` | [`Decimal`](#schema-decimal) |  |
| `end` | [`Date`](#schema-date) | yes |
| `multi_time` | boolean |  |
| `start` | [`Date`](#schema-date) | yes |
| `t1` | [`Decimal`](#schema-decimal) |  |
| `t2` | [`Decimal`](#schema-decimal) |  |
| `t3` | [`Decimal`](#schema-decimal) |  |
| `term` | string: monomial \| binomial | yes |
| `total_consumption` | [`Decimal`](#schema-decimal) |  |
| `user_group` | string: residential \| commercial \| industrial \| agricultural \| lighting | yes |
| `voltage_level` | string: lv \| mv | yes |

### <a id="schema-publicbilltariff"></a>`PublicBillTariff`

| field | type | required |
|---|---|---|
| `effective_from` | [`Date`](#schema-date) | yes |
| `group_used` | string | yes |
| `source` | string? |  |

### <a id="schema-publiccontactrequest"></a>`PublicContactRequest`

| field | type | required |
|---|---|---|
| `elapsed_ms` | integer |  |
| `email` | string (email) | yes |
| `message` | string | yes |
| `name` | string | yes |
| `phone` | string |  |
| `subject` | string |  |
| `website` | string |  |

### <a id="schema-publicdemorequest"></a>`PublicDemoRequest`

| field | type | required |
|---|---|---|
| `company` | string | yes |
| `elapsed_ms` | integer |  |
| `email` | string (email) | yes |
| `message` | string |  |
| `name` | string | yes |
| `phone` | string | yes |
| `role` | string |  |
| `website` | string |  |

### <a id="schema-reactiveanalyzer"></a>`ReactiveAnalyzer`

| field | type | required |
|---|---|---|
| `analyzer_id` | `UuidUUID` | yes |
| `building_id` | `UuidUUID` |  |
| `capacitive_limit` | [`Decimal`](#schema-decimal) |  |
| `capacitive_ratio` | [`Decimal`](#schema-decimal) |  |
| `exempt_reason` | string: term \| user_group \| generation \| below_kw? |  |
| `has_data` | boolean | yes |
| `inductive_limit` | [`Decimal`](#schema-decimal) |  |
| `inductive_ratio` | [`Decimal`](#schema-decimal) |  |
| `installed_power_kw` | [`Decimal`](#schema-decimal) |  |
| `penalty_applies` | boolean | yes |

### <a id="schema-reactiveextreme"></a>`ReactiveExtreme`

| field | type | required |
|---|---|---|
| `analyzer_id` | `UuidUUID` | yes |
| `building_id` | `UuidUUID` |  |
| `ratio` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-reactivestatus"></a>`ReactiveStatus`

| field | type | required |
|---|---|---|
| `analyzers` | [`ReactiveAnalyzer`](#schema-reactiveanalyzer)[] | yes |
| `highest_capacitive` | [`ReactiveExtreme`](#schema-reactiveextreme) |  |
| `highest_inductive` | [`ReactiveExtreme`](#schema-reactiveextreme) |  |
| `month` | string | yes |

### <a id="schema-registers"></a>`Registers`

| field | type | required |
|---|---|---|
| `active_export` | [`Decimal`](#schema-decimal) |  |
| `active_import` | [`Decimal`](#schema-decimal) |  |
| `reactive_capacitive_export` | [`Decimal`](#schema-decimal) |  |
| `reactive_capacitive_import` | [`Decimal`](#schema-decimal) |  |
| `reactive_inductive_export` | [`Decimal`](#schema-decimal) |  |
| `reactive_inductive_import` | [`Decimal`](#schema-decimal) |  |
| `t1_export` | [`Decimal`](#schema-decimal) |  |
| `t1_import` | [`Decimal`](#schema-decimal) |  |
| `t2_export` | [`Decimal`](#schema-decimal) |  |
| `t2_import` | [`Decimal`](#schema-decimal) |  |
| `t3_export` | [`Decimal`](#schema-decimal) |  |
| `t3_import` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-renewableanalytics"></a>`RenewableAnalytics`

| field | type | required |
|---|---|---|
| `average_generation_kwh` | [`Decimal`](#schema-decimal) |  |
| `consumption_optimisation` | string? |  |
| `data_availability_pct` | [`Decimal`](#schema-decimal) |  |
| `efficiency_change_30d_pct` | [`Decimal`](#schema-decimal) |  |
| `financial` | [`RenewableFinancial`](#schema-renewablefinancial) | yes |
| `maintenance_required` | string? |  |
| `peak_generation_at` | string (date-time)? |  |
| `peak_generation_kwh` | [`Decimal`](#schema-decimal) |  |
| `peak_hour` | integer? |  |
| `system_efficiency_pct` | [`Decimal`](#schema-decimal) |  |
| `trend` | [`RenewablePoint`](#schema-renewablepoint)[] | yes |
| `unavailable` | [`Unavailable`](#schema-unavailable) | yes |

### <a id="schema-renewableefficiency"></a>`RenewableEfficiency`

| field | type | required |
|---|---|---|
| `battery_pct` | [`Decimal`](#schema-decimal) |  |
| `grid_pct` | [`Decimal`](#schema-decimal) |  |
| `inverter_pct` | [`Decimal`](#schema-decimal) |  |
| `overall_pct` | [`Decimal`](#schema-decimal) |  |
| `panel_pct` | [`Decimal`](#schema-decimal) |  |
| `recommendations` | string[] | yes |
| `trend` | [`RenewablePoint`](#schema-renewablepoint)[] | yes |
| `unavailable` | [`Unavailable`](#schema-unavailable) | yes |

### <a id="schema-renewableenvironmental"></a>`RenewableEnvironmental`

| field | type | required |
|---|---|---|
| `car_km` | [`Decimal`](#schema-decimal) |  |
| `co2_avoided_kg` | [`Decimal`](#schema-decimal) |  |
| `coal_kg` | [`Decimal`](#schema-decimal) |  |
| `factors` | [`EquivalenceFactor`](#schema-equivalencefactor)[] | yes |
| `generation_kwh` | [`Decimal`](#schema-decimal) |  |
| `grid_factor` | [`Decimal`](#schema-decimal) |  |
| `grid_factor_source` | string? |  |
| `grid_factor_unit` | string? |  |
| `homes` | [`Decimal`](#schema-decimal) |  |
| `trees` | [`Decimal`](#schema-decimal) |  |
| `unavailable` | [`Unavailable`](#schema-unavailable) | yes |

### <a id="schema-renewablefinancial"></a>`RenewableFinancial`

| field | type | required |
|---|---|---|
| `bill_savings` | [`Decimal`](#schema-decimal) |  |
| `currency` | string? |  |
| `export_price` | [`Decimal`](#schema-decimal) |  |
| `import_price` | [`Decimal`](#schema-decimal) |  |
| `month_earnings` | [`Decimal`](#schema-decimal) |  |
| `net_today` | [`Decimal`](#schema-decimal) |  |
| `payback_years` | [`Decimal`](#schema-decimal) |  |
| `roi_pct` | [`Decimal`](#schema-decimal) |  |
| `today_export_revenue` | [`Decimal`](#schema-decimal) |  |
| `today_import_cost` | [`Decimal`](#schema-decimal) |  |
| `total_savings` | [`Decimal`](#schema-decimal) |  |
| `unavailable` | [`Unavailable`](#schema-unavailable) | yes |
| `year_earnings` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-renewableforecast"></a>`RenewableForecast`

| field | type | required |
|---|---|---|
| `accuracy_daily_pct` | [`Decimal`](#schema-decimal) |  |
| `accuracy_overall_pct` | [`Decimal`](#schema-decimal) |  |
| `accuracy_weekly_pct` | [`Decimal`](#schema-decimal) |  |
| `consumption_next_24h_kwh` | [`Decimal`](#schema-decimal) |  |
| `consumption_next_28d_kwh` | [`Decimal`](#schema-decimal) |  |
| `consumption_next_7d_kwh` | [`Decimal`](#schema-decimal) |  |
| `estimated_generation_kwh` | [`Decimal`](#schema-decimal) |  |
| `net_excess_kwh` | [`Decimal`](#schema-decimal) |  |
| `unavailable` | [`Unavailable`](#schema-unavailable) | yes |
| `weather_impact` | string? |  |

### <a id="schema-renewablegridinteraction"></a>`RenewableGridInteraction`

| field | type | required |
|---|---|---|
| `currency` | string? |  |
| `direction` | string? |  |
| `export_price` | [`Decimal`](#schema-decimal) |  |
| `frequency_hz` | [`Decimal`](#schema-decimal) |  |
| `import_price` | [`Decimal`](#schema-decimal) |  |
| `net_today` | [`Decimal`](#schema-decimal) |  |
| `power_factor` | [`Decimal`](#schema-decimal) |  |
| `today_export_kwh` | [`Decimal`](#schema-decimal) |  |
| `today_import_kwh` | [`Decimal`](#schema-decimal) |  |
| `unavailable` | [`Unavailable`](#schema-unavailable) | yes |
| `voltage_v` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-renewableoverview"></a>`RenewableOverview`

| field | type | required |
|---|---|---|
| `active_generation_kwh` | [`Decimal`](#schema-decimal) |  |
| `average_generation_kwh` | [`Decimal`](#schema-decimal) |  |
| `capacitive_generation_kvarh` | [`Decimal`](#schema-decimal) |  |
| `inductive_generation_kvarh` | [`Decimal`](#schema-decimal) |  |
| `unavailable` | [`Unavailable`](#schema-unavailable) | yes |

### <a id="schema-renewablepoint"></a>`RenewablePoint`

| field | type | required |
|---|---|---|
| `kwh` | [`Decimal`](#schema-decimal) |  |
| `ts` | string (date-time) | yes |

### <a id="schema-renewablerealtime"></a>`RenewableRealtime`

| field | type | required |
|---|---|---|
| `avg_power_kw` | [`Decimal`](#schema-decimal) |  |
| `current_power_kw` | [`Decimal`](#schema-decimal) |  |
| `max_power_kw` | [`Decimal`](#schema-decimal) |  |
| `series_24h` | [`RenewablePoint`](#schema-renewablepoint)[] | yes |
| `status` | string: producing \| idle \| no_data? |  |
| `system_efficiency_pct` | [`Decimal`](#schema-decimal) |  |
| `today_kwh` | [`Decimal`](#schema-decimal) |  |
| `unavailable` | [`Unavailable`](#schema-unavailable) | yes |

### <a id="schema-renewablesystemstatus"></a>`RenewableSystemStatus`

| field | type | required |
|---|---|---|
| `average_efficiency_pct` | [`Decimal`](#schema-decimal) |  |
| `battery` | string? |  |
| `grid_connection` | string? |  |
| `inverter` | string? |  |
| `last_reading_at` | string (date-time)? |  |
| `monitoring` | string? |  |
| `overall` | string? |  |
| `security` | string? |  |
| `solar_panels` | string? |  |
| `total_generation_kwh` | [`Decimal`](#schema-decimal) |  |
| `unavailable` | [`Unavailable`](#schema-unavailable) | yes |

### <a id="schema-report"></a>`Report`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` | yes |
| `building_name` | string | yes |
| `created_at` | string (date-time) | yes |
| `id` | `UuidUUID` | yes |
| `payload` | [`ReportPayload`](#schema-reportpayload) |  |
| `period` | string | yes |
| `plant_selection` | string: all \| grid \| rooftop | yes |
| `processed_at` | string (date-time)? |  |
| `status` | string: pending \| completed \| error | yes |
| `type` | string: monthly \| yearly | yes |

### <a id="schema-reportbuildingline"></a>`ReportBuildingLine`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` | yes |
| `currency` | string? |  |
| `name` | string | yes |
| `purchase_price` | [`Decimal`](#schema-decimal) |  |
| `tariff_name` | string? |  |

### <a id="schema-reportcarbon"></a>`ReportCarbon`

| field | type | required |
|---|---|---|
| `consumption_t` | [`Decimal`](#schema-decimal) |  |
| `factor` | [`Decimal`](#schema-decimal) | yes |
| `factor_unit` | string | yes |
| `net_t` | [`Decimal`](#schema-decimal) |  |
| `reduction_t` | [`Decimal`](#schema-decimal) |  |
| `source_year` | integer? |  |

### <a id="schema-reportcurrencydelta"></a>`ReportCurrencyDelta`

| field | type | required |
|---|---|---|
| `currency` | string | yes |
| `pct` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-reportdelta"></a>`ReportDelta`

| field | type | required |
|---|---|---|
| `pct` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-reportemailaccepted"></a>`ReportEmailAccepted`

| field | type | required |
|---|---|---|
| `job_id` | string | yes |

### <a id="schema-reportemailrequest"></a>`ReportEmailRequest`

| field | type | required |
|---|---|---|
| `to` | string[] | yes |

### <a id="schema-reportfigure"></a>`ReportFigure`

| field | type | required |
|---|---|---|
| `excluded` | boolean |  |
| `of` | integer | yes |
| `partial` | boolean |  |
| `value` | [`Decimal`](#schema-decimal) |  |
| `with_data` | integer | yes |

### <a id="schema-reportgenerateaccepted"></a>`ReportGenerateAccepted`

| field | type | required |
|---|---|---|
| `items` | [`ReportGenerateItem`](#schema-reportgenerateitem)[] | yes |

### <a id="schema-reportgenerateitem"></a>`ReportGenerateItem`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` | yes |
| `job_id` | string | yes |
| `report_id` | `UuidUUID` | yes |

### <a id="schema-reportgeneraterequest"></a>`ReportGenerateRequest`

| field | type | required |
|---|---|---|
| `building_ids` | `UuidUUID`[] | yes |
| `period` | string | yes |
| `plant_ids` | `UuidUUID`[] |  |
| `plant_selection` | string: all \| grid \| rooftop | yes |
| `type` | string: monthly \| yearly | yes |

### <a id="schema-reportmoney"></a>`ReportMoney`

| field | type | required |
|---|---|---|
| `currency` | string | yes |
| `of` | integer | yes |
| `value` | [`Decimal`](#schema-decimal) | yes |
| `with_data` | integer | yes |

### <a id="schema-reportmonthpoint"></a>`ReportMonthPoint`

| field | type | required |
|---|---|---|
| `current` | [`Decimal`](#schema-decimal) |  |
| `month` | integer | yes |
| `previous` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-reportmonthly"></a>`ReportMonthly`

| field | type | required |
|---|---|---|
| `average_purchase_price` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `bill` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `bill_chart` | [`ReportMonthPoint`](#schema-reportmonthpoint)[] | yes |
| `bill_delta` | [`ReportCurrencyDelta`](#schema-reportcurrencydelta)[] | yes |
| `bill_prev` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `buildings` | [`ReportBuildingLine`](#schema-reportbuildingline)[] | yes |
| `capacitive` | [`ReportFigure`](#schema-reportfigure) | yes |
| `capacitive_ratio` | [`Decimal`](#schema-decimal) |  |
| `chart_currency` | string? |  |
| `consumption` | [`ReportFigure`](#schema-reportfigure) | yes |
| `consumption_chart` | [`ReportMonthPoint`](#schema-reportmonthpoint)[] | yes |
| `consumption_delta` | [`ReportDelta`](#schema-reportdelta) | yes |
| `consumption_prev` | [`ReportFigure`](#schema-reportfigure) | yes |
| `daily_consumption` | [`ReportFigure`](#schema-reportfigure) | yes |
| `daily_production` | [`ReportFigure`](#schema-reportfigure) | yes |
| `days_in_month` | integer | yes |
| `distribution_cost` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `energy_cost` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `inductive` | [`ReportFigure`](#schema-reportfigure) | yes |
| `inductive_ratio` | [`Decimal`](#schema-decimal) |  |
| `month` | integer | yes |
| `omitted_currencies` | string[] | yes |
| `partial` | boolean | yes |
| `plant_selection` | string | yes |
| `plants` | [`ReportPlantLine`](#schema-reportplantline)[] | yes |
| `production` | [`ReportFigure`](#schema-reportfigure) | yes |
| `production_delta` | [`ReportDelta`](#schema-reportdelta) | yes |
| `production_prev` | [`ReportFigure`](#schema-reportfigure) | yes |
| `reactive_penalty` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `reactive_penalty_delta` | [`ReportCurrencyDelta`](#schema-reportcurrencydelta)[] | yes |
| `reactive_penalty_prev` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `rooftop` | [`ReportFigure`](#schema-reportfigure) | yes |
| `rooftop_feed_in` | [`ReportPriceRange`](#schema-reportpricerange) | yes |
| `rooftop_prev` | [`ReportFigure`](#schema-reportfigure) | yes |
| `t1` | [`ReportFigure`](#schema-reportfigure) | yes |
| `t2` | [`ReportFigure`](#schema-reportfigure) | yes |
| `t3` | [`ReportFigure`](#schema-reportfigure) | yes |
| `taxes` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `utility` | [`ReportFigure`](#schema-reportfigure) | yes |
| `utility_feed_in` | [`ReportPriceRange`](#schema-reportpricerange) | yes |
| `utility_prev` | [`ReportFigure`](#schema-reportfigure) | yes |
| `year` | integer | yes |

### <a id="schema-reportpage"></a>`ReportPage`

| field | type | required |
|---|---|---|
| `items` | [`ReportSummary`](#schema-reportsummary)[] | yes |
| `next_cursor` | string? |  |
| `total` | integer (int64) | yes |

### <a id="schema-reportpayload"></a>`ReportPayload`

| field | type | required |
|---|---|---|
| `monthly` | [`ReportMonthly`](#schema-reportmonthly) |  |
| `period` | string | yes |
| `type` | string: monthly \| yearly | yes |
| `version` | integer | yes |
| `yearly` | [`ReportYearly`](#schema-reportyearly) |  |

### <a id="schema-reportplantline"></a>`ReportPlantLine`

| field | type | required |
|---|---|---|
| `achievement_pct` | [`Decimal`](#schema-decimal) |  |
| `feed_in` | [`Decimal`](#schema-decimal) |  |
| `kind` | string | yes |
| `name` | string | yes |
| `plant_id` | `UuidUUID` | yes |
| `production` | [`ReportFigure`](#schema-reportfigure) | yes |
| `target` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-reportpricerange"></a>`ReportPriceRange`

| field | type | required |
|---|---|---|
| `max` | [`Decimal`](#schema-decimal) |  |
| `min` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-reportsummary"></a>`ReportSummary`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` | yes |
| `building_name` | string | yes |
| `created_at` | string (date-time) | yes |
| `id` | `UuidUUID` | yes |
| `period` | string | yes |
| `plant_selection` | string: all \| grid \| rooftop | yes |
| `processed_at` | string (date-time)? |  |
| `status` | string: pending \| completed \| error | yes |
| `type` | string: monthly \| yearly | yes |

### <a id="schema-reportyearmonthrow"></a>`ReportYearMonthRow`

| field | type | required |
|---|---|---|
| `bill` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `consumption` | [`ReportFigure`](#schema-reportfigure) | yes |
| `month` | integer | yes |
| `reactive_penalty` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `rooftop` | [`ReportFigure`](#schema-reportfigure) | yes |

### <a id="schema-reportyearpoint"></a>`ReportYearPoint`

| field | type | required |
|---|---|---|
| `bill` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `consumption` | [`Decimal`](#schema-decimal) |  |
| `production` | [`Decimal`](#schema-decimal) |  |
| `year` | integer | yes |

### <a id="schema-reportyearly"></a>`ReportYearly`

| field | type | required |
|---|---|---|
| `achievement_pct` | [`Decimal`](#schema-decimal) |  |
| `bill` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `bill_delta` | [`ReportCurrencyDelta`](#schema-reportcurrencydelta)[] | yes |
| `buildings` | [`ReportBuildingLine`](#schema-reportbuildingline)[] | yes |
| `carbon` | [`ReportCarbon`](#schema-reportcarbon) |  |
| `carbon_reason` | string |  |
| `chart_currency` | string? |  |
| `consumption` | [`ReportFigure`](#schema-reportfigure) | yes |
| `consumption_delta` | [`ReportDelta`](#schema-reportdelta) | yes |
| `daily_consumption` | [`ReportFigure`](#schema-reportfigure) | yes |
| `daily_production` | [`ReportFigure`](#schema-reportfigure) | yes |
| `daily_rooftop` | [`ReportFigure`](#schema-reportfigure) | yes |
| `daily_utility` | [`ReportFigure`](#schema-reportfigure) | yes |
| `grid_share_pct` | [`Decimal`](#schema-decimal) |  |
| `history` | [`ReportYearPoint`](#schema-reportyearpoint)[] | yes |
| `months` | [`ReportYearMonthRow`](#schema-reportyearmonthrow)[] | yes |
| `omitted_currencies` | string[] | yes |
| `partial` | boolean | yes |
| `plant_selection` | string | yes |
| `plants` | [`ReportPlantLine`](#schema-reportplantline)[] | yes |
| `production` | [`ReportFigure`](#schema-reportfigure) | yes |
| `reactive_penalty` | [`ReportMoney`](#schema-reportmoney)[] | yes |
| `rooftop` | [`ReportFigure`](#schema-reportfigure) | yes |
| `solar_share_pct` | [`Decimal`](#schema-decimal) |  |
| `target` | [`Decimal`](#schema-decimal) |  |
| `utility` | [`ReportFigure`](#schema-reportfigure) | yes |
| `year` | integer | yes |

### <a id="schema-resetpasswordrequest"></a>`ResetPasswordRequest`

| field | type | required |
|---|---|---|
| `password` | string | yes |
| `token` | string | yes |

### <a id="schema-revenueperiod"></a>`RevenuePeriod`

| field | type | required |
|---|---|---|
| `amounts` | [`MoneyAmount`](#schema-moneyamount)[] | yes |
| `partial` | boolean | yes |
| `since` | [`Date`](#schema-date) |  |
| `unpriced_days` | integer | yes |

### <a id="schema-role"></a>`Role`

string: admin \| company_admin \| company_readonly_admin \| building_admin \| building_readonly_admin \| demo

### <a id="schema-smtpputrequest"></a>`SMTPPutRequest`

| field | type | required |
|---|---|---|
| `from_address` | string (email) | yes |
| `host` | string | yes |
| `password` | string |  |
| `port` | integer (int32) | yes |
| `secure` | boolean |  |
| `username` | string |  |

### <a id="schema-smtpsettings"></a>`SMTPSettings`

| field | type | required |
|---|---|---|
| `from_address` | string | yes |
| `has_password` | boolean | yes |
| `host` | string | yes |
| `port` | integer (int32) | yes |
| `secure` | boolean | yes |
| `updated_at` | string (date-time) | yes |
| `username` | string | yes |

### <a id="schema-smtptestrequest"></a>`SMTPTestRequest`

| field | type | required |
|---|---|---|
| `to` | string (email) | yes |

### <a id="schema-session"></a>`Session`

| field | type | required |
|---|---|---|
| `client` | string: web \| mobile | yes |
| `created_at` | string (date-time) | yes |
| `current` | boolean | yes |
| `expires_at` | string (date-time) | yes |
| `id` | `UuidUUID` | yes |
| `ip` | string? |  |
| `last_used_at` | string (date-time)? |  |
| `user_agent` | string? |  |

### <a id="schema-sessionlist"></a>`SessionList`

| field | type | required |
|---|---|---|
| `items` | [`Session`](#schema-session)[] | yes |

### <a id="schema-solartariff"></a>`SolarTariff`

| field | type | required |
|---|---|---|
| `created_at` | string (date-time) | yes |
| `currency` | string: TRY \| USD \| EUR |  |
| `effective_from` | [`Date`](#schema-date) | yes |
| `feed_in_tariff` | [`Decimal`](#schema-decimal) | yes |
| `id` | `UuidUUID` | yes |
| `notes` | string? |  |
| `plant_id` | `UuidUUID` | yes |
| `purchase_price` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-solartarifffields"></a>`SolarTariffFields`

| field | type | required |
|---|---|---|
| `currency` | string: TRY \| USD \| EUR |  |
| `effective_from` | [`Date`](#schema-date) | yes |
| `feed_in_tariff` | [`Decimal`](#schema-decimal) | yes |
| `notes` | string? |  |
| `plant_id` | `UuidUUID` | yes |
| `purchase_price` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-solartariffpage"></a>`SolarTariffPage`

| field | type | required |
|---|---|---|
| `items` | [`SolarTariff`](#schema-solartariff)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-tariff"></a>`Tariff`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` |  |
| `contracted_power_kw` | [`Decimal`](#schema-decimal) |  |
| `created_at` | string (date-time) | yes |
| `currency` | string: TRY \| USD \| EUR | yes |
| `distribution_cost` | [`Decimal`](#schema-decimal) | yes |
| `distribution_price_source` | string: kbk \| fixed |  |
| `effective_from` | [`Date`](#schema-date) | yes |
| `energy_type` | string: grid_energy \| green_energy | yes |
| `extra_charges` | [`TariffExtraCharge`](#schema-tariffextracharge)[] |  |
| `generation_price_per_kwh` | [`Decimal`](#schema-decimal) |  |
| `generation_usage` | string: none \| subtract_from_consumption \| subtract_from_total | yes |
| `green_energy_distribution_cost` | [`Decimal`](#schema-decimal) |  |
| `green_energy_price` | [`Decimal`](#schema-decimal) |  |
| `id` | `UuidUUID` | yes |
| `kbk_distribution_cost_tl_per_kwh` | [`Decimal`](#schema-decimal) |  |
| `kbk_energy` | [`Decimal`](#schema-decimal) |  |
| `kbk_overuse_price` | [`Decimal`](#schema-decimal) |  |
| `kbk_power_price` | [`Decimal`](#schema-decimal) |  |
| `kbk_reactive_power` | [`Decimal`](#schema-decimal) |  |
| `kbk_t1` | [`Decimal`](#schema-decimal) |  |
| `kbk_t2` | [`Decimal`](#schema-decimal) |  |
| `kbk_t3` | [`Decimal`](#schema-decimal) |  |
| `manual_yekdem` | [`TariffManualYekdem`](#schema-tariffmanualyekdem)[] |  |
| `name` | string? |  |
| `overuse_price` | [`Decimal`](#schema-decimal) |  |
| `overuse_threshold_kwh_per_day` | [`Decimal`](#schema-decimal) |  |
| `power_price_source` | string: kbk \| fixed |  |
| `power_unit_price` | [`Decimal`](#schema-decimal) |  |
| `price_type` | string: single_time \| multi_time | yes |
| `reactive_power_price` | [`Decimal`](#schema-decimal) | yes |
| `reactive_price_source` | string: kbk \| fixed |  |
| `single_time_price` | [`Decimal`](#schema-decimal) |  |
| `supply_company` | string: incumbent \| private | yes |
| `t1_price` | [`Decimal`](#schema-decimal) |  |
| `t2_price` | [`Decimal`](#schema-decimal) |  |
| `t3_price` | [`Decimal`](#schema-decimal) |  |
| `taxes` | [`TariffTax`](#schema-tarifftax)[] |  |
| `term` | string: monomial \| binomial | yes |
| `updated_at` | string (date-time) | yes |
| `use_manual_yekdem` | boolean |  |
| `use_ptf_yekdem` | boolean |  |
| `user_group` | string: residential \| residential_plus \| commercial \| commercial_plus \| industrial \| agricultural \| lighting \| martyrs_families \| public_lighting | yes |
| `vat_rate` | [`Decimal`](#schema-decimal) | yes |
| `voltage_level` | string: lv \| mv | yes |

### <a id="schema-tariffextracharge"></a>`TariffExtraCharge`

| field | type | required |
|---|---|---|
| `amount` | [`Decimal`](#schema-decimal) | yes |
| `basis` | string: per_kwh \| per_contracted_kw \| per_max_demand_kw \| fixed_per_period \| pct_of_energy | yes |
| `name` | string | yes |

### <a id="schema-tarifffields"></a>`TariffFields`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` |  |
| `contracted_power_kw` | [`Decimal`](#schema-decimal) |  |
| `currency` | string: TRY \| USD \| EUR | yes |
| `distribution_cost` | [`Decimal`](#schema-decimal) | yes |
| `distribution_price_source` | string: kbk \| fixed |  |
| `effective_from` | [`Date`](#schema-date) | yes |
| `energy_type` | string: grid_energy \| green_energy | yes |
| `extra_charges` | [`TariffExtraCharge`](#schema-tariffextracharge)[] |  |
| `generation_price_per_kwh` | [`Decimal`](#schema-decimal) |  |
| `generation_usage` | string: none \| subtract_from_consumption \| subtract_from_total | yes |
| `green_energy_distribution_cost` | [`Decimal`](#schema-decimal) |  |
| `green_energy_price` | [`Decimal`](#schema-decimal) |  |
| `kbk_distribution_cost_tl_per_kwh` | [`Decimal`](#schema-decimal) |  |
| `kbk_energy` | [`Decimal`](#schema-decimal) |  |
| `kbk_overuse_price` | [`Decimal`](#schema-decimal) |  |
| `kbk_power_price` | [`Decimal`](#schema-decimal) |  |
| `kbk_reactive_power` | [`Decimal`](#schema-decimal) |  |
| `kbk_t1` | [`Decimal`](#schema-decimal) |  |
| `kbk_t2` | [`Decimal`](#schema-decimal) |  |
| `kbk_t3` | [`Decimal`](#schema-decimal) |  |
| `manual_yekdem` | [`TariffManualYekdem`](#schema-tariffmanualyekdem)[] |  |
| `name` | string? |  |
| `overuse_price` | [`Decimal`](#schema-decimal) |  |
| `overuse_threshold_kwh_per_day` | [`Decimal`](#schema-decimal) |  |
| `power_price_source` | string: kbk \| fixed |  |
| `power_unit_price` | [`Decimal`](#schema-decimal) |  |
| `price_type` | string: single_time \| multi_time | yes |
| `reactive_power_price` | [`Decimal`](#schema-decimal) | yes |
| `reactive_price_source` | string: kbk \| fixed |  |
| `single_time_price` | [`Decimal`](#schema-decimal) |  |
| `supply_company` | string: incumbent \| private | yes |
| `t1_price` | [`Decimal`](#schema-decimal) |  |
| `t2_price` | [`Decimal`](#schema-decimal) |  |
| `t3_price` | [`Decimal`](#schema-decimal) |  |
| `taxes` | [`TariffTax`](#schema-tarifftax)[] |  |
| `term` | string: monomial \| binomial | yes |
| `use_manual_yekdem` | boolean |  |
| `use_ptf_yekdem` | boolean |  |
| `user_group` | string: residential \| residential_plus \| commercial \| commercial_plus \| industrial \| agricultural \| lighting \| martyrs_families \| public_lighting | yes |
| `vat_rate` | [`Decimal`](#schema-decimal) | yes |
| `voltage_level` | string: lv \| mv | yes |

### <a id="schema-tariffmanualyekdem"></a>`TariffManualYekdem`

| field | type | required |
|---|---|---|
| `month` | integer | yes |
| `value` | [`Decimal`](#schema-decimal) | yes |
| `year` | integer | yes |

### <a id="schema-tariffsummary"></a>`TariffSummary`

| field | type | required |
|---|---|---|
| `effective_from` | [`Date`](#schema-date) | yes |
| `id` | `UuidUUID` | yes |
| `name` | string? |  |

### <a id="schema-tariffsummaryitem"></a>`TariffSummaryItem`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` |  |
| `effective_from` | [`Date`](#schema-date) | yes |
| `id` | `UuidUUID` | yes |
| `name` | string? |  |
| `price_type` | string | yes |
| `term` | string | yes |
| `use_ptf_yekdem` | boolean | yes |

### <a id="schema-tariffsummaryitempage"></a>`TariffSummaryItemPage`

| field | type | required |
|---|---|---|
| `items` | [`TariffSummaryItem`](#schema-tariffsummaryitem)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-tarifftax"></a>`TariffTax`

| field | type | required |
|---|---|---|
| `name` | string | yes |
| `rate` | [`Decimal`](#schema-decimal) | yes |

### <a id="schema-tarifftemplate"></a>`TariffTemplate`

| field | type | required |
|---|---|---|
| `created_at` | string (date-time) | yes |
| `description` | string? |  |
| `id` | `UuidUUID` | yes |
| `is_default` | boolean |  |
| `name` | string | yes |
| `tariff` | [`TariffFields`](#schema-tarifffields) | yes |
| `updated_at` | string (date-time) | yes |

### <a id="schema-tarifftemplateapplyrequest"></a>`TariffTemplateApplyRequest`

| field | type | required |
|---|---|---|
| `building_ids` | `UuidUUID`[] | yes |
| `effective_from` | [`Date`](#schema-date) | yes |

### <a id="schema-tarifftemplatefields"></a>`TariffTemplateFields`

| field | type | required |
|---|---|---|
| `description` | string? |  |
| `is_default` | boolean |  |
| `name` | string | yes |
| `tariff` | [`TariffFields`](#schema-tarifffields) | yes |

### <a id="schema-tarifftemplatepage"></a>`TariffTemplatePage`

| field | type | required |
|---|---|---|
| `items` | [`TariffTemplate`](#schema-tarifftemplate)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-tarifftemplateupdaterequest"></a>`TariffTemplateUpdateRequest`

| field | type | required |
|---|---|---|
| `description` | string? |  |
| `is_default` | boolean |  |
| `name` | string | yes |
| `tariff` | [`TariffFields`](#schema-tarifffields) | yes |

### <a id="schema-tariffupdaterequest"></a>`TariffUpdateRequest`

| field | type | required |
|---|---|---|
| `building_id` | `UuidUUID` |  |
| `contracted_power_kw` | [`Decimal`](#schema-decimal) |  |
| `currency` | string: TRY \| USD \| EUR | yes |
| `distribution_cost` | [`Decimal`](#schema-decimal) | yes |
| `distribution_price_source` | string: kbk \| fixed |  |
| `effective_from` | [`Date`](#schema-date) | yes |
| `energy_type` | string: grid_energy \| green_energy | yes |
| `extra_charges` | [`TariffExtraCharge`](#schema-tariffextracharge)[] |  |
| `generation_price_per_kwh` | [`Decimal`](#schema-decimal) |  |
| `generation_usage` | string: none \| subtract_from_consumption \| subtract_from_total | yes |
| `green_energy_distribution_cost` | [`Decimal`](#schema-decimal) |  |
| `green_energy_price` | [`Decimal`](#schema-decimal) |  |
| `kbk_distribution_cost_tl_per_kwh` | [`Decimal`](#schema-decimal) |  |
| `kbk_energy` | [`Decimal`](#schema-decimal) |  |
| `kbk_overuse_price` | [`Decimal`](#schema-decimal) |  |
| `kbk_power_price` | [`Decimal`](#schema-decimal) |  |
| `kbk_reactive_power` | [`Decimal`](#schema-decimal) |  |
| `kbk_t1` | [`Decimal`](#schema-decimal) |  |
| `kbk_t2` | [`Decimal`](#schema-decimal) |  |
| `kbk_t3` | [`Decimal`](#schema-decimal) |  |
| `manual_yekdem` | [`TariffManualYekdem`](#schema-tariffmanualyekdem)[] |  |
| `name` | string? |  |
| `overuse_price` | [`Decimal`](#schema-decimal) |  |
| `overuse_threshold_kwh_per_day` | [`Decimal`](#schema-decimal) |  |
| `power_price_source` | string: kbk \| fixed |  |
| `power_unit_price` | [`Decimal`](#schema-decimal) |  |
| `price_type` | string: single_time \| multi_time | yes |
| `reactive_power_price` | [`Decimal`](#schema-decimal) | yes |
| `reactive_price_source` | string: kbk \| fixed |  |
| `single_time_price` | [`Decimal`](#schema-decimal) |  |
| `supply_company` | string: incumbent \| private | yes |
| `t1_price` | [`Decimal`](#schema-decimal) |  |
| `t2_price` | [`Decimal`](#schema-decimal) |  |
| `t3_price` | [`Decimal`](#schema-decimal) |  |
| `taxes` | [`TariffTax`](#schema-tarifftax)[] |  |
| `term` | string: monomial \| binomial | yes |
| `use_manual_yekdem` | boolean |  |
| `use_ptf_yekdem` | boolean |  |
| `user_group` | string: residential \| residential_plus \| commercial \| commercial_plus \| industrial \| agricultural \| lighting \| martyrs_families \| public_lighting | yes |
| `vat_rate` | [`Decimal`](#schema-decimal) | yes |
| `voltage_level` | string: lv \| mv | yes |

### <a id="schema-unavailable"></a>`Unavailable`

object

### <a id="schema-user"></a>`User`

| field | type | required |
|---|---|---|
| `created_at` | string (date-time) | yes |
| `email` | string | yes |
| `id` | `UuidUUID` | yes |
| `is_active` | boolean | yes |
| `last_login_at` | string (date-time)? |  |
| `locale` | [`Locale`](#schema-locale) | yes |
| `name` | string | yes |
| `phone` | string? |  |
| `role` | [`Role`](#schema-role) | yes |

### <a id="schema-usercreaterequest"></a>`UserCreateRequest`

| field | type | required |
|---|---|---|
| `email` | string (email) | yes |
| `is_active` | boolean? |  |
| `name` | string | yes |
| `password` | string | yes |
| `phone` | string? |  |
| `role` | [`Role`](#schema-role) | yes |

### <a id="schema-userpage"></a>`UserPage`

| field | type | required |
|---|---|---|
| `items` | [`User`](#schema-user)[] | yes |
| `next_cursor` | string? |  |

### <a id="schema-userupdaterequest"></a>`UserUpdateRequest`

| field | type | required |
|---|---|---|
| `email` | string (email)? |  |
| `is_active` | boolean? |  |
| `name` | string? |  |
| `phone` | string? |  |
| `role` | [`Role`](#schema-role) |  |

### <a id="schema-uuiduuid"></a>`UuidUUID`

string (uuid)

### <a id="schema-vacationperiod"></a>`VacationPeriod`

| field | type | required |
|---|---|---|
| `description` | string? |  |
| `end_date` | [`Date`](#schema-date) | yes |
| `id` | `UuidUUID` |  |
| `start_date` | [`Date`](#schema-date) | yes |

### <a id="schema-vacations"></a>`Vacations`

| field | type | required |
|---|---|---|
| `periods` | [`VacationPeriod`](#schema-vacationperiod)[] | yes |
| `weekend_days` | integer[] | yes |
| `weekend_source` | string: company \| default | yes |

### <a id="schema-vacationsputrequest"></a>`VacationsPutRequest`

| field | type | required |
|---|---|---|
| `periods` | [`VacationPeriod`](#schema-vacationperiod)[] |  |
| `weekend_days` | integer[] |  |

### <a id="schema-weather"></a>`Weather`

| field | type | required |
|---|---|---|
| `available` | boolean | yes |
| `current` | [`WeatherCurrent`](#schema-weathercurrent) |  |
| `days` | [`WeatherDay`](#schema-weatherday)[] | yes |
| `potential_basis` | string |  |
| `reason` | string: weather_not_configured \| location_not_configured \| weather_unavailable |  |

### <a id="schema-weathercurrent"></a>`WeatherCurrent`

| field | type | required |
|---|---|---|
| `humidity_pct` | [`Decimal`](#schema-decimal) |  |
| `precipitation_pct` | [`Decimal`](#schema-decimal) |  |
| `pressure_hpa` | [`Decimal`](#schema-decimal) |  |
| `temperature_c` | [`Decimal`](#schema-decimal) |  |
| `uv_index` | [`Decimal`](#schema-decimal) |  |
| `visibility_km` | [`Decimal`](#schema-decimal) |  |
| `weather_code` | integer? |  |
| `wind_kmh` | [`Decimal`](#schema-decimal) |  |

### <a id="schema-weatherday"></a>`WeatherDay`

| field | type | required |
|---|---|---|
| `date` | [`Date`](#schema-date) | yes |
| `max_c` | [`Decimal`](#schema-decimal) |  |
| `min_c` | [`Decimal`](#schema-decimal) |  |
| `potential` | string: high \| medium \| low? |  |
| `precipitation_pct` | [`Decimal`](#schema-decimal) |  |
| `shortwave_mj_m2` | [`Decimal`](#schema-decimal) |  |
| `uv_index_max` | [`Decimal`](#schema-decimal) |  |
| `weather_code` | integer? |  |

