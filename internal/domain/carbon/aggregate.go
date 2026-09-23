package carbon

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Record is an activity as the aggregates read it.
type Record struct {
	Sub        string
	Start, End time.Time
	KgCO2e     decimal.Decimal
	Quantity   decimal.Decimal
	Status     model.CarbonStatus
	Automated  bool
}

// SubTotal is one sub-category's emission.
type SubTotal struct {
	Sub    string
	KgCO2e decimal.Decimal
}

// Overview is R310's year figures, every one a sum of the same R308 shares.
type Overview struct {
	Total   decimal.Decimal
	Count   int
	Pending int
	ByMain  map[string]decimal.Decimal
	ByScope map[model.CarbonScope]decimal.Decimal
	Monthly [12]decimal.Decimal
	Highest *SubTotal
}

func counts(r Record) bool { return r.Status != model.CarbonStatusRejected } // R309

// BuildYear aggregates the records overlapping a calendar year. Each record's
// share is computed per month once and added everywhere, so the totals
// reconcile exactly.
func BuildYear(records []Record, year int) Overview {
	o := Overview{ByMain: map[string]decimal.Decimal{}, ByScope: map[model.CarbonScope]decimal.Decimal{}}
	for _, m := range Mains() {
		o.ByMain[m] = decimal.Zero
	}
	for _, s := range model.CarbonScopes() {
		o.ByScope[s] = decimal.Zero
	}
	bySub := map[string]decimal.Decimal{}
	yearStart, yearEnd := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
	for _, r := range records {
		if !counts(r) || civil(r.End).Before(yearStart) || civil(r.Start).After(yearEnd) {
			continue
		}
		o.Count++
		if r.Status == model.CarbonStatusPending {
			o.Pending++
		}
		sub, _ := SubByKey(r.Sub)
		d := Dated{Start: r.Start, End: r.End, Value: r.KgCO2e}
		for m := 0; m < 12; m++ {
			from := time.Date(year, time.Month(m+1), 1, 0, 0, 0, 0, time.UTC)
			v := Share(d, from, from.AddDate(0, 1, -1))
			if v.IsZero() {
				continue
			}
			o.Monthly[m] = o.Monthly[m].Add(v)
			o.Total = o.Total.Add(v)
			o.ByMain[sub.Main] = o.ByMain[sub.Main].Add(v)
			o.ByScope[sub.Scope] = o.ByScope[sub.Scope].Add(v)
			bySub[r.Sub] = bySub[r.Sub].Add(v)
		}
	}
	for _, k := range sortedKeys(bySub) {
		if o.Highest == nil || bySub[k].GreaterThan(o.Highest.KgCO2e) {
			o.Highest = &SubTotal{Sub: k, KgCO2e: bySub[k]}
		}
	}
	return o
}

func sortedKeys(m map[string]decimal.Decimal) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
