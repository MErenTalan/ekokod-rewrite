//go:build integration

package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/scheduler"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

// entryIDs returns the scheduler entry IDs currently visible in Redis.
func entryIDs(t *testing.T, insp *asynq.Inspector) []string {
	t.Helper()
	entries, err := insp.SchedulerEntries()
	require.NoError(t, err)
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}

// TestSchedulerBuildsAFreshAsynqSchedulerEveryTerm pins the invariant that
// every leadership term builds and runs its own *asynq.Scheduler, rather
// than reusing one from an earlier term on the same Scheduler.Run call.
//
// asynq schedulers cannot be restarted: calling Start() on an
// already-Shutdown() *asynq.Scheduler returns "asynq: the scheduler has
// already been stopped" without registering or running anything. Scheduler
// currently gets this right by constructing a new asynq.Scheduler inside the
// lead closure passed to Elector.Run, so a fresh instance is built on every
// call - but nothing pins that. A natural-looking refactor that hoists the
// asynq.NewScheduler(...) construction out of the closure (built once, then
// reused across terms) would compile fine and would leave every term after
// the first with no cron actually running in Redis, while IsLeader() still
// reports true cluster-wide - a silent, total loss of scheduled work with no
// error surfaced anywhere (the reviewer noted the Start() error path doesn't
// even attempt to shut anything down).
//
// This test forces exactly that lose/regain cycle on a single Scheduler.Run
// call - by killing the Postgres backend holding the advisory lock directly,
// the same technique elector_integration_test.go's
// TestLeadContextIsCancelledWhenTheConnectionDies uses, never cancelling
// Scheduler's own context - and asserts, via asynq.Inspector.SchedulerEntries,
// that the second term actually registers a *new* entry ID. A reused,
// already-stopped asynq.Scheduler would never produce one: its Start() call
// would simply fail.
func TestSchedulerBuildsAFreshAsynqSchedulerEveryTerm(t *testing.T) {
	dsn := testfixtures.StartPostgresUnmigrated(t)
	redisCfg := testfixtures.RedisConfig(t)
	log := testfixtures.DiscardLogger()

	// F2's entries() adds two more cron entries (integration.sync_dispatch,
	// epias.sync_prices) driven by cfg.Schedule — a zero-valued Schedule
	// gives them an empty (invalid) cron string, which fails Register and
	// stops the scheduler from ever reaching "running" at all. Schedule
	// must be set to something asynq's cron parser accepts, same as every
	// other test in this file that now exercises Run/entries() end to end.
	cfg := &config.Config{
		Redis:    redisCfg,
		Timezone: time.UTC,
		Schedule: config.Schedule{Ingestion: "0 3 * * *", EPIAS: "0 14 * * *", Billing: "0 6 * * *", Demo: "15 * * * *",
			Alarms: "0 * * * *", ReportsMonthly: "0 6 2 * *", ReportsYearly: "0 7 3 1 *",
			ISolarSync: "*/15 * * * *", ISolarAlarms: "10 * * * *", Carbon: "30 4 * * *"},
	}
	pool := testfixtures.NewPool(t, dsn)

	opt, err := job.RedisOpt(redisCfg)
	require.NoError(t, err)
	insp := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = insp.Close() })

	sched := scheduler.New(cfg, pool, log)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = sched.Run(ctx) }()

	// Term 1: wait for the scheduler to win leadership, build its
	// asynq.Scheduler, register the noop entry and heartbeat it into Redis.
	var term1 []string
	require.Eventually(t, func() bool {
		term1 = entryIDs(t, insp)
		return len(term1) > 0
	}, 20*time.Second, 250*time.Millisecond, "the first term never registered a scheduler entry")

	// Force the loss of leadership by killing the Postgres backend holding
	// the advisory lock directly, from a completely separate connection.
	// Scheduler's own ctx is never touched: the same Scheduler.Run call must
	// notice on its own, shut its asynq.Scheduler down (which also clears
	// its entries from Redis), and go on to campaign for and regain
	// leadership by itself.
	admin := testfixtures.NewPool(t, dsn)
	var pid int32
	require.NoError(t, admin.QueryRow(context.Background(),
		`select pid from pg_locks where locktype = 'advisory' limit 1`).Scan(&pid))
	_, err = admin.Exec(context.Background(), `select pg_terminate_backend($1)`, pid)
	require.NoError(t, err)

	// Term 2: the same Scheduler.Run call must regain leadership and
	// register a *new* entry, distinct from term 1's. A hoisted, reused
	// *asynq.Scheduler would never produce a new (or any) entry ID here,
	// because its Start() call would fail with "the scheduler has already
	// been stopped" - the regression this test exists to catch.
	require.Eventually(t, func() bool {
		term2 := entryIDs(t, insp)
		if len(term2) == 0 {
			return false
		}
		for _, id := range term2 {
			for _, old := range term1 {
				if id == old {
					return false
				}
			}
		}
		return true
	}, 45*time.Second, 250*time.Millisecond,
		"the second leadership term never registered a fresh scheduler entry - "+
			"a reused asynq.Scheduler across terms would silently stop cron cluster-wide")
}
