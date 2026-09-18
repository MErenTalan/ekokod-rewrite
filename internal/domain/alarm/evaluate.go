package alarm

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// divisionScale matches internal/domain/reactive: every ratio is a division,
// and a division needs a declared scale to be reproducible.
const divisionScale int32 = 20

var hundred = decimal.NewFromInt(100)

// Breach is one threshold that was crossed.
type Breach struct {
	// Field is the setting that was crossed (FieldInductive, FieldPowerMax, …).
	Field string
	// Measured and Threshold are exact.
	Measured, Threshold decimal.Decimal
	// Window is the period measured; the zero value means a point-in-time check.
	Window Window
	// Extra carries the type's own context: period_key for an invoice breach,
	// last_reading_at for a communication breach.
	Extra map[string]string
}

// Verdict is one analyzer's outcome for one rule.
type Verdict struct {
	AnalyzerID uuid.UUID
	Breaches   []Breach
	// NoVerdict lists the settings that could not be decided for lack of data
	// (R214, R215, R216, R217). It is NOT a breach and never notifies — the
	// difference between "compliant" and "we do not know" is the whole point.
	NoVerdict []string
}

// Fired reports whether the verdict breached anything.
func (v Verdict) Fired() bool { return len(v.Breaches) > 0 }

// ReactiveInput is one analyzer's readings per configured group. Each slice
// holds the readings inside that group's own window, ascending by Ts, because
// each group may be measured over a different period.
type ReactiveInput struct {
	AnalyzerID uuid.UUID

	Inductive, Capacitive, ActiveMax, ActiveMin []model.MeterReading

	// Windows is the window each group was loaded for, keyed by field name, so
	// a breach can say what it measured. A missing entry yields a zero Window.
	Windows map[string]Window
}

// register picks one cumulative register off a reading.
type register func(model.MeterReading) *decimal.Decimal

func activeImport(r model.MeterReading) *decimal.Decimal    { return r.ActiveImport }
func inductiveImport(r model.MeterReading) *decimal.Decimal { return r.ReactiveInductiveImport }
func capacitiveImport(r model.MeterReading) *decimal.Decimal {
	return r.ReactiveCapacitiveImport
}

// diff returns last−first for one register, or ok=false when there are fewer
// than two readings or either end is missing. A nil register is UNKNOWN, never
// zero: reading it as zero would turn "no data" into "compliant".
func diff(rows []model.MeterReading, pick register) (decimal.Decimal, bool) {
	if len(rows) < 2 {
		return decimal.Zero, false
	}
	first, last := pick(rows[0]), pick(rows[len(rows)-1])
	if first == nil || last == nil {
		return decimal.Zero, false
	}
	return last.Sub(*first), true
}

// EvaluateReactive applies the four reactive groups (R215): first and last
// reading in each group's window, as legacy did, so a rebuilt history matches.
func EvaluateReactive(a model.Alarm, in ReactiveInput) Verdict {
	v := Verdict{AnalyzerID: in.AnalyzerID}
	gs := groups(a)

	rows := map[string][]model.MeterReading{
		FieldInductive:  in.Inductive,
		FieldCapacitive: in.Capacitive,
		FieldActiveMax:  in.ActiveMax,
		FieldActiveMin:  in.ActiveMin,
	}
	regs := map[string]register{
		FieldInductive:  inductiveImport,
		FieldCapacitive: capacitiveImport,
	}

	// The order is fixed rather than a map range, so NoVerdict and Breaches are
	// deterministic and a test can assert on them.
	for _, field := range []string{FieldInductive, FieldCapacitive} {
		v.ratio(field, gs[field], rows[field], regs[field], in.Windows[field])
	}
	v.bound(FieldActiveMax, gs[FieldActiveMax], rows[FieldActiveMax], true, in.Windows[FieldActiveMax])
	v.bound(FieldActiveMin, gs[FieldActiveMin], rows[FieldActiveMin], false, in.Windows[FieldActiveMin])
	return v
}

// ratio is one reactive ratio group: (reactive_diff / active_diff) × 100
// against the threshold. A non-positive active difference yields 0, which is
// legacy's own behaviour and is kept deliberately (R215).
func (v *Verdict) ratio(field string, g group, rows []model.MeterReading, pick register, w Window) {
	if g.threshold == nil {
		return
	}
	reactiveDiff, okReactive := diff(rows, pick)
	activeDiff, okActive := diff(rows, activeImport)
	if !okReactive || !okActive {
		v.NoVerdict = append(v.NoVerdict, field)
		return
	}
	pct := decimal.Zero
	if activeDiff.IsPositive() {
		pct = reactiveDiff.DivRound(activeDiff, divisionScale).Mul(hundred)
	}
	if pct.GreaterThan(*g.threshold) {
		v.Breaches = append(v.Breaches, Breach{Field: field, Measured: pct, Threshold: *g.threshold, Window: w})
	}
}

// bound is an active-consumption maximum (over=true) or minimum.
func (v *Verdict) bound(field string, g group, rows []model.MeterReading, over bool, w Window) {
	if g.threshold == nil {
		return
	}
	d, ok := diff(rows, activeImport)
	if !ok {
		v.NoVerdict = append(v.NoVerdict, field)
		return
	}
	if (over && d.GreaterThan(*g.threshold)) || (!over && d.LessThan(*g.threshold)) {
		v.Breaches = append(v.Breaches, Breach{Field: field, Measured: d, Threshold: *g.threshold, Window: w})
	}
}
