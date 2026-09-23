package scheduler

// White-box (package scheduler, not scheduler_test) unit tests for the two
// Carried-forward-defect fixes and the F2 entries() additions that need
// access to unexported surfaces (releaseAdvisoryLock, entries). Neither
// test below needs Docker/Postgres: entries() is a pure function of *Config,
// and releaseAdvisoryLock is exercised through a fake satisfying the narrow
// advisoryUnlockQueryRower interface, not a real Postgres session — see
// advisoryUnlockQueryRower's doc comment in elector.go for why a real
// session cannot naturally reach the false branch through Elector's own
// public Run API (its own acquire/release pairing is always balanced).

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
)

// fakeUnlockRow is a pgx.Row that reports a fixed (unlocked, err) pair.
type fakeUnlockRow struct {
	unlocked bool
	err      error
}

func (r fakeUnlockRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	b, ok := dest[0].(*bool)
	if !ok {
		panic("fakeUnlockRow.Scan: dest[0] is not *bool")
	}
	*b = r.unlocked
	return nil
}

// fakeUnlockConn implements advisoryUnlockQueryRower and always returns row.
type fakeUnlockConn struct{ row fakeUnlockRow }

func (c fakeUnlockConn) QueryRow(context.Context, string, ...any) pgx.Row { return c.row }

// TestElectorLogsFailedUnlock pins Carried-forward defect 1 (F1 plan):
// releaseAdvisoryLock now reads pg_advisory_unlock's boolean and logs a
// warning when it is false, rather than discarding it (the pre-fix code
// used conn.Exec, which never even looked at the returned row).
func TestElectorLogsFailedUnlock(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	e := &Elector{key: LockKeyScheduler, log: log}

	e.releaseAdvisoryLock(fakeUnlockConn{row: fakeUnlockRow{unlocked: false}})

	require.Contains(t, buf.String(), "advisory unlock reported the lock was not held")
}

// TestElectorLogsFailedUnlockQueryError guards the OTHER branch stays
// intact: a genuine query error is still logged (as before the fix), and
// must not be confused with the new false-boolean warning's message.
func TestElectorLogsFailedUnlockQueryError(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	e := &Elector{key: LockKeyScheduler, log: log}

	e.releaseAdvisoryLock(fakeUnlockConn{row: fakeUnlockRow{err: context.DeadlineExceeded}})

	require.Contains(t, buf.String(), "advisory unlock failed")
	require.NotContains(t, buf.String(), "reported the lock was not held")
}

// TestSchedulerEntriesIncludeIngestionAndPrices pins entries()'s F2
// additions: the F0 noop heartbeat stays first, followed by
// integration.sync_dispatch on cfg.Schedule.Ingestion and epias.sync_prices
// on cfg.Schedule.EPIAS, both carrying cfg.Worker.MaxRetries.
func TestSchedulerEntriesIncludeIngestionAndPrices(t *testing.T) {
	cfg := &config.Config{
		Timezone: time.UTC,
		Schedule: config.Schedule{Ingestion: "0 3 * * *", EPIAS: "0 14 * * *", Billing: "0 6 * * *",
			Demo: "15 * * * *", Alarms: "0 * * * *", ReportsMonthly: "0 6 2 * *", ReportsYearly: "0 7 3 1 *",
			ISolarSync: "*/15 * * * *", ISolarAlarms: "10 * * * *", Carbon: "30 4 * * *"},
		Worker: config.Worker{MaxRetries: 5},
	}
	s := &Scheduler{cfg: cfg, log: slog.New(slog.DiscardHandler)}

	entries, err := s.entries()
	require.NoError(t, err)
	require.Len(t, entries, 11)

	require.Equal(t, "@every 1h", entries[0].Cron)
	require.Equal(t, job.TypeNoop, entries[0].Task.Type())

	require.Equal(t, cfg.Schedule.Ingestion, entries[1].Cron)
	require.Equal(t, job.TypeIntegrationSyncDispatch, entries[1].Task.Type())

	require.Equal(t, cfg.Schedule.EPIAS, entries[2].Cron)
	require.Equal(t, job.TypeEPIASSyncPrices, entries[2].Task.Type())

	// TestSchedulerRegistersBillingDispatch (F4 Task 10).
	require.Equal(t, cfg.Schedule.Billing, entries[3].Cron)
	require.Equal(t, job.TypeBillingDispatch, entries[3].Task.Type())

	// F7: alarm.dispatch on cfg.Schedule.Alarms (01 §8, hourly).
	require.Equal(t, cfg.Schedule.Alarms, entries[5].Cron)
	require.Equal(t, job.TypeAlarmDispatch, entries[5].Task.Type())
	require.NotEmpty(t, entries[5].Cron, "an entry with an empty cron spec would silently never run")

	// F8b R268: the two report ticks.
	require.Equal(t, cfg.Schedule.ReportsMonthly, entries[6].Cron)
	require.Equal(t, job.TypeReportDispatchMonthly, entries[6].Task.Type())
	require.Equal(t, cfg.Schedule.ReportsYearly, entries[7].Cron)
	require.Equal(t, job.TypeReportDispatchYearly, entries[7].Task.Type())

	// F9 R288: the two iSolar ticks.
	require.Equal(t, cfg.Schedule.ISolarSync, entries[8].Cron)
	require.Equal(t, job.TypeSolarDispatchSync, entries[8].Task.Type())
	require.Equal(t, cfg.Schedule.ISolarAlarms, entries[9].Cron)
	require.Equal(t, job.TypeSolarFetchAlarms, entries[9].Task.Type())

	// F10a R311: the daily carbon accrual.
	require.Equal(t, cfg.Schedule.Carbon, entries[10].Cron)
	require.Equal(t, job.TypeCarbonAccrual, entries[10].Task.Type())
}
