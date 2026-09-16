package consumption

import (
	"context"
	"fmt"
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
	// Users validates that ResolveAnomaly's resolvedBy is a real user of
	// sc.CompanyID BEFORE any write (review I2/P6: a resolvedBy rejected only
	// by AnomalyRepository.Resolve's own SQL check let the reset reading get
	// written first, unresolved). Optional like Analyzers: left nil, this
	// pre-check is skipped and AnomalyRepository.Resolve's own check is the
	// only guard, exactly as before this fix round.
	Users store.UserRepository
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
// Suspicion; it never returns a number (removed-behaviour 1). R98: it also
// never returns a bucket whose To is after the deps clock's Now — an open
// or still-accruing period is neither billed as a number nor marked
// Partial, it is simply absent from the result until it closes (an
// invoice-grade figure is complete or absent, never partial). Writing the
// anomaly row and the operator message is Task 8's ConsumptionAndRecord
// wrapper, on the same type.
func (b *Billing) Consumption(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error) {
	rows, _, err := b.consumptionWithGaps(ctx, sc, req)
	return rows, err
}

// bucketGap is R97's own type: one requested bucket that produced no row,
// and which of its two boundary instants (in "start", "end" order) R96's
// per-instant resolver could not resolve. Window is req.Level's own whole
// bucket, never a clipped one.
type bucketGap struct {
	Window     energy.Window
	Boundaries []string
}

// consumptionWithGaps is Consumption's own implementation, extended (R97) to
// also return each analyzer's gaps: the requested buckets that produced no
// row, alongside which boundary side(s) R96 could not resolve. Consumption
// itself (above) discards the second return value — only
// ConsumptionAndRecord (anomalies.go) needs it, to write one
// missing_readings anomaly per gap. Splitting this way, rather than
// recomputing gaps with a second pass, means ConsumptionAndRecord never
// re-derives a bucket Consumption already derived (R97 spec point 1).
func (b *Billing) consumptionWithGaps(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, map[uuid.UUID][]bucketGap, error) {
	if err := validateRequest(sc, req); err != nil {
		return nil, nil, err
	}

	window := energy.Window{From: req.Range.From, To: req.Range.To}
	buckets := energy.Buckets(req.Level, window, istanbul)
	if len(buckets) == 0 {
		return nil, nil, nil
	}

	var rows []Row
	gapsByAnalyzer := make(map[uuid.UUID][]bucketGap, len(req.AnalyzerIDs))
	for _, analyzerID := range req.AnalyzerIDs {
		// R97 rule 7 / Minor m-3: applyResolvedGapOverrides runs INSIDE
		// consumptionForAnalyzer now, reusing the SAME analyzerBoundaryData
		// already loaded for this analyzer's whole request — never a second
		// query set per overridden gap.
		analyzerRows, gaps, err := b.consumptionForAnalyzer(ctx, sc, analyzerID, req.Level, buckets)
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, analyzerRows...)
		if len(gaps) > 0 {
			gapsByAnalyzer[analyzerID] = gaps
		}
	}
	// C-6: a resolved manual_override/accepted anomaly must be reflected by
	// Consumption itself, not only by Task 8's ConsumptionAndRecord wrapper
	// — applyResolvedAnomalies (anomalies.go) post-processes rows in place.
	if err := b.applyResolvedAnomalies(ctx, sc, rows); err != nil {
		return nil, nil, err
	}
	return rows, gapsByAnalyzer, nil
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
func (b *Billing) consumptionForAnalyzer(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, level energy.Level, buckets []energy.Window) ([]Row, []bucketGap, error) {
	data, err := b.loadAnalyzerBoundaryData(ctx, sc, analyzerID, level, buckets)
	if err != nil {
		return nil, nil, err
	}

	// R103 point 2 (RC-1): "the analyzer has readings" and "pre-installation"
	// are decided from data.hasPriorReading — an UNCLAMPED existence check
	// (firstBoundaryLookback's own BoundaryReadings call, before
	// clampLookback ever runs) — never from the clamped look-back pool
	// (data.loadProfile/billing/daily) alone. A kind whose earliest reading
	// sits further back than this request's own clamp still counts as "the
	// analyzer has readings", and its existence disables the pre-install
	// skip entirely (earliest stays nil below): a reading further back than
	// the request's own window proves every bucket in THIS request is real
	// service, never before the meter was installed.
	anyReadings := data.hasPriorReading ||
		len(data.loadProfile) > 0 ||
		(data.useBilling && len(data.billing) > 0) ||
		(data.useDaily && len(data.daily) > 0)
	now := b.deps.Clock.Now()
	var earliest *time.Time
	if !data.hasPriorReading {
		earliest = earliestBoundaryReading(data)
	}

	rows := make([]Row, 0, len(buckets))
	var gaps []bucketGap
	for _, w := range buckets {
		// R98 (amending/generalising R97 ruling (i)): a bucket that is not
		// yet CLOSED — its own To after the deps clock's now — is dropped
		// before deriveRow ever runs: never a row (open/future consumption
		// is not invoice-grade, even when every boundary happens to
		// resolve), and never a gap either (otherwise every run over the
		// still-accruing current period would write one). closedBucket is
		// the ONE shared predicate both rulings now use, so they can never
		// drift apart again.
		if !closedBucket(w, now) {
			continue
		}
		row, ok := deriveRow(analyzerID, w, data, nil, level)
		if ok {
			rows = append(rows, row)
			continue
		}
		// 02 §3.1: no row, never a zero row. Task 7's plain Consumption
		// never writes missing_readings — that is Task 8's
		// ConsumptionAndRecord, only for a specifically requested billing
		// period, and only when the analyzer has readings at all (above).
		if !anyReadings {
			continue
		}
		// R103 point 1: at Hourly, §3.1 absorbs a missing hour into the
		// NEXT emitted row (the hour's own end boundary simply carries
		// forward to whichever later reading resolves it) — nothing is
		// actually unbillable, so no gap anomaly is ever written and no
		// Hourly missing_readings row can exist after this change.
		if level == energy.Hourly {
			continue
		}
		// R97 ruling (ii): a bucket that ends at or before the analyzer's
		// first-ever boundary reading is pre-installation, not missing.
		if earliest != nil && !w.To.After(*earliest) {
			continue
		}
		boundaries, gapErr := classifyGap(w, data.resolved)
		if gapErr != nil {
			return nil, nil, gapErr
		}
		gaps = append(gaps, bucketGap{Window: w, Boundaries: boundaries})
	}

	if len(gaps) > 0 {
		// R97 rule 7 / RC-1 point 2 (m2-2): the gap-override lookup runs
		// only over `gaps` — the buckets that already passed the
		// anyReadings/pre-install/now filters above. That is fine because,
		// as of RC-1, those filters are themselves range-independent
		// (hasPriorReading and earliest are computed from every kind's own
		// unclamped look-back, not from the loaded/clamped slices), so a
		// bucket that is a recorded gap today can never later fail them —
		// earliest only moves earlier and hasPrior only turns true as more
		// readings are backfilled. The lookup inherits that range
		// independence rather than needing its own separate, wider query.
		// A bucket that gets a row this way is removed from gaps: it is
		// either a row or a gap, never both.
		overrideRows, remaining, oerr := b.applyResolvedGapOverrides(ctx, sc, analyzerID, level, gaps, data)
		if oerr != nil {
			return nil, nil, oerr
		}
		if len(overrideRows) > 0 {
			rows = append(rows, overrideRows...)
			sort.Slice(rows, func(i, j int) bool { return rows[i].Window.From.Before(rows[j].Window.From) })
		}
		gaps = remaining
	}
	return rows, gaps, nil
}

// earliestBoundaryReading returns the earliest ts among every boundary-kind
// slice ALREADY LOADED for this request (R97 spec point 3's own phrasing —
// never a fresh, unbounded query): the minimum of each loaded kind's own
// first element, since every slice is sorted ascending by ts. Only called
// when data.hasPriorReading is false — RC-1: whenever an unclamped look-back
// found a reading before this request's own first bucket, that reading is
// the analyzer's true earliest, and the pre-install skip is disabled
// entirely rather than substituting a within-window reading that would
// wrongly mark real outage days as "before installation".
func earliestBoundaryReading(data analyzerBoundaryData) *time.Time {
	var earliest *time.Time
	consider := func(readings []energy.Reading) {
		if len(readings) == 0 {
			return
		}
		t := readings[0].TS
		if earliest == nil || t.Before(*earliest) {
			earliest = &t
		}
	}
	consider(data.loadProfile)
	if data.useBilling {
		consider(data.billing)
	}
	if data.useDaily {
		consider(data.daily)
	}
	return earliest
}

// applyResolvedGapOverrides implements R97 rule 7: for each of analyzerID's
// gaps, look up a resolved manual_override missing_readings anomaly for
// EXACTLY that bucket and, when one exists, synthesize the row it describes
// — Values from the overrides, Indexes from the end boundary reading (nil
// when there is none), MaxDemandKw from the same maxDemandInWindow helper
// every other row uses, ratios from the overridden values. It returns the
// synthesized rows and the gaps that did NOT resolve to an override (still
// genuine gaps, for the caller to write a missing_readings anomaly for).
//
// Minor m-3: data is the SAME analyzerBoundaryData the caller already loaded
// for the whole request — data.resolved already covers every bucket's own
// boundary instant (R96), so this never re-queries ReadingRepository per
// overridden gap.
func (b *Billing) applyResolvedGapOverrides(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, level energy.Level, gaps []bucketGap, data analyzerBoundaryData) ([]Row, []bucketGap, error) {
	from, to := gaps[0].Window.From, gaps[0].Window.To
	for _, g := range gaps[1:] {
		if g.Window.From.Before(from) {
			from = g.Window.From
		}
		if g.Window.To.After(to) {
			to = g.Window.To
		}
	}

	reason := string(energy.ReasonMissingReadings)
	anomalies, err := b.listAllAnomalies(ctx, sc, store.AnomalyFilter{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Reason:      &reason,
		Range:       &store.TimeRange{From: from, To: to.Add(time.Microsecond)},
	})
	if err != nil {
		return nil, gaps, err
	}

	byPeriod := make(map[anomalyPeriodKey]model.ConsumptionAnomaly, len(anomalies))
	for _, a := range anomalies {
		if a.ResolvedAt == nil || a.Resolution == nil || *a.Resolution != string(ResolveByOverride) {
			continue
		}
		byPeriod[anomalyPeriodKey{a.PeriodStart.UnixNano(), a.PeriodEnd.UnixNano()}] = a
	}
	if len(byPeriod) == 0 {
		return nil, gaps, nil
	}

	var overrideRows []Row
	remaining := make([]bucketGap, 0, len(gaps))
	for _, g := range gaps {
		key := anomalyPeriodKey{g.Window.From.UnixNano(), g.Window.To.UnixNano()}
		a, ok := byPeriod[key]
		if !ok {
			remaining = append(remaining, g)
			continue
		}

		overrides, derr := decodeOverrideValues(a.OverrideValues)
		if derr != nil {
			return nil, gaps, derr
		}
		// Minor m-3: end comes straight from the caller's own already-loaded
		// data.resolved (R96 resolves every bucket instant for the whole
		// request up front) — never a second loadAnalyzerBoundaryData call
		// per overridden gap.
		end := data.resolved[g.Window.To.UnixNano()]

		values := make(map[energy.Register]*decimal.Decimal, len(overrides))
		resolution := make(map[energy.Register]string, len(overrides))
		for reg, v := range overrides {
			val := v
			values[reg] = &val
			resolution[reg] = string(ResolveByOverride)
		}
		row := Row{
			AnalyzerID:  analyzerID,
			Window:      g.Window,
			Values:      values,
			MaxDemandKw: maxDemandInWindow(level, g.Window, data.loadProfile, data.daily, data.maxDemandBilling),
			Resolution:  resolution,
		}
		// R97 rule 7 / Minor m-2: Indexes is the end reading's own values
		// when one exists, else nil — never a map of nil entries.
		if end != nil {
			row.Source = end.Kind
			row.Indexes = endIndexes(end)
		}
		row.InductiveRatio = energy.Ratio(values[energy.ReactiveInductiveImport], values[energy.ActiveImport])
		row.CapacitiveRatio = energy.Ratio(values[energy.ReactiveCapacitiveImport], values[energy.ActiveImport])
		overrideRows = append(overrideRows, row)
	}
	return overrideRows, remaining, nil
}

// closedBucket implements R98: a bucket counts as closed exactly when its
// own To is at or before now — never a strict-before, since a bucket
// closing at exactly this instant has fully accrued. This is the ONE
// predicate consumptionForAnalyzer's loop uses both to decide whether a
// derived row may be emitted at all and whether a missed bucket may be
// recorded as a gap (R97 ruling (i)), so the two rulings can never resolve
// an open/future bucket differently from each other.
func closedBucket(w energy.Window, now time.Time) bool {
	return !w.To.After(now)
}

// classifyGap implements R97 spec point 2: with start/end the resolved
// readings at w's own two boundary instants, names which side(s) a bucket
// that produced no row is missing, in "start" then "end" order. When both
// are non-nil but landed on the exact same reading (R92: a zero-width pair
// never emits), the gap is reported as the end side only — the bucket
// AFTER this one starts fresh from that same reading, so from this bucket's
// own point of view it is the end that never arrived. Any other combination
// that still failed to emit is an internal invariant violation (Derive's
// only "no row" causes are a nil boundary, equal-instant boundaries, or
// reversed boundaries — the last never occurs for a real bucket) and is
// reported as an error rather than silently skipped.
func classifyGap(w energy.Window, resolved map[int64]*energy.Reading) ([]string, error) {
	start := resolved[w.From.UnixNano()]
	end := resolved[w.To.UnixNano()]
	switch {
	case start == nil && end == nil:
		return []string{"start", "end"}, nil
	case start == nil:
		return []string{"start"}, nil
	case end == nil:
		return []string{"end"}, nil
	case start.TS.Equal(end.TS):
		return []string{"end"}, nil
	default:
		return nil, fmt.Errorf("consumption: bucket %s-%s produced no row despite distinct resolved boundaries", w.From, w.To)
	}
}

// analyzerBoundaryData is everything loadAnalyzerBoundaryData loads once per
// (analyzer, request) and resolveBoundaries resolves once per boundary
// instant — extracted so C2's in-memory reset re-derivation
// (rederiveBucketWithReset, resolve.go) can reuse the EXACT same loads and
// the EXACT same per-instant boundary resolution deriveRow uses, rather than
// duplicating either (the copied selection logic R96 made stale is exactly
// what this fix round deletes).
type analyzerBoundaryData struct {
	loadProfile, billing, daily, resets, priors []energy.Reading
	maxDemandBilling                            []energy.Reading
	useBilling, useDaily                        bool
	resolved                                    map[int64]*energy.Reading
	// hasPriorReading is R103's own field (RC-1): true when ANY boundary
	// kind this request may use has a reading at or before the request's
	// own first bucket start — from firstBoundaryLookback's UNCLAMPED
	// BoundaryReadings call, before clampLookback ever runs. It answers
	// "does the analyzer have readings" and "is this pre-installation"
	// range-independently: a reading far enough back to be clamped out of
	// the loaded slices still proves the analyzer was in service before
	// this request's window, which the clamped slices alone cannot tell.
	hasPriorReading bool
}

// deriveRow derives ONE bucket's Row from already-loaded data, optionally
// with extraResets merged in (C2: an operator's proposed reset, checked in
// memory before anything is written). It returns ok=false exactly when
// energy.Derive did not emit (see classifyGap's doc for the exhaustive
// list) — the caller decides what "no row" means for its own purpose
// (Consumption: omit; ConsumptionAndRecord: a gap; ResolveAnomaly: reject).
func deriveRow(analyzerID uuid.UUID, w energy.Window, data analyzerBoundaryData, extraResets []energy.Reading, level energy.Level) (Row, bool) {
	start := data.resolved[w.From.UnixNano()]
	end := data.resolved[w.To.UnixNano()]

	resets := data.resets
	if len(extraResets) > 0 {
		// C2/I7: an injected reset SUPERSEDES any existing reset at the same
		// instant, matching what ReadingRepository.BulkInsert's own upsert
		// would actually produce in the database afterward — never a
		// duplicate reading at one ts, which would make deriveRegister's own
		// before-reset lookup between two same-instant segments
		// unsatisfiable (a spurious meter_reset suspicion with nothing
		// actually wrong).
		extraTS := make(map[int64]bool, len(extraResets))
		for _, e := range extraResets {
			extraTS[e.TS.UnixNano()] = true
		}
		merged := make([]energy.Reading, 0, len(data.resets)+len(extraResets))
		for _, r := range data.resets {
			if extraTS[r.TS.UnixNano()] {
				continue
			}
			merged = append(merged, r)
		}
		merged = append(merged, extraResets...)
		sort.Slice(merged, func(i, j int) bool { return merged[i].TS.Before(merged[j].TS) })
		resets = merged
	}

	derivation := energy.Derive(w, start, end, resets, data.priors)
	if !derivation.Emitted {
		return Row{}, false
	}

	row := Row{
		AnalyzerID: analyzerID,
		Window:     w,
		Values:     derivation.Values,
		Indexes:    endIndexes(end),
		// I-9: only the bucket's own sub-range of each already-sorted
		// source is ever scanned, never the whole loaded set per bucket.
		// R101: kinds are scoped to level (energy.MaxDemandKindsFor), so a
		// coarser kind's peak (daily at Hourly, billing at Hourly/Daily)
		// can never land inside a finer window.
		MaxDemandKw: maxDemandInWindow(level, w, data.loadProfile, data.daily, data.maxDemandBilling),
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
	return row, true
}

// loadAnalyzerBoundaryData implements I-17's query shape for one analyzer:
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
func (b *Billing) loadAnalyzerBoundaryData(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, level energy.Level, buckets []energy.Window) (analyzerBoundaryData, error) {
	first := buckets[0]
	rangeTo := buckets[len(buckets)-1].To

	lpLookback, lpHasPrior, err := b.firstBoundaryLookback(ctx, sc, analyzerID, model.ReadingKindLoadProfile, first)
	if err != nil {
		return analyzerBoundaryData{}, err
	}
	// R103 point 2 (RC-1): hasPriorReading is the OR of every kind's own
	// unclamped existence check, computed here — BEFORE any clampLookback
	// call below ever runs — so a kind's reading older than this request's
	// own clamp still counts.
	hasPriorReading := lpHasPrior
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
		var billingHasPrior bool
		billingLookback, billingHasPrior, err = b.firstBoundaryLookback(ctx, sc, analyzerID, model.ReadingKindBilling, first)
		if err != nil {
			return analyzerBoundaryData{}, err
		}
		hasPriorReading = hasPriorReading || billingHasPrior
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
		var dailyHasPrior bool
		dailyLookback, dailyHasPrior, err = b.firstBoundaryLookback(ctx, sc, analyzerID, model.ReadingKindDaily, first)
		if err != nil {
			return analyzerBoundaryData{}, err
		}
		hasPriorReading = hasPriorReading || dailyHasPrior
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
		return analyzerBoundaryData{}, err
	}
	loadProfile := toEnergyReadings(loadProfileRows)

	// Reset evidence (I-18): fetched once over the same shared minimum
	// look-back as load_profile, since no bucket's evidence window
	// (start.TS, end.TS] can begin before the earliest boundary reading any
	// kind in this request could select.
	resetRows, err := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: minLookback, To: rangeTo.Add(time.Microsecond)}, model.ReadingKindReset)
	if err != nil {
		return analyzerBoundaryData{}, err
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
			return analyzerBoundaryData{}, billingErr
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
			return analyzerBoundaryData{}, dailyErr
		}
		daily = toEnergyReadings(dailyRows)
	} else {
		dailyRows, dailyErr := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: first.From, To: rangeTo}, model.ReadingKindDaily)
		if dailyErr != nil {
			return analyzerBoundaryData{}, dailyErr
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
			return analyzerBoundaryData{}, billingErr
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

	return analyzerBoundaryData{
		loadProfile:      loadProfile,
		billing:          billing,
		daily:            daily,
		resets:           resets,
		priors:           priors,
		maxDemandBilling: maxDemandBilling,
		useBilling:       useBilling,
		useDaily:         useDaily,
		resolved:         resolved,
		hasPriorReading:  hasPriorReading,
	}, nil
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
//
// The second return, hasPrior (R103/RC-1), is this SAME BoundaryReadings
// call's own UNCLAMPED signal: true exactly when a reading of kind exists at
// or before first.From, before the caller's own clampLookback ever runs.
// This is the "cheapest existing call" RC-1 asks for — no extra query is
// added, the boolean is simply read off the call loadAnalyzerBoundaryData
// already makes.
func (b *Billing) firstBoundaryLookback(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, kind model.ReadingKind, first energy.Window) (lookback time.Time, hasPrior bool, err error) {
	startReading, _, err := b.deps.Readings.BoundaryReadings(ctx, sc, analyzerID, kind, first.From, first.To)
	if err != nil {
		return time.Time{}, false, err
	}
	if startReading != nil {
		return startReading.Ts, true, nil
	}
	return first.From, false, nil
}

// maxDemandInWindow implements I-9: each source slice is already sorted
// ascending by ts, so the bucket's own sub-range within each is found with
// sort.Search — O(log n) per source per bucket — instead of rescanning the
// whole loaded set on every bucket (O(buckets x readings), the shape that
// cost 2.78s for one analyzer-year of 15-minute readings at Hourly). Only
// the bounded sub-slice is ever handed to energy.MaxDemandOf.
//
// R101: the kind allowlist is energy.MaxDemandKindsFor(level), not the
// unqualified energy.MaxDemandKinds — a daily row's own day-peak must not
// win inside a single Hourly bucket, and a billing row's own month-peak
// must not win inside a single Hourly or Daily bucket (final review A M-2).
func maxDemandInWindow(level energy.Level, w energy.Window, sources ...[]energy.Reading) *decimal.Decimal {
	kinds := energy.MaxDemandKindsFor(level)
	var max *decimal.Decimal
	for _, src := range sources {
		lo := sort.Search(len(src), func(i int) bool { return !src[i].TS.Before(w.From) })
		hi := sort.Search(len(src), func(i int) bool { return !src[i].TS.Before(w.To) })
		if lo >= hi {
			continue
		}
		if m := energy.MaxDemandOf(w, src[lo:hi], kinds); m != nil && (max == nil || m.GreaterThan(*max)) {
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
