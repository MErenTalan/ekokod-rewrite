//go:build integration

package scheduler_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/scheduler"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/stretchr/testify/require"
)

// These tests share one Postgres advisory-lock key namespace
// (scheduler.LockKeyScheduler) and, where several run leader-election
// loops, contend for real wall-clock leadership transitions — running them
// in parallel with t.Parallel() would make one test's lock acquisition or
// backend-termination race another's, so every test in this file stays
// serial (the existing tests above already do; new ones below follow the
// same rule).

// TestLeaderElection is named in the F0 acceptance criteria: exactly one of two
// simultaneously started instances leads, and killing it transfers leadership.
func TestLeaderElection(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	log := testfixtures.DiscardLogger()
	const retry = 200 * time.Millisecond

	first := scheduler.NewElector(testfixtures.NewPool(t, dsn), scheduler.LockKeyScheduler, retry, log)
	second := scheduler.NewElector(testfixtures.NewPool(t, dsn), scheduler.LockKeyScheduler, retry, log)

	firstCtx, stopFirst := context.WithCancel(context.Background())
	secondCtx, stopSecond := context.WithCancel(context.Background())
	t.Cleanup(stopSecond)

	lead := func(ctx context.Context) error { <-ctx.Done(); return nil }
	go func() { _ = first.Run(firstCtx, lead) }()
	go func() { _ = second.Run(secondCtx, lead) }()

	require.Eventually(t, func() bool { return first.IsLeader() != second.IsLeader() },
		5*time.Second, 50*time.Millisecond, "exactly one instance must hold leadership")

	leaderIsFirst := first.IsLeader()
	require.False(t, first.IsLeader() && second.IsLeader(), "leadership must be exclusive")

	// Kill the leader; the follower must take over.
	if leaderIsFirst {
		stopFirst()
		require.Eventually(t, second.IsLeader, 5*time.Second, 50*time.Millisecond,
			"leadership must transfer when the leader stops")
	} else {
		stopSecond()
		require.Eventually(t, first.IsLeader, 5*time.Second, 50*time.Millisecond,
			"leadership must transfer when the leader stops")
		stopFirst()
	}
}

func TestLeadContextIsCancelledWhenTheProcessStops(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	log := testfixtures.DiscardLogger()

	elector := scheduler.NewElector(testfixtures.NewPool(t, dsn), scheduler.LockKeyScheduler, 100*time.Millisecond, log)
	ctx, cancel := context.WithCancel(context.Background())

	cancelled := make(chan struct{})
	go func() {
		_ = elector.Run(ctx, func(leadCtx context.Context) error {
			<-leadCtx.Done()
			close(cancelled)
			return nil
		})
	}()

	require.Eventually(t, elector.IsLeader, 5*time.Second, 50*time.Millisecond)
	cancel()

	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the lead function's context was not cancelled on shutdown")
	}
	require.False(t, elector.IsLeader())
}

// TestLeadContextIsCancelledWhenTheConnectionDies goes beyond the brief's own
// tests, which only exercise cooperative shutdown (cancelling the outer
// context, which cancels the lead context by Go context propagation alone —
// that would pass even if the elector never actually monitored the
// connection holding the advisory lock). This test kills the Postgres
// backend that holds the lock directly, bypassing the elector's context
// entirely, to prove leadership is actually monitored: if the connection
// holding a session-level advisory lock dies, another replica can acquire
// it, so this instance's lead context must be cancelled promptly or two
// replicas could fire the same cron entries simultaneously.
func TestLeadContextIsCancelledWhenTheConnectionDies(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	log := testfixtures.DiscardLogger()

	elector := scheduler.NewElector(testfixtures.NewPool(t, dsn), scheduler.LockKeyScheduler, 100*time.Millisecond, log)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cancelled := make(chan struct{})
	go func() {
		_ = elector.Run(ctx, func(leadCtx context.Context) error {
			<-leadCtx.Done()
			close(cancelled)
			return nil
		})
	}()

	require.Eventually(t, elector.IsLeader, 5*time.Second, 50*time.Millisecond)

	// Find and kill the backend holding the advisory lock from a completely
	// separate connection, simulating a dropped connection or a database
	// session that was killed out from under the elector.
	admin := testfixtures.NewPool(t, dsn)
	var pid int32
	require.NoError(t, admin.QueryRow(context.Background(),
		`select pid from pg_locks where locktype = 'advisory' limit 1`).Scan(&pid))
	_, err := admin.Exec(context.Background(), `select pg_terminate_backend($1)`, pid)
	require.NoError(t, err)

	select {
	case <-cancelled:
	case <-time.After(10 * time.Second):
		t.Fatal("the lead function's context was not cancelled when the underlying connection died")
	}
	require.Eventually(t, func() bool { return !elector.IsLeader() }, 5*time.Second, 50*time.Millisecond)
}

// TestNonPositiveRetryDoesNotPanic guards NewElector's exported surface:
// time.NewTicker panics on a non-positive interval, and Run used to pass the
// caller's retry to it unclamped while monitor already guarded the identical
// value. A zero here must degrade to a sane interval, not take the process
// down.
func TestNonPositiveRetryDoesNotPanic(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	log := testfixtures.DiscardLogger()

	elector := scheduler.NewElector(testfixtures.NewPool(t, dsn), scheduler.LockKeyScheduler, 0, log)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() {
		done <- elector.Run(ctx, func(leadCtx context.Context) error { <-leadCtx.Done(); return nil })
	}()

	require.Eventually(t, elector.IsLeader, 10*time.Second, 50*time.Millisecond,
		"a zero retry must still campaign rather than panic")
	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

// TestElectorBacksOffOnRepeatedLeadFailure pins Carried-forward defect 2
// (F1 plan): consecutive lead panics/errors back off exponentially (10s
// doubling to 5m), rather than retrying at the fixed campaign cadence. A
// fake scheduler.WithWait records every wait duration Run asks for and
// never actually sleeps, so the test runs instantly despite exercising
// several "seconds"-scale backoff steps. lead deliberately returns an
// error every single term (never merely "did not win"), which — through
// real Postgres advisory-lock acquisition — is exactly the
// panic-or-error path Carried-forward defect 2 targets, distinct from the
// ordinary not-currently-leading retry cadence the other tests in this
// file exercise.
func TestElectorBacksOffOnRepeatedLeadFailure(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	log := testfixtures.DiscardLogger()

	var mu sync.Mutex
	var waits []time.Duration
	stop := errors.New("enough samples")
	fakeWait := func(_ context.Context, d time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		waits = append(waits, d)
		if len(waits) >= 4 {
			return stop
		}
		return nil
	}

	elector := scheduler.NewElector(testfixtures.NewPool(t, dsn), scheduler.LockKeyScheduler,
		10*time.Second, log, scheduler.WithWait(fakeWait))

	leadErr := errors.New("boom")
	_ = elector.Run(context.Background(), func(context.Context) error { return leadErr })

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(waits), 3, "expected at least 3 recorded backoff waits")
	require.Less(t, waits[0], waits[1], "backoff must increase after a second consecutive failure")
	require.Less(t, waits[1], waits[2], "backoff must increase after a third consecutive failure")
	require.Equal(t, 10*time.Second, waits[0], "the first failure must back off at the 10s base")
}
