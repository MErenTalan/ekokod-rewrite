package ingest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fetchInternalTestNow anchors every table case below.
var fetchInternalTestNow = time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC)

// TestShouldEnqueueConsumptionRefresh is a pure table test of the enqueue
// decision: R100(1)'s hasLoadProfile gate, and R500 — a recent range enqueues
// exactly like an old one.
func TestShouldEnqueueConsumptionRefresh(t *testing.T) {
	past := fetchInternalTestNow.Add(-40 * 24 * time.Hour)
	recent := fetchInternalTestNow.Add(-time.Hour)

	cases := []struct {
		name           string
		hasEnqueuer    bool
		enabled        bool
		hasLoadProfile bool
		affectedFrom   *time.Time
		want           bool
	}{
		{"no enqueuer wired", false, true, true, &past, false},
		{"config disabled", true, false, true, &past, false},
		{"no load_profile persisted (R100(1))", true, true, false, &past, false},
		{"nil affected range", true, true, true, nil, false},
		{"recent affected range (R500)", true, true, true, &recent, true},
		{"old affected range", true, true, true, &past, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shouldEnqueueConsumptionRefresh(c.hasEnqueuer, c.enabled, c.hasLoadProfile, c.affectedFrom)
			require.Equal(t, c.want, got)
		})
	}
}

// TestNoteLoadProfilePersisted proves fetchAccumulator.hasLoadProfile only
// ever flips true for a load_profile kind with at least one row, never for
// any other kind and never for a zero count (R100(1)).
func TestNoteLoadProfilePersisted(t *testing.T) {
	t.Run("load_profile with rows sets it", func(t *testing.T) {
		acc := newFetchAccumulator()
		acc.noteLoadProfilePersisted("load_profile", 3)
		require.True(t, acc.hasLoadProfile)
	})

	t.Run("load_profile with zero rows leaves it false", func(t *testing.T) {
		acc := newFetchAccumulator()
		acc.noteLoadProfilePersisted("load_profile", 0)
		require.False(t, acc.hasLoadProfile)
	})

	t.Run("a non-load_profile kind never sets it, even with rows", func(t *testing.T) {
		acc := newFetchAccumulator()
		acc.noteLoadProfilePersisted("daily", 5)
		acc.noteLoadProfilePersisted("billing", 5)
		acc.noteLoadProfilePersisted("reset", 5)
		acc.noteLoadProfilePersisted("current_index", 5)
		require.False(t, acc.hasLoadProfile)
	})
}
