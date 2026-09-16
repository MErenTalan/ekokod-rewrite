// Package consumption owns the two consumption paths 04-data-model.md §4.3
// requires to be different functions with different guarantees (R61):
//
//   - Analytics.Consumption reads ONLY the continuous aggregates through
//     store.AnalyticsRepository. It is fast and slightly understates a
//     period at the finest levels (it misses the step between the last
//     reading of one bucket and the first of the next), and it never
//     touches meter_readings.
//   - Billing.Consumption reads ONLY meter_readings at true period
//     boundaries through store.ReadingRepository. It is exact, slower, and
//     is the only path that may feed an invoice. A suspect period returns
//     nil values and a Suspicion — never a number (removed-behaviour 1).
//
// There is deliberately no shared Service struct: Analytics and Billing are
// two independent types, each constructed alone. Neither type has a field
// for the other path's repository, so there is nothing to poison in a guard
// test — the type definitions themselves are the proof of separation.
//
// Suspect-period bookkeeping (writing consumption_anomalies rows and
// operator messages) is Task 8's ConsumptionAndRecord wrapper, added later
// on the same *Billing type. This file's Consumption methods never write
// anything; a suspect or missing period is simply omitted from the result
// (Analytics) or returned with a nil Values map and a populated Suspect map
// (Billing) — see Row's doc comment.
package consumption

import (
	"errors"
	"fmt"
	"time"

	// The zone database must be linked into this package's own binary
	// closure: internal/domain/energy already blank-imports time/tzdata,
	// but a package that resolves Europe/Istanbul directly (rather than only
	// ever receiving a *time.Location from a caller) documents that
	// dependency explicitly rather than relying on a transitive import
	// staying in place.
	_ "time/tzdata"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ErrInvalidRequest is this package's fail-closed validation sentinel.
// store.ErrValidation does not exist, so F3 defines its own, here, because
// Analytics and Billing both validate before any I/O.
var ErrInvalidRequest = errors.New("consumption: invalid request")

// ErrConflict is returned when a write would repeat an already-settled fact
// rather than a new one: re-resolving an already-resolved anomaly, or
// registering a reset at a timestamp a reset reading with DIFFERENT values
// already occupies. errors.Is(err, store.ErrConflict) is true, so a
// caller that only checks the store-level sentinel still recognises it.
var ErrConflict = fmt.Errorf("consumption: %w", store.ErrConflict)

// MaxCells bounds a single request's total cell count — len(AnalyzerIDs)
// times the number of buckets at Level over the request's Range — validated
// before any I/O on both consumption paths (R104(2), amending R99).
// MaxBuckets (bucket count alone, no analyzer factor) is unreachable at any
// level once MaxRequestSpan exists (a 9600h/400d span never produces more
// than 9600 Hourly buckets, the worst case), while the product R99 left
// uncapped was still large enough to hurt — one analyzer's own 400-day
// Hourly request retains ~14 MB of []Row, so a legal 50-analyzer request at
// the same span retains on the order of 0.7-1.4 GB before a caller can even
// serialise it. MaxCells is checked with the SAME shared predicate on both
// Billing.Consumption and Analytics.Consumption, before either makes a
// single repository call.
const MaxCells = 50000

// MaxAnalyzersPerRequest bounds AnalyzerIDs (R99): a request naming more
// than this many analyzers fails ErrInvalidRequest before any I/O. Combined
// with MaxRequestSpan below and MaxCells above, this bounds the
// len(AnalyzerIDs) * buckets cell count Analytics and Billing both
// preallocate, so neither can be driven to a multi-GB allocation.
const MaxAnalyzersPerRequest = 50

// MaxRequestSpan bounds Range.To - Range.From at every Level (R99): a
// request whose raw wall-clock span exceeds this fails ErrInvalidRequest
// before any I/O, regardless of how few buckets that span would expand to
// at Level — this is what actually bounds Billing's meter_readings load,
// which is proportional to the span, not to a bucket COUNT alone (a
// 10-year Yearly request is only 10 buckets but loads a decade of
// readings). F6 pages a longer report by year.
const MaxRequestSpan = 400 * 24 * time.Hour

// SettleDelayHourly, SettleDelayDaily and SettleDelayMonthly are R104(1)'s
// amendment to R98: a bucket is closed only once the ordinary ingestion lag
// for its own level's boundary data has had time to land, never merely once
// the clock has passed its own To. R98's clock-only rule would bill a
// daily-only meter's January as 300 instead of the true 310 when queried at
// Feb 1 06:00 — before the Feb 1 00:00 snapshot had arrived, even though it
// was well within R96's own boundary tolerance once it did — and, for the
// same reason, would write a false missing_readings anomaly for a Daily
// bucket in the few hours right after its own midnight.
// Each delay is at least that level's own boundary tolerance (R96's
// DailySnapshotTolerance/BillingSnapshotTolerance/LoadProfileBoundaryTolerance),
// so an in-tolerance late snapshot can still arrive and be used before the
// bucket is ever treated as closed: Hourly 2h (load_profile's own look-back
// is unbounded there, but a normal ingestion cycle is short); Daily 36h
// (DailySnapshotTolerance itself); Monthly and Yearly 72h
// (BillingSnapshotTolerance, the widest tolerance either level ever applies).
const (
	SettleDelayHourly  = 2 * time.Hour
	SettleDelayDaily   = 36 * time.Hour
	SettleDelayMonthly = 72 * time.Hour
	SettleDelayYearly  = 72 * time.Hour
)

// SettleDelay returns R104(1)'s settle delay for level: the ONE function
// closedBucket (billing.go) calls, so a bucket's closedness, whether a
// missing_readings gap may be recorded for it (R97/R103), and whether a
// resolved gap override may be emitted for it (billing.go's
// applyResolvedGapOverrides, which only ever runs over already-filtered
// `gaps`) can never disagree with each other. An unrecognised level returns
// 0, which is fail-OPEN, not fail-closed: closedBucket would treat such a
// bucket as already settled the instant its own To passes, with no extra
// wait at all — the opposite of MaxDemandKindsFor's nil-on-unrecognised
// default, which is fail-closed (no kind matches, so nothing is counted).
// This is safe only because every real caller validates Level (via
// validateRequest/validLevel) before SettleDelay is ever reached; SettleDelay
// itself performs no such check.
func SettleDelay(level energy.Level) time.Duration {
	switch level {
	case energy.Hourly:
		return SettleDelayHourly
	case energy.Daily:
		return SettleDelayDaily
	case energy.Monthly, energy.Yearly:
		return SettleDelayMonthly
	default:
		return 0
	}
}

// BillingSnapshotTolerance bounds R63's "covering": a billing-kind boundary
// reading older than this relative to its own bound does not count as
// covering the month.
const BillingSnapshotTolerance = 72 * time.Hour

// DailySnapshotTolerance bounds R95/R96's daily-kind fallback boundary: a
// daily-kind reading older than this relative to its own bound does not
// count as covering the boundary, exactly as BillingSnapshotTolerance bounds
// the billing-kind fallback for R63. `daily` is a fallback boundary kind at
// Daily, Monthly and Yearly only — never Hourly. 36h admits an end-of-day
// stamp taken at 23:59 or at the next day's 00:00 either way. R96 caps this
// (and every other kind's tolerance) at half the width of the bucket(s)
// meeting at the boundary instant — see boundaryTolerance in billing.go — so
// at Daily level this 36h never actually applies uncapped: a normal 24h day
// caps every kind at 12h.
const DailySnapshotTolerance = 36 * time.Hour

// LoadProfileBoundaryTolerance bounds R96's load_profile boundary at Daily,
// Monthly and Yearly levels: a load_profile reading older than this relative
// to its own bound does not count as a usable boundary candidate there, so a
// stale load_profile reading no longer beats a fresher daily pair (R96: an
// unbounded load_profile look-back at these levels let a stale snapshot win
// over correct, fresh daily-kind evidence). At Hourly level
// load_profile keeps 02 §3.1's original, deliberately unbounded look-back —
// this tolerance is never applied there.
const LoadProfileBoundaryTolerance = 36 * time.Hour

// SeriesRequest asks for one or more analyzers' series at one level.
//
// AnalyzerIDs is required and positional: empty means no rows, never "all"
// (R89 — resolving "every analyzer under a building" is F6's job, not
// F3's).
//
// Multi-analyzer semantics differ between the two paths. One invisible
// (not-Scope-visible, or nonexistent) analyzer id makes Billing fail the
// WHOLE request with store.ErrNotFound (ReadingRepository's per-call
// isolation contract), while Analytics silently omits that analyzer's rows
// and returns the rest (AnalyticsRepository's aggregate queries simply
// produce no rows for an id the Scope cannot see). A caller iterating
// multiple analyzers in one request should not expect the two paths to fail
// or partially-succeed the same way.
type SeriesRequest struct {
	AnalyzerIDs []uuid.UUID
	Level       energy.Level
	Range       store.TimeRange
}

// Row is one period. Values carries every register's consumption; a nil
// entry is unreported or suspect. Source records which reading kind
// produced the row (R62) — energy.Kind, not a bare string, so an invalid
// value cannot compile. Indexes carries the register's CLOSING index for
// the period (05 §5 "every index field", Task 9): Analytics fills it from
// the bucket's own closing index columns, Billing from the end boundary
// reading — a composed Analytics row (below) only ever has the seven
// registers migration 00005 gives consumption_daily a closing-index column
// to: the other five (every export register but active_export) are
// always nil there, exactly as they are absent from bucketIndexes' map.
// Partial is true for every bucket Analytics composes from consumption_daily
// at Monthly/Yearly (R94, amending R88) — the still-open trailing period AND
// any earlier closed-but-unrefreshed one — never set by Billing: R98 drops
// every bucket whose To is after BillingDeps.Clock's Now before it can ever
// become a row, so Billing has no open period left to mark Partial in the
// first place; it must never silently emit an open period as if it were a
// closed, complete one. A composed figure is closing-to-closing across
// whatever materialisation gap it fills and is NOT directly comparable to
// the later materialised row for the same window — it can differ by the
// boundary step the materialised aggregate itself measures differently once
// the underlying data is refreshed. Resolution is populated by Task 8's
// override-substitution logic in billing.go: a register present here was
// covered by a resolved manual_override anomaly for this window, and its
// Values entry is the operator-supplied figure, not a derived one. Task 7
// itself always leaves this nil — there is no anomaly-reading logic in
// Task 7's own Consumption.
type Row struct {
	AnalyzerID      uuid.UUID
	Window          energy.Window
	Values          map[energy.Register]*decimal.Decimal
	Indexes         map[energy.Register]*decimal.Decimal
	InductiveRatio  *decimal.Decimal
	CapacitiveRatio *decimal.Decimal
	MaxDemandKw     *decimal.Decimal
	Source          energy.Kind
	Suspect         map[energy.Register]energy.Suspicion
	Partial         bool
	Resolution      map[energy.Register]string
}

// istanbul is the one *time.Location every bucket boundary in this package
// is computed against (Global Constraints: "evaluated in Europe/Istanbul").
// It is resolved once, at package init, from the embedded tzdata copy this
// file blank-imports, so it can never fail at request time.
var istanbul = mustLoadIstanbul()

func mustLoadIstanbul() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		// Unreachable in practice: time/tzdata is blank-imported above
		// specifically so this lookup always succeeds, with no dependency
		// on the host's system tzdata. A panic here is a build-time defect
		// (a missing blank import), not a runtime condition to recover
		// from.
		panic(fmt.Sprintf("consumption: cannot load Europe/Istanbul: %v", err))
	}
	return loc
}

// validateRequest applies the shared fail-closed checks both Analytics and
// Billing run before any I/O: a valid Scope, a non-empty, duplicate-free
// AnalyzerIDs no longer than MaxAnalyzersPerRequest, a valid Level, a valid
// Range no wider than MaxRequestSpan, and a cell count (AnalyzerIDs times
// the bucket count at Level) within MaxCells (R99/R104(2): every check
// below runs before either path makes a single repository call).
func validateRequest(sc store.Scope, req SeriesRequest) error {
	if !sc.Valid() {
		return ErrInvalidRequest
	}
	if len(req.AnalyzerIDs) == 0 {
		return ErrInvalidRequest
	}
	if len(req.AnalyzerIDs) > MaxAnalyzersPerRequest {
		return ErrInvalidRequest
	}
	if hasDuplicateAnalyzerID(req.AnalyzerIDs) {
		return ErrInvalidRequest
	}
	if !validLevel(req.Level) {
		return ErrInvalidRequest
	}
	if !req.Range.Valid() {
		return ErrInvalidRequest
	}
	if req.Range.To.Sub(req.Range.From) > MaxRequestSpan {
		return ErrInvalidRequest
	}
	w := energy.Window{From: req.Range.From, To: req.Range.To}
	if cellsExceed(req.Level, w, istanbul, len(req.AnalyzerIDs), MaxCells) {
		return ErrInvalidRequest
	}
	return nil
}

// hasDuplicateAnalyzerID reports whether ids contains the same uuid.UUID
// twice (R99): a repeated id would otherwise silently double every total
// both paths compute for it, since neither path deduplicates AnalyzerIDs
// itself.
func hasDuplicateAnalyzerID(ids []uuid.UUID) bool {
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

// validLevel reports whether l is one of the four levels 02 §3.4 defines.
// Without this check an unrecognised Level passed validateRequest
// silently (cellsExceed reports "not exceeded" for a level Bucket does not
// recognise, since Bucket returns the invalid zero Window and the counting
// loop never starts), and both Consumption methods then returned
// (nil, nil) instead of ErrInvalidRequest — fetchBuckets' own default
// branch was unreachable in practice because validateRequest ran first.
func validLevel(l energy.Level) bool {
	switch l {
	case energy.Hourly, energy.Daily, energy.Monthly, energy.Yearly:
		return true
	default:
		return false
	}
}

// errRequired formats the missing-dependency error both NewAnalytics and
// NewBilling return, mirroring internal/credentials.New's Deps validation
// style.
func errRequired(field string) error {
	return fmt.Errorf("consumption: %s is required", field)
}

// cellsExceed reports whether analyzerCount times Level's bucket count over
// w would exceed max (R104(2), replacing the bucket-count-only MaxBuckets
// cap, which is unreachable under MaxRequestSpan), WITHOUT materialising
// energy.Buckets' full slice — the same stepping logic as
// energy.Buckets, but counting only, so that a pathological request (many
// analyzers over a long fine-grained range) is rejected in a bounded number
// of steps instead of allocating a result the caller was always going to
// refuse. An invalid window, an unrecognised level, or a non-positive
// analyzerCount reports false (not exceeded): those are caught by
// validateRequest's other checks first.
func cellsExceed(level energy.Level, w energy.Window, loc *time.Location, analyzerCount int, max int) bool {
	if analyzerCount <= 0 {
		return false
	}
	if !w.Valid() {
		return false
	}
	cur := energy.Bucket(level, w.From, loc)
	if !cur.Valid() {
		return false
	}
	count := 0
	for cur.From.Before(w.To) {
		count++
		if count*analyzerCount > max {
			return true
		}
		next := energy.Bucket(level, cur.To, loc)
		if !next.From.After(cur.From) {
			break
		}
		cur = next
	}
	return false
}
