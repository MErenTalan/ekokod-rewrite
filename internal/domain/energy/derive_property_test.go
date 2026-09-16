package energy_test

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// --- Permanent brute-force property test (fix round 3, item 7) ------------
//
// This makes review round 2's throwaway property test
// (zz_r2_property_test.go, deleted after that review) a permanent part of
// the suite. It is deliberately smaller than the review's 1,000,000-
// scenario / 8,000,000-window run (which took ~30s) so it stays well under
// this package's -race budget, but it is still a real brute-force check:
// for every register, in every emitted window, Derive must return either
// nil (suspect, or simply unreported when neither boundary carries the
// register) or EXACTLY the true consumption over [start.TS, end.TS] —
// never a wrong number.
//
// Each scenario derives its own seed from its index (seed = i*1000003+17),
// so a failing scenario is reproducible by index alone without replaying
// every earlier scenario's random draws.
//
// Physical model: a true consumption series on one physical meter, or two
// with a swap at a random minute within the scenario. Readings land at
// random minutes with random kinds (load_profile/billing/daily) and
// occasionally omit a register (7%). A COMPLETE reset row (both tracked
// registers) sits at the swap minute when a swap exists. A reading placed
// exactly at the swap minute carries the OLD meter's closing value half the
// time — the deliberate ambiguity R90/R92 exist to resolve. Windows are
// chosen from a per-kind (70%) or mixed-kind (30%) boundary pool via
// SelectBoundary, so a boundary sometimes lands exactly on a reading or
// reset instant by construction (they share the same integer-minute grid).
// priors is every load_profile reading, sorted ascending by ts (a subslice
// of the naturally ascending-by-minute readings slice); resets is the one
// reset row, if any.
//
// Mutations this test is proven to catch (fix-round-3 report has the
// verbatim counterexample for each):
//   - returning the end value on a negative delta instead of suspecting it
//   - dropping the R90 end-side equality check
//   - dropping the R92 start-side equality check
//   - allowing the prior search to include TS == start.TS
//   - taking the absolute value of a segment delta
//
// It is blind to reason-only mutations (which Reason a suspicion carries)
// and to mutations that only widen suspicion further (erring toward
// flagging more, never toward billing a wrong number) — reset_test.go's
// unit tests cover those.

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

// propertyRegisters is the small register set the property test exercises.
// Two registers (rather than all twelve) keep each scenario fast while
// still exercising Derive's per-register independence, the same shape
// I-9's minimal fixture needs: one sound register, one with a partial
// reset or ambiguous boundary.
var propertyRegisters = []energy.Register{energy.ActiveImport, energy.T1Import}

var propertyKinds = []energy.Kind{energy.KindLoadProfile, energy.KindBilling, energy.KindDaily}

// meterCum holds one physical meter's per-register cumulative values,
// cum[reg][m] = base[reg] + sum of that meter's own increments over its
// first m minutes of operation — m is relative to the meter's OWN
// installation instant, not the scenario's absolute minute.
type meterCum map[energy.Register][]int64

func genMeterCum(rng *rand.Rand, minutes int) meterCum {
	m := make(meterCum, len(propertyRegisters))
	for _, reg := range propertyRegisters {
		cum := make([]int64, minutes+1)
		cum[0] = int64(rng.Intn(50))
		for i := 1; i <= minutes; i++ {
			inc := int64(0)
			if rng.Float64() >= 0.2 { // 20% of minutes have zero usage
				inc = int64(rng.Intn(8)) // 0-7
			}
			cum[i] = cum[i-1] + inc
		}
		m[reg] = cum
	}
	return m
}

func decPtr(v int64) *decimal.Decimal {
	d := decimal.NewFromInt(v)
	return &d
}

// propertyScenario is one generated scenario.
type propertyScenario struct {
	t0         time.Time
	minutes    int
	swap       bool
	swapMinute int
	meter0     meterCum
	meter1     meterCum // nil unless swap
	readings   []energy.Reading
	resetRow   *energy.Reading
}

// readingPlan is a reading's minute and kind, decided before its values —
// the zero-gap construction below needs to know where the load_profile
// readings fall before it can finish generating meter0's array.
type readingPlan struct {
	minute int
	kind   energy.Kind
}

func generatePropertyScenario(seed int64) propertyScenario {
	rng := rand.New(rand.NewSource(seed))
	sc := propertyScenario{t0: t0, minutes: 8 + rng.Intn(17)} // 8..24 minutes

	sc.swap = rng.Float64() < 0.5
	if sc.swap {
		sc.swapMinute = 1 + rng.Intn(sc.minutes-1) // 1..minutes-1: room on both sides
	}

	// Phase 1: decide which minutes carry a reading, and each one's kind,
	// before generating cumulative values.
	var plans []readingPlan
	for m := 0; m <= sc.minutes; m++ {
		if rng.Float64() >= 0.6 { // 60% of minutes carry a reading
			continue
		}
		plans = append(plans, readingPlan{minute: m, kind: propertyKinds[rng.Intn(len(propertyKinds))]})
	}

	sc.meter0 = genMeterCum(rng, sc.minutes)
	if sc.swap {
		// R91/I-1 zero-gap construction (matches the review's original
		// property test's "zero-gap mode"). Derive's before-reset evidence
		// for a register is the single LAST load_profile reading strictly
		// between the segment start and the reset — it never falls back to
		// an earlier reading, even one reporting the register, when that
		// one exists but omits it (I-1), and never to a more recent
		// non-load_profile reading (R91). When a real gap exists between
		// that reading and the reset, a plain "full visibility" truth is
		// only a LOWER BOUND on what Derive can prove, not a code defect —
		// that is a genuine, deliberate domain property, not something
		// this test should flag. Flattening meter0's increments from the
		// last pre-swap load_profile minute through swapMinute collapses
		// that ambiguity: it makes the reading's value and meter0's true
		// close-out value at swapMinute identical by construction, so
		// every window this scenario generates has an EXACT answer
		// whenever Derive finds any usable reset evidence at all — it is
		// never merely a lower bound, keeping this test's "nil-or-exact"
		// assertion sound.
		lastLP := -1
		for _, p := range plans {
			if p.minute < sc.swapMinute && p.kind == energy.KindLoadProfile {
				lastLP = p.minute
			}
		}
		if lastLP >= 0 {
			for _, reg := range propertyRegisters {
				flat := sc.meter0[reg][lastLP]
				for i := lastLP + 1; i <= sc.swapMinute; i++ {
					sc.meter0[reg][i] = flat
				}
			}
		}

		sc.meter1 = genMeterCum(rng, sc.minutes-sc.swapMinute)
		sc.resetRow = &energy.Reading{
			TS:   sc.t0.Add(time.Duration(sc.swapMinute) * time.Minute),
			Kind: energy.KindReset,
			Values: map[energy.Register]*decimal.Decimal{
				propertyRegisters[0]: decPtr(sc.meter1[propertyRegisters[0]][0]),
				propertyRegisters[1]: decPtr(sc.meter1[propertyRegisters[1]][0]),
			}, // COMPLETE: both tracked registers, always.
		}
	}

	// Phase 2: fill in each planned reading's values from the (now final)
	// meter arrays.
	for _, p := range plans {
		useOld := sc.swap && p.minute == sc.swapMinute && rng.Float64() < 0.5
		values := make(map[energy.Register]*decimal.Decimal, len(propertyRegisters))
		for _, reg := range propertyRegisters {
			if rng.Float64() < 0.07 { // 7% missing per register
				continue
			}
			values[reg] = decPtr(sc.currentCum(reg, p.minute, useOld))
		}
		sc.readings = append(sc.readings, energy.Reading{
			TS: sc.t0.Add(time.Duration(p.minute) * time.Minute), Kind: p.kind, Values: values,
		})
	}
	return sc
}

// currentCum is the value reg reads at minute m. useOld forces the OLD
// meter's closing value even though the new meter is (or is about to be)
// installed — used only to build the deliberately ambiguous reading a
// reset instant sometimes carries, the exact shape R90/R92 disambiguate.
func (sc propertyScenario) currentCum(reg energy.Register, m int, useOld bool) int64 {
	if sc.swap && m >= sc.swapMinute && !useOld {
		return sc.meter1[reg][m-sc.swapMinute]
	}
	mm := m
	if sc.swap && mm > sc.swapMinute {
		mm = sc.swapMinute // clamp: useOld never requests m > swapMinute in practice
	}
	return sc.meter0[reg][mm]
}

// trueConsumption is the ground truth over (aMinute, bMinute] — the sum of
// whichever physical meter was actually installed during each elapsed
// minute. The swap-instant minute is attributed to the OLD meter: it was
// installed for the whole of that minute, and the new meter's own clock
// starts at zero exactly AT swapMinute.
func (sc propertyScenario) trueConsumption(reg energy.Register, aMinute, bMinute int) decimal.Decimal {
	total := int64(0)
	for m := aMinute + 1; m <= bMinute; m++ {
		if !sc.swap || m <= sc.swapMinute {
			total += sc.meter0[reg][m] - sc.meter0[reg][m-1]
		} else {
			rm := m - sc.swapMinute
			total += sc.meter1[reg][rm] - sc.meter1[reg][rm-1]
		}
	}
	return decimal.NewFromInt(total)
}

func minutesBetween(base, ts time.Time) int {
	return int(ts.Sub(base) / time.Minute)
}

// dump renders a scenario+window for a counterexample failure message.
func (sc propertyScenario) dump(start, end *energy.Reading, resets []energy.Reading) string {
	s := fmt.Sprintf("minutes=%d swap=%v swapMinute=%d\n", sc.minutes, sc.swap, sc.swapMinute)
	s += fmt.Sprintf("start=%s(%s) end=%s(%s)\n", start.TS, start.Kind, end.TS, end.Kind)
	for _, r := range sc.readings {
		s += fmt.Sprintf("reading ts=%s kind=%s ai=%v t1=%v\n", r.TS, r.Kind, r.Values[energy.ActiveImport], r.Values[energy.T1Import])
	}
	for _, r := range resets {
		s += fmt.Sprintf("reset ts=%s ai=%v t1=%v\n", r.TS, r.Values[energy.ActiveImport], r.Values[energy.T1Import])
	}
	return s
}

// deterministicNegativeAndStartPriorEdgeCases anchors three of item 7's
// required mutations that pure random generation structurally cannot reach:
// this model's true consumption is always non-negative by construction (real
// increments never go backward), and R90/R92's boundary-instant equality
// checks intercept a mismatched swap reading before its arithmetic ever
// reaches a segment-delta computation — so a genuine negative segment delta,
// and a same-instant start-side prior whose exclusion actually changes the
// answer (as opposed to one masked by the zero-gap construction, since using
// the boundary's own value as before-reset evidence always gives a zero
// delta there), essentially never arise from the random scenarios above.
// These three fixed fixtures close that gap; the two R90/R92 mutations
// listed alongside them ARE reached by the random generation above (proven
// in the fix-round-3 report).
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
			wantReason: energy.ReasonMeterReset,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.priors == nil && c.name == "prior-duplicate-of-start-excluded" {
				c.priors = []energy.Reading{*c.start}
			}
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

func TestDerivePropertyNeverBillsAWrongNumber(t *testing.T) {
	deterministicNegativeAndStartPriorEdgeCases(t)

	n := propertyScenarioCount()
	const windowsPerScenario = 5
	checked := 0

	for i := 0; i < n; i++ {
		seed := int64(i)*1000003 + 17
		sc := generatePropertyScenario(seed)

		var resets []energy.Reading
		if sc.resetRow != nil {
			resets = []energy.Reading{*sc.resetRow}
		}
		var priors []energy.Reading
		for _, r := range sc.readings {
			if r.Kind == energy.KindLoadProfile {
				priors = append(priors, r)
			}
		}

		wrng := rand.New(rand.NewSource(seed ^ 0x5bd1e995))
		for w := 0; w < windowsPerScenario; w++ {
			a := wrng.Intn(sc.minutes + 1)
			b := wrng.Intn(sc.minutes + 1)
			if a > b {
				a, b = b, a
			}

			var pool []energy.Reading
			if wrng.Float64() < 0.7 {
				kind := propertyKinds[wrng.Intn(len(propertyKinds))]
				for _, r := range sc.readings {
					if r.Kind == kind {
						pool = append(pool, r)
					}
				}
			} else {
				pool = sc.readings
			}

			start := energy.SelectBoundary(pool, sc.t0.Add(time.Duration(a)*time.Minute))
			end := energy.SelectBoundary(pool, sc.t0.Add(time.Duration(b)*time.Minute))
			if start == nil || end == nil {
				continue
			}

			win := energy.Window{From: sc.t0.Add(time.Duration(a) * time.Minute), To: sc.t0.Add(time.Duration(b) * time.Minute)}
			d := energy.Derive(win, start, end, resets, priors)
			if !d.Emitted {
				continue
			}
			checked++

			startMinute := minutesBetween(sc.t0, start.TS)
			endMinute := minutesBetween(sc.t0, end.TS)

			for _, reg := range propertyRegisters {
				val := d.Values[reg]
				susp, hasSusp := d.Suspect[reg]

				if val != nil {
					if hasSusp {
						t.Fatalf("COUNTEREXAMPLE seed=%d window=%d reg=%s: both a value (%s) and a suspicion (%+v)\nscenario=%s",
							seed, w, reg, val.String(), susp, sc.dump(start, end, resets))
					}
					truth := sc.trueConsumption(reg, startMinute, endMinute)
					if !val.Equal(truth) {
						t.Fatalf("COUNTEREXAMPLE seed=%d window=%d reg=%s: Derive=%s truth=%s\nscenario=%s",
							seed, w, reg, val.String(), truth.String(), sc.dump(start, end, resets))
					}
					continue
				}
				if hasSusp {
					switch susp.Reason {
					case energy.ReasonNegativeDelta:
						if susp.Delta == nil || !susp.Delta.IsNegative() {
							t.Fatalf("seed=%d window=%d reg=%s: negative_delta without a negative Delta (%v)\nscenario=%s",
								seed, w, reg, susp.Delta, sc.dump(start, end, resets))
						}
					case energy.ReasonMeterReset:
						if susp.Delta != nil {
							t.Fatalf("seed=%d window=%d reg=%s: meter_reset carries a Delta (%s)\nscenario=%s",
								seed, w, reg, susp.Delta.String(), sc.dump(start, end, resets))
						}
					}
				}
			}
		}
	}

	require.Greater(t, checked, n/2, "the scenario generator produced too few emitting windows to be a meaningful property test")
}
