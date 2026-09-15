package generation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// initialAnchorLookback is how far before a hook call's `from` to look for
// the first-ever load_profile row when no generation_anchors row exists yet
// (algorithm step 2).
const initialAnchorLookback = 7 * 24 * time.Hour

// initialAnchorOffset is subtracted from the first-ever row's Ts to produce
// that row's anchor timestamp, so the very first row itself is included
// (strictly after the anchor) when recomputeForward reads forward from
// anchorTs+1ns.
const initialAnchorOffset = 15 * time.Minute

// rangeSliceWindow bounds every forward Range read to at most this span
// (04-data-model.md §14: no unbounded hypertable read), regardless of how
// far behind the anchor is.
const rangeSliceWindow = 7 * 24 * time.Hour

// writeBatchSize caps how many recomputed rows accumulate before a
// BulkInsert flush (brief: "batches of ≤ 5000").
const writeBatchSize = 5000

// generationLockKeyPrefix names the per-analyzer distributed lease R52
// serialises every AfterPersist/Recompute call behind: key
// "generation:<analyzerID>".
const generationLockKeyPrefix = "generation:"

// generationLockTTL bounds the per-analyzer generation lease (R52). It must
// stay above the recompute bound: a full forward (or backward) recompute
// runs inside the same integration.fetch_readings asynq task, whose own
// Timeout is 10 minutes (job.NewFetchReadingsTask) — this TTL is set
// comfortably above that so the lease never expires out from under a
// legitimately still-running recompute, which would let a second caller
// acquire the same key while the first is still mid-write.
const generationLockTTL = 20 * time.Minute

// releaseLeaseTimeout bounds Release's own detached context below — the
// same folded-minor pattern internal/credentials/service.go's releaseLease
// uses (context.WithoutCancel, not the caller's ctx, which may already be
// cancelled or past its deadline by the time Release runs: R52 "released
// with a non-cancelled ctx").
const releaseLeaseTimeout = 5 * time.Second

// Accumulator reconciles PM5340's interval generation into active_export.
// It implements ingest.PostPersistHook for provider pm5340.
//
// Controller note (mid-task-11, Task 10 review fix round): AfterPersist's
// `from`/`to` will change from the chunk window to the exact persisted
// [minTs, maxTs] of the page just written — no signature change. Accumulator
// does not need to know which one it is receiving: see AfterPersist's doc.
type Accumulator struct {
	readings        store.ReadingRepository
	anchors         store.GenerationRepository
	clock           clock.Clock
	locker          lock.Locker
	futureTolerance time.Duration
}

// New builds an Accumulator. locker is the distributed lease AfterPersist
// and Recompute serialise every call behind, per analyzer (R52) — the
// worker wires the shared Redis lock; tests wire lock.NewMemory.
// futureTolerance bounds how far past "now" a forward recompute reaches
// (M12); zero defaults to ingest.DefaultFutureTolerance, the same default
// ingest.Service itself falls back to when its own Options.FutureTolerance
// is left zero — this package has never had its own separate default.
func New(readings store.ReadingRepository, anchors store.GenerationRepository, c clock.Clock, locker lock.Locker, futureTolerance time.Duration) *Accumulator {
	if futureTolerance <= 0 {
		futureTolerance = ingest.DefaultFutureTolerance
	}
	return &Accumulator{readings: readings, anchors: anchors, clock: c, locker: locker, futureTolerance: futureTolerance}
}

// Locker exposes the Accumulator's configured lock.Locker for tests that
// must prove which one a running worker wired in (M12/I3's graph test:
// TestBuildGraphGenerationHookUsesRedisLockAndConfiguredFutureTolerance) —
// production code never calls this.
func (a *Accumulator) Locker() lock.Locker { return a.locker }

// FutureTolerance exposes the Accumulator's configured recompute upper
// bound for the same graph-test purpose as Locker.
func (a *Accumulator) FutureTolerance() time.Duration { return a.futureTolerance }

var _ ingest.PostPersistHook = (*Accumulator)(nil)

// generationLockKey is R52's fixed key shape for one analyzer's lease.
func generationLockKey(analyzerID uuid.UUID) string {
	return generationLockKeyPrefix + analyzerID.String()
}

// withAnalyzerLock acquires the per-analyzer generation lease, runs fn while
// holding it, and always releases it afterward through a detached
// (WithoutCancel) context bounded by releaseLeaseTimeout — R52's "released
// with a non-cancelled ctx": Release must still reach Redis even when ctx
// itself is already done (a deadline, or the caller giving up), otherwise a
// lease can outlive its holder for its full TTL instead of being freed
// promptly for the next caller.
func (a *Accumulator) withAnalyzerLock(ctx context.Context, analyzerID uuid.UUID, fn func() error) error {
	lease, err := a.locker.Acquire(ctx, generationLockKey(analyzerID), generationLockTTL)
	if err != nil {
		return fmt.Errorf("generation: acquire lock for analyzer %s: %w", analyzerID, err)
	}
	defer func() {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseLeaseTimeout)
		defer cancel()
		_ = lease.Release(rctx)
	}()
	return fn()
}

// AfterPersist recomputes active_export for every load_profile row with
// ts >= from, up to now, anchored on the last derived row before from (or on
// the anchor itself when there is none). Only kind == load_profile and
// provider pm5340 are handled; every other call is a no-op.
//
// This method does not depend on exactly what `from`/`to` name — the chunk
// window or the exact persisted [minTs, maxTs] of the page just written (the
// latter is Task 10's post-review contract; see the package doc note below)
// — and never assumes a row exists at or immediately before `from`. `from`
// is only ever used as a LOWER bound: the base is the last already-derived
// row strictly BEFORE from (read fresh from the store, not assumed from the
// batch), or the anchor when there is none. `to` is only used to bound the
// one-time initial-anchor lookback window. The forward recomputation itself
// always runs to `now`, never to `to` (see recomputeForward) — so all
// correctness requires is `from` at-or-before, and `to` at-or-after, every
// row actually persisted in this call, which both the chunk window and the
// exact persisted range guarantee by construction.
//
// R52: the whole call — anchor read (and create), any pre-anchor backfill
// derivation, and the forward recompute's own reads and writes — runs under
// one per-analyzer lease (withAnalyzerLock), so two AfterPersist calls for
// the same analyzer can never interleave their own read-then-write cycles.
// Without it, a call that reads its base/rows while another call's own
// BulkInsert is still in flight would compute from a stale snapshot and
// then overwrite the other call's already-correct, more-complete result
// with its own stale one (TestAfterPersistIsSerialisedPerAnalyzer proves
// this both ways: PASS with the lock, FAIL with it removed).
func (a *Accumulator) AfterPersist(ctx context.Context, s store.Scope, an model.Analyzer, kind model.ReadingKind, from, to time.Time) error {
	if kind != model.ReadingKindLoadProfile || an.Provider != model.IntegrationProviderPM5340 {
		return nil
	}
	return a.withAnalyzerLock(ctx, an.ID, func() error {
		return a.afterPersistLocked(ctx, s, an.ID, from, to)
	})
}

// afterPersistLocked is AfterPersist's body, run under the per-analyzer
// lease withAnalyzerLock holds for the duration of this call.
func (a *Accumulator) afterPersistLocked(ctx context.Context, s store.Scope, analyzerID uuid.UUID, from, to time.Time) error {
	now := a.clock.Now()

	anchor, err := a.anchors.Anchor(ctx, s, analyzerID)
	if errors.Is(err, store.ErrNotFound) {
		anchor, err = a.createInitialAnchor(ctx, s, analyzerID, from, to)
	}
	if err != nil {
		return err
	}

	// R52, rows OLDER than the anchor: this call's `from` is only ever a
	// LOWER bound for what was actually persisted (see the doc above), so
	// `from <= anchor.AnchorTs` means the just-persisted page (or an
	// already-stored row from an earlier call) may reach at-or-before the
	// anchor's own timestamp — a backfill that landed before whatever the
	// anchor currently accounts for. `anchor.AnchorTs.Before(from)` being
	// false is exactly the complement of the normal forward-continuation
	// branch below, so the two are mutually exclusive by construction.
	if !anchor.AnchorTs.Before(from) {
		anchor, err = a.deriveBeforeAnchor(ctx, s, analyzerID, anchor, from)
		if err != nil {
			return err
		}
	}

	baseVal, baseTs := anchor.ActiveExport, anchor.AnchorTs
	if anchor.AnchorTs.Before(from) {
		// TimeRange{anchor.AnchorTs, from} is well-formed exactly because of
		// the guard above (Valid() requires From strictly before To).
		latest, lerr := a.readings.Latest(ctx, s, analyzerID, store.TimeRange{From: anchor.AnchorTs, To: from}, model.ReadingKindLoadProfile)
		if lerr != nil {
			return lerr
		}
		if latest != nil && latest.ActiveExport != nil {
			baseVal, baseTs = *latest.ActiveExport, latest.Ts
		}
	}

	return a.recomputeForward(ctx, s, analyzerID, baseVal, baseTs, now)
}

// deriveBeforeAnchor implements R52's two pre-anchor-backfill rules,
// dispatching on the CURRENT anchor's Source (read fresh, inside the lock,
// never assumed from a caller's stale copy):
//
//   - Source == "initial" (the synthetic value-0 anchor createInitialAnchor
//     stamps on first-ever contact — never a real device reading): moving it
//     back is safe and lossless, so the anchor itself moves to
//     (earliest known row at-or-before the OLD anchor − 15m), value 0, and
//     every row after the NEW anchor is fully re-derived by the caller's own
//     forward recompute (idempotent — see recomputeForward's doc — so
//     re-deriving rows this call didn't itself touch is correct, not
//     wasted-but-risky work). "Cumulative values shift uniformly" (R52):
//     every row after the old anchor shifts by exactly the newly-included
//     rows' interval sum, which is what a fresh forward recompute from the
//     earlier base naturally produces.
//   - Any other Source ("operator" or "migration" — a REAL meter-set
//     cumulative value at a real instant): the anchor must never move off a
//     value an operator or a migration explicitly asserted. Instead, rows
//     at-or-before the anchor's own timestamp are derived BACKWARD from it
//     (deriveOlderRows / AccumulateBackward) and written directly; the
//     anchor, and the caller's subsequent forward base selection, are both
//     left untouched.
//
// If no row is actually found at-or-before the anchor (the common case: a
// `from` that merely reaches back to the anchor's own instant with nothing
// older actually persisted there), this is a no-op and the original anchor
// is returned unchanged.
func (a *Accumulator) deriveBeforeAnchor(ctx context.Context, s store.Scope, analyzerID uuid.UUID, anchor model.GenerationAnchor, from time.Time) (model.GenerationAnchor, error) {
	// Bounded per §14: the older-rows probe never reads past the anchor's
	// own timestamp, and never before `from` — the same discipline
	// createInitialAnchor's own lookback-bounded probe already follows.
	older, err := a.readings.Range(ctx, s, analyzerID, store.TimeRange{From: from, To: anchor.AnchorTs.Add(time.Microsecond)}, model.ReadingKindLoadProfile)
	if err != nil {
		return model.GenerationAnchor{}, err
	}
	if len(older) == 0 {
		return anchor, nil
	}

	if anchor.Source == "initial" {
		return a.moveAnchorBack(ctx, s, analyzerID, older[0].Ts)
	}

	if err := a.deriveOlderRows(ctx, s, analyzerID, anchor, from); err != nil {
		return model.GenerationAnchor{}, err
	}
	return anchor, nil
}

// moveAnchorBack implements the "initial" half of R52: earliestTs is the
// earliest known row at-or-before the current (synthetic, value-0) anchor —
// the new anchor sits initialAnchorOffset before it, value 0, Source stays
// "initial", the same shape createInitialAnchor itself stamps.
func (a *Accumulator) moveAnchorBack(ctx context.Context, s store.Scope, analyzerID uuid.UUID, earliestTs time.Time) (model.GenerationAnchor, error) {
	anchor := model.GenerationAnchor{
		AnalyzerID:   analyzerID,
		AnchorTs:     earliestTs.Add(-initialAnchorOffset),
		ActiveExport: decimal.Zero,
		Source:       "initial",
	}
	if err := a.anchors.SetAnchor(ctx, s, anchor); err != nil {
		return model.GenerationAnchor{}, err
	}
	return anchor, nil
}

// deriveOlderRows implements the "operator"/"migration" half of R52: every
// row at-or-before anchor.AnchorTs, down to from, gets active_export derived
// BACKWARD via AccumulateBackward — anchor.ActiveExport minus the sum of
// every row's interval strictly after it, up to and including the anchor
// timestamp — without moving the anchor. The read walks backward from the
// anchor in rangeSliceWindow-bounded slices (§14, the same bound
// recomputeForward applies forward), nearest-to-anchor chunk first, so
// AccumulateBackward's sumAfter threads correctly across chunk boundaries;
// writes flush in batches of at most writeBatchSize, mirroring
// recomputeForward's own flush.
func (a *Accumulator) deriveOlderRows(ctx context.Context, s store.Scope, analyzerID uuid.UUID, anchor model.GenerationAnchor, from time.Time) error {
	sumAfter := decimal.Zero
	batch := make([]model.MeterReading, 0, writeBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if _, _, err := a.readings.BulkInsert(ctx, s, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	windowTo := anchor.AnchorTs.Add(time.Microsecond)
	for from.Before(windowTo) {
		windowFrom := windowTo.Add(-rangeSliceWindow)
		if windowFrom.Before(from) {
			windowFrom = from
		}

		rows, err := a.readings.Range(ctx, s, analyzerID, store.TimeRange{From: windowFrom, To: windowTo}, model.ReadingKindLoadProfile)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			var vals []decimal.Decimal
			vals, sumAfter = AccumulateBackward(anchor.ActiveExport, sumAfter, rows)
			for i := range rows {
				v := vals[i]
				rows[i].ActiveExport = &v
				batch = append(batch, rows[i])
				if len(batch) >= writeBatchSize {
					if err := flush(); err != nil {
						return err
					}
				}
			}
		}

		windowTo = windowFrom
	}

	return flush()
}

// Recompute rebuilds the whole series from the persisted anchor — the
// "recomputable deterministically if history is corrected" requirement.
// Unlike AfterPersist it never uses an already-derived row as a shortcut
// base: it always starts at the anchor itself, so a correction to any
// already-persisted interval value (or to the anchor) is fully propagated,
// not partially shadowed by a stale derived row that sits between the
// correction and now. It returns ErrNotFound (via the anchor lookup) when
// the analyzer has no anchor yet, or is not visible to s.
//
// R52: also serialised behind the same per-analyzer lease AfterPersist
// uses, so an operator-triggered Recompute can never interleave with a
// concurrent ingestion-driven AfterPersist for the same analyzer.
func (a *Accumulator) Recompute(ctx context.Context, s store.Scope, analyzerID uuid.UUID) error {
	return a.withAnalyzerLock(ctx, analyzerID, func() error {
		anchor, err := a.anchors.Anchor(ctx, s, analyzerID)
		if err != nil {
			return err
		}
		now := a.clock.Now()
		return a.recomputeForward(ctx, s, analyzerID, anchor.ActiveExport, anchor.AnchorTs, now)
	})
}

// createInitialAnchor implements algorithm step 2: on ErrNotFound, anchor at
// 15 minutes before the first load_profile row found in [from-7d, to]
// (inclusive of to — Range is half-open, so the upper bound is nudged by one
// microsecond: Postgres timestamptz is microsecond-precision, and pgx's wire
// encoding silently truncates a sub-microsecond offset away, which would
// leave the bound numerically equal to `to` at the database and exclude a
// row exactly at `to` instead of including it).
func (a *Accumulator) createInitialAnchor(ctx context.Context, s store.Scope, analyzerID uuid.UUID, from, to time.Time) (model.GenerationAnchor, error) {
	lookback := from.Add(-initialAnchorLookback)
	rows, err := a.readings.Range(ctx, s, analyzerID, store.TimeRange{From: lookback, To: to.Add(time.Microsecond)}, model.ReadingKindLoadProfile)
	if err != nil {
		return model.GenerationAnchor{}, err
	}
	if len(rows) == 0 {
		return model.GenerationAnchor{}, fmt.Errorf(
			"generation: no load_profile rows in [%s, %s] to anchor analyzer %s",
			lookback.Format(time.RFC3339), to.Format(time.RFC3339), analyzerID)
	}

	first := rows[0]
	anchor := model.GenerationAnchor{
		AnalyzerID:   analyzerID,
		AnchorTs:     first.Ts.Add(-initialAnchorOffset),
		ActiveExport: decimal.Zero,
		Source:       "initial",
	}
	if err := a.anchors.SetAnchor(ctx, s, anchor); err != nil {
		return model.GenerationAnchor{}, err
	}
	return anchor, nil
}

// recomputeForward reads load_profile rows strictly after baseTs, up to
// now+a.futureTolerance (M12: the SAME configured tolerance
// ingest.Validate enforces on the way in via cfg.Ingest.FutureTolerance,
// plumbed through New — never the package's own DefaultFutureTolerance
// constant, which would silently diverge from an operator's configured
// value; a reading this hook will ever be asked to recompute cannot be
// stamped further into the future than ingest already accepted with that
// SAME bound), in bounded rangeSliceWindow slices, computes each row's
// cumulative active_export with
// Accumulate seeded at baseVal, and writes every row back through
// BulkInsert in batches of at most writeBatchSize. It is idempotent: run
// twice over the same data with the same base, it produces byte-identical
// active_export values, because Accumulate is a pure function of
// (base, rows) and rows come back in the same Ts-ascending order every time.
//
// windowFrom is baseTs plus one MICROSECOND, not one nanosecond: Postgres
// timestamptz is microsecond-precision and pgx's wire encoding silently
// truncates anything finer, so a +1ns offset survives only in Go memory and
// arrives at the database numerically equal to baseTs — which would
// re-include the base row itself in the very first Range read and double
// -count its own IntervalGenerationKwh. This was caught by
// TestGenerationAccumulatesAcrossAGap during implementation (see the task
// report's mutation-proof section) before it reached the mutation-proof
// step: the base row's own interval value was being added a second time.
func (a *Accumulator) recomputeForward(ctx context.Context, s store.Scope, analyzerID uuid.UUID, baseVal decimal.Decimal, baseTs, now time.Time) error {
	upper := now.Add(a.futureTolerance)
	windowFrom := baseTs.Add(time.Microsecond)
	if !windowFrom.Before(upper) {
		return nil
	}

	cur := baseVal
	batch := make([]model.MeterReading, 0, writeBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if _, _, err := a.readings.BulkInsert(ctx, s, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	for windowFrom.Before(upper) {
		windowTo := windowFrom.Add(rangeSliceWindow)
		if windowTo.After(upper) {
			windowTo = upper
		}

		rows, err := a.readings.Range(ctx, s, analyzerID, store.TimeRange{From: windowFrom, To: windowTo}, model.ReadingKindLoadProfile)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			cums := Accumulate(cur, rows)
			cur = cums[len(cums)-1]
			for i := range rows {
				v := cums[i]
				rows[i].ActiveExport = &v
				batch = append(batch, rows[i])
				if len(batch) >= writeBatchSize {
					if err := flush(); err != nil {
						return err
					}
				}
			}
		}

		windowFrom = windowTo
	}

	return flush()
}
