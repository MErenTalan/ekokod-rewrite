//go:build integration

package scheduler_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/scheduler"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "timescale/timescaledb:2.30.0-pg16",
		tcpostgres.WithDatabase("ekokod"),
		tcpostgres.WithUsername("ekokod"),
		tcpostgres.WithPassword("ekokod"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return dsn
}

func newPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool, err := postgres.NewPool(context.Background(), config.DB{
		URL: dsn, MaxConns: 4, MinConns: 1,
		MaxConnLifetime: time.Hour, StatementTimeout: 10 * time.Second,
	}, log)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// TestLeaderElection is named in the F0 acceptance criteria: exactly one of two
// simultaneously started instances leads, and killing it transfers leadership.
func TestLeaderElection(t *testing.T) {
	dsn := startPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	const retry = 200 * time.Millisecond

	first := scheduler.NewElector(newPool(t, dsn), scheduler.LockKeyScheduler, retry, log)
	second := scheduler.NewElector(newPool(t, dsn), scheduler.LockKeyScheduler, retry, log)

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
	dsn := startPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	elector := scheduler.NewElector(newPool(t, dsn), scheduler.LockKeyScheduler, 100*time.Millisecond, log)
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
	dsn := startPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	elector := scheduler.NewElector(newPool(t, dsn), scheduler.LockKeyScheduler, 100*time.Millisecond, log)
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
	admin := newPool(t, dsn)
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
