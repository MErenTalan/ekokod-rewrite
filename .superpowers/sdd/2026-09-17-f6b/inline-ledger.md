# F6b inline ledger (git-ignored)

## Pre-task: test-infrastructure fix (b34faba)
`TestConsumptionSubjectsAndScope` failed deterministically on the F6a tip too (29750 vs 29760).
Diagnosis by probe: readings/hourly/daily complete for both analyzers; A1's August monthly bucket was
materialised (last−first inside the month = 743 h) while the second analyzer's was R94-composed
(closing-to-closing = 744 h). Cause: 00005's continuous-aggregate refresh policies run on the
container scheduler inside the short-lived clones. Fix: `dropRefreshPolicies` in the template and in
every clone + `TestIsolatedDBHasNoRefreshPolicies`. Product semantics untouched (R94 keeps its
documented boundary-step divergence — carried to the handoff as a PO question).

## Task 1 — R190 company_id, R191 permissions (e62983d)
Tests first: `TestOpenAPIDeclaresCompanyIDOnScopedRoutes` (red: `auth.logout must declare exactly one
company_id parameter`), `TestPermissionsMatrix`/`TestPermissionsFixtureMatchesTable` (red on both new
permissions), web `permissions.test.ts`.
Gate: `go test ./internal/api/v1/... ./internal/auth/ ./internal/testfixtures/ -tags=integration` ok;
golangci-lint 0 issues; `pnpm lint/typecheck/check:api/test` ok (425 tests).
Mutations:
- company_id declared on public routes too → RED (`system.openapi is public and must not declare company_id`). Restored.
- `settings.analyzers.edit` given to building_admin → first attempt INVALID (gofmt alignment made the
  string replace a no-op, tests stayed green); redone with a regex → RED (matrix + fixture guards). Restored.

## Task 2 — R192 GET /jobs/{id} (d99d9e5)
Service tests written after the first implementation draft (deviation from strict TDD, recorded);
the retention guard and the HTTP test were written red first
(`integration.refresh_analyzer must be retained`; matrix row missing → 418).
Deviation: `asynq.Retention` first landed on `NewSyncDispatchTask` by a mis-targeted edit; moved to
`NewSyncAnalyzersTask` (the scheduler's dispatch task is not watchable).
Gate: api/v1 (+kit, mw), service/jobs, job, apiwire integration ok; lint 0; web typecheck/check:api ok.
Mutations: drop the company check → RED; `retry` mapped to failed → RED; drop the analyzer visibility
check → RED. All restored.

## Task 3 — R193 GET /consumption/grouped (64f91ae)
Domain tests first (red: no non-test files), then service tests (red: missing deps), then HTTP.
Deviations: an unknown `group_by` answers **422 validation_failed** (the DTO enum, R154) rather than
the plan's 400; the plan's `TestGroupSumsNonNilAndMarksPartial` needed a day with all three registers
for the "not partial" case.
Gate: domain/grouping (-race), service/analysis, service/loadprofile, api/v1 integration ok;
golangci-lint 0 after fixing 4 findings; internal/arch ok; web typecheck ok.
Mutations: Sunday-start weeks → RED (2 tests); nil summed as zero → RED (2); December in the previous
winter → RED; hardcoded Sat/Sun instead of the company calendar → first attempt INVALID (did not
compile), redone → RED. All restored.

## Task 3b — R210 GridBox wiring numbers (ce12c9b)
Tests first at the service level, then HTTP. Deviations: `validate:"dive,required"` trips the repo's
`TestRequestTypesAreConsistent` guard (it reads `required` anywhere in the tag), so the DTO uses
`dive,min=1`; `credentials.Deps` gained `Buildings`, which required updating apiwire, worker and four
test call sites.
Gate: credentials, api/v1 (+kit, mw), worker integration ok; golangci-lint 0; web typecheck ok.
**One unreproduced failure:** a first combined run of credentials+api/v1+worker reported FAIL for
api/v1; the failing test name was lost to an over-filtered grep, and three subsequent identical runs
were green. Watched for at phase end.
Mutations: delete analyzers missing from a later wiring list → RED; replace the caller scope with a
same-company SystemScope → SURVIVED (equivalent: the foreign building belongs to another company
either way); removing the building check entirely → RED. All restored.

## Task 4 — R194 seed data + web plumbing (e4068c0)
Deviation: readings/bill first went into `E2EFixtures` and broke four Go tests written against empty
fixtures (bill PDF, comparison, bills scope, load-profile keys); moved to `seed.E2EData`, called only
by `ekokod seed e2e`, which is what R194 meant.
Also fixed: the OpenAPI reflector put an `enum` tag on the array instead of its items, so `include`
typed as a single value in the generated client (`moveArrayEnumsToItems` + `TestArrayEnumsDescribeTheirItems`).
D14 forced a design change in the building list: a checkbox cannot have a hidden label, so the
checkbox's own label is the building name and selection moved to an explicit "Seç" button.
New dev dependency: `openapi-typescript-helpers@0.1.0` (already a transitive dep) for the mutation
helper's path constraint.
Gate: seed/api/v1 integration ok; lint 0; web lint/typecheck/parity/contrast ok; 448 web tests.
Mutations: `istanbulToday` in UTC → RED; scope picker ignoring the stored building → RED; fixture
readings restarting each run → RED (demo idempotency). All restored.

## Task 5 — dashboard panels (9a03a0b)
Views + containers per R195; stories for each (the repo guard requires one per component).
Gate: lint, typecheck, 465 web tests.
Mutations: markers drawn for coordinate-less records → RED; latest bill always company scope → RED;
advisory read from rows[0] instead of any row → **SURVIVED** (the fixture had the over-limit analyzer
first) → added "only a later analyzer is over" case, mutation then RED. All restored.

## Task 13 — Calendar (commit a31a0fe)

Gates: lint 0, typecheck 0, vitest 178 files / 637 tests, i18n parity 17 ns / 860 keys,
contrast 70 pairs, check:api ok, `pnpm build` + `calendar.spec.ts` 4/4 (workers=1).

Mutations (each reverted after proving red):
1. `startOfWeek` → Sunday-first (`addDays(iso, -weekday)`): 4 red
   (range: month grid start, week range; month-view header order; dates test).
2. `layoutLanes` first free lane → always lane 0: 3 red
   (lane assignment, shared lane count, time-grid column widths).
3. `toEventRequest` all-day end no longer exclusive: 1 red (R202 whole-day cover).

Deviations: none. Two e2e fixes were my own test's fault — `getByRole('checkbox', {name:'Cuma'})`
also matched "Cumartesi" (now `exact`), and the restored weekend pair comes back in the API's
own order, so that one assertion accepts either order while the Friday+Saturday one stays strict.
D5 forbids hex literals outside `src/styles/**`, so the calendar fixtures, tests and stories take
their colours from `@/styles/event-palette` rather than naming them.
