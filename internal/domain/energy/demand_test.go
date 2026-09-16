package energy_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

func TestMaxDemandTakesTheMaximumNotTheDifference(t *testing.T) {
	rs := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAt(t0.Add(15*time.Minute), "40.25"),
		demandAt(t0.Add(30*time.Minute), "9"),
	}
	require.Equal(t, "40.25", energy.MaxDemand(win(t0, time.Hour), rs).String())
}

func TestMaxDemandIsNilWhenNoReadingCarriesOne(t *testing.T) {
	require.Nil(t, energy.MaxDemand(win(t0, time.Hour), []energy.Reading{*readingAt(t0, "100")}))
}

func TestMaxDemandBillingRowCounts(t *testing.T) {
	rs := []energy.Reading{demandAtKind(t0, energy.KindBilling, "55.5")}
	require.Equal(t, "55.5", energy.MaxDemand(win(t0, time.Hour), rs).String())
}

func TestMaxDemandIgnoresCurrentIndexEvenWhenHigher(t *testing.T) {
	// R65 re-ruled (C-3): current_index is stamped at ProfileDate by ARIL and
	// can carry the previous month's peak, so it must never win over a
	// counted kind even when its value is larger.
	rs := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAtKind(t0.Add(15*time.Minute), energy.KindCurrentIndex, "999"),
	}
	require.Equal(t, "12.5", energy.MaxDemand(win(t0, time.Hour), rs).String())
}

func TestMaxDemandIgnoresResetEvenWhenHigher(t *testing.T) {
	rs := []energy.Reading{
		demandAt(t0, "12.5"),
		demandAtKind(t0.Add(15*time.Minute), energy.KindReset, "999"),
	}
	require.Equal(t, "12.5", energy.MaxDemand(win(t0, time.Hour), rs).String())
}

func TestMaxDemandWindowIsHalfOpen(t *testing.T) {
	w := win(t0, time.Hour)
	rs := []energy.Reading{
		demandAt(w.From, "10"),
		demandAt(w.To, "999"),
	}
	require.Equal(t, "10", energy.MaxDemand(w, rs).String())
}
