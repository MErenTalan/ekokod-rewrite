package billing

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/reactive"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

// Quantities are one invoice subject's period registers.
type Quantities struct {
	ActiveImport                          *decimal.Decimal
	T1, T2, T3                            *decimal.Decimal
	ReactiveInductive, ReactiveCapacitive *decimal.Decimal
	ActiveExport                          *decimal.Decimal
	ActiveExportKnownSum                  decimal.Decimal // I-18: sum over members that reported export
	ActiveExportPartial                   bool            // I-18: some members reported export, some did not
	MaxDemandKw                           *decimal.Decimal
	IndexStart, IndexEnd                  map[string]*decimal.Decimal
}

// Input is everything one invoice computation needs.
type Input struct {
	PeriodKey        string
	Period           energy.Window
	Days             int
	Tariff           model.Tariff
	Taxes            []model.TariffTax
	ExtraCharges     []model.TariffExtraCharge
	Params           model.BillingParameters
	Quantities       Quantities
	InstalledPowerKw *decimal.Decimal
	Pricing          *tariff.Pricing // required iff Tariff.UsePtfYekdem
}

// Line is one invoice line; Amount is rounded (R112).
type Line struct {
	Code      string
	Quantity  *decimal.Decimal
	Unit      string
	UnitPrice *decimal.Decimal
	RatePct   *decimal.Decimal
	Amount    decimal.Decimal
}

// Line units.
const (
	UnitKwh    = "kWh"
	UnitKvarh  = "kVArh"
	UnitKw     = "kW"
	UnitPeriod = "period"
	UnitPct    = "%"
)

// Invoice is a computed invoice.
type Invoice struct {
	PeriodKey string
	Period    energy.Window
	Days      int

	TariffID            *uuid.UUID
	TariffEffectiveFrom *time.Time
	Currency            model.CurrencyCode

	ActiveImport, T1, T2, T3, Inductive, Capacitive, ActiveExport, NetConsumption decimal.Decimal
	LowTierKwh, HighTierKwh                                                       decimal.Decimal
	TieredApplied                                                                 bool
	MaxDemandKw                                                                   *decimal.Decimal
	DemandDataAvailable                                                           bool
	IndexStart, IndexEnd                                                          map[string]*decimal.Decimal

	EffectiveEnergyPrice, LowTierPrice, HighTierPrice *decimal.Decimal

	EnergyCost, DistributionCost, GreenEnergyCost, PowerCost, DemandOverrunCost, ReactivePenalty,
	ExtraChargesCost, OtherTaxesCost, VatBase, VatCost, GenerationCredit, TotalCost decimal.Decimal

	Reactive           reactive.Result
	ReactivePowerPrice *decimal.Decimal

	GenerationUsage       model.GenerationUsage
	GenerationPricePerKwh *decimal.Decimal

	PtfYekdemUsed bool
	Pricing       *tariff.Pricing

	Lines []Line
	Flags []Flag
}

// Compute prices one invoice. It fails only for input it cannot price at all
// (R113); every other data problem is a Flag.
func Compute(in Input) (Invoice, error) {
	t, q, p := in.Tariff, in.Quantities, in.Params
	switch {
	case q.ActiveImport == nil:
		return Invoice{}, fmt.Errorf("%w: active import is nil", ErrInvalidInput)
	case t.PriceType == model.PriceTypeMultiTime && (q.T1 == nil || q.T2 == nil || q.T3 == nil):
		return Invoice{}, fmt.Errorf("%w: multi-time tariff with a nil band register", ErrInvalidInput)
	case t.UsePtfYekdem && in.Pricing == nil:
		return Invoice{}, fmt.Errorf("%w: PTF tariff without pricing", ErrInvalidInput)
	case in.Days <= 0:
		return Invoice{}, fmt.Errorf("%w: days %d", ErrInvalidInput, in.Days)
	}
	mode := p.MoneyRoundingMode
	flags := map[Flag]bool{}
	inv := Invoice{
		PeriodKey: in.PeriodKey, Period: in.Period, Days: in.Days,
		Currency: t.Currency, MaxDemandKw: q.MaxDemandKw, DemandDataAvailable: q.MaxDemandKw != nil,
		IndexStart: q.IndexStart, IndexEnd: q.IndexEnd,
		ActiveImport: *q.ActiveImport, T1: val(q.T1), T2: val(q.T2), T3: val(q.T3),
		Inductive: val(q.ReactiveInductive), Capacitive: val(q.ReactiveCapacitive),
		GenerationUsage: t.GenerationUsage, GenerationPricePerKwh: t.GenerationPricePerKwh,
		PtfYekdemUsed: t.UsePtfYekdem, Pricing: in.Pricing,
	}
	if t.ID != uuid.Nil {
		id, eff := t.ID, t.EffectiveFrom
		inv.TariffID, inv.TariffEffectiveFrom = &id, &eff
	}
	add := func(l Line) decimal.Decimal {
		inv.Lines = append(inv.Lines, l)
		return l.Amount
	}
	priced := func(code string, qty decimal.Decimal, unit string, price decimal.Decimal) Line {
		return Line{Code: code, Quantity: &qty, Unit: unit, UnitPrice: &price, Amount: Round2(qty.Mul(price), mode)}
	}

	// 1. Net (02 §6.1).
	export := decimal.Zero
	if q.ActiveExport != nil {
		export = *q.ActiveExport
	} else if t.GenerationUsage != model.GenerationUsageNone {
		flags[FlagGenerationDataMissing] = true
	}
	inv.ActiveExport = export
	net := inv.ActiveImport
	if t.GenerationUsage == model.GenerationUsageSubtractFromConsumption {
		net = decimal.Max(decimal.Zero, net.Sub(export))
	}
	inv.NetConsumption = net

	// 2. Energy.
	energyRaw := decimal.Zero
	energyLine := func(code string, qty decimal.Decimal, price *decimal.Decimal) {
		if price == nil {
			return
		}
		energyRaw = energyRaw.Add(qty.Mul(*price))
		inv.EnergyCost = inv.EnergyCost.Add(add(priced(code, qty, UnitKwh, *price)))
	}
	bands := func() (b1, b2, b3 decimal.Decimal) {
		b1, b2, b3 = *q.T1, *q.T2, *q.T3
		if t.GenerationUsage != model.GenerationUsageSubtractFromConsumption {
			return b1, b2, b3
		}
		sum := b1.Add(b2).Add(b3)
		if sum.IsZero() {
			return decimal.Zero, decimal.Zero, decimal.Zero
		}
		scale := func(b decimal.Decimal) decimal.Decimal { return b.Mul(net).DivRound(sum, tariff.DivisionScale) } // R123
		return scale(b1), scale(b2), scale(b3)
	}
	switch {
	case t.PriceType == model.PriceTypeMultiTime:
		b1, b2, b3 := bands()
		p1, p2, p3 := t.T1Price, t.T2Price, t.T3Price
		if t.UsePtfYekdem { // R124
			p1, p2, p3 = in.Pricing.T1, in.Pricing.T2, in.Pricing.T3
		}
		energyLine(model.BillLineEnergyT1, b1, p1)
		energyLine(model.BillLineEnergyT2, b2, p2)
		energyLine(model.BillLineEnergyT3, b3, p3)
	case t.UsePtfYekdem: // R120: never tiered
		energyLine(model.BillLineEnergy, net, in.Pricing.EnergyUnitPrice)
	default:
		threshold, gated := tieringThreshold(t, p)
		wouldTier := gated && net.DivRound(decimal.NewFromInt(int64(in.Days)), tariff.DivisionScale).GreaterThan(threshold)
		switch {
		case wouldTier && t.OverusePrice == nil: // C-3: never a zero-price high tier
			flags[FlagTieringPriceMissing] = true
			energyLine(model.BillLineEnergy, net, t.SingleTimePrice)
		case wouldTier && p.TieringMode == model.TieringModeWholeConsumptionSwitch:
			inv.TieredApplied, inv.HighTierKwh = true, net
			inv.LowTierPrice, inv.HighTierPrice = t.SingleTimePrice, t.OverusePrice
			energyLine(model.BillLineEnergyHighTier, net, t.OverusePrice)
		case wouldTier:
			offset := decimal.Zero
			if t.GenerationUsage == model.GenerationUsageSubtractFromConsumption {
				offset = export // R122: spec formula kept verbatim
			}
			limit := decimal.Max(decimal.Zero, decimal.NewFromInt(int64(in.Days)).Mul(threshold).Sub(offset))
			inv.TieredApplied = true
			inv.LowTierKwh = decimal.Min(net, limit)
			inv.HighTierKwh = decimal.Max(decimal.Zero, net.Sub(inv.LowTierKwh))
			inv.LowTierPrice, inv.HighTierPrice = t.SingleTimePrice, t.OverusePrice
			if inv.LowTierKwh.IsPositive() {
				energyLine(model.BillLineEnergyLowTier, inv.LowTierKwh, t.SingleTimePrice)
			}
			energyLine(model.BillLineEnergyHighTier, inv.HighTierKwh, t.OverusePrice)
		default:
			energyLine(model.BillLineEnergy, net, t.SingleTimePrice)
		}
	}
	if net.IsPositive() {
		v := energyRaw.DivRound(net, tariff.DivisionScale)
		inv.EffectiveEnergyPrice = &v
	}

	fixedPower, fixedReactive, fixedDistribution := tariff.FixedUnitPrices(t)
	powerPrice, reactivePrice, distributionPrice := fixedPower, fixedReactive, fixedDistribution
	if t.UsePtfYekdem { // R119
		powerPrice, reactivePrice, distributionPrice = in.Pricing.PowerUnitPrice, in.Pricing.ReactiveUnitPrice, in.Pricing.DistributionUnitPrice
		if !in.Pricing.Complete {
			flags[FlagPTFDataMissing] = true
		}
	}

	// 3. Distribution on all net (removed-behaviour 3).
	if distributionPrice != nil {
		inv.DistributionCost = add(priced(model.BillLineDistribution, net, UnitKwh, *distributionPrice))
	}

	// 4. Green energy.
	if t.GreenEnergyPrice != nil || t.GreenEnergyDistributionCost != nil {
		inv.GreenEnergyCost = add(priced(model.BillLineGreenEnergy, net, UnitKwh, val(t.GreenEnergyPrice).Add(val(t.GreenEnergyDistributionCost))))
	}

	// 5. Power and overrun (R111, R125).
	if t.Term == model.TariffTermBinomial && t.ContractedPowerKw != nil && powerPrice != nil {
		contracted := *t.ContractedPowerKw
		inv.PowerCost = add(priced(model.BillLinePower, contracted, UnitKw, *powerPrice))
		if q.MaxDemandKw != nil && q.MaxDemandKw.GreaterThan(contracted) {
			inv.DemandOverrunCost = add(priced(model.BillLineDemandOverrun, q.MaxDemandKw.Sub(contracted), UnitKw, powerPrice.Mul(p.DemandOverrunMultiplier)))
		}
	}

	// 6. Reactive.
	inv.ReactivePowerPrice = reactivePrice
	inv.Reactive = reactive.Evaluate(reactive.Input{
		NetConsumption: net, Inductive: q.ReactiveInductive, Capacitive: q.ReactiveCapacitive,
		ActiveExport: q.ActiveExport, ActiveExportKnownSum: q.ActiveExportKnownSum, ActiveExportPartial: q.ActiveExportPartial,
		InstalledPowerKw: in.InstalledPowerKw, Term: t.Term, UserGroup: t.UserGroup, UnitPrice: reactivePrice, Params: p,
	})
	for _, m := range inv.Reactive.Missing {
		switch m {
		case reactive.MissingInstalledPower:
			flags[FlagInstalledPowerMissing] = true
		case reactive.MissingActiveExport:
			flags[FlagGenerationDataMissing] = true
		default:
			flags[FlagReactiveDataMissing] = true
		}
	}
	if reactivePrice != nil {
		for _, side := range []struct {
			code    string
			charged *decimal.Decimal
		}{{model.BillLineReactiveInductive, inv.Reactive.InductiveKvarhCharged}, {model.BillLineReactiveCapacitive, inv.Reactive.CapacitiveKvarhCharged}} {
			if side.charged != nil && side.charged.IsPositive() {
				inv.ReactivePenalty = inv.ReactivePenalty.Add(add(priced(side.code, *side.charged, UnitKvarh, *reactivePrice)))
			}
		}
	}

	// 7. Extra charges (R126), inside the VAT base.
	extras := slices.Clone(in.ExtraCharges)
	slices.SortStableFunc(extras, func(a, b model.TariffExtraCharge) int { return cmp.Compare(a.SortOrder, b.SortOrder) })
	for _, c := range extras {
		code := model.BillLineExtraPrefix + c.Name
		var l Line
		switch c.Basis {
		case model.ExtraChargeBasisPerKwh:
			l = priced(code, net, UnitKwh, c.Amount)
		case model.ExtraChargeBasisPerContractedKw:
			if t.ContractedPowerKw == nil {
				continue
			}
			l = priced(code, *t.ContractedPowerKw, UnitKw, c.Amount)
		case model.ExtraChargeBasisPerMaxDemandKw:
			if q.MaxDemandKw == nil {
				inv.DemandDataAvailable = false
				continue
			}
			l = priced(code, *q.MaxDemandKw, UnitKw, c.Amount)
		case model.ExtraChargeBasisFixedPerPeriod:
			l = priced(code, decimal.NewFromInt(1), UnitPeriod, c.Amount)
		case model.ExtraChargeBasisPctOfEnergy:
			rate := c.Amount
			l = Line{Code: code, Unit: UnitPct, RatePct: &rate, Amount: Round2(inv.EnergyCost.Mul(rate).DivRound(hundred, tariff.DivisionScale), mode)}
		default:
			continue
		}
		inv.ExtraChargesCost = inv.ExtraChargesCost.Add(add(l))
	}

	// 8. Named taxes on the rounded energy cost (R112).
	taxes := slices.Clone(in.Taxes)
	slices.SortStableFunc(taxes, func(a, b model.TariffTax) int { return cmp.Compare(a.SortOrder, b.SortOrder) })
	for _, tax := range taxes {
		rate := tax.Rate
		inv.OtherTaxesCost = inv.OtherTaxesCost.Add(add(Line{
			Code: model.BillLineTaxPrefix + tax.Name, Unit: UnitPct, RatePct: &rate,
			Amount: Round2(inv.EnergyCost.Mul(rate).DivRound(hundred, tariff.DivisionScale), mode),
		}))
	}

	// 9. VAT on the sum of rounded lines.
	for _, l := range inv.Lines {
		inv.VatBase = inv.VatBase.Add(l.Amount)
	}
	vatRate := t.VatRate
	inv.VatCost = add(Line{Code: model.BillLineVat, Unit: UnitPct, RatePct: &vatRate,
		Amount: Round2(inv.VatBase.Mul(vatRate).DivRound(hundred, tariff.DivisionScale), mode)})

	// 10. Generation credit after VAT, then total.
	if t.GenerationUsage == model.GenerationUsageSubtractFromTotal && q.ActiveExport != nil && t.GenerationPricePerKwh != nil {
		credit := priced(model.BillLineGenerationCredit, export, UnitKwh, *t.GenerationPricePerKwh)
		inv.GenerationCredit = credit.Amount
		credit.Amount = credit.Amount.Neg()
		add(credit)
	}
	inv.TotalCost = decimal.Max(decimal.Zero, inv.VatBase.Add(inv.VatCost).Sub(inv.GenerationCredit))
	inv.Flags = sortedFlags(flags)
	return inv, nil
}

// tieringThreshold applies R120's gates for a fixed single-time tariff and
// returns the kWh/day threshold (the tariff's override wins).
func tieringThreshold(t model.Tariff, p model.BillingParameters) (decimal.Decimal, bool) {
	groupThreshold, inGroup := p.TieringGroups[t.UserGroup]
	if t.PriceType != model.PriceTypeSingleTime || !inGroup ||
		!slices.Contains(p.TieringVoltageLevels, t.VoltageLevel) || !slices.Contains(p.TieringSupplyCompanies, t.SupplyCompany) {
		return decimal.Zero, false
	}
	if t.OveruseThresholdKwhPerDay != nil {
		return *t.OveruseThresholdKwhPerDay, true
	}
	return groupThreshold, true
}

func val(d *decimal.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	return *d
}
