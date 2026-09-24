package energy_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

func TestActivityWindowIsSevenDays(t *testing.T) {
	require.Equal(t, 7*24*time.Hour, energy.ActivityWindow)
}

func TestActivityStatusSixDaysAgoIsActive(t *testing.T) {
	now := time.Date(2025, 3, 14, 9, 0, 0, 0, time.UTC)
	last := now.AddDate(0, 0, -6)
	require.Equal(t, energy.StatusActive, energy.ActivityStatus(&last, now))
}

func TestActivityStatusEightDaysAgoIsPassive(t *testing.T) {
	now := time.Date(2025, 3, 14, 9, 0, 0, 0, time.UTC)
	last := now.AddDate(0, 0, -8)
	require.Equal(t, energy.StatusPassive, energy.ActivityStatus(&last, now))
}

func TestActivityStatusNilLastReadingIsPassive(t *testing.T) {
	now := time.Date(2025, 3, 14, 9, 0, 0, 0, time.UTC)
	require.Equal(t, energy.StatusPassive, energy.ActivityStatus(nil, now))
}

// TestActivityStatusExactlyAtTheBoundaryIsActive pins the boundary
// inclusivity ActivityStatus chose: "within the last 7 days" (02 §3.6) is
// implemented as <= ActivityWindow, so a last reading exactly ActivityWindow
// ago is still active.
func TestActivityStatusExactlyAtTheBoundaryIsActive(t *testing.T) {
	now := time.Date(2025, 3, 14, 9, 0, 0, 0, time.UTC)
	last := now.Add(-energy.ActivityWindow)
	require.Equal(t, energy.StatusActive, energy.ActivityStatus(&last, now))
}
