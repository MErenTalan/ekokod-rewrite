package icmal

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

const divisionScale int32 = 20

// Coefficient is one derived value aggregated over a group's rows (02 §8.3).
type Coefficient struct {
	Value            *decimal.Decimal // median, rounded 4 dp (rates 2 dp)
	Samples          int
	StdDev           *decimal.Decimal // population
	Stable           *bool            // stddev/median < 0.1; nil below 2 samples
	BackCalcErrorPct *decimal.Decimal // energy only: max over the group's rows
}

// Analysis is the derivation for one ETSO code.
type Analysis struct {
	EtsoCode                                        string
	Periods                                         []string
	EnergyKbk, DistributionTlPerKwh, PowerUnitPrice Coefficient
	ImpliedContractedPowerKw                        *decimal.Decimal
	ReactiveUnitPrice, ReactiveKbk, VatRate         Coefficient
	Taxes                                           map[string]Coefficient
	OveruseRequiresManualEntry                      bool
	IsMultiTime                                     *bool
	Term, VoltageLevel                              *string
	WithinTolerance                                 bool
	Warnings                                        []Warning
}

// Tax names derived per R131.
const (
	TaxBTV        = "BTV"
	TaxEnergyFund = "Enerji Fonu"
	TaxTRT        = "TRT"
)

var (
	hundred        = decimal.NewFromInt(100)
	backCalcLimit  = decimal.NewFromInt(2)
	reactiveMatch  = decimal.RequireFromString("0.0005") // 0.05 %
	stabilityLimit = decimal.RequireFromString("0.1")
)

// Analyse derives coefficients per ETSO code from regular rows only (R130,
// C-1): not cancelled, positive total kWh, no supplementary (Ek) consumption.
// base maps "YYYYMM" to (PTF month average + YEKDEM) / 1000; contracted maps an
// ETSO code to its contracted power (C-2); overrunMultiplier is the dated
// demand overrun multiplier.
func Analyse(rows []model.IcmalRow, base map[string]decimal.Decimal, contracted map[string]decimal.Decimal, overrunMultiplier decimal.Decimal) []Analysis {
	type group struct {
		analysis Analysis
		regular  []int
	}
	var order []string
	groups := map[string]*group{}
	var orphanWarnings []Warning
	for i, r := range rows {
		rowNo := i + 1
		if r.EtsoCode == nil {
			orphanWarnings = append(orphanWarnings, Warning{Row: rowNo, Code: WarnMissingEtso, Text: "row has no ETSO code"})
			continue
		}
		g := groups[*r.EtsoCode]
		if g == nil {
			g = &group{analysis: Analysis{EtsoCode: *r.EtsoCode, OveruseRequiresManualEntry: true, Taxes: map[string]Coefficient{}}}
			groups[*r.EtsoCode] = g
			order = append(order, *r.EtsoCode)
		}
		warn := func(code, text string) {
			g.analysis.Warnings = append(g.analysis.Warnings, Warning{Row: rowNo, Code: code, Text: text})
		}
		switch {
		case r.IsCancelled:
			warn(WarnCancelledExcluded, "cancelled invoice")
		case supplementary(r):
			warn(WarnSupplementaryExcluded, "supplementary correction of an earlier period")
		case r.TotalKwh == nil || !r.TotalKwh.IsPositive():
			warn(WarnZeroKwhExcluded, "no consumption on the row")
		default:
			g.regular = append(g.regular, i)
		}
	}

	consensus, matched := reactiveConsensus(rows, func(i int, code, text string) {
		g := groups[*rows[i].EtsoCode]
		g.analysis.Warnings = append(g.analysis.Warnings, Warning{Row: i + 1, Code: code, Text: text})
	}, func(i int) bool { return slices.Contains(groups[*rows[i].EtsoCode].regular, i) })

	within := true
	out := make([]Analysis, 0, len(order))
	for _, etso := range order {
		g := groups[etso]
		a := &g.analysis
		a.Warnings = append(a.Warnings, orphanWarnings...)
		var energy, distribution, power, reactiveKbk, vat, reactivePrices []decimal.Decimal
		var powerCharges []decimal.Decimal
		taxes := map[string][]decimal.Decimal{}
		type energyRow struct{ kwh, base, actual decimal.Decimal }
		var energyRows []energyRow
		periods := map[string]bool{}
		for _, i := range g.regular {
			r := rows[i]
			rowNo := i + 1
			periods[r.Period] = true
			if a.Term == nil {
				a.Term, a.VoltageLevel, a.IsMultiTime = r.Term, r.VoltageLevel, r.IsMultiTime
			}
			kwh := *r.TotalKwh
			b, hasBase := base[r.Period]
			if !hasBase {
				a.Warnings = append(a.Warnings, Warning{Row: rowNo, Code: WarnBasePriceMissing, Text: "no PTF/YEKDEM for " + r.Period})
			}
			if r.EnergyCharge != nil && hasBase && b.IsPositive() {
				actual := r.EnergyCharge.Sub(val(r.PriceDifference)).Sub(val(r.CorrectionAmount))
				energy = append(energy, actual.DivRound(kwh.Mul(b), divisionScale))
				energyRows = append(energyRows, energyRow{kwh, b, actual})
			}
			if r.DistributionCharge != nil {
				distribution = append(distribution, r.DistributionCharge.DivRound(kwh, divisionScale))
			}
			reactiveQty := val(r.InductiveKvarh).Add(val(r.CapacitiveKvarh))
			if val(r.ReactiveCharge).IsPositive() && reactiveQty.IsPositive() && hasBase && b.IsPositive() {
				reactiveKbk = append(reactiveKbk, r.ReactiveCharge.DivRound(reactiveQty.Mul(b), divisionScale))
			}
			if p, ok := matched[i]; ok {
				reactivePrices = append(reactivePrices, p)
			}
			if pc := val(r.PowerCharge); pc.IsPositive() { // R132, C-2
				powerCharges = append(powerCharges, pc)
				c, hasContract := contracted[etso]
				switch {
				case hasContract && c.IsPositive():
					power = append(power, pc.DivRound(c, divisionScale))
				case val(r.OveruseCharge).IsPositive() && val(r.DemandKw).IsPositive() && overrunMultiplier.IsPositive():
					base := pc.Add(r.OveruseCharge.DivRound(overrunMultiplier, divisionScale))
					power = append(power, base.DivRound(*r.DemandKw, divisionScale))
				default:
					a.Warnings = append(a.Warnings, Warning{Row: rowNo, Code: WarnPowerManualEntry, Text: "power price needs a contracted power or an overrun row"})
				}
			}
			if r.Vat != nil && r.VatBase != nil && r.VatBase.IsPositive() {
				vat = append(vat, r.Vat.Mul(hundred).DivRound(*r.VatBase, divisionScale))
			}
			if e := val(r.EnergyCharge); e.IsPositive() { // R131: taxes are % of energy
				for name, amount := range map[string]*decimal.Decimal{TaxBTV: r.Btv, TaxEnergyFund: r.EnergyFund, TaxTRT: r.Trt} {
					if amount != nil && !amount.IsZero() {
						taxes[name] = append(taxes[name], amount.Mul(hundred).DivRound(e, divisionScale))
					}
				}
			}
		}
		for p := range periods {
			a.Periods = append(a.Periods, p)
		}
		slices.Sort(a.Periods)

		a.EnergyKbk = aggregate(energy, 4)
		a.DistributionTlPerKwh = aggregate(distribution, 4)
		a.PowerUnitPrice = aggregate(power, 4)
		a.ReactiveKbk = aggregate(reactiveKbk, 4)
		a.VatRate = aggregate(vat, 2)
		for name, samples := range taxes {
			a.Taxes[name] = aggregate(samples, 2)
		}
		if len(reactivePrices) > 0 {
			a.ReactiveUnitPrice = aggregate(reactivePrices, 4)
			v := consensus
			a.ReactiveUnitPrice.Value = &v
		}
		if a.PowerUnitPrice.Stable != nil && !*a.PowerUnitPrice.Stable {
			a.Warnings = append(a.Warnings, Warning{Code: WarnPowerUnstable, Text: "PTF-based power pricing may not suit this customer"})
		}
		if a.PowerUnitPrice.Value != nil && a.PowerUnitPrice.Value.IsPositive() {
			implied := median(powerCharges).DivRound(*a.PowerUnitPrice.Value, divisionScale).Round(3)
			a.ImpliedContractedPowerKw = &implied
		}
		if kbk := a.EnergyKbk.Value; kbk != nil { // 02 §8.4 with the group median, never per row
			worst := decimal.Zero
			for _, er := range energyRows {
				if er.actual.IsZero() {
					continue
				}
				predicted := er.kwh.Mul(er.base).Mul(*kbk)
				errPct := predicted.Sub(er.actual).Abs().Mul(hundred).DivRound(er.actual.Abs(), divisionScale)
				worst = decimal.Max(worst, errPct)
			}
			a.EnergyKbk.BackCalcErrorPct = &worst
			within = within && worst.LessThan(backCalcLimit)
		}
		out = append(out, *a)
	}
	for i := range out {
		out[i].WithinTolerance = within
	}
	return out
}

// reactiveConsensus finds the import-wide reactive unit price (R132, I-12): the
// 4 dp value shared by the most rows' R/inductive or R/capacitive candidates.
// It returns, per matching regular row, the candidate within 0.05 % of it.
func reactiveConsensus(rows []model.IcmalRow, warn func(i int, code, text string), regular func(i int) bool) (decimal.Decimal, map[int]decimal.Decimal) {
	candidates := map[int][]decimal.Decimal{}
	tally := map[string]int{}
	var keys []string
	for i, r := range rows {
		if r.EtsoCode == nil || !regular(i) || !val(r.ReactiveCharge).IsPositive() {
			continue
		}
		var cs []decimal.Decimal
		for _, reg := range []*decimal.Decimal{r.InductiveKvarh, r.CapacitiveKvarh} {
			if reg != nil && reg.IsPositive() {
				cs = append(cs, r.ReactiveCharge.DivRound(*reg, divisionScale))
			}
		}
		if len(cs) == 0 {
			warn(i, WarnReactiveQuantity, "reactive charge with no reactive quantity")
			continue
		}
		candidates[i] = cs
		seen := map[string]bool{}
		for _, c := range cs {
			k := c.Round(4).String()
			if !seen[k] {
				seen[k] = true
				if tally[k] == 0 {
					keys = append(keys, k)
				}
				tally[k]++
			}
		}
	}
	if len(keys) == 0 {
		return decimal.Zero, nil
	}
	slices.SortFunc(keys, func(a, b string) int {
		if tally[a] != tally[b] {
			return tally[b] - tally[a]
		}
		return decimal.RequireFromString(a).Cmp(decimal.RequireFromString(b))
	})
	consensus := decimal.RequireFromString(keys[0])
	matched := map[int]decimal.Decimal{}
	for i, cs := range candidates {
		found := false
		for _, c := range cs {
			if c.Sub(consensus).Abs().LessThanOrEqual(consensus.Mul(reactiveMatch)) {
				matched[i], found = c, true
				break
			}
		}
		if !found {
			warn(i, WarnReactiveAmbiguous, "no reactive register matches the import-wide unit price")
		}
	}
	return consensus, matched
}

// supplementary reports an (Ek) correction row: negative kWh, supplementary
// consumption, or a subscriber group ending "(Ek)" (C-1).
func supplementary(r model.IcmalRow) bool {
	if r.TotalKwh != nil && r.TotalKwh.IsNegative() {
		return true
	}
	var raw map[string]string
	if json.Unmarshal(r.Raw, &raw) != nil {
		return false
	}
	for header, value := range raw {
		switch NormaliseHeader(header) {
		case NormaliseHeader("Ek Tüketim T0 Kwh"):
			v := strings.Trim(strings.TrimSpace(value), `"`)
			if v != "" && strings.Trim(v, "0.,-") != "" {
				return true
			}
		case NormaliseHeader("Fatura Abone Grubu"):
			if strings.HasSuffix(strings.TrimSpace(value), "(Ek)") {
				return true
			}
		}
	}
	return false
}

func aggregate(samples []decimal.Decimal, places int32) Coefficient {
	c := Coefficient{Samples: len(samples)}
	if len(samples) == 0 {
		return c
	}
	m := median(samples)
	v := m.Round(places)
	c.Value = &v
	if len(samples) >= 2 {
		sd := stdDev(samples)
		c.StdDev = &sd
		if !m.IsZero() {
			stable := sd.DivRound(m.Abs(), divisionScale).LessThan(stabilityLimit)
			c.Stable = &stable
		}
	}
	return c
}

func median(samples []decimal.Decimal) decimal.Decimal {
	s := slices.Clone(samples)
	slices.SortFunc(s, func(a, b decimal.Decimal) int { return a.Cmp(b) })
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return s[n/2-1].Add(s[n/2]).DivRound(decimal.NewFromInt(2), divisionScale)
}

// stdDev is the population standard deviation, square root by Newton's method.
func stdDev(samples []decimal.Decimal) decimal.Decimal {
	n := decimal.NewFromInt(int64(len(samples)))
	sum := decimal.Zero
	for _, s := range samples {
		sum = sum.Add(s)
	}
	mean := sum.DivRound(n, divisionScale)
	variance := decimal.Zero
	for _, s := range samples {
		d := s.Sub(mean)
		variance = variance.Add(d.Mul(d))
	}
	variance = variance.DivRound(n, divisionScale)
	if variance.IsZero() {
		return decimal.Zero
	}
	x := variance
	if x.LessThan(decimal.NewFromInt(1)) {
		x = decimal.NewFromInt(1)
	}
	two := decimal.NewFromInt(2)
	for range 100 {
		next := x.Add(variance.DivRound(x, divisionScale)).DivRound(two, divisionScale)
		if next.Equal(x) {
			break
		}
		x = next
	}
	return x.Round(10)
}

func val(d *decimal.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	return *d
}
