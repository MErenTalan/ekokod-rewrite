//go:build integration

package testfixtures

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// This file is F4 Task 0's requirement-6 proof, deliberately in package
// testfixtures (white-box, like isolated_db_internal_integration_test.go):
// requirement 6b calls ensureIsolatedTemplate directly, which is
// unexported.

// TestNewIsolatedDBClonesAreDistinctOnSharedServer is requirement 6a: with
// EKOKOD_TEST_PG_DSN set, two NewIsolatedDB clones must still be genuinely
// distinct databases — a write through one pool must be invisible through
// the other — exactly as TestNewIsolatedDBGivesEachCallItsOwnDatabase
// already proves for the container-per-binary path. This test additionally
// pins WHERE that isolation now comes from: isolatedProcessSalt in the
// clone name, not merely "a different container", since under the shared
// server there is only one container for every test binary that opts in.
//
// Skipped (not failed) when the env var is unset: this test is specifically
// about the shared-server path's behaviour, not a duplicate of the
// container-per-binary test that already runs unconditionally.
func TestNewIsolatedDBClonesAreDistinctOnSharedServer(t *testing.T) {
	if os.Getenv(isolatedTestDSNEnv) == "" {
		t.Skipf("%s not set; this test is specific to the shared-server path", isolatedTestDSNEnv)
	}
	ctx := context.Background()

	first := NewIsolatedDB(t)
	second := NewIsolatedDB(t)

	_, err := first.Exec(ctx, `insert into companies (name) values ('shared-server-probe')`)
	require.NoError(t, err)

	var onSecond int
	require.NoError(t, second.QueryRow(ctx,
		`select count(*) from companies where name = 'shared-server-probe'`).Scan(&onSecond))
	require.Zero(t, onSecond, "a row inserted through one clone must not be visible through another clone on the shared server")

	var onFirst int
	require.NoError(t, first.QueryRow(ctx,
		`select count(*) from companies where name = 'shared-server-probe'`).Scan(&onFirst))
	require.Equal(t, 1, onFirst, "the row must still be visible through the pool that inserted it")
}

// TestEnsureIsolatedTemplateIsSafeUnderConcurrentProcessSimulation is
// requirement 6b: several goroutines, each over its OWN root connection
// (pg_advisory_lock is session-level, so a separate connection per
// goroutine is a valid stand-in for a separate process — the lock cannot
// tell the difference), race ensureIsolatedTemplate for the SAME template
// name. Exactly one of them may actually build it; every one of them must
// return with no error, and afterwards exactly one usable (migrated,
// protected) template must exist under that name.
//
// A name distinct from the real shared template (isolatedTemplateName) is
// used deliberately: this test must not interfere with, or be interfered
// by, whatever other tests in this same process are doing with the actual
// template that NewIsolatedDB/NewEmptyDB depend on.
func TestEnsureIsolatedTemplateIsSafeUnderConcurrentProcessSimulation(t *testing.T) {
	ctx := context.Background()
	rootDSN := isolatedRoot(t)

	realName, err := isolatedTemplateName()
	require.NoError(t, err)
	raceName := realName + "_racesim"

	root, err := pgx.Connect(ctx, rootDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		raceIdent := pgx.Identifier{raceName}.Sanitize()
		// A protected template (IS_TEMPLATE true) refuses a plain DROP even
		// WITH (FORCE) — the same reason the real shared template is never
		// dropped except by container teardown. This is a throwaway
		// test-private template, so unmark it first, then drop it, or the
		// next run of this test finds a leftover template and never
		// actually exercises the race at all.
		_, _ = root.Exec(context.Background(), fmt.Sprintf("alter database %s with is_template false", raceIdent))
		_, _ = root.Exec(context.Background(), "drop database if exists "+raceIdent+" with (force)")
		_ = root.Close(context.Background())
	})

	const racers = 8
	errs := make([]error, racers)
	var wg sync.WaitGroup
	wg.Add(racers)
	for i := range racers {
		go func(i int) {
			defer wg.Done()
			errs[i] = ensureIsolatedTemplate(ctx, rootDSN, raceName)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "racer %d", i)
	}

	exists, err := templateExists(ctx, root, raceName)
	require.NoError(t, err)
	require.True(t, exists, "exactly one usable template must exist after the race")

	// "Usable" means fully protected, not merely present: a half-built
	// template would never even reach this name (see ensureIsolatedTemplate's
	// build-under-temp-name-then-rename doc comment), but confirm the
	// visible row really is the finished state, not a fluke of the name
	// check alone.
	var allowConn, isTemplate bool
	require.NoError(t, root.QueryRow(ctx,
		`select datallowconn, datistemplate from pg_database where datname = $1`, raceName,
	).Scan(&allowConn, &isTemplate))
	require.False(t, allowConn, "the raced template must refuse connections")
	require.True(t, isTemplate, "the raced template must be marked IS_TEMPLATE")

	// And clonable, proving it is actually migrated, not merely marked as a
	// template with an empty schema.
	cloneName := raceName + "_clone"
	require.NoError(t, cloneIsolatedDB(ctx, rootDSN, cloneName, raceName))
	t.Cleanup(func() { dropIsolatedDB(t, rootDSN, cloneName) })

	pool := NewPool(t, withDatabase(rootDSN, cloneName))
	var tables int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(*) from information_schema.tables where table_schema = 'public'`).Scan(&tables))
	require.Greater(t, tables, 0, "the raced template must have been fully migrated, not left empty")
}
