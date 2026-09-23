package carbon

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Group is one report group (a GHG scope or an ISO category) with its
// per-sub-category lines, sorted by key.
type Group struct {
	Key   string
	Total decimal.Decimal
	Lines []SubTotal
}

// Groups is R312's grouping over the window: GHG by scope, ISO by category,
// every group present even when zero.
func Groups(records []Record, kind string, from, to time.Time) (groups []Group, total decimal.Decimal, pending int) {
	var order []string
	if kind == "ghg" {
		for _, s := range model.CarbonScopes() {
			order = append(order, string(s))
		}
	} else {
		order = ISOCategories
	}
	lines := map[string]map[string]decimal.Decimal{}
	for _, k := range order {
		lines[k] = map[string]decimal.Decimal{}
	}
	for _, r := range records {
		if !counts(r) {
			continue
		}
		if civil(r.End).Before(civil(from)) || civil(r.Start).After(civil(to)) {
			continue
		}
		v := Share(Dated{Start: r.Start, End: r.End, Value: r.KgCO2e}, from, to)
		if r.Status == model.CarbonStatusPending {
			pending++
		}
		sub, _ := SubByKey(r.Sub)
		key := sub.ISO
		if kind == "ghg" {
			key = string(sub.Scope)
		}
		lines[key][r.Sub] = lines[key][r.Sub].Add(v)
		total = total.Add(v)
	}
	for _, k := range order {
		g := Group{Key: k}
		for _, s := range sortedKeys(lines[k]) {
			g.Lines = append(g.Lines, SubTotal{Sub: s, KgCO2e: lines[k][s]})
			g.Total = g.Total.Add(lines[k][s])
		}
		groups = append(groups, g)
	}
	return groups, total, pending
}

// Figures94 is 02 §9.4 over the automated grid and generation records.
type Figures94 struct {
	ConsumptionKwh, GenerationKwh, Factor decimal.Decimal
	ConsumptionT, ReductionT, NetT        decimal.Decimal
}

var thousand = decimal.NewFromInt(1000)

// Report94 is nil when no automated record overlaps the window: the report
// then says the meter figures are unavailable rather than showing zeros.
func Report94(records []Record, from, to time.Time, factor decimal.Decimal) *Figures94 {
	var f Figures94
	found := false
	for _, r := range records {
		if !r.Automated || !counts(r) || civil(r.End).Before(civil(from)) || civil(r.Start).After(civil(to)) {
			continue
		}
		q := Share(Dated{Start: r.Start, End: r.End, Value: r.Quantity}, from, to)
		switch r.Sub {
		case SubGrid:
			f.ConsumptionKwh, found = f.ConsumptionKwh.Add(q), true
		case SubGeneration:
			f.GenerationKwh, found = f.GenerationKwh.Add(q), true
		}
	}
	if !found {
		return nil
	}
	f.Factor = factor
	f.ConsumptionT = f.ConsumptionKwh.Mul(factor).Div(thousand)
	f.ReductionT = f.GenerationKwh.Mul(factor).Div(thousand)
	f.NetT = f.ConsumptionT.Sub(f.ReductionT)
	return &f
}
