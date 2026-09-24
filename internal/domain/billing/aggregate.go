package billing

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

// AggregateQuantities sums members per register (R116): nil in any member is
// nil. ActiveExportKnownSum sums the members that report export (I-18);
// MaxDemandKw is kept only for a single member (R111).
func AggregateQuantities(members []Quantities) Quantities {
	var out Quantities
	if len(members) == 0 {
		return out
	}
	sum := func(get func(Quantities) *decimal.Decimal) *decimal.Decimal {
		total := decimal.Zero
		for _, m := range members {
			v := get(m)
			if v == nil {
				return nil
			}
			total = total.Add(*v)
		}
		return &total
	}
	out.ActiveImport = sum(func(q Quantities) *decimal.Decimal { return q.ActiveImport })
	out.T1 = sum(func(q Quantities) *decimal.Decimal { return q.T1 })
	out.T2 = sum(func(q Quantities) *decimal.Decimal { return q.T2 })
	out.T3 = sum(func(q Quantities) *decimal.Decimal { return q.T3 })
	out.ReactiveInductive = sum(func(q Quantities) *decimal.Decimal { return q.ReactiveInductive })
	out.ReactiveCapacitive = sum(func(q Quantities) *decimal.Decimal { return q.ReactiveCapacitive })
	out.ActiveExport = sum(func(q Quantities) *decimal.Decimal { return q.ActiveExport })
	reported := 0
	for _, m := range members {
		out.ActiveExportKnownSum = out.ActiveExportKnownSum.Add(m.ActiveExportKnownSum)
		if m.ActiveExport != nil {
			reported++
		}
		out.ActiveExportPartial = out.ActiveExportPartial || m.ActiveExportPartial
	}
	out.ActiveExportPartial = out.ActiveExportPartial || (reported > 0 && reported < len(members))
	if len(members) == 1 {
		out.MaxDemandKw = members[0].MaxDemandKw
	}
	out.IndexStart = sumIndexes(members, func(q Quantities) map[string]*decimal.Decimal { return q.IndexStart })
	out.IndexEnd = sumIndexes(members, func(q Quantities) map[string]*decimal.Decimal { return q.IndexEnd })
	return out
}

func sumIndexes(members []Quantities, get func(Quantities) map[string]*decimal.Decimal) map[string]*decimal.Decimal {
	out := map[string]*decimal.Decimal{}
	for _, m := range members {
		for k := range get(m) {
			out[k] = nil
		}
	}
	for k := range out {
		total := decimal.Zero
		known := true
		for _, m := range members {
			v := get(m)[k]
			if v == nil {
				known = false
				break
			}
			total = total.Add(*v)
		}
		if known {
			out[k] = &total
		}
	}
	return out
}

// SumInstalledPower is nil when members is empty or any member is nil (R116).
func SumInstalledPower(members []*decimal.Decimal) *decimal.Decimal {
	if len(members) == 0 {
		return nil
	}
	total := decimal.Zero
	for _, m := range members {
		if m == nil {
			return nil
		}
		total = total.Add(*m)
	}
	return &total
}

// AggregateHours sums per hour only where every member has that hour (R116);
// an hour some members lack is reported missing.
func AggregateHours(members [][]tariff.HourConsumption) (sum []tariff.HourConsumption, missing []time.Time) {
	type acc struct {
		at    time.Time
		total decimal.Decimal
		count int
	}
	byHour := map[int64]*acc{}
	for _, series := range members {
		for _, h := range series {
			a := byHour[h.Hour.UnixNano()]
			if a == nil {
				a = &acc{at: h.Hour}
				byHour[h.Hour.UnixNano()] = a
			}
			a.total, a.count = a.total.Add(h.Kwh), a.count+1
		}
	}
	for _, a := range byHour {
		if a.count == len(members) {
			sum = append(sum, tariff.HourConsumption{Hour: a.at, Kwh: a.total})
		} else {
			missing = append(missing, a.at)
		}
	}
	sort.Slice(sum, func(i, j int) bool { return sum[i].Hour.Before(sum[j].Hour) })
	slices.SortFunc(missing, func(a, b time.Time) int { return a.Compare(b) })
	return sum, missing
}

// CombineCompany sums building invoices (R115). Lines merge by (Code, RatePct);
// a merged UnitPrice is nil when the buildings' prices differ (M-7). VAT is the
// sum of building VATs and TotalCost the sum of building totals.
func CombineCompany(periodKey string, buildings []Invoice) (Invoice, error) {
	if len(buildings) == 0 {
		return Invoice{}, fmt.Errorf("%w: no buildings", ErrInvalidInput)
	}
	first := buildings[0]
	out := Invoice{
		PeriodKey: periodKey, Period: first.Period, Currency: first.Currency,
		DemandDataAvailable: true, GenerationUsage: first.GenerationUsage,
	}
	type key struct{ code, rate string }
	index := map[key]int{}
	flags := map[Flag]bool{}
	for _, b := range buildings {
		if b.Currency != out.Currency {
			return Invoice{}, fmt.Errorf("%w: mixed currencies %s and %s", ErrInvalidInput, out.Currency, b.Currency)
		}
		if b.Period.From.Before(out.Period.From) {
			out.Period.From = b.Period.From
		}
		if b.Period.To.After(out.Period.To) {
			out.Period.To = b.Period.To
		}
		for _, pair := range []struct{ dst, src *decimal.Decimal }{
			{&out.ActiveImport, &b.ActiveImport}, {&out.T1, &b.T1}, {&out.T2, &b.T2}, {&out.T3, &b.T3},
			{&out.Inductive, &b.Inductive}, {&out.Capacitive, &b.Capacitive}, {&out.ActiveExport, &b.ActiveExport},
			{&out.NetConsumption, &b.NetConsumption}, {&out.LowTierKwh, &b.LowTierKwh}, {&out.HighTierKwh, &b.HighTierKwh},
			{&out.EnergyCost, &b.EnergyCost}, {&out.DistributionCost, &b.DistributionCost}, {&out.GreenEnergyCost, &b.GreenEnergyCost},
			{&out.PowerCost, &b.PowerCost}, {&out.DemandOverrunCost, &b.DemandOverrunCost}, {&out.ReactivePenalty, &b.ReactivePenalty},
			{&out.ExtraChargesCost, &b.ExtraChargesCost}, {&out.OtherTaxesCost, &b.OtherTaxesCost}, {&out.VatBase, &b.VatBase},
			{&out.VatCost, &b.VatCost}, {&out.GenerationCredit, &b.GenerationCredit}, {&out.TotalCost, &b.TotalCost},
		} {
			*pair.dst = pair.dst.Add(*pair.src)
		}
		out.TieredApplied = out.TieredApplied || b.TieredApplied
		out.DemandDataAvailable = out.DemandDataAvailable && b.DemandDataAvailable
		out.PtfYekdemUsed = out.PtfYekdemUsed || b.PtfYekdemUsed
		out.Reactive.Applied = out.Reactive.Applied || b.Reactive.Applied
		if b.GenerationUsage != out.GenerationUsage {
			out.GenerationUsage = model.GenerationUsageNone
		}
		for _, f := range b.Flags {
			flags[f] = true
		}
		for _, l := range b.Lines {
			k := key{l.Code, ""}
			if l.RatePct != nil {
				k.rate = l.RatePct.String()
			}
			i, ok := index[k]
			if !ok {
				index[k] = len(out.Lines)
				out.Lines = append(out.Lines, cloneLine(l))
				continue
			}
			m := &out.Lines[i]
			m.Amount = m.Amount.Add(l.Amount)
			if m.Quantity != nil && l.Quantity != nil {
				q := m.Quantity.Add(*l.Quantity)
				m.Quantity = &q
			} else {
				m.Quantity = nil
			}
			if m.UnitPrice == nil || l.UnitPrice == nil || !m.UnitPrice.Equal(*l.UnitPrice) {
				m.UnitPrice = nil
			}
		}
	}
	// Merged lines keep VAT and the generation credit last, as on a building invoice.
	rank := func(code string) int {
		switch code {
		case model.BillLineVat:
			return 1
		case model.BillLineGenerationCredit:
			return 2
		}
		return 0
	}
	slices.SortStableFunc(out.Lines, func(a, b Line) int { return rank(a.Code) - rank(b.Code) })
	out.Days = int((out.Period.To.Sub(out.Period.From) + 12*time.Hour) / (24 * time.Hour))
	if out.NetConsumption.IsPositive() {
		v := out.EnergyCost.DivRound(out.NetConsumption, tariff.DivisionScale)
		out.EffectiveEnergyPrice = &v
	}
	out.Flags = sortedFlags(flags)
	return out, nil
}

func cloneLine(l Line) Line {
	c := l
	if l.Quantity != nil {
		q := *l.Quantity
		c.Quantity = &q
	}
	return c
}
