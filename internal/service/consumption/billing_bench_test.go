package consumption_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// yearOfFifteenMinuteReadings builds one analyzer-year of 15-minute
// load_profile readings, each carrying a max_demand_kw value so MaxDemand
// has something to scan.
func yearOfFifteenMinuteReadings(analyzerID uuid.UUID, from, to time.Time) []model.MeterReading {
	var rows []model.MeterReading
	v := 1000
	for ts := from.Add(-15 * time.Minute); !ts.After(to); ts = ts.Add(15 * time.Minute) {
		rows = append(rows, readingRow(analyzerID, ts, model.ReadingKindLoadProfile, map[string]string{
			"active_import": fmt.Sprintf("%d", v),
			"max_demand_kw": "5",
		}))
		v++
	}
	return rows
}

// BenchmarkBillingYearHourly benchmarks one analyzer-year of
// 15-minute load_profile readings at Hourly (8760 buckets). Before
// maxDemandInWindow's fix, energy.MaxDemand rescanned the WHOLE loaded
// slice for every bucket (O(buckets x readings)); after, each bucket's own
// sub-range is found with sort.Search and only that bounded sub-slice is
// scanned. Recorded numbers: before ~2.78s/op, after a small fraction of
// that.
func BenchmarkBillingYearHourly(b *testing.B) {
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(b, err)
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, loc)
	to := from.AddDate(1, 0, 0)
	analyzerID := uuid.New()
	rows := yearOfFifteenMinuteReadings(analyzerID, from, to)

	billing, err := consumption.NewBilling(consumption.BillingDeps{
		Readings:  fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{model.ReadingKindLoadProfile: rows}},
		Anomalies: noAnomalies{},
		Ops:       noOps{},
		Clock:     clock.NewFake(to.Add(consumption.SettleDelayHourly)),
		Log:       testLog(b),
	})
	require.NoError(b, err)

	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	req := consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Level:       energy.Hourly,
		Range:       store.TimeRange{From: from, To: to},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := billing.Consumption(ctx, scope, req)
		require.NoError(b, err)
	}
}
