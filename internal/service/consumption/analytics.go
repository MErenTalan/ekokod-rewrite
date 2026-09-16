package consumption

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// AnalyticsDeps is everything the analytics path may reach. It deliberately
// has no reading repository: the analytics path must not be able to read
// the hypertable (R61).
type AnalyticsDeps struct {
	Analytics store.AnalyticsRepository
	Log       *slog.Logger
}

// Analytics is the fast, aggregate-only consumption path. There is no
// shared Service struct (R61): Analytics is constructed alone, and has no
// field of any other repository type to poison in a guard test.
type Analytics struct {
	deps AnalyticsDeps
}

// NewAnalytics validates deps and returns an Analytics.
func NewAnalytics(d AnalyticsDeps) (*Analytics, error) {
	switch {
	case d.Analytics == nil:
		return nil, errRequired("AnalyticsDeps.Analytics")
	case d.Log == nil:
		return nil, errRequired("AnalyticsDeps.Log")
	}
	return &Analytics{deps: d}, nil
}

// analyticsBucketKey identifies one (analyzer, bucket-start) cell across a
// multi-analyzer request. bucket is From.UnixNano() rather than the
// time.Time itself: Time is comparable and would work as a map key too, but
// hashing it exercises time.Time's own equality semantics (wall/monotonic
// mix) rather than the plain instant comparison this package's M-15 rule
// requires (.Equal, never ==) — converting to an int64 first sidesteps the
// question entirely rather than relying on two Time values built through
// different paths (a domain Bucket() call vs. a Postgres-decoded column)
// happening to compare correctly under ==.
type analyticsBucketKey struct {
	analyzer uuid.UUID
	bucket   int64
}

// Consumption serves charts and tables from the continuous aggregates. It
// is fast and slightly understates a period: a bucket misses the step
// between the last reading of one bucket and the first of the next (04
// §4.3). At Monthly/Yearly, the current OPEN period (the last window in the
// requested range) is composed from consumption_daily closing indexes
// (R88) — never from meter_readings, which would break R61's structural
// separation.
func (a *Analytics) Consumption(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error) {
	if err := validateRequest(sc, req); err != nil {
		return nil, err
	}

	window := energy.Window{From: req.Range.From, To: req.Range.To}
	wanted := energy.Buckets(req.Level, window, istanbul)
	if len(wanted) == 0 {
		return nil, nil
	}

	buckets, err := a.fetchBuckets(ctx, sc, req)
	if err != nil {
		return nil, err
	}

	byKey := make(map[analyticsBucketKey]model.ConsumptionBucket, len(buckets))
	for _, b := range buckets {
		byKey[analyticsBucketKey{b.AnalyzerID, b.Bucket.UnixNano()}] = b
	}

	// R88 only ever applies to the LAST window in the requested range, and
	// only at Monthly/Yearly: Analytics has no clock (AnalyticsDeps carries
	// none) and so has no way to tell "genuinely still open" apart from
	// "closed but not yet refreshed" other than position — the current
	// period, whatever it is, always trails. Any earlier missing bucket is
	// a materialisation gap, not an open period, and stays omitted, exactly
	// as a materialized_only view already behaves for every level today.
	needsOpenCompose := req.Level == energy.Monthly || req.Level == energy.Yearly
	lastWindow := wanted[len(wanted)-1]

	var toCompose []uuid.UUID
	if needsOpenCompose {
		for _, id := range req.AnalyzerIDs {
			if _, ok := byKey[analyticsBucketKey{id, lastWindow.From.UnixNano()}]; !ok {
				toCompose = append(toCompose, id)
			}
		}
	}

	var composed map[uuid.UUID]*Row
	if len(toCompose) > 0 {
		composed, err = a.composeOpenPeriods(ctx, sc, toCompose, req.Level, lastWindow)
		if err != nil {
			return nil, err
		}
	}

	rows := make([]Row, 0, len(wanted)*len(req.AnalyzerIDs))
	for _, id := range req.AnalyzerIDs {
		for i, w := range wanted {
			if b, ok := byKey[analyticsBucketKey{id, w.From.UnixNano()}]; ok {
				rows = append(rows, bucketToRow(b, w))
				continue
			}
			if needsOpenCompose && i == len(wanted)-1 {
				if row := composed[id]; row != nil {
					rows = append(rows, *row)
				}
			}
		}
	}
	return rows, nil
}

// fetchBuckets maps req.Level to the matching AnalyticsRepository method.
func (a *Analytics) fetchBuckets(ctx context.Context, sc store.Scope, req SeriesRequest) ([]model.ConsumptionBucket, error) {
	switch req.Level {
	case energy.Hourly:
		return a.deps.Analytics.ConsumptionHourly(ctx, sc, req.AnalyzerIDs, req.Range)
	case energy.Daily:
		return a.deps.Analytics.ConsumptionDaily(ctx, sc, req.AnalyzerIDs, req.Range)
	case energy.Monthly:
		return a.deps.Analytics.ConsumptionMonthly(ctx, sc, req.AnalyzerIDs, req.Range)
	case energy.Yearly:
		return a.deps.Analytics.ConsumptionYearly(ctx, sc, req.AnalyzerIDs, req.Range)
	default:
		return nil, ErrInvalidRequest
	}
}

// composeOpenPeriods implements R88 for every analyzer in analyzerIDs whose
// last requested window (w) has no materialized row: each register's value
// is the closing index of the last consumption_daily bucket inside w, minus
// the closing index of the last consumption_daily bucket before w began —
// nil when either is missing. It never reads meter_readings.
//
// The daily query is bounded to [previous-period-start, w.To): the previous
// period's own start is computed with energy.Bucket rather than an
// arbitrary lookback, so the query always reaches back exactly one period —
// never further, never less — regardless of how short w's own elapsed
// portion is.
func (a *Analytics) composeOpenPeriods(ctx context.Context, sc store.Scope, analyzerIDs []uuid.UUID, level energy.Level, w energy.Window) (map[uuid.UUID]*Row, error) {
	prev := energy.Bucket(level, w.From.Add(-time.Nanosecond), istanbul)
	dailyRange := store.TimeRange{From: prev.From, To: w.To}
	if !dailyRange.Valid() {
		return nil, nil
	}

	daily, err := a.deps.Analytics.ConsumptionDaily(ctx, sc, analyzerIDs, dailyRange)
	if err != nil {
		return nil, err
	}

	byAnalyzer := make(map[uuid.UUID][]model.ConsumptionBucket, len(analyzerIDs))
	for _, d := range daily {
		byAnalyzer[d.AnalyzerID] = append(byAnalyzer[d.AnalyzerID], d)
	}

	out := make(map[uuid.UUID]*Row, len(analyzerIDs))
	for _, id := range analyzerIDs {
		if row := composeOpenPeriodRow(id, w, byAnalyzer[id]); row != nil {
			out[id] = row
		}
	}
	return out, nil
}

// composeOpenPeriodRow is composeOpenPeriods' single-analyzer arithmetic
// (R88), factored out so it has nothing left to depend on but its inputs.
func composeOpenPeriodRow(analyzerID uuid.UUID, w energy.Window, dailyBuckets []model.ConsumptionBucket) *Row {
	var lastBefore, lastInside *model.ConsumptionBucket
	for i := range dailyBuckets {
		b := &dailyBuckets[i]
		switch {
		case b.Bucket.Before(w.From):
			if lastBefore == nil || b.Bucket.After(lastBefore.Bucket) {
				lastBefore = b
			}
		case !b.Bucket.Before(w.From) && b.Bucket.Before(w.To):
			if lastInside == nil || b.Bucket.After(lastInside.Bucket) {
				lastInside = b
			}
		}
	}
	if lastInside == nil {
		// Nothing at all inside the open period yet: no composed row.
		return nil
	}

	insideIdx := bucketIndexes(*lastInside)
	beforeIdx := bucketIndexes(zeroBucketIfNil(lastBefore))

	values := make(map[energy.Register]*decimal.Decimal, len(energy.AllRegisters()))
	for _, reg := range energy.AllRegisters() {
		inside := insideIdx[reg]
		before := beforeIdx[reg]
		if lastBefore == nil || inside == nil || before == nil {
			values[reg] = nil
			continue
		}
		v := inside.Sub(*before)
		values[reg] = &v
	}

	return &Row{
		AnalyzerID:      analyzerID,
		Window:          w,
		Values:          values,
		Indexes:         insideIdx,
		InductiveRatio:  energy.Ratio(values[energy.ReactiveInductiveImport], values[energy.ActiveImport]),
		CapacitiveRatio: energy.Ratio(values[energy.ReactiveCapacitiveImport], values[energy.ActiveImport]),
		MaxDemandKw:     lastInside.MaxDemandKw,
		Source:          energy.KindLoadProfile,
		Partial:         true,
	}
}

// zeroBucketIfNil returns *b, or a ConsumptionBucket with every index field
// nil when b is nil — so bucketIndexes always has a map to work with, and a
// missing "before" bucket reads the same as a bucket with nothing measured.
func zeroBucketIfNil(b *model.ConsumptionBucket) model.ConsumptionBucket {
	if b == nil {
		return model.ConsumptionBucket{}
	}
	return *b
}

// bucketToRow converts one materialised aggregate row into a Row. Ratios
// are computed from the bucket's own consumption values with energy.Ratio
// (R62), and Source is always energy.KindLoadProfile: every consumption_*
// continuous aggregate is built over kind = 'load_profile' rows only
// (migration 00005's filter).
func bucketToRow(b model.ConsumptionBucket, w energy.Window) Row {
	values := bucketValues(b)
	return Row{
		AnalyzerID:      b.AnalyzerID,
		Window:          w,
		Values:          values,
		Indexes:         bucketIndexes(b),
		InductiveRatio:  energy.Ratio(values[energy.ReactiveInductiveImport], values[energy.ActiveImport]),
		CapacitiveRatio: energy.Ratio(values[energy.ReactiveCapacitiveImport], values[energy.ActiveImport]),
		MaxDemandKw:     b.MaxDemandKw,
		Source:          energy.KindLoadProfile,
	}
}

// bucketValues maps a ConsumptionBucket's per-register consumption (delta)
// columns onto energy.Register. Every one of the twelve registers has an
// entry, nil where the aggregate's own column is nil.
func bucketValues(b model.ConsumptionBucket) map[energy.Register]*decimal.Decimal {
	return map[energy.Register]*decimal.Decimal{
		energy.ActiveImport:             b.ActiveConsumption,
		energy.ReactiveInductiveImport:  b.InductiveConsumption,
		energy.ReactiveCapacitiveImport: b.CapacitiveConsumption,
		energy.T1Import:                 b.T1Consumption,
		energy.T2Import:                 b.T2Consumption,
		energy.T3Import:                 b.T3Consumption,
		energy.ActiveExport:             b.ActiveGeneration,
		energy.ReactiveInductiveExport:  b.InductiveGeneration,
		energy.ReactiveCapacitiveExport: b.CapacitiveGeneration,
		energy.T1Export:                 b.T1Generation,
		energy.T2Export:                 b.T2Generation,
		energy.T3Export:                 b.T3Generation,
	}
}

// bucketIndexes maps a ConsumptionBucket's closing-index columns onto
// energy.Register. Unlike bucketValues, only the seven registers migration
// 00005 actually gives a closing-index column to are present as keys
// (active_import through t3_import, plus active_export): the aggregate has
// no per-register closing index for the other five export registers, and a
// missing key reads identically to an explicit nil for any caller doing a
// map lookup.
func bucketIndexes(b model.ConsumptionBucket) map[energy.Register]*decimal.Decimal {
	return map[energy.Register]*decimal.Decimal{
		energy.ActiveImport:             b.ActiveIndex,
		energy.ReactiveInductiveImport:  b.InductiveIndex,
		energy.ReactiveCapacitiveImport: b.CapacitiveIndex,
		energy.T1Import:                 b.T1Index,
		energy.T2Import:                 b.T2Index,
		energy.T3Import:                 b.T3Index,
		energy.ActiveExport:             b.ActiveGenerationIndex,
	}
}
