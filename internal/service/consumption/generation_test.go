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

func generationRow() consumption.Row {
	w := energy.Window{From: summaryT0, To: summaryT0.Add(time.Hour)}
	return consumption.Row{
		AnalyzerID: uuid.New(),
		Window:     w,
		Values: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport:             dec("10"),
			energy.ReactiveInductiveImport:  dec("2"),
			energy.ActiveExport:             dec("3"),
			energy.ReactiveInductiveExport:  dec("0.5"),
			energy.ReactiveCapacitiveExport: nil,
		},
		Indexes: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport: dec("1000"),
			energy.ActiveExport: dec("50"),
		},
		InductiveRatio:  dec("0.2"),
		CapacitiveRatio: dec("0.1"),
		MaxDemandKw:     dec("15"),
		Source:          energy.KindLoadProfile,
		Suspect:         map[energy.Register]energy.Suspicion{energy.ActiveImport: {Reason: energy.ReasonNegativeDelta}},
		Partial:         true,
		Resolution:      map[energy.Register]string{energy.ActiveImport: "manual override note"},
	}
}

func TestGenerationRowsReadsTheExportRegistersOnly(t *testing.T) {
	in := generationRow()
	out := consumption.GenerationRows([]consumption.Row{in})
	require.Len(t, out, 1)

	got := out[0]
	require.Len(t, got.Values, 6, "exactly the six export registers, never the import ones")
	require.Equal(t, "3", got.Values[energy.ActiveExport].String())
	require.Equal(t, "0.5", got.Values[energy.ReactiveInductiveExport].String())
	require.Nil(t, got.Values[energy.ReactiveCapacitiveExport])
	require.Nil(t, got.Values[energy.T1Export])
	require.Nil(t, got.Values[energy.T2Export])
	require.Nil(t, got.Values[energy.T3Export])
	_, hasImport := got.Values[energy.ActiveImport]
	require.False(t, hasImport, "an import register must not even be a key")

	require.Len(t, got.Indexes, 6)
	require.Equal(t, "50", got.Indexes[energy.ActiveExport].String())
	_, hasImportIndex := got.Indexes[energy.ActiveImport]
	require.False(t, hasImportIndex)
}

func TestGenerationRowsRatiosAreNilAndEverythingElseIsCopied(t *testing.T) {
	in := generationRow()
	out := consumption.GenerationRows([]consumption.Row{in})
	got := out[0]

	require.Nil(t, got.InductiveRatio, "ratios are import-side; they have no meaning on a generation row")
	require.Nil(t, got.CapacitiveRatio)

	require.Equal(t, in.AnalyzerID, got.AnalyzerID)
	require.Equal(t, in.Window, got.Window)
	require.Equal(t, in.MaxDemandKw.String(), got.MaxDemandKw.String())
	require.Equal(t, in.Source, got.Source)
	require.Equal(t, in.Partial, got.Partial)
	require.Contains(t, got.Suspect, energy.ActiveImport)
	require.Contains(t, got.Resolution, energy.ActiveImport)
}

// TestGenerationRowsNeverRendersASuspectExportRegisterAsANumber is
// task-9-review.md finding (b)/(c)'s counterpart for GenerationRows: a
// hand-built Row whose Values entry is non-nil for a suspect export
// register must come back nil, never the raw 999. Removing
// GenerationRows' soundValue check (reverting filterSoundValues to a bare
// filterRegisters call) must turn this red.
func TestGenerationRowsNeverRendersASuspectExportRegisterAsANumber(t *testing.T) {
	in := generationRow()
	in.Values[energy.ActiveExport] = dec("999")
	in.Suspect = map[energy.Register]energy.Suspicion{energy.ActiveExport: {Reason: energy.ReasonNegativeDelta}}

	out := consumption.GenerationRows([]consumption.Row{in})
	require.Nil(t, out[0].Values[energy.ActiveExport], "active_export is suspect: must never render as 999")
}

// TestGenerationRowsDoesNotAliasInputValuesMap is guard (c) from the task
// brief: GenerationRows must never share the input row's Values (or Indexes)
// map. Mutating the returned map must not change the input row's own map.
func TestGenerationRowsDoesNotAliasInputValuesMap(t *testing.T) {
	in := generationRow()
	rows := []consumption.Row{in}
	out := consumption.GenerationRows(rows)

	out[0].Values[energy.ActiveExport] = dec("999999")
	require.Equal(t, "3", rows[0].Values[energy.ActiveExport].String(), "mutating the output's Values map must not change the input row's map")

	out[0].Indexes[energy.ActiveExport] = dec("999999")
	require.Equal(t, "50", rows[0].Indexes[energy.ActiveExport].String(), "mutating the output's Indexes map must not change the input row's map")
}
