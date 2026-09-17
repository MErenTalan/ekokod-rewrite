package tariff

import (
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// YearMonth keys monthly market data.
type YearMonth struct {
	Year  int
	Month time.Month
}

// Market is the locally stored market data a period needs.
type Market struct {
	PTF          map[time.Time]decimal.Decimal // key: UTC hour start; TL/MWh
	Yekdem       map[YearMonth]decimal.Decimal // published; TL/MWh
	ManualYekdem map[YearMonth]decimal.Decimal // the tariff's own entries
}

// HourConsumption is one sound single-hour Billing row.
type HourConsumption struct {
	Hour time.Time // UTC hour start
	Kwh  decimal.Decimal
}

// HourDetail is one matched hour of the hourly method, unrounded.
type HourDetail struct {
	Hour                                   time.Time
	Kwh, PTF, Yekdem, Kbk, UnitPrice, Cost decimal.Decimal
}

// Method is the PTF pricing method used.
type Method string

// The PTF pricing methods (02 §7.1, §7.2).
const (
	MethodHourly        Method = "hourly"
	MethodPeriodAverage Method = "period_average"
)

// Incompleteness reasons (R109).
const (
	ReasonOverTolerance = "ptf_hours_over_tolerance"
	ReasonYekdemMissing = "yekdem_missing"
	ReasonNoPTF         = "no_ptf"
)

// Pricing is the outcome of PTF+YEKDEM pricing for one period.
type Pricing struct {
	Method                                                   Method
	BasePrice                                                *decimal.Decimal
	EnergyUnitPrice                                          *decimal.Decimal
	T1, T2, T3                                               *decimal.Decimal
	PowerUnitPrice, ReactiveUnitPrice, DistributionUnitPrice *decimal.Decimal
	PTFAverage, YekdemUsed                                   *decimal.Decimal
	HoursExpected, HoursMatched                              int
	HoursMissingPrice, HoursMissingConsumption               int
	Hourly                                                   []HourDetail
	Complete                                                 bool
	Reason                                                   string
}

// ErrNotPTF is returned by Price for a tariff without use_ptf_yekdem.
var ErrNotPTF = errors.New("tariff: not a PTF+YEKDEM tariff")

// ErrInvalidPeriod is returned by Price for an empty or misaligned period.
var ErrInvalidPeriod = errors.New("tariff: invalid pricing period")

var thousand = decimal.NewFromInt(1000)

// Price computes PTF+YEKDEM pricing (02 §7, R108, R109, R119, R124, I-17).
// The hourly method applies only to single-time tariffs with hourly data.
func Price(t model.Tariff, period energy.Window, hours []HourConsumption, hourlyAvailable bool,
	m Market, p model.BillingParameters, loc *time.Location) (Pricing, error) {
	if !t.UsePtfYekdem {
		return Pricing{}, ErrNotPTF
	}
	if !period.Valid() || period.From.Truncate(time.Hour) != period.From || period.To.Truncate(time.Hour) != period.To {
		return Pricing{}, ErrInvalidPeriod
	}
	yekdem := func(ts time.Time) *decimal.Decimal {
		lt := ts.In(loc)
		key := YearMonth{lt.Year(), lt.Month()}
		if t.UseManualYekdem {
			if v, ok := m.ManualYekdem[key]; ok {
				return &v
			}
		}
		if v, ok := m.Yekdem[key]; ok {
			return &v
		}
		return nil
	}

	out := Pricing{Method: MethodPeriodAverage}
	hourly := hourlyAvailable && t.PriceType == model.PriceTypeSingleTime // R124
	if hourly {
		out.Method = MethodHourly
	}
	ptfByHour := make(map[int64]decimal.Decimal, len(m.PTF))
	for ts, v := range m.PTF {
		ptfByHour[ts.UnixNano()] = v
	}
	kwh := make(map[int64]decimal.Decimal, len(hours))
	for _, h := range hours {
		kwh[h.Hour.UnixNano()] = h.Kwh
	}

	var ptfSum, yekdemSum, costSum, kwhSum decimal.Decimal
	priced, yekdemMissing := 0, false
	for h := period.From; h.Before(period.To); h = h.Add(time.Hour) {
		out.HoursExpected++
		y := yekdem(h)
		if y == nil {
			yekdemMissing = true
		} else {
			yekdemSum = yekdemSum.Add(*y)
		}
		ptf, hasPTF := ptfByHour[h.UnixNano()]
		if hasPTF {
			priced++
			ptfSum = ptfSum.Add(ptf)
		}
		if !hourly {
			continue
		}
		k, hasKwh := kwh[h.UnixNano()]
		switch {
		case !hasPTF || y == nil:
			out.HoursMissingPrice++
		case !hasKwh:
			out.HoursMissingConsumption++
		default:
			unit := ptf.Add(*y).DivRound(thousand, DivisionScale).Mul(*t.KbkEnergy)
			cost := k.Mul(unit)
			out.Hourly = append(out.Hourly, HourDetail{Hour: h.UTC(), Kwh: k, PTF: ptf, Yekdem: *y, Kbk: *t.KbkEnergy, UnitPrice: unit, Cost: cost})
			costSum, kwhSum = costSum.Add(cost), kwhSum.Add(k)
			out.HoursMatched++
		}
	}

	if !hourly {
		out.HoursMatched, out.HoursMissingPrice = priced, out.HoursExpected-priced
	}
	expected := decimal.NewFromInt(int64(out.HoursExpected))
	if priced > 0 {
		avg := ptfSum.DivRound(decimal.NewFromInt(int64(priced)), DivisionScale)
		out.PTFAverage = &avg
	}
	if !yekdemMissing {
		w := yekdemSum.DivRound(expected, DivisionScale) // I-17: hour-weighted across months
		out.YekdemUsed = &w
	}
	if out.PTFAverage != nil && out.YekdemUsed != nil {
		base := out.PTFAverage.Add(*out.YekdemUsed).DivRound(thousand, DivisionScale)
		out.BasePrice = &base
	}

	missing := out.HoursExpected - out.HoursMatched
	switch {
	case yekdemMissing:
		out.Reason = ReasonYekdemMissing
	case priced == 0:
		out.Reason = ReasonNoPTF
	case decimal.NewFromInt(int64(missing)).DivRound(expected, DivisionScale).GreaterThan(p.PTFMissingHourTolerance):
		out.Reason = ReasonOverTolerance
	}
	out.Complete = out.Reason == ""

	times := func(coefficient *decimal.Decimal) *decimal.Decimal {
		if out.BasePrice == nil || coefficient == nil {
			return nil // 02 §7.2: a nil coefficient is no line, never a fallback
		}
		v := out.BasePrice.Mul(*coefficient)
		return &v
	}
	out.EnergyUnitPrice = times(t.KbkEnergy)
	if hourly && kwhSum.IsPositive() {
		v := costSum.DivRound(kwhSum, DivisionScale) // R108: consumption-weighted
		out.EnergyUnitPrice = &v
	}
	out.T1, out.T2, out.T3 = times(t.KbkT1), times(t.KbkT2), times(t.KbkT3)

	fixedPower, fixedReactive, fixedDistribution := FixedUnitPrices(t)
	out.PowerUnitPrice, out.ReactiveUnitPrice, out.DistributionUnitPrice = fixedPower, fixedReactive, fixedDistribution
	if t.PowerPriceSource != model.PriceSourceFixed {
		out.PowerUnitPrice = times(t.KbkPowerPrice)
	}
	if t.ReactivePriceSource != model.PriceSourceFixed {
		out.ReactiveUnitPrice = times(t.KbkReactivePower)
	}
	if t.DistributionPriceSource != model.PriceSourceFixed {
		out.DistributionUnitPrice = t.KbkDistributionCostTlPerKwh // flat TL/kWh, not base-derived
	}
	return out, nil
}

// FixedUnitPrices returns the tariff's own power, reactive and distribution
// prices (R119 `fixed`, and every non-PTF tariff).
func FixedUnitPrices(t model.Tariff) (power, reactive, distribution *decimal.Decimal) {
	r, d := t.ReactivePowerPrice, t.DistributionCost
	return t.PowerUnitPrice, &r, &d
}
