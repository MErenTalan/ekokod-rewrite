package energy_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

func TestBucketMonthlyUsesIstanbulCalendar(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	// 2025-03-01T00:00+03:00 is 2025-02-28T21:00Z: a UTC-based implementation
	// would put this instant in February.
	ts := time.Date(2025, 2, 28, 22, 30, 0, 0, time.UTC)
	got := energy.Bucket(energy.Monthly, ts, loc)
	require.Equal(t, time.Date(2025, 3, 1, 0, 0, 0, 0, loc), got.From.In(loc))
	require.Equal(t, time.Date(2025, 4, 1, 0, 0, 0, 0, loc), got.To.In(loc))
}

func TestBucketDailyCrossesAHistoricalDSTTransition(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	// 2015-03-29: Türkiye still observed DST; the local day is 23 hours long.
	w := energy.Bucket(energy.Daily, time.Date(2015, 3, 29, 12, 0, 0, 0, loc), loc)
	require.Equal(t, 23*time.Hour, w.To.Sub(w.From))
}
