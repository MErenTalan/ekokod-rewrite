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
	// Bills are building-scope, non-superseded invoices of the year.
	Bills []model.Bill
	// Production is plant → month (1–12) → kWh; FeedIn plant → month → the
	// price effective on that month's first day.
	Production map[uuid.UUID]map[int]decimal.Decimal
	FeedIn     map[uuid.UUID]map[int]Price
}

// Month is one row of the monthly table; nil means no data, never zero.
type Month struct {
	Month           int
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
	Months                              [12]Month
	Total                               Month
	ConsumptionMonths, ProductionMonths int
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

// BuildYear computes R294's months and total.
func BuildYear(in YearInputs) Year {
	var y Year
	totalCost, totalRevenue, totalNet := sums{}, sums{}, sums{}
	for m := 1; m <= 12; m++ {
		row := Month{Month: m}
		key := fmt.Sprintf("%04d-%02d", in.Year, m)
		cost := sums{}
		for _, b := range in.Bills {
			if b.PeriodKey != key {
				continue
			}
			row.ConsumptionKwh = add(row.ConsumptionKwh, b.NetConsumption)
			cost[b.Currency] = cost[b.Currency].Add(b.TotalCost)
		}
		revenue := sums{}
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
