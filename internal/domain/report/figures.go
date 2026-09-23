package report

import (
	"slices"
	"time"

	"github.com/shopspring/decimal"
)

var hundred = decimal.NewFromInt(100)

// DaysIn is the number of days in a calendar month.
func DaysIn(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// DeltaOf is R259: (cur − prev) ÷ prev × 100 to one decimal, omitted when
// either side is missing or prev is zero.
func DeltaOf(cur, prev *decimal.Decimal) Delta {
	if cur == nil || prev == nil || prev.IsZero() {
		return Delta{}
	}
	pct := cur.Sub(*prev).Div(*prev).Mul(hundred).Round(1)
	return Delta{Pct: &pct}
}

// sumFigure adds the non-nil values; nil when none has data (R258).
func sumFigure(values []*decimal.Decimal, partial []bool) Figure {
	f := Figure{Of: len(values)}
	var total decimal.Decimal
	for i, v := range values {
		if v == nil {
			continue
		}
		f.WithData++
		total = total.Add(*v)
		if i < len(partial) && partial[i] {
			f.Partial = true
		}
	}
	if f.WithData > 0 {
		f.Value = &total
	}
	return f
}

// combine adds figures the plant selection did not exclude.
func combine(parts ...Figure) Figure {
	var out Figure
	var total decimal.Decimal
	for _, p := range parts {
		if p.Excluded {
			continue
		}
		out.Of += p.Of
		out.WithData += p.WithData
		out.Partial = out.Partial || p.Partial
		if p.Value != nil {
			total = total.Add(*p.Value)
			out.Value = &total
		}
	}
	return out
}

// perDay divides a figure by a day count, keeping its provenance.
func perDay(f Figure, days int) Figure {
	out := f
	if f.Value != nil {
		v := f.Value.DivRound(decimal.NewFromInt(int64(days)), 4)
		out.Value = &v
	}
	return out
}

// excluded is a figure the plant selection leaves out.
func excluded() Figure { return Figure{Excluded: true} }

// moneyByCurrency sums pick over the non-nil bills, one figure per currency,
// TRY first and the rest alphabetically (R270).
func moneyByCurrency(bills []*Bill, pick func(*Bill) decimal.Decimal) []MoneyFigure {
	idx := map[string]int{}
	var out []MoneyFigure
	for _, b := range bills {
		if b == nil {
			continue
		}
		i, ok := idx[b.Currency]
		if !ok {
			i = len(out)
			idx[b.Currency] = i
			out = append(out, MoneyFigure{Currency: b.Currency, Of: len(bills)})
		}
		out[i].Value = out[i].Value.Add(pick(b))
		out[i].WithData++
	}
	sortMoney(out)
	return out
}

func sortMoney(out []MoneyFigure) {
	slices.SortFunc(out, func(a, b MoneyFigure) int { return compareCurrency(a.Currency, b.Currency) })
}

func compareCurrency(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "TRY":
		return -1
	case b == "TRY":
		return 1
	case a < b:
		return -1
	}
	return 1
}

// moneyValue finds one currency's value in a list.
func moneyValue(list []MoneyFigure, currency string) *decimal.Decimal {
	for _, m := range list {
		if m.Currency == currency {
			v := m.Value
			return &v
		}
	}
	return nil
}

// moneyDeltas is one R259 delta per currency present in cur.
func moneyDeltas(cur, prev []MoneyFigure) []CurrencyDelta {
	out := make([]CurrencyDelta, 0, len(cur))
	for _, m := range cur {
		v := m.Value
		out = append(out, CurrencyDelta{Currency: m.Currency, Pct: DeltaOf(&v, moneyValue(prev, m.Currency)).Pct})
	}
	return out
}

// priceRange is R257's one-value-or-range over the resolved prices.
func priceRange(values []*decimal.Decimal) PriceRange {
	var r PriceRange
	for _, v := range values {
		if v == nil {
			continue
		}
		if r.Min == nil || v.LessThan(*r.Min) {
			x := *v
			r.Min = &x
		}
		if r.Max == nil || v.GreaterThan(*r.Max) {
			x := *v
			r.Max = &x
		}
	}
	return r
}

// ratio is num ÷ den × 100 to two decimals, nil when den is zero.
func ratio(num, den decimal.Decimal) *decimal.Decimal {
	if den.IsZero() {
		return nil
	}
	v := num.Div(den).Mul(hundred).Round(2)
	return &v
}

// chartCurrency picks the currency a money chart draws: TRY when present,
// else the first alphabetically; the rest are named as omitted (R270).
func chartCurrency(lists ...[]MoneyFigure) (*string, []string) {
	seen := map[string]bool{}
	var all []string
	for _, l := range lists {
		for _, m := range l {
			if !seen[m.Currency] {
				seen[m.Currency] = true
				all = append(all, m.Currency)
			}
		}
	}
	if len(all) == 0 {
		return nil, nil
	}
	slices.SortFunc(all, compareCurrency)
	first := all[0]
	return &first, all[1:]
}

func at(m map[int]MonthSeries, year, month int) *decimal.Decimal {
	s, ok := m[year]
	if !ok {
		return nil
	}
	return s[month-1]
}

func partialAt(m map[int][12]bool, year, month int) bool {
	s, ok := m[year]
	return ok && s[month-1]
}

func billAt(m map[int][12]*Bill, year, month int) *Bill {
	s, ok := m[year]
	if !ok {
		return nil
	}
	return s[month-1]
}
