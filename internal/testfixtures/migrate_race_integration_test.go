//go:build integration

package testfixtures_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestConcurrentMigrateUpOnDifferentDatabasesIsRaceFree is step 7 of the
// t.Parallel readiness plan (final-review-B-report.md §"t.Parallel
// readiness", item 7): goose v3.28.0's SetBaseFS/SetDialect/SetLogger are
// plain package-level variables with no synchronisation of their own —
// internal/store/postgres/migrate.go's gooseSetupOnce and gooseLogMu exist
// specifically to make calling them safe. That guard is only proven if two
// MigrateUp calls against two DIFFERENT databases, run concurrently, are
// race-free — which is exactly what NewEmptyDB now makes cheap to set up
// (two fresh, empty databases in the ONE shared container, rather than two
// containers), and exactly what every isolated-database repository test
// will do once the postgres package itself is converted to t.Parallel.
//
// Run under `-race`, this catches an unsynchronised read/write on goose's
// package-level state directly; it does not merely trust the mutex/sync.Once
// pairing in migrate.go from reading it.
func TestConcurrentMigrateUpOnDifferentDatabasesIsRaceFree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	log := testfixtures.DiscardLogger()

	dsnA := testfixtures.NewEmptyDB(t)
	dsnB := testfixtures.NewEmptyDB(t)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = postgres.MigrateUp(ctx, dsnA, log)
	}()
	go func() {
		defer wg.Done()
		errs[1] = postgres.MigrateUp(ctx, dsnB, log)
	}()
	wg.Wait()

	require.NoError(t, errs[0], "MigrateUp against database A")
	require.NoError(t, errs[1], "MigrateUp against database B")

	// Both databases must have reached the same, fully-migrated schema —
	// not merely "no error", since a race on goose's shared migration list
	// could in principle let one run silently skip or duplicate a step
	// without returning an error.
	poolA := testfixtures.NewPool(t, dsnA)
	poolB := testfixtures.NewPool(t, dsnB)

	var tablesA, tablesB int
	require.NoError(t, poolA.QueryRow(ctx,
		`select count(*) from information_schema.tables where table_schema = 'public'`).Scan(&tablesA))
	require.NoError(t, poolB.QueryRow(ctx,
		`select count(*) from information_schema.tables where table_schema = 'public'`).Scan(&tablesB))
	require.Equal(t, tablesA, tablesB, "both concurrently-migrated databases must end up with the same table count")
	require.Greater(t, tablesA, 0, "sanity: migrating must have created at least one table")
}
