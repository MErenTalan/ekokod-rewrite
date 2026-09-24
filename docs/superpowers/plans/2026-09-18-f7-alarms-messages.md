# F7 — Alarms and Messages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this
> plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This phase runs **inline in
> one session** — no parallel agents, no subagents, no Workflow (user's standing rule).

**Goal:** Monitoring that fires correctly and notifies reliably: four alarm rule types with full
settings and CRUD, an hourly evaluation pipeline that suppresses repeat notifications and can never
fire twice for the same invoice, e-mail delivery through the company's SMTP settings, and the
Messages screen with a job-history tab that lets an admin re-run any job.

**Architecture:** The evaluation logic is a **pure package** (`internal/domain/alarm`) that takes
already-loaded readings, bills and timestamps and returns verdicts — no repository, no clock, no
I/O, so every acceptance criterion in 09 §F7 is a table test. Around it sit three asynq tasks
following F2/F4's dispatch shape (`alarm.dispatch` platform tick → `alarm.evaluate` per company →
`alarm.notify` per event), a scoped CRUD service (`internal/service/alarms`) shaped like
`internal/service/calendar`, and an operations read service (`internal/service/ops`) behind
`GET /messages`, `GET /job-runs` and `POST /job-runs/{type}/trigger`. The web side follows the
conventions F6a/F6b fixed (R195–R199): route → `*-page` container → views, `useApiMutation` for
writes, `mockApi` for container tests, data-free stories.

**Tech Stack:** Go 1.25 (asynq, pgx/sqlc, shopspring/decimal, swaggest OpenAPI), Postgres 17 +
TimescaleDB, Redis, Next.js 15 App Router + React 19, TanStack Query via `openapi-react-query`,
Tailwind v4, Vitest + Storybook + Playwright.

**Spec:**
- `docs/rewrite/09-implementation-plan.md` §F7 — scope, acceptance criteria, verification
- `docs/rewrite/01-project-context.md` §7.12 (alarms), §7.13 (Messages), §8 (background work),
  §9 (notifications)
- `docs/rewrite/05-api-contract.md` §1 (conventions), §10 (alarms and messages)
- `docs/rewrite/04-data-model.md` §7 (alarms), §11 (operations) — already migrated in F1
- `docs/rewrite/07-design-system.md` §11 (accessibility), `design-system/bcem-energy/MASTER.md`
  then `OVERRIDES.md`
- Predecessor plans: `docs/superpowers/plans/2026-09-17-f6a-auth-api-skeleton.md` (R136–R189 +
  permission matrix), `docs/superpowers/plans/2026-09-17-f6b-core-screens.md` (R190–R210)

---

## Global Constraints

- **Branch/worktree:** `phase/f7-alarms` in `/home/personal/ekokod-f7-phase`, branched from
  `phase/f6b-screens`. Nothing is pushed; `main` is untouched; merging is the user's call.
- **Every command** is prefixed with
  `export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH" GOFLAGS=-p=2 GOMEMLIMIT=2GiB`.
- **Never run `make test` or `go test ./...`.** Run only the packages a task touches; the per-task
  gate is the **whole touched package**, not a `-run` filter (targeted filters missed four
  regressions in F6a).
- **Integration tests** need `ekokod-test-pg` and `ekokod-test-redis` (`docker ps` first; else
  `make test-db-up` / `make test-redis-up`; if Docker is down, tell the user) and then
  `EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 go test <pkg> -tags=integration -count=1 -parallel 4`.
- Test databases have **no continuous-aggregate refresh policies**; a test needing materialised
  data refreshes explicitly.
- **Web:** `pnpm test --maxWorkers=2`; Storybook, Playwright and `next build` one at a time, never
  concurrently. Check `free -m` and `ps -eo rss,comm | grep java` before heavy work; Playwright
  `--workers=1` when memory is tight. Never leave `next dev`/`next start`, `storybook dev`,
  `vitest --watch`, a static server or `bin/ekokod-e2e` running.
- **Do not touch** another project's Gradle/java processes, `dolmusum-dev-postgres-1` on 5432, or
  the dolmusum serve on 8080.
- **Money and energy cross the API as strings** (05 §1); `dto.Decimal` refuses JSON numbers.
  Timestamps render in Europe/Istanbul (R161).
- **404, never 403, for anything outside the caller's scope** (05 §1): one indistinguishable
  answer for unknown, foreign-company and out-of-scope ids.
- **Message keys are camelCase** (`pnpm check:i18n-parity` rejects anything else).
- **Comments say WHY in 1–3 lines.** Commit as soon as tests are green, then prove the guards red
  with your own mutation, then the next task.
- **Prove every guard red by mutation, restoring from a file backup** — never `git checkout` a file
  whose fix is not committed yet.
- Commit trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.
- Every new screen needs a line in `web/tests/e2e/responsive.spec.ts`'s `SCREENS` list.
- `make check-generate` needs a clean tree for `internal/store/postgres/sqlcgen` — commit first.
- **Test helper names in this plan are indicative, not authoritative.** Every Go test here is
  written against the fixture and harness helpers those files already use — `newHarness`,
  `h.Get`/`h.Post`, the `roleCompanyAdmin`-style role constants, `opsFixture`, `ptr`. Before
  writing a test, read the file you are appending to and reuse what is there. Where a helper
  genuinely does not exist yet (the inline task runner `TestTriggeredJobAppearsInJobRuns` needs,
  the `_fixture.ts` demo objects), add it once, in the file that needs it, and reuse it — never a
  second helper for a job an existing one already does.

---

## Product-owner decisions taken on 2026-09-18

These were asked before planning, with the evidence that prompted each question.

| # | Question | Decision |
|---|---|---|
| D-1 | **SMS** — 09 §F7 forbids offering it unimplemented; legacy accepts numbers and has no gateway anywhere (`grep`: only the UI, the model and demo data). | **Keep the UI field and store the numbers, but say so.** SMS stays selectable, numbers are saved, and the screen carries an R165-style note that SMS is not active yet. Every firing records `sms_not_sent` on the alarm event and surfaces it in Messages, so nothing is silently swallowed. No gateway in F7. |
| D-2 | **Voltage thresholds** — legacy reads `latestRec.voltage`, a field that exists in no payload. Verified against real data (`energy.analyzers.json`, 3926 load-profile rows: `t_p_kW`, `t_ri_kVarh`, `t_rc_kVarh`, `t_t1-3_kWh`, `u_*` — no voltage, no current) and against `model.MeterReading` (no voltage field). | **Drop voltage; make it a power alarm.** `voltage_max`/`voltage_min` are rejected and never shown; the type is labelled *Güç Alarmı* / *Power alarm*. |
| D-3 | **iSolar fault-alarm forwarding** — 01 §8/§9 put it inside the alarm check, `appendix/legacy-inventory.md` assigns `isolar.fetch_alarms` to F9. | **Stays in F9** with the solar-plant screens. F7 covers only the four analyzer-based rule types. `isolar_forwarded_alarms` (F1) and `MarkIsolarForwarded` stay unused until then. |
| D-4 | **`job_runs` history** has no screen in 01 §7.x; 09 §F7 only says "surfaced to admins". | **Second tab of `/ekorm/messages`.** Read: A + CA. Manual trigger: A only. Trigger allow-list: `alarm.evaluate`, `billing.dispatch`, `integration.sync_dispatch`, `epias.sync_prices`, `consumption.refresh`. |
| D-5 | Follow-up on D-1: is the user told SMS does not send? | **Yes** — the field stays *with* the "henüz gönderilmiyor" note. Chosen over the silent-legacy option. |

---

## Binding rulings (R211–R232)

Numbering continues from F6b's R210.

| # | Ruling |
|---|---|
| **R211** | **SMS is stored, never sent, and never silent.** `alarm_channels` accepts `sms` targets (`^\+?[0-9]{10,15}$`, ≤ 20 per rule). The notify path sends nothing to them and writes `"sms_not_sent"` into the alarm event's `detail.skipped_channels`, plus one `warning` operational message per firing that names how many numbers were not reached (a count, never the numbers — Messages rows are exported). The UI shows the R165-style caption `alarms.smsInactive`. A deliberate, recorded divergence from 09 §F7, per D-1/D-5. |
| **R212** | **`current_voltage_power` is a power-only alarm.** `voltage_max`/`voltage_min` in a request are rejected with 422 `validation_failed` (`{"voltage_max":["unsupported"]}`); the two columns and the SQL enum value stay as F1 transcribed them from 04 §7 (no migration). At least one of `power_max`/`power_min` is required. Reason recorded in the DTO and in `internal/domain/alarm`: no provider reports voltage or current. |
| **R213** | **Alarm visibility is the union of its analyzers' scopes.** `alarms` carries only `company_id`, so the *service* intersects: a rule is readable when **at least one** attached analyzer is inside the caller's scope (legacy semantics), a rule with no analyzers only by a company-wide scope (`AllBuildings`). A write requires **every** named analyzer to be in scope, which `ReplaceAnalyzers` already enforces with `ErrNotFound`. |
| **R214** | **Power is read from `MaxDemandKw` of the newest load-profile reading inside a 48-hour lookback.** `ReadingRepository.Latest` demands a `TimeRange`, and an unbounded "newest row" probes every chunk. A rule whose analyzer reported nothing in 48 h yields *no verdict* for the power alarm — recorded in the run summary, never fired as a breach. |
| **R215** | **Reactive ratios use the first and last reading in the window**, as legacy did: `ratio = (inductive_last − inductive_first) / (active_last − active_first)`, compared as a percentage against the threshold (`ratio × 100 > threshold`). A non-positive active difference yields ratio `0` (legacy behaviour, kept so a rebuilt history matches). Fewer than two readings in the window is *no verdict*, never a breach. Division scale 20, half-even, as `internal/domain/energy` does. |
| **R216** | **Data communication reads `Analyzer.LastReadingAt`** — no hypertable access — and fires when `now − last > threshold_hours`. An analyzer that never reported (`LastReadingAt == nil`) is *no verdict*, not a breach: a meter added five minutes ago has not lost communication. `communication_threshold_hours` is **required** at create (1–8760); legacy's silent default of 6 is gone. |
| **R217** | **The invoice alarm claims before it notifies.** It compares the two newest non-superseded `analyzer`-scope bills by `period_key` on `TotalCost`; a previous total of zero is *no verdict* (no percentage exists). Before any event is created, `MarkBillFired(alarm, bill)` claims the pair in one call; `claimed == false` skips the bill entirely. A crash between claim and send therefore loses one notification rather than sending forever — "fires once per invoice and never twice" wins over at-least-once. |
| **R218** | **Notification frequency suppresses the notification, never the event.** An alarm that breaches always writes an `alarm_events` row. The notification is then suppressed when any event for the same `(alarm, analyzer)` pair inside the frequency window already carries a non-NULL `notified_at`. A suppressed event carries `detail.notification_suppressed = true`, which is what distinguishes it from one still queued for delivery. A rule with no frequency configured notifies every firing. |
| **R219** | **Evaluations that do not fire write no `alarm_events` row.** One hourly run over every rule would otherwise write millions of "nothing happened" rows. Instead each `alarm.evaluate` writes one `job_runs` row with `processed`/`skipped`/`failed` and one `operational_messages` summary, which is what 01 §7.12's "history of evaluations and firings" is served by: firings in the alarm log, evaluations in Messages. |
| **R220** | **`job_runs` is the second tab of `/ekorm/messages`** (D-4), read by A + CA, triggered by A only, over the five-type allow-list. `POST /job-runs/{type}/trigger` takes an explicit scope and refuses any type outside the list with 422, so no caller can enqueue an arbitrary task name. |
| **R221** | **Alarm mail is plain text in both locales** through the existing `internal/mail` Sender (which composes quoted-printable text). Legacy's inline-HTML template is dropped; a multipart sender is not in F7's scope. Templates: `alarm_fired.tr.txt`, `alarm_fired.en.txt`. |
| **R222** | **A failed send is recorded, not retried.** `alarm.notify` writes the error onto the event (`MarkNotified` with a non-nil `notificationError`, `notified_at` left NULL), appends an `error` operational message, and returns **nil** so asynq does not retry it into a mail loop. 09 §F7: "it does not fail the job." |
| **R223** | **`POST /alarms/{id}/evaluate` is a dry run by default:** it evaluates now and returns the verdicts without writing an event, claiming a bill or sending anything. `?notify=true` runs the real firing path (A, CA only) and is exempt from `Idempotency-Key` replay (`NoIdempotency`), because a second deliberate dry run must be allowed to see fresh data. |
| **R224** | **The alarm's own error text never reaches the API.** Event `detail` is a curated JSON object (measured value, threshold, window) built by the domain package; an SMTP server's reply lands in `notification_error` and in the operational message's `detail`, both of which are operator-facing and pass through the store's scrubbing helpers — never a DSN or password. |
| **R225** | **One `alarm.evaluate` per company per hour.** `TaskID = "alarm.evaluate:<company>:<RFC3339 hour>"`, so a manual trigger inside the same hour as the cron tick is de-duplicated by asynq rather than double-notifying. `alarm.notify` is `TaskID = "alarm.notify:<event_id>"` — at most once per event. |
| **R226** | **Messages search is a case-insensitive substring over `message` and `detail`.** `store.MessageFilter` gains `Q string`; the SQL is `(message ilike '%'||$q||'%' or coalesce(detail,'') ilike '%'||$q||'%')` with `$q` escaped for `%`/`_`. Not full-text: the column has no index for it and the screen's window is one tenant's recent rows. Max length 200, longer is 422. |
| **R227** | **Messages and job runs are read-only to the tenant.** No endpoint deletes or edits an operational message or a job run; retention is a deployment concern (F15). |
| **R228** | **Six new permissions** extend R159's table: `alarms.read`, `alarms.edit`, `alarms.evaluate`, `messages.read`, `jobs.runs.read`, `jobs.trigger`. The permission rule table below is authoritative for which roles hold each; demo holds the two read permissions and nothing else. |
| **R229** | **The alarm rule form is one dialog with a type switch**, as legacy had it, not four routes: changing the type swaps the settings fieldset and clears the fields of the abandoned type from the draft, so a reactive threshold can never be submitted on a power rule. |
| **R230** | **A rule must carry at least one analyzer and at least one threshold** for its type. "At least one limit" is legacy's own `alert("En az bir alarm limiti girilmelidir")`, promoted from a browser alert to a 422 with per-field codes. |
| **R231** | **Recipients are validated, de-duplicated and capped**: e-mail `≤ 254` chars each, RFC-shaped, ≤ 50 per rule; SMS `^\+?[0-9]{10,15}$`, ≤ 20 per rule. `alarm_channels`' primary key `(alarm_id, channel, target)` already forbids exact duplicates; the service de-duplicates before writing rather than letting the insert fail. |
| **R232** | **Demo is read-only here too** (R150's shape): every alarm mutation, every dry run and every job trigger refuses the demo role, and the Messages screen shows real seeded rows. |

---

## Rule table — alarm types

`type` is the only field that says which settings are meaningful (`model.Alarm`'s own warning).
Every "no verdict" row is counted in the run summary and never notified.

| Type (`alarm_type`) | UI label (tr / en) | Settings used | Data source | Fires when | No verdict when |
|---|---|---|---|---|---|
| `reactive_limit` | Reactive Limit Algılama Alarmı / Reactive limit alarm | `inductive_ratio_threshold` + period, `capacitive_ratio_threshold` + period, `active_consumption_max` + period, `active_consumption_min` + period (≥ 1 group required) | `ReadingRepository.Latest`-bounded window reads of `ActiveImport`, `ReactiveInductiveImport`, `ReactiveCapacitiveImport` (kind `load_profile`) | any configured group breaches: `ratio×100 > threshold`, `active_diff > max`, `active_diff < min` | < 2 readings in that group's window |
| `data_communication` | Veri İletişim Alarmı / Data communication alarm | `communication_threshold_hours` (required, 1–8760) | `Analyzer.LastReadingAt` | `now − last_reading_at > threshold` | `LastReadingAt == nil` |
| `current_voltage_power` | Güç Alarmı / Power alarm (R212) | `power_max`, `power_min` (≥ 1 required); `voltage_max`/`voltage_min` **rejected** | `MaxDemandKw` of the newest `load_profile` reading in a 48 h window (R214) | `kw > power_max` or `kw < power_min` | no reading in 48 h, or that reading's `MaxDemandKw` is nil |
| `invoice_increase` | Fatura Alarmı / Invoice alarm | `invoice_threshold_pct` (required, > 0) | two newest non-superseded `analyzer`-scope bills by `period_key`, field `TotalCost` | `(latest − previous) / previous × 100 > threshold` **and** `MarkBillFired` claimed the pair | < 2 bills, or `previous.TotalCost == 0`, or the pair was already claimed |

Common to every type: analyzer multi-select (≥ 1, R230), notification frequency (value + unit,
optional), channels (`email` and/or `sms`), recipients per channel (R231), enabled flag.

---

## Rule table — permissions (extends R159)

| Permission | A | CA | CR | BA | BR | D | Guards |
|---|---|---|---|---|---|---|---|
| `alarms.read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | `GET /alarms`, `GET /alarms/{id}`, `GET /alarms/{id}/events` |
| `alarms.edit` | ✓ | ✓ | | ✓ | | | `POST /alarms`, `PATCH /alarms/{id}`, `DELETE /alarms/{id}` |
| `alarms.evaluate` | ✓ | ✓ | | | | | `POST /alarms/{id}/evaluate` |
| `messages.read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | `GET /messages` |
| `jobs.runs.read` | ✓ | ✓ | | | | | `GET /job-runs` |
| `jobs.trigger` | ✓ | | | | | | `POST /job-runs/{type}/trigger` |

Demo holds only the two read permissions, and `auth.IsReadOnly` already refuses it on every
mutating route. `alarms.edit` includes BA because 05 §10 lists `A CA BA` on the CRUD rows;
R213's write check keeps a BA inside its buildings.

---

## Rule table — routes (05 §10)

| Method | Path | Roles | OperationID | Request → Response |
|---|---|---|---|---|
| GET | `/alarms` | A CA CR BA BR D | `alarms.list` | `dto.AlarmListRequest` → `dto.Page[dto.Alarm]` |
| POST | `/alarms` | A CA BA | `alarms.create` | `dto.AlarmFields` → `dto.Alarm` (201) |
| GET | `/alarms/{id}` | A CA CR BA BR D | `alarms.get` | `dto.IDPath` → `dto.Alarm` |
| PATCH | `/alarms/{id}` | A CA BA | `alarms.update` | `dto.AlarmUpdateRequest` → `dto.Alarm` |
| DELETE | `/alarms/{id}` | A CA BA | `alarms.delete` | `dto.IDPath` → 204 |
| GET | `/alarms/{id}/events` | A CA CR BA BR D | `alarms.events.list` | `dto.AlarmEventsRequest` → `dto.Page[dto.AlarmEvent]` |
| POST | `/alarms/{id}/evaluate` | A CA | `alarms.evaluate` | `dto.AlarmEvaluateRequest` → `dto.AlarmEvaluation` |
| GET | `/messages` | A CA CR BA BR D | `messages.list` | `dto.MessagesRequest` → `dto.Page[dto.Message]` |
| GET | `/job-runs` | A CA | `jobRuns.list` | `dto.JobRunsRequest` → `dto.Page[dto.JobRun]` |
| POST | `/job-runs/{type}/trigger` | A | `jobRuns.trigger` | `dto.JobTriggerRequest` → `dto.JobAccepted` (202) |

`PATCH /alarms/{id}` is a **full replace** of the mutable fields, exactly as
`PATCH /calendar/events/{id}` is (that route's own summary says "Replace an event") — partial
merge over a wide nullable settings row would make "clear this threshold" unexpressible.

---

## File map

**Go — new**

| File | Responsibility |
|---|---|
| `internal/domain/alarm/alarm.go` | `Kind`, `Settings`, `Validate` — the per-type settings contract and its validation codes |
| `internal/domain/alarm/evaluate.go` | `Evaluate(Input) Verdict` — pure, per type; no I/O |
| `internal/domain/alarm/window.go` | `Window(now, value, unit)` → `store.TimeRange`-shaped `From`/`To`; reactive first/last differencing |
| `internal/domain/alarm/frequency.go` | `Suppressed(lastNotified *time.Time, now, value, unit)` — R218 |
| `internal/domain/alarm/message.go` | `Describe(Verdict, locale)` — the breach sentences and the curated `detail` object (R224) |
| `internal/service/alarms/alarms.go` | scoped CRUD + R213 visibility intersection + R230/R231 validation |
| `internal/service/alarms/evaluate.go` | `Run` (the per-company evaluation used by the job) and `DryRun` (R223) |
| `internal/service/ops/ops.go` | `Messages`, `JobRuns`, `Trigger` — the read/trigger service behind 05 §10's last three rows |
| `internal/job/alarm.go` | `alarm.dispatch`, `alarm.evaluate`, `alarm.notify` task types, payloads, TaskIDs, handlers |
| `internal/job/notify.go` | `Notifier` interface + the handler that sends one event's mail |
| `internal/mail/templates/alarm_fired.tr.txt`, `alarm_fired.en.txt` | R221 plain-text bodies |
| `internal/api/v1/handlers_alarms.go` | `alarmRoutes()` + handlers |
| `internal/api/v1/handlers_ops.go` | `opsRoutes()` + handlers |
| `internal/api/v1/dto/alarms.go` | `Alarm`, `AlarmFields`, `AlarmEvent`, `AlarmEvaluation`, requests |
| `internal/api/v1/dto/ops.go` | `Message`, `JobRun`, `JobTriggerRequest`, requests |

**Go — modified**

| File | Change |
|---|---|
| `internal/auth/roles.go` | six permissions (R228) |
| `internal/api/v1/dto/auth.go` | `Permission.Enum()` gains the six names |
| `internal/api/v1/routes.go` | `Table()` concatenates `alarmRoutes(), opsRoutes()` |
| `internal/api/v1/router.go` | `Handlers` gains `Alarms *alarms.Service`, `Ops *ops.Service` |
| `internal/apiwire/wiring.go` | builds both services; passes the asynq client to `ops` |
| `internal/store/repository.go` | `MessageFilter.Q` (R226); `AlarmEventFilter.Notified *bool` |
| `internal/store/postgres/queries/operations.sql`, `alarms.sql` | the two filter changes |
| `internal/job/task.go` | `Handlers` gains `AlarmDispatch`, `AlarmEvaluate`, `AlarmNotify`; `Register` routes the three types |
| `internal/scheduler/scheduler.go` | `alarm.dispatch` entry on `cfg.Schedule.Alarms` |
| `internal/platform/config/config.go`, `load.go` | `Schedule.Alarms` (default `0 * * * *`, 01 §8) |
| `internal/seed/{fixtures.go,readings.go}` | e2e alarm rules, events, messages and job runs |
| `internal/worker/worker.go` | wires the three alarm handlers |

**Web — new** (all under `web/src`)

| File | Responsibility |
|---|---|
| `app/ekorm/alarms/page.tsx` | route shell → `AlarmsPage` |
| `app/ekorm/messages/page.tsx` | route shell → `MessagesPage` |
| `features/alarms/alarms-page.tsx` + `.test.tsx` | container: list, filter, dialogs, mutations |
| `features/alarms/alarm-table.tsx` + `.test.tsx` + `.stories.tsx` | the list view (name, type, analyzers, enabled toggle, actions) |
| `features/alarms/alarm-dialog.tsx` + `.test.tsx` + `.stories.tsx` | the one dialog with the type switch (R229) |
| `features/alarms/alarm-draft.ts` + `.test.ts` | draft ↔ request mapping, per-type clearing, client-side R230 checks |
| `features/alarms/events-dialog.tsx` + `.test.tsx` + `.stories.tsx` | the alarm log |
| `features/alarms/evaluate-dialog.tsx` + `.test.tsx` + `.stories.tsx` | dry-run verdicts (R223) |
| `features/messages/messages-page.tsx` + `.test.tsx` | container: two tabs, filters, search |
| `features/messages/message-table.tsx` + `.test.tsx` + `.stories.tsx` | §7.13 table with type and status badges |
| `features/messages/job-runs-table.tsx` + `.test.tsx` + `.stories.tsx` | job history + trigger button (A only) |
| `messages/tr/alarms.json`, `messages/en/alarms.json` | alarm namespace |
| `messages/tr/messages.json`, `messages/en/messages.json` | messages namespace |
| `tests/e2e/alarms.spec.ts`, `tests/e2e/messages.spec.ts` | 09 §F7's e2e verification line |

**Web — modified:** `components/shell/nav-config.ts` (permissions on the two leaves),
`lib/session/permissions.fixture.json` (regenerated), `lib/api/schema.d.ts` (`make openapi`),
`tests/e2e/responsive.spec.ts` (two `SCREENS` lines), `messages/index.ts` (two namespaces).

---

## Task 1: Permissions and navigation gating

**Files:**
- Modify: `internal/auth/roles.go:68-89` (`permissionTable`)
- Modify: `internal/api/v1/dto/auth.go:21-29` (`Permission.Enum`)
- Modify: `web/src/components/shell/nav-config.ts` (the `alarms` group's leaves)
- Modify: `web/src/lib/session/permissions.fixture.json` (regenerated from Go)
- Test: `internal/auth/permissions_test.go`, `internal/auth/permissions_fixture_test.go` (existing
  guards), `web/src/components/shell/nav-config.test.ts`

**Interfaces:**
- Consumes: `auth.Roles`, `auth.PermissionsFor`, `auth.AllPermissions` (unchanged signatures).
- Produces: the six permission names of R228, which every later task's route rows reference:
  `"alarms.read"`, `"alarms.edit"`, `"alarms.evaluate"`, `"messages.read"`, `"jobs.runs.read"`,
  `"jobs.trigger"`.

- [ ] **Step 1: Write the failing test**

Append to `internal/auth/permissions_test.go`:

```go
// F7 R228: the six alarm, message and job permissions, per role.
func TestF7PermissionsPerRole(t *testing.T) {
	t.Parallel()
	cases := []struct {
		perm  string
		roles []model.UserRole
	}{
		{"alarms.read", []model.UserRole{model.UserRoleAdmin, model.UserRoleCompanyAdmin,
			model.UserRoleCompanyReadonlyAdmin, model.UserRoleBuildingAdmin,
			model.UserRoleBuildingReadonlyAdmin, model.UserRoleDemo}},
		{"alarms.edit", []model.UserRole{model.UserRoleAdmin, model.UserRoleCompanyAdmin,
			model.UserRoleBuildingAdmin}},
		{"alarms.evaluate", []model.UserRole{model.UserRoleAdmin, model.UserRoleCompanyAdmin}},
		{"messages.read", []model.UserRole{model.UserRoleAdmin, model.UserRoleCompanyAdmin,
			model.UserRoleCompanyReadonlyAdmin, model.UserRoleBuildingAdmin,
			model.UserRoleBuildingReadonlyAdmin, model.UserRoleDemo}},
		{"jobs.runs.read", []model.UserRole{model.UserRoleAdmin, model.UserRoleCompanyAdmin}},
		{"jobs.trigger", []model.UserRole{model.UserRoleAdmin}},
	}
	for _, c := range cases {
		t.Run(c.perm, func(t *testing.T) {
			for _, r := range model.UserRoles() {
				want := slices.Contains(c.roles, r)
				got := slices.Contains(auth.PermissionsFor(r), c.perm)
				require.Equalf(t, want, got, "role %s / permission %s", r, c.perm)
			}
		})
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/auth/ -run TestF7PermissionsPerRole -count=1`
Expected: FAIL — every subtest reports `want true, got false` (the permissions do not exist yet).

- [ ] **Step 3: Add the permissions**

In `internal/auth/roles.go`'s `permissionTable`, after the `integrations.credentials` line:

```go
	// F7 R228. alarms.edit includes BA because 05 §10 lists A CA BA on the CRUD
	// rows; the service keeps a BA inside its own buildings (R213).
	"alarms.read":    AllRoles,
	"alarms.edit":    Roles(roleA, roleCA, roleBA),
	"alarms.evaluate": Roles(roleA, roleCA),
	"messages.read":  AllRoles,
	"jobs.runs.read": Roles(roleA, roleCA),
	"jobs.trigger":   Roles(roleA),
```

- [ ] **Step 4: Run the whole package**

Run: `go test ./internal/auth/ -count=1`
Expected: `TestF7PermissionsPerRole` PASS; `TestPermissionEnumMatchesAuth`-style guards in
`internal/api/v1` and the web fixture guard now FAIL — that is the next step, not a surprise.

- [ ] **Step 5: Extend the OpenAPI enum and regenerate the fixture**

Add the six names to `Permission.Enum()` in `internal/api/v1/dto/auth.go`, **sorted** (the route
table test compares against `auth.AllPermissions()`, which sorts):

```go
		"admin.companies", "alarms.edit", "alarms.evaluate", "alarms.read", "analyzers.refresh",
		"anomaly.check", "bills.compute", "calendar.edit", "integrations.credentials",
		"jobs.runs.read", "jobs.trigger", "messages.read", "nav.core", "nav.financial",
		"nav.solar_plants", "settings.analyzers", "settings.analyzers.edit", "settings.buildings",
		"settings.company", "settings.company.edit", "settings.integrations", "settings.plants",
		"settings.smtp", "settings.users", "write",
```

Then regenerate `web/src/lib/session/permissions.fixture.json` so that each role's array is exactly
`auth.PermissionsFor(role)`. `internal/auth/permissions_fixture_test.go` prints the expected JSON in
its failure message — copy that.

Run: `go test ./internal/auth/ ./internal/api/v1/ -run 'Permission|RouteTable' -count=1` then
`export PATH=... && cd web && pnpm vitest run src/lib/session/permissions.test.ts`
Expected: PASS.

- [ ] **Step 6: Gate the navigation leaves**

In `web/src/components/shell/nav-config.ts`, the `alarms` group:

```ts
  { id: 'alarms', labelKey: 'alarms', icon: Bell, children: [
    { id: 'alarmsManual', labelKey: 'manual', href: '/ekorm/alarms', icon: BellRing, permission: 'alarms.read' },
    { id: 'messages', labelKey: 'messages', href: '/ekorm/messages', icon: MessageSquare, permission: 'messages.read' },
    { id: 'alarmsAi', labelKey: 'ai', href: '/ekorm/alarms/ai', icon: Sparkles, disabled: true } ] },
```

Both permissions are held by all six roles, so `navFor` output is unchanged for every role —
which is the point: the assertion is that the leaf declares its permission, so a later permission
change moves the nav with it. Add to `web/src/components/shell/nav-config.test.ts`:

```ts
it('gates the alarm and message leaves on their own permissions (R228)', () => {
  const leaves = navigation.flatMap((e) => (isGroup(e) ? e.children : [e]));
  expect(leaves.find((l) => l.href === '/ekorm/alarms')?.permission).toBe('alarms.read');
  expect(leaves.find((l) => l.href === '/ekorm/messages')?.permission).toBe('messages.read');
});
```

- [ ] **Step 7: Regenerate OpenAPI and run the gates**

```bash
make openapi
go test ./internal/auth/ ./internal/api/v1/ -count=1 -race
cd web && pnpm lint && pnpm typecheck && pnpm check:api && pnpm test --maxWorkers=2
```
Expected: all green, `make openapi` leaves `internal/api/v1/openapi.json` and
`web/src/lib/api/schema.d.ts` with only the six added enum members.

- [ ] **Step 8: Prove the guards red by mutation**

1. `cp internal/auth/roles.go /tmp/roles.bak`; change `"jobs.trigger": Roles(roleA)` to
   `Roles(roleA, roleCA)`; run `go test ./internal/auth/ -run TestF7PermissionsPerRole -count=1` →
   must FAIL on `jobs.trigger` / `company_admin`, **and** the fixture guard must fail. Restore from
   `/tmp/roles.bak`.
2. `cp web/src/components/shell/nav-config.ts /tmp/nav.bak`; delete
   `permission: 'messages.read'`; run `pnpm vitest run src/components/shell/nav-config.test.ts` →
   must FAIL. Restore.

- [ ] **Step 9: Commit**

```bash
git add internal/auth internal/api/v1/dto/auth.go internal/api/v1/openapi.json \
  web/src/lib/session/permissions.fixture.json web/src/components/shell/nav-config.ts \
  web/src/components/shell/nav-config.test.ts web/src/lib/api/schema.d.ts
git commit -m "feat(f7): alarm, message and job permissions (R228)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: `internal/domain/alarm` — settings contract and validation

**Files:**
- Create: `internal/domain/alarm/alarm.go`
- Test: `internal/domain/alarm/alarm_test.go`

**Interfaces:**
- Consumes: `model.Alarm`, `model.AlarmType`, `model.PeriodUnit`, `model.NotifyChannel`.
- Produces — every later task depends on these exact names:

```go
// Package alarm evaluates alarm rules (01 §7.12). It is pure: no repository,
// no clock, no I/O. Callers load readings, bills and timestamps and pass them in.
package alarm

// Field names used in validation codes; they match the DTO's JSON names.
const (
	FieldInductive       = "inductive_ratio_threshold"
	FieldCapacitive      = "capacitive_ratio_threshold"
	FieldActiveMax       = "active_consumption_max"
	FieldActiveMin       = "active_consumption_min"
	FieldCommsHours      = "communication_threshold_hours"
	FieldPowerMax        = "power_max"
	FieldPowerMin        = "power_min"
	FieldVoltageMax      = "voltage_max"
	FieldVoltageMin      = "voltage_min"
	FieldInvoicePct      = "invoice_threshold_pct"
	FieldAnalyzers       = "analyzer_ids"
	FieldSettings        = "settings"
	FieldFrequencyValue  = "notification_frequency_value"
)

// Validation codes.
const (
	CodeRequired    = "required"
	CodeUnsupported = "unsupported" // R212: voltage has no data source anywhere
	CodeAtLeastOne  = "at_least_one"
	CodeRange       = "range"
)

// ValidationError names the offending fields and their codes; the service
// turns it into perr.Validation.WithParams.
type ValidationError struct{ Fields map[string][]string }

func (e *ValidationError) Error() string

// Validate checks a rule's settings against its type (R212, R216, R230).
// It never looks at channels or recipients — the service owns R231.
func Validate(a model.Alarm, analyzerCount int) error
```

- [ ] **Step 1: Write the failing test**

`internal/domain/alarm/alarm_test.go`:

```go
package alarm_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func dec(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }
func i32(v int32) *int32            { return &v }
func unit(u model.PeriodUnit) *model.PeriodUnit { return &u }

func fields(t *testing.T, err error) map[string][]string {
	t.Helper()
	var ve *alarm.ValidationError
	require.ErrorAs(t, err, &ve)
	return ve.Fields
}

func TestValidateRequiresAnAnalyzer(t *testing.T) {
	t.Parallel()
	a := model.Alarm{Type: model.AlarmTypeDataCommunication, CommunicationThresholdHours: i32(6)}
	require.Equal(t, []string{alarm.CodeRequired},
		fields(t, alarm.Validate(a, 0))[alarm.FieldAnalyzers])
	require.NoError(t, alarm.Validate(a, 1))
}

func TestValidateReactiveNeedsOneCompleteGroup(t *testing.T) {
	t.Parallel()
	// R230: a threshold without its period is not a group.
	bare := model.Alarm{Type: model.AlarmTypeReactiveLimit, InductiveRatioThreshold: dec("20")}
	require.Equal(t, []string{alarm.CodeAtLeastOne},
		fields(t, alarm.Validate(bare, 1))[alarm.FieldSettings])

	full := bare
	full.InductivePeriodValue, full.InductivePeriodUnit = i32(24), unit(model.PeriodUnitHours)
	require.NoError(t, alarm.Validate(full, 1))
}

func TestValidateRejectsVoltage(t *testing.T) {
	t.Parallel()
	// R212: no provider reports voltage; legacy read a field that never existed.
	a := model.Alarm{Type: model.AlarmTypeCurrentVoltagePower, PowerMax: dec("100"), VoltageMax: dec("400")}
	require.Equal(t, []string{alarm.CodeUnsupported},
		fields(t, alarm.Validate(a, 1))[alarm.FieldVoltageMax])

	a.VoltageMax = nil
	require.NoError(t, alarm.Validate(a, 1))
}

func TestValidatePowerNeedsOneBound(t *testing.T) {
	t.Parallel()
	a := model.Alarm{Type: model.AlarmTypeCurrentVoltagePower}
	require.Equal(t, []string{alarm.CodeAtLeastOne},
		fields(t, alarm.Validate(a, 1))[alarm.FieldSettings])
}

func TestValidateCommsHoursRequiredAndBounded(t *testing.T) {
	t.Parallel()
	// R216: legacy's silent default of 6 hours is gone.
	a := model.Alarm{Type: model.AlarmTypeDataCommunication}
	require.Equal(t, []string{alarm.CodeRequired},
		fields(t, alarm.Validate(a, 1))[alarm.FieldCommsHours])

	a.CommunicationThresholdHours = i32(8761)
	require.Equal(t, []string{alarm.CodeRange},
		fields(t, alarm.Validate(a, 1))[alarm.FieldCommsHours])
}

func TestValidateInvoicePctPositive(t *testing.T) {
	t.Parallel()
	a := model.Alarm{Type: model.AlarmTypeInvoiceIncrease}
	require.Equal(t, []string{alarm.CodeRequired},
		fields(t, alarm.Validate(a, 1))[alarm.FieldInvoicePct])

	a.InvoiceThresholdPct = dec("0")
	require.Equal(t, []string{alarm.CodeRange},
		fields(t, alarm.Validate(a, 1))[alarm.FieldInvoicePct])

	a.InvoiceThresholdPct = dec("20")
	require.NoError(t, alarm.Validate(a, 1))
}

func TestValidateIgnoresOtherTypesSettings(t *testing.T) {
	t.Parallel()
	// model.Alarm's own warning: Type is the ONLY thing that says which fields
	// are meaningful, so a stray field from another type is not an error —
	// the service clears it. This pins that Validate does not reject it.
	a := model.Alarm{Type: model.AlarmTypeDataCommunication, CommunicationThresholdHours: i32(6),
		InvoiceThresholdPct: dec("20")}
	require.NoError(t, alarm.Validate(a, 1))
}

func TestValidateFrequencyNeedsBothOrNeither(t *testing.T) {
	t.Parallel()
	a := model.Alarm{Type: model.AlarmTypeDataCommunication, CommunicationThresholdHours: i32(6),
		NotificationFrequencyValue: i32(6)}
	require.Equal(t, []string{alarm.CodeRequired},
		fields(t, alarm.Validate(a, 1))[alarm.FieldFrequencyValue])

	a.NotificationFrequencyUnit = unit(model.PeriodUnitHours)
	require.NoError(t, alarm.Validate(a, 1))
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/domain/alarm/ -count=1`
Expected: FAIL — `package internal/domain/alarm is not in std` / build failure, the package does
not exist.

- [ ] **Step 3: Write the implementation**

`internal/domain/alarm/alarm.go` — the shape; fill in every branch the tests above name:

```go
func (e *ValidationError) Error() string { return fmt.Sprintf("alarm: invalid settings: %v", e.Fields) }

func Validate(a model.Alarm, analyzerCount int) error {
	f := map[string][]string{}
	if analyzerCount < 1 {
		f[FieldAnalyzers] = []string{CodeRequired} // R230
	}
	// A frequency is optional, but half of one is a bug, not a default.
	if (a.NotificationFrequencyValue == nil) != (a.NotificationFrequencyUnit == nil) {
		f[FieldFrequencyValue] = []string{CodeRequired}
	} else if v := a.NotificationFrequencyValue; v != nil && (*v < 1 || *v > 8760) {
		f[FieldFrequencyValue] = []string{CodeRange}
	}
	switch a.Type {
	case model.AlarmTypeReactiveLimit:
		groups := 0
		for _, g := range []struct {
			threshold *decimal.Decimal
			value     *int32
			u         *model.PeriodUnit
		}{
			{a.InductiveRatioThreshold, a.InductivePeriodValue, a.InductivePeriodUnit},
			{a.CapacitiveRatioThreshold, a.CapacitivePeriodValue, a.CapacitivePeriodUnit},
			{a.ActiveConsumptionMax, a.ActiveConsumptionMaxPeriodValue, a.ActiveConsumptionMaxPeriodUnit},
			{a.ActiveConsumptionMin, a.ActiveConsumptionMinPeriodValue, a.ActiveConsumptionMinPeriodUnit},
		} {
			if g.threshold != nil && g.value != nil && g.u != nil && *g.value >= 1 && *g.value <= 8760 {
				groups++
			}
		}
		if groups == 0 {
			f[FieldSettings] = []string{CodeAtLeastOne}
		}
	case model.AlarmTypeDataCommunication:
		switch h := a.CommunicationThresholdHours; {
		case h == nil:
			f[FieldCommsHours] = []string{CodeRequired}
		case *h < 1 || *h > 8760:
			f[FieldCommsHours] = []string{CodeRange}
		}
	case model.AlarmTypeCurrentVoltagePower:
		// R212: the columns stay (F1 transcribed 04 §7) but nothing may set them.
		if a.VoltageMax != nil {
			f[FieldVoltageMax] = []string{CodeUnsupported}
		}
		if a.VoltageMin != nil {
			f[FieldVoltageMin] = []string{CodeUnsupported}
		}
		if a.PowerMax == nil && a.PowerMin == nil {
			f[FieldSettings] = []string{CodeAtLeastOne}
		}
	case model.AlarmTypeInvoiceIncrease:
		switch p := a.InvoiceThresholdPct; {
		case p == nil:
			f[FieldInvoicePct] = []string{CodeRequired}
		case !p.IsPositive():
			f[FieldInvoicePct] = []string{CodeRange}
		}
	}
	if len(f) == 0 {
		return nil
	}
	return &ValidationError{Fields: f}
}
```

- [ ] **Step 4: Run the whole package**

Run: `go test ./internal/domain/alarm/ -count=1 -race`
Expected: PASS, 8 tests.

- [ ] **Step 5: Prove the guards red by mutation**

`cp internal/domain/alarm/alarm.go /tmp/alarm.bak`, then one at a time:
1. Delete the `FieldVoltageMax` branch → `TestValidateRejectsVoltage` must FAIL.
2. Change `groups == 0` to `groups < 0` → `TestValidateReactiveNeedsOneCompleteGroup` must FAIL.
3. Change the frequency XOR to `&&` → `TestValidateFrequencyNeedsBothOrNeither` must FAIL.
Restore from `/tmp/alarm.bak` after each. A surviving mutation is a finding: the test is wrong.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/alarm
git commit -m "feat(f7): alarm settings validation, voltage rejected (R212, R216, R230)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: `internal/domain/alarm` — reactive-limit evaluation

**Files:**
- Create: `internal/domain/alarm/window.go`, `internal/domain/alarm/evaluate.go`
- Test: `internal/domain/alarm/evaluate_reactive_test.go`

**Interfaces:**
- Consumes: Task 2's package; `model.MeterReading`, `model.Bill`, `model.Analyzer`.
- Produces:

```go
// Window is the half-open lookback [To-period, To) a group is measured over.
type Window struct{ From, To time.Time }

// NewWindow builds the lookback ending at now. An unknown unit yields hours.
func NewWindow(now time.Time, value int32, u model.PeriodUnit) Window

// Breach is one threshold that was crossed.
type Breach struct {
	// Field is the setting that was crossed (FieldInductive, FieldPowerMax, …).
	Field string
	// Measured and Threshold are exact; Measured is nil only for a Field that
	// carries no number (none today).
	Measured, Threshold decimal.Decimal
	// Window is the period measured, zero for point-in-time checks.
	Window Window
	// Extra carries the type's own context: period_key for an invoice breach,
	// last_reading_at for a communication breach.
	Extra map[string]string
}

// Verdict is one analyzer's outcome for one rule.
type Verdict struct {
	AnalyzerID uuid.UUID
	Breaches   []Breach
	// NoVerdict lists the fields that could not be decided for lack of data
	// (R214, R215, R216, R217). It is NOT a breach and never notifies.
	NoVerdict []string
}

// Fired reports whether the verdict breached anything.
func (v Verdict) Fired() bool { return len(v.Breaches) > 0 }

// ReactiveInput is one analyzer's readings per configured group. Each slice is
// the readings inside that group's own window, ascending by Ts.
type ReactiveInput struct {
	AnalyzerID                            uuid.UUID
	Inductive, Capacitive, ActiveMax, ActiveMin []model.MeterReading
	Windows                               map[string]Window
}

// EvaluateReactive applies the four reactive groups (R215).
func EvaluateReactive(a model.Alarm, in ReactiveInput) Verdict
```

- [ ] **Step 1: Write the failing test**

`internal/domain/alarm/evaluate_reactive_test.go`:

```go
package alarm_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

var base = time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)

// reading builds a load-profile row with the three registers the reactive
// groups difference.
func reading(h int, active, inductive, capacitive string) model.MeterReading {
	a := decimal.RequireFromString(active)
	i := decimal.RequireFromString(inductive)
	c := decimal.RequireFromString(capacitive)
	return model.MeterReading{Ts: base.Add(time.Duration(h) * time.Hour), Kind: model.ReadingKindLoadProfile,
		ActiveImport: &a, ReactiveInductiveImport: &i, ReactiveCapacitiveImport: &c}
}

func reactiveRule() model.Alarm {
	return model.Alarm{Type: model.AlarmTypeReactiveLimit,
		InductiveRatioThreshold: dec("20"), InductivePeriodValue: i32(24), InductivePeriodUnit: unit(model.PeriodUnitHours)}
}

func TestReactiveFiresWhenInductiveRatioExceeds(t *testing.T) {
	t.Parallel()
	// 100 kWh active, 25 kVarh inductive → 25% > 20%.
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		Inductive: []model.MeterReading{reading(0, "1000", "100", "0"), reading(24, "1100", "125", "0")}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.True(t, v.Fired())
	require.Len(t, v.Breaches, 1)
	require.Equal(t, alarm.FieldInductive, v.Breaches[0].Field)
	require.Equal(t, "25", v.Breaches[0].Measured.String())
	require.Equal(t, "20", v.Breaches[0].Threshold.String())
}

func TestReactiveStaysSilentAtOrBelowThreshold(t *testing.T) {
	t.Parallel()
	// Exactly 20% is not "> 20": legacy used a strict comparison.
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		Inductive: []model.MeterReading{reading(0, "1000", "100", "0"), reading(24, "1100", "120", "0")}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.False(t, v.Fired())
	require.Empty(t, v.NoVerdict)
}

func TestReactiveNonPositiveActiveDiffYieldsZeroRatio(t *testing.T) {
	t.Parallel()
	// R215, legacy: a flat or falling active register gives ratio 0, not a
	// division by zero and not a breach.
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		Inductive: []model.MeterReading{reading(0, "1000", "100", "0"), reading(24, "1000", "500", "0")}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.False(t, v.Fired())
	require.Empty(t, v.NoVerdict)
}

func TestReactiveFewerThanTwoReadingsIsNoVerdict(t *testing.T) {
	t.Parallel()
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		Inductive: []model.MeterReading{reading(0, "1000", "100", "0")}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.False(t, v.Fired())
	require.Equal(t, []string{alarm.FieldInductive}, v.NoVerdict)
}

func TestReactiveActiveConsumptionMaxAndMin(t *testing.T) {
	t.Parallel()
	a := model.Alarm{Type: model.AlarmTypeReactiveLimit,
		ActiveConsumptionMax: dec("50"), ActiveConsumptionMaxPeriodValue: i32(24), ActiveConsumptionMaxPeriodUnit: unit(model.PeriodUnitHours),
		ActiveConsumptionMin: dec("10"), ActiveConsumptionMinPeriodValue: i32(24), ActiveConsumptionMinPeriodUnit: unit(model.PeriodUnitHours)}

	over := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		ActiveMax: []model.MeterReading{reading(0, "1000", "0", "0"), reading(24, "1100", "0", "0")},
		ActiveMin: []model.MeterReading{reading(0, "1000", "0", "0"), reading(24, "1100", "0", "0")}}
	v := alarm.EvaluateReactive(a, over)
	require.Len(t, v.Breaches, 1)
	require.Equal(t, alarm.FieldActiveMax, v.Breaches[0].Field)
	require.Equal(t, "100", v.Breaches[0].Measured.String())

	under := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		ActiveMax: []model.MeterReading{reading(0, "1000", "0", "0"), reading(24, "1005", "0", "0")},
		ActiveMin: []model.MeterReading{reading(0, "1000", "0", "0"), reading(24, "1005", "0", "0")}}
	v = alarm.EvaluateReactive(a, under)
	require.Len(t, v.Breaches, 1)
	require.Equal(t, alarm.FieldActiveMin, v.Breaches[0].Field)
}

func TestReactiveMissingRegisterIsNoVerdictNotZero(t *testing.T) {
	t.Parallel()
	// A nil register is unknown, not zero: a nil inductive reading must never
	// be read as "0 kVarh, therefore compliant".
	first, last := reading(0, "1000", "100", "0"), reading(24, "1100", "125", "0")
	last.ReactiveInductiveImport = nil
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(), Inductive: []model.MeterReading{first, last}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.False(t, v.Fired())
	require.Equal(t, []string{alarm.FieldInductive}, v.NoVerdict)
}

func TestNewWindowUnits(t *testing.T) {
	t.Parallel()
	require.Equal(t, base.Add(-24*time.Hour), alarm.NewWindow(base, 24, model.PeriodUnitHours).From)
	require.Equal(t, base.AddDate(0, 0, -3), alarm.NewWindow(base, 3, model.PeriodUnitDays).From)
	require.Equal(t, base, alarm.NewWindow(base, 3, model.PeriodUnitDays).To)
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/domain/alarm/ -count=1`
Expected: FAIL — `undefined: alarm.EvaluateReactive`, `undefined: alarm.NewWindow`.

- [ ] **Step 3: Write the implementation**

`internal/domain/alarm/window.go`:

```go
// NewWindow builds the lookback ending at now. Days are calendar days in the
// instant's own location, so a 3-day window over a DST change is still 3 days.
func NewWindow(now time.Time, value int32, u model.PeriodUnit) Window {
	if u == model.PeriodUnitDays {
		return Window{From: now.AddDate(0, 0, -int(value)), To: now}
	}
	return Window{From: now.Add(-time.Duration(value) * time.Hour), To: now}
}
```

`internal/domain/alarm/evaluate.go` — the reactive half:

```go
const divisionScale int32 = 20 // as internal/domain/reactive does

// diff returns last−first for the register pick, or ok=false when either end
// is missing: a nil register is unknown, never zero.
func diff(rows []model.MeterReading, pick func(model.MeterReading) *decimal.Decimal) (decimal.Decimal, bool) {
	if len(rows) < 2 {
		return decimal.Zero, false
	}
	first, last := pick(rows[0]), pick(rows[len(rows)-1])
	if first == nil || last == nil {
		return decimal.Zero, false
	}
	return last.Sub(*first), true
}

func EvaluateReactive(a model.Alarm, in ReactiveInput) Verdict {
	v := Verdict{AnalyzerID: in.AnalyzerID}
	active := func(r model.MeterReading) *decimal.Decimal { return r.ActiveImport }

	// ratio group: (reactive_diff / active_diff) × 100 > threshold (R215).
	ratioGroup := func(field string, rows []model.MeterReading, threshold *decimal.Decimal,
		pick func(model.MeterReading) *decimal.Decimal) {
		if threshold == nil {
			return
		}
		reactiveDiff, ok := diff(rows, pick)
		activeDiff, ok2 := diff(rows, active)
		if !ok || !ok2 {
			v.NoVerdict = append(v.NoVerdict, field)
			return
		}
		pct := decimal.Zero // legacy: a non-positive active diff is ratio 0
		if activeDiff.IsPositive() {
			pct = reactiveDiff.DivRound(activeDiff, divisionScale).Mul(decimal.NewFromInt(100))
		}
		if pct.GreaterThan(*threshold) {
			v.Breaches = append(v.Breaches, Breach{Field: field, Measured: pct,
				Threshold: *threshold, Window: in.Windows[field]})
		}
	}
	ratioGroup(FieldInductive, in.Inductive, a.InductiveRatioThreshold,
		func(r model.MeterReading) *decimal.Decimal { return r.ReactiveInductiveImport })
	ratioGroup(FieldCapacitive, in.Capacitive, a.CapacitiveRatioThreshold,
		func(r model.MeterReading) *decimal.Decimal { return r.ReactiveCapacitiveImport })

	boundGroup := func(field string, rows []model.MeterReading, bound *decimal.Decimal, over bool) {
		if bound == nil {
			return
		}
		d, ok := diff(rows, active)
		if !ok {
			v.NoVerdict = append(v.NoVerdict, field)
			return
		}
		if (over && d.GreaterThan(*bound)) || (!over && d.LessThan(*bound)) {
			v.Breaches = append(v.Breaches, Breach{Field: field, Measured: d,
				Threshold: *bound, Window: in.Windows[field]})
		}
	}
	boundGroup(FieldActiveMax, in.ActiveMax, a.ActiveConsumptionMax, true)
	boundGroup(FieldActiveMin, in.ActiveMin, a.ActiveConsumptionMin, false)
	return v
}
```

- [ ] **Step 4: Run the whole package**

Run: `go test ./internal/domain/alarm/ -count=1 -race`
Expected: PASS, 15 tests.

- [ ] **Step 5: Prove the guards red by mutation**

`cp internal/domain/alarm/evaluate.go /tmp/eval.bak`, then one at a time:
1. `pct.GreaterThan(*threshold)` → `GreaterThanOrEqual` →
   `TestReactiveStaysSilentAtOrBelowThreshold` must FAIL.
2. In `diff`, replace the nil check with `first == nil && last == nil` →
   `TestReactiveMissingRegisterIsNoVerdictNotZero` must FAIL.
3. Drop the `activeDiff.IsPositive()` guard →
   `TestReactiveNonPositiveActiveDiffYieldsZeroRatio` must FAIL (or panic on division by zero,
   which is also a failure).
Restore after each.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/alarm
git commit -m "feat(f7): reactive-limit evaluation, legacy ratio semantics (R215)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: `internal/domain/alarm` — the other three types, frequency and messages

**Files:**
- Modify: `internal/domain/alarm/evaluate.go`
- Create: `internal/domain/alarm/frequency.go`, `internal/domain/alarm/message.go`
- Test: `internal/domain/alarm/evaluate_others_test.go`,
  `internal/domain/alarm/frequency_test.go`, `internal/domain/alarm/message_test.go`

**Interfaces:**
- Consumes: Task 3's `Verdict`, `Breach`, `Window`, `NewWindow`, field and code constants.
- Produces:

```go
// EvaluateComms fires when the analyzer has been silent too long (R216).
// lastReadingAt nil is no verdict: a meter added minutes ago has not lost comms.
func EvaluateComms(a model.Alarm, analyzerID uuid.UUID, lastReadingAt *time.Time, now time.Time) Verdict

// EvaluatePower compares MaxDemandKw of the newest reading in the lookback
// against power_max/power_min (R212, R214). latest nil is no verdict.
func EvaluatePower(a model.Alarm, analyzerID uuid.UUID, latest *model.MeterReading, w Window) Verdict

// EvaluateInvoice compares the two newest bills on TotalCost (R217). The caller
// has already ordered them newest-first and has NOT yet claimed anything.
func EvaluateInvoice(a model.Alarm, analyzerID uuid.UUID, latest, previous *model.Bill) Verdict

// Suppressed reports whether a notification is suppressed because one was
// already delivered inside the frequency window (R218). A rule with no
// frequency configured never suppresses.
func Suppressed(a model.Alarm, lastNotifiedAt *time.Time, now time.Time) bool

// Describe renders the operator-facing sentence for a verdict in locale "tr"
// or "en", and the curated detail object that lands in alarm_events.detail
// (R224). It never carries a provider or SMTP string.
func Describe(a model.Alarm, v Verdict, analyzerLabel, locale string) (summary string, lines []string, detail map[string]any)
```

- [ ] **Step 1: Write the failing tests**

`internal/domain/alarm/evaluate_others_test.go`:

```go
package alarm_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func commsRule() model.Alarm {
	return model.Alarm{Type: model.AlarmTypeDataCommunication, CommunicationThresholdHours: i32(6)}
}

func TestCommsFiresWhenLastReadingIsOlderThanThreshold(t *testing.T) {
	t.Parallel()
	last := base.Add(-7 * time.Hour)
	v := alarm.EvaluateComms(commsRule(), uuid.New(), &last, base)
	require.True(t, v.Fired())
	require.Equal(t, alarm.FieldCommsHours, v.Breaches[0].Field)
	require.Equal(t, "7", v.Breaches[0].Measured.String())
	require.Equal(t, "6", v.Breaches[0].Threshold.String())
	require.Equal(t, last.Format(time.RFC3339), v.Breaches[0].Extra["last_reading_at"])
}

func TestCommsStaysSilentInsideThreshold(t *testing.T) {
	t.Parallel()
	last := base.Add(-5 * time.Hour)
	require.False(t, alarm.EvaluateComms(commsRule(), uuid.New(), &last, base).Fired())
}

func TestCommsExactlyAtThresholdStaysSilent(t *testing.T) {
	t.Parallel()
	// "older than this window" is strict, as legacy's `diffHours > thresholdHours` was.
	last := base.Add(-6 * time.Hour)
	require.False(t, alarm.EvaluateComms(commsRule(), uuid.New(), &last, base).Fired())
}

func TestCommsNeverReportedIsNoVerdict(t *testing.T) {
	t.Parallel()
	v := alarm.EvaluateComms(commsRule(), uuid.New(), nil, base)
	require.False(t, v.Fired())
	require.Equal(t, []string{alarm.FieldCommsHours}, v.NoVerdict)
}

func powerRule() model.Alarm {
	return model.Alarm{Type: model.AlarmTypeCurrentVoltagePower, PowerMax: dec("100"), PowerMin: dec("5")}
}

func withDemand(kw string) *model.MeterReading {
	d := decimal.RequireFromString(kw)
	return &model.MeterReading{Ts: base, Kind: model.ReadingKindLoadProfile, MaxDemandKw: &d}
}

func TestPowerFiresOverMaxAndUnderMin(t *testing.T) {
	t.Parallel()
	w := alarm.NewWindow(base, 48, model.PeriodUnitHours)
	over := alarm.EvaluatePower(powerRule(), uuid.New(), withDemand("120.5"), w)
	require.Len(t, over.Breaches, 1)
	require.Equal(t, alarm.FieldPowerMax, over.Breaches[0].Field)
	require.Equal(t, "120.5", over.Breaches[0].Measured.String())

	under := alarm.EvaluatePower(powerRule(), uuid.New(), withDemand("2"), w)
	require.Len(t, under.Breaches, 1)
	require.Equal(t, alarm.FieldPowerMin, under.Breaches[0].Field)
}

func TestPowerInsideBoundsStaysSilent(t *testing.T) {
	t.Parallel()
	w := alarm.NewWindow(base, 48, model.PeriodUnitHours)
	v := alarm.EvaluatePower(powerRule(), uuid.New(), withDemand("50"), w)
	require.False(t, v.Fired())
	require.Empty(t, v.NoVerdict)
}

func TestPowerNoReadingOrNoDemandIsNoVerdict(t *testing.T) {
	t.Parallel()
	w := alarm.NewWindow(base, 48, model.PeriodUnitHours)
	// R214: nothing in 48 h.
	require.Equal(t, []string{alarm.FieldPowerMax, alarm.FieldPowerMin},
		alarm.EvaluatePower(powerRule(), uuid.New(), nil, w).NoVerdict)
	// A reading that carries no max demand is not 0 kW.
	require.Equal(t, []string{alarm.FieldPowerMax, alarm.FieldPowerMin},
		alarm.EvaluatePower(powerRule(), uuid.New(),
			&model.MeterReading{Ts: base, Kind: model.ReadingKindLoadProfile}, w).NoVerdict)
}

func TestPowerNeverReadsVoltage(t *testing.T) {
	t.Parallel()
	// R212: even a stored voltage threshold produces no breach — the rule was
	// validated away, and the evaluator has no voltage input to compare.
	a := powerRule()
	a.VoltageMax, a.VoltageMin = dec("1"), dec("100000")
	v := alarm.EvaluatePower(a, uuid.New(), withDemand("50"), alarm.NewWindow(base, 48, model.PeriodUnitHours))
	require.False(t, v.Fired())
	for _, b := range v.Breaches {
		require.NotContains(t, []string{alarm.FieldVoltageMax, alarm.FieldVoltageMin}, b.Field)
	}
}

func bill(period, total string) *model.Bill {
	return &model.Bill{ID: uuid.New(), Scope: model.BillScopeAnalyzer, PeriodKey: period,
		TotalCost: decimal.RequireFromString(total)}
}

func invoiceRule() model.Alarm {
	return model.Alarm{Type: model.AlarmTypeInvoiceIncrease, InvoiceThresholdPct: dec("20")}
}

func TestInvoiceFiresAboveThreshold(t *testing.T) {
	t.Parallel()
	v := alarm.EvaluateInvoice(invoiceRule(), uuid.New(), bill("2026-08", "1300"), bill("2026-07", "1000"))
	require.True(t, v.Fired())
	require.Equal(t, alarm.FieldInvoicePct, v.Breaches[0].Field)
	require.Equal(t, "30", v.Breaches[0].Measured.String())
	require.Equal(t, "2026-08", v.Breaches[0].Extra["period_key"])
}

func TestInvoiceStaysSilentAtOrBelowThreshold(t *testing.T) {
	t.Parallel()
	require.False(t, alarm.EvaluateInvoice(invoiceRule(), uuid.New(),
		bill("2026-08", "1200"), bill("2026-07", "1000")).Fired())
}

func TestInvoiceZeroPreviousIsNoVerdict(t *testing.T) {
	t.Parallel()
	// No percentage exists against zero, and treating it as "infinite increase"
	// would fire on every first invoice after an idle period.
	v := alarm.EvaluateInvoice(invoiceRule(), uuid.New(), bill("2026-08", "1300"), bill("2026-07", "0"))
	require.False(t, v.Fired())
	require.Equal(t, []string{alarm.FieldInvoicePct}, v.NoVerdict)
}

func TestInvoiceFewerThanTwoBillsIsNoVerdict(t *testing.T) {
	t.Parallel()
	v := alarm.EvaluateInvoice(invoiceRule(), uuid.New(), bill("2026-08", "1300"), nil)
	require.False(t, v.Fired())
	require.Equal(t, []string{alarm.FieldInvoicePct}, v.NoVerdict)
}
```

`internal/domain/alarm/frequency_test.go`:

```go
package alarm_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func freqRule(value int32, u model.PeriodUnit) model.Alarm {
	return model.Alarm{NotificationFrequencyValue: i32(value), NotificationFrequencyUnit: unit(u)}
}

func TestSuppressedInsideWindowAllowedAfterIt(t *testing.T) {
	t.Parallel()
	a := freqRule(6, model.PeriodUnitHours)
	inside := base.Add(-5 * time.Hour)
	require.True(t, alarm.Suppressed(a, &inside, base))
	outside := base.Add(-7 * time.Hour)
	require.False(t, alarm.Suppressed(a, &outside, base))
}

func TestSuppressedExactlyAtWindowEdgeAllows(t *testing.T) {
	t.Parallel()
	// The window is half-open: a notification exactly one period old has aged out.
	edge := base.Add(-6 * time.Hour)
	require.False(t, alarm.Suppressed(freqRule(6, model.PeriodUnitHours), &edge, base))
}

func TestNoFrequencyNeverSuppresses(t *testing.T) {
	t.Parallel()
	recent := base.Add(-1 * time.Minute)
	require.False(t, alarm.Suppressed(model.Alarm{}, &recent, base))
}

func TestNoPreviousNotificationNeverSuppresses(t *testing.T) {
	t.Parallel()
	require.False(t, alarm.Suppressed(freqRule(30, model.PeriodUnitDays), nil, base))
}

func TestSuppressedCountsDaysAsCalendarDays(t *testing.T) {
	t.Parallel()
	a := freqRule(2, model.PeriodUnitDays)
	inside := base.AddDate(0, 0, -1)
	require.True(t, alarm.Suppressed(a, &inside, base))
	outside := base.AddDate(0, 0, -3)
	require.False(t, alarm.Suppressed(a, &outside, base))
}
```

`internal/domain/alarm/message_test.go`:

```go
package alarm_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func TestDescribeRendersBothLocales(t *testing.T) {
	t.Parallel()
	a := reactiveRule()
	a.Name = "Endüktif izleme"
	in := alarm.ReactiveInput{Inductive: []model.MeterReading{
		reading(0, "1000", "100", "0"), reading(24, "1100", "125", "0")}}
	v := alarm.EvaluateReactive(a, in)

	tr, lines, detail := alarm.Describe(a, v, "A-1", "tr")
	require.Contains(t, tr, "Endüktif izleme")
	require.Contains(t, tr, "A-1")
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], "25")
	require.Contains(t, lines[0], "20")
	require.Equal(t, "A-1", detail["analyzer"])
	breaches, ok := detail["breaches"].([]map[string]any)
	require.True(t, ok)
	require.Equal(t, alarm.FieldInductive, breaches[0]["field"])
	require.Equal(t, "25", breaches[0]["measured"])
	require.Equal(t, "20", breaches[0]["threshold"])

	en, _, _ := alarm.Describe(a, v, "A-1", "en")
	require.NotEqual(t, tr, en)
}

func TestDescribeFallsBackToTurkish(t *testing.T) {
	t.Parallel()
	a := commsRule()
	a.Name = "İletişim"
	last := base.Add(-9 * time.Hour)
	v := alarm.EvaluateComms(a, uuid.New(), &last, base)
	fallback, _, _ := alarm.Describe(a, v, "A-1", "de")
	turkish, _, _ := alarm.Describe(a, v, "A-1", "tr")
	require.Equal(t, turkish, fallback)
}

func TestDescribeDetailCarriesNoRawStrings(t *testing.T) {
	t.Parallel()
	// R224: detail is curated. Only these keys ever appear, so no provider or
	// SMTP text can ride along in it.
	a := commsRule()
	a.Name = "İletişim"
	last := base.Add(-9 * time.Hour)
	_, _, detail := alarm.Describe(a, alarm.EvaluateComms(a, uuid.New(), &last, base), "A-1", "tr")
	for k := range detail {
		require.Contains(t, []string{"analyzer", "alarm_type", "breaches", "no_verdict"}, k)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./internal/domain/alarm/ -count=1`
Expected: FAIL — `undefined: alarm.EvaluateComms`, `alarm.EvaluatePower`,
`alarm.EvaluateInvoice`, `alarm.Suppressed`, `alarm.Describe`.

- [ ] **Step 3: Write the implementations**

Append to `internal/domain/alarm/evaluate.go`:

```go
func EvaluateComms(a model.Alarm, analyzerID uuid.UUID, lastReadingAt *time.Time, now time.Time) Verdict {
	v := Verdict{AnalyzerID: analyzerID}
	if a.CommunicationThresholdHours == nil {
		return v
	}
	if lastReadingAt == nil {
		// R216: never reported is not "lost communication".
		v.NoVerdict = append(v.NoVerdict, FieldCommsHours)
		return v
	}
	elapsed := now.Sub(*lastReadingAt).Hours()
	threshold := decimal.NewFromInt(int64(*a.CommunicationThresholdHours))
	measured := decimal.NewFromFloat(elapsed).Round(3)
	if measured.GreaterThan(threshold) {
		v.Breaches = append(v.Breaches, Breach{Field: FieldCommsHours, Measured: measured,
			Threshold: threshold, Extra: map[string]string{"last_reading_at": lastReadingAt.Format(time.RFC3339)}})
	}
	return v
}

func EvaluatePower(a model.Alarm, analyzerID uuid.UUID, latest *model.MeterReading, w Window) Verdict {
	v := Verdict{AnalyzerID: analyzerID}
	// R212: voltage is never read — there is no input for it.
	if latest == nil || latest.MaxDemandKw == nil {
		if a.PowerMax != nil {
			v.NoVerdict = append(v.NoVerdict, FieldPowerMax)
		}
		if a.PowerMin != nil {
			v.NoVerdict = append(v.NoVerdict, FieldPowerMin)
		}
		return v
	}
	kw := *latest.MaxDemandKw
	if a.PowerMax != nil && kw.GreaterThan(*a.PowerMax) {
		v.Breaches = append(v.Breaches, Breach{Field: FieldPowerMax, Measured: kw, Threshold: *a.PowerMax, Window: w})
	}
	if a.PowerMin != nil && kw.LessThan(*a.PowerMin) {
		v.Breaches = append(v.Breaches, Breach{Field: FieldPowerMin, Measured: kw, Threshold: *a.PowerMin, Window: w})
	}
	return v
}

func EvaluateInvoice(a model.Alarm, analyzerID uuid.UUID, latest, previous *model.Bill) Verdict {
	v := Verdict{AnalyzerID: analyzerID}
	if a.InvoiceThresholdPct == nil {
		return v
	}
	// R217: no percentage exists without two bills and a non-zero baseline.
	if latest == nil || previous == nil || previous.TotalCost.IsZero() {
		v.NoVerdict = append(v.NoVerdict, FieldInvoicePct)
		return v
	}
	pct := latest.TotalCost.Sub(previous.TotalCost).
		DivRound(previous.TotalCost, divisionScale).Mul(decimal.NewFromInt(100))
	if pct.GreaterThan(*a.InvoiceThresholdPct) {
		v.Breaches = append(v.Breaches, Breach{Field: FieldInvoicePct, Measured: pct,
			Threshold: *a.InvoiceThresholdPct, Extra: map[string]string{
				"period_key":    latest.PeriodKey,
				"bill_id":       latest.ID.String(),
				"previous_key":  previous.PeriodKey,
				"latest_total":  latest.TotalCost.String(),
				"previous_total": previous.TotalCost.String(),
			}})
	}
	return v
}
```

`internal/domain/alarm/frequency.go`:

```go
// Suppressed is R218: the frequency limits the NOTIFICATION, not the event.
// The window is half-open, so a notification exactly one period old has aged out.
func Suppressed(a model.Alarm, lastNotifiedAt *time.Time, now time.Time) bool {
	if lastNotifiedAt == nil || a.NotificationFrequencyValue == nil || a.NotificationFrequencyUnit == nil {
		return false
	}
	w := NewWindow(now, *a.NotificationFrequencyValue, *a.NotificationFrequencyUnit)
	return lastNotifiedAt.After(w.From)
}
```

`internal/domain/alarm/message.go` — one sentence per field per locale, built from a table:

```go
// phrases is the breach sentence per field per locale. The format arguments are
// always (measured, threshold) so a wrong pairing cannot compile away silently.
var phrases = map[string]map[string]string{
	"tr": {
		FieldInductive:  "Endüktif oran %%%s eşiği aştı (%%%s)",
		FieldCapacitive: "Kapasitif oran %%%s eşiği aştı (%%%s)",
		FieldActiveMax:  "Aktif tüketim %s kWh maksimum limiti aştı (%s kWh)",
		FieldActiveMin:  "Aktif tüketim %s kWh minimum limitin altına düştü (%s kWh)",
		FieldCommsHours: "Analizör %s saattir veri göndermiyor (eşik: %s saat)",
		FieldPowerMax:   "Güç %s kW maksimum limiti aştı (%s kW)",
		FieldPowerMin:   "Güç %s kW minimum limitin altına düştü (%s kW)",
		FieldInvoicePct: "Fatura %%%s arttı (eşik: %%%s)",
	},
	"en": {
		FieldInductive:  "Inductive ratio %s%% exceeded the threshold (%s%%)",
		FieldCapacitive: "Capacitive ratio %s%% exceeded the threshold (%s%%)",
		FieldActiveMax:  "Active consumption %s kWh exceeded the maximum (%s kWh)",
		FieldActiveMin:  "Active consumption %s kWh fell below the minimum (%s kWh)",
		FieldCommsHours: "The analyzer has sent no data for %s hours (threshold: %s hours)",
		FieldPowerMax:   "Power %s kW exceeded the maximum (%s kW)",
		FieldPowerMin:   "Power %s kW fell below the minimum (%s kW)",
		FieldInvoicePct: "The invoice rose by %s%% (threshold: %s%%)",
	},
}

func Describe(a model.Alarm, v Verdict, analyzerLabel, locale string) (string, []string, map[string]any) {
	if _, ok := phrases[locale]; !ok {
		locale = "tr" // tr is the product's default (05 §1)
	}
	lines := make([]string, 0, len(v.Breaches))
	rows := make([]map[string]any, 0, len(v.Breaches))
	for _, b := range v.Breaches {
		lines = append(lines, fmt.Sprintf(phrases[locale][b.Field], b.Measured.String(), b.Threshold.String()))
		row := map[string]any{"field": b.Field, "measured": b.Measured.String(), "threshold": b.Threshold.String()}
		if !b.Window.From.IsZero() {
			row["window_from"], row["window_to"] = b.Window.From.Format(time.RFC3339), b.Window.To.Format(time.RFC3339)
		}
		for k, val := range b.Extra {
			row[k] = val
		}
		rows = append(rows, row)
	}
	// The summary is the rule and the meter; the locale lives in the lines.
	summary := fmt.Sprintf("%s — %s", a.Name, analyzerLabel)
	detail := map[string]any{"analyzer": analyzerLabel, "alarm_type": string(a.Type), "breaches": rows}
	if len(v.NoVerdict) > 0 {
		detail["no_verdict"] = v.NoVerdict
	}
	return summary, lines, detail
}
```

> **Note for the implementer:** `phrases` uses `%%%s` where a literal `%` precedes a value (`%25`),
> because `fmt` needs `%%` for the sign. The `message_test.go` assertions on `"25"` and `"20"`
> catch a mis-escaped verb immediately.

- [ ] **Step 4: Run the whole package**

Run: `go test ./internal/domain/alarm/ -count=1 -race`
Expected: PASS, 30 tests. Then `golangci-lint run --concurrency 2 ./internal/domain/alarm/...` → 0
issues.

- [ ] **Step 5: Prove the guards red by mutation**

Back up each file first. One at a time:
1. `measured.GreaterThan(threshold)` → `GreaterThanOrEqual` in `EvaluateComms` →
   `TestCommsExactlyAtThresholdStaysSilent` must FAIL.
2. Remove the `previous.TotalCost.IsZero()` check → `TestInvoiceZeroPreviousIsNoVerdict` must FAIL.
3. `lastNotifiedAt.After(w.From)` → `!w.From.After(*lastNotifiedAt)` →
   `TestSuppressedExactlyAtWindowEdgeAllows` must FAIL.
4. In `EvaluatePower`, treat a nil `MaxDemandKw` as `decimal.Zero` →
   `TestPowerNoReadingOrNoDemandIsNoVerdict` must FAIL **and** the min bound would fire, which is
   exactly the bug the test exists for.
5. Add `"raw": "smtp: 535 auth failed"` to `detail` → `TestDescribeDetailCarriesNoRawStrings` must
   FAIL.
Restore after each.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/alarm
git commit -m "feat(f7): comms, power and invoice evaluation, frequency, messages (R211-R224)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Store — message search and a notified filter

**Files:**
- Modify: `internal/store/repository.go` (`MessageFilter`, `AlarmEventFilter`)
- Modify: `internal/store/postgres/queries/operations.sql`,
  `internal/store/postgres/queries/alarms.sql`
- Modify: `internal/store/postgres/operations.go`, `internal/store/postgres/alarms.go`
- Regenerate: `internal/store/postgres/sqlcgen/` (`make sqlc` / `make check-generate`)
- Test: `internal/store/postgres/operations_integration_test.go`,
  `internal/store/postgres/alarms_integration_test.go`

**Interfaces:**
- Consumes: `store.MessageFilter`, `store.AlarmEventFilter`, `store.OpsRepository`,
  `store.AlarmRepository` (all F1).
- Produces:

```go
// MessageFilter gains, per R226:
	// Q is a case-insensitive substring matched against message and detail.
	// Not full-text: there is no index for it and the screen's window is one
	// tenant's recent rows. Max 200 characters; the service rejects longer.
	Q string

// AlarmEventFilter gains:
	// Notified, when non-nil, keeps only events whose notified_at is set
	// (true) or NULL (false). Undelivered stays for the NULL-only case it
	// already serves; Notified is what the frequency check needs (R218).
	Notified *bool
```

- [ ] **Step 1: Write the failing test**

Append to `internal/store/postgres/operations_integration_test.go`:

```go
// R226: the Messages screen's search box.
func TestListMessagesSearchesMessageAndDetail(t *testing.T) {
	t.Parallel()
	ctx, repo, sc := opsFixture(t)

	seed := []model.OperationalMessage{
		{Kind: "job", Category: "analyzer-refresh", Status: "success", Message: "Analizör yenilendi"},
		{Kind: "alarm", Category: "alarm-trigger", Status: "warning", Message: "Alarm tetiklendi",
			Detail: ptr("Endüktif oran %25")},
		{Kind: "system", Category: "login", Status: "info", Message: "Oturum açıldı"},
	}
	for _, m := range seed {
		_, err := repo.AppendMessage(ctx, sc, m)
		require.NoError(t, err)
	}

	got, err := repo.ListMessages(ctx, sc, store.MessageFilter{Q: "alarm"})
	require.NoError(t, err)
	require.Len(t, got, 1, "matches the message column, case-insensitively")

	got, err = repo.ListMessages(ctx, sc, store.MessageFilter{Q: "ENDÜKTİF"})
	require.NoError(t, err)
	require.Len(t, got, 1, "matches the detail column too")

	// A LIKE metacharacter is a literal, not a wildcard: '%' must not match all.
	got, err = repo.ListMessages(ctx, sc, store.MessageFilter{Q: "%"})
	require.NoError(t, err)
	require.Len(t, got, 1, "only the row whose detail literally contains %")

	got, err = repo.ListMessages(ctx, sc, store.MessageFilter{Q: "yok"})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestListMessagesSearchCombinesWithFilters(t *testing.T) {
	t.Parallel()
	ctx, repo, sc := opsFixture(t)
	for _, m := range []model.OperationalMessage{
		{Kind: "alarm", Category: "alarm-trigger", Status: "warning", Message: "Alarm tetiklendi"},
		{Kind: "job", Category: "alarm-evaluate", Status: "success", Message: "Alarm değerlendirmesi"},
	} {
		_, err := repo.AppendMessage(ctx, sc, m)
		require.NoError(t, err)
	}
	got, err := repo.ListMessages(ctx, sc, store.MessageFilter{Q: "alarm", Kinds: []string{"job"}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "job", got[0].Kind)
}
```

Append to `internal/store/postgres/alarms_integration_test.go`:

```go
// R218: the frequency check needs the newest DELIVERED event for a pair.
func TestListEventsNotifiedFilter(t *testing.T) {
	t.Parallel()
	ctx, repo, sc, alarmID, analyzerID := alarmEventFixture(t)

	delivered, err := repo.CreateEvent(ctx, sc, model.AlarmEvent{AlarmID: alarmID,
		AnalyzerID: &analyzerID, Message: "delivered"})
	require.NoError(t, err)
	require.NoError(t, repo.MarkNotified(ctx, sc, delivered.ID, time.Now().UTC(), nil))

	_, err = repo.CreateEvent(ctx, sc, model.AlarmEvent{AlarmID: alarmID,
		AnalyzerID: &analyzerID, Message: "pending"})
	require.NoError(t, err)

	yes, no := true, false
	got, err := repo.ListEvents(ctx, sc, store.AlarmEventFilter{AlarmID: &alarmID,
		AnalyzerID: &analyzerID, Notified: &yes})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "delivered", got[0].Message)
	require.NotNil(t, got[0].NotifiedAt)

	got, err = repo.ListEvents(ctx, sc, store.AlarmEventFilter{AlarmID: &alarmID,
		AnalyzerID: &analyzerID, Notified: &no})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "pending", got[0].Message)

	got, err = repo.ListEvents(ctx, sc, store.AlarmEventFilter{AlarmID: &alarmID, AnalyzerID: &analyzerID})
	require.NoError(t, err)
	require.Len(t, got, 2, "a nil Notified filters nothing")
}
```

> If `opsFixture`, `alarmEventFixture` or `ptr` do not exist under those names in the two test
> files, reuse whatever the file already uses to build a scoped repository — do not add a second
> fixture helper for the same job.

- [ ] **Step 2: Run them and watch them fail**

```bash
docker ps --format '{{.Names}}' | grep -E 'ekokod-test-(pg|redis)'   # both, else make test-db-up / test-redis-up
EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' \
EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
go test ./internal/store/postgres/ -tags=integration -count=1 -parallel 4 \
  -run 'TestListMessagesSearch|TestListEventsNotifiedFilter'
```
Expected: FAIL to compile — `unknown field Q in struct literal`, `unknown field Notified`.

- [ ] **Step 3: Add the filter fields and the SQL**

`store.MessageFilter.Q` and `store.AlarmEventFilter.Notified` exactly as the Interfaces block
above spells them, then the two queries. In `operations.sql`'s message list, add to the `where`
chain:

```sql
  and (
    sqlc.arg(q)::text = ''
    or message ilike '%' || replace(replace(replace(sqlc.arg(q)::text, '\', '\\'), '%', '\%'), '_', '\_') || '%'
    or coalesce(detail, '') ilike '%' || replace(replace(replace(sqlc.arg(q)::text, '\', '\\'), '%', '\%'), '_', '\_') || '%'
  )
```

Escaping `\`, `%` and `_` is what makes `Q = "%"` match one row rather than all of them — the
third assertion in `TestListMessagesSearchesMessageAndDetail` is that guard.

In `alarms.sql`'s event list, add:

```sql
  and (sqlc.narg(notified)::boolean is null
       or (sqlc.narg(notified)::boolean and e.notified_at is not null)
       or (not sqlc.narg(notified)::boolean and e.notified_at is null))
```

Then thread both through the Go wrappers in `operations.go` and `alarms.go` and regenerate:

```bash
make sqlc   # or the generate target the Makefile defines for sqlcgen
```

- [ ] **Step 4: Run the whole package**

```bash
EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' \
EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
go test ./internal/store/postgres/ -tags=integration -count=1 -parallel 4
go test ./internal/store/... -count=1 -race
make check-generate   # needs a clean tree: commit first if it complains
```
Expected: PASS, and `check-generate` reports no diff.

- [ ] **Step 5: Prove the guards red by mutation**

1. Remove the three `replace(...)` escapes from the message query, regenerate, run
   `TestListMessagesSearchesMessageAndDetail` → the `Q = "%"` assertion must FAIL with 3 rows.
2. Change `ilike` to `like` → the `"ENDÜKTİF"` assertion must FAIL.
3. Invert the `notified` branch → `TestListEventsNotifiedFilter` must FAIL.
Restore from backups and regenerate each time.

- [ ] **Step 6: Commit**

```bash
git add internal/store
git commit -m "feat(f7): message search and a notified event filter (R218, R226)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: `internal/service/alarms` — scoped CRUD

**Files:**
- Create: `internal/service/alarms/alarms.go`, `internal/service/alarms/alarms_test.go`
- Test: also `internal/store/postgres/scope_isolation_integration_test.go` (extend the sweep)

**Interfaces:**
- Consumes: `store.AlarmRepository`, `store.AnalyzerRepository`, `clock.Clock`,
  `alarm.Validate`, `alarm.ValidationError`, `perr.Validation`, `perr.NotFound`.
- Produces:

```go
// Package alarms manages alarm rules (01 §7.12, 05 §10). Visibility is R213:
// alarms carry only company_id, so this service intersects with the Scope's
// building branch through the rule's own analyzers.
package alarms

// Rule is one alarm with the attachments the API returns together with it.
type Rule struct {
	Alarm     model.Alarm
	Analyzers []AnalyzerRef
	Channels  []model.AlarmChannel
}

// AnalyzerRef is an attached analyzer, labelled for the list column
// "applied analyzers" (01 §7.12) so the screen needs no second request.
// LastReadingAt rides along because the data-communication evaluation (R216)
// needs exactly that and nothing else — no second read, no hypertable access.
type AnalyzerRef struct {
	ID                 uuid.UUID
	InstallationNumber string
	BuildingID         *uuid.UUID
	LastReadingAt      *time.Time
}

// Input is a create or full replace. Settings live on Alarm; the service
// clears every field that does not belong to Alarm.Type before writing.
type Input struct {
	Name        string
	Type        model.AlarmType
	IsEnabled   bool
	Alarm       model.Alarm
	AnalyzerIDs []uuid.UUID
	Channels    []model.AlarmChannel
}

type Deps struct {
	Alarms    store.AlarmRepository
	Analyzers store.AnalyzerRepository
	Clock     clock.Clock

	// Notify and Evaluate are OPTIONAL and nil in the API process, which only
	// does CRUD and dry runs. The worker sets both. A method reached without
	// its own deps returns perr.Unavailable rather than panicking on a nil
	// interface — the same nil-tolerant contract job.Register already follows.
	// Their types are declared in Tasks 8 and 9; declare the fields as
	// *NotifyDeps / *EvaluateDeps when those tasks land, and leave them out
	// of Deps until then so this task compiles on its own.
	Notify   *NotifyDeps
	Evaluate *EvaluateDeps
}

// Service holds the deps plus loc, the Europe/Istanbul location New loads
// (internal/service may not import internal/api, so dto.Istanbul is off
// limits — internal/service/billing loads its own the same way).
type Service struct {
	d   Deps
	nd  *NotifyDeps
	ed  *EvaluateDeps
	loc *time.Location
}

func New(d Deps) (*Service, error)

func (s *Service) List(ctx context.Context, sc store.Scope, f store.AlarmFilter) ([]Rule, error)
func (s *Service) Get(ctx context.Context, sc store.Scope, id uuid.UUID) (Rule, error)
func (s *Service) Create(ctx context.Context, sc store.Scope, in Input) (Rule, error)
func (s *Service) Update(ctx context.Context, sc store.Scope, id uuid.UUID, in Input) (Rule, error)
func (s *Service) Delete(ctx context.Context, sc store.Scope, id uuid.UUID) error
func (s *Service) Events(ctx context.Context, sc store.Scope, id uuid.UUID, f store.AlarmEventFilter) ([]model.AlarmEvent, error)
```

- [ ] **Step 1: Write the failing test**

`internal/service/alarms/alarms_test.go` uses in-memory fakes for the two repositories (the
package's own `fakeAlarms`/`fakeAnalyzers`, in the file), so the whole suite is a unit test:

```go
func TestCreateRejectsInvalidSettings(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t, oneAnalyzerInScope)
	_, err := svc.Create(t.Context(), companyScope, alarms.Input{
		Name: "Gerilim", Type: model.AlarmTypeCurrentVoltagePower,
		Alarm:       model.Alarm{VoltageMax: dec("400")},
		AnalyzerIDs: []uuid.UUID{analyzerA},
	})
	// R212 travels from the domain package to a 422 with per-field codes.
	require.Equal(t, http.StatusUnprocessableEntity, perr.StatusOf(err))
	require.Equal(t, "validation_failed", perr.CodeOf(err))
}

func TestCreateClearsSettingsOfOtherTypes(t *testing.T) {
	t.Parallel()
	svc, fake := newTestService(t, oneAnalyzerInScope)
	got, err := svc.Create(t.Context(), companyScope, alarms.Input{
		Name: "İletişim", Type: model.AlarmTypeDataCommunication,
		// A reactive threshold on a comms rule is a leftover from the dialog's
		// type switch; it must never reach the row (model.Alarm's own warning).
		Alarm:       model.Alarm{CommunicationThresholdHours: i32(6), InductiveRatioThreshold: dec("20")},
		AnalyzerIDs: []uuid.UUID{analyzerA},
	})
	require.NoError(t, err)
	require.Nil(t, got.Alarm.InductiveRatioThreshold)
	require.Nil(t, fake.stored[got.Alarm.ID].InductiveRatioThreshold)
}

func TestCreateRequiresEveryAnalyzerInScope(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t, oneAnalyzerInScope)
	_, err := svc.Create(t.Context(), buildingScope, alarms.Input{
		Name: "Karışık", Type: model.AlarmTypeDataCommunication,
		Alarm:       model.Alarm{CommunicationThresholdHours: i32(6)},
		AnalyzerIDs: []uuid.UUID{analyzerA, analyzerForeign},
	})
	// R213: one indistinguishable 404, never a 403 that confirms existence.
	require.Equal(t, http.StatusNotFound, perr.StatusOf(err))
}

func TestListShowsARuleWhenOneAnalyzerIsInScope(t *testing.T) {
	t.Parallel()
	// R213, legacy semantics: a building admin sees a company-wide rule that
	// touches one of their meters, and not one that touches none of them.
	svc, _ := newTestService(t, twoBuildingsOneScoped)
	got, err := svc.List(t.Context(), buildingScope, store.AlarmFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Bizim bina", got[0].Alarm.Name)
}

func TestGetHidesARuleWithNoAnalyzerInScope(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t, twoBuildingsOneScoped)
	_, err := svc.Get(t.Context(), buildingScope, otherBuildingsAlarm)
	require.Equal(t, http.StatusNotFound, perr.StatusOf(err))
	// The company-wide scope still sees it, so the 404 is about scope, not existence.
	_, err = svc.Get(t.Context(), companyScope, otherBuildingsAlarm)
	require.NoError(t, err)
}

func TestRuleWithNoAnalyzersIsCompanyWideOnly(t *testing.T) {
	t.Parallel()
	// A rule whose analyzers were all deleted cannot be intersected, so only a
	// scope that spans the company may see it (R213).
	svc, _ := newTestService(t, orphanRule)
	_, err := svc.Get(t.Context(), buildingScope, orphanAlarmID)
	require.Equal(t, http.StatusNotFound, perr.StatusOf(err))
	_, err = svc.Get(t.Context(), companyScope, orphanAlarmID)
	require.NoError(t, err)
}

func TestChannelsAreValidatedDeduplicatedAndCapped(t *testing.T) {
	t.Parallel()
	svc, fake := newTestService(t, oneAnalyzerInScope)
	base := alarms.Input{Name: "Kanal", Type: model.AlarmTypeDataCommunication,
		Alarm: model.Alarm{CommunicationThresholdHours: i32(6)}, AnalyzerIDs: []uuid.UUID{analyzerA}}

	bad := base
	bad.Channels = []model.AlarmChannel{{Channel: model.NotifyChannelEmail, Target: "not-an-email"}}
	require.Equal(t, http.StatusUnprocessableEntity, perr.StatusOf(mustErr(svc.Create(t.Context(), companyScope, bad))))

	badSMS := base
	badSMS.Channels = []model.AlarmChannel{{Channel: model.NotifyChannelSMS, Target: "0555 123 45 67"}}
	require.Equal(t, http.StatusUnprocessableEntity, perr.StatusOf(mustErr(svc.Create(t.Context(), companyScope, badSMS))))

	// R231: the same address twice is de-duplicated, not an insert conflict.
	dup := base
	dup.Channels = []model.AlarmChannel{
		{Channel: model.NotifyChannelEmail, Target: "ops@example.com"},
		{Channel: model.NotifyChannelEmail, Target: "ops@example.com"},
		{Channel: model.NotifyChannelSMS, Target: "+905551234567"},
	}
	got, err := svc.Create(t.Context(), companyScope, dup)
	require.NoError(t, err)
	require.Len(t, got.Channels, 2)
	require.Len(t, fake.channels[got.Alarm.ID], 2)

	tooMany := base
	for i := 0; i < 51; i++ {
		tooMany.Channels = append(tooMany.Channels,
			model.AlarmChannel{Channel: model.NotifyChannelEmail, Target: fmt.Sprintf("a%d@example.com", i)})
	}
	require.Equal(t, http.StatusUnprocessableEntity, perr.StatusOf(mustErr(svc.Create(t.Context(), companyScope, tooMany))))
}

func TestUpdateIsAFullReplaceOfSettings(t *testing.T) {
	t.Parallel()
	// A threshold left out of a PATCH is CLEARED, which is what makes
	// "remove this limit" expressible at all.
	svc, _ := newTestService(t, oneAnalyzerInScope)
	created, err := svc.Create(t.Context(), companyScope, alarms.Input{
		Name: "Reaktif", Type: model.AlarmTypeReactiveLimit,
		Alarm: model.Alarm{InductiveRatioThreshold: dec("20"), InductivePeriodValue: i32(24),
			InductivePeriodUnit: unit(model.PeriodUnitHours), CapacitiveRatioThreshold: dec("15"),
			CapacitivePeriodValue: i32(24), CapacitivePeriodUnit: unit(model.PeriodUnitHours)},
		AnalyzerIDs: []uuid.UUID{analyzerA}})
	require.NoError(t, err)

	updated, err := svc.Update(t.Context(), companyScope, created.Alarm.ID, alarms.Input{
		Name: "Reaktif", Type: model.AlarmTypeReactiveLimit,
		Alarm: model.Alarm{InductiveRatioThreshold: dec("30"), InductivePeriodValue: i32(12),
			InductivePeriodUnit: unit(model.PeriodUnitHours)},
		AnalyzerIDs: []uuid.UUID{analyzerA}})
	require.NoError(t, err)
	require.Equal(t, "30", updated.Alarm.InductiveRatioThreshold.String())
	require.Nil(t, updated.Alarm.CapacitiveRatioThreshold)
}

func TestEventsRefuseARuleOutOfScope(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t, twoBuildingsOneScoped)
	_, err := svc.Events(t.Context(), buildingScope, otherBuildingsAlarm, store.AlarmEventFilter{})
	require.Equal(t, http.StatusNotFound, perr.StatusOf(err))
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/service/alarms/ -count=1`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the implementation**

The shape that satisfies every test above:

```go
func New(d Deps) (*Service, error) {
	if d.Alarms == nil || d.Analyzers == nil || d.Clock == nil {
		return nil, errors.New("alarms: Alarms, Analyzers and Clock are required")
	}
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return nil, errors.Join(errors.New("alarms: load Europe/Istanbul"), err)
	}
	return &Service{d: d, nd: d.Notify, ed: d.Evaluate, loc: loc}, nil
}

// emailRe and phoneRe are R231. The e-mail shape is deliberately loose — the
// SMTP server is the real judge — but it must contain exactly one @ with
// something either side and no whitespace.
var (
	emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$`)
	phoneRe = regexp.MustCompile(`^\+?[0-9]{10,15}$`)
)

const (
	maxEmailTargets = 50
	maxSMSTargets   = 20
)

// settingsFor copies ONLY the fields Type gives meaning to, so a leftover from
// the dialog's type switch can never reach the row.
func settingsFor(in Input) model.Alarm {
	out := model.Alarm{Name: strings.TrimSpace(in.Name), Type: in.Type, IsEnabled: in.IsEnabled,
		NotificationFrequencyValue: in.Alarm.NotificationFrequencyValue,
		NotificationFrequencyUnit:  in.Alarm.NotificationFrequencyUnit}
	switch in.Type {
	case model.AlarmTypeReactiveLimit:
		out.InductiveRatioThreshold, out.InductivePeriodValue, out.InductivePeriodUnit =
			in.Alarm.InductiveRatioThreshold, in.Alarm.InductivePeriodValue, in.Alarm.InductivePeriodUnit
		out.CapacitiveRatioThreshold, out.CapacitivePeriodValue, out.CapacitivePeriodUnit =
			in.Alarm.CapacitiveRatioThreshold, in.Alarm.CapacitivePeriodValue, in.Alarm.CapacitivePeriodUnit
		out.ActiveConsumptionMax, out.ActiveConsumptionMaxPeriodValue, out.ActiveConsumptionMaxPeriodUnit =
			in.Alarm.ActiveConsumptionMax, in.Alarm.ActiveConsumptionMaxPeriodValue, in.Alarm.ActiveConsumptionMaxPeriodUnit
		out.ActiveConsumptionMin, out.ActiveConsumptionMinPeriodValue, out.ActiveConsumptionMinPeriodUnit =
			in.Alarm.ActiveConsumptionMin, in.Alarm.ActiveConsumptionMinPeriodValue, in.Alarm.ActiveConsumptionMinPeriodUnit
	case model.AlarmTypeDataCommunication:
		out.CommunicationThresholdHours = in.Alarm.CommunicationThresholdHours
	case model.AlarmTypeCurrentVoltagePower:
		// R212: voltage is NOT copied even if the caller sent it — Validate has
		// already rejected it, and this makes a bypass impossible.
		out.PowerMax, out.PowerMin = in.Alarm.PowerMax, in.Alarm.PowerMin
	case model.AlarmTypeInvoiceIncrease:
		out.InvoiceThresholdPct = in.Alarm.InvoiceThresholdPct
	}
	return out
}

// normaliseChannels de-duplicates, validates and caps (R231).
func normaliseChannels(in []model.AlarmChannel) ([]model.AlarmChannel, error) {
	seen := map[string]bool{}
	out := make([]model.AlarmChannel, 0, len(in))
	counts := map[model.NotifyChannel]int{}
	for _, c := range in {
		target := strings.TrimSpace(c.Target)
		switch c.Channel {
		case model.NotifyChannelEmail:
			if len(target) > 254 || !emailRe.MatchString(target) {
				return nil, validation("channels", "email")
			}
			target = strings.ToLower(target)
		case model.NotifyChannelSMS:
			if !phoneRe.MatchString(target) {
				return nil, validation("channels", "phone")
			}
		default:
			return nil, validation("channels", "oneof")
		}
		key := string(c.Channel) + "|" + target
		if seen[key] {
			continue
		}
		seen[key] = true
		counts[c.Channel]++
		out = append(out, model.AlarmChannel{Channel: c.Channel, Target: target})
	}
	if counts[model.NotifyChannelEmail] > maxEmailTargets || counts[model.NotifyChannelSMS] > maxSMSTargets {
		return nil, validation("channels", "max")
	}
	return out, nil
}

// visible is R213: at least one attached analyzer inside the Scope, or a Scope
// that spans the whole company.
func (s *Service) visible(ctx context.Context, sc store.Scope, alarmID uuid.UUID) ([]AnalyzerRef, error) {
	attached, err := s.d.Alarms.Analyzers(ctx, sc, alarmID) // already company-scoped
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(attached))
	for _, a := range attached {
		ids = append(ids, a.AnalyzerID)
	}
	var refs []AnalyzerRef
	if len(ids) > 0 {
		// The analyzer repository applies the Scope's building branch, so what
		// comes back IS the intersection.
		rows, err := s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{IDs: ids})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			refs = append(refs, AnalyzerRef{ID: r.ID, InstallationNumber: r.InstallationNumber, BuildingID: r.BuildingID})
		}
	}
	if len(refs) == 0 && !sc.AllBuildings {
		return nil, perr.New(perr.NotFound.Code, perr.NotFound.HTTPStatus, "errors.alarm.notFound")
	}
	return refs, nil
}
```

`Create`/`Update` order: `settingsFor` → `alarm.Validate(out, len(in.AnalyzerIDs))` (map a
`*alarm.ValidationError` onto `perr.Validation.WithParams`) → `normaliseChannels` → repository
`Create`/`Update` → `ReplaceAnalyzers` → `ReplaceChannels` → re-read for the returned `Rule`.
`ReplaceAnalyzers` returning `ErrNotFound` for an out-of-scope analyzer is what
`TestCreateRequiresEveryAnalyzerInScope` asserts, so do **not** pre-filter the ids.

- [ ] **Step 4: Run the whole package**

Run: `go test ./internal/service/alarms/ -count=1 -race` → PASS (10 tests).
Then `golangci-lint run --concurrency 2 ./internal/service/alarms/...` → 0 issues.

- [ ] **Step 5: Extend the isolation sweep**

`internal/store/postgres/scope_isolation_integration_test.go` already runs `TestScopeIsolation`
over every scoped repository. Confirm the alarm repository's new `Notified` path is covered by the
existing table; if the sweep enumerates filters, add the `Notified` case there rather than writing
a new test.

- [ ] **Step 6: Prove the guards red by mutation**

1. In `settingsFor`, add `out.VoltageMax = in.Alarm.VoltageMax` to the power branch →
   `TestCreateRejectsInvalidSettings` still passes (Validate catches it), so **also** temporarily
   weaken `Validate`'s voltage branch: the two together must break a test. If nothing breaks,
   that is a finding — add an assertion that the stored row's `VoltageMax` is nil.
2. In `visible`, change `len(refs) == 0 && !sc.AllBuildings` to `len(refs) == 0 && false` →
   `TestGetHidesARuleWithNoAnalyzerInScope` and `TestRuleWithNoAnalyzersIsCompanyWideOnly` must
   FAIL.
3. Drop the `seen[key]` check → `TestChannelsAreValidatedDeduplicatedAndCapped` must FAIL.
4. In `settingsFor`, copy `in.Alarm.CapacitiveRatioThreshold` unconditionally →
   `TestUpdateIsAFullReplaceOfSettings` must FAIL.
Restore after each.

- [ ] **Step 7: Commit**

```bash
git add internal/service/alarms internal/store/postgres/scope_isolation_integration_test.go
git commit -m "feat(f7): alarm rule CRUD with analyzer-intersected visibility (R213, R230, R231)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: API — alarm routes

**Files:**
- Create: `internal/api/v1/handlers_alarms.go`, `internal/api/v1/dto/alarms.go`
- Modify: `internal/api/v1/routes.go:70-73` (`Table()`), `internal/api/v1/router.go:38-53`
  (`Handlers`), `internal/apiwire/wiring.go`
- Test: `internal/api/v1/alarms_http_integration_test.go`; the existing
  `authz_matrix_test.go` and `route_table_test.go` pick the new rows up automatically

**Interfaces:**
- Consumes: Task 6's `alarms.Service`; `dto.Page`, `dto.PageRequest`, `dto.Decimal`, `dto.IDPath`,
  `dto.CompanyScopeQuery`, `serve`, `mw.ScopeFrom`.
- Produces the DTOs the web client is generated from:

```go
// AlarmSettings is the union of every type's settings; which fields are
// meaningful follows from Type (R212 rejects the two voltage fields outright).
type AlarmSettings struct {
	InductiveRatioThreshold  *Decimal `json:"inductive_ratio_threshold,omitempty"`
	InductivePeriodValue     *int32   `json:"inductive_period_value,omitempty" minimum:"1" maximum:"8760"`
	InductivePeriodUnit      *string  `json:"inductive_period_unit,omitempty" enum:"hours,days"`
	CapacitiveRatioThreshold *Decimal `json:"capacitive_ratio_threshold,omitempty"`
	CapacitivePeriodValue    *int32   `json:"capacitive_period_value,omitempty" minimum:"1" maximum:"8760"`
	CapacitivePeriodUnit     *string  `json:"capacitive_period_unit,omitempty" enum:"hours,days"`
	ActiveConsumptionMax     *Decimal `json:"active_consumption_max,omitempty"`
	ActiveConsumptionMaxPeriodValue *int32  `json:"active_consumption_max_period_value,omitempty" minimum:"1" maximum:"8760"`
	ActiveConsumptionMaxPeriodUnit  *string `json:"active_consumption_max_period_unit,omitempty" enum:"hours,days"`
	ActiveConsumptionMin     *Decimal `json:"active_consumption_min,omitempty"`
	ActiveConsumptionMinPeriodValue *int32  `json:"active_consumption_min_period_value,omitempty" minimum:"1" maximum:"8760"`
	ActiveConsumptionMinPeriodUnit  *string `json:"active_consumption_min_period_unit,omitempty" enum:"hours,days"`
	CommunicationThresholdHours *int32 `json:"communication_threshold_hours,omitempty" minimum:"1" maximum:"8760"`
	PowerMax *Decimal `json:"power_max,omitempty"`
	PowerMin *Decimal `json:"power_min,omitempty"`
	// VoltageMax and VoltageMin exist only so a client sending them gets a
	// 422 naming the field instead of a silent drop (R212). No provider
	// reports voltage; legacy read a field that never existed.
	VoltageMax *Decimal `json:"voltage_max,omitempty"`
	VoltageMin *Decimal `json:"voltage_min,omitempty"`
	InvoiceThresholdPct *Decimal `json:"invoice_threshold_pct,omitempty"`
}

type AlarmChannelDTO struct {
	Channel string `json:"channel" validate:"required,oneof=email sms" required:"true" enum:"email,sms"`
	Target  string `json:"target" validate:"required,max=254" required:"true"`
}

type AlarmFields struct {
	Name        string            `json:"name" validate:"required,max=200" required:"true"`
	Type        string            `json:"type" validate:"required,oneof=reactive_limit data_communication current_voltage_power invoice_increase" required:"true" enum:"reactive_limit,data_communication,current_voltage_power,invoice_increase"`
	IsEnabled   bool              `json:"is_enabled"`
	AnalyzerIDs []uuid.UUID       `json:"analyzer_ids" validate:"required,min=1,max=200" required:"true"`
	Channels    []AlarmChannelDTO `json:"channels" validate:"max=70,dive"`
	NotificationFrequencyValue *int32  `json:"notification_frequency_value,omitempty" minimum:"1" maximum:"8760"`
	NotificationFrequencyUnit  *string `json:"notification_frequency_unit,omitempty" enum:"hours,days"`
	Settings    AlarmSettings     `json:"settings"`
}

type AlarmUpdateRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	AlarmFields
}

type AlarmAnalyzer struct {
	ID                 uuid.UUID `json:"id" required:"true"`
	InstallationNumber string    `json:"installation_number" required:"true"`
	BuildingID         *uuid.UUID `json:"building_id"`
}

type Alarm struct {
	ID        uuid.UUID       `json:"id" required:"true"`
	Name      string          `json:"name" required:"true"`
	Type      string          `json:"type" required:"true" enum:"reactive_limit,data_communication,current_voltage_power,invoice_increase"`
	IsEnabled bool            `json:"is_enabled" required:"true"`
	Analyzers []AlarmAnalyzer `json:"analyzers" required:"true"`
	Channels  []AlarmChannelDTO `json:"channels" required:"true"`
	NotificationFrequencyValue *int32  `json:"notification_frequency_value"`
	NotificationFrequencyUnit  *string `json:"notification_frequency_unit"`
	Settings  AlarmSettings   `json:"settings" required:"true"`
	CreatedAt time.Time       `json:"created_at" required:"true"`
	UpdatedAt time.Time       `json:"updated_at" required:"true"`
}

type AlarmListRequest struct {
	CompanyScopeQuery
	PageRequest
	Type      *string    `query:"type" json:"-" enum:"reactive_limit,data_communication,current_voltage_power,invoice_increase"`
	IsEnabled *bool      `query:"is_enabled" json:"-"`
	AnalyzerID *uuid.UUID `query:"analyzer_id" json:"-"`
}

type AlarmEventsRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	CompanyScopeQuery
	PageRequest
	From *time.Time `query:"from" json:"-"`
	To   *time.Time `query:"to" json:"-"`
}

type AlarmEvent struct {
	ID          uuid.UUID  `json:"id" required:"true"`
	AnalyzerID  *uuid.UUID `json:"analyzer_id"`
	TriggeredAt time.Time  `json:"triggered_at" required:"true"`
	Message     string     `json:"message" required:"true"`
	// Detail is the curated object of R224, never a provider string.
	Detail            json.RawMessage `json:"detail,omitempty"`
	NotifiedAt        *time.Time      `json:"notified_at"`
	NotificationError *string         `json:"notification_error"`
}

type AlarmEvaluateRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	CompanyScopeQuery
	// Notify runs the real firing path instead of a dry run (R223).
	Notify bool `query:"notify" json:"-"`
}

// AlarmEvaluationBreach is one crossed threshold, already rendered.
type AlarmEvaluationBreach struct {
	Field     string  `json:"field" required:"true"`
	Measured  Decimal `json:"measured" required:"true"`
	Threshold Decimal `json:"threshold" required:"true"`
	Message   string  `json:"message" required:"true"`
}

type AlarmEvaluationAnalyzer struct {
	AnalyzerID         uuid.UUID               `json:"analyzer_id" required:"true"`
	InstallationNumber string                  `json:"installation_number" required:"true"`
	Fired              bool                    `json:"fired" required:"true"`
	Breaches           []AlarmEvaluationBreach `json:"breaches" required:"true"`
	// NoVerdict names the settings that could not be decided for lack of data.
	NoVerdict []string `json:"no_verdict" required:"true"`
}

type AlarmEvaluation struct {
	EvaluatedAt time.Time                 `json:"evaluated_at" required:"true"`
	DryRun      bool                      `json:"dry_run" required:"true"`
	Analyzers   []AlarmEvaluationAnalyzer `json:"analyzers" required:"true"`
	// NotificationsSent is 0 on a dry run and when the frequency suppressed
	// every firing (R218).
	NotificationsSent int `json:"notifications_sent" required:"true"`
}
```

- [ ] **Step 1: Write the failing test**

`internal/api/v1/alarms_http_integration_test.go`, on the existing harness
(`harness_integration_test.go`) with the `integration` build tag:

```go
//go:build integration

func TestAlarmCRUDRoundTrip(t *testing.T) {
	h := newHarness(t)
	body := map[string]any{"name": "İletişim", "type": "data_communication", "is_enabled": true,
		"analyzer_ids": []string{h.AnalyzerA.String()},
		"channels":     []map[string]string{{"channel": "email", "target": "ops@example.com"}},
		"settings":     map[string]any{"communication_threshold_hours": 6}}

	created := h.Post(t, roleCompanyAdmin, "/api/v1/alarms", body)
	require.Equal(t, http.StatusCreated, created.Code)
	var alarm dto.Alarm
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &alarm))
	require.Len(t, alarm.Analyzers, 1)
	require.Equal(t, "ops@example.com", alarm.Channels[0].Target)

	listed := h.Get(t, roleBuildingAdmin, "/api/v1/alarms")
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listed.Body.String(), alarm.ID.String())

	del := h.Delete(t, roleCompanyAdmin, "/api/v1/alarms/"+alarm.ID.String())
	require.Equal(t, http.StatusNoContent, del.Code)
	require.Equal(t, http.StatusNotFound, h.Get(t, roleCompanyAdmin, "/api/v1/alarms/"+alarm.ID.String()).Code)
}

func TestAlarmVoltageIsRejectedWith422(t *testing.T) {
	h := newHarness(t)
	res := h.Post(t, roleCompanyAdmin, "/api/v1/alarms", map[string]any{
		"name": "Gerilim", "type": "current_voltage_power", "analyzer_ids": []string{h.AnalyzerA.String()},
		"settings": map[string]any{"power_max": "100", "voltage_max": "400"}})
	require.Equal(t, http.StatusUnprocessableEntity, res.Code)
	require.Contains(t, res.Body.String(), "voltage_max")
	require.Contains(t, res.Body.String(), "validation_failed")
}

func TestAlarmOfAnotherCompanyIs404(t *testing.T) {
	h := newHarness(t)
	foreign := h.SeedForeignAlarm(t)
	require.Equal(t, http.StatusNotFound, h.Get(t, roleCompanyAdmin, "/api/v1/alarms/"+foreign.String()).Code)
	require.Equal(t, http.StatusNotFound, h.Get(t, roleCompanyAdmin, "/api/v1/alarms/"+foreign.String()+"/events").Code)
	require.Equal(t, http.StatusNotFound, h.Post(t, roleCompanyAdmin, "/api/v1/alarms/"+foreign.String()+"/evaluate", nil).Code)
}

func TestAlarmDryRunWritesNothing(t *testing.T) {
	h := newHarness(t)
	id := h.SeedCommsAlarm(t, 1) // a 1-hour threshold on an analyzer with stale readings
	res := h.Post(t, roleCompanyAdmin, "/api/v1/alarms/"+id.String()+"/evaluate", nil)
	require.Equal(t, http.StatusOK, res.Code)

	var out dto.AlarmEvaluation
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &out))
	require.True(t, out.DryRun)
	require.True(t, out.Analyzers[0].Fired)
	require.Zero(t, out.NotificationsSent)

	// R223: a dry run leaves no event behind.
	events := h.Get(t, roleCompanyAdmin, "/api/v1/alarms/"+id.String()+"/events")
	require.Equal(t, http.StatusOK, events.Code)
	require.Contains(t, events.Body.String(), `"items":[]`)
}
```

- [ ] **Step 2: Run it and watch it fail**

```bash
EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' \
EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
go test ./internal/api/v1/ -tags=integration -count=1 -parallel 4 -run TestAlarm
```
Expected: FAIL — no such routes (404 on every call) and `undefined: dto.Alarm`.

- [ ] **Step 3: Write the routes, DTOs and handlers**

`alarmRoutes()` mirrors the route table above, using `auth.AllRoles` for the three reads,
`auth.Roles(roleA, roleCA, roleBA)` for the three writes and `auth.Roles(roleA, roleCA)` for
`evaluate` (which also carries `NoIdempotency: true`, R223, and `Entity: "alarm"` on the
mutating rows so the audit middleware records them). Add `alarmRoutes()` to `Table()`'s
`slices.Concat`, add `Alarms *alarms.Service` to `Handlers`, and build it in
`internal/apiwire/wiring.go` next to `calendarService`:

```go
	alarmService, err := alarms.New(alarms.Deps{
		Alarms:    postgres.NewAlarmRepository(pool),
		Analyzers: postgres.NewAnalyzerRepository(pool),
		Clock:     opts.Clock,
	})
	if err != nil {
		return nil, nil, err
	}
```

The `evaluate` handler calls `alarms.Service.DryRun`. **This task declares** the three types and
the dry-run half in a new `internal/service/alarms/evaluate.go`; Task 9 extends the same file with
`Run` and the real firing path:

```go
// Evaluation is what a dry run returns and what a real run summarises.
type Evaluation struct {
	EvaluatedAt       time.Time
	DryRun            bool
	Analyzers         []AnalyzerEvaluation
	NotificationsSent int
}

type AnalyzerEvaluation struct {
	AnalyzerID         uuid.UUID
	InstallationNumber string
	Verdict            alarm.Verdict
	Lines              []string
}

// DryRun is R223. notify=true runs the real path; the dry run writes nothing.
func (s *Service) DryRun(ctx context.Context, sc store.Scope, id uuid.UUID, notify bool) (Evaluation, error)

// EvaluateRule is the one-rule path DryRun and Task 9's Run share. With
// dryRun true it loads, evaluates and describes, and writes nothing at all.
func (s *Service) EvaluateRule(ctx context.Context, sc store.Scope, rule Rule, now time.Time, dryRun bool) (Evaluation, error)
```

Implement the `dryRun == true` path here — it needs no mail, no job and no events, only the reads
the rule's type calls for (the four load-profile `Range` calls, `AnalyzerRef.LastReadingAt`, the
48-hour `Latest`, or the two-bill `List`), then `alarm.Evaluate*` and `alarm.Describe`. For
`notify=true`, return `perr.Unavailable` with message key `errors.alarm.notifyUnavailable` until
Task 9 lands; `TestAlarmDryRunWritesNothing` covers the path that exists and Task 9 adds the
`notify=true` test. The invoice dry run must **not** call `MarkBillFired` — a dry run that claimed
a bill would silence the real firing (R217, R223 together).

- [ ] **Step 4: Run the whole package**

```bash
go test ./internal/api/v1/... -count=1 -race
EKOKOD_TEST_PG_DSN=... EKOKOD_TEST_REDIS_URL=... TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
go test ./internal/api/v1/ -tags=integration -count=1 -parallel 4
make openapi && cd web && pnpm check:api && pnpm typecheck
```
Expected: PASS. `authz_matrix_test.go` now exercises ten new rows × six roles with no edit — if it
fails, the route's `Roles` set disagrees with the permission table of Task 1; fix the route, not
the matrix.

- [ ] **Step 5: Prove the guards red by mutation**

1. Widen `POST /alarms`'s `Roles` to `auth.AllRoles` → `TestAuthorizationMatrix` must FAIL for the
   read-only roles.
2. Drop `VoltageMax` from `dto.AlarmSettings` → `TestAlarmVoltageIsRejectedWith422` must FAIL
   (the field is silently ignored, which is exactly the failure mode R212 forbids).
3. Make `DryRun` create an event → `TestAlarmDryRunWritesNothing` must FAIL.
4. Change the out-of-scope answer to 403 → `TestAlarmOfAnotherCompanyIs404` must FAIL.
Restore after each.

- [ ] **Step 6: Commit**

```bash
git add internal/api internal/apiwire web/src/lib/api/schema.d.ts
git commit -m "feat(f7): alarm CRUD, events and dry-run routes (05 §10, R212, R223)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Alarm mail templates and the notifier

**Files:**
- Create: `internal/mail/templates/alarm_fired.tr.txt`, `internal/mail/templates/alarm_fired.en.txt`
- Modify: `internal/mail/templates.go` (the `subjects` map and one constructor)
- Create: `internal/service/alarms/notify.go`, `internal/service/alarms/notify_test.go`
- Test: `internal/mail/templates_test.go` (extend)

**Interfaces:**
- Consumes: `mail.Sender`, `mail.Message`, `store.SMTPRepository` (`Get`, `OpenPassword`),
  `store.AlarmRepository.MarkNotified`, `store.OpsRepository.AppendMessage`, Task 4's
  `alarm.Describe`.
- Produces:

```go
// In internal/mail:

// AlarmFired is the alarm notification body (R221, plain text in both locales).
// The caller sets To.
func AlarmFired(locale, alarmName, analyzerLabel string, lines []string, firedAt time.Time) Message

// In internal/service/alarms:

// Notification is one event's delivery job.
type Notification struct {
	CompanyID  uuid.UUID
	AlarmID    uuid.UUID
	EventID    uuid.UUID
	AnalyzerID *uuid.UUID
}

// NotifyDeps is what Notify needs beyond the CRUD service's own deps.
type NotifyDeps struct {
	SMTP   store.SMTPRepository
	Mail   mail.Sender
	Ops    store.OpsRepository
	Locale string // the company's outbound locale; "tr" unless configured
}

// Notify delivers one alarm event and records the outcome. It returns nil for
// a delivery failure (R222: "it does not fail the job") and an error only when
// the event itself cannot be read or marked, which IS worth a retry.
func (s *Service) Notify(ctx context.Context, n Notification) error
```

- [ ] **Step 1: Write the failing test**

`internal/service/alarms/notify_test.go` — the sender is a fake that records or fails:

```go
type fakeSender struct {
	sent []mail.Message
	err  error
}

func (f *fakeSender) Send(_ context.Context, _ model.SMTPSettings, _ []byte, m mail.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

func TestNotifySendsToEveryEmailTargetOnce(t *testing.T) {
	t.Parallel()
	svc, fakes := newNotifyService(t, withChannels(
		email("ops@example.com"), email("boss@example.com"), sms("+905551234567")))
	require.NoError(t, svc.Notify(t.Context(), fakes.notification))

	require.Len(t, fakes.sender.sent, 1, "one message addressed to both recipients, not one each")
	require.ElementsMatch(t, []string{"ops@example.com", "boss@example.com"}, fakes.sender.sent[0].To)
	require.Contains(t, fakes.sender.sent[0].Text, "6 saat")
	require.NotNil(t, fakes.alarms.notified[fakes.notification.EventID])
	require.Nil(t, fakes.alarms.notifyErr[fakes.notification.EventID])
}

func TestNotifyRecordsSMSAsNotSent(t *testing.T) {
	t.Parallel()
	// R211: the numbers are stored, nothing is sent, and Messages says so.
	svc, fakes := newNotifyService(t, withChannels(email("ops@example.com"), sms("+905551234567")))
	require.NoError(t, svc.Notify(t.Context(), fakes.notification))

	var warnings []model.OperationalMessage
	for _, m := range fakes.ops.messages {
		if m.Status == "warning" {
			warnings = append(warnings, m)
		}
	}
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0].Message, "SMS")
	require.Contains(t, warnings[0].Message, "1")
	// Never the number itself: Messages rows are exported.
	require.NotContains(t, warnings[0].Message, "905551234567")
	require.NotContains(t, derefOr(warnings[0].Detail, ""), "905551234567")
}

func TestNotifyWithOnlySMSTargetsSendsNoMailButStillRecords(t *testing.T) {
	t.Parallel()
	svc, fakes := newNotifyService(t, withChannels(sms("+905551234567")))
	require.NoError(t, svc.Notify(t.Context(), fakes.notification))
	require.Empty(t, fakes.sender.sent)
	// The event is not "delivered": nobody was actually told.
	require.Nil(t, fakes.alarms.notified[fakes.notification.EventID])
	require.NotNil(t, fakes.alarms.notifyErr[fakes.notification.EventID])
}

func TestNotifyFailedSendIsRecordedAndDoesNotFailTheJob(t *testing.T) {
	t.Parallel()
	// 09 §F7 acceptance: recorded on the event, surfaced in Messages, job survives.
	svc, fakes := newNotifyService(t, withChannels(email("ops@example.com")))
	fakes.sender.err = errors.New("535 authentication failed")

	require.NoError(t, svc.Notify(t.Context(), fakes.notification), "R222")
	require.Nil(t, fakes.alarms.notified[fakes.notification.EventID])
	recorded := fakes.alarms.notifyErr[fakes.notification.EventID]
	require.NotNil(t, recorded)
	require.Contains(t, *recorded, "535")

	var errs int
	for _, m := range fakes.ops.messages {
		if m.Status == "error" && m.Kind == "alarm" {
			errs++
		}
	}
	require.Equal(t, 1, errs)
}

func TestNotifyMissingSMTPSettingsIsRecordedNotRetried(t *testing.T) {
	t.Parallel()
	svc, fakes := newNotifyService(t, withChannels(email("ops@example.com")), withoutSMTP())
	require.NoError(t, svc.Notify(t.Context(), fakes.notification))
	require.NotNil(t, fakes.alarms.notifyErr[fakes.notification.EventID])
	require.Contains(t, *fakes.alarms.notifyErr[fakes.notification.EventID], "SMTP")
}

func TestNotifyNeverLeaksTheSMTPPassword(t *testing.T) {
	t.Parallel()
	svc, fakes := newNotifyService(t, withChannels(email("ops@example.com")),
		withSMTPPassword("s3cr3t-pass"))
	fakes.sender.err = errors.New("dial tcp: bad handshake")
	require.NoError(t, svc.Notify(t.Context(), fakes.notification))
	for _, m := range fakes.ops.messages {
		require.NotContains(t, m.Message, "s3cr3t-pass")
		require.NotContains(t, derefOr(m.Detail, ""), "s3cr3t-pass")
	}
	require.NotContains(t, *fakes.alarms.notifyErr[fakes.notification.EventID], "s3cr3t-pass")
}
```

Extend `internal/mail/templates_test.go`:

```go
func TestAlarmFiredRendersBothLocales(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 18, 14, 30, 0, 0, time.UTC)
	tr := mail.AlarmFired("tr", "İletişim", "A-1", []string{"Analizör 7 saattir veri göndermiyor"}, at)
	require.Contains(t, tr.Subject, "Alarm")
	require.Contains(t, tr.Text, "İletişim")
	require.Contains(t, tr.Text, "A-1")
	require.Contains(t, tr.Text, "7 saattir")

	en := mail.AlarmFired("en", "Comms", "A-1", []string{"No data for 7 hours"}, at)
	require.NotEqual(t, tr.Subject, en.Subject)
	require.Contains(t, en.Text, "No data for 7 hours")

	// An unknown locale falls back to Turkish, as render() already does.
	require.Equal(t, tr.Subject, mail.AlarmFired("de", "İletişim", "A-1",
		[]string{"Analizör 7 saattir veri göndermiyor"}, at).Subject)
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./internal/mail/ ./internal/service/alarms/ -count=1`
Expected: FAIL — `undefined: mail.AlarmFired`, `undefined: (*alarms.Service).Notify`.

- [ ] **Step 3: Write the templates and the notifier**

`internal/mail/templates/alarm_fired.tr.txt`:

```
Alarm tetiklendi: {{.AlarmName}}

Analizör: {{.AnalyzerLabel}}
Zaman: {{.FiredAt}}

Tespit edilen durumlar:
{{range .Lines}}- {{.}}
{{end}}
Bu ileti ekokod tarafından otomatik olarak gönderildi. Lütfen yanıtlamayın.
```

`internal/mail/templates/alarm_fired.en.txt`:

```
Alarm triggered: {{.AlarmName}}

Analyzer: {{.AnalyzerLabel}}
Time: {{.FiredAt}}

What was detected:
{{range .Lines}}- {{.}}
{{end}}
This message was sent automatically by ekokod. Please do not reply.
```

Add to `subjects`:

```go
	"alarm_fired": {"tr": "Alarm tetiklendi", "en": "Alarm triggered"},
```

and the constructor:

```go
// AlarmFired is R221. The timestamp is rendered in Europe/Istanbul by the
// caller, so the template only formats what it is given.
func AlarmFired(locale, alarmName, analyzerLabel string, lines []string, firedAt time.Time) Message {
	return render("alarm_fired", locale, map[string]any{
		"AlarmName": alarmName, "AnalyzerLabel": analyzerLabel, "Lines": lines,
		"FiredAt": firedAt.Format("02.01.2006 15:04"),
	})
}
```

`internal/service/alarms/notify.go`:

```go
// Notify is R211, R222 and R224 in one place: mail goes out, SMS is recorded as
// not sent, and every failure lands on the event and in Messages without
// failing the task.
func (s *Service) Notify(ctx context.Context, n Notification) error {
	sc := store.SystemScope(n.CompanyID)
	rule, err := s.Get(ctx, sc, n.AlarmID)
	if err != nil {
		return err // unreadable rule IS worth a retry
	}
	var emails, smsCount = []string{}, 0
	for _, c := range rule.Channels {
		switch c.Channel {
		case model.NotifyChannelEmail:
			emails = append(emails, c.Target)
		case model.NotifyChannelSMS:
			smsCount++
		}
	}
	events, err := s.d.Alarms.ListEvents(ctx, sc, store.AlarmEventFilter{
		AlarmID: &n.AlarmID, Page: store.Page{Limit: 200}})
	if err != nil {
		return err
	}
	event, ok := findEvent(events, n.EventID)
	if !ok {
		return nil // the event is gone; nothing to deliver
	}
	label := analyzerLabel(rule, n.AnalyzerID)
	lines := linesFrom(event) // event.Detail's rendered breach sentences

	now := s.d.Clock.Now()
	if smsCount > 0 {
		// R211: say it once, with a count and never a number.
		s.appendMessage(ctx, sc, n, "warning", fmt.Sprintf(
			"SMS bildirimi henüz etkin değil: %d numaraya ulaşılmadı (%s)", smsCount, rule.Alarm.Name))
	}
	if len(emails) == 0 {
		reason := "bildirim kanalı yok: yalnızca SMS numarası tanımlı"
		if smsCount == 0 {
			reason = "bildirim kanalı tanımlı değil"
		}
		return s.markFailed(ctx, sc, n, reason, nil)
	}
	settings, err := s.nd.SMTP.Get(ctx, sc)
	if err != nil {
		return s.markFailed(ctx, sc, n, "şirketin SMTP ayarları yok", err)
	}
	password, err := s.nd.SMTP.OpenPassword(ctx, sc)
	if err != nil {
		return s.markFailed(ctx, sc, n, "SMTP parolası okunamadı", err)
	}
	// s.loc is Europe/Istanbul, loaded in New. NOT dto.Istanbul: internal/service
	// may not import internal/api (TestServiceLayerImportBoundaries), which is
	// why internal/service/billing loads its own location the same way.
	msg := mail.AlarmFired(s.nd.Locale, rule.Alarm.Name, label, lines, now.In(s.loc))
	msg.To = emails
	if err := s.nd.Mail.Send(ctx, settings, password, msg); err != nil {
		return s.markFailed(ctx, sc, n, "e-posta gönderilemedi", err)
	}
	if err := s.d.Alarms.MarkNotified(ctx, sc, n.EventID, now, nil); err != nil {
		return err
	}
	s.appendMessage(ctx, sc, n, "warning", fmt.Sprintf("Alarm bildirimi gönderildi: %s (%s)", rule.Alarm.Name, label))
	return nil
}

// markFailed records the failure on the event and in Messages, then returns nil
// (R222). The cause's text is scrubbed of any fragment the secret package knows.
func (s *Service) markFailed(ctx context.Context, sc store.Scope, n Notification, reason string, cause error) error {
	text := reason
	if cause != nil {
		text = reason + ": " + secret.Scrub(cause.Error())
	}
	if err := s.d.Alarms.MarkNotified(ctx, sc, n.EventID, time.Time{}, &text); err != nil {
		return err // could not even record it — retry
	}
	s.appendMessage(ctx, sc, n, "error", "Alarm bildirimi gönderilemedi: "+text)
	return nil
}
```

> `MarkNotified` with a zero `at` and a non-nil error must leave `notified_at` NULL — check the F1
> implementation and, if it stamps the zero time instead, pass the sentinel the repository's own
> test uses. This is exactly the "fired and nobody was told" state `model.AlarmEvent` documents.
> If `secret.Scrub` does not exist under that name, use whatever `internal/platform/secret`
> exposes for the same job (`secret.Wrap`'s fragment handling) — never write a raw SMTP reply.

- [ ] **Step 4: Run the packages**

Run: `go test ./internal/mail/ ./internal/service/alarms/ -count=1 -race` → PASS.

- [ ] **Step 5: Prove the guards red by mutation**

1. Return the send error from `Notify` instead of `markFailed` →
   `TestNotifyFailedSendIsRecordedAndDoesNotFailTheJob` must FAIL.
2. Put the SMS numbers into the warning message → `TestNotifyRecordsSMSAsNotSent` must FAIL.
3. Mark the event notified on the SMS-only path →
   `TestNotifyWithOnlySMSTargetsSendsNoMailButStillRecords` must FAIL.
4. Drop the scrubbing in `markFailed` and make the fake sender's error contain the password →
   `TestNotifyNeverLeaksTheSMTPPassword` must FAIL.
Restore after each.

- [ ] **Step 6: Commit**

```bash
git add internal/mail internal/service/alarms
git commit -m "feat(f7): alarm mail in both locales, SMS recorded as not sent (R211, R221, R222)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: The job pipeline — dispatch, evaluate, notify

**Files:**
- Create: `internal/job/alarm.go`, `internal/job/alarm_test.go`
- Create: `internal/service/alarms/evaluate.go`,
  `internal/service/alarms/evaluate_integration_test.go`
- Modify: `internal/job/task.go` (`Handlers`, `Register`), `internal/scheduler/scheduler.go`
  (`entries`), `internal/platform/config/config.go` + `load.go` (`Schedule.Alarms`),
  `internal/worker/worker.go`, `internal/apiwire/wiring.go`
- Test: `internal/job/alarm_test.go`, `internal/scheduler/scheduler_test.go` (extend),
  `internal/service/alarms/evaluate_integration_test.go`

**Interfaces:**
- Consumes: Tasks 2–4 (`alarm.Evaluate*`, `alarm.Suppressed`, `alarm.Describe`), Task 6
  (`alarms.Service`), Task 8 (`Notify`), `store.ReadingRepository`, `store.BillRepository`,
  `store.OpsRepository`, `store.TenancyRepository` (for the company list), `job.TaskOptions`.
- Produces:

```go
// In internal/job:

const (
	TypeAlarmDispatch = "alarm.dispatch" // platform tick, 01 §8 hourly
	TypeAlarmEvaluate = "alarm.evaluate" // one company
	TypeAlarmNotify   = "alarm.notify"   // one event
)

type AlarmEvaluatePayload struct {
	CompanyID uuid.UUID `json:"company_id"`
	// Hour is the tick this evaluation belongs to, and the TaskID's suffix
	// (R225): one evaluation per company per hour, cron or manual.
	Hour time.Time `json:"hour"`
}

type AlarmNotifyPayload struct {
	CompanyID  uuid.UUID  `json:"company_id"`
	AlarmID    uuid.UUID  `json:"alarm_id"`
	EventID    uuid.UUID  `json:"event_id"`
	AnalyzerID *uuid.UUID `json:"analyzer_id,omitempty"`
}

func AlarmEvaluateTaskID(p AlarmEvaluatePayload) string // "alarm.evaluate:<company>:<RFC3339 hour>"
func AlarmNotifyTaskID(p AlarmNotifyPayload) string     // "alarm.notify:<event>"

func NewAlarmDispatchTask(o TaskOptions) (*asynq.Task, error)
func NewAlarmEvaluateTask(p AlarmEvaluatePayload, o TaskOptions) (*asynq.Task, error)
func NewAlarmNotifyTask(p AlarmNotifyPayload, o TaskOptions) (*asynq.Task, error)

type AlarmDispatcher interface{ Dispatch(ctx context.Context) error }
type AlarmEvaluator interface{ Evaluate(ctx context.Context, p AlarmEvaluatePayload) error }
type AlarmNotifier interface{ Notify(ctx context.Context, p AlarmNotifyPayload) error }

// In internal/service/alarms — Evaluation, AnalyzerEvaluation, DryRun and
// EvaluateRule's dry-run half already exist from Task 7; this task adds Run,
// EvaluateDeps and EvaluateRule's writing half to the same file.

// EvaluateDeps are the reads and the enqueue the real run needs.
type EvaluateDeps struct {
	Readings store.ReadingRepository
	Bills    store.BillRepository
	Ops      store.OpsRepository
	Enqueue  func(ctx context.Context, p job.AlarmNotifyPayload) error
}

// Run evaluates every enabled rule of one company, records events and job_runs,
// and enqueues notifications. It is idempotent per hour through R225's TaskID.
func (s *Service) Run(ctx context.Context, companyID uuid.UUID, now time.Time) error
```

- [ ] **Step 1: Write the failing tests**

`internal/job/alarm_test.go` — the task plumbing, no database:

```go
func TestAlarmEvaluateTaskIDIsOnePerCompanyPerHour(t *testing.T) {
	t.Parallel()
	// R225: a manual trigger inside the cron tick's own hour is de-duplicated
	// by asynq rather than notifying twice.
	company := uuid.New()
	hour := time.Date(2026, 9, 18, 14, 0, 0, 0, time.UTC)
	a := job.AlarmEvaluateTaskID(job.AlarmEvaluatePayload{CompanyID: company, Hour: hour})
	b := job.AlarmEvaluateTaskID(job.AlarmEvaluatePayload{CompanyID: company, Hour: hour.Add(37 * time.Minute)})
	require.Equal(t, a, b, "the payload carries the hour already truncated")

	next := job.AlarmEvaluateTaskID(job.AlarmEvaluatePayload{CompanyID: company, Hour: hour.Add(time.Hour)})
	require.NotEqual(t, a, next)
}

func TestNewAlarmTasksRejectEmptyPayloads(t *testing.T) {
	t.Parallel()
	_, err := job.NewAlarmEvaluateTask(job.AlarmEvaluatePayload{}, job.TaskOptions{})
	require.Error(t, err)
	_, err = job.NewAlarmNotifyTask(job.AlarmNotifyPayload{CompanyID: uuid.New()}, job.TaskOptions{})
	require.Error(t, err, "an event id is required")
}

func TestRegisterRoutesAlarmTypesOnlyWhenHandlersArePresent(t *testing.T) {
	t.Parallel()
	// The same nil-tolerant contract every other task type follows.
	mux := asynq.NewServeMux()
	job.Register(mux, &job.Handlers{Log: slog.Default()})
	require.Error(t, mux.Handler(asynq.NewTask(job.TypeAlarmEvaluate, nil)).ProcessTask(t.Context(),
		asynq.NewTask(job.TypeAlarmEvaluate, nil)), "no handler registered")
}
```

`internal/service/alarms/evaluate_integration_test.go` — the 09 §F7 acceptance criteria, against
the real database:

```go
//go:build integration

// 09 §F7: each type fires on a fixture that should trigger it and stays silent
// on one that should not — one test per type, per direction.
func TestEvaluateFiresPerTypeAndStaysSilent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seed    func(t *testing.T, f *evalFixture) uuid.UUID // returns the alarm id
		wantFire bool
	}{
		{"reactive fires", seedReactiveBreaching, true},
		{"reactive silent", seedReactiveCompliant, false},
		{"comms fires", seedCommsStale, true},
		{"comms silent", seedCommsFresh, false},
		{"power fires", seedPowerOverMax, true},
		{"power silent", seedPowerInsideBounds, false},
		{"invoice fires", seedInvoiceIncrease, true},
		{"invoice silent", seedInvoiceFlat, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEvalFixture(t)
			id := tc.seed(t, f)
			require.NoError(t, f.svc.Run(t.Context(), f.CompanyID, f.Now))

			events, err := f.alarms.ListEvents(t.Context(), f.Scope, store.AlarmEventFilter{AlarmID: &id})
			require.NoError(t, err)
			if tc.wantFire {
				require.Len(t, events, 1)
			} else {
				require.Empty(t, events, "R219: a non-firing evaluation writes no event")
			}
		})
	}
}

func TestEvaluateFrequencySuppressesThenAllows(t *testing.T) {
	f := newEvalFixture(t)
	id := seedCommsStaleWithFrequency(t, f, 6, model.PeriodUnitHours)

	require.NoError(t, f.svc.Run(t.Context(), f.CompanyID, f.Now))
	require.Len(t, f.enqueued, 1, "the first firing notifies")
	f.markDelivered(t, id, f.Now)

	// R218: a second firing inside the window records the event but sends nothing.
	require.NoError(t, f.svc.Run(t.Context(), f.CompanyID, f.Now.Add(time.Hour)))
	require.Len(t, f.enqueued, 1)
	events := f.events(t, id)
	require.Len(t, events, 2)
	require.Contains(t, string(events[0].Detail), "notification_suppressed")

	// Past the window it notifies again.
	require.NoError(t, f.svc.Run(t.Context(), f.CompanyID, f.Now.Add(7*time.Hour)))
	require.Len(t, f.enqueued, 2)
}

func TestEvaluateInvoiceFiresOnceAcrossRepeatedRuns(t *testing.T) {
	f := newEvalFixture(t)
	id := seedInvoiceIncrease(t, f)

	for i := 0; i < 3; i++ {
		require.NoError(t, f.svc.Run(t.Context(), f.CompanyID, f.Now.Add(time.Duration(i)*time.Hour)))
	}
	// R217: MarkBillFired claims the pair once, so three evaluations fire once.
	require.Len(t, f.events(t, id), 1)
	require.Len(t, f.enqueued, 1)
}

func TestEvaluateRecordsOneJobRunAndOneSummaryMessage(t *testing.T) {
	f := newEvalFixture(t)
	seedCommsStale(t, f)
	seedCommsFresh(t, f)
	require.NoError(t, f.svc.Run(t.Context(), f.CompanyID, f.Now))

	runs, err := f.ops.ListRuns(t.Context(), f.Scope, store.JobRunFilter{JobType: ptr("alarm.evaluate")})
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "success", runs[0].Status)
	require.EqualValues(t, 2, runs[0].Processed)
	require.NotNil(t, runs[0].FinishedAt)

	msgs, err := f.ops.ListMessages(t.Context(), f.Scope,
		store.MessageFilter{Kinds: []string{"job"}, Categories: []string{"alarm-evaluate"}})
	require.NoError(t, err)
	require.Len(t, msgs, 1, "R219: one summary, not one row per rule")
}

func TestEvaluateOneFailingRuleDoesNotAbortTheRun(t *testing.T) {
	f := newEvalFixture(t)
	broken := seedRuleWithDeletedAnalyzer(t, f) // its only analyzer is soft-deleted
	healthy := seedCommsStale(t, f)

	require.NoError(t, f.svc.Run(t.Context(), f.CompanyID, f.Now), "01 §8: isolated failure")
	require.Len(t, f.events(t, healthy), 1)
	require.Empty(t, f.events(t, broken))

	runs, err := f.ops.ListRuns(t.Context(), f.Scope, store.JobRunFilter{JobType: ptr("alarm.evaluate")})
	require.NoError(t, err)
	require.Equal(t, "partial", runs[0].Status)
	require.EqualValues(t, 1, runs[0].Failed)
}

func TestEvaluateSkipsDisabledRules(t *testing.T) {
	f := newEvalFixture(t)
	id := seedCommsStale(t, f)
	f.disable(t, id)
	require.NoError(t, f.svc.Run(t.Context(), f.CompanyID, f.Now))
	require.Empty(t, f.events(t, id))
}
```

- [ ] **Step 2: Run them and watch them fail**

```bash
go test ./internal/job/ -count=1 -run TestAlarm
EKOKOD_TEST_PG_DSN=... EKOKOD_TEST_REDIS_URL=... TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
go test ./internal/service/alarms/ -tags=integration -count=1 -parallel 4
```
Expected: FAIL — `undefined: job.TypeAlarmEvaluate`, `undefined: (*alarms.Service).Run`.

- [ ] **Step 3: Write the task plumbing**

`internal/job/alarm.go` follows `internal/job/billing.go` line for line: payload structs,
`AlarmEvaluateTaskID` / `AlarmNotifyTaskID`, three constructors with `asynq.Timeout` (dispatch 30
min, evaluate 10 min, notify 2 min) and `integMaxRetryOptions(o)`, three interfaces, three
handlers wrapping `ClassifyForRetry` (and `SkipRetry` on a decode failure). `NewAlarmEvaluateTask`
truncates `p.Hour` to the hour before encoding, which is what
`TestAlarmEvaluateTaskIDIsOnePerCompanyPerHour` pins. Register the three types in `Register`,
each behind its own nil check.

Scheduler entry, next to `billingDispatch`:

```go
	alarmDispatch, err := job.NewAlarmDispatchTask(retryOpts)
	if err != nil {
		return nil, err
	}
	// …
		{Cron: s.cfg.Schedule.Alarms, Task: alarmDispatch},
```

`config.Schedule.Alarms` defaults to `"0 * * * *"` (01 §8: hourly). Add it to the config struct,
`load.go`'s defaults and `docs/rewrite/appendix/env-reference.md`'s table.

- [ ] **Step 4: Write the evaluation service**

`internal/service/alarms/evaluate.go`:

```go
// Run is 01 §8's job contract for the alarm check: idempotent per hour (R225),
// isolated per rule, and observable through job_runs plus one summary message.
func (s *Service) Run(ctx context.Context, companyID uuid.UUID, now time.Time) error {
	sc := store.SystemScope(companyID)
	enabled := true
	rules, err := s.List(ctx, sc, store.AlarmFilter{IsEnabled: &enabled, Page: store.Page{Limit: 500}})
	if err != nil {
		return err
	}
	scopeJSON, _ := json.Marshal(map[string]any{"company_id": companyID, "rules": len(rules)})
	run, err := s.ed.Ops.StartRun(ctx, sc, model.JobRun{CompanyID: &companyID,
		JobType: job.TypeAlarmEvaluate, Scope: scopeJSON, StartedAt: now, Status: "running"})
	if err != nil {
		return err
	}
	var processed, skipped, failed, notified int32
	for _, rule := range rules {
		ev, err := s.EvaluateRule(ctx, sc, rule, now, false)
		switch {
		case err != nil:
			// 01 §8: one failing rule never aborts the run.
			failed++
			s.ruleFailed(ctx, sc, rule, err)
		default:
			processed++
			notified += int32(ev.NotificationsSent)
			if !firedAny(ev) {
				skipped++
			}
		}
	}
	status := "success"
	if failed > 0 {
		status = "partial"
		if processed == 0 {
			status = "failed"
		}
	}
	if _, err := s.ed.Ops.FinishRun(ctx, sc, run.ID, status, processed, skipped, failed, nil,
		mustJSON(map[string]any{"notified": notified}), s.d.Clock.Now()); err != nil {
		return err
	}
	// R219: one summary per run, not one row per rule.
	return s.appendRunSummary(ctx, sc, status, processed, skipped, failed, notified)
}
```

`EvaluateRule` loads exactly what the rule's type needs and calls the pure evaluators:

- `reactive_limit` → one `Readings.Range(sc, analyzer, NewWindow(now, value, unit), load_profile)`
  per configured group (four at most), into `alarm.ReactiveInput` with its `Windows` map.
- `data_communication` → `AnalyzerRef.LastReadingAt`, which Task 6 already loads onto every
  `Rule.Analyzers` entry, so the evaluation needs no second read and no hypertable access (R216).
- `current_voltage_power` → `Readings.Latest(sc, analyzer, NewWindow(now, 48, hours), load_profile)`.
- `invoice_increase` → `Bills.List(sc, store.BillFilter{AnalyzerID: &id, BillScope: &analyzerScope,
  Page: store.Page{Limit: 2}})`, newest first; then `MarkBillFired` **before** creating the event.

Then, for a real run and each fired verdict: `Describe` → `CreateEvent` with the curated detail →
`alarm.Suppressed` against the newest notified event for the pair
(`ListEvents{AlarmID, AnalyzerID, Notified: &yes, Page: {Limit: 1}}`) → either `ed.Enqueue` a
notify payload and `NotificationsSent++`, or re-write the event's detail with
`notification_suppressed: true`. A dry run stops after `Describe`.

Wire `AlarmDispatch`/`AlarmEvaluate`/`AlarmNotify` in `internal/worker/worker.go` and
`internal/apiwire/wiring.go`, and replace Task 7's `perr.Unavailable` stub with the real
`notify=true` path (`EvaluateRule(..., dryRun=false)`).

`Dispatch` lists companies with at least one enabled alarm and enqueues one
`alarm.evaluate` per company with `Hour: now.Truncate(time.Hour)`, recording a platform run
through `AdminJournalRepository.StartPlatformRun` — the same shape `internal/ingest/dispatch.go`
already uses.

- [ ] **Step 5: Run everything these files touch**

```bash
go test ./internal/job/ ./internal/scheduler/ ./internal/service/alarms/ ./internal/worker/ \
  ./internal/platform/config/ -count=1 -race
EKOKOD_TEST_PG_DSN=... EKOKOD_TEST_REDIS_URL=... TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
go test ./internal/service/alarms/ ./internal/job/ -tags=integration -count=1 -parallel 4
golangci-lint run --concurrency 2 --build-tags=integration ./internal/...
```
Expected: PASS, 0 lint issues.

- [ ] **Step 6: Prove the guards red by mutation**

1. Drop the `Hour` truncation → `TestAlarmEvaluateTaskIDIsOnePerCompanyPerHour` must FAIL.
2. Move `MarkBillFired` to after the notify enqueue →
   `TestEvaluateInvoiceFiresOnceAcrossRepeatedRuns` must FAIL (three events).
3. Replace `alarm.Suppressed` with `false` → `TestEvaluateFrequencySuppressesThenAllows` must FAIL.
4. Let a rule's error `return` out of the loop → `TestEvaluateOneFailingRuleDoesNotAbortTheRun`
   must FAIL.
5. Create an event for a non-firing verdict → the eight-case `wantFire: false` subtests and
   `TestEvaluateRecordsOneJobRunAndOneSummaryMessage` must FAIL.
Restore after each.

- [ ] **Step 7: Commit**

```bash
git add internal/job internal/scheduler internal/service/alarms internal/worker \
  internal/platform/config internal/apiwire docs/rewrite/appendix/env-reference.md
git commit -m "feat(f7): alarm dispatch/evaluate/notify pipeline (R217, R218, R219, R225)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: `internal/service/ops` and the messages / job-run routes

**Files:**
- Create: `internal/service/ops/ops.go`, `internal/service/ops/ops_test.go`
- Create: `internal/api/v1/handlers_ops.go`, `internal/api/v1/dto/ops.go`
- Create: `internal/api/v1/ops_http_integration_test.go`
- Modify: `internal/api/v1/routes.go`, `internal/api/v1/router.go`, `internal/apiwire/wiring.go`

**Interfaces:**
- Consumes: `store.OpsRepository`, `job.Client` (the asynq enqueuer), Task 5's `MessageFilter.Q`.
- Produces:

```go
// Package ops is the read side of 01 §7.13 and §7.16: the Messages screen, the
// job-run history and manual job triggering.
package ops

// Triggerable is the allow-list of R220. Nothing outside it can be enqueued,
// so no caller can name an arbitrary asynq task type.
var Triggerable = []string{
	job.TypeAlarmEvaluate, job.TypeBillingDispatch, job.TypeIntegrationSyncDispatch,
	job.TypeEPIASSyncPrices, job.TypeConsumptionRefresh,
}

type Deps struct {
	Ops     store.OpsRepository
	Enqueue func(ctx context.Context, t *asynq.Task) (string, error)
	Clock   clock.Clock
}

func New(d Deps) (*Service, error)

func (s *Service) Messages(ctx context.Context, sc store.Scope, f store.MessageFilter) ([]model.OperationalMessage, error)
func (s *Service) JobRuns(ctx context.Context, sc store.Scope, f store.JobRunFilter) ([]model.JobRun, error)

// Trigger enqueues one allow-listed job for the caller's company and returns
// the asynq task id, which GET /jobs/{id} (R192) can then watch.
func (s *Service) Trigger(ctx context.Context, sc store.Scope, jobType string) (string, error)
```

DTOs:

```go
type MessagesRequest struct {
	CompanyScopeQuery
	PageRequest
	Kind   *string `query:"kind" json:"-" enum:"alarm,job,system"`
	Status *string `query:"status" json:"-" enum:"success,error,warning,info"`
	Q      string  `query:"q" json:"-" validate:"max=200"`
	From   *time.Time `query:"from" json:"-"`
	To     *time.Time `query:"to" json:"-"`
}

type Message struct {
	ID          int64      `json:"id" required:"true"`
	Kind        string     `json:"kind" required:"true" enum:"alarm,job,system"`
	Category    string     `json:"category" required:"true"`
	Status      string     `json:"status" required:"true" enum:"success,error,warning,info"`
	Message     string     `json:"message" required:"true"`
	Detail      *string    `json:"detail"`
	RelatedType *string    `json:"related_type"`
	RelatedID   *uuid.UUID `json:"related_id"`
	CreatedAt   time.Time  `json:"created_at" required:"true"`
}

type JobRunsRequest struct {
	CompanyScopeQuery
	PageRequest
	JobType *string `query:"job_type" json:"-"`
	Status  *string `query:"status" json:"-" enum:"running,success,partial,failed"`
}

type JobRun struct {
	ID         uuid.UUID       `json:"id" required:"true"`
	JobType    string          `json:"job_type" required:"true"`
	Scope      json.RawMessage `json:"scope" required:"true"`
	StartedAt  time.Time       `json:"started_at" required:"true"`
	FinishedAt *time.Time      `json:"finished_at"`
	Status     string          `json:"status" required:"true" enum:"running,success,partial,failed"`
	Processed  int32           `json:"processed" required:"true"`
	Skipped    int32           `json:"skipped" required:"true"`
	Failed     int32           `json:"failed" required:"true"`
	Error      *string         `json:"error"`
}

type JobTriggerRequest struct {
	// Type is the allow-listed job name (R220); anything else is 422.
	Type string `path:"type" json:"-" validate:"required,oneof=alarm.evaluate billing.dispatch integration.sync_dispatch epias.sync_prices consumption.refresh" enum:"alarm.evaluate,billing.dispatch,integration.sync_dispatch,epias.sync_prices,consumption.refresh"`
	CompanyScopeQuery
}
```

- [ ] **Step 1: Write the failing tests**

`internal/service/ops/ops_test.go`:

```go
func TestTriggerRefusesAnythingOffTheAllowList(t *testing.T) {
	t.Parallel()
	svc, fake := newOpsService(t)
	for _, bad := range []string{"system.noop", "billing.generate", "alarm.notify", "", "../etc"} {
		_, err := svc.Trigger(t.Context(), companyScope, bad)
		require.Equal(t, http.StatusUnprocessableEntity, perr.StatusOf(err), bad)
	}
	require.Empty(t, fake.enqueued, "R220: nothing off the list ever reaches asynq")
}

func TestTriggerEnqueuesEveryAllowListedType(t *testing.T) {
	t.Parallel()
	svc, fake := newOpsService(t)
	for _, good := range ops.Triggerable {
		id, err := svc.Trigger(t.Context(), companyScope, good)
		require.NoError(t, err, good)
		require.NotEmpty(t, id)
	}
	require.Len(t, fake.enqueued, len(ops.Triggerable))
}

func TestTriggerScopesAlarmEvaluateToTheCallersCompany(t *testing.T) {
	t.Parallel()
	svc, fake := newOpsService(t)
	_, err := svc.Trigger(t.Context(), companyScope, job.TypeAlarmEvaluate)
	require.NoError(t, err)
	var p job.AlarmEvaluatePayload
	require.NoError(t, json.Unmarshal(fake.enqueued[0].Payload(), &p))
	require.Equal(t, companyScope.CompanyID, p.CompanyID, "never another tenant's company id")
}

func TestMessagesRejectsAnOverlongQuery(t *testing.T) {
	t.Parallel()
	svc, _ := newOpsService(t)
	_, err := svc.Messages(t.Context(), companyScope, store.MessageFilter{Q: strings.Repeat("a", 201)})
	require.Equal(t, http.StatusUnprocessableEntity, perr.StatusOf(err))
}
```

`internal/api/v1/ops_http_integration_test.go`:

```go
//go:build integration

func TestMessagesFiltersReturnTheExpectedSets(t *testing.T) {
	h := newHarness(t)
	h.SeedMessages(t) // one alarm/warning, one job/success, one system/info
	for _, tc := range []struct{ query string; want int }{
		{"", 3},
		{"?kind=alarm", 1},
		{"?status=success", 1},
		{"?q=tetiklendi", 1},
		{"?kind=job&status=success", 1},
		{"?kind=job&status=error", 0},
	} {
		res := h.Get(t, roleCompanyAdmin, "/api/v1/messages"+tc.query)
		require.Equal(t, http.StatusOK, res.Code, tc.query)
		var page dto.Page[dto.Message]
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &page))
		require.Len(t, page.Items, tc.want, tc.query)
	}
}

func TestMessagesNeverShowAnotherTenantsRows(t *testing.T) {
	h := newHarness(t)
	h.SeedForeignMessages(t)
	res := h.Get(t, roleCompanyAdmin, "/api/v1/messages")
	require.Equal(t, http.StatusOK, res.Code)
	require.NotContains(t, res.Body.String(), "yabancı şirket")
}

func TestJobRunsAreAdminAndCompanyAdminOnly(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusOK, h.Get(t, roleAdmin, "/api/v1/job-runs").Code)
	require.Equal(t, http.StatusOK, h.Get(t, roleCompanyAdmin, "/api/v1/job-runs").Code)
	for _, r := range []string{roleCompanyReadonly, roleBuildingAdmin, roleBuildingReadonly, roleDemo} {
		require.Equal(t, http.StatusForbidden, h.Get(t, r, "/api/v1/job-runs").Code, r)
	}
}

// 09 §F7 acceptance: a manually triggered job appears in job_runs with its
// scope and result.
func TestTriggeredJobAppearsInJobRuns(t *testing.T) {
	h := newHarness(t)
	res := h.Post(t, roleAdmin, "/api/v1/job-runs/alarm.evaluate/trigger", nil)
	require.Equal(t, http.StatusAccepted, res.Code)
	var acc dto.JobAccepted
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &acc))
	require.NotEmpty(t, acc.JobID)

	h.RunEnqueuedTasks(t) // the harness's inline worker
	runs := h.Get(t, roleAdmin, "/api/v1/job-runs?job_type=alarm.evaluate")
	require.Equal(t, http.StatusOK, runs.Code)
	var page dto.Page[dto.JobRun]
	require.NoError(t, json.Unmarshal(runs.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	require.Contains(t, string(page.Items[0].Scope), h.CompanyID.String())
	require.Equal(t, "success", page.Items[0].Status)
}

func TestTriggerIsAdminOnlyAndRefusesUnknownTypes(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusForbidden,
		h.Post(t, roleCompanyAdmin, "/api/v1/job-runs/alarm.evaluate/trigger", nil).Code)
	require.Equal(t, http.StatusUnprocessableEntity,
		h.Post(t, roleAdmin, "/api/v1/job-runs/system.noop/trigger", nil).Code)
}
```

- [ ] **Step 2: Run them and watch them fail**

```bash
go test ./internal/service/ops/ -count=1
EKOKOD_TEST_PG_DSN=... EKOKOD_TEST_REDIS_URL=... TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
go test ./internal/api/v1/ -tags=integration -count=1 -parallel 4 -run 'TestMessages|TestJobRuns|TestTrigger'
```
Expected: FAIL — the package and the three routes do not exist.

- [ ] **Step 3: Write the service, DTOs, routes and handlers**

`Trigger` validates against `Triggerable` with `slices.Contains` **before** building a task, then
builds the type's own task with the caller's company id (for `alarm.evaluate`,
`AlarmEvaluatePayload{CompanyID: sc.CompanyID, Hour: s.d.Clock.Now().Truncate(time.Hour)}`; the
four platform ticks take no payload) and returns `client.Enqueue`'s task id.

The three route rows use `auth.AllRoles` for `/messages`, `auth.Roles(roleA, roleCA)` for
`/job-runs` and `auth.Roles(roleA)` for the trigger, with `Entity: "job_run"`,
`PlatformAudit: false` and `Status: http.StatusAccepted` on the trigger.

- [ ] **Step 4: Run the gates**

```bash
go test ./internal/service/ops/ ./internal/api/v1/... -count=1 -race
EKOKOD_TEST_PG_DSN=... EKOKOD_TEST_REDIS_URL=... TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
go test ./internal/api/v1/ -tags=integration -count=1 -parallel 4
make openapi && cd web && pnpm check:api && pnpm typecheck
```

- [ ] **Step 5: Prove the guards red by mutation**

1. Replace the `Triggerable` check with "any non-empty string" →
   `TestTriggerRefusesAnythingOffTheAllowList` must FAIL.
2. Build the alarm payload from a query parameter instead of `sc.CompanyID` →
   `TestTriggerScopesAlarmEvaluateToTheCallersCompany` must FAIL.
3. Widen `/job-runs` to `auth.AllRoles` → `TestJobRunsAreAdminAndCompanyAdminOnly` and the
   authorisation matrix must FAIL.
Restore after each.

- [ ] **Step 6: Commit**

```bash
git add internal/service/ops internal/api internal/apiwire web/src/lib/api/schema.d.ts
git commit -m "feat(f7): messages, job-run history and allow-listed triggering (R220, R226, R227)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: Web — the Alarms screen

**Files:**
- Create: `web/src/app/ekorm/alarms/page.tsx`
- Create: `web/src/features/alarms/alarms-page.tsx` + `.test.tsx`
- Create: `web/src/features/alarms/alarm-table.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/alarms/alarm-dialog.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/alarms/alarm-draft.ts` + `.test.ts`
- Create: `web/src/features/alarms/events-dialog.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/alarms/evaluate-dialog.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/alarms/_fixture.ts` (data-free stories need it; R199)
- Create: `web/messages/tr/alarms.json`, `web/messages/en/alarms.json`
- Modify: `web/messages/index.ts`

**Interfaces:**
- Consumes: the generated client (`$api`, `useApiMutation`), `useScopeParams`, `useSession().can`,
  `PageHeader`, `Table`/`TableContainer`, `Dialog`, `Skeleton`, `RadioGroup`, `ScopePicker`
  patterns from F6b; the DTOs of Tasks 7 and 10.
- Produces:

```ts
// alarm-draft.ts — the dialog's local state and its mapping to the request.
export type AlarmKind = 'reactive_limit' | 'data_communication' | 'current_voltage_power' | 'invoice_increase';

export type AlarmDraft = {
  id?: string;
  name: string;
  type: AlarmKind;
  isEnabled: boolean;
  analyzerIds: string[];
  emailTargets: string;   // comma-separated, as §7.12 specifies
  smsTargets: string;     // comma-separated; stored, never sent (R211)
  channels: { email: boolean; sms: boolean };
  frequencyValue: string;
  frequencyUnit: 'hours' | 'days';
  settings: Record<string, string>;  // raw field strings, keyed by the DTO's names
};

export function emptyDraft(): AlarmDraft;
export function draftFrom(alarm: Alarm): AlarmDraft;
/** Clears the fields of every other type (R229), so a type switch cannot smuggle them. */
export function clearForType(draft: AlarmDraft, type: AlarmKind): AlarmDraft;
/** The 422 the server would return, checked client-side first (R230). */
export function draftErrors(draft: AlarmDraft): Record<string, string>;
export function toAlarmRequest(draft: AlarmDraft): AlarmFields;
```

- [ ] **Step 1: Write the failing unit tests for the draft**

`web/src/features/alarms/alarm-draft.test.ts`:

```ts
import { describe, expect, it } from 'vitest';

import { clearForType, draftErrors, emptyDraft, toAlarmRequest } from './alarm-draft';

describe('alarm draft', () => {
  it('clears the other types’ settings when the type switches (R229)', () => {
    const reactive = { ...emptyDraft(), type: 'reactive_limit' as const,
      settings: { inductive_ratio_threshold: '20', inductive_period_value: '24' } };
    const power = clearForType(reactive, 'current_voltage_power');
    expect(power.settings.inductive_ratio_threshold).toBeUndefined();
    expect(power.type).toBe('current_voltage_power');
  });

  it('never offers voltage fields (R212)', () => {
    const power = { ...emptyDraft(), type: 'current_voltage_power' as const,
      settings: { power_max: '100', voltage_max: '400' } };
    expect(toAlarmRequest(power).settings).not.toHaveProperty('voltage_max');
  });

  it('requires an analyzer and one limit (R230)', () => {
    expect(draftErrors(emptyDraft())).toHaveProperty('analyzerIds');
    const withAnalyzer = { ...emptyDraft(), analyzerIds: ['a-1'], name: 'Test' };
    expect(draftErrors(withAnalyzer)).toHaveProperty('settings');
    const complete = { ...withAnalyzer, type: 'data_communication' as const,
      settings: { communication_threshold_hours: '6' } };
    expect(draftErrors(complete)).toEqual({});
  });

  it('splits comma-separated recipients and drops blanks', () => {
    const draft = { ...emptyDraft(), analyzerIds: ['a-1'], name: 'T',
      type: 'data_communication' as const, settings: { communication_threshold_hours: '6' },
      channels: { email: true, sms: true },
      emailTargets: 'a@b.com, , c@d.com ', smsTargets: '+905551112233,' };
    expect(toAlarmRequest(draft).channels).toEqual([
      { channel: 'email', target: 'a@b.com' },
      { channel: 'email', target: 'c@d.com' },
      { channel: 'sms', target: '+905551112233' },
    ]);
  });

  it('omits a channel’s targets when the channel is unticked', () => {
    const draft = { ...emptyDraft(), analyzerIds: ['a-1'], name: 'T',
      type: 'data_communication' as const, settings: { communication_threshold_hours: '6' },
      channels: { email: true, sms: false }, emailTargets: 'a@b.com', smsTargets: '+905551112233' };
    expect(toAlarmRequest(draft).channels).toEqual([{ channel: 'email', target: 'a@b.com' }]);
  });

  it('rejects a frequency value without its unit', () => {
    const draft = { ...emptyDraft(), analyzerIds: ['a-1'], name: 'T',
      type: 'data_communication' as const, settings: { communication_threshold_hours: '6' },
      frequencyValue: '6', frequencyUnit: '' as never };
    expect(draftErrors(draft)).toHaveProperty('frequencyUnit');
  });
});
```

- [ ] **Step 2: Run them and watch them fail**

Run: `cd web && pnpm vitest run src/features/alarms/alarm-draft.test.ts`
Expected: FAIL — the module does not exist.

- [ ] **Step 3: Write `alarm-draft.ts`**

`SETTINGS_BY_TYPE` is the single source of which fields each type owns. `clearForType` keeps only
that type's keys, `toAlarmRequest` emits only them (so `voltage_max` can never be sent), and
`draftErrors` mirrors Task 2's `Validate` branch for branch:

```ts
/** Which settings each type owns — the mirror of the Go switch in internal/domain/alarm.
 *  voltage_max / voltage_min appear NOWHERE: no provider reports voltage (R212). */
export const SETTINGS_BY_TYPE: Record<AlarmKind, readonly string[]> = {
  reactive_limit: [
    'inductive_ratio_threshold', 'inductive_period_value', 'inductive_period_unit',
    'capacitive_ratio_threshold', 'capacitive_period_value', 'capacitive_period_unit',
    'active_consumption_max', 'active_consumption_max_period_value', 'active_consumption_max_period_unit',
    'active_consumption_min', 'active_consumption_min_period_value', 'active_consumption_min_period_unit',
  ],
  data_communication: ['communication_threshold_hours'],
  current_voltage_power: ['power_max', 'power_min'],
  invoice_increase: ['invoice_threshold_pct'],
};

/** The four reactive groups, each complete only with its threshold AND its period (R230). */
const REACTIVE_GROUPS = [
  ['inductive_ratio_threshold', 'inductive_period_value', 'inductive_period_unit'],
  ['capacitive_ratio_threshold', 'capacitive_period_value', 'capacitive_period_unit'],
  ['active_consumption_max', 'active_consumption_max_period_value', 'active_consumption_max_period_unit'],
  ['active_consumption_min', 'active_consumption_min_period_value', 'active_consumption_min_period_unit'],
] as const;
```

- [ ] **Step 4: Write the failing container test**

`web/src/features/alarms/alarms-page.test.tsx`, on the `mockApi` route table:

```tsx
const ROUTES = {
  'GET /api/v1/alarms': { items: demoAlarms, next_cursor: null },
  'GET /api/v1/analyzers': { items: demoAnalyzers, next_cursor: null },
  'POST /api/v1/alarms': Response.json({ ...demoAlarms[0], id: 'al-9' }, { status: 201 }),
  'PATCH /api/v1/alarms/al-1': Response.json(demoAlarms[0]),
  'DELETE /api/v1/alarms/al-1': new Response(null, { status: 204 }),
  'GET /api/v1/alarms/al-1/events': { items: demoEvents, next_cursor: null },
  'POST /api/v1/alarms/al-1/evaluate': Response.json(demoEvaluation),
};

it('lists rules with their type, analyzers and state', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<AlarmsPage />, { me: me('company_admin') });
  expect(await screen.findByText('Endüktif izleme')).toBeVisible();
  expect(screen.getByText('Reaktif Limit Algılama Alarmı')).toBeVisible();
  expect(screen.getByText('A-1')).toBeVisible();
});

it('filters by active and passive (§7.12)', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<AlarmsPage />, { me: me('company_admin') });
  await screen.findByText('Endüktif izleme');
  await userEvent.click(screen.getByRole('radio', { name: 'Pasif' }));
  await waitFor(() => expect(queryOf(api.calls, 'GET', '/api/v1/alarms', 1).get('is_enabled')).toBe('false'));
});

it('hides every write control from a read-only role (R159)', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<AlarmsPage />, { me: me('company_readonly_admin') });
  await screen.findByText('Endüktif izleme');
  expect(screen.queryByRole('button', { name: 'Yeni Alarm Ekle' })).toBeNull();
  expect(screen.queryByRole('button', { name: /Düzenle/ })).toBeNull();
  expect(screen.queryByRole('button', { name: /Şimdi değerlendir/ })).toBeNull();
});

it('shows the dry-run verdicts without claiming anything was sent (R223)', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<AlarmsPage />, { me: me('company_admin') });
  await screen.findByText('Endüktif izleme');
  await userEvent.click(screen.getAllByRole('button', { name: /Şimdi değerlendir/ })[0]);
  expect(await screen.findByText(/Deneme değerlendirmesi/)).toBeVisible();
  expect(screen.getByText(/Bildirim gönderilmedi/)).toBeVisible();
});

it('says SMS is not active yet (R211)', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<AlarmsPage />, { me: me('company_admin') });
  await userEvent.click(await screen.findByRole('button', { name: 'Yeni Alarm Ekle' }));
  await userEvent.click(screen.getByRole('checkbox', { name: 'SMS' }));
  expect(screen.getByText('SMS bildirimi henüz etkin değil')).toBeVisible();
});

it('shows the alarm log with delivery state', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<AlarmsPage />, { me: me('company_admin') });
  await screen.findByText('Endüktif izleme');
  await userEvent.click(screen.getAllByRole('button', { name: /Detaylar/ })[0]);
  expect(await screen.findByText(/Endüktif oran/)).toBeVisible();
  expect(screen.getByText(/Gönderilemedi/)).toBeVisible();
});

it('surfaces a failed list request instead of an empty screen (R-F6b/QueryErrorToaster)', async () => {
  api = mockApi({ ...ROUTES, 'GET /api/v1/alarms': Response.json(
    { error: { code: 'internal', message: 'Bir hata oluştu' } }, { status: 500 }) });
  renderWithProviders(<AlarmsPage />, { me: me('company_admin') });
  expect(await screen.findByText('Bir hata oluştu')).toBeVisible();
});
```

- [ ] **Step 5: Run it and watch it fail, then build the screen**

Run: `pnpm vitest run src/features/alarms` → FAIL (no `AlarmsPage`).

Build `alarms-page.tsx` as the container (list query + the three dialogs + four mutations),
`alarm-table.tsx` as the pure list view (name, type label, analyzer chips, an enabled switch,
and the actions of §7.12), `alarm-dialog.tsx` as the one type-switching form,
`events-dialog.tsx` and `evaluate-dialog.tsx` as read-only dialogs. Stories are data-free and own
their opener (the F6b house pattern); `_fixture.ts` holds `demoAlarms`, `demoEvents`,
`demoEvaluation`, `demoAnalyzers`.

§7.12 lists five actions — edit, delete, **view details**, **view logs** and the enable toggle.
Legacy had one "Detaylar" button that opened the log modal, so details and logs are the **same**
dialog: `events-dialog.tsx` shows the rule's settings as a read-only summary above its event
history, and the table exposes it once, labelled `alarms.details`. "Evaluate now" is F7's own
addition (R223, 05 §10's `POST /alarms/{id}/evaluate`) and is gated on `alarms.evaluate`, so a BA
sees edit and delete but not it.

The i18n namespace `alarms` reuses legacy's own Turkish copy where it exists
(`docs/rewrite/appendix/i18n-tr.json`'s `alarms.*`), camelCased, plus the new keys: `smsInactive`
("SMS bildirimi henüz etkin değil"), `evaluateNow`, `dryRunTitle`, `dryRunNoNotification`,
`noVerdict`, `types.power` ("Güç Alarmı", **not** legacy's "Akım - Voltaj - Güç Alarmı" — R212).

- [ ] **Step 6: Run the web gates**

```bash
cd web && pnpm lint && pnpm typecheck && pnpm check:api && pnpm check:i18n-parity \
  && pnpm check:contrast && pnpm test --maxWorkers=2
free -m && pnpm storybook:build && STORIES='Features/Alarms' pnpm test:a11y --workers=1
```
Expected: all green; the a11y chunk covers the new stories.

- [ ] **Step 7: Prove the guards red by mutation**

1. Let `toAlarmRequest` spread the whole `settings` object → the voltage test must FAIL.
2. Drop the `can('alarms.edit')` guard around the toolbar → the read-only-role test must FAIL.
3. Remove the `smsInactive` caption → its test must FAIL.
4. Remove `clearForType` from the type-switch handler → the draft test must FAIL.
Restore after each.

- [ ] **Step 8: Commit**

```bash
git add web/src/app/ekorm/alarms web/src/features/alarms web/messages
git commit -m "feat(f7): the Alarms screen with the four rule types (R211, R212, R229, R230)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 12: Web — the Messages screen with the job-history tab

**Files:**
- Create: `web/src/app/ekorm/messages/page.tsx`
- Create: `web/src/features/messages/messages-page.tsx` + `.test.tsx`
- Create: `web/src/features/messages/message-table.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/messages/job-runs-table.tsx` + `.test.tsx` + `.stories.tsx`
- Create: `web/src/features/messages/_fixture.ts`
- Create: `web/messages/tr/messages.json`, `web/messages/en/messages.json`
- Modify: `web/messages/index.ts`

**Interfaces:**
- Consumes: `dto.Message`, `dto.JobRun`, `dto.JobAccepted`; `useJob` (R192) to watch a triggered
  job; `RefreshActions` as the UI precedent for a trigger button.
- Produces: `MessagesPage`, `MessageTable`, `JobRunsTable` (all exported from their own files).

- [ ] **Step 1: Write the failing container test**

`web/src/features/messages/messages-page.test.tsx`:

```tsx
const ROUTES = {
  'GET /api/v1/messages': { items: demoMessages, next_cursor: null },
  'GET /api/v1/job-runs': { items: demoRuns, next_cursor: null },
  'POST /api/v1/job-runs/alarm.evaluate/trigger': Response.json({ job_id: 'task-1' }, { status: 202 }),
  'GET /api/v1/jobs/task-1': { id: 'task-1', type: 'alarm.evaluate', status: 'succeeded' },
};

it('shows the §7.13 table with type and status badges', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<MessagesPage />, { me: me('company_admin') });
  expect(await screen.findByText('Alarm tetiklendi')).toBeVisible();
  expect(screen.getByText('Alarmlar')).toBeVisible();
  expect(screen.getByText('Uyarı')).toBeVisible();
});

it('sends the search term and both filters to the API (R226)', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<MessagesPage />, { me: me('company_admin') });
  await screen.findByText('Alarm tetiklendi');

  await userEvent.type(screen.getByRole('searchbox', { name: 'Arama' }), 'tetiklendi');
  await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Tip' }), 'alarm');
  await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Durum' }), 'warning');

  await waitFor(() => {
    const q = queryOf(api.calls, 'GET', '/api/v1/messages', api.calls.filter(
      (c) => new URL(c.url).pathname === '/api/v1/messages').length - 1);
    expect(q.get('q')).toBe('tetiklendi');
    expect(q.get('kind')).toBe('alarm');
    expect(q.get('status')).toBe('warning');
  });
});

it('hides the job-history tab from a role without jobs.runs.read', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<MessagesPage />, { me: me('building_admin') });
  await screen.findByText('Alarm tetiklendi');
  expect(screen.queryByRole('tab', { name: 'İş Geçmişi' })).toBeNull();
  // …and never requests it.
  expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/job-runs')).toBe(false);
});

it('shows the job-history tab to a company admin without a trigger button', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<MessagesPage />, { me: me('company_admin') });
  await userEvent.click(await screen.findByRole('tab', { name: 'İş Geçmişi' }));
  expect(await screen.findByText('alarm.evaluate')).toBeVisible();
  expect(screen.getByText('Başarılı')).toBeVisible();
  expect(screen.queryByRole('button', { name: /Şimdi çalıştır/ })).toBeNull();
});

it('lets an admin trigger a job and watches it to completion (R220, R192)', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<MessagesPage />, { me: me('admin') });
  await userEvent.click(await screen.findByRole('tab', { name: 'İş Geçmişi' }));
  await userEvent.click(screen.getByRole('button', { name: 'alarm.evaluate — Şimdi çalıştır' }));
  expect(await screen.findByText(/Tamamlandı/)).toBeVisible();
});

it('shows the counts a run reports', async () => {
  api = mockApi(ROUTES);
  renderWithProviders(<MessagesPage />, { me: me('admin') });
  await userEvent.click(await screen.findByRole('tab', { name: 'İş Geçmişi' }));
  expect(await screen.findByText('12')).toBeVisible();  // processed
  expect(screen.getByText('3')).toBeVisible();          // skipped
  expect(screen.getByText('1')).toBeVisible();          // failed
});

it('shows the empty state when nothing matches the filters', async () => {
  api = mockApi({ ...ROUTES, 'GET /api/v1/messages': { items: [], next_cursor: null } });
  renderWithProviders(<MessagesPage />, { me: me('company_admin') });
  expect(await screen.findByText('Filtre kriterlerine uygun mesaj bulunamadı')).toBeVisible();
});
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd web && pnpm vitest run src/features/messages`
Expected: FAIL — no `MessagesPage`.

- [ ] **Step 3: Build the screen**

`MessageTable`'s columns are exactly §7.13's: type badge (with the icon legacy used), status badge,
message (with its category beneath, as legacy showed it), detail, **related entity** — rendered
from `related_type` + `related_id`, linking to the alarm when `related_type == "alarm"` and shown
as an em dash when both are null — and the timestamp split into date and time. `JobRunsTable`'s
columns are job type, scope, started, finished, status badge and the three counts.

Two tabs; the second is rendered **and queried** only when `can('jobs.runs.read')`, so a BA never
issues the request the test forbids. The search box is debounced (300 ms) before it reaches the
query key. The trigger button is behind `can('jobs.trigger')`, uses `useApiMutation` and then
`useJob(jobId)` for the watch, exactly as `RefreshActions` does. Manual refresh re-fetches both
queries. The five job types come from the generated enum on `JobTriggerRequest.Type`, so the
allow-list cannot drift from the server's.

- [ ] **Step 4: Run the web gates**

```bash
cd web && pnpm lint && pnpm typecheck && pnpm check:api && pnpm check:i18n-parity \
  && pnpm check:contrast && pnpm test --maxWorkers=2
free -m && pnpm storybook:build && STORIES='Features/Messages' pnpm test:a11y --workers=1
```

- [ ] **Step 5: Prove the guards red by mutation**

1. Render the job-runs query unconditionally → the building-admin test must FAIL on the request
   assertion.
2. Drop the `can('jobs.trigger')` guard → the company-admin test must FAIL.
3. Stop forwarding `q` → the filter test must FAIL.
Restore after each.

- [ ] **Step 6: Commit**

```bash
git add web/src/app/ekorm/messages web/src/features/messages web/messages
git commit -m "feat(f7): Messages screen with the job-history tab (§7.13, R220, R226)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 13: Seed, e2e and the F7 acceptance run

**Files:**
- Modify: `internal/seed/fixtures.go`, `internal/seed/readings.go` (or wherever `E2EData` lives)
- Modify: `web/tests/e2e/responsive.spec.ts` (`SCREENS`)
- Create: `web/tests/e2e/alarms.spec.ts`, `web/tests/e2e/messages.spec.ts`

**Interfaces:**
- Consumes: `ekokod seed e2e` (R194), the Playwright `login`/`USERS` fixtures.
- Produces seeded state every e2e spec can rely on: one rule per type (four rules), one fired and
  delivered event, one fired and failed event, three operational messages (one per kind) and two
  job runs (one `success`, one `partial`).

- [ ] **Step 1: Extend the e2e seed**

In `seed.E2EData`, after the readings and the building bill, add the four rules attached to `A1`,
their channels (`email` plus one `sms` target so the R211 caption has something to describe), the
two events, the three messages and the two job runs. Keep it inside `E2EData` — `E2EFixtures` is
read by four Go tests written against empty fixtures (an F6b deviation worth not repeating).

- [ ] **Step 2: Write the failing e2e specs**

`web/tests/e2e/alarms.spec.ts`:

```ts
test('lists the seeded rules and filters by state', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/alarms');
  await expect(page.getByRole('heading', { level: 1, name: 'Alarmlar' })).toBeVisible();
  await expect(page.getByText('Veri İletişim Alarmı')).toBeVisible();
  await expect(page.getByText('Güç Alarmı')).toBeVisible();
  await expect(page.getByText('Akım - Voltaj - Güç')).toHaveCount(0);  // R212
});

test('creates a data-communication rule end to end', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/alarms');
  await page.getByRole('button', { name: 'Yeni Alarm Ekle' }).click();
  await page.getByLabel('Ad').fill('E2E iletişim');
  await page.getByLabel('Tip').selectOption('data_communication');
  await page.getByRole('checkbox', { name: /A-1/ }).check();
  await page.getByLabel('İletişim Kesinti Eşiği (saat)').fill('6');
  await page.getByRole('checkbox', { name: 'E-posta' }).check();
  await page.getByLabel(/E-posta Alıcıları/).fill('e2e@ornek.com.tr');
  await page.getByRole('button', { name: 'Kaydet' }).click();
  await expect(page.getByText('E2E iletişim')).toBeVisible();
});

test('refuses a rule with no limit', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/alarms');
  await page.getByRole('button', { name: 'Yeni Alarm Ekle' }).click();
  await page.getByLabel('Ad').fill('Eksik');
  await page.getByRole('checkbox', { name: /A-1/ }).check();
  await page.getByRole('button', { name: 'Kaydet' }).click();
  await expect(page.getByText('En az bir alarm limiti girilmelidir')).toBeVisible();
});

test('shows a dry-run verdict and the alarm log', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/alarms');
  const row = page.getByRole('row', { name: /Veri İletişim/ });
  await row.getByRole('button', { name: /Şimdi değerlendir/ }).click();
  await expect(page.getByText(/Deneme değerlendirmesi/)).toBeVisible();
  await page.getByRole('button', { name: 'Kapat' }).click();
  await row.getByRole('button', { name: /Detaylar/ }).click();
  await expect(page.getByText(/Gönderilemedi/)).toBeVisible();
});

test('a read-only role sees no write control', async ({ page }) => {
  await login(page, USERS.companyReadonly.email);
  await page.goto('/ekorm/alarms');
  await expect(page.getByText('Veri İletişim Alarmı')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Yeni Alarm Ekle' })).toHaveCount(0);
});
```

`web/tests/e2e/messages.spec.ts`:

```ts
test('filters the seeded messages by type, status and search', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/messages');
  await expect(page.getByRole('heading', { level: 1, name: 'Tüm Mesajlar' })).toBeVisible();
  await expect(page.getByRole('row')).toHaveCount(4); // header + three kinds

  await page.getByLabel('Tip').selectOption('alarm');
  await expect(page.getByRole('row')).toHaveCount(2);

  await page.getByLabel('Tip').selectOption('all');
  await page.getByLabel('Arama').fill('tetiklendi');
  await expect(page.getByRole('row')).toHaveCount(2);
});

test('an admin triggers a job and sees it in the history', async ({ page }) => {
  await login(page, USERS.admin.email);
  await page.goto('/ekorm/messages');
  await page.getByRole('tab', { name: 'İş Geçmişi' }).click();
  await expect(page.getByText('alarm.evaluate')).toBeVisible();
  await page.getByRole('button', { name: /alarm\.evaluate — Şimdi çalıştır/ }).click();
  await expect(page.getByText(/Kuyruğa alındı|Çalışıyor|Tamamlandı/)).toBeVisible();
});

test('a building admin sees messages but no job history', async ({ page }) => {
  await login(page, USERS.buildingAdmin.email);
  await page.goto('/ekorm/messages');
  await expect(page.getByRole('heading', { level: 1, name: 'Tüm Mesajlar' })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'İş Geçmişi' })).toHaveCount(0);
});
```

Add both screens to `responsive.spec.ts`:

```ts
  ['alarms', '/ekorm/alarms'],
  ['messages', '/ekorm/messages'],
```

- [ ] **Step 3: Run the e2e suite in the foreground**

```bash
free -m && ps -eo rss,comm | grep java     # if memory is tight, keep --workers=1 (it already is)
cd web && pnpm build && PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1
```
Expected: the previous 39 tests plus 8 new ones, and `responsive.spec.ts`'s four widths now walk
seven screens. **When a UI test fails, ask whether it is the test or the product** — three F6b
failures were the matchers, not the app (`Cuma` also matches `Cumartesi`; a strict-mode row match;
a click before the grid rendered).

- [ ] **Step 4: Run the whole F7 acceptance set**

```bash
# Go, package by package, with the race detector
go test ./internal/domain/alarm/ ./internal/service/alarms/ ./internal/service/ops/ \
  ./internal/mail/ ./internal/job/ ./internal/scheduler/ ./internal/worker/ ./internal/auth/ \
  ./internal/api/v1/... ./internal/store/... ./internal/seed/ ./internal/arch/ -count=1 -race

# Integration, on the shared containers
EKOKOD_TEST_PG_DSN='postgres://ekokod:ekokod@localhost:55432/postgres?sslmode=disable' \
EKOKOD_TEST_REDIS_URL=redis://localhost:56379/0 TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
go test ./internal/api/v1/ ./internal/service/alarms/ ./internal/store/postgres/ ./internal/seed/ \
  -tags=integration -count=1 -parallel 4

# 09 §F7's own verification block, run verbatim. The alarm integration tests live in
# internal/service/alarms (the job package holds only the task plumbing), so the
# spec's ./internal/job/... line is run as written AND followed by the package that
# actually carries the acceptance tests — a green vacuous run is not evidence.
go test ./internal/domain/alarm/... -v
go test ./internal/job/... -run TestAlarm -tags=integration -v
go test ./internal/service/alarms/... -run TestEvaluate -tags=integration -v
go test ./internal/mail/... -v
cd web && pnpm exec playwright test tests/e2e/alarms.spec.ts tests/e2e/messages.spec.ts --workers=1

golangci-lint run --concurrency 2 --build-tags=integration ./internal/...
make check-generate && make openapi   # both must leave no diff

cd web && pnpm lint && pnpm typecheck && pnpm check:api && pnpm check:i18n-parity \
  && pnpm check:contrast && pnpm test --maxWorkers=2 && pnpm audit --audit-level=high && pnpm build
pnpm storybook:build
STORIES='Features/Alarms,Features/Messages' pnpm test:a11y --workers=1
PW_PROJECT=e2e pnpm exec playwright test --project=e2e --workers=1
```

Then confirm nothing is left running: no `next dev`/`next start`, no `storybook dev`, no
`vitest --watch`, no static server, no `bin/ekokod-e2e`.

- [ ] **Step 5: Commit**

```bash
git add internal/seed web/tests
git commit -m "test(f7): e2e alarm and message specs, seeded fixtures, responsive coverage

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 6: Whole-phase self-review (once, at the end)**

Read the full diff `git diff phase/f6b-screens...HEAD` in this order, and fix what you find with a
guard that you then prove red:

1. **Authorisation and tenancy first.** Every new route's role set against Task 1's table; every
   new repository call's Scope; R213's intersection on *every* path (list, get, events, evaluate,
   notify); the trigger allow-list; that no handler reads `company_id` from the query itself.
2. **Then the job contract** (01 §8): idempotent (R225), resumable, isolated failure, observable,
   manually triggerable — one assertion each, already written in Task 9; confirm they still hold
   at the tip.
3. **Then 07 §11 accessibility and the design rules**: run the real artefact at 375/768/1024/1440
   (four of F6b's six phase-end findings were only visible there), the contrast gate, the
   touch-target and keyboard sweeps, and check that no dialog invents its own pattern.
4. **Then honesty**: nothing claims a notification it did not send; the SMS caption is present
   wherever SMS can be ticked; `no_verdict` is shown as "could not be decided", never as
   "compliant".

Record every finding and its fix in the inline ledger at
`/mnt/c/Users/meren/Desktop/Work/ekokod-rewrite/.superpowers/sdd/2026-09-18-f7/inline-ledger.md`.

---

## Acceptance map — 09 §F7

| Acceptance criterion | Test |
|---|---|
| Each alarm type fires on a fixture that should trigger it and stays silent on one that should not — one test per type, per direction | `TestEvaluateFiresPerTypeAndStaysSilent` (8 subtests, Task 9) plus the pure-domain pairs in Tasks 3–4 |
| Notification frequency suppresses a second notification inside the window and allows one after it | `TestEvaluateFrequencySuppressesThenAllows` (Task 9), `TestSuppressedInsideWindowAllowedAfterIt` (Task 4) |
| The invoice alarm fires once per invoice and never twice, across repeated evaluations | `TestEvaluateInvoiceFiresOnceAcrossRepeatedRuns` (Task 9) |
| The data-communication alarm fires when the last reading is older than the threshold | `TestCommsFiresWhenLastReadingIsOlderThanThreshold`, `TestCommsExactlyAtThresholdStaysSilent` (Task 4); `comms fires` / `comms silent` (Task 9) |
| A failed e-mail send is recorded on the alarm event and surfaced in Messages; it does not fail the job | `TestNotifyFailedSendIsRecordedAndDoesNotFailTheJob` (Task 8) |
| Messages filters return the expected sets on a seeded fixture | `TestMessagesFiltersReturnTheExpectedSets` (Task 10), `messages.spec.ts` first test (Task 13) |
| A manually triggered job appears in `job_runs` with its scope and result | `TestTriggeredJobAppearsInJobRuns` (Task 10) |
| Alarm rule CRUD, event history and dry-run evaluation | `TestAlarmCRUDRoundTrip`, `TestAlarmDryRunWritesNothing` (Task 7); `alarms.spec.ts` (Task 13) |
| E-mail delivery through the company's SMTP settings, templates in both locales | `TestAlarmFiredRendersBothLocales` (Task 8), `TestNotifySendsToEveryEmailTargetOnce` (Task 8) |
| SMS: implemented against a real gateway **or removed from the UI**, not offered unimplemented | **Deliberate divergence, D-1/D-5, R211:** offered *with* a "not active yet" caption and a recorded non-delivery. `TestNotifyRecordsSMSAsNotSent` (Task 8), the `smsInactive` container test (Task 11) |
| `job_runs` history surfaced to admins, with manual job triggering | `TestJobRunsAreAdminAndCompanyAdminOnly`, `TestTriggerIsAdminOnlyAndRefusesUnknownTypes` (Task 10); the four job-history container tests (Task 12) |

---

## Open questions for the product owner (defaults ship)

- **Q-C1** `alarms.edit` includes **building_admin** because 05 §10 lists `A CA BA`, so a building
  admin can create a company-level rule over their own meters. Should rule editing be company-admin
  and above instead?
- **Q-C2** R213 makes a rule visible to a building-scoped role when **one** of its analyzers is in
  scope — so a BA can see, and edit, a rule that also points at meters they cannot see (the
  analyzer chips for those are hidden). Legacy behaved the same way. Should such a rule instead be
  read-only for them?
- **Q-C3** R219 writes no `alarm_events` row for an evaluation that does not fire; §7.12 calls the
  alarm log "a history of evaluations and firings". Is the run summary in Messages enough, or do
  you want a row per evaluation (millions per month)?
- **Q-C4** R214 reads power from `MaxDemandKw` — the **interval maximum demand**, which is the
  closest thing the data model has to legacy's `t_p_kW`. Confirm that a power alarm on interval max
  demand is what you mean by "Güç Maks/Min".
- **Q-C5** R221 drops legacy's HTML alarm mail for plain text in both locales. Do you want the
  branded HTML body back in a later phase (it needs a multipart sender)?
- **Q-C6** The notification frequency window is per **(rule, analyzer)** pair. Legacy never
  enforced it at all, so there is no precedent: should one noisy meter be able to suppress
  notifications for the rule's *other* meters (per-rule) instead?
- **Q-C7** R226's search is a substring over `message` and `detail`. Is a date range on the
  Messages screen wanted too (the API already takes `from`/`to`; the screen does not offer it)?

Carried forward unchanged: every open question in F6b's handoff (Q-B1…Q-B4, Q-A1…Q-A5, F5's
Q1–Q7, F4's real-invoice comparison and dated-parameter items, F1–F3's).

---

## Verification at the F7 tip — fill in as the phase lands

- Go `-race`, package by package: _(list)_
- Integration (`-tags=integration`): _(list)_
- `golangci-lint run --concurrency 2 --build-tags=integration ./internal/...` → _(issues)_
- `make check-generate`, `make openapi` → _(diff)_
- Web: `pnpm lint`, `pnpm typecheck`, `pnpm check:api`, `pnpm check:i18n-parity`,
  `pnpm check:contrast`, `pnpm test --maxWorkers=2`, `pnpm audit --audit-level=high`, `pnpm build`
  → _(results)_
- a11y chunks → _(counts)_
- e2e → _(passed)_
- Nothing left running: _(confirm)_
