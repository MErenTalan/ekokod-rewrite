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

// ReportPayload is R312's snapshot, stored in carbon_reports.payload and
// rendered on demand: later catalogue or activity changes never alter it.
type ReportPayload struct {
	ReportType  string          `json:"report_type"` // ghg | iso
	Company     string          `json:"company"`
	Building    string          `json:"building"`
	Address     *string         `json:"address,omitempty"`
	From        string          `json:"from"` // YYYY-MM-DD
	To          string          `json:"to"`
	GeneratedAt time.Time       `json:"generated_at"`
	Groups      []PayloadGroup  `json:"groups"`
	TotalKgCO2e decimal.Decimal `json:"total_kgco2e"`
	Pending     int             `json:"pending_count"`
	Figures94   *Payload94      `json:"figures_94"`
}

// PayloadGroup is one report group with its lines.
type PayloadGroup struct {
	Key         string          `json:"key"`
	TotalKgCO2e decimal.Decimal `json:"total_kgco2e"`
	Lines       []PayloadLine   `json:"lines"`
}

// PayloadLine is one sub-category's emission in a group.
type PayloadLine struct {
	Sub    string          `json:"sub_category"`
	KgCO2e decimal.Decimal `json:"kgco2e"`
}

// Payload94 is §9.4 with the grid factor it used (R313).
type Payload94 struct {
	ConsumptionKwh    decimal.Decimal `json:"consumption_kwh"`
	GenerationKwh     decimal.Decimal `json:"generation_kwh"`
	GridFactor        decimal.Decimal `json:"grid_factor"`
	GridFactorUnit    string          `json:"grid_factor_unit"`
	GridFactorSource  *string         `json:"grid_factor_source,omitempty"`
	GridFactorYear    *int16          `json:"grid_factor_year,omitempty"`
	ConsumptionT      decimal.Decimal `json:"consumption_emission_t"`
	ProductionReductT decimal.Decimal `json:"production_reduction_t"`
	NetT              decimal.Decimal `json:"net_emission_t"`
}
