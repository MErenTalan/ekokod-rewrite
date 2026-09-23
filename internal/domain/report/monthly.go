package report

import (
	"github.com/shopspring/decimal"
)

// BuildMonthly is the monthly report of 02 §10.1 under R254–R259 and R270.
func BuildMonthly(in MonthlyInput) Monthly {
	y, mo := in.Year, in.Month
	days := DaysIn(y, mo)
	m := Monthly{Year: y, Month: mo, DaysInMonth: days, Selection: in.Selection}

	buildingFig := func(year int, pick func(BuildingInput) *decimal.Decimal) Figure {
		vals := make([]*decimal.Decimal, len(in.Buildings))
		partial := make([]bool, len(in.Buildings))
		for i, b := range in.Buildings {
			vals[i] = pick(b)
			partial[i] = partialAt(b.Partial, year, mo)
		}
		return sumFigure(vals, partial)
	}
	series := func(year int, field func(BuildingInput) map[int]MonthSeries) Figure {
		return buildingFig(year, func(b BuildingInput) *decimal.Decimal { return at(field(b), year, mo) })
	}
	cons := func(b BuildingInput) map[int]MonthSeries { return b.Consumption }
	export := func(b BuildingInput) map[int]MonthSeries { return b.Export }

	m.Consumption = series(y, cons)
	m.ConsumptionPrev = series(y-1, cons)
	m.DailyConsumption = perDay(m.Consumption, days)
	m.T1 = buildingFig(y, func(b BuildingInput) *decimal.Decimal { return b.T1 })
	m.T2 = buildingFig(y, func(b BuildingInput) *decimal.Decimal { return b.T2 })
	m.T3 = buildingFig(y, func(b BuildingInput) *decimal.Decimal { return b.T3 })
	m.Inductive = buildingFig(y, func(b BuildingInput) *decimal.Decimal { return b.Inductive })
	m.Capacitive = buildingFig(y, func(b BuildingInput) *decimal.Decimal { return b.Capacitive })
	m.InductiveRatio, m.CapacitiveRatio = reactiveRatios(in.Buildings, y, mo)

	// R256: the plant selection decides which production sources count.
	m.Rooftop, m.RooftopPrev = excluded(), excluded()
	if in.Selection != SelectionGrid {
		m.Rooftop, m.RooftopPrev = series(y, export), series(y-1, export)
	}
	m.Utility, m.UtilityPrev = excluded(), excluded()
	if in.Selection != SelectionRooftop {
		m.Utility, m.UtilityPrev = utility(in.Plants, y, mo), utility(in.Plants, y-1, mo)
	}
	m.Production = combine(m.Rooftop, m.Utility)
	m.ProductionPrev = combine(m.RooftopPrev, m.UtilityPrev)
	m.DailyProduction = perDay(m.Production, days)
	m.ConsumptionDelta = DeltaOf(m.Consumption.Value, m.ConsumptionPrev.Value)
	m.ProductionDelta = DeltaOf(m.Production.Value, m.ProductionPrev.Value)

	cur := make([]*Bill, len(in.Buildings))
	prev := make([]*Bill, len(in.Buildings))
	var genPrices []*decimal.Decimal
	for i, b := range in.Buildings {
		cur[i] = billAt(b.Bills, y, mo)
		prev[i] = billAt(b.Bills, y-1, mo)
		line := BuildingLine{BuildingID: b.ID, Name: b.Name, TariffName: b.TariffName}
		if cur[i] != nil {
			line.PurchasePrice = cur[i].EffectivePrice
			c := cur[i].Currency
			line.Currency = &c
			genPrices = append(genPrices, cur[i].GenerationPrice)
		}
		m.Buildings = append(m.Buildings, line)
	}
	total := func(b *Bill) decimal.Decimal { return b.Total }
	reactive := func(b *Bill) decimal.Decimal { return b.ReactivePenalty }
	m.Bill = moneyByCurrency(cur, total)
	m.BillPrev = moneyByCurrency(prev, total)
	m.EnergyCost = moneyByCurrency(cur, func(b *Bill) decimal.Decimal { return b.EnergyCost })
	m.DistributionCost = moneyByCurrency(cur, func(b *Bill) decimal.Decimal { return b.DistributionCost })
	m.Taxes = moneyByCurrency(cur, func(b *Bill) decimal.Decimal { return b.Taxes })
	m.ReactivePenalty = moneyByCurrency(cur, reactive)
	m.ReactivePenaltyPrev = moneyByCurrency(prev, reactive)
	m.BillDelta = moneyDeltas(m.Bill, m.BillPrev)
	m.ReactivePenaltyDelta = moneyDeltas(m.ReactivePenalty, m.ReactivePenaltyPrev)
	m.AveragePurchasePrice = averagePrice(cur, m.EnergyCost)

	if in.Selection != SelectionGrid {
		m.RooftopFeedIn = priceRange(genPrices)
	}
	var feedIns []*decimal.Decimal
	for _, p := range in.Plants {
		m.Plants = append(m.Plants, PlantLine{PlantID: p.ID, Name: p.Name, Kind: p.Kind, FeedIn: p.FeedIn,
			Production: sumFigure([]*decimal.Decimal{at(p.Production, y, mo)}, nil)})
		if p.Kind == KindGrid {
			feedIns = append(feedIns, p.FeedIn)
		}
	}
	if in.Selection != SelectionRooftop {
		m.UtilityFeedIn = priceRange(feedIns)
	}

	// R271: January…December of the report year against the previous year.
	allBills := func(year, month int) []*Bill {
		out := make([]*Bill, len(in.Buildings))
		for i, b := range in.Buildings {
			out[i] = billAt(b.Bills, year, month)
		}
		return out
	}
	var yearBills [][]MoneyFigure
	for month := 1; month <= 12; month++ {
		yearBills = append(yearBills, moneyByCurrency(allBills(y, month), total), moneyByCurrency(allBills(y-1, month), total))
	}
	m.ChartCurrency, m.OmittedCurrencies = chartCurrency(yearBills...)
	for month := 1; month <= 12; month++ {
		c := func(year int) *decimal.Decimal {
			return buildingFigAt(in.Buildings, year, month).Value
		}
		m.ConsumptionChart = append(m.ConsumptionChart, MonthPoint{Month: month, Current: c(y), Previous: c(y - 1)})
		point := MonthPoint{Month: month}
		if m.ChartCurrency != nil {
			point.Current = moneyValue(yearBills[(month-1)*2], *m.ChartCurrency)
			point.Previous = moneyValue(yearBills[(month-1)*2+1], *m.ChartCurrency)
		}
		m.BillChart = append(m.BillChart, point)
	}
	m.Partial = m.Consumption.Partial || m.Production.Partial
	return m
}

// buildingFigAt is Σ consumption over buildings for one month.
func buildingFigAt(bs []BuildingInput, year, month int) Figure {
	vals := make([]*decimal.Decimal, len(bs))
	for i, b := range bs {
		vals[i] = at(b.Consumption, year, month)
	}
	return sumFigure(vals, nil)
}

// utility is Σ production of the grid-kind plants (R256).
func utility(plants []PlantInput, year, month int) Figure {
	var vals []*decimal.Decimal
	for _, p := range plants {
		if p.Kind == KindGrid {
			vals = append(vals, at(p.Production, year, month))
		}
	}
	return sumFigure(vals, nil)
}

// reactiveRatios weights by active consumption (02 §10.1): Σ reactive ÷
// Σ active over buildings that have both.
func reactiveRatios(bs []BuildingInput, year, month int) (*decimal.Decimal, *decimal.Decimal) {
	var active, ind, capa decimal.Decimal
	var anyInd, anyCap bool
	for _, b := range bs {
		a := at(b.Consumption, year, month)
		if a == nil {
			continue
		}
		if b.Inductive != nil {
			active = active.Add(*a)
			ind = ind.Add(*b.Inductive)
			anyInd = true
			if b.Capacitive != nil {
				capa = capa.Add(*b.Capacitive)
				anyCap = true
			}
		}
	}
	var ri, rc *decimal.Decimal
	if anyInd {
		ri = ratio(ind, active)
	}
	if anyCap {
		rc = ratio(capa, active)
	}
	return ri, rc
}

// averagePrice is R257: Σ energy_cost ÷ Σ net_consumption per currency.
func averagePrice(bills []*Bill, energy []MoneyFigure) []MoneyFigure {
	net := moneyByCurrency(bills, func(b *Bill) decimal.Decimal { return b.NetConsumption })
	var out []MoneyFigure
	for _, e := range energy {
		n := moneyValue(net, e.Currency)
		if n == nil || n.IsZero() {
			continue
		}
		out = append(out, MoneyFigure{Currency: e.Currency, Value: e.Value.DivRound(*n, 6), WithData: e.WithData, Of: e.Of})
	}
	return out
}
