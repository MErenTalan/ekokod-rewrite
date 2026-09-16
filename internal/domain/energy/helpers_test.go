package energy_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// t0 is an arbitrary, fixed instant every test in this package anchors its
// windows and readings to.
var t0 = time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

// dec parses a decimal literal into a *decimal.Decimal, failing the test
// binary at init-adjacent call time (via decimal.RequireFromString) if the
// literal is malformed — every literal in this package's tests is a fixed
// constant, so a parse failure here is a typo in the test, not a runtime
// condition to handle gracefully.
func dec(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

// readingAt builds a load-profile reading at ts with active_import set to
// activeImport and every other register absent.
func readingAt(ts time.Time, activeImport string) *energy.Reading {
	return &energy.Reading{
		TS:   ts,
		Kind: energy.KindLoadProfile,
		Values: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport: dec(activeImport),
		},
	}
}

// resetAt builds a meter-reset reading at ts (02 §3.2, KindReset) carrying
// only active_import — a reset row is evidence of a physical register
// reset, never a full multi-register reading, so later tasks that consume
// this helper should not expect other registers to be populated. Every
// later task that needs a reset fixture (Task 2's Derive, in particular)
// must use this instead of redefining it.
func resetAt(ts time.Time, value string) *energy.Reading {
	return &energy.Reading{
		TS:   ts,
		Kind: energy.KindReset,
		Values: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport: dec(value),
		},
	}
}

// demandAt builds a load-profile reading at ts carrying only MaxDemandKw —
// max_demand is not a cumulative register (02 §3.5) and is never part of
// Reading.Values, so this helper is the one later tasks (load-profile
// summarisation, in particular) should use for a demand-only fixture
// instead of redefining it.
func demandAt(ts time.Time, kw string) energy.Reading {
	return energy.Reading{
		TS:          ts,
		Kind:        energy.KindLoadProfile,
		MaxDemandKw: dec(kw),
	}
}

// demandAtKind builds a reading at ts of the given kind carrying only
// MaxDemandKw, for tests that need a demand-only fixture of a kind other
// than load_profile (demandAt's fixed kind). Task 3's MaxDemand tests use
// this to exercise MaxDemandKinds's allowlist against billing,
// current_index and reset rows.
func demandAtKind(ts time.Time, kind energy.Kind, kw string) energy.Reading {
	return energy.Reading{
		TS:          ts,
		Kind:        kind,
		MaxDemandKw: dec(kw),
	}
}

// win builds a half-open Window of length d starting at from.
func win(from time.Time, d time.Duration) energy.Window {
	return energy.Window{From: from, To: from.Add(d)}
}

// vals builds a Values map carrying exactly active_import and t1_import,
// for tests that need more than one register without readingAt's
// single-register shape.
func vals(active, t1 string) map[energy.Register]*decimal.Decimal {
	return map[energy.Register]*decimal.Decimal{
		energy.ActiveImport: dec(active),
		energy.T1Import:     dec(t1),
	}
}

// istanbul loads the Europe/Istanbul location, failing the test immediately
// if it cannot be resolved (it always can: internal/domain/energy blank-
// imports time/tzdata precisely so this never depends on the host's system
// tzdata). Every test in this package that needs the location uses this
// helper instead of loading it inline.
func istanbul(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	return loc
}

// TestHelperResetAtAndDemandAtShapes pins resetAt and demandAt's shapes for
// the later tasks that will reuse them (Task 2's Derive for resetAt, load-
// profile summarisation for demandAt), since neither is exercised by this
// task's own tests otherwise.
func TestHelperResetAtAndDemandAtShapes(t *testing.T) {
	r := resetAt(t0, "0.0000")
	require.Equal(t, energy.KindReset, r.Kind)
	require.Equal(t, "0", r.Value(energy.ActiveImport).String())
	require.Nil(t, r.Value(energy.T1Import), "resetAt carries only active_import")

	d := demandAt(t0, "7.5")
	require.Equal(t, energy.KindLoadProfile, d.Kind)
	require.NotNil(t, d.MaxDemandKw)
	require.Equal(t, "7.5", d.MaxDemandKw.String())
	require.Nil(t, d.Values, "demandAt carries no cumulative registers")
}
