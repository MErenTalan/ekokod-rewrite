# Appendix — Legacy Inventory

A complete listing of what existed in the legacy system, and where each item lands in the rewrite.
Use this as a coverage checklist: when a module is finished, confirm every row that maps to it is
accounted for.

---

## 1. Application routes

### Real product screens — all must exist in the rewrite

| Legacy route | New route | Phase |
|--------------|-----------|-------|
| `/` (dashboard) | `/ekorm` | F6 |
| `/consumption` | `/ekorm/consumption` | F6 |
| `/load-profile` | `/ekorm/load-profile` | F6 |
| `/predict` | `/ekorm/predict` | F13 |
| `/ai` | `/ekorm/ai` | F13 |
| `/solar-plants` | `/ekorm/solar-plants` | F9 |
| `/renewable-energy` | `/ekorm/renewable-energy` | F9 |
| `/financial-analysis` | `/ekorm/financial-analysis` | F9 |
| `/bills` | `/ekorm/bills` | F8 |
| `/bills-and-tariffs` | merged into `/ekorm/bills` and `/ekorm/tariffs` | F8 |
| `/tariffs` | `/ekorm/tariffs` | F8 |
| `/alarms` | `/ekorm/alarms` | F7 |
| `/messages` | `/ekorm/messages` | F7 |
| `/reports` | `/ekorm/reports` | F8 |
| `/settings/account` | `/ekorm/settings` | F6 |
| `/carbon-footprint` | `/ekorm/carbon-footprint` | F10 |
| `/iso-50001` | `/ekorm/iso-50001` | F11 |
| `/calendar` | `/ekorm/calendar` | F6 |
| `/auth/login` | `/auth/login` | F6 |
| `/auth/forgot-password` | `/auth/forgot-password` | F6 |
| `/auth/two-steps` | `/auth/two-steps` | F6 |
| `/auth/error` | `/auth/error` | F6 |
| `/auth/maintenance` | `/auth/maintenance` | F6 |
| `/auth/register` | `/auth/register` (flag-disabled) | F6 |
| `/site/homepage` | `/` | F12 |
| `/site/about` | `/about` | F12 |
| `/site/references` | `/references` | F12 |
| `/site/documents` | `/documents` | F12 |
| `/site/toolkit` | `/toolkit` | F12 |
| `/site/pricing` | `/pricing` (flag-disabled) | F12 |
| `/site/request-demo` | `/request-demo` | F12 |
| `/site/contact` | `/contact` | F12 |
| `/site/blog` | `/blog` | F12 |
| `/site/blog/detail/[slug]` | `/blog/[slug]` | F12 |
| `/site/billCalculate` | `/bill-calculator` | F12 |
| `/ekocm/*` (separate application) | `/ekorm/carbon-footprint` | F10 |

### Template demo routes — deliberately dropped

These are Flowbite admin-template leftovers with no business function. Confirmed with the product
owner as out of scope.

`/ui-components/*` (33 pages) · `/headless-ui/*` (6) · `/headless-form/*` (10) ·
`/react-tables/*` (12) · `/tables/*` (4) · `/forms/*` (6) · `/charts/*` (7) · `/icons/*` (2) ·
`/widgets/*` (3) · `/theme-pages/faq` · `/theme-pages/pricing` · `/theme-pages/casl` ·
`/theme-pages/account-settings` (superseded by `/ekorm/settings`) · `/landingpage` ·
`/site/blog2` · `/site/blog/post` · demo `apps/*` components (calendar demo, email, contacts,
blog demo)

---

## 2. API endpoints

| Legacy endpoint | New endpoint | Phase |
|-----------------|--------------|-------|
| `POST /api/auth/[...nextauth]` | `/api/v1/auth/*` | F6 |
| `POST /api/register` | `/api/v1/auth/register` (flagged) | F6 |
| `GET/POST /api/mobile/auth/login`, `/me` | `/api/v1/mobile/auth/*` | F6 |
| `/api/users` | `/api/v1/users` | F6 |
| `/api/company` | `/api/v1/companies` | F6 |
| `/api/company/events` | `/api/v1/calendar/events` | F6 |
| `/api/company/vacations` | `/api/v1/calendar/vacations` | F6 |
| `/api/building` | `/api/v1/buildings` | F6 |
| `/api/building-comparison` | `/api/v1/buildings/{id}/comparison` | F6 |
| `/api/analyzer` | `/api/v1/analyzers` | F6 |
| `/api/consumption` | `/api/v1/consumption` | F3 |
| `/api/load-profile` | `/api/v1/load-profile` | F3 |
| `/api/bill` | `/api/v1/bills` | F4 |
| `/api/bill/building` | `/api/v1/bills` (scope=building) | F4 |
| `/api/bill/company` | `/api/v1/bills` (scope=company) | F4 |
| `/api/bill/hourly-details` | `/api/v1/bills/{id}/hourly-detail` | F4 |
| `/api/bill/demo` | demo dataset served by `/api/v1/bills` | F6 |
| `/api/tariff` | `/api/v1/tariffs` | F4 |
| `/api/tariff-templates` | `/api/v1/tariff-templates` | F8 |
| `/api/tariffs/import-icmal` | `/api/v1/icmal-imports` | F8 |
| `/api/buildings/bulk-tariff`, `/current`, `/history` | `/api/v1/buildings/bulk-tariff*` | F8 |
| `/api/powerplant`, `/data`, `/tariff` | `/api/v1/power-plants*`, `/plants/{id}/production`, `/solar-tariffs` | F9 |
| `/api/alarm` | `/api/v1/alarms` | F7 |
| `/api/messages` | `/api/v1/messages` | F7 |
| `/api/reports`, `/archive`, `/export/pdf`, `/send-email`, `/download/excel/[id]` | `/api/v1/reports*` | F8 |
| `/api/carbon-footprint` and sub-routes | `/api/v1/carbon/*` | F10 |
| `/api/iso/*` (files, notes, projectDates, projectFiles, downloadProjectFiles) | `/api/v1/iso50001/*` | F11 |
| `/api/integration`, `/setup` | `/api/v1/integration-definitions`, `/integration-credentials` | F2 |
| `/api/integration/osos/setup`, `/refresh` | credential + `discover` + `backfill` | F2 |
| `/api/integration/gridbox/setup`, `/refresh` | same | F2 |
| `/api/integration/aril/setup`, `/refresh` | same | F2 |
| `/api/integration/pm5340/setup`, `/refresh` | same | F2 |
| `/api/integration/isolar/*` (setup, callback, status, plants, devices, realtime, historical, historical/export, alarms, alarm-emails) | `/api/v1/plants/*` and `/integrations/isolar/*` | F9 |
| `/api/epias` | `/api/v1/market-prices` (internal) + the sync job | F2 |
| `/api/predict`, `/predict-hourly`, `/predict-weekly`, `/predict-monthly`, `/ai-predict`, `/ai/predict` | `/api/v1/forecast*` | F13 |
| `/api/check-anomaly`, `/ai/check-alarm` | `/api/v1/anomaly/check` | F13 |
| `/api/smtp` | `/api/v1/smtp-settings` | F6 |
| `/api/settings/account`, `/update-password` | `/api/v1/profile`, `/auth/change-password` | F6 |
| `/api/contact` | `/api/v1/public/contact` | F12 |
| `/api/request-demo` | `/api/v1/public/demo-request` | F12 |
| `/api/health` | `/health/live`, `/health/ready` | F0 |
| `/api/seed-admin` | `bcem seed --admin` CLI | F1 |
| `/api/cron/*` (7 endpoints) | **removed** — replaced by in-process jobs | F0/F2 |

---

## 3. Database collections

| Legacy collection | Destination | Notes |
|-------------------|-------------|-------|
| `companies` | `companies`, `integration_credentials`, `company_weekend_days`, `company_vacations`, `calendar_events` | Embedded arrays exploded into tables |
| `users` | `users`, `user_password_history` | |
| `buildings` | `buildings`, `building_contacts`, `tariffs`, `tariff_taxes`, `tariff_manual_yekdem` | `billHistory` → `legacy_bills` |
| `analyzers` | `analyzers`, `meter_readings` | `energyValues.*`, `hourlyValues` and `billHistory` all exploded |
| `consumptions` | **discarded** | Replaced by continuous aggregates |
| `tariffs` | `tariffs`, `national_tariff_schedule` | |
| `tarifftemplates` | `tariff_templates` | |
| `powerplants` | `power_plants`, `power_plant_monthly_targets`, `power_plant_devices`, `power_plant_alarm_recipients`, `solar_tariffs`, `plant_production` | |
| `alarms` | `alarms`, `alarm_analyzers`, `alarm_channels`, `alarm_events`, `alarm_fired_bills` | |
| `reports` | `legacy_reports`; **recomputed** into `reports` | |
| `carbonfootprint` | `carbon_activities`, `carbon_selected_activities` | |
| `carbonreporthistories` | `carbon_reports` + `stored_files` | |
| `companyemissionfactors` | `emission_factors`, `emission_factor_conversions` | |
| `epiashistories` | `market_prices_hourly`, `yekdem_monthly` | |
| `integrations` | `integration_definitions` | |
| `smtpsettings` | `smtp_settings` | |
| `logs` | `operational_messages` | Retained for a configurable window |
| `aipredict` | **discarded** | Regenerated |

---

## 4. Scheduled jobs

| Legacy cron entry | Schedule | New job | Phase |
|-------------------|----------|---------|-------|
| `GET /api/cron/refresh-analyzers` | `0 3 * * *` | `integration.sync_analyzers` → `integration.fetch_readings` | F2 |
| `GET /api/cron/calculate-consumptions` | on demand | `consumption.refresh` | F3 |
| `GET /api/cron/generate-bills` | `0 5 * * *` | `billing.generate` → `billing.render_pdf` | F4 |
| `GET /api/cron/alarm-check` | `0 * * * *` | `alarm.evaluate` → `alarm.notify` | F7 |
| `GET /api/cron/generate-reports` | monthly/yearly | `report.generate` → `report.deliver` | F8 |
| `GET /api/cron/ai-predict` | daily | `forecast.run` | F13 |
| `GET /api/cron/daily-carbon` | daily | `carbon.daily_accrual` | F10 |
| *(none — was synchronous)* | — | `epias.sync_prices` | F2 |
| *(none)* | — | `isolar.sync_plant`, `isolar.fetch_alarms` | F9 |

---

## 5. Integrations

| Provider | Subtypes in production | Phase |
|----------|------------------------|-------|
| OSOS | `Baskent`, `Aydem`, `Meramedas`, plus any configured per deployment | F2 |
| GridBox | per deployment | F2 |
| ARIL | per deployment | F2 |
| PM5340 | per company (local HTTP service) | F2 |
| iSolarCloud | EU / CN / AU regions | F2 adapter, F9 features |
| EPİAŞ | single | F2 |

---

## 6. Runtime services

| Legacy service | Fate |
|----------------|------|
| `web` (Next.js, PM2 inside a container) | Split into `api` (Go) + `web` (Next.js) |
| `carbon-web` (second Next.js app at `/opt/carbon`) | **Removed** — merged into the single application |
| `worker` (BullMQ consumption worker) | Replaced by the Go worker |
| `chronos` (foundation-model forecasting service) | Behind the ML service contract |
| `ai-service` (Flask, direct MongoDB access) | Replaced by the FastAPI service with no database access |
| `mongo` | Replaced by PostgreSQL + TimescaleDB |
| `redis` | Retained |

---

## 7. Shell scripts to replace

| Legacy script | Replacement |
|---------------|-------------|
| `setup-cron-jobs.sh` | **Deleted** — scheduling is in-process |
| `create-admin.sh` | `bcem seed --admin` |
| `create-offline-package.sh`, `create-tar.sh` | `make offline-bundle` |
| `install-offline-package.sh`, `install-tar.sh` | `install.sh` |
| `run-project-basic.sh`, `start.sh`, `runsh.sh` | `make dev`, `docker compose up` |
| `test-deployment.sh`, `test-offline-setup.sh` | `scripts/test-offline-install.sh` + CI |
| `upgrade_mongo.sh` | Not applicable |
| `seed_admin.ts`, `scripts/migrate-remove-default-tariffs.ts` | `bcem seed`, `bcem migrate` |
| `train_model.py`, `test_*.py`, `final_integration_test.py` | ML service test suite |
| `reproduce_blocking.js` | Not applicable — the blocking problem is designed out |

---

## 8. Assets to migrate

| Asset | Notes |
|-------|-------|
| Emission factor master catalogue | Several thousand entries covering fuels, vehicles, flights, freight, refrigerants, materials and waste. **Seed data.** |
| GHG ↔ ISO 14064 category mapping | **Seed data.** |
| ISO 50001 clause texts (Turkish and English) and downloadable templates | **Seed data.** |
| Blog articles: "Elektrik faturam neden yüksek" parts 1 and 2, with images | Content migration |
| National tariff schedule (currently hard-coded constants in the public calculator) | **Seed data**, with effective dates |
| Marker icons, logos, brand imagery | Reviewed against the new design system; replaced where the old visual language shows through |
| Roboto font files (used for PDF rendering) | Replaced by the fonts in `07-design-system.md` §3, subset for PDF use |
| `icmalverileri.csv` | Test fixture for the icmal parser |
| i18n catalogues (~2,750 keys, `tr` + `en`) | **Seed data** for `next-intl` — see `i18n-tr.json` and `i18n-en.json` |
