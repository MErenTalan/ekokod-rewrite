package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func bd(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }

// hb builds an hourly boundaryBucket whose active_import runs first..last,
// with its first reading offset from the bucket start.
func hb(id uuid.UUID, bucket time.Time, offset time.Duration, first, last string) boundaryBucket {
	b := boundaryBucket{firstTS: bucket.Add(offset)}
	b.b.AnalyzerID = id
	b.b.Bucket = bucket
	b.start[0], b.end[0] = bd(first), bd(last)
	return b
}

func activeOf(t *testing.T, got []boundaryBucket, i int) string {
	t.Helper()
	v := got[i].b.ActiveConsumption
	if v == nil {
		return "nil"
	}
	return v.String()
}

var t0 = time.Date(2025, time.March, 3, 6, 0, 0, 0, time.UTC)

func TestBoundaryHourlyMeterIsNotZero(t *testing.T) {
	id := uuid.New()
	rows := []boundaryBucket{
		hb(id, t0, 0, "100", "100"),
		hb(id, t0.Add(time.Hour), 0, "110", "110"),
		hb(id, t0.Add(2*time.Hour), 0, "125", "125"),
	}
	got := boundaryDifference(energy.Hourly, rows, store.TimeRange{From: t0, To: t0.Add(3 * time.Hour)})
	require.Len(t, got, 3)
	require.Equal(t, "10", activeOf(t, got, 0))
	require.Equal(t, "15", activeOf(t, got, 1))
	require.Equal(t, "0", activeOf(t, got, 2), "no successor: own last − own first")
}

func TestBoundaryQuarterHourMeterCountsTheWholeHour(t *testing.T) {
	id := uuid.New()
	rows := []boundaryBucket{
		hb(id, t0, 0, "100", "103"),                  // 00,15,30,45
		hb(id, t0.Add(time.Hour), 0, "104", "107"),   // step 45→00 is 1
		hb(id, t0.Add(2*time.Hour), 0, "108", "111"), // look-ahead only
	}
	got := boundaryDifference(energy.Hourly, rows, store.TimeRange{From: t0, To: t0.Add(2 * time.Hour)})
	require.Len(t, got, 2, "the look-ahead bucket is dropped")
	require.Equal(t, "4", activeOf(t, got, 0))
	require.Equal(t, "4", activeOf(t, got, 1))
}

func TestBoundaryOffsetMeterTelescopes(t *testing.T) {
	id := uuid.New()
	rows := []boundaryBucket{
		hb(id, t0, 5*time.Minute, "100", "103"),
		hb(id, t0.Add(time.Hour), 5*time.Minute, "104", "107"),
		hb(id, t0.Add(2*time.Hour), 5*time.Minute, "108", "111"),
	}
	got := boundaryDifference(energy.Hourly, rows, store.TimeRange{From: t0.Add(time.Hour), To: t0.Add(3 * time.Hour)})
	require.Len(t, got, 2, "the look-behind bucket is dropped")
	require.Equal(t, "4", activeOf(t, got, 0), "107 − 103: from the previous bucket's last")
	require.Equal(t, "4", activeOf(t, got, 1))
}

func TestBoundaryGapFallsBackToOwnFirst(t *testing.T) {
	id := uuid.New()
	rows := []boundaryBucket{
		hb(id, t0, 5*time.Minute, "100", "103"),
		hb(id, t0.Add(3*time.Hour), 5*time.Minute, "150", "153"),
	}
	got := boundaryDifference(energy.Hourly, rows, store.TimeRange{From: t0, To: t0.Add(4 * time.Hour)})
	require.Equal(t, "3", activeOf(t, got, 0))
	require.Equal(t, "3", activeOf(t, got, 1), "never absorbs the gap")
}

func TestBoundaryAnalyzersDoNotBleed(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	rows := []boundaryBucket{
		hb(a, t0, 0, "100", "100"),
		hb(b, t0.Add(time.Hour), 0, "900", "900"),
	}
	got := boundaryDifference(energy.Hourly, rows, store.TimeRange{From: t0, To: t0.Add(2 * time.Hour)})
	require.Equal(t, "0", activeOf(t, got, 0))
	require.Equal(t, "0", activeOf(t, got, 1))
}

func TestBoundaryNilRegisterStaysNil(t *testing.T) {
	id := uuid.New()
	rows := []boundaryBucket{hb(id, t0, 0, "100", "100"), hb(id, t0.Add(time.Hour), 0, "110", "110")}
	rows[1].start[0] = nil
	got := boundaryDifference(energy.Hourly, rows, store.TimeRange{From: t0, To: t0.Add(time.Hour)})
	require.Equal(t, "0", activeOf(t, got, 0), "a nil successor start falls back to own last")
	require.Nil(t, got[0].b.InductiveConsumption)
}

func TestBoundaryDailyUsesIstanbulDays(t *testing.T) {
	id := uuid.New()
	d0 := time.Date(2025, time.March, 3, 0, 0, 0, 0, istanbulDays)
	rows := []boundaryBucket{
		hb(id, d0, 0, "100", "123"),
		hb(id, d0.AddDate(0, 0, 1), 0, "124", "147"),
	}
	got := boundaryDifference(energy.Daily, rows, store.TimeRange{From: d0, To: d0.AddDate(0, 0, 1)})
	require.Len(t, got, 1)
	require.Equal(t, "24", activeOf(t, got, 0))
}

func TestWidenForBoundaries(t *testing.T) {
	m := time.Date(2025, time.March, 15, 0, 0, 0, 0, istanbulDays)
	got := widenForBoundaries(energy.Monthly, store.TimeRange{From: m, To: m.AddDate(0, 1, 0)})
	require.True(t, got.From.Equal(time.Date(2025, time.March, 1, 0, 0, 0, 0, istanbulDays)), got.From)
	require.True(t, got.To.Equal(time.Date(2025, time.June, 1, 0, 0, 0, 0, istanbulDays)), got.To)
	h := widenForBoundaries(energy.Hourly, store.TimeRange{From: t0, To: t0.Add(time.Hour)})
	require.True(t, h.From.Equal(t0.Add(-time.Hour)))
	require.True(t, h.To.Equal(t0.Add(2*time.Hour)))
}
