package consumption

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// BillingDeps is everything the billing path may reach. It deliberately has
// no analytics repository: the billing path must not be able to read an
// aggregate.
type BillingDeps struct {
	Readings  store.ReadingRepository
	Anomalies store.AnomalyRepository
	Ops       store.OpsRepository
	Clock     clock.Clock
	Log       *slog.Logger
}

// Billing is the exact, hypertable-only consumption path. There is no
// shared Service struct (R61): Billing is constructed alone, and has no
// field of any AnalyticsRepository type to poison in a guard test.
//
// Anomalies, Ops and Clock are unused by Consumption itself (this file):
// they exist on this type because Task 8's ConsumptionAndRecord,
// ListAnomalies and ResolveAnomaly — added later, on this same type — need
// them, and R61 requires ONE Billing type rather than a second one that
// would have to duplicate Consumption.
type Billing struct {
	deps BillingDeps
}

// NewBilling validates deps and returns a Billing.
func NewBilling(d BillingDeps) (*Billing, error) {
	switch {
	case d.Readings == nil:
		return nil, errRequired("BillingDeps.Readings")
	case d.Anomalies == nil:
		return nil, errRequired("BillingDeps.Anomalies")
	case d.Ops == nil:
		return nil, errRequired("BillingDeps.Ops")
	case d.Clock == nil:
		return nil, errRequired("BillingDeps.Clock")
	case d.Log == nil:
		return nil, errRequired("BillingDeps.Log")
	}
	return &Billing{deps: d}, nil
}

// Consumption derives consumption from meter_readings at the true period
// boundaries (02 §3.1, §3.2). It is exact, slower, and the only path that
// may feed an invoice. A suspect period returns nil values and a
// Suspicion; it never returns a number (removed-behaviour 1). Writing the
// anomaly row and the operator message is Task 8's ConsumptionAndRecord
// wrapper, on the same type.
func (b *Billing) Consumption(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error) {
	if err := validateRequest(sc, req); err != nil {
		return nil, err
	}

	window := energy.Window{From: req.Range.From, To: req.Range.To}
	buckets := energy.Buckets(req.Level, window, istanbul)
	if len(buckets) == 0 {
		return nil, nil
	}

	var rows []Row
	for _, analyzerID := range req.AnalyzerIDs {
		analyzerRows, err := b.consumptionForAnalyzer(ctx, sc, analyzerID, req.Level, buckets)
		if err != nil {
			return nil, err
		}
		rows = append(rows, analyzerRows...)
	}
	return rows, nil
}

// consumptionForAnalyzer implements I-17's query shape for one analyzer:
// each kind Billing needs is read ONCE over the whole request, then every
// bucket's boundaries are selected in memory with energy.SelectBoundary.
//
// C1: load_profile itself, reset evidence, and priors all load from the
// EARLIEST look-back of every boundary kind this request may use — never
// just load_profile's own. A billing-kind (Monthly, R63) or daily-kind
// (R95) start boundary can sit further back in time than load_profile's own
// look-back, and a reset row can fall inside that earlier evidence window:
// loading resets only from load_profile's look-back silently drops it, and
// the derived number comes out wrong instead of suspect (the review's P1
// probe: 4150 instead of the true 5150, unflagged).
func (b *Billing) consumptionForAnalyzer(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, level energy.Level, buckets []energy.Window) ([]Row, error) {
	first := buckets[0]
	rangeTo := buckets[len(buckets)-1].To

	lpLookback, err := b.firstBoundaryLookback(ctx, sc, analyzerID, model.ReadingKindLoadProfile, first)
	if err != nil {
		return nil, err
	}
	// R96/Minor M-c: at Daily/Monthly/Yearly, load_profile now carries its
	// own tolerance (LoadProfileBoundaryTolerance) — a load_profile reading
	// older than that relative to first.From could never resolve ANY
	// boundary this request will ever ask for, so clamping the look-back to
	// first.From-tolerance is safe (never drops evidence a real boundary
	// resolution could use) and bounds how far back a very old, permanently
	// stale kind can force this request's Range calls to reach. At Hourly,
	// §3.1's look-back stays deliberately unbounded (Q7) — never clamped.
	if level != energy.Hourly {
		lpLookback = clampLookback(lpLookback, first.From, LoadProfileBoundaryTolerance)
	}
	minLookback := lpLookback

	// Monthly-only: billing-kind readings, for R62/R63's preference.
	useBilling := level == energy.Monthly
	var billingLookback time.Time
	if useBilling {
		billingLookback, err = b.firstBoundaryLookback(ctx, sc, analyzerID, model.ReadingKindBilling, first)
		if err != nil {
			return nil, err
		}
		billingLookback = clampLookback(billingLookback, first.From, BillingSnapshotTolerance)
		if billingLookback.Before(minLookback) {
			minLookback = billingLookback
		}
	}

	// R95/R96: daily is a fallback boundary kind at Daily/Monthly/Yearly,
	// never Hourly. Its own look-back feeds the shared minimum too (C1), on
	// the same reasoning as billing's.
	useDaily := level != energy.Hourly
	var dailyLookback time.Time
	if useDaily {
		dailyLookback, err = b.firstBoundaryLookback(ctx, sc, analyzerID, model.ReadingKindDaily, first)
		if err != nil {
			return nil, err
		}
		dailyLookback = clampLookback(dailyLookback, first.From, DailySnapshotTolerance)
		if dailyLookback.Before(minLookback) {
			minLookback = dailyLookback
		}
	}

	// The single load_profile read serves THREE purposes at once: the
	// load_profile boundary candidate pool, the R55 priors Derive uses for
	// every reset segment regardless of which kind ended up as the window's
	// own boundary, and one of MaxDemand's sources. It is loaded from
	// minLookback — the earliest look-back of any kind this request may use
	// — not merely load_profile's own, so that a reset sitting between an
	// earlier billing/daily look-back and load_profile's own is still
	// covered by a prior reading when Derive needs one.
	//
	// The +1µs, not +1ns: ReadingRange's SQL is `mr.ts < to_ts` (exclusive),
	// and Range's own TimeRange is half-open by contract (repository.go), so
	// the widening step must add the SMALLEST increment Postgres's
	// timestamptz can actually represent to make rangeTo itself count as "at
	// or before". That increment is one MICROSECOND, not one nanosecond:
	// timestamptz stores microsecond precision, so
	// `rangeTo.Add(time.Nanosecond)` round-trips through the wire encoding
	// back to exactly rangeTo — the exact off-by-nothing bug this comment is
	// here so nobody reintroduces (found by
	// TestBillingAndAnalyticsDifferByTheBucketBoundaryStep failing against a
	// real database with 30 where 40 was expected: the reading exactly at
	// rangeTo was silently excluded). Every Range call below that reaches to
	// rangeTo uses the same +1µs widening.
	loadProfileRows, err := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: minLookback, To: rangeTo.Add(time.Microsecond)}, model.ReadingKindLoadProfile)
	if err != nil {
		return nil, err
	}
	loadProfile := toEnergyReadings(loadProfileRows)

	// Reset evidence (I-18): fetched once over the same shared minimum
	// look-back as load_profile, since no bucket's evidence window
	// (start.TS, end.TS] can begin before the earliest boundary reading any
	// kind in this request could select.
	resetRows, err := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: minLookback, To: rangeTo.Add(time.Microsecond)}, model.ReadingKindReset)
	if err != nil {
		return nil, err
	}
	resets := toEnergyReadings(resetRows)

	// R55/I-1: priors are load_profile readings regardless of the window's
	// own boundary kind — a billing- or daily-kind derivation still looks
	// for load_profile priors, because those are the readings dense enough
	// to have one. loadProfile is already sorted ascending by ts (the SQL
	// ReadingRange query's ORDER BY, per R91's precondition) and is passed
	// straight through: it is never merged with another read, so its order
	// is exactly the order the single query returned it in.
	priors := loadProfile

	var billing []energy.Reading
	if useBilling {
		billingRows, billingErr := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: billingLookback, To: rangeTo.Add(time.Microsecond)}, model.ReadingKindBilling)
		if billingErr != nil {
			return nil, billingErr
		}
		billing = toEnergyReadings(billingRows)
	}

	// R95: daily readings, for the Daily/Monthly/Yearly fallback boundary
	// kind. At Hourly, daily is never a boundary candidate, but R65 still
	// allows it as a MaxDemand source, so it is fetched over the plain
	// window instead of a look-back-widened one.
	var daily []energy.Reading
	if useDaily {
		dailyRows, dailyErr := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: dailyLookback, To: rangeTo.Add(time.Microsecond)}, model.ReadingKindDaily)
		if dailyErr != nil {
			return nil, dailyErr
		}
		daily = toEnergyReadings(dailyRows)
	} else {
		dailyRows, dailyErr := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: first.From, To: rangeTo}, model.ReadingKindDaily)
		if dailyErr != nil {
			return nil, dailyErr
		}
		daily = toEnergyReadings(dailyRows)
	}

	// MaxDemand also allows billing-kind readings at every level, not only
	// Monthly (R65): when Monthly already loaded them above, that slice is
	// reused; otherwise they are fetched here, over the plain window, for
	// MaxDemand alone.
	maxDemandBilling := billing
	if !useBilling {
		billingRows, billingErr := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: first.From, To: rangeTo}, model.ReadingKindBilling)
		if billingErr != nil {
			return nil, billingErr
		}
		maxDemandBilling = toEnergyReadings(billingRows)
	}

	// R96: every distinct boundary instant this request touches (len(buckets)+1
	// of them) is resolved to at most ONE reading exactly once, keyed by its
	// own instant — never once per period. This is what makes period N's end
	// and period N+1's start look up the SAME reading, so consumption
	// telescopes exactly within a level (K2): there is no per-period
	// re-derivation of a shared boundary that could disagree with its
	// neighbour.
	resolved := resolveBoundaries(level, buckets, loadProfile, billing, daily, useBilling, useDaily)

	rows := make([]Row, 0, len(buckets))
	for _, w := range buckets {
		start := resolved[w.From.UnixNano()]
		end := resolved[w.To.UnixNano()]

		derivation := energy.Derive(w, start, end, resets, priors)
		if !derivation.Emitted {
			// 02 §3.1: no row, never a zero row. Task 7's plain Consumption
			// never writes missing_readings — that is Task 8's
			// ConsumptionAndRecord, only for a specifically requested
			// billing period.
			continue
		}

		row := Row{
			AnalyzerID: analyzerID,
			Window:     w,
			Values:     derivation.Values,
			Indexes:    endIndexes(end),
			// I-9: only the bucket's own sub-range of each already-sorted
			// source is ever scanned, never the whole loaded set per bucket.
			MaxDemandKw: maxDemandInWindow(w, loadProfile, daily, maxDemandBilling),
			// Source is exactly derivation.Source: the END boundary's own
			// Kind (R96), or start.Kind when end is a reset row (M-4). Under
			// R96 a period's start and end boundary can resolve to DIFFERENT
			// kinds — resolveBoundaries chose each independently — so Source
			// always names the END kind specifically, never "the period's
			// kind" (there no longer is one).
			Source:  derivation.Source,
			Suspect: derivation.Suspect,
		}
		row.InductiveRatio, row.CapacitiveRatio = energy.Ratios(derivation)
		rows = append(rows, row)
	}
	return rows, nil
}

// resolveBoundaries implements R96: resolves EVERY distinct boundary instant
// buckets touches — buckets[0].From plus every bucket's own To, len(buckets)+1
// instants in total — to at most one reading, exactly ONCE per instant,
// keyed by that instant's UnixNano. Because resolution depends only on the
// instant itself (never on which period is asking, nor on which side of a
// period it falls), looking the same instant up for period N's end and
// period N+1's start always yields the identical *energy.Reading pointer —
// this is what makes a level's consumption telescope exactly (R96/K2): there
// is no possibility of two different, per-period re-derivations of a shared
// boundary disagreeing with each other.
func resolveBoundaries(level energy.Level, buckets []energy.Window, loadProfile, billing, daily []energy.Reading, useBilling, useDaily bool) map[int64]*energy.Reading {
	out := make(map[int64]*energy.Reading, len(buckets)+1)
	resolve := func(bound time.Time) {
		key := bound.UnixNano()
		if _, ok := out[key]; ok {
			return
		}
		out[key] = resolveBoundary(level, bound, loadProfile, billing, daily, useBilling, useDaily)
	}
	resolve(buckets[0].From)
	for _, w := range buckets {
		resolve(w.To)
	}
	return out
}

// resolveBoundary implements R96's per-instant kind precedence:
//
//	Monthly:        billing (BillingSnapshotTolerance) -> load_profile (LoadProfileBoundaryTolerance) -> daily (DailySnapshotTolerance)
//	Daily / Yearly: load_profile (LoadProfileBoundaryTolerance) -> daily (DailySnapshotTolerance)
//	Hourly:         load_profile only, §3.1 unchanged — NO tolerance, SelectBoundary's plain look-back rule
//
// A later-precedence kind is tried only when an earlier one has no candidate
// AT ALL within its own tolerance (boundaryTolerance, below) for THIS bound —
// never because the two together failed to form a usable pair. A boundary
// with no candidate in any tried kind resolves to nil, and Derive already
// turns a nil boundary into "no row" for the periods on both sides of it.
func resolveBoundary(level energy.Level, bound time.Time, loadProfile, billing, daily []energy.Reading, useBilling, useDaily bool) *energy.Reading {
	if level == energy.Hourly {
		// §3.1 unchanged: load_profile only, unbounded look-back (Q7), no
		// tolerance check at all.
		return energy.SelectBoundary(loadProfile, bound)
	}
	if useBilling {
		if r := boundaryCandidate(billing, bound, boundaryTolerance(level, bound, BillingSnapshotTolerance)); r != nil {
			return r
		}
	}
	if r := boundaryCandidate(loadProfile, bound, boundaryTolerance(level, bound, LoadProfileBoundaryTolerance)); r != nil {
		return r
	}
	if useDaily {
		if r := boundaryCandidate(daily, bound, boundaryTolerance(level, bound, DailySnapshotTolerance)); r != nil {
			return r
		}
	}
	return nil
}

// boundaryCandidate returns readings' own SelectBoundary(bound) reading only
// when it lies within [bound-tol, bound] — SelectBoundary's own contract
// already guarantees TS <= bound, so only the lower bound needs checking
// here — nil otherwise (no candidate of this kind for this instant).
func boundaryCandidate(readings []energy.Reading, bound time.Time, tol time.Duration) *energy.Reading {
	r := energy.SelectBoundary(readings, bound)
	if r == nil || r.TS.Before(bound.Add(-tol)) {
		return nil
	}
	return r
}

// boundaryTolerance implements R96's K1 fix: a kind's own snapshot tolerance
// is capped at HALF the width of the bucket(s) that meet at bound, so a
// tolerance wider than a bucket can never let one candidate answer for two
// neighbouring buckets at once (K1: an uncapped 36h daily tolerance at Daily
// level could move two entire days into one row around a single missing
// snapshot). Both the bucket ending at bound and the bucket beginning at
// bound are consulted — their widths can differ across a DST transition, in
// either direction — and the SMALLER of the two governs, so a boundary
// touching a short bucket (a Daily bucket on a 23h fall day caps every kind
// at 11.5h) is never treated as more forgiving than that short bucket
// allows. energy.Bucket is called with bound and bound-1ns rather than
// anything from the caller's own buckets slice, so this is exactly the
// calendar-correct bucket width regardless of where bound sits in the
// request (the first or last instant of the whole request still gets its
// true neighbouring bucket's width, even though that neighbour itself may
// fall outside the request).
func boundaryTolerance(level energy.Level, bound time.Time, kindTolerance time.Duration) time.Duration {
	prev := energy.Bucket(level, bound.Add(-time.Nanosecond), istanbul)
	next := energy.Bucket(level, bound, istanbul)
	width := prev.To.Sub(prev.From)
	if w := next.To.Sub(next.From); w < width {
		width = w
	}
	if half := width / 2; half < kindTolerance {
		return half
	}
	return kindTolerance
}

// clampLookback implements Minor M-c (performance, no correctness impact): a
// kind's look-back returned by firstBoundaryLookback can be an arbitrarily
// old reading's own ts — for example a `daily` reading from a year ago, when
// that is the only one BoundaryReadings finds at or before first.From. Such
// a reading can never actually resolve any boundary this request will ever
// ask for: R96 caps every kind's tolerance at boundaryTolerance, which is at
// most kindTolerance itself, so a reading older than first.From-kindTolerance
// is already too stale to qualify at first.From, and every LATER bucket's
// own bound is even further from it. Clamping the look-back up to
// first.From-kindTolerance therefore never drops evidence a real boundary
// resolution could have used, while bounding how far back a single very old
// reading can force this request's Range calls (load_profile, reset, and the
// kind's own Range) to reach.
func clampLookback(lookback, firstFrom time.Time, kindTolerance time.Duration) time.Time {
	if floor := firstFrom.Add(-kindTolerance); lookback.Before(floor) {
		return floor
	}
	return lookback
}

// endIndexes returns the closing index of every register directly from the
// end boundary reading's own values (05 §5 "every index field"): the
// billing path's closing index is never derived, it is simply what the end
// reading itself reported, independent of whether that register's
// consumption came out suspect.
func endIndexes(end *energy.Reading) map[energy.Register]*decimal.Decimal {
	out := make(map[energy.Register]*decimal.Decimal, len(energy.AllRegisters()))
	for _, reg := range energy.AllRegisters() {
		out[reg] = end.Value(reg)
	}
	return out
}

// firstBoundaryLookback implements I-17's query shape for computing where a
// single kind's look-back should begin: it reads the FIRST bucket's own
// start boundary reading of kind, and returns that reading's ts, or
// first.From when no such reading exists (matching a caller that has never
// reported that kind at all). The caller decides separately how to load the
// kind's Range (C1: sometimes from an earlier, shared minimum across every
// kind the request may use, never blindly from this one kind's own
// look-back).
func (b *Billing) firstBoundaryLookback(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, kind model.ReadingKind, first energy.Window) (time.Time, error) {
	startReading, _, err := b.deps.Readings.BoundaryReadings(ctx, sc, analyzerID, kind, first.From, first.To)
	if err != nil {
		return time.Time{}, err
	}
	if startReading != nil {
		return startReading.Ts, nil
	}
	return first.From, nil
}

// maxDemandInWindow implements I-9: each source slice is already sorted
// ascending by ts, so the bucket's own sub-range within each is found with
// sort.Search — O(log n) per source per bucket — instead of rescanning the
// whole loaded set on every bucket (O(buckets x readings), the shape that
// cost 2.78s for one analyzer-year of 15-minute readings at Hourly). Only
// the bounded sub-slice is ever handed to energy.MaxDemand.
func maxDemandInWindow(w energy.Window, sources ...[]energy.Reading) *decimal.Decimal {
	var max *decimal.Decimal
	for _, src := range sources {
		lo := sort.Search(len(src), func(i int) bool { return !src[i].TS.Before(w.From) })
		hi := sort.Search(len(src), func(i int) bool { return !src[i].TS.Before(w.To) })
		if lo >= hi {
			continue
		}
		if m := energy.MaxDemand(w, src[lo:hi]); m != nil && (max == nil || m.GreaterThan(*max)) {
			v := *m
			max = &v
		}
	}
	return max
}

// toEnergyReadings converts a slice of stored readings, preserving order —
// it never sorts and never merges with another slice (I-2's precondition
// stays the caller's single query's own SQL ordering).
func toEnergyReadings(rows []model.MeterReading) []energy.Reading {
	out := make([]energy.Reading, 0, len(rows))
	for _, r := range rows {
		out = append(out, toEnergyReading(r))
	}
	return out
}
