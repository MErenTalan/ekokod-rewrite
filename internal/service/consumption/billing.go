package consumption

import (
	"context"
	"log/slog"
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
func (b *Billing) consumptionForAnalyzer(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, level energy.Level, buckets []energy.Window) ([]Row, error) {
	first := buckets[0]
	rangeTo := buckets[len(buckets)-1].To

	loadProfile, lpLookback, err := b.loadKindFromFirstBoundary(ctx, sc, analyzerID, model.ReadingKindLoadProfile, first, rangeTo)
	if err != nil {
		return nil, err
	}

	// Reset evidence: fetched once over the same lower bound as the
	// load_profile read, since no bucket's evidence window (start.TS,
	// end.TS] can begin before the very first bucket's own start reading.
	resetRows, err := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: lpLookback, To: rangeTo.Add(time.Microsecond)}, model.ReadingKindReset)
	if err != nil {
		return nil, err
	}
	resets := toEnergyReadings(resetRows)

	// R55/I-1: priors are load_profile readings regardless of the window's
	// own boundary kind — a billing-kind derivation still looks for
	// load_profile priors, because those are the readings dense enough to
	// have one. loadProfile is already sorted ascending by ts (the SQL
	// ReadingRange query's ORDER BY, per R91's precondition) and is passed
	// straight through: it is never merged with another read, so its order
	// is exactly the order the single query returned it in.
	priors := loadProfile

	// Monthly-only: billing-kind readings, for R62/R63's preference. Loaded
	// with the same lookback method as load_profile so SelectBoundary can
	// resolve a billing-kind boundary for the first bucket too.
	var billing []energy.Reading
	if level == energy.Monthly {
		billing, _, err = b.loadKindFromFirstBoundary(ctx, sc, analyzerID, model.ReadingKindBilling, first, rangeTo)
		if err != nil {
			return nil, err
		}
	}

	// Max demand (R65/C-3): daily-kind readings are never used for boundary
	// selection, only for MaxDemandKinds — loaded once over the plain
	// window, since MaxDemand only ever looks inside it.
	dailyRows, err := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: first.From, To: rangeTo}, model.ReadingKindDaily)
	if err != nil {
		return nil, err
	}
	daily := toEnergyReadings(dailyRows)

	// MaxDemand also allows billing-kind readings at every level, not only
	// Monthly (R65): when Monthly already loaded them above, that slice is
	// reused; otherwise they are fetched here, over the plain window, for
	// MaxDemand alone.
	maxDemandBilling := billing
	if level != energy.Monthly {
		billingRows, billingErr := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: first.From, To: rangeTo}, model.ReadingKindBilling)
		if billingErr != nil {
			return nil, billingErr
		}
		maxDemandBilling = toEnergyReadings(billingRows)
	}

	maxDemandReadings := make([]energy.Reading, 0, len(loadProfile)+len(daily)+len(maxDemandBilling))
	maxDemandReadings = append(maxDemandReadings, loadProfile...)
	maxDemandReadings = append(maxDemandReadings, daily...)
	maxDemandReadings = append(maxDemandReadings, maxDemandBilling...)

	rows := make([]Row, 0, len(buckets))
	for _, w := range buckets {
		boundaryReadings := selectBoundarySource(level, w, loadProfile, billing)

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
			AnalyzerID:  analyzerID,
			Window:      w,
			Values:      derivation.Values,
			Indexes:     endIndexes(end),
			MaxDemandKw: energy.MaxDemand(w, maxDemandReadings),
			// Source is exactly derivation.Source (end.Kind, or start.Kind
			// when end is a reset row — M-4): the readings' own .Kind field
			// already carries billing vs. load_profile correctly, because
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
// boundaries are selected from (R62/R63): Monthly tries billing-kind
// coverage first, staleness-bounded by BillingSnapshotTolerance, and falls
// back to load_profile; every other level always uses load_profile. A
// period never mixes kinds (R63): the returned slice is used for BOTH
// start and end.
func selectBoundarySource(level energy.Level, w energy.Window, loadProfile, billing []energy.Reading) []energy.Reading {
	if level != energy.Monthly {
		return loadProfile
	}
	if monthCoveredByBilling(w, billing) {
		return billing
	}
	return loadProfile
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

// loadKindFromFirstBoundary implements I-17's query shape: it reads the
// FIRST bucket's own start boundary reading of kind to learn where the
// lookback should begin, then reads the whole kind ONCE over
// [lookback, rangeTo + 1µs) — a single query per (analyzer, kind) pair
// instead of one per bucket. The returned slice is exactly what Range
// returned, in the order Range returned it (ascending by ts — R91's
// precondition; verified against internal/store/postgres/queries/
// readings.sql's ReadingRange `order by mr.ts`) — never re-sorted, and
// never merged with any other read.
//
// The +1µs, not +1ns: ReadingRange's SQL is `mr.ts < to_ts` (exclusive), and
// Range's own TimeRange is half-open by contract (repository.go), so the
// widening step must add the SMALLEST increment Postgres's timestamptz can
// actually represent to make rangeTo itself count as "at or before". That
// increment is one MICROSECOND, not one nanosecond: timestamptz stores
// microsecond precision, so `rangeTo.Add(time.Nanosecond)` round-trips
// through the wire encoding back to exactly rangeTo — the exact
// off-by-nothing bug this comment is here so nobody reintroduces (found by
// TestBillingAndAnalyticsDifferByTheBucketBoundaryStep failing against a
// real database with 30 where 40 was expected: the reading exactly at
// rangeTo was silently excluded).
func (b *Billing) loadKindFromFirstBoundary(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, kind model.ReadingKind, first energy.Window, rangeTo time.Time) (readings []energy.Reading, lookback time.Time, err error) {
	startReading, _, err := b.deps.Readings.BoundaryReadings(ctx, sc, analyzerID, kind, first.From, first.To)
	if err != nil {
		return nil, time.Time{}, err
	}
	lookback = first.From
	if startReading != nil {
		lookback = startReading.Ts
	}
	rows, err := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: lookback, To: rangeTo.Add(time.Microsecond)}, kind)
	if err != nil {
		return nil, time.Time{}, err
	}
	return toEnergyReadings(rows), lookback, nil
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
