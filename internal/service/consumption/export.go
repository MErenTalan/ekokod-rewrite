package consumption

import (
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// Export renders a series as ordered columns and string cells for
// /consumption/export. F3 renders no file: the CSV/XLSX encoder is F6/F8
// (R77).
type Export struct {
	Columns []string
	Rows    [][]string // decimals as unrounded strings; an unavailable value is ""
}

// ExportRows renders rows into Export's fixed column order:
// analyzer_id, period_start, period_end, source, partial, then every
// register in energy.AllRegisters() order as its consumption column, then
// every register's index column (<register>_index, same order), then
// inductive_ratio, capacitive_ratio, max_demand_kw, suspect_registers.
// Timestamps are RFC3339 in UTC. Every decimal cell is its unrounded
// String() — no presentation rounding here, that is F6's. A nil value is ""
// — never "0". suspect_registers is a ";"-joined list of the row's suspect
// registers in AllRegisters() order.
func ExportRows(rows []Row) Export {
	e := Export{
		Columns: exportColumns(),
		Rows:    make([][]string, len(rows)),
	}
	for i := range rows {
		e.Rows[i] = exportRow(&rows[i])
	}
	return e
}

func exportColumns() []string {
	registers := energy.AllRegisters()
	cols := make([]string, 0, 5+2*len(registers)+4)
	cols = append(cols, "analyzer_id", "period_start", "period_end", "source", "partial")
	for _, reg := range registers {
		cols = append(cols, string(reg))
	}
	for _, reg := range registers {
		cols = append(cols, string(reg)+"_index")
	}
	cols = append(cols, "inductive_ratio", "capacitive_ratio", "max_demand_kw", "suspect_registers")
	return cols
}

func exportRow(r *Row) []string {
	registers := energy.AllRegisters()
	row := make([]string, 0, 5+2*len(registers)+4)
	row = append(row,
		r.AnalyzerID.String(),
		r.Window.From.UTC().Format(time.RFC3339),
		r.Window.To.UTC().Format(time.RFC3339),
		string(r.Source),
		strconv.FormatBool(r.Partial),
	)
	for _, reg := range registers {
		row = append(row, decimalCell(r.Values[reg]))
	}
	for _, reg := range registers {
		row = append(row, decimalCell(r.Indexes[reg]))
	}
	row = append(row,
		decimalCell(r.InductiveRatio),
		decimalCell(r.CapacitiveRatio),
		decimalCell(r.MaxDemandKw),
		suspectRegisterList(r.Suspect),
	)
	return row
}

// decimalCell renders d as its unrounded String(), or "" when d is nil — an
// unavailable value is never rendered as "0".
func decimalCell(d *decimal.Decimal) string {
	if d == nil {
		return ""
	}
	return d.String()
}

// suspectRegisterList joins suspect's registers, in energy.AllRegisters()
// order, with ";".
func suspectRegisterList(suspect map[energy.Register]energy.Suspicion) string {
	if len(suspect) == 0 {
		return ""
	}
	names := make([]string, 0, len(suspect))
	for _, reg := range energy.AllRegisters() {
		if _, ok := suspect[reg]; ok {
			names = append(names, string(reg))
		}
	}
	return strings.Join(names, ";")
}
