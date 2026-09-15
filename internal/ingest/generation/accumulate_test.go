package generation_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest/generation"
)

// accTestAnalyzerID is fixed so every row built below shares one identity;
// Accumulate never reads AnalyzerID, but a realistic row is easier to reason
// about than a zero-value one.
var accTestAnalyzerID = uuid.MustParse("22222222-2222-4222-8222-222222222222")

// rowsWithIntervals builds n load_profile rows 15 minutes apart starting at
// a fixed base timestamp, one per literal in lits; "" means a nil
// IntervalGenerationKwh (an unavailable interval register, never a zero
// one — 06 §5, removed-behaviour 21).
func rowsWithIntervals(lits ...string) []model.MeterReading {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	rows := make([]model.MeterReading, len(lits))
	for i, lit := range lits {
		r := model.MeterReading{
			AnalyzerID:        accTestAnalyzerID,
			Ts:                base.Add(time.Duration(i) * 15 * time.Minute),
			Kind:              model.ReadingKindLoadProfile,
			MultiplierApplied: decimal.NewFromInt(1),
			SourceProvider:    model.IntegrationProviderPM5340,
		}
		if lit != "" {
			v := decimal.RequireFromString(lit)
			r.IntervalGenerationKwh = &v
		}
		rows[i] = r
	}
	return rows
}

// TestAccumulateSkipsNilAndIsExact is the brief's Step 1 fixture verbatim.
func TestAccumulateSkipsNilAndIsExact(t *testing.T) {
	rows := rowsWithIntervals("0.25", "", "0.50", "0.125") // "" = nil
	got := generation.Accumulate(decimal.RequireFromString("100"), rows)
	want := []string{"100.25", "100.25", "100.75", "100.875"}
	for i := range want {
		require.True(t, decimal.RequireFromString(want[i]).Equal(got[i]), "row %d: %s", i, got[i])
	}
}

// TestAccumulateEmptyRowsReturnsEmpty guards the degenerate case: no rows,
// no output, base untouched.
func TestAccumulateEmptyRowsReturnsEmpty(t *testing.T) {
	got := generation.Accumulate(decimal.RequireFromString("42"), nil)
	require.Empty(t, got)
}

// TestAccumulateAllNilCarriesBaseForward: every row is a nil interval, so
// every output equals base exactly — the running value carries forward
// without ever being coerced to zero.
func TestAccumulateAllNilCarriesBaseForward(t *testing.T) {
	rows := rowsWithIntervals("", "", "")
	got := generation.Accumulate(decimal.RequireFromString("7.5"), rows)
	for i, v := range got {
		require.True(t, decimal.RequireFromString("7.5").Equal(v), "row %d: %s", i, v)
	}
}

// TestAccumulateIsDecimalExactNoFloatDrift proves the running sum accrues
// fractional kWh without float rounding error across many rows — the reason
// this whole package is built on shopspring/decimal, never float64.
func TestAccumulateIsDecimalExactNoFloatDrift(t *testing.T) {
	lits := make([]string, 0, 1000)
	for range 1000 {
		lits = append(lits, "0.1")
	}
	rows := rowsWithIntervals(lits...)
	got := generation.Accumulate(decimal.Zero, rows)
	want := decimal.RequireFromString("100")
	require.True(t, want.Equal(got[len(got)-1]), "want exactly 100, got %s", got[len(got)-1])
}
