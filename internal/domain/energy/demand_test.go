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

// --- R101: MaxDemandKindsFor / MaxDemandOf ----------------------------------

// TestMaxDemandKindsForMatchesTheRuledSetPerLevel pins R101's exact table:
// Hourly load_profile only; Daily load_profile+daily; Monthly and Yearly
// load_profile+daily+billing (MaxDemandKinds' original, unqualified set).
func TestMaxDemandKindsForMatchesTheRuledSetPerLevel(t *testing.T) {
	require.ElementsMatch(t, []energy.Kind{energy.KindLoadProfile}, energy.MaxDemandKindsFor(energy.Hourly))
	require.ElementsMatch(t, []energy.Kind{energy.KindLoadProfile, energy.KindDaily}, energy.MaxDemandKindsFor(energy.Daily))
	require.ElementsMatch(t, []energy.Kind{energy.KindLoadProfile, energy.KindDaily, energy.KindBilling}, energy.MaxDemandKindsFor(energy.Monthly))
	require.ElementsMatch(t, []energy.Kind{energy.KindLoadProfile, energy.KindDaily, energy.KindBilling}, energy.MaxDemandKindsFor(energy.Yearly))
	require.Equal(t, energy.MaxDemandKinds, energy.MaxDemandKindsFor(energy.Yearly), "Monthly/Yearly's set is exactly MaxDemandKinds")
}

// TestMaxDemandOfAtHourlyExcludesADailyRow is R101's M-2 fix: at Hourly, a
// daily-kind row's day-peak must NOT win — it used to, under the
// unqualified MaxDemandKinds allowlist MaxDemand still uses by default.
func TestMaxDemandOfAtHourlyExcludesADailyRow(t *testing.T) {
	rs := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAtKind(t0.Add(15*time.Minute), energy.KindDaily, "77.25"),
	}
	kinds := energy.MaxDemandKindsFor(energy.Hourly)
	requireDecimal(t, energy.MaxDemandOf(win(t0, time.Hour), rs, kinds), "12.5")
}

// TestMaxDemandOfAtHourlyExcludesABillingRow is the same fix for a
// billing-kind row at Hourly.
func TestMaxDemandOfAtHourlyExcludesABillingRow(t *testing.T) {
	rs := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAtKind(t0.Add(15*time.Minute), energy.KindBilling, "999"),
	}
	kinds := energy.MaxDemandKindsFor(energy.Hourly)
	requireDecimal(t, energy.MaxDemandOf(win(t0, time.Hour), rs, kinds), "12.5")
}

// TestMaxDemandOfAtDailyAllowsDailyButExcludesBilling is R101: at Daily, a
// daily-kind row counts (it is that day's own stamped peak) but a
// billing-kind row (a whole month's peak) must not leak into a single day.
func TestMaxDemandOfAtDailyAllowsDailyButExcludesBilling(t *testing.T) {
	kinds := energy.MaxDemandKindsFor(energy.Daily)

	rs := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAtKind(t0.Add(15*time.Minute), energy.KindDaily, "77.25"),
	}
	requireDecimal(t, energy.MaxDemandOf(win(t0, time.Hour), rs, kinds), "77.25")

	rsBilling := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAtKind(t0.Add(15*time.Minute), energy.KindBilling, "999"),
	}
	requireDecimal(t, energy.MaxDemandOf(win(t0, time.Hour), rsBilling, kinds), "12.5")
}

// TestMaxDemandOfAtMonthlyAndYearlyAllowsEveryCountedKind is R101: Monthly
// and Yearly allow load_profile, daily AND billing rows — exactly
// MaxDemandKinds' original set — since a billing row's month-wide peak is
// only ever attributable at those two coarser levels.
func TestMaxDemandOfAtMonthlyAndYearlyAllowsEveryCountedKind(t *testing.T) {
	for _, level := range []energy.Level{energy.Monthly, energy.Yearly} {
		kinds := energy.MaxDemandKindsFor(level)
		rs := []energy.Reading{
			demandAt(t0, "12.5"),
			demandAtKind(t0.Add(15*time.Minute), energy.KindBilling, "999"),
		}
		requireDecimal(t, energy.MaxDemandOf(win(t0, time.Hour), rs, kinds), "999")
	}
}

// TestMaxDemandEqualsMaxDemandOfWithMaxDemandKinds proves MaxDemand is
// exactly MaxDemandOf(w, readings, MaxDemandKinds) (R101's doc comment):
// nothing about MaxDemand's own behaviour has changed.
func TestMaxDemandEqualsMaxDemandOfWithMaxDemandKinds(t *testing.T) {
	rs := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAtKind(t0.Add(15*time.Minute), energy.KindDaily, "77.25"),
		demandAtKind(t0.Add(30*time.Minute), energy.KindBilling, "55.5"),
	}
	w := win(t0, time.Hour)
	want := energy.MaxDemandOf(w, rs, energy.MaxDemandKinds)
	got := energy.MaxDemand(w, rs)
	require.Equal(t, want.String(), got.String())
}
