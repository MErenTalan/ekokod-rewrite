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
// never corrupt rows. A suspect export register's Values entry comes back
// nil (soundValue, sound.go) even if the input row's own Values[reg] is
// itself non-nil — the same guard Balance and ExportRows apply
// (task-9-review.md). A suspect export register's Indexes entry comes back
// nil too (soundIndex, sound.go), exactly as ExportRows blanks a suspect
// register's closing index (final review A M-4: the two helpers must not
// diverge on this rule).
func GenerationRows(rows []Row) []Row {
	out := make([]Row, len(rows))
	for i := range rows {
		r := rows[i]
		out[i] = Row{
			AnalyzerID:      r.AnalyzerID,
			Window:          r.Window,
			Values:          filterSoundValues(r, exportRegisters),
			Indexes:         filterSoundIndexes(r, exportRegisters),
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

// filterSoundValues builds a fresh map holding only keep's keys (never the
// input row's own map), each kept register going through soundValue
// (sound.go) rather than a bare map read, so a register in r.Suspect comes
// back nil even when r.Values itself still holds a number for it.
func filterSoundValues(r Row, keep []energy.Register) map[energy.Register]*decimal.Decimal {
	out := make(map[energy.Register]*decimal.Decimal, len(keep))
	for _, reg := range keep {
		out[reg] = soundValue(r, reg)
	}
	return out
}

// filterSoundIndexes is filterSoundValues' sibling for r.Indexes (final
// review A M-4): each kept register goes through soundIndex (sound.go)
// rather than a bare map read, so a suspect register's closing index is
// blanked here exactly as ExportRows already blanks it (sound.go's
// soundIndex is the ONE place both callers derive that rule from, so they
// cannot drift apart again).
func filterSoundIndexes(r Row, keep []energy.Register) map[energy.Register]*decimal.Decimal {
	out := make(map[energy.Register]*decimal.Decimal, len(keep))
	for _, reg := range keep {
		out[reg] = soundIndex(r, reg)
	}
	return out
}
