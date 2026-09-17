package analysis

import (
	"sort"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
)

// SumByWindow folds per-analyzer rows into one building row per window (R160):
// a register sums only when every one of n analyzers reported a sound value,
// otherwise it is nil and the row is partial; ratios are recomputed from the
// sums; max demand is the maximum; indexes are dropped.
func SumByWindow(rows []consumption.Row, n int) []consumption.Row {
	byStart := map[int64][]consumption.Row{}
	for _, r := range rows {
		byStart[r.Window.From.UnixNano()] = append(byStart[r.Window.From.UnixNano()], r)
	}
	starts := make([]int64, 0, len(byStart))
	for k := range byStart {
		starts = append(starts, k)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i] < starts[j] })

	out := make([]consumption.Row, 0, len(starts))
	for _, start := range starts {
		group := byStart[start]
		row := consumption.Row{Window: group[0].Window, Source: group[0].Source, Values: map[energy.Register]*decimal.Decimal{}}
		for _, reg := range energy.AllRegisters() {
			sum, reported := decimal.Zero, 0
			for _, r := range group {
				v := r.Values[reg]
				if _, suspect := r.Suspect[reg]; suspect || v == nil {
					continue
				}
				sum = sum.Add(*v)
				reported++
			}
			switch {
			case reported == n && n > 0:
				total := sum
				row.Values[reg] = &total
			case reported > 0:
				row.Partial = true
			}
		}
		for _, r := range group {
			row.Partial = row.Partial || r.Partial
			if r.MaxDemandKw != nil && (row.MaxDemandKw == nil || r.MaxDemandKw.GreaterThan(*row.MaxDemandKw)) {
				m := *r.MaxDemandKw
				row.MaxDemandKw = &m
			}
		}
		if len(group) < n {
			row.Partial = true
		}
		row.InductiveRatio = ratio(row.Values[energy.ReactiveInductiveImport], row.Values[energy.ActiveImport])
		row.CapacitiveRatio = ratio(row.Values[energy.ReactiveCapacitiveImport], row.Values[energy.ActiveImport])
		out = append(out, row)
	}
	return out
}

func ratio(reg, active *decimal.Decimal) *decimal.Decimal {
	if reg == nil || active == nil || !active.IsPositive() {
		return nil
	}
	v := reg.DivRound(*active, energy.DivisionScale)
	return &v
}
