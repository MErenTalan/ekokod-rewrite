package energy_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

func TestMaxDemandTakesTheMaximumNotTheDifference(t *testing.T) {
	rs := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAt(t0.Add(15*time.Minute), "40.25"),
		demandAt(t0.Add(30*time.Minute), "9"),
	}
	requireDecimal(t, energy.MaxDemand(win(t0, time.Hour), rs), "40.25")
}

func TestMaxDemandIsNilWhenNoReadingCarriesOne(t *testing.T) {
	require.Nil(t, energy.MaxDemand(win(t0, time.Hour), []energy.Reading{*readingAt(t0, "100")}))
}

func TestMaxDemandBillingRowCounts(t *testing.T) {
	rs := []energy.Reading{demandAtKind(t0, energy.KindBilling, "55.5")}
	requireDecimal(t, energy.MaxDemand(win(t0, time.Hour), rs), "55.5")
}

// TestMaxDemandDailyRowCounts proves KindDaily is honored by
// MaxDemandKinds's allowlist (C-3): a daily row must be able to win the
// maximum, not just load_profile and billing rows.
func TestMaxDemandDailyRowCounts(t *testing.T) {
	rs := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAtKind(t0.Add(15*time.Minute), energy.KindDaily, "77.25"),
	}
	requireDecimal(t, energy.MaxDemand(win(t0, time.Hour), rs), "77.25")
}

func TestMaxDemandIgnoresCurrentIndexEvenWhenHigher(t *testing.T) {
	// R65 re-ruled (C-3): current_index is stamped at ProfileDate by ARIL and
	// can carry the previous month's peak, so it must never win over a
	// counted kind even when its value is larger.
	rs := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAtKind(t0.Add(15*time.Minute), energy.KindCurrentIndex, "999"),
	}
	requireDecimal(t, energy.MaxDemand(win(t0, time.Hour), rs), "12.5")
}

func TestMaxDemandIgnoresResetEvenWhenHigher(t *testing.T) {
	rs := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAtKind(t0.Add(15*time.Minute), energy.KindReset, "999"),
	}
	requireDecimal(t, energy.MaxDemand(win(t0, time.Hour), rs), "12.5")
}

func TestMaxDemandWindowIsHalfOpen(t *testing.T) {
	w := win(t0, time.Hour)
	rs := []energy.Reading{
		demandAt(w.From, "10"),
		demandAt(w.To, "999"),
	}
	requireDecimal(t, energy.MaxDemand(w, rs), "10")
}

// TestMaxDemandDoesNotAliasTheInputReadingsPointer proves the *decimal.Decimal
// MaxDemand returns is an independent copy, not the winning reading's own
// MaxDemandKw pointer. Mutating the returned value's pointee must not affect
// the input reading — if MaxDemand ever returned &rs[i].MaxDemandKw's target
// directly (aliasing), this mutation would corrupt the input and this test
// would fail.
func TestMaxDemandDoesNotAliasTheInputReadingsPointer(t *testing.T) {
	rs := []energy.Reading{demandAt(t0, "12.5")}

	got := energy.MaxDemand(win(t0, time.Hour), rs)
	require.NotNil(t, got)
	require.NotSame(t, rs[0].MaxDemandKw, got, "MaxDemand must not alias the input reading's MaxDemandKw pointer")

	*got = decimal.NewFromInt(999)

	requireDecimal(t, rs[0].MaxDemandKw, "12.5")
}
