package billing_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
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

func TestResolvePeriodSavedNeighbours(t *testing.T) {
	loc := istanbul(t)
	at := func(m time.Month, d, h int) time.Time { return time.Date(2026, m, d, h, 0, 0, 0, loc) }
	jul := &energy.Window{From: at(7, 17, 16), To: at(8, 18, 16)}
	aug := &energy.Window{From: at(8, 18, 16), To: at(9, 16, 15)}

	// R556: a saved key wins outright.
	w, err := billing.ResolvePeriod("2026-07", 16, nil, jul, nil, loc)
	require.NoError(t, err)
	require.Equal(t, *jul, w)

	// Unsaved key between saved neighbours takes their edges.
	w, err = billing.ResolvePeriod("2026-08", 16, jul, nil, nil, loc)
	require.NoError(t, err)
	require.True(t, w.From.Equal(at(8, 18, 16)) && w.To.Equal(at(9, 16, 0)), "%v", w)
	w, err = billing.ResolvePeriod("2026-06", 16, nil, nil, jul, loc)
	require.NoError(t, err)
	require.True(t, w.From.Equal(at(6, 16, 0)) && w.To.Equal(at(7, 17, 16)), "%v", w)
	w, err = billing.ResolvePeriod("2026-08", 16, jul, nil, &energy.Window{From: at(9, 16, 15), To: at(10, 16, 0)}, loc)
	require.NoError(t, err)
	require.Equal(t, *aug, w)

	// A neighbour that swallows the cut-off window is a conflict.
	_, err = billing.ResolvePeriod("2026-08", 16, &energy.Window{From: at(8, 1, 0), To: at(9, 20, 0)}, nil, nil, loc)
	require.ErrorIs(t, err, billing.ErrPeriodConflict)
}

func TestValidateSavedPeriod(t *testing.T) {
	loc := istanbul(t)
	at := func(m time.Month, d, h, min int) time.Time { return time.Date(2026, m, d, h, min, 0, 0, loc) }
	require.NoError(t, billing.ValidateSavedPeriod(energy.Window{From: at(8, 18, 16, 0), To: at(9, 16, 15, 0)}))
	for _, bad := range []energy.Window{
		{From: at(8, 18, 16, 30), To: at(9, 16, 0, 0)}, // not hour-aligned
		{From: at(9, 16, 0, 0), To: at(8, 18, 0, 0)},   // reversed
		{From: at(7, 1, 0, 0), To: at(9, 2, 0, 0)},     // > 62 days
	} {
		require.ErrorIs(t, billing.ValidateSavedPeriod(bad), billing.ErrInvalidInput, "%v", bad)
	}
}

func TestShiftKeyAndKeyOf(t *testing.T) {
	loc := istanbul(t)
	require.Equal(t, "2026-01", billing.ShiftKey("2025-12", 1))
	require.Equal(t, "2025-12", billing.ShiftKey("2026-01", -1))
	require.Equal(t, "2026-08", billing.KeyOf(time.Date(2026, 8, 18, 16, 0, 0, 0, loc), loc))
	require.Equal(t, "2026-08", billing.KeyOf(time.Date(2026, 7, 31, 22, 0, 0, 0, time.UTC), loc)) // 01:00 Istanbul
}
