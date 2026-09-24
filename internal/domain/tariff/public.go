package tariff

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// PublicError names the input the public calculator refuses (F12a R351).
type PublicError struct{ Field, Code string }

func (e *PublicError) Error() string { return "public bill: " + e.Field + ": " + e.Code }

// PublicInput is the public calculator's form (01 §7.19).
type PublicInput struct {
	Group      model.DistributionUserGroup
	Level      model.VoltageLevel
	Term       model.TariffTerm
	MultiTime  bool
	Start, End time.Time
	// Total and T1–T3 follow Q-H4: bands price a multi-time bill, the total a
	// single-time one; both given must agree.
	Total, T1, T2, T3     *decimal.Decimal
	Demand, ContractPower *decimal.Decimal
}

// PublicResult is R352's breakdown; money is rounded to 2 dp after an unrounded sum.
type PublicResult struct {
	Energy, Distribution, Power, Overuse decimal.Decimal
	VatBase, Vat, Total, VatRate         decimal.Decimal
	Days                                 int
	GroupUsed                            model.DistributionUserGroup
	Row                                  model.NationalTariffScheduleEntry
	Basis                                string // total | bands
}

// Picker returns the schedule row in force for a group on the period's start.
type Picker func(model.DistributionUserGroup, model.VoltageLevel, model.TariffTerm) (model.NationalTariffScheduleEntry, bool)

var (
	publicGroups = map[model.DistributionUserGroup]model.DistributionUserGroup{
		model.UserGroupResidential: model.UserGroupResidentialPlus, model.UserGroupCommercial: model.UserGroupCommercialPlus,
		model.UserGroupIndustrial: "", model.UserGroupAgricultural: "", model.UserGroupLighting: "",
	}
	maxValue  = decimal.New(1, 9)
	tolerance = decimal.RequireFromString("0.01")
	thirty    = decimal.NewFromInt(30)
	hundred   = decimal.NewFromInt(100)
)

func refuse(field, code string) error { return &PublicError{Field: field, Code: code} }

func zeroIfNil(v *decimal.Decimal) decimal.Decimal {
	if v == nil {
		return decimal.Zero
	}
	return *v
}

func civilDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// PublicBill is the public estimator: every charge line, the power charge
// included (legacy omitted it from the total and the VAT base).
func PublicBill(in PublicInput, pick Picker) (PublicResult, error) {
	plus, ok := publicGroups[in.Group]
	switch {
	case !ok:
		return PublicResult{}, refuse("group", "invalid")
	case !in.Level.Valid():
		return PublicResult{}, refuse("voltage_level", "invalid")
	case !in.Term.Valid() || in.Term == model.TariffTermBinomial && in.Level != model.VoltageLevelMV:
		return PublicResult{}, refuse("term", "invalid")
	}
	start, end := civilDay(in.Start), civilDay(in.End)
	if end.Before(start) {
		return PublicResult{}, refuse("end", "invalid_range")
	}
	days := int(end.Sub(start).Hours()/24) + 1
	if days > 366 {
		return PublicResult{}, refuse("end", "range_too_long")
	}
	for field, v := range map[string]*decimal.Decimal{"total_consumption": in.Total, "t1": in.T1, "t2": in.T2, "t3": in.T3,
		"demand": in.Demand, "contract_power": in.ContractPower} {
		if v != nil && (v.IsNegative() || !v.LessThan(maxValue)) {
			return PublicResult{}, refuse(field, "out_of_range")
		}
	}
	kwh, basis, err := consumption(in)
	if err != nil {
		return PublicResult{}, err
	}
	row, found := pick(in.Group, in.Level, in.Term)
	if !found {
		return PublicResult{}, refuse("start", "no_tariff")
	}
	used := in.Group
	if row.DailyThresholdKwh != nil && plus != "" && kwh.Div(decimal.NewFromInt(int64(days))).GreaterThan(*row.DailyThresholdKwh) {
		if high, ok := pick(plus, in.Level, in.Term); ok {
			row, used = high, plus
		}
	}
	if in.MultiTime && (row.T1Price == nil || row.T2Price == nil || row.T3Price == nil) {
		return PublicResult{}, refuse("multi_time", "not_available")
	}
	r := PublicResult{Days: days, GroupUsed: used, Row: row, Basis: basis, VatRate: row.VatRate}
	energy := kwh.Mul(row.EnergyPrice)
	if in.MultiTime {
		energy = zeroIfNil(in.T1).Mul(*row.T1Price).Add(zeroIfNil(in.T2).Mul(*row.T2Price)).Add(zeroIfNil(in.T3).Mul(*row.T3Price))
	}
	distribution := kwh.Mul(row.DistributionPrice)
	power, overuse := decimal.Zero, decimal.Zero
	if in.Term == model.TariffTermBinomial {
		if in.ContractPower == nil || !in.ContractPower.IsPositive() {
			return PublicResult{}, refuse("contract_power", "required")
		}
		// Q-H3: a monthly price pro-rated by the period's days.
		power = zeroIfNil(row.PowerPrice).Mul(*in.ContractPower).Mul(decimal.NewFromInt(int64(days))).Div(thirty)
		if in.Demand != nil && in.Demand.GreaterThan(*in.ContractPower) {
			overuse = in.Demand.Sub(*in.ContractPower).Mul(zeroIfNil(row.OverusePrice))
		}
	}
	base := energy.Add(distribution).Add(power).Add(overuse)
	vat := base.Mul(row.VatRate).Div(hundred)
	r.Energy, r.Distribution, r.Power, r.Overuse = energy.Round(2), distribution.Round(2), power.Round(2), overuse.Round(2)
	r.VatBase, r.Vat, r.Total = base.Round(2), vat.Round(2), base.Add(vat).Round(2)
	return r, nil
}

// consumption is Q-H4's documented rule for total against T1–T3.
func consumption(in PublicInput) (decimal.Decimal, string, error) {
	sum := zeroIfNil(in.T1).Add(zeroIfNil(in.T2)).Add(zeroIfNil(in.T3))
	hasBands := sum.IsPositive()
	hasTotal := in.Total != nil && in.Total.IsPositive()
	if hasTotal && hasBands && in.Total.Sub(sum).Abs().GreaterThan(tolerance) {
		return decimal.Zero, "", refuse("total_consumption", "mismatch")
	}
	if in.MultiTime {
		for i, v := range []*decimal.Decimal{in.T1, in.T2, in.T3} {
			if v == nil {
				return decimal.Zero, "", refuse([]string{"t1", "t2", "t3"}[i], "required")
			}
		}
		return sum, "bands", nil
	}
	if hasTotal {
		return *in.Total, "total", nil
	}
	return sum, "total", nil
}

// SchedulePicker picks, per (group, voltage, term), the row with the latest
// effective_from not after on (civil dates).
func SchedulePicker(rows []model.NationalTariffScheduleEntry, on time.Time) Picker {
	day := civilDay(on)
	return func(g model.DistributionUserGroup, l model.VoltageLevel, t model.TariffTerm) (model.NationalTariffScheduleEntry, bool) {
		var best *model.NationalTariffScheduleEntry
		for i := range rows {
			r := &rows[i]
			if r.UserGroup != g || r.VoltageLevel != l || r.Term != t || civilDay(r.EffectiveFrom).After(day) {
				continue
			}
			if best == nil || r.EffectiveFrom.After(best.EffectiveFrom) {
				best = r
			}
		}
		if best == nil {
			return model.NationalTariffScheduleEntry{}, false
		}
		return *best, true
	}
}
