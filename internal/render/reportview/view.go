// Package reportview turns a stored report payload into the sections both
// renderers print, so the PDF and the workbook cannot disagree (R271).
package reportview

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/render"
)

// Series is one chart series; a nil value is a gap, never a zero bar.
type Series struct {
	Name   string
	Values []*decimal.Decimal
}

// Chart is a grouped bar chart and its unit.
type Chart struct {
	Unit       string
	Categories []string
	Series     []Series
}

// Section is one titled block: a table (Header + Rows), an optional chart
// whose data table the rows are, and notes printed under it.
type Section struct {
	Key, Title string
	Header     []string
	Rows       [][]string
	Chart      *Chart
	Notes      []string
}

// Document is a whole report.
type Document struct {
	Locale   string
	Title    string
	Lines    []string
	Notes    []string
	Sections []Section
}

type formatter struct {
	l   labels
	loc string
}

// Build lays out p in the given locale ("tr" unless "en").
func Build(p report.Payload, company string, buildings []string, locale string) Document {
	if locale != "en" {
		locale = "tr"
	}
	f := formatter{l: catalogue[locale], loc: locale}
	doc := Document{Locale: locale}
	switch {
	case p.Monthly != nil:
		f.monthly(&doc, *p.Monthly, company, buildings)
	case p.Yearly != nil:
		f.yearly(&doc, *p.Yearly, company, buildings)
	}
	return doc
}

func (f formatter) num(v decimal.Decimal, places int32) string {
	return render.FormatDecimal(v, places, f.loc)
}

func (f formatter) figure(fig report.Figure, unit, subject string) string {
	switch {
	case fig.Excluded:
		return f.l.notSelected
	case fig.Value == nil:
		return f.l.noData
	}
	s := f.num(*fig.Value, 2) + " " + unit
	if fig.WithData < fig.Of {
		s += " (" + fmt.Sprintf(f.l.coverage, fig.WithData, fig.Of, subject) + ")"
	}
	return s
}

func (f formatter) money(list []report.MoneyFigure) string {
	if len(list) == 0 {
		return f.l.noData
	}
	parts := make([]string, 0, len(list))
	for _, m := range list {
		s := render.FormatMoney(m.Value, f.loc) + " " + m.Currency
		if m.WithData < m.Of {
			s += " (" + fmt.Sprintf(f.l.coverage, m.WithData, m.Of, f.l.buildingsWord) + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

func (f formatter) price(r report.PriceRange) string {
	switch {
	case r.Min == nil:
		return f.l.noData
	case r.Max == nil || r.Min.Equal(*r.Max):
		return f.num(*r.Min, 4) + " " + f.l.perKwh
	}
	return f.num(*r.Min, 4) + " – " + f.num(*r.Max, 4) + " " + f.l.perKwh
}

func (f formatter) pct(v *decimal.Decimal) string {
	if v == nil {
		return f.l.noData
	}
	return "%" + f.num(*v, 1)
}

func (f formatter) delta(d report.Delta) string {
	if d.Pct == nil {
		return "—"
	}
	sign := ""
	if d.Pct.IsPositive() {
		sign = "+"
	}
	return sign + f.num(*d.Pct, 1) + " %"
}

func (f formatter) currencyDeltas(ds []report.CurrencyDelta) string {
	if len(ds) == 0 {
		return "—"
	}
	parts := make([]string, 0, len(ds))
	for _, d := range ds {
		parts = append(parts, d.Currency+" "+f.delta(report.Delta{Pct: d.Pct}))
	}
	return strings.Join(parts, "; ")
}

func (f formatter) opt(v *decimal.Decimal, places int32, unit string) string {
	if v == nil {
		return f.l.noData
	}
	return f.num(*v, places) + " " + unit
}

func (f formatter) header(doc *Document, title, company, period string, buildings []string) {
	doc.Title = title
	doc.Lines = []string{f.l.company + ": " + company, f.l.buildings + ": " + strings.Join(buildings, ", "), f.l.period + ": " + period}
}

func (f formatter) chartCurrency(c *string) string {
	if c == nil {
		return "TRY"
	}
	return *c
}

func (f formatter) omittedNote(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	return []string{f.l.omitted + ": " + strings.Join(list, ", ")}
}

func (f formatter) monthly(doc *Document, m report.Monthly, company string, buildings []string) {
	l := f.l
	period := l.months[m.Month-1] + " " + strconv.Itoa(m.Year)
	f.header(doc, l.monthlyTitle, company, period, buildings)
	if m.Partial {
		doc.Notes = append(doc.Notes, l.partial)
	}
	var tariffs, prices []string
	for _, b := range m.Buildings {
		name := l.noData
		if b.TariffName != nil {
			name = *b.TariffName
		}
		tariffs = append(tariffs, b.Name+": "+name)
		price := l.noData
		if b.PurchasePrice != nil {
			price = f.num(*b.PurchasePrice, 4) + " " + l.perKwh
		}
		prices = append(prices, b.Name+": "+price)
	}
	avg := l.noData
	if len(m.AveragePurchasePrice) > 0 {
		var parts []string
		for _, a := range m.AveragePurchasePrice {
			parts = append(parts, f.num(a.Value, 4)+" "+a.Currency+"/kWh")
		}
		avg = strings.Join(parts, "; ")
	}
	values := [15]string{
		period, strings.Join(buildings, ", "), strings.Join(tariffs, "; "), strings.Join(prices, "; "), avg,
		f.price(m.RooftopFeedIn), f.price(m.UtilityFeedIn),
		f.figure(m.Consumption, "kWh", l.buildingsWord), f.figure(m.DailyConsumption, l.kwhDay, l.buildingsWord),
		f.figure(m.Rooftop, "kWh", l.buildingsWord), f.figure(m.Utility, "kWh", l.plantsWord),
		f.figure(m.Production, "kWh", l.buildingsWord+"/"+l.plantsWord), f.figure(m.DailyProduction, l.kwhDay, l.buildingsWord+"/"+l.plantsWord),
		f.money(m.Bill), f.money(m.ReactivePenalty),
	}
	info := Section{Key: "info", Title: l.info}
	for i, label := range l.infoRows {
		info.Rows = append(info.Rows, []string{label, values[i]})
	}
	summary := Section{Key: "summary", Title: l.summary, Rows: [][]string{
		{l.summaryRows[0], f.figure(m.Consumption, "kWh", l.buildingsWord), f.delta(m.ConsumptionDelta)},
		{l.summaryRows[1], f.figure(m.Production, "kWh", l.buildingsWord+"/"+l.plantsWord), f.delta(m.ProductionDelta)},
		{l.summaryRows[2], f.money(m.Bill), f.currencyDeltas(m.BillDelta)},
		{l.summaryRows[3], f.money(m.ReactivePenalty), f.currencyDeltas(m.ReactivePenaltyDelta)},
	}}
	cur, prev := strconv.Itoa(m.Year), strconv.Itoa(m.Year-1)
	currency := f.chartCurrency(m.ChartCurrency)
	doc.Sections = append(doc.Sections, info, summary,
		f.monthChart("consumption_chart", l.consumptionChart, l.kwhMonth, cur, prev, m.ConsumptionChart, nil),
		f.monthChart("bill_chart", l.billChart, fmt.Sprintf(l.moneyMonth, currency), cur, prev, m.BillChart, f.omittedNote(m.OmittedCurrencies)))
	f.dropEmptyCharts(doc)
}

func (f formatter) monthChart(key, title, unit, cur, prev string, points []report.MonthPoint, notes []string) Section {
	s := Section{Key: key, Title: title + " (" + unit + ")", Header: []string{f.l.month, cur, prev}, Notes: notes,
		Chart: &Chart{Unit: unit, Series: []Series{{Name: cur}, {Name: prev}}}}
	for _, p := range points {
		name := f.l.months[p.Month-1]
		s.Chart.Categories = append(s.Chart.Categories, name)
		s.Chart.Series[0].Values = append(s.Chart.Series[0].Values, p.Current)
		s.Chart.Series[1].Values = append(s.Chart.Series[1].Values, p.Previous)
		s.Rows = append(s.Rows, []string{name, f.opt(p.Current, 2, ""), f.opt(p.Previous, 2, "")})
	}
	return s
}

func (f formatter) yearly(doc *Document, y report.Yearly, company string, buildings []string) {
	l := f.l
	f.header(doc, l.yearlyTitle, company, strconv.Itoa(y.Year), buildings)
	if y.Partial {
		doc.Notes = append(doc.Notes, l.partial)
	}
	table := Section{Key: "consumption_table", Title: l.consumptionTable,
		Header: []string{l.month, l.consumption, l.rooftop, l.bill, l.reactive}}
	for _, r := range y.Months {
		table.Rows = append(table.Rows, []string{l.months[r.Month-1], f.figure(r.Consumption, "", l.buildingsWord),
			f.figure(r.Rooftop, "", l.buildingsWord), f.money(r.Bill), f.money(r.ReactivePenalty)})
	}
	table.Rows = append(table.Rows,
		[]string{l.total, f.figure(y.Consumption, "", l.buildingsWord), f.figure(y.Rooftop, "", l.buildingsWord), f.money(y.Bill), f.money(y.ReactivePenalty)},
		[]string{l.dailyConsumption, f.figure(y.DailyConsumption, "", l.buildingsWord), "", "", ""},
		[]string{l.dailyRooftop, "", f.figure(y.DailyRooftop, "", l.buildingsWord), "", ""})

	solar := Section{Key: "solar", Title: l.solar, Rows: [][]string{
		{l.solarTotal, f.figure(y.Utility, l.kwhYear, l.plantsWord)},
		{l.target, f.opt(y.Target, 2, l.kwhYear)},
		{l.achievement, f.pct(y.AchievementPct)},
		{l.dailyProduction, f.figure(y.DailyUtility, l.kwhDay, l.plantsWord)},
	}}
	target := &Chart{Unit: l.kwhYear, Series: []Series{{Name: l.targetSeries}, {Name: l.actualSeries}}}
	for _, p := range y.Plants {
		solar.Rows = append(solar.Rows, []string{p.Name, f.figure(p.Production, "kWh", l.plantsWord) + " / " +
			f.opt(p.Target, 2, "kWh") + " / " + f.pct(p.AchievementPct)})
		target.Categories = append(target.Categories, p.Name)
		target.Series[0].Values = append(target.Series[0].Values, p.Target)
		target.Series[1].Values = append(target.Series[1].Values, p.Production.Value)
	}

	summary := Section{Key: "summary", Title: l.summary, Rows: [][]string{
		{l.yearlySummary[0], f.figure(y.Consumption, "kWh", l.buildingsWord), f.delta(y.ConsumptionDelta)},
		{l.yearlySummary[1], f.figure(y.Production, "kWh", l.buildingsWord+"/"+l.plantsWord), ""},
		{l.yearlySummary[2], f.money(y.Bill), f.currencyDeltas(y.BillDelta)},
		{l.yearlySummary[3], f.pct(y.AchievementPct), ""},
	}}

	currency := f.chartCurrency(y.ChartCurrency)
	history := Section{Key: "history_chart", Title: l.historyChart + " (" + l.kwhYear + ")",
		Header: []string{"", l.yearlyConsumption, l.yearlyProduction},
		Chart:  &Chart{Unit: l.kwhYear, Series: []Series{{Name: l.yearlyConsumption}, {Name: l.yearlyProduction}}}}
	bill := Section{Key: "bill_chart", Title: l.yearlyBillChart + " (" + fmt.Sprintf(l.moneyYear, currency) + ")",
		Header: []string{"", l.bill}, Notes: f.omittedNote(y.OmittedCurrencies),
		Chart: &Chart{Unit: fmt.Sprintf(l.moneyYear, currency), Series: []Series{{Name: l.bill}}}}
	for _, h := range y.History {
		year := strconv.Itoa(h.Year)
		history.Chart.Categories = append(history.Chart.Categories, year)
		history.Chart.Series[0].Values = append(history.Chart.Series[0].Values, h.Consumption)
		history.Chart.Series[1].Values = append(history.Chart.Series[1].Values, h.Production)
		history.Rows = append(history.Rows, []string{year, f.opt(h.Consumption, 2, ""), f.opt(h.Production, 2, "")})
		var v *decimal.Decimal
		for _, m := range h.Bill {
			if m.Currency == currency {
				x := m.Value
				v = &x
			}
		}
		bill.Chart.Categories = append(bill.Chart.Categories, year)
		bill.Chart.Series[0].Values = append(bill.Chart.Series[0].Values, v)
		bill.Rows = append(bill.Rows, []string{year, f.opt(v, 2, "")})
	}
	targetSection := Section{Key: "target_chart", Title: l.targetChart + " (" + l.kwhYear + ")", Chart: target,
		Header: []string{"", l.targetSeries, l.actualSeries}}
	for i, name := range target.Categories {
		targetSection.Rows = append(targetSection.Rows, []string{name, f.opt(target.Series[0].Values[i], 2, ""), f.opt(target.Series[1].Values[i], 2, "")})
	}

	comparison := Section{Key: "comparison", Title: l.comparison, Rows: [][]string{
		{l.compConsumption, f.figure(y.Consumption, l.kwhYear, l.buildingsWord)},
		{l.compProduction, f.figure(y.Production, l.kwhYear, l.buildingsWord+"/"+l.plantsWord)},
		{l.solarShare, f.pct(y.SolarSharePct)},
		{l.gridShare, f.pct(y.GridSharePct)},
	}}

	carbon := Section{Key: "carbon", Title: l.carbon}
	if c := y.Carbon; c != nil {
		carbon.Rows = [][]string{
			{l.carbonConsumption, f.opt(c.ConsumptionT, 3, l.tonYear)},
			{l.carbonReduction, f.opt(c.ReductionT, 3, l.tonYear)},
			{l.carbonNet, f.opt(c.NetT, 3, l.tonYear)},
			{l.carbonFactor, f.num(c.Factor, 4) + " " + c.FactorUnit},
		}
		note := l.carbonNote
		if c.SourceYear != nil {
			note += " (" + l.sourceYear + " " + strconv.Itoa(*c.SourceYear) + ")"
		}
		carbon.Notes = []string{note}
	} else {
		carbon.Notes = []string{l.carbonUnavailable}
	}
	doc.Sections = append(doc.Sections, table, solar, summary, history, bill, targetSection, comparison, carbon)
	f.dropEmptyCharts(doc)
}

// dropEmptyCharts replaces a chart with nothing to draw by a sentence: an
// empty axis frame says nothing (07 §5).
func (f formatter) dropEmptyCharts(doc *Document) {
	for i := range doc.Sections {
		c := doc.Sections[i].Chart
		if c == nil {
			continue
		}
		empty := true
		for _, s := range c.Series {
			for _, v := range s.Values {
				if v != nil && v.IsPositive() {
					empty = false
				}
			}
		}
		if empty {
			doc.Sections[i].Chart = nil
			doc.Sections[i].Notes = append([]string{f.l.noChartData}, doc.Sections[i].Notes...)
		}
	}
}
