package consumption_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
)

// summaryWindow builds a Window from an hour offset off a fixed anchor, so
// this file's fixtures can talk about "row 0", "row 1", … without repeating
// full timestamps, and so windows sort the same way the rows are declared.
var summaryT0 = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

func summaryWindow(hourOffset int) energy.Window {
	from := summaryT0.Add(time.Duration(hourOffset) * time.Hour)
	return energy.Window{From: from, To: from.Add(time.Hour)}
}

func TestSummariseLeavesATotalNilWhenNoRowReportedTheRegister(t *testing.T) {
	rows := []consumption.Row{
		{
			Window: summaryWindow(0),
			Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("10")},
		},
	}
	s := consumption.Summarise(rows)
	require.Nil(t, s.Totals[energy.ReactiveInductiveImport], "no row ever reported this register")
	require.Nil(t, s.Averages[energy.ReactiveInductiveImport])
}

// TestSummariseGivesANilTotalWhenARegisterIsReportedOnlyBySuspectRows is the
// plan's Step 1 fixture verbatim (Task 9): every row that carries
// active_import is suspect for it; every other row simply never reports it.
// The total must be nil, not the sum of the sound registers' worth of zero
// contribution from the suspect rows.
func TestSummariseGivesANilTotalWhenARegisterIsReportedOnlyBySuspectRows(t *testing.T) {
	rows := []consumption.Row{
		{
			Window:  summaryWindow(0),
			Values:  map[energy.Register]*decimal.Decimal{},
			Suspect: map[energy.Register]energy.Suspicion{energy.ActiveImport: {Reason: energy.ReasonNegativeDelta}},
		},
		{
			Window:  summaryWindow(1),
			Values:  map[energy.Register]*decimal.Decimal{},
			Suspect: map[energy.Register]energy.Suspicion{energy.ActiveImport: {Reason: energy.ReasonNegativeDelta}},
		},
	}
	s := consumption.Summarise(rows)
	require.Nil(t, s.Totals[energy.ActiveImport])
	require.Equal(t, 2, s.Suspect)
}

func TestSummariseCountsSuspectRowsAndExcludesThemFromTotals(t *testing.T) {
	rows := []consumption.Row{
		{
			Window: summaryWindow(0),
			Values: map[energy.Register]*decimal.Decimal{
				energy.ActiveImport:            dec("10"),
				energy.ReactiveInductiveImport: dec("2"),
			},
		},
		{
			Window: summaryWindow(1),
			Values: map[energy.Register]*decimal.Decimal{
				energy.ActiveImport: dec("5"),
				// reactive_inductive_import is nil (unreported) here, but the
				// register itself is NOT suspect on this row.
			},
			Suspect: map[energy.Register]energy.Suspicion{energy.T1Import: {Reason: energy.ReasonMissingReadings}},
		},
	}
	s := consumption.Summarise(rows)
	require.Equal(t, "15", s.Totals[energy.ActiveImport].String())
	require.Equal(t, "2", s.Totals[energy.ReactiveInductiveImport].String(), "only row 0 reported this register")
	require.Equal(t, 1, s.Suspect, "only row 1 carries a suspect register")
	require.Equal(t, 2, s.Rows)
}

func TestAveragesDividesOnlyByContributingRowsNotAllRows(t *testing.T) {
	// I-mutation (b): active_import is reported by rows 0 and 1 (values 10
	// and 20 -> average 15), but reactive_inductive_import is reported by
	// row 0 only (value 6 -> average 6, not 6/3 = 2 across all three rows).
	rows := []consumption.Row{
		{
			Window: summaryWindow(0),
			Values: map[energy.Register]*decimal.Decimal{
				energy.ActiveImport:            dec("10"),
				energy.ReactiveInductiveImport: dec("6"),
			},
		},
		{
			Window: summaryWindow(1),
			Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("20")},
		},
		{
			Window: summaryWindow(2),
			Values: map[energy.Register]*decimal.Decimal{},
		},
	}
	s := consumption.Summarise(rows)
	require.Equal(t, "15", s.Averages[energy.ActiveImport].String())
	require.Equal(t, "6", s.Averages[energy.ReactiveInductiveImport].String(), "divided by the ONE row that contributed, not by all three rows")
}

func TestPeakAndValleyIgnoreSuspectRows(t *testing.T) {
	rows := []consumption.Row{
		{
			Window:  summaryWindow(0),
			Values:  map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("100")},
			Suspect: map[energy.Register]energy.Suspicion{energy.ActiveImport: {Reason: energy.ReasonNegativeDelta}},
		},
		{
			Window: summaryWindow(1),
			Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("5")},
		},
		{
			Window: summaryWindow(2),
			Values: map[energy.Register]*decimal.Decimal{}, // nil active_import
		},
		{
			Window: summaryWindow(3),
			Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("8")},
		},
	}
	s := consumption.Summarise(rows)
	require.NotNil(t, s.Peak)
	require.Equal(t, "8", s.Peak.Values[energy.ActiveImport].String(), "row 0 (100) is suspect and must be ignored")
	require.NotNil(t, s.Valley)
	require.Equal(t, "5", s.Valley.Values[energy.ActiveImport].String())
}

// TestValleyIncludesAZeroConsumptionRow is task-9-review.md's minor
// finding: a measured zero is a valid Valley candidate, not "no data" (Global
// Constraints: "zero means measured zero"). Row 1's active_import of 0 must
// win Valley over row 0's 5, not be treated as absent/suspect.
func TestValleyIncludesAZeroConsumptionRow(t *testing.T) {
	rows := []consumption.Row{
		{Window: summaryWindow(0), Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("5")}},
		{Window: summaryWindow(1), Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("0")}},
	}
	s := consumption.Summarise(rows)
	require.NotNil(t, s.Valley)
	require.True(t, s.Valley.Values[energy.ActiveImport].IsZero(), "the measured-zero row must win Valley, not be skipped as if it were nil")
	require.True(t, s.Valley.Window.From.Equal(summaryWindow(1).From))
}

func TestPeakAndValleyTieBreaksOnEarliestWindow(t *testing.T) {
	rows := []consumption.Row{
		{Window: summaryWindow(2), Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("10")}},
		{Window: summaryWindow(0), Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("10")}},
		{Window: summaryWindow(1), Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("10")}},
	}
	s := consumption.Summarise(rows)
	require.True(t, s.Peak.Window.From.Equal(summaryWindow(0).From), "all three rows tie at 10; the earliest window wins")
	require.True(t, s.Valley.Window.From.Equal(summaryWindow(0).From), "same tie rule applies to Valley")
}

// TestPeakAndValleyAreCopiesNotPointersIntoTheSlice proves Summarise's Peak
// and Valley never alias rows: mutating the map behind *s.Peak must not
// change rows itself. This is guard (a) from the task brief ("Peak returns a
// pointer into the input slice" is the mutation this test must catch).
func TestPeakAndValleyAreCopiesNotPointersIntoTheSlice(t *testing.T) {
	rows := []consumption.Row{
		{
			AnalyzerID: uuid.New(),
			Window:     summaryWindow(0),
			Values:     map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("10")},
		},
	}
	s := consumption.Summarise(rows)
	require.NotNil(t, s.Peak)

	// Mutate the map reachable from the returned pointer.
	s.Peak.Values[energy.ActiveImport] = dec("999")
	require.Equal(t, "10", rows[0].Values[energy.ActiveImport].String(), "mutating *s.Peak must not change the input slice")

	s.Valley.Values[energy.ActiveImport] = dec("777")
	require.Equal(t, "10", rows[0].Values[energy.ActiveImport].String(), "mutating *s.Valley must not change the input slice")
}

func TestSummariseMaxDemandKwIsMaximumAcrossRows(t *testing.T) {
	rows := []consumption.Row{
		{Window: summaryWindow(0), Values: map[energy.Register]*decimal.Decimal{}, MaxDemandKw: dec("12.5")},
		{Window: summaryWindow(1), Values: map[energy.Register]*decimal.Decimal{}, MaxDemandKw: dec("40.1")},
		{Window: summaryWindow(2), Values: map[energy.Register]*decimal.Decimal{}, MaxDemandKw: nil},
	}
	s := consumption.Summarise(rows)
	require.Equal(t, "40.1", s.MaxDemandKw.String())
}

func TestSummariseOfNoRowsIsAllNilAndZeroCounts(t *testing.T) {
	s := consumption.Summarise(nil)
	require.Nil(t, s.Peak)
	require.Nil(t, s.Valley)
	require.Nil(t, s.MaxDemandKw)
	require.Equal(t, 0, s.Rows)
	require.Equal(t, 0, s.Suspect)
	require.Nil(t, s.Totals[energy.ActiveImport])
}
