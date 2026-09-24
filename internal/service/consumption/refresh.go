package consumption

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/google/uuid"
)

// Enqueuer re-enqueues consumption.refresh (R100(4)): when RefreshConsumption
// cannot acquire the global lock because another refresh already holds it,
// the handler re-enqueues the SAME payload through this seam instead of
// consuming its own retry budget. It is satisfied structurally by
// worker/wiring.go's consumptionRefreshEnqueuer — the very same adapter
// internal/ingest.ConsumptionRefreshEnqueuer uses to enqueue the FIRST
// consumption.refresh from a fetch run. Both interfaces share this one
// method shape by design (this package cannot import internal/ingest — see
// R72's layering — so it declares its own copy rather than reusing
// ingest's), which is what lets one adapter type satisfy both with no glue.
type Enqueuer interface {
	EnqueueConsumptionRefresh(ctx context.Context, p job.ConsumptionRefreshPayload) error
}

// RefreshDeps are Refresher's dependencies.
type RefreshDeps struct {
	Aggregates store.AdminAggregateRepository
	// Locker guards the single global consumptionRefreshLockKey (R100(4),
	// replacing R71's per-view keys): every refresh — of every view, for
	// every window — serialises against this ONE lease, so at most one
	// consumption.refresh handler is ever doing platform-wide work at a
	// time.
	Locker lock.Locker
	// Enqueuer re-enqueues on lock contention (R100(4)). See Enqueuer.
	Enqueuer Enqueuer
	// Clock resolves "now" for the policy-horizon clip (R100(3)): a view is
	// only refreshed for the part of its window older than
	// now-startOffset, since the view's own scheduled policy already
	// covers the rest.
	Clock clock.Clock
	// LockTTL bounds the single global lease: there is no lease renewal
	// (see Refresher's doc comment below), so a lease that expires
	// mid-refresh can at worst let a second, idempotent refresh of the
	// same window start concurrently — it can never make Refresh itself
	// fail. Default 30 minutes (R100(5)), comfortably longer than a
	// platform-wide yearly recompute.
	LockTTL  time.Duration
	Location *time.Location // Europe/Istanbul
	Log      *slog.Logger
}

// Refresher implements job.Refresher: it is consumption.refresh's handler
// (R71, R72, amended by R100).
//
// R100(3)/(4): for each of store.ConsumptionViews(), finest first (hourly,
// daily, monthly, yearly), the payload's [From, To) window is expanded to
// whole buckets of that view's own level, then clipped to the part OLDER
// than that view's own policy start_offset — the scheduled
// add_continuous_aggregate_policy job already keeps the rest live, so
// refreshing it manually would be pure, platform-wide, redundant load.
// A view with nothing older than its own horizon
// is skipped entirely. The whole call is serialised by ONE global lock
// (consumptionRefreshLockKey), not a lock per view: on contention
// (lock.ErrNotAcquired) the handler re-enqueues the SAME payload through
// RefreshDeps.Enqueuer and returns nil, consuming no retry budget and never
// risking archival: a per-view-lock design would let
// a burst of distinct windows exhaust retries and archive, permanently
// blocking that window's id.
//
// R72: this is platform work, not tenant work — refresh_continuous_aggregate
// takes a time range only and necessarily covers every tenant's buckets in
// it, so RefreshConsumption takes no store.Scope; a scoped signature here
// would be a lie about what the underlying call can restrict.
//
// There is no lease renewal: platform/lock.Locker has no renew operation,
// only Acquire and Release. LockTTL is chosen (by the caller, at
// construction) to exceed the longest platform-wide refresh, so this is not
// expected to matter in practice; if a lease DOES expire before a refresh
// finishes, the worst case is a second call acquiring the same key and
// running the same, idempotent refresh_continuous_aggregate concurrently —
// never a correctness failure, since refreshing the same window twice
// produces the same result both times. Release always runs against a
// context.WithoutCancel derivative of ctx (R100(5)): a cancelled or
// timed-out ctx must never leak the lease for the full TTL just because the
// caller gave up.
type Refresher struct {
	aggregates store.AdminAggregateRepository
	locker     lock.Locker
	enqueuer   Enqueuer
	clock      clock.Clock
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
	if d.Enqueuer == nil {
		return nil, errors.New("consumption: RefreshDeps.Enqueuer is required")
	}
	if d.Clock == nil {
		return nil, errors.New("consumption: RefreshDeps.Clock is required")
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
		enqueuer:   d.Enqueuer,
		clock:      d.Clock,
		lockTTL:    d.LockTTL,
		loc:        d.Location,
		log:        d.Log,
	}, nil
}

// consumptionRefreshLockKey is R100(4)'s single global lock key. It
// replaces R71's per-view key ("consumption.refresh:" + view) so a burst of
// refreshes touching every view serialises against ONE lease instead of
// racing four independent ones — a per-view-lock design lets lock
// contention exhaust an asynq task's retry budget and archive it,
// permanently blocking that window.
const consumptionRefreshLockKey = "consumption.refresh"

// Policy start_offset values, one per consumption continuous aggregate
// (R100(3)). These MUST match
// internal/store/postgres/migrations/00005_continuous_aggregates.sql's
// add_continuous_aggregate_policy calls exactly:
//
//	consumption_hourly:  start_offset => interval '30 days'
//	consumption_daily:   start_offset => interval '90 days'
//	consumption_monthly: start_offset => interval '1 year'
//	consumption_yearly:  start_offset => interval '5 years'
//
// TestPolicyStartOffsetsMatchMigration parses that file and fails the
// moment the two drift.
const (
	hourlyPolicyStartOffset  = 30 * 24 * time.Hour
	dailyPolicyStartOffset   = 90 * 24 * time.Hour
	monthlyPolicyStartOffset = 365 * 24 * time.Hour
	yearlyPolicyStartOffset  = 5 * 365 * 24 * time.Hour
)

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

// policyStartOffsetForView maps a store.AggregateView to its own
// add_continuous_aggregate_policy start_offset (R100(3)) — see the policy
// constants' doc comment for the exact migration 00005 values this must
// track.
func policyStartOffsetForView(view store.AggregateView) (time.Duration, error) {
	switch view {
	case store.ViewConsumptionHourly:
		return hourlyPolicyStartOffset, nil
	case store.ViewConsumptionDaily:
		return dailyPolicyStartOffset, nil
	case store.ViewConsumptionMonthly:
		return monthlyPolicyStartOffset, nil
	case store.ViewConsumptionYearly:
		return yearlyPolicyStartOffset, nil
	default:
		return 0, fmt.Errorf("consumption: no policy start_offset mapped for aggregate view %q", view)
	}
}

// clipToPolicyHorizon narrows rng — already expanded to whole buckets of
// level by RefreshConsumption — to the part OLDER than the horizon
// now.Add(-startOffset), floored down to that horizon's own bucket start
// (R100(3)'s "clipped to whole buckets"). The scheduled
// add_continuous_aggregate_policy job already keeps everything at or after
// the horizon live, so refreshing that part again would be redundant,
// platform-wide work. ok is false when rng has nothing older than the
// horizon at all, meaning the view needs no manual refresh.
func clipToPolicyHorizon(level energy.Level, rng store.TimeRange, startOffset time.Duration, now time.Time, loc *time.Location) (store.TimeRange, bool) {
	horizon := energy.Bucket(level, now.Add(-startOffset), loc).From
	if !rng.From.Before(horizon) {
		return store.TimeRange{}, false
	}
	to := rng.To
	if to.After(horizon) {
		to = horizon
	}
	return store.TimeRange{From: rng.From, To: to}, true
}

// validPayload reports whether p is well-formed enough to refresh: a real
// company, both times set, and From strictly before To.
func validPayload(p job.ConsumptionRefreshPayload) bool {
	return p.CompanyID != uuid.Nil && !p.From.IsZero() && !p.To.IsZero() && p.From.Before(p.To)
}

// RefreshConsumption implements job.Refresher. An invalid payload is
// job.SkipRetry'd — no retry will ever make a nil company or a reversed
// window valid.
//
// Lock contention (lock.ErrNotAcquired) is NOT reported as a retryable
// error: R100(4) re-enqueues the same payload (through deps.Enqueuer, with
// its own fresh debounce slot and ProcessIn delay — see
// job.NewConsumptionRefreshTask) and returns nil, so this task completes
// successfully having done no work, and the retry budget stays untouched
// for failures that actually need it.
//
// Any other failure — the lock acquisition itself erroring for a reason
// other than contention, or a view's own refresh failing — aborts the
// remaining views and is returned as a plain (retryable) error, so asynq's
// ordinary retry/backoff applies.
func (r *Refresher) RefreshConsumption(ctx context.Context, p job.ConsumptionRefreshPayload) error {
	if !validPayload(p) {
		return job.SkipRetry(fmt.Errorf("consumption: invalid refresh payload (company=%s from=%s to=%s)",
			p.CompanyID, p.From, p.To))
	}

	lease, err := r.locker.Acquire(ctx, consumptionRefreshLockKey, r.lockTTL)
	if err != nil {
		if errors.Is(err, lock.ErrNotAcquired) {
			if reErr := r.enqueuer.EnqueueConsumptionRefresh(ctx, p); reErr != nil {
				return fmt.Errorf("consumption: re-enqueue after lock contention: %w", reErr)
			}
			return nil
		}
		return fmt.Errorf("consumption: acquire lock: %w", err)
	}
	defer func() {
		// R100(5): released with an UNCANCELLED context — ctx may already
		// be done (a cancelled or timed-out caller), and a Release that
		// inherited that cancellation would fail before it ever reaches
		// Redis, leaking the lease for the rest of lockTTL.
		releaseCtx := context.WithoutCancel(ctx)
		if releaseErr := lease.Release(releaseCtx); releaseErr != nil {
			r.log.Warn("consumption.refresh: release lock failed", "error", releaseErr)
		}
	}()

	now := r.clock.Now()
	for _, view := range store.ConsumptionViews() {
		level, err := levelForView(view)
		if err != nil {
			return err
		}
		startOffset, err := policyStartOffsetForView(view)
		if err != nil {
			return err
		}

		from := energy.Bucket(level, p.From, r.loc).From
		to := energy.Bucket(level, p.To.Add(-time.Nanosecond), r.loc).To
		rng := store.TimeRange{From: from, To: to}

		clipped, ok := clipToPolicyHorizon(level, rng, startOffset, now, r.loc)
		if !ok {
			continue
		}

		if err := r.aggregates.Refresh(ctx, view, clipped); err != nil {
			return fmt.Errorf("consumption: refresh %s: %w", view, err)
		}
	}
	return nil
}
