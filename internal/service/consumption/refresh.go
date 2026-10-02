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
// (R71, R72, R100, amended by R500).
//
// For each of store.ConsumptionViews(), finest first, the payload's
// [From, To) window is expanded to whole buckets of that view's own level and
// refreshed in full. R500: the window is NOT clipped to the view's policy
// horizon any more — rows written behind a view's watermark stay invisible
// until a refresh covers them, and the scheduled policy only comes round
// every 30-60 minutes, so a fetch's own data was missing from every chart
// until then. A refresh recomputes only the buckets a write invalidated.
//
// The whole call is serialised by ONE global lock (R100(4)): Acquire waits
// for the holder, and a wait cut short by ctx re-enqueues the same payload.
//
// R72: this is platform work, not tenant work — refresh_continuous_aggregate
// takes a time range only and necessarily covers every tenant's buckets in
// it, so RefreshConsumption takes no store.Scope.
//
// There is no lease renewal: LockTTL exceeds the longest refresh, and a
// second concurrent refresh of one window is idempotent anyway. Release runs
// against a context.WithoutCancel derivative of ctx (R100(5)).
type Refresher struct {
	aggregates store.AdminAggregateRepository
	locker     lock.Locker
	enqueuer   Enqueuer
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

// levelForView maps a store.AggregateView to the energy.Level whose bucket
// boundaries expand the refresh window for that view.
func levelForView(view store.AggregateView) (energy.Level, error) {
	switch view {
	case store.ViewConsumptionHourly:
		return energy.Hourly, nil
	case store.ViewConsumptionDaily:
		return energy.Daily, nil
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
// window valid. A lock wait that ctx cut short re-enqueues the same payload
// and returns nil, so it costs no retry budget (R100(4)); any other failure
// is returned as a plain (retryable) error.
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
		// R100(5): released with an UNCANCELLED context, or a cancelled
		// caller leaks the lease for the rest of lockTTL.
		if releaseErr := lease.Release(context.WithoutCancel(ctx)); releaseErr != nil {
			r.log.Warn("consumption.refresh: release lock failed", "error", releaseErr)
		}
	}()

	for _, view := range store.ConsumptionViews() {
		level, err := levelForView(view)
		if err != nil {
			return err
		}
		rng := store.TimeRange{
			From: energy.Bucket(level, p.From, r.loc).From,
			To:   energy.Bucket(level, p.To.Add(-time.Nanosecond), r.loc).To,
		}
		if err := r.aggregates.Refresh(ctx, view, rng); err != nil {
			return fmt.Errorf("consumption: refresh %s: %w", view, err)
		}
	}
	return nil
}
