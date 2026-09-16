package billing_test

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

const propertySeed = 20260917

// TestComputePropertyInvariants compares billing.Compute with oracleCompute, an
// independent straight-line reading of 02 §6 and the F4 rulings, over random
// tariffs and quantities, plus structural invariants. EKOKOD_PROPERTY_N
// overrides the scenario count.
func TestComputePropertyInvariants(t *testing.T) {
	n := 5000
	if testing.Short() {
		n = 500
	}
	if v := os.Getenv("EKOKOD_PROPERTY_N"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("EKOKOD_PROPERTY_N: %v", err)
		}
		n = parsed
	}
	loc := istanbul(t)
	for i := 0; i < n; i++ {
		rng := rand.New(rand.NewPCG(propertySeed, uint64(i)))
		in := randomInput(rng, loc)
		inv, err := billing.Compute(in)
		if err != nil {
			t.Fatalf("scenario %d: %v", i, err)
		}
		if msg := checkScenario(in, inv); msg != "" {
			t.Fatalf("scenario %d (seed %d,%d): %s", i, propertySeed, i, msg)
		}
	}
}

func checkScenario(in billing.Input, inv billing.Invoice) string {
	want := oracleCompute(in)
	got := map[string]decimal.Decimal{}
	sum := decimal.Zero
	for _, l := range inv.Lines {
		if _, dup := got[l.Code]; dup {
			return "duplicate line " + l.Code
		}
		got[l.Code] = l.Amount
		if !l.Amount.Equal(l.Amount.Truncate(2)) {
			return fmt.Sprintf("line %s amount %s has more than 2 decimals", l.Code, l.Amount)
		}
		if l.Code != model.BillLineVat && l.Code != model.BillLineGenerationCredit {
			sum = sum.Add(l.Amount)
		}
		if l.Code == model.BillLineDistribution && !l.Quantity.Equal(inv.NetConsumption) {
			return "distribution quantity is not the net consumption"
		}
		tier := l.Code == model.BillLineEnergyLowTier || l.Code == model.BillLineEnergyHighTier
		if tier && (in.Tariff.PriceType == model.PriceTypeMultiTime || in.Tariff.UsePtfYekdem ||
			in.Tariff.OverusePrice == nil || !slices.Contains(in.Params.TieringSupplyCompanies, in.Tariff.SupplyCompany)) {
			return "tier line on an untierable tariff"
		}
		if strings.HasPrefix(l.Code, "reactive_") && inv.Reactive.Exempt {
			return "reactive line on an exempt invoice"
		}
	}
	if msg := sameLines(want.lines, got); msg != "" {
		return msg
	}
	for name, pair := range map[string][2]decimal.Decimal{
		"net": {want.net, inv.NetConsumption}, "vat_base": {want.vatBase, inv.VatBase}, "vat": {want.vat, inv.VatCost},
		"credit": {want.credit, inv.GenerationCredit}, "total": {want.total, inv.TotalCost},
		"low_tier": {want.low, inv.LowTierKwh}, "high_tier": {want.high, inv.HighTierKwh},
	} {
		if !pair[0].Equal(pair[1]) {
			return fmt.Sprintf("%s: oracle %s, compute %s", name, pair[0], pair[1])
		}
	}
	if want.tiered != inv.TieredApplied {
		return fmt.Sprintf("tiered: oracle %v, compute %v", want.tiered, inv.TieredApplied)
	}
	if want.demandAvailable != inv.DemandDataAvailable {
		return "demand_data_available differs"
	}
	wantFlags := make([]billing.Flag, 0, len(want.flags))
	for f := range want.flags {
		wantFlags = append(wantFlags, f)
	}
	slices.Sort(wantFlags)
	if !slices.Equal(wantFlags, inv.Flags) {
		return fmt.Sprintf("flags: oracle %v, compute %v", wantFlags, inv.Flags)
	}
	if !sum.Equal(inv.VatBase) {
		return "vat base is not the sum of the non-vat, non-credit lines"
	}
	if !inv.TotalCost.Equal(decimal.Max(decimal.Zero, inv.VatBase.Add(inv.VatCost).Sub(inv.GenerationCredit))) {
		return "total is not max(0, base + vat − credit)"
	}
	if inv.TieredApplied && !inv.LowTierKwh.Add(inv.HighTierKwh).Equal(inv.NetConsumption) {
		return "tiers do not sum to net"
	}
	t := in.Tariff
	if t.UsePtfYekdem {
		for code, kbk := range map[string]*decimal.Decimal{
			model.BillLinePower: kbkIf(t.PowerPriceSource, t.KbkPowerPrice), model.BillLineReactiveInductive: kbkIf(t.ReactivePriceSource, t.KbkReactivePower),
			model.BillLineDistribution: kbkIf(t.DistributionPriceSource, t.KbkDistributionCostTlPerKwh),
		} {
			if _, ok := got[code]; ok && kbk == nil {
				return "a nil kbk-sourced coefficient produced a " + code + " line"
			}
		}
	}
	// Metamorphic pair: a different reactive price moves only the reactive lines and VAT.
	other := in
	if t.UsePtfYekdem {
		pr := *in.Pricing
		pr.ReactiveUnitPrice = decPtr(decimal.NewFromInt(9))
		other.Pricing = &pr
	} else {
		other.Tariff.ReactivePowerPrice = decimal.NewFromInt(9)
	}
	inv2, err := billing.Compute(other)
	if err != nil {
		return err.Error()
	}
	for _, l := range inv2.Lines {
		if strings.HasPrefix(l.Code, "reactive_") || l.Code == model.BillLineVat {
			continue
		}
		if a, ok := got[l.Code]; !ok || !a.Equal(l.Amount) {
			return "changing the reactive price changed line " + l.Code
		}
	}
	return ""
}

func kbkIf(src model.PriceSource, kbk *decimal.Decimal) *decimal.Decimal {
	if src == model.PriceSourceFixed {
		return decPtr(decimal.NewFromInt(1))
	}
	return kbk
}

func sameLines(want, got map[string]decimal.Decimal) string {
	for code, a := range want {
		g, ok := got[code]
		if !ok {
			return "compute is missing line " + code
		}
		if !a.Equal(g) {
			return fmt.Sprintf("line %s: oracle %s, compute %s", code, a, g)
		}
	}
	for code := range got {
		if _, ok := want[code]; !ok {
			return "compute has an extra line " + code
		}
	}
	return ""
}

type oracleOut struct {
	lines                                       map[string]decimal.Decimal
	net, low, high, vatBase, vat, credit, total decimal.Decimal
	tiered, demandAvailable                     bool
	flags                                       map[billing.Flag]bool
}

// oracleRound is R112 written out: kuruş = floor(|x|·100 + ½) (half-up) or
// the even neighbour on an exact half (half-even), sign restored.
func oracleRound(x decimal.Decimal, mode model.MoneyRoundingMode) decimal.Decimal {
	neg := x.Sign() < 0
	scaled := x.Abs().Mul(decimal.NewFromInt(100))
	floor := scaled.Floor()
	frac := scaled.Sub(floor)
	half := decimal.RequireFromString("0.5")
	up := frac.GreaterThan(half) || (frac.Equal(half) && (mode != model.MoneyRoundingHalfEven || !floor.Mod(decimal.NewFromInt(2)).IsZero()))
	if up {
		floor = floor.Add(decimal.NewFromInt(1))
	}
	out := floor.Div(decimal.NewFromInt(100))
	if neg {
		return out.Neg()
	}
	return out
}

func oracleCompute(in billing.Input) oracleOut {
	t, q, p := in.Tariff, in.Quantities, in.Params
	o := oracleOut{lines: map[string]decimal.Decimal{}, flags: map[billing.Flag]bool{}}
	round := func(x decimal.Decimal) decimal.Decimal { return oracleRound(x, p.MoneyRoundingMode) }
	zero := decimal.Zero
	charge := func(code string, amount decimal.Decimal) decimal.Decimal {
		o.lines[code] = amount
		return amount
	}

	exportKwh := zero
	if q.ActiveExport != nil {
		exportKwh = *q.ActiveExport
	} else if t.GenerationUsage != model.GenerationUsageNone {
		o.flags[billing.FlagGenerationDataMissing] = true
	}
	net := *q.ActiveImport
	if t.GenerationUsage == model.GenerationUsageSubtractFromConsumption {
		net = net.Sub(exportKwh)
		if net.IsNegative() {
			net = zero
		}
	}
	o.net = net

	energyCost := zero
	if t.PriceType == model.PriceTypeMultiTime {
		b := []decimal.Decimal{*q.T1, *q.T2, *q.T3}
		if t.GenerationUsage == model.GenerationUsageSubtractFromConsumption {
			total := b[0].Add(b[1]).Add(b[2])
			for i := range b {
				if total.IsZero() {
					b[i] = zero
				} else {
					b[i] = b[i].Mul(net).DivRound(total, 20)
				}
			}
		}
		prices := []*decimal.Decimal{t.T1Price, t.T2Price, t.T3Price}
		if t.UsePtfYekdem {
			prices = []*decimal.Decimal{in.Pricing.T1, in.Pricing.T2, in.Pricing.T3}
		}
		for i, code := range []string{"energy_t1", "energy_t2", "energy_t3"} {
			if prices[i] != nil {
				energyCost = energyCost.Add(charge(code, round(b[i].Mul(*prices[i]))))
			}
		}
	} else if t.UsePtfYekdem {
		if in.Pricing.EnergyUnitPrice != nil {
			energyCost = charge("energy", round(net.Mul(*in.Pricing.EnergyUnitPrice)))
		}
	} else {
		groupThreshold, groupOK := p.TieringGroups[t.UserGroup]
		threshold := groupThreshold
		if t.OveruseThresholdKwhPerDay != nil {
			threshold = *t.OveruseThresholdKwhPerDay
		}
		days := decimal.NewFromInt(int64(in.Days))
		over := groupOK && slices.Contains(p.TieringVoltageLevels, t.VoltageLevel) &&
			slices.Contains(p.TieringSupplyCompanies, t.SupplyCompany) && net.DivRound(days, 20).GreaterThan(threshold)
		switch {
		case !over:
			energyCost = charge("energy", round(net.Mul(*t.SingleTimePrice)))
		case t.OverusePrice == nil:
			o.flags[billing.FlagTieringPriceMissing] = true
			energyCost = charge("energy", round(net.Mul(*t.SingleTimePrice)))
		case p.TieringMode == model.TieringModeWholeConsumptionSwitch:
			o.tiered, o.high = true, net
			energyCost = charge("energy_high_tier", round(net.Mul(*t.OverusePrice)))
		default:
			offset := zero
			if t.GenerationUsage == model.GenerationUsageSubtractFromConsumption {
				offset = exportKwh
			}
			limit := days.Mul(threshold).Sub(offset)
			if limit.IsNegative() {
				limit = zero
			}
			o.tiered = true
			o.low = net
			if limit.LessThan(net) {
				o.low = limit
			}
			o.high = net.Sub(o.low)
			if o.low.IsPositive() {
				energyCost = energyCost.Add(charge("energy_low_tier", round(o.low.Mul(*t.SingleTimePrice))))
			}
			energyCost = energyCost.Add(charge("energy_high_tier", round(o.high.Mul(*t.OverusePrice))))
		}
	}

	distributionPrice := &t.DistributionCost
	powerPrice := t.PowerUnitPrice
	reactivePrice := &t.ReactivePowerPrice
	if t.UsePtfYekdem {
		distributionPrice, powerPrice, reactivePrice = in.Pricing.DistributionUnitPrice, in.Pricing.PowerUnitPrice, in.Pricing.ReactiveUnitPrice
		if !in.Pricing.Complete {
			o.flags[billing.FlagPTFDataMissing] = true
		}
	}
	lineSum := energyCost
	if distributionPrice != nil {
		lineSum = lineSum.Add(charge("distribution", round(net.Mul(*distributionPrice))))
	}
	if t.GreenEnergyPrice != nil || t.GreenEnergyDistributionCost != nil {
		g := zero
		if t.GreenEnergyPrice != nil {
			g = g.Add(*t.GreenEnergyPrice)
		}
		if t.GreenEnergyDistributionCost != nil {
			g = g.Add(*t.GreenEnergyDistributionCost)
		}
		lineSum = lineSum.Add(charge("green_energy", round(net.Mul(g))))
	}
	o.demandAvailable = q.MaxDemandKw != nil
	if t.Term == model.TariffTermBinomial && t.ContractedPowerKw != nil && powerPrice != nil {
		lineSum = lineSum.Add(charge("power", round(t.ContractedPowerKw.Mul(*powerPrice))))
		if q.MaxDemandKw != nil && q.MaxDemandKw.GreaterThan(*t.ContractedPowerKw) {
			excess := q.MaxDemandKw.Sub(*t.ContractedPowerKw)
			lineSum = lineSum.Add(charge("demand_overrun", round(excess.Mul(*powerPrice).Mul(p.DemandOverrunMultiplier))))
		}
	}

	// Reactive (02 §6.6, R117, R118, I-18).
	exempt := slices.Contains(p.ReactiveExemptTerms, t.Term) || slices.Contains(p.ReactiveExemptUserGroups, t.UserGroup) ||
		q.ActiveExportKnownSum.GreaterThan(p.ReactiveGenerationExemptKwh) ||
		(in.InstalledPowerKw != nil && p.ReactiveExemptBelowKw != nil && in.InstalledPowerKw.LessThan(*p.ReactiveExemptBelowKw))
	if !exempt {
		if q.ActiveExport == nil && q.ActiveExportPartial {
			o.flags[billing.FlagGenerationDataMissing] = true
		}
		var band *model.ReactiveBand
		if in.InstalledPowerKw != nil {
			for i := range p.ReactiveBands {
				b := p.ReactiveBands[i]
				if !in.InstalledPowerKw.LessThan(b.MinKw) && (b.MaxKw == nil || in.InstalledPowerKw.LessThan(*b.MaxKw)) {
					band = &b
					break
				}
			}
		}
		if band == nil {
			o.flags[billing.FlagInstalledPowerMissing] = true
		} else {
			for _, side := range []struct {
				code  string
				reg   *decimal.Decimal
				limit decimal.Decimal
			}{{"reactive_inductive", q.ReactiveInductive, band.Inductive}, {"reactive_capacitive", q.ReactiveCapacitive, band.Capacitive}} {
				if side.reg == nil {
					o.flags[billing.FlagReactiveDataMissing] = true
					continue
				}
				var exceeded bool
				if net.IsZero() {
					exceeded = side.reg.IsPositive()
				} else {
					exceeded = side.reg.DivRound(net, 30).GreaterThan(side.limit)
				}
				if !exceeded || reactivePrice == nil {
					continue
				}
				kvarh := *side.reg
				if p.ReactivePenaltyBasis == model.ReactivePenaltyBasisExcessOverLimit {
					kvarh = kvarh.Sub(side.limit.Mul(net))
				}
				if kvarh.IsPositive() {
					lineSum = lineSum.Add(charge(side.code, round(kvarh.Mul(*reactivePrice))))
				}
			}
		}
	}

	extras := slices.Clone(in.ExtraCharges)
	slices.SortStableFunc(extras, func(a, b model.TariffExtraCharge) int { return cmp.Compare(a.SortOrder, b.SortOrder) })
	for _, c := range extras {
		var qty *decimal.Decimal
		switch c.Basis {
		case model.ExtraChargeBasisPerKwh:
			qty = &net
		case model.ExtraChargeBasisPerContractedKw:
			qty = t.ContractedPowerKw
		case model.ExtraChargeBasisPerMaxDemandKw:
			qty = q.MaxDemandKw
			if qty == nil {
				o.demandAvailable = false
			}
		case model.ExtraChargeBasisFixedPerPeriod:
			qty = decPtr(decimal.NewFromInt(1))
		case model.ExtraChargeBasisPctOfEnergy:
			lineSum = lineSum.Add(charge("extra:"+c.Name, round(energyCost.Mul(c.Amount).Div(decimal.NewFromInt(100)))))
			continue
		}
		if qty != nil {
			lineSum = lineSum.Add(charge("extra:"+c.Name, round(qty.Mul(c.Amount))))
		}
	}
	for _, tax := range in.Taxes {
		lineSum = lineSum.Add(charge("tax:"+tax.Name, round(energyCost.Mul(tax.Rate).Div(decimal.NewFromInt(100)))))
	}
	o.vatBase = lineSum
	o.vat = charge("vat", round(lineSum.Mul(t.VatRate).Div(decimal.NewFromInt(100))))
	if t.GenerationUsage == model.GenerationUsageSubtractFromTotal && q.ActiveExport != nil && t.GenerationPricePerKwh != nil {
		o.credit = round(exportKwh.Mul(*t.GenerationPricePerKwh))
		charge("generation_credit", o.credit.Neg())
	}
	o.total = o.vatBase.Add(o.vat).Sub(o.credit)
	if o.total.IsNegative() {
		o.total = zero
	}
	return o
}

func decPtr(v decimal.Decimal) *decimal.Decimal { return &v }

// randomInput draws one scenario across every tariff shape (Task 4 generator).
func randomInput(r *rand.Rand, loc *time.Location) billing.Input {
	dec := func(maxUnits int, places int32) decimal.Decimal {
		return decimal.New(r.Int64N(int64(maxUnits)*pow10(places)+1), -places)
	}
	maybe := func(pct int, v decimal.Decimal) *decimal.Decimal {
		if r.IntN(100) < pct {
			return nil
		}
		return &v
	}
	pick := func(n int) int { return r.IntN(n) }
	groups := model.DistributionUserGroups()

	p := seedParams()
	p.ReactivePenaltyBasis = model.ReactivePenaltyBases()[pick(2)]
	p.TieringMode = model.TieringModes()[pick(2)]
	p.MoneyRoundingMode = model.MoneyRoundingModes()[pick(2)]
	p.DemandOverrunMultiplier = dec(3, 2)
	p.ReactiveGenerationExemptKwh = dec(2, 1)
	if pick(5) == 0 {
		p.ReactiveExemptBelowKw = nil
		p.ReactiveBands[0].MinKw = decimal.Zero
	}
	p.ReactiveExemptTerms = subset(r, model.TariffTerms())
	p.ReactiveExemptUserGroups = subset(r, groups)
	p.TieringVoltageLevels = subset(r, model.VoltageLevels())
	p.TieringSupplyCompanies = subset(r, model.SupplyCompanies())

	t := model.Tariff{
		ID: uuid.New(), Currency: model.CurrencyTRY, EnergyType: model.EnergyTypeGrid,
		VoltageLevel: model.VoltageLevels()[pick(2)], UserGroup: groups[pick(len(groups))],
		PriceType: model.PriceTypes()[pick(2)], Term: model.TariffTerms()[pick(2)], SupplyCompany: model.SupplyCompanies()[pick(2)],
		GenerationUsage: model.GenerationUsages()[pick(3)], VatRate: dec(25, 1),
		DistributionCost: dec(2, 6), ReactivePowerPrice: dec(4, 4),
		UsePtfYekdem:     pick(10) < 3,
		PowerPriceSource: model.PriceSources()[pick(2)], ReactivePriceSource: model.PriceSources()[pick(2)], DistributionPriceSource: model.PriceSources()[pick(2)],
	}
	if t.GenerationUsage == model.GenerationUsageSubtractFromTotal {
		t.GenerationPricePerKwh = decPtr(dec(3, 4))
	}
	if pick(5) == 0 {
		t.GreenEnergyPrice, t.GreenEnergyDistributionCost = maybe(30, dec(1, 4)), maybe(30, dec(1, 4))
	}
	if t.Term == model.TariffTermBinomial {
		t.ContractedPowerKw, t.PowerUnitPrice = decPtr(dec(500, 1)), maybe(10, dec(60, 4))
	}
	if t.UsePtfYekdem {
		t.KbkEnergy = decPtr(dec(2, 4))
		t.KbkT1, t.KbkT2, t.KbkT3 = maybe(20, dec(2, 4)), maybe(20, dec(2, 4)), maybe(20, dec(2, 4))
		t.KbkPowerPrice, t.KbkReactivePower, t.KbkDistributionCostTlPerKwh = maybe(30, dec(40, 4)), maybe(30, dec(2, 4)), maybe(30, dec(2, 6))
	} else if t.PriceType == model.PriceTypeSingleTime {
		t.SingleTimePrice = decPtr(dec(5, 6))
		t.OverusePrice = maybe(25, dec(7, 6))
		if pick(5) == 0 {
			t.OveruseThresholdKwhPerDay = decPtr(dec(40, 1).Add(decimal.NewFromInt(1)))
		}
	} else {
		t.T1Price, t.T2Price, t.T3Price = decPtr(dec(5, 6)), decPtr(dec(5, 6)), decPtr(dec(5, 6))
		t.OverusePrice = maybe(25, dec(7, 6))
	}

	days := 28 + pick(4)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	imp := dec(20000, 3)
	if pick(20) == 0 {
		imp = decimal.Zero
	}
	q := billing.Quantities{ActiveImport: &imp}
	t1 := imp.Mul(decimal.New(r.Int64N(1001), -3))
	t2 := imp.Sub(t1).Mul(decimal.New(r.Int64N(1001), -3))
	t3 := imp.Sub(t1).Sub(t2).Add(dec(50, 2))
	q.T1, q.T2, q.T3 = &t1, &t2, &t3
	q.ReactiveInductive = maybe(10, imp.Mul(decimal.New(r.Int64N(800), -3)))
	q.ReactiveCapacitive = maybe(10, imp.Mul(decimal.New(r.Int64N(400), -3)))
	if pick(7) == 0 && q.ReactiveInductive != nil {
		q.ReactiveInductive = decPtr(dec(3, 2))
	}
	switch pick(6) {
	case 0:
		q.ActiveExportKnownSum, q.ActiveExportPartial = dec(3, 1), pick(2) == 0
	case 1, 2:
		e := dec(3, 2)
		q.ActiveExport, q.ActiveExportKnownSum = &e, e
	default:
		e := imp.Mul(decimal.New(r.Int64N(1300), -3))
		q.ActiveExport, q.ActiveExportKnownSum = &e, e
	}
	if pick(5) != 0 {
		q.MaxDemandKw = decPtr(dec(700, 2))
	}

	in := billing.Input{
		PeriodKey: "2026-01", Period: energy.Window{From: from, To: from.AddDate(0, 0, days)}, Days: days,
		Tariff: t, Params: p, Quantities: q, InstalledPowerKw: maybe(10, dec(80, 2)),
	}
	for i := range pick(4) {
		in.Taxes = append(in.Taxes, model.TariffTax{Name: fmt.Sprintf("T%d", i), Rate: dec(10, 3), SortOrder: int16(pick(3))})
	}
	for i := range pick(4) {
		basis := model.ExtraChargeBases()[pick(5)]
		if basis == model.ExtraChargeBasisPerContractedKw && t.ContractedPowerKw == nil {
			basis = model.ExtraChargeBasisPerKwh
		}
		in.ExtraCharges = append(in.ExtraCharges, model.TariffExtraCharge{Name: fmt.Sprintf("X%d", i), Basis: basis,
			Amount: dec(60, 3).Sub(decimal.NewFromInt(10)), SortOrder: int16(pick(3))})
	}
	if t.UsePtfYekdem {
		base := dec(4, 6)
		times := func(k *decimal.Decimal) *decimal.Decimal {
			if k == nil {
				return nil
			}
			return decPtr(base.Mul(*k))
		}
		pr := tariff.Pricing{Method: tariff.MethodPeriodAverage, BasePrice: &base, EnergyUnitPrice: times(t.KbkEnergy),
			T1: times(t.KbkT1), T2: times(t.KbkT2), T3: times(t.KbkT3), Complete: pick(5) != 0}
		if pick(2) == 0 {
			pr.EnergyUnitPrice = decPtr(dec(5, 20)) // an hourly consumption-weighted price
		}
		pr.PowerUnitPrice, pr.ReactiveUnitPrice, pr.DistributionUnitPrice = tariff.FixedUnitPrices(t)
		if t.PowerPriceSource == model.PriceSourceKbk {
			pr.PowerUnitPrice = times(t.KbkPowerPrice)
		}
		if t.ReactivePriceSource == model.PriceSourceKbk {
			pr.ReactiveUnitPrice = times(t.KbkReactivePower)
		}
		if t.DistributionPriceSource == model.PriceSourceKbk {
			pr.DistributionUnitPrice = t.KbkDistributionCostTlPerKwh
		}
		in.Pricing = &pr
	}
	return in
}

func subset[T any](r *rand.Rand, all []T) []T {
	var out []T
	for _, v := range all {
		if r.IntN(2) == 0 {
			out = append(out, v)
		}
	}
	return out
}

func pow10(n int32) int64 {
	out := int64(1)
	for range n {
		out *= 10
	}
	return out
}
