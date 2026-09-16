package energy

import (
	"time"

	"github.com/shopspring/decimal"
)

// Register names one cumulative register on a meter reading (02 §2.1).
type Register string

// The twelve cumulative registers a meter reading may carry (02 §2.1).
const (
	ActiveImport             Register = "active_import"
	ReactiveInductiveImport  Register = "reactive_inductive_import"
	ReactiveCapacitiveImport Register = "reactive_capacitive_import"
	T1Import                 Register = "t1_import"
	T2Import                 Register = "t2_import"
	T3Import                 Register = "t3_import"
	ActiveExport             Register = "active_export"
	ReactiveInductiveExport  Register = "reactive_inductive_export"
	ReactiveCapacitiveExport Register = "reactive_capacitive_export"
	T1Export                 Register = "t1_export"
	T2Export                 Register = "t2_export"
	T3Export                 Register = "t3_export"
)

// allRegisters is the single canonical order every exported iteration below
// uses. max_demand is deliberately absent: it is not a cumulative register
// (02 §3.5) and is carried on Reading.MaxDemandKw instead.
var allRegisters = []Register{
	ActiveImport, ReactiveInductiveImport, ReactiveCapacitiveImport,
	T1Import, T2Import, T3Import,
	ActiveExport, ReactiveInductiveExport, ReactiveCapacitiveExport,
	T1Export, T2Export, T3Export,
}

// AllRegisters is every register in a stable order. Iteration order of a map
// is never relied on: every exported result that ranges over registers uses
// this. The returned slice is a fresh copy; callers may not mutate the
// package's canonical order through it.
func AllRegisters() []Register {
	out := make([]Register, len(allRegisters))
	copy(out, allRegisters)
	return out
}

// Kind mirrors model.ReadingKind (04 §4.1). current_index never takes part in
// §3.1 differencing (R64); reset rows are evidence, not boundaries.
type Kind string

// The five reading classes (02 §2.3).
const (
	KindLoadProfile  Kind = "load_profile"
	KindDaily        Kind = "daily"
	KindBilling      Kind = "billing"
	KindReset        Kind = "reset"
	KindCurrentIndex Kind = "current_index"
)

// Reading is one timestamped snapshot. A nil register value means the meter
// did not report it; it is never treated as zero (removed-behaviour 21).
type Reading struct {
	TS          time.Time
	Kind        Kind
	Values      map[Register]*decimal.Decimal
	MaxDemandKw *decimal.Decimal
}

// Value returns the register's value, or nil when absent.
func (r *Reading) Value(reg Register) *decimal.Decimal {
	if r == nil {
		return nil
	}
	return r.Values[reg]
}

// Level is an aggregation level (02 §3.4).
type Level string

// The four aggregation levels (02 §3.4).
const (
	Hourly  Level = "hourly"
	Daily   Level = "daily"
	Monthly Level = "monthly"
	Yearly  Level = "yearly"
)

// Window is half-open: [From, To).
type Window struct{ From, To time.Time }

// Valid reports whether the window is well formed: From strictly before To.
func (w Window) Valid() bool {
	return w.From.Before(w.To)
}

// Contains reports whether ts falls in [From, To).
func (w Window) Contains(ts time.Time) bool {
	return !ts.Before(w.From) && ts.Before(w.To)
}

// Suspicion records why a register could not be derived (02 §3.2, R58-R60).
type Suspicion struct {
	Reason    Reason
	Delta     *decimal.Decimal // the negative difference that triggered it, when known
	ResetRows int              // reset rows found inside the window
}

// Reason names why a register's difference could not be derived (02 §3.2).
type Reason string

// The three reasons a register can be left suspect (02 §3.2, R58-R60).
const (
	ReasonNegativeDelta   Reason = "negative_delta"
	ReasonMeterReset      Reason = "meter_reset"
	ReasonMissingReadings Reason = "missing_readings"
)

// Derivation is the result for one window.
//
// Emitted is false for §3.1's "no row" cases: a missing boundary reading, or
// a start and end that are the same reading. A caller must not turn
// !Emitted into a zero row.
type Derivation struct {
	Window  Window
	Emitted bool
	Source  Kind
	Values  map[Register]*decimal.Decimal // nil entry = suspect or unreported
	Suspect map[Register]Suspicion
}

// SuspectRegisters returns the suspect registers in AllRegisters order.
func (d Derivation) SuspectRegisters() []Register {
	out := make([]Register, 0, len(d.Suspect))
	for _, reg := range allRegisters {
		if _, ok := d.Suspect[reg]; ok {
			out = append(out, reg)
		}
	}
	return out
}
