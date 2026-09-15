package ingest

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// RejectReason names why Dedupe or Validate refused a candidate reading.
type RejectReason string

// The RejectReason values. Every rejected row (Dedupe's conflicting
// duplicates, Validate's own checks) carries exactly one of these.
const (
	// RejectUnparseableTs is a zero Ts after normalisation — the adapter
	// could not parse the provider's timestamp.
	RejectUnparseableTs RejectReason = "unparseable_timestamp"
	// RejectFuture is a Ts more than Options.FutureTolerance past now.
	RejectFuture RejectReason = "future_timestamp"
	// RejectAllNull is a row with every register nil.
	RejectAllNull RejectReason = "all_registers_null"
	// RejectNegative is a row with any register below zero.
	RejectNegative RejectReason = "negative_value"
	// RejectSanityJump is a register jump beyond Options.SanityMultiple times
	// the point's typical interval consumption. A RejectSanityJump entry
	// with Register "sustained" is NOT an exclusion — see Validate's doc.
	RejectSanityJump RejectReason = "sanity_jump"
	// RejectConflictingDuplicate is D1: two rows sharing (AnalyzerID, Ts,
	// Kind) with different register values. Dedupe refuses the whole group.
	RejectConflictingDuplicate RejectReason = "conflicting_duplicate"
)

// Rejection records one candidate reading Dedupe or Validate declined to
// keep, or — for the RejectSanityJump/"sustained" case only — a reading that
// WAS kept but is flagged for an operator (see Validate).
type Rejection struct {
	Ts     time.Time
	Kind   model.ReadingKind
	Reason RejectReason
	// Register names the offending register for RejectNegative and
	// RejectSanityJump. It is "sustained" for the fourth-consecutive-jump
	// flag (Validate) and empty for reasons that are not register-specific.
	Register string
}

// registerAccessor names one register field of a MeterReading and how to
// read it, so the AllNull/Negative/sanity-jump checks and Dedupe's equality
// check can all walk the same list instead of repeating fourteen field
// names four times over.
type registerAccessor struct {
	name string
	get  func(model.MeterReading) *decimal.Decimal
}

// allRegisters is every register AllNull and Negative validate against: the
// twelve cumulative import/export registers plus the two interval values
// (MaxDemandKw, IntervalGenerationKwh) — 06 §9's "every register is null"
// and "a register value is negative" apply to all fourteen.
var allRegisters = []registerAccessor{
	{"active_import", func(r model.MeterReading) *decimal.Decimal { return r.ActiveImport }},
	{"reactive_inductive_import", func(r model.MeterReading) *decimal.Decimal { return r.ReactiveInductiveImport }},
	{"reactive_capacitive_import", func(r model.MeterReading) *decimal.Decimal { return r.ReactiveCapacitiveImport }},
	{"t1_import", func(r model.MeterReading) *decimal.Decimal { return r.T1Import }},
	{"t2_import", func(r model.MeterReading) *decimal.Decimal { return r.T2Import }},
	{"t3_import", func(r model.MeterReading) *decimal.Decimal { return r.T3Import }},
	{"active_export", func(r model.MeterReading) *decimal.Decimal { return r.ActiveExport }},
	{"reactive_inductive_export", func(r model.MeterReading) *decimal.Decimal { return r.ReactiveInductiveExport }},
	{"reactive_capacitive_export", func(r model.MeterReading) *decimal.Decimal { return r.ReactiveCapacitiveExport }},
	{"t1_export", func(r model.MeterReading) *decimal.Decimal { return r.T1Export }},
	{"t2_export", func(r model.MeterReading) *decimal.Decimal { return r.T2Export }},
	{"t3_export", func(r model.MeterReading) *decimal.Decimal { return r.T3Export }},
	{"max_demand_kw", func(r model.MeterReading) *decimal.Decimal { return r.MaxDemandKw }},
	{"interval_generation_kwh", func(r model.MeterReading) *decimal.Decimal { return r.IntervalGenerationKwh }},
}

// cumulativeRegisters is the twelve import/export index registers the
// sanity-jump check (R13) and DetectNegativeDeltas (R14) compare across
// consecutive readings. MaxDemandKw is an interval maximum, not cumulative
// (02-domain-rules.md §3.5), and IntervalGenerationKwh is already an
// interval value (06 §5, R1); neither has a meaningful "delta since the last
// reading", so both are excluded here — allRegisters above is only for the
// AllNull/Negative checks, which apply to every register regardless of
// shape.
var cumulativeRegisters = allRegisters[:12]

// sanityMinHistorySamples is the fewest register deltas a 7-day history must
// contain before the sanity-jump check trusts the median it computes from
// them. Below this, Validate skips the check entirely for that register
// rather than compare against a median built from too little data to be
// meaningful.
const sanityMinHistorySamples = 30

// sanityMaxConsecutiveJumps is how many consecutive candidate rows Validate
// rejects as RejectSanityJump before concluding the "jump" is a genuine,
// sustained shift (a multiplier correction, a meter replacement) rather than
// corrupt data, and accepting the row that would have been the fourth
// rejection in a row — see Validate's doc.
const sanityMaxConsecutiveJumps = 3

// Validate applies 06 §9 / R11-R14 to rows, in order, against prev (the last
// accepted reading before this batch) and history (readings from the 7 days
// before the window, used only to size the sanity-jump threshold).
//
// R11: the commissioning-date rule is NOT applied — a reading is never
// rejected for being "too old".
//
// Checks run in this order for each row: zero Ts (RejectUnparseableTs), Ts
// beyond now+FutureTolerance (RejectFuture), every register nil
// (RejectAllNull), any register negative (RejectNegative), then the
// sanity-jump check (R13). A row rejected by any of the first four checks
// never reaches the sanity-jump check and never updates the "effective
// previous reading" the jump check compares against.
//
// Sanity jump (R13): for each of cumulativeRegisters, the threshold is
// history's own median consecutive delta for that register, times
// o.SanityMultiple — but only when history contains at least
// sanityMinHistorySamples deltas for that register; with fewer, the check is
// skipped (every row passes) because a median from too little data is not
// trustworthy. A row whose delta from the effective previous reading exceeds
// the threshold on any register is a "jump". The first
// sanityMaxConsecutiveJumps (3) consecutive jumps are rejected
// (RejectSanityJump, Register = the first offending register found). A
// FOURTH consecutive jump is treated as a genuine sustained shift rather
// than corruption: the row IS kept (appended to valid, and becomes the new
// effective previous reading), but a Rejection{Reason: RejectSanityJump,
// Register: "sustained"} is still appended to rejected, purely as an
// operator flag — Service turns it into an `error` operational message
// rather than folding it into the ordinary rejection-count warning. The
// streak resets after the fourth row, whether it is a further jump or not.
func Validate(prev *model.MeterReading, history []model.MeterReading, rows []model.MeterReading, now time.Time, o Options) (valid []model.MeterReading, rejected []Rejection) {
	thresholds := sanityThresholds(history, o.SanityMultiple)

	var effectivePrev *model.MeterReading
	if prev != nil {
		p := *prev
		effectivePrev = &p
	}
	consecutiveJumps := 0

	for _, row := range rows {
		switch {
		case row.Ts.IsZero():
			rejected = append(rejected, Rejection{Ts: row.Ts, Kind: row.Kind, Reason: RejectUnparseableTs})
			continue
		case row.Ts.After(now.Add(o.FutureTolerance)):
			rejected = append(rejected, Rejection{Ts: row.Ts, Kind: row.Kind, Reason: RejectFuture})
			continue
		case allRegistersNil(row):
			rejected = append(rejected, Rejection{Ts: row.Ts, Kind: row.Kind, Reason: RejectAllNull})
			continue
		}
		if reg, ok := anyRegisterNegative(row); ok {
			rejected = append(rejected, Rejection{Ts: row.Ts, Kind: row.Kind, Reason: RejectNegative, Register: reg})
			continue
		}

		if reg, jumped := isSanityJump(effectivePrev, row, thresholds); jumped {
			consecutiveJumps++
			if consecutiveJumps <= sanityMaxConsecutiveJumps {
				rejected = append(rejected, Rejection{Ts: row.Ts, Kind: row.Kind, Reason: RejectSanityJump, Register: reg})
				continue
			}
			// Fourth consecutive jump: accept it as a sustained shift, flag
			// it, and reset the streak.
			rejected = append(rejected, Rejection{Ts: row.Ts, Kind: row.Kind, Reason: RejectSanityJump, Register: "sustained"})
			consecutiveJumps = 0
		} else {
			consecutiveJumps = 0
		}

		valid = append(valid, row)
		r := row
		effectivePrev = &r
	}
	return valid, rejected
}

func allRegistersNil(row model.MeterReading) bool {
	for _, reg := range allRegisters {
		if reg.get(row) != nil {
			return false
		}
	}
	return true
}

func anyRegisterNegative(row model.MeterReading) (register string, negative bool) {
	for _, reg := range allRegisters {
		if v := reg.get(row); v != nil && v.IsNegative() {
			return reg.name, true
		}
	}
	return "", false
}

// sanityThresholds computes, for each of cumulativeRegisters, median(delta)
// * multiple — or reports ok=false when history has fewer than
// sanityMinHistorySamples deltas for that register.
type sanityThreshold struct {
	value decimal.Decimal
	ok    bool
}

func sanityThresholds(history []model.MeterReading, multiple decimal.Decimal) map[string]sanityThreshold {
	out := make(map[string]sanityThreshold, len(cumulativeRegisters))
	for _, reg := range cumulativeRegisters {
		deltas := registerDeltas(history, reg)
		if len(deltas) < sanityMinHistorySamples {
			out[reg.name] = sanityThreshold{ok: false}
			continue
		}
		out[reg.name] = sanityThreshold{value: median(deltas).Mul(multiple), ok: true}
	}
	return out
}

// registerDeltas walks history in order and returns the consecutive delta
// for reg wherever both readings carry a non-nil value for it.
func registerDeltas(history []model.MeterReading, reg registerAccessor) []decimal.Decimal {
	var deltas []decimal.Decimal
	var prevVal *decimal.Decimal
	for _, r := range history {
		v := reg.get(r)
		if v != nil && prevVal != nil {
			deltas = append(deltas, v.Sub(*prevVal).Abs())
		}
		if v != nil {
			prevVal = v
		}
	}
	return deltas
}

func median(vals []decimal.Decimal) decimal.Decimal {
	sorted := make([]decimal.Decimal, len(vals))
	copy(sorted, vals)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].LessThan(sorted[j]) })
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return sorted[n/2-1].Add(sorted[n/2]).Div(decimal.NewFromInt(2))
}

// isSanityJump reports whether row jumps on any cumulativeRegisters register
// relative to prev, using thresholds. Only the first offending register is
// named — Validate rejects the whole row on the first violation found, and
// walking cumulativeRegisters in a fixed order makes which one is named
// deterministic.
func isSanityJump(prev *model.MeterReading, row model.MeterReading, thresholds map[string]sanityThreshold) (register string, jumped bool) {
	if prev == nil {
		return "", false
	}
	for _, reg := range cumulativeRegisters {
		th, ok := thresholds[reg.name]
		if !ok || !th.ok {
			continue
		}
		pv, cv := reg.get(*prev), reg.get(row)
		if pv == nil || cv == nil {
			continue
		}
		delta := cv.Sub(*pv).Abs()
		if delta.GreaterThan(th.value) {
			return reg.name, true
		}
	}
	return "", false
}
