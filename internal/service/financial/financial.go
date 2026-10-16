// Package financial is 01 §7.9's company-wide view (R294): consumption and
// cost from the building invoices, production from the plants, revenue at
// each plant's feed-in tariff, and the arithmetic between them.
package financial

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Money is one currency's amount; currencies never add (R253).
type Money struct {
	Currency model.CurrencyCode
	Amount   decimal.Decimal
}

// Price is a feed-in price with its currency.
type Price struct {
	Value    decimal.Decimal
	Currency model.CurrencyCode
}

// YearInputs is a year's building bills, plant production and feed-in prices.
type YearInputs struct {
	Year int
	// Bills are the year's non-superseded building- and analyzer-scope
	// invoices; an analyzer bill counts only when its building has no
	// building bill that month.
	Bills []model.Bill
	// Production is plant → month (1–12) → kWh; FeedIn plant → month → the
	// price effective on that month's first day.
	Production map[uuid.UUID]map[int]decimal.Decimal
	FeedIn     map[uuid.UUID]map[int]Price
	// Metered is month → the building analyzers' measured import/export,
	// used for a month without bills.
	Metered map[int]Metered
}

// Metered is a month's measured grid import and export, kWh; nil = no data.
type Metered struct{ ImportKwh, ExportKwh *decimal.Decimal }

// Month is one row of the monthly table; nil means no data, never zero.
// Consumption is the gross grid import (a prosumer's netted figure would be
// 0); the meter's export counts as production. Measured marks a month whose
// kWh come from meter readings because it has no bill.
type Month struct {
	Month           int
	Measured        bool
	ConsumptionKwh  *decimal.Decimal
	Cost            []Money
	ProductionKwh   *decimal.Decimal
	Revenue         []Money
	RevenuePartial  bool
	OffsetKwh       *decimal.Decimal
	GridPurchaseKwh *decimal.Decimal
	GridSaleKwh     *decimal.Decimal
	Net             []Money
}

// Year is twelve months, their total and how many months had data.
type Year struct {
	Months                                          [12]Month
	Total                                           Month
	ConsumptionMonths, ProductionMonths, BillMonths int
}

type sums map[model.CurrencyCode]decimal.Decimal

func (s sums) list() []Money {
	out := []Money{}
	for _, c := range model.CurrencyCodes() {
		if v, ok := s[c]; ok {
			out = append(out, Money{Currency: c, Amount: v.Round(2)})
		}
	}
	return out
}

func add(a *decimal.Decimal, b decimal.Decimal) *decimal.Decimal {
	if a == nil {
		return &b
	}
	v := a.Add(b)
	return &v
}

func counts(b model.Bill, whole map[uuid.UUID]bool) bool {
	switch b.Scope {
	case model.BillScopeBuilding:
		return true
	case model.BillScopeAnalyzer:
		return b.BuildingID == nil || !whole[*b.BuildingID]
	}
	return false
}

// BuildYear computes R294's months and total.
func BuildYear(in YearInputs) Year {
	var y Year
	totalCost, totalRevenue, totalNet := sums{}, sums{}, sums{}
	for m := 1; m <= 12; m++ {
		row := Month{Month: m}
		key := fmt.Sprintf("%04d-%02d", in.Year, m)
		cost := sums{}
		var export *decimal.Decimal
		billed := false
		whole := map[uuid.UUID]bool{}
		for _, b := range in.Bills {
			if b.PeriodKey == key && b.Scope == model.BillScopeBuilding && b.BuildingID != nil {
				whole[*b.BuildingID] = true
			}
		}
		for _, b := range in.Bills {
			if b.PeriodKey != key || !counts(b, whole) {
				continue
			}
			billed = true
			row.ConsumptionKwh = add(row.ConsumptionKwh, b.ActiveImport)
			export = add(export, b.ActiveExport)
			cost[b.Currency] = cost[b.Currency].Add(b.TotalCost)
		}
		if billed {
			y.BillMonths++
		} else if mt, ok := in.Metered[m]; ok {
			row.ConsumptionKwh, export = mt.ImportKwh, mt.ExportKwh
			row.Measured = row.ConsumptionKwh != nil || export != nil
		}
		revenue := sums{}
		if export != nil && export.IsPositive() {
			// No feed-in price is known for a meter's export.
			row.ProductionKwh, row.RevenuePartial = add(row.ProductionKwh, *export), true
		}
		for plant, months := range in.Production {
			kwh, ok := months[m]
			if !ok {
				continue
			}
			row.ProductionKwh = add(row.ProductionKwh, kwh)
			price, priced := in.FeedIn[plant][m]
			if !priced {
				row.RevenuePartial = true
				continue
			}
			revenue[price.Currency] = revenue[price.Currency].Add(kwh.Mul(price.Value))
		}
		row.Cost, row.Revenue = cost.list(), revenue.list()
		if row.ConsumptionKwh != nil && row.ProductionKwh != nil {
			offset := row.ProductionKwh.Sub(*row.ConsumptionKwh)
			purchase, sale := decimal.Max(offset.Neg(), decimal.Zero), decimal.Max(offset, decimal.Zero)
			row.OffsetKwh, row.GridPurchaseKwh, row.GridSaleKwh = &offset, &purchase, &sale
		}
		// Net is cost − revenue whenever there is a cost (legacy dropped the
		// revenue when consumption exceeded production).
		net := sums{}
		if len(cost) > 0 {
			for c, v := range cost {
				net[c] = v.Sub(revenue[c])
			}
			for c, v := range revenue {
				if _, ok := cost[c]; !ok {
					net[c] = v.Neg()
				}
			}
		}
		row.Net = net.list()
		for c, v := range cost {
			totalCost[c] = totalCost[c].Add(v)
		}
		for c, v := range revenue {
			totalRevenue[c] = totalRevenue[c].Add(v)
		}
		for c, v := range net {
			totalNet[c] = totalNet[c].Add(v)
		}
		if row.ConsumptionKwh != nil {
			y.ConsumptionMonths++
			y.Total.ConsumptionKwh = add(y.Total.ConsumptionKwh, *row.ConsumptionKwh)
		}
		if row.ProductionKwh != nil {
			y.ProductionMonths++
			y.Total.ProductionKwh = add(y.Total.ProductionKwh, *row.ProductionKwh)
		}
		y.Total.RevenuePartial = y.Total.RevenuePartial || row.RevenuePartial
		y.Total.Measured = y.Total.Measured || row.Measured
		y.Months[m-1] = row
	}
	y.Total.Cost, y.Total.Revenue, y.Total.Net = totalCost.list(), totalRevenue.list(), totalNet.list()
	if y.Total.ConsumptionKwh != nil && y.Total.ProductionKwh != nil {
		offset := y.Total.ProductionKwh.Sub(*y.Total.ConsumptionKwh)
		purchase, sale := decimal.Max(offset.Neg(), decimal.Zero), decimal.Max(offset, decimal.Zero)
		y.Total.OffsetKwh, y.Total.GridPurchaseKwh, y.Total.GridSaleKwh = &offset, &purchase, &sale
	}
	return y
}
