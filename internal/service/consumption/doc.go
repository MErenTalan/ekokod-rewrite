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

// SeriesRequest asks for one or more analyzers' series at one level.
//
// AnalyzerIDs is required and positional: empty means no rows, never "all"
// (R89 — resolving "every analyzer under a building" is F6's job, not
// F3's).
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
// reading. Partial is true for an open (still-in-progress) period Analytics
// composes from consumption_daily (R88) — never set by Billing, which only
// ever reads closed periods. Resolution is populated by Task 8's
// override-substitution logic in billing.go (C-6): a register present here
// was covered by a resolved manual_override anomaly for this window, and
// its Values entry is the operator-supplied figure, not a derived one. Task
// 7 itself always leaves this nil — there is no anomaly-reading logic in
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
// Billing run before any I/O: a valid Scope, a non-empty AnalyzerIDs, a
// valid Range, and a bucket count at Level within MaxBuckets.
func validateRequest(sc store.Scope, req SeriesRequest) error {
	if !sc.Valid() {
		return ErrInvalidRequest
	}
	if len(req.AnalyzerIDs) == 0 {
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
