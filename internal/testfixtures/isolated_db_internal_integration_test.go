//go:build integration

package testfixtures

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// This file is a WHITE-BOX companion to isolated_db_integration_test.go,
// deliberately in package testfixtures (not testfixtures_test), because I7a
// (template protection) is only provable against the unexported
// isolatedTemplate, ensureIsolatedTemplate and cloneIsolatedDB — the
// black-box tests in isolated_db_integration_test.go only ever see
// NewIsolatedDB's finished pool, never the template database itself.

// TestCreateDatabaseTemplateFailsWith55006WhileASessionIsConnected pins the
// underlying Postgres mechanism the whole template-protection design exists
// to defend against, deterministically rather than by chance: while ANY
// session — this test's own, a straggler client, or TimescaleDB's
// per-database background worker scheduler — is connected to a database,
// `CREATE DATABASE … TEMPLATE` against it fails with SQLSTATE 55006
// ("source database … is being accessed by other users"). Nothing here
// exercises this package's own code; it documents the failure mode that
// terminateSessionsAndRetry and the ALLOW_CONNECTIONS protection both exist
// to remove.
//
// This deliberately does NOT use the shared template: after the I7a fix
// that database refuses every connection once isolatedTemplate has protected it
// (see TestIsolatedTemplateRefusesConnectionsAfterSetup), which would make
// this test's own holder connection fail before it could ever demonstrate
// 55006. A throwaway, UNPROTECTED database makes this test prove the
// generic Postgres mechanism independently of whether this package's own
// fix is applied.
func TestCreateDatabaseTemplateFailsWith55006WhileASessionIsConnected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rootDSN := isolatedRoot(t)

	root, err := pgx.Connect(ctx, rootDSN)
	require.NoError(t, err)
	defer func() { _ = root.Close(ctx) }()

	// Named per-call via isolatedDBSeq: isolatedRoot's container, and every
	// name this test creates in it, is shared across every t.Parallel
	// caller AND across every `go test -count=N` repeat within the same
	// process — a literal name here collided with itself on repeat two.
	seq := isolatedDBSeq.Add(1)
	sourceName := fmt.Sprintf("mechanism_probe_source_%d", seq)
	cloneName := fmt.Sprintf("mechanism_probe_clone_%d", seq)

	sourceIdent := pgx.Identifier{sourceName}.Sanitize()
	_, err = root.Exec(ctx, "create database "+sourceIdent)
	require.NoError(t, err, "creating the throwaway, unprotected source database")
	defer func() {
		_, _ = root.Exec(ctx, "drop database if exists "+sourceIdent+" with (force)")
	}()

	// A session connected to the source, held open for the duration —
	// analogous to a straggler client or the TimescaleDB scheduler backend
	// that terminateSessionsAndRetry's pre-emptive terminate exists to catch.
	holder, err := pgx.Connect(ctx, withDatabase(rootDSN, sourceName))
	require.NoError(t, err)
	defer func() { _ = holder.Close(ctx) }()

	cloneIdent := pgx.Identifier{cloneName}.Sanitize()
	_, err = root.Exec(ctx, fmt.Sprintf("create database %s template %s", cloneIdent, sourceIdent))
	require.Error(t, err, "CREATE DATABASE ... TEMPLATE against a source with an open session must fail")

	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a *pgconn.PgError, got %T: %v", err, err)
	require.Equal(t, "55006", pgErr.Code, "expected SQLSTATE 55006 (source database is being accessed by other users), got %s: %v", pgErr.Code, err)
	t.Logf("observed expected 55006: %v", err)
}

// TestIsolatedTemplateRefusesConnectionsAfterSetup is the regression test for
// I7a. Before the fix, isolatedTemplate left the template fully connectable
// after migrating it, so nothing stopped a straggler session (or
// TimescaleDB's own per-database scheduler, spawned because migrations add
// continuous-aggregate and compression policies — see 00005) from attaching
// to it at any point after setup, which is exactly the session
// TestCreateDatabaseTemplateFailsWith55006WhileASessionIsConnected shows
// turns a concurrent clone into a 55006 failure. After the fix, the template
// is permanently marked ALLOW_CONNECTIONS false once isolatedTemplate
// returns, so no session — however persistent, however it reconnects — can
// ever attach to it again: connecting is refused at the protocol level,
// before any query (including a terminate) could even race it.
//
// This test FAILS on the pre-fix code (connecting succeeds) and PASSES once
// isolatedTemplate protects the template — see the task-0p report for the
// observed pre-fix failure.
func TestIsolatedTemplateRefusesConnectionsAfterSetup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rootDSN := isolatedRoot(t)
	templateName := isolatedTemplate(t, rootDSN)

	_, err := pgx.Connect(ctx, withDatabase(rootDSN, templateName))
	require.Error(t, err, "the template must refuse new connections once isolatedTemplate has protected it")

	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a *pgconn.PgError refusing the connection, got %T: %v", err, err)
	require.Equal(t, "55000", pgErr.Code,
		"expected SQLSTATE 55000 (object_not_in_prerequisite_state: \"database … is not currently accepting connections\", what Postgres returns for ALLOW_CONNECTIONS false), got %s: %v", pgErr.Code, err)
	t.Logf("observed expected connection refusal: %v", err)
}

// TestCloneIsolatedDBSurvivesConcurrentConnectionFloodOnTemplate is I7a's
// stress proof: many goroutines repeatedly attempt to connect directly to
// the template (the closest a test can get to simulating an
// uncooperative straggler or a scheduler that keeps trying to reconnect)
// while many other goroutines clone it via cloneIsolatedDB, all
// concurrently, under -race. Every connection attempt must be refused
// (proving the template stays protected under contention, not just at
// rest) and every clone must succeed (proving the protection, not luck,
// is what makes cloning reliable).
func TestCloneIsolatedDBSurvivesConcurrentConnectionFloodOnTemplate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rootDSN := isolatedRoot(t)
	templateName := isolatedTemplate(t, rootDSN)

	stop := make(chan struct{})
	var floodAttempts, floodRefused atomic.Int64
	var floodWG sync.WaitGroup
	for i := 0; i < 8; i++ {
		floodWG.Add(1)
		go func() {
			defer floodWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				floodAttempts.Add(1)
				conn, err := pgx.Connect(ctx, withDatabase(rootDSN, templateName))
				if err != nil {
					floodRefused.Add(1)
					continue
				}
				// Should never happen once the template is protected — leave
				// the assertion to the caller via floodAttempts/floodRefused
				// rather than failing from inside a goroutine.
				_ = conn.Close(ctx)
			}
		}()
	}

	var cloneWG sync.WaitGroup
	var cloneFailures atomic.Int64
	for i := 0; i < 16; i++ {
		cloneWG.Add(1)
		go func(i int) {
			defer cloneWG.Done()
			name := fmt.Sprintf("flood_clone_%d", i)
			if err := cloneIsolatedDB(ctx, rootDSN, name, templateName); err != nil {
				cloneFailures.Add(1)
				t.Errorf("clone %d failed under connection-flood contention: %v", i, err)
				return
			}
			root, err := pgx.Connect(ctx, rootDSN)
			if err != nil {
				return
			}
			defer func() { _ = root.Close(ctx) }()
			_, _ = root.Exec(ctx, "drop database if exists "+pgx.Identifier{name}.Sanitize()+" with (force)")
		}(i)
	}
	cloneWG.Wait()
	close(stop)
	floodWG.Wait()

	t.Logf("connection-flood attempts: %d, refused: %d, clone failures: %d",
		floodAttempts.Load(), floodRefused.Load(), cloneFailures.Load())
	require.Equal(t, floodAttempts.Load(), floodRefused.Load(),
		"every connection attempt against the protected template must be refused")
	require.Zero(t, cloneFailures.Load(), "no clone may fail while the template is protected, however hard something floods it with connection attempts")
}
