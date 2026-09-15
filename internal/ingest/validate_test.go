package ingest_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
)

func validateOpts() ingest.Options {
	return ingest.Options{FutureTolerance: ingest.DefaultFutureTolerance, SanityMultiple: ingest.DefaultSanityMultiple()}
}

func TestValidateRejectsFutureBeyondTolerance(t *testing.T) {
	now := ts("2026-09-01T12:00:00Z")

	within := reading("2026-09-01T12:14:00Z", map[string]string{"active_import": "10.0000"})
	beyond := reading("2026-09-01T12:16:00Z", map[string]string{"active_import": "10.0000"})

	valid, rejected := ingest.Validate(nil, nil, []model.MeterReading{within, beyond}, now, validateOpts())

	require.Len(t, valid, 1)
	require.True(t, valid[0].Ts.Equal(within.Ts))
	require.Len(t, rejected, 1)
	require.Equal(t, ingest.RejectFuture, rejected[0].Reason)
	require.True(t, rejected[0].Ts.Equal(beyond.Ts))
}

func TestValidateRejectsAllNull(t *testing.T) {
	now := ts("2026-09-01T12:00:00Z")
	allNull := model.MeterReading{AnalyzerID: testAnalyzerID, Ts: ts("2026-09-01T11:00:00Z"), Kind: model.ReadingKindLoadProfile}
	ok := reading("2026-09-01T11:00:00Z", map[string]string{"active_import": "5.0000"})

	valid, rejected := ingest.Validate(nil, nil, []model.MeterReading{allNull, ok}, now, validateOpts())

	require.Len(t, valid, 1)
	require.Len(t, rejected, 1)
	require.Equal(t, ingest.RejectAllNull, rejected[0].Reason)
}

func TestValidateRejectsNegativeValue(t *testing.T) {
	now := ts("2026-09-01T12:00:00Z")
	negative := reading("2026-09-01T11:00:00Z", map[string]string{"active_import": "-1.0000"})
	ok := reading("2026-09-01T11:05:00Z", map[string]string{"active_import": "5.0000"})

	valid, rejected := ingest.Validate(nil, nil, []model.MeterReading{negative, ok}, now, validateOpts())

	require.Len(t, valid, 1)
	require.Len(t, rejected, 1)
	require.Equal(t, ingest.RejectNegative, rejected[0].Reason)
	require.Equal(t, "active_import", rejected[0].Register)
}

func TestValidateDoesNotApplyCommissioningDate(t *testing.T) {
	now := ts("2026-09-01T12:00:00Z")
	old := reading("1990-01-01T00:00:00Z", map[string]string{"active_import": "5.0000"})

	valid, rejected := ingest.Validate(nil, nil, []model.MeterReading{old}, now, validateOpts())

	require.Empty(t, rejected, "R11: the commissioning-date rule is not applied")
	require.Len(t, valid, 1)
}

// buildSanityHistory returns n+1 hourly readings of active_import starting
// at startVal and stepping by step each hour, so registerDeltas sees exactly
// n identical deltas of |step| — median(deltas) == |step|.
func buildSanityHistory(n int, startVal, step decimal.Decimal) []model.MeterReading {
	base := ts("2026-08-25T00:00:00Z")
	out := make([]model.MeterReading, n+1)
	val := startVal
	for i := 0; i <= n; i++ {
		tsN := base.Add(time.Duration(i) * time.Hour)
		v := val
		out[i] = reading(tsN.Format(time.RFC3339), map[string]string{"active_import": v.String()})
		val = val.Add(step)
	}
	return out
}

// buildSanityHistoryFlatThenStep returns nFlat consecutive IDENTICAL
// readings (nFlat zero-delta pairs) followed by nStep readings that each
// increase by step (nStep positive-delta pairs, every pair still 1h apart,
// the same cadence as the flat run). R13: sanityThresholds' median must be
// computed from only the nStep POSITIVE deltas — the nFlat zero ones (an
// idle register) must never pull it down.
func buildSanityHistoryFlatThenStep(nFlat, nStep int, startVal, step decimal.Decimal) []model.MeterReading {
	base := ts("2026-08-20T00:00:00Z")
	out := make([]model.MeterReading, 0, nFlat+1+nStep)
	val := startVal
	i := 0
	for ; i <= nFlat; i++ {
		out = append(out, reading(base.Add(time.Duration(i)*time.Hour).Format(time.RFC3339), map[string]string{"active_import": val.String()}))
	}
	for j := 0; j < nStep; j++ {
		val = val.Add(step)
		out = append(out, reading(base.Add(time.Duration(i)*time.Hour).Format(time.RFC3339), map[string]string{"active_import": val.String()}))
		i++
	}
	return out
}

// TestValidateSanityJump pins R13's core arithmetic: limit =
// max(multiple × median(positive deltas) × elapsedIntervals, 1), with
// elapsedIntervals == 1 here (the candidate sits exactly one history
// interval — 1h — after prev, matching history's own hourly cadence), so
// limit collapses to multiple × median == 10 × 2 == 20, same as this
// suite's original numbers.
func TestValidateSanityJump(t *testing.T) {
	now := ts("2026-09-01T12:00:00Z")
	o := validateOpts() // SanityMultiple = 10

	// 24 hourly deltas of 2.0 each: median = 2.0, threshold (elapsed == 1
	// interval) == 10 * 2.0 * 1 == 20. R13 pins the minimum at 24 deltas.
	history := buildSanityHistory(24, decimal.Zero, decimal.NewFromInt(2))
	prev := history[len(history)-1]
	prevVal := *prev.ActiveImport
	next := prev.Ts.Add(time.Hour) // elapsedIntervals == 1

	t.Run("delta 25 exceeds threshold 20 and is rejected", func(t *testing.T) {
		jumped := reading(next.Format(time.RFC3339), map[string]string{"active_import": prevVal.Add(decimal.NewFromInt(25)).String()})
		valid, rejected := ingest.Validate(&prev, history, []model.MeterReading{jumped}, now, o)
		require.Empty(t, valid)
		require.Len(t, rejected, 1)
		require.Equal(t, ingest.RejectSanityJump, rejected[0].Reason)
		require.Equal(t, "active_import", rejected[0].Register)
	})

	t.Run("delta 19 is within threshold 20 and is kept", func(t *testing.T) {
		ok := reading(next.Format(time.RFC3339), map[string]string{"active_import": prevVal.Add(decimal.NewFromInt(19)).String()})
		valid, rejected := ingest.Validate(&prev, history, []model.MeterReading{ok}, now, o)
		require.Empty(t, rejected)
		require.Len(t, valid, 1)
	})

	t.Run("23 history deltas is too few and the check is skipped", func(t *testing.T) {
		shortHistory := buildSanityHistory(23, decimal.Zero, decimal.NewFromInt(2))
		shortPrev := shortHistory[len(shortHistory)-1]
		shortPrevVal := *shortPrev.ActiveImport
		huge := reading(shortPrev.Ts.Add(time.Hour).Format(time.RFC3339), map[string]string{"active_import": shortPrevVal.Add(decimal.NewFromInt(1000)).String()})
		valid, rejected := ingest.Validate(&shortPrev, shortHistory, []model.MeterReading{huge}, now, o)
		require.Empty(t, rejected, "fewer than 24 POSITIVE deltas: the sanity check must not run at all")
		require.Len(t, valid, 1)
	})
}

// TestValidateSanityJumpScalesWithElapsedGap proves R13's elapsedIntervals
// scaling (C1, mutation: drop the ×elapsedIntervals factor): a 4-hour gap
// (4 typical hourly intervals) since the effective previous reading must
// widen the allowance to 4× the single-interval limit, so an otherwise
// implausible-looking delta that is really just four ordinary intervals'
// worth of consumption is accepted.
func TestValidateSanityJumpScalesWithElapsedGap(t *testing.T) {
	now := ts("2026-09-01T12:00:00Z")
	o := validateOpts() // SanityMultiple = 10

	history := buildSanityHistory(24, decimal.Zero, decimal.NewFromInt(2)) // median 2.0, single-interval threshold 20
	prev := history[len(history)-1]
	prevVal := *prev.ActiveImport

	// 4 hours later == 4 typical (1h) intervals elapsed: limit ==
	// 10 * 2.0 * 4 == 80. A delta of 40 is four ordinary 10-per-interval
	// jumps' worth of gap-catchup consumption, well under 80 — but it
	// WOULD exceed the naive single-interval limit of 20 without the
	// elapsedIntervals factor, which is exactly the deviation from R13
	// this test pins.
	afterGap := reading(prev.Ts.Add(4*time.Hour).Format(time.RFC3339), map[string]string{"active_import": prevVal.Add(decimal.NewFromInt(40)).String()})
	valid, rejected := ingest.Validate(&prev, history, []model.MeterReading{afterGap}, now, o)
	require.Empty(t, rejected, "a 4h gap must scale the limit to 4x, accepting normal catch-up consumption")
	require.Len(t, valid, 1)
}

// TestValidateSanityJumpFloorAtOne proves R13's max(..., 1) floor (C1,
// mutation: drop the floor): a register whose typical delta is tiny
// (0.001) produces a raw limit (10 * 0.001 * 1 == 0.01) far below 1 — the
// floor is what stops that raw limit from rejecting a perfectly ordinary
// reading whose own delta (0.5) is nowhere near implausible in absolute
// terms.
func TestValidateSanityJumpFloorAtOne(t *testing.T) {
	now := ts("2026-09-01T12:00:00Z")
	o := validateOpts() // SanityMultiple = 10

	history := buildSanityHistory(24, decimal.Zero, decimal.RequireFromString("0.001")) // median 0.001, raw limit 0.01
	prev := history[len(history)-1]
	prevVal := *prev.ActiveImport

	row := reading(prev.Ts.Add(time.Hour).Format(time.RFC3339), map[string]string{"active_import": prevVal.Add(decimal.RequireFromString("0.5")).String()})
	valid, rejected := ingest.Validate(&prev, history, []model.MeterReading{row}, now, o)
	require.Empty(t, rejected, "the floor of 1 must win over a raw limit (0.01) smaller than it")
	require.Len(t, valid, 1)
}

// TestValidateSanityJumpMedianExcludesIdlePeriods proves R13's "median over
// POSITIVE deltas only" (C1, mutation: include zero/negative deltas in the
// median): a register that is mostly idle (40 zero-delta hours) but
// genuinely steps by 5 every so often (24 such steps, meeting the 24-sample
// minimum on its own) must have its threshold sized from THOSE 24 steps
// (median 5, threshold 50) — not diluted toward zero by the 64 total
// samples, which is what a median computed over every delta (idle hours
// included) would do.
func TestValidateSanityJumpMedianExcludesIdlePeriods(t *testing.T) {
	now := ts("2026-09-01T12:00:00Z")
	o := validateOpts() // SanityMultiple = 10

	history := buildSanityHistoryFlatThenStep(40, 24, decimal.Zero, decimal.NewFromInt(5))
	prev := history[len(history)-1]
	prevVal := *prev.ActiveImport

	// delta 40 < the correct threshold 50 (10 * median-of-24-positive-fives
	// * 1 interval), but WOULD exceed a threshold computed by including the
	// 40 idle zero-deltas (which pulls the median toward 0).
	row := reading(prev.Ts.Add(time.Hour).Format(time.RFC3339), map[string]string{"active_import": prevVal.Add(decimal.NewFromInt(40)).String()})
	valid, rejected := ingest.Validate(&prev, history, []model.MeterReading{row}, now, o)
	require.Empty(t, rejected, "the median must come from the 24 positive deltas alone, not diluted by 40 idle zero-deltas")
	require.Len(t, valid, 1)
}

// TestValidateSanityJumpNeverRejectsADecrease proves R13's "only an INCREASE
// is a sanity jump" (C1, mutation: Abs() the tested delta): a huge DECREASE
// (50000 -> 5) is never a RejectSanityJump — a decrease is R14's concern
// (DetectNegativeDeltas / an unresolved consumption_anomalies row, proved
// separately by TestIngestionNegativeIndexDeltaCreatesUnresolvedAnomaly),
// and the reading is stored either way.
func TestValidateSanityJumpNeverRejectsADecrease(t *testing.T) {
	now := ts("2026-09-01T12:00:00Z")
	o := validateOpts() // SanityMultiple = 10

	history := buildSanityHistory(24, decimal.Zero, decimal.NewFromInt(2)) // threshold 20 at elapsed == 1
	prev := history[len(history)-1]
	prev.ActiveImport = decimalPtr(decimal.NewFromInt(50000))

	decrease := reading(prev.Ts.Add(time.Hour).Format(time.RFC3339), map[string]string{"active_import": "5"})
	valid, rejected := ingest.Validate(&prev, history, []model.MeterReading{decrease}, now, o)
	require.Empty(t, rejected, "a decrease, however large, is never a sanity jump")
	require.Len(t, valid, 1)
}

func decimalPtr(d decimal.Decimal) *decimal.Decimal { return &d }

// TestValidateSanityJumpStopsAfterThreeConsecutive pins the WITHIN-ONE-CALL
// shape of R13's streak (Options.SanityStreak left nil, so Validate falls
// back to a call-scoped streak): three consecutive jumps rejected, the
// fourth kept and flagged "sustained". Each row's gap from the unchanged
// effective-previous-reading (rejected rows never update it) grows by 1h
// per row, so the limit scales with it too (20, 40, 60, 80) — delta 100
// still exceeds all four, so all four are still jumps.
func TestValidateSanityJumpStopsAfterThreeConsecutive(t *testing.T) {
	now := ts("2026-09-01T18:00:00Z")
	o := validateOpts()

	history := buildSanityHistory(24, decimal.Zero, decimal.NewFromInt(2)) // threshold 20 at elapsed == 1
	prev := history[len(history)-1]
	prevVal := *prev.ActiveImport

	var rows []model.MeterReading
	for i := range 4 {
		tsN := prev.Ts.Add(time.Duration(i+1) * time.Hour)
		rows = append(rows, reading(tsN.Format(time.RFC3339), map[string]string{
			"active_import": prevVal.Add(decimal.NewFromInt(100)).String(),
		}))
	}

	valid, rejected := ingest.Validate(&prev, history, rows, now, o)

	require.Len(t, valid, 1, "only the fourth, sustained jump is kept")
	require.True(t, valid[0].Ts.Equal(rows[3].Ts))

	require.Len(t, rejected, 4)
	for i := range 3 {
		require.Equal(t, ingest.RejectSanityJump, rejected[i].Reason, fmt.Sprintf("rejection %d", i))
		require.Equal(t, "active_import", rejected[i].Register)
	}
	require.Equal(t, ingest.RejectSanityJump, rejected[3].Reason)
	require.Equal(t, "sustained", rejected[3].Register, "the fourth consecutive jump is flagged, not silently accepted")
	require.True(t, rejected[3].Ts.Equal(rows[3].Ts))
}

// TestValidateSanityJumpStreakPersistsAcrossPages proves R13's "at most
// three consecutive rejections per analyzer PER RUN" (C1, mutation: reset
// the streak per page/call instead of sharing Options.SanityStreak): two
// SEPARATE Validate calls (simulating two fetch pages of the same run)
// sharing one *ingest.SanityStreak must not each get their own "three
// strikes" — the streak's third rejection in call 1 plus its first row in
// call 2 must already be past the limit, so call 2's own first jump is the
// STREAK's fourth (flagged, accepted), and every jump after that in the
// same run — the streak never re-arms — is accepted silently, with no
// further flag and no reject-3-accept-1 cycling.
func TestValidateSanityJumpStreakPersistsAcrossPages(t *testing.T) {
	now := ts("2026-09-01T18:00:00Z")
	o := validateOpts()
	o.SanityStreak = &ingest.SanityStreak{}

	history := buildSanityHistory(24, decimal.Zero, decimal.NewFromInt(2)) // threshold 20 at elapsed == 1
	prev := history[len(history)-1]
	prevVal := *prev.ActiveImport

	jumpRow := func(hoursAfterPrev int) model.MeterReading {
		tsN := prev.Ts.Add(time.Duration(hoursAfterPrev) * time.Hour)
		return reading(tsN.Format(time.RFC3339), map[string]string{"active_import": prevVal.Add(decimal.NewFromInt(500)).String()})
	}

	// Page 1: two consecutive jumps — both rejected (streak 1, 2). Neither
	// is kept, so `prev` for page 2 (below) stays the SAME reading, exactly
	// as fetch.go leaves it when a page's `valid` is empty.
	page1 := []model.MeterReading{jumpRow(1), jumpRow(2)}
	valid1, rejected1 := ingest.Validate(&prev, history, page1, now, o)
	require.Empty(t, valid1)
	require.Len(t, rejected1, 2)
	for _, r := range rejected1 {
		require.Equal(t, ingest.RejectSanityJump, r.Reason)
		require.Equal(t, "active_import", r.Register)
	}

	// Page 2: row 3 is the streak's THIRD consecutive rejection; row 4 is
	// its FOURTH (flagged "sustained", kept, streak trips); row 5 must be
	// accepted silently — no flag, no re-arming.
	page2 := []model.MeterReading{jumpRow(3), jumpRow(4), jumpRow(5)}
	valid2, rejected2 := ingest.Validate(&prev, history, page2, now, o)

	require.Len(t, valid2, 2, "the sustained (4th) jump and everything after it in the run are kept")
	require.True(t, valid2[0].Ts.Equal(page2[1].Ts))
	require.True(t, valid2[1].Ts.Equal(page2[2].Ts))

	require.Len(t, rejected2, 2, "row 3 (real reject) + row 4 (sustained flag) — row 5 gets no rejection entry at all")
	require.Equal(t, ingest.RejectSanityJump, rejected2[0].Reason)
	require.Equal(t, "active_import", rejected2[0].Register)
	require.True(t, rejected2[0].Ts.Equal(page2[0].Ts))
	require.Equal(t, ingest.RejectSanityJump, rejected2[1].Reason)
	require.Equal(t, "sustained", rejected2[1].Register)
	require.True(t, rejected2[1].Ts.Equal(page2[1].Ts))
}
