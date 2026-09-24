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
// mix) rather than the plain instant comparison this key needs
// (.Equal, never ==) — converting to an int64 first sidesteps the
// question entirely rather than relying on two Time values built through
// different paths (a domain Bucket() call vs. a Postgres-decoded column)
// happening to compare correctly under ==.
type analyticsBucketKey struct {
	analyzer uuid.UUID
	bucket   int64
}

// Consumption serves charts and tables from the continuous aggregates. The
// repository boundary-differences each bucket (F15q R471), so a bucket with
// readings on its edges matches Billing. At Monthly/Yearly, EVERY requested bucket absent from the
// materialised view — whether it is the still-open trailing period or an
// earlier closed month/year the refresh policy has not reached yet (R94,
// amending R88) — is composed from consumption_daily closing indexes,
// clock-free — never from meter_readings, which would break R61's
// structural separation.
func (a *Analytics) Consumption(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error) {
	if err := validateRequest(sc, req); err != nil {
		return nil, err
	}

	window := energy.Window{From: req.Range.From, To: req.Range.To}
	wanted := energy.Buckets(req.Level, window, istanbul)
	if len(wanted) == 0 {
		return nil, nil
	}

	// The aggregate is queried over the request WIDENED to whole
	// buckets of Level — [wanted[0].From, wanted[last].To) — never
	// req.Range directly. The materialised view's own predicate is
	// `bucket >= from_ts`, so a From landing mid-bucket (e.g. a monthly
	// request starting on the 15th) would otherwise silently drop the
	// bucket containing it, even though Billing (which already expands to
	// whole buckets) returns it.
	queryRange := store.TimeRange{From: wanted[0].From, To: wanted[len(wanted)-1].To}

	buckets, err := a.fetchBuckets(ctx, sc, req, queryRange)
	if err != nil {
		return nil, err
	}

	byKey := make(map[analyticsBucketKey]model.ConsumptionBucket, len(buckets))
	for _, b := range buckets {
		byKey[analyticsBucketKey{b.AnalyzerID, b.Bucket.UnixNano()}] = b
	}

	// R94 (amends R88): every requested bucket absent from the materialised
	// view, at Monthly/Yearly, is a composition candidate — not only the
	// trailing one. Analytics has no clock and so has no way to tell
	// "genuinely still open" apart from "closed but not yet refreshed", and
	// R94's fix is to stop trying: compose whichever windows are missing,
	// wherever they fall in the request, as long as consumption_daily has
	// buckets to compose them from.
	needsCompose := req.Level == energy.Monthly || req.Level == energy.Yearly

	missing := make(map[uuid.UUID][]energy.Window)
	if needsCompose {
		for _, id := range req.AnalyzerIDs {
			for _, w := range wanted {
				if _, ok := byKey[analyticsBucketKey{id, w.From.UnixNano()}]; !ok {
					missing[id] = append(missing[id], w)
				}
			}
		}
	}

	var composed map[analyticsBucketKey]Row
	if len(missing) > 0 {
		composed, err = a.composeMissingPeriods(ctx, sc, req.Level, wanted, missing)
		if err != nil {
			return nil, err
		}
	}

	rows := make([]Row, 0, len(wanted)*len(req.AnalyzerIDs))
	for _, id := range req.AnalyzerIDs {
		for _, w := range wanted {
			key := analyticsBucketKey{id, w.From.UnixNano()}
			if b, ok := byKey[key]; ok {
				rows = append(rows, bucketToRow(b, w))
				continue
			}
			if row, ok := composed[key]; ok {
				rows = append(rows, row)
			}
		}
	}
	return rows, nil
}

// fetchBuckets maps req.Level to the matching AnalyticsRepository method,
// queried over r rather than req.Range directly (r is the request
// widened to whole buckets of req.Level by the caller).
func (a *Analytics) fetchBuckets(ctx context.Context, sc store.Scope, req SeriesRequest, r store.TimeRange) ([]model.ConsumptionBucket, error) {
	switch req.Level {
	case energy.Hourly:
		return a.deps.Analytics.ConsumptionHourly(ctx, sc, req.AnalyzerIDs, r)
	case energy.Daily:
		return a.deps.Analytics.ConsumptionDaily(ctx, sc, req.AnalyzerIDs, r)
	case energy.Monthly:
		return a.deps.Analytics.ConsumptionMonthly(ctx, sc, req.AnalyzerIDs, r)
	case energy.Yearly:
		return a.deps.Analytics.ConsumptionYearly(ctx, sc, req.AnalyzerIDs, r)
	default:
		return nil, ErrInvalidRequest
	}
}

// composeMissingPeriods implements R94 (amending R88): for every analyzer in
// missing, compose every one of its listed windows from consumption_daily —
// each register's value is the closing index of the last consumption_daily
// bucket inside that window, minus the closing index of the last
// consumption_daily bucket before it began; nil when either is missing. It
// never reads meter_readings.
//
// ONE ConsumptionDaily call covers every analyzer and every missing window
// at once, bounded to [Bucket(level, wanted[0].From-1ns).From,
// wanted[last].To): the previous period's own start (before the very first
// requested window) is computed with energy.Bucket rather than an arbitrary
// look-back, so the query reaches back exactly one period before the
// earliest possible missing window — never further, never less — and
// forward to the end of the request.
func (a *Analytics) composeMissingPeriods(ctx context.Context, sc store.Scope, level energy.Level, wanted []energy.Window, missing map[uuid.UUID][]energy.Window) (map[analyticsBucketKey]Row, error) {
	analyzerIDs := make([]uuid.UUID, 0, len(missing))
	for id := range missing {
		analyzerIDs = append(analyzerIDs, id)
	}

	first := wanted[0]
	last := wanted[len(wanted)-1]
	prev := energy.Bucket(level, first.From.Add(-time.Nanosecond), istanbul)
	dailyRange := store.TimeRange{From: prev.From, To: last.To}
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

	out := make(map[analyticsBucketKey]Row)
	for id, windows := range missing {
		dailyBuckets := byAnalyzer[id]
		for _, w := range windows {
			if row := composeOpenPeriodRow(id, w, dailyBuckets); row != nil {
				out[analyticsBucketKey{id, w.From.UnixNano()}] = *row
			}
		}
	}
	return out, nil
}

// composeOpenPeriodRow is composeMissingPeriods' single-window arithmetic
// (R94, amending R88), factored out so it has nothing left to depend on but
// its inputs.
//
// When lastBefore is nil (no consumption_daily bucket exists before w
// at all), there is nothing to subtract from and no composed row is
// emitted — never a row whose every value is nil. bucketIndexes only ever
// has keys for the seven registers migration 00005 gives a closing-index
// column to; the other five (every export register but active_export) are
// therefore always nil in a composed row, same as in a materialised one.
//
// MaxDemandKw is the MAXIMUM of MaxDemandKw over every daily bucket
// strictly inside w, not merely the last one: a period's peak day is not
// necessarily its most recent day.
func composeOpenPeriodRow(analyzerID uuid.UUID, w energy.Window, dailyBuckets []model.ConsumptionBucket) *Row {
	var lastBefore, lastInside *model.ConsumptionBucket
	var maxDemand *decimal.Decimal
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
			if b.MaxDemandKw != nil && (maxDemand == nil || b.MaxDemandKw.GreaterThan(*maxDemand)) {
				v := *b.MaxDemandKw
				maxDemand = &v
			}
		}
	}
	if lastInside == nil || lastBefore == nil {
		// Nothing inside the period yet, or nothing before it to
		// subtract from — either way there is no sound composed figure, so
		// no row is emitted (never an all-nil-values row).
		return nil
	}

	insideIdx := bucketIndexes(*lastInside)
	beforeIdx := bucketIndexes(*lastBefore)

	values := make(map[energy.Register]*decimal.Decimal, len(energy.AllRegisters()))
	for _, reg := range energy.AllRegisters() {
		inside := insideIdx[reg]
		before := beforeIdx[reg]
		if inside == nil || before == nil {
			values[reg] = nil
			continue
		}
		v := inside.Sub(*before)
		// R102: a composed figure can go negative across a meter swap
		// exactly as a materialised bucket's own delta column can
		// (bucketValues above) — nonNegative applies the same rule here.
		values[reg] = nonNegative(&v)
	}

	return &Row{
		AnalyzerID:      analyzerID,
		Window:          w,
		Values:          values,
		Indexes:         insideIdx,
		InductiveRatio:  energy.Ratio(values[energy.ReactiveInductiveImport], values[energy.ActiveImport]),
		CapacitiveRatio: energy.Ratio(values[energy.ReactiveCapacitiveImport], values[energy.ActiveImport]),
		MaxDemandKw:     maxDemand,
		Source:          energy.KindLoadProfile,
		// R94: a composed row is closing-to-closing across whatever
		// materialisation gap it fills and may differ from the later
		// materialised row by the boundary step once the view
		// catches up — Partial marks it as not directly comparable.
		Partial: true,
	}
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
// entry, nil where the aggregate's own column is nil OR negative (R102: a
// meter swap or other index discontinuity can make last-first inside a
// bucket negative; Analytics has no suspicion channel, so a negative figure
// comes back as an ordinary "no data" nil rather than a number nobody
// flagged as wrong).
func bucketValues(b model.ConsumptionBucket) map[energy.Register]*decimal.Decimal {
	return map[energy.Register]*decimal.Decimal{
		energy.ActiveImport:             nonNegative(b.ActiveConsumption),
		energy.ReactiveInductiveImport:  nonNegative(b.InductiveConsumption),
		energy.ReactiveCapacitiveImport: nonNegative(b.CapacitiveConsumption),
		energy.T1Import:                 nonNegative(b.T1Consumption),
		energy.T2Import:                 nonNegative(b.T2Consumption),
		energy.T3Import:                 nonNegative(b.T3Consumption),
		energy.ActiveExport:             nonNegative(b.ActiveGeneration),
		energy.ReactiveInductiveExport:  nonNegative(b.InductiveGeneration),
		energy.ReactiveCapacitiveExport: nonNegative(b.CapacitiveGeneration),
		energy.T1Export:                 nonNegative(b.T1Generation),
		energy.T2Export:                 nonNegative(b.T2Generation),
		energy.T3Export:                 nonNegative(b.T3Generation),
	}
}

// nonNegative returns d unchanged, or nil when d is itself nil or negative
// (R102): the Analytics path never returns a negative
// consumption for a register — a materialised bucket's own delta column or a
// composed row's closing-index subtraction can go negative across a meter
// swap, and since Analytics has no suspicion channel (only Billing records
// suspect periods), the only sound thing to surface is "unavailable", never
// the negative number itself.
func nonNegative(d *decimal.Decimal) *decimal.Decimal {
	if d == nil || d.IsNegative() {
		return nil
	}
	return d
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
