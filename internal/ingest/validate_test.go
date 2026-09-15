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

func TestValidateSanityJump(t *testing.T) {
	now := ts("2026-09-01T12:00:00Z")
	o := validateOpts() // SanityMultiple = 10

	history := buildSanityHistory(30, decimal.Zero, decimal.NewFromInt(2)) // median delta = 2.0, threshold = 20
	prev := history[len(history)-1]
	prevVal := *prev.ActiveImport

	t.Run("delta 25 exceeds threshold 20 and is rejected", func(t *testing.T) {
		jumped := reading("2026-09-01T11:00:00Z", map[string]string{"active_import": prevVal.Add(decimal.NewFromInt(25)).String()})
		valid, rejected := ingest.Validate(&prev, history, []model.MeterReading{jumped}, now, o)
		require.Empty(t, valid)
		require.Len(t, rejected, 1)
		require.Equal(t, ingest.RejectSanityJump, rejected[0].Reason)
		require.Equal(t, "active_import", rejected[0].Register)
	})

	t.Run("delta 19 is within threshold 20 and is kept", func(t *testing.T) {
		ok := reading("2026-09-01T11:00:00Z", map[string]string{"active_import": prevVal.Add(decimal.NewFromInt(19)).String()})
		valid, rejected := ingest.Validate(&prev, history, []model.MeterReading{ok}, now, o)
		require.Empty(t, rejected)
		require.Len(t, valid, 1)
	})

	t.Run("23 history deltas is too few and the check is skipped", func(t *testing.T) {
		shortHistory := buildSanityHistory(23, decimal.Zero, decimal.NewFromInt(2))
		shortPrev := shortHistory[len(shortHistory)-1]
		shortPrevVal := *shortPrev.ActiveImport
		huge := reading("2026-09-01T11:00:00Z", map[string]string{"active_import": shortPrevVal.Add(decimal.NewFromInt(1000)).String()})
		valid, rejected := ingest.Validate(&shortPrev, shortHistory, []model.MeterReading{huge}, now, o)
		require.Empty(t, rejected, "fewer than 30 deltas: the sanity check must not run at all")
		require.Len(t, valid, 1)
	})
}

func TestValidateSanityJumpStopsAfterThreeConsecutive(t *testing.T) {
	now := ts("2026-09-01T18:00:00Z")
	o := validateOpts()

	history := buildSanityHistory(30, decimal.Zero, decimal.NewFromInt(2)) // threshold 20
	prev := history[len(history)-1]
	prevVal := *prev.ActiveImport

	var rows []model.MeterReading
	for i := range 4 {
		tsN := ts("2026-09-01T13:00:00Z").Add(time.Duration(i) * time.Hour)
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
