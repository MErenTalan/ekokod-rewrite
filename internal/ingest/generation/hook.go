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

// Accumulator reconciles PM5340's interval generation into active_export.
// It implements ingest.PostPersistHook for provider pm5340.
//
// Controller note (mid-task-11, Task 10 review fix round): AfterPersist's
// `from`/`to` will change from the chunk window to the exact persisted
// [minTs, maxTs] of the page just written — no signature change. Accumulator
// does not need to know which one it is receiving: see AfterPersist's doc.
type Accumulator struct {
	readings store.ReadingRepository
	anchors  store.GenerationRepository
	clock    clock.Clock
}

// New builds an Accumulator.
func New(readings store.ReadingRepository, anchors store.GenerationRepository, c clock.Clock) *Accumulator {
	return &Accumulator{readings: readings, anchors: anchors, clock: c}
}

var _ ingest.PostPersistHook = (*Accumulator)(nil)

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
func (a *Accumulator) AfterPersist(ctx context.Context, s store.Scope, an model.Analyzer, kind model.ReadingKind, from, to time.Time) error {
	if kind != model.ReadingKindLoadProfile || an.Provider != model.IntegrationProviderPM5340 {
		return nil
	}
	now := a.clock.Now()

	anchor, err := a.anchors.Anchor(ctx, s, an.ID)
	if errors.Is(err, store.ErrNotFound) {
		anchor, err = a.createInitialAnchor(ctx, s, an.ID, from, to)
	}
	if err != nil {
		return err
	}

	baseVal, baseTs := anchor.ActiveExport, anchor.AnchorTs
	if anchor.AnchorTs.Before(from) {
		// TimeRange{anchor.AnchorTs, from} is well-formed exactly because of
		// the guard above (Valid() requires From strictly before To).
		latest, lerr := a.readings.Latest(ctx, s, an.ID, store.TimeRange{From: anchor.AnchorTs, To: from}, model.ReadingKindLoadProfile)
		if lerr != nil {
			return lerr
		}
		if latest != nil && latest.ActiveExport != nil {
			baseVal, baseTs = *latest.ActiveExport, latest.Ts
		}
	}

	return a.recomputeForward(ctx, s, an.ID, baseVal, baseTs, now)
}

// Recompute rebuilds the whole series from the persisted anchor — the
// "recomputable deterministically if history is corrected" requirement.
// Unlike AfterPersist it never uses an already-derived row as a shortcut
// base: it always starts at the anchor itself, so a correction to any
// already-persisted interval value (or to the anchor) is fully propagated,
// not partially shadowed by a stale derived row that sits between the
// correction and now. It returns ErrNotFound (via the anchor lookup) when
// the analyzer has no anchor yet, or is not visible to s.
func (a *Accumulator) Recompute(ctx context.Context, s store.Scope, analyzerID uuid.UUID) error {
	anchor, err := a.anchors.Anchor(ctx, s, analyzerID)
	if err != nil {
		return err
	}
	now := a.clock.Now()
	return a.recomputeForward(ctx, s, analyzerID, anchor.ActiveExport, anchor.AnchorTs, now)
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
// now+FutureTolerance (the same tolerance ingest.Validate enforces on the
// way in — a reading this hook will ever be asked to recompute cannot be
// stamped further into the future than ingest already accepted), in bounded
// rangeSliceWindow slices, computes each row's cumulative active_export with
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
	upper := now.Add(ingest.DefaultFutureTolerance)
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
