package consumption

import (
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// BalanceRow is 05 §5 /energy-balance, derived from registers only.
// Inverter-based self-consumption is F9 and is not approximated here (R78).
// GridImport is identically equal to Consumption and GridExport is
// identically equal to Generation until F9 — four names over two register
// reads, by design, not an oversight.
type BalanceRow struct {
	Window      energy.Window
	Consumption *decimal.Decimal // active_import
	Generation  *decimal.Decimal // active_export
	GridImport  *decimal.Decimal // == Consumption
	GridExport  *decimal.Decimal // == Generation
}

// Balance derives one BalanceRow per input Row — active_import and
// active_export are already both present in Row.Values, so this needs no
// second fetch either. A nil register value (unreported or suspect) stays
// nil, never a substituted zero.
func Balance(rows []Row) []BalanceRow {
	out := make([]BalanceRow, len(rows))
	for i := range rows {
		r := &rows[i]
		consumption := r.Values[energy.ActiveImport]
		generation := r.Values[energy.ActiveExport]
		out[i] = BalanceRow{
			Window:      r.Window,
			Consumption: consumption,
			Generation:  generation,
			GridImport:  consumption,
			GridExport:  generation,
		}
	}
	return out
}
