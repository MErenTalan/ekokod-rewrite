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

// exportGoldenHeader pins ExportRows' full, fixed column order (task-9
// Clarifications): analyzer_id, period_start, period_end, source, partial,
// then every register in energy.AllRegisters() order as its consumption
// column, then every register's index column (<register>_index, same
// order), then inductive_ratio, capacitive_ratio, max_demand_kw,
// suspect_registers. 5 + 12 + 12 + 4 = 33 columns.
var exportGoldenHeader = []string{
	"analyzer_id", "period_start", "period_end", "source", "partial",
	"active_import", "reactive_inductive_import", "reactive_capacitive_import",
	"t1_import", "t2_import", "t3_import",
	"active_export", "reactive_inductive_export", "reactive_capacitive_export",
	"t1_export", "t2_export", "t3_export",
	"active_import_index", "reactive_inductive_import_index", "reactive_capacitive_import_index",
	"t1_import_index", "t2_import_index", "t3_import_index",
	"active_export_index", "reactive_inductive_export_index", "reactive_capacitive_export_index",
	"t1_export_index", "t2_export_index", "t3_export_index",
	"inductive_ratio", "capacitive_ratio", "max_demand_kw", "suspect_registers",
}

// activeImportColumn is exportGoldenHeader's index of the active_import
// consumption cell — the fixtures below plant their probe value there.
const activeImportColumn = 5

// TestExportColumnsAreStableAndUnrounded is Task 9's golden-header test
// (Step 1/Step 5). The fixture's active_import value carries a 4th
// decimal digit (12.2525) precisely so a "round to 3 decimals" mutation is
// observable — "12.253"/"12.252" would not equal "12.2525".
func TestExportColumnsAreStableAndUnrounded(t *testing.T) {
	analyzerID := uuid.New()
	w0 := energy.Window{From: summaryT0, To: summaryT0.Add(time.Hour)}
	w1 := energy.Window{From: summaryT0.Add(time.Hour), To: summaryT0.Add(2 * time.Hour)}
	rows := []consumption.Row{
		{
			AnalyzerID: analyzerID,
			Window:     w0,
			Values:     map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("12.2525")},
			Source:     energy.KindLoadProfile,
		},
		{
			AnalyzerID: analyzerID,
			Window:     w1,
			Values:     map[energy.Register]*decimal.Decimal{}, // active_import unreported
			Source:     energy.KindLoadProfile,
		},
	}

	e := consumption.ExportRows(rows)
	require.Equal(t, exportGoldenHeader, e.Columns)
	require.Len(t, e.Rows, 2)
	require.Equal(t, "12.2525", e.Rows[0][activeImportColumn], "unrounded String(), never rounded")
	require.Equal(t, "", e.Rows[1][activeImportColumn], "an unavailable value is empty, never 0")
}
