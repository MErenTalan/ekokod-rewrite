package billing_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
)

func TestPeriodCutoffExamples(t *testing.T) {
	loc := istanbul(t)
	day := func(y int, m time.Month, dd int) time.Time { return time.Date(y, m, dd, 0, 0, 0, 0, loc) }
	cases := []struct {
		key      string
		cutoff   int
		from, to time.Time
		days     int
	}{
		{"2025-12", 1, day(2025, 12, 1), day(2026, 1, 1), 31},
		{"2025-12", 15, day(2025, 12, 15), day(2026, 1, 15), 31},
		{"2026-02", 31, day(2026, 2, 28), day(2026, 3, 31), 31},
		{"2026-01", 31, day(2026, 1, 31), day(2026, 2, 28), 28},
		{"2024-02", 30, day(2024, 2, 29), day(2024, 3, 30), 30},
	}
	for _, c := range cases {
		w, err := billing.Period(c.key, c.cutoff, loc)
		require.NoError(t, err)
		require.True(t, w.From.Equal(c.from), "%s/%d from %s", c.key, c.cutoff, w.From)
		require.True(t, w.To.Equal(c.to), "%s/%d to %s", c.key, c.cutoff, w.To)
		require.Equal(t, c.days, billing.DaysInPeriod(w, loc))
	}
	for _, bad := range []string{"2025-13", "2025-1", "25-12-01", "2025/12", "+025-12"} {
		_, err := billing.Period(bad, 1, loc)
		require.ErrorIs(t, err, billing.ErrInvalidInput, bad)
	}
	_, err := billing.Period("2025-12", 32, loc)
	require.ErrorIs(t, err, billing.ErrInvalidInput)
}

func TestDaysInPeriodNeverAddsOne(t *testing.T) {
	loc := istanbul(t)
	w, err := billing.Period("2025-12", 1, loc)
	require.NoError(t, err)
	require.Equal(t, 31, billing.DaysInPeriod(w, loc))
}

func TestLatestClosedPeriodKey(t *testing.T) {
	loc := istanbul(t)
	settle := 72 * time.Hour
	require.Equal(t, "2025-12", billing.LatestClosedPeriodKey(1, time.Date(2026, 2, 3, 23, 59, 0, 0, loc), settle, loc))
	require.Equal(t, "2026-01", billing.LatestClosedPeriodKey(1, time.Date(2026, 2, 4, 0, 0, 0, 0, loc), settle, loc))
	require.Equal(t, "2025-12", billing.LatestClosedPeriodKey(15, time.Date(2026, 2, 17, 23, 0, 0, 0, loc), settle, loc))
	require.Equal(t, "2026-01", billing.LatestClosedPeriodKey(15, time.Date(2026, 2, 18, 0, 0, 0, 0, loc), settle, loc))
}
