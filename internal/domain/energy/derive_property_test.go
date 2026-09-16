package energy_test

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// --- Permanent brute-force property test (fix round 4, I-11) --------------
//
// This is the widened replacement for the fix-round-3 generator. That
// earlier generator flattened every meter's increments across a swap to
// zero (the "zero-gap" construction), which made its "nil-or-exact-truth"
// assertion sound but also made it structurally blind to R93 — the round's
// main rule — because a reset row in that model always carried both
// tracked registers and never sat exactly on a window boundary. See the
// fix-round-3 review's I-11 finding and the verified prototype at
// .superpowers/sdd/2026-09-16-f3-consumption-engine/task-2-r3-property-prototype.go.txt.
//
// Physical model. Each scenario is a minute grid of N ticks with 0-3 meter
// swaps at distinct ticks (biased toward the grid's edges, with a 12%
// chance a swap shares the previous swap's instant, producing two reset
// rows on the same tick — the shape R92(3)'s tie-break exists for). At most
// one swap has NO reset row at all (the true "no evidence" case §3.2 can
// never detect) and, when present, is built so any segment crossing it is
// negative by construction, which is what makes the negative-delta
// mutations reachable without a fixed anchor. Every OTHER swap gets a reset
// row that omits each register independently with 7% probability — R93's
// central case — rather than always being complete. A third register
// (T2Import) is never reported by any reading or reset, for the
// over-flag invariant below; T1Import is reported scenario-wide with 70%
// probability. Meter bases are drawn from {0, 0-49, 0-8999} for every
// meter (not just the "new" one), so a wrongly accepted start or end value
// produces a clearly positive wrong number rather than one that merges
// into a coincidentally-flagged negative difference.
//
// Readings (load_profile/billing/daily) are placed independently per tick
// (50%/15%/10%, 60% each at a swap tick, so several kinds can share an
// instant), and a reading at a swap tick carries an old-or-intermediate
// meter's value half the time — the deliberate ambiguity R90/R92 exist to
// resolve. 8 windows per scenario draw their bounds from a swap instant, a
// +30s offset, or a tick from -2 to N+2, over a pool that is one kind,
// mixed, or "mixed plus every reset row" (reset rows sorted before or
// after same-instant readings at random); when a chosen boundary is itself
// a reset row, that one row is left out of the resets slice passed to
// Derive half the time (the M-13/R93-boundary case).
//
// Oracle. The full truth (start.TS, end.TS] is accepted whenever Derive
// returns it. Otherwise, the ONLY other acceptable non-suspect value is
// the R55/R91 answer computed directly from the physical meter series: the
// sum, over every physical reset row in the window (whether or not it was
// passed to Derive), of the truth from the previous credited point up to
// the last load_profile reading strictly before that reset (the exact
// before-reset search Derive itself performs), plus the truth from the
// last reset to end. If that reading is missing or lacks the register, ANY
// non-nil value is a counterexample — this is what exposes a wrong prior
// search (M4) without needing a fixed anchor. Every register is also
// checked for the general invariants: a boundary that lacks the register
// gives nil with no suspicion; when both boundaries report it, a value and
// a suspicion never both appear and never both are absent;
// ReasonNegativeDelta always carries a negative Delta and ReasonMeterReset
// always carries a nil Delta; Emitted matches start.TS.Before(end.TS).
//
// Mutations this generator is proven (fix-round-4 report has the verbatim
// counterexample or table entry for each) to catch on its own, with no
// fixed anchor: dropping R90's end-instant check, dropping R92's
// start-instant check, narrowing either gate to load_profile only,
// loosening either equality check to an inequality, undoing R93 on the
// window side or the start side, treating a nil reset value as zero,
// ignoring a reset-kind boundary missing from resets, letting the prior
// search include TS == start.TS, taking .Abs() of a segment delta, and
// billing a negative segment — including the specific "return the end
// value on a negative final segment" shape the fix-round-3 generator
// missed.
//
// It remains blind to reason-only mutations (which Reason a suspicion
// carries, never which value it bills) and to mutations that only widen
// suspicion further (erring toward flagging more, never toward billing a
// wrong number) — reset_test.go's unit tests cover those.

const propertyDefaultScenarios = 20000

// propertyScenarioCount lets a caller opt into a much larger run (the
// optional env knob the task allows) without touching the default budget
// this file's normal `go test` run stays inside.
func propertyScenarioCount() int {
	if v := os.Getenv("ENERGY_PROPERTY_SCENARIOS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return propertyDefaultScenarios
}

// propertyRegisters is the register set the property test exercises.
// T2Import is deliberately never reported by any reading or reset row (see
// generatePropertyScenario), which is what lets the over-flag invariant
// ("unreported implies never suspect") be checked on a real register name.
var propertyRegisters = []energy.Register{energy.ActiveImport, energy.T1Import, energy.T2Import}

var propertyKinds = []energy.Kind{energy.KindLoadProfile, energy.KindBilling, energy.KindDaily}

// propertyMeter is one physical meter's per-register cumulative series,
// valid on ticks [install, remove].
type propertyMeter struct {
	install, remove int
	cum             map[energy.Register][]int64
}

// propertySwap is one physical meter replacement at tick. unreset marks the
// one swap per scenario (at most) that gets no reset row at all.
type propertySwap struct {
	tick    int
	unreset bool
}

// propertyScenario is one generated scenario: a sequence of physical
// meters joined at swaps, the readings and reset rows observed against
// them, and the bookkeeping needed to compute the ground truth for any
// window.
type propertyScenario struct {
	n        int
	meters   []propertyMeter
	swaps    []propertySwap
	readings []energy.Reading // non-reset, sorted by TS (stable)
	tickOf   map[time.Time]int
	resets   []energy.Reading // sorted by TS
	priors   []energy.Reading // the load_profile subset of readings
	hasT1    bool

	// oldAtSwap marks a reading (keyed by TS.String()+string(Kind)) that
	// was generated from an old-or-intermediate physical meter at a swap
	// tick (the deliberate R90/R92 ambiguity, see the reading loop below).
	// The oracle uses this to know when a reading's own reported value is
	// stale evidence, not a trustworthy boundary (see
	// endOldOffMeterMismatch).
	oldAtSwap map[string]bool
}

func propertyTS(tick, offSeconds int) time.Time {
	return t0.Add(time.Duration(tick)*time.Minute + time.Duration(offSeconds)*time.Second)
}

// propertyBase draws a meter's starting cumulative value from a mix of
// zero, small and large bases, so that accepting a wrong boundary value
// produces a clearly wrong (not coincidentally plausible) number.
func propertyBase(rng *rand.Rand) int64 {
	switch rng.Intn(3) {
	case 0:
		return 0
	case 1:
		return int64(rng.Intn(50))
	default:
		return int64(rng.Intn(9000))
	}
}

func decPtr(v int64) *decimal.Decimal {
	d := decimal.NewFromInt(v)
	return &d
}

func generatePropertyScenario(seed int64) *propertyScenario {
	rng := rand.New(rand.NewSource(seed))
	sc := &propertyScenario{n: 10 + rng.Intn(31), tickOf: map[time.Time]int{}, oldAtSwap: map[string]bool{}} // 10..40 ticks
	sc.hasT1 = rng.Float64() < 0.7

	// 0-3 swaps, biased toward the grid's edges, with a chance of two
	// swaps sharing one instant (R92(3)'s tie-break case, M8).
	var nSwaps int
	switch x := rng.Float64(); {
	case x < 0.35:
		nSwaps = 0
	case x < 0.7:
		nSwaps = 1
	case x < 0.92:
		nSwaps = 2
	default:
		nSwaps = 3
	}
	var ticks []int
	for i := 0; i < nSwaps; i++ {
		if i > 0 && rng.Float64() < 0.12 {
			ticks = append(ticks, ticks[len(ticks)-1]) // same-instant double swap
			continue
		}
		var tk int
		if rng.Float64() < 0.3 {
			tk = []int{0, 1, sc.n - 1, sc.n}[rng.Intn(4)]
		} else {
			tk = rng.Intn(sc.n + 1)
		}
		ticks = append(ticks, tk)
	}
	sort.Ints(ticks)
	for i, tk := range ticks {
		sw := propertySwap{tick: tk}
		// At most one unreset swap, and only at a tick with no other swap
		// at the same instant (an unreset swap has no reset row to
		// disambiguate a same-instant reset, which would be a different,
		// untested shape).
		distinct := (i == 0 || ticks[i-1] < tk) && (i == len(ticks)-1 || ticks[i+1] > tk)
		alreadyHasUnreset := false
		for _, prev := range sc.swaps {
			alreadyHasUnreset = alreadyHasUnreset || prev.unreset
		}
		if distinct && !alreadyHasUnreset && rng.Float64() < 0.25 {
			sw.unreset = true
		}
		sc.swaps = append(sc.swaps, sw)
	}

	// Build the meter chain. An unreset swap's replacement meter starts
	// HIGH (1000-8999) while the meter it replaces ends near its own
	// small/zero base, so any segment spanning it is negative by
	// construction — this is what makes the negative-delta mutations
	// reachable without a fixed anchor (§3.2 can never detect an upward
	// swap with no reset row from the registers alone).
	install := 0
	for k := 0; k <= len(sc.swaps); k++ {
		remove := sc.n
		if k < len(sc.swaps) {
			remove = sc.swaps[k].tick
		}
		base := map[energy.Register]int64{}
		for _, reg := range propertyRegisters {
			switch {
			case k < len(sc.swaps) && sc.swaps[k].unreset:
				base[reg] = 1000 + int64(rng.Intn(8000))
			case k > 0 && sc.swaps[k-1].unreset:
				base[reg] = 0
			default:
				base[reg] = propertyBase(rng)
			}
		}
		m := propertyMeter{install: install, remove: remove, cum: map[energy.Register][]int64{}}
		for _, reg := range propertyRegisters {
			c := make([]int64, sc.n+1)
			c[install] = base[reg]
			for t := install + 1; t <= remove; t++ {
				inc := int64(0)
				if rng.Float64() >= 0.2 { // 20% of ticks have zero usage
					inc = int64(rng.Intn(8))
				}
				c[t] = c[t-1] + inc
			}
			m.cum[reg] = c
		}
		sc.meters = append(sc.meters, m)
		install = remove
	}

	reports := func(reg energy.Register) bool {
		switch reg {
		case energy.T2Import:
			return false // never reported by this meter family
		case energy.T1Import:
			return sc.hasT1
		}
		return true
	}
	mkVals := func(meter, tick int, missProb float64) map[energy.Register]*decimal.Decimal {
		v := map[energy.Register]*decimal.Decimal{}
		for _, reg := range propertyRegisters {
			if !reports(reg) || rng.Float64() < missProb {
				continue
			}
			v[reg] = decPtr(sc.meters[meter].cum[reg][tick])
		}
		return v
	}

	// Reset rows: one per swap that isn't the (at most one) unreset swap,
	// each independently omitting a register 7% of the time — R93's
	// central case, rather than the fix-round-3 generator's always-
	// complete row.
	for k, sw := range sc.swaps {
		if sw.unreset {
			continue
		}
		row := energy.Reading{TS: propertyTS(sw.tick, 0), Kind: energy.KindReset, Values: mkVals(k+1, sw.tick, 0.07)}
		sc.resets = append(sc.resets, row)
		sc.tickOf[row.TS] = sw.tick
	}

	probs := []float64{0.5, 0.15, 0.1} // load_profile, billing, daily
	for t := 0; t <= sc.n; t++ {
		kBefore, kAfter := 0, 0
		for _, sw := range sc.swaps {
			if sw.tick < t {
				kBefore++
			}
			if sw.tick <= t {
				kAfter++
			}
		}
		for ki, kind := range propertyKinds {
			p := probs[ki]
			if kAfter > kBefore {
				p = 0.6 // more likely to be observed right at a swap instant
			}
			if rng.Float64() >= p {
				continue
			}
			meter, off := kAfter, 0
			if kAfter > kBefore && rng.Float64() < 0.5 {
				meter = kBefore + rng.Intn(kAfter-kBefore) // old or intermediate meter at the swap instant
			} else if rng.Float64() < 0.25 {
				off = 1 + rng.Intn(59) // off the exact minute
			}
			r := energy.Reading{TS: propertyTS(t, off), Kind: kind, Values: mkVals(meter, t, 0.07)}
			sc.readings = append(sc.readings, r)
			sc.tickOf[r.TS] = t
			if meter < kAfter {
				sc.oldAtSwap[r.TS.String()+string(kind)] = true
			}
		}
	}
	sort.SliceStable(sc.readings, func(i, j int) bool { return sc.readings[i].TS.Before(sc.readings[j].TS) })
	for _, r := range sc.readings {
		if r.Kind == energy.KindLoadProfile {
			sc.priors = append(sc.priors, r)
		}
	}
	return sc
}

// truth sums, over ticks (a, b], each tick's increment on whichever
// physical meter was installed during it. This is the ground truth Derive
// must never exceed, and must equal exactly whenever it returns a value
// with no usable reset evidence crossed.
func (sc *propertyScenario) truth(reg energy.Register, a, b int) int64 {
	var tot int64
	for t := a + 1; t <= b; t++ {
		k := 0
		for _, sw := range sc.swaps {
			if sw.tick < t {
				k++
			}
		}
		c := sc.meters[k].cum[reg]
		tot += c[t] - c[t-1]
	}
	return tot
}

func propertyReadingSummary(r *energy.Reading) string {
	s := ""
	for _, reg := range propertyRegisters {
		if v := r.Value(reg); v != nil {
			s += fmt.Sprintf("%s=%s ", reg[:2], v)
		}
	}
	return s
}

// dump renders a scenario+window for a counterexample failure message.
func (sc *propertyScenario) dump(start, end *energy.Reading, resets []energy.Reading) string {
	s := fmt.Sprintf("n=%d hasT1=%v swaps=", sc.n, sc.hasT1)
	for _, sw := range sc.swaps {
		s += fmt.Sprintf("{tick=%d unreset=%v} ", sw.tick, sw.unreset)
	}
	s += fmt.Sprintf("\nstart=%s %s %s\nend=%s %s %s\n",
		start.TS.Format("15:04:05"), start.Kind, propertyReadingSummary(start),
		end.TS.Format("15:04:05"), end.Kind, propertyReadingSummary(end))
	for k, m := range sc.meters {
		s += fmt.Sprintf("meter%d [%d,%d] ai=%v\n", k, m.install, m.remove, m.cum[energy.ActiveImport][m.install:m.remove+1])
	}
	for _, r := range sc.readings {
		s += fmt.Sprintf("  %s %s %s\n", r.TS.Format("15:04:05"), r.Kind, propertyReadingSummary(&r))
	}
	for _, r := range resets {
		s += fmt.Sprintf("  passed reset %s %s\n", r.TS.Format("15:04:05"), propertyReadingSummary(&r))
	}
	return s
}

// deterministicNegativeAndStartPriorEdgeCases anchors 3 fixed fixtures that
// are cheap to keep even though the widened random generator above now
// reaches the same mutations (fix-round-4 report): a plain-difference
// negative delta, a reset-path segment negative delta, and a prior that
// duplicates the start reading itself (R91 must exclude it by strict TS >
// start.TS, never use it as before-reset evidence).
func deterministicNegativeAndStartPriorEdgeCases(t *testing.T) {
	cases := []struct {
		name       string
		start, end *energy.Reading
		resets     []energy.Reading
		priors     []energy.Reading
		wantReason energy.Reason
		wantDelta  bool // require a non-nil, negative Suspicion.Delta
	}{
		{
			// A reset AT start.TS matching start's own value (so R92(2)
			// passes silently) forces Derive into deriveRegister's
			// len(windowResets)==0 plain-difference branch instead of the
			// outer Derive-level fast path (which calls the separate,
			// unmutated Difference function directly and would not
			// exercise this mutation at all).
			name:       "plain-difference-negative-delta",
			start:      &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: decPtr(900)}},
			end:        &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: decPtr(12)}},
			resets:     []energy.Reading{{TS: t0, Kind: energy.KindReset, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: decPtr(900)}}},
			wantReason: energy.ReasonNegativeDelta,
			wantDelta:  true,
		},
		{
			name:       "reset-path-segment-negative-delta",
			start:      &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: decPtr(900)}},
			end:        &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: decPtr(40)}},
			resets:     []energy.Reading{{TS: t0.Add(40 * time.Minute), Kind: energy.KindReset, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: decPtr(0)}}},
			priors:     []energy.Reading{{TS: t0.Add(30 * time.Minute), Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: decPtr(800)}}}, // 800-900 = -100
			wantReason: energy.ReasonNegativeDelta,
			wantDelta:  true,
		},
		{
			name:   "prior-duplicate-of-start-excluded",
			start:  &energy.Reading{TS: t0, Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: decPtr(900)}},
			end:    &energy.Reading{TS: t0.Add(time.Hour), Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: decPtr(40)}},
			resets: []energy.Reading{{TS: t0.Add(40 * time.Minute), Kind: energy.KindReset, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: decPtr(0)}}},
			// priors duplicates start itself (Task 7 always includes the
			// boundary reading in priors) with NO other reading between
			// start.TS and the reset — R91 must exclude it (strict TS >
			// start.TS), never use it as before-reset evidence.
			priors:     []energy.Reading{{TS: t0, Kind: energy.KindLoadProfile, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: decPtr(900)}}},
			wantReason: energy.ReasonMeterReset,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := energy.Derive(energy.Window{From: t0, To: t0.Add(time.Hour)}, c.start, c.end, c.resets, c.priors)
			val := d.Values[energy.ActiveImport]
			susp, hasSusp := d.Suspect[energy.ActiveImport]
			if val != nil {
				t.Fatalf("COUNTEREXAMPLE %s: Derive=%s, want suspect (%s)", c.name, val.String(), c.wantReason)
			}
			if !hasSusp || susp.Reason != c.wantReason {
				t.Fatalf("%s: want reason=%s, got %+v", c.name, c.wantReason, susp)
			}
			if c.wantDelta && (susp.Delta == nil || !susp.Delta.IsNegative()) {
				t.Fatalf("%s: want a non-nil negative Suspicion.Delta, got %v", c.name, susp.Delta)
			}
		})
	}
}

// endOldOffMeterMismatch reports whether end is a reading generated from an
// old-or-intermediate physical meter at a swap tick (sc.oldAtSwap), AND a
// reset row EXISTS at exactly end.TS in resets (the slice actually passed
// to Derive for this window) whose value for reg is nil or differs from
// end's own reported value.
//
// This is the review's minimal oracle fix (final-review-B-report.md,
// property test section): the R55/R91 gap allowance below exists for
// segments Derive legitimately cannot resolve past a reset it has no
// evidence for at all — an "unreset" swap (no reset row anywhere) is that
// case and is deliberately left to the existing exp-based check. This
// helper instead targets the narrower shape where a reset row DOES cover
// end.TS but end's own reported value doesn't agree with it: reset.go's
// R90 end-side check exists precisely to catch a reading physically taken
// off the meter being replaced, reported at the exact instant of the
// swap. When this is true, the R55-gap fallback does not apply to reg:
// Derive must return nil (suspect) or the exact full truth, never a
// number built from that stale evidence, even one that happens to equal
// the R55/R91 formula's output.
//
// The value-equality exemption (no mismatch when the reset row's value for
// reg equals end's own value) is required: an old meter's final reading
// can legitimately coincide with the new meter's base, and R90 then
// correctly bills across the swap using that shared value.
func (sc *propertyScenario) endOldOffMeterMismatch(end *energy.Reading, reg energy.Register, resets []energy.Reading) bool {
	if !sc.oldAtSwap[end.TS.String()+string(end.Kind)] {
		return false
	}
	var last *energy.Reading
	for i := range resets {
		if resets[i].TS.Equal(end.TS) {
			r := resets[i]
			last = &r
		}
	}
	if last == nil {
		return false
	}
	rv := last.Value(reg)
	if rv == nil {
		return true
	}
	return !rv.Equal(*end.Value(reg))
}

func TestDerivePropertyNeverBillsAWrongNumber(t *testing.T) {
	deterministicNegativeAndStartPriorEdgeCases(t)

	n := propertyScenarioCount()
	stats := map[string]int{}
	checked := 0

	for i := 0; i < n; i++ {
		seed := int64(i)*7919 + 3
		sc := generatePropertyScenario(seed)
		wrng := rand.New(rand.NewSource(seed ^ 0x2545F491))

		pools := map[string][]energy.Reading{}
		for _, r := range sc.readings {
			pools[string(r.Kind)] = append(pools[string(r.Kind)], r)
		}
		pools["mixed"] = sc.readings
		withResets := append(append([]energy.Reading{}, sc.readings...), sc.resets...)
		resetsFirst := wrng.Float64() < 0.5
		sort.SliceStable(withResets, func(i, j int) bool {
			if withResets[i].TS.Equal(withResets[j].TS) && resetsFirst {
				return withResets[i].Kind == energy.KindReset && withResets[j].Kind != energy.KindReset
			}
			return withResets[i].TS.Before(withResets[j].TS)
		})
		pools["withResets"] = withResets
		poolNames := []string{"load_profile", "billing", "daily", "mixed", "mixed", "withResets", "withResets"}

		bound := func() time.Time {
			switch x := wrng.Float64(); {
			case x < 0.25 && len(sc.swaps) > 0:
				return propertyTS(sc.swaps[wrng.Intn(len(sc.swaps))].tick, 0)
			case x < 0.4:
				return propertyTS(wrng.Intn(sc.n+1), 30)
			default:
				return propertyTS(wrng.Intn(sc.n+5)-2, 0)
			}
		}

		for w := 0; w < 8; w++ {
			a, b := bound(), bound()
			if b.Before(a) {
				a, b = b, a
			}
			pool := pools[poolNames[wrng.Intn(len(poolNames))]]
			start := energy.SelectBoundary(pool, a)
			end := energy.SelectBoundary(pool, b)
			if start == nil || end == nil {
				continue
			}

			// A boundary that is itself a reset row is left out of the
			// resets slice passed to Derive half the time (the M-13/R93
			// boundary case), identified by pointer identity so only that
			// exact row (never a same-instant sibling) is dropped. This is
			// restricted to instants with exactly one reset row: dropping
			// one of TWO same-instant reset rows lands on M-15 (a known,
			// accepted Minor defect — end billed against the OTHER row at
			// the same instant — that needs two reset rows sharing an
			// instant, which the primary key (analyzer_id, ts, kind) makes
			// unreachable in production), not a code counterexample.
			resets := sc.resets
			for _, bnd := range []*energy.Reading{start, end} {
				if bnd.Kind != energy.KindReset || wrng.Float64() >= 0.5 {
					continue
				}
				nAt := 0
				for _, r := range sc.resets {
					if r.TS.Equal(bnd.TS) {
						nAt++
					}
				}
				if nAt != 1 {
					continue
				}
				var kept []energy.Reading
				for _, r := range resets {
					if !r.TS.Equal(bnd.TS) || reflect.ValueOf(r.Values).Pointer() != reflect.ValueOf(bnd.Values).Pointer() {
						kept = append(kept, r)
					}
				}
				resets = kept
			}

			d := energy.Derive(energy.Window{From: a, To: b}, start, end, resets, sc.priors)
			if d.Emitted != start.TS.Before(end.TS) {
				t.Fatalf("seed=%d w=%d: Emitted=%v for start=%s end=%s", seed, w, d.Emitted, start.TS, end.TS)
			}
			if !d.Emitted {
				continue
			}
			checked++

			sTick, eTick := sc.tickOf[start.TS], sc.tickOf[end.TS]
			for _, reg := range propertyRegisters {
				val := d.Values[reg]
				susp, hasSusp := d.Suspect[reg]
				fail := func(msg string, args ...any) {
					t.Helper()
					t.Fatalf("COUNTEREXAMPLE seed=%d w=%d reg=%s: %s\n%s", seed, w, reg, fmt.Sprintf(msg, args...), sc.dump(start, end, resets))
				}

				if start.Value(reg) == nil || end.Value(reg) == nil {
					if val != nil || hasSusp {
						fail("boundary lacks register but val=%v susp=%+v", val, susp)
					}
					stats["unreported"]++
					continue
				}
				if val == nil {
					if !hasSusp {
						fail("both boundaries report it but nil with no suspicion")
					}
					if susp.Reason == energy.ReasonNegativeDelta && (susp.Delta == nil || !susp.Delta.IsNegative()) {
						fail("negative_delta without negative Delta")
					}
					if susp.Reason == energy.ReasonMeterReset && susp.Delta != nil {
						fail("meter_reset with Delta")
					}
					stats["suspect_"+string(susp.Reason)]++
					continue
				}
				if hasSusp {
					fail("value %s and suspicion %+v", val, susp)
				}
				if val.Equal(decimal.NewFromInt(sc.truth(reg, sTick, eTick))) {
					stats["exact"]++
					continue // the full physical truth is never a wrong bill
				}
				if sc.endOldOffMeterMismatch(end, reg, resets) {
					fail("Derive=%s but end is an old/intermediate-meter reading at a reset instant whose value mismatches (or is uncovered by) the reset row — the R55-gap allowance does not apply to usage the end reading itself measured; want nil or the full truth=%d",
						val, sc.truth(reg, sTick, eTick))
				}
				// Expected per R55/R91 from the PHYSICAL series: sum of
				// physical usage over each segment's credited interval
				// (prev, lastPrior] and (lastReset, end].
				var exp int64
				prevTS, prevTick := start.TS, sTick
				for _, r := range sc.resets { // every physical reset row, passed or not
					if !r.TS.After(start.TS) || r.TS.After(end.TS) {
						continue
					}
					var p *energy.Reading
					for j := range sc.priors {
						if sc.priors[j].TS.After(prevTS) && sc.priors[j].TS.Before(r.TS) {
							p = &sc.priors[j]
						}
					}
					if p == nil || p.Value(reg) == nil {
						fail("billed %s across reset at %s with no usable before-reset prior", val, r.TS)
					}
					exp += sc.truth(reg, prevTick, sc.tickOf[p.TS])
					prevTS, prevTick = r.TS, sc.tickOf[r.TS]
				}
				exp += sc.truth(reg, prevTick, eTick)
				if !val.Equal(decimal.NewFromInt(exp)) {
					fail("Derive=%s expected=%d (full truth=%d)", val, exp, sc.truth(reg, sTick, eTick))
				}
				stats["exact"]++
				if exp != sc.truth(reg, sTick, eTick) {
					stats["exact_below_full_truth_by_R55_gap"]++
				}
			}
		}
	}

	t.Logf("stats: %v", stats)
	require.Greater(t, checked, n/2, "the scenario generator produced too few emitting windows to be a meaningful property test")
}
