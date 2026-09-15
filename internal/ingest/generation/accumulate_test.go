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

// TestAccumulateBackwardInvertsAccumulate is the round-trip proof in
// AccumulateBackward's own doc: seeding refVal with Accumulate's own last
// output for a row set, and sumAfter with zero, must reproduce Accumulate's
// per-row outputs exactly.
//
// Hand derivation (R52's Σ formula, by hand, right to left over
// rows=[0.25, nil, 0.50, 0.125], refVal=100.875, sumAfter starts at 0):
//   - row[3] (0.125): out = 100.875 − 0       = 100.875; sumAfter -> 0.125
//   - row[2] (0.50):  out = 100.875 − 0.125   = 100.75;  sumAfter -> 0.625
//   - row[1] (nil):   out = 100.875 − 0.625   = 100.25;  sumAfter unchanged (nil adds nothing)
//   - row[0] (0.25):  out = 100.875 − 0.625   = 100.25;  sumAfter -> 0.875
//
// which is [100.25, 100.25, 100.75, 100.875] — byte-identical to
// TestAccumulateSkipsNilAndIsExact's forward result for the same rows and
// base 100. Final sumAfter (0.875) is the total of every non-nil interval
// (0.25+0.50+0.125), matching Accumulate's own base-to-final delta.
func TestAccumulateBackwardInvertsAccumulate(t *testing.T) {
	rows := rowsWithIntervals("0.25", "", "0.50", "0.125")
	base := decimal.RequireFromString("100")
	forward := generation.Accumulate(base, rows)
	refVal := forward[len(forward)-1]
	require.True(t, decimal.RequireFromString("100.875").Equal(refVal))

	backward, sumAfter := generation.AccumulateBackward(refVal, decimal.Zero, rows)
	require.Len(t, backward, len(forward))
	for i := range forward {
		require.True(t, forward[i].Equal(backward[i]),
			"row %d: forward %s != backward %s", i, forward[i], backward[i])
	}
	require.True(t, decimal.RequireFromString("0.875").Equal(sumAfter),
		"final sumAfter must equal the total non-nil interval sum: got %s", sumAfter)
}

// TestAccumulateBackwardOperatorAnchor is R52's hand-derived operator/
// meter-set scenario, computed independently of the round-trip proof above:
// an anchor of 100 (a real meter-set reading, NOT synthetic-zero) at 10:00,
// with three OLDER rows discovered by a backfill — 09:00 (1 kWh), 09:15
// (2 kWh), 09:30 (1.5 kWh) — and nothing else between 09:30 and 10:00.
//
// By hand, right to left (Σ over (t, 10:00]):
//   - 09:30: Σ(09:30,10:00] = 0 (nothing between)         -> 100 − 0   = 100
//   - 09:15: Σ(09:15,10:00] = interval(09:30) = 1.5        -> 100 − 1.5 = 98.5
//   - 09:00: Σ(09:00,10:00] = interval(09:15)+interval(09:30) = 3.5 -> 100 − 3.5 = 96.5
//
// Forward sanity check: 96.5 + 1 (09:00) = 97.5; + 2 (09:15) = 99.5; + 1.5
// (09:30) = 101 ... that is NOT 100, which is expected and correct: the
// anchor's own 100 already accounts for everything up to and including
// 09:30 (Σ formula's own semantics — see the package doc), so re-adding
// 09:30's own interval on top would double-count it. The real invariant is
// active_export(09:30) alone must equal refVal exactly when nothing sits
// between it and the anchor, which the derivation above shows.
func TestAccumulateBackwardOperatorAnchor(t *testing.T) {
	rows := rowsWithIntervals("1", "2", "1.5") // 09:00, 09:15, 09:30
	refVal := decimal.RequireFromString("100")

	got, sumAfter := generation.AccumulateBackward(refVal, decimal.Zero, rows)
	require.Len(t, got, 3)
	require.True(t, decimal.RequireFromString("96.5").Equal(got[0]), "09:00: got %s", got[0])
	require.True(t, decimal.RequireFromString("98.5").Equal(got[1]), "09:15: got %s", got[1])
	require.True(t, decimal.RequireFromString("100").Equal(got[2]), "09:30: got %s", got[2])
	require.True(t, decimal.RequireFromString("4.5").Equal(sumAfter), "sumAfter: got %s", sumAfter)
}

// TestAccumulateBackwardChunkingCarriesSumAfter proves deriveOlderRows'
// chunked-call pattern (nearest-to-anchor chunk first, sumAfter threaded
// into the next, farther-back chunk) produces the same result as one
// unchunked call — same three rows as TestAccumulateBackwardOperatorAnchor,
// split into a [09:30] chunk (nearest the anchor) and a [09:00,09:15] chunk
// (farther back), processed in that order.
func TestAccumulateBackwardChunkingCarriesSumAfter(t *testing.T) {
	rows := rowsWithIntervals("1", "2", "1.5") // 09:00, 09:15, 09:30
	refVal := decimal.RequireFromString("100")

	nearChunk := rows[2:3] // 09:30
	farChunk := rows[0:2]  // 09:00, 09:15

	nearOut, sumAfter := generation.AccumulateBackward(refVal, decimal.Zero, nearChunk)
	require.True(t, decimal.RequireFromString("100").Equal(nearOut[0]))
	require.True(t, decimal.RequireFromString("1.5").Equal(sumAfter))

	farOut, finalSumAfter := generation.AccumulateBackward(refVal, sumAfter, farChunk)
	require.True(t, decimal.RequireFromString("96.5").Equal(farOut[0]), "09:00: got %s", farOut[0])
	require.True(t, decimal.RequireFromString("98.5").Equal(farOut[1]), "09:15: got %s", farOut[1])
	require.True(t, decimal.RequireFromString("4.5").Equal(finalSumAfter))
}
