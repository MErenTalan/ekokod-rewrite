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
// store.ErrValidation does not exist (pre-flight finding): F3 defines its
// own, here, because Analytics and Billing both validate before any I/O.
var ErrInvalidRequest = errors.New("consumption: invalid request")

// MaxBuckets bounds a single request's bucket count at its Level: a request
// whose [Range.From, Range.To) would expand past this many buckets fails
// ErrInvalidRequest before any I/O (I-17) — a runaway range is a client
// error, not a slow query.
const MaxBuckets = 10000

// BillingSnapshotTolerance bounds R63's "covering": a billing-kind boundary
// reading older than this relative to its own bound does not count as
// covering the month (I-3).
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
// stale load_profile reading no longer beats a fresher daily pair (R96's K3
// finding: an unbounded load_profile look-back at these levels let a stale
// snapshot win over correct, fresh daily-kind evidence). At Hourly level
// load_profile keeps 02 §3.1's original, deliberately unbounded look-back —
// this tolerance is never applied there.
const LoadProfileBoundaryTolerance = 36 * time.Hour

// SeriesRequest asks for one or more analyzers' series at one level.
//
// AnalyzerIDs is required and positional: empty means no rows, never "all"
// (R89 — resolving "every analyzer under a building" is F6's job, not
// F3's).
//
// M3: multi-analyzer semantics differ between the two paths. One invisible
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
// to (M-2): the other five (every export register but active_export) are
// always nil there, exactly as they are absent from bucketIndexes' map.
// Partial is true for every bucket Analytics composes from consumption_daily
// at Monthly/Yearly (R94, amending R88) — the still-open trailing period AND
// any earlier closed-but-unrefreshed one — never set by Billing, which only
// ever reads closed periods. M-4: a composed figure is closing-to-closing
// across whatever materialisation gap it fills and is NOT directly
// comparable to the later materialised row for the same window — it can
// differ by the boundary step the materialised aggregate itself measures
// differently once the underlying data is refreshed. Resolution is
// populated by Task 8's override-substitution logic in billing.go (C-6): a
// register present here was covered by a resolved manual_override anomaly
// for this window, and its Values entry is the operator-supplied figure,
// not a derived one. Task 7 itself always leaves this nil — there is no
// anomaly-reading logic in Task 7's own Consumption.
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
// Billing run before any I/O: a valid Scope, a non-empty AnalyzerIDs, a
// valid Level, a valid Range, and a bucket count at Level within MaxBuckets.
func validateRequest(sc store.Scope, req SeriesRequest) error {
	if !sc.Valid() {
		return ErrInvalidRequest
	}
	if len(req.AnalyzerIDs) == 0 {
		return ErrInvalidRequest
	}
	if !validLevel(req.Level) {
		return ErrInvalidRequest
	}
	if !req.Range.Valid() {
		return ErrInvalidRequest
	}
	w := energy.Window{From: req.Range.From, To: req.Range.To}
	if bucketCountExceeds(req.Level, w, istanbul, MaxBuckets) {
		return ErrInvalidRequest
	}
	return nil
}

// validLevel reports whether l is one of the four levels 02 §3.4 defines
// (M-1). Without this check an unrecognised Level passed validateRequest
// silently (bucketCountExceeds reports "not exceeded" for a level Bucket
// does not recognise, since Bucket returns the invalid zero Window and the
// counting loop never starts), and both Consumption methods then returned
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

// bucketCountExceeds reports whether Level's bucket count over w would
// exceed max, WITHOUT materialising energy.Buckets' full slice — the same
// stepping logic as energy.Buckets, but counting only, so that a
// pathological request (a multi-century hourly range) is rejected in O(max)
// steps instead of allocating a result the caller was always going to
// refuse. An invalid window or an unrecognised level reports false (not
// exceeded): those are caught by validateRequest's other checks first.
func bucketCountExceeds(level energy.Level, w energy.Window, loc *time.Location, max int) bool {
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
		if count > max {
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
