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
