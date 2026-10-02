package consumption

import (
	"context"
	"log/slog"

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

// Consumption serves charts and tables from the aggregates. The repository
// boundary-differences each bucket (F15q R471), so a bucket with readings on
// its edges matches Billing. R499: months and years are rolled up from
// consumption_daily by the repository, so every period that has data has a
// row; one the next period's first reading does not close is marked Partial.
func (a *Analytics) Consumption(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error) {
	if err := validateRequest(sc, req); err != nil {
		return nil, err
	}

	window := energy.Window{From: req.Range.From, To: req.Range.To}
	wanted := energy.Buckets(req.Level, window, istanbul)
	if len(wanted) == 0 {
		return nil, nil
	}

	// The aggregate is queried over the request WIDENED to whole buckets of
	// Level, never req.Range directly: a From landing mid-bucket would
	// otherwise drop the bucket containing it, which Billing returns.
	queryRange := store.TimeRange{From: wanted[0].From, To: wanted[len(wanted)-1].To}

	buckets, err := a.fetchBuckets(ctx, sc, req, queryRange)
	if err != nil {
		return nil, err
	}

	byKey := make(map[analyticsBucketKey]model.ConsumptionBucket, len(buckets))
	for _, b := range buckets {
		byKey[analyticsBucketKey{b.AnalyzerID, b.Bucket.UnixNano()}] = b
	}

	coarse := req.Level == energy.Monthly || req.Level == energy.Yearly
	rows := make([]Row, 0, len(wanted)*len(req.AnalyzerIDs))
	for _, id := range req.AnalyzerIDs {
		for _, w := range wanted {
			b, ok := byKey[analyticsBucketKey{id, w.From.UnixNano()}]
			if !ok {
				continue
			}
			row := bucketToRow(b, w)
			row.Partial = coarse && !b.ClosedByBoundary
			rows = append(rows, row)
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

// bucketToRow converts one aggregate row into a Row. Ratios are computed from
// the bucket's own consumption values with energy.Ratio (R62), and Source is
// always energy.KindLoadProfile: every consumption aggregate is built over
// kind = 'load_profile' rows only.
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
// meter swap or other index discontinuity can make a bucket's difference
// negative; Analytics has no suspicion channel, so a negative figure comes
// back as an ordinary "no data" nil rather than a number nobody flagged as
// wrong).
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
// (R102): the Analytics path never returns a negative consumption for a
// register — a bucket's difference can go negative across a meter swap, and
// since Analytics has no suspicion channel (only Billing records suspect
// periods), the only sound thing to surface is "unavailable".
func nonNegative(d *decimal.Decimal) *decimal.Decimal {
	if d == nil || d.IsNegative() {
		return nil
	}
	return d
}

// bucketIndexes maps a ConsumptionBucket's closing-index fields onto
// energy.Register. Only the seven registers the model carries a closing index
// for are present as keys (active_import through t3_import, plus
// active_export); a missing key reads like an explicit nil.
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
