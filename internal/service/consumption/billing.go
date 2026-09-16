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
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
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

	// Locker serialises Task 8's ConsumptionAndRecord check-then-create per
	// (analyzer, period_start) (C-6): consumption_anomalies' own partial
	// index claims no database-level uniqueness, so this lock is what
	// actually prevents two concurrent runs from creating two rows for the
	// same period. Required only by ConsumptionAndRecord; left nil, this
	// type still constructs and Consumption still works exactly as Task 7
	// built it.
	Locker lock.Locker
	// Analyzers resolves the analyzer's own provider when Task 8's
	// ResolveAnomaly (ResolveByRegisteringReset) writes an operator-entered
	// reset reading. Required only by ResolveAnomaly.
	Analyzers store.AnalyzerRepository
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
	// C-6: a resolved manual_override/accepted anomaly must be reflected by
	// Consumption itself, not only by Task 8's ConsumptionAndRecord wrapper
	// — applyResolvedAnomalies (anomalies.go) post-processes rows in place.
	if err := b.applyResolvedAnomalies(ctx, sc, rows); err != nil {
		return nil, err
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
	minLookback := lpLookback

	// Monthly-only: billing-kind readings, for R62/R63's preference.
	useBilling := level == energy.Monthly
	var billingLookback time.Time
	if useBilling {
		billingLookback, err = b.firstBoundaryLookback(ctx, sc, analyzerID, model.ReadingKindBilling, first)
		if err != nil {
			return nil, err
		}
		if billingLookback.Before(minLookback) {
			minLookback = billingLookback
		}
	}

	// R95: daily is a fallback boundary kind at Daily/Monthly/Yearly, never
	// Hourly. Its own look-back feeds the shared minimum too (C1), on the
	// same reasoning as billing's.
	useDaily := level != energy.Hourly
	var dailyLookback time.Time
	if useDaily {
		dailyLookback, err = b.firstBoundaryLookback(ctx, sc, analyzerID, model.ReadingKindDaily, first)
		if err != nil {
			return nil, err
		}
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

	rows := make([]Row, 0, len(buckets))
	for _, w := range buckets {
		boundaryReadings := selectBoundarySource(level, w, loadProfile, billing, daily)

		start := energy.SelectBoundary(boundaryReadings, w.From)
		end := energy.SelectBoundary(boundaryReadings, w.To)

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
			// Source is exactly derivation.Source (end.Kind, or start.Kind
			// when end is a reset row — M-4): the readings' own .Kind field
			// already carries the winning kind correctly, because
			// boundaryReadings was loaded from the one Range call scoped to
			// that specific kind.
			Source:  derivation.Source,
			Suspect: derivation.Suspect,
		}
		row.InductiveRatio, row.CapacitiveRatio = energy.Ratios(derivation)
		rows = append(rows, row)
	}
	return rows, nil
}

// selectBoundarySource picks which loaded kind's readings a bucket's
// boundaries are selected from (R62/R63/R95). A period never mixes kinds:
// the returned slice is used for BOTH start and end, or is empty/nil when
// nothing qualifies (SelectBoundary then resolves both boundaries to nil and
// the bucket is skipped downstream).
//
// Precedence, never mixing within one period:
//   - Monthly:        billing (R63, staleness-bounded) -> load_profile -> daily
//   - Daily / Yearly:  load_profile -> daily
//   - Hourly:          load_profile only — daily is NEVER a boundary kind here
//
// The load_profile step falls back only when load_profile itself does not
// emit a usable pair for this window (a nil boundary, or the same reading
// selected for both ends). The daily step falls back only when the daily
// pair additionally lies within DailySnapshotTolerance of its own bounds.
func selectBoundarySource(level energy.Level, w energy.Window, loadProfile, billing, daily []energy.Reading) []energy.Reading {
	if level == energy.Monthly && monthCoveredByBilling(w, billing) {
		return billing
	}
	if boundaryPairEmits(loadProfile, w) {
		return loadProfile
	}
	if level != energy.Hourly && dailyBoundaryCovers(w, daily) {
		return daily
	}
	return nil
}

// boundaryPairEmits reports whether readings resolves to two distinct,
// usable boundary readings for w: both non-nil and not the same instant.
// This is the "does load_profile emit" test R95's fallback chain steps past
// only when it is false.
func boundaryPairEmits(readings []energy.Reading, w energy.Window) bool {
	start := energy.SelectBoundary(readings, w.From)
	end := energy.SelectBoundary(readings, w.To)
	return start != nil && end != nil && !start.TS.Equal(end.TS)
}

// dailyBoundaryCovers implements R95's daily-fallback staleness bound: a
// daily boundary reading counts only when BOTH the start and end candidate
// lie within DailySnapshotTolerance of their own bound — the same
// bound-minus-tolerance shape monthCoveredByBilling uses for R63, at 36h
// instead of 72h. SelectBoundary already guarantees reading.TS <= bound by
// construction, so only the lower bound is checked here.
func dailyBoundaryCovers(w energy.Window, daily []energy.Reading) bool {
	start := energy.SelectBoundary(daily, w.From)
	end := energy.SelectBoundary(daily, w.To)
	if start == nil || end == nil {
		return false
	}
	if start.TS.Before(w.From.Add(-DailySnapshotTolerance)) {
		return false
	}
	if end.TS.Before(w.To.Add(-DailySnapshotTolerance)) {
		return false
	}
	return true
}

// monthCoveredByBilling implements R63: a month counts as covered by
// billing-kind readings only when BOTH the start and end boundary readings
// are non-nil AND each lies within BillingSnapshotTolerance of its own
// bound — bound - 72h <= reading.TS <= bound. SelectBoundary already
// guarantees reading.TS <= bound by construction, so only the lower bound
// is checked here.
func monthCoveredByBilling(w energy.Window, billing []energy.Reading) bool {
	start := energy.SelectBoundary(billing, w.From)
	end := energy.SelectBoundary(billing, w.To)
	if start == nil || end == nil {
		return false
	}
	if start.TS.Before(w.From.Add(-BillingSnapshotTolerance)) {
		return false
	}
	if end.TS.Before(w.To.Add(-BillingSnapshotTolerance)) {
		return false
	}
	return true
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
