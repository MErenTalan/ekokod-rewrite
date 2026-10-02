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
	// RejectWrongAnalyzer is I2: a row an adapter returned in response to
	// THIS request but stamped with a different AnalyzerID. It never
	// reaches Validate, BulkInsert, or this analyzer's cursor.
	RejectWrongAnalyzer RejectReason = "wrong_analyzer"
	// RejectWrongKind is I2: a row whose Kind is neither the requested
	// p.Kind nor model.ReadingKindReset — the one other kind the fetch loop
	// expects mixed into a p.Kind page, for negative-delta suppression.
	RejectWrongKind RejectReason = "wrong_kind"
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

// sanityMinHistorySamples is the fewest POSITIVE register deltas a 7-day
// history must contain before the sanity-jump check (R13) trusts what it
// computes from them. Below this, Validate skips the check entirely for that
// register. R13 pins this at 24.
const sanityMinHistorySamples = 24

// sanityMaxConsecutiveJumps is how many consecutive candidate rows Validate
// rejects as RejectSanityJump, per analyzer per run (SanityStreak), before
// concluding the "jump" is a genuine, sustained shift (a multiplier
// correction, a meter replacement) rather than corrupt data — see
// SanityStreak's and Validate's docs.
const sanityMaxConsecutiveJumps = 3

// SanityStreak carries R13's "at most three consecutive [sanity-jump]
// rejections per analyzer per run" state across every Validate call one
// FetchReadings run makes — a run pages through possibly many chunks, and
// Validate itself is a pure function with no memory of its own between
// calls. Service creates exactly one SanityStreak per run and threads the
// same pointer, via Options.SanityStreak, through every Validate call that
// run makes for its one analyzer and kind.
//
// Once the streak trips (a fourth consecutive jump), it stays tripped for
// the rest of the run: every later row passes the sanity check
// unconditionally, matching R13's own text ("readings are accepted... so a
// genuine step never silently discards data forever"). The streak never
// re-arms mid-run — it cannot cycle reject-3-accept-1-reject-3... forever
// against a sustained step change.
//
// A caller that leaves Options.SanityStreak nil (every existing unit test,
// and any other single-page caller) gets a streak scoped to that one
// Validate call only: three consecutive rejections then a flagged,
// accepted fourth, exactly as before — but nothing carries over to a
// second call, which is the per-run behaviour only Service needs.
type SanityStreak struct {
	consecutive int
	tripped     bool
	// held are the readings rejected while the current streak is in doubt
	// (R509): a streak that proves sustained gives them back, one that ends
	// with an ordinary reading drops them for good.
	held []model.MeterReading
}

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
// Sanity jump (R13, amended by R509): for each of cumulativeRegisters,
// "typical" is the LARGEST of that register's positive consecutive deltas in
// history (zero and negative deltas are idle or reversing periods, not a rate
// of consumption), computed only when history holds at least
// sanityMinHistorySamples (24) such deltas; with fewer, the check is skipped
// for that register. The threshold scales with elapsed time:
// elapsedIntervals = (row.Ts − prev.Ts) ÷ history's own median spacing, and
// the limit is max(o.SanityMultiple × largest × elapsedIntervals, 1), so a
// longer gap widens the allowance. Only an INCREASE beyond that limit is a
// "jump": a decrease is R14's concern, and the reading is stored either way.
//
// The first sanityMaxConsecutiveJumps (3) consecutive jumps, counted by
// streak (per run when Service supplies one, per call otherwise), are held
// back as rejected. An ordinary reading after them means they were a spike:
// they stay rejected and the streak starts over. A FOURTH consecutive jump
// means a genuine sustained shift (a meter replacement, a multiplier
// correction): it is kept, the held readings are given back with it, and one
// Rejection{Reason: RejectSanityJump, Register: "sustained"} is appended
// purely as an operator flag. From then on the streak is tripped and the
// check rejects nothing for the rest of its scope.
func Validate(prev *model.MeterReading, history []model.MeterReading, rows []model.MeterReading, now time.Time, o Options) (valid []model.MeterReading, rejected []Rejection) {
	thresholds := sanityThresholds(history, o.SanityMultiple)

	streak := o.SanityStreak
	if streak == nil {
		streak = &SanityStreak{}
	}

	var effectivePrev *model.MeterReading
	if prev != nil {
		p := *prev
		effectivePrev = &p
	}

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

		if !streak.tripped {
			if reg, jumped := isSanityJump(effectivePrev, row, thresholds, o.SanityMultiple); jumped {
				streak.consecutive++
				if streak.consecutive <= sanityMaxConsecutiveJumps {
					rejected = append(rejected, Rejection{Ts: row.Ts, Kind: row.Kind, Reason: RejectSanityJump, Register: reg})
					streak.held = append(streak.held, row)
					continue
				}
				// The streak's fourth consecutive jump: a sustained shift.
				// Flag it, trip the streak (it never re-arms), and give
				// back the readings held while it was in doubt (R509) —
				// they were real, and with them went the peak demand.
				streak.tripped = true
				rejected = withoutHeld(rejected, streak.held)
				valid = append(valid, streak.held...)
				streak.held = nil
				rejected = append(rejected, Rejection{Ts: row.Ts, Kind: row.Kind, Reason: RejectSanityJump, Register: "sustained"})
			} else {
				streak.consecutive = 0
				streak.held = nil // a spike that came back down stays rejected
			}
		}

		valid = append(valid, row)
		r := row
		effectivePrev = &r
	}
	return valid, rejected
}

// withoutHeld drops this call's sanity-jump rejections of the held readings:
// they are accepted after all. One held on an earlier page was already
// reported there and simply is not in rejected.
func withoutHeld(rejected []Rejection, held []model.MeterReading) []Rejection {
	if len(held) == 0 {
		return rejected
	}
	ts := make(map[int64]bool, len(held))
	for _, h := range held {
		ts[h.Ts.UnixNano()] = true
	}
	kept := rejected[:0]
	for _, r := range rejected {
		if r.Reason == RejectSanityJump && ts[r.Ts.UnixNano()] {
			continue
		}
		kept = append(kept, r)
	}
	return kept
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

// sanityThreshold is R13's "typical" for one register, as amended by R509:
// the LARGEST of its positive consecutive deltas in history, and the median
// time gap between the pairs of readings that produced those deltas. The
// median delta of a site that idles at night is its standby load, and ten
// times that is an ordinary working hour: on a real PM5340 history the
// median turned 826 good daytime readings into jumps, the maximum none. ok
// is false when history held fewer than sanityMinHistorySamples such deltas.
type sanityThreshold struct {
	maxDelta decimal.Decimal
	spacing  time.Duration
	ok       bool
}

func sanityThresholds(history []model.MeterReading, multiple decimal.Decimal) map[string]sanityThreshold {
	out := make(map[string]sanityThreshold, len(cumulativeRegisters))
	for _, reg := range cumulativeRegisters {
		deltas, spacings := registerDeltaSamples(history, reg)
		if len(deltas) < sanityMinHistorySamples {
			out[reg.name] = sanityThreshold{ok: false}
			continue
		}
		out[reg.name] = sanityThreshold{maxDelta: decimal.Max(deltas[0], deltas[1:]...), spacing: medianDuration(spacings), ok: true}
	}
	return out
}

// registerDeltaSamples walks history in order and, for every consecutive
// pair of readings that both carry a non-nil value for reg, returns the
// delta and the Ts gap between them — but ONLY for pairs whose delta is
// POSITIVE (R13: "positive deltas only" — a zero delta is an
// idle interval and a negative one is a decrease, neither of which
// describes a typical rate of consumption) and whose Ts gap is itself
// positive (out-of-order or duplicate-timestamp history rows never
// contribute a sample). deltas[i] and spacings[i] describe the same pair,
// so a caller computing "how many typical intervals elapsed" from spacing
// divides into a gap measured the same way the delta itself was.
func registerDeltaSamples(history []model.MeterReading, reg registerAccessor) (deltas []decimal.Decimal, spacings []time.Duration) {
	var prevVal *decimal.Decimal
	var prevTs time.Time
	for _, r := range history {
		v := reg.get(r)
		if v != nil {
			if prevVal != nil {
				gap := r.Ts.Sub(prevTs)
				delta := v.Sub(*prevVal)
				if gap > 0 && delta.IsPositive() {
					deltas = append(deltas, delta)
					spacings = append(spacings, gap)
				}
			}
			prevVal = v
			prevTs = r.Ts
		}
	}
	return deltas, spacings
}

func medianDuration(vals []time.Duration) time.Duration {
	sorted := make([]time.Duration, len(vals))
	copy(sorted, vals)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// isSanityJump reports whether row jumps on any cumulativeRegisters
// register relative to prev, using thresholds and multiple (R13's
// EKOKOD_INGEST_SANITY_MULTIPLE, o.SanityMultiple). Only the first
// offending register is named — Validate rejects the whole row on the
// first violation found, and walking cumulativeRegisters in a fixed order
// makes which one is named deterministic.
//
// A jump is tested only for an INCREASE (delta.IsPositive()): R13's sanity
// rule is about an implausible rate of consumption, and a decrease is
// R14's concern (DetectNegativeDeltas), never this one — it is stored
// either way. The limit scales with elapsed time: elapsedIntervals =
// (row.Ts − prev.Ts) ÷ th.spacing, limit = max(multiple × th.maxDelta ×
// elapsedIntervals, 1), so a gap several times the register's usual
// reporting interval widens the allowance proportionally instead of
// comparing a multi-interval delta against a single-interval threshold.
func isSanityJump(prev *model.MeterReading, row model.MeterReading, thresholds map[string]sanityThreshold, multiple decimal.Decimal) (register string, jumped bool) {
	if prev == nil {
		return "", false
	}
	tsGap := row.Ts.Sub(prev.Ts)
	if tsGap <= 0 {
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
		delta := cv.Sub(*pv)
		if !delta.IsPositive() {
			continue
		}
		elapsedIntervals := decimal.NewFromInt(tsGap.Nanoseconds()).Div(decimal.NewFromInt(th.spacing.Nanoseconds()))
		limit := multiple.Mul(th.maxDelta).Mul(elapsedIntervals)
		if limit.LessThan(decimal.NewFromInt(1)) {
			limit = decimal.NewFromInt(1)
		}
		if delta.GreaterThan(limit) {
			return reg.name, true
		}
	}
	return "", false
}
