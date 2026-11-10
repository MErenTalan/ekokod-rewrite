package tariff

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

var half = decimal.RequireFromString("0.5")

// priceSKTT is the Tebliğ formula (RG 30307 md. 6, R567), proven against
// three MEPAŞ bills to 0.000032 TL/kWh:
//
//	SKTT = (PTF_d + YEKDEM_d) × KBK
//
// PTF_d averages EPİAŞ's daily volume-weighted PTF over the read dates,
// both included at half weight (so the weights sum to the day count).
// YEKDEM_d uses the same weights over the EPDK forecast (R566), falling
// back to the tariff's manual entry first and the published value last.
// Consumption does not weight anything: hourly readings are not needed.
func priceSKTT(t model.Tariff, period energy.Window, m Market, p model.BillingParameters, loc *time.Location) Pricing {
	out := Pricing{Method: MethodSKTT}
	for h := period.From; h.Before(period.To); h = h.Add(time.Hour) {
		out.HoursExpected++
		if _, ok := m.PTF[h.UTC()]; ok {
			out.HoursMatched++
		}
	}
	out.HoursMissingPrice = out.HoursExpected - out.HoursMatched

	first, last := localDate(period.From, loc), localDate(period.To, loc)
	var ptfSum, ptfWeight, yekdemSum, totalWeight decimal.Decimal
	yekdemMissing := false
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		w := decimal.NewFromInt(1)
		if !first.Equal(last) && (day.Equal(first) || day.Equal(last)) {
			w = half
		}
		totalWeight = totalWeight.Add(w)
		if v, ok := dailyWeightedPTF(day, m); ok {
			ptfSum, ptfWeight = ptfSum.Add(v.Mul(w)), ptfWeight.Add(w)
		}
		if y, ok := skttYekdem(t, m, YearMonth{day.Year(), day.Month()}); ok {
			yekdemSum = yekdemSum.Add(y.Mul(w))
		} else {
			yekdemMissing = true
		}
	}

	if ptfWeight.IsPositive() {
		avg := ptfSum.DivRound(ptfWeight, DivisionScale)
		out.PTFAverage = &avg
	}
	if !yekdemMissing {
		y := yekdemSum.DivRound(totalWeight, DivisionScale)
		out.YekdemUsed = &y
	}
	if out.PTFAverage != nil && out.YekdemUsed != nil {
		base := out.PTFAverage.Add(*out.YekdemUsed).DivRound(thousand, DivisionScale)
		out.BasePrice = &base
	}
	missing := totalWeight.Sub(ptfWeight).DivRound(totalWeight, DivisionScale)
	switch {
	case yekdemMissing:
		out.Reason = ReasonYekdemMissing
	case ptfWeight.IsZero():
		out.Reason = ReasonNoPTF
	case missing.GreaterThan(p.PTFMissingHourTolerance):
		out.Reason = ReasonOverTolerance
	}
	return out
}

// dailyWeightedPTF is EPİAŞ's "ağırlıklı ortalama günlük PTF" for one
// Istanbul day: Σ quantity·PTF / Σ quantity over every hour, rounded to
// kuruş as EPİAŞ publishes it. Any hour without both values → no value.
func dailyWeightedPTF(day time.Time, m Market) (decimal.Decimal, bool) {
	var sum, qty decimal.Decimal
	for h := day; h.Before(day.AddDate(0, 0, 1)); h = h.Add(time.Hour) {
		ptf, ok1 := m.PTF[h.UTC()]
		q, ok2 := m.Quantity[h.UTC()]
		if !ok1 || !ok2 {
			return decimal.Decimal{}, false
		}
		sum, qty = sum.Add(ptf.Mul(q)), qty.Add(q)
	}
	if !qty.IsPositive() {
		return decimal.Decimal{}, false
	}
	return sum.DivRound(qty, DivisionScale).Round(2), true
}

func skttYekdem(t model.Tariff, m Market, key YearMonth) (decimal.Decimal, bool) {
	if t.UseManualYekdem {
		if v, ok := m.ManualYekdem[key]; ok {
			return v, true
		}
	}
	if v, ok := m.YekdemForecast[key]; ok {
		return v, true
	}
	v, ok := m.Yekdem[key]
	return v, ok
}

// localDate is t's Istanbul calendar date at midnight.
func localDate(t time.Time, loc *time.Location) time.Time {
	lt := t.In(loc)
	return time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, loc)
}
