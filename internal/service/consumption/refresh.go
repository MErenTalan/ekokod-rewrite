// Package consumption owns the consumption.refresh handler (Task 11a): the
// platform-wide, per-window, per-view-locked refresh of TimescaleDB's
// consumption continuous aggregates.
package consumption

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/google/uuid"
)

// RefreshDeps are Refresher's dependencies.
type RefreshDeps struct {
	Aggregates store.AdminAggregateRepository
	Locker     lock.Locker
	// LockTTL must exceed the longest single-view refresh: there is no
	// lease renewal (see Refresher's doc comment below), so a lease that
	// expires mid-refresh can at worst let a second, idempotent refresh of
	// the same view start concurrently — it can never make Refresh itself
	// fail.
	LockTTL  time.Duration
	Location *time.Location // Europe/Istanbul
	Log      *slog.Logger
}

// Refresher implements job.Refresher: it is consumption.refresh's handler
// (R71, R72).
//
// R71: for each of store.ConsumptionViews(), finest first (hourly, daily,
// monthly, yearly), the payload's [From, To) window is expanded to whole
// buckets of that view's own level and refreshed under a lock held for
// exactly that view, for exactly the duration of its own refresh. Finest
// first matters because a coarser view's own story about a window should
// never be seen to lag a finer view that already reflects it.
//
// R72: this is platform work, not tenant work — refresh_continuous_aggregate
// takes a time range only and necessarily covers every tenant's buckets in
// it, so RefreshConsumption takes no store.Scope; a scoped signature here
// would be a lie about what the underlying call can restrict.
//
// There is no lease renewal: platform/lock.Locker has no renew operation,
// only Acquire and Release. LockTTL is chosen (by the caller, at
// construction) to exceed the longest single-view refresh, so this is not
// expected to matter in practice; if a lease DOES expire before a refresh
// finishes, the worst case is a second call acquiring the same key and
// running the same, idempotent refresh_continuous_aggregate concurrently —
// never a correctness failure, since refreshing the same window twice
// produces the same result both times.
type Refresher struct {
	aggregates store.AdminAggregateRepository
	locker     lock.Locker
	lockTTL    time.Duration
	loc        *time.Location
	log        *slog.Logger
}

var _ job.Refresher = (*Refresher)(nil)

// NewRefresher validates d and returns a Refresher.
func NewRefresher(d RefreshDeps) (*Refresher, error) {
	if d.Aggregates == nil {
		return nil, errors.New("consumption: RefreshDeps.Aggregates is required")
	}
	if d.Locker == nil {
		return nil, errors.New("consumption: RefreshDeps.Locker is required")
	}
	if d.LockTTL <= 0 {
		return nil, errors.New("consumption: RefreshDeps.LockTTL must be positive")
	}
	if d.Location == nil {
		return nil, errors.New("consumption: RefreshDeps.Location is required")
	}
	if d.Log == nil {
		return nil, errors.New("consumption: RefreshDeps.Log is required")
	}
	return &Refresher{
		aggregates: d.Aggregates,
		locker:     d.Locker,
		lockTTL:    d.LockTTL,
		loc:        d.Location,
		log:        d.Log,
	}, nil
}

// levelForView maps a store.AggregateView to the energy.Level whose bucket
// boundaries expand the refresh window for that view. An unmapped view is a
// programmer error (store.ConsumptionViews() and this switch have drifted)
// and is reported as such rather than silently refreshing the wrong range.
func levelForView(view store.AggregateView) (energy.Level, error) {
	switch view {
	case store.ViewConsumptionHourly:
		return energy.Hourly, nil
	case store.ViewConsumptionDaily:
		return energy.Daily, nil
	case store.ViewConsumptionMonthly:
		return energy.Monthly, nil
	case store.ViewConsumptionYearly:
		return energy.Yearly, nil
	default:
		return "", fmt.Errorf("consumption: no level mapped for aggregate view %q", view)
	}
}

// validPayload reports whether p is well-formed enough to refresh: a real
// company, both times set, and From strictly before To.
func validPayload(p job.ConsumptionRefreshPayload) bool {
	return p.CompanyID != uuid.Nil && !p.From.IsZero() && !p.To.IsZero() && p.From.Before(p.To)
}

// RefreshConsumption implements job.Refresher. An invalid payload is
// job.SkipRetry'd — no retry will ever make a nil company or a reversed
// window valid. Any other failure — a lock that could not be acquired, or
// the refresh itself failing — aborts the remaining views and is returned
// as a plain (retryable) error, so asynq's ordinary retry/backoff applies.
func (r *Refresher) RefreshConsumption(ctx context.Context, p job.ConsumptionRefreshPayload) error {
	if !validPayload(p) {
		return job.SkipRetry(fmt.Errorf("consumption: invalid refresh payload (company=%s from=%s to=%s)",
			p.CompanyID, p.From, p.To))
	}

	for _, view := range store.ConsumptionViews() {
		level, err := levelForView(view)
		if err != nil {
			return err
		}

		from := energy.Bucket(level, p.From, r.loc).From
		to := energy.Bucket(level, p.To.Add(-time.Nanosecond), r.loc).To
		rng := store.TimeRange{From: from, To: to}

		if err := r.refreshView(ctx, view, rng); err != nil {
			return err
		}
	}
	return nil
}

// refreshView acquires the per-view lock, refreshes rng, and releases the
// lease on every path (the defer runs whether Refresh succeeds, fails, or
// the lock was never even reached because Acquire itself failed — in which
// case there is no lease to release, hence the nil check).
func (r *Refresher) refreshView(ctx context.Context, view store.AggregateView, rng store.TimeRange) error {
	key := "consumption.refresh:" + string(view)
	lease, err := r.locker.Acquire(ctx, key, r.lockTTL)
	if err != nil {
		if errors.Is(err, lock.ErrNotAcquired) {
			r.log.Warn("consumption.refresh: lock not acquired", "view", view, "error", err)
		}
		return fmt.Errorf("consumption: acquire lock for %s: %w", view, err)
	}
	defer func() {
		if releaseErr := lease.Release(ctx); releaseErr != nil {
			r.log.Warn("consumption.refresh: release lock failed", "view", view, "error", releaseErr)
		}
	}()

	if err := r.aggregates.Refresh(ctx, view, rng); err != nil {
		return fmt.Errorf("consumption: refresh %s: %w", view, err)
	}
	return nil
}
