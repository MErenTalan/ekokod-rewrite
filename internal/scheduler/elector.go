// Package scheduler enqueues periodic work. Exactly one instance is active at a
// time, elected with a Postgres session-scoped advisory lock. There is no OS
// cron and no shared API key.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LockKeyScheduler is the advisory-lock key for scheduler leadership.
// The value spells "ekokod" in ASCII.
const LockKeyScheduler int64 = 0x656B6F6B6F64

// maxWatchInterval bounds how long it can take to notice that the connection
// holding the advisory lock has died, independent of how long retry (the
// re-acquisition interval passed to NewElector) is configured to be. Without
// this cap, a production retry of e.g. 10s (see New) would mean a dead
// connection could go undetected for up to 10s, during which another
// replica could acquire the lock while this one still believes it leads.
const maxWatchInterval = 2 * time.Second

// backoffBase and backoffMax bound the exponential backoff Run applies
// after a lead panic or error (Carried-forward defect 2, F1 plan): 10s,
// doubling on every consecutive failure, capped at 5 minutes. A "failed
// attempt" here is an *error* from attempt (an acquire/lock-query failure,
// or lead itself panicking or returning an error) — never the ordinary
// "did not win the election this round" outcome (attempt returns nil when
// pg_try_advisory_lock simply reports false), which keeps retrying at the
// caller's configured, non-backed-off retry cadence.
const (
	backoffBase = 10 * time.Second
	backoffMax  = 5 * time.Minute
)

// backoffDuration returns the wait after n consecutive failures (n >= 1):
// backoffBase, doubling each time, capped at backoffMax.
func backoffDuration(n int) time.Duration {
	if n <= 1 {
		return backoffBase
	}
	d := backoffBase
	for i := 1; i < n; i++ {
		if d >= backoffMax {
			return backoffMax
		}
		d *= 2
	}
	if d > backoffMax {
		return backoffMax
	}
	return d
}

// waitFunc waits out one retry/backoff interval, honouring ctx. Real
// production use is realWait (a timer); tests inject a fake that returns
// immediately and records the durations it was asked to wait, so a backoff
// test never actually sleeps.
type waitFunc func(ctx context.Context, d time.Duration) error

// realWait is the production waitFunc: a real timer, cancellable by ctx.
func realWait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Elector acquires and holds scheduler leadership.
type Elector struct {
	pool   *pgxpool.Pool
	key    int64
	retry  time.Duration
	log    *slog.Logger
	leader atomic.Bool
	wait   waitFunc
}

// ElectorOption configures an optional testing seam on an Elector.
type ElectorOption func(*Elector)

// WithWait overrides the wait Run uses between attempts (both the normal
// retry cadence and the exponential backoff after a failure). Tests use a
// fake, non-blocking implementation that records the durations it was
// asked to wait. Production callers never need this: NewElector defaults
// to a real timer.
func WithWait(fn waitFunc) ElectorOption {
	return func(e *Elector) { e.wait = fn }
}

// NewElector builds an Elector that retries acquisition every retry interval.
func NewElector(pool *pgxpool.Pool, key int64, retry time.Duration, log *slog.Logger, opts ...ElectorOption) *Elector {
	e := &Elector{pool: pool, key: key, retry: retry, log: log, wait: realWait}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// IsLeader reports whether this instance currently holds leadership. It is
// read concurrently with the goroutine that updates it (Run, and the
// connection watcher it starts), so it is backed by atomic.Bool rather than
// a plain field.
func (e *Elector) IsLeader() bool { return e.leader.Load() }

// Run campaigns for leadership until ctx is done. While leading it calls lead
// with a context that is cancelled as soon as leadership is lost, whether
// because ctx was cancelled or because the connection holding the lock died.
// Run returns promptly when ctx is cancelled at every stage: while waiting to
// acquire, while leading, and while waiting to retry.
func (e *Elector) Run(ctx context.Context, lead func(ctx context.Context) error) error {
	// time.NewTimer panics on a non-positive interval, and retry reaches
	// here straight from the exported NewElector, so a caller's zero would
	// crash the process rather than fail a check. Clamp it exactly as
	// monitor already clamps its own copy of the same value, instead of
	// returning an error: a retry interval is a tuning knob, not a
	// correctness input, and there is no caller that could act on the error.
	// Only the lower bound is shared with monitor: monitor additionally caps
	// its interval at maxWatchInterval, which must not apply here or a
	// production retry of 10s would silently become 2s.
	retry := e.retry
	if retry <= 0 {
		retry = maxWatchInterval
	}
	wait := e.wait
	if wait == nil {
		wait = realWait
	}

	// consecutiveFailures counts attempt() returning a genuine error (a
	// panic inside lead, lead itself returning an error, or the
	// acquire/advisory-lock query failing) — never the ordinary "another
	// instance is leading" outcome, which attempt reports as a nil error.
	// Carried-forward defect 2 (F1 plan): only the failure path backs off
	// exponentially; the normal campaign cadence is untouched.
	var consecutiveFailures int
	for {
		d := retry
		if err := e.attempt(ctx, lead); err != nil && ctx.Err() == nil {
			consecutiveFailures++
			d = backoffDuration(consecutiveFailures)
			e.log.Error("leader election attempt failed",
				slog.String("error", err.Error()),
				slog.Int("consecutive_failures", consecutiveFailures),
				slog.Duration("backoff", d))
		} else {
			consecutiveFailures = 0
		}
		if werr := wait(ctx, d); werr != nil {
			return ctx.Err()
		}
	}
}

// attempt tries once to become leader and, if successful, leads until the
// lock or the context is lost. It always releases the advisory lock and the
// connection it was acquired on before returning, including when lead
// panics.
func (e *Elector) attempt(ctx context.Context, lead func(ctx context.Context) error) (err error) {
	conn, err := e.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	// Postgres session-level advisory locks are bound to the connection that
	// takes them, not to the pool or a transaction. This connection must be
	// held for exactly as long as leadership lasts and released back to the
	// pool only afterwards - never handed back early while the lock is held.
	defer conn.Release()

	var acquired bool
	if err := conn.QueryRow(ctx, "select pg_try_advisory_lock($1)", e.key).Scan(&acquired); err != nil {
		return fmt.Errorf("try advisory lock: %w", err)
	}
	if !acquired {
		return nil
	}

	e.leader.Store(true)
	e.log.Info("scheduler leadership acquired", slog.Int64("lock_key", e.key))

	// leadCtx is deliberately NOT derived from ctx with context.WithCancel(ctx):
	// only the monitor goroutine below is allowed to cancel it, and monitor
	// always clears the leader flag first. If leadCtx were a direct child of
	// ctx, cancelling ctx would close leadCtx.Done() through Go's own context
	// propagation with no chance for this code to run in between, so a
	// caller's lead function could observe cancellation while IsLeader()
	// still (briefly, but visibly to a concurrent reader) reported true.
	leadCtx, cancelLead := context.WithCancel(context.Background())

	// monitorCtx bounds the monitor goroutine's lifetime: it ends either
	// because ctx (the caller's) is done, or because lead returned/panicked
	// on its own and the defer below asks it to stop.
	monitorCtx, stopMonitor := context.WithCancel(ctx)

	// monitorDone is closed once the monitor goroutine has actually
	// returned, not merely been asked to stop. We must wait for it before
	// touching conn again below (to run pg_advisory_unlock and to release
	// it): a pgx connection is not safe for concurrent use, and the monitor
	// calls conn.Ping concurrently with the rest of this function.
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		e.monitor(monitorCtx, conn, cancelLead)
	}()

	defer func() {
		stopMonitor()
		<-monitorDone

		// Idempotent belt-and-braces: monitor already cleared this on every
		// path that cancels leadCtx, but lead can also return on its own
		// (an error, or simply choosing not to wait for leadCtx.Done()),
		// in which case monitor never ran its own Store(false).
		cancelLead()
		e.leader.Store(false)

		// Release the session-level advisory lock explicitly, on the same
		// connection that took it. If the connection is already dead this
		// call fails harmlessly: Postgres releases every advisory lock
		// held by a session when that session ends, so the lock is not
		// actually leaked either way.
		e.releaseAdvisoryLock(conn)
		e.log.Info("scheduler leadership released")

		// A panic inside lead must still release the lock (the defers above
		// already ran) and must not crash the whole process: recover it and
		// report it as an ordinary error so Run's loop keeps campaigning.
		if r := recover(); r != nil {
			err = fmt.Errorf("lead panicked: %v", r)
		}
	}()

	return lead(leadCtx)
}

// monitor cancels leadership - clearing the leader flag before cancelling
// leadCtx, always in that order - as soon as either ctx is done or the
// connection holding the advisory lock stops responding. The latter is what
// makes leadership loss safe: if the connection holding a session-level
// advisory lock dies, Postgres lets another replica acquire it, so this
// instance's lead context must be cancelled promptly or two replicas could
// fire the same cron entries simultaneously.
func (e *Elector) monitor(ctx context.Context, conn *pgxpool.Conn, cancelLead context.CancelFunc) {
	interval := e.retry
	if interval <= 0 || interval > maxWatchInterval {
		interval = maxWatchInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.leader.Store(false)
			cancelLead()
			return
		case <-ticker.C:
			pingCtx, cancelPing := context.WithTimeout(ctx, interval)
			pingErr := conn.Ping(pingCtx)
			cancelPing()
			if pingErr != nil {
				if ctx.Err() == nil {
					e.log.Warn("lost the connection holding scheduler leadership",
						slog.String("error", pingErr.Error()))
				}
				e.leader.Store(false)
				cancelLead()
				return
			}
		}
	}
}

// advisoryUnlockQueryRower is the minimal surface releaseAdvisoryLock needs
// from *pgxpool.Conn — narrowed so the false/warning branch (Postgres
// reports a session releasing a lock it did not hold) is directly testable
// with a fake, without needing a real Postgres session. In production this
// is never actually reachable through Elector's own acquire/release
// pairing (every attempt() pairs exactly one pg_try_advisory_lock with
// exactly one pg_advisory_unlock on the same session, so the net count is
// always balanced) — it exists to surface an external anomaly (the
// session's lock state was already cleared out from under it) loudly
// instead of silently, per Carried-forward defect 1 (F1 plan).
type advisoryUnlockQueryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// releaseAdvisoryLock releases the advisory lock on conn's session, using a
// fresh short-lived context so a caller shutdown does not itself prevent
// the release. Carried-forward defect 1 (F1 plan): the boolean
// pg_advisory_unlock reports is now read (QueryRow.Scan), not discarded
// (the old code used Exec, which never even saw the return value), and a
// false result — the session did not hold the lock — is logged as a
// warning distinct from an outright query error.
func (e *Elector) releaseAdvisoryLock(conn advisoryUnlockQueryRower) {
	unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var unlocked bool
	if err := conn.QueryRow(unlockCtx, "select pg_advisory_unlock($1)", e.key).Scan(&unlocked); err != nil {
		e.log.Warn("advisory unlock failed; the lock is released when the session ends",
			slog.String("error", err.Error()))
		return
	}
	if !unlocked {
		e.log.Warn("advisory unlock reported the lock was not held; the lock is released when the session ends")
	}
}
