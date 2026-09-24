package ingest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fetchInternalTestNow anchors every table case below.
var fetchInternalTestNow = time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC)

// TestShouldEnqueueConsumptionRefresh is a pure, unit-level table test of
// the decision shouldEnqueueConsumptionRefresh implements: R73's
// threshold, amended by R100(1) (hasLoadProfile) and R100(6) (29-day
// threshold, one day of margin inside consumption_hourly's own 30-day
// policy window).
func TestShouldEnqueueConsumptionRefresh(t *testing.T) {
	past := fetchInternalTestNow.Add(-40 * 24 * time.Hour)
	recent := fetchInternalTestNow.Add(-1 * 24 * time.Hour)

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
		{"recent affected range (inside the policy window)", true, true, true, &recent, false},
		{"everything satisfied", true, true, true, &past, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shouldEnqueueConsumptionRefresh(c.hasEnqueuer, c.enabled, c.hasLoadProfile, c.affectedFrom, fetchInternalTestNow)
			require.Equal(t, c.want, got)
		})
	}
}

// TestShouldEnqueueConsumptionRefreshTwentyNineDayBoundary pins R100(6)'s
// exact boundary as a pure, unit-level test (the integration-level proof —
// TestFetchDoesNotEnqueueAtExactlyTwentyNineDaysBoundary /
// TestFetchEnqueuesJustPastTheTwentyNineDayBoundary in
// fetch_refresh_test.go — drives the real pipeline; this pins the boundary
// function itself without a database). A mutation changing
// consumptionRefreshThreshold's 29 back to 30, or Before to
// Before-or-equal, must turn one of these two cases red.
func TestShouldEnqueueConsumptionRefreshTwentyNineDayBoundary(t *testing.T) {
	// Deliberately a LITERAL 29*24h, not consumptionRefreshThreshold itself
	// — comparing against the constant would make this test tautological
	// (it would still pass even if the constant drifted back to 30 days).
	exactlyTwentyNineDays := fetchInternalTestNow.Add(-29 * 24 * time.Hour)
	require.False(t, shouldEnqueueConsumptionRefresh(true, true, true, &exactlyTwentyNineDays, fetchInternalTestNow),
		"exactly 29 days ago is still within the policy window — the boundary is exclusive")

	justPast := exactlyTwentyNineDays.Add(-time.Microsecond)
	require.True(t, shouldEnqueueConsumptionRefresh(true, true, true, &justPast, fetchInternalTestNow),
		"one microsecond past the 29-day threshold must enqueue")
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
