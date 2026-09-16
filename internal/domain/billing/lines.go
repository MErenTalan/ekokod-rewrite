package billing

import (
	"encoding/json"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// ToModel maps an Invoice onto its persisted rows. Labels are line codes,
// localised at render time; ComputedAt and ownership ids are the caller's.
func ToModel(inv Invoice, scope model.BillScope) (model.Bill, []model.BillLine, []model.BillHourlyDetail) {
	b := model.Bill{
		Scope: scope, PeriodKey: inv.PeriodKey, PeriodStart: inv.Period.From.UTC(), PeriodEnd: inv.Period.To.UTC(),
		DaysInPeriod: int32(inv.Days), TariffID: inv.TariffID, TariffEffectiveFrom: inv.TariffEffectiveFrom, Currency: inv.Currency,
		ActiveImport: inv.ActiveImport, T1Kwh: inv.T1, T2Kwh: inv.T2, T3Kwh: inv.T3,
		InductiveKvarh: inv.Inductive, CapacitiveKvarh: inv.Capacitive, ActiveExport: inv.ActiveExport,
		NetConsumption: inv.NetConsumption, LowTierKwh: inv.LowTierKwh, HighTierKwh: inv.HighTierKwh,
		MaxDemandKw: inv.MaxDemandKw, TieredApplied: inv.TieredApplied, DemandDataAvailable: inv.DemandDataAvailable,
		IndexStart: indexJSON(inv.IndexStart), IndexEnd: indexJSON(inv.IndexEnd),
		EffectiveEnergyPrice: inv.EffectiveEnergyPrice, LowTierPrice: inv.LowTierPrice, HighTierPrice: inv.HighTierPrice,
		EnergyCost: inv.EnergyCost, DistributionCost: inv.DistributionCost, GreenEnergyCost: inv.GreenEnergyCost,
		PowerCost: inv.PowerCost, DemandOverrunCost: inv.DemandOverrunCost, ReactivePenalty: inv.ReactivePenalty,
		ExtraChargesCost: inv.ExtraChargesCost, OtherTaxesCost: inv.OtherTaxesCost, VatBase: inv.VatBase,
		VatCost: inv.VatCost, GenerationCredit: inv.GenerationCredit, TotalCost: inv.TotalCost,
		InductiveRatio: inv.Reactive.InductiveRatio, CapacitiveRatio: inv.Reactive.CapacitiveRatio,
		InductiveThreshold: inv.Reactive.InductiveLimit, CapacitiveThreshold: inv.Reactive.CapacitiveLimit,
		ReactivePenaltyApplied: inv.Reactive.Applied, ReactivePowerPrice: inv.ReactivePowerPrice,
		GenerationUsage: inv.GenerationUsage, GenerationPricePerKwh: inv.GenerationPricePerKwh,
		PtfYekdemUsed: inv.PtfYekdemUsed, Status: model.BillStatusIssued,
	}
	if len(inv.Flags) > 0 {
		reasons := make([]string, len(inv.Flags))
		for i, f := range inv.Flags {
			reasons[i] = string(f)
		}
		reason := strings.Join(reasons, ",")
		b.Status, b.FlagReason = model.BillStatusFlagged, &reason
	}
	var hourly []model.BillHourlyDetail
	if pr := inv.Pricing; pr != nil {
		expected, matched := int32(pr.HoursExpected), int32(pr.HoursMatched)
		missing, consumptionMissing := expected-matched, int32(pr.HoursMissingConsumption)
		b.PtfHoursExpected, b.PtfHoursMatched, b.PtfHoursMissing, b.ConsumptionHoursMissing = &expected, &matched, &missing, &consumptionMissing
		b.PtfAverage, b.YekdemUsed = pr.PTFAverage, pr.YekdemUsed
		for _, h := range pr.Hourly {
			hourly = append(hourly, model.BillHourlyDetail{Ts: h.Hour, Consumption: h.Kwh, PTF: h.PTF, Yekdem: h.Yekdem, Kbk: h.Kbk, UnitPrice: h.UnitPrice, Cost: h.Cost})
		}
	}
	lines := make([]model.BillLine, len(inv.Lines))
	for i, l := range inv.Lines {
		unit := l.Unit
		lines[i] = model.BillLine{Code: l.Code, Label: l.Code, Quantity: l.Quantity, Unit: &unit, UnitPrice: l.UnitPrice,
			RatePct: l.RatePct, Amount: l.Amount, SortOrder: int16(i)}
	}
	return b, lines, hourly
}

// indexJSON encodes register indexes with decimals as strings (05 §1); nil
// entries stay null and a nil map is the empty object the column requires.
func indexJSON(m map[string]*decimal.Decimal) json.RawMessage {
	out := make(map[string]*string, len(m))
	for k, v := range m {
		if v == nil {
			out[k] = nil
			continue
		}
		s := v.String()
		out[k] = &s
	}
	raw, _ := json.Marshal(out) // a map of strings cannot fail to marshal
	return raw
}
