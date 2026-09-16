package consumption

import (
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// exportRegisters is exactly the six export-side registers (05 §5
// /generation, R78) — GenerationRows narrows Values and Indexes to these.
var exportRegisters = []energy.Register{
	energy.ActiveExport,
	energy.ReactiveInductiveExport,
	energy.ReactiveCapacitiveExport,
	energy.T1Export,
	energy.T2Export,
	energy.T3Export,
}

// GenerationRows narrows a series to the export registers only (05 §5
// /generation, R78) — same Row shape, same Window per element, Values and
// Indexes limited to ActiveExport/ReactiveInductiveExport/
// ReactiveCapacitiveExport/T1Export/T2Export/T3Export. Ratios are nil: they
// are import-side (reactive-over-active-import) and have no meaning on a
// generation row. Everything else (AnalyzerID, Window, MaxDemandKw, Source,
// Suspect, Partial, Resolution) is copied from the input row. No new fetch:
// the caller already has the rows from Task 7's Analytics.Consumption or
// Billing.Consumption. The returned Values and Indexes maps are new maps —
// never the input row's own maps — so a caller mutating a returned row can
// never corrupt rows.
func GenerationRows(rows []Row) []Row {
	out := make([]Row, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = Row{
			AnalyzerID:      r.AnalyzerID,
			Window:          r.Window,
			Values:          filterRegisters(r.Values, exportRegisters),
			Indexes:         filterRegisters(r.Indexes, exportRegisters),
			InductiveRatio:  nil,
			CapacitiveRatio: nil,
			MaxDemandKw:     r.MaxDemandKw,
			Source:          r.Source,
			Suspect:         cloneSuspectMap(r.Suspect),
			Partial:         r.Partial,
			Resolution:      cloneResolutionMap(r.Resolution),
		}
	}
	return out
}

// filterRegisters builds a fresh map holding only keep's keys, each mapped
// to m's own value for that key (nil when m is nil or does not have it) —
// never the input map m itself.
func filterRegisters(m map[energy.Register]*decimal.Decimal, keep []energy.Register) map[energy.Register]*decimal.Decimal {
	out := make(map[energy.Register]*decimal.Decimal, len(keep))
	for _, reg := range keep {
		out[reg] = m[reg]
	}
	return out
}
