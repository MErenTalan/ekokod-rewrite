package report

import (
	"github.com/shopspring/decimal"
)

var thousand = decimal.NewFromInt(1000)

// PlantTarget is E-1 / R260: the plant's yearly target when set, else the
// sum of its twelve monthly targets, else nil. Zero is not a target.
func PlantTarget(p PlantInput) *decimal.Decimal {
	if p.YearlyTarget != nil && p.YearlyTarget.IsPositive() {
		v := *p.YearlyTarget
		return &v
	}
	if len(p.MonthlyTargets) != 12 {
		return nil
	}
	var sum decimal.Decimal
	for _, v := range p.MonthlyTargets {
		sum = sum.Add(v)
	}
	if !sum.IsPositive() {
		return nil
	}
	return &sum
}

// yearTotal sums one year of a series; nil when no month has data.
func yearTotal(m map[int]MonthSeries, partial map[int][12]bool, year int) (*decimal.Decimal, bool) {
	s, ok := m[year]
	if !ok {
		return nil, false
	}
	var total decimal.Decimal
	seen, part := false, false
	for i, v := range s {
		if v == nil {
			continue
		}
		seen = true
		total = total.Add(*v)
		part = part || partialAt(partial, year, i+1)
	}
	if !seen {
		return nil, false
	}
	return &total, part
}

// BuildYearly is the yearly report of 02 §10.2 under R256–R262 and R270.
func BuildYearly(in YearlyInput) Yearly {
	y := in.Year
	days := 365
	if DaysIn(y, 2) == 29 {
		days = 366
	}
	out := Yearly{Year: y, Selection: in.Selection}
	useRooftop := in.Selection != SelectionGrid
	useUtility := in.Selection != SelectionRooftop

	for _, b := range in.Buildings {
		out.Buildings = append(out.Buildings, BuildingLine{BuildingID: b.ID, Name: b.Name, TariffName: b.TariffName})
	}
	for month := 1; month <= 12; month++ {
		row := YearMonthRow{Month: month, Rooftop: excluded()}
		cons := make([]*decimal.Decimal, len(in.Buildings))
		exp := make([]*decimal.Decimal, len(in.Buildings))
		part := make([]bool, len(in.Buildings))
		bills := make([]*Bill, len(in.Buildings))
		for i, b := range in.Buildings {
			cons[i], exp[i] = at(b.Consumption, y, month), at(b.Export, y, month)
			part[i] = partialAt(b.Partial, y, month)
			bills[i] = billAt(b.Bills, y, month)
		}
		row.Consumption = sumFigure(cons, part)
		if useRooftop {
			row.Rooftop = sumFigure(exp, part)
		}
		row.Bill = moneyByCurrency(bills, func(b *Bill) decimal.Decimal { return b.Total })
		row.ReactivePenalty = moneyByCurrency(bills, func(b *Bill) decimal.Decimal { return b.ReactivePenalty })
		out.Months = append(out.Months, row)
	}

	totals := func(year int) (cons, rooftop, utility Figure) {
		cv := make([]*decimal.Decimal, len(in.Buildings))
		ev := make([]*decimal.Decimal, len(in.Buildings))
		cp := make([]bool, len(in.Buildings))
		ep := make([]bool, len(in.Buildings))
		for i, b := range in.Buildings {
			cv[i], cp[i] = yearTotal(b.Consumption, b.Partial, year)
			ev[i], ep[i] = yearTotal(b.Export, b.Partial, year)
		}
		cons, rooftop, utility = sumFigure(cv, cp), excluded(), excluded()
		if useRooftop {
			rooftop = sumFigure(ev, ep)
		}
		if useUtility {
			var pv []*decimal.Decimal
			for _, p := range in.Plants {
				if p.Kind == KindGrid {
					v, _ := yearTotal(p.Production, nil, year)
					pv = append(pv, v)
				}
			}
			utility = sumFigure(pv, nil)
		}
		return cons, rooftop, utility
	}
	billsOf := func(year int, pick func(*Bill) decimal.Decimal) []MoneyFigure {
		return yearMoney(in.Buildings, year, pick)
	}
	total := func(b *Bill) decimal.Decimal { return b.Total }

	out.Consumption, out.Rooftop, out.Utility = totals(y)
	out.Production = combine(out.Rooftop, out.Utility)
	out.Bill = billsOf(y, total)
	out.ReactivePenalty = billsOf(y, func(b *Bill) decimal.Decimal { return b.ReactivePenalty })
	out.DailyConsumption = perDay(out.Consumption, days)
	out.DailyRooftop = perDay(out.Rooftop, days)
	out.DailyProduction = perDay(out.Production, days)
	out.DailyUtility = perDay(out.Utility, days)
	out.Partial = out.Consumption.Partial || out.Production.Partial

	prevCons, _, _ := totals(y - 1)
	out.ConsumptionDelta = DeltaOf(out.Consumption.Value, prevCons.Value)
	out.BillDelta = moneyDeltas(out.Bill, billsOf(y-1, total))

	// R260: the target and the rate are over the utility-scale plants that
	// count, and the rate only over plants that have a target.
	var target, actual, measuredTarget decimal.Decimal
	withTarget, withBoth := false, false
	for _, p := range in.Plants {
		prod, _ := yearTotal(p.Production, nil, y)
		line := PlantLine{PlantID: p.ID, Name: p.Name, Kind: p.Kind, FeedIn: p.FeedIn,
			Production: sumFigure([]*decimal.Decimal{prod}, nil), Target: PlantTarget(p)}
		if line.Target != nil && prod != nil {
			line.AchievementPct = pct(*prod, *line.Target)
		}
		out.Plants = append(out.Plants, line)
		if !useUtility || p.Kind != KindGrid || line.Target == nil {
			continue
		}
		withTarget = true
		target = target.Add(*line.Target)
		if prod != nil {
			withBoth = true
			actual = actual.Add(*prod)
			measuredTarget = measuredTarget.Add(*line.Target)
		}
	}
	if withTarget {
		out.Target = &target
	}
	if withBoth {
		out.AchievementPct = pct(actual, measuredTarget)
	}

	// R261: the share of consumption production could have met.
	if c := out.Consumption.Value; c != nil && c.IsPositive() && out.Production.Value != nil {
		met := decimal.Min(*out.Production.Value, *c)
		solar := met.Div(*c).Mul(hundred).Round(1)
		grid := hundred.Sub(solar)
		out.SolarSharePct, out.GridSharePct = &solar, &grid
	}

	if in.Factor == nil {
		out.CarbonReason = "grid_factor_missing"
	} else {
		out.Carbon = carbon(*in.Factor, out.Consumption.Value, out.Production.Value)
	}

	var historyBills [][]MoneyFigure
	for year := y - 2; year <= y; year++ {
		cons, rooftop, utility := totals(year)
		b := billsOf(year, total)
		historyBills = append(historyBills, b)
		out.History = append(out.History, YearPoint{Year: year, Consumption: cons.Value,
			Production: combine(rooftop, utility).Value, Bill: b})
	}
	out.ChartCurrency, out.OmittedCurrencies = chartCurrency(historyBills...)
	return out
}

// yearMoney sums a year of building bills per currency; WithData counts the
// buildings with at least one bill in that currency.
func yearMoney(bs []BuildingInput, year int, pick func(*Bill) decimal.Decimal) []MoneyFigure {
	var out []MoneyFigure
	for _, b := range bs {
		var all []*Bill
		for month := 1; month <= 12; month++ {
			all = append(all, billAt(b.Bills, year, month))
		}
		for _, m := range moneyByCurrency(all, pick) {
			found := false
			for i := range out {
				if out[i].Currency == m.Currency {
					out[i].Value = out[i].Value.Add(m.Value)
					out[i].WithData++
					found = true
				}
			}
			if !found {
				out = append(out, MoneyFigure{Currency: m.Currency, Value: m.Value, WithData: 1})
			}
		}
	}
	for i := range out {
		out[i].Of = len(bs)
	}
	sortMoney(out)
	return out
}

// pct is part ÷ whole × 100 to one decimal, nil when whole is zero.
func pct(part, whole decimal.Decimal) *decimal.Decimal {
	if whole.IsZero() {
		return nil
	}
	v := part.Div(whole).Mul(hundred).Round(1)
	return &v
}

// carbon is 02 §9.4, not clamped (R262).
func carbon(f GridFactor, cons, prod *decimal.Decimal) *Carbon {
	c := &Carbon{Factor: f.Value, FactorUnit: f.Unit, SourceYear: f.SourceYear}
	tonnes := func(kwh *decimal.Decimal) *decimal.Decimal {
		if kwh == nil {
			return nil
		}
		v := kwh.Mul(f.Value).Div(thousand).Round(3)
		return &v
	}
	c.ConsumptionT, c.ReductionT = tonnes(cons), tonnes(prod)
	if c.ConsumptionT != nil && c.ReductionT != nil {
		net := c.ConsumptionT.Sub(*c.ReductionT)
		c.NetT = &net
	}
	return c
}
