package analysis_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/analysis"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
)

func dp(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }

func TestSumBuildingRows(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	w1 := energy.Window{From: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	w2 := energy.Window{From: w1.To, To: w1.To.Add(24 * time.Hour)}
	rows := []consumption.Row{
		{AnalyzerID: a, Window: w1, Values: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport: dp("100"), energy.ReactiveInductiveImport: dp("20"), energy.ReactiveCapacitiveImport: dp("4")},
			MaxDemandKw: dp("12"), Indexes: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dp("5000")}},
		{AnalyzerID: b, Window: w1, Values: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport: dp("300"), energy.ReactiveInductiveImport: dp("80"), energy.ReactiveCapacitiveImport: nil},
			MaxDemandKw: dp("30")},
		{AnalyzerID: a, Window: w2, Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dp("50")},
			Suspect: map[energy.Register]energy.Suspicion{energy.ActiveImport: {}}},
	}
	out := analysis.SumByWindow(rows, 2)
	require.Len(t, out, 2)
	first := out[0]
	require.Equal(t, "400", first.Values[energy.ActiveImport].String())
	require.Equal(t, "100", first.Values[energy.ReactiveInductiveImport].String())
	require.Nil(t, first.Values[energy.ReactiveCapacitiveImport], "one analyzer missing capacitive: no building value")
	require.True(t, first.Partial)
	require.Equal(t, "0.25", first.InductiveRatio.String())
	require.Nil(t, first.CapacitiveRatio)
	require.Equal(t, "30", first.MaxDemandKw.String())
	require.Nil(t, first.Indexes, "building rows carry no indexes")

	second := out[1]
	require.Nil(t, second.Values[energy.ActiveImport], "a suspect value is not summed")
	require.True(t, second.Partial, "analyzer b has no row for the window")

	require.Empty(t, analysis.SumByWindow(nil, 2))
}
