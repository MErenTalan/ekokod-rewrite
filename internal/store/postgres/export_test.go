package postgres

import "github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"

// Queries exposes BillRepository's unexported generated-query handle to the
// external postgres_test package.
//
// F2 Task 0P2, step 4 (testfixtures -> postgres import cycle): testfixtures
// imports this package (for MigrateUp), so postgres importing testfixtures
// back would cycle — which is exactly why
// bills_hourly_detail_internal_test.go used to live in package postgres
// itself (same-package code can reach an unexported field with no export
// needed) and duplicate ~20 lines of testfixtures.StartPostgres +
// postgres.NewPool locally to boot its own container, rather than share
// testfixtures.NewIsolatedDB's one container with the rest of this
// package's integration tests.
//
// This accessor is the leaf: it exports exactly the one thing that test
// needs — direct access to the generated query set, to call
// BillHourlyDetailInsert without going through
// BillRepository.ReplaceHourlyDetail's Go-side requireVisible pre-check —
// and nothing else, so the test itself could move to postgres_test and use
// testfixtures.NewIsolatedDB like every other integration test in this
// directory.
func (r *BillRepository) Queries() *sqlcgen.Queries { return r.q }

// HighestEmbeddedVersion exposes the newest embedded migration version, so a
// round-trip test can step down to a fixed version as migrations are added.
func HighestEmbeddedVersion() (int64, error) { return highestEmbeddedVersion() }
