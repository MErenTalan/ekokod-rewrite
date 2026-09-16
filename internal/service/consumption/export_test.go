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

func TestExportRowsRendersTimestampsAsRFC3339UTC(t *testing.T) {
	ist, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	from := time.Date(2026, 3, 1, 12, 0, 0, 0, ist)
	to := from.Add(time.Hour)

	rows := []consumption.Row{{
		AnalyzerID: uuid.New(),
		Window:     energy.Window{From: from, To: to},
		Values:     map[energy.Register]*decimal.Decimal{},
		Source:     energy.KindBilling,
	}}
	e := consumption.ExportRows(rows)
	require.Equal(t, from.UTC().Format(time.RFC3339), e.Rows[0][1])
	require.Equal(t, to.UTC().Format(time.RFC3339), e.Rows[0][2])
	require.Contains(t, e.Rows[0][1], "Z", "rendered in UTC, not +03:00")
}

func TestExportRowsRendersSourceAndPartial(t *testing.T) {
	w := energy.Window{From: summaryT0, To: summaryT0.Add(time.Hour)}
	rows := []consumption.Row{{
		AnalyzerID: uuid.New(),
		Window:     w,
		Values:     map[energy.Register]*decimal.Decimal{},
		Source:     energy.KindLoadProfile,
		Partial:    true,
	}}
	e := consumption.ExportRows(rows)
	require.Equal(t, "load_profile", e.Rows[0][3])
	require.Equal(t, "true", e.Rows[0][4])
}

func TestExportRowsRendersIndexColumnsAndRatiosAndMaxDemand(t *testing.T) {
	w := energy.Window{From: summaryT0, To: summaryT0.Add(time.Hour)}
	rows := []consumption.Row{{
		AnalyzerID:      uuid.New(),
		Window:          w,
		Values:          map[energy.Register]*decimal.Decimal{},
		Indexes:         map[energy.Register]*decimal.Decimal{energy.ActiveImport: dec("1234.5")},
		InductiveRatio:  dec("0.11"),
		CapacitiveRatio: dec("0.22"),
		MaxDemandKw:     dec("77.7"),
		Source:          energy.KindLoadProfile,
	}}
	e := consumption.ExportRows(rows)
	require.Equal(t, exportGoldenHeader, e.Columns)

	activeImportIndexColumn := indexOf(exportGoldenHeader, "active_import_index")
	require.Equal(t, "1234.5", e.Rows[0][activeImportIndexColumn])

	inductiveRatioColumn := indexOf(exportGoldenHeader, "inductive_ratio")
	capacitiveRatioColumn := indexOf(exportGoldenHeader, "capacitive_ratio")
	maxDemandColumn := indexOf(exportGoldenHeader, "max_demand_kw")
	require.Equal(t, "0.11", e.Rows[0][inductiveRatioColumn])
	require.Equal(t, "0.22", e.Rows[0][capacitiveRatioColumn])
	require.Equal(t, "77.7", e.Rows[0][maxDemandColumn])
}

func TestExportRowsSuspectRegistersJoinedInAllRegistersOrder(t *testing.T) {
	w := energy.Window{From: summaryT0, To: summaryT0.Add(time.Hour)}
	rows := []consumption.Row{{
		AnalyzerID: uuid.New(),
		Window:     w,
		Values:     map[energy.Register]*decimal.Decimal{},
		Source:     energy.KindLoadProfile,
		// Declared out of AllRegisters() order on purpose: the join must
		// still come out in canonical order, not map iteration order.
		Suspect: map[energy.Register]energy.Suspicion{
			energy.T1Import:     {Reason: energy.ReasonMissingReadings},
			energy.ActiveImport: {Reason: energy.ReasonNegativeDelta},
		},
	}}
	e := consumption.ExportRows(rows)
	suspectColumn := indexOf(exportGoldenHeader, "suspect_registers")
	require.Equal(t, "active_import;t1_import", e.Rows[0][suspectColumn])
}

func TestExportRowsEmptySuspectIsEmptyString(t *testing.T) {
	w := energy.Window{From: summaryT0, To: summaryT0.Add(time.Hour)}
	rows := []consumption.Row{{
		AnalyzerID: uuid.New(),
		Window:     w,
		Values:     map[energy.Register]*decimal.Decimal{},
		Source:     energy.KindLoadProfile,
	}}
	e := consumption.ExportRows(rows)
	suspectColumn := indexOf(exportGoldenHeader, "suspect_registers")
	require.Equal(t, "", e.Rows[0][suspectColumn])
}

func TestExportRowsOfNoRowsIsEmpty(t *testing.T) {
	e := consumption.ExportRows(nil)
	require.Equal(t, exportGoldenHeader, e.Columns)
	require.Empty(t, e.Rows)
}

// indexOf returns the position of name in header, failing loudly (a huge
// negative index that panics on use) rather than silently reading column 0
// if the header ever loses that column.
func indexOf(header []string, name string) int {
	for i, h := range header {
		if h == name {
			return i
		}
	}
	panic("indexOf: column not found: " + name)
}
